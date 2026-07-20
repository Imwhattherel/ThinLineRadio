// Copyright (C) 2025 Thinline Dynamic Solutions
//
// geocode_external_stats.go — track outbound Census geocode requests.

package mapping

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// ExternalGeocodeResult classifies one outbound geocoder HTTP attempt.
type ExternalGeocodeResult struct {
	Provider       string // census
	Query          string
	MatchedAddress string
	Lat            float64
	Lon            float64
	OK             bool
	Detail         string // error, out_of_bounds, no_match, ...
}

type externalGeocodeCounters struct {
	censusSent       atomic.Uint64
	censusOK         atomic.Uint64
	cacheHit         atomic.Uint64
	missCacheHit     atomic.Uint64
	skippedExternal  atomic.Uint64
}

var extGeocodeStats externalGeocodeCounters

// RecordExternalGeocodeSend logs one outbound HTTP geocode attempt.
func RecordExternalGeocodeSend(res ExternalGeocodeResult) {
	p := strings.ToLower(strings.TrimSpace(res.Provider))
	switch p {
	case "census":
		extGeocodeStats.censusSent.Add(1)
		if res.OK {
			extGeocodeStats.censusOK.Add(1)
		}
	}
	line := fmt.Sprintf("%s provider=%s ok=%v query=%q matched=%q lat=%.6f lon=%.6f detail=%q",
		time.Now().Format(time.RFC3339), p, res.OK, truncateGeoLog(res.Query, 120),
		truncateGeoLog(res.MatchedAddress, 100), res.Lat, res.Lon, res.Detail)
	appendGeocodeExternalLog(line)
	if !res.OK {
		log.Printf("[GEOCODE] %s FAIL %s q=%q detail=%s", p, res.Detail, truncateGeoLog(res.Query, 80), res.Detail)
	}
}

func RecordGeocodeCacheHit() {
	extGeocodeStats.cacheHit.Add(1)
}

func RecordGeocodeMissCacheHit() {
	extGeocodeStats.missCacheHit.Add(1)
}

// RecordGeocodeSkippedExternal counts an external request avoided by the
// SkipExternalGeocode flag. Counted only — local variant probing skips dozens
// per call, and a log line each would flood the output. The total is visible
// in the external-geocode stats snapshot.
func RecordGeocodeSkippedExternal(reason string) {
	_ = reason
	extGeocodeStats.skippedExternal.Add(1)
}

func geocodeExternalLogPath() string {
	if p := strings.TrimSpace(os.Getenv("TLR_GEOCODE_LOG")); p != "" {
		return p
	}
	return "/tmp/tlr_geocode_external.log"
}

func appendGeocodeExternalLog(line string) {
	f, err := os.OpenFile(geocodeExternalLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, line)
}

func truncateGeoLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
