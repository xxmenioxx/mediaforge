package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSettingsUpdateUpsertsExistingKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:settings-upsert?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "trackProfilePathAssignments", Value: models.JSONMap{"assignments": models.JSONMap{"old": "profile"}}}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/trackProfilePathAssignments", strings.NewReader(`{"value":{"assignments":{"new":"profile"}}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var count int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "trackProfilePathAssignments").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings count=%d", count)
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "trackProfilePathAssignments").Error; err != nil {
		t.Fatal(err)
	}
	assignments, ok := setting.Value["assignments"].(map[string]any)
	if !ok || assignments["new"] != "profile" {
		t.Fatalf("unexpected value: %#v", setting.Value)
	}
}
