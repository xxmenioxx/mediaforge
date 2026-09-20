package handlers

import (
	"fmt"
	"strings"
)

const (
	translationProviderOpenAI = "openai"
	translationProviderDeepL  = "deepl"
	translationProviderGoogle = "google"
	translationProviderAzure  = "azure"
)

// translationProviderForSettings is the single provider-selection seam. V1
// intentionally constructs only the existing Ollama adapter; the other
// identifiers reserve explicit failure states for future adapters without
// making them usable settings.
func translationProviderForSettings(settings translationSettings) (TranslationProvider, error) {
	providerName := strings.TrimSpace(settings.Provider)
	switch providerName {
	case "":
		return nil, translationProviderConfigurationError("translation provider is not configured")
	case translationProviderOllama:
		settings.Provider = providerName
		provider, err := newOllamaTranslationProvider(settings)
		if err != nil {
			return nil, translationProviderConfigurationError("%s", err.Error())
		}
		return provider, nil
	case translationProviderOpenAI, translationProviderDeepL, translationProviderGoogle, translationProviderAzure:
		return nil, translationProviderConfigurationError("translation provider %q is not implemented", providerName)
	default:
		return nil, translationProviderConfigurationError("unknown translation provider %q", providerName)
	}
}

func translationProviderConfigurationError(format string, args ...any) error {
	return &TranslationProviderError{
		Class: translationProviderErrorConfiguration,
		Err:   fmt.Errorf(format, args...),
	}
}
