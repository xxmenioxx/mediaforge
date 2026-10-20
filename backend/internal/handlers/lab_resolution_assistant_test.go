package handlers

import (
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestLabResolutionDiagnosticForAmbiguousIVTCKeepsBothGatesReviewable(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"cadenceMode": "auto", "fieldStructureMode": "auto", "cadenceFieldOrder": "auto",
		"frameStructureMode": "auto", "frameStructureGopMode": "auto", "effectiveOutputFrameRateUnknown": true,
	}}
	interlace := InterlaceAnalysis{
		Version: interlaceAnalysisVersion, Status: "telecine_suspected", Confidence: .72,
		IVTCValidation: &IVTCValidation{SelectedOrder: "", Confidence: .5},
	}
	cadence := CadenceAnalysis{
		Version: cadenceAnalysisVersion, Type: "hard_telecine", DeclaredFrameRate: "30000/1001",
		DeclaredFPS: 30000.0 / 1001.0, EffectiveFPS: 24000.0 / 1001.0,
		Confidence: .72, SampleCount: 3, AmbiguousSampleCount: 1, FrameRateMismatch: true,
	}
	recommendation := CadenceRecommendation{Version: 1, Operation: "inverse_telecine", OutputFrameRate: "24000/1001", Confidence: .72}
	stream := MediaStream{Width: 720, Height: 480, FieldOrder: "tt", FrameRate: "30000/1001", PixelFormat: "yuv420p"}
	decision := buildLabResolutionDiagnostic(nil, "/media/raw/sin-limite.mkv", profile, MediaStreamInventory{Video: []MediaStream{stream}}, interlace, cadence, recommendation, compatiblePreviewSourceRevision{Size: 1234, ModifiedAtNS: 99, Mode: 0o644})
	if decision.Status != "resolution_required" || decision.PlanningStatus != "NEEDS_REVIEW" || decision.Version != 1 {
		t.Fatalf("unexpected diagnostic status: %#v", decision)
	}
	if decision.Effective["frameRateKnown"].(bool) || decision.Effective["frameRate"] != nil {
		t.Fatalf("ambiguous cadence unexpectedly resolved effective FPS: %#v", decision.Effective)
	}
	if len(decision.BlockingFields) < 2 {
		t.Fatalf("expected separate Fidelity blockers and GOP blocker: %#v", decision.BlockingFields)
	}
	if decision.Actions[0].ID != "reanalyze_cadence" || decision.Actions[0].Eligible {
		t.Fatalf("Deep Cadence must be disabled without a current saved snapshot: %#v", decision.Actions)
	}
	if decision.Actions[1].ID != "preserve_source" || decision.Actions[1].Eligible {
		t.Fatalf("ambiguous hard telecine must not offer Preserve as supported: %#v", decision.Actions[1])
	}
	if !decision.Actions[2].Eligible {
		t.Fatalf("ambiguous field order should expose explicit comparison: %#v", decision.Actions[2])
	}
	if workerStringValue(profile.WorkerConfig["cadenceMode"]) != "auto" || workerStringValue(profile.WorkerConfig["fieldStructureMode"]) != "auto" {
		t.Fatalf("diagnostic mutated the requested profile: %#v", profile.WorkerConfig)
	}
}

func TestLabResolutionPreserveReportsUnverifiedFieldStructureAuthority(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"cadenceMode": "preserve", "fieldStructureMode": "auto", "frameStructureMode": "off",
		"effectiveOutputFrameRate": "30000/1001", "effectiveOutputProgressiveUnreliable": true,
	}}
	stream := MediaStream{Width: 720, Height: 480, FieldOrder: "tt", FrameRate: "30000/1001", PixelFormat: "yuv420p"}
	source := canonicalFidelitySource{effective: models.JSONMap{
		"width": stream.Width, "height": stream.Height, "sampleAspectRatio": "", "displayAspectRatio": "",
		"frameRate": stream.FrameRate, "fieldStructure": normalizedFieldStructure(stream.FieldOrder),
		"pixelFormat": stream.PixelFormat, "bitDepth": bitDepthFromPixelFormat(stream.PixelFormat),
	}}
	interlace := InterlaceAnalysis{Status: "telecine_suspected", Confidence: .7}
	cadence := CadenceAnalysis{Type: "hard_telecine", FrameRateMismatch: true}
	recommendation := CadenceRecommendation{Operation: "inverse_telecine", OutputFrameRate: "24000/1001"}
	effective, err := resolvePreviewVideoProfile(nil, "/media/raw/preserve.mkv", profile, MediaStreamInventory{Video: []MediaStream{stream}}, interlace, cadence, recommendation)
	if err != nil {
		t.Fatalf("preserve Preview plan should prepare for Fidelity inspection: %v", err)
	}
	if workerStringValue(effective.WorkerConfig["cadenceMode"]) != "preserve" || strings.Contains(workerStringValue(effective.WorkerConfig["videoFilters"]), "fieldmatch=") {
		t.Fatalf("frozen preview did not preserve cadence: %#v", effective.WorkerConfig)
	}
	expectations, _ := fidelityExpectedOutput(effective, source)
	frameRate := expectations["frameRate"]
	fieldStructure := expectations["fieldStructure"]
	if frameRate.state != fidelityAuthorityPreserve || frameRate.value != "30000/1001" {
		t.Fatalf("explicit cadence Preserve should preserve the supported source rate: %#v", frameRate)
	}
	if fieldStructure.state != fidelityAuthorityUnresolved || !fieldStructure.unreliable || fieldStructure.value != nil {
		t.Fatalf("cadence Preserve alone must not establish field-structure authority: %#v", fieldStructure)
	}
}

func TestLabResolutionUnsupportedFPSTransformStaysBlocked(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"cadenceMode": "auto", "fieldStructureMode": "preserve", "frameStructureMode": "auto",
		"videoFilters": "fps=not-a-rate", "effectiveOutputFrameRateUnknown": true,
	}}
	interlace := InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "interlaced", Confidence: .99}
	cadence := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "interlaced", Confidence: .99, DeclaredFPS: 25, EffectiveFPS: 25}
	recommendation := CadenceRecommendation{Version: 1, Operation: "preserve", Confidence: .99}
	diagnostic := buildLabResolutionDiagnostic(nil, "/media/raw/unsupported.mkv", profile, MediaStreamInventory{Video: []MediaStream{{Width: 640, Height: 480, FrameRate: "25/1", FieldOrder: "tt"}}}, interlace, cadence, recommendation, compatiblePreviewSourceRevision{Size: 100, ModifiedAtNS: 10})
	if diagnostic.Effective["frameRateKnown"].(bool) {
		t.Fatalf("unsupported FPS transform was inferred: %#v", diagnostic.Effective)
	}
	if diagnostic.Actions[1].Eligible {
		t.Fatalf("Preserve must not mask an unsupported explicit FPS transform: %#v", diagnostic.Actions[1])
	}
}

func TestAutomaticGOPResolutionUsesTypedBackwardCompatibleError(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"frameStructureMode": "auto", "frameStructureGopMode": "auto", "effectiveOutputFrameRateUnknown": true,
	}}
	_, err := resolveAutomaticFrameStructure(nil, "/media/raw/ambiguous.mkv", profile)
	if !isFrameStructureResolutionRequired(err) {
		t.Fatalf("expected typed resolution-required error, got %T: %v", err, err)
	}
	if err.Error() != "frame structure GOP resolution requires a known effective output frame rate after an explicit FPS transform" {
		t.Fatalf("legacy error text changed: %v", err)
	}
}

func TestCompatiblePreviewResolutionPayloadKeepsLegacyErrorAndAddsEnvelope(t *testing.T) {
	diagnostic := &labResolutionDiagnostic{Version: labResolutionDiagnosticVersion, Status: "resolution_required", PlanningStatus: "NEEDS_REVIEW"}
	prepareErr := &compatiblePreviewPrepareError{status: 422, err: &frameStructureResolutionRequiredError{}, resolution: diagnostic}
	payload := compatiblePreviewPreparePayload(prepareErr)
	if payload["error"] != prepareErr.Error() || payload["resolution"] != diagnostic || compatiblePreviewPrepareStatus(prepareErr) != 422 {
		t.Fatalf("resolution response changed legacy error/status fields: %#v", payload)
	}
}

func TestLabResolutionMixedCadenceDoesNotRecommendPreserve(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"cadenceMode": "auto", "fieldStructureMode": "auto", "frameStructureMode": "auto",
		"effectiveOutputFrameRateUnknown": true,
	}}
	interlace := InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "interlaced", Confidence: .99}
	cadence := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "mixed", Confidence: .75, DeclaredFPS: 30000.0 / 1001.0, EffectiveFPS: 24, FrameRateMismatch: true}
	diagnostic := buildLabResolutionDiagnostic(nil, "/media/raw/mixed.mkv", profile,
		MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, FrameRate: "30000/1001", FieldOrder: "tt"}}},
		interlace, cadence, CadenceRecommendation{Version: 1, Operation: "review"}, compatiblePreviewSourceRevision{Size: 100, ModifiedAtNS: 10})
	if diagnostic.Actions[1].Eligible || diagnostic.Actions[1].Recommended {
		t.Fatalf("mixed cadence must remain review-blocked: %#v", diagnostic.Actions[1])
	}
}
