package mapping

import "testing"

func TestStreetNamesSTTMatchDurstClackClagg(t *testing.T) {
	if !StreetNamesSTTMatch("DURST CLACK ROAD", "DURST CLAGG ROAD") {
		t.Fatal("CLACK→CLAGG must STT-match on Durst Clagg Road")
	}
	if TranscriptContradictsAddressStreet("3610 DURST CLAGG ROAD",
		"STATION 11 SQUAWK HALL 3610 DURST CLACK ROAD CROSSES OF STATE ROUTE 305") {
		t.Fatal("gateway CLAGG pin must not be cleared for spoken CLACK")
	}
}
