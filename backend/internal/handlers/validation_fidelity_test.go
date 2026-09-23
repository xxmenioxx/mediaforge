package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestCanonicalFrameFidelityIVTCSeparatesIntentFromValidation(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"effectiveOutputFrameRate":         "24000/1001",
		"effectiveOutputProgressive":       true,
		"effectiveCadenceOperation":        "ivtc",
		"effectiveCadenceFieldOrder":       "tff",
		"effectiveResidualCombingFallback": "selective",
		"effectiveAutomaticMotionFilters":  "fieldmatch=order=tff,bwdif=mode=send_field",
	}}
	source := fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt")
	output := fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive")
	report := runCanonicalFidelity(t, profile, source, output, nil)
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" {
		t.Fatalf("resolved IVTC output rejected: %#v", report)
	}
	if fields["frameRate"].(models.JSONMap)["status"] != "passed" || fields["fieldStructure"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("IVTC fields were not validated against the plan: %#v", fields)
	}
	if fields["frameRate"].(models.JSONMap)["changed_intentionally"] != true {
		t.Fatalf("source to expected IVTC change was not reported: %#v", fields["frameRate"])
	}

	notHonored := runCanonicalFidelity(t, profile, source, fidelityFrameProbe("h264", "yuv420p", "30000/1001", "tt"), nil)
	badFields := notHonored["fields"].(models.JSONMap)
	if notHonored["status"] != "mismatch" || badFields["frameRate"].(models.JSONMap)["status"] != "mismatch" || badFields["fieldStructure"].(models.JSONMap)["status"] != "mismatch" {
		t.Fatalf("unhonored IVTC output was not rejected: %#v", notHonored)
	}
}

func TestCanonicalFrameFidelityQSVWorkingFormatUsesObservableSemantics(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder":               "hevc_qsv",
		"pixFmt":                     "p010le",
		"effectiveOutputFrameRate":   "24/1",
		"effectiveOutputProgressive": true,
		"hevcLevelEffective":         "4.0",
		"hevcLevelTier":              "main",
	}}
	source := fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "progressive")
	output := fidelityFrameProbe("hevc", "yuv420p10le", "24/1", "progressive")
	output["streams"].([]interface{})[0].(map[string]any)["profile"] = "Main 10"
	report := runCanonicalFidelity(t, profile, source, output, testHEVCBitstreamEvidence("4.0", "main"))
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["pixelFormat"].(models.JSONMap)["status"] != "passed" || fields["bitDepth"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("QSV p010le/yuv420p10le semantics rejected: %#v", report)
	}
	if report["workingPixelFormat"] != "p010le" || output["streams"].([]interface{})[0].(map[string]any)["pix_fmt"] != "yuv420p10le" {
		t.Fatalf("working/output pixel spellings were not retained diagnostically: %#v", report)
	}
}

func TestCanonicalFrameFidelityQSVMain10RejectsEightBitOutput(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "pixFmt": "p010le", "hevcLevelEffective": "4.0", "hevcLevelTier": "main",
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "progressive"), fidelityFrameProbe("hevc", "yuv420p", "24/1", "progressive"), testHEVCBitstreamEvidence("4.0", "main"))
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "mismatch" || fields["pixelFormat"].(models.JSONMap)["status"] != "mismatch" || fields["bitDepth"].(models.JSONMap)["status"] != "mismatch" {
		t.Fatalf("eight-bit output passed a frozen Main10 plan: %#v", report)
	}
}

func TestCanonicalFrameFidelitySoftwareTenBitKeepsExactObservableFormat(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "pixFmt": "yuv420p10le", "hevcLevelEffective": "4.0", "hevcLevelTier": "main",
	}}
	output := fidelityFrameProbe("hevc", "yuv420p10le", "24/1", "progressive")
	output["streams"].([]interface{})[0].(map[string]any)["profile"] = "Main 10"
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "progressive"), output, testHEVCBitstreamEvidence("4.0", "main"))
	if report["fields"].(models.JSONMap)["pixelFormat"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("software observable format did not pass: %#v", report)
	}
}

func TestCanonicalFrameFidelityOptionalChromaEvidenceDoesNotPoisonAggregate(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"pixFmt": "yuv420p", "effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true}}
	source := fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive")
	output := fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive")
	delete(source["streams"].([]interface{})[0].(map[string]any), "chroma_location")
	delete(output["streams"].([]interface{})[0].(map[string]any), "chroma_location")
	report := runCanonicalFidelity(t, profile, source, output, nil)
	if report["status"] != "passed" {
		t.Fatalf("missing optional chroma evidence poisoned aggregate: %#v", report)
	}
	if report["fields"].(models.JSONMap)["chromaLocation"].(models.JSONMap)["status"] != "not_applicable" {
		t.Fatalf("missing chroma evidence was not marked optional: %#v", report)
	}
}

func TestCanonicalFrameFidelityUsesMeasuredFPSBeforeAverageRate(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "24000/1001"}}
	output := fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive")
	stream := output["streams"].([]interface{})[0].(map[string]any)
	stream["nb_read_frames"] = "240"
	stream["duration"] = "10.01"
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "progressive"), output, nil)
	field := report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)
	if field["status"] != "passed" || field["actual"] == "30000/1001" {
		t.Fatalf("measured FPS did not outrank avg_frame_rate: %#v", field)
	}
}

func TestCanonicalFrameFidelityRFrameRateAloneIsUnverified(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "24000/1001"}}
	output := fidelityFrameProbe("h264", "yuv420p", "", "progressive")
	output["streams"].([]interface{})[0].(map[string]any)["r_frame_rate"] = "24000/1001"
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "progressive"), output, nil)
	if report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)["status"] != "unverified" || report["status"] != "unverified" {
		t.Fatalf("r_frame_rate was treated as authoritative: %#v", report)
	}
}

func TestCanonicalFrameFidelityFilterTextCannotOverrideFrozenGeometry(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"pixFmt": "yuv420p", "videoFilters": "scale=1280:720,fps=24000/1001", "effectiveOutputFrameRate": "30000/1001", "effectiveOutputProgressive": true}}
	output := fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive")
	output["streams"].([]interface{})[0].(map[string]any)["width"] = 1280
	output["streams"].([]interface{})[0].(map[string]any)["height"] = 720
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "progressive"), output, nil)
	fields := report["fields"].(models.JSONMap)
	for _, name := range []string{"width", "height", "sampleAspectRatio", "displayAspectRatio", "frameSize"} {
		field := fields[name].(models.JSONMap)
		if field["status"] != "not_applicable" {
			t.Fatalf("filter text or source invented geometry authority for %s: %#v", name, field)
		}
		if name == "frameSize" {
			expected := field["expected"].(models.JSONMap)
			if expected["width"] != nil || expected["height"] != nil {
				t.Fatalf("filter text or source invented composite geometry authority: %#v", field)
			}
		} else if field["expected"] != nil {
			t.Fatalf("filter text or source invented geometry authority for %s: %#v", name, field)
		}
	}
	if report["status"] != "passed" {
		t.Fatalf("unresolved filter-only geometry poisoned otherwise valid fields: %#v", report)
	}
}

func TestCanonicalFrameFidelityExplicitGeometryPreserveUsesSource(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "cadenceMode": "preserve", "fieldStructureMode": "preserve",
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{
			RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscaleKeepSource,
			SourceWidth: 720, SourceHeight: 480, SourceSAR: "32:27", SourceDAR: "16:9",
			TargetWidth: 720, TargetHeight: 480, TargetSAR: "32:27", UpscaleApplied: false,
		},
	}}
	source := fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive")
	report := runCanonicalFidelity(t, profile, source, fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), nil)
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["width"].(models.JSONMap)["status"] != "passed" || fields["width"].(models.JSONMap)["authorityState"] != "preserve" {
		t.Fatalf("explicit geometry preserve was not honored: %#v", report)
	}
}

func TestCanonicalFrameFidelityMissingFPSAuthorityDoesNotPreserveSource(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"pixFmt": "yuv420p", "effectiveOutputProgressive": true}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive"), nil)
	field := report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)
	if report["status"] != "unverified" || field["status"] != "unverified" || field["expected"] != nil {
		t.Fatalf("missing FPS authority silently preserved source FPS: %#v", field)
	}
}

func TestCanonicalFrameFidelityCadencePreserveUsesSourceFPS(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"pixFmt": "yuv420p", "cadenceMode": "preserve"}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), nil)
	field := report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)
	if field["status"] != "passed" || field["expected"] != "30000/1001" || field["authorityState"] != "preserve" {
		t.Fatalf("explicit cadence preserve did not authorize source FPS: %#v", field)
	}
}

func TestCanonicalFrameFidelityFieldPreserveDoesNotPreserveFPS(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "fieldStructureMode": "preserve", "cadenceMode": "auto",
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "24000/1001", "tt"), nil)
	fields := report["fields"].(models.JSONMap)
	fps := fields["frameRate"].(models.JSONMap)
	structure := fields["fieldStructure"].(models.JSONMap)
	if fps["status"] != "unverified" || fps["expected"] != nil || structure["status"] != "passed" || structure["authorityState"] != "preserve" {
		t.Fatalf("field-structure preserve leaked into cadence or failed to preserve fields: %#v", fields)
	}
}

func TestCanonicalFrameFidelityIVTCOverridesFieldPreserve(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "fieldStructureMode": "preserve", "cadenceMode": "inverse_telecine",
		"effectiveCadenceOperation": "inverse_telecine", "effectiveOutputFrameRate": "24000/1001",
		"effectiveOutputProgressive": true,
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive"), nil)
	fields := report["fields"].(models.JSONMap)
	if fields["frameRate"].(models.JSONMap)["expected"] != "24000/1001" || fields["fieldStructure"].(models.JSONMap)["expected"] != "progressive" || report["status"] != "passed" {
		t.Fatalf("resolved IVTC output was overridden by field preserve: %#v", report)
	}
}

func TestCanonicalFrameFidelityCadencePreserveDoesNotPreserveFieldStructure(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "cadenceMode": "preserve", "fieldStructureMode": "deinterlace",
		"effectiveOutputProgressive": true,
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), nil)
	fields := report["fields"].(models.JSONMap)
	if fields["frameRate"].(models.JSONMap)["status"] != "passed" || fields["fieldStructure"].(models.JSONMap)["expected"] != "progressive" || fields["fieldStructure"].(models.JSONMap)["authorityState"] != "transform" {
		t.Fatalf("cadence preserve leaked into field structure: %#v", fields)
	}
}

func TestCanonicalFrameFidelityCadencePreserveWithUnknownFieldAuthority(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "cadenceMode": "preserve", "fieldStructureMode": "auto",
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "30000/1001", "tt"), nil)
	fields := report["fields"].(models.JSONMap)
	if fields["frameRate"].(models.JSONMap)["status"] != "passed" || fields["fieldStructure"].(models.JSONMap)["status"] != "unverified" || fields["fieldStructure"].(models.JSONMap)["expected"] != nil {
		t.Fatalf("cadence preserve borrowed authority for unknown field structure: %#v", fields)
	}
}

func TestCanonicalFrameFidelityExplicitFieldPreserveUsesSourceStructure(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "fieldStructureMode": "preserve", "cadenceMode": "auto",
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "30000/1001", "tt"), nil)
	field := report["fields"].(models.JSONMap)["fieldStructure"].(models.JSONMap)
	if field["status"] != "passed" || field["expected"] != "interlaced" || field["authorityState"] != "preserve" {
		t.Fatalf("explicit field-structure preserve did not use source structure: %#v", field)
	}
}

func TestCanonicalFrameFidelityUnknownCadenceIsUnverified(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "effectiveOutputFrameRateUnknown": true, "effectiveOutputProgressive": true,
	}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), nil)
	field := report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)
	if report["status"] != "unverified" || field["status"] != "unverified" || field["expected"] != nil {
		t.Fatalf("unknown cadence became an implicit preserve: %#v", field)
	}
}

func TestCanonicalFrameFidelityProgressiveKnownnessIsNotDefaulted(t *testing.T) {
	tests := []struct {
		name       string
		worker     models.JSONMap
		fieldOrder string
		want       string
	}{
		{name: "explicit true", worker: models.JSONMap{"effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true, "pixFmt": "yuv420p"}, fieldOrder: "progressive", want: "passed"},
		{name: "explicit false", worker: models.JSONMap{"effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": false, "pixFmt": "yuv420p"}, fieldOrder: "tt", want: "passed"},
		{name: "absent", worker: models.JSONMap{"effectiveOutputFrameRate": "24/1", "pixFmt": "yuv420p"}, fieldOrder: "progressive", want: "unverified"},
		{name: "unreliable", worker: models.JSONMap{"effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true, "effectiveOutputProgressiveUnreliable": true, "pixFmt": "yuv420p"}, fieldOrder: "progressive", want: "unverified"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "h264", WorkerConfig: test.worker}
			source := fidelityFrameProbe("h264", "yuv420p", "24/1", test.fieldOrder)
			report := runCanonicalFidelity(t, profile, source, fidelityFrameProbe("h264", "yuv420p", "24/1", test.fieldOrder), nil)
			field := report["fields"].(models.JSONMap)["fieldStructure"].(models.JSONMap)
			if field["status"] != test.want {
				t.Fatalf("progressive knownness was collapsed: %#v", field)
			}
		})
	}
}

func TestCanonicalFrameFidelityPixelAuthorityAbsenceDoesNotPreserveSource(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive"), fidelityFrameProbe("h264", "yuv420p10le", "24/1", "progressive"), nil)
	fields := report["fields"].(models.JSONMap)
	for _, name := range []string{"pixelFormat", "bitDepth"} {
		field := fields[name].(models.JSONMap)
		if field["status"] != "not_applicable" || field["expected"] != nil {
			t.Fatalf("source pixel semantics became an implicit expectation for %s: %#v", name, field)
		}
	}
}

func TestCanonicalFrameFidelityExplicitPixelPreserveUsesSourceSemantics(t *testing.T) {
	profile := models.Profile{VideoCodec: "copy", WorkerConfig: models.JSONMap{"cadenceMode": "preserve"}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive"), fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive"), nil)
	fields := report["fields"].(models.JSONMap)
	if fields["pixelFormat"].(models.JSONMap)["status"] != "passed" || fields["bitDepth"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("explicit pixel preserve did not compare source semantics: %#v", fields)
	}
}

func TestCanonicalFrameFidelityHEVCCodecAliasesUseFrozenLevelTier(t *testing.T) {
	for _, codec := range []string{"x265", "x265_10bit", "hevc", "hevc_10bit"} {
		t.Run(codec, func(t *testing.T) {
			tenBit := strings.Contains(codec, "10bit")
			pixel := "yuv420p"
			profileName := "Main"
			if tenBit {
				pixel, profileName = "yuv420p10le", "Main 10"
			}
			profile := models.Profile{VideoCodec: codec, WorkerConfig: models.JSONMap{
				"videoEncoder": "libx265", "pixFmt": pixel, "effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true,
				"hevcLevelEffective": "4.0", "hevcLevelTier": "main",
			}}
			output := fidelityFrameProbe("hevc", pixel, "24/1", "progressive")
			output["streams"].([]interface{})[0].(map[string]any)["profile"] = profileName
			report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "progressive"), output, testHEVCBitstreamEvidence("4.0", "main"))
			fields := report["fields"].(models.JSONMap)
			if fields["hevcLevel"].(models.JSONMap)["status"] == "not_applicable" || fields["hevcTier"].(models.JSONMap)["status"] == "not_applicable" {
				t.Fatalf("%s was not recognized as HEVC: %#v", codec, fields)
			}
		})
	}
}

func TestCanonicalFrameFidelityX265TenBitQSVUsesMain10Semantics(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265_10bit", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "pixFmt": "p010le", "effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true,
		"hevcLevelEffective": "4.0", "hevcLevelTier": "main",
	}}
	output := fidelityFrameProbe("hevc", "yuv420p10le", "24/1", "progressive")
	output["streams"].([]interface{})[0].(map[string]any)["profile"] = "Main 10"
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "progressive"), output, testHEVCBitstreamEvidence("4.0", "main"))
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["pixelFormat"].(models.JSONMap)["status"] != "passed" || fields["bitDepth"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("x265_10bit QSV Main10 semantics were not accepted: %#v", report)
	}
}

func TestCanonicalFrameFidelitySmartUpscaleUsesResolvedGeometry(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true,
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{
			RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p,
			SourceWidth: 720, SourceHeight: 480, SourceSAR: "32:27", SourceDAR: "16:9",
			TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
		},
	}}
	output := fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive")
	outputStream := output["streams"].([]interface{})[0].(map[string]any)
	outputStream["width"], outputStream["height"] = 1280, 720
	outputStream["sample_aspect_ratio"], outputStream["display_aspect_ratio"] = "1:1", "16:9"
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "tt"), output, nil)
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["width"].(models.JSONMap)["authority"] != "resolved_upscale" || fields["displayAspectRatio"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("resolved Smart Upscale geometry was not canonical: %#v", report)
	}
}

func TestCanonicalFrameFidelityRestorationUsesResolvedOutputGeometry(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "effectiveOutputFrameRate": "24/1", "effectiveOutputProgressive": true,
		"resolvedRestorationPlan": ResolvedRestorationPlan{
			Version:             1,
			SourceStorage:       RestorationGeometry{Width: 720, Height: 480, SAR: "32:27", DAR: "16:9"},
			EffectiveGeometry:   RestorationGeometry{Width: 704, Height: 448, SAR: "40:33", DAR: "40:21"},
			ResolvedOutput:      RestorationGeometry{Width: 1280, Height: 720, SAR: "1:1", DAR: "16:9"},
			RequiresVideoEncode: true, Executable: true,
		},
	}}
	output := fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive")
	outputStream := output["streams"].([]interface{})[0].(map[string]any)
	outputStream["width"], outputStream["height"] = 1280, 720
	outputStream["sample_aspect_ratio"], outputStream["display_aspect_ratio"] = "1:1", "16:9"
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("mpeg2video", "yuv420p", "24/1", "tt"), output, nil)
	if report["status"] != "passed" || report["fields"].(models.JSONMap)["width"].(models.JSONMap)["authority"] != "resolved_restoration" {
		t.Fatalf("resolved restoration geometry was not canonical: %#v", report)
	}
}

func TestCanonicalFidelitySurfacePlansShareEvaluator(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
	}}
	source := fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt")
	output := fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive")
	preview := evaluateCanonicalFidelity(fidelityAuthority{profile: profile, worker: map[string]interface{}(profile.WorkerConfig), name: "preview_effective_plan"}, source, output, nil)
	testEncode := evaluateCanonicalFidelity(fidelityAuthority{profile: profile, worker: map[string]interface{}(profile.WorkerConfig), name: "test_encode_effective_plan"}, source, output, nil)
	previewExpected := preview["expected"].(models.JSONMap)
	testExpected := testEncode["expected"].(models.JSONMap)
	if preview["status"] != "passed" || testEncode["status"] != "passed" || previewExpected["frameRate"] != testExpected["frameRate"] || previewExpected["fieldStructure"] != testExpected["fieldStructure"] {
		t.Fatalf("surface effective plans diverged in the canonical evaluator: preview=%#v test=%#v", preview, testEncode)
	}
}

func TestCanonicalFrameFidelityUnknownSourceCanUseAuthoritativeExpectedPlan(t *testing.T) {
	profile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
		"effectiveOutputWidth": 1280, "effectiveOutputHeight": 720,
	}}
	output := fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive")
	outputStream := output["streams"].([]interface{})[0].(map[string]any)
	outputStream["width"], outputStream["height"] = 1280, 720
	report := runCanonicalFidelity(t, profile, map[string]any{}, output, nil)
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["width"].(models.JSONMap)["source"] != nil || fields["width"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("authoritative plan could not validate unknown source: %#v", report)
	}
}

func TestCanonicalFrameFidelityQueueSnapshotIgnoresLaterProfileMutation(t *testing.T) {
	frozenProfile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "yuv420p", "effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
	}}
	snapshot, err := scheduler.CaptureProfileSnapshot(frozenProfile, time.Now(), "fidelity-freeze")
	if err != nil {
		t.Fatal(err)
	}
	mutatedProfile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{
		"pixFmt": "p010le", "effectiveOutputFrameRate": "30000/1001", "effectiveOutputProgressive": false,
	}}
	_ = mutatedProfile
	report := validateJobFrameFidelity(testEncodeTestDB(t, "canonical-fidelity-freeze"), models.QueueJob{ID: 1, ProfileSnapshot: snapshot}, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "tt"), fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive"), nil)
	fields := report["fields"].(models.JSONMap)
	if fields["frameRate"].(models.JSONMap)["expected"] != "24000/1001" || fields["fieldStructure"].(models.JSONMap)["expected"] != "progressive" || report["workingPixelFormat"] != "yuv420p" {
		t.Fatalf("Fidelity read mutable state instead of frozen snapshot: %#v", report)
	}
}

func TestCanonicalFrameFidelityPreservesChromaLocation(t *testing.T) {
	profile := models.Profile{VideoCodec: "copy", WorkerConfig: models.JSONMap{"cadenceMode": "preserve"}}
	report := runCanonicalFidelity(t, profile, fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive"), fidelityFrameProbe("h264", "yuv420p", "24/1", "progressive"), nil)
	field := report["fields"].(models.JSONMap)["chromaLocation"].(models.JSONMap)
	if field["status"] != "passed" || field["source"] != "left" || field["actual"] != "left" {
		t.Fatalf("preserved chroma location was not validated: %#v", field)
	}
}

func TestFidelityArtifactIdentityRequiresSameExecutionEvidence(t *testing.T) {
	planID := uint(17)
	job := models.QueueJob{ID: 42, MediaPath: "/media/source.mkv", ActiveExecutionPlanID: &planID}
	verified := map[string]any{
		"job":           map[string]any{"id": float64(42), "mediaPath": "/media/source.mkv"},
		"executionPlan": map[string]any{"id": float64(17)},
	}
	if ok, _ := verifiedFidelityArtifactIdentity(verified, job); !ok {
		t.Fatal("same execution artifact was not verified")
	}
	ambiguous := map[string]any{"job": map[string]any{"id": float64(42), "mediaPath": "/media/source.mkv"}}
	if ok, _ := verifiedFidelityArtifactIdentity(ambiguous, job); ok {
		t.Fatal("ambiguous latest artifact became authoritative")
	}
	retry := map[string]any{"job": map[string]any{"id": float64(42), "mediaPath": "/media/source.mkv"}, "executionPlan": map[string]any{"id": float64(18)}}
	if ok, _ := verifiedFidelityArtifactIdentity(retry, job); ok {
		t.Fatal("different retry artifact became authoritative")
	}
}

func TestCanonicalFrameFidelityVerifiedArtifactOutranksSnapshot(t *testing.T) {
	planID := uint(17)
	snapshotProfile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "24000/1001", "pixFmt": "yuv420p"}}
	snapshot, err := scheduler.CaptureProfileSnapshot(snapshotProfile, time.Now(), "snapshot")
	if err != nil {
		t.Fatal(err)
	}
	artifactProfile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "30000/1001", "pixFmt": "yuv420p"}}
	dir := t.TempDir()
	db := testEncodeTestDB(t, "canonical-fidelity-artifact")
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"asIsReportsPath": dir}}).Error; err != nil {
		t.Fatal(err)
	}
	writeFidelityArtifact(t, dir, 17, "/media/source.mkv", planID, artifactProfile)
	job := models.QueueJob{ID: 17, MediaPath: "/media/source.mkv", ActiveExecutionPlanID: &planID, ProfileSnapshot: snapshot}
	report := validateJobFrameFidelity(db, job, fidelityFrameProbe("mpeg2video", "yuv420p", "24000/1001", "progressive"), fidelityFrameProbe("h264", "yuv420p", "30000/1001", "progressive"), nil)
	if report["authority"] != "verified_execution_artifact" || report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)["expected"] != "30000/1001" {
		t.Fatalf("verified execution artifact did not outrank snapshot: %#v", report)
	}
}

func TestCanonicalFrameFidelityAmbiguousArtifactFallsBackToSnapshot(t *testing.T) {
	planID := uint(17)
	snapshotProfile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "24000/1001", "pixFmt": "yuv420p"}}
	snapshot, err := scheduler.CaptureProfileSnapshot(snapshotProfile, time.Now(), "snapshot")
	if err != nil {
		t.Fatal(err)
	}
	artifactProfile := models.Profile{VideoCodec: "h264", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "30000/1001", "pixFmt": "p010le"}}
	dir := t.TempDir()
	db := testEncodeTestDB(t, "canonical-fidelity-ambiguous")
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"asIsReportsPath": dir}}).Error; err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(map[string]any{
		"job":     map[string]any{"id": float64(17), "mediaPath": "/media/source.mkv"},
		"profile": artifactProfile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "17-job-17-retry.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{ID: 17, MediaPath: "/media/source.mkv", ActiveExecutionPlanID: &planID, ProfileSnapshot: snapshot}
	report := validateJobFrameFidelity(db, job, fidelityFrameProbe("mpeg2video", "yuv420p", "30000/1001", "progressive"), fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive"), nil)
	if report["authority"] != "queue_profile_snapshot" || report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)["expected"] != "24000/1001" {
		t.Fatalf("ambiguous artifact overrode frozen snapshot: %#v", report)
	}
	if len(report["authorityDiagnostics"].(models.JSONList)) == 0 {
		t.Fatalf("ambiguous artifact was not retained diagnostically: %#v", report)
	}
}

func writeFidelityArtifact(t *testing.T, dir string, jobID uint, mediaPath string, planID uint, profile models.Profile) {
	t.Helper()
	content, err := json.Marshal(map[string]any{
		"job":           map[string]any{"id": jobID, "mediaPath": mediaPath},
		"profile":       profile,
		"executionPlan": map[string]any{"id": planID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d-job-%d-as-is.json", jobID, jobID)), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCanonicalFidelity(t *testing.T, profile models.Profile, source, output map[string]any, evidence models.JSONMap) models.JSONMap {
	t.Helper()
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "fidelity-test")
	if err != nil {
		t.Fatal(err)
	}
	return validateJobFrameFidelity(testEncodeTestDB(t, "canonical-fidelity"), models.QueueJob{ID: 1, ProfileSnapshot: snapshot}, source, output, evidence)
}

func fidelityFrameProbe(codec, pixelFormat, frameRate, fieldOrder string) map[string]any {
	return map[string]any{"streams": []interface{}{map[string]any{
		"codec_type": "video", "codec_name": codec, "profile": "Main", "width": 720, "height": 480,
		"sample_aspect_ratio": "32:27", "display_aspect_ratio": "16:9", "avg_frame_rate": frameRate,
		"r_frame_rate": frameRate, "field_order": fieldOrder, "pix_fmt": pixelFormat, "chroma_location": "left",
	}}}
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
		name          string
		width, height int
		dar           string
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

func TestSmartUpscaleValidationRejectsSARMismatchWithCorrectGeometry(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"resolvedUpscaleDecision": ResolvedUpscaleDecision{
		RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p,
		TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true,
	}}}
	report := validateSmartUpscaleOutput(profile, nil, models.JSONMap{
		"width": 1280, "height": 720, "sampleAspectRatio": "4:3", "displayAspectRatio": "16:9",
	})
	fields := report["fields"].(models.JSONMap)
	if fields["geometry"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("correct geometry did not pass: %#v", report)
	}
	if fields["sampleAspectRatio"].(models.JSONMap)["status"] != "mismatch" || report["status"] != "mismatch" {
		t.Fatalf("incorrect SAR did not fail Smart Upscale validation: %#v", report)
	}
}

func TestSmartUpscaleValidationAcceptsIVTCOutput(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"effectiveOutputFrameRate": "24000/1001", "effectiveOutputProgressive": true,
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true},
	}}
	report := validateSmartUpscaleOutput(profile, models.JSONMap{"width": 720, "height": 480, "fieldOrder": "tt"}, models.JSONMap{
		"width": 1280, "height": 720, "sampleAspectRatio": "1:1", "displayAspectRatio": "16:9",
		"frameRate": "24000/1001", "realFrameRate": "24000/1001", "fieldOrder": "progressive",
	})
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["geometry"].(models.JSONMap)["status"] != "passed" || fields["frameRate"].(models.JSONMap)["status"] != "validated" || fields["frameStructure"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("valid IVTC + Smart Upscale output rejected: %#v", report)
	}
}

func TestSmartUpscaleValidationAcceptsDeinterlacedOutput(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"effectiveOutputProgressive": true,
		"resolvedUpscaleDecision":    ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true},
	}}
	report := validateSmartUpscaleOutput(profile, models.JSONMap{"width": 720, "height": 480, "fieldOrder": "bb"}, models.JSONMap{
		"width": 1280, "height": 720, "sampleAspectRatio": "1:1", "displayAspectRatio": "16:9", "fieldOrder": "progressive",
	})
	fields := report["fields"].(models.JSONMap)
	if report["status"] != "passed" || fields["frameStructure"].(models.JSONMap)["status"] != "passed" {
		t.Fatalf("valid deinterlace + Smart Upscale output rejected: %#v", report)
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
	probeScript := "#!/bin/sh\ncase \"$*\" in\n  *output.mkv*) printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"hevc\",\"width\":720,\"height\":480,\"sample_aspect_ratio\":\"1:1\",\"display_aspect_ratio\":\"3:2\",\"avg_frame_rate\":\"24000/1001\",\"r_frame_rate\":\"24000/1001\",\"field_order\":\"progressive\"}],\"format\":{\"size\":\"7\"}}' ;;\n  *) printf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"mpeg2video\",\"width\":720,\"height\":480,\"sample_aspect_ratio\":\"32:27\",\"display_aspect_ratio\":\"16:9\",\"avg_frame_rate\":\"30000/1001\",\"r_frame_rate\":\"30000/1001\",\"field_order\":\"progressive\"}],\"format\":{\"size\":\"7\"}}' ;;\nesac\n"
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
	worker := map[string]interface{}{"hevcLevelMode": "recommended", "hevcLevel": "4.0", "hevcLevelEffective": "4.0", "hevcLevelTier": "main"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1", "bitrate": "6418000"}

	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "bitrate": 6418000}, testHEVCBitstreamEvidence("4.0", "main"))
	if validated["status"] != "validated" || validated["levelStatus"] != "validated" || validated["tierStatus"] != "validated" || validated["bitrateStatus"] != "unverified" || validated["requested"] != "4.0" {
		t.Fatalf("validated result=%#v", validated)
	}

	mismatch := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc"}, testHEVCBitstreamEvidence("5.0", "main"))
	if mismatch["status"] != "mismatch" || mismatch["output"] != "5.0" {
		t.Fatalf("mismatch result=%#v", mismatch)
	}
}

func TestHEVCLevelValidationChecksAutoOutputAgainstAssetRecommendation(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "auto", "videoEncoder": "hevc_qsv", "hevcLevelEffective": "4.0", "hevcLevelTier": "main"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1", "bitrate": "6418000"}
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc"}, testHEVCBitstreamEvidence("4.0", "main"))
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
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc"}, testHEVCBitstreamEvidence("4.1", "main"))
	if validated["status"] != "validated" || validated["requested"] != "4.1" {
		t.Fatalf("validation ignored frozen effective Level: %#v", validated)
	}
}

func TestHEVCLevelValidationFallbackUsesEffectiveOutputFPS(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "auto", "videoEncoder": "hevc_qsv", "effectiveOutputFrameRate": "60/1", "hevcLevelEffective": "4.1", "hevcLevelTier": "main"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1"}
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc"}, testHEVCBitstreamEvidence("4.1", "main"))
	if validated["status"] != "validated" || validated["requested"] != "4.1" {
		t.Fatalf("validation fallback used stale source FPS: %#v", validated)
	}
}

func TestHEVCLevelValidationChecksObservedMainTierBitrate(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "custom", "hevcLevel": "4.0", "hevcLevelEffective": "4.0", "hevcLevelTier": "main", "videoEncoder": "hevc_qsv"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1"}
	output := models.JSONMap{"codec": "hevc", "bitrate": 13_000_000}
	validated := validateHEVCLevelField(worker, source, output, testHEVCBitstreamEvidence("4.0", "main"))
	if validated["status"] != "validated" || validated["bitrateStatus"] != "unverified" || validated["effectiveTier"] != "main" {
		t.Fatalf("Main Tier bitrate validation=%#v", validated)
	}
}

func TestHEVCLevelValidationRejectsCustomLevelBelowOutputGeometry(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "custom", "hevcLevel": "4.0", "hevcLevelEffective": "4.0", "videoEncoder": "hevc_qsv"}
	output := models.JSONMap{"codec": "hevc", "width": 3840, "height": 2160, "frameRate": "24/1"}
	validated := validateHEVCLevelField(worker, models.JSONMap{}, output, testHEVCBitstreamEvidence("4.0", "main"))
	if validated["status"] != "mismatch" || validated["requirementsStatus"] != "mismatch" {
		t.Fatalf("Custom Level geometry validation=%#v", validated)
	}
}

func testHEVCBitstreamEvidence(level, tier string) models.JSONMap {
	return models.JSONMap{
		"levelEvidenceAvailable": true,
		"tierEvidenceAvailable":  true,
		"generalLevel":           level,
		"generalTier":            tier,
	}
}

func TestHEVCLevelValidationEvidenceStatusVocabulary(t *testing.T) {
	output := models.JSONMap{"codec": "hevc"}
	worker := map[string]interface{}{"hevcLevelEffective": "4.0", "hevcLevelTier": "main", "videoEncoder": "hevc_qsv"}
	if got := validateHEVCLevelField(worker, nil, output)["status"]; got != "unavailable" {
		t.Fatalf("missing trace_headers evidence status=%v", got)
	}
	malformed := models.JSONMap{"levelEvidenceAvailable": true, "generalLevel": "4.0", "tierEvidenceAvailable": false}
	if got := validateHEVCLevelField(worker, nil, output, malformed)["status"]; got != "unavailable" {
		t.Fatalf("incomplete parameter-set evidence status=%v", got)
	}
	withoutExpected := validateHEVCLevelField(map[string]interface{}{"videoEncoder": "hevc_qsv"}, nil, output, testHEVCBitstreamEvidence("4.0", "main"))
	if withoutExpected["status"] != "unverified" || withoutExpected["expectedLevel"] != "" || withoutExpected["actualLevel"] != "4.0" {
		t.Fatalf("evidence without frozen expected decision was not unverified: %#v", withoutExpected)
	}
	failedProbe := hevcBitstreamEvidenceFromStructure(HEVCBitstreamStructure{}, fmt.Errorf("trace_headers unavailable"))
	if failedProbe["probeStatus"] != "unavailable" || failedProbe["levelEvidenceAvailable"] != false || failedProbe["tierEvidenceAvailable"] != false {
		t.Fatalf("failed bounded probe did not remain unavailable: %#v", failedProbe)
	}
	availableProbe := hevcBitstreamEvidenceFromStructure(HEVCBitstreamStructure{LevelIDC: 123, LevelKnown: true, TierFlag: 0, TierKnown: true}, nil)
	if availableProbe["probeOutcome"] != "available" || availableProbe["probeStatus"] != nil || availableProbe["generalLevel"] != "4.1" || availableProbe["generalTier"] != "main" {
		t.Fatalf("successful bounded probe was described as insufficient: %#v", availableProbe)
	}
}
