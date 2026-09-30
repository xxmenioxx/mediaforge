package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestTestEncodeCleanupRejectsUnsafeArtifacts(t *testing.T) {
	tests := []struct {
		name      string
		path      func(root string) string
		configure func(*models.TestEncode, *models.AssetRecord)
	}{
		{
			name: "path traversal",
			path: func(root string) string { return root + "/../escape.mkv" },
		},
		{
			name: "source media",
			path: func(root string) string { return filepath.Join(root, "source.mkv") },
			configure: func(test *models.TestEncode, _ *models.AssetRecord) {
				test.SourcePath = test.OutputPath
			},
		},
		{
			name: "current Library asset",
			path: func(root string) string { return filepath.Join(root, "asset.mkv") },
			configure: func(_ *models.TestEncode, asset *models.AssetRecord) {
				asset.Status = "library"
			},
		},
		{
			name: "symlink escape",
			path: func(root string) string { return filepath.Join(root, "linked.mkv") },
		},
	}

	for index, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := testEncodeTestDB(t, "test-encode-unsafe-cleanup-"+string(rune('a'+index)))
			root := t.TempDir()
			outside := filepath.Join(t.TempDir(), "outside.mkv")
			if err := os.WriteFile(outside, []byte("protected"), 0o644); err != nil {
				t.Fatal(err)
			}
			library := models.Library{Name: "TV", DestinationPath: root}
			if err := db.Create(&library).Error; err != nil {
				t.Fatal(err)
			}
			path := testCase.path(root)
			if testCase.name == "symlink escape" {
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte("protected"), 0o644); err != nil {
				t.Fatal(err)
			}
			test := models.TestEncode{LibraryID: library.ID, SourcePath: filepath.Join(root, "source.mkv"), Status: testEncodeReady, OutputPath: path}
			asset := models.AssetRecord{Path: path, FileName: filepath.Base(path)}
			if testCase.configure != nil {
				testCase.configure(&test, &asset)
			}
			if err := db.Create(&test).Error; err != nil {
				t.Fatal(err)
			}
			if testCase.name == "current Library asset" {
				if err := db.Create(&asset).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&models.TestEncodeArtifact{
				TestEncodeID: test.ID, LibraryID: library.ID, Path: path,
				Type: models.TestEncodeArtifactMediaOutput, Expected: true, State: "owned",
			}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := (AssetHandler{db: db}).removeTestEncodeFilesWithEvidence(test); err == nil {
				t.Fatal("unsafe cleanup unexpectedly succeeded")
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("unsafe path was modified: %v", err)
			}
			if _, err := os.Stat(outside); err != nil {
				t.Fatalf("outside target was modified: %v", err)
			}
		})
	}

	t.Run("active operation", func(t *testing.T) {
		db := testEncodeTestDB(t, "test-encode-active-cleanup")
		root := t.TempDir()
		library := models.Library{Name: "TV", DestinationPath: root}
		if err := db.Create(&library).Error; err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "active.mkv")
		if err := os.WriteFile(path, []byte("active"), 0o644); err != nil {
			t.Fatal(err)
		}
		test := models.TestEncode{LibraryID: library.ID, SourcePath: filepath.Join(root, "source.mkv"), Status: testEncodeRunning, OutputPath: path}
		if err := db.Create(&test).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.TestEncodeArtifact{TestEncodeID: test.ID, LibraryID: library.ID, Path: path, Type: models.TestEncodeArtifactMediaOutput, State: "owned"}).Error; err != nil {
			t.Fatal(err)
		}
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		testEncodeProcesses.Lock()
		testEncodeProcesses.cancel[test.ID] = cancel
		testEncodeProcesses.Unlock()
		defer func() {
			testEncodeProcesses.Lock()
			delete(testEncodeProcesses.cancel, test.ID)
			testEncodeProcesses.Unlock()
		}()
		if _, err := (AssetHandler{db: db}).removeTestEncodeFilesWithEvidence(test); err == nil {
			t.Fatal("active Test Encode cleanup unexpectedly succeeded")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("active artifact was modified: %v", err)
		}
	})
}
