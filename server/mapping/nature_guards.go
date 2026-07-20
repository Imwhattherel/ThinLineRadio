// Copyright (C) 2025 Thinline Dynamic Solutions
//
// nature_guards.go — incident nature validation, generic-label rejection, and
// optional normalization to configured keyword-list terms.

package mapping

import (
	"regexp"
	"sort"
	"strings"
)

var (
	fallingObjectRE       = regexp.MustCompile(`(?i)\bFALLING\s+(HER|HIS|THE|THEIR|A|AN|THIS|MY|YOUR|OUR)\b`)
	fallIncidentContextRE = regexp.MustCompile(`(?i)\b(FALL\s+PATIENT|FALL\s+VICTIM|FOR\s+A\s+FALL|FOR\s+FALL|FELL\s+DOWN|HAS\s+FALLEN|HAVE\s+FALLEN|FROM\s+A\s+FALL|LIFT\s+ASSIST|ON\s+THE\s+FLOOR|UNABLE\s+TO\s+GET\s+UP|CAN'?T\s+GET\s+UP|STILL\s+DOWN|INJURED\s+IN\s+FALL|SLIPPED\s+AND\s+FELL)\b`)
	dispatchDemographicLeadRE = regexp.MustCompile(`(?i)^(?:\d{1,3}[- ]?YEAR[- ]?OLD\s+)?(?:MALE|FEMALE)\s+`)
	// House number + street name ending in COURT ("150 CHARLES COURT").
	streetAddressCourtRE = regexp.MustCompile(`(?i)\b\d{1,6}(?:[\s\-]+\d{1,4})?(?:[\s,\-]+[A-Z0-9][A-Z0-9'\-]*){0,6}\s+COURT\b`)
)

// genericNatureLabels are useless catch-alls the LLM must not emit.
var genericNatureLabels = map[string]bool{
	"ALERT": true, "UNKNOWN": true, "EMERGENCY": true, "GENERAL": true,
	"INCIDENT": true, "SITUATION": true, "N/A": true, "NA": true, "NONE": true,
	"UNCLASSIFIED": true, "OTHER": true, "MISC": true, "RADIO": true,
	"MULTI-INCIDENT": true, "MULTI INCIDENT": true,
	"ON-SITE": true, "ON SITE": true,
	"ALL CLEAR": true, "DISPATCH CLEAR": true,
}

// apparatusOnlyNatureLabels are unit/apparatus tokens that sometimes appear in
// keyword lists but must never be the incident nature on a map pin.
var apparatusOnlyNatureLabels = map[string]bool{
	"LADDER": true, "ENGINE": true, "MEDIC": true, "SQUAD": true, "TRUCK": true,
	"BATTALION": true, "RESCUE": true, "UNIT": true, "CHANNEL": true, "CAR": true,
	"EMS": true, "ALS": true, "BLS": true, "PATIENT": true, "MEDFOR": true,
	"CHIEF": true, "CAPTAIN": true, "TANKER": true, "BRUSH": true, "QUINT": true,
}

var (
	ilpersonNatureRE         = regexp.MustCompile(`(?i)\bIL\s*PERSON\b`)
	weightLimitNatureRE      = regexp.MustCompile(`(?i)^OVER\s*\d+\s*LIMIT$`)
	apparatusDispatchPhraseRE = regexp.MustCompile(`(?i)^(MEDIC|ENGINE|SQUAD|RESCUE|LADDER|TRUCK|UNIT|BATTALION|CHIEF|CAPTAIN)\s+(RUN|CALL|DISPATCH)$`)
	policeStatusNatureRE     = regexp.MustCompile(`(?i)^(10[- ]?\d{1,2}|CODE\s+(CLEAR|BLUE|VIOLET|4)|CLEAR\s+CODE|TROUBLE\s+UNKNOWN|TRAFFIC\s+STOP|STAND\s+BY|NOTICE|ADVICE|TOW|READ|RELEASE|MOTION|HANGOUT|HANG\s+UP)$`)
)

func isApparatusOnlyNatureLabel(nature string) bool {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if apparatusOnlyNatureLabels[n] {
		return true
	}
	return apparatusDispatchPhraseRE.MatchString(n)
}

const maxNatureLabelLen = 22

func isExactNatureCode(nature string, natureCodes []string) bool {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" {
		return false
	}
	for _, code := range natureCodes {
		if strings.ToUpper(strings.TrimSpace(code)) == n {
			return true
		}
	}
	return false
}

// IsGenericNature reports catch-all nature labels that carry no dispatch meaning.
func IsGenericNature(nature string) bool {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" {
		return false
	}
	if genericNatureLabels[n] {
		return true
	}
	if isApparatusOnlyNatureLabel(n) {
		return true
	}
	if weightLimitNatureRE.MatchString(n) {
		return true
	}
	if policeStatusNatureRE.MatchString(n) {
		return true
	}
	// "GENERAL ALERT", "MEDICAL EMERGENCY" with no specifics — keep MEDICAL EMERGENCY?
	if n == "MEDICAL EMERGENCY" || n == "POLICE EMERGENCY" {
		return true
	}
	return false
}

// ScrubIncidentNature corrects false-positive nature labels inferred from STT
// homophones and verb forms that are not medical fall dispatches.
func ScrubIncidentNature(nature, transcript string) string {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" || IsGenericNature(n) {
		return ""
	}
	u := strings.ToUpper(transcript)
	switch n {
	case "FALL", "FELL", "FALLEN":
		if IsFallNatureFalsePositive(u) {
			return ""
		}
	case "LAKE EMERGENCY":
		if natureKeywordIsLakeDepartmentFalsePositive(" "+u+" ", n) {
			return ""
		}
		if inferWiresTreesDownNature(transcript, nil) != "" {
			return ""
		}
	}
	return n
}

// clampNatureLabel shortens LLM-invented phrases to configured keywords or the
// first few words that fit the display limit. Exact configured call-nature
// labels are preserved at full length for call-card display.
func clampNatureLabel(nature string, natureCodes []string) string {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" {
		return n
	}
	if isExactNatureCode(n, natureCodes) {
		return n
	}
	if len(n) <= maxNatureLabelLen {
		return n
	}
	flat := strings.Join(strings.Fields(strings.NewReplacer(",", " ", ".", " ", ";", " ").Replace(n)), " ")
	if norm := NormalizeNatureToKeywords(flat, natureCodes); norm != "" {
		if len(norm) <= maxNatureLabelLen || isExactNatureCode(norm, natureCodes) {
			return norm
		}
	}
	fields := strings.Fields(n)
	for len(fields) > 1 && len(strings.Join(fields, " ")) > maxNatureLabelLen {
		fields = fields[:len(fields)-1]
	}
	out := strings.Join(fields, " ")
	if len(out) > maxNatureLabelLen && len(fields) > 0 {
		return fields[0]
	}
	return out
}

var natureAnchorStopwords = map[string]bool{
	"POSSIBLE": true, "PROBABLE": true, "SUSPECTED": true, "REPORTED": true,
	"WITH": true, "THE": true, "AND": true, "FOR": true, "A": true, "AN": true,
	"OF": true, "TO": true, "ON": true, "AT": true, "IN": true, "OR": true,
}

// natureAnchoredInTranscript rejects invented or over-broad nature labels that
// do not appear in what dispatch actually said.
func natureAnchoredInTranscript(nature, transcript string) bool {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" {
		return true
	}
	u := " " + strings.ToUpper(transcript) + " "
	switch n {
	case "CRASH":
		return strings.Contains(u, " CRASH ") || strings.Contains(u, " MVA ") ||
			strings.Contains(u, " ACCIDENT ") || strings.Contains(u, " COLLISION ") ||
			strings.Contains(u, " WRECK ") || strings.Contains(u, " CRASH WITH ")
	case "PATIENT":
		return strings.Contains(u, " PATIENT ") && !strings.Contains(u, " FALL PATIENT ")
	case "MEDICAL":
		return strings.Contains(u, " MEDICAL ") || strings.Contains(u, " EMS ") ||
			strings.Contains(u, " MEDIC ") || strings.Contains(u, " AMBULANCE ")
	case "CRITICAL":
		return strings.Contains(u, " CRITICAL ")
	case "FIRE":
		return strings.Contains(u, " FIRE ") && !strings.Contains(u, " FIREARM ") &&
			!natureKeywordIsNegated(u, "FIRE")
	case "COURT", "CT":
		if courtNatureExplicitlySpoken(u) {
			return true
		}
		return false
	case "VICTIM":
		return strings.Contains(u, " VICTIM ")
	case "INJURY":
		return strings.Contains(u, " INJURY ") || strings.Contains(u, " INJURED ")
	case "ACCIDENT":
		return strings.Contains(u, " ACCIDENT ") || strings.Contains(u, " AUTO ACCIDENT") ||
			strings.Contains(u, " MVA ") || strings.Contains(u, " CRASH ")
	case "SMOKE":
		return (strings.Contains(u, " SMOKE SHOWING ") || strings.Contains(u, " SMOKE ")) &&
			!strings.Contains(u, " FIREARM ") && !natureKeywordIsNegated(u, "SMOKE")
	case "CHASE":
		if strings.Contains(u, " CHASE TRAIL") || strings.Contains(u, " TO VIEW") ||
			strings.Contains(u, " MARK ME IN ROUTE ") {
			return false
		}
		return strings.Contains(u, " CHASE ") || strings.Contains(u, " PURSUIT ")
	case "WIRES/TREES DOWN":
		return transcriptDescribesWiresTreesDown(transcript)
	}
	if strings.Contains(n, "SUICID") || strings.Contains(n, "MENTAL") {
		return transcriptDescribesSuicideRisk(u)
	}
	if wholeWordContains(u, n) || transcriptContainsNatureKeyword(u, n) {
		return true
	}
	matched, required := 0, 0
	for _, w := range strings.Fields(n) {
		w = strings.TrimRight(w, ".,;/")
		if len(w) < 3 || natureAnchorStopwords[w] {
			continue
		}
		required++
		if strings.Contains(u, " "+w+" ") || strings.Contains(u, " "+w+"'") {
			matched++
		}
	}
	if required == 0 {
		return false
	}
	return matched >= 1
}

// IsFallNatureFalsePositive reports whether "fall" nature was inferred from
// conversational "falling [pronoun] …" (usually STT for "following") rather than
// a patient-down dispatch.
func IsFallNatureFalsePositive(transcriptUpper string) bool {
	u := strings.ToUpper(transcriptUpper)
	if !strings.Contains(u, "FALLING") && !strings.Contains(u, " FALL ") {
		return false
	}
	if fallIncidentContextRE.MatchString(u) {
		return false
	}
	if fallingObjectRE.MatchString(u) {
		return true
	}
	if strings.Contains(u, "FALLING") {
		return true
	}
	return false
}

// wholeWordContains reports whether term appears in paddedUpper bounded by
// spaces (paddedUpper must already include leading/trailing spaces).
func wholeWordContains(paddedUpper, term string) bool {
	term = strings.ToUpper(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	if strings.Contains(paddedUpper, " "+term+" ") {
		return true
	}
	needle := " " + term
	for _, end := range []string{",", ".", ";", ":"} {
		if strings.Contains(paddedUpper, needle+end) {
			return true
		}
	}
	return false
}

// fdRequestingPDNature labels inter-agency requests where fire/EMS tones police
// on the radio ("THE FD IS REQUESTING PD ON THE RAD") rather than a PD squad tone.
func fdRequestingPDNature(paddedUpper string) string {
	if strings.Contains(paddedUpper, " REQUESTING PD ") ||
		strings.Contains(paddedUpper, " REQUESTING POLICE ") ||
		strings.Contains(paddedUpper, " IS REQUESTING PD ") ||
		strings.Contains(paddedUpper, " IS REQUESTING POLICE ") {
		return "FD ASSIST"
	}
	return ""
}

// inferCardiacSymptomNature maps tingling/pressure cardiac symptoms to chest pain.
func inferCardiacSymptomNature(transcript string, natureCodes []string) string {
	u := " " + strings.ToUpper(transcript) + " "
	if strings.Contains(u, " CHEST TINGLING") || strings.Contains(u, " CHEST PAIN") ||
		strings.Contains(u, " CHEST PAINS") || strings.Contains(u, " CHEST PRESSURE") ||
		(strings.Contains(u, " CHEST ") && strings.Contains(u, " TINGLING")) ||
		(strings.Contains(u, " LEFT ARM ") && strings.Contains(u, " TINGLING")) {
		if norm := NormalizeNatureToKeywords("CHEST PAIN", natureCodes); norm != "" {
			return norm
		}
		return "CHEST PAIN"
	}
	return ""
}

// inferSeizureActivityNature prefers seizure wording over trailing consciousness vitals.
func inferSeizureActivityNature(transcript string, natureCodes []string) string {
	u := strings.ToUpper(transcript)
	if !strings.Contains(u, "SEIZURE") {
		return ""
	}
	if norm := NormalizeNatureToKeywords("SEIZURES", natureCodes); norm != "" {
		return norm
	}
	for _, pref := range []string{"SEIZURE-LIKE ACTIVITY", "SEIZURE LIKE ACTIVITY", "SEIZURE"} {
		if strings.Contains(u, pref) {
			if norm := NormalizeNatureToKeywords(pref, natureCodes); norm != "" {
				return norm
			}
		}
	}
	return "SEIZURES"
}

func isLawEnforcementCrimeNature(n string) bool {
	ku := strings.ToUpper(strings.TrimSpace(n))
	for _, crime := range []string{
		"COUNTERFEIT", "FORGERY", "FRAUD", "ROBBERY", "BURGLARY", "THEFT", "ASSAULT",
		"DOMESTIC", "PURSUIT", "WARRANT", "SHOTS FIRED", "STABBING",
	} {
		if strings.Contains(ku, crime) {
			return true
		}
	}
	return false
}

// natureKeywordIsLakeDepartmentFalsePositive reports when a lake-emergency phrase
// matched only inside a fire-department name ("Geneva on the Lake Fire").
func natureKeywordIsLakeDepartmentFalsePositive(paddedUpper, keyword string) bool {
	ku := strings.ToUpper(strings.TrimSpace(keyword))
	if ku != "LAKE EMERGENCY" && ku != "LAKE RESCUE" && ku != "BOAT EMERGENCY" && ku != "ON THE LAKE" {
		return false
	}
	for _, cue := range []string{
		" ON THE LAKE FIRE", " TO YOU ON THE LAKE FIRE", " ATTENTION TO YOU ON THE LAKE FIRE",
		" LAKE FIRE,", " LAKE FIRE ", " LAKE FIRE DEPARTMENT", " LAKE FD ",
	} {
		if strings.Contains(paddedUpper, cue) {
			return true
		}
	}
	return false
}

// keyword matched only inside "BREAK ROOM" facility phrasing.
func natureKeywordIsBreakRoomFalsePositive(paddedUpper, keyword string) bool {
	ku := strings.ToUpper(strings.TrimSpace(keyword))
	if ku != "BREAK" && ku != "WATER BREAK" && !strings.HasSuffix(ku, " BREAK") {
		return false
	}
	return strings.Contains(paddedUpper, " BREAK ROOM") ||
		strings.Contains(paddedUpper, " IN THE BREAK ROOM")
}

func natureKeywordBlockedInMedicalDispatch(paddedUpper, keyword string) bool {
	ku := strings.ToUpper(strings.TrimSpace(keyword))
	if ku == "" {
		return false
	}
	if !strings.Contains(paddedUpper, " CHEST ") && !strings.Contains(paddedUpper, " TINGLING") &&
		!strings.Contains(paddedUpper, " SHORTNESS OF BREATH") {
		return false
	}
	for _, crime := range []string{
		"COUNTERFEIT", "FORGERY", "FRAUD", "ROBBERY", "BURGLARY", "THEFT", "ASSAULT",
		"DOMESTIC", "PURSUIT", "WARRANT", "SHOTS FIRED", "STABBING",
	} {
		if strings.Contains(ku, crime) {
			return true
		}
	}
	return false
}

// transcriptContainsNatureKeyword reports whether a configured keyword appears
// in the transcript literally or as a morphological variant (DEHYDRATED ↔
// DEHYDRATION). Multi-word keywords still require an exact phrase match.
func transcriptContainsNatureKeyword(paddedUpper, keyword string) bool {
	kw := strings.ToUpper(strings.TrimSpace(keyword))
	if kw == "" {
		return false
	}
	if wholeWordContains(paddedUpper, kw) {
		return true
	}
	if strings.Contains(kw, " ") {
		return false
	}
	for _, tok := range strings.Fields(strings.Trim(paddedUpper, " ")) {
		tok = strings.Trim(tok, ".,;:!?'\"")
		if natureWordStemMatch(tok, kw) {
			return true
		}
	}
	return false
}

func transcriptNatureKeywordIndex(paddedUpper, keyword string) int {
	kw := strings.ToUpper(strings.TrimSpace(keyword))
	if kw == "" {
		return -1
	}
	if idx := strings.Index(paddedUpper, " "+kw+" "); idx >= 0 {
		return idx
	}
	if idx := strings.Index(paddedUpper, " "+kw+","); idx >= 0 {
		return idx
	}
	if strings.Contains(kw, " ") {
		return -1
	}
	best := -1
	for _, tok := range strings.Fields(strings.Trim(paddedUpper, " ")) {
		raw := tok
		tok = strings.Trim(tok, ".,;:!?'\"")
		if !natureWordStemMatch(tok, kw) {
			continue
		}
		if idx := strings.Index(paddedUpper, " "+raw); idx >= 0 {
			if best < 0 || idx < best {
				best = idx
			}
		}
	}
	return best
}

// natureWordStemMatch reports whether a transcript token and a configured nature
// keyword share the same medical word stem (DEHYDRATED/DEHYDRATION, INJURED/INJURY).
func natureWordStemMatch(transcriptTok, keyword string) bool {
	a := strings.ToUpper(strings.TrimSpace(transcriptTok))
	b := strings.ToUpper(strings.TrimSpace(keyword))
	if a == b {
		return true
	}
	if len(a) < 5 || len(b) < 5 {
		return false
	}
	longerTok, shorterTok := a, b
	if len(b) > len(a) {
		longerTok, shorterTok = b, a
	}
	if strings.HasPrefix(longerTok, shorterTok) && len(longerTok) > len(shorterTok) &&
		!natureMorphologicalSuffix(longerTok[len(shorterTok):]) {
		return false // COURTESY must not match COURT
	}
	stem := 0
	for stem < len(a) && stem < len(b) && a[stem] == b[stem] {
		stem++
	}
	if stem < 5 {
		return false
	}
	longer := len(a)
	if len(b) > longer {
		longer = len(b)
	}
	// Reject prefix-only overlap (BLACK in BLACK HAWK → BLACKMAIL).
	if longer-stem > 3 {
		return false
	}
	shorter := len(a)
	if len(b) < shorter {
		shorter = len(b)
	}
	// An "-LY" adverb is a different word class than a plural/base noun form:
	// DISORDERLY vs DISORDERS ("seizure disorders" is not "disorderly"),
	// COSTLY vs COSTS. Never treat them as the same stem.
	tailA := a[stem:]
	tailB := b[stem:]
	if (tailA == "LY") != (tailB == "LY") {
		return false
	}
	// Allow short grammatical suffix differences (-ED/-ION/-ING/-NESS).
	return stem >= shorter-3
}

func natureMorphologicalSuffix(suffix string) bool {
	suffix = strings.ToUpper(strings.TrimSpace(suffix))
	if suffix == "" {
		return true
	}
	for _, suf := range []string{
		"NESS", "ING", "ION", "TION", "SION", "ED", "S", "LY", "AL", "ITY",
		"IVE", "OUS", "MENT", "ABLE", "IBLE", "ER", "OR", "IST", "ISM",
	} {
		if suffix == suf {
			return true
		}
	}
	return false
}

// NormalizeNatureToKeywords maps an LLM phrase to the closest configured keyword
// when it clearly contains one (longest match wins). Otherwise the LLM phrase
// is kept verbatim.
func NormalizeNatureToKeywords(nature string, natureCodes []string) string {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" {
		return ""
	}
	padded := " " + n + " "
	codes := sortedNatureCodes(natureCodes)
	for _, kw := range codes {
		if kw == n {
			return kw
		}
	}
	best := ""
	bestPos := -1
	bestLen := 0
	for _, kw := range codes {
		if kw == "" {
			continue
		}
		if !wholeWordContains(padded, kw) && !natureWordStemMatch(n, kw) {
			continue
		}
		idx := strings.Index(padded, " "+kw+" ")
		if idx < 0 {
			idx = 0
		}
		if bestPos < 0 || idx < bestPos || (idx == bestPos && len(kw) > bestLen) {
			bestPos = idx
			bestLen = len(kw)
			best = kw
		}
	}
	if best != "" {
		return best
	}
	for _, kw := range codes {
		if len(n) >= 4 && kw != "" && strings.Contains(" "+kw+" ", " "+n+" ") {
			return kw
		}
	}
	return n
}

// inferFireworksComplaintNature classifies caller-reported fireworks when the
// transcript names fireworks and reads like a complaint / area check, not fire
// suppression.
func inferFireworksComplaintNature(transcript string) string {
	u := " " + strings.Map(func(r rune) rune {
		switch r {
		case '.', ',', ';', ':', '!', '?', '\'', '"':
			return ' '
		}
		return r
	}, strings.ToUpper(transcript)) + " "
	if !strings.Contains(u, " FIREWORKS ") && !strings.Contains(u, " FIREWORK ") {
		return ""
	}
	for _, cue := range []string{
		" COMPLAINT", " REQUESTING AN OFFICER", " CHECK THE AREA",
		" CALLER ", " REPORTING ", " LIGHTING OFF ", " SHOOTING OFF ",
		" SETTING OFF ",
	} {
		if strings.Contains(u, cue) {
			return "FIREWORKS COMPLAINT"
		}
	}
	return ""
}

var animalDispatchTokens = []string{
	"RACCOON", "DEER", "COYOTE", "SKUNK", "OPOSSUM", "SNAKE", "FOX", "BEAR",
	"DOG", "CAT", "BIRD", "HORSE", "COW", "LIVESTOCK", "SQUIRREL", "GROUNDHOG",
}

// inferAnimalComplaintNature classifies sick/injured wildlife or animal-control
// calls so "INJURED RACCOON" does not become human INJURY.
func inferAnimalComplaintNature(transcript string, natureCodes []string) string {
	u := " " + strings.ToUpper(transcript) + " "
	hasAnimal := false
	for _, a := range animalDispatchTokens {
		if strings.Contains(u, " "+a+" ") {
			hasAnimal = true
			break
		}
	}
	if !hasAnimal {
		return ""
	}
	animalCue := false
	for _, cue := range []string{
		" SICK ", " INJURED ", " HURT ", " ANIMAL ", " WILD ", " BACKYARD ",
		" COMPLAINT ", " POSSIBLY ",
	} {
		if strings.Contains(u, cue) {
			animalCue = true
			break
		}
	}
	if !animalCue {
		return ""
	}
	prefs := []string{"ANIMAL COMPLAINT", "WILD ANIMAL", "ANIMAL", "SICK ANIMAL", "INJURED ANIMAL"}
	for _, p := range prefs {
		if norm := NormalizeNatureToKeywords(p, natureCodes); norm != "" {
			return norm
		}
	}
	for _, kw := range sortedNatureCodes(natureCodes) {
		ku := strings.ToUpper(strings.TrimSpace(kw))
		if strings.Contains(ku, "ANIMAL") {
			return ku
		}
	}
	return ""
}

// inferCourtesyTransportNature labels officer-initiated courtesy rides/transports.
func inferCourtesyTransportNature(transcript string, natureCodes []string) string {
	u := " " + strings.ToUpper(transcript) + " "
	for _, phrase := range []string{"COURTESY TRANSPORT", "COURTESY RIDE"} {
		if strings.Contains(u, " "+phrase+" ") || strings.Contains(u, " "+phrase+" TO ") {
			if norm := NormalizeNatureToKeywords("TRANSPORT PERSON OR PRISONER", natureCodes); norm != "" {
				return norm
			}
			return "TRANSPORT PERSON OR PRISONER"
		}
	}
	return ""
}

func isTransportPersonNature(nature string) bool {
	n := strings.ToUpper(strings.TrimSpace(nature))
	return n == "TRANSPORT PERSON OR PRISONER" || strings.Contains(n, "PRISONER TRANSPORT") ||
		strings.Contains(n, "COURT TRANSPORT")
}

func sortedNatureCodes(natureCodes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, kw := range natureCodes {
		k := strings.ToUpper(strings.TrimSpace(kw))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// dispatchHarnessMarkers are RealIncidentMarkers that prove a toned dispatch but
// must not become the incident nature label (SQUAD CALL ≠ nature of call).
var dispatchHarnessMarkers = map[string]bool{
	"SQUAD CALL": true, "SQUAW CALL": true, "SWAB CALL": true, "SQUARE CALL": true,
	"2ND SQUAD": true, "SECOND SQUAD": true, "2ND CALL": true, "SECOND CALL": true,
	"MUTUALLY": true,
}

func isDispatchHarnessMarker(marker string) bool {
	return dispatchHarnessMarkers[strings.ToUpper(strings.TrimSpace(marker))]
}

// victimQualifierNature returns the incident type when dispatch qualified a
// victim descriptor ("FALL VICTIM", "FIRE VICTIM") instead of bare "VICTIM".
func victimQualifierNature(paddedUpper string, natureCodes []string) string {
	for _, code := range sortedNatureCodes(natureCodes) {
		c := strings.ToUpper(strings.TrimSpace(code))
		if c == "" || c == "VICTIM" || isDispatchHarnessMarker(c) || isApparatusOnlyNatureLabel(c) || IsGenericNature(c) {
			continue
		}
		if strings.Contains(paddedUpper, " "+c+" VICTIM ") {
			return c
		}
	}
	for _, q := range []string{"FALL", "FIRE", "CRASH", "MVA", "ASSAULT", "SHOOTING", "STABBING", "DROWNING"} {
		if strings.Contains(paddedUpper, " "+q+" VICTIM ") {
			if norm := NormalizeNatureToKeywords(q, natureCodes); norm != "" {
				return norm
			}
			return q
		}
	}
	return ""
}

// inferForADispatchNature recovers the incident type from toned-dispatch phrasing
// ("… FOR A BURNING SMELL") when the opener word is an STT mishear (WARREN→WARRANT).
func inferForADispatchNature(transcript string, natureCodes []string) string {
	padded := " " + strings.ToUpper(transcript) + " "
	for _, lead := range []string{" FOR A ", " FOR AN "} {
		idx := strings.Index(padded, lead)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(padded[idx+len(lead):])
		for _, sep := range []string{" TIMEOUT", " TIME OUT", " YOUR TIME", " DISPATCH CLEAR", " TIME TO"} {
			if i := strings.Index(rest, sep); i > 0 {
				rest = strings.TrimSpace(rest[:i])
			}
		}
		rest = strings.TrimRight(rest, ".,;")
		if rest == "" {
			continue
		}
		if n := natureFromForAPhrase(rest, natureCodes); n != "" {
			return n
		}
	}
	return ""
}

func stripDispatchDemographicLead(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	for i := 0; i < 3; i++ {
		next := strings.TrimSpace(dispatchDemographicLeadRE.ReplaceAllString(s, ""))
		if next == s {
			break
		}
		s = next
	}
	for _, lead := range []string{"HAVING ", "WITH ", "COMPLAINING OF ", "C/O ", "REPORTING "} {
		if strings.HasPrefix(s, lead) {
			s = strings.TrimSpace(s[len(lead):])
		}
	}
	return s
}

func natureFromForAPhrase(rest string, natureCodes []string) string {
	rest = stripDispatchDemographicLead(rest)
	if rest == "" {
		return ""
	}
	best := ""
	bestLen := 0
	segments := []string{rest}
	for _, seg := range strings.Split(rest, ",") {
		seg = stripDispatchDemographicLead(strings.TrimSpace(seg))
		if seg != "" {
			segments = append(segments, seg)
		}
	}
	for _, seg := range segments {
		paddedSeg := " " + seg + " "
		for _, code := range sortedNatureCodes(natureCodes) {
			ku := strings.ToUpper(strings.TrimSpace(code))
			if ku == "" || isApparatusOnlyNatureLabel(ku) || isDispatchHarnessMarker(ku) {
				continue
			}
			if strings.HasPrefix(seg+" ", ku+" ") || seg == ku || transcriptContainsNatureKeyword(paddedSeg, ku) {
				if len(ku) > bestLen {
					bestLen = len(ku)
					best = ku
				}
			}
		}
	}
	if best != "" {
		return best
	}
	if w := inferWiresTreesDownNature(" "+rest, natureCodes); w != "" {
		return w
	}
	if n := localNature(" "+rest+" ", natureCodes); n != "" {
		return n
	}
	return ""
}

func natureAtTranscriptOpener(nature, transcript string) bool {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(transcript)))
	n := strings.ToUpper(strings.TrimSpace(nature))
	if len(fields) == 0 || n == "" {
		return false
	}
	return fields[0] == n
}

// courtNatureExplicitlySpoken reports real court/legal dispatch context — not
// a thoroughfare suffix on an address ("150 CHARLES COURT").
func courtNatureExplicitlySpoken(paddedUpper string) bool {
	for _, cue := range []string{
		" COURT ORDER", " COURT HEARING", " COURT PAPER", " COURT TRANSPORT",
		" GO TO COURT", " AT COURT ", " IN COURT ", " COURTHOUSE",
	} {
		if strings.Contains(paddedUpper, cue) {
			return true
		}
	}
	return false
}

// natureLabelIsStreetSuffixFalsePositive reports when a configured nature label
// only appears as a street suffix in a spoken address (CHARLES COURT ≠ COURT).
func natureLabelIsStreetSuffixFalsePositive(label, paddedUpper string) bool {
	n := strings.ToUpper(strings.TrimSpace(label))
	switch n {
	case "COURT", "CT":
		if courtNatureExplicitlySpoken(paddedUpper) {
			return false
		}
		return streetAddressCourtRE.MatchString(strings.Trim(paddedUpper, " "))
	default:
		return false
	}
}

func scrubNatureStreetSuffixFalsePositive(nature, transcript string) string {
	n := strings.ToUpper(strings.TrimSpace(nature))
	if n == "" {
		return ""
	}
	if natureLabelIsStreetSuffixFalsePositive(n, " "+strings.ToUpper(transcript)+" ") {
		return ""
	}
	return nature
}

func defaultUnknownProblemNature(natureCodes []string) string {
	for _, candidate := range []string{
		"UNKNOWN PROBLEM", "EMS UNKNOWN PROBLEM/UNCLASSIFIED", "UNKNOWN PROBLEM/UNCLASSIFIED",
	} {
		if isExactNatureCode(candidate, natureCodes) {
			return strings.ToUpper(strings.TrimSpace(candidate))
		}
		if norm := NormalizeNatureToKeywords(candidate, natureCodes); norm != "" {
			return norm
		}
	}
	return ""
}

// natureKeywordIsNegated reports when dispatch explicitly ruled out a keyword
// ("NO SMOKE OR FIRE") rather than describing an active incident.
func natureKeywordIsNegated(paddedUpper, keyword string) bool {
	kw := strings.ToUpper(strings.TrimSpace(keyword))
	if kw != "FIRE" && kw != "SMOKE" {
		return false
	}
	if strings.Contains(paddedUpper, " NO SMOKE OR FIRE") {
		return true
	}
	for _, prefix := range []string{" NO ", " NOT "} {
		if strings.Contains(paddedUpper, prefix+kw+" ") ||
			strings.Contains(paddedUpper, prefix+kw+".") ||
			strings.Contains(paddedUpper, prefix+kw+",") {
			return true
		}
	}
	return false
}

// transcriptDescribesWiresTreesDown reports utility line or tree-on-roadway hazards.
func transcriptDescribesWiresTreesDown(transcript string) bool {
	u := " " + strings.ToUpper(transcript) + " "
	for _, cue := range []string{
		" WIRES DOWN", " WIRE DOWN", " TREE DOWN", " TREES DOWN", " TREEDOWN",
		" TORE DOWN A WIRE", " TORE DOWN THE WIRE", " KNOCKED DOWN A WIRE", " PULLED DOWN A WIRE",
		" POWER LINE DOWN", " STRUCK A POWER LINE", " HIT A POWER LINE",
		" WIRES ARE ON", " LINES DOWN", " LINE DOWN",
		" POWER LINES ARE ON", " POWER LINES ON THE GROUND",
		" POWER LINE ON THE GROUND", " LINES ARE ON THE GROUND", " LINES ON THE GROUND",
		" TREE ACROSS THE ROADWAY", " TREE ACROSS THE ROAD", " TREE ACROSS ROAD",
	} {
		if strings.Contains(u, cue) {
			return true
		}
	}
	if strings.Contains(u, " POWER LINE") &&
		(strings.Contains(u, " CUT DOWN A TREE") || strings.Contains(u, " CUT DOWN THE TREE") ||
			strings.Contains(u, " TREE HAS FALLEN") || strings.Contains(u, " TREE FELL")) {
		return true
	}
	return false
}

// inferWiresTreesDownNature recovers utility line / tree-on-wire dispatches.
func inferWiresTreesDownNature(transcript string, natureCodes []string) string {
	if !transcriptDescribesWiresTreesDown(transcript) {
		return ""
	}
	if norm := NormalizeNatureToKeywords("WIRES/TREES DOWN", natureCodes); norm != "" {
		return norm
	}
	return "WIRES/TREES DOWN"
}

// inferSpecificDispatchNature recovers a real incident type from dispatch
// phrasing when keyword lists only matched an apparatus token (LADDER 12, etc.).
func inferSpecificDispatchNature(transcript string, natureCodes []string) string {
	if wires := inferWiresTreesDownNature(transcript, natureCodes); wires != "" {
		return wires
	}
	u := strings.ToUpper(transcript)
	padded := " " + u + " "
	if ilpersonNatureRE.MatchString(u) || strings.Contains(padded, " ILL PERSON ") ||
		strings.Contains(u, "ILPERSON") || strings.Contains(u, "ILLPERSON") {
		return "ILL PERSON"
	}
	if strings.Contains(padded, " COMPLAINT ABOUT ") || strings.Contains(padded, " COMPLAINT REGARDING ") ||
		strings.Contains(padded, " COMPLAINT FOR ") {
		for _, kw := range sortedNatureCodes(natureCodes) {
			ku := strings.ToUpper(strings.TrimSpace(kw))
			if strings.Contains(ku, "COMPLAINT") && wholeWordContains(padded, ku) {
				return ku
			}
		}
		if norm := NormalizeNatureToKeywords("NOISE COMPLAINT", natureCodes); norm != "" {
			return norm
		}
		if wholeWordContains(padded, "COMPLAINT") {
			return "COMPLAINT"
		}
	}
	if strings.Contains(padded, " PARKING COMPLAINT") || strings.Contains(padded, " HAVE A PARKING COMPLAINT") {
		if norm := NormalizeNatureToKeywords("PARKING COMPLAINT", natureCodes); norm != "" {
			return norm
		}
		for _, kw := range sortedNatureCodes(natureCodes) {
			ku := strings.ToUpper(strings.TrimSpace(kw))
			if strings.Contains(ku, "PARKING") && strings.Contains(ku, "COMPLAINT") && wholeWordContains(padded, ku) {
				return ku
			}
		}
	}
	if fallIncidentContextRE.MatchString(u) {
		if wholeWordContains(padded, "FALL PATIENT") {
			return "FALL PATIENT"
		}
		return "FALL"
	}
	for _, phrase := range []string{"EXTREME WEAKNESS", "GENERALIZED WEAKNESS", "SUDDEN WEAKNESS"} {
		if strings.Contains(padded, " "+phrase) {
			if norm := NormalizeNatureToKeywords("WEAKNESS", natureCodes); norm != "" {
				return norm
			}
			return "WEAKNESS"
		}
	}
	if strings.Contains(padded, " WEAKNESS") {
		if norm := NormalizeNatureToKeywords("WEAKNESS", natureCodes); norm != "" {
			return norm
		}
		return "WEAKNESS"
	}
	if strings.Contains(padded, " AUTO ACCIDENT") || strings.Contains(padded, " MOTOR VEHICLE ACCIDENT") ||
		strings.Contains(padded, " MVA ") {
		for _, label := range []string{
			"CRASH WITH REPORTED INJURIES", "CRASH PROPERTY DAMAGE", "MVA", "ACCIDENT",
		} {
			if norm := NormalizeNatureToKeywords(label, natureCodes); norm != "" {
				return norm
			}
		}
	}
	if strings.Contains(padded, " TROUBLE ALARM") {
		if norm := NormalizeNatureToKeywords("TROUBLE ALARM", natureCodes); norm != "" {
			return norm
		}
		return "TROUBLE ALARM"
	}
	if strings.Contains(padded, " CONCERNED WITH HIS WELFARE") ||
		strings.Contains(padded, " CONCERNED WITH HER WELFARE") ||
		strings.Contains(padded, " CONCERNED WITH THEIR WELFARE") {
		if norm := NormalizeNatureToKeywords("WELFARE CHECK", natureCodes); norm != "" {
			return norm
		}
		return "WELFARE CHECK"
	}
	if strings.Contains(padded, " REFUSING TO LEAVE") ||
		strings.Contains(padded, " REFUSING, AND NOW") ||
		strings.Contains(padded, " ASKED TO LEAVE") {
		if norm := NormalizeNatureToKeywords("TRESPASS", natureCodes); norm != "" {
			return norm
		}
		if norm := NormalizeNatureToKeywords("DISTURBANCE", natureCodes); norm != "" {
			return norm
		}
		return "DISTURBANCE"
	}
	// Re-run keyword inference without apparatus-only tokens.
	filtered := make([]string, 0, len(natureCodes))
	for _, kw := range natureCodes {
		k := strings.ToUpper(strings.TrimSpace(kw))
		if k == "" || isApparatusOnlyNatureLabel(k) || isDispatchHarnessMarker(k) {
			continue
		}
		filtered = append(filtered, kw)
	}
	if n := localNature(u, filtered); n != "" && !isApparatusOnlyNatureLabel(n) && !isDispatchHarnessMarker(n) {
		return n
	}
	best := ""
	bestLen := 0
	for _, m := range RealIncidentMarkers {
		mk := strings.TrimSpace(strings.ToUpper(m))
		if mk == "" || isDispatchHarnessMarker(mk) {
			continue
		}
		if strings.Contains(padded, mk) {
			if best == "" || len(mk) > bestLen {
				best = mk
				bestLen = len(mk)
			}
		}
	}
	if best != "" {
		if best == "FELL" {
			return "FALL"
		}
		return best
	}
	return ""
}

var chiefComplaintNatureRE = regexp.MustCompile(`(?i)(?:CHIEF COMPLAINT|COMPLAINT TODAY)(?:\s+IS)?(?:\s+GOING TO BE)?\s+([A-Z][A-Z\s/\-]{2,40})`)

// inferChiefComplaintNature recovers the medic-stated chief complaint over vitals noise.
func inferChiefComplaintNature(transcript string, natureCodes []string) string {
	m := chiefComplaintNatureRE.FindStringSubmatch(strings.ToUpper(transcript))
	if len(m) < 2 {
		return ""
	}
	phrase := strings.TrimSpace(m[1])
	phrase = strings.TrimRight(phrase, ".,;")
	for _, tail := range []string{" HE SAID", " SHE SAID", " HE DID", " SHE DID", " RIGHT NOW"} {
		if idx := strings.Index(phrase, tail); idx > 0 {
			phrase = strings.TrimSpace(phrase[:idx])
		}
	}
	if phrase == "" {
		return ""
	}
	if norm := NormalizeNatureToKeywords(phrase, natureCodes); norm != "" {
		return norm
	}
	return phrase
}

// FinalizeIncidentNature validates, scrubs, and optionally normalizes nature.
// When the LLM/rule output is empty, keyword-list inference runs only when the
// transcript looks like real incident or relay traffic (not pure sign-off).
func FinalizeIncidentNature(nature, transcript string, natureCodes []string, isDispatch bool) string {
	return FinalizeIncidentNatureWithMap(nature, transcript, natureCodes, nil, isDispatch, nil, false)
}

// squadCallDispatchRE matches a "squad call" EMS tone-out marker, tolerating the
// common STT manglings of "squad" ("squaw", "squawk", "squall") and of "call"
// ("crawl", "hall"). It deliberately requires the trailing call word so an
// apparatus reference ("SQUAD 23", "SQUAD RESPONDING") is never mistaken for a
// tone-out. A squad tone-out is an EMS response even when the dispatcher speaks
// only a location and no ailment.
var squadCallDispatchRE = regexp.MustCompile(`(?i)\bSQUA(?:D|WK?|LL)\s+(?:CALL|CRAWL|HALL)\b`)

// forThatMedicalRE matches Trumbull-style EMS framing without the words
// "squad call" ("STATION 7 FOR THAT MEDICAL AT 605 WEST 3RD").
var forThatMedicalRE = regexp.MustCompile(`(?i)\bFOR\s+(?:THAT|THE|A)\s+MEDICAL\b`)

// inferSquadCallEMSNature returns the configured generic EMS label when the
// transcript is a squad (EMS) tone-out but nothing more specific classified.
// Used only as a last resort, just before the UNKNOWN PROBLEM catch-all, so a
// location-only squad dispatch ("STATION 28 SQUAD CALL, HILLVIEW DRIVE") lands
// on a real EMS nature instead of unknown.
func inferSquadCallEMSNature(transcript string, natureCodes []string) string {
	if !squadCallDispatchRE.MatchString(transcript) && !forThatMedicalRE.MatchString(transcript) {
		return ""
	}
	const ems = "EMERGENCY MEDICAL ASSISTANCE"
	if isExactNatureCode(ems, natureCodes) {
		return ems
	}
	return ""
}

func FinalizeIncidentNatureWithMap(nature, transcript string, natureCodes, matchTerms []string, isDispatch bool, phraseToLabel map[string]string, deferDefaultUnknown bool) string {
	match := effectiveMatchTerms(natureCodes, matchTerms)
	u := strings.ToUpper(transcript)
	if strings.Contains(u, "INJURED IN FALL") {
		nature = "FALL PATIENT"
	}
	n := ScrubIncidentNature(nature, transcript)
	n = CanonicalizeNatureLabel(n, phraseToLabel)
	n = NormalizeNatureToKeywords(n, natureCodes)
	n = clampNatureLabel(n, natureCodes)
	if forA := inferForADispatchNature(transcript, natureCodes); forA != "" {
		if n == "" || natureAtTranscriptOpener(n, transcript) ||
			strings.EqualFold(n, "LAKE EMERGENCY") ||
			natureKeywordIsLakeDepartmentFalsePositive(" "+u+" ", n) {
			n = forA
		}
	}
	if wires := inferWiresTreesDownNature(transcript, natureCodes); wires != "" {
		if n == "" || !isExactNatureCode(n, natureCodes) ||
			strings.EqualFold(n, "LAKE EMERGENCY") ||
			natureKeywordIsLakeDepartmentFalsePositive(" "+u+" ", n) {
			n = wires
		}
	}
	// Evidence-weighted correction: when another configured nature explains
	// strictly more transcript words than the working label, the words win
	// ("POWER LINES HANGING VERY LOW" is utility-line evidence, not a lone
	// gerund match). A scored pick is anchored by construction — its
	// configured phrases were found in the transcript.
	scoredAnchored := false
	if scored := scoredNatureOverride(n, transcript, natureCodes, match, phraseToLabel); scored != "" {
		n = scored
		scoredAnchored = true
	} else if natureScoredEvidenceConfirms(n, transcript, natureCodes, match, phraseToLabel) {
		scoredAnchored = true
	}
	if !scoredAnchored && !natureAnchoredInTranscript(n, transcript) {
		n = ""
	}
	if IsGenericNature(n) {
		n = ""
	}
	if n == "" || isApparatusOnlyNatureLabel(n) {
		if specific := inferSpecificDispatchNature(transcript, natureCodes); specific != "" {
			n = specific
			n = ScrubIncidentNature(n, transcript)
			n = NormalizeNatureToKeywords(n, natureCodes)
			n = clampNatureLabel(n, natureCodes)
		} else if cardiac := inferCardiacSymptomNature(transcript, natureCodes); cardiac != "" {
			n = cardiac
		} else if isApparatusOnlyNatureLabel(n) {
			n = ""
		}
	}
	if cardiac := inferCardiacSymptomNature(transcript, natureCodes); cardiac != "" &&
		(n == "" || isApparatusOnlyNatureLabel(n) || isLawEnforcementCrimeNature(n)) {
		n = cardiac
	}
	if seizure := inferSeizureActivityNature(transcript, natureCodes); seizure != "" {
		if n == "" || isApparatusOnlyNatureLabel(n) || strings.EqualFold(n, "CONSCIOUSNESS") ||
			strings.Contains(strings.ToUpper(n), "CONSCIOUS") {
			n = seizure
		}
	}
	if suicide := inferSuicideNature(transcript, natureCodes); suicide != "" {
		if n == "" || isApparatusOnlyNatureLabel(n) {
			n = suicide
		}
	}
	if n == "" && shouldInferNatureFromTranscript(transcript, isDispatch) {
		inferred := localNature(strings.ToUpper(transcript), match)
		n = ScrubIncidentNature(inferred, transcript)
		// A matched phrase belongs to its configured parent label; map it
		// before keyword containment can pick a shorter homonym code
		// ("POWER LINES HANGING" is a utility phrase, not the HANGING code).
		n = CanonicalizeNatureLabel(n, phraseToLabel)
		n = NormalizeNatureToKeywords(n, natureCodes)
		n = clampNatureLabel(n, natureCodes)
		// Spoken "UNKNOWN PROBLEM" is a catch-all, not a classified complaint —
		// clear it so squad EMS / scored evidence can still assign a real label.
		if isBareCatchAllUnknownNature(n) {
			n = ""
		}
		if scored := scoredNatureOverride(n, transcript, natureCodes, match, phraseToLabel); scored != "" {
			n = scored
			scoredAnchored = true
		} else if natureScoredEvidenceConfirms(n, transcript, natureCodes, match, phraseToLabel) {
			scoredAnchored = true
		}
		if !scoredAnchored && !natureAnchoredInTranscript(n, transcript) {
			n = ""
		}
		if isApparatusOnlyNatureLabel(n) {
			if specific := inferSpecificDispatchNature(transcript, natureCodes); specific != "" {
				n = ScrubIncidentNature(specific, transcript)
				n = NormalizeNatureToKeywords(n, natureCodes)
				n = clampNatureLabel(n, natureCodes)
			} else {
				n = ""
			}
		}
	}
	if fw := inferFireworksComplaintNature(transcript); fw != "" {
		n = fw
	}
	if animal := inferAnimalComplaintNature(transcript, natureCodes); animal != "" {
		n = animal
	}
	// Late evidence-weighted pass: inference helpers above can settle on a
	// label matched from a single word; when another configured nature
	// explains strictly more transcript words, the words win.
	if scored := scoredNatureOverride(n, transcript, natureCodes, match, phraseToLabel); scored != "" {
		n = scored
	}
	// Nothing settled yet — take the best complaint evidence (response-mode
	// demoted) before falling through to squad EMS / UNKNOWN catch-alls.
	if strings.TrimSpace(n) == "" {
		if best := bestScoredNature(transcript, natureCodes, match, phraseToLabel, 1); best != "" {
			n = best
		}
	}
	if natureLoneGerundEvidence(n, transcript, natureCodes, match, phraseToLabel) {
		n = ""
	}
	if TranscriptIsRadioChatter(transcript) && !containsRealIncidentMarker(" "+strings.ToUpper(transcript)+" ") {
		n = ""
	}
	if TranscriptIsPatchInService(transcript) || TranscriptIsUnitOnSiteStatus(transcript) ||
		TranscriptIsMedicEnRouteUpdate(transcript) || TranscriptIsUnitEnRouteBriefing(transcript) ||
		TranscriptIsOfficerDirectedVisit(transcript) || TranscriptIsHospitalBedCoordination(transcript) {
		n = ""
	}
	if TranscriptIsTransportNarrative(transcript) {
		if tp := inferCourtesyTransportNature(transcript, natureCodes); tp != "" {
			n = tp
		} else if n != "" && !isTransportPersonNature(n) {
			n = ""
		}
	}
	if strings.Contains(strings.ToUpper(n), "BLOOD PRESSURE") {
		if chief := inferChiefComplaintNature(transcript, natureCodes); chief != "" {
			n = chief
		} else if strings.Contains(u, "ON THE MONITOR") || strings.Contains(u, "HEART RATE") {
			n = ""
		}
	}
	if IsGenericNature(n) {
		return ""
	}
	if strings.EqualFold(n, "INJURED") && strings.Contains(u, "INJURED IN FALL") {
		n = "FALL PATIENT"
	}
	if strings.EqualFold(n, "VICTIM") {
		if q := victimQualifierNature(" "+u+" ", natureCodes); q != "" {
			n = q
		}
	}
	n = harmonizeNatureSynonyms(n, transcript, natureCodes)
	n = scrubNatureStreetSuffixFalsePositive(n, transcript)
	if !deferDefaultUnknown && strings.TrimSpace(n) == "" && shouldInferNatureFromTranscript(transcript, isDispatch) {
		if ems := inferSquadCallEMSNature(transcript, natureCodes); ems != "" {
			n = ems
		} else {
			n = defaultUnknownProblemNature(natureCodes)
		}
	}
	n = CanonicalizeNatureLabel(n, phraseToLabel)
	// Every classified call must land on a configured nature code. Map stray
	// labels through phrase evidence and keyword containment; when nothing in
	// the catalog fits, fall back to the configured catch-all instead of
	// publishing an invented label.
	if strings.TrimSpace(n) != "" && len(natureCodes) > 0 && !isExactNatureCode(n, natureCodes) {
		if norm := NormalizeNatureToKeywords(n, natureCodes); isExactNatureCode(norm, natureCodes) {
			n = norm
		} else if scored := bestScoredNature(transcript, natureCodes, match, phraseToLabel, 2); scored != "" &&
			isExactNatureCode(scored, natureCodes) {
			n = scored
		} else if !deferDefaultUnknown && shouldInferNatureFromTranscript(transcript, isDispatch) {
			n = defaultUnknownProblemNature(natureCodes)
		} else {
			n = ""
		}
	}
	return n
}

// DefaultUnknownProblemLabel returns the configured catch-all nature label.
func DefaultUnknownProblemLabel(natureCodes []string) string {
	return defaultUnknownProblemNature(natureCodes)
}

// ShouldApplyUnknownNatureFallback reports whether a transcript should receive
// the catch-all unknown label when no specific category matched.
func ShouldApplyUnknownNatureFallback(transcript string) bool {
	if TranscriptIsLicensePlateReadout(transcript) {
		return false
	}
	if transcriptIsIncompleteSquadOpener(transcript) || transcriptIsBareUnitStatus(transcript) {
		return false
	}
	if TranscriptIsPhoneticAlphabetRollCall(transcript) {
		return false
	}
	return shouldInferNatureFromTranscript(transcript, false)
}

// ShouldEnqueueCallNatureBackfill reports whether a call should enter the
// nature backfill queue (UNKNOWN relabel, or empty nature with incident signal).
func ShouldEnqueueCallNatureBackfill(oldNature, transcript string) bool {
	old := strings.TrimSpace(oldNature)
	if IsDefaultUnknownNatureLabel(old) {
		return true
	}
	if old != "" {
		return false
	}
	if TranscriptIsLicensePlateReadout(transcript) {
		return false
	}
	if TranscriptIsRadioChatter(transcript) && !containsRealIncidentMarker(" "+strings.ToUpper(transcript)+" ") {
		return false
	}
	words := strings.Fields(strings.TrimSpace(transcript))
	if len(words) <= 2 && !containsRealIncidentMarker(" "+strings.ToUpper(transcript)+" ") {
		return false
	}
	return shouldInferNatureFromTranscript(transcript, false)
}

// SanitizeCallNatureAssignment clears catch-all, chatter, and incomplete-opener
// labels before persisting call nature from rules or OpenAI.
func SanitizeCallNatureAssignment(transcript, nature string, natureCodes []string) string {
	nature = strings.ToUpper(strings.TrimSpace(nature))
	if nature == "" {
		return ""
	}
	// Bare UNKNOWN PROBLEM is a last-resort catch-all. EMS UNKNOWN is a real
	// classified label ("UNKNOWN MEDICAL") and must not be rewritten to the
	// generic UNKNOWN PROBLEM default.
	if isBareCatchAllUnknownNature(nature) {
		if ShouldApplyUnknownNatureFallback(transcript) {
			return DefaultUnknownProblemLabel(natureCodes)
		}
		return ""
	}
	u := " " + strings.ToUpper(transcript) + " "
	if TranscriptIsLicensePlateReadout(transcript) {
		return ""
	}
	if TranscriptIsRadioChatter(transcript) && !containsRealIncidentMarker(u) {
		return ""
	}
	if transcriptIsIncompleteSquadOpener(transcript) {
		return ""
	}
	if transcriptIsBareUnitStatus(transcript) {
		return ""
	}
	if corrected := correctFalseMedicalAlarmNature(u, nature, natureCodes); corrected != nature {
		return corrected
	}
	return nature
}

// correctFalseMedicalAlarmNature rewrites ALARM DROP MEDICAL when the dispatch
// is a burglar/ATM/tamper-style alarm (or otherwise has no medical/pendant cue).
func correctFalseMedicalAlarmNature(paddedUpper, nature string, natureCodes []string) string {
	if strings.ToUpper(strings.TrimSpace(nature)) != "ALARM DROP MEDICAL" {
		return nature
	}
	if medicalAlarmEvidenceInTranscript(paddedUpper) {
		return nature
	}
	if securityAlarmEvidenceInTranscript(paddedUpper) {
		if label := clampNatureLabel("BURGLARY", natureCodes); label != "" {
			return label
		}
		return ""
	}
	if fireAlarmEvidenceInTranscript(paddedUpper) {
		if label := clampNatureLabel("FIRE ALARM DROP", natureCodes); label != "" {
			return label
		}
	}
	// Model picked medical with no medical/pendant wording — drop it.
	return ""
}

func medicalAlarmEvidenceInTranscript(paddedUpper string) bool {
	for _, kw := range []string{
		" MEDICAL ALARM ", " MEDICAL ALERT ", " MEDICAL PANIC ", " MEDICAL DROP ",
		" PENDANT ", " PENNANT ", " LIFELINE ", " PERSONAL ALARM ", " HELP BUTTON ",
		" MEDICAL ",
	} {
		if strings.Contains(paddedUpper, kw) {
			return true
		}
	}
	return false
}

func securityAlarmEvidenceInTranscript(paddedUpper string) bool {
	for _, kw := range []string{
		" TAMPER ", " BURGLAR ", " BURG ALARM ", " INTRUSION ", " HOLD UP ", " HOLDUP ",
		" ATM ", " ETM ", " SILENT ALARM ", " AUDIBLE ALARM ",
	} {
		if strings.Contains(paddedUpper, kw) {
			return true
		}
	}
	return false
}

func fireAlarmEvidenceInTranscript(paddedUpper string) bool {
	for _, kw := range []string{
		" FIRE ALARM ", " SMOKE ALARM ", " SMOKE DETECTOR ", " AUTOMATIC ALARM ",
		" TROUBLE ALARM ", " PULL STATION ",
	} {
		if strings.Contains(paddedUpper, kw) {
			return true
		}
	}
	return false
}

func transcriptIsIncompleteSquadOpener(transcript string) bool {
	u := strings.TrimRight(strings.ToUpper(strings.TrimSpace(transcript)), ".")
	if !strings.Contains(u, "SQUAD CALL") {
		return false
	}
	words := strings.Fields(u)
	if len(words) <= 4 && strings.HasSuffix(u, "SQUAD CALL") {
		return true
	}
	if strings.Contains(u, "SQUAD CALL,") || strings.Contains(u, "SQUAD CALL ") {
		if len(words) <= 6 {
			for _, m := range []string{
				"FALL", "BREATHING", "PAIN", "BLEEDING", "UNCONSCIOUS", "CHEST",
				"OVERDOSE", "SEIZURE", "INJURY", "MEDICAL", "PATIENT", "VOMIT",
				"STROKE", "CARDIAC", "LAYING", "LYING", "DRIVEWAY", "MALE", "FEMALE",
				"YEAR-OLD", "YEAR OLD",
			} {
				if strings.Contains(u, m) {
					return false
				}
			}
			if !screenHasHouseNumberWord(u) {
				return true
			}
		}
	}
	if strings.Contains(u, "SQUAD CALL IS ") {
		for _, m := range []string{
			"FALL", "BREATHING", "PAIN", "BLEEDING", "UNCONSCIOUS", "CHEST",
			"OVERDOSE", "SEIZURE", "INJURY", "MEDICAL", "PATIENT", "VOMIT",
			"STROKE", "CARDIAC", "LAYING", "LYING", "DRIVEWAY",
		} {
			if strings.Contains(u, m) {
				return false
			}
		}
		return true
	}
	return false
}

func transcriptIsBareUnitStatus(transcript string) bool {
	u := strings.TrimRight(strings.ToUpper(strings.TrimSpace(transcript)), ".")
	if containsRealIncidentMarker(" " + u + " ") {
		return false
	}
	words := strings.Fields(strings.ReplaceAll(u, ",", " "))
	if len(words) == 0 || len(words) > 5 {
		return false
	}
	switch words[0] {
	case "MEDIC", "ENGINE", "RESCUE", "SQUAD", "LADDER", "CHIEF", "CAPTAIN", "BATTALION", "CRESCUE":
		return true
	default:
		return false
	}
}

// IsDefaultUnknownNatureLabel reports catch-all unknown labels assigned when no
// specific category matched — OpenAI classify should run before these are set.
func IsDefaultUnknownNatureLabel(nature string) bool {
	n := strings.ToUpper(strings.TrimSpace(nature))
	switch n {
	case "UNKNOWN PROBLEM", "EMS UNKNOWN PROBLEM/UNCLASSIFIED", "UNKNOWN PROBLEM/UNCLASSIFIED":
		return true
	default:
		return false
	}
}

// isBareCatchAllUnknownNature is the dispatcher catch-all ("UNKNOWN PROBLEM")
// without a medical qualifier. Spoken catch-alls must not lock classification —
// a squad tone-out still deserves EMERGENCY MEDICAL ASSISTANCE, and
// "UNKNOWN MEDICAL" still maps to EMS UNKNOWN PROBLEM/UNCLASSIFIED.
func isBareCatchAllUnknownNature(nature string) bool {
	switch strings.ToUpper(strings.TrimSpace(nature)) {
	case "UNKNOWN PROBLEM", "UNKNOWN PROBLEM/UNCLASSIFIED":
		return true
	default:
		return false
	}
}

func harmonizeNatureSynonyms(n, transcript string, natureCodes []string) string {
	u := " " + strings.ToUpper(transcript) + " "
	if len(n) > maxNatureLabelLen && !isExactNatureCode(n, natureCodes) {
		if specific := natureFromForAPhrase(n, natureCodes); specific != "" {
			n = specific
		} else if kw := localNature(u, natureCodes); kw != "" {
			n = kw
		}
	}
	if strings.Contains(u, " THROWING UP") || strings.Contains(u, " THROW UP") {
		if x := NormalizeNatureToKeywords("VOMITING", natureCodes); x != "" {
			return x
		}
		if strings.Contains(strings.ToUpper(n), "THROW") || strings.Contains(strings.ToUpper(n), "STOMACH") {
			return "VOMITING"
		}
	}
	if strings.Contains(u, " ACCIDENT ") || strings.Contains(u, " AUTO ACCIDENT") ||
		strings.Contains(u, " THREE-CAR ACCIDENT") {
		if strings.EqualFold(n, "CRASH") {
			if x := NormalizeNatureToKeywords("ACCIDENT", natureCodes); x != "" {
				return x
			}
			return "ACCIDENT"
		}
	}
	if strings.Contains(u, " SMOKE SHOWING ") {
		if strings.EqualFold(n, "SMOKE") {
			if x := NormalizeNatureToKeywords("SMOKE SHOWING", natureCodes); x != "" {
				return x
			}
			return "SMOKE SHOWING"
		}
	}
	if TranscriptIsOnScenePatientUpdate(transcript) {
		for _, cue := range []string{
			"SHORTNESS OF BREATH", "CHEST PAIN", "DIFFICULTY BREATHING",
			"BREATHING PROBLEMS", "UNCONSCIOUS", "SEIZURE", "STROKE", "OVERDOSE",
		} {
			if strings.Contains(u, " "+cue) {
				if x := NormalizeNatureToKeywords(cue, natureCodes); x != "" {
					return x
				}
				return cue
			}
		}
	}
	return n
}

func transcriptDescribesSuicideRisk(paddedUpper string) bool {
	for _, cue := range []string{
		" ATTEMPTED SUICIDE", " ATTEMPTED TO KILL", " INTENDS TO KILL", " INTENDS TO STRANGLE",
		" STRANGLING HIMSELF", " STRANGLE HIMSELF", " STRANGLE MYSELF", " STRANGLING MYSELF",
		" SUICIDAL", " SUICIDE ATTEMPT", " SUICIDE THREAT", " VIA HANGING", " HANGING HIMSELF",
		" KILL HIMSELF", " KILL MYSELF", " KILL HERSELF", " KILL THEMSELF",
	} {
		if strings.Contains(paddedUpper, cue) {
			return true
		}
	}
	return transcriptContainsNatureKeyword(paddedUpper, "SUICIDE") ||
		transcriptContainsNatureKeyword(paddedUpper, "SUICIDAL")
}

func inferSuicideNature(transcript string, natureCodes []string) string {
	u := " " + strings.ToUpper(transcript) + " "
	if !transcriptDescribesSuicideRisk(u) {
		return ""
	}
	for _, pref := range []string{
		"SUICIDE ATTEMPT", "SUICIDAL", "SUICIDE THREAT", "SUICIDE", "MENTAL HEALTH", "MENTAL",
	} {
		if norm := NormalizeNatureToKeywords(pref, natureCodes); norm != "" {
			return norm
		}
	}
	return "SUICIDAL"
}

func shouldInferNatureFromTranscript(transcript string, isDispatch bool) bool {
	if isDispatch {
		return true
	}
	u := " " + strings.ToUpper(transcript) + " "
	if containsRealIncidentMarker(u) {
		return true
	}
	if TranscriptLikelyHasLocation(transcript, nil) &&
		!TranscriptIsRadioChatter(transcript) {
		return true
	}
	// Relay/callback with a described situation (custody, caller reporting, etc.)
	for _, m := range []string{
		" CALLING IN", " CALLER ", " REPORTING ", " COMPLAINT", " DISPUTE",
		" CUSTODY", " DOMESTIC", " SUBJECT ", " SUSPECT ",
	} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// NaturePromptHints builds compact category examples plus a sample of configured
// keywords for the LLM prompt (multi-word phrases prioritized).
func NaturePromptHints(natureCodes []string) string {
	const examples = `Examples by category (use when clearly applicable; otherwise summarize in 2-5 words):
- Fire: STRUCTURE FIRE, VEHICLE FIRE, FIRE ALARM, GAS LEAK, SMOKE INVESTIGATION
- Medical: CARDIAC ARREST, FALL PATIENT, UNCONSCIOUS, LIFT ASSIST, DIFFICULTY BREATHING
- Law: DOMESTIC, CHILD CUSTODY, CUSTODY DISPUTE, MVA, WELFARE CHECK, SHOTS FIRED, DISTURBANCE
- General: MUTUAL AID, WIRES DOWN, TREE DOWN, WATER RESCUE`
	codes := sortedNatureCodes(natureCodes)
	if len(codes) == 0 {
		return examples
	}
	limit := 50
	if len(codes) < limit {
		limit = len(codes)
	}
	sample := strings.Join(codes[:limit], ", ")
	return examples + "\nConfigured agency terms (prefer exact match when the situation fits): " + sample
}
