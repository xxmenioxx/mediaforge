package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func assetTracksPatchRequest(t *testing.T, router http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPatch, "/api/assets/tracks?path="+url.QueryEscape(path), bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func newAssetTracksTestServer(t *testing.T) (*gorm.DB, *gin.Engine, string) {
	t.Helper()
	db, router, root := newAssetEditTestServer(t)
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AssetMaintenanceOperation{}); err != nil {
		t.Fatal(err)
	}
	return db, router, root
}

func TestAssetTracksPatchChangesOnlySuppliedTracksFields(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{
		KeepVideoStreams: []int{0}, KeepAudioStreams: []int{1, 2}, KeepSubtitleStreams: []int{3},
		VideoMetadata:                  map[int]StreamMetadataOverride{0: {Title: "Picture"}},
		SubtitleRoleOverrides:          map[int]SubtitleRoleOverride{3: {Role: "forced"}},
		SubtitleSidecarFormatsByStream: map[int][]string{3: {"srt"}},
		VideoCodec:                     "x265", QualityValue: 18,
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[1]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
	got := assetConversionOverrides(db)[cleanPath]
	if !reflect.DeepEqual(got.KeepAudioStreams, []int{1}) {
		t.Fatalf("keepAudioStreams=%v, want [1]", got.KeepAudioStreams)
	}
	if !reflect.DeepEqual(got.KeepVideoStreams, original.KeepVideoStreams) || !reflect.DeepEqual(got.KeepSubtitleStreams, original.KeepSubtitleStreams) {
		t.Fatalf("unpatched stream selections changed: video=%v subtitle=%v", got.KeepVideoStreams, got.KeepSubtitleStreams)
	}
	if !reflect.DeepEqual(got.VideoMetadata, original.VideoMetadata) || !reflect.DeepEqual(got.SubtitleRoleOverrides, original.SubtitleRoleOverrides) || !reflect.DeepEqual(got.SubtitleSidecarFormatsByStream, original.SubtitleSidecarFormatsByStream) {
		t.Fatalf("unpatched Tracks metadata changed: %#v", got)
	}
	if got.VideoCodec != "x265" || got.QualityValue != 18 {
		t.Fatalf("unrelated conversion values changed: codec=%q quality=%d", got.VideoCodec, got.QualityValue)
	}
}

func TestAssetTracksPatchPersistsExplicitEmptySelection(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: {
		KeepAudioStreams: []int{1}, VideoCodec: "x265", QualityValue: 18,
	}}); err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("empty tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Conversion map[string]json.RawMessage `json:"conversion"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := string(payload.Conversion["keepAudioStreams"]); got != "[]" {
		t.Fatalf("response keepAudioStreams=%s, want []", got)
	}
	got, ok := assetConversionOverrides(db)[cleanPath]
	if !ok || got.KeepAudioStreams == nil || len(got.KeepAudioStreams) != 0 {
		t.Fatalf("explicit empty selection was not retained: present=%t value=%#v", ok, got.KeepAudioStreams)
	}
	if got.VideoCodec != "x265" || got.QualityValue != 18 {
		t.Fatalf("unrelated conversion values changed: codec=%q quality=%d", got.VideoCodec, got.QualityValue)
	}
}

func TestAssetTracksPatchPersistsVideoAndSubtitleSelectionsWithoutConversionChanges(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{
		KeepVideoStreams: []int{0, 1}, KeepAudioStreams: []int{2}, KeepSubtitleStreams: []int{4, 5},
		VideoCodec: "x265", QualityValue: 18,
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepVideoStreams":[1],"keepSubtitleStreams":[]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("tracks selection patch status=%d body=%s", response.Code, response.Body.String())
	}
	got := assetConversionOverrides(db)[cleanPath]
	if !reflect.DeepEqual(got.KeepVideoStreams, []int{1}) || got.KeepSubtitleStreams == nil || len(got.KeepSubtitleStreams) != 0 {
		t.Fatalf("video/subtitle selections were not preserved exactly: video=%v subtitle=%#v", got.KeepVideoStreams, got.KeepSubtitleStreams)
	}
	if !reflect.DeepEqual(got.KeepAudioStreams, original.KeepAudioStreams) || got.VideoCodec != "x265" || got.QualityValue != 18 {
		t.Fatalf("unpatched audio or conversion fields changed: %#v", got)
	}
}

func TestAssetTracksPatchCanClearOnlyChangedRoleAndSidecarFields(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{
		KeepAudioStreams: []int{1}, VideoCodec: "x265", QualityValue: 18,
		SubtitleRoleOverrides:          map[int]SubtitleRoleOverride{3: {Role: "forced"}},
		SubtitleSidecarFormatsByStream: map[int][]string{3: {"srt"}},
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"subtitleRoleOverrides":{},"subtitleSidecarFormatsByStream":{},"videoMetadata":{"0":{"title":"Main","language":"eng"}}}`)
	if response.Code != http.StatusOK {
		t.Fatalf("tracks metadata patch status=%d body=%s", response.Code, response.Body.String())
	}
	got := assetConversionOverrides(db)[cleanPath]
	if len(got.SubtitleRoleOverrides) != 0 || len(got.SubtitleSidecarFormatsByStream) != 0 {
		t.Fatalf("explicit clears were not applied: roles=%v sidecars=%v", got.SubtitleRoleOverrides, got.SubtitleSidecarFormatsByStream)
	}
	if !reflect.DeepEqual(got.VideoMetadata, map[int]StreamMetadataOverride{0: {Title: "Main", Language: "eng"}}) {
		t.Fatalf("video metadata=%v", got.VideoMetadata)
	}
	if !reflect.DeepEqual(got.KeepAudioStreams, original.KeepAudioStreams) || got.VideoCodec != "x265" || got.QualityValue != 18 {
		t.Fatalf("unpatched selection or conversion values changed: %#v", got)
	}
}

func TestAssetTracksPatchRejectsUnrelatedConversionFieldsWithoutMutation(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{KeepAudioStreams: []int{1, 2}, VideoCodec: "x265", QualityValue: 18}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[1],"videoCodec":"h264","qualityValue":20}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unrelated field patch status=%d body=%s", response.Code, response.Body.String())
	}
	if got := assetConversionOverrides(db)[cleanPath]; !reflect.DeepEqual(got.KeepAudioStreams, original.KeepAudioStreams) || got.VideoCodec != original.VideoCodec || got.QualityValue != original.QualityValue {
		t.Fatalf("rejected patch mutated conversion state: %#v", got)
	}
}

func TestAssetTracksPatchRejectsActiveQueueFromDirectRequestDespiteStaleFrontendCapability(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{KeepAudioStreams: []int{2}, VideoCodec: "x265", QualityValue: 18}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{
		MediaPath: path, Status: JobStatusQueued, Stage: JobStageQueued, TrackProfileKey: "spanish",
		ProfileSnapshot:      models.JSONMap{"videoCodec": "x265"},
		TrackProfileSnapshot: models.JSONMap{"resolvedTrackPlan": models.JSONMap{"keepAudioStreams": models.JSONList{1, 2}}},
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	var before models.QueueJob
	if err := db.First(&before, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	// This calls the API directly, without consulting the cached manageTracks
	// capability that may have been allowed when the user opened Asset Info.
	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[1]}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "asset has an active Queue job and Tracks cannot be modified") {
		t.Fatalf("tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
	if got := assetConversionOverrides(db)[cleanPath]; !reflect.DeepEqual(got, original) {
		t.Fatalf("active Queue job mutated conversion overrides: before=%#v after=%#v", original, got)
	}
	var after models.QueueJob
	if err := db.First(&after, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.ProfileSnapshot, before.ProfileSnapshot) || !reflect.DeepEqual(after.TrackProfileSnapshot, before.TrackProfileSnapshot) || after.TrackProfileKey != before.TrackProfileKey {
		t.Fatalf("Queue snapshot changed after Tracks patch: before=%#v after=%#v", before, after)
	}
}

func TestAssetTracksPatchRejectsActiveMaintenanceWithoutMutation(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{
		KeepAudioStreams: []int{2}, SubtitleMetadata: map[int]StreamMetadataOverride{3: {Title: "Commentary"}},
		VideoCodec: "x265", QualityValue: 18,
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}
	operation := models.AssetMaintenanceOperation{
		ID: "active-tracks-patch-maintenance", OperationType: "remove_tracks", AssetPath: path,
		Status: maintenanceStatusRunning, Phase: "rewriting",
	}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"subtitleMetadata":{"3":{"title":"New title"}}}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "asset has an active maintenance operation and Tracks cannot be modified") {
		t.Fatalf("tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
	if got := assetConversionOverrides(db)[cleanPath]; !reflect.DeepEqual(got, original) {
		t.Fatalf("active maintenance mutated conversion overrides: before=%#v after=%#v", original, got)
	}
}

func TestAssetTracksPatchAllowsHistoricalQueueJob(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{KeepAudioStreams: []int{2}, VideoCodec: "x265", QualityValue: 18}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now()
	job := models.QueueJob{
		MediaPath: path, Status: JobStatusCompleted, Stage: JobStageReadyToPublish, PublishedAt: &publishedAt,
		TrackProfileSnapshot: models.JSONMap{"keepAudioStreams": models.JSONList{2}},
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[1]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("historical Queue job blocked Tracks patch: status=%d body=%s", response.Code, response.Body.String())
	}
	got := assetConversionOverrides(db)[cleanPath]
	if !reflect.DeepEqual(got.KeepAudioStreams, []int{1}) || got.VideoCodec != original.VideoCodec || got.QualityValue != original.QualityValue {
		t.Fatalf("historical Queue save produced unexpected conversion state: %#v", got)
	}
}

func TestAssetTracksPatchMalformedValuesReturnBadRequest(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	original := AssetConversionOverrideState{KeepAudioStreams: []int{2}, VideoCodec: "x265", QualityValue: 18}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{cleanPath: original}); err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":"invalid"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed Tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
	if got := assetConversionOverrides(db)[cleanPath]; !reflect.DeepEqual(got, original) {
		t.Fatalf("malformed Tracks patch mutated conversion overrides: before=%#v after=%#v", original, got)
	}
}

func TestAssetTracksPatchLeavesHistoricalQueueSnapshotsUnchanged(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	writeTestFile(t, path, "media")
	job := models.QueueJob{
		MediaPath: path, Status: JobStatusFailed, Stage: JobStageFailed, TrackProfileKey: "spanish",
		ProfileSnapshot:      models.JSONMap{"videoCodec": "x265"},
		TrackProfileSnapshot: models.JSONMap{"resolvedTrackPlan": models.JSONMap{"keepAudioStreams": models.JSONList{1, 2}}},
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	var before models.QueueJob
	if err := db.First(&before, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[1]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
	var after models.QueueJob
	if err := db.First(&after, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.ProfileSnapshot, before.ProfileSnapshot) || !reflect.DeepEqual(after.TrackProfileSnapshot, before.TrackProfileSnapshot) || after.TrackProfileKey != before.TrackProfileKey {
		t.Fatalf("Queue snapshot changed after Tracks patch: before=%#v after=%#v", before, after)
	}
}
