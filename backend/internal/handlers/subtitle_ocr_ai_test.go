package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const subtitleOCRAITestModel = "ocr-review-fixture:latest"

type subtitleOCRAITestAdmission struct {
	mu             sync.Mutex
	requests       []airesource.Request
	releases       int
	decisionStatus airesource.DecisionStatus
}

func (admission *subtitleOCRAITestAdmission) Admit(_ context.Context, request airesource.Request) airesource.Decision {
	admission.mu.Lock()
	defer admission.mu.Unlock()
	admission.requests = append(admission.requests, request)
	if admission.decisionStatus != "" && admission.decisionStatus != airesource.Admitted {
		return airesource.Decision{Status: admission.decisionStatus, Reason: "fixture_denied"}
	}
	return airesource.Decision{Status: airesource.Admitted, Lease: &airesource.Lease{}}
}

func (*subtitleOCRAITestAdmission) Await(context.Context, airesource.WaitHandle) airesource.Decision {
	return airesource.Decision{Status: airesource.Unavailable}
}

func (*subtitleOCRAITestAdmission) CancelWait(context.Context, airesource.WaitHandle) airesource.Decision {
	return airesource.Decision{Status: airesource.Denied}
}

func (admission *subtitleOCRAITestAdmission) Release(context.Context, airesource.Lease) error {
	admission.mu.Lock()
	defer admission.mu.Unlock()
	admission.releases++
	return nil
}

type subtitleOCRAITestServer struct {
	server        *httptest.Server
	mu            sync.Mutex
	chatCalls     int
	processCalls  int
	tagsCalls     int
	warmupCalls   int
	requestModels []string
	warmupModels  []string
	events        []string
	requests      []subtitleProofreadingProviderRequest
	mutate        func()
	resident      bool
	installed     bool
	warmupFailed  bool
	warmupStarted chan struct{}
	warmupRelease chan struct{}
	malformed     bool
	model         string
	suggestion    string
}

func newSubtitleOCRAITestServer(t *testing.T, model string) *subtitleOCRAITestServer {
	t.Helper()
	state := &subtitleOCRAITestServer{model: model, resident: true, installed: true, suggestion: "Hello world"}
	state.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/ps":
			state.mu.Lock()
			state.processCalls++
			state.events = append(state.events, "ps")
			resident := state.resident
			state.mu.Unlock()
			models := []map[string]string{}
			if resident {
				models = append(models, map[string]string{"name": state.model, "model": state.model})
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"models": models})
		case "/api/tags":
			state.mu.Lock()
			state.tagsCalls++
			state.events = append(state.events, "tags")
			installed := state.installed
			state.mu.Unlock()
			models := []map[string]string{}
			if installed {
				models = append(models, map[string]string{"name": state.model, "model": state.model})
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"models": models})
		case "/api/generate":
			var body map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			var model string
			_ = json.Unmarshal(body["model"], &model)
			if _, hasPrompt := body["prompt"]; hasPrompt {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.warmupCalls++
			state.warmupModels = append(state.warmupModels, model)
			state.events = append(state.events, "warmup")
			started, release, failed := state.warmupStarted, state.warmupRelease, state.warmupFailed
			state.mu.Unlock()
			if started != nil {
				select {
				case started <- struct{}{}:
				default:
				}
			}
			if release != nil {
				select {
				case <-release:
				case <-request.Context().Done():
					return
				}
			}
			if failed {
				writer.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(writer).Encode(map[string]string{"error": "fixture warmup failed"})
				return
			}
			state.mu.Lock()
			state.resident = true
			state.mu.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{"done": true})
		case "/api/chat":
			var chat ollamaChatRequest
			if err := json.NewDecoder(request.Body).Decode(&chat); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.chatCalls++
			state.events = append(state.events, "chat")
			state.requestModels = append(state.requestModels, chat.Model)
			state.mu.Unlock()
			if len(chat.Messages) != 1 {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			if state.mutate != nil {
				state.mutate()
			}
			if state.malformed {
				_ = json.NewEncoder(writer).Encode(map[string]any{"message": map[string]string{"content": "not-json"}})
				return
			}
			start := strings.LastIndex(chat.Messages[0].Content, "{\"contractVersion\"")
			if start < 0 {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			var proofreadRequest subtitleProofreadingProviderRequest
			if err := json.Unmarshal([]byte(chat.Messages[0].Content[start:]), &proofreadRequest); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.requests = append(state.requests, proofreadRequest)
			state.mu.Unlock()
			findings := make([]map[string]any, 0, len(proofreadRequest.Cues))
			for _, cue := range proofreadRequest.Cues {
				findings = append(findings, map[string]any{
					"cueId": cue.CueID, "sourceTextHash": cue.SourceHash, "status": "suggested",
					"suggestedText": state.suggestion, "category": "spelling", "reasonCode": "ocr-confusion",
					"reason": "A bounded OCR confusion may be present.",
				})
			}
			inner, _ := json.Marshal(map[string]any{
				"contractVersion": proofreadRequest.ContractVersion,
				"findings":        findings,
			})
			_ = json.NewEncoder(writer).Encode(map[string]any{"message": map[string]string{"content": string(inner)}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(state.server.Close)
	return state
}

func (state *subtitleOCRAITestServer) counts() (process, chat int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.processCalls, state.chatCalls
}

func (state *subtitleOCRAITestServer) captured() ([]string, []subtitleProofreadingProviderRequest) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]string(nil), state.requestModels...), append([]subtitleProofreadingProviderRequest(nil), state.requests...)
}

func (state *subtitleOCRAITestServer) warmupCounts() (tags, warmups int, models []string, events []string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.tagsCalls, state.warmupCalls, append([]string(nil), state.warmupModels...), append([]string(nil), state.events...)
}

func (state *subtitleOCRAITestServer) waitForWarmup(t *testing.T) {
	t.Helper()
	select {
	case <-state.warmupStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("model warmup did not begin")
	}
}

func newSubtitleOCRAITestDB(t *testing.T, endpoint string, enabled bool, model string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:ocr-ai-adapter-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	settings := []models.AppSetting{
		{Key: ocrAISettingsKey, Value: models.JSONMap{"enabled": enabled, "model": model}},
		{Key: ollamaProviderSettingsKey, Value: models.JSONMap{"endpoint": endpoint}},
		{Key: translationSettingsKey, Value: models.JSONMap{"provider": "ollama", "ollamaModel": "translation-only-model:latest"}},
	}
	for _, setting := range settings {
		if err := db.Create(&setting).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newSubtitleOCRAIFixtureReport(t *testing.T, fixture canonicalOCRReviewFixture) subtitleOCRQualityReport {
	t.Helper()
	_, _, snapshots, err := canonicalOCRReviewTargetSnapshots(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	report := analyzeCanonicalOCRSnapshot(snapshots[0])
	report.BaselineRevision = subtitleRevisionSetKey(report.RevisionSet)
	report.ReviewTarget = fixture.target
	return report
}

func subtitleOCRAIEnabledLifecycle(db *gorm.DB, server *subtitleOCRAITestServer, admission *subtitleOCRAITestAdmission, enabled bool) *AIModelLifecycle {
	manager := newAIModelLifecycle(db, admission, server.server.Client(), enabled, "ocr-ai-test-domain")
	return &AIModelLifecycle{manager: manager}
}

func TestSubtitleOCRAIUsesExplicitModelAndLeavesCanonicalApprovalFlowUnchanged(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
	fixture.evidence.CorrectionProvenance = []canonicalOCRCorrectionProvenance{{
		CueID: 1, Action: "apply", CandidateSource: "contextual-ocr-dictionary", TextHash: boundedTextHash("Hello|world"),
		BeforeTextHash: boundedTextHash("Hello|world"), AfterTextHash: boundedTextHash("Hello|world"),
		DictionaryLanguage: "eng", DictionaryRevision: "dictionary-revision-262", OperationID: "dictionary-operation"}}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), fixture.sidecar, "srt", fixture.target.LogicalTransformKey, fixture.evidence, "dictionary-correction", ""); err != nil {
		t.Fatal(err)
	}
	selection, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request)
	if err != nil || !found || selection.ReviewTarget == nil {
		t.Fatalf("canonical OCR target could not be reselected after dictionary analysis: found=%v err=%v", found, err)
	}
	fixture.target = selection.ReviewTarget
	server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
	db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
	admission := &subtitleOCRAITestAdmission{}
	lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
	beforeSidecar, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, ok := loadPublishedCanonicalOCRSnapshot(fixture.sidecar, "srt", fixture.target.LogicalTransformKey)
	if !ok {
		t.Fatal("canonical OCR snapshot is unavailable")
	}
	beforeProvenance := append([]canonicalOCRCorrectionProvenance(nil), beforeSnapshot.Evidence.CorrectionProvenance...)

	report := newSubtitleOCRAIFixtureReport(t, fixture)
	report, err = enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-review", report, fixture.target, db, lifecycle, server.server.Client())
	if err != nil {
		t.Fatalf("AI review failed unexpectedly: %v", err)
	}
	if report.OCRAI == nil || report.OCRAI.State != "suggestions_ready" || report.OCRAI.Model != subtitleOCRAITestModel || report.OCRAI.SuggestedCount != 1 {
		t.Fatalf("explicit OCR model did not produce a bounded review suggestion: %#v", report.OCRAI)
	}
	suggestion := report.Findings[0].AISuggestion
	if suggestion == nil || suggestion.CueID != 1 || suggestion.OriginalHash != subtitleProofreadingTextHash("Hello|world") ||
		suggestion.ProposedText != "Hello world" || suggestion.Model != subtitleOCRAITestModel ||
		suggestion.BaselineRevision != fixture.target.BaselineRevision || strings.TrimSpace(suggestion.Reason) == "" {
		t.Fatalf("AI suggestion was not bound to the exact cue, hash, model, and revision: %#v", suggestion)
	}
	models, requests := server.captured()
	if len(models) != 1 || models[0] != subtitleOCRAITestModel || len(requests) != 1 ||
		len(requests[0].Cues) != 1 || requests[0].Cues[0].CueID != "ocr-cue-1" ||
		requests[0].Cues[0].SourceHash != suggestion.OriginalHash {
		t.Fatalf("provider did not receive only the exact suspicious cue on the explicit OCR model: models=%v requests=%#v", models, requests)
	}
	admission.mu.Lock()
	admitted := append([]airesource.Request(nil), admission.requests...)
	admission.mu.Unlock()
	if len(admitted) != 1 || admitted[0].ModelID != subtitleOCRAITestModel || admitted[0].Purpose != airesource.PurposeInference ||
		admitted[0].ConsumerID != aiModelConsumerSubtitleOCR {
		t.Fatalf("resource admission did not fence inference to the selected OCR model: %#v", admitted)
	}
	afterSidecar, err := os.ReadFile(fixture.sidecar)
	if err != nil || string(afterSidecar) != string(beforeSidecar) {
		t.Fatalf("AI suggestion mutated the canonical sidecar: %v", err)
	}
	afterSnapshot, ok := loadPublishedCanonicalOCRSnapshot(fixture.sidecar, "srt", fixture.target.LogicalTransformKey)
	if !ok || !reflect.DeepEqual(afterSnapshot.Evidence.CorrectionProvenance, beforeProvenance) {
		t.Fatalf("AI suggestion changed canonical OCR approval provenance: %#v", afterSnapshot)
	}

	provider := &bitmapTranslationProviderSpy{}
	runner := func(ctx context.Context, operationID, _ string, _ subtitleTranslationOperationRequest) error {
		_, executionErr := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
			MediaPath: fixture.mediaPath, Stream: fixture.stream, RequestedLanguage: "en", OCRLanguage: "eng", OCRMode: "accurate",
			SourceRepresentation: "pgs", TargetLocale: "es-419", RequestedRepresentation: "srt",
			Provider: provider, Settings: bitmapTranslationExecutionTestSettings(), OperationID: operationID,
		})
		return executionErr
	}
	translationID, err := startSubtitleTranslationOperation(testSubtitleTranslationOperationRequest(fixture.mediaPath), runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(translationID)
	failed := waitForSubtitleTranslationOperation(t, translationID, "error")
	if failed.Translation == nil || failed.Translation.OCRReviewTarget == nil || failed.Translation.OCRReviewTarget.State != "exact" || provider.calls != 0 {
		t.Fatalf("translation did not keep using the canonical approval gate: %#v calls=%d", failed.Translation, provider.calls)
	}
	correctionID, err := startSubtitleOCRCorrectionOperation(subtitleOCRCorrectionInput{
		MediaPath: fixture.mediaPath, SidecarPath: fixture.sidecar, SidecarPaths: failed.Translation.OCRReviewTarget.SidecarPaths,
		Format: "srt", LogicalTransformKey: failed.Translation.OCRReviewTarget.LogicalTransformKey,
		BaselineRevision: failed.Translation.OCRReviewTarget.BaselineRevision,
		Revisions:        failed.Translation.OCRReviewTarget.Revisions, ReviewTarget: failed.Translation.OCRReviewTarget,
		Corrections: []subtitleOCRCorrection{{CueID: 1, Action: "approve"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(correctionID)
	waitForSubtitleOperationStatus(t, correctionID, "completed")
	retryID, err := retrySubtitleTranslationOperation(translationID, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(retryID)
	completed := waitForSubtitleTranslationOperation(t, retryID, "completed")
	if completed.Phase != "completed" || provider.calls != 1 {
		t.Fatalf("Translation retry did not consume the separately approved canonical OCR cue: phase=%s calls=%d", completed.Phase, provider.calls)
	}
}

func TestSubtitleOCRAIDisabledAndAdmissionUnavailableSendNoModelRequests(t *testing.T) {
	t.Run("disabled by default even when Translation has a model", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		db := newSubtitleOCRAITestDB(t, server.server.URL, false, "")
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-disabled", report, fixture.target, db, nil, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "disabled" {
			t.Fatalf("disabled OCR AI was not kept off: report=%#v err=%v", report.OCRAI, err)
		}
		process, chat := server.counts()
		if process != 0 || chat != 0 {
			t.Fatalf("disabled OCR AI made provider requests: /api/ps=%d /api/chat=%d", process, chat)
		}
	})
	t.Run("resource admission unavailable", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-no-admission", report, fixture.target, db, nil, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "resource_unavailable" {
			t.Fatalf("unavailable admission did not fail closed: report=%#v err=%v", report.OCRAI, err)
		}
		process, chat := server.counts()
		if process != 0 || chat != 0 {
			t.Fatalf("missing admission gate made provider requests: /api/ps=%d /api/chat=%d", process, chat)
		}
	})
	t.Run("model-load admission denied", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{decisionStatus: airesource.Denied}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-denied-load", report, fixture.target, db, lifecycle, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "resource_unavailable" || report.Findings[0].AISuggestion != nil {
			t.Fatalf("denied model-load admission was not a nonfatal fail-closed result: report=%#v err=%v", report.OCRAI, err)
		}
		_, chat := server.counts()
		_, warmups, _, _ := server.warmupCounts()
		admission.mu.Lock()
		requests := append([]airesource.Request(nil), admission.requests...)
		admission.mu.Unlock()
		if warmups != 0 || chat != 0 || len(requests) != 1 || requests[0].Purpose != airesource.PurposeModelLoad {
			t.Fatalf("denied model-load admission allowed warmup or inference: warmups=%d chat=%d requests=%#v", warmups, chat, requests)
		}
	})
}

func TestSubtitleOCRAIColdStartsSelectedModelAndRejectsMalformedResponse(t *testing.T) {
	t.Run("model is installed but not resident", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-not-resident", report, fixture.target, db, lifecycle, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "suggestions_ready" || report.Findings[0].AISuggestion == nil {
			t.Fatalf("installed nonresident OCR model did not cold-start into suggestions: report=%#v err=%v", report.OCRAI, err)
		}
		process, chat := server.counts()
		tags, warmups, warmupModels, events := server.warmupCounts()
		if process < 2 || tags != 1 || warmups != 1 || chat != 1 || len(warmupModels) != 1 ||
			warmupModels[0] != subtitleOCRAITestModel || admission.releases != 2 {
			t.Fatalf("cold-start lifecycle did not warm then infer with released leases: /api/ps=%d /api/tags=%d /api/generate=%d /api/chat=%d warmupModels=%v releases=%d", process, tags, warmups, chat, warmupModels, admission.releases)
		}
		warmupIndex, chatIndex := -1, -1
		for i, event := range events {
			if event == "warmup" && warmupIndex < 0 {
				warmupIndex = i
			}
			if event == "chat" && chatIndex < 0 {
				chatIndex = i
			}
		}
		if warmupIndex < 0 || chatIndex <= warmupIndex {
			t.Fatalf("inference chat did not follow model warmup: %v", events)
		}
		admission.mu.Lock()
		admitted := append([]airesource.Request(nil), admission.requests...)
		admission.mu.Unlock()
		if len(admitted) != 2 || admitted[0].Purpose != airesource.PurposeModelLoad ||
			admitted[1].Purpose != airesource.PurposeInference || admitted[0].ConsumerID != aiModelConsumerSubtitleOCR ||
			admitted[1].ConsumerID != aiModelConsumerSubtitleOCR || admitted[0].ModelID != subtitleOCRAITestModel ||
			admitted[1].ModelID != subtitleOCRAITestModel {
			t.Fatalf("model-load and inference admission were not attributed to OCR AI: %#v", admitted)
		}
	})
	t.Run("warmup failure does not infer or attach suggestions", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		server.warmupFailed = true
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-warmup-failed", report, fixture.target, db, lifecycle, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "warmup_failed" || report.Findings[0].AISuggestion != nil {
			t.Fatalf("failed warmup was not reported without suggestions: report=%#v err=%v", report.OCRAI, err)
		}
		_, chat := server.counts()
		_, warmups, _, _ := server.warmupCounts()
		if warmups != 1 || chat != 0 || admission.releases != 1 {
			t.Fatalf("failed warmup reached inference or leaked the load lease: warmups=%d chat=%d releases=%d", warmups, chat, admission.releases)
		}
	})
	t.Run("malformed response", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.malformed = true
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-malformed", report, fixture.target, db, lifecycle, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "provider_failed" || report.Findings[0].AISuggestion != nil {
			t.Fatalf("malformed model response was not discarded: report=%#v err=%v", report, err)
		}
	})
}

func TestSubtitleOCRAIRevalidatesAfterWarmupAndHonorsCancellation(t *testing.T) {
	t.Run("cancel during warmup", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		server.warmupStarted = make(chan struct{}, 1)
		server.warmupRelease = make(chan struct{})
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fixtureReport := newSubtitleOCRAIFixtureReport(t, fixture)
		type result struct {
			report subtitleOCRQualityReport
			err    error
		}
		completed := make(chan result, 1)
		go func() {
			report, err := enrichSubtitleOCRReportWithAI(ctx, "ocr-ai-cancel-warmup", fixtureReport, fixture.target, db, lifecycle, server.server.Client())
			completed <- result{report: report, err: err}
		}()
		server.waitForWarmup(t)
		cancel()
		got := <-completed
		select {
		case <-server.warmupRelease:
		default:
			close(server.warmupRelease)
		}
		if got.err != nil || got.report.OCRAI == nil || got.report.OCRAI.State != "cancelled" || got.report.Findings[0].AISuggestion != nil {
			t.Fatalf("cancelled warmup produced inference or suggestions: state=%#v suggestion=%#v err=%v", got.report.OCRAI, got.report.Findings[0].AISuggestion, got.err)
		}
		_, chat := server.counts()
		if chat != 0 {
			t.Fatalf("cancelled model warmup issued %d chat requests", chat)
		}
	})
	t.Run("source revision changes during warmup", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		server.warmupStarted = make(chan struct{}, 1)
		server.warmupRelease = make(chan struct{})
		var releaseOnce sync.Once
		releaseWarmup := func() { releaseOnce.Do(func() { close(server.warmupRelease) }) }
		defer releaseWarmup()
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		fixtureReport := newSubtitleOCRAIFixtureReport(t, fixture)
		type result struct {
			report subtitleOCRQualityReport
			err    error
		}
		completed := make(chan result, 1)
		go func() {
			report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-source-changed-warmup", fixtureReport, fixture.target, db, lifecycle, server.server.Client())
			completed <- result{report: report, err: err}
		}()
		server.waitForWarmup(t)
		if err := os.WriteFile(fixture.mediaPath, []byte("changed protected source during warmup"), 0o600); err != nil {
			t.Fatal(err)
		}
		releaseWarmup()
		got := <-completed
		if !errors.Is(got.err, errSubtitleOCRAIStaleTarget) || got.report.OCRAI == nil || got.report.OCRAI.State != "stale" || got.report.Findings[0].AISuggestion != nil {
			t.Fatalf("source change during model warmup was not fenced: state=%#v suggestion=%#v err=%v", got.report.OCRAI, got.report.Findings[0].AISuggestion, got.err)
		}
		_, chat := server.counts()
		if chat != 0 {
			t.Fatalf("stale source reached inference with %d chat requests", chat)
		}
	})
	t.Run("OCR model setting changes during warmup", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		server.warmupStarted = make(chan struct{}, 1)
		server.warmupRelease = make(chan struct{})
		var releaseOnce sync.Once
		releaseWarmup := func() { releaseOnce.Do(func() { close(server.warmupRelease) }) }
		defer releaseWarmup()
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		fixtureReport := newSubtitleOCRAIFixtureReport(t, fixture)
		type result struct {
			report subtitleOCRQualityReport
			err    error
		}
		completed := make(chan result, 1)
		go func() {
			report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-settings-changed-warmup", fixtureReport, fixture.target, db, lifecycle, server.server.Client())
			completed <- result{report: report, err: err}
		}()
		server.waitForWarmup(t)
		if err := db.Model(&models.AppSetting{}).Where("key = ?", ocrAISettingsKey).
			Update("value", models.JSONMap{"enabled": true, "model": "replacement-ocr-model:latest"}).Error; err != nil {
			t.Fatal(err)
		}
		releaseWarmup()
		got := <-completed
		if got.err != nil || got.report.OCRAI == nil || got.report.OCRAI.State != "configuration_changed" || got.report.Findings[0].AISuggestion != nil {
			t.Fatalf("OCR model change during warmup was not fenced: state=%#v suggestion=%#v err=%v", got.report.OCRAI, got.report.Findings[0].AISuggestion, got.err)
		}
		_, chat := server.counts()
		if chat != 0 {
			t.Fatalf("settings change during warmup reached inference with %d chat requests", chat)
		}
	})
}

func TestSubtitleOCRAIInvalidatesResultsWhenCanonicalTargetChanges(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(canonicalOCRReviewFixture)
	}{
		{name: "sidecar revision", mutate: func(fixture canonicalOCRReviewFixture) {
			if err := os.WriteFile(fixture.sidecar, []byte("1\n00:00:01,000 --> 00:00:02,000\nChanged cue text\n"), 0o600); err != nil {
				panic(err)
			}
		}},
		{name: "source revision", mutate: func(fixture canonicalOCRReviewFixture) {
			if err := os.WriteFile(fixture.mediaPath, []byte("changed protected source with a different size"), 0o600); err != nil {
				panic(err)
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
			server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
			server.mutate = func() { testCase.mutate(fixture) }
			db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
			admission := &subtitleOCRAITestAdmission{}
			lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
			report := newSubtitleOCRAIFixtureReport(t, fixture)
			report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-stale", report, fixture.target, db, lifecycle, server.server.Client())
			if !errors.Is(err, errSubtitleOCRAIStaleTarget) || report.OCRAI == nil ||
				report.OCRAI.State != "stale" || report.Findings[0].AISuggestion != nil {
				t.Fatalf("stale source/revision was not invalidated: report=%#v suggestion=%#v err=%v", report.OCRAI, report.Findings[0].AISuggestion, err)
			}
		})
	}
}
