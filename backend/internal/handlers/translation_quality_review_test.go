package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
