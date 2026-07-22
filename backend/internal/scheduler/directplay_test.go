package scheduler

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPlannedDirectPlayFlagsHEVCMain10BrowserRisk(t *testing.T) {
	db := directPlayTestDB(t)
	profile := models.Profile{Container: "mkv", CodecFamily: "hevc", BitDepth: 10, AudioCodec: "copy", PreserveSubtitles: true, WorkerConfig: models.JSONMap{}}
	report, err := EvaluatePlannedDirectPlay(db, profile)
	if err != nil {
		t.Fatal(err)
	}
	if report.Risk == "low" || report.LowestScore >= 85 {
		t.Fatalf("expected compatibility risk: %#v", report)
	}
}

func TestPlannedDirectPlayRewardsCompatibilityProfile(t *testing.T) {
	db := directPlayTestDB(t)
	profile := models.Profile{Container: "mp4", CodecFamily: "h264", BitDepth: 8, AudioCodec: "aac", WorkerConfig: models.JSONMap{"addAacStereoDefault": true, "preferSrtSubtitles": true}}
	report, err := EvaluatePlannedDirectPlay(db, profile)
	if err != nil {
		t.Fatal(err)
	}
	if report.LowestScore != 100 || report.Risk != "low" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestPlannedDirectPlayCanRequireReview(t *testing.T) {
	db := directPlayTestDB(t)
	setting := models.AppSetting{Key: "directPlay", Value: models.JSONMap{"enabled": true, "strategy": "balanced", "targetClients": []string{"jellyfin_web"}, "minimumScore": 90, "enforcement": "block"}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{Container: "mkv", CodecFamily: "hevc", BitDepth: 10, AudioCodec: "copy", PreserveSubtitles: true, WorkerConfig: models.JSONMap{}}
	report, err := EvaluatePlannedDirectPlay(db, profile)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Blocked {
		t.Fatalf("expected DirectPlay review block: %#v", report)
	}
}

func TestActualDirectPlayReadsFinalStreams(t *testing.T) {
	db := directPlayTestDB(t)
	probe := models.JSONMap{
		"format": map[string]any{"format_name": "matroska,webm"},
		"streams": []any{
			map[string]any{"codec_type": "video", "codec_name": "hevc", "pix_fmt": "yuv420p10le"},
			map[string]any{"codec_type": "audio", "codec_name": "aac", "channels": float64(2), "disposition": map[string]any{"default": float64(1)}},
			map[string]any{"codec_type": "subtitle", "codec_name": "ass"},
		},
	}
	report, err := EvaluateActualDirectPlay(db, probe)
	if err != nil {
		t.Fatal(err)
	}
	if report.Estimated || report.Risk == "low" || len(report.Clients) == 0 {
		t.Fatalf("unexpected final report: %#v", report)
	}
}

func directPlayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	return db
}
