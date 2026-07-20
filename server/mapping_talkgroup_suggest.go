// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Admin talkgroup location suggestions: Gemini proposes city/label + radius
// from talkgroup names; TLR resolves lat/lon via boundaries then gateway search.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"rdio-scanner/server/mapping"
)

// TalkgroupLocationSuggest is one draft row returned to the admin dialog.
type TalkgroupLocationSuggest struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	TalkgroupLabel  string  `json:"talkgroupLabel"`
	GeoCity         string  `json:"geoCity"`
	GeoLat          float64 `json:"geoLat"`
	GeoLon          float64 `json:"geoLon"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	LocationContext string  `json:"locationContext"`
	Source          string  `json:"source"` // boundary | search | gemini_only | skipped
	Error           string  `json:"error,omitempty"`
}

type geminiTalkgroupSuggestItem struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	GeoCity         string  `json:"geoCity"`
	LocationContext string  `json:"locationContext"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	SearchQuery     string  `json:"searchQuery"`
}

// SuggestTalkgroupLocations fills draft geo for talkgroups on a system.
// When talkgroupIds is non-empty, only those IDs are considered (for client
// batching). onlyEmpty skips talkgroups that already have their own coords.
func (controller *Controller) SuggestTalkgroupLocations(systemId uint64, onlyEmpty bool, talkgroupIds []uint64) ([]TalkgroupLocationSuggest, error) {
	sys, ok := controller.Systems.GetSystemById(systemId)
	if !ok || sys == nil {
		return nil, fmt.Errorf("system %d not found", systemId)
	}

	apiKey := strings.TrimSpace(controller.Options.TranscriptionConfig.GeminiAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(controller.Options.TranscriptionConfig.GoogleAPIKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Gemini API key not configured (Admin → Transcription → Gemini API key)")
	}
	model := strings.TrimSpace(controller.Options.TranscriptionConfig.GeminiModel)
	if model == "" {
		model = defaultGeminiModel
	}

	rows, err := controller.ListTalkgroupLocations(systemId)
	if err != nil {
		return nil, err
	}
	if len(talkgroupIds) > 0 {
		want := map[uint64]bool{}
		for _, id := range talkgroupIds {
			if id != 0 {
				want[id] = true
			}
		}
		filtered := make([]TalkgroupLocationRow, 0, len(want))
		for _, row := range rows {
			if want[row.TalkgroupId] {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	bias := buildToneSetSuggestBias(sys)

	var need []TalkgroupLocationRow
	var out []TalkgroupLocationSuggest
	for _, row := range rows {
		hasCoords := row.GeoLat != 0 || row.GeoLon != 0
		if onlyEmpty && hasCoords && !row.Inherit {
			out = append(out, TalkgroupLocationSuggest{
				TalkgroupId:     row.TalkgroupId,
				TalkgroupLabel:  row.TalkgroupLabel,
				GeoCity:         row.GeoCity,
				GeoLat:          row.GeoLat,
				GeoLon:          row.GeoLon,
				GeoRadiusMiles:  clampRadiusMiles(row.GeoRadiusMiles),
				LocationContext: row.LocationContext,
				Source:          "skipped",
			})
			continue
		}
		need = append(need, row)
	}
	if len(need) == 0 {
		return out, nil
	}

	suggestions, err := geminiSuggestTalkgroupPlaces(apiKey, model, bias, need)
	if err != nil {
		return nil, err
	}
	byID := map[uint64]geminiTalkgroupSuggestItem{}
	for _, s := range suggestions {
		if s.TalkgroupId == 0 {
			continue
		}
		byID[s.TalkgroupId] = s
	}

	store := NewMappingStore(controller.Database)
	gatewayURL := ""
	gatewayKey := strings.TrimSpace(controller.Options.RelayServerAPIKey)
	if controller.NominatimAccessAllowed() {
		gatewayURL = controller.NominatimGatewayURLSnapshot()
	}
	geoOpts := biasToGeoOptions(bias)

	type pendingTG struct {
		idx     int
		searchQ string
	}
	draft := make([]TalkgroupLocationSuggest, 0, len(need))
	var pending []pendingTG

	for _, row := range need {
		sug, ok := byID[row.TalkgroupId]
		item := TalkgroupLocationSuggest{
			TalkgroupId:    row.TalkgroupId,
			TalkgroupLabel: row.TalkgroupLabel,
		}
		if !ok {
			sug = heuristicTalkgroupSuggest(row, bias)
		}
		item.GeoCity = strings.TrimSpace(sug.GeoCity)
		item.LocationContext = strings.TrimSpace(sug.LocationContext)
		if item.LocationContext == "" {
			item.LocationContext = item.GeoCity
		}
		radius := sug.GeoRadiusMiles
		if radius <= 0 {
			radius = 25
		}
		radius = clampRadiusMiles(radius)

		searchQ := strings.TrimSpace(sug.SearchQuery)
		if searchQ == "" {
			searchQ = item.GeoCity
		}
		locality := localityNameForBoundary(searchQ, item.GeoCity, row.TalkgroupLabel)

		if locality != "" && bias.HomeLat != 0 && bias.HomeLon != 0 {
			if lat, lon, br, ok := store.BoundaryCentroidForLocality(locality, bias.HomeLat, bias.HomeLon); ok {
				item.GeoLat = lat
				item.GeoLon = lon
				item.Source = "boundary"
				if br > radius {
					radius = clampRadiusMiles(br)
				}
			}
		}
		item.GeoRadiusMiles = radius
		idx := len(draft)
		draft = append(draft, item)
		if item.Source != "boundary" && searchQ != "" {
			pending = append(pending, pendingTG{idx: idx, searchQ: searchQ})
		}
	}

	if len(pending) > 0 && gatewayURL != "" && gatewayKey != "" {
		queries := make([]mapping.BatchSearchQuery, 0, len(pending))
		for _, p := range pending {
			queries = append(queries, mapping.BatchSearchQuery{
				ID:    fmt.Sprintf("%d", draft[p.idx].TalkgroupId),
				Query: p.searchQ,
			})
		}
		hits := mapping.GeocodeNominatimSearchBatch(gatewayURL, gatewayKey, queries, geoOpts)
		for _, p := range pending {
			id := fmt.Sprintf("%d", draft[p.idx].TalkgroupId)
			hit := hits[id]
			if hit.OK {
				draft[p.idx].GeoLat = hit.Lat
				draft[p.idx].GeoLon = hit.Lon
				draft[p.idx].Source = "search"
				draft[p.idx].Error = ""
			} else if draft[p.idx].Error == "" {
				draft[p.idx].Error = hit.Detail
			}
		}
	}

	for i := range draft {
		if draft[i].Source == "boundary" || draft[i].Source == "search" {
			continue
		}
		draft[i].Source = "gemini_only"
		if draft[i].Error == "" && draft[i].GeoCity == "" {
			draft[i].Error = "no_suggestion"
		} else if draft[i].Error == "" {
			draft[i].Error = "geocode_failed"
		}
	}
	out = append(out, draft...)
	return out, nil
}

func heuristicTalkgroupSuggest(row TalkgroupLocationRow, bias toneSetSuggestBias) geminiTalkgroupSuggestItem {
	name := strings.TrimSpace(row.TalkgroupLabel)
	locality := localityNameForBoundary(name, "", name)
	city := locality
	if st := mapping.StateAbbrev(bias.State); st != "" && locality != "" {
		city = locality + ", " + st
	} else if bias.State != "" && locality != "" {
		city = locality + ", " + bias.State
	}
	ctx := bias.CountyState
	if ctx == "" {
		ctx = city
	}
	q := city
	if bias.CountyState != "" && !strings.Contains(strings.ToLower(q), strings.ToLower(bias.CountyState)) {
		q = city + ", " + bias.CountyState
	}
	return geminiTalkgroupSuggestItem{
		TalkgroupId:     row.TalkgroupId,
		GeoCity:         city,
		LocationContext: ctx,
		GeoRadiusMiles:  25,
		SearchQuery:     q,
	}
}

func geminiSuggestTalkgroupPlaces(apiKey, model string, bias toneSetSuggestBias, rows []TalkgroupLocationRow) ([]geminiTalkgroupSuggestItem, error) {
	type inRow struct {
		TalkgroupId    uint64 `json:"talkgroupId"`
		TalkgroupLabel string `json:"talkgroupLabel"`
	}
	inputs := make([]inRow, 0, len(rows))
	for _, r := range rows {
		inputs = append(inputs, inRow{
			TalkgroupId:    r.TalkgroupId,
			TalkgroupLabel: r.TalkgroupLabel,
		})
	}
	payloadRows, _ := json.Marshal(inputs)

	regionBits := []string{}
	if bias.CountyState != "" {
		regionBits = append(regionBits, bias.CountyState)
	}
	if bias.CityHint != "" {
		regionBits = append(regionBits, bias.CityHint)
	}
	if bias.State != "" {
		regionBits = append(regionBits, bias.State)
	}
	region := strings.Join(regionBits, "; ")
	if region == "" {
		region = "United States (infer state from talkgroup names when possible)"
	}

	prompt := fmt.Sprintf(`You help configure radio scanner talkgroup jurisdictions for an incident map.

System: %q
Region bias: %s

For each talkgroup below, propose a place label and a Nominatim-style search query for the community / city / township the channel covers. Do NOT invent latitude or longitude.

Rules:
- geoCity: short label like "Warren, OH" or "Niles, OH"
- locationContext: county/region context when known (e.g. "Trumbull County, Ohio")
- searchQuery: best forward-geocode query including state (and county when known)
- geoRadiusMiles: typical coverage 10–30 for city/dispatch channels, 15–40 for county-wide; default 25; max 40
- Strip channel codes / radio jargon; use the place name implied by the talkgroup label
- Keep talkgroupId exactly as given
- Return one object per input talkgroup
- If the label is clearly county-wide dispatch with no city, use the county seat or primary city from the region bias

Talkgroups JSON:
%s`, bias.SystemLabel, region, string(payloadRows))

	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.1,
			"responseMimeType": "application/json",
			"responseSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"suggestions": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"talkgroupId":     map[string]any{"type": "integer"},
								"geoCity":         map[string]any{"type": "string"},
								"locationContext": map[string]any{"type": "string"},
								"geoRadiusMiles":  map[string]any{"type": "number"},
								"searchQuery":     map[string]any{"type": "string"},
							},
							"required": []string{"talkgroupId", "geoCity", "searchQuery"},
						},
					},
				},
				"required": []string{"suggestions"},
			},
		},
	}
	rawPayload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: encode request: %w", err)
	}

	url := fmt.Sprintf(geminiGenerateContentURL, model) + "?key=" + apiKey
	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(rawPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return nil, fmt.Errorf("gemini: status %d: %s", resp.StatusCode, msg)
	}

	var apiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	if apiResp.Error != nil && apiResp.Error.Message != "" {
		return nil, fmt.Errorf("gemini: %s", apiResp.Error.Message)
	}
	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini: empty response")
	}
	text := strings.TrimSpace(apiResp.Candidates[0].Content.Parts[0].Text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```JSON")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}
	var parsed struct {
		Suggestions []geminiTalkgroupSuggestItem `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("gemini: parse suggestions: %w", err)
	}
	return parsed.Suggestions, nil
}
