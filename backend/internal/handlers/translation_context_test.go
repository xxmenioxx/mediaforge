package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTranslationContextNeighborsUseCanonicalOutsideCueOrder(t *testing.T) {
	cues := translationBatchingTestCues(5)
	settings := translationBatchingTestSettings(2, 64*1024)
	settings.MaxContextCues = 2
	settings.MaxContextChars = 4096
	batches, metadata, err := translationBatchesForCuesWithMediaContext("en", "es-419", cues, translationMediaContextInput{}, settings)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Status != translationContextUnavailable || metadata.Source != "" {
		t.Fatalf("missing context status was not informational: %#v", metadata)
	}
	if len(batches) != 3 {
		t.Fatalf("unexpected batches: %#v", batches)
	}
	assertTranslationContextIDs(t, batches[0], []string{"cue-c", "cue-d"})
	assertTranslationContextIDs(t, batches[1], []string{"cue-b", "cue-e"})
	assertTranslationContextIDs(t, batches[2], []string{"cue-c", "cue-d"})
	for _, batch := range batches {
		for _, contextCue := range batch.Context.NeighboringCues {
			for _, targetCue := range batch.Cues {
				if contextCue.ID == targetCue.ID {
					t.Fatalf("target cue %q was included in its own context", contextCue.ID)
				}
			}
		}
	}
	assertTranslationBatchesValidAndOrdered(t, cues, batches, settings)
}

func TestTranslationContextIsDeterministicAndRespectsCueLimit(t *testing.T) {
	cues := translationBatchingTestCues(6)
	settings := translationBatchingTestSettings(3, 64*1024)
	settings.MaxContextCues = 1
	first, metadata, err := translationBatchesForCuesWithMediaContext("ja", "es-ES", cues, translationMediaContextInput{Source: "library metadata"}, settings)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Status != translationContextUnavailable || metadata.Source != "library metadata" {
		t.Fatalf("context source/status was not preserved: %#v", metadata)
	}
	second, _, err := translationBatchesForCuesWithMediaContext("ja", "es-ES", cues, translationMediaContextInput{Source: "library metadata"}, settings)
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
		t.Fatalf("identical context input was not deterministic: %s != %s", firstJSON, secondJSON)
	}
	for _, batch := range first {
		if len(batch.Context.NeighboringCues) > settings.MaxContextCues {
			t.Fatalf("context cue limit exceeded: %#v", batch.Context)
		}
		if batch.SourceLanguage != "ja" || batch.TargetLocale != "es-ES" {
			t.Fatalf("context changed language/locale: %#v", batch)
		}
	}
}

func TestTranslationMediaContextNormalizesFieldsAndPreservesCharacterOrder(t *testing.T) {
	settings := translationBatchingTestSettings(2, 64*1024)
	settings.MaxContextChars = 4096
	input := translationMediaContextInput{
		Source: "  embedded metadata  ",
		Media: TranslationMediaContext{
			Title:          "  Title  ",
			Year:           " 2026 ",
			MovieSeries:    " Series ",
			Season:         " 2 ",
			Episode:        " 4 ",
			Synopsis:       "  A bounded synopsis.  ",
			CharacterNames: []string{"  Ana ", "Bruno", "   ", " Carlos  "},
		},
	}
	batches, metadata, err := translationBatchesForCuesWithMediaContext("en", "es-419", translationBatchingTestCues(1), input, settings)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Status != translationContextAvailable || metadata.Source != "embedded metadata" {
		t.Fatalf("media context status/source mismatch: %#v", metadata)
	}
	media := batches[0].Context.Media
	if media.Title != "Title" || media.Year != "2026" || media.MovieSeries != "Series" || media.Season != "2" || media.Episode != "4" || media.Synopsis != "A bounded synopsis." {
		t.Fatalf("media context fields were not normalized: %#v", media)
	}
	if strings.Join(media.CharacterNames, ",") != "Ana,Bruno,Carlos" {
		t.Fatalf("character order/content changed: %#v", media.CharacterNames)
	}
	if translationMediaContextSize(media) > settings.MaxContextChars {
		t.Fatalf("media context exceeds configured bound: %#v", media)
	}
}

func TestTranslationMediaContextBoundsSynopsisAndRejectsRequiredOverflow(t *testing.T) {
	settings := translationBatchingTestSettings(2, 64*1024)
	settings.MaxContextChars = 24
	input := translationMediaContextInput{Media: TranslationMediaContext{Title: "Title", Synopsis: "これは非常に長い説明です"}}
	batches, _, err := translationBatchesForCuesWithMediaContext("en", "es-419", translationBatchingTestCues(1), input, settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := translationMediaContextSize(batches[0].Context.Media); got > settings.MaxContextChars {
		t.Fatalf("bounded synopsis exceeds context limit: %d", got)
	}
	if !strings.HasPrefix(batches[0].Context.Media.Synopsis, "これは") {
		t.Fatalf("bounded synopsis did not retain a valid prefix: %q", batches[0].Context.Media.Synopsis)
	}
	if !strings.HasPrefix(batches[0].Context.Media.Synopsis, string([]rune(batches[0].Context.Media.Synopsis))) {
		t.Fatal("synopsis was not valid UTF-8")
	}

	settings.MaxContextChars = 4
	if batches, _, err := translationBatchesForCuesWithMediaContext("en", "es-419", translationBatchingTestCues(1), translationMediaContextInput{Media: TranslationMediaContext{Title: "Title"}}, settings); err == nil || batches != nil {
		t.Fatalf("required metadata overflow did not fail closed: batches=%#v err=%v", batches, err)
	}
}

func TestTranslationContextParticipatesInPerBatchSizeAndRecomputesAfterSplit(t *testing.T) {
	cues := translationBatchingTestCues(5)
	plainSettings := translationBatchingTestSettings(3, 64*1024)
	plainBatch, err := translationBatchesForCues("en", "es-419", cues[:3], nil, plainSettings)
	if err != nil || len(plainBatch) != 1 {
		t.Fatalf("plain three-cue batch did not fit: batches=%#v err=%v", plainBatch, err)
	}
	encodedPlain, err := json.Marshal(plainBatch[0])
	if err != nil {
		t.Fatal(err)
	}
	settings := plainSettings
	settings.MaxRequestBytes = len(encodedPlain) + 230
	settings.MaxContextCues = 2
	settings.MaxContextChars = 4096
	input := translationMediaContextInput{Media: TranslationMediaContext{Title: strings.Repeat("M", 80)}}
	batches, _, err := translationBatchesForCuesWithMediaContext("en", "es-419", cues, input, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) <= 1 {
		t.Fatalf("context did not participate in encoded-size packing: %#v", batches)
	}
	start := 0
	normalized, err := normalizeTranslationMediaContext(input, settings)
	if err != nil {
		t.Fatal(err)
	}
	for index, batch := range batches {
		end := start + len(batch.Cues)
		expectedContext, err := boundedTranslationContextForRange(start, end, cues, normalized.Media, "en", "es-419", settings)
		if err != nil {
			t.Fatal(err)
		}
		expectedJSON, err := json.Marshal(expectedContext)
		if err != nil {
			t.Fatal(err)
		}
		actualJSON, err := json.Marshal(batch.Context)
		if err != nil {
			t.Fatal(err)
		}
		if string(actualJSON) != string(expectedJSON) {
			t.Fatalf("batch %d context was not recomputed for range %d:%d: actual=%s expected=%s", index, start, end, actualJSON, expectedJSON)
		}
		start = end
	}
	if start != len(cues) {
		t.Fatalf("batch ranges did not cover all cues: %d/%d", start, len(cues))
	}
	assertTranslationBatchesValidAndOrdered(t, cues, batches, settings)
}

func TestTranslationContextDoesNotCreatePartialResultsForOversizedTarget(t *testing.T) {
	settings := translationBatchingTestSettings(2, 1024)
	settings.MaxContextCues = 2
	settings.MaxContextChars = 4096
	cues := []TranslationCue{{ID: "too-large", Text: strings.Repeat("x", 2000)}}
	batches, _, err := translationBatchesForCuesWithMediaContext("ja", "es-419", cues, translationMediaContextInput{Media: TranslationMediaContext{Title: "Title"}}, settings)
	if err == nil || batches != nil {
		t.Fatalf("oversized target returned partial context batches: batches=%#v err=%v", batches, err)
	}
}

func assertTranslationContextIDs(t *testing.T, batch TranslationBatch, want []string) {
	t.Helper()
	if batch.Context == nil {
		t.Fatalf("batch has no context: %#v", batch)
	}
	got := make([]string, 0, len(batch.Context.NeighboringCues))
	for _, cue := range batch.Context.NeighboringCues {
		got = append(got, cue.ID)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("context IDs=%v, want %v", got, want)
	}
}
