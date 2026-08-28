package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AssetConfigurationHandler struct{ db *gorm.DB }

func NewAssetConfigurationHandler(db *gorm.DB) AssetConfigurationHandler {
	return AssetConfigurationHandler{db: db}
}

func (h AssetConfigurationHandler) ListSourceGroups(c *gin.Context) {
	var groups []models.SourceGroup
	if err := h.db.Order("name asc").Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

type sourceGroupUpdateInput struct {
	Name    string `json:"name"`
	Enabled *bool  `json:"enabled"`
}

func (h AssetConfigurationHandler) UpdateSourceGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source group id"})
		return
	}
	var input sourceGroupUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]interface{}{}
	if name := strings.TrimSpace(input.Name); name != "" {
		updates["name"] = name
	}
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
	}
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no source group changes supplied"})
		return
	}
	result := h.db.Model(&models.SourceGroup{}).Where("id = ?", uint(id)).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "source group not found"})
		return
	}
	var group models.SourceGroup
	if err := h.db.First(&group, uint(id)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, group)
}

func (h AssetConfigurationHandler) ListScopeConfigurations(c *gin.Context) {
	var configurations []models.AssetScopeConfiguration
	query := h.db.Order("scope_type asc, scope_key asc")
	if scopeType := strings.TrimSpace(c.Query("scopeType")); scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
	}
	if scopeKey := strings.TrimSpace(c.Query("scopeKey")); scopeKey != "" {
		query = query.Where("scope_key = ?", filepath.Clean(scopeKey))
	}
	if err := query.Find(&configurations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, configurations)
}

func (h AssetConfigurationHandler) UpsertScopeConfiguration(c *gin.Context) {
	var input AssetScopeConfigurationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	configuration, err := upsertAssetScopeConfiguration(h.db, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, configuration)
}

func (h AssetConfigurationHandler) Effective(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	configuration, err := effectiveAssetConfiguration(h.db, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, configuration)
}

const (
	assetScopeSourceGroup   = "source_group"
	assetScopeLogicalGroup  = "logical_group"
	assetScopePath          = "path"
	assetScopeAsset         = "asset"
	configSelectionInherit  = "inherit"
	configSelectionValue    = "value"
	configSelectionDisabled = "disabled"
)

type AssetConfigurationValue struct {
	Selection            string `json:"selection"`
	Source               string `json:"source,omitempty"`
	SourceKey            string `json:"sourceKey,omitempty"`
	SourceName           string `json:"sourceName,omitempty"`
	VideoProfileID       uint   `json:"videoProfileId,omitempty"`
	ProfileKey           string `json:"profileKey,omitempty"`
	Category             string `json:"category,omitempty"`
	DestinationLibraryID uint   `json:"destinationLibraryId,omitempty"`
}

type EffectiveAssetConfiguration struct {
	AssetPath   string                  `json:"assetPath"`
	Video       AssetConfigurationValue `json:"video"`
	Audio       AssetConfigurationValue `json:"audio"`
	Tracks      AssetConfigurationValue `json:"tracks"`
	Category    AssetConfigurationValue `json:"category"`
	Destination AssetConfigurationValue `json:"destination"`
}

type EffectiveAssetConfigurationBatchInput struct {
	AssetIDs []uint `json:"assetIds" binding:"required"`
}

type EffectiveAssetConfigurationBatchResponse struct {
	Configurations  map[string]EffectiveAssetConfiguration `json:"configurations"`
	MissingAssetIDs []uint                                 `json:"missingAssetIds"`
}

const (
	batchChangeNoChange = "no_change"
	batchChangeInherit  = "inherit"
	batchChangeValue    = "value"
	batchChangeDisabled = "disabled"
)

type LogicalGroupConfigurationChange struct {
	Mode                 string `json:"mode" binding:"required"`
	VideoProfileID       uint   `json:"videoProfileId,omitempty"`
	ProfileKey           string `json:"profileKey,omitempty"`
	Category             string `json:"category,omitempty"`
	DestinationLibraryID uint   `json:"destinationLibraryId,omitempty"`
}

type ConfigureLogicalGroupsBatchInput struct {
	LogicalGroupPaths []string                        `json:"logicalGroupPaths" binding:"required,min=1,max=500"`
	Video             LogicalGroupConfigurationChange `json:"video" binding:"required"`
	Audio             LogicalGroupConfigurationChange `json:"audio" binding:"required"`
	Tracks            LogicalGroupConfigurationChange `json:"tracks" binding:"required"`
	Category          LogicalGroupConfigurationChange `json:"category" binding:"required"`
	Destination       LogicalGroupConfigurationChange `json:"destination" binding:"required"`
}

type ConfigureLogicalGroupsBatchResponse struct {
	LogicalGroupPaths []string `json:"logicalGroupPaths"`
	ChangedDimensions []string `json:"changedDimensions"`
}

func (h AssetConfigurationHandler) ConfigureLogicalGroupsBatch(c *gin.Context) {
	var input ConfigureLogicalGroupsBatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	paths, changes, err := normalizeLogicalGroupBatchInput(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing []string
	if err := h.db.Model(&models.AssetRecord{}).
		Distinct("logical_group_path").
		Where("status = ? AND logical_group_path IN ?", "unprocessed", paths).
		Pluck("logical_group_path", &existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	existingSet := make(map[string]bool, len(existing))
	for _, path := range existing {
		existingSet[filepath.Clean(path)] = true
	}
	for _, path := range paths {
		if !existingSet[path] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("logical group is not an active Unprocessed scope: %s", path)})
			return
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, path := range paths {
			for _, mediaType := range []string{"video", "audio", "tracks"} {
				change := changes[mediaType]
				if change.Mode == batchChangeNoChange {
					continue
				}
				selection := change.Mode
				if selection == batchChangeValue {
					selection = "profile"
				}
				if _, _, err := applyProfileAssignment(tx, ProfileAssignmentInput{
					TargetType: assetScopeLogicalGroup, TargetPath: path, MediaType: mediaType,
					Selection: selection, VideoProfileID: change.VideoProfileID, ProfileKey: change.ProfileKey,
				}); err != nil {
					return fmt.Errorf("configure %s for %s: %w", mediaType, path, err)
				}
			}

			categoryChange, destinationChange := changes["category"], changes["destination"]
			if categoryChange.Mode == batchChangeNoChange && destinationChange.Mode == batchChangeNoChange {
				continue
			}
			current := models.AssetScopeConfiguration{
				ScopeType: assetScopeLogicalGroup, ScopeKey: path,
				CategorySelection: configSelectionInherit, DestinationSelection: configSelectionInherit,
			}
			if err := tx.Where("scope_type = ? AND scope_key = ?", assetScopeLogicalGroup, path).First(&current).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if categoryChange.Mode != batchChangeNoChange {
				current.CategorySelection = categoryChange.Mode
				current.Category = categoryChange.Category
			}
			if destinationChange.Mode != batchChangeNoChange {
				current.DestinationSelection = destinationChange.Mode
				current.DestinationLibraryID = destinationChange.DestinationLibraryID
			}
			if _, err := upsertAssetScopeConfiguration(tx, AssetScopeConfigurationInput{
				ScopeType: assetScopeLogicalGroup, ScopeKey: path,
				CategorySelection: current.CategorySelection, Category: current.Category,
				DestinationSelection: current.DestinationSelection, DestinationLibraryID: current.DestinationLibraryID,
			}); err != nil {
				return fmt.Errorf("configure scope values for %s: %w", path, err)
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	changed := make([]string, 0, len(changes))
	for _, dimension := range []string{"video", "audio", "tracks", "category", "destination"} {
		if changes[dimension].Mode != batchChangeNoChange {
			changed = append(changed, dimension)
		}
	}
	c.JSON(http.StatusOK, ConfigureLogicalGroupsBatchResponse{LogicalGroupPaths: paths, ChangedDimensions: changed})
}

func normalizeLogicalGroupBatchInput(input ConfigureLogicalGroupsBatchInput) ([]string, map[string]LogicalGroupConfigurationChange, error) {
	paths := make([]string, 0, len(input.LogicalGroupPaths))
	seen := map[string]bool{}
	for _, rawPath := range input.LogicalGroupPaths {
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("at least one valid logical group path is required")
	}
	changes := map[string]LogicalGroupConfigurationChange{
		"video": input.Video, "audio": input.Audio, "tracks": input.Tracks,
		"category": input.Category, "destination": input.Destination,
	}
	changed := false
	for dimension, change := range changes {
		change.Mode = strings.ToLower(strings.TrimSpace(change.Mode))
		change.ProfileKey = strings.TrimSpace(change.ProfileKey)
		change.Category = strings.TrimSpace(change.Category)
		if change.Mode != batchChangeNoChange && change.Mode != batchChangeInherit && change.Mode != batchChangeValue && change.Mode != batchChangeDisabled {
			return nil, nil, fmt.Errorf("invalid %s change mode", dimension)
		}
		if change.Mode != batchChangeNoChange {
			changed = true
		}
		if change.Mode == batchChangeValue {
			switch dimension {
			case "video":
				if change.VideoProfileID == 0 {
					return nil, nil, fmt.Errorf("video profile is required")
				}
			case "audio", "tracks":
				if change.ProfileKey == "" {
					return nil, nil, fmt.Errorf("%s profile is required", dimension)
				}
			case "category":
				if change.Category == "" {
					return nil, nil, fmt.Errorf("category value is required")
				}
			case "destination":
				if change.DestinationLibraryID == 0 {
					return nil, nil, fmt.Errorf("destination library is required")
				}
			}
		}
		changes[dimension] = change
	}
	if !changed {
		return nil, nil, fmt.Errorf("at least one dimension must change")
	}
	return paths, changes, nil
}

func (h AssetConfigurationHandler) EffectiveBatch(c *gin.Context) {
	var input EffectiveAssetConfigurationBatchInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.AssetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds must contain at least one asset"})
		return
	}
	if len(input.AssetIDs) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds cannot contain more than 1000 assets"})
		return
	}

	assetIDs := make([]uint, 0, len(input.AssetIDs))
	seen := make(map[uint]bool, len(input.AssetIDs))
	for _, assetID := range input.AssetIDs {
		if assetID == 0 || seen[assetID] {
			continue
		}
		seen[assetID] = true
		assetIDs = append(assetIDs, assetID)
	}
	if len(assetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds must contain a valid asset id"})
		return
	}

	var records []models.AssetRecord
	if err := h.db.Where("id IN ?", assetIDs).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	byID := make(map[uint]models.AssetRecord, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	response := EffectiveAssetConfigurationBatchResponse{
		Configurations:  map[string]EffectiveAssetConfiguration{},
		MissingAssetIDs: []uint{},
	}
	resolver, err := loadAssetConfigurationResolver(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, assetID := range assetIDs {
		record, exists := byID[assetID]
		if !exists {
			response.MissingAssetIDs = append(response.MissingAssetIDs, assetID)
			continue
		}
		configuration := resolver.resolve(record)
		response.Configurations[strconv.FormatUint(uint64(assetID), 10)] = configuration
	}
	c.JSON(http.StatusOK, response)
}

type assetScopeTarget struct {
	Type string
	Key  string
	Name string
}

type assetConfigurationResolver struct {
	sourceGroups   map[uint]models.SourceGroup
	assignments    map[string]models.ProfileAssignment
	configurations map[string]models.AssetScopeConfiguration
}

func loadAssetConfigurationResolver(db *gorm.DB) (assetConfigurationResolver, error) {
	resolver := assetConfigurationResolver{
		sourceGroups: map[uint]models.SourceGroup{}, assignments: map[string]models.ProfileAssignment{}, configurations: map[string]models.AssetScopeConfiguration{},
	}
	if db.Migrator().HasTable(&models.SourceGroup{}) {
		var groups []models.SourceGroup
		if err := db.Find(&groups).Error; err != nil {
			return resolver, err
		}
		for _, group := range groups {
			resolver.sourceGroups[group.ID] = group
		}
	}
	if db.Migrator().HasTable(&models.ProfileAssignment{}) {
		var assignments []models.ProfileAssignment
		if err := db.Find(&assignments).Error; err != nil {
			return resolver, err
		}
		for _, assignment := range assignments {
			resolver.assignments[assignmentKey(assignment.TargetType, assignment.TargetPath, assignment.MediaType)] = assignment
		}
	}
	if db.Migrator().HasTable(&models.AssetScopeConfiguration{}) {
		var configurations []models.AssetScopeConfiguration
		if err := db.Find(&configurations).Error; err != nil {
			return resolver, err
		}
		for _, configuration := range configurations {
			resolver.configurations[scopeKey(configuration.ScopeType, configuration.ScopeKey)] = configuration
		}
	}
	return resolver, nil
}

func (resolver assetConfigurationResolver) targetsForRecord(record models.AssetRecord) []assetScopeTarget {
	assetPath := filepath.Clean(record.Path)
	pathTarget := filepath.Dir(assetPath)
	if strings.TrimSpace(record.GroupPath) != "" && strings.TrimSpace(record.RootPath) != "" {
		pathTarget = filepath.Clean(filepath.Join(record.RootPath, filepath.FromSlash(record.GroupPath)))
	}
	targets := []assetScopeTarget{}
	if record.Status == "unprocessed" && record.SourceGroupID > 0 {
		if group, exists := resolver.sourceGroups[record.SourceGroupID]; exists {
			targets = append(targets, assetScopeTarget{Type: assetScopeSourceGroup, Key: filepath.Clean(group.SourcePath), Name: group.Name})
		}
	}
	if logical := strings.TrimSpace(record.LogicalGroupPath); record.Status == "unprocessed" && logical != "" {
		targets = append(targets, assetScopeTarget{Type: assetScopeLogicalGroup, Key: filepath.Clean(logical), Name: filepath.Base(filepath.Clean(logical))})
	}
	targets = append(targets,
		assetScopeTarget{Type: assetScopePath, Key: pathTarget, Name: filepath.Base(pathTarget)},
		assetScopeTarget{Type: assetScopeAsset, Key: assetPath, Name: record.FileName},
	)
	return targets
}

func (resolver assetConfigurationResolver) resolve(record models.AssetRecord) EffectiveAssetConfiguration {
	clean := filepath.Clean(record.Path)
	result := EffectiveAssetConfiguration{AssetPath: clean}
	result.Video.Selection = configSelectionInherit
	result.Audio.Selection = configSelectionInherit
	result.Tracks.Selection = configSelectionInherit
	result.Category.Selection = configSelectionInherit
	result.Destination.Selection = configSelectionInherit

	targets := resolver.targetsForRecord(record)
	for _, target := range targets {
		for _, mediaType := range []string{"video", "audio", "tracks"} {
			assignment, ok := resolver.assignments[assignmentKey(target.Type, target.Key, mediaType)]
			if !ok {
				continue
			}
			value := AssetConfigurationValue{
				Selection: assignment.Selection, Source: target.Type, SourceKey: target.Key, SourceName: target.Name,
				VideoProfileID: assignment.VideoProfileID, ProfileKey: assignment.ProfileKey,
			}
			switch mediaType {
			case "video":
				result.Video = value
			case "audio":
				result.Audio = value
			case "tracks":
				result.Tracks = value
			}
		}
	}
	for _, target := range targets {
		configuration, ok := resolver.configurations[scopeKey(target.Type, target.Key)]
		if !ok {
			continue
		}
		if configuration.CategorySelection != "" && configuration.CategorySelection != configSelectionInherit {
			result.Category = AssetConfigurationValue{Selection: configuration.CategorySelection, Category: configuration.Category, Source: target.Type, SourceKey: target.Key, SourceName: target.Name}
		}
		if configuration.DestinationSelection != "" && configuration.DestinationSelection != configSelectionInherit {
			result.Destination = AssetConfigurationValue{Selection: configuration.DestinationSelection, DestinationLibraryID: configuration.DestinationLibraryID, Source: target.Type, SourceKey: target.Key, SourceName: target.Name}
		}
	}
	return result
}

func assetScopeTargetsForRecord(db *gorm.DB, record models.AssetRecord) ([]assetScopeTarget, error) {
	resolver, err := loadAssetConfigurationResolver(db)
	if err != nil {
		return nil, err
	}
	return resolver.targetsForRecord(record), nil
}

func effectiveAssetConfiguration(db *gorm.DB, assetPath string) (EffectiveAssetConfiguration, error) {
	clean := filepath.Clean(assetPath)
	var record models.AssetRecord
	if err := db.Where("path = ?", clean).First(&record).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return EffectiveAssetConfiguration{}, err
		}
		record = models.AssetRecord{Path: clean, FileName: filepath.Base(clean)}
	}
	resolver, err := loadAssetConfigurationResolver(db)
	if err != nil {
		return EffectiveAssetConfiguration{}, err
	}
	return resolver.resolve(record), nil
}

func effectiveProfileAssignments(db *gorm.DB, assetPath string) (map[string]models.ProfileAssignment, error) {
	configuration, err := effectiveAssetConfiguration(db, assetPath)
	if err != nil {
		return nil, err
	}
	resolved := map[string]models.ProfileAssignment{}
	for mediaType, value := range map[string]AssetConfigurationValue{"video": configuration.Video, "audio": configuration.Audio, "tracks": configuration.Tracks} {
		if value.Source == "" || value.Selection == configSelectionInherit {
			continue
		}
		resolved[mediaType] = models.ProfileAssignment{TargetType: value.Source, TargetPath: value.SourceKey, MediaType: mediaType, Selection: value.Selection, VideoProfileID: value.VideoProfileID, ProfileKey: value.ProfileKey}
	}
	return resolved, nil
}

func assignmentKey(targetType, targetPath, mediaType string) string {
	return strings.ToLower(strings.TrimSpace(targetType)) + "\x00" + filepath.Clean(targetPath) + "\x00" + strings.ToLower(strings.TrimSpace(mediaType))
}

func scopeKey(scopeType, key string) string {
	return strings.ToLower(strings.TrimSpace(scopeType)) + "\x00" + filepath.Clean(key)
}

type AssetScopeConfigurationInput struct {
	ScopeType            string `json:"scopeType" binding:"required"`
	ScopeKey             string `json:"scopeKey" binding:"required"`
	CategorySelection    string `json:"categorySelection"`
	Category             string `json:"category"`
	DestinationSelection string `json:"destinationSelection"`
	DestinationLibraryID uint   `json:"destinationLibraryId"`
}

func validateAssetScopeType(value string) bool {
	switch value {
	case assetScopeSourceGroup, assetScopeLogicalGroup, assetScopePath, assetScopeAsset:
		return true
	default:
		return false
	}
}

func normalizeScopeSelection(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return configSelectionInherit
	}
	return value
}

func upsertAssetScopeConfiguration(db *gorm.DB, input AssetScopeConfigurationInput) (models.AssetScopeConfiguration, error) {
	input.ScopeType = strings.ToLower(strings.TrimSpace(input.ScopeType))
	input.ScopeKey = filepath.Clean(strings.TrimSpace(input.ScopeKey))
	input.CategorySelection = normalizeScopeSelection(input.CategorySelection)
	input.DestinationSelection = normalizeScopeSelection(input.DestinationSelection)
	input.Category = strings.TrimSpace(input.Category)
	if !validateAssetScopeType(input.ScopeType) || input.ScopeKey == "." {
		return models.AssetScopeConfiguration{}, fmt.Errorf("invalid asset configuration scope")
	}
	if input.CategorySelection != configSelectionInherit && input.CategorySelection != configSelectionValue && input.CategorySelection != configSelectionDisabled {
		return models.AssetScopeConfiguration{}, fmt.Errorf("invalid category selection")
	}
	if input.DestinationSelection != configSelectionInherit && input.DestinationSelection != configSelectionValue && input.DestinationSelection != configSelectionDisabled {
		return models.AssetScopeConfiguration{}, fmt.Errorf("invalid destination selection")
	}
	if input.CategorySelection == configSelectionValue && input.Category == "" {
		return models.AssetScopeConfiguration{}, fmt.Errorf("category value is required")
	}
	if input.DestinationSelection == configSelectionValue {
		if input.DestinationLibraryID == 0 {
			return models.AssetScopeConfiguration{}, fmt.Errorf("destination library is required")
		}
		var count int64
		if err := db.Model(&models.Library{}).Where("id = ?", input.DestinationLibraryID).Count(&count).Error; err != nil {
			return models.AssetScopeConfiguration{}, err
		}
		if count == 0 {
			return models.AssetScopeConfiguration{}, fmt.Errorf("destination library does not exist")
		}
	}
	configuration := models.AssetScopeConfiguration{
		ScopeType: input.ScopeType, ScopeKey: input.ScopeKey,
		CategorySelection: input.CategorySelection, Category: input.Category,
		DestinationSelection: input.DestinationSelection, DestinationLibraryID: input.DestinationLibraryID,
	}
	if input.CategorySelection != configSelectionValue {
		configuration.Category = ""
	}
	if input.DestinationSelection != configSelectionValue {
		configuration.DestinationLibraryID = 0
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "scope_type"}, {Name: "scope_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"category_selection", "category", "destination_selection", "destination_library_id", "updated_at"}),
	}).Create(&configuration).Error; err != nil {
		return models.AssetScopeConfiguration{}, err
	}
	if err := db.Where("scope_type = ? AND scope_key = ?", configuration.ScopeType, configuration.ScopeKey).First(&configuration).Error; err != nil {
		return models.AssetScopeConfiguration{}, err
	}
	return configuration, nil
}

func sortedScopeTargets(targets []assetScopeTarget) []assetScopeTarget {
	copyTargets := append([]assetScopeTarget(nil), targets...)
	sort.SliceStable(copyTargets, func(i, j int) bool { return scopeRank(copyTargets[i].Type) < scopeRank(copyTargets[j].Type) })
	return copyTargets
}

func scopeRank(scope string) int {
	switch scope {
	case assetScopeSourceGroup:
		return 0
	case assetScopeLogicalGroup:
		return 1
	case assetScopePath:
		return 2
	case assetScopeAsset:
		return 3
	default:
		return 4
	}
}
