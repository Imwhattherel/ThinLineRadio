// Copyright (C) 2025 Thinline Dynamic Solutions
//
// dispatch_locality.go — helpers for the dispatch-named community a caller
// explicitly resolves to (mutual-aid destinations, DispatchSpokenLocality).
// Mining free-text city/community names out of every transcript for geocode
// bias was removed: it produced false positives on medical/radio phrasing
// (e.g. "NEGATIVE ILLNESS" read as a city). Coverage bias now comes only from
// admin-configured talkgroup/tone-set geo bounds and explicit mutual-aid
// destinations.

package mapping

import (
	"strings"
)

var dispatchLocalityStopwords = map[string]bool{
	"STRUCTURE": true, "BASEMENT": true, "ALARM": true, "FIRE": true,
	"MEDICAL": true, "SQUAD": true, "STATION": true, "MUTUAL": true,
	"OFF": true, "DUTY": true, "OFF-DUTY": true, "ASSISTING": true,
	"SECOND": true, "TONES": true, "TONE": true, "THE": true, "ITS": true,
	// Dispatcher self-correction/clarification fillers — never a place name.
	"CORRECTION": true, "CORRECT": true, "DISREGARD": true, "REPEAT": true,
	"REPEATING": true, "AGAIN": true, "UPDATE": true, "UPDATED": true,
	// Property/scene descriptors from "IT'S ABANDONED / VACANT / OCCUPIED" —
	// adjectives about the location, not the community name.
	"ABANDONED": true, "VACANT": true, "OCCUPIED": true, "UNOCCUPIED": true,
	// Certainty hedges that lead a nature description ("POSSIBLE CARDIAC
	// ARREST, 602 GARY DRIVE") — no real community is ever named "Possible" or
	// "Active", so any candidate opening with one of these is dispatch
	// narration about the call, not a place, regardless of what noun follows.
	"POSSIBLE": true, "PROBABLE": true, "SUSPECTED": true, "REPORTED": true,
	"UNCONFIRMED": true, "CONFIRMED": true, "ACTIVE": true,
}

// dispatchFacilityMarkers identify business/facility names that must not be
// treated as embedded dispatch cities ("BELLARIA PIZZA, 882 …").
var dispatchFacilityMarkers = []string{
	"PIZZA", "RECOVERY", "REHAB", "CLUB", "APARTMENTS", "APARTMENT", "RESTAURANT", "CLINIC",
	"HOSPITAL", "NURSING", "CENTER", "STORE", "MARKET", "DONUTS", "DONUT",
	"CHURCH", "SCHOOL", "LOUNGE", "TAVERN", "GRILL", "DINER", "MOTEL",
	"HOTEL", "PHARMACY", "AUTO", "SALON", "MANOR", "MOBILE", "LIBRARY",
	"MUSEUM", "PLAZA", "MEDICAL", "UNIVERSITY", "COLLEGE", "VETERANS", "VA ",
}

func isDispatchFacilityName(name string) bool {
	u := strings.ToUpper(strings.TrimSpace(name))
	for _, m := range dispatchFacilityMarkers {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

func normalizeDispatchLocalityName(city string) string {
	return strings.TrimSpace(city)
}

func dispatchLocalityNamePlausible(city string) bool {
	if len(city) < 3 || len(strings.Fields(city)) > 3 || startsWithDigit(city) {
		return false
	}
	for _, w := range strings.Fields(city) {
		if dispatchLocalityStopwords[w] {
			return false
		}
	}
	if isDispatchFacilityName(city) {
		return false
	}
	for _, bad := range []string{"RECOVERY", "MALE", "FEMALE", "YEAR", "OLD", "CHEST", "PAIN", "PAINS"} {
		if strings.Contains(city, bad) {
			return false
		}
	}
	return true
}

// DispatchSpokenLocalityFromGeo returns a city/community the dispatcher named in
// the transcript, excluding the tone-set GeoCity label.
func DispatchSpokenLocalityFromGeo(geo *GeoOptions) string {
	if geo == nil {
		return ""
	}
	return normalizeDispatchLocalityName(strings.TrimSpace(geo.DispatchSpokenLocality))
}

// PinContradictsSpokenLocality reports when a geocoder claims the dispatch-named
// community in the formatted address but places the pin far from that place.
func PinContradictsSpokenLocality(lat, lon float64, formatted string, geo *GeoOptions) bool {
	spoken := DispatchSpokenLocalityFromGeo(geo)
	if spoken == "" || geo == nil || geo.SkipExternalGeocode {
		return false
	}
	fu := strings.ToUpper(strings.TrimSpace(formatted))
	if !strings.Contains(fu, strings.ToUpper(spoken)) {
		return false
	}
	// A pin inside the resolved boundary disc of the spoken community is by
	// definition in that community — no external verification needed.
	if pinInsideSpokenLocalityDisc(lat, lon, geo) {
		return false
	}
	state := strings.TrimSpace(geo.State)
	if state == "" {
		state = DeriveState(geo.LocationContext, "", geo.CityHint)
	}
	q := spoken
	if state != "" {
		q += ", " + state
	}
	clat, clon, _, ok, _ := GeocodeCensusOneLineTracked(q, geo)
	if !ok || clat == 0 || clon == 0 {
		// Cannot verify place center; reject city-labeled matches for scoped dispatches.
		return true
	}
	return haversineMeters(lat, lon, clat, clon)/1609.34 > 4.0
}
