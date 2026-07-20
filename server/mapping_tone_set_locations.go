// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Admin-triggered tone set location assignment: the admin enters lat/lon
// (and an optional city label) directly for each tone set, persisted on the
// talkgroups.toneSets JSON column.

package main

import (
	"fmt"
	"strings"
)

// clampRadiusMiles keeps a tone-set/talkgroup coverage radius in a sane band.
func clampRadiusMiles(r float64) float64 {
	if r <= 0 {
		return 5
	}
	if r < 1 {
		return 1
	}
	if r > 40 {
		return 40
	}
	return r
}

// ToneSetLocationRow is one tone set with optional geo for the admin dialog.
type ToneSetLocationRow struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	TalkgroupLabel  string  `json:"talkgroupLabel"`
	ToneSetId       string  `json:"toneSetId"`
	Label           string  `json:"label"`
	GeoCity         string  `json:"geoCity"`
	GeoLat          float64 `json:"geoLat"`
	GeoLon          float64 `json:"geoLon"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	LocationContext string  `json:"locationContext"`
}

// ToneSetLocationApply is one reviewed row submitted by the admin.
type ToneSetLocationApply struct {
	TalkgroupId     uint64  `json:"talkgroupId"`
	ToneSetId       string  `json:"toneSetId"`
	GeoCity         string  `json:"geoCity"`
	GeoLat          float64 `json:"geoLat"`
	GeoLon          float64 `json:"geoLon"`
	GeoRadiusMiles  float64 `json:"geoRadiusMiles"`
	LocationContext string  `json:"locationContext"`
	Clear           bool    `json:"clear"`
}

// ListToneSetLocations returns every tone set configured on a system.
func (controller *Controller) ListToneSetLocations(systemId uint64) ([]ToneSetLocationRow, error) {
	sys, ok := controller.Systems.GetSystemById(systemId)
	if !ok || sys == nil {
		return nil, fmt.Errorf("system %d not found", systemId)
	}
	out := []ToneSetLocationRow{}
	for _, tg := range sys.Talkgroups.List {
		if tg == nil || len(tg.ToneSets) == 0 {
			continue
		}
		for _, ts := range tg.ToneSets {
			if strings.TrimSpace(ts.Id) == "" {
				continue
			}
			row := ToneSetLocationRow{
				TalkgroupId:     tg.Id,
				TalkgroupLabel:  tg.Label,
				ToneSetId:       ts.Id,
				Label:           ts.Label,
				GeoCity:         ts.GeoCity,
				GeoLat:          ts.GeoLat,
				GeoLon:          ts.GeoLon,
				GeoRadiusMiles:  ts.GeoRadiusMiles,
				LocationContext: ts.LocationContext,
			}
			if row.GeoRadiusMiles <= 0 && row.GeoCity != "" {
				row.GeoRadiusMiles = 5
			}
			out = append(out, row)
		}
	}
	return out, nil
}

// ApplyToneSetLocations persists tone set geo rows on the matching talkgroups.
func (controller *Controller) ApplyToneSetLocations(systemId uint64, rows []ToneSetLocationApply) (applied, cleared int, err error) {
	sys, ok := controller.Systems.GetSystemById(systemId)
	if !ok || sys == nil {
		return 0, 0, fmt.Errorf("system %d not found", systemId)
	}

	byTalkgroup := map[uint64][]ToneSetLocationApply{}
	for _, row := range rows {
		if row.TalkgroupId == 0 || strings.TrimSpace(row.ToneSetId) == "" {
			continue
		}
		byTalkgroup[row.TalkgroupId] = append(byTalkgroup[row.TalkgroupId], row)
	}

	for tgId, tgRows := range byTalkgroup {
		tg, ok := sys.Talkgroups.GetTalkgroupById(tgId)
		if !ok || tg == nil {
			continue
		}
		changed := false
		for _, row := range tgRows {
			for i := range tg.ToneSets {
				if tg.ToneSets[i].Id != row.ToneSetId {
					continue
				}
				if row.Clear || (row.GeoLat == 0 && row.GeoLon == 0 && strings.TrimSpace(row.GeoCity) == "") {
					tg.ToneSets[i].GeoCity = ""
					tg.ToneSets[i].GeoLat = 0
					tg.ToneSets[i].GeoLon = 0
					tg.ToneSets[i].GeoRadiusMiles = 0
					tg.ToneSets[i].LocationContext = ""
					cleared++
					changed = true
					break
				}
				radius := clampRadiusMiles(row.GeoRadiusMiles)
				if radius <= 0 {
					radius = 5
				}
				tg.ToneSets[i].GeoCity = strings.TrimSpace(row.GeoCity)
				tg.ToneSets[i].GeoLat = row.GeoLat
				tg.ToneSets[i].GeoLon = row.GeoLon
				tg.ToneSets[i].GeoRadiusMiles = radius
				if ctx := strings.TrimSpace(row.LocationContext); ctx != "" {
					tg.ToneSets[i].LocationContext = ctx
				} else if tg.ToneSets[i].GeoCity != "" {
					tg.ToneSets[i].LocationContext = tg.ToneSets[i].GeoCity
				}
				applied++
				changed = true
				break
			}
		}
		if !changed {
			continue
		}
		toneSetsJson, serErr := SerializeToneSets(tg.ToneSets)
		if serErr != nil {
			return applied, cleared, serErr
		}
		if _, e := controller.Database.Sql.Exec(
			`UPDATE "talkgroups" SET "toneSets" = $1 WHERE "talkgroupId" = $2`,
			toneSetsJson, tg.Id); e != nil {
			return applied, cleared, e
		}
	}

	return applied, cleared, nil
}
