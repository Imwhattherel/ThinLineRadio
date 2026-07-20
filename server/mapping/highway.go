// Copyright (C) 2025 Thinline Dynamic Solutions
//
// highway.go — universal handling for limited-access highway references
// (turnpikes, interstates, freeways). Dispatch transcripts describe these as
// directional references ("turnpike eastbound", "271 northbound") that are not
// addressable points; geocoding them verbatim drops a confident but wrong pin.
// We strip the direction word, normalize the route, and (state-aware) qualify a
// bare "TURNPIKE" so it resolves to the real toll road. Anchoring to a named
// cross street is handled by the caller (process.go).

package mapping

import (
	"regexp"
	"strings"
)

// highwayDirectionRE matches travel-direction words that pollute geocoding:
// spelled-out "EASTBOUND"/"EAST BOUND" and the abbreviations EB/WB/NB/SB.
var highwayDirectionRE = regexp.MustCompile(`(?i)\b(?:EAST|WEST|NORTH|SOUTH)\s?BOUND\b|\b(?:EB|WB|NB|SB)\b`)

// interstateRE matches interstate references like "I-80", "I 80", or
// "INTERSTATE 80" (1–3 digits).
var interstateRE = regexp.MustCompile(`(?i)\b(?:I[\s-]?|INTERSTATE\s+)(\d{1,3})\b`)

// turnpikeWordRE matches a bare TURNPIKE token for state qualification.
var turnpikeWordRE = regexp.MustCompile(`(?i)\bTURNPIKE\b`)

// limitedAccessTokens are highway classes that are not addressable by a single
// point — they need a cross street / milepost to be located.
var limitedAccessTokens = []string{
	"TURNPIKE", "TOLL ROAD", "TOLLWAY", "THRUWAY", "FREEWAY", "EXPRESSWAY", "INTERSTATE",
}

// IsLimitedAccessHighwayAddress reports whether an address is a limited-access
// highway reference (turnpike, interstate, freeway, etc.).
func IsLimitedAccessHighwayAddress(addr string) bool {
	u := " " + strings.ToUpper(strings.TrimSpace(addr)) + " "
	if u == "  " {
		return false
	}
	for _, t := range limitedAccessTokens {
		if strings.Contains(u, " "+t+" ") {
			return true
		}
	}
	return interstateRE.MatchString(addr)
}

// stripHighwayDirectionals removes EASTBOUND/WB/etc. and collapses whitespace.
func stripHighwayDirectionals(addr string) string {
	out := highwayDirectionRE.ReplaceAllString(addr, " ")
	return strings.Join(strings.Fields(out), " ")
}

// qualifyTurnpikeWithState rewrites a bare "TURNPIKE" to "<STATE> TURNPIKE"
// (e.g. "BAR RD & TURNPIKE" → "BAR RD & OHIO TURNPIKE") so Google resolves the
// actual toll road. No-op when the state is unknown or already present.
func qualifyTurnpikeWithState(addr, state string) string {
	if strings.TrimSpace(state) == "" {
		return addr
	}
	su := strings.ToUpper(strings.TrimSpace(state))
	if strings.Contains(strings.ToUpper(addr), su) {
		return addr // already qualified (e.g. "OHIO TURNPIKE")
	}
	if !turnpikeWordRE.MatchString(addr) {
		return addr
	}
	return turnpikeWordRE.ReplaceAllString(addr, su+" TURNPIKE")
}

// normalizeHighwayAddress cleans a limited-access highway reference for
// geocoding and reports whether the address was such a reference. When it is:
//   - directional words are stripped ("TURNPIKE EASTBOUND" → "TURNPIKE"),
//   - interstates are normalized ("INTERSTATE 80"/"I 80" → "I-80"),
//   - a bare turnpike is qualified with the assigned state.
//
// Returns the (possibly unchanged) address and isHighway=false for normal
// street addresses so callers can skip the highway-specific logic.
func normalizeHighwayAddress(addr, state string) (string, bool) {
	if strings.TrimSpace(addr) == "" || !IsLimitedAccessHighwayAddress(addr) {
		return addr, false
	}
	out := stripHighwayDirectionals(addr)
	out = interstateRE.ReplaceAllString(out, "I-$1")
	out = qualifyTurnpikeWithState(out, state)
	out = strings.Join(strings.Fields(out), " ")
	return strings.TrimSpace(out), true
}
