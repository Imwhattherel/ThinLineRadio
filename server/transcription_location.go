// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"strings"
)

// appendTranscriptionLocationContext appends jurisdiction context from the
// talkgroup/system incident-mapping config to an STT prompt when available.
func appendTranscriptionLocationContext(prompt string, system *System, talkgroup *Talkgroup, toneSeq *ToneSequence) string {
	if system == nil {
		return prompt
	}
	cfg, _, _ := resolveIncidentMappingForCall(system, talkgroup, toneSeq)
	loc := transcriptionLocationLabel(cfg)
	if loc == "" {
		return prompt
	}
	block := "Location context: " + loc + ". Prefer local place and street spellings for this jurisdiction."
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return block
	}
	return prompt + "\n\n" + block
}

func transcriptionLocationLabel(cfg IncidentMappingConfig) string {
	loc := strings.TrimSpace(cfg.LocationContext)
	if loc == "" {
		loc = strings.TrimSpace(cfg.GeoCity)
	}
	st := strings.TrimSpace(cfg.GeoState)
	if st == "" {
		return loc
	}
	if loc == "" {
		return st
	}
	if strings.Contains(strings.ToUpper(loc), strings.ToUpper(st)) {
		return loc
	}
	return loc + ", " + st
}
