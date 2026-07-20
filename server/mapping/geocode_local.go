// Copyright (C) 2025 Thinline Dynamic Solutions
//
// geocode_local.go — coverage/pin validation helpers. Street pins come from
// Nominatim; local OSM geometry lookups were removed.

package mapping

import (
	"fmt"
	"regexp"
	"strings"
)

// LatLon is a single WGS-84 coordinate.
type LatLon struct {
	Lat float64
	Lon float64
}

func finalizeLocalCoords(curated *CuratedAlert, geoOpts *GeoOptions) bool {
	clearLocalPinOutsideCoverage(curated, geoOpts)
	return curated.Lat != "" && curated.Lon != ""
}

// coverageMaxRadiusMi returns the maximum distance from the service-area center
// for accepting or retaining a pin: configured radius plus geoBoundsBufferMiles.
func coverageMaxRadiusMi(geo *GeoOptions) float64 {
	_, _, radiusMi, ok := geocodeBiasCenterMi(geo)
	if !ok {
		return 0
	}
	return radiusMi + geoBoundsBufferMiles
}

func pinOutsideCoverageBias(lat, lon float64, geo *GeoOptions) bool {
	biasLat, biasLon, _, ok := geocodeBiasCenterMi(geo)
	if !ok {
		return false
	}
	maxMi := coverageMaxRadiusMi(geo)
	if maxMi <= 0 {
		return false
	}
	if haversineMeters(biasLat, biasLon, lat, lon) <= maxMi*1609.34 {
		return false
	}
	// Mutual-aid dispatches legitimately land outside the home radius; a pin
	// inside the dispatch-spoken community's disc is in coverage.
	return !pinInsideSpokenLocalityDisc(lat, lon, geo)
}

// pinInsideSpokenLocalityDisc reports whether a pin sits inside the secondary
// allowed disc around the community the dispatcher named (resolved from
// imported boundary centroids by the caller).
func pinInsideSpokenLocalityDisc(lat, lon float64, geo *GeoOptions) bool {
	if geo == nil || geo.SpokenLocalityRadiusMi <= 0 ||
		(geo.SpokenLocalityLat == 0 && geo.SpokenLocalityLon == 0) {
		return false
	}
	return haversineMeters(geo.SpokenLocalityLat, geo.SpokenLocalityLon, lat, lon)/1609.34 <= geo.SpokenLocalityRadiusMi
}

// ohioPAEasternBorderLon is the approximate OH/PA state line in the
// Trumbull–Mercer corridor; coordinates east of this band are Pennsylvania.
const ohioPAEasternBorderLon = -80.5185

func pinEastOfOhioPABorder(lat, lon float64) bool {
	if lat < 40.85 || lat > 41.65 {
		return false
	}
	return lon > ohioPAEasternBorderLon
}

func pinOutsideConfiguredState(lat, lon float64, geo *GeoOptions) bool {
	if geo == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(geo.State)) {
	case "ohio", "oh":
		return pinEastOfOhioPABorder(lat, lon)
	default:
		return false
	}
}

// pinOutsideAbsoluteHomeCeiling enforces GeoOptions.HomeLat/HomeLon/
// HomeMaxRadiusMi unconditionally — independent of whatever BoundsLat/
// BoundsRadiusMi a downstream override (mutual aid, tone-set fallback,
// spoken-locality hint) has rewritten for the current request. A pin this
// far from the system's actual configured home is only legitimate when it
// lands inside a real, admin-configured escape hatch — a named mutual-aid
// destination or a resolved spoken-locality boundary centroid — never merely
// because some heuristic happened to widen or zero the per-request bounds.
// This is the backstop for call 266121-class bugs: a mutual-aid "unrecognized
// city" fallback zeroed BoundsRadiusMi entirely and let a same-named street
// 120 miles away pass every other check unchallenged.
func pinOutsideAbsoluteHomeCeiling(lat, lon float64, geo *GeoOptions) bool {
	if geo == nil || geo.HomeLat == 0 || geo.HomeMaxRadiusMi <= 0 {
		return false
	}
	if haversineMeters(geo.HomeLat, geo.HomeLon, lat, lon)/1609.34 <= geo.HomeMaxRadiusMi {
		return false
	}
	if pinInsideSpokenLocalityDisc(lat, lon, geo) {
		return false
	}
	for _, d := range geo.MutualAidDestinations {
		if d.Lat == 0 && d.Lon == 0 {
			continue
		}
		radius := d.RadiusMi
		if radius <= 0 {
			radius = 10
		}
		if haversineMeters(d.Lat, d.Lon, lat, lon)/1609.34 <= radius {
			return false
		}
	}
	return true
}

func pinOutsideServiceArea(lat, lon float64, geo *GeoOptions) bool {
	return pinOutsideCoverageBias(lat, lon, geo) || pinOutsideConfiguredState(lat, lon, geo) ||
		pinOutsideAbsoluteHomeCeiling(lat, lon, geo)
}

// PinOutsideCoverage reports whether coordinates fall outside the configured
// service-area center + radius + geoBoundsBufferMiles.
func PinOutsideCoverage(lat, lon float64, geo *GeoOptions) bool {
	return pinOutsideServiceArea(lat, lon, geo)
}

// ScopedGeocodeRadiusMi is the inner search disc for disambiguating
// county-wide imports within a tone-set jurisdiction (75% of radius, min 3mi).
func ScopedGeocodeRadiusMi(radiusMi float64) float64 {
	if radiusMi <= 0 {
		return 0
	}
	inner := radiusMi * 0.75
	if inner < 3 {
		inner = 3
	}
	if inner > radiusMi {
		inner = radiusMi
	}
	return inner
}

// PinOutsideScopedInner reports when a scoped geocode candidate landed on a
// distant homonym segment still inside the outer tone-set radius (e.g. South
// Avenue near Poland when Boardman dispatch meant Porter's Corners).
func PinOutsideScopedInner(lat, lon float64, geo *GeoOptions) bool {
	if geo == nil || geo.BoundsRadiusMi <= 0 || geo.BoundsLat == 0 {
		return false
	}
	return haversineMeters(geo.BoundsLat, geo.BoundsLon, lat, lon)/1609.34 > ScopedGeocodeRadiusMi(geo.BoundsRadiusMi)
}

// clearLocalPinOutsideCoverage drops coordinates that fall outside the
// configured service-area center + radius (+ geoBoundsBufferMiles).
func clearLocalPinOutsideCoverage(curated *CuratedAlert, geo *GeoOptions) {
	if curated == nil || geo == nil || curated.Lat == "" || curated.Lon == "" {
		return
	}
	var lat, lon float64
	fmt.Sscanf(curated.Lat, "%f", &lat)
	fmt.Sscanf(curated.Lon, "%f", &lon)
	if pinOutsideServiceArea(lat, lon, geo) {
		curated.Lat = ""
		curated.Lon = ""
	}
}

var numberedRouteRE = regexp.MustCompile(`(?i)\b(?:COUNTY ROAD|COUNTY RD|COUNTY ROUTE|CR|STATE ROUTE|STATE RD|ST RTE|SR|US RTE|US|RT|ROUTE|HWY|HIGHWAY)\s+\d`)

func addressUsesNumberedRoute(street string) bool {
	return numberedRouteRE.MatchString(strings.ToUpper(strings.TrimSpace(street)))
}
