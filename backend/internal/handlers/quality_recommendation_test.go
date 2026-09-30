package handlers

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGOPEditDoesNotChangeUnmeasuredSizeEstimate(t *testing.T) {
	intent := quality.NewIntent(quality.IntentInput{SourceVideoBitrate: 4_000_000, DurationSeconds: 1440})
	base := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"frameStructureGopMode": "recommended", "frameStructureGopFrames": 72}}
	long := base
	long.WorkerConfig = cloneJSONMap(base.WorkerConfig)
	long.WorkerConfig["frameStructureGopFrames"] = 180
	shortEstimate := genericEncoderRecommendation(base, intent, "libx265")
	longEstimate := genericEncoderRecommendation(long, intent, "libx265")
	if shortEstimate.EstimatedOutputSizeMin == nil || longEstimate.EstimatedOutputSizeMin == nil || *shortEstimate.EstimatedOutputSizeMin != *longEstimate.EstimatedOutputSizeMin || *shortEstimate.EstimatedOutputSizeMax != *longEstimate.EstimatedOutputSizeMax {
		t.Fatalf("GOP duration must not invent an unmeasured storage-saving multiplier: short=%#v long=%#v", shortEstimate, longEstimate)
	}
}

func TestBPPPFRecommendationUsesEffectiveOutputValuesWithoutChangingProfile(t *testing.T) {
	target := int64(1_000_000)
	profile := models.Profile{
		VideoCodec: "x265", CodecFamily: "hevc", QualityValue: 18, OptimizationIntent: "balanced",
		WorkerConfig: models.JSONMap{
			"effectiveOutputWidth": 1280, "effectiveOutputHeight": 720,
			"effectiveOutputFrameRate": "24000/1001", "videoEncoder": "libx265",
		},
	}
	intent := quality.NewIntent(quality.IntentInput{SourceWidth: 3840, SourceHeight: 2160, SourceVideoBitrate: 20_000_000})
	recommendation := quality.EncoderRecommendation{TargetBitrate: &target}
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 3840, Height: 2160, FrameRate: "60/1"}}}
	assessment := bpppfAssessmentForRecommendation(profile, "libx265", intent, recommendation, streams)
	want := float64(target) / (1280 * 720 * (24000.0 / 1001.0))
	if assessment.OutputWidth != 1280 || assessment.OutputHeight != 720 || math.Abs(assessment.EffectiveFPS-24000.0/1001.0) > 0.0001 || math.Abs(assessment.BPPPF-want) > 0.000001 {
		t.Fatalf("rail used source instead of effective output values: %#v want bpppf %.6f", assessment, want)
	}
	if profile.QualityValue != 18 || profile.WorkerConfig["effectiveOutputWidth"] != 1280 {
		t.Fatalf("rail changed the selected profile: %#v", profile)
	}
}

func TestQualityRecommendationUsesActiveRuntimeQSVFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:quality-recommendation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RuntimeState{}); err != nil {
		t.Fatal(err)
	}
	encoders := models.JSONMap{"hevc_qsv": models.JSONMap{
		"listed": true, "usable": true, "main10": true,
		"qsvIcqMain8": false, "qsvCqpMain8": true,
		"qsvIcqMain10": false, "qsvLaIcqMain10": false, "qsvCqpMain10": true,
		"testedModes": models.JSONMap{"qsvCqpMain8": true, "qsvCqpMain10": true},
	}}
	if err := db.Create(&models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), Encoders: encoders, StableFingerprint: "quality-test", FingerprintVersion: "v1"}).Error; err != nil {
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
	if response.CapabilitySource != "active_runtime_state" || response.Recommendation.EffectiveRateControl != "cqp" {
		t.Fatalf("unexpected recommendation: %#v", response)
	}
	command := strings.Join(response.FFmpegVideoArguments, " ")
	if !strings.Contains(command, "-global_quality 28") || !strings.Contains(command, "-flags +qscale") || strings.Contains(command, "-look_ahead") {
		t.Fatalf("unexpected effective arguments: %s", command)
	}
}
