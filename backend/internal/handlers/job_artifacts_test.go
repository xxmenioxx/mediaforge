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
