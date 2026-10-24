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
	requestModels []string
	requests      []subtitleProofreadingProviderRequest
	mutate        func()
	resident      bool
	malformed     bool
	model         string
	suggestion    string
}

func newSubtitleOCRAITestServer(t *testing.T, model string) *subtitleOCRAITestServer {
	t.Helper()
	state := &subtitleOCRAITestServer{model: model, resident: true, suggestion: "Hello world"}
	state.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/ps":
			state.mu.Lock()
			state.processCalls++
			resident := state.resident
			state.mu.Unlock()
			models := []map[string]string{}
			if resident {
				models = append(models, map[string]string{"name": state.model, "model": state.model})
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"models": models})
		case "/api/chat":
			var chat ollamaChatRequest
			if err := json.NewDecoder(request.Body).Decode(&chat); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.chatCalls++
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
	if len(admitted) != 1 || admitted[0].ModelID != subtitleOCRAITestModel || admitted[0].Purpose != airesource.PurposeInference {
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
}

func TestSubtitleOCRAIRequiresResidentSelectedModelAndRejectsMalformedResponse(t *testing.T) {
	t.Run("model is not resident", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 2)
		server := newSubtitleOCRAITestServer(t, subtitleOCRAITestModel)
		server.resident = false
		db := newSubtitleOCRAITestDB(t, server.server.URL, true, subtitleOCRAITestModel)
		admission := &subtitleOCRAITestAdmission{}
		lifecycle := subtitleOCRAIEnabledLifecycle(db, server, admission, true)
		report := newSubtitleOCRAIFixtureReport(t, fixture)
		report, err := enrichSubtitleOCRReportWithAI(context.Background(), "ocr-ai-not-resident", report, fixture.target, db, lifecycle, server.server.Client())
		if err != nil || report.OCRAI == nil || report.OCRAI.State != "model_unavailable" {
			t.Fatalf("nonresident OCR model was not reported safely: report=%#v err=%v", report.OCRAI, err)
		}
		process, chat := server.counts()
		if process != 1 || chat != 0 || admission.releases != 1 {
			t.Fatalf("nonresident model reached inference or leaked admission: /api/ps=%d /api/chat=%d releases=%d", process, chat, admission.releases)
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
