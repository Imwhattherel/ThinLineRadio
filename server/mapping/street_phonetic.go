// Copyright (C) 2025 Thinline Dynamic Solutions
//
// street_phonetic.go — phonetic keys and scored STT street-core matching.
// Complements edit-distance rules in streetmatch.go for homophones that share
// little orthographic overlap (YOULL→YULE, BEACH→BEECH).

package mapping

import (
	"strings"
)

const sttMatchScoreThreshold = 0.72

// sttMatchContext carries gazetteer guard state for core-level scoring.
type sttMatchContext struct {
	queryCoreExists bool
	coreSet         map[string]bool
}

type streetPhoneticIndex struct {
	coreSet map[string]bool
	byKey   map[string][]string // phonetic key → street cores
}

func (scope *ScopeData) phoneticIndex() *streetPhoneticIndex {
	if scope == nil {
		return nil
	}
	scope.phoneticOnce.Do(func() {
		scope.phoneticIdx = buildStreetPhoneticIndex(scope.KnownStreets)
	})
	return scope.phoneticIdx
}

func buildStreetPhoneticIndex(knownStreets []string) *streetPhoneticIndex {
	idx := &streetPhoneticIndex{
		coreSet: map[string]bool{},
		byKey:   map[string][]string{},
	}
	seenCore := map[string]bool{}
	for _, ks := range knownStreets {
		_, core, _ := splitStreetParts(CanonicalStreetName(ks))
		if core == "" || seenCore[core] {
			continue
		}
		seenCore[core] = true
		idx.coreSet[core] = true
		for _, key := range streetSTTPhoneticKeys(core) {
			idx.byKey[key] = appendPhoneticCore(idx.byKey[key], core)
		}
	}
	return idx
}

func appendPhoneticCore(list []string, core string) []string {
	for _, c := range list {
		if c == core {
			return list
		}
	}
	return append(list, core)
}

// PhoneticStreetCoreCandidates returns imported street cores whose phonetic
// keys overlap the query core. Skips lookup when the query core already exists.
func (scope *ScopeData) PhoneticStreetCoreCandidates(qCore string) []string {
	qCore = strings.ToUpper(strings.TrimSpace(qCore))
	if qCore == "" {
		return nil
	}
	idx := scope.phoneticIndex()
	if idx == nil {
		return nil
	}
	if idx.coreSet[qCore] {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, key := range streetSTTPhoneticKeys(qCore) {
		for _, core := range idx.byKey[key] {
			if core == qCore || seen[core] {
				continue
			}
			seen[core] = true
			out = append(out, core)
		}
	}
	return out
}

func isStreetVowel(c byte) bool {
	switch c {
	case 'A', 'E', 'I', 'O', 'U', 'Y':
		return true
	default:
		return false
	}
}

// streetConsonantSkeleton returns a compact consonant frame for phonetic compare.
// The opening letter is always kept; interior vowels are dropped and repeated
// consonants collapsed (YOULL→YL, YULE→YL, BEACH→BCH).
func streetConsonantSkeleton(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	last := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			continue
		}
		if i == 0 {
			b.WriteByte(c)
			last = c
			continue
		}
		if isStreetVowel(c) {
			continue
		}
		if c == last {
			continue
		}
		b.WriteByte(c)
		last = c
	}
	return b.String()
}

func streetSTTPhoneticKeys(core string) []string {
	core = strings.ToUpper(strings.TrimSpace(core))
	if core == "" || streetCoreHasDigit(core) {
		return nil
	}
	seen := map[string]bool{}
	add := func(k string) {
		if len(k) < 2 {
			return
		}
		if !seen[k] {
			seen[k] = true
		}
	}
	for _, variant := range sttDoubledLetterVariants(core) {
		sk := streetConsonantSkeleton(variant)
		add(sk)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

func streetSTTPhoneticKeysOverlap(a, b string) bool {
	ka := streetSTTPhoneticKeys(a)
	kb := streetSTTPhoneticKeys(b)
	if len(ka) == 0 || len(kb) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, k := range ka {
		set[k] = true
	}
	for _, k := range kb {
		if !set[k] {
			continue
		}
		// Require full skeleton keys — 4-char prefixes (KNGS) false-match KINGSGRAVE/KINGSTON.
		if len(k) >= 5 {
			return true
		}
		if k == streetConsonantSkeleton(a) && k == streetConsonantSkeleton(b) {
			return true
		}
	}
	return false
}

func sttCoreOpeningDigraph(core string) string {
	if len(core) >= 2 {
		return core[:2]
	}
	return core
}

func scoreStreetSkeletonMatch(qCore, cCore string) float64 {
	qSk := streetConsonantSkeleton(qCore)
	cSk := streetConsonantSkeleton(cCore)
	if qSk == "" || cSk == "" || len(qSk) < 2 || len(cSk) < 2 {
		return 0
	}
	if qSk == cSk {
		return 0.88
	}
	// Fuzzy skeleton only on short cores — long names like BELMONT/BEAUMONT share a
	// close skeleton but are distinct thoroughfares.
	if len(qCore) > 5 || len(cCore) > 5 {
		return 0
	}
	if len(qCore) >= 5 && len(cCore) >= 5 &&
		sttCoreOpeningDigraph(qCore) != sttCoreOpeningDigraph(cCore) {
		return 0
	}
	if len(qSk) >= 3 && len(cSk) >= 3 && levenshtein(qSk, cSk) <= 1 {
		return 0.78
	}
	return 0
}

// ScoreStreetSTTCoreMatch returns a confidence score in [0,1] for two street
// name cores after radio STT. Zero means no match.
func ScoreStreetSTTCoreMatch(qCore, cCore string, ctx *sttMatchContext) float64 {
	qCore = strings.ToUpper(strings.TrimSpace(qCore))
	cCore = strings.ToUpper(strings.TrimSpace(cCore))
	if qCore == "" || cCore == "" {
		return 0
	}
	if qCore == cCore {
		return 1
	}
	if streetTokensNeverSTTMatch(qCore, cCore) {
		return 0
	}
	if streetCoreHasDigit(qCore) || streetCoreHasDigit(cCore) {
		return 0
	}
	if ctx != nil && ctx.coreSet != nil &&
		ctx.coreSet[qCore] && ctx.coreSet[cCore] && qCore != cCore {
		return 0
	}

	score := 0.0
	if sttDoubledLetterHomophone(qCore, cCore) {
		score = maxFloat(score, 0.82)
	}
	if sk := scoreStreetSkeletonMatch(qCore, cCore); sk > score {
		score = sk
	}
	if streetSTTPhoneticKeysOverlap(qCore, cCore) {
		score = maxFloat(score, 0.80)
	}
	if absInt(len(cCore)-len(qCore)) <= 2 {
		maxDist := sttStreetTokenMaxEditDistance(qCore, cCore)
		d := levenshtein(qCore, cCore)
		if d <= maxDist && qCore[0] == cCore[0] {
			if d <= 1 || (len(qCore) >= 2 && len(cCore) >= 2 && qCore[:2] == cCore[:2]) {
				allowLev := true
				if d >= 2 && len(qCore) >= 6 {
					qSk, cSk := streetConsonantSkeleton(qCore), streetConsonantSkeleton(cCore)
					if qSk != cSk {
						skTol := 1
						if d >= 3 {
							skTol = 2 // STAMBALL/STAMBAUGH → STMBL↔STMBGH
						}
						skOK := len(qSk) >= 2 && len(cSk) >= 2 &&
							qSk[:2] == cSk[:2] &&
							levenshtein(qSk, cSk) <= skTol
						if !skOK {
							allowLev = false
						}
					}
				}
				if allowLev {
					levScore := 0.68 + 0.20*float64(maxDist-d+1)/float64(maxDist+1)
					score = maxFloat(score, levScore)
				}
			}
		}
		// Leading vowel STT (ALCETTA↔ELSETTA): onset vowel differs, but the
		// rest of the stem is within one edit (soft C/S, etc.).
		if isStreetVowel(qCore[0]) && isStreetVowel(cCore[0]) &&
			qCore[0] != cCore[0] &&
			len(qCore) >= 6 && len(cCore) >= 6 &&
			levenshtein(qCore[1:], cCore[1:]) <= 1 {
			score = maxFloat(score, 0.80)
		}
	}
	if ctx != nil && ctx.queryCoreExists && cCore != qCore && score < 0.85 {
		return 0
	}
	if score < sttMatchScoreThreshold {
		return 0
	}
	return score
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// StreetTokensSTTMatchScore exposes the raw scorer for tests and tuning.
func StreetTokensSTTMatchScore(a, b string) float64 {
	return ScoreStreetSTTCoreMatch(a, b, nil)
}
