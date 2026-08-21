package handlers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
