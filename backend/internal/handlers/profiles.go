package handlers

import (
	"net/http"
	"strconv"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProfileHandler struct {
	db *gorm.DB
}

type ProfileInput struct {
	Name              string         `json:"name" binding:"required"`
	Description       string         `json:"description"`
	Container         string         `json:"container" binding:"required"`
	VideoCodec        string         `json:"videoCodec" binding:"required"`
	AudioCodec        string         `json:"audioCodec" binding:"required"`
	QualityMode       string         `json:"qualityMode" binding:"required"`
	QualityValue      int            `json:"qualityValue" binding:"required"`
	PreserveHDR       bool           `json:"preserveHdr"`
	PreserveSubtitles bool           `json:"preserveSubtitles"`
	PreserveChapters  bool           `json:"preserveChapters"`
	WorkerConfig      models.JSONMap `json:"workerConfig"`
	Disabled          bool           `json:"disabled"`
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

	profile := models.Profile{
		Name:              input.Name,
		Description:       input.Description,
		Container:         input.Container,
		VideoCodec:        input.VideoCodec,
		AudioCodec:        input.AudioCodec,
		QualityMode:       input.QualityMode,
		QualityValue:      input.QualityValue,
		PreserveHDR:       input.PreserveHDR,
		PreserveSubtitles: input.PreserveSubtitles,
		PreserveChapters:  input.PreserveChapters,
		WorkerConfig:      input.WorkerConfig,
		Disabled:          input.Disabled,
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

	var profile models.Profile
	if err := h.db.First(&profile, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	profile.Name = input.Name
	profile.Description = input.Description
	profile.Container = input.Container
	profile.VideoCodec = input.VideoCodec
	profile.AudioCodec = input.AudioCodec
	profile.QualityMode = input.QualityMode
	profile.QualityValue = input.QualityValue
	profile.PreserveHDR = input.PreserveHDR
	profile.PreserveSubtitles = input.PreserveSubtitles
	profile.PreserveChapters = input.PreserveChapters
	profile.WorkerConfig = input.WorkerConfig
	profile.Disabled = input.Disabled

	if err := h.db.Save(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, profile)
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
