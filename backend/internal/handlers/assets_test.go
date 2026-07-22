package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLogicalAssetGroupPathUsesTopLevelFolder(t *testing.T) {
	tests := map[string]string{
		"1.mkv":                             "",
		"movies/1.mkv":                      "movies",
		"movies/movie1/movie.mkv":           "movies/movie1",
		"movies/movie1/extras/extra1.mkv":   "movies/movie1",
		"series/show/season1/episode01.mkv": "series/show",
		"series/show/season2/episode01.mkv": "series/show",
	}

	for input, expected := range tests {
		if actual := logicalAssetGroupPath(input); actual != expected {
			t.Fatalf("logicalAssetGroupPath(%q) = %q, expected %q", input, actual, expected)
		}
	}
}

func TestManualReviewApprovalIsPersistedAfterDisablingBlock(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:manual-review-approval?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	assetPath := filepath.Join(root, "Baccano", "Season0", "NCED Calling.mkv")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Library{Name: "Anime", SourcePath: root, DestinationPath: root}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/review", NewAssetHandler(db).UpdateReview)
	request := httptest.NewRequest(http.MethodPost, "/api/assets/review?path="+url.QueryEscape(assetPath), strings.NewReader(`{"requiresReview":false,"source":"manual","reason":"","tags":[]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	review := reviewForPath(assetPath, assetReviewOverrides(db))
	if review.RequiresReview || review.Source != "manual" || review.UpdatedAt.IsZero() {
		t.Fatalf("manual approval was not persisted: %#v", review)
	}
}

func TestPreviewModesUseFastProxyAndExactQualityFilters(t *testing.T) {
	if got := previewVideoFilterChain("", "quick"); !strings.Contains(got, "854") {
		t.Fatalf("quick preview filter=%q", got)
	}
	if got := previewVideoFilterChain("eq=contrast=1.1", "quality"); got != "eq=contrast=1.1" {
		t.Fatalf("quality preview changed filters: %q", got)
	}
	if got := previewVideoFilterChain("", "quality"); got != "null" {
		t.Fatalf("quality preview should preserve resolution: %q", got)
	}
}

func TestPreviewCacheKeyChangesWithSourceAndOptions(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstInfo, _ := os.Stat(mediaPath)
	first := previewCacheKey(mediaPath, firstInfo, []string{"-crf", "20"})
	changedOption := previewCacheKey(mediaPath, firstInfo, []string{"-crf", "21"})
	if first == changedOption {
		t.Fatal("preview options must participate in cache identity")
	}
	if err := os.WriteFile(mediaPath, []byte("different source"), 0o600); err != nil {
		t.Fatal(err)
	}
	secondInfo, _ := os.Stat(mediaPath)
	if first == previewCacheKey(mediaPath, secondInfo, []string{"-crf", "20"}) {
		t.Fatal("source size and modification time must participate in cache identity")
	}
}

func TestDeleteConvertedRestoresArchivedOriginalAndPreservesJob(t *testing.T) {
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "success")
	relative := filepath.Join("movies", "movie.mkv")
	convertedPath := filepath.Join(libraryRoot, relative)
	archivePath := filepath.Join(archiveRoot, relative)
	restorePath := filepath.Join(rawRoot, relative)
	writeTestFile(t, convertedPath, "converted")
	writeTestFile(t, archivePath, "original")
	job := models.QueueJob{MediaPath: restorePath, LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: convertedPath, OriginalArchivedPath: archivePath}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: convertedPath, RootPath: libraryRoot, RelativePath: filepath.ToSlash(relative), FileName: "movie.mkv", Status: "converted", LibraryID: 1, LibraryName: "Movies"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/delete-converted", NewAssetHandler(db).DeleteConverted)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/delete-converted?path="+url.QueryEscape(convertedPath), strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(convertedPath); !os.IsNotExist(err) {
		t.Fatalf("converted still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(convertedPath)); !os.IsNotExist(err) {
		t.Fatalf("empty converted asset directory was not removed: %v", err)
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("archive still exists: %v", err)
	}
	content, err := os.ReadFile(restorePath)
	if err != nil || string(content) != "original" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
	var preserved models.QueueJob
	if err := db.First(&preserved, job.ID).Error; err != nil {
		t.Fatalf("job history was removed: %v", err)
	}
	if !strings.Contains(preserved.Notes, "restored to Raw") {
		t.Fatalf("missing audit note: %q", preserved.Notes)
	}
	if preserved.PublicationRetiredAt == nil {
		t.Fatal("publication was not retired from automatic reconciliation")
	}
}

func TestRecoverArchiveAssetMovesOriginalBackToRaw(t *testing.T) {
	db, rawRoot, _, archiveRoot := safeDeleteTestDB(t, "recover-archive")
	relative := filepath.Join("anime", "Baccano", "episode.mkv")
	archivePath := filepath.Join(archiveRoot, "processed-originals", relative)
	rawPath := filepath.Join(rawRoot, relative)
	writeTestFile(t, archivePath, "original")
	record := models.AssetRecord{Path: archivePath, RootPath: archiveRoot, RelativePath: filepath.ToSlash(relative), FileName: "episode.mkv", Status: "archive", LibraryName: "Original Archive"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/recover", NewAssetHandler(db).Recover)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/recover?path="+url.QueryEscape(archivePath), nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(rawPath)
	if err != nil || string(content) != "original" {
		t.Fatalf("recovered content=%q err=%v", content, err)
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("archive source still exists: %v", err)
	}
}

func TestDeleteConvertedCanResumeWhenConvertedAlreadyWentMissing(t *testing.T) {
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "resume")
	convertedPath := filepath.Join(libraryRoot, "movie.mkv")
	archivePath := filepath.Join(archiveRoot, "movie.mkv")
	restorePath := filepath.Join(rawRoot, "movie.mkv")
	writeTestFile(t, archivePath, "original")
	job := models.QueueJob{MediaPath: restorePath, LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: convertedPath, OriginalArchivedPath: archivePath}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: convertedPath, RootPath: libraryRoot, RelativePath: "movie.mkv", FileName: "movie.mkv", Status: "converted", Missing: true}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/delete-converted", NewAssetHandler(db).DeleteConverted)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/delete-converted?path="+url.QueryEscape(convertedPath), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(restorePath)
	if err != nil || string(content) != "original" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
	var count int64
	if err := db.Model(&models.AssetRecord{}).Where("path = ?", convertedPath).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("stale converted inventory count=%d err=%v", count, err)
	}
}

func TestDeleteConvertedIsBlockedWhenArchivedOriginalIsMissing(t *testing.T) {
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "missing")
	convertedPath := filepath.Join(libraryRoot, "movie.mkv")
	archivePath := filepath.Join(archiveRoot, "movie.mkv")
	writeTestFile(t, convertedPath, "converted")
	job := models.QueueJob{MediaPath: filepath.Join(rawRoot, "movie.mkv"), LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: convertedPath, OriginalArchivedPath: archivePath}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: convertedPath, RootPath: libraryRoot, RelativePath: "movie.mkv", FileName: "movie.mkv", Status: "converted"}).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/delete-converted", NewAssetHandler(db).DeleteConverted)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/delete-converted?path="+url.QueryEscape(convertedPath), nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(convertedPath)
	if err != nil || string(content) != "converted" {
		t.Fatalf("converted was changed: content=%q err=%v", content, err)
	}
}

func safeDeleteTestDB(t *testing.T, name string) (*gorm.DB, string, string, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:safe-delete-"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.QueueJob{}, &models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rawRoot, libraryRoot, archiveRoot := filepath.Join(root, "raw"), filepath.Join(root, "library"), filepath.Join(root, "archive")
	for _, path := range []string{rawRoot, libraryRoot, archiveRoot} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": rawRoot, "libraryRoot": libraryRoot, "originalsArchivePath": archiveRoot, "stagingPath": filepath.Join(root, "staging")}}).Error; err != nil {
		t.Fatal(err)
	}
	return db, rawRoot, libraryRoot, archiveRoot
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMVForgeOutputPathsRequireCompletedOrPublishedJobEvidence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-provenance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	jobs := []models.QueueJob{
		{MediaPath: "/raw/a.mkv", Status: JobStatusCompleted, OutputPath: "/library/a.mkv"},
		{MediaPath: "/raw/b.mkv", Status: JobStatusFailed, OutputPath: "/library/b.mkv"},
		{MediaPath: "/raw/c.mkv", Status: JobStatusCompleted, OutputPath: "/work/c.mkv", PublishedPath: "/library/c.mkv"},
	}
	for _, job := range jobs {
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}

	paths := mvForgeOutputPaths(db)
	if !paths[filepath.Clean("/library/a.mkv")] || !paths[filepath.Clean("/library/c.mkv")] {
		t.Fatalf("expected completed and published outputs, got %v", paths)
	}
	if paths[filepath.Clean("/library/b.mkv")] {
		t.Fatal("failed job output must remain unverified")
	}
}

func TestMergeAssetMetadataStateDeduplicatesCategoriesAndTags(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	merged := mergeAssetMetadataState(
		AssetMetadataState{
			Categories: []string{"anime", "movie"},
			Tags:       []string{"dvd-source"},
			UpdatedAt:  older,
		},
		AssetMetadataState{
			Categories: []string{"Anime", "extras"},
			Tags:       []string{"dvd-source", "mono"},
			UpdatedAt:  newer,
		},
	)

	if !merged.UpdatedAt.Equal(newer) {
		t.Fatalf("expected newest update timestamp")
	}
	assertStringList(t, merged.Categories, []string{"anime", "movie", "extras"})
	assertStringList(t, merged.Tags, []string{"dvd-source", "mono"})
}

func TestAssetKeySetUsesRelativePathToAvoidBasenameCollisions(t *testing.T) {
	keys := assetKeySet([]Asset{
		{RelativePath: "movies/Pearl Jam MSG Disc Two (2003)_t08.mkv", FileName: "Pearl Jam MSG Disc Two (2003)_t08.mkv"},
	})

	if _, ok := keys[assetKey("movies/Pearl Jam MSG Disc Two (2003)_t08.mkv")]; !ok {
		t.Fatalf("expected converted key to include relative path")
	}
	if _, ok := keys[assetKey("series/SIMPSONS_SEASON5_D1/extras/Pearl Jam MSG Disc Two (2003)_t08.mkv")]; ok {
		t.Fatalf("did not expect basename collision from a different source path")
	}
}

func TestArchivedOriginalForJobUsesRecordedPathThenLegacyNote(t *testing.T) {
	job := models.QueueJob{MediaPath: "/raw/movies/movie.mkv", OriginalArchivedPath: "/archive/exact.mkv", Notes: "Original archived: /archive/legacy.mkv"}
	if actual := archivedOriginalForJob(job, "/raw", "/archive"); actual != "/archive/exact.mkv" {
		t.Fatalf("recorded archive path=%q", actual)
	}
	job.OriginalArchivedPath = ""
	if actual := archivedOriginalForJob(job, "/raw", "/archive"); actual != "/archive/legacy.mkv" {
		t.Fatalf("legacy archive note path=%q", actual)
	}
	job.Notes = ""
	if actual := archivedOriginalForJob(job, "/raw", "/archive"); actual != filepath.Clean("/archive/movies/movie.mkv") {
		t.Fatalf("derived archive path=%q", actual)
	}
}

func assertStringList(t *testing.T, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("actual=%v expected=%v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("actual=%v expected=%v", actual, expected)
		}
	}
}
