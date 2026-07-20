// Copyright (C) 2025 Thinline Dynamic Solutions

package mapping

import "sync"

// CuratedAlert holds the structured fields extracted from a raw transcript.
type CuratedAlert struct {
	UnitLocation        string // e.g. "STA40"
	CommonName          string // e.g. "HOWMET AEROSPACE" — empty if none
	NatureDesc          string // e.g. "STRUCTURE FIRE"
	Address             string // e.g. "2518 POPLAR ST" (street only — no apt)
	AptUnit             string // e.g. "APT 2" or "UNIT 2B" — kept separate so geocoding works on the base address
	CrossStreet1        string // e.g. "OAKVIEW DR"
	CrossStreet2        string // e.g. "SALT SPRINGS YOUNGSTOWN RD"
	Notes               string // clean narrative only
	Lat                 string // e.g. "41.185307"
	Lon                 string // e.g. "-80.780949"
	AudioLink           string // playback URL — appended AFTER lat/lon so it doesn't break field parsing
	CorrectedTranscript string // STT transcript with garbled words/streets corrected by OpenAI
}

// GeoOptions controls how the outbound geocode chain (Thinline Geocoding API)
// biases its results.
type GeoOptions struct {
	LocationContext string  // e.g. "Trumbull County, Ohio" — appended to query (fallback)
	State           string  // canonical US state (e.g. "Ohio") — filters by administrative area
	CityHint        string  // tone set GeoCity — appended to query alongside bounds
	BoundsLat       float64 // center latitude for bounds-based bias
	BoundsLon       float64 // center longitude for bounds-based bias
	BoundsRadiusMi  float64 // radius in miles (0 = disabled)
	// ExtraKnownStreets: destination department streets merged into forward-
	// geocode scoring (mutual-aid override).
	ExtraKnownStreets []string
	// NominatimDirectURL/APIKey wire up the self-hosted Nominatim add-on,
	// reached DIRECTLY by this TLR server now (via the nominatim-gateway
	// service, see server/mapping/geocode_relay_nominatim.go) rather than
	// proxied through the relay server. Populated from this TLR instance's
	// relay settings, not per-system config — every system shares the one
	// relay subscription. Empty NominatimDirectURL disables the lookup
	// entirely (no subscription configured, or relay hasn't reported a
	// gateway_url yet).
	NominatimDirectURL string `json:"-"`
	NominatimAPIKey    string `json:"-"`
	// DispatchSpokenLocality is a city/community named in the dispatch transcript
	// (not the tone-set GeoCity label). Drives homonym bias and centroid fallbacks.
	DispatchSpokenLocality string `json:"-"`
	// DispatchTranscript is the full cleaned dispatch text for this call.
	// When set, nominatim-gateway POST /transcript receives it (or a Gemini
	// short address when extract-address is enabled).
	DispatchTranscript string `json:"-"`
	// SkipExternalGeocode disables live outbound geocode HTTP (local + DB
	// cache only) — including the Thinline Geocoding API relay call, and the
	// narrow Census place-center check in dispatch_locality.go.
	SkipExternalGeocode bool `json:"-"`
	// SpokenLocality{Lat,Lon,RadiusMi} define a secondary allowed disc around
	// the community the dispatcher named (resolved from imported boundary
	// centroids). Mutual-aid dispatches legitimately land outside the home
	// coverage radius; pins inside this disc are accepted.
	SpokenLocalityLat      float64 `json:"-"`
	SpokenLocalityLon      float64 `json:"-"`
	SpokenLocalityRadiusMi float64 `json:"-"`
	// MutualAidDestinations lists peer jurisdiction geocode centers (from
	// talkgroup incident-mapping configs) used when mutual aid names a city.
	MutualAidDestinations []MutualAidDestination `json:"-"`
	// HomeLat/HomeLon/HomeMaxRadiusMi anchor an unconditional outer coverage
	// ceiling derived once from the system's own configured home center/
	// radius, set a single time per call and never touched by any downstream
	// override (mutual aid, tone-set fallback, spoken-locality hints all
	// rewrite BoundsLat/BoundsRadiusMi for their own request-scoped purposes).
	// PinOutsideCoverage enforces this in addition to — not instead of — the
	// normal Bounds check, so a bug in any one of those heuristics can never
	// again silently accept a pin hundreds of miles from the system's real
	// coverage area just because it happened to zero or widen BoundsRadiusMi.
	// Legitimate mutual aid to a real, admin-configured peer jurisdiction is
	// still allowed even when farther than this from home — it is checked
	// against MutualAidDestinations/SpokenLocality separately, not against
	// this ceiling.
	HomeLat         float64 `json:"-"`
	HomeLon         float64 `json:"-"`
	HomeMaxRadiusMi float64 `json:"-"`
}

// MutualAidDestination is a geocode bias center for a named jurisdiction.
type MutualAidDestination struct {
	CityLabel  string
	Lat        float64
	Lon        float64
	RadiusMi   float64
}

// KnownPlace is a named facility with CAD-aligned coordinates scoped to a street group.
type KnownPlace struct {
	GroupID     int64   `json:"groupId"`
	PlaceKey    string  `json:"placeKey"`
	DisplayName string  `json:"displayName"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	AddressHint string  `json:"addressHint"`
	Source      string  `json:"source"`
	UpdatedAt   int64   `json:"updatedAt"`
}

// StreetCorrection maps a bad STT output to the correct street name.
type StreetCorrection struct {
	ID          int64  `json:"id"`
	BadName     string `json:"badName"`
	CorrectName string `json:"correctName"`
}

// ScopeData holds per–street-group streets, corrections, and known places used
// by extraction and geocoding. Loaded by the server and passed into mapping.
type ScopeData struct {
	KnownStreets []string
	Corrections  []StreetCorrection
	KnownPlaces  []KnownPlace

	phoneticOnce sync.Once
	phoneticIdx  *streetPhoneticIndex
}
