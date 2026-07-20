// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"rdio-scanner/server/mapping"
)

const callNatureBackfillBatchSize = 10

type callNatureBackfillStats struct {
	PhrasesAdded        int
	PhrasesPruned       int
	UnknownPhrasesReset int
	CallsScanned        int
	CallsSkipped        int
	CallsUpdated        int
	CallsClassified     int
	CallsCleared        int
	OpenAIBatches       int
}

type pendingNatureCall struct {
	callID     int64
	transcript string
	oldNature  string
}

func loadCallNatureMatchData(db *Database) CallNatureMatchData {
	cache := NewCallNaturesCache(nil)
	_ = cache.Read(db)
	openAI := false
	if db != nil && db.Sql != nil {
		var raw string
		if err := db.Sql.QueryRow(`SELECT "value" FROM "options" WHERE "key"='mappingIntegration'`).Scan(&raw); err == nil && raw != "" {
			var m map[string]any
			if json.Unmarshal([]byte(raw), &m) == nil {
				if v, ok := m["callNatureOpenAIClassify"].(bool); ok {
					openAI = v
				}
				if !openAI {
					if eng, ok := m["mappingEngine"].(string); ok && strings.EqualFold(eng, "local") {
						openAI = loadOpenAIKeyConfigured(db)
					}
				}
			}
		} else if loadOpenAIKeyConfigured(db) {
			openAI = true
		}
	}
	return cache.MatchData(openAI)
}

func loadOpenAIKeyConfigured(db *Database) bool {
	if db == nil || db.Sql == nil {
		return false
	}
	var raw string
	if err := db.Sql.QueryRow(`SELECT "value" FROM "options" WHERE "key"='openAIIntegration'`).Scan(&raw); err != nil {
		return false
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return false
	}
	v, _ := m["apiKey"].(string)
	return strings.TrimSpace(v) != ""
}

func loadOpenAIClassifyCredentials(db *Database) (apiKey, model string, classify bool) {
	if db == nil || db.Sql == nil {
		return "", "", false
	}
	data := loadCallNatureMatchData(db)
	classify = data.OpenAIClassify
	var raw string
	if err := db.Sql.QueryRow(`SELECT "value" FROM "options" WHERE "key"='openAIIntegration'`).Scan(&raw); err == nil && raw != "" {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			apiKey, _ = m["apiKey"].(string)
			if v, ok := m["model"].(string); ok && strings.TrimSpace(v) != "" {
				model = v
			}
		}
	}
	if model == "" {
		var mi string
		if err := db.Sql.QueryRow(`SELECT "value" FROM "options" WHERE "key"='mappingIntegration'`).Scan(&mi); err == nil && mi != "" {
			var m map[string]any
			if json.Unmarshal([]byte(mi), &m) == nil {
				if v, ok := m["openAIModel"].(string); ok {
					model = v
				}
			}
		}
	}
	return strings.TrimSpace(apiKey), strings.TrimSpace(model), classify
}

func classifyCallNaturePhraseOnly(transcript string, data CallNatureMatchData) string {
	return mapping.ResolveIncidentNature(transcript, mapping.ProcessInput{
		NatureCodes:              data.Labels,
		NatureMatchTerms:         data.MatchTerms,
		NaturePhraseToLabel:      data.PhraseToLabel,
		CallNatureOpenAIClassify: false,
	})
}

func backfillNatureUpdateAllowed(oldNature, newNature string) bool {
	old := strings.ToUpper(strings.TrimSpace(oldNature))
	newNat := strings.ToUpper(strings.TrimSpace(newNature))
	if newNat == "" {
		return old != ""
	}
	if mapping.IsDefaultUnknownNatureLabel(newNat) && old != "" && !mapping.IsDefaultUnknownNatureLabel(old) {
		return false
	}
	return old != newNat
}

func markCallNatureReviewed(db *Database, callID int64) error {
	ts := time.Now().UnixMilli()
	_, err := db.Sql.Exec(
		`UPDATE "calls" SET "incidentNatureReviewedAt" = $1 WHERE "callId" = $2 AND "incidentNatureReviewedAt" = 0`,
		ts, callID,
	)
	return err
}

func mergeCallNaturePhrases(db *Database, additions map[string][]string) (int, error) {
	if db == nil || db.Sql == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	total := 0
	for label, phrases := range additions {
		label = strings.ToUpper(strings.TrimSpace(label))
		if label == "" || len(phrases) == 0 || mapping.IsDefaultUnknownNatureLabel(label) {
			continue
		}
		var id int64
		var phrasesJSON string
		err := db.Sql.QueryRow(
			`SELECT "callNatureId", "phrases" FROM "callNatures" WHERE "label" = $1`, label,
		).Scan(&id, &phrasesJSON)
		if err != nil {
			continue
		}
		existing := map[string]bool{label: true}
		var current []string
		if phrasesJSON != "" && phrasesJSON != "[]" {
			_ = json.Unmarshal([]byte(phrasesJSON), &current)
		}
		for _, p := range current {
			existing[strings.ToUpper(strings.TrimSpace(p))] = true
		}
		changed := false
		for _, p := range phrases {
			p = strings.ToUpper(strings.TrimSpace(p))
			if p == "" || existing[p] || !mapping.IsAcceptableCallNaturePhrase(p) ||
				misleadingCallNaturePhrase(label, p) {
				continue
			}
			existing[p] = true
			current = append(current, p)
			total++
			changed = true
		}
		if !changed {
			continue
		}
		sort.Strings(current)
		b, _ := json.Marshal(current)
		if _, err := db.Sql.Exec(
			`UPDATE "callNatures" SET "phrases" = $1 WHERE "callNatureId" = $2`,
			string(b), id,
		); err != nil {
			return total, fmt.Errorf("update %q: %w", label, err)
		}
	}
	return total, nil
}

func pruneCallNaturePhrases(db *Database) (removed int, err error) {
	if db == nil || db.Sql == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	rows, err := db.Sql.Query(`SELECT "callNatureId", "label", "phrases" FROM "callNatures"`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var label, phrasesJSON string
		if rows.Scan(&id, &label, &phrasesJSON) != nil {
			continue
		}
		label = strings.ToUpper(strings.TrimSpace(label))
		var current []string
		if phrasesJSON != "" && phrasesJSON != "[]" {
			_ = json.Unmarshal([]byte(phrasesJSON), &current)
		}
		var kept []string
		seen := map[string]bool{}
		for _, p := range current {
			p = strings.ToUpper(strings.TrimSpace(p))
			if p == "" || seen[p] {
				continue
			}
			if !mapping.IsAcceptableCallNaturePhrase(p) || misleadingCallNaturePhrase(label, p) {
				removed++
				continue
			}
			seen[p] = true
			kept = append(kept, p)
		}
		if len(kept) == len(current) {
			continue
		}
		sort.Strings(kept)
		b, _ := json.Marshal(kept)
		if _, err := db.Sql.Exec(
			`UPDATE "callNatures" SET "phrases" = $1 WHERE "callNatureId" = $2`,
			string(b), id,
		); err != nil {
			return removed, err
		}
	}
	return removed, rows.Err()
}

// resetUnknownNaturePhrases keeps only canonical catch-all aliases on UNKNOWN labels.
func resetUnknownNaturePhrases(db *Database) (int, error) {
	if db == nil || db.Sql == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	allowed := supplementalCallNaturePhrases()
	reset := 0
	for _, label := range []string{
		"UNKNOWN PROBLEM", "EMS UNKNOWN PROBLEM/UNCLASSIFIED", "UNKNOWN PROBLEM/UNCLASSIFIED",
	} {
		phrases, ok := allowed[label]
		if !ok {
			phrases = []string{label}
		}
		seen := map[string]bool{}
		var kept []string
		for _, p := range append([]string{label}, phrases...) {
			p = strings.ToUpper(strings.TrimSpace(p))
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			kept = append(kept, p)
		}
		sort.Strings(kept)
		b, _ := json.Marshal(kept)
		res, err := db.Sql.Exec(`UPDATE "callNatures" SET "phrases" = $1 WHERE "label" = $2`, string(b), label)
		if err != nil {
			return reset, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			reset++
		}
	}
	return reset, nil
}

func applyNatureBackfillResult(db *Database, call pendingNatureCall, nature string, stats *callNatureBackfillStats) error {
	ts := time.Now().UnixMilli()
	if nature == "" {
		if strings.TrimSpace(call.oldNature) != "" {
			if _, err := db.Sql.Exec(
				`UPDATE "calls" SET "incidentNature" = '', "incidentNatureReviewedAt" = $1 WHERE "callId" = $2`,
				ts, call.callID,
			); err != nil {
				return err
			}
			stats.CallsCleared++
			stats.CallsUpdated++
			return nil
		}
		return markCallNatureReviewed(db, call.callID)
	}
	stats.CallsClassified++
	newNat := strings.ToUpper(strings.TrimSpace(nature))
	if !backfillNatureUpdateAllowed(call.oldNature, newNat) {
		return markCallNatureReviewed(db, call.callID)
	}
	if _, err := db.Sql.Exec(
		`UPDATE "calls" SET "incidentNature" = $1, "incidentNatureReviewedAt" = $2 WHERE "callId" = $3`,
		newNat, ts, call.callID,
	); err != nil {
		return err
	}
	stats.CallsUpdated++
	return nil
}

func flushNatureBackfillBatch(db *Database, batch []pendingNatureCall, data CallNatureMatchData, openAIKey, openAIModel string, useOpenAI bool, stats *callNatureBackfillStats) error {
	if len(batch) == 0 {
		return nil
	}
	ids := make([]string, len(batch))
	for i, call := range batch {
		ids[i] = strconv.FormatInt(call.callID, 10)
	}
	log.Printf("[INFO] call nature backfill: processing callIds=%s", strings.Join(ids, ","))

	natures := make([]string, len(batch))
	for i, call := range batch {
		natures[i] = classifyCallNaturePhraseOnly(call.transcript, data)
	}
	if useOpenAI && data.OpenAIClassify && strings.TrimSpace(openAIKey) != "" {
		var need []pendingNatureCall
		var needIdx []int
		var needTranscripts []string
		for i, call := range batch {
			if strings.TrimSpace(natures[i]) != "" && !mapping.IsDefaultUnknownNatureLabel(natures[i]) {
				continue
			}
			need = append(need, call)
			needIdx = append(needIdx, i)
			needTranscripts = append(needTranscripts, call.transcript)
		}
		for start := 0; start < len(needTranscripts); start += callNatureBackfillBatchSize {
			end := start + callNatureBackfillBatchSize
			if end > len(needTranscripts) {
				end = len(needTranscripts)
			}
			stats.OpenAIBatches++
			picks := mapping.ClassifyCallNatureWithOpenAIBatch(openAIKey, openAIModel, needTranscripts[start:end], data.Labels)
			for j, pick := range picks {
				if strings.TrimSpace(pick) == "" {
					continue
				}
				natures[needIdx[start+j]] = pick
			}
		}
	}
	for i, call := range batch {
		nature := mapping.SanitizeCallNatureAssignment(call.transcript, natures[i], data.Labels)
		if strings.TrimSpace(nature) == "" && mapping.ShouldApplyUnknownNatureFallback(call.transcript) {
			nature = mapping.DefaultUnknownProblemLabel(data.Labels)
			nature = mapping.SanitizeCallNatureAssignment(call.transcript, nature, data.Labels)
		}
		if err := applyNatureBackfillResult(db, call, nature, stats); err != nil {
			return err
		}
	}
	return nil
}

func backfillCallIncidentNatures(db *Database, data CallNatureMatchData, useOpenAI bool, maxCalls int) (callNatureBackfillStats, error) {
	var stats callNatureBackfillStats
	openAIKey, openAIModel, _ := loadOpenAIClassifyCredentials(db)
	rows, err := db.Sql.Query(`
		SELECT "callId", COALESCE("transcript", ''), COALESCE("incidentNature", '')
		FROM "calls"
		WHERE COALESCE("transcript", '') <> ''
		  AND "incidentNatureReviewedAt" = 0
		  AND (
		    COALESCE("incidentNature", '') = ''
		    OR "incidentNature" ILIKE '%UNKNOWN%'
		  )
		ORDER BY
		  CASE WHEN "incidentNature" ILIKE '%UNKNOWN%' THEN 0 ELSE 1 END,
		  "callId"`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	batch := make([]pendingNatureCall, 0, callNatureBackfillBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := flushNatureBackfillBatch(db, batch, data, openAIKey, openAIModel, useOpenAI, &stats); err != nil {
			return err
		}
		log.Printf("[INFO] call nature backfill: batch done updated=%d classified=%d cleared=%d skipped=%d openai_batches=%d",
			stats.CallsUpdated, stats.CallsClassified, stats.CallsCleared, stats.CallsSkipped, stats.OpenAIBatches)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		var call pendingNatureCall
		if rows.Scan(&call.callID, &call.transcript, &call.oldNature) != nil {
			continue
		}
		if !mapping.ShouldEnqueueCallNatureBackfill(call.oldNature, call.transcript) {
			stats.CallsSkipped++
			if err := markCallNatureReviewed(db, call.callID); err != nil {
				return stats, err
			}
			if mapping.IsDefaultUnknownNatureLabel(call.oldNature) {
				if _, err := db.Sql.Exec(
					`UPDATE "calls" SET "incidentNature" = '', "incidentNatureReviewedAt" = $1 WHERE "callId" = $2`,
					time.Now().UnixMilli(), call.callID,
				); err != nil {
					return stats, err
				}
				stats.CallsCleared++
				stats.CallsUpdated++
			}
			continue
		}
		if maxCalls > 0 && stats.CallsScanned >= maxCalls {
			break
		}
		stats.CallsScanned++
		batch = append(batch, call)
		if len(batch) >= callNatureBackfillBatchSize {
			if err := flush(); err != nil {
				return stats, err
			}
		}
	}
	if err := flush(); err != nil {
		return stats, err
	}
	return stats, rows.Err()
}

func backfillPhraseMaintenanceEnabled() bool {
	return os.Getenv("TLR_BACKFILL_NATURE_RESET") == "1"
}

func runCallNatureBackfill(db *Database, useOpenAI bool, maxCalls int) (callNatureBackfillStats, error) {
	var stats callNatureBackfillStats

	if backfillPhraseMaintenanceEnabled() {
		pruned, err := pruneCallNaturePhrases(db)
		if err != nil {
			return stats, fmt.Errorf("prune phrases: %w", err)
		}
		stats.PhrasesPruned = pruned

		reset, err := resetUnknownNaturePhrases(db)
		if err != nil {
			return stats, fmt.Errorf("reset unknown phrases: %w", err)
		}
		stats.UnknownPhrasesReset = reset

		added, err := mergeCallNaturePhrases(db, supplementalCallNaturePhrases())
		if err != nil {
			return stats, fmt.Errorf("merge supplemental phrases: %w", err)
		}
		stats.PhrasesAdded = added
	}

	data := loadCallNatureMatchData(db)
	callStats, err := backfillCallIncidentNatures(db, data, useOpenAI, maxCalls)
	if err != nil {
		return stats, err
	}
	stats.CallsScanned = callStats.CallsScanned
	stats.CallsSkipped = callStats.CallsSkipped
	stats.CallsUpdated = callStats.CallsUpdated
	stats.CallsClassified = callStats.CallsClassified
	stats.CallsCleared = callStats.CallsCleared
	stats.OpenAIBatches = callStats.OpenAIBatches
	return stats, nil
}

func backfillMaxCallsFromEnv() int {
	if v := strings.TrimSpace(os.Getenv("TLR_BACKFILL_NATURE_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// cleanupReviewedUnknownNatures clears catch-all labels on already-reviewed calls
// when updated guards show the transcript is chatter or incomplete.
func cleanupReviewedUnknownNatures(db *Database, data CallNatureMatchData) (cleared int, err error) {
	if db == nil || db.Sql == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	rows, err := db.Sql.Query(`
		SELECT "callId", COALESCE("transcript", ''), COALESCE("incidentNature", '')
		FROM "calls"
		WHERE "incidentNatureReviewedAt" > 0
		  AND "incidentNature" ILIKE '%UNKNOWN%'`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	ts := time.Now().UnixMilli()
	for rows.Next() {
		var callID int64
		var transcript, nature string
		if rows.Scan(&callID, &transcript, &nature) != nil {
			continue
		}
		clean := mapping.SanitizeCallNatureAssignment(transcript, nature, data.Labels)
		if clean != "" || !mapping.IsDefaultUnknownNatureLabel(nature) {
			continue
		}
		if _, err := db.Sql.Exec(
			`UPDATE "calls" SET "incidentNature" = '', "incidentNatureReviewedAt" = $1 WHERE "callId" = $2`,
			ts, callID,
		); err != nil {
			return cleared, err
		}
		cleared++
	}
	return cleared, rows.Err()
}
