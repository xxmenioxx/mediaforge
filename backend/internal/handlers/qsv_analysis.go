package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"sort"
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
	Assessment            string                 `json:"assessment"`
	Source                string                 `json:"source"`
}

type FrameStructureWindow struct {
	Position        float64                   `json:"position"`
	StartSeconds    float64                   `json:"startSeconds"`
	DurationSeconds float64                   `json:"durationSeconds"`
	Analysis        QSVFrameStructureAnalysis `json:"analysis"`
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
	AdaptiveIRequested bool `json:"adaptiveIRequested"`
	AdaptiveIEffective bool `json:"adaptiveIEffective"`
	AdaptiveBRequested bool `json:"adaptiveBRequested"`
	AdaptiveBEffective bool `json:"adaptiveBEffective"`
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

func recommendFrameStructure(source QSVFrameStructureAnalysis, fps float64, contentType, policy string, advancedAllowed, adaptiveISupported, adaptiveBSupported bool) FrameStructureRecommendation {
	if fps <= 0 {
		fps = 30
	}
	seconds := 3.0
	if source.AverageGOPLength > 0 {
		seconds = source.AverageGOPLength / fps
	}
	seconds = math.Max(2, math.Min(4, seconds))
	if strings.Contains(strings.ToLower(contentType), "anime") || strings.Contains(strings.ToLower(contentType), "animation") {
		seconds += 0.25
	}
	if strings.EqualFold(policy, "compatibility") {
		seconds = math.Min(seconds, 2)
	}
	seconds = math.Max(2, math.Min(4, seconds))
	target := nearestFriendlyGOP(int(math.Round(fps * seconds)))
	maxB := 3
	if source.MaxConsecutiveBFrames >= 1 && source.MaxConsecutiveBFrames <= 4 {
		maxB = source.MaxConsecutiveBFrames
	}
	result := FrameStructureRecommendation{TargetGOPFrames: target, TargetGOPSeconds: float64(target) / fps, MaxBFrames: maxB, AdaptiveI: advancedAllowed && adaptiveISupported, AdaptiveB: advancedAllowed && adaptiveBSupported, SourceAverageGOP: source.AverageGOPLength, SourceMaxBRun: source.MaxConsecutiveBFrames, SourceBRatio: source.BFrameRatio, Confidence: source.Confidence}
	if result.Confidence == "" {
		result.Confidence = "low"
	}
	result.Reasons = []string{fmt.Sprintf("Source GOP %.1f frames was normalized to an encoder-friendly target of %d frames.", source.AverageGOPLength, target), fmt.Sprintf("Source longest B-run is %d; recommended maximum B depth is %d.", source.MaxConsecutiveBFrames, maxB)}
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
	if output.BFrameRatio >= 0.9 {
		warnings = append(warnings, fmt.Sprintf("B-frames make up %.1f%% of the sample. This is unusually high and may indicate an unstable or unsuitable GOP structure.", output.BFrameRatio*100))
	}
	if output.PFrames == 0 && output.FramesAnalyzed >= 30 {
		warnings = append(warnings, "No P-frames were detected in the output sample. Review the GOP and B-frame behavior before using this profile for production.")
	}
	if output.MaxConsecutiveBFrames >= 12 {
		warnings = append(warnings, fmt.Sprintf("The output contains a run of %d consecutive B-frames. Long B-frame runs can reduce compatibility and make seeking less predictable.", output.MaxConsecutiveBFrames))
	}
	if source.FramesAnalyzed > 0 && source.PFrames > 0 && output.PFrames == 0 &&
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
	return result, err
}

func analyzeVideoFrameStructureInterval(ctx context.Context, path, interval string) (QSVFrameStructureAnalysis, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", interval,
		"-show_entries", "frame=pict_type,key_frame",
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
		Frames []struct {
			PictureType string `json:"pict_type"`
			KeyFrame    int    `json:"key_frame"`
		} `json:"frames"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		return QSVFrameStructureAnalysis{}, fmt.Errorf(
			"decode frame structure analysis: %w",
			err,
		)
	}

	result := QSVFrameStructureAnalysis{Version: 2, Source: "ffprobe_frames"}

	consecutiveBFrames := 0
	lastKeyFrameIndex := -1
	gopLengths := []int{}

	for frameIndex, frame := range probe.Frames {
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

	return result, nil
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
	if policy.Windows <= 0 {
		policy = defaultFrameStructureSamplingPolicy()
	}
	windowSeconds := policy.WindowSeconds
	if policy.Adaptive {
		switch {
		case duration <= 60:
			windowSeconds = duration
			policy.Windows = 1
			policy.Positions = []float64{0.5}
		case duration < 120:
			windowSeconds = 10
		case duration < 600:
			windowSeconds = 15
		default:
			windowSeconds = 20
		}
	}
	if windowSeconds <= 0 {
		windowSeconds = 20
	}
	positions := policy.Positions
	if len(positions) < policy.Windows {
		positions = defaultFrameStructureSamplingPolicy().Positions
	}
	result := QSVFrameStructureAnalysis{Version: 2, Source: "ffprobe_distributed_windows", AssetDurationSeconds: duration, WindowLengthSeconds: windowSeconds, Positions: append([]float64(nil), positions[:policy.Windows]...)}
	gopWeightedTotal := 0.0
	windowGOPs := []float64{}
	intervals := [][2]float64{}
	for index := 0; index < policy.Windows; index++ {
		position := positions[index]
		start := math.Max(0, math.Min(duration-windowSeconds, duration*position-windowSeconds/2))
		actualDuration := math.Min(windowSeconds, duration-start)
		analysis, err := analyzeVideoFrameStructureInterval(ctx, path, fmt.Sprintf("%.3f%%+%.3f", start, actualDuration))
		if err != nil {
			return QSVFrameStructureAnalysis{}, err
		}
		analysis.SampledSeconds = actualDuration
		result.Windows = append(result.Windows, FrameStructureWindow{Position: position, StartSeconds: start, DurationSeconds: actualDuration, Analysis: analysis})
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
	if duration > 0 {
		result.CoverageRatio = result.SampledSeconds / duration
	}
	result.Variability = frameStructureVariability(windowGOPs)
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
