package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRuntimeGetReturnsLiveStateWithoutAuditIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:runtime-live-api?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RuntimeState{}, &models.RuntimeSnapshot{}); err != nil {
		t.Fatal(err)
	}
	state := models.RuntimeState{
		SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(),
		OS: "linux", Architecture: "amd64", StableFingerprint: "runtime-stable-v1:test", FingerprintVersion: "v1",
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/system/runtime", NewRuntimeHandler(db).Latest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/system/runtime", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["stateKind"] != "live" || body["stableFingerprint"] != state.StableFingerprint {
		t.Fatalf("unexpected live provenance: %#v", body)
	}
	if _, exists := body["auditSnapshotId"]; exists {
		t.Fatalf("normal live GET exposed audit identity: %#v", body)
	}
	if id, ok := body["id"].(float64); !ok || uint(id) != state.ID {
		t.Fatalf("live technical ID missing or changed: %#v", body["id"])
	}
	var snapshots int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&snapshots).Error; err != nil {
		t.Fatal(err)
	}
	if snapshots != 0 {
		t.Fatalf("GET created %d audit snapshots", snapshots)
	}
}
