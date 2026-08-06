package scheduler

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVideoToolboxOutputEstimateIncludesCopiedAudio(t *testing.T) {
	profile := models.Profile{AudioCodec: "copy", PreserveSubtitles: true, WorkerConfig: models.JSONMap{"hardwareQualityPreset": "recommended"}}
	source := mediaEstimate{DurationSeconds: 3600, VideoBitrate: 4_000_000, AudioBitrate: 384_000, SubtitleBitrate: 8_000, VideoHeight: 480, AudioTracks: 2}
	estimate, ok := estimatePlannedOutput(profile, "hevc_videotoolbox", source, 0)
	if !ok {
		t.Fatal("expected a VideoToolbox source-aware estimate")
	}
	// Recommended SD uses a 1.29 Mbps base and 1.33 Mbps effective Auto target; audio must be
	// added independently rather than hidden in the source container bitrate.
	if estimate.VideoBytes != 598_500_000 || estimate.AudioBytes != 172_800_000 || estimate.SubtitleBytes != 3_600_000 {
		t.Fatalf("unexpected stream breakdown: %#v", estimate)
	}
	if estimate.MinBytes >= estimate.MaxBytes || estimate.Method != "videotoolbox_source_video_bitrate" || estimate.Confidence != "medium" {
		t.Fatalf("unexpected VideoToolbox estimate: %#v", estimate)
	}
}

func TestProfileEstimateFingerprintChangesWithEffectiveConfig(t *testing.T) {
	base := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "globalQuality": 25}}
	changed := base
	changed.WorkerConfig = models.JSONMap{"videoEncoder": "hevc_qsv", "globalQuality": 22}
	if ProfileEstimateFingerprint(base) == ProfileEstimateFingerprint(changed) {
		t.Fatal("quality change must invalidate measured estimate")
	}
}

func TestPersistedSampleRequiresMatchingProfileAndSource(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sample-estimate?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	profile := models.Profile{ID: 4, ProfileVersion: 2, VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv"}}
	record := models.JSONMap{"path": path, "profileId": profile.ID, "profileVersion": profile.ProfileVersion, "profileFingerprint": ProfileEstimateFingerprint(profile), "sourceSize": strconv.FormatInt(info.Size(), 10), "sourceModifiedNs": strconv.FormatInt(info.ModTime().UnixNano(), 10), "estimate": models.JSONMap{"estimatedVideoBytes": int64(1234), "effectiveEncoder": "hevc_qsv"}}
	if err := db.Create(&models.AppSetting{Key: "profileSampleEstimates", Value: models.JSONMap{"records": []interface{}{record}}}).Error; err != nil {
		t.Fatal(err)
	}
	if bytes, encoder, ok := persistedProfileSampleEstimate(db, path, profile); !ok || bytes != 1234 || encoder != "hevc_qsv" {
		t.Fatalf("matching sample not found: %d %q %v", bytes, encoder, ok)
	}
	profile.QualityValue = 18
	if _, _, ok := persistedProfileSampleEstimate(db, path, profile); ok {
		t.Fatal("changed profile reused stale sample")
	}
	profile.QualityValue = 20
	time.Sleep(time.Millisecond)
	if err := os.WriteFile(path, []byte("changed source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := persistedProfileSampleEstimate(db, path, profile); ok {
		t.Fatal("changed source reused stale sample")
	}
}

func TestBitrateFromMKVStatisticsTags(t *testing.T) {
	if got := bitrateFromStreamTags(map[string]string{"BPS-eng": "640000"}, 100); got != 640000 {
		t.Fatalf("BPS bitrate=%d", got)
	}
	if got := bitrateFromStreamTags(map[string]string{"NUMBER_OF_BYTES-eng": "8000000"}, 100); got != 640000 {
		t.Fatalf("byte-derived bitrate=%d", got)
	}
}

func TestQSVOutputEstimateIsRangeAndKeepsAudio(t *testing.T) {
	profile := models.Profile{AudioCodec: "copy", WorkerConfig: models.JSONMap{"hardwareQualityPreset": "recommended"}}
	source := mediaEstimate{DurationSeconds: 100, VideoBitrate: 4_000_000, AudioBitrate: 640_000, AudioTracks: 3}
	estimate, ok := estimatePlannedOutput(profile, "hevc_qsv", source, 0)
	if !ok || estimate.MinBytes >= estimate.MaxBytes {
		t.Fatalf("expected QSV estimate range, got %#v (ok=%v)", estimate, ok)
	}
	if estimate.AudioBytes != 8_000_000 || estimate.Method != "qsv_preset_calibration_range" || estimate.Confidence != "low" {
		t.Fatalf("unexpected QSV estimate: %#v", estimate)
	}
}

func TestQSVOutputEstimateUsesMeasuredAndHistoricalEvidence(t *testing.T) {
	profile := models.Profile{AudioCodec: "copy", WorkerConfig: models.JSONMap{"hardwareQualityPreset": "recommended", "qsvRateControl": "icq"}}
	measured := mediaEstimate{DurationSeconds: 100, VideoBitrate: 4_000_000, MeasuredVideoBitrate: 900_000}
	estimate, ok := estimatePlannedOutput(profile, "hevc_qsv", measured, 0)
	if !ok || estimate.Confidence != "high" || estimate.Method != "five_distributed_profile_samples" {
		t.Fatalf("unexpected measured estimate: %#v", estimate)
	}
	historical := mediaEstimate{DurationSeconds: 100, VideoBitrate: 4_000_000, HistoricalRatioMin: .2, HistoricalRatioMax: .3, HistoricalSamples: 3}
	estimate, ok = estimatePlannedOutput(profile, "hevc_qsv", historical, 0)
	if !ok || estimate.Confidence != "medium" || estimate.Method != "qsv_historical_encoder_ratios" {
		t.Fatalf("unexpected historical estimate: %#v", estimate)
	}
}

func TestHistoricalQSVRatiosRequireTwoCompatibleSamples(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:qsv-history?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	records := []interface{}{
		models.JSONMap{"estimate": models.JSONMap{"effectiveEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "durationSeconds": 100.0, "sourceVideoBitrate": int64(4_000_000), "estimatedVideoBytes": int64(10_000_000)}},
		models.JSONMap{"estimate": models.JSONMap{"effectiveEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "durationSeconds": 100.0, "sourceVideoBitrate": int64(4_000_000), "estimatedVideoBytes": int64(15_000_000)}},
	}
	if err := db.Create(&models.AppSetting{Key: "profileSampleEstimates", Value: models.JSONMap{"records": records}}).Error; err != nil {
		t.Fatal(err)
	}
	minimum, maximum, count := historicalQSVSampleRatios(db, "recommended")
	if count != 2 || minimum != .2 || maximum != .3 {
		t.Fatalf("unexpected historical ratios: min=%f max=%f count=%d", minimum, maximum, count)
	}
}

func TestHistoricalQSVRatiosIncludeCompletedEncoderResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:qsv-completed-history?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	records := []interface{}{
		models.JSONMap{"effectiveEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "sourceVideoBitrate": 4_000_000.0, "outputVideoBitrate": 2_000_000.0},
		models.JSONMap{"effectiveEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "sourceVideoBitrate": 4_000_000.0, "outputVideoBitrate": 2_400_000.0},
		models.JSONMap{"effectiveEncoder": "hevc_videotoolbox", "hardwareQualityPreset": "recommended", "sourceVideoBitrate": 4_000_000.0, "outputVideoBitrate": 1_000_000.0},
	}
	if err := db.Create(&models.AppSetting{Key: "encoderResultHistory", Value: models.JSONMap{"records": records}}).Error; err != nil {
		t.Fatal(err)
	}
	minimum, maximum, count := historicalQSVSampleRatios(db, "recommended")
	if count != 2 || minimum != 0.5 || maximum != 0.6 {
		t.Fatalf("completed QSV history = %.2f..%.2f (%d), want 0.50..0.60 (2)", minimum, maximum, count)
	}
}

func TestVideoToolboxEstimateUsesCroppedOutputHeight(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"hardwareQualityPreset": "recommended", "videoFilters": "crop=720:460:0:10"}}
	target, ok := videoToolboxTargetKbps(profile, mediaEstimate{VideoBitrate: 1_000_000, VideoWidth: 1920, VideoHeight: 1080})
	if !ok || target != 1290 {
		t.Fatalf("expected SD floor after crop, got %d (ok=%v)", target, ok)
	}
}
