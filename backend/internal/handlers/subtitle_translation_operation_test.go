package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func testSubtitleTranslationOperationRequest(path string) subtitleTranslationOperationRequest {
	return subtitleTranslationOperationRequest{
		AssetPath:               path,
		StreamIndex:             4,
		Format:                  "srt",
		SourceIdentity:          "embedded:4:source",
		SourceReference:         "stream:4",
		RequestedSourceLanguage: "en",
		TargetLocale:            "es-419",
		RequestedRepresentation: "srt",
		Provider:                "ollama",
		ProviderModel:           "qwen2.5:7b",
		ContextMode:             "bounded",
		ContextSource:           "asset-metadata",
		SourceRevision:          "revision-1",
	}
}

func deleteSubtitleTranslationOperation(id string) {
	subtitleExtractionOperations.Lock()
	delete(subtitleExtractionOperations.items, id)
	subtitleExtractionOperations.Unlock()
}

func waitForSubtitleTranslationOperation(t *testing.T, id, status string) subtitleExtractionOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := snapshotSubtitleOperation(id)
		if snapshot.Status == status {
			return snapshot
		}
		if snapshot.Status == "error" && status != "error" {
			t.Fatalf("translation operation failed while waiting for %q: %#v", status, snapshot)
		}
		if snapshot.Status == "cancelled" && status != "cancelled" {
			t.Fatalf("translation operation was cancelled while waiting for %q: %#v", status, snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for translation operation %q to become %q", id, status)
	return subtitleExtractionOperation{}
}

func TestSubtitleTranslationOperationLifecycleAndPolling(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	request := testSubtitleTranslationOperationRequest(filepath.Join(root, "movie.mkv"))
	runnerStarted := make(chan string, 1)
	progressSeen := make(chan subtitleExtractionOperation, 1)
	release := make(chan struct{})
	runner := func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
		if err := os.WriteFile(filepath.Join(workspace, "owned-work"), []byte("bounded"), 0o600); err != nil {
			return err
		}
		runnerStarted <- workspace
		updateSubtitleTranslationProgress(operationID, "translating", 1, 3, "Translating subtitle batches")
		progressSeen <- snapshotSubtitleOperation(operationID)
		updateSubtitleTranslationProgress(operationID, "preparing", 0, 3, "must not regress")
		<-release
		updateSubtitleTranslationProgress(operationID, "validating", 3, 3, "Validating translated subtitles")
		return nil
	}

	operationID, err := startSubtitleTranslationOperation(request, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	initial := snapshotSubtitleOperation(operationID)
	if initial.ID != operationID || initial.Kind != subtitleTranslationOperationKind || initial.Status != "running" || initial.Phase != "preparing" {
		t.Fatalf("unexpected translation operation creation: %#v", initial)
	}
	if initial.Progress < 0 || initial.Progress > 100 || initial.translationRequest == nil || initial.Translation == nil || !initial.Translation.Cancellable || initial.Translation.Retryable {
		t.Fatalf("translation operation did not retain bounded request/progress: %#v", initial)
	}

	workspace := <-runnerStarted
	progress := <-progressSeen
	if progress.Phase != "translating" || progress.Processed != 1 || progress.Total != 3 || progress.Progress < initial.Progress {
		t.Fatalf("unexpected translation progress: %#v", progress)
	}
	close(release)
	completed := waitForSubtitleTranslationOperation(t, operationID, "completed")
	if completed.Phase != "completed" || completed.Progress != 100 || completed.Processed != 3 || completed.Total != 3 || completed.Translation == nil || completed.Translation.Cancellable || completed.Translation.Retryable {
		t.Fatalf("unexpected completed translation operation: %#v", completed)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("translation workspace survived successful cleanup: %q (%v)", workspace, err)
	}

	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Message = "illegal terminal mutation"
	})
	if snapshot := snapshotSubtitleOperation(operationID); snapshot.Message == "illegal terminal mutation" {
		t.Fatal("terminal translation operation was mutated")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/assets/subtitle-operations", (&AssetHandler{}).ListSubtitleExtractionOperations)
	router.GET("/api/assets/subtitle-operations/:id", (&AssetHandler{}).GetSubtitleExtractionOperation)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/assets/subtitle-operations?path="+request.AssetPath, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"kind":"translation"`) {
		t.Fatalf("translation operation was not listable: status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/assets/subtitle-operations/"+operationID, nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"completed"`) {
		t.Fatalf("translation operation was not fetchable: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSubtitleTranslationOperationFailureCleanupAndRetry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	request := testSubtitleTranslationOperationRequest(filepath.Join(root, "movie.mkv"))
	errorText := strings.Repeat("provider diagnostic ", 300)
	failedWorkspace := make(chan string, 1)
	failingRunner := func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
		failedWorkspace <- workspace
		return errors.New(errorText)
	}

	operationID, err := startSubtitleTranslationOperation(request, failingRunner)
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForSubtitleTranslationOperation(t, operationID, "error")
	defer deleteSubtitleTranslationOperation(operationID)
	workspace := <-failedWorkspace
	if failed.Phase != "preparing" || failed.Error == "" || len(failed.Error) > maxSubtitleTranslationOperationError+3 {
		t.Fatalf("unexpected bounded translation failure: %#v", failed)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("translation workspace survived failure cleanup: %q (%v)", workspace, err)
	}

	retryStarted := make(chan struct{}, 1)
	retryRunner := func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
		retryStarted <- struct{}{}
		updateSubtitleTranslationProgress(operationID, "translating", 1, 1, "Translating subtitle batch")
		return nil
	}
	retryID, err := retrySubtitleTranslationOperation(operationID, retryRunner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(retryID)
	if retryID == operationID {
		t.Fatal("retry reused the terminal translation operation identity")
	}
	<-retryStarted
	retried := waitForSubtitleTranslationOperation(t, retryID, "completed")
	if retried.Progress != 100 || snapshotSubtitleOperation(operationID).Status != "error" {
		t.Fatalf("retry changed terminal predecessor or did not complete: old=%#v new=%#v", snapshotSubtitleOperation(operationID), retried)
	}
	if _, err := retrySubtitleTranslationOperation(retryID, retryRunner); err == nil {
		t.Fatal("completed translation operation was unexpectedly retryable")
	}
}

func TestSubtitleTranslationOperationCancellationAndRetry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	request := testSubtitleTranslationOperationRequest(filepath.Join(root, "movie.mkv"))
	runnerStarted := make(chan string, 1)
	runnerCancelled := make(chan struct{}, 1)
	runner := func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
		runnerStarted <- workspace
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.Canceled) {
			runnerCancelled <- struct{}{}
		}
		return ctx.Err()
	}

	operationID, err := startSubtitleTranslationOperation(request, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	workspace := <-runnerStarted
	if _, err := retrySubtitleTranslationOperation(operationID, runner); err == nil {
		t.Fatal("running translation operation was unexpectedly retryable")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/subtitle-operations/:id/cancel", (&AssetHandler{}).CancelSubtitleExtractionOperation)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+operationID+"/cancel", nil))
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("translation cancellation returned status=%d body=%s", response.Code, response.Body.String())
	}
	<-runnerCancelled
	cancelled := waitForSubtitleTranslationOperation(t, operationID, "cancelled")
	if cancelled.Error != "" || cancelled.Phase != "cleanup" {
		t.Fatalf("cancellation was not terminal/cancellation-safe: %#v", cancelled)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("translation workspace survived cancellation cleanup: %q (%v)", workspace, err)
	}

	retryID, err := retrySubtitleTranslationOperation(operationID, func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(retryID)
	if retryID == operationID {
		t.Fatal("cancelled translation retry reused the old identity")
	}
	waitForSubtitleTranslationOperation(t, retryID, "completed")
	if old := snapshotSubtitleOperation(operationID); old.Status != "cancelled" {
		t.Fatalf("cancelled predecessor changed after retry: %#v", old)
	}
}

func TestSubtitleTranslationOperationRequestIsBounded(t *testing.T) {
	request := testSubtitleTranslationOperationRequest("/tmp/movie.mkv")
	request.ProviderModel = strings.Repeat("m", maxSubtitleTranslationRequestField+1)
	_, err := startSubtitleTranslationOperation(request, func(context.Context, string, string, subtitleTranslationOperationRequest) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "provider model") {
		t.Fatalf("oversized translation request was not rejected: %v", err)
	}
	if strings.Contains(fmt.Sprint(err), "secret") {
		t.Fatal("bounded request validation exposed an unrelated secret")
	}
}

func TestSubtitleTranslationOperationFailureViewIsTypedAndSecretSafe(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		status       string
		failureClass subtitleTranslationFailureClass
		diagnostic   string
		runningPhase string
		wantPhase    string
	}{
		{name: "unavailable", err: &TranslationProviderError{Class: translationProviderErrorUnavailable, Err: errors.New("SUPER_SECRET_TOKEN_123")}, status: "error", failureClass: subtitleTranslationFailureProviderUnavailable, diagnostic: "Translation provider is unavailable.", runningPhase: "translating", wantPhase: "translating"},
		{name: "timeout", err: &TranslationProviderError{Class: translationProviderErrorTimeout, Err: errors.New("PRIVATE_PROVIDER_RESPONSE_456")}, status: "error", failureClass: subtitleTranslationFailureProviderTimeout, diagnostic: "Translation provider request timed out.", runningPhase: "translating", wantPhase: "translating"},
		{name: "configuration", err: &TranslationProviderError{Class: translationProviderErrorConfiguration, Err: errors.New("Authorization: Bearer SUPER_SECRET_TOKEN_123")}, status: "error", failureClass: subtitleTranslationFailureProviderConfiguration, diagnostic: "Translation provider configuration is invalid.", runningPhase: "translating", wantPhase: "translating"},
		{name: "malformed", err: malformedTranslationResultError("provider response PRIVATE_PROVIDER_RESPONSE_456"), status: "error", failureClass: subtitleTranslationFailureMalformedResponse, diagnostic: "Translation provider returned a malformed response.", runningPhase: "translating", wantPhase: "translating"},
		{name: "stale source", err: staleTranslationSourceError("source changed at /private/source"), status: "error", failureClass: subtitleTranslationFailureStaleSource, diagnostic: "Subtitle source changed before publication.", runningPhase: "validating", wantPhase: "validating"},
		{name: "OCR review required", err: subtitleOCRReviewRequiredError(errors.New("canonical OCR cue 12 requires review: private path /workspace/private")), status: "error", failureClass: subtitleTranslationFailureOCRReviewRequired, diagnostic: "OCR review is required before translation.", runningPhase: "ocr", wantPhase: "ocr_review"},
		{name: "OCR failed", err: subtitleOCRFailedError(errors.New("OCR command failed at /workspace/private")), status: "error", failureClass: subtitleTranslationFailureOCRFailed, diagnostic: "Canonical OCR could not be prepared for translation.", runningPhase: "ocr", wantPhase: "ocr"},
		{name: "cancelled", err: context.Canceled, status: "cancelled", failureClass: subtitleTranslationFailureCancelled, diagnostic: "Subtitle translation cancelled."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			request := testSubtitleTranslationOperationRequest(filepath.Join(root, "movie.mkv"))
			var operationID string
			operationID, err := startSubtitleTranslationOperation(request, func(_ context.Context, runningOperationID, _ string, _ subtitleTranslationOperationRequest) error {
				if test.runningPhase != "" {
					updateSubtitleTranslationProgress(runningOperationID, test.runningPhase, 0, 0, "Running test stage")
				}
				return test.err
			})
			if err != nil {
				t.Fatal(err)
			}
			defer deleteSubtitleTranslationOperation(operationID)
			operation := waitForSubtitleTranslationOperation(t, operationID, test.status)
			if operation.Translation == nil || operation.Translation.FailureClass != string(test.failureClass) || operation.Translation.Diagnostic != test.diagnostic || !operation.Translation.Retryable || operation.Translation.Cancellable {
				t.Fatalf("unexpected translation view: %#v", operation.Translation)
			}
			if test.wantPhase != "" && operation.Phase != test.wantPhase {
				t.Fatalf("terminal phase=%q, want %q", operation.Phase, test.wantPhase)
			}
			if test.name == "malformed" && (operation.Progress != 40 || operation.Processed != 0 || operation.Total != 0) {
				t.Fatalf("malformed failure reported phase progress as cue completion: progress=%v processed=%d total=%d", operation.Progress, operation.Processed, operation.Total)
			}
			encoded, err := json.Marshal(operation)
			if err != nil {
				t.Fatal(err)
			}
			serialized := string(encoded)
			for _, secret := range []string{"SUPER_SECRET_TOKEN_123", "PRIVATE_PROVIDER_RESPONSE_456", "Authorization: Bearer", "/private/source", "/workspace/private"} {
				if strings.Contains(serialized, secret) {
					t.Fatalf("serialized operation leaked %q: %s", secret, serialized)
				}
			}
		})
	}
}

func TestSubtitleTranslationOperationIncludesSanitizedBatchDiagnostics(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	request := testSubtitleTranslationOperationRequest(filepath.Join(root, "movie.mkv"))
	expected, actual := 3, 2
	providerErr := &TranslationProviderError{Class: translationProviderErrorMalformed, Err: &translationResponseDiagnosticError{
		Category: translationResponseDiagnosticCueCountMismatch, ExpectedCueCount: &expected, ActualCueCount: &actual,
		Cause: errors.New("PRIVATE_SUBTITLE_TEXT /private/source.mkv"),
	}}
	wrapped := withTranslationBatchDiagnostic(providerErr, 1, 4, expected)
	operationID, startErr := startSubtitleTranslationOperation(request, func(_ context.Context, _ string, _ string, _ subtitleTranslationOperationRequest) error {
		return wrapped
	})
	if startErr != nil {
		t.Fatal(startErr)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	operation := waitForSubtitleTranslationOperation(t, operationID, "error")
	if operation.Translation == nil || operation.Translation.ProviderDiagnostics == nil {
		t.Fatalf("sanitized provider diagnostics missing: %#v", operation.Translation)
	}
	diagnostics := operation.Translation.ProviderDiagnostics
	if diagnostics.Category != translationResponseDiagnosticCueCountMismatch || diagnostics.BatchIndex != 2 || diagnostics.BatchCount != 4 || diagnostics.ExpectedCueCount == nil || *diagnostics.ExpectedCueCount != expected || diagnostics.ActualCueCount == nil || *diagnostics.ActualCueCount != actual {
		t.Fatalf("unexpected provider diagnostics: %#v", diagnostics)
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE_SUBTITLE_TEXT", "/private/source.mkv"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("serialized operation leaked %q: %s", secret, encoded)
		}
	}
}

func TestOllamaMalformedInnerJSONReportsOriginalTranslationFailureState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOllamaTestResponse(t, w, "MODEL_RESPONSE_SENTINEL", 0, 0, 0)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "translategemma:12b"))
	if err != nil {
		t.Fatal(err)
	}

	sourceContent := []byte("[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}ASS_CUE_SENTINEL{\\i0}\n")
	originalSource := append([]byte(nil), sourceContent...)
	request := testSubtitleTranslationOperationRequest(filepath.Join(root, "source.mkv"))
	request.StreamIndex = -1
	request.Format = "ass"
	request.SourceIdentity = "synthetic-ass-source"
	request.SourceReference = "external:synthetic-ass"
	request.RequestedRepresentation = "ass"
	request.Provider = translationProviderOllama
	request.ProviderModel = "translategemma:12b"
	request.ContextMode = "none"
	request.ContextSource = "none"

	source := subtitleTranslationSourceResponse{Representation: "ass", Language: "en"}
	execution := subtitleTranslationExecutionResult{}
	operationID, err := startSubtitleTranslationOperation(request, func(ctx context.Context, operationID, _ string, operationRequest subtitleTranslationOperationRequest) error {
		updateSubtitleTranslationProgress(operationID, "translating", 0, 0, "Translating subtitle cues")
		var executeErr error
		execution, executeErr = executeTextTranslation(ctx, source, sourceContent, operationRequest, provider, defaultTranslationSettings(), nil, nil)
		return executeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)

	operation := waitForSubtitleTranslationOperation(t, operationID, "error")
	if operation.Status != "error" || operation.Phase != "translating" || operation.Progress != 40 || operation.Processed != 0 || operation.Total != 0 || operation.StreamIndex != -1 || operation.Format != "ass" || operation.Kind != subtitleTranslationOperationKind {
		t.Fatalf("unexpected malformed-provider terminal state: %#v", operation)
	}
	if operation.Translation == nil || operation.Translation.Provider != translationProviderOllama || operation.Translation.Model != "translategemma:12b" || operation.Translation.RequestedSourceLanguage != "en" || operation.Translation.TargetLocale != "es-419" || operation.Translation.OutputRepresentation != "ass" || operation.Translation.FailureClass != string(subtitleTranslationFailureMalformedResponse) || operation.Translation.Diagnostic != "Translation provider returned a malformed response." || !operation.Translation.Retryable || operation.Translation.Cancellable {
		t.Fatalf("unexpected malformed-provider translation view: %#v", operation.Translation)
	}
	diagnostics := operation.Translation.ProviderDiagnostics
	if diagnostics == nil || diagnostics.Category != translationResponseDiagnosticInnerJSONInvalid || diagnostics.BatchIndex != 1 || diagnostics.BatchCount != 1 || diagnostics.ExpectedCueCount == nil || *diagnostics.ExpectedCueCount != 1 || diagnostics.ActualCueCount != nil {
		t.Fatalf("malformed response subreason=%#v, want inner JSON failure for batch 1 of 1", diagnostics)
	}
	if len(execution.Outputs) != 0 || string(sourceContent) != string(originalSource) {
		t.Fatalf("malformed response produced output or modified source: outputs=%d sourceChanged=%v", len(execution.Outputs), string(sourceContent) != string(originalSource))
	}
	encodedDiagnostics, err := json.Marshal(operation.Translation.ProviderDiagnostics)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"MODEL_RESPONSE_SENTINEL", "ASS_CUE_SENTINEL", "synthetic-ass-source", request.AssetPath} {
		if strings.Contains(string(encodedDiagnostics), sentinel) || strings.Contains(operation.Error, sentinel) || strings.Contains(operation.Translation.Diagnostic, sentinel) {
			t.Fatalf("malformed response diagnostics exposed %q", sentinel)
		}
	}
}

func TestSubtitleTranslationFailureClassDoesNotTreatPlainDeadlineAsProviderTimeout(t *testing.T) {
	if got := subtitleTranslationFailureClassOf(context.DeadlineExceeded); got != subtitleTranslationFailureTranslationFailed {
		t.Fatalf("plain deadline class=%q, want %q", got, subtitleTranslationFailureTranslationFailed)
	}

	typed := &TranslationProviderError{Class: translationProviderErrorTimeout, Err: context.DeadlineExceeded}
	if got := subtitleTranslationFailureClassOf(typed); got != subtitleTranslationFailureProviderTimeout {
		t.Fatalf("typed deadline class=%q, want %q", got, subtitleTranslationFailureProviderTimeout)
	}
	wrapper := fmt.Errorf("provider request: %w", typed)
	if got := subtitleTranslationFailureClassOf(wrapper); got != subtitleTranslationFailureProviderTimeout {
		t.Fatalf("wrapped typed deadline class=%q, want %q", got, subtitleTranslationFailureProviderTimeout)
	}
}

func TestBoundedSubtitleTranslationErrorIsUnicodeSafe(t *testing.T) {
	const suffix = "..."
	tests := []struct {
		name       string
		message    string
		wantExact  bool
		wantSuffix bool
	}{
		{name: "short ASCII", message: "provider unavailable", wantExact: true},
		{name: "long ASCII", message: strings.Repeat("a", maxSubtitleTranslationOperationError+10), wantSuffix: true},
		{name: "multibyte bytes exceed rune bound", message: strings.Repeat("界", maxSubtitleTranslationOperationError/2+1), wantExact: true},
		{name: "multibyte rune bound", message: strings.Repeat("界", maxSubtitleTranslationOperationError+1), wantSuffix: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boundedSubtitleTranslationError(errors.New(test.message))
			if !utf8.ValidString(got) {
				t.Fatal("bounded diagnostic is not valid UTF-8")
			}
			if test.wantExact && got != test.message {
				t.Fatalf("short diagnostic changed: got %q want %q", got, test.message)
			}
			if test.wantSuffix {
				if !strings.HasSuffix(got, suffix) {
					t.Fatalf("bounded diagnostic lacks suffix: %q", got)
				}
				if gotRunes := len([]rune(got)); gotRunes != maxSubtitleTranslationOperationError+len([]rune(suffix)) {
					t.Fatalf("bounded diagnostic rune length=%d want=%d", gotRunes, maxSubtitleTranslationOperationError+len([]rune(suffix)))
				}
			}
		})
	}
}
