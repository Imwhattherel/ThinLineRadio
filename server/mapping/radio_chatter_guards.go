// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Universal radio-chatter guards for status traffic, officer admin checks,
// garbled tone openers, and mileage/vehicle mis-extracts that should never pin.

package mapping

import (
	"regexp"
	"strings"
)

var unitMileageRepeatRE = regexp.MustCompile(`(?i)\b(\d{1,2})\s+MILES?\b`)

// unitRadioChannelRE matches apparatus/unit radio sign-offs ("101 RADIO, WE'RE CLEAR").
var unitRadioChannelRE = regexp.MustCompile(`(?i)\b(\d{1,4})\s+RADIO\b`)

// unitWillBeStatusRE matches apparatus availability ("RESCUE 1 WILL BE IN COURT")
// where the auxiliary verb WILL is misread as a street stem.
var unitWillBeStatusRE = regexp.MustCompile(`(?i)\b(?:RESCUE|MEDIC|ENGINE|SQUAD|LADDER|TRUCK|UNIT|CAR|BATTALION|CHANNEL|MED)\s+(\d{1,5})\s+WILL\s+BE\b`)

// dispatchStreetUnitWillBeRE matches suffixed harvests like "WILL BE IN COURT".
var dispatchStreetUnitWillBeRE = regexp.MustCompile(`(?i)^WILL\s+(?:BE|BOTH)\b`)

// officer10_3AtRE matches 10-3 / 103 officer checks after PreCleanTranscript may
// collapse "10-3" → "103".
var officer10_3AtRE = regexp.MustCompile(`(?i)\b(?:10[\s-]?3|103)\s+AT\b`)

var vehicleColorTokens = map[string]bool{
	"WHITE": true, "BLACK": true, "RED": true, "BLUE": true,
	"SILVER": true, "GRAY": true, "GREY": true, "GREEN": true, "GOLD": true,
}

// TranscriptBlocksIncidentPin reports radio traffic that may mention number-like
// tokens or street names but must not produce a map pin.
func TranscriptBlocksIncidentPin(transcript string) bool {
	if strings.TrimSpace(transcript) == "" {
		return false
	}
	return TranscriptIsSceneClearanceChatter(transcript) ||
		TranscriptIsUnitStatusChatter(transcript) ||
		TranscriptIsUnitMileageChatter(transcript) ||
		TranscriptIsOfficerAdminChatter(transcript) ||
		TranscriptIsOnSceneAdvisory(transcript) ||
		TranscriptIsLocationNarrative(transcript) ||
		TranscriptIsGarbledToneOpener(transcript) ||
		TranscriptIsUtilityFieldOpsChatter(transcript) ||
		TranscriptIsTowOrVehicleOpsChatter(transcript) ||
		TranscriptIsRadioCommsQualityChatter(transcript) ||
		TranscriptIsDispatchResourceStatusChatter(transcript) ||
		TranscriptIsUnitWillBeStatusChatter(transcript) ||
		TranscriptIsPatientClinicalFollowUp(transcript) ||
		TranscriptIsPersonDescriptionReadout(transcript) ||
		TranscriptIsUnitRadioSignOffChatter(transcript)
}

// TranscriptIsUnitRadioSignOffChatter reports unit radio check-ins and clear
// traffic ("101 RADIO, WE'RE CLEAR") mistaken for RADIO ROAD addresses.
func TranscriptIsUnitRadioSignOffChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := strings.ToUpper(transcript)
	if !unitRadioChannelRE.MatchString(transcript) {
		return false
	}
	for _, m := range []string{
		" WE'RE CLEAR", " WE ARE CLEAR", " WE'LL BE CLEAR", " WE WILL BE CLEAR",
		" I'LL GET THAT", " TIED UP WITH ", " SHOW YOU CLEAR", " YOU CAN CLEAR",
		" GOOD TO CLEAR", " WE'RE GOOD",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// AddressIsUnitRadioSignOffMisextract reports when a unit radio channel ("101
// RADIO") was snapped to a RADIO ROAD/ RD thoroughfare.
func AddressIsUnitRadioSignOffMisextract(addr, transcript string) bool {
	if TranscriptIsUnitRadioSignOffChatter(transcript) {
		return true
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	stFields := strings.Fields(street)
	if len(stFields) == 0 || stFields[0] != "RADIO" {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, house+" RADIO ROAD") || strings.Contains(u, house+" RADIO RD") ||
		strings.Contains(u, house+" RADIO STREET") || strings.Contains(u, house+" RADIO ST") {
		return false
	}
	for _, m := range unitRadioChannelRE.FindAllStringSubmatch(transcript, -1) {
		if len(m) >= 2 && m[1] == house {
			return true
		}
	}
	return strings.Contains(u, house+" RADIO")
}

// knownStreetHarvestIsUnitRadioSignOff blocks gazetteer harvest of "101 RADIO"
// from "101 RADIO, WE'RE CLEAR" when RADIO is the channel not a street stem.
func knownStreetHarvestIsUnitRadioSignOff(house, streetStem, transcript string) bool {
	streetStem = strings.ToUpper(strings.TrimSpace(streetStem))
	if streetStem != "RADIO" {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, house+" RADIO ROAD") || strings.Contains(u, house+" RADIO RD") ||
		strings.Contains(u, house+" RADIO STREET") || strings.Contains(u, house+" RADIO ST") {
		return false
	}
	if !strings.Contains(u, house+" RADIO") {
		return false
	}
	return TranscriptIsUnitRadioSignOffChatter(transcript) ||
		strings.Contains(u, " WE'RE CLEAR") || strings.Contains(u, " WE ARE CLEAR") ||
		strings.Contains(u, " I'LL GET THAT") || strings.Contains(u, " TIED UP WITH ")
}

// TranscriptIsUnitWillBeStatusChatter reports unit availability updates where
// the auxiliary verb WILL is glued to a status phrase ("RESCUE 1 WILL BE IN
// COURT", "RESCUE 1 WILL BE ON THE WAY") — not a street address.
func TranscriptIsUnitWillBeStatusChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := strings.ToUpper(transcript)
	if unitWillBeStatusRE.MatchString(transcript) {
		return true
	}
	for _, p := range []string{
		" WILL BE IN COURT", " WILL BE IN THE COURT",
		" WILL BE ON THE WAY", " WILL BE ON THEIR WAY",
		" WILL BOTH BE CLEAR", " WILL BOTH BE",
	} {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}

// TranscriptIsPatientClinicalFollowUp reports medic unit patient descriptions
// with demographics/symptoms but no dispatch location ("IT'LL BE FOR A 29-YEAR-OLD
// FEMALE, NAUSEOUS… WEAK AND BUSY").
func TranscriptIsPatientClinicalFollowUp(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	// Age + symptom is normal toned EMS dispatch copy, not a post-arrival clinical
	// update — keep the house+street pin ("4100 WESTBROOK … 41-YEAR-OLD … FEVER").
	if transcriptHasCleanNumberedDispatchAddress(transcript) {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "-YEAR-OLD") && !strings.Contains(u, " YEAR OLD") {
		return false
	}
	for _, m := range []string{
		"NAUSEOUS", "NAUSEA", "VOMITING", "WEAK", "WEAKNESS", "DIZZY", "DIZZINESS",
		"UNCONSCIOUS", "BREATHING", "CHEST PAIN", "ABDOMINAL", "SEIZURE", "FAINT",
		"LIGHTHEADED", "FEVER", "CHILLS",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return strings.Contains(u, " WEAK AND ") || strings.Contains(u, " WEAK & ")
}

// TranscriptIsPersonDescriptionReadout reports BOLO/person physical descriptions
// with age + sex and height/weight/color, but no dispatch location
// ("54-YEAR-OLD FEMALE, 5'2\", 150, BURNT GREEN").
func TranscriptIsPersonDescriptionReadout(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "-YEAR-OLD") && !strings.Contains(u, " YEAR OLD") {
		return false
	}
	if !strings.Contains(u, "FEMALE") && !strings.Contains(u, "MALE") {
		return false
	}
	if personHeightRE.MatchString(transcript) {
		return true
	}
	for _, m := range []string{
		" BURNT ", " IN COLOR", " LBS", " POUNDS", " DARK GREEN", " LIGHT BLUE",
		" RED HAIR", " BROWN HAIR", " BLOND", " BLONDE",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

var personHeightRE = regexp.MustCompile(`\d'\s*\d|\b\d\s*FOOT\b|\b\d\s*FT\b`)

// houseNumberFromUnitWillBeStatus reports when a captured house number is the
// apparatus id before "WILL BE" status phrasing.
func houseNumberFromUnitWillBeStatus(house, transcript string) bool {
	h := strings.TrimSpace(house)
	if h == "" {
		return false
	}
	for _, m := range unitWillBeStatusRE.FindAllStringSubmatch(transcript, -1) {
		if len(m) >= 2 && m[1] == h {
			return true
		}
	}
	u := strings.ToUpper(transcript)
	if strings.Contains(u, h+" WILL BE IN COURT") ||
		strings.Contains(u, h+" WILL BE ON THE WAY") ||
		strings.Contains(u, h+" WILL BOTH BE CLEAR") {
		return true
	}
	return false
}

// dispatchStreetIsUnitWillBeStatusPhrase reports suffixed captures whose stem
// is the auxiliary verb WILL ("1 WILL BE IN COURT", "1 WILL BE ON THE WAY").
func dispatchStreetIsUnitWillBeStatusPhrase(street string) bool {
	return dispatchStreetUnitWillBeRE.MatchString(strings.TrimSpace(street))
}

// dispatchStreetIsRadioSignOff rejects timeout/hand-off chatter harvested as a
// street ("1706 YOUR TURN", "1225 YOUR TIME OUT").
func dispatchStreetIsRadioSignOff(street string) bool {
	u := strings.ToUpper(strings.TrimSpace(street))
	if u == "" {
		return false
	}
	return strings.HasPrefix(u, "YOUR TURN") || strings.HasPrefix(u, "YOUR TIME") ||
		strings.HasPrefix(u, "TIMEOUT") || strings.HasPrefix(u, "TIME OUT") ||
		u == "TURN"
}

// radioCommsNoiseTerms are STT tokens that fuzzy-match real street names but
// name radio/audio quality in status traffic (STATIC→STACEY, BROKEN→BROOKE).
var radioCommsNoiseTerms = map[string]bool{
	"STATIC": true, "BROKEN": true, "GARBLED": true, "SCRATCHY": true,
	"GARBAGE": true, "FEEDBACK": true, "INTERFERENCE": true,
}

// TokenIsRadioCommsNoise reports tokens that must not be treated as streets or
// intersection sides even when they fuzzy-match the gazetteer.
func TokenIsRadioCommsNoise(tok string) bool {
	return radioCommsNoiseTerms[strings.ToUpper(strings.TrimSpace(tok))]
}

// TranscriptIsRadioCommsQualityChatter reports copy/ack traffic about signal
// quality with no toned dispatch body (e.g. "COPY, BUT IT'S STILL STATIC AND BROKEN").
func TranscriptIsRadioCommsQualityChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, p := range []string{
		"STILL STATIC", "STATIC AND BROKEN", "IT'S STILL STATIC", "ITS STILL STATIC",
		"RADIO CHECK", "HOW DO YOU COPY", "OPEN MIC", "BREAKING UP", "CUTTING OUT",
		"BAD AUDIO", "HARD TO HEAR", "CAN'T HEAR", "CANT HEAR", "DEAD AIR",
		"WON'T COPY", "BARELY COPY", "SCRATCHY", "GARBLED",
	} {
		if strings.Contains(u, p) {
			return true
		}
	}
	trimmed := strings.TrimSpace(u)
	if strings.HasPrefix(trimmed, "COPY") && len(strings.Fields(trimmed)) <= 12 {
		for tok := range radioCommsNoiseTerms {
			if strings.Contains(u, tok) {
				return true
			}
		}
	}
	return false
}

// TranscriptIsDispatchResourceStatusChatter reports mutual-aid / crew-availability
// updates that name jurisdictions but not a dispatch location.
func TranscriptIsDispatchResourceStatusChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	if strings.Contains(u, " CHANGE ON YOUR CODE ") {
		return true
	}
	if strings.Contains(u, " WITHOUT A FULL CREW") &&
		(strings.Contains(u, " CLEARED") || strings.Contains(u, " RESPONDING")) {
		return true
	}
	if strings.Contains(u, " ONLY HAVE ONE") && strings.Contains(u, " WOULD NOT BE RESPONDING") {
		return true
	}
	if strings.Contains(u, " THEY CLEARED") && strings.Contains(u, " UNIT IN ROUTE") {
		return true
	}
	if strings.Contains(u, " I DID CALL ") && strings.Contains(u, " UNIT IN ROUTE FROM INSIDE THE TOWNSHIP") {
		return true
	}
	return false
}

func transcriptBlocksPinBaseOK(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	return !containsRealIncidentMarker(u) && !stationDispatchRE.MatchString(transcript)
}

// TranscriptIsSceneClearanceChatter reports scene clearances and code-6 style
// sign-offs that mention a number but are not new dispatches.
func TranscriptIsSceneClearanceChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	for _, m := range []string{
		" CODE 4", " CODE FOUR", " CODE 6", " CODE 6TH", " NO NEED FOR CHECK", " PATIENTS LOADING", " WE'LL BE CLEAR", " WE WILL BE CLEAR",
		" WE'RE CLEAR", " WE ARE CLEAR",
		" ALL DONE ANYWAY", " HE'S ALL DONE", " HES ALL DONE", " LINCOLN CLEAR", " SCENE CLEAR",
		" THEY CLEARED", " SO THEY CLEARED",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// TranscriptIsUnitStatusChatter reports apparatus/unit status without a new incident.
func TranscriptIsUnitStatusChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	for _, m := range []string{
		" SERVICE RETURNING", " OPEN MIC", " IS RESPONDING",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	if strings.Contains(u, " ANCHOR ") && strings.Contains(u, " RESPONDING") {
		return true
	}
	return false
}

// TranscriptIsUnitMileageChatter reports repeated unit mileage readouts ("18 MILES, 18 MILES")
// mistaken for a street address.
func TranscriptIsUnitMileageChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := strings.ToUpper(transcript)
	if strings.Count(u, " MILES") < 2 {
		return false
	}
	return unitMileageRepeatRE.MatchString(u)
}

// TranscriptIsOnSceneAdvisory reports size-ups and status traffic from units
// already on scene that are not toned dispatches naming a new incident location.
func TranscriptIsOnSceneAdvisory(transcript string) bool {
	if strings.TrimSpace(transcript) == "" {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	if containsRealIncidentMarker(u) || stationDispatchRE.MatchString(transcript) {
		return false
	}
	if strings.Contains(u, " ON SCENE FOR STATION") || strings.Contains(u, "FAMILIES ON SCENE") ||
		strings.Contains(u, " ON SCENE REQUESTING") {
		return false
	}
	if !strings.Contains(u, " ON SCENE") && !strings.Contains(u, " ON-SCENE") {
		return false
	}
	// "THE CALLER'S NOT ON SCENE ADVISING …" — the 911 caller (a bystander or
	// family member) explicitly is NOT at the location and is phoning in a
	// live report (e.g. CO detector readings). This is the opposite signal of
	// the "unit already arrived, giving a status update" chatter this
	// function screens for, and is a real new-dispatch narrative that must
	// not be blocked from pinning.
	if strings.Contains(u, " NOT ON SCENE") || strings.Contains(u, " NOT ON-SCENE") {
		return false
	}
	// Size-ups that restate a house-number + street address are still map-worthy
	// ("798 KENTWOOD DRIVE. 798 KENTWOOD DRIVE. … PD'S ON SCENE").
	if transcriptHasCleanNumberedDispatchAddress(transcript) {
		return false
	}
	for _, m := range []string{
		" ON SCENE ADVISING", " IS ON SCENE ADVISING", " ON SCENE WITH ",
		" SQUAD ON SCENE", " ENGINE SQUAD ON SCENE", " FIRE IS ON SCENE",
		" PD IS ON SCENE", " POLICE IS ON SCENE", " WE HAVE A ",
		" NO OTHER UNITS", " APPEARS CODE 6", " LOOKS LIKE A ",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	if !strings.Contains(u, " RESPOND") && !strings.Contains(u, " SQUAD CALL") &&
		!strings.Contains(u, " FOR A ") && !strings.Contains(u, " DISPATCH") {
		return true
	}
	return false
}

// TranscriptIsOfficerAdminChatter reports officer checks, walk-offs, lobby visits,
// and follow-up logs that are not toned dispatches.
func TranscriptIsOfficerAdminChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	if strings.Contains(u, " WALK-OFF") || strings.Contains(u, " WALK OFF") ||
		strings.Contains(u, " MALE WALK-OFF") || strings.Contains(u, " FEMALE WALK-OFF") {
		return true
	}
	if strings.Contains(u, " GENTLEMAN IN THE LOBBY") || strings.Contains(u, " IN THE LOBBY TO SPEAK") {
		return true
	}
	if strings.Contains(u, " I'LL BE OUT AT") &&
		(strings.Contains(u, " FOLLOW UP") || strings.Contains(u, " FOLLOW-UP")) {
		return true
	}
	if transcriptHasOfficer10_3Check(transcript) {
		return true
	}
	if strings.Contains(u, " UNDER THIS CALL LOG ") {
		return true
	}
	return false
}

func transcriptHasOfficer10_3Check(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	if strings.Contains(u, " MOTION ALARM") || strings.Contains(u, " FIRE ALARM") ||
		strings.Contains(u, " BURGLAR ALARM") || strings.Contains(u, " ALARM DROP") {
		return false
	}
	if officer10_3AtRE.MatchString(transcript) {
		return true
	}
	if strings.Contains(u, " 10-3 FOR ") || strings.Contains(u, " 10 3 FOR ") ||
		strings.Contains(u, " 103 FOR ") {
		return true
	}
	return false
}

// TranscriptIsGarbledToneOpener reports STATION openers with nonsense or remote
// place names and no real dispatch body (SCOTT HALL / COLUMBUS garble).
func TranscriptIsGarbledToneOpener(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	if !strings.Contains(u, " STATION ") {
		return false
	}
	if containsRealIncidentMarker(u) || stationDispatchRE.MatchString(transcript) {
		return false
	}
	for _, m := range []string{
		" SCOTT HALL", " WALMART, GOLDIE", " COLUMBUS.",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// TranscriptIsUtilityFieldOpsChatter reports water/utility field coordination that
// names an address only as a work site, not a public-safety dispatch.
func TranscriptIsUtilityFieldOpsChatter(transcript string) bool {
	if !transcriptBlocksPinBaseOK(transcript) {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	if strings.Contains(u, " CAN'T FIND A METER") || strings.Contains(u, " CAN NOT FIND A METER") ||
		strings.Contains(u, " CANNOT FIND A METER") {
		return true
	}
	if strings.Contains(u, " CAN YOU SEND ME A PICTURE OF ") {
		return true
	}
	if strings.Contains(u, " CAN YOU CALL 330-") || strings.Contains(u, " CAN YOU CALL 330 ") {
		return true
	}
	return false
}

// TranscriptIsTowOrVehicleOpsChatter reports tow/vehicle descriptions mistaken for addresses.
func TranscriptIsTowOrVehicleOpsChatter(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	if containsRealIncidentMarker(u) || stationDispatchRE.MatchString(transcript) {
		return false
	}
	return strings.Contains(u, " WHEELS LT OUT OF") ||
		strings.Contains(u, " NEED A TOW FOR A ") ||
		strings.Contains(u, " TOW FOR A ")
}

// AddressIsUnitWillBeStatusMisextract reports when WILL/WILL CT/WILL WAY came from
// unit status phrasing rather than a dispatcher-stated street.
func AddressIsUnitWillBeStatusMisextract(addr, transcript string) bool {
	if TranscriptIsUnitWillBeStatusChatter(transcript) {
		return true
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" {
		return false
	}
	if !houseNumberFromUnitWillBeStatus(house, transcript) {
		return false
	}
	stFields := strings.Fields(street)
	if len(stFields) == 0 || stFields[0] == "WILL" {
		return true
	}
	return dispatchStreetIsUnitWillBeStatusPhrase(street)
}

// AddressIsUnitMileageReference reports when a unit mileage readout was snapped to
// a MILES street ("18 MILES, 18 MILES" → 18 MILES ST).
func AddressIsUnitMileageReference(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" {
		return false
	}
	stFields := strings.Fields(street)
	if len(stFields) == 0 || stFields[0] != "MILES" {
		return false
	}
	u := strings.ToUpper(transcript)
	matches := unitMileageRepeatRE.FindAllStringSubmatch(u, -1)
	if len(matches) < 2 {
		return false
	}
	for _, m := range matches {
		if len(m) == 2 && m[1] == house {
			return true
		}
	}
	return strings.Count(u, " MILES") >= 2 && strings.Count(u, house+" MILES") >= 1
}

// AddressIsVehicleYearMisextract reports when a model year was used as a house number
// while the make appears only in vehicle description ("2018 CHEVY" vs 2018 CHERRY AVE).
func AddressIsVehicleYearMisextract(addr, transcript string) bool {
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
	u := " " + strings.ToUpper(transcript) + " "
	for make := range vehicleMakeTokens {
		if !strings.Contains(u, make) {
			continue
		}
		if vehicleMakeTokens[strings.Fields(street)[0]] {
			return false
		}
		if strings.Contains(street, make) {
			return false
		}
		return true
	}
	return false
}

// localApproximateCentroidAllowed gates fuzzy street/cross centroids to toned dispatches.
func localApproximateCentroidAllowed(cleaned string) bool {
	u := " " + strings.ToUpper(cleaned) + " "
	if containsRealIncidentMarker(u) || stationDispatchRE.MatchString(cleaned) {
		return true
	}
	return false
}
