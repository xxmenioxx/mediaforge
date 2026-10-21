package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type assetInfoTranslationProviderStub struct {
	calls int
	fn    func(TranslationBatch) TranslationBatchResult
}

func (provider *assetInfoTranslationProviderStub) Translate(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	provider.calls++
	return provider.fn(batch), nil
}

func (provider *assetInfoTranslationProviderStub) resultFor(batch TranslationBatch) TranslationBatchResult {
	result := TranslationBatchResult{ContractVersion: batch.ContractVersion}
	for _, cue := range batch.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: strings.Replace(cue.Text, "Hello", "Hola", 1)})
	}
	return result
}

func TestTranslationCapabilityAvailabilityIsBackendAuthoritative(t *testing.T) {
	base := defaultTranslationSettings()
	base.Provider = translationProviderOllama
	base.OllamaEndpoint = "http://nas:11434"
	base.OllamaModel = "llama3"
	cases := []struct {
		name      string
		mutate    func(*translationSettings)
		available bool
		reason    string
	}{
		{name: "not configured", mutate: func(settings *translationSettings) { settings.Provider = "" }, reason: "Translation provider is not configured."},
		{name: "untested", mutate: func(settings *translationSettings) { settings.Readiness = translationReadinessUntested }, reason: "Translation provider configuration must be tested."},
		{name: "disabled", mutate: func(settings *translationSettings) { settings.Readiness = translationReadinessDisabled }, reason: "Subtitle translation is disabled."},
		{name: "ready and enabled", mutate: func(settings *translationSettings) {
			settings.Readiness = translationReadinessReady
			settings.Enabled = true
			settings.ReadinessIdentity = translationProviderConfigurationIdentity(*settings)
		}, available: true},
		{name: "stale readiness", mutate: func(settings *translationSettings) {
			settings.Readiness = translationReadinessReady
			settings.Enabled = true
			settings.ReadinessIdentity = "stale"
		}, reason: "Translation provider configuration must be tested."},
		{name: "model unavailable", mutate: func(settings *translationSettings) { settings.Readiness = translationReadinessModelNotFound }, reason: "Configured translation model is unavailable."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := base
			testCase.mutate(&settings)
			available, reason := translationCapabilityAvailability(settings)
			if available != testCase.available || reason != testCase.reason {
				t.Fatalf("availability = %v, %q; want %v, %q", available, reason, testCase.available, testCase.reason)
			}
		})
	}
}

func TestRequestedTranslationOutputPreservesTextAndBitmapRules(t *testing.T) {
	for _, testCase := range []struct {
		source, requested, want string
	}{
		{source: "srt", requested: "", want: "srt"},
		{source: "ass", requested: "ass", want: "ass"},
		{source: "ssa", requested: "ssa", want: "ssa"},
		{source: "bitmap", requested: "", want: "srt"},
		{source: "bitmap", requested: "both", want: "both"},
	} {
		got, err := requestedTranslationOutput(testCase.source, testCase.requested)
		if err != nil || got != testCase.want {
			t.Fatalf("requestedTranslationOutput(%q, %q) = %q, %v; want %q", testCase.source, testCase.requested, got, err, testCase.want)
		}
	}
	if _, err := requestedTranslationOutput("srt", "ass"); err == nil {
		t.Fatal("expected text representation change to be rejected")
	}
	if _, err := requestedTranslationOutput("bitmap", "ssa"); err == nil {
		t.Fatal("expected bitmap SSA output to be rejected")
	}
}

func TestExecuteTextTranslationSRTUsesExactCueContract(t *testing.T) {
	provider := &assetInfoTranslationProviderStub{}
	provider.fn = provider.resultFor
	settings := defaultTranslationSettings()
	source := subtitleTranslationSourceResponse{
		Type:           "external",
		SourceIdentity: "external:/media/movie.eng.srt:revision",
		SourceRevision: "revision",
		Representation: "srt",
		ExternalPath:   "/media/movie.eng.srt",
		Codec:          "srt",
		Language:       "eng",
	}
	result, err := executeTextTranslation(context.Background(), source, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n\n"), subtitleTranslationOperationRequest{
		RequestedSourceLanguage: "auto",
		TargetLocale:            "es-419",
		RequestedRepresentation: "srt",
		SourceIdentity:          source.SourceIdentity,
	}, provider, settings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || !strings.Contains(string(result.Outputs["srt"]), "Hola") || strings.Contains(string(result.Outputs["srt"]), "Hello") {
		t.Fatalf("unexpected SRT result: calls=%d output=%q", provider.calls, result.Outputs["srt"])
	}
	if !strings.Contains(string(result.Outputs["srt"]), "00:00:01,000 --> 00:00:02,000") {
		t.Fatalf("translated SRT lost canonical timing: %q", result.Outputs["srt"])
	}
}

func TestExecuteTextTranslationAutoAmbiguityStopsBeforeProvider(t *testing.T) {
	provider := &assetInfoTranslationProviderStub{}
	provider.fn = provider.resultFor
	settings := defaultTranslationSettings()
	source := subtitleTranslationSourceResponse{
		Type:           "external",
		SourceIdentity: "external:/media/movie.und.srt:revision",
		SourceRevision: "revision",
		Representation: "srt",
		ExternalPath:   "/media/movie.und.srt",
		Codec:          "srt",
		Language:       "und",
	}

	result, err := executeTextTranslation(context.Background(), source, []byte("1\n00:00:01,000 --> 00:00:02,000\n123 !!!\n\n"), subtitleTranslationOperationRequest{
		RequestedSourceLanguage: "auto",
		TargetLocale:            "es-419",
		RequestedRepresentation: "srt",
		SourceIdentity:          source.SourceIdentity,
	}, provider, settings, nil, nil)
	if err == nil {
		t.Fatal("expected ambiguous Auto source language to fail closed")
	}
	if provider.calls != 0 {
		t.Fatalf("ambiguous Auto source invoked provider %d times", provider.calls)
	}
	if result.Outputs != nil {
		t.Fatalf("ambiguous Auto source returned translated outputs: %#v", result.Outputs)
	}
}

func TestExecuteTextTranslationJapaneseSourceUsesCanonicalLanguage(t *testing.T) {
	provider := &assetInfoTranslationProviderStub{}
	var observed TranslationBatch
	provider.fn = func(batch TranslationBatch) TranslationBatchResult {
		observed = batch
		return provider.resultFor(batch)
	}
	settings := defaultTranslationSettings()
	source := subtitleTranslationSourceResponse{
		Type:           "external",
		SourceIdentity: "external:/media/movie.jpn.srt:revision",
		SourceRevision: "revision",
		Representation: "srt",
		ExternalPath:   "/media/movie.jpn.srt",
		Codec:          "srt",
		Language:       "jpn",
	}

	result, err := executeTextTranslation(context.Background(), source, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n\n"), subtitleTranslationOperationRequest{
		RequestedSourceLanguage: "ja",
		TargetLocale:            "es-419",
		RequestedRepresentation: "srt",
		SourceIdentity:          source.SourceIdentity,
	}, provider, settings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || observed.SourceLanguage != "ja" || observed.TargetLocale != "es-419" {
		t.Fatalf("provider batch=%#v calls=%d, want Japanese es-419 batch and one call", observed, provider.calls)
	}
	if !strings.Contains(string(result.Outputs["srt"]), "Hola") {
		t.Fatalf("Japanese source did not produce translated SRT: %q", result.Outputs["srt"])
	}
}

func TestExecuteTextTranslationASSAndSSAKeepProtectedStructure(t *testing.T) {
	fixtures := []struct {
		name, representation, format string
	}{
		{name: "ASS", representation: "ass", format: "Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"},
		{name: "SSA", representation: "ssa", format: "Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			provider := &assetInfoTranslationProviderStub{}
			provider.fn = provider.resultFor
			settings := defaultTranslationSettings()
			source := subtitleTranslationSourceResponse{
				Type: "external", SourceIdentity: "external:/media/movie." + fixture.representation,
				SourceRevision: "revision", Representation: fixture.representation,
				ExternalPath: "/media/movie." + fixture.representation, Codec: fixture.representation,
				Language: "eng",
			}
			content := "[Script Info]\nTitle: Fixture\n\n[Events]\nFormat: " + fixture.format + "\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}Hello{\\pos(10,20)}\n"
			result, err := executeTextTranslation(context.Background(), source, []byte(content), subtitleTranslationOperationRequest{
				RequestedSourceLanguage: "en", TargetLocale: "es-ES", RequestedRepresentation: fixture.representation, SourceIdentity: source.SourceIdentity,
			}, provider, settings, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			output := string(result.Outputs[fixture.representation])
			if !strings.Contains(output, "{\\an8}Hola{\\pos(10,20)}") || !strings.Contains(output, "Format: "+fixture.format) {
				t.Fatalf("protected %s structure was not preserved: %q", fixture.name, output)
			}
		})
	}
}

func TestTranslationCapabilityOutputsAreBackendSelected(t *testing.T) {
	cases := map[string][]string{
		"srt":    {"srt"},
		"ass":    {"ass"},
		"ssa":    {"ssa"},
		"bitmap": {"srt", "ass", "both"},
	}
	for source, want := range cases {
		got := translationCapabilityOutputs(source)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("capability outputs for %s = %v; want %v", source, got, want)
		}
	}
}

type assetInfoTranslationOllamaFixture struct {
	mu              sync.Mutex
	paths           []string
	batches         []TranslationBatch
	failures        []error
	factorySettings []translationSettings
	calls           int
	factoryCalls    int
	readinessCalls  int
}

func newAssetInfoTranslationOllamaFixture() *assetInfoTranslationOllamaFixture {
	return &assetInfoTranslationOllamaFixture{}
}

func (fixture *assetInfoTranslationOllamaFixture) Translate(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.paths = append(fixture.paths, "/api/chat")
	fixture.batches = append(fixture.batches, batch)
	fixture.calls++
	if len(fixture.failures) > 0 {
		err := fixture.failures[0]
		fixture.failures = fixture.failures[1:]
		return TranslationBatchResult{}, err
	}
	result := TranslationBatchResult{ContractVersion: batch.ContractVersion}
	for _, cue := range batch.Cues {
		text := strings.ReplaceAll(cue.Text, "Hello", "Hola")
		text = strings.ReplaceAll(text, "안녕하세요", "Hola")
		text = strings.ReplaceAll(text, "Bonjour", "Hola")
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: text})
	}
	return result, nil
}

func (fixture *assetInfoTranslationOllamaFixture) TestConnection(context.Context) TranslationProviderReadiness {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.readinessCalls++
	return TranslationProviderReadiness{State: translationReadinessReady}
}

func (fixture *assetInfoTranslationOllamaFixture) pathsSnapshot() []string {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]string(nil), fixture.paths...)
}

func (fixture *assetInfoTranslationOllamaFixture) callsSnapshot() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}

func (fixture *assetInfoTranslationOllamaFixture) batchesSnapshot() []TranslationBatch {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]TranslationBatch(nil), fixture.batches...)
}

func (fixture *assetInfoTranslationOllamaFixture) readinessCallsSnapshot() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.readinessCalls
}

func (fixture *assetInfoTranslationOllamaFixture) factoryCallsSnapshot() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.factoryCalls
}

func (fixture *assetInfoTranslationOllamaFixture) factory() func(translationSettings) (TranslationProvider, error) {
	return func(settings translationSettings) (TranslationProvider, error) {
		fixture.mu.Lock()
		fixture.factoryCalls++
		fixture.factorySettings = append(fixture.factorySettings, settings)
		fixture.mu.Unlock()
		return fixture, nil
	}
}

func (fixture *assetInfoTranslationOllamaFixture) factorySettingsSnapshot() []translationSettings {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]translationSettings(nil), fixture.factorySettings...)
}

func readyAssetInfoTranslationSettings(endpoint string) translationSettings {
	settings := defaultTranslationSettings()
	settings.Provider = translationProviderOllama
	settings.OllamaEndpoint = endpoint
	settings.OllamaModel = "fixture-model"
	settings.Enabled = true
	settings.Readiness = translationReadinessReady
	settings.ReadinessIdentity = translationProviderConfigurationIdentity(settings)
	return settings
}

func postAssetInfoTranslationStart(t *testing.T, handler AssetHandler, body string) (int, string) {
	t.Helper()
	router := gin.New()
	router.POST("/api/assets/subtitle-translation", handler.StartSubtitleTranslation)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-translation", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func postAssetInfoTranslationCapability(t *testing.T, handler AssetHandler, body string) (int, string) {
	t.Helper()
	router := gin.New()
	router.POST("/api/assets/subtitle-translation/capability", handler.SubtitleTranslationCapability)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-translation/capability", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func postAssetInfoTranslationRetry(t *testing.T, handler AssetHandler, operationID string) (int, string) {
	t.Helper()
	router := gin.New()
	router.POST("/api/assets/subtitle-operations/:id/retry", handler.RetrySubtitleTranslation)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+operationID+"/retry", nil)
	router.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func subtitleTranslationOperationCountForTest() int {
	subtitleExtractionOperations.RLock()
	defer subtitleExtractionOperations.RUnlock()
	return len(subtitleExtractionOperations.items)
}

func configureAssetInfoTranslationFixture(t *testing.T, dbPath, endpoint string) *gorm.DB {
	t.Helper()
	mediaPath := filepath.Join(dbPath, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	db := translationSourceTestDB(t, dbPath, mediaPath)
	persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings(endpoint))
	return db
}

func assetInfoPublishedFiles(t *testing.T, root, mediaPath string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Join(root, entry.Name()) == mediaPath {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = content
	}
	return files
}

func installAssetInfoTranslationFFmpeg(t *testing.T, fixture string) string {
	t.Helper()
	bin := t.TempDir()
	fixturePath := filepath.Join(bin, "subtitle.fixture")
	argsPath := filepath.Join(bin, "ffmpeg.args")
	writeTestFile(t, fixturePath, fixture)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\nset -eu\nlast=\nfor arg in \"$@\"; do last=\"$arg\"; done\nprintf '%s\\n' \"$@\" > \"$MVFORGE_TRANSLATION_FFMPEG_ARGS\"\ncp \"$MVFORGE_TRANSLATION_FFMPEG_SOURCE\" \"$last\"\n")
	t.Setenv("MVFORGE_TRANSLATION_FFMPEG_SOURCE", fixturePath)
	t.Setenv("MVFORGE_TRANSLATION_FFMPEG_ARGS", argsPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}

func startAssetInfoTranslationAndWait(t *testing.T, handler AssetHandler, body string) subtitleExtractionOperation {
	t.Helper()
	status, responseBody := postAssetInfoTranslationStart(t, handler, body)
	if status != http.StatusAccepted {
		t.Fatalf("translation start status=%d body=%s", status, responseBody)
	}
	var response struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	if response.Operation.ID == "" || response.Operation.Kind != subtitleTranslationOperationKind {
		t.Fatalf("unexpected translation operation response: %#v", response)
	}
	return waitForSubtitleTranslationOperation(t, response.Operation.ID, "completed")
}

func TestAssetInfoTranslationEntryPointExternalSRTAndASS(t *testing.T) {
	for _, testCase := range []struct {
		name, extension, language, sourceLanguage, locale, expected, model, effectiveLanguage, sourceText string
	}{
		{name: "English SRT", extension: "srt", language: "eng", sourceLanguage: "en", locale: "es-419", expected: "Hola", effectiveLanguage: "en", sourceText: "Hello"},
		{name: "Japanese SRT", extension: "srt", language: "jpn", sourceLanguage: "ja", locale: "es-419", expected: "Hola", effectiveLanguage: "ja", sourceText: "Hello"},
		{name: "English ASS", extension: "ass", language: "eng", sourceLanguage: "en", locale: "es-ES", expected: "{\\an8}Hola{\\pos(10,20)}", effectiveLanguage: "en", sourceText: "Hello"},
		{name: "Korean SRT explicit ko", extension: "srt", language: "kor", sourceLanguage: "ko", locale: "es-419", expected: "Hola", model: "translategemma:4b", effectiveLanguage: "ko", sourceText: "안녕하세요"},
		{name: "Korean SRT alias and metadata Auto", extension: "srt", language: "kor", sourceLanguage: "auto", locale: "es-419", expected: "Hola", model: "translategemma:12b", effectiveLanguage: "ko", sourceText: "안녕하세요"},
		{name: "Korean ASS alias", extension: "ass", language: "kor", sourceLanguage: "kor", locale: "es-ES", expected: "{\\an8}Hola{\\pos(10,20)}", model: "translategemma:27b", effectiveLanguage: "ko", sourceText: "안녕하세요"},
		{name: "French SRT alias and metadata Auto", extension: "srt", language: "fra", sourceLanguage: "auto", locale: "es-419", expected: "Hola", model: "translategemma:4b", effectiveLanguage: "fr", sourceText: "Bonjour"},
		{name: "French ASS legacy fre", extension: "ass", language: "fra", sourceLanguage: "fre", locale: "es-ES", expected: "{\\an8}Hola{\\pos(10,20)}", model: "translategemma:12b", effectiveLanguage: "fr", sourceText: "Bonjour"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			mediaPath := filepath.Join(root, "episode.mkv")
			writeTestFile(t, mediaPath, "media fixture")
			sourcePath := filepath.Join(root, "episode."+testCase.language+"."+testCase.extension)
			content := "1\n00:00:01,000 --> 00:00:02,000\n" + testCase.sourceText + "\n\n"
			if testCase.extension == "ass" {
				content = "[Script Info]\nTitle: Fixture\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, AlphaLevel, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,1,2,0,2,10,10,10,0,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}" + testCase.sourceText + "{\\pos(10,20)}\n"
			}
			writeTestFile(t, sourcePath, content)
			fixture := newAssetInfoTranslationOllamaFixture()
			db := translationSourceTestDB(t, root, mediaPath)
			settings := readyAssetInfoTranslationSettings("http://nas:11434")
			if testCase.model != "" {
				settings.OllamaModel = testCase.model
				settings.ReadinessIdentity = translationProviderConfigurationIdentity(settings)
			}
			persistTranslationSettingsFixture(t, db, settings)
			handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
			request := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":%q,"targetLocale":%q,"outputRepresentation":%q}`, mediaPath, sourcePath, testCase.sourceLanguage, testCase.locale, testCase.extension)
			capabilityStatus, capabilityBody := postAssetInfoTranslationCapability(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, sourcePath))
			if capabilityStatus != http.StatusOK {
				t.Fatalf("capability status=%d body=%s", capabilityStatus, capabilityBody)
			}
			var capability subtitleTranslationCapabilityResponse
			if err := json.Unmarshal([]byte(capabilityBody), &capability); err != nil {
				t.Fatal(err)
			}
			if hasKoreanFrench := len(capability.SourceLanguages) == 5; hasKoreanFrench != (testCase.model != "") {
				t.Fatalf("capability source languages=%v for model %q", capability.SourceLanguages, testCase.model)
			}
			if paths := fixture.pathsSnapshot(); len(paths) != 0 {
				t.Fatalf("capability performed provider readiness work: %v", paths)
			}
			if fixture.factoryCallsSnapshot() != 0 || fixture.readinessCallsSnapshot() != 0 {
				t.Fatalf("capability constructed or tested a provider: factories=%d readiness=%d", fixture.factoryCallsSnapshot(), fixture.readinessCallsSnapshot())
			}
			beforeSource, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			operation := startAssetInfoTranslationAndWait(t, handler, request)
			if operation.Result == nil || len(operation.Result.Artifacts) != 1 || len(operation.Result.Created) != 1 {
				t.Fatalf("translation result did not expose the published artifact: %#v", operation.Result)
			}
			requestedLanguage, err := normalizeTranslationLanguage(testCase.sourceLanguage, true)
			if err != nil {
				t.Fatal(err)
			}
			metadata := operation.Result.Artifacts[0].Translation
			if metadata == nil || metadata.RequestedSourceLanguage != requestedLanguage || metadata.EffectiveSourceLanguage != testCase.effectiveLanguage {
				t.Fatalf("artifact language identity=%#v, want requested=%q effective=%q", metadata, requestedLanguage, testCase.effectiveLanguage)
			}
			if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, beforeSource) {
				t.Fatalf("source changed during translation: err=%v", err)
			}
			if fixture.callsSnapshot() != 1 {
				t.Fatalf("provider calls=%d, want 1", fixture.callsSnapshot())
			}
			batches := fixture.batchesSnapshot()
			if len(batches) != 1 || batches[0].SourceLanguage != testCase.effectiveLanguage || batches[0].TargetLocale != testCase.locale {
				t.Fatalf("provider language pair=%#v, want %s to %s", batches, testCase.effectiveLanguage, testCase.locale)
			}
			if fixture.readinessCallsSnapshot() != 0 {
				t.Fatalf("normal translation performed readiness calls=%d", fixture.readinessCallsSnapshot())
			}
			for _, path := range fixture.pathsSnapshot() {
				if path != "/api/chat" {
					t.Fatalf("normal translation performed readiness preflight: %v", fixture.pathsSnapshot())
				}
			}
			published := assetInfoPublishedFiles(t, root, mediaPath)
			if len(published) != 2 {
				t.Fatalf("published files=%v, want source sidecar plus translated sidecar", published)
			}
			found := false
			for name, value := range published {
				if strings.Contains(strings.ToLower(name), "."+strings.ToLower(testCase.locale)+".") {
					found = true
					if !strings.Contains(string(value), testCase.expected) {
						t.Fatalf("translated %s output=%q", testCase.name, value)
					}
				}
			}
			if !found {
				t.Fatalf("translated %s sidecar was not published: %v", testCase.name, published)
			}
		})
	}
}

func TestAssetInfoTranslationCapabilityAllowsPublishedLibraryVariants(t *testing.T) {
	for _, status := range []string{"converted", "published_as_is", "accepted"} {
		t.Run(status, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "episode.mkv")
			sourcePath := filepath.Join(root, "episode.eng.srt")
			writeTestFile(t, mediaPath, "media fixture")
			writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\nHello\n")
			db := translationSourceTestDB(t, root, mediaPath)
			if err := db.Model(&models.AssetRecord{}).Where("path = ?", mediaPath).Update("status", status).Error; err != nil {
				t.Fatal(err)
			}
			handler := AssetHandler{db: db}
			request := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, sourcePath)
			capabilityStatus, capabilityBody := postAssetInfoTranslationCapability(t, handler, request)
			if capabilityStatus != http.StatusOK {
				t.Fatalf("capability status=%d body=%s", capabilityStatus, capabilityBody)
			}
			var capability subtitleTranslationCapabilityResponse
			if err := json.Unmarshal([]byte(capabilityBody), &capability); err != nil {
				t.Fatal(err)
			}
			if !capability.Eligible || capability.Source.ExternalPath != sourcePath || capability.Source.SourceRevision == "" {
				t.Fatalf("published lifecycle %q was not independently eligible for its exact source: %#v", status, capability)
			}
		})
	}
}

func TestAssetInfoTranslationCapabilityAndStartRejectActiveWork(t *testing.T) {
	for _, kind := range []string{"queue", "maintenance"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "episode.mkv")
			sourcePath := filepath.Join(root, "episode.eng.srt")
			writeTestFile(t, mediaPath, "media fixture")
			writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\nHello\n")
			db := translationSourceTestDB(t, root, mediaPath)
			if err := db.AutoMigrate(&models.QueueJob{}, &models.AssetMaintenanceOperation{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&models.AssetRecord{}).Where("path = ?", mediaPath).Update("status", "converted").Error; err != nil {
				t.Fatal(err)
			}
			var record models.AssetRecord
			if err := db.Where("path = ?", mediaPath).First(&record).Error; err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "queue":
				if err := db.Create(&models.QueueJob{
					MediaPath: mediaPath, PublishedPath: mediaPath, LibraryID: 1,
					Status: JobStatusQueued, Stage: JobStageQueued,
				}).Error; err != nil {
					t.Fatal(err)
				}
			case "maintenance":
				if err := db.Create(&models.AssetMaintenanceOperation{
					ID: "maintenance-test", AssetRecordID: record.ID, AssetPath: mediaPath,
					Status: maintenanceStatusRunning, Phase: "running",
				}).Error; err != nil {
					t.Fatal(err)
				}
			}

			handler := AssetHandler{db: db}
			request := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, sourcePath)
			capabilityStatus, capabilityBody := postAssetInfoTranslationCapability(t, handler, request)
			if capabilityStatus != http.StatusUnprocessableEntity {
				t.Fatalf("capability status=%d body=%s", capabilityStatus, capabilityBody)
			}
			var capability subtitleTranslationCapabilityResponse
			if err := json.Unmarshal([]byte(capabilityBody), &capability); err != nil {
				t.Fatal(err)
			}
			if capability.Eligible || !strings.Contains(capability.Reason, "active Queue or maintenance") {
				t.Fatalf("active %s work did not block capability with a concrete reason: %#v", kind, capability)
			}

			startRequest := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":"auto","targetLocale":"es-419"}`, mediaPath, sourcePath)
			startStatus, startBody := postAssetInfoTranslationStart(t, handler, startRequest)
			if startStatus != http.StatusUnprocessableEntity || !strings.Contains(startBody, "active Queue or maintenance") {
				t.Fatalf("active %s work start status=%d body=%s", kind, startStatus, startBody)
			}
		})
	}
}

func TestAssetInfoTranslationCapabilityRejectsMissingUnsupportedAndArchivedSources(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		selectedName   string
		writeSelected  bool
		assetStatus    string
		wantReasonPart string
	}{
		{name: "missing exact sidecar", selectedName: "episode.jpn.srt", assetStatus: "converted"},
		{name: "unsupported format", selectedName: "episode.eng.vtt", writeSelected: true, assetStatus: "converted"},
		{name: "archive original", selectedName: "episode.eng.srt", writeSelected: true, assetStatus: "archive", wantReasonPart: "archived originals"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "episode.mkv")
			writeTestFile(t, mediaPath, "media fixture")
			if testCase.writeSelected {
				writeTestFile(t, filepath.Join(root, testCase.selectedName), "subtitle fixture")
			}
			if testCase.name == "missing exact sidecar" {
				writeTestFile(t, filepath.Join(root, "episode.eng.srt"), "1\n00:00:01,000 --> 00:00:02,000\nSibling\n")
			}
			db := translationSourceTestDB(t, root, mediaPath)
			if err := db.Model(&models.AssetRecord{}).Where("path = ?", mediaPath).Update("status", testCase.assetStatus).Error; err != nil {
				t.Fatal(err)
			}
			handler := AssetHandler{db: db}
			selectedPath := filepath.Join(root, testCase.selectedName)
			request := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, selectedPath)
			capabilityStatus, capabilityBody := postAssetInfoTranslationCapability(t, handler, request)
			if capabilityStatus != http.StatusUnprocessableEntity {
				t.Fatalf("capability status=%d body=%s", capabilityStatus, capabilityBody)
			}
			var capability subtitleTranslationCapabilityResponse
			if err := json.Unmarshal([]byte(capabilityBody), &capability); err != nil {
				t.Fatal(err)
			}
			if capability.Eligible || capability.Source.ExternalPath == selectedPath {
				t.Fatalf("blocked source was accepted or substituted: %#v", capability)
			}
			if testCase.wantReasonPart != "" && !strings.Contains(capability.Reason, testCase.wantReasonPart) {
				t.Fatalf("reason=%q does not identify archived source", capability.Reason)
			}
		})
	}
}

func TestAssetInfoTranslationStartRejectsUnverifiedKoreanAndFrenchModel(t *testing.T) {
	for _, testCase := range []struct {
		language, sourceLanguage string
	}{
		{language: "kor", sourceLanguage: "ko"},
		{language: "fra", sourceLanguage: "fr"},
		{language: "kor", sourceLanguage: "auto"},
		{language: "fra", sourceLanguage: "auto"},
	} {
		t.Run(testCase.language+"/"+testCase.sourceLanguage, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "episode.mkv")
			sourcePath := filepath.Join(root, "episode."+testCase.language+".srt")
			writeTestFile(t, mediaPath, "media fixture")
			writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\nsubtitle\n\n")
			db := translationSourceTestDB(t, root, mediaPath)
			persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings("http://nas:11434"))
			fixture := newAssetInfoTranslationOllamaFixture()
			handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
			body := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":%q,"targetLocale":"es-419"}`, mediaPath, sourcePath, testCase.sourceLanguage)
			status, response := postAssetInfoTranslationStart(t, handler, body)
			if status != http.StatusConflict || !strings.Contains(response, "TranslateGemma") {
				t.Fatalf("unsupported model status=%d response=%s", status, response)
			}
			if fixture.factoryCallsSnapshot() != 0 || fixture.callsSnapshot() != 0 {
				t.Fatalf("unsupported pair constructed or called provider: factories=%d calls=%d", fixture.factoryCallsSnapshot(), fixture.callsSnapshot())
			}
		})
	}
}

func TestAssetInfoTranslationRetryRefreshesSourceAndProviderThroughHTTP(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	mediaPath := filepath.Join(root, "episode.mkv")
	sourcePath := filepath.Join(root, "episode.kor.srt")
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\n안녕하세요 v1\n\n")
	db := translationSourceTestDB(t, root, mediaPath)
	settingsV1 := readyAssetInfoTranslationSettings("http://nas:11434")
	settingsV1.OllamaModel = "translategemma:4b"
	settingsV1.ReadinessIdentity = translationProviderConfigurationIdentity(settingsV1)
	persistTranslationSettingsFixture(t, db, settingsV1)
	fixture := newAssetInfoTranslationOllamaFixture()
	fixture.failures = []error{&TranslationProviderError{Class: translationProviderErrorUnavailable, Err: errors.New("first provider attempt")}}
	handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
	startBody := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":"kor","targetLocale":"es-419","outputRepresentation":"srt"}`, mediaPath, sourcePath)
	status, responseBody := postAssetInfoTranslationStart(t, handler, startBody)
	if status != http.StatusAccepted {
		t.Fatalf("initial translation status=%d body=%s", status, responseBody)
	}
	var startResponse struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(responseBody), &startResponse); err != nil {
		t.Fatal(err)
	}
	oldID := startResponse.Operation.ID
	oldOperation := waitForSubtitleTranslationOperation(t, oldID, "error")
	if oldOperation.Translation == nil || !oldOperation.Translation.Retryable || oldOperation.Translation.Model != "translategemma:4b" || oldOperation.translationRequest.RequestedSourceLanguage != "ko" {
		t.Fatalf("unexpected terminal predecessor: %#v", oldOperation.Translation)
	}
	oldRevision := oldOperation.translationRequest.SourceRevision

	writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\n안녕하세요 v2\n\n")
	settingsV2 := settingsV1
	settingsV2.OllamaModel = "translategemma:12b"
	settingsV2.ReadinessIdentity = translationProviderConfigurationIdentity(settingsV2)
	persistTranslationSettingsFixture(t, db, settingsV2)

	retryStatus, retryBody := postAssetInfoTranslationRetry(t, handler, oldID)
	if retryStatus != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", retryStatus, retryBody)
	}
	var retryResponse struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(retryBody), &retryResponse); err != nil {
		t.Fatal(err)
	}
	if retryResponse.Operation.ID == "" || retryResponse.Operation.ID == oldID {
		t.Fatalf("retry did not create a new operation: %#v", retryResponse.Operation)
	}
	newOperation := waitForSubtitleTranslationOperation(t, retryResponse.Operation.ID, "completed")
	if newOperation.translationRequest.SourceRevision == oldRevision {
		t.Fatalf("retry reused source revision %q", oldRevision)
	}
	if newOperation.Translation == nil || newOperation.Translation.Provider != "ollama" || newOperation.Translation.Model != "translategemma:12b" || newOperation.translationRequest.RequestedSourceLanguage != "ko" {
		t.Fatalf("retry did not use current provider configuration: %#v", newOperation.Translation)
	}
	if oldAfter := snapshotSubtitleOperation(oldID); oldAfter.ID != oldID || oldAfter.Status != "error" || oldAfter.translationRequest.SourceRevision != oldRevision {
		t.Fatalf("predecessor changed during retry: %#v", oldAfter)
	}
	if fixture.readinessCallsSnapshot() != 0 {
		t.Fatalf("retry performed readiness preflight: %d", fixture.readinessCallsSnapshot())
	}
	settingsSeen := fixture.factorySettingsSnapshot()
	if len(settingsSeen) != 2 || settingsSeen[0].OllamaModel != "translategemma:4b" || settingsSeen[1].OllamaModel != "translategemma:12b" {
		t.Fatalf("provider factory settings=%v, want TranslateGemma 4B then 12B", settingsSeen)
	}
	published := assetInfoPublishedFiles(t, root, mediaPath)
	var translated []byte
	for name, content := range published {
		if strings.Contains(name, ".es-419.") {
			translated = content
		}
	}
	if !strings.Contains(string(translated), "Hola v2") {
		t.Fatalf("retry output did not use current source: %q", translated)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(got), "안녕하세요 v2") {
		t.Fatalf("exact source changed after retry: err=%v content=%q", err, got)
	}
}

func TestAssetInfoTranslationRetryRejectsDisappearedExactSourceThroughHTTP(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	mediaPath := filepath.Join(root, "episode.mkv")
	sourcePath := filepath.Join(root, "episode.eng.srt")
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n")
	db := translationSourceTestDB(t, root, mediaPath)
	persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings("http://nas:11434"))
	fixture := newAssetInfoTranslationOllamaFixture()
	fixture.failures = []error{&TranslationProviderError{Class: translationProviderErrorUnavailable, Err: errors.New("first provider attempt")}}
	handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
	body := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":"en","targetLocale":"es-419","outputRepresentation":"srt"}`, mediaPath, sourcePath)
	status, responseBody := postAssetInfoTranslationStart(t, handler, body)
	if status != http.StatusAccepted {
		t.Fatalf("initial translation status=%d body=%s", status, responseBody)
	}
	var response struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	waitForSubtitleTranslationOperation(t, response.Operation.ID, "error")
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	before := subtitleTranslationOperationCountForTest()
	retryStatus, retryBody := postAssetInfoTranslationRetry(t, handler, response.Operation.ID)
	if retryStatus < http.StatusBadRequest || retryStatus >= http.StatusInternalServerError {
		t.Fatalf("missing exact source retry status=%d body=%s", retryStatus, retryBody)
	}
	if after := subtitleTranslationOperationCountForTest(); after != before {
		t.Fatalf("missing source retry created an operation: before=%d after=%d", before, after)
	}
}

func TestAssetInfoTranslationRetryRejectsRunningAndCompletedThroughHTTP(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	request := testSubtitleTranslationOperationRequest(filepath.Join(root, "episode.mkv"))
	release := make(chan struct{})
	runningID, err := startSubtitleTranslationOperation(request, func(ctx context.Context, _ string, _ string, _ subtitleTranslationOperationRequest) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(runningID)
	if status, body := postAssetInfoTranslationRetry(t, AssetHandler{}, runningID); status != http.StatusConflict {
		t.Fatalf("running retry status=%d body=%s, want %d", status, body, http.StatusConflict)
	}
	subtitleExtractionOperations.RLock()
	runningCancel := subtitleExtractionOperations.items[runningID].cancel
	subtitleExtractionOperations.RUnlock()
	runningCancel()
	waitForSubtitleTranslationOperation(t, runningID, "cancelled")

	completedID, err := startSubtitleTranslationOperation(request, func(context.Context, string, string, subtitleTranslationOperationRequest) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(completedID)
	waitForSubtitleTranslationOperation(t, completedID, "completed")
	if status, body := postAssetInfoTranslationRetry(t, AssetHandler{}, completedID); status != http.StatusConflict {
		t.Fatalf("completed retry status=%d body=%s, want %d", status, body, http.StatusConflict)
	}
	close(release)
}

func TestAssetInfoTranslationRetryAcceptsCancelledThroughHTTP(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	mediaPath := filepath.Join(root, "episode.mkv")
	sourcePath := filepath.Join(root, "episode.eng.srt")
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, sourcePath, "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n")
	db := translationSourceTestDB(t, root, mediaPath)
	persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings("http://nas:11434"))
	fixture := newAssetInfoTranslationOllamaFixture()
	fixture.failures = []error{context.Canceled}
	handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
	body := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":"en","targetLocale":"es-419","outputRepresentation":"srt"}`, mediaPath, sourcePath)
	status, responseBody := postAssetInfoTranslationStart(t, handler, body)
	if status != http.StatusAccepted {
		t.Fatalf("initial translation status=%d body=%s", status, responseBody)
	}
	var response struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	cancelled := waitForSubtitleTranslationOperation(t, response.Operation.ID, "cancelled")
	if cancelled.Translation == nil || !cancelled.Translation.Retryable {
		t.Fatalf("cancelled operation was not retryable: %#v", cancelled.Translation)
	}
	retryStatus, retryBody := postAssetInfoTranslationRetry(t, handler, response.Operation.ID)
	if retryStatus != http.StatusAccepted {
		t.Fatalf("cancelled retry status=%d body=%s", retryStatus, retryBody)
	}
	var retryResponse struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(retryBody), &retryResponse); err != nil {
		t.Fatal(err)
	}
	if retryResponse.Operation.ID == response.Operation.ID {
		t.Fatal("cancelled retry reused the predecessor identity")
	}
	waitForSubtitleTranslationOperation(t, retryResponse.Operation.ID, "completed")
}

func TestAssetInfoTranslationEntryPointEmbeddedSSAUsesCanonicalMuxer(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	mediaPath := filepath.Join(root, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	installTranslationSourceFFProbe(t, `{"streams":[{"index":4,"id":"ssa-4","codec_type":"subtitle","codec_name":"ssa","tags":{"language":"eng","title":"SSA"},"disposition":{}}]}`)
	argsPath := installAssetInfoTranslationFFmpeg(t, "[Script Info]\nTitle: Fixture\n\n[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}Hello\\Nworld\n")
	fixture := newAssetInfoTranslationOllamaFixture()
	db := translationSourceTestDB(t, root, mediaPath)
	persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings("http://nas:11434"))
	handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
	beforeSource, _ := os.ReadFile(mediaPath)
	operation := startAssetInfoTranslationAndWait(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"embedded","embedded":{"streamIndex":4}},"sourceLanguage":"en","targetLocale":"es-ES","outputRepresentation":"ssa"}`, mediaPath))
	if operation.Format != "ssa" {
		t.Fatalf("operation format=%q, want ssa", operation.Format)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-c:s\ncopy\n") || !strings.Contains(string(args), "-f\nass\n") {
		t.Fatalf("SSA extraction did not use canonical ass muxer: %q", args)
	}
	if got, err := os.ReadFile(mediaPath); err != nil || !bytes.Equal(got, beforeSource) {
		t.Fatalf("embedded source changed: err=%v", err)
	}
	published := assetInfoPublishedFiles(t, root, mediaPath)
	var output []byte
	for name, value := range published {
		if strings.HasSuffix(name, ".ssa") {
			output = value
		}
	}
	if !strings.Contains(string(output), "{\\an8}Hola\\Nworld") {
		t.Fatalf("translated SSA structure was not preserved: %q", output)
	}
	if fixture.callsSnapshot() != 1 {
		t.Fatalf("provider calls=%d, want 1", fixture.callsSnapshot())
	}
}

func TestAssetInfoTranslationEntryPointBitmapPGSAndVobSub(t *testing.T) {
	for _, testCase := range []struct {
		name, codec, streamID, requestedOutput string
		wantOutputs                            int
	}{
		{name: "PGS both", codec: "hdmv_pgs_subtitle", streamID: "pgs-4", requestedOutput: "both", wantOutputs: 2},
		{name: "VobSub SRT", codec: "dvd_subtitle", streamID: "vobsub-4", requestedOutput: "srt", wantOutputs: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "episode.mkv")
			writeTestFile(t, mediaPath, "media fixture")
			installTranslationSourceFFProbe(t, fmt.Sprintf(`{"streams":[{"index":4,"id":%q,"codec_type":"subtitle","codec_name":%q,"tags":{"language":"eng"},"disposition":{}}]}`, testCase.streamID, testCase.codec))
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			stream := FFProbeStream{Index: 4, ID: testCase.streamID, CodecType: "subtitle", CodecName: testCase.codec, Tags: map[string]string{"language": "eng"}}
			if testCase.codec == "hdmv_pgs_subtitle" {
				bin := installCanonicalOCRTestTools(t, "#!/bin/sh\noutput=\nfor argument do\n  case \"$argument\" in\n    --output-filename:*) output=\"${argument#--output-filename:}\" ;;\n  esac\ndone\nprintf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hello' > \"$output\"\n")
				installPGSSUPTestExtractor(t, bin)
			} else {
				cacheBitmapTranslationEvidence(t, mediaPath, stream, "eng", "clean", "eng", []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Hello", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
			}
			fixture := newAssetInfoTranslationOllamaFixture()
			db := translationSourceTestDB(t, root, mediaPath)
			persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings("http://nas:11434"))
			handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
			beforeSource, _ := os.ReadFile(mediaPath)
			operation := startAssetInfoTranslationAndWait(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"embedded","embedded":{"streamIndex":4}},"sourceLanguage":"en","targetLocale":"es-419","outputRepresentation":%q}`, mediaPath, testCase.requestedOutput))
			if operation.Kind != subtitleTranslationOperationKind {
				t.Fatalf("operation kind=%q, want translation", operation.Kind)
			}
			if fixture.callsSnapshot() != 1 {
				t.Fatalf("provider calls=%d, want one shared translation", fixture.callsSnapshot())
			}
			if operation.Result == nil || len(operation.Result.Artifacts) != testCase.wantOutputs {
				t.Fatalf("bitmap translation result=%#v, want %d published artifacts", operation.Result, testCase.wantOutputs)
			}
			for _, path := range fixture.pathsSnapshot() {
				if path != "/api/chat" {
					t.Fatalf("bitmap translation performed readiness preflight: %v", fixture.pathsSnapshot())
				}
			}
			if got, err := os.ReadFile(mediaPath); err != nil || !bytes.Equal(got, beforeSource) {
				t.Fatalf("bitmap source changed: err=%v", err)
			}
			published := assetInfoPublishedFiles(t, root, mediaPath)
			if len(published) != testCase.wantOutputs {
				t.Fatalf("published files=%v, want %d translated outputs", published, testCase.wantOutputs)
			}
			for name, value := range published {
				if !strings.Contains(name, ".es-419.") || !strings.Contains(string(value), "Hola") {
					t.Fatalf("unexpected bitmap publication %q=%q", name, value)
				}
			}
		})
	}
}

func TestAssetInfoTranslationBitmapAutoAmbiguityStopsBeforeProvider(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	installTranslationSourceFFProbe(t, `{"streams":[{"index":4,"id":"pgs-ambiguous","codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"und"},"disposition":{}}]}`)
	bin := installCanonicalOCRTestTools(t, `#!/bin/sh
output=
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' '123 !!!' > "$output"
`)
	installPGSSUPTestExtractor(t, bin)
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	db := translationSourceTestDB(t, root, mediaPath)
	persistTranslationSettingsFixture(t, db, readyAssetInfoTranslationSettings("http://nas:11434"))
	fixture := newAssetInfoTranslationOllamaFixture()
	handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
	beforeSource, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	status, responseBody := postAssetInfoTranslationStart(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"embedded","embedded":{"streamIndex":4}},"sourceLanguage":"auto","targetLocale":"es-419","outputRepresentation":"srt"}`, mediaPath))
	if status != http.StatusAccepted {
		t.Fatalf("translation start status=%d body=%s", status, responseBody)
	}
	var response struct {
		Operation subtitleExtractionOperation `json:"operation"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatal(err)
	}
	operation := waitForSubtitleTranslationOperation(t, response.Operation.ID, "error")
	if fixture.callsSnapshot() != 0 {
		t.Fatalf("ambiguous Auto source invoked provider %d times", fixture.callsSnapshot())
	}
	if operation.Status != "error" || operation.Translation == nil || operation.Translation.FailureClass != string(subtitleTranslationFailureTranslationFailed) {
		t.Fatalf("unexpected ambiguous Auto operation: %#v", operation)
	}
	if got, err := os.ReadFile(mediaPath); err != nil || !bytes.Equal(got, beforeSource) {
		t.Fatalf("bitmap source changed: err=%v", err)
	}
	if published := assetInfoPublishedFiles(t, root, mediaPath); len(published) != 0 {
		t.Fatalf("ambiguous Auto source published translated files: %v", published)
	}
}

func TestTranslationPublicationTargetsUseCanonicalArtifactType(t *testing.T) {
	request := subtitleTranslationOperationRequest{
		SourceIdentity:          "external:/media/episode.eng.forced.ass:revision-forced",
		SourceRevision:          "revision-forced",
		RequestedSourceLanguage: "en",
		TargetLocale:            "es-419",
		RequestedRepresentation: "ass",
		Provider:                translationProviderOllama,
		ProviderModel:           "fixture-model",
		ContextMode:             "none",
		ContextSource:           "none",
	}
	source := subtitleTranslationSourceResponse{
		Type:            "external",
		SourceIdentity:  request.SourceIdentity,
		Representation:  "ass",
		Codec:           "ass",
		Language:        "eng",
		ArtifactID:      "external-subtitle:forced",
		Title:           "Forced SDH",
		Default:         true,
		SourceForced:    true,
		EffectiveForced: true,
		Role:            SubtitleRoleSDH,
		RoleProvenance:  "classifier",
		RoleConfidence:  "high",
		RoleClassifier:  "classifier-v1",
	}
	targets, err := translationPublicationTargets(request, source, "en", TranslationProviderMetrics{}, "operation", map[string][]byte{"ass": []byte("fixture")}, filepath.Join(t.TempDir(), "episode.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("translation targets=%d, want 1", len(targets))
	}
	target := targets[0]
	artifact := target.Artifact
	if artifact.Type != "subtitle_sidecar" || artifact.Mode != "translation" || artifact.Language != "es-419" || artifact.Format != "ass" {
		t.Fatalf("translation artifact identity=%#v", artifact)
	}
	if artifact.Default != source.Default || artifact.Forced != source.EffectiveForced || artifact.SourceForced != source.SourceForced || artifact.Role != source.Role || artifact.RoleProvenance != source.RoleProvenance || artifact.RoleConfidence != source.RoleConfidence || artifact.RoleClassifierVersion != source.RoleClassifier || artifact.Title != source.Title {
		t.Fatalf("translation artifact lost source metadata: %#v", artifact)
	}
	if artifact.Translation == nil || artifact.Translation.SourceArtifactIdentity != source.SourceIdentity || artifact.Translation.SourceRevision != request.SourceRevision || artifact.Translation.SourceRepresentation != source.Representation || artifact.Translation.OutputRepresentation != "ass" {
		t.Fatalf("translation metadata is not bound to exact source: %#v", artifact.Translation)
	}
	if got, want := filepath.Base(target.FinalPath), "episode.es-419.eng.forced.sdh.ass"; got != want {
		t.Fatalf("translated path basename=%q, want %q", got, want)
	}
}
