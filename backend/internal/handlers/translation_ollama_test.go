package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
					Stream bool   `json:"stream"`
					Format string `json:"format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				if request.Model != "qwen2.5:7b" || request.Stream || request.Format != "json" || len(request.Messages) != 1 || request.Messages[0].Role != "user" {
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
	if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err == nil || !strings.Contains(strings.ToLower(err.Error()), "ollama") {
		t.Fatalf("unreachable endpoint error was not actionable: %v", err)
	}
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
