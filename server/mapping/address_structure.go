// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_structure.go — universal structural plausibility rules for extracted
// addresses. No street, city, or place names appear here; every rule is about
// the shape of the text (digit counts, function words, sentence punctuation).

package mapping

import "strings"

// addressFunctionWords are English function words that cannot stand alone as a
// street stem and cannot end one ("11 ON WAY", "1522 BORN TO ST").
var addressFunctionWords = map[string]bool{
	"ON": true, "THE": true, "TO": true, "OF": true, "IN": true, "AT": true,
	"A": true, "AN": true, "BE": true, "IS": true, "WAS": true, "ARE": true,
	"OR": true, "BUT": true, "IT": true, "THAT": true, "THIS": true,
	"WITH": true, "FROM": true, "IF": true, "SO": true, "AS": true,
}

// addressFillerWords are narrative words that can never be part of a street
// name between its core and suffix ("EAST 112TH IF POSSIBLE ST").
var addressFillerWords = map[string]bool{
	"IF": true, "HAVE": true, "HAS": true, "HAD": true, "POSSIBLE": true,
	"POSSIBLY": true, "PROBABLY": true, "MAYBE": true, "PLEASE": true,
	"COPY": true, "OKAY": true, "OK": true,
}

// tokenIsNarrativeContraction reports first/second-person and negation
// contractions ("I'M", "WE'RE", "DON'T") that mark sentence narrative, never
// street names. Possessives ("CAPTAIN'S") are not matched.
func tokenIsNarrativeContraction(tok string) bool {
	tok = strings.ToUpper(strings.Trim(tok, ".,;:"))
	for _, suf := range []string{"'M", "'LL", "'RE", "'VE", "N'T", "I'D"} {
		if strings.HasSuffix(tok, suf) {
			return true
		}
	}
	return false
}

// AddressStructurallyImplausible rejects extracted addresses violating
// universal structure rules:
//   - house numbers longer than five digits (plate/serial readbacks)
//   - street text containing sentence punctuation or narrative contractions
//   - street stems that are entirely function words or end in one
//   - bare all-digit stems with a generic suffix but no ordinal marker ("82 CT")
func AddressStructurallyImplausible(addr string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house != "" {
		digits := 0
		for _, r := range house {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits > 5 {
			return true
		}
	}
	street = strings.TrimSpace(street)
	if street == "" {
		return false
	}
	if strings.ContainsAny(street, "?!") {
		return true
	}
	for _, tok := range strings.Fields(street) {
		if tokenIsNarrativeContraction(tok) {
			return true
		}
	}
	stem, suffix := streetNameAndSuffix(street)
	fields := strings.Fields(strings.TrimSpace(stem))
	if len(fields) == 0 {
		return false
	}
	allFunction := true
	for _, f := range fields {
		if !addressFunctionWords[f] {
			allFunction = false
			break
		}
	}
	if allFunction {
		return true
	}
	if addressFunctionWords[fields[len(fields)-1]] {
		return true
	}
	if suffix != "" && len(fields) == 1 && isAllDigits(fields[0]) && !isOrdinalStreetToken(fields[0]) {
		return true
	}
	return false
}

// stripOrdinalStreetFiller drops narrative filler tokens that STT wedged
// between an ordinal street core and its thoroughfare suffix
// ("EAST 112TH IF POSSIBLE ST" → "EAST 112TH ST").
func stripOrdinalStreetFiller(addr string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(addr)))
	ordIdx := -1
	for i, f := range fields {
		if isOrdinalStreetToken(f) {
			ordIdx = i
			break
		}
	}
	if ordIdx < 0 || ordIdx >= len(fields)-1 {
		return addr
	}
	kept := fields[:ordIdx+1]
	changed := false
	for _, f := range fields[ordIdx+1:] {
		if addressFillerWords[f] || addressFunctionWords[f] {
			changed = true
			continue
		}
		kept = append(kept, f)
	}
	if !changed {
		return addr
	}
	return strings.Join(kept, " ")
}

// dedupeTrailingStreetSuffix collapses a duplicated thoroughfare suffix
// ("SOUTH AVENUE AVE" → "SOUTH AVENUE") produced by suffix-append passes.
func dedupeTrailingStreetSuffix(addr string) string {
	fields := strings.Fields(strings.TrimSpace(addr))
	if len(fields) < 3 {
		return addr
	}
	last := strings.ToUpper(fields[len(fields)-1])
	prev := strings.ToUpper(fields[len(fields)-2])
	if localStreetSuffixes[last] && localStreetSuffixes[prev] &&
		canonicalStreetTokens(last) == canonicalStreetTokens(prev) {
		return strings.Join(fields[:len(fields)-1], " ")
	}
	return addr
}
