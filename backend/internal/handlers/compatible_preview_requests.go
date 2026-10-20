package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	compatiblePreviewRequestRetention = 2 * time.Hour
	compatiblePreviewRequestMaxItems  = 500
	compatiblePreviewRequestMaxBody   = 2 << 20
)

type compatiblePreviewRequest struct {
	Path                             string          `json:"path"`
	PreflightCandidateID             string          `json:"preflightCandidateId,omitempty"`
	PreflightRevision                uint            `json:"preflightRevision,omitempty"`
	PreflightAssetID                 string          `json:"preflightAssetId,omitempty"`
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
	QSVMBBRCMode                     string          `json:"qsvMBBRCMode,omitempty"`
	QSVRDOMode                       string          `json:"qsvRDOMode,omitempty"`
	QSVPStrategy                     int             `json:"qsvPStrategy,omitempty"`
	Mode                             string          `json:"mode,omitempty"`
	PreviewNormalization             string          `json:"previewNormalization,omitempty"`
	SubtitleStreamIndex              *int            `json:"subtitleStreamIndex,omitempty"`
	Ephemeral                        bool            `json:"ephemeral,omitempty"`
}

type compatiblePreviewRequestEntry struct {
	mu                sync.RWMutex
	prepareMu         sync.Mutex
	Request           compatiblePreviewRequest
	Descriptor        *compatiblePreviewDescriptor
	CreatedAt         time.Time
	AccessedAt        time.Time
	ExpiresAt         time.Time
	DurableAccessedAt time.Time
	persistenceDB     *gorm.DB
	recoveryState     string
	recoveryReason    string
}

var compatiblePreviewRequests = struct {
	sync.Mutex
	items      map[string]*compatiblePreviewRequestEntry
	tombstones map[string]compatiblePreviewRequestTombstone
}{items: map[string]*compatiblePreviewRequestEntry{}, tombstones: map[string]compatiblePreviewRequestTombstone{}}

type compatiblePreviewPendingTombstone struct {
	db        *gorm.DB
	tombstone compatiblePreviewRequestTombstone
}

func persistCompatiblePreviewPendingTombstones(pending []compatiblePreviewPendingTombstone) {
	for _, item := range pending {
		persistCompatiblePreviewRequestTombstone(item.db, item.tombstone)
	}
}

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
	input.QSVMBBRCMode = normalizedQSVMBBRCMode(input.QSVMBBRCMode)
	input.QSVRDOMode = normalizedQSVRDOMode(input.QSVRDOMode)
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
	if normalized.Ephemeral {
		scope := make([]byte, 16)
		if _, err := rand.Read(scope); err != nil {
			return "", fmt.Errorf("create ephemeral compatible preview request scope: %w", err)
		}
		encoded = append(encoded, 0)
		encoded = append(encoded, scope...)
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
	pending := cleanupCompatiblePreviewRequestsLocked(now)
	entry, exists := compatiblePreviewRequests.items[id]
	if !exists {
		delete(compatiblePreviewRequests.tombstones, id)
		entry = &compatiblePreviewRequestEntry{Request: normalized, CreatedAt: now, ExpiresAt: now.Add(compatiblePreviewRequestRetention)}
		compatiblePreviewRequests.items[id] = entry
	}
	compatiblePreviewRequests.Unlock()
	persistCompatiblePreviewPendingTombstones(pending)

	entry.mu.Lock()
	if !exists {
		entry.Request = normalized
		entry.CreatedAt = now
		entry.ExpiresAt = now.Add(compatiblePreviewRequestRetention)
	}
	entry.AccessedAt = now
	entry.mu.Unlock()
	compatiblePreviewRequests.Lock()
	trimPending := trimCompatiblePreviewRequestsLocked()
	compatiblePreviewRequests.Unlock()
	persistCompatiblePreviewPendingTombstones(trimPending)

	return id, normalized, nil
}

func newCompatiblePreviewRequestEntry(request compatiblePreviewRequest, now time.Time, db *gorm.DB) *compatiblePreviewRequestEntry {
	return &compatiblePreviewRequestEntry{
		Request:           request,
		CreatedAt:         now,
		AccessedAt:        now,
		ExpiresAt:         now.Add(compatiblePreviewRequestRetention),
		DurableAccessedAt: now,
		persistenceDB:     db,
	}
}

func installCompatiblePreviewRequestEntry(id string, entry *compatiblePreviewRequestEntry) {
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items[id] = entry
	delete(compatiblePreviewRequests.tombstones, id)
	pending := trimCompatiblePreviewRequestsLocked()
	compatiblePreviewRequests.Unlock()
	persistCompatiblePreviewPendingTombstones(pending)
}

func loadCompatiblePreviewRequest(id string, now time.Time) (compatiblePreviewRequest, bool) {
	entry, ok := loadCompatiblePreviewRequestEntry(id, now)
	if !ok {
		return compatiblePreviewRequest{}, false
	}
	entry.mu.RLock()
	request := entry.Request
	entry.mu.RUnlock()
	return request, true
}

func loadCompatiblePreviewRequestEntry(id string, now time.Time) (*compatiblePreviewRequestEntry, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false
	}
	compatiblePreviewRequests.Lock()
	pending := cleanupCompatiblePreviewRequestsLocked(now)
	entry, ok := compatiblePreviewRequests.items[id]
	compatiblePreviewRequests.Unlock()
	persistCompatiblePreviewPendingTombstones(pending)
	if !ok {
		return nil, false
	}
	entry.mu.Lock()
	if !entry.ExpiresAt.After(now) {
		entry.mu.Unlock()
		return nil, false
	}
	entry.AccessedAt = now
	entry.ExpiresAt = now.Add(compatiblePreviewRequestRetention)
	shouldTouch := entry.persistenceDB != nil && (entry.DurableAccessedAt.IsZero() || now.Sub(entry.DurableAccessedAt) >= compatiblePreviewDurableTouchInterval)
	if shouldTouch {
		entry.DurableAccessedAt = now
	}
	db := entry.persistenceDB
	entry.mu.Unlock()
	if shouldTouch {
		persistCompatiblePreviewRequestTouch(db, id, now, now.Add(compatiblePreviewRequestRetention))
	}
	return entry, true
}

func cleanupCompatiblePreviewRequestsLocked(now time.Time) []compatiblePreviewPendingTombstone {
	var pending []compatiblePreviewPendingTombstone
	for id, entry := range compatiblePreviewRequests.items {
		entry.mu.RLock()
		expiresAt := entry.ExpiresAt
		if expiresAt.IsZero() {
			expiresAt = entry.AccessedAt.Add(compatiblePreviewRequestRetention)
		}
		db := entry.persistenceDB
		entry.mu.RUnlock()
		if !expiresAt.After(now) {
			delete(compatiblePreviewRequests.items, id)
			tombstone := compatiblePreviewRequestTombstone{RequestID: id, ExpiresAt: expiresAt, TombstoneExpiresAt: now.Add(compatiblePreviewTombstoneRetention), Reason: "request expired"}
			compatiblePreviewRequests.tombstones[id] = tombstone
			pending = append(pending, compatiblePreviewPendingTombstone{db: db, tombstone: tombstone})
		}
	}
	for id, tombstone := range compatiblePreviewRequests.tombstones {
		if !tombstone.TombstoneExpiresAt.After(now) {
			delete(compatiblePreviewRequests.tombstones, id)
		}
	}
	return pending
}

func trimCompatiblePreviewRequestsLocked() []compatiblePreviewPendingTombstone {
	var pending []compatiblePreviewPendingTombstone
	if len(compatiblePreviewRequests.items) <= compatiblePreviewRequestMaxItems {
		return pending
	}
	type candidate struct {
		id       string
		accessed time.Time
	}
	candidates := make([]candidate, 0, len(compatiblePreviewRequests.items))
	for id, entry := range compatiblePreviewRequests.items {
		entry.mu.RLock()
		accessedAt := entry.AccessedAt
		entry.mu.RUnlock()
		candidates = append(candidates, candidate{id: id, accessed: accessedAt})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].accessed.Before(candidates[j].accessed) })
	for _, item := range candidates[:len(candidates)-compatiblePreviewRequestMaxItems] {
		if entry := compatiblePreviewRequests.items[item.id]; entry != nil {
			entry.mu.RLock()
			expiresAt := entry.ExpiresAt
			db := entry.persistenceDB
			entry.mu.RUnlock()
			delete(compatiblePreviewRequests.items, item.id)
			tombstone := compatiblePreviewRequestTombstone{RequestID: item.id, ExpiresAt: expiresAt, TombstoneExpiresAt: time.Now().Add(compatiblePreviewTombstoneRetention), Reason: "request retention bound reached"}
			compatiblePreviewRequests.tombstones[item.id] = tombstone
			pending = append(pending, compatiblePreviewPendingTombstone{db: db, tombstone: tombstone})
		}
	}
	trimCompatiblePreviewTombstonesLocked()
	return pending
}

func trimCompatiblePreviewTombstonesLocked() {
	if len(compatiblePreviewRequests.tombstones) <= compatiblePreviewTombstoneMaxItems {
		return
	}
	type candidate struct {
		id      string
		expires time.Time
	}
	candidates := make([]candidate, 0, len(compatiblePreviewRequests.tombstones))
	for id, tombstone := range compatiblePreviewRequests.tombstones {
		candidates = append(candidates, candidate{id: id, expires: tombstone.TombstoneExpiresAt})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].expires.Before(candidates[j].expires) })
	for _, item := range candidates[:len(candidates)-compatiblePreviewTombstoneMaxItems] {
		delete(compatiblePreviewRequests.tombstones, item.id)
	}
}

func compatiblePreviewRequestLookup(id string, now time.Time) (*compatiblePreviewRequestEntry, *compatiblePreviewRequestTombstone, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, false
	}
	compatiblePreviewRequests.Lock()
	pending := cleanupCompatiblePreviewRequestsLocked(now)
	entry := compatiblePreviewRequests.items[id]
	tombstone, tombstoneOK := compatiblePreviewRequests.tombstones[id]
	compatiblePreviewRequests.Unlock()
	persistCompatiblePreviewPendingTombstones(pending)
	if entry != nil {
		entry.mu.Lock()
		entry.AccessedAt = now
		entry.ExpiresAt = now.Add(compatiblePreviewRequestRetention)
		shouldTouch := entry.persistenceDB != nil && (entry.DurableAccessedAt.IsZero() || now.Sub(entry.DurableAccessedAt) >= compatiblePreviewDurableTouchInterval)
		if shouldTouch {
			entry.DurableAccessedAt = now
		}
		db := entry.persistenceDB
		entry.mu.Unlock()
		if shouldTouch {
			persistCompatiblePreviewRequestTouch(db, id, now, now.Add(compatiblePreviewRequestRetention))
		}
		return entry, nil, true
	}
	if tombstoneOK {
		return nil, &tombstone, true
	}
	return nil, nil, false
}

func (h AssetHandler) CreateCompatiblePreviewRequest(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, compatiblePreviewRequestMaxBody)
	var input compatiblePreviewRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "compatible preview request must contain valid JSON: " + err.Error()})
		return
	}
	if strings.TrimSpace(input.PreflightCandidateID) != "" {
		if err := resolveCompatiblePreviewPreflight(h.db, &input); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "batch_preflight_context_stale"})
			return
		}
	}
	now := time.Now()
	normalized, err := normalizeCompatiblePreviewRequest(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := compatiblePreviewRequestIdentity(normalized)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entry, tombstone, known := compatiblePreviewRequestLookup(id, now)
	var descriptor *compatiblePreviewDescriptor
	var installEntry *compatiblePreviewRequestEntry
	if known && entry != nil {
		entry.mu.RLock()
		recoveryState := entry.recoveryState
		currentDescriptor := entry.Descriptor
		entry.mu.RUnlock()
		if recoveryState == "" && currentDescriptor != nil && currentDescriptor.frozen && !currentDescriptor.SourceRevision.matches(currentDescriptor.Path) {
			entry.mu.Lock()
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = "source revision changed after preview preparation"
			db := entry.persistenceDB
			entry.mu.Unlock()
			persistCompatiblePreviewRequestStale(db, id, "source revision changed after preview preparation")
			recoveryState = string(compatiblePreviewArtifactStale)
		}
		if recoveryState == "" {
			entry.mu.Lock()
			entry.persistenceDB = h.db
			entry.mu.Unlock()
			descriptor, err = h.compatiblePreviewDescriptorForEntry(entry, id)
			if err == nil {
				installEntry = entry
			}
		}
	}
	if installEntry == nil {
		// Prepare a replacement off-registry. The previous active/recovery entry or
		// tombstone remains addressable until preparation and persistence succeed.
		candidate := newCompatiblePreviewRequestEntry(normalized, now, h.db)
		descriptor, err = h.compatiblePreviewDescriptorForEntry(candidate, id)
		if err != nil {
			var preparationErr *compatiblePreviewPrepareError
			if errors.As(err, &preparationErr) {
				c.JSON(preparationErr.status, compatiblePreviewPreparePayload(preparationErr))
			} else if lifecycleErr, ok := err.(*compatiblePreviewLifecycleError); ok {
				writeCompatiblePreviewError(c, lifecycleErr)
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			}
			return
		}
		installEntry = candidate
	}

	if descriptor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "compatible preview descriptor is unavailable"})
		return
	}
	descriptor.frozen = h.db != nil && descriptor.RetentionPolicy != compatiblePreviewEphemeralRetention
	if err := persistCompatiblePreviewPreparedRequest(h.db, id, normalized, descriptor, now); err != nil {
		// Do not remove the old entry/tombstone. A failed recreate remains a
		// deterministic lifecycle response instead of becoming unknown.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist compatible preview request: " + err.Error()})
		return
	}
	descriptor.frozen = h.db != nil && descriptor.RetentionPolicy != compatiblePreviewEphemeralRetention
	if installEntry != entry || tombstone != nil || !known {
		installCompatiblePreviewRequestEntry(id, installEntry)
	}
	c.JSON(http.StatusCreated, gin.H{
		"requestId":        id,
		"cacheIdentity":    descriptor.ArtifactID,
		"expiresInSeconds": int(compatiblePreviewRequestRetention.Seconds()),
		"path":             normalized.Path,
	})
}

func resolveCompatiblePreviewPreflight(db *gorm.DB, input *compatiblePreviewRequest) error {
	if db == nil || input == nil {
		return fmt.Errorf("Batch Preflight preview context is unavailable")
	}
	candidateID, assetID := strings.TrimSpace(input.PreflightCandidateID), strings.TrimSpace(input.PreflightAssetID)
	if candidateID == "" || assetID == "" || input.PreflightRevision == 0 {
		return fmt.Errorf("Batch Preflight preview requires candidate, revision, and asset identity")
	}
	var candidate models.QueueBatchPreflight
	if err := db.First(&candidate, "id = ?", candidateID).Error; err != nil {
		return fmt.Errorf("load Batch Preflight candidate: %w", err)
	}
	if candidate.Revision != input.PreflightRevision || !candidate.Resolved {
		return fmt.Errorf("Batch Preflight candidate revision is stale or unresolved")
	}
	state, err := queuePreflightStateFromRecord(candidate)
	if err != nil {
		return fmt.Errorf("decode Batch Preflight candidate: %w", err)
	}
	for _, asset := range state.Assets {
		if asset.ID != assetID {
			continue
		}
		if asset.ResolvedJob == nil || (asset.Status != queuePreflightReady && asset.Status != queuePreflightQueued) {
			return fmt.Errorf("candidate asset has no approved resolved plan")
		}
		if filepath.Clean(asset.MediaPath) != filepath.Clean(input.Path) || filepath.Clean(asset.ResolvedJob.MediaPath) != filepath.Clean(input.Path) {
			return fmt.Errorf("candidate asset identity does not match preview source")
		}
		currentRevision, revisionErr := queuePreflightSourceRevision(input.Path)
		if revisionErr != nil || currentRevision != asset.SourceRevision {
			return fmt.Errorf("source identity changed after Batch Preflight")
		}
		profile, err := scheduler.RestoreProfileSnapshot(asset.ResolvedJob.ProfileSnapshot)
		if err != nil {
			return fmt.Errorf("restore candidate profile snapshot: %w", err)
		}
		profile = applySelectedEncoder(profile, asset.SelectedEncoder)
		input.Profile, input.ProfileID = &profile, 0
		return nil
	}
	return fmt.Errorf("candidate asset identity was not found")
}

func (h AssetHandler) compatiblePreviewRequest(c *gin.Context, operation string) {
	requestID := strings.TrimSpace(c.Param("requestId"))
	entry, tombstone, ok := compatiblePreviewRequestLookup(requestID, time.Now())
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "compatible preview request is unknown", "code": "unknown_request", "state": "unknown"})
		return
	}
	if tombstone != nil {
		writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(compatiblePreviewRequestStateExpiredTombstone, tombstone.Reason))
		return
	}
	entry.mu.RLock()
	recoveryState, recoveryReason := entry.recoveryState, entry.recoveryReason
	entry.mu.RUnlock()
	if recoveryState != "" {
		writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(recoveryState, recoveryReason))
		return
	}
	descriptor, err := h.compatiblePreviewDescriptorForEntry(entry, requestID)
	if err != nil {
		writeCompatiblePreviewError(c, err)
		return
	}
	h.servePreparedCompatiblePreview(c, descriptor, operation)
}

func deleteCompatiblePreviewRequest(id string, expected *compatiblePreviewRequestEntry) {
	compatiblePreviewRequests.Lock()
	if current, ok := compatiblePreviewRequests.items[strings.TrimSpace(id)]; ok && current == expected {
		delete(compatiblePreviewRequests.items, strings.TrimSpace(id))
	}
	compatiblePreviewRequests.Unlock()
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
		QSVRateControl: c.Query("qsvRateControl"), QSVMBBRCMode: c.Query("qsvMBBRCMode"), QSVRDOMode: c.Query("qsvRDOMode"), Mode: c.Query("mode"), PreviewNormalization: c.Query("previewNormalization"),
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
