package handlers

import (
	"net/http"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/profiletemplates"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

type ProfileTemplateCloneInput struct {
	Name            string `json:"name" binding:"required"`
	Scope           string `json:"scope"`
	TemplateVersion int    `json:"templateVersion" binding:"required"`
}

func (h ProfileHandler) ListTemplates(c *gin.Context) {
	c.JSON(http.StatusOK, profiletemplates.All())
}

func (h ProfileHandler) CloneTemplate(c *gin.Context) {
	var input ProfileTemplateCloneInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile name is required"})
		return
	}

	template, ok := profiletemplates.Get(c.Param("key"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile template not found"})
		return
	}
	if input.TemplateVersion != template.Version {
		c.JSON(http.StatusConflict, gin.H{"error": "profile template version is no longer available"})
		return
	}
	scope := strings.ToLower(strings.TrimSpace(input.Scope))
	if scope == "" {
		scope = "asset"
	}
	if scope != "asset" && scope != "path" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile scope must be asset or path"})
		return
	}

	profile := template.Definition.NewProfile(input.Name, scope, template.Description)
	profile.TemplateKey = template.Key
	profile.TemplateVersion = template.Version
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
