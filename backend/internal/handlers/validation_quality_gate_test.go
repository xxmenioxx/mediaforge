package handlers

import (
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

func TestResultArtifactIdentityBindsFrozenExecutionAndDetectsReplacement(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-result-identity", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	output := filepath.Join(root, "output.mkv")
	if err := os.WriteFile(output, []byte("first output generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	trackPlan, err := resolvedTrackPlanMap(ResolvedTrackPlan{AttachmentPolicy: AttachmentPolicyAuto})
	if err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(12)
	job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber, ProfileSnapshot: models.JSONMap{"frozen": true}, TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: trackPlan}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 3, Status: "frozen", SelectedEncoder: "hevc_qsv"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := resultIdentityFromArtifact(artifact)
	if !ok || identity.ArtifactID != "job-result:"+strconv.FormatUint(uint64(job.ID), 10)+":execution:12" || identity.GenerationVersion != 1 {
		t.Fatalf("canonical Result identity missing or malformed: %#v", artifact["resultIdentity"])
	}
	resultManifest := unknownRecord(artifact["artifacts"])
	primary := unknownRecord(resultManifest["primary"])
	if stringFromUnknown(primary["path"]) != output || stringFromUnknown(primary["status"]) != "ready" {
		t.Fatalf("Result manifest did not preserve the worker output as primary evidence: %#v", primary)
	}
	binding, ok := resultBindingFromArtifact(artifact)
	if !ok || binding.ExecutionNumber != 12 || binding.ExecutionPlanID != plan.ID || binding.ExecutionPlanVersion != plan.Version || binding.FrozenQueueFingerprint == "" {
		t.Fatalf("Result artifact is not bound to the frozen execution: %#v", artifact["executionBinding"])
	}
	currentRevision, err := resultOutputRevisionForPath(output)
	if err != nil || currentRevision != identity.OutputRevision {
		t.Fatalf("Result output revision=%q current=%q err=%v", identity.OutputRevision, currentRevision, err)
	}
	boundCandidate := bindPerceptualInputToResult(perceptualArtifactInput{CandidateKind: "final_output", SourceRevision: "frozen-source"}, identity)
	if boundCandidate.CandidateArtifactID != identity.ArtifactID || boundCandidate.GenerationToken != identity.GenerationToken || boundCandidate.GenerationVersion != int64(identity.GenerationVersion) || boundCandidate.CandidateRevision != identity.OutputRevision {
		t.Fatalf("same-Result CandidateIdentity is not bound to the canonical Result generation: %#v", boundCandidate)
	}
	job.ValidationRunID = "run-before-replacement"
	if err := db.Create(&models.ValidationRun{
		RunID: job.ValidationRunID, JobID: job.ID, ExecutionNumber: executionNumber,
		ExecutionPlanID: plan.ID, ExecutionPlanVersion: plan.Version, ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
		ResultArtifactID: identity.ArtifactID, ResultGenerationToken: identity.GenerationToken, ResultGenerationVersion: identity.GenerationVersion,
		ResultOutputRevision: identity.OutputRevision, SchemaVersion: "v1", ValidatorVersion: "v1", RulesetID: "test", RulesetVersion: "1",
		ActorSource: "deployment", ActorID: "local_operator", Outcome: "PASS", Findings: models.JSONList{}, Report: models.JSONMap{},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("replacement output generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if allowed, reason := canonicalRunAllowsPublish(db, job, false); allowed || reason == "" {
		t.Fatalf("replaced output was allowed to publish: allowed=%t reason=%q", allowed, reason)
	}
}

func TestResultManifestIntegrityRequiresFrozenArtifactsAndWorkerOutput(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "result.mkv")
	if err := os.WriteFile(output, []byte("output"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{StreamIndex: 4, Format: "srt", Mode: "original"}}}
	planSnapshot, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), OutputPath: output, PublishedPath: filepath.Join(root, "library", "stale.mkv"), TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planSnapshot}}
	planned := plannedSubtitleArtifacts(job)[0]
	planned.Status = "ready"
	job.SubtitleArtifacts = sidecarArtifactsJSON([]SubtitleArtifact{planned}, nil)

	manifest := jobResultArtifactOutputs(job)
	sidecars, expected, actual, matched := resultManifestEvidence(job, map[string]any{"artifacts": manifest})
	if !matched || len(sidecars) != 1 || stringFromUnknown(unknownRecord(actual["primary"])["path"]) != output {
		t.Fatalf("canonical Result manifest did not match frozen required output: sidecars=%#v expected=%#v actual=%#v matched=%t", sidecars, expected, actual, matched)
	}

	manifest["sidecars"] = models.JSONList{}
	_, _, _, matched = resultManifestEvidence(job, map[string]any{"artifacts": manifest})
	if matched {
		t.Fatal("Result manifest missing a frozen required sidecar was accepted")
	}

	manifest = jobResultArtifactOutputs(job)
	wrongPrimary := models.JSONMap{"type": "primary_media", "path": job.PublishedPath, "status": "ready"}
	manifest["primary"] = wrongPrimary
	_, _, _, matched = resultManifestEvidence(job, map[string]any{"artifacts": manifest})
	if matched {
		t.Fatal("published destination path was accepted as produced Result output")
	}
}

func TestMissingOutputStillProducesNonOverrideableValidationRun(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-missing-output", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(8)
	job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), Status: JobStatusCompleted, ExecutionNumber: &executionNumber, ProfileSnapshot: models.JSONMap{}, TrackProfileSnapshot: models.JSONMap{}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	identity, binding, err := resultIdentityForJob(db, job)
	if err != nil || identity == nil || identity.OutputRevision == "" || binding == nil {
		t.Fatalf("missing output did not retain an addressable Result generation: identity=%#v binding=%#v err=%v", identity, binding, err)
	}
	artifact := jobArtifact{
		GeneratedAt: time.Now().UTC(), Kind: "result", Job: job, ResultIdentity: identity,
		ExecutionBinding: binding, Result: map[string]any{"status": "completed"},
		OutputProbe: map[string]any{"error": "output is missing"}, Artifacts: jobResultArtifactOutputs(job),
	}
	if err := writeJobArtifact(db, job, "result", artifact); err != nil {
		t.Fatal(err)
	}
	gate, err := (ValidationHandler{db: db}).validateAndPersist(job, "local_operator")
	if err != nil {
		t.Fatal(err)
	}
	if gate.Run.Outcome != "FAIL" {
		t.Fatalf("missing output outcome=%s, want FAIL", gate.Run.Outcome)
	}
	for _, key := range []string{"artifact.identity", "artifact.integrity", "artifact.manifest"} {
		found := false
		for _, finding := range gate.Findings {
			if finding.Key == key {
				found = true
				if finding.State != "FAIL" || finding.Overrideable {
					t.Fatalf("identity/integrity finding %s state=%s overrideable=%t", key, finding.State, finding.Overrideable)
				}
			}
		}
		if !found {
			t.Fatalf("missing output omitted %s finding", key)
		}
	}
}

func TestValidationRuleCategoriesUseStableEvidenceFindings(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-rule-categories", &models.QueueJob{}, &models.ExecutionPlan{})
	job := models.QueueJob{ID: 1, MediaPath: "/media/source.mkv", OutputPath: "/media/result.mkv", ProfileSnapshot: models.JSONMap{}, TrackProfileSnapshot: models.JSONMap{}}
	findings, _ := evaluateValidationRules(db, job, map[string]any{"outputProbe": map[string]any{"error": "unavailable"}}, resultArtifactIdentity{
		ArtifactID: "job-result:1:execution:1", GenerationToken: "generation", GenerationVersion: 1, OutputRevision: "revision",
	}, resultExecutionBinding{})
	want := []string{
		"artifact.identity", "artifact.manifest", "artifact.integrity", "execution.binding",
		"duration.timeline", "video.properties", "audio.tracks_actions", "subtitles.streams_actions",
		"chapters.actions", "attachments.stream_actions", "attachments.font_artifacts",
		"sidecars.package", "publication.readiness",
	}
	byKey := map[string]validationFinding{}
	for _, finding := range findings {
		byKey[finding.Key] = finding
		encoded, err := json.Marshal(finding)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"key", "expected", "actual", "severity", "disposition", "state", "evidenceReferences"} {
			if _, exists := fields[field]; !exists {
				t.Fatalf("finding %s omits required field %s: %s", finding.Key, field, encoded)
			}
		}
	}
	for _, key := range want {
		finding, ok := byKey[key]
		if !ok || finding.Severity == "" || finding.Disposition == "" || len(finding.EvidenceReferences) == 0 {
			t.Fatalf("stable rule %s lacks complete evidence finding: %#v", key, finding)
		}
	}
	if byKey["sidecars.package"].State != "NOT_APPLICABLE" || byKey["duration.timeline"].State != "UNVERIFIED" {
		t.Fatalf("missing evidence was not kept explicit: sidecars=%s duration=%s", byKey["sidecars.package"].State, byKey["duration.timeline"].State)
	}
}

func TestValidationRetryAppendsImmutableRunsForSameResult(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-run-retry", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	output := filepath.Join(root, "output.mkv")
	if err := os.WriteFile(output, []byte("non-empty output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(4)
	job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber, ProfileSnapshot: models.JSONMap{}, TrackProfileSnapshot: models.JSONMap{}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER validation_runs_no_update BEFORE UPDATE ON validation_runs BEGIN SELECT RAISE(ABORT, 'validation runs are immutable'); END`).Error; err != nil {
		t.Fatal(err)
	}
	handler := ValidationHandler{db: db}
	first, err := handler.validateAndPersist(job, "system")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	second, err := handler.validateAndPersist(job, "system")
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.RunID == second.Run.RunID || first.Run.ResultGenerationToken != second.Run.ResultGenerationToken || first.Run.ResultOutputRevision != second.Run.ResultOutputRevision {
		t.Fatalf("retry changed Result identity or reused ValidationRun: first=%#v second=%#v", first.Run, second.Run)
	}
	var count int64
	if err := db.Model(&models.ValidationRun{}).Where("job_id = ?", job.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("runs=%d err=%v, want two append-only attempts", count, err)
	}
	if err := db.Model(&models.ValidationRun{}).Where("run_id = ?", first.Run.RunID).Update("outcome", "PASS").Error; err == nil {
		t.Fatal("immutable ValidationRun accepted an update")
	}
}

func TestWorkerAutomatedPipelineValidatesOnlyAfterResultArtifactExists(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-worker-order", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	output := filepath.Join(root, "worker-output.mkv")
	if err := os.WriteFile(output, []byte("worker result bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"autoValidationEnabled": true}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(30)
	job := models.QueueJob{MediaPath: filepath.Join(root, "worker-source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber}
	if err := db.Create(&models.AppSetting{Key: "assetReviewOverrides", Value: models.JSONMap{"entries": models.JSONMap{job.MediaPath: models.JSONMap{"requiresReview": true, "reason": "preflight review"}}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLatestJobArtifact(db, job, "result"); err != nil {
		t.Fatalf("Result artifact was not committed before worker Validation: %v", err)
	}
	result := (WorkerHandler{db: db}).runAutomatedPipeline(job)
	if result["validationRunId"] == nil || result["validationOutcome"] == nil {
		t.Fatalf("automated pipeline did not persist a canonical run after Result creation: %#v", result)
	}
	var run models.ValidationRun
	if err := db.Where("run_id = ?", result["validationRunId"]).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if run.ActorSource != "system" || run.ActorID != "system" {
		t.Fatalf("worker Validation actor=%s/%s want system/system", run.ActorSource, run.ActorID)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.ValidationRunID != run.RunID || job.Stage != JobStageValidationReview {
		t.Fatalf("worker did not project canonical run/review stage: run=%q expected=%q stage=%q outcome=%q", job.ValidationRunID, run.RunID, job.Stage, run.Outcome)
	}
}

func TestExplicitLegacyRevalidationExtendsExistingResultArtifact(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-legacy-bind", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	output := filepath.Join(root, "legacy-output.mkv")
	if err := os.WriteFile(output, []byte("legacy output bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	resultsDir := filepath.Join(root, "results")
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": resultsDir}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(21)
	job := models.QueueJob{MediaPath: filepath.Join(root, "legacy-source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	legacy := jobArtifact{
		GeneratedAt: time.Now().UTC(), Kind: "result", Job: job,
		Result:           map[string]any{"status": "completed"},
		OutputProbe:      map[string]any{"streams": []any{map[string]any{"codec_type": "video", "codec_name": "h264", "width": 640, "height": 360}}, "format": map[string]any{"duration": "12.0"}},
		ValidationReport: models.JSONMap{"colorPolicy": models.JSONMap{"status": "passed"}, "frameFidelity": models.JSONMap{"legacy": true}},
	}
	if err := writeJobArtifact(db, job, "result", legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := (ValidationHandler{db: db}).validateAndPersist(job, "local_operator"); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK || binding.ExecutionNumber != executionNumber || binding.ExecutionPlanID != plan.ID {
		t.Fatalf("explicit legacy revalidation did not extend the existing Result artifact: identity=%#v binding=%#v", identity, binding)
	}
	files, err := filepath.Glob(filepath.Join(resultsDir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("legacy bind created a parallel Result artifact: files=%v err=%v", files, err)
	}
	var jobAfter models.QueueJob
	if err := db.First(&jobAfter, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	var run models.ValidationRun
	if err := db.Where("run_id = ?", jobAfter.ValidationRunID).First(&run).Error; err != nil {
		t.Fatal(err)
	}
	if _, staleCopied := run.Report["frameFidelity"]; staleCopied {
		t.Fatal("legacy unbound frameFidelity evidence was relabeled as current evidence")
	}
}

func TestPublisherEligibilityUsesCanonicalRunNotQueueSummary(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-publisher-authority", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	output := filepath.Join(root, "publish-output.mkv")
	if err := os.WriteFile(output, []byte("publish output bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(42)
	job := models.QueueJob{MediaPath: filepath.Join(root, "publish-source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	job.ValidationStatus = ValidationStatusPassed
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, _ := canonicalRunAllowsPublish(db, job, false); allowed {
		t.Fatal("legacy QueueJob passed projection authorized publication without a canonical run")
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK {
		t.Fatalf("Result artifact identity unavailable: %#v", artifact)
	}
	run := models.ValidationRun{
		RunID: "validation-run:canonical-pass", JobID: job.ID, ExecutionNumber: executionNumber,
		ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
		ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint, ResultArtifactID: identity.ArtifactID,
		ResultGenerationToken: identity.GenerationToken, ResultGenerationVersion: identity.GenerationVersion,
		ResultOutputRevision: identity.OutputRevision, SchemaVersion: validationSchemaVersion,
		ValidatorVersion: validationEngineVersion, RulesetID: validationRulesetID, RulesetVersion: validationRulesetVersion,
		ActorSource: "system", ActorID: "system", Outcome: "PASS", Findings: models.JSONList{}, Report: models.JSONMap{},
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	job.ValidationRunID = run.RunID
	job.ValidationOutcome = "FAIL"
	job.ValidationStatus = ValidationStatusFailed
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, reason := canonicalRunAllowsPublish(db, job, false); !allowed {
		t.Fatalf("canonical PASS was overridden by read-only QueueJob projections: %s", reason)
	}
	if allowed, reason := canonicalRunAllowsPublish(db, job, true); !allowed {
		t.Fatalf("default clean PASS did not authorize automatic advancement: %s", reason)
	}
}

func TestAutomaticAdvancementRequiresAllowedCanonicalOutcome(t *testing.T) {
	tests := []struct {
		name         string
		outcome      string
		actionable   bool
		findingState string
		policy       []any
		unbound      bool
		wantAllowed  bool
	}{
		{name: "clean pass defaults allowed", outcome: "PASS", wantAllowed: true},
		{name: "warning needs policy", outcome: "PASS_WITH_WARNINGS", wantAllowed: false},
		{name: "configured non-actionable warning", outcome: "PASS_WITH_WARNINGS", policy: []any{"PASS", "PASS_WITH_WARNINGS"}, wantAllowed: true},
		{name: "actionable warning blocks even when configured", outcome: "PASS_WITH_WARNINGS", actionable: true, findingState: "WARNING", policy: []any{"PASS", "PASS_WITH_WARNINGS"}, wantAllowed: false},
		{name: "review required blocks", outcome: "REVIEW_REQUIRED", actionable: true, findingState: "UNVERIFIED", wantAllowed: false},
		{name: "failure blocks", outcome: "FAIL", actionable: true, findingState: "FAIL", wantAllowed: false},
		{name: "legacy outcome without run blocks", outcome: "PASS", unbound: true, wantAllowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := testEncodeTestDB(t, "quality-gate-auto-"+strings.ReplaceAll(test.name, " ", "-"), &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
			root := t.TempDir()
			output := filepath.Join(root, "auto-output.mkv")
			if err := os.WriteFile(output, []byte("same canonical Result"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
				t.Fatal(err)
			}
			if len(test.policy) > 0 {
				if err := db.Create(&models.AppSetting{Key: "validation", Value: models.JSONMap{"autoAdvanceOutcomes": test.policy}}).Error; err != nil {
					t.Fatal(err)
				}
			}
			executionNumber := uint(5)
			job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
			if err := db.Create(&plan).Error; err != nil {
				t.Fatal(err)
			}
			job.ActiveExecutionPlanID = &plan.ID
			if err := db.Save(&job).Error; err != nil {
				t.Fatal(err)
			}
			if test.unbound {
				job.ValidationStatus = ValidationStatusPassed
				if err := db.Save(&job).Error; err != nil {
					t.Fatal(err)
				}
				if allowed, _ := canonicalRunAllowsPublish(db, job, true); allowed != test.wantAllowed {
					t.Fatalf("unbound legacy outcome allowed=%t want=%t", allowed, test.wantAllowed)
				}
				return
			}
			if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
				t.Fatal(err)
			}
			artifact, _, err := readLatestJobArtifact(db, job, "result")
			if err != nil {
				t.Fatal(err)
			}
			identity, identityOK := resultIdentityFromArtifact(artifact)
			binding, bindingOK := resultBindingFromArtifact(artifact)
			if !identityOK || !bindingOK {
				t.Fatalf("Result identity missing: %#v", artifact)
			}
			run := models.ValidationRun{
				RunID: "validation-run:" + strings.ReplaceAll(test.name, " ", "-"), JobID: job.ID,
				ExecutionNumber: executionNumber, ExecutionPlanID: binding.ExecutionPlanID,
				ExecutionPlanVersion: binding.ExecutionPlanVersion, ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
				ResultArtifactID: identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
				ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
				SchemaVersion: validationSchemaVersion, ValidatorVersion: validationEngineVersion,
				RulesetID: validationRulesetID, RulesetVersion: validationRulesetVersion,
				ActorSource: "system", ActorID: "system", Outcome: test.outcome, ActionableWarnings: test.actionable,
				Findings: models.JSONList{}, Report: models.JSONMap{},
			}
			if test.findingState != "" {
				finding := models.JSONMap{"key": "duration.timeline", "severity": "warning", "disposition": "advisory", "state": test.findingState, "actionable": test.actionable, "overrideable": true}
				if test.findingState == "UNVERIFIED" || test.findingState == "FAIL" {
					finding["disposition"], finding["severity"] = "blocking", "error"
				}
				run.Findings = models.JSONList{finding}
			}
			if err := db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			job.ValidationRunID = run.RunID
			if err := db.Save(&job).Error; err != nil {
				t.Fatal(err)
			}
			allowed, reason := canonicalRunAllowsPublish(db, job, true)
			if allowed != test.wantAllowed {
				t.Fatalf("automatic handoff allowed=%t want=%t reason=%q", allowed, test.wantAllowed, reason)
			}
		})
	}
}

func TestValidationOutcomeReductionUsesRulesAndDispositionNotScore(t *testing.T) {
	tests := []struct {
		name     string
		findings []validationFinding
		want     string
	}{
		{name: "clean pass", findings: []validationFinding{{Disposition: "blocking", State: "PASS"}, {Disposition: "blocking", State: "NOT_APPLICABLE"}}, want: "PASS"},
		{name: "advisory warning", findings: []validationFinding{{Disposition: "advisory", State: "WARNING", Actionable: true}}, want: "PASS_WITH_WARNINGS"},
		{name: "unresolved blocking evidence", findings: []validationFinding{{Disposition: "blocking", State: "UNVERIFIED"}}, want: "REVIEW_REQUIRED"},
		{name: "known blocking violation wins", findings: []validationFinding{{Disposition: "blocking", State: "UNVERIFIED"}, {Disposition: "blocking", State: "FAIL"}}, want: "FAIL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, _ := reduceValidationOutcome(test.findings)
			if outcome != test.want {
				t.Fatalf("outcome=%q want=%q", outcome, test.want)
			}
		})
	}
}

func TestOverrideUsesServerActorRequiresReasonAndCannotOverrideIdentity(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-override-actor", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	output := filepath.Join(root, "output.mkv")
	if err := os.WriteFile(output, []byte("non-empty output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": filepath.Join(root, "results")}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(8)
	job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), OutputPath: output, Status: JobStatusCompleted, ExecutionNumber: &executionNumber, ProfileSnapshot: models.JSONMap{}, TrackProfileSnapshot: models.JSONMap{}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: "frozen"}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	gate, err := (ValidationHandler{db: db}).validateAndPersist(job, "system")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gate.Run.ActorSource != "system" || gate.Run.ActorID != "system" {
		t.Fatalf("automatic validation actor is not system: %q/%q", gate.Run.ActorSource, gate.Run.ActorID)
	}
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.POST("/validation/jobs/:id/overrides", NewValidationHandler(db).CreateOverride)
	requestBody := `{"reason":"Accept this measured timeline exception","ruleKeys":["duration.timeline"],"requestedHandoff":"publisher","actorId":"client_admin"}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/overrides", strings.NewReader(requestBody)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("override status=%d body=%s findings=%#v", recorder.Code, recorder.Body.String(), gate.Findings)
	}
	var event models.ValidationOverride
	if err := json.Unmarshal(recorder.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.ActorSource != "deployment" || event.ActorID != "local_operator" || event.Reason == "" || len(event.RuleKeys) != 1 || event.OverrideVersion != validationOverrideV1 || event.CreatedAt.IsZero() {
		t.Fatalf("override audit event is incomplete or trusted client actor: %#v", event)
	}
	var unchanged models.ValidationRun
	if err := db.Where("run_id = ?", gate.Run.RunID).First(&unchanged).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Outcome != gate.Run.Outcome {
		t.Fatalf("override mutated immutable outcome: %q -> %q", gate.Run.Outcome, unchanged.Outcome)
	}
	badBody := `{"reason":"try identity override","ruleKeys":["artifact.identity"],"requestedHandoff":"publisher"}`
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/overrides", strings.NewReader(badBody)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("identity failure override status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReviewAndHistoryListsReturnBoundedSummaries(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-review-history-summaries", &models.QueueJob{}, &models.ValidationRun{}, &models.ValidationOverride{})
	jobs := []models.QueueJob{
		{MediaPath: "/raw/fail.mkv", OutputPath: "/stage/fail.mkv", Status: JobStatusCompleted, ValidationStatus: ValidationStatusFailed, ValidationOutcome: "FAIL", ValidationRunID: "run-fail", ValidationReport: models.JSONMap{"large": strings.Repeat("x", 2048)}},
		{MediaPath: "/raw/actionable.mkv", OutputPath: "/stage/actionable.mkv", Status: JobStatusCompleted, ValidationStatus: ValidationStatusWarning, ValidationOutcome: "PASS_WITH_WARNINGS", ValidationRunID: "run-warning", ValidationActionable: true},
		{MediaPath: "/raw/clean.mkv", OutputPath: "/stage/clean.mkv", Status: JobStatusCompleted, ValidationStatus: ValidationStatusPassed, ValidationOutcome: "PASS", ValidationRunID: "run-pass"},
		{MediaPath: "/raw/published.mkv", OutputPath: "/stage/published.mkv", Status: JobStatusCompleted, ValidationStatus: ValidationStatusPassed, ValidationOutcome: "PASS", ValidationRunID: "run-published", PublishedAt: timePointer(time.Now().UTC())},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	handler := NewValidationHandler(db)
	router.GET("/validation/review", handler.ReviewList)
	router.GET("/validation/history", handler.HistoryList)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/validation/review?limit=1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("Review Center list status=%d body=%s", response.Code, response.Body.String())
	}
	var review map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	items, _ := review["items"].([]any)
	if len(items) != 1 || review["total"] != float64(2) {
		t.Fatalf("exceptions query did not return bounded exception summaries: %#v", review)
	}
	if _, exists := items[0].(map[string]any)["validationReport"]; exists {
		t.Fatal("Review Center list returned full ValidationReport evidence")
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/validation/history?limit=100", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("History list status=%d body=%s", response.Code, response.Body.String())
	}
	var history map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	historyItems, _ := history["items"].([]any)
	if history["total"] != float64(4) || len(historyItems) != 4 {
		t.Fatalf("History omitted completed or published work: %#v", history)
	}
	for _, raw := range historyItems {
		if _, exists := raw.(map[string]any)["validationReport"]; exists {
			t.Fatal("History list returned full ValidationReport evidence")
		}
	}
}

func timePointer(value time.Time) *time.Time { return &value }
