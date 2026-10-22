package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const subtitleTranslationQualityReviewerMaxCues = 100
const subtitleTranslationQualityReviewerMaxBytes = 128 * 1024

type subtitleTranslationQualityReviewerResult struct {
	Provider   string                                      `json:"provider"`
	Model      string                                      `json:"model"`
	State      string                                      `json:"state"`
	Findings   []subtitleTranslationQualityReviewerFinding `json:"findings,omitempty"`
	Diagnostic string                                      `json:"diagnostic,omitempty"`
	CheckedAt  string                                      `json:"checkedAt,omitempty"`
}

type subtitleTranslationQualityReviewerFinding struct {
	CueID         string `json:"cueId"`
	Verdict       string `json:"verdict"`
	Reason        string `json:"reason,omitempty"`
	SuggestedText string `json:"suggestedText,omitempty"`
}

type subtitleTranslationQualityReviewProviderRequest struct {
	Version      string                              `json:"version"`
	TargetLocale string                              `json:"targetLocale"`
	Cues         []subtitleTranslationQualityFinding `json:"cues"`
}

type subtitleTranslationQualityReviewProviderResponse struct {
	Version  string                                      `json:"version"`
	Findings []subtitleTranslationQualityReviewerFinding `json:"findings"`
}

// requestSubtitleTranslationQualitySecondOpinion uses a separately configured
// Ollama model and validates its bounded advisory response. It never mutates cues.
func requestSubtitleTranslationQualitySecondOpinion(ctx context.Context, endpoint, model, translatorModel, targetLocale string, cues []subtitleTranslationQualityFinding) *subtitleTranslationQualityReviewerResult {
	result := &subtitleTranslationQualityReviewerResult{Provider: translationProviderOllama, Model: model, State: "unavailable", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if strings.TrimSpace(model) == "" {
		result.State = "not_configured"
		return result
	}
	if strings.TrimSpace(model) == strings.TrimSpace(translatorModel) {
		result.Diagnostic = "reviewer model must differ from translator model"
		return result
	}
	if ctx == nil || ctx.Err() != nil || len(cues) == 0 {
		result.Diagnostic = "no bounded cue findings were available for independent review"
		return result
	}
	if len(cues) > subtitleTranslationQualityReviewerMaxCues {
		cues = cues[:subtitleTranslationQualityReviewerMaxCues]
	}
	provider, err := newOllamaTranslationProvider(translationSettings{Provider: translationProviderOllama, OllamaEndpoint: endpoint, OllamaModel: model, MaxRequestBytes: subtitleTranslationQualityReviewerMaxBytes})
	if err != nil {
		result.Diagnostic = "independent reviewer configuration is invalid"
		return result
	}
	request := subtitleTranslationQualityReviewProviderRequest{Version: "translation-quality-review-v1", TargetLocale: targetLocale, Cues: cues}
	data, err := json.Marshal(request)
	if err != nil || len(data) > subtitleTranslationQualityReviewerMaxBytes {
		result.Diagnostic = "review request exceeded its bound"
		return result
	}
	prompt := "Assess each source/translated cue pair for meaning preservation and target-language quality. Source text is the reference; do not infer OCR correctness, alter cue identity, timing, or formatting. Return JSON only with version and exactly one finding for each cue in input order. Verdict must be clean, review, or suggested. Suggestions are advisory only and will never be applied automatically. Keep reasons concise and suggested text under 4000 characters. Input: " + string(data)
	body, err := json.Marshal(ollamaChatRequest{Model: model, Messages: []ollamaChatMessage{{Role: "user", Content: prompt}}, Stream: false, Format: "json", KeepAlive: ollamaModelKeepAlive})
	if err != nil || len(body) > subtitleTranslationQualityReviewerMaxBytes {
		result.Diagnostic = "review request exceeded its bound"
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.chatURL.String(), bytes.NewReader(body))
	if err != nil {
		result.Diagnostic = "review request could not be created"
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := provider.client.Do(req)
	if err != nil {
		result.Diagnostic = "independent reviewer request failed"
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Diagnostic = "independent reviewer returned an error"
		return result
	}
	encoded, err := readBoundedOllamaBody(ctx, resp.Body, subtitleProofreadingMaxResponseBody)
	if err != nil {
		result.Diagnostic = "independent reviewer response was unavailable"
		return result
	}
	var outer ollamaChatResponse
	if json.Unmarshal(encoded, &outer) != nil || strings.TrimSpace(outer.Message.Content) == "" {
		result.Diagnostic = "independent reviewer response was malformed"
		return result
	}
	var parsed subtitleTranslationQualityReviewProviderResponse
	decoder := json.NewDecoder(strings.NewReader(outer.Message.Content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&parsed) != nil || decoder.Decode(new(any)) != io.EOF || parsed.Version != request.Version || len(parsed.Findings) != len(cues) {
		result.Diagnostic = "independent reviewer response failed validation"
		return result
	}
	for index := range parsed.Findings {
		finding := &parsed.Findings[index]
		if finding.CueID != cues[index].CueID || (finding.Verdict != "clean" && finding.Verdict != "review" && finding.Verdict != "suggested") || len([]rune(finding.Reason)) > 500 || len([]rune(finding.SuggestedText)) > 4000 || (finding.Verdict != "suggested" && finding.SuggestedText != "") {
			result.Diagnostic = "independent reviewer response failed validation"
			return result
		}
	}
	result.State = "completed"
	result.Findings = parsed.Findings
	return result
}
