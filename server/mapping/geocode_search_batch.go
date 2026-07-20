// Copyright (C) 2025 Thinline Dynamic Solutions
//
// geocode_search_batch.go — TLR client for nominatim-gateway POST /search/batch.
// One HTTP round-trip for many place/city lookups (talkgroup / tone-set Suggest).

package mapping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var searchBatchGeocodeClient = &http.Client{Timeout: 120 * time.Second}

// BatchSearchQuery is one place lookup keyed for result matching.
type BatchSearchQuery struct {
	ID    string
	Query string
}

type batchSearchRequestBody struct {
	Queries []batchSearchQueryBody `json:"queries"`
	Viewbox string                 `json:"viewbox,omitempty"`
	Limit   string                 `json:"limit,omitempty"`
}

type batchSearchQueryBody struct {
	ID string `json:"id"`
	Q  string `json:"q"`
}

type batchSearchResponseBody struct {
	Results []batchSearchResultBody `json:"results"`
}

type batchSearchResultBody struct {
	ID          string  `json:"id"`
	OK          bool    `json:"ok"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DisplayName string  `json:"display_name"`
	Detail      string  `json:"detail"`
}

// GeocodeNominatimSearchBatch asks the gateway to resolve many place queries
// in one request. Returns a map keyed by query ID. Missing/failed IDs are
// omitted or present with OK=false Detail set.
func GeocodeNominatimSearchBatch(gatewayURL, apiKey string, queries []BatchSearchQuery, geo *GeoOptions) map[string]SearchGeocodeHit {
	out := map[string]SearchGeocodeHit{}
	gatewayURL = strings.TrimRight(strings.TrimSpace(gatewayURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if gatewayURL == "" || apiKey == "" || len(queries) == 0 {
		for _, q := range queries {
			out[q.ID] = SearchGeocodeHit{Detail: "not_configured"}
		}
		return out
	}
	if geo != nil && geo.SkipExternalGeocode {
		for _, q := range queries {
			out[q.ID] = SearchGeocodeHit{Detail: "skip_external"}
		}
		return out
	}

	reqBody := batchSearchRequestBody{
		Limit:   "3",
		Viewbox: geocodeViewboxParam(geo),
	}
	for _, q := range queries {
		id := strings.TrimSpace(q.ID)
		qq := strings.TrimSpace(q.Query)
		if id == "" || qq == "" {
			continue
		}
		reqBody.Queries = append(reqBody.Queries, batchSearchQueryBody{ID: id, Q: qq})
	}
	if len(reqBody.Queries) == 0 {
		return out
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		for _, q := range reqBody.Queries {
			out[q.ID] = SearchGeocodeHit{Detail: "encode_error"}
		}
		return out
	}

	req, err := http.NewRequest(http.MethodPost, gatewayURL+"/search/batch", bytes.NewReader(payload))
	if err != nil {
		for _, q := range reqBody.Queries {
			out[q.ID] = SearchGeocodeHit{Detail: "request_error"}
		}
		return out
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := searchBatchGeocodeClient.Do(req)
	if err != nil {
		for _, q := range reqBody.Queries {
			out[q.ID] = SearchGeocodeHit{Detail: "transport_error"}
		}
		return out
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusPaymentRequired {
		for _, q := range reqBody.Queries {
			out[q.ID] = SearchGeocodeHit{Detail: "no_subscription"}
		}
		return out
	}
	if resp.StatusCode == http.StatusNotFound {
		// Older gateway without /search/batch — fall back to sequential GET /search.
		return geocodeNominatimSearchBatchFallback(gatewayURL, apiKey, queries, geo)
	}
	if resp.StatusCode != http.StatusOK {
		detail := fmt.Sprintf("gateway_status_%d", resp.StatusCode)
		for _, q := range reqBody.Queries {
			out[q.ID] = SearchGeocodeHit{Detail: detail}
		}
		return out
	}

	var parsed batchSearchResponseBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		for _, q := range reqBody.Queries {
			out[q.ID] = SearchGeocodeHit{Detail: "decode_error"}
		}
		return out
	}
	for _, r := range parsed.Results {
		if !r.OK || (r.Lat == 0 && r.Lon == 0) {
			detail := strings.TrimSpace(r.Detail)
			if detail == "" {
				detail = "no_geocode_hit"
			}
			out[r.ID] = SearchGeocodeHit{Detail: detail}
			continue
		}
		if PinOutsideCoverage(r.Lat, r.Lon, geo) {
			out[r.ID] = SearchGeocodeHit{Detail: "outside_coverage"}
			continue
		}
		out[r.ID] = SearchGeocodeHit{
			Lat:         r.Lat,
			Lon:         r.Lon,
			DisplayName: r.DisplayName,
			OK:          true,
		}
	}
	return out
}

func geocodeNominatimSearchBatchFallback(gatewayURL, apiKey string, queries []BatchSearchQuery, geo *GeoOptions) map[string]SearchGeocodeHit {
	out := map[string]SearchGeocodeHit{}
	for _, q := range queries {
		id := strings.TrimSpace(q.ID)
		if id == "" {
			continue
		}
		out[id] = GeocodeNominatimSearch(gatewayURL, apiKey, q.Query, geo)
	}
	return out
}
