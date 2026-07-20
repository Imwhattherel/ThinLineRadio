// Copyright (C) 2025 Thinline Dynamic Solutions

package mapping

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testGeoWithBounds() *GeoOptions {
	return &GeoOptions{
		BoundsLat:      41.5,
		BoundsLon:      -80.7,
		BoundsRadiusMi: 15,
	}
}

func TestScanTranscriptForStreetsNoGatewayConfigured(t *testing.T) {
	if hits := ScanTranscriptForStreets("", "key", "LONGWOOD", testGeoWithBounds()); hits != nil {
		t.Fatalf("expected nil with no gateway URL, got %v", hits)
	}
	if hits := ScanTranscriptForStreets("http://example.invalid", "", "LONGWOOD", testGeoWithBounds()); hits != nil {
		t.Fatalf("expected nil with no API key, got %v", hits)
	}
}

func TestScanTranscriptForStreetsRequiresViewbox(t *testing.T) {
	// No bounds on geo at all -> geocodeViewboxParam returns "" -> must not call out.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hits := ScanTranscriptForStreets(srv.URL, "key", "LONGWOOD", nil)
	if hits != nil {
		t.Fatalf("expected nil hits, got %v", hits)
	}
	if called {
		t.Fatalf("expected no HTTP call when viewbox is empty")
	}
}

func TestScanTranscriptForStreetsReturnsHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req streetScanRequestBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Viewbox == "" {
			t.Errorf("expected non-empty viewbox in request body")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(streetScanResponseBody{
			Hits: []StreetScanHit{{MatchedText: "LONGWOOD", StreetName: "LONGWOOD DRIVE", Exact: false}},
		})
	}))
	defer srv.Close()

	hits := ScanTranscriptForStreets(srv.URL, "key", "1717 LONGWOOD", testGeoWithBounds())
	if len(hits) != 1 || hits[0].StreetName != "LONGWOOD DRIVE" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}
