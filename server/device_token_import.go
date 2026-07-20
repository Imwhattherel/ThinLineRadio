// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Helpers for config export/import of push device tokens.

package main

import "strings"

// normalizeImportedDeviceTokenFields reconciles legacy exports that only stored
// the push token in "token" with newer exports that include fcmToken/pushType.
func normalizeImportedDeviceTokenFields(m map[string]any) (token, fcmToken, pushType string) {
	token = mapString(m, "token")
	fcmToken = mapString(m, "fcmToken")
	if fcmToken == "" {
		fcmToken = mapString(m, "fcm_token")
	}
	pushType = mapString(m, "pushType")
	if pushType == "" {
		pushType = mapString(m, "push_type")
	}

	if fcmToken == "" && token != "" {
		fcmToken = token
	}
	if token == "" && fcmToken != "" {
		token = fcmToken
	}
	if pushType == "" && fcmToken != "" {
		if strings.HasPrefix(fcmToken, "voip:") {
			pushType = "voip"
		} else {
			pushType = "fcm"
		}
	}
	return token, fcmToken, pushType
}

// deviceTokenDisplayFields returns the push key and type shown in admin UI.
func deviceTokenDisplayFields(dt *DeviceToken) (displayToken, pushType string, ok bool) {
	if dt == nil {
		return "", "", false
	}
	displayToken = dt.FCMToken
	pushType = dt.PushType
	if displayToken == "" {
		displayToken = dt.Token
	}
	if displayToken == "" {
		return "", "", false
	}
	if pushType == "" {
		if strings.HasPrefix(displayToken, "voip:") {
			pushType = "voip"
		} else {
			pushType = "fcm"
		}
	}
	return displayToken, pushType, true
}

func mapString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
