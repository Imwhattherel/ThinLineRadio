// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Admin-triggered talkgroup location assignment: city/label + lat/lon for
// each talkgroup's incidentMappingConfig. Cleared rows return to inherit.

package main

import (
	"fmt"
	"strings"
)

// TalkgroupLocationRow is one talkgroup with optional geo for the admin dialog.
type TalkgroupLocationRow struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	TalkgroupLabel  string  `json:"talkgroupLabel"`
	Inherit         bool    `json:"inherit"`
	Enabled         bool    `json:"enabled"`
	GeoCity         string  `json:"geoCity"`
	GeoLat          float64 `json:"geoLat"`
	GeoLon          float64 `json:"geoLon"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	LocationContext string  `json:"locationContext"`
}

// TalkgroupLocationApply is one reviewed row submitted by the admin.
type TalkgroupLocationApply struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	GeoCity         string  `json:"geoCity"`
	GeoLat          float64 `json:"geoLat"`
	GeoLon          float64 `json:"geoLon"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	LocationContext string  `json:"locationContext"`
	Clear           bool    `json:"clear"`
}

// ListTalkgroupLocations returns every talkgroup on a system with its own
// incident-mapping geo (not the inherited/merged effective geo).
func (controller *Controller) ListTalkgroupLocations(systemId uint64) ([]TalkgroupLocationRow, error) {
	sys, ok := controller.Systems.GetSystemById(systemId)
	if !ok || sys == nil {
		return nil, fmt.Errorf("system %d not found", systemId)
	}
	out := []TalkgroupLocationRow{}
	for _, tg := range sys.Talkgroups.List {
		if tg == nil {
			continue
		}
		cfg := tg.IncidentMapping
		row := TalkgroupLocationRow{
			TalkgroupId:     tg.Id,
			TalkgroupLabel:  tg.Label,
			Inherit:         cfg.Inherit,
			Enabled:         cfg.Enabled,
			GeoCity:         cfg.GeoCity,
			GeoLat:          cfg.GeoLat,
			GeoLon:          cfg.GeoLon,
			GeoRadiusMiles:  cfg.GeoRadiusMiles,
			LocationContext: cfg.LocationContext,
		}
		if row.GeoRadiusMiles <= 0 && (row.GeoCity != "" || row.GeoLat != 0) {
			row.GeoRadiusMiles = 25
		}
		out = append(out, row)
	}
	return out, nil
}

// ApplyTalkgroupLocations persists talkgroup incident-mapping geo.
// Clear / empty geo returns the talkgroup to inherit=true.
func (controller *Controller) ApplyTalkgroupLocations(systemId uint64, rows []TalkgroupLocationApply) (applied, cleared int, err error) {
	sys, ok := controller.Systems.GetSystemById(systemId)
	if !ok || sys == nil {
		return 0, 0, fmt.Errorf("system %d not found", systemId)
	}

	for _, row := range rows {
		if row.TalkgroupId == 0 {
			continue
		}
		tg, ok := sys.Talkgroups.GetTalkgroupById(row.TalkgroupId)
		if !ok || tg == nil {
			continue
		}
		cfg := tg.IncidentMapping
		if row.Clear || (row.GeoLat == 0 && row.GeoLon == 0 && strings.TrimSpace(row.GeoCity) == "") {
			cfg.Inherit = true
			cfg.GeoCity = ""
			cfg.GeoLat = 0
			cfg.GeoLon = 0
			cfg.GeoRadiusMiles = 0
			cfg.LocationContext = ""
			cfg.GeoState = ""
			// Keep Enabled / ExtractAddressWithGemini as-is; inherit uses system.
			tg.IncidentMapping = cfg
			cleared++
		} else {
			radius := clampRadiusMiles(row.GeoRadiusMiles)
			if row.GeoRadiusMiles <= 0 {
				radius = 25
			}
			cfg.Inherit = false
			cfg.Enabled = true
			cfg.GeoCity = strings.TrimSpace(row.GeoCity)
			cfg.GeoLat = row.GeoLat
			cfg.GeoLon = row.GeoLon
			cfg.GeoRadiusMiles = radius
			if ctx := strings.TrimSpace(row.LocationContext); ctx != "" {
				cfg.LocationContext = ctx
			} else {
				cfg.LocationContext = cfg.GeoCity
			}
			cfg.GeoState = ""
			tg.IncidentMapping = cfg
			applied++
		}
		if _, e := controller.Database.Sql.Exec(
			`UPDATE "talkgroups" SET "incidentMappingConfig" = $1 WHERE "talkgroupId" = $2`,
			tg.IncidentMapping.JSON(), tg.Id); e != nil {
			return applied, cleared, e
		}
	}

	return applied, cleared, nil
}
