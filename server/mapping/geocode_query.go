// Copyright (C) 2025 Thinline Dynamic Solutions
//
// geocode_query.go — normalize dispatch address phrases for local extract /
// card matching. Multi-line Nominatim /search query fan-out lived here and
// was removed; live pins use POST /transcript on nominatim-gateway.

package mapping

import (
	"regexp"
	"strconv"
	"strings"
)

var stateRouteRE = regexp.MustCompile(`(?i)\b(?:STATE\s+(?:ROUTE|RT|ROAD|HWY)|STATE\s+RTE|SR)\s*-?\s*(\d{1,4})\b`)

var geocodeStreetSuffixes = map[string]bool{
	"ST": true, "STREET": true, "AVE": true, "AVENUE": true, "RD": true, "ROAD": true,
	"DR": true, "DRIVE": true, "LN": true, "LANE": true, "CT": true, "COURT": true,
	"TRL": true, "TRAIL": true, "BLVD": true, "WAY": true, "PL": true, "PLACE": true,
	"PT": true, "POINT": true, "CIR": true, "CIRCLE": true, "PKWY": true, "HWY": true, "HIGHWAY": true,
}

func splitAtCityPrefix(street string) (city, rest string, ok bool) {
	s := strings.TrimSpace(street)
	if !strings.HasPrefix(strings.ToUpper(s), "AT ") {
		return "", s, false
	}
	tokens := strings.Fields(strings.TrimSpace(s[3:]))
	if len(tokens) < 2 {
		return "", s, false
	}
	for k := len(tokens) - 1; k >= 1; k-- {
		streetPart := strings.Join(tokens[k:], " ")
		if streetPartLooksGeocodable(streetPart) && geocodeStreetMinTokens(streetPart) {
			return strings.Join(tokens[:k], " "), streetPart, true
		}
	}
	return "", s, false
}

func geocodeStreetMinTokens(street string) bool {
	fields := strings.Fields(strings.TrimSpace(street))
	if len(fields) >= 2 {
		return true
	}
	return len(fields) == 1 && stateRouteRE.MatchString(strings.ToUpper(street))
}

func streetPartLooksGeocodable(street string) bool {
	u := strings.ToUpper(strings.TrimSpace(street))
	if u == "" {
		return false
	}
	if stateRouteRE.MatchString(u) {
		return true
	}
	fields := strings.Fields(u)
	if len(fields) == 0 {
		return false
	}
	if geocodeStreetSuffixes[fields[len(fields)-1]] {
		return true
	}
	if _, err := strconv.Atoi(fields[0]); err == nil && len(fields) >= 2 {
		return true
	}
	return false
}

// CorrectAddressPhraseASR applies universal ASR normalization to a full address.
func CorrectAddressPhraseASR(addr string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if street == "" {
		return normalizeSpokenOrdinalTokens(strings.TrimSpace(addr))
	}
	street = normalizeSpokenOrdinalTokens(street)
	if house != "" {
		return strings.TrimSpace(house + " " + street)
	}
	return strings.TrimSpace(street)
}

// CleanStreetForGeocode strips dispatch junk and normalizes state route references
// before street-index / TIGER lookup.
func CleanStreetForGeocode(street string, geo *GeoOptions) string {
	street = strings.TrimSpace(street)
	if street == "" {
		return ""
	}
	if _, rest, ok := splitAtCityPrefix(street); ok {
		street = rest
	}
	street = normalizeSpokenOrdinalTokens(street)
	street = strings.Join(strings.Fields(street), " ")
	return strings.TrimSpace(street)
}

// EmbeddedGeocodeCity returns a city extracted from "AT NEWTON FALLS …" phrasing.
func EmbeddedGeocodeCity(street string) string {
	if city, _, ok := splitAtCityPrefix(strings.TrimSpace(street)); ok {
		return strings.TrimSpace(city)
	}
	return ""
}

// sttConfusableStreetTokenPairs are whole-token STT swaps that share little
// Levenshtein overlap but are routinely confused on radio ("HARVARD"/"HUBBARD").
var sttConfusableStreetTokenPairs = [][2]string{
	{"HARVARD", "HUBBARD"},
	{"ALLISON", "ALLYSON"}, // STT often drops/adds the Y on Allyson Drive
	// Gateway-corrected radio STT (so GeocodeResultPlausible accepts the pin).
	{"ROCKWOOD", "LOCKWOOD"},
	{"ROSKWOOD", "LOCKWOOD"},
	{"DEVIL", "DIBBLE"},
	{"WILCON", "WILSON"},
	{"MSGREGOR", "MCGREGOR"},
	{"MACGREGOR", "MCGREGOR"},
}

func sttConfusableTokenAlt(tok string) string {
	tok = strings.ToUpper(strings.TrimSpace(tok))
	for _, pair := range sttConfusableStreetTokenPairs {
		switch tok {
		case pair[0]:
			return pair[1]
		case pair[1]:
			return pair[0]
		}
	}
	return ""
}

// streetsAreSTTConfusableEquivalent reports when two street phrases match after
// confusable-token swaps and/or swapping a two-word stem before the type
// ("HARVARD YOUNGSTOWN RD" ↔ "YOUNGSTOWN HUBBARD RD").
func streetsAreSTTConfusableEquivalent(a, b string) bool {
	a = CanonicalStreetName(strings.ToUpper(strings.TrimSpace(a)))
	b = CanonicalStreetName(strings.ToUpper(strings.TrimSpace(b)))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	for _, av := range append([]string{a}, sttConfusableStreetTokenVariants(a)...) {
		for _, bv := range append([]string{b}, sttConfusableStreetTokenVariants(b)...) {
			if av == bv {
				return true
			}
			for _, asw := range twoWordStemOrderSwapVariants(av) {
				if asw == bv {
					return true
				}
			}
			for _, bsw := range twoWordStemOrderSwapVariants(bv) {
				if av == bsw {
					return true
				}
			}
		}
	}
	return false
}

// sttConfusableStreetTokenVariants replaces confusable tokens in a street name.
func sttConfusableStreetTokenVariants(base string) []string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(base)))
	if len(fields) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{CanonicalStreetName(base): true}
	for i, f := range fields {
		if localStreetSuffixes[f] || streetDirTokens[f] || isStreetQuadrantSuffix(f) {
			continue
		}
		for _, pair := range sttConfusableStreetTokenPairs {
			alt := ""
			switch f {
			case pair[0]:
				alt = pair[1]
			case pair[1]:
				alt = pair[0]
			}
			if alt == "" {
				continue
			}
			nf := append([]string{}, fields...)
			nf[i] = alt
			canon := CanonicalStreetName(strings.Join(nf, " "))
			if canon == "" || seen[canon] {
				continue
			}
			seen[canon] = true
			out = append(out, canon)
		}
	}
	return out
}

// twoWordStemNoSwapTokens are name components that form a single named place
// with the other stem ("PLEASANT PARK CT") — swapping yields garbage queries.
var twoWordStemNoSwapTokens = map[string]bool{
	"PARK": true, "PLACE": true, "HEIGHTS": true, "HILLS": true, "VIEW": true,
	"GROVE": true, "WOODS": true, "LAKE": true, "CREEK": true, "VALLEY": true,
	"MANOR": true, "SQUARE": true, "POINT": true, "LANDING": true, "RUN": true,
}

// twoWordStemOrderSwapVariants swaps the two stem words before a thoroughfare
// type ("HARVARD YOUNGSTOWN RD" → "YOUNGSTOWN HARVARD RD"). City-pair connector
// roads are often spoken/STT'd in either order.
func twoWordStemOrderSwapVariants(base string) []string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(base)))
	if len(fields) != 3 || !localStreetSuffixes[fields[2]] {
		return nil
	}
	if streetDirTokens[fields[0]] || streetDirTokens[fields[1]] ||
		isStreetQuadrantSuffix(fields[0]) || isStreetQuadrantSuffix(fields[1]) {
		return nil
	}
	if twoWordStemNoSwapTokens[fields[0]] || twoWordStemNoSwapTokens[fields[1]] {
		return nil
	}
	swapped := CanonicalStreetName(fields[1] + " " + fields[0] + " " + fields[2])
	if swapped == "" || strings.EqualFold(swapped, CanonicalStreetName(base)) {
		return nil
	}
	return []string{swapped}
}

func uniqueStrings(ss ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func isAlphaWordToken(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
