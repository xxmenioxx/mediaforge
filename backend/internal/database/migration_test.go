package database

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateUpgradesLegacyQueueWithoutLosingJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{Name: "Legacy", Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "libx265"}}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	legacySchema := `CREATE TABLE queue_jobs (id integer primary key autoincrement, media_path text not null, library_id integer not null, profile_id integer not null, priority integer default 5, status text default 'queued', progress integer default 0, created_at datetime, updated_at datetime)`
	if err := db.Exec(legacySchema).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO queue_jobs (media_path, library_id, profile_id, status, priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", "/raw/legacy.mkv", 1, profile.ID, "queued", 5).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var job models.QueueJob
	if err := db.First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.MediaPath != "/raw/legacy.mkv" || job.Stage != "queued" || len(job.ProfileSnapshot) == 0 || job.ActiveExecutionPlanID == nil {
		t.Fatalf("legacy job was not fully upgraded: %#v", job)
	}
	var reservation models.SchedulerReservation
	if err := db.Where("job_id = ?", job.ID).First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.AssetKey != job.MediaPath {
		t.Fatalf("reservation was not backfilled: %#v", reservation)
	}
}

func TestMigrateRuntimePolicyPreservesLegacyCustomLimitsAsOverrides(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "runtime.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "runtimePolicy", Value: models.JSONMap{"mode": "manual", "selectedProfile": "workstation_balanced", "fallbackProfile": "desktop_safe", "preventSleepDuringJobs": true}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "schedulerLimits", Value: models.JSONMap{"useProfileDefaults": false, "maxRunningJobs": 4, "minFreeRamGb": 12, "allowDirectMode": false}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "runtimePolicy").Error; err != nil {
		t.Fatal(err)
	}
	if intFromJSON(setting.Value["schemaVersion"]) != 2 || setting.Value["preferredProfile"] != "workstation_balanced" {
		t.Fatalf("runtime policy was not migrated: %#v", setting.Value)
	}
	overrides, ok := setting.Value["overrides"].(map[string]any)
	if !ok {
		t.Fatalf("runtime overrides missing: %#v", setting.Value)
	}
	profile, ok := overrides["workstation_balanced"].(map[string]any)
	if !ok || intFromJSON(profile["maxRunningJobs"]) != 4 || intFromJSON(profile["minFreeRamGb"]) != 12 || profile["preventSleepDuringJobs"] != true {
		t.Fatalf("legacy values not preserved: %#v", overrides)
	}
}

func TestMigrateReclassifiesLegacyHardwareQualityPreset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "preset.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{Name: "Legacy Compact", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "hevc_videotoolbox", "hardwareQualityPreset": "compact"}}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&profile, profile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if profile.WorkerConfig["hardwareQualityPreset"] != "recommended" || intFromJSON(profile.WorkerConfig["hardwareQualityPresetScale"]) != 2 {
		t.Fatalf("legacy preset quality intent was not preserved: %#v", profile.WorkerConfig)
	}
}

func TestMigratePreservesEveryLegacyHardwareQualityLevel(t *testing.T) {
	cases := map[string]string{
		"compact": "recommended", "medium": "best_quality", "recommended": "high_quality",
		"best_quality": "archive", "high_quality": "master",
	}
	for legacy, expected := range cases {
		profile := models.Profile{ProfileVersion: 1, WorkerConfig: models.JSONMap{"hardwareQualityPreset": legacy}}
		if !migrateHardwareQualityPresetScale(&profile) {
			t.Fatalf("legacy preset %s was not migrated", legacy)
		}
		if profile.WorkerConfig["hardwareQualityPreset"] != expected || intFromJSON(profile.WorkerConfig["hardwareQualityPresetScale"]) != 2 {
			t.Fatalf("legacy preset %s migrated to %#v, expected %s", legacy, profile.WorkerConfig, expected)
		}
	}
}

func TestMigrateBackfillsLegacyTrackPathAssignments(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "track-assignments.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/anime/Show"
	if err := db.Create(&models.AppSetting{Key: "trackProfilePathAssignments", Value: models.JSONMap{
		"assignments": models.JSONMap{path: "jpn-spa"},
	}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{
		"profiles": models.JSONList{models.JSONMap{"key": "jpn-spa", "name": "Japanese + Spanish", "sourceAssetPath": "/media/raw/anime/Show/01.mkv"}},
	}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var assignment models.ProfileAssignment
	if err := db.Where("target_type = ? AND target_path = ? AND media_type = ?", "path", path, "tracks").First(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if assignment.ProfileKey != "jpn-spa" || assignment.Selection != "profile" {
		t.Fatalf("legacy assignment not preserved: %#v", assignment)
	}
	var profiles models.AppSetting
	if err := db.First(&profiles, "key = ?", "trackProfiles").Error; err != nil {
		t.Fatal(err)
	}
	values := settingValuesForTest(profiles.Value["profiles"])
	if len(values) != 1 || jsonMapFromAny(values[0])["scope"] != "path" {
		t.Fatalf("legacy assigned profile was not promoted to path scope: %#v", profiles.Value)
	}
	var legacyCount int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "trackProfilePathAssignments").Count(&legacyCount).Error; err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy assignment setting remained after migration")
	}
}

func TestMigrateAddsDraftIntentAndSnapshotRecommendationColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "recommendation-columns.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&models.Profile{}, "optimization_intent") {
		t.Fatal("profiles.optimization_intent was not migrated")
	}
	if !db.Migrator().HasColumn(&models.ScanResult{}, "frame_structure_recommendation") {
		t.Fatal("scan_results.frame_structure_recommendation was not migrated")
	}
	if !db.Migrator().HasColumn(&models.ScanResult{}, "hevc_level_recommendation") {
		t.Fatal("scan_results.hevc_level_recommendation was not migrated")
	}
	if !db.Migrator().HasColumn(&models.ScanResult{}, "cadence_analysis") {
		t.Fatal("expected cadence_analysis scan column")
	}
	if !db.Migrator().HasColumn(&models.ScanResult{}, "cadence_recommendation") {
		t.Fatal("expected cadence_recommendation scan column")
	}
}

func TestMigrateAddsTestEncodeLifecycleTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test-encode-tables.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&models.TestEncode{}) || !db.Migrator().HasTable(&models.TaskReservation{}) {
		t.Fatal("Test Encode lifecycle tables were not migrated")
	}
	for _, column := range []string{"requested_configuration", "effective_configuration", "configuration_hash", "ffmpeg_command", "temporary_path", "expires_at"} {
		if !db.Migrator().HasColumn(&models.TestEncode{}, column) {
			t.Fatalf("test_encodes.%s was not migrated", column)
		}
	}
}

func TestMigrateRestoresMissingTestEncodeFFmpegCommandColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test-encode-command-column.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	testEncode := models.TestEncode{
		SourcePath:          "/media/source.mkv",
		LibraryID:           1,
		ConfigurationSource: "profile",
		ConfigurationHash:   "legacy-hash",
		Status:              "queued",
		Phase:               "queued",
	}
	if err := db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&models.TestEncode{}, "FFmpegCommand"); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&models.TestEncode{}, "ffmpeg_command") {
		t.Fatal("test setup did not remove test_encodes.ffmpeg_command")
	}

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&models.TestEncode{}, "ffmpeg_command") {
		t.Fatal("migration did not restore test_encodes.ffmpeg_command")
	}
	if err := db.Model(&models.TestEncode{}).Where("id = ?", testEncode.ID).Update("ffmpeg_command", "ffmpeg -i source.mkv output.mkv").Error; err != nil {
		t.Fatal(err)
	}
	var migrated models.TestEncode
	if err := db.First(&migrated, testEncode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.SourcePath != testEncode.SourcePath || migrated.FFmpegCommand == "" {
		t.Fatalf("existing Test Encode was not preserved and upgraded: %#v", migrated)
	}
}

func settingValuesForTest(value any) []any {
	switch values := value.(type) {
	case []any:
		return values
	case models.JSONList:
		return []any(values)
	default:
		return nil
	}
}
