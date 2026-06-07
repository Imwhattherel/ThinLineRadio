// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func (controller *Controller) sendToneLearnReviewEmail(system *System, talkgroup *Talkgroup, cand toneLearnCandidate, records []toneLearnCallRecord, cfg AutoLearnToneSetConfig) {
	if controller.EmailService == nil {
		controller.Logs.LogEvent(LogLevelWarn, "tone auto-learn: email service unavailable")
		return
	}

	label := controller.suggestToneLearnLabel(system, talkgroup, cand, records)

	// Mark emailed before send to avoid duplicate storms on retry races
	now := time.Now().UnixMilli()
	updateQuery := `UPDATE "toneSetLearnCandidates" SET "reviewEmailedAt" = $1 WHERE "systemId" = $2 AND "talkgroupId" = $3 AND "signatureHash" = $4 AND ("reviewEmailedAt" IS NULL OR "reviewEmailedAt" = 0)`
	if controller.Database.Config.DbType != DbTypePostgresql {
		updateQuery = `UPDATE "toneSetLearnCandidates" SET "reviewEmailedAt" = ? WHERE "systemId" = ? AND "talkgroupId" = ? AND "signatureHash" = ? AND ("reviewEmailedAt" IS NULL OR "reviewEmailedAt" = 0)`
	}
	res, err := controller.Database.Sql.Exec(updateQuery, now, system.Id, talkgroup.Id, cand.SignatureHash)
	if err != nil {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("tone auto-learn: mark emailed failed: %v", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return
	}

	patternLine := ""
	switch cand.PatternType {
	case toneLearnPatternABPair:
		patternLine = fmt.Sprintf("A+B pair — A: %.1f Hz (%.2fs), B: %.1f Hz (%.2fs)", cand.AFrequency, cand.ADuration, cand.BFrequency, cand.BDuration)
	case toneLearnPatternLong:
		patternLine = fmt.Sprintf("Long tone — %.1f Hz (%.2fs)", cand.LongFrequency, cand.LongDuration)
	}

	var transcriptHTML strings.Builder
	for i, r := range records {
		transcriptHTML.WriteString(fmt.Sprintf("<h3>Call %d (ID %d)</h3><pre style=\"white-space:pre-wrap;background:#f4f4f4;padding:12px;border-radius:6px;\">%s</pre>", i+1, r.CallId, escapeHTML(r.Transcript)))
	}

	subjectLabel := label
	if label == "UNKNOWN" {
		subjectLabel = "Name unclear"
	}
	subject := fmt.Sprintf("[TLR] Tone learn review — %s / %s — %s", system.Label, talkgroup.Label, subjectLabel)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:sans-serif;max-width:720px;margin:0 auto;padding:20px;">
<h2>Tone set learn review</h2>
<p><strong>Stacked tones detected</strong> (multiple tone patterns on the same voice call). No tone set was added automatically — review the samples below and add tone sets manually in admin if appropriate.</p>
<ul>
<li><strong>System:</strong> %s</li>
<li><strong>Talkgroup:</strong> %s (TGID %d)</li>
<li><strong>Pattern:</strong> %s</li>
<li><strong>Suggested name (OpenAI):</strong> %s</li>
</ul>
%s
<p style="color:#666;font-size:13px;">MP3 recordings from each call are attached.</p>
</body></html>`,
		escapeHTML(system.Label),
		escapeHTML(talkgroup.Label),
		talkgroup.TalkgroupRef,
		escapeHTML(patternLine),
		escapeHTML(label),
		transcriptHTML.String(),
	)

	var attachments []EmailAttachment
	for _, r := range records {
		call, err := controller.Calls.GetCall(r.CallId)
		if err != nil || call == nil || len(call.Audio) == 0 {
			continue
		}
		mp3, err := encodeCallAudioToMP3(call.Audio, call.AudioMime)
		if err != nil {
			controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("tone auto-learn: mp3 encode failed for call %d: %v", r.CallId, err))
			continue
		}
		attachments = append(attachments, EmailAttachment{
			Filename:    fmt.Sprintf("call-%d.mp3", r.CallId),
			ContentType: "audio/mpeg",
			Data:        mp3,
		})
	}

	adminEmails := controller.getSystemAdminEmails()
	if len(adminEmails) == 0 {
		controller.Logs.LogEvent(LogLevelWarn, "tone auto-learn: no system admin emails found")
		return
	}

	for _, email := range adminEmails {
		if err := controller.EmailService.SendEmailWithAttachments(email, subject, htmlBody, attachments); err != nil {
			controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("tone auto-learn: email to %s failed: %v", email, err))
		}
	}

	controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("tone auto-learn: review email sent for talkgroup %d signature %s (label=%s)", talkgroup.TalkgroupRef, cand.SignatureHash[:8], label))
}

func (controller *Controller) getSystemAdminEmails() []string {
	rows, err := controller.Database.Sql.Query(`SELECT "email" FROM "users" WHERE "systemAdmin" = true AND "email" != ''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err == nil && email != "" {
			emails = append(emails, email)
		}
	}
	return emails
}

func encodeCallAudioToMP3(audio []byte, mime string) ([]byte, error) {
	ffArgs := []string{
		"-i", "pipe:0",
		"-vn",
		"-acodec", "libmp3lame",
		"-q:a", "4",
		"-f", "mp3",
		"-loglevel", "error",
		"pipe:1",
	}
	cmd := exec.Command("ffmpeg", ffArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		defer stdin.Close()
		stdin.Write(audio)
	}()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%v: %s", err, stderr.String())
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty mp3")
	}
	return stdout.Bytes(), nil
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
