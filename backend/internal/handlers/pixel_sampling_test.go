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
  'crop=700:400:10:40' >&2
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

	if !interlaceOK || interlace.SampledFrames != 100 || !cropOK || crop.width != 700 || crop.height != 400 {
		t.Fatalf("shared evidence interlace=%#v ok=%t crop=%#v ok=%t", interlace, interlaceOK, crop, cropOK)
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
