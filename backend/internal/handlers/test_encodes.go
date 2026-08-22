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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	testEncodeOwnerType           = "test_encode"
	testEncodeWaiting             = "waiting"
	testEncodeRunning             = "generating"
	testEncodeReady               = "ready"
	testEncodeFailed              = "failed"
	testEncodeCanceled            = "canceled"
	testEncodeDeleted             = "deleted"
	testEncodeFFmpegCommandColumn = "ffmpeg_command"
)

type testEncodeRequest struct {
	SourcePath          string                       `json:"sourcePath" binding:"required"`
	LibraryID           uint                         `json:"libraryId" binding:"required"`
	ConfigurationSource string                       `json:"configurationSource" binding:"required"`
	ProfileID           uint                         `json:"profileId"`
	AudioProfileKey     string                       `json:"audioProfileKey"`
	TrackProfileKey     string                       `json:"trackProfileKey"`
	ProcessingMode      string                       `json:"processingMode"`
	ResolveAssignments  bool                         `json:"resolveAssignments"`
	LabProfile          *models.Profile              `json:"labProfile"`
	LabAudioProfile     models.JSONMap               `json:"labAudioProfile"`
	LabTrackOverride    AssetConversionOverrideState `json:"labTrackOverride"`
	StartMode           string                       `json:"startMode"`
	StartSeconds        float64                      `json:"startSeconds"`
	DurationSeconds     int                          `json:"durationSeconds"`
	ConfigurationToken  string                       `json:"configurationToken"`
}

type resolvedTestEncode struct {
	job       models.QueueJob
	profile   models.Profile
	audio     *audioEnhancementProfile
	override  AssetConversionOverrideState
	library   models.Library
	requested models.JSONMap
}

var testEncodeProcesses = struct {
	sync.Mutex
	cancel map[uint]context.CancelFunc
}{cancel: map[uint]context.CancelFunc{}}

var testEncodeDispatchMu sync.Mutex

func recoverInterruptedTestEncodes(db *gorm.DB) {
	if db == nil || !db.Migrator().HasTable(&models.TestEncode{}) {
		return
	}
	var interrupted []models.TestEncode
	_ = db.Where("status IN ?", []string{testEncodeWaiting, testEncodeRunning}).Find(&interrupted).Error
	for _, test := range interrupted {
		_ = (AssetHandler{db: db}).removeTestEncodeFiles(test)
		now := time.Now()
		_ = db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Updates(map[string]any{
			"status": testEncodeFailed, "phase": "interrupted", "progress": 0,
			"error_message": "Test Encode was interrupted by application restart", "completed_at": &now, "updated_at": now,
		}).Error
	}
	if db.Migrator().HasTable(&models.TaskReservation{}) {
		_ = db.Where("owner_type = ?", testEncodeOwnerType).Delete(&models.TaskReservation{}).Error
	}
}

func (h AssetHandler) ListTestEncodes(c *gin.Context) {
	query := h.db.Where("deleted_at IS NULL").Order("created_at DESC")
	if sourcePath := strings.TrimSpace(c.Query("sourcePath")); sourcePath != "" {
		resolved, err := h.resolveMediaPath(sourcePath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		query = query.Where("source_path = ?", filepath.Clean(resolved))
	}
	var tests []models.TestEncode
	if err := query.Find(&tests).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for index := range tests {
		h.decorateTestEncode(&tests[index])
	}
	c.JSON(http.StatusOK, tests)
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
	now := time.Now()
	modified := info.ModTime()
	expires := now.Add(time.Duration(testEncodeRetentionHours(h.db)) * time.Hour)
	test := models.TestEncode{
		SourcePath: resolved.job.MediaPath, SourceFingerprint: fingerprint, SourceSizeBytes: info.Size(), SourceModifiedAt: &modified,
		LibraryID: resolved.library.ID, ConfigurationSource: normalizedTestEncodeConfigurationSource(input.ConfigurationSource),
		RequestedConfiguration: resolved.requested, ProfileID: resolved.job.ProfileID, ProfileVersion: resolved.job.ProfileVersion,
		StartSeconds: start, DurationSeconds: duration, Status: testEncodeWaiting, Phase: "resolving_capacity", Progress: 0,
		ExpiresAt: &expires, CreatedAt: now, UpdatedAt: now,
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
	_ = h.db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Updates(map[string]any{
		"status": testEncodeCanceled, "phase": "canceled", "canceled_at": &now, "updated_at": now,
	}).Error
	_ = scheduler.ReleaseTaskReservation(h.db, testEncodeOwnerType, test.ID)
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
	updates := map[string]any{"keep": input.Keep, "updated_at": time.Now()}
	if input.Keep {
		updates["expires_at"] = nil
	} else {
		expires := time.Now().Add(time.Duration(testEncodeRetentionHours(h.db)) * time.Hour)
		updates["expires_at"] = &expires
	}
	if err := h.db.Model(&test).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.db.First(&test, test.ID).Error
	c.JSON(http.StatusOK, test)
}

func (h AssetHandler) DeleteTestEncode(c *gin.Context) {
	test, ok := h.testEncodeFromParam(c)
	if !ok {
		return
	}
	if test.Status == testEncodeWaiting || test.Status == testEncodeRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "cancel the generating test before deleting it"})
		return
	}
	if err := h.removeTestEncodeFiles(test); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	if err := h.db.Model(&test).Updates(map[string]any{
		"status": testEncodeDeleted, "phase": "deleted", "deleted_at": &now, "output_path": "", "temporary_path": "", "updated_at": now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": test.ID, "status": testEncodeDeleted})
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
	if source == "lab_draft" {
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
		if encoded, err := json.Marshal(input.LabTrackOverride); err == nil {
			_ = json.Unmarshal(encoded, &job.TrackProfileSnapshot)
		}
		if len(job.TrackProfileSnapshot) > 0 {
			job.TrackProfileKey = "lab-draft"
		}
		queue.captureInterlaceSnapshot(path, job.ProfileSnapshot)
		requested["labProfile"] = profile
		requested["labAudioProfile"] = input.LabAudioProfile
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
		if job.ProfileID == 0 {
			return resolvedTestEncode{}, fmt.Errorf("an effective video profile is required for Test Encode V1")
		}
		if err := queue.captureProfile(&job, job.ProfileID, "test_encode_effective_asset"); err != nil {
			return resolvedTestEncode{}, err
		}
		if err := queue.captureSupplementalProfiles(&job); err != nil {
			return resolvedTestEncode{}, err
		}
		requested["profileId"] = job.ProfileID
		requested["audioProfileKey"] = job.AudioProfileKey
		requested["trackProfileKey"] = job.TrackProfileKey
		requested["profileResolution"] = job.ProfileResolution
	}
	worker := WorkerHandler{db: h.db}
	profile, err := worker.profileForJob(job)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	override := conversionOverrideForJob(job, assetConversionOverrides(h.db))
	profile = applyAssetConversionOverrideToProfile(profile, override)
	profile, err = resolveAutomaticFrameStructure(h.db, path, profile)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["qsvAssetAnalysisPath"] = path
	if frozen, ok := job.ProfileSnapshot[interlaceAnalysisSnapshotKey]; ok {
		profile.WorkerConfig[interlaceAnalysisSnapshotKey] = frozen
	}
	audio, err := worker.audioProfileForJob(job)
	if err != nil {
		return resolvedTestEncode{}, err
	}
	requested["sourceConfigurationHash"] = configurationHash(models.JSONMap{
		"profile": profile, "audio": audio, "override": override, "profileResolution": job.ProfileResolution,
	})
	return resolvedTestEncode{job: job, profile: profile, audio: audio, override: override, library: library, requested: requested}, nil
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
	duration := min(120, max(5, requestedDuration))
	if requestedDuration == 0 {
		duration = 20
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

func testEncodeRetentionHours(db *gorm.DB) int {
	var setting models.AppSetting
	if db.First(&setting, "key = ?", "testEncodePolicy").Error == nil {
		return min(24*365, max(1, intSetting(setting.Value, "retentionHours", 24)))
	}
	if db.First(&setting, "key = ?", "housekeeping").Error == nil {
		return min(24*365, max(1, intSetting(setting.Value, "testEncodeRetentionHours", 24)))
	}
	return 24
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
		_ = scheduler.ReleaseTaskReservation(h.db, testEncodeOwnerType, id)
	}()

	var test models.TestEncode
	if h.db.First(&test, id).Error != nil {
		return
	}
	if test.Status != testEncodeWaiting {
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
	planDecision := models.ExecutionPlan{
		SelectedEncoder: encoder, EstimatedOutputMaxBytes: max(64<<20, test.SourceSizeBytes/20),
		EstimatedWorkspaceBytes: max(64<<20, test.SourceSizeBytes/20),
		Reservation:             models.JSONMap{"jobType": string(scheduler.JobTypeTestEncode)},
	}

	var workerName string
	for {
		if ctx.Err() != nil {
			return
		}
		testEncodeDispatchMu.Lock()
		decision, decisionErr := scheduler.EvaluateResources(h.db, &planDecision)
		if decisionErr == nil && decision.Allowed {
			workerName = decision.Worker.Worker
			decisionErr = scheduler.ActivateTaskReservation(h.db, testEncodeOwnerType, id, test.SourcePath, planDecision, workerName)
		}
		testEncodeDispatchMu.Unlock()
		if decisionErr != nil {
			h.failTestEncode(id, decisionErr)
			return
		}
		if decision.Allowed {
			break
		}
		_ = h.db.Model(&models.TestEncode{}).Where("id = ? AND status = ?", id, testEncodeWaiting).Updates(map[string]any{
			"phase": decision.WaitingState, "updated_at": time.Now(),
		}).Error
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	effective := models.JSONMap{
		"profile": plan.Profile, "override": plan.Override, "streamPlan": resolveEffectiveStreamPlan(plan),
		"effectiveEncoder": encoder, "worker": workerName, "runtimeSnapshotId": planDecision.RuntimeSnapshotID,
	}
	effectiveHash := configurationHash(effective)
	plan.OutputMetadata["MVFORGE_CONFIG_HASH"] = effectiveHash
	args := FFmpegCommandBuilder{}.Build(plan)
	command := "ffmpeg " + shellJoin(args)
	now := time.Now()
	result := h.db.Model(&models.TestEncode{}).Where("id = ? AND status = ?", id, testEncodeWaiting).Updates(map[string]any{
		"status": testEncodeRunning, "phase": "generating", "progress": 5, "started_at": &now,
		"worker_name": workerName, "runtime_snapshot_id": planDecision.RuntimeSnapshotID,
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

	artifactPlan := plan
	artifactPlan.OutputPath = output
	subtitleArtifacts, err := generateSubtitleArtifacts(ctx, artifactPlan)
	if err != nil {
		h.failTestEncode(id, fmt.Errorf("generate test subtitle sidecars: %w", err))
		return
	}
	externalArtifacts, err := generateTestExternalSubtitleArtifacts(ctx, artifactPlan, subtitleArtifacts)
	if err != nil {
		for _, artifact := range subtitleArtifacts {
			_ = os.Remove(artifact.StagedPath)
		}
		h.failTestEncode(id, fmt.Errorf("generate external test subtitle sidecars: %w", err))
		return
	}
	subtitleArtifacts = append(subtitleArtifacts, externalArtifacts...)
	if err := h.db.Model(&models.TestEncode{}).Where("id = ?", id).Update("subtitle_artifacts", subtitleArtifactsJSON(subtitleArtifacts)).Error; err != nil {
		for _, artifact := range subtitleArtifacts {
			_ = os.Remove(artifact.StagedPath)
		}
		h.failTestEncode(id, err)
		return
	}
	if err := h.runTestEncodeFFmpeg(ctx, id, args, float64(test.DurationSeconds)); err != nil {
		for _, artifact := range subtitleArtifacts {
			_ = os.Remove(artifact.StagedPath)
		}
		_ = os.Remove(temporary)
		if ctx.Err() == nil {
			h.failTestEncode(id, err)
		}
		return
	}
	if err := os.Rename(temporary, output); err != nil {
		h.failTestEncode(id, fmt.Errorf("activate test output: %w", err))
		return
	}
	outputInfo, err := os.Stat(output)
	if err != nil || outputInfo.Size() == 0 {
		h.failTestEncode(id, fmt.Errorf("test output validation failed: output is missing or empty"))
		return
	}
	streams, err := probeMediaStreams(output)
	if err != nil {
		h.failTestEncode(id, fmt.Errorf("probe test output: %w", err))
		return
	}
	report := testEncodeValidationReport(plan, streams, test.DurationSeconds)
	completed := time.Now()
	_ = h.db.Model(&models.TestEncode{}).Where("id = ?", id).Updates(map[string]any{
		"status": testEncodeReady, "phase": "ready", "progress": 100, "completed_at": &completed,
		"output_size_bytes": outputInfo.Size(), "subtitle_artifacts": subtitleArtifactsJSON(subtitleArtifacts),
		"validation_report": report, "temporary_path": "", "updated_at": completed,
	}).Error
}

func plannedTestEncodeOutputPath(db *gorm.DB, job models.QueueJob, library models.Library, profile models.Profile) string {
	if pathIsInside(job.MediaPath, library.DestinationPath) {
		extension := "." + strings.TrimPrefix(profile.Container, ".")
		return strings.TrimSuffix(job.MediaPath, filepath.Ext(job.MediaPath)) + extension
	}
	return plannedOutputPathForJob(db, job, library, profile)
}

func testEncodeValidationReport(plan MediaJobPlan, streams MediaStreamInventory, expectedDuration int) models.JSONMap {
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
	return models.JSONMap{
		"passed":          len(warnings) == len(plan.StreamValidationWarnings) && durationValid,
		"durationSeconds": streams.Duration, "expectedDurationSeconds": expectedDuration,
		"videoTracks": len(streams.Video), "expectedVideoTracks": len(streamPlan.Video),
		"audioTracks": len(streams.Audio), "expectedAudioTracks": len(streamPlan.Audio),
		"subtitleTracks": len(streams.Subtitle), "expectedSubtitleTracks": len(streamPlan.Subtitles),
		"streamPlan": streamPlan, "warnings": warnings,
	}
}

func generateTestExternalSubtitleArtifacts(ctx context.Context, plan MediaJobPlan, existing []SubtitleArtifact) ([]SubtitleArtifact, error) {
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
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
		if plan.SegmentStartSeconds > 0 {
			args = append(args, "-ss", strconv.FormatFloat(plan.SegmentStartSeconds, 'f', -1, 64))
		}
		args = append(args, "-i", sidecar.Path)
		if plan.SegmentDurationSeconds > 0 {
			args = append(args, "-t", strconv.Itoa(plan.SegmentDurationSeconds))
		}
		args = append(args, "-c:s", sidecar.Format, "-f", sidecar.Format, outputPath)
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
	_ = h.removeTestEncodeFiles(test)
	now := time.Now()
	_ = h.db.Model(&models.TestEncode{}).Where("id = ?", id).Updates(map[string]any{
		"status": testEncodeFailed, "phase": "failed", "progress": 0, "error_message": err.Error(), "completed_at": &now, "updated_at": now,
	}).Error
}

func configurationHash(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (h AssetHandler) removeTestEncodeFiles(test models.TestEncode) error {
	var library models.Library
	if err := h.db.First(&library, test.LibraryID).Error; err != nil {
		return err
	}
	for _, candidate := range append([]string{test.OutputPath, test.TemporaryPath}, subtitleArtifactPaths(test.SubtitleArtifacts)...) {
		candidate = filepath.Clean(strings.TrimSpace(candidate))
		if candidate == "." || candidate == "" {
			continue
		}
		if !pathIsInside(candidate, library.DestinationPath) {
			return fmt.Errorf("refusing to delete Test Encode file outside its Library destination")
		}
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
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
