// Copyright (C) 2025 Thinline Dynamic Solutions
//
// transcript_normalize.go — plain-text normalization applied as soon as an STT
// transcript is available (before keyword alerts, call-nature parsing, or
// geocoding). Strips punctuation/special characters, keeps letters and digits,
// inserts spaces at letter↔digit boundaries (except ordinal suffixes), and
// concatenates digit-digit hyphens (10-20 → 1020).

package mapping

import "strings"

// NormalizeTranscriptPlainText converts a transcript to uppercase plain text:
// letters, digits, and spaces only. Hyphens between digits are removed so the
// digits concatenate; all other punctuation/symbols become spaces. A space is
// inserted between a letter and a digit (either order), except for street
// ordinal suffixes so 88TH / 2ND / 3RD / 1ST stay glued. Whitespace is collapsed.
func NormalizeTranscriptPlainText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Preserve intersection semantics before & is stripped.
	s = strings.ReplaceAll(s, "&", " AND ")

	runes := []rune(strings.ToUpper(s))
	var b strings.Builder
	b.Grow(len(runes) + 8)

	const (
		kindNone   = 0
		kindLetter = 1
		kindDigit  = 2
	)
	prevKind := kindNone

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r >= 'A' && r <= 'Z':
			if prevKind == kindDigit {
				// Keep ordinal suffixes glued: 88TH, 2ND, 3RD, 1ST — do NOT
				// insert a space (that would turn 3RD into "3 RD" and look like
				// bare thoroughfare RD after a house number).
				if n := ordinalStreetSuffixLen(runes, i); n > 0 {
					for j := 0; j < n; j++ {
						b.WriteRune(runes[i+j])
					}
					i += n - 1
					prevKind = kindLetter
					continue
				}
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			prevKind = kindLetter
		case r >= '0' && r <= '9':
			if prevKind == kindLetter {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			prevKind = kindDigit
		case r == '-' || r == '–' || r == '—':
			// Digit-digit hyphen → concatenate ("10-20" → "1020").
			if i > 0 && i+1 < len(runes) {
				prev, next := runes[i-1], runes[i+1]
				if prev >= '0' && prev <= '9' && next >= '0' && next <= '9' {
					continue
				}
			}
			if prevKind != kindNone {
				b.WriteByte(' ')
				prevKind = kindNone
			}
		default:
			// Whitespace and all other punctuation/special chars → space.
			if prevKind != kindNone {
				b.WriteByte(' ')
				prevKind = kindNone
			}
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// ordinalStreetSuffixLen returns 2 when runes[i:] starts with ST/ND/RD/TH as a
// complete letter token (end or followed by a non-letter). Otherwise 0.
func ordinalStreetSuffixLen(runes []rune, i int) int {
	if i+1 >= len(runes) {
		return 0
	}
	a, b := runes[i], runes[i+1]
	if a < 'A' || a > 'Z' || b < 'A' || b > 'Z' {
		return 0
	}
	// Must be exactly two letters (not STREET, NORTH, …).
	if i+2 < len(runes) && runes[i+2] >= 'A' && runes[i+2] <= 'Z' {
		return 0
	}
	switch string([]rune{a, b}) {
	case "ST", "ND", "RD", "TH":
		return 2
	default:
		return 0
	}
}
