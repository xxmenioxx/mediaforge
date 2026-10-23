package handlers

import (
	"errors"
	"math"
)

const (
	subtitleSyncModelFixedOffset = "fixed_offset"
	subtitleSyncModelAffine      = "affine"
	subtitleSyncModelSegmented   = "segmented"

	maxSubtitleSyncSegments        = 128
	maxSubtitleSyncModelDurationMs = int64(7 * 24 * 60 * 60 * 1000)
	maxSubtitleSyncModelScale      = 4.0
	minSubtitleSyncModelScale      = 0.25
	maxSubtitleSyncDiagnosticCount = 32
	maxSubtitleSyncDiagnosticBytes = 512
)

// validateSubtitleSyncProposalShape enforces the engine-neutral transform
// contract. It validates representation only; engine-specific Apply methods
// still decide which models they can execute.
func validateSubtitleSyncProposalShape(proposal SubtitleSyncProposal) error {
	if proposal.Engine == "" || proposal.EngineVersion == "" {
		return errors.New("subtitle sync engine identity is required")
	}
	if proposal.OffsetMilliseconds < -maxSubtitleSyncOffsetMs || proposal.OffsetMilliseconds > maxSubtitleSyncOffsetMs {
		return errors.New("subtitle sync offset exceeds the model bound")
	}
	if !validSubtitleSyncScale(proposal.Scale) {
		return errors.New("subtitle sync scale is outside the supported model bound")
	}
	if math.IsNaN(proposal.Confidence) || math.IsInf(proposal.Confidence, 0) || proposal.Confidence < 0 || proposal.Confidence > 1 {
		return errors.New("subtitle sync confidence must be between zero and one")
	}
	if len(proposal.Segments) > maxSubtitleSyncSegments {
		return errors.New("subtitle sync proposal exceeds the segment-count bound")
	}
	if len(proposal.Preview) > maxSubtitleSyncPreview {
		return errors.New("subtitle sync proposal exceeds the preview bound")
	}
	if len(proposal.Diagnostics) > maxSubtitleSyncDiagnosticCount {
		return errors.New("subtitle sync proposal exceeds the diagnostic-count bound")
	}
	for _, diagnostic := range proposal.Diagnostics {
		if len(diagnostic) > maxSubtitleSyncDiagnosticBytes {
			return errors.New("subtitle sync proposal diagnostic exceeds the size bound")
		}
	}

	switch proposal.Model {
	case subtitleSyncModelFixedOffset:
		if proposal.Scale != 1 || len(proposal.Segments) != 0 {
			return errors.New("fixed-offset proposals require scale one and no segments")
		}
	case subtitleSyncModelAffine:
		if len(proposal.Segments) != 0 {
			return errors.New("affine proposals cannot contain segments")
		}
	case subtitleSyncModelSegmented:
		if err := validateSubtitleSyncSegmentShapes(proposal.Segments); err != nil {
			return err
		}
	default:
		return errors.New("unsupported subtitle sync model")
	}
	return nil
}

func validateSubtitleSyncSegmentShapes(segments []SubtitleSyncSegment) error {
	if len(segments) == 0 {
		return errors.New("segmented proposals require at least one segment")
	}
	var previousEnd int64
	previousOpenEnded := false
	for index, segment := range segments {
		if !validSubtitleSyncScale(segment.Scale) {
			return errors.New("subtitle sync segment scale is outside the model bound")
		}
		if segment.OffsetMilliseconds < -maxSubtitleSyncOffsetMs || segment.OffsetMilliseconds > maxSubtitleSyncOffsetMs {
			return errors.New("subtitle sync segment offset exceeds the model bound")
		}
		if previousOpenEnded {
			return errors.New("subtitle sync segments cannot follow an open-ended segment")
		}
		start := int64(0)
		if segment.StartMilliseconds != nil {
			start = *segment.StartMilliseconds
			if start < 0 || start > maxSubtitleSyncModelDurationMs {
				return errors.New("subtitle sync segment start is outside the duration bound")
			}
		} else if index != 0 {
			return errors.New("only the first subtitle sync segment may omit its start")
		}
		var end int64
		if segment.EndMilliseconds != nil {
			end = *segment.EndMilliseconds
			if end < 0 || end > maxSubtitleSyncModelDurationMs || end <= start {
				return errors.New("subtitle sync segment end is invalid or outside the duration bound")
			}
		} else if index != len(segments)-1 {
			return errors.New("only the final subtitle sync segment may omit its end")
		} else {
			end = maxSubtitleSyncModelDurationMs
			if end <= start {
				return errors.New("subtitle sync segment end is invalid or outside the duration bound")
			}
		}
		if index > 0 && start < previousEnd {
			return errors.New("subtitle sync segments must be ordered and non-overlapping")
		}
		if segment.EndMilliseconds == nil {
			previousOpenEnded = true
		} else {
			previousEnd = end
		}
	}
	return nil
}

func validSubtitleSyncScale(scale float64) bool {
	return !math.IsNaN(scale) && !math.IsInf(scale, 0) && scale >= minSubtitleSyncModelScale && scale <= maxSubtitleSyncModelScale
}
