package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func Open(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	dsn := path
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite has a single writer. Serializing writes at the connection pool
	// prevents SQLITE_BUSY/SQLITE_LOCKED failures across background services.
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.Library{},
		&models.Profile{},
		&models.ProfileAssignment{},
		&models.SourceGroup{},
		&models.AssetScopeConfiguration{},
		&models.QueueJob{},
		&models.QueueBatchPreflight{},
		&models.ExecutionPlan{},
		&models.RuntimeSnapshot{},
		&models.RuntimeState{},
		&models.SchedulerReservation{},
		&models.WorkerNode{},
		&models.ScanResult{},
		&models.AssetRecord{},
		&models.DirectPublication{},
		&models.AssetMaintenanceOperation{},
		&models.TestEncode{},
		&models.TestEncodeArtifact{},
		&models.ProfileQualification{},
		&models.TaskReservation{},
		&models.AppSetting{},
		&models.OCRDictionaryEntry{},
		&models.ProtectedCredential{},
		&models.CompatiblePreviewRequestRecord{},
		&models.CompatiblePreviewArtifactRecord{},
		&models.ValidationRun{},
		&models.ValidationOverride{},
		&models.PublisherPublication{},
		&models.PublisherPublicationAttempt{},
		&models.PublisherAttemptEvidence{},
		&models.PublisherAuditEvent{},
		&models.AssetPublishedOutbox{},
		&models.JellyfinIntegrationOutcome{},
		&models.AIResourceAdmissionRecord{},
		&models.AIResourceAdmissionCounter{},
	); err != nil {
		return err
	}
	if err := seedOCRDictionaryEntries(db); err != nil {
		return err
	}
	if err := backfillTestEncodeArtifactOwnership(db); err != nil {
		return err
	}
	if err := backfillTestEncodePolicy(db); err != nil {
		return err
	}
	if err := migrateSeededProfileProvenance(db); err != nil {
		return err
	}
	if err := migrateLegacyJellyfinCredential(db); err != nil {
		return err
	}
	if err := migrateValidationAppendOnly(db); err != nil {
		return err
	}
	if err := migrateProfileQualificationAppendOnly(db); err != nil {
		return err
	}
	if err := migratePublisherAppendOnly(db); err != nil {
		return err
	}
	if err := migrateRuntimePolicy(db); err != nil {
		return err
	}
	if err := runtimeinfo.InitializeLegacyState(db); err != nil {
		return err
	}

	if err := normalizeProfileContracts(db); err != nil {
		return err
	}
	if err := backfillProfileScopes(db); err != nil {
		return err
	}
	if err := backfillSourceGroups(db); err != nil {
		return err
	}
	if err := backfillLegacyTrackProfileAssignments(db); err != nil {
		return err
	}
	if err := backfillQueueProfileSnapshots(db); err != nil {
		return err
	}
	if err := backfillQueueLifecycleStages(db); err != nil {
		return err
	}
	if err := backfillQueueExecutionNumbers(db); err != nil {
		return err
	}
	if err := backfillQueuePositions(db); err != nil {
		return err
	}
	if err := scheduler.ReconcileReservations(db); err != nil {
		return err
	}
	return backfillPendingExecutionPlans(db)
}

func seedOCRDictionaryEntries(db *gorm.DB) error {
	entries := []models.OCRDictionaryEntry{
		{ID: "builtin:eng:im", Language: "eng", Pattern: "|'m", Replacement: "I'm", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
		{ID: "builtin:eng:ive", Language: "eng", Pattern: "|'ve", Replacement: "I've", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
		{ID: "builtin:eng:ill", Language: "eng", Pattern: "|'ll", Replacement: "I'll", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
		{ID: "builtin:eng:id", Language: "eng", Pattern: "|'d", Replacement: "I'd", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
		{ID: "builtin:eng:cant", Language: "eng", Pattern: "| can't", Replacement: "I can't", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
		{ID: "builtin:eng:dont", Language: "eng", Pattern: "| don't", Replacement: "I don't", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
		{ID: "builtin:eng:am_i", Language: "eng", Pattern: "am |", Replacement: "am I", MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1},
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"language", "pattern", "replacement", "match_mode", "boundary_policy",
			"left_context", "right_context", "case_policy", "priority", "source", "rule_version", "updated_at",
		}),
	}).Create(&entries).Error
}

func backfillTestEncodeArtifactOwnership(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&models.TestEncode{}) || !db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var tests []models.TestEncode
		if err := tx.Find(&tests).Error; err != nil {
			return fmt.Errorf("load Test Encodes for artifact ownership backfill: %w", err)
		}
		for _, test := range tests {
			createdAt := test.CreatedAt
			if createdAt.IsZero() {
				createdAt = time.Now().UTC()
			}
			artifacts := []models.TestEncodeArtifact{}
			seen := map[string]struct{}{}
			add := func(path, kind string, expected bool, sizeBytes int64) {
				path = filepath.Clean(strings.TrimSpace(path))
				if path == "." || path == "" {
					return
				}
				if _, exists := seen[path]; exists {
					return
				}
				seen[path] = struct{}{}
				artifacts = append(artifacts, models.TestEncodeArtifact{
					TestEncodeID: test.ID, LibraryID: test.LibraryID, Path: path, Type: kind,
					Expected: expected, State: "owned", SizeBytes: max(int64(0), sizeBytes), CreatedAt: createdAt, UpdatedAt: createdAt,
				})
			}
			add(test.OutputPath, models.TestEncodeArtifactMediaOutput, true, test.OutputSizeBytes)
			add(test.TemporaryPath, models.TestEncodeArtifactTemporaryMedia, false, 0)
			for _, raw := range test.SubtitleArtifacts {
				var item map[string]any
				switch value := raw.(type) {
				case map[string]any:
					item = value
				case models.JSONMap:
					item = value
				}
				if item == nil {
					continue
				}
				path, _ := item["stagedPath"].(string)
				add(path, models.TestEncodeArtifactSubtitle, true, 0)
			}
			for _, artifact := range artifacts {
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "test_encode_id"}, {Name: "path"}},
					DoNothing: true,
				}).Create(&artifact).Error; err != nil {
					return fmt.Errorf("backfill Test Encode %d artifact %q: %w", test.ID, artifact.Path, err)
				}
			}
		}
		return nil
	})
}

func backfillTestEncodePolicy(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&models.AppSetting{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var existing models.AppSetting
		findErr := tx.First(&existing, "key = ?", "testEncodePolicy").Error
		if findErr != nil && findErr != gorm.ErrRecordNotFound {
			return findErr
		}
		legacyRetentionHours := 0
		if existing.Value != nil {
			legacyRetentionHours = intFromJSON(existing.Value["temporaryRetentionHours"])
			if legacyRetentionHours <= 0 {
				legacyRetentionHours = intFromJSON(existing.Value["retentionHours"])
			}
		}
		if legacyRetentionHours <= 0 && tx.Migrator().HasTable(&models.AppSetting{}) {
			var housekeeping models.AppSetting
			if err := tx.First(&housekeeping, "key = ?", "housekeeping").Error; err == nil {
				legacyRetentionHours = intFromJSON(housekeeping.Value["testEncodeRetentionHours"])
			} else if err != gorm.ErrRecordNotFound {
				return err
			}
		}
		if legacyRetentionHours <= 0 {
			legacyRetentionHours = models.DefaultTestEncodePolicy().TemporaryRetentionHours
		}
		legacyRetentionHours = min(24*365, max(1, legacyRetentionHours))

		value := models.JSONMap{}
		for key, item := range existing.Value {
			value[key] = item
		}
		defaults := models.DefaultTestEncodePolicy().JSONMap()
		for key, item := range defaults {
			if _, exists := value[key]; !exists {
				value[key] = item
			}
		}
		if intFromJSON(existing.Value["temporaryRetentionHours"]) <= 0 {
			value["temporaryRetentionHours"] = legacyRetentionHours
		}
		setting := models.AppSetting{Key: "testEncodePolicy", Value: value, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if findErr == nil {
			if err := tx.Model(&existing).Updates(map[string]any{"value": value, "updated_at": setting.UpdatedAt}).Error; err != nil {
				return fmt.Errorf("backfill Test Encode Settings policy: %w", err)
			}
			return nil
		}
		if err := tx.Create(&setting).Error; err != nil {
			return fmt.Errorf("create Test Encode Settings policy: %w", err)
		}
		return nil
	})
}

func migrateLegacyJellyfinCredential(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var setting models.AppSetting
		err := tx.First(&setting, "key = ?", "jellyfin").Error
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if setting.Value == nil {
			setting.Value = models.JSONMap{}
		}
		legacyToken, hasLegacyToken := setting.Value["apiToken"]
		credentialRef, hasCredentialRef := setting.Value["credentialRef"]
		if !hasLegacyToken && hasCredentialRef && credentialRef == "jellyfin.apiToken" {
			return nil
		}
		if token, ok := legacyToken.(string); ok && token != "" {
			var credential models.ProtectedCredential
			err := tx.First(&credential, "credential_ref = ?", "jellyfin.apiToken").Error
			if err == gorm.ErrRecordNotFound {
				credential = models.ProtectedCredential{CredentialRef: "jellyfin.apiToken", SecretValue: token}
				if err := tx.Create(&credential).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
		delete(setting.Value, "apiToken")
		setting.Value["credentialRef"] = "jellyfin.apiToken"
		return tx.Model(&models.AppSetting{}).Where("key = ?", "jellyfin").UpdateColumn("value", setting.Value).Error
	})
}

func migratePublisherAppendOnly(db *gorm.DB) error {
	if err := db.Exec("DROP INDEX IF EXISTS idx_publisher_publications_queue_job_id").Error; err != nil {
		return err
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_publisher_publications_queue_job_execution ON publisher_publications(queue_job_id, execution_number)").Error; err != nil {
		return err
	}
	for _, table := range []string{"publisher_publication_attempts", "publisher_attempt_evidences", "publisher_audit_events"} {
		for _, operation := range []string{"UPDATE", "DELETE"} {
			statement := fmt.Sprintf(
				"CREATE TRIGGER IF NOT EXISTS %s_no_%s BEFORE %s ON %s BEGIN SELECT RAISE(ABORT, '%s is append-only'); END",
				table, strings.ToLower(operation), operation, table, table,
			)
			if err := db.Exec(statement).Error; err != nil {
				return err
			}
		}
	}
	if err := db.Exec(`CREATE TRIGGER IF NOT EXISTS publisher_publications_identity_no_update
		BEFORE UPDATE ON publisher_publications
		WHEN OLD.publication_id IS NOT NEW.publication_id
			OR OLD.queue_job_id IS NOT NEW.queue_job_id
			OR OLD.execution_number IS NOT NEW.execution_number
			OR OLD.execution_plan_id IS NOT NEW.execution_plan_id
			OR OLD.execution_plan_version IS NOT NEW.execution_plan_version
			OR OLD.execution_plan_fingerprint IS NOT NEW.execution_plan_fingerprint
			OR OLD.library_id IS NOT NEW.library_id
			OR OLD.destination_root IS NOT NEW.destination_root
			OR OLD.destination_path IS NOT NEW.destination_path
			OR OLD.publish_mode IS NOT NEW.publish_mode
			OR OLD.result_artifact_id IS NOT NEW.result_artifact_id
			OR OLD.result_generation_token IS NOT NEW.result_generation_token
			OR OLD.result_generation_version IS NOT NEW.result_generation_version
			OR OLD.result_output_revision IS NOT NEW.result_output_revision
			OR OLD.validation_run_id IS NOT NEW.validation_run_id
			OR OLD.validation_override_event_ids IS NOT NEW.validation_override_event_ids
			OR OLD.source_asset_record_id IS NOT NEW.source_asset_record_id
			OR OLD.source_asset_record_created_at IS NOT NEW.source_asset_record_created_at
			OR OLD.source_asset_record_modified_at IS NOT NEW.source_asset_record_modified_at
			OR OLD.source_asset_record_size_bytes IS NOT NEW.source_asset_record_size_bytes
			OR OLD.source_path IS NOT NEW.source_path
			OR OLD.created_at IS NOT NEW.created_at
		BEGIN SELECT RAISE(ABORT, 'publisher publication identity is immutable'); END`).Error; err != nil {
		return err
	}
	return nil
}

func migrateValidationAppendOnly(db *gorm.DB) error {
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS validation_runs_no_update BEFORE UPDATE ON validation_runs BEGIN SELECT RAISE(ABORT, 'validation runs are immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS validation_runs_no_delete BEFORE DELETE ON validation_runs BEGIN SELECT RAISE(ABORT, 'validation runs are append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS validation_overrides_no_update BEFORE UPDATE ON validation_overrides BEGIN SELECT RAISE(ABORT, 'validation overrides are immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS validation_overrides_no_delete BEFORE DELETE ON validation_overrides BEGIN SELECT RAISE(ABORT, 'validation overrides are append-only'); END`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateProfileQualificationAppendOnly(db *gorm.DB) error {
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS profile_qualifications_no_update BEFORE UPDATE ON profile_qualifications BEGIN SELECT RAISE(ABORT, 'profile qualifications are immutable'); END`,
		`CREATE TRIGGER IF NOT EXISTS profile_qualifications_no_delete BEFORE DELETE ON profile_qualifications BEGIN SELECT RAISE(ABORT, 'profile qualifications are append-only'); END`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillSourceGroups(db *gorm.DB) error {
	var records []models.AssetRecord
	if err := db.Where("status = ? AND missing = ?", "unprocessed", false).Find(&records).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			parts := strings.Split(filepath.ToSlash(filepath.Clean(record.RelativePath)), "/")
			if len(parts) < 2 || parts[0] == "." || strings.TrimSpace(parts[0]) == "" {
				continue
			}
			root := filepath.Clean(record.RootPath)
			group := models.SourceGroup{Name: parts[0], RelativePath: parts[0], SourcePath: filepath.Join(root, filepath.FromSlash(parts[0])), Enabled: true}
			if err := tx.Where("relative_path = ?", group.RelativePath).FirstOrCreate(&group).Error; err != nil {
				return err
			}
			logicalParts := parts[:1]
			if len(parts) > 2 {
				logicalParts = parts[:2]
			}
			updates := map[string]interface{}{
				"source_group_id":    group.ID,
				"logical_group_path": filepath.Join(root, filepath.FromSlash(strings.Join(logicalParts, "/"))),
				"source_path":        filepath.Clean(record.Path),
			}
			if err := tx.Model(&models.AssetRecord{}).Where("id = ?", record.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func backfillQueuePositions(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Where("queue_position = 0").Order("priority asc, created_at asc, id asc").Find(&jobs).Error; err != nil {
		return err
	}
	var highest int64
	if err := db.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for index := range jobs {
			highest++
			if err := tx.Model(&jobs[index]).Update("queue_position", highest).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func backfillProfileScopes(db *gorm.DB) error {
	var profiles []models.Profile
	if err := db.Where("scope = '' OR scope IS NULL").Find(&profiles).Error; err != nil {
		return err
	}
	for index := range profiles {
		if err := db.Model(&profiles[index]).Update("scope", "asset").Error; err != nil {
			return err
		}
	}
	return nil
}

func stringFromJSON(value any) string {
	text, _ := value.(string)
	return text
}

// trackProfilePathAssignments predates first-class profile assignments. Move
// those bindings into the relational assignment table and mark the referenced
// profiles as path profiles so existing installations keep their behavior.
func backfillLegacyTrackProfileAssignments(db *gorm.DB) error {
	var assignmentSetting models.AppSetting
	if err := db.First(&assignmentSetting, "key = ?", "trackProfilePathAssignments").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	assignments := jsonMapFromAny(assignmentSetting.Value["assignments"])
	if len(assignments) == 0 {
		return nil
	}

	keys := map[string]bool{}
	for targetPath, rawKey := range assignments {
		profileKey := strings.TrimSpace(stringFromJSON(rawKey))
		targetPath = filepath.Clean(strings.TrimSpace(targetPath))
		if profileKey == "" || targetPath == "." {
			continue
		}
		keys[profileKey] = true
		assignment := models.ProfileAssignment{
			TargetType: "path", TargetPath: targetPath, MediaType: "tracks",
			Selection: "profile", ProfileKey: profileKey,
		}
		if err := db.Where("target_type = ? AND target_path = ? AND media_type = ?", assignment.TargetType, assignment.TargetPath, assignment.MediaType).
			FirstOrCreate(&assignment).Error; err != nil {
			return err
		}
	}

	var profilesSetting models.AppSetting
	if err := db.First(&profilesSetting, "key = ?", "trackProfiles").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	var profiles []any
	switch values := profilesSetting.Value["profiles"].(type) {
	case []any:
		profiles = values
	case models.JSONList:
		profiles = []any(values)
	default:
		return nil
	}
	changed := false
	for _, raw := range profiles {
		profile := jsonMapFromAny(raw)
		if !keys[strings.TrimSpace(stringFromJSON(profile["key"]))] || stringFromJSON(profile["scope"]) == "path" {
			continue
		}
		profile["scope"] = "path"
		changed = true
	}
	if changed {
		if err := db.Model(&profilesSetting).Update("value", profilesSetting.Value).Error; err != nil {
			return err
		}
	}
	return db.Delete(&assignmentSetting).Error
}

func jsonMapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case models.JSONMap:
		return map[string]any(typed)
	default:
		return nil
	}
}

func backfillQueueExecutionNumbers(db *gorm.DB) error {
	// IDs were historically exposed as job numbers. Preserve those numbers for
	// work that actually started, while leaving never-executed queue entries as
	// numberless placeholders.
	return db.Model(&models.QueueJob{}).
		Where("execution_number IS NULL").
		Where("started_at IS NOT NULL OR status IN ?", []string{"running", "completed", "failed"}).
		Update("execution_number", gorm.Expr("id")).Error
}

func migrateRuntimePolicy(db *gorm.DB) error {
	var setting models.AppSetting
	result := db.Where("key = ?", "runtimePolicy").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 || intFromJSON(setting.Value["schemaVersion"]) >= 2 {
		return result.Error
	}
	mode, _ := setting.Value["mode"].(string)
	selected, _ := setting.Value["selectedProfile"].(string)
	if _, ok := runtimeinfo.RuntimeProfile(selected); !ok {
		selected = "desktop_balanced"
	}
	preferred := "auto"
	if mode == "manual" {
		preferred = selected
	}
	config := runtimeinfo.RuntimePolicyConfig{SchemaVersion: 2, Mode: mode, PreferredProfile: preferred, FallbackProfile: "desktop_safe", Overrides: map[string]runtimeinfo.RuntimeProfileOverride{}}
	if fallback, ok := setting.Value["fallbackProfile"].(string); ok && runtimeinfo.ValidRuntimeProfile(fallback) && fallback != "auto" {
		config.FallbackProfile = fallback
	}
	override := runtimeinfo.RuntimeProfileOverride{}
	if value, exists := setting.Value["pauseWhenOnBattery"]; exists {
		parsed := boolFromJSON(value)
		override.PauseWhenOnBattery = &parsed
	}
	if value, exists := setting.Value["preventSleepDuringJobs"]; exists {
		parsed := boolFromJSON(value)
		override.PreventSleepDuringJobs = &parsed
	}
	var limits models.AppSetting
	if db.Where("key = ?", "schedulerLimits").Limit(1).Find(&limits).RowsAffected > 0 && !boolFromJSON(limits.Value["useProfileDefaults"]) {
		override.MaxRunningJobs = intPointer(limits.Value["maxRunningJobs"])
		override.MaxVideoJobs = intPointer(limits.Value["maxVideoJobs"])
		override.MaxSoftwareX265Jobs = intPointer(limits.Value["maxSoftwareX265Jobs"])
		override.MaxHardwareEncodeJobs = intPointer(limits.Value["maxHardwareEncodeJobs"])
		override.MaxAudioJobs = intPointer(limits.Value["maxAudioJobs"])
		override.MaxLabJobs = intPointer(limits.Value["maxLabJobs"])
		override.MinFreeRAMGB = int64Pointer(limits.Value["minFreeRamGb"])
		override.MinFreeWorkGB = int64Pointer(limits.Value["minFreeWorkGb"])
		override.MinFreeLibraryGB = int64Pointer(limits.Value["minFreeLibraryGb"])
		override.MaxWorkspaceGB = int64Pointer(limits.Value["maxWorkspaceGb"])
		if value, exists := limits.Value["allowDirectMode"]; exists {
			parsed := boolFromJSON(value)
			override.AllowDirectMode = &parsed
		}
	}
	config.Overrides[selected] = override
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	var value models.JSONMap
	if err := json.Unmarshal(encoded, &value); err != nil {
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func intFromJSON(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}
func boolFromJSON(value any) bool { parsed, _ := value.(bool); return parsed }
func intPointer(value any) *int {
	parsed := intFromJSON(value)
	if parsed <= 0 {
		return nil
	}
	return &parsed
}
func int64Pointer(value any) *int64 {
	parsed := int64(intFromJSON(value))
	if parsed <= 0 {
		return nil
	}
	return &parsed
}

func backfillQueueLifecycleStages(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Find(&jobs).Error; err != nil {
		return err
	}
	for i := range jobs {
		job := &jobs[i]
		stage := "queued"
		if job.PublishedAt != nil {
			stage = "completed"
		} else {
			switch job.Status {
			case "running":
				stage = "converting"
			case "completed":
				stage = "ready_to_publish"
			case "failed":
				stage = "failed"
			case "canceled":
				stage = "canceled"
			}
		}
		if job.Stage == stage {
			continue
		}
		now := time.Now()
		if err := db.Model(job).Updates(map[string]any{"stage": stage, "stage_updated_at": &now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillPendingExecutionPlans(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Where("active_execution_plan_id IS NULL").Find(&jobs).Error; err != nil {
		return err
	}
	for i := range jobs {
		job := &jobs[i]
		if len(job.ProfileSnapshot) == 0 {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, err := scheduler.CreatePendingExecutionPlan(tx, job, "Execution plan created from migration backfill")
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func backfillQueueProfileSnapshots(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Where("profile_snapshot IS NULL OR profile_snapshot = ? OR profile_version <= 0", "{}").Find(&jobs).Error; err != nil {
		return err
	}
	for i := range jobs {
		job := &jobs[i]
		var profile models.Profile
		// Historical jobs may point to a soft-deleted profile. Recover that
		// contract when possible; Find avoids logging an expected not-found as
		// a database error when the profile was permanently removed.
		result := db.Unscoped().Where("id = ?", job.ProfileID).Limit(1).Find(&profile)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		now := time.Now()
		snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, "migration_backfill")
		if err != nil {
			return err
		}
		job.ProfileVersion = max(profile.ProfileVersion, 1)
		job.ProfileSnapshot = snapshot
		job.ProfileCapturedAt = &now
		if err := db.Save(job).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeProfileContracts(db *gorm.DB) error {
	var profiles []models.Profile
	if err := db.Find(&profiles).Error; err != nil {
		return err
	}
	for i := range profiles {
		profile := &profiles[i]
		presetChanged := migrateHardwareQualityPresetScale(profile)
		if profile.CodecFamily != "" && profile.EncoderPolicy != "" && len(profile.AllowedEncoders) > 0 {
			if presetChanged {
				if err := db.Save(profile).Error; err != nil {
					return err
				}
			}
			continue
		}
		if err := scheduler.ApplyAuthoritativeContract(profile); err != nil {
			return err
		}
		if err := db.Save(profile).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateHardwareQualityPresetScale(profile *models.Profile) bool {
	if profile.WorkerConfig == nil || intFromJSON(profile.WorkerConfig["hardwareQualityPresetScale"]) >= 2 {
		return false
	}
	presetValue, _ := profile.WorkerConfig["hardwareQualityPreset"].(string)
	preset := strings.ToLower(strings.TrimSpace(presetValue))
	if preset == "" || preset == "custom" {
		return false
	}
	mapped := map[string]string{
		"compact": "recommended", "medium": "best_quality", "recommended": "high_quality",
		"best_quality": "archive", "high_quality": "master",
	}[preset]
	if mapped == "" {
		mapped = "custom"
	}
	profile.WorkerConfig["hardwareQualityPreset"] = mapped
	profile.WorkerConfig["hardwareQualityPresetScale"] = 2
	profile.ProfileVersion = max(profile.ProfileVersion+1, 2)
	return true
}

func Seed(db *gorm.DB) error {
	if err := seedSettings(db); err != nil {
		return err
	}
	if err := seedStorageRolesFromPaths(db); err != nil {
		return err
	}
	return normalizeProfileContracts(db)
}

func seedSettings(db *gorm.DB) error {
	defaults := []models.AppSetting{
		{
			Key: "paths",
			Value: models.JSONMap{
				"rawRoot":              "/media/raw",
				"libraryRoot":          "/media/library",
				"stagingPath":          "/media/staging",
				"originalsArchivePath": "/media/originals_archive",
				"asIsReportsPath":      "/media/reports/as-is",
				"resultsReportsPath":   "/media/reports/results",
				"logsPath":             "/media/reports/logs",
			},
		},
		{
			Key: "workers",
			Value: models.JSONMap{
				"defaultWorkerName":       "local-worker",
				"autoWorkerEnabled":       true,
				"maxConcurrentJobs":       1,
				"maxJobsPerBatch":         10,
				"delaySecondsBetweenJobs": 30,
				"batchCooldownSeconds":    600,
				"dryRunOnly":              true,
			},
		},
		{
			Key: "assetDefaults",
			Value: models.JSONMap{
				"schemaVersion": 1,
				"video": models.JSONMap{
					"source": "custom", "custom": models.JSONMap{"videoCodec": "copy"}, "removeNonPrimaryVideoTracks": false,
				},
				"audio": models.JSONMap{
					"source": "custom",
					"custom": models.JSONMap{
						"addCompatibleTrack": false, "compatibleTrackQuality": 192,
						"compatibleTrackChannels": 2, "compatibleTrackSource": "resolved",
						"preferredDefaultLanguage": "",
					},
				},
				"tracks": models.JSONMap{
					"source": "custom",
					"custom": models.JSONMap{
						"trackDispositionVersion": 1, "audioMode": "all", "subtitleMode": "all",
						"subtitleDisposition": "keep", "subtitleSidecarFormats": models.JSONList{"srt"},
						"attachmentPolicy": "auto", "fontAttachmentExportPolicy": "none", "chapterPolicy": "keep", "ocrMode": "accurate",
						"defaultAudioLanguage": "", "defaultSubtitleLanguage": "",
					},
				},
			},
		},
		{
			Key: "pipelineAutomation",
			Value: models.JSONMap{
				"autoAnalysisEnabled":               false,
				"reviewMode":                        "conditional",
				"autoExecutionEnabled":              true,
				"autoValidationEnabled":             false,
				"autoPublisherEnabled":              false,
				"publisherOverwriteEnabled":         false,
				"publishedJobReconciliationEnabled": false,
			},
		},
		{
			Key: "runtimePolicy",
			Value: models.JSONMap{
				"schemaVersion":    2,
				"mode":             "automatic",
				"preferredProfile": "auto",
				"fallbackProfile":  "desktop_safe",
				"overrides":        models.JSONMap{},
			},
		},
		{
			Key:   airesource.SettingsKey,
			Value: airesource.DefaultSettingsValue(),
		},
		{
			Key: "workingHours",
			Value: models.JSONMap{"enabled": false, "timezone": "America/Mexico_City", "preset": "disabled", "windows": []models.JSONMap{}, "outsideWindowPolicy": models.JSONMap{
				"startNewHeavyJobs": true, "continueRunningJobs": true, "allowAnalysisJobs": true, "allowValidationJobs": true,
				"allowPublisherJobs": true, "allowCleanupJobs": true, "allowLabPreviews": true,
			}},
		},
		{
			Key:   "workspace",
			Value: models.JSONMap{"preferredMode": "copy_to_work_disk", "fallbackMode": "wait", "allowDirectMode": false, "estimateRequiredSpace": true},
		},
		{
			Key: "directPlay",
			Value: models.JSONMap{
				"enabled": true, "strategy": "balanced", "targetClients": []string{"jellyfin_web", "jellyfin_android_tv", "jellyfin_roku", "jellyfin_webos", "apple_tv"},
				"minimumScore": 70, "enforcement": "warn",
			},
		},
		{Key: "frameStructureSampling", Value: models.JSONMap{"adaptive": true, "windows": 5, "windowSeconds": 20, "positions": []float64{0.08, 0.27, 0.50, 0.73, 0.92}}},
		{
			Key:   "housekeeping",
			Value: models.JSONMap{"autoEnabled": true, "intervalHours": 24, "failedRetentionDays": 7, "canceledRetentionDays": 3, "orphanRetentionDays": 7},
		},
		{
			Key: "validation",
			Value: models.JSONMap{
				"minimumScore":         90,
				"requireDurationMatch": true,
			},
		},
		{
			Key: "cancellationPolicy",
			Value: models.JSONMap{
				"keepLogsAndDiagnostics":         true,
				"deleteGeneratedFiles":           false,
				"deletePartialOutputFromStaging": false,
				"controlledRoots": []string{
					"/media/staging",
					"/mwp/work",
					"/mwp/work/temp",
					"/tmp/mvforge",
				},
			},
		},
		{
			Key: "originalRetentionPolicy",
			Value: models.JSONMap{
				"keepOriginalsDays":      30,
				"autoDeleteEnabled":      false,
				"processedOriginalsPath": "/media/originals_archive/processed-originals",
			},
		},
		{
			Key: "assetInventory",
			Value: models.JSONMap{
				"autoSyncEnabled":     true,
				"syncIntervalMinutes": 60,
				"reconciliationMode":  "exact",
			},
		},
		{
			Key: "audioEnhancementProfiles",
			Value: models.JSONMap{
				"profiles": []models.JSONMap{
					{
						"key":                   "gentle-normalize",
						"name":                  "Gentle Normalize",
						"description":           "Safely evens out quiet or inconsistent audio without changing the character too much.",
						"intent":                "Low-risk loudness normalization",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Good first pass for TV recordings, DVD rips, and uneven episode batches.",
					},
					{
						"key":                   "dialogue-clarity",
						"name":                  "Dialogue Clarity",
						"description":           "Adds gentle voice presence and compression for dialogue-heavy older sources.",
						"intent":                "Make voices easier to understand",
						"filters":               "highpass=f=80,equalizer=f=1800:t=q:w=1.1:g=2.5,acompressor=threshold=-20dB:ratio=2.2:attack=20:release=250,loudnorm=I=-18:TP=-2:LRA=9",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Useful for old anime dubs, TV captures, and sources where voices sit behind music/effects.",
					},
					{
						"key":                   "old-source-cleanup",
						"name":                  "Old Source Cleanup",
						"description":           "Reduces light hiss/noise, trims rumble, and normalizes loudness for older TV/anime audio.",
						"intent":                "Cleanup for aged stereo or mono sources",
						"filters":               "highpass=f=70,lowpass=f=15000,afftdn=nf=-25,acompressor=threshold=-22dB:ratio=2:attack=25:release=300,loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "More aggressive. Preview before applying because denoise filters can create artifacts.",
					},
					{
						"key":                   "speech-neural-denoise",
						"name":                  "Speech Neural Denoise",
						"description":           "Uses FFmpeg arnndn with an external speech denoise model, then normalizes loudness.",
						"intent":                "Reduce speech noise with a neural model",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "/mvforge/models/audio/rnnoise-model.rnnn",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Requires a valid arnndn/RNNoise model path mounted inside the backend container. Test before batch use.",
					},
					{
						"key":                   "hiss-reduction-light",
						"name":                  "Hiss Reduction Light",
						"description":           "Gently reduces high-frequency hiss while keeping ambience and music mostly intact.",
						"intent":                "Light hiss cleanup",
						"filters":               "highpass=f=55,lowpass=f=17000,afftdn=nf=-20,loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Start here for music, concerts, and sources where noise reduction should be subtle.",
					},
					{
						"key":                   "hiss-reduction-medium",
						"name":                  "Hiss Reduction Medium",
						"description":           "Moderate hiss reduction for TV, tape, and older stereo or mono sources.",
						"intent":                "Balanced hiss cleanup",
						"filters":               "highpass=f=65,lowpass=f=15500,afftdn=nf=-24,acompressor=threshold=-22dB:ratio=1.8:attack=25:release=250,loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Good default for old TV/anime audio. Preview for watery artifacts before batch use.",
					},
					{
						"key":                   "hiss-reduction-strong",
						"name":                  "Hiss Reduction Strong",
						"description":           "Aggressive hiss cleanup for noisy spoken material where artifacts are acceptable.",
						"intent":                "Strong hiss cleanup",
						"filters":               "highpass=f=80,lowpass=f=13500,afftdn=nf=-32,acompressor=threshold=-24dB:ratio=2.4:attack=20:release=300,loudnorm=I=-18:TP=-2:LRA=9",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Use for dialogue-first sources. Strong denoise can damage music, ambience, and cymbals.",
					},
					{
						"key":                   "old-mono-cleanup",
						"name":                  "Old Mono Cleanup",
						"description":           "Centers old mono-style sources, reduces rumble/hiss, and normalizes loudness.",
						"intent":                "Mono restoration",
						"filters":               "highpass=f=80,lowpass=f=14500,afftdn=nf=-24,acompressor=threshold=-22dB:ratio=2:attack=25:release=280,loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "",
						"channelMode":           "dual-mono",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Best for old mono dubs, VHS/DVD extras, and sources that should play evenly on both speakers.",
					},
					{
						"key":                   "old-anime-dialogue-cleanup",
						"name":                  "Old Anime Dialogue Cleanup",
						"description":           "Raises dialogue presence while gently reducing hiss and low-end rumble.",
						"intent":                "Old anime and TV dialogue",
						"filters":               "highpass=f=75,lowpass=f=15000,afftdn=nf=-23,equalizer=f=1800:t=q:w=1.1:g=2,acompressor=threshold=-21dB:ratio=2.1:attack=18:release=260,loudnorm=I=-18:TP=-2:LRA=9",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Designed for old anime/TV dubs where voices need help but music should not be crushed.",
					},
					{
						"key":                   "concert-pcm-preserve",
						"name":                  "Concert PCM Preserve",
						"description":           "Keeps concert dynamics mostly intact with only light cleanup and safe loudness.",
						"intent":                "Concert and PCM preservation",
						"filters":               "highpass=f=40,lowpass=f=19000,loudnorm=I=-19:TP=-2:LRA=14",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "flac",
						"targetLoudness":        -19,
						"truePeak":              -2,
						"notes":                 "Good for concerts and PCM tracks. Avoid heavy denoise unless a sample proves it is needed.",
					},
					{
						"key":                   "mono-dual-mono-safe",
						"name":                  "Mono to Dual Mono Safe",
						"description":           "Duplicates a mono source into left and right channels without fake stereo widening.",
						"intent":                "Safe mono compatibility",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "dual-mono",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Use for mono tracks that should play evenly on stereo devices. This does not create true stereo.",
					},
					{
						"key":                   "downmix-mono-safe",
						"name":                  "Downmix to Mono Safe",
						"description":           "Mixes stereo or multichannel audio into one centered mono track.",
						"intent":                "Mono compatibility",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "downmix-mono",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Useful when stereo image or phase problems make old sources clearer as a centered mono track.",
					},
					{
						"key":                   "mono-light-stereo-experimental",
						"name":                  "Mono to Light Stereo Experimental",
						"description":           "Creates a subtle pseudo-stereo image from mono using a small delay and widening.",
						"intent":                "Experimental mono widening",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "light-stereo",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Preview carefully. Pseudo-stereo can make dialogue or old dubs sound phasey if pushed too far.",
					},
				},
			},
		},
		{
			Key: "assetTypes",
			Value: models.JSONMap{
				"types": []models.JSONMap{
					{
						"key":        "movies",
						"label":      "Movies",
						"extensions": []string{".mkv", ".mp4", ".avi", ".mov"},
					},
					{
						"key":        "tv",
						"label":      "TV Shows",
						"extensions": []string{".mkv", ".mp4"},
					},
					{
						"key":        "anime",
						"label":      "Anime",
						"extensions": []string{".mkv", ".mp4"},
					},
					{
						"key":        "music-videos",
						"label":      "Music Videos",
						"extensions": []string{".mkv", ".mp4", ".mov"},
					},
					{
						"key":        "concerts",
						"label":      "Concerts",
						"extensions": []string{".mkv", ".mp4"},
					},
					{
						"key":        "home-videos",
						"label":      "Home Videos",
						"extensions": []string{".mp4", ".mov"},
					},
				},
			},
		},
		{
			Key: "assetCategories",
			Value: models.JSONMap{
				"categories": []string{"movie", "anime", "series", "episode", "season", "extras", "special", "concert", "music-video", "documentary"},
			},
		},
	}

	for _, setting := range defaults {
		if err := db.FirstOrCreate(&setting, models.AppSetting{Key: setting.Key}).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedStorageRolesFromPaths(db *gorm.DB) error {
	var existing int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "storageRoles").Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var paths models.AppSetting
	if err := db.Where("key = ?", "paths").First(&paths).Error; err != nil {
		return err
	}
	pathValue := func(key, fallback string) string {
		if value, ok := paths.Value[key].(string); ok && value != "" {
			return value
		}
		return fallback
	}
	reports := pathValue("resultsReportsPath", "/media/reports")
	roles := models.JSONMap{
		"raw": models.JSONMap{"path": pathValue("rawRoot", "/media/raw")}, "library": models.JSONMap{"path": pathValue("libraryRoot", "/media/library")},
		"originals_archive": models.JSONMap{"path": pathValue("originalsArchivePath", "/media/originals_archive")}, "work": models.JSONMap{"path": pathValue("stagingPath", "/media/staging")},
		"cache": models.JSONMap{"path": filepath.Join(filepath.Dir(reports), "cache")}, "reports": models.JSONMap{"path": reports}, "logs": models.JSONMap{"path": pathValue("logsPath", "/media/reports/logs")},
	}
	return db.Create(&models.AppSetting{Key: "storageRoles", Value: roles}).Error
}
