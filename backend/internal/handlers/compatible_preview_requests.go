package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	compatiblePreviewRequestRetention = 2 * time.Hour
	compatiblePreviewRequestMaxItems  = 500
	compatiblePreviewRequestMaxBody   = 2 << 20
)

type compatiblePreviewRequest struct {
	Path                             string          `json:"path"`
	ProfileID                        uint            `json:"profileId,omitempty"`
	Profile                          *models.Profile `json:"profile,omitempty"`
	Start                            string          `json:"start,omitempty"`
	Seconds                          int             `json:"seconds,omitempty"`
	VideoCodec                       string          `json:"videoCodec,omitempty"`
	QualityValue                     float64         `json:"qualityValue,omitempty"`
	VideoPreset                      string          `json:"videoPreset,omitempty"`
	PixelFormat                      string          `json:"pixFmt,omitempty"`
	VideoFilters                     string          `json:"videoFilters,omitempty"`
	X265Params                       string          `json:"x265Params,omitempty"`
	VideoEncoder                     string          `json:"videoEncoder,omitempty"`
	UseHardware                      bool            `json:"useHardwareIfAvailable,omitempty"`
	HardwareQualityPreset            string          `json:"hardwareQualityPreset,omitempty"`
	VideoToolboxBitrateMbps          float64         `json:"videoToolboxBitrateMbps,omitempty"`
	VideoToolboxMaxrateMbps          float64         `json:"videoToolboxMaxrateMbps,omitempty"`
	VideoToolboxBufferMbps           float64         `json:"videoToolboxBufferMbps,omitempty"`
	VideoToolboxQualityProfile       int             `json:"videoToolboxQualityProfile,omitempty"`
	VideoToolboxProfile              string          `json:"videoToolboxProfile,omitempty"`
	VideoToolboxGOP                  int             `json:"videoToolboxGop,omitempty"`
	VideoToolboxRealtime             bool            `json:"videoToolboxRealtime,omitempty"`
	VideoToolboxBFramePolicy         string          `json:"videoToolboxBFramePolicy,omitempty"`
	VideoToolboxBFrames              int             `json:"videoToolboxBFrames,omitempty"`
	VideoToolboxAutoAdjustBitrate    bool            `json:"videoToolboxAutoAdjustBitrate,omitempty"`
	VideoToolboxAllowFrameReordering bool            `json:"videoToolboxAllowFrameReordering,omitempty"`
	VideoToolboxPowerEfficiency      bool            `json:"videoToolboxPowerEfficiency,omitempty"`
	GlobalQuality                    int             `json:"globalQuality,omitempty"`
	QSVRateControl                   string          `json:"qsvRateControl,omitempty"`
	QSVLookAheadDepth                int             `json:"qsvLookAheadDepth,omitempty"`
	QSVExtendedBRC                   bool            `json:"qsvExtendedBRC,omitempty"`
	QSVAdaptiveI                     bool            `json:"qsvAdaptiveI,omitempty"`
	QSVAdaptiveB                     bool            `json:"qsvAdaptiveB,omitempty"`
	QSVPStrategy                     int             `json:"qsvPStrategy,omitempty"`
	Mode                             string          `json:"mode,omitempty"`
	PreviewNormalization             string          `json:"previewNormalization,omitempty"`
	SubtitleStreamIndex              *int            `json:"subtitleStreamIndex,omitempty"`
	Ephemeral                        bool            `json:"ephemeral,omitempty"`
}

type compatiblePreviewRequestEntry struct {
	Request    compatiblePreviewRequest
	CreatedAt  time.Time
	AccessedAt time.Time
}

var compatiblePreviewRequests = struct {
	sync.Mutex
	items map[string]compatiblePreviewRequestEntry
}{items: map[string]compatiblePreviewRequestEntry{}}

func normalizeCompatiblePreviewRequest(input compatiblePreviewRequest) (compatiblePreviewRequest, error) {
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return input, fmt.Errorf("path is required")
	}
	if input.Start == "" {
		input.Start = "00:00:00"
	}
	start, ok := boundedPreviewStart(input.Start)
	if !ok {
		return input, fmt.Errorf("start must be HH:MM:SS or seconds")
	}
	input.Start = start
	if input.Seconds == 0 {
		input.Seconds = 20
	} else {
		input.Seconds = boundedPreviewSeconds(strconv.Itoa(input.Seconds))
	}
	input.Mode = normalizedPreviewMode(input.Mode)
	input.PreviewNormalization = normalizedPreviewNormalizationMode(input.PreviewNormalization)
	if input.VideoEncoder == "" {
		input.VideoEncoder = "auto"
	}
	if input.GlobalQuality == 0 {
		input.GlobalQuality = 25
	}
	if input.QSVRateControl == "" {
		input.QSVRateControl = "icq"
	}
	if input.QSVLookAheadDepth == 0 {
		input.QSVLookAheadDepth = 40
	}
	input.QSVPStrategy = min(2, max(0, input.QSVPStrategy))
	if input.SubtitleStreamIndex != nil && *input.SubtitleStreamIndex < 0 {
		return input, fmt.Errorf("subtitleStreamIndex must be a non-negative stream index")
	}
	return input, nil
}

func compatiblePreviewRequestIdentity(input compatiblePreviewRequest) (string, error) {
	normalized, err := normalizeCompatiblePreviewRequest(input)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode compatible preview request: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func storeCompatiblePreviewRequest(input compatiblePreviewRequest, now time.Time) (string, compatiblePreviewRequest, error) {
	normalized, err := normalizeCompatiblePreviewRequest(input)
	if err != nil {
		return "", input, err
	}
	id, err := compatiblePreviewRequestIdentity(normalized)
	if err != nil {
		return "", input, err
	}
	compatiblePreviewRequests.Lock()
	defer compatiblePreviewRequests.Unlock()
	cleanupCompatiblePreviewRequestsLocked(now)
	entry, exists := compatiblePreviewRequests.items[id]
	if !exists {
		entry = compatiblePreviewRequestEntry{Request: normalized, CreatedAt: now}
	}
	entry.AccessedAt = now
	compatiblePreviewRequests.items[id] = entry
	trimCompatiblePreviewRequestsLocked()
	return id, normalized, nil
}

func loadCompatiblePreviewRequest(id string, now time.Time) (compatiblePreviewRequest, bool) {
	compatiblePreviewRequests.Lock()
	defer compatiblePreviewRequests.Unlock()
	cleanupCompatiblePreviewRequestsLocked(now)
	entry, ok := compatiblePreviewRequests.items[strings.TrimSpace(id)]
	if !ok {
		return compatiblePreviewRequest{}, false
	}
	entry.AccessedAt = now
	compatiblePreviewRequests.items[id] = entry
	return entry.Request, true
}

func cleanupCompatiblePreviewRequestsLocked(now time.Time) {
	for id, entry := range compatiblePreviewRequests.items {
		if now.Sub(entry.AccessedAt) > compatiblePreviewRequestRetention {
			delete(compatiblePreviewRequests.items, id)
		}
	}
}

func trimCompatiblePreviewRequestsLocked() {
	if len(compatiblePreviewRequests.items) <= compatiblePreviewRequestMaxItems {
		return
	}
	type candidate struct {
		id       string
		accessed time.Time
	}
	candidates := make([]candidate, 0, len(compatiblePreviewRequests.items))
	for id, entry := range compatiblePreviewRequests.items {
		candidates = append(candidates, candidate{id: id, accessed: entry.AccessedAt})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].accessed.Before(candidates[j].accessed) })
	for _, item := range candidates[:len(candidates)-compatiblePreviewRequestMaxItems] {
		delete(compatiblePreviewRequests.items, item.id)
	}
}

func (h AssetHandler) CreateCompatiblePreviewRequest(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, compatiblePreviewRequestMaxBody)
	var input compatiblePreviewRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "compatible preview request must contain valid JSON: " + err.Error()})
		return
	}
	id, normalized, err := storeCompatiblePreviewRequest(input, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"requestId":        id,
		"cacheIdentity":    id,
		"expiresInSeconds": int(compatiblePreviewRequestRetention.Seconds()),
		"path":             normalized.Path,
	})
}

func (h AssetHandler) compatiblePreviewRequest(c *gin.Context, operation string) {
	input, ok := loadCompatiblePreviewRequest(c.Param("requestId"), time.Now())
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "compatible preview request expired or was not found"})
		return
	}
	h.serveCompatiblePreview(c, input, operation)
}

func (h AssetHandler) CompatiblePreviewRequestVideo(c *gin.Context) {
	h.compatiblePreviewRequest(c, "video")
}

func (h AssetHandler) CompatiblePreviewRequestFrame(c *gin.Context) {
	frame := strings.ToLower(strings.TrimSpace(c.Query("frame")))
	if frame != "source" && frame != "output" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "frame must be source or output"})
		return
	}
	h.compatiblePreviewRequest(c, frame+"_frame")
}

func (h AssetHandler) CompatiblePreviewRequestInspection(c *gin.Context) {
	h.compatiblePreviewRequest(c, "inspect")
}

func (h AssetHandler) CompatiblePreviewRequestMetrics(c *gin.Context) {
	h.compatiblePreviewRequest(c, "metrics")
}

func compatiblePreviewRequestFromQuery(c *gin.Context) (compatiblePreviewRequest, error) {
	input := compatiblePreviewRequest{
		Path: c.Query("path"), Start: c.Query("start"), VideoCodec: c.Query("videoCodec"), VideoPreset: c.Query("videoPreset"),
		PixelFormat: c.Query("pixFmt"), VideoFilters: c.Query("videoFilters"), X265Params: c.Query("x265Params"), VideoEncoder: c.Query("videoEncoder"),
		QSVRateControl: c.Query("qsvRateControl"), Mode: c.Query("mode"), PreviewNormalization: c.Query("previewNormalization"),
	}
	profileID, _ := strconv.ParseUint(c.Query("profileId"), 10, 32)
	input.ProfileID = uint(profileID)
	input.Seconds, _ = strconv.Atoi(c.Query("seconds"))
	input.QualityValue, _ = strconv.ParseFloat(c.Query("qualityValue"), 64)
	input.UseHardware, _ = strconv.ParseBool(c.Query("useHardwareIfAvailable"))
	input.GlobalQuality, _ = strconv.Atoi(c.Query("globalQuality"))
	input.QSVLookAheadDepth, _ = strconv.Atoi(c.Query("qsvLookAheadDepth"))
	input.QSVExtendedBRC, _ = strconv.ParseBool(c.Query("qsvExtendedBRC"))
	input.QSVAdaptiveI, _ = strconv.ParseBool(c.Query("qsvAdaptiveI"))
	input.QSVAdaptiveB, _ = strconv.ParseBool(c.Query("qsvAdaptiveB"))
	input.QSVPStrategy, _ = strconv.Atoi(c.Query("qsvPStrategy"))
	input.Ephemeral, _ = strconv.ParseBool(c.Query("ephemeral"))
	if raw := strings.TrimSpace(c.Query("subtitleStreamIndex")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return input, fmt.Errorf("subtitleStreamIndex must be a non-negative stream index")
		}
		input.SubtitleStreamIndex = &value
	}
	if raw := strings.TrimSpace(c.Query("profile")); raw != "" {
		var profile models.Profile
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			return input, fmt.Errorf("profile must contain valid JSON: %w", err)
		}
		input.Profile = &profile
	}
	return normalizeCompatiblePreviewRequest(input)
}
