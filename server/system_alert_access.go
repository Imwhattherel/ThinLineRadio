// Copyright (C) 2025 Thinline Dynamic Solutions
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import (
	"encoding/json"
	"strconv"
)

func parseSystemAlertData(alert *SystemAlert) *SystemAlertData {
	if alert == nil || alert.Data == "" || alert.Data == "{}" {
		return nil
	}
	var data SystemAlertData
	if err := json.Unmarshal([]byte(alert.Data), &data); err != nil {
		return nil
	}
	return &data
}

func (controller *Controller) userHasApiKeyAlertAccess(user *User, apiKeyId uint64) bool {
	if user == nil || apiKeyId == 0 {
		return false
	}
	apikey, ok := controller.Apikeys.GetById(apiKeyId)
	if !ok {
		return false
	}
	switch v := apikey.Systems.(type) {
	case string:
		if v == "*" {
			return user.HasAnySystemAccess()
		}
		return false
	case []any:
		for _, entry := range v {
			scope, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			idVal, ok := scope["id"]
			if !ok {
				continue
			}
			var systemRef uint
			switch id := idVal.(type) {
			case float64:
				systemRef = uint(id)
			case string:
				if parsed, err := strconv.ParseUint(id, 10, 32); err == nil {
					systemRef = uint(parsed)
				}
			}
			if systemRef > 0 && controller.userHasSystemScopeAccess(user, systemRef) {
				return true
			}
		}
	}
	return false
}

func (controller *Controller) userHasSystemAlertAccess(user *User, alert *SystemAlert) bool {
	if user == nil || alert == nil {
		return false
	}
	if !isSystemAlertVisibleToUser(alert.AlertType, user.SystemAdmin) {
		return false
	}
	if user.SystemAdmin {
		return true
	}

	data := parseSystemAlertData(alert)

	switch alert.AlertType {
	case "manual":
		return true
	case "no_audio", "no_audio_received":
		if data == nil || data.SystemId == 0 {
			return false
		}
		sys, ok := controller.Systems.GetSystemById(data.SystemId)
		if !ok {
			return false
		}
		return controller.userHasSystemScopeAccess(user, sys.SystemRef)
	case "api_key_no_audio":
		if data == nil || data.ApiKeyId == 0 {
			return false
		}
		return controller.userHasApiKeyAlertAccess(user, data.ApiKeyId)
	case "tone_detection_issue":
		if data == nil || data.SystemId == 0 || data.TalkgroupId == 0 {
			return false
		}
		sys, ok := controller.Systems.GetSystemById(data.SystemId)
		if !ok {
			return false
		}
		tg, ok := sys.Talkgroups.GetTalkgroupById(data.TalkgroupId)
		if !ok {
			return false
		}
		return controller.userHasTalkgroupScopeAccess(user, sys.SystemRef, tg.TalkgroupRef)
	default:
		return false
	}
}
