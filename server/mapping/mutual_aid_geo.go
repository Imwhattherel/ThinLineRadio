// Copyright (C) 2025 Thinline Dynamic Solutions
//
// mutual_aid_geo.go — when a home department tones mutual aid to another
// jurisdiction, forward-geocode bias must follow the destination city, not the
// dispatching tone set (alert 6915 / call 159947 regressions).

package mapping

import (
	"log"
	"regexp"
	"strings"
)

// mutualAidViaCityRE matches mutual-aid routing via an address with a spoken
// destination city after the street ("18 MUTUAL AID VIA 601 WEST PARK AVENUE, HUBBARD VIA …").
var (
	mutualAidViaCityRE = regexp.MustCompile(`(?i)MUTUAL\s+AID\s+VIA\s+.+?,\s*([A-Z][A-Z\-]{2,20})\b(?:\s+VIA|\s*,|\s+FOR\b|\s+TIME|\s+\d|$)`)
	// MESPO/MESCO fire-district shorthand on mutual-aid tones ("MUTUAL AID, MESCO 534").
	mutualAidMespoRE = regexp.MustCompile(`(?i)\bMESC?O(?:\s+\d{2,4})?\b`)
)

// MutualAidJurisdictionHint returns a city anchor such as "Garrettsville, Ohio"
// when the transcript names mutual aid to another jurisdiction.
func MutualAidJurisdictionHint(transcript string, geo *GeoOptions) string {
	city := peerAgencyRequestCity(transcript)
	if city == "" {
		city = mutualAidJurisdictionCity(transcript)
	}
	return formatLocalityWithState(city, geo)
}

// peerAgencyRequestCity returns a destination city when another agency requests
// apparatus without speaking "mutual aid" (STT "NALL CITY" for Niles City).
func peerAgencyRequestCity(transcript string) string {
	t := strings.ToUpper(strings.TrimSpace(transcript))
	if strings.Contains(t, "NALL CITY") {
		return "NILES CITY"
	}
	return ""
}

// TranscriptIsPeerAgencySquadRequest reports when another jurisdiction's police
// or fire agency requests apparatus without speaking a street address (Warren
// 167191 "NALL CITY POLICE … MULTIPLE SQUADS").
func TranscriptIsPeerAgencySquadRequest(transcript string) bool {
	t := strings.ToUpper(strings.TrimSpace(transcript))
	if peerAgencyRequestCity(transcript) != "" && strings.Contains(t, "SQUAD") {
		return true
	}
	return strings.Contains(t, "REQUEST COMES FROM") && strings.Contains(t, "POLICE") &&
		strings.Contains(t, "SQUAD")
}

// mutualAidPhraseAliases are the transcript spellings of "mutual aid" this
// package treats equivalently: the real phrase, and "MUTOID" — a documented
// Whisper mis-hearing that compresses "MUTUAL AID" into one garbled word
// ("MUTOID REQUEST" for "MUTUAL AID REQUEST"). Without recognizing the
// garbled form, the destination-city override below never triggers and the
// pin gets checked against the wrong (home-jurisdiction) coverage area
// entirely.
var mutualAidPhraseAliases = []string{"MUTUAL AID", "MUTOID"}

func transcriptHasMutualAidMarker(t string) bool {
	for _, alias := range mutualAidPhraseAliases {
		if strings.Contains(t, alias) {
			return true
		}
	}
	return false
}

func mutualAidJurisdictionCity(transcript string) string {
	t := strings.ToUpper(strings.TrimSpace(transcript))
	if !transcriptHasMutualAidMarker(t) {
		return ""
	}
	if mutualAidMespoRE.MatchString(t) {
		return "MESPO"
	}
	for _, phrase := range mutualAidPhraseAliases {
		for _, suffix := range []string{
			" FOR ", " TO ", " INTO ", " REQUESTS ",
			// Bare "MUTUAL AID REQUEST, <city>, <location>" — the dispatcher
			// states the request then the destination jurisdiction directly,
			// with no FOR/TO/INTO connector.
			" REQUEST, ", " REQUEST ",
		} {
			if city := extractCityAfterMarker(t, phrase+suffix); city != "" {
				return city
			}
		}
	}
	if city := mutualAidViaDestinationCity(t); city != "" {
		return city
	}
	// "MUTUAL AID TO GARRETTVILLE …" — only look for TO in a short window after
	// the mutual-aid marker. Searching the whole remainder falsely treats patient
	// narrative ("ALLERGIC REACTION TO HIS NEW MEDICATION") as a destination city
	// and clears home coverage bounds.
	for _, alias := range mutualAidPhraseAliases {
		if idx := strings.Index(t, alias); idx >= 0 {
			window := t[idx:]
			if len(window) > 96 {
				window = window[:96]
			}
			if city := extractCityAfterMarker(window, " TO "); city != "" {
				return city
			}
		}
	}
	// Fallback: "... IN <city>..." (e.g. mutual aid into structure in Howland).
	if idx := strings.Index(t, " IN "); idx >= 0 {
		rest := strings.TrimSpace(t[idx+4:])
		if rest == "" {
			return ""
		}
		end := len(rest)
		if i := strings.Index(rest, ","); i >= 0 {
			end = i
		}
		for _, stop := range []string{" TIME ", " STATION", " FOR "} {
			if i := strings.Index(rest[:end], stop); i >= 0 && i < end {
				end = i
			}
		}
		city := strings.TrimSpace(rest[:end])
		if len(city) < 3 || len(strings.Fields(city)) > 4 || startsWithDigit(city) {
			return ""
		}
		if strings.Contains(city, "STRUCTURE") || strings.Contains(city, "BASEMENT") {
			return ""
		}
		if fields := strings.Fields(city); len(fields) >= 2 && isAllDigits(fields[1]) {
			city = fields[0]
		}
		return city
	}
	return ""
}

func formatLocalityWithState(city string, geo *GeoOptions) string {
	city = strings.TrimSpace(city)
	if city == "" {
		return ""
	}
	state := ""
	if geo != nil {
		state = strings.TrimSpace(geo.State)
		if state == "" {
			state = DeriveState(geo.LocationContext, "", geo.CityHint)
		}
	}
	if state != "" {
		return city + ", " + state
	}
	return city
}

func mutualAidViaDestinationCity(t string) string {
	for _, m := range mutualAidViaCityRE.FindAllStringSubmatch(t, -1) {
		if len(m) != 2 {
			continue
		}
		city := strings.TrimSpace(m[1])
		if mutualAidCityPlausible(city) {
			return city
		}
	}
	return ""
}

func mutualAidCityPlausible(city string) bool {
	if len(city) < 3 || len(strings.Fields(city)) > 3 || startsWithDigit(city) {
		return false
	}
	for _, w := range strings.Fields(city) {
		if dispatchLocalityStopwords[w] {
			return false
		}
	}
	for _, bad := range []string{
		"STRUCTURE", "BASEMENT", "ALARM", "FIRE", "AVENUE", "STREET", "ROAD", "DRIVE",
		"MEDICATION", "ALLERGIC", "REACTION", "PATIENT", "FEMALE", "MALE", "YEAR",
	} {
		if strings.Contains(city, bad) {
			return false
		}
	}
	fields := strings.Fields(city)
	if len(fields) > 0 {
		switch fields[0] {
		case "HIS", "HER", "THEIR", "A", "AN", "THE", "POSSIBLE", "NEW":
			return false
		}
	}
	return dispatchLocalityNamePlausible(city)
}

// startsWithDigit reports whether s's first character is a digit. A real
// city/community name is never numeric-led, but a garbled house number/
// address fragment ("8 SOUTH STATE" from STT-split "38 SOUTH STATE" glued to
// an unrelated "MUTUAL"/"MUTUALATED" upstream) looks exactly like one to the
// loose word-window heuristics below — reject it before it can be mistaken
// for a mutual-aid destination city, which would otherwise zero out the home
// coverage bounds entirely and let an unrelated same-named street hundreds of
// miles away pass every downstream coverage check unchallenged.
func startsWithDigit(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && s[0] >= '0' && s[0] <= '9'
}

func extractCityAfterMarker(t, marker string) string {
	idx := strings.Index(t, marker)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(t[idx+len(marker):])
	if rest == "" {
		return ""
	}
	end := len(rest)
	if i := strings.Index(rest, ","); i >= 0 {
		end = i
	}
	for _, stop := range []string{" TIME ", " STATION", " AT ", " ON ", " FOR "} {
		if i := strings.Index(rest[:end], stop); i >= 0 {
			end = i
		}
	}
	city := strings.TrimSpace(rest[:end])
	// "MUTUAL AID REQUEST FOR MACA" — after "REQUEST " the capture still
	// starts with FOR/TO/INTO; strip those connectors before treating the
	// remainder as a city.
	for {
		fields := strings.Fields(city)
		if len(fields) == 0 {
			return ""
		}
		switch fields[0] {
		case "FOR", "TO", "INTO", "THE":
			city = strings.TrimSpace(strings.Join(fields[1:], " "))
			continue
		}
		break
	}
	if fields := strings.Fields(city); len(fields) >= 2 && isAllDigits(fields[1]) {
		city = fields[0]
	}
	city = normalizeMutualAidSpokenCitySTT(city)
	if len(city) < 3 || len(strings.Fields(city)) > 3 || startsWithDigit(city) {
		return ""
	}
	for _, bad := range []string{"STRUCTURE", "BASEMENT", "ALARM", "FIRE", "ODOR", "ORDER", "GAS", "LEAK", "SMOKE"} {
		if strings.Contains(city, bad) {
			return ""
		}
	}
	if !mutualAidCityPlausible(city) {
		return ""
	}
	return city
}

// normalizeMutualAidSpokenCitySTT folds common Whisper misses for destination
// jurisdictions (MACA→MECCA) without street-specific hardcodes.
func normalizeMutualAidSpokenCitySTT(city string) string {
	city = strings.ToUpper(strings.TrimSpace(city))
	switch city {
	case "MACA", "MECKA", "MEKA":
		return "MECCA"
	}
	return city
}

// ApplyMutualAidGeoOverride returns a copy of geo with home tone-set bounds
// cleared and CityHint set to the mutual-aid destination when the transcript
// names one. The input geo is never mutated.
func ApplyMutualAidGeoOverride(geo *GeoOptions, transcript string) *GeoOptions {
	if geo == nil {
		return nil
	}
	ma := MutualAidJurisdictionHint(transcript, geo)
	if ma == "" {
		ma = AssistingJurisdictionHint(transcript, geo)
	}
	if ma == "" {
		return geo
	}
	cp := *geo
	cp.LocationContext = ""
	cp.CityHint = ma
	cp.DispatchSpokenLocality = strings.TrimSpace(strings.SplitN(ma, ",", 2)[0])
	if dest := mutualAidDestinationBounds(cp.DispatchSpokenLocality, geo.MutualAidDestinations); dest != nil {
		cp.BoundsLat = dest.Lat
		cp.BoundsLon = dest.Lon
		cp.BoundsRadiusMi = dest.RadiusMi
		log.Printf("[INFO] geocode: mutual-aid cityHint=%q bounds=%.4f,%.4f r=%.1f",
			cp.CityHint, cp.BoundsLat, cp.BoundsLon, cp.BoundsRadiusMi)
	} else {
		cp.BoundsLat = 0
		cp.BoundsLon = 0
		cp.BoundsRadiusMi = 0
		log.Printf("[INFO] geocode: mutual-aid cityHint=%q (cleared home bounds)", cp.CityHint)
	}
	return &cp
}

func mutualAidDestinationBounds(spokenCity string, dests []MutualAidDestination) *MutualAidDestination {
	spoken := normalizeMutualAidCityLabel(spokenCity)
	if spoken == "" || len(dests) == 0 {
		return nil
	}
	var best *MutualAidDestination
	for i := range dests {
		d := &dests[i]
		if d.Lat == 0 && d.Lon == 0 {
			continue
		}
		if !mutualAidCityLabelsMatch(spoken, d.CityLabel) {
			continue
		}
		if best == nil || d.RadiusMi > best.RadiusMi {
			best = d
		}
	}
	return best
}

func normalizeMutualAidCityLabel(city string) string {
	city = strings.ToUpper(strings.TrimSpace(city))
	city = strings.TrimSuffix(city, ", OH")
	city = strings.TrimSuffix(city, ", OHIO")
	city = strings.ReplaceAll(city, ",", " ")
	city = strings.Join(strings.Fields(city), " ")
	city = strings.TrimSuffix(city, " CITY")
	city = strings.TrimSuffix(city, " TOWNSHIP")
	city = strings.TrimSuffix(city, " TWP")
	return strings.TrimSpace(city)
}

func mutualAidCityLabelsMatch(spoken, configured string) bool {
	a := normalizeMutualAidCityLabel(spoken)
	b := normalizeMutualAidCityLabel(configured)
	if a == "" || b == "" {
		return false
	}
	if a == b || strings.HasPrefix(a, b+" ") || strings.HasPrefix(b, a+" ") {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

var assistingLocalityRE = regexp.MustCompile(`(?i)\bASSISTING\s+([A-Z][A-Z\-]{3,20})\b`)

// AssistingJurisdictionHint returns a city anchor from "ASSISTING <jurisdiction>"
// mutual-aid vehicle/fire dispatches ("ASSISTING COITSVILLE, 91 … ROAD").
func AssistingJurisdictionHint(transcript string, geo *GeoOptions) string {
	m := assistingLocalityRE.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(transcript)))
	if len(m) != 2 {
		return ""
	}
	city := strings.TrimSpace(m[1])
	if !dispatchLocalityNamePlausible(city) {
		return ""
	}
	return formatLocalityWithState(city, geo)
}
