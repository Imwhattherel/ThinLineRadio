// Copyright (C) 2025 Thinline Dynamic Solutions
//
// streetmatch.go — universal fuzzy street-name matching for the local geocoder.
// Handles two real-world classes of mismatch between dispatch transcripts and
// OSM street names, without any per-department data:
//
//  1. Whitespace splits/joins: "KIM ROSE LN" (STT) vs "KIMROSE LN" (OSM).
//  2. Near-spellings / homophones: "BLANE ST" vs "BLAINE ST",
//     "BEECH ST" vs "BEACH ST", "YULE ST" vs "YOULL ST" (phonetic skeleton).
//
// Scored matching lives in street_phonetic.go (consonant skeleton + phonetic
// index on ScopeData.KnownStreets). This file keeps canonical token parsing
// and BestFuzzyStreetMatch orchestration.

package mapping

import (
	"sort"
	"strings"
)

// stripStreetSpaces removes spaces for collapsed STT/gazetteer comparison
// ("AUSTIN TOWN WARREN" vs "AUSTINTOWN WARREN").
func stripStreetSpaces(s string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(s)), " ", "")
}

// streetTypeTokens are the canonical trailing thoroughfare types produced by
// canonicalStreetTokens.
var streetTypeTokens = map[string]bool{
	"RD": true, "ST": true, "AVE": true, "DR": true, "LN": true, "CT": true,
	"BLVD": true, "PL": true, "WAY": true, "TRL": true, "HWY": true,
	"PKWY": true, "CIR": true, "TERR": true,
}

// streetDirTokens are the canonical leading/trailing directionals.
var streetDirTokens = map[string]bool{
	"N": true, "S": true, "E": true, "W": true,
	"NE": true, "NW": true, "SE": true, "SW": true,
}

// StreetNameCore returns the space-collapsed core of a canonical street name
// (directionals and street type stripped).
func StreetNameCore(canonical string) string {
	_, core, _ := splitStreetParts(canonical)
	return core
}

// splitStreetParts breaks a canonical street name into a leading directional, a
// core name, and a trailing street type. Any of the three may be empty.
func splitStreetParts(canonical string) (dir, core, stype string) {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canonical)))
	if len(fields) == 0 {
		return "", "", ""
	}
	// A trailing quadrant directional sits after the type ("OLIVE AVE NE"),
	// so it must be stripped before the type check below or the type token
	// stays hidden behind it and this function reports stype="" for a street
	// that plainly has one. That empty type then defeats every qType!=""&&
	// cType!="" guard downstream (BestFuzzyStreetMatch, StreetNamesSTTMatch),
	// letting "OLIVE STREET" (a real, different street) fuzzy-match "OLIVE
	// AVE NE" purely because the trailing NE hid AVE's type.
	if len(fields) > 2 && streetDirTokens[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	if len(fields) > 1 && streetTypeTokens[fields[len(fields)-1]] {
		stype = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	// A directional token can BE the street name ("SOUTH AVENUE", "EAST AVE
	// SE") — only strip a leading directional when a real name token remains
	// afterward, matching StreetCoreTypeKey's guard so the two functions never
	// disagree about what counts as the name.
	if len(fields) > 1 && streetDirTokens[fields[0]] && hasStreetNameToken(fields[1:]) {
		dir = fields[0]
		fields = fields[1:]
	}
	core = strings.Join(fields, "")
	return dir, core, stype
}

// StreetCoreTypeKey returns a directional-insensitive (core, type) for a
// canonical street name. Leading and trailing directionals are stripped, the
// trailing thoroughfare type is separated, and the remaining name tokens are
// space-collapsed. "STANTON AVE", "STANTON AVE SE" and "N STANTON AVENUE" all
// share core "STANTON", type "AVE".
func StreetCoreTypeKey(canonical string) (core, stype string) {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canonical)))
	// A directional token can BE the street name (SOUTH AVENUE, EAST AVE SE).
	// Only strip a directional when real name tokens remain afterward —
	// otherwise SOUTH AVE and EAST AVE SE both collapse to the core "AVE" and
	// every directional-named street matches every other one.
	for len(fields) > 1 && streetDirTokens[fields[len(fields)-1]] &&
		hasNonTypeToken(fields[:len(fields)-1]) {
		fields = fields[:len(fields)-1]
	}
	for len(fields) > 1 && streetDirTokens[fields[0]] &&
		hasStreetNameToken(fields[1:]) {
		fields = fields[1:]
	}
	if len(fields) > 1 && streetTypeTokens[fields[len(fields)-1]] {
		stype = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	core = strings.Join(fields, "")
	return core, stype
}

// hasStreetNameToken reports whether any token is a real name word (neither a
// directional nor a thoroughfare type).
func hasStreetNameToken(fields []string) bool {
	for _, w := range fields {
		if !streetDirTokens[w] && !streetTypeTokens[w] {
			return true
		}
	}
	return false
}

// hasNonTypeToken reports whether any token is not a thoroughfare type — a
// directional counts, so "EAST AVE" qualifies as a name for quadrant stripping.
func hasNonTypeToken(fields []string) bool {
	for _, w := range fields {
		if !streetTypeTokens[w] {
			return true
		}
	}
	return false
}

// StreetDirectional returns the leading or trailing directional quadrant of a
// street name ("OLIVE AVE NE" → "NE", "716 OLIVE STREET" → ""), canonicalizing
// first so spelled and abbreviated forms both resolve. Exported for the geocode
// store's directional gating.
func StreetDirectional(name string) string {
	return streetDirToken(CanonicalStreetName(name))
}

// streetDirToken returns the leading or trailing directional of a canonical
// street name ("E 85TH ST" → "E", "STANTON AVE SE" → "SE"), or "" when none.
func streetDirToken(canonical string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canonical)))
	if len(fields) > 1 && streetDirTokens[fields[0]] {
		return fields[0]
	}
	if len(fields) > 1 && streetDirTokens[fields[len(fields)-1]] {
		return fields[len(fields)-1]
	}
	return ""
}

// DirectionalVariantNames returns the candidate names that are the same street
// as the query, used to disambiguate by coverage proximity. It shares the
// query's directional-insensitive core+type, but it only substitutes a
// different directional quadrant when the query itself omits one:
//
//   - "STANTON AVE" (no quadrant) ↔ "STANTON AVE SE"  — allowed (dispatch dropped it)
//   - "EAST 85TH ST"              ✗ "WEST 85TH ST"     — rejected (dispatcher said EAST)
//
// A candidate with no directional is always compatible (it may be the same
// unsigned street).
func DirectionalVariantNames(query string, candidates []string) []string {
	qc, qt := StreetCoreTypeKey(query)
	if qc == "" {
		return nil
	}
	qd := streetDirToken(query)
	var out []string
	for _, c := range candidates {
		cc, ct := StreetCoreTypeKey(c)
		if cc != qc || ct != qt {
			continue
		}
		cd := streetDirToken(c)
		// When the query specifies a directional, never swap it for a different
		// one; only the same directional (or an unsigned variant) is the "same"
		// street.
		if qd != "" && cd != "" && cd != qd {
			continue
		}
		out = append(out, c)
	}
	return out
}

// streetCoreNameTokens returns the core name tokens of a canonical street name
// with leading/trailing directionals and the trailing thoroughfare type
// removed: "N CARVER NILES RD" → ["CARVER","NILES"].
func streetCoreNameTokens(canonical string) []string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(canonical)))
	for len(fields) > 1 && streetDirTokens[fields[0]] {
		fields = fields[1:]
	}
	for len(fields) > 1 && streetDirTokens[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	if len(fields) > 1 && streetTypeTokens[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	return fields
}

// sameStreetCoreTokenSet reports whether two canonical street names share the
// same multiset of core name tokens regardless of order — connector roads named
// for the two towns they link are written either way ("CARVER NILES RD" vs
// "NILES CARVER RD"). Requires at least two tokens so single-word streets don't
// collapse together, and only used as a fuzzy fallback after an exact
// normalized match has already failed.
func sameStreetCoreTokenSet(a, b string) bool {
	ta := streetCoreNameTokens(a)
	tb := streetCoreNameTokens(b)
	if len(ta) < 2 || len(ta) != len(tb) {
		return false
	}
	ca := append([]string(nil), ta...)
	cb := append([]string(nil), tb...)
	sort.Strings(ca)
	sort.Strings(cb)
	for i := range ca {
		if ca[i] != cb[i] {
			return false
		}
	}
	return true
}

func streetCoreHasDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// BestFuzzyStreetMatch finds the candidate street name that best matches the
// (canonical) query under whitespace-insensitive and near-spelling rules.
// Candidates must also be canonical. Returns the matched candidate and true, or
// ("", false) when none is close enough.
//
// Rules (precision-first):
//   - When both sides carry a street type, the types must agree (ST != AVE).
//   - When both sides carry a directional, the directionals must agree (E != W).
//   - Whitespace-only differences (collapsed cores equal) always match.
//   - Otherwise a small Levenshtein distance on the space-collapsed core matches
//     (<=1 for short names, <=2 for cores of length >= 7).
//   - Numeric/ordinal cores ("10TH", "SR 88") never fuzzy-match by edit
//     distance — only exact collapsed equality — so "10TH" can't become "11TH".
//   - When the query core already exists on any imported street (any type or
//     directional), edit-distance matching to a different core is blocked
//     (GLENWOOD DR must not become GALEWOOD DR).
func BestFuzzyStreetMatch(query string, candidates []string) (string, bool) {
	qDir, qCore, qType := splitStreetParts(query)
	if qCore == "" {
		return "", false
	}
	qHasDigit := streetCoreHasDigit(qCore)

	coreSet := map[string]bool{}
	for _, c := range candidates {
		_, cc, _ := splitStreetParts(c)
		if cc != "" {
			coreSet[cc] = true
		}
	}
	queryCoreExists := coreSet[qCore]

	best := ""
	bestScore := 0.0
	for _, c := range candidates {
		cDir, cCore, cType := splitStreetParts(c)
		if cCore == "" {
			continue
		}
		if qType != "" && cType != "" && qType != cType {
			continue
		}
		if qDir != "" && cDir != "" && qDir != cDir {
			continue
		}
		if cCore == qCore {
			return c, true // whitespace-only difference
		}
		// Two distinct imported cores — never conflate by edit distance.
		if cCore != qCore && coreSet[qCore] && coreSet[cCore] {
			continue
		}
		// Same core name tokens in a different order ("CARVER NILES RD" vs
		// "NILES CARVER RD"): the type/directional checks above already passed,
		// so treat it as the same street.
		if sameStreetCoreTokenSet(query, c) {
			return c, true
		}
		if qHasDigit || streetCoreHasDigit(cCore) {
			continue // never fuzzy-match numeric/ordinal streets
		}
		ctx := &sttMatchContext{queryCoreExists: queryCoreExists, coreSet: coreSet}
		if score := ScoreStreetSTTCoreMatch(qCore, cCore, ctx); score > bestScore {
			bestScore = score
			best = c
		}
	}
	if best != "" && bestScore >= sttMatchScoreThreshold {
		return best, true
	}
	return "", false
}

// StreetTokensSTTMatch reports whether two single-word tokens are likely the
// same street-name word after radio STT (BEACH/BEECH, MELTON/MILTON).
func StreetTokensSTTMatch(a, b string) bool {
	return ScoreStreetSTTCoreMatch(a, b, nil) >= sttMatchScoreThreshold
}

// sttStreetTokenMaxEditDistance returns the Levenshtein threshold for likely
// same-word STT errors. Short names with a shared opening digraph (QUAYLE→QUAIL)
// need two edits; long names already tolerate two.
func sttStreetTokenMaxEditDistance(a, b string) int {
	if len(a) >= 7 || len(b) >= 7 {
		// Long STT slips at edit distance 3 (STAMBALL→STAMBAUGH,
		// MAHOGANY→MAHONING) when the opening digraph and a 4-char prefix agree.
		if len(a) >= 2 && len(b) >= 2 && a[0] == b[0] && a[1] == b[1] {
			n := 0
			for n < len(a) && n < len(b) && a[n] == b[n] {
				n++
			}
			if n >= 4 {
				return 3
			}
		}
		return 2
	}
	if len(a) >= 5 && len(b) >= 5 && len(a) >= 2 && len(b) >= 2 &&
		a[0] == b[0] && a[1] == b[1] &&
		absInt(len(a)-len(b)) <= 1 {
		return 2
	}
	// Short names: STT often drops a doubled letter or vowel (YOULL→YULE).
	if len(a) >= 4 && len(b) >= 4 && a[0] == b[0] &&
		absInt(len(a)-len(b)) <= 1 {
		return 2
	}
	return 1
}

// sttDoubledLetterHomophone reports when STT drops a doubled consonant
// (YOULL→YULE) or similar near-homophone on the collapsed form.
func sttDoubledLetterHomophone(a, b string) bool {
	a = strings.ToUpper(strings.TrimSpace(a))
	b = strings.ToUpper(strings.TrimSpace(b))
	if a == "" || b == "" || a[0] != b[0] {
		return false
	}
	if streetCoreHasDigit(a) || streetCoreHasDigit(b) {
		return false
	}
	if a == b {
		return true
	}
	if absInt(len(a)-len(b)) > 2 {
		return false
	}
	for _, va := range sttDoubledLetterVariants(a) {
		for _, vb := range sttDoubledLetterVariants(b) {
			if va == vb {
				return true
			}
			if (va != a || vb != b) &&
				levenshtein(va, vb) <= 2 &&
				absInt(len(va)-len(vb)) <= 1 &&
				len(va) <= 5 && len(vb) <= 5 {
				return true
			}
		}
	}
	return false
}

func sttDoubledLetterVariants(s string) []string {
	seen := map[string]bool{s: true}
	out := []string{s}
	for i := 0; i < len(s)-1; i++ {
		if s[i] != s[i+1] {
			continue
		}
		v := s[:i] + s[i+1:]
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// streetDirectionalsCompatible reports when two direction tokens mean the same
// quadrant (SOUTH ≡ S, NORTHEAST ≡ NE).
func streetDirectionalsCompatible(a, b string) bool {
	a = strings.ToUpper(strings.TrimSpace(a))
	b = strings.ToUpper(strings.TrimSpace(b))
	if a == "" || b == "" {
		return a == b
	}
	if a == b {
		return true
	}
	canon := map[string]string{
		"N": "N", "NORTH": "N",
		"S": "S", "SOUTH": "S",
		"E": "E", "EAST": "E",
		"W": "W", "WEST": "W",
		"NE": "NE", "NORTHEAST": "NE",
		"NW": "NW", "NORTHWEST": "NW",
		"SE": "SE", "SOUTHEAST": "SE",
		"SW": "SW", "SOUTHWEST": "SW",
	}
	return canon[a] != "" && canon[a] == canon[b]
}

func streetTokensNeverSTTMatch(a, b string) bool {
	blocked := map[string]map[string]bool{
		"CREEK":     {"COURT": true},
		"COURT":     {"CREEK": true},
	}
	ab, ok := blocked[a]
	return ok && ab[b]
}

// StreetNamesSTTMatch reports whether two street names likely refer to the same
// thoroughfare after STT errors. Uses the same fuzzy rules as gazetteer lookup.
func StreetNamesSTTMatch(a, b string) bool {
	ca := CanonicalStreetName(a)
	cb := CanonicalStreetName(b)
	if ca == "" || cb == "" {
		return false
	}
	if ca == cb {
		return true
	}
	matched, ok := BestFuzzyStreetMatch(ca, []string{cb})
	if ok && matched == cb {
		return true
	}
	// Multi-word stems: BestFuzzyStreetMatch scores the glued core
	// ("DURSTCLACK" vs "DURSTCLAGG", lev=2) and can miss a one-token STT
	// slip that token match accepts ("CLACK"↔"CLAGG"). Align word-by-word.
	return streetWordTokensSTTMatch(ca, cb)
}

// streetWordTokensSTTMatch reports when two streets share the same word
// count and every corresponding name token is an STT match (identical or
// fuzzy). Suffixes must be compatible when both sides name one.
func streetWordTokensSTTMatch(a, b string) bool {
	aName, aSuf := streetNameAndSuffix(a)
	bName, bSuf := streetNameAndSuffix(b)
	if aSuf != "" && bSuf != "" && !streetSuffixesCompatible(aSuf, bSuf) {
		return false
	}
	aToks := streetNameWordTokens(aName)
	bToks := streetNameWordTokens(bName)
	if len(aToks) == 0 || len(aToks) != len(bToks) {
		return false
	}
	for i := range aToks {
		if aToks[i] == bToks[i] {
			continue
		}
		if !StreetTokensSTTMatch(aToks[i], bToks[i]) {
			return false
		}
	}
	return true
}

// FuzzyStreetVariants returns alternate canonical spellings from candidates that
// fuzzy-match the query — STT mis-hearings resolved against the local gazetteer.
func FuzzyStreetVariants(streetOnly string, candidates []string) []string {
	q := CanonicalStreetName(streetOnly)
	if q == "" || len(candidates) == 0 {
		return nil
	}
	seen := map[string]bool{}
	canonicalCands := make([]string, 0, len(candidates))
	for _, c := range candidates {
		cc := CanonicalStreetName(c)
		if cc == "" || seen[cc] {
			continue
		}
		seen[cc] = true
		canonicalCands = append(canonicalCands, cc)
	}
	matched, ok := BestFuzzyStreetMatch(q, canonicalCands)
	if !ok || matched == q {
		return nil
	}
	return []string{matched}
}
