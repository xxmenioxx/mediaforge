package handlers

import (
	"fmt"
	"math"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func timingEvidence(videoStart, audioStart float64) avTimingEvidence {
	videoPresentation, audioPresentation := secondsPointer(videoStart), secondsPointer(audioStart)
	return avTimingEvidence{
		Video: &avStreamTiming{
			Index: 0, StartSeconds: secondsPointer(videoStart), FirstPresentedSeconds: videoPresentation,
			FirstPresentedSource: "frame.best_effort_timestamp_time", FirstPresentedOrigin: "test_fixture",
		},
		Audio: []avStreamTiming{{
			Index: 1, StartSeconds: secondsPointer(audioStart), FirstPacketPTSSeconds: audioPresentation,
			FirstPacketPTSSource: "packet.pts_time", FirstPacketPTSOrigin: "test_fixture",
		}}, FrameRate: "24000/1001",
	}
}

func TestValidateAVTimingPreservesRelativeOffset(t *testing.T) {
	report := validateAVTiming(timingEvidence(0, .080), timingEvidence(.042, .122))
	if report["status"] != "validated" {
		t.Fatalf("preserved non-zero offset must validate: %#v", report)
	}
	track := report["tracks"].([]models.JSONMap)[0]
	if math.Abs(track["introducedOffsetMs"].(float64)) > .001 {
		t.Fatalf("introduced offset=%v want 0", track["introducedOffsetMs"])
	}
}

func TestValidateAVTimingAllowsApproximatelyOneFrame(t *testing.T) {
	report := validateAVTiming(timingEvidence(0, 0), timingEvidence(1001.0/24000.0, 0))
	if report["status"] != "validated" {
		t.Fatalf("one-frame offset must remain within initial tolerance: %#v", report)
	}
	if report["withinTolerance"] != true || report["toleranceFrames"] != float64(1) || report["frameRateUsed"] != "24000/1001" {
		t.Fatalf("one-frame policy must be explicit in the report: %#v", report)
	}
	track := report["tracks"].([]models.JSONMap)[0]
	if track["withinTolerance"] != true || math.Abs(track["introducedOffsetFrames"].(float64)+1) > .001 {
		t.Fatalf("one-frame difference must remain visible: %#v", track)
	}
}

func TestValidateAVTimingUsesPresentationTimestampsNotDTS(t *testing.T) {
	sourceVideoPTS, sourceVideoDTS, sourceAudioPTS := 10.0, 9.9, 10.08
	outputVideoPTS, outputVideoDTS, outputAudioPTS := .042, -.058, .122
	source := timingEvidence(0, 0)
	source.Video.FirstPresentedSeconds, source.Video.FirstPacketDTSSeconds = &sourceVideoPTS, &sourceVideoDTS
	source.Audio[0].FirstPacketPTSSeconds = &sourceAudioPTS
	output := timingEvidence(0, 0)
	output.Video.FirstPresentedSeconds, output.Video.FirstPacketDTSSeconds = &outputVideoPTS, &outputVideoDTS
	output.Audio[0].FirstPacketPTSSeconds = &outputAudioPTS
	report := validateAVTiming(source, output)
	track := report["tracks"].([]models.JSONMap)[0]
	if report["status"] != "validated" || math.Abs(track["introducedOffsetMs"].(float64)) > .001 {
		t.Fatalf("presentation relationship was not preserved: %#v", report)
	}
}

func TestValidateAVTimingFramePolicyBoundaries(t *testing.T) {
	tests := []struct {
		frames float64
		status string
	}{
		{frames: .999, status: "validated"},
		{frames: 1, status: "validated"},
		{frames: 1.001, status: "warning"},
		{frames: 2, status: "warning"},
		{frames: 2.001, status: "mismatch"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%.3f_frames", test.frames), func(t *testing.T) {
			report := validateAVTiming(timingEvidence(0, 0), timingEvidence(test.frames*1001.0/24000.0, 0))
			if report["status"] != test.status {
				t.Fatalf("status=%v want %s at %.3f frames: %#v", report["status"], test.status, test.frames, report)
			}
		})
	}
}

func TestValidateAVTimingQuantizedTwoFrameBoundaryWarns(t *testing.T) {
	source := avTimingEvidence{
		Video:     &avStreamTiming{Index: 0, StartSeconds: secondsPointer(.067), FirstPresentedSeconds: secondsPointer(.067), TimeBase: "1/1000"},
		Audio:     []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0), TimeBase: "1/1000"}},
		FrameRate: "30000/1001",
	}
	output := avTimingEvidence{
		Video:     &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0), TimeBase: "1/1000"},
		Audio:     []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0), TimeBase: "1/1000"}},
		FrameRate: "30000/1001",
	}
	report := validateAVTiming(source, output)
	track := report["tracks"].([]models.JSONMap)[0]
	if report["status"] != "warning" || track["status"] != "warning" {
		t.Fatalf("quantized 67 ms production boundary must remain advisory: %#v", report)
	}
	if got := track["introducedOffsetFrames"].(float64); got <= 2 || got >= 2.02 {
		t.Fatalf("fixture should represent the slightly-over-two-frame boundary, got %.6f frames", got)
	}
	if got := track["timestampQuantizationToleranceMs"].(float64); got < .267 || got > 33.366/16 {
		t.Fatalf("time-base allowance=%f ms is not bounded to the observed quantization: %#v", got, track)
	}
}

func TestValidateAVTimingRejectsQuantizedRegressionBeyondAllowance(t *testing.T) {
	frameSeconds := 1001.0 / 30000.0
	source := avTimingEvidence{
		Video:     &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), TimeBase: "1/1000"},
		Audio:     []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0), TimeBase: "1/1000"}},
		FrameRate: "30000/1001",
	}
	source.Video.FirstPresentedSeconds = secondsPointer(0)
	output := avTimingEvidence{
		Video:     &avStreamTiming{Index: 0, StartSeconds: secondsPointer(2.25 * frameSeconds), FirstPresentedSeconds: secondsPointer(2.25 * frameSeconds), TimeBase: "1/1000"},
		Audio:     []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0), TimeBase: "1/1000"}},
		FrameRate: "30000/1001",
	}
	if report := validateAVTiming(source, output); report["status"] != "mismatch" {
		t.Fatalf("a clear regression beyond two frames must still mismatch: %#v", report)
	}
}

func TestValidateAVTimingUsesEffectiveOutputFPSForFrameEquivalence(t *testing.T) {
	source := timingEvidence(0, 0)
	source.FrameRate = "30000/1001"
	output := timingEvidence(1001.0/24000.0, 0)
	output.FrameRate = "24000/1001"
	report := validateAVTiming(source, output)
	track := report["tracks"].([]models.JSONMap)[0]
	if report["frameRateUsed"] != "24000/1001" {
		t.Fatalf("frame equivalence did not record effective output FPS: %#v", report)
	}
	if got := track["introducedOffsetFrames"].(float64); math.Abs(got+1) > .000001 {
		t.Fatalf("introduced frames=%f want -1 using output FPS, not source 29.970", got)
	}
}

func TestValidateAVTimingWithoutOutputFPSIsUnverified(t *testing.T) {
	output := timingEvidence(0, 0)
	output.FrameRate = ""
	if report := validateAVTiming(timingEvidence(0, 0), output); report["status"] != "unverified" {
		t.Fatalf("missing frame-rate evidence must remain unverified: %#v", report)
	}
}

func TestValidateAVTimingRejectsLargeTestEncodeOffset(t *testing.T) {
	report := validateAVTiming(timingEvidence(0, 0), timingEvidence(.563, 0))
	if report["status"] != "mismatch" {
		t.Fatalf("large Test Encode offset must mismatch: %#v", report)
	}
}

func TestValidateAVTimingReportsDriftAsNotMeasured(t *testing.T) {
	report := validateAVTiming(timingEvidence(0, 0), timingEvidence(0, 0))
	if report["driftStatus"] != "not_measured" {
		t.Fatalf("start-time evidence must not claim drift validation: %#v", report)
	}
}

func TestValidateAVTimingSeverityIsOrderIndependent(t *testing.T) {
	statuses := func(frameOffsets ...float64) string {
		source := avTimingEvidence{Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24000/1001"}
		output := avTimingEvidence{Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24000/1001"}
		for index, frames := range frameOffsets {
			source.Audio = append(source.Audio, avStreamTiming{Index: index + 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0)})
			output.Audio = append(output.Audio, avStreamTiming{Index: index + 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(frames * 1001.0 / 24000.0)})
		}
		return validateAVTiming(source, output)["status"].(string)
	}
	for _, test := range []struct {
		name    string
		offsets []float64
		want    string
	}{
		{name: "mismatch then warning", offsets: []float64{3, 1.5}, want: "mismatch"},
		{name: "warning then validated", offsets: []float64{1.5, .5}, want: "warning"},
		{name: "validated mismatch warning", offsets: []float64{.5, 3, 1.5}, want: "mismatch"},
		{name: "reversed", offsets: []float64{1.5, 3, .5}, want: "mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := statuses(test.offsets...); got != test.want {
				t.Fatalf("aggregate=%s want %s", got, test.want)
			}
		})
	}
}

func TestValidateAVTimingDistinguishesMissingEvidenceFromZero(t *testing.T) {
	missingVideo := timingEvidence(0, 0)
	missingVideo.Video.FirstPresentedSeconds = nil
	if report := validateAVTiming(missingVideo, timingEvidence(0, 0)); report["status"] != "unverified" {
		t.Fatalf("missing video evidence must be unverified: %#v", report)
	}
	missingAudio := timingEvidence(0, 0)
	missingAudio.Audio[0].FirstPacketPTSSeconds = nil
	if report := validateAVTiming(timingEvidence(0, 0), missingAudio); report["status"] != "unverified" {
		t.Fatalf("missing audio evidence must be unverified: %#v", report)
	}
	if report := validateAVTiming(timingEvidence(0, 0), timingEvidence(0, 0)); report["status"] != "validated" {
		t.Fatalf("explicit zero timestamps must remain valid evidence: %#v", report)
	}
}

func TestValidateAVTimingDoesNotHidePartiallyMissingAudioEvidence(t *testing.T) {
	source := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{
			{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0)},
			{Index: 2},
		},
	}
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{
			{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0)},
			{Index: 2},
		},
	}
	report := validateAVTiming(source, output)
	if report["status"] != "unverified" {
		t.Fatalf("a validated track hid missing evidence on another track: %#v", report)
	}
	tracks := report["tracks"].([]models.JSONMap)
	if tracks[0]["status"] != "validated" || tracks[1]["status"] != "unverified" {
		t.Fatalf("per-track evidence status was lost: %#v", tracks)
	}
}

func TestValidateAVTimingDoesNotValidateOnlyCommonTrackPrefix(t *testing.T) {
	source := timingEvidence(0, 0)
	source.Audio = append(source.Audio, avStreamTiming{Index: 2, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0)})
	report := validateAVTiming(source, timingEvidence(0, 0))
	if report["status"] != "unverified" {
		t.Fatalf("missing output track was ignored after validating common prefix: %#v", report)
	}
}

func TestOptionalSecondsRejectsInvalidAndNonFiniteValues(t *testing.T) {
	for _, value := range []string{"", "N/A", "invalid", "NaN", "+Inf", "-Inf"} {
		if got := optionalSeconds(value); got != nil {
			t.Fatalf("optionalSeconds(%q)=%v want unavailable", value, *got)
		}
	}
	if got := optionalSeconds("0.000"); got == nil || *got != 0 {
		t.Fatalf("explicit zero was not retained: %v", got)
	}
}

func TestFirstPacketPTSandDTSAreCapturedIndependently(t *testing.T) {
	timing := avStreamTiming{}
	captureFirstPacketTimes(&timing, "N/A", "-0.042")
	captureFirstPacketTimes(&timing, "0.000", "0.000")
	if timing.FirstPacketPTSSeconds == nil || *timing.FirstPacketPTSSeconds != 0 {
		t.Fatalf("first valid PTS was not captured: %#v", timing)
	}
	if timing.FirstPacketDTSSeconds == nil || *timing.FirstPacketDTSSeconds != -.042 {
		t.Fatalf("first valid DTS was overwritten: %#v", timing)
	}
}

func TestAVTimingFromProbeParsesFFprobeStringTimestamps(t *testing.T) {
	evidence := avTimingFromProbeWithOrigin(map[string]any{
		"streams": []interface{}{
			map[string]interface{}{"index": float64(0), "codec_type": "video", "start_time": "0.000000", "time_base": "1/24000", "avg_frame_rate": "24000/1001"},
			map[string]interface{}{"index": float64(1), "codec_type": "audio", "start_time": "0.000000", "time_base": "1/48000"},
		},
		"packets": []interface{}{map[string]interface{}{"stream_index": float64(1), "pts_time": "0.000000", "dts_time": "0.000000"}},
		"frames": []interface{}{
			map[string]interface{}{"media_type": "video", "stream_index": float64(0), "best_effort_timestamp_time": "0.042000"},
			map[string]interface{}{"media_type": "audio", "stream_index": float64(1), "best_effort_timestamp_time": "0.000000"},
		},
	}, "queue.sourceEvidence")
	if evidence.Video == nil || evidence.Video.StartSeconds == nil || *evidence.Video.StartSeconds != 0 || evidence.Video.FirstPresentedSeconds == nil || math.Abs(*evidence.Video.FirstPresentedSeconds-.042) > .0001 || evidence.Video.TimeBase != "1/24000" || evidence.Video.FirstPresentedSource != "frame.best_effort_timestamp_time" || evidence.Video.FirstPresentedOrigin != "queue.sourceEvidence" || len(evidence.Audio) != 1 || evidence.Audio[0].TimeBase != "1/48000" || evidence.Audio[0].FirstPresentedSeconds == nil || *evidence.Audio[0].FirstPresentedSeconds != 0 || evidence.Audio[0].FirstPacketPTSSeconds == nil || evidence.Audio[0].FirstPacketPTSSource != "packet.pts_time" || evidence.Audio[0].FirstPacketPTSOrigin != "queue.sourceEvidence" {
		t.Fatalf("FFprobe timing strings were not parsed: %#v", evidence)
	}
}

func TestAVTimingReadIntervalIsBoundedToTwoSeconds(t *testing.T) {
	if got := avTimingReadInterval(0); got != "%+2" {
		t.Fatalf("initial timing probe interval=%q want a two-second bound", got)
	}
	if got := avTimingReadInterval(4.5); got != "4.5%+2" {
		t.Fatalf("offset timing probe interval=%q want 4.5%%+2", got)
	}
}

func TestAVTimingFallbackProbesOnlyMissingSideOnce(t *testing.T) {
	source := timingEvidence(0, 0)
	output := timingEvidence(0, 0)
	output.Video.FirstPresentedSeconds = nil
	output.Audio[0].FirstPacketPTSSeconds = nil
	var paths []string
	probe := func(path string, intervalStart float64) (avTimingEvidence, error) {
		paths = append(paths, path)
		if intervalStart != 0 {
			t.Fatalf("fallback probe start=%f want initial interval", intervalStart)
		}
		measured := timingEvidence(.042, 0)
		measured.Video.FirstPresentedOrigin = ""
		measured.Audio[0].FirstPacketPTSOrigin = ""
		markAVTimingPresentationOrigin(&measured, "bounded_ffprobe_fallback")
		measured.PresentationProbeStatus = "completed"
		return measured, nil
	}
	measuredSource, measuredOutput := avTimingWithBoundedPresentationFallback(source, output, "/source.mkv", "/output.mkv", probe)
	if len(paths) != 1 || paths[0] != "/output.mkv" {
		t.Fatalf("fallback probe paths=%v want only the side missing presentation evidence", paths)
	}
	if measuredSource.PresentationProbeStatus != "" || measuredOutput.PresentationProbeStatus != "completed" || measuredOutput.Video.FirstPresentedSeconds == nil || *measuredOutput.Video.FirstPresentedSeconds != .042 || measuredOutput.Video.FirstPresentedOrigin != "bounded_ffprobe_fallback" {
		t.Fatalf("fallback evidence was not merged with provenance: source=%#v output=%#v", measuredSource, measuredOutput)
	}
}

func TestAVTimingFallbackFailureKeepsOffsetsUnverified(t *testing.T) {
	source, output := timingEvidence(0, 0), timingEvidence(0, 0)
	source.Video.FirstPresentedSeconds, output.Video.FirstPresentedSeconds = nil, nil
	source.Audio[0].FirstPresentedSeconds, source.Audio[0].FirstPacketPTSSeconds = nil, nil
	output.Audio[0].FirstPresentedSeconds, output.Audio[0].FirstPacketPTSSeconds = nil, nil
	probeCalls := 0
	probe := func(string, float64) (avTimingEvidence, error) {
		probeCalls++
		return avTimingEvidence{}, fmt.Errorf("probe unavailable")
	}
	source, output = avTimingWithBoundedPresentationFallback(source, output, "/source.mkv", "/output.mkv", probe)
	report := validateAVTiming(source, output)
	tracks := report["tracks"].([]models.JSONMap)
	if probeCalls != 2 || source.PresentationProbeStatus != "failed" || output.PresentationProbeStatus != "failed" || report["status"] != "unverified" || len(tracks) != 1 {
		t.Fatalf("failed bounded probes fabricated or hid evidence: calls=%d source=%#v output=%#v report=%#v", probeCalls, source, output, report)
	}
	if _, exists := tracks[0]["introducedOffsetMs"]; exists {
		t.Fatalf("failed probe recorded a fabricated offset: %#v", tracks[0])
	}
}

func TestValidateAVTimingComparesVC1AndOutputAtSamePresented42msFrame(t *testing.T) {
	probe := func(streamStart, firstVideoFrame, audioPTS string) map[string]any {
		return map[string]any{
			"streams": []interface{}{
				map[string]interface{}{"index": float64(0), "codec_type": "video", "start_time": streamStart, "time_base": "1/1000", "avg_frame_rate": "24000/1001"},
				map[string]interface{}{"index": float64(1), "codec_type": "audio", "start_time": "0.000000", "time_base": "1/1000"},
			},
			"packets": []interface{}{map[string]interface{}{"stream_index": float64(1), "pts_time": audioPTS}},
			"frames":  []interface{}{map[string]interface{}{"media_type": "video", "stream_index": float64(0), "best_effort_timestamp_time": firstVideoFrame}},
		}
	}
	source := avTimingFromProbeWithOrigin(probe("0.000000", "0.042000", "0.000000"), "queue.sourceEvidence")
	output := avTimingFromProbeWithOrigin(probe("0.042000", "0.042000", "0.000000"), "result.outputProbe")
	report := validateAVTiming(source, output)
	track := report["tracks"].([]models.JSONMap)[0]
	if report["status"] != "validated" || math.Abs(track["sourceOffsetMs"].(float64)+42) > .001 || math.Abs(track["outputOffsetMs"].(float64)+42) > .001 || math.Abs(track["introducedOffsetMs"].(float64)) > .001 {
		t.Fatalf("same 42 ms presented frame was treated as an introduced offset: %#v", report)
	}
	if track["decision"] != "validated" || track["passToleranceFrames"] != float64(1) || track["warningToleranceFrames"] != float64(2) {
		t.Fatalf("per-track decision or policy thresholds are missing: %#v", track)
	}
	evidenceUsed := track["evidenceUsed"].(models.JSONMap)
	sourceVideoEvidence := evidenceUsed["sourceVideo"].(models.JSONMap)
	if sourceVideoEvidence["field"] != "frame.best_effort_timestamp_time" || sourceVideoEvidence["origin"] != "queue.sourceEvidence" || sourceVideoEvidence["timestampSeconds"] != .042 {
		t.Fatalf("source presentation provenance was not reported: %#v", evidenceUsed)
	}
}

func TestValidateAVTimingH264ZeroPresentedTimestampControl(t *testing.T) {
	probe := func() map[string]any {
		return map[string]any{
			"streams": []interface{}{
				map[string]interface{}{"index": float64(0), "codec_type": "video", "start_time": "0.000000", "time_base": "1/1000", "avg_frame_rate": "24000/1001"},
				map[string]interface{}{"index": float64(1), "codec_type": "audio", "start_time": "0.000000", "time_base": "1/1000"},
			},
			"packets": []interface{}{map[string]interface{}{"stream_index": float64(1), "pts_time": "0.000000"}},
			"frames":  []interface{}{map[string]interface{}{"media_type": "video", "stream_index": float64(0), "best_effort_timestamp_time": "0.000000"}},
		}
	}
	report := validateAVTiming(avTimingFromProbe(probe()), avTimingFromProbe(probe()))
	track := report["tracks"].([]models.JSONMap)[0]
	if report["status"] != "validated" || track["introducedOffsetMs"] != float64(0) {
		t.Fatalf("zero-offset presentation control did not pass: %#v", report)
	}
}

func TestValidateAVTimingDoesNotFallBackToStreamStartTime(t *testing.T) {
	source, output := timingEvidence(0, 0), timingEvidence(0, 0)
	source.Video.FirstPresentedSeconds = nil
	output.Video.FirstPresentedSeconds = nil
	source.Audio[0].FirstPresentedSeconds, source.Audio[0].FirstPacketPTSSeconds = nil, nil
	output.Audio[0].FirstPresentedSeconds, output.Audio[0].FirstPacketPTSSeconds = nil, nil
	report := validateAVTiming(source, output)
	tracks := report["tracks"].([]models.JSONMap)
	if report["status"] != "unverified" || len(tracks) != 1 || tracks[0]["status"] != "unverified" {
		t.Fatalf("stream start times were used without presentation anchors: %#v", report)
	}
	if tracks[0]["decision"] != "unverified" || tracks[0]["evidenceUsed"].(models.JSONMap)["sourceVideo"].(models.JSONMap)["available"] != false {
		t.Fatalf("missing presentation provenance did not remain explicit: %#v", tracks[0])
	}
	if _, exists := tracks[0]["introducedOffsetMs"]; exists {
		t.Fatalf("unverified evidence invented an introduced offset: %#v", tracks[0])
	}
}

func TestValidateAVTimingDetectsIntroducedAudioDrift(t *testing.T) {
	report := validateAVTiming(timingEvidence(0, 0), timingEvidence(0, .100))
	track := report["tracks"].([]models.JSONMap)[0]
	if report["status"] != "mismatch" || track["decision"] != "mismatch" || math.Abs(track["introducedOffsetMs"].(float64)-100) > .001 {
		t.Fatalf("real introduced audio offset was hidden: %#v", report)
	}
}

func TestValidateAVTimingFFprobeDecimalStringsKeepProductionBoundaryAdvisory(t *testing.T) {
	probe := func(videoStart, audioStart string) map[string]any {
		return map[string]any{"streams": []interface{}{
			map[string]interface{}{"index": float64(0), "codec_type": "video", "start_time": videoStart, "time_base": "1/1000", "avg_frame_rate": "30000/1001"},
			map[string]interface{}{"index": float64(1), "codec_type": "audio", "start_time": audioStart, "time_base": "1/1000"},
		}, "frames": []interface{}{
			map[string]interface{}{"media_type": "video", "stream_index": float64(0), "best_effort_timestamp_time": videoStart},
			map[string]interface{}{"media_type": "audio", "stream_index": float64(1), "best_effort_timestamp_time": audioStart},
		}}
	}
	source := avTimingFromProbe(probe("0.067000", "0.000000"))
	output := avTimingFromProbe(probe("0.000000", "0.000000"))
	report := validateAVTiming(source, output)
	track := report["tracks"].([]models.JSONMap)[0]
	if report["status"] != "warning" || track["status"] != "warning" {
		t.Fatalf("ffprobe decimal timestamp boundary must remain warning: %#v", report)
	}
	if track["passToleranceFrames"] != float64(1) || track["warningToleranceFrames"] != float64(2) || track["decision"] != "warning" {
		t.Fatalf("track report lost policy tolerance details: %#v", track)
	}
}

func TestAVTimingForSelectedAudioMatchesAbsoluteInputIndex(t *testing.T) {
	source := avTimingEvidence{Video: &avStreamTiming{Index: 0}, Audio: []avStreamTiming{{Index: 1}, {Index: 4}}, FrameRate: "24/1"}
	output := avTimingEvidence{Video: &avStreamTiming{Index: 0}, Audio: []avStreamTiming{{Index: 1}}, FrameRate: "24/1"}
	selectedSource, selectedOutput := avTimingForSelectedAudio(source, output, []int{4})
	if len(selectedSource.Audio) != 1 || selectedSource.Audio[0].Index != 4 || len(selectedOutput.Audio) != 1 {
		t.Fatalf("selected audio timing mismatch: source=%#v output=%#v", selectedSource, selectedOutput)
	}
}

func TestAVTimingForStreamPlanDoesNotPairRemovedAudioByArrayPosition(t *testing.T) {
	source := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)},
		Audio: []avStreamTiming{
			{Index: 1, Language: "spa", Title: "Spanish", StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0)},
			{Index: 2, Language: "eng", Title: "English", StartSeconds: secondsPointer(.080), FirstPacketPTSSeconds: secondsPointer(.080)},
			{Index: 3, Language: "eng", Title: "Commentary", StartSeconds: secondsPointer(.160), FirstPacketPTSSeconds: secondsPointer(.160)},
		},
		FrameRate: "24000/1001",
	}
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(.042), FirstPresentedSeconds: secondsPointer(.042)},
		Audio: []avStreamTiming{
			{Index: 1, Language: "eng", Title: "English", StartSeconds: secondsPointer(.122), FirstPacketPTSSeconds: secondsPointer(.122)},
			{Index: 2, Language: "eng", Title: "Commentary", StartSeconds: secondsPointer(.202), FirstPacketPTSSeconds: secondsPointer(.202)},
		},
		FrameRate: "24000/1001",
	}
	plan := ResolvedStreamPlan{Audio: []PlannedStream{
		{InputIndex: 2, Action: "copy", OutputIndex: 1, OutputTypeIndex: 0},
		{InputIndex: 3, Action: "copy", OutputIndex: 2, OutputTypeIndex: 1},
	}}
	selectedSource, selectedOutput := avTimingForStreamPlan(source, output, plan)
	if len(selectedSource.Audio) != 2 || selectedSource.Audio[0].Index != 2 || selectedSource.Audio[1].Index != 3 {
		t.Fatalf("removed Spanish track was paired by array position: %#v", selectedSource.Audio)
	}
	report := validateAVTiming(selectedSource, selectedOutput)
	if report["status"] != "validated" {
		t.Fatalf("English and Commentary preserved offsets should validate: %#v", report)
	}
	tracks := report["tracks"].([]models.JSONMap)
	if tracks[0]["sourceAudioIndex"] != 2 || tracks[1]["sourceAudioIndex"] != 3 {
		t.Fatalf("report lost absolute source track identity: %#v", tracks)
	}
}

func TestAVTimingForStreamPlanSkipsDerivedWhenOriginalExists(t *testing.T) {
	source := timingEvidence(0, 0)
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{
			{Index: 1, StartSeconds: secondsPointer(0), FirstPacketPTSSeconds: secondsPointer(0)},
			{Index: 2, StartSeconds: secondsPointer(.5), FirstPacketPTSSeconds: secondsPointer(.5)},
		},
	}
	duplicate := 1
	plan := ResolvedStreamPlan{Audio: []PlannedStream{
		{InputIndex: 1, OutputIndex: 1, OutputTypeIndex: 0, Action: "copy"},
		{InputIndex: 1, OutputIndex: 2, OutputTypeIndex: 1, Action: "derive", DuplicateOf: &duplicate},
	}}
	selectedSource, selectedOutput := avTimingForStreamPlan(source, output, plan)
	if len(selectedSource.Audio) != 1 || len(selectedOutput.Audio) != 1 || selectedOutput.Audio[0].Index != 1 {
		t.Fatalf("derived audio replaced canonical comparison: source=%#v output=%#v", selectedSource, selectedOutput)
	}
}

func TestAVTimingForStreamPlanUsesDerivedWhenOriginalIsNotPreserved(t *testing.T) {
	source := timingEvidence(0, .080)
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(.042), FirstPresentedSeconds: secondsPointer(.042)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(.122), FirstPacketPTSSeconds: secondsPointer(.122)}},
	}
	duplicate := 1
	plan := ResolvedStreamPlan{Audio: []PlannedStream{{InputIndex: 1, OutputIndex: 1, OutputTypeIndex: 0, Action: "derive", DuplicateOf: &duplicate}}}
	selectedSource, selectedOutput := avTimingForStreamPlan(source, output, plan)
	if report := validateAVTiming(selectedSource, selectedOutput); report["status"] != "validated" {
		t.Fatalf("derived-only replacement did not preserve timing identity: %#v", report)
	}
}

func TestAVTimingForStreamPlanMissingOutputDoesNotShiftLaterPairs(t *testing.T) {
	for _, missingOutput := range []int{1, 2} {
		t.Run(fmt.Sprintf("missing_output_%d", missingOutput), func(t *testing.T) {
			source := avTimingEvidence{Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24/1"}
			output := avTimingEvidence{Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0), FirstPresentedSeconds: secondsPointer(0)}, FrameRate: "24/1"}
			plan := ResolvedStreamPlan{}
			for index := 1; index <= 3; index++ {
				source.Audio = append(source.Audio, avStreamTiming{Index: index, StartSeconds: secondsPointer(float64(index) / 10), FirstPacketPTSSeconds: secondsPointer(float64(index) / 10)})
				plan.Audio = append(plan.Audio, PlannedStream{InputIndex: index, OutputIndex: index, OutputTypeIndex: index - 1, Action: "copy"})
				if index != missingOutput {
					output.Audio = append(output.Audio, avStreamTiming{Index: index, StartSeconds: secondsPointer(float64(index) / 10), FirstPacketPTSSeconds: secondsPointer(float64(index) / 10)})
				}
			}
			selectedSource, selectedOutput := avTimingForStreamPlan(source, output, plan)
			report := validateAVTiming(selectedSource, selectedOutput)
			tracks := report["tracks"].([]models.JSONMap)
			if report["status"] != "unverified" || len(tracks) != 3 {
				t.Fatalf("missing planned output was not represented atomically: %#v", report)
			}
			for index, track := range tracks {
				want := "validated"
				if index+1 == missingOutput {
					want = "unverified"
				}
				if track["status"] != want || track["sourceAudioIndex"] != index+1 || track["outputAudioIndex"] != index+1 {
					t.Fatalf("track %d shifted after missing output: %#v", index+1, tracks)
				}
			}
		})
	}
}

func TestAVTimingForQueueValidationFailsClosedWithoutFrozenPlan(t *testing.T) {
	source := timingEvidence(0, 0)
	output := timingEvidence(0, 0)
	selectedSource, selectedOutput := avTimingForQueueValidation(source, output, ResolvedStreamPlan{}, false)
	report := validateAVTiming(selectedSource, selectedOutput)
	if report["status"] != "unverified" || len(selectedSource.Audio) != 0 || len(selectedOutput.Audio) != 1 {
		t.Fatalf("Queue revived positional A/V matching without the AS-IS stream plan: %#v", report)
	}
}
