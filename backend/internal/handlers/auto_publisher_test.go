package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAutoPublisherDoesNotRearchiveRetiredPublication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:auto-publisher-retired?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rawRoot, archiveRoot := filepath.Join(root, "raw"), filepath.Join(root, "archive")
	originalPath := filepath.Join(rawRoot, "movies", "movie.mkv")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": rawRoot, "originalsArchivePath": archiveRoot}}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	job := models.QueueJob{MediaPath: originalPath, LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: filepath.Join(root, "library", "movie.mkv"), PublishedAt: &now, PublicationRetiredAt: &now}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	(&AutoPublisher{db: db}).reconcilePublishedJobs()
	if _, err := os.Stat(originalPath); err != nil {
		t.Fatalf("retired publication rearchived the Raw original: %v", err)
	}
	entries, err := os.ReadDir(archiveRoot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unexpected archive entries: %v", entries)
	}
}
