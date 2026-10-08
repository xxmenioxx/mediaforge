package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOCRDictionaryReevaluationBindsLanguageRevisionsAndPublishesThroughCorrection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "dictionary.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.OCRDictionaryEntry{}); err != nil {
		t.Fatal(err)
	}
	rule := testOCRDictionaryEntry("builtin:eng:im", "|'m", "I'm", 100)
	rule.Source = "builtin"
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}

	sidecar := filepath.Join(root, "episode.srt")
	source := filepath.Join(root, "source.mkv")
	originalText := "Well, |'m here."
	writeTestSRT(t, sidecar, originalText)
	if err := os.WriteFile(source, []byte("test source identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	evidence := &canonicalOCREvidence{
		Identity:          canonicalOCRIdentity{SourcePath: source, SourceSize: info.Size(), SourceModTimeUnixNano: info.ModTime().UnixNano(), SourceSnapshotVersion: "stat-v1", RequestedLanguage: "eng"},
		RequestedLanguage: "eng", Complete: true, Integrity: "published-canonical-cues-v1",
		Cues: []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: originalText, SourceOrdinal: 17}},
	}
	logical := "subtitle:17:generated:srt"
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", logical, evidence, "initial-publication", ""); err != nil {
		t.Fatal(err)
	}
	owner := publishedSubtitleOwnerIdentity(sidecar, "srt", logical)
	snapshotPath := publishedSubtitleSnapshotPath(owner)
	originalSidecar, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	originalSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}

	legacyPreview, err := buildOCRDictionaryReevaluationPreview(db, ocrDictionaryReevaluationInput{SidecarPath: sidecar, Format: "srt", LogicalTransformKey: logical})
	if err != nil {
		t.Fatal(err)
	}
	if !legacyPreview.preview.RequiresLanguageChoice || legacyPreview.preview.DictionaryLanguage != "" || len(legacyPreview.preview.Changes) != 0 {
		t.Fatalf("legacy normalized eng inherited English rules: %#v", legacyPreview.preview)
	}
	selectedPreview, err := buildOCRDictionaryReevaluationPreview(db, ocrDictionaryReevaluationInput{SidecarPath: sidecar, Format: "srt", LogicalTransformKey: logical, DictionaryLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if selectedPreview.preview.RequiresLanguageChoice || selectedPreview.preview.DictionaryLanguage != "eng" || selectedPreview.preview.DictionaryLanguageSource != "reevaluation-selection" || len(selectedPreview.preview.Changes) != 1 {
		t.Fatalf("explicit language was not bound to the preview: %#v", selectedPreview.preview)
	}
	change := selectedPreview.preview.Changes[0]
	if change.CueID != 17 || change.Before != originalText || change.After != "Well, I'm here." || len(change.RuleIDs) != 1 || change.RuleIDs[0] != rule.ID {
		t.Fatalf("unexpected preview change: %#v", change)
	}
	unchangedSidecar, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	unchangedSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(originalSidecar, unchangedSidecar) || !bytes.Equal(originalSnapshot, unchangedSnapshot) {
		t.Fatal("re-evaluation preview changed the published sidecar or canonical snapshot")
	}

	gin.SetMode(gin.TestMode)
	handler := AssetHandler{db: db}
	router := gin.New()
	router.POST("/apply", handler.ApplyOCRDictionaryReevaluation)
	applyBody := func(preview ocrDictionaryReevaluationPreview) map[string]any {
		return map[string]any{
			"sidecarPath": sidecar, "format": "srt", "logicalTransformKey": logical,
			"dictionaryLanguage": "en", "previewId": preview.PreviewID,
			"artifactRevision": preview.ArtifactRevision, "dictionaryRevision": preview.DictionaryRevision,
			"acceptedCueIds": []int{17},
		}
	}
	if err := db.Model(&models.OCRDictionaryEntry{}).Where("id = ?", rule.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	staleBody, _ := json.Marshal(applyBody(selectedPreview.preview))
	staleRecorder := httptest.NewRecorder()
	staleRouterRequest := httptest.NewRequest(http.MethodPost, "/apply", bytes.NewReader(staleBody))
	staleRouterRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(staleRecorder, staleRouterRequest)
	if staleRecorder.Code != http.StatusConflict {
		t.Fatalf("stale dictionary preview returned %d: %s", staleRecorder.Code, staleRecorder.Body.String())
	}
	stillOriginal, err := os.ReadFile(sidecar)
	if err != nil || !bytes.Equal(stillOriginal, originalSidecar) {
		t.Fatalf("stale preview changed published output: %q (%v)", stillOriginal, err)
	}
	if err := db.Model(&models.OCRDictionaryEntry{}).Where("id = ?", rule.ID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	currentPreview, err := buildOCRDictionaryReevaluationPreview(db, ocrDictionaryReevaluationInput{SidecarPath: sidecar, Format: "srt", LogicalTransformKey: logical, DictionaryLanguage: "en"})
	if err != nil {
		t.Fatal(err)
	}
	freshBody, _ := json.Marshal(applyBody(currentPreview.preview))
	freshRecorder := httptest.NewRecorder()
	freshRequest := httptest.NewRequest(http.MethodPost, "/apply", bytes.NewReader(freshBody))
	freshRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(freshRecorder, freshRequest)
	if freshRecorder.Code != http.StatusAccepted {
		t.Fatalf("fresh preview apply returned %d: %s", freshRecorder.Code, freshRecorder.Body.String())
	}
	var accepted struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(freshRecorder.Body.Bytes(), &accepted); err != nil || accepted.OperationID == "" {
		t.Fatalf("apply response has no operation id: %s (%v)", freshRecorder.Body.String(), err)
	}
	waitForOCRDictionaryCorrection(t, accepted.OperationID)

	correctedSidecar, err := os.ReadFile(sidecar)
	if err != nil || !bytes.Contains(correctedSidecar, []byte("Well, I'm here.")) {
		t.Fatalf("accepted dictionary correction did not publish: %q (%v)", correctedSidecar, err)
	}
	current, ok := loadPublishedCanonicalOCRSnapshot(sidecar, "srt", logical)
	if !ok || current.Evidence.Cues[0].Text != "Well, I'm here." || current.Evidence.DictionaryLanguage != "eng" || len(current.History) != 1 {
		t.Fatalf("corrected canonical baseline or bounded history is missing: %#v", current)
	}
	if current.History[0].Evidence.Cues[0].Text != originalText {
		t.Fatalf("predecessor evidence does not preserve the pre-correction cue: %#v", current.History[0])
	}
	provenance := current.Evidence.CorrectionProvenance
	if len(provenance) != 1 || provenance[0].OperationID != accepted.OperationID || provenance[0].DictionaryRevision != currentPreview.preview.DictionaryRevision || len(provenance[0].DictionaryRuleIDs) != 1 || provenance[0].DictionaryRuleIDs[0] != rule.ID || provenance[0].BeforeTextHash == provenance[0].AfterTextHash {
		t.Fatalf("committed correction provenance is incomplete: %#v", provenance)
	}
}

func waitForOCRDictionaryCorrection(t *testing.T, operationID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		subtitleExtractionOperations.RLock()
		operation := subtitleExtractionOperations.items[operationID]
		status, message := "", ""
		if operation != nil {
			status, message = operation.Status, operation.Error
		}
		subtitleExtractionOperations.RUnlock()
		if status == "completed" {
			return
		}
		if status == "error" || status == "cancelled" {
			t.Fatalf("OCR correction operation %s ended %s: %s", operationID, status, message)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("OCR correction operation %s did not complete", operationID)
}
