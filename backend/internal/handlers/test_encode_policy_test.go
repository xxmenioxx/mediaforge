package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestTestEncodePolicySettingsRegistryValidatesAndNormalizes(t *testing.T) {
	definition := settingDefinitionFor(testEncodePolicySettingKey)
	if definition.Domain != "test-encode" || definition.Validate == nil || definition.Normalize == nil {
		t.Fatalf("Test Encode policy is not registered with domain validation and normalization: %#v", definition)
	}
	input := models.JSONMap{"defaultStartMode": " MIDDLE ", "retentionHours": 36}
	if err := definition.Validate(settingsMutationContext{}, input); err != nil {
		t.Fatal(err)
	}
	normalized, err := definition.Normalize(settingsMutationContext{}, input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized["defaultDurationSeconds"] != 20 || normalized["defaultStartMode"] != "middle" ||
		normalized["defaultRetention"] != "temporary" || normalized["temporaryRetentionHours"] != 36 ||
		normalized["failedRetentionHours"] != 6 {
		t.Fatalf("unexpected normalized Test Encode policy: %#v", normalized)
	}
}

func TestResolveTestEncodeWindowUsesDomainPolicyDefaults(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-policy-window", &models.AppSetting{})
	path := "/raw/policy-defaults.mkv"
	if err := db.Create(&models.ScanResult{Path: path, Duration: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: testEncodePolicySettingKey, Value: models.JSONMap{
		"defaultDurationSeconds":  40,
		"defaultStartMode":        "middle",
		"defaultRetention":        "keep",
		"temporaryRetentionHours": 72,
		"failedRetentionHours":    8,
		"maxReconcileBatchSize":   80,
		"maxCleanupBatchSize":     15,
	}}).Error; err != nil {
		t.Fatal(err)
	}
	start, duration := resolveTestEncodeWindow(db, path, "", 0, 0)
	if start != 30 || duration != 40 {
		t.Fatalf("policy window defaults got start=%v duration=%d, want start=30 duration=40", start, duration)
	}
	policy := testEncodePolicyFromSettings(db)
	if policy.DefaultRetention != "keep" || policy.TemporaryRetentionHours != 72 || policy.FailedRetentionHours != 8 {
		t.Fatalf("unexpected loaded lifecycle defaults: %#v", policy)
	}
}

func TestTestEncodePolicyRejectsInvalidLifecycleValues(t *testing.T) {
	for _, value := range []models.JSONMap{
		{"defaultDurationSeconds": 121},
		{"defaultStartMode": "custom"},
		{"defaultRetention": "forever"},
		{"temporaryRetentionHours": 0},
		{"failedRetentionHours": 8761},
		{"maxReconcileBatchSize": 251},
		{"maxCleanupBatchSize": 101},
	} {
		if err := validateTestEncodePolicyValue(value); err == nil {
			t.Errorf("expected policy validation failure for %#v", value)
		}
	}
}
