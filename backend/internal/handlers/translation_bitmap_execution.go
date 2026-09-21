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
	OCRLanguage             string
	SourceLanguageMetadata  []string
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
	Source            *bitmapTranslationSource
	Outputs           map[string][]byte
	Metrics           TranslationProviderMetrics
	EffectiveLanguage string
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
		request.OCRLanguage,
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
	preparedEvidence, err := prepareCanonicalOCREvidenceForTranslation(ctx, source.Evidence, request.OnProgress)
	if err != nil {
		return bitmapTranslationExecutionResult{}, err
	}
	source.Evidence = preparedEvidence
	if len(source.Envelopes) == 0 {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("canonical bitmap OCR produced no usable translation envelopes")
	}
	representation := source.Envelopes[0].Representation
	source.Envelopes = translationCueEnvelopesForBitmapCues(
		representation.SourceIdentity,
		representation.SourceRepresentation,
		representation.LogicalTransformKey,
		preparedEvidence.Cues,
	)

	languageCues := make([]translationSourceCue, 0, len(preparedEvidence.Cues))
	for _, cue := range preparedEvidence.Cues {
		languageCues = append(languageCues, translationSourceCue{
			SourceOrdinal:    cue.SourceOrdinal,
			Text:             cue.Text,
			Start:            cue.Start,
			End:              cue.End,
			TimingProvenance: cue.TimingProvenance,
		})
	}
	language, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{
		RequestedLanguage:      request.RequestedLanguage,
		SourceLanguageMetadata: request.SourceLanguageMetadata,
		Cues:                   languageCues,
	})
	if err != nil {
		return bitmapTranslationExecutionResult{}, fmt.Errorf("canonical OCR source language is unavailable: %w", err)
	}
	effectiveLanguage := language.EffectiveLanguage
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
	return bitmapTranslationExecutionResult{Source: source, Outputs: outputs, Metrics: metrics, EffectiveLanguage: effectiveLanguage}, nil
}

func validateBitmapTranslationRequestedRepresentation(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", translationRepresentationSRT, translationRepresentationASS, "both":
		return nil
	default:
		return fmt.Errorf("bitmap translation does not support output representation %s", value)
	}
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
