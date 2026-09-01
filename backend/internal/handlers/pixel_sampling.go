package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

type sharedPixelEvidence struct {
	interlace   InterlaceAnalysis
	interlaceOK bool
	crop        cropCandidate
	cropOK      bool
	restoration restorationWindowEvidence
}

type pixelSamplingSession struct {
	path          string
	width         int
	height        int
	cropWindows   map[int]SamplingWindow
	evidence      map[int]sharedPixelEvidence
	restoration   []restorationWindowEvidence
	sharedDecodes int
}

func newPixelSamplingSession(path string, width, height int, plan SamplingPlan) *pixelSamplingSession {
	session := &pixelSamplingSession{
		path: path, width: width, height: height,
		cropWindows: map[int]SamplingWindow{}, evidence: map[int]sharedPixelEvidence{},
	}
	for _, window := range representativeSamplingWindows(plan, 3, plan.cropWindowMaximum()) {
		session.cropWindows[samplingPositionKey(window.Position)] = window
	}
	return session
}

func samplingPositionKey(position float64) int {
	return int(position*1_000_000 + 0.5)
}

func (session *pixelSamplingSession) interlaceSample(ctx context.Context, window SamplingWindow, seconds int) (InterlaceAnalysis, bool) {
	key := samplingPositionKey(window.Position)
	if cropWindow, shared := session.cropWindows[key]; shared {
		evidence := runSharedPixelProbeContext(ctx, session.path, window.StartSeconds, seconds, cropWindow.StartSeconds)
		session.evidence[key] = evidence
		if evidence.restoration.available() {
			session.restoration = append(session.restoration, evidence.restoration)
		}
		if evidence.interlaceOK {
			session.sharedDecodes++
			return evidence.interlace, true
		}
	}
	return runIDETContext(ctx, session.path, window.StartSeconds, seconds, "idet")
}

func (session *pixelSamplingSession) restorationAnalysis() RestorationAnalysis {
	return classifyRestorationEvidence(session.restoration)
}

func (session *pixelSamplingSession) hasSharedCrop(position float64) bool {
	evidence, ok := session.evidence[samplingPositionKey(position)]
	return ok && evidence.cropOK
}

func (session *pixelSamplingSession) cropCandidate(ctx context.Context, window SamplingWindow) (cropCandidate, bool) {
	evidence, ok := session.evidence[samplingPositionKey(window.Position)]
	if !ok || !evidence.cropOK {
		return cropCandidateAtContext(ctx, session.path, window.StartSeconds, session.width, session.height)
	}
	candidate := evidence.crop
	if session.width-candidate.width < max(8, session.width/100) && session.height-candidate.height < max(8, session.height/100) {
		if tolerant, tolerantOK := cropCandidateAtLimitContext(ctx, session.path, window.StartSeconds, 64); tolerantOK {
			return tolerant, true
		}
	}
	return candidate, true
}

func runSharedPixelProbeContext(parent context.Context, path string, interlaceStart float64, interlaceSeconds int, cropStart float64) sharedPixelEvidence {
	ctx, cancel := context.WithTimeout(parent, time.Duration(interlaceSeconds+30)*time.Second)
	defer cancel()
	cropOffset := max(0.0, cropStart-interlaceStart)
	filter := fmt.Sprintf(
		"[0:v:0]split=3[idet_in][crop_in][restoration_in];[idet_in]idet[idet_out];[crop_in]trim=start=%.3f:duration=3,setpts=PTS-STARTPTS,fps=3,cropdetect=limit=24:round=2:reset=0[crop_out];[restoration_in]trim=start=%.3f:duration=3,setpts=PTS-STARTPTS,%s[restoration_out]",
		cropOffset,
		cropOffset,
		restorationMetricFilterChain(),
	)
	args := []string{
		"-hide_banner", "-ss", fmt.Sprintf("%.3f", interlaceStart), "-i", path,
		"-t", strconv.Itoa(interlaceSeconds), "-filter_complex", filter,
		"-map", "[idet_out]", "-map", "[crop_out]", "-map", "[restoration_out]", "-an", "-sn", "-f", "null", "-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	interlace, interlaceOK := parseIDETOutput(stderr.String())
	crop, cropOK := parseCropCandidateOutput(stderr.String())
	restoration := parseRestorationWindowEvidence(stderr.String())
	if !interlaceOK && !cropOK && restorationMetricFilterUnavailable(stderr.String()) {
		return runSharedPixelProbeWithoutRestorationContext(parent, path, interlaceStart, interlaceSeconds, cropStart)
	}
	return sharedPixelEvidence{interlace: interlace, interlaceOK: interlaceOK, crop: crop, cropOK: cropOK, restoration: restoration}
}

func runSharedPixelProbeWithoutRestorationContext(parent context.Context, path string, interlaceStart float64, interlaceSeconds int, cropStart float64) sharedPixelEvidence {
	ctx, cancel := context.WithTimeout(parent, time.Duration(interlaceSeconds+30)*time.Second)
	defer cancel()
	cropOffset := max(0.0, cropStart-interlaceStart)
	filter := fmt.Sprintf("[0:v:0]split=2[idet_in][crop_in];[idet_in]idet[idet_out];[crop_in]trim=start=%.3f:duration=3,setpts=PTS-STARTPTS,fps=3,cropdetect=limit=24:round=2:reset=0[crop_out]", cropOffset)
	args := []string{"-hide_banner", "-ss", fmt.Sprintf("%.3f", interlaceStart), "-i", path, "-t", strconv.Itoa(interlaceSeconds), "-filter_complex", filter, "-map", "[idet_out]", "-map", "[crop_out]", "-an", "-sn", "-f", "null", "-"}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	interlace, interlaceOK := parseIDETOutput(stderr.String())
	crop, cropOK := parseCropCandidateOutput(stderr.String())
	return sharedPixelEvidence{interlace: interlace, interlaceOK: interlaceOK, crop: crop, cropOK: cropOK}
}
