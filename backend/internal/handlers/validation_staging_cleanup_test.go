package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type validationStagingFixture struct {
	validation queueValidationDismissalFixture
	workspace  string
}

func newValidationStagingFixture(t *testing.T) validationStagingFixture {
	t.Helper()
	fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
	db := fixture.delivery.db
	if err := db.AutoMigrate(
		&models.SchedulerReservation{}, &models.AssetMaintenanceOperation{}, &models.TestEncode{}, &models.TestEncodeArtifact{},
		&models.ValidationStagingCleanupPreview{}, &models.ValidationStagingCleanupEvidence{},
	); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Dir(fixture.delivery.output)
	sidecar := strings.TrimSuffix(fixture.delivery.output, filepath.Ext(fixture.delivery.output)) + ".eng.ass"
	if err := os.WriteFile(sidecar, []byte("previewed staged sidecar"), 0o600); err != nil {
		t.Fatal(err)
	}
	return validationStagingFixture{validation: fixture, workspace: workspace}
}

func dismissValidationStagingFixture(t *testing.T, fixture validationStagingFixture) {
	t.Helper()
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.DELETE("/api/queue/jobs/:id", NewQueueHandler(fixture.validation.delivery.db).Dismiss)
	request := httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(fixture.validation.delivery.job.ID)), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Dismiss status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidationStagingCleanupDismissThenCleansOnlyPreviewedWorkspace(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	db := fixture.validation.delivery.db
	preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.State != "ready" || preview.PreviewID == "" || len(preview.Files) != 2 || preview.TotalBytes == 0 {
		t.Fatalf("preview did not bind the exact workspace files: %#v", preview)
	}
	dismissValidationStagingFixture(t, fixture)
	result, err := executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "cleaned" || result.RecoveredBytes != preview.TotalBytes || len(result.RemovedPaths) < 3 {
		t.Fatalf("cleanup result=%#v preview=%#v", result, preview)
	}
	if _, err := os.Stat(fixture.workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace remains after cleanup, stat err=%v", err)
	}
	if _, err := os.Stat(fixture.validation.delivery.original); err != nil {
		t.Fatalf("RAW original was changed by cleanup: %v", err)
	}
	if _, err := os.Stat(fixture.validation.resultPath); err != nil {
		t.Fatalf("Result report was changed by cleanup: %v", err)
	}
	var job models.QueueJob
	if err := db.First(&job, fixture.validation.delivery.job.ID).Error; err != nil || job.DismissedAt == nil || job.ValidationRunID != fixture.validation.run.RunID {
		t.Fatalf("Dismiss/Validation history changed: job=%#v err=%v", job, err)
	}
	var evidence []models.ValidationStagingCleanupEvidence
	if err := db.Where("job_id = ?", job.ID).Order("id asc").Find(&evidence).Error; err != nil || len(evidence) != 2 || evidence[0].Event != "started" || evidence[1].Outcome != "cleaned" {
		t.Fatalf("cleanup evidence is incomplete: %#v err=%v", evidence, err)
	}
	repeated, err := executeValidationStagingCleanup(db, job.ID, preview.PreviewID, "local_operator")
	if err != nil || repeated.Outcome != "cleaned" || repeated.RecoveredBytes != result.RecoveredBytes {
		t.Fatalf("repeated confirmation was not idempotent: result=%#v err=%v", repeated, err)
	}
	var policy HousekeepingPolicy
	candidate, eligible, err := housekeepingCandidate(db, job.ID, fixture.workspace, time.Now(), policy, time.Now())
	if err != nil || eligible || candidate.SafeToDelete {
		t.Fatalf("global Housekeeping protection changed for dismissed job: %#v eligible=%v err=%v", candidate, eligible, err)
	}
}

func TestValidationStagingCleanupBlocksChangedManifestAndActiveReferences(t *testing.T) {
	for _, mode := range []string{"changed_manifest", "active_reservation", "other_job_sidecar"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newValidationStagingFixture(t)
			db := fixture.validation.delivery.db
			preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
			if err != nil || preview.State != "ready" {
				t.Fatalf("preview=%#v err=%v", preview, err)
			}
			dismissValidationStagingFixture(t, fixture)
			if mode == "changed_manifest" {
				if err := os.WriteFile(filepath.Join(fixture.workspace, "new-file.mkv"), []byte("newly staged"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if mode == "active_reservation" {
				if err := db.Create(&models.SchedulerReservation{JobID: fixture.validation.delivery.job.ID, AssetKey: fixture.validation.delivery.job.MediaPath, State: "active", WorkerName: "worker"}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				other := models.QueueJob{MediaPath: "/raw/other.mkv", Status: JobStatusCompleted, SubtitleArtifacts: models.JSONList{filepath.Join(fixture.workspace, "output.eng.ass")}}
				if err := db.Create(&other).Error; err != nil {
					t.Fatal(err)
				}
			}
			result, err := executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
			if err != nil || result.Outcome != "blocked" {
				t.Fatalf("unsafe cleanup was not blocked: result=%#v err=%v", result, err)
			}
			if _, err := os.Stat(fixture.validation.delivery.output); err != nil {
				t.Fatalf("workspace content was deleted after block: %v", err)
			}
			var job models.QueueJob
			if err := db.First(&job, fixture.validation.delivery.job.ID).Error; err != nil || job.DismissedAt == nil {
				t.Fatalf("cleanup block rolled back Dismiss: job=%#v err=%v", job, err)
			}
		})
	}
}

func TestValidationStagingCleanupNoopForAbsentWorkspace(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	if err := os.Remove(fixture.validation.delivery.output); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(strings.TrimSuffix(fixture.validation.delivery.output, filepath.Ext(fixture.validation.delivery.output)) + ".eng.ass"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.workspace); err != nil {
		t.Fatal(err)
	}
	preview, err := createValidationStagingPreview(fixture.validation.delivery.db, fixture.validation.delivery.job.ID)
	if err != nil || preview.State != "noop" || preview.TotalBytes != 0 || preview.PreviewID == "" {
		t.Fatalf("absent workspace preview=%#v err=%v", preview, err)
	}
	dismissValidationStagingFixture(t, fixture)
	result, err := executeValidationStagingCleanup(fixture.validation.delivery.db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
	if err != nil || result.Outcome != "noop" || result.RecoveredBytes != 0 {
		t.Fatalf("absent workspace cleanup=%#v err=%v", result, err)
	}
}

func TestValidationStagingCleanupRejectsSymlinkWorkspaceEntry(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	outside := filepath.Join(fixture.validation.delivery.root, "outside.txt")
	if err := os.WriteFile(outside, []byte("must remain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.workspace, "escape.txt")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	preview, err := createValidationStagingPreview(fixture.validation.delivery.db, fixture.validation.delivery.job.ID)
	if err != nil || preview.State != "blocked" || len(preview.Blockers) == 0 || preview.PreviewID != "" {
		t.Fatalf("unsafe symlink preview=%#v err=%v", preview, err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside target changed: %v", err)
	}
}

func TestValidationStagingCleanupPartialRemovalReportsBytesAndFreshRetry(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	files, _, total, err := validationStagingManifest(fixture.workspace)
	if err != nil || len(files) != 3 {
		t.Fatalf("manifest=%#v total=%d err=%v", files, total, err)
	}
	ordered := append([]validationStagingFile(nil), files...)
	sort.Slice(ordered, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(ordered[i].Relative), pathDepth(ordered[j].Relative)
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind == "file"
		}
		return ordered[i].Relative > ordered[j].Relative
	})
	if ordered[0].Kind != "file" || ordered[1].Kind != "file" {
		t.Fatalf("expected two previewed files before the workspace root: %#v", ordered)
	}
	partialManifest := append([]validationStagingFile(nil), files...)
	for index := range partialManifest {
		if partialManifest[index].Relative == ordered[1].Relative {
			partialManifest[index].Identity = "changed-after-preview"
		}
	}
	removed, recovered, removeErr := removeValidationStagingManifest(filepath.Dir(fixture.workspace), fixture.workspace, partialManifest)
	if removeErr == nil || len(removed) != 1 || recovered != ordered[0].SizeBytes {
		t.Fatalf("partial removal was not accurately reported: removed=%#v recovered=%d err=%v", removed, recovered, removeErr)
	}
	remaining, _, remainingBytes, err := validationStagingManifest(fixture.workspace)
	if err != nil || len(remaining) != 2 || remainingBytes != total-recovered {
		t.Fatalf("remaining manifest=%#v bytes=%d total=%d err=%v", remaining, remainingBytes, total, err)
	}
	removed, retriedBytes, removeErr := removeValidationStagingManifest(filepath.Dir(fixture.workspace), fixture.workspace, remaining)
	if removeErr != nil || retriedBytes != remainingBytes || len(removed) != 2 {
		t.Fatalf("fresh retry did not remove only the remaining workspace: removed=%#v bytes=%d err=%v", removed, retriedBytes, removeErr)
	}
}

func TestValidationStagingCleanupConcurrentConfirmationConsumesPreviewOnce(t *testing.T) {
	fixture := newValidationStagingFixture(t)
	db := fixture.validation.delivery.db
	preview, err := createValidationStagingPreview(db, fixture.validation.delivery.job.ID)
	if err != nil || preview.PreviewID == "" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	dismissValidationStagingFixture(t, fixture)
	results := make([]validationStagingCleanupResponse, 2)
	errorsSeen := make([]error, 2)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errorsSeen[index] = executeValidationStagingCleanup(db, fixture.validation.delivery.job.ID, preview.PreviewID, "local_operator")
		}(index)
	}
	group.Wait()
	for index := range results {
		if errorsSeen[index] != nil || results[index].Outcome != "cleaned" || results[index].RecoveredBytes != preview.TotalBytes {
			t.Fatalf("confirmation %d was not idempotently fenced: result=%#v err=%v", index, results[index], errorsSeen[index])
		}
	}
	var events int64
	if err := db.Model(&models.ValidationStagingCleanupEvidence{}).Where("job_id = ? AND event = ?", fixture.validation.delivery.job.ID, "completed").Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("preview was consumed more than once: events=%d err=%v", events, err)
	}
}

func TestValidationStagingCleanupResponseContractSerializes(t *testing.T) {
	encoded, err := json.Marshal(validationStagingCleanupResponse{Outcome: "blocked", RemovedPaths: []string{}, RecoveredBytes: 0})
	if err != nil || !strings.Contains(string(encoded), `"outcome":"blocked"`) || !strings.Contains(string(encoded), `"recoveredBytes":0`) {
		t.Fatalf("cleanup result response contract=%s err=%v", encoded, err)
	}
}
