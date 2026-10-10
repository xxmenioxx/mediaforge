package database

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/validationpolicy"
)

func TestValidationPolicySettingsMigration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "validation-policy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	legacy := models.AppSetting{Key: "validation", Value: models.JSONMap{
		"minimumScore": 0, "requireDurationMatch": false, "autoAdvanceOutcomes": []string{"PASS", "PASS_WITH_WARNINGS"},
	}}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var migrated models.AppSetting
	if err := db.First(&migrated, "key = ?", "validation").Error; err != nil {
		t.Fatal(err)
	}
	if migrated.Value["preset"] != validationpolicy.Custom || migrated.Value["basedOn"] != validationpolicy.Default {
		t.Fatalf("legacy duration choice was not represented as a Custom policy: %#v", migrated.Value)
	}
	rules, _ := migrated.Value["rules"].(map[string]any)
	duration, _ := rules["duration.timeline"].(map[string]any)
	if duration["enabled"] != false {
		t.Fatalf("legacy false duration choice was not migrated: %#v", duration)
	}
	if _, exists := migrated.Value["minimumScore"]; exists {
		t.Fatal("obsolete minimumScore remains in migrated settings")
	}
	if _, exists := migrated.Value["requireDurationMatch"]; exists {
		t.Fatal("legacy requireDurationMatch remains in migrated settings")
	}
	if outcomes, ok := migrated.Value["autoAdvanceOutcomes"].([]any); !ok || len(outcomes) != 2 {
		t.Fatalf("independent automatic outcome policy was not preserved: %#v", migrated.Value["autoAdvanceOutcomes"])
	}
}

func TestNewInstallSeedsDefaultValidationPolicy(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "validation-default.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Seed(db); err != nil {
		t.Fatal(err)
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "validation").Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value["preset"] != validationpolicy.Default || setting.Value["schemaVersion"] != float64(validationpolicy.SchemaVersion) {
		t.Fatalf("factory Validation policy is not Default: %#v", setting.Value)
	}
}
