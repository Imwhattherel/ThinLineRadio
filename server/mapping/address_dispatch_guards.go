// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_dispatch_guards.go — reject extracted addresses whose house numbers
// come from apparatus IDs, station numbers, or STT concatenation rather than
// a dispatcher-stated street address.

package mapping

import (
	"regexp"
	"strings"
)

var (
	dispatchUnitPrefixRE = regexp.MustCompile(`(?i)\b(?:MED|SQUAD|MEDFOR|LADDER|ENGINE|MEDIC|TRUCK|CAR|BATTALION|UNIT|CHANNEL|RESCUE)\s+(\d{1,5})\b`)
	engineMegaNumberRE   = regexp.MustCompile(`(?i)\b(?:DISPATCHING\s+)?ENGINE\s+(\d{4,6})\b`)
	callsignBeforeHouseRE = regexp.MustCompile(`\b([A-Z]{2,5})\s+(\d{2,4})\b`)
	// Captures SQUAD CALL <n> <token>; caller decides if token is a sector letter.
	squadCallRefHouseRE = regexp.MustCompile(`(?i)\bSQUAD CALL\s+(\d{3,5})\s+([A-Z]+)`)
	feetMangledHouseRE  = regexp.MustCompile(`(?i)\b(\d{3,4})\s+FEET\s+(\d{1,4})\s+`)
)

// squadCallRefIsSectorLetter is true for CAD sector/incident letters ("A"), not
// street stems ("BROOKWOOD").
func squadCallRefIsSectorLetter(tok string) bool {
	tok = strings.TrimSpace(strings.ToUpper(tok))
	return len(tok) == 1 && tok[0] >= 'A' && tok[0] <= 'Z'
}

// callsignTagNoise are apparatus/unit tokens that precede numbers in dispatch
// traffic — not broadcast callsigns like WPKM 221.
var callsignTagNoise = map[string]bool{
	"ENGINE": true, "MEDIC": true, "SQUAD": true, "TRUCK": true, "CAR": true,
	"UNIT": true, "BATTALION": true, "STATION": true, "CHANNEL": true, "MED": true,
	"LADDER": true, "RESCUE": true, "CHIEF": true, "CENTRAL": true, "CALL": true,
	"ROUTE": true, // EN ROUTE / ON ROUTE / highway ROUTE — not broadcast callsigns.
	"TO": true, "AT": true, "ON": true, "FOR": true, // "SQUAD CALL TO 40 MAIN"
}

// SanitizeEnginePrefixedAddress strips apparatus/engine mega-numbers glued onto
// a street when dispatch named the engine but no house ("ENGINE 52471, COMSTOCK").
func SanitizeEnginePrefixedAddress(addr, transcript string) string {
	addr = strings.ToUpper(strings.TrimSpace(addr))
	if addr == "" {
		return addr
	}
	if AddressHouseNumberFollowsEngineBlob(addr, transcript) {
		_, street := splitHouseAndStreet(addr)
		return strings.TrimSpace(street)
	}
	if AddressHouseNumberIsEngineUnitConcatenation(addr, transcript) {
		house, street := splitHouseAndStreet(addr)
		for splitAt := 1; splitAt <= 2 && splitAt < len(house); splitAt++ {
			rest := house[splitAt:]
			if rest != "" {
				u := strings.ToUpper(transcript)
				if strings.Contains(u, rest+" ") || strings.Contains(u, ", "+rest) ||
					strings.Contains(u, " "+rest+",") {
					return strings.TrimSpace(rest + " " + street)
				}
			}
		}
	}
	return addr
}

// AddressHouseNumberIsStationCorridorPOI reports when the extracted house is a
// leading apparatus/station id before a corridor street with a POI reference
// ("28, NORTH MAIN, IN FRONT OF THE ARBY'S" — station 28 on SR corridor, not
// 28 N Main St).
func AddressHouseNumberIsStationCorridorPOI(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) > 3 {
		return false
	}
	u := strings.ToUpper(transcript)
	head := strings.Fields(street)
	if len(head) == 0 {
		return false
	}
	if !strings.Contains(u, house+", "+head[0]) {
		return false
	}
	if strings.Contains(u, "IN FRONT OF THE ") || strings.Contains(u, "IN FRONT OF A ") {
		return true
	}
	return strings.Contains(u, "STATION "+house) &&
		(strings.Contains(u, " ON THE ROADWAY") || strings.Contains(u, " ON ROADWAY"))
}

// stripGluedQuadrantWhenCommaSpoken rewrites "COMSTOCK STREET NORTHWEST" to
// "COMSTOCK STREET" when dispatch spoke the quadrant after a comma.
func stripGluedQuadrantWhenCommaSpoken(addr, transcript string) string {
	addr = strings.ToUpper(strings.TrimSpace(addr))
	if addr == "" {
		return addr
	}
	fields := strings.Fields(addr)
	if len(fields) < 3 {
		return addr
	}
	last := fields[len(fields)-1]
	if !isStreetQuadrantSuffix(last) {
		return addr
	}
	stem := strings.Join(fields[:len(fields)-1], " ")
	u := strings.ToUpper(transcript)
	if strings.Contains(u, stem+", "+last) {
		return stem
	}
	abbr := map[string]string{
		"NORTHWEST": "NW", "NORTHEAST": "NE", "SOUTHWEST": "SW", "SOUTHEAST": "SE",
	}[last]
	if abbr != "" && strings.Contains(u, stem+", "+abbr) {
		return stem
	}
	return addr
}

// AddressHouseNumberIsEngineUnitConcatenation reports when a 4–6 digit house
// number is STT/extractor glue of an apparatus id plus a real house number
// ("DISPATCHING ENGINE 5, 2185 PEACE" → "52185 PEACE").
func AddressHouseNumberIsEngineUnitConcatenation(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) < 4 {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "ENGINE") && !strings.Contains(u, "DISPATCHING") {
		return false
	}
	for splitAt := 1; splitAt <= 2 && splitAt < len(house); splitAt++ {
		app := house[:splitAt]
		rest := house[splitAt:]
		if rest == "" {
			continue
		}
		hasEngineApp := strings.Contains(u, "ENGINE "+app+" ") ||
			strings.Contains(u, "ENGINE "+app+",") ||
			strings.Contains(u, "ENGINE "+app+".") ||
			strings.Contains(u, "DISPATCHING ENGINE "+app+" ") ||
			strings.Contains(u, "DISPATCHING ENGINE "+app+",") ||
			strings.HasSuffix(u, "ENGINE "+app)
		if !hasEngineApp {
			continue
		}
		if strings.Contains(u, rest+" ") || strings.Contains(u, ", "+rest) ||
			strings.Contains(u, " "+rest+",") || strings.Contains(u, " "+rest+" ") {
			return true
		}
	}
	return false
}

// AddressHouseNumberFollowsEngineBlob reports when STT merged an apparatus id
// and house number into one token after ENGINE ("ENGINE 52185 PEACE AVENUE").
func AddressHouseNumberFollowsEngineBlob(addr, transcript string) bool {
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || len(house) < 4 {
		return false
	}
	for _, m := range engineMegaNumberRE.FindAllStringSubmatch(strings.ToUpper(transcript), -1) {
		if len(m) >= 2 && m[1] == house {
			return true
		}
	}
	return false
}

// callsignMatchIsSpokenInfinitive reports "GOING TO BE 2175" / "TO BE 2175" —
// not a broadcast callsign like WPKM 221.
func callsignMatchIsSpokenInfinitive(transcriptUpper, match string) bool {
	idx := strings.Index(transcriptUpper, match)
	if idx <= 0 {
		return false
	}
	before := strings.TrimSpace(transcriptUpper[:idx])
	return strings.HasSuffix(before, "GOING TO") || strings.HasSuffix(before, " TO")
}

// AddressHouseNumberIsCallsignIdentifier reports when the extracted house number
// is really a broadcast callsign suffix ("WPKM 221, WARREN CITY FIRE…").
func AddressHouseNumberIsCallsignIdentifier(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) < 2 || len(house) > 4 || !isAllDigits(house) {
		return false
	}
	u := strings.ToUpper(transcript)
	stHead := ""
	if fields := strings.Fields(street); len(fields) > 0 {
		stHead = fields[0]
	}
	for _, m := range callsignBeforeHouseRE.FindAllStringSubmatch(u, -1) {
		if len(m) < 3 || m[2] != house {
			continue
		}
		if callsignTagNoise[m[1]] {
			continue
		}
		if callsignMatchIsSpokenInfinitive(u, m[0]) {
			continue
		}
		if stHead != "" && strings.Contains(u, house+" "+stHead) {
			return false
		}
		stem, _ := streetNameAndSuffix(street)
		if stem != "" && collapsedStreetWordsAfterHouse(u, house) == stripStreetSpaces(stem) {
			return false
		}
		return true
	}
	return false
}

// AddressIsMunicipalityAgencyName reports when the extracted street is really a
// municipality + agency label ("WARREN CITY" from "WARREN CITY FIRE DEPARTMENT",
// or "WARREN" from "221, WARREN CITY FIRE…").
func AddressIsMunicipalityAgencyName(addr, transcript string) bool {
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	fields := strings.Fields(street)
	if len(fields) == 0 {
		return false
	}
	u := strings.ToUpper(transcript)
	if agencyDispatchInTranscript(u) {
		return false
	}
	if len(fields) >= 2 && fields[len(fields)-1] == "CITY" {
		stem := strings.Join(fields, " ")
		for _, role := range []string{"FIRE DEPARTMENT", "FIRE", "POLICE DEPARTMENT", "POLICE", "EMS"} {
			if strings.Contains(u, stem+" "+role) {
				return true
			}
		}
		return false
	}
	if len(fields) == 1 {
		stem := fields[0]
		for _, suffix := range []string{
			"CITY FIRE DEPARTMENT", "CITY FIRE", "CITY POLICE DEPARTMENT", "CITY POLICE", "CITY EMS",
		} {
			if strings.Contains(u, stem+" "+suffix) {
				return true
			}
		}
	}
	return false
}

func agencyDispatchInTranscript(u string) bool {
	return strings.Contains(u, " DISPATCHING ") || strings.Contains(u, " SQUAD CALL ") ||
		strings.Contains(u, " FOR A ") || strings.Contains(u, " FOR AN ")
}

// AddressHouseNumberFromPriorityCode reports when the house number is a scene
// priority code ("CODE 4. POLAND ONLY HAVE ONE" → 4 POLAND).
func AddressHouseNumberFromPriorityCode(addr, transcript string) bool {
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	return houseNumberFromPriorityCode(house, transcript)
}

// AddressIsJurisdictionAvailabilityMisextract reports when the street token is a
// municipality named in mutual-aid crew-availability chatter, not a dispatch address.
func AddressIsJurisdictionAvailabilityMisextract(addr, transcript string) bool {
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if street == "" {
		return false
	}
	return suffixlessStreetIsJurisdictionRoster(street, transcript) ||
		AddressHouseNumberFromPriorityCode(addr, transcript)
}

// AddressHouseNumberIsBareStationToken reports when the house number is only a
// station id ("STATION 18, YOUNGSTOWN KINGSVILLE ROAD") rather than a dispatched
// street address.
func AddressHouseNumberIsBareStationToken(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) > 4 {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "STATION "+house) {
		return false
	}
	head := strings.Fields(street)
	if len(head) == 0 {
		return false
	}
	stHead := head[0]
	// "STATION 18, YOUNGSTOWN …" — station roster, not "18 YOUNGSTOWN …" dispatch.
	if strings.Contains(u, "STATION "+house+", "+stHead) {
		return true
	}
	for _, pat := range []string{
		house + ", " + stHead,
		"TO " + house + ",",
		"TO " + house + " " + stHead,
		"AT " + house + ",",
		"AT " + house + " " + stHead,
		"RESPOND TO " + house,
		"RESPONDING TO " + house,
	} {
		if strings.Contains(u, pat) {
			return false
		}
	}
	return strings.Contains(u, "STATION "+house)
}

// AddressHouseNumberIsStationNumberConcatenation reports when a 3–4 digit house
// number is STT glue of two apparatus station ids ("TO STATION 3435" → stations
// 34 and 35, or leading "3435 LIBERTY HEALTHCARE" with no real house number).
func AddressHouseNumberIsStationNumberConcatenation(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) < 3 || len(house) > 4 || !isAllDigits(house) {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, "STATION "+house) ||
		strings.Contains(u, "TO STATION "+house) ||
		strings.Contains(u, "FOR STATION "+house) {
		return true
	}
	for splitAt := 1; splitAt <= len(house)-1; splitAt++ {
		a, b := house[:splitAt], house[splitAt:]
		if !isAllDigits(a) || !isAllDigits(b) || !plausibleFireStationNumber(a) || !plausibleFireStationNumber(b) {
			continue
		}
		if stationPairMentionedInTranscript(u, a, b) {
			return true
		}
	}
	// STT dropped "STATION" and glued station ids before a facility name.
	if len(house) == 4 && strings.HasPrefix(strings.TrimSpace(u), house+" ") &&
		!hasStreetSuffix(street) && addressStreetLooksLikeFacility(street) &&
		plausibleFireStationNumber(house[:2]) && plausibleFireStationNumber(house[2:]) {
		return true
	}
	return false
}

func plausibleFireStationNumber(s string) bool {
	if s == "" || !isAllDigits(s) {
		return false
	}
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n >= 1 && n <= 99
}

func stationPairMentionedInTranscript(u, a, b string) bool {
	patterns := []string{
		"STATION " + a + " AND " + b,
		"STATION " + a + ", " + b,
		"STATION " + a + " " + b + ",",
		"STATION " + a + ", STATION " + b,
		"STATION " + a + " STATION " + b,
		"TO STATION " + a + " AND " + b,
		"TO STATION " + a + ", " + b,
		"TO STATION " + a + " " + b,
	}
	for _, p := range patterns {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}

func addressStreetLooksLikeFacility(street string) bool {
	for _, w := range strings.Fields(strings.ToUpper(strings.TrimSpace(street))) {
		if facilityAddressWords[w] {
			return true
		}
	}
	return false
}

var facilityAddressWords = map[string]bool{
	"HEALTHCARE": true, "HOSPITAL": true, "MEDICAL": true, "MANOR": true,
	"NURSING": true, "REHAB": true, "REHABILITATION": true, "CLINIC": true,
	"SCHOOL": true, "CHURCH": true, "CENTER": true, "CENTRE": true,
}

// AddressHouseNumberIsRouteFragment reports when the house number only appears
// after a bare "ROUTE" token ("ROUTE 20889, COLBY ROAD") rather than as a
// dispatched street address.
func AddressHouseNumberIsRouteFragment(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	// "IS EN ROUTE 1918 …" is unit status, not highway "ROUTE 1918".
	if strings.Contains(u, "EN ROUTE "+house+" ") || strings.Contains(u, "EN ROUTE "+house+",") ||
		strings.Contains(u, "EN ROUTE "+house+".") {
		return false
	}
	// Numbered highways ("STATE ROUTE 88 NORTHEAST", "SR 88") — the route
	// number must never conflict with a real house on another street when STT
	// glues a directional + nature word ("88 NORTHEAST GAS").
	if strings.Contains(u, "STATE ROUTE "+house) || strings.Contains(u, "ST ROUTE "+house) ||
		strings.Contains(u, "SR "+house+" ") || strings.Contains(u, "SR "+house+",") ||
		strings.Contains(u, "SR "+house+".") {
		return true
	}
	if !(strings.Contains(u, "ROUTE "+house+" ") || strings.Contains(u, "ROUTE "+house+",") ||
		strings.Contains(u, "ROUTE "+house+".")) {
		return false
	}
	streetHead := strings.Fields(street)[0]
	// Compass directionals after a route number are not the street name of a
	// "mutual aid route <house> <street>" path phrasing.
	if directionalWordsCorrection[streetHead] {
		return true
	}
	for _, ok := range []string{
		"RESPOND TO " + house,
		"RESPONDING TO " + house,
		" AT " + house + ",",
		" AT " + house + " ",
		"TO " + house + ", " + streetHead,
		// "MUTUAL AID ROUTE 3550 CARVER NILES" — "ROUTE" here means "the path to
		// take", not a highway route number: the house number is immediately
		// followed by the SAME street's own name, not a separate street after a
		// comma (the real route-fragment shape is "ROUTE 20889, COLBY ROAD",
		// where the comma separates the bogus route number from the actual
		// street). Directly continuing into the street's first word with no
		// comma confirms the number belongs to this address, not to "ROUTE".
		"ROUTE " + house + " " + streetHead,
	} {
		if strings.Contains(u, ok) {
			return false
		}
	}
	return true
}

// AddressIsBareRouteStemMisextract reports when a house number was glued to a bare
// "ROUTE" stem while dispatch named a numbered route with BETWEEN cross streets
// ("8768 ROUTE" from "876-8 ROUTE 7 BETWEEN RICHARD AND WINGATE").
func AddressIsBareRouteStemMisextract(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" {
		return false
	}
	street = strings.TrimSpace(street)
	if street != "ROUTE" && street != "RTE" && street != "RT" {
		return false
	}
	cleaned := PreCleanTranscript(transcript)
	u := strings.ToUpper(cleaned)
	if !localBetweenRE.MatchString(u) {
		return false
	}
	if routeN, ok := primaryStateRouteInTranscript(cleaned); ok && routeN > 0 {
		return true
	}
	return false
}

// AddressHouseNumberIsDispatchUnitPrefix reports when the leading number is a
// unit designator glued to the street ("MED 7145 OLIVE" where MED is the
// medic unit and 7145 is the address — but "SQUAD 2514 HIGH" where 2514 is
// the house on a squad call is kept). Rejects when the prefix token is a unit
// type and the transcript does not contain a dispatch respond phrase.
func AddressHouseNumberIsDispatchUnitPrefix(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	for _, m := range dispatchUnitPrefixRE.FindAllStringSubmatch(u, -1) {
		if len(m) < 2 || m[1] != house {
			continue
		}
		prefix := strings.ToUpper(strings.TrimSpace(m[0]))
		// Squad + number at same start as address on a squad-toned call is normal.
		if strings.HasPrefix(prefix, "SQUAD "+house) &&
			(strings.Contains(u, " SQUAD CALL") || strings.Contains(u, " SQUAD CALL,")) {
			continue
		}
		if strings.Contains(u, " RESPOND TO "+house) ||
			strings.Contains(u, " RESPONDING TO "+house) ||
			strings.Contains(u, " RESPONSE TO "+house) {
			continue
		}
		// "MEDIC, 4918 BUSHNELL CAMPBELL ROAD" — the number after apparatus is
		// the call address, not a unit id sharing the house digits.
		stHead := strings.Fields(street)
		if len(stHead) > 0 && strings.Contains(u, house+" "+stHead[0]) &&
			(hasStreetSuffix(street) || streetHasGeocodableSuffix(street) || len(stHead) >= 2) {
			continue
		}
		// Repeated "MED 8, 1158, KEYES" style — number appears twice with street.
		if len(stHead) > 0 && strings.Count(u, house+" "+stHead[0]) >= 1 &&
			strings.Count(u, house) >= 2 {
			continue
		}
		return true
	}
	return false
}

// AddressHouseNumberNotSpokenInTranscript reports when the extracted house
// number never appears in the transcript (common with invented LLM digits).
func AddressHouseNumberNotSpokenInTranscript(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, house) {
		return false
	}
	// Hyphenated digit readout: 1-1-4-2-5 for 11425
	if spokenDigitsMatchHouse(house, u) {
		return false
	}
	return true
}

func spokenDigitsMatchHouse(house, upperTranscript string) bool {
	digits := strings.Fields(strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '-' || r == ' ' {
			return ' '
		}
		return -1
	}, upperTranscript))
	var buf strings.Builder
	for _, d := range digits {
		if len(d) == 1 && d[0] >= '0' && d[0] <= '9' {
			buf.WriteByte(d[0])
		}
	}
	return buf.String() == house
}

var batchRunBeforeStreetRE = regexp.MustCompile(`(?i)\bON\s+A\s+(\d{2})-(\d{3,5})\s+`)

// AddressHouseNumberIsBatchRunNumber reports when STT merged a dispatch run/batch
// token ("ON A 34-0635 SHARON MILL") into a fake house number (340635).
func AddressHouseNumberIsBatchRunNumber(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) < 5 {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, m := range batchRunBeforeStreetRE.FindAllStringSubmatch(u, -1) {
		if len(m) >= 3 && m[1]+m[2] == house {
			return true
		}
	}
	return false
}

// AddressIsPainScaleFragment reports when a house number is really a pain score
// ("10 OUT OF 10 LOWER LEFT QUADRANT" → "10 LOWER LEFT").
func AddressIsPainScaleFragment(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) > 2 {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "OUT OF 10") && !strings.Contains(u, "OUT OF TEN") {
		return false
	}
	stU := strings.ToUpper(street)
	return strings.Contains(u, "LOWER LEFT") && strings.Contains(stU, "LOWER") ||
		strings.Contains(u, "UPPER RIGHT") && strings.Contains(stU, "UPPER") ||
		strings.Contains(u, "QUADRANT") && strings.Contains(stU, "LEFT")
}

// AddressIsVitalSignFragment reports when suffixless extraction glued a blood
// pressure or respiration fragment into a fake street ("121 OVER 90, MORE THAN
// 16 TIMES" → "90 MORE THAN").
func AddressIsVitalSignFragment(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	stU := strings.ToUpper(street)
	if stU == "MORE THAN" || strings.HasPrefix(stU, "MORE THAN ") {
		if strings.Contains(u, "OVER "+house) ||
			strings.Contains(u, house+", MORE THAN") ||
			strings.Contains(u, house+" MORE THAN") {
			return true
		}
	}
	if strings.Contains(u, "BLOOD PRESSURE") && strings.Contains(street, "&") {
		return true
	}
	if strings.Contains(u, "BLOOD PRESSURE") && strings.Contains(u, " OVER "+house) {
		return true
	}
	if strings.Contains(u, "TIMES A MINUTE") && (stU == "MORE THAN" || strings.Contains(u, house+" MORE THAN")) {
		return true
	}
	return false
}

var vehicleMakeTokens = map[string]bool{
	"FORD": true, "CHEVY": true, "CHEVROLET": true, "DODGE": true, "TOYOTA": true,
	"HONDA": true, "NISSAN": true, "GMC": true, "JEEP": true, "BMW": true, "HYUNDAI": true,
}

// AddressIsVehicleDescription reports when a model-year + make was extracted as
// an address ("2016 FORD ESCAPE" from a stolen-vehicle broadcast).
func AddressIsVehicleDescription(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) != 4 || !isAllDigits(house) {
		return false
	}
	year := 0
	for _, r := range house {
		year = year*10 + int(r-'0')
	}
	if year < 1980 || year > 2035 {
		return false
	}
	fields := strings.Fields(street)
	if len(fields) == 0 {
		return false
	}
	if len(fields) >= 2 && vehicleMakeTokens[fields[0]] {
		return true
	}
	u := " " + strings.ToUpper(transcript) + " "
	if vehicleColorTokens[fields[0]] {
		for _, body := range []string{" TRUCK", " SUV", " VAN", " SEDAN", " VEHICLE", " 4-DOOR", " 4 DOOR"} {
			if strings.Contains(u, body) {
				return true
			}
		}
		for make := range vehicleMakeTokens {
			if strings.Contains(u, make) {
				return true
			}
		}
	}
	return false
}

// AddressHasSttJunkStreetTail reports trailing STT garbage on a street name
// ("3901 SUPERIOR TAKES" from "superior thanks" on a gas leak).
func AddressHasSttJunkStreetTail(addr string) bool {
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	fields := strings.Fields(street)
	if len(fields) == 0 {
		return false
	}
	switch fields[len(fields)-1] {
	case "TAKES", "THANKS", "SEE", "ALONG", "THIS", "THAT", "YES", "SHOW", "SHOWING", "IT'S", "ITS", "I'M", "IM", "IT'LL", "ITLL":
		return true
	}
	if len(fields) >= 2 && fields[len(fields)-2] == "GATE" && fields[len(fields)-1] == "IT'S" {
		return true
	}
	return false
}

// AddressIsFacilityGateReference reports when STT glued a facility gate id to a
// fake street ("14 GATE IT'S" from "CHARLIE 14 GATE, IT'S A COFFEE SPILL").
func AddressIsFacilityGateReference(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, " GATE") && !strings.Contains(u, " GATE,") {
		return false
	}
	stFields := strings.Fields(street)
	if len(stFields) == 0 {
		return false
	}
	if stFields[0] == "GATE" {
		return true
	}
	if stFields[len(stFields)-1] == "GATE" && strings.Contains(u, " AT THE ") {
		return true
	}
	return false
}

// collectTranscriptDispatchAddresses gathers house+street candidates named in a
// toned dispatch (suffixed streets, suffixless forms, and known-street snaps).
func collectTranscriptDispatchAddresses(transcript string, knownStreets []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(a string) {
		a = strings.TrimSpace(strings.ToUpper(a))
		if a == "" || seen[a] {
			return
		}
		seen[a] = true
		out = append(out, a)
	}
	for _, a := range extractDispatchAddressesFromTranscript(transcript) {
		add(a)
	}
	if len(knownStreets) > 0 {
		for _, a := range extractKnownStreetAddressesFromTranscript(transcript, knownStreets) {
			add(a)
		}
	}
	return out
}

// TranscriptHasConflictingDispatchAddresses reports when the dispatch names two
// or more different house numbers as street addresses (e.g. a preamble stub
// before SQUAD CALL and the real toned location after). Repeated readbacks of
// the same house number are not treated as a conflict.
func TranscriptHasConflictingDispatchAddresses(transcript string, scope *ScopeData) bool {
	streets := []string(nil)
	if scope != nil {
		streets = scope.KnownStreets
	}
	houses := map[string]bool{}
	for _, a := range collectTranscriptDispatchAddresses(transcript, streets) {
		if !dispatchAddressCountsForConflictCheck(a, transcript, scope) {
			continue
		}
		h, _ := splitHouseAndStreet(a)
		if h == "" {
			continue
		}
		houses[h] = true
	}
	return len(houses) >= 2
}

// ConflictDispatchHouseNumbers lists distinct house numbers that trigger
// TranscriptHasConflictingDispatchAddresses (for eval/diagnostics).
func ConflictDispatchHouseNumbers(transcript string, scope *ScopeData) []string {
	streets := []string(nil)
	if scope != nil {
		streets = scope.KnownStreets
	}
	houses := map[string]bool{}
	for _, a := range collectTranscriptDispatchAddresses(transcript, streets) {
		if !dispatchAddressCountsForConflictCheck(a, transcript, scope) {
			continue
		}
		h, _ := splitHouseAndStreet(a)
		if h != "" {
			houses[h] = true
		}
	}
	out := make([]string, 0, len(houses))
	for h := range houses {
		out = append(out, h)
	}
	return out
}

// ExtractKnownStreetAddressesForTest exposes known-street harvesting for eval.
func ExtractKnownStreetAddressesForTest(transcript string, knownStreets []string) []string {
	return extractKnownStreetAddressesFromTranscript(transcript, knownStreets)
}

// SnapExtractedAddressToKnownStreetForTest exposes gazetteer snapping for eval.
func SnapExtractedAddressToKnownStreetForTest(addr string, knownStreets []string, transcript string) string {
	return snapExtractedAddressToKnownStreet(addr, knownStreets, transcript)
}

// ExtractDispatchAddressesForTest exposes dispatch-address harvesting for eval.
func ExtractDispatchAddressesForTest(transcript string) []string {
	return extractDispatchAddressesFromTranscript(transcript)
}

// CollectTranscriptDispatchAddresses exposes dispatch-address harvesting for tests.
func CollectTranscriptDispatchAddresses(transcript string, scope *ScopeData) []string {
	streets := []string(nil)
	if scope != nil {
		streets = scope.KnownStreets
	}
	return collectTranscriptDispatchAddresses(transcript, streets)
}

// dispatchAddressCountsForConflictCheck filters dispatch-address candidates that
// are really call types, apparatus ids, or other non-location fragments.
func dispatchAddressCountsForConflictCheck(addr, transcript string, scope *ScopeData) bool {
	house, street := splitHouseAndStreet(addr)
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	if house == "911" && (strings.Contains(street, "HANG") || strings.Contains(u, "911 HANG-UP") || strings.Contains(u, "911 HANG UP")) {
		return false
	}
	if stationHouseNumberLooksLikeUnit(house, transcript) ||
		AddressHouseNumberIsStationNumberConcatenation(addr, transcript) ||
		dispatchAddressIsApartmentUnitHarvest(addr, transcript) ||
		dispatchAddressIsGluedStationRollCall(addr, transcript) ||
		dispatchAddressIsFeetMangledDuplicate(addr, transcript) ||
		dispatchAddressIsLanesUnavailableStub(addr, transcript) ||
		AddressHouseNumberIsRouteFragment(addr, transcript) ||
		AddressHouseNumberIsBareStationToken(addr, transcript) ||
		AddressHouseNumberIsStationCorridorPOI(addr, transcript) ||
		AddressHouseNumberIsDispatchUnitPrefix(addr, transcript) ||
		AddressConflictsWithDispatchTimestamp(addr, transcript) ||
		dispatchAddressIsRoomExtensionFragment(addr, transcript) ||
		dispatchAddressStreetIsApparatusReference(street) ||
		dispatchAddressIsMutualAidUnitLabel(addr, transcript) ||
		dispatchAddressIsRequestAdminFragment(addr, transcript) ||
		dispatchAddressIsAlarmPanelZoneFragment(addr, transcript) ||
		dispatchAddressIsSubsumedByLongerDispatch(addr, transcript, scope) ||
		dispatchAddressIsTruncatedHouseOfLongerDispatch(addr, transcript, scope) ||
		dispatchAddressIsDistrictTownCoverage(addr, transcript) ||
		dispatchAddressIsRingRouteTimestamp(addr, transcript) ||
		AddressIsVehicleDescription(addr, transcript) ||
		AddressIsVehicleYearMisextract(addr, transcript) ||
		AddressIsConversationalNoise(addr) ||
		dispatchSuffixlessStreetIsNonLocationPhrase(street) ||
		dispatchAddressIsDigitInsertionDuplicate(addr, transcript) {
		return false
	}
	if hasStreetSuffix(street) {
		return true
	}
	if scope != nil {
		for _, ks := range scope.KnownStreets {
			ks = strings.ToUpper(strings.TrimSpace(ks))
			stU := strings.ToUpper(street)
			if stU == ks || strings.HasSuffix(ks, " "+stU) || strings.HasPrefix(ks, stU+" ") {
				return true
			}
		}
	}
	return suffixlessStreetNamePlausible(street) || suffixlessSingleStreetPlausible(street)
}

// dispatchAddressIsFeetMangledDuplicate reports STT debris like "650 FEET 2
// NORTH ROAD" where the trailing digit is not a separate dispatch address.
func dispatchAddressIsFeetMangledDuplicate(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, m := range feetMangledHouseRE.FindAllStringSubmatch(u, -1) {
		if len(m) < 3 || m[2] != house {
			continue
		}
		stHead := strings.Fields(street)
		if len(stHead) == 0 {
			continue
		}
		if strings.Contains(u, "FEET "+house+" "+stHead[0]) {
			return true
		}
	}
	return false
}

// dispatchAddressIsRoomExtensionFragment reports facility room/extension digits or
// clinical fragments mis-harvested as dispatch street addresses.
func dispatchAddressIsRoomExtensionFragment(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" {
		return false
	}
	u := strings.ToUpper(normalizeDispatchAddressSeparators(transcript))
	if dispatchHouseFromRoomPhoneExtension(house, u) {
		return true
	}
	return dispatchSuffixlessStreetIsNonLocationPhrase(street)
}

// dispatchAddressStreetIsApparatusReference reports bare tokens like STATION
// that dispatchers use before a station number ("1623. STATION 7") rather than
// as a street name.
func dispatchAddressStreetIsApparatusReference(street string) bool {
	switch strings.ToUpper(strings.TrimSpace(street)) {
	case "STATION", "STAGE", "DUTY":
		return true
	}
	return false
}

// dispatchAddressIsMutualAidUnitLabel reports apparatus numbers paired with
// "MUTUAL AID" ("FOR 37 TO 31, PLEASE, MUTUAL AID") rather than house addresses.
func dispatchAddressIsMutualAidUnitLabel(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" {
		return false
	}
	st := strings.ToUpper(strings.TrimSpace(street))
	if st != "MUTUAL AID" && st != "MUTUAL" {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, marker := range []string{
		" TO " + house + ",",
		" TO " + house + ".",
		" TO " + house + " ",
		house + " MUTUAL AID",
		house + ", MUTUAL AID",
	} {
		if strings.Contains(u, marker) {
			return true
		}
	}
	return false
}

// dispatchAddressIsRequestAdminFragment reports apparatus roll-call numbers glued
// to administrative phrasing ("31 THIS REQUEST COMES FROM …").
func dispatchAddressIsRequestAdminFragment(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	fields := strings.Fields(street)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "THIS", "REQUEST", "COMES", "ACCORDING", "NEED", "MULTIPLE":
		return true
	}
	if strings.HasPrefix(street, "THIS REQUEST") || strings.HasPrefix(street, "REQUEST COMES") {
		return true
	}
	return stationHouseNumberLooksLikeUnit(house, transcript)
}

// dispatchAddressIsAlarmPanelZoneFragment reports alarm-panel zone numbers misread
// as house numbers ("WATER FLOW 217. TIMEOUT" → "217 TIMEOUT").
func dispatchAddressIsAlarmPanelZoneFragment(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	switch strings.ToUpper(street) {
	case "TIMEOUT", "TIME", "FLOW", "ZONE", "POINT", "ALARM", "TROUBLE", "SUPERVISORY":
		return true
	}
	for _, cue := range []string{"WATER FLOW ", "FIRE FLOW ", "FLOW ", "ZONE ", "POINT ", "PANEL "} {
		if strings.Contains(u, cue+house) {
			return true
		}
	}
	return false
}

// dispatchAddressIsApartmentUnitHarvest reports apartment/room numbers misread as
// house+street ("219 SIXTY" from "APARTMENT 219. SIXTY-SIX-YEAR-OLD …").
func dispatchAddressIsApartmentUnitHarvest(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) > 4 || hasStreetSuffix(street) {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, prefix := range []string{
		"APARTMENT " + house,
		"APT " + house,
		"APT. " + house,
		"UNIT " + house,
		"ROOM " + house,
		"RM " + house,
		"RM. " + house,
	} {
		if strings.Contains(u, prefix) {
			return true
		}
	}
	return false
}

// dispatchAddressIsGluedStationRollCall reports STT-glued apparatus station ids
// at the tone opener ("3435, YOU'RE TURNING IN …" / "3435 SQUALL CALL …").
func dispatchAddressIsGluedStationRollCall(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || hasStreetSuffix(street) {
		return false
	}
	if AddressHouseNumberIsStationNumberConcatenation(addr, transcript) {
		return true
	}
	if len(house) != 4 || !isAllDigits(house) {
		return false
	}
	if !plausibleFireStationNumber(house[:2]) || !plausibleFireStationNumber(house[2:]) {
		return false
	}
	trim := strings.TrimSpace(strings.ToUpper(transcript))
	if !strings.HasPrefix(trim, house+",") && !strings.HasPrefix(trim, house+" ") {
		return false
	}
	return dispatchSuffixlessStreetIsNonLocationPhrase(street) ||
		dispatchAddressIsRequestAdminFragment(addr, transcript)
}

// dispatchAddressIsSubsumedByLongerDispatch drops prefix captures like
// "619 NORTH" when the same transcript also names "619 NORTH MAIN".
func dispatchAddressIsSubsumedByLongerDispatch(addr, transcript string, scope *ScopeData) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	streets := []string(nil)
	if scope != nil {
		streets = scope.KnownStreets
	}
	for _, other := range collectTranscriptDispatchAddresses(transcript, streets) {
		if strings.EqualFold(other, addr) {
			continue
		}
		oh, ost := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(other)))
		if oh != house {
			continue
		}
		if ost == street {
			continue
		}
		if strings.HasPrefix(ost, street+" ") {
			return true
		}
	}
	return false
}

// dispatchAddressIsTruncatedHouseOfLongerDispatch reports STT debris where a
// shorter digit run is harvested as its own house ("282 SOUTHERN KENFORD")
// while the real address repeats with more digits ("2821 VERA AVENUE").
func dispatchAddressIsTruncatedHouseOfLongerDispatch(addr, transcript string, scope *ScopeData) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || !isAllDigits(house) {
		return false
	}
	streets := []string(nil)
	if scope != nil {
		streets = scope.KnownStreets
	}
	for _, other := range collectTranscriptDispatchAddresses(transcript, streets) {
		if strings.EqualFold(other, addr) {
			continue
		}
		oh, ost := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(other)))
		if oh == "" || oh == house || !isAllDigits(oh) {
			continue
		}
		if !strings.HasPrefix(oh, house) || len(oh) <= len(house) {
			continue
		}
		// STT often re-reads the same house with a digit inserted ("3433" →
		// "34633"). That is not a longer true address subsuming a truncation.
		if houseNumbersAreDigitInsertionVariants(house, oh) {
			continue
		}
		// Prefer a longer house that carries a real thoroughfare (or is
		// restated) over a shorter prefix glued to unrelated words.
		if hasStreetSuffix(ost) || addressConfirmedByRepeatedDispatchFraming(other, transcript) {
			return true
		}
		if !hasStreetSuffix(street) && len(ost) >= len(street) {
			return true
		}
	}
	return false
}

// houseNumbersAreDigitInsertionVariants reports when two digit strings differ
// by exactly one inserted digit (3433 vs 34633) — a common STT re-read error.
func houseNumbersAreDigitInsertionVariants(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" || a == b || !isAllDigits(a) || !isAllDigits(b) {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b) != len(a)+1 {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[:i]+b[i+1:] == a {
			return true
		}
	}
	return false
}

// houseNumbersAreSingleDigitTypos reports same-length houses that differ by
// exactly one digit ("3460" vs "3470" on the same street) — STT re-read noise.
func houseNumbersAreSingleDigitTypos(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" || a == b || !isAllDigits(a) || !isAllDigits(b) || len(a) != len(b) {
		return false
	}
	diffs := 0
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			diffs++
			if diffs > 1 {
				return false
			}
		}
	}
	return diffs == 1
}

// houseNumbersAreSttTwins reports digit-insertion or single-digit re-read pairs.
func houseNumbersAreSttTwins(a, b string) bool {
	return houseNumbersAreDigitInsertionVariants(a, b) || houseNumbersAreSingleDigitTypos(a, b)
}

// dispatchAddressIsDigitInsertionDuplicate reports the longer mangled house
// when the transcript also has the shorter true house on the same street.
func dispatchAddressIsDigitInsertionDuplicate(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || !isAllDigits(house) {
		return false
	}
	streetCanon := CanonicalStreetName(street)
	for _, other := range extractDispatchAddressesFromTranscript(transcript) {
		if strings.EqualFold(other, addr) {
			continue
		}
		oh, ost := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(other)))
		if oh == "" || !isAllDigits(oh) {
			continue
		}
		if CanonicalStreetName(ost) != streetCanon && !strings.EqualFold(ost, street) {
			continue
		}
		if houseNumbersAreDigitInsertionVariants(house, oh) && len(house) > len(oh) {
			return true
		}
		// Prefer the earlier / fuller restatement when houses are one-digit twins.
		if houseNumbersAreSingleDigitTypos(house, oh) {
			if idxSelf := strings.Index(strings.ToUpper(transcript), house); idxSelf >= 0 {
				if idxOther := strings.Index(strings.ToUpper(transcript), oh); idxOther >= 0 && idxOther < idxSelf {
					return true
				}
			}
			_, selfSuf := streetNameAndSuffix(street)
			_, otherSuf := streetNameAndSuffix(ost)
			if selfSuf == "" && otherSuf != "" {
				return true
			}
		}
	}
	return false
}

// dispatchStreetIsApparatusStatus reports street tokens that are apparatus roster
// fragments ("DUTY OFF" from "28, DUTY, OFF DUTY"), not thoroughfare names.
func dispatchStreetIsApparatusStatus(street string) bool {
	switch strings.ToUpper(strings.TrimSpace(street)) {
	case "DUTY", "DUTY OFF", "OFF DUTY", "OFF-DUTY":
		return true
	}
	return false
}

// AddressIsDutyOffApparatusLabel reports when extraction glued apparatus roster
// tokens into a fake address ("28 DUTY OFF" from "28, DUTY, OFF DUTY, …").
func AddressIsDutyOffApparatusLabel(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if street == "" || !dispatchStreetIsApparatusStatus(street) {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, "OFF DUTY") || strings.Contains(u, "OFF-DUTY") || strings.Contains(u, "DUTY OFF DUTY") {
		return true
	}
	return house != "" && plausibleFireStationNumber(house)
}

// AddressHouseNumberIsSquadCallReferenceNumber reports when the house number is
// a CAD/incident letter reference after SQUAD CALL ("SQUAD CALL 1465 A") rather
// than a dispatcher-stated street ("SQUAD CALL 7731 BROOKWOOD DRIVE" or
// "SQUAD CALL, 5060 GIRDLE ROAD").
func AddressHouseNumberIsSquadCallReferenceNumber(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || len(house) < 3 {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, " SQUAD CALL ") && !strings.Contains(u, " SQUAD CALL,") {
		return false
	}
	// Real address forms — keep the house whether or not a comma follows.
	if hasStreetSuffix(street) {
		return false
	}
	stHead := strings.Fields(street)
	if len(stHead) == 0 {
		return false
	}
	if regexp.MustCompile(`(?i)\bSQUAD CALL\s*,?\s*` + regexp.QuoteMeta(house) + `\s*,?\s*` + regexp.QuoteMeta(stHead[0])).MatchString(u) &&
		len(stHead[0]) >= 3 {
		return false
	}
	if strings.Count(u, house+" "+stHead[0]) >= 2 {
		return false
	}
	if strings.Contains(u, house+" "+house+" "+stHead[0]) {
		return false
	}
	// Only strip when this house is SQUAD CALL <n> followed by a sector letter.
	m := regexp.MustCompile(`(?i)\bSQUAD CALL\s+` + regexp.QuoteMeta(house) + `\s+([A-Z]+)`).FindStringSubmatch(u)
	return len(m) == 2 && squadCallRefIsSectorLetter(m[1])
}

// dispatchAddressIsLanesUnavailableStub reports STT debris like "37, LANES NOT
// AVAILABLE" where the leading number is not a second dispatch address. Also
// rejects known-street snaps such as "37 LANE" from the same status phrase.
func dispatchAddressIsLanesUnavailableStub(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "LANES NOT AVAILABLE") &&
		!strings.Contains(u, "LANES ARE NOT AVAILABLE") &&
		!strings.Contains(u, "LANES ARE UNAVAILABLE") &&
		!strings.Contains(u, "LANES UNAVAILABLE") &&
		!strings.Contains(u, "LANE SPIES ARE UNAVAILABLE") {
		return false
	}
	stU := strings.ToUpper(street)
	if strings.HasPrefix(stU, "LANES NOT") ||
		strings.HasPrefix(stU, "LANES ") && strings.Contains(stU, "NOT") {
		return true
	}
	if stU == "LANE" || stU == "LANES" {
		return strings.Contains(u, house+", LANES") ||
			strings.Contains(u, house+" LANES") ||
			strings.HasPrefix(u, house+", LANES")
	}
	return false
}

// dispatchAddressIsDistrictTownCoverage reports FD district openers misread as
// house+street ("DISTRICT 91, SECOND TOWN" → "91 SECOND TOWN") that must not
// conflict with a real numbered address later in the same tone-out.
func dispatchAddressIsDistrictTownCoverage(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, "DISTRICT "+house) || strings.Contains(u, "DIST "+house) {
		return true
	}
	stU := strings.ToUpper(strings.TrimSpace(street))
	switch stU {
	case "SECOND TOWN", "FIRST TOWN", "THIRD TOWN", "FOURTH TOWN",
		"SECOND TONE", "FIRST TONE", "THIRD TONE":
		return true
	}
	fields := strings.Fields(stU)
	if len(fields) == 2 && (fields[1] == "TOWN" || fields[1] == "TONE") &&
		(fields[0] == "FIRST" || fields[0] == "SECOND" || fields[0] == "THIRD" || fields[0] == "FOURTH") {
		return true
	}
	return false
}

// dispatchAddressIsRingRouteTimestamp reports HHMM tokens from dispatch
// "YOUR RING ROUTE HAS 1739. IT'S FOR 3807 CUMBERLAND…" mis-harvested as
// "1739 IT'S" and wrongly conflicting with the real house number.
func dispatchAddressIsRingRouteTimestamp(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || !isAllDigits(house) || len(house) < 3 || len(house) > 4 {
		return false
	}
	u := strings.ToUpper(transcript)
	if AddressConflictsWithDispatchTimestamp(addr, transcript) {
		return true
	}
	stU := strings.ToUpper(strings.TrimSpace(street))
	if stU == "IT'S" || stU == "ITS" || dispatchSuffixlessStreetIsNonLocationPhrase(stU) {
		if strings.Contains(u, "RING ROUTE HAS "+house) ||
			strings.Contains(u, "YOUR RING ROUTE HAS "+house) ||
			strings.Contains(u, "RING ROUTE "+house) {
			return true
		}
	}
	return false
}

// maybeStripBareStationHouse drops a station roster id glued as a house number
// ("STATION 11, LARRY LANE" → "LARRY LANE").
func maybeStripBareStationHouse(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	if !AddressHouseNumberIsBareStationToken(curated.Address, transcript) {
		return
	}
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if street == "" {
		return
	}
	curated.Address = street
}

// maybeStripSquadCallReferenceHouse drops a bogus leading house number while
// keeping the street when SQUAD CALL is followed by an incident reference.
func maybeStripSquadCallReferenceHouse(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return
	}
	if !AddressHouseNumberIsSquadCallReferenceNumber(curated.Address, transcript) {
		return
	}
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if street == "" {
		return
	}
	curated.Address = street
}

// AddressIsPreambleBeforeSquadCall reports when the extracted address appears
// only before SQUAD CALL while a different house+street follows the toned opener.
func AddressIsPreambleBeforeSquadCall(addr, transcript string) bool {
	idx := squadCallIndex(transcript)
	if idx < 0 {
		return false
	}
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" {
		return false
	}
	u := strings.ToUpper(transcript)
	before := u[:idx]
	if !strings.Contains(before, house) {
		return false
	}
	for _, a := range extractDispatchAddressesFromTranscript(u[idx:]) {
		h2, _ := splitHouseAndStreet(a)
		if h2 != "" && h2 != house {
			return true
		}
	}
	return false
}

// AddressIsTruncatedStreetVersusTranscript reports when the extracted street is
// a strict prefix of a longer house+street phrase spoken on the toned dispatch
// ("256 ROSE" vs "256 ROSE GARDEN").
func AddressIsTruncatedStreetVersusTranscript(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	if len(strings.Fields(street)) != 1 {
		return false
	}
	for _, a := range extractDispatchAddressesFromTranscript(transcript) {
		h2, st2 := splitHouseAndStreet(a)
		if h2 == house && st2 != street && strings.HasPrefix(st2, street+" ") {
			if !dispatchHouseStreetPhraseInTranscript(transcript, h2, st2) {
				continue
			}
			return true
		}
	}
	u := strings.ToUpper(transcript)
	spokenNS := collapsedStreetWordsAfterHouse(u, house)
	stemNS := stripStreetSpaces(street)
	return len(spokenNS) > len(stemNS) && strings.HasPrefix(spokenNS, stemNS)
}

func dispatchHouseStreetPhraseInTranscript(transcript, house, street string) bool {
	u := " " + strings.ToUpper(strings.TrimSpace(transcript)) + " "
	phrase := strings.ToUpper(strings.TrimSpace(house + " " + street))
	if phrase == "" {
		return false
	}
	for _, tail := range []string{",", ".", ";", " "} {
		if strings.Contains(u, " "+phrase+tail) {
			return true
		}
	}
	return strings.HasSuffix(strings.TrimSpace(u), " "+phrase)
}

func squadCallIndex(transcript string) int {
	u := strings.ToUpper(transcript)
	for _, needle := range []string{" SQUAD CALL ", " SQUAD CALL,", " SQUAD CALL."} {
		if i := strings.Index(u, needle); i >= 0 {
			return i
		}
	}
	return -1
}

// DispatchAddressFailsLocalPinGates combines apparatus/station/route guards
// used before the local engine may place a pin.
func DispatchAddressFailsLocalPinGates(addr, transcript string, scope *ScopeData) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return true
	}
	if AddressHouseNumberIsEngineUnitConcatenation(addr, transcript) ||
		AddressHouseNumberFollowsEngineBlob(addr, transcript) ||
		AddressHouseNumberIsBareStationToken(addr, transcript) ||
		AddressHouseNumberIsStationCorridorPOI(addr, transcript) ||
		AddressHouseNumberIsStationNumberConcatenation(addr, transcript) ||
		AddressHouseNumberIsRouteFragment(addr, transcript) ||
		AddressHouseNumberIsDispatchUnitPrefix(addr, transcript) ||
		AddressHouseNumberIsBatchRunNumber(addr, transcript) ||
		AddressHouseNumberIsCallsignIdentifier(addr, transcript) ||
		AddressIsMunicipalityAgencyName(addr, transcript) ||
		TranscriptIsRadioTestAnnouncement(transcript) ||
		AddressHouseNumberNotSpokenInTranscript(addr, transcript) ||
		AddressIsFacilityGateReference(addr, transcript) ||
		AddressHasSttJunkStreetTail(addr) ||
		TranscriptIsDispatchClearSignOff(transcript) ||
		TranscriptIsRoutineUnitLog(transcript) ||
		TranscriptIsOfficerDirectedVisit(transcript) ||
		TranscriptIsHospitalBedCoordination(transcript) ||
		TranscriptHasConflictingDispatchAddresses(transcript, scope) ||
		AddressIsPreambleBeforeSquadCall(addr, transcript) ||
		AddressIsTruncatedStreetVersusTranscript(addr, transcript) ||
		AddressIsDutyOffApparatusLabel(addr, transcript) {
		return true
	}
	if !AddressHasGeocodableAnchor(addr, scope) {
		return true
	}
	if !AddressAlignsWithTranscript(addr, transcript, scope) {
		return true
	}
	return false
}
