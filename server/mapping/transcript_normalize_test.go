package mapping

import "testing"

func TestNormalizeTranscriptPlainText(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  hello  ", "HELLO"},
		{"AT 10701 EAST BOULEVARD, AT THE CLEVELAND VA MEDICAL CENTER.", "AT 10701 EAST BOULEVARD AT THE CLEVELAND VA MEDICAL CENTER"},
		{"10-20 CENTER STREET", "1020 CENTER STREET"},
		{"APT#2", "APT 2"},
		{"MAIN & ELM", "MAIN AND ELM"},
		{"U.S. ROUTE 422", "U S ROUTE 422"},
		{"STATION40", "STATION 40"},
		{"40STATION", "40 STATION"},
		{"85-E-212 STREET", "85 E 212 STREET"},
		{"O'BRIEN ROAD", "O BRIEN ROAD"},
		{"CROSS OF 88, AND BRADLEY", "CROSS OF 88 AND BRADLEY"},
		{"74-YEAR-OLD", "74 YEAR OLD"},
		// Ordinals stay glued — do not become "88 TH" / "3 RD".
		{"3235 W 88TH ST", "3235 W 88TH ST"},
		{"113 E 2ND ST", "113 E 2ND ST"},
		{"3RD STREET", "3RD STREET"},
		{"1ST AVENUE", "1ST AVENUE"},
		{"798 RICHMOND RD", "798 RICHMOND RD"},
		{"798 RICHMOND RD.", "798 RICHMOND RD"},
		{"125TH", "125TH"},
	}
	for _, tt := range tests {
		got := NormalizeTranscriptPlainText(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeTranscriptPlainText(%q)=\n  %q\nwant %q", tt.in, got, tt.want)
		}
	}
}

func TestPreCleanTranscriptRunsPlainNormalizeFirst(t *testing.T) {
	got := PreCleanTranscript("AT 10701 EAST BOULEVARD, AT THE CLEVELAND VA.")
	if got == "" {
		t.Fatal("empty")
	}
	for _, bad := range []string{",", ".", "-", "#", "&", "'"} {
		if containsRune(got, bad[0]) {
			t.Errorf("PreClean still has %q in %q", bad, got)
		}
	}
}

func containsRune(s string, r byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == r {
			return true
		}
	}
	return false
}
