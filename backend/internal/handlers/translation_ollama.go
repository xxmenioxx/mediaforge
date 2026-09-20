package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ollamaTranslationHTTPTimeout     = 60 * time.Second
	ollamaTranslationMaxRequestBody  = 2 * 1024 * 1024
	ollamaTranslationMaxResponseBody = 2 * 1024 * 1024
)

type ollamaTranslationProvider struct {
	chatURL           *url.URL
	model             string
	client            *http.Client
	maxCuesPerRequest int
	maxRequestBytes   int
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Format   string              `json:"format"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	PromptEvalCount int   `json:"prompt_eval_count"`
	EvalCount       int   `json:"eval_count"`
	TotalDuration   int64 `json:"total_duration"`
}

func newOllamaTranslationProvider(settings translationSettings) (*ollamaTranslationProvider, error) {
	return newOllamaTranslationProviderWithClient(settings, &http.Client{Timeout: ollamaTranslationHTTPTimeout})
}

func newOllamaTranslationProviderWithClient(settings translationSettings, client *http.Client) (*ollamaTranslationProvider, error) {
	if settings.Provider != translationProviderOllama {
		return nil, fmt.Errorf("translation provider must be configured as %q", translationProviderOllama)
	}
	if strings.TrimSpace(settings.OllamaEndpoint) == "" {
		return nil, fmt.Errorf("ollama endpoint is required")
	}
	if strings.TrimSpace(settings.OllamaModel) == "" {
		return nil, fmt.Errorf("ollama model is required")
	}
	chatURL, err := ollamaChatURL(settings.OllamaEndpoint)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: ollamaTranslationHTTPTimeout}
	}
	return &ollamaTranslationProvider{
		chatURL:           chatURL,
		model:             strings.TrimSpace(settings.OllamaModel),
		client:            client,
		maxCuesPerRequest: settings.MaxCuesPerRequest,
		maxRequestBytes:   settings.MaxRequestBytes,
	}, nil
}

func ollamaChatURL(endpoint string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpoint))
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("ollama endpoint must be an http or https URL without credentials")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/chat"
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed, nil
}

func (provider *ollamaTranslationProvider) Translate(ctx context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	if provider == nil || provider.client == nil || provider.chatURL == nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("ollama translation provider is not configured")}
	}
	settings := defaultTranslationSettings()
	if provider.maxCuesPerRequest > 0 {
		settings.MaxCuesPerRequest = provider.maxCuesPerRequest
	}
	if provider.maxRequestBytes > 0 {
		settings.MaxRequestBytes = provider.maxRequestBytes
	}
	if err := validateTranslationBatch(batch, settings); err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: err}
	}
	prompt, err := ollamaTranslationPrompt(batch)
	if err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: err}
	}
	requestBody := ollamaChatRequest{
		Model: provider.model,
		Messages: []ollamaChatMessage{{
			Role:    "user",
			Content: prompt,
		}},
		Stream: false,
		Format: "json",
	}
	encodedRequest, err := json.Marshal(requestBody)
	if err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("encode Ollama request: %w", err)}
	}
	if len(encodedRequest) > ollamaTranslationMaxRequestBody {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("Ollama request exceeds %d-byte adapter limit", ollamaTranslationMaxRequestBody)}
	}
	if provider.maxRequestBytes > 0 && len(encodedRequest) > provider.maxRequestBytes {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("Ollama request exceeds configured translation request size of %d bytes", provider.maxRequestBytes)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.chatURL.String(), bytes.NewReader(encodedRequest))
	if err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("create Ollama request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(req)
	if err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: fmt.Errorf("Ollama request failed: %w", err)}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: fmt.Errorf("Ollama returned HTTP status %d", response.StatusCode)}
	}
	encodedResponse, err := readBoundedOllamaResponse(response.Body)
	if err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: err}
	}
	var outer ollamaChatResponse
	if err := json.Unmarshal(encodedResponse, &outer); err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorMalformed, Err: fmt.Errorf("decode Ollama response: %w", err)}
	}
	result, err := decodeTranslationBatchResult([]byte(outer.Message.Content))
	if err != nil {
		return TranslationBatchResult{}, err
	}
	result.Metrics = TranslationProviderMetrics{
		Provider:      translationProviderOllama,
		Model:         provider.model,
		RequestCount:  1,
		InputTokens:   outer.PromptEvalCount,
		OutputTokens:  outer.EvalCount,
		LatencyMillis: ollamaDurationMillis(outer.TotalDuration),
	}
	return result, nil
}

func ollamaTranslationPrompt(batch TranslationBatch) (string, error) {
	payload, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("encode translation contract: %w", err)
	}
	return "Translate only the target cue text in this translation-provider batch. Preserve every cue ID exactly once, return exactly one translated text for every target cue, and do not add, remove, merge, split, or reorder cues. Do not translate neighboring or media context. Do not emit explanations or markdown. Return only the structured translation result JSON.\n\n" + string(payload), nil
}

func readBoundedOllamaResponse(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, ollamaTranslationMaxResponseBody+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read bounded Ollama response: %w", err)
	}
	if len(encoded) > ollamaTranslationMaxResponseBody {
		return nil, fmt.Errorf("Ollama response exceeds %d-byte adapter limit", ollamaTranslationMaxResponseBody)
	}
	return encoded, nil
}

func ollamaDurationMillis(nanoseconds int64) int64 {
	if nanoseconds <= 0 {
		return 0
	}
	return nanoseconds / int64(time.Millisecond)
}
