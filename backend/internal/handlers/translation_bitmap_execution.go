package handlers

import (
	"context"
	"fmt"
	"strings"
)

// bitmapTranslationExecutionRequest is an operation-local input for the
// bitmap translation boundary. It intentionally carries no publication or
// operation state; callers remain responsible for those concerns.
type bitmapTranslationExecutionRequest struct {
	MediaPath               string
	Stream                  FFProbeStream
	RequestedLanguage       string
	OCRMode                 string
	SegmentStart            float64
	SegmentDuration         int
	SourceIdentity          string
	SourceRepresentation    string
	LogicalTransformKey     string
	TargetLocale            string
	RequestedRepresentation string
	Provider                TranslationProvider
	Settings                translationSettings
	TranslationContext      *TranslationContext
	OnProgress              OCRProgressFunc
}

type bitmapTranslationExecutionResult struct {
	Source  *bitmapTranslationSource
	Outputs map[string][]byte
	Metrics TranslationProviderMetrics
}

// executeBitmapTranslation is the single provider boundary for bitmap
// subtitle translation. Canonical OCR and its correction policy complete
// before batching or provider work begins; serializers consume one merged
// canonical translated cue set after all provider results validate.
func executeBitmapTranslation(ctx context.Context, request bitmapTranslationExecutionRequest) (bitmapTranslationExecutionResult, error) {
	if ctx == nil {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("bitmap translation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return bitmapTranslationExecutionResult{}, err
	}
	if request.Provider == nil {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("bitmap translation provider is required")
	}
	if err := validateBitmapTranslationRequestedRepresentation(request.RequestedRepresentation); err != nil {
		return bitmapTranslationExecutionResult{}, err
	}

	source, err := bitmapTranslationSourceFor(
		ctx,
		request.MediaPath,
		request.Stream,
		request.RequestedLanguage,
		request.OCRMode,
		request.SegmentStart,
		request.SegmentDuration,
		request.SourceIdentity,
		request.SourceRepresentation,
		request.LogicalTransformKey,
		request.OnProgress,
	)
	if err != nil {
		return bitmapTranslationExecutionResult{}, err
	}
	if err := validateBitmapTranslationQualityForTranslation(source.Evidence); err != nil {
		return bitmapTranslationExecutionResult{}, err
	}

	effectiveLanguage, found, err := translationLanguageFromMetadata([]string{source.EffectiveLanguage})
	if err != nil {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("canonical OCR effective language is unavailable: %w", err)
	}
	if !found || (effectiveLanguage != "en" && effectiveLanguage != "ja") {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("canonical OCR effective language %q is unsupported for translation", source.EffectiveLanguage)
	}
	if err := ctx.Err(); err != nil {
		return bitmapTranslationExecutionResult{}, err
	}

	batches, err := translationBatchesForCueEnvelopes(effectiveLanguage, request.TargetLocale, source.Envelopes, request.TranslationContext, request.Settings)
	if err != nil {
		return bitmapTranslationExecutionResult{}, err
	}
	if len(batches) == 0 {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("canonical bitmap OCR produced no translation batches")
	}

	translated := make([]TranslatedCue, 0, len(source.Envelopes))
	metrics := TranslationProviderMetrics{}
	for index, batch := range batches {
		if err := ctx.Err(); err != nil {
			return bitmapTranslationExecutionResult{}, err
		}
		result, err := request.Provider.Translate(ctx, batch)
		if err != nil {
			return bitmapTranslationExecutionResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return bitmapTranslationExecutionResult{}, err
		}
		validated, err := validateTranslationBatchResult(batch, result)
		if err != nil {
			return bitmapTranslationExecutionResult{}, fmt.Errorf("validate translated bitmap batch %d: %w", index, err)
		}
		translated = append(translated, validated...)
		metrics = aggregateTranslationProviderMetrics(metrics, result.Metrics)
	}

	outputs, err := serializeTranslatedBitmapCuesFromResult(source.Evidence.Cues, source.Envelopes, translated, request.RequestedRepresentation)
	if err != nil {
		return bitmapTranslationExecutionResult{}, err
	}
	return bitmapTranslationExecutionResult{Source: source, Outputs: outputs, Metrics: metrics}, nil
}

func validateBitmapTranslationRequestedRepresentation(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", translationRepresentationSRT, translationRepresentationASS, "both":
		return nil
	default:
		return fmt.Errorf("bitmap translation does not support output representation %s", value)
	}
}

// validateBitmapTranslationQualityForTranslation keeps unresolved OCR findings
// out of provider requests. It deliberately does not run selective OCR: that
// capability is bounded and owned by the OCR quality lifecycle, and a source
// that still needs it is not safe translation input in this task.
func validateBitmapTranslationQualityForTranslation(evidence *canonicalOCREvidence) error {
	if evidence == nil || !evidence.Complete || len(evidence.Cues) == 0 {
		return fmt.Errorf("canonical OCR evidence is incomplete for translation")
	}
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: *cloneCanonicalOCREvidence(evidence)}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	byID := make(map[int]canonicalBitmapSubtitleCue, len(evidence.Cues))
	for _, cue := range evidence.Cues {
		byID[cue.SourceOrdinal] = cue
	}
	for _, finding := range report.Findings {
		if finding.Status == subtitleOCRQualityStatusClean {
			continue
		}
		cue, ok := byID[finding.CueID]
		if !ok {
			return fmt.Errorf("canonical OCR cue %d cannot be safely translated", finding.CueID)
		}
		capability := selectiveOCRCueCapabilityFor(snapshot, cue)
		if !capability.Available {
			return fmt.Errorf("canonical OCR cue %d requires review before translation: %s", finding.CueID, capability.Reason)
		}
		return fmt.Errorf("canonical OCR cue %d requires bounded selective correction before translation", finding.CueID)
	}
	return nil
}

func aggregateTranslationProviderMetrics(total, current TranslationProviderMetrics) TranslationProviderMetrics {
	if total.Provider == "" {
		total.Provider = current.Provider
	}
	if total.Model == "" {
		total.Model = current.Model
	}
	total.RequestCount += current.RequestCount
	total.InputTokens += current.InputTokens
	total.OutputTokens += current.OutputTokens
	if current.LatencyMillis > 0 {
		total.LatencyMillis += current.LatencyMillis
	}
	return total
}
