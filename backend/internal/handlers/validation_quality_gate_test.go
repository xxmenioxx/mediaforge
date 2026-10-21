package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/validationpolicy"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestValidationProbeDurationParsesSupportedValues(t *testing.T) {
	tests := []struct {
		name  string
		probe map[string]any
		want  float64
	}{
		{name: "ffprobe numeric string", probe: map[string]any{"format": map[string]any{"duration": "7415.449000"}}, want: 7415.449},
		{name: "float64", probe: map[string]any{"format": map[string]any{"duration": float64(12.5)}}, want: 12.5},
		{name: "float32", probe: map[string]any{"format": map[string]any{"duration": float32(12.5)}}, want: 12.5},
		{name: "int", probe: map[string]any{"format": map[string]any{"duration": int(12)}}, want: 12},
		{name: "int64", probe: map[string]any{"format": map[string]any{"duration": int64(12)}}, want: 12},
		{name: "json number", probe: map[string]any{"format": map[string]any{"duration": json.Number("12.5")}}, want: 12.5},
		{name: "invalid string", probe: map[string]any{"format": map[string]any{"duration": "invalid"}}},
		{name: "not available string", probe: map[string]any{"format": map[string]any{"duration": "N/A"}}},
		{name: "empty string", probe: map[string]any{"format": map[string]any{"duration": ""}}},
		{name: "nil duration", probe: map[string]any{"format": map[string]any{"duration": nil}}},
		{name: "NaN", probe: map[string]any{"format": map[string]any{"duration": math.NaN()}}},
		{name: "positive infinity", probe: map[string]any{"format": map[string]any{"duration": math.Inf(1)}}},
		{name: "zero", probe: map[string]any{"format": map[string]any{"duration": 0}}},
		{name: "negative", probe: map[string]any{"format": map[string]any{"duration": -1}}},
		{name: "missing duration", probe: map[string]any{"format": map[string]any{}}},
		{name: "missing format", probe: map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validationProbeDuration(test.probe); got != test.want {
				t.Fatalf("validationProbeDuration()=%v, want %v", got, test.want)
			}
		})
	}
}

func TestValidationDurationTimelineUsesStringProbeEvidenceAndExistingTolerance(t *testing.T) {
	tests := []struct {
		name           string
		sourceDuration any
		outputDuration any
		maxPercent     float64
		wantState      string
	}{
		{name: "job 2148 durations within tolerance", sourceDuration: "7415.449000", outputDuration: "7415.455000", wantState: "PASS"},
		{name: "duration mismatch outside tolerance", sourceDuration: "7415.449000", outputDuration: "7600.000000", wantState: "FAIL"},
		{name: "custom duration tolerance is consumed", sourceDuration: "100.000000", outputDuration: "104.000000", maxPercent: 5, wantState: "PASS"},
		{name: "unavailable duration evidence", sourceDuration: "7415.449000", outputDuration: "N/A", wantState: "UNVERIFIED"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := testEncodeTestDB(t, "quality-gate-duration-strings-"+strconv.Itoa(index), &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
			root := t.TempDir()
			if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{
				"resultsReportsPath": filepath.Join(root, "results"),
				"asIsReportsPath":    filepath.Join(root, "as-is"),
			}}).Error; err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(root, "output.mkv")
			if err := os.WriteFile(outputPath, []byte("disposable validation fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			executionNumber := uint(12)
			job := models.QueueJob{MediaPath: filepath.Join(root, "source.mkv"), OutputPath: outputPath, Status: JobStatusCompleted, ExecutionNumber: &executionNumber, ProfileSnapshot: models.JSONMap{}, TrackProfileSnapshot: models.JSONMap{}}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			if err := writeJobArtifact(db, job, "as-is", jobArtifact{
				Job:         job,
				SourceProbe: map[string]any{"format": map[string]any{"duration": test.sourceDuration}},
			}); err != nil {
				t.Fatal(err)
			}

			artifact := map[string]any{
				"job":         map[string]any{"executionNumber": executionNumber},
				"outputProbe": map[string]any{"format": map[string]any{"duration": test.outputDuration}},
			}
			var selectedPolicy []validationpolicy.Policy
			if test.maxPercent > 0 {
				policy := validationpolicy.DefaultPolicy()
				rule := policy.Rules["duration.timeline"]
				rule.Thresholds["maxPercent"] = test.maxPercent
				policy.Rules["duration.timeline"] = rule
				policy.Preset, policy.BasedOn = validationpolicy.Custom, validationpolicy.Default
				selectedPolicy = append(selectedPolicy, policy)
			}
			findings, _ := evaluateValidationRules(db, job, artifact, resultArtifactIdentity{}, resultExecutionBinding{ExecutionNumber: executionNumber}, selectedPolicy...)
			for _, finding := range findings {
				if finding.Key == "duration.timeline" {
					if finding.State != test.wantState {
						t.Fatalf("duration.timeline state=%s, want %s (expected=%v actual=%v)", finding.State, test.wantState, finding.Expected, finding.Actual)
					}
					return
				}
			}
			t.Fatal("duration.timeline finding is missing")
		})
	}
}

func TestQualityGateVideoPropertiesUsesCanonicalHEVCBitstreamEvidence(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"hevcLevel": "4.0", "hevcTier": "main",
		"hevcLevelEffective": "4.0", "hevcLevelTier": "main",
	}}
	outputProbe := models.JSONMap{"streams": []any{models.JSONMap{
		"codec_type": "video", "codec_name": "hevc", "level": 120, "tier": "high",
		"width": 1920, "height": 1080, "avg_frame_rate": "24/1", "field_order": "progressive",
		"pix_fmt": "yuv420p", "bits_per_raw_sample": "8",
	}}}
	video := firstStreamByType(outputProbe, "video")
	evidence := testHEVCBitstreamEvidence("4.0", "main")

	db := testEncodeTestDB(t, "quality-gate-hevc-video-properties-pass", &models.QueueJob{}, &models.ExecutionPlan{})
	expected, actual, state := evaluateFrozenVideoProperties(db, models.QueueJob{}, resultExecutionBinding{}, profile, video, outputProbe, evidence)
	if state != "PASS" || actual["level"] != "4.0" || actual["tier"] != "main" {
		t.Fatalf("matching frozen HEVC bitstream did not pass: state=%s expected=%#v actual=%#v", state, expected, actual)
	}

	canonical := evaluateCanonicalFidelity(fidelityAuthority{name: "queue_profile_snapshot", profile: profile, worker: map[string]interface{}(profile.WorkerConfig)}, nil, outputProbe, evidence)
	canonicalFields := unknownRecord(canonical["fields"])
	for _, name := range []string{"hevcLevel", "hevcTier"} {
		field := unknownRecord(canonicalFields[name])
		actualKey := map[string]string{"hevcLevel": "level", "hevcTier": "tier"}[name]
		if field["actual"] != actual[actualKey] || field["status"] != "passed" {
			t.Fatalf("video.properties and canonical Frame Fidelity disagree for %s: property=%#v canonical=%#v", name, actual[actualKey], field)
		}
	}

	t.Run("level mismatch fails", func(t *testing.T) {
		db := testEncodeTestDB(t, "quality-gate-hevc-video-properties-level-mismatch", &models.QueueJob{}, &models.ExecutionPlan{})
		_, actual, state := evaluateFrozenVideoProperties(db, models.QueueJob{}, resultExecutionBinding{}, profile, video, outputProbe, testHEVCBitstreamEvidence("4.1", "main"))
		if state != "FAIL" || actual["level"] != "4.1" {
			t.Fatalf("HEVC Level mismatch was not blocking: state=%s actual=%#v", state, actual)
		}
	})

	t.Run("tier mismatch fails", func(t *testing.T) {
		db := testEncodeTestDB(t, "quality-gate-hevc-video-properties-tier-mismatch", &models.QueueJob{}, &models.ExecutionPlan{})
		_, actual, state := evaluateFrozenVideoProperties(db, models.QueueJob{}, resultExecutionBinding{}, profile, video, outputProbe, testHEVCBitstreamEvidence("4.0", "high"))
		if state != "FAIL" || actual["tier"] != "high" {
			t.Fatalf("HEVC Tier mismatch was not blocking: state=%s actual=%#v", state, actual)
		}
	})

	t.Run("unavailable evidence stays unverified", func(t *testing.T) {
		db := testEncodeTestDB(t, "quality-gate-hevc-video-properties-unavailable", &models.QueueJob{}, &models.ExecutionPlan{})
		unavailable := models.JSONMap{
			"levelEvidenceAvailable": false, "tierEvidenceAvailable": false,
			"generalLevel": "4.0", "generalTier": "main",
		}
		_, actual, state := evaluateFrozenVideoProperties(db, models.QueueJob{}, resultExecutionBinding{}, profile, video, outputProbe, unavailable)
		if state != "UNVERIFIED" || actual["level"] != "" || actual["tier"] != "" {
			t.Fatalf("unavailable HEVC bitstream evidence was inferred: state=%s actual=%#v", state, actual)
		}
	})

	t.Run("generic ffprobe level is not HEVC authority", func(t *testing.T) {
		db := testEncodeTestDB(t, "quality-gate-hevc-video-properties-probe-level", &models.QueueJob{}, &models.ExecutionPlan{})
		_, actual, state := evaluateFrozenVideoProperties(db, models.QueueJob{}, resultExecutionBinding{}, profile, video, outputProbe, nil)
		if state != "UNVERIFIED" || actual["level"] != "" {
			t.Fatalf("generic ffprobe level=120 was treated as HEVC Level evidence: state=%s actual=%#v", state, actual)
		}
	})

	t.Run("non-HEVC codec ignores HEVC fields", func(t *testing.T) {
		db := testEncodeTestDB(t, "quality-gate-hevc-video-properties-non-hevc", &models.QueueJob{}, &models.ExecutionPlan{})
		nonHEVC := models.Profile{VideoCodec: "h264", WorkerConfig: profile.WorkerConfig}
		nonHEVCProbe := models.JSONMap{"streams": []any{models.JSONMap{
			"codec_type": "video", "codec_name": "h264", "level": 120, "tier": "high",
		}}}
		_, actual, state := evaluateFrozenVideoProperties(db, models.QueueJob{}, resultExecutionBinding{}, nonHEVC, firstStreamByType(nonHEVCProbe, "video"), nonHEVCProbe, nil)
		if state != "PASS" || actual["level"] != "" || actual["tier"] != "" {
			t.Fatalf("non-HEVC video properties were affected by HEVC fields: state=%s actual=%#v", state, actual)
		}
	})
}

func TestQualityGateVideoPropertiesPersistsCanonicalFieldOutcomes(t *testing.T) {
	worker := models.JSONMap{
		"pixFmt": "p010le", "profile": "main10", "videoEncoder": "hevc_qsv", "hevcLevelEffective": "4.0", "hevcLevelTier": "main",
		"effectiveOutputFrameRate": "30000/1001", "effectiveCadenceOperation": "preserve",
		"effectiveOutputProgressive": true, "effectiveOutputWidth": 1920, "effectiveOutputHeight": 1080,
		"effectiveOutputSAR": "1:1", "effectiveOutputDAR": "16:9",
		"finalColorPolicy": "preserve", "effectiveFinalColorPolicy": "preserve",
	}
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: worker}
	sourceProbe, outputProbe := main10VideoProbeWithoutColorMetadata()
	state, actual := evaluateQualityGateVideoPropertiesFixture(t, profile, sourceProbe, outputProbe, testHEVCBitstreamEvidence("4.0", "main"))
	fields := unknownRecord(actual["fields"])
	if state != "PASS" || fields == nil {
		t.Fatalf("verified Main10 properties with optional absent color metadata should pass: state=%s actual=%#v", state, actual)
	}
	for _, name := range []string{"codec", "width", "height", "frameRate", "fieldStructure", "pixelFormat", "bitDepth", "hevcLevel", "hevcTier"} {
		if field := unknownRecord(fields[name]); field == nil || field["status"] != "passed" {
			t.Fatalf("expected canonical field %s to pass: %#v", name, fields[name])
		}
	}
	color := unknownRecord(fields["colorPolicy"])
	if color == nil || color["status"] != "not_applicable" || color["required"] != false || !strings.Contains(stringFromUnknown(color["reason"]), "No reliable source color metadata") {
		t.Fatalf("optional color metadata did not retain actionable not-applicable evidence: %#v", color)
	}
	if aggregateVideoPropertiesValidationState(fields) != state {
		t.Fatalf("top-level state=%s disagrees with persisted canonical fields=%#v", state, fields)
	}

	t.Run("explicit color contract remains unverified when absent", func(t *testing.T) {
		requiredWorker := cloneJSONMap(worker)
		requiredWorker["finalColorPolicy"] = "normalize_bt709"
		requiredWorker["effectiveFinalColorPolicy"] = "normalize_bt709"
		requiredProfile := models.Profile{VideoCodec: "hevc", WorkerConfig: requiredWorker}
		state, actual := evaluateQualityGateVideoPropertiesFixture(t, requiredProfile, sourceProbe, outputProbe, testHEVCBitstreamEvidence("4.0", "main"))
		fields := unknownRecord(actual["fields"])
		color := unknownRecord(fields["colorPolicy"])
		if state != "UNVERIFIED" || color["status"] != "unverified" || color["required"] != true {
			t.Fatalf("missing required color evidence must block: state=%s fields=%#v", state, fields)
		}
		if aggregateVideoPropertiesValidationState(fields) != state {
			t.Fatalf("top-level state=%s disagrees with persisted canonical fields=%#v", state, fields)
		}
	})

	t.Run("explicit color contract stays fail closed without source probe", func(t *testing.T) {
		requiredWorker := cloneJSONMap(worker)
		requiredWorker["finalColorPolicy"] = "normalize_bt709"
		requiredWorker["effectiveFinalColorPolicy"] = "normalize_bt709"
		requiredProfile := models.Profile{VideoCodec: "hevc", WorkerConfig: requiredWorker}
		state, actual := evaluateQualityGateVideoPropertiesFixture(t, requiredProfile, nil, outputProbe, testHEVCBitstreamEvidence("4.0", "main"))
		fields := unknownRecord(actual["fields"])
		color := unknownRecord(fields["colorPolicy"])
		if state != "UNVERIFIED" || color["status"] != "unverified" || color["required"] != true || !strings.Contains(stringFromUnknown(color["reason"]), "source-probe evidence is unavailable") {
			t.Fatalf("missing frozen source evidence must not erase an explicit color contract: state=%s fields=%#v", state, fields)
		}
	})

	t.Run("explicit color mismatch fails", func(t *testing.T) {
		requiredWorker := cloneJSONMap(worker)
		requiredWorker["finalColorPolicy"] = "normalize_bt709"
		requiredWorker["effectiveFinalColorPolicy"] = "normalize_bt709"
		requiredProfile := models.Profile{VideoCodec: "hevc", WorkerConfig: requiredWorker}
		mismatchedOutput := main10ColorProbe("bt2020nc", "smpte2084", "bt2020", "tv")
		state, actual := evaluateQualityGateVideoPropertiesFixture(t, requiredProfile, sourceProbe, mismatchedOutput, testHEVCBitstreamEvidence("4.0", "main"))
		fields := unknownRecord(actual["fields"])
		color := unknownRecord(fields["colorPolicy"])
		if state != "FAIL" || color["status"] != "mismatch" || color["required"] != true {
			t.Fatalf("frozen color mismatch must fail: state=%s fields=%#v", state, fields)
		}
		if aggregateVideoPropertiesValidationState(fields) != state {
			t.Fatalf("top-level state=%s disagrees with persisted canonical fields=%#v", state, fields)
		}
	})
}

func TestQualityGatePixelSemanticPolicyUsesCanonicalPixelFormatField(t *testing.T) {
	worker := models.JSONMap{
		"pixFmt": "p010le", "profile": "main10", "videoEncoder": "hevc_qsv", "hevcLevelEffective": "4.0", "hevcLevelTier": "main",
		"effectiveOutputFrameRate": "30000/1001", "effectiveCadenceOperation": "preserve",
		"effectiveOutputProgressive": true, "effectiveOutputWidth": 1920, "effectiveOutputHeight": 1080,
		"effectiveOutputSAR": "1:1", "effectiveOutputDAR": "16:9",
		"finalColorPolicy": "preserve", "effectiveFinalColorPolicy": "preserve",
	}
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: worker}
	sourceProbe, outputProbe := main10VideoProbeWithoutColorMetadata()
	firstStreamByType(outputProbe, "video")["pix_fmt"] = "yuv422p10le"

	defaultRule := validationpolicy.DefaultPolicy().Rules["video.properties"]
	if !defaultRule.Fields["pixelFormat"] {
		t.Fatal("Default policy must enable canonical pixelFormat semantics")
	}
	defaultState, defaultActual := evaluateQualityGateVideoPropertiesFixture(t, profile, sourceProbe, outputProbe, testHEVCBitstreamEvidence("4.0", "main"), defaultRule.Fields)
	defaultFields := unknownRecord(defaultActual["fields"])
	pixel := unknownRecord(defaultFields["pixelFormat"])
	bitDepth := unknownRecord(defaultFields["bitDepth"])
	if defaultState != "FAIL" || pixel["policyEnabled"] != true || pixel["status"] != "mismatch" {
		t.Fatalf("Default did not include the chroma semantic mismatch in the composite result: state=%s pixel=%#v", defaultState, pixel)
	}
	if bitDepth["policyEnabled"] != true || bitDepth["status"] != "passed" {
		t.Fatalf("pixel semantic mismatch should leave independently passing bit-depth evidence enabled: %#v", bitDepth)
	}

	t.Run("custom disables only pixel semantics", func(t *testing.T) {
		custom := validationpolicy.Normalize(map[string]any{
			"preset": validationpolicy.Custom, "basedOn": validationpolicy.Default,
			"rules": map[string]any{"video.properties": map[string]any{"fields": map[string]any{"pixelFormat": false, "bitDepth": true}}},
		})
		customRule := custom.Rules["video.properties"]
		if custom.Preset != validationpolicy.Custom || custom.BasedOn != validationpolicy.Default || customRule.Fields["pixelFormat"] || !customRule.Fields["bitDepth"] {
			t.Fatalf("Custom pixelFormat edit did not preserve independent bitDepth policy: %#v", customRule)
		}
		customState, customActual := evaluateQualityGateVideoPropertiesFixture(t, profile, sourceProbe, outputProbe, testHEVCBitstreamEvidence("4.0", "main"), customRule.Fields)
		customFields := unknownRecord(customActual["fields"])
		pixel := unknownRecord(customFields["pixelFormat"])
		bitDepth := unknownRecord(customFields["bitDepth"])
		if pixel["policyEnabled"] != false || pixel["status"] != "mismatch" {
			t.Fatalf("disabled pixelFormat must remain persisted as mismatch evidence: %#v", pixel)
		}
		if bitDepth["policyEnabled"] != true || bitDepth["status"] != "passed" || customState != "PASS" {
			t.Fatalf("disabled pixelFormat still affected composite state or disabled bitDepth: state=%s bitDepth=%#v", customState, bitDepth)
		}
	})
}

func evaluateQualityGateVideoPropertiesFixture(t *testing.T, profile models.Profile, sourceProbe, outputProbe models.JSONMap, hevcEvidence models.JSONMap, fieldPolicies ...map[string]bool) (string, models.JSONMap) {
	t.Helper()
	root := t.TempDir()
	dbName := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	db := testEncodeTestDB(t, "quality-gate-video-fields-"+dbName, &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{
		"resultsReportsPath": filepath.Join(root, "results"), "asIsReportsPath": filepath.Join(root, "as-is"),
	}}).Error; err != nil {
		t.Fatal(err)
	}
	executionNumber := uint(1)
	job := models.QueueJob{
		MediaPath: filepath.Join(root, "synthetic-source.mkv"), OutputPath: filepath.Join(root, "synthetic-result.mkv"),
		Status: JobStatusCompleted, ExecutionNumber: &executionNumber,
		ProfileSnapshot: models.JSONMap{"workerConfig": profile.WorkerConfig},
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobArtifact(db, job, "as-is", jobArtifact{Job: job, Profile: profile, SourceProbe: sourceProbe}); err != nil {
		t.Fatalf("write synthetic frozen source evidence: %v", err)
	}
	video := firstStreamByType(outputProbe, "video")
	_, actual, state := evaluateFrozenVideoProperties(db, job, resultExecutionBinding{ExecutionNumber: executionNumber}, profile, video, outputProbe, hevcEvidence, fieldPolicies...)
	return state, actual
}

func main10VideoProbeWithoutColorMetadata() (models.JSONMap, models.JSONMap) {
	sourceStream := map[string]any{
		"codec_type": "video", "codec_name": "hevc", "profile": "Main 10", "width": 1920, "height": 1080,
		"avg_frame_rate": "30000/1001", "field_order": "progressive", "sample_aspect_ratio": "1:1",
		"display_aspect_ratio": "16:9", "pix_fmt": "yuv420p10le", "chroma_location": "left",
	}
	outputStream := make(map[string]any, len(sourceStream))
	for key, value := range sourceStream {
		outputStream[key] = value
	}
	return models.JSONMap{"streams": []any{sourceStream}}, models.JSONMap{"streams": []any{outputStream}}
}

func main10ColorProbe(matrix, transfer, primaries, colorRange string) models.JSONMap {
	_, output := main10VideoProbeWithoutColorMetadata()
	stream := firstStreamByType(output, "video")
	stream["color_space"], stream["color_transfer"], stream["color_primaries"], stream["color_range"] = matrix, transfer, primaries, colorRange
	return output
}

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
	if !strings.HasPrefix(identity.OutputRevision, "result-output-revision-v2-") || identity.OutputRevisionEvidence == nil || identity.OutputRevisionEvidence.ContentRevision != identity.OutputRevision || identity.OutputRevisionEvidence.SizeBytes <= 0 || identity.OutputRevisionEvidence.ContentSHA256 == "" || identity.OutputRevisionEvidence.BoundedFingerprint == "" {
		t.Fatalf("Result content revision evidence was not persisted: %#v", artifact["resultIdentity"])
	}
	identityWithoutDiagnostics := map[string]any{}
	for key, value := range unknownRecord(artifact["resultIdentity"]) {
		identityWithoutDiagnostics[key] = value
	}
	delete(identityWithoutDiagnostics, "outputRevisionEvidence")
	if _, complete := resultIdentityFromArtifact(map[string]any{"resultIdentity": identityWithoutDiagnostics}); complete {
		t.Fatal("v2 Result identity without its persisted content evidence was accepted")
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
	if err := os.Chmod(output, 0o640); err != nil {
		t.Fatal(err)
	}
	validationFindings, _ := evaluateValidationRules(db, job, artifact, identity, binding)
	identityPass := false
	for _, finding := range validationFindings {
		if finding.Key == "artifact.identity" {
			identityPass = finding.State == "PASS" && !finding.Overrideable
			if !identityPass {
				t.Fatalf("chmod-only Result metadata change failed or became overrideable: %#v", finding)
			}
			expected, expectedOK := finding.Expected.(*resultOutputRevisionEvidence)
			actual, actualOK := finding.Actual.(*resultOutputRevisionEvidence)
			if !expectedOK || !actualOK || expected.ContentRevision != actual.ContentRevision || expected.Filesystem.Mode == actual.Filesystem.Mode {
				t.Fatalf("Validation did not preserve content evidence while reporting the chmod: expected=%#v actual=%#v", finding.Expected, finding.Actual)
			}
		}
	}
	if !identityPass {
		t.Fatal("Validation omitted the canonical artifact.identity finding")
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
	var canonicalRun models.ValidationRun
	if err := db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&canonicalRun).Error; err != nil || !runMatchesResult(canonicalRun, artifact) {
		t.Fatalf("canonical ValidationRun did not match its Result generation: run=%#v err=%v", canonicalRun, err)
	}
	staleGenerationToken := canonicalRun
	staleGenerationToken.ResultGenerationToken += "-stale"
	if runMatchesResult(staleGenerationToken, artifact) {
		t.Fatal("Result generation token mismatch was accepted")
	}
	staleGenerationVersion := canonicalRun
	staleGenerationVersion.ResultGenerationVersion++
	if runMatchesResult(staleGenerationVersion, artifact) {
		t.Fatal("Result generation version mismatch was accepted")
	}
	if err := os.WriteFile(output, []byte("replacement output generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedFindings, _ := evaluateValidationRules(db, job, artifact, identity, binding)
	identityFailed := false
	for _, finding := range changedFindings {
		if finding.Key == "artifact.identity" {
			identityFailed = finding.State == "FAIL" && !finding.Overrideable
			if !identityFailed {
				t.Fatalf("changed Result content did not remain blocking and non-overrideable: %#v", finding)
			}
		}
	}
	if !identityFailed {
		t.Fatal("Validation omitted artifact.identity after Result content changed")
	}
	if allowed, reason := canonicalRunAllowsPublish(db, job, false); allowed || reason == "" {
		t.Fatalf("replaced output was allowed to publish: allowed=%t reason=%q", allowed, reason)
	}
}

func TestResultOutputRevisionV2IgnoresFilesystemMetadata(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output.mkv")
	content := []byte(strings.Repeat("stable media content ", 256))
	if err := os.WriteFile(output, content, 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := resultOutputRevisionEvidenceForPath(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(original.ContentRevision, "result-output-revision-v2-") {
		t.Fatalf("new Result output revision has no v2 prefix: %q", original.ContentRevision)
	}

	metadataOnly := *original
	metadataOnly.Filesystem.ModifiedAtUnixNano++
	metadataOnly.Filesystem.Mode ^= 0o111
	metadataOnly.Filesystem.Device = "different-device"
	metadataOnly.Filesystem.Inode = "different-inode"
	metadataRevision, err := resultOutputRevisionV2ForEvidence(&metadataOnly)
	if err != nil || metadataRevision != original.ContentRevision {
		t.Fatalf("filesystem diagnostics changed the v2 content revision: original=%q metadata=%q err=%v", original.ContentRevision, metadataRevision, err)
	}

	if err := os.Chmod(output, 0o640); err != nil {
		t.Fatal(err)
	}
	changedTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(output, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	chmodEvidence, err := resultOutputRevisionEvidenceForPath(output)
	if err != nil {
		t.Fatal(err)
	}
	if chmodEvidence.ContentRevision != original.ContentRevision || chmodEvidence.Filesystem.Mode == original.Filesystem.Mode || chmodEvidence.Filesystem.ModifiedAtUnixNano == original.Filesystem.ModifiedAtUnixNano {
		t.Fatalf("chmod/mtime-only change was not isolated from content identity: original=%#v after=%#v", original, chmodEvidence)
	}

	replacement := filepath.Join(root, "replacement.mkv")
	if err := os.WriteFile(replacement, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, output); err != nil {
		t.Fatal(err)
	}
	replacementEvidence, err := resultOutputRevisionEvidenceForPath(output)
	if err != nil {
		t.Fatal(err)
	}
	if replacementEvidence.ContentRevision != original.ContentRevision {
		t.Fatalf("same-content inode replacement changed the v2 revision: original=%q replacement=%q", original.ContentRevision, replacementEvidence.ContentRevision)
	}
	if original.Filesystem.Inode != "" && replacementEvidence.Filesystem.Inode != "" && original.Filesystem.Inode == replacementEvidence.Filesystem.Inode {
		t.Fatalf("test replacement did not change inode: %#v", replacementEvidence.Filesystem)
	}
	currentEvidence, err := resultOutputRevisionForIdentity(output, original.ContentRevision)
	if err != nil || currentEvidence.ContentRevision != original.ContentRevision {
		t.Fatalf("same content with changed filesystem identity did not match: evidence=%#v err=%v", currentEvidence, err)
	}
}

func TestResultOutputRevisionV2RejectsChangedContentTruncationAndMissingOutput(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output.mkv")
	content := []byte(strings.Repeat("bounded bytes", 256))
	if err := os.WriteFile(output, content, 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := resultOutputRevisionEvidenceForPath(output)
	if err != nil {
		t.Fatal(err)
	}

	mutated := append([]byte(nil), content...)
	mutated[0] ^= 0xff
	if err := os.WriteFile(output, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := resultOutputRevisionForIdentity(output, original.ContentRevision)
	if err != nil || changed.ContentRevision == original.ContentRevision {
		t.Fatalf("same-size content mutation retained the Result revision: evidence=%#v err=%v", changed, err)
	}

	if err := os.WriteFile(output, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	truncated, err := resultOutputRevisionForIdentity(output, original.ContentRevision)
	if err != nil || truncated.ContentRevision == original.ContentRevision || truncated.SizeBytes == original.SizeBytes {
		t.Fatalf("truncation retained the Result revision: evidence=%#v err=%v", truncated, err)
	}

	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if _, err := resultOutputRevisionForIdentity(output, original.ContentRevision); err == nil {
		t.Fatal("missing Result output was accepted")
	}
}

func TestResultOutputRevisionDetectsContentChangesOutsideBoundedSamples(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "large-output.mkv")
	content := make([]byte, 8<<20)
	for index := range content {
		content[index] = byte(index % 251)
	}
	if err := os.WriteFile(output, content, 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := resultOutputRevisionEvidenceForPath(output)
	if err != nil {
		t.Fatal(err)
	}
	content[2<<20] ^= 0xff // outside mediaFileFingerprint's first, middle, and final samples
	if err := os.WriteFile(output, content, 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := resultOutputRevisionForIdentity(output, original.ContentRevision)
	if err != nil {
		t.Fatal(err)
	}
	if changed.BoundedFingerprint != original.BoundedFingerprint {
		t.Fatalf("test mutation unexpectedly changed the bounded sample: before=%q after=%q", original.BoundedFingerprint, changed.BoundedFingerprint)
	}
	if changed.ContentSHA256 == original.ContentSHA256 || changed.ContentRevision == original.ContentRevision {
		t.Fatalf("unsampled byte mutation retained the canonical content identity: before=%#v after=%#v", original, changed)
	}
}

func TestLegacyResultOutputRevisionV1KeepsMetadataSensitiveSemantics(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "legacy-output.mkv")
	if err := os.WriteFile(output, []byte("legacy result content"), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := resultOutputRevisionEvidenceForPath(output)
	if err != nil {
		t.Fatal(err)
	}
	legacyRevision, err := resultOutputRevisionV1ForEvidence(evidence)
	if err != nil || !strings.HasPrefix(legacyRevision, "result-output-revision-v1-") {
		t.Fatalf("legacy revision fixture was not v1: revision=%q err=%v", legacyRevision, err)
	}
	legacyCurrent, err := resultOutputRevisionForIdentity(output, legacyRevision)
	if err != nil || legacyCurrent.ContentRevision != legacyRevision {
		t.Fatalf("unchanged v1 Result did not retain its original revision semantics: current=%#v err=%v", legacyCurrent, err)
	}
	if err := os.Chmod(output, 0o640); err != nil {
		t.Fatal(err)
	}
	legacyAfterChmod, err := resultOutputRevisionForIdentity(output, legacyRevision)
	if err != nil || legacyAfterChmod.ContentRevision == legacyRevision {
		t.Fatalf("legacy v1 Result was silently upgraded to metadata-independent semantics: current=%#v err=%v", legacyAfterChmod, err)
	}
	newRevision, err := resultOutputRevisionForPath(output)
	if err != nil || newRevision == legacyRevision || !strings.HasPrefix(newRevision, "result-output-revision-v2-") {
		t.Fatalf("new v2 semantics were not versioned separately from v1: revision=%q err=%v", newRevision, err)
	}
}

func TestResultOutputRevisionRejectsUnreadableOutput(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "unreadable.mkv")
	if err := os.WriteFile(output, []byte("unreadable output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(output, 0); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err == nil {
		_ = file.Close()
		t.Skip("current test process can read mode-zero files")
	}
	if _, err := resultOutputRevisionEvidenceForPath(output); err == nil {
		t.Fatal("unreadable Result output was accepted")
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

func TestQualityGateAVAlignmentPersistsDetailedFindingAdditively(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-av-alignment-details", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	root := t.TempDir()
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{
		"resultsReportsPath": filepath.Join(root, "results"),
		"asIsReportsPath":    filepath.Join(root, "as-is"),
	}}).Error; err != nil {
		t.Fatal(err)
	}
	sourcePath, outputPath := filepath.Join(root, "source.mkv"), filepath.Join(root, "output.mkv")
	for _, path := range []string{sourcePath, outputPath} {
		if err := os.WriteFile(path, []byte("disposable validation fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	executionNumber := uint(12)
	job := models.QueueJob{
		MediaPath: sourcePath, OutputPath: outputPath, Status: JobStatusCompleted,
		ExecutionNumber: &executionNumber, ProfileSnapshot: models.JSONMap{}, TrackProfileSnapshot: models.JSONMap{},
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	streamPlan := ResolvedStreamPlan{Audio: []PlannedStream{{InputIndex: 1, OutputIndex: 1, OutputTypeIndex: 0, Action: "copy"}}}
	probe := func(videoStart, videoFrame string) map[string]any {
		return map[string]any{
			"format": map[string]any{"duration": "100"},
			"streams": []any{
				models.JSONMap{"index": 0, "codec_type": "video", "codec_name": "h264", "start_time": videoStart, "time_base": "1/1000", "avg_frame_rate": "24000/1001"},
				models.JSONMap{"index": 1, "codec_type": "audio", "codec_name": "aac", "start_time": "0.000000", "time_base": "1/1000"},
			},
			"packets": []any{models.JSONMap{"stream_index": 1, "pts_time": "0.000000", "dts_time": "0.000000"}},
			"frames":  []any{models.JSONMap{"media_type": "video", "stream_index": 0, "best_effort_timestamp_time": videoFrame}},
		}
	}
	sourceProbe := probe("0.000000", "0.042000")
	outputProbe := probe("0.042000", "0.042000")
	if err := writeJobArtifact(db, job, "as-is", jobArtifact{Job: job, SourceProbe: sourceProbe, StreamPlan: streamPlan}); err != nil {
		t.Fatal(err)
	}
	artifact := map[string]any{"job": map[string]any{"executionNumber": executionNumber}, "outputProbe": outputProbe}
	findings, _ := evaluateValidationRules(db, job, artifact, resultArtifactIdentity{}, resultExecutionBinding{ExecutionNumber: executionNumber})
	var alignment *validationFinding
	for index := range findings {
		if findings[index].Key == "timeline.av_alignment" {
			alignment = &findings[index]
			break
		}
	}
	if alignment == nil || alignment.State != "PASS" {
		t.Fatalf("presentation-aligned source and output did not persist a PASS finding: %#v", alignment)
	}
	if _, ok := alignment.Expected.(avTimingEvidence); !ok {
		t.Fatalf("expected source evidence shape changed: %T", alignment.Expected)
	}
	if _, ok := alignment.Actual.(avTimingEvidence); !ok {
		t.Fatalf("actual output evidence shape changed: %T", alignment.Actual)
	}
	details, ok := alignment.Details.(models.JSONMap)
	if !ok {
		t.Fatalf("A/V alignment finding omitted optional details: %T", alignment.Details)
	}
	tracks, ok := details["tracks"].([]models.JSONMap)
	if !ok || len(tracks) != 1 || tracks[0]["decision"] != "validated" || math.Abs(tracks[0]["introducedOffsetMs"].(float64)) > .001 {
		t.Fatalf("persisted per-track comparison details are incomplete: %#v", details)
	}
	encoded, err := json.Marshal(*alignment)
	if err != nil {
		t.Fatal(err)
	}
	var persisted validationFinding
	if err := json.Unmarshal(encoded, &persisted); err != nil || persisted.Details == nil || persisted.Key != "timeline.av_alignment" {
		t.Fatalf("finding details did not survive persistence encoding: finding=%#v err=%v", persisted, err)
	}
	var historical validationFinding
	if err := json.Unmarshal([]byte(`{"key":"timeline.av_alignment","expected":{"frameRate":"24000/1001"},"actual":{"frameRate":"24000/1001"}}`), &historical); err != nil || historical.Details != nil {
		t.Fatalf("historical finding without optional details is no longer readable: finding=%#v err=%v", historical, err)
	}
}

func TestAddChapterFindingUsesExplicitOutputEvidence(t *testing.T) {
	tests := []struct {
		name          string
		chaptersKept  bool
		chapterPolicy ChapterPolicy
		probe         models.JSONMap
		wantState     string
		wantActual    any
	}{
		{name: "keep with output chapters", chaptersKept: true, probe: models.JSONMap{"chapters": []any{models.JSONMap{"id": 0}}}, wantState: "PASS", wantActual: 1},
		{name: "keep with real chapter loss", chaptersKept: true, probe: models.JSONMap{"chapters": []any{}}, wantState: "FAIL", wantActual: 0},
		{name: "remove with no output chapters", chapterPolicy: ChapterPolicyRemove, probe: models.JSONMap{"chapters": []any{}}, wantState: "PASS", wantActual: 0},
		{name: "missing chapter evidence", chaptersKept: true, probe: models.JSONMap{"streams": []any{}}, wantState: "UNVERIFIED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trackPlan, err := resolvedTrackPlanMap(ResolvedTrackPlan{ChaptersKept: test.chaptersKept, ChapterPolicy: test.chapterPolicy})
			if err != nil {
				t.Fatal(err)
			}
			job := models.QueueJob{TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: trackPlan}}
			var got validationFinding
			add := func(key string, expected, actual any, severity, disposition, state string, actionable, overrideable bool, explanation string, refs ...string) {
				if key == "chapters.actions" {
					got = validationFinding{Key: key, Expected: expected, Actual: actual, State: state}
				}
			}
			addChapterFinding(add, job, test.probe)
			if got.Key == "" || got.State != test.wantState || got.Actual != test.wantActual {
				t.Fatalf("chapter finding=%#v, want state=%s actual=%#v", got, test.wantState, test.wantActual)
			}
		})
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
	if first.Run.PolicyPreset != validationpolicy.Default || first.Run.PolicyFingerprint == "" || first.Run.PolicySnapshot["policyVersion"] != validationpolicy.PolicyVersion {
		t.Fatalf("ValidationRun did not freeze its default policy: %#v", first.Run)
	}
	oldOverride := models.ValidationOverride{
		EventID: "validation-override:retry-old-run", ValidationRunID: first.Run.RunID, Scope: "rule",
		ResultArtifactID: first.Run.ResultArtifactID, ResultGenerationToken: first.Run.ResultGenerationToken,
		ResultGenerationVersion: first.Run.ResultGenerationVersion, ResultOutputRevision: first.Run.ResultOutputRevision,
		PolicySchemaVersion: first.Run.PolicySchemaVersion, PolicyVersion: first.Run.PolicyVersion, PolicyFingerprint: first.Run.PolicyFingerprint,
		ActorSource: "deployment", ActorID: "local_operator", RuleKeys: models.JSONList{"duration.timeline"},
		RequestedHandoff: "publisher", OverrideVersion: validationOverrideV1, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&oldOverride).Error; err != nil {
		t.Fatal(err)
	}
	changedPolicy := validationpolicy.DefaultPolicy()
	durationPolicy := changedPolicy.Rules["duration.timeline"]
	durationPolicy.Thresholds["maxPercent"] = 3
	changedPolicy.Rules["duration.timeline"] = durationPolicy
	changedPolicy.Preset, changedPolicy.BasedOn = validationpolicy.Custom, validationpolicy.Default
	if err := db.Create(&models.AppSetting{Key: "validation", Value: validationPolicyJSONMap(changedPolicy)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	second, err := handler.validateAndPersist(job, "system")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.Run.RunID == second.Run.RunID || first.Run.ResultGenerationToken != second.Run.ResultGenerationToken || first.Run.ResultOutputRevision != second.Run.ResultOutputRevision {
		t.Fatalf("retry changed Result identity or reused ValidationRun: first=%#v second=%#v", first.Run, second.Run)
	}
	if second.Run.PolicyPreset != validationpolicy.Custom || second.Run.PolicyFingerprint == first.Run.PolicyFingerprint {
		t.Fatalf("explicit revalidation did not freeze the new policy: first=%#v second=%#v", first.Run, second.Run)
	}
	var unchangedFirst models.ValidationRun
	if err := db.Where("run_id = ?", first.Run.RunID).First(&unchangedFirst).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedFirst.PolicyFingerprint != first.Run.PolicyFingerprint || unchangedFirst.PolicySnapshot["preset"] != validationpolicy.Default {
		t.Fatalf("settings change rewrote historical ValidationRun policy: %#v", unchangedFirst)
	}
	if allowed, reason := canonicalRunAllowsPublish(db, job, false); allowed || reason == "" {
		t.Fatalf("override from the previous ValidationRun authorized revalidation: allowed=%t reason=%q", allowed, reason)
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
	result, err := (WorkerHandler{db: db}).runAutomatedPipeline(job)
	if err != nil {
		t.Fatalf("automatic Validation pipeline failed: %v", err)
	}
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

func TestWorkerAutomatedPipelineStopsWhenValidatingStageCannotPersist(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-worker-stage-failure", &models.QueueJob{}, &models.AppSetting{}, &models.ValidationRun{}, &models.ExecutionPlan{}, &models.ValidationOverride{})
	if err := db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"autoValidationEnabled": true}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageConverting}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_validating_stage BEFORE UPDATE OF stage ON queue_jobs WHEN NEW.stage = 'validating' BEGIN SELECT RAISE(ABORT, 'validating stage write rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	_, err := (WorkerHandler{db: db}).runAutomatedPipeline(job)
	if err == nil || !strings.Contains(err.Error(), "persist validating") {
		t.Fatalf("validating persistence failure was not propagated: %v", err)
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != JobStageConverting {
		t.Fatalf("failed validating transition changed stage to %q", persisted.Stage)
	}
	var runs int64
	if err := db.Model(&models.ValidationRun{}).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("Validation ran after its stage write failed: %d runs", runs)
	}
}

func TestManualValidationPropagatesFallbackReviewStageFailure(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-manual-review-stage-failure", &models.QueueJob{}, &models.AppSetting{}, &models.ValidationRun{}, &models.ExecutionPlan{}, &models.ValidationOverride{})
	job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageCompleted}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_validation_review BEFORE UPDATE OF stage ON queue_jobs WHEN NEW.stage = 'validation_review' BEGIN SELECT RAISE(ABORT, 'review stage write rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/api/validation/jobs/:id", NewValidationHandler(db).ValidateJob)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10), nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "failed to persist validation_review stage") {
		t.Fatalf("fallback stage persistence cause was lost: %s", recorder.Body.String())
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != JobStageValidating {
		t.Fatalf("failed fallback stage changed persisted stage to %q", persisted.Stage)
	}
}

func TestLegacyResultRemainsImmutableAndRequiresCanonicalExecution(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-legacy-immutable", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
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
	legacyReport := models.JSONMap{"legacyStatus": "passed", "frameFidelity": models.JSONMap{"legacy": true}}
	job := models.QueueJob{
		MediaPath: filepath.Join(root, "legacy-source.mkv"), OutputPath: output, Status: JobStatusCompleted,
		ExecutionNumber: &executionNumber, ValidationStatus: ValidationStatusPassed,
		ValidationScore: 100, ValidationReport: legacyReport,
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
	legacy := jobArtifact{
		GeneratedAt: time.Now().UTC(), Kind: "result", Job: job,
		Result:           map[string]any{"status": "completed"},
		OutputProbe:      map[string]any{"streams": []any{map[string]any{"codec_type": "video", "codec_name": "h264", "width": 640, "height": 360}}, "format": map[string]any{"duration": "12.0"}},
		ValidationReport: models.JSONMap{"colorPolicy": models.JSONMap{"status": "passed"}, "frameFidelity": models.JSONMap{"legacy": true}},
	}
	if err := writeJobArtifact(db, job, "result", legacy); err != nil {
		t.Fatal(err)
	}
	legacyArtifact, legacyPath, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	legacyBytes, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resultIdentityFromArtifact(legacyArtifact); ok {
		t.Fatal("legacy Result unexpectedly has canonical identity before validation")
	}
	if _, ok := resultBindingFromArtifact(legacyArtifact); ok {
		t.Fatal("legacy Result unexpectedly has an execution binding before validation")
	}
	if _, err := (ValidationHandler{db: db}).validateAndPersist(job, "local_operator"); err == nil || !strings.Contains(err.Error(), "historical or unbound") {
		t.Fatalf("unbound historical Result validation err=%v, want fail-closed requeue error", err)
	}
	legacyBytesAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacyBytesAfter) != string(legacyBytes) {
		t.Fatal("validation changed the historical Result artifact bytes")
	}
	legacyAfter, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resultIdentityFromArtifact(legacyAfter); ok {
		t.Fatal("historical Result acquired Validation-minted identity")
	}
	if _, ok := resultBindingFromArtifact(legacyAfter); ok {
		t.Fatal("historical Result acquired a Validation-minted execution binding")
	}
	var jobAfter models.QueueJob
	if err := db.First(&jobAfter, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if jobAfter.ValidationStatus != ValidationStatusPassed || jobAfter.ValidationScore != 100 || jobAfter.ValidationReport["legacyStatus"] != "passed" || jobAfter.ValidationRunID != "" {
		t.Fatalf("legacy QueueJob validation history was not preserved as unbound: %#v", jobAfter)
	}
	if frame := unknownRecord(jobAfter.ValidationReport["frameFidelity"]); frame["legacy"] != true {
		t.Fatalf("legacy QueueJob frameFidelity history was not preserved: %#v", frame)
	}
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	reviewRouter := gin.New()
	reviewRouter.GET("/validation/review", NewValidationHandler(db).ReviewList)
	reviewResponse := httptest.NewRecorder()
	reviewRouter.ServeHTTP(reviewResponse, httptest.NewRequest(http.MethodGet, "/validation/review", nil))
	if reviewResponse.Code != http.StatusOK {
		t.Fatalf("legacy review list status=%d body=%s", reviewResponse.Code, reviewResponse.Body.String())
	}
	var reviewPage struct {
		Items []validationSummaryRow `json:"items"`
	}
	if err := json.Unmarshal(reviewResponse.Body.Bytes(), &reviewPage); err != nil {
		t.Fatal(err)
	}
	legacyVisible := false
	for _, item := range reviewPage.Items {
		if item.ID == job.ID && item.ValidationStatus == ValidationStatusPassed && item.ValidationRunID == "" {
			legacyVisible = true
		}
	}
	if !legacyVisible {
		t.Fatalf("legacy QueueJob status is not readable as unbound history: %#v", reviewPage.Items)
	}
	if allowed, reason := canonicalRunAllowsPublish(db, jobAfter, false); allowed || reason == "" {
		t.Fatal("legacy QueueJob status authorized Publisher without a canonical run")
	}
	if _, err := (PublisherHandler{db: db}).publishQueueJobWithMode(jobAfter, false, false); err == nil {
		t.Fatal("Publisher accepted the legacy QueueJob status without a canonical run")
	}
	if allowed, reason := canonicalRunAllowsPublish(db, jobAfter, true); allowed || reason == "" {
		t.Fatal("legacy QueueJob status authorized automatic publication without a canonical run")
	}

	newExecutionNumber := executionNumber + 1
	newPlan := models.ExecutionPlan{JobID: job.ID, Version: plan.Version + 1, Status: "frozen"}
	if err := db.Create(&newPlan).Error; err != nil {
		t.Fatal(err)
	}
	jobAfter.ExecutionNumber = &newExecutionNumber
	jobAfter.ActiveExecutionPlanID = &newPlan.ID
	if err := db.Save(&jobAfter).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, jobAfter, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	canonicalArtifact, canonicalPath, err := readLatestJobArtifact(db, jobAfter, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, identityOK := resultIdentityFromArtifact(canonicalArtifact)
	binding, bindingOK := resultBindingFromArtifact(canonicalArtifact)
	if !identityOK || !bindingOK || binding.ExecutionNumber != newExecutionNumber || binding.ExecutionPlanID != newPlan.ID || canonicalPath == legacyPath {
		t.Fatalf("new execution did not create a distinct canonical Result: identity=%#v binding=%#v path=%q", identity, binding, canonicalPath)
	}
	gate, err := (ValidationHandler{db: db}).validateAndPersist(jobAfter, "local_operator")
	if err != nil || gate.Run.RunID == "" || gate.Run.ResultGenerationToken != identity.GenerationToken || gate.Run.ResultOutputRevision != identity.OutputRevision {
		t.Fatalf("new canonical Result did not validate normally: run=%#v err=%v", gate.Run, err)
	}
	if _, staleCopied := gate.Run.Report["frameFidelity"]; staleCopied {
		t.Fatal("legacy unbound frameFidelity evidence was relabeled as current evidence")
	}
	legacyBytesFinal, err := os.ReadFile(legacyPath)
	if err != nil || string(legacyBytesFinal) != string(legacyBytes) {
		t.Fatalf("historical Result changed after canonical re-execution: err=%v", err)
	}
	files, err := filepath.Glob(filepath.Join(resultsDir, "*.json"))
	if err != nil || len(files) != 2 {
		t.Fatalf("Result history files=%v err=%v; want legacy and canonical artifacts in the existing store", files, err)
	}
}

func TestResultArtifactWriteNeverOverwritesExistingGeneration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "result.json")
	if err := writeImmutableResultArtifact(path, []byte("historical Result")); err != nil {
		t.Fatal(err)
	}
	if err := writeImmutableResultArtifact(path, []byte("new canonical Result")); err != nil {
		t.Fatal(err)
	}
	historical, err := os.ReadFile(path)
	if err != nil || string(historical) != "historical Result" {
		t.Fatalf("historical Result changed: content=%q err=%v", historical, err)
	}
	canonical, err := os.ReadFile(filepath.Join(root, "result-1.json"))
	if err != nil || string(canonical) != "new canonical Result" {
		t.Fatalf("new Result was not appended to the existing artifact store: content=%q err=%v", canonical, err)
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
		name               string
		outcome            string
		actionable         bool
		findingState       string
		findingDisposition string
		policy             []any
		unbound            bool
		wantAllowed        bool
		wantManualAllowed  bool
	}{
		{name: "clean pass defaults allowed", outcome: "PASS", wantAllowed: true, wantManualAllowed: true},
		{name: "warning needs policy", outcome: "PASS_WITH_WARNINGS", findingState: "WARNING", wantManualAllowed: true},
		{name: "empty policy defaults to pass only", outcome: "PASS_WITH_WARNINGS", findingState: "WARNING", policy: []any{}, wantManualAllowed: true},
		{name: "configured non-actionable advisory warning", outcome: "PASS_WITH_WARNINGS", findingState: "WARNING", policy: []any{"PASS", "PASS_WITH_WARNINGS"}, wantAllowed: true, wantManualAllowed: true},
		{name: "configured actionable advisory warning", outcome: "PASS_WITH_WARNINGS", actionable: true, findingState: "WARNING", policy: []any{"PASS", "PASS_WITH_WARNINGS"}, wantAllowed: true, wantManualAllowed: true},
		{name: "configured policy with blocking warning stays blocked", outcome: "REVIEW_REQUIRED", findingState: "WARNING", findingDisposition: "blocking", policy: []any{"PASS", "PASS_WITH_WARNINGS", "REVIEW_REQUIRED"}},
		{name: "critical unverified evidence blocks", outcome: "REVIEW_REQUIRED", actionable: true, findingState: "UNVERIFIED", findingDisposition: "blocking"},
		{name: "advisory unverified evidence is a warning", outcome: "PASS_WITH_WARNINGS", findingState: "UNVERIFIED", policy: []any{"PASS", "PASS_WITH_WARNINGS"}, wantAllowed: true, wantManualAllowed: true},
		{name: "failure blocks", outcome: "FAIL", actionable: true, findingState: "FAIL", findingDisposition: "blocking"},
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
			if test.policy != nil {
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
				if allowed, _ := canonicalRunAllowsPublish(db, job, false); allowed {
					t.Fatal("unbound legacy Validation outcome authorized manual Publisher")
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
				disposition := test.findingDisposition
				if disposition == "" {
					disposition = "advisory"
				}
				severity := "warning"
				if disposition == "blocking" {
					severity = "error"
				}
				finding := models.JSONMap{"key": "duration.timeline", "severity": severity, "disposition": disposition, "state": test.findingState, "actionable": test.actionable, "overrideable": true}
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
			manualAllowed, manualReason := canonicalRunAllowsPublish(db, job, false)
			if manualAllowed != test.wantManualAllowed {
				t.Fatalf("manual handoff allowed=%t want=%t reason=%q", manualAllowed, test.wantManualAllowed, manualReason)
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
		{name: "blocking warning requires review", findings: []validationFinding{{Disposition: "blocking", State: "WARNING", Actionable: true}}, want: "REVIEW_REQUIRED"},
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

func TestValidationHandoffDecisionUsesBlockingDispositionNotActionability(t *testing.T) {
	tests := []struct {
		name     string
		outcome  string
		findings []validationFinding
		want     bool
	}{
		{name: "clean pass", outcome: "PASS", findings: []validationFinding{{Disposition: "blocking", State: "PASS"}}, want: true},
		{name: "nonactionable advisory warning", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "advisory", State: "WARNING"}}, want: true},
		{name: "actionable advisory warning", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "advisory", State: "WARNING", Actionable: true}}, want: true},
		{name: "advisory unverified warning", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "advisory", State: "UNVERIFIED"}}, want: true},
		{name: "blocking warning", outcome: "REVIEW_REQUIRED", findings: []validationFinding{{Disposition: "blocking", State: "WARNING"}}, want: false},
		{name: "critical unverified evidence", outcome: "REVIEW_REQUIRED", findings: []validationFinding{{Disposition: "blocking", State: "UNVERIFIED"}}, want: false},
		{name: "known failure", outcome: "FAIL", findings: []validationFinding{{Disposition: "blocking", State: "FAIL"}}, want: false},
		{name: "review required", outcome: "REVIEW_REQUIRED", findings: []validationFinding{{Disposition: "blocking", State: "REVIEW_REQUIRED"}}, want: false},
		{name: "inconsistent warning outcome", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "blocking", State: "PASS"}}, want: false},
		{name: "unknown unresolved disposition", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "unknown", State: "WARNING"}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, reason := validationHandoffDecision(test.outcome, test.findings)
			if got != test.want {
				t.Fatalf("handoff eligible=%t want=%t reason=%q", got, test.want, reason)
			}
			if !test.want && reason == "" {
				t.Fatal("blocked handoff must explain its reason")
			}
		})
	}
}

func TestPersistValidationHandoffStageKeepsManualAndAutomaticDecisionsAligned(t *testing.T) {
	tests := []struct {
		name       string
		outcome    string
		findings   []validationFinding
		actionable bool
		policy     []any
		automatic  bool
		wantStage  string
	}{
		{name: "manual pass", outcome: "PASS", automatic: false, wantStage: JobStageReadyToPublish},
		{name: "automatic pass defaults allowed", outcome: "PASS", automatic: true, wantStage: JobStageReadyToPublish},
		{name: "manual actionable advisory warning", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "advisory", State: "WARNING", Actionable: true}}, actionable: true, wantStage: JobStageReadyToPublish},
		{name: "automatic advisory warning requires explicit policy", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "advisory", State: "WARNING", Actionable: true}}, actionable: true, automatic: true, wantStage: JobStageValidationReview},
		{name: "automatic advisory warning explicitly allowed", outcome: "PASS_WITH_WARNINGS", findings: []validationFinding{{Disposition: "advisory", State: "WARNING", Actionable: true}}, actionable: true, policy: []any{"PASS", "PASS_WITH_WARNINGS"}, automatic: true, wantStage: JobStageReadyToPublish},
		{name: "blocking warning remains review", outcome: "REVIEW_REQUIRED", findings: []validationFinding{{Disposition: "blocking", State: "WARNING"}}, automatic: true, policy: []any{"PASS", "PASS_WITH_WARNINGS", "REVIEW_REQUIRED"}, wantStage: JobStageValidationReview},
		{name: "failure remains review", outcome: "FAIL", findings: []validationFinding{{Disposition: "blocking", State: "FAIL"}}, automatic: false, wantStage: JobStageValidationReview},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := testEncodeTestDB(t, "handoff-stage-"+strings.ReplaceAll(test.name, " ", "-"), &models.QueueJob{}, &models.AppSetting{})
			if test.policy != nil {
				if err := db.Create(&models.AppSetting{Key: "validation", Value: models.JSONMap{"autoAdvanceOutcomes": test.policy}}).Error; err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().UTC()
			job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageValidating, StageUpdatedAt: &now, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageValidating, "at": now.Format(time.RFC3339)}}}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			run := models.ValidationRun{Outcome: test.outcome, ActionableWarnings: test.actionable}
			for _, finding := range test.findings {
				key := finding.Key
				if key == "" {
					key = "test.finding"
				}
				run.Findings = append(run.Findings, models.JSONMap{
					"key": key, "disposition": finding.Disposition, "state": finding.State,
					"actionable": finding.Actionable, "overrideable": finding.Overrideable,
				})
			}
			stage, _, err := persistValidationHandoffStage(db, &job, run, test.findings, test.automatic)
			if err != nil {
				t.Fatal(err)
			}
			if stage != test.wantStage {
				t.Fatalf("stage=%q want=%q", stage, test.wantStage)
			}
			var persisted models.QueueJob
			if err := db.First(&persisted, job.ID).Error; err != nil {
				t.Fatal(err)
			}
			if persisted.Stage != test.wantStage || len(persisted.StageHistory) != 2 {
				t.Fatalf("persisted stage=%q history=%#v", persisted.Stage, persisted.StageHistory)
			}
		})
	}
}

func TestPersistValidationHandoffStageFailureStopsBeforePublisher(t *testing.T) {
	db := testEncodeTestDB(t, "handoff-stage-ready-failure", &models.QueueJob{}, &models.AppSetting{}, &models.PublisherPublication{})
	now := time.Now().UTC()
	job := models.QueueJob{Status: JobStatusCompleted, Stage: JobStageValidating, StageUpdatedAt: &now, StageHistory: models.JSONList{models.JSONMap{"stage": JobStageValidating, "at": now.Format(time.RFC3339)}}}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_ready_handoff BEFORE UPDATE OF stage ON queue_jobs WHEN NEW.stage = 'ready_to_publish' BEGIN SELECT RAISE(ABORT, 'ready stage write rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	run := models.ValidationRun{Outcome: "PASS"}
	stage, _, err := persistValidationHandoffStage(db, &job, run, nil, false)
	if err == nil || stage != "" {
		t.Fatalf("failed Ready transition stage=%q err=%v", stage, err)
	}
	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != JobStageValidating {
		t.Fatalf("failed Ready transition changed stage to %q", persisted.Stage)
	}
	var publications int64
	if err := db.Model(&models.PublisherPublication{}).Count(&publications).Error; err != nil {
		t.Fatal(err)
	}
	if publications != 0 {
		t.Fatalf("failed Ready transition created %d Publisher rows", publications)
	}
}

func TestValidationScoreIsInformationalOnly(t *testing.T) {
	policy := validationpolicy.DefaultPolicy()
	rule := policy.Rules["duration.timeline"]
	rule.FailurePenalty = 100
	policy.Rules["duration.timeline"] = rule
	finding := validationFinding{Key: "duration.timeline", Disposition: "blocking", State: "FAIL", Actionable: true}
	if score := validationDisplayScore([]validationFinding{finding}, policy); score != 0 {
		t.Fatalf("informational score=%d, want 0", score)
	}
	if outcome, _ := reduceValidationOutcome([]validationFinding{finding}); outcome != "FAIL" {
		t.Fatalf("score configuration changed canonical outcome to %s", outcome)
	}
}

func TestProfileQualificationPassDoesNotBypassFinalValidationFailure(t *testing.T) {
	qualification := models.ProfileQualification{Outcome: models.QualityOutcomePass}
	if qualification.Outcome != models.QualityOutcomePass {
		t.Fatal("test precondition: qualification should pass")
	}
	finalOutputFinding := validationFinding{Key: "output.integrity", Disposition: "blocking", State: "FAIL"}
	outcome, _ := reduceValidationOutcome([]validationFinding{finalOutputFinding})
	if outcome != models.QualityOutcomeFail {
		t.Fatalf("pre-batch qualification PASS bypassed final output Validation: outcome=%s", outcome)
	}
}

func TestOverrideUsesServerActorAllowsEmptyReasonAndCannotOverrideIdentity(t *testing.T) {
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
	requestBody := `{"scope":"rule","reason":"","ruleKeys":["duration.timeline"],"requestedHandoff":"publisher","actorId":"client_admin"}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/overrides", strings.NewReader(requestBody)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("override status=%d body=%s findings=%#v", recorder.Code, recorder.Body.String(), gate.Findings)
	}
	var event models.ValidationOverride
	if err := json.Unmarshal(recorder.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.ActorSource != "deployment" || event.ActorID != "local_operator" || event.Reason != "" || len(event.RuleKeys) != 1 || event.Scope != "rule" || event.OverrideVersion != validationOverrideV1 || event.CreatedAt.IsZero() {
		t.Fatalf("override audit event is incomplete or trusted client actor: %#v", event)
	}
	if event.ResultArtifactID != gate.Run.ResultArtifactID || event.ResultGenerationToken != gate.Run.ResultGenerationToken || event.ResultGenerationVersion != gate.Run.ResultGenerationVersion || event.ResultOutputRevision != gate.Run.ResultOutputRevision || event.PolicyVersion != gate.Run.PolicyVersion || event.PolicyFingerprint != gate.Run.PolicyFingerprint {
		t.Fatalf("override is not bound to exact Result and policy identity: %#v", event)
	}
	var existingOverrides []models.ValidationOverride
	if err := db.Where("validation_run_id = ?", gate.Run.RunID).Find(&existingOverrides).Error; err != nil {
		t.Fatal(err)
	}
	covered, _ := validationOverrideCoverage(existingOverrides, gate.Run, false)
	wantKeys := make([]string, 0)
	for key := range overrideableUnresolvedFindings(gate.Findings) {
		if !covered[key] {
			wantKeys = append(wantKeys, key)
		}
	}
	sort.Strings(wantKeys)
	if len(wantKeys) == 0 {
		t.Fatal("test precondition: no unresolved overrideable findings remain for skip-all")
	}
	allBody, err := json.Marshal(map[string]any{"scope": "all_overrideable", "reason": "", "ruleKeys": wantKeys, "requestedHandoff": "publisher"})
	if err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/overrides", strings.NewReader(string(allBody))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("skip-all status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var allEvent models.ValidationOverride
	if err := json.Unmarshal(recorder.Body.Bytes(), &allEvent); err != nil {
		t.Fatal(err)
	}
	if allEvent.Scope != "all_overrideable" || len(allEvent.RuleKeys) != len(wantKeys) {
		t.Fatalf("skip-all did not store its exact selected scope: %#v", allEvent)
	}
	for index, expected := range wantKeys {
		if fmt.Sprint(allEvent.RuleKeys[index]) != expected {
			t.Fatalf("skip-all key %d=%v want %s", index, allEvent.RuleKeys[index], expected)
		}
	}
	staleBody := `{"scope":"all_overrideable","reason":"","ruleKeys":["duration.timeline"],"requestedHandoff":"publisher"}`
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10)+"/overrides", strings.NewReader(staleBody)))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale skip-all scope status=%d body=%s", recorder.Code, recorder.Body.String())
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
	db := testEncodeTestDB(t, "quality-gate-review-history-summaries", &models.QueueJob{}, &models.ValidationRun{}, &models.ValidationOverride{}, &models.PublisherPublication{}, &models.PublisherAttemptEvidence{})
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

func TestHistoryListProjectsPublisherAndArchiveFactsWithBoundedBatchQueries(t *testing.T) {
	db := testEncodeTestDB(t, "quality-gate-history-publisher-batch", &models.QueueJob{}, &models.PublisherPublication{}, &models.PublisherAttemptEvidence{})
	now := time.Now().UTC()
	for index := uint(1); index <= 3; index++ {
		execution := index
		job := models.QueueJob{
			ExecutionNumber: &execution, MediaPath: "/missing/raw/episode-" + strconv.FormatUint(uint64(index), 10) + ".mkv",
			Status: JobStatusCompleted, PublishedPath: "/queue/projection.mkv", PublishedAt: &now,
			PublicationID: "queue-projection-must-not-be-authoritative", PublicationState: PublicationFailed,
			PublicationSummary: "stale Queue projection", OriginalArchiveStatus: "queue-archive-copy",
			ValidationRunID: "validation-run:queue-current",
		}
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		publicationID := "publication:history:" + strconv.FormatUint(uint64(index), 10)
		overrideIDs := models.JSONList{}
		if index == 3 {
			overrideIDs = models.JSONList{"validation-override:" + strconv.FormatUint(uint64(index), 10)}
		}
		publication := models.PublisherPublication{
			PublicationID: publicationID, QueueJobID: job.ID, ExecutionNumber: execution,
			State: PublicationPublished, DestinationPath: "/frozen/library/episode-" + strconv.FormatUint(uint64(index), 10) + ".mkv",
			PublishMode: PublishModeReplaceLibrary, AttemptCount: 2,
			ValidationRunID:            "validation-run:frozen:" + strconv.FormatUint(uint64(index), 10),
			ValidationOverrideEventIDs: overrideIDs,
			OriginalArchiveStatus:      "failed", OriginalArchivedPath: "/archive/original-" + strconv.FormatUint(uint64(index), 10) + ".mkv",
		}
		if err := db.Create(&publication).Error; err != nil {
			t.Fatal(err)
		}
		if index == 3 {
			oldExecution := models.PublisherPublication{PublicationID: "publication:history:older-execution", QueueJobID: job.ID, ExecutionNumber: 2, State: PublicationFailed, DestinationPath: "/obsolete/library/episode-3.mkv", PublishMode: PublishModeStandard}
			if err := db.Create(&oldExecution).Error; err != nil {
				t.Fatal(err)
			}
		}
		for _, evidence := range []models.PublisherAttemptEvidence{
			{EvidenceID: "evidence:old:" + strconv.FormatUint(uint64(index), 10), PublicationID: publicationID, AttemptID: "attempt:old", Phase: "delivery", Outcome: "failed", Summary: "older outcome", CreatedAt: now.Add(-time.Minute)},
			{EvidenceID: "evidence:latest:" + strconv.FormatUint(uint64(index), 10), PublicationID: publicationID, AttemptID: "attempt:latest", Phase: "commit", Outcome: "published", Summary: "latest persisted outcome", CreatedAt: now},
		} {
			if err := db.Create(&evidence).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	legacyPublishedAt := now.Add(-time.Hour)
	legacy := models.QueueJob{
		MediaPath: "/missing/raw/legacy.mkv", Status: JobStatusCompleted,
		PublishedPath: "/historical/library/legacy.mkv", PublishedAt: &legacyPublishedAt,
		PublicationState: PublicationPublished,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}

	queryCount := 0
	rowQueryCount := 0
	queries := make([]string, 0, 8)
	if err := db.Callback().Query().After("gorm:query").Register("history_publisher_query_count", func(tx *gorm.DB) {
		queryCount++
		queries = append(queries, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatal("register History query counter:", err)
	}
	if err := db.Callback().Row().After("gorm:row").Register("history_publisher_row_query_count", func(tx *gorm.DB) {
		rowQueryCount++
		queries = append(queries, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatal("register History row query counter:", err)
	}
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.GET("/validation/history", NewValidationHandler(db).HistoryList)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/validation/history?limit=100", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("History list status=%d body=%s", response.Code, response.Body.String())
	}
	var history validationSummaryPage
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Total != 4 || len(history.Items) != 4 {
		t.Fatalf("History row count=%d total=%d, want 4: %#v", len(history.Items), history.Total, history)
	}
	if queryCount+rowQueryCount != 6 {
		t.Fatalf("History used %d persisted queries for 4 rows; want bounded count/page plus staging-schema checks and publication/evidence batches: %v", queryCount+rowQueryCount, queries)
	}
	var canonical *validationSummaryRow
	var canonicalWithoutOverride *validationSummaryRow
	var legacyRow *validationSummaryRow
	for index := range history.Items {
		row := &history.Items[index]
		if row.Publisher != nil && row.Publisher.PublicationID == "publication:history:3" {
			canonical = row
		}
		if row.Publisher != nil && row.Publisher.PublicationID == "publication:history:2" {
			canonicalWithoutOverride = row
		}
		if row.ID == legacy.ID {
			legacyRow = row
		}
	}
	if canonical == nil || canonical.Publisher == nil {
		t.Fatalf("canonical Publisher projection is missing: %#v", history.Items)
	}
	projection := canonical.Publisher
	if projection.PublicationID != "publication:history:3" || projection.State != PublicationPublished || projection.DestinationPath != "/frozen/library/episode-3.mkv" || projection.PublishMode != PublishModeReplaceLibrary {
		t.Fatalf("History did not project canonical identity, state, destination, and frozen mode: %#v", projection)
	}
	if projection.AttemptCount != 2 || projection.LatestAttempt == nil || projection.LatestAttempt.Outcome != "published" || projection.LatestAttempt.Summary != "latest persisted outcome" {
		t.Fatalf("History did not project the bounded latest attempt summary: %#v", projection)
	}
	if projection.ValidationRunID != "validation-run:frozen:3" || !projection.ValidationOverridePresent || len(projection.ValidationOverrideEventIDs) != 1 || projection.ValidationOverrideEventIDs[0] != "validation-override:3" {
		t.Fatalf("History lost the frozen ValidationRun/override references: %#v", projection)
	}
	if canonicalWithoutOverride == nil || canonicalWithoutOverride.Publisher.ValidationOverridePresent || len(canonicalWithoutOverride.Publisher.ValidationOverrideEventIDs) != 0 {
		t.Fatalf("History did not preserve explicit absence of a frozen override: %#v", canonicalWithoutOverride)
	}
	publisherBatchBound := false
	for _, query := range queries {
		if strings.Contains(query, "execution_number = ?") {
			publisherBatchBound = true
			break
		}
	}
	if !publisherBatchBound {
		t.Fatalf("Publisher batch query was not bound to the page's exact executions: %v", queries)
	}
	if canonical.OriginalArchiveStatus != "failed" || canonical.OriginalArchivedPath != "/archive/original-3.mkv" || projection.State != PublicationPublished {
		t.Fatalf("#174 archive outcome was conflated with canonical publication state: row=%#v", canonical)
	}
	if canonical.PublicationID != "" || canonical.PublicationState != "" || canonical.PublicationSummary != "" {
		t.Fatalf("History exposed QueueJob publication projections as canonical authority: %#v", canonical)
	}
	if legacyRow == nil || legacyRow.Publisher != nil || legacyRow.PublicationID != "" || legacyRow.PublicationState != "" || legacyRow.PublishedPath != "/historical/library/legacy.mkv" || legacyRow.PublishedAt == nil {
		t.Fatalf("legacy PublishedAt/PublishedPath did not remain explicitly unbound: %#v", legacyRow)
	}
}

func timePointer(value time.Time) *time.Time { return &value }
