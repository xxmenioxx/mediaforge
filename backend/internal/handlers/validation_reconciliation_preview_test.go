package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestReconciliationPreviewNeedsIndependentPerAssetIntentAndDoesNotMutate(t *testing.T) {
	fixture := newReconciliationPreviewFixture(t, "FAIL")
	before := fixture.job
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.GET("/validation/jobs/:id/reconciliation-preview", NewValidationHandler(fixture.db).ReconciliationPreview)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/validation/jobs/"+stringID(fixture.job.ID)+"/reconciliation-preview", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response historicalReconciliationPreview
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "NEEDS_EVIDENCE" || response.ExpectedContract.Status != "NEEDS_EVIDENCE" || len(response.ExpectedContract.MissingEvidence) == 0 {
		t.Fatalf("preview inferred an expectation without immutable per-asset intent: %#v", response)
	}
	if len(response.AffectedFindings) != 2 || response.AffectedFindings[0].ProposedExpected != nil || response.AffectedFindings[1].ProposedExpected != nil {
		t.Fatalf("track/sidecar findings should retain originals and have no guessed replacement: %#v", response.AffectedFindings)
	}
	if len(response.UnchangedFindings) != 1 || response.UnchangedFindings[0].Key != "video.properties" {
		t.Fatalf("non-track video finding must remain outside reconciliation scope: %#v", response.UnchangedFindings)
	}
	if response.Binding.QueueJobID != fixture.job.ID || response.Binding.ResultArtifactID == "" || response.Binding.ExecutionPlanFingerprint == "" {
		t.Fatalf("preview did not return the exact identity binding: %#v", response.Binding)
	}
	var after models.QueueJob
	if err := fixture.db.First(&after, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only preview changed QueueJob:\nbefore=%#v\nafter=%#v", before, after)
	}
	var runCount int64
	if err := fixture.db.Model(&models.ValidationRun{}).Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("read-only preview created or removed ValidationRuns: %d", runCount)
	}
}

func TestReconciliationPreviewBlocksReplacedOutputRevision(t *testing.T) {
	fixture := newReconciliationPreviewFixture(t, "FAIL")
	if err := os.WriteFile(fixture.output, []byte("replaced result bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := buildHistoricalReconciliationPreview(fixture.db, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "BLOCKED" || !previewReasonContains(preview, "output bytes do not match") {
		t.Fatalf("replaced Result output was not blocked: %#v", preview)
	}
}

func TestReconciliationPreviewBlocksChangedSidecarIdentity(t *testing.T) {
	fixture := newReconciliationPreviewFixture(t, "FAIL")
	fixture.job.SubtitleArtifacts = models.JSONList{models.JSONMap{"artifactId": "different-sidecar", "status": "ready"}}
	if err := fixture.db.Save(&fixture.job).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := buildHistoricalReconciliationPreview(fixture.db, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "BLOCKED" || !previewReasonContains(preview, "sidecar identity evidence differs") {
		t.Fatalf("stale sidecar identity was not blocked: %#v", preview)
	}
}

func TestReconciliationPreviewBlocksNormalUnaffectedJob(t *testing.T) {
	fixture := newReconciliationPreviewFixture(t, "PASS")
	preview, err := buildHistoricalReconciliationPreview(fixture.db, fixture.job)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != "BLOCKED" || len(preview.AffectedFindings) != 0 {
		t.Fatalf("normal unaffected job entered reconciliation candidate set: %#v", preview)
	}
}

func TestReconciliationPreviewKeepsAllKnownTrackPatternsNeedsEvidence(t *testing.T) {
	cases := []struct {
		name      string
		findings  models.JSONList
		wantKeys  int
		wantVideo bool
	}{
		{
			name: "audio count mismatch",
			findings: models.JSONList{
				models.JSONMap{"key": "audio.tracks_actions", "state": "FAIL", "expected": 3, "actual": 2, "evidenceReferences": []any{"queue.trackSnapshot", "result.outputProbe"}},
			},
			wantKeys: 1,
		},
		{
			name: "audio subtitle and manifest mismatch",
			findings: models.JSONList{
				models.JSONMap{"key": "audio.tracks_actions", "state": "FAIL", "expected": 3, "actual": 2},
				models.JSONMap{"key": "subtitles.streams_actions", "state": "FAIL", "expected": 1, "actual": 0},
				models.JSONMap{"key": "artifact.manifest", "state": "FAIL", "expected": []any{"sidecar-1"}, "actual": []any{}},
			},
			wantKeys: 3,
		},
		{
			name: "subtitle sidecar mismatch with video fps unknown",
			findings: models.JSONList{
				models.JSONMap{"key": "subtitles.streams_actions", "state": "FAIL", "expected": 1, "actual": 0},
				models.JSONMap{"key": "artifact.manifest", "state": "FAIL", "expected": []any{"sidecar-1"}, "actual": []any{}},
				models.JSONMap{"key": "video.properties", "state": "UNVERIFIED", "expected": "effective output frame rate", "actual": nil},
			},
			wantKeys:  2,
			wantVideo: true,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReconciliationPreviewFixture(t, "FAIL")
			var run models.ValidationRun
			if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&run).Error; err != nil {
				t.Fatal(err)
			}
			run.Findings = test.findings
			if err := fixture.db.Save(&run).Error; err != nil {
				t.Fatal(err)
			}
			preview, err := buildHistoricalReconciliationPreview(fixture.db, fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Status != "NEEDS_EVIDENCE" || preview.ExpectedContract.Status != "NEEDS_EVIDENCE" || len(preview.AffectedFindings) != test.wantKeys {
				t.Fatalf("known historical pattern was not kept fail-closed: %#v", preview)
			}
			for _, finding := range preview.AffectedFindings {
				if finding.ProposedExpected != nil {
					t.Fatalf("a proposed expectation was inferred from the output for %s: %#v", finding.Key, finding)
				}
			}
			if got := len(preview.UnchangedFindings) > 0; got != test.wantVideo {
				t.Fatalf("video.properties classification=%t, want %t: %#v", got, test.wantVideo, preview.UnchangedFindings)
			}
		})
	}
}

func TestResolveHistoricalExpectedContractDoesNotUseOutputOrQueueTrackSnapshotAsAuthority(t *testing.T) {
	fixture := newReconciliationPreviewFixture(t, "FAIL")
	var plan models.ExecutionPlan
	if err := fixture.db.First(&plan, *fixture.job.ActiveExecutionPlanID).Error; err != nil {
		t.Fatal(err)
	}
	withOneAudio := map[string]any{"outputProbe": map[string]any{"streams": []any{map[string]any{"codec_type": "audio"}}}}
	withManyAudio := map[string]any{"outputProbe": map[string]any{"streams": []any{map[string]any{"codec_type": "audio"}, map[string]any{"codec_type": "audio"}, map[string]any{"codec_type": "audio"}}}}
	first := resolveHistoricalExpectedContract(fixture.job, plan, withOneAudio)
	second := resolveHistoricalExpectedContract(fixture.job, plan, withManyAudio)
	if first.Status != "NEEDS_EVIDENCE" || !reflect.DeepEqual(first, second) {
		t.Fatalf("expected-contract resolver used output counts or Queue snapshot as corrected authority: first=%#v second=%#v", first, second)
	}
	if !strings.Contains(first.Reason, "different asset path") {
		t.Fatalf("crossed snapshot evidence was not reported as a mismatch: %#v", first)
	}
}

type reconciliationPreviewFixture struct {
	db     *gorm.DB
	job    models.QueueJob
	output string
}

func newReconciliationPreviewFixture(t *testing.T, outcome string) reconciliationPreviewFixture {
	t.Helper()
	root := t.TempDir()
	db := testEncodeTestDB(t, t.Name(), &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.AppSetting{})
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "output.mkv")
	if err := os.WriteFile(output, []byte("synthetic output revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(4)
	runID := "preview-run-1"
	job := models.QueueJob{
		MediaPath: filepath.Join(root, "asset.mkv"), OutputPath: output, Status: JobStatusCompleted,
		Stage: JobStageValidationReview, ExecutionNumber: &executionNumber,
		ProfileSnapshot: models.JSONMap{"profileVersion": 1},
		TrackProfileKey: "tracks.default",
		TrackProfileSnapshot: models.JSONMap{"resolvedTrackPlan": models.JSONMap{
			"resolvedForAsset": filepath.Join(root, "other-asset.mkv"), "audioActions": []any{models.JSONMap{"action": "keep", "streamIndex": 1}},
		}},
		ProfileResolution: models.JSONMap{"tracks": models.JSONMap{"source": "global", "profileData": models.JSONMap{"audio": "keep"}}},
		SubtitleArtifacts: models.JSONList{models.JSONMap{"artifactId": "sidecar-1", "status": "ready"}},
		ValidationStatus:  ValidationStatusFailed, ValidationOutcome: outcome, ValidationRunID: runID,
	}
	if outcome == "PASS" {
		job.ValidationStatus = ValidationStatusPassed
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 2, Status: "frozen", Constraints: models.JSONMap{"mode": "synthetic"}}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	identity, binding, err := resultIdentityForJob(db, job)
	if err != nil || identity == nil || binding == nil {
		t.Fatalf("fixture Result identity: identity=%#v binding=%#v err=%v", identity, binding, err)
	}
	artifact := jobArtifact{
		GeneratedAt: time.Now().UTC(), Kind: "result", Job: job, ResultIdentity: identity, ExecutionBinding: binding,
		OutputProbe: map[string]any{"streams": []any{map[string]any{"codec_type": "audio"}, map[string]any{"codec_type": "subtitle"}}},
		Artifacts:   models.JSONMap{"primary": models.JSONMap{"type": "primary_media", "path": output, "status": "ready"}, "sidecars": job.SubtitleArtifacts},
	}
	if err := writeJobArtifact(db, job, "result", artifact); err != nil {
		t.Fatal(err)
	}
	findings := models.JSONList{
		models.JSONMap{"key": "audio.tracks_actions", "state": "FAIL", "expected": "persisted audio plan", "actual": 2, "evidenceReferences": []any{"queue.trackSnapshot", "result.outputProbe"}, "explanation": "Original track comparison."},
		models.JSONMap{"key": "artifact.manifest", "state": "FAIL", "expected": []any{"sidecar-1"}, "actual": []any{}, "evidenceReferences": []any{"result.artifactManifest", "queue.trackSnapshot"}, "explanation": "Original sidecar comparison."},
		models.JSONMap{"key": "video.properties", "state": "UNVERIFIED", "expected": "effective frame rate", "actual": nil, "evidenceReferences": []any{"result.outputProbe"}, "explanation": "Unrelated video evidence remains unresolved."},
	}
	if err := db.Create(&models.ValidationRun{
		RunID: runID, JobID: job.ID, ExecutionNumber: binding.ExecutionNumber,
		ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
		ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
		ResultArtifactID:         identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
		ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
		SchemaVersion: "test", ValidatorVersion: "test", RulesetID: "test", RulesetVersion: "test",
		ActorSource: "test", ActorID: "test", Outcome: outcome, Findings: findings,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	return reconciliationPreviewFixture{db: db, job: job, output: output}
}

func previewReasonContains(preview historicalReconciliationPreview, substring string) bool {
	for _, reason := range preview.Reasons {
		if strings.Contains(reason, substring) {
			return true
		}
	}
	return false
}

func stringID(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
