package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

const subtitleProofreadingMaxRequestBody = 128 * 1024
const subtitleProofreadingMaxResponseBody = 128 * 1024

type subtitleProofreadingProviderCue struct {
	CueID       string                         `json:"cueId"`
	SourceHash  string                         `json:"sourceTextHash"`
	Text        string                         `json:"text"`
	ReasonCode  string                         `json:"reasonCode,omitempty"`
	Neighboring []subtitleProofreadingNeighbor `json:"neighboringCues,omitempty"`
}

type subtitleProofreadingProviderRequest struct {
	ContractVersion string                            `json:"contractVersion"`
	Language        string                            `json:"language"`
	Cues            []subtitleProofreadingProviderCue `json:"cues"`
}

type subtitleProofreadingProviderFinding struct {
	CueID         string  `json:"cueId"`
	SourceHash    string  `json:"sourceTextHash"`
	Status        string  `json:"status"`
	SuggestedText string  `json:"suggestedText,omitempty"`
	Category      string  `json:"category,omitempty"`
	ReasonCode    string  `json:"reasonCode,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	Confidence    float64 `json:"confidence,omitempty"`
	validatedText string  `json:"-"`
}

type subtitleProofreadingProviderResponse struct {
	ContractVersion string                                `json:"contractVersion"`
	Findings        []subtitleProofreadingProviderFinding `json:"findings"`
}

type subtitleProofreadingProviderResult struct {
	Findings []subtitleProofreadingProviderFinding
	Metrics  TranslationProviderMetrics
}

func (provider *ollamaTranslationProvider) Proofread(ctx context.Context, language string, cues []subtitleProofreadingCue) (subtitleProofreadingProviderResult, error) {
	if provider == nil || provider.client == nil || provider.chatURL == nil {
		return subtitleProofreadingProviderResult{}, errors.New("shared Ollama proofreading provider is not configured")
	}
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return subtitleProofreadingProviderResult{}, errors.New("proofreading context is required")
		}
		return subtitleProofreadingProviderResult{}, ctx.Err()
	}
	if len(cues) == 0 || len(cues) > subtitleProofreadingMaxBatchCues {
		return subtitleProofreadingProviderResult{}, errors.New("proofreading batch cue count is outside the supported bound")
	}
	request := subtitleProofreadingProviderRequest{ContractVersion: subtitleProofreadingContractVersion, Language: language}
	for _, cue := range cues {
		providerText, err := subtitleProofreadingProviderText(cue)
		if err != nil {
			return subtitleProofreadingProviderResult{}, err
		}
		request.Cues = append(request.Cues, subtitleProofreadingProviderCue{
			CueID: cue.ID, SourceHash: cue.Hash, Text: providerText,
			Neighboring: append([]subtitleProofreadingNeighbor(nil), cue.Neighbors...),
		})
	}
	prompt, err := subtitleProofreadingPrompt(request)
	if err != nil {
		return subtitleProofreadingProviderResult{}, err
	}
	body, err := json.Marshal(ollamaChatRequest{
		Model:    provider.model,
		Messages: []ollamaChatMessage{{Role: "user", Content: prompt}},
		Stream:   false, Format: "json", KeepAlive: ollamaModelKeepAlive,
	})
	if err != nil {
		return subtitleProofreadingProviderResult{}, fmt.Errorf("encode proofreading request: %w", err)
	}
	maximumRequest := subtitleProofreadingMaxRequestBody
	if provider.maxRequestBytes > 0 && provider.maxRequestBytes < maximumRequest {
		maximumRequest = provider.maxRequestBytes
	}
	if len(body) > maximumRequest {
		return subtitleProofreadingProviderResult{}, fmt.Errorf("proofreading request exceeds %d-byte bound", maximumRequest)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.chatURL.String(), bytes.NewReader(body))
	if err != nil {
		return subtitleProofreadingProviderResult{}, fmt.Errorf("create proofreading request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(req)
	if err != nil {
		return subtitleProofreadingProviderResult{}, ollamaProviderErrorForRequest(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return subtitleProofreadingProviderResult{}, fmt.Errorf("Ollama returned HTTP status %d", response.StatusCode)
	}
	encoded, err := readBoundedOllamaBody(ctx, response.Body, subtitleProofreadingMaxResponseBody)
	if err != nil {
		return subtitleProofreadingProviderResult{}, err
	}
	var outer ollamaChatResponse
	if err := json.Unmarshal(encoded, &outer); err != nil || strings.TrimSpace(outer.Message.Content) == "" {
		return subtitleProofreadingProviderResult{}, errors.New("Ollama proofreading response is malformed")
	}
	findings, err := decodeSubtitleProofreadingResponse([]byte(outer.Message.Content), request, cues, language, provider.model)
	if err != nil {
		return subtitleProofreadingProviderResult{}, err
	}
	return subtitleProofreadingProviderResult{
		Findings: findings,
		Metrics: TranslationProviderMetrics{
			Provider: translationProviderOllama, Model: provider.model, RequestCount: 1,
			InputTokens: outer.PromptEvalCount, OutputTokens: outer.EvalCount,
			LatencyMillis: ollamaDurationMillis(outer.TotalDuration),
		},
	}, nil
}

func subtitleProofreadingPrompt(request subtitleProofreadingProviderRequest) (string, error) {
	if request.ContractVersion != subtitleProofreadingContractVersion || strings.TrimSpace(request.Language) == "" || request.Language == "unknown" {
		return "", errors.New("proofreading language and contract are required")
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode proofreading contract: %w", err)
	}
	instructions := "You are a subtitle proofreader, not a translator or dialogue writer. Review only the target cue text in this bounded batch. Keep the source language " + request.Language + ". Preserve meaning, slang, honorifics, names, line breaks, intentional symbols, and every protected placeholder exactly once in the same order. Do not infer audio or source-language fidelity. Do not edit neighboring context. Return exactly one finding for each input cue in input order. Use status clean, suggested, or needs_review. For a suggestion, return same-language text and a short reason code/category/reason. For uncertainty, use needs_review and do not invent wording. Confidence is advisory only. Return JSON only with contractVersion and findings; include cueId and sourceTextHash exactly from the request.\n\n"
	return instructions + string(encoded), nil
}

func decodeSubtitleProofreadingResponse(payload []byte, request subtitleProofreadingProviderRequest, cues []subtitleProofreadingCue, language, model string) ([]subtitleProofreadingProviderFinding, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var response subtitleProofreadingProviderResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("malformed proofreading response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("proofreading response contains trailing JSON values")
	}
	if response.ContractVersion != request.ContractVersion || len(response.Findings) != len(request.Cues) {
		return nil, errors.New("proofreading response contract or cue count does not match request")
	}
	byID := make(map[string]subtitleProofreadingCue, len(cues))
	for _, cue := range cues {
		byID[cue.ID] = cue
	}
	allowedCategories := map[string]struct{}{
		"encoding/mojibake": {}, "replacement glyphs": {}, "OCR-like confusion": {},
		"spelling": {}, "punctuation/spacing": {}, "consistency": {},
		"possible contextual mismatch": {},
	}
	validated := make([]subtitleProofreadingProviderFinding, len(response.Findings))
	seen := make(map[string]struct{}, len(response.Findings))
	for index, finding := range response.Findings {
		expected := request.Cues[index]
		if finding.CueID != expected.CueID || finding.SourceHash != expected.SourceHash {
			return nil, errors.New("proofreading response cue identity or source hash does not match request")
		}
		if _, duplicate := seen[finding.CueID]; duplicate {
			return nil, errors.New("proofreading response contains a duplicate cue")
		}
		seen[finding.CueID] = struct{}{}
		switch finding.Status {
		case "clean":
			if finding.SuggestedText != "" {
				return nil, errors.New("clean proofreading finding unexpectedly contains a suggestion")
			}
		case "suggested", "needs_review":
			if finding.Status == "suggested" && strings.TrimSpace(finding.SuggestedText) == "" {
				return nil, errors.New("proofreading suggestion is empty")
			}
			if finding.SuggestedText != "" {
				cue, ok := byID[finding.CueID]
				canonical, normalizeErr := normalizeSubtitleProofreadingProtectedText(cue, finding.SuggestedText)
				if !ok || normalizeErr != nil || validateSubtitleProofreadingCandidate(cue, canonical, cueRepresentation(cue), language) != nil {
					finding.Status = "needs_review"
					finding.SuggestedText = ""
				} else {
					finding.validatedText = canonical
				}
			}
		default:
			return nil, errors.New("proofreading response contains an unsupported status")
		}
		if finding.Category != "" {
			if _, ok := allowedCategories[finding.Category]; !ok {
				return nil, errors.New("proofreading response contains an unsupported category")
			}
		}
		if finding.Confidence < 0 || finding.Confidence > 1 {
			return nil, errors.New("proofreading response confidence is outside advisory bounds")
		}
		if len(finding.Reason) > 512 || len(finding.ReasonCode) > 64 || !utf8.ValidString(finding.Reason) {
			return nil, errors.New("proofreading response reason is malformed or oversized")
		}
		if finding.Status == "suggested" && (finding.Category == "" || finding.ReasonCode == "" || strings.TrimSpace(finding.Reason) == "") {
			return nil, errors.New("proofreading suggestion is missing its category or reason")
		}
		validated[index] = finding
	}
	_ = model
	return validated, nil
}

func cueRepresentation(cue subtitleProofreadingCue) string {
	if cue.ASSDialogue != nil {
		return "ass"
	}
	return "srt"
}
