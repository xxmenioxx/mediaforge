package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	mu             sync.Mutex
	paths          []string
	batches        []TranslationBatch
	calls          int
	factoryCalls   int
	readinessCalls int
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
	result := TranslationBatchResult{ContractVersion: batch.ContractVersion}
	for _, cue := range batch.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: strings.ReplaceAll(cue.Text, "Hello", "Hola")})
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
	return func(translationSettings) (TranslationProvider, error) {
		fixture.mu.Lock()
		fixture.factoryCalls++
		fixture.mu.Unlock()
		return fixture, nil
	}
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

func configureAssetInfoTranslationFixture(t *testing.T, dbPath, endpoint string) *gorm.DB {
	t.Helper()
	mediaPath := filepath.Join(dbPath, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	db := translationSourceTestDB(t, dbPath, mediaPath)
	if err := saveTranslationSettings(db, readyAssetInfoTranslationSettings(endpoint)); err != nil {
		t.Fatal(err)
	}
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
		name, extension, locale, expected string
	}{
		{name: "SRT", extension: "srt", locale: "es-419", expected: "Hola"},
		{name: "ASS", extension: "ass", locale: "es-ES", expected: "{\\an8}Hola{\\pos(10,20)}"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			mediaPath := filepath.Join(root, "episode.mkv")
			writeTestFile(t, mediaPath, "media fixture")
			sourcePath := filepath.Join(root, "episode.eng."+testCase.extension)
			content := "1\n00:00:01,000 --> 00:00:02,000\nHello\n\n"
			if testCase.extension == "ass" {
				content = "[Script Info]\nTitle: Fixture\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, AlphaLevel, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,1,2,0,2,10,10,10,0,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}Hello{\\pos(10,20)}\n"
			}
			writeTestFile(t, sourcePath, content)
			fixture := newAssetInfoTranslationOllamaFixture()
			db := translationSourceTestDB(t, root, mediaPath)
			if err := saveTranslationSettings(db, readyAssetInfoTranslationSettings("http://nas:11434")); err != nil {
				t.Fatal(err)
			}
			handler := AssetHandler{db: db, translationProviderFactory: fixture.factory()}
			request := fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}},"sourceLanguage":"en","targetLocale":%q,"outputRepresentation":%q}`, mediaPath, sourcePath, testCase.locale, testCase.extension)
			capabilityStatus, capabilityBody := postAssetInfoTranslationCapability(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, sourcePath))
			if capabilityStatus != http.StatusOK {
				t.Fatalf("capability status=%d body=%s", capabilityStatus, capabilityBody)
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
			if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, beforeSource) {
				t.Fatalf("source changed during translation: err=%v", err)
			}
			if fixture.callsSnapshot() != 1 {
				t.Fatalf("provider calls=%d, want 1", fixture.callsSnapshot())
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

func TestAssetInfoTranslationEntryPointEmbeddedSSAUsesCanonicalMuxer(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	mediaPath := filepath.Join(root, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	installTranslationSourceFFProbe(t, `{"streams":[{"index":4,"id":"ssa-4","codec_type":"subtitle","codec_name":"ssa","tags":{"language":"eng","title":"SSA"},"disposition":{}}]}`)
	argsPath := installAssetInfoTranslationFFmpeg(t, "[Script Info]\nTitle: Fixture\n\n[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}Hello\\Nworld\n")
	fixture := newAssetInfoTranslationOllamaFixture()
	db := translationSourceTestDB(t, root, mediaPath)
	if err := saveTranslationSettings(db, readyAssetInfoTranslationSettings("http://nas:11434")); err != nil {
		t.Fatal(err)
	}
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
				writeExecutable(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\noutput=\nfor argument do output=\"$argument\"; done\ncase \"$output\" in *.sup) : > \"$output\" ;; esac\n")
			} else {
				cacheBitmapTranslationEvidence(t, mediaPath, stream, "eng", "clean", "eng", []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Hello", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}})
			}
			fixture := newAssetInfoTranslationOllamaFixture()
			db := translationSourceTestDB(t, root, mediaPath)
			if err := saveTranslationSettings(db, readyAssetInfoTranslationSettings("http://nas:11434")); err != nil {
				t.Fatal(err)
			}
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
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\noutput=\nfor argument do output=\"$argument\"; done\ncase \"$output\" in *.sup) : > \"$output\" ;; esac\n")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	db := translationSourceTestDB(t, root, mediaPath)
	if err := saveTranslationSettings(db, readyAssetInfoTranslationSettings("http://nas:11434")); err != nil {
		t.Fatal(err)
	}
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
		SourceIdentity:          "external:/media/episode.eng.srt:revision",
		SourceRevision:          "revision",
		RequestedSourceLanguage: "en",
		TargetLocale:            "es-419",
		RequestedRepresentation: "srt",
		Provider:                translationProviderOllama,
		ProviderModel:           "fixture-model",
		ContextMode:             "none",
		ContextSource:           "none",
	}
	source := subtitleTranslationSourceResponse{Type: "external", SourceIdentity: request.SourceIdentity, Representation: "srt", Codec: "srt", ArtifactID: "external-subtitle:fixture"}
	targets, err := translationPublicationTargets(request, source, "en", TranslationProviderMetrics{}, "operation", map[string][]byte{"srt": []byte("fixture")}, filepath.Join(t.TempDir(), "episode.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Artifact.Type != "subtitle_sidecar" {
		t.Fatalf("translation artifact type=%q, want subtitle_sidecar", targets[0].Artifact.Type)
	}
}
