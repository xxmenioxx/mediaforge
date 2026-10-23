package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTranslationSettingsMissingAreUnavailableAndSafe(t *testing.T) {
	settings := loadTranslationSettings(nil)
	if settings.Provider != "" || settings.OllamaEndpoint != "" || settings.OllamaModel != "" {
		t.Fatalf("missing settings unexpectedly enabled translation: %#v", settings)
	}
	if settings.MaxCuesPerRequest != translationDefaultMaxCuesPerRequest || settings.MaxRequestBytes != translationDefaultMaxRequestBytes || settings.MaxContextCues != translationDefaultMaxContextCues || settings.MaxContextChars != translationDefaultMaxContextChars {
		t.Fatalf("missing settings did not receive bounded defaults: %#v", settings)
	}

	normalized, err := normalizeTranslationSettingsValue(models.JSONMap{})
	if err != nil {
		t.Fatal(err)
	}
	if _, hasEndpoint := normalized["ollamaEndpoint"]; hasEndpoint || normalized["ollamaModel"] != "" {
		t.Fatalf("defaults invented endpoint/model: %#v", normalized)
	}
}

func TestTranslationSettingsPersistAndReloadExactValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:translation-settings-persistence?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	want := models.JSONMap{
		"provider":             translationProviderOllama,
		"ollamaEndpoint":       "https://ollama.lan:11434/api",
		"ollamaModel":          "qwen2.5:7b",
		"qualityReviewerModel": "reviewer:latest",
		"maxCuesPerRequest":    24,
		"maxRequestBytes":      32768,
		"maxContextCues":       6,
		"maxContextChars":      2048,
	}
	request := httptest.NewRequest(http.MethodPost, "/api/settings/"+translationSettingsKey, strings.NewReader(`{"value":{"provider":"ollama","ollamaEndpoint":"https://ollama.lan:11434/api","ollamaModel":"qwen2.5:7b","qualityReviewerModel":"reviewer:latest","maxCuesPerRequest":24,"maxRequestBytes":32768,"maxContextCues":6,"maxContextChars":2048}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	loaded := loadTranslationSettings(db)
	if loaded.Provider != want["provider"] || loaded.OllamaEndpoint != want["ollamaEndpoint"] || loaded.OllamaModel != want["ollamaModel"] || loaded.QualityReviewerModel != want["qualityReviewerModel"] || loaded.MaxCuesPerRequest != 24 || loaded.MaxRequestBytes != 32768 || loaded.MaxContextCues != 6 || loaded.MaxContextChars != 2048 {
		t.Fatalf("reloaded settings differ: %#v", loaded)
	}
}

func TestTranslationSettingsRejectCredentialsAndUnsafePolicy(t *testing.T) {
	for name, value := range map[string]models.JSONMap{
		"credentials":       {"ollamaEndpoint": "https://user:secret@ollama.lan:11434"},
		"too many cues":     {"maxCuesPerRequest": 129},
		"too large request": {"maxRequestBytes": 1024*1024 + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeTranslationSettingsValue(value); err == nil {
				t.Fatalf("unsafe setting accepted: %#v", value)
			}
		})
	}
}

func TestTranslationSettingsMalformedLegacyValueFallsBackSafely(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:translation-settings-legacy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: translationSettingsKey, Value: models.JSONMap{"provider": "unknown", "ollamaEndpoint": "localhost:11434"}}).Error; err != nil {
		t.Fatal(err)
	}
	settings := loadTranslationSettings(db)
	if settings.Provider != "" || settings.OllamaEndpoint != "" || settings.OllamaModel != "" {
		t.Fatalf("malformed legacy value enabled translation: %#v", settings)
	}
	if settings.MaxCuesPerRequest != translationDefaultMaxCuesPerRequest {
		t.Fatalf("malformed legacy value lost safe defaults: %#v", settings)
	}
}

func TestTranslationSettingsMalformedPersistedValuesNeverRemainUsable(t *testing.T) {
	longModel := strings.Repeat("m", 257)
	for name, value := range map[string]models.JSONMap{
		"invalid endpoint": {
			"provider":       translationProviderOllama,
			"ollamaEndpoint": "not-a-url",
			"ollamaModel":    "qwen2.5:7b",
		},
		"oversized model": {
			"provider":       translationProviderOllama,
			"ollamaEndpoint": "https://ollama.lan:11434",
			"ollamaModel":    longModel,
		},
		"invalid policy": {
			"provider":        translationProviderOllama,
			"ollamaEndpoint":  "https://ollama.lan:11434",
			"ollamaModel":     "qwen2.5:7b",
			"maxContextChars": 16385,
		},
		"unsupported provider": {
			"provider":       "openai",
			"ollamaEndpoint": "https://ollama.lan:11434",
			"ollamaModel":    "qwen2.5:7b",
		},
	} {
		t.Run(name, func(t *testing.T) {
			settings, err := translationSettingsFromValue(value, false)
			if err != nil {
				t.Fatalf("non-strict parsing returned error: %v", err)
			}
			if translationSettingsUsable(settings) {
				t.Fatalf("malformed persisted value remained usable: %#v", settings)
			}
			if settings.Provider != "" || settings.OllamaEndpoint != "" || settings.OllamaModel != "" {
				t.Fatalf("malformed persisted value retained partial configuration: %#v", settings)
			}
		})
	}
}
