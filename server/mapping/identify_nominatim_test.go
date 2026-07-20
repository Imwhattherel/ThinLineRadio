// Copyright (C) 2025 Thinline Dynamic Solutions

package mapping

import "testing"

func TestAddressCandidateHasDigit(t *testing.T) {
	if !AddressCandidateHasDigit("1717 LONGWOOD") {
		t.Fatal("expected digit address")
	}
	if !AddressCandidateHasDigit("1137 E 125TH ST") {
		t.Fatal("expected ordinal address")
	}
	if AddressCandidateHasDigit("MAIN STREET") {
		t.Fatal("suffix-only should not count as digit candidate")
	}
}

func TestDropUnimportAnchoredAddressKeepsDigitCandidates(t *testing.T) {
	scope := &ScopeData{KnownStreets: []string{"OTHER ROAD"}}
	curated := &CuratedAlert{Address: "1717 LONGWOOD DR"}
	DropUnimportAnchoredAddress(curated, "RESPOND 1717 LONGWOOD DRIVE", scope)
	if curated.Address == "" {
		t.Fatal("digit address must pass through for Nominatim even without gazetteer match")
	}
}

func TestDropUnimportAnchoredAddressStillDropsNonDigitUngrounded(t *testing.T) {
	scope := &ScopeData{KnownStreets: []string{"OTHER ROAD"}}
	// Address not spoken in transcript and not in gazetteer.
	curated := &CuratedAlert{Address: "ZYZZYX HOLLOW"}
	DropUnimportAnchoredAddress(curated, "UNITS CLEAR FROM THE LAST RUN", scope)
	if curated.Address != "" {
		t.Fatalf("non-digit ungrounded chatter should still clear, got %q", curated.Address)
	}
}

func TestProcessDigitAddressDoesNotLocalPin(t *testing.T) {
	out := Process(ProcessInput{
		Transcript: "SQUAD NEEDED AT 1717 LONGWOOD DRIVE FOR A FALL",
		ToneSetLabel: "TEST",
		Scope: &ScopeData{
			KnownStreets: []string{"LONGWOOD DRIVE", "LONGWOOD ROAD"},
		},
		Geo: &GeoOptions{BoundsLat: 41.5, BoundsLon: -81.5, BoundsRadiusMi: 5, State: "OH"},
	})
	if out.Primary == nil {
		t.Fatal("expected primary")
	}
	if !AddressCandidateHasDigit(out.Primary.Address) {
		t.Fatalf("expected digit address extract, got %q", out.Primary.Address)
	}
	if out.Primary.Lat != "" || out.Primary.Lon != "" {
		t.Fatalf("Process must not local-pin digit addresses; Nominatim owns placement (lat=%q lon=%q source=%q)",
			out.Primary.Lat, out.Primary.Lon, out.Source)
	}
}

func TestApplyExtractedAddressGuardsRejectsStationApparatusLabel(t *testing.T) {
	curated := &CuratedAlert{Address: "40 STATION"}
	ApplyExtractedAddressGuards(curated, "STATION 40 AND 41 FOR THE STRUCTURE FIRE", nil)
	if curated.Address != "" {
		t.Fatalf("station apparatus digit must not remain as address, got %q", curated.Address)
	}
}

// Empty card address after a location-bearing transcript must be "failed"
// (gateway may still recover), not "skipped" (which blocked POST /transcript).
func TestProcessEmptyAddressAfterLocationScreenIsFailedNotSkipped(t *testing.T) {
	out := Process(ProcessInput{
		Transcript:   "YOU CAN SHOW MYSELF AND 32 EN ROUTE TO THE STATION",
		ToneSetLabel: "TEST",
		Scope:        &ScopeData{},
		Geo:          &GeoOptions{BoundsLat: 41.5, BoundsLon: -81.5, BoundsRadiusMi: 5, State: "OH"},
	})
	if out.Status == "skipped" {
		t.Fatalf("location-bearing extract miss must not be skipped (blocks gateway); got status=%q addr=%q",
			out.Status, out.Primary.Address)
	}
}

func TestProcessNoLocationPreScreenStillSkipped(t *testing.T) {
	out := Process(ProcessInput{
		Transcript:   "COPY THAT UNIT CLEAR",
		ToneSetLabel: "TEST",
		Scope:        &ScopeData{},
		Geo:          &GeoOptions{BoundsLat: 41.5, BoundsLon: -81.5, BoundsRadiusMi: 5, State: "OH"},
	})
	if out.Status != "skipped" {
		t.Fatalf("chatter with no location signal should stay skipped, got %q", out.Status)
	}
}
