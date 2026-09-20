package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type bitmapTranslationProviderSpy struct {
	mu       sync.Mutex
	calls    int
	batches  []TranslationBatch
	response func(TranslationBatch) TranslationBatchResult
}

func (p *bitmapTranslationProviderSpy) Translate(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	p.mu.Lock()
	p.calls++
	p.batches = append(p.batches, batch)
	p.mu.Unlock()
	if p.response != nil {
		return p.response(batch), nil
	}
	result := TranslationBatchResult{ContractVersion: batch.ContractVersion, Metrics: TranslationProviderMetrics{Provider: "spy", Model: "test", RequestCount: 1}}
	for _, cue := range batch.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: "Hola " + cue.Text})
	}
	return result, nil
}

func bitmapTranslationExecutionTestSettings() translationSettings {
	settings := defaultTranslationSettings()
	settings.MaxCuesPerRequest = 1
	settings.MaxRequestBytes = 64 * 1024
	return settings
}

func cacheBitmapTranslationExecutionEvidence(t *testing.T, mediaPath string, stream FFProbeStream, language string, cues []canonicalBitmapSubtitleCue) {
	t.Helper()
	cacheBitmapTranslationEvidence(t, mediaPath, stream, language, "accurate", "eng", cues)
}

func TestExecuteBitmapTranslationRunsProviderAfterCanonicalPreparationAndFansOutOnce(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 4, ID: "vob-4", CodecName: "dvd_subtitle"}
	geometry := &bitmapCueGeometry{Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 1200, BitmapY: 800, BitmapWidth: 500, BitmapHeight: 120, SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}
	cues := []canonicalBitmapSubtitleCue{
		{Start: 1, End: 2, Text: "Hello", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute, Geometry: geometry},
		{Start: 3, End: 4, Text: "World", SourceOrdinal: 2, TimingProvenance: originalSourceAbsolute, Geometry: &bitmapCueGeometry{Start: 3, End: 4, CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 100, BitmapY: 80, BitmapWidth: 400, BitmapHeight: 120, SourceOrdinal: 2, TimingProvenance: originalSourceAbsolute}},
	}
	cacheBitmapTranslationExecutionEvidence(t, mediaPath, stream, "eng", cues)
	provider := &bitmapTranslationProviderSpy{}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		SourceIdentity: "source", SourceRepresentation: "vobsub", LogicalTransformKey: "logical",
		TargetLocale: "es-419", RequestedRepresentation: "both", Provider: provider,
		Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || len(result.Outputs) != 2 {
		t.Fatalf("provider calls=%d outputs=%v, want two bounded calls and SRT/ASS", provider.calls, result.Outputs)
	}
	if !strings.Contains(string(result.Outputs[translationRepresentationSRT]), "Hola Hello") || strings.Contains(string(result.Outputs[translationRepresentationSRT]), "\\an") {
		t.Fatalf("translated SRT=%q", result.Outputs[translationRepresentationSRT])
	}
	ass := string(result.Outputs[translationRepresentationASS])
	if !strings.Contains(ass, "Hola Hello") || !strings.Contains(ass, "{\\an") {
		t.Fatalf("translated ASS lost canonical geometry=%q", ass)
	}
	if result.Source == nil || result.Source.EffectiveLanguage != "eng" || result.Metrics.RequestCount != 2 {
		t.Fatalf("unexpected execution result=%#v", result)
	}
	for _, batch := range provider.batches {
		for _, cue := range batch.Cues {
			if strings.Contains(cue.Text, mediaPath) || strings.Contains(cue.Text, "geometry") {
				t.Fatalf("provider saw non-text source data: %#v", cue)
			}
		}
	}
	if cues[0].Text != "Hello" || cues[0].Geometry != geometry {
		t.Fatalf("source canonical cue was mutated: %#v", cues[0])
	}
}

func TestExecuteBitmapTranslationUsesCanonicalEffectiveJapaneseLanguage(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 2, ID: "vob-2", CodecName: "dvd_subtitle"}
	cacheBitmapTranslationEvidence(t, mediaPath, stream, "jpn", "accurate", "jpn_vert", []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "こんにちは", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
	provider := &bitmapTranslationProviderSpy{}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "jpn", OCRMode: "accurate",
		SourceRepresentation: "vobsub", TargetLocale: "es-ES", RequestedRepresentation: "srt",
		Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source.EffectiveLanguage != "jpn_vert" || len(provider.batches) != 1 || provider.batches[0].SourceLanguage != "ja" {
		t.Fatalf("canonical effective language was not normalized for provider: source=%q batches=%#v", result.Source.EffectiveLanguage, provider.batches)
	}
}

func TestExecuteBitmapTranslationFailsClosedBeforeProviderForReviewCue(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 1, ID: "vob-1", CodecName: "dvd_subtitle"}
	cacheBitmapTranslationExecutionEvidence(t, mediaPath, stream, "eng", []canonicalBitmapSubtitleCue{{Start: 0, End: 31, Text: "bad{", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
	provider := &bitmapTranslationProviderSpy{}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err == nil || !strings.Contains(err.Error(), "requires review") || provider.calls != 0 || result.Outputs != nil {
		t.Fatalf("review cue was not gated before provider: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestExecuteBitmapTranslationRejectsMalformedProviderResultWithoutOutputs(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 1, ID: "vob-1", CodecName: "dvd_subtitle"}
	cacheBitmapTranslationExecutionEvidence(t, mediaPath, stream, "eng", []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Hello", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
	provider := &bitmapTranslationProviderSpy{response: func(batch TranslationBatch) TranslationBatchResult {
		return TranslationBatchResult{ContractVersion: batch.ContractVersion, Cues: []TranslatedCue{{ID: "unexpected", Text: "Hola"}}}
	}}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err == nil || provider.calls != 1 || result.Outputs != nil || result.Source != nil {
		t.Fatalf("malformed provider result produced usable output: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestExecuteBitmapTranslationCancellationBeforeAndAfterOCRPreparation(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 1, ID: "vob-1", CodecName: "dvd_subtitle"}
	cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Hello", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}}
	cacheBitmapTranslationExecutionEvidence(t, mediaPath, stream, "eng", cues)

	provider := &bitmapTranslationProviderSpy{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "raw", TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings()})
	if !errors.Is(err, context.Canceled) || provider.calls != 0 || result.Outputs != nil {
		t.Fatalf("pre-OCR cancellation was not preserved: result=%#v calls=%d err=%v", result, provider.calls, err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	result, err = executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate", TargetLocale: "es-419",
		Provider: provider, Settings: bitmapTranslationExecutionTestSettings(), OnProgress: func(_, _ int) { cancel() },
	})
	if !errors.Is(err, context.Canceled) || provider.calls != 0 || result.Outputs != nil {
		t.Fatalf("post-OCR-pre-provider cancellation was not preserved: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestExecuteBitmapTranslationOCRFailureAndZeroRecognitionDoNotCallProvider(t *testing.T) {
	for _, test := range []struct {
		name       string
		seconvBody string
	}{
		{name: "fatal OCR failure", seconvBody: "printf '%s\\n' 'fatal malformed input' >&2\nexit 17"},
		{name: "zero recognition", seconvBody: "printf '%s\\n' 'No subtitles recognised' >&2\nexit 17"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "movie.mkv")
			if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
				t.Fatal(err)
			}
			bin := installCanonicalOCRTestTools(t, "#!/bin/sh\n"+test.seconvBody+"\n")
			writeExecutable(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\ncase \"$output\" in *.sup) : > \"$output\" ;; esac\n")
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			provider := &bitmapTranslationProviderSpy{}
			result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
				MediaPath: mediaPath, Stream: FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle"}, RequestedLanguage: "eng", OCRMode: "raw",
				TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
			})
			if err == nil || provider.calls != 0 || result.Outputs != nil {
				t.Fatalf("OCR failure reached provider or produced output: result=%#v calls=%d err=%v", result, provider.calls, err)
			}
		})
	}
}

func TestExecuteBitmapTranslationCancellationDuringOCRDoesNotCallProvider(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := installCanonicalOCRTestTools(t, "#!/bin/sh\ntouch \"$MVFORGE_TRANSLATION_OCR_STARTED\"\nwhile true; do sleep 1; done\n")
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\ncase \"$output\" in *.sup) : > \"$output\" ;; esac\n")
	started := filepath.Join(root, "ocr-started")
	t.Setenv("MVFORGE_TRANSLATION_OCR_STARTED", started)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	provider := &bitmapTranslationProviderSpy{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
			MediaPath: mediaPath, Stream: FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle"}, RequestedLanguage: "eng", OCRMode: "raw",
			TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
		})
		done <- err
	}()
	waitForFile(t, started)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || provider.calls != 0 {
			t.Fatalf("OCR cancellation was not preserved: calls=%d err=%v", provider.calls, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bitmap translation did not return after OCR cancellation")
	}
}

func TestAggregateTranslationProviderMetrics(t *testing.T) {
	got := aggregateTranslationProviderMetrics(TranslationProviderMetrics{Provider: "ollama", Model: "model"}, TranslationProviderMetrics{Provider: "ollama", Model: "model", RequestCount: 1, InputTokens: 2, OutputTokens: 3, LatencyMillis: 4})
	if got.RequestCount != 1 || got.InputTokens != 2 || got.OutputTokens != 3 || got.LatencyMillis != 4 {
		t.Fatalf("metrics were not aggregated: %#v", got)
	}
	if got.Provider != "ollama" || got.Model != "model" {
		t.Fatalf("provider/model metrics changed: %#v", got)
	}
}
