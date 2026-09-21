package handlers

import (
	"context"
	"errors"
	"testing"
)

func TestTranslationProviderContractCarriesOnlyBoundedTextInputs(t *testing.T) {
	batch := TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  "ja",
		TargetLocale:    "es-419",
		Cues:            []TranslationCue{{ID: "cue-1", Text: "こんにちは"}},
		Context: &TranslationContext{
			NeighboringCues: []TranslationContextCue{{ID: "cue-0", Text: "前の台詞"}},
			Media:           TranslationMediaContext{Title: "Example", Year: "2026"},
		},
	}
	if batch.ContractVersion == "" || batch.SourceLanguage != "ja" || batch.TargetLocale != "es-419" || len(batch.Cues) != 1 || batch.Cues[0].ID != "cue-1" || batch.Cues[0].Text != "こんにちは" || batch.Context == nil {
		t.Fatalf("unexpected translation batch: %#v", batch)
	}

	result := TranslationBatchResult{
		ContractVersion: translationProviderContractVersion,
		Cues:            []TranslatedCue{{ID: "cue-1", Text: "Hola"}},
		Metrics:         TranslationProviderMetrics{Provider: translationProviderOllama, Model: "qwen2.5:7b", RequestCount: 1},
	}
	if result.Cues[0].ID != batch.Cues[0].ID || result.Cues[0].Text == "" || result.Metrics.Provider == "" {
		t.Fatalf("unexpected translation result: %#v", result)
	}
	if err := validateTranslationBatch(batch, defaultTranslationSettings()); err != nil {
		t.Fatalf("valid bounded batch rejected: %v", err)
	}
	batch.Context = nil
	if err := validateTranslationBatch(batch, defaultTranslationSettings()); err != nil {
		t.Fatalf("nil context rejected: %v", err)
	}
	if batch.Context != nil {
		t.Fatal("nil context unexpectedly populated")
	}
	batch.Context = &TranslationContext{}
	batch.Context.NeighboringCues = make([]TranslationContextCue, defaultTranslationSettings().MaxContextCues+1)
	if err := validateTranslationBatch(batch, defaultTranslationSettings()); err == nil {
		t.Fatal("oversized context accepted")
	}
}

func TestTranslationProviderErrorClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want translationProviderErrorClass
	}{
		{name: "configuration", err: &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: errors.New("missing model")}, want: translationProviderErrorConfiguration},
		{name: "transport", err: &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: errors.New("connection refused")}, want: translationProviderErrorUnavailable},
		{name: "malformed", err: &TranslationProviderError{Class: translationProviderErrorMalformed, Err: errors.New("invalid json")}, want: translationProviderErrorMalformed},
		{name: "cancelled", err: context.Canceled, want: translationProviderErrorCancelled},
		{name: "timeout", err: context.DeadlineExceeded, want: translationProviderErrorTimeout},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := translationProviderErrorClassOf(test.err); got != test.want {
				t.Fatalf("class=%q, want %q", got, test.want)
			}
		})
	}
}
