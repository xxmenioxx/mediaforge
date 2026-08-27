package handlers

import (
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

type assetScopeTarget struct {
	Type string
	Key  string
	Name string
}

func assetScopeTargetsForRecord(db *gorm.DB, record models.AssetRecord) ([]assetScopeTarget, error) {
	assetPath := filepath.Clean(record.Path)
	pathTarget := filepath.Dir(assetPath)
	if strings.TrimSpace(record.GroupPath) != "" && strings.TrimSpace(record.RootPath) != "" {
		pathTarget = filepath.Clean(filepath.Join(record.RootPath, filepath.FromSlash(record.GroupPath)))
	}
	targets := []assetScopeTarget{}
	if record.SourceGroupID > 0 && db.Migrator().HasTable(&models.SourceGroup{}) {
		var group models.SourceGroup
		if err := db.First(&group, record.SourceGroupID).Error; err == nil {
			targets = append(targets, assetScopeTarget{Type: assetScopeSourceGroup, Key: filepath.Clean(group.SourcePath), Name: group.Name})
		} else if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}
	if logical := strings.TrimSpace(record.LogicalGroupPath); logical != "" {
		targets = append(targets, assetScopeTarget{Type: assetScopeLogicalGroup, Key: filepath.Clean(logical), Name: filepath.Base(filepath.Clean(logical))})
	}
	targets = append(targets,
		assetScopeTarget{Type: assetScopePath, Key: pathTarget, Name: filepath.Base(pathTarget)},
		assetScopeTarget{Type: assetScopeAsset, Key: assetPath, Name: record.FileName},
	)
	return targets, nil
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
	targets, err := assetScopeTargetsForRecord(db, record)
	if err != nil {
		return EffectiveAssetConfiguration{}, err
	}
	result := EffectiveAssetConfiguration{AssetPath: clean}
	result.Video.Selection = configSelectionInherit
	result.Audio.Selection = configSelectionInherit
	result.Tracks.Selection = configSelectionInherit
	result.Category.Selection = configSelectionInherit
	result.Destination.Selection = configSelectionInherit

	if db.Migrator().HasTable(&models.ProfileAssignment{}) {
		var assignments []models.ProfileAssignment
		if err := db.Find(&assignments).Error; err != nil {
			return EffectiveAssetConfiguration{}, err
		}
		byTarget := map[string]models.ProfileAssignment{}
		for _, assignment := range assignments {
			byTarget[assignmentKey(assignment.TargetType, assignment.TargetPath, assignment.MediaType)] = assignment
		}
		for _, target := range targets {
			for _, mediaType := range []string{"video", "audio", "tracks"} {
				assignment, ok := byTarget[assignmentKey(target.Type, target.Key, mediaType)]
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
	}

	if db.Migrator().HasTable(&models.AssetScopeConfiguration{}) {
		var configurations []models.AssetScopeConfiguration
		if err := db.Find(&configurations).Error; err != nil {
			return EffectiveAssetConfiguration{}, err
		}
		byTarget := map[string]models.AssetScopeConfiguration{}
		for _, configuration := range configurations {
			byTarget[scopeKey(configuration.ScopeType, configuration.ScopeKey)] = configuration
		}
		for _, target := range targets {
			configuration, ok := byTarget[scopeKey(target.Type, target.Key)]
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
	}
	return result, nil
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
