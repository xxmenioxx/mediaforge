package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestOllamaProviderSettingsEndpointValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   models.JSONMap
		valid   bool
		wantURL string
	}{
		{name: "remote HTTP", value: models.JSONMap{"endpoint": "http://ollama.example:11434"}, valid: true, wantURL: "http://ollama.example:11434"},
		{name: "remote HTTPS", value: models.JSONMap{"endpoint": "https://ollama.example/api"}, valid: true, wantURL: "https://ollama.example/api"},
		{name: "empty configuration", value: models.JSONMap{"endpoint": ""}, valid: true},
		{name: "missing endpoint", value: models.JSONMap{}, valid: false},
		{name: "endpoint is not a string", value: models.JSONMap{"endpoint": 42}, valid: false},
		{name: "unsupported scheme", value: models.JSONMap{"endpoint": "ftp://ollama.example"}, valid: false},
		{name: "missing host", value: models.JSONMap{"endpoint": "http:///api"}, valid: false},
		{name: "credentials", value: models.JSONMap{"endpoint": "https://user:secret@ollama.example"}, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := normalizeOllamaProviderSettingsValue(test.value)
			if test.valid && err != nil {
				t.Fatalf("normalize endpoint: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatalf("expected endpoint validation failure")
			}
			if test.valid && normalized["endpoint"] != test.wantURL {
				t.Fatalf("endpoint=%#v, want %q", normalized["endpoint"], test.wantURL)
			}
		})
	}

	db := newTranslationReadinessTestDB(t)
	configuration, err := loadOllamaProviderConfiguration(db)
	if err != nil || configuration.State != ollamaProviderStateUnconfigured || configuration.Endpoint != "" {
		t.Fatalf("missing shared configuration=%#v err=%v", configuration, err)
	}
	configured, err := normalizeOllamaProviderSettingsValue(models.JSONMap{"endpoint": "https://ollama.example", "futureProviderField": "preserved"})
	if err != nil {
		t.Fatal(err)
	}
	if configured["futureProviderField"] != "preserved" {
		t.Fatalf("future provider field was lost: %#v", configured)
	}
	if _, hasModelChoice := configured["model"]; hasModelChoice {
		t.Fatalf("endpoint normalization invented a consumer model choice: %#v", configured)
	}
}

func TestMigrateOllamaProviderSettingsPreservesTranslationAndUnrelatedSettings(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	endpoint := "https://ollama.example:11434/api"
	seedLegacyOllamaTranslation(t, db, endpoint)
	if err := db.Create(&models.AppSetting{Key: "unrelatedSetting", Value: models.JSONMap{"keep": true}}).Error; err != nil {
		t.Fatal(err)
	}

	if err := MigrateOllamaProviderSettings(db); err != nil {
		t.Fatal(err)
	}
	var shared, legacy, unrelated models.AppSetting
	if err := db.First(&shared, "key = ?", ollamaProviderSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if shared.Value["endpoint"] != endpoint {
		t.Fatalf("shared endpoint=%#v", shared.Value["endpoint"])
	}
	if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, duplicate := legacy.Value["ollamaEndpoint"]; duplicate {
		t.Fatalf("legacy endpoint remained persisted: %#v", legacy.Value)
	}
	if legacy.Value["ollamaModel"] != "translategemma:12b" || legacy.Value["enabled"] != true || !settingIntegerEquals(legacy.Value, "maxCuesPerRequest", 23) || !settingIntegerEquals(legacy.Value, "maxRequestBytes", 32768) || !settingIntegerEquals(legacy.Value, "maxContextCues", 5) || !settingIntegerEquals(legacy.Value, "maxContextChars", 1536) {
		t.Fatalf("Translation settings changed during migration: %#v", legacy.Value)
	}
	if legacy.Value["readiness"] != string(translationReadinessReady) || legacy.Value["readinessConfigIdentity"] != translationProviderConfigurationIdentity(translationSettings{Provider: translationProviderOllama, OllamaEndpoint: endpoint, OllamaModel: "translategemma:12b"}) {
		t.Fatalf("matching readiness was not preserved: %#v", legacy.Value)
	}
	roleFields, _ := json.Marshal(models.JSONMap{"primary": "translator", "future": []any{"fast", "fallback"}})
	storedRoleFields, _ := json.Marshal(legacy.Value["ollamaModelRoles"])
	if !bytes.Equal(storedRoleFields, roleFields) {
		t.Fatalf("future model-role fields changed: %s want %s", storedRoleFields, roleFields)
	}
	if err := db.First(&unrelated, "key = ?", "unrelatedSetting").Error; err != nil || unrelated.Value["keep"] != true {
		t.Fatalf("unrelated setting changed: %#v err=%v", unrelated, err)
	}
	loaded := loadTranslationSettings(db)
	if loaded.OllamaEndpoint != endpoint || loaded.OllamaModel != "translategemma:12b" || !loaded.Enabled || loaded.Readiness != translationReadinessReady {
		t.Fatalf("migrated Translation runtime settings=%#v", loaded)
	}
	if err := MigrateOllamaProviderSettings(db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, duplicate := legacy.Value["ollamaEndpoint"]; duplicate {
		t.Fatalf("idempotent migration restored legacy endpoint: %#v", legacy.Value)
	}
}

func TestMigrateOllamaProviderSettingsEqualInvalidAndConflictStates(t *testing.T) {
	t.Run("equal endpoints collapse to shared setting", func(t *testing.T) {
		db := newTranslationReadinessTestDB(t)
		seedLegacyOllamaTranslation(t, db, "http://ollama.example")
		if err := db.Create(&models.AppSetting{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": "http://ollama.example"}}).Error; err != nil {
			t.Fatal(err)
		}
		if err := MigrateOllamaProviderSettings(db); err != nil {
			t.Fatal(err)
		}
		var legacy models.AppSetting
		if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
			t.Fatal(err)
		}
		if _, present := legacy.Value["ollamaEndpoint"]; present {
			t.Fatalf("duplicate endpoint was not removed: %#v", legacy.Value)
		}
	})

	t.Run("conflicting endpoints fail closed and retain both values", func(t *testing.T) {
		db := newTranslationReadinessTestDB(t)
		seedLegacyOllamaTranslation(t, db, "http://legacy.example")
		if err := db.Create(&models.AppSetting{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": "http://shared.example"}}).Error; err != nil {
			t.Fatal(err)
		}
		if err := MigrateOllamaProviderSettings(db); err != nil {
			t.Fatal(err)
		}
		configuration, err := loadOllamaProviderConfiguration(db)
		if err != nil || configuration.State != ollamaProviderStateMigrationConflict || configuration.Endpoint != "" {
			t.Fatalf("conflicting configuration=%#v err=%v", configuration, err)
		}
		var legacy, shared models.AppSetting
		if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.First(&shared, "key = ?", ollamaProviderSettingsKey).Error; err != nil {
			t.Fatal(err)
		}
		if legacy.Value["ollamaEndpoint"] != "http://legacy.example" || shared.Value["endpoint"] != "http://shared.example" {
			t.Fatalf("conflict values were discarded: legacy=%#v shared=%#v", legacy.Value, shared.Value)
		}
		loaded := loadTranslationSettings(db)
		if loaded.OllamaEndpoint != "" || loaded.OllamaModel != "translategemma:12b" || !loaded.Enabled {
			t.Fatalf("conflicting Translation configuration was not preserved while blocked: %#v", loaded)
		}
		if available, _ := translationCapabilityAvailability(loaded); available {
			t.Fatal("conflicting provider endpoints were treated as usable")
		}
		projection, err := translationSettingsProjection(db, legacy.Value)
		if err != nil {
			t.Fatal(err)
		}
		if projection["ollamaEndpoint"] != "http://shared.example" || projection["ollamaProviderState"] != ollamaProviderStateMigrationConflict || projection["enabled"] != false || projection["readiness"] != string(translationReadinessInvalidConfiguration) {
			t.Fatalf("legacy Settings projection did not show fail-closed conflict: %#v", projection)
		}
		var persisted models.AppSetting
		if err := db.First(&persisted, "key = ?", translationSettingsKey).Error; err != nil {
			t.Fatal(err)
		}
		if persisted.Value["enabled"] != true || persisted.Value["readiness"] != string(translationReadinessReady) {
			t.Fatalf("conflict projection mutated saved Translation state: %#v", persisted.Value)
		}
	})

	t.Run("invalid legacy endpoint remains explicit and unusable", func(t *testing.T) {
		db := newTranslationReadinessTestDB(t)
		seedLegacyOllamaTranslation(t, db, "ftp://legacy.example")
		if err := MigrateOllamaProviderSettings(db); err != nil {
			t.Fatal(err)
		}
		configuration, err := loadOllamaProviderConfiguration(db)
		if err != nil || configuration.State != ollamaProviderStateInvalid || configuration.Endpoint != "" {
			t.Fatalf("invalid legacy configuration=%#v err=%v", configuration, err)
		}
		var sharedCount int64
		if err := db.Model(&models.AppSetting{}).Where("key = ?", ollamaProviderSettingsKey).Count(&sharedCount).Error; err != nil {
			t.Fatal(err)
		}
		if sharedCount != 0 {
			t.Fatalf("invalid endpoint created shared configuration: count=%d", sharedCount)
		}
	})

	t.Run("malformed canonical setting is invalid rather than unconfigured", func(t *testing.T) {
		db := newTranslationReadinessTestDB(t)
		if err := db.Create(&models.AppSetting{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"futureField": true}}).Error; err != nil {
			t.Fatal(err)
		}
		configuration, err := loadOllamaProviderConfiguration(db)
		if err != nil || configuration.State != ollamaProviderStateInvalid {
			t.Fatalf("malformed canonical setting=%#v err=%v", configuration, err)
		}
	})

	t.Run("absent configuration remains unconfigured", func(t *testing.T) {
		db := newTranslationReadinessTestDB(t)
		if err := MigrateOllamaProviderSettings(db); err != nil {
			t.Fatal(err)
		}
		configuration, err := loadOllamaProviderConfiguration(db)
		if err != nil || configuration.State != ollamaProviderStateUnconfigured {
			t.Fatalf("absent configuration=%#v err=%v", configuration, err)
		}
	})
}

func TestMigrateOllamaProviderSettingsRollsBackPartialMigration(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	seedLegacyOllamaTranslation(t, db, "http://ollama.example")
	if err := db.Exec(`CREATE TRIGGER reject_translation_migration BEFORE UPDATE ON app_settings WHEN OLD.key = 'subtitleTranslationSettings' BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateOllamaProviderSettings(db); err == nil {
		t.Fatal("expected migration update failure")
	}
	var sharedCount int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", ollamaProviderSettingsKey).Count(&sharedCount).Error; err != nil {
		t.Fatal(err)
	}
	if sharedCount != 0 {
		t.Fatalf("partial migration left shared endpoint behind: count=%d", sharedCount)
	}
}

func TestEmptyLegacyEndpointMigrationPreservesSavedTranslationModelAndLimits(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	seedLegacyOllamaTranslation(t, db, "")
	if err := MigrateOllamaProviderSettings(db); err != nil {
		t.Fatal(err)
	}
	loaded := loadTranslationSettings(db)
	if loaded.ConfigurationState != ollamaProviderStateUnconfigured || loaded.OllamaEndpoint != "" || loaded.OllamaModel != "translategemma:12b" || loaded.MaxCuesPerRequest != 23 || loaded.MaxRequestBytes != 32768 {
		t.Fatalf("saved Translation model/policy was lost while unconfigured: %#v", loaded)
	}
	var legacy models.AppSetting
	if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.Value["enabled"] != true || !settingIntegerEquals(legacy.Value, "maxCuesPerRequest", 23) {
		t.Fatalf("unconfigured migration changed saved enablement/policy: %#v", legacy.Value)
	}
}

func TestLegacyTranslationSettingsAPIProjectsSharedEndpointWithoutDuplicateStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTranslationReadinessTestDB(t)
	handler := NewSettingsHandler(db)
	router := gin.New()
	router.POST("/api/settings/:key", handler.Update)
	router.GET("/api/settings", handler.List)
	requestBody := `{"value":{"provider":"ollama","ollamaEndpoint":"https://ollama.example:11434","ollamaModel":"translategemma:12b","maxCuesPerRequest":23,"maxRequestBytes":32768,"maxContextCues":5,"maxContextChars":1536,"ollamaModelRoles":{"primary":"translator","fallback":"future-model"}}}`
	post := httptest.NewRequest(http.MethodPost, "/api/settings/"+translationSettingsKey, strings.NewReader(requestBody))
	post.Header.Set("Content-Type", "application/json")
	postResponse := httptest.NewRecorder()
	router.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", postResponse.Code, postResponse.Body.String())
	}
	var saved models.AppSetting
	if err := json.Unmarshal(postResponse.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Value["ollamaEndpoint"] != "https://ollama.example:11434" {
		t.Fatalf("legacy response endpoint projection=%#v", saved.Value)
	}
	var legacy, shared models.AppSetting
	if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, duplicate := legacy.Value["ollamaEndpoint"]; duplicate {
		t.Fatalf("legacy endpoint was persisted a second time: %#v", legacy.Value)
	}
	if _, hasFutureRoleField := legacy.Value["ollamaModelRoles"]; !hasFutureRoleField {
		t.Fatalf("future model-role field was lost on legacy Settings write: %#v", legacy.Value)
	}
	if err := db.First(&shared, "key = ?", ollamaProviderSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if shared.Value["endpoint"] != "https://ollama.example:11434" {
		t.Fatalf("canonical endpoint=%#v", shared.Value)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("refetch status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
	var settings []models.AppSetting
	if err := json.Unmarshal(getResponse.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	foundTranslation := false
	for _, setting := range settings {
		if setting.Key == translationSettingsKey && setting.Value["ollamaEndpoint"] != "https://ollama.example:11434" {
			t.Fatalf("refetched legacy endpoint projection=%#v", setting.Value)
		}
		if setting.Key == translationSettingsKey {
			foundTranslation = true
		}
	}
	if !foundTranslation {
		t.Fatalf("refetch omitted legacy Translation setting: %#v", settings)
	}
	if loaded := loadTranslationSettings(db); loaded.OllamaModel != "translategemma:12b" || loaded.MaxCuesPerRequest != 23 {
		t.Fatalf("Translation selection or policy did not survive refetch: %#v", loaded)
	}
	minimalSave := httptest.NewRequest(http.MethodPost, "/api/settings/"+translationSettingsKey, strings.NewReader(`{"value":{"provider":"ollama","ollamaEndpoint":"https://ollama.example:11434","ollamaModel":"translategemma:14b"}}`))
	minimalSave.Header.Set("Content-Type", "application/json")
	minimalResponse := httptest.NewRecorder()
	router.ServeHTTP(minimalResponse, minimalSave)
	if minimalResponse.Code != http.StatusOK {
		t.Fatalf("minimal compatibility update status=%d body=%s", minimalResponse.Code, minimalResponse.Body.String())
	}
	if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, hasFutureRoleField := legacy.Value["ollamaModelRoles"]; !hasFutureRoleField || legacy.Value["ollamaModel"] != "translategemma:14b" {
		t.Fatalf("minimal update dropped future role data or model selection: %#v", legacy.Value)
	}
}

func TestSharedOllamaConflictResolutionPreservesTranslationSelectionAndRetestsReadiness(t *testing.T) {
	db := newTranslationReadinessTestDB(t)
	seedLegacyOllamaTranslation(t, db, "http://legacy.example")
	if err := db.Create(&models.AppSetting{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": "http://shared.example", "futureModelRoles": models.JSONMap{"fast": "future-model"}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateOllamaProviderSettings(db); err != nil {
		t.Fatal(err)
	}
	updated, err := NewSettingsHandler(db).updateSettings(context.Background(), []SettingInput{{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": "http://shared.example"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 {
		t.Fatalf("updated settings=%#v", updated)
	}
	var legacy models.AppSetting
	if err := db.First(&legacy, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, duplicate := legacy.Value["ollamaEndpoint"]; duplicate {
		t.Fatalf("resolved legacy endpoint remained: %#v", legacy.Value)
	}
	if legacy.Value["ollamaModel"] != "translategemma:12b" || legacy.Value["enabled"] != true || !settingIntegerEquals(legacy.Value, "maxCuesPerRequest", 23) {
		t.Fatalf("conflict resolution changed Translation preferences: %#v", legacy.Value)
	}
	if legacy.Value["readiness"] != string(translationReadinessUntested) || legacy.Value["readinessConfigIdentity"] != "" {
		t.Fatalf("stale readiness was not invalidated: %#v", legacy.Value)
	}
	var shared models.AppSetting
	if err := db.First(&shared, "key = ?", ollamaProviderSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, hasFutureRoleField := shared.Value["futureModelRoles"]; !hasFutureRoleField {
		t.Fatalf("shared provider update dropped a future role field: %#v", shared.Value)
	}
	configuration, err := loadOllamaProviderConfiguration(db)
	if err != nil || configuration.State != ollamaProviderStateConfigured || configuration.Endpoint != "http://shared.example" {
		t.Fatalf("resolved provider configuration=%#v err=%v", configuration, err)
	}
}

func seedLegacyOllamaTranslation(t *testing.T, db *gorm.DB, endpoint string) {
	t.Helper()
	settings := translationSettings{
		Provider:            translationProviderOllama,
		OllamaEndpoint:      endpoint,
		OllamaModel:         "translategemma:12b",
		MaxCuesPerRequest:   23,
		MaxRequestBytes:     32768,
		MaxContextCues:      5,
		MaxContextChars:     1536,
		Enabled:             true,
		Readiness:           translationReadinessReady,
		ReadinessCheckedAt:  "2026-10-08T10:00:00Z",
		ReadinessDiagnostic: "",
		Extra:               models.JSONMap{"ollamaModelRoles": models.JSONMap{"primary": "translator", "future": []any{"fast", "fallback"}}},
	}
	settings.ReadinessIdentity = translationProviderConfigurationIdentity(settings)
	value := translationSettingsValue(settings)
	value["ollamaEndpoint"] = endpoint
	if err := db.Create(&models.AppSetting{Key: translationSettingsKey, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
}

func persistTranslationSettingsFixture(t *testing.T, db *gorm.DB, settings translationSettings) {
	t.Helper()
	if settings.OllamaEndpoint != "" {
		shared := models.AppSetting{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": settings.OllamaEndpoint}}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).Create(&shared).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := saveTranslationSettings(db, settings); err != nil {
		t.Fatal(err)
	}
}

func settingIntegerEquals(value models.JSONMap, key string, expected int) bool {
	actual, ok := integerSettingValue(value[key])
	return ok && actual == expected
}
