//go:build natureeval

package mapping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type natureGoldCase struct {
	ID       uint64
	Expect   string // exact label, or "" for blank/non-incident
	AllowAny []string
	Text     string
}

func natureEvalGold() []natureGoldCase {
	return []natureGoldCase{
		{173846, "THREATS", nil, "2005 YULE STREET A FEMALE IN THE UNIT YELLING AT A CHILD FOR THE PAST WEEK THE NEIGHBORS THAT LIVE NEXT TO THEM SAY THAT THE FEMALE SAID THAT SHE S GOING TO GET A GUN AND SHOOT YOU AND SHOOT THE CHILD"},
		{173852, "", nil, "THANKS FOR YOUR SHOT HAVE A LONG ONE"},
		{172131, "", []string{"ALARM DROP MEDICAL", "FIRE ALARM DROP"}, "35 THERE ARE NO CALLS ON SHOTS THEY RE CALLING US FROM POLICE IT S AN ALARM AT 160"},
		{169919, "", nil, "SORRY DO YOU HAVE A CAD THAT YOU GUYS ARE USING FOR THAT POINT IT S ALL IMPORTANT IS IT THE SHOT SMARTER CAD 1585"},
		{167029, "", nil, "IT S A NEGATIVE NO ONE SHOT NO PROPERTY STRUCK"},
		{170326, "FOLLOW UP", []string{"RECOVERED VEHICLE", "INVESTIGATION"}, "SO THIS CAR THAT WE RECOVERED DOWN HERE IT S ON WASHINGTON THE ONE FROM THE SHOOTING AT 8811 DETROIT WE RE GOING TO PROCESS TOW THAT CAR DO YOU HAVE ANY EITHER CARS WORKING"},
		{168815, "", []string{"TRANSPORT PERSON OR PRISONER", "EVALUATION", "GENERAL ILLNESS"}, "MYSELF ONE AND ONE FOUR WE RE GOING TO SEE YOU IN 12 AT RISK PATIENTS WHICH IS GOING TO GET CHANGED INTO A HOSPITAL GUN"},
		{172395, "SHOTS FIRED", []string{"SHOTS BEING HEARD", "ACTIVE SHOOTER"}, "I M GOING TO HAVE YOU CHECK THIS SHOT SPOTTER 3312 SEYMOUR 3312 SEYMOUR IT S GOING TO BE FOR THREE ROUNDS IT DID INDICATE A POSSIBLE DRIVE BY MOVING SHOOTER AT 8 MILES AN HOUR THE PRIORITY WAS 1674"},
		{168958, "SHOTS FIRED", []string{"SHOTS BEING HEARD"}, "READY AS AN AUDIENCE TO BE ADVISED WE JUST RECEIVED A CALL FROM CPD STATING ON E 43RD STREET IN OWL S PLACE THEY RECEIVED TWO SHOTS FIRED TWO SHOTS FIRED"},
		{165954, "SHOTS FIRED", []string{"SHOTS BEING HEARD"}, "THANK YOU 8707 DETROIT 8707 DETROIT 9 ROUNDS DETECTED SHOT SPOT ALERT IT S A 1 CAT 1585 1585"},
		{169363, "SHOTS FIRED", []string{"ACTIVE SHOOTER", "ASSAULT"}, "I HAD AN 89 YEAH I OBSERVED THAT ON REAL TIME AT ABOUT 9 21 HOURS THAT VEHICLE PULLS UP ON A MALE IN FOOT A MALE IN FOOT GOES RUNNING FIRES A SHOT TO HONDA AND THE HONDA RETURNS FIRE"},
		{168777, "THREATS", []string{"ASSAULT", "DOMESTIC", "HARASSMENT"}, "WE HAD 9202 MARSHALL AVENUE WE HAD MELISSA CALLING IN STATING THAT THERE WAS A MALE THREATENING TO ASSAULT HER HUSBAND"},
		{172256, "PERSON WITH GUN", nil, "FOR TWO MALES WITH GUNS CARLOS SAYS THEY RE BOTH DRESSED AS WOMEN IN WHITE OUTFITS WITH DREADLOCKS NEAR THE LIQUOR STORE 93 KANSAS"},
	}
}

func loadNatureEvalLabels(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("/tmp/tlr-nature-labels.txt")
	if err != nil {
		t.Fatalf("labels: %v", err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	if len(out) < 20 {
		t.Fatalf("too few labels: %d", len(out))
	}
	return out
}

func loadNatureEvalKey(t *testing.T) string {
	t.Helper()
	if k := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); k != "" {
		return k
	}
	b, err := os.ReadFile("/tmp/tlr-oai-key.txt")
	if err != nil {
		t.Fatalf("api key: %v", err)
	}
	k := strings.TrimSpace(string(b))
	if k == "" {
		t.Fatal("empty api key")
	}
	return k
}

func natureGoldOK(g natureGoldCase, got string) bool {
	got = strings.ToUpper(strings.TrimSpace(got))
	if g.Expect == "" && len(g.AllowAny) == 0 {
		return got == ""
	}
	if got == "" && g.Expect == "" {
		return true
	}
	if g.Expect != "" && got == strings.ToUpper(g.Expect) {
		return true
	}
	for _, a := range g.AllowAny {
		a = strings.ToUpper(strings.TrimSpace(a))
		if got == a || (a != "" && strings.Contains(got, a)) {
			return true
		}
	}
	// blank expected with allow-list: blank also OK
	if g.Expect == "" && got == "" {
		return true
	}
	return false
}

func classifyWithSystemPrompt(apiKey, model, system, transcript string, labels []string) (string, error) {
	labels = openAIClassifyLabels(labels)
	if apiKey == "" || transcript == "" || len(labels) == 0 {
		return "", fmt.Errorf("missing input")
	}
	if model == "" {
		model = DefaultOpenAIModel
	}
	user := fmt.Sprintf("Categories:\n%s\n\nTranscript:\n%s", strings.Join(labels, "\n"), transcript)
	body := map[string]any{
		"model":           model,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	oaiResp, err := PostOpenAIWithRetry(OpenAIChatCompletionsURL, apiKey, bodyBytes)
	if err != nil {
		return "", err
	}
	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}
	var parsed struct {
		Nature string `json:"nature"`
	}
	content := strings.TrimSpace(oaiResp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", fmt.Errorf("parse %q: %w", content, err)
	}
	pick := strings.ToUpper(strings.TrimSpace(parsed.Nature))
	labelSet := map[string]bool{}
	for _, l := range labels {
		labelSet[strings.ToUpper(strings.TrimSpace(l))] = true
	}
	if pick == "" || !labelSet[pick] || IsDefaultUnknownNatureLabel(pick) {
		return "", nil
	}
	return pick, nil
}

func TestNatureOpenAIEvalMatrix(t *testing.T) {
	key := loadNatureEvalKey(t)
	labels := loadNatureEvalLabels(t)
	gold := natureEvalGold()

	current := `You classify radio dispatch transcripts into exactly one incident category.
Reply with JSON only: {"nature":"CATEGORY LABEL"}
Use only a label from the provided list. If nothing fits, reply {"nature":""}.`

	v2 := `You classify radio dispatch transcripts into exactly one incident category.
Reply with JSON only: {"nature":"CATEGORY LABEL"}
Use only a label from the provided list. If nothing fits, reply {"nature":""}.

Judge the WHOLE message, not isolated words.
- SHOTS FIRED / SHOTS BEING HEARD: only when shots were heard, fired, shot-spotter/shot-spot alert, drive-by, rounds detected, or someone was shot. Do NOT use these for threats to shoot later, "going to get a gun and shoot", sports/radio slang "shot", CAD chatter, or explicit negatives ("no one shot", "no calls on shots").
- THREATS: verbal threats of future violence (including threaten to shoot / get a gun and shoot) when no shots have occurred.
- PERSON WITH GUN: subject currently has/with a firearm — not "hospital gun" medical jargon and not a future threat to obtain a gun.
- Follow-ups, tows, and status chatter about an earlier shooting are not a new SHOTS FIRED incident unless new shots are reported.
Never use UNKNOWN PROBLEM or other catch-all unknown labels.`

	type run struct {
		name   string
		model  string
		system string
	}
	runs := []run{
		{"current/gpt-4o-mini", "gpt-4o-mini", current},
		{"current/gpt-5.4-mini", "gpt-5.4-mini", current},
		{"v2/gpt-4o-mini", "gpt-4o-mini", v2},
		{"v2/gpt-5.4-mini", "gpt-5.4-mini", v2},
		{"v2/gpt-4.1-mini", "gpt-4.1-mini", v2},
	}

	var report bytes.Buffer
	for _, r := range runs {
		ok, n := 0, 0
		fmt.Fprintf(&report, "\n=== %s ===\n", r.name)
		for _, g := range gold {
			n++
			got, err := classifyWithSystemPrompt(key, r.model, r.system, g.Text, labels)
			if err != nil {
				fmt.Fprintf(&report, "ERR %d %v\n", g.ID, err)
				continue
			}
			pass := natureGoldOK(g, got)
			if pass {
				ok++
			}
			mark := "FAIL"
			if pass {
				mark = "ok"
			}
			fmt.Fprintf(&report, "%s %d expect=%q got=%q\n", mark, g.ID, g.Expect, got)
		}
		fmt.Fprintf(&report, "SCORE %s: %d/%d\n", r.name, ok, n)
		t.Logf("%s: %d/%d", r.name, ok, n)
	}
	t.Log(report.String())
	fmt.Print(report.String())
}
