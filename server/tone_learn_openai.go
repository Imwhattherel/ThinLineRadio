// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type toneLearnNameResponse struct {
	Label string `json:"label"`
}

func (controller *Controller) suggestToneLearnLabel(system *System, talkgroup *Talkgroup, cand toneLearnCandidate, records []toneLearnCallRecord) string {
	var transcriptBlocks []string
	for i, r := range records {
		transcriptBlocks = append(transcriptBlocks, fmt.Sprintf("Call %d transcript:\n%s", i+1, r.Transcript))
	}

	existingLabels := []string{}
	for _, ts := range talkgroup.ToneSets {
		if ts.Label != "" {
			existingLabels = append(existingLabels, ts.Label)
		}
	}

	patternDesc := ""
	switch cand.PatternType {
	case toneLearnPatternABPair:
		patternDesc = fmt.Sprintf("Two-tone pair: A=%.1f Hz (%.2fs), B=%.1f Hz (%.2fs)",
			cand.AFrequency, cand.ADuration, cand.BFrequency, cand.BDuration)
	case toneLearnPatternLong:
		patternDesc = fmt.Sprintf("Long tone: %.1f Hz for %.2fs", cand.LongFrequency, cand.LongDuration)
	}

	systemPrompt := `You identify fire/EMS/police paging tone-outs from radio dispatch transcripts.
Given multiple transcripts from the same paging tone pattern, return ONE short label for the department or station being toned out (e.g. "Station 12", "North EMS", "Akron FD").
Return JSON only: {"label":"..."}. If unclear, use {"label":"UNKNOWN"}. Do not include tone frequencies in the label.`

	userPrompt := fmt.Sprintf(`System: %s
Talkgroup: %s (TGID %d)
Pattern: %s
Existing tone set labels on this channel (do not duplicate): %s

%s

What department or station is being toned out?`,
		system.Label,
		talkgroup.Label,
		talkgroup.TalkgroupRef,
		patternDesc,
		strings.Join(existingLabels, ", "),
		strings.Join(transcriptBlocks, "\n\n"),
	)

	content, err := controller.openAIChatJSON(systemPrompt, userPrompt)
	if err != nil {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("tone auto-learn OpenAI: %v", err))
		return "UNKNOWN"
	}

	var nameResp toneLearnNameResponse
	if err := json.Unmarshal([]byte(content), &nameResp); err != nil {
		return "UNKNOWN"
	}
	label := strings.TrimSpace(nameResp.Label)
	if label == "" {
		return "UNKNOWN"
	}
	return label
}

func resolveOpenAIBaseURL(apiURL string) string {
	url := strings.TrimSuffix(strings.TrimSpace(apiURL), "/")
	if url == "" {
		return "https://api.openai.com"
	}
	return url
}
