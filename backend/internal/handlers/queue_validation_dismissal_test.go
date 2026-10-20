package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

type queueValidationDismissalFixture struct {
	delivery   publisherDeliveryFixture
	run        models.ValidationRun
	result     map[string]any
	resultPath string
}

func newQueueValidationDismissalFixture(t *testing.T, outcome string) queueValidationDismissalFixture {
	t.Helper()
	delivery := newPublisherDeliveryFixture(t, false)
	if err := delivery.db.AutoMigrate(&models.AssetMaintenanceOperation{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	job := delivery.job
	job.Stage = JobStageValidationReview
	job.PublicationID = ""
	job.PublicationState = ""
	job.PublicationSummary = ""
	if err := delivery.db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	var run models.ValidationRun
	if err := delivery.db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	state := "FAIL"
	if outcome == models.QualityOutcomeReviewRequired {
		state = "REVIEW_REQUIRED"
	}
	run.Outcome = outcome
	run.Findings = models.JSONList{models.JSONMap{
		"key": "artifact.integrity", "expected": "current output", "actual": "unresolved",
		"severity": "error", "disposition": "blocking", "state": state,
		"actionable": true, "overrideable": false, "explanation": "synthetic dismissal fixture",
		"evidenceReferences": []any{"result.outputProbe"},
	}}
	if err := delivery.db.Save(&run).Error; err != nil {
		t.Fatal(err)
	}
	job.ValidationOutcome = outcome
	job.ValidationStatus = ValidationStatusFailed
	if err := delivery.db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	delivery.job = job
	result, resultPath, err := readLatestJobArtifact(delivery.db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	return queueValidationDismissalFixture{delivery: delivery, run: run, result: result, resultPath: resultPath}
}

func TestDismissCompletedValidationExceptionsPreservesHistoryAndAllowsFreshQueue(t *testing.T) {
	for _, outcome := range []string{models.QualityOutcomeFail, models.QualityOutcomeReviewRequired} {
		t.Run(outcome, func(t *testing.T) {
			fixture := newQueueValidationDismissalFixture(t, outcome)
			delivery := fixture.delivery
			if err := scheduler.LockQueuedAsset(delivery.db, delivery.job); err != nil {
				t.Fatal(err)
			}
			outputBefore, err := os.ReadFile(delivery.output)
			if err != nil {
				t.Fatal(err)
			}
			sourceBefore, err := os.ReadFile(delivery.original)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(delivery.sidecar, []byte("preserved staged sidecar"), 0o600); err != nil {
				t.Fatal(err)
			}
			sidecarBefore, err := os.ReadFile(delivery.sidecar)
			if err != nil {
				t.Fatal(err)
			}
			resultBefore, err := os.ReadFile(fixture.resultPath)
			if err != nil {
				t.Fatal(err)
			}

			previousGinMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			t.Cleanup(func() { gin.SetMode(previousGinMode) })
			router := gin.New()
			router.DELETE("/api/queue/jobs/:id", NewQueueHandler(delivery.db).Dismiss)
			router.GET("/api/queue/jobs", NewQueueHandler(delivery.db).List)
			validation := NewValidationHandler(delivery.db)
			router.GET("/api/validation/review", validation.ReviewList)
			router.GET("/api/validation/history", validation.HistoryList)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(delivery.job.ID)), nil))
			if response.Code != http.StatusOK {
				t.Fatalf("dismiss status=%d body=%s", response.Code, response.Body.String())
			}
			var dismissed models.QueueJob
			if err := json.Unmarshal(response.Body.Bytes(), &dismissed); err != nil {
				t.Fatal(err)
			}
			if dismissed.DismissedAt == nil || dismissed.Status != JobStatusCompleted || dismissed.ValidationRunID != fixture.run.RunID {
				t.Fatalf("dismissal changed historical job state: %#v", dismissed)
			}
			dismissedAt := *dismissed.DismissedAt

			var jobs []models.QueueJob
			var reservations, plans, runs int64
			if err := delivery.db.Where("id = ?", delivery.job.ID).Find(&jobs).Error; err != nil || len(jobs) != 1 {
				t.Fatalf("historical QueueJob was not preserved: rows=%d err=%v", len(jobs), err)
			}
			if err := delivery.db.Model(&models.SchedulerReservation{}).Where("job_id = ?", delivery.job.ID).Count(&reservations).Error; err != nil || reservations != 0 {
				t.Fatalf("reservation was not released exactly once: count=%d err=%v", reservations, err)
			}
			if err := delivery.db.Model(&models.ExecutionPlan{}).Where("job_id = ?", delivery.job.ID).Count(&plans).Error; err != nil || plans != 1 {
				t.Fatalf("frozen ExecutionPlan count=%d err=%v", plans, err)
			}
			if err := delivery.db.Model(&models.ValidationRun{}).Where("job_id = ?", delivery.job.ID).Count(&runs).Error; err != nil || runs != 1 {
				t.Fatalf("ValidationRun count=%d err=%v", runs, err)
			}
			for path, expected := range map[string][]byte{
				delivery.output: outputBefore, delivery.original: sourceBefore,
				delivery.sidecar: sidecarBefore, fixture.resultPath: resultBefore,
			} {
				actual, readErr := os.ReadFile(path)
				if readErr != nil || string(actual) != string(expected) {
					t.Fatalf("dismissal changed preserved file %q: err=%v", path, readErr)
				}
			}

			queueContext, err := newQueueBatchValidationContext(delivery.db)
			if err != nil {
				t.Fatal(err)
			}
			if queueContext.assetHasOpenJob(delivery.job.MediaPath, 0) {
				t.Fatal("dismissed completed job still blocks fresh Queue preflight")
			}
			if active, err := (QueueHandler{db: delivery.db}).assetHasOpenJob(delivery.job.MediaPath, 0); err != nil || active {
				t.Fatalf("fresh Queue eligibility active=%t err=%v", active, err)
			}

			listResponse := httptest.NewRecorder()
			router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/queue/jobs", nil))
			if listResponse.Code != http.StatusOK {
				t.Fatalf("Queue list status=%d body=%s", listResponse.Code, listResponse.Body.String())
			}
			var activeJobs []models.QueueJob
			if err := json.Unmarshal(listResponse.Body.Bytes(), &activeJobs); err != nil || len(activeJobs) != 0 {
				t.Fatalf("dismissed job remained in Queue: jobs=%#v err=%v", activeJobs, err)
			}
			reviewResponse := httptest.NewRecorder()
			router.ServeHTTP(reviewResponse, httptest.NewRequest(http.MethodGet, "/api/validation/review?view=all", nil))
			var review validationSummaryPage
			if err := json.Unmarshal(reviewResponse.Body.Bytes(), &review); err != nil || review.Total != 0 {
				t.Fatalf("dismissed job remained in active Validation: total=%d err=%v", review.Total, err)
			}
			historyResponse := httptest.NewRecorder()
			router.ServeHTTP(historyResponse, httptest.NewRequest(http.MethodGet, "/api/validation/history", nil))
			var history validationSummaryPage
			if err := json.Unmarshal(historyResponse.Body.Bytes(), &history); err != nil || history.Total != 1 || len(history.Items) != 1 {
				t.Fatalf("History omitted dismissed job: total=%d err=%v", history.Total, err)
			}
			if history.Items[0].DismissedAt == nil || !history.Items[0].DismissedAt.Equal(dismissedAt) || history.Items[0].ValidationOutcome != outcome {
				t.Fatalf("History omitted dismissal status/timestamp: %#v", history.Items[0])
			}

			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(delivery.job.ID)), nil))
			if response.Code != http.StatusOK {
				t.Fatalf("repeated dismissal status=%d body=%s", response.Code, response.Body.String())
			}
			var repeated models.QueueJob
			if err := json.Unmarshal(response.Body.Bytes(), &repeated); err != nil || repeated.DismissedAt == nil || !repeated.DismissedAt.Equal(dismissedAt) {
				t.Fatalf("repeated dismissal was not idempotent: job=%#v err=%v", repeated, err)
			}
			if err := delivery.db.Model(&models.SchedulerReservation{}).Where("job_id = ?", delivery.job.ID).Count(&reservations).Error; err != nil || reservations != 0 {
				t.Fatalf("repeated dismissal changed reservations: count=%d err=%v", reservations, err)
			}
		})
	}
}

func TestDismissCompletedValidationDoesNotRequireOutputFile(t *testing.T) {
	fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
	delivery := fixture.delivery
	if err := os.Remove(delivery.output); err != nil {
		t.Fatalf("remove synthetic output before dismissal: %v", err)
	}
	if _, err := os.Stat(delivery.output); !os.IsNotExist(err) {
		t.Fatalf("expected output file to be absent before dismissal, stat err=%v", err)
	}

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.DELETE("/api/queue/jobs/:id", NewQueueHandler(delivery.db).Dismiss)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(delivery.job.ID)), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("dismiss status=%d body=%s", response.Code, response.Body.String())
	}
	var dismissed models.QueueJob
	if err := json.Unmarshal(response.Body.Bytes(), &dismissed); err != nil {
		t.Fatal(err)
	}
	if dismissed.DismissedAt == nil || dismissed.Status != JobStatusCompleted || dismissed.ValidationRunID != fixture.run.RunID {
		t.Fatalf("dismissal changed historical job identity: %#v", dismissed)
	}
	if _, err := os.Stat(delivery.output); !os.IsNotExist(err) {
		t.Fatalf("dismissal recreated the absent output file, stat err=%v", err)
	}
}

func TestDismissCompletedValidationRejectsStaleOrConflictingJobsWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *queueValidationDismissalFixture)
	}{
		{name: "missing active run", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			fixture.delivery.job.ValidationRunID = "missing-run"
			if err := fixture.delivery.db.Save(&fixture.delivery.job).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "stale output revision binding", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			if err := fixture.delivery.db.Model(&models.ValidationRun{}).Where("run_id = ?", fixture.run.RunID).Update("result_output_revision", "stale-output-revision").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "stale frozen plan", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			if err := fixture.delivery.db.Model(&models.ExecutionPlan{}).Where("job_id = ?", fixture.delivery.job.ID).Update("selected_encoder", "stale-encoder").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "successful outcome", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			var run models.ValidationRun
			if err := fixture.delivery.db.Where("run_id = ?", fixture.run.RunID).First(&run).Error; err != nil {
				t.Fatal(err)
			}
			run.Outcome = models.QualityOutcomePass
			run.Findings = models.JSONList{models.JSONMap{"key": "artifact.integrity", "state": "PASS", "disposition": "blocking"}}
			if err := fixture.delivery.db.Save(&run).Error; err != nil {
				t.Fatal(err)
			}
			fixture.delivery.job.ValidationOutcome = models.QualityOutcomePass
			if err := fixture.delivery.db.Save(&fixture.delivery.job).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "ready to publish", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			fixture.delivery.job.Stage = JobStageReadyToPublish
			if err := fixture.delivery.db.Save(&fixture.delivery.job).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "published", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			now := time.Now().UTC()
			fixture.delivery.job.PublishedAt = &now
			fixture.delivery.job.PublishedPath = fixture.delivery.destination
			if err := fixture.delivery.db.Save(&fixture.delivery.job).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "publisher identity exists", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			candidate, identity, err := publisherIdentityForJob(fixture.delivery.db, fixture.delivery.job, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := (PublisherHandler{db: fixture.delivery.db}).persistPublicationIdentity(candidate, identity); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "active maintenance", mutate: func(t *testing.T, fixture *queueValidationDismissalFixture) {
			if err := fixture.delivery.db.Create(&models.AssetMaintenanceOperation{ID: "active-dismiss-maintenance", AssetPath: fixture.delivery.job.MediaPath, Status: maintenanceStatusRunning, Phase: maintenanceStatusRunning}).Error; err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
			test.mutate(t, &fixture)
			if err := scheduler.LockQueuedAsset(fixture.delivery.db, fixture.delivery.job); err != nil {
				t.Fatal(err)
			}
			previousGinMode := gin.Mode()
			gin.SetMode(gin.TestMode)
			t.Cleanup(func() { gin.SetMode(previousGinMode) })
			router := gin.New()
			router.DELETE("/api/queue/jobs/:id", NewQueueHandler(fixture.delivery.db).Dismiss)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(fixture.delivery.job.ID)), nil))
			if response.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var persisted models.QueueJob
			if err := fixture.delivery.db.First(&persisted, fixture.delivery.job.ID).Error; err != nil {
				t.Fatal(err)
			}
			if persisted.DismissedAt != nil {
				t.Fatalf("conflicting dismissal partially changed QueueJob: %#v", persisted)
			}
			var reservations int64
			if err := fixture.delivery.db.Model(&models.SchedulerReservation{}).Where("job_id = ?", fixture.delivery.job.ID).Count(&reservations).Error; err != nil || reservations != 1 {
				t.Fatalf("conflicting dismissal released reservation: count=%d err=%v", reservations, err)
			}
		})
	}
}

func TestDismissAndPublisherHandoffHaveSingleWinner(t *testing.T) {
	t.Run("dismissal wins before Publisher identity creation", func(t *testing.T) {
		fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
		candidate, identity, err := publisherIdentityForJob(fixture.delivery.db, fixture.delivery.job, false)
		if err != nil {
			t.Fatal(err)
		}
		previousGinMode := gin.Mode()
		gin.SetMode(gin.TestMode)
		t.Cleanup(func() { gin.SetMode(previousGinMode) })
		router := gin.New()
		router.DELETE("/api/queue/jobs/:id", NewQueueHandler(fixture.delivery.db).Dismiss)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(fixture.delivery.job.ID)), nil))
		if response.Code != http.StatusOK {
			t.Fatalf("dismiss status=%d body=%s", response.Code, response.Body.String())
		}
		if _, err := (PublisherHandler{db: fixture.delivery.db}).persistPublicationIdentity(candidate, identity); err == nil {
			t.Fatal("Publisher created an identity after Queue dismissal won")
		} else {
			var publishErr publishError
			if !errors.As(err, &publishErr) || publishErr.Status != http.StatusConflict {
				t.Fatalf("Publisher returned unexpected error after dismissal: %v", err)
			}
		}
		var publications int64
		if err := fixture.delivery.db.Model(&models.PublisherPublication{}).Where("queue_job_id = ?", fixture.delivery.job.ID).Count(&publications).Error; err != nil || publications != 0 {
			t.Fatalf("Publisher identity was partially persisted: count=%d err=%v", publications, err)
		}
	})

	t.Run("Publisher identity wins before dismissal", func(t *testing.T) {
		fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
		candidate, identity, err := publisherIdentityForJob(fixture.delivery.db, fixture.delivery.job, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (PublisherHandler{db: fixture.delivery.db}).persistPublicationIdentity(candidate, identity); err != nil {
			t.Fatal(err)
		}
		previousGinMode := gin.Mode()
		gin.SetMode(gin.TestMode)
		t.Cleanup(func() { gin.SetMode(previousGinMode) })
		router := gin.New()
		router.DELETE("/api/queue/jobs/:id", NewQueueHandler(fixture.delivery.db).Dismiss)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(fixture.delivery.job.ID)), nil))
		if response.Code != http.StatusConflict {
			t.Fatalf("dismiss raced past Publisher identity: status=%d body=%s", response.Code, response.Body.String())
		}
		var job models.QueueJob
		if err := fixture.delivery.db.First(&job, fixture.delivery.job.ID).Error; err != nil {
			t.Fatal(err)
		}
		if job.DismissedAt != nil || job.PublicationState != PublicationReady {
			t.Fatalf("Publisher winning identity was not preserved: %#v", job)
		}
	})

	t.Run("simultaneous Queue and Publisher requests", func(t *testing.T) {
		fixture := newQueueValidationDismissalFixture(t, models.QualityOutcomeFail)
		candidate, identity, err := publisherIdentityForJob(fixture.delivery.db, fixture.delivery.job, false)
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, err := fixture.delivery.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		// Serialize SQLite connection use while both goroutines race for the
		// transaction boundary; the row checks still decide the winner.
		sqlDB.SetMaxOpenConns(1)
		previousGinMode := gin.Mode()
		gin.SetMode(gin.TestMode)
		t.Cleanup(func() { gin.SetMode(previousGinMode) })
		router := gin.New()
		router.DELETE("/api/queue/jobs/:id", NewQueueHandler(fixture.delivery.db).Dismiss)
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		dismissStatus := make(chan int, 1)
		publisherErr := make(chan error, 1)
		go func() {
			defer wait.Done()
			<-start
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/"+strconv.Itoa(int(fixture.delivery.job.ID)), nil))
			dismissStatus <- response.Code
		}()
		go func() {
			defer wait.Done()
			<-start
			_, publishErr := (PublisherHandler{db: fixture.delivery.db}).persistPublicationIdentity(candidate, identity)
			publisherErr <- publishErr
		}()
		close(start)
		wait.Wait()
		status, publishErr := <-dismissStatus, <-publisherErr
		dismissWon, publisherWon := status == http.StatusOK, publishErr == nil
		if dismissWon == publisherWon {
			t.Fatalf("Queue and Publisher did not have a single transaction winner: dismiss=%d publishErr=%v", status, publishErr)
		}
		var job models.QueueJob
		if err := fixture.delivery.db.First(&job, fixture.delivery.job.ID).Error; err != nil {
			t.Fatal(err)
		}
		var publications int64
		if err := fixture.delivery.db.Model(&models.PublisherPublication{}).Where("queue_job_id = ?", job.ID).Count(&publications).Error; err != nil {
			t.Fatal(err)
		}
		if dismissWon && (job.DismissedAt == nil || publications != 0) {
			t.Fatalf("Dismiss winner left Publisher state: job=%#v publications=%d", job, publications)
		}
		if publisherWon && (job.DismissedAt != nil || publications != 1 || job.PublicationState != PublicationReady) {
			t.Fatalf("Publisher winner was dismissed or incompletely persisted: job=%#v publications=%d", job, publications)
		}
	})
}

func TestPublisherCannotConsumeDismissedJobWithHistoricalValidationApproval(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	if err := fixture.db.AutoMigrate(&models.ValidationOverride{}); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(fixture.db, fixture.job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := resultIdentityFromArtifact(artifact)
	if !ok {
		t.Fatal("test fixture Result identity is missing")
	}
	var run models.ValidationRun
	if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.ValidationOverride{
		EventID: "dismiss-test-old-override", ValidationRunID: run.RunID, Scope: "rule",
		ResultArtifactID: identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
		ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
		PolicySchemaVersion: run.PolicySchemaVersion, PolicyVersion: run.PolicyVersion, PolicyFingerprint: run.PolicyFingerprint,
		ActorSource: "operator", ActorID: "operator", Reason: "historical approval", RuleKeys: models.JSONList{"duration.timeline"},
		RequestedHandoff: "publisher", OverrideVersion: validationOverrideV1, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	candidate, resultIdentity, err := publisherIdentityForJob(fixture.db, fixture.job, false)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := (PublisherHandler{db: fixture.db}).persistPublicationIdentity(candidate, resultIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.QueueJob{}).Where("id = ?", fixture.job.ID).Update("dismissed_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	var dismissedJob models.QueueJob
	if err := fixture.db.First(&dismissedJob, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := transitionJobStage(fixture.db, &dismissedJob, JobStagePublishing); err == nil {
		t.Fatal("dismissed Queue job transitioned into Publisher work")
	}

	handler := PublisherHandler{db: fixture.db}
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.POST("/api/publisher/jobs/:id/publish", handler.PublishJob)
	router.POST("/api/publisher/publications/:publicationId/actions", handler.PublisherAction)
	router.GET("/api/publisher/eligibility", handler.ListManualEligibility)
	publishResponse := httptest.NewRecorder()
	router.ServeHTTP(publishResponse, httptest.NewRequest(http.MethodPost, "/api/publisher/jobs/"+strconv.Itoa(int(fixture.job.ID))+"/publish", strings.NewReader(`{"reason":"old UI action"}`)))
	if publishResponse.Code != http.StatusConflict {
		t.Fatalf("stale direct publish status=%d body=%s", publishResponse.Code, publishResponse.Body.String())
	}
	claim, _, err := handler.claimPublication(publication.PublicationID, publicationActorOperator, publicationActorOperator, false)
	var publishErr publishError
	if claim || !errors.As(err, &publishErr) || publishErr.Status != http.StatusConflict {
		t.Fatalf("dismissed publication claim accepted: claim=%t err=%v", claim, err)
	}
	eligibilityResponse := httptest.NewRecorder()
	router.ServeHTTP(eligibilityResponse, httptest.NewRequest(http.MethodGet, "/api/publisher/eligibility", nil))
	var eligibility PublisherEligibilityResponse
	if err := json.Unmarshal(eligibilityResponse.Body.Bytes(), &eligibility); err != nil || len(eligibility.Items) != 0 {
		t.Fatalf("dismissed job remained in manual Publisher eligibility: items=%#v err=%v", eligibility.Items, err)
	}
	actionResponse := httptest.NewRecorder()
	actionResponse.Header().Set("Content-Type", "application/json")
	router.ServeHTTP(actionResponse, httptest.NewRequest(http.MethodPost, "/api/publisher/publications/"+publication.PublicationID+"/actions", strings.NewReader(`{"action":"retry","reason":"stale action"}`)))
	if actionResponse.Code != http.StatusConflict {
		t.Fatalf("stale Publisher action status=%d body=%s", actionResponse.Code, actionResponse.Body.String())
	}
	var storedPublication models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&storedPublication).Error; err != nil {
		t.Fatal(err)
	}
	if storedPublication.State != PublicationReady || storedPublication.AttemptCount != 0 || storedPublication.ActiveAttemptID != "" {
		t.Fatalf("dismissed Publisher identity was claimed or modified: %#v", storedPublication)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("dismissed job created destination output: stat err=%v", err)
	}
}
