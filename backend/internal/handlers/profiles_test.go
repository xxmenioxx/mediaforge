package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestProfileUpdateWithoutScopePreservesPathScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-scope-preserve?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.ProfileAssignment{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{Name: "Path", Scope: "path", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{}, ProfileVersion: 1}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	payload := ProfileInput{Name: profile.Name, Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 19, WorkerConfig: models.JSONMap{}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(profile.ID), 10)}}
	context.Request = httptest.NewRequest(http.MethodPost, "/api/profiles/1", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	NewProfileHandler(db).Update(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := db.First(&profile, profile.ID).Error; err != nil {
		t.Fatal(err)
	}
	if profile.Scope != "path" {
		t.Fatalf("omitted scope changed path profile to %q", profile.Scope)
	}
}

func TestProfileCreateRejectsInvalidUpscaleRequest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-invalid-upscale?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	payload := ProfileInput{
		Name: "Invalid upscale", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy",
		QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"upscaleMode": "custom", "upscaleCustomHeight": 721},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/profiles", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	NewProfileHandler(db).Create(context)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "positive even height") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProfileCreateStripsResolvedRestorationPlan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-strip-restoration-plan?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	payload := ProfileInput{
		Name: "Restoration intent only", Container: "mkv", VideoCodec: "hevc", AudioCodec: "copy",
		QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{
			"videoFilters":             "hqdn3d=4:3:6:4.5",
			resolvedRestorationPlanKey: ResolvedRestorationPlan{Version: 1, ResolvedFilterChain: "hqdn3d=1:1:1:1"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/profiles", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	NewProfileHandler(db).Create(context)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var profile models.Profile
	if err := db.Where("name = ?", payload.Name).First(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if _, exists := profile.WorkerConfig[resolvedRestorationPlanKey]; exists {
		t.Fatalf("asset-specific restoration plan persisted in normal profile: %#v", profile.WorkerConfig)
	}
	if profile.WorkerConfig["videoFilters"] != "hqdn3d=4:3:6:4.5" {
		t.Fatalf("restoration request was changed: %#v", profile.WorkerConfig)
	}
}
