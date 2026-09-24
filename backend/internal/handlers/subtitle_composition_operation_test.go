package handlers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func waitForSubtitleCompositionOperation(t *testing.T, id string) subtitleExtractionOperation {
	t.Helper()
	for attempts := 0; attempts < 100000; attempts++ {
		subtitleExtractionOperations.RLock()
		operation := subtitleExtractionOperations.items[id]
		if operation != nil && (operation.Status == "completed" || operation.Status == "error" || operation.Status == "cancelled") {
			snapshot := cloneSubtitleExtractionOperationSnapshot(operation)
			subtitleExtractionOperations.RUnlock()
			return snapshot
		}
		subtitleExtractionOperations.RUnlock()
		runtime.Gosched()
	}
	t.Fatal("subtitle composition operation did not reach a terminal state")
	return subtitleExtractionOperation{}
}

func compositionOperationFixture(t *testing.T, directory, name, text string, source SubtitleCompositionSource) SubtitleCompositionDocument {
	t.Helper()
	path := filepath.Join(directory, name)
	content := "[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default," + text + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := parseASSDocument("ass", content)
	if err != nil {
		t.Fatal(err)
	}
	source.Identity = "external:" + path
	source.Revision = revision
	source.Representation = "ass"
	return SubtitleCompositionDocument{Source: source, Document: document}
}

func compositionOperationRequestFixture(t *testing.T, directory string) subtitleCompositionOperationRequest {
	primary := compositionOperationFixture(t, directory, "primary.ass", "Hello", compositionTestSource("", true, SubtitleRoleFull, CompositionCueDialogue))
	supplemental := compositionOperationFixture(t, directory, "signs.ass", `{\an8}Sign`, compositionTestSource("", false, SubtitleRoleSigns, CompositionCueSign))
	policy := compositionTestPolicy()
	identity, err := SubtitleCompositionOutputIdentity([]SubtitleCompositionSource{primary.Source, supplemental.Source}, policy, policy.StylePolicyVersion, "ass", "standard")
	if err != nil {
		t.Fatal(err)
	}
	return subtitleCompositionOperationRequest{AssetPath: filepath.Join(directory, "episode.mkv"), OutputPath: filepath.Join(directory, "episode-"+identity[len(identity)-12:]+".ass"), Primary: primary, Supplementals: []SubtitleCompositionDocument{supplemental}, Policy: policy, OutputIdentity: identity, PublicationMode: "standard"}
}

func TestSubtitleCompositionOperationPublishesAtomicallyAndExactRetryReusesIdentity(t *testing.T) {
	directory := t.TempDir()
	request := compositionOperationRequestFixture(t, directory)
	firstID, err := startSubtitleCompositionOperation(request)
	if err != nil {
		t.Fatal(err)
	}
	first := waitForSubtitleCompositionOperation(t, firstID)
	if first.Status != "completed" || first.Composition == nil || first.Composition.OutputIdentity != request.OutputIdentity {
		t.Fatalf("composition did not complete with frozen metadata: %#v", first)
	}
	if _, err := os.Stat(request.OutputPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(request.OutputPath + ".composition.json"); err != nil {
		t.Fatal(err)
	}
	secondID, err := startSubtitleCompositionOperation(request)
	if err != nil {
		t.Fatal(err)
	}
	second := waitForSubtitleCompositionOperation(t, secondID)
	if second.Status != "completed" || second.Composition.OutputIdentity != first.Composition.OutputIdentity {
		t.Fatalf("exact retry did not reuse identity: first=%#v second=%#v", first.Composition, second.Composition)
	}
}

func TestSubtitleCompositionOperationRejectsStaleOneOfManySourcesBeforePublication(t *testing.T) {
	directory := t.TempDir()
	request := compositionOperationRequestFixture(t, directory)
	if err := os.WriteFile(strings.TrimPrefix(request.Supplementals[0].Source.Identity, "external:"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	operationID, err := startSubtitleCompositionOperation(request)
	if err != nil {
		t.Fatal(err)
	}
	operation := waitForSubtitleCompositionOperation(t, operationID)
	if operation.Status != "error" || operation.Result != nil {
		t.Fatalf("stale one-of-many source was not rejected: %#v", operation)
	}
	if _, statErr := os.Stat(request.OutputPath); !os.IsNotExist(statErr) {
		t.Fatalf("stale composition published an output: %v", statErr)
	}
}

func TestSubtitleCompositionPublicationBarrierRejectsCancelledBeforeCommit(t *testing.T) {
	directory := t.TempDir()
	stage := filepath.Join(directory, "stage.ass")
	output := filepath.Join(directory, "output.ass")
	if err := os.WriteFile(stage, []byte("ASS"), 0o644); err != nil {
		t.Fatal(err)
	}
	operationID := "subtitle-compose-barrier-test"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{ID: operationID, Status: "running", cancel: func() {}, publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Unlock()
	view := &subtitleCompositionOperationView{OutputIdentity: "identity"}
	if err := publishSubtitleComposition(operationID, ctx, stage, output, view); err == nil {
		t.Fatal("cancelled composition crossed publication barrier")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("cancelled composition left output: %v", err)
	}
	subtitleExtractionOperations.Lock()
	delete(subtitleExtractionOperations.items, operationID)
	subtitleExtractionOperations.Unlock()
}

func TestSubtitleCompositionOperationRevalidatesFrozenSourcesAtPublicationBoundary(t *testing.T) {
	directory := t.TempDir()
	request := compositionOperationRequestFixture(t, directory)
	originalHook := subtitleCompositionBeforePublicationHook
	defer func() { subtitleCompositionBeforePublicationHook = originalHook }()
	subtitleCompositionBeforePublicationHook = func() {
		path := strings.TrimPrefix(request.Supplementals[0].Source.Identity, "external:")
		if err := os.WriteFile(path, []byte("changed at publication boundary"), 0o644); err != nil {
			t.Fatalf("change frozen supplemental: %v", err)
		}
	}
	operationID, err := startSubtitleCompositionOperation(request)
	if err != nil {
		t.Fatal(err)
	}
	operation := waitForSubtitleCompositionOperation(t, operationID)
	if operation.Status != "error" || !strings.Contains(operation.Error, "stale_source") {
		t.Fatalf("publication-boundary stale source was not rejected: %#v", operation)
	}
	if _, statErr := os.Stat(request.OutputPath); !os.IsNotExist(statErr) {
		t.Fatalf("stale publication left canonical output: %v", statErr)
	}
	if _, statErr := os.Stat(strings.TrimSuffix(request.OutputPath, filepath.Ext(request.OutputPath)) + ".fonts"); !os.IsNotExist(statErr) {
		t.Fatalf("stale publication left attachment directory: %v", statErr)
	}
}

func TestSubtitleCompositionAttachmentPublicationIsAtomicAndUsesExistingArtifactBoundary(t *testing.T) {
	directory := t.TempDir()
	stage := filepath.Join(directory, "stage.ass")
	fontStage := filepath.Join(directory, "stage.font.ttf")
	output := filepath.Join(directory, "output.ass")
	if err := os.WriteFile(stage, []byte("ASS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fontStage, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	view := &subtitleCompositionOperationView{OutputIdentity: "identity", OutputPath: output, Provenance: SubtitleCompositionProvenance{AttachmentDecisions: []SubtitleCompositionAttachmentDecision{{SafeName: "Font.ttf", Action: "added"}}}}
	staged := []subtitleCompositionStagedAttachment{{Path: fontStage, SafeName: "Font.ttf", Decision: SubtitleCompositionAttachmentDecision{OriginalName: "Font.ttf", SafeName: "Font.ttf", Digest: "digest", Action: "added"}}}
	if err := publishSubtitleCompositionUnit("subtitle-compose-attachment", context.Background(), subtitleCompositionOperationRequest{OutputPath: output, OutputIdentity: "identity"}, stage, output, view, staged); err != nil {
		t.Fatal(err)
	}
	fontPath := filepath.Join(strings.TrimSuffix(output, filepath.Ext(output))+".fonts", "Font.ttf")
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(fontPath); err != nil || string(content) != "font" {
		t.Fatalf("published font attachment=%q err=%v", content, err)
	}

	cancelStage := filepath.Join(directory, "cancel.ass")
	cancelFont := filepath.Join(directory, "cancel.ttf")
	cancelOutput := filepath.Join(directory, "cancel-output.ass")
	_ = os.WriteFile(cancelStage, []byte("ASS"), 0o644)
	_ = os.WriteFile(cancelFont, []byte("font"), 0o644)
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := publishSubtitleCompositionUnit("subtitle-compose-cancel-attachment", cancelCtx, subtitleCompositionOperationRequest{OutputPath: cancelOutput, OutputIdentity: "cancel"}, cancelStage, cancelOutput, &subtitleCompositionOperationView{OutputIdentity: "cancel"}, []subtitleCompositionStagedAttachment{{Path: cancelFont, SafeName: "Font.ttf", Decision: SubtitleCompositionAttachmentDecision{SafeName: "Font.ttf", Digest: "cancel", Action: "added"}}}); err == nil {
		t.Fatal("cancelled attachment publication crossed the barrier")
	}
	if _, err := os.Stat(cancelOutput); !os.IsNotExist(err) {
		t.Fatalf("cancelled attachment publication left ASS: %v", err)
	}
	cancelFontPath := filepath.Join(strings.TrimSuffix(cancelOutput, filepath.Ext(cancelOutput))+".fonts", "Font.ttf")
	if _, err := os.Stat(cancelFontPath); !os.IsNotExist(err) {
		t.Fatalf("cancelled attachment publication left font: %v", err)
	}

	failureStage := filepath.Join(directory, "failure.ass")
	failureFont := filepath.Join(directory, "failure.ttf")
	failureOutput := filepath.Join(directory, "failure-output.ass")
	_ = os.WriteFile(failureStage, []byte("ASS"), 0o644)
	_ = os.WriteFile(failureFont, []byte("new-font"), 0o644)
	failureDir := strings.TrimSuffix(failureOutput, filepath.Ext(failureOutput)) + ".fonts"
	if err := os.MkdirAll(failureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existingFont := filepath.Join(failureDir, "Font.ttf")
	if err := os.WriteFile(existingFont, []byte("existing-font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishSubtitleCompositionUnit("subtitle-compose-failure-attachment", context.Background(), subtitleCompositionOperationRequest{OutputPath: failureOutput, OutputIdentity: "failure"}, failureStage, failureOutput, &subtitleCompositionOperationView{OutputIdentity: "failure"}, []subtitleCompositionStagedAttachment{{Path: failureFont, SafeName: "Font.ttf", Decision: SubtitleCompositionAttachmentDecision{SafeName: "Font.ttf", Digest: "failure", Action: "added"}}}); err == nil {
		t.Fatal("attachment collision did not fail closed")
	}
	if _, err := os.Stat(failureOutput); !os.IsNotExist(err) {
		t.Fatalf("failed attachment publication left ASS: %v", err)
	}
	if content, err := os.ReadFile(existingFont); err != nil || string(content) != "existing-font" {
		t.Fatalf("failed attachment publication damaged existing font=%q err=%v", content, err)
	}
}
