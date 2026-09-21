package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func subtitleTranslationCancelRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/subtitle-operations/:id/cancel", (&AssetHandler{}).CancelSubtitleExtractionOperation)
	return router
}

func assertNoMediaAdjacentTranslationArtifacts(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if name == "ocr-cache" {
			continue
		}
		if strings.HasPrefix(name, ".mvforge-") ||
			strings.HasPrefix(name, "translation-output-") ||
			strings.HasPrefix(name, ".translation-rollback-") ||
			strings.HasSuffix(name, ".tmp") ||
			strings.HasSuffix(name, ".raw") ||
			strings.HasSuffix(name, ".bak") {
			t.Fatalf("media-adjacent translation artifact survived: %q", entry.Name())
		}
	}
}

func writeTranslationLifecycleSource(t *testing.T, root string) (string, translationSourceRevision) {
	t.Helper()
	sourcePath := filepath.Join(root, "source.srt")
	if err := os.WriteFile(sourcePath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	revision, err := captureTranslationExternalSourceRevision(sourcePath, "logical-source")
	if err != nil {
		t.Fatal(err)
	}
	return sourcePath, revision
}

func TestSubtitleTranslationLifecycleFailureCleanupAndMediaIsolation(t *testing.T) {
	tests := []struct {
		name   string
		runner func(context.Context, string, string, subtitleTranslationOperationRequest, string, translationSourceRevision) error
	}{
		{
			name: "provider failure",
			runner: func(_ context.Context, _ string, workspace string, _ subtitleTranslationOperationRequest, _ string, _ translationSourceRevision) error {
				if err := os.WriteFile(filepath.Join(workspace, "owned-work"), []byte("provider"), 0o600); err != nil {
					return err
				}
				return errors.New("provider failure")
			},
		},
		{
			name: "serializer validation failure",
			runner: func(_ context.Context, _ string, workspace string, _ subtitleTranslationOperationRequest, sourcePath string, revision translationSourceRevision) error {
				target := subtitleTranslationPublicationTarget{
					Format:    translationRepresentationSRT,
					FinalPath: filepath.Join(filepath.Dir(sourcePath), "translated.srt"),
					Artifact:  translationPublicationArtifact(t, sourcePath, revision, translationRepresentationSRT),
				}
				_, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("invalid")}, []subtitleTranslationPublicationTarget{target})
				return err
			},
		},
		{
			name: "stale source",
			runner: func(_ context.Context, operationID, workspace string, _ subtitleTranslationOperationRequest, sourcePath string, revision translationSourceRevision) error {
				target := subtitleTranslationPublicationTarget{
					Format:    translationRepresentationSRT,
					FinalPath: filepath.Join(filepath.Dir(sourcePath), "translated.srt"),
					Artifact:  translationPublicationArtifact(t, sourcePath, revision, translationRepresentationSRT),
				}
				staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")}, []subtitleTranslationPublicationTarget{target})
				if err != nil {
					return err
				}
				if err := os.WriteFile(sourcePath, []byte("1\n00:00:01,000 --> 00:00:02,000\nEdited\n"), 0o600); err != nil {
					return err
				}
				_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
					Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: staged,
				})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
			sourcePath, revision := writeTranslationLifecycleSource(t, root)
			request := testSubtitleTranslationOperationRequest(sourcePath)
			request.SourceRevision = revision.key()
			workspaceSeen := make(chan string, 1)
			operationID, err := startSubtitleTranslationOperation(request, func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
				workspaceSeen <- workspace
				return test.runner(ctx, operationID, workspace, request, sourcePath, revision)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer deleteSubtitleTranslationOperation(operationID)
			workspace := <-workspaceSeen
			failed := waitForSubtitleTranslationOperation(t, operationID, "error")
			if failed.publicationCommitted || failed.Status != "error" {
				t.Fatalf("failure did not remain pre-commit: %#v", failed)
			}
			if _, err := os.Stat(workspace); !os.IsNotExist(err) {
				t.Fatalf("operation workspace survived %s: %q (%v)", test.name, workspace, err)
			}
			if _, err := os.Stat(filepath.Join(root, "translated.srt")); !os.IsNotExist(err) {
				t.Fatalf("failed %s created translated output: %v", test.name, err)
			}
			assertNoMediaAdjacentTranslationArtifacts(t, root)
		})
	}
}

func TestSubtitleTranslationCancelBeforePublicationCleansWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	sourcePath, revision := writeTranslationLifecycleSource(t, root)
	request := testSubtitleTranslationOperationRequest(sourcePath)
	request.SourceRevision = revision.key()
	workspaceSeen := make(chan string, 1)
	runnerCancelled := make(chan struct{})
	operationID, err := startSubtitleTranslationOperation(request, func(ctx context.Context, _ string, workspace string, _ subtitleTranslationOperationRequest) error {
		if err := os.WriteFile(filepath.Join(workspace, "owned-work"), []byte("staged"), 0o600); err != nil {
			return err
		}
		workspaceSeen <- workspace
		<-ctx.Done()
		close(runnerCancelled)
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	workspace := <-workspaceSeen
	response := httptest.NewRecorder()
	subtitleTranslationCancelRouter().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+operationID+"/cancel", nil))
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("cancel returned status=%d body=%s", response.Code, response.Body.String())
	}
	<-runnerCancelled
	cancelled := waitForSubtitleTranslationOperation(t, operationID, "cancelled")
	if cancelled.publicationCommitted || cancelled.Error != "" {
		t.Fatalf("pre-publication cancellation was not clean: %#v", cancelled)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("cancelled operation workspace survived: %q (%v)", workspace, err)
	}
	assertNoMediaAdjacentTranslationArtifacts(t, root)
}

func TestSubtitleTranslationCancelWhileWaitingForPublicationLock(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	sourcePath, revision := writeTranslationLifecycleSource(t, root)
	request := testSubtitleTranslationOperationRequest(sourcePath)
	request.SourceRevision = revision.key()
	publicationReady := make(chan struct{})
	contextCancelled := make(chan struct{})
	workspaceSeen := make(chan string, 1)
	var finalPath = filepath.Join(root, "translated.srt")
	runner := func(ctx context.Context, operationID, workspace string, _ subtitleTranslationOperationRequest) error {
		workspaceSeen <- workspace
		target := subtitleTranslationPublicationTarget{Format: translationRepresentationSRT, FinalPath: finalPath, Artifact: translationPublicationArtifact(t, sourcePath, revision, translationRepresentationSRT)}
		staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")}, []subtitleTranslationPublicationTarget{target})
		if err != nil {
			return err
		}
		go func() {
			<-ctx.Done()
			close(contextCancelled)
		}()
		close(publicationReady)
		_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{Ctx: ctx, OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: staged})
		return err
	}

	subtitlePublicationMu.Lock()
	released := false
	defer func() {
		if !released {
			subtitlePublicationMu.Unlock()
		}
	}()
	operationID, err := startSubtitleTranslationOperation(request, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	workspace := <-workspaceSeen
	<-publicationReady
	cancelDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		subtitleTranslationCancelRouter().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+operationID+"/cancel", nil))
		cancelDone <- response
	}()
	select {
	case <-contextCancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel request did not signal the queued publisher")
	}
	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("queued publisher wrote output before publication lock release: %v", err)
	}
	subtitlePublicationMu.Unlock()
	released = true
	response := <-cancelDone
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("cancel returned status=%d body=%s", response.Code, response.Body.String())
	}
	cancelled := waitForSubtitleTranslationOperation(t, operationID, "cancelled")
	if cancelled.publicationCommitted {
		t.Fatal("queued publisher committed after cancellation")
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("queued cancellation left workspace: %q (%v)", workspace, err)
	}
	assertNoMediaAdjacentTranslationArtifacts(t, root)
}

func TestSubtitleTranslationCancelBetweenMembersRollsBackLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	sourcePath, revision := writeTranslationLifecycleSource(t, root)
	request := testSubtitleTranslationOperationRequest(sourcePath)
	request.SourceRevision = revision.key()
	finalSRT := filepath.Join(root, "translated.srt")
	finalASS := filepath.Join(root, "translated.ass")
	oldSRT := []byte("1\n00:00:01,000 --> 00:00:02,000\nOld SRT\n")
	oldASS := []byte("[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,Old ASS\n")
	if err := os.WriteFile(finalSRT, oldSRT, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalASS, oldASS, 0o600); err != nil {
		t.Fatal(err)
	}
	cancelDone := make(chan *httptest.ResponseRecorder, 1)
	var operationID string
	runner := func(ctx context.Context, operationIDValue, workspace string, _ subtitleTranslationOperationRequest) error {
		targets := []subtitleTranslationPublicationTarget{
			{Format: translationRepresentationSRT, FinalPath: finalSRT, Artifact: translationPublicationArtifact(t, sourcePath, revision, translationRepresentationSRT)},
			{Format: translationRepresentationASS, FinalPath: finalASS, Artifact: translationPublicationArtifact(t, sourcePath, revision, translationRepresentationASS)},
		}
		staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{
			translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n"),
			translationRepresentationASS: []byte("[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,Hola\n"),
		}, targets)
		if err != nil {
			return err
		}
		var calls atomic.Int32
		_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
			Ctx: ctx, OperationID: operationIDValue, Workspace: workspace, SourceRevision: revision, Targets: staged,
			PublishStage: func(stagedPath, finalPath string) error {
				if err := publishStagedSubtitleFile(stagedPath, finalPath); err != nil {
					return err
				}
				if calls.Add(1) == 1 {
					go func() {
						response := httptest.NewRecorder()
						subtitleTranslationCancelRouter().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+operationIDValue+"/cancel", nil))
						cancelDone <- response
					}()
					<-ctx.Done()
				}
				return nil
			},
		})
		return err
	}

	var err error
	operationID, err = startSubtitleTranslationOperation(request, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	cancelled := waitForSubtitleTranslationOperation(t, operationID, "cancelled")
	response := <-cancelDone
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("cancel returned status=%d body=%s", response.Code, response.Body.String())
	}
	if cancelled.publicationCommitted {
		t.Fatal("between-member cancellation committed publication")
	}
	for path, want := range map[string][]byte{finalSRT: oldSRT, finalASS: oldASS} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != string(want) {
			t.Fatalf("cancelled lifecycle changed %q: %q (%v)", path, got, readErr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "ocr-cache")); err != nil {
		t.Fatalf("expected internal workspace root to remain usable: %v", err)
	}
	assertNoMediaAdjacentTranslationArtifacts(t, root)
}

func TestSubtitleTranslationCompletionWinsCancellationAfterMetadataCommit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	sourcePath, revision := writeTranslationLifecycleSource(t, root)
	request := testSubtitleTranslationOperationRequest(sourcePath)
	request.SourceRevision = revision.key()
	finalPath := filepath.Join(root, "translated.srt")
	metadataCommitted := make(chan struct{})
	cancelDone := make(chan *httptest.ResponseRecorder, 1)
	operationID := ""
	runner := func(ctx context.Context, operationIDValue, workspace string, _ subtitleTranslationOperationRequest) error {
		target := subtitleTranslationPublicationTarget{Format: translationRepresentationSRT, FinalPath: finalPath, Artifact: translationPublicationArtifact(t, sourcePath, revision, translationRepresentationSRT)}
		staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")}, []subtitleTranslationPublicationTarget{target})
		if err != nil {
			return err
		}
		_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
			Ctx: ctx, OperationID: operationIDValue, Workspace: workspace, SourceRevision: revision, Targets: staged,
			CommitMetadata: func([]SubtitleArtifact) error {
				close(metadataCommitted)
				go func() {
					response := httptest.NewRecorder()
					subtitleTranslationCancelRouter().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+operationIDValue+"/cancel", nil))
					cancelDone <- response
				}()
				<-ctx.Done()
				return nil
			},
		})
		return err
	}

	var err error
	operationID, err = startSubtitleTranslationOperation(request, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	<-metadataCommitted
	completed := waitForSubtitleTranslationOperation(t, operationID, "completed")
	response := <-cancelDone
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("late cancel returned status=%d body=%s", response.Code, response.Body.String())
	}
	if completed.Status != "completed" || completed.Progress != 100 || !completed.publicationCommitted {
		t.Fatalf("completion did not win after metadata commit: %#v", completed)
	}
	if got, err := os.ReadFile(finalPath); err != nil || !strings.Contains(string(got), "Hola") {
		t.Fatalf("committed translation was rolled back after late cancellation: %q (%v)", got, err)
	}
	assertNoMediaAdjacentTranslationArtifacts(t, root)
}

func TestSubtitleTranslationFailurePreservesSharedOCREvidence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "eng"}}
	cues := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "Hello", SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}}
	cacheBitmapTranslationEvidence(t, mediaPath, stream, "eng", "accurate", "eng", cues)
	request := canonicalOCRRequestFor(mediaPath, stream, "eng", "accurate", 0, 0)
	withRuntime, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	cachePath := canonicalOCRCachePath(filepath.Join(root, "ocr-cache"), withRuntime.Identity.key())
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatal(err)
	}
	operationID, err := startSubtitleTranslationOperation(testSubtitleTranslationOperationRequest(mediaPath), func(context.Context, string, string, subtitleTranslationOperationRequest) error {
		return errors.New("translation consumer failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	waitForSubtitleTranslationOperation(t, operationID, "error")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("translation cleanup removed shared OCR evidence: %v", err)
	}
}

func TestSubtitleTranslationTerminalStatesRemainImmutable(t *testing.T) {
	tests := []struct {
		status   string
		phase    string
		progress float64
	}{
		{status: "completed", phase: "completed", progress: 100},
		{status: "error", phase: "error", progress: 42},
		{status: "cancelled", phase: "cleanup", progress: 17},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			id := "translation-terminal-" + test.status + "-" + strings.ReplaceAll(t.Name(), "/", "-")
			now := time.Now()
			subtitleExtractionOperations.Lock()
			subtitleExtractionOperations.items[id] = &subtitleExtractionOperation{
				ID: id, Kind: subtitleTranslationOperationKind, Status: test.status, Phase: test.phase, Progress: test.progress,
				Message: "terminal", Error: "terminal error", CreatedAt: now, UpdatedAt: now, publicationCommittedCh: make(chan struct{}),
			}
			subtitleExtractionOperations.Unlock()
			defer deleteSubtitleTranslationOperation(id)
			before := snapshotSubtitleOperation(id)
			updateSubtitleTranslationProgress(id, "cleanup", 99, 100, "late progress")
			updateSubtitleExtractionOperation(id, func(operation *subtitleExtractionOperation) {
				operation.Status = "running"
				operation.Message = "late callback"
				operation.Error = "late error"
			})
			response := httptest.NewRecorder()
			subtitleTranslationCancelRouter().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-operations/"+id+"/cancel", nil))
			after := snapshotSubtitleOperation(id)
			if after.Status != before.Status || after.Phase != before.Phase || after.Progress != before.Progress || after.Message != before.Message || after.Error != before.Error {
				t.Fatalf("terminal operation changed after late callbacks/cancel: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestSubtitleTranslationMetadataRollbackFailureKeepsBoundedDiagnostic(t *testing.T) {
	sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT)
	old := []byte("1\n00:00:01,000 --> 00:00:02,000\nOld\n")
	if err := os.WriteFile(targets[0].FinalPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		CommitMetadata:   func([]SubtitleArtifact) error { return errors.New("metadata commit failed") },
		RollbackMetadata: func([]SubtitleArtifact) error { return errors.New("rollback metadata unavailable") },
	})
	if err == nil || !strings.Contains(err.Error(), "metadata rollback diagnostic") {
		t.Fatalf("metadata rollback diagnostic was not preserved: %v", err)
	}
	if len(err.Error()) > maxSubtitleTranslationOperationError+128 {
		t.Fatalf("metadata rollback diagnostic was not bounded: %d", len(err.Error()))
	}
	if got, readErr := os.ReadFile(targets[0].FinalPath); readErr != nil || string(got) != string(old) {
		t.Fatalf("metadata rollback failure did not restore old translation: %q (%v)", got, readErr)
	}
	if snapshotSubtitleOperation(operationID).publicationCommitted {
		t.Fatal("metadata rollback failure marked publication committed")
	}
	if source, readErr := os.ReadFile(sourcePath); readErr != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("metadata rollback failure changed source: %q (%v)", source, readErr)
	}
}
