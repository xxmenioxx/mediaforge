package handlers

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type ffmpegFrameObservation struct {
	flags       []string
	checksums   []string
	frames      int
	duration    float64
	fps         string
	progressive bool
}

var showInfoObservationPattern = regexp.MustCompile(`showinfo.*n:\s*\d+.*i:([PTB]).*checksum:([0-9A-F]+)`)
var ffmpegFrameCountPattern = regexp.MustCompile(`frame=\s*(\d+)`)
var ffmpegDurationPattern = regexp.MustCompile(`time=00:00:(\d+\.\d+)`)
var ffmpegOutputFPSPattern = regexp.MustCompile(`Video:.*?,\s*([0-9]+(?:\.[0-9]+)?(?:/[0-9]+)?) fps,`)

func runSyntheticMotionChain(t *testing.T, source, filter string) ffmpegFrameObservation {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is required for the synthetic motion-chain harness")
	}
	command := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "info", "-f", "lavfi", "-i", source, "-vf", filter+",showinfo", "-an", "-f", "null", "-")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("synthetic FFmpeg chain failed: %v\n%s", err, output)
	}
	observation := ffmpegFrameObservation{}
	for _, match := range showInfoObservationPattern.FindAllStringSubmatch(string(output), -1) {
		observation.flags = append(observation.flags, match[1])
		observation.checksums = append(observation.checksums, match[2])
	}
	frameMatches := ffmpegFrameCountPattern.FindAllStringSubmatch(string(output), -1)
	if len(frameMatches) > 0 {
		observation.frames, _ = strconv.Atoi(frameMatches[len(frameMatches)-1][1])
	}
	durationMatches := ffmpegDurationPattern.FindAllStringSubmatch(string(output), -1)
	if len(durationMatches) > 0 {
		observation.duration, _ = strconv.ParseFloat(durationMatches[len(durationMatches)-1][1], 64)
	}
	fpsMatches := ffmpegOutputFPSPattern.FindAllStringSubmatch(string(output), -1)
	if len(fpsMatches) > 0 {
		observation.fps = fpsMatches[len(fpsMatches)-1][1]
	}
	observation.progressive = strings.Contains(string(output), "Video: wrapped_avframe, yuv420p(progressive)")
	if len(observation.flags) == 0 || observation.frames == 0 {
		t.Fatalf("synthetic FFmpeg chain did not produce parseable observations: %s", output)
	}
	return observation
}

func TestSelectiveIVTCFallbackPreservesCleanTelecineFrames(t *testing.T) {
	for _, test := range []struct {
		name  string
		order string
	}{
		{name: "tff", order: "tff"},
		{name: "bff", order: "bff"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "testsrc2=size=160x120:rate=24000/1001:duration=1"
			telecine := "telecine=pattern=23"
			if test.order == "bff" {
				telecine += ":first_field=bottom"
			}
			old := runSyntheticMotionChain(t, source, telecine+",fieldmatch=order="+test.order+":combmatch=full")
			selective := runSyntheticMotionChain(t, source, telecine+",fieldmatch=order="+test.order+":combmatch=full,bwdif=mode=send_frame:parity="+test.order+":deint=interlaced")
			if old.frames != 30 || selective.frames != 30 {
				t.Fatalf("selective stage changed clean pre-decimate frame count: old=%d selective=%d", old.frames, selective.frames)
			}
			if !equalStringSlices(old.flags, selective.flags) || !equalStringSlices(old.checksums, selective.checksums) {
				t.Fatalf("clean fieldmatched frames changed unexpectedly: old flags/checksums=%v/%v selective=%v/%v", old.flags, old.checksums, selective.flags, selective.checksums)
			}

			oldDecimated := runSyntheticMotionChain(t, source, telecine+",fieldmatch=order="+test.order+":combmatch=full,decimate,setfield=prog")
			selectiveDecimated := runSyntheticMotionChain(t, source, telecine+",fieldmatch=order="+test.order+":combmatch=full,bwdif=mode=send_frame:parity="+test.order+":deint=interlaced,decimate,setfield=prog")
			if oldDecimated.frames != 24 || selectiveDecimated.frames != 24 {
				t.Fatalf("clean decimate frame count changed: old=%d selective=%d", oldDecimated.frames, selectiveDecimated.frames)
			}
			assertIVTCOutputTiming(t, selectiveDecimated)
			if !allProgressive(selectiveDecimated.flags) {
				t.Fatalf("selective clean output was not progressive: %v", selectiveDecimated.flags)
			}
		})
	}
}

func TestSelectiveIVTCFallbackRepairsSyntheticResidualCombing(t *testing.T) {
	for _, test := range []struct {
		name      string
		fieldMode string
		order     string
		parity    string
		residual  string
	}{
		{name: "tff", fieldMode: "interleave_top", order: "tff", parity: "tff", residual: "T"},
		{name: "bff", fieldMode: "interleave_bottom", order: "bff", parity: "bff", residual: "B"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "nullsrc=size=160x120:rate=60000/1001:duration=1,geq=random(1)*255:128:128,tinterlace=mode=" + test.fieldMode
			beforeDecimate := runSyntheticMotionChain(t, source, "fieldmatch=order="+test.order+":combmatch=full")
			if beforeDecimate.frames != 30 || !containsFlag(beforeDecimate.flags, test.residual) {
				t.Fatalf("fixture did not produce a residual combed fieldmatch frame: frames=%d flags=%v", beforeDecimate.frames, beforeDecimate.flags)
			}

			old := runSyntheticMotionChain(t, source, "fieldmatch=order="+test.order+":combmatch=full,decimate")
			selective := runSyntheticMotionChain(t, source, "fieldmatch=order="+test.order+":combmatch=full,bwdif=mode=send_frame:parity="+test.parity+":deint=interlaced,decimate,setfield=prog")
			if old.frames != 24 || selective.frames != 24 {
				t.Fatalf("selective fallback changed cadence frame count: old=%d selective=%d", old.frames, selective.frames)
			}
			assertIVTCOutputTiming(t, selective)
			if !containsFlag(old.flags, test.residual) {
				t.Fatalf("control chain did not retain the synthetic residual combing: %v", old.flags)
			}
			if !allProgressive(selective.flags) {
				t.Fatalf("selective fallback did not produce progressive output: %v", selective.flags)
			}
		})
	}
}

func TestCanonicalIVTCDiagnosticsRetainResidualsAndMeasureOutputTiming(t *testing.T) {
	clean := runSyntheticCanonicalIVTCChain(t,
		"testsrc2=size=160x120:rate=24000/1001:duration=1,telecine=pattern=23", "tff")
	if clean.CanonicalOutputFrames != 24 || clean.CanonicalOutputProgressive != clean.CanonicalOutputFrames ||
		clean.CanonicalTimestampAnomalies != 0 || !nearFPS(clean.CanonicalOutputFrameRate, 24000.0/1001.0, .02) {
		t.Fatalf("canonical probe did not preserve clean 23.976 output timing: %#v", clean)
	}

	combed := runSyntheticCanonicalIVTCChain(t,
		"nullsrc=size=160x120:rate=60000/1001:duration=1,geq=random(1)*255:128:128,tinterlace=mode=interleave_top", "tff")
	if combed.ResidualInterlacedFrames == 0 {
		t.Fatalf("pre-bwdif fieldmatch probe hid synthetic residual combing: %#v", combed)
	}
	if combed.CanonicalOutputProgressive != combed.CanonicalOutputFrames || combed.CanonicalTimestampAnomalies != 0 ||
		!nearFPS(combed.CanonicalOutputFrameRate, 24000.0/1001.0, .02) {
		t.Fatalf("selective fallback chain did not expose progressive output timing separately from its residual count: %#v", combed)
	}
}

func runSyntheticCanonicalIVTCChain(t *testing.T, source, order string) InterlaceAnalysis {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is required for the synthetic canonical IVTC probe")
	}
	command := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "info", "-f", "lavfi", "-i", source,
		"-vf", canonicalIVTCDiagnosticFilter(order), "-an", "-sn", "-f", "null", "-")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("synthetic canonical IVTC probe failed: %v\n%s", err, output)
	}
	analysis, ok := parseIDETOutput(string(output))
	if !ok {
		t.Fatalf("synthetic canonical IVTC probe emitted no idet evidence: %s", output)
	}
	return analysis
}

func assertIVTCOutputTiming(t *testing.T, observation ffmpegFrameObservation) {
	t.Helper()
	if observation.duration < .99 || observation.duration > 1.01 {
		t.Fatalf("unexpected synthetic IVTC duration: %v", observation.duration)
	}
	if parseFrameRateValue(observation.fps) <= 0 || !nearFPS(parseFrameRateValue(observation.fps), 24000.0/1001.0, .02) {
		t.Fatalf("unexpected synthetic IVTC FPS: %q", observation.fps)
	}
	if !observation.progressive {
		t.Fatalf("synthetic IVTC output did not carry progressive metadata")
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func containsFlag(flags []string, expected string) bool {
	return strings.Contains(strings.Join(flags, ""), expected)
}

func allProgressive(flags []string) bool {
	return len(flags) > 0 && !containsFlag(flags, "T") && !containsFlag(flags, "B")
}
