package handlers

import (
	"net/http"
	"strconv"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LibraryHandler struct {
	db *gorm.DB
}

type LibraryInput struct {
	Name             string         `json:"name" binding:"required"`
	SourcePath       string         `json:"sourcePath"`
	DestinationPath  string         `json:"destinationPath" binding:"required"`
	Type             string         `json:"type" binding:"required"`
	ValidationRules  models.JSONMap `json:"validationRules"`
	DefaultProfileID *uint          `json:"defaultProfileId"`
}

func NewLibraryHandler(db *gorm.DB) LibraryHandler {
	return LibraryHandler{db: db}
}

func (h LibraryHandler) List(c *gin.Context) {
	var libraries []models.Library
	if err := h.db.Order("created_at desc").Find(&libraries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, libraries)
}

func (h LibraryHandler) Create(c *gin.Context) {
	var input LibraryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.ValidationRules == nil {
		input.ValidationRules = models.JSONMap{}
	}
	sourcePath, err := h.sourcePath(input.SourcePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	library := models.Library{
		Name:             input.Name,
		SourcePath:       sourcePath,
		DestinationPath:  input.DestinationPath,
		Type:             input.Type,
		ValidationRules:  input.ValidationRules,
		DefaultProfileID: input.DefaultProfileID,
	}

	if err := h.db.Create(&library).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, library)
}

func (h LibraryHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid library id"})
		return
	}

	var input LibraryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.ValidationRules == nil {
		input.ValidationRules = models.JSONMap{}
	}
	sourcePath, err := h.sourcePath(input.SourcePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var library models.Library
	if err := h.db.First(&library, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	library.Name = input.Name
	library.SourcePath = sourcePath
	library.DestinationPath = input.DestinationPath
	library.Type = input.Type
	library.ValidationRules = input.ValidationRules
	library.DefaultProfileID = input.DefaultProfileID

	if err := h.db.Save(&library).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, library)
}

func (h LibraryHandler) sourcePath(sourcePath string) (string, error) {
	if sourcePath != "" {
		return sourcePath, nil
	}
	return settingPath(h.db, "rawRoot", "/media/raw")
}
