package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
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
	if err := db.Model(&record).Update("status", "converted").Error; err != nil {
		t.Fatal(err)
	}
	effective, err = effectiveAssetConfiguration(db, assetPath)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Audio.Source != "" || effective.Category.Source != "" || effective.Destination.Source != "" {
		t.Fatalf("source provenance activated inheritance for converted asset: %#v", effective)
	}
}

func TestEffectiveAssetConfigurationBatchResolvesManyAssetsInOneRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := assetConfigurationTestDB(t)
	group := models.SourceGroup{Name: "Movies", RelativePath: "movies", SourcePath: "/media/raw/movies", Enabled: true}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	records := []models.AssetRecord{
		{Path: "/media/raw/movies/Akira/Akira.mkv", RootPath: "/media/raw", GroupPath: "movies/Akira", FileName: "Akira.mkv", Status: "unprocessed", SourceGroupID: group.ID, LogicalGroupPath: "/media/raw/movies/Akira", SourcePath: "/media/raw/movies/Akira/Akira.mkv"},
		{Path: "/media/raw/movies/Perfect Blue/Perfect Blue.mkv", RootPath: "/media/raw", GroupPath: "movies/Perfect Blue", FileName: "Perfect Blue.mkv", Status: "unprocessed", SourceGroupID: group.ID, LogicalGroupPath: "/media/raw/movies/Perfect Blue", SourcePath: "/media/raw/movies/Perfect Blue/Perfect Blue.mkv"},
		{Path: "/media/raw/movies/Your Name/Your Name.mkv", RootPath: "/media/raw", GroupPath: "movies/Your Name", FileName: "Your Name.mkv", Status: "unprocessed", SourceGroupID: group.ID, LogicalGroupPath: "/media/raw/movies/Your Name", SourcePath: "/media/raw/movies/Your Name/Your Name.mkv"},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeSourceGroup, TargetPath: group.SourcePath, MediaType: "video", Selection: "profile", VideoProfileID: 9}).Error; err != nil {
		t.Fatal(err)
	}
	mixedAssignments := []models.ProfileAssignment{
		{TargetType: assetScopeLogicalGroup, TargetPath: records[0].LogicalGroupPath, MediaType: "audio", Selection: "profile", ProfileKey: "logical-audio"},
		{TargetType: assetScopePath, TargetPath: filepath.Dir(records[1].Path), MediaType: "video", Selection: "disabled"},
		{TargetType: assetScopeAsset, TargetPath: records[2].Path, MediaType: "video", Selection: "profile", VideoProfileID: 11},
	}
	if err := db.Create(&mixedAssignments).Error; err != nil {
		t.Fatal(err)
	}
	configurations := []models.AssetScopeConfiguration{
		{ScopeType: assetScopeSourceGroup, ScopeKey: group.SourcePath, DestinationSelection: configSelectionValue, DestinationLibraryID: 7},
		{ScopeType: assetScopeLogicalGroup, ScopeKey: records[0].LogicalGroupPath, CategorySelection: configSelectionValue, Category: "movie"},
		{ScopeType: assetScopeAsset, ScopeKey: records[2].Path, DestinationSelection: configSelectionDisabled},
	}
	if err := db.Create(&configurations).Error; err != nil {
		t.Fatal(err)
	}
	queryCounts := map[string]int{}
	callbackName := "test:count-effective-batch-scans"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queryCounts[tx.Statement.Table]++
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(callbackName)

	body, err := json.Marshal(EffectiveAssetConfigurationBatchInput{AssetIDs: []uint{records[0].ID, records[1].ID, records[2].ID, 999999, records[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/api/assets/effective-configurations", NewAssetConfigurationHandler(db).EffectiveBatch)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/effective-configurations", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result EffectiveAssetConfigurationBatchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Configurations) != 3 || len(result.MissingAssetIDs) != 1 || result.MissingAssetIDs[0] != 999999 {
		t.Fatalf("unexpected batch response: %#v", result)
	}
	first := result.Configurations[strconv.FormatUint(uint64(records[0].ID), 10)]
	if first.Video.VideoProfileID != 9 || first.Audio.ProfileKey != "logical-audio" || first.Category.Category != "movie" || first.Destination.DestinationLibraryID != 7 {
		t.Fatalf("mixed source/logical inheritance was not resolved: %#v", first)
	}
	second := result.Configurations[strconv.FormatUint(uint64(records[1].ID), 10)]
	if second.Video.Selection != configSelectionDisabled || second.Video.Source != assetScopePath || second.Destination.DestinationLibraryID != 7 {
		t.Fatalf("path precedence was not resolved: %#v", second)
	}
	third := result.Configurations[strconv.FormatUint(uint64(records[2].ID), 10)]
	if third.Video.VideoProfileID != 11 || third.Video.Source != assetScopeAsset || third.Destination.Selection != configSelectionDisabled || third.Destination.Source != assetScopeAsset {
		t.Fatalf("asset precedence was not resolved: %#v", third)
	}
	for _, table := range []string{"asset_records", "source_groups", "profile_assignments", "asset_scope_configurations"} {
		if queryCounts[table] != 1 {
			t.Fatalf("batch resolver queried %s %d times, want once: %#v", table, queryCounts[table], queryCounts)
		}
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
	if hierarchy[0].AssetCount != 1 || hierarchy[0].TitleCount != 1 || hierarchy[0].PathCount != 1 || hierarchy[0].TotalSizeBytes != 10 {
		t.Fatalf("unexpected source metrics: %#v", hierarchy[0])
	}
}

func TestSourceGroupReconcileUsesConfiguredRootFirstSegment(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	for _, directory := range []string{"movies", "anime-movies"} {
		if err := os.MkdirAll(filepath.Join(rawRoot, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	records := []models.AssetRecord{
		{Path: filepath.Join(rawRoot, "movies", "Los aristogatos", "movie.mkv"), RootPath: rawRoot, RelativePath: "movies/Los aristogatos/movie.mkv", FileName: "movie.mkv", Status: "unprocessed"},
		{Path: filepath.Join(rawRoot, "anime-movies", "Porco_Rosso", "movie.mkv"), RootPath: rawRoot, RelativePath: "anime-movies/Porco_Rosso/movie.mkv", FileName: "movie.mkv", Status: "unprocessed"},
	}
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	var groups []models.SourceGroup
	if err := db.Order("relative_path asc").Find(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].RelativePath != "anime-movies" || groups[1].RelativePath != "movies" {
		t.Fatalf("nested titles became source groups: %#v", groups)
	}
	if records[0].SourceGroupID != groups[1].ID || records[1].SourceGroupID != groups[0].ID {
		t.Fatalf("records were assigned to the wrong top-level groups: %#v %#v", records, groups)
	}
}

func TestSourceGroupReconcileDisablesOrphanAndPreservesConfiguration(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	phantomPath := filepath.Join(rawRoot, "library-replacements")
	phantom := models.SourceGroup{Name: "Library replacements", RelativePath: "library-replacements", SourcePath: phantomPath, Enabled: true}
	if err := db.Create(&phantom).Error; err != nil {
		t.Fatal(err)
	}
	assignment := models.ProfileAssignment{TargetType: assetScopeSourceGroup, TargetPath: phantomPath, MediaType: "video", Selection: "profile", VideoProfileID: 17}
	configuration := models.AssetScopeConfiguration{ScopeType: assetScopeSourceGroup, ScopeKey: phantomPath, CategorySelection: configSelectionValue, Category: "movie"}
	historical := models.AssetRecord{Path: filepath.Join(phantomPath, "old.mkv"), RootPath: rawRoot, RelativePath: "library-replacements/old.mkv", FileName: "old.mkv", Status: "unprocessed", Missing: true, SourceGroupID: phantom.ID}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&configuration).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&historical).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileSourceGroups(db, rawRoot, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&phantom, phantom.ID).Error; err != nil {
		t.Fatal(err)
	}
	if phantom.Enabled {
		t.Fatalf("confirmed orphan remained enabled: %#v", phantom)
	}
	var assignmentCount, configurationCount, historicalCount int64
	db.Model(&models.ProfileAssignment{}).Where("id = ?", assignment.ID).Count(&assignmentCount)
	db.Model(&models.AssetScopeConfiguration{}).Where("id = ?", configuration.ID).Count(&configurationCount)
	db.Model(&models.AssetRecord{}).Where("id = ? AND source_group_id = ?", historical.ID, phantom.ID).Count(&historicalCount)
	if assignmentCount != 1 || configurationCount != 1 || historicalCount != 1 {
		t.Fatalf("orphan reconciliation removed provenance: assignment=%d configuration=%d history=%d", assignmentCount, configurationCount, historicalCount)
	}
}

func TestSourceGroupReconcileFailureDoesNotDisableGroups(t *testing.T) {
	db := assetConfigurationTestDB(t)
	group := models.SourceGroup{Name: "Movies", RelativePath: "movies", SourcePath: "/unavailable/raw/movies", Enabled: true}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileSourceGroups(db, filepath.Join(t.TempDir(), "not-mounted"), nil); err == nil {
		t.Fatal("missing source root was treated as an authoritative empty enumeration")
	}
	if err := db.First(&group, group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !group.Enabled {
		t.Fatalf("source group was disabled after enumeration failure: %#v", group)
	}
}

func TestSourceGroupHierarchyOmitsEnabledGroupWithoutVisibleAssets(t *testing.T) {
	db := assetConfigurationTestDB(t)
	group := models.SourceGroup{Name: "Empty", RelativePath: "empty", SourcePath: "/media/raw/empty", Enabled: true}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if hierarchy := buildAssetSourceGroups(db, nil, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{}}); len(hierarchy) != 0 {
		t.Fatalf("zero-current-asset group was exposed: %#v", hierarchy)
	}
}

func TestSourceGroupReconcileIsIdempotent(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rawRoot, "movies"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := reconcileSourceGroups(db, rawRoot, nil); err != nil {
		t.Fatal(err)
	}
	var first models.SourceGroup
	if err := db.Where("relative_path = ?", "movies").First(&first).Error; err != nil {
		t.Fatal(err)
	}
	stableUpdatedAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := db.Model(&models.SourceGroup{}).Where("id = ?", first.ID).UpdateColumn("updated_at", stableUpdatedAt).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileSourceGroups(db, rawRoot, nil); err != nil {
		t.Fatal(err)
	}
	var second models.SourceGroup
	if err := db.First(&second, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !second.Enabled || !second.UpdatedAt.Equal(stableUpdatedAt) {
		t.Fatalf("unchanged group was mutated during repeated reconciliation: %#v", second)
	}
}

func TestSourceGroupReconcileReEnablesReturningGroupAndPreservesConfiguration(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	moviesPath := filepath.Join(rawRoot, "movies")
	if err := os.MkdirAll(moviesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	group := models.SourceGroup{Name: "Movies", RelativePath: "movies", SourcePath: filepath.Join(rawRoot, "old-movies"), Enabled: false}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	assignment := models.ProfileAssignment{TargetType: assetScopeSourceGroup, TargetPath: group.SourcePath, MediaType: "video", Selection: "profile", VideoProfileID: 23}
	configuration := models.AssetScopeConfiguration{ScopeType: assetScopeSourceGroup, ScopeKey: group.SourcePath, CategorySelection: configSelectionValue, Category: "movie", DestinationSelection: configSelectionValue, DestinationLibraryID: 9}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&configuration).Error; err != nil {
		t.Fatal(err)
	}

	if err := reconcileSourceGroups(db, rawRoot, nil); err != nil {
		t.Fatal(err)
	}
	var restored models.SourceGroup
	if err := db.First(&restored, group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !restored.Enabled || restored.ID != group.ID || filepath.Clean(restored.SourcePath) != moviesPath {
		t.Fatalf("returning source group was not restored in place: %#v", restored)
	}
	var groupCount, assignmentCount, configurationCount int64
	if err := db.Model(&models.SourceGroup{}).Where("relative_path = ?", "movies").Count(&groupCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ProfileAssignment{}).Where("id = ? AND target_path = ?", assignment.ID, assignment.TargetPath).Count(&assignmentCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AssetScopeConfiguration{}).Where("id = ? AND scope_key = ?", configuration.ID, configuration.ScopeKey).Count(&configurationCount).Error; err != nil {
		t.Fatal(err)
	}
	if groupCount != 1 || assignmentCount != 1 || configurationCount != 1 {
		t.Fatalf("returning group lost identity or configuration: groups=%d assignments=%d configurations=%d", groupCount, assignmentCount, configurationCount)
	}

	stableUpdatedAt := restored.UpdatedAt
	if err := reconcileSourceGroups(db, rawRoot, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&restored, group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !restored.UpdatedAt.Equal(stableUpdatedAt) {
		t.Fatalf("idempotent reconciliation mutated returning group: before=%s after=%s", stableUpdatedAt, restored.UpdatedAt)
	}
}

func TestSourceGroupReconcileFailureDoesNotEnableDisabledGroup(t *testing.T) {
	db := assetConfigurationTestDB(t)
	group := models.SourceGroup{Name: "Movies", RelativePath: "movies", SourcePath: "/unavailable/raw/movies", Enabled: false}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileSourceGroups(db, filepath.Join(t.TempDir(), "not-mounted"), nil); err == nil {
		t.Fatal("missing source root was treated as authoritative")
	}
	if err := db.First(&group, group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if group.Enabled {
		t.Fatalf("failed discovery changed disabled group lifecycle: %#v", group)
	}
}

func TestSourceGroupReconcileHealsHistoricalPorcoRossoPhantom(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rawRoot, "anime-movies"), 0o755); err != nil {
		t.Fatal(err)
	}
	phantom := models.SourceGroup{Name: "Porco_Rosso", RelativePath: "Porco_Rosso", SourcePath: filepath.Join(rawRoot, "Porco_Rosso"), Enabled: true}
	if err := db.Create(&phantom).Error; err != nil {
		t.Fatal(err)
	}
	historicalPath := filepath.Join(rawRoot, "Porco_Rosso", "movie.mkv")
	currentPath := filepath.Join(rawRoot, "anime-movies", "Porco_Rosso", "movie.mkv")
	records := []models.AssetRecord{
		{Path: historicalPath, RootPath: rawRoot, RelativePath: "Porco_Rosso/movie.mkv", FileName: "movie.mkv", Status: "unprocessed", Missing: true, SourceGroupID: phantom.ID},
		{Path: currentPath, RootPath: rawRoot, RelativePath: "anime-movies/Porco_Rosso/movie.mkv", FileName: "movie.mkv", Status: "unprocessed"},
	}
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records[1:]); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&phantom, phantom.ID).Error; err != nil {
		t.Fatal(err)
	}
	if phantom.Enabled {
		t.Fatalf("historical phantom remained enabled: %#v", phantom)
	}
	hierarchy := buildAssetSourceGroups(db, records, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{filepath.Clean(historicalPath): true}})
	if len(hierarchy) != 1 || hierarchy[0].RelativePath != "anime-movies" || hierarchy[0].AssetCount != 1 {
		t.Fatalf("current hierarchy did not converge to anime-movies: %#v", hierarchy)
	}
}

func TestSourceGroupHierarchyMarksRootAndPreservesDeepChildPath(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	records := []models.AssetRecord{
		{Path: filepath.Join(rawRoot, "Anime", "Dragon Ball", "movie.mkv"), RootPath: rawRoot, RelativePath: "Anime/Dragon Ball/movie.mkv", FileName: "movie.mkv", Status: "unprocessed", SizeBytes: 10},
		{Path: filepath.Join(rawRoot, "Anime", "Dragon Ball", "Specials", "Movies", "special.mkv"), RootPath: rawRoot, RelativePath: "Anime/Dragon Ball/Specials/Movies/special.mkv", FileName: "special.mkv", Status: "unprocessed", SizeBytes: 20},
	}
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	hierarchy := buildAssetSourceGroups(db, records, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{}})
	logical := hierarchy[0].LogicalGroups[0]
	if logical.AssetCount != 2 || logical.PathCount != 2 || logical.TotalSizeBytes != 30 {
		t.Fatalf("unexpected logical metrics: %#v", logical)
	}
	if !logical.AssetPaths[0].IsLogicalGroupRoot || logical.AssetPaths[0].AssetCount != 1 {
		t.Fatalf("root path not identified: %#v", logical.AssetPaths[0])
	}
	child := logical.AssetPaths[1]
	if child.IsLogicalGroupRoot || child.DisplayPath != "Specials / Movies" || child.AssetCount != 1 || child.TotalSizeBytes != 20 {
		t.Fatalf("deep child path lost: %#v", child)
	}
}

func TestSourceGroupHierarchyKeepsActionableMissingAssetsVisible(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	presentPath := filepath.Join(rawRoot, "Movies", "Akira", "Akira.mkv")
	missingPath := filepath.Join(rawRoot, "Movies", "Akira", "Akira Bonus.mkv")
	historicalPath := filepath.Join(rawRoot, "Movies", "Akira", "Old Published Name.mkv")
	records := []models.AssetRecord{
		{Path: presentPath, RootPath: rawRoot, RelativePath: "Movies/Akira/Akira.mkv", FileName: "Akira.mkv", Status: "unprocessed", SizeBytes: 10},
		{Path: missingPath, RootPath: rawRoot, RelativePath: "Movies/Akira/Akira Bonus.mkv", FileName: "Akira Bonus.mkv", Status: "unprocessed", SizeBytes: 20, Missing: true},
		{Path: historicalPath, RootPath: rawRoot, RelativePath: "Movies/Akira/Old Published Name.mkv", FileName: "Old Published Name.mkv", Status: "unprocessed", SizeBytes: 30, Missing: true},
	}
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	hierarchy := buildAssetSourceGroups(db, records, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{filepath.Clean(historicalPath): true}})
	if len(hierarchy) != 1 || len(hierarchy[0].LogicalGroups) != 1 || len(hierarchy[0].LogicalGroups[0].AssetPaths) != 1 {
		t.Fatalf("unexpected hierarchy: %#v", hierarchy)
	}
	assets := hierarchy[0].LogicalGroups[0].AssetPaths[0].Assets
	if len(assets) != 2 || hierarchy[0].AssetCount != 2 || hierarchy[0].TotalSizeBytes != 30 {
		t.Fatalf("actionable missing asset was not retained: %#v", hierarchy[0])
	}
	if !assets[0].Missing && !assets[1].Missing {
		t.Fatalf("missing state was lost: %#v", assets)
	}
	for _, asset := range assets {
		if filepath.Clean(asset.Path) == filepath.Clean(historicalPath) {
			t.Fatalf("historical missing path was exposed as actionable: %#v", asset)
		}
	}
}

func TestSourceGroupHierarchyOnlyIncludesUnprocessedAndSortsNaturally(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	paths := []string{"Season10", "Season2", "Season1"}
	records := make([]models.AssetRecord, 0, len(paths)+1)
	for _, season := range paths {
		assetPath := filepath.Join(rawRoot, "Anime", "Dragon Ball", season, "episode01.mkv")
		records = append(records, models.AssetRecord{Path: assetPath, RootPath: rawRoot, RelativePath: filepath.ToSlash(filepath.Join("Anime", "Dragon Ball", season, "episode01.mkv")), FileName: "episode01.mkv", Status: "unprocessed"})
	}
	convertedPath := filepath.Join(rawRoot, "Anime", "Dragon Ball", "Season3", "converted.mkv")
	records = append(records, models.AssetRecord{Path: convertedPath, RootPath: rawRoot, RelativePath: "Anime/Dragon Ball/Season3/converted.mkv", FileName: "converted.mkv", Status: "converted"})
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	hierarchy := buildAssetSourceGroups(db, records, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{}})
	paths = []string{}
	for _, path := range hierarchy[0].LogicalGroups[0].AssetPaths {
		paths = append(paths, path.Name)
	}
	want := []string{"Season1", "Season2", "Season10"}
	if len(paths) != len(want) {
		t.Fatalf("converted asset was included: %v", paths)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("natural order=%v want=%v", paths, want)
		}
	}
}

func TestShallowSourceAssetDoesNotUseFilenameAsLogicalGroup(t *testing.T) {
	db := assetConfigurationTestDB(t)
	rawRoot := t.TempDir()
	assetPath := filepath.Join(rawRoot, "Movies", "Akira.mkv")
	records := []models.AssetRecord{{Path: assetPath, RootPath: rawRoot, RelativePath: "Movies/Akira.mkv", FileName: "Akira.mkv", Status: "unprocessed"}}
	if err := reconcileSourceGroups(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if err := annotateSourceRecords(db, rawRoot, records); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(records[0].LogicalGroupPath) == "Akira.mkv" {
		t.Fatalf("filename became logical group: %s", records[0].LogicalGroupPath)
	}
	hierarchy := buildAssetSourceGroups(db, records, nil, nil, nil, MissingClassification{HistoricalPaths: map[string]bool{}})
	if len(hierarchy) != 1 || len(hierarchy[0].LogicalGroups) != 1 || hierarchy[0].LogicalGroups[0].Name != "Root" {
		t.Fatalf("unexpected shallow hierarchy: %#v", hierarchy)
	}
}

func TestConfigureLogicalGroupsBatchPreservesNoChangeAndDescendantOverrides(t *testing.T) {
	db := assetConfigurationTestDB(t)
	oldProfile := models.Profile{Name: "Old path profile", Scope: "path", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 22}
	newProfile := models.Profile{Name: "New path profile", Scope: "path", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 18}
	if err := db.Create(&oldProfile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&newProfile).Error; err != nil {
		t.Fatal(err)
	}
	oldLibrary := models.Library{Name: "Old", DestinationPath: "/library/old"}
	if err := db.Create(&oldLibrary).Error; err != nil {
		t.Fatal(err)
	}
	groups := []string{"/media/raw/anime/Akira", "/media/raw/anime/Beck"}
	records := []models.AssetRecord{
		{Path: groups[0] + "/Season1/E01.mkv", LogicalGroupPath: groups[0], SourcePath: groups[0] + "/Season1/E01.mkv", FileName: "E01.mkv", Status: "unprocessed"},
		{Path: groups[1] + "/Season1/E01.mkv", LogicalGroupPath: groups[1], SourcePath: groups[1] + "/Season1/E01.mkv", FileName: "E01.mkv", Status: "unprocessed"},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	for _, group := range groups {
		if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: group, MediaType: "video", Selection: "profile", VideoProfileID: oldProfile.ID}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: group, MediaType: "audio", Selection: "disabled"}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := upsertAssetScopeConfiguration(db, AssetScopeConfigurationInput{ScopeType: assetScopeLogicalGroup, ScopeKey: group, CategorySelection: configSelectionValue, Category: "anime", DestinationSelection: configSelectionValue, DestinationLibraryID: oldLibrary.ID}); err != nil {
			t.Fatal(err)
		}
	}
	descendants := []models.ProfileAssignment{
		{TargetType: assetScopePath, TargetPath: filepath.Dir(records[0].Path), MediaType: "video", Selection: "disabled"},
		{TargetType: assetScopeAsset, TargetPath: records[0].Path, MediaType: "tracks", Selection: "disabled"},
	}
	if err := db.Create(&descendants).Error; err != nil {
		t.Fatal(err)
	}

	response := configureLogicalGroupsBatchRequest(t, db, ConfigureLogicalGroupsBatchInput{
		LogicalGroupPaths: groups,
		Video:             LogicalGroupConfigurationChange{Mode: batchChangeValue, VideoProfileID: newProfile.ID},
		Audio:             LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
		Tracks:            LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
		Category:          LogicalGroupConfigurationChange{Mode: batchChangeDisabled},
		Destination:       LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
	}, http.StatusOK)
	if len(response.ChangedDimensions) != 2 || response.ChangedDimensions[0] != "video" || response.ChangedDimensions[1] != "category" {
		t.Fatalf("unexpected changed dimensions: %#v", response)
	}
	for _, group := range groups {
		var assignments []models.ProfileAssignment
		if err := db.Where("target_type = ? AND target_path = ?", assetScopeLogicalGroup, group).Order("media_type").Find(&assignments).Error; err != nil {
			t.Fatal(err)
		}
		if len(assignments) != 2 || assignments[0].MediaType != "audio" || assignments[0].Selection != "disabled" || assignments[1].VideoProfileID != newProfile.ID {
			t.Fatalf("partial update changed untouched assignments for %s: %#v", group, assignments)
		}
		var configuration models.AssetScopeConfiguration
		if err := db.Where("scope_type = ? AND scope_key = ?", assetScopeLogicalGroup, group).First(&configuration).Error; err != nil {
			t.Fatal(err)
		}
		if configuration.CategorySelection != configSelectionDisabled || configuration.DestinationLibraryID != oldLibrary.ID {
			t.Fatalf("partial scope update lost destination for %s: %#v", group, configuration)
		}
	}
	var descendantCount int64
	if err := db.Model(&models.ProfileAssignment{}).Where("target_type IN ?", []string{assetScopePath, assetScopeAsset}).Count(&descendantCount).Error; err != nil || descendantCount != 2 {
		t.Fatalf("descendant overrides changed count=%d err=%v", descendantCount, err)
	}

	configureLogicalGroupsBatchRequest(t, db, ConfigureLogicalGroupsBatchInput{
		LogicalGroupPaths: groups,
		Video:             LogicalGroupConfigurationChange{Mode: batchChangeInherit},
		Audio:             LogicalGroupConfigurationChange{Mode: batchChangeDisabled},
		Tracks:            LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
		Category:          LogicalGroupConfigurationChange{Mode: batchChangeInherit},
		Destination:       LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
	}, http.StatusOK)
	for _, group := range groups {
		var videoCount int64
		if err := db.Model(&models.ProfileAssignment{}).Where("target_type = ? AND target_path = ? AND media_type = ?", assetScopeLogicalGroup, group, "video").Count(&videoCount).Error; err != nil || videoCount != 0 {
			t.Fatalf("inherit did not remove video for %s count=%d err=%v", group, videoCount, err)
		}
		var audio models.ProfileAssignment
		if err := db.Where("target_type = ? AND target_path = ? AND media_type = ?", assetScopeLogicalGroup, group, "audio").First(&audio).Error; err != nil || audio.Selection != "disabled" {
			t.Fatalf("disabled audio was not retained for %s: %#v err=%v", group, audio, err)
		}
		var configuration models.AssetScopeConfiguration
		if err := db.Where("scope_type = ? AND scope_key = ?", assetScopeLogicalGroup, group).First(&configuration).Error; err != nil || configuration.CategorySelection != configSelectionInherit || configuration.DestinationLibraryID != oldLibrary.ID {
			t.Fatalf("inherit/no-change scope result for %s: %#v err=%v", group, configuration, err)
		}
	}
}

func TestConfigureLogicalGroupsBatchRollsBackAllGroups(t *testing.T) {
	db := assetConfigurationTestDB(t)
	oldProfile := models.Profile{Name: "Old", Scope: "path", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 22}
	newProfile := models.Profile{Name: "New", Scope: "path", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 18}
	if err := db.Create(&oldProfile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&newProfile).Error; err != nil {
		t.Fatal(err)
	}
	groups := []string{"/media/raw/anime/A", "/media/raw/anime/B"}
	for _, group := range groups {
		record := models.AssetRecord{Path: group + "/E01.mkv", LogicalGroupPath: group, SourcePath: group + "/E01.mkv", FileName: "E01.mkv", Status: "unprocessed"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: group, MediaType: "video", Selection: "profile", VideoProfileID: oldProfile.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	callbackName := "test:fail-second-logical-group"
	if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		assignment, ok := tx.Statement.Dest.(*models.ProfileAssignment)
		if ok && filepath.Clean(assignment.TargetPath) == filepath.Clean(groups[1]) {
			tx.AddError(errors.New("forced second group failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(callbackName)

	configureLogicalGroupsBatchRequest(t, db, ConfigureLogicalGroupsBatchInput{
		LogicalGroupPaths: groups,
		Video:             LogicalGroupConfigurationChange{Mode: batchChangeValue, VideoProfileID: newProfile.ID},
		Audio:             LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
		Tracks:            LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
		Category:          LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
		Destination:       LogicalGroupConfigurationChange{Mode: batchChangeNoChange},
	}, http.StatusBadRequest)
	for _, group := range groups {
		var assignment models.ProfileAssignment
		if err := db.Where("target_type = ? AND target_path = ? AND media_type = ?", assetScopeLogicalGroup, group, "video").First(&assignment).Error; err != nil || assignment.VideoProfileID != oldProfile.ID {
			t.Fatalf("batch was not rolled back for %s: %#v err=%v", group, assignment, err)
		}
	}
}

func configureLogicalGroupsBatchRequest(t *testing.T, db *gorm.DB, input ConfigureLogicalGroupsBatchInput, wantStatus int) ConfigureLogicalGroupsBatchResponse {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/asset-scope-configurations/logical-groups/batch", NewAssetConfigurationHandler(db).ConfigureLogicalGroupsBatch)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/asset-scope-configurations/logical-groups/batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("status=%d want=%d body=%s", response.Code, wantStatus, response.Body.String())
	}
	var result ConfigureLogicalGroupsBatchResponse
	if wantStatus == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}
