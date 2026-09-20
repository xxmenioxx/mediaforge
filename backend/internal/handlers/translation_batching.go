package handlers

import (
	"encoding/json"
	"fmt"
)

// translationBatchesForCueEnvelopes packs canonical cues in source order. The
// provider sees only one text-only TranslationCue per canonical envelope; all
// source identity remains in the envelope outside this batching boundary.
func translationBatchesForCueEnvelopes(sourceLanguage, targetLocale string, envelopes []translationCueEnvelope, context *TranslationContext, settings translationSettings) ([]TranslationBatch, error) {
	cues := make([]TranslationCue, 0, len(envelopes))
	for _, envelope := range envelopes {
		cues = append(cues, TranslationCue{ID: envelope.StableID, Text: envelope.Text})
	}
	return translationBatchesForCues(sourceLanguage, targetLocale, cues, context, settings)
}

// translationBatchesForCues uses ordered first-fit packing. The existing
// provider validator is the authoritative check for both cue-count and the
// encoded JSON request-size limit, including any context attached to a batch.
func translationBatchesForCues(sourceLanguage, targetLocale string, cues []TranslationCue, context *TranslationContext, settings translationSettings) ([]TranslationBatch, error) {
	if settings.MaxCuesPerRequest <= 0 || settings.MaxRequestBytes <= 0 {
		return nil, fmt.Errorf("translation batching limits must be positive")
	}
	if len(cues) == 0 {
		return []TranslationBatch{}, nil
	}

	base := TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  sourceLanguage,
		TargetLocale:    targetLocale,
		Context:         context,
	}
	batches := make([]TranslationBatch, 0, (len(cues)+settings.MaxCuesPerRequest-1)/settings.MaxCuesPerRequest)
	current := base
	for _, cue := range cues {
		candidate := batchWithCue(current, cue)
		if err := validateTranslationBatch(candidate, settings); err == nil {
			current = candidate
			continue
		}

		if len(current.Cues) == 0 {
			return nil, oversizedTranslationCueError(candidate, cue, settings)
		}
		batches = append(batches, current)
		single := batchWithCue(base, cue)
		if err := validateTranslationBatch(single, settings); err != nil {
			return nil, oversizedTranslationCueError(single, cue, settings)
		}
		current = single
	}
	if len(current.Cues) > 0 {
		batches = append(batches, current)
	}

	for index, batch := range batches {
		if err := validateTranslationBatch(batch, settings); err != nil {
			return nil, fmt.Errorf("validate translation batch %d: %w", index, err)
		}
	}
	if err := validateTranslationBatchOrder(cues, batches); err != nil {
		return nil, err
	}
	return batches, nil
}

func batchWithCue(batch TranslationBatch, cue TranslationCue) TranslationBatch {
	batch.Cues = append(append([]TranslationCue(nil), batch.Cues...), cue)
	return batch
}

func oversizedTranslationCueError(batch TranslationBatch, cue TranslationCue, settings translationSettings) error {
	encoded, err := json.Marshal(batch)
	if err == nil && len(encoded) > settings.MaxRequestBytes {
		return fmt.Errorf("translation cue %q exceeds max request size of %d bytes", cue.ID, settings.MaxRequestBytes)
	}
	return fmt.Errorf("translation cue %q cannot form a valid translation batch", cue.ID)
}

func validateTranslationBatchOrder(cues []TranslationCue, batches []TranslationBatch) error {
	flat := make([]TranslationCue, 0, len(cues))
	for _, batch := range batches {
		flat = append(flat, batch.Cues...)
	}
	if len(flat) != len(cues) {
		return fmt.Errorf("translation batches do not preserve cue count")
	}
	for index := range cues {
		if flat[index].ID != cues[index].ID || flat[index].Text != cues[index].Text {
			return fmt.Errorf("translation batches do not preserve cue order at index %d", index)
		}
	}
	return nil
}
