package mapping

import "testing"

func TestSuppressUnclassifiedPinOnAlert(t *testing.T) {
	t.Parallel()
	status := "geocoded"
	curated := &CuratedAlert{
		NatureDesc: "UNKNOWN PROBLEM",
		Lat:        "41.5",
		Lon:        "-81.5",
		Address:    "123 MAIN ST",
	}
	SuppressUnclassifiedPinOnAlert(curated, &status)
	if curated.Lat != "" || curated.Lon != "" {
		t.Fatalf("expected pin cleared, got lat=%q lon=%q", curated.Lat, curated.Lon)
	}
	if curated.Address != "123 MAIN ST" {
		t.Fatalf("address should be preserved, got %q", curated.Address)
	}
	if status != "failed" {
		t.Fatalf("status=%q want failed", status)
	}

	ok := &CuratedAlert{NatureDesc: "STRUCTURE FIRE", Lat: "41.5", Lon: "-81.5"}
	st := "geocoded"
	SuppressUnclassifiedPinOnAlert(ok, &st)
	if ok.Lat == "" || ok.Lon == "" {
		t.Fatal("classified nature should keep pin")
	}
	if st != "geocoded" {
		t.Fatalf("status=%q want geocoded", st)
	}
}
