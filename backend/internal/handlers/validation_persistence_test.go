package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

func TestValidateJobPersistsSiblingColorAndFrameFidelityReports(t *testing.T) {
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(tempDir, "source.mkv")
	outputPath := filepath.Join(tempDir, "output.mkv")
	for _, path := range []string{sourcePath, outputPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	probeScript := "#!/bin/sh\ncase \"$*\" in\n  *output.mkv*) printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"profile\":\"High\",\"width\":720,\"height\":480,\"sample_aspect_ratio\":\"32:27\",\"display_aspect_ratio\":\"16:9\",\"avg_frame_rate\":\"30000/1001\",\"r_frame_rate\":\"30000/1001\",\"field_order\":\"progressive\",\"pix_fmt\":\"yuv420p\",\"chroma_location\":\"left\",\"color_space\":\"smpte170m\",\"color_transfer\":\"smpte170m\",\"color_primaries\":\"smpte170m\",\"color_range\":\"tv\",\"nb_read_frames\":\"30\",\"duration\":\"1.001\"}],\"format\":{\"size\":\"7\",\"duration\":\"1.001\"}}' ;;\n  *) printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"mpeg2video\",\"profile\":\"Main\",\"width\":720,\"height\":480,\"sample_aspect_ratio\":\"32:27\",\"display_aspect_ratio\":\"16:9\",\"avg_frame_rate\":\"30000/1001\",\"r_frame_rate\":\"30000/1001\",\"field_order\":\"progressive\",\"pix_fmt\":\"yuv420p\",\"chroma_location\":\"left\",\"color_space\":\"smpte170m\",\"color_transfer\":\"smpte170m\",\"color_primaries\":\"smpte170m\",\"color_range\":\"tv\",\"nb_read_frames\":\"30\",\"duration\":\"1.001\"}],\"format\":{\"size\":\"7\",\"duration\":\"1.001\"}}' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"finalColorPolicy": "preserve", "effectiveFinalColorPolicy": "preserve",
		"effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{
			RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p,
			SourceWidth: 720, SourceHeight: 480, TargetWidth: 1280, TargetHeight: 720,
			TargetSAR: "1:1", UpscaleApplied: true,
		},
	}}
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "validation-persistence-test")
	if err != nil {
		t.Fatal(err)
	}
	db := testEncodeTestDB(t, "validation-report-json-cycle", &models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{})
	reportsDir := filepath.Join(tempDir, "reports")
	asIsReportsDir := filepath.Join(tempDir, "as-is")
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": reportsDir, "asIsReportsPath": asIsReportsDir}}).Error; err != nil {
		t.Fatal(err)
	}
	execution := uint(1)
	job := models.QueueJob{
		MediaPath: sourcePath, OutputPath: outputPath, Status: JobStatusCompleted,
		ProfileSnapshot: snapshot, ValidationStatus: ValidationStatusPending, ExecutionNumber: &execution,
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
	if err := writeJobAsIsArtifact(db, job, profile, nil, "", job.ProcessingMode, ResolvedStreamPlan{}); err != nil {
		t.Fatalf("write AS-IS evidence: %v", err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatalf("write canonical Result: %v", err)
	}

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	router := gin.New()
	router.POST("/api/validation/jobs/:id", NewValidationHandler(db).ValidateJob)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/validation/jobs/"+strconv.FormatUint(uint64(job.ID), 10), nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ValidateJob status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var returned ValidationResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &returned); err != nil {
		t.Fatalf("decode validation response: %v", err)
	}
	if returned.Outcome != "FAIL" || returned.Status != ValidationStatusFailed {
		t.Fatalf("frozen video property failure was not retained: outcome=%q status=%q findings=%#v", returned.Outcome, returned.Status, returned.Findings)
	}
	failedRule := false
	for _, finding := range returned.Findings {
		if finding.Key == "video.properties" && finding.State == "FAIL" {
			failedRule = true
		}
	}
	if !failedRule {
		t.Fatalf("expected failed frozen video property finding in Validation result: %#v", returned.Findings)
	}

	if _, err := json.Marshal(returned.Report); err != nil {
		t.Fatalf("final ValidationReport is not JSON-marshallable: %v", err)
	}
	colorPolicy, ok := validationReportRecord(returned.Report["colorPolicy"])
	if !ok {
		t.Fatalf("colorPolicy sibling is missing: %#v", returned.Report)
	}
	frameFidelity, ok := validationReportRecord(returned.Report["frameFidelity"])
	if !ok {
		t.Fatalf("frameFidelity sibling is missing: %#v", returned.Report)
	}
	if _, nested := colorPolicy["frameFidelity"]; nested {
		t.Fatalf("frameFidelity was attached back into colorPolicy: %#v", colorPolicy)
	}
	specialized, ok := validationReportRecord(frameFidelity["specialized"])
	if !ok {
		t.Fatalf("Frame Fidelity specialized evidence is missing: %#v", frameFidelity)
	}
	specializedColor, ok := validationReportRecord(specialized["colorPolicy"])
	if !ok || !reflect.DeepEqual(specializedColor, colorPolicy) {
		t.Fatalf("specialized color policy evidence was not preserved: %#v", specialized)
	}
	if _, ok := specialized["cadence"]; !ok {
		t.Fatalf("specialized cadence evidence was dropped: %#v", specialized)
	}
	if _, ok := specialized["hevcLevel"]; !ok {
		t.Fatalf("specialized HEVC evidence was dropped: %#v", specialized)
	}
	if _, ok := validationReportRecord(frameFidelity["fields"]); !ok {
		t.Fatalf("Frame Fidelity fields are missing: %#v", frameFidelity)
	}

	var persisted models.QueueJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.ValidationStatus != ValidationStatusFailed {
		t.Fatalf("normal failed check was not persisted as a Validation result: status=%q", persisted.ValidationStatus)
	}
	if persisted.ValidationOutcome != "FAIL" || persisted.ValidationRunID == "" {
		t.Fatalf("immutable ValidationRun projection missing: outcome=%q run=%q", persisted.ValidationOutcome, persisted.ValidationRunID)
	}
	if _, err := json.Marshal(persisted.ValidationReport); err != nil {
		t.Fatalf("persisted ValidationReport is not JSON-marshallable: %v", err)
	}
	if _, ok := validationReportRecord(persisted.ValidationReport["colorPolicy"]); !ok {
		t.Fatalf("persisted colorPolicy report is unavailable: %#v", persisted.ValidationReport)
	}
	if _, ok := validationReportRecord(persisted.ValidationReport["frameFidelity"]); !ok {
		t.Fatalf("persisted Frame Fidelity report is unavailable: %#v", persisted.ValidationReport)
	}
}

func validationReportRecord(value any) (map[string]any, bool) {
	switch record := value.(type) {
	case map[string]any:
		return record, true
	case models.JSONMap:
		return map[string]any(record), true
	default:
		return nil, false
	}
}
