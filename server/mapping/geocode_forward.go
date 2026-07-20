// Copyright (C) 2025 Thinline Dynamic Solutions
//
// geocode_forward.go — forward geocode with dispatch-shorthand fallbacks and
// bias-aware candidate selection (e.g. GP EASTERLY vs GEAUGA PORTAGE EASTERLY).

package mapping

import (
	"regexp"
	"strings"
)

var (
	// House-number + named street, requires a thoroughfare word so we don't
	// match dispatch noise like "STATION 40-41-42 FOR THE …". One- and two-digit
	// house numbers are allowed (e.g. "41 SOUTH MAIN STREET"); the suffix gate
	// keeps "41 SQUAD CALL" and "7 AND 8 FOR …" from matching.
	transcriptDispatchAddressRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+([A-Z][A-Z\s\-]{2,40}?\s+(?:RD|ROAD|ST|STREET|AVE|AVENUE|DR|DRIVE|LN|LANE|CT|COURT|BLVD|BOULEVARD|PL|PLACE|PT|POINT|WAY|CIR|CIRCLE|TRL|TRAIL|TERR|HWY|HIGHWAY|PKWY|PARKWAY|RUN))\b`)

	// Suffixless two-word street after a house number.
	transcriptSuffixlessAddressRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+(?:AT\s+)?([A-Z][A-Z'\-]{2,}\s+[A-Z][A-Z'\-]{2,})\b`)

	transcriptHouseStreetLeadRE = regexp.MustCompile(`\b(\d{1,6})\s+[A-Z][A-Z'\-]{2,}`)

	// STT sometimes glues two house numbers before a suffixless street
	// ("1851-7525 WARREN SHARON" → prefer 7525 WARREN SHARON).
	transcriptDoubleHouseSuffixlessRE = regexp.MustCompile(`(?i)\b(\d{3,5})\s+(\d{3,5})\s+([A-Z][A-Z'\-]{2,}\s+[A-Z][A-Z'\-]{2,})\b`)

	// Cleveland-style grid addresses: "85 E 212 ST".
	transcriptGridEStreetRE = regexp.MustCompile(`(?i)\b(\d{1,4})\s+E\s+(\d{2,4})\s+(?:ST|STREET|RD|ROAD)\b`)

	// Directional numbered "grid" streets spoken without a thoroughfare suffix,
	// which is how urban numbered grids are almost always dispatched:
	// "3105 WEST 52ND", "1820 EAST 9TH", "455 W 25TH". The ordinal ("52ND") and
	// the trailing street type ("STREET"/"AVENUE") are both optional. A bare
	// single-letter directional ("455 W 25") must carry an ordinal suffix or an
	// explicit street type so a stray "5 W 3" cannot masquerade as an address;
	// the spelled directionals ("WEST 52") do not need that guard.
	transcriptGridDirStreetRE = regexp.MustCompile(`(?i)\b(\d{1,5})\s+(EAST|WEST|NORTH|SOUTH|E|W|N|S)\s+(\d{1,3}(?:ST|ND|RD|TH)?)(?:\s+(ST|STREET|AVE|AVENUE|BLVD|BOULEVARD|PL|PLACE))?\b`)

	// Repeated house number before a single-word suffixless street
	// ("12018 12018 BROOKLAWN" after digit-chain collapse).
	transcriptRepeatedHouseSuffixlessRE = regexp.MustCompile(`(?i)\b(\d{3,6})\s+(\d{3,6})\s+([A-Z][A-Z'\-]{3,})\b`)

	// Dispatch readback repeats house + suffixless street
	// ("12018 BROOKLAWN, 12018 BROOKLAWN, THERE'S …" or "21 FAIRVIEW, 21 FAIRVIEW").
	transcriptRepeatedDispatchReadbackRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+([A-Z][A-Z'\-]{3,})\s+(\d{1,6})\s+([A-Z][A-Z'\-]{3,})\b`)

	// "THE NUMBER 12, 12 SCOTT" — dispatcher explicitly frames the house
	// number with "the number", then repeats it immediately before a
	// single-word suffixless street. That explicit framing plus the
	// repetition (verified in code, not the regex — RE2 has no backrefs) is a
	// strong enough signal to accept even a short 1-2 digit house number,
	// unlike the general suffixless patterns above which require 3+ digits to
	// avoid misreading ages/counts/priority codes as house numbers. The
	// street suffix itself (e.g. "LANE") is intentionally never guessed here —
	// the bare "12 SCOTT" is left for the external geocoder to resolve.
	transcriptNumberFramedRepeatedHouseRE = regexp.MustCompile(`(?i)\bNUMBER\s+(\d{1,4})\s*,?\s+(\d{1,4})\s+([A-Z][A-Z'\-]{2,})\b`)

	// Single-word suffixless street after a house number ("12018 BROOKLAWN").
	transcriptSingleSuffixlessAddressRE = regexp.MustCompile(`(?i)\b(\d{3,6})\s+([A-Z][A-Z'\-]{3,})\b`)

	// Numbered county routes: "3201 COUNTY ROAD 225".
	transcriptCountyRoadRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+COUNTY\s+(?:ROAD|RD|ROUTE|RT)\s+(\d{1,4})\b`)

	countyRoadStemOnlyRE = regexp.MustCompile(`(?i)^COUNTY\s+(?:ROAD|RD|ROUTE|RT)$`)

	// Intersection in transcript: "X AND Y" or "X / Y" or "X & Y" between two
	// route/street tokens. Allows 2-char tokens (e.g. "88") on either side.
	transcriptIntersectionRE = regexp.MustCompile(`(?i)\b([A-Z0-9](?:[A-Z0-9\s\-]{0,30}?[A-Z0-9])?)\s+(?:AND|&|/)\s+([A-Z0-9](?:[A-Z0-9\s\-]{0,30}?[A-Z0-9])?)\b`)

	// Tokens that disqualify a transcript-extracted street segment.
	// CENTRAL is intentionally omitted: Cleveland (and many cities) have Central Ave
	// as a real thoroughfare ("4908 CENTRAL"). "CENTRAL" as a unit/callsign tag is
	// handled separately via callsignTagNoise.
	dispatchNoiseTokens = []string{"STATION", "STATIONS", "ENGINE", "MEDIC", "SQUAD", "YEAR", "YEARS", "MALE", "FEMALE", "TIME", "FOR THE", "MVA", "FOR AN", "FROM", "POSSIBLE", "POSSIBLY", "WITH", "PERFORMING", "THURSDAY", "RADIO", "TEST"}

	// stationApparatusPrefixRE matches a captured intersection side that is
	// *entirely* "STATION"/"STATIONS" followed by one or more comma-separated
	// numbers, e.g. "STATION 40" (from "STATION 40 AND 41") or "STATIONS 45, 22"
	// (from "STATIONS 45, 22 AND 21"). Anchored at both ends so it only fires
	// when the label directly and exclusively owns the number(s) on this side —
	// not merely present somewhere earlier in a longer, unrelated capture like
	// "STATION 12, RESPOND TO 534". This is a fire-apparatus station roll call,
	// never a road, so the whole candidate is rejected before trimNoisePrefix
	// has a chance to strip the label and leave a bare number that looks
	// route-ish.
	stationApparatusPrefixRE = regexp.MustCompile(`^STATIONS?\s+\d{1,3}(\s*,\s*\d{1,3})*$`)

	// stationApparatusRollCallTailRE matches a dangling "STATION(S) <n>(, <n>)*,"
	// list immediately preceding an intersection match, e.g. the text before
	// "22 AND 21" in "STATIONS 45, 22 AND 21" — side A ("22") isn't itself
	// prefixed with STATION, but it's still a station number, not a route.
	stationApparatusRollCallTailRE = regexp.MustCompile(`\bSTATIONS?\s+\d{1,3}(\s*,\s*\d{1,3})*\s*,\s*$`)
)

// multiHyphenDigitsRE matches three or more digit groups joined by hyphens,
// e.g. "4-0-3-9" which dispatch uses to spell out house numbers character-by-character.
var multiHyphenDigitsRE = regexp.MustCompile(`\b\d(?:-\d){2,}\b`)

// dispatchHouseReadbackBeforeStreetRE matches toned readback like "7525-7525 DAWSON"
// before generic hyphen-pair dedupe collapses it to a single house number.
var dispatchHouseReadbackBeforeStreetRE = regexp.MustCompile(`(?i)\b(\d{3,6})-(\d{1,6})\s+([A-Z][A-Z'-]{2,})`)

// hyphenDigitPairRE matches any two digit groups separated by a hyphen so we
// can post-process duplicates like "754-754" → "754" (RE2 has no backrefs).
var hyphenDigitPairRE = regexp.MustCompile(`\b(\d{2,5})-(\d{2,5})\b`)

// hyphenQuadDigitRE matches four digit groups joined by hyphens, used to detect
// "X-Y-X-Y" doubled-pair patterns ("30-23-30-23" → "3023").
var hyphenQuadDigitRE = regexp.MustCompile(`\b(\d{2,4})-(\d{2,4})-(\d{2,4})-(\d{2,4})\b`)

// repeatedHouseHyphenSuffixlessRE matches STT-glued readbacks like
// "144-144-WESTGATE" before digit-pair dedupe leaves "144-WESTGATE".
var repeatedHouseHyphenSuffixlessRE = regexp.MustCompile(`(?i)\b(\d{2,6})-(\d{2,6})-([A-Z][A-Z'-]{2,})\b`)

// houseHyphenSuffixlessStreetRE matches "144-WESTGATE" → "144 WESTGATE".
var houseHyphenSuffixlessStreetRE = regexp.MustCompile(`(?i)\b(\d{2,6})-([A-Z][A-Z'-]{2,})\b`)

// repeatedHouseBeforeRouteRE matches dispatch readback like "3613-3613-STATE ROUTE 88"
// before the generic hyphen-pair deduper leaves a spurious "3613-STATE ROUTE 88".
var repeatedHouseBeforeRouteRE = regexp.MustCompile(`(?i)\b(\d{3,6})-(\d{3,6})-(STATE ROUTE|ST ROUTE|SR)\s+(\d{1,3})\b`)

// hyphenHouseBeforeRouteRE repairs leftover "3613-STATE ROUTE 88" after partial dedupe.
var hyphenHouseBeforeRouteRE = regexp.MustCompile(`(?i)\b(\d{3,6})-(STATE ROUTE|ST ROUTE|SR)\s+(\d{1,3})\b`)

// transcriptHouseStateRouteRE matches numbered addresses on state routes ("3613 SR 88").
var transcriptHouseStateRouteRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+(?:STATE ROUTE|ST ROUTE|SR)\s+(\d{1,3})\b`)

// hyphenatedCompoundStreetRE matches letter-only compound street tokens such as
// "BARCLAY-MESSERLY" or "STROOP-HICKOX". STT and OpenAI often keep the hyphen
// when dispatch said two words; Google geocodes the space-separated form.
var hyphenatedCompoundStreetRE = regexp.MustCompile(`(?i)\b([A-Z]{2,})-([A-Z]{2,})\b`)

// sttCommaPausedCompoundStreetRE joins STT comma pauses inside multi-word road
// names ("YOUNGSTOWN, WARREN ROAD" → "YOUNGSTOWN WARREN ROAD").
var sttCommaPausedCompoundStreetRE = regexp.MustCompile(`(?i)\b([A-Z][A-Z']{1,}(?:\s+[A-Z][A-Z']{1,}){0,2})\s*,\s+([A-Z][A-Z']{1,}(?:\s+[A-Z][A-Z']{1,}){0,1}\s+(?:ROAD|RD|STREET|ST|AVENUE|AVE|DRIVE|DR|LANE|LN|BOULEVARD|BLVD|WAY|COURT|CT|PLACE|PL|TRAIL|TRL|HIGHWAY|HWY|PARKWAY|PKWY))\b`)

// sttCommaPausedIntersectionStemRE joins the same pause when the compound road
// is abbreviated before AND ("YOUNGSTOWN, WARREN AND NORTH ROAD").
var sttCommaPausedIntersectionStemRE = regexp.MustCompile(`(?i)\b([A-Z][A-Z']{2,}(?:\s+[A-Z][A-Z']{2,}){0,2})\s*,\s+([A-Z][A-Z']{2,})\s+(AND|&)\s+`)

// cityCommaStandaloneStreetStems are common street names that follow a real
// city/locality comma ("IN WARREN, MAIN STREET") — not compound arterials.
var cityCommaStandaloneStreetStems = map[string]bool{
	"MAIN": true, "HIGH": true, "CENTER": true, "CENTRE": true, "MARKET": true,
	"BROAD": true, "CHURCH": true, "STATE": true, "COUNTY": true, "FIRST": true,
	"SECOND": true, "THIRD": true, "FOURTH": true, "FIFTH": true, "SIXTH": true,
	"PARK": true, "OAK": true, "ELM": true, "MAPLE": true, "PINE": true,
	"WATER": true, "RIVER": true, "LAKE": true, "HILL": true, "MILL": true,
}

// dispatchSpelledEStreetStutterRE drops a garbled STT prefix when dispatch
// repeats a spelled east-side route ("85-E-2-123-85-E-2-1-2" → "85-E-2-1-2").
var dispatchSpelledEStreetStutterRE = regexp.MustCompile(`(?i)\b(\d{1,4})-E-2-(\d+)-(\d{1,4})-E-2-(\d)-(\d)\b`)

// dispatchSpelledEStreetRE rewrites spelled east-side routes such as
// "85-E-2-1-2" into "85 E 212" before address extraction.
var dispatchSpelledEStreetRE = regexp.MustCompile(`(?i)\b(\d{1,4})-E-2-(\d)-(\d)\b`)

// dispatchSpelledEStreetBareRE matches grid-style east-side routes without a
// trailing street type ("85 E 212" from spelled STT).
var dispatchSpelledEStreetBareRE = regexp.MustCompile(`(?i)\b(\d{1,4})\s+E\s+(\d{2,4})\b`)

// bareStreetBeforeFacilityRE matches "850 COLUMBIA AT THE …" when STT drops
// the street-type suffix before a facility name.
var bareStreetBeforeFacilityRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+([A-Z]{3,})\s+AT\s+THE\b`)

// expandHyphenatedCompoundStreetNames rewrites compound street tokens from
// "BARCLAY-MESSERLY" → "BARCLAY MESSERLY". Digit-hyphen tokens (house numbers,
// route shorthands) are left unchanged.
func expandHyphenatedCompoundStreetNames(addr string) string {
	return hyphenatedCompoundStreetRE.ReplaceAllString(addr, "$1 $2")
}

// joinSttCommaPausedCompoundStreets removes STT commas that split multi-word
// road names ("YOUNGSTOWN, WARREN ROAD AND NORTH ROAD"). Skips "CITY, MAIN
// STREET" style locality commas where the following stem is a common standalone
// street name.
func joinSttCommaPausedCompoundStreets(transcript string) string {
	if transcript == "" || !strings.Contains(transcript, ",") {
		return transcript
	}
	out := sttCommaPausedCompoundStreetRE.ReplaceAllStringFunc(transcript, func(m string) string {
		sub := sttCommaPausedCompoundStreetRE.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		rightFields := strings.Fields(strings.ToUpper(sub[2]))
		if len(rightFields) == 0 || cityCommaStandaloneStreetStems[rightFields[0]] {
			return m
		}
		return sub[1] + " " + sub[2]
	})
	return sttCommaPausedIntersectionStemRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := sttCommaPausedIntersectionStemRE.FindStringSubmatch(m)
		if len(sub) != 4 {
			return m
		}
		if cityCommaStandaloneStreetStems[strings.ToUpper(strings.TrimSpace(sub[2]))] {
			return m
		}
		return sub[1] + " " + sub[2] + " " + sub[3] + " "
	})
}

func expandSpelledDirectionalStreetInDispatchTranscript(transcript string) string {
	t := strings.ReplaceAll(transcript, "-E2-", "-E-2-")
	t = dispatchSpelledEStreetStutterRE.ReplaceAllStringFunc(t, func(m string) string {
		sub := dispatchSpelledEStreetStutterRE.FindStringSubmatch(m)
		if len(sub) != 6 || sub[1] != sub[3] {
			return m
		}
		return sub[3] + "-E-2-" + sub[4] + "-" + sub[5]
	})
	t = dispatchSpelledEStreetRE.ReplaceAllString(t, "$1 E 2$2$3")
	return appendStreetSuffixToBareEStreet(t)
}

func appendStreetSuffixToBareEStreet(transcript string) string {
	idxs := dispatchSpelledEStreetBareRE.FindAllStringSubmatchIndex(transcript, -1)
	if len(idxs) == 0 {
		return transcript
	}
	var b strings.Builder
	last := 0
	for _, pair := range idxs {
		if len(pair) < 6 {
			continue
		}
		start, end := pair[0], pair[1]
		after := strings.ToUpper(strings.TrimSpace(transcript[end:]))
		if streetTypeFollowsBareEStreet(after) {
			b.WriteString(transcript[last:end])
			last = end
			continue
		}
		b.WriteString(transcript[last:start])
		b.WriteString(transcript[pair[2]:pair[3]])
		b.WriteString(" E ")
		b.WriteString(transcript[pair[4]:pair[5]])
		b.WriteString(" ST")
		last = end
	}
	b.WriteString(transcript[last:])
	return b.String()
}

func streetTypeFollowsBareEStreet(after string) bool {
	if after == "" {
		return false
	}
	first := strings.Fields(after)
	if len(first) == 0 {
		return false
	}
	return geocodeStreetSuffixes[first[0]]
}

func expandBareStreetBeforeFacility(transcript string) string {
	return bareStreetBeforeFacilityRE.ReplaceAllStringFunc(transcript, func(m string) string {
		sub := bareStreetBeforeFacilityRE.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		street := strings.ToUpper(strings.TrimSpace(sub[2]))
		if geocodeStreetSuffixes[street] {
			return m
		}
		for _, noise := range dispatchNoiseTokens {
			if street == noise || strings.HasPrefix(street, noise) {
				return m
			}
		}
		return sub[1] + " " + street + " ROAD AT THE"
	})
}

// collapseMultiHyphenSpelledDigits collapses dispatch STT digit artifacts:
//   - "4-0-3-9" → "4039" (single-digit spelling)
//   - "754-754" → "754" (STT repetition of one number)
//   - "30-23-30-23" → "3023" (STT repetition of a multi-syllable number)
//
// Universal — pattern only.
func collapseMultiHyphenSpelledDigits(transcript string) string {
	out := dispatchHouseReadbackBeforeStreetRE.ReplaceAllStringFunc(transcript, func(m string) string {
		sub := dispatchHouseReadbackBeforeStreetRE.FindStringSubmatch(m)
		if len(sub) != 4 || sub[1] != sub[2] {
			return m
		}
		return sub[1] + " " + sub[1] + " " + sub[3]
	})
	out = repeatedHouseBeforeRouteRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := repeatedHouseBeforeRouteRE.FindStringSubmatch(m)
		if len(sub) != 5 || sub[1] != sub[2] {
			return m
		}
		return sub[1] + " " + sub[3] + " " + sub[4]
	})
	out = repeatedHouseHyphenSuffixlessRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := repeatedHouseHyphenSuffixlessRE.FindStringSubmatch(m)
		if len(sub) != 4 || sub[1] != sub[2] || !suffixlessSingleStreetPlausible(sub[3]) {
			return m
		}
		return sub[1] + " " + sub[3]
	})
	out = multiHyphenDigitsRE.ReplaceAllStringFunc(out, func(m string) string {
		return strings.ReplaceAll(m, "-", "")
	})
	out = hyphenQuadDigitRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := hyphenQuadDigitRE.FindStringSubmatch(m)
		if len(sub) == 5 && sub[1] == sub[3] && sub[2] == sub[4] {
			return sub[1] + sub[2]
		}
		return m
	})
	out = hyphenDigitPairRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := hyphenDigitPairRE.FindStringSubmatch(m)
		if len(sub) == 3 && sub[1] == sub[2] {
			return sub[1]
		}
		return m
	})
	out = houseHyphenSuffixlessStreetRE.ReplaceAllStringFunc(out, func(m string) string {
		sub := houseHyphenSuffixlessStreetRE.FindStringSubmatch(m)
		if len(sub) != 3 || !suffixlessSingleStreetPlausible(sub[2]) {
			return m
		}
		return sub[1] + " " + sub[2]
	})
	out = hyphenHouseBeforeRouteRE.ReplaceAllString(out, "$1 $2 $3")
	return out
}

// stayRouteMishearRE matches STT "STAY ROUTE" for dispatch "STATE ROUTE".
var stayRouteMishearRE = regexp.MustCompile(`(?i)\bSTAY\s+(ROUTE|RTE)\b`)

// dispatchHouseRouteRE matches a house number followed by a numbered state route.
var dispatchHouseRouteRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s*,?\s*(?:STATE\s+(?:ROUTE|RTE)|ST\s+(?:ROUTE|RTE)|SR|STAY\s+(?:ROUTE|RTE))\s+(\d{1,4})\b`)

var (
	usDotRouteRE = regexp.MustCompile(`(?i)\bU\.?\s*S\.?\s+(ROUTE|RTE|HWY|HIGHWAY)\b`)
	usDotBareRE  = regexp.MustCompile(`(?i)\bU\.\s*S\.`)
)

// expandUSPeriodAbbreviations rewrites spoken "U.S. ROUTE 422" to "US ROUTE 422"
// before cross-street regexes run. Those patterns treat "." as a phrase
// terminator, so "CROSSES OF U.S. ROUTE 422" otherwise captures only "U".
func expandUSPeriodAbbreviations(transcript string) string {
	if transcript == "" || !strings.Contains(strings.ToUpper(transcript), "U.") {
		return transcript
	}
	out := usDotRouteRE.ReplaceAllString(transcript, "US $1")
	out = usDotBareRE.ReplaceAllString(out, "US")
	return out
}

// expandSpokenStateRoutesInDispatchTranscript rewrites dispatch route tokens such
// as "BETWEEN 5-4" (State Route 534) before OpenAI / matching.
func expandSpokenStateRoutesInDispatchTranscript(transcript string) string {
	out := expandUSPeriodAbbreviations(transcript)
	out = stayRouteMishearRE.ReplaceAllStringFunc(out, func(m string) string {
		if strings.Contains(strings.ToUpper(m), "RTE") {
			return "ST RTE"
		}
		return "STATE ROUTE"
	})
	out = collapseMultiHyphenSpelledDigits(out)
	// Single hyphen "5-4" used as a route number (only when preceded/followed by
	// connectors that show it's not a house number).
	replacements := []struct{ pat, rep string }{
		{`(?i)\bBETWEEN\s+5-4\b`, `BETWEEN SR 534`},
		{`(?i)\bAND\s+5-4\b`, `AND SR 534`},
		{`(?i)\bAT\s+5-4\b`, `AT SR 534`},
		{`(?i)\b5-4\s+IN\b`, `SR 534 IN`},
	}
	for _, r := range replacements {
		re := regexp.MustCompile(r.pat)
		out = re.ReplaceAllString(out, r.rep)
	}
	return out
}

// dispatchRouteStreetAfterHouse returns "STATE ROUTE N" when dispatch named a
// numbered state route after the house ("3379, STAY ROUTE 46" → STATE ROUTE 46).
func dispatchRouteStreetAfterHouse(house, transcript string) string {
	house = strings.ToUpper(strings.TrimSpace(house))
	if house == "" {
		return ""
	}
	u := strings.ToUpper(strings.TrimSpace(transcript))
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(house) + `\s*,?\s*(?:STATE\s+(?:ROUTE|RTE)|ST\s+(?:ROUTE|RTE)|SR|STAY\s+(?:ROUTE|RTE))\s+(\d{1,4})\b`)
	m := re.FindStringSubmatch(u)
	if len(m) < 2 {
		return ""
	}
	return "STATE ROUTE " + strings.TrimSpace(m[1])
}

// applyHouseStateRouteFromTranscript rewrites an address when dispatch named a
// numbered state route for the same house number.
func applyHouseStateRouteFromTranscript(addr, transcript string) string {
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" {
		return addr
	}
	cleaned := PreCleanTranscript(transcript)
	if route := dispatchRouteStreetAfterHouse(house, cleaned); route != "" {
		return house + " " + route
	}
	return addr
}

// transcriptContainsNoise returns true when the candidate looks like dispatch
// chatter (station numbers, age/gender, etc.) rather than a real address.
func transcriptContainsNoise(s string) bool {
	u := " " + strings.ToUpper(s) + " "
	for _, n := range dispatchNoiseTokens {
		if strings.Contains(u, " "+n+" ") {
			return true
		}
	}
	return false
}

// dispatchSuffixedPointStreetPlausible rejects chatter like "FIRE ALARM EAGLE
// POINT" where POINT is a thoroughfare suffix but the leading tokens are not
// a street name ("EAGLE POINT" is valid).
func dispatchSuffixedPointStreetPlausible(st string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(st)))
	if len(fields) < 2 {
		return true
	}
	last := fields[len(fields)-1]
	if last != "POINT" && last != "PT" {
		return true
	}
	if len(fields) != 2 {
		return false
	}
	return !suffixlessStreetStopwords[fields[0]] && !screenNumberWordNoise[fields[0]]
}

// stationHouseNumberLooksLikeUnit reports when a captured house number is really
// a station/unit label ("STATION 12, FIRE ALARM, EAGLE POINT").
func stationHouseNumberLooksLikeUnit(house, transcript string) bool {
	h := strings.TrimSpace(house)
	if h == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, "STATION "+h) ||
		strings.Contains(u, "STATION "+h+",") ||
		strings.Contains(u, "STATION "+h+" ") {
		return true
	}
	// "STATIONS 37 AND 31" — apparatus roll-call, not a house number.
	if strings.Contains(u, "STATIONS") {
		if strings.Contains(u, "STATIONS "+h+" ") || strings.Contains(u, "STATIONS "+h+".") ||
			strings.Contains(u, "STATIONS "+h+",") {
			return true
		}
		if strings.Contains(u, " AND "+h+".") || strings.Contains(u, " AND "+h+",") ||
			strings.Contains(u, " AND "+h+" ") {
			return true
		}
	}
	// STT glues station ids at the tone opener ("3435, YOU'RE TURNING IN …" = stations 34+35).
	if len(h) == 4 && isAllDigits(h) && plausibleFireStationNumber(h[:2]) && plausibleFireStationNumber(h[2:]) {
		if dispatchRepeatsHouseWithStreetName(u, h) {
			return false
		}
		trim := strings.TrimSpace(u)
		if strings.HasPrefix(trim, h+",") || strings.HasPrefix(trim, h+" ") {
			rest := strings.TrimLeft(trim[len(h):], ", ")
			if toks := strings.Fields(rest); len(toks) > 0 {
				tok := strings.TrimRight(toks[0], ".,;:?!")
				// "1306 WEST 116TH" is a house opener, not glued stations 13+06.
				if houseNumberOpenerIsStreetContinuation(tok) {
					return false
				}
			}
			return true
		}
	}
	// "28, DUTY, OFF DUTY" / "46 DUTY OFF DUTY" — apparatus roster, not house 28.
	if plausibleFireStationNumber(h) && (strings.Contains(u, "OFF DUTY") || strings.Contains(u, "OFF-DUTY")) {
		if strings.Contains(u, h+", DUTY") || strings.Contains(u, h+" DUTY") ||
			strings.Contains(u, h+", DUTY,") {
			return true
		}
	}
	// "21, AUTO ACCIDENT, …" / "21 AUTO ACCIDENT, …" — station/apparatus id before incident type.
	if plausibleFireStationNumber(h) {
		for _, opener := range []string{
			"AUTO ACCIDENT", "MOTOR VEHICLE ACCIDENT", "FIRE ALARM", "SQUAD CALL",
			"STRUCTURE FIRE", "VEHICLE FIRE", "GAS LEAK", "WIRES DOWN", "MVA",
		} {
			if strings.HasPrefix(u, h+", "+opener) || strings.HasPrefix(u, h+" "+opener) {
				return true
			}
		}
	}
	return false
}

// priorityCodeHouseRE matches dispatch priority codes ("CODE 4", "YOUR CODE 6")
// that suffixless harvesters misread as house numbers.
var priorityCodeHouseRE = regexp.MustCompile(`(?i)\b(?:YOUR\s+)?CODE\s+(\d{1,2})\b`)

// houseNumberFromPriorityCode reports when a captured house number is really a
// scene-priority code spoken immediately before a jurisdiction name.
// dispatchAddressIsUnitPositionSelfReport matches "(THEY/WE/HE/SHE) ARE/IS/WAS
// ON <num> <street>" — a responding unit reporting its OWN current travel
// position ("THEY ARE ON 11 PASSING NEW ROAD"), not the incident's dispatched
// address. This phrasing looks identical to a real house-number address to
// transcriptDispatchAddressRE and, worse, its scoring bonus for later
// transcript position lets it beat the real (earlier) dispatch address
// entirely — so it must be excluded at extraction, not merely scored lower.
var dispatchAddressIsUnitPositionSelfReportRE = regexp.MustCompile(
	`(?i)\b(?:THEY|WE|HE|SHE|UNIT|UNITS)\s+(?:ARE|IS|WAS|WERE)\s+ON\s+(\d{1,6})\s+`)

func dispatchAddressIsUnitPositionSelfReport(num, transcript string) bool {
	num = strings.TrimSpace(num)
	if num == "" {
		return false
	}
	for _, m := range dispatchAddressIsUnitPositionSelfReportRE.FindAllStringSubmatch(transcript, -1) {
		if len(m) >= 2 && m[1] == num {
			return true
		}
	}
	return false
}

func houseNumberFromPriorityCode(house, transcript string) bool {
	h := strings.TrimSpace(house)
	if h == "" {
		return false
	}
	for _, m := range priorityCodeHouseRE.FindAllStringSubmatch(transcript, -1) {
		if len(m) >= 2 && m[1] == h {
			return true
		}
	}
	return false
}

// houseNumberFromAge reports demographics like "AGE 24, NOTHING ENTERED" where
// the age digits were harvested as a house number.
func houseNumberFromAge(house, transcript string) bool {
	h := strings.TrimSpace(house)
	if h == "" || !isAllDigits(h) || len(h) > 3 {
		return false
	}
	u := strings.ToUpper(transcript)
	if !regexp.MustCompile(`\bAGE\s+`+regexp.QuoteMeta(h)+`\b`).MatchString(u) {
		return false
	}
	// Real "24 MAIN ST" still contains the house beside a street token that is
	// not the AGE phrase; allow those through when the address street appears
	// next to the number outside the AGE clause.
	return true
}

// AddressHouseNumberFromAge clears extractions whose house number only appears
// as a spoken age ("AGE 24").
func AddressHouseNumberFromAge(addr, transcript string) bool {
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	return houseNumberFromAge(house, transcript)
}

// houseNumberOpenerIsStreetContinuation reports when the token immediately after
// a leading house number is street material (WEST / 116TH / CEDAR), not unit
// chatter (YOU'RE / GOT / CALLING).
func houseNumberOpenerIsStreetContinuation(tok string) bool {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	if tok == "" {
		return false
	}
	if homonymStreetDirToken(tok) || streetDirTokens[canonicalStreetTokens(tok)] {
		return true
	}
	if ordinalDigitTokenRE.MatchString(tok) {
		return true
	}
	if localStreetSuffixes[tok] {
		return true
	}
	if strings.Contains(tok, "'") || suffixlessStreetStopwords[tok] {
		return false
	}
	return len(tok) >= 3
}

// dispatchRepeatsHouseWithStreetName reports toned readbacks like "1417, 1417
// KIEFER ROAD" where a four-digit opener is a real house number, not glued
// station ids ("3435" = stations 34 and 35).
func dispatchRepeatsHouseWithStreetName(transcript, house string) bool {
	u := strings.ToUpper(strings.TrimSpace(transcript))
	house = strings.TrimSpace(house)
	if house == "" {
		return false
	}
	patterns := []string{
		house + " " + house + " ",
		house + ", " + house + " ",
		house + "," + house + " ",
		house + "- " + house + " ",
		house + "-" + house + " ",
	}
	for _, p := range patterns {
		if idx := strings.Index(u, p); idx >= 0 {
			after := strings.TrimSpace(u[idx+len(p):])
			if after == "" {
				continue
			}
			tok := strings.FieldsFunc(after, func(r rune) bool {
				return r == ',' || r == '.'
			})[0]
			if len(tok) >= 3 && !localStreetSuffixes[tok] && !suffixlessStreetStopwords[tok] {
				return true
			}
		}
	}
	// Full CAD read-back: "1864 LELAND AVENUE, 1864 LELAND" — the house
	// number followed by a street phrase, then (later) the same house number
	// again followed by the same street word. This is the standard dispatch
	// convention of restating the whole address once for radio clarity, not
	// the immediate digit stutter ("1864 1864 LELAND") the patterns above
	// catch — it needs its own scan since the two house-number mentions are
	// separated by the full street name/suffix rather than adjacent.
	fields := strings.Fields(u)
	firstIdx := -1
	for i, f := range fields {
		if strings.TrimRight(f, ",.") == house {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 || firstIdx+1 >= len(fields) {
		return false
	}
	firstStreetWord := strings.TrimRight(fields[firstIdx+1], ",.")
	if len(firstStreetWord) < 3 || localStreetSuffixes[firstStreetWord] || suffixlessStreetStopwords[firstStreetWord] {
		return false
	}
	for i := firstIdx + 1; i < len(fields)-1; i++ {
		if strings.TrimRight(fields[i], ",.") != house {
			continue
		}
		if strings.TrimRight(fields[i+1], ",.") == firstStreetWord {
			return true
		}
	}
	return false
}

func houseNumberLooksLikeCalendarYear(num, streetToken string) bool {
	if len(num) != 4 || !isAllDigits(num) {
		return false
	}
	if num < "2020" || num > "2039" {
		return false
	}
	// "2024 ROBBINS" is a valid house number + street, not a year mention.
	if suffixlessSingleStreetPlausible(streetToken) {
		return false
	}
	return true
}

// normalizeDispatchAddressSeparators rewrites STT comma pauses between address
// tokens ("2720, SALT SPRINGS, YOUNGSTOWN ROAD") into spaces so the house
// number regex can see a continuous "2720 … ROAD" phrase.
func normalizeDispatchAddressSeparators(transcript string) string {
	transcript = strings.ReplaceAll(transcript, ".", " ")
	return strings.Join(strings.Fields(strings.ReplaceAll(transcript, ",", " ")), " ")
}

// dispatchHouseMatchPrecededByHyphen reports when a captured house number is the
// trailing digit of a hyphenated STT token ("2-1 FAIRVIEW" → false match on "1").
func dispatchHouseMatchPrecededByHyphen(t string, matchStart int) bool {
	if matchStart <= 0 {
		return false
	}
	p := matchStart
	for p > 0 && (t[p-1] == ' ' || t[p-1] == '\t') {
		p--
	}
	return p > 0 && t[p-1] == '-'
}

// dispatchHouseFromRoomPhoneExtension reports when a house number is the trailing
// digits of a facility room/extension readout ("ROOM 328-690") rather than a street address.
func dispatchHouseFromRoomPhoneExtension(house, t string) bool {
	house = strings.TrimSpace(house)
	if house == "" {
		return false
	}
	pat := `(?i)\b(?:ROOM|RM|APARTMENT|APT|UNIT)\.?\s+(?:#?\s*)?(?:\d{1,4}-)*` + regexp.QuoteMeta(house) + `\b`
	return regexp.MustCompile(pat).MatchString(t)
}

// dispatchHouseFromLotOrUnitLabel reports trailer-park / lot labels spoken as
// "LOT 14" / "SPACE 12" after a real street address — not house numbers.
func dispatchHouseFromLotOrUnitLabel(house, t string) bool {
	house = strings.TrimSpace(house)
	if house == "" {
		return false
	}
	pat := `(?i)\b(?:LOT|SPACE|SPC|TRAILER|APT|APARTMENT|UNIT)\.?\s*#?\s*` + regexp.QuoteMeta(house) + `\b`
	return regexp.MustCompile(pat).MatchString(t)
}

// dispatchSuffixedStreetIsHyphenCompound reports when a harvested thoroughfare
// suffix is really the first half of a hyphenated incident phrase in the
// transcript (DRIVE inside DRIVE-BY). Go's \b treats "-" as a boundary, so the
// dispatch address regex otherwise matches DRIVE as AVENUE/DRIVE/etc.
func dispatchSuffixedStreetIsHyphenCompound(street, transcript string) bool {
	street = strings.ToUpper(strings.TrimSpace(street))
	transcript = strings.ToUpper(transcript)
	if street == "" || transcript == "" {
		return false
	}
	fields := strings.Fields(street)
	last := fields[len(fields)-1]
	if !localStreetSuffixes[last] && !geocodeStreetSuffixes[last] {
		return false
	}
	return strings.Contains(transcript, last+"-")
}

// dispatchStreetHasNarrativeFiller rejects multi-word "street" captures that
// include spoken narration between the stem and a thoroughfare suffix
// ("CENTRAL LOOKS LIKE WE HAVE A DRIVE" from a DRIVE-BY broadcast).
func dispatchStreetHasNarrativeFiller(street string) bool {
	for _, w := range strings.Fields(strings.ToUpper(strings.TrimSpace(street))) {
		switch w {
		case "LOOKS", "LIKE", "HAVE", "HAS", "HAD", "WE", "WE'RE", "WERE",
			"THEY", "THEY'RE", "THEIR", "SOMEONE", "SOMEBODY", "GETTING",
			"STILL", "CURRENTLY", "SUSPECT", "VICTIM", "OVER":
			return true
		}
	}
	return false
}

// dispatchSuffixlessStreetIsNonLocationPhrase reports clinical/time fragments that
// suffixless harvesters misread as street names ("690 NOW CHEST", "15 MINUTES AGO",
// "21 AUTO ACCIDENT" where 21 is apparatus and AUTO ACCIDENT is the incident type).
func dispatchSuffixlessStreetIsNonLocationPhrase(st string) bool {
	st = strings.ToUpper(strings.TrimSpace(st))
	switch st {
	case "NOW CHEST", "CHEST TINGLING", "LEFT ARM", "MINUTES AGO", "MINUTE AGO", "WE'RE GOING", "WERE GOING", "THIS REQUEST",
		"AUTO ACCIDENT", "MOTOR VEHICLE ACCIDENT", "MOTOR VEHICLE", "INJURY ACCIDENT",
		"YOUR TURN", "YOUR TIME", "YOUR TIMEOUT", "YOUR TIME OUT",
		"NOTHING ENTERED", "NOTHING FOUND", "NO WANTS", "NO WARRANTS",
		"HE'S WAITING", "HES WAITING", "SHE'S WAITING", "SHES WAITING",
		"WAITING FOR", "SECOND TOWN", "FIRST TOWN", "THIRD TOWN", "FOURTH TOWN",
		"CAN YOU", "START HEADING", "HEADING OVER",
		"OVER AT", "OVER BY", "OVER NEAR",
		"IT'S", "ITS",
		// CAD priority / status codes ("019 PRIORITY 2") — never thoroughfares.
		"PRIORITY", "PRIORITIES",
		// Alarm/nature fragments ("5 COMMERCIAL PRIVATE" from BATTALION 5, …).
		"COMMERCIAL PRIVATE", "PRIVATE ALARM", "COMMERCIAL ALARM":
		return true
	}
	fields := strings.Fields(st)
	if len(fields) == 0 {
		return false
	}
	if fields[len(fields)-1] == "OVER" {
		return true
	}
	// Contractions / pronouns never appear in US street names ("75 HE'S WAITING",
	// "1739 IT'S" from "RING ROUTE HAS 1739. IT'S FOR …", "48 THAT'S RAY" from
	// a BMV person return).
	for _, f := range fields {
		switch strings.Trim(f, ".,") {
		case "HE'S", "SHE'S", "WE'RE", "THEY'RE", "I'M", "IT'S", "THAT'S",
			"HES", "SHES", "WERE", "IM", "ITS", "THATS":
			return true
		}
	}
	if len(fields) >= 2 && (fields[0] == "WE'RE" || fields[0] == "WERE") && fields[1] == "GOING" {
		return true
	}
	if len(fields) == 2 && fields[1] == "TOWN" &&
		(fields[0] == "FIRST" || fields[0] == "SECOND" || fields[0] == "THIRD" || fields[0] == "FOURTH") {
		return true
	}
	if fields[0] == "YOUR" && (len(fields) == 1 || fields[1] == "TURN" || fields[1] == "TIME" || fields[1] == "TIMEOUT") {
		return true
	}
	if fields[0] == "NOTHING" {
		return true
	}
	switch fields[0] {
	case "NOW", "LEFT", "RIGHT", "MINUTES", "MINUTE", "ABOUT", "STARTED", "ALSO",
		"CAN", "START", "HEADING", "PRIORITY", "PRIORITIES":
		return true
	case "YOU'RE", "WERE", "TURNING", "SQUALL", "SQUAD", "SIXTY", "SIX", "SORRY", "I'M", "IM":
		return true
	}
	if len(fields) >= 2 && fields[0] == "CAN" && fields[1] == "YOU" {
		return true
	}
	if len(fields) >= 2 && fields[len(fields)-1] == "AGO" {
		return true
	}
	return false
}

// suffixlessStreetIsJurisdictionRoster reports when a suffixless capture is a
// municipality/department named in crew-availability chatter ("POLAND ONLY HAVE ONE").
func suffixlessStreetIsJurisdictionRoster(street, transcript string) bool {
	stem, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(street)))
	if stem == "" {
		for _, f := range strings.Fields(strings.ToUpper(strings.TrimSpace(street))) {
			if len(f) >= 3 {
				stem = f
				break
			}
		}
	}
	if stem == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	return strings.Contains(u, stem+" ONLY HAVE") ||
		strings.Contains(u, stem+" WOULD NOT BE RESPONDING")
}

// extractDispatchAddressesFromTranscript finds "4039 SOME ST" style tokens. It
// first merges dispatch-style hyphenated house numbers ("45-18 EAST ROYALTY
// ROAD" → "4518 EAST ROYALTY ROAD") so grid/queens-style addresses extract even
// when the street is not in the known-streets gazetteer.
func extractDispatchAddressesFromTranscript(transcript string) []string {
	// BMV person/plate returns ("51 OUT OF GENEVA, VALID") must not harvest
	// unit IDs + name fragments as house/street ("48 THAT'S RAY").
	if TranscriptIsPlateOrLookupChatter(transcript) || TranscriptIsLicensePlateReadout(transcript) ||
		TranscriptIsPersonDescriptionReadout(transcript) {
		return nil
	}
	t := strings.ToUpper(normalizeDispatchAddressSeparators(
		collapseMultiHyphenSpelledDigits(expandHyphenHouseNumbersInDispatchTranscript(transcript))))
	var out []string
	seen := map[string]bool{}
	for _, m := range transcriptCountyRoadRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 3 {
			continue
		}
		q := strings.TrimSpace(m[1] + " COUNTY ROAD " + m[2])
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptHouseStateRouteRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 3 {
			continue
		}
		num := strings.TrimSpace(m[1])
		routeNum := strings.TrimSpace(m[2])
		if num == "" || routeNum == "" {
			continue
		}
		if stationHouseNumberLooksLikeUnit(num, t) {
			continue
		}
		q := strings.TrimSpace(num + " SR " + routeNum)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptDispatchAddressRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 3 {
			continue
		}
		num := strings.TrimSpace(m[1])
		st := strings.TrimSpace(m[2])
		if num == "" || st == "" || len(st) < 3 {
			continue
		}
		// "41, 42 IN THE AREA OF SALT SPRINGS YOUNGSTOWN ROAD" — the number is a
		// unit/station self-report, not a house number, and the marker phrase
		// precedes (not follows) the real street. dispatchStreetCutAtConnector
		// only trims a *trailing* connector, so a leading one like this would
		// otherwise survive whole and get glued onto the number as a fabricated
		// house address ("42 IN THE AREA OF SALT SPRINGS YOUNGSTOWN ROAD").
		if dispatchStreetHasLeadingSelfLocationMarker(st) {
			continue
		}
		st = dispatchStreetCutAtConnector(st)
		if st == "" || len(st) < 3 || strings.Contains(st, " BETWEEN ") {
			continue
		}
		st = maybeExtendCompoundThoroughfareSuffix(num, st, t)
		if transcriptContainsNoise(st) {
			continue
		}
		if dispatchSuffixlessStreetIsNonLocationPhrase(st) {
			continue
		}
		// "4908 CENTRAL … DRIVE-BY SHOOTING" — after periods become spaces, DRIVE
		// matches as a thoroughfare suffix inside DRIVE-BY and harvests narrative
		// filler as the street name. Reject hyphen-compound suffixes and narrative
		// stems so the real suffixless "4908 CENTRAL" survives truncation guards.
		if dispatchSuffixedStreetIsHyphenCompound(st, t) || dispatchStreetHasNarrativeFiller(st) {
			continue
		}
		if dispatchStreetIsApparatusStatus(st) {
			continue
		}
		if dispatchStreetIsUnitWillBeStatusPhrase(st) {
			continue
		}
		if dispatchStreetIsRadioSignOff(st) {
			continue
		}
		if !dispatchSuffixedPointStreetPlausible(st) {
			continue
		}
		if stationHouseNumberLooksLikeUnit(num, t) {
			continue
		}
		if houseNumberFromPriorityCode(num, t) {
			continue
		}
		if houseNumberFromAge(num, t) {
			continue
		}
		if houseNumberFromUnitWillBeStatus(num, t) {
			continue
		}
		if dispatchAddressIsUnitPositionSelfReport(num, t) {
			continue
		}
		// "3201 COUNTY ROAD" without route number — prefer transcriptCountyRoadRE match.
		if countyRoadStemOnlyRE.MatchString(st) {
			continue
		}
		q := strings.TrimSpace(num + " " + st)
		if AddressIsConversationalNoise(q) {
			continue
		}
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptGridEStreetRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 3 {
			continue
		}
		q := strings.TrimSpace(m[1] + " E " + m[2] + " ST")
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, idx := range transcriptGridDirStreetRE.FindAllStringSubmatchIndex(t, -1) {
		if len(idx) < 10 {
			continue
		}
		full0 := idx[0]
		full1 := idx[1]
		house := strings.TrimSpace(t[idx[2]:idx[3]])
		dirRaw := strings.ToUpper(strings.TrimSpace(t[idx[4]:idx[5]]))
		ord := strings.ToUpper(strings.TrimSpace(t[idx[6]:idx[7]]))
		suf := ""
		if idx[8] >= 0 {
			suf = strings.ToUpper(strings.TrimSpace(t[idx[8]:idx[9]]))
		}
		// A bare single-letter directional needs an ordinal suffix ("25TH") or an
		// explicit street type; otherwise it is too weak to be an address.
		if len(dirRaw) == 1 && !ordinalDigitTokenRE.MatchString(ord) && suf == "" {
			continue
		}
		// "STATE ROUTE 45 NORTH. 74-YEAR-OLD MALE" — after periods flatten to
		// spaces, this looks identical to a real grid address ("3105 WEST
		// 52ND"), but 45 is a route number (already claimed by the preceding
		// ROUTE/SR keyword as a directional route reference) and 74 is the next
		// clause's spoken age, not an ordinal street continuation. Both
		// conditions must hold for a genuine match to be rejected here, since
		// either alone is common in real addresses (e.g. a house number that
		// happens to also be a plausible age, or "COUNTY ROAD 45").
		if bareNumberedStreetPrecededByRouteRE.MatchString(t[:full0]) &&
			bareNumberedStreetFollowedByAgeRE.MatchString(t[full1:]) {
			continue
		}
		street := expandGridDirectionalWord(dirRaw) + " " + gridOrdinalWithSuffix(ord) + " " + gridStreetTypeWord(suf)
		q := strings.TrimSpace(house + " " + street)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptDoubleHouseSuffixlessRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 4 {
			continue
		}
		st := strings.TrimSpace(m[3])
		if !suffixlessStreetNamePlausible(st) {
			continue
		}
		q := strings.TrimSpace(m[2] + " " + st)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptHyphenPairBeforeSuffixlessRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 4 {
			continue
		}
		st := strings.TrimSpace(m[3])
		if dispatchSuffixlessStreetIsNonLocationPhrase(st) {
			continue
		}
		if dispatchHouseFromRoomPhoneExtension(strings.TrimSpace(m[2]), t) {
			continue
		}
		if dispatchHouseFromLotOrUnitLabel(strings.TrimSpace(m[2]), t) {
			continue
		}
		if !suffixlessStreetNamePlausible(st) {
			continue
		}
		q := strings.TrimSpace(m[2] + " " + st)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptRepeatedHouseSuffixlessRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 4 || m[1] != m[2] {
			continue
		}
		st := strings.TrimSpace(m[3])
		st = dispatchStreetCutAtConnector(st)
		if st == "" || !suffixlessSingleStreetPlausible(st) {
			continue
		}
		q := strings.TrimSpace(m[1] + " " + st)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptRepeatedDispatchReadbackRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 5 || m[1] != m[3] || m[2] != m[4] {
			continue
		}
		st := strings.TrimSpace(m[2])
		st = dispatchStreetCutAtConnector(st)
		if st == "" || !suffixlessSingleStreetPlausible(st) {
			continue
		}
		if stationHouseNumberLooksLikeUnit(m[1], t) {
			continue
		}
		q := strings.TrimSpace(m[1] + " " + st)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptNumberFramedRepeatedHouseRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 4 || m[1] != m[2] {
			continue
		}
		num := strings.TrimSpace(m[1])
		st := strings.TrimSpace(m[3])
		st = dispatchStreetCutAtConnector(st)
		if st == "" || !suffixlessSingleStreetPlausible(st) {
			continue
		}
		if dispatchSuffixlessStreetIsNonLocationPhrase(st) {
			continue
		}
		if stationHouseNumberLooksLikeUnit(num, t) {
			continue
		}
		q := strings.TrimSpace(num + " " + st)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, m := range transcriptSingleSuffixlessAddressRE.FindAllStringSubmatchIndex(t, -1) {
		if len(m) < 6 {
			continue
		}
		if dispatchHouseMatchPrecededByHyphen(t, m[2]) {
			continue
		}
		num := strings.TrimSpace(t[m[2]:m[3]])
		st := strings.TrimSpace(t[m[4]:m[5]])
		if dispatchHouseFromRoomPhoneExtension(num, t) {
			continue
		}
		if dispatchHouseFromLotOrUnitLabel(num, t) {
			continue
		}
		st = dispatchStreetCutAtConnector(st)
		if num == "" || st == "" || !suffixlessSingleStreetPlausible(st) {
			continue
		}
		if suffixlessCaptureHasObjectPhraseTail(num, st, t) ||
			suffixlessMatchFollowedByNarrativeVerb(num, st, t) {
			continue
		}
		if dispatchSuffixlessStreetIsNonLocationPhrase(st) {
			continue
		}
		if suffixlessStreetIsJurisdictionRoster(st, t) {
			continue
		}
		if stationHouseNumberLooksLikeUnit(num, t) {
			continue
		}
		if houseNumberFromPriorityCode(num, t) {
			continue
		}
		if houseNumberFromAge(num, t) {
			continue
		}
		if houseNumberFromUnitWillBeStatus(num, t) {
			continue
		}
		if houseNumberLooksLikeCalendarYear(num, st) {
			continue
		}
		full := t[m[0]:m[1]]
		if suffixlessMatchFollowedBySuffix(t, full) {
			continue
		}
		if suffixlessMatchFollowedByRouteNumber(t, full) {
			continue
		}
		if suffixlessSingleStreetIsAgencyCityPrefix(t, full, st) {
			continue
		}
		q := strings.TrimSpace(num + " " + st)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	for _, pair := range transcriptSuffixlessAddressRE.FindAllStringSubmatchIndex(t, -1) {
		if len(pair) < 6 {
			continue
		}
		if dispatchHouseMatchPrecededByHyphen(t, pair[2]) {
			continue
		}
		m := t[pair[0]:pair[1]]
		num := strings.TrimSpace(t[pair[2]:pair[3]])
		st := strings.TrimSpace(t[pair[4]:pair[5]])
		if !strings.Contains(strings.ToUpper(transcript), st) {
			if f := strings.Fields(st); len(f) >= 2 && suffixlessSingleStreetPlausible(f[0]) {
				st = f[0]
			}
		}
		if dispatchHouseFromRoomPhoneExtension(num, t) {
			continue
		}
		if dispatchHouseFromLotOrUnitLabel(num, t) {
			continue
		}
		if dispatchSuffixlessStreetIsNonLocationPhrase(st) {
			continue
		}
		if num == "" || !suffixlessStreetNamePlausible(st) {
			continue
		}
		st = dispatchStreetCutAtConnector(st)
		if st == "" || !suffixlessStreetNamePlausible(st) {
			continue
		}
		if dispatchStreetIsApparatusStatus(st) {
			continue
		}
		if suffixlessStreetIsJurisdictionRoster(st, t) {
			continue
		}
		if stationHouseNumberLooksLikeUnit(num, t) {
			continue
		}
		if houseNumberFromPriorityCode(num, t) {
			continue
		}
		if houseNumberFromAge(num, t) {
			continue
		}
		if houseNumberFromUnitWillBeStatus(num, t) {
			continue
		}
		if suffixlessMatchFollowedBySuffix(t, m) {
			continue
		}
		q := strings.TrimSpace(num + " " + st)
		if AddressIsConversationalNoise(q) || dispatchSuffixlessStreetIsNonLocationPhrase(st) {
			continue
		}
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	return out
}

// maybeExtendCompoundThoroughfareSuffix recovers "SOUTH PARKWAY DRIVE" when the
// dispatch-address regex latched onto PARKWAY as the thoroughfare suffix and
// the next spoken word is another street type (DRIVE/ROAD/STREET/…).
func maybeExtendCompoundThoroughfareSuffix(house, street, transcript string) string {
	house = strings.ToUpper(strings.TrimSpace(house))
	street = strings.ToUpper(strings.TrimSpace(street))
	if house == "" || street == "" {
		return street
	}
	fields := strings.Fields(street)
	last := fields[len(fields)-1]
	switch last {
	case "PARKWAY", "PKWY", "WAY", "CIRCLE", "CIR", "TRAIL", "TRL", "POINT", "PT", "PLACE", "PL", "RUN":
	default:
		return street
	}
	needle := house + " " + street
	idx := strings.Index(transcript, needle)
	if idx < 0 {
		return street
	}
	rest := strings.TrimSpace(transcript[idx+len(needle):])
	toks := strings.Fields(rest)
	if len(toks) == 0 {
		return street
	}
	next := strings.TrimRight(toks[0], ".,;:?!")
	if !localStreetSuffixes[next] || next == last {
		return street
	}
	return street + " " + next
}

// suffixlessSingleStreetIsAgencyCityPrefix reports when a single-word suffixless
// capture ("221 WARREN") is really "221, WARREN CITY FIRE…" agency roster.
func suffixlessSingleStreetIsAgencyCityPrefix(t, fullMatch, streetToken string) bool {
	streetToken = strings.ToUpper(strings.TrimSpace(streetToken))
	if streetToken == "" {
		return false
	}
	idx := strings.Index(t, strings.ToUpper(fullMatch))
	if idx < 0 {
		return false
	}
	after := strings.TrimSpace(t[idx+len(fullMatch):])
	return strings.HasPrefix(after, "CITY") &&
		(strings.Contains(t, streetToken+" CITY FIRE") ||
			strings.Contains(t, streetToken+" CITY POLICE") ||
			strings.Contains(t, streetToken+" CITY EMS"))
}

// preferDispatchAddress picks the best candidate when the transcript names the
// same street more than once (e.g. "3082 AT HOWLAND WILSON … 82 HOWLAND WILSON").
func preferDispatchAddress(addrs []string, transcript string, knownStreets []string) string {
	if len(addrs) == 0 {
		return ""
	}
	filtered := make([]string, 0, len(addrs))
	for _, a := range addrs {
		_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(a)))
		if dispatchStreetIsSpelledLetters(street) {
			continue
		}
		if AddressIsConversationalNoise(a) || dispatchSuffixlessStreetIsNonLocationPhrase(street) {
			continue
		}
		// Drop highway route numbers glued to directionals/nature words
		// ("88 NORTHEAST GAS" from "STATE ROUTE 88 NORTHEAST, GAS STOVE…")
		// so a real house address earlier in the same tone wins.
		if AddressHouseNumberIsRouteFragment(a, transcript) {
			continue
		}
		if dispatchAddressIsDigitInsertionDuplicate(a, transcript) {
			continue
		}
		if dispatchAddressIsTruncatedHouseOfLongerDispatch(a, transcript, nil) {
			continue
		}
		filtered = append(filtered, a)
	}
	if len(filtered) == 0 {
		return ""
	}
	addrs = filtered
	if len(addrs) == 1 {
		return addrs[0]
	}
	best := addrs[0]
	bestScore := scoreDispatchAddress(addrs[0], transcript, knownStreets)
	for _, a := range addrs[1:] {
		if s := scoreDispatchAddress(a, transcript, knownStreets); s > bestScore {
			bestScore = s
			best = a
		}
	}
	return inheritSameHouseThoroughfareSuffix(best, addrs)
}

// inheritSameHouseThoroughfareSuffix copies ROAD/AVE/… from another same-house
// candidate when the preferred correction is suffixless ("1050 SOUTH GREEN"
// after "1050 SCREEN ROAD").
func inheritSameHouseThoroughfareSuffix(best string, addrs []string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(best)))
	if house == "" || street == "" || hasStreetSuffix(street) {
		return best
	}
	var suffix string
	for _, a := range addrs {
		h, st := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(a)))
		if h != house || st == "" || !hasStreetSuffix(st) {
			continue
		}
		fields := strings.Fields(st)
		suf := fields[len(fields)-1]
		if localStreetSuffixes[suf] {
			suffix = suf
			break
		}
	}
	if suffix == "" {
		return best
	}
	return house + " " + street + " " + suffix
}

// spokenStreetContinuesAfterMatch reports when dispatch spoke more of the street
// name immediately after a prefix capture ("2855 TIMBER" before "CREEK NORTH").
func spokenStreetContinuesAfterMatch(u, house, street string) bool {
	needle := house + " " + street
	idx := strings.Index(u, needle)
	if idx < 0 {
		return false
	}
	rest := strings.TrimLeft(u[idx+len(needle):], " ")
	if rest == "" {
		return false
	}
	for _, sep := range []string{"BETWEEN ", "CROSS ", "CROSSES ", "NEAR ", "AT ", "FOR "} {
		if strings.HasPrefix(rest, sep) {
			return false
		}
	}
	tok := strings.TrimRight(strings.Fields(rest)[0], ".,;:")
	return len(tok) >= 2 && !suffixlessStreetStopwords[tok]
}

func transcriptContainsDispatchHouseStreet(u, house, street string) bool {
	padded := " " + u + " "
	house = strings.ToUpper(strings.TrimSpace(house))
	street = strings.ToUpper(strings.TrimSpace(street))
	if house == "" || street == "" {
		return false
	}
	for _, tail := range []string{" ", ",", ".", ";"} {
		if strings.Contains(padded, " "+house+" "+street+tail) {
			return true
		}
	}
	return false
}

func scoreDispatchAddress(addr, transcript string, knownStreets []string) int {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return 0
	}
	u := strings.ToUpper(transcript)
	score := 0
	if transcriptContainsDispatchHouseStreet(u, house, street) {
		score += 100
		if spokenStreetContinuesAfterMatch(u, house, street) &&
			!collapsedSpokenCompletesKnownStem(u, house, knownStreets) {
			score -= 250
		}
	} else if strings.Contains(u, house+" "+street) {
		// Substring-only hit (e.g. house 9 inside 19) — weak signal.
		score += 20
	}
	score += len(strings.Fields(street)) * 5
	score += len(house) * 2
	firstStreet := strings.Fields(street)
	if len(firstStreet) > 0 && strings.Contains(u, house+" AT "+firstStreet[0]) {
		score -= 50
	}
	if idx := strings.LastIndex(u, house+" "); idx >= 0 {
		score += idx / 10
	}
	if strings.Contains(street, " BETWEEN ") || strings.Contains(street, " AND ") {
		score -= 200
	}
	if strings.Contains(street, " CROSS ") || strings.HasSuffix(street, " CROSS") {
		score -= 300
	}
	if transcriptHouseStateRouteRE.MatchString(addr) {
		score += 500
	}
	return score
}

// suffixlessMatchFollowedByRouteNumber reports when a suffixless capture ends at
// "STATE" immediately before "ROUTE 88" ("3613 STATE ROUTE 88"), or when the
// capture ends at "ROUTE" immediately before the route number ("8768 ROUTE 7").
func suffixlessMatchFollowedByRouteNumber(t, fullMatch string) bool {
	idx := strings.Index(t, fullMatch)
	if idx < 0 {
		return false
	}
	rest := strings.TrimSpace(t[idx+len(fullMatch):])
	fields := strings.Fields(rest)
	if len(fields) >= 2 && (fields[0] == "ROUTE" || fields[0] == "RTE") && isAllDigits(fields[1]) {
		return true
	}
	stem := strings.Fields(strings.TrimSpace(fullMatch))
	if len(stem) >= 2 {
		last := stem[len(stem)-1]
		if last == "ROUTE" || last == "RTE" || last == "RT" || last == "HWY" || last == "HIGHWAY" {
			if len(fields) > 0 && isAllDigits(fields[0]) && isShortRouteNumber(fields[0]) {
				return true
			}
		}
	}
	return false
}

// suffixlessMatchFollowedBySuffix reports when a suffixless capture is the
// prefix of a longer street ("4518 EAST" before "ROYALTY ROAD", or before "ROAD").
func suffixlessMatchFollowedBySuffix(t, fullMatch string) bool {
	idx := strings.Index(t, fullMatch)
	if idx < 0 {
		return false
	}
	rest := strings.TrimSpace(t[idx+len(fullMatch):])
	if rest == "" {
		return false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return false
	}
	if localStreetSuffixes[fields[0]] {
		return true
	}
	for i := 0; i < len(fields) && i < 5; i++ {
		if localStreetSuffixes[fields[i]] {
			return i > 0
		}
	}
	return false
}

// transcriptHyphenPairBeforeSuffixlessRE matches "1851-7525 WARREN SHARON"
// where hyphenated digit pairs are STT station/address glue, not one house number.
var transcriptHyphenPairBeforeSuffixlessRE = regexp.MustCompile(`(?i)\b(\d{3,5})-(\d{3,5})\s+([A-Z][A-Z'\-]{2,}\s+[A-Z][A-Z'\-]{2,})\b`)

// suffixlessStreetStopwords are tokens that must not appear in a captured
// suffixless street name ("82 AT HOWLAND" is rejected by the regex shape).
var suffixlessStreetStopwords = map[string]bool{
	"AT": true, "THE": true, "FOR": true, "AND": true, "WITH": true, "FROM": true,
	"RED": true, "SUV": true, "FIRE": true, "VEHICLE": true, "STATION": true,
	"ENGINE": true, "MEDIC": true, "SQUAD": true, "ALS": true, "PD": true,
	"YEAR": true, "YEARS": true, "MALE": true, "FEMALE": true, "OLD": true,
	"LOBBY": true, "HALL": true, "BUILDING": true, "ENTRANCE": true,
	"MORE": true, "THAN": true, "TIMES": true, "MINUTE": true, "MINUTES": true,
	// Narrative time/duration units ("35 WEEKS PREGNANT", "3 DAYS AGO") — never
	// a street name, but the bare 2-word suffixless pattern otherwise matches
	// "<number> <word> <word>" indiscriminately.
	"WEEK": true, "WEEKS": true, "MONTH": true, "MONTHS": true,
	"DAY": true, "DAYS": true, "HOUR": true, "HOURS": true, "PREGNANT": true,
	"BLOOD": true, "PRESSURE": true, "HEART": true, "RATE": true,
	"BETWEEN": true, "NEAR": true, "TAKES": true, "THANKS": true,
	"THERE'S": true, "THERES": true, "THERE": true, "POSSIBLY": true, "PROBABLY": true,
	// Pronoun / hedging tails after a bare stem ("1354 ANDREWS IT SOUNDS LIKE…")
	// must stop the spoken-street phrase — otherwise the card becomes
	// "ANDREWS IT SOUNDS" and ClearPinWhenStreetContradictsTranscript drops a
	// good Nominatim pin for "ANDREWS AVENUE".
	"IT": true, "ITS": true, "IT'S": true, "SOUNDS": true, "SOUND": true,
	"IT'LL": true, "ITLL": true, "BE": true,
	// Municipality/agency tokens ("WARREN CITY FIRE DEPARTMENT" → not "221 WARREN CITY").
	"CITY": true, "COUNTY": true, "TOWNSHIP": true, "DEPARTMENT": true,
	"GOING": true, "WE'RE": true, "WERE": true, "WELL": true,
	// "22 MUTOID REQUEST" (STT-garbled "MUTUAL AID REQUEST") — no street is
	// ever named "___ Request"; this word is always dispatch-radio phrasing
	// (mutual aid / assistance / backup request), never a thoroughfare.
	"REQUEST": true,
	// Unit coordination / narrative verbs ("MEET 110 BACK AT THE JAIL",
	// "110 TAKE A LOOK") — never thoroughfare names.
	"BACK": true, "TAKE": true, "LOOK": true, "MEET": true, "JAIL": true,
	"DONE": true, "ONCE": true, "YEAH": true,
	// Person/vehicle description fragments ("150 BURNT GREEN", "2436 SENATOR DAVID").
	"BURNT": true, "SENATOR": true,
	// Schedule / admin chatter ("16 HAVE OFF A MONTH").
	"HAVE": true, "OFF": true,
	// Narrative after a house+street ("4908 CENTRAL LOOKS LIKE …") — never
	// thoroughfare tokens; also stops collapsedStreetWordsAfterHouse so
	// AddressIsTruncatedStreetVersusTranscript does not treat CENTRAL as truncated.
	"LOOKS": true, "LIKE": true, "WE": true, "SOMEONE": true, "SOMEBODY": true,
	// Time/adverb tails after a street ("4908 CENTRAL RIGHT NOW").
	"RIGHT": true, "NOW": true, "THAT": true,
	// Caller/narrative STT ("LOT 14. COLLAR ADVISED THAT…" — COLLAR←CALLER).
	"CALLER": true, "COLLAR": true, "ADVISED": true, "ADVISES": true, "LAWYER": true,
	// Unit radio procedure ("50 CAN YOU START HEADING OVER…") — never a street.
	"CAN": true, "YOU": true, "START": true, "HEADING": true,
	// Clause openers after a house+street ("9705 LORETTA. WE'VE GOT A…").
	"WE'VE": true, "WEVE": true, "THEY'VE": true, "THEYVE": true,
	"I'VE": true, "IVE": true, "SHE'S": true, "SHES": true, "HE'S": true, "HES": true,
	"GOT": true, "GET": true, "GETTING": true,
	// CAD status codes ("019 PRIORITY 2") — never thoroughfare names.
	"PRIORITY": true, "PRIORITIES": true,
}

// streetNameIsLoneDirectional reports when the street is nothing but a
// directional word ("WEST", "NORTHEAST", "N"). A lone directional is never a
// street — it is the truncated head of a numbered grid street ("WEST 52ND") or
// dispatch narration, and must not be emitted or scored as an address.
func streetNameIsLoneDirectional(st string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(st)))
	if len(fields) != 1 {
		return false
	}
	switch fields[0] {
	case "NORTH", "SOUTH", "EAST", "WEST",
		"NORTHEAST", "NORTHWEST", "SOUTHEAST", "SOUTHWEST":
		return true
	}
	return streetDirTokens[canonicalStreetTokens(fields[0])]
}

func suffixlessSingleStreetPlausible(st string) bool {
	st = strings.ToUpper(strings.TrimSpace(st))
	if TokenIsRadioCommsNoise(st) {
		return false
	}
	if streetNameIsLoneDirectional(st) {
		return false
	}
	if len(st) < 3 || suffixlessStreetStopwords[st] || screenNumberWordNoise[st] {
		return false
	}
	if localStreetSuffixes[st] && st != "POINT" && st != "PT" {
		return false
	}
	return !transcriptContainsNoise(st)
}

func suffixlessStreetNamePlausible(st string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(st)))
	if len(fields) != 2 {
		return false
	}
	for _, f := range fields {
		if len(f) < 3 || suffixlessStreetStopwords[f] || screenNumberWordNoise[f] {
			return false
		}
		if localStreetSuffixes[f] {
			// "EAGLE POINT" — POINT is part of the street name, not a thoroughfare type.
			if f == "POINT" || f == "PT" {
				continue
			}
			return false
		}
	}
	return !transcriptContainsNoise(st)
}

// isIncompleteRouteSide reports a route keyword without its number (e.g. "SR"
// from a regex that stopped at the word boundary before "534").
func isIncompleteRouteSide(side string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(side)))
	return len(fields) == 1 && localRouteKeywords[fields[0]]
}

// extendIncompleteIntersectionSide recovers route numbers or street suffixes when
// the intersection regex stopped at a word boundary (e.g. "SR" before "534",
// "ELM" before "ST").
func extendIncompleteIntersectionSide(side, following string) string {
	u := strings.ToUpper(strings.TrimSpace(side))
	if u == "" {
		return side
	}
	rest := strings.Fields(strings.TrimSpace(strings.ToUpper(following)))
	if len(rest) == 0 {
		return side
	}
	fields := strings.Fields(u)

	// Bare route keyword: "SR" + "534" → "SR 534"
	if len(fields) == 1 && localRouteKeywords[fields[0]] {
		if isAllDigits(rest[0]) && isShortRouteNumber(rest[0]) {
			return fields[0] + " " + rest[0]
		}
		return side
	}

	// Street name missing suffix: "ELM" + "ST" → "ELM ST"
	last := fields[len(fields)-1]
	if !localStreetSuffixes[last] && localStreetSuffixes[rest[0]] {
		return u + " " + rest[0]
	}
	return side
}

// extractDispatchIntersectionsFromTranscript finds intersection phrases like
// "534 AND 88" or "DEFOREST AND NILES" with no house number. Universal — uses
// pattern only, plus optional knownStreets to qualify route/road tokens.
func extractDispatchIntersectionsFromTranscript(transcript string, knownStreets []string) []string {
	t := strings.ToUpper(collapseMultiHyphenSpelledDigits(transcript))
	t = normalizeRouteTokens(t)
	if !strings.Contains(t, " AND ") && !strings.Contains(t, " & ") && !strings.Contains(t, " / ") {
		return nil
	}
	known := map[string]bool{}
	for _, s := range knownStreets {
		known[strings.ToUpper(strings.TrimSpace(s))] = true
	}
	expandBareRoute := func(x string) string {
		u := strings.ToUpper(strings.TrimSpace(x))
		// Only 1–3 digit bare numbers are treated as routes. 4+ digit numbers in
		// dispatch are almost always unit/vehicle numbers or addresses (e.g.
		// "VEHICLE 8629"), not state routes, so we leave them unexpanded to avoid
		// fabricating an intersection like "SR 8629 & ...".
		if len(u) >= 1 && len(u) <= 3 && isAllDigits(u) {
			return "SR " + u
		}
		return u
	}
	isRouteish := func(x string) bool {
		u := strings.ToUpper(x)
		if known[u] {
			return true
		}
		if strings.HasPrefix(u, "SR ") || strings.HasPrefix(u, "CR ") || strings.HasPrefix(u, "US ") || strings.HasPrefix(u, "HWY ") {
			return true
		}
		for _, w := range []string{"RD", "ROAD", "ST", "STREET", "AVE", "AVENUE", "DR", "DRIVE", "BLVD", "PL", "PLACE", "WAY", "TRL", "TRAIL", "HWY", "HIGHWAY", "LN", "LANE"} {
			if u == w || strings.HasSuffix(u, " "+w) {
				return true
			}
		}
		return false
	}
	// Trim noise prefix tokens off candidate sides so "FOR AN MVA 534" becomes "534".
	trimNoisePrefix := func(side string) string {
		fields := strings.Fields(side)
		drop := 0
		noisy := map[string]bool{"FOR": true, "AN": true, "A": true, "ON": true, "OF": true, "AT": true, "IN": true,
			"MVA": true, "WITH": true, "POSSIBLE": true, "POSSIBLY": true, "STATION": true,
			"ENGINE": true, "MEDIC": true, "SQUAD": true}
		for drop < len(fields) && noisy[fields[drop]] {
			drop++
		}
		if drop > 0 {
			fields = fields[drop:]
		}
		return strings.Join(fields, " ")
	}
	var out []string
	seen := map[string]bool{}
	for _, loc := range transcriptIntersectionRE.FindAllStringSubmatchIndex(t, -1) {
		if len(loc) < 6 {
			continue
		}
		aRaw := strings.TrimSpace(t[loc[2]:loc[3]])
		// "STATION 40 AND 41" / "STATIONS 45, 22 AND 21" are fire-apparatus
		// roll calls naming station numbers, never a road intersection — a
		// bare number labeled STATION is a station number no matter how
		// route-like it looks once the label is trimmed away below, and
		// trimNoisePrefix's own STATION removal would otherwise let
		// expandBareRoute fabricate "SR 40 & SR 41" out of it (alert on call
		// 284904 — "STATION 40 AND 41 FOR A TREE DOWN WITH POWER LINES").
		if stationApparatusPrefixRE.MatchString(aRaw) || stationApparatusRollCallTailRE.MatchString(t[:loc[2]]) {
			continue
		}
		a := trimNoisePrefix(aRaw)
		b := strings.TrimSpace(t[loc[4]:loc[5]])
		a = stripCrossStreetCaptureNoise(a)
		b = stripCrossStreetCaptureNoise(b)
		if loc[1] < len(t) {
			b = extendIncompleteIntersectionSide(b, t[loc[1]:])
		}
		if a == "" || b == "" || len(a) < 2 || len(b) < 2 {
			continue
		}
		if isIncompleteRouteSide(a) || isIncompleteRouteSide(b) {
			continue
		}
		if len(strings.Fields(a)) > 4 || len(strings.Fields(b)) > 4 {
			continue
		}
		if transcriptContainsNoise(a) || transcriptContainsNoise(b) {
			continue
		}
		aE := expandBareRoute(a)
		bE := expandBareRoute(b)
		if IntersectionSideIsNonStreet(aE) || IntersectionSideIsNonStreet(bE) {
			continue
		}
		if !isRouteish(aE) && !isRouteish(bE) {
			continue
		}
		// A fully bare "<digits> AND <digits>" pair — neither side an explicit
		// ROUTE/SR marker nor a known gazetteer street, just two short numbers
		// — is at least as likely to be unit/station numbers whose "STATION"
		// label STT dropped ("22 AND 45 FOR A FIRE ALARM" ← "STATION 22,
		// STATION 45 FOR A FIRE ALARM 534") as a genuine route intersection.
		// The STATION-prefix check above only catches the label when STT kept
		// it; when STT drops it entirely, the only remaining signal that
		// distinguishes a real bare-route intersection is an explicit
		// location cue before the pair ("RESPOND TO 534 AND 88 FOR AN MVA" —
		// see TestRealRouteIntersectionStillDetected).
		if isAllDigits(a) && isAllDigits(b) && !known[aE] && !known[bE] &&
			!bareIntersectionHasDispatchCueBeforePair(t, a, b) {
			continue
		}
		q := aE + " AND " + bE
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	return out
}

// extractKnownStreetAddressesFromTranscript scans the transcript for
// "<house number> <known street>" pairs using the group's known streets, even
// when the street has no thoroughfare suffix (e.g. GP EASTERLY).

// deriveStreetAbbreviations returns dispatch-shorthand aliases for a multi-word
// street name. For "GEAUGA PORTAGE EASTERLY RD NW" it returns variants like
// "GP EASTERLY" (initials of the first two words + the rest) and progressively
// shorter forms. Universal — uses word structure, not per-department data.
func deriveStreetAbbreviations(name string) []string {
	u := strings.ToUpper(strings.TrimSpace(name))
	if u == "" {
		return nil
	}
	fields := strings.Fields(u)
	if len(fields) < 3 {
		return nil
	}
	// Drop trailing thoroughfare/directional words to find descriptive words.
	suffixWords := map[string]bool{
		"RD": true, "ROAD": true, "ST": true, "STREET": true, "AVE": true, "AVENUE": true,
		"DR": true, "DRIVE": true, "LN": true, "LANE": true, "BLVD": true, "BOULEVARD": true,
		"WAY": true, "CT": true, "COURT": true, "PL": true, "PLACE": true, "TRL": true, "TRAIL": true,
		"HWY": true, "HIGHWAY": true, "PKWY": true, "PARKWAY": true, "TERR": true,
		"N": true, "S": true, "E": true, "W": true, "NE": true, "NW": true, "SE": true, "SW": true,
	}
	descriptive := []string{}
	for _, f := range fields {
		if suffixWords[f] {
			break
		}
		descriptive = append(descriptive, f)
	}
	if len(descriptive) < 3 {
		return nil
	}
	// Build "<initials of first N words> <remaining words>" for N=2..len-1.
	var out []string
	seen := map[string]bool{}
	for n := 2; n < len(descriptive); n++ {
		initials := ""
		for i := 0; i < n; i++ {
			initials += string(descriptive[i][0])
		}
		alias := initials + " " + strings.Join(descriptive[n:], " ")
		if !seen[alias] {
			seen[alias] = true
			out = append(out, alias)
		}
	}
	return out
}

func extractKnownStreetAddressesFromTranscript(transcript string, knownStreets []string) []string {
	if len(knownStreets) == 0 {
		return nil
	}
	t := strings.ToUpper(normalizeDispatchAddressSeparators(
		collapseMultiHyphenSpelledDigits(expandHyphenHouseNumbersInDispatchTranscript(transcript))))
	var out []string
	seen := map[string]bool{}
	scan := func(needle string) {
		if len(needle) < 4 {
			return
		}
		idx := 0
		for idx < len(t) {
			i := strings.Index(t[idx:], needle)
			if i < 0 {
				break
			}
			pos := idx + i
			if pos > 0 && t[pos-1] != ' ' {
				idx = pos + 1
				continue
			}
			// Require a word boundary on the right too — otherwise a bare stem
			// like "STATION" (from gazetteer entries "STATION ROAD"/"STATION
			// STREET") matches as a prefix of the unrelated word "STATIONS" in
			// "STATIONS 40 AND 42" (fire station roll call), fabricating a bogus
			// "912 STATION" address out of the "9-1-2" radio preamble digits.
			if end := pos + len(needle); end < len(t) && t[end] >= 'A' && t[end] <= 'Z' {
				idx = pos + 1
				continue
			}
			// Walk backwards over whitespace then collect digits as house number.
			look := pos
			for look > 0 && t[look-1] == ' ' {
				look--
			}
			house := ""
			for look > 0 && t[look-1] >= '0' && t[look-1] <= '9' {
				look--
				house = string(t[look]) + house
			}
			// Route/apparatus hyphen blobs ("434-88, PORTER") — trailing digits
			// before a known street are not house numbers.
			if look > 0 && t[look-1] == '-' {
				idx = pos + len(needle)
				continue
			}
			if len(house) >= 1 && len(house) <= 6 {
				gapStart := look + len(house)
				if gapStart < pos {
					gap := strings.TrimSpace(t[gapStart:pos])
					if gap != "" && !strings.HasPrefix(strings.TrimSpace(needle), gap) {
						idx = pos + len(needle)
						continue
					}
				}
				// "256 ROSE" inside "256 ROSE GARDEN" is a prefix capture — keep the
				// full spoken compound for dispatch extraction instead.
				if len(strings.Fields(needle)) == 1 && spokenStreetContinuesAfterMatch(t, house, needle) {
					idx = pos + len(needle)
					continue
				}
				if knownStreetHarvestIsUnitRadioSignOff(house, needle, t) {
					idx = pos + len(needle)
					continue
				}
				q := house + " " + needle
				if !seen[q] {
					seen[q] = true
					out = append(out, q)
				}
			}
			idx = pos + len(needle)
		}
	}
	for _, name := range knownStreets {
		st := strings.ToUpper(strings.TrimSpace(name))
		scan(st)
		// Dispatch often drops the suffix ("2024 ROBBINS" for Robbins Ave).
		if nameStem, _ := streetNameAndSuffix(st); nameStem != "" && nameStem != st {
			scan(nameStem)
		}
		for _, alias := range deriveStreetAbbreviations(st) {
			scan(alias)
		}
	}
	scanCollapsedKnownStreetAddresses(t, knownStreets, seen, &out)
	return out
}

// collapsedStreetWordsAfterHouse returns the dispatch street phrase after a house
// number with spaces removed ("AUSTIN TOWN WARREN" → "AUSTINTOWNWARREN").
func collapsedStreetWordsAfterHouse(transcript, house string) string {
	u := strings.ToUpper(strings.TrimSpace(transcript))
	house = strings.ToUpper(strings.TrimSpace(house))
	idx := strings.Index(u, house+" ")
	if idx < 0 {
		idx = strings.Index(u, house+",")
	}
	if idx < 0 {
		return ""
	}
	after := strings.TrimSpace(u[idx+len(house):])
	var flat []string
	for _, tok := range strings.Fields(after) {
		tok = strings.TrimRight(tok, ".,;:")
		flat = append(flat, strings.Fields(strings.ReplaceAll(tok, "-", " "))...)
	}
	var words []string
	for i := 0; i < len(flat); i++ {
		part := flat[i]
		if len(words) == 1 {
			only := words[0]
			if len(only) >= 8 && !localStreetSuffixes[only] && !localRouteKeywords[only] && !isStreetQuadrantSuffix(only) {
				return stripStreetSpaces(only)
			}
		}
		if part == house {
			if len(words) == 0 {
				continue // STT often repeats the house ("9212 9212 KINGSGRAVE")
			}
			return stripStreetSpaces(strings.Join(words, " "))
		}
		if len(part) < 2 || suffixlessStreetStopwords[part] {
			return stripStreetSpaces(strings.Join(words, " "))
		}
		switch part {
		case "BETWEEN", "CROSS", "CROSSES", "CROSSUP", "FOR", "NEAR", "AT", "AND":
			return stripStreetSpaces(strings.Join(words, " "))
		}
		// "9715 CLINTON OVER AT TRIAD…" — OVER AT is not street continuation.
		if part == "OVER" && i+1 < len(flat) {
			next := strings.TrimRight(flat[i+1], ".,;:?!")
			switch next {
			case "AT", "BY", "NEAR", "THE", "ON", "IN", "TO":
				return stripStreetSpaces(strings.Join(words, " "))
			}
		}
		words = append(words, part)
		if len(words) >= 5 {
			break
		}
	}
	return stripStreetSpaces(strings.Join(words, " "))
}

func collapsedSpokenCompletesKnownStem(transcript, house string, knownStreets []string) bool {
	spokenNS := collapsedStreetWordsAfterHouse(transcript, house)
	if len(spokenNS) < 10 {
		return false
	}
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		stem, sfx := streetNameAndSuffix(ku)
		if sfx == "" || stem == "" {
			continue
		}
		if stripStreetSpaces(stem) == spokenNS {
			return true
		}
	}
	return false
}

// scanCollapsedKnownStreetAddresses matches gazetteer stems when STT splits a
// compound street name across tokens ("AUSTIN TOWN WARREN" for Austintown Warren Rd).
func scanCollapsedKnownStreetAddresses(t string, knownStreets []string, seen map[string]bool, out *[]string) {
	if len(knownStreets) == 0 {
		return
	}
	type stemHit struct {
		stem string
		ns   string
	}
	stemSeen := map[string]bool{}
	var stems []stemHit
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		stem, sfx := streetNameAndSuffix(ku)
		if sfx == "" || stem == "" {
			continue
		}
		ns := stripStreetSpaces(stem)
		if len(ns) < 10 || stemSeen[ns] {
			continue
		}
		stemSeen[ns] = true
		stems = append(stems, stemHit{stem: stem, ns: ns})
	}
	for _, m := range transcriptHouseStreetLeadRE.FindAllStringSubmatch(t, -1) {
		if len(m) < 2 {
			continue
		}
		house := strings.TrimSpace(m[1])
		if house == "" || stationHouseNumberLooksLikeUnit(house, t) {
			continue
		}
		spokenNS := collapsedStreetWordsAfterHouse(t, house)
		if len(spokenNS) < 10 {
			continue
		}
		for _, hit := range stems {
			if spokenNS != hit.ns {
				continue
			}
			q := house + " " + hit.stem
			if !seen[q] {
				seen[q] = true
				*out = append(*out, q)
			}
		}
	}
}

// pickBestKnownStreetAddress chooses the known-street match that best aligns
// with the transcript when multiple gazetteer streets matched the same house number.
func pickBestKnownStreetAddress(matches []string, transcript string) string {
	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return matches[0]
	}
	u := strings.ToUpper(transcript)
	best := matches[0]
	bestScore := scoreKnownStreetAddressMatch(best, u)
	for _, m := range matches[1:] {
		if s := scoreKnownStreetAddressMatch(m, u); s > bestScore {
			bestScore = s
			best = m
		}
	}
	return best
}

func scoreKnownStreetAddressMatch(addr, transcript string) int {
	house, st := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if st == "" {
		return 0
	}
	score := len(st) * 10
	if transcriptContainsDispatchHouseStreet(transcript, house, st) {
		score += 2000
	}
	if dispatchAddressIsFeetMangledDuplicate(addr, transcript) {
		score -= 5000
	}
	score += len(house) * 50
	if strings.Contains(transcript, st) {
		score += 1000
	}
	_, suffix := streetNameAndSuffix(st)
	switch suffix {
	case "AVE", "AVENUE":
		if strings.Contains(transcript, "AVENUE") || strings.Contains(transcript, " AVE") {
			score += 500
		}
	case "ST", "STREET":
		if strings.Contains(transcript, "STREET") || strings.Contains(transcript, " ST") {
			score += 500
		}
	}
	if StreetHasOrdinalCore(st) {
		for _, spoken := range ordinalStreetNameVariants(st) {
			if strings.Contains(transcript, spoken) {
				score += 300
			}
		}
	}
	return score
}

func geocodeBiasCenterMi(geo *GeoOptions) (lat, lon, radiusMi float64, ok bool) {
	if geo == nil || geo.BoundsLat == 0 || geo.BoundsRadiusMi <= 0 {
		return 0, 0, 0, false
	}
	return geo.BoundsLat, geo.BoundsLon, geo.BoundsRadiusMi, true
}

func forwardGeocodeOutsideBias(lat, lon, biasLat, biasLon, radiusMi float64) bool {
	if biasLat == 0 || radiusMi <= 0 {
		return false
	}
	return haversineMeters(biasLat, biasLon, lat, lon) > radiusMi*1609.34
}

// forwardGeocodeWithFallback geocodes the alert address with multiple candidate
// isIntersectionQuery reports whether a candidate string is "X AND Y" / "X & Y" / "X / Y".
func isIntersectionQuery(q string) bool {
	u := strings.ToUpper(q)
	if strings.Contains(u, " AND ") || strings.Contains(u, " & ") || strings.Contains(u, " / ") {
		// Reject "5 AND 6" style if first or last char is a digit-only token w/o road word.
		_, st := splitHouseAndStreet(strings.TrimSpace(u))
		if st != "" && strings.HasPrefix(strings.TrimSpace(u), "") {
			// Address with house number that mentions "AND" later (e.g. between X AND Y) — not an intersection.
			fields := strings.Fields(u)
			if len(fields) > 0 && isAllDigits(fields[0]) {
				return false
			}
		}
		return true
	}
	return false
}

// splitIntersectionQuery parses "A AND B" / "A & B" / "A / B" into two streets.
func splitIntersectionQuery(q string) (string, string) {
	u := trimDispatchAreaPrefix(q)
	for _, sep := range []string{" AND ", " & ", " / "} {
		if i := strings.Index(u, sep); i > 0 {
			return strings.TrimSpace(u[:i]), strings.TrimSpace(u[i+len(sep):])
		}
	}
	return "", ""
}

// TrimDispatchAreaPrefix strips lead-ins like "AREA OF" from dispatch addresses.
func TrimDispatchAreaPrefix(addr string) string {
	return trimDispatchAreaPrefix(addr)
}

// ParseIntersectionQuery parses "A AND B" / "A & B" / "A / B" into two streets.
func ParseIntersectionQuery(q string) (string, string) {
	return splitIntersectionQuery(q)
}

// trimDispatchAreaPrefix strips non-address lead-ins so "AREA OF X & Y" geocodes
// as intersection X & Y.
func trimDispatchAreaPrefix(addr string) string {
	u := strings.ToUpper(strings.TrimSpace(addr))
	for {
		changed := false
		for _, p := range []string{"AREA OF ", "VICINITY OF ", "IN THE AREA OF ", "IN THE VICINITY OF "} {
			if strings.HasPrefix(u, p) {
				u = strings.TrimSpace(u[len(p):])
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return u
}

