package handlers

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestRestorationRecommendationsReuseMatureResolvers(t *testing.T) {
	tests := []struct {
		name        string
		interlace   InterlaceAnalysis
		cadence     CadenceAnalysis
		cadencePlan CadenceRecommendation
		want        string
	}{
		{
			name: "telecine", interlace: InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "telecine", Confidence: .95, RecommendedMode: "ivtc_tff"},
			cadence:     CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "hard_telecine", Confidence: .95},
			cadencePlan: CadenceRecommendation{Version: 1, Operation: "inverse_telecine", OutputFrameRate: "24000/1001", Confidence: .95}, want: "ivtc",
		},
		{name: "interlaced", interlace: InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "interlaced", Confidence: .95, DetectedFieldOrder: "tff", RecommendedMode: "bwdif_tff"}, want: "deinterlace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scan := recommendationScan(test.interlace, test.cadence, test.cadencePlan)
			plan := buildRestorationRecommendationPlan(scan, proposedProfileForScan(scan), false)
			item := recommendationByID(t, plan, "frame_structure")
			if item.State != RestorationRecommendationRecommended || item.RecommendedValue != test.want || !item.Actionable() {
				t.Fatalf("frame recommendation=%#v", item)
			}
		})
	}
}

func TestRestorationRecommendationsReuseSmartUpscaleDecision(t *testing.T) {
	scan := recommendationScan(InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", Confidence: .95}, CadenceAnalysis{}, CadenceRecommendation{})
	proposal := proposedProfileForScan(scan)
	proposal.WorkerConfig["upscaleMode"] = "auto"
	plan := buildRestorationRecommendationPlan(scan, proposal, false)
	item := recommendationByID(t, plan, "upscale")
	if item.State != RestorationRecommendationRecommended || item.RecommendedValue != "720p" || item.ResolvedOutput == nil || item.ResolvedOutput.Width != 1280 || item.ResolvedOutput.Height != 720 {
		t.Fatalf("upscale recommendation=%#v", item)
	}

	hd := scan
	hd.Width, hd.Height = 1280, 720
	hd.VideoStreams = models.JSONList{map[string]any{"width": 1280, "height": 720, "sampleAspectRatio": "1:1", "displayAspectRatio": "16:9"}}
	keep := recommendationByID(t, buildRestorationRecommendationPlan(hd, proposal, false), "upscale")
	if keep.RecommendedValue != "keep_source" || keep.State != RestorationRecommendationRecommended {
		t.Fatalf("Keep Source must remain an explicit informational/actionable resolver result: %#v", keep)
	}
}

func TestRestorationRecommendationsPreserveResolvedSharpenAndVideoCopyReason(t *testing.T) {
	scan := recommendationScan(InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", Confidence: .95}, CadenceAnalysis{}, CadenceRecommendation{})
	proposal := proposedProfileForScan(scan)
	proposal.WorkerConfig["upscaleMode"] = "auto"
	proposal.WorkerConfig["upscaleSharpen"] = "light"
	plan := buildRestorationRecommendationPlan(scan, proposal, false)
	sharpen := recommendationByID(t, plan, "sharpen")
	if sharpen.State != RestorationRecommendationRecommended || sharpen.RecommendedValue != "light" || sharpen.Patch["upscaleSharpen"] != "light" {
		t.Fatalf("resolved Smart Upscale sharpen was not resurfaced: %#v", sharpen)
	}

	proposal.VideoCodec = "copy"
	proposal.WorkerConfig["upscaleMode"] = "1080p"
	copyPlan := buildRestorationRecommendationPlan(scan, proposal, false)
	upscale := recommendationByID(t, copyPlan, "upscale")
	if upscale.CurrentValue != "1080p" || upscale.RecommendedValue != "keep_source" || len(upscale.Reasons) != 1 || upscale.Reasons[0] != "keep_source_video_copy" || len(upscale.Warnings) == 0 {
		t.Fatalf("Video Copy request/resolution evidence was lost: %#v", upscale)
	}
}

func TestRestorationRecommendationsDoNotInventEvidenceThresholds(t *testing.T) {
	value := .123456
	analysis := RestorationAnalysis{
		Version: 1, Status: "available", Source: "sampled_ffmpeg_metrics", Windows: 3, SampledFrames: 9,
		Blocking:    RestorationSignalEvidence{Availability: "available", Severity: "unclassified", Value: &value, Confidence: "high", SupportingEvidence: []string{"lavfi.block mean=0.123456"}},
		Noise:       RestorationSignalEvidence{Availability: "ambiguous", Severity: "unknown", Value: &value, Confidence: "low"},
		Grain:       RestorationSignalEvidence{Availability: "ambiguous", Severity: "unknown", Value: &value, Confidence: "low"},
		ChromaNoise: RestorationSignalEvidence{Availability: "ambiguous", Severity: "unknown", Value: &value, Confidence: "low"},
		Banding:     unavailableRestorationSignal("No canonical banding metric exists."),
		Ringing:     unavailableRestorationSignal("No canonical ringing metric exists."),
		EdgeDetail:  unavailableRestorationSignal("No canonical detail metric exists."),
	}
	scan := recommendationScan(InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", Confidence: .95}, CadenceAnalysis{}, CadenceRecommendation{})
	scan.RestorationAnalysis = recommendationJSONMap(analysis)
	plan := buildRestorationRecommendationPlan(scan, proposedProfileForScan(scan), false)

	for _, id := range []string{"deblock", "denoise", "chroma_nr"} {
		item := recommendationByID(t, plan, id)
		if item.State != RestorationRecommendationManualReview || item.RecommendedValue != "" || len(item.Patch) != 0 {
			t.Fatalf("%s must remain manual review without a preset: %#v", id, item)
		}
	}
	for _, id := range []string{"deband", "ringing", "edge_detail"} {
		item := recommendationByID(t, plan, id)
		if item.State != RestorationRecommendationNone || item.RecommendedValue != "" || len(item.Patch) != 0 {
			t.Fatalf("%s must distinguish unavailable from Off: %#v", id, item)
		}
	}
	if got := plan.RestorationEvidence.Blocking.Value; got == nil || *got != value || plan.RestorationEvidence.Windows != 3 || plan.RestorationEvidence.SampledFrames != 9 {
		t.Fatalf("calibration evidence was not preserved: %#v", plan.RestorationEvidence)
	}
}

func TestApplyRestorationRecommendationsOnlyAppliesActionableSelection(t *testing.T) {
	plan := RestorationRecommendationPlan{Recommendations: []RestorationRecommendation{
		{ID: "frame_structure", State: RestorationRecommendationRecommended, Patch: models.JSONMap{"deinterlaceMode": "force"}},
		{ID: "upscale", State: RestorationRecommendationRecommended, Patch: models.JSONMap{"upscaleMode": "auto"}},
		{ID: "deblock", State: RestorationRecommendationManualReview, Patch: models.JSONMap{"deblock": "medium"}},
		{ID: "deband", State: RestorationRecommendationNone, Patch: models.JSONMap{"deband": "off"}},
	}}
	current := models.JSONMap{"deinterlaceMode": "off", "upscaleMode": "disabled", "deblock": "custom"}
	before := cloneWorkerConfig(current)

	all := applyRestorationRecommendations(current, plan, nil)
	if all["deinterlaceMode"] != "force" || all["upscaleMode"] != "auto" || all["deblock"] != "custom" {
		t.Fatalf("Apply All changed a non-actionable domain: %#v", all)
	}
	selected := applyRestorationRecommendations(current, plan, []string{"upscale", "deblock"})
	if selected["upscaleMode"] != "auto" || selected["deinterlaceMode"] != "off" || selected["deblock"] != "custom" {
		t.Fatalf("Apply Selected ignored actionable/state boundaries: %#v", selected)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatalf("recommendation application mutated the source config: got=%#v want=%#v", current, before)
	}
}

func TestRecommendationGenerationDoesNotMutateProfileAndQueueLockIsPresentationOnly(t *testing.T) {
	scan := recommendationScan(InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", Confidence: .95}, CadenceAnalysis{}, CadenceRecommendation{})
	proposal := proposedProfileForScan(scan)
	before := cloneWorkerConfig(proposal.WorkerConfig)
	plan := buildRestorationRecommendationPlan(scan, proposal, true)
	if !reflect.DeepEqual(proposal.WorkerConfig, before) {
		t.Fatalf("plan generation mutated profile request: got=%#v want=%#v", proposal.WorkerConfig, before)
	}
	if !plan.ApplyLocked || plan.ApplyLockReason == "" || len(plan.Recommendations) == 0 {
		t.Fatalf("locked plan should stay visible and explain why applying is disabled: %#v", plan)
	}
}

func recommendationScan(interlace InterlaceAnalysis, cadence CadenceAnalysis, cadencePlan CadenceRecommendation) models.ScanResult {
	return models.ScanResult{
		Path: "/media/raw/test.mkv", VideoCodec: "mpeg2video", Width: 720, Height: 480,
		VideoStreams:           models.JSONList{map[string]any{"width": 720, "height": 480, "sampleAspectRatio": "32:27", "displayAspectRatio": "16:9", "avgFrameRate": "30000/1001"}},
		FrameStructureAnalysis: models.JSONMap{"version": 2, "confidence": "high"},
		InterlaceAnalysis:      recommendationJSONMap(interlace), CadenceAnalysis: recommendationJSONMap(cadence), CadenceRecommendation: recommendationJSONMap(cadencePlan),
	}
}

func recommendationJSONMap(value any) models.JSONMap {
	encoded, _ := json.Marshal(value)
	result := models.JSONMap{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func recommendationByID(t *testing.T, plan RestorationRecommendationPlan, id string) RestorationRecommendation {
	t.Helper()
	for _, recommendation := range plan.Recommendations {
		if recommendation.ID == id {
			return recommendation
		}
	}
	t.Fatalf("recommendation %q not found: %#v", id, plan.Recommendations)
	return RestorationRecommendation{}
}
