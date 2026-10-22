package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListSubtitleTranslationQualityReviewsIsScopedToAssetAndIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:translation-quality-reviews-%p?mode=memory&cache=shared", t)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubtitleTranslationQualityReview{}); err != nil {
		t.Fatal(err)
	}
	for _, review := range []models.SubtitleTranslationQualityReview{
		{OperationID: "op-current", AssetPath: "/media/a.mkv", SourceIdentity: "source-a", SourceRevision: "rev-a", QualityReport: `{"policyVersion":"v1","state":"review_required","cueCount":1,"cleanCount":0,"warningCount":1,"reviewCount":1,"findings":[{"cueId":"cue-1","sourceOrdinal":0,"sourceText":"hi","translatedText":"","reasons":["empty_translation"]}]}`, Decisions: "[]", Status: "open"},
		{OperationID: "op-other-source", AssetPath: "/media/a.mkv", SourceIdentity: "source-b", SourceRevision: "rev-b", QualityReport: `{"policyVersion":"v1","state":"clean","cueCount":1,"cleanCount":1,"warningCount":0,"reviewCount":0}`, Decisions: "[]", Status: "reviewed"},
		{OperationID: "op-other-asset", AssetPath: "/media/b.mkv", SourceIdentity: "source-a", SourceRevision: "rev-a", QualityReport: `{"policyVersion":"v1","state":"clean","cueCount":1,"cleanCount":1,"warningCount":0,"reviewCount":0}`, Decisions: "[]", Status: "reviewed"},
	} {
		if err := db.Create(&review).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/reviews", (AssetHandler{db: db}).ListSubtitleTranslationQualityReviews)
	request := httptest.NewRequest(http.MethodGet, "/reviews?assetPath=%2Fmedia%2Fa.mkv&sourceIdentity=source-a", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !containsAll(body, `"op-current"`, `"persisted":true`) || containsAny(body, `"op-other-source"`, `"op-other-asset"`) {
		t.Fatalf("review list was not identity-scoped or persisted: %s", body)
	}
}

func TestListSubtitleTranslationQualityReviewsRequiresIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/reviews", (AssetHandler{}).ListSubtitleTranslationQualityReviews)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/reviews?assetPath=%2Fmedia%2Fa.mkv", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSubtitleTranslationQualityEditPublishesThroughCanonicalPipeline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-workspace"))
	mediaPath := filepath.Join(root, "episode.mkv")
	sourcePath := filepath.Join(root, "episode.en.srt")
	sourceContent := []byte("1\n00:00:01,000 --> 00:00:02,000\nWe waited until the train arrived.\n\n")
	fullTranslatedText := strings.TrimSpace(strings.Repeat("El tren llegó tarde. ", 24))
	translatedContent := []byte("1\n00:00:01,000 --> 00:00:02,000\n" + fullTranslatedText + "\n\n")
	if err := os.WriteFile(mediaPath, []byte("synthetic media identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, sourceContent, 0o600); err != nil {
		t.Fatal(err)
	}
	db := translationSourceTestDB(t, root, mediaPath)
	if err := db.AutoMigrate(&models.SubtitleTranslationQualityReview{}); err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler{db: db}
	request := subtitleTranslationOperationRequest{
		AssetPath: mediaPath, SourceType: "external", SourceReference: sourcePath,
		RequestedSourceLanguage: "en", TargetLocale: "es-419", RequestedRepresentation: translationRepresentationSRT,
		Provider: "ollama", ProviderModel: "qwen3:8b", ContextMode: "none", ContextSource: "none",
	}
	_, source, stream, err := handler.resolveTranslationExecutionSource(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	sourceRevision, err := translationSourceRevisionForResolvedSource(source, stream)
	if err != nil {
		t.Fatal(err)
	}
	request.SourceIdentity = source.SourceIdentity
	request.SourceRevision = sourceRevision.key()
	targets, err := translationPublicationTargets(request, source, "en", TranslationProviderMetrics{}, "translation-operation", map[string][]byte{
		translationRepresentationSRT: translatedContent,
	}, mediaPath)
	if err != nil || len(targets) != 1 {
		t.Fatalf("translation targets=%#v err=%v", targets, err)
	}
	target := targets[0]
	if err := os.WriteFile(target.FinalPath, translatedContent, 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := target.Artifact
	artifact.PublishedPath = target.FinalPath
	operationID := "quality-review-publish-e2e"
	report := subtitleTranslationQualityReport{
		PolicyVersion: subtitleTranslationQualityPolicyVersion,
		State:         "review_required",
		CueCount:      1,
		WarningCount:  1,
		ReviewCount:   1,
		Findings: []subtitleTranslationQualityFinding{{
			CueID: "cue-1", SourceOrdinal: 0, SourceText: "We waited until the train arrived.",
			TranslatedText: truncateSubtitleTranslationQualityText(fullTranslatedText), Reasons: []string{"meaning_drift"},
		}},
	}
	if err := persistSubtitleTranslationQualityReview(db, operationID, request, report, []SubtitleArtifact{artifact}); err != nil {
		t.Fatal(err)
	}
	var originalReview models.SubtitleTranslationQualityReview
	if err := db.First(&originalReview, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	var originalTargets []subtitleTranslationQualityTarget
	if err := json.Unmarshal([]byte(originalReview.TargetsMetadata), &originalTargets); err != nil || len(originalTargets) != 1 {
		t.Fatalf("original target revision is unavailable: %#v err=%v", originalTargets, err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/reviews/:id/decision", handler.DecideSubtitleTranslationQuality)
	router.GET("/reviews/:id", handler.GetSubtitleTranslationQualityReview)
	editableCueResponse := httptest.NewRecorder()
	router.ServeHTTP(editableCueResponse, httptest.NewRequest(http.MethodGet, "/reviews/"+operationID+"?cueId=cue-1", nil))
	if editableCueResponse.Code != http.StatusOK {
		t.Fatalf("editable cue response status=%d body=%s", editableCueResponse.Code, editableCueResponse.Body.String())
	}
	var editableCue subtitleTranslationQualityEditableCueResponse
	if err := json.Unmarshal(editableCueResponse.Body.Bytes(), &editableCue); err != nil ||
		editableCue.Text != fullTranslatedText || editableCue.SourceIdentity != request.SourceIdentity ||
		editableCue.SourceRevision != request.SourceRevision || len(editableCue.TargetRevisions) != 1 {
		t.Fatalf("editable cue was not returned in full with its verified identity: cue=%#v err=%v", editableCue, err)
	}
	originalInfo, err := os.Stat(target.FinalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.FinalPath, []byte("stale sidecar revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleCueResponse := httptest.NewRecorder()
	router.ServeHTTP(staleCueResponse, httptest.NewRequest(http.MethodGet, "/reviews/"+operationID+"?cueId=cue-1", nil))
	if staleCueResponse.Code != http.StatusConflict {
		t.Fatalf("stale editable cue status=%d body=%s, want conflict", staleCueResponse.Code, staleCueResponse.Body.String())
	}
	if err := os.WriteFile(target.FinalPath, translatedContent, originalInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target.FinalPath, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	decision := `{"cueId":"cue-1","action":"edit","text":"Esperamos hasta que llegó el tren."}`
	response := httptest.NewRecorder()
	if err := db.Exec("CREATE TRIGGER fail_quality_review_decision BEFORE UPDATE ON subtitle_translation_quality_reviews WHEN NEW.decisions <> OLD.decisions BEGIN SELECT RAISE(ABORT, 'injected review decision failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reviews/"+operationID+"/decision", strings.NewReader(decision)))
	if response.Code != http.StatusConflict {
		t.Fatalf("database failure status=%d body=%s, want conflict", response.Code, response.Body.String())
	}
	rolledBack, err := os.ReadFile(target.FinalPath)
	if err != nil || string(rolledBack) != string(translatedContent) {
		t.Fatalf("database failure did not compensate the published file: %q (%v)", rolledBack, err)
	}
	var afterFailure models.SubtitleTranslationQualityReview
	if err := db.First(&afterFailure, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	restoredRevision, revisionErr := captureTranslationExternalSourceRevision(target.FinalPath, "")
	if afterFailure.Status != "open" || afterFailure.Decisions != "[]" ||
		afterFailure.TargetsMetadata != originalReview.TargetsMetadata ||
		revisionErr != nil || restoredRevision.key() != originalTargets[0].SourceRevision {
		t.Fatalf("failed edit left review metadata inconsistent: %#v", afterFailure)
	}
	if err := db.Exec("DROP TRIGGER fail_quality_review_decision").Error; err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reviews/"+operationID+"/decision", strings.NewReader(decision)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"reviewed"`) {
		t.Fatalf("edit decision status=%d body=%s", response.Code, response.Body.String())
	}
	updated, err := os.ReadFile(target.FinalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "00:00:01,000 --> 00:00:02,000") || !strings.Contains(string(updated), "Esperamos hasta que llegó el tren.") || strings.Contains(string(updated), "El tren llegó tarde.") {
		t.Fatalf("canonical publisher did not preserve SRT timing and publish the approved cue: %q", updated)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || string(got) != string(sourceContent) {
		t.Fatalf("source sidecar changed: err=%v content=%q", err, got)
	}
	var saved models.SubtitleTranslationQualityReview
	if err := db.First(&saved, "operation_id = ?", operationID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != "reviewed" {
		t.Fatalf("review status=%q, want reviewed", saved.Status)
	}
	var decisions []subtitleTranslationQualityDecision
	if err := json.Unmarshal([]byte(saved.Decisions), &decisions); err != nil || len(decisions) != 1 || decisions[0].Action != "edit" || decisions[0].Text != "Esperamos hasta que llegó el tren." {
		t.Fatalf("persisted edit decision=%#v err=%v", decisions, err)
	}
	targetRevision, err := captureTranslationExternalSourceRevision(target.FinalPath, "")
	if err != nil {
		t.Fatal(err)
	}
	var persistedTargets []subtitleTranslationQualityTarget
	if err := json.Unmarshal([]byte(saved.TargetsMetadata), &persistedTargets); err != nil || len(persistedTargets) != 1 || persistedTargets[0].SourceRevision != targetRevision.key() {
		t.Fatalf("published target revision was not persisted: targets=%#v err=%v", persistedTargets, err)
	}

	duplicate := httptest.NewRecorder()
	router.ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/reviews/"+operationID+"/decision", strings.NewReader(decision)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate decision status=%d body=%s, want conflict", duplicate.Code, duplicate.Body.String())
	}
}

func containsAll(value string, expected ...string) bool {
	for _, item := range expected {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}

func containsAny(value string, unexpected ...string) bool {
	for _, item := range unexpected {
		if strings.Contains(value, item) {
			return true
		}
	}
	return false
}
