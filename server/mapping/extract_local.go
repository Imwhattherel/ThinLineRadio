// Copyright (C) 2025 Thinline Dynamic Solutions
//
// extract_local.go — rule/gazetteer-based transcript extraction with no LLM.
// Produces the same CuratedAlert shape as the OpenAI extractor by reusing the
// existing transcript address/intersection extractors and adding regex parsers
// for cross streets, nature, common name, and apartment/unit. Used by the local
// (zero-cost) mapping engine.

package mapping

import (
	"regexp"
	"strings"
)

var (
	localBetweenRE = regexp.MustCompile(`(?i)\bBETWEEN\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bAT\b|\bTIME\b|\bFOR\b|\bWITH\b|\bNEAR\b|\bON\b|$)`)
	// Comma before AND is common STT ("CROSS OF 88, AND BRADLEY BROWNLEE").
	localCrossOfRE = regexp.MustCompile(`(?i)\bCROSS(?:ES)?\s+OF\s+(.{1,40}?)(?:,?\s+AND\s+(.{2,40}?))?(?:[.,;]|\bAT\b|\bTIME\b|\bFOR\b|\bWITH\b|\bSORT\b|$)`)
	// "CROSSROADS OF CUMBERLAND, CIRCLE, YOLANDA, ORKNEY, AND PUMBROOK" —
	// comma lists (with a glued bare thoroughfare type after a stem) are
	// common; CROSSROADS is not matched by localCrossOfRE (CROSS + ROADS).
	localCrossroadsListRE = regexp.MustCompile(`(?i)\bCROSS(?:ROADS?|ES)?\s+OF\s+(.+?)(?:\.\s+|\bFOR\b|\bTHE\s+CO\b|\bTIME\b|\bNEGATIVE\b|\bTHEY\b|$)`)
	// "REPEATING YOUR CROSS STREETS OF COVINGTON STREET AND FOSTER STREET".
	localCrossStreetsOfRE = regexp.MustCompile(`(?i)\b(?:REPEATING\s+)?(?:YOUR\s+)?CROSS\s+STREETS?\s+OF\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bFOR\b|\bTHIS\b|\bAT\b|\bTIME\b|$)`)
	// "THE CROSS STREETS ARE GOING TO BE SOUTHERN AND ERIE STREET" / "CROSS
	// STREET IS BRADLEY AND FOSTER" — the other common dispatcher phrasing,
	// stating the cross streets rather than asking "of" them.
	localCrossStreetsAreRE = regexp.MustCompile(`(?i)\bCROSS\s+STREETS?\s+(?:ARE|IS)\s+(?:GOING\s+TO\s+BE\s+)?(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bFOR\b|\bTHIS\b|\bAT\b|\bTIME\b|$)`)
	// STT garbles "cross of" as one token ("CROSSUP BIANCO AND ETHEL",
	// "CROSSUP, OUR HAVEN, AND MAPLEWOOD").
	localCrossUpRE   = regexp.MustCompile(`(?i)\bCROSSUP[,]?\s+(.{1,40}?)(?:,?\s+AND\s+(.{2,40}?))?(?:[.,;]|\bAT\b|\bTIME\b|\bFOR\b|\bWITH\b|\b\d+-YEAR|\b\d+-YEAR|$)`)
	localCrossesUpRE = regexp.MustCompile(`(?i)\bCROSSES\s+UP\s+(.{2,40}?)(?:[.,;]|\bAND\b|\bTIME\b|\bFOR\b|\bNORTON\b|\bCORNY\b|$)`)
	// "(YOUR) CROSSES ARE LAMONT AND LEMOYNE" — dispatcher stating both cross
	// streets. Must run before localCrossesBareRE (below), which only
	// captures one side and would otherwise truncate this at "AND".
	localCrossesAreRE = regexp.MustCompile(`(?i)\bCROSSES\s+(?:ARE|IS|GOING\s+TO\s+BE)\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bTO\b|\bAT\b|\bTIME\b|\bFOR\b|$)`)
	// "CROSS WITH ALDRIDGE AND DEHOFF" — dispatcher naming both cross streets
	// with "WITH" instead of "ARE"/"OF".
	localCrossWithRE = regexp.MustCompile(`(?i)\bCROSS(?:ES)?\s+WITH\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bTO\b|\bAT\b|\bTIME\b|\bFOR\b|$)`)
	// Bare "CROSSES NORTH DAVIS IN THE DEAD END" (not CROSSES UP / CROSSES OF).
	localCrossesBareRE       = regexp.MustCompile(`(?i)\bCROSSES\s+(.{2,40}?)(?:\s+IN\s+THE\s+|\s+IN\s+|\s+AT\b|[.,;]|\bAND\b|\bFOR\b|\bTIME\b|$)`)
	localStateRouteRE        = regexp.MustCompile(`(?i)\b(?:STATE ROUTE|ST ROUTE|SR)\s+(\d{1,3})\b`)
	localBareNumberedRouteRE = regexp.MustCompile(`(?i)\b(?:STATE\s+ROUTE|ST\s+ROUTE|SR|ROUTE|RT|RTE)\s+(\d{1,3})\b`)
	localCornerRE            = regexp.MustCompile(`(?i)\bCORNER\s+OF\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bAT\b|\bTIME\b|$)`)
	localCornerHouseRE       = regexp.MustCompile(`(?i)\bCORNER\s+HOUSE\s+OF\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\bLANES\b|\bTIME\b|\bFOR\b|$)`)
	localOffRE               = regexp.MustCompile(`(?i)\bOFF(?:\s+OF)?\s+(.{2,40}?)(?:[.,;]|\bAT\b|\bTIME\b|$)`)
	localAcrossRE            = regexp.MustCompile(`(?i)\bACROSS\s+(?:FROM\s+)?(.{2,40}?)(?:\s+IN\s+THE\s+|\s+IN\s+|\s+AT\b|[.,;]|\bTIME\b|$)`)
	// "ACROSS OF WILLOW, AND NOW IT'S CORTLAND" ← STT for "cross of Willow and Niles Cortland".
	localAcrossOfAndRE = regexp.MustCompile(`(?i)\bACROSS\s+OF\s+(.{2,30}?)(?:,\s*|\s+)AND\s+(.{2,40}?)(?:[.,;]|\s+TIME(?:\s+OUT)?|\s+TIMEOUT|\s+\d{1,2}-|\s+\d+-YEAR|$)`)
	// "ACROSS THE U.S. ROUTE 422 AND STATE ROUTE 534" — second-tones STT for CROSSES OF.
	localAcrossTheAndRE = regexp.MustCompile(`(?i)\bACROSS\s+THE\s+(.{2,40}?)\s+AND\s+(.{2,40}?)(?:[.,;]|\s+TIME(?:\s+OUT)?|\s+TIMEOUT|\s+\d{1,2}-|\s+\d+-YEAR|$)`)
	nowItsCrossRE      = regexp.MustCompile(`(?i)^NOW\s+IT'?S\s+(.+)$`)
	localAptRE         = regexp.MustCompile(`(?i)\b(APT|APARTMENT|UNIT|SUITE|STE|ROOM|RM|LOT)\.?\s*#?\s*([A-Z0-9]{1,5})\b`)
	// Single-token facility before house ("AT FAIRHAVEN, 420 …") — kept for
	// known-place / "AT <NAME>, <house>" paths.
	localAtFacilityRE = regexp.MustCompile(`(?i)\bAT (?:THE )?([A-Z][A-Z'\-]{3,})\b`)
	// Multi-word facility after AT / AT THE ("AT THE CLEVELAND VA MEDICAL CENTER").
	// Stops before narrative verbs / agencies so we don't swallow the rest of the
	// dispatch. Digits are required only after AT THE (house addresses use AT 123).
	localAtTheFacilityRE = regexp.MustCompile(`(?i)\bAT\s+(?:THE\s+)?([A-Z][A-Z0-9'\-]{2,}(?:\s+[A-Z][A-Z0-9'\-]{1,}){0,6})(?:[.,;]|\s+HAD\b|\s+HAS\b|\s+THEY\b|\s+FOR\b|\s+WITH\b|\s+TIME\b|\s+VAPD\b|\s+PD\b|\s+FD\b|$)`)
	// Facility then city then house ("NEW DAY RECOVERY, RIVER FALLS, 150 CHARLES COURT").
	localFacilityCityRE = regexp.MustCompile(`(?i),\s*([A-Z][A-Z\s\-&]{4,40}?),\s*([A-Z][A-Z\s\-]{2,20}?),\s*\d{1,6}`)
	// Facility then house ("BELLARIA PIZZA, 882-882, WEST LIBERTY STREET").
	localFacilityBeforeHouseRE = regexp.MustCompile(`(?i),\s*([A-Z][A-Z\s\-&]{4,40}?),\s*(?:\d{1,6}[\s,\-]|\d{1,3}-\d{1,3}\s)`)
	// Facility after routing VIA chain ("HUBBARD VIA WEST PARK MANOR VIA APARTMENT 317").
	localFacilityViaAptRE = regexp.MustCompile(`(?i)\bVIA\s+([A-Z][A-Z\s\-&]{4,40}?)\s+VIA\s+(?:APARTMENT|APT|UNIT)`)
	// Named mobile-home park ("IN THE FOWLER MOBILE HOME PARK").
	localInTheMobileHomeParkRE = regexp.MustCompile(`(?i)\bIN THE ([A-Z][A-Z\s\-&]{3,30}?\s+MOBILE HOME PARK)\b`)
	// Facility/building qualifier after a street ("IN THE MEDIA PLAZA FOR…").
	localInTheFacilityRE = regexp.MustCompile(`(?i)\bIN THE ([A-Z][A-Z0-9\s\-&']{2,40}?)(?:\s+FOR\b|\s*,|\.)`)
	// Street after OFF OF phrasing ("8976 … OFF OF HOWLAND WILSON SOUTHEAST").
	localOffOfStreetRE      = regexp.MustCompile(`(?i)\bOFF\s+OF\s+(.{4,45}?)(?:\s+FOR\b|\s+FOR\s+A\b|\s*,|\s+\d{1,2}-YEAR|\s+TIME\b|$)`)
	localMangledPrefixAndRE = regexp.MustCompile(`(?i)\b(\d{3,5})-\d{1,4}-\d{1,3},\s*([A-Z][A-Z\s\-]{2,25}?)\s+AND\s+([A-Z][A-Z\s\-]{2,25}?)(?:\s+FOR\b|\s+TIME\b|,|\.)`)
	// Digits are required so numbered routes ("PARKMAN AND ROUTE 5") match.
	// Sides still must start with a letter so bare unit pairs ("22 AND 45") stay out.
	localBareStreetAndRE = regexp.MustCompile(`(?i)(?:^|[,.\s])([A-Z][A-Z0-9\s\-]{2,25}?)\s+AND\s+([A-Z][A-Z0-9\s\-]{2,25}?)(?:\s+FOR\b|\s+TIME\b|\s+\d{1,2}-YEAR|,|\.)`)
	localBareStreetCommaRE = regexp.MustCompile(`(?i)(?:^|[,.\s])([A-Z][A-Z0-9\s\-]{2,25}?)\s*,\s*([A-Z][A-Z0-9\s\-]{2,25}?)(?:\s*,|\s*\.|$)`)
	// STT often says "IN" for "AND" at intersections ("EAST BROAD IN NORTH STATE").
	// Require a trailing incident/time cue so "IN THE AREA OF" chatter stays out.
	localBareStreetInRE = regexp.MustCompile(`(?i)(?:^|[,.\s])([A-Z][A-Z0-9\s\-]{2,25}?)\s+IN\s+([A-Z][A-Z0-9\s\-]{2,25}?)(?:\s+FOR\b|\s+TIME\b|\s+\d{1,2}-YEAR|,|\.)`)
)

// as a street name even when it isn't in the known-streets gazetteer.
var localStreetSuffixes = map[string]bool{
	"RD": true, "ROAD": true, "ST": true, "STREET": true, "AVE": true, "AVENUE": true,
	"DR": true, "DRIVE": true, "LN": true, "LANE": true, "CT": true, "COURT": true,
	"BLVD": true, "BOULEVARD": true, "PL": true, "PLACE": true, "PT": true, "POINT": true, "WAY": true,
	"CIR": true, "CIRCLE": true, "TRL": true, "TRAIL": true, "TERR": true, "TER": true,
	"HWY": true, "HIGHWAY": true, "PKWY": true, "PARKWAY": true,
	"RUN": true, // "REVERE RUN" — OSM/TIGER thoroughfare type, not just a verb
}

// localCrossNoiseTrailers are connector / filler words trimmed from the tail of
// a captured cross-street phrase.
var localCrossNoiseTrailers = map[string]bool{
	"THE": true, "A": true, "AN": true, "AT": true, "ON": true, "IN": true, "OF": true,
	"NEAR": true, "BY": true, "OFF": true, "AND": true, "OR": true, "TO": true,
	"FOR": true, "WITH": true, "IS": true, "ARE": true,
	"RIGHT": true, "LEFT": true,
}

// ExtractLocal parses a (pre-cleaned) transcript into a CuratedAlert without any
// LLM call. It never invents data: address fields are populated only from the
// transcript's own extractors and the system's known-streets / known-places
// gazetteer. Returns the primary alert and any extra incidents (currently
// always single — local extraction defaults to one incident).
func ExtractLocal(transcript, toneSetLabel string, scope *ScopeData, natureCodes []string) (*CuratedAlert, []*CuratedAlert) {
	if scope == nil {
		scope = &ScopeData{}
	}
	upper := strings.ToUpper(transcript)

	curated := &CuratedAlert{
		UnitLocation:        strings.ToUpper(strings.TrimSpace(toneSetLabel)),
		NatureDesc:          localNature(upper, natureCodes),
		Notes:               upper,
		CorrectedTranscript: upper,
	}

	// Address: prefer mangled house + intersection, then house on known street,
	// then explicit dispatch house numbers, then bare "X AND Y" (medical status
	// like CONSCIOUS AND BREATHING is lower priority than 7525 DAWSON).
	if addr := extractMangledPrefixIntersection(transcript, scope); addr != "" {
		curated.Address = addr
	} else if route := extractBareNumberedRouteWithBetween(upper, scope); route != "" {
		curated.Address = route
	} else if known := extractKnownStreetAddressesFromTranscript(transcript, scope.KnownStreets); len(known) > 0 {
		addr := pickBestKnownStreetAddress(known, transcript)
		addr = appendSpokenTrailingDirectional(addr, transcript)
		curated.Address = snapExtractedAddressToKnownStreet(addr, scope.KnownStreets, transcript)
	} else if off := extractOffOfStreetAddress(upper, scope); off != "" {
		curated.Address = off
	} else if addr := extractBareAndIntersectionAddress(transcript, scope); addr != "" {
		curated.Address = addr
	} else if addr := extractBareInIntersectionAddress(transcript, scope); addr != "" {
		curated.Address = addr
	} else if addr := extractCommaSeparatedIntersectionAddress(transcript, scope); addr != "" {
		curated.Address = addr
	} else if addrs := extractDispatchAddressesFromTranscript(transcript); len(addrs) > 0 {
		addr := preferDispatchAddress(addrs, transcript, scope.KnownStreets)
		addr = appendSpokenTrailingDirectional(addr, transcript)
		addr = canonicalizeMisheardStreetSuffix(addr, scope.KnownStreets)
		curated.Address = snapExtractedAddressToKnownStreet(addr, scope.KnownStreets, transcript)
	} else if hwy := extractHighwayCrossAddress(upper, scope); hwy != "" {
		curated.Address = hwy
	} else if ix := extractDispatchIntersectionsFromTranscript(transcript, scope.KnownStreets); len(ix) > 0 {
		a, b := splitIntersectionQuery(ix[0])
		a = cleanIntersectionSide(a)
		b = cleanIntersectionSide(b)
		// Accept an intersection only when the (fully captured) first side is a
		// plausible street. The shared extractor truncates the second side, so
		// we validate the first. This rejects false positives like
		// "VEHICLE 8629 AND I ARE CLEAR", where a unit number becomes "SR 8629"
		// (an implausible 4-digit route) paired with a non-street fragment.
		if a != "" && b != "" &&
			!IntersectionSideIsNonStreet(a) && !IntersectionSideIsNonStreet(b) &&
			isPlausibleLocalStreet(a, scope) && isPlausibleLocalStreet(b, scope) &&
			AddressAlignsWithTranscript(a+" & "+b, transcript, scope) &&
			!AddressIsUnitStandDownOrCancel(a+" & "+b, transcript) {
			curated.Address = a + " & " + b
		}
	}

	// Cross streets from explicit dispatcher phrasings.
	cs1, cs2 := localCrossStreets(upper, scope)
	curated.CrossStreet1 = cs1
	curated.CrossStreet2 = cs2

	// Apartment / unit identifier.
	if m := localAptRE.FindStringSubmatch(upper); len(m) == 3 {
		curated.AptUnit = strings.ToUpper(m[1]) + " " + strings.ToUpper(m[2])
	}

	// Common name from the known-places gazetteer (named facilities only).
	curated.CommonName = localCommonName(upper, scope)
	if curated.CommonName == "" {
		curated.CommonName = localFacilityCommonName(upper, scope)
	}
	if curated.CommonName == "" {
		curated.CommonName = localFacilityBeforeCity(upper)
	}
	if curated.CommonName == "" {
		curated.CommonName = localFacilityBeforeHouse(upper)
	}
	if curated.CommonName == "" {
		curated.CommonName = localFacilityViaApartment(upper)
	}
	if curated.CommonName == "" {
		curated.CommonName = localInTheMobileHomePark(upper)
	}

	if strings.TrimSpace(curated.Address) != "" {
		curated.Address = AlignAddressStreetFromScopedTranscript(curated.Address, transcript, scope)
	} else if cs1 != "" && cs2 != "" &&
		!IntersectionSideIsNonStreet(cs1) && !IntersectionSideIsNonStreet(cs2) &&
		isPlausibleLocalStreet(cs1, scope) && isPlausibleLocalStreet(cs2, scope) &&
		AddressAlignsWithTranscript(cs1+" & "+cs2, transcript, scope) &&
		!AddressIsUnitStandDownOrCancel(cs1+" & "+cs2, transcript) {
		// Cross-street-only dispatches ("CROSSES ARE LAMONT AND LEMOYNE") give
		// the incident location purely as an intersection with no house
		// address. Promote the two resolved cross streets into the primary
		// Address field so downstream guards/geocoding treat this exactly
		// like any other extracted intersection ("A & B") — otherwise a
		// fully-resolved intersection is silently dropped because
		// curated.Address was never populated.
		curated.Address = cs1 + " & " + cs2
	}

	return curated, nil
}

// localRouteKeywords are the leading tokens that mark a numbered route. A side
// like "SR 88" or "I 90" is a valid street only when paired with a sane route
// number (see isPlausibleLocalStreet).
var localRouteKeywords = map[string]bool{
	"SR": true, "US": true, "CR": true, "HWY": true,
	"I": true, "INTERSTATE": true, "ROUTE": true, "RT": true, "RTE": true,
}

// isShortRouteNumber reports whether s is a 1–3 digit route number. Real state /
// US / interstate routes are almost never 4+ digits, so a value like "8629"
// (a unit/vehicle number) is rejected.
func isShortRouteNumber(s string) bool {
	if len(s) < 1 || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isPlausibleLocalStreet reports whether one side of an extracted intersection
// looks like a real street: a known street, a named thoroughfare (ends in a
// street-type suffix), or a numbered route with a sane route number. Bare
// numbers and stray word fragments are rejected so the local engine never
// geocodes noise like "8629 & I ARE".
func isPlausibleLocalStreet(side string, scope *ScopeData) bool {
	u := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(side)))
	fields := strings.Fields(u)
	if len(fields) == 0 {
		return false
	}
	if scope != nil {
		for _, ks := range scope.KnownStreets {
			ksU := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(ks)))
			if ksU == "" {
				continue
			}
			if u == ksU {
				return true
			}
			// Multi-word known streets may match as a full embedded phrase only
			// (never a single-word substring like MEDLEY inside "AROUND MEDLEY …").
			if len(strings.Fields(ksU)) >= 2 && strings.Contains(" "+u+" ", " "+ksU+" ") {
				return true
			}
		}
	}
	// Numbered route: keyword + sane route number (e.g. "SR 88", "I 90").
	if len(fields) >= 2 && localRouteKeywords[fields[0]] && isShortRouteNumber(fields[1]) {
		return true
	}
	// Named street: full name with thoroughfare suffix (e.g. "DREXEL AVE").
	// A lone given-name token ("GEORGE") is not a street even when longer
	// known streets share the prefix.
	if len(fields) >= 2 && localStreetSuffixes[fields[len(fields)-1]] {
		return true
	}
	// "LAKE ROAD WEST" / "MAIN STREET NORTH" — trailing cardinal after type.
	if len(fields) >= 3 && localStreetSuffixes[fields[len(fields)-2]] &&
		isStreetTrailingCardinal(fields[len(fields)-1]) {
		return true
	}
	if len(fields) == 1 {
		if scope != nil {
			for _, ks := range scope.KnownStreets {
				if normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(ks))) == u {
					return true
				}
			}
			// Bare stem shared by multiple suffixed gazetteer entries ("SOUTHERN"
			// -> "SOUTHERN AVENUE"/"SOUTHERN BOULEVARD"/...). The ambiguity is
			// resolved (or left bare) upstream by snapCrossStreet/pickCrossStreetPrefixMatch
			// — here we only need to confirm the bare name is a real local
			// thoroughfare, not noise, before it's used to build an intersection.
			if suffixlessKnownGazetteerStem(u, scope) {
				return true
			}
		}
		return false
	}
	// Trumbull-style dispatch drops thoroughfare suffixes ("7525 WARREN SHARON").
	// When imported streets exist, suffixless phrases must resolve to that
	// gazetteer — arbitrary word pairs are not locations.
	if scope != nil && len(scope.KnownStreets) > 0 {
		return suffixlessPhraseInGazetteer(u, scope)
	}
	return suffixlessStreetNamePlausible(u)
}

// suffixlessPhraseInGazetteer reports whether a suffixless spoken street phrase
// can be tied to an imported thoroughfare for this agency (prefix snap, stem
// match, or directional alias). Returns false when imports exist but the phrase
// does not resolve — universal "no real street, no location" grounding.
func suffixlessPhraseInGazetteer(phrase string, scope *ScopeData) bool {
	if scope == nil || len(scope.KnownStreets) == 0 {
		return false
	}
	phrase = normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(phrase)))
	if phrase == "" {
		return false
	}
	for _, ks := range scope.KnownStreets {
		ksU := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(ks)))
		if ksU == "" {
			continue
		}
		if phrase == ksU {
			return true
		}
		if strings.HasPrefix(ksU, phrase+" ") {
			return true
		}
		if len(strings.Fields(ksU)) >= 2 && strings.Contains(" "+phrase+" ", " "+ksU+" ") {
			return true
		}
	}
	if stem := snapDirectionalStemCrossStreet(phrase, scope); stem != "" {
		return true
	}
	// Leading quadrant before a suffixless stem ("EAST GLENDOLA" → GLENDOLA AVENUE).
	if fields := strings.Fields(phrase); len(fields) == 2 {
		if streetDirTokens[canonicalStreetTokens(fields[0])] {
			stem := fields[1]
			if suffixlessKnownGazetteerStem(stem, scope) {
				return true
			}
			for _, ks := range scope.KnownStreets {
				ksU := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(ks)))
				if strings.HasPrefix(ksU, stem+" ") {
					return true
				}
			}
		}
	}
	// Do not fuzzy-snap for anchoring: tail tokens like "THROUGH" can match
	// unrelated imported cores ("THRUSH") and approve non-location chatter.
	if len(strings.Fields(phrase)) == 1 {
		return suffixlessKnownGazetteerStem(phrase, scope)
	}
	return false
}

// cleanIntersectionSide strips a leading incident-type phrase ("WIRES DOWN",
// "MVA", …) and leading filler words from one side of an extracted intersection
// so the remaining tokens are just the street name.
func cleanIntersectionSide(side string) string {
	u := strings.ToUpper(strings.TrimSpace(side))
	for changed := true; changed; {
		changed = false
		for _, m := range RealIncidentMarkers {
			mk := strings.TrimSpace(m)
			if mk != "" && strings.HasPrefix(u, mk+" ") {
				u = strings.TrimSpace(u[len(mk):])
				changed = true
			}
		}
	}
	fields := strings.Fields(u)
	for len(fields) > 0 && localCrossNoiseTrailers[fields[0]] {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

// localNature returns the incident nature. It prefers a configured keyword
// (from the system's Keyword Lists, passed in as natureCodes) — whichever
// matches earliest in the transcript — then falls back to the earliest built-in
// real-incident marker, then "". "First detected keyword wins."
func localNature(upper string, natureCodes []string) string {
	padded := " " + upper + " "
	if n := fdRequestingPDNature(padded); n != "" {
		return n
	}

	// 1. Configured keyword lists: longest match wins; earliest position breaks
	// ties. Matching is token-based (shared with the nature scorer) so plural
	// phrase words, hyphenated compounds, and phrasal verbs behave uniformly.
	tokens := natureTranscriptTokens(upper)
	best := ""
	bestPos := -1
	bestLen := 0
	for _, code := range natureCodes {
		c := strings.ToUpper(strings.TrimSpace(code))
		if c == "" {
			continue
		}
		if isApparatusOnlyNatureLabel(c) || IsGenericNature(c) || isDispatchHarnessMarker(c) ||
			isBareCatchAllUnknownNature(c) {
			continue
		}
		if natureKeywordBlockedInMedicalDispatch(padded, c) {
			continue
		}
		if natureKeywordIsNegated(padded, c) {
			continue
		}
		if natureKeywordIsBreakRoomFalsePositive(padded, c) {
			continue
		}
		if natureKeywordIsLakeDepartmentFalsePositive(padded, c) {
			continue
		}
		if c == "INJURED" && strings.Contains(padded, " INJURED IN FALL ") {
			continue
		}
		if c == "VICTIM" && victimQualifierNature(padded, natureCodes) != "" {
			continue
		}
		positions := natureTermTokenMatch(tokens, c)
		if len(positions) == 0 {
			continue
		}
		idx := positions[0]
		if natureLabelIsStreetSuffixFalsePositive(c, padded) {
			continue
		}
		if best == "" || len(c) > bestLen || (len(c) == bestLen && idx < bestPos) {
			best = c
			bestPos = idx
			bestLen = len(c)
		}
	}
	if best != "" {
		return best
	}

	// 2. Built-in incident markers: longest match wins.
	best = ""
	bestPos = -1
	bestLen = 0
	for _, m := range RealIncidentMarkers {
		mk := strings.ToUpper(strings.TrimSpace(m))
		if mk == "" || isDispatchHarnessMarker(mk) {
			continue
		}
		if idx := strings.Index(padded, mk); idx >= 0 {
			if best == "" || len(mk) > bestLen || (len(mk) == bestLen && idx < bestPos) {
				best = mk
				bestPos = idx
				bestLen = len(mk)
			}
		}
	}
	if best != "" {
		if strings.EqualFold(best, "FELL") {
			return "FALL"
		}
		return best
	}

	// 3. Standard EMS dispatch phrases not always present in keyword lists.
	if ilpersonNatureRE.MatchString(upper) || strings.Contains(padded, " ILL PERSON ") ||
		strings.Contains(upper, "ILPERSON") {
		for _, pref := range []string{"ILL PERSON", "ILPERSON", "SICK PERSON"} {
			if n := matchNatureCodePref(padded, pref, natureCodes); n != "" {
				return n
			}
		}
		return "ILL PERSON"
	}

	return ""
}

func matchNatureCodePref(padded, pref string, natureCodes []string) string {
	p := strings.ToUpper(strings.TrimSpace(pref))
	for _, code := range natureCodes {
		c := strings.ToUpper(strings.TrimSpace(code))
		if c == "" {
			continue
		}
		if c == p || strings.Contains(c, p) || strings.Contains(p, c) {
			if wholeWordContains(padded, c) || strings.Contains(c, " ") == strings.Contains(p, " ") {
				return c
			}
		}
	}
	return ""
}

// localCommonName matches named (non-address) known places present in the
// transcript. Auto-learned address pins (display names starting with a house
// number) are skipped so we never use a street address as a common name.
//
// Matching is whole-word and length-gated: a junk OSM POI named "BACK" must not
// match inside "CALLBACK", and short generic names are ignored entirely. The
// longest (most specific) match wins.
func localCommonName(upper string, scope *ScopeData) string {
	best := ""
	for i := range scope.KnownPlaces {
		name := strings.ToUpper(strings.TrimSpace(scope.KnownPlaces[i].DisplayName))
		if len(strings.Fields(name)) < 2 && len(name) < 10 {
			continue // single short OSM names (e.g. "MEDLEY") must not snap from chatter
		}
		if name[0] >= '0' && name[0] <= '9' {
			continue // address-style pin, not a facility name
		}
		if containsWholeWord(upper, name) && len(name) > len(best) {
			if PlaceMentionIsNarrativeSubject(upper, name) {
				continue
			}
			best = name
		}
	}
	return best
}

// localFacilityCommonName captures a dispatch-named facility immediately before
// the street address ("AT FAIRHAVEN, 420 LINCOLN WAY") or a multi-word landmark
// after AT / AT THE ("AT THE CLEVELAND VA MEDICAL CENTER").
func localFacilityCommonName(upper string, scope *ScopeData) string {
	if name := localAtTheFacilityCommonName(upper, scope); name != "" {
		return name
	}
	idx := 0
	for idx < len(upper) {
		loc := localAtFacilityRE.FindStringSubmatchIndex(upper[idx:])
		if loc == nil {
			break
		}
		name := strings.TrimSpace(upper[idx+loc[2] : idx+loc[3]])
		if len(name) >= 4 && !suffixlessStreetStopwords[name] && !localStreetSuffixes[name] {
			if scope != nil {
				if p := scope.LookupKnownPlace(NormalizePlaceKey(name)); p != nil {
					return strings.ToUpper(strings.TrimSpace(p.DisplayName))
				}
			}
			rest := strings.TrimSpace(upper[idx+loc[1]:])
			if strings.HasPrefix(rest, ",") || regexp.MustCompile(`(?i)^,?\s*\d{1,6}\s+`).MatchString(rest) {
				return name
			}
		}
		idx += loc[1]
	}
	return ""
}

// localAtTheFacilityCommonName prefers multi-word AT THE / AT facility phrases
// that look like landmarks (hospital, medical center, school, …) or match the
// known-places gazetteer.
func localAtTheFacilityCommonName(upper string, scope *ScopeData) string {
	best := ""
	for _, m := range localAtTheFacilityRE.FindAllStringSubmatch(upper, -1) {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		name = trimFacilityTrailingNoise(name)
		if !plausibleFacilityCommonName(name) {
			continue
		}
		if scope != nil {
			if p := scope.LookupKnownPlace(NormalizePlaceKey(name)); p != nil {
				return strings.ToUpper(strings.TrimSpace(p.DisplayName))
			}
		}
		if isDispatchFacilityName(name) || len(strings.Fields(name)) >= 2 {
			if len(name) > len(best) {
				best = name
			}
		}
	}
	return best
}

func trimFacilityTrailingNoise(name string) string {
	words := strings.Fields(strings.ToUpper(strings.TrimSpace(name)))
	for len(words) > 0 {
		w := words[len(words)-1]
		if localCrossNoiseTrailers[w] || localStreetSuffixes[w] || suffixlessStreetStopwords[w] {
			words = words[:len(words)-1]
			continue
		}
		break
	}
	return strings.Join(words, " ")
}

func plausibleFacilityCommonName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if len(name) < 4 {
		return false
	}
	words := strings.Fields(name)
	if len(words) == 0 || len(words) > 7 {
		return false
	}
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}
	for _, bad := range []string{"STATION", "SQUAD", "MUTUAL", "OFF-DUTY", "OFF DUTY", "CODE", "CAT NUMBER"} {
		if strings.Contains(name, bad) && !isDispatchFacilityName(name) {
			return false
		}
	}
	if len(words) == 1 && (suffixlessStreetStopwords[words[0]] || localStreetSuffixes[words[0]]) {
		return false
	}
	return true
}

// localFacilityBeforeCity captures a named facility immediately before a
// dispatch city and house number ("NEW DAY RECOVERY, RIVER FALLS, 150 …").
func localFacilityBeforeCity(upper string) string {
	for _, m := range localFacilityCityRE.FindAllStringSubmatch(upper, -1) {
		if len(m) != 3 {
			continue
		}
		facility := strings.TrimSpace(m[1])
		city := strings.TrimSpace(m[2])
		if len(facility) < 4 || len(strings.Fields(facility)) > 5 {
			continue
		}
		if len(city) < 3 || len(strings.Fields(city)) > 3 {
			continue
		}
		for _, w := range strings.Fields(city) {
			if dispatchLocalityStopwords[w] {
				return ""
			}
		}
		for _, bad := range []string{"STATION", "SQUAD", "MUTUAL", "OFF-DUTY", "OFF DUTY"} {
			if strings.Contains(facility, bad) {
				return ""
			}
		}
		return facility
	}
	return ""
}

// localFacilityBeforeHouse captures a named business immediately before the
// house number when no city token is spoken ("BELLARIA PIZZA, 882 …").
func localFacilityBeforeHouse(upper string) string {
	for _, m := range localFacilityBeforeHouseRE.FindAllStringSubmatch(upper, -1) {
		if len(m) != 2 {
			continue
		}
		facility := strings.TrimSpace(m[1])
		if len(facility) < 4 || len(strings.Fields(facility)) > 5 {
			continue
		}
		for _, bad := range []string{"STATION", "SQUAD", "MUTUAL", "OFF-DUTY", "OFF DUTY", "FOR THE", "FOR AN"} {
			if strings.Contains(facility, bad) {
				return ""
			}
		}
		if !isDispatchFacilityName(facility) {
			continue
		}
		return facility
	}
	return ""
}

// localFacilityViaApartment captures a facility named in a double-VIA routing
// chain before an apartment ("HUBBARD VIA WEST PARK MANOR VIA APARTMENT 317").
func localFacilityViaApartment(upper string) string {
	for _, m := range localFacilityViaAptRE.FindAllStringSubmatch(upper, -1) {
		if len(m) != 2 {
			continue
		}
		facility := strings.TrimSpace(m[1])
		if len(facility) < 4 || len(strings.Fields(facility)) > 5 {
			continue
		}
		if localFacilityViaCaptureIsAddress(facility) {
			continue
		}
		skip := false
		for _, bad := range []string{"STATION", "SQUAD", "MUTUAL", "OFF-DUTY", "OFF DUTY"} {
			if strings.Contains(facility, bad) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if isDispatchFacilityName(facility) {
			return facility
		}
	}
	return ""
}

func localFacilityViaCaptureIsAddress(capture string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(capture)))
	if len(fields) == 0 {
		return true
	}
	if fields[0][0] >= '0' && fields[0][0] <= '9' {
		return true
	}
	for _, f := range fields {
		if localStreetSuffixes[f] {
			return true
		}
	}
	return false
}

func localInTheMobileHomePark(upper string) string {
	if m := localInTheMobileHomeParkRE.FindStringSubmatch(upper); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractOffOfStreetAddress captures dispatch phrasing where the location is
// named after OFF OF ("8976 … OFF OF HOWLAND WILSON SOUTHEAST").
func extractOffOfStreetAddress(upper string, scope *ScopeData) string {
	for _, m := range localOffOfStreetRE.FindAllStringSubmatch(upper, -1) {
		if len(m) != 2 {
			continue
		}
		raw := strings.TrimSpace(m[1])
		if raw == "" || dispatchStreetIsSpelledLetters(raw) {
			continue
		}
		house := ""
		if idx := strings.Index(upper, m[0]); idx > 0 {
			prefix := strings.TrimSpace(upper[max(0, idx-32):idx])
			if hm := regexp.MustCompile(`\b(\d{3,6})\b`).FindAllString(prefix, -1); len(hm) > 0 {
				house = hm[len(hm)-1]
			}
		}
		street := strings.TrimSpace(raw)
		if house != "" {
			street = house + " " + street
		}
		if scope != nil && len(scope.KnownStreets) > 0 {
			street = snapExtractedAddressToKnownStreet(street, scope.KnownStreets, upper)
		}
		if AddressHasGeocodableAnchor(street, scope) {
			return street
		}
	}
	return ""
}

func dispatchStreetIsSpelledLetters(street string) bool {
	street = strings.ToUpper(strings.TrimSpace(street))
	if strings.Contains(street, "-") {
		parts := strings.Split(street, "-")
		if len(parts) >= 2 {
			allLetters := true
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if len(p) != 1 || p < "A" || p > "Z" {
					allLetters = false
					break
				}
			}
			if allLetters {
				return true
			}
		}
	}
	return false
}

// containsWholeWord reports whether needle appears in haystack bounded by
// non-alphanumeric characters (or string edges) on both sides, so "BACK" does
// not match within "CALLBACK".
func containsWholeWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	from := 0
	for {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(needle)
		leftOK := start == 0 || !isAlphaNum(rune(haystack[start-1]))
		rightOK := end == len(haystack) || !isAlphaNum(rune(haystack[end]))
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
}

func isAlphaNum(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// snapExtractedAddressToKnownStreet rewrites a dispatch-extracted address when
// STT glued a locality onto a known street ("2720 SALT SPRINGS YOUNGSTOWN ROAD"
// → "2720 SALT SPRINGS ROAD" when SALT SPRINGS ROAD is in the gazetteer).
func snapExtractedAddressToKnownStreet(addr string, knownStreets []string, transcript string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(knownStreets) == 0 {
		return addr
	}
	if addressUsesNumberedRoute(street) {
		return addr
	}
	// Cleveland-style grid streets ("E 212 ST") must not snap to a bare
	// directional stem like known "E STREET".
	if fields := strings.Fields(street); len(fields) >= 3 && fields[0] == "E" && isAllDigits(fields[1]) {
		return addr
	}
	extName, extSuffix := streetNameAndSuffix(street)
	if extSuffix == "" && suffixlessHomonymsNeedCoveragePick(extName, knownStreets) {
		return addr
	}
	if extSuffix == "" {
		spokenNS := collapsedStreetWordsAfterHouse(strings.ToUpper(transcript), house)
		if len(spokenNS) >= 10 {
			for _, ks := range knownStreets {
				ku := strings.ToUpper(strings.TrimSpace(ks))
				stem, ksSuffix := streetNameAndSuffix(ku)
				if ksSuffix == "" || stem == "" {
					continue
				}
				if stripStreetSpaces(stem) == spokenNS {
					return house + " " + ku
				}
			}
		}
	}
	if extSuffix == "" && StreetHasOrdinalCore(street) {
		if inferred := inferOrdinalSuffixFromTranscript(house, street, transcript); inferred != "" {
			extSuffix = inferred
		}
	}
	qCanon := CanonicalStreetName(street)
	qDir, qCore, qType := splitStreetParts(qCanon)
	suffixlessSingle := extSuffix == "" && len(strings.Fields(street)) == 1
	bestStreet := ""
	bestLen := 0
	exactNameStreet := ""
	// exactFullMatch tracks a known street that is a byte-for-byte match of the
	// full extracted street (name+suffix, no extra directional/quadrant). This
	// must win over any longer known street that merely has `street` as a
	// prefix (e.g. dispatch said plain "MAHONING AVENUE", and the gazetteer
	// separately has both "MAHONING AVENUE" and "MAHONING AVENUE NORTHWEST" —
	// the bare exact match is correct; the longer superset match must not win
	// just because len() is bigger).
	exactFullMatch := ""
	paddedTranscript := " " + strings.ToUpper(transcript) + " "
	streetSnapOK := func(ku string) bool {
		if strings.TrimSpace(transcript) == "" {
			return true
		}
		return gazetteerStreetAlignsWithTranscript(strings.ToUpper(strings.TrimSpace(ku)), paddedTranscript)
	}
	preferExactTokenHomonyms := false
	if suffixlessSingle {
		n := 0
		for _, ks := range knownStreets {
			kn, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(ks)))
			if kn == extName && streetSnapOK(strings.ToUpper(strings.TrimSpace(ks))) {
				n++
			}
		}
		preferExactTokenHomonyms = n >= 2
	}
	// Suffixless stems with conflicting thoroughfare homonyms ("520 FENTON" with
	// both ROAD and STREET imports) defer suffix choice to coverage refinement.
	if suffixlessSingle && preferExactTokenHomonyms && suffixlessHomonymsNeedCoveragePick(extName, knownStreets) {
		return addr
	}
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		if ku == "" {
			continue
		}
		ksName, ksSuffix := streetNameAndSuffix(ku)
		if extSuffix == "" && len(strings.Fields(street)) == 1 &&
			ksName != extName && !strings.HasPrefix(ksName, extName+" ") {
			continue
		}
		if strings.Contains(street, "CREEK") && !strings.Contains(ku, "CREEK") {
			continue
		}
		if strings.Contains(ku, "COURT") && !strings.Contains(street, "COURT") &&
			strings.Contains(street, "CREEK") {
			continue
		}
		if extSuffix != "" {
			_, ksType := StreetCoreTypeKey(CanonicalStreetName(ku))
			if ksType != "" && ksType != extSuffix {
				continue
			}
		}
		if extSuffix != "" && ksSuffix != "" && !streetSuffixesCompatible(extSuffix, ksSuffix) {
			continue
		}
		if strings.HasPrefix(ku, street+" ") {
			if preferExactTokenHomonyms {
				// "10 SCOTT" with many SCOTT* homonyms — exactNameStreet picks among them.
			} else {
				if !ordinalStreetSuffixesCompatible(street, ku) {
					continue
				}
				if !streetSnapOK(ku) {
					continue
				}
				if len(ku) > bestLen {
					bestLen = len(ku)
					bestStreet = ku
				}
				continue
			}
		}
		if street == ku || strings.HasPrefix(street, ku+" ") {
			if !ordinalStreetSuffixesCompatible(street, ku) {
				continue
			}
			if !streetSnapOK(ku) {
				continue
			}
			if street == ku && exactFullMatch == "" {
				exactFullMatch = ku
			}
			if len(ku) > bestLen {
				bestLen = len(ku)
				bestStreet = ku
			}
			continue
		}
		cCanon := CanonicalStreetName(ku)
		if cCanon == "" {
			continue
		}
		cDir, cCore, cType := splitStreetParts(cCanon)
		// Dispatch said SOUTH MILTON — do not rewrite to unsigned MILTON BOULEVARD.
		if qDir != "" {
			if cDir != "" && !streetDirectionalsCompatible(qDir, cDir) {
				continue
			}
			if cDir == "" {
				continue
			}
		}
		if extName == ksName && streetSuffixesCompatible(extSuffix, ksSuffix) {
			if len(strings.Fields(street)) == 1 && knownStreetHasLongerPrefix(street, knownStreets) &&
				spokenStreetContinuesAfterMatch(strings.ToUpper(transcript), house, street) {
				continue
			}
			if !streetSnapOK(ku) {
				continue
			}
			if extSuffix != "" {
				if exactNameStreet == "" || len(ku) > len(exactNameStreet) {
					exactNameStreet = ku
				}
			}
		}
		if !streetSuffixesCompatible(extSuffix, ksSuffix) {
			continue
		}
		if !ordinalStreetSuffixesCompatible(street, ku) {
			continue
		}
		coreMatch := qCore != "" && qCore == cCore
		if !coreMatch && qCore != "" && cCore != "" {
			qk, _ := StreetCoreTypeKey(qCanon)
			ck, _ := StreetCoreTypeKey(cCanon)
			if qk != "" && ck != "" {
				coreMatch = ScoreStreetSTTCoreMatch(qk, ck, nil) >= sttMatchScoreThreshold
			} else {
				coreMatch = StreetTokensSTTMatch(qCore, cCore)
			}
		}
		if coreMatch && len(strings.Fields(extName)) == 1 {
			u := strings.ToUpper(transcript)
			if spokenStreetContinuesAfterMatch(u, house, extName) {
				coreMatch = false
			} else if len(strings.Fields(ksName)) > 1 && !strings.HasPrefix(ksName, extName+" ") {
				coreMatch = false
			}
		}
		if coreMatch {
			if qType != "" && cType != "" && qType != cType {
				continue
			}
			if q := spokenStreetQuadrant(street); q != "" {
				if kq := spokenStreetQuadrant(ksName); kq != "" && !streetDirectionalsCompatible(q, kq) {
					continue
				}
			}
			if !streetSnapOK(ku) {
				continue
			}
			if !preferExactTokenHomonyms {
				if len(ku) > bestLen {
					bestLen = len(ku)
					bestStreet = ku
				}
			}
			continue
		}
		if extName == ksName {
			if !streetSnapOK(ku) {
				continue
			}
			if len(ku) > bestLen {
				bestLen = len(ku)
				bestStreet = ku
			}
			continue
		}
		if StreetNamesSTTMatch(extName, ksName) {
			if len(strings.Fields(extName)) == 1 && streetLeadingNameToken(ksName) != extName {
				continue
			}
			if len(strings.Fields(extName)) == 1 && len(strings.Fields(ksName)) > 1 &&
				!strings.HasPrefix(ksName, extName+" ") {
				continue
			}
			if q := spokenStreetQuadrant(street); q != "" {
				if kq := spokenStreetQuadrant(ksName); kq != "" && !streetDirectionalsCompatible(q, kq) {
					continue
				}
			}
			if !streetSnapOK(ku) {
				continue
			}
			if len(ku) > bestLen {
				bestLen = len(ku)
				bestStreet = ku
			}
			continue
		}
		fields := strings.Fields(ku)
		if len(fields) < 2 || !localStreetSuffixes[fields[len(fields)-1]] {
			continue
		}
		// Suffixless dispatch streets ("7525 WARREN SHARON") must not snap to a
		// different thoroughfare that only shares the leading token ("WARREN AVENUE").
		if extSuffix == "" {
			continue
		}
		stem := strings.Join(fields[:len(fields)-1], " ")
		if strings.HasPrefix(street, stem+" ") && streetSuffixesCompatible(extSuffix, ksSuffix) {
			if len(ku) > bestLen {
				bestLen = len(ku)
				bestStreet = ku
			}
		}
	}
	if exactFullMatch != "" && !homonymSwapContradictsTranscript(addr, exactFullMatch, transcript) {
		return house + " " + exactFullMatch
	}
	if exactNameStreet != "" && extSuffix != "" &&
		!homonymSwapContradictsTranscript(addr, exactNameStreet, transcript) {
		return house + " " + exactNameStreet
	}
	if bestStreet != "" && !homonymSwapContradictsTranscript(addr, bestStreet, transcript) {
		return house + " " + bestStreet
	}
	if extSuffix == "" {
		if len(strings.Fields(street)) == 1 {
			return addr
		}
		cands := make([]string, 0, len(knownStreets))
		for _, ks := range knownStreets {
			if cc := CanonicalStreetName(ks); cc != "" {
				cands = append(cands, cc)
			}
		}
		if matched, ok := bestCollapsedCoreStreetMatch(street, cands); ok {
			for _, ks := range knownStreets {
				if CanonicalStreetName(ks) == matched {
					return house + " " + strings.ToUpper(strings.TrimSpace(ks))
				}
			}
		}
	}
	return addr
}

func suffixlessHomonymsNeedCoveragePick(stem string, knownStreets []string) bool {
	stem = strings.ToUpper(strings.TrimSpace(stem))
	if stem == "" {
		return false
	}
	suffixes := map[string]bool{}
	for _, ks := range knownStreets {
		name, suffix := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(ks)))
		if name != stem || suffix == "" {
			continue
		}
		if key, _ := StreetCoreTypeKey(suffix); key != "" {
			suffixes[key] = true
		}
	}
	return len(suffixes) >= 2
}

func knownStreetHasLongerPrefix(street string, knownStreets []string) bool {
	street = strings.ToUpper(strings.TrimSpace(street))
	if street == "" {
		return false
	}
	prefix := street + " "
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		if strings.HasPrefix(ku, prefix) && ku != street {
			return true
		}
	}
	return false
}

func streetNameAndSuffix(street string) (name, suffix string) {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(street)))
	n := len(fields)
	if n == 0 {
		return street, ""
	}
	// A trailing quadrant directional ("OLIVE STREET SOUTHEAST", "GARY AVE
	// NW") sits after the thoroughfare type, not in place of it. Nominatim's
	// display_name spells these out routinely; without checking one token
	// further back, the real suffix (STREET/AVE) is hidden behind the
	// direction and every result ending in a quadrant gets treated as
	// suffix-less, which upstream callers read as "not a real street match".
	sufIdx := n - 1
	if n >= 2 && directionalWordsCorrection[fields[n-1]] {
		sufIdx = n - 2
	}
	if sufIdx >= 1 && localStreetSuffixes[fields[sufIdx]] {
		suffix = canonicalStreetTokens(fields[sufIdx])
		nameFields := make([]string, 0, n-1)
		nameFields = append(nameFields, fields[:sufIdx]...)
		nameFields = append(nameFields, fields[sufIdx+1:]...)
		return strings.Join(nameFields, " "), suffix
	}
	return street, ""
}

func streetSuffixesCompatible(extSuffix, ksSuffix string) bool {
	if extSuffix == "" || ksSuffix == "" {
		return true
	}
	return extSuffix == ksSuffix
}

// glueCrossroadsListItems joins a bare thoroughfare type onto the prior stem
// ("CUMBERLAND", "CIRCLE" → "CUMBERLAND CIRCLE") so STT comma-splits of a
// typed cross street stay one name.
func glueCrossroadsListItems(parts []string) []string {
	var out []string
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		fields := strings.Fields(p)
		if len(fields) == 1 && localStreetSuffixes[fields[0]] && len(out) > 0 {
			prev := out[len(out)-1]
			if _, prevSuf := streetNameAndSuffix(prev); prevSuf == "" {
				out[len(out)-1] = strings.TrimSpace(prev + " " + fields[0])
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// crossStreetsFromCrossroadsList parses "CROSSROADS OF A, B, AND C" / "CROSS OF …"
// comma lists into snapped cross-street names.
func crossStreetsFromCrossroadsList(upper string, scope *ScopeData) []string {
	m := localCrossroadsListRE.FindStringSubmatch(upper)
	if len(m) < 2 {
		return nil
	}
	clause := strings.ToUpper(strings.TrimSpace(m[1]))
	if clause == "" || !strings.Contains(clause, ",") {
		// Pair forms ("CROSS OF FOSTER AND BRADLEY") stay on localCrossOfRE.
		return nil
	}
	clause = strings.ReplaceAll(clause, " AND ", ", ")
	var raw []string
	for _, part := range strings.Split(clause, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		raw = append(raw, part)
	}
	raw = glueCrossroadsListItems(raw)
	var out []string
	seen := map[string]bool{}
	for _, part := range raw {
		snapped := snapCrossStreet(part, scope)
		if snapped == "" {
			snapped = strings.ToUpper(strings.TrimSpace(part))
		}
		if snapped == "" || seen[snapped] {
			continue
		}
		seen[snapped] = true
		out = append(out, snapped)
	}
	return out
}

// localCrossStreets extracts up to two cross streets from explicit dispatcher
// phrasings ("BETWEEN X AND Y", "CROSS(ES) OF X (AND Y)", "CORNER OF X AND Y",
// "OFF (OF) X"), snapping each to a known street when possible.
func localCrossStreets(upper string, scope *ScopeData) (cs1, cs2 string) {
	set := func(a, b string) {
		if cs1 == "" {
			cs1 = a
		}
		if cs2 == "" {
			cs2 = b
		}
	}
	if m := localBetweenRE.FindStringSubmatch(upper); len(m) == 3 {
		set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
	}
	if cs1 == "" {
		if list := crossStreetsFromCrossroadsList(upper, scope); len(list) > 0 {
			if len(list) >= 1 {
				cs1 = list[0]
			}
			if len(list) >= 2 {
				cs2 = list[1]
			}
		}
	}
	if cs1 == "" {
		if m := localCornerRE.FindStringSubmatch(upper); len(m) == 3 {
			set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
		}
	}
	if cs1 == "" {
		if m := localCornerHouseRE.FindStringSubmatch(upper); len(m) == 3 {
			set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
		}
	}
	if cs1 == "" {
		if m := localAcrossOfAndRE.FindStringSubmatch(upper); len(m) >= 3 {
			set(snapCrossStreet(strings.TrimSpace(m[1]), scope), snapCrossStreet(strings.TrimSpace(m[2]), scope))
		}
	}
	if cs1 == "" {
		if m := localAcrossTheAndRE.FindStringSubmatch(upper); len(m) >= 3 {
			set(snapCrossStreet(strings.TrimSpace(m[1]), scope), snapCrossStreet(strings.TrimSpace(m[2]), scope))
		}
	}
	if cs1 == "" {
		if m := localCrossStreetsOfRE.FindStringSubmatch(upper); len(m) == 3 {
			set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
		}
	}
	if cs1 == "" {
		if m := localCrossStreetsAreRE.FindStringSubmatch(upper); len(m) == 3 {
			set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
		}
	}
	if cs1 == "" {
		if m := localCrossOfRE.FindStringSubmatch(upper); len(m) >= 2 {
			second := ""
			if len(m) == 3 {
				second = snapCrossStreet(m[2], scope)
			}
			set(snapCrossStreet(m[1], scope), second)
		}
	}
	if cs1 == "" {
		if m := localCrossUpRE.FindStringSubmatch(upper); len(m) >= 2 {
			second := ""
			if len(m) == 3 {
				second = snapCrossStreet(m[2], scope)
			}
			set(snapCrossStreet(m[1], scope), second)
		}
	}
	if cs1 == "" {
		if m := localCrossesUpRE.FindStringSubmatch(upper); len(m) == 2 {
			set(snapCrossStreet(m[1], scope), "")
		}
	}
	if cs1 == "" {
		if m := localCrossesAreRE.FindStringSubmatch(upper); len(m) == 3 {
			set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
		}
	}
	if cs1 == "" {
		if m := localCrossWithRE.FindStringSubmatch(upper); len(m) == 3 {
			set(snapCrossStreet(m[1], scope), snapCrossStreet(m[2], scope))
		}
	}
	if cs1 == "" {
		if m := localCrossesBareRE.FindStringSubmatch(upper); len(m) == 2 {
			raw := strings.TrimSpace(m[1])
			head := strings.ToUpper(strings.Fields(raw)[0])
			if head != "UP" && head != "OF" {
				set(snapCrossStreet(raw, scope), "")
			}
		}
	}
	if cs1 == "" {
		if m := localOffRE.FindStringSubmatch(upper); len(m) == 2 {
			raw := strings.TrimSpace(m[1])
			if !offCrossStreetCaptureIsStatusPhrase(upper, raw) {
				set(snapCrossStreet(raw, scope), "")
			}
		}
	}
	if cs1 == "" {
		if m := localAcrossRE.FindStringSubmatch(upper); len(m) == 2 {
			set(snapCrossStreet(m[1], scope), "")
		}
	}
	return cs1, cs2
}

func offCrossStreetCaptureIsStatusPhrase(upper, capture string) bool {
	capture = strings.ToUpper(strings.TrimSpace(capture))
	if capture != "DUTY" && capture != "DUTIES" {
		return false
	}
	return strings.Contains(upper, "OFF DUTY") || strings.Contains(upper, "OFF-DUTY") ||
		strings.Contains(upper, "DUTY OFF DUTY")
}

// resolveNowItsCrossStreet expands STT "NOW IT'S CORTLAND" to a gazetteer name
// like NILES CORTLAND ROAD when the mangled tail matches a known compound street.
func resolveNowItsCrossStreet(tail string, scope *ScopeData) string {
	tail = strings.ToUpper(strings.TrimSpace(tail))
	if tail == "" || scope == nil {
		return ""
	}
	var matches []string
	for _, ks := range scope.KnownStreets {
		ksU := strings.ToUpper(strings.TrimSpace(ks))
		for i, w := range strings.Fields(ksU) {
			if w == tail && i >= 1 {
				matches = append(matches, ksU)
				break
			}
		}
	}
	if len(matches) == 0 {
		return ""
	}
	return pickNowItsCompoundStreet(tail, matches)
}

// stripCrossStreetCaptureNoise removes dispatcher phrasing glued onto a captured
// street ("STREETS OF COVINGTON STREET" → "COVINGTON STREET").
func stripCrossStreetCaptureNoise(side string) string {
	side = strings.TrimSpace(strings.ToUpper(side))
	for {
		trimmed := side
		for _, p := range []string{
			"REPEATING YOUR CROSS STREETS OF ",
			"YOUR CROSS STREETS OF ",
			"CROSS STREETS OF ",
			"CROSS STREET OF ",
			"STREETS OF ",
			"CROSS OF ",
		} {
			if strings.HasPrefix(side, p) {
				side = strings.TrimSpace(side[len(p):])
				break
			}
		}
		fields := strings.Fields(side)
		if len(fields) >= 2 && (fields[0] == "STREETS" || fields[0] == "STREET") && fields[1] == "OF" {
			side = strings.TrimSpace(strings.Join(fields[2:], " "))
		}
		if side == trimmed {
			break
		}
	}
	return side
}

// snapCrossStreet cleans a captured cross-street phrase and resolves it to a
// known street name when possible. Returns "" when the phrase is not a
// recognizable street (so we never fabricate a cross street from filler words).
func snapCrossStreet(raw string, scope *ScopeData) string {
	if scope == nil {
		scope = &ScopeData{}
	}
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(raw)))
	for len(fields) > 0 && localCrossNoiseTrailers[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	for len(fields) > 0 && localCrossNoiseTrailers[fields[0]] {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 4 {
		fields = fields[len(fields)-4:]
	}
	candidate := strings.Join(fields, " ")
	candidate = stripCrossStreetCaptureNoise(candidate)
	candidate = ApplyCorrections(scope, candidate)
	candidate = normalizeRouteTokens(candidate)
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	if bareCrossStreetPrefixTooVague(candidate) {
		return ""
	}
	if m := nowItsCrossRE.FindStringSubmatch(candidate); len(m) == 2 {
		if resolved := resolveNowItsCrossStreet(m[1], scope); resolved != "" {
			return resolved
		}
	}
	if snapped := fuzzySnapCollapsedCrossStreet(candidate, scope); snapped != "" {
		return snapped
	}

	// Exact / substring match against the known-streets gazetteer.
	var prefixMatches []string
	for _, ks := range scope.KnownStreets {
		ksU := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(ks)))
		if ksU == "" {
			continue
		}
		if candidate == ksU {
			return ksU
		}
		// Do not snap "OAK HILL" to a shorter imported name like "OAK …" that
		// merely appears as a substring of the spoken compound.
		if strings.Contains(candidate, ksU) && len(ksU) >= len(candidate) {
			return ksU
		}
		if strings.HasPrefix(ksU, candidate+" ") {
			prefixMatches = append(prefixMatches, ksU)
			continue
		}
		cf := strings.Fields(candidate)
		if len(cf) == 1 && len(cf[0]) >= 4 && strings.HasPrefix(ksU, cf[0]+" ") {
			prefixMatches = append(prefixMatches, ksU)
		}
	}
	if len(prefixMatches) > 0 {
		return pickCrossStreetPrefixMatch(candidate, prefixMatches)
	}
	if stem := snapDirectionalStemCrossStreet(candidate, scope); stem != "" {
		return stem
	}

	// Accept a bare route ("SR 88") or a street ending in a thoroughfare word.
	cf := strings.Fields(candidate)
	if len(cf) >= 2 {
		switch cf[0] {
		case "SR", "US", "CR", "HWY":
			return candidate
		}
	}
	if len(cf) > 0 && localStreetSuffixes[cf[len(cf)-1]] {
		return candidate
	}
	// Directional + suffixless stem ("EAST BROAD", "NORTH STATE") — common when
	// STT drops STREET/AVENUE at intersections.
	if len(cf) == 2 && streetDirTokens[canonicalStreetTokens(cf[0])] &&
		suffixlessSingleStreetPlausible(cf[1]) {
		return candidate
	}
	if len(cf) == 1 && isShortRouteNumber(cf[0]) {
		if snapped := snapBareRouteNumberCrossStreet(cf[0], scope); snapped != "" {
			return snapped
		}
	}
	if len(cf) == 1 && suffixlessSingleStreetPlausible(cf[0]) {
		if TokenIsRadioCommsNoise(cf[0]) || tokenIsPatientSymptom(cf[0]) {
			return ""
		}
		if snapped := fuzzySnapCrossStreetStem(cf[0], scope); snapped != "" {
			return snapped
		}
		return cf[0]
	}
	return ""
}

// snapBareRouteNumberCrossStreet maps dispatch shorthand like "CROSS OF 88" to a
// gazetteer route name such as STATE ROUTE 88 when the number alone appears in
// the known-streets list.
func snapBareRouteNumberCrossStreet(routeNum string, scope *ScopeData) string {
	if scope == nil || !isShortRouteNumber(routeNum) {
		return ""
	}
	var matches []string
	seen := map[string]bool{}
	for _, ks := range scope.KnownStreets {
		ksU := strings.ToUpper(strings.TrimSpace(ks))
		if ksU == "" || seen[ksU] {
			continue
		}
		if m := localStateRouteRE.FindStringSubmatch(ksU); len(m) == 2 && m[1] == routeNum {
			seen[ksU] = true
			matches = append(matches, ksU)
			continue
		}
		if m := localBareNumberedRouteRE.FindStringSubmatch(ksU); len(m) == 2 && m[1] == routeNum {
			seen[ksU] = true
			matches = append(matches, ksU)
			continue
		}
		cf := strings.Fields(normalizeRouteTokens(ksU))
		if len(cf) == 2 && cf[0] == "SR" && cf[1] == routeNum {
			seen[ksU] = true
			matches = append(matches, ksU)
		}
	}
	if len(matches) == 0 {
		return "SR " + routeNum
	}
	best := matches[0]
	for _, m := range matches[1:] {
		if strings.Contains(m, "STATE ROUTE") && !strings.Contains(best, "STATE ROUTE") {
			best = m
		}
	}
	return best
}

// crossStreetLeadingFillers are common STT prefixes that get prepended to an
// otherwise recognizable cross-street name ("OUR HAVEN" for ARHAVEN).
var crossStreetLeadingFillers = map[string]bool{
	"OUR": true, "THE": true, "A": true, "AN": true, "OR": true,
}

// tailFuzzySnapTooLoose rejects isolated tail-token fuzzy snaps that only share
// weak edit-distance similarity ("THROUGH" → "THRUSH"). Real STT repairs either
// embed the tail in the matched core ("HAVEN" in "ARHAVEN") or are one-edit homophones.
func tailFuzzySnapTooLoose(tailQuery, matchedCore string) bool {
	tailQuery = stripStreetSpaces(CanonicalStreetName(tailQuery))
	matchedCore = stripStreetSpaces(CanonicalStreetName(matchedCore))
	if tailQuery == "" || matchedCore == "" {
		return true
	}
	if strings.Contains(matchedCore, tailQuery) || strings.Contains(tailQuery, matchedCore) {
		return false
	}
	if sttDoubledLetterHomophone(tailQuery, matchedCore) {
		return false
	}
	if levenshtein(tailQuery, matchedCore) <= 1 {
		return false
	}
	return true
}

// fuzzySnapCollapsedCrossStreet maps STT multi-word cross-street phrases to a
// known street by collapsing spaces and fuzzy-matching the core name.
func fuzzySnapCollapsedCrossStreet(candidate string, scope *ScopeData) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(candidate)))
	if scope == nil || len(fields) < 2 {
		return ""
	}
	collapsed := stripStreetSpaces(candidate)
	if len(collapsed) < 4 {
		return ""
	}
	type keyed struct {
		ks, core string
	}
	var entries []keyed
	seen := map[string]bool{}
	for _, ks := range scope.KnownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		if ku == "" || seen[ku] {
			continue
		}
		seen[ku] = true
		core := stripStreetSpaces(streetNameAndSuffixFirst(ku))
		if core == "" {
			continue
		}
		entries = append(entries, keyed{ks: ku, core: core})
	}
	if len(entries) == 0 {
		return ""
	}
	cores := make([]string, len(entries))
	for i, e := range entries {
		cores[i] = e.core
	}
	// lookup resolves a matched core back to its full gazetteer street name —
	// but only when exactly one distinct street shares that core. When
	// several real, differently-typed streets collapse to the same core
	// ("OAK HILL AVENUE"/"OAK HILL DRIVE"/"OAK HILL ROAD" all → "OAKHILL"),
	// picking whichever happened to be first in the gazetteer slice would
	// fabricate a specific thoroughfare type the transcript never said.
	// Returning "" here defers to the caller's prefix-match path, which
	// falls back to the bare, undecorated name in that ambiguous case.
	lookup := func(matchedCore string) string {
		var found string
		for _, e := range entries {
			if e.core != matchedCore {
				continue
			}
			if found != "" && !strings.EqualFold(found, e.ks) {
				return ""
			}
			found = e.ks
		}
		return found
	}
	queryCores := []string{collapsed}
	tailRaw := strings.Join(fields[1:], " ")
	tail := stripStreetSpaces(tailRaw)
	// A bare thoroughfare-type suffix ("STREET", "AVENUE", ...) as the whole
	// remainder — i.e. candidate was just "NAME SUFFIX" — is not a real
	// distinguishing stem and must never be used as its own fuzzy query core.
	// Left unguarded, canonicalizing it collapses to its abbreviation ("ST")
	// which then substring-matches almost any unrelated core starting with
	// those two letters ("STATE", "STANDARD", ...), fabricating a completely
	// wrong street out of an address that already has an exact gazetteer
	// match ("ERIE STREET") available via the caller's plain lookup.
	if tail != "" && tail != collapsed && !localStreetSuffixes[strings.ToUpper(strings.TrimSpace(tailRaw))] {
		queryCores = append(queryCores, tail)
	}
	for _, qc := range uniqueStrings(queryCores...) {
		matched, ok := BestFuzzyStreetMatch(CanonicalStreetName(qc), cores)
		if !ok {
			continue
		}
		// The same "is this actually close enough" tightness check applies
		// whether qc is the full glued candidate or just its tail — without
		// it, a query as different as "ERIESTREET" (candidate "ERIE STREET"
		// left un-stemmed here, unlike the gazetteer cores it's compared
		// against) can "win" a fuzzy match against an unrelated stem (e.g.
		// "STATE") merely because every real score is bad, silently
		// replacing an address that had an exact gazetteer match available
		// moments later in the caller.
		if tailFuzzySnapTooLoose(qc, matched) {
			continue
		}
		if ks := lookup(matched); ks != "" {
			return ks
		}
		// The core matched but resolved ambiguously across 2+ real streets —
		// don't fall through to the looser tail-suffix heuristic below, which
		// would just pick a different, equally arbitrary homonym.
		return ""
	}
	// Multi-word STT may prepend a spurious syllable while the tail still
	// matches the end of a known street core ("OUR HAVEN" → ARHAVEN).
	if len(tail) >= 4 {
		for _, e := range entries {
			prefixLen := len(e.core) - len(tail)
			if prefixLen < 0 || prefixLen > 3 {
				continue
			}
			if strings.HasSuffix(e.core, tail) {
				return e.ks
			}
		}
	}
	return ""
}

func fuzzySnapCrossStreetStem(stem string, scope *ScopeData) string {
	if scope == nil || len(scope.KnownStreets) == 0 {
		return ""
	}
	cands := make([]string, 0, len(scope.KnownStreets))
	seen := map[string]bool{}
	for _, ks := range scope.KnownStreets {
		if cc := CanonicalStreetName(ks); cc != "" && !seen[cc] {
			seen[cc] = true
			cands = append(cands, cc)
		}
	}
	matched, ok := BestFuzzyStreetMatch(CanonicalStreetName(stem), cands)
	if !ok {
		return ""
	}
	for _, ks := range scope.KnownStreets {
		if CanonicalStreetName(ks) == matched {
			return strings.ToUpper(strings.TrimSpace(ks))
		}
	}
	return ""
}

// bareCrossStreetPrefixTooVague reports when a single-word cross-street capture
// is only a directional ("BETWEEN NORTH AND VALLEY" while the address is NORTH
// RIVER ROAD) and must not prefix-snap to NORTH ROAD / NORTH BANK.
func bareCrossStreetPrefixTooVague(candidate string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(candidate)))
	if len(fields) != 1 {
		return false
	}
	switch fields[0] {
	case "NORTH", "SOUTH", "EAST", "WEST":
		return true
	default:
		return false
	}
}

// snapDirectionalStemCrossStreet maps dispatch shorthand like "CHAMPION EAST"
// to gazetteer names such as "CHAMPION AVENUE EAST".
func snapDirectionalStemCrossStreet(candidate string, scope *ScopeData) string {
	if scope == nil {
		return ""
	}
	cf := strings.Fields(strings.ToUpper(strings.TrimSpace(candidate)))
	if len(cf) != 2 {
		return ""
	}
	stem, dir := cf[0], canonicalStreetTokens(cf[1])
	if !streetDirTokens[dir] {
		return ""
	}
	best := ""
	bestLen := 0
	for _, ks := range scope.KnownStreets {
		ksU := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(ks)))
		ksf := strings.Fields(ksU)
		if len(ksf) < 3 || ksf[0] != stem || canonicalStreetTokens(ksf[len(ksf)-1]) != dir {
			continue
		}
		hasSuffix := false
		for i := 1; i < len(ksf)-1; i++ {
			if localStreetSuffixes[ksf[i]] {
				hasSuffix = true
				break
			}
		}
		if !hasSuffix {
			continue
		}
		if len(ksU) > bestLen {
			bestLen = len(ksU)
			best = ksU
		}
	}
	return best
}

// pickNowItsCompoundStreet resolves STT "NOW IT'S CORTLAND" to NILES CORTLAND ROAD
// rather than EVERETT CORTLAND HULL ROAD when both contain the spoken tail word.
func pickNowItsCompoundStreet(tail string, matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return matches[0]
	}
	tail = strings.ToUpper(strings.TrimSpace(tail))
	best := matches[0]
	bestScore := nowItsCompoundStreetScore(tail, best)
	for _, m := range matches[1:] {
		if s := nowItsCompoundStreetScore(tail, m); s > bestScore {
			best, bestScore = m, s
		}
	}
	return best
}

func nowItsCompoundStreetScore(tail, full string) int {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(full)))
	score := 0
	for i, w := range fields {
		if w != tail {
			continue
		}
		if i > 0 {
			score += 2
		}
		if i == len(fields)-2 && len(fields) > 1 && localStreetSuffixes[fields[len(fields)-1]] {
			score += 10
		}
		if i < len(fields)-2 {
			score -= 5
		}
	}
	score -= len(fields)
	return score
}

// pickCrossStreetPrefixMatch chooses among several gazetteer streets that share
// the same spoken prefix ("NORTH RIVER" → ROAD vs DRIVE).
func pickCrossStreetPrefixMatch(candidate string, matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return matches[0]
	}
	// Prefer matches without a trailing directional the candidate never
	// mentioned. Dispatch saying bare "STATE" should not become "STATE ROAD
	// NORTHWEST" just because that's the only imported "STATE ROAD*" — a
	// directional the gazetteer attached to only one of several same-stem
	// streets must never be invented.
	var plain []string
	for _, m := range matches {
		if !hasTrailingDirectionalInCanonical(CanonicalStreetName(m)) {
			plain = append(plain, m)
		}
	}
	if len(plain) > 0 {
		matches = plain
	}
	if len(matches) == 1 {
		return matches[0]
	}
	// Genuine ambiguity: the same bare name resolves to two or more distinct
	// real thoroughfare types (e.g. both "OAK HILL DRIVE" and "OAK HILL
	// AVENUE" exist) with nothing in the transcript indicating which one
	// dispatch meant. Guessing a specific type here would fabricate data the
	// transcript never said, so fall back to the bare, undecorated name
	// instead of a coin-flip pick that is wrong roughly as often as it's
	// right.
	firstSuffix := crossStreetMatchSuffix(candidate, matches[0])
	ambiguous := false
	for _, m := range matches[1:] {
		if crossStreetMatchSuffix(candidate, m) != firstSuffix {
			ambiguous = true
			break
		}
	}
	if ambiguous {
		return candidate
	}
	return matches[0]
}

// crossStreetMatchSuffix returns the canonical thoroughfare-type token
// (e.g. "RD", "AVE") that `full` adds onto `candidate`.
func crossStreetMatchSuffix(candidate, full string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(full), strings.ToUpper(candidate)))
	if rest == "" {
		return ""
	}
	return canonicalStreetTokens(strings.Fields(rest)[0])
}

// extractBareNumberedRouteWithBetween returns a numbered route (SR 7, STATE ROUTE 7)
// when dispatch names BETWEEN cross streets but did not give a credible house on
// that route ("876-8 ROUTE 7 … BETWEEN RICHARD AND WINGATE" — the leading digits
// are milepost/STT noise, not 8768 ROUTE AVE).
func extractBareNumberedRouteWithBetween(upper string, scope *ScopeData) string {
	betweenLoc := localBetweenRE.FindStringSubmatchIndex(upper)
	if betweenLoc == nil {
		return ""
	}
	mLoc := localBareNumberedRouteRE.FindStringSubmatchIndex(upper)
	if len(mLoc) != 4 {
		return ""
	}
	// The numbered route must BE the dispatched location before "BETWEEN", not
	// one of the two cross streets named after it ("2065 FLAG EAST ROAD WEST
	// BETWEEN CREASTER ASTRIBULA AND STATE ROUTE 45 NORTH" — the route is the
	// SECOND cross street of an already-named house address, not a bare route
	// dispatch). Reject when the route match falls inside either BETWEEN...AND
	// capture span.
	if len(betweenLoc) >= 6 {
		g1s, g1e := betweenLoc[2], betweenLoc[3]
		g2s, g2e := betweenLoc[4], betweenLoc[5]
		rs, re := mLoc[0], mLoc[1]
		if (rs >= g1s && re <= g1e) || (rs >= g2s && re <= g2e) {
			return ""
		}
	}
	m := localBareNumberedRouteRE.FindStringSubmatch(upper)
	if len(m) != 2 {
		return ""
	}
	routeNum := strings.TrimSpace(m[1])
	if routeNum == "" || !isShortRouteNumber(routeNum) {
		return ""
	}
	idx := strings.Index(strings.ToUpper(upper), strings.ToUpper(m[0]))
	if idx >= 3 {
		before := strings.ToUpper(upper[max(0, idx-3):idx])
		if strings.HasSuffix(strings.TrimSpace(before), "EN") {
			return ""
		}
	}
	for _, hm := range transcriptHouseStateRouteRE.FindAllStringSubmatch(upper, -1) {
		if len(hm) >= 3 && strings.TrimSpace(hm[2]) == routeNum {
			house := strings.TrimSpace(hm[1])
			if house != "" && !stationHouseNumberLooksLikeUnit(house, upper) {
				return house + " " + snapBareRouteNumberCrossStreet(routeNum, scope)
			}
		}
	}
	return snapBareRouteNumberCrossStreet(routeNum, scope)
}

// extractHighwayCrossAddress builds "SR 88 & HOFFMAN RD" style intersections
// from dispatch phrasing like "STATE ROUTE 88, CROSSES UP HOFFMAN".
func extractHighwayCrossAddress(upper string, scope *ScopeData) string {
	m := localStateRouteRE.FindStringSubmatch(upper)
	if len(m) != 2 {
		return ""
	}
	route := "SR " + strings.TrimSpace(m[1])
	cross := ""
	if cm := localCrossesUpRE.FindStringSubmatch(upper); len(cm) == 2 {
		crossRaw := cm[1]
		if idx := strings.Index(crossRaw, ","); idx >= 0 {
			crossRaw = crossRaw[:idx]
		}
		cross = snapCrossStreet(crossRaw, scope)
	}
	if cross == "" {
		cs1, _ := localCrossStreets(upper, scope)
		cross = cs1
	}
	if cross == "" || cross == route {
		return ""
	}
	a, b := route, cross
	if isPlausibleLocalStreet(cross, scope) || hasStreetSuffix(cross) {
		return a + " & " + b
	}
	return ""
}

func extractMangledPrefixIntersection(transcript string, scope *ScopeData) string {
	m := localMangledPrefixAndRE.FindStringSubmatch(transcript)
	if len(m) != 4 {
		return ""
	}
	spokenA := strings.ToUpper(strings.TrimSpace(m[2]))
	spokenB := strings.ToUpper(strings.TrimSpace(m[3]))
	a, b := spokenA, spokenB
	if snappedA := snapCrossStreet(spokenA, scope); snappedA != "" {
		snappedB := snapCrossStreet(spokenB, scope)
		if snappedB == "" {
			snappedB = spokenB
		}
		if AddressAlignsWithTranscript(snappedA+" & "+snappedB, transcript, scope) {
			a, b = snappedA, snappedB
		}
	}
	if a == "" || b == "" ||
		IntersectionSideIsNonStreet(a) || IntersectionSideIsNonStreet(b) ||
		!bareIntersectionSidePlausible(a, scope) || !bareIntersectionSidePlausible(b, scope) {
		return ""
	}
	if !AddressAlignsWithTranscript(a+" & "+b, transcript, scope) {
		return ""
	}
	return a + " & " + b
}

// ExtractMangledPrefixIntersectionForTest exposes mangled-prefix intersection parsing for eval.
func ExtractMangledPrefixIntersectionForTest(transcript string, scope *ScopeData) string {
	return extractMangledPrefixIntersection(transcript, scope)
}

// transcriptHasCleanNumberedDispatchAddress reports whether the transcript
// contains an unambiguous "### STREET" dispatch address. When one exists, a
// bare "X AND Y" elsewhere in the same transcript is virtually always the
// CROSS STREET annotation for that address ("602 GARY DRIVE, CROSSES OF DORIS
// AND MOORE"), not a substitute primary address — the numbered address must
// win the address slot, with X/Y still captured separately as cross streets
// by localCrossStreets.
func transcriptHasCleanNumberedDispatchAddress(transcript string) bool {
	return len(extractDispatchAddressesFromTranscript(transcript)) > 0
}

func extractBareAndIntersectionAddress(transcript string, scope *ScopeData) string {
	if transcriptHasCleanNumberedDispatchAddress(transcript) {
		return ""
	}
	for _, m := range localBareStreetAndRE.FindAllStringSubmatch(transcript, -1) {
		if len(m) != 3 {
			continue
		}
		if addr := validateBareIntersectionCapture(strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), transcript, scope); addr != "" {
			return addr
		}
	}
	return ""
}

// extractBareInIntersectionAddress recovers STT "X IN Y" for spoken "X AND Y"
// intersections. "IN" is far noisier than "AND", so both sides must look like
// streets AND the exact "X IN Y" phrase must be restated (CAD readback) or
// preceded by a dispatch location cue.
func extractBareInIntersectionAddress(transcript string, scope *ScopeData) string {
	if transcriptHasCleanNumberedDispatchAddress(transcript) {
		return ""
	}
	for _, m := range localBareStreetInRE.FindAllStringSubmatch(transcript, -1) {
		if len(m) != 3 {
			continue
		}
		rawA := strings.TrimSpace(m[1])
		rawB := strings.TrimSpace(m[2])
		if bareInIntersectionSideBlocked(rawA) || bareInIntersectionSideBlocked(rawB) {
			continue
		}
		rawAU := strings.ToUpper(rawA)
		rawBU := strings.ToUpper(rawB)
		if !bareIntersectionPhraseRepeatedWithConnector(transcript, rawAU, rawBU, " IN ") &&
			!bareIntersectionHasDispatchCueBeforePairWithConnector(transcript, rawAU, rawBU, " IN ") {
			continue
		}
		if addr := validateBareIntersectionCapture(rawA, rawB, transcript, scope); addr != "" {
			return addr
		}
	}
	return ""
}

func bareInIntersectionSideBlocked(side string) bool {
	u := strings.ToUpper(strings.TrimSpace(side))
	if u == "" {
		return true
	}
	// "IN THE AREA OF …", "IN FRONT OF …", "IN ROUTE …"
	for _, bad := range []string{"THE ", "FRONT ", "AREA ", "ROUTE ", "SERVICE ", "PROGRESS "} {
		if strings.HasPrefix(u, bad) {
			return true
		}
	}
	return false
}

func bareIntersectionPhraseRepeatedWithConnector(transcript, rawA, rawB, connector string) bool {
	u := strings.ToUpper(transcript)
	a := strings.ToUpper(strings.TrimSpace(rawA))
	b := strings.ToUpper(strings.TrimSpace(rawB))
	if a == "" || b == "" {
		return false
	}
	phrase := a + connector + b
	return strings.Count(u, phrase) >= 2
}

func bareIntersectionHasDispatchCueBeforePairWithConnector(transcript, rawA, rawB, connector string) bool {
	u := strings.ToUpper(transcript)
	a := strings.ToUpper(strings.TrimSpace(rawA))
	b := strings.ToUpper(strings.TrimSpace(rawB))
	idx := strings.Index(u, a+connector+b)
	if idx < 0 {
		return false
	}
	before := " " + u[:idx] + " "
	for _, cue := range bareIntersectionDispatchCues {
		if strings.Contains(before, cue) {
			return true
		}
	}
	return false
}

func extractCommaSeparatedIntersectionAddress(transcript string, scope *ScopeData) string {
	if transcriptHasCleanNumberedDispatchAddress(transcript) {
		return ""
	}
	u := strings.ToUpper(transcript)
	for i := 0; i < len(u); i++ {
		if u[i] != ',' {
			continue
		}
		for _, m := range localBareStreetCommaRE.FindAllStringSubmatch(u[i:], -1) {
			if len(m) != 3 {
				continue
			}
			if addr := validateBareIntersectionCapture(strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), transcript, scope); addr != "" {
				return addr
			}
		}
	}
	return ""
}

// bareIntersectionDispatchCues are spoken phrases that mark the following
// "X AND Y" as a geographic intersection rather than conversational listing
// ("PRINTOUT SCOTT AND LAUREN" is not "AT THE CORNER OF SCOTT AND LAUREN").
var bareIntersectionDispatchCues = []string{
	" CORNER OF ", " AT THE CORNER", " CROSS OF ", " CROSSES ", " CROSSING ",
	" CROSS STREET", " INTERSECTION ", " INTERSECTION OF ", " BETWEEN ",
	" RESPOND TO ", " RESPONDING TO ", " EN ROUTE TO ", " ENROUTE TO ",
	" BLOCK OF ", " LOCATED AT ", " IN THE AREA OF ", " VICINITY OF ",
}

func validateBareIntersectionCapture(rawA, rawB, transcript string, scope *ScopeData) string {
	if TranscriptIsAdministrativeLocationReference(transcript) &&
		!containsRealIncidentMarker(" "+strings.ToUpper(transcript)+" ") {
		return ""
	}
	rawA = trimBareIntersectionLeadIn(rawA)
	rawB = trimBareIntersectionLeadIn(rawB)
	if intersectionSideIsPatientStatus(rawA) || intersectionSideIsPatientStatus(rawB) {
		return ""
	}
	if !intersectionRawSideStructurallyValid(rawA, scope) || !intersectionRawSideStructurallyValid(rawB, scope) {
		return ""
	}
	rawAU := strings.ToUpper(strings.TrimSpace(rawA))
	rawBU := strings.ToUpper(strings.TrimSpace(rawB))
	if intersectionSideIsSingleBareToken(rawAU) && intersectionSideIsSingleBareToken(rawBU) &&
		!bareSingleTokenIntersectionDispatchValid(transcript, rawAU, rawBU) {
		return ""
	}
	a := snapCrossStreet(rawA, scope)
	b := snapCrossStreet(rawB, scope)
	if a == "" || b == "" ||
		IntersectionSideIsNonStreet(a) || IntersectionSideIsNonStreet(b) ||
		intersectionSideIsPatientStatus(a) || intersectionSideIsPatientStatus(b) ||
		intersectionCaptureIsMedicalFacilityPhrase(a, b, transcript) ||
		!bareIntersectionSidePlausible(a, scope) || !bareIntersectionSidePlausible(b, scope) {
		return ""
	}
	if !AddressAlignsWithTranscript(a+" & "+b, transcript, scope) {
		return ""
	}
	return a + " & " + b
}

// trimBareIntersectionLeadIn strips dispatch location lead-ins the bare-intersection
// regex often sweeps into side A ("RESPOND TO MAIN", "CROSS OF WEST KNIFE").
func trimBareIntersectionLeadIn(side string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(side)))
	for len(fields) > 1 && bareIntersectionLeadInWord(fields[0]) {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

func bareIntersectionLeadInWord(w string) bool {
	if intersectionLeadNonStreets[w] {
		return true
	}
	switch w {
	case "CORNER", "CROSS", "CROSSES", "CROSSING", "RESPOND", "RESPONDING",
		"EN", "ROUTE", "BLOCK", "LOCATED", "AREA", "VICINITY", "INTERSECTION",
		"BETWEEN", "THE", "OF", "AT", "TO", "STATION":
		return true
	}
	return false
}

// intersectionRawSideStructurallyValid rejects captures where non-street words
// were swept into an intersection side ("PRINTOUT SCOTT") or a multi-word
// phrase lacks any street structure and does not match the gazetteer.
func intersectionRawSideStructurallyValid(raw string, scope *ScopeData) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(raw)))
	if len(fields) == 0 {
		return false
	}
	if len(fields) == 1 {
		w := fields[0]
		return suffixlessSingleStreetPlausible(w) || isShortRouteNumber(w) || localStreetSuffixes[w]
	}
	hasStructure := false
	for _, w := range fields {
		if streetDirTokens[w] || directionalWordsCorrection[w] || localRouteKeywords[w] || localStreetSuffixes[w] {
			hasStructure = true
			break
		}
	}
	if hasStructure {
		for _, w := range fields {
			if streetDirTokens[w] || directionalWordsCorrection[w] || localRouteKeywords[w] || localStreetSuffixes[w] || isShortRouteNumber(w) {
				continue
			}
			if !suffixlessSingleStreetPlausible(w) {
				return false
			}
		}
		return true
	}
	candidate := strings.Join(fields, " ")
	if scope != nil {
		for _, ks := range scope.KnownStreets {
			ku := strings.ToUpper(strings.TrimSpace(ks))
			if ku == candidate || strings.HasPrefix(ku, candidate+" ") {
				return true
			}
		}
	}
	return false
}

func intersectionSideIsSingleBareToken(side string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(side)))
	if len(fields) != 1 {
		return false
	}
	w := fields[0]
	if localStreetSuffixes[w] || isShortRouteNumber(w) {
		return false
	}
	if _, ok := ohioStateRouteNumberInText(w); ok {
		return false
	}
	return true
}

func bareSingleTokenIntersectionDispatchValid(transcript, rawA, rawB string) bool {
	if transcriptSpeaksStreetSuffixForBareToken(rawA, transcript) ||
		transcriptSpeaksStreetSuffixForBareToken(rawB, transcript) {
		return true
	}
	if bareIntersectionHasDispatchCueBeforePair(transcript, rawA, rawB) {
		return true
	}
	return bareIntersectionPhraseRepeated(transcript, rawA, rawB)
}

// bareIntersectionPhraseRepeated reports whether the exact "A AND B" phrase
// occurs twice in the transcript ("NASH AND TAVERN, NASH AND TAVERN") — the
// standard CAD/dispatch convention of restating cross streets once for radio
// clarity, with no house number involved. Two dispatchers independently
// naming the identical pair of bare words is itself strong evidence this is a
// real location, not incidental phrasing (which is never said twice
// verbatim), so it stands in for an explicit "cross of"/"corner of" cue.
func bareIntersectionPhraseRepeated(transcript, rawA, rawB string) bool {
	u := strings.ToUpper(transcript)
	a := strings.ToUpper(strings.TrimSpace(rawA))
	b := strings.ToUpper(strings.TrimSpace(rawB))
	if a == "" || b == "" {
		return false
	}
	phrase := a + " AND " + b
	return strings.Count(u, phrase) >= 2
}

func transcriptSpeaksStreetSuffixForBareToken(token, transcript string) bool {
	token = strings.ToUpper(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	for suf := range localStreetSuffixes {
		if strings.Contains(u, " "+token+" "+suf+" ") ||
			strings.Contains(u, " "+token+" "+suf+",") ||
			strings.Contains(u, " "+token+", "+suf+" ") {
			return true
		}
	}
	return false
}

func bareIntersectionHasDispatchCueBeforePair(transcript, rawA, rawB string) bool {
	u := strings.ToUpper(transcript)
	a := strings.ToUpper(strings.TrimSpace(rawA))
	b := strings.ToUpper(strings.TrimSpace(rawB))
	idx := strings.Index(u, a+" AND "+b)
	if idx < 0 {
		aTok := a
		if fields := strings.Fields(a); len(fields) > 0 {
			aTok = fields[len(fields)-1]
		}
		idx = strings.Index(u, aTok+" AND "+b)
		if idx < 0 {
			return false
		}
	}
	before := " " + u[:idx] + " "
	for _, cue := range bareIntersectionDispatchCues {
		if strings.Contains(before, cue) {
			return true
		}
	}
	return false
}

var patientStatusIntersectionTerms = map[string]bool{
	"CONSCIOUS": true, "UNCONSCIOUS": true, "BREATHING": true, "ALERT": true,
	"ORIENTED": true, "RESPONSIVE": true, "UNRESPONSIVE": true, "PULSE": true,
	"SEIZURE": true, "VOMITING": true, "HEMORRHAGE": true, "BLEEDING": true,
	"WEAK": true, "WEAKNESS": true, "BUSY": true, "DIZZY": true, "DIZZINESS": true,
	"NAUSEOUS": true, "NAUSEA": true, "NAUSEATED": true, "VOMIT": true,
	"FAINT": true, "FAINTED": true, "LIGHTHEADED": true, "LETHARGIC": true,
	"DROWSY": true, "FEVER": true, "CHILLS": true, "SWEATING": true, "PALE": true,
}

func intersectionSideIsPatientStatus(side string) bool {
	for _, w := range strings.Fields(strings.ToUpper(strings.TrimSpace(side))) {
		if patientStatusIntersectionTerms[w] {
			return true
		}
	}
	return false
}

func tokenIsPatientSymptom(tok string) bool {
	return patientStatusIntersectionTerms[strings.ToUpper(strings.TrimSpace(tok))]
}

func bareIntersectionSidePlausible(side string, scope *ScopeData) bool {
	if isPlausibleLocalStreet(side, scope) {
		return true
	}
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(side)))
	if len(fields) == 1 {
		if scope != nil && len(scope.KnownStreets) > 0 {
			return suffixlessKnownGazetteerStem(fields[0], scope)
		}
		return suffixlessSingleStreetPlausible(fields[0])
	}
	if scope != nil && len(scope.KnownStreets) > 0 {
		return false
	}
	return suffixlessStreetNamePlausible(side)
}
