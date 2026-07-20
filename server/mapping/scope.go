// Copyright (C) 2025 Thinline Dynamic Solutions
//
// scope.go — street corrections, scope lookups, and shared street-token helpers.

package mapping

import (
	"regexp"
	"strconv"
	"strings"
)

// numericOrdinalStreetRE matches numbered streets like W 7TH ST (not SR/OH routes).
var numericOrdinalStreetRE = regexp.MustCompile(`(?i)\b(?:[NSEW]\s+)?(\d{1,3})\s*(ST|TH|ND|RD)\b`)

func ohioStateRouteNumberInText(s string) (int, bool) {
	m := stateRouteRE.FindStringSubmatch(normalizeRouteTokens(strings.ToUpper(s)))
	if len(m) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func geocodeMatchedLooksLikeOrdinalStreet(routeNum int, formatted string) bool {
	for _, m := range numericOrdinalStreetRE.FindAllStringSubmatch(formatted, -1) {
		n, err := strconv.Atoi(m[1])
		if err == nil && n == routeNum {
			return true
		}
	}
	return false
}

// CanonicalStreetName normalizes a street name for matching: uppercased, route
// tokens normalized (STATE ROUTE → SR), street types and directionals collapsed
// to canonical abbreviations (BOULEVARD → BLVD, WEST → W), and unit suffixes
// dropped. This is the key the local geocoder uses on both sides of a lookup so
// OSM's full names ("West Clifton Boulevard") match dispatch abbreviations
// ("W Clifton Blvd").
func CanonicalStreetName(s string) string {
	return canonicalStreetTokens(s)
}

// ApplyCorrections applies all corrections in scope to text (uppercased).
func ApplyCorrections(scope *ScopeData, text string) string {
	if text == "" {
		return text
	}
	upper := strings.ToUpper(text)
	if scope == nil {
		return upper
	}
	for _, c := range scope.Corrections {
		upper = safeReplaceCorrection(upper, strings.ToUpper(c.BadName), strings.ToUpper(c.CorrectName))
	}
	return upper
}

// ApplyMistranscriptionCorrections applies only pure-replacement corrections
// (correct does not contain bad as substring) — safe for transcript-derived text.
func ApplyMistranscriptionCorrections(scope *ScopeData, text string) string {
	if text == "" {
		return text
	}
	upper := strings.ToUpper(text)
	if scope == nil {
		return upper
	}
	for _, c := range scope.Corrections {
		badU := strings.ToUpper(c.BadName)
		correctU := strings.ToUpper(c.CorrectName)
		if strings.Contains(correctU, badU) {
			continue
		}
		upper = safeReplaceCorrection(upper, badU, correctU)
	}
	return upper
}

// bestCollapsedCoreStreetMatch finds a gazetteer street when STT glues or drops
// spaces in a compound name (KINGSGRAVE → KING GRAVES ROAD, YOULL → YULE).
func bestCollapsedCoreStreetMatch(queryStreet string, candidates []string) (string, bool) {
	qCanon := CanonicalStreetName(queryStreet)
	qCore, qType := StreetCoreTypeKey(qCanon)
	if qCore == "" {
		return "", false
	}
	qDir, _, _ := splitStreetParts(qCanon)
	qColl := stripStreetSpaces(qCore)
	// exactCoreMatches collects every candidate whose stripped core matches
	// the query exactly (deferred rather than returned immediately) so that
	// when a bare, typeless, directionless query like "MAHONING" matches many
	// gazetteer homonyms ("MAHONING AVENUE", "MAHONING COURT NORTHWEST", ...),
	// we can pick the most plausible one (shortest / no unspoken directional)
	// instead of whichever happened to be first in the caller's slice.
	var exactCoreMatches []string
	best := ""
	bestDist := 1 << 30
	for _, c := range candidates {
		// Canonicalize before splitting the core: gazetteer names carry long
		// suffixes ("VIENNA AVENUE"), and without abbreviation the type token is
		// not recognized, gluing it into the core ("VIENNAAVENUE") so a wrong
		// short-suffix street (BRIANNA WAY) out-scores the true match.
		cCanon := CanonicalStreetName(c)
		cCore, cType := StreetCoreTypeKey(cCanon)
		if cCore == "" {
			continue
		}
		if qType != "" && cType != "" && qType != cType {
			continue
		}
		// The directional is part of the street identity, not phonetic noise:
		// "WEST 52ND" and "EAST 52ND" are different streets and must never
		// collapse to the same "52ND" core match.
		if qDir != "" {
			if cDir, _, _ := splitStreetParts(cCanon); cDir != "" && !streetDirectionalsCompatible(qDir, cDir) {
				continue
			}
		}
		if qCore == cCore {
			exactCoreMatches = append(exactCoreMatches, c)
			continue
		}
		cColl := stripStreetSpaces(cCore)
		d := levenshtein(qColl, cColl)
		if d > 2 || d >= bestDist {
			continue
		}
		// STT glued "KINGSGRAVE" must not beat a distant "KINGSTON" when a
		// collapsed KING-GRAVES import is within two edits.
		if strings.Contains(qColl, "GRAV") || strings.Contains(qColl, "GRAVE") {
			if !strings.Contains(cColl, "GRAV") {
				continue
			}
		}
		if strings.Contains(cColl, "STON") &&
			!strings.Contains(cColl, "GRAV") &&
			(strings.Contains(qColl, "GRAV") || strings.Contains(qColl, "GRAVE")) {
			continue
		}
		bestDist = d
		best = c
	}
	if len(exactCoreMatches) > 0 {
		// A bare, typeless query ("MAHONING") that collapses onto homonyms of
		// genuinely different thoroughfare types ("MAHONING AVENUE" vs
		// "MAHONING ROAD" — real, distinct streets, not directional variants
		// of the same one) has no spoken signal to disambiguate by type.
		// pickPlainestCoreMatch's word-count/length tiebreak is only sound
		// for variants of the *same* type (plain vs directional-suffixed);
		// applied across different types it silently fabricates a specific
		// street the dispatcher never named. Defer to the caller instead of
		// guessing wrong.
		if qType == "" && distinctCoreTypeCount(exactCoreMatches) > 1 {
			return "", false
		}
		return pickPlainestCoreMatch(exactCoreMatches, qDir), true
	}
	if best != "" && bestDist <= 2 {
		return best, true
	}
	return "", false
}

// distinctCoreTypeCount counts how many distinct non-empty thoroughfare types
// (AVE, RD, CT, ...) appear among a set of gazetteer names that all share the
// same stripped core.
func distinctCoreTypeCount(matches []string) int {
	types := map[string]bool{}
	for _, m := range matches {
		_, t := StreetCoreTypeKey(CanonicalStreetName(m))
		if t != "" {
			types[t] = true
		}
	}
	return len(types)
}

// pickPlainestCoreMatch chooses among several gazetteer streets that all
// share the same stripped core name (homonyms differing only by thoroughfare
// type and/or trailing directional). Without any spoken signal preferring one
// over another, the plainest form — no trailing directional the query never
// mentioned — is the least speculative choice; ties break on fewer words,
// then shortest string, then first-seen for determinism.
func pickPlainestCoreMatch(matches []string, qDir string) string {
	if len(matches) == 1 {
		return matches[0]
	}
	best := matches[0]
	bestCanon := CanonicalStreetName(best)
	bestHasDir := hasTrailingDirectionalInCanonical(bestCanon)
	bestWords := len(strings.Fields(bestCanon))
	for _, c := range matches[1:] {
		cCanon := CanonicalStreetName(c)
		cHasDir := hasTrailingDirectionalInCanonical(cCanon)
		cWords := len(strings.Fields(cCanon))
		better := false
		switch {
		case qDir == "" && cHasDir != bestHasDir:
			// Prefer the candidate without an unspoken trailing directional.
			better = !cHasDir && bestHasDir
		case cWords != bestWords:
			better = cWords < bestWords
		case len(cCanon) != len(bestCanon):
			better = len(cCanon) < len(bestCanon)
		}
		if better {
			best = c
			bestCanon = cCanon
			bestHasDir = cHasDir
			bestWords = cWords
		}
	}
	return best
}

var suffixPhoneticAlternates = map[string][]string{
	"CORD": {"COURT", "CT"},
	"CORE": {"COURT", "CT"},
}

// correctSuffixPhoneticStreet rewrites STT mis-heard suffixes when the stem
// matches a known import ("ANGEL CORD" → "ANGEL COURT").
func correctSuffixPhoneticStreet(street string, knownStreets []string) (string, bool) {
	street = strings.ToUpper(strings.TrimSpace(street))
	name, sfx := streetNameAndSuffix(street)
	if name == "" {
		return "", false
	}
	if sfx == "" {
		fields := strings.Fields(street)
		if len(fields) < 2 {
			return "", false
		}
		last := fields[len(fields)-1]
		if _, ok := suffixPhoneticAlternates[last]; !ok {
			return "", false
		}
		name = strings.Join(fields[:len(fields)-1], " ")
		sfx = last
	}
	alts, ok := suffixPhoneticAlternates[sfx]
	if !ok {
		return "", false
	}
	nameU := strings.ToUpper(name)
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		kName, kSfx := streetNameAndSuffix(ku)
		if kName != nameU {
			continue
		}
		for _, alt := range alts {
			if kSfx == alt || strings.HasSuffix(ku, " "+alt) {
				return ku, true
			}
		}
	}
	return "", false
}

// ApplyFuzzyGazetteerStreetCorrection rewrites a dispatch street to the nearest
// imported thoroughfare when STT mis-heard the name (YULE → YOULL STREET).
func ApplyFuzzyGazetteerStreetCorrection(addr string, scope *ScopeData) string {
	if scope == nil || len(scope.KnownStreets) == 0 {
		return addr
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return addr
	}
	if qCanon := CanonicalStreetName(street); qCanon != "" {
		for _, ks := range scope.KnownStreets {
			if CanonicalStreetName(ks) == qCanon {
				return addr
			}
		}
	}
	cands := make([]string, 0, len(scope.KnownStreets))
	seen := map[string]bool{}
	for _, ks := range scope.KnownStreets {
		if cc := CanonicalStreetName(ks); cc != "" && !seen[cc] {
			seen[cc] = true
			cands = append(cands, cc)
		}
	}
	matched, ok := bestCollapsedCoreStreetMatch(street, cands)
	if !ok {
		matched, ok = BestFuzzyStreetMatch(CanonicalStreetName(street), cands)
	}
	if !ok {
		if corrected, ok2 := correctSuffixPhoneticStreet(street, scope.KnownStreets); ok2 {
			return house + " " + corrected
		}
		return addr
	}
	if matched == CanonicalStreetName(street) {
		if corrected, ok2 := correctSuffixPhoneticStreet(street, scope.KnownStreets); ok2 {
			return house + " " + corrected
		}
		return addr
	}
	for _, ks := range scope.KnownStreets {
		if CanonicalStreetName(ks) == matched {
			pick := strings.ToUpper(strings.TrimSpace(ks))
			if fuzzyGazetteerSwapAddsUnspokenLeadingDirection(street, pick) {
				return addr
			}
			return house + " " + pick
		}
	}
	return addr
}

// fuzzyGazetteerSwapAddsUnspokenLeadingDirection blocks fuzzy rewrites that
// prepend a directional to an already-correct import (WARD AVENUE SE → NORTH WARD AVENUE).
func fuzzyGazetteerSwapAddsUnspokenLeadingDirection(fromStreet, toStreet string) bool {
	fromName, _ := streetNameAndSuffix(fromStreet)
	toName, _ := streetNameAndSuffix(toStreet)
	pickLead := streetLeadingQualifier(toName)
	if pickLead == "" || streetLeadingQualifier(fromName) != "" {
		return false
	}
	fromStem := homonymStreetStemTokens(fromStreet)
	toStem := homonymStreetStemTokens(toStreet)
	if len(fromStem) == 0 || len(toStem) <= len(fromStem) {
		return false
	}
	for i, t := range fromStem {
		if i >= len(toStem) || toStem[i] != t {
			return false
		}
	}
	return true
}

// LookupKnownPlace finds a facility pin by common_name / address key within scope.
func (scope *ScopeData) LookupKnownPlace(displayName string) *KnownPlace {
	if scope == nil {
		return nil
	}
	key := NormalizePlaceKey(displayName)
	if key == "" {
		return nil
	}
	for i := range scope.KnownPlaces {
		if scope.KnownPlaces[i].PlaceKey == key {
			return &scope.KnownPlaces[i]
		}
	}
	var best *KnownPlace
	for i := range scope.KnownPlaces {
		p := &scope.KnownPlaces[i]
		if placeKeysMatch(key, p.PlaceKey) {
			if best == nil || len(p.PlaceKey) > len(best.PlaceKey) {
				best = p
			}
		}
	}
	return best
}

// ListKnownPlaces returns every pin in scope.
func (scope *ScopeData) ListKnownPlaces() []KnownPlace {
	if scope == nil {
		return nil
	}
	return scope.KnownPlaces
}

func placeKeysMatch(queryKey, storedKey string) bool {
	if queryKey == storedKey {
		return true
	}
	if len(queryKey) < 4 || len(storedKey) < 4 {
		return false
	}
	qHouse, qRest := splitHouseAndStreet(queryKey)
	sHouse, sRest := splitHouseAndStreet(storedKey)
	if qHouse != "" || sHouse != "" {
		if qHouse != sHouse {
			return false
		}
		return addressStreetEquivalent(qRest, sRest)
	}
	return strings.Contains(queryKey, storedKey) || strings.Contains(storedKey, queryKey)
}

func addressStreetEquivalent(a, b string) bool {
	an := canonicalStreetTokens(a)
	bn := canonicalStreetTokens(b)
	if an == "" || bn == "" {
		return false
	}
	if an == bn {
		return true
	}
	_, aSuffix := streetNameAndSuffix(a)
	_, bSuffix := streetNameAndSuffix(b)
	if strings.HasPrefix(an, bn) || strings.HasPrefix(bn, an) {
		if aSuffix != "" && bSuffix != "" && !streetSuffixesCompatible(aSuffix, bSuffix) {
			return false
		}
		return true
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func splitHouseAndStreet(addr string) (houseNum, streetOnly string) {
	parts := strings.SplitN(strings.TrimSpace(addr), " ", 2)
	if len(parts) == 2 && isAllDigits(parts[0]) {
		return parts[0], parts[1]
	}
	return "", addr
}

func canonicalStreetTokens(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	u = strings.ReplaceAll(u, ".", "")
	u = strings.ReplaceAll(u, ",", " ")
	u = normalizeRouteTokens(u)
	u = strings.ReplaceAll(u, "-", " ")
	u = collapseCompoundDirectionals(u)
	fields := strings.Fields(u)
	streetType := map[string]string{
		"ROAD": "RD", "RD": "RD",
		"STREET": "ST", "ST": "ST",
		"AVENUE": "AVE", "AVE": "AVE",
		"DRIVE": "DR", "DR": "DR",
		"LANE": "LN", "LN": "LN",
		"COURT": "CT", "CT": "CT",
		"BOULEVARD": "BLVD", "BLVD": "BLVD",
		"PLACE": "PL", "PL": "PL",
		"WAY":   "WAY",
		"TRAIL": "TRL", "TRL": "TRL",
		"HIGHWAY": "HWY", "HWY": "HWY",
		"PARKWAY": "PKWY", "PKWY": "PKWY",
		"CIRCLE": "CIR", "CIR": "CIR",
		"TERRACE": "TERR", "TERR": "TERR",
	}
	dir := map[string]string{
		"NORTH": "N", "N": "N",
		"SOUTH": "S", "S": "S",
		"EAST": "E", "E": "E",
		"WEST": "W", "W": "W",
		"NORTHEAST": "NE", "NE": "NE",
		"NORTHWEST": "NW", "NW": "NW",
		"SOUTHEAST": "SE", "SE": "SE",
		"SOUTHWEST": "SW", "SW": "SW",
	}
	unitSkip := map[string]bool{"UNIT": true, "APT": true, "ROOM": true, "SUITE": true, "STE": true, "RM": true, "APARTMENT": true, "#": true}
	out := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		w := fields[i]
		if unitSkip[w] {
			i++
			continue
		}
		if canon, ok := streetType[w]; ok {
			out = append(out, canon)
			continue
		}
		if canon, ok := dir[w]; ok {
			out = append(out, canon)
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

// compoundDirectionalRE folds a two-word quadrant into its single-word spelling
// so every form collapses to one canonical token downstream: "NORTH EAST",
// "NORTH E", and "N EAST" → "NORTHEAST" (→ "NE"). At least one side must be the
// fully spelled word — a bare "N E" is left alone because "E"/"W" can be a real
// grid street name ("NORTH E STREET").
var compoundDirectionalRE = regexp.MustCompile(`(?i)\b(NORTH|SOUTH)\s+(EAST|WEST|E|W)\b|\b(N|S)\s+(EAST|WEST)\b`)

// collapseCompoundDirectionals rewrites spaced quadrant directionals to the
// joined spelled form ("NORTH EAST" → "NORTHEAST"), which the directional table
// then maps to the abbreviation. This makes NE, NORTHEAST, and NORTH EAST all
// equivalent.
func collapseCompoundDirectionals(s string) string {
	return compoundDirectionalRE.ReplaceAllStringFunc(strings.ToUpper(s), func(m string) string {
		f := strings.Fields(m)
		if len(f) != 2 {
			return m
		}
		switch string(f[0][0]) + string(f[1][0]) {
		case "NE":
			return "NORTHEAST"
		case "NW":
			return "NORTHWEST"
		case "SE":
			return "SOUTHEAST"
		case "SW":
			return "SOUTHWEST"
		}
		return m
	})
}

func normalizeRouteTokens(s string) string {
	s = strings.ToUpper(s)
	// Periods in "U.S." must go before US ROUTE collapsing — otherwise
	// phrase capture that stops at "." never sees a US route token.
	s = strings.ReplaceAll(s, "U.S.", "US")
	s = strings.ReplaceAll(s, "U. S.", "US")
	repl := []struct{ old, new string }{
		{"STATE ROUTE", "SR"},
		{"STATE RTE", "SR"},
		{"ST ROUTE", "SR"},
		{"ST RTE", "SR"},
		{"US ROUTE", "US"},
		{"US RTE", "US"},
		{"COUNTY ROUTE", "CR"},
		{"COUNTY ROAD", "CR"},
		{"CO RTE", "CR"},
		{"CO RD", "CR"},
		{"C-ROUTE", "SR"},
		{"C ROUTE", "SR"},
		{"OHIO-", "SR "},
		{"OH-", "SR "},
		{"ROUTE ", "SR "},
		{"HIGHWAY ", "HWY "},
	}
	for _, r := range repl {
		s = strings.ReplaceAll(s, r.old, r.new)
	}
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

var genericStreetTypeWord = map[string]bool{
	"AVENUE": true, "AVE": true, "STREET": true, "ST": true, "ROAD": true, "RD": true,
	"DRIVE": true, "DR": true, "LANE": true, "LN": true, "BOULEVARD": true, "BLVD": true,
	"COURT": true, "CT": true, "CIRCLE": true, "CIR": true, "HIGHWAY": true, "HWY": true,
	"WAY": true, "PLACE": true, "PL": true, "TERRACE": true, "TRAIL": true, "RUN": true,
	"SE": true, "SW": true, "NE": true, "NW": true, "N": true, "S": true, "E": true, "W": true,
}

func significantStreetTokens(streetOnly string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToUpper(streetOnly)) {
		w = strings.TrimRight(w, ",")
		if parts := hyphenatedCompoundStreetRE.FindStringSubmatch(w); len(parts) == 3 {
			for _, p := range []string{parts[1], parts[2]} {
				if len(p) >= 4 && !genericStreetTypeWord[p] {
					out = append(out, p)
				}
			}
			continue
		}
		if len(w) < 4 || genericStreetTypeWord[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// GeocodeResultPlausible reports whether a forward-geocoder's matched address
// plausibly corresponds to the dispatch query (same gate Google picks use).
func GeocodeResultPlausible(origAddress, matchedAddress string) bool {
	return geocodeFormattedStreetPlausible(origAddress, matchedAddress)
}

func geocodeFormattedStreetPlausible(origAddress, formattedStreetUpper string) bool {
	house, st := splitHouseAndStreet(strings.TrimSpace(origAddress))
	if st == "" {
		return true
	}
	if idx := strings.Index(st, ","); idx > 0 {
		st = strings.TrimSpace(st[:idx])
	}
	rawUpper := strings.ToUpper(strings.TrimSpace(formattedStreetUpper))
	fu := normalizeRouteTokens(stripLeadingHouseNumberComma(normalizeNominatimDisplayName(rawUpper)))
	stNorm := normalizeRouteTokens(strings.ToUpper(st))
	// Census often spells PARKWAY while our query canon is PKWY
	// ("S PKWY DR" vs "S PARKWAY DR"). Search both the display form and the
	// canonical street so type aliases still count as evidenced.
	fuHaystack := fu
	if fuStreet := formattedStreetCoreForPlausible(fu); fuStreet != "" {
		if canon := CanonicalStreetName(fuStreet); canon != "" && !strings.Contains(fu, canon) {
			fuHaystack = fu + " " + canon
		}
	}
	// Intersection queries must evidence BOTH sides in the matched label. A
	// road-only arterial hit ("Youngstown Warren Road, Girard…") for
	// "YOUNGSTOWN WARREN RD and NORTH RD" is not the cross with North Road.
	if a, b := splitIntersectionQuery(stNorm); a != "" && b != "" {
		if house != "" {
			return false
		}
		if geocodeFormattedLooksCityOnly(fu) {
			return false
		}
		return intersectionSidePlausibleInFormatted(a, fu) && intersectionSidePlausibleInFormatted(b, fu)
	}
	if qRoute, ok := ohioStateRouteNumberInText(stNorm); ok {
		if geocodeMatchedLooksLikeOrdinalStreet(qRoute, fu) {
			return false
		}
		if mRoute, mOk := ohioStateRouteNumberInText(fu); mOk {
			return qRoute == mRoute
		}
		return false
	}
	// Business/POI query ("Sheetz", "Dollar General"): Nominatim prefixes the
	// display_name with the shop/amenity label before the street address.
	if house == "" && stNorm != "" && strings.HasPrefix(rawUpper, stNorm+",") {
		if !geocodeFormattedLooksCityOnly(fu) && geocodeFormattedHasStreetThoroughfare(rawUpper) {
			return true
		}
	}
	// A street was requested; a city/place-only result ("Cleveland, OH, USA")
	// is never a street match regardless of query shape.
	if geocodeFormattedLooksCityOnly(fu) {
		return false
	}
	// A house-level query must get a house-level match. A formatted result with
	// no leading house number ("Lakeview Boulevard, Painesville, OH, USA") is a
	// street centroid, not the dispatched address.
	if house != "" && !formattedHasLeadingHouseNumber(fu) {
		return false
	}
	_, qSuffix := streetNameAndSuffix(stNorm)
	mSuffix := formattedStreetSuffix(fu)
	if qSuffix != "" && mSuffix != "" && !streetSuffixesCompatible(qSuffix, mSuffix) {
		return false
	}
	toks := significantStreetTokens(stNorm)
	if len(toks) == 0 {
		for _, w := range strings.Fields(stNorm) {
			if len(w) >= 2 && strings.Contains(fu, w) {
				return true
			}
		}
		return true
	}
	if len(toks) == 1 {
		if strings.Contains(fuHaystack, toks[0]) {
			return true
		}
		return streetTokenHomophoneInFormatted(toks[0], fuHaystack)
	}
	// Multi-word streets: require every significant token to appear, STT-match,
	// or share a long prefix with a formatted token. Longest-token-only used
	// to reject gateway-confirmed corrections like HOAGLAND BLACKSTOKE →
	// HOAGLAND BLACKSTUB (exact shared stem + STT-garbled second word).
	// At least one token must be exact/STT so two weak prefixes alone can't pin.
	hits, anchored := 0, 0
	for _, tok := range toks {
		if strings.Contains(fuHaystack, tok) || streetTokenHomophoneInFormatted(tok, fuHaystack) {
			hits++
			anchored++
			continue
		}
		// "SOUTH PARKWAY DRIVE" vs Census "S PARKWAY DR" — spelled compass
		// directionals must match the geocoder's single-letter preDirection.
		if streetTokenCompassMatchesFormatted(tok, fuHaystack) {
			hits++
			anchored++
			continue
		}
		if streetTokenPrefixNearMissInFormatted(tok, fuHaystack) {
			hits++
		}
	}
	return hits == len(toks) && anchored >= 1
}

// formattedStreetCoreForPlausible returns the house+street (or street-only)
// head of a geocoder label before city/state commas.
func formattedStreetCoreForPlausible(fu string) string {
	fu = strings.TrimSpace(fu)
	if fu == "" {
		return ""
	}
	if idx := strings.Index(fu, ","); idx > 0 {
		fu = strings.TrimSpace(fu[:idx])
	}
	if _, st := splitHouseAndStreet(fu); st != "" {
		return st
	}
	return fu
}

// streetTokenCompassMatchesFormatted reports NORTH/SOUTH/EAST/WEST matching a
// single-letter preDirection in the geocoder label (and the reverse).
func streetTokenCompassMatchesFormatted(tok, fu string) bool {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	fu = strings.ToUpper(strings.TrimSpace(fu))
	if tok == "" || fu == "" {
		return false
	}
	padded := " " + fu + " "
	switch tok {
	case "NORTH", "SOUTH", "EAST", "WEST":
		abbr := map[string]string{"NORTH": "N", "SOUTH": "S", "EAST": "E", "WEST": "W"}[tok]
		return strings.Contains(padded, " "+abbr+" ") || strings.HasPrefix(fu, abbr+" ")
	case "N", "S", "E", "W":
		return strings.Contains(fu, expandCompassAbbrevToken(tok))
	}
	return false
}

// intersectionSidePlausibleInFormatted reports whether one side of an
// "A and B" query is evidenced in the geocoder's matched label. Lone
// directional street names abbreviated by CanonicalStreetName ("N RD" from
// "NORTH ROAD") expand back to NORTH/SOUTH/EAST/WEST for the check.
func intersectionSidePlausibleInFormatted(side, fu string) bool {
	side = strings.ToUpper(strings.TrimSpace(side))
	if idx := strings.Index(side, ","); idx > 0 {
		side = strings.TrimSpace(side[:idx])
	}
	if side == "" {
		return false
	}
	toks := significantStreetTokens(side)
	toks = append(toks, directionalStreetNameTokens(side)...)
	seen := map[string]bool{}
	var uniq []string
	for _, tok := range toks {
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		uniq = append(uniq, tok)
	}
	if len(uniq) == 0 {
		for _, w := range strings.Fields(side) {
			if len(w) >= 2 && !genericStreetTypeWord[w] {
				uniq = append(uniq, w)
			}
		}
	}
	if len(uniq) == 0 {
		return false
	}
	for _, tok := range uniq {
		if strings.Contains(fu, tok) || streetTokenHomophoneInFormatted(tok, fu) {
			continue
		}
		if exp := expandCompassAbbrevToken(tok); exp != tok && strings.Contains(fu, exp) {
			continue
		}
		return false
	}
	return true
}

// directionalStreetNameTokens recovers NORTH/SOUTH/EAST/WEST when a street
// side was abbreviated to a lone compass letter before its suffix ("N RD").
func directionalStreetNameTokens(side string) []string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(side)))
	if len(fields) == 0 {
		return nil
	}
	var out []string
	for i, f := range fields {
		exp := expandCompassAbbrevToken(f)
		if exp == f {
			continue
		}
		// Lone letter before a thoroughfare type, or the only name token.
		if i+1 < len(fields) && genericStreetTypeWord[fields[i+1]] {
			out = append(out, exp)
			continue
		}
		if len(fields) == 1 {
			out = append(out, exp)
		}
	}
	return out
}

func expandCompassAbbrevToken(tok string) string {
	switch strings.ToUpper(strings.TrimSpace(tok)) {
	case "N":
		return "NORTH"
	case "S":
		return "SOUTH"
	case "E":
		return "EAST"
	case "W":
		return "WEST"
	default:
		return strings.ToUpper(strings.TrimSpace(tok))
	}
}

// leadingHouseNumberCommaRE matches Nominatim's display_name convention of
// separating the house number from the street with a comma ("716, OLIVE
// STREET SOUTHEAST, NILES, ...") rather than a space ("716 OLIVE STREET,
// NILES, ..." — what Census returns). Every downstream helper here
// (geocodeFormattedLooksCityOnly, formattedHasLeadingHouseNumber,
// formattedStreetSuffix) truncates at the first comma expecting it to be the
// street/city boundary; against Nominatim's format that comma is actually
// the house-number/street boundary, so it would strip the match down to a
// bare number and every one of those checks would spuriously fail. Folding
// the leading "NUMBER," into "NUMBER " up front makes both formats look
// identical to everything downstream.
var leadingHouseNumberCommaRE = regexp.MustCompile(`^(\d+[A-Z]?),\s*`)

// nominatimPOIHousePrefixRE matches Nominatim POI display names like
// "Sheetz, 2721, Salt Springs Road, …" where the business label precedes
// the comma-separated house number and street.
var nominatimPOIHousePrefixRE = regexp.MustCompile(`^[^,\d][^,]*,\s*(\d+[A-Z]?),\s*(.+)$`)

// normalizeNominatimDisplayName folds POI-prefixed Nominatim display_name
// strings into the same shape as bare address results so house-number and
// street-suffix checks work on both formats.
func normalizeNominatimDisplayName(formattedUpper string) string {
	line := strings.TrimSpace(formattedUpper)
	if m := nominatimPOIHousePrefixRE.FindStringSubmatch(line); len(m) == 3 {
		return m[1] + ", " + m[2]
	}
	parts := strings.SplitN(line, ",", 2)
	if len(parts) != 2 {
		return line
	}
	first := strings.TrimSpace(parts[0])
	rest := strings.TrimSpace(parts[1])
	if first == "" || rest == "" || isAllDigits(first) {
		return line
	}
	if h, st := splitHouseAndStreet(first); h != "" || st == "" {
		return line
	}
	if !geocodeFormattedHasStreetThoroughfare(rest) {
		return line
	}
	return rest
}

// geocodeFormattedHasStreetThoroughfare reports whether any early segment of
// a geocoder display_name contains a street-type suffix (RD, ST, AVE, …).
func geocodeFormattedHasStreetThoroughfare(formattedUpper string) bool {
	line := strings.ToUpper(strings.TrimSpace(formattedUpper))
	parts := strings.Split(line, ",")
	limit := len(parts)
	if limit > 4 {
		limit = 4
	}
	for i := 0; i < limit; i++ {
		seg := normalizeRouteTokens(strings.TrimSpace(parts[i]))
		if _, suf := streetNameAndSuffix(seg); suf != "" {
			return true
		}
	}
	return false
}

func stripLeadingHouseNumberComma(formattedUpper string) string {
	return leadingHouseNumberCommaRE.ReplaceAllString(strings.TrimSpace(formattedUpper), "$1 ")
}

// geocodeFormattedLooksCityOnly reports when a forward-geocoder returned only a
// municipality (e.g. "Painesville, OH, USA") with no street thoroughfare.
func geocodeFormattedLooksCityOnly(formattedUpper string) bool {
	line := strings.ToUpper(strings.TrimSpace(formattedUpper))
	if line == "" {
		return true
	}
	if idx := strings.Index(line, ","); idx > 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if line == "" {
		return true
	}
	if _, st := splitHouseAndStreet(line); st == "" {
		return true
	}
	return formattedStreetSuffix(line) == ""
}

// formattedHasLeadingHouseNumber reports whether a geocoder's matched address
// starts with a house number ("453 LAKEVIEW BLVD, …" yes; "LAKEVIEW BLVD, …" no).
func formattedHasLeadingHouseNumber(formattedUpper string) bool {
	line := strings.ToUpper(strings.TrimSpace(formattedUpper))
	if idx := strings.Index(line, ","); idx > 0 {
		line = strings.TrimSpace(line[:idx])
	}
	house, _ := splitHouseAndStreet(line)
	return house != ""
}

// formattedStreetSuffix extracts the thoroughfare type from a geocoder's matched
// address line ("420 LINCOLN AVE, NILES, OH" → AVE).
func formattedStreetSuffix(formattedUpper string) string {
	line := strings.ToUpper(strings.TrimSpace(formattedUpper))
	if idx := strings.Index(line, ","); idx > 0 {
		line = strings.TrimSpace(line[:idx])
	}
	_, st := splitHouseAndStreet(line)
	if st == "" {
		return ""
	}
	_, suf := streetNameAndSuffix(st)
	return suf
}

func streetTokenHomophoneInFormatted(queryTok, formattedUpper string) bool {
	queryTok = strings.ToUpper(strings.TrimSpace(queryTok))
	if queryTok == "" {
		return false
	}
	// Longer tokens (WELKER vs WALKER, VIANNA vs VIENNA) used to require
	// gazetteer confirmation before being trusted on edit distance alone —
	// that local street gazetteer no longer exists. StreetTokensSTTMatch /
	// ScoreStreetSTTCoreMatch already scale edit-distance tolerance to token
	// length and add a consonant-skeleton check for longer distances, so it's
	// safe to rely on the score alone — confirmed real miss: call 2758's
	// "VIANNA AVENUE" (STT vowel-swap of "VIENNA AVENUE") was rejected here
	// purely because "VIANNA" is 6 characters, even though nominatim-gateway
	// had already geographically confirmed the correction (fuzzy.go).
	for _, tok := range strings.Fields(formattedUpper) {
		tok = strings.Trim(tok, ",.")
		if StreetTokensSTTMatch(queryTok, tok) {
			return true
		}
		if alt := sttConfusableTokenAlt(queryTok); alt != "" && (alt == tok || StreetTokensSTTMatch(alt, tok)) {
			return true
		}
		if alt := sttConfusableTokenAlt(tok); alt != "" && (alt == queryTok || StreetTokensSTTMatch(queryTok, alt)) {
			return true
		}
		// Silent/leading K before N: STT "NOLWOOD" for KNOLLWOOD, "NIGHT" for KNIGHT.
		if strings.HasPrefix(tok, "KN") && strings.HasPrefix(queryTok, "N") &&
			StreetTokensSTTMatch(queryTok, tok[1:]) {
			return true
		}
		if strings.HasPrefix(queryTok, "KN") && strings.HasPrefix(tok, "N") &&
			StreetTokensSTTMatch(queryTok[1:], tok) {
			return true
		}
	}
	return false
}

// streetTokenPrefixNearMissInFormatted accepts a long query token that shares
// a long leading run with some formatted street token but isn't close enough
// for StreetTokensSTTMatch (live: BLACKSTOKE vs BLACKSTUB — edit distance 3,
// consonant skeletons diverge on the final letter). Only used for multi-word
// streets that already share another exact token, so a lone single-token
// query cannot pin on a weak prefix alone.
func streetTokenPrefixNearMissInFormatted(queryTok, formattedUpper string) bool {
	queryTok = strings.ToUpper(strings.TrimSpace(queryTok))
	if len(queryTok) < 6 {
		return false
	}
	for _, raw := range strings.Fields(formattedUpper) {
		tok := strings.Trim(raw, ",.")
		if len(tok) < 6 || localStreetSuffixes[tok] {
			continue
		}
		if absInt(len(queryTok)-len(tok)) > 3 || queryTok[0] != tok[0] {
			continue
		}
		n := 0
		limit := len(queryTok)
		if len(tok) < limit {
			limit = len(tok)
		}
		for n < limit && queryTok[n] == tok[n] {
			n++
		}
		if n >= 6 {
			return true
		}
	}
	return false
}

// safeReplaceCorrection replaces every occurrence of bad with correct while
// guarding against two classes of duplicate-token corruption:
//
//  1. "Contained" overlap — when bad is a substring of correct, e.g.
//     {bad:"ST RTE 45", correct:"ST RTE 45 NW"}, applying naively to a string
//     that already contains "ST RTE 45 NW" would double the directional →
//     "ST RTE 45 NW NW". We skip any bad occurrence that already sits inside
//     an existing correct match.
//
//  2. "Tail" overlap — when correct ends with one or more street-type or
//     directional tokens (RD, ST, AVE, DR, NW, N, …) and the text immediately
//     following a candidate bad already contains those same tail tokens, the
//     substitution would duplicate them. Examples we hit in production:
//     - {bad:"HERNER", correct:"HERNER RD NW"} on
//     "HERNER COUNTY LINE RD NW" → "HERNER RD NW COUNTY LINE RD NW"
//     - {bad:"SOUTH SPRINGS", correct:"SALT SPRINGS YOUNGSTOWN RD"} on
//     "SOUTH SPRINGS YOUNGSTOWN RD" → "SALT SPRINGS YOUNGSTOWN RD YOUNGSTOWN RD"
//
// Substitutions are also constrained to whole-word matches so a correction
// like {bad:"HERNER", correct:"HERNER RD NW"} cannot mid-word-replace inside
// "GUTHERNER ST".
func safeReplaceCorrection(text, bad, correct string) string {
	if bad == "" || bad == correct {
		return text
	}

	// Pre-compute regions where correct already occupies the text (contained-
	// overlap guard). When correct does not contain bad this is unnecessary.
	var covered []bool
	if strings.Contains(correct, bad) {
		covered = make([]bool, len(text))
		for start := 0; start < len(text); {
			i := strings.Index(text[start:], correct)
			if i < 0 {
				break
			}
			i += start
			end := i + len(correct)
			if end > len(text) {
				end = len(text)
			}
			for j := i; j < end; j++ {
				covered[j] = true
			}
			start = end
		}
	}

	// Pre-compute the trailing street-type/directional tokens of correct.
	// These are the tokens that would duplicate if the text immediately after
	// bad already contains them.
	correctWords := strings.Fields(correct)
	var tailIndicators []string
	for i := len(correctWords) - 1; i >= 0; i-- {
		w := correctWords[i]
		if !isStreetTypeOrDirectional(w) {
			break
		}
		tailIndicators = append([]string{w}, tailIndicators...)
	}

	var out strings.Builder
	out.Grow(len(text) + len(correct))
	for i := 0; i < len(text); {
		matched := i+len(bad) <= len(text) && text[i:i+len(bad)] == bad
		if !matched {
			out.WriteByte(text[i])
			i++
			continue
		}
		// Whole-word boundary check on both sides of the matched span.
		if !leftIsCorrectionWordBoundary(text, i) || !rightIsCorrectionWordBoundary(text, i+len(bad)) {
			out.WriteByte(text[i])
			i++
			continue
		}
		// Contained-overlap guard.
		if covered != nil && covered[i] {
			out.WriteByte(text[i])
			i++
			continue
		}
		// Tail-overlap guard (street-type/directional duplication after).
		if len(tailIndicators) > 0 && tailWouldDuplicate(text[i+len(bad):], tailIndicators) {
			out.WriteByte(text[i])
			i++
			continue
		}
		// Symmetric word-overlap guard (catches cases where bad sits inside
		// correct as a substring, not just at the end). Substituting bad with
		// correct can duplicate words on either side:
		//
		//   text:    "ST RTE 46 & SALT SPRINGS YOUNGSTOWN"
		//   bad:     "SPRINGS"          (at position after "SALT ")
		//   correct: "SALT SPRINGS YOUNGSTOWN RD"
		//   naive:   "ST RTE 46 & SALT SALT SPRINGS YOUNGSTOWN RD YOUNGSTOWN"
		//
		// Prefix overlap: text just before bad ends with the words of
		// correct that precede bad. Suffix overlap: text just after bad
		// begins with the words of correct that follow bad. Either is a
		// sign the surrounding text already encodes those tokens and the
		// substitution would only duplicate them.
		if pre, suf, ok := splitCorrectAroundBad(correct, bad); ok {
			if pre != "" && textBeforeEndsWithWords(text, i, pre) {
				out.WriteByte(text[i])
				i++
				continue
			}
			if suf != "" && textAfterStartsWithFirstWord(text[i+len(bad):], suf) {
				out.WriteByte(text[i])
				i++
				continue
			}
		}
		out.WriteString(correct)
		i += len(bad)
	}
	return out.String()
}

// splitCorrectAroundBad returns the words of correct that come before and
// after the first whole-word occurrence of bad inside correct, plus whether
// such a split was found. Empty strings (and ok=false) when bad is not a
// substring of correct on a word boundary.
func splitCorrectAroundBad(correct, bad string) (pre, suf string, ok bool) {
	cu := strings.ToUpper(correct)
	bu := strings.ToUpper(bad)
	idx := strings.Index(cu, bu)
	if idx < 0 {
		return "", "", false
	}
	if !leftIsCorrectionWordBoundary(cu, idx) || !rightIsCorrectionWordBoundary(cu, idx+len(bu)) {
		return "", "", false
	}
	return strings.TrimSpace(cu[:idx]), strings.TrimSpace(cu[idx+len(bu):]), true
}

// textBeforeEndsWithWords reports whether the contiguous block of text
// ending at position i in text matches words (token by token, canonicalized).
// Trailing whitespace before i is ignored.
func textBeforeEndsWithWords(text string, i int, words string) bool {
	target := strings.Fields(words)
	if len(target) == 0 {
		return false
	}
	prefix := strings.TrimRight(text[:i], " \t\r\n")
	preWords := strings.Fields(prefix)
	if len(preWords) < len(target) {
		return false
	}
	for j := range target {
		if canonicalIndicator(strings.ToUpper(preWords[len(preWords)-len(target)+j])) !=
			canonicalIndicator(strings.ToUpper(target[j])) {
			return false
		}
	}
	return true
}

// textAfterStartsWithFirstWord reports whether after begins with the first
// word of words (canonicalized). Matching just the first word is enough: any
// 1-token suffix overlap is sign of a duplication-producing substitution.
func textAfterStartsWithFirstWord(after, words string) bool {
	target := strings.Fields(words)
	if len(target) == 0 {
		return false
	}
	afterWords := strings.Fields(after)
	if len(afterWords) == 0 {
		return false
	}
	return canonicalIndicator(strings.ToUpper(afterWords[0])) ==
		canonicalIndicator(strings.ToUpper(target[0]))
}

// streetTypeWords are the road-classifier tokens we recognize at the tail of
// a correction's "correct" form. Recognizing these lets us detect duplicate
// tail tokens that would result from a naive substitution.
var streetTypeWords = map[string]bool{
	"RD": true, "ROAD": true,
	"ST": true, "STREET": true,
	"AVE": true, "AVENUE": true,
	"DR": true, "DRIVE": true,
	"LN": true, "LANE": true,
	"BLVD": true, "BOULEVARD": true,
	"WAY": true,
	"PL":  true, "PLACE": true,
	"CT": true, "COURT": true,
	"PKWY": true, "PARKWAY": true,
	"HWY": true, "HIGHWAY": true,
	"TER": true, "TERRACE": true, "TERR": true,
	"TRL": true, "TRAIL": true,
	"CIR": true, "CIRCLE": true,
	"SR": true, // state route
	"US": true, // US route
}

var directionalWordsCorrection = map[string]bool{
	"N": true, "S": true, "E": true, "W": true,
	"NE": true, "NW": true, "SE": true, "SW": true,
	"NORTH": true, "SOUTH": true, "EAST": true, "WEST": true,
	"NORTHEAST": true, "NORTHWEST": true, "SOUTHEAST": true, "SOUTHWEST": true,
}

func isStreetTypeOrDirectional(w string) bool {
	u := strings.ToUpper(strings.TrimSpace(w))
	if streetTypeWords[u] || directionalWordsCorrection[u] {
		return true
	}
	_, ok := usStateAbbr[u]
	return ok
}

// leftIsCorrectionWordBoundary reports whether position i in text is the
// start of a word (i.e. the character immediately before i is not a word
// character, or i is at the start of the string).
func leftIsCorrectionWordBoundary(text string, i int) bool {
	if i <= 0 {
		return true
	}
	return !isCorrectionWordChar(text[i-1])
}

// rightIsCorrectionWordBoundary reports whether position i in text is the
// end of a word (i.e. the character at i is not a word character, or i is at
// the end of the string).
func rightIsCorrectionWordBoundary(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	return !isCorrectionWordChar(text[i])
}

func isCorrectionWordChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// tailWouldDuplicate reports whether any of the indicator tokens (correct's
// trailing type/directional words) appear inside the next len(indicators)+6
// words of after. Tokens are compared via canonicalIndicator so spelled-out
// directionals collapse to their abbreviation ("NORTH" ≡ "N"), and street
// types collapse to canonical form ("ROAD" ≡ "RD"). This is what lets us
// detect that substituting bad="534" → correct="ST RTE 534 NW" against
// "6006 534 NORTH" would produce the malformed "6006 ST RTE 534 NW NORTH".
func tailWouldDuplicate(after string, indicators []string) bool {
	if len(indicators) == 0 || after == "" {
		return false
	}
	canonIndicators := make(map[string]bool, len(indicators))
	for _, ind := range indicators {
		canonIndicators[canonicalIndicator(ind)] = true
	}
	const window = 6
	max := len(indicators) + window
	count := 0
	for _, w := range strings.Fields(after) {
		if count >= max {
			break
		}
		count++
		if canonIndicators[canonicalIndicator(w)] {
			return true
		}
	}
	return false
}

// canonicalIndicator collapses street-type and directional word variants to
// a single canonical form so tail-overlap comparison ignores synonyms.
// ROAD ↔ RD, AVENUE ↔ AVE, NORTH ↔ N, SOUTHWEST ↔ SW, etc.
func canonicalIndicator(w string) string {
	u := strings.ToUpper(strings.TrimSpace(w))
	switch u {
	case "ROAD":
		return "RD"
	case "STREET":
		return "ST"
	case "AVENUE":
		return "AVE"
	case "DRIVE":
		return "DR"
	case "LANE":
		return "LN"
	case "BOULEVARD":
		return "BLVD"
	case "PLACE":
		return "PL"
	case "COURT":
		return "CT"
	case "PARKWAY":
		return "PKWY"
	case "HIGHWAY":
		return "HWY"
	case "TERRACE":
		return "TER"
	case "TRAIL":
		return "TRL"
	case "CIRCLE":
		return "CIR"
	case "NORTH":
		return "N"
	case "SOUTH":
		return "S"
	case "EAST":
		return "E"
	case "WEST":
		return "W"
	case "NORTHEAST":
		return "NE"
	case "NORTHWEST":
		return "NW"
	case "SOUTHEAST":
		return "SE"
	case "SOUTHWEST":
		return "SW"
	}
	return u
}
