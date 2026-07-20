// Copyright (C) 2025 Thinline Dynamic Solutions
//
// location_narrative.go — universal guards for POI / chain-restaurant names and
// other place tokens used as narration subjects ("OLIVE GARDEN ADVISES…") rather
// than dispatch locations ("AT OLIVE GARDEN", "OLIVE GARDEN, 123 MAIN").

package mapping

import (
	"regexp"
	"strings"
)

var narrativeVerbAfterPlaceRE = regexp.MustCompile(
	`(?i)\b(\d{1,6})\s+(?:[A-Z][A-Z'\-]+\s+){1,4}(ADVISES|ADVISED|PUSHING|PUSHED|TOWARD|TOWARDS|HEADING|HEADED|MOVING|MOVED|PULLING|PULLED|REPORTED|REPORTING|SAID|SAYS|SAYING)\b`,
)

var narrativeVerbTokens = map[string]bool{
	"ADVISES": true, "ADVISED": true, "PUSHING": true, "PUSHED": true,
	"TOWARD": true, "TOWARDS": true, "HEADING": true, "HEADED": true,
	"MOVING": true, "MOVED": true, "PULLING": true, "PULLED": true,
	"REPORTED": true, "REPORTING": true, "SAID": true, "SAYS": true, "SAYING": true,
}

// PlaceMentionIsNarrativeSubject reports when a facility/POI name is followed by
// a status verb ("OLIVE GARDEN ADVISES") instead of a dispatch location cue.
func PlaceMentionIsNarrativeSubject(transcript, placeName string) bool {
	name := strings.ToUpper(strings.TrimSpace(placeName))
	if name == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	idx := strings.Index(u, name)
	if idx < 0 {
		return false
	}
	after := strings.TrimSpace(u[idx+len(name):])
	if after == "" {
		return false
	}
	first := strings.Fields(after)
	if len(first) == 0 {
		return false
	}
	return narrativeVerbTokens[strings.TrimRight(first[0], ".,;:")]
}

// PlaceMentionIsDispatchContext reports when a known place is named as a dispatch
// destination rather than the grammatical subject of unit narration.
func PlaceMentionIsDispatchContext(transcript, placeName string) bool {
	if PlaceMentionIsNarrativeSubject(transcript, placeName) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	name := strings.ToUpper(strings.TrimSpace(placeName))
	if !strings.Contains(u, name) {
		return false
	}
	for _, prep := range []string{" AT " + name, " TO " + name, " FOR " + name, " NEAR " + name, " BY " + name} {
		if strings.Contains(u, prep) {
			return true
		}
	}
	if idx := strings.Index(u, name+","); idx >= 0 {
		rest := strings.TrimSpace(u[idx+len(name)+1:])
		if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
			return true
		}
	}
	if containsRealIncidentMarker(u) || stationDispatchRE.MatchString(transcript) {
		return strings.Contains(u, " AT "+name) || strings.Contains(u, name+",")
	}
	return false
}

// AddressIsPlaceNarrativeNotDispatch reports when a house+place extract is
// immediately followed by narration in the transcript ("66 OLIVE GARDEN ADVISES").
func AddressIsPlaceNarrativeNotDispatch(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	// Thoroughfare-suffixed streets are dispatch locations. After punctuation is
	// stripped to spaces, "7814 BEEMAN. SAYS THAT…" becomes "7814 BEEMAN SAYS"
	// and would look like a POI narrating — keep the pin.
	if hasStreetSuffix(street) || streetHasGeocodableSuffix(street) {
		return false
	}
	u := strings.ToUpper(transcript)
	prefix := house + " " + street
	if idx := strings.Index(u, prefix); idx >= 0 {
		rest := strings.TrimSpace(u[idx+len(prefix):])
		if words := strings.Fields(rest); len(words) > 0 &&
			narrativeVerbTokens[strings.TrimRight(words[0], ".,;:")] {
			return true
		}
	}
	// House-scoped only — a blanket transcript scan matched unrelated openers
	// like "3-7. WE'RE HEADED TO 4908 CENTRAL" as if 4908 itself were narrative.
	pat := `(?i)\b` + regexp.QuoteMeta(house) + `\s+(?:[A-Z][A-Z'\-]+\s+){1,4}` +
		`(ADVISES|ADVISED|PUSHING|PUSHED|TOWARD|TOWARDS|HEADING|HEADED|MOVING|MOVED|PULLING|PULLED|REPORTED|REPORTING|SAID|SAYS|SAYING)\b`
	re := regexp.MustCompile(pat)
	loc := re.FindStringIndex(u)
	if loc == nil {
		return false
	}
	// "1050 SOUTH GREEN CALLER SAID…" is dispatch + reporting-party clause, not
	// a POI speaking ("OLIVE GARDEN SAID…").
	span := u[loc[0]:loc[1]]
	if strings.Contains(span, "CALLER") || strings.Contains(span, "COLLAR") {
		return false
	}
	return true
}

// suffixlessCaptureHasObjectPhraseTail reports when a suffixless "<house> <word>"
// harvest is immediately followed by an object phrase ("1718 CONNECTION WITH
// THIS ARREST") rather than a street continuation or dispatch nature clause.
func suffixlessCaptureHasObjectPhraseTail(house, street, transcript string) bool {
	house = strings.TrimSpace(strings.ToUpper(house))
	street = strings.TrimSpace(strings.ToUpper(street))
	if house == "" || street == "" || len(strings.Fields(street)) != 1 {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, sep := range []string{" ", ", "} {
		prefix := house + sep + street
		idx := strings.Index(u, prefix)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(u[idx+len(prefix):])
		rest = strings.TrimLeft(rest, ".,;:")
		words := strings.Fields(rest)
		if len(words) < 2 || words[0] != "WITH" {
			continue
		}
		switch words[1] {
		case "THIS", "THAT", "THE", "A", "AN", "MY", "YOUR", "HIS", "HER", "THEIR":
			return true
		}
	}
	return false
}

// suffixlessMatchFollowedByNarrativeVerb rejects "<house> <words>" location-screen
// hits when the transcript uses those words as a narration subject.
func suffixlessMatchFollowedByNarrativeVerb(house, streetWords, transcript string) bool {
	house = strings.TrimSpace(strings.ToUpper(house))
	streetWords = strings.TrimSpace(strings.ToUpper(streetWords))
	if house == "" || streetWords == "" {
		return false
	}
	if suffixlessCaptureHasObjectPhraseTail(house, streetWords, transcript) {
		return true
	}
	return AddressIsPlaceNarrativeNotDispatch(house+" "+streetWords, transcript)
}

// parkingLotOfAddressRE matches dispatches that name a street address as the
// parking-lot destination ("PARKING LOT OF 1737 SOUTH RACCOON") rather than
// unit status narration about already being in a lot.
var parkingLotOfAddressRE = regexp.MustCompile(`(?i)\bPARKING\s+LOT\s+OF\s+(\d{1,6})\s+[A-Z]`)

// TranscriptParkingLotQualifiesDispatch reports when "PARKING LOT" introduces a
// house-number street address instead of unit movement/status at a lot.
func TranscriptParkingLotQualifiesDispatch(transcript string) bool {
	return parkingLotOfAddressRE.MatchString(transcript)
}

// TranscriptIsLocationNarrative reports unit narration about movement or status at
// a commercial/POI area without a toned dispatch to a new incident.
func TranscriptIsLocationNarrative(transcript string) bool {
	if strings.TrimSpace(transcript) == "" {
		return false
	}
	if TranscriptParkingLotQualifiesDispatch(transcript) {
		return false
	}
	// A clean house-number street ("331 ROBBINS AVENUE … PARKING LOT") is a
	// real dispatch location; "PARKING LOT" / movement cues after that must
	// not wipe the pin (live: Niles PD call 3709).
	if transcriptHasCleanNumberedDispatchAddress(transcript) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	if containsRealIncidentMarker(u) || stationDispatchRE.MatchString(transcript) {
		return false
	}
	for _, m := range []string{
		" ADVISES ", " ADVISED ", " PUSHING ", " TOWARDS THE ", " TOWARD THE ",
		" PARKING LOT", " STILL IN THAT ", " IN THAT AREA ", " ACTUALLY ",
		" THEY'RE STILL ", " THEY ARE STILL ", " MOVING THE ", " HEADED TO ",
		" HEADING TO ",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return narrativeVerbAfterPlaceRE.MatchString(transcript)
}
