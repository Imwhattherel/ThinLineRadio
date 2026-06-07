// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import "testing"

func TestToneHistoryShouldExpandLookback(t *testing.T) {
	cfgRequired := 3
	resp := &ToneHistoryAnalyzeResponse{CallsScanned: 20}
	aggs := map[string]*toneHistoryAgg{
		"x": {records: []toneLearnCallRecord{{CallId: 1}}},
	}
	if !toneHistoryShouldExpandLookback(resp, aggs, cfgRequired, toneHistoryInitialHours) {
		t.Fatal("partial below threshold should expand")
	}

	resp.Suggestions = []ToneHistorySuggestion{{Label: "ok"}}
	if toneHistoryShouldExpandLookback(resp, aggs, cfgRequired, toneHistoryInitialHours) {
		t.Fatal("suggestions found should not expand")
	}

	resp = &ToneHistoryAnalyzeResponse{CallsScanned: 20, CallsWithCandidates: 0}
	if !toneHistoryShouldExpandLookback(resp, map[string]*toneHistoryAgg{}, cfgRequired, toneHistoryInitialHours) {
		t.Fatal("no candidates should expand")
	}

	resp = &ToneHistoryAnalyzeResponse{CallsScanned: toneHistoryMaxCalls}
	if toneHistoryShouldExpandLookback(resp, map[string]*toneHistoryAgg{}, cfgRequired, toneHistoryInitialHours) {
		t.Fatal("max calls should stop")
	}
}

func TestToneHistoryNextLookbackHours(t *testing.T) {
	if got := toneHistoryNextLookbackHours(168); got != 336 {
		t.Fatalf("168 -> %d", got)
	}
	if got := toneHistoryNextLookbackHours(480); got != toneHistoryMaxHours {
		t.Fatalf("480 -> %d", got)
	}
}
