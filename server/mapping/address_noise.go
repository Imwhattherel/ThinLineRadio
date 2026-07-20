// Copyright (C) 2025 Thinline Dynamic Solutions
//
// address_noise.go — reject extracted "addresses" that are actually radio
// chatter. Speech-to-text + the LLM occasionally latch onto conversational
// speech and emit a bogus intersection, e.g. "you can show myself and 32 en
// route to George Ford" → address "YOU CAN SHOW MYSELF & SR 32". Reflexive /
// second-person pronouns and en-route phrases never appear in US street names,
// so their presence is a reliable, universal signal that the extraction is not
// a real location.

package mapping

import (
	"regexp"
	"strings"
)

// addressReflexivePronounRE matches reflexive pronouns, which never occur in
// street names.
var addressReflexivePronounRE = regexp.MustCompile(`(?i)\b(?:MYSELF|YOURSELF|HIMSELF|HERSELF|OURSELVES|THEMSELVES|YOURSELVES)\b`)

// addressSecondPersonRE matches second-person pronouns ("YOU"/"YOUR"). "US" and
// "I" are deliberately excluded — they collide with "US 422" / "I-80" routes.
var addressSecondPersonRE = regexp.MustCompile(`(?i)\b(?:YOU|YOUR|YOURS)\b`)

// addressChatterPhraseRE matches dispatch/radio procedure phrases. Word
// boundaries keep "STATE ROUTE", "MARTIN ROUTE", etc. from matching.
var addressChatterPhraseRE = regexp.MustCompile(`(?i)\b(?:SHOW ME|SHOW MYSELF|SHOW US|EN\s?ROUTE|IN ROUTE|RESPONDING|BE ADVISED|DISREGARD|STAND BY|STANDBY)\b`)

// AddressIsConversationalNoise reports whether an extracted address is radio
// chatter rather than a real location. Universal — pattern-only, no
// per-department data.
func AddressIsConversationalNoise(addr string) bool {
	u := strings.ToUpper(strings.TrimSpace(addr))
	if u == "" {
		return false
	}
	return addressReflexivePronounRE.MatchString(u) ||
		addressSecondPersonRE.MatchString(u) ||
		addressChatterPhraseRE.MatchString(u)
}

// officerMovementRE matches a first-person statement of intent to travel
// somewhere ("I'll head over to…", "I'm going to…"). A unit narrating its own
// movement is not a dispatch to an incident, so a street named only in this
// context should not be mapped.
var officerMovementRE = regexp.MustCompile(`(?i)\bI(?:'?LL|'?M| WILL| AM|'VE|'D)?\s+(?:HEAD|HEADING|GO|GOING|GONNA|SWING|DRIVE|RUN|RUNNING|STOP|STOPPING|BE|WENT)\b`)

// officerClosurePhrases mark scene-closure / status-check chatter that never
// accompanies a real dispatched incident.
var officerClosurePhrases = []string{
	"CHECKS OKAY", "CHECK'S OKAY", "CHECKS OUT", "CHECK'S OUT",
	"CHECKS GOOD", "CHECKS CLEAR", "ALL CLEAR", "CODE 4", "CODE FOUR",
}

// TranscriptIsOfficerNarrative reports whether a transcript is a unit's own
// conversational narrative (self-movement or scene-closure) rather than a CAD
// dispatch. Used together with a "no house number" guard so a street the
// officer merely says they're heading to (e.g. "I'll head over to Porter Road")
// is not pinned as an incident. Universal — pattern-only.
func TranscriptIsOfficerNarrative(transcript string) bool {
	u := strings.ToUpper(transcript)
	if officerMovementRE.MatchString(u) {
		return true
	}
	for _, p := range officerClosurePhrases {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}
