// Copyright (C) 2025 Thinline Dynamic Solutions
//
// ordinal_street.go — guards for numbered ordinal streets (5TH AVE, 3RD ST)
// where AVE↔ST suffix swaps and homonym snaps cause wrong pins.

package mapping

import (
	"regexp"
	"strings"
)

var ordinalDigitTokenRE = regexp.MustCompile(`^\d+(ST|ND|RD|TH)$`)

var ordinalWordToDigit = map[string]string{
	"FIRST": "1ST", "SECOND": "2ND", "THIRD": "3RD", "FOURTH": "4TH", "FIFTH": "5TH",
	"SIXTH": "6TH", "SEVENTH": "7TH", "EIGHTH": "8TH", "NINTH": "9TH", "TENTH": "10TH",
}

var ordinalDigitToWord = map[string]string{
	"1ST": "FIRST", "2ND": "SECOND", "3RD": "THIRD", "4TH": "FOURTH", "5TH": "FIFTH",
	"6TH": "SIXTH", "7TH": "SEVENTH", "8TH": "EIGHTH", "9TH": "NINTH", "10TH": "TENTH",
}

// expandGridDirectionalWord spells out a single-letter grid directional so an
// extracted numbered street matches the gazetteer form ("W" → "WEST 52ND ...").
func expandGridDirectionalWord(d string) string {
	switch strings.ToUpper(strings.TrimSpace(d)) {
	case "E":
		return "EAST"
	case "W":
		return "WEST"
	case "N":
		return "NORTH"
	case "S":
		return "SOUTH"
	}
	return strings.ToUpper(strings.TrimSpace(d))
}

// gridOrdinalWithSuffix normalizes a spoken grid number to its ordinal form so
// "52" becomes "52ND" (and an already-suffixed "52ND" is left unchanged),
// matching how numbered grid streets are stored in the gazetteer.
func gridOrdinalWithSuffix(tok string) string {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	if tok == "" || ordinalDigitTokenRE.MatchString(tok) {
		return tok
	}
	n := tok
	if len(n) >= 2 {
		switch n[len(n)-2:] {
		case "11", "12", "13":
			return n + "TH"
		}
	}
	switch n[len(n)-1:] {
	case "1":
		return n + "ST"
	case "2":
		return n + "ND"
	case "3":
		return n + "RD"
	default:
		return n + "TH"
	}
}

// gridStreetTypeWord returns the canonical thoroughfare type for a grid street,
// preserving a spoken type and defaulting to STREET (numbered directional grid
// streets are overwhelmingly "STREET" when the type is dropped in dispatch).
func gridStreetTypeWord(suf string) string {
	switch strings.ToUpper(strings.TrimSpace(suf)) {
	case "AVE", "AVENUE":
		return "AVENUE"
	case "BLVD", "BOULEVARD":
		return "BOULEVARD"
	case "PL", "PLACE":
		return "PLACE"
	default:
		return "STREET"
	}
}

// bareNumberedStreetRE matches a numbered grid street address where dispatch
// (or STT) dropped the ordinal suffix on the street number — "387-EAST-161"
// (hyphen-glued) or "1180 EAST 113" (plain space-separated) instead of
// "387 EAST 161ST"/"1180 EAST 113TH". Both renderings are common: dispatchers
// frequently drop the "TH"/"ST" when reading numbered streets aloud, and STT
// never adds it back. Without the suffix, ExtractLocal can't recognize the
// number as a numbered-street token, and even when it does (via gazetteer
// lookup) the later AddressAlignsWithTranscript alignment check rejects the
// address since "161ST" never appears verbatim in the transcript — so this
// runs in PreCleanTranscript to rewrite the transcript text itself before any
// extraction happens. Requires the trailing number to be 2-3 digits (grid
// street numbers are never 1 or 4+ digits) and a leading house number, so
// this doesn't fire on unrelated digit-direction-digit sequences (badge
// numbers, unit IDs, times, headings without a dispatch address). A 24-hour
// audit of live dispatch transcripts found zero false positives for this
// pattern — every match was a genuine numbered-street address.
var bareNumberedStreetRE = regexp.MustCompile(`(?i)\b(\d{2,6})[\s-]+(NORTH|SOUTH|EAST|WEST|N|S|E|W)[\s-]+(\d{2,3})\b`)

// bareNumberedStreetPrecededByRouteRE / bareNumberedStreetFollowedByAgeRE guard
// against two ways the digit-direction-digit shape can straddle an unrelated
// clause boundary rather than naming one real numbered grid street:
//   - "...STATE ROUTE 45 NORTH. 74-YEAR-OLD MALE..." — after PreCleanTranscript
//     flattens the period to a space, "45 NORTH 74" looks identical to a bare
//     numbered street, but 45 is itself a route number (already claimed as a
//     directional route reference) and 74 is the very next sentence's age.
var (
	bareNumberedStreetPrecededByRouteRE = regexp.MustCompile(`(?i)\b(?:STATE\s+ROUTE|ST\s+ROUTE|SR|ROUTE|RT|RTE|HWY|HIGHWAY)\s*$`)
	bareNumberedStreetFollowedByAgeRE   = regexp.MustCompile(`(?i)^[\s-]*YEAR`)
)

// expandBareNumberedStreetOrdinal rewrites "387-EAST-161" / "1180 EAST 113"
// -> "387 EAST 161ST" / "1180 EAST 113TH" so the numbered-street extractors —
// which require whitespace-separated tokens and an ordinal suffix on the
// street number — can see the address at all, and so it appears verbatim in
// the (now-rewritten) transcript for downstream alignment checks.
func expandBareNumberedStreetOrdinal(transcript string) string {
	var b strings.Builder
	last := 0
	for _, idx := range bareNumberedStreetRE.FindAllStringSubmatchIndex(transcript, -1) {
		full0, full1 := idx[0], idx[1]
		house := transcript[idx[2]:idx[3]]
		dir := transcript[idx[4]:idx[5]]
		num := transcript[idx[6]:idx[7]]
		if bareNumberedStreetPrecededByRouteRE.MatchString(transcript[:full0]) ||
			bareNumberedStreetFollowedByAgeRE.MatchString(transcript[full1:]) {
			b.WriteString(transcript[last:full1])
			last = full1
			continue
		}
		b.WriteString(transcript[last:full0])
		b.WriteString(house + " " + expandGridDirectionalWord(dir) + " " + gridOrdinalWithSuffix(num))
		last = full1
	}
	b.WriteString(transcript[last:])
	return b.String()
}

// houseLeadingSaintAbbrevRE matches a house number directly followed by "ST"
// or "ST." leading a street name ("4021 ST. CLAIR" ← "Saint Clair Avenue").
// "ST" is ambiguous — it means STREET as a trailing suffix after a name, but
// dispatch/STT abbreviates SAINT the same way as a leading qualifier before
// one. Since STREET never leads a street name (only ever trails it), any
// "ST"/"ST." immediately after a house number and before another word is
// unambiguously the SAINT form. Without this, ExtractLocal never recognizes
// the street name at all ("ST" alone isn't a real street) and the call falls
// through to "skipped" even though "SAINT CLAIR AVENUE" is a real,
// gazetteer-known street.
var houseLeadingSaintAbbrevRE = regexp.MustCompile(`(?i)\b(\d{1,6})\s+ST\.?\s+([A-Z][A-Z']+)\b`)

// saintAbbrevExcludedNextWords are tokens that follow "ST"/"ST." for reasons
// unrelated to a Saint-prefixed street name (route abbreviations) and should
// not be rewritten.
var saintAbbrevExcludedNextWords = map[string]bool{
	"RT": true, "RTE": true, "ROUTE": true, "HWY": true, "HIGHWAY": true,
}

// expandHouseLeadingSaintAbbreviation rewrites "4021 ST. CLAIR" -> "4021 SAINT
// CLAIR" so the street name is recognizable to the extractors and gazetteer
// matching below.
func expandHouseLeadingSaintAbbreviation(transcript string) string {
	return houseLeadingSaintAbbrevRE.ReplaceAllStringFunc(transcript, func(m string) string {
		sub := houseLeadingSaintAbbrevRE.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		if saintAbbrevExcludedNextWords[strings.ToUpper(sub[2])] {
			return m
		}
		return sub[1] + " SAINT " + sub[2]
	})
}

// StreetHasOrdinalCore reports when the street name is or contains an ordinal
// token ("5TH AVENUE", "EAST 3RD STREET").
func StreetHasOrdinalCore(street string) bool {
	for _, f := range strings.Fields(strings.ToUpper(strings.TrimSpace(street))) {
		if isOrdinalStreetToken(f) {
			return true
		}
	}
	return false
}

func isOrdinalStreetToken(tok string) bool {
	tok = canonicalOrdinalASRToken(tok)
	if tok == "" {
		return false
	}
	if ordinalDigitTokenRE.MatchString(tok) {
		return true
	}
	_, ok := ordinalWordToDigit[tok]
	return ok
}

// normalizeSpokenOrdinalTokens repairs universal STT near-misses on spoken
// ordinals (FIFITH→FIFTH, 5THH→5TH) without per-street correction tables.
func normalizeSpokenOrdinalTokens(text string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(text)))
	if len(fields) == 0 {
		return text
	}
	changed := false
	for i, f := range fields {
		if norm := canonicalOrdinalASRToken(f); norm != f {
			fields[i] = norm
			changed = true
		}
	}
	if !changed {
		return text
	}
	return strings.Join(fields, " ")
}

func canonicalOrdinalASRToken(tok string) string {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	if tok == "" {
		return tok
	}
	if ordinalDigitTokenRE.MatchString(tok) {
		return tok
	}
	if _, ok := ordinalWordToDigit[tok]; ok {
		return tok
	}
	if len(tok) < 3 || len(tok) > 12 {
		return tok
	}
	if !strings.HasSuffix(tok, "TH") && !strings.HasSuffix(tok, "ND") &&
		!strings.HasSuffix(tok, "RD") && !strings.HasSuffix(tok, "ST") {
		return tok
	}
	for word := range ordinalWordToDigit {
		if ordinalASRNearMiss(tok, word) {
			return word
		}
	}
	for digit := range ordinalDigitToWord {
		if ordinalASRNearMiss(tok, digit) {
			return digit
		}
	}
	return tok
}

func ordinalASRNearMiss(got, want string) bool {
	if got == want || len(got) < 3 || len(want) < 3 {
		return false
	}
	if levenshtein(got, want) != 1 {
		return false
	}
	prefixLen := 3
	if len(want) < prefixLen {
		prefixLen = len(want)
	}
	if len(got) < prefixLen {
		return false
	}
	return got[:prefixLen] == want[:prefixLen]
}

// ordinalStreetNameVariants returns 5TH↔FIFTH style alternates for geocode lookup.
func ordinalStreetNameVariants(street string) []string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(street)))
	if len(fields) == 0 {
		return nil
	}
	var out []string
	for i, f := range fields {
		if word, ok := ordinalDigitToWord[f]; ok {
			alt := append([]string{}, fields...)
			alt[i] = word
			out = append(out, strings.Join(alt, " "))
		}
		if digit, ok := ordinalWordToDigit[f]; ok {
			alt := append([]string{}, fields...)
			alt[i] = digit
			out = append(out, strings.Join(alt, " "))
		}
	}
	return out
}

// ordinalStreetSuffixesCompatible blocks AVE↔ST swaps on ordinal streets while
// still allowing compatible abbreviations (AVE↔AVENUE).
func ordinalStreetSuffixesCompatible(a, b string) bool {
	if !StreetHasOrdinalCore(a) && !StreetHasOrdinalCore(b) {
		return true
	}
	_, aType := StreetCoreTypeKey(CanonicalStreetName(a))
	_, bType := StreetCoreTypeKey(CanonicalStreetName(b))
	if aType != "" && bType != "" {
		return aType == bType
	}
	_, aSuffix := streetNameAndSuffix(a)
	_, bSuffix := streetNameAndSuffix(b)
	if aSuffix == "" || bSuffix == "" {
		return true
	}
	return streetSuffixesCompatible(aSuffix, bSuffix)
}

// inferOrdinalSuffixFromTranscript reads AVENUE vs STREET from dispatch when
// extraction captured only the ordinal ("4726 5TH" from "4726 5TH AVENUE").
func inferOrdinalSuffixFromTranscript(house, street, transcript string) string {
	u := strings.ToUpper(strings.TrimSpace(transcript))
	if u == "" {
		return ""
	}
	ord := ""
	for _, f := range strings.Fields(strings.ToUpper(strings.TrimSpace(street))) {
		if isOrdinalStreetToken(f) {
			ord = f
			break
		}
	}
	if ord == "" {
		return ""
	}
	check := func(fragment string) string {
		if house != "" && strings.Contains(u, house+" "+fragment) {
			_, suf := streetNameAndSuffix(fragment)
			return suf
		}
		if strings.Contains(u, fragment) {
			_, suf := streetNameAndSuffix(fragment)
			return suf
		}
		return ""
	}
	for _, frag := range []string{ord + " AVENUE", ord + " AVE", ord + " STREET", ord + " ST"} {
		if suf := check(frag); suf != "" {
			return suf
		}
	}
	return ""
}

// inferGeneralStreetSuffixFromTranscript reads AVENUE vs ROAD (etc.) from dispatch
// when extraction or homonym snapping substituted a different thoroughfare type.
func inferGeneralStreetSuffixFromTranscript(house, street, transcript string) string {
	u := strings.ToUpper(strings.TrimSpace(transcript))
	if u == "" || house == "" {
		return ""
	}
	stem, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(street)))
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return ""
	}
	check := func(fragment string) string {
		frag := strings.ToUpper(strings.TrimSpace(fragment))
		if frag == "" {
			return ""
		}
		if house != "" && strings.Contains(u, house+" "+frag) {
			_, suf := streetNameAndSuffix(frag)
			return suf
		}
		if strings.Contains(u, " "+frag+" ") || strings.Contains(u, " "+frag+",") ||
			strings.Contains(u, " "+frag+".") {
			_, suf := streetNameAndSuffix(frag)
			return suf
		}
		return ""
	}
	for _, suf := range []string{
		"AVENUE", "AVE", "ROAD", "RD", "STREET", "ST", "DRIVE", "DR",
		"BOULEVARD", "BLVD", "LANE", "LN", "COURT", "CT", "PLACE", "PL",
	} {
		if s := check(stem + " " + suf); s != "" {
			return s
		}
	}
	return ""
}

func streetEndsWithQuadrantSuffix(street string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(street)))
	if len(fields) == 0 {
		return false
	}
	switch fields[len(fields)-1] {
	case "SE", "SW", "NE", "NW", "SOUTHEAST", "SOUTHWEST", "NORTHEAST", "NORTHWEST":
		return true
	default:
		return false
	}
}

// AlignAddressSuffixFromTranscript restores the dispatcher's spoken thoroughfare
// type when extraction omitted or substituted a suffix.
func AlignAddressSuffixFromTranscript(addr, transcript string) string {
	return alignAddressSuffixFromTranscript(addr, transcript)
}

// alignAddressSuffixFromTranscript restores the dispatcher's spoken thoroughfare
// type when extraction or homonym snapping substituted a different one
// (4726 5TH STREET SW ← "4726 5TH AVENUE").
func alignAddressSuffixFromTranscript(addr, transcript string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return addr
	}
	want := inferOrdinalSuffixFromTranscript(house, street, transcript)
	if want == "" {
		want = inferGeneralStreetSuffixFromTranscript(house, street, transcript)
	}
	if want == "" {
		return addr
	}
	_, have := StreetCoreTypeKey(CanonicalStreetName(street))
	if have == want {
		return addr
	}
	if have == "" && want != "" {
		// Transcript names a thoroughfare type the extracted address omitted
		// ("4125 WEST MOTT" ← "WEST MOTT DRIVE" spoken elsewhere in dispatch).
		if streetEndsWithQuadrantSuffix(street) {
			return addr
		}
		return house + " " + street + " " + want
	}
	if have != "" && have != want {
		u := strings.ToUpper(transcript)
		needle := house + " "
		idx := strings.Index(u, needle)
		if idx < 0 {
			return addr
		}
		tail := strings.TrimSpace(u[idx+len(needle):])
		end := len(tail)
		for i, ch := range tail {
			if ch == ',' || ch == '.' || ch == ';' || ch == '?' || ch == '!' {
				end = i
				break
			}
		}
		frag := strings.TrimSpace(tail[:end])
		if frag == "" {
			return addr
		}
		_, fragType := StreetCoreTypeKey(CanonicalStreetName(frag))
		if fragType == want {
			return house + " " + frag
		}
	}
	return addr
}
