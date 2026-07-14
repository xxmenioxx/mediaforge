package handlers

import (
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
