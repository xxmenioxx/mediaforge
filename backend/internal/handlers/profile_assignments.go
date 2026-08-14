package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProfileAssignmentHandler struct{ db *gorm.DB }

type ProfileAssignmentInput struct {
	TargetType     string `json:"targetType" binding:"required"`
	TargetPath     string `json:"targetPath" binding:"required"`
	MediaType      string `json:"mediaType" binding:"required"`
	Selection      string `json:"selection" binding:"required"`
	VideoProfileID uint   `json:"videoProfileId"`
	ProfileKey     string `json:"profileKey"`
}

func NewProfileAssignmentHandler(db *gorm.DB) ProfileAssignmentHandler {
	return ProfileAssignmentHandler{db: db}
}

func (h ProfileAssignmentHandler) List(c *gin.Context) {
	var assignments []models.ProfileAssignment
	query := h.db.Order("target_type, target_path, media_type")
	if value := strings.TrimSpace(c.Query("targetType")); value != "" {
		query = query.Where("target_type = ?", value)
	}
	if value := strings.TrimSpace(c.Query("targetPath")); value != "" {
		query = query.Where("target_path = ?", filepath.Clean(value))
	}
	if err := query.Find(&assignments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, assignments)
}

func (h ProfileAssignmentHandler) Upsert(c *gin.Context) {
	var input ProfileAssignmentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.TargetType = strings.ToLower(strings.TrimSpace(input.TargetType))
	input.MediaType = strings.ToLower(strings.TrimSpace(input.MediaType))
	input.Selection = strings.ToLower(strings.TrimSpace(input.Selection))
	input.TargetPath = filepath.Clean(strings.TrimSpace(input.TargetPath))
	input.ProfileKey = strings.TrimSpace(input.ProfileKey)
	if (input.TargetType != "asset" && input.TargetType != "path") ||
		(input.MediaType != "video" && input.MediaType != "audio" && input.MediaType != "tracks") ||
		(input.Selection != "profile" && input.Selection != "disabled" && input.Selection != "inherit") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile assignment"})
		return
	}
	if input.Selection == "inherit" {
		if input.TargetType != "asset" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inherit is only valid for asset assignments"})
			return
		}
		if err := h.db.Where("target_type = ? AND target_path = ? AND media_type = ?", input.TargetType, input.TargetPath, input.MediaType).Delete(&models.ProfileAssignment{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "inherited", "targetType": input.TargetType, "targetPath": input.TargetPath, "mediaType": input.MediaType})
		return
	}
	if input.Selection == "profile" {
		if err := h.validateProfile(input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	assignment := models.ProfileAssignment{
		TargetType: input.TargetType, TargetPath: input.TargetPath, MediaType: input.MediaType,
		Selection: input.Selection, VideoProfileID: input.VideoProfileID, ProfileKey: input.ProfileKey,
	}
	if input.Selection == "disabled" {
		assignment.VideoProfileID = 0
		assignment.ProfileKey = ""
	}
	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "target_type"}, {Name: "target_path"}, {Name: "media_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"selection", "video_profile_id", "profile_key", "updated_at"}),
	}).Create(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Where("target_type = ? AND target_path = ? AND media_type = ?", input.TargetType, input.TargetPath, input.MediaType).First(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, assignment)
}

func (h ProfileAssignmentHandler) validateProfile(input ProfileAssignmentInput) error {
	if input.MediaType == "video" {
		if input.VideoProfileID == 0 {
			return errProfileAssignmentMissingProfile
		}
		var profile models.Profile
		if err := h.db.First(&profile, input.VideoProfileID).Error; err != nil {
			return errProfileAssignmentMissingProfile
		}
		if profile.Disabled {
			return errProfileAssignmentInactiveProfile
		}
		if normalizedStoredProfileScope(profile.Scope) != input.TargetType {
			return errProfileAssignmentScope
		}
		return nil
	}
	if input.ProfileKey == "" {
		return errProfileAssignmentMissingProfile
	}
	settingKey := "audioEnhancementProfiles"
	if input.MediaType == "tracks" {
		settingKey = "trackProfiles"
	}
	var setting models.AppSetting
	if err := h.db.Where("key = ?", settingKey).First(&setting).Error; err != nil {
		return errProfileAssignmentMissingProfile
	}
	profiles := settingProfileValues(setting.Value["profiles"])
	for _, raw := range profiles {
		profile := settingProfileObject(raw)
		if profile == nil || strings.TrimSpace(stringFromUnknown(profile["key"])) != input.ProfileKey {
			continue
		}
		if storedSettingProfileScope(profile) != input.TargetType {
			return errProfileAssignmentScope
		}
		disabled, _ := profile["disabled"].(bool)
		if disabled || strings.TrimSpace(stringFromUnknown(profile["deletedAt"])) != "" {
			return errProfileAssignmentInactiveProfile
		}
		return nil
	}
	return errProfileAssignmentMissingProfile
}

func settingProfileValues(value any) []any {
	switch profiles := value.(type) {
	case []any:
		return profiles
	case models.JSONList:
		return []any(profiles)
	default:
		return nil
	}
}

func settingProfileObject(value any) map[string]any {
	switch profile := value.(type) {
	case map[string]any:
		return profile
	case models.JSONMap:
		return map[string]any(profile)
	default:
		return nil
	}
}

func storedSettingProfileScope(profile map[string]any) string {
	if scope := strings.TrimSpace(stringFromUnknown(profile["scope"])); scope != "" {
		return normalizedProfileScope(scope)
	}
	return "asset"
}

var (
	errProfileAssignmentMissingProfile  = &profileAssignmentError{"selected profile does not exist"}
	errProfileAssignmentScope           = &profileAssignmentError{"selected profile has a different application scope"}
	errProfileAssignmentInactiveProfile = &profileAssignmentError{"selected profile is disabled or deleted"}
)

type profileAssignmentError struct{ message string }

func (e *profileAssignmentError) Error() string { return e.message }

func profileAssignmentsForAsset(db *gorm.DB, assetPath string) (map[string]models.ProfileAssignment, error) {
	clean := filepath.Clean(assetPath)
	targets := []string{clean}
	var record models.AssetRecord
	if err := db.Where("path = ?", clean).First(&record).Error; err == nil && strings.TrimSpace(record.GroupPath) != "" {
		targets = append(targets, filepath.Clean(filepath.Join(record.RootPath, record.GroupPath)))
	} else {
		targets = append(targets, filepath.Dir(clean))
	}
	var assignments []models.ProfileAssignment
	if err := db.Where("(target_type = ? AND target_path = ?) OR (target_type = ? AND target_path = ?)", "asset", targets[0], "path", targets[1]).Find(&assignments).Error; err != nil {
		return nil, err
	}
	resolved := map[string]models.ProfileAssignment{}
	for _, assignment := range assignments {
		if existing, ok := resolved[assignment.MediaType]; !ok || (existing.TargetType == "path" && assignment.TargetType == "asset") {
			resolved[assignment.MediaType] = assignment
		}
	}
	return resolved, nil
}
