package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

func TestUpsertAssetRecordUpdatesExistingPathAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-upsert?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/anime/Digimon/episode01.mp4"
	first := models.AssetRecord{
		Path: path, RootPath: "/media/raw", RelativePath: "anime/Digimon/episode01.mp4",
		FileName: "episode01.mp4", Status: "unprocessed", SizeBytes: 10, SyncedAt: time.Now(),
	}
	second := first
	second.RootPath = "/media/library/anime"
	second.RelativePath = "Digimon/episode01.mp4"
	second.Status = "converted"
	second.SizeBytes = 20
	if err := upsertAssetRecord(db, first); err != nil {
		t.Fatal(err)
	}
	if err := upsertAssetRecord(db, second); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.AssetRecord{}).Where("path = ?", path).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("records with same path = %d, want 1", count)
	}
	var stored models.AssetRecord
	if err := db.First(&stored, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "converted" || stored.SizeBytes != 20 || stored.RootPath != second.RootPath {
		t.Fatalf("record was not updated: %#v", stored)
	}
}

func TestAssetInventoryIncludesLatestTechnicalSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-technical-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/movies/Movie.mkv"
	if err := db.Create(&models.AssetRecord{
		Path: path, RootPath: "/media/raw", RelativePath: "movies/Movie.mkv",
		FileName: "Movie.mkv", Status: "unprocessed", SizeBytes: 1234, SyncedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ScanResult{
		Path: path, FileName: "Movie.mkv", VideoCodec: "hevc", Width: 720, Height: 480,
		Duration: 7153, Bitrate: 2_740_000, HDR: false,
	}).Error; err != nil {
		t.Fatal(err)
	}

	inventory, err := NewAssetHandler(db).assetInventoryFromDB()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Unprocessed) != 1 || inventory.Unprocessed[0].Technical == nil {
		t.Fatalf("technical snapshot missing from inventory: %#v", inventory.Unprocessed)
	}
	technical := inventory.Unprocessed[0].Technical
	if technical.VideoCodec != "hevc" || technical.Width != 720 || technical.Height != 480 ||
		technical.Duration != 7153 || technical.Bitrate != 2_740_000 || technical.HDR {
		t.Fatalf("unexpected technical snapshot: %#v", technical)
	}
}

func TestPublishAsIsMovesPathAndRegistersOriginalPublication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:publish-as-is?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Library{}, &models.AssetRecord{}, &models.DirectPublication{},
		&models.QueueJob{}, &models.ScanResult{}, &models.AppSetting{},
	); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	archiveRoot := filepath.Join(root, "archive")
	libraryRoot := filepath.Join(root, "library")
	sourcePath := filepath.Join(rawRoot, "series", "Digimon")
	videoPath := filepath.Join(sourcePath, "episode01.mp4")
	writeTestFile(t, videoPath, "small-h264-video")
	writeTestFile(t, filepath.Join(sourcePath, "episode01.spa.srt"), "subtitle")
	if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Series", SourcePath: filepath.Join(rawRoot, "series"), DestinationPath: libraryRoot, Type: "series"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": rawRoot, "originalsArchivePath": archiveRoot}}).Error; err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(videoPath)
	record := models.AssetRecord{
		Path: videoPath, RootPath: rawRoot, RelativePath: "series/Digimon/episode01.mp4",
		GroupPath: "series/Digimon", FileName: "episode01.mp4", Extension: ".mp4",
		SizeBytes: info.Size(), ModifiedAt: info.ModTime(), Status: "unprocessed", Missing: false, SyncedAt: time.Now(),
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAssetHandler(db)
	router.POST("/api/assets/publish-as-is", handler.PublishAsIs)
	router.POST("/api/assets/return-published-as-is", handler.ReturnPublishedAsIs)
	body := fmt.Sprintf(`{"sourcePath":%q,"destinationLibraryId":%d}`, sourcePath, library.ID)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/publish-as-is", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	destinationPath := filepath.Join(libraryRoot, "Digimon")
	publishedVideo := filepath.Join(destinationPath, "episode01.mp4")
	for _, expected := range []string{publishedVideo, filepath.Join(destinationPath, "episode01.spa.srt")} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("published file missing %q: %v", expected, err)
		}
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source path still exists: %v", err)
	}
	var publication models.DirectPublication
	if err := db.First(&publication, "published_path = ?", publishedVideo).Error; err != nil {
		t.Fatal(err)
	}
	if publication.SourcePath != videoPath || publication.PublishedFingerprint == "" {
		t.Fatalf("incomplete publication record: %#v", publication)
	}
	inventory, err := NewAssetHandler(db).assetInventoryFromDB()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Library) != 1 || inventory.Library[0].Status != "published_as_is" || inventory.Library[0].PublicationMode != "as_is" {
		t.Fatalf("unexpected Library inventory: %#v", inventory.Library)
	}
	if len(inventory.Converted) != 0 || len(inventory.Unprocessed) != 0 {
		t.Fatalf("asset was classified incorrectly: converted=%#v unprocessed=%#v", inventory.Converted, inventory.Unprocessed)
	}

	returnResponse := httptest.NewRecorder()
	returnRequest := httptest.NewRequest(http.MethodPost, "/api/assets/return-published-as-is?path="+url.QueryEscape(publishedVideo), strings.NewReader(`{}`))
	returnRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(returnResponse, returnRequest)
	if returnResponse.Code != http.StatusOK {
		t.Fatalf("return status=%d body=%s", returnResponse.Code, returnResponse.Body.String())
	}
	for _, expected := range []string{videoPath, filepath.Join(sourcePath, "episode01.spa.srt")} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("returned file missing %q: %v", expected, err)
		}
	}
	publicationID := publication.ID
	publication = models.DirectPublication{}
	if err := db.First(&publication, publicationID).Error; err != nil {
		t.Fatal(err)
	}
	if publication.ReturnedAt == nil {
		t.Fatal("direct publication was not retired")
	}
	inventory, err = handler.assetInventoryFromDB()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Library) != 0 || len(inventory.Unprocessed) != 1 {
		t.Fatalf("returned asset was classified incorrectly: library=%#v unprocessed=%#v", inventory.Library, inventory.Unprocessed)
	}

	republishResponse := httptest.NewRecorder()
	republishRequest := httptest.NewRequest(http.MethodPost, "/api/assets/publish-as-is", strings.NewReader(body))
	republishRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(republishResponse, republishRequest)
	if republishResponse.Code != http.StatusOK {
		t.Fatalf("republish status=%d body=%s", republishResponse.Code, republishResponse.Body.String())
	}
	var publications int64
	if err := db.Model(&models.DirectPublication{}).Where("published_path = ?", publishedVideo).Count(&publications).Error; err != nil {
		t.Fatal(err)
	}
	if publications != 1 {
		t.Fatalf("direct publication records=%d, want 1", publications)
	}
	publication = models.DirectPublication{}
	if err := db.First(&publication, publicationID).Error; err != nil {
		t.Fatal(err)
	}
	if publication.ReturnedAt != nil {
		t.Fatal("republished direct publication remained retired")
	}
}

func TestDirectPublicationRelativeGroupAvoidsDuplicateLibraryCategory(t *testing.T) {
	if got := directPublicationRelativeGroup("anime/Digimon 2", "/media/library/anime"); got != "Digimon 2" {
		t.Fatalf("relative group = %q, want Digimon 2", got)
	}
	if got := directPublicationRelativeGroup("anime/Digimon 2", "/media/library/cartoon"); got != "Digimon 2" {
		t.Fatalf("cross-category relative group = %q, want Digimon 2", got)
	}
	if got := directPublicationRelativeGroup("anime/anime/Digimon 2", "/media/library/anime"); got != "Digimon 2" {
		t.Fatalf("repeated category relative group = %q, want Digimon 2", got)
	}
	if got := directPublicationRelativeGroup("collection/Old Boy", "/media/library/anime"); got != filepath.Join("collection", "Old Boy") {
		t.Fatalf("non-category folder was removed: %q", got)
	}
}

func TestDirectPublicationEpisodeNamingRenamesMediaAndSubtitleSidecars(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "raw", "anime", "Digimon 2")
	destinationPath := filepath.Join(root, "library", "anime", "Digimon 2")
	records := []models.AssetRecord{
		{Path: filepath.Join(sourcePath, "Digimon_Adventure_02_E01-[Group].mp4")},
		{Path: filepath.Join(sourcePath, "Digimon_Adventure_02_E02-[Group].mp4")},
	}
	for _, record := range records {
		writeTestFile(t, filepath.Join(destinationPath, filepath.Base(record.Path)), "video")
	}
	writeTestFile(t, filepath.Join(destinationPath, "Digimon_Adventure_02_E01-[Group].spa.srt"), "subtitle")
	library := models.Library{ValidationRules: models.JSONMap{"episodeNamingEnabled": true}}

	published, rollback, err := applyDirectPublicationEpisodeNames(sourcePath, destinationPath, records, library)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rollback)
	expectedFirst := filepath.Join(destinationPath, "Digimon 2 - S01E01.mp4")
	expectedSecond := filepath.Join(destinationPath, "Digimon 2 - S01E02.mp4")
	if published[records[0].Path] != expectedFirst || published[records[1].Path] != expectedSecond {
		t.Fatalf("unexpected publication names: %#v", published)
	}
	for _, expected := range []string{
		expectedFirst,
		expectedSecond,
		filepath.Join(destinationPath, "Digimon 2 - S01E01.spa.srt"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Fatalf("renamed output missing %q: %v", expected, err)
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

func TestSplitPreviewFiltersPlacesSubtitlesBeforeCrop(t *testing.T) {
	before, after := splitPreviewFiltersAtCrop("fieldmatch=order=tff,decimate,crop=720:286:0:98,hqdn3d=1.5:1.5:6:6")
	if before != "fieldmatch=order=tff,decimate" {
		t.Fatalf("unexpected filters before crop: %q", before)
	}
	if after != "crop=720:286:0:98,hqdn3d=1.5:1.5:6:6" {
		t.Fatalf("unexpected crop filters: %q", after)
	}
}

func TestEscapeSubtitleFilterPath(t *testing.T) {
	got := escapeSubtitleFilterPath(`/media/Movies/Director's Cut: Film.mkv`)
	want := `/media/Movies/Director\'s Cut\: Film.mkv`
	if got != want {
		t.Fatalf("escaped path=%q want=%q", got, want)
	}
}

func TestPreviewTimestampSeconds(t *testing.T) {
	if got := previewTimestampSeconds("01:02:03"); got != 3723 {
		t.Fatalf("timestamp seconds=%d", got)
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

func TestSubtitleExtractionPlansCanSelectTrackAndOutputFormat(t *testing.T) {
	streamIndex := 3
	plans, unsupported := subtitleExtractionPlansForRequest("/media/library/Movie.mkv", []FFProbeStream{
		{Index: 2, CodecType: "subtitle", CodecName: "ass", Tags: map[string]string{"language": "spa"}},
		{Index: 3, CodecType: "subtitle", CodecName: "subrip", Tags: map[string]string{"language": "eng"}},
	}, SubtitleExtractionInput{StreamIndex: &streamIndex, Format: "ass"})

	if len(unsupported) != 0 || len(plans) != 1 {
		t.Fatalf("unexpected plans=%#v unsupported=%#v", plans, unsupported)
	}
	if plans[0].StreamIndex != 3 || plans[0].Codec != "ass" || plans[0].OutputPath != "/media/library/Movie.eng.3.ass" {
		t.Fatalf("unexpected selected plan: %#v", plans[0])
	}
}

func TestExternalSubtitleManagementIsScopedToAssetSidecars(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:external-subtitles?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mkv")
	sidecar := filepath.Join(root, "Movie.spa.default.srt")
	assSidecar := filepath.Join(root, "Movie.eng.ass")
	other := filepath.Join(root, "Other.srt")
	writeTestFile(t, mediaPath, "video")
	writeTestFile(t, sidecar, "1\n00:00:01,000 --> 00:00:02,000\nHola\n")
	writeTestFile(t, assSidecar, "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hello\n")
	writeTestFile(t, other, "must remain")
	if err := db.Create(&models.Library{Name: "Movies", DestinationPath: root}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: mediaPath, RootPath: root, RelativePath: "Movie.mkv", FileName: "Movie.mkv", Status: "converted"}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAssetHandler(db)
	router.GET("/api/assets/external-subtitles", handler.ExternalSubtitles)
	router.GET("/api/assets/external-subtitles/content", handler.ExternalSubtitleContent)
	router.PUT("/api/assets/external-subtitles", handler.UpdateExternalSubtitle)
	router.DELETE("/api/assets/external-subtitles", handler.DeleteExternalSubtitle)

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/assets/external-subtitles?path="+url.QueryEscape(mediaPath), nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listed []ExternalSubtitle
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].FileName != "Movie.eng.ass" || listed[1].Language != "spa" || !listed[1].Default {
		t.Fatalf("unexpected sidecars: %#v", listed)
	}

	updateBody := `{"subtitlePath":` + strconv.Quote(sidecar) + `,"content":"updated subtitle"}`
	updateResponse := httptest.NewRecorder()
	router.ServeHTTP(updateResponse, httptest.NewRequest(http.MethodPut, "/api/assets/external-subtitles?path="+url.QueryEscape(mediaPath), strings.NewReader(updateBody)))
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	if content, err := os.ReadFile(sidecar); err != nil || string(content) != "updated subtitle" {
		t.Fatalf("sidecar was not updated: %q %v", content, err)
	}

	foreignBody := `{"subtitlePath":` + strconv.Quote(other) + `}`
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/api/assets/external-subtitles?path="+url.QueryEscape(mediaPath), strings.NewReader(foreignBody)))
	if deleteResponse.Code != http.StatusBadRequest {
		t.Fatalf("foreign sidecar delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("foreign subtitle was modified: %v", err)
	}

	deleteBody := `{"subtitlePath":` + strconv.Quote(assSidecar) + `}`
	deleteResponse = httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/api/assets/external-subtitles?path="+url.QueryEscape(mediaPath), strings.NewReader(deleteBody)))
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if _, err := os.Stat(assSidecar); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar deletion, got %v", err)
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
	seconv := filepath.Join(bin, "seconv")
	tesseract := filepath.Join(bin, "tesseract")
	probeScript := `#!/bin/sh
printf '%s' '{"format":{"filename":"Baccano.mkv"},"streams":[{"index":2,"codec_type":"subtitle","codec_name":"ass","tags":{"language":"spa"}},{"index":3,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"eng"}},{"index":4,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"jpn"}}]}'
`
	ffmpegScript := `#!/bin/sh
for argument do output="$argument"; done
if [ "$output" = "-" ]; then exit 0; fi
printf '%s\n' 'generated subtitle' > "$output"
`
	seconvScript := `#!/bin/sh
for argument do case "$argument" in --output-filename:*) output_filename="${argument#--output-filename:}" ;; esac; done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'OCR subtitle' > "$output_filename"
`
	if err := os.WriteFile(ffprobe, []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffmpeg, []byte(ffmpegScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seconv, []byte(seconvScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tesseract, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
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
	if len(first.Created) != 3 || len(first.Existing) != 0 || len(first.Unsupported) != 0 {
		t.Fatalf("unexpected first extraction result: %#v", first)
	}
	for _, path := range []string{
		filepath.Join(root, "Baccano.spa.2.ass"),
		filepath.Join(root, "Baccano.eng.3.srt"),
		filepath.Join(root, "Baccano.jpn.4.srt"),
	} {
		content, err := os.ReadFile(path)
		if err != nil || len(content) == 0 {
			t.Fatalf("sidecar %q content=%q err=%v", path, content, err)
		}
	}

	second := run()
	if len(second.Created) != 0 || len(second.Existing) != 3 {
		t.Fatalf("repeat extraction should preserve existing sidecars: %#v", second)
	}
}

func TestExtractSubtitlesRunsOCRForExplicitBitmapTrackOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:subtitle-ocr?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	mediaPath := filepath.Join(root, "DVD.mkv")
	if err := os.WriteFile(mediaPath, []byte("dvd"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Library{Name: "Anime", DestinationPath: root}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: mediaPath, RootPath: root, RelativePath: "DVD.mkv", FileName: "DVD.mkv", Status: "library"}).Error; err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	probeScript := `#!/bin/sh
printf '%s' '{"format":{"filename":"DVD.mkv"},"streams":[{"index":4,"id":"0x7","codec_type":"subtitle","codec_name":"dvd_subtitle","tags":{"language":"spa"}}]}'
`
	seconvScript := `#!/bin/sh
output_filename=""
track=""
language=""
for argument do
  case "$argument" in
    --output-filename:*) output_filename="${argument#--output-filename:}" ;;
    --track-number:*) track="${argument#--track-number:}" ;;
    --ocr-language:*) language="${argument#--ocr-language:}" ;;
  esac
done
[ "$track" = "7" ] || exit 20
[ "$language" = "spa" ] || exit 21
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola' > "$output_filename"
`
	for name, content := range map[string]string{
		"ffprobe":   probeScript,
		"seconv":    seconvScript,
		"tesseract": "#!/bin/sh\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/extract-subtitles", NewAssetHandler(db).ExtractSubtitles)
	response := httptest.NewRecorder()
	body := `{"streamIndex":4,"format":"srt","ocrLanguage":"spa"}`
	request := httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles?path="+url.QueryEscape(mediaPath), strings.NewReader(body))
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	outputPath := filepath.Join(root, "DVD.spa.4.srt")
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "Hola") {
		t.Fatalf("unexpected OCR output: %q", content)
	}
}

func TestBitmapSubtitleOCRHelpers(t *testing.T) {
	if got := matroskaTrackNumber(FFProbeStream{Index: 4, ID: "0x7"}); got != 7 {
		t.Fatalf("hex track ID = %d, want 7", got)
	}
	if got := matroskaTrackNumber(FFProbeStream{Index: 4, ID: float64(8)}); got != 8 {
		t.Fatalf("numeric track ID = %d, want 8", got)
	}
	if got := matroskaTrackNumber(FFProbeStream{Index: 4}); got != 5 {
		t.Fatalf("fallback track ID = %d, want 5", got)
	}
	for input, want := range map[string]string{"spa": "spa", "es": "spa", "jpn": "jpn", "ja": "jpn", "eng": "eng", "und": "eng"} {
		if got := normalizedOCRLanguage(input, ""); got != want {
			t.Fatalf("OCR language %q = %q, want %q", input, got, want)
		}
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
	convertedSidecar := strings.TrimSuffix(convertedPath, filepath.Ext(convertedPath)) + ".spa.srt"
	writeTestFile(t, convertedSidecar, "keep this subtitle")
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
	if content, err := os.ReadFile(convertedSidecar); err != nil || string(content) != "keep this subtitle" {
		t.Fatalf("converted sidecar was removed: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Dir(convertedPath)); err != nil {
		t.Fatalf("library path containing sidecars was removed: %v", err)
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
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "recover-archive")
	relative := filepath.Join("anime", "Baccano", "episode.mkv")
	archivePath := filepath.Join(archiveRoot, "processed-originals", relative)
	rawPath := filepath.Join(rawRoot, relative)
	writeTestFile(t, archivePath, "original")
	librarySidecar := filepath.Join(libraryRoot, "anime", "Baccano", "episode.spa.srt")
	writeTestFile(t, librarySidecar, "keep this subtitle")
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
	if content, err := os.ReadFile(librarySidecar); err != nil || string(content) != "keep this subtitle" {
		t.Fatalf("Library sidecar was removed during Archive recovery: content=%q err=%v", content, err)
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

func TestReadableMediaRootsIncludeArchiveWithoutExpandingLibraryMutations(t *testing.T) {
	db, _, _, archiveRoot := safeDeleteTestDB(t, "archive-preview")
	handler := NewAssetHandler(db)
	archivePath := filepath.Join(archiveRoot, "documentaries", "movie.mkv")

	allowed, err := handler.pathBelongsToReadableMediaRoot(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatalf("expected archived media to be readable: %s", archivePath)
	}

	allowed, err = handler.pathBelongsToLibrary(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatalf("archive must not be accepted by mutation-only library validation: %s", archivePath)
	}

	outsidePath := filepath.Join(filepath.Dir(archiveRoot), "private", "movie.mkv")
	allowed, err = handler.pathBelongsToReadableMediaRoot(outsidePath)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatalf("path outside configured media roots was accepted: %s", outsidePath)
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
