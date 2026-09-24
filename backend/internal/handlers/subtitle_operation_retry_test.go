package handlers

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSubtitleRestyleRetryUsesBoundedSnapshotAndFreshOperation(t *testing.T) {
	for _, status := range []string{"error", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			root := t.TempDir()
			sourcePath := filepath.Join(root, "source.ass")
			source := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hello\n"
			if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			revision, err := mediaFileFingerprint(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			oldID := "subtitle-retry-" + status
			request := subtitleRestyleRequest{Mode: "restyle", SourcePath: sourcePath, SourceRevision: revision, TargetPath: filepath.Join(root, "source.styled.ass"), Representation: "ass", Policy: defaultSubtitleStylePolicy()}
			subtitleExtractionOperations.Lock()
			subtitleExtractionOperations.items[oldID] = &subtitleExtractionOperation{ID: oldID, Kind: "restyle", Status: status, restyleRequest: &request}
			subtitleExtractionOperations.Unlock()
			newID := ""
			defer func() {
				subtitleExtractionOperations.Lock()
				delete(subtitleExtractionOperations.items, oldID)
				if newID != "" {
					delete(subtitleExtractionOperations.items, newID)
				}
				subtitleExtractionOperations.Unlock()
			}()

			fresh, err := subtitleRestyleRetryRequest(oldID)
			if err != nil {
				t.Fatal(err)
			}
			newID, err = startSubtitleRestyleOperation(fresh)
			if err != nil {
				t.Fatal(err)
			}
			if newID == oldID {
				t.Fatal("retry reused the old operation ID")
			}
			completed := waitForSubtitleOperationStatus(t, newID, "completed")
			if completed.Status != "completed" || completed.Retryable {
				t.Fatalf("retry did not complete as a fresh non-retryable operation: %#v", completed)
			}
		})
	}
}

func TestSubtitleRestyleRetryRejectsCompletedAndStaleSources(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.ass")
	if err := os.WriteFile(sourcePath, []byte("source-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	request := subtitleRestyleRequest{Mode: "restyle", SourcePath: sourcePath, SourceRevision: revision, Representation: "ass", Policy: defaultSubtitleStylePolicy()}
	completedID := "subtitle-retry-completed"
	staleID := "subtitle-retry-stale"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[completedID] = &subtitleExtractionOperation{ID: completedID, Kind: "restyle", Status: "completed", restyleRequest: &request}
	subtitleExtractionOperations.items[staleID] = &subtitleExtractionOperation{ID: staleID, Kind: "restyle", Status: "error", restyleRequest: &request}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, completedID)
		delete(subtitleExtractionOperations.items, staleID)
		subtitleExtractionOperations.Unlock()
	}()
	if _, err := subtitleRestyleRetryRequest(completedID); err == nil {
		t.Fatal("completed restyle was retryable")
	}
	if err := os.WriteFile(sourcePath, []byte("source-b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := subtitleRestyleRetryRequest(staleID); !errors.Is(err, errSubtitleRestyleRetryStaleSource) {
		t.Fatalf("stale source retry error=%v", err)
	}
}

func TestRetrySubtitleOperationDispatchesRestyleThroughSharedRegistry(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.ass")
	source := "[Script Info]\nScriptType: v4.00+\n\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,Hello\n"
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	revision, err := mediaFileFingerprint(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	oldID := "subtitle-dispatch-restyle"
	request := subtitleRestyleRequest{Mode: "restyle", SourcePath: sourcePath, SourceRevision: revision, TargetPath: filepath.Join(root, "source.styled.ass"), Representation: "ass", Policy: defaultSubtitleStylePolicy()}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[oldID] = &subtitleExtractionOperation{ID: oldID, Kind: "restyle", Status: "error", restyleRequest: &request}
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, oldID)
		subtitleExtractionOperations.Unlock()
	}()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: oldID}}
	(AssetHandler{}).RetrySubtitleOperation(context)
	if recorder.Code != 202 {
		t.Fatalf("retry dispatcher status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Operation struct {
			ID string `json:"id"`
		} `json:"operation"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Operation.ID == "" || response.Operation.ID == oldID {
		t.Fatalf("retry dispatcher response=%s err=%v", recorder.Body.String(), err)
	}
	completed := waitForSubtitleOperationStatus(t, response.Operation.ID, "completed")
	if completed.Kind != "restyle" {
		t.Fatalf("dispatcher created wrong operation kind: %#v", completed)
	}
	subtitleExtractionOperations.Lock()
	delete(subtitleExtractionOperations.items, response.Operation.ID)
	subtitleExtractionOperations.Unlock()
}
