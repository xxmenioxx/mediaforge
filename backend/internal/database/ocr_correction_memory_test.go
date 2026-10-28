package database

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateAddsOCRCorrectionMemoryTablesWithoutBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ocr-correction-memory-existing-database?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.OCRDictionaryEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "existing-setting", Value: models.JSONMap{"preserve": true}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OCRDictionaryEntry{ID: "existing-rule", Language: "eng", Pattern: "cant", Replacement: "can't"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate existing database: %v", err)
	}
	if !db.Migrator().HasTable(&models.OCRCorrectionMemoryEntry{}) || !db.Migrator().HasTable(&models.OCRCorrectionMemoryEvidence{}) {
		t.Fatal("OCR correction memory tables were not created")
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "existing-setting").Error; err != nil || setting.Value["preserve"] != true {
		t.Fatalf("existing setting changed during migration: %#v err=%v", setting, err)
	}
	var rule models.OCRDictionaryEntry
	if err := db.First(&rule, "id = ?", "existing-rule").Error; err != nil || rule.Replacement != "can't" {
		t.Fatalf("existing OCR dictionary rule changed during migration: %#v err=%v", rule, err)
	}
	var entries int64
	if err := db.Model(&models.OCRCorrectionMemoryEntry{}).Count(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if entries != 0 {
		t.Fatalf("migration synthesized %d learned entries", entries)
	}
}
