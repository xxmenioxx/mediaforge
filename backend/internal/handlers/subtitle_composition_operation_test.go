package handlers

import (
	"context"
	"errors"
	"io"
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

func TestEmbeddedSubtitleCompositionUsesCanonicalRepresentationPreservingExtraction(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalRunner := subtitleCompositionRunCommand
	defer func() { subtitleCompositionRunCommand = originalRunner }()
	var commands [][]string
	subtitleCompositionRunCommand = func(_ context.Context, _ string, args []string, _ io.Writer, _ io.Writer) error {
		commands = append(commands, append([]string(nil), args...))
		output := args[len(args)-1]
		content := ""
		if strings.Contains(strings.Join(args, " "), "-f srt") {
			content = "1\n00:00:01,000 --> 00:00:02,000\nEmbedded SRT\n"
		} else {
			content = "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,Embedded ASS\n"
		}
		return os.WriteFile(output, []byte(content), 0o644)
	}
	srtStream := FFProbeStream{Index: 2, CodecType: "subtitle", CodecName: "subrip"}
	assStream := FFProbeStream{Index: 3, CodecType: "subtitle", CodecName: "ass"}
	srtDocument, err := extractEmbeddedSubtitleCompositionDocument(context.Background(), mediaPath, srtStream)
	if err != nil {
		t.Fatal(err)
	}
	assDocument, err := extractEmbeddedSubtitleCompositionDocument(context.Background(), mediaPath, assStream)
	if err != nil {
		t.Fatal(err)
	}
	if len(srtDocument.Events) != 1 || srtDocument.Events[0].Fields[len(srtDocument.Events[0].Fields)-1] != "Embedded SRT" {
		t.Fatalf("embedded SRT was not normalized through parseCompositionSRT: %#v", srtDocument)
	}
	if len(assDocument.Events) != 1 || assDocument.Events[0].Style != "Default" {
		t.Fatalf("embedded ASS did not preserve shared ASS structure: %#v", assDocument)
	}
	for _, args := range commands {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "-c:s copy -f ass") && strings.HasSuffix(args[len(args)-1], ".srt") {
			t.Fatalf("embedded SRT used an ASS muxer: %v", args)
		}
	}
	if !strings.Contains(strings.Join(commands[0], " "), "-f srt") {
		t.Fatalf("embedded SRT did not use the existing SRT extraction boundary: %v", commands[0])
	}
	if _, err := extractEmbeddedSubtitleCompositionDocument(context.Background(), mediaPath, FFProbeStream{Index: 4, CodecType: "subtitle", CodecName: "hdmv_pgs_subtitle"}); err == nil {
		t.Fatal("bitmap extraction bypassed the canonical OCR boundary")
	}

	primarySource := compositionTestSource("embedded-srt", true, SubtitleRoleFull, CompositionCueDialogue)
	primarySource.Representation = "srt"
	supplementalSource := compositionTestSource("embedded-ass", false, SubtitleRoleSigns, CompositionCueSign)
	if _, _, _, err := ComposeSubtitleDocuments(compositionTestPolicy(), SubtitleCompositionDocument{Source: primarySource, Document: srtDocument}, SubtitleCompositionDocument{Source: supplementalSource, Document: assDocument}); err != nil {
		t.Fatalf("embedded SRT primary plus ASS supplemental composition failed: %v", err)
	}
	assPrimarySource := compositionTestSource("embedded-ass-primary", true, SubtitleRoleFull, CompositionCueDialogue)
	srtSupplementalSource := compositionTestSource("embedded-srt-supplemental", false, SubtitleRoleFull, CompositionCueDialogue)
	if _, _, _, err := ComposeSubtitleDocuments(compositionTestPolicy(), SubtitleCompositionDocument{Source: assPrimarySource, Document: assDocument}, SubtitleCompositionDocument{Source: srtSupplementalSource, Document: srtDocument}); err != nil {
		t.Fatalf("ASS primary plus embedded SRT supplemental composition failed: %v", err)
	}
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

func TestSubtitleCompositionRetryRequiresRetryableTerminalState(t *testing.T) {
	directory := t.TempDir()
	request := compositionOperationRequestFixture(t, directory)
	for _, test := range []struct {
		name   string
		status string
	}{
		{name: "running", status: "running"},
		{name: "completed", status: "completed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := "subtitle-compose-retry-" + test.name
			subtitleExtractionOperations.Lock()
			subtitleExtractionOperations.items[id] = &subtitleExtractionOperation{ID: id, Kind: "compose", Status: test.status, Retryable: true, compositionRequest: &request}
			subtitleExtractionOperations.Unlock()
			defer func() {
				subtitleExtractionOperations.Lock()
				delete(subtitleExtractionOperations.items, id)
				subtitleExtractionOperations.Unlock()
			}()
			if _, err := subtitleCompositionRetryRequest(id); err == nil {
				t.Fatalf("%s composition was accepted for retry", test.status)
			}
		})
	}
	for _, test := range []struct {
		name   string
		status string
	}{
		{name: "failed", status: "error"},
		{name: "cancelled", status: "cancelled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := "subtitle-compose-retry-" + test.name
			subtitleExtractionOperations.Lock()
			subtitleExtractionOperations.items[id] = &subtitleExtractionOperation{ID: id, Kind: "compose", Status: test.status, Retryable: true, compositionRequest: &request}
			subtitleExtractionOperations.Unlock()
			defer func() {
				subtitleExtractionOperations.Lock()
				delete(subtitleExtractionOperations.items, id)
				subtitleExtractionOperations.Unlock()
			}()
			if _, err := subtitleCompositionRetryRequest(id); err != nil {
				t.Fatalf("%s composition was not accepted for retry: %v", test.status, err)
			}
		})
	}
}

func TestSubtitleCompositionRetryPreservesTerminalOperationAndRejectsStaleSnapshot(t *testing.T) {
	directory := t.TempDir()
	request := compositionOperationRequestFixture(t, directory)
	oldID := "subtitle-compose-retry-old"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[oldID] = &subtitleExtractionOperation{ID: oldID, Kind: "compose", Status: "error", Retryable: true, compositionRequest: &request}
	subtitleExtractionOperations.Unlock()
	newRequest, err := subtitleCompositionRetryRequest(oldID)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := startSubtitleCompositionOperation(newRequest)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForSubtitleCompositionOperation(t, newID)
	if newID == oldID || completed.Status != "completed" || completed.Retryable {
		t.Fatalf("retry did not create an independent successful operation: old=%s new=%s result=%#v", oldID, newID, completed)
	}
	subtitleExtractionOperations.RLock()
	old := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[oldID])
	subtitleExtractionOperations.RUnlock()
	if old.Status != "error" {
		t.Fatalf("old composition operation was mutated by retry: %#v", old)
	}

	staleID := "subtitle-compose-retry-stale"
	if err := os.WriteFile(strings.TrimPrefix(request.Supplementals[0].Source.Identity, "external:"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[staleID] = &subtitleExtractionOperation{ID: staleID, Kind: "compose", Status: "error", Retryable: true, compositionRequest: &request}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, oldID)
		delete(subtitleExtractionOperations.items, newID)
		delete(subtitleExtractionOperations.items, staleID)
		subtitleExtractionOperations.Unlock()
	}()
	if _, err := subtitleCompositionRetryRequest(staleID); !errors.Is(err, errSubtitleCompositionRetryStaleSource) {
		t.Fatalf("stale composition retry error=%v", err)
	}
}

func TestSubtitleCompositionTerminalRetryabilityDistinguishesStaleAndCancelled(t *testing.T) {
	staleID := "subtitle-compose-terminal-stale"
	cancelledID := "subtitle-compose-terminal-cancelled"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[staleID] = &subtitleExtractionOperation{ID: staleID, Kind: "compose", Status: "running", publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.items[cancelledID] = &subtitleExtractionOperation{ID: cancelledID, Kind: "compose", Status: "running", publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, staleID)
		delete(subtitleExtractionOperations.items, cancelledID)
		subtitleExtractionOperations.Unlock()
	}()
	finishSubtitleCompositionOperation(staleID, context.Background(), errors.New("stale_source: frozen source changed"), false)
	stale := snapshotSubtitleOperation(staleID)
	if stale.Status != "error" || stale.Retryable {
		t.Fatalf("stale terminal retryability=%#v", stale)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	finishSubtitleCompositionOperation(cancelledID, ctx, context.Canceled, false)
	cancelled := snapshotSubtitleOperation(cancelledID)
	if cancelled.Status != "cancelled" || !cancelled.Retryable {
		t.Fatalf("cancelled terminal retryability=%#v", cancelled)
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
