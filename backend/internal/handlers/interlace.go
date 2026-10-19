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
	ResidualInterlacedFrames     int               `json:"residualInterlacedFrames,omitempty"`
	CanonicalOutputFrames        int               `json:"canonicalOutputFrames,omitempty"`
	CanonicalOutputProgressive   int               `json:"canonicalOutputProgressiveFrames,omitempty"`
	CanonicalOutputFrameRate     float64           `json:"canonicalOutputFrameRate,omitempty"`
	CanonicalTimestampAnomalies  int               `json:"canonicalTimestampAnomalies,omitempty"`
	RepeatedTop                  int               `json:"repeatedTop"`
	RepeatedBottom               int               `json:"repeatedBottom"`
	SampledFrames                int               `json:"sampledFrames"`
	WindowStart                  float64           `json:"windowStart"`
	WindowSeconds                int               `json:"windowSeconds"`
	SampleCount                  int               `json:"sampleCount"`
	FrameSignalSampleCount       int               `json:"frameSignalSampleCount"`
	AnalysisDepth                string            `json:"analysisDepth,omitempty"`
	WindowsRequested             int               `json:"windowsRequested,omitempty"`
	MaximumWindows               int               `json:"maximumWindows,omitempty"`
	DeepAnalysisTriggered        bool              `json:"deepAnalysisTriggered,omitempty"`
	DepthReason                  string            `json:"depthReason,omitempty"`
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
	interlaceAnalysisDepthQuick = "quick"
	interlaceAnalysisDepthDeep  = "deep"
)

const (
	ivtcDeepEvidenceVersion        = 2
	ivtcDeepMaximumExtraWindows    = 2
	ivtcDeepMinimumValidWindows    = 3
	ivtcWindowOutcomeValid         = "valid"
	ivtcWindowOutcomeInconclusive  = "inconclusive"
	ivtcWindowOutcomeContradictory = "contradictory"
)

const (
	interlaceAnalysisVersion      = 3
	interlaceAnalysisCacheVersion = 4
	interlaceAnalysisSnapshotKey  = "interlaceAnalysisSnapshot"
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
	DeepAnalysisVersion       int                    `json:"deepAnalysisVersion,omitempty"`
	DeepAnalysisStatus        string                 `json:"deepAnalysisStatus,omitempty"`
	EvidenceSnapshotUpdatedAt string                 `json:"evidenceSnapshotUpdatedAt,omitempty"`
	SourceFrameRate           string                 `json:"sourceFrameRate,omitempty"`
	ExpectedOutputFrameRate   string                 `json:"expectedOutputFrameRate,omitempty"`
	SourceIdentity            *IVTCSourceIdentity    `json:"sourceIdentity,omitempty"`
	DurationSeconds           float64                `json:"durationSeconds,omitempty"`
	WindowSeconds             int                    `json:"windowSeconds,omitempty"`
	BaselineWindowCount       int                    `json:"baselineWindowCount,omitempty"`
	AttemptedWindows          int                    `json:"attemptedWindows,omitempty"`
	InconclusiveWindows       int                    `json:"inconclusiveWindows,omitempty"`
	ContradictoryWindows      int                    `json:"contradictoryWindows,omitempty"`
	TFFProgressive            int                    `json:"tffProgressive"`
	TFFClassified             int                    `json:"tffClassified"`
	TFFProgressiveRatio       float64                `json:"tffProgressiveRatio"`
	BFFProgressive            int                    `json:"bffProgressive"`
	BFFClassified             int                    `json:"bffClassified"`
	BFFProgressiveRatio       float64                `json:"bffProgressiveRatio"`
	SelectedOrder             string                 `json:"selectedOrder,omitempty"`
	Confidence                float64                `json:"confidence"`
	ValidWindows              int                    `json:"validWindows"`
	ValidatedWindows          int                    `json:"validatedWindows"`
	Windows                   []IVTCWindowValidation `json:"windows,omitempty"`
}

type IVTCSourceIdentity struct {
	Path              string `json:"path"`
	SnapshotID        uint   `json:"snapshotId"`
	AssetRecordID     uint   `json:"assetRecordId,omitempty"`
	SnapshotUpdatedAt string `json:"snapshotUpdatedAt"`
	SizeBytes         int64  `json:"sizeBytes"`
	ModifiedAt        string `json:"modifiedAt"`
	Mode              uint32 `json:"mode"`
	Device            string `json:"device"`
	Inode             string `json:"inode"`
}

type IVTCWindowValidation struct {
	Start                          float64 `json:"start"`
	Outcome                        string  `json:"outcome,omitempty"`
	Reason                         string  `json:"reason,omitempty"`
	SelectionBasis                 string  `json:"selectionBasis,omitempty"`
	DiagnosticsAvailable           bool    `json:"diagnosticsAvailable,omitempty"`
	TFFProgressive                 int     `json:"tffProgressive,omitempty"`
	TFFClassified                  int     `json:"tffClassified,omitempty"`
	TFFResidualInterlaced          int     `json:"tffResidualInterlaced,omitempty"`
	TFFRepeatedTop                 int     `json:"tffRepeatedTop,omitempty"`
	TFFRepeatedBottom              int     `json:"tffRepeatedBottom,omitempty"`
	TFFCanonicalOutputFrames       int     `json:"tffCanonicalOutputFrames,omitempty"`
	TFFCanonicalOutputProgressive  int     `json:"tffCanonicalOutputProgressiveFrames,omitempty"`
	TFFCanonicalOutputFrameRate    float64 `json:"tffCanonicalOutputFrameRate,omitempty"`
	TFFCanonicalTimestampAnomalies int     `json:"tffCanonicalTimestampAnomalies,omitempty"`
	TFFCanonicalIDETInterlaced     int     `json:"tffCanonicalIdetInterlacedFrames,omitempty"`
	TFFCanonicalIDETClassified     int     `json:"tffCanonicalIdetClassifiedFrames,omitempty"`
	BFFProgressive                 int     `json:"bffProgressive,omitempty"`
	BFFClassified                  int     `json:"bffClassified,omitempty"`
	BFFResidualInterlaced          int     `json:"bffResidualInterlaced,omitempty"`
	BFFRepeatedTop                 int     `json:"bffRepeatedTop,omitempty"`
	BFFRepeatedBottom              int     `json:"bffRepeatedBottom,omitempty"`
	BFFCanonicalOutputFrames       int     `json:"bffCanonicalOutputFrames,omitempty"`
	BFFCanonicalOutputProgressive  int     `json:"bffCanonicalOutputProgressiveFrames,omitempty"`
	BFFCanonicalOutputFrameRate    float64 `json:"bffCanonicalOutputFrameRate,omitempty"`
	BFFCanonicalTimestampAnomalies int     `json:"bffCanonicalTimestampAnomalies,omitempty"`
	BFFCanonicalIDETInterlaced     int     `json:"bffCanonicalIdetInterlacedFrames,omitempty"`
	BFFCanonicalIDETClassified     int     `json:"bffCanonicalIdetClassifiedFrames,omitempty"`
	TFFProgressiveRatio            float64 `json:"tffProgressiveRatio"`
	BFFProgressiveRatio            float64 `json:"bffProgressiveRatio"`
	SelectedOrder                  string  `json:"selectedOrder,omitempty"`
	Confidence                     float64 `json:"confidence"`
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
	idetMultiPattern        = regexp.MustCompile(`Multi frame detection:\s*TFF:\s*(\d+)\s*BFF:\s*(\d+)\s*Progressive:\s*(\d+)\s*Undetermined:\s*(\d+)`)
	idetRepeatedPattern     = regexp.MustCompile(`Repeated Fields:\s*Neither:\s*(\d+)\s*Top:\s*(\d+)\s*Bottom:\s*(\d+)`)
	fieldmatchResidualFrame = regexp.MustCompile(`Frame #\d+ at [^\s]+ is still interlaced`)
	showinfoFramePattern    = regexp.MustCompile(`showinfo.*n:\s*(\d+)\s+pts:\s*(-?\d+)\s+pts_time:([^\s]+)\s+duration:\s*(-?\d+)\s+duration_time:([^\s]+).*?\bi:([PTB])`)
)

func detectInterlace(path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	return detectInterlaceContext(context.Background(), path, fieldOrder, duration, windowSeconds)
}

func detectInterlaceContext(ctx context.Context, path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	return detectInterlaceWithFrameSignalsContext(ctx, path, fieldOrder, duration, windowSeconds, true)
}

func detectInterlaceWithFrameSignalsContext(ctx context.Context, path, fieldOrder string, duration float64, windowSeconds int, collectFrameSignals bool) InterlaceAnalysis {
	plan := canonicalSamplingPlan(duration, defaultFrameStructureSamplingPolicy())
	return detectInterlaceWithSamplingPlanContext(ctx, path, fieldOrder, windowSeconds, collectFrameSignals, plan)
}

func detectInterlaceWithSamplingPlanContext(ctx context.Context, path, fieldOrder string, windowSeconds int, collectFrameSignals bool, plan SamplingPlan) InterlaceAnalysis {
	return detectInterlaceWithSharedFrameEvidenceContext(ctx, path, fieldOrder, windowSeconds, collectFrameSignals, plan, QSVFrameStructureAnalysis{})
}

func detectInterlaceWithSharedFrameEvidenceContext(ctx context.Context, path, fieldOrder string, windowSeconds int, collectFrameSignals bool, plan SamplingPlan, frameStructure QSVFrameStructureAnalysis) InterlaceAnalysis {
	return detectInterlaceWithSharedEvidenceContext(ctx, path, fieldOrder, windowSeconds, collectFrameSignals, plan, frameStructure, nil)
}

func adaptiveInterlaceSamplingPlan(plan SamplingPlan, frameStructure QSVFrameStructureAnalysis, fieldOrder string) (SamplingPlan, string, string) {
	deep := func(reason string) (SamplingPlan, string, string) {
		return plan, interlaceAnalysisDepthDeep, reason
	}
	if len(plan.Positions) <= 2 {
		return deep("canonical plan already requires two or fewer windows")
	}
	if plan.InterlaceValidation == "always" {
		return deep("analysis policy requires full interlace validation")
	}
	if frameStructure.ConfidenceScore < plan.EarlyConfidenceThreshold {
		return deep("shared frame confidence is below the quick-validation threshold")
	}
	if frameStructure.WindowCount < 3 || len(frameStructure.Windows) < 3 {
		return deep("shared frame evidence has fewer than three regions")
	}
	if fieldOrderFamily(fieldOrder) != "" {
		return deep("container field order requires deep validation")
	}
	minimumFPS, maximumFPS := 0.0, 0.0
	for _, window := range frameStructure.Windows {
		signals := window.FrameSignals
		if signals.DecodedFrames <= 0 || signals.EffectiveFPS <= 0 {
			return deep("shared frame evidence is incomplete")
		}
		if signals.InterlacedFrames > 0 || signals.ProgressiveFrames < signals.DecodedFrames {
			return deep("shared frame flags are interlaced or mixed")
		}
		if signals.RepeatPictFrames > 0 {
			return deep("repeat_pict requires telecine validation")
		}
		cadence := strings.ToLower(strings.TrimSpace(signals.Cadence))
		if strings.Contains(cadence, "mixed") || strings.Contains(cadence, "telecine") || strings.Contains(cadence, "unstable") {
			return deep("shared cadence evidence is suspicious")
		}
		if minimumFPS == 0 || signals.EffectiveFPS < minimumFPS {
			minimumFPS = signals.EffectiveFPS
		}
		if signals.EffectiveFPS > maximumFPS {
			maximumFPS = signals.EffectiveFPS
		}
	}
	if maximumFPS-minimumFPS > 0.50 {
		return deep("effective FPS varies across shared regions")
	}
	quickWindows := representativeSamplingWindows(plan, plan.WindowSeconds, 2)
	quickPlan := plan
	quickPlan.Positions = make([]float64, 0, len(quickWindows))
	for _, window := range quickWindows {
		quickPlan.Positions = append(quickPlan.Positions, window.Position)
	}
	quickPlan.Adaptive = false
	quickPlan.InitialWindows = len(quickPlan.Positions)
	return quickPlan, interlaceAnalysisDepthQuick, "high-confidence progressive shared evidence"
}

func detectInterlaceWithSharedEvidenceContext(ctx context.Context, path, fieldOrder string, windowSeconds int, collectFrameSignals bool, plan SamplingPlan, frameStructure QSVFrameStructureAnalysis, pixelSession *pixelSamplingSession) InterlaceAnalysis {
	windowSeconds = normalizedAnalysisSeconds(windowSeconds)
	containerOrder := normalizeFieldOrder(fieldOrder)
	analysis := InterlaceAnalysis{
		Version: interlaceAnalysisVersion,
		Status:  interlaceStatusFromFieldOrder(fieldOrder), FieldOrder: containerOrder,
		ContainerFieldOrder: containerOrder, Source: "ffprobe", WindowSeconds: windowSeconds,
	}
	analysis.WindowsRequested = len(plan.Positions)
	sharedFrameSignalSamples := 0
	for _, samplingWindow := range plan.windows(float64(windowSeconds)) {
		start := samplingWindow.StartSeconds
		var sample InterlaceAnalysis
		var ok bool
		if pixelSession != nil {
			sample, ok = pixelSession.interlaceSample(ctx, samplingWindow, windowSeconds)
		} else {
			sample, ok = runIDETContext(ctx, path, start, windowSeconds, "idet")
		}
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
		if signals, ok := sharedFrameSignalsForPosition(frameStructure, samplingWindow.Position); ok {
			window.FrameSignals = signals
			analysis.FrameSignalSampleCount++
			sharedFrameSignalSamples++
		} else if collectFrameSignals {
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
	if pixelSession != nil && pixelSession.sharedDecodes > 0 && sharedFrameSignalSamples > 0 {
		analysis.Source = "idet_shared_pixel_and_frame_probe"
	} else if pixelSession != nil && pixelSession.sharedDecodes > 0 {
		analysis.Source = "idet_shared_pixel_probe"
	} else if sharedFrameSignalSamples > 0 {
		analysis.Source = "idet_and_shared_frame_probe"
	} else if analysis.FrameSignalSampleCount > 0 {
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
		validateIVTCContext(ctx, path, plan.AssetDurationSeconds, windowSeconds, &analysis)
	}
	return analysis
}

func sharedFrameSignalsForPosition(frameStructure QSVFrameStructureAnalysis, position float64) (FrameSignalSummary, bool) {
	for _, window := range frameStructure.Windows {
		if math.Abs(window.Position-position) < 0.000001 && window.FrameSignals.DecodedFrames > 0 {
			return window.FrameSignals, true
		}
	}
	return FrameSignalSummary{}, false
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
	windows := canonicalSamplingPlan(duration, defaultFrameStructureSamplingPolicy()).windows(float64(windowSeconds))
	starts := make([]float64, 0, len(windows))
	for _, window := range windows {
		starts = append(starts, window.StartSeconds)
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
	return parseIDETOutput(stderr.String())
}

func parseIDETOutput(output string) (InterlaceAnalysis, bool) {
	multiMatches := idetMultiPattern.FindAllStringSubmatch(output, -1)
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
	analysis.ResidualInterlacedFrames = len(fieldmatchResidualFrame.FindAllString(output, -1))
	repeatedMatches := idetRepeatedPattern.FindAllStringSubmatch(output, -1)
	if len(repeatedMatches) > 0 {
		repeated := repeatedMatches[len(repeatedMatches)-1]
		analysis.RepeatedTop = atoi(repeated[2])
		analysis.RepeatedBottom = atoi(repeated[3])
	}
	parseCanonicalOutputTimeline(output, &analysis)
	return analysis, true
}

type canonicalOutputFrame struct {
	index       int
	PTS         float64
	duration    float64
	progressive bool
}

func parseCanonicalOutputTimeline(output string, analysis *InterlaceAnalysis) {
	frames := make([]canonicalOutputFrame, 0)
	for _, match := range showinfoFramePattern.FindAllStringSubmatch(output, -1) {
		index, indexErr := strconv.Atoi(match[1])
		pts, ptsErr := strconv.ParseFloat(match[3], 64)
		duration, durationErr := strconv.ParseFloat(match[5], 64)
		if indexErr != nil || ptsErr != nil || durationErr != nil {
			continue
		}
		frames = append(frames, canonicalOutputFrame{index: index, PTS: pts, duration: duration, progressive: match[6] == "P"})
	}
	analysis.CanonicalOutputFrames = len(frames)
	for _, frame := range frames {
		if frame.progressive {
			analysis.CanonicalOutputProgressive++
		}
	}
	for index := 1; index < len(frames); index++ {
		previous, current := frames[index-1], frames[index]
		if current.index != previous.index+1 || previous.duration <= 0 || current.PTS <= previous.PTS || math.Abs((current.PTS-previous.PTS)-previous.duration) > 0.000001 {
			analysis.CanonicalTimestampAnomalies++
		}
	}
	if len(frames) == 0 {
		return
	}
	elapsed := frames[len(frames)-1].PTS + frames[len(frames)-1].duration - frames[0].PTS
	if elapsed > 0 {
		analysis.CanonicalOutputFrameRate = float64(len(frames)) / elapsed
	}
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
	validation := &IVTCValidation{DurationSeconds: duration, WindowSeconds: seconds}
	_ = appendIVTCValidationWindows(ctx, path, seconds, validation, starts, runIDETContext)
	finalizeIVTCValidation(analysis, validation)
}

type ivtcWindowRunner func(context.Context, string, float64, int, string) (InterlaceAnalysis, bool)

func appendIVTCValidationWindows(ctx context.Context, path string, seconds int, validation *IVTCValidation, starts []float64, run ivtcWindowRunner) error {
	for _, start := range starts {
		window, err := compareIVTCWindow(ctx, path, start, seconds, run)
		if err != nil {
			return err
		}
		validation.Windows = append(validation.Windows, window)
		validation.TFFProgressive += window.TFFProgressive
		validation.TFFClassified += window.TFFClassified
		validation.BFFProgressive += window.BFFProgressive
		validation.BFFClassified += window.BFFClassified
	}
	return nil
}

func compareIVTCWindow(ctx context.Context, path string, start float64, seconds int, run ivtcWindowRunner) (IVTCWindowValidation, error) {
	window := IVTCWindowValidation{Start: start, Outcome: ivtcWindowOutcomeInconclusive}
	tff, tffOK := run(ctx, path, start, seconds, "fieldmatch=order=tff,decimate,idet")
	if err := ctx.Err(); err != nil {
		return window, err
	}
	bff, bffOK := run(ctx, path, start, seconds, "fieldmatch=order=bff,decimate,idet")
	if err := ctx.Err(); err != nil {
		return window, err
	}
	if !tffOK || !bffOK {
		window.Reason = "TFF/BFF comparison did not produce parseable idet evidence"
		return window, nil
	}
	tffCanonical, tffCanonicalOK := run(ctx, path, start, seconds, canonicalIVTCDiagnosticFilter("tff"))
	if err := ctx.Err(); err != nil {
		return window, err
	}
	bffCanonical, bffCanonicalOK := run(ctx, path, start, seconds, canonicalIVTCDiagnosticFilter("bff"))
	if err := ctx.Err(); err != nil {
		return window, err
	}
	if !tffCanonicalOK || !bffCanonicalOK || tffCanonical.CanonicalOutputFrames == 0 || bffCanonical.CanonicalOutputFrames == 0 {
		window.Reason = "canonical IVTC residual/timestamp diagnostics were unavailable"
		return window, nil
	}
	window.TFFProgressive = tff.Progressive
	window.TFFClassified = tff.TFF + tff.BFF + tff.Progressive
	window.BFFProgressive = bff.Progressive
	window.BFFClassified = bff.TFF + bff.BFF + bff.Progressive
	populateIVTCCanonicalDiagnostics(&window, tffCanonical, bffCanonical)
	if !window.DiagnosticsAvailable {
		window.Reason = "canonical IVTC output frame rate or progressive-frame evidence was unavailable"
		return window, nil
	}
	if window.TFFClassified == 0 || window.BFFClassified == 0 {
		window.Reason = "TFF/BFF comparison produced no classified frames"
		return window, nil
	}
	if window.TFFCanonicalTimestampAnomalies > 0 || window.BFFCanonicalTimestampAnomalies > 0 {
		window.Reason = "canonical IVTC output has non-monotonic or irregular frame timestamps"
		return window, nil
	}
	window.TFFProgressiveRatio = float64(window.TFFProgressive) / float64(window.TFFClassified)
	window.BFFProgressiveRatio = float64(window.BFFProgressive) / float64(window.BFFClassified)
	window.SelectedOrder, window.Confidence = validatedIVTCOrder(window.TFFProgressiveRatio, window.BFFProgressiveRatio)
	if window.SelectedOrder == "" {
		window.Reason = "per-window evidence did not meet the existing confidence and margin thresholds"
		return window, nil
	}
	window.Outcome = ivtcWindowOutcomeValid
	return window, nil
}

func canonicalIVTCDiagnosticFilter(order string) string {
	return "fieldmatch=order=" + order + ":combmatch=full,idet,bwdif=mode=send_frame:parity=" + order + ":deint=interlaced,decimate,showinfo,setfield=prog"
}

func populateIVTCCanonicalDiagnostics(window *IVTCWindowValidation, tff, bff InterlaceAnalysis) {
	tffIDETClassified := tff.TFF + tff.BFF + tff.Progressive
	bffIDETClassified := bff.TFF + bff.BFF + bff.Progressive
	window.DiagnosticsAvailable = tff.CanonicalOutputFrames > 0 && bff.CanonicalOutputFrames > 0 &&
		tff.CanonicalOutputFrameRate > 0 && bff.CanonicalOutputFrameRate > 0 &&
		tffIDETClassified > 0 && bffIDETClassified > 0 &&
		tff.CanonicalOutputProgressive == tff.CanonicalOutputFrames && bff.CanonicalOutputProgressive == bff.CanonicalOutputFrames
	window.TFFResidualInterlaced = tff.ResidualInterlacedFrames
	window.TFFRepeatedTop = tff.RepeatedTop
	window.TFFRepeatedBottom = tff.RepeatedBottom
	window.TFFCanonicalOutputFrames = tff.CanonicalOutputFrames
	window.TFFCanonicalOutputProgressive = tff.CanonicalOutputProgressive
	window.TFFCanonicalOutputFrameRate = tff.CanonicalOutputFrameRate
	window.TFFCanonicalTimestampAnomalies = tff.CanonicalTimestampAnomalies
	window.TFFCanonicalIDETInterlaced = tff.TFF + tff.BFF
	window.TFFCanonicalIDETClassified = tffIDETClassified
	window.BFFResidualInterlaced = bff.ResidualInterlacedFrames
	window.BFFRepeatedTop = bff.RepeatedTop
	window.BFFRepeatedBottom = bff.RepeatedBottom
	window.BFFCanonicalOutputFrames = bff.CanonicalOutputFrames
	window.BFFCanonicalOutputProgressive = bff.CanonicalOutputProgressive
	window.BFFCanonicalOutputFrameRate = bff.CanonicalOutputFrameRate
	window.BFFCanonicalTimestampAnomalies = bff.CanonicalTimestampAnomalies
	window.BFFCanonicalIDETInterlaced = bff.TFF + bff.BFF
	window.BFFCanonicalIDETClassified = bffIDETClassified
}

func finalizeIVTCValidation(analysis *InterlaceAnalysis, validation *IVTCValidation) {
	validation.AttemptedWindows = len(validation.Windows)
	if validation.TFFClassified > 0 {
		validation.TFFProgressiveRatio = float64(validation.TFFProgressive) / float64(validation.TFFClassified)
	}
	if validation.BFFClassified > 0 {
		validation.BFFProgressiveRatio = float64(validation.BFFProgressive) / float64(validation.BFFClassified)
	}
	applyIVTCValidation(analysis, validation)
	analysis.IVTCValidation = validation
}

type ivtcAdditionalWindow struct {
	Start          float64
	Seconds        int
	SelectionBasis string
	priority       int
}

func selectDeepIVTCAdditionalWindows(analysis InterlaceAnalysis, plan SamplingPlan, maximum int) []ivtcAdditionalWindow {
	maximum = min(maximum, ivtcDeepMaximumExtraWindows)
	if maximum <= 0 || analysis.IVTCValidation == nil {
		return nil
	}
	seconds := max(1, analysis.IVTCValidation.WindowSeconds)
	if analysis.IVTCValidation.WindowSeconds <= 0 {
		seconds = max(1, analysis.WindowSeconds)
	}
	used := make([]float64, 0, len(analysis.IVTCValidation.Windows))
	for _, window := range analysis.IVTCValidation.Windows {
		used = append(used, window.Start)
	}
	candidates := make([]ivtcAdditionalWindow, 0, len(plan.SelectedWindows)+len(analysis.SampledAt)+len(plan.Positions))
	add := func(start, windowSeconds float64, basis string, priority int) {
		if start < 0 || (plan.AssetDurationSeconds > 0 && start+windowSeconds > plan.AssetDurationSeconds) {
			return
		}
		for _, existing := range used {
			if math.Abs(existing-start) < 0.5 {
				return
			}
		}
		for candidateIndex := range candidates {
			existing := &candidates[candidateIndex]
			if math.Abs(existing.Start-start) < 0.5 {
				if priority < existing.priority {
					existing.priority = priority
					existing.SelectionBasis = basis
				}
				return
			}
		}
		candidates = append(candidates, ivtcAdditionalWindow{Start: start, Seconds: max(1, int(math.Round(windowSeconds))), SelectionBasis: basis, priority: priority})
	}
	for _, window := range plan.SelectedWindows {
		basis, priority := "canonical_sampling_plan", 3
		switch {
		case containsSamplingString(window.Roles, SamplingRoleHighMotion):
			basis, priority = "existing_high_motion_sampling_evidence", 0
		case containsSamplingString(window.Roles, SamplingRoleHighComplexity) || containsSamplingString(window.Roles, SamplingRoleWorstPredicted) || containsSamplingString(window.Roles, SamplingRoleHighTextureOrGrain):
			basis, priority = "existing_high_value_sampling_evidence", 2
		}
		add(window.StartSeconds, window.DurationSeconds, basis, priority)
	}
	for _, window := range analysis.Windows {
		basis, priority := "existing_distributed_interlace_sample", 3
		cadence := strings.ToLower(strings.TrimSpace(window.FrameSignals.Cadence))
		if window.FrameSignals.RepeatPictFrames > 0 || strings.Contains(cadence, "telecine") || strings.Contains(cadence, "mixed") || window.FrameSignals.InterlacedFrames > 0 {
			basis, priority = "existing_temporal_cadence_evidence", 1
		}
		add(window.Start, float64(max(1, window.Seconds)), basis, priority)
	}
	for _, start := range analysis.SampledAt {
		add(start, float64(seconds), "existing_distributed_interlace_sample", 3)
	}
	for _, window := range plan.windows(float64(seconds)) {
		add(window.StartSeconds, window.DurationSeconds, "distributed_canonical_fallback", 4)
	}

	selected := make([]ivtcAdditionalWindow, 0, maximum)
	for len(selected) < maximum && len(candidates) > 0 {
		best := 0
		bestDistance := -1.0
		for index, candidate := range candidates {
			distance := math.MaxFloat64
			for _, start := range used {
				if current := math.Abs(candidate.Start - start); current < distance {
					distance = current
				}
			}
			if len(used) == 0 {
				distance = candidate.Start
			}
			if candidate.priority < candidates[best].priority || (candidate.priority == candidates[best].priority && distance > bestDistance) {
				best, bestDistance = index, distance
			}
		}
		chosen := candidates[best]
		selected = append(selected, chosen)
		used = append(used, chosen.Start)
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Start < selected[j].Start })
	return selected
}

func extendIVTCValidationContext(ctx context.Context, path string, analysis *InterlaceAnalysis, plan SamplingPlan, identity *IVTCSourceIdentity, run ivtcWindowRunner, progress ...func(completed, total int)) error {
	if analysis == nil || analysis.IVTCValidation == nil || len(analysis.IVTCValidation.Windows) < 3 {
		return fmt.Errorf("deep cadence analysis requires the existing three IVTC windows")
	}
	if analysis.IVTCValidation.DeepAnalysisVersion == ivtcDeepEvidenceVersion {
		return fmt.Errorf("deep cadence analysis is already current for this snapshot")
	}
	additional := selectDeepIVTCAdditionalWindows(*analysis, plan, ivtcDeepMaximumExtraWindows)
	if len(additional) == 0 {
		return fmt.Errorf("deep cadence analysis has no unused sampling windows")
	}
	validation := *analysis.IVTCValidation
	validation.Windows = append([]IVTCWindowValidation(nil), validation.Windows...)
	validation.DeepAnalysisVersion = ivtcDeepEvidenceVersion
	validation.SourceIdentity = identity
	validation.BaselineWindowCount = len(validation.Windows)
	if validation.DurationSeconds <= 0 {
		validation.DurationSeconds = plan.AssetDurationSeconds
	}
	if validation.WindowSeconds <= 0 {
		validation.WindowSeconds = analysis.WindowSeconds
	}
	if err := refreshIVTCCanonicalDiagnostics(ctx, path, validation.WindowSeconds, validation.Windows, run); err != nil {
		return err
	}
	for index, candidate := range additional {
		window, err := compareIVTCWindow(ctx, path, candidate.Start, candidate.Seconds, run)
		if err != nil {
			return err
		}
		window.SelectionBasis = candidate.SelectionBasis
		validation.Windows = append(validation.Windows, window)
		validation.TFFProgressive += window.TFFProgressive
		validation.TFFClassified += window.TFFClassified
		validation.BFFProgressive += window.BFFProgressive
		validation.BFFClassified += window.BFFClassified
		if len(progress) > 0 && progress[0] != nil {
			progress[0](index+1, len(additional))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	finalizeIVTCValidation(analysis, &validation)
	return nil
}

func refreshIVTCCanonicalDiagnostics(ctx context.Context, path string, seconds int, windows []IVTCWindowValidation, run ivtcWindowRunner) error {
	for index := range windows {
		window := &windows[index]
		tff, tffOK := run(ctx, path, window.Start, seconds, canonicalIVTCDiagnosticFilter("tff"))
		if err := ctx.Err(); err != nil {
			return err
		}
		bff, bffOK := run(ctx, path, window.Start, seconds, canonicalIVTCDiagnosticFilter("bff"))
		if err := ctx.Err(); err != nil {
			return err
		}
		if !tffOK || !bffOK || tff.CanonicalOutputFrames == 0 || bff.CanonicalOutputFrames == 0 {
			window.DiagnosticsAvailable = false
			if window.SelectedOrder != "" {
				window.SelectedOrder = ""
			}
			window.Outcome = ivtcWindowOutcomeInconclusive
			window.Reason = "canonical IVTC residual/timestamp diagnostics were unavailable"
			continue
		}
		populateIVTCCanonicalDiagnostics(window, tff, bff)
		if !window.DiagnosticsAvailable {
			window.SelectedOrder = ""
			window.Outcome = ivtcWindowOutcomeInconclusive
			window.Reason = "canonical IVTC output frame rate or progressive-frame evidence was unavailable"
			continue
		}
		if window.TFFCanonicalTimestampAnomalies > 0 || window.BFFCanonicalTimestampAnomalies > 0 {
			window.SelectedOrder = ""
			window.Outcome = ivtcWindowOutcomeInconclusive
			window.Reason = "canonical IVTC output has non-monotonic or irregular frame timestamps"
		}
	}
	return nil
}

func resetStaleDeepIVTCEvidence(analysis *InterlaceAnalysis) bool {
	if analysis == nil || analysis.IVTCValidation == nil {
		return false
	}
	validation := analysis.IVTCValidation
	baselineCount := validation.BaselineWindowCount
	if validation.DeepAnalysisVersion != ivtcDeepEvidenceVersion || baselineCount < 3 || baselineCount > len(validation.Windows) {
		return false
	}
	validation.Windows = append([]IVTCWindowValidation(nil), validation.Windows[:baselineCount]...)
	validation.DeepAnalysisVersion = 0
	validation.DeepAnalysisStatus = "stale"
	validation.EvidenceSnapshotUpdatedAt = ""
	validation.SourceFrameRate = ""
	validation.ExpectedOutputFrameRate = ""
	validation.SourceIdentity = nil
	validation.BaselineWindowCount = 0
	validation.TFFProgressive, validation.TFFClassified = 0, 0
	validation.BFFProgressive, validation.BFFClassified = 0, 0
	for index := range validation.Windows {
		window := &validation.Windows[index]
		window.DiagnosticsAvailable = false
		window.TFFResidualInterlaced, window.TFFRepeatedTop, window.TFFRepeatedBottom = 0, 0, 0
		window.BFFResidualInterlaced, window.BFFRepeatedTop, window.BFFRepeatedBottom = 0, 0, 0
		window.TFFCanonicalOutputFrames, window.TFFCanonicalOutputProgressive = 0, 0
		window.TFFCanonicalOutputFrameRate, window.TFFCanonicalTimestampAnomalies = 0, 0
		window.TFFCanonicalIDETInterlaced, window.TFFCanonicalIDETClassified = 0, 0
		window.BFFCanonicalOutputFrames, window.BFFCanonicalOutputProgressive = 0, 0
		window.BFFCanonicalOutputFrameRate, window.BFFCanonicalTimestampAnomalies = 0, 0
		window.BFFCanonicalIDETInterlaced, window.BFFCanonicalIDETClassified = 0, 0
		validation.TFFProgressive += window.TFFProgressive
		validation.TFFClassified += window.TFFClassified
		validation.BFFProgressive += window.BFFProgressive
		validation.BFFClassified += window.BFFClassified
	}
	if validation.TFFClassified > 0 {
		validation.TFFProgressiveRatio = float64(validation.TFFProgressive) / float64(validation.TFFClassified)
	} else {
		validation.TFFProgressiveRatio = 0
	}
	if validation.BFFClassified > 0 {
		validation.BFFProgressiveRatio = float64(validation.BFFProgressive) / float64(validation.BFFClassified)
	} else {
		validation.BFFProgressiveRatio = 0
	}
	analysis.Status = "telecine_suspected"
	analysis.FieldOrder = ""
	analysis.DetectedFieldOrder = ""
	analysis.FieldOrderMismatch = false
	analysis.RecommendedMode = ""
	analysis.RecommendedAction = "review"
	analysis.RecommendedFieldMetadataMode = ""
	analysis.RecommendedFilter = ""
	analysis.AutomaticFilter = ""
	analysis.DecisionReason = "deep IVTC evidence is stale for this snapshot revision"
	finalizeIVTCValidation(analysis, validation)
	return true
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
	classifyIVTCValidationWindows(validation, selected)
	if selected != "" && ivtcWindowsAgree(validation, selected) && (validation.DeepAnalysisVersion == 0 || deepIVTCWindowsSufficient(validation, selected)) {
		validation.SelectedOrder = selected
		if validation.DeepAnalysisVersion > 0 {
			validation.DeepAnalysisStatus = "resolved"
		}
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
		analysis.RecommendedFilter = "fieldmatch=order=" + selected + ":combmatch=full,bwdif=mode=send_frame:parity=" + selected + ":deint=interlaced,decimate,setfield=prog"
		analysis.AutomaticFilter = analysis.RecommendedFilter
		return
	}
	validation.SelectedOrder = ""
	if validation.DeepAnalysisVersion > 0 {
		validation.DeepAnalysisStatus = "review_required"
		analysis.Status = "telecine_suspected"
		analysis.DetectedFieldOrder = ""
		analysis.FieldOrderMismatch = false
		analysis.RecommendedMode = ""
		analysis.RecommendedAction = "review"
		analysis.RecommendedFieldMetadataMode = ""
		analysis.RecommendedFilter = ""
		analysis.AutomaticFilter = ""
		if validation.ContradictoryWindows > 0 {
			analysis.DecisionReason = "deep IVTC windows confidently selected different field orders; cadence may be mixed and requires asset-scoped review"
		} else if validation.InconclusiveWindows > 0 {
			analysis.DecisionReason = "deep IVTC evidence remained insufficient: one or more windows did not meet the existing confidence and margin thresholds"
		} else {
			analysis.DecisionReason = "deep IVTC evidence did not meet the existing aggregate confidence, agreement, or distribution requirements"
		}
	}
}

func classifyIVTCValidationWindows(validation *IVTCValidation, selected string) {
	validation.ValidatedWindows = 0
	validation.ValidWindows = 0
	validation.InconclusiveWindows = 0
	validation.ContradictoryWindows = 0
	orders := map[string]bool{}
	for _, window := range validation.Windows {
		if window.SelectedOrder != "" {
			orders[window.SelectedOrder] = true
		}
	}
	for index := range validation.Windows {
		window := &validation.Windows[index]
		switch {
		case window.SelectedOrder == "":
			window.Outcome = ivtcWindowOutcomeInconclusive
			validation.InconclusiveWindows++
		case selected != "" && window.SelectedOrder != selected:
			window.Outcome = ivtcWindowOutcomeContradictory
			window.Reason = "confident per-window order contradicts the aggregate order"
			validation.ValidatedWindows++
			validation.ContradictoryWindows++
		case selected == "" && len(orders) > 1:
			window.Outcome = ivtcWindowOutcomeContradictory
			window.Reason = "confident windows selected different field orders"
			validation.ValidatedWindows++
			validation.ContradictoryWindows++
		default:
			window.Outcome = ivtcWindowOutcomeValid
			window.Reason = ""
			validation.ValidWindows++
			validation.ValidatedWindows++
		}
	}
}

func deepIVTCWindowsSufficient(validation *IVTCValidation, selected string) bool {
	if validation == nil || validation.DeepAnalysisVersion != ivtcDeepEvidenceVersion || validation.ValidWindows < ivtcDeepMinimumValidWindows || validation.ContradictoryWindows > 0 {
		return false
	}
	starts := make([]float64, 0, validation.ValidatedWindows)
	for _, window := range validation.Windows {
		if !window.DiagnosticsAvailable || window.TFFCanonicalTimestampAnomalies > 0 || window.BFFCanonicalTimestampAnomalies > 0 {
			return false
		}
		if window.SelectedOrder == selected && window.Outcome == ivtcWindowOutcomeValid {
			starts = append(starts, window.Start)
		}
	}
	if len(starts) < ivtcDeepMinimumValidWindows {
		return false
	}
	sort.Float64s(starts)
	minimumSpan := float64(max(1, validation.WindowSeconds) * 2)
	if validation.DurationSeconds > 0 && validation.DurationSeconds*0.4 > minimumSpan {
		minimumSpan = validation.DurationSeconds * 0.4
	}
	return starts[len(starts)-1]-starts[0] >= minimumSpan
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
