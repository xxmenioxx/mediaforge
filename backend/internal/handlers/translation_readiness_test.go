package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTranslationReadinessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:translation-readiness-" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func translationReadinessTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewSettingsHandler(db)
	router.POST("/api/settings/:key", handler.Update)
	router.POST("/api/settings/subtitle-translation/test-connection", handler.TestTranslationConnection)
	router.POST("/api/settings/subtitle-translation/enable", handler.EnableTranslation)
	return router
}

func persistTranslationReadinessSettings(t *testing.T, db *gorm.DB, settings translationSettings) {
	t.Helper()
	if err := saveTranslationSettings(db, settings); err != nil {
		t.Fatal(err)
	}
}

func postTranslationReadiness(t *testing.T, router http.Handler, path string) (int, translationReadinessResponseBody, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var body translationReadinessResponseBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response status=%d body=%s: %v", response.Code, response.Body.String(), err)
	}
	return response.Code, body, response.Body.String()
}

func TestTranslationReadinessDefaultsAndLegacyConfigurationRemainDisabled(t *testing.T) {
	settings := loadTranslationSettings(nil)
	if settings.Enabled || settings.Readiness != translationReadinessNotConfigured {
		t.Fatalf("missing settings=%#v, want disabled and not configured", settings)
	}
	db := newTranslationReadinessTestDB(t)
	if err := db.Create(&models.AppSetting{Key: translationSettingsKey, Value: models.JSONMap{
		"provider":       translationProviderOllama,
		"ollamaEndpoint": "http://ollama.example",
		"ollamaModel":    "model",
	}}).Error; err != nil {
		t.Fatal(err)
	}
	loaded := loadTranslationSettings(db)
	if loaded.Enabled || loaded.Readiness != translationReadinessUntested || loaded.ReadinessIdentity != "" {
		t.Fatalf("legacy configured settings=%#v, want disabled and untested", loaded)
	}
}

func TestTranslationReadinessTestConnectionChecksModelWithoutTranslation(t *testing.T) {
	var showRequests, chatRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			showRequests++
			var request ollamaModelRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode model request: %v", err)
			}
			if request.Model != "qwen2.5:7b" {
				t.Errorf("model=%q, want qwen2.5:7b", request.Model)
			}
			_, _ = io.WriteString(w, `{"modelfile":"model"}`)
		case "/api/chat":
			chatRequests++
			http.Error(w, "translation must not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db := newTranslationReadinessTestDB(t)
	settings := ollamaTestSettings(server.URL, "qwen2.5:7b")
	settings.Readiness = translationReadinessUntested
	persistTranslationReadinessSettings(t, db, settings)

	status, body, responseText := postTranslationReadiness(t, translationReadinessTestRouter(db), "/api/settings/subtitle-translation/test-connection")
	if status != http.StatusOK || body.Readiness != string(translationReadinessReady) || body.Enabled {
		t.Fatalf("status=%d body=%#v response=%s", status, body, responseText)
	}
	if showRequests != 1 || chatRequests != 0 {
		t.Fatalf("show requests=%d chat requests=%d", showRequests, chatRequests)
	}
	loaded := loadTranslationSettings(db)
	if loaded.Readiness != translationReadinessReady || loaded.Enabled || loaded.ReadinessIdentity != translationProviderConfigurationIdentity(loaded) {
		t.Fatalf("persisted readiness=%#v", loaded)
	}
}

func TestTranslationReadinessMissingModelAndUnavailableEndpointFailClosed(t *testing.T) {
	t.Run("model missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "private provider response", http.StatusNotFound)
		}))
		defer server.Close()
		db := newTranslationReadinessTestDB(t)
		settings := ollamaTestSettings(server.URL, "missing-model")
		persistTranslationReadinessSettings(t, db, settings)
		status, body, responseText := postTranslationReadiness(t, translationReadinessTestRouter(db), "/api/settings/subtitle-translation/test-connection")
		if status != http.StatusOK || body.Readiness != string(translationReadinessModelNotFound) || body.Enabled {
			t.Fatalf("status=%d body=%#v", status, body)
		}
		if strings.Contains(responseText, "private provider response") {
			t.Fatalf("response leaked provider body: %s", responseText)
		}
	})

	t.Run("unavailable endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		endpoint := server.URL
		server.Close()
		db := newTranslationReadinessTestDB(t)
		persistTranslationReadinessSettings(t, db, ollamaTestSettings(endpoint, "model"))
		status, body, _ := postTranslationReadiness(t, translationReadinessTestRouter(db), "/api/settings/subtitle-translation/test-connection")
		if status != http.StatusOK || body.Readiness != string(translationReadinessUnavailable) || body.Enabled {
			t.Fatalf("status=%d body=%#v", status, body)
		}
	})
}

func TestTranslationEnableRevalidatesAndDoesNotTrustStaleReadiness(t *testing.T) {
	var showRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			http.NotFound(w, r)
			return
		}
		showRequests++
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	db := newTranslationReadinessTestDB(t)
	settings := ollamaTestSettings(server.URL, "model")
	settings.Readiness = translationReadinessReady
	settings.ReadinessIdentity = "stale"
	persistTranslationReadinessSettings(t, db, settings)
	status, body, _ := postTranslationReadiness(t, translationReadinessTestRouter(db), "/api/settings/subtitle-translation/enable")
	if status != http.StatusOK || !body.Enabled || body.Readiness != string(translationReadinessReady) {
		t.Fatalf("status=%d body=%#v", status, body)
	}
	if showRequests != 1 {
		t.Fatalf("readiness requests=%d, want one", showRequests)
	}
	if loaded := loadTranslationSettings(db); !loaded.Enabled {
		t.Fatalf("successful enable was not persisted: %#v", loaded)
	}
}

func TestTranslationEnableRequiresConfiguredProvider(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	status, body, _ := postTranslationReadiness(t, translationReadinessTestRouter(db), "/api/settings/subtitle-translation/enable")
	if status != http.StatusBadRequest || body.Enabled || body.Readiness != string(translationReadinessNotConfigured) {
		t.Fatalf("status=%d body=%#v", status, body)
	}
}

func TestTranslationGenericSettingsCannotBypassEnablement(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	router := translationReadinessTestRouter(db)
	request := httptest.NewRequest(http.MethodPost, "/api/settings/"+translationSettingsKey, strings.NewReader(`{"value":{"provider":"ollama","ollamaEndpoint":"http://ollama.example","ollamaModel":"model","enabled":true}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want bad request", response.Code, response.Body.String())
	}
	if loaded := loadTranslationSettings(db); loaded.Enabled {
		t.Fatalf("generic settings update enabled translation: %#v", loaded)
	}
}

func TestTranslationSettingsProviderChangesInvalidateReadinessButPolicyChangesDoNot(t *testing.T) {
	for _, test := range []struct {
		name  string
		value models.JSONMap
	}{
		{name: "provider", value: models.JSONMap{"provider": ""}},
		{name: "endpoint", value: models.JSONMap{"provider": translationProviderOllama, "ollamaEndpoint": "http://new.example", "ollamaModel": "model"}},
		{name: "model", value: models.JSONMap{"provider": translationProviderOllama, "ollamaEndpoint": "http://ollama.example", "ollamaModel": "new-model"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := newTranslationReadinessTestDB(t)
			settings := ollamaTestSettings("http://ollama.example", "model")
			settings.Enabled = true
			settings.Readiness = translationReadinessReady
			settings.ReadinessIdentity = translationProviderConfigurationIdentity(settings)
			persistTranslationReadinessSettings(t, db, settings)
			router := translationReadinessTestRouter(db)
			request := httptest.NewRequest(http.MethodPost, "/api/settings/"+translationSettingsKey, strings.NewReader(mustJSONSettingsValue(t, test.value)))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			loaded := loadTranslationSettings(db)
			if loaded.Enabled || loaded.Readiness == translationReadinessReady || loaded.ReadinessIdentity != "" {
				t.Fatalf("configuration change retained readiness: %#v", loaded)
			}
		})
	}

	t.Run("bounded policy", func(t *testing.T) {
		db := newTranslationReadinessTestDB(t)
		settings := ollamaTestSettings("http://ollama.example", "model")
		settings.Enabled = true
		settings.Readiness = translationReadinessReady
		settings.ReadinessIdentity = translationProviderConfigurationIdentity(settings)
		persistTranslationReadinessSettings(t, db, settings)
		value := models.JSONMap{
			"provider":          translationProviderOllama,
			"ollamaEndpoint":    settings.OllamaEndpoint,
			"ollamaModel":       settings.OllamaModel,
			"maxCuesPerRequest": settings.MaxCuesPerRequest + 1,
		}
		request := httptest.NewRequest(http.MethodPost, "/api/settings/"+translationSettingsKey, strings.NewReader(mustJSONSettingsValue(t, value)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		translationReadinessTestRouter(db).ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		loaded := loadTranslationSettings(db)
		if !loaded.Enabled || loaded.Readiness != translationReadinessReady || loaded.ReadinessIdentity != settings.ReadinessIdentity {
			t.Fatalf("policy change invalidated readiness: %#v", loaded)
		}
	})
}

func mustJSONSettingsValue(t *testing.T, value models.JSONMap) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]models.JSONMap{"value": value})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestTranslationReadinessCancellationAndFactorySeam(t *testing.T) {
	started := make(chan struct{})
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), &http.Client{
		Transport: ollamaRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan TranslationProviderReadiness, 1)
	go func() { resultCh <- provider.TestConnection(ctx) }()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("readiness request did not start")
	}
	select {
	case result := <-resultCh:
		if result.State != translationReadinessUnavailable || !strings.Contains(result.Diagnostic, "cancelled") {
			t.Fatalf("result=%#v, want cancelled unavailable", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled readiness request did not return")
	}

	checker, err := translationProviderReadinessCheckerForSettings(ollamaTestSettings("http://ollama.test", "model"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := checker.(*ollamaTranslationProvider); !ok {
		t.Fatalf("checker type=%T, want existing Ollama adapter", checker)
	}
	if _, err := translationProviderReadinessCheckerForSettings(translationSettings{Provider: translationProviderOpenAI}); err == nil || translationProviderErrorClassOf(err) != translationProviderErrorConfiguration {
		t.Fatalf("future readiness provider did not fail as configuration: %v", err)
	}
}

func TestTranslationReadinessClientTimeoutIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings(server.URL, "model"), &http.Client{Timeout: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	result := provider.TestConnection(context.Background())
	if result.State != translationReadinessUnavailable || !strings.Contains(result.Diagnostic, "timed out") {
		t.Fatalf("result=%#v, want timeout diagnostic", result)
	}
}
