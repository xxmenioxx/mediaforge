package handlers

import (
	"encoding/json"
	"fmt"
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

func TestSubtitleExtractionPlansPreserveASSAndConvertOtherTextTracksToSRT(t *testing.T) {
	plans, unsupported := subtitleExtractionPlans("/media/library/Movie.mkv", []FFProbeStream{
		{Index: 0, CodecType: "video", CodecName: "hevc"},
		{Index: 2, CodecType: "subtitle", CodecName: "ass", Tags: map[string]string{"language": "spa"}},
		{Index: 3, CodecType: "subtitle", CodecName: "subrip", Tags: map[string]string{"language": "eng"}},
		{Index: 4, CodecType: "subtitle", CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "jpn"}},
	})

	if len(plans) != 2 {
		t.Fatalf("expected two text subtitle plans, got %#v", plans)
	}
	if plans[0].Codec != "ass" || plans[0].OutputPath != "/media/library/Movie.spa.2.ass" {
		t.Fatalf("unexpected ASS extraction plan: %#v", plans[0])
	}
	if plans[1].Codec != "srt" || plans[1].OutputPath != "/media/library/Movie.eng.3.srt" {
		t.Fatalf("unexpected SRT extraction plan: %#v", plans[1])
	}
	if len(unsupported) != 1 || unsupported[0] != "stream 4 (hdmv_pgs_subtitle)" {
		t.Fatalf("unexpected unsupported subtitle tracks: %#v", unsupported)
	}
}

func TestSubtitleExtractionPlansUseSafeUndefinedLanguage(t *testing.T) {
	plans, _ := subtitleExtractionPlans("/media/library/Movie.mkv", []FFProbeStream{
		{Index: 7, CodecType: "subtitle", CodecName: "webvtt", Tags: map[string]string{"language": "../../ES MX"}},
		{Index: 8, CodecType: "subtitle", CodecName: "srt"},
	})

	if plans[0].OutputPath != "/media/library/Movie.es-mx.7.srt" {
		t.Fatalf("unsafe or unexpected language filename: %q", plans[0].OutputPath)
	}
	if plans[1].OutputPath != "/media/library/Movie.und.8.srt" {
		t.Fatalf("missing language should use und: %q", plans[1].OutputPath)
	}
}

func TestExtractSubtitlesCreatesSidecarsAndPreservesExistingFiles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:subtitle-extraction?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	mediaPath := filepath.Join(root, "Baccano.mkv")
	if err := os.WriteFile(mediaPath, []byte("converted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Library{Name: "Anime", DestinationPath: root}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: mediaPath, RootPath: root, RelativePath: "Baccano.mkv", FileName: "Baccano.mkv", Status: "library"}).Error; err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	ffprobe := filepath.Join(bin, "ffprobe")
	ffmpeg := filepath.Join(bin, "ffmpeg")
	probeScript := `#!/bin/sh
printf '%s' '{"format":{"filename":"Baccano.mkv"},"streams":[{"index":2,"codec_type":"subtitle","codec_name":"ass","tags":{"language":"spa"}},{"index":3,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"eng"}},{"index":4,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"jpn"}}]}'
`
	ffmpegScript := `#!/bin/sh
for argument do output="$argument"; done
if [ "$output" = "-" ]; then exit 0; fi
printf '%s\n' 'generated subtitle' > "$output"
`
	if err := os.WriteFile(ffprobe, []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffmpeg, []byte(ffmpegScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/extract-subtitles", NewAssetHandler(db).ExtractSubtitles)
	run := func() SubtitleExtractionResult {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles?path="+url.QueryEscape(mediaPath), strings.NewReader("{}"))
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var result SubtitleExtractionResult
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	first := run()
	if len(first.Created) != 2 || len(first.Existing) != 0 || len(first.Unsupported) != 1 {
		t.Fatalf("unexpected first extraction result: %#v", first)
	}
	for _, path := range []string{
		filepath.Join(root, "Baccano.spa.2.ass"),
		filepath.Join(root, "Baccano.eng.3.srt"),
	} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "generated subtitle\n" {
			t.Fatalf("sidecar %q content=%q err=%v", path, content, err)
		}
	}

	second := run()
	if len(second.Created) != 0 || len(second.Existing) != 2 {
		t.Fatalf("repeat extraction should preserve existing sidecars: %#v", second)
	}
}

func TestMigratePathMovesAllFilesAndReconcilesConvertedPublication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-path-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	documentariesRoot := filepath.Join(root, "documentaries")
	seriesRoot := filepath.Join(root, "series")
	sourcePath := filepath.Join(documentariesRoot, "TATU")
	destinationPath := filepath.Join(seriesRoot, "TATU")
	videoPath := filepath.Join(sourcePath, "_Ttitle.mkv")
	sidecarPath := filepath.Join(sourcePath, "_Ttitle.spa.srt")
	writeTestFile(t, videoPath, "converted")
	writeTestFile(t, sidecarPath, "subtitle")
	documentaries := models.Library{Name: "Documentaries", DestinationPath: documentariesRoot}
	series := models.Library{Name: "Series", DestinationPath: seriesRoot}
	if err := db.Create(&documentaries).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{
		Path: videoPath, RootPath: documentariesRoot, RelativePath: "TATU/_Ttitle.mkv",
		FileName: "_Ttitle.mkv", Status: "converted", LibraryID: documentaries.ID, LibraryName: documentaries.Name,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	job := models.QueueJob{
		MediaPath: filepath.Join(root, "raw", "documentaries", "TATU", "_Ttitle.mkv"),
		LibraryID: documentaries.ID, ProfileID: 1, Status: JobStatusCompleted,
		PublishedPath: videoPath, PublishedAt: &now, PlannedPublishedPath: videoPath,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		videoPath: {KeepSubtitleStreams: []int{2}},
	}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/migrate-path", NewAssetHandler(db).MigratePath)
	body := fmt.Sprintf(`{"sourcePath":%q,"destinationLibraryId":%d}`, sourcePath, series.ID)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/migrate-path", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source path still exists: %v", err)
	}
	for _, expected := range []string{
		filepath.Join(destinationPath, "_Ttitle.mkv"),
		filepath.Join(destinationPath, "_Ttitle.spa.srt"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("migrated file missing %q: %v", expected, err)
		}
	}
	if err := db.First(&record, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.Path != filepath.Join(destinationPath, "_Ttitle.mkv") || record.LibraryID != series.ID || record.LibraryName != series.Name {
		t.Fatalf("inventory was not reconciled: %#v", record)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.PublishedPath != record.Path || job.LibraryID != series.ID || job.PlannedPublishedPath != videoPath {
		t.Fatalf("publication was not reconciled while preserving plan: %#v", job)
	}
	overrides := assetConversionOverrides(db)
	if _, exists := overrides[videoPath]; exists {
		t.Fatal("old path override was preserved")
	}
	if _, exists := overrides[record.Path]; !exists {
		t.Fatal("override was not migrated to the new path")
	}
}

func TestReconcileMovedPublishedAssetUsesExactFingerprint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:exact-asset-reconciliation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	oldPath := filepath.Join(root, "documentaries", "TATU", "_Ttitle.mkv")
	newPath := filepath.Join(root, "series", "Renamed TATU Documentary.mkv")
	writeTestFile(t, newPath, "converted-content")
	info, _ := os.Stat(newPath)
	fingerprint, err := mediaFileFingerprint(newPath)
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{
		MediaPath: filepath.Join(root, "raw", "_Ttitle.mkv"), LibraryID: 1, ProfileID: 1,
		Status: JobStatusCompleted, PublishedPath: oldPath, PlannedPublishedPath: oldPath,
		PublishedFingerprint: fingerprint, PublishedSizeBytes: info.Size(),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		oldPath: {KeepAudioStreams: []int{1}},
	}); err != nil {
		t.Fatal(err)
	}
	candidate := models.AssetRecord{Path: newPath, FileName: filepath.Base(newPath), SizeBytes: info.Size(), LibraryID: 2, LibraryName: "Series"}

	reconciled, reviews, err := reconcileMovedPublishedAssets(db, []models.AssetRecord{candidate})
	if err != nil {
		t.Fatal(err)
	}
	if reconciled != 1 || reviews != 0 {
		t.Fatalf("reconciled=%d reviews=%d", reconciled, reviews)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.PublishedPath != newPath || job.LibraryID != 2 || job.PlannedPublishedPath != oldPath {
		t.Fatalf("job was not reconciled correctly: %#v", job)
	}
	overrides := assetConversionOverrides(db)
	if _, exists := overrides[oldPath]; exists {
		t.Fatal("old override path was preserved")
	}
	if _, exists := overrides[newPath]; !exists {
		t.Fatal("override was not migrated")
	}
}

func TestReconcileLegacyPossibleMatchesRequiresReview(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:legacy-asset-reconciliation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	oldPath := "/library/documentaries/TATU/TATU_Ttitle.mkv"
	job := models.QueueJob{MediaPath: "/raw/TATU/TATU_Ttitle.mkv", LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: oldPath, PublishedSizeBytes: 100}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	candidates := []models.AssetRecord{
		{Path: "/library/series/TATU/TATU-new.mkv", FileName: "TATU-new.mkv", SizeBytes: 100},
		{Path: "/library/videos/TATU/TATU.mkv", FileName: "TATU.mkv", SizeBytes: 100},
	}
	reconciled, reviews, err := reconcileMovedPublishedAssets(db, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled != 0 || reviews != 2 {
		t.Fatalf("legacy match must require review: reconciled=%d reviews=%d", reconciled, reviews)
	}
	for _, candidate := range candidates {
		review := reviewForPath(candidate.Path, assetReviewOverrides(db))
		if !review.RequiresReview || review.Source != "sync-reconciliation" {
			t.Fatalf("candidate was not marked for review: %#v", review)
		}
	}
}

func TestConfirmLegacyPublicationReconciliationUpdatesJob(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:confirm-legacy-reconciliation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AssetRecord{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	oldPath := "/library/documentaries/TATU/_Ttitle.mkv"
	newPath := filepath.Join(t.TempDir(), "series", "TATU", "_Ttitle.mkv")
	writeTestFile(t, newPath, "converted")
	job := models.QueueJob{MediaPath: "/raw/documentaries/TATU/_Ttitle.mkv", LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: oldPath}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: newPath, FileName: "_Ttitle.mkv", Status: "converted", LibraryID: 2, LibraryName: "Series"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveAssetReviewOverrides(db, map[string]AssetReviewState{
		newPath: {
			RequiresReview: true, Source: "sync-reconciliation",
			Tags:      []string{"relocated-publication", fmt.Sprintf("reconciliation-job-%d", job.ID)},
			UpdatedAt: time.Now(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/reconcile-publication", NewAssetHandler(db).ConfirmPublicationReconciliation)
	body := fmt.Sprintf(`{"jobId":%d,"path":%q}`, job.ID, newPath)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/reconcile-publication", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.PublishedPath != newPath || job.LibraryID != record.LibraryID {
		t.Fatalf("publication was not confirmed: %#v", job)
	}
	if reviewForPath(newPath, assetReviewOverrides(db)).RequiresReview {
		t.Fatal("reconciliation review was not cleared")
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
	unrelatedPath := filepath.Join(rawRoot, "movies", "other.mkv")
	seedRecoveredAssetOverrides(t, db, convertedPath, archivePath, restorePath, unrelatedPath)

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
	assertRecoveredAssetOverridesReset(t, db, unrelatedPath, convertedPath, archivePath, restorePath)
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
	unrelatedPath := filepath.Join(rawRoot, "anime", "Baccano", "other.mkv")
	seedRecoveredAssetOverrides(t, db, archivePath, rawPath, unrelatedPath)

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
	assertRecoveredAssetOverridesReset(t, db, unrelatedPath, archivePath, rawPath)
}

func seedRecoveredAssetOverrides(t *testing.T, db *gorm.DB, paths ...string) {
	t.Helper()
	conversion := map[string]AssetConversionOverrideState{}
	reviews := map[string]AssetReviewState{}
	for _, path := range paths {
		conversion[path] = AssetConversionOverrideState{TrackProfileKey: "test-profile", KeepSubtitleStreams: []int{2}}
		reviews[path] = AssetReviewState{RequiresReview: true, Reason: "test review", Source: "test", Tags: []string{"test"}, UpdatedAt: time.Now()}
	}
	if err := saveAssetConversionOverrides(db, conversion); err != nil {
		t.Fatal(err)
	}
	if err := saveAssetReviewOverrides(db, reviews); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveredAssetOverridesReset(t *testing.T, db *gorm.DB, unrelatedPath string, resetPaths ...string) {
	t.Helper()
	conversion := assetConversionOverrides(db)
	reviews := assetReviewOverrides(db)
	for _, path := range resetPaths {
		if _, exists := conversion[filepath.Clean(path)]; exists {
			t.Fatalf("conversion override was preserved for recovered asset %q", path)
		}
		if _, exists := reviews[filepath.Clean(path)]; exists {
			t.Fatalf("review override was preserved for recovered asset %q", path)
		}
	}
	if _, exists := conversion[filepath.Clean(unrelatedPath)]; !exists {
		t.Fatalf("unrelated conversion override was removed: %q", unrelatedPath)
	}
	if _, exists := reviews[filepath.Clean(unrelatedPath)]; !exists {
		t.Fatalf("unrelated review override was removed: %q", unrelatedPath)
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

func TestDeleteConvertedUsesNewestActiveJobAndRetiresSharedPublications(t *testing.T) {
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "shared-publication")
	convertedPath := filepath.Join(libraryRoot, "anime", "Baccano", "Season0", "episode.mkv")
	olderArchivePath := filepath.Join(archiveRoot, "wrong", "episode.mkv")
	latestArchivePath := filepath.Join(archiveRoot, "anime", "Baccano", "Season0", "episode.mkv")
	latestRestorePath := filepath.Join(rawRoot, "anime", "Baccano", "Season0", "episode.mkv")
	writeTestFile(t, convertedPath, "latest converted")
	writeTestFile(t, latestArchivePath, "latest original")
	olderPublishedAt := time.Now().Add(time.Hour)
	latestPublishedAt := time.Now()
	jobs := []models.QueueJob{
		{
			MediaPath:            filepath.Join(rawRoot, "wrong", "episode.mkv"),
			LibraryID:            1,
			ProfileID:            1,
			Status:               JobStatusCompleted,
			PublishedPath:        convertedPath,
			OriginalArchivedPath: olderArchivePath,
			PublishedAt:          &olderPublishedAt,
		},
		{
			MediaPath:            latestRestorePath,
			LibraryID:            1,
			ProfileID:            1,
			Status:               JobStatusCompleted,
			PublishedPath:        convertedPath,
			OriginalArchivedPath: latestArchivePath,
			PublishedAt:          &latestPublishedAt,
		},
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join("anime", "Baccano", "Season0", "episode.mkv")
	if err := db.Create(&models.AssetRecord{Path: convertedPath, RootPath: libraryRoot, RelativePath: filepath.ToSlash(relative), FileName: "episode.mkv", Status: "converted"}).Error; err != nil {
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
	content, err := os.ReadFile(latestRestorePath)
	if err != nil || string(content) != "latest original" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
	var preserved []models.QueueJob
	if err := db.Where("published_path = ?", convertedPath).Order("id").Find(&preserved).Error; err != nil {
		t.Fatal(err)
	}
	if len(preserved) != 2 || preserved[0].PublicationRetiredAt == nil || preserved[1].PublicationRetiredAt == nil {
		t.Fatalf("shared publications were not retired: %#v", preserved)
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

func TestClassifyMissingRecordsSeparatesMovedAndActionableFiles(t *testing.T) {
	records := []models.AssetRecord{
		{Path: "/raw/moved.mkv", FileName: "moved.mkv", Status: "unprocessed", SizeBytes: 100, Missing: true},
		{Path: "/archive/moved.mkv", FileName: "moved.mkv", Status: "archive", SizeBytes: 100},
		{Path: "/raw/lost.mkv", FileName: "lost.mkv", Status: "unprocessed", SizeBytes: 200, Missing: true},
		{Path: "/library/movie-new.mkv", FileName: "movie-new.mkv", Status: "converted", SizeBytes: 300, Missing: true},
		{Path: "/library/movie.mkv", FileName: "movie.mkv", Status: "converted", SizeBytes: 250},
	}

	classification := classifyMissingRecords(records)
	if classification.Total != 3 || classification.Historical != 2 || classification.Actionable != 1 {
		t.Fatalf("unexpected missing classification: %#v", classification)
	}
}

func TestMissingMediaIdentityNormalizesRenamedSpanishTitle(t *testing.T) {
	legacy := missingMediaIdentity("El increíble castillo vagabundo\uf00a (2004)_Ttitle.mkv")
	current := missingMediaIdentity("El increible Castillo Vagabundo (2004)-new.mkv")
	if legacy != current {
		t.Fatalf("expected matching identities, got %q and %q", legacy, current)
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
