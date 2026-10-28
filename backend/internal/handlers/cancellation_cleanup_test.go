package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func canceledCleanupFixture(t *testing.T, deleteGenerated, deletePartial bool) (*gorm.DB, housekeepingTestRoots, models.QueueJob, string, string) {
	t.Helper()
	db, roots := newHousekeepingTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	setting := models.AppSetting{Key: "cancellationPolicy", Value: models.JSONMap{
		"deleteGeneratedFiles":           deleteGenerated,
		"deletePartialOutputFromStaging": deletePartial,
		"controlledRoots":                []string{roots.work},
	}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := models.QueueJob{
		ID:         42,
		MediaPath:  filepath.Join(roots.raw, "source.mkv"),
		OutputPath: filepath.Join(roots.work, "job-42", "partial.mkv"),
		Status:     JobStatusCanceled, Stage: JobStageCanceled, FinishedAt: &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Dir(job.OutputPath)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(job.OutputPath, []byte("synthetic partial output"), 0o644); err != nil {
		t.Fatal(err)
	}
	return db, roots, job, workspace, job.OutputPath
}

func persistCleanupIntentForTest(t *testing.T, db *gorm.DB, jobID uint) models.QueueJob {
	t.Helper()
	var job models.QueueJob
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&job, jobID).Error; err != nil {
			return err
		}
		return persistCanceledJobCleanupIntent(tx, &job)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, jobID).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func TestQueuedCancellationPersistsIntentBeforeAcknowledgingWithoutDeletingFiles(t *testing.T) {
	db, _, job, _, output := canceledCleanupFixture(t, false, true)
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": JobStatusQueued, "stage": JobStageQueued, "finished_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/cancel", func(c *gin.Context) { NewQueueHandler(db).requestCancellation(c, job.ID) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/cancel", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != JobStatusCanceled || stored.CleanupStatus != canceledCleanupStatusPending || stored.CleanupOperationID == "" {
		t.Fatalf("cancellation acknowledgment preceded durable cleanup intent: %#v", stored)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("HTTP cancellation performed filesystem cleanup: %v", err)
	}
}

func TestRunningCancellationPersistsCleanupOnlyAfterWorkerAcknowledgment(t *testing.T) {
	db, _, job, _, output := canceledCleanupFixture(t, false, true)
	started := time.Now().Add(-time.Minute).UTC()
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status": JobStatusRunning, "stage": JobStageConverting, "started_at": started, "finished_at": nil,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SchedulerReservation{JobID: job.ID, AssetKey: job.MediaPath, State: scheduler.ReservationStateActive}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/cancel", func(c *gin.Context) { NewQueueHandler(db).requestCancellation(c, job.ID) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/cancel", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}
	var pending models.QueueJob
	if err := db.First(&pending, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != JobStatusRunning || pending.CleanupOperationID != "" || pending.CleanupStatus != "" {
		t.Fatalf("cleanup was authorized before worker exit acknowledgment: %#v", pending)
	}
	var reservations int64
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Count(&reservations).Error; err != nil || reservations != 1 {
		t.Fatalf("reservation was released before acknowledgment: count=%d err=%v", reservations, err)
	}

	finalized, ok, err := finalizeQueueJobCancellationAfterProcessTreeExit(db, job.ID)
	if err != nil || !ok {
		t.Fatalf("worker acknowledgment finalized=%t err=%v", ok, err)
	}
	if finalized.Status != JobStatusCanceled || finalized.CleanupStatus != canceledCleanupStatusPending || finalized.CleanupOperationID == "" || finalized.ProcessTreeExitVerifiedAt == nil {
		t.Fatalf("terminal acknowledgment did not atomically persist cleanup intent: %#v", finalized)
	}
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Count(&reservations).Error; err != nil || reservations != 0 {
		t.Fatalf("acknowledged cancellation retained reservation: count=%d err=%v", reservations, err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("worker acknowledgment performed filesystem cleanup in its transaction: %v", err)
	}
	if processed, err := processPendingCanceledJobCleanups(db, 1); err != nil || processed != 1 {
		t.Fatalf("Housekeeping processed=%d err=%v", processed, err)
	}
	if err := db.First(&finalized, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finalized.CleanupStatus != canceledCleanupStatusCompleted {
		t.Fatalf("verified process-tree exit did not permit Housekeeping cleanup: %#v", finalized)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("Housekeeping did not remove the authorized output after verified exit: %v", err)
	}
}

func TestCanceledJobCleanupIntentIsDurableAndHousekeepingReconcilesAfterRestart(t *testing.T) {
	db, roots, job, workspace, output := canceledCleanupFixture(t, false, true)
	source := filepath.Join(roots.raw, "source.mkv")
	if err := os.WriteFile(source, []byte("immutable synthetic source"), 0o644); err != nil {
		t.Fatal(err)
	}
	stored := persistCleanupIntentForTest(t, db, job.ID)
	if stored.CleanupStatus != canceledCleanupStatusPending || stored.CleanupOperationID == "" || stored.CleanupTargetPath != output {
		t.Fatalf("cleanup intent was not durably persisted: %#v", stored)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("intent persistence performed filesystem cleanup before Housekeeping claimed it: %v", err)
	}

	// A new Housekeeping cycle discovers the persisted pending operation.
	processed, err := processPendingCanceledJobCleanups(db, 1)
	if err != nil || processed != 1 {
		t.Fatalf("Housekeeping recovery processed=%d err=%v", processed, err)
	}
	var finished models.QueueJob
	if err := db.First(&finished, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finished.CleanupStatus != canceledCleanupStatusCompleted || finished.CleanupAttemptCount != 1 {
		t.Fatalf("cleanup completion was not persisted: %#v", finished)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("authorized partial output remains: %v", err)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("partial-output policy removed the containing workspace: %v", err)
	}
	if contents, err := os.ReadFile(source); err != nil || string(contents) != "immutable synthetic source" {
		t.Fatalf("source outside the work role changed: contents=%q err=%v", contents, err)
	}
}

func TestCanceledJobCleanupGeneratedWorkspaceUsesHousekeepingAndRejectsSymlinks(t *testing.T) {
	db, roots, job, workspace, _ := canceledCleanupFixture(t, true, false)
	source := filepath.Join(roots.raw, "source.mkv")
	if err := os.WriteFile(source, []byte("source stays"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(workspace, "linked-source.mkv")
	if err := os.Symlink(source, linked); err != nil {
		t.Fatal(err)
	}
	persistCleanupIntentForTest(t, db, job.ID)
	if _, err := processPendingCanceledJobCleanups(db, 1); err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CleanupStatus != canceledCleanupStatusFailed || !strings.Contains(stored.CleanupError, "unsafe or unreadable") {
		t.Fatalf("symlink workspace was not fail-closed: %#v", stored)
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatalf("symlink entry was unexpectedly removed: %v", err)
	}
	if contents, err := os.ReadFile(source); err != nil || string(contents) != "source stays" {
		t.Fatalf("symlink target changed: contents=%q err=%v", contents, err)
	}
}

func TestCanceledJobCleanupRejectsOutsideWorkRoleEvenWhenPolicyRootAllowsIt(t *testing.T) {
	db, roots, job, _, output := canceledCleanupFixture(t, false, true)
	outside := filepath.Join(t.TempDir(), "outside.mkv")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Update("output_path", outside).Error; err != nil {
		t.Fatal(err)
	}
	var policy models.AppSetting
	if err := db.Where("key = ?", "cancellationPolicy").First(&policy).Error; err != nil {
		t.Fatal(err)
	}
	policy.Value = models.JSONMap{"deleteGeneratedFiles": false, "deletePartialOutputFromStaging": true, "controlledRoots": []string{roots.work, filepath.Dir(outside)}}
	if err := db.Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	stored := persistCleanupIntentForTest(t, db, job.ID)
	if stored.CleanupStatus != canceledCleanupStatusPending {
		t.Fatalf("configured allow-list should persist intent for runtime Housekeeping validation: %#v", stored)
	}
	if _, err := processPendingCanceledJobCleanups(db, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was removed despite work-role boundary: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("unrelated in-work fixture changed: %v", err)
	}
	var finished models.QueueJob
	if err := db.First(&finished, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finished.CleanupStatus != canceledCleanupStatusFailed {
		t.Fatalf("outside work-role target should remain failed and visible: %#v", finished)
	}
}

func TestCanceledJobCleanupDuplicateIntentSchedulingIsIdempotent(t *testing.T) {
	db, _, job, _, _ := canceledCleanupFixture(t, false, true)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- db.Transaction(func(tx *gorm.DB) error {
				var current models.QueueJob
				if err := tx.First(&current, job.ID).Error; err != nil {
					return err
				}
				return persistCanceledJobCleanupIntent(tx, &current)
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("duplicate intent scheduling failed: %v", err)
		}
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CleanupOperationID != fmt.Sprintf("queue-canceled-job-%d", job.ID) || stored.CleanupAttemptCount != 0 || stored.CleanupStatus != canceledCleanupStatusPending {
		t.Fatalf("duplicate scheduling created a second operation or attempt: %#v", stored)
	}
}

func TestCanceledJobCleanupExpiredLeaseFencesStaleCallbacksAndRecovers(t *testing.T) {
	db, _, job, _, _ := canceledCleanupFixture(t, false, true)
	persistCleanupIntentForTest(t, db, job.ID)
	startedAt := time.Now().UTC()
	first, claimed, err := claimNextCanceledJobCleanup(db, startedAt, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("first claim=%t err=%v", claimed, err)
	}
	expiredAt := startedAt.Add(2 * time.Minute)
	if err := assertCanceledJobCleanupLease(db, first, expiredAt); !errors.Is(err, errCanceledCleanupLeaseLost) {
		t.Fatalf("expired lease remained authorized: %v", err)
	}
	second, claimed, err := claimNextCanceledJobCleanup(db, expiredAt, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("recovery claim=%t err=%v", claimed, err)
	}
	if second.Job.CleanupOperationID != first.Job.CleanupOperationID || second.LeaseToken == first.LeaseToken || second.Job.CleanupAttemptCount != 2 {
		t.Fatalf("recovery did not retain operation identity and fence the old attempt: first=%#v second=%#v", first, second)
	}
	if err := finishCanceledJobCleanup(db, first, expiredAt, nil); !errors.Is(err, errCanceledCleanupLeaseLost) {
		t.Fatalf("stale completion callback changed the new attempt: %v", err)
	}
	if err := finishCanceledJobCleanup(db, second, expiredAt.Add(time.Second), nil); err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CleanupStatus != canceledCleanupStatusCompleted || stored.CleanupLeaseToken != "" {
		t.Fatalf("recovered operation did not complete cleanly: %#v", stored)
	}
}

func TestCanceledJobCleanupMissingLeaseMetadataFailsClosed(t *testing.T) {
	db, _, job, _, output := canceledCleanupFixture(t, false, true)
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"cleanup_operation_id":     fmt.Sprintf("queue-canceled-job-%d", job.ID),
		"cleanup_target_path":      output,
		"cleanup_owner_revision":   canceledCleanupOwnerRevision(job),
		"cleanup_status":           canceledCleanupStatusRunning,
		"cleanup_lease_token":      "",
		"cleanup_lease_expires_at": nil,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := claimNextCanceledJobCleanup(db, time.Now().UTC(), time.Minute); err != nil || claimed {
		t.Fatalf("incomplete lease unexpectedly reclaimed: claimed=%t err=%v", claimed, err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CleanupStatus != canceledCleanupStatusFailed || !strings.Contains(stored.CleanupError, "lease metadata is incomplete") {
		t.Fatalf("incomplete lease was not made visible as fail-closed: %#v", stored)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("incomplete lease removed its target: %v", err)
	}
}

func TestCanceledJobCleanupFailureRetriesSameDurableOperation(t *testing.T) {
	db, _, job, _, _ := canceledCleanupFixture(t, false, true)
	persistCleanupIntentForTest(t, db, job.ID)
	startedAt := time.Now().UTC()
	first, claimed, err := claimNextCanceledJobCleanup(db, startedAt, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("first claim=%t err=%v", claimed, err)
	}
	if err := finishCanceledJobCleanup(db, first, startedAt.Add(time.Second), errors.New(strings.Repeat("x", 1500))); err != nil {
		t.Fatal(err)
	}
	var failed models.QueueJob
	if err := db.First(&failed, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if failed.CleanupStatus != canceledCleanupStatusFailed || len([]rune(failed.CleanupError)) != 1000 || failed.CleanupNextAttemptAt == nil {
		t.Fatalf("bounded failure and retry schedule missing: %#v", failed)
	}
	if _, claimed, err := claimNextCanceledJobCleanup(db, startedAt.Add(30*time.Second), time.Minute); err != nil || claimed {
		t.Fatalf("retry ignored its backoff: claimed=%t err=%v", claimed, err)
	}
	second, claimed, err := claimNextCanceledJobCleanup(db, startedAt.Add(2*time.Minute), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("retry claim=%t err=%v", claimed, err)
	}
	if second.Job.CleanupOperationID != first.Job.CleanupOperationID || second.Job.CleanupAttemptCount != 2 || second.LeaseToken == first.LeaseToken {
		t.Fatalf("retry changed durable operation identity or reused lease: first=%#v second=%#v", first, second)
	}
	if err := finishCanceledJobCleanup(db, second, startedAt.Add(2*time.Minute+time.Second), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCanceledJobCleanupDefersWhileActiveAndAllowsDismissedRecovery(t *testing.T) {
	db, _, activeJob, _, activeOutput := canceledCleanupFixture(t, false, true)
	started := time.Now().Add(-time.Minute).UTC()
	if err := db.Model(&models.QueueJob{}).Where("id = ?", activeJob.ID).Updates(map[string]any{"started_at": started, "finished_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	persistCleanupIntentForTest(t, db, activeJob.ID)
	if _, err := processPendingCanceledJobCleanups(db, 1); err != nil {
		t.Fatal(err)
	}
	var deferred models.QueueJob
	if err := db.First(&deferred, activeJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if deferred.CleanupStatus != canceledCleanupStatusPending || deferred.CleanupNextAttemptAt == nil {
		t.Fatalf("active worker ownership did not defer cleanup: %#v", deferred)
	}
	if _, err := os.Stat(activeOutput); err != nil {
		t.Fatalf("active workspace was modified: %v", err)
	}

	db2, _, dismissedJob, _, dismissedOutput := canceledCleanupFixture(t, false, true)
	dismissedAt := time.Now().UTC()
	if err := db2.Model(&models.QueueJob{}).Where("id = ?", dismissedJob.ID).Update("dismissed_at", dismissedAt).Error; err != nil {
		t.Fatal(err)
	}
	persistCleanupIntentForTest(t, db2, dismissedJob.ID)
	if _, err := processPendingCanceledJobCleanups(db2, 1); err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db2.First(&stored, dismissedJob.ID).Error; err != nil {
		t.Fatalf("dismissed Queue history was deleted during cleanup: %v", err)
	}
	if stored.DismissedAt == nil || stored.CleanupStatus != canceledCleanupStatusCompleted {
		t.Fatalf("dismissed cleanup recovery state is incorrect: %#v", stored)
	}
	if _, err := os.Stat(dismissedOutput); !os.IsNotExist(err) {
		t.Fatalf("dismissed job cleanup did not reconcile output: %v", err)
	}
}

func TestCanceledJobCleanupRequiresVerifiedProcessTreeExitForStartedJobs(t *testing.T) {
	db, _, job, _, output := canceledCleanupFixture(t, false, true)
	started := time.Now().Add(-time.Minute).UTC()
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Update("started_at", started).Error; err != nil {
		t.Fatal(err)
	}
	persistCleanupIntentForTest(t, db, job.ID)
	if _, err := processPendingCanceledJobCleanups(db, 1); err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CleanupStatus != canceledCleanupStatusFailed || !strings.Contains(stored.CleanupError, "verified process-tree exit") {
		t.Fatalf("started job without process-tree evidence was not blocked: %#v", stored)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("unverified started job output was removed: %v", err)
	}
}

func TestCanceledJobCleanupReconcilesLegacyRunningStateAfterRestart(t *testing.T) {
	db, _, job, _, output := canceledCleanupFixture(t, false, true)
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{"cleanup_status": canceledCleanupStatusRunning, "cleanup_error": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := processPendingCanceledJobCleanups(db, 1); err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CleanupOperationID == "" || stored.CleanupStatus != canceledCleanupStatusCompleted {
		t.Fatalf("legacy process-local state was not adopted and recovered: %#v", stored)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("recovered operation left output behind: %v", err)
	}
}
