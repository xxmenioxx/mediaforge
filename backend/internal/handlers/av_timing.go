package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type avStreamTiming struct {
	Index                 int      `json:"index"`
	TimeBase              string   `json:"timeBase,omitempty"`
	StartSeconds          *float64 `json:"startSeconds,omitempty"`
	FirstPacketPTSSeconds *float64 `json:"firstPacketPtsSeconds,omitempty"`
	FirstPacketDTSSeconds *float64 `json:"firstPacketDtsSeconds,omitempty"`
	FirstPresentedSeconds *float64 `json:"firstPresentedSeconds,omitempty"`
	FirstPacketPTSSource  string   `json:"firstPacketPtsSource,omitempty"`
	FirstPacketPTSOrigin  string   `json:"firstPacketPtsOrigin,omitempty"`
	FirstPresentedSource  string   `json:"firstPresentedSource,omitempty"`
	FirstPresentedOrigin  string   `json:"firstPresentedOrigin,omitempty"`
	Language              string   `json:"language,omitempty"`
	Title                 string   `json:"title,omitempty"`
}

const avTimingProbeWindowSeconds = 2.0
const avTimingProbeTimeout = 5 * time.Second

func probeAVTiming(path string, intervalStart float64) (avTimingEvidence, error) {
	interval := avTimingReadInterval(intervalStart)
	args := []string{
		"-v", "error", "-read_intervals", interval,
		"-show_entries", "stream=index,codec_type,start_time,time_base,avg_frame_rate:stream_tags=language,title:packet=stream_index,pts_time,dts_time:frame=media_type,stream_index,best_effort_timestamp_time",
		"-show_streams", "-show_packets", "-show_frames", "-of", "json", path,
	}
	ctx, cancel := context.WithTimeout(context.Background(), avTimingProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return avTimingEvidence{}, fmt.Errorf("probe A/V timing: %s", detail)
	}
	var response struct {
		Streams []struct {
			Index     int               `json:"index"`
			CodecType string            `json:"codec_type"`
			StartTime string            `json:"start_time"`
			TimeBase  string            `json:"time_base"`
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
		PacketsAndFrames []struct {
			Type        string `json:"type"`
			MediaType   string `json:"media_type"`
			StreamIndex int    `json:"stream_index"`
			PTS         string `json:"pts_time"`
			DTS         string `json:"dts_time"`
			Presented   string `json:"best_effort_timestamp_time"`
		} `json:"packets_and_frames"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return avTimingEvidence{}, fmt.Errorf("parse A/V timing probe: %w", err)
	}
	evidence := avTimingEvidence{Audio: []avStreamTiming{}}
	byIndex := map[int]*avStreamTiming{}
	for _, stream := range response.Streams {
		timing := avStreamTiming{Index: stream.Index, TimeBase: stream.TimeBase, StartSeconds: optionalSeconds(stream.StartTime), Language: stream.Tags["language"], Title: stream.Tags["title"]}
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
		if (frame.MediaType == "video" || frame.MediaType == "audio") && timing != nil && timing.FirstPresentedSeconds == nil {
			if presented := optionalSeconds(frame.Presented); presented != nil {
				timing.FirstPresentedSeconds = presented
				timing.FirstPresentedSource = "frame.best_effort_timestamp_time"
			}
		}
	}
	for _, item := range response.PacketsAndFrames {
		timing := byIndex[item.StreamIndex]
		switch item.Type {
		case "packet":
			if timing != nil {
				captureFirstPacketTimes(timing, item.PTS, item.DTS)
			}
		case "frame":
			if (item.MediaType == "video" || item.MediaType == "audio") && timing != nil && timing.FirstPresentedSeconds == nil {
				if presented := optionalSeconds(item.Presented); presented != nil {
					timing.FirstPresentedSeconds = presented
					timing.FirstPresentedSource = "frame.best_effort_timestamp_time"
				}
			}
		}
	}
	markAVTimingPresentationOrigin(&evidence, "bounded_ffprobe_fallback")
	evidence.PresentationProbeStatus = "completed"
	return evidence, nil
}

func avTimingReadInterval(intervalStart float64) string {
	window := strconv.FormatFloat(avTimingProbeWindowSeconds, 'f', -1, 64)
	if intervalStart > 0 {
		return strconv.FormatFloat(intervalStart, 'f', -1, 64) + "%+" + window
	}
	return "%+" + window
}

func captureFirstPacketTimes(timing *avStreamTiming, pts, dts string) {
	if timing.FirstPacketPTSSeconds == nil {
		if timestamp := optionalSeconds(pts); timestamp != nil {
			timing.FirstPacketPTSSeconds = timestamp
			timing.FirstPacketPTSSource = "packet.pts_time"
		}
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
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return nil
	}
	return &seconds
}

type avTimingEvidence struct {
	Video                   *avStreamTiming  `json:"video,omitempty"`
	Audio                   []avStreamTiming `json:"audio"`
	FrameRate               string           `json:"frameRate,omitempty"`
	PresentationProbeStatus string           `json:"presentationProbeStatus,omitempty"`
}

type avTimingTolerancePolicy struct {
	ValidatedFrames float64
	MismatchFrames  float64
}

var initialAVTimingTolerance = avTimingTolerancePolicy{
	ValidatedFrames: 1,
	MismatchFrames:  2,
}

// A/V timestamps are quantized to each stream's time_base. The four timestamps
// in an introduced-offset comparison can therefore contribute half a tick of
// uncertainty each. Cap that allowance at 1/16 frame so coarse or malformed
// time bases can never move the policy boundary by a meaningful fraction of a
// frame. Floating point comparison itself is handled with one ULP below.
const avTimingMaximumQuantizationFrameFraction = 1.0 / 16.0

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
	return avTimingFromProbeWithOrigin(probe, "stored_probe")
}

func avTimingFromProbeWithOrigin(probe map[string]any, origin string) avTimingEvidence {
	evidence := avTimingEvidence{Audio: []avStreamTiming{}}
	streams, _ := probe["streams"].([]interface{})
	byIndex := map[int]*avStreamTiming{}
	for _, raw := range streams {
		stream := unknownRecord(raw)
		if stream == nil {
			continue
		}
		timing := avStreamTiming{
			Index: workerIntValue(stream["index"], -1), TimeBase: stringFromUnknown(stream["time_base"]),
			StartSeconds: optionalSeconds(stringFromUnknown(stream["start_time"])),
		}
		if tags := unknownRecord(stream["tags"]); tags != nil {
			timing.Language = stringFromUnknown(tags["language"])
			timing.Title = stringFromUnknown(tags["title"])
		}
		switch stringFromUnknown(stream["codec_type"]) {
		case "video":
			if evidence.Video == nil {
				evidence.Video = &timing
				evidence.FrameRate = stringFromUnknown(stream["avg_frame_rate"])
				byIndex[timing.Index] = evidence.Video
			}
		case "audio":
			evidence.Audio = append(evidence.Audio, timing)
		}
	}
	for index := range evidence.Audio {
		byIndex[evidence.Audio[index].Index] = &evidence.Audio[index]
	}
	packets, _ := probe["packets"].([]interface{})
	for _, raw := range packets {
		packet := unknownRecord(raw)
		if packet == nil {
			continue
		}
		if timing := byIndex[workerIntValue(packet["stream_index"], -1)]; timing != nil {
			captureFirstPacketTimes(timing, stringFromUnknown(packet["pts_time"]), stringFromUnknown(packet["dts_time"]))
		}
	}
	frames, _ := probe["frames"].([]interface{})
	for _, raw := range frames {
		frame := unknownRecord(raw)
		if frame == nil {
			continue
		}
		mediaType := stringFromUnknown(frame["media_type"])
		if mediaType != "video" && mediaType != "audio" {
			continue
		}
		timing := byIndex[workerIntValue(frame["stream_index"], -1)]
		if timing == nil || timing.FirstPresentedSeconds != nil {
			continue
		}
		if presented := optionalSeconds(stringFromUnknown(frame["best_effort_timestamp_time"])); presented != nil {
			timing.FirstPresentedSeconds = presented
			timing.FirstPresentedSource = "frame.best_effort_timestamp_time"
		}
	}
	markAVTimingPresentationOrigin(&evidence, origin)
	return evidence
}

func markAVTimingPresentationOrigin(evidence *avTimingEvidence, origin string) {
	mark := func(timing *avStreamTiming) {
		if timing.FirstPresentedSeconds != nil && timing.FirstPresentedOrigin == "" {
			timing.FirstPresentedOrigin = origin
		}
		if timing.FirstPacketPTSSeconds != nil && timing.FirstPacketPTSOrigin == "" {
			timing.FirstPacketPTSOrigin = origin
		}
	}
	if evidence.Video != nil {
		mark(evidence.Video)
	}
	for index := range evidence.Audio {
		mark(&evidence.Audio[index])
	}
}

func avTimingWithBoundedPresentationFallback(source, output avTimingEvidence, sourcePath, outputPath string, probe func(string, float64) (avTimingEvidence, error)) (avTimingEvidence, avTimingEvidence) {
	if len(source.Audio) == 0 && len(output.Audio) == 0 {
		return source, output
	}
	if avTimingNeedsPresentationProbe(source) && sourcePath != "" {
		measured, err := probe(sourcePath, 0)
		if err != nil {
			source.PresentationProbeStatus = "failed"
		} else {
			mergeAVTimingPresentationEvidence(&source, measured)
		}
	}
	if avTimingNeedsPresentationProbe(output) && outputPath != "" {
		measured, err := probe(outputPath, 0)
		if err != nil {
			output.PresentationProbeStatus = "failed"
		} else {
			mergeAVTimingPresentationEvidence(&output, measured)
		}
	}
	return source, output
}

func avTimingNeedsPresentationProbe(evidence avTimingEvidence) bool {
	if evidence.Video == nil || evidence.Video.FirstPresentedSeconds == nil {
		return true
	}
	for _, audio := range evidence.Audio {
		if audio.FirstPresentedSeconds == nil && audio.FirstPacketPTSSeconds == nil {
			return true
		}
	}
	return false
}

func mergeAVTimingPresentationEvidence(target *avTimingEvidence, measured avTimingEvidence) {
	target.PresentationProbeStatus = "completed"
	if target.FrameRate == "" {
		target.FrameRate = measured.FrameRate
	}
	if target.Video == nil && measured.Video != nil {
		video := *measured.Video
		target.Video = &video
	} else if target.Video != nil && measured.Video != nil && (target.Video.Index < 0 || target.Video.Index == measured.Video.Index) {
		// Both probes retain only their first video stream. If the stored probe
		// omitted its index, the bounded fallback still identifies that same
		// first stream without guessing among multiple candidates.
		mergeAVStreamPresentationEvidence(target.Video, *measured.Video)
	}
	for index := range target.Audio {
		for _, measuredAudio := range measured.Audio {
			if target.Audio[index].Index == measuredAudio.Index {
				mergeAVStreamPresentationEvidence(&target.Audio[index], measuredAudio)
				break
			}
		}
	}
}

func mergeAVStreamPresentationEvidence(target *avStreamTiming, measured avStreamTiming) {
	if target.FirstPresentedSeconds == nil && measured.FirstPresentedSeconds != nil {
		target.FirstPresentedSeconds = measured.FirstPresentedSeconds
		target.FirstPresentedSource = measured.FirstPresentedSource
		target.FirstPresentedOrigin = measured.FirstPresentedOrigin
	}
	if target.FirstPacketPTSSeconds == nil && measured.FirstPacketPTSSeconds != nil {
		target.FirstPacketPTSSeconds = measured.FirstPacketPTSSeconds
		target.FirstPacketPTSSource = measured.FirstPacketPTSSource
		target.FirstPacketPTSOrigin = measured.FirstPacketPTSOrigin
	}
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
		sourceTiming := avStreamTiming{Index: stream.InputIndex, Language: stream.Language, Title: stream.Title}
		for _, audio := range source.Audio {
			if audio.Index == stream.InputIndex {
				sourceTiming = audio
				break
			}
		}
		outputTiming := avStreamTiming{Index: stream.OutputIndex, Language: stream.Language, Title: stream.Title}
		for _, audio := range output.Audio {
			if audio.Index == stream.OutputIndex {
				outputTiming = audio
				break
			}
		}
		// Append the pair atomically so missing evidence for one planned stream
		// cannot shift every comparison that follows it.
		selectedSource.Audio = append(selectedSource.Audio, sourceTiming)
		selectedOutput.Audio = append(selectedOutput.Audio, outputTiming)
	}
	return selectedSource, selectedOutput
}

func avTimingForQueueValidation(source, output avTimingEvidence, plan ResolvedStreamPlan, planAvailable bool) (avTimingEvidence, avTimingEvidence) {
	if planAvailable {
		return avTimingForStreamPlan(source, output, plan)
	}
	// Without the frozen AS-IS plan, output positions cannot be paired safely
	// after removal, reordering, or derivation. Remove source audio evidence so
	// validation fails closed instead of comparing the positional common prefix.
	source.Audio = nil
	return source, output
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
	return validateAVTimingWithThresholds(source, output, initialAVTimingTolerance.ValidatedFrames, initialAVTimingTolerance.MismatchFrames)
}

func validateAVTimingWithThresholds(source, output avTimingEvidence, passFrames, warningFrames float64) models.JSONMap {
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
	if passFrames < 0 || warningFrames < passFrames {
		passFrames, warningFrames = initialAVTimingTolerance.ValidatedFrames, initialAVTimingTolerance.MismatchFrames
	}
	frameDurationMs := 1000 / fps
	toleranceMs := frameDurationMs * passFrames
	trackCount := min(len(source.Audio), len(output.Audio))
	tracks := make([]models.JSONMap, 0, trackCount)
	overall := "validated"
	if len(source.Audio) != len(output.Audio) {
		overall = "unverified"
	}
	for index := 0; index < trackCount; index++ {
		sourceVideoStart, sourceVideoOK := videoPresentationStart(*source.Video)
		outputVideoStart, outputVideoOK := videoPresentationStart(*output.Video)
		sourceAudioStart, sourceAudioOK := audioPresentationStart(source.Audio[index])
		outputAudioStart, outputAudioOK := audioPresentationStart(output.Audio[index])
		validatedBoundaryMs := frameDurationMs * passFrames
		mismatchBoundaryMs := frameDurationMs * warningFrames
		quantizationToleranceMs := avTimingQuantizationToleranceMs(
			frameDurationMs,
			*source.Video, source.Audio[index], *output.Video, output.Audio[index],
		)
		evidenceUsed := models.JSONMap{
			"sourceVideo": presentationTimestampEvidence(*source.Video, true),
			"sourceAudio": presentationTimestampEvidence(source.Audio[index], false),
			"outputVideo": presentationTimestampEvidence(*output.Video, true),
			"outputAudio": presentationTimestampEvidence(output.Audio[index], false),
		}
		if !sourceVideoOK || !outputVideoOK || !sourceAudioOK || !outputAudioOK {
			tracks = append(tracks, models.JSONMap{
				"sourceAudioIndex": source.Audio[index].Index, "outputAudioIndex": output.Audio[index].Index,
				"language": source.Audio[index].Language, "title": source.Audio[index].Title,
				"passToleranceFrames": passFrames, "passToleranceMs": validatedBoundaryMs,
				"warningToleranceFrames": warningFrames, "warningToleranceMs": mismatchBoundaryMs,
				"timestampQuantizationToleranceMs": quantizationToleranceMs,
				"evidenceUsed":                     evidenceUsed, "withinTolerance": false,
				"decision": "unverified", "status": "unverified",
			})
			overall = higherAVTimingSeverity(overall, "unverified")
			continue
		}
		sourceOffset := (sourceAudioStart - sourceVideoStart) * 1000
		outputOffset := (outputAudioStart - outputVideoStart) * 1000
		introduced := outputOffset - sourceOffset
		equivalentFrames := introduced / frameDurationMs
		status := "validated"
		absoluteOffsetMs := math.Abs(introduced)
		withinTolerance := atOrBelowAVTimingBoundary(absoluteOffsetMs, validatedBoundaryMs)
		if !withinTolerance {
			status = "warning"
		}
		if exceedsAVTimingBoundary(absoluteOffsetMs, mismatchBoundaryMs+quantizationToleranceMs) {
			status = "mismatch"
		}
		overall = higherAVTimingSeverity(overall, status)
		tracks = append(tracks, models.JSONMap{
			"sourceAudioIndex": source.Audio[index].Index, "outputAudioIndex": output.Audio[index].Index,
			"language": source.Audio[index].Language, "title": source.Audio[index].Title,
			"sourceOffsetMs": sourceOffset, "outputOffsetMs": outputOffset, "introducedOffsetMs": introduced,
			"introducedOffsetFrames": equivalentFrames, "timestampQuantizationToleranceMs": quantizationToleranceMs,
			"passToleranceFrames": passFrames, "passToleranceMs": validatedBoundaryMs,
			"warningToleranceFrames": warningFrames, "warningToleranceMs": mismatchBoundaryMs,
			"evidenceUsed": evidenceUsed, "withinTolerance": withinTolerance,
			"decision": status, "status": status,
		})
	}
	report["status"] = overall
	report["withinTolerance"] = overall == "validated"
	report["frameRateUsed"] = output.FrameRate
	report["frameDurationMs"] = frameDurationMs
	report["toleranceMs"] = toleranceMs
	report["toleranceFrames"] = passFrames
	report["mismatchFrames"] = warningFrames
	report["tracks"] = tracks
	report["driftStatus"] = "not_measured"
	return report
}

func avTimingQuantizationToleranceMs(frameDurationMs float64, timings ...avStreamTiming) float64 {
	if frameDurationMs <= 0 || math.IsNaN(frameDurationMs) || math.IsInf(frameDurationMs, 0) {
		return 0
	}
	uncertaintySeconds := 0.0
	for _, timing := range timings {
		if quantum := avTimingTimeBaseSeconds(timing.TimeBase); quantum > 0 {
			uncertaintySeconds += quantum / 2
		}
	}
	maximumSeconds := frameDurationMs / 1000 * avTimingMaximumQuantizationFrameFraction
	return math.Min(uncertaintySeconds, maximumSeconds) * 1000
}

func avTimingTimeBaseSeconds(value string) float64 {
	numeratorText, denominatorText, ok := strings.Cut(strings.TrimSpace(value), "/")
	if !ok {
		return 0
	}
	numerator, numeratorErr := strconv.ParseFloat(strings.TrimSpace(numeratorText), 64)
	denominator, denominatorErr := strconv.ParseFloat(strings.TrimSpace(denominatorText), 64)
	if numeratorErr != nil || denominatorErr != nil || numerator <= 0 || denominator <= 0 {
		return 0
	}
	seconds := numerator / denominator
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	return seconds
}

func atOrBelowAVTimingBoundary(value, boundary float64) bool {
	return value <= math.Nextafter(boundary, math.Inf(1))
}

func exceedsAVTimingBoundary(value, boundary float64) bool {
	return value > math.Nextafter(boundary, math.Inf(1))
}

func videoPresentationStart(timing avStreamTiming) (float64, bool) {
	if timing.FirstPresentedSeconds != nil {
		return *timing.FirstPresentedSeconds, true
	}
	return 0, false
}

func presentationTimestampEvidence(timing avStreamTiming, video bool) models.JSONMap {
	if video {
		if timing.FirstPresentedSeconds == nil {
			return models.JSONMap{"available": false}
		}
		return presentationEvidence(timing.FirstPresentedSeconds, timing.FirstPresentedSource, timing.FirstPresentedOrigin)
	}
	if timing.FirstPresentedSeconds != nil {
		return presentationEvidence(timing.FirstPresentedSeconds, timing.FirstPresentedSource, timing.FirstPresentedOrigin)
	}
	if timing.FirstPacketPTSSeconds != nil {
		return presentationEvidence(timing.FirstPacketPTSSeconds, timing.FirstPacketPTSSource, timing.FirstPacketPTSOrigin)
	}
	return models.JSONMap{"available": false}
}

func presentationEvidence(timestamp *float64, field, origin string) models.JSONMap {
	evidence := models.JSONMap{"available": true, "timestampSeconds": *timestamp, "unit": "seconds"}
	if field != "" {
		evidence["field"] = field
	}
	if origin != "" {
		evidence["origin"] = origin
	}
	return evidence
}

func audioPresentationStart(timing avStreamTiming) (float64, bool) {
	if timing.FirstPresentedSeconds != nil {
		return *timing.FirstPresentedSeconds, true
	}
	if timing.FirstPacketPTSSeconds != nil {
		return *timing.FirstPacketPTSSeconds, true
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
