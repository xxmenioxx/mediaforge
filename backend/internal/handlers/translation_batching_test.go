package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTranslationBatchesForCuesBelowLimitsUseOneBatch(t *testing.T) {
	settings := translationBatchingTestSettings(3, 4096)
	cues := translationBatchingTestCues(2)
	batches, err := translationBatchesForCues("en", "es-419", cues, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0].Cues) != 2 {
		t.Fatalf("batches=%#v, want one two-cue batch", batches)
	}
	assertTranslationBatchesValidAndOrdered(t, cues, batches, settings)
}

func TestTranslationBatchesSplitAtCueCountWithoutReordering(t *testing.T) {
	settings := translationBatchingTestSettings(2, 4096)
	cues := translationBatchingTestCues(5)
	batches, err := translationBatchesForCues("en", "es-419", cues, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 || len(batches[0].Cues) != 2 || len(batches[1].Cues) != 2 || len(batches[2].Cues) != 1 {
		t.Fatalf("unexpected cue-count packing: %#v", batches)
	}
	assertTranslationBatchesValidAndOrdered(t, cues, batches, settings)
}

func TestTranslationBatchesSplitAtEncodedRequestSize(t *testing.T) {
	cues := []TranslationCue{
		{ID: "cue-a", Text: strings.Repeat("a", 40)},
		{ID: "cue-b", Text: "b"},
		{ID: "cue-c", Text: "c"},
	}
	settings := translationBatchingTestSettings(3, encodedTranslationBatchSize(t, TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  "en",
		TargetLocale:    "es-419",
		Cues:            cues[:2],
	})-1)
	batches, err := translationBatchesForCues("en", "es-419", cues, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || len(batches[0].Cues) != 1 || len(batches[1].Cues) != 2 {
		t.Fatalf("unexpected encoded-size packing: %#v", batches)
	}
	assertTranslationBatchesValidAndOrdered(t, cues, batches, settings)
}

func TestTranslationBatchesUseEncodedUnicodeAndIdentitySize(t *testing.T) {
	unicodeCue := TranslationCue{ID: "cue-1", Text: "字幕こんにちは"}
	base := TranslationBatch{ContractVersion: translationProviderContractVersion, SourceLanguage: "en", TargetLocale: "es-419", Cues: []TranslationCue{unicodeCue}}
	settings := translationBatchingTestSettings(2, encodedTranslationBatchSize(t, base))
	if _, err := translationBatchesForCues(base.SourceLanguage, base.TargetLocale, []TranslationCue{unicodeCue}, nil, settings); err != nil {
		t.Fatalf("unicode cue should fit at exact encoded size: %v", err)
	}

	longID := unicodeCue
	longID.ID = strings.Repeat("cue-id-", 20)
	if _, err := translationBatchesForCues(base.SourceLanguage, base.TargetLocale, []TranslationCue{longID}, nil, settings); err == nil {
		t.Fatal("long cue ID exceeded encoded size but was accepted")
	}

	longLanguage := translationBatchingTestSettings(2, encodedTranslationBatchSize(t, base))
	if _, err := translationBatchesForCues(strings.Repeat("en-", 20), base.TargetLocale, []TranslationCue{unicodeCue}, nil, longLanguage); err == nil {
		t.Fatal("long source language exceeded encoded size but was accepted")
	}
}

func TestTranslationBatchesAreDeterministicAndPreserveCueBoundaries(t *testing.T) {
	settings := translationBatchingTestSettings(3, 4096)
	cues := translationBatchingTestCues(8)
	first, err := translationBatchesForCues("ja", "es-ES", cues, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := translationBatchesForCues("ja", "es-ES", cues, nil, settings)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("identical input produced different batches: %s != %s", firstJSON, secondJSON)
	}
	assertTranslationBatchesValidAndOrdered(t, cues, first, settings)
}

func TestTranslationBatchesRejectOversizedSingleCueWithoutPartialResults(t *testing.T) {
	settings := translationBatchingTestSettings(4, 1024)
	cues := []TranslationCue{{ID: "too-large", Text: strings.Repeat("x", 2000)}}
	batches, err := translationBatchesForCues("en", "es-419", cues, nil, settings)
	if err == nil || batches != nil {
		t.Fatalf("oversized cue returned partial/valid batches: batches=%#v err=%v", batches, err)
	}
	if !strings.Contains(err.Error(), "too-large") || !strings.Contains(err.Error(), "max request size") {
		t.Fatalf("oversized cue error was not actionable: %v", err)
	}
}

func TestTranslationBatchesIncludeContextInEncodedSize(t *testing.T) {
	context := &TranslationContext{Media: TranslationMediaContext{Synopsis: strings.Repeat("context", 40)}}
	base := TranslationBatch{ContractVersion: translationProviderContractVersion, SourceLanguage: "en", TargetLocale: "es-419", Context: context, Cues: []TranslationCue{{ID: "cue-1", Text: "Hello"}}}
	settings := translationBatchingTestSettings(2, encodedTranslationBatchSize(t, base)-1)
	if batches, err := translationBatchesForCues("en", "es-419", base.Cues, context, settings); err == nil || batches != nil {
		t.Fatalf("context size was not enforced: batches=%#v err=%v", batches, err)
	}
}

func TestTranslationBatchesForEnvelopesPreserveStableIDs(t *testing.T) {
	envelopes := []translationCueEnvelope{
		{StableID: "cue-1", Text: "one"},
		{StableID: "cue-2", Text: "two"},
	}
	batches, err := translationBatchesForCueEnvelopes("en", "es-419", envelopes, nil, translationBatchingTestSettings(2, 4096))
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || batches[0].Cues[0].ID != "cue-1" || batches[0].Cues[1].ID != "cue-2" {
		t.Fatalf("stable envelope IDs were not preserved: %#v", batches)
	}
}

func translationBatchingTestSettings(maxCues, maxBytes int) translationSettings {
	settings := defaultTranslationSettings()
	settings.MaxCuesPerRequest = maxCues
	settings.MaxRequestBytes = maxBytes
	return settings
}

func translationBatchingTestCues(count int) []TranslationCue {
	cues := make([]TranslationCue, 0, count)
	for index := 0; index < count; index++ {
		cues = append(cues, TranslationCue{ID: "cue-" + string(rune('a'+index)), Text: "text-" + string(rune('a'+index))})
	}
	return cues
}

func encodedTranslationBatchSize(t *testing.T, batch TranslationBatch) int {
	t.Helper()
	encoded, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	return len(encoded)
}

func assertTranslationBatchesValidAndOrdered(t *testing.T, cues []TranslationCue, batches []TranslationBatch, settings translationSettings) {
	t.Helper()
	if err := validateTranslationBatchOrder(cues, batches); err != nil {
		t.Fatal(err)
	}
	for index, batch := range batches {
		if err := validateTranslationBatch(batch, settings); err != nil {
			t.Fatalf("batch %d failed existing validation: %v", index, err)
		}
	}
}
