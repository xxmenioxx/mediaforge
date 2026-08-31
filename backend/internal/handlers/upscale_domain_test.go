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
