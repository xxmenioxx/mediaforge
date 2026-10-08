package handlers

import (
	"context"
	"fmt"
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
	evidence, foundPublished := publishedCanonicalOCREvidenceForTranslation(mediaPath, stream)
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

func publishedCanonicalOCREvidenceForTranslation(mediaPath string, stream FFProbeStream) (*canonicalOCREvidence, bool) {
	artifact, found := findPublishedCanonicalOCRArtifactForEmbeddedSource(mediaPath, stream)
	if !found {
		return nil, false
	}
	snapshot, valid := loadPublishedCanonicalOCRSnapshot(artifact.Path, artifact.Format, artifact.LogicalTransform)
	if !valid || snapshot == nil {
		return nil, false
	}
	evidence := &snapshot.Evidence
	streamID := strings.TrimSpace(fmt.Sprint(stream.ID))
	if evidence.Identity.StreamIndex != stream.Index || evidence.Identity.StreamCodec != strings.ToLower(strings.TrimSpace(stream.CodecName)) ||
		(streamID != "" && strings.TrimSpace(evidence.Identity.StreamID) != streamID) || !evidence.Complete || len(evidence.Cues) == 0 ||
		strings.TrimSpace(evidence.EffectiveLanguage) == "" {
		return nil, false
	}
	return cloneCanonicalOCREvidence(evidence), true
}
