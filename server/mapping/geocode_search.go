// Copyright (C) 2025 Thinline Dynamic Solutions
//
// geocode_search.go — TLR client for nominatim-gateway GET /search.
// Used for place/city/township lookups (tone-set jurisdiction centers),
// not for dispatch street pins (those use POST /transcript).

package mapping

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var searchGeocodeClient = &http.Client{Timeout: 15 * time.Second}

// SearchGeocodeHit is one pin from gateway GET /search.
type SearchGeocodeHit struct {
	Lat         float64
	Lon         float64
	DisplayName string
	Class       string
	Type        string
	OK          bool
	Detail      string
}

type nominatimSearchResult struct {
	Lat         json.Number `json:"lat"`
	Lon         json.Number `json:"lon"`
	DisplayName string      `json:"display_name"`
	Class       string      `json:"class"`
	Type        string      `json:"type"`
}

// GeocodeNominatimSearch asks the gateway GET /search for a short place query.
func GeocodeNominatimSearch(gatewayURL, apiKey, query string, geo *GeoOptions) SearchGeocodeHit {
	gatewayURL = strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	query = strings.TrimSpace(query)
	if gatewayURL == "" || apiKey == "" || query == "" {
		return SearchGeocodeHit{Detail: "not_configured"}
	}
	if geo != nil && geo.SkipExternalGeocode {
		return SearchGeocodeHit{Detail: "skip_external"}
	}

	u, err := url.Parse(gatewayURL + "/search")
	if err != nil {
		return SearchGeocodeHit{Detail: "bad_url"}
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("limit", "5")
	if vb := geocodeViewboxParam(geo); vb != "" {
		q.Set("viewbox", vb)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return SearchGeocodeHit{Detail: "request_error"}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := searchGeocodeClient.Do(req)
	if err != nil {
		return SearchGeocodeHit{Detail: "transport_error"}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusPaymentRequired {
		return SearchGeocodeHit{Detail: "no_subscription"}
	}
	if resp.StatusCode != http.StatusOK {
		return SearchGeocodeHit{Detail: fmt.Sprintf("gateway_status_%d", resp.StatusCode)}
	}

	var results []nominatimSearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return SearchGeocodeHit{Detail: "decode_error"}
	}
	for _, r := range results {
		lat, lon := parseNominatimCoord(r.Lat), parseNominatimCoord(r.Lon)
		if lat == 0 && lon == 0 {
			continue
		}
		if PinOutsideCoverage(lat, lon, geo) {
			continue
		}
		return SearchGeocodeHit{
			Lat:         lat,
			Lon:         lon,
			DisplayName: r.DisplayName,
			Class:       r.Class,
			Type:        r.Type,
			OK:          true,
		}
	}
	if len(results) == 0 {
		return SearchGeocodeHit{Detail: "no_geocode_hit"}
	}
	return SearchGeocodeHit{Detail: "outside_coverage"}
}

func parseNominatimCoord(n json.Number) float64 {
	if n == "" {
		return 0
	}
	f, err := n.Float64()
	if err == nil {
		return f
	}
	f, err = strconv.ParseFloat(string(n), 64)
	if err != nil {
		return 0
	}
	return f
}
