package handlers

import (
	"net/http"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
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
	state, err := runtimeinfo.Current(h.db)
	if err == gorm.ErrRecordNotFound {
		state, _, err = runtimeinfo.DetectAndUpsert(h.db)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, liveRuntimeResponse(state, nil))
}

func (h RuntimeHandler) Refresh(c *gin.Context) {
	capabilities.ResetEncoderCache()
	state, transitionID, err := runtimeinfo.DetectAndUpsert(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, liveRuntimeResponse(state, transitionID))
}

func (h RuntimeHandler) Profiles(c *gin.Context) {
	detected := "desktop_safe"
	if state, err := runtimeinfo.Current(h.db); err == nil && state.RecommendedProfile != "" {
		detected = state.RecommendedProfile
	}
	effective, err := runtimeinfo.ResolveEffectiveRuntimePolicy(h.db, detected)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RuntimeProfilesResponse{Profiles: runtimeinfo.OfficialRuntimeProfiles(), Effective: effective})
}

func liveRuntimeResponse(state models.RuntimeState, transitionID *uint) models.RuntimeState {
	state.StateKind = "live"
	state.AuditSnapshotID = transitionID
	return state
}
