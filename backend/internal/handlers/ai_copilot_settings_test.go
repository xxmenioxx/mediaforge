package handlers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAICopilotSettingsRegistryNormalizesAndValidatesIndependentSelection(t *testing.T) {
	definition := settingDefinitionFor(aiCopilotSettingsKey)
	if definition.Domain != "ai-copilot" || definition.Normalize == nil {
		t.Fatalf("AI Copilot setting is missing its canonical policy: %#v", definition)
	}

	for name, value := range map[string]models.JSONMap{
		"disabled without a model":       {"enabled": false, "model": "", "preference": "balanced"},
		"disabled with a selected model": {"enabled": false, "model": " model-a ", "preference": "quality", "future": "preserved"},
		"enabled with a selected model":  {"enabled": true, "model": "model-a", "preference": "compression"},
	} {
		t.Run(name, func(t *testing.T) {
			normalized, err := definition.Normalize(settingsMutationContext{}, value)
			if err != nil {
				t.Fatal(err)
			}
			model, _ := value["model"].(string)
			if normalized["model"] != strings.TrimSpace(model) || normalized["enabled"] != value["enabled"] || normalized["preference"] != value["preference"] {
				t.Fatalf("unexpected normalized AI Copilot settings: %#v", normalized)
			}
			if extension, present := value["future"]; present && normalized["future"] != extension {
				t.Fatalf("extension field was not preserved: %#v", normalized)
			}
		})
	}

	for name, value := range map[string]models.JSONMap{
		"non-boolean enabled":      {"enabled": "true"},
		"non-string model":         {"model": 7},
		"model with newline":       {"model": "model\nother"},
		"oversized model":          {"model": strings.Repeat("a", aiCopilotModelMaxLength+1)},
		"enabled without a model":  {"enabled": true},
		"enabled with blank model": {"enabled": true, "model": " \t "},
		"invalid preference":       {"preference": "fast"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := definition.Normalize(settingsMutationContext{}, value); err == nil {
				t.Fatalf("invalid AI Copilot settings passed normalization: %#v", value)
			}
		})
	}
}

func TestAICopilotSettingsMigrationSeedsDefaultAndPreservesExistingValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ai-copilot-settings-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	translation := models.AppSetting{Key: translationSettingsKey, Value: models.JSONMap{"provider": "ollama", "ollamaModel": "translate-model", "enabled": true}}
	existingCopilot := models.AppSetting{Key: aiCopilotSettingsKey, Value: models.JSONMap{"enabled": false, "model": "retained-model", "preference": "quality"}}
	if err := db.Create(&translation).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&existingCopilot).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateAICopilotSettings(db); err != nil {
		t.Fatal(err)
	}

	var savedTranslation, savedCopilot models.AppSetting
	if err := db.First(&savedTranslation, "key = ?", translationSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&savedCopilot, "key = ?", aiCopilotSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(savedTranslation.Value, translation.Value) {
		t.Fatalf("Copilot migration changed Translation settings: got %#v, want %#v", savedTranslation.Value, translation.Value)
	}
	if !reflect.DeepEqual(savedCopilot.Value, existingCopilot.Value) {
		t.Fatalf("Copilot migration changed an existing selection: got %#v, want %#v", savedCopilot.Value, existingCopilot.Value)
	}

	if err := db.Delete(&models.AppSetting{}, "key = ?", aiCopilotSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateAICopilotSettings(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&savedCopilot, "key = ?", aiCopilotSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(savedCopilot.Value, defaultAICopilotSettingsValue()) {
		t.Fatalf("unexpected AI Copilot default: %#v", savedCopilot.Value)
	}
}
