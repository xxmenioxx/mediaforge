package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const translationProviderContractVersion = "translation-provider-v1"

type translationProviderErrorClass string

const (
	translationProviderErrorConfiguration translationProviderErrorClass = "configuration"
	translationProviderErrorUnavailable   translationProviderErrorClass = "unavailable/transport"
	translationProviderErrorTimeout       translationProviderErrorClass = "timeout"
	translationProviderErrorMalformed     translationProviderErrorClass = "malformed response"
	translationProviderErrorCancelled     translationProviderErrorClass = "cancelled"
)

// TranslationProvider is deliberately limited to text translation. MVForge
// retains cue timing, ordering, structure, geometry, serialization, and
// publication ownership outside this interface.
type TranslationProvider interface {
	Translate(context.Context, TranslationBatch) (TranslationBatchResult, error)
}

type TranslationBatch struct {
	ContractVersion string
	SourceLanguage  string
	TargetLocale    string
	Cues            []TranslationCue
	Context         *TranslationContext
}

func translationSourceLanguagesForProvider(provider, model string) []string {
	languages := []string{"auto", "en", "ja"}
	if supportsTranslationGemmaLanguagePair(provider, model) {
		languages = append(languages, "ko", "fr")
	}
	return languages
}

func supportsTranslationGemmaLanguagePair(provider, model string) bool {
	if !strings.EqualFold(strings.TrimSpace(provider), translationProviderOllama) {
		return false
	}
	model = strings.ToLower(strings.TrimSpace(model))
	if separator := strings.LastIndex(model, "/"); separator >= 0 {
		model = model[separator+1:]
	}
	switch model {
	case "translategemma", "translategemma:latest", "translategemma:4b", "translategemma:12b", "translategemma:27b":
		return true
	default:
		return false
	}
}

func validateTranslationProviderLanguageCapability(provider, model, sourceLanguage string) error {
	language, err := normalizeTranslationLanguage(sourceLanguage, false)
	if err != nil {
		return err
	}
	if language != "ko" && language != "fr" {
		return nil
	}
	if !supportsTranslationGemmaLanguagePair(provider, model) {
		return fmt.Errorf("configured translation provider/model is not verified for %s to Spanish; select Ollama TranslateGemma (4B, 12B, or 27B)", translationLanguageName(language))
	}
	return nil
}

func translationLanguageName(language string) string {
	switch language {
	case "en":
		return "English"
	case "ja":
		return "Japanese"
	case "ko":
		return "Korean"
	case "fr":
		return "French"
	default:
		return language
	}
}

type TranslationCue struct {
	ID   string
	Text string
}

type TranslationContext struct {
	NeighboringCues []TranslationContextCue
	Media           TranslationMediaContext
}

type TranslationContextCue struct {
	ID   string
	Text string
}

type TranslationMediaContext struct {
	Title          string
	Year           string
	MovieSeries    string
	Season         string
	Episode        string
	Synopsis       string
	CharacterNames []string
}

type TranslationBatchResult struct {
	ContractVersion string
	Cues            []TranslatedCue
	Metrics         TranslationProviderMetrics
}

type TranslatedCue struct {
	ID   string
	Text string
}

type TranslationProviderMetrics struct {
	Provider      string
	Model         string
	RequestCount  int
	InputTokens   int
	OutputTokens  int
	LatencyMillis int64
}

func validateTranslationBatch(batch TranslationBatch, settings translationSettings) error {
	if batch.ContractVersion != translationProviderContractVersion {
		return fmt.Errorf("unsupported translation contract version")
	}
	if strings.TrimSpace(batch.SourceLanguage) == "" || strings.TrimSpace(batch.TargetLocale) == "" {
		return fmt.Errorf("translation language and locale are required")
	}
	if _, err := normalizeTranslationLanguage(batch.SourceLanguage, false); err != nil {
		return fmt.Errorf("unsupported translation source language: %w", err)
	}
	if _, err := normalizeTranslationTargetLocale(batch.TargetLocale); err != nil {
		return fmt.Errorf("unsupported translation target locale: %w", err)
	}
	if len(batch.Cues) == 0 || len(batch.Cues) > settings.MaxCuesPerRequest {
		return fmt.Errorf("translation batch must contain between 1 and %d cues", settings.MaxCuesPerRequest)
	}
	for _, cue := range batch.Cues {
		if strings.TrimSpace(cue.ID) == "" || strings.TrimSpace(cue.Text) == "" {
			return fmt.Errorf("translation cues require stable IDs and text")
		}
	}
	if batch.Context != nil {
		if len(batch.Context.NeighboringCues) > settings.MaxContextCues {
			return fmt.Errorf("translation context exceeds cue limit")
		}
		contextChars := 0
		for _, cue := range batch.Context.NeighboringCues {
			contextChars += len(cue.ID) + len(cue.Text)
		}
		contextChars += len(batch.Context.Media.Title) + len(batch.Context.Media.Year) + len(batch.Context.Media.MovieSeries) + len(batch.Context.Media.Season) + len(batch.Context.Media.Episode) + len(batch.Context.Media.Synopsis)
		for _, name := range batch.Context.Media.CharacterNames {
			contextChars += len(name)
		}
		if contextChars > settings.MaxContextChars {
			return fmt.Errorf("translation context exceeds character limit")
		}
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("encode translation batch: %w", err)
	}
	if len(encoded) > settings.MaxRequestBytes {
		return fmt.Errorf("translation batch exceeds request size limit")
	}
	return nil
}

type TranslationProviderError struct {
	Class translationProviderErrorClass
	Err   error
}

func (e *TranslationProviderError) Error() string {
	if e == nil || e.Err == nil {
		return string(e.Class)
	}
	return string(e.Class) + ": " + e.Err.Error()
}

func (e *TranslationProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func translationProviderErrorClassOf(err error) translationProviderErrorClass {
	var providerErr *TranslationProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		return providerErr.Class
	}
	if errors.Is(err, context.Canceled) {
		return translationProviderErrorCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return translationProviderErrorTimeout
	}
	return translationProviderErrorMalformed
}
