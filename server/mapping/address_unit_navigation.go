// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_unit_navigation.go — reject unit-to-unit coordination and "175 OR
// HEAD OVER TO WARREN AVENUE" style extractions that are not incident locations.

package mapping

import (
	"regexp"
	"strings"
)

var (
	unitToUnitRE        = regexp.MustCompile(`(?i)\b\d{1,4}\s+TO\s+\d{1,4}\b`)
	addressUnitChoiceRE = regexp.MustCompile(`(?i)\bOR\s+(?:HEAD\s+OVER\s+TO|GO\s+TO|HEAD\s+TO|PROCEED\s+TO|SWING\s+BY)\b`)
	unitStandOutRE        = regexp.MustCompile(`(?i)\b(\d{1,4})\s+AND\s+(\d{1,4})\s+CAN\s+STAND\s+(?:OUT|DOWN)\b`)
	cancelUnitsRE         = regexp.MustCompile(`(?i)\bCANCEL\s+(\d{1,4})\s+AND\s+(\d{1,4})\b`)
	decimalUnitPairRE     = regexp.MustCompile(`(?i)\b(\d{1,2})\.(\d{1,2})\s+AND\s+(\d{1,2})\.(\d{1,2})\b`)
	unitMeetNumberRE      = regexp.MustCompile(`(?i)\bMEET\s+\d{1,4}\b`)
)

// TranscriptIsUnitCoordinationChatter reports unit-to-unit questions about where
// to go next ("117 TO 116, DO YOU WANT ME TO KEEP GOING WITH 175 OR HEAD OVER
// TO WARREN AVENUE?") — not toned dispatches with an incident location.
func TranscriptIsUnitCoordinationChatter(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	if containsRealIncidentMarker(u) {
		return false
	}
	if unitMeetNumberRE.MatchString(u) &&
		(strings.Contains(u, " BACK AT ") || strings.Contains(u, " AT THE JAIL") ||
			strings.Contains(u, " TAKE A LOOK") || strings.Contains(u, " WHEN YOU GET BACK") ||
			strings.Contains(u, " SQUAD MEET ")) {
		return true
	}
	if !unitToUnitRE.MatchString(u) {
		return false
	}
	for _, m := range []string{
		" DO YOU WANT ", " DO YOU WANT ME ", " WANT ME TO ", " KEEP GOING WITH ",
		" SHOULD I ", " CAN I ", " WOULD YOU LIKE ", " YOU WANT ME ",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// AddressIsUnitNavigationChoice reports when the LLM glued a unit/vehicle
// number onto a movement choice ("175 OR HEAD OVER TO WARREN AVENUE").
func AddressIsUnitNavigationChoice(addr, transcript string) bool {
	u := strings.ToUpper(strings.TrimSpace(addr))
	if u == "" || !addressUnitChoiceRE.MatchString(u) {
		return false
	}
	if TranscriptIsUnitCoordinationChatter(transcript) {
		return true
	}
	house, street := splitHouseAndStreet(u)
	if house != "" && len(house) <= 4 && addressUnitChoiceRE.MatchString(street) {
		return true
	}
	return addressUnitChoiceRE.MatchString(u)
}

// AddressIsUnitStandDownOrCancel reports when two unit/vehicle numbers were
// misread as a street intersection ("215 AND 246 CAN STAND OUT", "CANCEL 215
// AND 246" → not SR 215 & SR 246).
func AddressIsUnitStandDownOrCancel(addr, transcript string) bool {
	a := strings.ToUpper(strings.TrimSpace(addr))
	if strings.HasPrefix(a, "CANCEL ") {
		return true
	}
	if !transcriptHasUnitStandDownOrCancel(transcript) {
		return false
	}
	if a == "" {
		return true
	}
	left, right := splitIntersectionQuery(a)
	if left != "" && right != "" && intersectionSidesAreBareUnitNumbers(left, right) {
		return true
	}
	return false
}

func transcriptHasUnitStandDownOrCancel(transcript string) bool {
	u := strings.ToUpper(transcript)
	return unitStandOutRE.MatchString(u) || cancelUnitsRE.MatchString(u)
}

func intersectionSidesAreBareUnitNumbers(a, b string) bool {
	return sideIsBareUnitNumber(a) && sideIsBareUnitNumber(b)
}

func sideIsBareUnitNumber(side string) bool {
	s := strings.ToUpper(strings.TrimSpace(side))
	for _, prefix := range []string{"SR ", "ST RTE ", "STATE ROUTE ", "US ", "CR ", "RT ", "RTE "} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(s[len(prefix):])
			break
		}
	}
	fields := strings.Fields(s)
	if len(fields) != 1 || len(fields[0]) < 1 || len(fields[0]) > 4 {
		return false
	}
	for _, r := range fields[0] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// AddressIsMisreadDecimalOrUnitPair reports when dispatch radio IDs like
// "16.22 AND 16.30" were misread as a route intersection ("SR 22 & SR 16").
func AddressIsMisreadDecimalOrUnitPair(addr, transcript string) bool {
	if !decimalUnitPairRE.MatchString(strings.ToUpper(transcript)) {
		return false
	}
	left, right := splitIntersectionQuery(strings.ToUpper(strings.TrimSpace(addr)))
	if left == "" || right == "" {
		return false
	}
	return intersectionSidesAreBareUnitNumbers(left, right)
}
