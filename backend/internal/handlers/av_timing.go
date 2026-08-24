package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type avStreamTiming struct {
	Index                 int      `json:"index"`
	StartSeconds          *float64 `json:"startSeconds,omitempty"`
	FirstPacketPTSSeconds *float64 `json:"firstPacketPtsSeconds,omitempty"`
	FirstPacketDTSSeconds *float64 `json:"firstPacketDtsSeconds,omitempty"`
	FirstPresentedSeconds *float64 `json:"firstPresentedSeconds,omitempty"`
	Language              string   `json:"language,omitempty"`
	Title                 string   `json:"title,omitempty"`
}

func probeAVTiming(path string, intervalStart float64) (avTimingEvidence, error) {
	interval := "%+2"
	if intervalStart > 0 {
		interval = strconv.FormatFloat(intervalStart, 'f', -1, 64) + "%+2"
	}
	args := []string{
		"-v", "error", "-read_intervals", interval,
		"-show_entries", "stream=index,codec_type,start_time,avg_frame_rate:stream_tags=language,title:packet=stream_index,pts_time,dts_time:frame=media_type,stream_index,best_effort_timestamp_time",
		"-show_streams", "-show_packets", "-show_frames", "-of", "json", path,
	}
	cmd := exec.Command("ffprobe", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return avTimingEvidence{}, fmt.Errorf("probe A/V timing: %s", strings.TrimSpace(stderr.String()))
	}
	var response struct {
		Streams []struct {
			Index     int               `json:"index"`
			CodecType string            `json:"codec_type"`
			StartTime string            `json:"start_time"`
			FrameRate string            `json:"avg_frame_rate"`
			Tags      map[string]string `json:"tags"`
		} `json:"streams"`
		Packets []struct {
			StreamIndex int    `json:"stream_index"`
			PTS         string `json:"pts_time"`
			DTS         string `json:"dts_time"`
		} `json:"packets"`
		Frames []struct {
			MediaType   string `json:"media_type"`
			StreamIndex int    `json:"stream_index"`
			Presented   string `json:"best_effort_timestamp_time"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return avTimingEvidence{}, fmt.Errorf("parse A/V timing probe: %w", err)
	}
	evidence := avTimingEvidence{Audio: []avStreamTiming{}}
	byIndex := map[int]*avStreamTiming{}
	for _, stream := range response.Streams {
		timing := avStreamTiming{Index: stream.Index, StartSeconds: optionalSeconds(stream.StartTime), Language: stream.Tags["language"], Title: stream.Tags["title"]}
		switch stream.CodecType {
		case "video":
			if evidence.Video == nil {
				evidence.Video, evidence.FrameRate = &timing, stream.FrameRate
				byIndex[stream.Index] = evidence.Video
			}
		case "audio":
			evidence.Audio = append(evidence.Audio, timing)
		}
	}
	for index := range evidence.Audio {
		byIndex[evidence.Audio[index].Index] = &evidence.Audio[index]
	}
	for _, packet := range response.Packets {
		timing := byIndex[packet.StreamIndex]
		if timing == nil {
			continue
		}
		captureFirstPacketTimes(timing, packet.PTS, packet.DTS)
	}
	for _, frame := range response.Frames {
		timing := byIndex[frame.StreamIndex]
		if frame.MediaType == "video" && timing != nil && timing.FirstPresentedSeconds == nil {
			timing.FirstPresentedSeconds = optionalSeconds(frame.Presented)
		}
	}
	return evidence, nil
}

func captureFirstPacketTimes(timing *avStreamTiming, pts, dts string) {
	if timing.FirstPacketPTSSeconds == nil {
		timing.FirstPacketPTSSeconds = optionalSeconds(pts)
	}
	if timing.FirstPacketDTSSeconds == nil {
		timing.FirstPacketDTSSeconds = optionalSeconds(dts)
	}
}

func optionalSeconds(value string) *float64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return nil
	}
	seconds := parseFloat(value)
	return &seconds
}

type avTimingEvidence struct {
	Video     *avStreamTiming  `json:"video,omitempty"`
	Audio     []avStreamTiming `json:"audio"`
	FrameRate string           `json:"frameRate,omitempty"`
}

type avTimingTolerancePolicy struct {
	ValidatedFrames float64
	MismatchFrames  float64
}

var initialAVTimingTolerance = avTimingTolerancePolicy{
	ValidatedFrames: 1,
	MismatchFrames:  2,
}

const avTimingFrameComparisonEpsilon = 1e-6

func avTimingFromInventory(streams MediaStreamInventory) avTimingEvidence {
	evidence := avTimingEvidence{Audio: []avStreamTiming{}}
	if len(streams.Video) > 0 {
		video := streams.Video[0]
		evidence.Video = &avStreamTiming{Index: video.Index}
		if video.StartTimeValid {
			evidence.Video.StartSeconds = secondsPointer(video.StartTime)
		}
		evidence.FrameRate = video.FrameRate
	}
	for _, audio := range streams.Audio {
		timing := avStreamTiming{Index: audio.Index, Language: audio.Language, Title: audio.Title}
		if audio.StartTimeValid {
			timing.StartSeconds = secondsPointer(audio.StartTime)
		}
		evidence.Audio = append(evidence.Audio, timing)
	}
	return evidence
}

func avTimingFromProbe(probe map[string]any) avTimingEvidence {
	evidence := avTimingEvidence{Audio: []avStreamTiming{}}
	streams, _ := probe["streams"].([]interface{})
	for _, raw := range streams {
		stream, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		timing := avStreamTiming{Index: workerIntValue(stream["index"], -1), StartSeconds: optionalSeconds(stringFromUnknown(stream["start_time"]))}
		if tags := unknownRecord(stream["tags"]); tags != nil {
			timing.Language = stringFromUnknown(tags["language"])
			timing.Title = stringFromUnknown(tags["title"])
		}
		switch stringFromUnknown(stream["codec_type"]) {
		case "video":
			if evidence.Video == nil {
				evidence.Video = &timing
				evidence.FrameRate = stringFromUnknown(stream["avg_frame_rate"])
			}
		case "audio":
			evidence.Audio = append(evidence.Audio, timing)
		}
	}
	return evidence
}

func avTimingForStreamPlan(source, output avTimingEvidence, plan ResolvedStreamPlan) (avTimingEvidence, avTimingEvidence) {
	selectedSource := source
	selectedSource.Audio = []avStreamTiming{}
	selectedOutput := output
	selectedOutput.Audio = []avStreamTiming{}
	canonicalInputs := map[int]bool{}
	for _, stream := range plan.Audio {
		if stream.Action != "derive" {
			canonicalInputs[stream.InputIndex] = true
		}
	}
	for _, stream := range plan.Audio {
		if stream.Action == "derive" && canonicalInputs[stream.InputIndex] {
			continue
		}
		for _, audio := range source.Audio {
			if audio.Index == stream.InputIndex {
				selectedSource.Audio = append(selectedSource.Audio, audio)
				break
			}
		}
		if stream.OutputTypeIndex >= 0 && stream.OutputTypeIndex < len(output.Audio) {
			selectedOutput.Audio = append(selectedOutput.Audio, output.Audio[stream.OutputTypeIndex])
		}
	}
	return selectedSource, selectedOutput
}

func avTimingForSelectedAudio(source, output avTimingEvidence, inputIndexes []int) (avTimingEvidence, avTimingEvidence) {
	if len(inputIndexes) == 0 {
		return source, output
	}
	selectedSource := source
	selectedSource.Audio = []avStreamTiming{}
	for _, inputIndex := range inputIndexes {
		for _, audio := range source.Audio {
			if audio.Index == inputIndex {
				selectedSource.Audio = append(selectedSource.Audio, audio)
				break
			}
		}
	}
	selectedOutput := output
	if len(selectedOutput.Audio) > len(selectedSource.Audio) {
		selectedOutput.Audio = selectedOutput.Audio[:len(selectedSource.Audio)]
	}
	return selectedSource, selectedOutput
}

func validateAVTiming(source, output avTimingEvidence) models.JSONMap {
	report := models.JSONMap{"status": "unverified", "source": source, "output": output, "tracks": []models.JSONMap{}, "driftStatus": "not_measured"}
	if len(source.Audio) == 0 && len(output.Audio) == 0 {
		report["status"] = "not_applicable"
		report["driftStatus"] = "not_measured"
		return report
	}
	if source.Video == nil || output.Video == nil || len(source.Audio) == 0 || len(output.Audio) == 0 {
		return report
	}
	fps := parseFrameRateValue(output.FrameRate)
	if fps <= 0 {
		return report
	}
	frameDurationMs := 1000 / fps
	toleranceMs := frameDurationMs * initialAVTimingTolerance.ValidatedFrames
	trackCount := min(len(source.Audio), len(output.Audio))
	tracks := make([]models.JSONMap, 0, trackCount)
	overall := "validated"
	for index := 0; index < trackCount; index++ {
		sourceVideoStart, sourceVideoOK := videoPresentationStart(*source.Video)
		outputVideoStart, outputVideoOK := videoPresentationStart(*output.Video)
		sourceAudioStart, sourceAudioOK := audioPresentationStart(source.Audio[index])
		outputAudioStart, outputAudioOK := audioPresentationStart(output.Audio[index])
		if !sourceVideoOK || !outputVideoOK || !sourceAudioOK || !outputAudioOK {
			tracks = append(tracks, models.JSONMap{
				"sourceAudioIndex": source.Audio[index].Index, "outputAudioIndex": output.Audio[index].Index,
				"language": source.Audio[index].Language, "title": source.Audio[index].Title,
				"withinTolerance": false, "status": "unverified",
			})
			overall = higherAVTimingSeverity(overall, "unverified")
			continue
		}
		sourceOffset := (sourceAudioStart - sourceVideoStart) * 1000
		outputOffset := (outputAudioStart - outputVideoStart) * 1000
		introduced := outputOffset - sourceOffset
		equivalentFrames := introduced / frameDurationMs
		absoluteFrames := math.Abs(equivalentFrames)
		status := "validated"
		withinTolerance := absoluteFrames <= initialAVTimingTolerance.ValidatedFrames+avTimingFrameComparisonEpsilon
		if !withinTolerance {
			status = "warning"
		}
		if absoluteFrames > initialAVTimingTolerance.MismatchFrames+avTimingFrameComparisonEpsilon {
			status = "mismatch"
		}
		overall = higherAVTimingSeverity(overall, status)
		tracks = append(tracks, models.JSONMap{
			"sourceAudioIndex": source.Audio[index].Index, "outputAudioIndex": output.Audio[index].Index,
			"language": source.Audio[index].Language, "title": source.Audio[index].Title,
			"sourceOffsetMs": sourceOffset, "outputOffsetMs": outputOffset, "introducedOffsetMs": introduced,
			"introducedOffsetFrames": equivalentFrames, "withinTolerance": withinTolerance, "status": status,
		})
	}
	report["status"] = overall
	report["withinTolerance"] = overall == "validated"
	report["frameRateUsed"] = output.FrameRate
	report["frameDurationMs"] = frameDurationMs
	report["toleranceMs"] = toleranceMs
	report["toleranceFrames"] = initialAVTimingTolerance.ValidatedFrames
	report["mismatchFrames"] = initialAVTimingTolerance.MismatchFrames
	report["tracks"] = tracks
	report["driftStatus"] = "not_measured"
	return report
}

func videoPresentationStart(timing avStreamTiming) (float64, bool) {
	if timing.FirstPresentedSeconds != nil {
		return *timing.FirstPresentedSeconds, true
	}
	if timing.FirstPacketPTSSeconds != nil {
		return *timing.FirstPacketPTSSeconds, true
	}
	if timing.StartSeconds != nil {
		return *timing.StartSeconds, true
	}
	return 0, false
}

func audioPresentationStart(timing avStreamTiming) (float64, bool) {
	if timing.FirstPacketPTSSeconds != nil {
		return *timing.FirstPacketPTSSeconds, true
	}
	if timing.StartSeconds != nil {
		return *timing.StartSeconds, true
	}
	return 0, false
}

func secondsPointer(value float64) *float64 { return &value }

func higherAVTimingSeverity(current, candidate string) string {
	rank := map[string]int{"validated": 0, "unverified": 1, "warning": 2, "mismatch": 3}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}
