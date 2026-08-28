package handlers

import (
	"errors"
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
	assignment, inherited, err := applyProfileAssignment(h.db, input)
	if err != nil {
		var inputErr *profileAssignmentInputError
		status := http.StatusInternalServerError
		if errors.As(err, &inputErr) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	if inherited {
		c.JSON(http.StatusOK, gin.H{"status": "inherited", "targetType": assignment.TargetType, "targetPath": assignment.TargetPath, "mediaType": assignment.MediaType})
		return
	}
	c.JSON(http.StatusOK, assignment)
}

func applyProfileAssignment(db *gorm.DB, input ProfileAssignmentInput) (models.ProfileAssignment, bool, error) {
	input.TargetType = strings.ToLower(strings.TrimSpace(input.TargetType))
	input.MediaType = strings.ToLower(strings.TrimSpace(input.MediaType))
	input.Selection = strings.ToLower(strings.TrimSpace(input.Selection))
	input.TargetPath = filepath.Clean(strings.TrimSpace(input.TargetPath))
	input.ProfileKey = strings.TrimSpace(input.ProfileKey)
	assignment := models.ProfileAssignment{
		TargetType: input.TargetType, TargetPath: input.TargetPath, MediaType: input.MediaType,
		Selection: input.Selection, VideoProfileID: input.VideoProfileID, ProfileKey: input.ProfileKey,
	}
	if !validateAssetScopeType(input.TargetType) || input.TargetPath == "." ||
		(input.MediaType != "video" && input.MediaType != "audio" && input.MediaType != "tracks") ||
		(input.Selection != "profile" && input.Selection != "disabled" && input.Selection != "inherit") {
		return assignment, false, &profileAssignmentInputError{message: "invalid profile assignment"}
	}
	if input.Selection == "inherit" {
		err := db.Where("target_type = ? AND target_path = ? AND media_type = ?", input.TargetType, input.TargetPath, input.MediaType).Delete(&models.ProfileAssignment{}).Error
		return assignment, true, err
	}
	if input.Selection == "profile" {
		if err := (ProfileAssignmentHandler{db: db}).validateProfile(input); err != nil {
			return assignment, false, &profileAssignmentInputError{message: err.Error()}
		}
	}
	if input.Selection == "disabled" {
		assignment.VideoProfileID = 0
		assignment.ProfileKey = ""
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "target_type"}, {Name: "target_path"}, {Name: "media_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"selection", "video_profile_id", "profile_key", "updated_at"}),
	}).Create(&assignment).Error; err != nil {
		return assignment, false, err
	}
	if err := db.Where("target_type = ? AND target_path = ? AND media_type = ?", input.TargetType, input.TargetPath, input.MediaType).First(&assignment).Error; err != nil {
		return assignment, false, err
	}
	return assignment, false, nil
}

type profileAssignmentInputError struct{ message string }

func (e *profileAssignmentInputError) Error() string { return e.message }

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
		expectedScope := "path"
		if input.TargetType == assetScopeAsset {
			expectedScope = "asset"
		}
		if normalizedStoredProfileScope(profile.Scope) != expectedScope {
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
		expectedScope := "path"
		if input.TargetType == assetScopeAsset {
			expectedScope = "asset"
		}
		if storedSettingProfileScope(profile) != expectedScope {
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
	return effectiveProfileAssignments(db, assetPath)
}
