package handlers

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func TestTerminalStageForStatus(t *testing.T) {
	cases := map[string]string{JobStatusQueued: JobStageQueued, JobStatusRunning: JobStageConverting, JobStatusCompleted: JobStageValidationReview, JobStatusFailed: JobStageFailed, JobStatusCanceled: JobStageCanceled}
	for status, expected := range cases {
		if actual := terminalStageForStatus(status); actual != expected {
			t.Fatalf("status %s: expected %s, got %s", status, expected, actual)
		}
	}
}

func TestStageForStatusUpdateDoesNotTreatConversionAsPublication(t *testing.T) {
	publishedAt := time.Now()
	tests := []struct {
		name           string
		job            models.QueueJob
		previousStatus string
		want           string
	}{
		{name: "new conversion completion waits for validation", job: models.QueueJob{Status: JobStatusCompleted, Stage: JobStageConverting}, previousStatus: JobStatusRunning, want: JobStageValidationReview},
		{name: "repeated completion preserves current handoff", job: models.QueueJob{Status: JobStatusCompleted, Stage: JobStageReadyToPublish}, previousStatus: JobStatusCompleted, want: JobStageReadyToPublish},
		{name: "published completion remains terminal", job: models.QueueJob{Status: JobStatusCompleted, Stage: JobStageCompleted, PublishedAt: &publishedAt}, previousStatus: JobStatusCompleted, want: JobStageCompleted},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := stageForStatusUpdate(test.job, test.previousStatus); got != test.want {
				t.Fatalf("stage=%q want=%q", got, test.want)
			}
		})
	}
}

func TestTransitionJobStagePropagatesPersistenceFailureWithoutMutatingCaller(t *testing.T) {
	db := testEncodeTestDB(t, "lifecycle-stage-write-failure", &models.QueueJob{})
	now := time.Now().UTC()
	job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageValidating, StageUpdatedAt: &now, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageValidating, "at": now.Format(time.RFC3339)}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_ready_stage BEFORE UPDATE OF stage ON queue_jobs WHEN NEW.stage = 'ready_to_publish' BEGIN SELECT RAISE(ABORT, 'stage write rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}

	err := transitionJobStage(db, &job, JobStageReadyToPublish)
	if err == nil {
		t.Fatal("expected stage persistence error")
	}
	if job.Stage != JobStageValidating || len(job.StageHistory) != 1 {
		t.Fatalf("failed transition mutated caller: stage=%q history=%#v", job.Stage, job.StageHistory)
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != JobStageValidating || len(persisted.StageHistory) != 1 {
		t.Fatalf("failed transition changed persisted history: stage=%q history=%#v", persisted.Stage, persisted.StageHistory)
	}
}

func TestTransitionJobStageRejectsStaleConcurrentWriterWithoutLosingHistory(t *testing.T) {
	db := testEncodeTestDB(t, "lifecycle-stage-stale-writer", &models.QueueJob{})
	now := time.Now().UTC()
	job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageValidating, StageUpdatedAt: &now, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageValidating, "at": now.Format(time.RFC3339)}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	var first, stale models.QueueJob
	if err := db.First(&first, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stale, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := transitionJobStage(db, &first, JobStageValidationReview); err != nil {
		t.Fatal(err)
	}
	if err := transitionJobStage(db, &stale, JobStageReadyToPublish); err == nil {
		t.Fatal("stale writer overwrote a newer Validation handoff")
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	secondStage := ""
	if len(persisted.StageHistory) > 1 {
		if event, ok := persisted.StageHistory[1].(map[string]any); ok {
			secondStage, _ = event["stage"].(string)
		}
	}
	if persisted.Stage != JobStageValidationReview || len(persisted.StageHistory) != 2 || secondStage != JobStageValidationReview {
		t.Fatalf("newer stage history was not preserved: stage=%q history=%#v", persisted.Stage, persisted.StageHistory)
	}
}

func TestTransitionJobStageSameTargetIsIdempotentAcrossStaleCopies(t *testing.T) {
	db := testEncodeTestDB(t, "lifecycle-stage-idempotent", &models.QueueJob{})
	now := time.Now().UTC()
	job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageValidating, StageUpdatedAt: &now, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageValidating, "at": now.Format(time.RFC3339)}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	var first, stale models.QueueJob
	if err := db.First(&first, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stale, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := transitionJobStage(db, &first, JobStageReadyToPublish); err != nil {
		t.Fatal(err)
	}
	if err := transitionJobStage(db, &stale, JobStageReadyToPublish); err != nil {
		t.Fatalf("idempotent duplicate transition failed: %v", err)
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != JobStageReadyToPublish || len(persisted.StageHistory) != 2 {
		t.Fatalf("duplicate stage event was recorded: stage=%q history=%#v", persisted.Stage, persisted.StageHistory)
	}
}

func TestWorkerStatusCompletionWaitsForValidationAndPreservesLaterHandoff(t *testing.T) {
	db := testEncodeTestDB(t, "worker-status-completion-stage", &models.QueueJob{})
	now := time.Now().UTC()
	job := models.QueueJob{Status: JobStatusRunning, Stage: JobStageConverting, StageUpdatedAt: &now, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageConverting, "at": now.Format(time.RFC3339)}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/workers/jobs/:id/status", NewWorkerHandler(db).UpdateJobStatus)
	complete := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/workers/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/status", strings.NewReader(`{"status":"completed","progress":100}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		return recorder
	}
	if recorder := complete(); recorder.Code != http.StatusOK {
		t.Fatalf("completion status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != JobStatusCompleted || persisted.Stage != JobStageValidationReview {
		t.Fatalf("status-only completion bypassed Validation: status=%q stage=%q", persisted.Status, persisted.Stage)
	}
	if err := transitionJobStage(db, &persisted, JobStageReadyToPublish); err != nil {
		t.Fatal(err)
	}
	if recorder := complete(); recorder.Code != http.StatusOK {
		t.Fatalf("repeated completion status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != JobStageReadyToPublish || len(persisted.StageHistory) != 3 {
		t.Fatalf("repeated completion regressed or duplicated handoff stage=%q history=%#v", persisted.Stage, persisted.StageHistory)
	}
}
