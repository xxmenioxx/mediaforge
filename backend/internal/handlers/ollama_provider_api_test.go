package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type ollamaProviderTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper ollamaProviderTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func ollamaProviderTestResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}

func TestOllamaConnectionTestUsesOnlyVersionEndpointAndPreservesTranslation(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	seedLegacyOllamaTranslation(t, db, "http://ollama.test:11434")
	requests := 0
	client := &http.Client{Transport: ollamaProviderTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/api/version" {
			return nil, fmt.Errorf("unexpected Ollama request %s %s", request.Method, request.URL.Path)
		}
		return ollamaProviderTestResponse(request, http.StatusOK, `{"version":"0.6.8"}`), nil
	})}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/ollama/test-connection", (SettingsHandler{db: db, ollamaHTTPClient: client}).TestOllamaConnection)
	status, responseBody := invokeOllamaHandler(t, router, http.MethodPost, "/api/settings/ollama/test-connection", "")
	var result ollamaConnectionTestResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || result.State != "reachable" || result.ServerVersion != "0.6.8" || result.CheckedAt == "" || requests != 1 {
		t.Fatalf("status=%d result=%#v requests=%d", status, result, requests)
	}
	var stored models.AppSetting
	if err := db.First(&stored, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Value["ollamaModel"] != "translategemma:12b" || stored.Value["enabled"] != true || stored.Value["readiness"] != string(translationReadinessReady) {
		t.Fatalf("connection test changed Translation selection/readiness: %#v", stored.Value)
	}
	if _, duplicate := stored.Value["ollamaEndpoint"]; duplicate {
		t.Fatalf("migration left duplicate endpoint behind: %#v", stored.Value)
	}
}

func TestOllamaConnectionTestClassifiesInvalidUnavailableTimeoutCancellationAndOversize(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		context   func() context.Context
		transport ollamaProviderTestRoundTripper
		wantState string
	}{
		{name: "invalid endpoint", endpoint: "ftp://ollama.test", wantState: ollamaProviderStateInvalid},
		{name: "service unavailable", endpoint: "http://ollama.test", transport: func(request *http.Request) (*http.Response, error) {
			return ollamaProviderTestResponse(request, http.StatusBadGateway, `sensitive upstream detail`), nil
		}, wantState: "unavailable"},
		{name: "timeout", endpoint: "http://ollama.test", transport: func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }, wantState: "unavailable"},
		{name: "canceled", endpoint: "http://ollama.test", context: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, transport: func(*http.Request) (*http.Response, error) { return nil, errors.New("transport should not run") }, wantState: "unavailable"},
		{name: "oversized response", endpoint: "http://ollama.test", transport: func(request *http.Request) (*http.Response, error) {
			return ollamaProviderTestResponse(request, http.StatusOK, `{"version":"`+strings.Repeat("x", ollamaVersionMaxResponseBody)+`"}`), nil
		}, wantState: "invalid_response"},
		{name: "malformed response", endpoint: "http://ollama.test", transport: func(request *http.Request) (*http.Response, error) {
			return ollamaProviderTestResponse(request, http.StatusOK, `not-json`), nil
		}, wantState: "invalid_response"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.context != nil {
				ctx = test.context()
			}
			var client ollamaProviderAPIClient
			if test.transport != nil {
				client.client = &http.Client{Transport: test.transport}
			}
			result := testOllamaConnection(ctx, test.endpoint, client)
			if result.State != test.wantState {
				t.Fatalf("state=%q want=%q result=%#v", result.State, test.wantState, result)
			}
			if strings.Contains(result.Diagnostic, "sensitive") || strings.Contains(result.Diagnostic, "not-json") {
				t.Fatalf("response body leaked through diagnostic: %#v", result)
			}
		})
	}
}

func TestOllamaInstalledModelCatalogMapsMetadataAndIsBounded(t *testing.T) {
	requests := 0
	client := ollamaProviderAPIClient{client: &http.Client{Transport: ollamaProviderTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/api/tags" {
			return nil, fmt.Errorf("unexpected Ollama request %s %s", request.Method, request.URL.Path)
		}
		return ollamaProviderTestResponse(request, http.StatusOK, `{"models":[{"name":"zeta:latest","model":"zeta:latest","digest":"sha-z","size":2048,"modified_at":"2026-10-08T10:00:00Z","details":{"parent_model":"zeta","format":"gguf","family":"llama","families":["llama"],"parameter_size":"7B","quantization_level":"Q4_K_M"}},{"name":"alpha:latest","model":"alpha:latest","digest":"sha-a"}]}`), nil
	})}}
	result := listOllamaInstalledModels(context.Background(), "http://ollama.test", client)
	if result.State != "success" || result.DiscoveredAt == "" || requests != 1 || len(result.InstalledModels) != 2 {
		t.Fatalf("catalog result=%#v requests=%d", result, requests)
	}
	if result.InstalledModels[0].ID != "alpha:latest" || result.InstalledModels[0].Name != "alpha:latest" || result.InstalledModels[1].ID != "zeta:latest" || result.InstalledModels[1].SizeBytes != 2048 || result.InstalledModels[1].Details == nil || result.InstalledModels[1].Details.ParameterSize != "7B" {
		t.Fatalf("catalog metadata mapping=%#v", result.InstalledModels)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		InstalledModels []json.RawMessage `json:"installedModels"`
	}
	if err := json.Unmarshal(encoded, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.InstalledModels) != 2 || strings.Contains(string(catalog.InstalledModels[0]), `"loaded"`) || strings.Contains(string(catalog.InstalledModels[0]), `"available"`) {
		t.Fatalf("installed catalog implies loaded/available model state: %s", encoded)
	}
}

func TestOllamaInstalledModelCatalogDistinguishesEmptyAndFailure(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantState string
	}{
		{name: "empty success", status: http.StatusOK, body: `{"models":[]}`, wantState: "success"},
		{name: "malformed", status: http.StatusOK, body: `secret malformed payload`, wantState: "invalid_response"},
		{name: "missing models array", status: http.StatusOK, body: `{}`, wantState: "invalid_response"},
		{name: "http error", status: http.StatusServiceUnavailable, body: `secret provider response`, wantState: "unavailable"},
		{name: "oversized", status: http.StatusOK, body: `{"models":[]}` + strings.Repeat(" ", ollamaTagsMaxResponseBody), wantState: "invalid_response"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := ollamaProviderAPIClient{client: &http.Client{Transport: ollamaProviderTestRoundTripper(func(request *http.Request) (*http.Response, error) {
				return ollamaProviderTestResponse(request, test.status, test.body), nil
			})}}
			result := listOllamaInstalledModels(context.Background(), "http://ollama.test", client)
			if result.State != test.wantState {
				t.Fatalf("state=%q want=%q result=%#v", result.State, test.wantState, result)
			}
			if test.name == "empty success" && (result.DiscoveredAt == "" || len(result.InstalledModels) != 0) {
				t.Fatalf("empty installed catalog was not distinct success: %#v", result)
			}
			if strings.Contains(result.Diagnostic, "secret") || strings.Contains(result.Diagnostic, "malformed payload") {
				t.Fatalf("raw response leaked through diagnostic: %#v", result)
			}
		})
	}
}

func TestOllamaModelReadinessChecksMetadataWithoutClaimingResidency(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: ollamaProviderTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/api/show" {
			return nil, fmt.Errorf("unexpected Ollama request %s %s", request.Method, request.URL.Path)
		}
		var payload ollamaModelRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload.Model != "translator:latest" {
			return nil, fmt.Errorf("unexpected model request: %#v err=%v", payload, err)
		}
		return ollamaProviderTestResponse(request, http.StatusOK, `{"modelfile":"metadata only","details":{}}`), nil
	})}
	readiness := checkOllamaModelReadiness(context.Background(), "http://ollama.test", "translator:latest", client)
	result, status := ollamaModelReadinessResult("translator:latest", readiness, time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC))
	if status != http.StatusOK || result.State != "available" || !result.Available || result.ModelID != "translator:latest" || result.Scope != "metadata_check" || result.CheckedAt == "" || requests != 1 {
		t.Fatalf("status=%d result=%#v requests=%d", status, result, requests)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"loaded"`) || strings.Contains(string(encoded), `"inferenceReady"`) {
		t.Fatalf("metadata check implies residency or inference readiness: %s", encoded)
	}

	missingClient := &http.Client{Transport: ollamaProviderTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		return ollamaProviderTestResponse(request, http.StatusNotFound, `{"error":"secret model payload"}`), nil
	})}
	missing, status := ollamaModelReadinessResult("missing:tag", checkOllamaModelReadiness(context.Background(), "http://ollama.test", "missing:tag", missingClient), time.Now())
	if status != http.StatusNotFound || missing.State != "model_not_found" || missing.Available || strings.Contains(missing.Diagnostic, "secret") {
		t.Fatalf("model missing result=%#v status=%d", missing, status)
	}
}

func TestOllamaModelReadinessFailureStatesRemainBounded(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		modelID   string
		context   func() context.Context
		transport ollamaProviderTestRoundTripper
		wantState string
		wantCode  int
	}{
		{name: "invalid endpoint", endpoint: "ftp://ollama.test", modelID: "model", wantState: ollamaProviderStateInvalid, wantCode: http.StatusBadRequest},
		{name: "service failure", endpoint: "http://ollama.test", modelID: "model", transport: func(request *http.Request) (*http.Response, error) {
			return ollamaProviderTestResponse(request, http.StatusInternalServerError, `secret model response`), nil
		}, wantState: "unavailable", wantCode: http.StatusServiceUnavailable},
		{name: "timeout", endpoint: "http://ollama.test", modelID: "model", transport: func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }, wantState: "unavailable", wantCode: http.StatusServiceUnavailable},
		{name: "canceled", endpoint: "http://ollama.test", modelID: "model", context: func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }, transport: func(*http.Request) (*http.Response, error) { return nil, context.Canceled }, wantState: "unavailable", wantCode: http.StatusServiceUnavailable},
		{name: "oversized show response", endpoint: "http://ollama.test", modelID: "model", transport: func(request *http.Request) (*http.Response, error) {
			return ollamaProviderTestResponse(request, http.StatusOK, `{"modelfile":"`+strings.Repeat("x", ollamaReadinessMaxResponseBody)+`"}`), nil
		}, wantState: "invalid_response", wantCode: http.StatusBadGateway},
		{name: "malformed show response", endpoint: "http://ollama.test", modelID: "model", transport: func(request *http.Request) (*http.Response, error) {
			return ollamaProviderTestResponse(request, http.StatusOK, `not-json`), nil
		}, wantState: "invalid_response", wantCode: http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.context != nil {
				ctx = test.context()
			}
			var client *http.Client
			if test.transport != nil {
				client = &http.Client{Transport: test.transport}
			}
			response, status := ollamaModelReadinessResult(test.modelID, checkOllamaModelReadiness(ctx, test.endpoint, test.modelID, client), time.Now())
			if status != test.wantCode || response.State != test.wantState {
				t.Fatalf("status=%d state=%q want status=%d state=%q response=%#v", status, response.State, test.wantCode, test.wantState, response)
			}
			if strings.Contains(response.Diagnostic, "secret") || strings.Contains(response.Diagnostic, strings.Repeat("x", 64)) {
				t.Fatalf("raw response leaked through diagnostic: %#v", response)
			}
		})
	}
}

func TestOllamaProviderHandlersUseSharedEndpointForConsumerModelIDs(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	seedLegacyOllamaTranslation(t, db, "http://ollama.test:11434")
	requests := make([]string, 0, 4)
	client := &http.Client{Transport: ollamaProviderTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/api/tags":
			if request.Method != http.MethodGet {
				return nil, fmt.Errorf("catalog request method=%s", request.Method)
			}
			return ollamaProviderTestResponse(request, http.StatusOK, `{"models":[{"name":"consumer:latest"}]}`), nil
		case "/api/show":
			if request.Method != http.MethodPost {
				return nil, fmt.Errorf("model readiness request method=%s", request.Method)
			}
			var payload ollamaModelRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				return nil, fmt.Errorf("model request=%#v err=%v", payload, err)
			}
			switch payload.Model {
			case "consumer:latest":
				return ollamaProviderTestResponse(request, http.StatusOK, `{"modelfile":"metadata"}`), nil
			case "missing:latest":
				return ollamaProviderTestResponse(request, http.StatusNotFound, `{"error":"private missing model details"}`), nil
			case "offline:latest":
				return ollamaProviderTestResponse(request, http.StatusServiceUnavailable, `private provider details`), nil
			default:
				return nil, fmt.Errorf("unexpected model ID %q", payload.Model)
			}
		default:
			return nil, fmt.Errorf("unexpected Ollama API path %q", request.URL.Path)
		}
	})}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := SettingsHandler{db: db, ollamaHTTPClient: client}
	router.GET("/api/settings/ollama/models", handler.ListOllamaModels)
	router.POST("/api/settings/ollama/model-readiness", handler.TestOllamaModelReadiness)
	status, body := invokeOllamaHandler(t, router, http.MethodGet, "/api/settings/ollama/models", "")
	var catalog ollamaModelsResponse
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || catalog.State != "success" || len(catalog.InstalledModels) != 1 || catalog.InstalledModels[0].ID != "consumer:latest" {
		t.Fatalf("catalog status=%d result=%#v", status, catalog)
	}
	status, body = invokeOllamaHandler(t, router, http.MethodPost, "/api/settings/ollama/model-readiness", `{"modelId":"consumer:latest"}`)
	var readiness ollamaModelReadinessResponse
	if err := json.Unmarshal(body, &readiness); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || readiness.State != "available" || !readiness.Available || readiness.Scope != "metadata_check" {
		t.Fatalf("readiness status=%d result=%#v", status, readiness)
	}
	// Independent feature selectors may share one installed model; this
	// consumer-neutral check does not create or change either selection.
	status, body = invokeOllamaHandler(t, router, http.MethodPost, "/api/settings/ollama/model-readiness", `{"modelId":"consumer:latest"}`)
	if err := json.Unmarshal(body, &readiness); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || readiness.State != "available" || readiness.ModelID != "consumer:latest" {
		t.Fatalf("shared model readiness status=%d result=%#v", status, readiness)
	}
	status, body = invokeOllamaHandler(t, router, http.MethodPost, "/api/settings/ollama/model-readiness", `{"modelId":"missing:latest"}`)
	if err := json.Unmarshal(body, &readiness); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusNotFound || readiness.State != "model_not_found" || readiness.Available || strings.Contains(readiness.Diagnostic, "private") {
		t.Fatalf("missing consumer model result status=%d result=%#v", status, readiness)
	}
	status, body = invokeOllamaHandler(t, router, http.MethodPost, "/api/settings/ollama/model-readiness", `{"modelId":"offline:latest"}`)
	if err := json.Unmarshal(body, &readiness); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusServiceUnavailable || readiness.State != "unavailable" || readiness.Available || strings.Contains(readiness.Diagnostic, "private") {
		t.Fatalf("offline consumer model result status=%d result=%#v", status, readiness)
	}
	if len(requests) != 5 || requests[0] != "GET /api/tags" || requests[1] != "POST /api/show" || requests[2] != "POST /api/show" || requests[3] != "POST /api/show" || requests[4] != "POST /api/show" {
		t.Fatalf("unexpected provider request sequence: %#v", requests)
	}
	var translation models.AppSetting
	if err := db.First(&translation, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if translation.Value["ollamaModel"] != "translategemma:12b" || translation.Value["enabled"] != true || translation.Value["readiness"] != string(translationReadinessReady) {
		t.Fatalf("discovery changed Translation model/readiness: %#v", translation.Value)
	}
}

func invokeOllamaHandler(t *testing.T, router http.Handler, method, path, body string) (int, []byte) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response.Code, response.Body.Bytes()
}
