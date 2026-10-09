package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaDurationMillis(t *testing.T) {
	oldFalseOverflowThreshold := math.MaxInt64 / int64(time.Millisecond)
	tests := []struct {
		name        string
		nanoseconds int64
		wantMillis  int64
	}{
		{name: "zero", nanoseconds: 0, wantMillis: 0},
		{name: "negative", nanoseconds: -1, wantMillis: 0},
		{name: "twenty-five milliseconds", nanoseconds: 25_000_000, wantMillis: 25},
		{
			name:        "above old false overflow threshold",
			nanoseconds: oldFalseOverflowThreshold + int64(time.Millisecond),
			wantMillis:  (oldFalseOverflowThreshold + int64(time.Millisecond)) / int64(time.Millisecond),
		},
		{name: "maximum int64", nanoseconds: math.MaxInt64, wantMillis: math.MaxInt64 / int64(time.Millisecond)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ollamaDurationMillis(test.nanoseconds); got != test.wantMillis {
				t.Fatalf("ollamaDurationMillis(%d)=%d, want %d", test.nanoseconds, got, test.wantMillis)
			}
		})
	}
}

func TestOllamaTranslationProviderUsesConfiguredChatEndpointAndContract(t *testing.T) {
	for _, endpointSuffix := range []string{"", "/", "/ollama"} {
		t.Run(fmt.Sprintf("endpoint-%q", endpointSuffix), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				wantPath := "/api/chat"
				if endpointSuffix == "/ollama" {
					wantPath = "/ollama/api/chat"
				}
				if r.URL.Path != wantPath {
					t.Errorf("path=%q, want %q", r.URL.Path, wantPath)
				}
				var request struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					Stream    bool   `json:"stream"`
					Format    string `json:"format"`
					KeepAlive string `json:"keep_alive"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				if request.Model != "qwen2.5:7b" || request.Stream || request.Format != "json" || request.KeepAlive != "30m" || len(request.Messages) != 1 || request.Messages[0].Role != "user" {
					t.Errorf("unexpected Ollama request envelope: %#v", request)
				}
				content := request.Messages[0].Content
				for _, expected := range []string{"translation-provider-v1", "en", "es-419", "cue-1", "Hello", "nearby", "Title"} {
					if !strings.Contains(content, expected) {
						t.Errorf("request content missing %q: %s", expected, content)
					}
				}
				for _, forbidden := range []string{"sourceRevision", "Timing", "Geometry", "media/path.mkv", "Authorization"} {
					if strings.Contains(content, forbidden) {
						t.Errorf("request content exposed %q: %s", forbidden, content)
					}
				}
				writeOllamaTestResponse(t, w, `{"ContractVersion":"translation-provider-v1","Cues":[{"ID":"cue-1","Text":"Hola"}]}`, 12, 7, 25_000_000)
			}))
			defer server.Close()
			settings := ollamaTestSettings(server.URL+endpointSuffix, "qwen2.5:7b")
			provider, err := newOllamaTranslationProvider(settings)
			if err != nil {
				t.Fatal(err)
			}
			batch := TranslationBatch{
				ContractVersion: translationProviderContractVersion,
				SourceLanguage:  "en",
				TargetLocale:    "es-419",
				Cues:            []TranslationCue{{ID: "cue-1", Text: "Hello"}},
				Context: &TranslationContext{
					NeighboringCues: []TranslationContextCue{{ID: "nearby", Text: "Earlier"}},
					Media:           TranslationMediaContext{Title: "Title"},
				},
			}
			result, err := provider.Translate(context.Background(), batch)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 || len(result.Cues) != 1 || result.Cues[0].ID != "cue-1" || result.Cues[0].Text != "Hola" {
				t.Fatalf("unexpected request/result: requests=%d result=%#v", requests, result)
			}
			if result.Metrics.Provider != translationProviderOllama || result.Metrics.Model != "qwen2.5:7b" || result.Metrics.RequestCount != 1 || result.Metrics.InputTokens != 12 || result.Metrics.OutputTokens != 7 || result.Metrics.LatencyMillis != 25 {
				t.Fatalf("unexpected mapped metrics: %#v", result.Metrics)
			}
		})
	}
}

func TestOllamaTranslationProviderDoesNotPerformHealthOrModelChecks(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected preflight path: %q", r.URL.Path)
		}
		writeOllamaTestResponse(t, w, `{"ContractVersion":"translation-provider-v1","Cues":[{"ID":"cue-1","Text":"Hola"}]}`, 0, 0, 0)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("request count=%d, want one chat request", requests)
	}
}

func TestOllamaTranslationProviderConfigurationFailsClosed(t *testing.T) {
	cases := []struct {
		name     string
		settings translationSettings
	}{
		{name: "provider", settings: translationSettings{Provider: "other", OllamaEndpoint: "http://127.0.0.1", OllamaModel: "model"}},
		{name: "endpoint", settings: translationSettings{Provider: translationProviderOllama, OllamaModel: "model"}},
		{name: "model", settings: translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://127.0.0.1"}},
		{name: "invalid endpoint", settings: translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "not-a-url", OllamaModel: "model"}},
		{name: "credential endpoint", settings: translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://user:pass@example.com", OllamaModel: "model"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newOllamaTranslationProvider(test.settings); err == nil {
				t.Fatal("invalid Ollama configuration accepted")
			}
		})
	}
}

func TestOllamaTranslationProviderCancellationBeforeWork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := provider.Translate(ctx, ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(err), err)
	}
	assertEmptyOllamaResult(t, result)
	if requests != 0 {
		t.Fatalf("cancelled request reached server: %d", requests)
	}
}

func TestOllamaTranslationProviderCancellationDuringRequest(t *testing.T) {
	started := make(chan struct{})
	requestCancelled := make(chan struct{})
	client := &http.Client{Transport: ollamaRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(requestCancelled)
		return nil, req.Context().Err()
	})}
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type translationCall struct {
		result TranslationBatchResult
		err    error
	}
	call := make(chan translationCall, 1)
	go func() {
		result, err := provider.Translate(ctx, ollamaTestBatch())
		call <- translationCall{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider request did not reach server")
	}
	cancel()
	var outcome translationCall
	select {
	case outcome = <-call:
	case <-time.After(time.Second):
		t.Fatal("provider did not return after cancellation")
	}
	if outcome.err == nil || translationProviderErrorClassOf(outcome.err) != translationProviderErrorCancelled || !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("in-flight cancellation was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(outcome.err), outcome.err)
	}
	assertEmptyOllamaResult(t, outcome.result)
	select {
	case <-requestCancelled:
	case <-time.After(time.Second):
		t.Fatal("HTTP transport did not observe request cancellation")
	}
}

func TestOllamaTranslationProviderCallerDeadlineIsTimeout(t *testing.T) {
	started := make(chan struct{})
	deadlineObserved := make(chan struct{})
	client := &http.Client{Transport: ollamaRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(deadlineObserved)
		return nil, req.Context().Err()
	})}
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	type translationCall struct {
		result TranslationBatchResult
		err    error
	}
	call := make(chan translationCall, 1)
	go func() {
		result, err := provider.Translate(ctx, ollamaTestBatch())
		call <- translationCall{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider request did not reach server")
	}
	select {
	case outcome := <-call:
		if outcome.err == nil || translationProviderErrorClassOf(outcome.err) != translationProviderErrorTimeout || !errors.Is(outcome.err, context.DeadlineExceeded) {
			t.Fatalf("deadline was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(outcome.err), outcome.err)
		}
		assertEmptyOllamaResult(t, outcome.result)
	case <-time.After(time.Second):
		t.Fatal("provider did not return after deadline")
	}
	select {
	case <-deadlineObserved:
	case <-time.After(time.Second):
		t.Fatal("HTTP transport did not observe request deadline")
	}
}

func TestOllamaTranslationProviderHTTPClientTimeoutIsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings(server.URL, "model"), &http.Client{Timeout: 25 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Translate(context.Background(), ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorTimeout {
		t.Fatalf("HTTP client timeout was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(err), err)
	}
	if translationProviderErrorClassOf(err) == translationProviderErrorCancelled {
		t.Fatalf("HTTP client timeout was classified as cancellation: %v", err)
	}
	assertEmptyOllamaResult(t, result)
}

func TestOllamaTranslationProviderResponseReadCancellation(t *testing.T) {
	started := make(chan struct{})
	client := &http.Client{Transport: ollamaRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       &ollamaBlockingResponseBody{ctx: req.Context(), started: started},
			Request:    req,
		}, nil
	})}
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type translationCall struct {
		result TranslationBatchResult
		err    error
	}
	call := make(chan translationCall, 1)
	go func() {
		result, err := provider.Translate(ctx, ollamaTestBatch())
		call <- translationCall{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("response read did not start")
	}
	cancel()
	select {
	case outcome := <-call:
		if outcome.err == nil || translationProviderErrorClassOf(outcome.err) != translationProviderErrorCancelled || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("response-read cancellation was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(outcome.err), outcome.err)
		}
		assertEmptyOllamaResult(t, outcome.result)
	case <-time.After(time.Second):
		t.Fatal("provider did not return after response-read cancellation")
	}
}

func TestOllamaTranslationProviderHTTPStatusFailures(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprintf("status-%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "sensitive provider response body")
			}))
			defer server.Close()
			provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Translate(context.Background(), ollamaTestBatch())
			if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorUnavailable {
				t.Fatalf("HTTP failure class=%q err=%v", translationProviderErrorClassOf(err), err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", status)) || strings.Contains(err.Error(), "sensitive provider response body") {
				t.Fatalf("HTTP failure diagnostic was not bounded: %v", err)
			}
			assertEmptyOllamaResult(t, result)
		})
	}
}

func TestOllamaTranslationProviderMalformedResponses(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{name: "invalid outer JSON", payload: "{"},
		{name: "missing message", payload: `{}`},
		{name: "empty assistant content", payload: `{"message":{"content":""}}`},
		{name: "invalid inner JSON", payload: `{"message":{"content":"not-json"}}`},
		{name: "unknown inner field", payload: `{"message":{"content":"{\"contractVersion\":\"translation-provider-v1\",\"cues\":[{\"id\":\"cue-1\",\"text\":\"Hola\",\"start\":1}]}"}}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, test.payload)
			}))
			defer server.Close()
			provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Translate(context.Background(), ollamaTestBatch())
			if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorMalformed {
				t.Fatalf("malformed response class=%q err=%v", translationProviderErrorClassOf(err), err)
			}
			assertEmptyOllamaResult(t, result)
		})
	}
}

func TestOllamaTranslationProviderRejectsConfiguredRequestBodyOverflow(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	settings := ollamaTestSettings(server.URL, "model")
	settings.MaxRequestBytes = 1024
	provider, err := newOllamaTranslationProviderWithClient(settings, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	batch := ollamaTestBatch()
	batch.Cues[0].Text = strings.Repeat("x", 900)
	if _, err := provider.Translate(context.Background(), batch); err == nil {
		t.Fatal("request body overflow accepted")
	}
	if requests != 0 {
		t.Fatalf("overflow made an HTTP request: %d", requests)
	}
}

func TestOllamaTranslationProviderRejectsMalformedOrOversizedResponsesWithoutRetainingBody(t *testing.T) {
	t.Run("malformed inner JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeOllamaTestResponse(t, w, "not-json", 0, 0, 0)
		}))
		defer server.Close()
		provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err == nil || strings.Contains(err.Error(), "not-json") {
			t.Fatalf("malformed response was not safely rejected: %v", err)
		}
	})
	t.Run("bounded outer response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", ollamaTranslationMaxResponseBody+1)))
		}))
		defer server.Close()
		provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err == nil || strings.Contains(err.Error(), strings.Repeat("x", 100)) {
			t.Fatalf("oversized response was not safely bounded: %v", err)
		}
	})
}

func TestOllamaTranslationProviderUnreachableEndpointIsActionable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := server.URL
	server.Close()
	settings := ollamaTestSettings(endpoint, "model")
	provider, err := newOllamaTranslationProviderWithClient(settings, &http.Client{Timeout: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Translate(context.Background(), ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorUnavailable || !strings.Contains(strings.ToLower(err.Error()), "ollama") {
		t.Fatalf("unreachable endpoint error was not actionable: %v", err)
	}
	assertEmptyOllamaResult(t, result)
}

func assertEmptyOllamaResult(t *testing.T, result TranslationBatchResult) {
	t.Helper()
	if result.ContractVersion != "" || len(result.Cues) != 0 || result.Metrics != (TranslationProviderMetrics{}) {
		t.Fatalf("failed request returned a result: %#v", result)
	}
}

type ollamaRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f ollamaRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type ollamaBlockingResponseBody struct {
	ctx     context.Context
	started chan<- struct{}
}

func (b *ollamaBlockingResponseBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *ollamaBlockingResponseBody) Close() error {
	return nil
}

func ollamaTestSettings(endpoint, model string) translationSettings {
	settings := defaultTranslationSettings()
	settings.Provider = translationProviderOllama
	settings.OllamaEndpoint = endpoint
	settings.OllamaModel = model
	return settings
}

func ollamaTestBatch() TranslationBatch {
	return TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  "en",
		TargetLocale:    "es-419",
		Cues:            []TranslationCue{{ID: "cue-1", Text: "Hello"}},
	}
}

func writeOllamaTestResponse(t *testing.T, w http.ResponseWriter, content string, promptTokens, outputTokens int, totalDuration int64) {
	t.Helper()
	payload := map[string]any{
		"message":           map[string]string{"role": "assistant", "content": content},
		"prompt_eval_count": promptTokens,
		"eval_count":        outputTokens,
		"total_duration":    totalDuration,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Error(err)
	}
}
