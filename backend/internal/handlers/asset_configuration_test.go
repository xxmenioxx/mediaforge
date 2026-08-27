package handlers

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func assetConfigurationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "configuration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.SourceGroup{}, &models.AssetRecord{}, &models.Profile{}, &models.ProfileAssignment{}, &models.AssetScopeConfiguration{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEffectiveAssetConfigurationResolvesAllScopesAndDisabled(t *testing.T) {
	db := assetConfigurationTestDB(t)
	group := models.SourceGroup{Name: "Anime", RelativePath: "anime", SourcePath: "/media/raw/anime", Enabled: true}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	assetPath := "/media/raw/anime/Dragon Ball GT/Season1/episode01.mkv"
	record := models.AssetRecord{
		Path: assetPath, RootPath: "/media/raw", RelativePath: "anime/Dragon Ball GT/Season1/episode01.mkv",
		GroupPath: "anime/Dragon Ball GT/Season1", FileName: "episode01.mkv", Status: "unprocessed",
		SourceGroupID: group.ID, LogicalGroupPath: "/media/raw/anime/Dragon Ball GT", SourcePath: assetPath,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	assignments := []models.ProfileAssignment{
		{TargetType: assetScopeSourceGroup, TargetPath: group.SourcePath, MediaType: "video", Selection: "profile", VideoProfileID: 11},
		{TargetType: assetScopePath, TargetPath: filepath.Dir(assetPath), MediaType: "video", Selection: configSelectionDisabled},
		{TargetType: assetScopeLogicalGroup, TargetPath: record.LogicalGroupPath, MediaType: "audio", Selection: "profile", ProfileKey: "dialogue"},
	}
	if err := db.Create(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Anime", DestinationPath: "/media/library/anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := upsertAssetScopeConfiguration(db, AssetScopeConfigurationInput{
		ScopeType: assetScopeSourceGroup, ScopeKey: group.SourcePath,
		CategorySelection: configSelectionValue, Category: "anime",
		DestinationSelection: configSelectionValue, DestinationLibraryID: library.ID,
	}); err != nil {
		t.Fatal(err)
	}

	effective, err := effectiveAssetConfiguration(db, assetPath)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Video.Selection != configSelectionDisabled || effective.Video.Source != assetScopePath {
		t.Fatalf("expected path disabled video to override source group, got %#v", effective.Video)
	}
	if effective.Audio.ProfileKey != "dialogue" || effective.Audio.Source != assetScopeLogicalGroup {
		t.Fatalf("unexpected audio resolution: %#v", effective.Audio)
	}
	if effective.Category.Category != "anime" || effective.Destination.DestinationLibraryID != library.ID {
		t.Fatalf("unexpected non-profile resolution: %#v %#v", effective.Category, effective.Destination)
	}
}

func TestSourceGroupHierarchyUsesBackendIdentity(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	assetPath := filepath.Join(rawRoot, "anime", "Beck Mongolian Chop Squad", "Season1", "episode01.mkv")
	records := []models.AssetRecord{{Path: assetPath, RootPath: rawRoot, RelativePath: filepath.ToSlash("anime/Beck Mongolian Chop Squad/Season1/episode01.mkv"), FileName: "episode01.mkv", Status: "unprocessed", SizeBytes: 10}}
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if records[0].SourceGroupID == 0 {
		t.Fatal("source group was not assigned")
	}
	if got := filepath.Base(records[0].LogicalGroupPath); got != "Beck Mongolian Chop Squad" {
		t.Fatalf("unexpected logical group %q", got)
	}
	hierarchy := buildAssetSourceGroups(db, records, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{}})
	if len(hierarchy) != 1 || len(hierarchy[0].LogicalGroups) != 1 || len(hierarchy[0].LogicalGroups[0].AssetPaths) != 1 {
		t.Fatalf("unexpected hierarchy: %#v", hierarchy)
	}
	if hierarchy[0].LogicalGroups[0].AssetPaths[0].Name != "Season1" {
		t.Fatalf("unexpected actual path: %#v", hierarchy[0].LogicalGroups[0].AssetPaths[0])
	}
}
