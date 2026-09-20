package handlers

import (
	"fmt"
	"strings"
)

const (
	maxTranslationProvenanceString        = 4096
	maxTranslationProvenanceOperationID   = 256
	maxTranslationProvenanceContextSource = 256
	maxTranslationProvenanceStatus        = 64
	maxTranslationProvenanceRequests      = 100_000
	maxTranslationProvenanceTokens        = 100_000_000
	maxTranslationProvenanceLatencyMs     = int64(7 * 24 * 60 * 60 * 1000)
)

// SubtitleTranslationProvenance is deliberately limited to bounded audit
// fields. It has no request/response, prompt, header, credential, or cue
// payload fields, so those values cannot be persisted through this type.
type SubtitleTranslationProvenance struct {
	TranslationContractVersion string `json:"translationContractVersion"`
	IdentityKey                string `json:"identityKey"`
	OperationID                string `json:"operationId,omitempty"`
	Provider                   string `json:"provider"`
	ProviderModel              string `json:"providerModel"`
	ProviderContractVersion    string `json:"providerContractVersion"`
	RequestedSourceLanguage    string `json:"requestedSourceLanguage"`
	EffectiveSourceLanguage    string `json:"effectiveSourceLanguage"`
	TargetLocale               string `json:"targetLocale"`
	ContextContractVersion     string `json:"contextContractVersion"`
	ContextMode                string `json:"contextMode"`
	ContextSource              string `json:"contextSource,omitempty"`
	SourceRevision             string `json:"sourceRevision"`
	ValidationStatus           string `json:"validationStatus"`
	OutputRepresentation       string `json:"outputRepresentation"`
	RequestCount               int    `json:"requestCount,omitempty"`
	InputTokens                int    `json:"inputTokens,omitempty"`
	OutputTokens               int    `json:"outputTokens,omitempty"`
	LatencyMillis              int64  `json:"latencyMillis,omitempty"`
}

// SubtitleTranslationProvenanceView is the bounded API-safe projection for
// later Asset Info responses. It intentionally omits source revision and
// identity internals as well as every provider payload/configuration field.
type SubtitleTranslationProvenanceView struct {
	Provider                string `json:"provider"`
	ProviderModel           string `json:"providerModel"`
	RequestedSourceLanguage string `json:"requestedSourceLanguage"`
	EffectiveSourceLanguage string `json:"effectiveSourceLanguage"`
	TargetLocale            string `json:"targetLocale"`
	ContextMode             string `json:"contextMode"`
	ContextSource           string `json:"contextSource,omitempty"`
	ValidationStatus        string `json:"validationStatus"`
	RequestCount            int    `json:"requestCount,omitempty"`
	InputTokens             int    `json:"inputTokens,omitempty"`
	OutputTokens            int    `json:"outputTokens,omitempty"`
	LatencyMillis           int64  `json:"latencyMillis,omitempty"`
}

// buildSubtitleTranslationProvenance derives identity-bound fields from the
// canonical artifact metadata. Metric provider/model values, when present,
// must agree with that metadata; callers cannot supply an arbitrary audit map.
func buildSubtitleTranslationProvenance(metadata SubtitleTranslationMetadata, operationID, contextSource, validationStatus string, metrics TranslationProviderMetrics) (*SubtitleTranslationProvenance, error) {
	identity, err := translationIdentityFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	if metrics.Provider != "" && strings.ToLower(strings.TrimSpace(metrics.Provider)) != identity.Provider {
		return nil, fmt.Errorf("translation provenance provider does not match artifact identity")
	}
	if metrics.Model != "" && strings.TrimSpace(metrics.Model) != identity.ProviderModel {
		return nil, fmt.Errorf("translation provenance model does not match artifact identity")
	}
	if err := validateTranslationProvenanceString("operation identity", operationID, maxTranslationProvenanceOperationID, true); err != nil {
		return nil, err
	}
	if err := validateTranslationProvenanceString("context source", contextSource, maxTranslationProvenanceContextSource, true); err != nil {
		return nil, err
	}
	validationStatus = strings.ToLower(strings.TrimSpace(validationStatus))
	if err := validateTranslationProvenanceString("validation status", validationStatus, maxTranslationProvenanceStatus, false); err != nil {
		return nil, err
	}
	for name, value := range map[string]string{
		"translation contract version": identity.TranslationContractVersion,
		"identity key":                 identity.key(),
		"provider":                     identity.Provider,
		"provider model":               identity.ProviderModel,
		"provider contract version":    identity.ProviderContractVersion,
		"source revision":              identity.SourceRevision,
		"output representation":        identity.OutputRepresentation,
	} {
		if err := validateTranslationProvenanceString(name, value, maxTranslationProvenanceString, false); err != nil {
			return nil, err
		}
	}

	return &SubtitleTranslationProvenance{
		TranslationContractVersion: identity.TranslationContractVersion,
		IdentityKey:                identity.key(),
		OperationID:                strings.TrimSpace(operationID),
		Provider:                   identity.Provider,
		ProviderModel:              identity.ProviderModel,
		ProviderContractVersion:    identity.ProviderContractVersion,
		RequestedSourceLanguage:    identity.RequestedSourceLanguage,
		EffectiveSourceLanguage:    identity.EffectiveSourceLanguage,
		TargetLocale:               identity.TargetLocale,
		ContextContractVersion:     identity.ContextContractVersion,
		ContextMode:                identity.ContextMode,
		ContextSource:              strings.TrimSpace(contextSource),
		SourceRevision:             identity.SourceRevision,
		ValidationStatus:           validationStatus,
		OutputRepresentation:       identity.OutputRepresentation,
		RequestCount:               normalizeTranslationProvenanceMetric(metrics.RequestCount, maxTranslationProvenanceRequests),
		InputTokens:                normalizeTranslationProvenanceMetric(metrics.InputTokens, maxTranslationProvenanceTokens),
		OutputTokens:               normalizeTranslationProvenanceMetric(metrics.OutputTokens, maxTranslationProvenanceTokens),
		LatencyMillis:              normalizeTranslationProvenanceLatency(metrics.LatencyMillis),
	}, nil
}

// validateSubtitleTranslationProvenance is the trust boundary for persisted
// provenance. The identity fields are derived from the containing metadata,
// and every persisted bounded field is checked rather than normalized.
func validateSubtitleTranslationProvenance(metadata SubtitleTranslationMetadata, provenance *SubtitleTranslationProvenance) error {
	if provenance == nil {
		return fmt.Errorf("translation provenance is missing")
	}
	identity, err := translationIdentityFromMetadata(metadata)
	if err != nil {
		return err
	}
	identityFields := []struct {
		name     string
		actual   string
		expected string
	}{
		{"translation contract version", provenance.TranslationContractVersion, identity.TranslationContractVersion},
		{"identity key", provenance.IdentityKey, identity.key()},
		{"provider", provenance.Provider, identity.Provider},
		{"provider model", provenance.ProviderModel, identity.ProviderModel},
		{"provider contract version", provenance.ProviderContractVersion, identity.ProviderContractVersion},
		{"requested source language", provenance.RequestedSourceLanguage, identity.RequestedSourceLanguage},
		{"effective source language", provenance.EffectiveSourceLanguage, identity.EffectiveSourceLanguage},
		{"target locale", provenance.TargetLocale, identity.TargetLocale},
		{"context contract version", provenance.ContextContractVersion, identity.ContextContractVersion},
		{"context mode", provenance.ContextMode, identity.ContextMode},
		{"source revision", provenance.SourceRevision, identity.SourceRevision},
		{"output representation", provenance.OutputRepresentation, identity.OutputRepresentation},
	}
	for _, field := range identityFields {
		if err := validateTranslationProvenanceString(field.name, field.actual, maxTranslationProvenanceString, false); err != nil {
			return err
		}
		if field.actual != field.expected {
			return fmt.Errorf("translation provenance %s does not match artifact identity", field.name)
		}
	}
	if err := validateTranslationProvenanceString("operation identity", provenance.OperationID, maxTranslationProvenanceOperationID, true); err != nil {
		return err
	}
	if err := validateTranslationProvenanceString("context source", provenance.ContextSource, maxTranslationProvenanceContextSource, true); err != nil {
		return err
	}
	if err := validateTranslationProvenanceString("validation status", provenance.ValidationStatus, maxTranslationProvenanceStatus, false); err != nil {
		return err
	}
	if err := validateTranslationProvenanceMetric("request count", provenance.RequestCount, maxTranslationProvenanceRequests); err != nil {
		return err
	}
	if err := validateTranslationProvenanceMetric("input tokens", provenance.InputTokens, maxTranslationProvenanceTokens); err != nil {
		return err
	}
	if err := validateTranslationProvenanceMetric("output tokens", provenance.OutputTokens, maxTranslationProvenanceTokens); err != nil {
		return err
	}
	if provenance.LatencyMillis < 0 || provenance.LatencyMillis > maxTranslationProvenanceLatencyMs {
		return fmt.Errorf("translation provenance latency is outside the bounded range")
	}
	return nil
}

func validateTranslationProvenanceMetric(name string, value, maximum int) error {
	if value < 0 || value > maximum {
		return fmt.Errorf("translation provenance %s is outside the bounded range", name)
	}
	return nil
}

// Negative and excessive provider values are clamped to bounded values. Zero
// remains the explicit unavailable metric, avoiding invented measurements and
// preventing malformed providers from creating oversized artifact metadata.
func normalizeTranslationProvenanceMetric(value, maximum int) int {
	if value <= 0 {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}

func normalizeTranslationProvenanceLatency(value int64) int64 {
	if value <= 0 {
		return 0
	}
	if value > maxTranslationProvenanceLatencyMs {
		return maxTranslationProvenanceLatencyMs
	}
	return value
}

func validateTranslationProvenanceString(name, value string, maximum int, allowEmpty bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("translation provenance %s is required", name)
	}
	if len(value) > maximum {
		return fmt.Errorf("translation provenance %s exceeds bounded length", name)
	}
	return nil
}

func translationProvenanceAPISafeView(metadata SubtitleTranslationMetadata) (*SubtitleTranslationProvenanceView, error) {
	if metadata.Provenance == nil {
		return nil, nil
	}
	if err := validateSubtitleTranslationProvenance(metadata, metadata.Provenance); err != nil {
		return nil, err
	}
	provenance := metadata.Provenance
	return &SubtitleTranslationProvenanceView{
		Provider:                provenance.Provider,
		ProviderModel:           provenance.ProviderModel,
		RequestedSourceLanguage: provenance.RequestedSourceLanguage,
		EffectiveSourceLanguage: provenance.EffectiveSourceLanguage,
		TargetLocale:            provenance.TargetLocale,
		ContextMode:             provenance.ContextMode,
		ContextSource:           provenance.ContextSource,
		ValidationStatus:        provenance.ValidationStatus,
		RequestCount:            provenance.RequestCount,
		InputTokens:             provenance.InputTokens,
		OutputTokens:            provenance.OutputTokens,
		LatencyMillis:           provenance.LatencyMillis,
	}, nil
}

func subtitleTranslationProvenanceFromJSON(value any) *SubtitleTranslationProvenance {
	switch typed := value.(type) {
	case *SubtitleTranslationProvenance:
		return typed
	case SubtitleTranslationProvenance:
		copy := typed
		return &copy
	}
	raw, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return &SubtitleTranslationProvenance{
		TranslationContractVersion: stringFromUnknown(raw["translationContractVersion"]),
		IdentityKey:                stringFromUnknown(raw["identityKey"]),
		OperationID:                stringFromUnknown(raw["operationId"]),
		Provider:                   stringFromUnknown(raw["provider"]),
		ProviderModel:              stringFromUnknown(raw["providerModel"]),
		ProviderContractVersion:    stringFromUnknown(raw["providerContractVersion"]),
		RequestedSourceLanguage:    stringFromUnknown(raw["requestedSourceLanguage"]),
		EffectiveSourceLanguage:    stringFromUnknown(raw["effectiveSourceLanguage"]),
		TargetLocale:               stringFromUnknown(raw["targetLocale"]),
		ContextContractVersion:     stringFromUnknown(raw["contextContractVersion"]),
		ContextMode:                stringFromUnknown(raw["contextMode"]),
		ContextSource:              stringFromUnknown(raw["contextSource"]),
		SourceRevision:             stringFromUnknown(raw["sourceRevision"]),
		ValidationStatus:           stringFromUnknown(raw["validationStatus"]),
		OutputRepresentation:       stringFromUnknown(raw["outputRepresentation"]),
		RequestCount:               intValueSetting(raw["requestCount"], 0),
		InputTokens:                intValueSetting(raw["inputTokens"], 0),
		OutputTokens:               intValueSetting(raw["outputTokens"], 0),
		LatencyMillis:              int64(intValueSetting(raw["latencyMillis"], 0)),
	}
}
