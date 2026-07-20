// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"strings"
	"testing"
)

func TestParseGeminiTranscriptJSON(t *testing.T) {
	tr, addr := parseGeminiTranscriptJSON(`{"transcript":"ENGINE 5 TO 5219 STAMBAUGH","address":"5219 STAMBAUGH AVE"}`)
	if tr != "ENGINE 5 TO 5219 STAMBAUGH" || addr != "5219 STAMBAUGH AVE" {
		t.Fatalf("got transcript=%q address=%q", tr, addr)
	}
	tr, addr = parseGeminiTranscriptJSON("```json\n{\"transcript\":\"NOISE\",\"address\":\"\"}\n```")
	if tr != "NOISE" || addr != "" {
		t.Fatalf("fenced: transcript=%q address=%q", tr, addr)
	}
}

func TestNormalizeGeminiTranscript(t *testing.T) {
	got := normalizeGeminiTranscript("  five-two, one nine.  ")
	if got != "FIVE TWO ONE NINE" {
		t.Fatalf("got %q", got)
	}
}

func TestGeminiBasePromptIsCompact(t *testing.T) {
	p := geminiBasePrompt(true, "")
	if len(p) > 220 {
		t.Fatalf("base extract prompt too long (%d): %q", len(p), p)
	}
	for _, part := range []string{"ALL CAPS", "digits", "address"} {
		if !strings.Contains(p, part) {
			t.Fatalf("prompt missing %q:\n%s", part, p)
		}
	}
	// Schema owns JSON shape — do not restate a JSON example in the prompt.
	if strings.Contains(p, `"transcript"`) || strings.Contains(strings.ToLower(p), "json") {
		t.Fatalf("prompt should not restate JSON schema:\n%s", p)
	}
}

func TestGeminiExtraContextDropsDuplicateInstructionPrompt(t *testing.T) {
	whispers := "Transcribe this public-safety radio / dispatch audio.\nOutput ONLY the spoken words as plain text in ALL CAPS."
	if got := geminiExtraContext(whispers); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	withLoc := whispers + "\n\nLocation context: Cuyahoga County, Ohio. Prefer local place and street spellings for this jurisdiction."
	got := geminiExtraContext(withLoc)
	if !strings.HasPrefix(got, "Location context:") || strings.Contains(got, "Transcribe") {
		t.Fatalf("expected only location line, got %q", got)
	}
	if got := geminiExtraContext("UNIT 2391 STAMBAUGH AVE"); !strings.Contains(got, "STAMBAUGH") {
		t.Fatalf("short vocab hint should be kept, got %q", got)
	}
}
