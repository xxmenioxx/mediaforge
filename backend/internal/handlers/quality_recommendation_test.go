package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestQualityRecommendationUsesActiveRuntimeQSVFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:quality-recommendation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RuntimeSnapshot{}); err != nil {
		t.Fatal(err)
	}
	encoders := models.JSONMap{"hevc_qsv": models.JSONMap{
		"listed": true, "usable": true, "main10": true,
		"qsvIcqMain8": false, "qsvCqpMain8": true,
		"qsvIcqMain10": false, "qsvLaIcqMain10": false, "qsvCqpMain10": true,
		"testedModes": models.JSONMap{"qsvCqpMain8": true, "qsvCqpMain10": true},
	}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), Encoders: encoders}).Error; err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{
		Name: "QSV recommendation", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
		BitDepth: 10, PixelFormat: "p010le",
		WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "hardwareQualityPresetScale": 2, "qsvRateControl": "la_icq", "pixFmt": "p010le"},
	}
	payload, _ := json.Marshal(qualityRecommendationInput{Profile: profile})
	request := httptest.NewRequest(http.MethodPost, "/api/assets/quality-recommendation", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	NewAssetHandler(db).QualityRecommendation(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response qualityRecommendationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CapabilitySource != "active_runtime_snapshot" || response.Recommendation.EffectiveRateControl != "cqp" {
		t.Fatalf("unexpected recommendation: %#v", response)
	}
	command := strings.Join(response.FFmpegVideoArguments, " ")
	if !strings.Contains(command, "-global_quality 28") || !strings.Contains(command, "-flags +qscale") || strings.Contains(command, "-look_ahead") {
		t.Fatalf("unexpected effective arguments: %s", command)
	}
}
