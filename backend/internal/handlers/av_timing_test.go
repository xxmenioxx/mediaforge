package handlers

import (
	"fmt"
	"math"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func timingEvidence(videoStart, audioStart float64) avTimingEvidence {
	return avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(videoStart)},
		Audio: []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(audioStart)}}, FrameRate: "24000/1001",
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
		source := avTimingEvidence{Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0)}, FrameRate: "24000/1001"}
		output := avTimingEvidence{Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0)}, FrameRate: "24000/1001"}
		for index, frames := range frameOffsets {
			source.Audio = append(source.Audio, avStreamTiming{Index: index + 1, StartSeconds: secondsPointer(0)})
			output.Audio = append(output.Audio, avStreamTiming{Index: index + 1, StartSeconds: secondsPointer(frames * 1001.0 / 24000.0)})
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
	missingVideo.Video.StartSeconds = nil
	if report := validateAVTiming(missingVideo, timingEvidence(0, 0)); report["status"] != "unverified" {
		t.Fatalf("missing video evidence must be unverified: %#v", report)
	}
	missingAudio := timingEvidence(0, 0)
	missingAudio.Audio[0].StartSeconds = nil
	if report := validateAVTiming(timingEvidence(0, 0), missingAudio); report["status"] != "unverified" {
		t.Fatalf("missing audio evidence must be unverified: %#v", report)
	}
	if report := validateAVTiming(timingEvidence(0, 0), timingEvidence(0, 0)); report["status"] != "validated" {
		t.Fatalf("explicit zero timestamps must remain valid evidence: %#v", report)
	}
}

func TestValidateAVTimingDoesNotHidePartiallyMissingAudioEvidence(t *testing.T) {
	source := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{
			{Index: 1, StartSeconds: secondsPointer(0)},
			{Index: 2},
		},
	}
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{
			{Index: 1, StartSeconds: secondsPointer(0)},
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
	evidence := avTimingFromProbe(map[string]any{"streams": []interface{}{
		map[string]interface{}{"index": float64(0), "codec_type": "video", "start_time": "0.042000", "avg_frame_rate": "24000/1001"},
		map[string]interface{}{"index": float64(1), "codec_type": "audio", "start_time": "0.000000"},
	}})
	if evidence.Video == nil || evidence.Video.StartSeconds == nil || math.Abs(*evidence.Video.StartSeconds-.042) > .0001 || len(evidence.Audio) != 1 {
		t.Fatalf("FFprobe timing strings were not parsed: %#v", evidence)
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
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0)},
		Audio: []avStreamTiming{
			{Index: 1, Language: "spa", Title: "Spanish", StartSeconds: secondsPointer(0)},
			{Index: 2, Language: "eng", Title: "English", StartSeconds: secondsPointer(.080)},
			{Index: 3, Language: "eng", Title: "Commentary", StartSeconds: secondsPointer(.160)},
		},
		FrameRate: "24000/1001",
	}
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(.042)},
		Audio: []avStreamTiming{
			{Index: 1, Language: "eng", Title: "English", StartSeconds: secondsPointer(.122)},
			{Index: 2, Language: "eng", Title: "Commentary", StartSeconds: secondsPointer(.202)},
		},
		FrameRate: "24000/1001",
	}
	plan := ResolvedStreamPlan{Audio: []PlannedStream{
		{InputIndex: 2, Action: "copy", OutputTypeIndex: 0},
		{InputIndex: 3, Action: "copy", OutputTypeIndex: 1},
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
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(0)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{
			{Index: 1, StartSeconds: secondsPointer(0)},
			{Index: 2, StartSeconds: secondsPointer(.5)},
		},
	}
	duplicate := 1
	plan := ResolvedStreamPlan{Audio: []PlannedStream{
		{InputIndex: 1, OutputTypeIndex: 0, Action: "copy"},
		{InputIndex: 1, OutputTypeIndex: 1, Action: "derive", DuplicateOf: &duplicate},
	}}
	selectedSource, selectedOutput := avTimingForStreamPlan(source, output, plan)
	if len(selectedSource.Audio) != 1 || len(selectedOutput.Audio) != 1 || selectedOutput.Audio[0].Index != 1 {
		t.Fatalf("derived audio replaced canonical comparison: source=%#v output=%#v", selectedSource, selectedOutput)
	}
}

func TestAVTimingForStreamPlanUsesDerivedWhenOriginalIsNotPreserved(t *testing.T) {
	source := timingEvidence(0, .080)
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: secondsPointer(.042)}, FrameRate: "24000/1001",
		Audio: []avStreamTiming{{Index: 1, StartSeconds: secondsPointer(.122)}},
	}
	duplicate := 1
	plan := ResolvedStreamPlan{Audio: []PlannedStream{{InputIndex: 1, OutputTypeIndex: 0, Action: "derive", DuplicateOf: &duplicate}}}
	selectedSource, selectedOutput := avTimingForStreamPlan(source, output, plan)
	if report := validateAVTiming(selectedSource, selectedOutput); report["status"] != "validated" {
		t.Fatalf("derived-only replacement did not preserve timing identity: %#v", report)
	}
}
