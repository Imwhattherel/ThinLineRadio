// Copyright (C) 2025 Thinline Dynamic Solutions
//
// One-off: Discover tones on all LFD calls (talkgroupId 2764) from local Postgres.

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func tightAutoLearnToneSetConfig() AutoLearnToneSetConfig {
	cfg := DefaultAutoLearnToneSetConfig()
	cfg.AToneMaxDuration = 0.9
	cfg.BToneMaxDuration = 2.5
	cfg.normalize()
	return cfg
}

func formatToneDetails(tones []Tone) string {
	if len(tones) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(tones))
	for i, t := range tones {
		parts = append(parts, fmt.Sprintf("#%d %.2fHz dur=%.3fs start=%.3fs end=%.3fs", i+1, t.Frequency, t.Duration, t.StartTime, t.EndTime))
	}
	return strings.Join(parts, "\n    ")
}

func TestDiscoverLFDAll20FromDB(t *testing.T) {
	dsn := os.Getenv("TLR_TONE_DEBUG_DSN")
	if dsn == "" {
		dsn = "postgresql://michaelchambers:asdfasd5456456df@localhost:5432/rdio_scanner"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	var optValue string
	err = db.QueryRow(`SELECT value FROM options WHERE key = 'autoLearnToneSetConfig' LIMIT 1`).Scan(&optValue)
	if err != nil {
		t.Logf("options autoLearnToneSetConfig: query err %v (using defaults only)", err)
	} else {
		t.Logf("DB options.autoLearnToneSetConfig: %s", optValue)
	}

	const systemId, talkgroupId uint64 = 44, 2764
	defaultCfg := DefaultAutoLearnToneSetConfig()
	defaultCfg.normalize()
	tightCfg := tightAutoLearnToneSetConfig()

	t.Logf("defaultCfg: A=[%.2f,%.2f]s B=[%.2f,%.2f]s", defaultCfg.AToneMinDuration, defaultCfg.AToneMaxDuration, defaultCfg.BToneMinDuration, defaultCfg.BToneMaxDuration)
	t.Logf("tightCfg:   A=[%.2f,%.2f]s B=[%.2f,%.2f]s", tightCfg.AToneMinDuration, tightCfg.AToneMaxDuration, tightCfg.BToneMinDuration, tightCfg.BToneMaxDuration)

	rows, err := db.Query(`
		SELECT "callId", "audio", "audioMime", "audioFilename", length("audio") AS audio_len
		FROM calls
		WHERE "systemId" = $1 AND "talkgroupId" = $2
		ORDER BY "callId"`, systemId, talkgroupId)
	if err != nil {
		t.Fatalf("query calls: %v", err)
	}
	defer rows.Close()

	type row struct {
		callId        uint64
		audio         []byte
		audioMime     string
		audioFilename string
		audioLen      int
	}
	var calls []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.callId, &r.audio, &r.audioMime, &r.audioFilename, &r.audioLen); err != nil {
			t.Fatalf("scan: %v", err)
		}
		calls = append(calls, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	t.Logf("calls loaded: %d (talkgroupId %d)", len(calls), talkgroupId)

	detector := NewToneDetector()
	sort.Slice(calls, func(i, j int) bool { return calls[i].callId < calls[j].callId })

	for _, c := range calls {
		mime := toneHistoryAudioMime(c.audioMime, c.audioFilename)
		marker := ""
		if c.callId == 45383606 {
			marker = " *** FOCUS callId 45383606 ***"
		}
		t.Logf("\n========== callId=%d%s ==========", c.callId, marker)
		t.Logf("  audioFilename=%q audioMime=%q resolvedMime=%q bytes=%d", c.audioFilename, c.audioMime, mime, c.audioLen)

		tones, derr := detector.Discover(c.audio, mime)
		if derr != nil {
			t.Logf("  Discover ERROR: %v", derr)
			continue
		}
		t.Logf("  tone count: %d", len(tones))
		t.Logf("  tones:\n    %s", formatToneDetails(tones))

		defCands := extractToneLearnCandidates(tones, defaultCfg, systemId, talkgroupId)
		tightCands := extractToneLearnCandidates(tones, tightCfg, systemId, talkgroupId)
		t.Logf("  candidates (default A=%.1f-%.1f B=%.1f-%.1f): %d [%s]",
			defaultCfg.AToneMinDuration, defaultCfg.AToneMaxDuration,
			defaultCfg.BToneMinDuration, defaultCfg.BToneMaxDuration,
			len(defCands), formatCandidates(defCands))
		t.Logf("  candidates (tight A=%.1f-%.1f B=%.1f-%.1f): %d [%s]",
			tightCfg.AToneMinDuration, tightCfg.AToneMaxDuration,
			tightCfg.BToneMinDuration, tightCfg.BToneMaxDuration,
			len(tightCands), formatCandidates(tightCands))

		if c.callId == 45383606 {
			b, _ := json.MarshalIndent(map[string]any{
				"tones":            tones,
				"defaultCandidates": defCands,
				"tightCandidates":   tightCands,
			}, "", "  ")
			t.Logf("  45383606 JSON dump:\n%s", string(b))
		}
	}
}
