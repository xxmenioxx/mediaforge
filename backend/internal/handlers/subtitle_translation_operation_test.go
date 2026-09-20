package handlers

import (
	"context"
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
	if initial.Progress < 0 || initial.Progress > 100 || initial.translationRequest == nil {
		t.Fatalf("translation operation did not retain bounded request/progress: %#v", initial)
	}

	workspace := <-runnerStarted
	progress := <-progressSeen
	if progress.Phase != "translating" || progress.Processed != 1 || progress.Total != 3 || progress.Progress < initial.Progress {
		t.Fatalf("unexpected translation progress: %#v", progress)
	}
	close(release)
	completed := waitForSubtitleTranslationOperation(t, operationID, "completed")
	if completed.Phase != "completed" || completed.Progress != 100 || completed.Processed != 3 || completed.Total != 3 {
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
	if failed.Phase != "error" || failed.Error == "" || len(failed.Error) > maxSubtitleTranslationOperationError+3 {
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
