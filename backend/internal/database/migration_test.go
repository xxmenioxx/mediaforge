package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestValidationRunsAndOverridesAreAppendOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:validation-append-only?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate validation quality gate: %v", err)
	}
	run := models.ValidationRun{
		RunID: "validation-run:test", JobID: 1, ExecutionNumber: 1, ExecutionPlanID: 1, ExecutionPlanVersion: 1,
		ExecutionPlanFingerprint: "plan", ResultArtifactID: "result", ResultGenerationToken: "generation",
		ResultGenerationVersion: 1, ResultOutputRevision: "revision", SchemaVersion: "schema", ValidatorVersion: "validator",
		RulesetID: "rules", RulesetVersion: "1", ActorSource: "system", ActorID: "system",
		Outcome: "PASS", Findings: models.JSONList{}, Report: models.JSONMap{}, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ValidationRun{}).Where("run_id = ?", run.RunID).Update("outcome", "FAIL").Error; err == nil {
		t.Fatal("ValidationRun update was not rejected")
	}
	if err := db.Where("run_id = ?", run.RunID).Delete(&models.ValidationRun{}).Error; err == nil {
		t.Fatal("ValidationRun delete was not rejected")
	}
	event := models.ValidationOverride{
		EventID: "validation-override:test", ValidationRunID: run.RunID, ActorSource: "deployment", ActorID: "local_operator",
		Reason: "required for append-only migration test", RuleKeys: models.JSONList{"video.properties"},
		RequestedHandoff: "publisher", OverrideVersion: "validation-override-v1", CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ValidationOverride{}).Where("event_id = ?", event.EventID).Update("reason", "rewritten").Error; err == nil {
		t.Fatal("ValidationOverride update was not rejected")
	}
	if err := db.Where("event_id = ?", event.EventID).Delete(&models.ValidationOverride{}).Error; err == nil {
		t.Fatal("ValidationOverride delete was not rejected")
	}
}

func TestMigrateAddsJellyfinOutcomeLedgerToExistingSettingsDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:jellyfin-ledger-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "existing", Value: models.JSONMap{"kept": true}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "jellyfin", Value: models.JSONMap{"baseUrl": "http://jellyfin:8096", "apiToken": "legacy-migration-secret"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate existing database: %v", err)
	}
	if !db.Migrator().HasTable(&models.JellyfinIntegrationOutcome{}) {
		t.Fatal("Jellyfin integration outcome table was not migrated")
	}
	if !db.Migrator().HasTable(&models.ProtectedCredential{}) {
		t.Fatal("protected credential table was not migrated")
	}
	var jellyfinSetting models.AppSetting
	if err := db.First(&jellyfinSetting, "key = ?", "jellyfin").Error; err != nil {
		t.Fatal(err)
	}
	if _, stillStored := jellyfinSetting.Value["apiToken"]; stillStored || jellyfinSetting.Value["credentialRef"] != "jellyfin.apiToken" {
		t.Fatalf("legacy Jellyfin token was not removed from AppSetting: %#v", jellyfinSetting.Value)
	}
	var protected models.ProtectedCredential
	if err := db.First(&protected, "credential_ref = ?", "jellyfin.apiToken").Error; err != nil || protected.SecretValue != "legacy-migration-secret" {
		t.Fatalf("legacy Jellyfin token was not migrated: %#v err=%v", protected, err)
	}
	var existing models.AppSetting
	if err := db.First(&existing, "key = ?", "existing").Error; err != nil || existing.Value["kept"] != true {
		t.Fatalf("existing settings row was not preserved: %#v err=%v", existing, err)
	}
	now := time.Now().UTC()
	row := models.JellyfinIntegrationOutcome{SourceKey: "asset-published:event", Reason: "asset_published", EffectiveTarget: "server-wide", Status: "pending", FirstTriggeredAt: now, DueAt: now}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := row
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("duplicate Jellyfin source identity was not constrained")
	}
}

func TestPublisherAttemptEvidenceAndAuditAreAppendOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:publisher-append-only?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate Publisher delivery: %v", err)
	}
	publication := models.PublisherPublication{
		PublicationID: "publication:test", QueueJobID: 1, ExecutionNumber: 1,
		ExecutionPlanID: 1, ExecutionPlanVersion: 1, ExecutionPlanFingerprint: "plan",
		LibraryID: 1, DestinationRoot: "/library", DestinationPath: "/library/episode.mkv",
		PublishMode: "standard", ResultArtifactID: "result", ResultGenerationToken: "generation",
		ResultGenerationVersion: 1, ResultOutputRevision: "revision", ValidationRunID: "validation-run:test",
		State: "READY", Manifest: models.JSONList{}, OriginalArchiveStatus: "pending",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&publication).Update("state", "HELD").Error; err != nil {
		t.Fatalf("mutable Publisher state update was rejected: %v", err)
	}
	if err := db.Model(&publication).Update("destination_path", "/library/changed.mkv").Error; err == nil {
		t.Fatal("Publisher publication identity update was not rejected")
	}
	secondExecution := publication
	secondExecution.ID = 0
	secondExecution.PublicationID = "publication:test:execution-2"
	secondExecution.ExecutionNumber = 2
	secondExecution.State = "READY"
	if err := db.Create(&secondExecution).Error; err != nil {
		t.Fatalf("a later Queue execution could not receive a new publication identity: %v", err)
	}
	attempt := models.PublisherPublicationAttempt{AttemptID: "attempt:test", PublicationID: "publication:test", FencingGeneration: 1, ActorSource: "local_operator", ActorID: "local_operator", CreatedAt: time.Now().UTC()}
	evidence := models.PublisherAttemptEvidence{EvidenceID: "attempt-evidence:test", PublicationID: attempt.PublicationID, AttemptID: attempt.AttemptID, Version: 1, Phase: "claim", Outcome: "acquired", CreatedAt: time.Now().UTC()}
	audit := models.PublisherAuditEvent{EventID: "publisher-action:test", PublicationID: attempt.PublicationID, Version: 1, Action: "hold", ActorSource: "local_operator", ActorID: "local_operator", Reason: "review", CreatedAt: time.Now().UTC()}
	for _, value := range []any{&attempt, &evidence, &audit} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&models.PublisherPublicationAttempt{}).Where("attempt_id = ?", attempt.AttemptID).Update("actor_id", "rewritten").Error; err == nil {
		t.Fatal("Publisher attempt claim evidence update was not rejected")
	}
	if err := db.Where("attempt_id = ?", attempt.AttemptID).Delete(&models.PublisherPublicationAttempt{}).Error; err == nil {
		t.Fatal("Publisher attempt claim evidence delete was not rejected")
	}
	if err := db.Model(&models.PublisherAttemptEvidence{}).Where("evidence_id = ?", evidence.EvidenceID).Update("outcome", "rewritten").Error; err == nil {
		t.Fatal("Publisher attempt phase evidence update was not rejected")
	}
	if err := db.Where("evidence_id = ?", evidence.EvidenceID).Delete(&models.PublisherAttemptEvidence{}).Error; err == nil {
		t.Fatal("Publisher attempt phase evidence delete was not rejected")
	}
	if err := db.Model(&models.PublisherAuditEvent{}).Where("event_id = ?", audit.EventID).Update("reason", "rewritten").Error; err == nil {
		t.Fatal("Publisher action audit update was not rejected")
	}
	if err := db.Where("event_id = ?", audit.EventID).Delete(&models.PublisherAuditEvent{}).Error; err == nil {
		t.Fatal("Publisher action audit delete was not rejected")
	}
}

func TestMigrateInitializesRuntimeStateFromNewestLegacySnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "runtime-state.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RuntimeSnapshot{}); err != nil {
		t.Fatal(err)
	}
	older := models.RuntimeSnapshot{DetectedAt: time.Unix(100, 0).UTC(), Architecture: "amd64", SelectedProfile: "desktop_safe"}
	newer := models.RuntimeSnapshot{DetectedAt: time.Unix(200, 0).UTC(), Architecture: "arm64", SelectedProfile: "workstation_balanced"}
	if err := db.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var state models.RuntimeState
	if err := db.Where("singleton_key = ?", models.RuntimeStateSingletonKey).First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Architecture != newer.Architecture || state.SelectedProfile != newer.SelectedProfile || state.StableFingerprint == "" {
		t.Fatalf("runtime state was not initialized from newest legacy snapshot: %#v", state)
	}
	var count int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("legacy runtime history changed during migration: %d", count)
	}
}

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

func TestMigrateBackfillsSourceGroupIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "source-groups.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{
		Path:     "/media/raw/anime/Beck Mongolian Chop Squad/Season1/episode01.mkv",
		RootPath: "/media/raw", RelativePath: "anime/Beck Mongolian Chop Squad/Season1/episode01.mkv",
		GroupPath: "anime/Beck Mongolian Chop Squad/Season1", FileName: "episode01.mkv", Status: "unprocessed",
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&record, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.SourceGroupID == 0 || record.LogicalGroupPath != "/media/raw/anime/Beck Mongolian Chop Squad" || record.SourcePath != record.Path {
		t.Fatalf("source identity was not backfilled: %#v", record)
	}
	var group models.SourceGroup
	if err := db.First(&group, record.SourceGroupID).Error; err != nil {
		t.Fatal(err)
	}
	if group.Name != "anime" || group.SourcePath != "/media/raw/anime" {
		t.Fatalf("unexpected source group: %#v", group)
	}
}

func TestMigrateDoesNotBackfillSourceGroupsFromMissingHistoricalAssets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "missing-source-groups.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	records := []models.AssetRecord{
		{Path: "/media/raw/Porco_Rosso/movie.mkv", RootPath: "/media/raw", RelativePath: "Porco_Rosso/movie.mkv", FileName: "movie.mkv", Status: "unprocessed", Missing: true},
		{Path: "/media/raw/library-replacements/old.mkv", RootPath: "/media/raw", RelativePath: "library-replacements/old.mkv", FileName: "old.mkv", Status: "unprocessed", Missing: true},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var groupCount int64
	if err := db.Model(&models.SourceGroup{}).Count(&groupCount).Error; err != nil {
		t.Fatal(err)
	}
	if groupCount != 0 {
		t.Fatalf("missing historical assets created %d source groups", groupCount)
	}
	var preserved []models.AssetRecord
	if err := db.Order("id asc").Find(&preserved).Error; err != nil {
		t.Fatal(err)
	}
	if len(preserved) != 2 || !preserved[0].Missing || !preserved[1].Missing || preserved[0].SourceGroupID != 0 || preserved[1].SourceGroupID != 0 {
		t.Fatalf("historical assets were not preserved unchanged: %#v", preserved)
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
