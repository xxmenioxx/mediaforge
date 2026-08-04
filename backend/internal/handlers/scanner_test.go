package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIsHDRDoesNotTreatTenBitSDRAsHDR(t *testing.T) {
	stream := FFProbeStream{
		Profile: "Main 10", PixFmt: "yuv420p10le",
		ColorTransfer: "smpte170m", ColorPrimaries: "smpte170m",
	}
	if isHDR(stream) {
		t.Fatal("10-bit SDR must not be classified as HDR")
	}
}

func TestPersistFinalAssetSnapshotReplacesStalePathAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:final-asset-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/library/anime/Akira (1988)/Akira (1988).mkv"
	if err := db.Create(&models.ScanResult{Path: path, FileName: "Akira (1988).mkv", VideoCodec: "mpeg2video", SizeBytes: 10}).Error; err != nil {
		t.Fatal(err)
	}
	final := models.ScanResult{Path: path, FileName: "Akira (1988).mkv", VideoCodec: "hevc", SizeBytes: 20, Width: 720, Height: 460}
	if err := persistFinalAssetSnapshot(db, &final); err != nil {
		t.Fatal(err)
	}
	var scans []models.ScanResult
	if err := db.Where("path = ?", path).Find(&scans).Error; err != nil {
		t.Fatal(err)
	}
	if len(scans) != 1 || scans[0].VideoCodec != "hevc" || scans[0].SizeBytes != 20 || scans[0].Height != 460 {
		t.Fatalf("unexpected final snapshot: %#v", scans)
	}
}

func TestIsHDRDetectsPQAndHDRMetadata(t *testing.T) {
	if !isHDR(FFProbeStream{ColorTransfer: "smpte2084", ColorPrimaries: "bt2020"}) {
		t.Fatal("PQ transfer must be classified as HDR")
	}
	if !isHDR(FFProbeStream{SideDataList: []map[string]any{{"side_data_type": "Mastering display metadata"}}}) {
		t.Fatal("mastering display metadata must be classified as HDR")
	}
}

func TestStreamSizeBytesUsesMatroskaStatistics(t *testing.T) {
	stream := FFProbeStream{Tags: map[string]string{"NUMBER_OF_BYTES-eng": "46764973", "BPS-eng": "252532"}}
	if got := streamSizeBytes(stream); got != 46764973 {
		t.Fatalf("size=%d", got)
	}
	if got := streamBitrate(stream); got != 252532 {
		t.Fatalf("bitrate=%d", got)
	}
}

func TestScanCacheInvalidatesWhenMediaWasReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(path, []byte("new converted asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	modifiedAt := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	current := models.ScanResult{SizeBytes: info.Size(), CreatedAt: modifiedAt.Add(time.Second)}
	if !scanCacheMatchesFile(current, info) {
		t.Fatal("matching size and older media mtime should reuse the scan")
	}
	if scanCacheMatchesFile(models.ScanResult{SizeBytes: info.Size() + 1, CreatedAt: time.Now()}, info) {
		t.Fatal("changed media size must invalidate the scan")
	}
	if scanCacheMatchesFile(models.ScanResult{SizeBytes: info.Size(), CreatedAt: modifiedAt.Add(-time.Second)}, info) {
		t.Fatal("media modified after the scan must invalidate the scan")
	}
}
