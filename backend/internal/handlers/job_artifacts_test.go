package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
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
