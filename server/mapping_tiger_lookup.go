// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Street pins are geocoded by a single POST /transcript to nominatim-gateway
// (see incident_mapping.go). The old multi-query /search fan-out that lived
// here was removed.

package main

import "strings"

// geocodeMissReasonIsBoundsDependent reports miss reasons that depend on the
// request-scoped tone-set / coverage disc. Caching those as permanent misses
// blocks later retries that widen BoundsRadiusMi for the same query line.
func geocodeMissReasonIsBoundsDependent(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "out_of_bounds", "outside_coverage", "far_from_dispatch_cross":
		return true
	default:
		return false
	}
}
