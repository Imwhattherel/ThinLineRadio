// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIIntegrationResolvedChatModel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", defaultOpenAIChatModel},
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"gpt-4o", "gpt-4o"},
		{"not-a-real-model", defaultOpenAIChatModel},
	}
	for _, tc := range tests {
		oai := OpenAIIntegration{Model: tc.in}
		if got := oai.resolvedChatModel(); got != tc.want {
			t.Fatalf("model %q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestOpenAIChatJSONNoAPIKey(t *testing.T) {
	controller := &Controller{
		Options: &Options{
			OpenAIIntegration: OpenAIIntegration{},
		},
	}
	_, err := controller.openAIChatJSON("system", "user")
	if err == nil {
		t.Fatal("expected error with empty API key")
	}
	if !strings.Contains(err.Error(), "api key not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIChatJSONInvalidKeyReturnsHTTPError(t *testing.T) {
	controller := &Controller{
		Options: &Options{
			OpenAIIntegration: OpenAIIntegration{
				APIKey:  "sk-test-invalid-key",
				BaseURL: "https://api.openai.com",
				Model:   "gpt-4o-mini",
			},
		},
	}
	_, err := controller.openAIChatJSON("Return JSON only.", `{"label":"test"}`)
	if err == nil {
		t.Fatal("expected HTTP error from OpenAI with invalid key")
	}
	if !strings.Contains(err.Error(), "openai status") {
		t.Fatalf("expected openai status error, got: %v", err)
	}
	t.Logf("OpenAI response (expected failure): %v", err)
}

func TestOpenAIChatJSONBadBaseURLReturns404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	controller := &Controller{
		Options: &Options{
			OpenAIIntegration: OpenAIIntegration{
				APIKey:  "sk-test",
				BaseURL: srv.URL,
				Model:   "gpt-4o-mini",
			},
		},
	}
	_, err := controller.openAIChatJSON("system", "user")
	if err == nil {
		t.Fatal("expected 404 error")
	}
	if !strings.Contains(err.Error(), "openai status 404") {
		t.Fatalf("expected status 404 in error, got: %v", err)
	}
}
