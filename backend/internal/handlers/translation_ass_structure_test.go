package handlers

import (
	"strings"
	"testing"
)

func TestTranslationASSStructureRoundTripPreservesDocumentAndProtectedDialogue(t *testing.T) {
	source := `[Script Info]
Title: unchanged

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Comment: 0,0:00:00.00,0:00:01.00,Default,,,0,0,0,,not dialogue
Dialogue: 2,0:00:01.00,0:00:03.00,Default,Actor,010,020,030,fx,{\an8\pos(100,200)\k20}Hello, comma\Nworld{\i1}!
Dialogue: 0,0:00:04.00,0:00:05.00,Default,,,,,,{\p1}m 0 0 l 100 100{\p0}
`
	document, err := parseTranslationASSDocument("ass", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Events) != 2 || document.Events[0].SourceOrdinal != 0 || document.Events[1].SourceOrdinal != 1 {
		t.Fatalf("unexpected event ordinals: %#v", document.Events)
	}
	if got, err := document.ReconstructDialogueSegments(map[int][]string{
		0: document.Events[0].Dialogue.OriginalTextSegments(),
		1: document.Events[1].Dialogue.OriginalTextSegments(),
	}); err != nil || got != source {
		t.Fatalf("no-op reconstruction changed source: err=%v got=%q", err, got)
	}

	text := document.Events[0].Dialogue.TranslatableText()
	if text != "Hello, commaworld!" {
		t.Fatalf("unexpected provider text=%q", text)
	}
	for _, protected := range []string{"{\\an8\\pos(100,200)}", "\\N", "{\\i1}"} {
		if strings.Contains(text, protected) {
			t.Fatalf("protected token entered provider text: %q", protected)
		}
	}
	if got, err := document.ReconstructDialogueSegments(map[int][]string{0: {"Hola, coma", "mundo", "!"}}); err != nil || !strings.Contains(got, "{\\an8\\pos(100,200)\\k20}Hola, coma\\Nmundo{\\i1}!") {
		t.Fatalf("protected reconstruction failed: err=%v got=%q", err, got)
	}
	if document.Events[1].Dialogue.TranslatableText() != "" {
		t.Fatal("drawing payload was treated as translatable text")
	}
}

func TestTranslationSSAParsingUsesActiveFormatAndPreservesCommas(t *testing.T) {
	source := `[Script Info]
ScriptType: v4.00+

[Events]
Format: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: Marked=0,0:00:01.00,0:00:02.00,Default,Speaker,0,0,0,,Hello, SSA, comma
`
	document, err := parseTranslationASSDocument("ssa", source)
	if err != nil {
		t.Fatal(err)
	}
	if document.Representation != "ssa" || len(document.Events) != 1 || document.Events[0].Dialogue.TranslatableText() != "Hello, SSA, comma" {
		t.Fatalf("unexpected SSA parse: %#v", document)
	}
	if got, err := document.ReconstructDialogueSegments(map[int][]string{0: document.Events[0].Dialogue.OriginalTextSegments()}); err != nil || got != source {
		t.Fatalf("SSA no-op reconstruction changed source: err=%v got=%q", err, got)
	}
}

func TestTranslationASSSourceCuesFeedNeutralEnvelopeWithoutProviderStructure(t *testing.T) {
	source := "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\pos(10,20)}Hello\n"
	document, err := parseTranslationASSDocument("ass", source)
	if err != nil {
		t.Fatal(err)
	}
	cues := document.TranslationSourceCues("asset.mkv#stream=3", "transform-1")
	if len(cues) != 1 || cues[0].SourceOrdinal != 0 || cues[0].Text != "Hello" || cues[0].Start != 1 || cues[0].End != 2 {
		t.Fatalf("unexpected neutral source cue: %#v", cues)
	}
	envelopes := translationCueEnvelopesForSourceCues("asset.mkv#stream=3", "ass", "transform-1", cues)
	batch := translationBatchForCueEnvelopes("eng", "es-419", envelopes, nil)
	if len(batch.Cues) != 1 || batch.Cues[0].ID != envelopes[0].StableID || batch.Cues[0].Text != "Hello" || batch.Context != nil {
		t.Fatalf("unexpected provider batch: %#v", batch)
	}
}

func TestTranslationASSMalformedStructureFailsClosed(t *testing.T) {
	for name, source := range map[string]string{
		"missing format":     "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,Hello\n",
		"text not last":      "[Events]\nFormat: Layer, Text, Start, End\nDialogue: 0,Hello,0:00:01.00,0:00:02.00\n",
		"short event":        "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,Default\n",
		"bad timing":         "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,not-time,0:00:02.00,Default,Hello\n",
		"unterminated block": "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\pos(1,2)Hello\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseTranslationASSDocument("ass", source); err == nil {
				t.Fatal("unsafe ASS structure was accepted")
			}
		})
	}
}
