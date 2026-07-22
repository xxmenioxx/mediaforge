package handlers

import (
	"net/http"
	"strconv"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ExecutionPlanHandler struct {
	db *gorm.DB
}

func (h ExecutionPlanHandler) Approve(c *gin.Context) { h.review(c, true) }

func (h ExecutionPlanHandler) Reject(c *gin.Context) { h.review(c, false) }

func (h ExecutionPlanHandler) review(c *gin.Context, approve bool) {
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	planID, err := strconv.ParseUint(c.Param("planId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id"})
		return
	}
	plan, err := scheduler.SetPlanApproval(h.db, uint(jobID), uint(planID), approve)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func NewExecutionPlanHandler(db *gorm.DB) ExecutionPlanHandler {
	return ExecutionPlanHandler{db: db}
}

func (h ExecutionPlanHandler) ListForJob(c *gin.Context) {
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	var count int64
	if err := h.db.Model(&models.QueueJob{}).Where("id = ?", uint(jobID)).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	var plans []models.ExecutionPlan
	if err := h.db.Where("job_id = ?", uint(jobID)).Order("version desc").Find(&plans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plans)
}
