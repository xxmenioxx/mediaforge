package database

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacyQueuePathRow struct {
	ID            uint `gorm:"primaryKey"`
	LibraryID     uint
	MediaPath     string
	PublishedPath string
}

func (legacyQueuePathRow) TableName() string { return "queue_jobs" }

type legacyDirectPublicationPathRow struct {
	ID            uint `gorm:"primaryKey"`
	LibraryID     uint
	PublishedPath string
}

func (legacyDirectPublicationPathRow) TableName() string { return "direct_publications" }

func TestMigrateBackfillsPublicationPathKeysForHistoricalRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:publication-path-key-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacyQueuePathRow{}, &legacyDirectPublicationPathRow{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	legacyPath := root + string(filepath.Separator) + "legacy" + string(filepath.Separator) + ".." + string(filepath.Separator) + "episode.mkv"
	if err := db.Create(&legacyQueuePathRow{LibraryID: 4, MediaPath: legacyPath, PublishedPath: legacyPath}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyDirectPublicationPathRow{LibraryID: 4, PublishedPath: legacyPath}).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE queue_jobs ADD COLUMN media_path_key TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE queue_jobs ADD COLUMN published_path_key TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE direct_publications ADD COLUMN published_path_key TEXT NOT NULL DEFAULT ''",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := backfillPublicationPathKeys(db); err != nil {
		t.Fatal(err)
	}
	want := models.CanonicalPublicationPathKey(legacyPath)
	var queue struct {
		MediaPathKey     string
		PublishedPathKey string
	}
	if err := db.Table("queue_jobs").First(&queue).Error; err != nil {
		t.Fatal(err)
	}
	if queue.PublishedPathKey != want {
		t.Fatalf("QueueJob path key=%q, want %q", queue.PublishedPathKey, want)
	}
	if queue.MediaPathKey != want {
		t.Fatalf("QueueJob source path key=%q, want %q", queue.MediaPathKey, want)
	}
	var direct struct {
		PublishedPathKey string
	}
	if err := db.Table("direct_publications").First(&direct).Error; err != nil {
		t.Fatal(err)
	}
	if direct.PublishedPathKey != want {
		t.Fatalf("DirectPublication path key=%q, want %q", direct.PublishedPathKey, want)
	}
	if err := backfillPublicationPathKeys(db); err != nil {
		t.Fatalf("idempotent migration rerun: %v", err)
	}
}
