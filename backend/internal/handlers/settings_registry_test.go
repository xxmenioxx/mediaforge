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
