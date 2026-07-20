// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Summary payload returned after a full config import.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type ConfigImportSummaryItem struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Expected int    `json:"expected"`
	Imported int    `json:"imported"`
	OK       bool   `json:"ok"`
}

func importPayloadLen(v any) int {
	switch s := v.(type) {
	case []any:
		return len(s)
	default:
		return 0
	}
}

func importSystemsNestedCount(m map[string]any, field string) int {
	systems, ok := m["systems"].([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, item := range systems {
		sm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if arr, ok := sm[field].([]any); ok {
			total += len(arr)
		}
	}
	return total
}

func (admin *Admin) countSystemsTalkgroups() int {
	total := 0
	for _, sys := range admin.Controller.Systems.List {
		if sys.Talkgroups != nil {
			total += len(sys.Talkgroups.List)
		}
	}
	return total
}

func (admin *Admin) countSystemsUnits() int {
	total := 0
	for _, sys := range admin.Controller.Systems.List {
		if sys.Units != nil {
			total += len(sys.Units.List)
		}
	}
	return total
}

func (admin *Admin) countDeviceTokens() int {
	admin.Controller.DeviceTokens.mutex.RLock()
	defer admin.Controller.DeviceTokens.mutex.RUnlock()
	return len(admin.Controller.DeviceTokens.tokens)
}

func (admin *Admin) countUserAlertPreferences() int {
	var count int
	if err := admin.Controller.Database.Sql.QueryRow(`SELECT COUNT(*) FROM "userAlertPreferences"`).Scan(&count); err != nil {
		return 0
	}
	return count
}

func (admin *Admin) buildConfigImportSummary(importPayload map[string]any) []ConfigImportSummaryItem {
	add := func(key, label string, expected, imported int) ConfigImportSummaryItem {
		ok := expected == imported
		if expected == 0 && imported == 0 {
			ok = true
		}
		return ConfigImportSummaryItem{
			Key:      key,
			Label:    label,
			Expected: expected,
			Imported: imported,
			OK:       ok,
		}
	}

	items := []ConfigImportSummaryItem{}

	if expected := importPayloadLen(importPayload["apikeys"]); expected > 0 {
		items = append(items, add("apikeys", "API Keys", expected, len(admin.Controller.Apikeys.List)))
	}
	if expected := importPayloadLen(importPayload["dirwatch"]); expected > 0 {
		items = append(items, add("dirwatch", "Dirwatch", expected, len(admin.Controller.Dirwatches.List)))
	}
	if expected := importPayloadLen(importPayload["downstreams"]); expected > 0 {
		items = append(items, add("downstreams", "Downstreams", expected, len(admin.Controller.Downstreams.List)))
	}
	if expected := importPayloadLen(importPayload["groups"]); expected > 0 {
		items = append(items, add("groups", "Groups", expected, len(admin.Controller.Groups.List)))
	}
	if expected := importPayloadLen(importPayload["tags"]); expected > 0 {
		items = append(items, add("tags", "Tags", expected, len(admin.Controller.Tags.List)))
	}
	if expected := importPayloadLen(importPayload["systems"]); expected > 0 {
		items = append(items, add("systems", "Systems", expected, len(admin.Controller.Systems.List)))
	}
	if expected := importSystemsNestedCount(importPayload, "talkgroups"); expected > 0 {
		items = append(items, add("talkgroups", "Talkgroups", expected, admin.countSystemsTalkgroups()))
	}
	if expected := importSystemsNestedCount(importPayload, "units"); expected > 0 {
		items = append(items, add("units", "Units", expected, admin.countSystemsUnits()))
	}
	if _, hasOptions := importPayload["options"].(map[string]any); hasOptions {
		items = append(items, add("options", "Options", 1, 1))
	}
	if expected := importPayloadLen(importPayload["userGroups"]); expected > 0 {
		items = append(items, add("userGroups", "User Groups", expected, len(admin.Controller.UserGroups.GetAll())))
	}
	if expected := importPayloadLen(importPayload["users"]); expected > 0 {
		items = append(items, add("users", "Users", expected, len(admin.Controller.Users.GetAllUsers())))
	}
	if expected := importPayloadLen(importPayload["keywordLists"]); expected > 0 {
		items = append(items, add("keywordLists", "Keyword Lists", expected, len(admin.Controller.KeywordListsCache.GetAllLists())))
	}
	if expected := importPayloadLen(importPayload["userAlertPreferences"]); expected > 0 {
		items = append(items, add("userAlertPreferences", "Alert Preferences", expected, admin.countUserAlertPreferences()))
	}
	if expected := importPayloadLen(importPayload["deviceTokens"]); expected > 0 {
		items = append(items, add("deviceTokens", "Push Device Tokens", expected, admin.countDeviceTokens()))
	}

	return items
}

func (admin *Admin) writeConfigResponse(w http.ResponseWriter, extras map[string]any) {
	m := map[string]any{
		"config":             admin.GetConfig(),
		"passwordNeedChange": admin.Controller.Options.adminPasswordNeedChange,
	}
	if _, docker := os.LookupEnv("DOCKER"); docker {
		m["docker"] = docker
	}
	for k, v := range extras {
		m[k] = v
	}
	if b, err := json.Marshal(m); err == nil {
		w.Write(b)
	} else {
		w.WriteHeader(http.StatusExpectationFailed)
	}
}

func formatImportSummaryMessage(items []ConfigImportSummaryItem) string {
	failed := 0
	for _, item := range items {
		if !item.OK {
			failed++
		}
	}
	if failed == 0 {
		return fmt.Sprintf("All %d sections imported successfully", len(items))
	}
	return fmt.Sprintf("%d of %d sections imported with mismatches", failed, len(items))
}
