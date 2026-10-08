package handlers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// bitmapTranslationSource is an operation-local view of canonical bitmap OCR
// evidence. OCR identity, execution, and quality policy remain owned by the
// canonical OCR domains; translation only receives the final cue view.
type bitmapTranslationSource struct {
	EffectiveLanguage string
	Evidence          *canonicalOCREvidence
	Envelopes         []translationCueEnvelope
}

func bitmapTranslationSourceFor(
	ctx context.Context,
	mediaPath string,
	stream FFProbeStream,
	requestedLanguage string,
	ocrMode string,
	segmentStart float64,
	segmentDuration int,
	sourceIdentity string,
	sourceRepresentation string,
	logicalTransformKey string,
	onProgress OCRProgressFunc,
) (*bitmapTranslationSource, error) {
	if ctx == nil {
		return nil, fmt.Errorf("bitmap translation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	codec := strings.ToLower(strings.TrimSpace(stream.CodecName))
	switch codec {
	case "hdmv_pgs_subtitle", "pgssub", "dvd_subtitle":
	default:
		return nil, fmt.Errorf("bitmap translation does not support subtitle codec %q", stream.CodecName)
	}
	request := canonicalOCRRequestFor(mediaPath, stream, requestedLanguage, ocrMode, segmentStart, segmentDuration)
	evidence, foundPublished, selectionErr := publishedCanonicalOCREvidenceForTranslation(request)
	if selectionErr != nil {
		return nil, subtitleOCRReviewRequiredError(fmt.Errorf("published canonical OCR evidence is ambiguous"))
	}
	if foundPublished {
		if onProgress != nil {
			onProgress(1, 1)
		}
	} else {
		var err error
		evidence, err = canonicalOCREvidenceForRequest(ctx, request, onProgress)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR failed: %w", err))
		}
	}
	if evidence == nil || !evidence.Complete || len(evidence.Cues) == 0 || strings.TrimSpace(evidence.EffectiveLanguage) == "" {
		return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR returned incomplete evidence"))
	}
	finalEvidence, err := correctedCanonicalOCREvidenceForTranslation(evidence)
	if err != nil {
		return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR correction failed: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceIdentity) == "" {
		sourceIdentity = finalEvidence.Identity.key()
	}
	if strings.TrimSpace(sourceRepresentation) == "" {
		sourceRepresentation = codec
	}
	envelopes := translationCueEnvelopesForBitmapCues(sourceIdentity, sourceRepresentation, logicalTransformKey, finalEvidence.Cues)
	if len(envelopes) != len(finalEvidence.Cues) || len(envelopes) == 0 {
		return nil, subtitleOCRFailedError(fmt.Errorf("canonical bitmap OCR produced no usable translation envelopes"))
	}
	return &bitmapTranslationSource{
		EffectiveLanguage: finalEvidence.EffectiveLanguage,
		Evidence:          finalEvidence,
		Envelopes:         envelopes,
	}, nil
}

func publishedCanonicalOCREvidenceForTranslation(request canonicalOCRRequest) (*canonicalOCREvidence, bool, error) {
	artifacts := findPublishedCanonicalOCRArtifactsForEmbeddedSource(request.MediaPath, request.Stream)
	compatible := make([]canonicalOCRArtifactDescriptor, 0, len(artifacts))
	for _, artifact := range artifacts {
		snapshot := artifact.Snapshot
		if snapshot == nil {
			continue
		}
		evidence := &snapshot.Evidence
		if !canonicalOCREvidenceMatchesTranslationRequest(evidence, request) || !evidence.Complete || len(evidence.Cues) == 0 ||
			strings.TrimSpace(evidence.EffectiveLanguage) == "" {
			continue
		}
		compatible = append(compatible, artifact)
	}
	if len(compatible) == 0 {
		return nil, false, nil
	}

	// A published manual or deterministic correction is the most recent
	// canonical decision for translation. Older uncorrected SRT/ASS snapshots
	// must not win merely because their format sorts first.
	corrected := make([]canonicalOCRArtifactDescriptor, 0, len(compatible))
	for _, artifact := range compatible {
		if len(artifact.Snapshot.Evidence.CorrectionProvenance) > 0 {
			corrected = append(corrected, artifact)
		}
	}
	if len(corrected) > 0 {
		compatible = corrected
	}
	for i := 1; i < len(compatible); i++ {
		if !canonicalOCRTranslationEvidenceEquivalent(&compatible[0].Snapshot.Evidence, &compatible[i].Snapshot.Evidence) {
			return nil, false, fmt.Errorf("multiple compatible canonical OCR corrections disagree")
		}
	}
	sort.SliceStable(compatible, func(i, j int) bool {
		left, right := compatible[i], compatible[j]
		leftRank, rightRank := 1, 1
		if left.Format == "srt" {
			leftRank = 0
		}
		if right.Format == "srt" {
			rightRank = 0
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.LogicalTransform < right.LogicalTransform
	})
	return cloneCanonicalOCREvidence(&compatible[0].Snapshot.Evidence), true, nil
}

func canonicalOCREvidenceMatchesTranslationRequest(evidence *canonicalOCREvidence, request canonicalOCRRequest) bool {
	if evidence == nil {
		return false
	}
	identity := evidence.Identity
	requested := request.Identity
	return identity.Version == requested.Version &&
		identity.SourcePath == requested.SourcePath &&
		identity.SourceSize == requested.SourceSize &&
		identity.SourceModTimeUnixNano == requested.SourceModTimeUnixNano &&
		identity.SourceSnapshotVersion == requested.SourceSnapshotVersion &&
		identity.StreamIndex == requested.StreamIndex &&
		identity.StreamID == requested.StreamID &&
		identity.StreamCodec == requested.StreamCodec &&
		identity.RequestedLanguage == requested.RequestedLanguage &&
		identity.OCRMode == requested.OCRMode &&
		identity.PreparationScope == requested.PreparationScope &&
		identity.SegmentStart == requested.SegmentStart &&
		identity.SegmentDuration == requested.SegmentDuration &&
		identity.PreparedStreamMapping == requested.PreparedStreamMapping &&
		identity.PreparedRepresentation == requested.PreparedRepresentation &&
		identity.PipelineProvenance == requested.PipelineProvenance &&
		evidence.RequestedLanguage == request.Language && evidence.OCRMode == request.Mode
}

func canonicalOCRTranslationEvidenceEquivalent(left, right *canonicalOCREvidence) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.EffectiveLanguage == right.EffectiveLanguage &&
		left.RequestedLanguage == right.RequestedLanguage &&
		left.OCRMode == right.OCRMode &&
		reflect.DeepEqual(left.Cues, right.Cues) &&
		reflect.DeepEqual(left.CorrectionProvenance, right.CorrectionProvenance)
}
