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
