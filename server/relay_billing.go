// Copyright (C) 2025 Thinline Dynamic Solutions
//
// relay_billing.go — fetch billing plans and start Stripe checkout via relay.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func relayAPIKeyRequest(controller *Controller, method, path string, body map[string]any) (map[string]any, int, error) {
	relayURL := getRelayServerURL()
	apiKey := strings.TrimSpace(controller.Options.RelayServerAPIKey)
	if apiKey == "" {
		return nil, 0, fmt.Errorf("relay server API key is not configured")
	}
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	var req *http.Request
	var err error
	if reqBody != nil {
		req, err = http.NewRequest(method, relayURL+path, bytes.NewReader(reqBody))
	} else {
		req, err = http.NewRequest(method, relayURL+path, nil)
	}
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if token := controller.RelayAccountAccessTokenSnapshot(); token != "" {
		req.Header.Set("X-Account-Token", token)
	}
	client := &http.Client{Timeout: relayAccountHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	var data map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			snippet := strings.TrimSpace(string(raw))
			if len(snippet) > 120 {
				snippet = snippet[:120] + "…"
			}
			if strings.HasPrefix(snippet, "<") {
				return nil, resp.StatusCode, fmt.Errorf("relay returned HTML instead of JSON — rebuild/restart the relay server")
			}
			return nil, resp.StatusCode, fmt.Errorf("relay returned an invalid response")
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := "relay request failed"
		if data != nil {
			if e, ok := data["error"].(string); ok && e != "" {
				msg = e
			}
		}
		return data, resp.StatusCode, fmt.Errorf("%s", msg)
	}
	return data, resp.StatusCode, nil
}

// RelayBillingCatalog returns active plans and entitlements for this server's API key.
func (controller *Controller) RelayBillingCatalog() (map[string]any, error) {
	data, _, err := relayAPIKeyRequest(controller, http.MethodGet, "/api/billing/catalog", nil)
	if err != nil {
		return nil, err
	}
	if data == nil || data["plans"] == nil {
		return nil, fmt.Errorf("relay billing catalog response was empty — rebuild/restart the relay server")
	}
	return data, nil
}

// RelayPlanSubscribe starts Stripe Checkout for the given plan slug.
func (controller *Controller) RelayPlanSubscribe(planSlug string) (string, error) {
	planSlug = strings.TrimSpace(strings.ToLower(planSlug))
	if planSlug == "" {
		return "", fmt.Errorf("plan_slug is required")
	}
	data, _, err := relayAPIKeyRequest(controller, http.MethodPost, "/api/billing/subscribe", map[string]any{
		"plan_slug": planSlug,
	})
	if err != nil {
		return "", err
	}
	checkoutURL, _ := data["checkout_url"].(string)
	if checkoutURL == "" {
		return "", fmt.Errorf("relay did not return a checkout URL")
	}
	return checkoutURL, nil
}
