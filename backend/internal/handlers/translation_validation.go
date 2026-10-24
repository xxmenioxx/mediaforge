package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// translationProviderResultWire is deliberately narrower than the internal
// provider result. Strict decoding prevents a provider from smuggling timing,
// ordering, or subtitle structure through an otherwise valid cue result.
const (
	translationResponseDiagnosticOuterEnvelopeInvalid    = "outer_envelope_invalid_json"
	translationResponseDiagnosticOuterEnvelopeTruncated  = "outer_envelope_truncated"
	translationResponseDiagnosticAssistantContentEmpty   = "assistant_content_empty"
	translationResponseDiagnosticInnerJSONInvalid        = "inner_json_invalid"
	translationResponseDiagnosticInnerJSONTruncated      = "inner_json_truncated"
	translationResponseDiagnosticInnerJSONTrailing       = "inner_json_trailing_data"
	translationResponseDiagnosticResponseTruncated       = "response_truncated"
	translationResponseDiagnosticResponseTooLarge        = "response_too_large"
	translationResponseDiagnosticContractVersionMismatch = "contract_version_mismatch"
	translationResponseDiagnosticCueCountMismatch        = "cue_count_mismatch"
	translationResponseDiagnosticCueIdentityInvalid      = "cue_identity_invalid"
	translationResponseDiagnosticCueOrderInvalid         = "cue_order_invalid"
	translationResponseDiagnosticCueTextEmpty            = "cue_text_empty"
	translationResponseDiagnosticASSStructureInvalid     = "ass_structure_invalid"
	translationResponseDiagnosticHTTPStatus              = "http_non_2xx"
	translationResponseDiagnosticProviderUnavailable     = "provider_unavailable"
	translationResponseDiagnosticProviderTimeout         = "provider_timeout"
	translationResponseDiagnosticProviderCancelled       = "provider_cancelled"
	translationResponseDiagnosticProviderConfiguration   = "provider_configuration"
	translationResponseDiagnosticProviderResponseInvalid = "provider_response_invalid"
)

type translationResponseDiagnosticError struct {
	Category         string
	BatchIndex       int
	BatchCount       int
	ExpectedCueCount *int
	ActualCueCount   *int
	HTTPStatus       int
	Cause            error
}

func (diagnostic *translationResponseDiagnosticError) Error() string {
	if diagnostic == nil {
		return "translation response rejected"
	}
	if diagnostic.HTTPStatus > 0 {
		return fmt.Sprintf("Ollama returned HTTP status %d", diagnostic.HTTPStatus)
	}
	return "translation response rejected (" + diagnostic.Category + ")"
}

func (diagnostic *translationResponseDiagnosticError) Unwrap() error {
	if diagnostic == nil {
		return nil
	}
	return diagnostic.Cause
}

func malformedTranslationResponseError(category string, expectedCueCount, actualCueCount *int) *TranslationProviderError {
	return &TranslationProviderError{
		Class: translationProviderErrorMalformed,
		Err: &translationResponseDiagnosticError{
			Category: category, ExpectedCueCount: expectedCueCount, ActualCueCount: actualCueCount,
		},
	}
}

func translationCueCountPointer(count int) *int {
	return &count
}

func withTranslationBatchDiagnostic(err error, batchIndex, batchCount, expectedCueCount int) error {
	if err == nil {
		return nil
	}
	var existing *translationResponseDiagnosticError
	diagnostic := translationResponseDiagnosticError{}
	if errors.As(err, &existing) && existing != nil {
		diagnostic = *existing
	} else {
		switch translationProviderErrorClassOf(err) {
		case translationProviderErrorUnavailable:
			diagnostic.Category = translationResponseDiagnosticProviderUnavailable
		case translationProviderErrorTimeout:
			diagnostic.Category = translationResponseDiagnosticProviderTimeout
		case translationProviderErrorCancelled:
			diagnostic.Category = translationResponseDiagnosticProviderCancelled
		case translationProviderErrorConfiguration:
			diagnostic.Category = translationResponseDiagnosticProviderConfiguration
		default:
			diagnostic.Category = translationResponseDiagnosticProviderResponseInvalid
		}
	}
	diagnostic.BatchIndex = batchIndex + 1
	diagnostic.BatchCount = batchCount
	diagnostic.Cause = err
	if diagnostic.ExpectedCueCount == nil {
		diagnostic.ExpectedCueCount = translationCueCountPointer(expectedCueCount)
	}
	return &diagnostic
}

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
		category := translationResponseDiagnosticInnerJSONInvalid
		if errors.Is(err, io.ErrUnexpectedEOF) {
			category = translationResponseDiagnosticInnerJSONTruncated
		}
		return TranslationBatchResult{}, malformedTranslationResponseError(category, nil, nil)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return TranslationBatchResult{}, malformedTranslationResponseError(translationResponseDiagnosticInnerJSONTrailing, nil, nil)
		}
		category := translationResponseDiagnosticInnerJSONTrailing
		if errors.Is(err, io.ErrUnexpectedEOF) {
			category = translationResponseDiagnosticInnerJSONTruncated
		}
		return TranslationBatchResult{}, malformedTranslationResponseError(category, nil, nil)
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
		return nil, malformedTranslationResponseError(translationResponseDiagnosticContractVersionMismatch, nil, nil)
	}
	if len(result.Cues) != len(batch.Cues) {
		return nil, malformedTranslationResponseError(translationResponseDiagnosticCueCountMismatch, translationCueCountPointer(len(batch.Cues)), translationCueCountPointer(len(result.Cues)))
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
			return nil, malformedTranslationResponseError(translationResponseDiagnosticCueIdentityInvalid, nil, nil)
		}
		if _, exists := seen[cue.ID]; exists {
			return nil, malformedTranslationResponseError(translationResponseDiagnosticCueIdentityInvalid, nil, nil)
		}
		seen[cue.ID] = struct{}{}

		expectedIndex, exists := expectedIndexes[cue.ID]
		if !exists {
			return nil, malformedTranslationResponseError(translationResponseDiagnosticCueIdentityInvalid, nil, nil)
		}
		if expectedIndex != index {
			return nil, malformedTranslationResponseError(translationResponseDiagnosticCueOrderInvalid, nil, nil)
		}
		if strings.TrimSpace(cue.Text) == "" {
			return nil, malformedTranslationResponseError(translationResponseDiagnosticCueTextEmpty, nil, nil)
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
		return nil, malformedTranslationResponseError(translationResponseDiagnosticCueCountMismatch, translationCueCountPointer(len(translatableEvents)), translationCueCountPointer(len(validated)))
	}
	for index, event := range translatableEvents {
		if _, err := event.Dialogue.ReconstructProviderText(validated[index].Text); err != nil {
			return nil, malformedTranslationResponseError(translationResponseDiagnosticASSStructureInvalid, nil, nil)
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
