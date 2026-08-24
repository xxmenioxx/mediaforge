package handlers

import (
	"fmt"
	"math"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func timingEvidence(videoStart, audioStart float64) avTimingEvidence {
	return avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: videoStart},
		Audio: []avStreamTiming{{Index: 1, StartSeconds: audioStart}}, FrameRate: "24000/1001",
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

func TestAVTimingFromProbeParsesFFprobeStringTimestamps(t *testing.T) {
	evidence := avTimingFromProbe(map[string]any{"streams": []interface{}{
		map[string]interface{}{"index": float64(0), "codec_type": "video", "start_time": "0.042000", "avg_frame_rate": "24000/1001"},
		map[string]interface{}{"index": float64(1), "codec_type": "audio", "start_time": "0.000000"},
	}})
	if evidence.Video == nil || math.Abs(evidence.Video.StartSeconds-.042) > .0001 || len(evidence.Audio) != 1 {
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
		Video: &avStreamTiming{Index: 0},
		Audio: []avStreamTiming{
			{Index: 1, Language: "spa", Title: "Spanish", StartSeconds: 0},
			{Index: 2, Language: "eng", Title: "English", StartSeconds: .080},
			{Index: 3, Language: "eng", Title: "Commentary", StartSeconds: .160},
		},
		FrameRate: "24000/1001",
	}
	output := avTimingEvidence{
		Video: &avStreamTiming{Index: 0, StartSeconds: .042},
		Audio: []avStreamTiming{
			{Index: 1, Language: "eng", Title: "English", StartSeconds: .122},
			{Index: 2, Language: "eng", Title: "Commentary", StartSeconds: .202},
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
