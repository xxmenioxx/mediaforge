package database

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/profiletemplates"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type legacySeededProfile struct {
	ID                 uint   `gorm:"primaryKey"`
	Name               string `gorm:"not null;uniqueIndex"`
	Scope              string `gorm:"not null;default:'';index"`
	Description        string
	Container          string            `gorm:"not null"`
	VideoCodec         string            `gorm:"not null"`
	CodecFamily        string            `gorm:"not null;default:''"`
	EncoderPolicy      string            `gorm:"not null;default:''"`
	PreferredEncoder   string            `gorm:"not null;default:''"`
	AllowedEncoders    models.StringList `gorm:"type:json"`
	FallbackPolicy     string            `gorm:"not null;default:''"`
	BitDepth           int               `gorm:"not null;default:0"`
	PixelFormat        string            `gorm:"not null;default:''"`
	QualityStrategy    string            `gorm:"not null;default:''"`
	OptimizationIntent string            `gorm:"not null;default:''"`
	ProfileVersion     int               `gorm:"not null;default:1"`
	AudioCodec         string            `gorm:"not null"`
	QualityMode        string            `gorm:"not null"`
	QualityValue       int               `gorm:"not null"`
	PreserveHDR        bool
	PreserveSubtitles  bool
	PreserveChapters   bool
	WorkerConfig       models.JSONMap `gorm:"type:json"`
	Disabled           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (legacySeededProfile) TableName() string { return "profiles" }

func TestMigrateSeededProfilesPreservesAssignmentsAndQueueSnapshots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy-profiles.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacySeededProfile{}, &models.ProfileAssignment{}, &models.QueueJob{}); err != nil {
		t.Fatal(err)
	}

	var assignedID uint
	expectedProfiles := make(map[uint]models.Profile, 7)
	legacyCount := 0
	for _, template := range profiletemplates.All() {
		if template.LegacySeedPreset == "" {
			continue
		}
		legacyCount++
		profile := template.Definition.NewProfile(template.Name, "asset", template.Description)
		profile.ProfileVersion = 7
		if err := scheduler.ApplyAuthoritativeContract(&profile); err != nil {
			t.Fatalf("normalize legacy fixture %q: %v", template.Key, err)
		}
		legacy := legacySeededProfile{
			Name: profile.Name, Scope: profile.Scope, Description: profile.Description,
			Container: profile.Container, VideoCodec: profile.VideoCodec, CodecFamily: profile.CodecFamily,
			EncoderPolicy: profile.EncoderPolicy, PreferredEncoder: profile.PreferredEncoder,
			AllowedEncoders: profile.AllowedEncoders, FallbackPolicy: profile.FallbackPolicy,
			BitDepth: profile.BitDepth, PixelFormat: profile.PixelFormat, QualityStrategy: profile.QualityStrategy,
			OptimizationIntent: profile.OptimizationIntent, ProfileVersion: profile.ProfileVersion,
			AudioCodec: profile.AudioCodec, QualityMode: profile.QualityMode, QualityValue: profile.QualityValue,
			PreserveHDR: profile.PreserveHDR, PreserveSubtitles: profile.PreserveSubtitles,
			PreserveChapters: profile.PreserveChapters, WorkerConfig: profile.WorkerConfig,
		}
		if err := db.Create(&legacy).Error; err != nil {
			t.Fatalf("create legacy fixture %q: %v", template.Key, err)
		}
		profile.ID = legacy.ID
		expectedProfiles[legacy.ID] = profile
		if template.Key == "hevc-small-size" {
			assignedID = legacy.ID
			assignment := models.ProfileAssignment{TargetType: "asset", TargetPath: "/raw/movie.mkv", MediaType: "video", Selection: "profile", VideoProfileID: legacy.ID}
			if err := db.Create(&assignment).Error; err != nil {
				t.Fatal(err)
			}
			capturedAt := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
			snapshot, err := scheduler.CaptureProfileSnapshot(profile, capturedAt, "migration-test")
			if err != nil {
				t.Fatal(err)
			}
			job := models.QueueJob{
				MediaPath: "/raw/movie.mkv", LibraryID: 1, ProfileID: legacy.ID, ProfileVersion: 7,
				Status: "completed", Stage: "ready_to_publish", ProfileCapturedAt: &capturedAt,
				ProfileSnapshot: snapshot,
			}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if legacyCount != 7 || assignedID == 0 {
		t.Fatalf("legacy seeded fixture count=%d assigned profile=%d", legacyCount, assignedID)
	}

	var assignmentBefore models.ProfileAssignment
	if err := db.Where("video_profile_id = ?", assignedID).First(&assignmentBefore).Error; err != nil {
		t.Fatal(err)
	}
	var jobBefore models.QueueJob
	if err := db.Where("profile_id = ?", assignedID).First(&jobBefore).Error; err != nil {
		t.Fatal(err)
	}
	snapshotData, err := json.Marshal(jobBefore.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotBefore models.JSONMap
	if err := json.Unmarshal(snapshotData, &snapshotBefore); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(db); err != nil {
		var stored []models.Profile
		_ = db.Find(&stored).Error
		for _, profile := range stored {
			if _, resolveErr := scheduler.ResolveExecutionConstraints(profile); resolveErr != nil {
				t.Fatalf("Migrate: %v; profile %q codec=%q family=%q encoder=%q allowed=%v", err, profile.Name, profile.VideoCodec, profile.CodecFamily, profile.PreferredEncoder, profile.AllowedEncoders)
			}
		}
		t.Fatal(err)
	}

	var profiles []models.Profile
	if err := db.Order("id asc").Find(&profiles).Error; err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 7 {
		t.Fatalf("migrated profile count=%d, want 7", len(profiles))
	}
	for _, profile := range profiles {
		template, ok := profiletemplates.Get(profile.TemplateKey)
		if !ok || profile.TemplateVersion != template.Version {
			t.Errorf("profile %d has no matching template provenance: %#v", profile.ID, profile)
		}
		if profile.TemplateKey != stringConfigValue(profile.WorkerConfig["preset"]) {
			t.Errorf("profile %d was associated with the wrong seed template: preset=%v key=%q", profile.ID, profile.WorkerConfig["preset"], profile.TemplateKey)
		}
		if profile.ProfileVersion != 7 {
			t.Errorf("profile %d version changed during migration: %d", profile.ID, profile.ProfileVersion)
		}
		expected, found := expectedProfiles[profile.ID]
		if !found || definitionJSON(t, profile.Definition()) != definitionJSON(t, expected.Definition()) {
			t.Errorf("profile %d definition changed during migration", profile.ID)
		}
	}

	var assignmentAfter models.ProfileAssignment
	if err := db.Where("id = ?", assignmentBefore.ID).First(&assignmentAfter).Error; err != nil {
		t.Fatal(err)
	}
	if assignmentAfter != assignmentBefore || assignmentAfter.VideoProfileID != assignedID {
		t.Fatalf("assignment changed during migration: before=%#v after=%#v", assignmentBefore, assignmentAfter)
	}
	var jobAfter models.QueueJob
	if err := db.Where("id = ?", jobBefore.ID).First(&jobAfter).Error; err != nil {
		t.Fatal(err)
	}
	if jobAfter.ProfileID != jobBefore.ProfileID || jobAfter.ProfileVersion != jobBefore.ProfileVersion || !reflect.DeepEqual(jobAfter.ProfileSnapshot, snapshotBefore) {
		t.Fatalf("Queue snapshot or profile binding changed: before=%#v after=%#v", jobBefore, jobAfter)
	}
}

func stringConfigValue(value any) string {
	text, _ := value.(string)
	return text
}

func definitionJSON(t *testing.T, definition models.ProfileDefinition) string {
	t.Helper()
	data, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSeedDoesNotPersistBuiltInProfileTemplates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "new-profiles.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := Seed(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.Profile{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("new installation seeded %d assignable profiles", count)
	}
}
