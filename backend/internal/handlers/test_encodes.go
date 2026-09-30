package handlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	legacyTestEncodeOwnerType     = "test_encode"
	testEncodeWaiting             = "waiting"
	testEncodeRunning             = "generating"
	testEncodeReady               = "ready"
	testEncodeFailed              = "failed"
	testEncodeCanceled            = "canceled"
	testEncodeDeleted             = "deleted"
	testEncodeFFmpegCommandColumn = "ffmpeg_command"
)

type testEncodeRequest struct {
	SourcePath           string                       `json:"sourcePath" binding:"required"`
	LibraryID            uint                         `json:"libraryId" binding:"required"`
	ConfigurationSource  string                       `json:"configurationSource" binding:"required"`
	ProfileID            uint                         `json:"profileId"`
	AudioProfileKey      string                       `json:"audioProfileKey"`
	TrackProfileKey      string                       `json:"trackProfileKey"`
	ProcessingMode       string                       `json:"processingMode"`
	ResolveAssignments   bool                         `json:"resolveAssignments"`
	LabProfile           *models.Profile              `json:"labProfile"`
	LabAudioProfile      models.JSONMap               `json:"labAudioProfile"`
	LabTrackProfile      models.JSONMap               `json:"labTrackProfile"`
	LabTrackOverride     AssetConversionOverrideState `json:"labTrackOverride"`
	StartMode            string                       `json:"startMode"`
	StartSeconds         float64                      `json:"startSeconds"`
	DurationSeconds      int                          `json:"durationSeconds"`
	ConfigurationToken   string                       `json:"configurationToken"`
	PreflightCandidateID string                       `json:"preflightCandidateId"`
	PreflightRevision    uint                         `json:"preflightRevision"`
	PreflightAssetID     string                       `json:"preflightAssetId"`
}

type resolvedTestEncode struct {
	job             models.QueueJob
	profile         models.Profile
	audio           *audioEnhancementProfile
	override        AssetConversionOverrideState
	library         models.Library
	requested       models.JSONMap
	subtitleStyling SubtitleStylePolicy
}

var testEncodeProcesses = struct {
	sync.Mutex
	cancel map[uint]context.CancelFunc
}{cancel: map[uint]context.CancelFunc{}}

func recoverInterruptedTestEncodes(db *gorm.DB) {
	if db == nil || !db.Migrator().HasTable(&models.TestEncode{}) {
		return
	}
	var interrupted []models.TestEncode
	_ = db.Where("status IN ?", []string{testEncodeWaiting, testEncodeRunning}).Find(&interrupted).Error
	for _, test := range interrupted {
		now := time.Now()
		updates := map[string]any{
			"status": testEncodeFailed, "phase": "interrupted", "progress": 0,
			"error_message": "Test Encode was interrupted by application restart", "completed_at": &now, "updated_at": now,
		}
		if !test.Keep {
			expires := now.Add(time.Duration(testEncodePolicyFromSettings(db).FailedRetentionHours) * time.Hour)
			updates["expires_at"] = &expires
		}
		if _, cleanupErr := (AssetHandler{db: db}).removeTestEncodeFilesWithEvidence(test); cleanupErr != nil {
			updates["reconciliation_status"] = "cleanup_failed"
			updates["reconciliation_message"] = cleanupErr.Error()
		}
		_ = db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Updates(updates).Error
	}
	// Remove reservations created by older releases. They are legacy data and
	// must never affect scheduler capacity after Test Encode was decoupled.
	if db.Migrator().HasTable(&models.TaskReservation{}) {
		_ = db.Where("owner_type = ?", legacyTestEncodeOwnerType).Delete(&models.TaskReservation{}).Error
	}
}

func (h AssetHandler) ListTestEncodes(c *gin.Context) {
	h.listTestEncodeInventory(c)
}

func (h AssetHandler) GetTestEncode(c *gin.Context) {
	test, ok := h.testEncodeFromParam(c)
	if !ok {
		return
	}
	h.decorateTestEncode(&test)
	c.JSON(http.StatusOK, test)
}

func (h AssetHandler) CreateTestEncode(c *gin.Context) {
	var input testEncodeRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resolved, err := h.resolveTestEncodeRequest(input)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	info, err := os.Stat(resolved.job.MediaPath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "source asset is not readable"})
		return
	}
	fingerprint, err := mediaFileFingerprint(resolved.job.MediaPath)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "fingerprint source: " + err.Error()})
		return
	}
	start, duration := resolveTestEncodeWindow(h.db, resolved.job.MediaPath, input.StartMode, input.StartSeconds, input.DurationSeconds)
	sourceDuration := testEncodeCanonicalSourceDuration(h.db, resolved.job.MediaPath, start, duration)
	plan := h.samplingPlanForFrozenWindow(
		resolved.job.MediaPath, fingerprint, fingerprint, sourceDuration, float64(start), float64(duration), samplingPlanPolicyVersion,
	)
	if resolved.requested == nil {
		resolved.requested = models.JSONMap{}
	}
	resolved.requested["samplingPlan"] = samplingPlanMap(plan)
	resolved.requested["samplingPlanIdentity"] = plan.PlanIdentity
	now := time.Now()
	modified := info.ModTime()
	policy := testEncodePolicyFromSettings(h.db)
	keep := policy.DefaultRetention == "keep"
	var expiresAt *time.Time
	if !keep {
		expires := now.Add(time.Duration(policy.TemporaryRetentionHours) * time.Hour)
		expiresAt = &expires
	}
	test := models.TestEncode{
		SourcePath: resolved.job.MediaPath, SourceFingerprint: fingerprint, SourceSizeBytes: info.Size(), SourceModifiedAt: &modified,
		LibraryID: resolved.library.ID, ConfigurationSource: normalizedTestEncodeConfigurationSource(input.ConfigurationSource),
		RequestedConfiguration: resolved.requested, ProfileID: resolved.job.ProfileID, ProfileVersion: resolved.job.ProfileVersion,
		StartSeconds: start, DurationSeconds: duration, Status: testEncodeRunning, Phase: "preparing", Progress: 1,
		StartedAt: &now,
		Keep:      keep, ExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now,
	}
	var record models.AssetRecord
	if h.db.Where("path = ?", resolved.job.MediaPath).Order("updated_at DESC").First(&record).Error == nil {
		test.SourceAssetID = record.ID
	}
	if err := h.db.Create(&test).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	go h.executeTestEncode(test.ID, resolved)
	c.JSON(http.StatusAccepted, test)
}

func (h AssetHandler) CancelTestEncode(c *gin.Context) {
	test, ok := h.testEncodeFromParam(c)
	if !ok {
		return
	}
	if test.Status != testEncodeWaiting && test.Status != testEncodeRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "only waiting or generating tests can be canceled"})
		return
	}
	testEncodeProcesses.Lock()
	cancel := testEncodeProcesses.cancel[test.ID]
	testEncodeProcesses.Unlock()
	if cancel != nil {
		cancel()
	}
	now := time.Now()
	updates := map[string]any{
		"status": testEncodeCanceled, "phase": "canceled", "canceled_at": &now, "updated_at": now,
	}
	if !test.Keep {
		expires := now.Add(time.Duration(testEncodePolicyFromSettings(h.db).FailedRetentionHours) * time.Hour)
		updates["expires_at"] = &expires
	}
	_ = h.db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Updates(updates).Error
	c.JSON(http.StatusAccepted, gin.H{"status": testEncodeCanceled, "id": test.ID})
}

func (h AssetHandler) KeepTestEncode(c *gin.Context) {
	test, ok := h.testEncodeFromParam(c)
	if !ok {
		return
	}
	var input struct {
		Keep bool `json:"keep"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	updates := map[string]any{"keep": input.Keep, "updated_at": now}
	if input.Keep {
		updates["expires_at"] = nil
	} else {
		policy := testEncodePolicyFromSettings(h.db)
		hours := policy.TemporaryRetentionHours
		if test.Status == testEncodeFailed || test.Status == testEncodeCanceled {
			hours = policy.FailedRetentionHours
		}
		expires := now.Add(time.Duration(hours) * time.Hour)
		updates["expires_at"] = &expires
	}
	if err := h.db.Model(&test).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.db.First(&test, test.ID).Error
	c.JSON(http.StatusOK, test)
}

func (h AssetHandler) testEncodeFromParam(c *gin.Context) (models.TestEncode, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid test encode id"})
		return models.TestEncode{}, false
	}
	var test models.TestEncode
	if err := h.db.First(&test, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "test encode not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return models.TestEncode{}, false
	}
	return test, true
}

func (h AssetHandler) resolveTestEncodeRequest(input testEncodeRequest) (resolvedTestEncode, error) {
	path, err := h.resolveMediaPath(input.SourcePath)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	allowed, err := h.pathBelongsToReadableMediaRoot(path)
	if err != nil || !allowed {
		return resolvedTestEncode{}, fmt.Errorf("source path is outside configured readable media roots")
	}
	var library models.Library
	if err := h.db.First(&library, input.LibraryID).Error; err != nil {
		return resolvedTestEncode{}, fmt.Errorf("load destination library: %w", err)
	}
	job := models.QueueJob{
		MediaPath: path, LibraryID: input.LibraryID, ProfileID: input.ProfileID,
		AudioProfileKey: strings.TrimSpace(input.AudioProfileKey), TrackProfileKey: strings.TrimSpace(input.TrackProfileKey),
		ProcessingMode: normalizeQueueProcessingMode(input.ProcessingMode),
	}
	if job.ProcessingMode == "" {
		job.ProcessingMode = ProcessingModeFullEncode
	}
	queue := QueueHandler{db: h.db}
	source := normalizedTestEncodeConfigurationSource(input.ConfigurationSource)
	requested := models.JSONMap{
		"configurationSource": source, "processingMode": job.ProcessingMode,
		"requestedProfileId": input.ProfileID, "requestedAudioProfileKey": input.AudioProfileKey,
		"requestedTrackProfileKey": input.TrackProfileKey, "resolveAssignments": input.ResolveAssignments,
		"configurationToken": strings.TrimSpace(input.ConfigurationToken),
	}
	if source == "batch_preflight" {
		candidateID := strings.TrimSpace(input.PreflightCandidateID)
		assetID := strings.TrimSpace(input.PreflightAssetID)
		if candidateID == "" || assetID == "" || input.PreflightRevision == 0 {
			return resolvedTestEncode{}, fmt.Errorf("batch_preflight requires candidate, revision, and asset identity")
		}
		var candidate models.QueueBatchPreflight
		if err := h.db.First(&candidate, "id = ?", candidateID).Error; err != nil {
			return resolvedTestEncode{}, fmt.Errorf("load Batch Preflight candidate: %w", err)
		}
		if candidate.Revision != input.PreflightRevision || !candidate.Resolved {
			return resolvedTestEncode{}, fmt.Errorf("Batch Preflight candidate revision is stale or unresolved")
		}
		state, err := queuePreflightStateFromRecord(candidate)
		if err != nil {
			return resolvedTestEncode{}, fmt.Errorf("decode Batch Preflight candidate: %w", err)
		}
		var resolved *queueBatchPreflightAsset
		for index := range state.Assets {
			if state.Assets[index].ID == assetID {
				resolved = &state.Assets[index]
				break
			}
		}
		if resolved == nil || resolved.ResolvedJob == nil || (resolved.Status != queuePreflightReady && resolved.Status != queuePreflightQueued) {
			return resolvedTestEncode{}, fmt.Errorf("candidate asset has no approved resolved plan")
		}
		if filepath.Clean(resolved.MediaPath) != filepath.Clean(path) || filepath.Clean(resolved.ResolvedJob.MediaPath) != filepath.Clean(path) {
			return resolvedTestEncode{}, fmt.Errorf("candidate asset identity does not match source path")
		}
		currentRevision, revisionErr := queuePreflightSourceRevision(path)
		if revisionErr != nil || currentRevision != resolved.SourceRevision {
			return resolvedTestEncode{}, fmt.Errorf("source identity changed after Batch Preflight")
		}
		job = *resolved.ResolvedJob
		if job.LibraryID != input.LibraryID {
			return resolvedTestEncode{}, fmt.Errorf("destination does not match the candidate plan")
		}
		if _, frozen := job.ProfileSnapshot[queuePreparationFrozenSnapshotKey]; !frozen {
			return resolvedTestEncode{}, fmt.Errorf("candidate profile snapshot is not frozen")
		}
		requested["batchPreflightCandidateId"] = candidateID
		requested["batchPreflightRevision"] = input.PreflightRevision
		requested["batchPreflightAssetId"] = assetID
		requested["profileId"] = job.ProfileID
		requested["audioProfileKey"] = job.AudioProfileKey
		requested["trackProfileKey"] = job.TrackProfileKey
		requested["profileResolution"] = job.ProfileResolution
	} else if source == "lab_draft" {
		if input.LabProfile == nil {
			return resolvedTestEncode{}, fmt.Errorf("lab_draft requires labProfile")
		}
		profile := *input.LabProfile
		profile.ID = 0
		if profile.ProfileVersion < 1 {
			profile.ProfileVersion = 1
		}
		now := time.Now()
		snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, "test_encode_lab_draft")
		if err != nil {
			return resolvedTestEncode{}, err
		}
		job.ProfileSnapshot, job.ProfileVersion, job.ProfileCapturedAt = snapshot, profile.ProfileVersion, &now
		job.AudioProfileSnapshot = cloneTestEncodeJSONMap(input.LabAudioProfile)
		if len(job.AudioProfileSnapshot) > 0 {
			job.AudioProfileKey = "lab-draft"
		}
		labTrackOverride, err := resolveLabTrackOverride(h.db, path, input.LabTrackProfile, input.LabTrackOverride)
		if err != nil {
			return resolvedTestEncode{}, err
		}
		if encoded, err := json.Marshal(labTrackOverride); err == nil {
			_ = json.Unmarshal(encoded, &job.TrackProfileSnapshot)
		}
		if len(job.TrackProfileSnapshot) > 0 {
			job.TrackProfileKey = "lab-draft"
		}
		queue.captureInterlaceSnapshot(path, job.ProfileSnapshot)
		requested["labProfile"] = profile
		requested["labAudioProfile"] = input.LabAudioProfile
		requested["labTrackProfile"] = input.LabTrackProfile
		requested["labTrackOverride"] = input.LabTrackOverride
	} else {
		queueInput := QueueJobInput{
			MediaPath: path, LibraryID: input.LibraryID, ProfileID: input.ProfileID,
			AudioProfileKey: input.AudioProfileKey, TrackProfileKey: input.TrackProfileKey,
			ProcessingMode: job.ProcessingMode, ResolveProfileAssignments: input.ResolveAssignments,
		}
		if input.ResolveAssignments {
			resolution, err := queue.resolveProfileAssignments(&queueInput)
			if err != nil {
				return resolvedTestEncode{}, err
			}
			job.ProfileResolution = resolution
		}
		job.ProfileID, job.AudioProfileKey, job.TrackProfileKey = queueInput.ProfileID, queueInput.AudioProfileKey, queueInput.TrackProfileKey
		job.ProcessingMode = normalizeQueueProcessingMode(queueInput.ProcessingMode)
		videoResolution := settingProfileObject(job.ProfileResolution["video"])
		customAssetDefaults := strings.EqualFold(workerStringValue(videoResolution["source"]), assetDefaultsGlobalSource) &&
			strings.EqualFold(workerStringValue(videoResolution["selection"]), configSelectionValue) &&
			len(settingProfileObject(videoResolution["profileData"])) > 0
		if job.ProfileID == 0 && !customAssetDefaults {
			return resolvedTestEncode{}, fmt.Errorf("an effective video profile is required for Test Encode V1")
		}
		if err := queue.captureSupplementalProfiles(&job); err != nil {
			return resolvedTestEncode{}, err
		}
		if err := queue.captureProfile(&job, job.ProfileID, "test_encode_effective_asset"); err != nil {
			return resolvedTestEncode{}, err
		}
		requested["profileId"] = job.ProfileID
		requested["audioProfileKey"] = job.AudioProfileKey
		requested["trackProfileKey"] = job.TrackProfileKey
		requested["profileResolution"] = job.ProfileResolution
	}
	worker := WorkerHandler{db: h.db}
	subtitleStyling := subtitleStylingPolicyFromDB(h.db)
	profile, err := worker.profileForJob(job)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	labTrackOverride, err := resolveLabTrackOverride(h.db, path, input.LabTrackProfile, input.LabTrackOverride)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	override := testEncodeConversionOverride(source, job, labTrackOverride, assetConversionOverrides(h.db))
	profile, err = resolveTestEncodeVideoProfile(h.db, path, job, profile, override)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	audio, err := worker.audioProfileForJob(job)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	requested["sourceConfigurationHash"] = configurationHash(models.JSONMap{
		"profile": profile, "audio": audio, "override": override, "profileResolution": job.ProfileResolution, "subtitleStylePolicyVersion": subtitleStyling.Version,
	})
	requested["subtitleStylePolicyVersion"] = subtitleStyling.Version
	return resolvedTestEncode{job: job, profile: profile, audio: audio, override: override, library: library, requested: requested, subtitleStyling: subtitleStyling}, nil
}

func resolveTestEncodeVideoProfile(db *gorm.DB, path string, job models.QueueJob, profile models.Profile, override AssetConversionOverrideState) (models.Profile, error) {
	if _, frozen := job.ProfileSnapshot[queuePreparationFrozenSnapshotKey]; frozen {
		if selected, ok := job.ProfileSnapshot[models.QueuePreflightSelectedEncoderKey].(string); ok && strings.TrimSpace(selected) != "" {
			profile = applySelectedEncoder(profile, selected)
		}
		profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
		profile.WorkerConfig["qsvAssetAnalysisPath"] = path
		return profile, nil
	}
	profile = applyAssetConversionOverrideToProfile(profile, override)
	profile.WorkerConfig = restorationWorkerConfigForAsset(profile.WorkerConfig, path)
	profile = applyFrozenCadenceResolution(job, profile)
	profile, err := resolveAutomaticFrameStructure(db, path, profile)
	if err != nil {
		return models.Profile{}, err
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["qsvAssetAnalysisPath"] = path
	if frozen, ok := job.ProfileSnapshot[interlaceAnalysisSnapshotKey]; ok {
		profile.WorkerConfig[interlaceAnalysisSnapshotKey] = frozen
	}
	return profile, nil
}

func resolveLabTrackOverride(db *gorm.DB, mediaPath string, profile models.JSONMap, explicit AssetConversionOverrideState) (AssetConversionOverrideState, error) {
	if len(profile) == 0 {
		return explicit, nil
	}
	resolved := resolveTrackProfileForAsset(db, mediaPath, cloneTestEncodeJSONMap(profile))
	markSubtitleTrackSources(resolved, "asset_profile")
	resolved[subtitleTrackSourceKey] = "asset_profile"
	canonical, versionErr := canonicalTrackDispositionProfile(resolved)
	if versionErr != nil {
		return AssetConversionOverrideState{}, fmt.Errorf("LAB track profile: %w", versionErr)
	}
	if !canonical {
		var legacy AssetConversionOverrideState
		if encoded, err := json.Marshal(resolved); err != nil || json.Unmarshal(encoded, &legacy) != nil {
			return AssetConversionOverrideState{}, fmt.Errorf("LAB track profile: serialize legacy override")
		}
		return mergeTrackProfileBelowAssetOverride(legacy, explicit), nil
	}
	var scan models.ScanResult
	if err := db.Where("path = ?", filepath.Clean(mediaPath)).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return AssetConversionOverrideState{}, fmt.Errorf("LAB track profile: asset snapshot: %w", err)
	}
	if len(explicit.SubtitleSidecarFormatsByStream) > 0 {
		resolved["subtitleSidecarFormatsByStream"] = explicit.SubtitleSidecarFormatsByStream
	}
	if len(explicit.SubtitleRoleOverrides) > 0 {
		resolved["subtitleRoleOverrides"] = bindSubtitleRoleOverrides(scan, explicit.SubtitleRoleOverrides)
	}
	resolved = applyPersistedSubtitleSettings(resolved, generateSRTFallbackForASSSetting(db), persistedSubtitleFieldSource(db, generateSRTFallbackForASSKey))
	resolved = applyPersistedSubtitleStylingSettings(resolved, subtitleStylingPolicyFromDB(db), persistedSubtitleFieldSource(db, subtitleStylingPolicyKey))
	trackPlan, err := resolveTrackPlan(scan, resolved)
	if err != nil {
		return AssetConversionOverrideState{}, fmt.Errorf("LAB track profile: resolve track plan: %w", err)
	}
	trackPlanMap, err := resolvedTrackPlanMap(trackPlan)
	if err != nil {
		return AssetConversionOverrideState{}, fmt.Errorf("LAB track profile: serialize track plan: %w", err)
	}
	resolved[resolvedTrackPlanSnapshotKey] = trackPlanMap
	var semantic AssetConversionOverrideState
	if encoded, err := json.Marshal(resolved); err != nil || json.Unmarshal(encoded, &semantic) != nil {
		return AssetConversionOverrideState{}, fmt.Errorf("LAB track profile: serialize resolved override")
	}
	return mergeTrackProfileBelowAssetOverride(semantic, explicit), nil
}

func testEncodeConversionOverride(source string, job models.QueueJob, labOverride AssetConversionOverrideState, entries map[string]AssetConversionOverrideState) AssetConversionOverrideState {
	if source == "batch_preflight" {
		return AssetConversionOverrideState{}
	}
	if source == "lab_draft" {
		// The LAB draft is the authoritative video configuration for this kind of
		// Test Encode. Only the track choices submitted with that same draft may
		// supplement it; persisted asset overrides must not replace unsaved LAB
		// Frame Structure or HEVC Level values.
		return applyJobSelectionsToConversionOverride(job, labOverride)
	}
	return conversionOverrideForJob(job, entries)
}

func (h AssetHandler) decorateTestEncode(test *models.TestEncode) {
	if test == nil || test.DeletedAt != nil {
		return
	}
	if info, err := os.Stat(test.SourcePath); err != nil || info.IsDir() || info.Size() != test.SourceSizeBytes || (test.SourceModifiedAt != nil && !info.ModTime().Equal(*test.SourceModifiedAt)) {
		test.Stale = true
		test.StaleReason = "Source asset changed or is no longer available"
		return
	}
	if test.ConfigurationSource != "effective_asset" || test.Status == testEncodeWaiting {
		return
	}
	requestedHash := strings.TrimSpace(workerStringValue(test.RequestedConfiguration["sourceConfigurationHash"]))
	if requestedHash == "" {
		return
	}
	request := testEncodeRequest{
		SourcePath: test.SourcePath, LibraryID: test.LibraryID,
		ConfigurationSource: "effective_asset",
		ProfileID:           uint(intValueSetting(test.RequestedConfiguration["requestedProfileId"], int(test.ProfileID))),
		AudioProfileKey:     workerStringValue(test.RequestedConfiguration["requestedAudioProfileKey"]),
		TrackProfileKey:     workerStringValue(test.RequestedConfiguration["requestedTrackProfileKey"]),
		ProcessingMode:      workerStringValue(test.RequestedConfiguration["processingMode"]),
		ResolveAssignments:  boolSetting(test.RequestedConfiguration["resolveAssignments"], false),
	}
	resolved, err := h.resolveTestEncodeRequest(request)
	if err != nil {
		test.Stale = true
		test.StaleReason = "Current effective configuration can no longer be resolved"
		return
	}
	currentHash := workerStringValue(resolved.requested["sourceConfigurationHash"])
	if currentHash != requestedHash {
		test.Stale = true
		test.StaleReason = "Configuration changed since this test was generated"
	}
}

func normalizedTestEncodeConfigurationSource(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "lab_draft") {
		return "lab_draft"
	}
	if strings.EqualFold(strings.TrimSpace(value), "batch_preflight") {
		return "batch_preflight"
	}
	return "effective_asset"
}

func cloneTestEncodeJSONMap(value models.JSONMap) models.JSONMap {
	if value == nil {
		return models.JSONMap{}
	}
	encoded, _ := json.Marshal(value)
	result := models.JSONMap{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func resolveTestEncodeWindow(db *gorm.DB, path, mode string, custom float64, requestedDuration int) (float64, int) {
	policy := testEncodePolicyFromSettings(db)
	if requestedDuration == 0 {
		requestedDuration = policy.DefaultDurationSeconds
	}
	duration := min(120, max(5, requestedDuration))
	if strings.TrimSpace(mode) == "" {
		mode = policy.DefaultStartMode
	}
	var scan models.ScanResult
	_ = db.Where("path = ?", filepath.Clean(path)).Order("updated_at DESC").First(&scan).Error
	maxStart := maxFloat(0, scan.Duration-float64(duration))
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "beginning":
		return 0, duration
	case "middle":
		return maxStart / 2, duration
	case "custom":
		return min(maxStart, maxFloat(0, custom)), duration
	default:
		for _, analysis := range []models.JSONMap{scan.InterlaceAnalysis, scan.CropAnalysis} {
			if values := workerSliceValue(analysis["sampledAt"]); len(values) > 0 {
				candidate := workerNumberValue(values[len(values)/2], maxStart*.25)
				return min(maxStart, maxFloat(0, candidate)), duration
			}
		}
		return maxStart * .25, duration
	}
}

func testEncodeCanonicalSourceDuration(db *gorm.DB, path string, start float64, segmentDuration int) float64 {
	var scan models.ScanResult
	if db != nil && db.Where("path = ?", filepath.Clean(path)).Order("updated_at DESC").First(&scan).Error == nil && scan.Duration > 0 {
		return scan.Duration
	}
	if probe := ffprobeJSON(path); probe != nil {
		if duration := validationProbeDuration(probe); duration > 0 {
			return duration
		}
	}
	// An unavailable source duration is still explicit and bounded; it must not
	// make a non-zero segment pretend that the entire asset is that segment.
	return maxFloat(start+float64(segmentDuration), float64(segmentDuration))
}

func testEncodeRetentionHours(db *gorm.DB) int {
	return testEncodePolicyFromSettings(db).TemporaryRetentionHours
}

func (h AssetHandler) executeTestEncode(id uint, resolved resolvedTestEncode) {
	ctx, cancel := context.WithCancel(context.Background())
	testEncodeProcesses.Lock()
	testEncodeProcesses.cancel[id] = cancel
	testEncodeProcesses.Unlock()
	defer func() {
		cancel()
		testEncodeProcesses.Lock()
		delete(testEncodeProcesses.cancel, id)
		testEncodeProcesses.Unlock()
	}()

	var test models.TestEncode
	if h.db.First(&test, id).Error != nil {
		return
	}
	if test.Status != testEncodeRunning {
		return
	}
	planned := plannedTestEncodeOutputPath(h.db, resolved.job, resolved.library, resolved.profile)
	ext := filepath.Ext(planned)
	base := strings.TrimSuffix(filepath.Base(planned), ext)
	output := filepath.Join(filepath.Dir(planned), fmt.Sprintf("%s - MVForge Test T%d%s", base, id, ext))
	// Keep the real container extension last so FFmpeg can infer the muxer while
	// the hidden sibling remains clearly distinguishable from a completed test.
	temporary := filepath.Join(filepath.Dir(output), "."+strings.TrimSuffix(filepath.Base(output), ext)+".partial"+ext)
	if !pathIsInside(output, resolved.library.DestinationPath) {
		h.failTestEncode(id, fmt.Errorf("planned test output is outside the selected Library destination"))
		return
	}
	if _, err := os.Stat(output); err == nil {
		h.failTestEncode(id, fmt.Errorf("planned Test Encode output already exists"))
		return
	} else if !os.IsNotExist(err) {
		h.failTestEncode(id, fmt.Errorf("inspect planned Test Encode output: %w", err))
		return
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		h.failTestEncode(id, err)
		return
	}
	plan, err := buildMediaJobPlanWithOverride(resolved.job.MediaPath, temporary, resolved.profile, resolved.audio, true, resolved.override)
	if err != nil {
		h.failTestEncode(id, err)
		return
	}
	plan.SourceAssetPath = resolved.job.MediaPath
	plan.SubtitleStyling = resolved.subtitleStyling
	applyEpisodeVideoTrackTitle(h.db, &plan, resolved.job, resolved.library)
	plan.SegmentStartSeconds, plan.SegmentDurationSeconds = test.StartSeconds, test.DurationSeconds
	plan.IndependentSubtitleArtifacts = true
	plan.AllowEmptySubtitleArtifacts = true
	plan.OutputMetadata = map[string]string{
		"MVFORGE_TEST": "1", "MVFORGE_TEST_ID": strconv.FormatUint(uint64(id), 10),
		"MVFORGE_SOURCE_ASSET_ID": strconv.FormatUint(uint64(test.SourceAssetID), 10),
		"MVFORGE_PROFILE_ID":      strconv.FormatUint(uint64(test.ProfileID), 10),
		"MVFORGE_CREATED_AT":      test.CreatedAt.UTC().Format(time.RFC3339),
		"title":                   fmt.Sprintf("MVForge Test T%d", id),
	}
	encoder := resolvedVideoEncoder(plan.Profile)
	runtimeSnapshotID, snapshotErr := bindTestEncodeRuntimeSnapshot(h.db, &test)
	if snapshotErr != nil {
		h.failTestEncode(id, snapshotErr)
		return
	}
	executorName := "test-encode-local"

	effective := models.JSONMap{
		"profile": plan.Profile, "override": plan.Override, "streamPlan": resolveEffectiveStreamPlan(plan),
		"effectiveEncoder": encoder, "executor": executorName, "runtimeSnapshotId": runtimeSnapshotID,
	}
	effectiveHash := configurationHash(effective)
	plan.OutputMetadata["MVFORGE_CONFIG_HASH"] = effectiveHash
	args := FFmpegCommandBuilder{}.Build(plan)
	command := "ffmpeg " + shellJoin(args)
	now := time.Now()
	result := h.db.Model(&models.TestEncode{}).Where("id = ? AND status = ?", id, testEncodeRunning).Updates(map[string]any{
		"phase": "generating", "progress": 5,
		"worker_name": executorName, "runtime_snapshot_id": runtimeSnapshotID,
		"effective_encoder": encoder, "effective_configuration": effective, "configuration_hash": effectiveHash,
		// The model pins this column name so partial V1 databases migrate consistently.
		testEncodeFFmpegCommandColumn: command, "output_path": output, "temporary_path": temporary, "updated_at": now,
	})
	if result.Error != nil {
		h.failTestEncode(id, result.Error)
		return
	}
	if result.RowsAffected == 0 || ctx.Err() != nil {
		return
	}
	if err := persistTestEncodeArtifactSet(h.db, id, testEncodeArtifactSet(test, output, temporary, nil, false)); err != nil {
		h.failTestEncode(id, err)
		return
	}

	artifactPlan := plan
	artifactPlan.OutputPath = temporary
	subtitleArtifacts, err := generateSubtitleArtifacts(ctx, artifactPlan)
	if err != nil {
		_ = h.db.Model(&models.TestEncode{}).Where("id = ?", id).Update("subtitle_artifacts", subtitleArtifactsJSON(subtitleArtifacts)).Error
		_ = persistTestEncodeArtifactSet(h.db, id, testEncodeArtifactSet(test, output, temporary, subtitleArtifacts, false))
		h.failTestEncode(id, fmt.Errorf("generate test subtitle sidecars: %w", err))
		return
	}
	externalArtifacts, err := generateTestExternalSubtitleArtifacts(ctx, artifactPlan, subtitleArtifacts)
	if err != nil {
		_ = h.db.Model(&models.TestEncode{}).Where("id = ?", id).Update("subtitle_artifacts", subtitleArtifactsJSON(subtitleArtifacts)).Error
		_ = persistTestEncodeArtifactSet(h.db, id, testEncodeArtifactSet(test, output, temporary, subtitleArtifacts, false))
		h.failTestEncode(id, fmt.Errorf("generate external test subtitle sidecars: %w", err))
		return
	}
	subtitleArtifacts = append(subtitleArtifacts, externalArtifacts...)
	if err := h.db.Model(&models.TestEncode{}).Where("id = ?", id).Update("subtitle_artifacts", subtitleArtifactsJSON(subtitleArtifacts)).Error; err != nil {
		h.failTestEncode(id, err)
		return
	}
	if err := persistTestEncodeArtifactSet(h.db, id, testEncodeArtifactSet(test, output, temporary, subtitleArtifacts, false)); err != nil {
		h.failTestEncode(id, err)
		return
	}
	if err := h.runTestEncodeFFmpeg(ctx, id, args, float64(test.DurationSeconds)); err != nil {
		if ctx.Err() == nil {
			h.failTestEncode(id, err)
		} else {
			var canceled models.TestEncode
			if h.db.First(&canceled, id).Error == nil {
				_, _ = h.removeTestEncodeFilesForOwner(canceled, id)
			}
		}
		return
	}
	if err := os.Rename(temporary, output); err != nil {
		h.failTestEncode(id, fmt.Errorf("activate test output: %w", err))
		return
	}
	if err := activateTestSubtitleArtifacts(subtitleArtifacts, temporary, output); err != nil {
		h.failTestEncode(id, fmt.Errorf("activate test subtitle sidecars: %w", err))
		return
	}
	if err := persistTestEncodeArtifactSet(h.db, id, testEncodeArtifactSet(test, output, "", subtitleArtifacts, true)); err != nil {
		h.failTestEncode(id, err)
		return
	}
	outputInfo, err := os.Stat(output)
	if err != nil || outputInfo.Size() == 0 {
		h.failTestEncode(id, fmt.Errorf("test output validation failed: output is missing or empty"))
		return
	}
	streams, err := probeMediaStreamsWithFrameCount(output, true)
	if err != nil {
		h.failTestEncode(id, fmt.Errorf("probe test output: %w", err))
		return
	}
	sourceTiming, outputTiming := avTimingFromInventory(plan.Streams), avTimingFromInventory(streams)
	if measured, timingErr := probeAVTiming(plan.InputPath, plan.SegmentStartSeconds); timingErr == nil {
		sourceTiming = measured
	}
	if measured, timingErr := probeAVTiming(output, 0); timingErr == nil {
		outputTiming = measured
	}
	report := testEncodeValidationReportWithTiming(plan, streams, test.DurationSeconds, sourceTiming, outputTiming)
	if frozen, ok := samplingMap(test.RequestedConfiguration["samplingPlan"]); ok {
		if samplingPlan, planOK := samplingPlanFromMap(frozen); planOK {
			// Selection provenance is frozen with the Test Encode request and is
			// reported separately from the output validation verdict.
			report["samplingPlanIdentity"] = samplingPlan.PlanIdentity
			report["samplingPolicyVersion"] = samplingPlan.PolicyVersion
			report["samplingWindows"] = samplingPlan.SelectedWindows
			report["samplingFallback"] = samplingPlan.Fallback
			sourceRevision := test.SourceFingerprint
			if currentRevision, revisionErr := mediaFileFingerprint(test.SourcePath); revisionErr == nil && currentRevision != sourceRevision {
				// The output was produced from a different source revision. Keep the
				// validation report, but do not create a comparison pair for stale
				// material.
				report["perceptualEvidenceWarning"] = "source revision changed before perceptual analysis; evidence was not reused"
			} else if candidateRevision, revisionErr := mediaFileFingerprint(output); revisionErr == nil {
				evidence, evidenceErr := collectPerceptualEvidenceUnderHeavySlot(ctx, samplingPlan, resolved.profile, perceptualArtifactInput{
					SourcePath: test.SourcePath, CandidatePath: output, SourceRevision: sourceRevision,
					CandidateRevision: candidateRevision, CandidateKind: "test_encode", CandidateArtifactID: fmt.Sprintf("test-encode:%d", id),
					ConfigurationIdentity: test.ConfigurationHash,
					GenerationToken:       fmt.Sprintf("test-encode:%d", id), GenerationVersion: 1,
					SegmentStartSeconds: test.StartSeconds, SegmentDuration: float64(test.DurationSeconds),
					CandidateDuration: float64(test.DurationSeconds), FullOutput: false,
				})
				if evidenceErr != nil {
					report["perceptualEvidenceWarning"] = evidenceErr.Error()
				} else {
					report["perceptualEvidence"] = evidence
				}
			}
		}
	}
	completed := time.Now()
	artifacts := testEncodeArtifactSet(test, output, "", subtitleArtifacts, true)
	for index := range artifacts {
		if info, statErr := os.Stat(artifacts[index].Path); statErr == nil && !info.IsDir() {
			artifacts[index].SizeBytes = info.Size()
		}
	}
	if err := persistTestEncodeArtifactSet(h.db, id, artifacts); err != nil {
		h.failTestEncode(id, err)
		return
	}
	var readyResult *gorm.DB
	readyResultErr := h.db.Transaction(func(tx *gorm.DB) error {
		var current models.TestEncode
		if err := tx.First(&current, id).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"status": testEncodeReady, "phase": "ready", "progress": 100, "completed_at": &completed,
			"output_size_bytes": outputInfo.Size(), "subtitle_artifacts": subtitleArtifactsJSON(subtitleArtifacts),
			"validation_report": report, "temporary_path": "", "updated_at": completed,
		}
		if current.Keep {
			updates["expires_at"] = nil
		} else {
			expires := completed.Add(time.Duration(testEncodePolicyFromSettings(tx).TemporaryRetentionHours) * time.Hour)
			updates["expires_at"] = &expires
		}
		readyResult = tx.Model(&models.TestEncode{}).Where("id = ? AND status = ?", id, testEncodeRunning).Updates(updates)
		return readyResult.Error
	})
	if readyResultErr != nil {
		h.failTestEncode(id, readyResultErr)
		return
	}
	validationPassed, _ := report["passed"].(bool)
	if readyResult != nil && readyResult.RowsAffected == 1 && validationPassed {
		_ = recordJellyfinTestEncodeReady(h.db, id, test.LibraryID, true, validationPassed)
	}
}

// bindTestEncodeRuntimeSnapshot binds immutable runtime evidence exactly once
// for a Test Encode record. Re-entry for the same prepared record reuses the
// existing reference; it never follows current telemetry or overwrites
// provenance. The transaction keeps the read-and-bind lifecycle atomic for
// SQLite's single-writer model.
func bindTestEncodeRuntimeSnapshot(db *gorm.DB, test *models.TestEncode) (*uint, error) {
	var boundID *uint
	err := db.Transaction(func(tx *gorm.DB) error {
		var current models.TestEncode
		if err := tx.First(&current, test.ID).Error; err != nil {
			return fmt.Errorf("load Test Encode %d for runtime freeze: %w", test.ID, err)
		}
		if current.RuntimeSnapshotID != nil {
			var snapshot models.RuntimeSnapshot
			if err := tx.First(&snapshot, *current.RuntimeSnapshotID).Error; err != nil {
				return fmt.Errorf("load frozen Test Encode runtime snapshot %d: %w", *current.RuntimeSnapshotID, err)
			}
			boundID = current.RuntimeSnapshotID
			*test = current
			return nil
		}

		state, err := runtimeinfo.Current(tx)
		if err != nil {
			return fmt.Errorf("capture Test Encode runtime evidence: %w", err)
		}
		snapshot, err := runtimeinfo.CaptureAuditSnapshot(tx, state, "test_encode_freeze")
		if err != nil {
			return fmt.Errorf("capture Test Encode runtime evidence: %w", err)
		}
		result := tx.Model(&models.TestEncode{}).
			Where("id = ? AND runtime_snapshot_id IS NULL", current.ID).
			Update("runtime_snapshot_id", snapshot.ID)
		if result.Error != nil {
			return fmt.Errorf("bind Test Encode runtime snapshot: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			if err := tx.First(&current, test.ID).Error; err != nil {
				return fmt.Errorf("reload Test Encode after runtime freeze race: %w", err)
			}
			if current.RuntimeSnapshotID == nil {
				return fmt.Errorf("Test Encode %d runtime freeze lost its binding", test.ID)
			}
			boundID = current.RuntimeSnapshotID
		} else {
			boundID = &snapshot.ID
			current.RuntimeSnapshotID = boundID
		}
		*test = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return boundID, nil
}

func activateTestSubtitleArtifacts(artifacts []SubtitleArtifact, temporaryMediaPath, outputMediaPath string) error {
	temporaryBase := strings.TrimSuffix(temporaryMediaPath, filepath.Ext(temporaryMediaPath))
	outputBase := strings.TrimSuffix(outputMediaPath, filepath.Ext(outputMediaPath))
	type activatedPath struct{ source, destination string }
	activated := []activatedPath{}
	destinations := make([]string, len(artifacts))
	for index, artifact := range artifacts {
		if !strings.HasPrefix(artifact.StagedPath, temporaryBase+".") {
			return fmt.Errorf("subtitle artifact is outside the Test Encode temporary basename: %s", artifact.StagedPath)
		}
		destination := outputBase + strings.TrimPrefix(artifact.StagedPath, temporaryBase)
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("test subtitle destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(artifact.StagedPath, destination); err != nil {
			for rollbackIndex := len(activated) - 1; rollbackIndex >= 0; rollbackIndex-- {
				_ = os.Rename(activated[rollbackIndex].destination, activated[rollbackIndex].source)
			}
			return err
		}
		activated = append(activated, activatedPath{source: artifact.StagedPath, destination: destination})
		destinations[index] = destination
	}
	for index := range artifacts {
		artifacts[index].StagedPath = destinations[index]
	}
	return nil
}

func plannedTestEncodeOutputPath(db *gorm.DB, job models.QueueJob, library models.Library, profile models.Profile) string {
	if pathIsInside(job.MediaPath, library.DestinationPath) {
		extension := "." + strings.TrimPrefix(profile.Container, ".")
		return strings.TrimSuffix(job.MediaPath, filepath.Ext(job.MediaPath)) + extension
	}
	return plannedOutputPathForJob(db, job, library, profile)
}

func testEncodeValidationReport(plan MediaJobPlan, streams MediaStreamInventory, expectedDuration int) models.JSONMap {
	return testEncodeValidationReportWithTiming(plan, streams, expectedDuration, avTimingFromInventory(plan.Streams), avTimingFromInventory(streams))
}

func testEncodeValidationReportWithTiming(plan MediaJobPlan, streams MediaStreamInventory, expectedDuration int, sourceTiming, outputTiming avTimingEvidence) models.JSONMap {
	streamPlan := resolveEffectiveStreamPlan(plan)
	warnings := append([]string{}, plan.StreamValidationWarnings...)
	if len(streams.Video) != len(streamPlan.Video) {
		warnings = append(warnings, fmt.Sprintf("Expected %d video tracks but measured %d", len(streamPlan.Video), len(streams.Video)))
	}
	if len(streams.Audio) != len(streamPlan.Audio) {
		warnings = append(warnings, fmt.Sprintf("Expected %d audio tracks but measured %d", len(streamPlan.Audio), len(streams.Audio)))
	}
	if len(streams.Subtitle) != len(streamPlan.Subtitles) {
		warnings = append(warnings, fmt.Sprintf("Expected %d embedded subtitle tracks but measured %d", len(streamPlan.Subtitles), len(streams.Subtitle)))
	}
	durationTolerance := maxFloat(2, float64(expectedDuration)*0.1)
	durationValid := streams.Duration > 0 && streams.Duration <= float64(expectedDuration)+durationTolerance
	if !durationValid {
		warnings = append(warnings, fmt.Sprintf("Measured duration %.3fs is outside the Test Encode window", streams.Duration))
	}
	cadenceValidation := models.JSONMap{"status": "not_applicable"}
	frameCountValidation := models.JSONMap{"status": "not_applicable"}
	timingSource, timingOutput := avTimingForStreamPlan(sourceTiming, outputTiming, streamPlan)
	timingValidation := validateAVTiming(timingSource, timingOutput)
	if timingValidation["status"] == "unverified" {
		warnings = append(warnings, "Output A/V timestamp alignment could not be verified from available evidence")
	} else if timingValidation["status"] == "warning" || timingValidation["status"] == "mismatch" {
		warnings = append(warnings, fmt.Sprintf("Output A/V timestamp alignment is %s", timingValidation["status"]))
	}
	if len(streams.Video) > 0 {
		video := streams.Video[0]
		cadenceValidation = validateCadenceOutputFrameRate(map[string]interface{}(plan.Profile.WorkerConfig), models.JSONMap{
			"frameRate": video.FrameRate, "realFrameRate": video.RealFrameRate,
			"frameCount": video.DecodedFrames, "duration": streams.Duration,
		})
		if cadenceValidation["status"] == "mismatch" || cadenceValidation["status"] == "unverified" {
			warnings = append(warnings, fmt.Sprintf("Output cadence validation is %s", cadenceValidation["status"]))
		}
		expectedFPS := parseFrameRateValue(workerStringValue(plan.Profile.WorkerConfig["effectiveOutputFrameRate"]))
		if expectedFPS > 0 {
			expectedFrames := int64(math.Round(expectedFPS * float64(expectedDuration)))
			frameStatus := "validated"
			if video.DecodedFrames <= 0 {
				frameStatus = "unverified"
			} else if frameAbsInt(int(video.DecodedFrames-expectedFrames)) > 1 {
				frameStatus = "mismatch"
			}
			frameCountValidation = models.JSONMap{"status": frameStatus, "expected": expectedFrames, "observed": video.DecodedFrames, "toleranceFrames": 1}
			if frameStatus != "validated" {
				warnings = append(warnings, fmt.Sprintf("Output frame-count validation is %s", frameStatus))
			}
		}
	}
	return models.JSONMap{
		"passed":          len(warnings) == len(plan.StreamValidationWarnings) && durationValid,
		"durationSeconds": streams.Duration, "expectedDurationSeconds": expectedDuration,
		"videoTracks": len(streams.Video), "expectedVideoTracks": len(streamPlan.Video),
		"audioTracks": len(streams.Audio), "expectedAudioTracks": len(streamPlan.Audio),
		"subtitleTracks": len(streams.Subtitle), "expectedSubtitleTracks": len(streamPlan.Subtitles),
		"cadence": cadenceValidation, "frameCount": frameCountValidation, "avTiming": timingValidation,
		"streamPlan": streamPlan, "warnings": warnings,
	}
}

func generateTestExternalSubtitleArtifacts(ctx context.Context, plan MediaJobPlan, existing []SubtitleArtifact) ([]SubtitleArtifact, error) {
	if plan.ResolvedTracks != nil {
		return nil, nil
	}
	sidecars, err := externalSubtitlesForMedia(plan.SourceAssetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sourceBase := strings.TrimSuffix(plan.SourceAssetPath, filepath.Ext(plan.SourceAssetPath))
	outputBase := strings.TrimSuffix(plan.OutputPath, filepath.Ext(plan.OutputPath))
	used := map[string]bool{}
	for _, artifact := range existing {
		used[filepath.Clean(artifact.StagedPath)] = true
	}
	artifacts := []SubtitleArtifact{}
	completed := false
	defer func() {
		if !completed {
			for _, artifact := range artifacts {
				_ = os.Remove(artifact.StagedPath)
			}
		}
	}()
	for _, sidecar := range sidecars {
		suffix := strings.TrimPrefix(sidecar.Path, sourceBase)
		if suffix == sidecar.Path || suffix == "" {
			continue
		}
		outputPath := outputBase + suffix
		if used[filepath.Clean(outputPath)] {
			continue
		}
		args := externalSubtitleSampleArgs(plan, sidecar, outputPath)
		commandOutput, runErr := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
		content, readErr := os.ReadFile(outputPath)
		if runErr != nil || readErr != nil || !validSubtitleSidecar(sidecar.Format, content) {
			_ = os.Remove(outputPath)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if copyErr := copyFileContents(sidecar.Path, outputPath); copyErr != nil {
				return nil, fmt.Errorf("external subtitle %s could not be sampled (%s) or copied: %w", sidecar.FileName, strings.TrimSpace(string(commandOutput)), copyErr)
			}
		}
		info, statErr := os.Stat(outputPath)
		if statErr != nil || info.Size() == 0 {
			return nil, fmt.Errorf("external subtitle %s produced an empty Test Encode sidecar", sidecar.FileName)
		}
		used[filepath.Clean(outputPath)] = true
		artifacts = append(artifacts, SubtitleArtifact{StreamIndex: -1, SourceCodec: sidecar.Format, Format: sidecar.Format, Language: sidecar.Language, Default: sidecar.Default, StagedPath: outputPath, SizeBytes: info.Size()})
	}
	completed = true
	return artifacts, nil
}

func externalSubtitleSampleArgs(plan MediaJobPlan, sidecar ExternalSubtitle, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	inputSeek, outputTrim := segmentedSeekWindow(plan.SegmentStartSeconds)
	if inputSeek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(inputSeek, 'f', -1, 64))
	}
	args = append(args, "-i", sidecar.Path)
	if outputTrim > 0 {
		args = append(args, "-ss", strconv.FormatFloat(outputTrim, 'f', -1, 64))
	}
	if plan.SegmentDurationSeconds > 0 {
		args = append(args, "-t", strconv.Itoa(plan.SegmentDurationSeconds))
	}
	return append(args, "-c:s", sidecar.Format, "-f", sidecar.Format, outputPath)
}

func (h AssetHandler) runTestEncodeFFmpeg(ctx context.Context, id uint, args []string, duration float64) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", ffmpegArgsWithProgress(args)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var stderrBuffer bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&stderrBuffer, stderr); close(done) }()
	scanner := bufio.NewScanner(stdout)
	last := 5
	for scanner.Scan() {
		if progress, ok := progressFromFFmpegLine(scanner.Text(), duration); ok && progress > last {
			last = progress
			_ = h.db.Model(&models.TestEncode{}).Where("id = ? AND status = ?", id, testEncodeRunning).Updates(map[string]any{
				"progress": min(95, progress), "updated_at": time.Now(),
			}).Error
		}
	}
	err = cmd.Wait()
	<-done
	if err != nil {
		message := strings.TrimSpace(lastOutputLines(stderrBuffer.String(), 14))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("test encode FFmpeg failed: %s", message)
	}
	return nil
}

func (h AssetHandler) failTestEncode(id uint, err error) {
	if err == nil {
		return
	}
	var test models.TestEncode
	_ = h.db.First(&test, id).Error
	if test.Status == testEncodeCanceled {
		return
	}
	now := time.Now()
	updates := map[string]any{
		"status": testEncodeFailed, "phase": "failed", "progress": 0, "error_message": err.Error(), "completed_at": &now, "updated_at": now,
	}
	if !test.Keep {
		expires := now.Add(time.Duration(testEncodePolicyFromSettings(h.db).FailedRetentionHours) * time.Hour)
		updates["expires_at"] = &expires
	}
	_, cleanupErr := h.removeTestEncodeFilesForOwner(test, id)
	if cleanupErr != nil {
		updates["reconciliation_status"] = "cleanup_failed"
		updates["reconciliation_message"] = cleanupErr.Error()
	}
	_ = h.db.Model(&models.TestEncode{}).Where("id = ?", id).Updates(updates).Error
}

func configurationHash(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (h AssetHandler) removeTestEncodeFiles(test models.TestEncode) error {
	_, err := h.removeTestEncodeFilesWithEvidence(test)
	return err
}

func subtitleArtifactPaths(values models.JSONList) []string {
	result := []string{}
	for _, raw := range values {
		item := settingProfileObject(raw)
		if path := strings.TrimSpace(workerStringValue(item["stagedPath"])); path != "" {
			result = append(result, path)
		}
	}
	return result
}
