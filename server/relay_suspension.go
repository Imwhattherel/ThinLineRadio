// Copyright (C) 2025 Thinline Dynamic Solutions
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>
//
// Relay-driven full suspension: blocks public web listener and WebSocket (not /admin).
// Push notifications remain disabled while suspended even if the operator unlocks the public UI from admin.

package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type relaySuspensionSnapshot struct {
	suspended bool
	message   string
}

func (controller *Controller) getRelaySuspensionSnapshot() relaySuspensionSnapshot {
	controller.RelaySuspensionMu.RLock()
	defer controller.RelaySuspensionMu.RUnlock()
	return relaySuspensionSnapshot{
		suspended: controller.RelayFullySuspended,
		message:   controller.RelaySuspendMessage,
	}
}

func (controller *Controller) applyRelaySuspensionState(fullySuspended bool, message string) {
	controller.RelaySuspensionMu.Lock()
	was := controller.RelayFullySuspended
	controller.RelayFullySuspended = fullySuspended
	controller.RelaySuspendMessage = message
	controller.RelaySuspensionMu.Unlock()

	if !fullySuspended {
		controller.Options.mutex.Lock()
		controller.Options.RelayOwnerUnlockedPublicClient = false
		controller.Options.mutex.Unlock()
		if err := controller.Options.Write(controller.Database); err != nil {
			log.Printf("relay suspension: failed to persist cleared owner unlock: %v", err)
		}
	}
	if was != fullySuspended {
		log.Printf("relay suspension: fully_suspended=%v", fullySuspended)
	}
}

// RelayPushSuspended is true when the relay has fully suspended this server (push must not be sent).
func (controller *Controller) RelayPushSuspended() bool {
	controller.RelaySuspensionMu.RLock()
	defer controller.RelaySuspensionMu.RUnlock()
	return controller.RelayFullySuspended
}

// IsPublicWebListenerBlocked is true when the public web app and non-admin WebSockets must be denied.
func (controller *Controller) IsPublicWebListenerBlocked() bool {
	controller.RelaySuspensionMu.RLock()
	fs := controller.RelayFullySuspended
	controller.RelaySuspensionMu.RUnlock()
	if !fs {
		return false
	}
	controller.Options.mutex.Lock()
	unlocked := controller.Options.RelayOwnerUnlockedPublicClient
	controller.Options.mutex.Unlock()
	return !unlocked
}

func (controller *Controller) PublicSuspensionMessage() string {
	controller.RelaySuspensionMu.RLock()
	defer controller.RelaySuspensionMu.RUnlock()
	return controller.RelaySuspendMessage
}

func (controller *Controller) disconnectPublicWebClientsForSuspension() {
	controller.Clients.mutex.Lock()
	defer controller.Clients.mutex.Unlock()
	for c := range controller.Clients.Map {
		if c == nil || c.Conn == nil {
			continue
		}
		if c.IsAdmin {
			continue
		}
		_ = c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4000, "server suspended"))
		c.Conn.Close()
	}
}

func (controller *Controller) pollRelaySuspensionOnce() {
	relayURL := getRelayServerURL()
	apiKey := strings.TrimSpace(controller.Options.RelayServerAPIKey)
	if apiKey == "" {
		return
	}
	u := relayURL + "/api/keys/details"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return
	}
	req.Header.Set("X-Rdio-Auth", getRelayServerAuthKey())
	req.Header.Set("X-API-Key", apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}
	suspended := false
	switch v := data["fully_suspended"].(type) {
	case bool:
		suspended = v
	case float64:
		suspended = v != 0
	}
	msg, _ := data["suspend_message"].(string)
	prev := controller.getRelaySuspensionSnapshot()
	controller.applyRelaySuspensionState(suspended, msg)
	if suspended && !prev.suspended {
		go controller.disconnectPublicWebClientsForSuspension()
	}
}

func (controller *Controller) startRelaySuspensionPoller() {
	controller.pollRelaySuspensionOnce()
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		controller.pollRelaySuspensionOnce()
	}
}

// RelaySuspensionWebhookHandler receives suspension updates from the ThinLine relay server.
// POST /api/webhook/relay-suspension — authenticated with X-API-Key matching this server's relay API key.
func (api *Api) RelaySuspensionWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.exitWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	key := r.Header.Get("X-API-Key")
	if key == "" || key != api.Controller.Options.RelayServerAPIKey {
		api.exitWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var body struct {
		FullySuspended bool   `json:"fully_suspended"`
		SuspendMessage string `json:"suspend_message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.exitWithError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	prev := api.Controller.getRelaySuspensionSnapshot()
	api.Controller.applyRelaySuspensionState(body.FullySuspended, body.SuspendMessage)
	if body.FullySuspended && !prev.suspended {
		go api.Controller.disconnectPublicWebClientsForSuspension()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func writePublicRelaySuspensionPage(w http.ResponseWriter, controller *Controller) {
	custom := strings.TrimSpace(controller.PublicSuspensionMessage())
	if custom != "" {
		custom = "<p style=\"margin:16px 0;padding:12px;background:#fff8e1;border-radius:8px;border:1px solid #ffe082;\"><strong>Notice from administration:</strong><br>" + html.EscapeString(custom) + "</p>"
	}
	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Server suspended</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;background:#121212;color:#e0e0e0;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;}
.box{max-width:560px;background:#1e1e1e;border:1px solid #333;border-radius:12px;padding:28px;}
h1{color:#ef5350;font-size:22px;margin:0 0 12px;}
p{line-height:1.55;color:#ccc;}
a{color:#64b5f6;}
.small{font-size:13px;color:#888;margin-top:20px;}
</style>
</head>
<body>
<div class="box">
<h1>This scanner server is suspended</h1>
<p>The public listener interface has been disabled by <strong>Thinline Radio Administration</strong>.</p>
%s
<p class="small"><strong>If you operate this server:</strong> do not contact support on behalf of listeners. Email <a href="mailto:support@thinlineds.com">support@thinlineds.com</a> from your operator email so we can review your case. You may still sign in to the <strong>admin</strong> area on this host to manage settings or use &ldquo;Unlock public web listener&rdquo; there to restore the web app for listeners; <strong>mobile push notifications stay disabled</strong> until administration clears the suspension on the relay.</p>
</div>
</body>
</html>`, custom)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, page)
}
