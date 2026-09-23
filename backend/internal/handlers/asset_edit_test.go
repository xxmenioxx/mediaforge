package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAssetEditTestServer(t *testing.T) (*gorm.DB, *gin.Engine, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:asset-edit-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.Library{}, &models.AssetRecord{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := db.Create(&models.Library{Name: "Test", SourcePath: root, DestinationPath: root}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAssetHandler(db)
	router.POST("/api/assets/edit", handler.Edit)
	router.POST("/api/assets/metadata", handler.UpdateMetadata)
	router.POST("/api/assets/conversion", handler.UpdateConversion)
	return db, router, root
}

func assetEditRequest(t *testing.T, router http.Handler, path string, input AssetEditInput) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/assets/edit?path="+url.QueryEscape(path), bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAssetEditPersistsNormalizedMetadataAndConversion(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	response := assetEditRequest(t, router, path, AssetEditInput{
		Metadata: AssetMetadataUpdateInput{Categories: []string{" Drama ", "Drama"}, Tags: []string{" tag "}},
		Conversion: AssetConversionUpdateInput{
			KeepAudioStreams:      []int{2, 1, 2},
			SubtitleRoleOverrides: map[int]SubtitleRoleOverride{},
			UpscaleMode:           "1080p",
		},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result AssetEditResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Clean(path) || len(result.Metadata.Categories) != 1 || result.Metadata.Categories[0] != "Drama" || result.Conversion.UpscaleMode != "1080p" {
		t.Fatalf("unexpected compound response: %#v", result)
	}
	if got := assetMetadataOverrides(db)[filepath.Clean(path)]; len(got.Tags) != 1 || got.Tags[0] != "tag" {
		t.Fatalf("metadata was not persisted: %#v", got)
	}
	if got := assetConversionOverrides(db)[filepath.Clean(path)]; len(got.KeepAudioStreams) != 2 || got.KeepAudioStreams[0] != 1 || got.KeepAudioStreams[1] != 2 {
		t.Fatalf("conversion was not normalized and persisted: %#v", got)
	}
}

func TestAssetEditPreservesSubtitleRoleBindingNormalization(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	if err := db.Create(&models.ScanResult{
		Path:            path,
		SizeBytes:       5,
		Duration:        10,
		UpdatedAt:       time.Now(),
		SubtitleStreams: models.JSONList{map[string]any{"index": 3, "codec": "subrip", "language": "spa", "forced": false}},
	}).Error; err != nil {
		t.Fatal(err)
	}

	response := assetEditRequest(t, router, path, AssetEditInput{
		Conversion: AssetConversionUpdateInput{
			SubtitleRoleOverrides: map[int]SubtitleRoleOverride{3: {Role: SubtitleRoleCommentary}},
		},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result AssetEditResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	role, ok := result.Conversion.SubtitleRoleOverrides[3]
	if !ok || role.Role != SubtitleRoleCommentary || role.SourceSnapshotFingerprint == "" || role.StreamSignature == "" {
		t.Fatalf("subtitle role was not normalized and bound to the persisted scan: %#v", result.Conversion.SubtitleRoleOverrides)
	}
}

func TestAssetEditValidationAndAuthorizationDoNotMutate(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	if err := saveAssetMetadataOverrides(db, map[string]AssetMetadataState{filepath.Clean(path): {Categories: []string{"Existing"}}}); err != nil {
		t.Fatal(err)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{filepath.Clean(path): {UpscaleMode: "720p"}}); err != nil {
		t.Fatal(err)
	}
	response := assetEditRequest(t, router, path, AssetEditInput{
		Metadata:   AssetMetadataUpdateInput{Categories: []string{"New"}},
		Conversion: AssetConversionUpdateInput{UpscaleMode: "not-valid"},
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("validation status=%d body=%s", response.Code, response.Body.String())
	}
	if got := assetMetadataOverrides(db)[filepath.Clean(path)].Categories[0]; got != "Existing" {
		t.Fatalf("validation changed metadata to %q", got)
	}
	if got := assetConversionOverrides(db)[filepath.Clean(path)].UpscaleMode; got != "720p" {
		t.Fatalf("validation changed conversion to %q", got)
	}

	outside := filepath.Join(t.TempDir(), "outside.mkv")
	writeTestFile(t, outside, "media")
	response = assetEditRequest(t, router, outside, AssetEditInput{})
	if response.Code != http.StatusForbidden {
		t.Fatalf("authorization status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAssetEditRollsBackBothMapsWhenSecondStoreFails(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	if err := saveAssetMetadataOverrides(db, map[string]AssetMetadataState{cleanPath: {Categories: []string{"Before"}}}); err != nil {
		t.Fatal(err)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: {UpscaleMode: "720p"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_asset_conversion_update BEFORE UPDATE ON app_settings WHEN NEW.key = 'assetConversionOverrides' BEGIN SELECT RAISE(ABORT, 'forced conversion failure'); END`).Error; err != nil {
		t.Fatal(err)
	}

	response := assetEditRequest(t, router, path, AssetEditInput{
		Metadata:   AssetMetadataUpdateInput{Categories: []string{"After"}},
		Conversion: AssetConversionUpdateInput{UpscaleMode: "1080p"},
	})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("rollback status=%d body=%s", response.Code, response.Body.String())
	}
	if got := assetMetadataOverrides(db)[cleanPath].Categories[0]; got != "Before" {
		t.Fatalf("metadata was not rolled back: %q", got)
	}
	if got := assetConversionOverrides(db)[cleanPath].UpscaleMode; got != "720p" {
		t.Fatalf("conversion changed despite rollback: %q", got)
	}
}

func TestAssetEditConcurrentDifferentAssetsPreservesBothEntries(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	pathA := filepath.Join(root, "a.mkv")
	pathB := filepath.Join(root, "b.mkv")
	writeTestFile(t, pathA, "a")
	writeTestFile(t, pathB, "b")
	inputs := []struct {
		path string
		cat  string
	}{
		{pathA, "A"},
		{pathB, "B"},
	}
	responses := make([]*httptest.ResponseRecorder, len(inputs))
	var wait sync.WaitGroup
	for index, input := range inputs {
		wait.Add(1)
		go func(index int, input struct {
			path string
			cat  string
		}) {
			defer wait.Done()
			responses[index] = assetEditRequest(t, router, input.path, AssetEditInput{
				Metadata:   AssetMetadataUpdateInput{Categories: []string{input.cat}},
				Conversion: AssetConversionUpdateInput{KeepAudioStreams: []int{index + 1}},
			})
		}(index, input)
	}
	wait.Wait()
	for index, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
	metadata := assetMetadataOverrides(db)
	conversion := assetConversionOverrides(db)
	if metadata[filepath.Clean(pathA)].Categories[0] != "A" || metadata[filepath.Clean(pathB)].Categories[0] != "B" {
		t.Fatalf("concurrent metadata entries lost: %#v", metadata)
	}
	if len(conversion[filepath.Clean(pathA)].KeepAudioStreams) != 1 || len(conversion[filepath.Clean(pathB)].KeepAudioStreams) != 1 {
		t.Fatalf("concurrent conversion entries lost: %#v", conversion)
	}
}

func TestAssetEditCompatibilityEndpointsRetainContractsAndEmptySemantics(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)

	metadataBody := `{"categories":["Drama"],"tags":["tag"]}`
	request := httptest.NewRequest(http.MethodPost, "/api/assets/metadata?path="+url.QueryEscape(path), bytes.NewBufferString(metadataBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("metadata compatibility status=%d body=%s", response.Code, response.Body.String())
	}

	conversionBody := `{"keepAudioStreams":[2],"subtitleRoleOverrides":{}}`
	request = httptest.NewRequest(http.MethodPost, "/api/assets/conversion?path="+url.QueryEscape(path), bytes.NewBufferString(conversionBody))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("conversion compatibility status=%d body=%s", response.Code, response.Body.String())
	}

	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: {KeepAudioStreams: []int{2}}}); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/assets/conversion?path="+url.QueryEscape(path), bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("empty conversion compatibility status=%d body=%s", response.Code, response.Body.String())
	}
	if _, ok := assetConversionOverrides(db)[cleanPath]; ok {
		t.Fatal("empty compatibility conversion did not remove the override")
	}
}
