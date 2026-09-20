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
	case "hdmv_pgs_subtitle", "pgssub", "dvd_subtitle", "dvb_subtitle":
	default:
		return nil, fmt.Errorf("bitmap translation does not support subtitle codec %q", stream.CodecName)
	}
	request := canonicalOCRRequestFor(mediaPath, stream, requestedLanguage, ocrMode, segmentStart, segmentDuration)
	evidence, err := canonicalOCREvidenceForRequest(ctx, request, onProgress)
	if err != nil {
		return nil, fmt.Errorf("canonical bitmap OCR failed: %w", err)
	}
	if evidence == nil || !evidence.Complete || len(evidence.Cues) == 0 || strings.TrimSpace(evidence.EffectiveLanguage) == "" {
		return nil, fmt.Errorf("canonical bitmap OCR returned incomplete evidence")
	}
	finalEvidence, err := correctedCanonicalOCREvidenceForTranslation(evidence)
	if err != nil {
		return nil, fmt.Errorf("canonical bitmap OCR correction failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceIdentity) == "" {
		sourceIdentity = request.Identity.key()
	}
	if strings.TrimSpace(sourceRepresentation) == "" {
		sourceRepresentation = codec
	}
	envelopes := translationCueEnvelopesForBitmapCues(sourceIdentity, sourceRepresentation, logicalTransformKey, finalEvidence.Cues)
	if len(envelopes) != len(finalEvidence.Cues) || len(envelopes) == 0 {
		return nil, fmt.Errorf("canonical bitmap OCR produced no usable translation envelopes")
	}
	return &bitmapTranslationSource{
		EffectiveLanguage: finalEvidence.EffectiveLanguage,
		Evidence:          finalEvidence,
		Envelopes:         envelopes,
	}, nil
}
