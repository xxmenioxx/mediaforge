package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
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

func TestConcurrentOriginalArchivalCreatesOnlyOneArchive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:concurrent-original-archive?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	archiveRoot := filepath.Join(root, "archive")
	original := filepath.Join(rawRoot, "anime", "Baccano", "episode.mkv")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": rawRoot, "originalsArchivePath": archiveRoot}}).Error; err != nil {
		t.Fatal(err)
	}

	handler := PublisherHandler{db: db}
	job := models.QueueJob{MediaPath: original}
	results := make(chan string, 2)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			archived, archiveErr := handler.archivePublishedOriginal(job)
			results <- archived
			errors <- archiveErr
		}()
	}
	workers.Wait()
	close(results)
	close(errors)
	for archiveErr := range errors {
		if archiveErr != nil {
			t.Fatalf("concurrent archive failed: %v", archiveErr)
		}
	}
	nonEmptyResults := 0
	for archived := range results {
		if archived != "" {
			nonEmptyResults++
		}
	}
	if nonEmptyResults != 1 {
		t.Fatalf("archive operations returning a path=%d want=1", nonEmptyResults)
	}
	entries, err := os.ReadDir(filepath.Join(archiveRoot, "anime", "Baccano"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive entries=%d want=1", len(entries))
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
	originalSidecar := strings.TrimSuffix(original, filepath.Ext(original)) + ".spa.srt"
	if err := os.WriteFile(originalSidecar, []byte("library subtitle"), 0o600); err != nil {
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
	if got, err := os.ReadFile(originalSidecar); err != nil || string(got) != "library subtitle" {
		t.Fatalf("library sidecar was moved or removed: content=%q err=%v", got, err)
	}
}

func TestCopyExternalSubtitleSidecarsToPublishedAsset(t *testing.T) {
	root := t.TempDir()
	rawMedia := filepath.Join(root, "raw", "Movies", "Original Name.mkv")
	libraryMedia := filepath.Join(root, "library", "Movies", "Published Name.mkv")
	for _, mediaPath := range []string{rawMedia, libraryMedia} {
		if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mediaPath, []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rawBase := strings.TrimSuffix(rawMedia, filepath.Ext(rawMedia))
	srt := rawBase + ".spa.default.srt"
	ass := rawBase + ".eng.2.ass"
	if err := os.WriteFile(srt, []byte("spanish"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ass, []byte("english"), 0o600); err != nil {
		t.Fatal(err)
	}

	copied, err := copyExternalSubtitleSidecars(rawMedia, libraryMedia)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 2 {
		t.Fatalf("copied sidecars=%#v", copied)
	}
	libraryBase := strings.TrimSuffix(libraryMedia, filepath.Ext(libraryMedia))
	for path, want := range map[string]string{
		libraryBase + ".spa.default.srt": "spanish",
		libraryBase + ".eng.2.ass":       "english",
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("published sidecar %q content=%q err=%v", path, got, readErr)
		}
	}
	for path, want := range map[string]string{srt: "spanish", ass: "english"} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("raw sidecar %q was moved or removed: content=%q err=%v", path, got, readErr)
		}
	}

	if err := os.WriteFile(libraryBase+".spa.default.srt", []byte("edited in library"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyExternalSubtitleSidecars(rawMedia, libraryMedia); err != nil {
		t.Fatalf("different Library sidecar must not block publication: %v", err)
	}
	if got, err := os.ReadFile(libraryBase + ".spa.default.srt"); err != nil || string(got) != "edited in library" {
		t.Fatalf("existing Library sidecar was overwritten: content=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(libraryBase + ".mvf.spa.default.srt"); err != nil || string(got) != "spanish" {
		t.Fatalf("incoming sidecar was not preserved with MVF name: content=%q err=%v", got, err)
	}

	if err := os.WriteFile(libraryBase+".mvf.spa.default.srt", []byte("another version"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyExternalSubtitleSidecars(rawMedia, libraryMedia); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(libraryBase + ".mvf-2.spa.default.srt"); err != nil || string(got) != "spanish" {
		t.Fatalf("second collision was not preserved: content=%q err=%v", got, err)
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
