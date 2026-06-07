// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ToneHistoryAnalyzeRequest struct {
	SystemId    uint64 `json:"systemId"`
	TalkgroupId uint64 `json:"talkgroupId"`
	Limit       int    `json:"limit"`
	Hours       int    `json:"hours"`
}

type ToneHistorySuggestion struct {
	PatternType string   `json:"patternType"`
	PatternDesc string   `json:"patternDesc"`
	CallCount   int      `json:"callCount"`
	CallIds     []uint64 `json:"callIds"`
	Label       string   `json:"label"`
	ToneSet     ToneSet  `json:"toneSet"`
}

type ToneHistoryPartialPattern struct {
	PatternDesc string `json:"patternDesc"`
	CallCount   int    `json:"callCount"`
}

type ToneHistoryAnalyzeResponse struct {
	CallsScanned            int                         `json:"callsScanned"`
	CallsWithTones          int                         `json:"callsWithTones"`
	CallsWithCandidates     int                         `json:"callsWithCandidates"`
	DiscoverErrors          int                         `json:"discoverErrors"`
	PatternsBelowThreshold  int                         `json:"patternsBelowThreshold"`
	PartialPatterns         []ToneHistoryPartialPattern `json:"partialPatterns,omitempty"`
	CallsRequired           int                         `json:"callsRequired"`
	LookbackHours           int                         `json:"lookbackHours"`
	Suggestions             []ToneHistorySuggestion     `json:"suggestions"`
	Message                 string                      `json:"message,omitempty"`
}

const (
	toneHistoryInitialHours = 168  // 7 days
	toneHistoryInitialLimit = 200
	toneHistoryBatchLimit   = 200
	toneHistoryMaxHours     = 720  // 30 days
	toneHistoryMaxCalls     = 2000 // safety cap on total FFT work
)

type toneHistoryCallInput struct {
	callId             uint64
	audio              []byte
	audioMime          string
	audioFilename      string
	transcript         string
	reviewedTranscript string
	timestamp          int64
}

type toneHistoryAgg struct {
	cand    toneLearnCandidate
	records []toneLearnCallRecord
}

func toneLearnPatternDescription(cand toneLearnCandidate) string {
	switch cand.PatternType {
	case toneLearnPatternABPair:
		return fmt.Sprintf("Two-tone pair: A=%.1f Hz (%.2fs), B=%.1f Hz (%.2fs)",
			cand.AFrequency, cand.ADuration, cand.BFrequency, cand.BDuration)
	case toneLearnPatternLong:
		return fmt.Sprintf("Long tone: %.1f Hz for %.2fs", cand.LongFrequency, cand.LongDuration)
	default:
		return string(cand.PatternType)
	}
}

func toneHistoryAudioMime(audioMime, audioFilename string) string {
	mime := strings.TrimSpace(audioMime)
	if mime != "" {
		return mime
	}
	lower := strings.ToLower(audioFilename)
	switch {
	case strings.HasSuffix(lower, ".m4a"), strings.HasSuffix(lower, ".mp4"):
		return "audio/mp4"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	default:
		return "audio/mp4"
	}
}

func (controller *Controller) analyzeTalkgroupToneHistory(systemId, talkgroupId uint64, limit, hours int) (*ToneHistoryAnalyzeResponse, error) {
	if controller == nil || controller.Database == nil {
		return nil, fmt.Errorf("server not ready")
	}
	if systemId == 0 || talkgroupId == 0 {
		return nil, fmt.Errorf("systemId and talkgroupId are required")
	}

	sys, ok := controller.Systems.GetSystemById(systemId)
	if !ok {
		return nil, fmt.Errorf("system %d not found", systemId)
	}
	talkgroup, ok := sys.Talkgroups.GetTalkgroupById(talkgroupId)
	if !ok {
		return nil, fmt.Errorf("talkgroup %d not found on system %d", talkgroupId, systemId)
	}

	batchLimit := toneHistoryInitialLimit
	if limit > 0 {
		batchLimit = limit
	}
	if batchLimit > toneHistoryBatchLimit {
		batchLimit = toneHistoryBatchLimit
	}

	lookbackHours := toneHistoryInitialHours
	if hours > 0 {
		lookbackHours = hours
	}
	if lookbackHours > toneHistoryMaxHours {
		lookbackHours = toneHistoryMaxHours
	}

	cfg := controller.Options.AutoLearnToneSetConfig
	cfg.normalize()

	resp := &ToneHistoryAnalyzeResponse{
		CallsRequired: cfg.CallsRequired,
		LookbackHours: lookbackHours,
		Suggestions:   []ToneHistorySuggestion{},
	}

	aggregates := make(map[string]*toneHistoryAgg)
	scannedIds := make(map[uint64]bool)
	detector := NewToneDetector()
	var firstDiscoverErr string

	for {
		batch, err := controller.fetchToneHistoryCallBatch(systemId, talkgroupId, lookbackHours, batchLimit, scannedIds)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			if !toneHistoryShouldExpandLookback(resp, aggregates, cfg.CallsRequired, lookbackHours) {
				break
			}
			if lookbackHours >= toneHistoryMaxHours || resp.CallsScanned >= toneHistoryMaxCalls {
				break
			}
			lookbackHours = toneHistoryNextLookbackHours(lookbackHours)
			resp.LookbackHours = lookbackHours
			continue
		}

		for _, call := range batch {
			scannedIds[call.callId] = true
			resp.CallsScanned++

			mime := toneHistoryAudioMime(call.audioMime, call.audioFilename)
			tones, err := detector.Discover(call.audio, mime)
			if err != nil {
				resp.DiscoverErrors++
				if firstDiscoverErr == "" {
					firstDiscoverErr = fmt.Sprintf("call %d: %v", call.callId, err)
				}
				continue
			}
			if len(tones) == 0 {
				continue
			}
			resp.CallsWithTones++

			candidates := extractToneLearnCandidates(tones, cfg, systemId, talkgroupId)
			if len(candidates) == 0 {
				relaxed := relaxedAutoLearnToneSetConfig(cfg)
				if relaxed.AToneMaxDuration != cfg.AToneMaxDuration || relaxed.BToneMaxDuration != cfg.BToneMaxDuration {
					candidates = extractToneLearnCandidates(tones, relaxed, systemId, talkgroupId)
				}
			}
			if len(candidates) == 0 {
				continue
			}
			resp.CallsWithCandidates++

			stackedCall := len(candidates) > 1
			transcriptText := ""
			if call.reviewedTranscript != "" {
				transcriptText = strings.ToUpper(call.reviewedTranscript)
			} else if call.transcript != "" {
				transcriptText = strings.ToUpper(call.transcript)
			}

			for _, cand := range candidates {
				if toneSetExistsOnTalkgroup(talkgroup.ToneSets, cand, cfg.FrequencyToleranceHz) {
					continue
				}
				agg, exists := aggregates[cand.SignatureHash]
				if !exists {
					aggregates[cand.SignatureHash] = &toneHistoryAgg{cand: cand}
					agg = aggregates[cand.SignatureHash]
				}
				dup := false
				for _, r := range agg.records {
					if r.CallId == call.callId {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				agg.records = append(agg.records, toneLearnCallRecord{
					CallId:      call.callId,
					Transcript:  transcriptText,
					Timestamp:   call.timestamp,
					StackedCall: stackedCall,
				})
			}
		}

		controller.buildToneHistorySuggestions(sys, talkgroup, cfg, aggregates, resp)

		if len(resp.Suggestions) > 0 {
			break
		}
		if !toneHistoryShouldExpandLookback(resp, aggregates, cfg.CallsRequired, lookbackHours) {
			break
		}
		if resp.CallsScanned >= toneHistoryMaxCalls {
			break
		}
		if lookbackHours >= toneHistoryMaxHours {
			break
		}
		lookbackHours = toneHistoryNextLookbackHours(lookbackHours)
		resp.LookbackHours = lookbackHours
	}

	controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf(
		"tone history analyze: system=%d talkgroup=%d (TGID %d) lookbackHours=%d scanned=%d withTones=%d withCandidates=%d discoverErrors=%d belowThreshold=%d suggestions=%d aDur=%.1f-%.1f bDur=%.1f-%.1f callsRequired=%d%s",
		systemId, talkgroupId, talkgroup.TalkgroupRef, resp.LookbackHours,
		resp.CallsScanned, resp.CallsWithTones, resp.CallsWithCandidates, resp.DiscoverErrors,
		resp.PatternsBelowThreshold, len(resp.Suggestions),
		cfg.AToneMinDuration, cfg.AToneMaxDuration, cfg.BToneMinDuration, cfg.BToneMaxDuration, cfg.CallsRequired,
		func() string {
			if firstDiscoverErr != "" {
				return " firstDiscoverErr=" + firstDiscoverErr
			}
			return ""
		}(),
	))

	if len(resp.Suggestions) == 0 {
		lookbackLabel := toneHistoryLookbackLabel(resp.LookbackHours)
		switch {
		case resp.CallsScanned == 0:
			resp.Message = fmt.Sprintf("No calls with audio found in %s for this talkgroup.", lookbackLabel)
		case resp.CallsWithTones == 0:
			resp.Message = fmt.Sprintf(
				"Scanned %d calls (%s) but FFT found no sustained tones (discover errors: %d). Stored audio may be voice-only or ffmpeg could not decode calls.",
				resp.CallsScanned, lookbackLabel, resp.DiscoverErrors,
			)
		case resp.CallsWithCandidates == 0:
			hint := ""
			relaxed := relaxedAutoLearnToneSetConfig(cfg)
			if relaxed.AToneMaxDuration > cfg.AToneMaxDuration || relaxed.BToneMaxDuration > cfg.BToneMaxDuration {
				hint = fmt.Sprintf(
					" Many Ohio paging tones are ~1.0s A and ~3s B — try A max %.1fs and B max %.1fs in Admin → Options.",
					relaxed.AToneMaxDuration, relaxed.BToneMaxDuration,
				)
			}
			resp.Message = fmt.Sprintf(
				"Scanned %d calls (%s), %d had raw tones, but none matched auto-learn duration windows (A %.1f–%.1fs, B %.1f–%.1fs, long ≥%.1fs).%s",
				resp.CallsScanned, lookbackLabel, resp.CallsWithTones, cfg.AToneMinDuration, cfg.AToneMaxDuration, cfg.BToneMinDuration, cfg.BToneMaxDuration, cfg.LongToneMinDuration, hint,
			)
		case resp.PatternsBelowThreshold > 0:
			resp.Message = fmt.Sprintf(
				"Found %d pattern(s) but none reached %d calls yet (scanned %d calls in %s, %d with matching A+B/long patterns). See partial patterns below.",
				resp.PatternsBelowThreshold, cfg.CallsRequired, resp.CallsScanned, lookbackLabel, resp.CallsWithCandidates,
			)
		default:
			resp.Message = fmt.Sprintf(
				"No new tone patterns with at least %d matching calls in %s (scanned %d calls, %d with tones).",
				cfg.CallsRequired, lookbackLabel, resp.CallsScanned, resp.CallsWithTones,
			)
		}
	}

	return resp, nil
}

func toneHistoryNextLookbackHours(current int) int {
	next := current * 2
	if next > toneHistoryMaxHours {
		return toneHistoryMaxHours
	}
	return next
}

func toneHistoryLookbackLabel(hours int) string {
	days := hours / 24
	if days >= 1 && hours%24 == 0 {
		if days == 1 {
			return "the last day"
		}
		return fmt.Sprintf("the last %d days", days)
	}
	return fmt.Sprintf("the last %d hours", hours)
}

func toneHistoryShouldExpandLookback(resp *ToneHistoryAnalyzeResponse, aggregates map[string]*toneHistoryAgg, callsRequired, lookbackHours int) bool {
	if len(resp.Suggestions) > 0 {
		return false
	}
	if resp.CallsScanned >= toneHistoryMaxCalls {
		return false
	}
	if lookbackHours >= toneHistoryMaxHours {
		return false
	}
	for _, agg := range aggregates {
		if len(agg.records) > 0 && len(agg.records) < callsRequired {
			return true
		}
	}
	if resp.PatternsBelowThreshold > 0 {
		return true
	}
	if resp.CallsWithCandidates == 0 {
		return true
	}
	return false
}

func (controller *Controller) fetchToneHistoryCallBatch(systemId, talkgroupId uint64, hours, limit int, exclude map[uint64]bool) ([]toneHistoryCallInput, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour).UnixMilli()
	fetchLimit := limit * 3
	if fetchLimit < limit+50 {
		fetchLimit = limit + 50
	}
	if fetchLimit > 600 {
		fetchLimit = 600
	}

	var query string
	if controller.Database.Config.DbType == DbTypePostgresql {
		query = `SELECT "callId", "audio", "audioMime", "audioFilename", "transcript", "reviewedTranscript", "timestamp" FROM "calls" WHERE "systemId" = $1 AND "talkgroupId" = $2 AND "timestamp" >= $3 AND length("audio") > 0 ORDER BY "timestamp" DESC LIMIT $4`
	} else {
		query = `SELECT "callId", "audio", "audioMime", "audioFilename", "transcript", "reviewedTranscript", "timestamp" FROM "calls" WHERE "systemId" = ? AND "talkgroupId" = ? AND "timestamp" >= ? AND length("audio") > 0 ORDER BY "timestamp" DESC LIMIT ?`
	}

	rows, err := controller.Database.Sql.Query(query, systemId, talkgroupId, since, fetchLimit)
	if err != nil {
		return nil, fmt.Errorf("query calls: %w", err)
	}
	defer rows.Close()

	out := make([]toneHistoryCallInput, 0, limit)
	for rows.Next() {
		var (
			callId             uint64
			audio              []byte
			audioMime          string
			audioFilename      string
			transcript         sql.NullString
			reviewedTranscript sql.NullString
			timestamp          int64
		)
		if err := rows.Scan(&callId, &audio, &audioMime, &audioFilename, &transcript, &reviewedTranscript, &timestamp); err != nil {
			return nil, fmt.Errorf("scan call: %w", err)
		}
		if exclude[callId] {
			continue
		}
		row := toneHistoryCallInput{
			callId:        callId,
			audio:         audio,
			audioMime:     audioMime,
			audioFilename: audioFilename,
			timestamp:     timestamp,
		}
		if reviewedTranscript.Valid {
			row.reviewedTranscript = strings.TrimSpace(reviewedTranscript.String)
		}
		if transcript.Valid {
			row.transcript = strings.TrimSpace(transcript.String)
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calls: %w", err)
	}
	return out, nil
}

func (controller *Controller) buildToneHistorySuggestions(sys *System, talkgroup *Talkgroup, cfg AutoLearnToneSetConfig, aggregates map[string]*toneHistoryAgg, resp *ToneHistoryAnalyzeResponse) {
	resp.Suggestions = []ToneHistorySuggestion{}
	resp.PartialPatterns = []ToneHistoryPartialPattern{}
	resp.PatternsBelowThreshold = 0

	for _, agg := range aggregates {
		if len(agg.records) < cfg.CallsRequired {
			resp.PatternsBelowThreshold++
			if len(resp.PartialPatterns) < 10 {
				resp.PartialPatterns = append(resp.PartialPatterns, ToneHistoryPartialPattern{
					PatternDesc: toneLearnPatternDescription(agg.cand),
					CallCount:   len(agg.records),
				})
			}
			continue
		}
		if toneLearnCandidateNeedsReview(agg.records) {
			continue
		}
		if toneSetExistsOnTalkgroup(talkgroup.ToneSets, agg.cand, cfg.FrequencyToleranceHz) {
			continue
		}

		label := controller.suggestToneLearnLabel(sys, talkgroup, agg.cand, agg.records)
		if label == "" || label == "UNKNOWN" {
			label = fmt.Sprintf("Learned %s", strings.ToUpper(string(agg.cand.PatternType)))
		}

		toneSet := agg.cand.ToneSetDraft
		toneSet.Label = label

		callIds := make([]uint64, len(agg.records))
		for i, r := range agg.records {
			callIds[i] = r.CallId
		}

		resp.Suggestions = append(resp.Suggestions, ToneHistorySuggestion{
			PatternType: string(agg.cand.PatternType),
			PatternDesc: toneLearnPatternDescription(agg.cand),
			CallCount:   len(agg.records),
			CallIds:     callIds,
			Label:       label,
			ToneSet:     toneSet,
		})
	}
}

func (admin *Admin) ToneHistoryAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	token := admin.GetAuthorization(r)
	if !admin.ValidateToken(token) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req ToneHistoryAnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := admin.Controller.analyzeTalkgroupToneHistory(req.SystemId, req.TalkgroupId, req.Limit, req.Hours)
	if err != nil {
		admin.Controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("tone history analyze failed: %s", err.Error()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, escapeQuotes(err.Error()))))
		return
	}

	if b, err := json.Marshal(result); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
