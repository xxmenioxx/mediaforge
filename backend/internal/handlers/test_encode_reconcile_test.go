package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestReconcileTestEncodeUsesPersistedOwnershipAndSeparateStatus(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		outputExists  bool
		sidecarExists bool
		keep          bool
		want          string
	}{
		{name: "existing output is healthy", outputExists: true, sidecarExists: true, want: "healthy"},
		{name: "kept output removed by user", keep: true, want: "missing_output"},
		{name: "media missing with owned sidecar remaining", sidecarExists: true, want: "partial"},
		{name: "media present with expected sidecar missing", outputExists: true, want: "partial"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			db := testEncodeTestDB(t, "reconcile-"+testCase.name)
			library := models.Library{Name: "Test", DestinationPath: root}
			if err := db.Create(&library).Error; err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(root, "sample-test.mkv")
			if testCase.outputExists {
				if err := os.WriteFile(output, []byte("test media"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sidecar := filepath.Join(root, "sample-test.en.srt")
			if testCase.sidecarExists {
				if err := os.WriteFile(sidecar, []byte("subtitle"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			testEncode := models.TestEncode{
				SourcePath: filepath.Join(root, "source.mkv"), LibraryID: library.ID,
				ConfigurationSource: "global", ConfigurationHash: "test", Status: testEncodeReady,
				Phase: "ready", OutputPath: output, Keep: testCase.keep,
			}
			if err := db.Create(&testEncode).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.TestEncodeArtifact{
				TestEncodeID: testEncode.ID, LibraryID: library.ID, Path: output,
				Type: models.TestEncodeArtifactMediaOutput, Expected: true,
				State: "owned", CreatedAt: time.Now(),
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.TestEncodeArtifact{
				TestEncodeID: testEncode.ID, LibraryID: library.ID, Path: sidecar,
				Type: models.TestEncodeArtifactSubtitle, Expected: true,
				State: "owned", CreatedAt: time.Now(),
			}).Error; err != nil {
				t.Fatal(err)
			}
			result, err := (AssetHandler{db: db}).reconcileTestEncode(testEncode)
			if err != nil {
				t.Fatal(err)
			}
			if result.ReconciliationStatus != testCase.want {
				t.Fatalf("reconciliation=%q want=%q result=%#v", result.ReconciliationStatus, testCase.want, result)
			}
			if result.OutputExists != testCase.outputExists {
				t.Fatalf("OutputExists=%v want=%v", result.OutputExists, testCase.outputExists)
			}
			var persisted models.TestEncode
			if err := db.First(&persisted, testEncode.ID).Error; err != nil {
				t.Fatal(err)
			}
			if persisted.Status != testEncodeReady || persisted.Keep != testEncode.Keep || persisted.LastReconciledAt == nil {
				t.Fatalf("filesystem reconciliation changed execution or retention state: %#v", persisted)
			}
		})
	}
}

func TestReconcileMarksInterruptedRunningRowStaleButSkipsActiveOperation(t *testing.T) {
	db := testEncodeTestDB(t, "reconcile-interrupted-test-encode")
	root := t.TempDir()
	library := models.Library{Name: "Test", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	for _, active := range []bool{false, true} {
		testEncode := models.TestEncode{
			SourcePath: "/raw/source.mkv", LibraryID: library.ID, ConfigurationSource: "global",
			ConfigurationHash: "test", Status: testEncodeRunning, Phase: "encoding", CreatedAt: time.Now().Add(-time.Hour),
		}
		if err := db.Create(&testEncode).Error; err != nil {
			t.Fatal(err)
		}
		if active {
			testEncodeProcesses.Lock()
			testEncodeProcesses.cancel[testEncode.ID] = context.CancelFunc(func() {})
			testEncodeProcesses.Unlock()
			t.Cleanup(func() {
				testEncodeProcesses.Lock()
				delete(testEncodeProcesses.cancel, testEncode.ID)
				testEncodeProcesses.Unlock()
			})
		}
		result, err := (AssetHandler{db: db}).reconcileTestEncode(testEncode)
		if err != nil {
			t.Fatal(err)
		}
		if active && !result.SkippedActive {
			t.Fatal("active Test Encode operation was reconciled")
		}
		if !active && result.ReconciliationStatus != "stale" {
			t.Fatalf("interrupted execution reconciliation=%q, want stale", result.ReconciliationStatus)
		}
		var persisted models.TestEncode
		if err := db.First(&persisted, testEncode.ID).Error; err != nil {
			t.Fatal(err)
		}
		if persisted.Status != testEncodeRunning {
			t.Fatalf("reconciliation changed execution state to %q", persisted.Status)
		}
	}
}
