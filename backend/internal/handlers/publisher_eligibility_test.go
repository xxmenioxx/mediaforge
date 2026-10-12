package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

func publisherEligibilityReviewFixture(t *testing.T) publisherDeliveryFixture {
	t.Helper()
	fixture := newPublisherDeliveryFixture(t, false)
	if err := os.MkdirAll(fixture.libraryRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	var run models.ValidationRun
	if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	run.Outcome = "REVIEW_REQUIRED"
	run.PolicySchemaVersion = 1
	run.PolicyVersion = "validation-policy-v1"
	run.PolicyPreset = "custom"
	run.PolicyFingerprint = "sha256:publisher-eligibility-policy"
	run.PolicySnapshot = models.JSONMap{"schemaVersion": 1, "policyVersion": "validation-policy-v1", "preset": "custom"}
	run.Findings = models.JSONList{models.JSONMap{
		"key": "duration.timeline", "disposition": "blocking", "state": "REVIEW_REQUIRED", "overrideable": true,
	}}
	if err := fixture.db.Save(&run).Error; err != nil {
		t.Fatal(err)
	}
	fixture.job.ValidationOutcome = "REVIEW_REQUIRED"
	fixture.job.ValidationStatus = ValidationStatusFailed
	if err := fixture.db.Save(&fixture.job).Error; err != nil {
		t.Fatal(err)
	}
	return fixture
}

func addPublisherEligibilityOverride(t *testing.T, fixture publisherDeliveryFixture) models.ValidationOverride {
	t.Helper()
	var run models.ValidationRun
	if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	event := models.ValidationOverride{
		EventID: "validation-override:publisher-eligibility:" + t.Name(), ValidationRunID: run.RunID,
		Scope: "rule", ResultArtifactID: run.ResultArtifactID, ResultGenerationToken: run.ResultGenerationToken,
		ResultGenerationVersion: run.ResultGenerationVersion, ResultOutputRevision: run.ResultOutputRevision,
		PolicySchemaVersion: run.PolicySchemaVersion, PolicyVersion: run.PolicyVersion, PolicyFingerprint: run.PolicyFingerprint,
		ActorSource: publicationActorOperator, ActorID: publicationActorOperator, Reason: "Reviewed the unresolved timeline finding",
		RuleKeys: models.JSONList{"duration.timeline"}, RequestedHandoff: "publisher", OverrideVersion: "v1", CreatedAt: time.Now().UTC(),
	}
	if err := fixture.db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	return event
}

func requestPublisherEligibility(t *testing.T, fixture publisherDeliveryFixture, query string) (int, PublisherEligibilityResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/publisher/eligibility", PublisherHandler{db: fixture.db}.ListManualEligibility)
	request := httptest.NewRequest(http.MethodGet, "/api/publisher/eligibility"+query, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	var result PublisherEligibilityResponse
	if response.Code == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return response.Code, result
}

func TestPublisherManualEligibilityAllowsReviewRequiredPublisherOverrideReadOnly(t *testing.T) {
	fixture := publisherEligibilityReviewFixture(t)
	event := addPublisherEligibilityOverride(t, fixture)
	var runBefore models.ValidationRun
	if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&runBefore).Error; err != nil {
		t.Fatal(err)
	}
	findingsBefore, _ := json.Marshal(runBefore.Findings)
	var jobBefore models.QueueJob
	if err := fixture.db.First(&jobBefore, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	outputBefore, err := os.Stat(fixture.output)
	if err != nil {
		t.Fatal(err)
	}

	status, response := requestPublisherEligibility(t, fixture, "?limit=500")
	if status != http.StatusOK || response.Limit != publisherEligibilityLimit || len(response.Items) != 1 {
		t.Fatalf("bounded eligibility response status=%d response=%#v", status, response)
	}
	candidate := response.Items[0]
	if candidate.Job.ID != fixture.job.ID || candidate.Status != "ALLOWED_BY_OVERRIDE" || candidate.Reason != "" {
		t.Fatalf("manual Publisher eligibility did not recognize the canonical override: %#v", candidate)
	}
	if allowed, reason := canonicalRunAllowsPublish(fixture.db, fixture.job, true); allowed || reason == "" {
		t.Fatalf("Publisher-only override unexpectedly authorized AutoPublisher: allowed=%t reason=%q", allowed, reason)
	}
	var publications int64
	if err := fixture.db.Model(&models.PublisherPublication{}).Count(&publications).Error; err != nil || publications != 0 {
		t.Fatalf("eligibility listing persisted publication identity: count=%d err=%v", publications, err)
	}
	var jobAfter models.QueueJob
	if err := fixture.db.First(&jobAfter, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if jobAfter.Status != jobBefore.Status || jobAfter.Stage != jobBefore.Stage || jobAfter.Notes != jobBefore.Notes || !jobAfter.UpdatedAt.Equal(jobBefore.UpdatedAt) {
		t.Fatalf("eligibility listing changed QueueJob lifecycle state: before=%#v after=%#v", jobBefore, jobAfter)
	}
	var runAfter models.ValidationRun
	if err := fixture.db.Where("run_id = ?", runBefore.RunID).First(&runAfter).Error; err != nil {
		t.Fatal(err)
	}
	findingsAfter, _ := json.Marshal(runAfter.Findings)
	if !bytes.Equal(findingsBefore, findingsAfter) {
		t.Fatalf("eligibility listing changed immutable ValidationRun findings: before=%s after=%s", findingsBefore, findingsAfter)
	}
	var storedOverride models.ValidationOverride
	if err := fixture.db.Where("event_id = ?", event.EventID).First(&storedOverride).Error; err != nil || storedOverride.Reason != event.Reason {
		t.Fatalf("eligibility listing changed append-only override evidence: event=%#v err=%v", storedOverride, err)
	}
	if after, err := os.Stat(fixture.output); err != nil || after.Size() != outputBefore.Size() || !after.ModTime().Equal(outputBefore.ModTime()) {
		t.Fatalf("eligibility listing changed the staged Result: before=%#v after=%#v err=%v", outputBefore, after, err)
	}
	for _, path := range []string{fixture.destination, filepath.Join(fixture.libraryRoot, ".mvforge-staging"), fixture.archiveRoot} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("eligibility listing created or changed a delivery/archive path %q: %v", path, err)
		}
	}

	router := gin.New()
	router.POST("/api/publisher/jobs/:id/publish", PublisherHandler{db: fixture.db}.PublishJob)
	publishURL := "/api/publisher/jobs/" + strconv.FormatUint(uint64(fixture.job.ID), 10) + "/publish"
	request := httptest.NewRequest(http.MethodPost, publishURL, bytes.NewBufferString(`{"reason":"   "}`))
	request.Header.Set("Content-Type", "application/json")
	publishResponse := httptest.NewRecorder()
	router.ServeHTTP(publishResponse, request)
	if publishResponse.Code != http.StatusBadRequest {
		t.Fatalf("override-backed manual publication without a reason status=%d body=%s", publishResponse.Code, publishResponse.Body.String())
	}
	var publicationsAfterBlankReason int64
	if err := fixture.db.Model(&models.PublisherPublication{}).Count(&publicationsAfterBlankReason).Error; err != nil || publicationsAfterBlankReason != 0 {
		t.Fatalf("blank-reason request persisted a publication identity: count=%d err=%v", publicationsAfterBlankReason, err)
	}
	request = httptest.NewRequest(http.MethodPost, publishURL, bytes.NewBufferString(`{"reason":"Reviewed the unresolved timeline finding"}`))
	request.Header.Set("Content-Type", "application/json")
	publishResponse = httptest.NewRecorder()
	router.ServeHTTP(publishResponse, request)
	if publishResponse.Code != http.StatusOK {
		t.Fatalf("manual publication status=%d body=%s", publishResponse.Code, publishResponse.Body.String())
	}
	var publishResult PublishResult
	if err := json.Unmarshal(publishResponse.Body.Bytes(), &publishResult); err != nil || publishResult.Status != "published" {
		t.Fatalf("REVIEW_REQUIRED Result with a valid Publisher override was not manually publishable: result=%#v err=%v", publishResult, err)
	}
	var runAfterPublish models.ValidationRun
	if err := fixture.db.Where("run_id = ?", runBefore.RunID).First(&runAfterPublish).Error; err != nil {
		t.Fatal(err)
	}
	findingsAfterPublish, _ := json.Marshal(runAfterPublish.Findings)
	if !bytes.Equal(findingsBefore, findingsAfterPublish) {
		t.Fatalf("successful publication rewrote ValidationRun findings: before=%s after=%s", findingsBefore, findingsAfterPublish)
	}
	var overrideAfterPublish models.ValidationOverride
	if err := fixture.db.Where("event_id = ?", event.EventID).First(&overrideAfterPublish).Error; err != nil || overrideAfterPublish.Reason != event.Reason {
		t.Fatalf("successful publication changed append-only override evidence: event=%#v err=%v", overrideAfterPublish, err)
	}
}

func TestPublisherManualEligibilityBlocksCanonicalAndDeliveryFailures(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, fixture publisherDeliveryFixture)
		wantReason string
	}{
		{
			name: "missing publisher override",
			setup: func(t *testing.T, fixture publisherDeliveryFixture) {
				publisherEligibilityReviewFixtureUpdate(t, fixture, "REVIEW_REQUIRED", models.JSONList{models.JSONMap{
					"key": "duration.timeline", "disposition": "blocking", "state": "REVIEW_REQUIRED", "overrideable": true,
				}})
			},
			wantReason: "duration.timeline",
		},
		{
			name: "mandatory finding",
			setup: func(t *testing.T, fixture publisherDeliveryFixture) {
				publisherEligibilityReviewFixtureUpdate(t, fixture, "FAIL", models.JSONList{models.JSONMap{
					"key": "artifact.integrity", "disposition": "blocking", "state": "FAIL", "overrideable": true,
				}})
			},
			wantReason: "artifact.integrity",
		},
		{
			name: "mismatched Validation Result identity",
			setup: func(t *testing.T, fixture publisherDeliveryFixture) {
				var run models.ValidationRun
				if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&run).Error; err != nil {
					t.Fatal(err)
				}
				run.ResultArtifactID = "result:replaced"
				if err := fixture.db.Save(&run).Error; err != nil {
					t.Fatal(err)
				}
			},
			wantReason: "does not match the ValidationRun identity",
		},
		{
			name: "changed Result revision",
			setup: func(t *testing.T, fixture publisherDeliveryFixture) {
				if err := os.WriteFile(fixture.output, []byte("changed after validation"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantReason: "output has changed since Validation",
		},
		{
			name: "missing required sidecar",
			setup: func(t *testing.T, fixture publisherDeliveryFixture) {
				if err := os.MkdirAll(fixture.libraryRoot, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(fixture.sidecar); err != nil {
					t.Fatal(err)
				}
			},
			wantReason: "required subtitle artifacts are not ready",
		},
		{
			name: "occupied destination",
			setup: func(t *testing.T, fixture publisherDeliveryFixture) {
				if err := os.MkdirAll(filepath.Dir(fixture.destination), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture.destination, []byte("existing file"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantReason: "destination already exists",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPublisherDeliveryFixture(t, test.name == "missing required sidecar")
			if test.name != "missing required sidecar" {
				if err := os.MkdirAll(fixture.libraryRoot, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			test.setup(t, fixture)
			status, response := requestPublisherEligibility(t, fixture, "")
			if status != http.StatusOK || len(response.Items) != 1 {
				t.Fatalf("eligibility response status=%d response=%#v", status, response)
			}
			candidate := response.Items[0]
			if candidate.Status != "BLOCKED" || !strings.Contains(candidate.Reason, test.wantReason) {
				t.Fatalf("candidate did not report the actual blocking reason %q: %#v", test.wantReason, candidate)
			}
			var publications int64
			if err := fixture.db.Model(&models.PublisherPublication{}).Count(&publications).Error; err != nil || publications != 0 {
				t.Fatalf("blocked eligibility listing persisted publication identity: count=%d err=%v", publications, err)
			}
		})
	}
}

func publisherEligibilityReviewFixtureUpdate(t *testing.T, fixture publisherDeliveryFixture, outcome string, findings models.JSONList) {
	t.Helper()
	var run models.ValidationRun
	if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	run.Outcome = outcome
	run.Findings = findings
	if err := fixture.db.Save(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.QueueJob{}).Where("id = ?", fixture.job.ID).Updates(map[string]any{"validation_outcome": outcome, "validation_status": strings.ToLower(outcome)}).Error; err != nil {
		t.Fatal(err)
	}
}
