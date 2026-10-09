package database

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSeedSettingsAddsResourceDefaultsWithoutOverwritingOperatorOrUnrelatedValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ai-resource-settings-seed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "unrelatedSetting", Value: models.JSONMap{"preserved": true}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedSettings(db); err != nil {
		t.Fatal(err)
	}
	var resource models.AppSetting
	if err := db.First(&resource, "key = ?", airesource.SettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	settings, err := airesource.DecodeSettings(resource.Value)
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxResidentModels != 1 || settings.MaxConcurrentInferences != 1 || settings.MaxConcurrentModelLoads != 1 || settings.MinimumAvailableMemoryBytes != 6*(1<<30) {
		t.Fatalf("unexpected initial resource protection settings: %+v", settings)
	}
	updated := airesource.DefaultSettingsValue()
	updated["maxResidentModels"] = 2
	if err := db.Model(&models.AppSetting{}).Where("key = ?", airesource.SettingsKey).Update("value", updated).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedSettings(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, "key = ?", airesource.SettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	settings, err = airesource.DecodeSettings(resource.Value)
	if err != nil {
		t.Fatal(err)
	}
	if settings.MaxResidentModels != 2 {
		t.Fatalf("seeding overwrote operator policy: %+v", settings)
	}
	var unrelated models.AppSetting
	if err := db.First(&unrelated, "key = ?", "unrelatedSetting").Error; err != nil {
		t.Fatal(err)
	}
	if unrelated.Value["preserved"] != true {
		t.Fatalf("seeding changed unrelated settings: %#v", unrelated.Value)
	}
}
