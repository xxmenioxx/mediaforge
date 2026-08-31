package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
)

func TestFrameFidelityTreatsUnknownAsUnverified(t *testing.T) {
	result := frameFidelityValue(models.JSONMap{"sar": ""}, models.JSONMap{"sar": ""}, "sar", false)
	if result["status"] != "unverified" {
		t.Fatalf("status=%v", result["status"])
	}
}

func TestFrameFidelityAllowsIntentionalAspectChange(t *testing.T) {
	result := frameFidelityValue(models.JSONMap{"sar": "8:9"}, models.JSONMap{"sar": "23:27"}, "sar", true)
	if result["status"] != "changed_intentionally" {
		t.Fatalf("status=%v", result["status"])
	}
}

func TestSmartUpscaleValidationGeometryAspectAndFrameStructure(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{
			RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p,
			SourceWidth: 704, SourceHeight: 448, SourceSAR: "40:33", SourceDAR: "40:21",
			TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
			SharpenMode: UpscaleSharpenLight, Confidence: UpscaleConfidenceHigh,
			Reasons: []string{"reliable_sd_progressive_output"},
		},
	}}
	source := models.JSONMap{"width": 720, "height": 480, "sampleAspectRatio": "32:27", "displayAspectRatio": "16:9"}
	output := models.JSONMap{"width": 1280, "height": 720, "sampleAspectRatio": "1/1", "displayAspectRatio": "16:9", "frameRate": "24000/1001", "realFrameRate": "23.976023976", "fieldOrder": "progressive"}
	report := validateSmartUpscaleOutput(profile, source, output)
	if report["status"] != "passed" {
		t.Fatalf("valid output rejected: %#v", report)
	}
	storage := report["sourceStorage"].(models.JSONMap)
	effective := report["effectiveGeometry"].(models.JSONMap)
	if storage["width"] != 720 || storage["height"] != 480 || effective["width"] != 704 || effective["height"] != 448 {
		t.Fatalf("storage/effective geometry conflated: %#v", report)
	}
}

func TestSmartUpscaleValidationRejectsWrongGeometryAndStretchedFourByThree(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": ResolvedUpscaleDecision{
		RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p, SourceWidth: 720, SourceHeight: 480,
		TargetWidth: 960, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
	}}}
	report := validateSmartUpscaleOutput(profile, models.JSONMap{"width": 720, "height": 480}, models.JSONMap{"width": 1280, "height": 720, "sampleAspectRatio": "1:1", "displayAspectRatio": "16:9"})
	if report["status"] != "mismatch" {
		t.Fatalf("stretched 4:3 output passed: %#v", report)
	}
	fields := report["fields"].(models.JSONMap)
	if fields["geometry"].(models.JSONMap)["status"] != "mismatch" || fields["displayAspectRatio"].(models.JSONMap)["status"] != "mismatch" {
		t.Fatalf("geometry/DAR mismatch missing: %#v", fields)
	}
}

func TestSmartUpscaleValidationAcceptanceGeometryModes(t *testing.T) {
	tests := []struct {
		name                 string
		width, height        int
		dar                  string
	}{
		{name: "NTSC anamorphic", width: 1280, height: 720, dar: "16:9"},
		{name: "NTSC four by three", width: 960, height: 720, dar: "4:3"},
		{name: "PAL anamorphic", width: 1280, height: 720, dar: "16:9"},
		{name: "explicit 1080p", width: 1920, height: 1080, dar: "16:9"},
		{name: "custom", width: 1200, height: 900, dar: "4:3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleModeCustom, ResolvedMode: ResolvedUpscaleCustom, TargetWidth: test.width, TargetHeight: test.height, TargetSAR: "1:1", UpscaleApplied: true}}}
			report := validateSmartUpscaleOutput(profile, nil, models.JSONMap{"width": test.width, "height": test.height, "sampleAspectRatio": "1:1", "displayAspectRatio": test.dar})
			if report["status"] != "passed" {
				t.Fatalf("acceptance output rejected: %#v", report)
			}
		})
	}
}

func TestSmartUpscaleKeepSourceIsReportedAsSuccess(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscaleKeepSource, SourceWidth: 1280, SourceHeight: 720, TargetWidth: 1280, TargetHeight: 720, SharpenMode: UpscaleSharpenOff, Reasons: []string{"keep_source_above_sd"}}}}
	report := validateSmartUpscaleOutput(profile, models.JSONMap{"width": 1280, "height": 720}, models.JSONMap{"width": 1280, "height": 720})
	if report["status"] != "passed" || report["validationResult"] != "keep_source" || report["requestedMode"] != UpscaleModeAuto {
		t.Fatalf("Keep Source reported as failure: %#v", report)
	}
}

func TestQueueValidationBlocksSmartUpscaleGeometryMismatch(t *testing.T) {
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
	probeScript := "#!/bin/sh\ncase \"$*\" in\n  *output.mkv*) printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"hevc\",\"width\":720,\"height\":480,\"sample_aspect_ratio\":\"1:1\",\"display_aspect_ratio\":\"3:2\",\"avg_frame_rate\":\"24000/1001\",\"r_frame_rate\":\"24000/1001\",\"field_order\":\"progressive\"}],\"format\":{\"size\":\"7\"}} ;;\n  *) printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"mpeg2video\",\"width\":720,\"height\":480,\"sample_aspect_ratio\":\"32:27\",\"display_aspect_ratio\":\"16:9\",\"avg_frame_rate\":\"30000/1001\",\"r_frame_rate\":\"30000/1001\",\"field_order\":\"progressive\"}],\"format\":{\"size\":\"7\"}} ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p, SourceWidth: 720, SourceHeight: 480, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true},
	}}
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "test")
	if err != nil {
		t.Fatal(err)
	}
	db := testEncodeTestDB(t, "smart-upscale-validation-mismatch")
	result := validateQueueJob(db, models.QueueJob{Status: JobStatusCompleted, MediaPath: sourcePath, OutputPath: outputPath, ProfileSnapshot: snapshot})
	if result.Status != ValidationStatusFailed {
		t.Fatalf("geometry mismatch was not blocking: %#v", result)
	}
	found := false
	for _, check := range result.Checks {
		if check.Key == "smart_upscale_output" && check.Status == "failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Smart Upscale blocking check missing: %#v", result.Checks)
	}
}

func TestHEVCLevelValidationChecksRecommendedOutputLevel(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "recommended", "hevcLevel": "4.0"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1", "bitrate": "6418000"}

	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "4.0"})
	if validated["status"] != "validated" || validated["requested"] != "4.0" {
		t.Fatalf("validated result=%#v", validated)
	}

	mismatch := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "5.0"})
	if mismatch["status"] != "mismatch" || mismatch["output"] != "5.0" {
		t.Fatalf("mismatch result=%#v", mismatch)
	}
}

func TestHEVCLevelValidationChecksAutoOutputAgainstAssetRecommendation(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "auto", "videoEncoder": "hevc_qsv"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1", "bitrate": "6418000"}
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "4.0"})
	if validated["status"] != "validated" || validated["requested"] != "4.0" {
		t.Fatalf("auto Level validation=%#v", validated)
	}
}

func TestHEVCLevelValidationUsesFrozenEffectiveLevel(t *testing.T) {
	worker := map[string]interface{}{
		"hevcLevelMode": "auto", "videoEncoder": "hevc_qsv",
		"hevcLevel": "4.0", "hevcLevelEffective": "4.1", "effectiveOutputFrameRate": "60/1",
	}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1"}
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "4.1"})
	if validated["status"] != "validated" || validated["requested"] != "4.1" {
		t.Fatalf("validation ignored frozen effective Level: %#v", validated)
	}
}

func TestHEVCLevelValidationFallbackUsesEffectiveOutputFPS(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "auto", "videoEncoder": "hevc_qsv", "effectiveOutputFrameRate": "60/1"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1"}
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "4.1"})
	if validated["status"] != "validated" || validated["requested"] != "4.1" {
		t.Fatalf("validation fallback used stale source FPS: %#v", validated)
	}
}

func TestHEVCLevelValidationChecksObservedMainTierBitrate(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "custom", "hevcLevel": "4.0", "hevcLevelTier": "main", "videoEncoder": "hevc_qsv"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1"}
	output := models.JSONMap{"codec": "hevc", "hevcLevel": "4.0", "bitrate": 13_000_000}
	validated := validateHEVCLevelField(worker, source, output)
	if validated["status"] != "mismatch" || validated["bitrateStatus"] != "exceeds_main_tier" || validated["effectiveTier"] != "main" {
		t.Fatalf("Main Tier bitrate validation=%#v", validated)
	}
}

func TestHEVCLevelValidationRejectsCustomLevelBelowOutputGeometry(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "custom", "hevcLevel": "4.0", "videoEncoder": "hevc_qsv"}
	output := models.JSONMap{"codec": "hevc", "hevcLevel": "4.0", "width": 3840, "height": 2160, "frameRate": "24/1"}
	validated := validateHEVCLevelField(worker, models.JSONMap{}, output)
	if validated["status"] != "mismatch" || validated["requirementsStatus"] != "exceeds_level_limits" {
		t.Fatalf("Custom Level geometry validation=%#v", validated)
	}
}
