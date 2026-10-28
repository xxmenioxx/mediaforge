package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSettingsRegistryValidationAndNormalization(t *testing.T) {
	workerDefinition := settingDefinitionFor("workers")
	if workerDefinition.Domain != "pipeline" || workerDefinition.Validate == nil {
		t.Fatalf("worker setting is missing canonical policy: %#v", workerDefinition)
	}
	if err := workerDefinition.Validate(settingsMutationContext{}, models.JSONMap{"jobStartPacingMode": "unknown"}); err == nil {
		t.Fatal("invalid worker pacing mode passed registry validation")
	}

	policyDefinition := settingDefinitionFor(profileSampleEstimatePolicySettingKey)
	if policyDefinition.Domain != "analysis" || policyDefinition.Normalize == nil {
		t.Fatalf("profile sample policy is missing canonical normalizer: %#v", policyDefinition)
	}
	normalized, err := policyDefinition.Normalize(settingsMutationContext{}, models.JSONMap{"softwareWindows": 2})
	if err != nil {
		t.Fatal(err)
	}
	if normalized["hardwareWindows"] != 5 || normalized["softwareWindows"] != 2 || normalized["operationTimeoutMinutes"] != 30 {
		t.Fatalf("unexpected registry normalized policy: %#v", normalized)
	}
	if settingDefinitionFor("legacyExistingSetting").Domain != "legacy" {
		t.Fatal("unknown legacy settings lost the compatibility definition")
	}

	resourceDefinition := settingDefinitionFor(airesource.SettingsKey)
	if resourceDefinition.Domain != "runtime" || resourceDefinition.Normalize == nil {
		t.Fatalf("resource policy setting is missing its backend contract: %#v", resourceDefinition)
	}
	resource, err := resourceDefinition.Normalize(settingsMutationContext{}, models.JSONMap{"maxResidentModels": 2})
	if err != nil {
		t.Fatal(err)
	}
	if resource["maxResidentModels"] != float64(2) || resource["maxConcurrentInferences"] != float64(1) {
		t.Fatalf("resource settings did not retain future limits and safe defaults: %#v", resource)
	}
	if _, err := resourceDefinition.Normalize(settingsMutationContext{}, models.JSONMap{"maxResidentModels": 0}); err == nil {
		t.Fatal("invalid resource limits passed backend settings normalization")
	}

	for key, domain := range map[string]string{
		"subtitleSettings":        "subtitles",
		translationSettingsKey:    "subtitles",
		ocrAISettingsKey:          "subtitles",
		aiCopilotSettingsKey:      "ai-copilot",
		ollamaProviderSettingsKey: "providers-integrations",
	} {
		if got := settingDefinitionFor(key).Domain; got != domain {
			t.Errorf("setting %q has domain %q, want %q", key, got, domain)
		}
	}
}

func TestResourceProtectionSettingUpdatePreservesUnrelatedSettings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-resource-protection?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProtectedCredential{}); err != nil {
		t.Fatal(err)
	}
	unrelated := models.AppSetting{Key: "unrelatedSetting", Value: models.JSONMap{"keep": "value"}}
	if err := db.Create(&unrelated).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewSettingsHandler(db)
	if _, err := handler.updateSettings(context.Background(), []SettingInput{{Key: airesource.SettingsKey, Value: models.JSONMap{"maxResidentModels": 2}}}); err != nil {
		t.Fatal(err)
	}
	var persisted models.AppSetting
	if err := db.First(&persisted, "key = ?", "unrelatedSetting").Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Value["keep"] != "value" {
		t.Fatalf("resource policy update changed unrelated setting: %#v", persisted.Value)
	}
	if _, err := handler.updateSettings(context.Background(), []SettingInput{{Key: airesource.SettingsKey, Value: models.JSONMap{"maxResidentModels": 0}}}); err == nil {
		t.Fatal("invalid resource policy update was accepted")
	}
	var resource models.AppSetting
	if err := db.First(&resource, "key = ?", airesource.SettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if resource.Value["maxResidentModels"] != float64(2) {
		t.Fatalf("invalid update overwrote valid policy: %#v", resource.Value)
	}
}

func TestAIResourceProtectionStatusAndPreviewExposeUnknownAuthorities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-resource-status?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.AIResourceAdmissionRecord{}, &models.AIResourceAdmissionCounter{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: airesource.SettingsKey, Value: airesource.DefaultSettingsValue()}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	settings := NewSettingsHandlerWithAdmission(db, airesource.NewService(db))
	router.GET("/settings/ai-resource-protection/status", settings.AIResourceProtectionStatus)
	router.POST("/settings/ai-resource-protection/preview", settings.PreviewAIResourceAdmission)

	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, httptest.NewRequest(http.MethodGet, "/settings/ai-resource-protection/status?capacityDomain=nas-a", nil))
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", statusResponse.Code, statusResponse.Body.String())
	}
	var status map[string]any
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	observations := status["observations"].(map[string]any)
	if observations["hostMemory"] != "unknown" || observations["providerResidency"] != "unknown" || observations["criticalServiceGuard"] != "unknown" {
		t.Fatalf("missing authorities should remain visibly unknown: %#v", observations)
	}
	server := status["ollamaServerConfiguration"].(map[string]any)
	if server["state"] != "unknown" || server["maxLoadedModels"] != nil || server["numParallel"] != nil {
		t.Fatalf("MVForge policy must not claim Ollama server values: %#v", server)
	}

	previewBody := `{"capacityDomain":"nas-a","providerConfigurationId":"provider-a","modelId":"model-a","consumerId":"settings-preview","workflowId":"settings-workflow","requestId":"preview-1","purpose":"MODEL_LOAD"}`
	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, httptest.NewRequest(http.MethodPost, "/settings/ai-resource-protection/preview", strings.NewReader(previewBody)))
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview map[string]any
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview["decision"] != string(airesource.Unavailable) || preview["reason"] != "resource_observation_unavailable" {
		t.Fatalf("preview should fail closed with explicit unavailable source: %#v", preview)
	}
	var records int64
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Count(&records).Error; err != nil {
		t.Fatal(err)
	}
	if records != 0 {
		t.Fatalf("HTTP preview created %d admission records", records)
	}
}

func TestSettingsBatchRollsBackAllValuesWhenWriteFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-batch-rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProtectedCredential{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_settings_batch BEFORE INSERT ON app_settings WHEN NEW.key = 'batch-fail' BEGIN SELECT RAISE(ABORT, 'forced batch failure'); END`).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/batch", NewSettingsHandler(db).BatchUpdate)
	request := httptest.NewRequest(http.MethodPost, "/api/settings/batch", strings.NewReader(`{"settings":[{"key":"batch-first","value":{"enabled":true}},{"key":"batch-fail","value":{"enabled":false}}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&models.AppSetting{}).Where("key IN ?", []string{"batch-first", "batch-fail"}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed batch left %d persisted settings", count)
	}
}
