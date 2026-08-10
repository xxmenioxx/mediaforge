package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type QSVFrameStructureAnalysis struct {
	Version               int     `json:"version"`
	SampleLimit           int     `json:"sampleLimit"`
	FramesAnalyzed        int     `json:"framesAnalyzed"`
	IFrames               int     `json:"iFrames"`
	PFrames               int     `json:"pFrames"`
	BFrames               int     `json:"bFrames"`
	KeyFrames             int     `json:"keyFrames"`
	BFrameRatio           float64 `json:"bFrameRatio"`
	HasBFrames            bool    `json:"hasBFrames"`
	MaxConsecutiveBFrames int     `json:"maxConsecutiveBFrames"`
	AverageGOPLength      float64 `json:"averageGopLength"`
	Assessment            string  `json:"assessment"`
	Source                string  `json:"source"`
}

type QSVFeatureStatus struct {
	AdaptiveIRequested bool `json:"adaptiveIRequested"`
	AdaptiveIEffective bool `json:"adaptiveIEffective"`
	AdaptiveBRequested bool `json:"adaptiveBRequested"`
	AdaptiveBEffective bool `json:"adaptiveBEffective"`
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

	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-read_intervals", fmt.Sprintf("%%+#%d", maxFrames),
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

	result := QSVFrameStructureAnalysis{Version: 1, SampleLimit: maxFrames, Source: "ffprobe_frames"}

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
		totalGOPLength := 0

		for _, length := range gopLengths {
			totalGOPLength += length
		}

		result.AverageGOPLength =
			float64(totalGOPLength) / float64(len(gopLengths))
	}

	result.HasBFrames = result.BFrames > 0

	applyQSVFrameStructureAssessment(&result)

	return result, nil
}
