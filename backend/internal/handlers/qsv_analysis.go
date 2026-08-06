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

func applyQSVFrameStructureAssessment(
	result *QSVFrameStructureAnalysis,
) {
	switch {
	case result.FramesAnalyzed == 0:
		result.Assessment = "No frames were available for analysis."

	case !result.HasBFrames:
		result.Assessment = "No B-frames were detected."

	case result.MaxConsecutiveBFrames >= 3:
		result.Assessment = "The stream uses a strong B-frame structure."

	case result.MaxConsecutiveBFrames > 0:
		result.Assessment = "The stream uses a limited B-frame structure."

	default:
		result.Assessment = "Frame structure was detected, but B-frame behavior is inconclusive."
	}
}

func qsvFrameStructureWarnings(
	adaptiveBRequested bool,
	output QSVFrameStructureAnalysis,
) []string {
	warnings := []string{}

	if adaptiveBRequested {
		switch {
		case !output.HasBFrames:
			warnings = append(
				warnings,
				"Adaptive B was requested, but no B-frames were detected in the encoded preview.",
			)

		case output.MaxConsecutiveBFrames <= 1:
			warnings = append(
				warnings,
				"Adaptive B produced only a limited B-frame structure in this preview sample.",
			)
		}
	}

	if output.HasBFrames && output.MaxConsecutiveBFrames >= 3 {
		warnings = append(
			warnings,
			"The encoded preview produced a strong B-frame structure.",
		)
	}

	if output.KeyFrames <= 1 {
		warnings = append(
			warnings,
			"The preview sample is too short to calculate a representative GOP average.",
		)
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

	result := QSVFrameStructureAnalysis{
		Source: "ffprobe_frames",
	}

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
