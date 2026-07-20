// Copyright (C) 2025 Thinline Dynamic Solutions
//
// known_places.go — Per–street-group facility pins for failover geocoding.

package mapping

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
)

var (
	geocodeUnitSuffixRE   = regexp.MustCompile(`(?i)\s+(?:ROOM|RM|APT|APARTMENT|UNIT|STE|SUITE|#)\s*#?\s*[A-Z0-9][A-Z0-9\-]*\s*$`)
	geocodeUnitDanglingRE = regexp.MustCompile(`(?i)\s+(?:ROOM|RM|APT|APARTMENT|UNIT|STE|SUITE)\s*$`)

	// Common facility-name words that should NOT count as distinctive identifiers
	// in the rare-word fallback (otherwise ASSISTED, ESTATES etc. would match too
	// many places).
	commonFacilityWords = map[string]bool{
		"ASSISTED": true, "LIVING": true, "ESTATES": true, "CENTER": true,
		"COMMUNITY": true, "HEALTHCARE": true, "HOSPITAL": true, "MEMORIAL": true,
		"FACILITY": true, "BUILDING": true, "APARTMENTS": true, "APARTMENT": true,
		"COMPLEX": true, "VILLAGE": true, "MANOR": true, "RESIDENCE": true,
		"SUITES": true, "TOWERS": true, "CHURCH": true, "SCHOOL": true,
		"SCHOOLS": true, "ACADEMY": true, "HEIGHTS": true, "COMPANY": true,
		"SERVICES": true, "MEDICAL": true, "WORKSHOP": true, "SHELTERED": true,
		"INFORMATION": true, "LOBBY": true, "OFFICE": true, "OFFICES": true,
		"ADMINISTRATION": true, "DEPARTMENT": true, "DEPARTMENTS": true,
	}
)

func isCommonFacilityWord(w string) bool { return commonFacilityWords[w] }

// NormalizePlaceKey canonicalizes a facility name for lookup (THE WINDSOR HOUSE → WINDSOR HOUSE).
func NormalizePlaceKey(name string) string {
	s := strings.ToUpper(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, ",", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if strings.HasPrefix(s, "THE ") {
		s = strings.TrimSpace(s[4:])
	}
	return strings.TrimSpace(s)
}

// stripGeocodeUnitSuffixes removes trailing room/unit tokens for street forward geocode
// so pins match CAD facility-level coordinates (not room-level Google hits).
func stripGeocodeUnitSuffixes(address string) string {
	s := strings.TrimSpace(address)
	for {
		next := geocodeUnitSuffixRE.ReplaceAllString(s, "")
		next = geocodeUnitDanglingRE.ReplaceAllString(next, "")
		next = strings.TrimSpace(next)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// stripTrailingAddressUnit normalizes an address by removing any trailing
// apt/room/unit identifier, including bare numeric tails that immediately
// follow a street type or directional (e.g. "200 E GLENDOLA AVE 10",
// "632 CHAMPION ST E 10A"). Used both at write time (so we stop ingesting
// dirty CAD address text into street_group_places) and during the one-shot
// cleanup pass over existing rows.
//
// Safe by construction — leaves untouched:
//   - intersection keys ("X AND Y" or "X & Y")
//   - addresses ending in a street type or directional with no tail
//   - route numbers ("3379 ST RTE 46" — the trailing "46" follows the route
//     marker "RTE", not a street type / directional)
//   - facility names and any other non-address strings (returned as-is when
//     the trailing token isn't a recognized unit identifier)
func stripTrailingAddressUnit(addr string) string {
	s := stripGeocodeUnitSuffixes(addr)
	upper := strings.ToUpper(s)
	if strings.Contains(upper, " AND ") || strings.Contains(upper, " & ") {
		return s
	}
	fields := strings.Fields(upper)
	if len(fields) < 3 {
		return s
	}

	streetTypes := map[string]bool{
		"RD": true, "ROAD": true,
		"ST": true, "STREET": true,
		"AVE": true, "AVENUE": true,
		"DR": true, "DRIVE": true,
		"LN": true, "LANE": true,
		"CT": true, "COURT": true,
		"BLVD": true, "BOULEVARD": true,
		"PL": true, "PLACE": true,
		"TRL": true, "TRAIL": true,
		"HWY": true, "HIGHWAY": true,
		"PKWY": true, "PARKWAY": true,
		"CIR": true, "CIRCLE": true,
		"TERR": true, "TERRACE": true,
		"WAY": true,
	}
	dirs := map[string]bool{
		"N": true, "S": true, "E": true, "W": true,
		"NE": true, "NW": true, "SE": true, "SW": true,
	}
	// Route markers signal that the following token is part of the route name,
	// not a unit identifier (e.g. "ST RTE 46", "US RTE 422").
	routeMarkers := map[string]bool{"RTE": true, "ROUTE": true, "SR": true, "CR": true, "US": true}

	last := fields[len(fields)-1]
	secondLast := fields[len(fields)-2]

	if streetTypes[last] || dirs[last] {
		return s
	}
	if routeMarkers[secondLast] {
		return s
	}
	if !isBareUnitToken(last) {
		return s
	}
	if !streetTypes[secondLast] && !dirs[secondLast] {
		return s
	}
	return strings.Join(fields[:len(fields)-1], " ")
}

// isBareUnitToken reports whether a token looks like an apt/room/unit
// identifier sitting at the end of an address with no preceding marker
// (e.g. "10", "10A", "2B", "5"). Conservative: short, alphanumeric, and
// MUST contain at least one digit so we never confuse a street letter
// (e.g. "MAIN ST B") for a unit suffix.
func isBareUnitToken(t string) bool {
	n := len(t)
	if n == 0 || n > 5 {
		return false
	}
	hasDigit := false
	for _, r := range t {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'A' && r <= 'Z':
			// allowed
		default:
			return false
		}
	}
	return hasDigit
}

// GeocodeQueryAddress returns the address string to send to Google for lat/lon (no unit/room).
func GeocodeQueryAddress(curated *CuratedAlert) string {
	base := strings.TrimSpace(curated.Address)
	if base == "" {
		return ""
	}
	if apt := strings.TrimSpace(curated.AptUnit); apt != "" {
		u := strings.ToUpper(apt)
		if !strings.Contains(strings.ToUpper(base), u) {
			base = strings.TrimSpace(base + " " + apt)
		}
	}
	return stripGeocodeUnitSuffixes(base)
}

// applyKnownPlaceCoords sets lat/lon from street_group_places when common_name or
// street address matches a learned CAD pin. When the match was on common name
// (e.g. "WINDSOR HOUSE"), the CAD-authoritative AddressHint also overwrites
// the STT-mangled street in curated.Address — STT regularly mishears facility
// street names (GLENDOLA → MENDOZA) and we shouldn't ship a wrong street just
// because we got the right coords. Apartment/unit suffixes from curated.AptUnit
// are preserved by the processor's reattach step.
//
// Returns (applied, fromCAD). fromCAD is true when the matched place was
// learned from Active911 (Source=="active911") — callers use that signal to
// give the resulting pin the same protections as a911Used coords (e.g. the
// downstream cross-street sanity check must not treat a CAD-derived pin as
// a possibly-wrong forward-geocode result that needs overriding).
func applyKnownPlaceCoords(scope *ScopeData, curated *CuratedAlert, allowAddressKeys bool, geo *GeoOptions) (applied, fromCAD bool) {
	if scope == nil || curated == nil {
		return false, false
	}
	commonNameKey := strings.TrimSpace(curated.CommonName)
	// The common-name key matches named facilities ("FIRE STATION 5"). The
	// address keys match auto-learned pins keyed by the street address itself.
	// The local (OSM) engine disables address keys because those pins are
	// stale/ambiguous (carried over from the prior external geocoder) and would
	// shadow the authoritative imported OSM street + address-point data.
	keys := []string{commonNameKey}
	if allowAddressKeys {
		keys = append(keys,
			NormalizePlaceKey(GeocodeQueryAddress(curated)),
			NormalizePlaceKey(curated.Address),
		)
	}
	seen := map[string]bool{}
	for idx, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		place := scope.LookupKnownPlace(k)
		if place == nil {
			continue
		}
		if idx == 0 {
			if knownPlaceConflictsWithCuratedAddress(curated, place) {
				continue
			}
			// Single-word OSM POI names must not override a failed/bogus address.
			name := strings.ToUpper(strings.TrimSpace(place.DisplayName))
			if len(strings.Fields(name)) == 1 && len(name) < 10 {
				continue
			}
		} else if knownPlaceAddressKeyConflicts(curated, place) {
			continue
		}
		if geo != nil && PinOutsideCoverage(place.Lat, place.Lon, geo) {
			continue
		}
		curated.Lat = fmt.Sprintf("%.6f", place.Lat)
		curated.Lon = fmt.Sprintf("%.6f", place.Lon)
		log.Printf("[INFO] known place: %q → lat=%.6f lon=%.6f (group=%d source=%s)",
			place.DisplayName, place.Lat, place.Lon, place.GroupID, place.Source)
		if idx == 0 {
			applyKnownPlaceAddressHint(curated, place)
		}
		return true, strings.EqualFold(strings.TrimSpace(place.Source), "active911")
	}
	return false, false
}

// applyKnownPlaceAddressHint overwrites curated.Address's street portion with
// the known place's CAD-derived AddressHint while preserving the dispatcher-
// stated house number. Used after a common-name match (or transcript facility
// match) so STT mangling of the street name (e.g. GLENDOLA → MENDOZA) doesn't
// ship with the email. No-op when the hint is empty, isn't a real address, or
// curated already has the canonical street. Universal — uses only the place's
// stored hint, no per-department data.
func applyKnownPlaceAddressHint(curated *CuratedAlert, place *KnownPlace) {
	if curated == nil || place == nil {
		return
	}
	hint := strings.TrimSpace(stripTrailingAddressUnit(place.AddressHint))
	if hint == "" {
		return
	}
	hHint, stHint := splitHouseAndStreet(strings.ToUpper(hint))
	if hHint == "" || stHint == "" {
		return // hint isn't a real "<house> <street>" address — facility-only
	}
	base := strings.ToUpper(strings.TrimSpace(stripTrailingAddressUnit(curated.Address)))
	hCur, stCur := splitHouseAndStreet(base)
	if stCur != "" && canonicalStreetTokens(stCur) == canonicalStreetTokens(stHint) {
		return // street already matches the hint
	}
	newAddr := strings.ToUpper(hint)
	if hCur != "" && hCur != hHint {
		newAddr = hCur + " " + strings.ToUpper(stHint)
	}
	log.Printf("[INFO] known place: replaced STT-mangled address %q with CAD-hinted street %q (from place %q)",
		curated.Address, newAddr, place.DisplayName)
	curated.Address = newAddr
}

// applyKnownIntersectionPin looks up a stored CAD intersection pin keyed by
// "<A> AND <B>" (canonical street-token form) and applies it to curated. Used
// for transcripts/extractions that resolve to an intersection without a house
// number. Returns true if a pin was applied.
func applyKnownIntersectionPin(scope *ScopeData, curated *CuratedAlert, intersectionQueries []string) bool {
	if scope == nil || curated == nil || len(intersectionQueries) == 0 {
		return false
	}
	if curated.Lat != "" && curated.Lon != "" {
		return false
	}
	places := scope.ListKnownPlaces()
	if len(places) == 0 {
		return false
	}
	for _, q := range intersectionQueries {
		canonQ := canonicalStreetTokens(q)
		for i := range places {
			p := places[i]
			pk := strings.ToUpper(strings.TrimSpace(p.PlaceKey))
			if !strings.Contains(pk, " AND ") && !strings.Contains(pk, " & ") {
				continue
			}
			canonP := canonicalStreetTokens(strings.ReplaceAll(pk, " & ", " AND "))
			if canonP == canonQ || canonsIntersectionMatch(canonQ, canonP) {
				curated.Lat = fmt.Sprintf("%.6f", p.Lat)
				curated.Lon = fmt.Sprintf("%.6f", p.Lon)
				log.Printf("[INFO] known intersection pin: %q → lat=%.6f lon=%.6f (group=%d)", p.DisplayName, p.Lat, p.Lon, p.GroupID)
				return true
			}
		}
	}
	return false
}

// findKnownStreetIntersectionPinFromTranscript scans the transcript for ANY two
// known streets in this group that co-occur (within ~200 chars) AND have a
// stored intersection pin in the group's place set. Universal — driven by
// per-group known-street list and CAD-learned intersection pins only.
//
// Matches each known street using four relaxations:
//   1. canonical form (handles ROAD↔RD etc.)
//   2. canonical without trailing thoroughfare type or directional (handles
//      "WEST PARK" in transcript matching stored "WEST PARK AVE")
//   3. canonical with all internal whitespace removed (handles STT splitting
//      compound names like "AUSTINTOWN" → "AUSTIN TOWN")
//   4. bare route number (handles "422" in transcript matching "US RTE 422 NW")
//
// When exactly ONE known street is found in transcript and the group has
// exactly ONE intersection pin involving that street, the pin is returned if
// (curated.Address has no house number) OR the transcript shows intersection
// indicators near the street ("NEAR <street>", "<street> AND", "AND <street>",
// "AT <street>"). Pass curated=nil to disable the single-street fallback.
func findKnownStreetIntersectionPinFromTranscript(scope *ScopeData, transcript string, knownStreets []string, curated *CuratedAlert) *KnownPlace {
	if scope == nil || len(knownStreets) == 0 {
		return nil
	}
	places := scope.ListKnownPlaces()
	if len(places) == 0 {
		return nil
	}
	intersectionPins := make([]KnownPlace, 0, len(places))
	for _, p := range places {
		k := strings.ToUpper(p.PlaceKey)
		if strings.Contains(k, " AND ") || strings.Contains(k, " & ") {
			intersectionPins = append(intersectionPins, p)
		}
	}
	if len(intersectionPins) == 0 {
		return nil
	}
	t := canonicalStreetTokens(transcript)
	tNoSpace := strings.ReplaceAll(t, " ", "")
	hits := []streetHit{}
	seenCanon := map[string]bool{}
	for _, s := range knownStreets {
		c := canonicalStreetTokens(s)
		if c == "" || seenCanon[c] {
			continue
		}
		seenCanon[c] = true
		// Full canonical: word-bounded substring so "SHANK" doesn't match
		// inside "SHANKSTOWN".
		if i := wordBoundedIndex(t, c); i >= 0 {
			hits = append(hits, streetHit{name: c, idx: i})
			continue
		}
		stripped := streetWithoutThoroughfare(c)
		matched := false
		if stripped != c && stripped != "" && len(stripped) >= 5 {
			if i := wordBoundedIndex(t, stripped); i >= 0 {
				hits = append(hits, streetHit{name: c, idx: i})
				matched = true
			}
		}
		if matched {
			continue
		}
		// Whitespace-stripped (for STT-split compound names like "AUSTINTOWN" →
		// "AUSTIN TOWN"). Try full and thoroughfare-stripped forms.
		for _, form := range []string{c, stripped} {
			if form == "" {
				continue
			}
			formNoSpace := strings.ReplaceAll(form, " ", "")
			if len(formNoSpace) < 10 {
				continue
			}
			if i := strings.Index(tNoSpace, formNoSpace); i >= 0 {
				hits = append(hits, streetHit{name: c, idx: i})
				matched = true
				break
			}
		}
	}
	// Consolidate hits that refer to the same physical reference in the
	// transcript (same idx, or stripped form overlap). We keep the longest
	// canonical form for each cluster so the intersection-pin lookup uses the
	// most specific name.
	hits = consolidateStreetHits(hits)
	// Two-street co-occurrence path.
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			a, b := hits[i].name, hits[j].name
			if abs(hits[i].idx-hits[j].idx) > 200 {
				continue
			}
			for k := range intersectionPins {
				p := intersectionPins[k]
				canonP := canonicalStreetTokens(strings.ReplaceAll(strings.ToUpper(p.PlaceKey), " & ", " AND "))
				if canonsIntersectionMatch(a+" AND "+b, canonP) || canonsIntersectionMatch(b+" AND "+a, canonP) {
					return &p
				}
			}
		}
	}
	// Single-street fallback: only fire when there's exactly one intersection
	// pin in the group involving the matched street AND the transcript clearly
	// implies an intersection.
	if len(hits) == 1 && curated != nil {
		streetCanon := hits[0].name
		var matchPin *KnownPlace
		matches := 0
		for k := range intersectionPins {
			p := intersectionPins[k]
			canonP := canonicalStreetTokens(strings.ReplaceAll(strings.ToUpper(p.PlaceKey), " & ", " AND "))
			if intersectionPinContainsStreet(canonP, streetCanon) {
				matches++
				matchPin = &intersectionPins[k]
			}
		}
		if matches == 1 && matchPin != nil {
			curatedHasHouse := false
			if h, _ := splitHouseAndStreet(strings.ToUpper(curated.Address)); h != "" {
				curatedHasHouse = true
			}
			if !curatedHasHouse || transcriptHasIntersectionIndicator(t, streetCanon) {
				return matchPin
			}
		}
	}
	return nil
}

// intersectionPinContainsStreet reports whether a canonical intersection key
// "A AND B" has streetCanon as one of its sides (using the same relaxations
// as the transcript scan).
func intersectionPinContainsStreet(intersectionCanon, streetCanon string) bool {
	a, b := splitIntersectionQuery(intersectionCanon)
	if a == "" || b == "" {
		return false
	}
	for _, side := range []string{a, b} {
		if side == streetCanon {
			return true
		}
		if streetWithoutThoroughfare(side) == streetCanon || streetWithoutThoroughfare(streetCanon) == side {
			return true
		}
		if streetWithoutThoroughfare(side) == streetWithoutThoroughfare(streetCanon) && streetWithoutThoroughfare(side) != "" {
			return true
		}
		if rc := streetRouteCore(side); rc != "" && rc == streetCanon {
			return true
		}
		if rc := streetRouteCore(streetCanon); rc != "" && rc == side {
			return true
		}
	}
	return false
}

// transcriptHasIntersectionIndicator reports whether the canonical transcript
// uses streetCanon (or its bare form) in an intersection-like phrase.
func transcriptHasIntersectionIndicator(canonTranscript, streetCanon string) bool {
	if canonTranscript == "" || streetCanon == "" {
		return false
	}
	cores := []string{streetCanon}
	if s := streetWithoutThoroughfare(streetCanon); s != "" && s != streetCanon {
		cores = append(cores, s)
	}
	if r := streetRouteCore(streetCanon); r != "" {
		cores = append(cores, r)
	}
	for _, c := range cores {
		// e.g. "NEAR HERNER COUNTY LINE", "HERNER COUNTY LINE AND", "AND HERNER COUNTY LINE", "AT HERNER COUNTY LINE"
		if strings.Contains(canonTranscript, "NEAR "+c) ||
			strings.Contains(canonTranscript, c+" AND ") ||
			strings.Contains(canonTranscript, " AND "+c) ||
			strings.Contains(canonTranscript, "AT "+c) ||
			strings.Contains(canonTranscript, "OFF "+c) ||
			strings.Contains(canonTranscript, "OFF OF "+c) ||
			strings.Contains(canonTranscript, "ON "+c+" AND") ||
			strings.Contains(canonTranscript, "AREA OF "+c) ||
			strings.Contains(canonTranscript, "INTERSECTION OF "+c) ||
			strings.Contains(canonTranscript, c+" INTERSECTION") {
			return true
		}
	}
	return false
}

// wordBoundedIndex returns the index of needle inside haystack, requiring
// space (or start/end) boundaries on both sides so partial-word matches
// ("SHANK" inside "SHANKSTOWN") are excluded. Returns -1 if not found.
func wordBoundedIndex(haystack, needle string) int {
	if needle == "" {
		return -1
	}
	idx := 0
	for {
		i := strings.Index(haystack[idx:], needle)
		if i < 0 {
			return -1
		}
		pos := idx + i
		before := pos == 0 || haystack[pos-1] == ' '
		end := pos + len(needle)
		after := end == len(haystack) || haystack[end] == ' '
		if before && after {
			return pos
		}
		idx = pos + 1
		if idx >= len(haystack) {
			return -1
		}
	}
}

// streetHit captures one known-street match in the transcript scan.
type streetHit struct {
	name string // canonical form to use in pin lookup
	idx  int    // position in canonical transcript
}

// consolidateStreetHits merges hits that refer to the same physical reference
// in the transcript. Two hits collapse when one canonical name is a prefix of
// the other or both reduce to the same thoroughfare-stripped core (e.g.
// "HERNER RD NW" and "HERNER COUNTY LINE RD NW" both share the core "HERNER").
// The longest canonical wins so the intersection-pin lookup uses the most
// specific name.
func consolidateStreetHits(hits []streetHit) []streetHit {
	if len(hits) <= 1 {
		return hits
	}
	sort.Slice(hits, func(i, j int) bool { return len(hits[i].name) > len(hits[j].name) })
	var out []streetHit
	for _, h := range hits {
		merged := false
		for k := range out {
			a := out[k].name
			b := h.name
			as := streetWithoutThoroughfare(a)
			bs := streetWithoutThoroughfare(b)
			if as == "" {
				as = a
			}
			if bs == "" {
				bs = b
			}
			if as == bs || strings.HasPrefix(as, bs+" ") || strings.HasPrefix(bs, as+" ") || as == b || bs == a {
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, h)
		}
	}
	return out
}

// containsWord reports whether haystack contains needle as a whole space-bounded word.
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	idx := 0
	for {
		i := strings.Index(haystack[idx:], needle)
		if i < 0 {
			return false
		}
		pos := idx + i
		before := pos == 0 || haystack[pos-1] == ' '
		end := pos + len(needle)
		after := end == len(haystack) || haystack[end] == ' '
		if before && after {
			return true
		}
		idx = pos + 1
		if idx >= len(haystack) {
			return false
		}
	}
}

// transcriptUsesWordAsStreet returns true when the given word appears in the
// transcript immediately followed by a thoroughfare-type token (DR/RD/AVE/...),
// indicating it's being used as a street name rather than a facility name.
func transcriptUsesWordAsStreet(tRaw, word string) bool {
	thoroughfareWords := []string{
		"DR", "DRIVE", "RD", "ROAD", "AVE", "AVENUE", "ST", "STREET",
		"LN", "LANE", "BLVD", "BOULEVARD", "CT", "COURT", "PL", "PLACE",
		"WAY", "TRL", "TRAIL", "HWY", "HIGHWAY", "PKWY", "PARKWAY",
		"CIR", "CIRCLE", "TER", "TERRACE",
	}
	// Canonicalize punctuation so "YOUNGSTOWN ROAD." (with a trailing period
	// from a dispatch sentence) still reads as a thoroughfare reference. Also
	// pad with spaces on both sides so HasSuffix / mid-string Contains lookups
	// are robust against punctuation and end-of-string. Previously this only
	// matched "WORD TYPE " (space terminated), so a transcript that said
	// "...SALT SPRINGS, YOUNGSTOWN ROAD. EMAIL..." failed to recognize that
	// YOUNGSTOWN was being used as a street name — causing the rare-word
	// fallback to fire and snap to an unrelated YOUNGSTOWN-bearing known
	// place (alert 6735 regression: 2725 SALT SPRINGS YOUNGSTOWN RD was
	// pulled to the ST RTE 46 AND SALT SPRINGS YOUNGSTOWN RD intersection).
	normalized := " " + tRaw + " "
	for _, r := range []string{".", ",", ";", ":", "!", "?", "-", "(", ")", "/"} {
		normalized = strings.ReplaceAll(normalized, r, " ")
	}
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}
	for _, t := range thoroughfareWords {
		needle := " " + word + " " + t + " "
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
}

// transcriptUsesWordAsGenericNoun reports when a rare-word facility token is
// ordinary English in the transcript ("find the information on Lawrence
// Crowe") rather than a facility name.
func transcriptUsesWordAsGenericNoun(transcript, word string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	w := strings.ToUpper(strings.TrimSpace(word))
	for _, p := range []string{
		" FIND THE " + w + " ON ",
		" FOUND THE " + w + " ON ",
		" GET THE " + w + " ON ",
		" HAVE THE " + w + " ON ",
		" THE " + w + " ON ",
		" ABLE TO FIND THE " + w + " ON ",
	} {
		if strings.Contains(u, p) {
			return true
		}
	}
	return isCommonFacilityWord(w)
}

// isIntersectionPlaceName reports whether a known-place display name describes
// an intersection rather than a single address or facility (e.g.
// "ST RTE 46 AND SALT SPRINGS YOUNGSTOWN RD"). Intersection pins are
// authoritative only for intersection queries; a house+street query
// ("2725 SALT SPRINGS YOUNGSTOWN RD") must not be snapped to one.
func isIntersectionPlaceName(name string) bool {
	u := " " + strings.ToUpper(strings.TrimSpace(name)) + " "
	return strings.Contains(u, " AND ") || strings.Contains(u, " & ")
}

// curatedHasHouseAndStreet reports whether curated.Address looks like a real
// house-number + street pair (e.g. "2725 SALT SPRINGS YOUNGSTOWN RD"). We use
// this to suppress fuzzy known-place fallbacks (rare-word and acronym) from
// snapping an otherwise-good address to an unrelated intersection or facility.
func curatedHasHouseAndStreet(curated *CuratedAlert) bool {
	if curated == nil {
		return false
	}
	h, st := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if h == "" {
		return false
	}
	return len(strings.Fields(strings.TrimSpace(st))) >= 1
}

// streetWithoutThoroughfare strips trailing thoroughfare-type words (RD/ST/...)
// AND trailing directionals (N/NW/...) from a canonical street string. Returns
// "" if removing the suffixes would leave nothing.
func streetWithoutThoroughfare(canon string) string {
	suffixes := map[string]bool{
		"RD": true, "ST": true, "AVE": true, "DR": true, "LN": true, "CT": true,
		"BLVD": true, "PL": true, "WAY": true, "TRL": true, "HWY": true,
		"PKWY": true, "CIR": true, "TERR": true,
		"N": true, "S": true, "E": true, "W": true,
		"NE": true, "NW": true, "SE": true, "SW": true,
	}
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canon)))
	for len(fields) > 0 {
		last := fields[len(fields)-1]
		if suffixes[last] {
			fields = fields[:len(fields)-1]
			continue
		}
		break
	}
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

// streetRouteCore returns the bare route number for a route-prefixed street
// (e.g. "US 422 NW" → "422", "SR 534" → "534"), or "" if the canonical form
// isn't a route street. Used only as a secondary match form because bare
// numbers risk false positives.
func streetRouteCore(canon string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canon)))
	if len(fields) < 2 {
		return ""
	}
	prefixes := map[string]bool{"US": true, "SR": true, "CR": true, "HWY": true, "I": true}
	if !prefixes[fields[0]] {
		return ""
	}
	for i := 1; i < len(fields); i++ {
		w := fields[i]
		if len(w) == 0 {
			continue
		}
		allDigit := true
		for _, r := range w {
			if r < '0' || r > '9' {
				allDigit = false
				break
			}
		}
		if allDigit {
			return w
		}
	}
	return ""
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// canonsIntersectionMatch reports whether two canonical "X AND Y" strings refer
// to the same intersection, allowing the two sides to be swapped.
func canonsIntersectionMatch(a, b string) bool {
	splitA := strings.SplitN(a, " AND ", 2)
	splitB := strings.SplitN(b, " AND ", 2)
	if len(splitA) != 2 || len(splitB) != 2 {
		return false
	}
	a1, a2 := strings.TrimSpace(splitA[0]), strings.TrimSpace(splitA[1])
	b1, b2 := strings.TrimSpace(splitB[0]), strings.TrimSpace(splitB[1])
	streetEq := func(x, y string) bool {
		if x == y {
			return true
		}
		return strings.HasPrefix(x, y) || strings.HasPrefix(y, x) ||
			strings.HasSuffix(x, y) || strings.HasSuffix(y, x)
	}
	return (streetEq(a1, b1) && streetEq(a2, b2)) || (streetEq(a1, b2) && streetEq(a2, b1))
}

// applyKnownPlaceFromTranscript scans the raw transcript for any known place
// in this group's pin set and applies its coords when a clear, multi-word match
// is found. Universal — uses only stored place names, no hard-coded data.
// Street-type variants (ROAD↔RD, STREET↔ST) are normalized on both sides.
func applyKnownPlaceFromTranscript(scope *ScopeData, curated *CuratedAlert, transcript string) (applied, fromCAD bool) {
	if scope == nil || curated == nil || strings.TrimSpace(transcript) == "" {
		return false, false
	}
	if curated.Lat != "" && curated.Lon != "" {
		return false, false
	}
	places := scope.ListKnownPlaces()
	if len(places) == 0 {
		return false, false
	}
	tRaw := strings.ToUpper(transcript)
	tCanon := canonicalStreetTokens(transcript)
	var best *KnownPlace
	bestLen := 0
	consider := func(p *KnownPlace, needleRaw, needleCanon string, haystack string) {
		if needleRaw == "" || needleCanon == "" {
			return
		}
		if !strings.Contains(haystack, needleCanon) {
			return
		}
		// Score by the LONGEST matched name to prefer specificity.
		if len(needleRaw) > bestLen {
			best = p
			bestLen = len(needleRaw)
		}
	}
	// Track ≥8-char distinctive words from facility names so we can fall back to
	// "single rare word in transcript" matching (e.g. CLEARVIEW in CLEARVIEW
	// LANTERN ESTATES ASSISTED LIVING). Only fire when exactly ONE place owns the
	// rare word to avoid ambiguity.
	rareWordOwners := map[string][]int{}
	for i := range places {
		p := places[i]
		name := strings.ToUpper(strings.TrimSpace(p.DisplayName))
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "THE ") {
			name = strings.TrimSpace(name[4:])
		}
		// Require ≥2 words and ≥10 chars to avoid spurious single-word matches.
		if len(strings.Fields(name)) < 2 || len(name) < 10 {
			continue
		}
		canon := canonicalStreetTokens(name)
		// Try matching by canonical address form (handles ROAD↔RD etc.).
		consider(&places[i], name, canon, tCanon)
		// Also try raw display-name (handles facility names with no street type).
		consider(&places[i], name, name, tRaw)
		// Drop trailing directional (N/S/E/W/NE/…) so "8100 SR 534 S" still
		// matches a transcript that says "8100 SR 534".
		if trimmed := trimTrailingDirectional(canon); trimmed != canon && trimmed != "" {
			consider(&places[i], name, trimmed, tCanon)
		}
		// Fuzzier: if the place has a house number, also try matching
		// "<house> <first significant street word>" so STT artifacts like
		// "754, AIRPORT" still snap to "754 AIRPORT RD".
		if h, st := splitHouseAndStreet(canon); h != "" && st != "" {
			sig := firstSignificantStreetWord(st)
			if sig != "" {
				consider(&places[i], name, h+" "+sig, tCanon)
			}
		}
		// Track distinctive long words from facility-style names (no house number)
		// so the rare-word fallback can identify the place.
		if h, _ := splitHouseAndStreet(name); h == "" {
			for _, w := range strings.Fields(name) {
				if len(w) >= 8 && !isCommonFacilityWord(w) {
					rareWordOwners[w] = append(rareWordOwners[w], i)
				}
			}
		}
	}
	if best == nil {
		// Distinctive-word fallback: rare 8+ char facility word that only one
		// place owns AND appears in transcript. Look at the TRANSCRIPT (not
		// curated.Address) for thoroughfare context — if the transcript says
		// "<word> <thoroughfare-type>" (DR/RD/AVE/...) the rare word is being
		// used as a street name, not a facility, so skip. Otherwise fire even
		// when curated has a house number (OpenAI may have fabricated a street
		// type like "CLEARVIEW DR" when the transcript actually said
		// "CLEARVIEW SUITES").
		curatedHouseStreet := curatedHasHouseAndStreet(curated)
		for word, owners := range rareWordOwners {
			if len(owners) != 1 {
				continue
			}
			if !strings.Contains(tRaw, " "+word+" ") && !strings.HasSuffix(tRaw, " "+word) && !strings.HasPrefix(tRaw, word+" ") {
				continue
			}
			if transcriptUsesWordAsStreet(tRaw, word) {
				continue
			}
			if transcriptUsesWordAsGenericNoun(tRaw, word) {
				continue
			}
			candidate := &places[owners[0]]
			// Intersection guard: if curated already has a clean house+street
			// address, refuse to snap to an intersection-typed known place. The
			// intersection's pin sits at the corner, not at the house, so the
			// match would always be off-route (see alert 6735: rare-word match
			// on "YOUNGSTOWN" pulled 2725 SALT SPRINGS YOUNGSTOWN RD to the
			// ST RTE 46 AND SALT SPRINGS YOUNGSTOWN RD intersection).
			if curatedHouseStreet && isIntersectionPlaceName(candidate.DisplayName) {
				continue
			}
			best = candidate
			bestLen = len(word)
			break
		}
	}
	if best == nil && acronymFallbackEligible(curated) {
		// First-word acronym fallback: if a pin's display name starts with a
		// 3–4 letter all-caps acronym (e.g. "OSP WARREN") and that acronym
		// appears as a standalone word in the transcript AND exactly one pin
		// in the group starts with that acronym, snap to it.  Skip common
		// directional / generic-English first words (WEST, NEW, OLD, …) so we
		// don't snap to "WEST PARK AVE AND SHARKEY RD" just because the
		// transcript said "BETWEEN BARCELONA AND WEST 3RD" — see tone alert
		// 7220 regression.
		acronymOwners := map[string][]int{}
		for i := range places {
			name := strings.ToUpper(strings.TrimSpace(places[i].DisplayName))
			fields := strings.Fields(name)
			if len(fields) < 2 {
				continue
			}
			first := fields[0]
			if len(first) < 3 || len(first) > 4 {
				continue
			}
			allLetters := true
			for _, r := range first {
				if r < 'A' || r > 'Z' {
					allLetters = false
					break
				}
			}
			if !allLetters {
				continue
			}
			if isAmbiguousAcronymWord(first) {
				continue
			}
			acronymOwners[first] = append(acronymOwners[first], i)
		}
		for acro, owners := range acronymOwners {
			if len(owners) != 1 {
				continue
			}
			pattern := " " + acro + " "
			if !strings.Contains(" "+tRaw+" ", pattern) {
				continue
			}
			p := &places[owners[0]]
			best = p
			bestLen = len(acro)
			break
		}
	}
	if best == nil {
		return false, false
	}
	if !PlaceMentionIsDispatchContext(transcript, best.DisplayName) {
		return false, false
	}
	if transcriptUsesBrandAsVehicle(transcript, best.DisplayName) ||
		knownPlaceConflictsWithCuratedAddress(curated, best) {
		return false, false
	}
	curated.Lat = fmt.Sprintf("%.6f", best.Lat)
	curated.Lon = fmt.Sprintf("%.6f", best.Lon)
	if strings.TrimSpace(curated.CommonName) == "" {
		curated.CommonName = best.DisplayName
	}
	log.Printf("[INFO] known place (transcript scan): %q → lat=%.6f lon=%.6f (group=%d source=%s)",
		best.DisplayName, best.Lat, best.Lon, best.GroupID, best.Source)
	// Same STT-correction logic as applyKnownPlaceCoords' common-name path:
	// transcript scan matched by facility name, so the curated street text
	// came from STT and may be mangled — overwrite with the CAD hint.
	applyKnownPlaceAddressHint(curated, best)
	return true, strings.EqualFold(strings.TrimSpace(best.Source), "active911")
}

func trimTrailingDirectional(canon string) string {
	dirs := map[string]bool{
		"N": true, "S": true, "E": true, "W": true,
		"NE": true, "NW": true, "SE": true, "SW": true,
	}
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canon)))
	for len(fields) > 0 && dirs[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}

var ambiguousAcronymWords = map[string]bool{
	"N": true, "S": true, "E": true, "W": true,
	"NE": true, "NW": true, "SE": true, "SW": true,
	"NORTH": true, "SOUTH": true, "EAST": true, "WEST": true,
	"NEW": true, "OLD": true, "BIG": true, "TOP": true, "MID": true,
	"END": true, "OFF": true, "ALL": true, "FOR": true, "ONE": true,
	"TWO": true, "MAY": true, "AND": true,
	"OAK": true, "ELM": true, "PIN": true, "RED": true, "BAY": true,
	"BAR": true, "HUB": true, "RUN": true, "BOX": true,
	"FIRE": true, "PARK": true, "LAKE": true, "HILL": true, "MILL": true,
	"FORT": true, "FORD": true, "WOOD": true, "MAIN": true, "HIGH": true,
	"LONG": true, "BEAR": true, "DEER": true, "PINE": true, "ROSE": true,
	"GLEN": true, "GATE": true, "FARM": true, "BARN": true, "POND": true,
	"VIEW": true, "STAR": true, "GRAY": true, "GREY": true, "GOLD": true,
	"SUN": true, "RIM": true,
	"LANE": true, // street suffix + store names (LANE BRYANT); never acronym-match from "VICTORIA LANE AND …"
}

func isAmbiguousAcronymWord(w string) bool {
	return ambiguousAcronymWords[strings.ToUpper(strings.TrimSpace(w))]
}

func acronymFallbackEligible(curated *CuratedAlert) bool {
	if curated == nil {
		return false
	}
	addr := strings.ToUpper(strings.TrimSpace(curated.Address))
	if addr == "" {
		return true
	}
	if strings.Contains(addr, "&") || strings.Contains(addr, " AND ") {
		return false
	}
	if strings.HasPrefix(addr, "AREA OF ") {
		return false
	}
	h, st := splitHouseAndStreet(addr)
	if h == "" {
		return true
	}
	stFields := strings.Fields(strings.TrimSpace(st))
	if len(stFields) == 0 {
		return true
	}
	return false
}

func firstSignificantStreetWord(street string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(street)))
	for i, f := range fields {
		if f == "SR" || f == "CR" || f == "US" || f == "HWY" {
			if i+1 < len(fields) {
				return f + " " + fields[i+1]
			}
			continue
		}
		if len(f) >= 4 {
			return f
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}
