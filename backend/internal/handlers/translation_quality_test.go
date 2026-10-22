package handlers

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeSubtitleTranslationQualitySignals(t *testing.T) {
	tests := []struct {
		name       string
		source     []translationSourceCue
		translated []TranslatedCue
		wantState  string
		want       []string
	}{
		{
			name:       "empty target is reviewable",
			source:     []translationSourceCue{{SourceOrdinal: 0, Text: "Hello there"}},
			translated: []TranslatedCue{{ID: "cue-1", Text: " "}},
			wantState:  "review_required", want: []string{"empty_translation"},
		},
		{
			name:       "short output and missing number are warnings",
			source:     []translationSourceCue{{SourceOrdinal: 0, Text: "The train arrives at platform 12 in the morning."}},
			translated: []TranslatedCue{{ID: "cue-2", Text: "Llega."}},
			wantState:  "review_required", want: []string{"unusually_short_translation", "number_not_preserved"},
		},
		{
			name:       "duplicate unrelated translation is flagged",
			source:     []translationSourceCue{{Text: "Good morning"}, {Text: "Good evening"}},
			translated: []TranslatedCue{{ID: "cue-1", Text: "Hola"}, {ID: "cue-2", Text: "Hola"}},
			wantState:  "review_required", want: []string{"duplicate_translation_for_distinct_source"},
		},
		{
			name:       "clean translation remains clean",
			source:     []translationSourceCue{{Text: "Hello"}},
			translated: []TranslatedCue{{ID: "cue-1", Text: "Hola"}},
			wantState:  "clean",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelopes := make([]translationCueEnvelope, len(tt.source))
			for index, cue := range tt.source {
				id := tt.translated[index].ID
				envelopes[index] = translationCueEnvelope{StableID: id, SourceOrdinal: cue.SourceOrdinal}
				if envelopes[index].SourceOrdinal == 0 && index > 0 {
					envelopes[index].SourceOrdinal = index
				}
			}
			got := analyzeSubtitleTranslationQuality(tt.source, envelopes, tt.translated)
			if got.State != tt.wantState {
				t.Fatalf("state = %q, want %q", got.State, tt.wantState)
			}
			var reasons []string
			for _, finding := range got.Findings {
				reasons = append(reasons, finding.Reasons...)
			}
			for _, want := range tt.want {
				found := false
				for _, reason := range reasons {
					if reason == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("finding reasons %v do not contain %q", reasons, want)
				}
			}
			if got.CueCount != len(tt.source) || got.CleanCount+got.ReviewCount != len(tt.source) {
				t.Errorf("counts do not account for cues: %#v", got)
			}
		})
	}
}

func TestApplySubtitleTranslationQualityTextPreservesSRTCueStructure(t *testing.T) {
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n\n2\n00:00:03,000 --> 00:00:04,000\nAdios\n")
	finding := subtitleTranslationQualityFinding{CueID: "cue-2", SourceOrdinal: 1}
	updated, err := applySubtitleTranslationQualityText(translationRepresentationSRT, subtitleTranslationSourceResponse{}, content, finding, "Hasta luego")
	if err != nil {
		t.Fatal(err)
	}
	cues := parseSRTBitmapSubtitleCues(string(updated))
	if len(cues) != 2 || cues[0].SourceOrdinal != 0 || cues[1].SourceOrdinal != 1 || cues[0].Text != "Hola" || cues[1].Text != "Hasta luego" || cues[1].Start != 3 || cues[1].End != 4 {
		t.Fatalf("edit changed cue structure: %#v", cues)
	}
}

func TestApplySubtitleTranslationQualityTextPreservesMultilineSRTAndRejectsInjectedTiming(t *testing.T) {
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n\n2\n00:00:03,000 --> 00:00:04,000\nAdios\n")
	finding := subtitleTranslationQualityFinding{CueID: "cue-1", SourceOrdinal: 0}

	updated, err := applySubtitleTranslationQualityText(translationRepresentationSRT, subtitleTranslationSourceResponse{}, content, finding, "Hasta luego\nllega mañana")
	if err != nil {
		t.Fatal(err)
	}
	cues := parseSRTBitmapSubtitleCues(string(updated))
	if len(cues) != 2 || cues[0].Start != 1 || cues[0].End != 2 || cues[0].Text != "Hasta luego\nllega mañana" ||
		cues[1].Start != 3 || cues[1].End != 4 || cues[1].Text != "Adios" {
		t.Fatalf("multiline edit changed SRT structure or unrelated text: %#v", cues)
	}

	injected := "Hasta luego\n\n3\n00:00:05,000 --> 00:00:06,000\nInjected cue"
	if output, err := applySubtitleTranslationQualityText(translationRepresentationSRT, subtitleTranslationSourceResponse{}, content, finding, injected); err == nil {
		t.Fatalf("timestamp injection was accepted: %q", output)
	}
}

func TestApplySubtitleTranslationQualityTextPreservesASSCueStructure(t *testing.T) {
	content := []byte("[Script Info]\nTitle: Quality edit fixture\nPlayResX: 1280\nPlayResY: 720\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Custom,Arial,42,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,0,1,1,2,0,8,20,30,40,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 2,0:00:01.25,0:00:03.50,Custom,Speaker,11,12,13,Effect,{\\an8}Hola\\Nsegunda línea{\\pos(100,200)}\nDialogue: 0,0:00:04.00,0:00:05.00,Default,,0,0,0,,Otro\n")
	sourceIdentity := "external:/media/movie.ass"
	before, err := parseTranslationASSDocument(translationRepresentationASS, string(content))
	if err != nil {
		t.Fatal(err)
	}
	beforeCues := before.TranslationSourceCues(sourceIdentity, "")
	if len(beforeCues) != 2 {
		t.Fatalf("unexpected ASS fixture cues: %#v", beforeCues)
	}
	beforeEnvelopes := translationCueEnvelopesForSourceCues(sourceIdentity, translationRepresentationASS, "", beforeCues)
	finding := subtitleTranslationQualityFinding{CueID: beforeEnvelopes[0].StableID, SourceOrdinal: 0}
	editText := strings.Replace(beforeCues[0].Text, "Hola", "Buenos días", 1)
	updated, err := applySubtitleTranslationQualityText(translationRepresentationASS, subtitleTranslationSourceResponse{SourceIdentity: sourceIdentity}, content, finding, editText)
	if err != nil {
		t.Fatal(err)
	}
	output := string(updated)
	for _, want := range []string{
		"Style: Custom,Arial,42,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,0,1,1,2,0,8,20,30,40,1",
		"Dialogue: 2,0:00:01.25,0:00:03.50,Custom,Speaker,11,12,13,Effect,{\\an8}Buenos días\\Nsegunda línea{\\pos(100,200)}",
		"Dialogue: 0,0:00:04.00,0:00:05.00,Default,,0,0,0,,Otro",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("ASS edit lost protected structure %q in output:\n%s", want, output)
		}
	}
	after, err := parseTranslationASSDocument(translationRepresentationASS, output)
	if err != nil {
		t.Fatal(err)
	}
	afterCues := after.TranslationSourceCues(sourceIdentity, "")
	if len(beforeCues) != 2 || len(afterCues) != 2 {
		t.Fatalf("ASS edit changed cue count: before=%#v after=%#v", beforeCues, afterCues)
	}
	afterEnvelopes := translationCueEnvelopesForSourceCues(sourceIdentity, translationRepresentationASS, "", afterCues)
	for i := range beforeCues {
		if beforeCues[i].SourceOrdinal != afterCues[i].SourceOrdinal || beforeCues[i].Start != afterCues[i].Start || beforeCues[i].End != afterCues[i].End {
			t.Fatalf("ASS edit changed cue identity or timestamps at %d: before=%#v after=%#v", i, beforeCues[i], afterCues[i])
		}
		if beforeEnvelopes[i].StableID != afterEnvelopes[i].StableID {
			t.Fatalf("ASS edit changed stable cue ID at %d: before=%q after=%q", i, beforeEnvelopes[i].StableID, afterEnvelopes[i].StableID)
		}
	}
	if afterCues[0].Text != editText || afterCues[1].Text != beforeCues[1].Text {
		t.Fatalf("ASS edit changed the wrong cue text: before=%#v after=%#v", beforeCues, afterCues)
	}
}

func TestSubtitleTranslationQualityReviewerNeverSelfReviews(t *testing.T) {
	result := requestSubtitleTranslationQualitySecondOpinion(nil, "http://ollama.invalid", "same-model", "same-model", "es-419", nil)
	if result.State != "unavailable" || result.Diagnostic == "" || len(result.Findings) != 0 {
		t.Fatalf("same translator model was not rejected: %#v", result)
	}
}

func TestAnalyzeSubtitleTranslationQualityFailsClosedOnMismatchAndBoundsFindings(t *testing.T) {
	if got := analyzeSubtitleTranslationQuality([]translationSourceCue{{SourceOrdinal: 0, Text: "a"}}, nil, nil); got.State != "not_analyzed" {
		t.Fatalf("mismatched cue count state = %q", got.State)
	}
	source := make([]translationSourceCue, subtitleTranslationQualityMaxFindings+3)
	envelopes := make([]translationCueEnvelope, len(source))
	translated := make([]TranslatedCue, len(source))
	for i := range source {
		source[i].SourceOrdinal, source[i].Text = i, "source"
		envelopes[i] = translationCueEnvelope{StableID: string(rune('a' + i)), SourceOrdinal: i}
		translated[i] = TranslatedCue{ID: envelopes[i].StableID, Text: ""}
	}
	got := analyzeSubtitleTranslationQuality(source, envelopes, translated)
	if len(got.Findings) != subtitleTranslationQualityMaxFindings || !got.Truncated {
		t.Fatalf("finding bound not enforced: %d, truncated=%v", len(got.Findings), got.Truncated)
	}
	want := []string{"empty_translation"}
	if !reflect.DeepEqual(got.Findings[0].Reasons, want) {
		t.Fatalf("unexpected reasons: %v", got.Findings[0].Reasons)
	}
}
