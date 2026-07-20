// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_malformed.go — recover or reject LLM addresses that glue a plate
// number or radio narrative onto a real street ("9211 THEY TERMINATED… ON
// STOKES BOULEVARD" → "STOKES BOULEVARD").

package mapping

import (
	"regexp"
	"strings"
)

var (
	phoneticPlateBlockRE = regexp.MustCompile(`(?i)\b(?:X-?RAY|XRAY|ADAM|BOY|CHARLIE|DAVID|EDWARD|FRANK|GEORGE|HENRY|IDA|JOHN|KING|LINCOLN|MARY|NAN|OCEAN|PAUL|QUEEN|ROBERT|SAM|TOM|UNION|VICTOR|WILLIAM|YOUNG|ZEBRA)(?:\s+(?:X-?RAY|XRAY|ADAM|BOY|CHARLIE|DAVID|EDWARD|FRANK|GEORGE|HENRY|IDA|JOHN|KING|LINCOLN|MARY|NAN|OCEAN|PAUL|QUEEN|ROBERT|SAM|TOM|UNION|VICTOR|WILLIAM|YOUNG|ZEBRA)){1,5}\s+\d{3,5}\b`)
	ohioPlateReadoutRE   = regexp.MustCompile(`(?i)\bOHIO\s+\d{1,3}\s+OF\b`)
	embeddedStreetOnRE   = regexp.MustCompile(`(?i)\b(?:ON|AT|NEAR)\s+([A-Z0-9][A-Z0-9\s\-]{0,40}?\s+(?:BLVD|BOULEVARD|RD|ROAD|ST|STREET|AVE|AVENUE|DR|DRIVE|LN|LANE|CT|COURT|WAY|PKWY|PARKWAY|HWY|HIGHWAY|CIR|CIRCLE|PL|PLACE|TRL|TRAIL))\b`)
	addressNarrativeRE   = regexp.MustCompile(`(?i)\b(?:THEY|WE|HE|SHE|I)\s+(?:TERMINATED|ENDED|STARTED|PURSUED|ADVISED|REPORTED|SAID)\b|\bTHE PURSUIT\b|\bPURSUED (?:THAT|A|THE)\b|\bHAVE OHIO\b|\bTERMINATED THE PURSUIT\b`)
	patientNameStreetRE  = regexp.MustCompile(`(?i)\bTHE NUMBER\s+(\d{1,6})\s*,\s*(?:[A-Z][A-Z'-]*\s+)*([A-Z][A-Z'-]{2,})\s*,?\s*\d{1,3}[- ]YEAR`)
	apparatusUnitStreetRE = regexp.MustCompile(`(?i)\b(?:STATION\s+\d+\s*,\s*)?(?:THE\s+)?NUMBER\s+(\d{1,3})\s+([A-Z][A-Z'-]{2,})\s*,?\s*NUMBER\s+(\d{1,3})\s+([A-Z][A-Z'-]{2,})\b`)
)

func transcriptHasPhoneticPlateReadout(transcript string) bool {
	u := strings.ToUpper(transcript)
	return phoneticPlateBlockRE.MatchString(u) || ohioPlateReadoutRE.MatchString(u)
}

func salvageEmbeddedStreetFromMalformed(addr string) string {
	if m := embeddedStreetOnRE.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(addr))); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func transcriptContainsPlateNumber(house, transcript string) bool {
	if house == "" || !isAllDigits(house) {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, house) {
		return false
	}
	return phoneticPlateBlockRE.MatchString(u) ||
		ohioPlateReadoutRE.MatchString(u) ||
		tempTagMarkerRE.MatchString(u) ||
		plateHyphenTokenRE.MatchString(u)
}

// AddressIsMalformedPlateOrNarrative reports when the LLM prefixed a plate
// number or pursuit/status narrative onto an otherwise valid street mention.
func AddressIsMalformedPlateOrNarrative(addr, transcript string) bool {
	u := strings.ToUpper(strings.TrimSpace(addr))
	if u == "" {
		return false
	}
	if addressNarrativeRE.MatchString(u) {
		return true
	}
	house, _ := splitHouseAndStreet(u)
	if house != "" && transcriptContainsPlateNumber(house, transcript) {
		return true
	}
	return false
}

// maybeSalvageMalformedAddress strips plate/narrative junk and keeps an
// embedded "ON STREET" when one is present; otherwise clears the address.
func maybeSalvageMalformedAddress(curated *CuratedAlert, transcript string, scope *ScopeData) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	if !AddressIsMalformedPlateOrNarrative(curated.Address, transcript) &&
		!AddressIsUnitNavigationChoice(curated.Address, transcript) {
		return
	}
	if salvaged := salvageEmbeddedStreetFromMalformed(curated.Address); salvaged != "" {
		salvaged = sanitizeAddressField(salvaged)
		if salvaged != "" && salvagedStreetIsGeocodable(salvaged, scope) &&
			!AddressIsMalformedPlateOrNarrative(salvaged, transcript) {
			curated.Address = salvaged
			return
		}
	}
	clearExtractAddressFields(curated)
	clearNatureIfPlateReadout(curated, transcript)
}

// maybeSalvageMissingDispatchHouse prepends a house number when dispatch spoke
// one before the street but extraction kept only the thoroughfare
// ("WESTGATE BOULEVARD" ← "108108 WESTGATE" after STT glued the readback).
func maybeSalvageMissingDispatchHouse(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if house != "" {
		return
	}
	if street == "" {
		street = strings.ToUpper(strings.TrimSpace(curated.Address))
	}
	if street == "" {
		return
	}
	if h := DispatchHouseFromTranscriptForStreet(transcript, street); h != "" {
		curated.Address = h + " " + street
	}
}

// maybeSalvagePatientNameStreet rewrites "10 LINDSAY ROSE" from
// "THE NUMBER 10, LINDSAY ROSE SCOTT, 64-YEAR-OLD …" when the trailing token
// before the age descriptor matches a known street stem.
func maybeSalvagePatientNameStreet(curated *CuratedAlert, transcript string, scope *ScopeData) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" || scope == nil {
		return
	}
	m := patientNameStreetRE.FindStringSubmatch(strings.ToUpper(transcript))
	if len(m) < 3 {
		return
	}
	house := strings.TrimSpace(m[1])
	stem := strings.TrimSpace(m[2])
	if house == "" || stem == "" || !knownStreetStemMatches(stem, scope.KnownStreets) {
		return
	}
	curHouse, curStreet := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if curHouse != house {
		return
	}
	if curStreet == stem || strings.HasPrefix(curStreet, stem+" ") {
		return
	}
	snapped := snapExtractedAddressToKnownStreet(house+" "+stem, scope.KnownStreets, transcript)
	if snapped == "" {
		snapped = house + " " + stem
	}
	curated.Address = snapped
}

// maybeSalvageApparatusUnitStreet recovers a suffixless street when dispatch
// names an apparatus unit on that street ("NUMBER 4 LARRY, NUMBER 4 LARRY").
func maybeSalvageApparatusUnitStreet(curated *CuratedAlert, transcript string, scope *ScopeData) {
	if curated == nil || scope == nil {
		return
	}
	u := strings.ToUpper(transcript)
	if m := apparatusUnitStreetRE.FindStringSubmatch(u); len(m) >= 5 {
		if strings.EqualFold(m[1], m[3]) && strings.EqualFold(m[2], m[4]) {
			stem := strings.TrimSpace(m[2])
			if stem != "" && knownStreetStemMatches(stem, scope.KnownStreets) {
				snapped := snapExtractedAddressToKnownStreet(stem, scope.KnownStreets, transcript)
				if snapped == "" || snapped == stem {
					snapped = pickKnownStreetForStem(stem, scope.KnownStreets)
				}
				if snapped != "" {
					curated.Address = snapped
					return
				}
			}
		}
	}
	addr := strings.ToUpper(strings.TrimSpace(curated.Address))
	if addr == "" {
		return
	}
	house, street := splitHouseAndStreet(addr)
	if house == "" || street == "" {
		return
	}
	if !strings.HasSuffix(street, " NUMBER") {
		return
	}
	stem := strings.TrimSpace(strings.TrimSuffix(street, " NUMBER"))
	if stem == "" || !knownStreetStemMatches(stem, scope.KnownStreets) {
		return
	}
	snapped := snapExtractedAddressToKnownStreet(stem, scope.KnownStreets, transcript)
	if snapped == "" || snapped == stem {
		snapped = pickKnownStreetForStem(stem, scope.KnownStreets)
	}
	if snapped != "" {
		curated.Address = snapped
	}
}

func knownStreetStemMatches(stem string, knownStreets []string) bool {
	stem = strings.ToUpper(strings.TrimSpace(stem))
	if stem == "" {
		return false
	}
	for _, ks := range knownStreets {
		nameStem, _ := streetNameAndSuffix(strings.ToUpper(strings.TrimSpace(ks)))
		if nameStem == stem || strings.HasPrefix(nameStem, stem+" ") {
			return true
		}
	}
	return false
}

func pickKnownStreetForStem(stem string, knownStreets []string) string {
	stem = strings.ToUpper(strings.TrimSpace(stem))
	var best string
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		nameStem, sfx := streetNameAndSuffix(ku)
		if nameStem != stem && !strings.HasPrefix(nameStem, stem+" ") {
			continue
		}
		if best == "" || (sfx != "" && !hasStreetSuffix(best)) {
			best = ku
		}
	}
	return best
}

func salvagedStreetIsGeocodable(addr string, scope *ScopeData) bool {
	if AddressHasGeocodableAnchor(addr, scope) {
		return true
	}
	u := strings.TrimSpace(strings.ToUpper(addr))
	if u == "" || strings.Contains(u, "&") {
		return false
	}
	house, street := splitHouseAndStreet(u)
	if house != "" {
		return false
	}
	if street == "" {
		street = u
	}
	return hasStreetSuffix(street)
}
