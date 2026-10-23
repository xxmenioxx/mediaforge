package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func manualPublisherRequestRouter(t *testing.T, fixture publisherDeliveryFixture) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := PublisherHandler{db: fixture.db}
	router.POST("/api/publisher/jobs/:id/publish", handler.PublishJob)
	router.POST("/api/publisher/publications/:publicationId/actions", handler.PublisherAction)
	router.GET("/api/publisher/publications", handler.ListPublications)
	return router
}

func sendManualPublisherRequest(t *testing.T, router http.Handler, jobID uint, reason, idempotencyKey string) (int, ManualPublishRequestResult, time.Duration) {
	t.Helper()
	body, err := json.Marshal(PublishJobInput{Reason: reason, IdempotencyKey: idempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/publisher/jobs/"+strconv.FormatUint(uint64(jobID), 10)+"/publish", bytes.NewReader(body))
	response := httptest.NewRecorder()
	started := time.Now()
	router.ServeHTTP(response, request)
	latency := time.Since(started)
	var result ManualPublishRequestResult
	if response.Code == http.StatusAccepted || response.Code == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode manual Publish response: %v; body=%s", err, response.Body.String())
		}
	}
	return response.Code, result, latency
}

func sendPublisherAction(t *testing.T, router http.Handler, publicationID, action, reason, idempotencyKey string) (int, ManualPublishRequestResult, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"action": action, "reason": reason, "idempotencyKey": idempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/publisher/publications/"+publicationID+"/actions", bytes.NewReader(body))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var result ManualPublishRequestResult
	if response.Code == http.StatusAccepted || (response.Code == http.StatusOK && action == "retry") {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode Publisher action response: %v; body=%s", err, response.Body.String())
		}
	}
	return response.Code, result, response.Body.String()
}

type manualPublisherRequestStatus struct {
	State               string `json:"state"`
	ManualRequestID     string `json:"manualRequestId"`
	ManualRequestStatus string `json:"manualRequestStatus"`
}

func readManualPublisherRequestStatuses(t *testing.T, router http.Handler) []manualPublisherRequestStatus {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/publisher/publications", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("publication status endpoint failed: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Items []struct {
			Publication manualPublisherRequestStatus `json:"publication"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	statuses := make([]manualPublisherRequestStatus, 0, len(response.Items))
	for _, item := range response.Items {
		statuses = append(statuses, item.Publication)
	}
	return statuses
}

func TestManualPublishAcceptsOneDurableRequestAndMeetsLocalAckTarget(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	router := manualPublisherRequestRouter(t, fixture)
	const requestKey = "manual-publish-request-key"
	latencies := make([]time.Duration, 0, 24)
	var accepted ManualPublishRequestResult
	for i := 0; i < 8; i++ {
		status, result, latency := sendManualPublisherRequest(t, router, fixture.job.ID, "operator approved manual delivery", requestKey)
		if status != http.StatusAccepted || result.Status != publisherManualRequestAccepted || result.RequestID == "" || result.PublicationID == "" {
			t.Fatalf("manual Publish was not durably accepted: status=%d result=%#v", status, result)
		}
		if accepted.RequestID != "" && result.RequestID != accepted.RequestID {
			t.Fatalf("duplicate request changed identity: first=%#v duplicate=%#v", accepted, result)
		}
		accepted = result
		latencies = append(latencies, latency)
	}

	db, err := fixture.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	const concurrentRequests = 16
	var wait sync.WaitGroup
	errs := make(chan error, concurrentRequests)
	concurrentLatencies := make(chan time.Duration, concurrentRequests)
	for i := 0; i < concurrentRequests; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			status, result, latency := sendManualPublisherRequest(t, router, fixture.job.ID, "operator approved manual delivery", requestKey)
			if status != http.StatusAccepted || result.RequestID != accepted.RequestID {
				errs <- fmt.Errorf("concurrent duplicate did not return the original request: status=%d result=%#v", status, result)
				return
			}
			concurrentLatencies <- latency
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	close(concurrentLatencies)
	for latency := range concurrentLatencies {
		latencies = append(latencies, latency)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[(len(latencies)*50+99)/100-1]
	p95 := latencies[(len(latencies)*95+99)/100-1]
	t.Logf("manual Publish ACK p50: %s, p95: %s across %d local SQLite fixture requests", p50, p95, len(latencies))
	if p95 > time.Second {
		t.Fatalf("manual Publish ACK p95 %s exceeds the one-second target", p95)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", accepted.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationReady || publication.ManualRequestStatus != publisherManualRequestAccepted || publication.AttemptCount != 0 {
		t.Fatalf("HTTP acceptance ran delivery work or lost intent: %#v", publication)
	}
	var acceptedAuditCount int64
	if err := fixture.db.Model(&models.PublisherAuditEvent{}).Where("publication_id = ? AND action = ?", publication.PublicationID, "publish_accepted").Count(&acceptedAuditCount).Error; err != nil {
		t.Fatal(err)
	}
	if acceptedAuditCount != 1 {
		t.Fatalf("duplicate clicks appended %d acceptance events", acceptedAuditCount)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("HTTP acceptance materialized a destination before worker execution: %v", err)
	}
	statuses := readManualPublisherRequestStatuses(t, router)
	if len(statuses) != 1 || statuses[0].ManualRequestID != accepted.RequestID || statuses[0].ManualRequestStatus != publisherManualRequestAccepted {
		t.Fatalf("publication status endpoint did not expose durable accepted progress: %#v", statuses)
	}
}

func TestManualPublishWorkerResumesAfterRestartWhenAutomaticPublishingIsDisabled(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	router := manualPublisherRequestRouter(t, fixture)
	status, accepted, _ := sendManualPublisherRequest(t, router, fixture.job.ID, "operator approved manual delivery", "restart-request-key")
	if status != http.StatusAccepted {
		t.Fatalf("manual Publish status=%d body did not accept request: %#v", status, accepted)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("accepted request was published before the worker tick: %v", err)
	}

	// A newly constructed Publisher worker uses only the persisted request and
	// the normal startup/periodic recovery path.
	if err := (&AutoPublisher{db: fixture.db}).tick(); err != nil {
		t.Fatalf("process accepted manual request after restart: %v", err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", accepted.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationPublished || publication.ManualRequestStatus != publisherManualRequestCompleted || publication.AttemptCount != 1 {
		t.Fatalf("manual publication did not reach one committed attempt: %#v", publication)
	}
	var outboxCount int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("manual publication created %d AssetPublished events", outboxCount)
	}
	if err := (&AutoPublisher{db: fixture.db}).tick(); err != nil {
		t.Fatalf("second recovery tick failed: %v", err)
	}
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.AttemptCount != 1 {
		t.Fatalf("repeated recovery started another attempt: %#v", publication)
	}
}

func TestManualPublisherRecoversExpiredAcceptedAttemptAfterRestart(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	router := manualPublisherRequestRouter(t, fixture)
	status, accepted, _ := sendManualPublisherRequest(t, router, fixture.job.ID, "operator approved manual delivery", "expired-attempt-key")
	if status != http.StatusAccepted {
		t.Fatalf("manual Publish status=%d result=%#v", status, accepted)
	}
	claimed, _, err := (PublisherHandler{db: fixture.db}).claimPublication(accepted.PublicationID, publicationActorOperator, publicationActorOperator, false)
	if err != nil || !claimed {
		t.Fatalf("simulate persisted manual attempt claim: claimed=%t err=%v", claimed, err)
	}
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", accepted.PublicationID).Update("lease_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", accepted.PublicationID).Update("manual_request_status", publisherManualRequestProcessing).Error; err != nil {
		t.Fatal(err)
	}
	statuses := readManualPublisherRequestStatuses(t, router)
	if len(statuses) != 1 || statuses[0].ManualRequestStatus != publisherManualRequestProcessing || statuses[0].State != PublicationPublishing {
		t.Fatalf("publication status endpoint did not expose active delivery progress: %#v", statuses)
	}
	if err := (&AutoPublisher{db: fixture.db}).tick(); err != nil {
		t.Fatalf("recover accepted manual attempt after restart: %v", err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", accepted.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationPublished || publication.ManualRequestStatus != publisherManualRequestCompleted || publication.AttemptCount != 1 {
		t.Fatalf("expired request did not recover its existing attempt exactly once: %#v", publication)
	}
	statuses = readManualPublisherRequestStatuses(t, router)
	if len(statuses) != 1 || statuses[0].ManualRequestStatus != publisherManualRequestCompleted || statuses[0].State != PublicationPublished {
		t.Fatalf("publication status endpoint did not expose final published progress: %#v", statuses)
	}
	var outboxCount int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("recovered request emitted %d AssetPublished events", outboxCount)
	}
}

func TestManualPublisherFailureRequiresAuditedRetryAndHoldPausesIt(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, true)
	router := manualPublisherRequestRouter(t, fixture)
	if err := os.Remove(fixture.sidecar); err != nil {
		t.Fatal(err)
	}
	status, accepted, _ := sendManualPublisherRequest(t, router, fixture.job.ID, "operator approved manual delivery", "initial-request-key")
	if status != http.StatusAccepted {
		t.Fatalf("manual Publish status=%d result=%#v", status, accepted)
	}
	if err := (&AutoPublisher{db: fixture.db}).tick(); err == nil {
		t.Fatal("worker did not report the required-sidecar failure")
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", accepted.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationBlocked || publication.ManualRequestStatus != publisherManualRequestFailed {
		t.Fatalf("missing required artifact was not surfaced as a terminal manual request failure: %#v", publication)
	}
	statuses := readManualPublisherRequestStatuses(t, router)
	if len(statuses) != 1 || statuses[0].ManualRequestStatus != publisherManualRequestFailed || statuses[0].State != PublicationBlocked {
		t.Fatalf("publication status endpoint did not expose failed progress: %#v", statuses)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("failed request promoted media before sidecar validation: %v", err)
	}
	if _, err := os.Stat(fixture.original); err != nil {
		t.Fatalf("failed request archived the original: %v", err)
	}
	status, _, _ = sendManualPublisherRequest(t, router, fixture.job.ID, "operator approved manual delivery", "initial-request-key")
	if status != http.StatusConflict {
		t.Fatalf("replayed failed request key was accepted again: status=%d", status)
	}
	if err := os.WriteFile(fixture.sidecar, []byte("[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,delivery sidecar\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const retryKey = "retry-request-key"
	status, retry, _ := sendPublisherAction(t, router, publication.PublicationID, "retry", "restored the required delivery artifact", retryKey)
	if status != http.StatusAccepted || retry.Status != publisherManualRequestAccepted || retry.RequestID == "" {
		t.Fatalf("manual retry was not durably accepted: status=%d result=%#v", status, retry)
	}
	duplicateStatus, duplicateRetry, _ := sendPublisherAction(t, router, publication.PublicationID, "retry", "restored the required delivery artifact", retryKey)
	if duplicateStatus != http.StatusAccepted || duplicateRetry.RequestID != retry.RequestID {
		t.Fatalf("duplicate retry did not return the accepted request: status=%d result=%#v", duplicateStatus, duplicateRetry)
	}
	var retryAuditCount int64
	if err := fixture.db.Model(&models.PublisherAuditEvent{}).Where("publication_id = ? AND action = ?", publication.PublicationID, "retry_accepted").Count(&retryAuditCount).Error; err != nil {
		t.Fatal(err)
	}
	if retryAuditCount != 1 {
		t.Fatalf("duplicate retry appended %d acceptance events", retryAuditCount)
	}
	status, _, actionBody := sendPublisherAction(t, router, publication.PublicationID, "hold", "pause while destination is reviewed", "")
	if status != http.StatusOK {
		t.Fatalf("hold failed: status=%d body=%s", status, actionBody)
	}
	if err := (&AutoPublisher{db: fixture.db}).tick(); err != nil {
		t.Fatalf("held request tick failed: %v", err)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("held manual request wrote the destination: %v", err)
	}
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationHeld || publication.ManualRequestStatus != publisherManualRequestAccepted {
		t.Fatalf("hold did not preserve the pending manual request: %#v", publication)
	}
	status, _, actionBody = sendPublisherAction(t, router, publication.PublicationID, "release", "resume reviewed destination", "")
	if status != http.StatusOK {
		t.Fatalf("release failed: status=%d body=%s", status, actionBody)
	}
	if err := (&AutoPublisher{db: fixture.db}).tick(); err != nil {
		t.Fatalf("released manual request did not resume: %v", err)
	}
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationPublished || publication.ManualRequestStatus != publisherManualRequestCompleted || publication.AttemptCount != 1 {
		t.Fatalf("retry after safe blocked state did not publish exactly once: %#v", publication)
	}
}

func TestManualPublishDoesNotAcceptWhenPrecheckOrAuditPersistenceFails(t *testing.T) {
	t.Run("canonical precheck", func(t *testing.T) {
		fixture := newPublisherDeliveryFixture(t, false)
		if err := fixture.db.Model(&models.ValidationRun{}).Where("run_id = ?", fixture.job.ValidationRunID).Update("outcome", "FAIL").Error; err != nil {
			t.Fatal(err)
		}
		status, _, _ := sendManualPublisherRequest(t, manualPublisherRequestRouter(t, fixture), fixture.job.ID, "operator approved manual delivery", "stale-validation-key")
		if status != http.StatusConflict {
			t.Fatalf("stale Validation was accepted: status=%d", status)
		}
		var count int64
		if err := fixture.db.Model(&models.PublisherPublication{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("pre-acceptance Validation failure created %d publication identities", count)
		}
	})

	t.Run("audit write failure", func(t *testing.T) {
		fixture := newPublisherDeliveryFixture(t, false)
		if err := fixture.db.Exec(`CREATE TRIGGER reject_publish_accepted BEFORE INSERT ON publisher_audit_events WHEN NEW.action = 'publish_accepted' BEGIN SELECT RAISE(ABORT, 'audit persistence unavailable'); END`).Error; err != nil {
			t.Fatal(err)
		}
		status, _, _ := sendManualPublisherRequest(t, manualPublisherRequestRouter(t, fixture), fixture.job.ID, "operator approved manual delivery", "audit-failure-key")
		if status == http.StatusAccepted {
			t.Fatal("Publisher acknowledged a request whose audit event could not be stored")
		}
		var publication models.PublisherPublication
		if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
			t.Fatal(err)
		}
		if publication.ManualRequestID != "" || publication.ManualRequestStatus != "" {
			t.Fatalf("failed audit transaction left an accepted request behind: %#v", publication)
		}
		var acceptedAuditCount int64
		if err := fixture.db.Model(&models.PublisherAuditEvent{}).Where("publication_id = ? AND action = ?", publication.PublicationID, "publish_accepted").Count(&acceptedAuditCount).Error; err != nil {
			t.Fatal(err)
		}
		if acceptedAuditCount != 0 {
			t.Fatalf("failed acceptance left %d audit events", acceptedAuditCount)
		}
	})
}
