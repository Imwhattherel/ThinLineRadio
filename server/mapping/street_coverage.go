// Copyright (C) 2025 Thinline Dynamic Solutions
//
// street_coverage.go — transcript/gazetteer street-name helpers used while
// identifying an address before Nominatim. Local OSM geometry homonym
// disambiguation was removed (StreetGeometry is always empty).

package mapping

import (
	"strconv"
	"strings"
)

var spokenTrailingDirectionals = map[string]string{
	" NORTHWEST": "NW", " NORTH WEST": "NW", " NW": "NW",
	" NORTHEAST": "NE", " NORTH EAST": "NE", " NE": "NE",
	" SOUTHWEST": "SW", " SOUTH WEST": "SW", " SW": "SW",
	" SOUTHEAST": "SE", " SOUTH EAST": "SE", " SE": "SE",
	" NORTH": "N", " SOUTH": "S", " EAST": "E", " WEST": "W",
}

// appendSpokenTrailingDirectional adds a trailing directional when dispatch
// spoke one after the street but extraction dropped it ("822 TODD AVENUE NW").
func appendSpokenTrailingDirectional(addr, transcript string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return addr
	}
	canon := CanonicalStreetName(street)
	if _, _, _ = splitStreetParts(canon); hasTrailingDirectionalInCanonical(canon) {
		return addr
	}
	u := strings.ToUpper(transcript)
	stU := strings.ToUpper(street)
	if qual := trailingConnectorQualifier(spokenStreetPhraseAfterHouse(u, house)); qual != "" &&
		trailingConnectorQualifier(street) == "" {
		return house + " " + stU + " " + qual
	}
	for spoken, dir := range spokenTrailingDirectionals {
		if transcriptContainsHouseStreetFragment(u, house, stU+spoken) {
			tail := strings.TrimSpace(spoken)
			if tail != "" {
				return house + " " + stU + " " + tail
			}
			return house + " " + stU + " " + dir
		}
	}
	return addr
}

// alignAddressLeadingQualifiersFromTranscript restores a spoken leading street
// qualifier when extraction dropped or mis-assigned it ("882 EAST LIBERTY" ←
// transcript "882 WEST LIBERTY STREET").
func alignAddressLeadingQualifiersFromTranscript(addr, transcript string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return addr
	}
	u := strings.ToUpper(transcript)
	name, suf := streetNameAndSuffix(street)
	if name == "" {
		return addr
	}
	stem := strings.Fields(name)
	for len(stem) > 0 && homonymStreetDirToken(stem[0]) {
		stem = stem[1:]
	}
	if len(stem) == 0 {
		return addr
	}
	head := stem[0]
	for _, qual := range []string{"WEST", "NORTH", "SOUTH", "EAST"} {
		if !transcriptLeadingQualifierMatches(u, house, qual+" "+head) {
			continue
		}
		rebuilt := qual + " " + strings.Join(stem, " ")
		if suf != "" {
			rebuilt += " " + suf
		}
		return house + " " + rebuilt
	}
	return addr
}

func transcriptLeadingQualifierMatches(u, house, fragment string) bool {
	if transcriptContainsHouseStreetFragment(u, house, fragment) {
		return true
	}
	// STT often repeats the house number: "882-882, WEST LIBERTY".
	dup := house + "-" + house
	if idx := strings.Index(u, dup); idx >= 0 && strings.Contains(u[idx:], fragment) {
		return true
	}
	return false
}

func streetLeadingQualifier(streetName string) string {
	for _, q := range []string{"WEST", "NORTH", "SOUTH", "EAST"} {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(streetName)), q+" ") {
			return q
		}
	}
	return ""
}

func streetHasLeadingQualifier(streetName, qual string) bool {
	u := strings.ToUpper(strings.TrimSpace(streetName))
	if strings.HasPrefix(u, qual+" ") {
		return true
	}
	if qual == "WEST" && strings.HasPrefix(u, "W ") {
		return true
	}
	return false
}

func hasTrailingDirectionalInCanonical(canon string) bool {
	fields := strings.Fields(canon)
	if len(fields) == 0 {
		return false
	}
	return streetDirTokens[fields[len(fields)-1]]
}

func transcriptContainsHouseStreetFragment(u, house, fragment string) bool {
	fragment = strings.ToUpper(strings.TrimSpace(fragment))
	if fragment == "" {
		return false
	}
	for _, sep := range []string{" ", ", ", ","} {
		needle := house + sep + fragment
		idx := strings.Index(u, needle)
		if idx < 0 {
			continue
		}
		// Require a real word boundary right after the fragment so a shorter
		// directional ("NORTH") doesn't falsely match as a prefix of a longer
		// one actually spoken ("NORTHEAST") — plain Contains would let "3401
		// ELM ROAD NORTH" match inside "3401 ELM ROAD NORTHEAST, STILL...".
		if end := idx + len(needle); end < len(u) {
			if c := u[end]; (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				continue
			}
		}
		return true
	}
	return false
}

func homonymStreetStemTokens(street string) []string {
	street = strings.ToUpper(strings.TrimSpace(street))
	if street == "" {
		return nil
	}
	if _, st := splitHouseAndStreet(street); st != "" {
		street = st
	}
	name, _ := streetNameAndSuffix(street)
	if name == "" {
		name = street
	}
	fields := strings.Fields(name)
	for len(fields) > 0 && homonymStreetDirToken(fields[0]) {
		fields = fields[1:]
	}
	for len(fields) > 1 && localStreetSuffixes[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	return fields
}

func homonymStreetDirToken(tok string) bool {
	return directionalWordsCorrection[tok]
}

func homonymSwapContradictsTranscript(addr, pick, transcript string) bool {
	if strings.TrimSpace(transcript) == "" {
		return false
	}
	if homonymSwapContradictsSpokenDirection(addr, pick, transcript) {
		return true
	}
	if homonymSwapAddsUnspokenLeadingDirection(addr, pick, transcript) {
		return true
	}
	if homonymSwapInsertsUnspokenStem(addr, pick, transcript) {
		return true
	}
	_, addrSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	pick = strings.ToUpper(strings.TrimSpace(pick))
	if addrSt == "" || pick == "" {
		return false
	}
	if addrName, _ := streetNameAndSuffix(addrSt); addrName != "" {
		if pickName, _ := streetNameAndSuffix(pick); pickName != "" {
			addrCollapsed := stripStreetSpaces(addrName)
			pickCollapsed := stripStreetSpaces(pickName)
			if addrCollapsed != "" && pickCollapsed != "" &&
				(StreetTokensSTTMatch(addrCollapsed, pickCollapsed) ||
					ScoreStreetSTTCoreMatch(addrCollapsed, pickCollapsed, nil) >= sttMatchScoreThreshold) {
				return false
			}
		}
	}
	addrStem := homonymStreetStemTokens(addrSt)
	pickStem := homonymStreetStemTokens(pick)
	if len(addrStem) == 0 || len(pickStem) == 0 {
		return false
	}
	addrCollapsed := stripStreetSpaces(strings.Join(addrStem, ""))
	pickCollapsed := stripStreetSpaces(strings.Join(pickStem, ""))
	if addrCollapsed != "" && pickCollapsed != "" && StreetTokensSTTMatch(addrCollapsed, pickCollapsed) {
		return false
	}
	padded := " " + strings.ToUpper(transcript) + " "
	_, addrHouse := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	for _, pw := range pickStem {
		if isStreetQuadrantSuffix(pw) || localStreetSuffixes[pw] {
			continue
		}
		if streetDirTokens[pw] || directionalWordsCorrection[pw] {
			head := ""
			if len(pickStem) > 0 {
				for _, tok := range pickStem {
					if !streetDirTokens[tok] && !directionalWordsCorrection[tok] &&
						!localStreetSuffixes[tok] && !isStreetQuadrantSuffix(tok) {
						head = tok
						break
					}
				}
			}
			if len(pickStem) > 1 && pickStem[len(pickStem)-1] == pw {
				prefix := strings.Join(pickStem[:len(pickStem)-1], " ")
				addrPrefix := strings.Join(addrStem, " ")
				if prefix == addrPrefix || strings.HasPrefix(prefix, addrPrefix+" ") || strings.HasPrefix(addrPrefix, prefix+" ") {
					continue
				}
			}
			if head != "" && (spokenDirectionForHouseStreetStem(padded, addrHouse, head, pw) ||
				transcriptLeadingQualifierMatches(strings.TrimSpace(padded), addrHouse, pw+" "+head)) {
				continue
			}
			if spokenTrailingDirectionForHouseStem(padded, addrHouse, head, pw) {
				continue
			}
			return true
		} else if wordInPaddedTranscript(padded, pw) {
			continue
		}
		for _, aw := range addrStem {
			if aw != pw && wordInPaddedTranscript(padded, aw) {
				return true
			}
		}
	}
	// Same stem with only an unspoken quadrant/thoroughfare extension is not a
	// contradictory homonym swap (NORTH RIVER ROAD → NORTH RIVER ROAD NE).
	if len(addrStem) > 0 && len(pickStem) >= len(addrStem) {
		match := true
		for i, aw := range addrStem {
			if i >= len(pickStem) || pickStem[i] != aw {
				match = false
				break
			}
		}
		if match {
			return false
		}
	}
	return false
}

// homonymSwapInsertsUnspokenStem blocks swaps that insert a directional or
// other qualifier the dispatcher did not say (SOUTH AVENUE → SOUTH WEST AVENUE).
func homonymSwapInsertsUnspokenStem(addr, pick, transcript string) bool {
	_, addrSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	pick = strings.ToUpper(strings.TrimSpace(pick))
	if addrSt == "" || pick == "" || strings.EqualFold(addrSt, pick) {
		return false
	}
	if strings.Contains(strings.ToUpper(transcript), pick) {
		return false
	}
	addrToks := map[string]bool{}
	for _, t := range strings.Fields(addrSt) {
		if localStreetSuffixes[t] || isStreetQuadrantSuffix(t) {
			continue
		}
		addrToks[t] = true
	}
	for _, t := range strings.Fields(pick) {
		if localStreetSuffixes[t] || isStreetQuadrantSuffix(t) || addrToks[t] {
			continue
		}
		if streetDirTokens[t] || directionalWordsCorrection[t] {
			return true
		}
	}
	return false
}

// homonymSwapContradictsSpokenDirection blocks homonym swaps that replace a
// spoken leading or trailing directional with a conflicting one (WEST LIBERTY
// must not become EAST LIBERTY).
func homonymSwapContradictsSpokenDirection(addr, pick, transcript string) bool {
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	pick = strings.ToUpper(strings.TrimSpace(pick))
	if pick == "" {
		return false
	}
	pickName, _ := streetNameAndSuffix(pick)
	if pickName == "" {
		pickName = pick
	}
	pickLead := streetLeadingQualifier(pickName)
	stem := homonymStreetStemTokens(pickName)
	if len(stem) == 0 {
		return false
	}
	head := stem[0]
	u := strings.ToUpper(transcript)
	_, addrSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	addrName, _ := streetNameAndSuffix(addrSt)
	addrLead := streetLeadingQualifier(addrName)
	if pickLead != "" {
		if spokenTrailingDirectionForHouseStem(u, house, head, pickLead) {
			return true
		}
		if pickLead != addrLead &&
			!spokenDirectionForHouseStreetStem(u, house, head, pickLead) &&
			!transcriptLeadingQualifierMatches(u, house, pickLead+" "+head) {
			return true
		}
	}
	for _, qual := range []string{"WEST", "NORTH", "SOUTH", "EAST"} {
		if !spokenDirectionForHouseStreetStem(u, house, head, qual) {
			continue
		}
		if pickLead != "" && !streetDirectionalsCompatible(qual, pickLead) {
			return true
		}
	}
	return false
}

func homonymSwapAddsUnspokenLeadingDirection(addr, pick, transcript string) bool {
	house, addrSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	pickSt := strings.ToUpper(strings.TrimSpace(pick))
	if _, st := splitHouseAndStreet(pickSt); st != "" {
		pickSt = st
	}
	pickName, _ := streetNameAndSuffix(pickSt)
	pickLead := streetLeadingQualifier(pickName)
	if pickLead == "" {
		return false
	}
	addrName, _ := streetNameAndSuffix(addrSt)
	if streetLeadingQualifier(addrName) == pickLead {
		return false
	}
	addrStem := homonymStreetStemTokens(addrSt)
	pickStem := homonymStreetStemTokens(pickSt)
	if len(addrStem) == 0 || len(pickStem) == 0 {
		return false
	}
	addrCollapsed := stripStreetSpaces(strings.Join(addrStem, ""))
	pickCollapsed := stripStreetSpaces(strings.Join(pickStem, ""))
	if addrCollapsed == "" || pickCollapsed == "" ||
		(!StreetTokensSTTMatch(addrCollapsed, pickCollapsed) && addrCollapsed != pickCollapsed) {
		return false
	}
	u := strings.ToUpper(transcript)
	head := pickStem[0]
	if spokenDirectionForHouseStreetStem(u, house, head, pickLead) ||
		transcriptLeadingQualifierMatches(u, house, pickLead+" "+head) ||
		spokenTrailingDirectionForHouseStem(u, house, head, pickLead) {
		return false
	}
	return true
}

func spokenTrailingDirectionForHouseStem(u, house, stemHead, qual string) bool {
	if house == "" || stemHead == "" || qual == "" {
		return false
	}
	for _, suf := range []string{"STREET", "ST", "ROAD", "RD", "AVENUE", "AVE", "BOULEVARD", "BLVD", "DRIVE", "DR", "LANE", "LN", "COURT", "CT"} {
		if strings.Contains(u, house+" "+stemHead+" "+suf+" "+qual) ||
			strings.Contains(u, stemHead+" "+suf+" "+qual) {
			return true
		}
	}
	return false
}

func spokenDirectionForHouseStreetStem(u, house, stemHead, qual string) bool {
	if transcriptLeadingQualifierMatches(u, house, qual+" "+stemHead) {
		return true
	}
	for _, suf := range []string{"STREET", "ST", "ROAD", "RD", "AVENUE", "AVE", "BOULEVARD", "BLVD", "DRIVE", "DR", "LANE", "LN", "COURT", "CT"} {
		if strings.Contains(u, house+" "+stemHead+" "+suf+" "+qual) ||
			strings.Contains(u, stemHead+" "+suf+" "+qual) {
			return true
		}
	}
	if strings.Contains(u, house+" "+stemHead+" "+qual) {
		return true
	}
	return false
}

func primaryStateRouteInTranscript(transcript string) (int, bool) {
	u := normalizeRouteTokens(strings.ToUpper(strings.TrimSpace(transcript)))
	for _, m := range transcriptHouseStateRouteRE.FindAllStringSubmatch(u, -1) {
		if len(m) >= 3 {
			n, err := strconv.Atoi(strings.TrimSpace(m[2]))
			if err == nil && n > 0 {
				return n, true
			}
		}
	}
	for _, m := range stateRouteRE.FindAllStringSubmatch(u, -1) {
		if len(m) >= 2 {
			n, err := strconv.Atoi(strings.TrimSpace(m[1]))
			if err == nil && n > 0 {
				return n, true
			}
		}
	}
	for _, idx := range localBareNumberedRouteRE.FindAllStringSubmatchIndex(u, -1) {
		if len(idx) < 4 {
			continue
		}
		if idx[0] >= 3 && strings.HasSuffix(strings.TrimSpace(u[max(0, idx[0]-3):idx[0]]), "EN") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(u[idx[2]:idx[3]]))
		if err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

func isGenericStateRoadStem(street string) bool {
	street = strings.ToUpper(strings.TrimSpace(street))
	if _, ok := ohioStateRouteNumberInText(normalizeRouteTokens(street)); ok {
		return false
	}
	return strings.Contains(street, "STATE ROAD") || strings.Contains(street, "STATE RD")
}

func homonymPinAlignStreetIsExactImport(street string, knownStreets []string) bool {
	street = strings.ToUpper(strings.TrimSpace(street))
	for _, ks := range knownStreets {
		if strings.EqualFold(strings.TrimSpace(ks), street) {
			return true
		}
	}
	return false
}

func streetNameAndSuffixFirst(street string) string {
	name, _ := streetNameAndSuffix(street)
	if name != "" {
		return name
	}
	return street
}

// streetLeadingNameToken returns the first significant word of a street name.
func streetLeadingNameToken(name string) string {
	for _, w := range strings.Fields(strings.ToUpper(strings.TrimSpace(name))) {
		if len(w) >= 3 && !streetDirTokens[w] && !localStreetSuffixes[w] {
			return w
		}
	}
	return ""
}

func streetsAreCoverageHomonyms(a, b string) bool {
	ca, cb := CanonicalStreetName(a), CanonicalStreetName(b)
	if ca == "" || cb == "" {
		return false
	}
	if ca == cb {
		return true
	}
	aName, aSuffix := streetNameAndSuffix(ca)
	bName, bSuffix := streetNameAndSuffix(cb)
	if streetLeadingTokensAreSTTHomonyms(aName, aSuffix, bName, bSuffix) {
		return true
	}
	aCore, aType := StreetCoreTypeKey(ca)
	bCore, bType := StreetCoreTypeKey(cb)
	if aType != "" && bType != "" && aType != bType {
		return false
	}
	if aCore == bCore {
		return true
	}
	if len(aCore) < 3 || len(bCore) < 3 {
		return false
	}
	if levenshtein(aCore, bCore) <= 1 && StreetTokensSTTMatch(aCore, bCore) {
		return true
	}
	// Glued multi-word cores often sit at lev≥2 for a single-token STT
	// slip (DURSTCLACK↔DURSTCLAGG). Fall back to per-token matching.
	return streetWordTokensSTTMatch(ca, cb)
}

func streetNameWordTokens(name string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToUpper(strings.TrimSpace(name))) {
		if streetDirTokens[w] || localStreetSuffixes[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// streetLeadingTokensAreSTTHomonyms reports single-token STT swaps on the same
// thoroughfare type (PEPPER AVENUE ↔ PEFFER AVENUE SOUTHEAST).
func streetLeadingTokensAreSTTHomonyms(aName, aSuffix, bName, bSuffix string) bool {
	aLead := streetLeadingNameToken(aName)
	bLead := streetLeadingNameToken(bName)
	if aLead == "" || bLead == "" || aLead == bLead {
		return false
	}
	if len(aLead) < 4 || len(bLead) < 4 {
		return false
	}
	// A single-token STT swap only makes sense when the two names share the same
	// word structure and every name token except the swapped lead is identical.
	// Comparing only the leading token declares a two-word name a homonym of a
	// one-word name — "CARRIAGE HILL" must not match "CRAIG" by ignoring HILL.
	aWords := streetNameWordTokens(aName)
	bWords := streetNameWordTokens(bName)
	if len(aWords) != len(bWords) || len(aWords) == 0 {
		return false
	}
	for i := 1; i < len(aWords); i++ {
		if aWords[i] != bWords[i] {
			return false
		}
	}
	if !streetSuffixesCompatible(aSuffix, bSuffix) {
		return false
	}
	return StreetTokensSTTMatch(aLead, bLead)
}

func streetsShareLeadingNameToken(a, b string) bool {
	aName, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(a)))
	bName, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(b)))
	aTok := streetLeadingNameToken(aName)
	bTok := streetLeadingNameToken(bName)
	return aTok != "" && aTok == bTok && len(aTok) >= 5
}
