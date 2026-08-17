package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type QueueHandler struct {
	db *gorm.DB
}

type QueueJobInput struct {
	MediaPath                 string `json:"mediaPath" binding:"required"`
	PublishMode               string `json:"publishMode"`
	BatchID                   string `json:"batchId"`
	BatchName                 string `json:"batchName"`
	BatchPosition             int    `json:"batchPosition"`
	LibraryID                 uint   `json:"libraryId" binding:"required"`
	ProfileID                 uint   `json:"profileId" binding:"required"`
	AudioProfileKey           string `json:"audioProfileKey"`
	TrackProfileKey           string `json:"trackProfileKey"`
	ProcessingMode            string `json:"processingMode"`
	Priority                  int    `json:"priority"`
	Notes                     string `json:"notes"`
	ResolveProfileAssignments bool   `json:"resolveProfileAssignments"`
}

const (
	VideoAssignmentOverrideOnly = "override_only"
	VideoAssignmentAudioOnly    = "audio_only"
)

type QueueJobUpdateInput struct {
	MediaPath       string  `json:"mediaPath"`
	LibraryID       *uint   `json:"libraryId"`
	ProfileID       *uint   `json:"profileId"`
	AudioProfileKey *string `json:"audioProfileKey"`
	TrackProfileKey *string `json:"trackProfileKey"`
	ProcessingMode  *string `json:"processingMode"`
	Priority        *int    `json:"priority"`
	Status          string  `json:"status"`
	Notes           string  `json:"notes"`
}

type QueueReorderInput struct {
	JobID       uint   `json:"jobId" binding:"required"`
	TargetJobID uint   `json:"targetJobId" binding:"required"`
	Placement   string `json:"placement" binding:"required"`
}

func NewQueueHandler(db *gorm.DB) QueueHandler {
	return QueueHandler{db: db}
}

func (h QueueHandler) List(c *gin.Context) {
	var jobs []models.QueueJob
	if err := h.db.Where("dismissed_at IS NULL").Order("queue_position asc, created_at asc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

func (h QueueHandler) Reorder(c *gin.Context) {
	var input QueueReorderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.JobID == input.TargetJobID || (input.Placement != "before" && input.Placement != "after") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reorder requires different queued jobs and placement before or after"})
		return
	}

	var ordered []models.QueueJob
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dismissed_at IS NULL AND status = ?", JobStatusQueued).
			Order("queue_position asc, priority asc, created_at asc, id asc").Find(&ordered).Error; err != nil {
			return err
		}
		draggedIndex, targetIndex := -1, -1
		for index := range ordered {
			if ordered[index].ID == input.JobID {
				draggedIndex = index
			}
			if ordered[index].ID == input.TargetJobID {
				targetIndex = index
			}
		}
		if draggedIndex < 0 || targetIndex < 0 {
			return gorm.ErrRecordNotFound
		}
		dragged := ordered[draggedIndex]
		ordered = append(ordered[:draggedIndex], ordered[draggedIndex+1:]...)
		targetIndex = -1
		for index := range ordered {
			if ordered[index].ID == input.TargetJobID {
				targetIndex = index
				break
			}
		}
		if input.Placement == "after" {
			targetIndex++
		}
		ordered = append(ordered, models.QueueJob{})
		copy(ordered[targetIndex+1:], ordered[targetIndex:])
		ordered[targetIndex] = dragged
		for index := range ordered {
			if err := tx.Model(&models.QueueJob{}).Where("id = ? AND status = ?", ordered[index].ID, JobStatusQueued).
				Update("queue_position", int64(index+1)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusConflict, gin.H{"error": "only currently queued jobs can be reordered"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": ordered})
}

func (h QueueHandler) Dismiss(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	var job models.QueueJob
	if err := h.db.First(&job, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job.DismissedAt != nil {
		c.JSON(http.StatusOK, job)
		return
	}
	if job.Status == JobStatusRunning || job.Status == JobStatusCompleted {
		c.JSON(http.StatusConflict, gin.H{"error": "running or completed jobs cannot be removed from queue"})
		return
	}
	if job.ExecutionNumber == nil && job.StartedAt == nil {
		err = h.db.Transaction(func(tx *gorm.DB) error {
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			if err := tx.Where("job_id = ?", job.ID).Delete(&models.ExecutionPlan{}).Error; err != nil {
				return err
			}
			return tx.Delete(&job).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"removed": true, "placeholderId": job.ID})
		return
	}

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		job.DismissedAt = &now
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		return scheduler.ReleaseReservation(tx, job.ID)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h QueueHandler) DismissBatch(c *gin.Context) {
	batchID := strings.TrimSpace(c.Param("batchId"))
	if batchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch id is required"})
		return
	}
	var jobs []models.QueueJob
	if err := h.db.Where("batch_id = ? AND dismissed_at IS NULL", batchID).Order("id asc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(jobs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	now := time.Now()
	removedPlaceholders := 0
	dismissedJobs := 0
	preservedCompleted := 0
	canceledProcesses := map[uint]bool{}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for index := range jobs {
			job := &jobs[index]
			if job.Status == JobStatusCompleted {
				preservedCompleted++
				continue
			}
			if job.ExecutionNumber == nil && job.StartedAt == nil {
				if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
					return err
				}
				if err := tx.Where("job_id = ?", job.ID).Delete(&models.ExecutionPlan{}).Error; err != nil {
					return err
				}
				if err := tx.Delete(job).Error; err != nil {
					return err
				}
				removedPlaceholders++
				continue
			}
			if job.Status == JobStatusRunning {
				canceledProcesses[job.ID] = cancelRunningJobProcess(job.ID)
				job.Status = JobStatusCanceled
				job.FinishedAt = &now
				if err := transitionJobStage(tx, job, JobStageCanceled); err != nil {
					return err
				}
			}
			job.DismissedAt = &now
			if err := tx.Save(job).Error; err != nil {
				return err
			}
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			dismissedJobs++
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, job := range jobs {
		if job.Status == JobStatusCanceled && !canceledProcesses[job.ID] {
			_ = cleanupCanceledJob(h.db, job)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"batchId":             batchID,
		"removedPlaceholders": removedPlaceholders,
		"dismissedJobs":       dismissedJobs,
		"preservedCompleted":  preservedCompleted,
	})
}

func queueUsesOverrideOnlyVideo(resolution models.JSONMap) bool {
	raw, ok := resolution["video"]
	if !ok {
		return false
	}

	video, ok := raw.(models.JSONMap)
	if !ok {
		if generic, genericOK := raw.(map[string]any); genericOK {
			return boolValue(generic["overrideOnly"], false)
		}
		return false
	}

	return boolValue(video["overrideOnly"], false)
}

func (h QueueHandler) Create(c *gin.Context) {
	var input QueueJobInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if review := reviewForPath(filepath.Clean(input.MediaPath), assetReviewOverrides(h.db)); review.RequiresReview {
		c.JSON(http.StatusConflict, gin.H{
			"error":  "asset requires review before queueing",
			"review": review,
		})
		return
	}
	if active, err := h.assetHasOpenJob(input.MediaPath, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if active {
		c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
		return
	}
	profileResolution := models.JSONMap{}
	if input.ResolveProfileAssignments {
		var err error
		profileResolution, err = h.resolveProfileAssignments(&input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "resolve profile assignments: " + err.Error()})
			return
		}
	}
	publishMode := input.PublishMode
	if publishMode == "" {
		publishMode = PublishModeStandard
	}
	if publishMode != PublishModeStandard && publishMode != PublishModeReplaceLibrary {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid publish mode"})
		return
	}
	processingMode := normalizeQueueProcessingMode(input.ProcessingMode)
	if strings.TrimSpace(input.ProcessingMode) != "" && processingMode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processing mode"})
		return
	}
	if publishMode == PublishModeReplaceLibrary {
		var library models.Library
		if err := h.db.First(&library, input.LibraryID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library not found"})
			return
		}
		if !pathIsInside(input.MediaPath, library.DestinationPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library replacement source must be inside the selected library destination"})
			return
		}
		if existing, err := h.activePublicationForSamePhysicalFile(input.MediaPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":         "this physical Library file already has an active MVForge publication through another library path",
				"publishedPath": existing.PublishedPath,
				"jobId":         existing.ID,
			})
			return
		}
	}

	priority := input.Priority
	if priority == 0 {
		priority = 5
	}

	job := models.QueueJob{
		MediaPath:         input.MediaPath,
		PublishMode:       publishMode,
		BatchID:           input.BatchID,
		BatchName:         input.BatchName,
		BatchPosition:     max(0, input.BatchPosition),
		LibraryID:         input.LibraryID,
		ProfileID:         input.ProfileID,
		AudioProfileKey:   input.AudioProfileKey,
		TrackProfileKey:   input.TrackProfileKey,
		ProfileResolution: profileResolution,
		ProcessingMode:    processingMode,
		Priority:          priority,
		Status:            "queued",
		Stage:             JobStageQueued,
		Notes:             input.Notes,
	}
	if err := h.captureSupplementalProfiles(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if queueUsesOverrideOnlyVideo(job.ProfileResolution) {
		if err := h.captureOverrideOnlyProfile(
			&job,
			"queue_create_override_only",
		); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		if err := h.captureProfile(
			&job,
			input.ProfileID,
			"queue_create",
		); err != nil {
			h.profileCaptureError(c, err)
			return
		}
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var highest int64
		if err := tx.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
			return err
		}
		job.QueuePosition = highest + 1
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := scheduler.LockQueuedAsset(tx, job); err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStageQueued); err != nil {
			return err
		}
		_, err := scheduler.CreatePendingExecutionPlan(tx, &job, "Execution plan created from queued profile snapshot")
		return err
	}); err != nil {
		if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
			c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, job)
}

func (h QueueHandler) resolveProfileAssignments(input *QueueJobInput) (models.JSONMap, error) {
	resolution := models.JSONMap{}
	assignments, err := profileAssignmentsForAsset(h.db, input.MediaPath)
	if err != nil {
		return nil, err
	}
	for mediaType, assignment := range assignments {
		resolution[mediaType] = models.JSONMap{
			"source":         assignment.TargetType,
			"targetPath":     assignment.TargetPath,
			"selection":      assignment.Selection,
			"videoProfileId": assignment.VideoProfileID,
			"profileKey":     assignment.ProfileKey,
			"overrideOnly":   assignment.Selection == VideoAssignmentOverrideOnly,
		}
		switch mediaType {

		case "video":
			switch assignment.Selection {
			case "profile":
				if assignment.VideoProfileID > 0 {
					input.ProfileID = assignment.VideoProfileID
					input.ProcessingMode = ProcessingModeFullEncode
				}

			case VideoAssignmentOverrideOnly:
				input.ProfileID = 0
				input.ProcessingMode = ProcessingModeFullEncode

			case VideoAssignmentAudioOnly, "disabled":
				// Keep "disabled" for backward compatibility with existing
				// stored assignments, but new UI should use "audio_only".
				input.ProfileID = 0
				input.ProcessingMode = ProcessingModeAudioOnly
			}

		case "audio":
			if assignment.Selection == "profile" {
				input.AudioProfileKey = assignment.ProfileKey
			} else if assignment.Selection == "disabled" {
				input.AudioProfileKey = ""
			}
		case "tracks":
			if assignment.Selection == "profile" {
				input.TrackProfileKey = assignment.ProfileKey
			} else if assignment.Selection == "disabled" {
				input.TrackProfileKey = ""
			}
		}
	}
	return resolution, nil
}

func (h QueueHandler) captureSupplementalProfiles(job *models.QueueJob) error {
	if strings.TrimSpace(job.AudioProfileKey) != "" {
		profile, err := settingProfileByKey(h.db, "audioEnhancementProfiles", job.AudioProfileKey)
		if err != nil {
			return fmt.Errorf("audio profile: %w", err)
		}
		job.AudioProfileSnapshot = models.JSONMap(profile)
	}
	if strings.TrimSpace(job.TrackProfileKey) != "" {
		profile, err := settingProfileByKey(h.db, "trackProfiles", job.TrackProfileKey)
		if err != nil {
			return fmt.Errorf("track profile: %w", err)
		}
		resolved := resolveTrackProfileForAsset(h.db, job.MediaPath, profile)
		if storedSettingProfileScope(profile) == "path" && strings.TrimSpace(workerStringValue(resolved["resolvedForAsset"])) == "" {
			return fmt.Errorf("track profile: a cached asset snapshot is required to resolve a path profile safely")
		}
		job.TrackProfileSnapshot = models.JSONMap(resolved)
	}
	return nil
}

func resolveTrackProfileForAsset(db *gorm.DB, mediaPath string, profile map[string]any) map[string]any {
	var scan models.ScanResult
	if err := db.Where("path = ?", filepath.Clean(mediaPath)).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return profile
	}
	scope := storedSettingProfileScope(profile)
	if scope == "path" {
		delete(profile, "keepVideoStreams")
		delete(profile, "keepAudioStreams")
		delete(profile, "keepSubtitleStreams")
		delete(profile, "videoMetadata")
		delete(profile, "audioMetadata")
		delete(profile, "subtitleMetadata")
		delete(profile, "subtitleTransforms")
	}
	if _, exists := profile["keepVideoStreams"]; !exists {
		indexes := snapshotStreamIndexes(scan.VideoStreams, nil)
		if workerStringValue(profile["videoMode"]) != "all" && len(indexes) > 1 {
			indexes = indexes[:1]
		}
		profile["keepVideoStreams"] = indexes
	}
	if _, exists := profile["keepAudioStreams"]; !exists {
		mode := workerStringValue(profile["audioMode"])
		languages := stringSet(profile["audioLanguages"])
		indexes := snapshotStreamIndexes(scan.AudioStreams, func(stream map[string]any) bool {
			if mode == "none" {
				return false
			}
			if boolValue(profile["dropCommentary"], true) && (boolValue(stream["comment"], false) || strings.Contains(strings.ToLower(workerStringValue(stream["title"])), "commentary")) {
				return false
			}
			if mode == "default" {
				return boolValue(stream["default"], false)
			}
			if mode == "languages" {
				return languages[strings.ToLower(workerStringValue(stream["language"]))]
			}
			return true
		})
		if mode == "default" && len(indexes) == 0 {
			all := snapshotStreamIndexes(scan.AudioStreams, nil)
			if len(all) > 0 {
				indexes = all[:1]
			}
		}
		profile["keepAudioStreams"] = indexes
	}
	if _, exists := profile["keepSubtitleStreams"]; !exists {
		mode := workerStringValue(profile["subtitleMode"])
		languages := stringSet(profile["subtitleLanguages"])
		profile["keepSubtitleStreams"] = snapshotStreamIndexes(scan.SubtitleStreams, func(stream map[string]any) bool {
			forced := boolValue(stream["forced"], false)
			languageMatch := languages[strings.ToLower(workerStringValue(stream["language"]))]
			switch mode {
			case "none":
				return false
			case "forced":
				return forced
			case "languages":
				return languageMatch
			case "forced-or-languages":
				return forced || languageMatch
			default:
				return true
			}
		})
	}
	if scope == "path" {
		if metadata := defaultLanguageMetadata(scan.AudioStreams, profile["keepAudioStreams"], workerStringValue(profile["defaultAudioLanguage"])); len(metadata) > 0 {
			profile["audioMetadata"] = metadata
		}
		if metadata := defaultLanguageMetadata(scan.SubtitleStreams, profile["keepSubtitleStreams"], workerStringValue(profile["defaultSubtitleLanguage"])); len(metadata) > 0 {
			profile["subtitleMetadata"] = metadata
		}
	}
	profile["resolvedForAsset"] = filepath.Clean(mediaPath)
	return profile
}

func defaultLanguageMetadata(streams models.JSONList, selected any, language string) map[string]any {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return nil
	}
	selectedIndexes := map[int]bool{}
	for _, value := range workerSliceValue(selected) {
		if index := streamIndexValue(value); index >= 0 {
			selectedIndexes[index] = true
		}
	}
	defaultIndex := -1
	for _, raw := range streams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if selectedIndexes[index] && strings.EqualFold(strings.TrimSpace(workerStringValue(stream["language"])), language) {
			defaultIndex = index
			break
		}
	}
	if defaultIndex < 0 {
		return nil
	}
	metadata := map[string]any{}
	for index := range selectedIndexes {
		metadata[strconv.Itoa(index)] = map[string]any{"default": index == defaultIndex}
	}
	return metadata
}

func workerSliceValue(value any) []any {
	switch values := value.(type) {
	case []any:
		return values
	case []int:
		result := make([]any, len(values))
		for index := range values {
			result[index] = values[index]
		}
		return result
	case models.JSONList:
		return []any(values)
	default:
		return nil
	}
}

func snapshotStreamIndexes(streams models.JSONList, keep func(map[string]any) bool) []int {
	indexes := []int{}
	for _, raw := range streams {
		var stream map[string]any
		switch value := raw.(type) {
		case map[string]any:
			stream = value
		case models.JSONMap:
			stream = map[string]any(value)
		default:
			continue
		}
		if keep != nil && !keep(stream) {
			continue
		}
		index := streamIndexValue(stream["index"])
		if index >= 0 {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func streamIndexValue(value any) int {
	number := workerNumberValue(value, -1)
	if number < 0 {
		return -1
	}
	return int(number)
}

func stringSet(value any) map[string]bool {
	result := map[string]bool{}
	for _, item := range workerStringSlice(value) {
		result[strings.ToLower(strings.TrimSpace(item))] = true
	}
	return result
}

func boolValue(value any, fallback bool) bool {
	parsed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return parsed
}

func settingProfileByKey(db *gorm.DB, settingKey, profileKey string) (map[string]any, error) {
	var setting models.AppSetting
	if err := db.Where("key = ?", settingKey).First(&setting).Error; err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	profiles := settingProfileValues(setting.Value["profiles"])
	for _, raw := range profiles {
		profile := settingProfileObject(raw)
		if profile != nil && strings.TrimSpace(stringFromUnknown(profile["key"])) == strings.TrimSpace(profileKey) {
			copy := map[string]any{}
			for key, value := range profile {
				copy[key] = value
			}
			return copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (h QueueHandler) activePublicationForSamePhysicalFile(mediaPath string) (*models.QueueJob, error) {
	requested, err := os.Stat(filepath.Clean(mediaPath))
	if err != nil {
		return nil, err
	}
	var publications []models.QueueJob
	if err := h.db.Where("publication_retired_at IS NULL AND published_path <> ''").Order("id desc").Find(&publications).Error; err != nil {
		return nil, err
	}
	for index := range publications {
		candidatePath := filepath.Clean(strings.TrimSpace(publications[index].PublishedPath))
		if candidatePath == "." {
			continue
		}
		candidate, statErr := os.Stat(candidatePath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, statErr
		}
		if os.SameFile(requested, candidate) {
			return &publications[index], nil
		}
	}
	return nil, nil
}

func (h QueueHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	var input QueueJobUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var job models.QueueJob
	if err := h.db.First(&job, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if input.Priority != nil {
		job.Priority = clamp(*input.Priority, 1, 10)
	}
	if queueJobConfigChanged(input) && job.Status != JobStatusQueued && job.Status != JobStatusFailed && job.Status != JobStatusCanceled {
		c.JSON(http.StatusConflict, gin.H{"error": "only queued, failed, or canceled jobs can be edited"})
		return
	}
	mediaPathChanged := input.MediaPath != "" && input.MediaPath != job.MediaPath
	if mediaPathChanged {
		if active, err := h.assetHasOpenJob(input.MediaPath, job.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if active {
			c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
			return
		}
		if review := reviewForPath(filepath.Clean(input.MediaPath), assetReviewOverrides(h.db)); review.RequiresReview {
			c.JSON(http.StatusConflict, gin.H{
				"error":  "asset requires review before queueing",
				"review": review,
			})
			return
		}
		job.MediaPath = input.MediaPath
	}
	if input.LibraryID != nil {
		job.LibraryID = *input.LibraryID
	}
	if job.PublishMode == PublishModeReplaceLibrary && (mediaPathChanged || input.LibraryID != nil) {
		var library models.Library
		if err := h.db.First(&library, job.LibraryID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library not found"})
			return
		}
		if !pathIsInside(job.MediaPath, library.DestinationPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library replacement source must remain inside the selected library destination"})
			return
		}
	}
	if input.AudioProfileKey != nil {
		job.AudioProfileKey = *input.AudioProfileKey
		job.AudioProfileSnapshot = nil
	}
	if input.TrackProfileKey != nil {
		job.TrackProfileKey = *input.TrackProfileKey
		job.TrackProfileSnapshot = nil
	}
	if input.AudioProfileKey != nil || input.TrackProfileKey != nil {
		if err := h.captureSupplementalProfiles(&job); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if input.ProfileID != nil || input.TrackProfileKey != nil {
		profileID := job.ProfileID
		if input.ProfileID != nil {
			profileID = *input.ProfileID
		}
		if err := h.captureProfile(&job, profileID, "queue_profile_update"); err != nil {
			h.profileCaptureError(c, err)
			return
		}
	}
	if input.ProcessingMode != nil {
		mode := normalizeQueueProcessingMode(*input.ProcessingMode)
		if strings.TrimSpace(*input.ProcessingMode) != "" && mode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processing mode"})
			return
		}
		job.ProcessingMode = mode
	}

	if input.Notes != "" {
		job.Notes = input.Notes
	}

	canceledRunningProcess := false
	previousStatus := job.Status
	if input.Status != "" {
		if !validJobStatus(input.Status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job status"})
			return
		}

		now := time.Now()
		switch input.Status {
		case JobStatusQueued:
			job.Status = JobStatusQueued
			job.Progress = 0
			job.WorkerName = ""
			job.ErrorMessage = ""
			job.StartedAt = nil
			job.FinishedAt = nil
		case JobStatusCanceled:
			canceledRunningProcess = cancelRunningJobProcess(job.ID)
			job.Status = JobStatusCanceled
			job.FinishedAt = &now
		default:
			job.Status = input.Status
		}
	}

	refreshExecutionPlan := input.ProfileID != nil || input.Status == JobStatusQueued
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if input.Status == JobStatusQueued && previousStatus != JobStatusQueued {
			var highest int64
			if err := tx.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
				return err
			}
			job.QueuePosition = highest + 1
		}
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		if input.Status != "" {
			if err := transitionJobStage(tx, &job, terminalStageForStatus(job.Status)); err != nil {
				return err
			}
		}
		if mediaPathChanged || input.Status == JobStatusQueued {
			if err := scheduler.UpdateAssetLock(tx, job); err != nil {
				return err
			}
		}
		if job.Status == JobStatusCanceled {
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
		}
		if refreshExecutionPlan {
			_, err := scheduler.CreatePendingExecutionPlan(tx, &job, "Execution plan replaced after explicit profile update")
			return err
		}
		return nil
	}); err != nil {
		if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
			c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job.Status == JobStatusCanceled && !canceledRunningProcess {
		_ = cleanupCanceledJob(h.db, job)
	}

	c.JSON(http.StatusOK, job)
}

func (h QueueHandler) captureOverrideOnlyProfile(
	job *models.QueueJob,
	source string,
) error {
	override := currentConversionOverrideForJob(
		*job,
		assetConversionOverrides(h.db),
	)

	if assetConversionOverrideEmpty(override) {
		return fmt.Errorf(
			"override-only video selection requires an asset conversion override",
		)
	}

	// Neutral baseline.
	//
	// This profile is intentionally not loaded from any persisted profile.
	// The asset override becomes the authoritative video configuration.
	profile := models.Profile{
		ID:             0,
		Name:           "Asset Override Only",
		ProfileVersion: 1,

		// MKV is the safest neutral container for MVForge because it can
		// preserve arbitrary audio/subtitle streams without inheriting a
		// container choice from the path profile.
		Container: "mkv",

		// Start from copy semantics. Asset video overrides are then applied
		// explicitly below.
		VideoCodec: "copy",
		AudioCodec: "copy",

		QualityMode:  "crf",
		QualityValue: 24,

		PreserveHDR:       true,
		PreserveSubtitles: true,
		PreserveChapters:  true,

		WorkerConfig: models.JSONMap{},
	}

	profile = applyAssetConversionOverrideToProfile(profile, override)

	// A full_encode override-only job must end up with an actual video
	// configuration. Do not silently inherit any persisted profile.
	videoEncoder := strings.TrimSpace(
		workerStringValue(profile.WorkerConfig["videoEncoder"]),
	)
	videoCodec := strings.TrimSpace(profile.VideoCodec)

	if videoEncoder == "" && (videoCodec == "" || videoCodec == "copy") {
		return fmt.Errorf(
			"override-only video selection requires videoCodec or videoEncoder in the asset override",
		)
	}

	// Asset-level encoder choice must remain authoritative for scheduler
	// planning, just like captureProfile().
	if strings.TrimSpace(override.PreferredEncoder) != "" ||
		strings.TrimSpace(override.VideoEncoder) != "" ||
		strings.TrimSpace(override.VideoCodec) != "" ||
		override.UseHardwareIfAvailable != nil {

		profile.EncoderPolicy = ""
		profile.PreferredEncoder = ""
		profile.AllowedEncoders = nil
		profile.FallbackPolicy = ""
		profile.CodecFamily = ""
		profile.BitDepth = 0
		profile.PixelFormat = ""
		profile.QualityStrategy = ""
	}

	now := time.Now()

	snapshot, err := scheduler.CaptureProfileSnapshot(
		profile,
		now,
		source,
	)
	if err != nil {
		return err
	}

	// Freeze the asset override into the same snapshot mechanism used by
	// normal profiles. The worker will therefore execute the exact override
	// that existed when the job was queued.
	encoded, err := json.Marshal(override)
	if err != nil {
		return err
	}

	var frozen map[string]any
	if err := json.Unmarshal(encoded, &frozen); err != nil {
		return err
	}

	snapshot[assetConversionOverrideSnapshotKey] = frozen
	snapshot["overrideOnly"] = true

	job.ProfileID = 0
	job.ProfileVersion = 1
	job.ProfileSnapshot = snapshot
	job.ProfileCapturedAt = &now

	return nil
}

var errQueueProfileDisabled = errors.New("profile is disabled")

func (h QueueHandler) captureProfile(job *models.QueueJob, profileID uint, source string) error {
	var profile models.Profile
	if err := h.db.First(&profile, profileID).Error; err != nil {
		return err
	}
	if profile.Disabled {
		return errQueueProfileDisabled
	}
	override := currentConversionOverrideForJob(*job, assetConversionOverrides(h.db))
	if !assetConversionOverrideEmpty(override) {
		profile = applyAssetConversionOverrideToProfile(profile, override)
	}
	if strings.TrimSpace(override.PreferredEncoder) != "" ||
		strings.TrimSpace(override.VideoEncoder) != "" ||
		strings.TrimSpace(override.VideoCodec) != "" ||
		override.UseHardwareIfAvailable != nil {
		// Asset processing preference must participate in immutable scheduler
		// planning. Clear the profile's authoritative encoder contract so the
		// effective worker configuration is resolved into the job snapshot.
		profile.EncoderPolicy = ""
		profile.PreferredEncoder = ""
		profile.AllowedEncoders = nil
		profile.FallbackPolicy = ""
		profile.CodecFamily = ""
		profile.BitDepth = 0
		profile.PixelFormat = ""
		profile.QualityStrategy = ""
	}
	now := time.Now()
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, source)
	if err != nil {
		return err
	}
	if !assetConversionOverrideEmpty(override) {
		encoded, encodeErr := json.Marshal(override)
		if encodeErr != nil {
			return encodeErr
		}
		var frozen map[string]any
		if decodeErr := json.Unmarshal(encoded, &frozen); decodeErr != nil {
			return decodeErr
		}
		snapshot[assetConversionOverrideSnapshotKey] = frozen
	}
	job.ProfileID = profile.ID
	job.ProfileVersion = max(profile.ProfileVersion, 1)
	job.ProfileSnapshot = snapshot
	job.ProfileCapturedAt = &now
	return nil
}

func (h QueueHandler) profileCaptureError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile not found"})
	case errors.Is(err, errQueueProfileDisabled):
		c.JSON(http.StatusConflict, gin.H{"error": "profile is disabled"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}

func queueJobConfigChanged(input QueueJobUpdateInput) bool {
	return input.MediaPath != "" || input.LibraryID != nil || input.ProfileID != nil || input.AudioProfileKey != nil || input.TrackProfileKey != nil || input.ProcessingMode != nil
}

func normalizeQueueProcessingMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "full_encode", "full-encode", "fullencode":
		return ProcessingModeFullEncode
	case "audio_only", "audio-only", "audioonly":
		return ProcessingModeAudioOnly
	default:
		return ""
	}
}

func (h QueueHandler) assetHasOpenJob(mediaPath string, excludeID uint) (bool, error) {
	cleanPath := filepath.Clean(mediaPath)
	var jobs []models.QueueJob
	query := h.db.
		Where("dismissed_at IS NULL").
		Where("status <> ?", JobStatusCanceled).
		Where("published_at IS NULL")
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Find(&jobs).Error; err != nil {
		return false, err
	}
	requested, requestedErr := os.Stat(cleanPath)
	for _, job := range jobs {
		candidatePath := filepath.Clean(job.MediaPath)
		if candidatePath == cleanPath {
			return true, nil
		}
		if requestedErr != nil {
			continue
		}
		candidate, err := os.Stat(candidatePath)
		if err == nil && os.SameFile(requested, candidate) {
			return true, nil
		}
	}
	return false, nil
}
