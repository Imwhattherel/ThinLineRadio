// Copyright (C) 2025 Thinline Dynamic Solutions

package main

import (
	"fmt"
	"strings"
	"time"
)

func (controller *Controller) sendUnitLearnReviewEmail(system *System, talkgroup *Talkgroup, unitRef uint, records []unitLearnCallRecord, suggestedLabel string, labelConflict bool) {
	if controller.EmailService == nil {
		controller.Logs.LogEvent(LogLevelWarn, "unit auto-learn: email service unavailable")
		return
	}

	now := time.Now().UnixMilli()
	updateQuery := `UPDATE "unitAliasLearnCandidates" SET "finalizedAt" = $1 WHERE "systemId" = $2 AND "talkgroupId" = $3 AND "unitRef" = $4 AND ("finalizedAt" IS NULL OR "finalizedAt" = 0)`
	if controller.Database.Config.DbType != DbTypePostgresql {
		updateQuery = `UPDATE "unitAliasLearnCandidates" SET "finalizedAt" = ? WHERE "systemId" = ? AND "talkgroupId" = ? AND "unitRef" = ? AND ("finalizedAt" IS NULL OR "finalizedAt" = 0)`
	}
	res, err := controller.Database.Sql.Exec(updateQuery, now, system.Id, talkgroup.Id, unitRef)
	if err != nil {
		controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("unit auto-learn: mark review emailed failed: %v", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return
	}

	reason := "OpenAI could not determine a consistent unit label."
	if labelConflict {
		reason = "Conflicting radio aliases were reported for the same unit ID across calls."
	}

	label := suggestedLabel
	if label == "" {
		label = controller.suggestUnitLearnLabel(system, talkgroup, unitRef, records)
	}
	subjectLabel := label
	if subjectLabel == "" || subjectLabel == "UNKNOWN" {
		subjectLabel = "Name unclear"
	}

	var transcriptHTML strings.Builder
	for i, r := range records {
		radio := r.RadioLabel
		if radio == "" {
			radio = "(none from radio source)"
		}
		transcriptHTML.WriteString(fmt.Sprintf(
			"<h3>Call %d (ID %d)</h3><p><strong>Radio alias:</strong> %s</p><pre style=\"white-space:pre-wrap;background:#f4f4f4;padding:12px;border-radius:6px;\">%s</pre>",
			i+1, r.CallId, escapeHTML(radio), escapeHTML(r.Transcript),
		))
	}

	subject := fmt.Sprintf("[TLR] Unit alias learn review — %s / %s — ref %d — %s", system.Label, talkgroup.Label, unitRef, subjectLabel)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:sans-serif;max-width:720px;margin:0 auto;padding:20px;">
<h2>Unit alias learn review</h2>
<p>%s No unit was added automatically — review the samples below and add the unit manually in admin if appropriate.</p>
<ul>
<li><strong>System:</strong> %s</li>
<li><strong>Talkgroup:</strong> %s (TGID %d)</li>
<li><strong>Radio unit ID:</strong> %d</li>
<li><strong>Suggested label (OpenAI):</strong> %s</li>
</ul>
%s
<p style="color:#666;font-size:13px;">MP3 recordings from each call are attached.</p>
</body></html>`,
		escapeHTML(reason),
		escapeHTML(system.Label),
		escapeHTML(talkgroup.Label),
		talkgroup.TalkgroupRef,
		unitRef,
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
			controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("unit auto-learn: mp3 encode failed for call %d: %v", r.CallId, err))
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
		controller.Logs.LogEvent(LogLevelWarn, "unit auto-learn: no system admin emails found")
		return
	}

	for _, email := range adminEmails {
		if err := controller.EmailService.SendEmailWithAttachments(email, subject, htmlBody, attachments); err != nil {
			controller.Logs.LogEvent(LogLevelWarn, fmt.Sprintf("unit auto-learn: email to %s failed: %v", email, err))
		}
	}

	controller.Logs.LogEvent(LogLevelInfo, fmt.Sprintf("unit auto-learn: review email sent for unitRef %d on talkgroup %d", unitRef, talkgroup.TalkgroupRef))
}
