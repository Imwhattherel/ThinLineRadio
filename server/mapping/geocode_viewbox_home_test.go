package mapping

import (
	"strings"
	"testing"
)

func TestGeocodeViewboxUsesBoundsNotHomeMax(t *testing.T) {
	geo := &GeoOptions{
		BoundsLat: 41.183, BoundsLon: -80.765, BoundsRadiusMi: 5,
		HomeLat: 41.183, HomeLon: -80.765, HomeMaxRadiusMi: 30,
	}
	vb := geocodeViewboxParam(geo)
	// 5+2=7mi box, not 30mi. 30mi west edge is past -81.3.
	if strings.HasPrefix(vb, "-81.3") || strings.HasPrefix(vb, "-81.2") || strings.HasPrefix(vb, "-81.1") {
		t.Fatalf("viewbox used HomeMax (too wide): %s", vb)
	}
	// Tight Niles box should start near -80.90
	if !strings.HasPrefix(vb, "-80.8") && !strings.HasPrefix(vb, "-80.9") {
		t.Fatalf("unexpected viewbox: %s", vb)
	}
}

func TestClearPinContradictionDisabled(t *testing.T) {
	c := &CuratedAlert{
		Address: "966 EVERETT CORTLAND HULL ROAD",
		Lat:     "41.32998", Lon: "-80.78683",
	}
	tr := "966 EVERETT HALL IN FRONT OF THE FAIRGROUNDS"
	if !TranscriptContradictsAddressStreet(c.Address, tr) {
		t.Fatal("expected contradict diagnostic still true for HALL vs CORTLAND HULL")
	}
	ClearPinWhenStreetContradictsTranscript(c, tr)
	if c.Lat == "" || c.Lon == "" {
		t.Fatal("ClearPin must not wipe gateway pin")
	}
}
