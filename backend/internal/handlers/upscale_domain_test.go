package handlers

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestParseUpscaleRequestDefaultsLegacyProfilesToDisabled(t *testing.T) {
	legacy := models.JSONMap{"videoEncoder": "hevc_qsv"}
	request, err := parseUpscaleRequest(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if request.Mode != UpscaleModeDisabled || request.Sharpen != UpscaleSharpenOff || len(legacy) != 1 {
		t.Fatalf("legacy request changed: request=%#v config=%#v", request, legacy)
	}
}

func TestAssetUpscaleOverrideAppliesOnlyExplicitDimensions(t *testing.T) {
	base := models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "auto", "upscaleSharpen": "off", "upscaleCustomHeight": 900}}
	custom := applyAssetConversionOverrideToProfile(base, AssetConversionOverrideState{UpscaleMode: "custom", UpscaleSharpen: "medium", UpscaleCustomHeight: 720})
	request, err := parseUpscaleRequest(custom.WorkerConfig)
	if err != nil {
		t.Fatal(err)
	}
	if request.Mode != UpscaleModeCustom || request.Sharpen != UpscaleSharpenMedium || request.CustomHeight != 720 {
		t.Fatalf("custom override not applied: %#v", request)
	}

	disabled := applyAssetConversionOverrideToProfile(base, AssetConversionOverrideState{UpscaleMode: "disabled"})
	if _, exists := disabled.WorkerConfig["upscaleCustomHeight"]; exists {
		t.Fatalf("explicit non-custom override retained stale custom height: %#v", disabled.WorkerConfig)
	}
	if disabled.WorkerConfig["upscaleSharpen"] != "off" {
		t.Fatalf("omitted sharpen should inherit: %#v", disabled.WorkerConfig)
	}
}

func TestResolveUpscaleGeometryPreservesAnamorphicDAR(t *testing.T) {
	tests := []struct {
		name       string
		stream     MediaStream
		filters    string
		mode       UpscaleMode
		custom     int
		wantWidth  int
		wantHeight int
	}{
		{name: "NTSC anamorphic 16:9", stream: MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9"}, mode: UpscaleMode720p, wantWidth: 1280, wantHeight: 720},
		{name: "NTSC 4:3", stream: MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3"}, mode: UpscaleMode720p, wantWidth: 960, wantHeight: 720},
		{name: "PAL anamorphic 16:9", stream: MediaStream{Width: 720, Height: 576, SampleAspectRatio: "64:45", DisplayAspectRatio: "16:9"}, mode: UpscaleMode720p, wantWidth: 1280, wantHeight: 720},
		{name: "crop changes effective DAR", stream: MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3"}, filters: "crop=704:448:8:16", mode: UpscaleMode720p, wantWidth: 1006, wantHeight: 720},
		{name: "odd calculated width rounds even", stream: MediaStream{Width: 853, Height: 480, SampleAspectRatio: "1:1", DisplayAspectRatio: "853:480"}, mode: UpscaleMode720p, wantWidth: 1280, wantHeight: 720},
		{name: "explicit 1080p", stream: MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9"}, mode: UpscaleMode1080p, wantWidth: 1920, wantHeight: 1080},
		{name: "custom height", stream: MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3"}, mode: UpscaleModeCustom, custom: 900, wantWidth: 1200, wantHeight: 900},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := models.JSONMap{"upscaleMode": string(test.mode), "upscaleSharpen": "medium", "videoFilters": test.filters}
			if test.custom > 0 {
				config["upscaleCustomHeight"] = test.custom
			}
			resolved := resolveUpscaleProfile(models.Profile{WorkerConfig: config}, MediaStreamInventory{Video: []MediaStream{test.stream}}, UpscaleAnalysisEvidence{})
			decision, ok := resolvedUpscaleDecisionFromProfile(resolved)
			if !ok || !decision.UpscaleApplied || decision.TargetWidth != test.wantWidth || decision.TargetHeight != test.wantHeight || decision.TargetSAR != "1:1" || decision.SharpenMode != UpscaleSharpenMedium {
				t.Fatalf("decision=%#v want %dx%d square-pixel medium", decision, test.wantWidth, test.wantHeight)
			}
			if decision.TargetWidth%2 != 0 || decision.TargetHeight%2 != 0 {
				t.Fatalf("target is not encoder-safe even geometry: %#v", decision)
			}
		})
	}
}

func TestResolveUpscaleKeepsSourceForVideoCopyWithoutRewritingRequest(t *testing.T) {
	tests := []struct {
		name   string
		config models.JSONMap
	}{
		{name: "auto", config: models.JSONMap{"upscaleMode": "auto", "upscaleSharpen": "light"}},
		{name: "1080p", config: models.JSONMap{"upscaleMode": "1080p", "upscaleSharpen": "medium"}},
		{name: "custom", config: models.JSONMap{"upscaleMode": "custom", "upscaleSharpen": "light", "upscaleCustomHeight": 900}},
	}
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9"}}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "copy", WorkerConfig: cloneWorkerConfig(test.config)}
			resolved := resolveUpscaleProfile(profile, streams, UpscaleAnalysisEvidence{FrameStructureAvailable: true})
			decision, ok := resolvedUpscaleDecisionFromProfile(resolved)
			if !ok || decision.ResolvedMode != ResolvedUpscaleKeepSource || decision.UpscaleApplied || decision.SharpenMode != UpscaleSharpenOff {
				t.Fatalf("copy decision=%#v", decision)
			}
			if !reflect.DeepEqual(profile.WorkerConfig, test.config) {
				t.Fatalf("request mutated: got %#v want %#v", profile.WorkerConfig, test.config)
			}
			if len(decision.Reasons) != 1 || decision.Reasons[0] != "keep_source_video_copy" || len(decision.Warnings) != 1 {
				t.Fatalf("copy evidence=%#v", decision)
			}
		})
	}
}

func TestResolveUpscalePreservesFrozenDecisionBeforeVideoCopyGuard(t *testing.T) {
	frozen := ResolvedUpscaleDecision{RequestedMode: UpscaleMode1080p, ResolvedMode: ResolvedUpscale1080p, SourceWidth: 720, SourceHeight: 480, TargetWidth: 1920, TargetHeight: 1080, TargetSAR: "1:1", UpscaleApplied: true, SharpenMode: UpscaleSharpenLight, Confidence: UpscaleConfidenceMedium}
	profile := models.Profile{VideoCodec: "copy", WorkerConfig: models.JSONMap{"upscaleMode": "disabled", "resolvedUpscaleDecision": frozen}}
	resolved := resolveUpscaleProfile(profile, MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480}}}, UpscaleAnalysisEvidence{})
	got, ok := resolvedUpscaleDecisionFromProfile(resolved)
	if !ok || !reflect.DeepEqual(*got, frozen) {
		t.Fatalf("frozen decision was re-resolved: got=%#v want=%#v", got, frozen)
	}
}

func TestResolveUpscaleExplicitDisabledAndNonUpscaleTarget(t *testing.T) {
	stream := MediaStream{Width: 1280, Height: 720, SampleAspectRatio: "1:1", DisplayAspectRatio: "16:9"}
	disabled := resolveUpscaleProfile(models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "disabled"}}, MediaStreamInventory{Video: []MediaStream{stream}}, UpscaleAnalysisEvidence{})
	decision, ok := resolvedUpscaleDecisionFromProfile(disabled)
	if !ok || decision.UpscaleApplied || decision.TargetWidth != 1280 || decision.TargetHeight != 720 {
		t.Fatalf("disabled decision=%#v", decision)
	}
	lower := resolveUpscaleProfile(models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "720p"}}, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, SampleAspectRatio: "1:1", DisplayAspectRatio: "16:9"}}}, UpscaleAnalysisEvidence{})
	lowerDecision, _ := resolvedUpscaleDecisionFromProfile(lower)
	if lowerDecision.UpscaleApplied || len(lowerDecision.Warnings) == 0 {
		t.Fatalf("lower target was silently treated as upscale: %#v", lowerDecision)
	}
}

func TestResolveUpscaleAutoPolicy(t *testing.T) {
	reliableProfile := models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "auto", "upscaleSharpen": "light", "effectiveOutputProgressive": true}}
	sd := MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9"}}}
	reliable := resolveUpscaleProfile(reliableProfile, sd, UpscaleAnalysisEvidence{FrameStructureAvailable: true})
	reliableDecision, _ := resolvedUpscaleDecisionFromProfile(reliable)
	if !reliableDecision.UpscaleApplied || reliableDecision.ResolvedMode != ResolvedUpscale720p || reliableDecision.TargetWidth != 1280 || reliableDecision.SharpenMode != UpscaleSharpenLight {
		t.Fatalf("reliable SD Auto=%#v", reliableDecision)
	}

	hd := resolveUpscaleProfile(reliableProfile, MediaStreamInventory{Video: []MediaStream{{Width: 1280, Height: 720, SampleAspectRatio: "1:1", DisplayAspectRatio: "16:9"}}}, UpscaleAnalysisEvidence{FrameStructureAvailable: true})
	hdDecision, _ := resolvedUpscaleDecisionFromProfile(hd)
	if hdDecision.UpscaleApplied || hdDecision.ResolvedMode != ResolvedUpscaleKeepSource {
		t.Fatalf("720p+ Auto=%#v", hdDecision)
	}

	unreliable := resolveUpscaleProfile(models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "auto"}}, sd, UpscaleAnalysisEvidence{})
	unreliableDecision, _ := resolvedUpscaleDecisionFromProfile(unreliable)
	if unreliableDecision.UpscaleApplied || len(unreliableDecision.Warnings) == 0 {
		t.Fatalf("unreliable Auto=%#v", unreliableDecision)
	}
}

func TestResolveUpscaleRunsAfterIVTCAndDeinterlace(t *testing.T) {
	stream := MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9", FrameRate: "30000/1001"}}}
	tests := []struct {
		name           string
		profile        models.Profile
		interlace      InterlaceAnalysis
		cadence        CadenceAnalysis
		recommendation CadenceRecommendation
	}{
		{
			name: "IVTC", profile: models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "auto", "cadenceMode": "auto"}},
			interlace:      InterlaceAnalysis{Status: "telecine", Confidence: .99},
			cadence:        CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "soft_telecine", Confidence: .99},
			recommendation: CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99},
		},
		{
			name: "deinterlace", profile: models.Profile{WorkerConfig: models.JSONMap{"upscaleMode": "auto", "fieldStructureMode": "deinterlace"}},
			interlace: InterlaceAnalysis{Status: "interlaced", Confidence: .99, DetectedFieldOrder: "tff"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			motion := resolveEffectiveVideoMotionProfile(test.profile, test.interlace, test.cadence, test.recommendation)
			resolved := resolveUpscaleProfile(motion, stream, UpscaleAnalysisEvidence{FrameStructureAvailable: true})
			decision, _ := resolvedUpscaleDecisionFromProfile(resolved)
			if !profileWorkerBool(motion, "effectiveOutputProgressive", false) || !decision.UpscaleApplied || decision.TargetWidth != 1280 || decision.TargetHeight != 720 {
				t.Fatalf("upscale did not consume effective progressive output: motion=%#v decision=%#v", motion.WorkerConfig, decision)
			}
		})
	}
}

func TestParseUpscaleRequestValidatesEnumsAndCustomHeight(t *testing.T) {
	tests := []struct {
		name   string
		config models.JSONMap
		valid  bool
	}{
		{name: "auto", config: models.JSONMap{"upscaleMode": "auto", "upscaleSharpen": "light"}, valid: true},
		{name: "custom even height", config: models.JSONMap{"upscaleMode": "custom", "upscaleSharpen": "medium", "upscaleCustomHeight": 900}, valid: true},
		{name: "invalid mode", config: models.JSONMap{"upscaleMode": "ai"}},
		{name: "invalid sharpen", config: models.JSONMap{"upscaleSharpen": "strong"}},
		{name: "missing custom height", config: models.JSONMap{"upscaleMode": "custom"}},
		{name: "odd custom height", config: models.JSONMap{"upscaleMode": "custom", "upscaleCustomHeight": 721}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseUpscaleRequest(test.config)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t err=%v", test.valid, err)
			}
		})
	}
}

func TestResolvedUpscaleDecisionSerializesInEffectiveVideoDecision(t *testing.T) {
	resolved := ResolvedUpscaleDecision{
		RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p,
		SourceWidth: 720, SourceHeight: 480, SourceSAR: "8:9", SourceDAR: "4:3",
		TargetWidth: 960, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
		SharpenMode: UpscaleSharpenLight, Confidence: UpscaleConfidenceHigh, Reasons: []string{"sd_source", "target_720p_recommended"}, Warnings: []string{},
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		t.Fatal(err)
	}
	var stored models.JSONMap
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": stored}}
	decision := effectiveVideoDecision(profile)
	upscale, ok := decision["upscale"].(*ResolvedUpscaleDecision)
	if !ok || !reflect.DeepEqual(*upscale, resolved) {
		t.Fatalf("resolved upscale decision did not serialize: %#v", decision["upscale"])
	}
	legacyDecision := effectiveVideoDecision(models.Profile{WorkerConfig: models.JSONMap{}})
	if _, exists := legacyDecision["upscale"]; exists {
		t.Fatalf("legacy profile fabricated a resolved decision: %#v", legacyDecision)
	}
}

func TestUpscaleAnalysisEvidenceReusesFactsAndMarksQualitySignalsUnavailable(t *testing.T) {
	scan := models.ScanResult{
		Width: 720, Height: 480, VideoCodec: "mpeg2video", Bitrate: 6_000_000,
		VideoStreams:           models.JSONList{map[string]interface{}{"sampleAspectRatio": "8:9", "displayAspectRatio": "4:3"}},
		CropAnalysis:           models.JSONMap{"status": "detected", "confidence": 0.96},
		FrameStructureAnalysis: models.JSONMap{"version": 2},
		InterlaceAnalysis:      models.JSONMap{"status": "progressive", "confidence": 0.99},
		CadenceAnalysis:        models.JSONMap{"type": "soft_telecine", "confidence": 0.98, "effectivePictureRate": "24000/1001"},
		CadenceRecommendation:  models.JSONMap{"operation": "remove_soft_telecine", "outputFrameRate": "24000/1001"},
	}
	evidence := upscaleAnalysisEvidence(scan)
	if evidence.SourceSAR != "8:9" || evidence.SourceDAR != "4:3" || !evidence.CropAvailable || !evidence.FrameStructureAvailable || !evidence.InterlaceAvailable || !evidence.CadenceAvailable || evidence.EffectiveFrameRate != "24000/1001" {
		t.Fatalf("existing snapshot facts were not reused: %#v", evidence)
	}
	if evidence.QualitySignals.Noise != "unavailable" || evidence.QualitySignals.Grain != "unavailable" || evidence.QualitySignals.Compression != "unavailable" || evidence.QualitySignals.Ringing != "unavailable" || evidence.QualitySignals.EdgeDetail != "unavailable" {
		t.Fatalf("missing quality signals were fabricated: %#v", evidence.QualitySignals)
	}
}
