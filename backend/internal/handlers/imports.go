package handlers

import (
	"net/http"

	"github.com/anuelvs/mediaforge/backend/internal/migrations"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ImportHandler struct {
	db *gorm.DB
}

func NewImportHandler(db *gorm.DB) ImportHandler {
	return ImportHandler{db: db}
}

func (h ImportHandler) ImportMWP(c *gin.Context) {
	summary, err := migrations.ImportMWP(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, summary)
}
