package mapping

import (
	"strings"
	"testing"
)

func TestStrongSpokenDispatchNotClearedByClinicalCopy(t *testing.T) {
	cases := []struct {
		name, addr, transcript string
	}{
		{
			"westbrook",
			"4100 WESTBROOK DRIVE",
			"ADVISING YOUR SQUAD HAS RESPONDED 4100 WESTBROOK DRIVE IT S A NEEDS OF THE LOBBY IT S A 41 YEAR OLD FEMALE WHO S THROWING UP AND HAS A FEVER",
		},
		{
			"bushnell",
			"4918 BUSHNELL CAMPBELL ROAD",
			"STATION 12 MUTUAL AID WITH 29 FOR A MEDIC 4918 BUSHNELL CAMPBELL ROAD CROSSES OF BRADLEY BROWNLEE AND BEACH SMITH ROAD 55 YEAR OLD MALE UNKNOWN INJURIES FOR A FALL TIME 1332",
		},
		{
			"beeman",
			"7814 BEEMAN AVENUE",
			"THAT IS A CODE 3 DAMAGE REPORT AT 7814 BEEMAN AVENUE THAT S 7814 BEEMAN SAYS THAT HIS VEHICLE WAS DAMAGED BY HIS GRANDFATHER S CAR AND HE NEEDS A REPORT",
		},
		{
			"bradford",
			"8700 BRADFORD LANE",
			"I HAVE A PARKING PERMISSION COMING FROM 8700 BRADFORD LANE THEY JUST MOVED IN THERE S NO REMOTE DRIVEWAY FOR ONE VEHICLE",
		},
		{
			"tibbetts",
			"1501 TIBBETTS WICK ASSISTED LIVING",
			"SQUAD CALL SHEPHERD OF THE VALLEY 1501 TIBBETTS WICK ASSISTED LIVING ROOM 222 97 YEAR OLD MALE GREAT HIP PAIN AFTER A FALL",
		},
		{
			"walnut",
			"65 WEST WALNUT NO CONTACT",
			"SILENT PANIC ALARM COMING FROM PAPA JOHN 65 WEST WALNUT NO CONTACT MADE WITH EMPLOYEES THERE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &CuratedAlert{Address: tc.addr}
			SanitizeExtractedAddress(c)
			ApplyExtractedAddressGuards(c, tc.transcript, nil)
			if strings.TrimSpace(c.Address) == "" {
				t.Fatalf("cleared address %q", tc.addr)
			}
		})
		t.Run(tc.name+"/process", func(t *testing.T) {
			out := Process(ProcessInput{
				Transcript: tc.transcript,
				Engine:     "local",
				Geo:        &GeoOptions{SkipExternalGeocode: true},
			})
			addr := ""
			if out.Primary != nil {
				addr = strings.TrimSpace(out.Primary.Address)
			}
			if addr == "" {
				t.Fatalf("Process cleared address; status=%s", out.Status)
			}
		})
	}
}

func TestPlaceNarrativeKeepsSuffixedStreets(t *testing.T) {
	tr := "DAMAGE REPORT AT 7814 BEEMAN AVENUE THAT S 7814 BEEMAN SAYS THAT HIS VEHICLE WAS DAMAGED"
	if AddressIsPlaceNarrativeNotDispatch("7814 BEEMAN AVENUE", tr) {
		t.Fatal("suffixed street should not be place-narrative")
	}
}

func TestPatientClinicalFollowUpAllowsNumberedDispatch(t *testing.T) {
	tr := "4100 WESTBROOK DRIVE 41 YEAR OLD FEMALE THROWING UP AND HAS A FEVER"
	if TranscriptIsPatientClinicalFollowUp(tr) {
		t.Fatal("toned house address should not be blocked as clinical follow-up")
	}
}

func TestStripFacilityPhraseTail(t *testing.T) {
	got := sanitizeAddressField("1501 TIBBETTS WICK ASSISTED LIVING")
	if got != "1501 TIBBETTS WICK" {
		t.Fatalf("got %q", got)
	}
	got = sanitizeAddressField("65 WEST WALNUT NO CONTACT")
	if got != "65 WEST WALNUT" {
		t.Fatalf("got %q", got)
	}
}
