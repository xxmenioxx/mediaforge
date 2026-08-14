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

func TestSettingsUpdateUpsertsExistingKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-upsert?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "trackProfilePathAssignments", Value: models.JSONMap{"assignments": models.JSONMap{"old": "profile"}}}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/trackProfilePathAssignments", strings.NewReader(`{"value":{"assignments":{"new":"profile"}}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var count int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "trackProfilePathAssignments").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings count=%d", count)
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "trackProfilePathAssignments").Error; err != nil {
		t.Fatal(err)
	}
	assignments, ok := setting.Value["assignments"].(map[string]any)
	if !ok || assignments["new"] != "profile" {
		t.Fatalf("unexpected value: %#v", setting.Value)
	}
}

func TestSettingsRejectsDisablingAssignedAudioProfile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-assigned-profile?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProfileAssignment{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: "path", TargetPath: "/media/raw/Show", MediaType: "audio", Selection: "profile", ProfileKey: "show-audio"}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/audioEnhancementProfiles", strings.NewReader(`{"value":{"profiles":[{"key":"show-audio","scope":"path","disabled":true}]}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "cannot be disabled") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMVForgePreferencesValidationRequiresDraftOnlyPreferenceFields(t *testing.T) {
	valid := models.JSONMap{
		"qualityGoal": "balanced", "executionPreference": "hardware", "preferredVideoEncoder": "hevc_qsv",
		"preferredLanguages": models.JSONList{"jpn", "spa", "eng"},
	}
	if err := validateMVForgePreferences(valid); err != nil {
		t.Fatalf("valid preferences rejected: %v", err)
	}
	invalid := models.JSONMap{"qualityGoal": "balanced", "executionPreference": "automatic", "preferredVideoEncoder": "auto", "preferredLanguages": models.JSONList{"jpn"}}
	if err := validateMVForgePreferences(invalid); err == nil {
		t.Fatal("unsupported execution preference was accepted")
	}
	mismatched := models.JSONMap{"qualityGoal": "balanced", "executionPreference": "hardware", "preferredVideoEncoder": "libx265", "preferredLanguages": models.JSONList{"jpn"}}
	if err := validateMVForgePreferences(mismatched); err == nil {
		t.Fatal("software encoder was accepted as a hardware preference")
	}
}

func TestOriginalRetentionPolicyValidation(t *testing.T) {
	valid := models.JSONMap{
		"keepOriginalsDays": 30, "autoDeleteEnabled": true,
		"processedOriginalsPath": "/media/originals_archive/processed-originals",
	}
	if err := validateOriginalRetentionPolicy(valid); err != nil {
		t.Fatalf("valid retention policy rejected: %v", err)
	}
	for name, value := range map[string]models.JSONMap{
		"negative days":  {"keepOriginalsDays": -1, "processedOriginalsPath": "/archive"},
		"relative path":  {"keepOriginalsDays": 30, "processedOriginalsPath": "archive"},
		"delete forever": {"keepOriginalsDays": 0, "autoDeleteEnabled": true, "processedOriginalsPath": "/archive"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateOriginalRetentionPolicy(value); err == nil {
				t.Fatal("invalid retention policy was accepted")
			}
		})
	}
}
