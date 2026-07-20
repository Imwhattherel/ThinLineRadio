// Copyright (C) 2025 Thinline Dynamic Solutions
//
// When dispatch repeats a house number with two street spellings ("5183 THOMAS
// STREET, 5183 TOP STREET"), prefer the variant that matches the gazetteer or
// appears first in the transcript.

package mapping

import (
	"log"
	"strconv"
	"strings"
)

// maybePreferTranscriptStreetVariant replaces an LLM address when the same house
// number appears in the transcript under a better street spelling.
func maybePreferTranscriptStreetVariant(curated *CuratedAlert, transcript string, scope *ScopeData) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	house, _ := splitHouseAndStreet(curated.Address)
	if house == "" {
		return
	}
	cleaned := PreCleanTranscript(transcript)
	u := strings.ToUpper(cleaned)
	best := strings.ToUpper(strings.TrimSpace(curated.Address))
	bestScore, bestIdx := scoreStreetVariant(best, u, scope)
	for _, c := range extractDispatchAddressesFromTranscript(cleaned) {
		c = strings.ToUpper(strings.TrimSpace(c))
		h, st := splitHouseAndStreet(c)
		if h != house || dispatchStreetIsSpelledLetters(st) {
			continue
		}
		sc, idx := scoreStreetVariant(c, u, scope)
		if gazetteerSpellingShouldBeatTranscriptHomonym(best, c, scope) {
			continue
		}
		if phoneticGazetteerPreferredOverSTT(c, best, scope) {
			continue
		}
		_, bestSt := splitHouseAndStreet(best)
		_, candSt := splitHouseAndStreet(c)
		related := streetsAreCoverageHomonyms(bestSt, candSt) ||
			StreetNamesSTTMatch(bestSt, candSt) ||
			StreetTokensSTTMatch(stripStreetSpaces(streetNameAndSuffixFirst(bestSt)),
				stripStreetSpaces(streetNameAndSuffixFirst(candSt)))
		take := false
		switch {
		case !related && bestIdx >= 0 && idx >= 0 && idx != bestIdx:
			// Unrelated same-house streets are a dispatcher correction
			// ("1050 SCREEN ROAD 1050 SOUTH GREEN") — later restatement wins
			// even when the earlier STT garble has a thoroughfare suffix score.
			take = idx > bestIdx
		case sc > bestScore:
			take = true
		case sc == bestScore && idx >= 0 && (bestIdx < 0 || idx < bestIdx):
			take = true
		}
		if take {
			bestScore = sc
			bestIdx = idx
			best = c
		}
	}
	if best != strings.ToUpper(strings.TrimSpace(curated.Address)) {
		cur := strings.ToUpper(strings.TrimSpace(curated.Address))
		curHouse, curSt := splitHouseAndStreet(cur)
		bestHouse, bestSt := splitHouseAndStreet(best)
		if curHouse == bestHouse && streetsAreCoverageHomonyms(bestSt, curSt) && len(curSt) > len(bestSt) &&
			hasTrailingDirectionalInCanonical(CanonicalStreetName(curSt)) &&
			!streetThoroughfareSuffixSpokenInTranscript(curSt, cleaned) {
			return
		}
		if curHouse == bestHouse && (streetsAreCoverageHomonyms(curSt, bestSt) || StreetNamesSTTMatch(curSt, bestSt)) &&
			scope != nil && homonymPinAlignStreetIsExactImport(curSt, scope.KnownStreets) {
			return
		}
		if curHouse == bestHouse && len(bestSt) < len(curSt) {
			spoken := spokenStreetPhraseAfterHouse(cleaned, curHouse)
			if spoken != "" {
				curCore := stripStreetSpaces(streetNameAndSuffixFirst(curSt))
				spokenCore := stripStreetSpaces(spoken)
				if curCore != "" && spokenCore != "" &&
					(strings.HasPrefix(curCore, spokenCore) || strings.HasPrefix(spokenCore, curCore)) {
					return
				}
			}
			// Keep "LAKE ROAD WEST" when the shorter harvest is only "LAKE ROAD"
			// and the trailing cardinal was spoken after the street.
			if hasTrailingDirectionalInCanonical(CanonicalStreetName(curSt)) &&
				!hasTrailingDirectionalInCanonical(CanonicalStreetName(bestSt)) &&
				transcriptContainsHouseStreetFragment(cleaned, curHouse, curSt) {
				return
			}
		}
		log.Printf("[INFO] address variant: prefer transcript %q over %q", best, curated.Address)
		curated.Address = best
	}
	if route := dispatchRouteStreetAfterHouse(house, cleaned); route != "" {
		routeAddr := house + " " + route
		if routeAddr != strings.ToUpper(strings.TrimSpace(curated.Address)) {
			log.Printf("[INFO] address variant: prefer state route %q over %q", routeAddr, curated.Address)
			curated.Address = routeAddr
		}
	}
	if scope != nil {
		h, st := splitHouseAndStreet(curated.Address)
		if alt := phoneticGazetteerStreetCorrection(h, st, scope.KnownStreets); alt != "" {
			curated.Address = h + " " + alt
		}
	}
}

func scoreStreetVariant(addr, transcriptUpper string, scope *ScopeData) (score, firstIdx int) {
	_, st := splitHouseAndStreet(addr)
	if st == "" {
		return 0, -1
	}
	if hasStreetSuffix(st) && streetThoroughfareSuffixSpokenInTranscript(st, transcriptUpper) {
		score += 2
	}
	if _, _, dir := splitStreetParts(CanonicalStreetName(st)); dir != "" {
		score += 3
	}
	if isPlausibleLocalStreet(st, scope) {
		score += 2
	}
	h, _ := splitHouseAndStreet(addr)
	if h != "" && dispatchHouseStreetPhraseInTranscript(transcriptUpper, h, st) {
		score += 12
	} else if spoken := spokenStreetPhraseAfterHouse(transcriptUpper, h); spoken != "" {
		canonSpoken := stripStreetSpaces(spoken)
		canonAddr := stripStreetSpaces(streetNameAndSuffixFirst(st))
		if canonSpoken != "" && canonAddr != "" && canonSpoken != canonAddr &&
			!strings.Contains(canonAddr, canonSpoken) && !strings.Contains(canonSpoken, canonAddr) {
			score -= 10
		}
	}
	if scope != nil {
		want := CanonicalStreetName(st)
		for _, ks := range scope.KnownStreets {
			if CanonicalStreetName(ks) == want {
				score += 6
				break
			}
		}
	}
	// A street whose stem is absent from the gazetteer but phonetically matches
	// a gazetteer stem is likely an STT mishearing — penalize it so the imported
	// spelling wins (universal rule; no per-street-name cases).
	if stemIsSTTVariantOfGazetteerStreet(st, scope) {
		score -= 15
	}
	score += len(strings.Fields(st))
	firstIdx = strings.Index(transcriptUpper, addr)
	if firstIdx < 0 {
		h, _ := splitHouseAndStreet(addr)
		firstIdx = strings.Index(transcriptUpper, h+" "+strings.ToUpper(st))
	}
	if firstIdx >= 0 {
		score += 5
	}
	if n, ok := ohioStateRouteNumberInText(normalizeRouteTokens(st)); ok {
		num := strconv.Itoa(n)
		u := normalizeRouteTokens(transcriptUpper)
		if strings.Contains(u, "STATE ROUTE "+num) || strings.Contains(u, " SR "+num) ||
			strings.Contains(u, " ST ROUTE "+num) {
			score += 20
		}
	} else if isGenericStateRoadStem(normalizeRouteTokens(st)) {
		if _, ok := primaryStateRouteInTranscript(transcriptUpper); ok {
			score -= 15
		}
	}
	return score, firstIdx
}

// phoneticGazetteerPreferredOverSTT reports when a transcript candidate street
// is a phonetic/STT variant of the current gazetteer-backed pick and must not
// replace it (raw STT spelling loses to the imported spelling).
func phoneticGazetteerPreferredOverSTT(candidate, current string, scope *ScopeData) bool {
	if scope == nil {
		return false
	}
	_, candSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(candidate)))
	_, curSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(current)))
	if candSt == "" || curSt == "" || strings.EqualFold(candSt, curSt) {
		return false
	}
	// Current pick must be backed by the gazetteer; the candidate must not be.
	if !homonymPinAlignStreetIsExactImport(curSt, scope.KnownStreets) ||
		homonymPinAlignStreetIsExactImport(candSt, scope.KnownStreets) {
		return false
	}
	return StreetNamesSTTMatch(candSt, curSt) ||
		StreetTokensSTTMatch(stripStreetSpaces(streetNameAndSuffixFirst(candSt)),
			stripStreetSpaces(streetNameAndSuffixFirst(curSt)))
}

// stemIsSTTVariantOfGazetteerStreet reports when a street stem is absent from
// the gazetteer but phonetically matches a gazetteer stem — i.e. it looks like
// an STT mishearing of an imported street.
func stemIsSTTVariantOfGazetteerStreet(street string, scope *ScopeData) bool {
	if scope == nil || len(scope.KnownStreets) == 0 {
		return false
	}
	canon := CanonicalStreetName(street)
	stem, _ := streetNameAndSuffix(canon)
	if stem == "" {
		return false
	}
	dir, _, _ := splitStreetParts(canon)
	fuzzyHit := false
	for _, ks := range scope.KnownStreets {
		kcanon := CanonicalStreetName(ks)
		kStem, _ := streetNameAndSuffix(kcanon)
		if kStem == "" {
			continue
		}
		if strings.EqualFold(stem, kStem) {
			return false // exact gazetteer stem — not a mishearing
		}
		// A differing directional makes these distinct streets, not a phonetic
		// mishearing: "WEST 52ND" is not an STT variant of "EAST 52ND".
		if kdir, _, _ := splitStreetParts(kcanon); dir != kdir {
			continue
		}
		if !fuzzyHit && StreetNamesSTTMatch(stem, kStem) {
			fuzzyHit = true
		}
	}
	return fuzzyHit
}

// maybeExpandTruncatedStreetFromTranscript upgrades a single-token street snap
// when dispatch spoke a longer phrase ("256 ROSE" → "256 ROSE GARDEN").
func maybeExpandTruncatedStreetFromTranscript(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	house, street := splitHouseAndStreet(curated.Address)
	if house == "" || street == "" || len(strings.Fields(street)) != 1 {
		return
	}
	cleaned := PreCleanTranscript(transcript)
	for _, a := range extractDispatchAddressesFromTranscript(cleaned) {
		h2, st2 := splitHouseAndStreet(a)
		if h2 == house && st2 != street && strings.HasPrefix(st2, street+" ") {
			if dispatchHouseStreetPhraseInTranscript(cleaned, h2, st2) {
				curated.Address = strings.ToUpper(strings.TrimSpace(a))
				return
			}
		}
	}
	if expanded := spokenStreetPhraseAfterHouse(cleaned, house); expanded != "" && expanded != street {
		curated.Address = house + " " + expanded
	}
}

// isConnectorRoadQualifier reports a trailing cardinal word that distinguishes
// parallel connector roads (NORTH RIDGE ROAD EAST vs WEST), not a quadrant suffix.
func isConnectorRoadQualifier(tok string) bool {
	switch strings.ToUpper(strings.TrimSpace(tok)) {
	case "EAST", "WEST", "NORTH", "SOUTH":
		return true
	}
	return false
}

// trailingConnectorQualifier returns a trailing EAST/WEST/NORTH/SOUTH on a
// street phrase when present ("NORTH RIDGE ROAD EAST" → "EAST").
func trailingConnectorQualifier(street string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(street)))
	if len(fields) < 2 {
		return ""
	}
	last := fields[len(fields)-1]
	if isConnectorRoadQualifier(last) {
		return last
	}
	return ""
}

var directionalGluePrefixes = []string{"NORTH", "SOUTH", "EAST", "WEST"}

// splitDirectionalGlueToken splits STT-glued tokens that begin with a cardinal
// directional ("NORTHRIDGE" → "NORTH RIDGE"). Universal — no per-street tables.
func splitDirectionalGlueToken(tok string) (string, bool) {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	for _, dir := range directionalGluePrefixes {
		if strings.HasPrefix(tok, dir) && len(tok) > len(dir)+3 {
			rest := tok[len(dir):]
			if isAlphaWordToken(rest) {
				return dir + " " + rest, true
			}
		}
	}
	return tok, false
}

// expandGluedStreetTokensInPhrase splits directional glue in each street token.
func expandGluedStreetTokensInPhrase(phrase string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(phrase)))
	if len(fields) == 0 {
		return ""
	}
	var out []string
	for _, f := range fields {
		if split, ok := splitDirectionalGlueToken(f); ok {
			out = append(out, strings.Fields(split)...)
			continue
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

func finalizeSpokenStreetPhrase(words []string) string {
	if len(words) == 0 {
		return ""
	}
	return expandGluedStreetTokensInPhrase(strings.Join(words, " "))
}

// spokenStreetPhraseAfterHouse returns the spaced street words dispatch spoke
// after a house number ("256 ROSE GARDEN"), stopping at narrative tails.
func spokenStreetPhraseAfterHouse(transcript, house string) string {
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
		tok = strings.TrimRight(tok, ".,;:?!")
		flat = append(flat, strings.Fields(strings.ReplaceAll(tok, "-", " "))...)
	}
	var words []string
	sawThoroughfare := false
	for i := 0; i < len(flat); i++ {
		part := flat[i]
		if part == house {
			if len(words) == 0 {
				continue
			}
			// "3802 EAST 57F, 3802 EAST 157TH" — incomplete/garbled first pass;
			// clear and keep scanning for the restated street.
			if spokenStreetPhraseLooksIncomplete(words) {
				words = nil
				sawThoroughfare = false
				continue
			}
			// Same-house restatement with a different street is a dispatcher
			// correction ("1050 SCREEN ROAD 1050 SOUTH GREEN") — keep scanning.
			// Leading directionals (SOUTH/EAST/…) count as part of the next street.
			if i+1 < len(flat) {
				next := strings.TrimRight(flat[i+1], ".,;:?!")
				if next != "" && !isAllDigits(next) && !suffixlessStreetStopwords[next] &&
					!localStreetSuffixes[next] {
					compare := next
					if homonymStreetDirToken(next) && i+2 < len(flat) {
						stem := strings.TrimRight(flat[i+2], ".,;:?!")
						if stem != "" && !isAllDigits(stem) && !suffixlessStreetStopwords[stem] &&
							!localStreetSuffixes[stem] {
							compare = stem
						}
					}
					first := words[0]
					if !strings.EqualFold(compare, first) &&
						!strings.HasPrefix(strings.ToUpper(compare), strings.ToUpper(first)) &&
						!strings.HasPrefix(strings.ToUpper(first), strings.ToUpper(compare)) {
						words = nil
						sawThoroughfare = false
						continue
					}
				}
			}
			return finalizeSpokenStreetPhrase(words)
		}
		if len(words) >= 1 && (words[len(words)-1] == "ROUTE" || words[len(words)-1] == "RTE" || words[len(words)-1] == "RT") &&
			isAllDigits(part) && isShortRouteNumber(part) {
			words = append(words, part)
			return finalizeSpokenStreetPhrase(words)
		}
		// "1717 LONGWOOD, 54 YEAR OLD" — age after a street stem is not part of the name.
		if isAllDigits(part) && spokenDigitIsAgeOrCountTail(flat, i) {
			return finalizeSpokenStreetPhrase(words)
		}
		if isAllDigits(part) && len(words) >= 1 && !sawThoroughfare &&
			!homonymStreetDirToken(words[len(words)-1]) && !localRouteKeywords[words[len(words)-1]] {
			return finalizeSpokenStreetPhrase(words)
		}
		// STT "EAST 57F" before correction "EAST 157TH" — skip the garbled ordinal.
		if isMalformedGridOrdinalToken(part) {
			continue
		}
		if len(part) < 2 || suffixlessStreetStopwords[part] {
			return finalizeSpokenStreetPhrase(words)
		}
		switch part {
		case "BETWEEN", "CROSS", "CROSSES", "CROSSROADS", "CROSSUP", "ACROSS", "FOR", "NEAR", "AT", "AND",
			"ROOM", "APARTMENT", "APT", "UNIT", "IN", "OF", "YEAR", "YEARS", "OLD", "FEMALE", "MALE",
			"FEMALES", "MALES":
			return finalizeSpokenStreetPhrase(words)
		}
		// "9715 CLINTON OVER AT TRIAD…" — OVER AT is a location cue, not a street word.
		if part == "OVER" && i+1 < len(flat) {
			next := strings.TrimRight(flat[i+1], ".,;:?!")
			switch next {
			case "AT", "BY", "NEAR", "THE", "ON", "IN", "TO":
				return finalizeSpokenStreetPhrase(words)
			}
		}
		if sawThoroughfare {
			if isConnectorRoadQualifier(part) {
				words = append(words, part)
				return finalizeSpokenStreetPhrase(words)
			}
			// "SOUTH PARKWAY DRIVE" — PARKWAY matched as a suffix first, but the
			// real thoroughfare type (DRIVE/ROAD/…) follows immediately.
			if localStreetSuffixes[part] {
				words = append(words, part)
			}
			return finalizeSpokenStreetPhrase(words)
		}
		words = append(words, part)
		if localStreetSuffixes[part] || hasStreetSuffix(strings.Join(words, " ")) {
			sawThoroughfare = true
		}
		if len(words) >= 4 && !sawThoroughfare {
			return finalizeSpokenStreetPhrase(words)
		}
	}
	return finalizeSpokenStreetPhrase(words)
}

// isMalformedGridOrdinalToken reports STT junk like "57F" that is not a real
// ordinal street token ("57TH", "52ND").
func isMalformedGridOrdinalToken(tok string) bool {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	if tok == "" || ordinalDigitTokenRE.MatchString(tok) {
		return false
	}
	hasDigit, hasLetter := false, false
	for _, r := range tok {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		default:
			return false
		}
	}
	return hasDigit && hasLetter
}

// spokenStreetPhraseLooksIncomplete reports a partial capture that should not
// win over a later restatement ("EAST" or "EAST 57F" before "EAST 157TH STREET").
func spokenStreetPhraseLooksIncomplete(words []string) bool {
	if len(words) == 0 {
		return true
	}
	if len(words) == 1 && (homonymStreetDirToken(words[0]) || streetDirTokens[canonicalStreetTokens(words[0])]) {
		return true
	}
	if len(words) == 2 && (homonymStreetDirToken(words[0]) || streetDirTokens[canonicalStreetTokens(words[0])]) &&
		isMalformedGridOrdinalToken(words[1]) {
		return true
	}
	return false
}

// spokenDigitIsAgeOrCountTail reports digits that introduce patient age/count
// phrasing ("54 YEAR OLD", "2 HOURS") rather than a grid/route number.
func spokenDigitIsAgeOrCountTail(fields []string, i int) bool {
	if i+1 >= len(fields) {
		return false
	}
	next := strings.TrimRight(fields[i+1], ".,;:?!")
	switch next {
	case "YEAR", "YEARS", "OLD", "YO", "MONTH", "MONTHS", "WEEK", "WEEKS",
		"DAY", "DAYS", "HOUR", "HOURS", "FEMALE", "MALE", "FEMALES", "MALES":
		return true
	}
	if i+2 < len(fields) {
		n2 := strings.TrimRight(fields[i+2], ".,;:?!")
		if next == "YEAR" || next == "YEARS" {
			return n2 == "OLD" || n2 == "FEMALE" || n2 == "MALE" || n2 == "YO"
		}
	}
	return false
}

// trimSpokenStreetPhrase shortens a spoken-street tail to the first complete
// thoroughfare (MORGAN ROAD) before narrative words (CARS ADVISING).
func trimSpokenStreetPhrase(spoken string) string {
	spoken = strings.ToUpper(strings.TrimSpace(spoken))
	if spoken == "" {
		return ""
	}
	var words []string
	sawThoroughfare := false
	for _, tok := range strings.Fields(spoken) {
		tok = strings.TrimRight(tok, ".,;:?!")
		if len(tok) < 2 || suffixlessStreetStopwords[tok] || facilityStreetStop[tok] {
			break
		}
		if sawThoroughfare {
			if isConnectorRoadQualifier(tok) {
				words = append(words, tok)
				return finalizeSpokenStreetPhrase(words)
			}
			if localStreetSuffixes[tok] {
				words = append(words, tok)
			}
			return finalizeSpokenStreetPhrase(words)
		}
		words = append(words, tok)
		if localStreetSuffixes[tok] || hasStreetSuffix(strings.Join(words, " ")) {
			sawThoroughfare = true
		}
	}
	return finalizeSpokenStreetPhrase(words)
}

// facilityStreetStop marks words that begin facility/CAD glue after a street
// stem ("TIBBETTS WICK ASSISTED LIVING", "WALNUT NO CONTACT").
var facilityStreetStop = map[string]bool{
	"ASSISTED": true, "SKILLED": true, "NURSING": true, "SENIOR": true,
	"CONTACT": true, "ATTEMPT": true, "HEALTH": true, "HEALTHCARE": true,
	"ROOM": true, "APARTMENT": true, "APT": true, "UNIT": true,
}

// transcriptExplicitDispatchStreet returns the house+street dispatch clearly
// spoke ("1418 MORGAN ROAD") when it appears verbatim in the transcript.
func transcriptExplicitDispatchStreet(house, transcript string) string {
	house = strings.TrimSpace(house)
	if house == "" || strings.TrimSpace(transcript) == "" {
		return ""
	}
	cleaned := PreCleanTranscript(transcript)
	u := " " + strings.ToUpper(strings.TrimSpace(cleaned)) + " "
	spoken := trimSpokenStreetPhrase(spokenStreetPhraseAfterHouse(strings.TrimSpace(u), house))
	if spoken == "" {
		return ""
	}
	if !dispatchHouseStreetPhraseInTranscript(u, house, spoken) {
		return ""
	}
	return spoken
}

// transcriptOverridesImportStreetSpelling reports when dispatch named a
// different house+street in the transcript than the extracted/import alias
// (1418 MORGAN ROAD vs card 1418 MORIAN ROAD).
func transcriptOverridesImportStreetSpelling(house, addrStreet, transcript string) bool {
	explicit := transcriptExplicitDispatchStreet(house, transcript)
	if explicit == "" {
		return false
	}
	return !strings.EqualFold(CanonicalStreetName(explicit), CanonicalStreetName(addrStreet))
}

// AlignAddressStreetFromTranscript restores the street dispatch spoke after the
// house number when homonym refinement substituted a different thoroughfare
// (2154 HELM ROAD ← ELM homonym; 1004 WILLOW DRIVE ← BOLDE homonym).
func AlignAddressStreetFromTranscript(addr, transcript string) string {
	return alignAddressStreetFromTranscript(addr, transcript, nil)
}

// AlignAddressStreetFromScopedTranscript is AlignAddressStreetFromTranscript with
// gazetteer scope for phonetic corrections (WILD DRIVE → WILLOW DRIVE).
func AlignAddressStreetFromScopedTranscript(addr, transcript string, scope *ScopeData) string {
	return alignAddressStreetFromTranscript(addr, transcript, scope)
}

func alignAddressStreetFromTranscript(addr, transcript string, scope *ScopeData) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || strings.TrimSpace(transcript) == "" {
		return addr
	}
	if addressUsesNumberedRoute(street) {
		return addr
	}
	// A trailing quadrant directional already on the card ("ELM ROAD
	// NORTHEAST") is not something spokenStreetPhraseAfterHouse re-derives —
	// it only tracks single-word EAST/WEST/NORTH/SOUTH connectors, not the
	// compound quadrant forms — so re-deriving the street stem below must not
	// silently drop it. Captured once here and restored on any reconstructed
	// (non-`addr`-passthrough) return below.
	origTrailingDir := trailingRawDirectionalToken(street)
	cleaned := PreCleanTranscript(transcript)
	u := strings.ToUpper(cleaned)
	if route := dispatchRouteStreetAfterHouse(house, u); route != "" {
		return house + " " + route
	}
	spoken := strings.TrimSpace(trimSpokenStreetPhrase(spokenStreetPhraseAfterHouse(u, house)))
	if spoken == "" {
		return addr
	}
	if explicit := transcriptExplicitDispatchStreet(house, transcript); explicit != "" &&
		!strings.EqualFold(CanonicalStreetName(explicit), CanonicalStreetName(street)) {
		spoken = explicit
	}
	// Dispatch often repeats the address, first bare then with the suffix
	// ("5229 MAHONING, 5229 MAHONING AVENUE"). If spokenStreetPhraseAfterHouse
	// latched onto the earlier bare repetition, `spoken` is just the stem with
	// no thoroughfare type. When that bare stem already equals the stem of the
	// address's current street (i.e. `street` already reflects a more complete
	// resolution of the exact same name — e.g. it was correctly snapped to an
	// exact "MAHONING AVENUE" gazetteer entry earlier), re-deriving from the
	// bare stem alone is strictly less informed and must not overwrite it with
	// a different, arbitrarily-picked homonym (MAHONING COURT, MAHONING
	// ROAD, ...) that the transcript never actually distinguished.
	// Same bare-repetition case as above, but the earlier mention was only
	// the FIRST WORD of a multi-word name ("5524 PRITCHARD..." before "...OLD
	// TOWN ROAD" is spoken) rather than the whole stem — `spoken` capturing
	// just "PRITCHARD" must not be treated as a distinguishing signal for a
	// *different*, shorter homonym ("PRITCHARD ROAD") when the address
	// already carries the fuller, correctly-resolved name.
	if stemName, _ := streetNameAndSuffix(street); stemName != "" {
		if strings.EqualFold(stemName, spoken) {
			return addr
		}
		if stemFields := strings.Fields(stemName); len(stemFields) > 1 && strings.EqualFold(stemFields[0], spoken) {
			return addr
		}
	}
	if scope != nil && len(scope.KnownStreets) > 0 {
		if ks := bestKnownStreetForSpokenPhrase(spoken, scope.KnownStreets); ks != "" {
			if strings.EqualFold(CanonicalStreetName(ks), CanonicalStreetName(street)) {
				// The spoken phrase resolves to the street already on the card
				// (STT "VIANNA" → gazetteer VIENNA AVENUE): the card spelling is
				// the corrected one — never restore the mishearing.
				return addr
			}
			return house + " " + ks
		}
	}
	if strings.EqualFold(CanonicalStreetName(spoken), CanonicalStreetName(street)) {
		return addr
	}
	spokenQual := trailingConnectorQualifier(spoken)
	addrQual := trailingConnectorQualifier(street)
	if spokenQual != "" && spokenQual != addrQual {
		// Spoken connector suffix (ROAD EAST) must not be dropped for a homonym
		// that only matched the stem (ROAD without EAST).
	} else {
		spokenCore := stripStreetSpaces(streetNameAndSuffixFirst(spoken))
		addrCore := stripStreetSpaces(streetNameAndSuffixFirst(street))
		if spokenCore != "" && spokenCore == addrCore {
			return addr
		}
	}
	if !transcriptNamesHouseStreet(u, house, spoken) {
		return addr
	}
	if importStreetShouldKeepAddrSpelling(house, spoken, street, transcript, scope) {
		return addr
	}
	restored := spoken
	if scope != nil {
		if alt := phoneticGazetteerStreetCorrection(house, spoken, scope.KnownStreets); alt != "" {
			restored = alt
		}
	}
	if hasStreetSuffix(restored) {
		return house + " " + withPreservedTrailingDirectional(restored, origTrailingDir)
	}
	_, haveType := StreetCoreTypeKey(CanonicalStreetName(street))
	if haveType != "" {
		return house + " " + withPreservedTrailingDirectional(restored+" "+haveType, origTrailingDir)
	}
	return house + " " + withPreservedTrailingDirectional(restored, origTrailingDir)
}

// trailingRawDirectionalToken returns the last word of street verbatim
// ("NORTHEAST", "NE", ...) when it's a quadrant directional, or "" otherwise.
func trailingRawDirectionalToken(street string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(street)))
	if len(fields) < 2 {
		return ""
	}
	last := fields[len(fields)-1]
	if directionalWordsCorrection[last] {
		return last
	}
	return ""
}

// withPreservedTrailingDirectional re-appends origDir to newStreet when
// newStreet doesn't already carry its own trailing directional, so realigning
// a street's stem/suffix from the transcript doesn't lose a quadrant
// directional that was already confirmed on the card.
func withPreservedTrailingDirectional(newStreet, origDir string) string {
	if origDir == "" || trailingRawDirectionalToken(newStreet) != "" {
		return newStreet
	}
	return newStreet + " " + origDir
}

// bestKnownStreetForSpokenPhrase picks a gazetteer street matching the spoken
// dispatch phrase, preferring trailing connector qualifiers (ROAD EAST vs ROAD).
func bestKnownStreetForSpokenPhrase(spoken string, knownStreets []string) string {
	spoken = strings.ToUpper(strings.TrimSpace(spoken))
	if spoken == "" || len(knownStreets) == 0 {
		return ""
	}
	for _, ks := range knownStreets {
		if strings.EqualFold(strings.TrimSpace(ks), spoken) {
			return strings.ToUpper(strings.TrimSpace(ks))
		}
	}
	qual := trailingConnectorQualifier(spoken)
	if qual != "" {
		if pick := pickKnownStreetWithTrailingQualifier(spoken, qual, knownStreets); pick != "" {
			return pick
		}
	}
	if match, ok := bestCollapsedCoreStreetMatch(spoken, knownStreets); ok {
		if qual != "" && !strings.HasSuffix(strings.ToUpper(strings.TrimSpace(match)), " "+qual) {
			if pick := pickKnownStreetWithTrailingQualifier(spoken, qual, knownStreets); pick != "" {
				return pick
			}
		}
		return match
	}
	return ""
}

func pickKnownStreetWithTrailingQualifier(spoken, qual string, known []string) string {
	spokenCore := stripStreetSpaces(streetNameAndSuffixFirst(spoken))
	qual = strings.ToUpper(strings.TrimSpace(qual))
	best := ""
	// Start at 0 so a candidate that shares only the trailing directional but
	// has no name-core match (score 0) can never win. Sharing "NORTH" is not a
	// street match — "ADAMS AVENUE, NORTH" must not become "CAMPING ACCESS LOOP
	// NORTH" just because it is the only gazetteer street ending in NORTH.
	bestScore := 0
	for _, ks := range known {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		if !strings.HasSuffix(ku, " "+qual) {
			continue
		}
		ksCore := stripStreetSpaces(streetNameAndSuffixFirst(ku))
		score := 0
		switch {
		case spokenCore != "" && spokenCore == ksCore:
			score = 100
		case spokenCore != "" && gluedSpokenStemAlignsWithGazetteer(spokenCore, ksCore):
			score = 80
		default:
			if match, ok := bestCollapsedCoreStreetMatch(spoken, []string{ks}); ok && strings.EqualFold(match, ks) {
				score = 60
			}
		}
		if score > bestScore {
			bestScore = score
			best = ku
		}
	}
	return best
}

// phoneticGazetteerStreetCorrection maps an STT-mangled spoken street to the
// single gazetteer street it phonetically matches on the same thoroughfare
// suffix. Returns "" when the spoken stem is already a gazetteer stem or the
// match is ambiguous.
func phoneticGazetteerStreetCorrection(house, spoken string, known []string) string {
	stem, suffix := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(spoken)))
	if stem == "" || suffix == "" {
		return ""
	}
	var match string
	for _, ks := range known {
		kStem, kSuffix := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(ks)))
		if kStem == "" {
			continue
		}
		if strings.EqualFold(stem, kStem) {
			return "" // spoken stem exists in gazetteer — nothing to correct
		}
		if kSuffix != suffix {
			continue
		}
		if StreetNamesSTTMatch(stem, kStem) || StreetTokensSTTMatch(stripStreetSpaces(stem), stripStreetSpaces(kStem)) {
			if match != "" && !strings.EqualFold(match, ks) {
				return "" // ambiguous across multiple gazetteer streets
			}
			match = ks
		}
	}
	return match
}

func importStreetShouldKeepAddrSpelling(house, spoken, addrStreet, transcript string, scope *ScopeData) bool {
	if transcriptOverridesImportStreetSpelling(house, addrStreet, transcript) {
		return false
	}
	orig := strings.ToUpper(strings.TrimSpace(spoken))
	spoken = trimSpokenStreetPhrase(orig)
	if spoken == "" {
		spoken = orig
	}
	spokenStem, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(spoken)))
	addrStem, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(addrStreet)))
	if spokenStem == "" || addrStem == "" {
		return false
	}
	if scope == nil || strings.EqualFold(spokenStem, addrStem) {
		return false
	}
	if !homonymPinAlignStreetIsExactImport(addrStreet, scope.KnownStreets) {
		return false
	}
	return StreetNamesSTTMatch(spoken, addrStreet) ||
		StreetTokensSTTMatch(stripStreetSpaces(streetNameAndSuffixFirst(spoken)),
			stripStreetSpaces(streetNameAndSuffixFirst(addrStreet)))
}

// gazetteerSpellingShouldBeatTranscriptHomonym reports when the card should keep
// an imported street name instead of a transcript STT homonym (VIALL over VILE).
func gazetteerSpellingShouldBeatTranscriptHomonym(current, candidate string, scope *ScopeData) bool {
	if scope == nil {
		return false
	}
	_, curSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(current)))
	_, candSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(candidate)))
	if curSt == "" || candSt == "" {
		return false
	}
	if !homonymPinAlignStreetIsExactImport(curSt, scope.KnownStreets) {
		return false
	}
	return StreetNamesSTTMatch(candSt, curSt) || streetsAreCoverageHomonyms(candSt, curSt)
}

func transcriptNamesHouseStreet(u, house, spoken string) bool {
	house = strings.ToUpper(strings.TrimSpace(house))
	spoken = strings.ToUpper(strings.TrimSpace(spoken))
	if house == "" || spoken == "" {
		return false
	}
	if dispatchHouseStreetPhraseInTranscript(u, house, spoken) {
		return true
	}
	for _, sep := range []string{", ", ",", " "} {
		if strings.Contains(u, house+sep+spoken) {
			return true
		}
	}
	return false
}
