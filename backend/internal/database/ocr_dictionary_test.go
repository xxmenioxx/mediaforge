package database

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestOCRDictionaryMigrationSeedsIdempotentlyAndPersistsEntries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ocr-dictionary.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var seeded int64
	if err := db.Model(&models.OCRDictionaryEntry{}).Where("source = ?", "builtin").Count(&seeded).Error; err != nil {
		t.Fatal(err)
	}
	if seeded != 7 {
		t.Fatalf("expected seven English built-ins, got %d", seeded)
	}
	if err := db.Model(&models.OCRDictionaryEntry{}).Where("id = ?", "builtin:eng:im").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	custom := models.OCRDictionaryEntry{
		ID: "user:test-rule", Language: "eng", Pattern: "| hello", Replacement: "I hello",
		MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Enabled: true,
		Source: "user", RuleVersion: 1,
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := restarted.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := Migrate(restarted); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Model(&models.OCRDictionaryEntry{}).Where("source = ?", "builtin").Count(&seeded).Error; err != nil {
		t.Fatal(err)
	}
	if seeded != 7 {
		t.Fatalf("expected idempotent seeding to retain seven built-ins, got %d", seeded)
	}
	var builtin models.OCRDictionaryEntry
	if err := restarted.First(&builtin, "id = ?", "builtin:eng:im").Error; err != nil {
		t.Fatal(err)
	}
	if builtin.Enabled {
		t.Fatal("migration reseeding unexpectedly re-enabled a disabled built-in")
	}
	var persisted models.OCRDictionaryEntry
	if err := restarted.First(&persisted, "id = ?", custom.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Pattern != custom.Pattern || persisted.Replacement != custom.Replacement || persisted.RuleVersion != custom.RuleVersion {
		t.Fatalf("custom rule did not persist across restart: %#v", persisted)
	}
}
