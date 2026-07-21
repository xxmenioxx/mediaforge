package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFilesEqualUsesContentNotOnlySize(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.mkv")
	right := filepath.Join(dir, "right.mkv")
	if err := os.WriteFile(left, []byte("same-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("same-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	equal, err := filesEqual(left, right)
	if err != nil || !equal {
		t.Fatalf("expected identical files: equal=%v err=%v", equal, err)
	}
	if err := os.WriteFile(right, []byte("other-value!"), 0o600); err != nil {
		t.Fatal(err)
	}
	equal, err = filesEqual(left, right)
	if err != nil || equal {
		t.Fatalf("expected different files: equal=%v err=%v", equal, err)
	}
}

func TestAutomatedPipelineDoesNotRequireAutoAnalysis(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pipeline-independent-stages?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{
		"autoAnalysisEnabled": false, "autoValidationEnabled": false, "autoPublisherEnabled": true,
	}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	result := NewWorkerHandler(db).runAutomatedPipeline(models.QueueJob{})
	if result["analysisStatus"] != "skipped" || result["stoppedAt"] != "validation" {
		t.Fatalf("unexpected independent pipeline result: %#v", result)
	}
}

func TestOriginalsArchivePathMapsLegacyContainerPathToConfiguredHostRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:archive-path-mapping?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	hostRoot := filepath.Join(t.TempDir(), "originals")
	settings := []models.AppSetting{
		{Key: "paths", Value: models.JSONMap{"originalsArchivePath": hostRoot}},
		{Key: "originalRetentionPolicy", Value: models.JSONMap{"processedOriginalsPath": "/media/originals_archive/processed-originals"}},
	}
	for _, setting := range settings {
		if err := db.Create(&setting).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := originalsArchivePath(db)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(hostRoot, "processed-originals")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanupStagedJobRemovesWholeJobDirectory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:staged-job-cleanup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"stagingPath": root}}).Error; err != nil {
		t.Fatal(err)
	}
	jobRoot := filepath.Join(root, "job-7")
	output := filepath.Join(jobRoot, "title", "output.mkv")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobRoot, ".DS_Store"), []byte("metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (PublisherHandler{db: db}).cleanupStagedJob(models.QueueJob{ID: 7, OutputPath: output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(jobRoot); !os.IsNotExist(err) {
		t.Fatalf("job directory still exists: %v", err)
	}
}

func TestPublishLibraryReplacementArchivesOriginalAndKeepsPath(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:library-replacement?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.QueueJob{}, &models.AppSetting{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	libraryRoot := filepath.Join(root, "library")
	archiveRoot := filepath.Join(root, "archive")
	stagingRoot := filepath.Join(root, "staging")
	original := filepath.Join(libraryRoot, "Anime", "Movie", "movie.mkv")
	output := filepath.Join(stagingRoot, "job-1", "movie.mkv")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("converted"), 0o600); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Anime", SourcePath: libraryRoot, DestinationPath: libraryRoot, Type: "movies"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"stagingPath": stagingRoot, "originalsArchivePath": archiveRoot}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: original, PublishMode: PublishModeReplaceLibrary, LibraryID: library.ID, ProfileID: 1, Status: JobStatusCompleted, Stage: JobStageValidating, OutputPath: output, ValidationStatus: ValidationStatusPassed}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	result, err := (PublisherHandler{db: db}).publishLibraryReplacement(job, library, models.Profile{Container: "mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if result.PublishedPath != original {
		t.Fatalf("published path=%q want=%q", result.PublishedPath, original)
	}
	got, err := os.ReadFile(original)
	if err != nil || string(got) != "converted" {
		t.Fatalf("replacement content=%q err=%v", got, err)
	}
	archived := filepath.Join(archiveRoot, "library-replacements", fmt.Sprintf("library-%d", library.ID), "Anime", "Movie", "movie.mkv")
	got, err = os.ReadFile(archived)
	if err != nil || string(got) != "original" {
		t.Fatalf("archive content=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Dir(output)); !os.IsNotExist(err) {
		t.Fatalf("workspace was not cleaned: %v", err)
	}
}

func TestPathIsInsideRejectsSiblingPrefix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	if !pathIsInside(filepath.Join(root, "movie.mkv"), root) {
		t.Fatal("expected library child to be accepted")
	}
	if pathIsInside(filepath.Join(root+"-other", "movie.mkv"), root) {
		t.Fatal("sibling prefix must not be accepted")
	}
}
