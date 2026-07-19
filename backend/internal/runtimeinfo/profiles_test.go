package runtimeinfo

import (
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEffectiveRuntimePolicyLayersOverridesWithoutMutatingOfficialProfile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:effective-runtime?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "runtimePolicy", Value: models.JSONMap{"schemaVersion": 2, "mode": "automatic", "preferredProfile": "workstation_balanced", "fallbackProfile": "desktop_safe", "overrides": models.JSONMap{"workstation_balanced": models.JSONMap{"maxSoftwareX265Jobs": 1, "preventSleepDuringJobs": true}}}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}

	effective, err := ResolveEffectiveRuntimePolicy(db, "desktop_safe")
	if err != nil {
		t.Fatal(err)
	}
	if effective.BaseProfile != "workstation_balanced" || effective.Values.MaxSoftwareX265Jobs != 1 || !effective.Values.PreventSleepDuringJobs {
		t.Fatalf("unexpected effective policy: %#v", effective)
	}
	official, _ := RuntimeProfile("workstation_balanced")
	if official.Values.MaxSoftwareX265Jobs != 2 || official.Values.PreventSleepDuringJobs {
		t.Fatalf("official profile was mutated: %#v", official)
	}
}

func TestAutomaticRuntimeUsesDetectedProfileWithoutPreference(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:auto-runtime?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "runtimePolicy", Value: models.JSONMap{"schemaVersion": 2, "mode": "automatic", "preferredProfile": "auto", "fallbackProfile": "desktop_safe", "overrides": models.JSONMap{}}}).Error; err != nil {
		t.Fatal(err)
	}
	effective, err := ResolveEffectiveRuntimePolicy(db, "nas_balanced")
	if err != nil {
		t.Fatal(err)
	}
	if effective.BaseProfile != "nas_balanced" {
		t.Fatalf("expected detected profile, got %#v", effective)
	}
}
