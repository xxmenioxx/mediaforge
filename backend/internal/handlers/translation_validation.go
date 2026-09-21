package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// translationProviderResultWire is deliberately narrower than the internal
// provider result. Strict decoding prevents a provider from smuggling timing,
// ordering, or subtitle structure through an otherwise valid cue result.
type translationProviderResultWire struct {
	ContractVersion string                         `json:"contractVersion"`
	Cues            []translationProviderCueWire   `json:"cues"`
	Metrics         translationProviderMetricsWire `json:"metrics"`
}

type translationProviderCueWire struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type translationProviderMetricsWire struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	RequestCount  int    `json:"requestCount"`
	InputTokens   int    `json:"inputTokens"`
	OutputTokens  int    `json:"outputTokens"`
	LatencyMillis int64  `json:"latencyMillis"`
}

// decodeTranslationBatchResult decodes the provider wire contract without
// accepting fields that are not owned by the text-only provider boundary.
func decodeTranslationBatchResult(payload []byte) (TranslationBatchResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()

	var wire translationProviderResultWire
	if err := decoder.Decode(&wire); err != nil {
		return TranslationBatchResult{}, malformedTranslationResultError("decode provider response: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return TranslationBatchResult{}, malformedTranslationResultError("provider response contains multiple JSON values")
		}
		return TranslationBatchResult{}, malformedTranslationResultError("decode trailing provider response: %v", err)
	}

	result := TranslationBatchResult{
		ContractVersion: wire.ContractVersion,
		Cues:            make([]TranslatedCue, 0, len(wire.Cues)),
		Metrics: TranslationProviderMetrics{
			Provider:      wire.Metrics.Provider,
			Model:         wire.Metrics.Model,
			RequestCount:  wire.Metrics.RequestCount,
			InputTokens:   wire.Metrics.InputTokens,
			OutputTokens:  wire.Metrics.OutputTokens,
			LatencyMillis: wire.Metrics.LatencyMillis,
		},
	}
	for _, cue := range wire.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: cue.Text})
	}
	return result, nil
}

// validateTranslationBatchResult validates the complete provider response
// before any translated text can be handed to a serializer. It returns a new
// slice in request order only after every result has passed validation.
func validateTranslationBatchResult(batch TranslationBatch, result TranslationBatchResult) ([]TranslatedCue, error) {
	if batch.ContractVersion != translationProviderContractVersion {
		return nil, malformedTranslationResultError("unsupported translation request contract version")
	}
	if result.ContractVersion != batch.ContractVersion {
		return nil, malformedTranslationResultError("unsupported translation response contract version")
	}
	if len(result.Cues) != len(batch.Cues) {
		return nil, malformedTranslationResultError("translation response cue count does not match request")
	}

	expectedIndexes := make(map[string]int, len(batch.Cues))
	for index, cue := range batch.Cues {
		if strings.TrimSpace(cue.ID) == "" {
			return nil, malformedTranslationResultError("translation request contains an empty cue ID")
		}
		if _, exists := expectedIndexes[cue.ID]; exists {
			return nil, malformedTranslationResultError("translation request contains duplicate cue ID %q", cue.ID)
		}
		expectedIndexes[cue.ID] = index
	}

	validated := make([]TranslatedCue, len(result.Cues))
	seen := make(map[string]struct{}, len(result.Cues))
	for index, cue := range result.Cues {
		if strings.TrimSpace(cue.ID) == "" {
			return nil, malformedTranslationResultError("translation response contains an empty cue ID")
		}
		if _, exists := seen[cue.ID]; exists {
			return nil, malformedTranslationResultError("translation response contains duplicate cue ID %q", cue.ID)
		}
		seen[cue.ID] = struct{}{}

		expectedIndex, exists := expectedIndexes[cue.ID]
		if !exists {
			return nil, malformedTranslationResultError("translation response contains unexpected cue ID %q", cue.ID)
		}
		if expectedIndex != index {
			return nil, malformedTranslationResultError("translation response reordered cue ID %q", cue.ID)
		}
		if strings.TrimSpace(cue.Text) == "" {
			return nil, malformedTranslationResultError("translation response contains empty text for cue %q", cue.ID)
		}
		validated[index] = cue
	}

	return validated, nil
}

// validateTranslationASSBatchResult applies generic cue validation and then
// reuses the existing ASS/SSA protected-dialogue validator for every
// translatable event before reconstruction or serialization.
func validateTranslationASSBatchResult(document translationASSDocument, batch TranslationBatch, result TranslationBatchResult) ([]TranslatedCue, error) {
	validated, err := validateTranslationBatchResult(batch, result)
	if err != nil {
		return nil, err
	}

	translatableEvents := make([]translationASSEvent, 0, len(document.Events))
	for _, event := range document.Events {
		if event.Dialogue.HasTranslatableText() {
			translatableEvents = append(translatableEvents, event)
		}
	}
	if len(translatableEvents) != len(validated) {
		return nil, malformedTranslationResultError("translation response does not match ASS/SSA dialogue cue count")
	}
	for index, event := range translatableEvents {
		if _, err := event.Dialogue.ReconstructProviderText(validated[index].Text); err != nil {
			return nil, malformedTranslationResultError("validate ASS/SSA cue %d: %v", index, err)
		}
	}
	return validated, nil
}

func malformedTranslationResultError(format string, args ...any) error {
	return &TranslationProviderError{
		Class: translationProviderErrorMalformed,
		Err:   fmt.Errorf(format, args...),
	}
}
