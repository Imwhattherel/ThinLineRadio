// Copyright (C) 2025 Thinline Dynamic Solutions
//
// auto_learn_purge.go — remove stale auto-learned address pins that conflict
// with a freshly resolved dispatch or sit outside the active geo bounds.

package mapping

import (
	"fmt"
	"strings"
)

const autoLearnCoordMismatchMi = 0.5

// ShouldPurgeAutoLearnPlace reports whether a stored auto_learn pin should be
// deleted after processing a call with curated address/coords.
func ShouldPurgeAutoLearnPlace(curated *CuratedAlert, place *KnownPlace, geo *GeoOptions) bool {
	if place == nil || !strings.EqualFold(strings.TrimSpace(place.Source), "auto_learn") {
		return false
	}
	if geo != nil && geo.BoundsRadiusMi > 0 && PinOutsideCoverage(place.Lat, place.Lon, geo) {
		return true
	}
	if curated == nil || strings.TrimSpace(curated.Address) == "" {
		return false
	}
	house, st := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if house == "" || st == "" {
		return false
	}
	placeHouse, placeSt := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(place.DisplayName)))
	if placeHouse == "" || placeSt == "" || house != placeHouse {
		return false
	}
	if knownPlaceAddressKeyConflicts(curated, place) {
		return true
	}
	if !addressStreetEquivalent(st, placeSt) {
		return true
	}
	if curated.Lat == "" || curated.Lon == "" {
		return false
	}
	var clat, clon float64
	fmt.Sscanf(curated.Lat, "%f", &clat)
	fmt.Sscanf(curated.Lon, "%f", &clon)
	if clat == 0 && clon == 0 {
		return false
	}
	distMi := haversineMeters(place.Lat, place.Lon, clat, clon) / 1609.344
	return distMi > autoLearnCoordMismatchMi
}

// CollectAutoLearnPlacesToPurge returns auto_learn pins in scope that conflict
// with the resolved call and should be removed from the database.
func CollectAutoLearnPlacesToPurge(scope *ScopeData, curated *CuratedAlert, geo *GeoOptions) []KnownPlace {
	if scope == nil || curated == nil {
		return nil
	}
	var out []KnownPlace
	seen := map[string]bool{}
	for _, place := range scope.KnownPlaces {
		if !ShouldPurgeAutoLearnPlace(curated, &place, geo) {
			continue
		}
		key := strings.TrimSpace(place.PlaceKey)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, place)
	}
	return out
}
