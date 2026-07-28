package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCanceledJobCleanupHonorsPartialOutputPolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cancellation-cleanup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "job-42")
	output := filepath.Join(workspace, "partial.mkv")
	input := filepath.Join(workspace, "_input", "source.mkv")
	for path, content := range map[string]string{output: "partial", input: "source"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	setting := models.AppSetting{Key: "cancellationPolicy", Value: models.JSONMap{
		"deleteGeneratedFiles":           false,
		"deletePartialOutputFromStaging": true,
		"controlledRoots":                []string{root},
	}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}

	if err := cleanupCanceledJob(db, models.QueueJob{ID: 42, OutputPath: output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("partial output was not removed")
	}
	if _, err := os.Stat(input); err != nil {
		t.Fatal("workspace input should remain when only partial output cleanup is enabled")
	}
}

func TestCanceledJobCleanupNeverLeavesControlledRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cancellation-boundary?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	controlled := t.TempDir()
	outside := filepath.Join(t.TempDir(), "job-7", "partial.mkv")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "cancellationPolicy", Value: models.JSONMap{
		"deleteGeneratedFiles": true, "controlledRoots": []string{controlled},
	}}).Error; err != nil {
		t.Fatal(err)
	}

	if err := cleanupCanceledJob(db, models.QueueJob{ID: 7, OutputPath: outside}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("cleanup removed a file outside controlled roots")
	}
}
