// Copyright (C) 2025 Thinline Dynamic Solutions

package mapping

import "strings"

// maybeAppendSpokenStreetSuffix upgrades truncated LLM addresses when the
// transcript clearly included a thoroughfare type ("13117 CEDAR" →
// "13117 CEDAR RD" when dispatch said "13117 CEDAR ROAD").
func maybeAppendSpokenStreetSuffix(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	house, street := splitHouseAndStreet(curated.Address)
	if house == "" || street == "" || hasStreetSuffix(street) {
		return
	}
	u := strings.ToUpper(transcript)
	stU := strings.ToUpper(street)
	for spoken, canon := range spokenStreetSuffixes {
		if strings.Contains(u, house+" "+stU+spoken) ||
			strings.Contains(u, house+", "+stU+spoken) ||
			strings.Contains(u, house+", "+stU+","+spoken) {
			curated.Address = house + " " + stU + " " + canon
			return
		}
	}
	if suf := inferGeneralStreetSuffixFromTranscript(house, street, transcript); suf != "" {
		curated.Address = house + " " + stU + " " + suf
	}
}

// maybeEnrichIntersectionSidesFromTranscript upgrades bare intersection sides
// when dispatch spoke the full name earlier ("MILES & GREEN" → "MILES RD &
// GREEN RD" after "MILES ROAD … GREEN ROAD" in the transcript).
func maybeEnrichIntersectionSidesFromTranscript(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	a, b := splitIntersectionQuery(curated.Address)
	if a == "" || b == "" {
		return
	}
	a2 := enrichIntersectionSideFromTranscript(a, transcript)
	b2 := enrichIntersectionSideFromTranscript(b, transcript)
	if a2 == strings.ToUpper(strings.TrimSpace(a)) && b2 == strings.ToUpper(strings.TrimSpace(b)) {
		return
	}
	curated.Address = a2 + " & " + b2
}

func enrichIntersectionSideFromTranscript(side, transcript string) string {
	side = strings.ToUpper(strings.TrimSpace(side))
	if side == "" || hasStreetSuffix(side) {
		return side
	}
	u := strings.ToUpper(transcript)
	for spoken, canon := range spokenStreetSuffixes {
		if transcriptContainsStreetPhrase(u, side+spoken) {
			return side + " " + canon
		}
	}
	return side
}

func transcriptContainsStreetPhrase(transcriptUpper, phrase string) bool {
	phrase = strings.ToUpper(strings.TrimSpace(phrase))
	if phrase == "" {
		return false
	}
	padded := " " + transcriptUpper + " "
	return strings.Contains(padded, " "+phrase+" ") ||
		strings.Contains(padded, " "+phrase+",") ||
		strings.Contains(padded, " "+phrase+".")
}

var spokenStreetSuffixes = map[string]string{
	" ROAD": "RD", " RD": "RD",
	" STREET": "ST", " ST": "ST",
	" AVENUE": "AVE", " AVE": "AVE",
	" DRIVE": "DR", " DR": "DR",
	" LANE": "LN", " LN": "LN",
	" BOULEVARD": "BLVD", " BLVD": "BLVD",
	" COURT": "CT", " CT": "CT",
	" PLACE": "PL", " PL": "PL",
}

// spokenThoroughfareMishears are STT mis-hearings of thoroughfare types on dispatch.
var spokenThoroughfareMishears = map[string]string{
	"DRILL": "DRIVE",
}

// canonicalizeMisheardStreetSuffix upgrades addresses when STT substituted a
// non-thoroughfare token for a real suffix ("SPRUCE DRILL" → "SPRUCE DRIVE").
func canonicalizeMisheardStreetSuffix(addr string, knownStreets []string) string {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return addr
	}
	fields := strings.Fields(street)
	if len(fields) < 2 {
		return addr
	}
	last := fields[len(fields)-1]
	rep, ok := spokenThoroughfareMishears[last]
	if !ok {
		return addr
	}
	stem := strings.Join(fields[:len(fields)-1], " ")
	candidate := stem + " " + rep
	for _, ks := range knownStreets {
		ku := strings.ToUpper(strings.TrimSpace(ks))
		if ku == candidate || strings.HasPrefix(ku, candidate+" ") {
			return house + " " + ku
		}
	}
	return house + " " + candidate
}
