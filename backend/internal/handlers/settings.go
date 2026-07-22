package handlers

import (
	"net/http"
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
