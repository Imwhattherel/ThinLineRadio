// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"encoding/json"
)

// applyBulkToneDetection enables or disables tone detection on talkgroups matching
// the system's bulk rollout tag selection. Called before talkgroups are persisted.
// Production tone matching stays on until bulk rollout is turned off manually.
func (system *System) applyBulkToneDetection() {
	if system == nil || system.Talkgroups == nil {
		return
	}

	tagSet := bulkToneTagSet(system.BulkToneDetectionTagIds)
	if len(tagSet) == 0 {
		return
	}

	system.BulkToneDetectionAutoOffDays = 0
	system.BulkToneDetectionExpiresAt = 0

	if system.BulkToneDetectionEnabled {
		for _, tg := range system.Talkgroups.List {
			if tagSet[tg.TagId] {
				tg.ToneDetectionEnabled = true
			}
		}
		return
	}

	// Bulk rollout off — disable tone detection on talkgroups in the configured tags only.
	for _, tg := range system.Talkgroups.List {
		if tagSet[tg.TagId] {
			tg.ToneDetectionEnabled = false
		}
	}
}

func bulkToneTagSet(tagIds []uint64) map[uint64]bool {
	out := make(map[uint64]bool, len(tagIds))
	for _, id := range tagIds {
		if id > 0 {
			out[id] = true
		}
	}
	return out
}

func parseBulkToneTagIds(jsonStr string) []uint64 {
	if jsonStr == "" || jsonStr == "[]" {
		return nil
	}
	var ids []uint64
	if err := json.Unmarshal([]byte(jsonStr), &ids); err != nil {
		return nil
	}
	return ids
}

func serializeBulkToneTagIds(ids []uint64) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(b)
}
