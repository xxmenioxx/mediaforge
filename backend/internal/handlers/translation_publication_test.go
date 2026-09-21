package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func registerSubtitleTranslationPublicationOperation(t *testing.T, operationID string) {
	t.Helper()
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Kind: subtitleTranslationOperationKind, Status: "running", Phase: "preparing",
		cancel: func() {}, publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Unlock()
	t.Cleanup(func() { deleteSubtitleTranslationOperation(operationID) })
}

func translationPublicationOperationID(t *testing.T) string {
	return "translation-publication-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
}

func translationPublicationMetadata(t *testing.T, sourcePath string, revision translationSourceRevision, format string) SubtitleTranslationMetadata {
	t.Helper()
	input := translationIdentityTestInput()
	input.SourceArtifactIdentity = filepath.Clean(sourcePath)
	input.SourceRevision = revision.key()
	input.OutputRepresentation = format
	identity, err := newTranslationIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	provenance, err := buildSubtitleTranslationProvenance(metadata, "operation-publication", "asset-metadata", "validated", TranslationProviderMetrics{Provider: identity.Provider, Model: identity.ProviderModel, RequestCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	metadata.Provenance = provenance
	return metadata
}

func translationPublicationArtifact(t *testing.T, sourcePath string, revision translationSourceRevision, format string) SubtitleArtifact {
	metadata := translationPublicationMetadata(t, sourcePath, revision, format)
	return SubtitleArtifact{
		ArtifactID:  "translated-" + format,
		Type:        "subtitle_sidecar",
		Format:      format,
		Mode:        "converted",
		Language:    "es",
		Status:      "planned",
		Translation: &metadata,
	}
}

func externalTranslationPublicationFixture(t *testing.T, formats ...string) (string, translationSourceRevision, string, []subtitleTranslationPublicationTarget, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.srt")
	sourceContent := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")
	if err := os.WriteFile(sourcePath, sourceContent, 0o600); err != nil {
		t.Fatal(err)
	}
	revision, err := captureTranslationExternalSourceRevision(sourcePath, "logical-source")
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	outputs := make(map[string][]byte, len(formats))
	targets := make([]subtitleTranslationPublicationTarget, 0, len(formats))
	for _, format := range formats {
		content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")
		if format == translationRepresentationASS {
			content = []byte("[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hola\n")
		}
		outputs[format] = content
		targets = append(targets, subtitleTranslationPublicationTarget{
			Format:    format,
			FinalPath: filepath.Join(root, "translated."+format),
			Artifact:  translationPublicationArtifact(t, sourcePath, revision, format),
		})
	}
	return sourcePath, revision, workspace, targets, outputs
}

func stageExternalTranslationFixture(t *testing.T, formats ...string) (string, translationSourceRevision, string, []subtitleTranslationPublicationTarget) {
	t.Helper()
	sourcePath, revision, workspace, targets, outputs := externalTranslationPublicationFixture(t, formats...)
	staged, err := stageSubtitleTranslationOutputs(workspace, outputs, targets)
	if err != nil {
		t.Fatal(err)
	}
	return sourcePath, revision, workspace, staged
}

func TestPublishSubtitleTranslationSingleOutputAndMetadata(t *testing.T) {
	sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT)
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	var committed []SubtitleArtifact
	published, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		CommitMetadata: func(artifacts []SubtitleArtifact) error {
			committed = append([]SubtitleArtifact(nil), artifacts...)
			return nil
		},
		RollbackMetadata: func([]SubtitleArtifact) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 || len(committed) != 1 || published[0].PublishedPath != targets[0].FinalPath {
		t.Fatalf("translation publication metadata was not committed: published=%#v committed=%#v", published, committed)
	}
	if got, err := os.ReadFile(targets[0].FinalPath); err != nil || !strings.Contains(string(got), "Hola") {
		t.Fatalf("translated SRT was not published: %q (%v)", got, err)
	}
	if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("source subtitle changed: %q (%v)", source, err)
	}
	if !snapshotSubtitleOperation(operationID).publicationCommitted {
		t.Fatal("successful translation publication was not marked committed")
	}
}

func TestPublishSubtitleTranslationExistingBothRollsBackOnSecondMemberFailure(t *testing.T) {
	sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT, translationRepresentationASS)
	oldSRT := []byte("1\n00:00:01,000 --> 00:00:02,000\nOld SRT\n")
	oldASS := []byte("[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,Old ASS\n")
	if err := os.WriteFile(targets[0].FinalPath, oldSRT, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targets[1].FinalPath, oldASS, 0o600); err != nil {
		t.Fatal(err)
	}
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	var calls atomic.Int32
	_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		PublishStage: func(staged, destination string) error {
			if calls.Add(1) == 2 {
				return errors.New("induced second member failure")
			}
			return publishStagedSubtitleFile(staged, destination)
		},
	})
	if err == nil {
		t.Fatal("second member failure unexpectedly committed translation")
	}
	for index, want := range [][]byte{oldSRT, oldASS} {
		got, readErr := os.ReadFile(targets[index].FinalPath)
		if readErr != nil || string(got) != string(want) {
			t.Fatalf("destination %d was not restored: %q (%v)", index, got, readErr)
		}
	}
	if snapshotSubtitleOperation(operationID).publicationCommitted {
		t.Fatal("failed translation publication was marked committed")
	}
	if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("source subtitle changed during rollback: %q (%v)", source, err)
	}
}

func TestPublishSubtitleTranslationNewBothRollsBackCreatedMember(t *testing.T) {
	sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT, translationRepresentationASS)
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	var calls atomic.Int32
	_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		PublishStage: func(staged, destination string) error {
			if calls.Add(1) == 2 {
				return errors.New("induced second member failure")
			}
			return publishStagedSubtitleFile(staged, destination)
		},
	})
	if err == nil {
		t.Fatal("new-output failure unexpectedly committed translation")
	}
	for _, target := range targets {
		if _, statErr := os.Stat(target.FinalPath); !os.IsNotExist(statErr) {
			t.Fatalf("failed publication left new destination %q: %v", target.FinalPath, statErr)
		}
	}
	if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("source subtitle changed during new-output rollback: %q (%v)", source, err)
	}
}

func TestPublishSubtitleTranslationRejectsInvalidStageAndMetadataFailure(t *testing.T) {
	sourcePath, revision, workspace, targets, _ := externalTranslationPublicationFixture(t, translationRepresentationSRT)
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	if _, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("not srt")}, targets); err == nil {
		t.Fatal("invalid translated stage was accepted")
	}
	if _, err := os.Stat(targets[0].FinalPath); !os.IsNotExist(err) {
		t.Fatalf("invalid stage changed destination: %v", err)
	}

	staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nNew\n")}, targets)
	if err != nil {
		t.Fatal(err)
	}
	old := []byte("1\n00:00:01,000 --> 00:00:02,000\nOld\n")
	if err := os.WriteFile(targets[0].FinalPath, old, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: staged,
		CommitMetadata:   func([]SubtitleArtifact) error { return errors.New("metadata commit failed") },
		RollbackMetadata: func([]SubtitleArtifact) error { return nil },
	})
	if err == nil {
		t.Fatal("metadata failure unexpectedly committed translation")
	}
	got, readErr := os.ReadFile(targets[0].FinalPath)
	if readErr != nil || string(got) != string(old) {
		t.Fatalf("metadata failure did not restore existing translation: %q (%v)", got, readErr)
	}
	if snapshotSubtitleOperation(operationID).publicationCommitted {
		t.Fatal("metadata failure marked publication committed")
	}
	if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("source subtitle changed after metadata failure: %q (%v)", source, err)
	}
}

func TestPublishSubtitleTranslationRequiresPairedMetadataCallbacks(t *testing.T) {
	sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT)
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	commitCalled := false
	rollbackCalled := false
	commitOnlyErr := func() error {
		_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
			Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
			CommitMetadata: func([]SubtitleArtifact) error {
				commitCalled = true
				return nil
			},
		})
		return err
	}
	if err := commitOnlyErr(); err == nil || !strings.Contains(err.Error(), "commit and rollback must be configured together") {
		t.Fatalf("commit without rollback was accepted: %v", err)
	}
	if commitCalled {
		t.Fatal("invalid commit-only request invoked CommitMetadata")
	}
	if _, err := os.Stat(targets[0].FinalPath); !os.IsNotExist(err) {
		t.Fatalf("invalid commit-only request changed final destination: %v", err)
	}

	_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		RollbackMetadata: func([]SubtitleArtifact) error {
			rollbackCalled = true
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "commit and rollback must be configured together") {
		t.Fatalf("rollback without commit was accepted: %v", err)
	}
	if rollbackCalled {
		t.Fatal("invalid rollback-only request invoked RollbackMetadata")
	}
	if snapshotSubtitleOperation(operationID).publicationCommitted {
		t.Fatal("invalid metadata callback configuration marked publication committed")
	}
	if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("metadata callback validation changed source: %q (%v)", source, err)
	}
}

func TestPublishSubtitleTranslationRechecksExternalAndEmbeddedSource(t *testing.T) {
	t.Run("external source edit", func(t *testing.T) {
		sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT)
		operationID := translationPublicationOperationID(t)
		registerSubtitleTranslationPublicationOperation(t, operationID)
		if err := os.WriteFile(sourcePath, []byte("1\n00:00:01,000 --> 00:00:02,000\nEdited\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
			Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		})
		if !errors.Is(err, errTranslationSourceStale) {
			t.Fatalf("external edit was not rejected as stale: %v", err)
		}
		if _, statErr := os.Stat(targets[0].FinalPath); !os.IsNotExist(statErr) {
			t.Fatalf("stale external source produced a translated destination: %v", statErr)
		}
	})

	t.Run("embedded stream replacement", func(t *testing.T) {
		root := t.TempDir()
		mediaPath := filepath.Join(root, "movie.mkv")
		if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
			t.Fatal(err)
		}
		base := translationEmbeddedSourceSnapshot{SourcePath: mediaPath, SourceSize: 5, SourceModTimeUnixNano: 1, SourceSnapshotVersion: "snapshot-1", StreamIndex: 4, StreamID: "stream-4", StreamSignature: "signature-a", StreamCodec: "hdmv_pgs_subtitle", SourceRepresentation: "bitmap"}
		revision, err := captureTranslationEmbeddedSourceRevision(base)
		if err != nil {
			t.Fatal(err)
		}
		workspace := filepath.Join(root, "workspace")
		if err := os.Mkdir(workspace, 0o700); err != nil {
			t.Fatal(err)
		}
		targets := []subtitleTranslationPublicationTarget{{Format: translationRepresentationSRT, FinalPath: filepath.Join(root, "translated.srt"), Artifact: translationPublicationArtifact(t, mediaPath, revision, translationRepresentationSRT)}}
		staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")}, targets)
		if err != nil {
			t.Fatal(err)
		}
		operationID := translationPublicationOperationID(t)
		registerSubtitleTranslationPublicationOperation(t, operationID)
		current := base
		current.StreamSignature = "signature-b"
		_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
			Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: staged,
			CurrentEmbeddedSource: func() (translationEmbeddedSourceSnapshot, error) { return current, nil },
		})
		if !errors.Is(err, errTranslationSourceStale) {
			t.Fatalf("embedded stream replacement was not rejected as stale: %v", err)
		}
		if _, statErr := os.Stat(targets[0].FinalPath); !os.IsNotExist(statErr) {
			t.Fatalf("stale embedded source produced a translated destination: %v", statErr)
		}
	})
}

func TestPublishSubtitleTranslationRecheckRunsInsidePublicationBarrier(t *testing.T) {
	sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT)
	operationID := translationPublicationOperationID(t)
	registerSubtitleTranslationPublicationOperation(t, operationID)
	sawLockedBarrier := false
	_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: context.Background(), OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
		RecheckSource: func(expected translationSourceRevision) error {
			if !subtitlePublicationMu.TryLock() {
				sawLockedBarrier = true
				return recheckTranslationExternalSourceRevision(expected)
			}
			subtitlePublicationMu.Unlock()
			return errors.New("publication lock was not held during source recheck")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawLockedBarrier {
		t.Fatal("source recheck did not run inside publication barrier")
	}
	if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
		t.Fatalf("source subtitle changed during locked recheck: %q (%v)", source, err)
	}
}

func TestPublishSubtitleTranslationCancellationRollsBackAndCommittedRunWins(t *testing.T) {
	t.Run("between members", func(t *testing.T) {
		sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT, translationRepresentationASS)
		oldSRT := []byte("1\n00:00:01,000 --> 00:00:02,000\nOld SRT\n")
		oldASS := []byte("[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,Old ASS\n")
		if err := os.WriteFile(targets[0].FinalPath, oldSRT, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targets[1].FinalPath, oldASS, 0o600); err != nil {
			t.Fatal(err)
		}
		operationID := translationPublicationOperationID(t)
		registerSubtitleTranslationPublicationOperation(t, operationID)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
			Ctx: ctx, OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
			PublishStage: func(staged, destination string) error {
				if err := publishStagedSubtitleFile(staged, destination); err != nil {
					return err
				}
				if calls.Add(1) == 1 {
					cancel()
				}
				return nil
			},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("between-member cancellation was not preserved: %v", err)
		}
		for index, want := range [][]byte{oldSRT, oldASS} {
			got, readErr := os.ReadFile(targets[index].FinalPath)
			if readErr != nil || string(got) != string(want) {
				t.Fatalf("cancelled publication did not restore member %d: %q (%v)", index, got, readErr)
			}
		}
		if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
			t.Fatalf("source subtitle changed after cancellation: %q (%v)", source, err)
		}
	})

	t.Run("after commit", func(t *testing.T) {
		sourcePath, revision, workspace, targets := stageExternalTranslationFixture(t, translationRepresentationSRT)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		operationID := translationPublicationOperationID(t)
		registerSubtitleTranslationPublicationOperation(t, operationID)
		publicationDone := make(chan error, 1)
		go func() {
			_, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
				Ctx: ctx, OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: targets,
			})
			publicationDone <- err
		}()
		<-snapshotSubtitleOperation(operationID).publicationCommittedCh
		cancel()
		if err := <-publicationDone; err != nil || !snapshotSubtitleOperation(operationID).publicationCommitted {
			t.Fatalf("late cancellation invalidated committed publication: err=%v op=%#v", err, snapshotSubtitleOperation(operationID))
		}
		if _, err := os.Stat(targets[0].FinalPath); err != nil {
			t.Fatalf("committed translation was rolled back after late cancellation: %v", err)
		}
		if source, err := os.ReadFile(sourcePath); err != nil || !strings.Contains(string(source), "Hello") {
			t.Fatalf("source subtitle changed after committed cancellation: %q (%v)", source, err)
		}
	})
}

func TestSubtitleTranslationRunnerCompletesAfterPublicationCommit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	sourcePath := filepath.Join(root, "source.srt")
	if err := os.WriteFile(sourcePath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	revision, err := captureTranslationExternalSourceRevision(sourcePath, "logical-source")
	if err != nil {
		t.Fatal(err)
	}
	request := testSubtitleTranslationOperationRequest(sourcePath)
	request.Format = translationRepresentationSRT
	operationID, err := startSubtitleTranslationOperation(request, func(ctx context.Context, operationID, workspace string, request subtitleTranslationOperationRequest) error {
		targets := []subtitleTranslationPublicationTarget{{Format: translationRepresentationSRT, FinalPath: filepath.Join(root, "translated.srt"), Artifact: translationPublicationArtifact(t, sourcePath, revision, translationRepresentationSRT)}}
		staged, err := stageSubtitleTranslationOutputs(workspace, map[string][]byte{translationRepresentationSRT: []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")}, targets)
		if err != nil {
			return err
		}
		_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{Ctx: ctx, OperationID: operationID, Workspace: workspace, SourceRevision: revision, Targets: staged})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteSubtitleTranslationOperation(operationID) })
	completed := waitForSubtitleTranslationOperation(t, operationID, "completed")
	if completed.Progress != 100 || !completed.publicationCommitted {
		t.Fatalf("committed translation runner did not complete: %#v", completed)
	}
	if _, err := os.Stat(filepath.Join(root, "translated.srt")); err != nil {
		t.Fatalf("translation output missing after committed runner: %v", err)
	}
}

func TestSubtitleTranslationPublicationEmbeddedSnapshotDoesNotHashMedia(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := translationEmbeddedSourceSnapshot{SourcePath: mediaPath, SourceSize: 5, SourceModTimeUnixNano: 1, SourceSnapshotVersion: "snapshot-1", StreamIndex: 4, StreamID: "stream-4", StreamSignature: "signature-a", StreamCodec: "hdmv_pgs_subtitle", SourceRepresentation: "bitmap"}
	revision, err := captureTranslationEmbeddedSourceRevision(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(revision.key(), "media") {
		t.Fatal("embedded revision key unexpectedly contains media content")
	}
}
