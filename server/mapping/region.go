// Copyright (C) 2025 Thinline Dynamic Solutions
//
// region.go — universal US-state derivation from a free-text location context.
// Used to bias geocoding by administrative area and to qualify generic highway
// names (e.g. "TURNPIKE" → "OHIO TURNPIKE") without any per-department config.

package mapping

import "strings"

// usStateFull maps an upper-cased full state name to its canonical form, which
// is what Google's geocoder accepts in components=administrative_area:<state>.
var usStateFull = map[string]string{
	"ALABAMA": "Alabama", "ALASKA": "Alaska", "ARIZONA": "Arizona",
	"ARKANSAS": "Arkansas", "CALIFORNIA": "California", "COLORADO": "Colorado",
	"CONNECTICUT": "Connecticut", "DELAWARE": "Delaware",
	"DISTRICT OF COLUMBIA": "District of Columbia", "FLORIDA": "Florida",
	"GEORGIA": "Georgia", "HAWAII": "Hawaii", "IDAHO": "Idaho",
	"ILLINOIS": "Illinois", "INDIANA": "Indiana", "IOWA": "Iowa",
	"KANSAS": "Kansas", "KENTUCKY": "Kentucky", "LOUISIANA": "Louisiana",
	"MAINE": "Maine", "MARYLAND": "Maryland", "MASSACHUSETTS": "Massachusetts",
	"MICHIGAN": "Michigan", "MINNESOTA": "Minnesota", "MISSISSIPPI": "Mississippi",
	"MISSOURI": "Missouri", "MONTANA": "Montana", "NEBRASKA": "Nebraska",
	"NEVADA": "Nevada", "NEW HAMPSHIRE": "New Hampshire", "NEW JERSEY": "New Jersey",
	"NEW MEXICO": "New Mexico", "NEW YORK": "New York",
	"NORTH CAROLINA": "North Carolina", "NORTH DAKOTA": "North Dakota",
	"OHIO": "Ohio", "OKLAHOMA": "Oklahoma", "OREGON": "Oregon",
	"PENNSYLVANIA": "Pennsylvania", "RHODE ISLAND": "Rhode Island",
	"SOUTH CAROLINA": "South Carolina", "SOUTH DAKOTA": "South Dakota",
	"TENNESSEE": "Tennessee", "TEXAS": "Texas", "UTAH": "Utah",
	"VERMONT": "Vermont", "VIRGINIA": "Virginia", "WASHINGTON": "Washington",
	"WEST VIRGINIA": "West Virginia", "WISCONSIN": "Wisconsin", "WYOMING": "Wyoming",
}

// usStateAbbr maps a two-letter postal abbreviation to the canonical full name.
var usStateAbbr = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
	"DC": "District of Columbia", "FL": "Florida", "GA": "Georgia", "HI": "Hawaii",
	"ID": "Idaho", "IL": "Illinois", "IN": "Indiana", "IA": "Iowa", "KS": "Kansas",
	"KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland",
	"MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota", "MS": "Mississippi",
	"MO": "Missouri", "MT": "Montana", "NE": "Nebraska", "NV": "Nevada",
	"NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico", "NY": "New York",
	"NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio", "OK": "Oklahoma",
	"OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island", "SC": "South Carolina",
	"SD": "South Dakota", "TN": "Tennessee", "TX": "Texas", "UT": "Utah",
	"VT": "Vermont", "VA": "Virginia", "WA": "Washington", "WV": "West Virginia",
	"WI": "Wisconsin", "WY": "Wyoming",
}

// DeriveState returns the canonical US state name found in any of the supplied
// context strings (e.g. "Trumbull County, Ohio", "Cleveland, OH"), or "" when
// none is recognized. It prefers comma-separated segments (read right-to-left,
// since "City, State" puts the state last) and falls back to the trailing
// one or two tokens. Universal — no per-department data.
func DeriveState(contexts ...string) string {
	for _, c := range contexts {
		if s := stateFromContext(c); s != "" {
			return s
		}
	}
	return ""
}

func stateFromContext(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	if u == "" {
		return ""
	}
	// Comma-separated segments, last-first (state typically trails the city/county).
	segs := strings.Split(u, ",")
	for i := len(segs) - 1; i >= 0; i-- {
		seg := strings.TrimSpace(segs[i])
		if full, ok := usStateFull[seg]; ok {
			return full
		}
		if full, ok := usStateAbbr[seg]; ok {
			return full
		}
	}
	// Trailing one/two tokens (e.g. "... NEW YORK", "... OH").
	fields := strings.Fields(u)
	if n := len(fields); n >= 1 {
		if full, ok := usStateAbbr[fields[n-1]]; ok {
			return full
		}
		if full, ok := usStateFull[fields[n-1]]; ok {
			return full
		}
		if n >= 2 {
			if full, ok := usStateFull[fields[n-2]+" "+fields[n-1]]; ok {
				return full
			}
		}
	}
	return ""
}

// StateAbbrev returns the two-letter postal abbreviation for a US state name or
// abbreviation. Returns "" when unrecognized.
func StateAbbrev(state string) string {
	u := strings.ToUpper(strings.TrimSpace(state))
	if u == "" {
		return ""
	}
	if len(u) == 2 {
		if _, ok := usStateAbbr[u]; ok {
			return u
		}
	}
	if full, ok := usStateFull[u]; ok {
		for abbr, name := range usStateAbbr {
			if name == full {
				return abbr
			}
		}
	}
	for abbr, name := range usStateAbbr {
		if strings.EqualFold(name, state) {
			return abbr
		}
	}
	return ""
}
