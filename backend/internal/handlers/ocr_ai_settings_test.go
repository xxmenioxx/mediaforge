package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestOCRAISettingsRegistryNormalizesAndValidatesIndependentSelection(t *testing.T) {
	definition := settingDefinitionFor(ocrAISettingsKey)
	if definition.Domain != "pipeline" || definition.Normalize == nil {
		t.Fatalf("OCR AI setting is missing its canonical policy: %#v", definition)
	}

	for name, value := range map[string]models.JSONMap{
		"disabled without a model":       {"enabled": false, "model": ""},
		"disabled with a selected model": {"enabled": false, "model": "  qwen2.5:7b  ", "future": "preserved"},
		"enabled with a selected model":  {"enabled": true, "model": "qwen2.5:7b"},
	} {
		t.Run(name, func(t *testing.T) {
			normalized, err := definition.Normalize(settingsMutationContext{}, value)
			if err != nil {
				t.Fatal(err)
			}
			if normalized["enabled"] != value["enabled"] {
				t.Fatalf("enabled changed during normalization: %#v", normalized)
			}
			model, _ := value["model"].(string)
			if normalized["model"] != strings.TrimSpace(model) {
				t.Fatalf("model was not normalized: %#v", normalized)
			}
			if extension, present := value["future"]; present && normalized["future"] != extension {
				t.Fatalf("extension field was not preserved: %#v", normalized)
			}
		})
	}

	for name, value := range map[string]models.JSONMap{
		"non-boolean enabled":           {"enabled": "true"},
		"non-string model":              {"model": 7},
		"model with newline":            {"model": "model\nother"},
		"oversized model":               {"model": strings.Repeat("a", ocrAIModelMaxLength+1)},
		"enabled without model":         {"enabled": true},
		"enabled with empty model":      {"enabled": true, "model": ""},
		"enabled with whitespace model": {"enabled": true, "model": " \t  "},
		"enabled with invalid model":    {"enabled": true, "model": "model\nother"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := definition.Normalize(settingsMutationContext{}, value); err == nil {
				t.Fatalf("invalid OCR AI setting passed normalization: %#v", value)
			}
		})
	}
}

func TestOCRAISettingsAPIRejectsInvalidEnabledSelectionAndReloadsValidValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ocr-ai-settings-api?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProtectedCredential{}); err != nil {
		t.Fatal(err)
	}
	translation := models.AppSetting{Key: translationSettingsKey, Value: models.JSONMap{
		"provider": "ollama", "ollamaModel": "translategemma:12b", "enabled": true, "future": "keep",
	}}
	if err := db.Create(&translation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: ocrAISettingsKey, Value: models.JSONMap{"enabled": false, "model": "qwen2.5:7b"}}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewSettingsHandler(db)
	router.POST("/api/settings/:key", handler.Update)
	router.GET("/api/settings", handler.List)

	post := func(key, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/settings/"+key, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	for _, body := range []string{
		`{"value":{"enabled":true,"model":""}}`,
		`{"value":{"enabled":true,"model":" \t "}}`,
		`{"value":{"enabled":true,"model":"model\nother"}}`,
		`{"value":{"enabled":true,"model":7}}`,
	} {
		response := post(ocrAISettingsKey, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid OCR AI update status=%d body=%s request=%s", response.Code, response.Body.String(), body)
		}
	}

	var unchangedOCR, unchangedTranslation models.AppSetting
	if err := db.First(&unchangedOCR, "key = ?", ocrAISettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&unchangedTranslation, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedOCR.Value["enabled"] != false || unchangedOCR.Value["model"] != "qwen2.5:7b" {
		t.Fatalf("rejected API updates changed persisted OCR AI settings: %#v", unchangedOCR.Value)
	}
	if !reflect.DeepEqual(unchangedTranslation.Value, translation.Value) {
		t.Fatalf("rejected API updates changed Translation settings: got %#v, want %#v", unchangedTranslation.Value, translation.Value)
	}

	for _, test := range []struct {
		name       string
		body       string
		wantEnable bool
		wantModel  string
	}{
		{name: "disabled empty model", body: `{"value":{"enabled":false,"model":""}}`, wantModel: ""},
		{name: "disabled retained model", body: `{"value":{"enabled":false,"model":"qwen2.5:7b"}}`, wantModel: "qwen2.5:7b"},
		{name: "enabled selected model", body: `{"value":{"enabled":true,"model":"qwen2.5:7b"}}`, wantEnable: true, wantModel: "qwen2.5:7b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := post(ocrAISettingsKey, test.body)
			if response.Code != http.StatusOK {
				t.Fatalf("valid OCR AI update status=%d body=%s", response.Code, response.Body.String())
			}
			var saved models.AppSetting
			if err := db.First(&saved, "key = ?", ocrAISettingsKey).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Value["enabled"] != test.wantEnable || saved.Value["model"] != test.wantModel {
				t.Fatalf("valid update persisted %#v, want enabled=%t model=%q", saved.Value, test.wantEnable, test.wantModel)
			}
		})
	}
	// Map iteration order is unspecified; leave a known enabled value for the reload assertion.
	enabledResponse := post(ocrAISettingsKey, `{"value":{"enabled":true,"model":"qwen2.5:7b"}}`)
	if enabledResponse.Code != http.StatusOK {
		t.Fatalf("enabled OCR AI update status=%d body=%s", enabledResponse.Code, enabledResponse.Body.String())
	}

	// Listing is the Settings reload path and must expose the persisted independent value.
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("Settings reload status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var settings []models.AppSetting
	if err := json.Unmarshal(listResponse.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	var reloadedOCR models.AppSetting
	for _, setting := range settings {
		if setting.Key == ocrAISettingsKey {
			reloadedOCR = setting
			break
		}
	}
	if reloadedOCR.Key != ocrAISettingsKey || reloadedOCR.Value["enabled"] != true || reloadedOCR.Value["model"] != "qwen2.5:7b" {
		t.Fatalf("Settings reload did not return the saved OCR AI choice: %#v", reloadedOCR)
	}
	if err := db.First(&unchangedTranslation, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(unchangedTranslation.Value, translation.Value) {
		t.Fatalf("valid OCR AI updates changed Translation settings: got %#v, want %#v", unchangedTranslation.Value, translation.Value)
	}
}

func TestOCRAISettingsBatchRejectsBeforeAnyWrites(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ocr-ai-settings-batch-atomic?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProtectedCredential{}); err != nil {
		t.Fatal(err)
	}
	translation := models.AppSetting{Key: translationSettingsKey, Value: models.JSONMap{
		"provider": "ollama", "ollamaModel": "translategemma:12b", "enabled": true,
	}}
	ocr := models.AppSetting{Key: ocrAISettingsKey, Value: models.JSONMap{"enabled": false, "model": "keep-model"}}
	if err := db.Create(&translation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ocr).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/batch", NewSettingsHandler(db).BatchUpdate)
	request := httptest.NewRequest(http.MethodPost, "/api/settings/batch", strings.NewReader(`{"settings":[{"key":"batch-should-not-persist","value":{"enabled":true}},{"key":"ocrAISettings","value":{"enabled":true,"model":"   "}}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("batch status=%d body=%s", response.Code, response.Body.String())
	}
	var created int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "batch-should-not-persist").Count(&created).Error; err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("invalid batch partially persisted an unrelated setting")
	}
	var savedOCR, savedTranslation models.AppSetting
	if err := db.First(&savedOCR, "key = ?", ocrAISettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&savedTranslation, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(savedOCR.Value, ocr.Value) {
		t.Fatalf("invalid batch changed OCR AI settings: got %#v, want %#v", savedOCR.Value, ocr.Value)
	}
	if !reflect.DeepEqual(savedTranslation.Value, translation.Value) {
		t.Fatalf("invalid batch changed Translation settings: got %#v, want %#v", savedTranslation.Value, translation.Value)
	}
}

func TestMigrateOCRAISettingsAddsDisabledDefaultWithoutChangingTranslation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}

	translation := models.AppSetting{
		Key: translationSettingsKey,
		Value: models.JSONMap{
			"provider":       "ollama",
			"ollamaEndpoint": "http://ollama.local:11434",
			"ollamaModel":    "translategemma:12b",
			"enabled":        true,
		},
	}
	if err := db.Create(&translation).Error; err != nil {
		t.Fatal(err)
	}

	if err := migrateOCRAISettings(db); err != nil {
		t.Fatal(err)
	}
	var savedOCR models.AppSetting
	if err := db.First(&savedOCR, "key = ?", ocrAISettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if savedOCR.Value["enabled"] != false || savedOCR.Value["model"] != "" {
		t.Fatalf("new OCR AI setting must start disabled and unselected: %#v", savedOCR.Value)
	}

	var savedTranslation models.AppSetting
	if err := db.First(&savedTranslation, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(savedTranslation.Value, translation.Value) {
		t.Fatalf("Translation configuration changed during OCR AI migration: got %#v, want %#v", savedTranslation.Value, translation.Value)
	}

	// Re-running the migration must not reset a later explicit OCR AI choice.
	if err := db.Model(&models.AppSetting{}).Where("key = ?", ocrAISettingsKey).Update("value", models.JSONMap{"enabled": false, "model": "qwen2.5:7b"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateOCRAISettings(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&savedOCR, "key = ?", ocrAISettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if savedOCR.Value["enabled"] != false || savedOCR.Value["model"] != "qwen2.5:7b" {
		t.Fatalf("idempotent migration replaced the saved OCR AI selection: %#v", savedOCR.Value)
	}
}
