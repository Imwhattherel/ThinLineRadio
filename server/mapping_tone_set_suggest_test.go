package main

import "testing"

func TestLocalityNameForBoundary(t *testing.T) {
	cases := []struct {
		search, city, label, want string
	}{
		{"Austinburg Township, Ashtabula County, OH", "", "", "Austinburg"},
		{"Jefferson, OH", "Jefferson Fire", "Jefferson Fire", "Jefferson"},
		{"", "", "Morgan Hose Fire", "Morgan"},
		{"NAD AUX", "", "NAD AUX", "NAD"},
	}
	for _, c := range cases {
		got := localityNameForBoundary(c.search, c.city, c.label)
		if got != c.want {
			t.Fatalf("localityNameForBoundary(%q,%q,%q)=%q want %q", c.search, c.city, c.label, got, c.want)
		}
	}
}
