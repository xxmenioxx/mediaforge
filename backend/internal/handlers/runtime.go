package handlers

import (
	"net/http"

	"github.com/anuelvs/mediaforge/backend/internal/capabilities"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type RuntimeHandler struct{ db *gorm.DB }

type RuntimeProfilesResponse struct {
	Profiles  []runtimeinfo.RuntimeProfileDefinition `json:"profiles"`
	Effective runtimeinfo.EffectiveRuntimePolicy     `json:"effective"`
}

func NewRuntimeHandler(db *gorm.DB) RuntimeHandler { return RuntimeHandler{db: db} }

func (h RuntimeHandler) Latest(c *gin.Context) {
	snapshot, err := runtimeinfo.Latest(h.db)
	if err == gorm.ErrRecordNotFound {
		snapshot, err = runtimeinfo.DetectAndSave(h.db)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h RuntimeHandler) Refresh(c *gin.Context) {
	capabilities.ResetEncoderCache()
	snapshot, err := runtimeinfo.DetectAndSave(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h RuntimeHandler) Profiles(c *gin.Context) {
	detected := "desktop_safe"
	if snapshot, err := runtimeinfo.Latest(h.db); err == nil && snapshot.RecommendedProfile != "" {
		detected = snapshot.RecommendedProfile
	}
	effective, err := runtimeinfo.ResolveEffectiveRuntimePolicy(h.db, detected)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RuntimeProfilesResponse{Profiles: runtimeinfo.OfficialRuntimeProfiles(), Effective: effective})
}
