package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	translationIdentityVersion         = 1
	translationIdentityContractVersion = "subtitle-translation-v1"
)

// translationIdentityInput contains only caller-supplied canonical identity
// inputs. Source revision capture and validation are deliberately outside this
// helper and belong to the later revision-protection task.
type translationIdentityInput struct {
	SourceArtifactIdentity  string
	SourceRevision          string
	SourceRepresentation    string
	SourceProvenance        string
	RequestedSourceLanguage string
	EffectiveSourceLanguage string
	TargetLocale            string
	Provider                string
	ProviderContractVersion string
	ProviderModel           string
	ContextContractVersion  string
	ContextMode             string
	OutputRepresentation    string
}

type translationIdentity struct {
	Version                    int    `json:"version"`
	TranslationContractVersion string `json:"translationContractVersion"`
	SourceArtifactIdentity     string `json:"sourceArtifactIdentity"`
	SourceRevision             string `json:"sourceRevision"`
	SourceRepresentation       string `json:"sourceRepresentation"`
	SourceProvenance           string `json:"sourceProvenance"`
	RequestedSourceLanguage    string `json:"requestedSourceLanguage"`
	EffectiveSourceLanguage    string `json:"effectiveSourceLanguage"`
	TargetLocale               string `json:"targetLocale"`
	Provider                   string `json:"provider"`
	ProviderContractVersion    string `json:"providerContractVersion"`
	ProviderModel              string `json:"providerModel"`
	ContextContractVersion     string `json:"contextContractVersion"`
	ContextMode                string `json:"contextMode"`
	OutputRepresentation       string `json:"outputRepresentation"`
}

// SubtitleTranslationMetadata is bounded artifact metadata for a translated
// subtitle. It extends SubtitleArtifact rather than creating another artifact
// type or publication path.
type SubtitleTranslationMetadata struct {
	IdentityKey                string                         `json:"identityKey"`
	IdentityVersion            int                            `json:"identityVersion"`
	TranslationContractVersion string                         `json:"translationContractVersion"`
	SourceArtifactIdentity     string                         `json:"sourceArtifactIdentity"`
	SourceRevision             string                         `json:"sourceRevision"`
	SourceRepresentation       string                         `json:"sourceRepresentation"`
	SourceProvenance           string                         `json:"sourceProvenance"`
	RequestedSourceLanguage    string                         `json:"requestedSourceLanguage"`
	EffectiveSourceLanguage    string                         `json:"effectiveSourceLanguage"`
	TargetLocale               string                         `json:"targetLocale"`
	Provider                   string                         `json:"provider"`
	ProviderContractVersion    string                         `json:"providerContractVersion"`
	ProviderModel              string                         `json:"providerModel"`
	ContextContractVersion     string                         `json:"contextContractVersion"`
	ContextMode                string                         `json:"contextMode"`
	OutputRepresentation       string                         `json:"outputRepresentation"`
	Provenance                 *SubtitleTranslationProvenance `json:"provenance,omitempty"`
}

func newTranslationIdentity(input translationIdentityInput) (translationIdentity, error) {
	targetLocale, err := normalizeTranslationTargetLocale(input.TargetLocale)
	if err != nil {
		return translationIdentity{}, err
	}
	requestedLanguage, err := normalizeTranslationLanguage(input.RequestedSourceLanguage, true)
	if err != nil {
		return translationIdentity{}, fmt.Errorf("requested source language: %w", err)
	}
	effectiveLanguage, err := normalizeTranslationLanguage(input.EffectiveSourceLanguage, false)
	if err != nil {
		return translationIdentity{}, fmt.Errorf("effective source language: %w", err)
	}

	identity := translationIdentity{
		Version:                    translationIdentityVersion,
		TranslationContractVersion: translationIdentityContractVersion,
		SourceArtifactIdentity:     strings.TrimSpace(input.SourceArtifactIdentity),
		SourceRevision:             strings.TrimSpace(input.SourceRevision),
		SourceRepresentation:       normalizeTranslationRepresentation(input.SourceRepresentation),
		SourceProvenance:           strings.TrimSpace(input.SourceProvenance),
		RequestedSourceLanguage:    requestedLanguage,
		EffectiveSourceLanguage:    effectiveLanguage,
		TargetLocale:               targetLocale,
		Provider:                   strings.ToLower(strings.TrimSpace(input.Provider)),
		ProviderContractVersion:    strings.TrimSpace(input.ProviderContractVersion),
		ProviderModel:              strings.TrimSpace(input.ProviderModel),
		ContextContractVersion:     strings.TrimSpace(input.ContextContractVersion),
		ContextMode:                strings.ToLower(strings.TrimSpace(input.ContextMode)),
		OutputRepresentation:       normalizeTranslationRepresentation(input.OutputRepresentation),
	}
	for name, value := range map[string]string{
		"source artifact identity":  identity.SourceArtifactIdentity,
		"source revision":           identity.SourceRevision,
		"source representation":     identity.SourceRepresentation,
		"source provenance":         identity.SourceProvenance,
		"provider":                  identity.Provider,
		"provider contract version": identity.ProviderContractVersion,
		"provider model":            identity.ProviderModel,
		"context contract version":  identity.ContextContractVersion,
		"context mode":              identity.ContextMode,
		"output representation":     identity.OutputRepresentation,
	} {
		if value == "" {
			return translationIdentity{}, fmt.Errorf("translation identity requires %s", name)
		}
	}
	if identity.OutputRepresentation != translationRepresentationSRT && identity.OutputRepresentation != translationRepresentationASS && identity.OutputRepresentation != translationRepresentationSSA {
		return translationIdentity{}, fmt.Errorf("unsupported translation output representation %q", identity.OutputRepresentation)
	}
	return identity, nil
}

func (identity translationIdentity) key() string {
	encoded, _ := json.Marshal(identity)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func (identity translationIdentity) metadata() SubtitleTranslationMetadata {
	return SubtitleTranslationMetadata{
		IdentityKey:                identity.key(),
		IdentityVersion:            identity.Version,
		TranslationContractVersion: identity.TranslationContractVersion,
		SourceArtifactIdentity:     identity.SourceArtifactIdentity,
		SourceRevision:             identity.SourceRevision,
		SourceRepresentation:       identity.SourceRepresentation,
		SourceProvenance:           identity.SourceProvenance,
		RequestedSourceLanguage:    identity.RequestedSourceLanguage,
		EffectiveSourceLanguage:    identity.EffectiveSourceLanguage,
		TargetLocale:               identity.TargetLocale,
		Provider:                   identity.Provider,
		ProviderContractVersion:    identity.ProviderContractVersion,
		ProviderModel:              identity.ProviderModel,
		ContextContractVersion:     identity.ContextContractVersion,
		ContextMode:                identity.ContextMode,
		OutputRepresentation:       identity.OutputRepresentation,
	}
}

func translationArtifactMatchesIdentity(artifact SubtitleArtifact, identity translationIdentity) bool {
	if artifact.Translation == nil {
		return false
	}
	if artifact.Translation.Provenance != nil {
		if err := validateSubtitleTranslationProvenance(*artifact.Translation, artifact.Translation.Provenance); err != nil {
			return false
		}
	}
	actual := *artifact.Translation
	actual.Provenance = nil
	return actual == identity.metadata()
}

func translationIdentityFromMetadata(metadata SubtitleTranslationMetadata) (translationIdentity, error) {
	identity, err := newTranslationIdentity(translationIdentityInput{
		SourceArtifactIdentity:  metadata.SourceArtifactIdentity,
		SourceRevision:          metadata.SourceRevision,
		SourceRepresentation:    metadata.SourceRepresentation,
		SourceProvenance:        metadata.SourceProvenance,
		RequestedSourceLanguage: metadata.RequestedSourceLanguage,
		EffectiveSourceLanguage: metadata.EffectiveSourceLanguage,
		TargetLocale:            metadata.TargetLocale,
		Provider:                metadata.Provider,
		ProviderContractVersion: metadata.ProviderContractVersion,
		ProviderModel:           metadata.ProviderModel,
		ContextContractVersion:  metadata.ContextContractVersion,
		ContextMode:             metadata.ContextMode,
		OutputRepresentation:    metadata.OutputRepresentation,
	})
	if err != nil {
		return translationIdentity{}, err
	}
	if metadata.IdentityVersion != identity.Version || metadata.TranslationContractVersion != identity.TranslationContractVersion || metadata.IdentityKey != identity.key() {
		return translationIdentity{}, fmt.Errorf("translation metadata identity key or version does not match canonical identity")
	}
	return identity, nil
}

func subtitleTranslationArtifactID(source SubtitleArtifact, metadata SubtitleTranslationMetadata) string {
	if metadata.IdentityKey == "" {
		return ""
	}
	sourceID := source.ArtifactID
	if sourceID == "" {
		sourceID = subtitleArtifactID(source.StreamIndex, source.Mode, source.Format)
	}
	return fmt.Sprintf("%s:translation:%s", sourceID, metadata.IdentityKey)
}

func resolvedTranslatedSubtitleStagedPath(base string, artifact SubtitleArtifact, metadata SubtitleTranslationMetadata, used map[string]struct{}) (string, error) {
	locale, err := normalizeTranslationTargetLocale(metadata.TargetLocale)
	if err != nil {
		return "", err
	}
	if normalizeTranslationRepresentation(artifact.Format) != metadata.OutputRepresentation {
		return "", fmt.Errorf("translated artifact format %q does not match output representation %q", artifact.Format, metadata.OutputRepresentation)
	}
	localePart := safeSubtitleFilenamePart(locale)
	if localePart == "" {
		return "", fmt.Errorf("translated artifact locale cannot be represented safely")
	}
	return resolvedSubtitleStagedPath(base+"."+localePart, artifact, used), nil
}

func normalizeTranslationTargetLocale(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "_", "-"))) {
	case "es-419":
		return "es-419", nil
	case "es-es":
		return "es-ES", nil
	default:
		return "", fmt.Errorf("unsupported translation target locale %q", value)
	}
}

func normalizeTranslationLanguage(value string, allowAuto bool) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		if allowAuto {
			return "auto", nil
		}
	case "en", "eng", "english":
		return "en", nil
	case "ja", "jpn", "japanese":
		return "ja", nil
	}
	return "", fmt.Errorf("unsupported translation language %q", value)
}

func normalizeTranslationRepresentation(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func subtitleTranslationMetadataFromJSON(value any) *SubtitleTranslationMetadata {
	switch typed := value.(type) {
	case *SubtitleTranslationMetadata:
		return typed
	case SubtitleTranslationMetadata:
		copy := typed
		return &copy
	}
	raw, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	return &SubtitleTranslationMetadata{
		IdentityKey:                stringFromUnknown(raw["identityKey"]),
		IdentityVersion:            intValueSetting(raw["identityVersion"], 0),
		TranslationContractVersion: stringFromUnknown(raw["translationContractVersion"]),
		SourceArtifactIdentity:     stringFromUnknown(raw["sourceArtifactIdentity"]),
		SourceRevision:             stringFromUnknown(raw["sourceRevision"]),
		SourceRepresentation:       stringFromUnknown(raw["sourceRepresentation"]),
		SourceProvenance:           stringFromUnknown(raw["sourceProvenance"]),
		RequestedSourceLanguage:    stringFromUnknown(raw["requestedSourceLanguage"]),
		EffectiveSourceLanguage:    stringFromUnknown(raw["effectiveSourceLanguage"]),
		TargetLocale:               stringFromUnknown(raw["targetLocale"]),
		Provider:                   stringFromUnknown(raw["provider"]),
		ProviderContractVersion:    stringFromUnknown(raw["providerContractVersion"]),
		ProviderModel:              stringFromUnknown(raw["providerModel"]),
		ContextContractVersion:     stringFromUnknown(raw["contextContractVersion"]),
		ContextMode:                stringFromUnknown(raw["contextMode"]),
		OutputRepresentation:       stringFromUnknown(raw["outputRepresentation"]),
		Provenance:                 subtitleTranslationProvenanceFromJSON(raw["provenance"]),
	}
}
