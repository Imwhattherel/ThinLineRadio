// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Guards against snapping a real street address to an unrelated retail POI when
// the LLM copies a vehicle/fleet brand from the transcript ("COACH TRUCK" →
// common_name COACH → Coach outlet store at the mall).

package mapping

import "strings"

// sanitizeFacilityCommonName clears facility common names that are really
// vehicle/fleet brands mentioned in the transcript.
func sanitizeFacilityCommonName(curated *CuratedAlert, transcript string) {
	if curated == nil {
		return
	}
	cn := strings.ToUpper(strings.TrimSpace(curated.CommonName))
	if cn == "" {
		return
	}
	if transcriptUsesBrandAsVehicle(transcript, cn) {
		curated.CommonName = ""
	}
}

// transcriptUsesBrandAsVehicle reports when a word is a motorcoach/fleet
// reference rather than a facility name ("TAKE A COACH TRUCK").
func transcriptUsesBrandAsVehicle(transcript, brand string) bool {
	brand = strings.ToUpper(strings.TrimSpace(brand))
	if brand == "" {
		return false
	}
	u := " " + strings.ToUpper(transcript) + " "
	for _, tail := range []string{" TRUCK", " BUS", " MOTORCOACH", " TRAILER", " VAN"} {
		if strings.Contains(u, " "+brand+tail) {
			return true
		}
	}
	if brand == "COACH" && strings.Contains(u, " COACH TRUCK") {
		return true
	}
	return false
}

// knownPlaceConflictsWithCuratedAddress reports when a POI common-name match
// would override a fully extracted house+street that shares no street token with
// the place (13002 LORRAINE AVENUE must not snap to the COACH outlet store).
func knownPlaceConflictsWithCuratedAddress(curated *CuratedAlert, place *KnownPlace) bool {
	if curated == nil || place == nil || !curatedHasHouseAndStreet(curated) {
		return false
	}
	_, st := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	stCanon := canonicalStreetTokens(st)
	if stCanon == "" {
		return false
	}
	placeText := canonicalStreetTokens(place.DisplayName + " " + place.AddressHint)
	for _, w := range strings.Fields(stCanon) {
		if len(w) < 4 {
			continue
		}
		if strings.Contains(placeText, w) {
			return false
		}
	}
	return true
}

// knownPlaceAddressKeyConflicts reports when an auto-learned address-key pin
// does not match the extracted street type (150 CHARLES COURT vs 150 CHARLES AVENUE).
func knownPlaceAddressKeyConflicts(curated *CuratedAlert, place *KnownPlace) bool {
	if curated == nil || place == nil {
		return false
	}
	_, st := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	_, placeSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(place.DisplayName)))
	if st == "" || placeSt == "" {
		return false
	}
	_, stSuffix := streetNameAndSuffix(st)
	_, placeSuffix := streetNameAndSuffix(placeSt)
	if stSuffix == "" || placeSuffix == "" {
		return false
	}
	if streetSuffixesCompatible(stSuffix, placeSuffix) {
		return false
	}
	return !addressStreetEquivalent(st, placeSt)
}

// maybeSalvageInTheFacility captures "131 WEST BOURBON STREET IN THE MEDIA PLAZA"
// facility qualifiers and strips the trailing preposition from the street field.
func maybeSalvageInTheFacility(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(transcript) == "" {
		return
	}
	sub := localInTheFacilityRE.FindStringSubmatch(transcript)
	if len(sub) < 2 {
		return
	}
	facility := strings.ToUpper(strings.TrimSpace(sub[1]))
	if facility == "" {
		return
	}
	if strings.TrimSpace(curated.CommonName) == "" {
		curated.CommonName = facility
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if street == "" {
		return
	}
	street = dispatchStreetCutAtConnector(street)
	street = stripAddressNarrativeTail(strings.TrimSpace(house + " " + street))
	if house != "" {
		_, street = splitHouseAndStreet(street)
	}
	if house != "" && street != "" {
		curated.Address = strings.TrimSpace(house + " " + street)
	} else if street != "" {
		curated.Address = street
	}
}
