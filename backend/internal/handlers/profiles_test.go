package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProfileUpdateRestoresSoftDeletedRecordWithoutDuplicateName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-restore?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{
		Name: "Castle", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy",
		QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{}, ProfileVersion: 1,
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&profile).Error; err != nil {
		t.Fatal(err)
	}

	payload := ProfileInput{
		Name: "Castle", Description: "edited", Container: "mkv", VideoCodec: "x265",
		AudioCodec: "copy", QualityMode: "crf", QualityValue: 19, WorkerConfig: models.JSONMap{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(profile.ID), 10)}}
	context.Request = httptest.NewRequest(http.MethodPost, "/api/profiles/"+strconv.FormatUint(uint64(profile.ID), 10), bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	NewProfileHandler(db).Update(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected restored profile update, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var restored models.Profile
	if err := db.First(&restored, profile.ID).Error; err != nil {
		t.Fatalf("profile was not restored: %v", err)
	}
	if restored.Description != "edited" || restored.QualityValue != 19 || restored.ProfileVersion != 2 {
		t.Fatalf("restored profile did not receive edits: %#v", restored)
	}
	var count int64
	if err := db.Unscoped().Where("name = ?", "Castle").Model(&models.Profile{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one profile row, got %d", count)
	}
}
