package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newOCRDictionaryTestAPI(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ocr-dictionary-api.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.OCRDictionaryEntry{}); err != nil {
		t.Fatal(err)
	}
	builtin := models.OCRDictionaryEntry{
		ID: "builtin:eng:im", Language: "eng", Pattern: "|'m", Replacement: "I'm",
		MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100,
		Enabled: true, Source: "builtin", RuleVersion: 1,
	}
	if err := db.Create(&builtin).Error; err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler{db: db}
	router := gin.New()
	router.GET("/entries", handler.ListOCRDictionary)
	router.POST("/entries", handler.CreateOCRDictionaryEntry)
	router.POST("/test-match", handler.TestOCRDictionaryRule)
	router.PUT("/entries/:id", handler.UpdateOCRDictionaryEntry)
	router.PATCH("/entries/:id/enabled", handler.SetOCRDictionaryEntryEnabled)
	router.DELETE("/entries/:id", handler.DeleteOCRDictionaryEntry)
	return db, router
}

func performOCRDictionaryRequest(t *testing.T, router http.Handler, method, path string, value any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if value != nil {
		if err := json.NewEncoder(&body).Encode(value); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestOCRDictionaryCRUDSearchValidationAndBuiltInProtection(t *testing.T) {
	db, router := newOCRDictionaryTestAPI(t)
	create := map[string]any{
		"language": "en", "pattern": "| hello", "replacement": "I hello",
		"matchMode": "literal", "boundaryPolicy": "word", "casePolicy": "exact", "priority": 5,
	}
	created := performOCRDictionaryRequest(t, router, http.MethodPost, "/entries", create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}
	var entry models.OCRDictionaryEntry
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(entry.ID, "user:") || entry.Language != "eng" || entry.RuleVersion != 1 || entry.Source != "user" {
		t.Fatalf("created entry has invalid server-owned fields: %#v", entry)
	}

	duplicate := performOCRDictionaryRequest(t, router, http.MethodPost, "/entries", create)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate create returned %d: %s", duplicate.Code, duplicate.Body.String())
	}
	unknownLanguage := map[string]any{"language": "xx", "pattern": "abc", "replacement": "def"}
	if response := performOCRDictionaryRequest(t, router, http.MethodPost, "/entries", unknownLanguage); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown language returned %d: %s", response.Code, response.Body.String())
	}
	unsafeBar := map[string]any{"language": "eng", "pattern": "|", "replacement": "I"}
	if response := performOCRDictionaryRequest(t, router, http.MethodPost, "/entries", unsafeBar); response.Code != http.StatusBadRequest {
		t.Fatalf("uncontextualized bar rule returned %d: %s", response.Code, response.Body.String())
	}

	search := performOCRDictionaryRequest(t, router, http.MethodGet, "/entries?language=english&source=user&search=hello&page=1&pageSize=1", nil)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), entry.ID) || !strings.Contains(search.Body.String(), `"total":1`) {
		t.Fatalf("bounded filtered search returned %d: %s", search.Code, search.Body.String())
	}

	update := map[string]any{
		"language": "eng", "pattern": "| hi", "replacement": "I hi",
		"matchMode": "literal", "boundaryPolicy": "word", "casePolicy": "exact", "priority": 7,
	}
	updated := performOCRDictionaryRequest(t, router, http.MethodPut, "/entries/"+entry.ID, update)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"ruleVersion":2`) || !strings.Contains(updated.Body.String(), `"pattern":"| hi"`) {
		t.Fatalf("user update returned %d: %s", updated.Code, updated.Body.String())
	}
	if response := performOCRDictionaryRequest(t, router, http.MethodPut, "/entries/builtin:eng:im", update); response.Code != http.StatusConflict {
		t.Fatalf("built-in edit returned %d: %s", response.Code, response.Body.String())
	}
	if response := performOCRDictionaryRequest(t, router, http.MethodDelete, "/entries/builtin:eng:im", nil); response.Code != http.StatusConflict {
		t.Fatalf("built-in deletion returned %d: %s", response.Code, response.Body.String())
	}
	if response := performOCRDictionaryRequest(t, router, http.MethodPatch, "/entries/builtin:eng:im/enabled", map[string]bool{"enabled": false}); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"enabled":false`) {
		t.Fatalf("built-in toggle returned %d: %s", response.Code, response.Body.String())
	}
	if response := performOCRDictionaryRequest(t, router, http.MethodDelete, "/entries/"+entry.ID, nil); response.Code != http.StatusNoContent {
		t.Fatalf("user deletion returned %d: %s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&models.OCRDictionaryEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("expected only protected built-in to remain, count=%d err=%v", count, err)
	}
}

func TestOCRDictionaryTestMatchPreviewsWithoutPersistence(t *testing.T) {
	db, router := newOCRDictionaryTestAPI(t)
	rule := map[string]any{
		"pattern": "| hello", "replacement": "I hello", "matchMode": "literal",
		"boundaryPolicy": "word", "casePolicy": "exact",
	}
	request := map[string]any{"language": "eng", "sample": "Maybe | hello, friend.", "rule": rule}
	response := performOCRDictionaryRequest(t, router, http.MethodPost, "/test-match", request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"result":"Maybe I hello, friend."`) || !strings.Contains(response.Body.String(), `"matchCount":1`) {
		t.Fatalf("test-match returned %d: %s", response.Code, response.Body.String())
	}
	if response := performOCRDictionaryRequest(t, router, http.MethodPost, "/test-match", map[string]any{"language": "eng", "sample": strings.Repeat("x", ocrDictionaryMaxSampleBytes+1), "rule": rule}); response.Code != http.StatusBadRequest {
		t.Fatalf("oversized sample returned %d: %s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&models.OCRDictionaryEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("test-match persisted a rule, count=%d err=%v", count, err)
	}
}
