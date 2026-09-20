package handlers

import (
	"strings"
	"testing"
)

func TestResolveTranslationOutputRepresentation(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		request string
		want    string
		invalid bool
	}{
		{name: "srt default", source: "srt", want: "srt"},
		{name: "srt explicit", source: "srt", request: "srt", want: "srt"},
		{name: "ass", source: "ass", want: "ass"},
		{name: "ssa", source: "ssa", want: "ssa"},
		{name: "bitmap default", source: "bitmap", want: "srt"},
		{name: "bitmap srt", source: "bitmap", request: "srt", want: "srt"},
		{name: "bitmap ass", source: "bitmap", request: "ass", want: "ass"},
		{name: "bitmap ssa rejected", source: "bitmap", request: "ssa", invalid: true},
		{name: "ass to srt rejected", source: "ass", request: "srt", invalid: true},
		{name: "ssa to ass rejected", source: "ssa", request: "ass", invalid: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveTranslationOutputRepresentation(test.source, test.request)
			if test.invalid {
				if err == nil {
					t.Fatalf("unsupported output accepted: %q", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("output=%q err=%v, want %q", got, err, test.want)
			}
		})
	}
}

func TestSerializeTranslatedSRTKeepsTextTimingAndNoGeometry(t *testing.T) {
	source := parseSRTBitmapSubtitleCues("1\n00:00:01,000 --> 00:00:02,000\nHello\n")
	source[0].Geometry = &bitmapCueGeometry{BitmapX: 10, BitmapY: 20, BitmapWidth: 300, BitmapHeight: 100}
	before := source[0]
	output, err := serializeTranslatedTextCues("srt", source, map[int]string{0: "Hola, mundo!"}, "")
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if !strings.Contains(text, "00:00:01,000 --> 00:00:02,000") || !strings.Contains(text, "Hola, mundo!") || strings.Contains(text, "\\an") || strings.Contains(text, "\\pos") {
		t.Fatalf("unexpected SRT output: %q", text)
	}
	if source[0].Text != before.Text || source[0].Geometry != before.Geometry {
		t.Fatal("SRT serialization mutated source cue")
	}
}

func TestSerializeTranslatedASSAndSSAKeepProtectedRepresentation(t *testing.T) {
	for _, representation := range []string{"ass", "ssa"} {
		t.Run(representation, func(t *testing.T) {
			source := "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)}Hello\\Nworld\n"
			document, err := parseTranslationASSDocument(representation, source)
			if err != nil {
				t.Fatal(err)
			}
			providerText := strings.Replace(document.Events[0].Dialogue.TranslatableText(), "Hello", "Hola", 1)
			output, err := serializeTranslatedASSDocument(document, map[int]string{0: providerText}, "")
			if err != nil {
				t.Fatal(err)
			}
			text := string(output)
			if !strings.Contains(text, "{\\an8\\pos(10,20)}Hola\\Nworld") || !strings.Contains(text, "Format: Layer, Start, End, Style, Text") {
				t.Fatalf("protected %s output changed structure: %q", representation, text)
			}
		})
	}
}

func TestSerializeTranslatedBitmapSelectsSRTOrASSWithoutMutatingOCRCues(t *testing.T) {
	geometry := &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 600, BitmapY: 80, BitmapWidth: 700, BitmapHeight: 120}
	source := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "OCR source", SourceOrdinal: 3, Geometry: geometry}}
	srt, err := serializeTranslatedBitmapCues(source, map[int]string{3: "Texto traducido"}, "")
	if err != nil {
		t.Fatal(err)
	}
	ass, err := serializeTranslatedBitmapCues(source, map[int]string{3: "Texto traducido"}, "ass")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(srt), "\\an") || strings.Contains(string(srt), "\\pos") || !strings.Contains(string(srt), "Texto traducido") {
		t.Fatalf("bitmap SRT contains geometry or missed text: %q", srt)
	}
	if !strings.Contains(string(ass), "Dialogue:") || !strings.Contains(string(ass), "\\an") || !strings.Contains(string(ass), "Texto traducido") {
		t.Fatalf("bitmap ASS did not use geometry-aware serializer: %q", ass)
	}
	if source[0].Text != "OCR source" || source[0].Geometry != geometry {
		t.Fatal("bitmap serialization mutated canonical OCR cue")
	}
	if _, err := serializeTranslatedBitmapCues(source, map[int]string{3: "Texto traducido"}, "ssa"); err == nil {
		t.Fatal("bitmap SSA output accepted")
	}
}
