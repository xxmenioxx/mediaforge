package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func assetTracksPatchRequest(t testing.TB, router http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPatch, "/api/assets/tracks?path="+url.QueryEscape(path), bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func newAssetTracksTestServer(t testing.TB) (*gorm.DB, *gin.Engine, string) {
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

func TestAssetTracksPatchConcurrentDifferentAssetsPreservesEntries(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	pathA := filepath.Join(root, "a.mkv")
	pathB := filepath.Join(root, "b.mkv")
	writeTestFile(t, pathA, "a")
	writeTestFile(t, pathB, "b")
	otherPath := filepath.Join(root, "other.mkv")
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		filepath.Clean(pathA):     {VideoCodec: "x265", KeepAudioStreams: []int{0}},
		filepath.Clean(pathB):     {VideoCodec: "x265", KeepSubtitleStreams: []int{0}},
		filepath.Clean(otherPath): {VideoCodec: "x265", QualityValue: 18},
	}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	inputs := []struct {
		path string
		body string
	}{
		{pathA, `{"keepAudioStreams":[1]}`},
		{pathB, `{"keepSubtitleStreams":[2]}`},
	}
	responses := make([]*httptest.ResponseRecorder, len(inputs))
	var wait sync.WaitGroup
	for index, input := range inputs {
		wait.Add(1)
		go func(index int, input struct{ path, body string }) {
			defer wait.Done()
			<-start
			responses[index] = assetTracksPatchRequest(t, router, input.path, input.body)
		}(index, input)
	}
	close(start)
	wait.Wait()
	for index, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}

	entries := assetConversionOverrides(db)
	if !reflect.DeepEqual(entries[filepath.Clean(pathA)].KeepAudioStreams, []int{1}) || len(entries[filepath.Clean(pathA)].KeepSubtitleStreams) != 0 {
		t.Fatalf("asset A conversion was lost or changed: %#v", entries[filepath.Clean(pathA)])
	}
	if !reflect.DeepEqual(entries[filepath.Clean(pathB)].KeepSubtitleStreams, []int{2}) || len(entries[filepath.Clean(pathB)].KeepAudioStreams) != 0 {
		t.Fatalf("asset B conversion was lost or changed: %#v", entries[filepath.Clean(pathB)])
	}
	if entries[filepath.Clean(otherPath)].QualityValue != 18 {
		t.Fatalf("unrelated conversion entry was lost: %#v", entries[filepath.Clean(otherPath)])
	}
}

func TestAssetTracksPatchConcurrentSameAssetPreservesPartialUpdates(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "same.mkv")
	writeTestFile(t, path, "media")
	cleanPath := filepath.Clean(path)
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		cleanPath: {VideoCodec: "x265", QualityValue: 18},
	}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	inputs := []string{
		`{"keepAudioStreams":[1]}`,
		`{"keepSubtitleStreams":[2]}`,
	}
	responses := make([]*httptest.ResponseRecorder, len(inputs))
	var wait sync.WaitGroup
	for index, body := range inputs {
		wait.Add(1)
		go func(index int, body string) {
			defer wait.Done()
			<-start
			responses[index] = assetTracksPatchRequest(t, router, path, body)
		}(index, body)
	}
	close(start)
	wait.Wait()
	for index, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("request %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}

	got := assetConversionOverrides(db)[cleanPath]
	if !reflect.DeepEqual(got.KeepAudioStreams, []int{1}) || !reflect.DeepEqual(got.KeepSubtitleStreams, []int{2}) {
		t.Fatalf("same-asset partial updates were lost: %#v", got)
	}
	if got.VideoCodec != "x265" || got.QualityValue != 18 {
		t.Fatalf("same-asset patch changed unrelated fields: %#v", got)
	}
}

func TestAssetConversionOverridesBulkDecodeSkipsMalformedEntries(t *testing.T) {
	db, _, _ := newAssetEditTestServer(t)
	validA := filepath.Clean("/media/a.mkv")
	malformed := filepath.Clean("/media/b.mkv")
	validC := filepath.Clean("/media/c.mkv")
	if err := db.Create(&models.AppSetting{
		Key: "assetConversionOverrides",
		Value: models.JSONMap{"entries": map[string]any{
			validA:    map[string]any{"keepAudioStreams": []int{1}},
			malformed: map[string]any{"keepAudioStreams": "invalid"},
			validC:    map[string]any{"keepSubtitleStreams": []int{3}},
		}},
	}).Error; err != nil {
		t.Fatal(err)
	}

	entries := assetConversionOverrides(db)
	if !reflect.DeepEqual(entries[validA].KeepAudioStreams, []int{1}) || !reflect.DeepEqual(entries[validC].KeepSubtitleStreams, []int{3}) {
		t.Fatalf("valid entries were not retained: %#v", entries)
	}
	if _, ok := entries[malformed]; ok {
		t.Fatalf("malformed conversion entry was retained: %#v", entries[malformed])
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

func TestAssetTracksPatchRejectsActiveMaintenanceForSameFileAlias(t *testing.T) {
	db, router, root := newAssetTracksTestServer(t)
	path := filepath.Join(root, "movie.mkv")
	alias := filepath.Join(root, "movie-alias.mkv")
	writeTestFile(t, path, "media")
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links are unavailable in this test filesystem: %v", err)
	}
	operation := models.AssetMaintenanceOperation{
		ID: "active-tracks-patch-maintenance-alias", OperationType: "remove_tracks", AssetPath: alias,
		Status: maintenanceStatusRunning, Phase: "rewriting",
	}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}

	response := assetTracksPatchRequest(t, router, path, `{"keepAudioStreams":[1]}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "asset has an active maintenance operation and Tracks cannot be modified") {
		t.Fatalf("tracks patch status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestActiveAssetMaintenanceSkipsFilesystemChecksWhenNoOperations(t *testing.T) {
	db, _, root := newAssetTracksTestServer(t)
	active, metrics, err := activeAssetMaintenanceWithMetrics(db, filepath.Join(root, "missing.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	if active || metrics.filesystemChecks != 0 {
		t.Fatalf("no-operation maintenance check = active:%t metrics:%#v, want no filesystem checks", active, metrics)
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
