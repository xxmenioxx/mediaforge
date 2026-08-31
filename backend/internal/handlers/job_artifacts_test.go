package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
)

func TestJobArtifactMatchesJobRequiresSameIDAndMediaPath(t *testing.T) {
	job := models.QueueJob{ID: 1, MediaPath: "/media/raw/series/current.mkv"}

	matching := map[string]any{
		"job": map[string]any{
			"id":        float64(1),
			"mediaPath": "/media/raw/series/current.mkv",
		},
	}
	if !jobArtifactMatchesJob(matching, job) {
		t.Fatalf("expected artifact to match current job")
	}

	wrongAsset := map[string]any{
		"job": map[string]any{
			"id":        float64(1),
			"mediaPath": "/media/raw/series/old-asset.mkv",
		},
	}
	if jobArtifactMatchesJob(wrongAsset, job) {
		t.Fatalf("expected artifact with old media path to be ignored")
	}

	wrongJob := map[string]any{
		"job": map[string]any{
			"id":        float64(2),
			"mediaPath": "/media/raw/series/current.mkv",
		},
	}
	if jobArtifactMatchesJob(wrongJob, job) {
		t.Fatalf("expected artifact with different job id to be ignored")
	}
}

func TestNormalizeAnalysisRecordHDRCorrectsTenBitSDR(t *testing.T) {
	records := []any{map[string]any{
		"scan": map[string]any{
			"hdr": true,
			"rawProbe": map[string]any{"streams": []any{map[string]any{
				"codec_type": "video", "codec_name": "hevc", "profile": "Main 10", "pix_fmt": "yuv420p10le", "color_range": "tv",
			}}},
		},
	}}
	if corrected := normalizeAnalysisRecordHDR(records); corrected != 1 {
		t.Fatalf("corrected=%d want=1", corrected)
	}
	scan := records[0].(map[string]any)["scan"].(map[string]any)
	if hdr, _ := scan["hdr"].(bool); hdr {
		t.Fatal("10-bit SDR historical snapshot still marked HDR")
	}
}

func TestAssetConversionReportPreservesFrameStructurePresetAndAdvancedValues(t *testing.T) {
	pStrategy := 1
	adaptiveI, adaptiveB := true, false
	report := assetConversionReport(AssetConversionOverrideState{
		FrameStructureMode: "compatible", FrameStructureGOPMode: "custom", FrameStructureGOPFrames: 75,
		FrameStructureBFrameMode: "off", FrameStructureMaxBFrames: 0,
		QSVAdaptiveI: &adaptiveI, QSVAdaptiveB: &adaptiveB, QSVPStrategy: &pStrategy,
	})
	if !report.HasOverrides || report.ProfileOverrides["frameStructureMode"] != "compatible" || report.ProfileOverrides["frameStructureGopFrames"] != 75 || report.ProfileOverrides["frameStructureBFrameMode"] != "off" || report.ProfileOverrides["qsvPStrategy"] != 1 {
		t.Fatalf("frame structure override was not preserved in job artifacts: %#v", report.ProfileOverrides)
	}
}

func TestTrackDecisionReportIncludesRequestedResolvedAndArtifacts(t *testing.T) {
	plan := ResolvedTrackPlan{
		AudioStreams:        []ResolvedTrackStream{{StreamIndex: 1, Language: "jpn"}},
		RemovedAudioStreams: []ResolvedTrackStream{{StreamIndex: 2, Language: "eng"}},
		SubtitleStreams:     []ResolvedSubtitleTrack{{StreamIndex: 3, Codec: "ass", Language: "spa", Action: SubtitleDispositionKeepAndExtract}},
		AttachmentPolicy:    AttachmentPolicyAuto, AttachmentsKept: true, AttachmentReason: "embedded ASS/SSA subtitle may require font attachments",
		ChapterPolicy: ChapterPolicyRemove, ChaptersKept: false,
		SidecarOutputs: []ResolvedTrackSidecar{{StreamIndex: 3, Codec: "ass", Language: "spa", Format: "ass"}},
	}
	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{TrackProfileKey: "anime-tracks", TrackProfileSnapshot: models.JSONMap{
		"key": "anime-tracks", "subtitleRules": models.JSONList{models.JSONMap{"language": "spa", "action": "keep_and_extract"}},
		resolvedTrackPlanSnapshotKey: planMap,
	}, SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{StreamIndex: 3, Format: "ass", Status: "ready"}})}
	report := AssetConversionReport{}
	attachTrackDecisionReport(&report, job)
	if report.TrackProfileKey != "anime-tracks" || report.ResolvedTrackPlan == nil || len(report.ResolvedTrackPlan.RemovedAudioStreams) != 1 || report.TrackRequested["key"] != "anime-tracks" || report.ResolvedTrackPlan.AttachmentReason == "" || report.ResolvedTrackPlan.FontAttachmentsExported {
		t.Fatalf("track decision report is incomplete: %#v", report)
	}
	outputs := jobArtifactOutputs(job)
	if sidecars := workerSliceValue(outputs["sidecars"]); len(sidecars) != 1 {
		t.Fatalf("artifact set did not include sidecars: %#v", outputs)
	}
}

func TestSmartUpscalePlanReportPreservesRequestedResolvedAndGeometry(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": ResolvedUpscaleDecision{
		RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p, SourceWidth: 704, SourceHeight: 448,
		SourceSAR: "40:33", SourceDAR: "40:21", TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1",
		UpscaleApplied: true, SharpenMode: UpscaleSharpenLight, Confidence: UpscaleConfidenceHigh,
		Reasons: []string{"reliable_sd_progressive_output"}, Warnings: []string{},
	}}}
	report := smartUpscalePlanReport(profile, map[string]any{"streams": []interface{}{map[string]interface{}{"codec_type": "video", "width": 720, "height": 480, "sample_aspect_ratio": "32:27", "display_aspect_ratio": "16:9"}}})
	if report["requestedMode"] != UpscaleModeAuto || report["resolvedMode"] != ResolvedUpscale720p || report["status"] != "planned" {
		t.Fatalf("decision history missing: %#v", report)
	}
	if report["sourceStorage"].(models.JSONMap)["width"] != 720 || report["effectiveGeometry"].(models.JSONMap)["width"] != 704 || report["resolvedOutput"].(models.JSONMap)["width"] != 1280 {
		t.Fatalf("history geometry conflated: %#v", report)
	}
}

func TestResultArtifactUsesValidatedSmartUpscaleReport(t *testing.T) {
	db := testEncodeTestDB(t, "result-artifact-validated-smart-upscale", &models.QueueJob{})
	reportsDir := filepath.Join(t.TempDir(), "results")
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"resultsReportsPath": reportsDir}}).Error; err != nil {
		t.Fatal(err)
	}
	decision := ResolvedUpscaleDecision{
		RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p,
		SourceWidth: 704, SourceHeight: 448, SourceSAR: "40:33", SourceDAR: "40:21",
		TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
		SharpenMode: UpscaleSharpenLight, Confidence: UpscaleConfidenceHigh,
		Reasons: []string{"reliable_sd_progressive_output"},
	}
	profile := models.Profile{Name: "Validated upscale", VideoCodec: "hevc", WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": decision}}
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "queue_create")
	if err != nil {
		t.Fatal(err)
	}
	validated := models.JSONMap{
		"requestedMode": UpscaleModeAuto, "resolvedMode": ResolvedUpscale720p, "upscaleApplied": true,
		"sourceStorage":     models.JSONMap{"width": 720, "height": 480, "sar": "32:27", "dar": "16:9"},
		"effectiveGeometry": models.JSONMap{"width": 704, "height": 448, "sar": "40:33", "dar": "40:21"},
		"resolvedOutput":    models.JSONMap{"width": 1280, "height": 720, "sar": "1:1", "dar": "16:9"},
		"actualOutput":      models.JSONMap{"width": 1280, "height": 720, "sar": "1/1", "dar": "16:9"},
		"validationResult":  "passed", "status": "passed", "reasons": models.JSONList{"reliable_sd_progressive_output"},
	}
	job := models.QueueJob{MediaPath: "/media/raw/validated-upscale.mkv", ProfileSnapshot: snapshot, ValidationReport: models.JSONMap{"smartUpscale": validated}, Status: JobStatusCompleted}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	report := unknownRecord(artifact["smartUpscale"])
	if report == nil || report["validationResult"] != "passed" || report["status"] != "passed" || stringFromUnknown(report["requestedMode"]) != "auto" || stringFromUnknown(report["resolvedMode"]) != "720p" {
		t.Fatalf("validated Smart Upscale report was replaced by planned data: %#v", report)
	}
	for key, wantWidth := range map[string]int{"sourceStorage": 720, "effectiveGeometry": 704, "resolvedOutput": 1280, "actualOutput": 1280} {
		if got := workerIntValue(unknownRecord(report[key])["width"], 0); got != wantWidth {
			t.Fatalf("%s width=%d want=%d report=%#v", key, got, wantWidth, report)
		}
	}
	if _, err := os.Stat(reportsDir); err != nil {
		t.Fatalf("result artifact directory missing: %v", err)
	}
}

func TestSmartUpscaleCopyKeepSourceHistoryPreservesRequestAndEvidence(t *testing.T) {
	profile := models.Profile{VideoCodec: "copy", WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": ResolvedUpscaleDecision{
		RequestedMode: UpscaleMode1080p, ResolvedMode: ResolvedUpscaleKeepSource, SourceWidth: 720, SourceHeight: 480,
		TargetWidth: 720, TargetHeight: 480, UpscaleApplied: false, SharpenMode: UpscaleSharpenOff,
		Reasons: []string{"keep_source_video_copy"}, Warnings: []string{"Smart Upscale requires video re-encoding; Video Codec is configured as Copy."},
	}}}
	report := smartUpscalePlanReport(profile, nil)
	if report["requestedMode"] != UpscaleMode1080p || report["resolvedMode"] != ResolvedUpscaleKeepSource || report["status"] != "keep_source" {
		t.Fatalf("Copy Keep Source request was not preserved: %#v", report)
	}
	if len(report["reasons"].([]string)) != 1 || len(report["warnings"].([]string)) != 1 {
		t.Fatalf("Copy Keep Source evidence missing: %#v", report)
	}
}
