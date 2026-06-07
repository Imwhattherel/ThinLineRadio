// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type unitLearnNameResponse struct {
	Label string `json:"label"`
}

func (controller *Controller) suggestUnitLearnLabel(system *System, talkgroup *Talkgroup, unitRef uint, records []unitLearnCallRecord) string {
	existingLabels := []string{}
	if system != nil && system.Units != nil {
		system.Units.mutex.Lock()
		for _, u := range system.Units.List {
			if strings.TrimSpace(u.Label) != "" {
				existingLabels = append(existingLabels, fmt.Sprintf("%s (ref %d)", u.Label, u.UnitRef))
			}
		}
		system.Units.mutex.Unlock()
	}

	var transcriptBlocks []string
	for i, r := range records {
		block := fmt.Sprintf("Call %d", i+1)
		if strings.TrimSpace(r.RadioLabel) != "" {
			block += fmt.Sprintf(" — radio alias from source: %s", r.RadioLabel)
		}
		if strings.TrimSpace(r.Transcript) != "" {
			block += fmt.Sprintf("\nTranscript:\n%s", r.Transcript)
		}
		transcriptBlocks = append(transcriptBlocks, block)
	}

	systemPrompt := `You identify fire/EMS/police radio units from dispatch transcripts.
Given multiple transmissions from the same radio unit ID (unitRef), return ONE short human label for that unit (e.g. "Rescue 41", "Engine 12", "Dispatch", "Car 3").
Return JSON only: {"label":"..."}. If unclear or conflicting, use {"label":"UNKNOWN"}.
Do not include the numeric radio ID in the label unless it is part of the unit name (e.g. "Unit 4521").`

	userPrompt := fmt.Sprintf(`System: %s
Talkgroup: %s (TGID %d)
Radio unit ID (unitRef): %d
Existing units on this system (do not duplicate): %s

%s

What is the best label for radio unit ID %d?`,
		system.Label,
		talkgroup.Label,
		talkgroup.TalkgroupRef,
		unitRef,
		strings.Join(existingLabels, ", "),
		strings.Join(transcriptBlocks, "\n\n"),
		unitRef,
	)

	content, err := controller.openAIChatJSON(systemPrompt, userPrompt)
	if err != nil {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("unit auto-learn OpenAI: %v", err))
		return "UNKNOWN"
	}

	var nameResp unitLearnNameResponse
	if err := json.Unmarshal([]byte(content), &nameResp); err != nil {
		return "UNKNOWN"
	}
	label := strings.TrimSpace(nameResp.Label)
	if label == "" {
		return "UNKNOWN"
	}
	return label
}
