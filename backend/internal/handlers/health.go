package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"service":   "mediaforge-api",
		"timestamp": time.Now().UTC(),
	})
}
