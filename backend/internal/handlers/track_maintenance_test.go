package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type maintenanceTestFixture struct {
	handler   AssetHandler
	db        *gorm.DB
	path      string
	original  []byte
	inventory TrackMaintenanceInventory
	remaining []TrackMaintenanceStream
	operation models.AssetMaintenanceOperation
	runtime   *trackMaintenanceRuntime
}

func newMaintenanceTestFixture(t *testing.T) maintenanceTestFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:maintenance-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.AssetMaintenanceOperation{}, &models.ScanResult{}, &models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mkv")
	original := []byte("protected original")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: path, FileName: filepath.Base(path), Extension: ".mkv", Status: "converted", LibraryID: 1, SizeBytes: int64(len(original))}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	inventory := maintenanceFixture()
	inventory.Path = path
	inventory.Fingerprint = "before"
	remaining, err := validateTrackRemoval(inventory, []int{2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	operation := models.AssetMaintenanceOperation{ID: fmt.Sprintf("operation-%d", time.Now().UnixNano()), OperationType: "remove_tracks", AssetRecordID: record.ID, AssetPath: path, AssetStatus: record.Status, Status: maintenanceStatusQueued, Phase: "queued", CreatedAt: time.Now()}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}
	runtime := &trackMaintenanceRuntime{
		probeInventory: func(_ context.Context, candidate string) (TrackMaintenanceInventory, error) {
			if filepath.Clean(candidate) == filepath.Clean(path) {
				return inventory, nil
			}
			actual := TrackMaintenanceInventory{Path: candidate, Chapters: inventory.Chapters, Streams: make([]TrackMaintenanceStream, len(remaining))}
			for index, stream := range remaining {
				stream.Index = index
				actual.Streams[index] = stream
			}
			return actual, nil
		},
		remux: func(_ context.Context, args []string) error {
			return os.WriteFile(args[len(args)-1], []byte("validated remux"), 0o644)
		},
		analyze: func(candidate string) (models.ScanResult, error) {
			return models.ScanResult{Path: candidate, FileName: filepath.Base(candidate), Container: "matroska", VideoCodec: "hevc", AudioTracks: 1}, nil
		},
		stat: os.Stat, rename: os.Rename, remove: os.Remove,
		fingerprint: func(string) (string, error) { return "after", nil },
		persist:     persistTrackMaintenanceResult,
	}
	handler := AssetHandler{db: db, trackMaintenanceRuntime: runtime}
	return maintenanceTestFixture{handler: handler, db: db, path: path, original: original, inventory: inventory, remaining: remaining, operation: operation, runtime: runtime}
}

func (fixture maintenanceTestFixture) assertOriginalUnchanged(t *testing.T) {
	t.Helper()
	content, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, fixture.original) {
		t.Fatalf("original changed after failed maintenance: got %q want %q", content, fixture.original)
	}
}

func (fixture maintenanceTestFixture) loadOperation(t *testing.T) models.AssetMaintenanceOperation {
	t.Helper()
	var operation models.AssetMaintenanceOperation
	if err := fixture.db.First(&operation, "id = ?", fixture.operation.ID).Error; err != nil {
		t.Fatal(err)
	}
	return operation
}

func maintenanceFixture() TrackMaintenanceInventory {
	return TrackMaintenanceInventory{Streams: []TrackMaintenanceStream{
		{Index: 0, Type: "video", Codec: "hevc", Profile: "Main 10", Width: 1920, Height: 1080, Default: true},
		{Index: 1, Type: "audio", Codec: "truehd", Language: "jpn", Channels: 6, Layout: "5.1", Default: true},
		{Index: 2, Type: "audio", Codec: "ac3", Language: "eng", Channels: 6, Layout: "5.1"},
		{Index: 3, Type: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "spa"},
		{Index: 4, Type: "attachment", Codec: "ttf", FileName: "font.ttf"},
		{Index: 5, Type: "data", Codec: "bin_data"},
	}}
}

func TestRecoverInterruptedTrackMaintenanceRestoresBackup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:maintenance-recovery?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetMaintenanceOperation{}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mkv")
	backup := filepath.Join(dir, ".movie.backup.mkv")
	temporary := filepath.Join(dir, ".movie.tmp.mkv")
	if err := os.WriteFile(path, []byte("interrupted result"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	operation := models.AssetMaintenanceOperation{ID: "recover-me", OperationType: "remove_tracks", AssetPath: path, Status: maintenanceStatusRunning, Phase: "committing", BackupPath: backup, TemporaryPath: temporary, CreatedAt: time.Now()}
	if err := db.Create(&operation).Error; err != nil {
		t.Fatal(err)
	}

	recoverInterruptedTrackMaintenance(db)

	content, err := os.ReadFile(path)
	if err != nil || string(content) != "original" {
		t.Fatalf("original was not restored: content=%q err=%v", content, err)
	}
	if err := db.First(&operation, "id = ?", operation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if operation.Status != maintenanceStatusFailed || operation.Phase != "interrupted" {
		t.Fatalf("operation was not closed as interrupted: %#v", operation)
	}
}

func TestValidateTrackRemovalPreservesUnselectedAndDataStreams(t *testing.T) {
	remaining, err := validateTrackRemoval(maintenanceFixture(), []int{2, 4})
	if err != nil {
		t.Fatal(err)
	}
	indexes := []int{}
	for _, stream := range remaining {
		indexes = append(indexes, stream.Index)
	}
	if !reflect.DeepEqual(indexes, []int{0, 1, 3, 5}) {
		t.Fatalf("remaining indexes=%v", indexes)
	}
}

func TestValidateTrackRemovalRejectsAllPlayableVideo(t *testing.T) {
	if _, err := validateTrackRemoval(maintenanceFixture(), []int{0}); err == nil {
		t.Fatal("expected removal of all playable video to be rejected")
	}
}

func TestValidateTrackRemovalRejectsDataSelection(t *testing.T) {
	if _, err := validateTrackRemoval(maintenanceFixture(), []int{5}); err == nil {
		t.Fatal("expected data stream selection to be rejected")
	}
}

func TestBuildRemoveTracksFFmpegArgsUsesAbsoluteMapsAndCopy(t *testing.T) {
	remaining, err := validateTrackRemoval(maintenanceFixture(), []int{2, 4})
	if err != nil {
		t.Fatal(err)
	}
	args := buildRemoveTracksFFmpegArgs("/library/input.mkv", "/library/.output.tmp.mkv", remaining)
	want := []string{"-hide_banner", "-nostdin", "-y", "-i", "/library/input.mkv", "-map", "0:0", "-map", "0:1", "-map", "0:3", "-map", "0:5", "-map_metadata", "0", "-map_chapters", "0", "-c", "copy", "/library/.output.tmp.mkv"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%#v want=%#v", args, want)
	}
}

func TestValidateRemuxedInventoryComparesIdentityNotAbsoluteIndex(t *testing.T) {
	expected, err := validateTrackRemoval(maintenanceFixture(), []int{2, 4})
	if err != nil {
		t.Fatal(err)
	}
	actual := TrackMaintenanceInventory{Streams: make([]TrackMaintenanceStream, len(expected))}
	for index, stream := range expected {
		stream.Index = index
		actual.Streams[index] = stream
	}
	if err := validateRemuxedTrackInventory(expected, actual); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteTrackRemovalSuccessRefreshesSnapshotWithoutQueueJobOrStatusChange(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	if err := fixture.db.Create(&models.ScanResult{Path: fixture.path, FileName: filepath.Base(fixture.path), AudioTracks: 2, SubtitleTracks: 1}).Error; err != nil {
		t.Fatal(err)
	}

	fixture.handler.executeTrackRemoval(fixture.operation.ID, fixture.inventory, fixture.remaining)

	operation := fixture.loadOperation(t)
	if operation.Status != maintenanceStatusComplete || operation.Phase != "completed" || operation.ResultFingerprint != "after" {
		t.Fatalf("unexpected completed operation: %#v", operation)
	}
	content, err := os.ReadFile(fixture.path)
	if err != nil || string(content) != "validated remux" {
		t.Fatalf("remux was not installed: content=%q err=%v", content, err)
	}
	var snapshots []models.ScanResult
	if err := fixture.db.Where("path = ?", fixture.path).Find(&snapshots).Error; err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].AudioTracks != 1 || snapshots[0].SubtitleTracks != 0 {
		t.Fatalf("snapshot was not refreshed: %#v", snapshots)
	}
	var record models.AssetRecord
	if err := fixture.db.Where("path = ?", fixture.path).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.Status != "converted" {
		t.Fatalf("asset status changed: %q", record.Status)
	}
	var queueJobs int64
	if err := fixture.db.Model(&models.QueueJob{}).Count(&queueJobs).Error; err != nil {
		t.Fatal(err)
	}
	if queueJobs != 0 {
		t.Fatalf("maintenance created %d queue jobs", queueJobs)
	}
}

func TestExecuteTrackRemovalFFmpegFailureLeavesOriginalUntouched(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	fixture.runtime.remux = func(context.Context, []string) error { return errors.New("injected ffmpeg failure") }

	fixture.handler.executeTrackRemoval(fixture.operation.ID, fixture.inventory, fixture.remaining)

	fixture.assertOriginalUnchanged(t)
	operation := fixture.loadOperation(t)
	if operation.Status != maintenanceStatusFailed || operation.Phase != "remuxing" || !strings.Contains(operation.ErrorMessage, "injected ffmpeg failure") {
		t.Fatalf("unexpected failure result: %#v", operation)
	}
}

func TestExecuteTrackRemovalValidationFailureLeavesOriginalUntouched(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	fixture.runtime.probeInventory = func(_ context.Context, candidate string) (TrackMaintenanceInventory, error) {
		return TrackMaintenanceInventory{Path: candidate, Streams: []TrackMaintenanceStream{{Index: 0, Type: "video", Codec: "unexpected"}}}, nil
	}

	fixture.handler.executeTrackRemoval(fixture.operation.ID, fixture.inventory, fixture.remaining)

	fixture.assertOriginalUnchanged(t)
	operation := fixture.loadOperation(t)
	if operation.Status != maintenanceStatusFailed || operation.Phase != "validating" {
		t.Fatalf("unexpected validation failure: %#v", operation)
	}
}

func TestExecuteTrackRemovalSwapFailureRestoresOriginal(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	renameCalls := 0
	fixture.runtime.rename = func(source, destination string) error {
		renameCalls++
		if renameCalls == 2 {
			return errors.New("injected replacement failure")
		}
		return os.Rename(source, destination)
	}

	fixture.handler.executeTrackRemoval(fixture.operation.ID, fixture.inventory, fixture.remaining)

	fixture.assertOriginalUnchanged(t)
	operation := fixture.loadOperation(t)
	if operation.Status != maintenanceStatusFailed || operation.Phase != "committing" {
		t.Fatalf("unexpected swap failure: %#v", operation)
	}
}

func TestExecuteTrackRemovalPersistenceFailureRollsBackOriginal(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	fixture.runtime.persist = func(AssetHandler, string, string, string, models.ScanResult, time.Time) error {
		return errors.New("injected database failure")
	}

	fixture.handler.executeTrackRemoval(fixture.operation.ID, fixture.inventory, fixture.remaining)

	fixture.assertOriginalUnchanged(t)
	operation := fixture.loadOperation(t)
	if operation.Status != maintenanceStatusFailed || operation.Phase != "committing" || !strings.Contains(operation.ErrorMessage, "injected database failure") {
		t.Fatalf("unexpected persistence failure: %#v", operation)
	}
}

func TestStartTrackRemovalRequiresConfirmation(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	request := httptest.NewRequest(http.MethodPost, "/api/assets/track-maintenance/remove", strings.NewReader(fmt.Sprintf(`{"path":%q,"streamIndexes":[2],"expectedFingerprint":"before","confirmed":false}`, fixture.path)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	fixture.handler.StartTrackRemoval(context)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "confirmation") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStartTrackRemovalRejectsStaleFingerprint(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	if err := fixture.db.Delete(&models.AssetMaintenanceOperation{}, "id = ?", fixture.operation.ID).Error; err != nil {
		t.Fatal(err)
	}
	fixture.inventory.Fingerprint = "current"
	fixture.runtime.probeInventory = func(context.Context, string) (TrackMaintenanceInventory, error) { return fixture.inventory, nil }
	request := httptest.NewRequest(http.MethodPost, "/api/assets/track-maintenance/remove", strings.NewReader(fmt.Sprintf(`{"path":%q,"streamIndexes":[2],"expectedFingerprint":"stale","confirmed":true}`, fixture.path)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = request

	fixture.handler.StartTrackRemoval(ginContext)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "asset changed") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStartTrackRemovalRejectsActiveQueueJob(t *testing.T) {
	fixture := newMaintenanceTestFixture(t)
	if err := fixture.db.Delete(&models.AssetMaintenanceOperation{}, "id = ?", fixture.operation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.QueueJob{MediaPath: fixture.path, LibraryID: 1, ProfileID: 1, Status: "queued", Stage: "queued"}).Error; err != nil {
		t.Fatal(err)
	}
	probeCalled := false
	fixture.runtime.probeInventory = func(context.Context, string) (TrackMaintenanceInventory, error) {
		probeCalled = true
		return TrackMaintenanceInventory{}, nil
	}
	request := httptest.NewRequest(http.MethodPost, "/api/assets/track-maintenance/remove", strings.NewReader(fmt.Sprintf(`{"path":%q,"streamIndexes":[2],"expectedFingerprint":"before","confirmed":true}`, fixture.path)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = request

	fixture.handler.StartTrackRemoval(ginContext)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "active Queue job") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if probeCalled {
		t.Fatal("asset was probed despite active queue job")
	}
}

func TestRemoveTracksFFmpegRemuxesDisposableMKV(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	dir := t.TempDir()
	subtitle := filepath.Join(dir, "subtitle.srt")
	attachment := filepath.Join(dir, "font.txt")
	input := filepath.Join(dir, "input.mkv")
	output := filepath.Join(dir, "output.mkv")
	if err := os.WriteFile(subtitle, []byte("1\n00:00:00,000 --> 00:00:00,800\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachment, []byte("disposable attachment"), 0o644); err != nil {
		t.Fatal(err)
	}
	createArgs := []string{"-v", "error", "-f", "lavfi", "-i", "color=c=black:s=160x90:d=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-f", "lavfi", "-i", "sine=frequency=880:duration=1", "-i", subtitle, "-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s", "-c:v", "mpeg4", "-c:a", "aac", "-c:s", "srt", "-attach", attachment, "-metadata:s:t", "mimetype=text/plain", input}
	if outputBytes, err := exec.Command(ffmpeg, createArgs...).CombinedOutput(); err != nil {
		t.Fatalf("create disposable MKV: %v: %s", err, outputBytes)
	}
	inventory, err := probeTrackMaintenanceInventory(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	remove := []int{}
	audioSeen := 0
	for _, stream := range inventory.Streams {
		switch stream.Type {
		case "audio":
			audioSeen++
			if audioSeen == 2 {
				remove = append(remove, stream.Index)
			}
		case "subtitle", "attachment":
			remove = append(remove, stream.Index)
		}
	}
	remaining, err := validateTrackRemoval(inventory, remove)
	if err != nil {
		t.Fatal(err)
	}
	if outputBytes, err := exec.Command(ffmpeg, buildRemoveTracksFFmpegArgs(input, output, remaining)...).CombinedOutput(); err != nil {
		t.Fatalf("remux disposable MKV: %v: %s", err, outputBytes)
	}
	actual, err := probeTrackMaintenanceInventory(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRemuxedTrackInventory(remaining, actual); err != nil {
		t.Fatal(err)
	}
	for _, stream := range actual.Streams {
		if stream.Type == "subtitle" || stream.Type == "attachment" {
			t.Fatalf("removed stream type remained in output: %#v", stream)
		}
	}
}

func TestTrackRemovalRequestJSONUsesAbsoluteStreamIndexes(t *testing.T) {
	payload := removeTracksInput{Path: "/library/movie.mkv", StreamIndexes: []int{2, 4}, ExpectedFingerprint: "fingerprint", Confirmed: true}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"streamIndexes":[2,4]`) {
		t.Fatalf("absolute indexes were not serialized: %s", encoded)
	}
}
