// Copyright (C) 2025 Thinline Dynamic Solutions
//
// census_geocode.go — free US Census geocoder (TIGER-backed, no API key).

package mapping

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const censusGeocoderURL = "https://geocoding.geo.census.gov/geocoder/locations/onelineaddress"

var censusClient = &http.Client{Timeout: 12 * time.Second}

type censusGeocodeResult struct {
	Lat               float64
	Lon               float64
	MatchedAddress    string
	StreetNorm        string
}

// GeocodeCensusOneLine calls geocoding.geo.census.gov for a single address line.
func GeocodeCensusOneLine(query string) (*censusGeocodeResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	params := url.Values{}
	params.Set("address", query)
	params.Set("benchmark", "Public_AR_Current")
	params.Set("format", "json")
	resp, err := censusClient.Get(censusGeocoderURL + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Result struct {
			AddressMatches []struct {
				MatchedAddress string `json:"matchedAddress"`
				Coordinates    struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				} `json:"coordinates"`
			} `json:"addressMatches"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Result.AddressMatches) == 0 {
		return nil, fmt.Errorf("no match")
	}
	m := payload.Result.AddressMatches[0]
	if m.Coordinates.Y == 0 && m.Coordinates.X == 0 {
		return nil, fmt.Errorf("empty coordinates")
	}
	if !censusMatchLooksValid(m.MatchedAddress) {
		return nil, fmt.Errorf("vague match %q", m.MatchedAddress)
	}
	return &censusGeocodeResult{
		Lat:            m.Coordinates.Y,
		Lon:            m.Coordinates.X,
		MatchedAddress: m.MatchedAddress,
	}, nil
}

// GeocodeCensusOneLineTracked geocodes one line and records stats. Returns detail on failure.
func GeocodeCensusOneLineTracked(query string, geo *GeoOptions) (lat, lon float64, matched string, ok bool, detail string) {
	res, err := GeocodeCensusOneLine(query)
	if err != nil || res == nil {
		detail = "no_match"
		if err != nil {
			detail = err.Error()
		}
		RecordExternalGeocodeSend(ExternalGeocodeResult{Provider: "census", Query: query, Detail: detail})
		return 0, 0, "", false, detail
	}
	if !geocodeFormattedStreetPlausible(query, res.MatchedAddress) {
		detail = "implausible_match"
		RecordExternalGeocodeSend(ExternalGeocodeResult{Provider: "census", Query: query, MatchedAddress: res.MatchedAddress, Lat: res.Lat, Lon: res.Lon, Detail: detail})
		return 0, 0, "", false, detail
	}
	if PinOutsideCoverage(res.Lat, res.Lon, geo) {
		detail = "out_of_bounds"
		RecordExternalGeocodeSend(ExternalGeocodeResult{Provider: "census", Query: query, MatchedAddress: res.MatchedAddress, Lat: res.Lat, Lon: res.Lon, Detail: detail})
		return 0, 0, "", false, detail
	}
	RecordExternalGeocodeSend(ExternalGeocodeResult{Provider: "census", Query: query, MatchedAddress: res.MatchedAddress, Lat: res.Lat, Lon: res.Lon, OK: true})
	return res.Lat, res.Lon, res.MatchedAddress, true, ""
}

func censusMatchLooksValid(matched string) bool {
	u := strings.ToUpper(strings.TrimSpace(matched))
	if u == "" {
		return false
	}
	// Reject county/ state centroids with no street route.
	if !strings.Contains(u, " ST") && !strings.Contains(u, " RD") && !strings.Contains(u, " AVE") &&
		!strings.Contains(u, " DR") && !strings.Contains(u, " LN") && !strings.Contains(u, " RTE") &&
		!strings.Contains(u, " SR ") && !strings.Contains(u, " HWY") && !strings.Contains(u, " TRL") &&
		!strings.Contains(u, " CT") && !strings.Contains(u, "&") {
		if strings.Contains(u, "COUNTY") || strings.Count(u, ",") <= 1 {
			return false
		}
	}
	return true
}
