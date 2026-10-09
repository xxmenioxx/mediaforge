package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ollamaTranslationHTTPTimeout     = 60 * time.Second
	ollamaModelKeepAlive             = "30m"
	ollamaTranslationMaxRequestBody  = 2 * 1024 * 1024
	ollamaTranslationMaxResponseBody = 2 * 1024 * 1024
	ollamaReadinessMaxResponseBody   = 256 * 1024
)

type ollamaTranslationProvider struct {
	chatURL           *url.URL
	modelURL          *url.URL
	model             string
	client            *http.Client
	maxCuesPerRequest int
	maxRequestBytes   int
}

type ollamaChatRequest struct {
	Model     string              `json:"model"`
	Messages  []ollamaChatMessage `json:"messages"`
	Stream    bool                `json:"stream"`
	Format    string              `json:"format"`
	KeepAlive string              `json:"keep_alive"`
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

type ollamaResponseBodyLimitError struct {
	maximum int
}

func (err *ollamaResponseBodyLimitError) Error() string {
	return fmt.Sprintf("Ollama response exceeds %d-byte adapter limit", err.maximum)
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
	modelURL, err := ollamaModelURL(settings.OllamaEndpoint)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: ollamaTranslationHTTPTimeout}
	}
	return &ollamaTranslationProvider{
		chatURL:           chatURL,
		modelURL:          modelURL,
		model:             strings.TrimSpace(settings.OllamaModel),
		client:            client,
		maxCuesPerRequest: settings.MaxCuesPerRequest,
		maxRequestBytes:   settings.MaxRequestBytes,
	}, nil
}

func ollamaChatURL(endpoint string) (*url.URL, error) {
	return ollamaAPIURL(endpoint, "/api/chat")
}

func ollamaModelURL(endpoint string) (*url.URL, error) {
	return ollamaAPIURL(endpoint, "/api/show")
}

func ollamaAPIURL(endpoint string, apiPath string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpoint))
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("ollama endpoint must be an http or https URL without credentials")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + apiPath
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed, nil
}

type ollamaModelRequest struct {
	Model string `json:"name"`
}

func (provider *ollamaTranslationProvider) TestConnection(ctx context.Context) TranslationProviderReadiness {
	if provider == nil || provider.client == nil || provider.modelURL == nil || strings.TrimSpace(provider.model) == "" {
		return TranslationProviderReadiness{
			State:      translationReadinessInvalidConfiguration,
			Diagnostic: "Ollama readiness configuration is incomplete",
		}
	}
	if ctx == nil {
		return TranslationProviderReadiness{
			State:      translationReadinessInvalidConfiguration,
			Diagnostic: "translation context is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return ollamaReadinessForRequestError(ctx, err)
	}
	encodedRequest, err := json.Marshal(ollamaModelRequest{Model: provider.model})
	if err != nil || len(encodedRequest) > ollamaTranslationMaxRequestBody {
		return TranslationProviderReadiness{
			State:      translationReadinessInvalidConfiguration,
			Diagnostic: "Ollama readiness request is invalid",
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.modelURL.String(), bytes.NewReader(encodedRequest))
	if err != nil {
		return TranslationProviderReadiness{
			State:      translationReadinessInvalidConfiguration,
			Diagnostic: "could not create Ollama readiness request",
		}
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(req)
	if err != nil {
		return ollamaReadinessForRequestError(ctx, err)
	}
	defer response.Body.Close()
	if err := ctx.Err(); err != nil {
		return ollamaReadinessForRequestError(ctx, err)
	}
	if response.StatusCode == http.StatusNotFound {
		return TranslationProviderReadiness{
			State:      translationReadinessModelNotFound,
			Diagnostic: "configured Ollama model was not found",
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return TranslationProviderReadiness{
			State:      translationReadinessUnavailable,
			Diagnostic: fmt.Sprintf("Ollama readiness returned HTTP status %d", response.StatusCode),
		}
	}
	encodedResponse, err := readBoundedOllamaBody(ctx, response.Body, ollamaReadinessMaxResponseBody)
	if err != nil {
		readiness := ollamaReadinessForRequestError(ctx, err)
		var limitErr *ollamaResponseBodyLimitError
		if errors.As(err, &limitErr) {
			readiness.FailureState = "invalid_response"
		}
		return readiness
	}
	if err := ctx.Err(); err != nil {
		return ollamaReadinessForRequestError(ctx, err)
	}
	if len(bytes.TrimSpace(encodedResponse)) == 0 || !json.Valid(encodedResponse) {
		return TranslationProviderReadiness{
			State:        translationReadinessUnavailable,
			Diagnostic:   "Ollama readiness returned a malformed response",
			FailureState: "invalid_response",
		}
	}
	return TranslationProviderReadiness{State: translationReadinessReady}
}

func ollamaReadinessForRequestError(ctx context.Context, err error) TranslationProviderReadiness {
	classified := ollamaProviderErrorForRequest(ctx, err)
	diagnostic := "Ollama readiness request failed"
	switch classified.Class {
	case translationProviderErrorTimeout:
		diagnostic = "Ollama readiness request timed out"
	case translationProviderErrorCancelled:
		diagnostic = "Ollama readiness request was cancelled"
	default:
		var limitErr *ollamaResponseBodyLimitError
		if errors.As(err, &limitErr) {
			diagnostic = fmt.Sprintf("Ollama readiness response exceeds %d-byte adapter limit", limitErr.maximum)
		}
	}
	return TranslationProviderReadiness{State: translationReadinessUnavailable, Diagnostic: diagnostic}
}

func (provider *ollamaTranslationProvider) Translate(ctx context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	if provider == nil || provider.client == nil || provider.chatURL == nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("ollama translation provider is not configured")}
	}
	if ctx == nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("translation context is required")}
	}
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
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
		Stream:    false,
		Format:    "json",
		KeepAlive: ollamaModelKeepAlive,
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
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.chatURL.String(), bytes.NewReader(encodedRequest))
	if err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: fmt.Errorf("create Ollama request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := provider.client.Do(req)
	if err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
	}
	defer response.Body.Close()
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: fmt.Errorf("Ollama returned HTTP status %d", response.StatusCode)}
	}
	encodedResponse, err := readBoundedOllamaResponse(ctx, response.Body)
	if err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
	}
	var outer ollamaChatResponse
	if err := json.Unmarshal(encodedResponse, &outer); err != nil {
		return TranslationBatchResult{}, &TranslationProviderError{Class: translationProviderErrorMalformed, Err: fmt.Errorf("decode Ollama response: %w", err)}
	}
	if strings.TrimSpace(outer.Message.Content) == "" {
		return TranslationBatchResult{}, malformedTranslationResultError("Ollama response has no assistant message content")
	}
	result, err := decodeTranslationBatchResult([]byte(outer.Message.Content))
	if err != nil {
		return TranslationBatchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return TranslationBatchResult{}, ollamaProviderErrorForRequest(ctx, err)
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

func readBoundedOllamaResponse(ctx context.Context, body io.Reader) ([]byte, error) {
	return readBoundedOllamaBody(ctx, body, ollamaTranslationMaxResponseBody)
}

func readBoundedOllamaBody(ctx context.Context, body io.Reader, maximum int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limited := io.LimitReader(body, int64(maximum)+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read bounded Ollama response: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(encoded) > maximum {
		return nil, &ollamaResponseBodyLimitError{maximum: maximum}
	}
	return encoded, nil
}

func ollamaProviderErrorForRequest(ctx context.Context, err error) *TranslationProviderError {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.Canceled) {
				return &TranslationProviderError{Class: translationProviderErrorCancelled, Err: ctxErr}
			}
			return &TranslationProviderError{Class: translationProviderErrorTimeout, Err: ctxErr}
		}
	}
	if errors.Is(err, context.Canceled) {
		return &TranslationProviderError{Class: translationProviderErrorCancelled, Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &TranslationProviderError{Class: translationProviderErrorTimeout, Err: err}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return &TranslationProviderError{Class: translationProviderErrorTimeout, Err: err}
	}
	return &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: fmt.Errorf("Ollama request failed: %w", err)}
}

func ollamaDurationMillis(nanoseconds int64) int64 {
	if nanoseconds <= 0 {
		return 0
	}
	return nanoseconds / int64(time.Millisecond)
}
