package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

type QSVFrameStructureAnalysis struct {
	Version               int                    `json:"version"`
	SampleLimit           int                    `json:"sampleLimit"`
	FramesAnalyzed        int                    `json:"framesAnalyzed"`
	IFrames               int                    `json:"iFrames"`
	PFrames               int                    `json:"pFrames"`
	BFrames               int                    `json:"bFrames"`
	KeyFrames             int                    `json:"keyFrames"`
	BFrameRatio           float64                `json:"bFrameRatio"`
	HasBFrames            bool                   `json:"hasBFrames"`
	MaxConsecutiveBFrames int                    `json:"maxConsecutiveBFrames"`
	AverageGOPLength      float64                `json:"averageGopLength"`
	MinimumGOPLength      int                    `json:"minimumGopLength"`
	MaximumGOPLength      int                    `json:"maximumGopLength"`
	CompleteGOPs          int                    `json:"completeGops"`
	SampledSeconds        float64                `json:"sampledSeconds,omitempty"`
	AssetDurationSeconds  float64                `json:"assetDurationSeconds,omitempty"`
	CoverageRatio         float64                `json:"coverageRatio,omitempty"`
	WindowCount           int                    `json:"windowCount,omitempty"`
	WindowLengthSeconds   float64                `json:"windowLengthSeconds,omitempty"`
	Positions             []float64              `json:"positions,omitempty"`
	Windows               []FrameStructureWindow `json:"windows,omitempty"`
	Variability           string                 `json:"variability,omitempty"`
	Confidence            string                 `json:"confidence,omitempty"`
	ConfidenceScore       float64                `json:"confidenceScore,omitempty"`
	ProcessCount          int                    `json:"processCount,omitempty"`
	WindowsRequested      int                    `json:"windowsRequested,omitempty"`
	EarlyStopped          bool                   `json:"earlyStopped,omitempty"`
	DeepAnalysisTriggered bool                   `json:"deepAnalysisTriggered,omitempty"`
	FrameSignals          FrameSignalSummary     `json:"frameSignals,omitempty"`
	Assessment            string                 `json:"assessment"`
	Source                string                 `json:"source"`
}

type FrameStructureWindow struct {
	Position        float64                   `json:"position"`
	StartSeconds    float64                   `json:"startSeconds"`
	DurationSeconds float64                   `json:"durationSeconds"`
	Analysis        QSVFrameStructureAnalysis `json:"analysis"`
	FrameSignals    FrameSignalSummary        `json:"frameSignals"`
}

type frameStructureProbeFrame struct {
	PictureType         string `json:"pict_type"`
	KeyFrame            int    `json:"key_frame"`
	InterlacedFrame     int    `json:"interlaced_frame"`
	TopFieldFirst       int    `json:"top_field_first"`
	RepeatPict          int    `json:"repeat_pict"`
	BestEffortTimestamp string `json:"best_effort_timestamp_time"`
	PacketDuration      string `json:"pkt_duration_time"`
}

type FrameStructureSamplingPolicy struct {
	Windows       int       `json:"windows"`
	WindowSeconds float64   `json:"windowSeconds"`
	Positions     []float64 `json:"positions"`
	Adaptive      bool      `json:"adaptive"`
}

func defaultFrameStructureSamplingPolicy() FrameStructureSamplingPolicy {
	return FrameStructureSamplingPolicy{Windows: 5, WindowSeconds: 20, Positions: []float64{0.08, 0.27, 0.50, 0.73, 0.92}, Adaptive: true}
}

func frameStructureSamplingPolicy(db *gorm.DB) FrameStructureSamplingPolicy {
	policy := defaultFrameStructureSamplingPolicy()
	if db == nil {
		return policy
	}
	var setting models.AppSetting
	if err := db.Where("key = ?", "frameStructureSampling").First(&setting).Error; err != nil {
		return policy
	}
	data, err := json.Marshal(setting.Value)
	if err != nil {
		return policy
	}
	_ = json.Unmarshal(data, &policy)
	if policy.Windows < 1 {
		policy.Windows = 1
	}
	if policy.Windows > 9 {
		policy.Windows = 9
	}
	if policy.WindowSeconds < 5 {
		policy.WindowSeconds = 5
	}
	if policy.WindowSeconds > 60 {
		policy.WindowSeconds = 60
	}
	if len(policy.Positions) < policy.Windows {
		policy.Positions = defaultFrameStructureSamplingPolicy().Positions
		policy.Windows = min(policy.Windows, len(policy.Positions))
	}
	return policy
}

type QSVFeatureStatus struct {
	RequestedGopFrames int     `json:"requestedGopFrames,omitempty"`
	RequestedBFrames   *int    `json:"requestedBFrames,omitempty"`
	AdaptiveIRequested bool    `json:"adaptiveIRequested"`
	AdaptiveIEffective bool    `json:"adaptiveIEffective"`
	AdaptiveBRequested bool    `json:"adaptiveBRequested"`
	AdaptiveBEffective bool    `json:"adaptiveBEffective"`
	GPBKnown           bool    `json:"gpbKnown"`
	GPBEffective       bool    `json:"gpbEffective"`
	GopRefDist         int     `json:"gopRefDist,omitempty"`
	GopPicSize         int     `json:"gopPicSize,omitempty"`
	BRefType           string  `json:"bRefType,omitempty"`
	PRefType           string  `json:"pRefType,omitempty"`
	RateControlMethod  string  `json:"rateControlMethod,omitempty"`
	TargetUsage        int     `json:"targetUsage,omitempty"`
	MeasuredKnown      bool    `json:"measuredKnown"`
	ReorderFrames      int     `json:"reorderFrames,omitempty"`
	DPBFrames          int     `json:"dpbFrames,omitempty"`
	EstimatedDPBMiB    float64 `json:"estimatedDpbMiB,omitempty"`
	TemporalLayers     int     `json:"temporalLayers,omitempty"`
	InterpretationMode string  `json:"interpretationMode,omitempty"`
	ContextSource      string  `json:"contextSource,omitempty"`
}

type HEVCBitstreamStructure struct {
	Known          bool
	ReorderFrames  int
	DPBFrames      int
	TemporalLayers int
}

func parseHEVCTraceHeaders(output string) HEVCBitstreamStructure {
	result := HEVCBitstreamStructure{}
	for _, line := range strings.Split(output, "\n") {
		value, ok := traceHeaderInt(line)
		if !ok {
			continue
		}
		switch {
		case strings.Contains(line, "sps_max_num_reorder_pics"):
			result.Known = true
			result.ReorderFrames = max(result.ReorderFrames, value)
		case strings.Contains(line, "sps_max_dec_pic_buffering_minus1"):
			result.Known = true
			result.DPBFrames = max(result.DPBFrames, value+1)
		case strings.Contains(line, "sps_max_sub_layers_minus1"):
			result.Known = true
			result.TemporalLayers = max(result.TemporalLayers, value+1)
		}
	}
	return result
}

func traceHeaderInt(line string) (int, bool) {
	separator := strings.LastIndex(line, "=")
	if separator < 0 {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(line[separator+1:]))
	return value, err == nil
}

type FrameStructureRecommendation struct {
	TargetGOPFrames  int      `json:"targetGopFrames"`
	TargetGOPSeconds float64  `json:"targetGopSeconds"`
	MaxBFrames       int      `json:"maxBFrames"`
	AdaptiveI        bool     `json:"adaptiveI"`
	AdaptiveB        bool     `json:"adaptiveB"`
	SourceAverageGOP float64  `json:"sourceAverageGop"`
	SourceMaxBRun    int      `json:"sourceMaxBRun"`
	SourceBRatio     float64  `json:"sourceBRatio"`
	Confidence       string   `json:"confidence"`
	Reasons          []string `json:"reasons"`
	Warnings         []string `json:"warnings"`
}

type FrameStructureValidation struct {
	Verdict    string   `json:"verdict"`
	Confidence string   `json:"confidence"`
	Reasons    []string `json:"reasons"`
	Warnings   []string `json:"warnings"`
}

// FrameStructureRecommendationSet is derived exclusively from immutable
// source facts. It is encoder-neutral: capability-gated controls such as QSV
// Adaptive I/B or VideoToolbox frame reordering are resolved later.
type FrameStructureRecommendationSet struct {
	Version               int                                     `json:"version"`
	SourceAnalysisVersion int                                     `json:"sourceAnalysisVersion"`
	FPS                   float64                                 `json:"fps"`
	RecommendedMaxBFrames int                                     `json:"recommendedMaxBFrames"`
	Confidence            string                                  `json:"confidence"`
	ByMode                map[string]FrameStructureRecommendation `json:"byMode"`
	Warnings              []string                                `json:"warnings,omitempty"`
}

func buildFrameStructureRecommendationSet(scan models.ScanResult) FrameStructureRecommendationSet {
	return buildFrameStructureRecommendationSetForFPS(scan, scanFrameRate(scan))
}

func buildFrameStructureRecommendationSetForFPS(scan models.ScanResult, fps float64) FrameStructureRecommendationSet {
	analysis := scan.FrameStructureAnalysis
	source := QSVFrameStructureAnalysis{
		Version:               workerIntValue(analysis["version"], 0),
		FramesAnalyzed:        workerIntValue(analysis["framesAnalyzed"], 0),
		AverageGOPLength:      workerNumberValue(analysis["averageGopLength"], 0),
		MaxConsecutiveBFrames: workerIntValue(analysis["maxConsecutiveBFrames"], 0),
		BFrameRatio:           workerNumberValue(analysis["bFrameRatio"], 0),
		Confidence:            strings.TrimSpace(stringFromUnknown(analysis["confidence"])),
	}
	result := FrameStructureRecommendationSet{
		Version:               1,
		SourceAnalysisVersion: source.Version,
		FPS:                   fps,
		Confidence:            source.Confidence,
		ByMode:                map[string]FrameStructureRecommendation{},
	}
	if result.Confidence == "" {
		result.Confidence = "low"
	}
	for _, mode := range []string{"compatible", "balanced", "maximum_compression"} {
		recommendation := recommendFrameStructure(source, fps, "", mode, false, false, false)
		result.ByMode[mode] = recommendation
		if mode == "balanced" {
			result.RecommendedMaxBFrames = recommendation.MaxBFrames
		}
		result.Warnings = append(result.Warnings, recommendation.Warnings...)
	}
	if result.RecommendedMaxBFrames <= 0 {
		result.RecommendedMaxBFrames = 3
	}
	return result
}

func frameStructureRecommendationMap(scan models.ScanResult) models.JSONMap {
	encoded, _ := json.Marshal(buildFrameStructureRecommendationSet(scan))
	value := models.JSONMap{}
	_ = json.Unmarshal(encoded, &value)
	return value
}

func ensureFrameStructureRecommendation(scan *models.ScanResult) bool {
	if scan == nil || workerIntValue(scan.FrameStructureRecommendation["version"], 0) >= 1 {
		return false
	}
	scan.FrameStructureRecommendation = frameStructureRecommendationMap(*scan)
	return true
}

func storedFrameStructureRecommendation(
	scan models.ScanResult,
	mode string,
) FrameStructureRecommendation {
	mode = strings.ToLower(strings.TrimSpace(mode))

	if mode == "auto" || mode == "off" || mode == "" {
		mode = "balanced"
	}

	if ensureFrameStructureRecommendation(&scan) {
		// Derived in memory for legacy snapshots.
	}

	var recommendation FrameStructureRecommendation

	byMode, _ := scan.FrameStructureRecommendation["byMode"].(map[string]interface{})

	if byMode == nil {
		if raw, ok := scan.FrameStructureRecommendation["byMode"].(models.JSONMap); ok {
			byMode = map[string]interface{}(raw)
		}
	}

	if byMode != nil {
		raw, ok := byMode[mode]
		if !ok {
			raw = byMode["balanced"]
		}

		encoded, _ := json.Marshal(raw)
		_ = json.Unmarshal(encoded, &recommendation)
	}

	//
	// FrameStructureRecommendation is derived cache.
	// FrameStructureAnalysis + FPS are the source of truth.
	//
	// A stored version marker alone does not guarantee that the cached
	// recommendation is complete or usable.
	//
	if recommendation.TargetGOPFrames > 0 &&
		recommendation.MaxBFrames > 0 {
		return recommendation
	}

	fresh := buildFrameStructureRecommendationSet(scan)

	if rebuilt, ok := fresh.ByMode[mode]; ok {
		return rebuilt
	}

	return fresh.ByMode["balanced"]
}

func recommendFrameStructure(source QSVFrameStructureAnalysis, fps float64, contentType, policy string, advancedAllowed, adaptiveISupported, adaptiveBSupported bool) FrameStructureRecommendation {
	if fps <= 0 {
		return FrameStructureRecommendation{MaxBFrames: 3, Confidence: "low", Warnings: []string{"A reliable asset frame rate is required before MVForge can calculate an automatic GOP recommendation."}}
	}
	mode := strings.ToLower(strings.TrimSpace(policy))
	seconds := 0.0
	if source.AverageGOPLength > 0 {
		seconds = source.AverageGOPLength / fps
	}
	if seconds <= 0 {
		switch mode {
		case "compatible", "compatibility":
			seconds = 2.5
		case "maximum_compression":
			seconds = 5
		default:
			seconds = 3.5
		}
	} else {
		seconds = math.Max(2, math.Min(4, seconds))
		switch mode {
		case "compatible", "compatibility":
			seconds = math.Min(seconds, 3)
		case "maximum_compression":
			seconds = math.Min(seconds+2, 5.5)
		case "balanced":
			seconds = math.Min(seconds+0.75, 4)
		}
	}
	if strings.EqualFold(source.Confidence, "low") {
		seconds = math.Min(seconds, 3.5)
	}
	seconds = math.Max(2, math.Min(8, seconds))
	target := int(math.Round(fps * seconds))
	maxB := 3
	if source.MaxConsecutiveBFrames >= 1 && source.MaxConsecutiveBFrames <= 4 {
		maxB = source.MaxConsecutiveBFrames
	}
	result := FrameStructureRecommendation{TargetGOPFrames: target, TargetGOPSeconds: float64(target) / fps, MaxBFrames: maxB, AdaptiveI: advancedAllowed && adaptiveISupported, AdaptiveB: advancedAllowed && adaptiveBSupported, SourceAverageGOP: source.AverageGOPLength, SourceMaxBRun: source.MaxConsecutiveBFrames, SourceBRatio: source.BFrameRatio, Confidence: source.Confidence}
	if result.Confidence == "" {
		result.Confidence = "low"
	}
	sourceSeconds := 0.0
	if source.AverageGOPLength > 0 {
		sourceSeconds = source.AverageGOPLength / fps
	}
	result.Reasons = []string{fmt.Sprintf("Source GOP %.1f frames is %.2f seconds; %s targets %.2f seconds (%d frames at %.3f fps).", source.AverageGOPLength, sourceSeconds, mode, seconds, target, fps), fmt.Sprintf("Source longest B-run is %d; recommended maximum B depth is %d.", source.MaxConsecutiveBFrames, maxB)}
	if advancedAllowed && !adaptiveISupported {
		result.Warnings = append(result.Warnings, "Adaptive I is desirable but unavailable for the active worker combination.")
	}
	if advancedAllowed && !adaptiveBSupported {
		result.Warnings = append(result.Warnings, "Adaptive B is desirable but unavailable for the active worker combination.")
	}
	return result
}

func nearestFriendlyGOP(value int) int {
	candidates := []int{24, 30, 48, 50, 60, 72, 75, 90, 96, 100, 120, 150, 180, 240}
	best := candidates[0]
	for _, v := range candidates {
		if frameAbsInt(v-value) < frameAbsInt(best-value) {
			best = v
		}
	}
	return best
}
func frameAbsInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func validateFrameStructureRecommendation(rec FrameStructureRecommendation, source, output QSVFrameStructureAnalysis) FrameStructureValidation {
	result := FrameStructureValidation{Verdict: "safe", Confidence: output.Confidence, Reasons: []string{"Output frame structure is reasonably close to the recommendation."}}
	if result.Confidence == "" {
		result.Confidence = "low"
	}
	extreme := output.PFrames == 0 && output.BFrameRatio >= 0.95 && output.MaxConsecutiveBFrames > max(20, rec.MaxBFrames*5)
	if extreme || (rec.TargetGOPFrames > 0 && output.AverageGOPLength > float64(rec.TargetGOPFrames)*2 && output.WindowCount >= 2) {
		result.Verdict = "reject"
		result.Reasons = []string{"Encoder output differs substantially from the requested frame structure."}
		result.Warnings = append(result.Warnings, "Recommendation not validated.")
		return result
	}
	if (rec.TargetGOPFrames > 0 && output.AverageGOPLength > float64(rec.TargetGOPFrames)*1.35) || output.MaxConsecutiveBFrames > rec.MaxBFrames+1 || output.Variability == "high" || (source.MaxConsecutiveBFrames > 0 && output.MaxConsecutiveBFrames > source.MaxConsecutiveBFrames*2) {
		result.Verdict = "review"
		result.Reasons = []string{"Output has moderate GOP/B-run deviation or high variation between sampled regions."}
	}
	return result
}

func applyQSVFrameStructureAssessment(
	result *QSVFrameStructureAnalysis,
) {
	switch {
	case result.FramesAnalyzed == 0:
		result.Assessment = "No frames were available for analysis."

	case !result.HasBFrames:
		result.Assessment = "No B-frames were detected."

	case result.PFrames == 0 && result.BFrameRatio >= 0.9 && result.MaxConsecutiveBFrames >= 12:
		result.Assessment = "Unusual frame structure: the sample is B-frame-dominant, has no detected P-frames, and contains very long B-frame runs."

	case result.MaxConsecutiveBFrames >= 3:
		result.Assessment = "The stream uses B-frames with a balanced I/P/B structure."

	case result.MaxConsecutiveBFrames > 0:
		result.Assessment = "The stream uses a limited B-frame structure."

	default:
		result.Assessment = "Frame structure was detected, but B-frame behavior is inconclusive."
	}
}

func qsvFrameStructureWarnings(
	features QSVFeatureStatus,
	source QSVFrameStructureAnalysis,
	output QSVFrameStructureAnalysis,
) []string {
	warnings := []string{}

	if features.AdaptiveIRequested && !features.AdaptiveIEffective {
		warnings = append(warnings, "Adaptive I was requested but the active worker did not validate it for this QSV combination, so MVForge left it out of the command.")
	}
	if features.AdaptiveBRequested && !features.AdaptiveBEffective {
		warnings = append(warnings, "Adaptive B was requested but the active worker did not validate it for this QSV combination, so MVForge left it out of the command.")
	}
	if output.FramesAnalyzed == 0 {
		return append(warnings, "No output frames were available, so MVForge could not inspect the QSV frame structure.")
	}
	qsvGPB := (features.InterpretationMode == "qsv_gpb" || features.InterpretationMode == "qsv_mixed_b_gpb") && features.GPBKnown && features.GPBEffective && features.GopRefDist >= 1
	if !qsvGPB && output.BFrameRatio >= 0.9 {
		warnings = append(warnings, fmt.Sprintf("B-frames make up %.1f%% of the sample. This is unusually high and may indicate an unstable or unsuitable GOP structure.", output.BFrameRatio*100))
	}
	if !qsvGPB && output.PFrames == 0 && output.FramesAnalyzed >= 30 {
		warnings = append(warnings, "No P-frames were detected in the output sample. Review the GOP and B-frame behavior before using this profile for production.")
	}
	if !qsvGPB && output.MaxConsecutiveBFrames >= 12 {
		warnings = append(warnings, fmt.Sprintf("The output contains a run of %d consecutive B-frames. Long B-frame runs can reduce compatibility and make seeking less predictable.", output.MaxConsecutiveBFrames))
	}
	if !qsvGPB && source.FramesAnalyzed > 0 && source.PFrames > 0 && output.PFrames == 0 &&
		output.BFrameRatio >= 0.9 && output.BFrameRatio-source.BFrameRatio >= 0.25 &&
		output.MaxConsecutiveBFrames >= 12 && output.MaxConsecutiveBFrames >= source.MaxConsecutiveBFrames*4 {
		warnings = append(warnings, fmt.Sprintf(
			"The output frame structure changed substantially from the source: B-frame share %.1f%% to %.1f%%, longest B-frame run %d to %d, and detected P-frames %d to 0. Review compatibility and visual quality before adopting this configuration as a standard profile.",
			source.BFrameRatio*100,
			output.BFrameRatio*100,
			source.MaxConsecutiveBFrames,
			output.MaxConsecutiveBFrames,
			source.PFrames,
		))
	}

	if output.KeyFrames <= 1 {
		warnings = append(
			warnings,
			"Only one keyframe was found, so the average GOP length is not representative for this sample.",
		)
	} else if output.AverageGOPLength >= 240 {
		warnings = append(warnings, fmt.Sprintf("The average GOP is %.0f frames, which is long for many playback and seeking workflows.", output.AverageGOPLength))
	}

	return warnings
}

func analyzeVideoFrameStructure(
	ctx context.Context,
	path string,
	maxFrames int,
) (QSVFrameStructureAnalysis, error) {
	if maxFrames <= 0 {
		maxFrames = 500
	}

	result, err := analyzeVideoFrameStructureInterval(ctx, path, fmt.Sprintf("%%+#%d", maxFrames))
	result.SampleLimit = maxFrames
	result.ProcessCount = 1
	return result, err
}

func analyzeVideoFrameStructureInterval(ctx context.Context, path, interval string) (QSVFrameStructureAnalysis, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", interval,
		"-show_entries", "frame=pict_type,key_frame,best_effort_timestamp_time,pkt_duration_time,interlaced_frame,top_field_first,repeat_pict",
		"-of", "json",
		path,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}

		return QSVFrameStructureAnalysis{}, fmt.Errorf(
			"frame structure analysis failed: %s",
			message,
		)
	}

	var probe struct {
		Frames []frameStructureProbeFrame `json:"frames"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		return QSVFrameStructureAnalysis{}, fmt.Errorf(
			"decode frame structure analysis: %w",
			err,
		)
	}

	return analyzeFrameStructureProbeFrames(probe.Frames), nil
}

func analyzeFrameStructureProbeFrames(frames []frameStructureProbeFrame) QSVFrameStructureAnalysis {
	result := QSVFrameStructureAnalysis{Version: 2, Source: "ffprobe_frames"}
	result.FrameSignals = frameSignalsFromStructureProbe(frames)

	consecutiveBFrames := 0
	lastKeyFrameIndex := -1
	gopLengths := []int{}

	for frameIndex, frame := range frames {
		result.FramesAnalyzed++

		switch strings.ToUpper(strings.TrimSpace(frame.PictureType)) {
		case "I":
			result.IFrames++
			consecutiveBFrames = 0
		case "P":
			result.PFrames++
			consecutiveBFrames = 0
		case "B":
			result.BFrames++
			consecutiveBFrames++
			if consecutiveBFrames > result.MaxConsecutiveBFrames {
				result.MaxConsecutiveBFrames = consecutiveBFrames
			}
		}

		if frame.KeyFrame == 1 {
			result.KeyFrames++

			if lastKeyFrameIndex >= 0 {
				gopLengths = append(gopLengths, frameIndex-lastKeyFrameIndex)
			}

			lastKeyFrameIndex = frameIndex
		}
	}

	if result.FramesAnalyzed > 0 {
		result.BFrameRatio =
			float64(result.BFrames) / float64(result.FramesAnalyzed)
	}

	if len(gopLengths) > 0 {
		result.CompleteGOPs = len(gopLengths)
		result.MinimumGOPLength = gopLengths[0]
		totalGOPLength := 0

		for _, length := range gopLengths {
			totalGOPLength += length
			if length < result.MinimumGOPLength {
				result.MinimumGOPLength = length
			}
			if length > result.MaximumGOPLength {
				result.MaximumGOPLength = length
			}
		}

		result.AverageGOPLength =
			float64(totalGOPLength) / float64(len(gopLengths))
	}

	result.HasBFrames = result.BFrames > 0

	applyQSVFrameStructureAssessment(&result)

	return result
}

func frameSignalsFromStructureProbe(frames []frameStructureProbeFrame) FrameSignalSummary {
	result := FrameSignalSummary{DecodedFrames: len(frames)}
	firstTimestamp, lastTimestamp, previousTimestamp := -1.0, -1.0, -1.0
	deltas := []float64{}
	repeatPositions := []int{}
	for index, frame := range frames {
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
	return result
}

func analyzeVideoFrameStructureIntervals(ctx context.Context, path string, windows []SamplingWindow) ([]QSVFrameStructureAnalysis, error) {
	if len(windows) == 0 {
		return nil, fmt.Errorf("frame structure analysis requires at least one sampling window")
	}
	intervals := make([]string, 0, len(windows))
	for _, window := range windows {
		intervals = append(intervals, fmt.Sprintf("%.3f%%+%.3f", window.StartSeconds, window.DurationSeconds))
	}
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", strings.Join(intervals, ","),
		"-show_entries", "frame=pict_type,key_frame,best_effort_timestamp_time,pkt_duration_time,interlaced_frame,top_field_first,repeat_pict",
		"-of", "json",
		path,
	}
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("multi-interval frame structure analysis failed: %s", message)
	}
	var probe struct {
		Frames []frameStructureProbeFrame `json:"frames"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		return nil, fmt.Errorf("decode multi-interval frame structure analysis: %w", err)
	}
	buckets := make([][]frameStructureProbeFrame, len(windows))
	for _, frame := range probe.Frames {
		timestamp, timestampErr := strconv.ParseFloat(strings.TrimSpace(frame.BestEffortTimestamp), 64)
		if timestampErr != nil || timestamp < 0 {
			return nil, fmt.Errorf("multi-interval frame structure output omitted an absolute frame timestamp")
		}
		selected := -1
		selectedDistance := math.MaxFloat64
		for index, window := range windows {
			end := window.StartSeconds + window.DurationSeconds
			if timestamp < window.StartSeconds-0.5 || timestamp > end+0.5 {
				continue
			}
			distance := math.Abs(timestamp - (window.StartSeconds + window.DurationSeconds/2))
			if distance < selectedDistance {
				selected, selectedDistance = index, distance
			}
		}
		if selected >= 0 {
			buckets[selected] = append(buckets[selected], frame)
		}
	}
	analyses := make([]QSVFrameStructureAnalysis, len(windows))
	for index, frames := range buckets {
		if len(frames) == 0 {
			return nil, fmt.Errorf("multi-interval frame structure output did not contain frames for window %d", index+1)
		}
		analyses[index] = analyzeFrameStructureProbeFrames(frames)
	}
	return analyses, nil
}

func analyzeVideoFrameStructureDistributed(ctx context.Context, path string, duration float64, policy FrameStructureSamplingPolicy) (QSVFrameStructureAnalysis, error) {
	if duration <= 0 {
		result, err := analyzeVideoFrameStructure(ctx, path, 500)
		if err == nil {
			result.Confidence = "low"
			result.Variability = "unknown"
			result.Source = "ffprobe_frames_duration_unknown"
		}
		return result, err
	}
	plan := canonicalSamplingPlan(duration, policy)
	return analyzeVideoFrameStructureWithSamplingPlan(ctx, path, plan)
}

func analyzeSamplingWindowsWithFallback(ctx context.Context, path string, windows []SamplingWindow) ([]QSVFrameStructureAnalysis, int, bool, error) {
	processCount := 0
	if len(windows) > 1 {
		processCount++
		if analyses, err := analyzeVideoFrameStructureIntervals(ctx, path, windows); err == nil {
			return analyses, processCount, true, nil
		}
	}
	analyses := make([]QSVFrameStructureAnalysis, 0, len(windows))
	for _, window := range windows {
		analysis, err := analyzeVideoFrameStructureInterval(ctx, path, fmt.Sprintf("%.3f%%+%.3f", window.StartSeconds, window.DurationSeconds))
		processCount++
		if err != nil {
			return nil, processCount, false, err
		}
		analyses = append(analyses, analysis)
	}
	return analyses, processCount, false, nil
}

func remainingSamplingWindows(all, selected []SamplingWindow) []SamplingWindow {
	selectedPositions := map[int]bool{}
	for _, window := range selected {
		selectedPositions[samplingPositionKey(window.Position)] = true
	}
	remaining := make([]SamplingWindow, 0, len(all)-len(selected))
	for _, window := range all {
		if !selectedPositions[samplingPositionKey(window.Position)] {
			remaining = append(remaining, window)
		}
	}
	return remaining
}

func frameEvidenceRequiresAdditionalWindows(analyses []QSVFrameStructureAnalysis) bool {
	if len(analyses) < 3 {
		return true
	}
	minimumFPS, maximumFPS := 0.0, 0.0
	completeGOPs := 0
	gopAverages := []float64{}
	for _, analysis := range analyses {
		signals := analysis.FrameSignals
		if signals.DecodedFrames == 0 || signals.EffectiveFPS <= 0 || signals.InterlacedFrames > 0 || signals.RepeatPictFrames > 0 {
			return true
		}
		if minimumFPS == 0 || signals.EffectiveFPS < minimumFPS {
			minimumFPS = signals.EffectiveFPS
		}
		if signals.EffectiveFPS > maximumFPS {
			maximumFPS = signals.EffectiveFPS
		}
		completeGOPs += analysis.CompleteGOPs
		if analysis.CompleteGOPs > 0 {
			gopAverages = append(gopAverages, analysis.AverageGOPLength)
		}
	}
	return maximumFPS-minimumFPS > 0.50 || completeGOPs < 2 || frameStructureVariability(gopAverages) == "high"
}

// frameEvidenceConfidenceScore intentionally uses a small set of observable,
// deterministic penalties. The score summarizes agreement; suspicious signals
// remain independent hard blockers for adaptive early stopping.
func frameEvidenceConfidenceScore(analyses []QSVFrameStructureAnalysis) float64 {
	if len(analyses) == 0 {
		return 0
	}
	score := 1.0
	minimumFPS, maximumFPS := 0.0, 0.0
	completeGOPs := 0
	gopAverages := []float64{}
	for _, analysis := range analyses {
		signals := analysis.FrameSignals
		if signals.DecodedFrames == 0 || signals.EffectiveFPS <= 0 {
			score -= 0.20
		}
		if signals.InterlacedFrames > 0 {
			score -= 0.30
		}
		if signals.RepeatPictFrames > 0 {
			score -= 0.30
		}
		if signals.EffectiveFPS > 0 {
			if minimumFPS == 0 || signals.EffectiveFPS < minimumFPS {
				minimumFPS = signals.EffectiveFPS
			}
			if signals.EffectiveFPS > maximumFPS {
				maximumFPS = signals.EffectiveFPS
			}
		}
		completeGOPs += analysis.CompleteGOPs
		if analysis.CompleteGOPs > 0 {
			gopAverages = append(gopAverages, analysis.AverageGOPLength)
		}
	}
	if maximumFPS-minimumFPS > 0.50 {
		score -= 0.30
	}
	if completeGOPs < 2 {
		score -= 0.20
	}
	switch frameStructureVariability(gopAverages) {
	case "high":
		score -= 0.25
	case "medium":
		score -= 0.10
	}
	return math.Round(max(0.0, min(1.0, score))*1000) / 1000
}

func analyzeVideoFrameStructureWithSamplingPlan(ctx context.Context, path string, plan SamplingPlan) (QSVFrameStructureAnalysis, error) {
	if plan.AssetDurationSeconds <= 0 {
		result, err := analyzeVideoFrameStructure(ctx, path, 500)
		if err == nil {
			result.Confidence = "low"
			result.ConfidenceScore = 0.25
			result.Variability = "unknown"
			result.Source = "ffprobe_frames_duration_unknown"
		}
		return result, err
	}
	result := QSVFrameStructureAnalysis{Version: 2, Source: "ffprobe_distributed_windows", AssetDurationSeconds: plan.AssetDurationSeconds, WindowLengthSeconds: plan.WindowSeconds, Positions: append([]float64(nil), plan.Positions...), WindowsRequested: len(plan.Positions)}
	gopWeightedTotal := 0.0
	windowGOPs := []float64{}
	intervals := [][2]float64{}
	windows := plan.windows(plan.WindowSeconds)
	selectedWindows := windows
	if plan.Adaptive && plan.InitialWindows > 0 && len(windows) > plan.InitialWindows {
		selectedWindows = representativeSamplingWindows(SamplingPlan{AssetDurationSeconds: plan.AssetDurationSeconds, WindowSeconds: plan.WindowSeconds, Positions: plan.Positions}, plan.WindowSeconds, plan.InitialWindows)
	}
	analyses, processCount, multiInterval, err := analyzeSamplingWindowsWithFallback(ctx, path, selectedWindows)
	if err != nil {
		return QSVFrameStructureAnalysis{}, err
	}
	result.ProcessCount += processCount
	if multiInterval {
		result.Source = "ffprobe_multi_interval_windows"
	}
	if len(selectedWindows) < len(windows) {
		initialConfidence := frameEvidenceConfidenceScore(analyses)
		if frameEvidenceRequiresAdditionalWindows(analyses) || initialConfidence < 0.98 {
			remaining := remainingSamplingWindows(windows, selectedWindows)
			additional, additionalProcesses, additionalMulti, additionalErr := analyzeSamplingWindowsWithFallback(ctx, path, remaining)
			if additionalErr != nil {
				return QSVFrameStructureAnalysis{}, additionalErr
			}
			selectedWindows = append(selectedWindows, remaining...)
			analyses = append(analyses, additional...)
			result.ProcessCount += additionalProcesses
			result.DeepAnalysisTriggered = true
			if additionalMulti {
				result.Source = "ffprobe_adaptive_multi_interval_windows"
			}
		} else {
			result.EarlyStopped = true
			result.Positions = make([]float64, 0, len(selectedWindows))
			for _, window := range selectedWindows {
				result.Positions = append(result.Positions, window.Position)
			}
		}
	}
	analysisByPosition := map[int]QSVFrameStructureAnalysis{}
	for index, window := range selectedWindows {
		analysisByPosition[samplingPositionKey(window.Position)] = analyses[index]
	}
	sort.SliceStable(selectedWindows, func(i, j int) bool { return selectedWindows[i].Position < selectedWindows[j].Position })
	analyses = analyses[:0]
	for _, window := range selectedWindows {
		analyses = append(analyses, analysisByPosition[samplingPositionKey(window.Position)])
	}
	for index, samplingWindow := range selectedWindows {
		position := samplingWindow.Position
		start := samplingWindow.StartSeconds
		actualDuration := samplingWindow.DurationSeconds
		analysis := analyses[index]
		analysis.SampledSeconds = actualDuration
		frameSignals := analysis.FrameSignals
		analysis.FrameSignals = FrameSignalSummary{}
		result.Windows = append(result.Windows, FrameStructureWindow{Position: position, StartSeconds: start, DurationSeconds: actualDuration, Analysis: analysis, FrameSignals: frameSignals})
		result.FramesAnalyzed += analysis.FramesAnalyzed
		result.IFrames += analysis.IFrames
		result.PFrames += analysis.PFrames
		result.BFrames += analysis.BFrames
		result.KeyFrames += analysis.KeyFrames
		result.CompleteGOPs += analysis.CompleteGOPs
		if analysis.MaxConsecutiveBFrames > result.MaxConsecutiveBFrames {
			result.MaxConsecutiveBFrames = analysis.MaxConsecutiveBFrames
		}
		if analysis.CompleteGOPs > 0 {
			gopWeightedTotal += analysis.AverageGOPLength * float64(analysis.CompleteGOPs)
			windowGOPs = append(windowGOPs, analysis.AverageGOPLength)
			if result.MinimumGOPLength == 0 || analysis.MinimumGOPLength < result.MinimumGOPLength {
				result.MinimumGOPLength = analysis.MinimumGOPLength
			}
			if analysis.MaximumGOPLength > result.MaximumGOPLength {
				result.MaximumGOPLength = analysis.MaximumGOPLength
			}
		}
		intervals = append(intervals, [2]float64{start, start + actualDuration})
	}
	if result.FramesAnalyzed > 0 {
		result.BFrameRatio = float64(result.BFrames) / float64(result.FramesAnalyzed)
	}
	if result.CompleteGOPs > 0 {
		result.AverageGOPLength = gopWeightedTotal / float64(result.CompleteGOPs)
	}
	result.HasBFrames = result.BFrames > 0
	result.WindowCount = len(result.Windows)
	result.SampledSeconds = mergedIntervalSeconds(intervals)
	if plan.AssetDurationSeconds > 0 {
		result.CoverageRatio = result.SampledSeconds / plan.AssetDurationSeconds
	}
	result.Variability = frameStructureVariability(windowGOPs)
	result.ConfidenceScore = frameEvidenceConfidenceScore(analyses)
	switch {
	case result.WindowCount >= 4 && result.CompleteGOPs >= 5 && result.Variability == "low":
		result.Confidence = "high"
	case result.WindowCount >= 3 && result.CompleteGOPs >= 2:
		result.Confidence = "medium"
	default:
		result.Confidence = "low"
	}
	applyQSVFrameStructureAssessment(&result)
	return result, nil
}

func mergedIntervalSeconds(intervals [][2]float64) float64 {
	sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
	total, end := 0.0, -1.0
	for _, v := range intervals {
		if v[0] > end {
			total += v[1] - v[0]
			end = v[1]
		} else if v[1] > end {
			total += v[1] - end
			end = v[1]
		}
	}
	return total
}
func frameStructureVariability(values []float64) string {
	if len(values) < 2 {
		return "unknown"
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	cv := math.Sqrt(variance/float64(len(values))) / math.Max(mean, 1)
	if cv > 0.35 {
		return "high"
	}
	if cv > 0.15 {
		return "medium"
	}
	return "low"
}
