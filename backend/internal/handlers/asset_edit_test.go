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
	router.POST("/api/assets/rename", handler.Rename)
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

func TestAssetEditSerializesWithRenameOverrideMigration(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	oldPath := filepath.Join(root, "movie.mkv")
	otherPath := filepath.Join(root, "other.mkv")
	writeTestFile(t, oldPath, "media")
	writeTestFile(t, otherPath, "other")
	if err := db.Create(&models.AssetRecord{
		Path: oldPath, RootPath: root, RelativePath: "movie.mkv", FileName: "movie.mkv",
		Status: "unprocessed", LibraryID: 1, LibraryName: "Test",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveAssetMetadataOverrides(db, map[string]AssetMetadataState{
		filepath.Clean(oldPath):   {Categories: []string{"Before"}},
		filepath.Clean(otherPath): {Categories: []string{"Other"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		filepath.Clean(oldPath):   {KeepAudioStreams: []int{1}},
		filepath.Clean(otherPath): {KeepAudioStreams: []int{2}},
	}); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	callbackName := "asset-edit-rename-barrier"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "app_settings" {
			return
		}
		once.Do(func() {
			close(entered)
			<-release
		})
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Update().Remove(callbackName)

	compoundDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		compoundDone <- assetEditRequest(t, router, oldPath, AssetEditInput{
			Metadata:   AssetMetadataUpdateInput{Categories: []string{"After"}},
			Conversion: AssetConversionUpdateInput{KeepAudioStreams: []int{3}},
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("compound save did not reach the deterministic AppSetting barrier")
	}

	renameDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, "/api/assets/rename", bytes.NewBufferString(`{"path":"`+oldPath+`","fileName":"renamed.mkv"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		renameDone <- response
	}()
	close(release)

	compoundResponse := <-compoundDone
	renameResponse := <-renameDone
	if compoundResponse.Code != http.StatusOK {
		t.Fatalf("compound save status=%d body=%s", compoundResponse.Code, compoundResponse.Body.String())
	}
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename status=%d body=%s", renameResponse.Code, renameResponse.Body.String())
	}

	newPath := filepath.Join(root, "renamed.mkv")
	metadata := assetMetadataOverrides(db)
	conversion := assetConversionOverrides(db)
	if got := metadata[filepath.Clean(newPath)]; len(got.Categories) != 1 || got.Categories[0] != "After" {
		t.Fatalf("compound metadata was not migrated: %#v", metadata)
	}
	if got := conversion[filepath.Clean(newPath)]; len(got.KeepAudioStreams) != 1 || got.KeepAudioStreams[0] != 3 {
		t.Fatalf("compound conversion was not migrated: %#v", conversion)
	}
	if metadata[filepath.Clean(otherPath)].Categories[0] != "Other" || conversion[filepath.Clean(otherPath)].KeepAudioStreams[0] != 2 {
		t.Fatalf("unrelated override entry was lost: metadata=%#v conversion=%#v", metadata, conversion)
	}
	if _, exists := metadata[filepath.Clean(oldPath)]; exists {
		t.Fatalf("old metadata path remained after rename: %#v", metadata)
	}
	if _, exists := conversion[filepath.Clean(oldPath)]; exists {
		t.Fatalf("old conversion path remained after rename: %#v", conversion)
	}
}

func TestAssetEditRejectsStaleSaveAfterRenameWins(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	oldPath := filepath.Join(root, "movie.mkv")
	otherPath := filepath.Join(root, "other.mkv")
	writeTestFile(t, oldPath, "media")
	writeTestFile(t, otherPath, "other")
	if err := db.Create(&models.AssetRecord{
		Path: oldPath, RootPath: root, RelativePath: "movie.mkv", FileName: "movie.mkv",
		Status: "unprocessed", LibraryID: 1, LibraryName: "Test",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveAssetMetadataOverrides(db, map[string]AssetMetadataState{
		filepath.Clean(oldPath):   {Categories: []string{"Before"}},
		filepath.Clean(otherPath): {Categories: []string{"Other"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		filepath.Clean(oldPath):   {KeepAudioStreams: []int{1}},
		filepath.Clean(otherPath): {KeepAudioStreams: []int{2}},
	}); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	callbackName := "asset-edit-rename-wins-barrier"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "app_settings" {
			return
		}
		once.Do(func() {
			close(entered)
			<-release
		})
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Update().Remove(callbackName)

	renameDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, "/api/assets/rename", bytes.NewBufferString(`{"path":"`+oldPath+`","fileName":"renamed.mkv"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		renameDone <- response
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("rename did not reach the deterministic AppSetting barrier")
	}

	staleSaveDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		staleSaveDone <- assetEditRequest(t, router, oldPath, AssetEditInput{
			Metadata:   AssetMetadataUpdateInput{Categories: []string{"Stale"}},
			Conversion: AssetConversionUpdateInput{KeepAudioStreams: []int{3}},
		})
	}()
	close(release)

	renameResponse := <-renameDone
	staleSaveResponse := <-staleSaveDone
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename status=%d body=%s", renameResponse.Code, renameResponse.Body.String())
	}
	if staleSaveResponse.Code != http.StatusConflict {
		t.Fatalf("stale save status=%d body=%s", staleSaveResponse.Code, staleSaveResponse.Body.String())
	}

	newPath := filepath.Join(root, "renamed.mkv")
	metadata := assetMetadataOverrides(db)
	conversion := assetConversionOverrides(db)
	if got := metadata[filepath.Clean(newPath)]; len(got.Categories) != 1 || got.Categories[0] != "Before" {
		t.Fatalf("rename metadata was not preserved: %#v", metadata)
	}
	if got := conversion[filepath.Clean(newPath)]; len(got.KeepAudioStreams) != 1 || got.KeepAudioStreams[0] != 1 {
		t.Fatalf("rename conversion was not preserved: %#v", conversion)
	}
	if metadata[filepath.Clean(otherPath)].Categories[0] != "Other" || conversion[filepath.Clean(otherPath)].KeepAudioStreams[0] != 2 {
		t.Fatalf("unrelated override entry was lost: metadata=%#v conversion=%#v", metadata, conversion)
	}
	if _, exists := metadata[filepath.Clean(oldPath)]; exists {
		t.Fatalf("stale metadata override was recreated: %#v", metadata)
	}
	if _, exists := conversion[filepath.Clean(oldPath)]; exists {
		t.Fatalf("stale conversion override was recreated: %#v", conversion)
	}
}

func TestRenameMapsAuthorizationReadFailureToInternalServerError(t *testing.T) {
	db, router, root := newAssetEditTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/assets/rename", bytes.NewBufferString(`{"path":"`+path+`","fileName":"renamed.mkv"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("authorization read failure status=%d body=%s", response.Code, response.Body.String())
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
