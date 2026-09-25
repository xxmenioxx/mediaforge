package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestClaimPreparationLockRegistrySerializesPerJobAndReleasesEntries(t *testing.T) {
	registry := claimPreparationLockRegistry{}
	firstKey := claimPreparationLockKey{jobID: 1}
	secondKey := claimPreparationLockKey{jobID: 2}
	unlockFirst := registry.lock(firstKey)

	sameStarted := make(chan struct{})
	sameAcquired := make(chan func(), 1)
	go func() {
		close(sameStarted)
		sameAcquired <- registry.lock(firstKey)
	}()
	<-sameStarted
	select {
	case unlock := <-sameAcquired:
		unlock()
		t.Fatal("same job preparation was not serialized")
	case <-time.After(50 * time.Millisecond):
	}

	differentAcquired := make(chan func(), 1)
	go func() {
		differentAcquired <- registry.lock(secondKey)
	}()
	select {
	case unlock := <-differentAcquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("different job preparation was unnecessarily serialized")
	}

	unlockFirst()
	select {
	case unlock := <-sameAcquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("same job preparation did not continue after release")
	}

	registry.mutex.Lock()
	remaining := len(registry.entries)
	registry.mutex.Unlock()
	if remaining != 0 {
		t.Fatalf("preparation lock entries=%d want=0", remaining)
	}
}

func workerProgressSettings(overrides models.JSONMap) models.JSONMap {
	values := models.JSONMap{
		"defaultWorkerName":             "local-worker",
		"autoWorkerEnabled":             true,
		"maxConcurrentJobs":             10,
		"maxJobsPerBatch":               0,
		"delaySecondsBetweenJobs":       0,
		"batchCooldownSeconds":          0,
		"jobStartPacingMode":            "progress",
		"startNextJobAtProgressPercent": 75,
	}
	for key, value := range overrides {
		values[key] = value
	}
	return values
}

func workerProgressTestDB(t *testing.T, settings models.JSONMap) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:worker-progress-%d?mode=memory&cache=shared&_busy_timeout=5000", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(8)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	disks := models.JSONMap{
		"workspace": models.JSONMap{"availableBytes": float64(500 << 30)},
		"library":   models.JSONMap{"availableBytes": float64(500 << 30)},
	}
	encoders := models.JSONMap{"libx265": models.JSONMap{"usable": true}}
	if err := db.Create(&models.RuntimeSnapshot{
		DetectedAt:           time.Now(),
		SelectedProfile:      "desktop_balanced",
		AvailableMemoryBytes: 16 << 30,
		Disks:                disks,
		Encoders:             encoders,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{
		SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "desktop_balanced",
		AvailableMemoryBytes: 16 << 30, Disks: disks, Encoders: encoders, StableFingerprint: "worker-test", FingerprintVersion: "v1",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "workers", Value: settings}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func createWorkerReference(t *testing.T, db *gorm.DB, status string, workerName string, progress int, startedAt time.Time) models.QueueJob {
	t.Helper()
	job := models.QueueJob{
		MediaPath:  fmt.Sprintf("/raw/reference-%d.mkv", time.Now().UnixNano()),
		LibraryID:  1,
		ProfileID:  1,
		Status:     status,
		Progress:   progress,
		WorkerName: workerName,
		StartedAt:  &startedAt,
		Priority:   1,
	}
	if terminalJobStatus(status) {
		finishedAt := startedAt.Add(time.Minute)
		job.FinishedAt = &finishedAt
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func createRunnableWorkerJob(t *testing.T, db *gorm.DB, queuePosition int64) models.QueueJob {
	t.Helper()
	job := models.QueueJob{
		MediaPath:     fmt.Sprintf("/raw/queued-%d.mkv", time.Now().UnixNano()),
		LibraryID:     1,
		ProfileID:     1,
		Status:        JobStatusQueued,
		Priority:      1,
		QueuePosition: queuePosition,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{
		JobID:           job.ID,
		Version:         1,
		Status:          scheduler.ExecutionPlanReady,
		SelectedEncoder: "libx265",
		WorkspaceMode:   scheduler.WorkspaceModeDirect,
		ApprovalStatus:  scheduler.ApprovalAutoApproved,
		Reservation:     models.JSONMap{},
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&job).Update("active_execution_plan_id", plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := scheduler.LockQueuedAsset(db, job); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestWorkerLimitsDefaultProgressPacingSettings(t *testing.T) {
	db := workerProgressTestDB(t, models.JSONMap{
		"defaultWorkerName":       "legacy-worker",
		"maxConcurrentJobs":       3,
		"maxJobsPerBatch":         10,
		"delaySecondsBetweenJobs": 30,
		"batchCooldownSeconds":    600,
	})
	limits, err := NewWorkerHandler(db).workerLimits()
	if err != nil {
		t.Fatal(err)
	}
	if limits.JobStartPacingMode != "time" || limits.StartNextJobAtProgressPercent != 75 {
		t.Fatalf("legacy worker pacing defaults = mode %q threshold %d", limits.JobStartPacingMode, limits.StartNextJobAtProgressPercent)
	}
	if limits.DelaySecondsBetweenJobs != 30 || limits.BatchCooldownSeconds != 600 {
		t.Fatalf("legacy worker pacing changed delay/batch settings: %#v", limits)
	}
}

func TestWorkerProgressPacingThresholdBoundariesUsePersistedProgress(t *testing.T) {
	cases := []struct {
		name     string
		progress int
		wantErr  error
	}{
		{name: "below", progress: 74, wantErr: errWorkerProgressPacingActive},
		{name: "exact", progress: 75},
		{name: "above", progress: 90},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := workerProgressTestDB(t, workerProgressSettings(nil))
			createWorkerReference(t, db, JobStatusRunning, "worker-a", tc.progress, time.Now().Add(-time.Hour))
			limits, err := NewWorkerHandler(db).workerLimits()
			if err != nil {
				t.Fatal(err)
			}
			gotErr := canClaimJob(db, limits, "worker-a")
			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("canClaimJob error = %v, want %v", gotErr, tc.wantErr)
			}
		})
	}
}

func TestWorkerProgressPacingFirstJobAndWorkerIsolation(t *testing.T) {
	db := workerProgressTestDB(t, workerProgressSettings(nil))
	createWorkerReference(t, db, JobStatusRunning, "worker-a", 20, time.Now().Add(-time.Hour))
	limits, err := NewWorkerHandler(db).workerLimits()
	if err != nil {
		t.Fatal(err)
	}
	if err := canClaimJob(db, limits, "worker-a"); !errors.Is(err, errWorkerProgressPacingActive) {
		t.Fatalf("worker-a pacing error = %v, want progress pacing", err)
	}
	if err := canClaimJob(db, limits, "worker-b"); err != nil {
		t.Fatalf("worker-b first claim error = %v", err)
	}
}

func TestWorkerProgressPacingUsesLatestStartedIDAndNoTerminalFallback(t *testing.T) {
	db := workerProgressTestDB(t, workerProgressSettings(nil))
	startedAt := time.Now().Add(-time.Hour)
	createWorkerReference(t, db, JobStatusRunning, "worker-a", 90, startedAt)
	createWorkerReference(t, db, JobStatusRunning, "worker-a", 20, startedAt)
	limits, err := NewWorkerHandler(db).workerLimits()
	if err != nil {
		t.Fatal(err)
	}
	if err := canClaimJob(db, limits, "worker-a"); !errors.Is(err, errWorkerProgressPacingActive) {
		t.Fatalf("latest ID reference error = %v, want progress pacing", err)
	}

	for _, status := range []string{JobStatusCompleted, JobStatusFailed, JobStatusCanceled} {
		db := workerProgressTestDB(t, workerProgressSettings(nil))
		createWorkerReference(t, db, JobStatusRunning, "worker-a", 90, time.Now().Add(-2*time.Hour))
		createWorkerReference(t, db, status, "worker-a", 20, time.Now().Add(-time.Hour))
		limits, err := NewWorkerHandler(db).workerLimits()
		if err != nil {
			t.Fatal(err)
		}
		if err := canClaimJob(db, limits, "worker-a"); err != nil {
			t.Fatalf("terminal status %s pacing error = %v, want nil", status, err)
		}
	}
}

func TestWorkerProgressPacingClaimHandoffAndCanonicalWorkerName(t *testing.T) {
	db := workerProgressTestDB(t, workerProgressSettings(models.JSONMap{
		"defaultWorkerName": " configured-worker ",
		"maxConcurrentJobs": 3,
	}))
	createWorkerReference(t, db, JobStatusRunning, "configured-worker", 80, time.Now().Add(-time.Hour))
	queued := createRunnableWorkerJob(t, db, 1)
	createRunnableWorkerJob(t, db, 2)

	claimed, err := NewWorkerHandler(db).claimNextJob("   ", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != queued.ID || claimed.WorkerName != "configured-worker" || claimed.Progress != 1 || claimed.StartedAt == nil {
		t.Fatalf("claimed job did not use canonical worker assignment: %#v", claimed)
	}
	var worker models.WorkerNode
	if err := db.First(&worker, "name = ?", "configured-worker").Error; err != nil {
		t.Fatal(err)
	}
	var reservation models.SchedulerReservation
	if err := db.First(&reservation, "job_id = ?", claimed.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.WorkerName != "configured-worker" {
		t.Fatalf("reservation worker = %q", reservation.WorkerName)
	}
	if _, err := NewWorkerHandler(db).claimNextJob("configured-worker", nil, ""); !errors.Is(err, errWorkerProgressPacingActive) {
		t.Fatalf("handoff claim error = %v, want progress pacing", err)
	}
}

func TestWorkerProgressPacingConcurrentClaimsCannotLeapfrogReference(t *testing.T) {
	db := workerProgressTestDB(t, workerProgressSettings(models.JSONMap{"maxConcurrentJobs": 3}))
	createWorkerReference(t, db, JobStatusRunning, "worker-a", 80, time.Now().Add(-time.Hour))
	createRunnableWorkerJob(t, db, 1)
	createRunnableWorkerJob(t, db, 2)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := NewWorkerHandler(db).claimNextJob("worker-a", nil, "")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	progressBlocks := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, errWorkerProgressPacingActive) {
			progressBlocks++
			continue
		}
		t.Fatalf("unexpected concurrent progress claim error: %v", err)
	}
	if successes != 1 || progressBlocks != 1 {
		t.Fatalf("concurrent claims successes=%d progressBlocks=%d, want 1/1", successes, progressBlocks)
	}
	var running int64
	if err := db.Model(&models.QueueJob{}).Where("status = ?", JobStatusRunning).Count(&running).Error; err != nil {
		t.Fatal(err)
	}
	if running != 2 {
		t.Fatalf("running jobs=%d, want A plus one newly claimed job", running)
	}
}

func TestWorkerProgressPacingPreservesHardGuards(t *testing.T) {
	tests := []struct {
		name     string
		settings models.JSONMap
		seed     func(*testing.T, *gorm.DB)
		wantErr  error
	}{
		{
			name:     "capacity",
			settings: workerProgressSettings(models.JSONMap{"maxConcurrentJobs": 1}),
			seed: func(t *testing.T, db *gorm.DB) {
				createWorkerReference(t, db, JobStatusRunning, "worker-a", 100, time.Now().Add(-time.Hour))
			},
			wantErr: errWorkerLimitReached,
		},
		{
			name: "batch cooldown",
			settings: workerProgressSettings(models.JSONMap{
				"maxJobsPerBatch":      1,
				"batchCooldownSeconds": 3600,
			}),
			seed: func(t *testing.T, db *gorm.DB) {
				createWorkerReference(t, db, JobStatusRunning, "worker-a", 100, time.Now().Add(-time.Minute))
			},
			wantErr: errWorkerBatchCooldownActive,
		},
		{
			name: "progress ignores time delay",
			settings: workerProgressSettings(models.JSONMap{
				"delaySecondsBetweenJobs": 3600,
			}),
			seed: func(t *testing.T, db *gorm.DB) {
				createWorkerReference(t, db, JobStatusRunning, "worker-a", 75, time.Now())
			},
		},
		{
			name: "time delay remains global",
			settings: workerProgressSettings(models.JSONMap{
				"jobStartPacingMode":      "time",
				"delaySecondsBetweenJobs": 3600,
			}),
			seed: func(t *testing.T, db *gorm.DB) {
				createWorkerReference(t, db, JobStatusRunning, "worker-a", 100, time.Now())
			},
			wantErr: errWorkerDelayActive,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := workerProgressTestDB(t, tc.settings)
			tc.seed(t, db)
			limits, err := NewWorkerHandler(db).workerLimits()
			if err != nil {
				t.Fatal(err)
			}
			gotErr := canClaimJob(db, limits, "worker-b")
			if !errors.Is(gotErr, tc.wantErr) {
				t.Fatalf("canClaimJob error = %v, want %v", gotErr, tc.wantErr)
			}
		})
	}
}

func TestWorkerProgressPacingDoesNotBypassSchedulerGuards(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *gorm.DB)
	}{
		{
			name: "resource policy",
			prepare: func(t *testing.T, db *gorm.DB) {
				var state models.RuntimeState
				if err := db.Where("singleton_key = ?", models.RuntimeStateSingletonKey).First(&state).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&state).Update("available_memory_bytes", 1).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "working hours",
			prepare: func(t *testing.T, db *gorm.DB) {
				if err := db.Create(&models.AppSetting{Key: "workingHours", Value: models.JSONMap{
					"enabled":             true,
					"timezone":            "America/Mexico_City",
					"windows":             models.JSONList{},
					"outsideWindowPolicy": models.JSONMap{"startNewHeavyJobs": false},
				}}).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := workerProgressTestDB(t, workerProgressSettings(nil))
			createWorkerReference(t, db, JobStatusRunning, "worker-a", 75, time.Now().Add(-time.Hour))
			createRunnableWorkerJob(t, db, 1)
			tc.prepare(t, db)
			_, err := NewWorkerHandler(db).claimNextJob("worker-a", nil, "")
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("claim error = %v, want no runnable claim after scheduler guard", err)
			}
		})
	}
}

func TestWorkerProgressPacingClearsDeferredClaimReference(t *testing.T) {
	db := workerProgressTestDB(t, workerProgressSettings(nil))
	startedAt := time.Now().Add(-time.Minute)
	job := createWorkerReference(t, db, JobStatusRunning, "worker-a", 10, startedAt)
	if err := db.Create(&models.SchedulerReservation{JobID: job.ID, AssetKey: job.MediaPath, State: scheduler.ReservationStateActive, WorkerName: "worker-a"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewWorkerHandler(db).returnPreparedJobToQueue(job.ID); err != nil {
		t.Fatal(err)
	}
	var deferredJob models.QueueJob
	if err := db.First(&deferredJob, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if deferredJob.Status != JobStatusQueued || deferredJob.WorkerName != "" || deferredJob.Progress != 0 || deferredJob.StartedAt != nil {
		t.Fatalf("deferred job retained pacing state: %#v", deferredJob)
	}
}

func TestClaimNextReturnsProgressPacingTransientResponse(t *testing.T) {
	db := workerProgressTestDB(t, workerProgressSettings(nil))
	createWorkerReference(t, db, JobStatusRunning, "local-worker", 74, time.Now().Add(-time.Hour))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/workers/claim", NewWorkerHandler(db).ClaimNext)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/workers/claim", strings.NewReader(`{"workerName":"local-worker"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "worker progress pacing") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestConcurrentClaimsRespectSingleWorkerSlot(t *testing.T) {
	dsn := fmt.Sprintf("file:claim-concurrent-%d?mode=memory&cache=shared&_busy_timeout=5000", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(4)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	disks := models.JSONMap{"workspace": models.JSONMap{"availableBytes": float64(500 << 30)}, "library": models.JSONMap{"availableBytes": float64(500 << 30)}}
	encoders := models.JSONMap{"libx265": models.JSONMap{"usable": true}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "desktop_balanced", AvailableMemoryBytes: 16 << 30, Disks: disks, Encoders: encoders}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "desktop_balanced", AvailableMemoryBytes: 16 << 30, Disks: disks, Encoders: encoders, StableFingerprint: "worker-progress-test", FingerprintVersion: "v1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "workers", Value: models.JSONMap{"defaultWorkerName": "local", "maxConcurrentJobs": 1, "maxJobsPerBatch": 0, "delaySecondsBetweenJobs": 0, "batchCooldownSeconds": 0}}).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		job := models.QueueJob{MediaPath: "/raw/job-" + string(rune('a'+index)) + ".mkv", LibraryID: 1, ProfileID: 1, Status: JobStatusQueued, Priority: 1}
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: scheduler.ExecutionPlanReady, SelectedEncoder: "libx265", WorkspaceMode: scheduler.WorkspaceModeDirect, ApprovalStatus: scheduler.ApprovalAutoApproved, Reservation: models.JSONMap{}}
		if err := db.Create(&plan).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&job).Update("active_execution_plan_id", plan.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := scheduler.LockQueuedAsset(db, job); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, claimErr := NewWorkerHandler(db).claimNextJob("local", nil, "")
			results <- claimErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for claimErr := range results {
		if claimErr == nil {
			successes++
			continue
		}
		if !errors.Is(claimErr, errWorkerLimitReached) && !errors.Is(claimErr, gorm.ErrRecordNotFound) {
			t.Fatalf("unexpected concurrent claim error: %v", claimErr)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful claim, got %d", successes)
	}
	var running int64
	if err := db.Model(&models.QueueJob{}).Where("status = ?", JobStatusRunning).Count(&running).Error; err != nil {
		t.Fatal(err)
	}
	var active int64
	if err := db.Model(&models.SchedulerReservation{}).Where("state = ?", scheduler.ReservationStateActive).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if running != 1 || active != 1 {
		t.Fatalf("single slot violated: running=%d active=%d", running, active)
	}
}

func TestClaimedJobExecutionFailureReleasesReservation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:claimed-execution-failure?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	job := models.QueueJob{
		MediaPath: "/raw/missing.mkv", LibraryID: 1, ProfileID: 1,
		Status: JobStatusRunning, Stage: JobStageClaimed, Progress: 1, StartedAt: &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	reservation := models.SchedulerReservation{
		JobID: job.ID, AssetKey: job.MediaPath, State: scheduler.ReservationStateActive,
	}
	if err := db.Create(&reservation).Error; err != nil {
		t.Fatal(err)
	}

	NewWorkerHandler(db).failClaimedJobExecution(job.ID, errors.New("input media is not readable"))

	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusFailed || job.Stage != JobStageFailed || job.Progress != 0 || job.FinishedAt == nil {
		t.Fatalf("claimed job was not closed after execution failure: %#v", job)
	}
	if err := db.First(&reservation, reservation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State == scheduler.ReservationStateActive {
		t.Fatalf("reservation remained active: %#v", reservation)
	}
}
