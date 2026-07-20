// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_license_plate.go — reject temp tags and plate readouts mis-extracted
// as addresses (e.g. "VIRGINIA TEMTAG 42824-GEORGE" → "42824 GEORGE").

package mapping

import (
	"regexp"
	"strings"
)

var (
	tempTagMarkerRE = regexp.MustCompile(`(?i)\b(?:TEMP\s*TAG|TEMPTAG|TEMTAG|TEM\s*TAG|TEMPORARY\s+TAG|TEMPORARY\s+PLATE)\b`)
	// plateHyphenTokenRE matches Ohio/Virginia-style temp tags: 42824-GEORGE.
	plateHyphenTokenRE = regexp.MustCompile(`(?i)\b(\d{3,7})-(ADAM|BOY|CHARLIE|DAVID|EDWARD|FRANK|GEORGE|HENRY|IDA|JOHN|KING|LINCOLN|MARY|NAN|OCEAN|PAUL|QUEEN|ROBERT|SAM|TOM|UNION|VICTOR|WILLIAM|X-?RAY|XRAY|YOUNG|ZEBRA)\b`)
	driversLicenseMarkerRE = regexp.MustCompile(`(?i)\b(?:DRIVER'?S?\s+LICENSE|OPERATOR'?S?\s+LICENSE)\b`)
	// hyphenatedDLNumberRE matches state DL formats like 4200-469-013-696.
	hyphenatedDLNumberRE = regexp.MustCompile(`\b(\d{3,4})-(\d{3})-(\d{3})-(\d{2,4})\b`)
)

// platePhoneticSuffixes are NATO/telephony words used as temp-tag suffixes.
var platePhoneticSuffixes = map[string]bool{
	"ADAM": true, "BOY": true, "CHARLIE": true, "DAVID": true, "EDWARD": true,
	"FRANK": true, "GEORGE": true, "HENRY": true, "IDA": true, "JOHN": true,
	"KING": true, "LINCOLN": true, "MARY": true, "NAN": true, "OCEAN": true,
	"PAUL": true, "QUEEN": true, "ROBERT": true, "SAM": true, "TOM": true,
	"UNION": true, "VICTOR": true, "WILLIAM": true, "XRAY": true, "X-RAY": true,
	"YOUNG": true, "ZEBRA": true,
}

// TranscriptIsLicensePlateReadout reports traffic-stop plate/tag readouts and
// BMV lookup chatter — not toned dispatches with a street address.
func TranscriptIsLicensePlateReadout(transcript string) bool {
	if TranscriptIsPlateOrLookupChatter(transcript) {
		return true
	}
	u := " " + strings.ToUpper(transcript) + " "
	if tempTagMarkerRE.MatchString(u) {
		return true
	}
	if driversLicenseMarkerRE.MatchString(u) {
		return true
	}
	for _, m := range []string{
		" LICENSE PLATE ", " LIC PLATE ", " PLATE NUMBER ", " PLATE IS ",
		" RUN THE PLATE ", " RUNNING PLATE ", " RUN A PLATE ", " RUNNING A ",
		" TAG NUMBER ", " RUNNING THE TAG ", " CHECK THE PLATE ", " RAN THE PLATE ",
		" OUT OF STATE TAG ", " OUT OF STATE PLATE ", " RUNNING TAG ",
		" RUN THAT TAG ", " RUN THAT PLATE ", " PLATE COMES BACK ",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	// State name immediately before a temp-tag marker (e.g. "VIRGINIA TEMTAG 42824").
	if stateTempTagRE.MatchString(u) {
		return true
	}
	return plateHyphenTokenRE.MatchString(u) || transcriptHasPhoneticPlateReadout(transcript)
}

var stateTempTagRE = regexp.MustCompile(`(?i)\b(?:ALABAMA|ALASKA|ARIZONA|ARKANSAS|CALIFORNIA|COLORADO|CONNECTICUT|DELAWARE|FLORIDA|GEORGIA|HAWAII|IDAHO|ILLINOIS|INDIANA|IOWA|KANSAS|KENTUCKY|LOUISIANA|MAINE|MARYLAND|MASSACHUSETTS|MICHIGAN|MINNESOTA|MISSISSIPPI|MISSOURI|MONTANA|NEBRASKA|NEVADA|(?:NEW\s+HAMPSHIRE|NEW\s+JERSEY|NEW\s+MEXICO|NEW\s+YORK)|NORTH\s+CAROLINA|NORTH\s+DAKOTA|OHIO|OKLAHOMA|OREGON|PENNSYLVANIA|RHODE\s+ISLAND|SOUTH\s+CAROLINA|SOUTH\s+DAKOTA|TENNESSEE|TEXAS|UTAH|VERMONT|VIRGINIA|WASHINGTON|WEST\s+VIRGINIA|WISCONSIN|WYOMING)\s+(?:TEMP\s*TAG|TEMPTAG|TEMTAG|TEM\s*TAG|TEMPORARY\s+TAG|TEMPORARY\s+PLATE)\b`)

// AddressIsLicensePlate reports when an extracted address is really a license
// plate or temp tag spoken on traffic-stop traffic.
func AddressIsLicensePlate(addr, transcript string) bool {
	house, street := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || street == "" || strings.Contains(street, "&") {
		return false
	}
	fields := strings.Fields(street)
	if len(fields) != 1 || hasStreetSuffix(street) || localRouteKeywords[fields[0]] {
		return false
	}
	suffix := fields[0]
	u := strings.ToUpper(transcript)

	// Exact hyphenated plate token in the transcript (42824-GEORGE).
	if plateTokenInTranscript(u, house, suffix) {
		return true
	}

	if !TranscriptIsLicensePlateReadout(transcript) {
		return false
	}

	// During a plate readout, a 4–7 digit "house" plus a single phonetic/name
	// token without a street suffix is almost never a dispatch address.
	if len(house) >= 4 && len(house) <= 7 && platePhoneticSuffixes[suffix] {
		return true
	}
	if len(house) >= 4 && len(house) <= 7 && commonGivenNames[suffix] {
		return true
	}
	return false
}

func plateTokenInTranscript(transcript, house, suffix string) bool {
	if plateHyphenTokenRE.MatchString(transcript) {
		for _, m := range plateHyphenTokenRE.FindAllStringSubmatch(transcript, -1) {
			if len(m) >= 3 && m[1] == house && strings.EqualFold(m[2], suffix) {
				return true
			}
		}
	}
	// Space-separated variant when STT drops the hyphen.
	spaced := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(house) + `\s+` + regexp.QuoteMeta(suffix) + `\b`)
	return spaced.MatchString(transcript) && (tempTagMarkerRE.MatchString(transcript) || platePhoneticSuffixes[suffix])
}

// AddressHouseIsLicenseNumberFragment reports when the extracted house number is
// the trailing segment of a spoken driver's-license number (4200-469-013-696 →
// 696) rather than a dispatch street number.
func AddressHouseIsLicenseNumberFragment(addr, transcript string) bool {
	if !TranscriptIsLicensePlateReadout(transcript) {
		return false
	}
	house, _ := splitHouseAndStreet(strings.ToUpper(strings.TrimSpace(addr)))
	if house == "" || len(house) < 2 || len(house) > 4 {
		return false
	}
	u := strings.ToUpper(transcript)
	for _, m := range hyphenatedDLNumberRE.FindAllStringSubmatch(u, -1) {
		if len(m) >= 5 && m[4] == house {
			return true
		}
	}
	return false
}

// AddressQualifiesForAutoLearn reports whether a geocoded address is safe to
// persist as a known place. Plate/unit identifiers must never be auto-learned.
func AddressQualifiesForAutoLearn(addr, transcript string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	return !AddressStructurallyImplausible(addr) &&
		!AddressIsLicensePlate(addr, transcript) &&
		!AddressIsUnitPersonIdentifier(addr, transcript) &&
		!AddressIsMalformedPlateOrNarrative(addr, transcript) &&
		!AddressIsUnitNavigationChoice(addr, transcript) &&
		!AddressIsUnitStandDownOrCancel(addr, transcript)
}
