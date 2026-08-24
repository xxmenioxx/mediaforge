package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type InterlaceAnalysis struct {
	Version                      int               `json:"version"`
	Codec                        string            `json:"codec,omitempty"`
	AverageFrameRate             string            `json:"averageFrameRate,omitempty"`
	RealFrameRate                string            `json:"realFrameRate,omitempty"`
	Status                       string            `json:"status"`
	FieldOrder                   string            `json:"fieldOrder"`
	ContainerFieldOrder          string            `json:"containerFieldOrder"`
	DetectedFieldOrder           string            `json:"detectedFieldOrder,omitempty"`
	FieldOrderMismatch           bool              `json:"fieldOrderMismatch"`
	Source                       string            `json:"source"`
	Confidence                   float64           `json:"confidence"`
	TFF                          int               `json:"tff"`
	BFF                          int               `json:"bff"`
	Progressive                  int               `json:"progressive"`
	Undetermined                 int               `json:"undetermined"`
	RepeatedTop                  int               `json:"repeatedTop"`
	RepeatedBottom               int               `json:"repeatedBottom"`
	SampledFrames                int               `json:"sampledFrames"`
	WindowStart                  float64           `json:"windowStart"`
	WindowSeconds                int               `json:"windowSeconds"`
	SampleCount                  int               `json:"sampleCount"`
	FrameSignalSampleCount       int               `json:"frameSignalSampleCount"`
	SampledAt                    []float64         `json:"sampledAt,omitempty"`
	RecommendedMode              string            `json:"recommendedMode,omitempty"`
	RecommendedFieldMetadataMode string            `json:"recommendedFieldMetadataMode,omitempty"`
	RecommendedFilter            string            `json:"recommendedFilter,omitempty"`
	IVTCValidation               *IVTCValidation   `json:"ivtcValidation,omitempty"`
	Windows                      []InterlaceWindow `json:"windows,omitempty"`
	RecommendedAction            string            `json:"recommendedAction,omitempty"`
	DecisionReason               string            `json:"decisionReason,omitempty"`
	AutomaticFilter              string            `json:"automaticFilter,omitempty"`
}

const (
	interlaceAnalysisVersion     = 3
	interlaceAnalysisSnapshotKey = "interlaceAnalysisSnapshot"
)

type InterlaceWindow struct {
	Start          float64            `json:"start"`
	Seconds        int                `json:"seconds"`
	Status         string             `json:"status"`
	Confidence     float64            `json:"confidence"`
	TFF            int                `json:"tff"`
	BFF            int                `json:"bff"`
	Progressive    int                `json:"progressive"`
	Undetermined   int                `json:"undetermined"`
	RepeatedTop    int                `json:"repeatedTop"`
	RepeatedBottom int                `json:"repeatedBottom"`
	SampledFrames  int                `json:"sampledFrames"`
	FrameSignals   FrameSignalSummary `json:"frameSignals"`
}

type FrameSignalSummary struct {
	DecodedFrames       int                   `json:"decodedFrames"`
	InterlacedFrames    int                   `json:"interlacedFrames"`
	ProgressiveFrames   int                   `json:"progressiveFrames"`
	TopFieldFirstFrames int                   `json:"topFieldFirstFrames"`
	BottomFirstFrames   int                   `json:"bottomFirstFrames"`
	RepeatPictFrames    int                   `json:"repeatPictFrames"`
	Cadence             string                `json:"cadence,omitempty"`
	ActualTimespan      float64               `json:"actualTimespan,omitempty"`
	EffectiveFPS        float64               `json:"effectiveFps,omitempty"`
	TimestampDeltas     TimestampDeltaSummary `json:"timestampDeltas,omitempty"`
}

type TimestampDeltaSummary struct {
	Count    int       `json:"count"`
	Minimum  float64   `json:"minimum,omitempty"`
	Maximum  float64   `json:"maximum,omitempty"`
	Dominant []float64 `json:"dominant,omitempty"`
}

type IVTCValidation struct {
	TFFProgressive      int                    `json:"tffProgressive"`
	TFFClassified       int                    `json:"tffClassified"`
	TFFProgressiveRatio float64                `json:"tffProgressiveRatio"`
	BFFProgressive      int                    `json:"bffProgressive"`
	BFFClassified       int                    `json:"bffClassified"`
	BFFProgressiveRatio float64                `json:"bffProgressiveRatio"`
	SelectedOrder       string                 `json:"selectedOrder,omitempty"`
	Confidence          float64                `json:"confidence"`
	ValidatedWindows    int                    `json:"validatedWindows"`
	Windows             []IVTCWindowValidation `json:"windows,omitempty"`
}

type IVTCWindowValidation struct {
	Start               float64 `json:"start"`
	TFFProgressiveRatio float64 `json:"tffProgressiveRatio"`
	BFFProgressiveRatio float64 `json:"bffProgressiveRatio"`
	SelectedOrder       string  `json:"selectedOrder,omitempty"`
	Confidence          float64 `json:"confidence"`
}

type ffprobeFrameResponse struct {
	Frames []struct {
		InterlacedFrame     int    `json:"interlaced_frame"`
		TopFieldFirst       int    `json:"top_field_first"`
		RepeatPict          int    `json:"repeat_pict"`
		BestEffortTimestamp string `json:"best_effort_timestamp_time"`
		PacketDuration      string `json:"pkt_duration_time"`
	} `json:"frames"`
}

var (
	idetMultiPattern    = regexp.MustCompile(`Multi frame detection:\s*TFF:\s*(\d+)\s*BFF:\s*(\d+)\s*Progressive:\s*(\d+)\s*Undetermined:\s*(\d+)`)
	idetRepeatedPattern = regexp.MustCompile(`Repeated Fields:\s*Neither:\s*(\d+)\s*Top:\s*(\d+)\s*Bottom:\s*(\d+)`)
)

func detectInterlace(path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	return detectInterlaceContext(context.Background(), path, fieldOrder, duration, windowSeconds)
}

func detectInterlaceContext(ctx context.Context, path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	return detectInterlaceWithFrameSignalsContext(ctx, path, fieldOrder, duration, windowSeconds, true)
}

func detectInterlaceWithFrameSignalsContext(ctx context.Context, path, fieldOrder string, duration float64, windowSeconds int, collectFrameSignals bool) InterlaceAnalysis {
	windowSeconds = normalizedAnalysisSeconds(windowSeconds)
	containerOrder := normalizeFieldOrder(fieldOrder)
	analysis := InterlaceAnalysis{
		Version: interlaceAnalysisVersion,
		Status:  interlaceStatusFromFieldOrder(fieldOrder), FieldOrder: containerOrder,
		ContainerFieldOrder: containerOrder, Source: "ffprobe", WindowSeconds: windowSeconds,
	}
	for _, start := range distributedInterlaceStarts(duration, windowSeconds) {
		sample, ok := runIDETContext(ctx, path, start, windowSeconds, "idet")
		if !ok {
			continue
		}
		analysis.TFF += sample.TFF
		analysis.BFF += sample.BFF
		analysis.Progressive += sample.Progressive
		analysis.Undetermined += sample.Undetermined
		analysis.RepeatedTop += sample.RepeatedTop
		analysis.RepeatedBottom += sample.RepeatedBottom
		analysis.SampledFrames += sample.SampledFrames
		analysis.SampledAt = append(analysis.SampledAt, start)
		window := interlaceWindowFromSample(start, windowSeconds, sample)
		if collectFrameSignals {
			if signals, signalsOK := runFrameSignalsContext(ctx, path, start, windowSeconds); signalsOK {
				window.FrameSignals = signals
				analysis.FrameSignalSampleCount++
			}
		}
		classifyInterlaceWindow(&window)
		analysis.Windows = append(analysis.Windows, window)
	}
	analysis.SampleCount = len(analysis.SampledAt)
	if analysis.SampleCount == 0 {
		return analysis
	}
	analysis.WindowStart = analysis.SampledAt[0]
	analysis.Source = "idet_multi_sample"
	if analysis.FrameSignalSampleCount > 0 {
		analysis.Source = "idet_and_ffprobe_multi_sample"
	}
	analysis.DetectedFieldOrder = dominantFieldOrder(analysis.TFF, analysis.BFF)
	if analysis.DetectedFieldOrder != "" {
		analysis.FieldOrder = analysis.DetectedFieldOrder
		analysis.FieldOrderMismatch = fieldOrderFamily(containerOrder) != "" &&
			fieldOrderFamily(containerOrder) != analysis.DetectedFieldOrder
	}
	classifyInterlace(&analysis)
	finalizeFieldMetadataAnalysis(&analysis)
	if shouldValidateIVTC(analysis) {
		validateIVTCContext(ctx, path, duration, windowSeconds, &analysis)
	}
	return analysis
}

func decodeInterlaceAnalysis(value any) (InterlaceAnalysis, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return InterlaceAnalysis{}, false
	}
	var analysis InterlaceAnalysis
	if json.Unmarshal(encoded, &analysis) != nil || analysis.Version < interlaceAnalysisVersion || strings.TrimSpace(analysis.Status) == "" {
		return InterlaceAnalysis{}, false
	}
	return analysis, true
}

func interlaceWindowFromSample(start float64, seconds int, sample InterlaceAnalysis) InterlaceWindow {
	return InterlaceWindow{
		Start: start, Seconds: seconds, TFF: sample.TFF, BFF: sample.BFF,
		Progressive: sample.Progressive, Undetermined: sample.Undetermined,
		RepeatedTop: sample.RepeatedTop, RepeatedBottom: sample.RepeatedBottom,
		SampledFrames: sample.SampledFrames,
	}
}

func runFrameSignalsContext(parent context.Context, path string, start float64, seconds int) (FrameSignalSummary, bool) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(seconds+30)*time.Second)
	defer cancel()
	interval := fmt.Sprintf("%.3f%%+%d", start, seconds)
	args := []string{"-v", "error", "-select_streams", "v:0", "-read_intervals", interval,
		"-show_frames", "-show_entries", "frame=best_effort_timestamp_time,pkt_duration_time,interlaced_frame,top_field_first,repeat_pict", "-of", "json", path}
	output, err := exec.CommandContext(ctx, "ffprobe", args...).Output()
	if err != nil {
		return FrameSignalSummary{}, false
	}
	var response ffprobeFrameResponse
	if json.Unmarshal(output, &response) != nil || len(response.Frames) == 0 {
		return FrameSignalSummary{}, false
	}
	result := FrameSignalSummary{DecodedFrames: len(response.Frames)}
	firstTimestamp, lastTimestamp := -1.0, -1.0
	previousTimestamp := -1.0
	deltas := []float64{}
	repeatPositions := make([]int, 0)
	for index, frame := range response.Frames {
		if timestamp := parseFloat(frame.BestEffortTimestamp); timestamp >= 0 {
			if firstTimestamp < 0 {
				firstTimestamp = timestamp
			}
			lastTimestamp = timestamp
			if previousTimestamp >= 0 && timestamp > previousTimestamp {
				deltas = append(deltas, timestamp-previousTimestamp)
			}
			previousTimestamp = timestamp
		}
		if frame.InterlacedFrame != 0 {
			result.InterlacedFrames++
			if frame.TopFieldFirst != 0 {
				result.TopFieldFirstFrames++
			} else {
				result.BottomFirstFrames++
			}
		} else {
			result.ProgressiveFrames++
		}
		if frame.RepeatPict > 0 {
			result.RepeatPictFrames++
			repeatPositions = append(repeatPositions, index)
		}
	}
	result.Cadence = describeRepeatCadence(repeatPositions)
	if firstTimestamp >= 0 && lastTimestamp >= firstTimestamp {
		result.ActualTimespan = lastTimestamp - firstTimestamp
		if result.ActualTimespan > 0 {
			result.EffectiveFPS = float64(max(0, result.DecodedFrames-1)) / result.ActualTimespan
		}
	}
	result.TimestampDeltas = summarizeTimestampDeltas(deltas)
	return result, true
}

func summarizeTimestampDeltas(deltas []float64) TimestampDeltaSummary {
	result := TimestampDeltaSummary{Count: len(deltas)}
	if len(deltas) == 0 {
		return result
	}
	result.Minimum, result.Maximum = deltas[0], deltas[0]
	buckets := map[int]int{}
	for _, delta := range deltas {
		result.Minimum = math.Min(result.Minimum, delta)
		result.Maximum = math.Max(result.Maximum, delta)
		buckets[int(math.Round(delta*1000))]++
	}
	type bucketCount struct{ ms, count int }
	ranked := []bucketCount{}
	for ms, count := range buckets {
		ranked = append(ranked, bucketCount{ms: ms, count: count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count == ranked[j].count {
			return ranked[i].ms < ranked[j].ms
		}
		return ranked[i].count > ranked[j].count
	})
	for index := 0; index < min(2, len(ranked)); index++ {
		result.Dominant = append(result.Dominant, float64(ranked[index].ms)/1000)
	}
	sort.Float64s(result.Dominant)
	return result
}

func describeRepeatCadence(positions []int) string {
	if len(positions) < 3 {
		return "none_or_insufficient"
	}
	minGap, maxGap := positions[1]-positions[0], positions[1]-positions[0]
	for index := 2; index < len(positions); index++ {
		gap := positions[index] - positions[index-1]
		minGap = min(minGap, gap)
		maxGap = max(maxGap, gap)
	}
	if maxGap-minGap <= 1 {
		return fmt.Sprintf("stable_every_%d_frames", max(1, (minGap+maxGap)/2))
	}
	return "irregular"
}

func finalizeFieldMetadataAnalysis(analysis *InterlaceAnalysis) {
	containerOrder := normalizeFieldOrder(analysis.ContainerFieldOrder)
	if analysis.Status == "progressive" {
		analysis.DetectedFieldOrder = "progressive"
		analysis.FieldOrder = "progressive"
		analysis.FieldOrderMismatch = fieldOrderFamily(containerOrder) != ""
		if analysis.FieldOrderMismatch {
			analysis.RecommendedFieldMetadataMode = "progressive"
		}
	}
	if analysis.Status == "interlaced" && analysis.RecommendedMode == "force" {
		// The recommended bwdif pass emits progressive frames.
		analysis.RecommendedFieldMetadataMode = "progressive"
	}
}

func distributedInterlaceStarts(duration float64, windowSeconds int) []float64 {
	if duration <= float64(windowSeconds) || duration <= 0 {
		return []float64{0}
	}
	maxStart := max(0, duration-float64(windowSeconds))
	starts := []float64{}
	seen := map[int]bool{}
	for _, position := range []float64{0.05, 0.25, 0.50, 0.75, 0.90} {
		start := min(maxStart, max(0, duration*position-float64(windowSeconds)/2))
		key := int(start * 1000)
		if !seen[key] {
			seen[key] = true
			starts = append(starts, start)
		}
	}
	return starts
}

func runIDET(path string, start float64, seconds int, filter string) (InterlaceAnalysis, bool) {
	return runIDETContext(context.Background(), path, start, seconds, filter)
}

func runIDETContext(parent context.Context, path string, start float64, seconds int, filter string) (InterlaceAnalysis, bool) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(seconds+30)*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-ss", fmt.Sprintf("%.3f", start), "-i", path, "-map", "0:v:0", "-t", strconv.Itoa(seconds), "-vf", filter, "-an", "-sn", "-f", "null", "-"}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && ctx.Err() != nil {
		return InterlaceAnalysis{}, false
	}

	multiMatches := idetMultiPattern.FindAllStringSubmatch(stderr.String(), -1)
	if len(multiMatches) == 0 {
		return InterlaceAnalysis{}, false
	}
	multi := multiMatches[len(multiMatches)-1]
	if len(multi) != 5 {
		return InterlaceAnalysis{}, false
	}
	analysis := InterlaceAnalysis{}
	analysis.TFF = atoi(multi[1])
	analysis.BFF = atoi(multi[2])
	analysis.Progressive = atoi(multi[3])
	analysis.Undetermined = atoi(multi[4])
	analysis.SampledFrames = analysis.TFF + analysis.BFF + analysis.Progressive + analysis.Undetermined
	repeatedMatches := idetRepeatedPattern.FindAllStringSubmatch(stderr.String(), -1)
	if len(repeatedMatches) > 0 {
		repeated := repeatedMatches[len(repeatedMatches)-1]
		analysis.RepeatedTop = atoi(repeated[2])
		analysis.RepeatedBottom = atoi(repeated[3])
	}
	return analysis, true
}

func dominantFieldOrder(tff, bff int) string {
	total := tff + bff
	if total == 0 {
		return ""
	}
	if float64(tff)/float64(total) >= 0.65 {
		return "tff"
	}
	if float64(bff)/float64(total) >= 0.65 {
		return "bff"
	}
	return ""
}

func fieldOrderFamily(value string) string {
	switch normalizeFieldOrder(value) {
	case "tt", "tb", "tff":
		return "tff"
	case "bb", "bt", "bff":
		return "bff"
	default:
		return ""
	}
}

func shouldValidateIVTC(analysis InterlaceAnalysis) bool {
	classified := analysis.TFF + analysis.BFF + analysis.Progressive
	return classified > 0 && analysis.Status == "telecine_suspected"
}

func validateIVTC(path string, duration float64, seconds int, analysis *InterlaceAnalysis) {
	validateIVTCContext(context.Background(), path, duration, seconds, analysis)
}

func validateIVTCContext(ctx context.Context, path string, duration float64, seconds int, analysis *InterlaceAnalysis) {
	starts := validationStarts(analysis.SampledAt, duration, seconds)
	validation := &IVTCValidation{}
	for _, start := range starts {
		tff, tffOK := runIDETContext(ctx, path, start, seconds, "fieldmatch=order=tff,decimate,idet")
		bff, bffOK := runIDETContext(ctx, path, start, seconds, "fieldmatch=order=bff,decimate,idet")
		if !tffOK || !bffOK {
			continue
		}
		tffClassified := tff.TFF + tff.BFF + tff.Progressive
		bffClassified := bff.TFF + bff.BFF + bff.Progressive
		window := IVTCWindowValidation{Start: start}
		if tffClassified > 0 {
			window.TFFProgressiveRatio = float64(tff.Progressive) / float64(tffClassified)
		}
		if bffClassified > 0 {
			window.BFFProgressiveRatio = float64(bff.Progressive) / float64(bffClassified)
		}
		window.SelectedOrder, window.Confidence = validatedIVTCOrder(window.TFFProgressiveRatio, window.BFFProgressiveRatio)
		validation.Windows = append(validation.Windows, window)
		validation.TFFProgressive += tff.Progressive
		validation.TFFClassified += tffClassified
		validation.BFFProgressive += bff.Progressive
		validation.BFFClassified += bffClassified
	}
	if validation.TFFClassified == 0 || validation.BFFClassified == 0 {
		return
	}
	validation.TFFProgressiveRatio = float64(validation.TFFProgressive) / float64(validation.TFFClassified)
	validation.BFFProgressiveRatio = float64(validation.BFFProgressive) / float64(validation.BFFClassified)
	for _, window := range validation.Windows {
		if window.SelectedOrder != "" {
			validation.ValidatedWindows++
		}
	}
	applyIVTCValidation(analysis, validation)
	analysis.IVTCValidation = validation
}

func validationStarts(sampled []float64, duration float64, seconds int) []float64 {
	if len(sampled) <= 3 {
		if len(sampled) > 0 {
			return sampled
		}
		return []float64{max(0, duration/2-float64(seconds)/2)}
	}
	return []float64{sampled[0], sampled[len(sampled)/2], sampled[len(sampled)-1]}
}

func validatedIVTCOrder(tffRatio, bffRatio float64) (string, float64) {
	selected, best, other := "tff", tffRatio, bffRatio
	if bffRatio > tffRatio {
		selected, best, other = "bff", bffRatio, tffRatio
	}
	if best >= 0.85 && best-other >= 0.15 {
		return selected, best
	}
	return "", best
}

func applyIVTCValidation(analysis *InterlaceAnalysis, validation *IVTCValidation) {
	selected, best := validatedIVTCOrder(validation.TFFProgressiveRatio, validation.BFFProgressiveRatio)
	validation.Confidence = best
	if selected != "" && ivtcWindowsAgree(validation, selected) {
		validation.SelectedOrder = selected
		analysis.Status = "telecine"
		analysis.Confidence = best
		analysis.DetectedFieldOrder = selected
		analysis.FieldOrder = selected
		analysis.FieldOrderMismatch = fieldOrderFamily(analysis.ContainerFieldOrder) != "" &&
			fieldOrderFamily(analysis.ContainerFieldOrder) != selected
		analysis.RecommendedMode = "ivtc_" + selected
		analysis.RecommendedAction = "ivtc"
		analysis.DecisionReason = "inverse telecine was validated in distributed samples"
		analysis.RecommendedFieldMetadataMode = "progressive"
		analysis.RecommendedFilter = "fieldmatch=order=" + selected + ",decimate"
		analysis.AutomaticFilter = analysis.RecommendedFilter
	}
}

func ivtcWindowsAgree(validation *IVTCValidation, selected string) bool {
	if len(validation.Windows) == 0 {
		// Compatibility for v2 fixtures and persisted validation blocks.
		return true
	}
	required := 2
	if len(validation.Windows) == 1 {
		required = 1
	}
	agree := 0
	for _, window := range validation.Windows {
		if window.SelectedOrder == selected {
			agree++
		}
	}
	return agree >= required && agree*2 > len(validation.Windows)
}

func classifyInterlace(analysis *InterlaceAnalysis) {
	analysis.Version = interlaceAnalysisVersion
	if analysis.DetectedFieldOrder == "" {
		analysis.DetectedFieldOrder = dominantFieldOrder(analysis.TFF, analysis.BFF)
	}
	if len(analysis.Windows) > 0 {
		classifyDistributedInterlace(analysis)
		return
	}
	classified := analysis.TFF + analysis.BFF + analysis.Progressive
	if classified == 0 {
		analysis.Status = "unknown"
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "no classified frames were available"
		return
	}
	interlaced := analysis.TFF + analysis.BFF
	interlacedRatio := float64(interlaced) / float64(classified)
	repeatedRatio := float64(analysis.RepeatedTop+analysis.RepeatedBottom) / float64(max(analysis.SampledFrames, 1))
	switch {
	case repeatedRatio >= 0.15 && interlacedRatio >= 0.20:
		analysis.Status = "telecine_suspected"
		analysis.Confidence = repeatedRatio
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "repeated fields suggest cadence, but inverse telecine is not validated"
	case interlacedRatio >= 0.70:
		analysis.Status = "interlaced"
		analysis.Confidence = interlacedRatio
		analysis.RecommendedMode = "force"
		analysis.RecommendedAction = "deinterlace"
		analysis.DecisionReason = "interlacing is consistent in the sampled frames"
		analysis.RecommendedFilter = bwdifFilter(*analysis)
		analysis.AutomaticFilter = analysis.RecommendedFilter
	case interlacedRatio >= 0.10:
		analysis.Status = "hybrid"
		analysis.Confidence = max(interlacedRatio, 1-interlacedRatio)
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "progressive and interlaced evidence is mixed"
	case interlacedRatio < 0.10:
		analysis.Status = "progressive"
		analysis.Confidence = 1 - interlacedRatio
		analysis.RecommendedAction = "none"
		analysis.DecisionReason = "sampled frames are consistently progressive"
	}
}

func classifyInterlaceWindow(window *InterlaceWindow) {
	classified := window.TFF + window.BFF + window.Progressive
	if classified == 0 {
		classifyWindowFromFrameSignals(window)
		return
	}
	interlacedRatio := float64(window.TFF+window.BFF) / float64(classified)
	repeatedRatio := float64(window.RepeatedTop+window.RepeatedBottom) / float64(max(window.SampledFrames, 1))
	if window.FrameSignals.DecodedFrames > 0 {
		signalRatio := float64(window.FrameSignals.InterlacedFrames) / float64(window.FrameSignals.DecodedFrames)
		if (interlacedRatio >= 0.70 && signalRatio < 0.10) || (interlacedRatio < 0.10 && signalRatio >= 0.70) {
			window.Status = "unknown"
			window.Confidence = max(interlacedRatio, signalRatio)
			return
		}
	}
	switch {
	case repeatedRatio >= 0.15 && interlacedRatio >= 0.20:
		window.Status, window.Confidence = "telecine_suspected", max(repeatedRatio, interlacedRatio)
	case interlacedRatio >= 0.70:
		window.Status, window.Confidence = "interlaced", interlacedRatio
	case interlacedRatio >= 0.10:
		window.Status, window.Confidence = "hybrid", max(interlacedRatio, 1-interlacedRatio)
	default:
		window.Status, window.Confidence = "progressive", 1-interlacedRatio
	}
}

func classifyWindowFromFrameSignals(window *InterlaceWindow) {
	signals := window.FrameSignals
	if signals.DecodedFrames == 0 {
		window.Status = "unknown"
		return
	}
	interlacedRatio := float64(signals.InterlacedFrames) / float64(signals.DecodedFrames)
	repeatedRatio := float64(signals.RepeatPictFrames) / float64(signals.DecodedFrames)
	switch {
	case strings.HasPrefix(signals.Cadence, "stable_") && repeatedRatio >= 0.15 && interlacedRatio >= 0.20:
		window.Status, window.Confidence = "telecine_suspected", max(repeatedRatio, interlacedRatio)
	case interlacedRatio >= 0.70:
		window.Status, window.Confidence = "interlaced", interlacedRatio
	case interlacedRatio >= 0.10:
		window.Status, window.Confidence = "hybrid", max(interlacedRatio, 1-interlacedRatio)
	default:
		window.Status, window.Confidence = "progressive", 1-interlacedRatio
	}
}

func classifyDistributedInterlace(analysis *InterlaceAnalysis) {
	counts := map[string]int{}
	confidenceTotal := 0.0
	classifiedWindows := 0
	for index := range analysis.Windows {
		window := &analysis.Windows[index]
		if window.Status == "" {
			classifyInterlaceWindow(window)
		}
		if window.Status == "unknown" {
			continue
		}
		counts[window.Status]++
		confidenceTotal += window.Confidence
		classifiedWindows++
	}
	if classifiedWindows == 0 {
		analysis.Status = "unknown"
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "no sample window contained enough classified frames"
		return
	}
	analysis.Confidence = confidenceTotal / float64(classifiedWindows)
	hasProgressive := counts["progressive"] > 0
	hasMotion := counts["interlaced"]+counts["telecine_suspected"]+counts["hybrid"] > 0
	switch {
	case hasProgressive && hasMotion:
		analysis.Status = "hybrid"
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "distributed windows disagree between progressive and field-based content"
	case counts["hybrid"] > 0:
		analysis.Status = "hybrid"
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "at least one sample window contains mixed frame evidence"
	case counts["telecine_suspected"] > 0:
		analysis.Status = "telecine_suspected"
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "repeated-field cadence is suspected and requires distributed IVTC validation"
	case counts["interlaced"] == classifiedWindows:
		analysis.Status = "interlaced"
		analysis.RecommendedMode = "force"
		analysis.RecommendedAction = "deinterlace"
		analysis.DecisionReason = "every classified window is consistently interlaced"
		analysis.RecommendedFilter = bwdifFilter(*analysis)
		analysis.AutomaticFilter = analysis.RecommendedFilter
	case counts["progressive"] == classifiedWindows:
		analysis.Status = "progressive"
		analysis.RecommendedAction = "none"
		analysis.DecisionReason = "every classified window is consistently progressive"
	default:
		analysis.Status = "unknown"
		analysis.RecommendedAction = "review"
		analysis.DecisionReason = "distributed evidence is insufficient or contradictory"
	}
}

func interlaceStatusFromFieldOrder(value string) string {
	switch normalizeFieldOrder(value) {
	case "tt", "bb", "tb", "bt":
		return "interlaced"
	case "progressive":
		return "progressive"
	default:
		return "unknown"
	}
}

func normalizeFieldOrder(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func atoi(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func effectiveDeinterlaceFilter(profileMode string, analysis InterlaceAnalysis) string {
	switch strings.ToLower(strings.TrimSpace(profileMode)) {
	case "off", "disabled", "none":
		return ""
	case "ivtc", "ivtc_tff", "ivtc_bff", "inverse_telecine":
		// IVTC is carried in videoFilters so it can preserve the selected
		// field order and must not be prefixed with ordinary deinterlacing.
		return ""
	case "force", "forced", "on":
		return bwdifFilter(analysis)
	default:
		if automatic := strings.TrimSpace(analysis.AutomaticFilter); automatic != "" {
			return automatic
		}
		if analysis.Status == "telecine" && strings.TrimSpace(analysis.RecommendedFilter) != "" {
			return strings.TrimSpace(analysis.RecommendedFilter)
		}
		if analysis.Status == "interlaced" {
			return bwdifFilter(analysis)
		}
		// Hybrid, unknown, and unvalidated telecine are review states. Auto must
		// not choose a destructive motion filter without conclusive evidence.
		return ""
	}
}

func bwdifFilter(analysis InterlaceAnalysis) string {
	parity := analysis.DetectedFieldOrder
	if parity != "tff" && parity != "bff" {
		// Older snapshots did not persist detectedFieldOrder even when IDET's
		// frame counts provided decisive evidence. Never discard that evidence
		// and fall back to possibly incorrect container metadata.
		parity = dominantFieldOrder(analysis.TFF, analysis.BFF)
	}
	if parity != "tff" && parity != "bff" {
		parity = "auto"
	}
	return "bwdif=mode=send_frame:parity=" + parity + ":deint=all"
}
