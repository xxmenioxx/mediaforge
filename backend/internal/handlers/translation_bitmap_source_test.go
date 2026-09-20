package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cacheBitmapTranslationEvidence(t *testing.T, mediaPath string, stream FFProbeStream, language, mode, effectiveLanguage string, cues []canonicalBitmapSubtitleCue) {
	t.Helper()
	request := canonicalOCRRequestFor(mediaPath, stream, language, mode, 0, 0)
	withRuntime, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	evidence := &canonicalOCREvidence{
		Identity:          withRuntime.Identity,
		RequestedLanguage: request.Language,
		EffectiveLanguage: effectiveLanguage,
		OCRMode:           request.Mode,
		Complete:          true,
		Integrity:         "canonical-cues-validated-v1",
		Cues:              cloneCanonicalCues(cues),
	}
	commitCanonicalOCREvidence(withRuntime, withRuntime.Identity.key(), evidence)
}

func TestBitmapTranslationSourceForPGSUsesCanonicalOCRAndBuildsEnvelopes(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("PGS source"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := installCanonicalRuntimeTools(t, "seconv", "eng spa jpn jpn_vert")
	dictionaryRoot := filepath.Join(root, "dictionary")
	if err := os.MkdirAll(dictionaryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dictionaryRoot, "en.dic"), []byte("9\nhouse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousDictionaryRoot := ocrDictionaryFolderOverride
	ocrDictionaryFolderOverride = dictionaryRoot
	defer func() { ocrDictionaryFolderOverride = previousDictionaryRoot }()
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
case "$output" in
  *.sup) : > "$output" ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'The h0use' > "$output"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "eng"}}
	source, err := bitmapTranslationSourceFor(context.Background(), mediaPath, stream, "eng", "accurate", 0, 0, "source-identity", "pgs", "logical-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if source.EffectiveLanguage != "eng" || source.Evidence == nil || len(source.Envelopes) != 1 {
		t.Fatalf("unexpected PGS translation source: %#v", source)
	}
	if source.Envelopes[0].Text != "The house" {
		t.Fatalf("deterministic canonical OCR correction was not applied before translation: %#v", source.Envelopes[0])
	}
	if source.Envelopes[0].Representation.SourceRepresentation != "pgs" || source.Envelopes[0].Representation.LogicalTransformKey != "logical-1" {
		t.Fatalf("translation envelope lost source identity: %#v", source.Envelopes[0].Representation)
	}
}

func TestBitmapTranslationSourceForVobSubReusesCanonicalEvidenceAndGeometry(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("VobSub source"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := installCanonicalRuntimeTools(t, "seconv", "eng spa jpn jpn_vert")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 2, ID: "vobsub-2", CodecName: "dvd_subtitle", Tags: map[string]string{"language": "jpn"}}
	geometry := &bitmapCueGeometry{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 100, BitmapY: 900, BitmapWidth: 800, BitmapHeight: 100, SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}
	cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "こんにちは", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute, Geometry: geometry}}
	cacheBitmapTranslationEvidence(t, mediaPath, stream, "jpn", "accurate", "jpn_vert", cues)

	first, err := bitmapTranslationSourceFor(context.Background(), mediaPath, stream, "jpn", "accurate", 0, 0, "source-identity", "vobsub", "logical-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bitmapTranslationSourceFor(context.Background(), mediaPath, stream, "jpn", "accurate", 0, 0, "source-identity", "vobsub", "logical-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []*bitmapTranslationSource{first, second} {
		if source.EffectiveLanguage != "jpn_vert" {
			t.Fatalf("canonical effective Japanese language=%q, want jpn_vert", source.EffectiveLanguage)
		}
		if !strings.Contains(source.Evidence.Message, "cache hit") {
			t.Fatalf("translation source did not reuse canonical evidence: %q", source.Evidence.Message)
		}
		if source.Envelopes[0].Geometry == nil || source.Envelopes[0].Geometry.BitmapX != 100 || source.Envelopes[0].Geometry.BitmapY != 900 {
			t.Fatalf("canonical bitmap geometry was not retained: %#v", source.Envelopes[0].Geometry)
		}
	}
	if first.Envelopes[0].Representation.SourceIdentity != "source-identity" {
		t.Fatalf("explicit source identity changed: %q", first.Envelopes[0].Representation.SourceIdentity)
	}
	emptyIdentity, err := bitmapTranslationSourceFor(context.Background(), mediaPath, stream, "jpn", "accurate", 0, 0, "", "vobsub", "logical-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := emptyIdentity.Envelopes[0].Representation.SourceIdentity, emptyIdentity.Evidence.Identity.key(); got != want {
		t.Fatalf("fallback source identity=%q, want final evidence identity %q", got, want)
	}
	if first.Evidence.OCRMode != "accurate" || second.Evidence.OCRMode != "accurate" {
		t.Fatalf("accurate canonical evidence was not reused: %#v %#v", first.Evidence, second.Evidence)
	}
}

func TestBitmapTranslationSourceRejectsUnsupportedCodecBeforeProviderWork(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("unsupported source"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, codec := range []string{"unsupported_bitmap", "dvb_subtitle"} {
		_, err := bitmapTranslationSourceFor(context.Background(), mediaPath, FFProbeStream{Index: 1, CodecName: codec}, "eng", "raw", 0, 0, "source", "bitmap", "logical", nil)
		if err == nil || !strings.Contains(err.Error(), "does not support subtitle codec") {
			t.Fatalf("codec=%q error=%v", codec, err)
		}
	}
}

func TestBitmapTranslationSourceCancellationReturnsNoUsableEnvelopes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := bitmapTranslationSourceFor(ctx, "/missing/movie.mkv", FFProbeStream{Index: 4, CodecName: "dvd_subtitle"}, "eng", "raw", 0, 0, "source", "vobsub", "logical", nil)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancellation error=%v", err)
	}
}
