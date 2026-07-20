// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_unit_identifier.go — reject house-number + first-name extractions that
// are really phonetic unit/person identifiers (e.g. "JOHN X-RAY GEORGE 4852").

package mapping

import (
	"regexp"
	"strings"
)

var (
	phoneticAlphabetWordRE = regexp.MustCompile(`(?i)\b(?:X-?RAY|XRAY|ADAM|BOY|CHARLIE|CHARLES|DAVID|EDWARD|FRANK|GEORGE|HENRY|IDA|JOHN|KING|LINCOLN|MARY|NAN|OCEAN|PAUL|QUEEN|ROBERT|SAM|TOM|UNION|VICTOR|WILLIAM|YOUNG|ZEBRA)\b`)
	hyphenInitialsRE       = regexp.MustCompile(`(?i)\b[A-Z](?:-[A-Z0-9]+){1,5}\b`)
)

// commonGivenNames are tokens that are person names far more often than lone
// street names (no RD/ST/AVE suffix).
var commonGivenNames = map[string]bool{
	"GEORGE": true, "JOHN": true, "JAMES": true, "MARY": true, "ROBERT": true,
	"MICHAEL": true, "WILLIAM": true, "DAVID": true, "RICHARD": true, "JOSEPH": true,
	"THOMAS": true, "CHARLES": true, "CHRISTOPHER": true, "DANIEL": true, "MATTHEW": true,
	"ANTHONY": true, "MARK": true, "DONALD": true, "STEVEN": true, "PAUL": true,
	"ANDREW": true, "JOSHUA": true, "KENNETH": true, "KEVIN": true, "BRIAN": true,
	"EDWARD": true, "RONALD": true, "TIMOTHY": true, "JASON": true, "JEFFREY": true,
	"FRANK": true, "SCOTT": true, "STEPHEN": true, "GARY": true, "DONNA": true,
	"SUSAN": true, "LINDA": true, "PATRICIA": true, "JENNIFER": true, "ELIZABETH": true,
	"BARBARA": true, "MARGARET": true, "DOROTHY": true, "NANCY": true, "KAREN": true,
	"BETTY": true, "HELEN": true, "SANDRA": true, "CAROL": true, "RUTH": true,
}

// AddressIsUnitPersonIdentifier reports when a house number + single token
// "street" is really a phonetic person/unit identifier from radio traffic.
func AddressIsUnitPersonIdentifier(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || strings.Contains(street, "&") {
		return false
	}
	// "THE NUMBER 12, 12 SCOTT" explicitly frames 12 as a house number and
	// repeats it before the street, which otherwise looks identical to a
	// badge/unit number repeated next to a common given name ("SCOTT"). The
	// explicit framing takes priority — it's a real dispatch address, not a
	// roll-call identifier.
	if addressConfirmedByRepeatedDispatchFraming(addr, transcript) {
		return false
	}
	if AddressStreetIsPhoneticAlphabetOnly(addr) {
		return true
	}
	fields := strings.Fields(street)
	if len(fields) != 1 || hasStreetSuffix(street) || localRouteKeywords[fields[0]] {
		return false
	}
	name := fields[0]
	if !commonGivenNames[name] {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, house) {
		return false
	}
	// Phonetic spelling / initials near the same number (JOHN X-RAY GEORGE 4852).
	if phoneticAlphabetWordRE.MatchString(u) || hyphenInitialsRE.MatchString(u) {
		return true
	}
	// Badge/unit numbers are often repeated in roll-call traffic.
	if strings.Count(u, house) >= 2 {
		return true
	}
	return false
}

// AddressStreetIsPhoneticAlphabetOnly reports house + multi-word "streets"
// that are really radio unit/person spellings ("108 KING UNION" from
// "108, KING UNION VICTOR, 4857, KUV 4857") — every street token is a
// phonetic-alphabet word and there is no thoroughfare suffix.
func AddressStreetIsPhoneticAlphabetOnly(addr string) bool {
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if street == "" || strings.Contains(street, "&") || hasStreetSuffix(street) {
		return false
	}
	fields := strings.Fields(street)
	if len(fields) < 2 {
		return false
	}
	for _, f := range fields {
		if localStreetSuffixes[f] || localRouteKeywords[f] {
			return false
		}
		if !phoneticAlphabetWordRE.MatchString(f) {
			return false
		}
	}
	return true
}

// TranscriptIsPhoneticAlphabetRollCall reports phonetic-alphabet / unit-check
// traffic ("MARY, CHARLES, HENRY, JOHN FRANK, 280, STAND BY") that must not
// geocode even when STT glues digits to STAND BY or snaps to a similar street.
func TranscriptIsPhoneticAlphabetRollCall(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	if containsRealIncidentMarker(u) ||
		strings.Contains(u, " DISPATCHING ") ||
		strings.Contains(u, " SQUAD CALL ") ||
		strings.Contains(u, " RESPOND ") ||
		strings.Contains(u, " RESPONDING TO ") {
		return false
	}
	if len(phoneticAlphabetWordRE.FindAllString(u, -1)) < 3 {
		return false
	}
	if strings.Contains(u, " STAND BY") || strings.Contains(u, " STANDBY") {
		return true
	}
	if strings.Contains(u, " I BELIEVE IT'S ") || strings.Contains(u, " I BELIEVE ITS ") {
		return true
	}
	return false
}

// AddressIsStandByStreetMisSnap reports when STT truncated "STAND BY" to a fake
// street ("280 STAND" → snapped "280 STAHL AVENUE") with no STAHL spoken.
func AddressIsStandByStreetMisSnap(addr, transcript string) bool {
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, " STAND BY") && !strings.Contains(u, " STANDBY") {
		return false
	}
	if strings.Contains(u, " STAHL") {
		return false
	}
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if street == "" {
		return false
	}
	stU := strings.ToUpper(street)
	return stU == "STAND" || strings.HasPrefix(stU, "STAND ") || strings.Contains(stU, "STAHL")
}
