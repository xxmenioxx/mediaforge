package handlers

import (
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestOCRAISettingsRegistryNormalizesAndValidatesIndependentSelection(t *testing.T) {
	definition := settingDefinitionFor(ocrAISettingsKey)
	if definition.Domain != "pipeline" || definition.Normalize == nil {
		t.Fatalf("OCR AI setting is missing its canonical policy: %#v", definition)
	}

	normalized, err := definition.Normalize(settingsMutationContext{}, models.JSONMap{
		"enabled": false,
		"model":   "  qwen2.5:7b  ",
		"future":  "preserved",
	})
	if err != nil {
		t.Fatal(err)
	}
	if normalized["enabled"] != false || normalized["model"] != "qwen2.5:7b" || normalized["future"] != "preserved" {
		t.Fatalf("unexpected OCR AI setting normalization: %#v", normalized)
	}

	for name, value := range map[string]models.JSONMap{
		"non-boolean enabled": {"enabled": "true"},
		"non-string model":    {"model": 7},
		"model with newline":  {"model": "model\nother"},
		"oversized model":     {"model": strings.Repeat("a", ocrAIModelMaxLength+1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := definition.Normalize(settingsMutationContext{}, value); err == nil {
				t.Fatalf("invalid OCR AI setting passed normalization: %#v", value)
			}
		})
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
