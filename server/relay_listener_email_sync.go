// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Listener emails on the relay: one-time full list after upgrade, then add/remove/update deltas.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)


func (controller *Controller) maybeBootstrapRelayListenerEmails() {
	if controller.Options.RelayServerAPIKey == "" {
		log.Printf("relay listener emails: skipping initial sync (relayServerAPIKey not configured)")
		return
	}
	if controller.Options.RelayListenerEmailsInitialSyncDone {
		return
	}
	if !controller.postRelayListenerEmailFullReplace() {
		log.Printf("relay listener emails: initial full sync failed — will retry on next restart")
		return
	}
	controller.Options.RelayListenerEmailsInitialSyncDone = true
	if err := controller.Options.Write(controller.Database); err != nil {
		log.Printf("relay listener emails: save initial-sync flag: %v", err)
		return
	}
	controller.SyncConfigToFile()
	log.Printf("relay listener emails: initial full sync completed and flag saved")
}

func (controller *Controller) postRelayListenerEmailFullReplace() bool {
	emails := collectListenerEmailsForRelay(controller.Users)
	log.Printf("relay listener emails: POST /api/scanner-listener-emails (%d listener email(s))", len(emails))
	payload, err := json.Marshal(map[string][]string{"emails": emails})
	if err != nil {
		log.Printf("relay listener emails: marshal: %v", err)
		return false
	}
	u := getRelayServerURL() + "/api/scanner-listener-emails"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		log.Printf("relay listener emails: request: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(controller.Options.RelayServerAPIKey))
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("relay listener emails: post full list: %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("relay listener emails: relay returned %d: %s", resp.StatusCode, string(body))
		return false
	}
	return true
}

func (controller *Controller) postRelayListenerEmailDelta(add, remove []string) {
	if controller.Options.RelayServerAPIKey == "" {
		return
	}
	add = dedupeNormEmails(add)
	remove = dedupeNormEmails(remove)
	if len(add) == 0 && len(remove) == 0 {
		return
	}
	payload, err := json.Marshal(map[string][]string{"add": add, "remove": remove})
	if err != nil {
		log.Printf("relay listener emails delta: marshal: %v", err)
		return
	}
	u := getRelayServerURL() + "/api/scanner-listener-emails/delta"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		log.Printf("relay listener emails delta: request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(controller.Options.RelayServerAPIKey))
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("relay listener emails delta: post: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("relay listener emails delta: relay returned %d: %s", resp.StatusCode, string(body))
	}
}

func (controller *Controller) relayListenerEmailAdded(email string) {
	controller.postRelayListenerEmailDelta([]string{email}, nil)
}

func (controller *Controller) relayListenerEmailRemoved(email string) {
	controller.postRelayListenerEmailDelta(nil, []string{email})
}

func (controller *Controller) relayListenerEmailChanged(oldEmail, newEmail string) {
	var add, remove []string
	if oldEmail != "" {
		remove = append(remove, oldEmail)
	}
	if newEmail != "" {
		add = append(add, newEmail)
	}
	controller.postRelayListenerEmailDelta(add, remove)
}

func dedupeNormEmails(in []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, e := range in {
		n := strings.TrimSpace(strings.ToLower(e))
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

func collectListenerEmailsForRelay(users *Users) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, u := range users.GetAllUsers() {
		e := NormalizeEmail(u.Email)
		if e == "" {
			continue
		}
		if err := ValidateEmail(e); err != nil {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out
}
