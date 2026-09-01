package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPixelSamplingSessionSharesDecodeForIDETAndCrop(t *testing.T) {
	binDir := t.TempDir()
	counterPath := filepath.Join(binDir, "calls")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	script := `#!/bin/sh
case "$*" in
  *"trim=start=8.500:duration=3"*) ;;
  *) exit 2 ;;
esac
printf x >> "$PIXEL_COUNTER"
printf '%s\n' \
  'Repeated Fields: Neither: 90 Top: 5 Bottom: 5' \
  'Multi frame detection: TFF: 10 BFF: 5 Progressive: 80 Undetermined: 5' \
  'crop=700:400:10:40' \
  'lavfi.block=10.500000' \
  'lavfi.bitplanenoise.0.1=0.030000' \
  'lavfi.bitplanenoise.1.1=0.050000' \
  'lavfi.bitplanenoise.2.1=0.070000' >&2
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXEL_COUNTER", counterPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	plan := SamplingPlan{AssetDurationSeconds: 100, WindowSeconds: 20, Positions: []float64{0.5}}
	session := newPixelSamplingSession("/fixture/movie.mkv", 720, 480, plan)
	interlaceWindow := plan.windows(20)[0]
	interlace, interlaceOK := session.interlaceSample(context.Background(), interlaceWindow, 20)
	cropWindow := representativeSamplingWindows(plan, 3, 3)[0]
	crop, cropOK := session.cropCandidate(context.Background(), cropWindow)
	restoration := session.restorationAnalysis()

	if !interlaceOK || interlace.SampledFrames != 100 || !cropOK || crop.width != 700 || crop.height != 400 || restoration.Blocking.Availability != "available" || restoration.Windows != 1 {
		t.Fatalf("shared evidence interlace=%#v ok=%t crop=%#v ok=%t restoration=%#v", interlace, interlaceOK, crop, cropOK, restoration)
	}
	calls, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(calls)) != "x" {
		t.Fatalf("expected one shared ffmpeg decode, calls=%q", calls)
	}
}

func TestSharedPixelProbeParsesEvidenceIndependently(t *testing.T) {
	evidence := sharedPixelEvidence{}
	if evidence.interlaceOK || evidence.cropOK {
		t.Fatalf("zero shared evidence must remain unavailable: %#v", evidence)
	}
	if _, ok := parseIDETOutput("crop=700:400:10:40"); ok {
		t.Fatal("crop output must not masquerade as IDET evidence")
	}
	if _, ok := parseCropCandidateOutput("Multi frame detection: TFF: 1 BFF: 0 Progressive: 9 Undetermined: 0"); ok {
		t.Fatal("IDET output must not masquerade as crop evidence")
	}
}

func TestSharedPixelProbeFallsBackWhenRestorationMetricsAreUnavailable(t *testing.T) {
	binDir := t.TempDir()
	counterPath := filepath.Join(binDir, "calls")
	script := `#!/bin/sh
printf x >> "$PIXEL_COUNTER"
case "$*" in
  *restoration_out*) printf '%s\n' "No such filter: 'blockdetect'" >&2; exit 1 ;;
esac
printf '%s\n' 'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' 'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' 'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXEL_COUNTER", counterPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	evidence := runSharedPixelProbeContext(context.Background(), "/fixture/movie.mkv", 40, 20, 48.5)
	if !evidence.interlaceOK || !evidence.cropOK || evidence.restoration.available() {
		t.Fatalf("fallback evidence mismatch: %#v", evidence)
	}
	calls, err := os.ReadFile(counterPath)
	if err != nil || string(calls) != "xx" {
		t.Fatalf("expected augmented probe plus compatibility fallback, calls=%q err=%v", calls, err)
	}
}

func TestQuickInterlaceDoesNotReduceCropDepthAndPreservesSharedDecode(t *testing.T) {
	binDir := t.TempDir()
	counterPath := filepath.Join(binDir, "calls")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	script := `#!/bin/sh
printf x >> "$PIXEL_COUNTER"
printf '%s\n' \
  'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' \
  'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' \
  'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXEL_COUNTER", counterPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	basePlan := canonicalSamplingPlan(1000, defaultFrameStructureSamplingPolicy())
	frame := QSVFrameStructureAnalysis{ConfidenceScore: 0.995, WindowCount: 3, Windows: []FrameStructureWindow{
		{Position: 0.08, FrameSignals: FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 24}},
		{Position: 0.50, FrameSignals: FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 24}},
		{Position: 0.92, FrameSignals: FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 24}},
	}}
	interlacePlan, depth, _ := adaptiveInterlaceSamplingPlan(basePlan, frame, "progressive")
	session := newPixelSamplingSession("/fixture/movie.mkv", 720, 480, basePlan)
	interlace := detectInterlaceWithSharedEvidenceContext(context.Background(), "/fixture/movie.mkv", "progressive", 20, false, interlacePlan, frame, session)
	crop := detectCropWithSharedPixelEvidenceContext(context.Background(), "/fixture/movie.mkv", 720, 480, basePlan, session)
	calls, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	if depth != interlaceAnalysisDepthQuick || string(calls) != "xxx" || interlace.SampleCount != 2 || crop.Windows != 3 || crop.RecommendedCrop != "700:400:10:40" {
		t.Fatalf("quick shared decode regressed: calls=%q interlace=%#v crop=%#v", calls, interlace, crop)
	}
}
