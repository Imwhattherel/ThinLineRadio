package mapping

import (
	"fmt"
	"strings"
)

// GeocodeHit is a pinned address lookup plus the imported street name used.
type GeocodeHit struct {
	Lat    float64
	Lon    float64
	Source string
	Street string // display name from import; empty when unknown
	// House is set when the geocoder resolved the point using a house
	// number different from what was queried (e.g. the digit-doubled
	// last-resort retry, "9494" -> "94") so the caller can correct the
	// displayed address to match the pin instead of showing the literal,
	// STT-garbled number.
	House string
	OK    bool
}

// restoreDispatchHouseOnAddress re-attaches a house number when homonym or import
// street refinement left a street-only card label.
func restoreDispatchHouseOnAddress(curated *CuratedAlert, transcript string) {
	if curated == nil || strings.TrimSpace(transcript) == "" {
		return
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if house != "" || street == "" {
		return
	}
	if h := DispatchHouseFromTranscriptForStreet(transcript, street); h != "" {
		curated.Address = h + " " + street
	}
}

func ApplyGeocodedStreetToAddress(curated *CuratedAlert, resolvedStreet string) {
	if curated == nil {
		return
	}
	resolvedStreet = strings.ToUpper(strings.TrimSpace(resolvedStreet))
	if resolvedStreet == "" {
		return
	}
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if house != "" {
		curated.Address = house + " " + resolvedStreet
	} else {
		curated.Address = resolvedStreet
	}
}

// StreetNameFromGeocodedFormatted extracts a card-ready street from a geocoder
// display_name / matchedAddress ("157, Youngstown Hubbard Road, Hubbard, …"
// → "YOUNGSTOWN HUBBARD ROAD").
func StreetNameFromGeocodedFormatted(formatted string) string {
	fu := normalizeRouteTokens(stripLeadingHouseNumberComma(normalizeNominatimDisplayName(strings.ToUpper(strings.TrimSpace(formatted)))))
	core := formattedStreetCoreForPlausible(fu)
	if core == "" {
		return ""
	}
	if _, st := splitHouseAndStreet(core); st != "" {
		core = st
	}
	if canon := CanonicalStreetName(core); canon != "" {
		// Prefer a spelled-out type on the card when the geocoder gave one.
		if hasStreetSuffix(core) && !hasStreetSuffix(canon) {
			return strings.TrimSpace(core)
		}
		return canon
	}
	return strings.TrimSpace(core)
}

// ApplyGeocodedHouseToAddress replaces the house number in curated.Address
// with the one the geocoder actually resolved, used when a last-resort retry
// (e.g. an STT digit-doubled house number) succeeded with a different number
// than what was extracted.
func ApplyGeocodedHouseToAddress(curated *CuratedAlert, resolvedHouse string) {
	if curated == nil {
		return
	}
	resolvedHouse = strings.TrimSpace(resolvedHouse)
	if resolvedHouse == "" {
		return
	}
	_, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if street == "" {
		return
	}
	curated.Address = resolvedHouse + " " + street
}

// TranscriptContradictsAddressStreet reports when dispatch named a different
// thoroughfare for the same house than the extracted/geocoded card label.
func TranscriptContradictsAddressStreet(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || strings.TrimSpace(transcript) == "" {
		return false
	}
	cleaned := PreCleanTranscript(transcript)
	u := strings.ToUpper(cleaned)
	if route := dispatchRouteStreetAfterHouse(house, u); route != "" {
		// Same numbered state route ("STATE ROUTE 193" vs geocoded "SR 193 NE")
		// — trailing NE/NW is geocoder labeling, not a different road.
		if sameNumberedStateRoute(route, street) {
			return false
		}
		if _, ok := ohioStateRouteNumberInText(street); ok {
			return !strings.EqualFold(CanonicalStreetName(route), CanonicalStreetName(street))
		}
		// Spoken STATE ROUTE N but the card/import label is the local road
		// name OSM uses for that highway ("WILSON SHARPSVILLE ROAD" for
		// SR 305). That is not a different thoroughfare.
		return false
	}
	spoken := strings.TrimSpace(spokenStreetPhraseAfterHouse(u, house))
	if spoken == "" {
		return false
	}
	if strings.EqualFold(CanonicalStreetName(spoken), CanonicalStreetName(street)) {
		return false
	}
	if sameNumberedStateRoute(spoken, street) {
		return false
	}
	spokenCore := stripStreetSpaces(streetNameAndSuffixFirst(spoken))
	addrCore := stripStreetSpaces(streetNameAndSuffixFirst(street))
	if spokenCore != "" && spokenCore == addrCore {
		_, spokenSuf := streetNameAndSuffix(spoken)
		_, addrSuf := streetNameAndSuffix(street)
		// Same stem but incompatible types ("ALLISON DRIVE" vs "ALLISON AVE")
		// is a real contradiction — geocoder snapped to the wrong street.
		if spokenSuf != "" && addrSuf != "" && !streetSuffixesCompatible(spokenSuf, addrSuf) {
			return true
		}
		return false
	}
	if StreetNamesSTTMatch(spoken, street) || streetsAreCoverageHomonyms(spoken, street) {
		return false
	}
	// STT "HARVARD YOUNGSTOWN" for geocoded "YOUNGSTOWN HUBBARD" — confusable
	// tokens and/or swapped city-pair stem order.
	if streetsAreSTTConfusableEquivalent(spoken, street) {
		return false
	}
	return transcriptNamesHouseStreet(u, house, spoken)
}

// sameNumberedStateRoute reports when both phrases are the same Ohio/state
// route number (SR / STATE ROUTE / ST RTE), ignoring trailing quadrant labels.
func sameNumberedStateRoute(a, b string) bool {
	an, aok := ohioStateRouteNumberInText(a)
	bn, bok := ohioStateRouteNumberInText(b)
	return aok && bok && an == bn
}

// ClearPinWhenStreetContradictsTranscript is retired. Nominatim-gateway owns
// street identity on the transcript-geocode path; TLR must not clear pins by
// re-running STT street matching against the card. Kept as a no-op so older
// call sites/tests compile; do not re-enable. Prefer
// TranscriptContradictsAddressStreet only for diagnostics/eval.
//
// Ownership split:
//   - gateway: which house/street/pin (fuzzy STT adopt, addr-index, viewbox)
//   - TLR: coverage radius, mutual-aid policy, card finalize when gateway did
//     not name the street, known-place POIs
func ClearPinWhenStreetContradictsTranscript(curated *CuratedAlert, transcript string) {
	_ = curated
	_ = transcript
}

// FinalizeIncidentCardAddress restores house/suffix from the transcript when
// Nominatim did not already name the street on the card.
func FinalizeIncidentCardAddress(curated *CuratedAlert, transcript string, scope *ScopeData, geo *GeoOptions, geocodedStreetNamed bool) bool {
	if curated == nil || strings.TrimSpace(transcript) == "" || TranscriptIsPeerAgencySquadRequest(transcript) {
		return geocodedStreetNamed
	}
	named := geocodedStreetNamed
	restoreDispatchHouseOnAddress(curated, transcript)
	// When Nominatim already named the street — e.g. STT "HARVARD YOUNGSTOWN"
	// → "YOUNGSTOWN HUBBARD" — do not pull the garbled transcript form back.
	if !geocodedStreetNamed {
		if aligned := AlignAddressStreetFromScopedTranscript(curated.Address, transcript, scope); strings.TrimSpace(aligned) != "" &&
			!strings.EqualFold(strings.TrimSpace(aligned), strings.TrimSpace(curated.Address)) {
			curated.Address = aligned
			named = true
		}
		curated.Address = AlignAddressSuffixFromTranscript(curated.Address, transcript)
	}
	// Transcript alignment can reintroduce sentence punctuation from the raw
	// transcript ("10818 GOODING? AVE") or append a second thoroughfare suffix
	// ("1300 EAST STREET ST"); scrub both from the final card.
	curated.Address = strings.TrimSpace(strings.NewReplacer("?", "", "!", "").Replace(curated.Address))
	curated.Address = dedupeTrailingStreetSuffix(curated.Address)
	restoreSpokenStateRouteCardLabel(curated, transcript)
	return named
}

// restoreSpokenStateRouteCardLabel keeps "4525 SR 305" on the card when
// import/Nominatim rewrote the label to the local road name for that highway
// ("WILSON SHARPSVILLE RD") while dispatch spoke the state route.
func restoreSpokenStateRouteCardLabel(curated *CuratedAlert, transcript string) {
	if curated == nil {
		return
	}
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(curated.Address)))
	if house == "" || street == "" {
		return
	}
	if _, ok := ohioStateRouteNumberInText(street); ok {
		return
	}
	route := dispatchRouteStreetAfterHouse(house, strings.ToUpper(PreCleanTranscript(transcript)))
	if route == "" {
		return
	}
	n, ok := ohioStateRouteNumberInText(route)
	if !ok {
		return
	}
	curated.Address = house + " SR " + fmt.Sprintf("%d", n)
}
