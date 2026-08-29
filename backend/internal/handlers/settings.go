package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingsHandler struct {
	db *gorm.DB
}

type SettingInput struct {
	Value models.JSONMap `json:"value" binding:"required"`
}

func NewSettingsHandler(db *gorm.DB) SettingsHandler {
	return SettingsHandler{db: db}
}

func (h SettingsHandler) List(c *gin.Context) {
	var settings []models.AppSetting
	if err := h.db.Order("key asc").Find(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func (h SettingsHandler) Update(c *gin.Context) {
	var input SettingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	key := c.Param("key")
	if key == "runtimePolicy" {
		if err := runtimeinfo.ValidateRuntimePolicyValue(input.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if key == analysisPolicySettingKey {
		if err := validateAnalysisPolicy(input.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		input.Value = normalizedAnalysisPolicyValue(input.Value)
	}
	if key == "mvforgePreferences" {
		if err := validateMVForgePreferences(input.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if key == "originalRetentionPolicy" {
		if err := validateOriginalRetentionPolicy(input.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if key == "trackProfiles" {
		if err := validateTrackProfileDomainPolicies(input.Value); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if key == "audioEnhancementProfiles" || key == "trackProfiles" {
		mediaType := "audio"
		if key == "trackProfiles" {
			mediaType = "tracks"
		}
		if err := validateSettingProfileAssignments(h.db, mediaType, input.Value); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if key == "trackProfiles" {
			input.Value = normalizeTrackProfilesForStorage(input.Value)
		}
	}
	setting := models.AppSetting{Key: key, Value: input.Value, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.First(&setting, "key = ?", key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if key == "pipelineAutomation" {
		if err := h.db.Model(&models.ExecutionPlan{}).
			Where("status = ? AND waiting_state = ? AND approval_status = ?", scheduler.ExecutionPlanWaiting, "WAITING_REVIEW", scheduler.ApprovalPending).
			Updates(map[string]any{"status": scheduler.ExecutionPlanPendingEvaluation, "waiting_state": ""}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if key == "directPlay" {
		if err := h.db.Model(&models.ExecutionPlan{}).
			Where("status = ? AND waiting_state = ?", scheduler.ExecutionPlanWaiting, scheduler.WaitingDirectPlayReview).
			Updates(map[string]any{"status": scheduler.ExecutionPlanPendingEvaluation, "waiting_state": ""}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := h.db.Model(&models.QueueJob{}).
			Where("status = ? AND published_at IS NULL", JobStatusCompleted).
			Updates(map[string]any{"validation_status": ValidationStatusPending, "validation_score": 0, "validation_report": models.JSONMap{}}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if key == "runtimePolicy" {
		if _, err := runtimeinfo.DetectAndSave(h.db); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, setting)
}

func validateTrackProfileDomainPolicies(value models.JSONMap) error {
	for index, raw := range settingProfileValues(value["profiles"]) {
		profile := settingProfileObject(raw)
		if profile == nil {
			continue
		}
		name := strings.TrimSpace(workerStringValue(profile["key"]))
		if name == "" {
			name = fmt.Sprintf("profile %d", index+1)
		}
		if _, err := canonicalTrackDispositionProfile(profile); err != nil {
			return fmt.Errorf("track profile %s: %w", name, err)
		}
		if rawValue, exists := profile["subtitleDisposition"]; exists {
			if _, err := ParseSubtitleDisposition(workerStringValue(rawValue)); err != nil {
				return fmt.Errorf("track profile %s: %w", name, err)
			}
		}
		if rawValue, exists := profile["attachmentPolicy"]; exists {
			if _, err := ParseAttachmentPolicy(workerStringValue(rawValue)); err != nil {
				return fmt.Errorf("track profile %s: %w", name, err)
			}
		}
		if rawValue, exists := profile["chapterPolicy"]; exists {
			if _, err := ParseChapterPolicy(workerStringValue(rawValue)); err != nil {
				return fmt.Errorf("track profile %s: %w", name, err)
			}
		}
		if err := validateSubtitleRules(profile); err != nil {
			return fmt.Errorf("track profile %s: %w", name, err)
		}
	}
	return nil
}

func normalizeTrackProfilesForStorage(value models.JSONMap) models.JSONMap {
	result := models.JSONMap{}
	for key, item := range value {
		result[key] = item
	}
	profiles := settingProfileValues(value["profiles"])
	normalized := make(models.JSONList, 0, len(profiles))
	for _, raw := range profiles {
		profile := settingProfileObject(raw)
		if profile == nil {
			normalized = append(normalized, raw)
			continue
		}
		copy := map[string]any{}
		for key, item := range profile {
			copy[key] = item
		}
		if storedSettingProfileScope(copy) == "path" {
			for _, key := range []string{"keepVideoStreams", "keepAudioStreams", "keepSubtitleStreams", "videoMetadata", "audioMetadata", "subtitleMetadata", "subtitleTransforms"} {
				delete(copy, key)
			}
			if rules := workerSliceValue(copy["subtitleRules"]); rules != nil {
				semantic := models.JSONList{}
				for _, rawRule := range rules {
					rule := settingProfileObject(rawRule)
					if _, concrete := rule["streamIndex"]; concrete {
						continue
					}
					semantic = append(semantic, rawRule)
				}
				copy["subtitleRules"] = semantic
			}
		}
		normalized = append(normalized, copy)
	}
	result["profiles"] = normalized
	return result
}

func validateOriginalRetentionPolicy(value models.JSONMap) error {
	days := intValueSetting(value["keepOriginalsDays"], -1)
	if days < 0 || days > 3650 {
		return fmt.Errorf("keepOriginalsDays must be between 0 and 3650")
	}
	if boolSetting(value["autoDeleteEnabled"], false) && days == 0 {
		return fmt.Errorf("keepOriginalsDays must be greater than 0 when automatic deletion is enabled")
	}
	archivePath := strings.TrimSpace(stringFromUnknown(value["processedOriginalsPath"]))
	if archivePath == "" || !strings.HasPrefix(archivePath, "/") {
		return fmt.Errorf("processedOriginalsPath must be an absolute path")
	}
	return nil
}

func validateMVForgePreferences(value models.JSONMap) error {
	if normalizedOptimizationIntent(stringFromUnknown(value["qualityGoal"])) == "" {
		return fmt.Errorf("qualityGoal must be maximum_savings, balanced, conservative, maximum_quality, or archive")
	}
	execution := strings.ToLower(strings.TrimSpace(stringFromUnknown(value["executionPreference"])))
	if execution != "software" && execution != "hardware" {
		return fmt.Errorf("executionPreference must be software or hardware")
	}
	if strings.TrimSpace(stringFromUnknown(value["preferredVideoEncoder"])) == "" {
		return fmt.Errorf("preferredVideoEncoder is required")
	}
	encoder := strings.ToLower(strings.TrimSpace(stringFromUnknown(value["preferredVideoEncoder"])))
	if execution == "hardware" && encoder != "auto" && !strings.HasSuffix(encoder, "_qsv") && !strings.HasSuffix(encoder, "_videotoolbox") && !strings.HasSuffix(encoder, "_nvenc") && !strings.HasSuffix(encoder, "_vaapi") && !strings.HasSuffix(encoder, "_amf") {
		return fmt.Errorf("preferredVideoEncoder must be auto or a hardware encoder when executionPreference is hardware")
	}
	if execution == "software" && encoder != "auto" && !strings.HasPrefix(encoder, "lib") {
		return fmt.Errorf("preferredVideoEncoder must be auto or a software encoder when executionPreference is software")
	}
	if len(normalizedPreferenceLanguages(value["preferredLanguages"])) == 0 {
		return fmt.Errorf("at least one preferred language is required")
	}
	return nil
}

func validateSettingProfileAssignments(db *gorm.DB, mediaType string, value models.JSONMap) error {
	if !db.Migrator().HasTable(&models.ProfileAssignment{}) {
		return nil
	}
	profiles := settingProfileValues(value["profiles"])
	byKey := map[string]map[string]any{}
	for _, raw := range profiles {
		profile := settingProfileObject(raw)
		if profile != nil {
			byKey[strings.TrimSpace(stringFromUnknown(profile["key"]))] = profile
		}
	}
	var assignments []models.ProfileAssignment
	if err := db.Where("media_type = ? AND selection = ?", mediaType, "profile").Find(&assignments).Error; err != nil {
		return err
	}
	for _, assignment := range assignments {
		profile, exists := byKey[assignment.ProfileKey]
		if !exists {
			return fmt.Errorf("profile %q is assigned and cannot be removed", assignment.ProfileKey)
		}
		if storedSettingProfileScope(profile) != assignment.TargetType {
			return fmt.Errorf("profile %q scope cannot change while assigned to a %s", assignment.ProfileKey, assignment.TargetType)
		}
		disabled, _ := profile["disabled"].(bool)
		if disabled || strings.TrimSpace(stringFromUnknown(profile["deletedAt"])) != "" {
			return fmt.Errorf("profile %q is assigned and cannot be disabled or deleted", assignment.ProfileKey)
		}
	}
	return nil
}
