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
)

func TestCompatiblePreviewRequestPreservesLargeProfileOutsideURI(t *testing.T) {
	largeEvidence := strings.Repeat("sampled restoration provenance,", 700)
	input := compatiblePreviewRequest{
		Path: "/media/raw/movies/DVD/movie.mkv", Start: "00:12:03", Seconds: 20, Mode: "quality", PreviewNormalization: "normalize_bt709",
		Profile: &models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
			"videoFilters": "deblock=filter=strong:block=8,hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024,exposure=exposure=0.12,eq=brightness=0:contrast=1:saturation=0.96:gamma=0.94",
			"upscaleMode":  "auto", "upscaleSharpen": "custom", "upscaleSharpenCustomStrength": 0.16,
			"fieldStructureMode": "deinterlace", "deinterlaceFieldOrder": "bff", "restorationRecommendationProvenance": largeEvidence,
		}},
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < 16*1024 {
		t.Fatalf("test profile is not large enough to reproduce proxy URI risk: %d bytes", len(encoded))
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/preview/compatible/requests", NewAssetHandler(nil).CreateCompatiblePreviewRequest)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/preview/compatible/requests", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.RequestID) != 64 || strings.Contains(request.RequestURI, "profile") || len(request.RequestURI) > 256 {
		t.Fatalf("large profile leaked into URI: id=%q uri=%q", created.RequestID, request.RequestURI)
	}
	stored, ok := loadCompatiblePreviewRequest(created.RequestID, time.Now())
	if !ok || stored.Profile == nil {
		t.Fatal("stored canonical preview request was not found")
	}
	worker := stored.Profile.WorkerConfig
	if workerStringValue(worker["videoFilters"]) == "" || jsonFloat64(worker["upscaleSharpenCustomStrength"]) != 0.16 || workerStringValue(worker["deinterlaceFieldOrder"]) != "bff" || workerStringValue(worker["restorationRecommendationProvenance"]) != largeEvidence {
		t.Fatalf("profile body was truncated or changed: %#v", worker)
	}
}

func TestCompatiblePreviewRequestIdentityIsDeterministicAndSensitive(t *testing.T) {
	now := time.Now()
	base := compatiblePreviewRequest{Path: "/media/raw/test.mkv", Start: "5", Seconds: 20, Mode: "quality", Profile: &models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"exposure": 0.12}}}
	first, _, err := storeCompatiblePreviewRequest(base, now)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := storeCompatiblePreviewRequest(base, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent request identity changed: %s != %s", first, second)
	}
	changed := base
	changed.Profile = &models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"exposure": 0.13}}
	third, _, err := storeCompatiblePreviewRequest(changed, now)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("meaningful profile change collided with existing preview request")
	}
	autoID, normalizedAuto, err := storeCompatiblePreviewRequest(compatiblePreviewRequest{Path: base.Path}, now)
	if err != nil {
		t.Fatal(err)
	}
	enabledID, normalizedEnabled, err := storeCompatiblePreviewRequest(compatiblePreviewRequest{Path: base.Path, QSVMBBRCMode: "enabled"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if normalizedAuto.QSVMBBRCMode != "auto" || normalizedEnabled.QSVMBBRCMode != "enabled" || autoID == enabledID {
		t.Fatalf("MBBRC request identity was not normalized/sensitive: auto=%q enabled=%q autoID=%q enabledID=%q", normalizedAuto.QSVMBBRCMode, normalizedEnabled.QSVMBBRCMode, autoID, enabledID)
	}
	loaded, ok := loadCompatiblePreviewRequest(enabledID, now)
	if !ok || loaded.QSVMBBRCMode != "enabled" {
		t.Fatalf("stored MBBRC request did not round trip: ok=%t request=%#v", ok, loaded)
	}
}

func TestCompatiblePreviewRequestNormalizesAndParsesMBBRCMode(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{{input: "", want: "auto"}, {input: "enabled", want: "enabled"}, {input: "disabled", want: "disabled"}, {input: "unknown", want: "auto"}} {
		normalized, err := normalizeCompatiblePreviewRequest(compatiblePreviewRequest{Path: "/media/raw/test.mkv", QSVMBBRCMode: test.input})
		if err != nil {
			t.Fatal(err)
		}
		if normalized.QSVMBBRCMode != test.want {
			t.Fatalf("qsvMBBRCMode %q normalized to %q, want %q", test.input, normalized.QSVMBBRCMode, test.want)
		}
	}

	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/preview?path=%2Fmedia%2Fraw%2Ftest.mkv&qsvMBBRCMode=enabled", nil)
	parsed, err := compatiblePreviewRequestFromQuery(context)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.QSVMBBRCMode != "enabled" {
		t.Fatalf("query qsvMBBRCMode=%q, want enabled", parsed.QSVMBBRCMode)
	}
}

func TestCompatiblePreviewRequestExpiresWithoutPersistentState(t *testing.T) {
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.Unlock()
	now := time.Now()
	id, _, err := storeCompatiblePreviewRequest(compatiblePreviewRequest{Path: "/media/raw/test.mkv"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loadCompatiblePreviewRequest(id, now.Add(compatiblePreviewRequestRetention+time.Second)); ok {
		t.Fatal("expired preview request remained available")
	}
}
