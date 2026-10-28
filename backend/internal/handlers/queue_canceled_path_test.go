package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCanceledPathPreviewAndDismissAreExactBoundedAndIdempotent(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	started := time.Now().Add(-time.Minute)
	executionNumber := uint(4)
	jobs := []models.QueueJob{
		{BatchID: "first", MediaPath: "/raw/group/one.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled, StartedAt: &started, ExecutionNumber: &executionNumber, Notes: "retained history"},
		{BatchID: "second", MediaPath: "/raw/group/two.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled},
		{BatchID: "mixed", MediaPath: "/raw/group/three.mkv", Status: JobStatusQueued, Stage: JobStageQueued},
		{BatchID: "prefix", MediaPath: "/raw/group-extra/four.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled},
		{BatchID: "child", MediaPath: "/raw/group/nested/five.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled},
		{BatchID: "validation", MediaPath: "/raw/group/six.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled, ValidationRunID: "validation-1"},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
		if index == 1 {
			if err := db.Create(&models.ExecutionPlan{JobID: jobs[index].ID, Version: 1, Status: scheduler.ExecutionPlanPendingEvaluation}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewQueueHandler(db)
	router.POST("/api/queue/canceled-path/preview", handler.PreviewCanceledPath)
	router.POST("/api/queue/canceled-path/dismiss", handler.DismissCanceledPath)

	previewResponse := httptest.NewRecorder()
	router.ServeHTTP(previewResponse, queuePathRequest(t, http.MethodPost, "/api/queue/canceled-path/preview", map[string]any{"pathGroup": "/raw/group"}))
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var preview queueCanceledPathPreview
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.PathGroup != "/raw/group" || preview.DismissibleJobs != 2 || preview.SkippedJobs != 2 || preview.AlreadyDismissedJobs != 0 || preview.Truncated {
		t.Fatalf("unexpected exact-path preview: %#v", preview)
	}
	if len(preview.Jobs) != 4 {
		t.Fatalf("prefix or descendant path crossed exact group boundary: got %d rows", len(preview.Jobs))
	}

	ids := []uint{jobs[0].ID, jobs[1].ID, jobs[2].ID, jobs[3].ID, jobs[0].ID}
	dismissResponse := httptest.NewRecorder()
	router.ServeHTTP(dismissResponse, queuePathRequest(t, http.MethodPost, "/api/queue/canceled-path/dismiss", map[string]any{"pathGroup": "/raw/group", "jobIds": ids}))
	if dismissResponse.Code != http.StatusOK {
		t.Fatalf("dismiss status=%d body=%s", dismissResponse.Code, dismissResponse.Body.String())
	}
	var dismissed queueCanceledPathDismissResult
	if err := json.Unmarshal(dismissResponse.Body.Bytes(), &dismissed); err != nil {
		t.Fatal(err)
	}
	if dismissed.SelectedJobs != 4 || dismissed.DismissedJobs != 2 || dismissed.SkippedJobs != 1 || dismissed.ConflictedJobs != 1 || dismissed.CleanedExecutionPlans != 1 {
		t.Fatalf("unexpected path dismissal counts: %#v", dismissed)
	}

	var retained models.QueueJob
	if err := db.First(&retained, jobs[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if retained.DismissedAt == nil || retained.ExecutionNumber == nil || retained.StartedAt == nil || retained.Notes != "retained history" {
		t.Fatalf("executed job provenance was not retained: %#v", retained)
	}
	var placeholder models.QueueJob
	if err := db.First(&placeholder, jobs[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if placeholder.DismissedAt == nil || placeholder.ExecutionNumber != nil || placeholder.StartedAt != nil {
		t.Fatalf("never-started placeholder dismissal did not preserve idempotent history: %#v", placeholder)
	}
	var placeholderPlans int64
	if err := db.Model(&models.ExecutionPlan{}).Where("job_id = ?", jobs[1].ID).Count(&placeholderPlans).Error; err != nil {
		t.Fatal(err)
	}
	if placeholderPlans != 0 {
		t.Fatalf("never-started placeholder execution plan was retained: %d", placeholderPlans)
	}
	for _, index := range []int{2, 3, 4, 5} {
		var stored models.QueueJob
		if err := db.First(&stored, jobs[index].ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.DismissedAt != nil {
			t.Fatalf("ineligible job %d was dismissed: %#v", stored.ID, stored)
		}
	}

	// Repeating the request cannot dismiss any additional records. Both rows are
	// counted as already dismissed, including the never-executed placeholder.
	retryResponse := httptest.NewRecorder()
	router.ServeHTTP(retryResponse, queuePathRequest(t, http.MethodPost, "/api/queue/canceled-path/dismiss", map[string]any{"pathGroup": "/raw/group", "jobIds": []uint{jobs[0].ID, jobs[1].ID}}))
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retryResponse.Code, retryResponse.Body.String())
	}
	var retried queueCanceledPathDismissResult
	if err := json.Unmarshal(retryResponse.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	if retried.DismissedJobs != 0 || retried.AlreadyDismissedJobs != 2 || retried.ConflictedJobs != 0 {
		t.Fatalf("retry was not idempotent: %#v", retried)
	}
}

func TestCanceledPathPreviewKeepsSymlinkAndTargetGroupsDistinct(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	root := t.TempDir()
	targetDir := filepath.Join(root, "target")
	aliasDir := filepath.Join(root, "alias")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "source-fixture.txt"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDir, aliasDir); err != nil {
		t.Skipf("symlink creation unavailable in this environment: %v", err)
	}
	jobs := []models.QueueJob{
		{MediaPath: filepath.Join(targetDir, "source-fixture.txt"), Status: JobStatusCanceled, Stage: JobStageCanceled},
		{MediaPath: filepath.Join(aliasDir, "source-fixture.txt"), Status: JobStatusCanceled, Stage: JobStageCanceled},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/canceled-path/preview", NewQueueHandler(db).PreviewCanceledPath)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, queuePathRequest(t, http.MethodPost, "/api/queue/canceled-path/preview", map[string]any{"pathGroup": aliasDir}))
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	var preview queueCanceledPathPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.DismissibleJobs != 1 || len(preview.Jobs) != 1 || preview.Jobs[0].ID != jobs[1].ID {
		t.Fatalf("symlink path group resolved into the target group: %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "source-fixture.txt")); err != nil {
		t.Fatalf("preview changed the source fixture: %v", err)
	}
}

func TestCanceledPathPreviewMatchesQueueRootGroup(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	paths := []string{"/top-level.mkv", "relative.mkv", "Root/from-named-root-folder.mkv", "Root/nested/deeper.mkv", "/Root/absolute-folder.mkv"}
	for _, mediaPath := range paths {
		job := models.QueueJob{MediaPath: mediaPath, Status: JobStatusCanceled, Stage: JobStageCanceled}
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/canceled-path/preview", NewQueueHandler(db).PreviewCanceledPath)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, queuePathRequest(t, http.MethodPost, "/api/queue/canceled-path/preview", map[string]any{"pathGroup": "Root"}))
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	var preview queueCanceledPathPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.DismissibleJobs != 3 || len(preview.Jobs) != 3 {
		t.Fatalf("root preview diverged from Queue path grouping: %#v", preview)
	}
}

func TestCanceledPathPreviewMatchesWindowsSeparators(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	job := models.QueueJob{MediaPath: `C:\RAW\Series\Episode.mkv`, Status: JobStatusCanceled, Stage: JobStageCanceled}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/canceled-path/preview", NewQueueHandler(db).PreviewCanceledPath)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, queuePathRequest(t, http.MethodPost, "/api/queue/canceled-path/preview", map[string]any{"pathGroup": `C:\RAW\Series`}))
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	var preview queueCanceledPathPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.PathGroup != "C:/RAW/Series" || preview.DismissibleJobs != 1 || len(preview.Jobs) != 1 || preview.Jobs[0].ID != job.ID {
		t.Fatalf("Windows path separators were not normalized consistently: %#v", preview)
	}
}

func TestQueuePathGroupKeepsQueueDisplayBoundaries(t *testing.T) {
	for _, test := range []struct {
		mediaPath string
		want      string
	}{
		{mediaPath: "/raw/Show/Episode.mkv", want: "/raw/Show"},
		{mediaPath: "/raw//Episode.mkv", want: "/raw/"},
		{mediaPath: `C:\RAW\Series\Episode.mkv`, want: "C:/RAW/Series"},
		{mediaPath: "relative.mkv", want: "Root"},
	} {
		if got := queueGroupFromMediaPath(test.mediaPath); got != test.want {
			t.Errorf("queueGroupFromMediaPath(%q)=%q want %q", test.mediaPath, got, test.want)
		}
		canonical, err := canonicalQueuePathGroup(test.want)
		if err != nil || canonical != test.want {
			t.Errorf("canonicalQueuePathGroup(%q)=%q err=%v", test.want, canonical, err)
		}
	}
}

func TestCanceledPathPreviewPrioritizesActiveRowsAndReportsTruncation(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	now := time.Now().UTC()
	jobs := []models.QueueJob{
		{MediaPath: "/raw/cap/old-one.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled, DismissedAt: &now},
		{MediaPath: "/raw/cap/old-two.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled, DismissedAt: &now},
		{MediaPath: "/raw/cap/current-one.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled},
		{MediaPath: "/raw/cap/current-two.mkv", Status: JobStatusCanceled, Stage: JobStageCanceled},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows, truncated, err := loadQueuePathGroupJobs(db, "/raw/cap", 2, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(rows) != 2 || rows[0].ID != jobs[2].ID || rows[1].ID != jobs[3].ID {
		t.Fatalf("bounded preview did not prioritize current jobs or report omissions: rows=%#v truncated=%t", rows, truncated)
	}
}

func TestQueueCancellationWaitsForRunningOperationAcknowledgment(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	started := time.Now().Add(-time.Minute)
	job := models.QueueJob{MediaPath: "/raw/cancel-running.mkv", Status: JobStatusRunning, Stage: JobStageConverting, StartedAt: &started}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := scheduler.LockQueuedAsset(db, job); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/jobs/:id", NewQueueHandler(db).Update)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, queuePathRequest(t, http.MethodPost, fmt.Sprintf("/api/queue/jobs/%d", job.ID), map[string]any{"status": JobStatusCanceled}))
	if response.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}
	var pending models.QueueJob
	if err := json.Unmarshal(response.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Status != JobStatusRunning || pending.CancellationRequestedAt == nil || pending.FinishedAt != nil {
		t.Fatalf("running cancellation was reported terminal before acknowledgment: %#v", pending)
	}
	var reservations int64
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 1 {
		t.Fatalf("cancellation released active reservation before acknowledgment: %d", reservations)
	}
	requeue := httptest.NewRecorder()
	router.ServeHTTP(requeue, queuePathRequest(t, http.MethodPost, fmt.Sprintf("/api/queue/jobs/%d", job.ID), map[string]any{"status": JobStatusQueued}))
	if requeue.Code != http.StatusConflict {
		t.Fatalf("requeue during pending cancellation status=%d body=%s", requeue.Code, requeue.Body.String())
	}
	if err := db.First(&pending, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != JobStatusRunning || pending.CancellationRequestedAt == nil {
		t.Fatalf("requeue changed running cancellation state: %#v", pending)
	}

	finalized, canceled, err := finalizeQueueJobCancellation(db, job.ID)
	if err != nil || !canceled {
		t.Fatalf("finalize canceled=%v err=%v", canceled, err)
	}
	if finalized.Status != JobStatusCanceled || finalized.FinishedAt == nil {
		t.Fatalf("worker acknowledgment did not terminalize cancellation: %#v", finalized)
	}
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("acknowledged cancellation retained reservation: %d", reservations)
	}

	workerRouter := gin.New()
	workerRouter.POST("/api/workers/jobs/:id/status", NewWorkerHandler(db).UpdateJobStatus)
	lateCallback := httptest.NewRecorder()
	workerRouter.ServeHTTP(lateCallback, queuePathRequest(t, http.MethodPost, fmt.Sprintf("/api/workers/jobs/%d/status", job.ID), map[string]any{"status": JobStatusCompleted, "progress": 100}))
	if lateCallback.Code != http.StatusOK {
		t.Fatalf("late callback status=%d body=%s", lateCallback.Code, lateCallback.Body.String())
	}
	if err := db.First(&finalized, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finalized.Status != JobStatusCanceled {
		t.Fatalf("late worker callback resurrected canceled job: %#v", finalized)
	}
}

func TestQueuedCancellationImmediatelyReleasesReservationAndDoesNotCreatePendingMarker(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	job := models.QueueJob{MediaPath: "/raw/cancel-queued.mkv", Status: JobStatusQueued, Stage: JobStageQueued}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := scheduler.LockQueuedAsset(db, job); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/jobs/:id", NewQueueHandler(db).Update)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, queuePathRequest(t, http.MethodPost, fmt.Sprintf("/api/queue/jobs/%d", job.ID), map[string]any{"status": JobStatusCanceled}))
	if response.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}
	var canceled models.QueueJob
	if err := db.First(&canceled, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if canceled.Status != JobStatusCanceled || canceled.FinishedAt == nil || canceled.CancellationRequestedAt != nil {
		t.Fatalf("never-started job did not become terminal immediately: %#v", canceled)
	}
	var reservations int64
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("queued cancellation retained reservation: %d", reservations)
	}
}

func TestPreparedCancellationDoesNotRequeueClaimedJob(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	now := time.Now().UTC()
	job := models.QueueJob{MediaPath: "/raw/prepared-cancel.mkv", Status: JobStatusRunning, Stage: JobStagePreparingWorkspace, StartedAt: &now, CancellationRequestedAt: &now}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := scheduler.LockQueuedAsset(db, job); err != nil {
		t.Fatal(err)
	}
	if err := NewWorkerHandler(db).returnPreparedJobToQueue(job.ID); err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != JobStatusCanceled {
		t.Fatalf("canceled claimed job was requeued: %#v", stored)
	}
}

func TestAssetLifecycleBecomesAvailableOnlyAfterCancellationAcknowledgment(t *testing.T) {
	path := "/media/raw/anime/cancel-lifecycle.mkv"
	record := lifecycleTestRawRecord(801, path)
	now := time.Now().UTC()
	job := models.QueueJob{
		ID: 802, MediaPath: path, Status: JobStatusRunning, Stage: JobStageConverting,
		StartedAt: &now, CancellationRequestedAt: &now,
	}
	whileStopping := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{Record: record, Records: []models.AssetRecord{record}, QueueJobs: []models.QueueJob{job}})
	if whileStopping.Lifecycle == AssetLifecycleAvailable || whileStopping.ActiveOperation == nil || whileStopping.ActiveOperation.Kind != "queue" {
		t.Fatalf("asset became available before process acknowledgment: %#v", whileStopping)
	}

	job.Status = JobStatusCanceled
	job.Stage = JobStageCanceled
	job.FinishedAt = &now
	afterAcknowledgment := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{Record: record, Records: []models.AssetRecord{record}, QueueJobs: []models.QueueJob{job}})
	if afterAcknowledgment.Lifecycle != AssetLifecycleAvailable || afterAcknowledgment.ActiveOperation != nil {
		t.Fatalf("eligible canceled source did not project Available: %#v", afterAcknowledgment)
	}
}

func TestCanceledJobCleanupErrorIsBounded(t *testing.T) {
	message := strings.Repeat("é", 1500)
	bounded := boundedCanceledCleanupError(fmt.Errorf("%s", message))
	if got := len([]rune(bounded)); got != 1000 {
		t.Fatalf("cleanup error rune count=%d want=1000", got)
	}
}

func TestBatchDismissReturnsWhileCanceledJobCleanupIsBlocked(t *testing.T) {
	db := queueCanceledPathTestDB(t)
	startedAt := time.Now().Add(-time.Minute)
	executionNumber := uint(22)
	job := models.QueueJob{
		BatchID: "slow-cleanup", MediaPath: "/raw/slow-cleanup.mkv", Status: JobStatusCanceled,
		Stage: JobStageCanceled, StartedAt: &startedAt, ExecutionNumber: &executionNumber,
		CleanupOperationID: "queue-canceled-job-slow-cleanup", CleanupStatus: canceledCleanupStatusPending,
		CleanupTargetPath: "/tmp/work/job-slow-cleanup/output.mkv", CleanupOwnerRevision: "synthetic-revision",
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		_, _ = processPendingCanceledJobCleanupsWith(db, 1, func(_ *gorm.DB, _ canceledCleanupClaim) error {
			close(started)
			<-release
			close(finished)
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("synthetic cleanup did not start")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/queue/batches/:batchId", NewQueueHandler(db).DismissBatch)
	response := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() {
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/batches/slow-cleanup", nil))
		close(returned)
	}()
	select {
	case <-returned:
		if response.Code != http.StatusOK {
			close(release)
			t.Fatalf("batch dismissal status=%d body=%s", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("batch dismissal waited for the blocked cleanup operation")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("synthetic cleanup did not finish after release")
	}
}

func BenchmarkCanceledPathDismiss(b *testing.B) {
	for _, jobCount := range []int{1, 25, 100} {
		b.Run(fmt.Sprintf("jobs_%d", jobCount), func(b *testing.B) {
			db, err := gorm.Open(sqlite.Open("file:"+fmt.Sprintf("queue-canceled-path-bench-%d", jobCount)+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				b.Fatal(err)
			}
			if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.SchedulerReservation{}, &models.AppSetting{}); err != nil {
				b.Fatal(err)
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/api/queue/canceled-path/dismiss", NewQueueHandler(db).DismissCanceledPath)
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.QueueJob{}).Error; err != nil {
					b.Fatal(err)
				}
				started := time.Now().UTC()
				ids := make([]uint, 0, jobCount)
				for index := 0; index < jobCount; index++ {
					executionNumber := uint(index + 1)
					job := models.QueueJob{
						MediaPath: fmt.Sprintf("/raw/bench/job-%d-%d.mkv", iteration, index),
						Status:    JobStatusCanceled, Stage: JobStageCanceled,
						StartedAt: &started, ExecutionNumber: &executionNumber,
					}
					if err := db.Create(&job).Error; err != nil {
						b.Fatal(err)
					}
					ids = append(ids, job.ID)
				}
				request := queueBenchmarkPathRequest(b, "/raw/bench", ids)
				recorder := httptest.NewRecorder()
				b.StartTimer()
				router.ServeHTTP(recorder, request)
				b.StopTimer()
				if recorder.Code != http.StatusOK {
					b.Fatalf("dismiss status=%d body=%s", recorder.Code, recorder.Body.String())
				}
			}
		})
	}
}

func queueCanceledPathTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.SchedulerReservation{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func queuePathRequest(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(method, url, strings.NewReader(string(data)))
}

func queueBenchmarkPathRequest(b *testing.B, pathGroup string, ids []uint) *http.Request {
	b.Helper()
	data, err := json.Marshal(map[string]any{"pathGroup": pathGroup, "jobIds": ids})
	if err != nil {
		b.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPost, "/api/queue/canceled-path/dismiss", strings.NewReader(string(data)))
}
