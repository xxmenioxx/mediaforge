package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProfileHandler struct {
	db *gorm.DB
}

type ProfileInput struct {
	Name               string         `json:"name" binding:"required"`
	Scope              string         `json:"scope"`
	Description        string         `json:"description"`
	Container          string         `json:"container" binding:"required"`
	VideoCodec         string         `json:"videoCodec" binding:"required"`
	CodecFamily        string         `json:"codecFamily"`
	EncoderPolicy      string         `json:"encoderPolicy"`
	PreferredEncoder   string         `json:"preferredEncoder"`
	AllowedEncoders    []string       `json:"allowedEncoders"`
	FallbackPolicy     string         `json:"fallbackPolicy"`
	BitDepth           int            `json:"bitDepth"`
	PixelFormat        string         `json:"pixelFormat"`
	QualityStrategy    string         `json:"qualityStrategy"`
	OptimizationIntent string         `json:"optimizationIntent"`
	AudioCodec         string         `json:"audioCodec" binding:"required"`
	QualityMode        string         `json:"qualityMode" binding:"required"`
	QualityValue       int            `json:"qualityValue" binding:"required"`
	PreserveHDR        bool           `json:"preserveHdr"`
	PreserveSubtitles  bool           `json:"preserveSubtitles"`
	PreserveChapters   bool           `json:"preserveChapters"`
	WorkerConfig       models.JSONMap `json:"workerConfig"`
	Disabled           bool           `json:"disabled"`
}

type ProfileDisableInput struct {
	Disabled bool `json:"disabled"`
}

func NewProfileHandler(db *gorm.DB) ProfileHandler {
	return ProfileHandler{db: db}
}

func (h ProfileHandler) List(c *gin.Context) {
	var profiles []models.Profile
	query := h.db
	if c.Query("includeDeleted") == "true" {
		query = query.Unscoped()
	}
	if c.Query("includeDisabled") != "true" {
		query = query.Where("disabled = ? OR disabled IS NULL", false)
	}

	if err := query.Order("name asc").Find(&profiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for index := range profiles {
		if strings.TrimSpace(profiles[index].Scope) == "" {
			profiles[index].Scope = "asset"
			_ = h.db.Model(&profiles[index]).Update("scope", profiles[index].Scope).Error
		}
	}

	c.JSON(http.StatusOK, profiles)
}

func (h ProfileHandler) Create(c *gin.Context) {
	var input ProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.WorkerConfig == nil {
		input.WorkerConfig = models.JSONMap{}
	}
	input.Scope = normalizedProfileScope(input.Scope)
	delete(input.WorkerConfig, "processingMode")
	if _, err := parseUpscaleRequest(input.WorkerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	profile := models.Profile{
		Name:               input.Name,
		Scope:              input.Scope,
		Description:        input.Description,
		Container:          input.Container,
		VideoCodec:         input.VideoCodec,
		CodecFamily:        input.CodecFamily,
		EncoderPolicy:      input.EncoderPolicy,
		PreferredEncoder:   input.PreferredEncoder,
		AllowedEncoders:    models.StringList(input.AllowedEncoders),
		FallbackPolicy:     input.FallbackPolicy,
		BitDepth:           input.BitDepth,
		PixelFormat:        input.PixelFormat,
		QualityStrategy:    input.QualityStrategy,
		OptimizationIntent: normalizedOptimizationIntent(input.OptimizationIntent),
		ProfileVersion:     1,
		AudioCodec:         input.AudioCodec,
		QualityMode:        input.QualityMode,
		QualityValue:       input.QualityValue,
		PreserveHDR:        input.PreserveHDR,
		PreserveSubtitles:  input.PreserveSubtitles,
		PreserveChapters:   input.PreserveChapters,
		WorkerConfig:       input.WorkerConfig,
		Disabled:           input.Disabled,
	}
	if err := scheduler.ApplyAuthoritativeContract(&profile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Create(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, profile)
}

func (h ProfileHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile id"})
		return
	}

	var input ProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.WorkerConfig == nil {
		input.WorkerConfig = models.JSONMap{}
	}
	delete(input.WorkerConfig, "processingMode")
	if _, err := parseUpscaleRequest(input.WorkerConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var profile models.Profile
	if err := h.db.Unscoped().First(&profile, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			if nameErr := h.db.Unscoped().Where("LOWER(name) = LOWER(?)", input.Name).First(&profile).Error; nameErr != nil {
				if nameErr == gorm.ErrRecordNotFound {
					c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": nameErr.Error()})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if strings.TrimSpace(input.Scope) == "" {
		input.Scope = normalizedStoredProfileScope(profile.Scope)
	} else {
		input.Scope = normalizedProfileScope(input.Scope)
	}

	profile.Name = input.Name
	if input.Scope != normalizedStoredProfileScope(profile.Scope) && h.db.Migrator().HasTable(&models.ProfileAssignment{}) {
		var incompatible int64
		if h.db.Model(&models.ProfileAssignment{}).Where("media_type = ? AND selection = ? AND video_profile_id = ? AND target_type <> ?", "video", "profile", profile.ID, input.Scope).Count(&incompatible).Error == nil && incompatible > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "profile scope cannot change while it has assignments of the other scope"})
			return
		}
	}
	profile.Scope = input.Scope
	profile.Description = input.Description
	profile.Container = input.Container
	profile.VideoCodec = input.VideoCodec
	profile.CodecFamily = input.CodecFamily
	profile.EncoderPolicy = input.EncoderPolicy
	profile.PreferredEncoder = input.PreferredEncoder
	profile.AllowedEncoders = models.StringList(input.AllowedEncoders)
	profile.FallbackPolicy = input.FallbackPolicy
	profile.BitDepth = input.BitDepth
	profile.PixelFormat = input.PixelFormat
	profile.QualityStrategy = input.QualityStrategy
	profile.OptimizationIntent = normalizedOptimizationIntent(input.OptimizationIntent)
	profile.AudioCodec = input.AudioCodec
	profile.QualityMode = input.QualityMode
	profile.QualityValue = input.QualityValue
	profile.PreserveHDR = input.PreserveHDR
	profile.PreserveSubtitles = input.PreserveSubtitles
	profile.PreserveChapters = input.PreserveChapters
	profile.WorkerConfig = input.WorkerConfig
	profile.Disabled = input.Disabled
	if input.Disabled && h.videoProfileIsAssigned(profile.ID) {
		c.JSON(http.StatusConflict, gin.H{"error": "profile is assigned and cannot be disabled"})
		return
	}
	profile.DeletedAt = gorm.DeletedAt{}
	profile.ProfileVersion = max(profile.ProfileVersion, 1) + 1
	if err := scheduler.ApplyAuthoritativeContract(&profile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Unscoped().Save(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, profile)
}

func normalizedProfileScope(value string) string {
	if value == "path" {
		return "path"
	}
	return "asset"
}

func normalizedStoredProfileScope(value string) string {
	if strings.TrimSpace(value) == "" {
		return "asset"
	}
	return normalizedProfileScope(value)
}

func normalizedOptimizationIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "maximum_savings", "balanced", "conservative", "maximum_quality", "archive":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func (h ProfileHandler) SetDisabled(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile id"})
		return
	}

	var input ProfileDisableInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var profile models.Profile
	if err := h.db.First(&profile, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if input.Disabled && h.videoProfileIsAssigned(profile.ID) {
		c.JSON(http.StatusConflict, gin.H{"error": "profile is assigned and cannot be disabled"})
		return
	}

	profile.Disabled = input.Disabled
	if err := h.db.Save(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, profile)
}

func (h ProfileHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile id"})
		return
	}

	var profile models.Profile
	if err := h.db.First(&profile, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.videoProfileIsAssigned(profile.ID) {
		c.JSON(http.StatusConflict, gin.H{"error": "profile is assigned and cannot be deleted"})
		return
	}

	profile.Disabled = true
	if err := h.db.Save(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Delete(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": profile.ID})
}

func (h ProfileHandler) videoProfileIsAssigned(profileID uint) bool {
	if !h.db.Migrator().HasTable(&models.ProfileAssignment{}) {
		return false
	}
	var count int64
	return h.db.Model(&models.ProfileAssignment{}).
		Where("media_type = ? AND selection = ? AND video_profile_id = ?", "video", "profile", profileID).
		Count(&count).Error == nil && count > 0
}
