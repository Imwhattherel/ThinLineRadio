// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import "math"

// milesBetween is an equirectangular distance approximation (good at city/county
// scale) used to rank/filter locations by proximity to a coverage center.
func milesBetween(lat1, lon1, lat2, lon2 float64) float64 {
	dlat := (lat1 - lat2) * 69.0
	dlon := (lon1 - lon2) * 69.0 * math.Cos(lat1*math.Pi/180)
	return math.Sqrt(dlat*dlat + dlon*dlon)
}
