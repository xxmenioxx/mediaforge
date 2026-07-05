package handlers

import (
	"net/http"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	var setting models.AppSetting
	if err := h.db.First(&setting, "key = ?", key).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		setting = models.AppSetting{Key: key}
	}

	setting.Value = input.Value
	if setting.CreatedAt.IsZero() {
		setting.CreatedAt = time.Now()
	}

	if err := h.db.Save(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, setting)
}
