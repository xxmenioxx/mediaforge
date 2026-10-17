package handlers

import (
	"context"
	"errors"
	"fmt"
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

func setupSelectiveBitmapTranslationFixture(t *testing.T, originalText, candidate string) (string, FFProbeStream, []canonicalBitmapSubtitleCue, string) {
	t.Helper()
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := installCanonicalOCRTestTools(t, fmt.Sprintf(`#!/bin/sh
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '1\n00:00:01,000 --> 00:00:02,000\n%%s\n' '%s' > "$output"
printf 'selective\n' >> "$SELECTIVE_OCR_LOG"
`, candidate))
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
printf 'bounded subtitle input' > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "ffprobe"), `#!/bin/sh
printf '%s' '{"streams":[{"index":0,"id":"0x1","codec_type":"subtitle","codec_name":"dvd_subtitle"}]}'
`)
	logPath := filepath.Join(root, "selective.log")
	t.Setenv("SELECTIVE_OCR_LOG", logPath)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 1, ID: "vob-1", CodecName: "dvd_subtitle"}
	cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: originalText, SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}}
	cacheBitmapTranslationExecutionEvidence(t, mediaPath, stream, "eng", cues)
	return mediaPath, stream, cues, logPath
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
	var batchProgress [][2]int
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		SourceIdentity: "source", SourceRepresentation: "vobsub", LogicalTransformKey: "logical",
		TargetLocale: "es-419", RequestedRepresentation: "both", Provider: provider,
		Settings: bitmapTranslationExecutionTestSettings(),
		OnBatch:  func(processed, total int) { batchProgress = append(batchProgress, [2]int{processed, total}) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || len(result.Outputs) != 2 {
		t.Fatalf("provider calls=%d outputs=%v, want two bounded calls and SRT/ASS", provider.calls, result.Outputs)
	}
	if len(batchProgress) != 2 || batchProgress[0] != [2]int{1, 2} || batchProgress[1] != [2]int{2, 2} {
		t.Fatalf("unexpected bitmap translation batch progress: %#v", batchProgress)
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
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "ja", OCRLanguage: "jpn", OCRMode: "accurate",
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

func TestExecuteBitmapTranslationPreservesRequestedLanguageIntent(t *testing.T) {
	type testCase struct {
		name              string
		requestedLanguage string
		ocrLanguage       string
		metadata          string
		cachedLanguage    string
		effectiveOCR      string
		text              string
		wantSource        string
		wantError         bool
	}
	cases := []testCase{
		{name: "auto english metadata", requestedLanguage: "auto", ocrLanguage: "eng", metadata: "eng", cachedLanguage: "eng", effectiveOCR: "eng", text: "Hello world", wantSource: "en"},
		{name: "auto japanese metadata", requestedLanguage: "auto", ocrLanguage: "jpn", metadata: "jpn", cachedLanguage: "jpn", effectiveOCR: "jpn", text: "こんにちは", wantSource: "ja"},
		{name: "auto japanese cue evidence", requestedLanguage: "auto", ocrLanguage: "eng", metadata: "und", cachedLanguage: "eng", effectiveOCR: "eng", text: "こんにちは 世界", wantSource: "ja"},
		{name: "auto ambiguous evidence", requestedLanguage: "auto", ocrLanguage: "eng", metadata: "und", cachedLanguage: "eng", effectiveOCR: "eng", text: "123 !!!", wantError: true},
		{name: "explicit english", requestedLanguage: "en", ocrLanguage: "eng", metadata: "und", cachedLanguage: "eng", effectiveOCR: "eng", text: "Hello world", wantSource: "en"},
		{name: "explicit japanese", requestedLanguage: "ja", ocrLanguage: "jpn", metadata: "und", cachedLanguage: "jpn", effectiveOCR: "jpn", text: "こんにちは", wantSource: "ja"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "movie.mkv")
			if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			stream := FFProbeStream{Index: 2, ID: "bitmap-2", CodecName: "dvd_subtitle", Tags: map[string]string{"language": testCase.metadata}}
			cacheBitmapTranslationEvidence(t, mediaPath, stream, testCase.cachedLanguage, "accurate", testCase.effectiveOCR, []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: testCase.text, SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
			provider := &bitmapTranslationProviderSpy{}
			result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
				MediaPath: mediaPath, Stream: stream, RequestedLanguage: testCase.requestedLanguage, OCRLanguage: testCase.ocrLanguage,
				SourceLanguageMetadata: []string{testCase.metadata}, OCRMode: "accurate", SourceRepresentation: "vobsub",
				TargetLocale: "es-419", RequestedRepresentation: "srt", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
			})
			if testCase.wantError {
				if err == nil || provider.calls != 0 || result.Outputs != nil {
					t.Fatalf("ambiguous Auto result=%#v err=%v providerCalls=%d, want fail closed before provider", result, err, provider.calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.batches) != 1 || provider.batches[0].SourceLanguage != testCase.wantSource {
				t.Fatalf("provider source language=%q batches=%#v, want %q", provider.batches[0].SourceLanguage, provider.batches, testCase.wantSource)
			}
		})
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
	if err == nil || !strings.Contains(err.Error(), "requires review") || subtitleTranslationFailureClassOf(err) != subtitleTranslationFailureOCRReviewRequired || provider.calls != 0 || result.Outputs != nil {
		t.Fatalf("review cue was not gated before provider: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestExecuteBitmapTranslationRunsAcceptedSelectiveOCRBeforeProvider(t *testing.T) {
	mediaPath, stream, original, logPath := setupSelectiveBitmapTranslationFixture(t, "The h�use", "The house")
	provider := &bitmapTranslationProviderSpy{}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		TargetLocale: "es-419", RequestedRepresentation: "srt", Provider: provider,
		Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || result.Source == nil || len(result.Outputs) != 1 {
		t.Fatalf("accepted selective correction did not reach provider/output: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
	if result.Source.Evidence.Cues[0].Text != "The house" || provider.batches[0].Cues[0].Text != "The house" {
		t.Fatalf("corrected cue did not reach provider: source=%#v batch=%#v", result.Source.Evidence.Cues, provider.batches[0].Cues)
	}
	if original[0].Text != "The h�use" {
		t.Fatalf("canonical source evidence was mutated: %#v", original)
	}
	log, err := os.ReadFile(logPath)
	if err != nil || strings.Count(string(log), "selective") != 3 {
		t.Fatalf("selective correction did not run exactly once without full OCR: log=%q err=%v", log, err)
	}
}

func TestExecuteBitmapTranslationRejectsUnacceptableSelectiveCandidate(t *testing.T) {
	mediaPath, stream, _, logPath := setupSelectiveBitmapTranslationFixture(t, "The h�use", "The h�use")
	provider := &bitmapTranslationProviderSpy{}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err == nil || !strings.Contains(err.Error(), "requires review") || subtitleTranslationFailureClassOf(err) != subtitleTranslationFailureOCRReviewRequired || provider.calls != 0 || result.Outputs != nil {
		t.Fatalf("unacceptable selective candidate was accepted: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil || strings.Count(string(log), "selective") != 3 {
		t.Fatalf("selective OCR did not succeed before candidate safety rejection: log=%q err=%v", log, readErr)
	}
}

func TestExecuteBitmapTranslationRejectsSelectiveCandidateBelowCanonicalThreshold(t *testing.T) {
	originalText := "Xyz{"
	candidateText := "Xyza"
	if subtitleOCRScoreForLanguage(candidateText, "eng") <= subtitleOCRScoreForLanguage(originalText, "eng") {
		t.Fatalf("test candidate is not raw-score better: original=%v candidate=%v", subtitleOCRScoreForLanguage(originalText, "eng"), subtitleOCRScoreForLanguage(candidateText, "eng"))
	}
	if subtitleOCRCandidateIsMateriallyBetter(originalText, candidateText, "eng") {
		t.Fatalf("test candidate unexpectedly passes canonical material-improvement threshold: original=%v candidate=%v", subtitleOCRScoreForLanguage(originalText, "eng"), subtitleOCRScoreForLanguage(candidateText, "eng"))
	}
	mediaPath, stream, _, _ := setupSelectiveBitmapTranslationFixture(t, originalText, candidateText)
	provider := &bitmapTranslationProviderSpy{}
	result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
		MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate",
		TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
	})
	if err == nil || !strings.Contains(err.Error(), "requires review") || subtitleTranslationFailureClassOf(err) != subtitleTranslationFailureOCRReviewRequired || provider.calls != 0 || result.Outputs != nil {
		t.Fatalf("below-threshold selective candidate was accepted: result=%#v calls=%d err=%v", result, provider.calls, err)
	}
}

func TestBitmapTranslationRetryUsesCorrectedPublishedCanonicalOCREvidence(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("PGS source"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'bad{' > "$output"
`)
	installPGSSUPTestExtractor(t, bin)
	lifecycleRoot := filepath.Join(root, "ocr-cache")
	persistentRoot := filepath.Join(root, "ocr-state")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", lifecycleRoot)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", persistentRoot)
	stream := FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "eng"}}
	generatedPath := filepath.Join(root, "generated.eng.srt")
	generated, err := generateBitmapSubtitleAtPathWithResult(context.Background(), mediaPath, stream, "srt", "eng", "accurate", generatedPath, 0, 0, nil)
	if err != nil {
		t.Fatalf("baseline OCR could not generate SRT: %v", err)
	}
	baseline, err := os.ReadFile(generatedPath)
	if err != nil || !validSubtitleSidecar("srt", baseline) || len(generated.Evidence.Cues) != 1 || generated.Evidence.Cues[0].Text != "bad{" {
		t.Fatalf("baseline OCR did not produce a valid reviewable SRT: text=%q err=%v", baseline, err)
	}

	logicalTransform := "subtitle:4:converted:srt"
	sidecarPath := filepath.Join(root, "movie.eng.srt")
	if err := os.WriteFile(sidecarPath, baseline, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecarPath, "srt", logicalTransform, generated.Evidence, "baseline-test", ""); err != nil {
		t.Fatal(err)
	}
	provider := &bitmapTranslationProviderSpy{}
	runner := func(ctx context.Context, operationID, _ string, _ subtitleTranslationOperationRequest) error {
		updateSubtitleTranslationProgress(operationID, "ocr", 0, 0, "Preparing canonical OCR subtitles")
		_, executionErr := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
			MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRLanguage: "eng", OCRMode: "accurate",
			TargetLocale: "es-419", RequestedRepresentation: "srt", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
			OnProgress: func(processed, total int) {
				updateSubtitleTranslationProgress(operationID, "ocr", processed, total, "Preparing canonical OCR subtitles")
			},
			OnBatch: func(processed, total int) {
				updateSubtitleTranslationProgress(operationID, "translating", processed, total, "Translating subtitle batches")
			},
		})
		return executionErr
	}
	request := testSubtitleTranslationOperationRequest(mediaPath)
	operationID, err := startSubtitleTranslationOperation(request, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	failed := waitForSubtitleTranslationOperation(t, operationID, "error")
	if failed.Phase != "ocr_review" || failed.Progress != 40 || failed.Translation == nil || failed.Translation.FailureClass != string(subtitleTranslationFailureOCRReviewRequired) || failed.Translation.Diagnostic != "OCR review is required before translation." || provider.calls != 0 {
		t.Fatalf("baseline review requirement was not surfaced before provider execution: operation=%#v calls=%d", failed, provider.calls)
	}

	oldRevision, _, err := publishedSubtitleRevisionFor(sidecarPath, logicalTransform)
	if err != nil {
		t.Fatal(err)
	}
	correctedEvidence := cloneCanonicalOCREvidence(generated.Evidence)
	correctedEvidence.Cues[0].Text = "The house"
	correctedEvidence.CorrectionProvenance = append(correctedEvidence.CorrectionProvenance, canonicalOCRCorrectionProvenance{
		CueID: 1, Action: "edit", CandidateSource: "manual-review", Validation: "canonical OCR correction", SelectedBy: "reviewer",
	})
	stagedCorrection := filepath.Join(t.TempDir(), "reviewed.eng.srt")
	writeTestSRT(t, stagedCorrection, correctedEvidence.Cues[0].Text)
	correctedSnapshot := &publishedCanonicalOCRSnapshot{Format: "srt", LogicalTransformKey: logicalTransform, Evidence: *correctedEvidence}
	if err := publishCorrectedSubtitleFiles(context.Background(), "review-correction", []string{sidecarPath}, []string{"srt"}, []string{stagedCorrection}, []*publishedCanonicalOCRSnapshot{correctedSnapshot}, []publishedSubtitleRevision{oldRevision}); err != nil {
		t.Fatalf("canonical OCR review correction was not published: %v", err)
	}

	retryID, err := retrySubtitleTranslationOperation(operationID, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(retryID)
	if retryID == operationID {
		t.Fatal("retry reused the failed translation operation identity")
	}
	completed := waitForSubtitleTranslationOperation(t, retryID, "completed")
	if completed.Phase != "completed" || provider.calls != 1 || len(provider.batches) != 1 || len(provider.batches[0].Cues) != 1 || provider.batches[0].Cues[0].Text != "The house" {
		t.Fatalf("retry did not translate the corrected canonical OCR evidence: operation=%#v batches=%#v calls=%d", completed, provider.batches, provider.calls)
	}
}

func TestExecuteBitmapTranslationCancellationDuringSelectiveOCRDoesNotCallProvider(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := installCanonicalOCRTestTools(t, `#!/bin/sh
touch "$SELECTIVE_OCR_STARTED"
while true; do sleep 1; done
`)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
printf 'bounded subtitle input' > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "ffprobe"), `#!/bin/sh
printf '%s' '{"streams":[{"index":0,"id":"0x1","codec_type":"subtitle","codec_name":"dvd_subtitle"}]}'
`)
	started := filepath.Join(root, "selective-started")
	t.Setenv("SELECTIVE_OCR_STARTED", started)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	stream := FFProbeStream{Index: 1, ID: "vob-1", CodecName: "dvd_subtitle"}
	cacheBitmapTranslationExecutionEvidence(t, mediaPath, stream, "eng", []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "The h�use", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
	provider := &bitmapTranslationProviderSpy{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
			MediaPath: mediaPath, Stream: stream, RequestedLanguage: "eng", OCRMode: "accurate", TargetLocale: "es-419",
			Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
		})
		done <- err
	}()
	waitForFile(t, started)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || provider.calls != 0 {
			t.Fatalf("selective OCR cancellation was not preserved: calls=%d err=%v", provider.calls, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("translation did not return after selective OCR cancellation")
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
			installPGSSUPTestExtractor(t, bin)
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			provider := &bitmapTranslationProviderSpy{}
			result, err := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
				MediaPath: mediaPath, Stream: FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle"}, RequestedLanguage: "eng", OCRMode: "raw",
				TargetLocale: "es-419", Provider: provider, Settings: bitmapTranslationExecutionTestSettings(),
			})
			if err == nil || subtitleTranslationFailureClassOf(err) != subtitleTranslationFailureOCRFailed || provider.calls != 0 || result.Outputs != nil {
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
	installPGSSUPTestExtractor(t, bin)
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
