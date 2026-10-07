package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func ollamaReadinessBodyOfSize(t *testing.T, size int) []byte {
	t.Helper()
	const prefix = `{"modelfile":"`
	const suffix = `"}`
	if size < len(prefix)+len(suffix) {
		t.Fatalf("requested readiness body size %d is too small", size)
	}
	body := []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
	if len(body) != size || !json.Valid(body) {
		t.Fatalf("generated readiness body size=%d valid=%t, want size=%d and valid JSON", len(body), json.Valid(body), size)
	}
	return body
}

func TestOllamaReadinessResponseBoundsAndClassifications(t *testing.T) {
	largeGemmaBody := ollamaReadinessBodyOfSize(t, 89_995)
	oversizedBody := ollamaReadinessBodyOfSize(t, ollamaReadinessMaxResponseBody+1)
	tests := []struct {
		name           string
		model          string
		statusCode     int
		responseBody   []byte
		requestErr     error
		wantState      translationProviderReadinessState
		wantDiagnostic string
	}{
		{
			name:           "valid response below the old limit",
			model:          "qwen2.5:7b",
			statusCode:     http.StatusOK,
			responseBody:   ollamaReadinessBodyOfSize(t, 32*1024),
			wantState:      translationReadinessReady,
			wantDiagnostic: "",
		},
		{
			name:           "valid translategemma response around 90 KB",
			model:          "translategemma:12b",
			statusCode:     http.StatusOK,
			responseBody:   largeGemmaBody,
			wantState:      translationReadinessReady,
			wantDiagnostic: "",
		},
		{
			name:           "response exceeds bounded readiness limit",
			model:          "translategemma:12b",
			statusCode:     http.StatusOK,
			responseBody:   oversizedBody,
			wantState:      translationReadinessUnavailable,
			wantDiagnostic: "Ollama readiness response exceeds 262144-byte adapter limit",
		},
		{
			name:           "model not found",
			model:          "missing-model",
			statusCode:     http.StatusNotFound,
			responseBody:   []byte(`{"error":"private provider response"}`),
			wantState:      translationReadinessModelNotFound,
			wantDiagnostic: "configured Ollama model was not found",
		},
		{
			name:           "other non-success HTTP status",
			model:          "model",
			statusCode:     http.StatusServiceUnavailable,
			responseBody:   []byte(`{"error":"private provider response"}`),
			wantState:      translationReadinessUnavailable,
			wantDiagnostic: "Ollama readiness returned HTTP status 503",
		},
		{
			name:           "malformed JSON",
			model:          "model",
			statusCode:     http.StatusOK,
			responseBody:   []byte(`{"modelfile":`),
			wantState:      translationReadinessUnavailable,
			wantDiagnostic: "Ollama readiness returned a malformed response",
		},
		{
			name:           "network request failure",
			model:          "model",
			requestErr:     errors.New("dial tcp 10.0.0.8:11434: connection refused"),
			wantState:      translationReadinessUnavailable,
			wantDiagnostic: "Ollama readiness request failed",
		},
		{
			name:           "request timeout",
			model:          "model",
			requestErr:     context.DeadlineExceeded,
			wantState:      translationReadinessUnavailable,
			wantDiagnostic: "Ollama readiness request timed out",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: ollamaRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodPost || request.URL.Path != "/api/show" {
					t.Errorf("request=%s %s, want POST /api/show", request.Method, request.URL.Path)
				}
				var modelRequest ollamaModelRequest
				if err := json.NewDecoder(request.Body).Decode(&modelRequest); err != nil {
					t.Errorf("decode /api/show request: %v", err)
				}
				if modelRequest.Model != test.model {
					t.Errorf("model=%q, want %q", modelRequest.Model, test.model)
				}
				if test.requestErr != nil {
					return nil, test.requestErr
				}
				return &http.Response{
					StatusCode: test.statusCode,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(test.responseBody))),
					Request:    request,
				}, nil
			})}
			provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", test.model), client)
			if err != nil {
				t.Fatal(err)
			}

			result := provider.TestConnection(context.Background())
			if result.State != test.wantState || result.Diagnostic != test.wantDiagnostic {
				t.Fatalf("result=%#v, want state=%q diagnostic=%q", result, test.wantState, test.wantDiagnostic)
			}
			if strings.Contains(result.Diagnostic, "10.0.0.8") || strings.Contains(result.Diagnostic, "private provider response") {
				t.Fatalf("readiness diagnostic exposed sensitive request or response data: %q", result.Diagnostic)
			}
		})
	}
}

func TestTranslationEnableAcceptsLargeOllamaShowResponse(t *testing.T) {
	const model = "translategemma:12b"
	responseBody := ollamaReadinessBodyOfSize(t, 89_995)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/show" {
			t.Errorf("request=%s %s, want POST /api/show", request.Method, request.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var modelRequest ollamaModelRequest
		if err := json.NewDecoder(request.Body).Decode(&modelRequest); err != nil {
			t.Errorf("decode /api/show request: %v", err)
		}
		if modelRequest.Model != model {
			t.Errorf("model=%q, want %q", modelRequest.Model, model)
		}
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	db := newTranslationReadinessTestDB(t)
	settings := ollamaTestSettings(server.URL, model)
	persistTranslationReadinessSettings(t, db, settings)
	status, body, responseText := postTranslationReadiness(t, translationReadinessTestRouter(db), "/api/settings/subtitle-translation/enable")
	if status != http.StatusOK || !body.Enabled || body.Readiness != string(translationReadinessReady) {
		t.Fatalf("status=%d body=%#v response=%s", status, body, responseText)
	}
	loaded := loadTranslationSettings(db)
	if !loaded.Enabled || loaded.Readiness != translationReadinessReady || loaded.ReadinessIdentity != translationProviderConfigurationIdentity(loaded) || loaded.ReadinessCheckedAt == "" {
		t.Fatalf("persisted readiness=%#v", loaded)
	}
}
