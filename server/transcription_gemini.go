// Copyright (C) 2025 Thinline Dynamic Solutions
//
// Gemini 3.1 Flash-Lite transcription via the Generative Language API.
// Returns JSON {transcript, address?} — ALL CAPS, digits 0-9, no punctuation;
// empty transcript when the audio is noise-only.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	defaultGeminiModel       = "gemini-3.1-flash-lite"
	geminiGenerateContentURL = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
)

// GeminiTranscription implements TranscriptionProvider for Gemini Flash-Lite.
type GeminiTranscription struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// GeminiConfig holds Gemini STT settings.
type GeminiConfig struct {
	APIKey         string
	Model          string
	TimeoutSeconds int
}

// NewGeminiTranscription creates a Gemini Flash-Lite transcription provider.
func NewGeminiTranscription(config *GeminiConfig) *GeminiTranscription {
	timeoutSecs := config.TimeoutSeconds
	if timeoutSecs <= 0 {
		timeoutSecs = 300
	}
	timeout := time.Duration(timeoutSecs) * time.Second
	model := strings.TrimSpace(config.Model)
	if model == "" {
		model = defaultGeminiModel
	}
	return &GeminiTranscription{
		apiKey: strings.TrimSpace(config.APIKey),
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: timeout,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}
}

func (g *GeminiTranscription) IsAvailable() bool {
	return g != nil && g.apiKey != ""
}

func (g *GeminiTranscription) GetName() string {
	return "Gemini Flash-Lite"
}

func (g *GeminiTranscription) GetSupportedLanguages() []string {
	return []string{"en", "auto"}
}

func (g *GeminiTranscription) Transcribe(audio []byte, options TranscriptionOptions) (*TranscriptionResult, error) {
	if !g.IsAvailable() {
		return nil, fmt.Errorf("gemini: API key not configured")
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("gemini: empty audio")
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}
		result, err := g.attemptTranscribe(audio, options)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isRetryableError(err) {
			break
		}
	}
	return nil, lastErr
}

// geminiBasePrompt is intentionally short: responseMimeType + responseSchema
// already force JSON fields, so we only state audio rules and optional address.
// extra should be compact jurisdiction/vocab context — not a second STT prompt.
func geminiBasePrompt(extractAddress bool, extra string) string {
	var b strings.Builder
	b.WriteString("Transcribe radio dispatch audio. ALL CAPS, no punctuation, spoken numbers as digits. ")
	b.WriteString("Empty transcript if no speech.\n")
	if extractAddress {
		b.WriteString("Fill address with the spoken street address or place name only; else leave it empty.\n")
	}
	if ctx := geminiExtraContext(extra); ctx != "" {
		b.WriteString(ctx)
		b.WriteByte('\n')
	}
	return b.String()
}

// geminiExtraContext keeps only useful location/vocab hints. Drops Whisper-style
// "Transcribe…" instruction blocks that duplicate (and can contradict) the base
// prompt + JSON schema.
func geminiExtraContext(extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return ""
	}
	// Already compacted (queue may pre-filter before attemptTranscribe).
	low := strings.ToLower(extra)
	if strings.HasPrefix(low, "location context:") || strings.HasPrefix(low, "context:") {
		return extra
	}
	var locs []string
	for _, line := range strings.Split(extra, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "location context:") {
			locs = append(locs, line)
		}
	}
	if len(locs) > 0 {
		return strings.Join(locs, " ")
	}
	if geminiLooksLikeInstructionPrompt(extra) {
		return ""
	}
	const max = 240
	if len(extra) > max {
		extra = strings.TrimSpace(extra[:max])
	}
	return "Context: " + extra
}

func geminiLooksLikeInstructionPrompt(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(low, "transcribe"):
		return true
	case strings.Contains(low, "output only the spoken"):
		return true
	case strings.Contains(low, "respond with json"):
		return true
	case strings.Contains(low, "output all text in all caps"):
		return true
	default:
		return false
	}
}

func (g *GeminiTranscription) attemptTranscribe(audio []byte, options TranscriptionOptions) (*TranscriptionResult, error) {
	mime := strings.TrimSpace(options.AudioMime)
	if mime == "" {
		mime = "audio/wav"
	}
	// Gemini expects audio/* ; strip codecs=… parameters.
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}

	prompt := geminiBasePrompt(options.ExtractAddress, options.InitialPrompt)
	schemaProps := map[string]any{
		"transcript": map[string]any{"type": "string"},
	}
	if options.ExtractAddress {
		schemaProps["address"] = map[string]any{"type": "string"}
	}
	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": prompt},
					{
						"inline_data": map[string]string{
							"mime_type": mime,
							"data":      base64.StdEncoding.EncodeToString(audio),
						},
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.0,
			"responseMimeType": "application/json",
			"responseSchema": map[string]any{
				"type":       "object",
				"properties": schemaProps,
				"required":   []string{"transcript"},
			},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: encode request: %w", err)
	}

	url := fmt.Sprintf(geminiGenerateContentURL, g.model) + "?key=" + g.apiKey
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		err := fmt.Errorf("gemini: status %d: %s", resp.StatusCode, msg)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return nil, fmt.Errorf("%w (retryable)", err)
		}
		return nil, err
	}

	var apiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	if apiResp.Error != nil && apiResp.Error.Message != "" {
		return nil, fmt.Errorf("gemini: %s", apiResp.Error.Message)
	}
	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return &TranscriptionResult{Transcript: "", Confidence: 1}, nil
	}
	text := strings.TrimSpace(apiResp.Candidates[0].Content.Parts[0].Text)
	transcript, address := parseGeminiTranscriptJSON(text)
	transcript = normalizeGeminiTranscript(transcript)
	address = normalizeGeminiAddress(address)
	if !options.ExtractAddress {
		address = ""
	}
	return &TranscriptionResult{
		Transcript:       transcript,
		ExtractedAddress: address,
		Confidence:       1,
		Language:         options.Language,
	}, nil
}

func parseGeminiTranscriptJSON(text string) (transcript, address string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	// Strip markdown fences if the model wraps JSON.
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```JSON")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}
	var parsed struct {
		Transcript string `json:"transcript"`
		Address    string `json:"address"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		return parsed.Transcript, parsed.Address
	}
	// Fallback: treat whole payload as transcript text.
	return text, ""
}

var geminiPunctRE = regexp.MustCompile(`[[:punct:]]+`)

func normalizeGeminiTranscript(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = geminiPunctRE.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func normalizeGeminiAddress(s string) string {
	s = normalizeGeminiTranscript(s)
	// Keep digits and letters/spaces only (already stripped punct).
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
