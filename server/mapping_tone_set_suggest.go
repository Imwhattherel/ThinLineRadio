// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Admin tone-set location suggestions: Gemini proposes city/label + radius
// from tone-set names; TLR resolves lat/lon via imported boundary centroids
// then nominatim-gateway GET /search. Nothing is persisted until Apply.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"rdio-scanner/server/mapping"
)

// ToneSetLocationSuggest is one draft row returned to the admin dialog.
type ToneSetLocationSuggest struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	ToneSetId       string  `json:"toneSetId"`
	Label           string  `json:"label"`
	GeoCity         string  `json:"geoCity"`
	GeoLat          float64 `json:"geoLat"`
	GeoLon          float64 `json:"geoLon"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	LocationContext string  `json:"locationContext"`
	Source          string  `json:"source"` // boundary | search | gemini_only | skipped
	Error           string  `json:"error,omitempty"`
}

type toneSetSuggestBias struct {
	SystemLabel string
	CityHint    string
	CountyState string
	State       string
	HomeLat     float64
	HomeLon     float64
	HomeRadius  float64
}

type geminiToneSuggestItem struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	ToneSetId       string  `json:"toneSetId"`
	GeoCity         string  `json:"geoCity"`
	LocationContext string  `json:"locationContext"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	SearchQuery     string  `json:"searchQuery"`
}

var localitySuffixRE = regexp.MustCompile(`(?i)\b(fire|rescue|ems|hose|township|twp|village|city|fd|pd|aux|auxiliary|dept|department|company|co)\b`)

// SuggestToneSetLocations fills draft geo for empty tone sets on a system.
func (controller *Controller) SuggestToneSetLocations(systemId uint64, onlyEmpty bool) ([]ToneSetLocationSuggest, error) {
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

	rows, err := controller.ListToneSetLocations(systemId)
	if err != nil {
		return nil, err
	}
	bias := buildToneSetSuggestBias(sys)

	var need []ToneSetLocationRow
	var out []ToneSetLocationSuggest
	for _, row := range rows {
		if onlyEmpty && (row.GeoLat != 0 || row.GeoLon != 0) {
			out = append(out, ToneSetLocationSuggest{
				TalkgroupId:     row.TalkgroupId,
				ToneSetId:       row.ToneSetId,
				Label:           row.Label,
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

	suggestions, err := geminiSuggestToneSetPlaces(apiKey, model, bias, need)
	if err != nil {
		return nil, err
	}
	byKey := map[string]geminiToneSuggestItem{}
	for _, s := range suggestions {
		if s.ToneSetId == "" || s.TalkgroupId == 0 {
			continue
		}
		byKey[fmt.Sprintf("%d|%s", s.TalkgroupId, s.ToneSetId)] = s
	}

	store := NewMappingStore(controller.Database)
	gatewayURL := ""
	gatewayKey := strings.TrimSpace(controller.Options.RelayServerAPIKey)
	if controller.NominatimAccessAllowed() {
		gatewayURL = controller.NominatimGatewayURLSnapshot()
	}
	geoOpts := biasToGeoOptions(bias)

	type pendingTS struct {
		idx     int
		key     string
		searchQ string
	}
	draft := make([]ToneSetLocationSuggest, 0, len(need))
	var pending []pendingTS

	for _, row := range need {
		key := fmt.Sprintf("%d|%s", row.TalkgroupId, row.ToneSetId)
		sug, ok := byKey[key]
		item := ToneSetLocationSuggest{
			TalkgroupId: row.TalkgroupId,
			ToneSetId:   row.ToneSetId,
			Label:       row.Label,
		}
		if !ok {
			sug = heuristicToneSuggest(row, bias)
		}
		item.GeoCity = strings.TrimSpace(sug.GeoCity)
		item.LocationContext = strings.TrimSpace(sug.LocationContext)
		if item.LocationContext == "" {
			item.LocationContext = item.GeoCity
		}
		radius := clampRadiusMiles(sug.GeoRadiusMiles)
		if radius <= 0 {
			radius = 5
		}

		searchQ := strings.TrimSpace(sug.SearchQuery)
		if searchQ == "" {
			searchQ = item.GeoCity
		}
		locality := localityNameForBoundary(searchQ, item.GeoCity, row.Label)

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
			pending = append(pending, pendingTS{idx: idx, key: key, searchQ: searchQ})
		}
	}

	if len(pending) > 0 && gatewayURL != "" && gatewayKey != "" {
		queries := make([]mapping.BatchSearchQuery, 0, len(pending))
		for _, p := range pending {
			queries = append(queries, mapping.BatchSearchQuery{ID: p.key, Query: p.searchQ})
		}
		hits := mapping.GeocodeNominatimSearchBatch(gatewayURL, gatewayKey, queries, geoOpts)
		for _, p := range pending {
			hit := hits[p.key]
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

func buildToneSetSuggestBias(sys *System) toneSetSuggestBias {
	bias := toneSetSuggestBias{SystemLabel: sys.Label}
	cfg := sys.IncidentMapping
	if cfg.GeoLat != 0 && cfg.GeoLon != 0 {
		bias.HomeLat = cfg.GeoLat
		bias.HomeLon = cfg.GeoLon
		bias.HomeRadius = cfg.GeoRadiusMiles
		bias.CityHint = strings.TrimSpace(cfg.GeoCity)
		bias.CountyState = strings.TrimSpace(cfg.LocationContext)
		bias.State = strings.TrimSpace(cfg.GeoState)
	}
	for _, tg := range sys.Talkgroups.List {
		if tg == nil {
			continue
		}
		merged := resolveIncidentMappingConfig(sys, tg)
		if merged.GeoLat == 0 || merged.GeoLon == 0 {
			continue
		}
		if bias.HomeLat == 0 {
			bias.HomeLat = merged.GeoLat
			bias.HomeLon = merged.GeoLon
			bias.HomeRadius = merged.GeoRadiusMiles
			bias.CityHint = strings.TrimSpace(merged.GeoCity)
			bias.CountyState = strings.TrimSpace(merged.LocationContext)
			bias.State = strings.TrimSpace(merged.GeoState)
		}
		// Prefer the largest coverage disc as county-ish search viewbox.
		if merged.GeoRadiusMiles > bias.HomeRadius {
			bias.HomeRadius = merged.GeoRadiusMiles
			if bias.HomeLat == 0 {
				bias.HomeLat = merged.GeoLat
				bias.HomeLon = merged.GeoLon
			}
		}
		if bias.CountyState == "" {
			bias.CountyState = strings.TrimSpace(merged.LocationContext)
		}
		if bias.State == "" {
			bias.State = strings.TrimSpace(merged.GeoState)
		}
		if bias.CityHint == "" {
			bias.CityHint = strings.TrimSpace(merged.GeoCity)
		}
	}
	if bias.HomeRadius <= 0 {
		bias.HomeRadius = 30
	}
	if bias.State == "" {
		bias.State = mapping.DeriveState(bias.CountyState, "", bias.CityHint)
	}
	return bias
}

func biasToGeoOptions(bias toneSetSuggestBias) *mapping.GeoOptions {
	geo := &mapping.GeoOptions{
		BoundsLat:       bias.HomeLat,
		BoundsLon:       bias.HomeLon,
		BoundsRadiusMi:  bias.HomeRadius,
		HomeLat:         bias.HomeLat,
		HomeLon:         bias.HomeLon,
		HomeMaxRadiusMi: bias.HomeRadius + 10,
		CityHint:        bias.CityHint,
		LocationContext: bias.CountyState,
		State:           bias.State,
	}
	return geo
}

func heuristicToneSuggest(row ToneSetLocationRow, bias toneSetSuggestBias) geminiToneSuggestItem {
	name := strings.TrimSpace(row.Label)
	if name == "" {
		name = row.ToneSetId
	}
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
	return geminiToneSuggestItem{
		TalkgroupId:     row.TalkgroupId,
		ToneSetId:       row.ToneSetId,
		GeoCity:         city,
		LocationContext: ctx,
		GeoRadiusMiles:  5,
		SearchQuery:     q,
	}
}

func localityNameForBoundary(searchQuery, geoCity, label string) string {
	candidates := []string{searchQuery, geoCity, label}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if i := strings.Index(c, ","); i >= 0 {
			c = strings.TrimSpace(c[:i])
		}
		c = localitySuffixRE.ReplaceAllString(c, " ")
		c = strings.Join(strings.Fields(c), " ")
		if c != "" {
			return c
		}
	}
	return ""
}

func geminiSuggestToneSetPlaces(apiKey, model string, bias toneSetSuggestBias, rows []ToneSetLocationRow) ([]geminiToneSuggestItem, error) {
	type inRow struct {
		TalkgroupId    uint64 `json:"talkgroupId"`
		TalkgroupLabel string `json:"talkgroupLabel"`
		ToneSetId      string `json:"toneSetId"`
		Label          string `json:"label"`
	}
	inputs := make([]inRow, 0, len(rows))
	for _, r := range rows {
		inputs = append(inputs, inRow{
			TalkgroupId:    r.TalkgroupId,
			TalkgroupLabel: r.TalkgroupLabel,
			ToneSetId:      r.ToneSetId,
			Label:          r.Label,
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
		region = "United States (infer state from tone-set / talkgroup names when possible)"
	}

	prompt := fmt.Sprintf(`You help configure radio scanner tone-set jurisdictions for an incident map.

System: %q
Region bias: %s

For each tone set below, propose a place label and a Nominatim-style search query for the community / township / city the agency covers. Do NOT invent latitude or longitude.

Rules:
- geoCity: short label like "Jefferson, OH" or "Austinburg Township, OH"
- locationContext: county/region context when known (e.g. "Ashtabula County, Ohio")
- searchQuery: best forward-geocode query including state (and county when known)
- geoRadiusMiles: typical coverage radius 3–15 for township/village fire, 5–12 for city fire/rescue, 5 default; max 40
- Strip agency words (Fire, Rescue, Hose, AUX) from the place name when forming geoCity
- Keep talkgroupId and toneSetId exactly as given
- Return one object per input tone set

Tone sets JSON:
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
								"toneSetId":       map[string]any{"type": "string"},
								"geoCity":         map[string]any{"type": "string"},
								"locationContext": map[string]any{"type": "string"},
								"geoRadiusMiles":  map[string]any{"type": "number"},
								"searchQuery":     map[string]any{"type": "string"},
							},
							"required": []string{"talkgroupId", "toneSetId", "geoCity", "searchQuery"},
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
		Suggestions []geminiToneSuggestItem `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("gemini: parse suggestions: %w", err)
	}
	return parsed.Suggestions, nil
}
