package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	queueCanceledPathPreviewLimit = 500
	queueCanceledPathScanLimit    = 10000
	queueCanceledPathRequestLimit = 500
)

type queueCanceledPathInput struct {
	PathGroup string `json:"pathGroup" binding:"required"`
}

type queueCanceledPathDismissInput struct {
	PathGroup string `json:"pathGroup" binding:"required"`
	JobIDs    []uint `json:"jobIds" binding:"required"`
}

type queueCanceledPathSkipReason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type queueCanceledPathPreviewJob struct {
	ID              uint       `json:"id"`
	Status          string     `json:"status"`
	BatchID         string     `json:"batchId,omitempty"`
	MediaPath       string     `json:"mediaPath"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	ValidationRunID string     `json:"validationRunId,omitempty"`
	PublishedAt     *time.Time `json:"publishedAt,omitempty"`
	CleanupStatus   string     `json:"cleanupStatus,omitempty"`
	CleanupError    string     `json:"cleanupError,omitempty"`
	Dismissible     bool       `json:"dismissible"`
	SkipReason      string     `json:"skipReason,omitempty"`
	DismissedAt     *time.Time `json:"dismissedAt,omitempty"`
}

type queueCanceledPathPreview struct {
	PathGroup            string                        `json:"pathGroup"`
	SelectedJobs         int                           `json:"selectedJobs"`
	DismissibleJobs      int                           `json:"dismissibleJobs"`
	SkippedJobs          int                           `json:"skippedJobs"`
	AlreadyDismissedJobs int                           `json:"alreadyDismissedJobs"`
	SkippedReasons       []queueCanceledPathSkipReason `json:"skippedReasons"`
	Jobs                 []queueCanceledPathPreviewJob `json:"jobs"`
	Truncated            bool                          `json:"truncated"`
}

type queueCanceledPathDismissResult struct {
	PathGroup             string                        `json:"pathGroup"`
	SelectedJobs          int                           `json:"selectedJobs"`
	DismissedJobs         int                           `json:"dismissedJobs"`
	AlreadyDismissedJobs  int                           `json:"alreadyDismissedJobs"`
	SkippedJobs           int                           `json:"skippedJobs"`
	ConflictedJobs        int                           `json:"conflictedJobs"`
	CleanedExecutionPlans int                           `json:"cleanedExecutionPlans"`
	SkippedReasons        []queueCanceledPathSkipReason `json:"skippedReasons"`
}

func (h QueueHandler) PreviewCanceledPath(c *gin.Context) {
	var input queueCanceledPathInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pathGroup, err := canonicalQueuePathGroup(input.PathGroup)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobs, truncated, err := loadQueuePathGroupJobs(h.db, pathGroup, queueCanceledPathPreviewLimit, queueCanceledPathScanLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	preview := queueCanceledPathPreview{PathGroup: pathGroup, Jobs: make([]queueCanceledPathPreviewJob, 0, len(jobs)), Truncated: truncated}
	reasons := map[string]int{}
	for _, job := range jobs {
		row := queueCanceledPathPreviewJob{
			ID: job.ID, Status: job.Status, BatchID: job.BatchID, MediaPath: job.MediaPath,
			StartedAt: job.StartedAt, ValidationRunID: job.ValidationRunID, PublishedAt: job.PublishedAt,
			CleanupStatus: job.CleanupStatus, CleanupError: job.CleanupError, DismissedAt: job.DismissedAt,
		}
		if job.DismissedAt != nil {
			preview.AlreadyDismissedJobs++
			row.SkipReason = "already_dismissed"
		} else if reason := queuePathDismissalSkipReason(job); reason != "" {
			preview.SkippedJobs++
			row.SkipReason = reason
			reasons[reason]++
		} else {
			preview.DismissibleJobs++
			row.Dismissible = true
		}
		preview.Jobs = append(preview.Jobs, row)
	}
	preview.SelectedJobs = preview.DismissibleJobs
	preview.SkippedReasons = sortedQueuePathReasons(reasons)
	c.JSON(http.StatusOK, preview)
}

func (h QueueHandler) DismissCanceledPath(c *gin.Context) {
	var input queueCanceledPathDismissInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pathGroup, err := canonicalQueuePathGroup(input.PathGroup)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.JobIDs) == 0 || len(input.JobIDs) > queueCanceledPathRequestLimit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "select between 1 and 500 queue jobs"})
		return
	}
	ids := uniqueSortedQueueJobIDs(input.JobIDs)
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "select between 1 and 500 queue jobs"})
		return
	}

	result := queueCanceledPathDismissResult{PathGroup: pathGroup, SelectedJobs: len(ids)}
	reasons := map[string]int{}
	now := time.Now().UTC()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			var job models.QueueJob
			if err := tx.First(&job, id).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					result.ConflictedJobs++
					reasons["not_found"]++
					continue
				}
				return err
			}
			if queueGroupFromMediaPath(job.MediaPath) != pathGroup {
				result.ConflictedJobs++
				reasons["path_group_changed"]++
				continue
			}
			if job.DismissedAt != nil {
				result.AlreadyDismissedJobs++
				continue
			}
			if reason := queuePathDismissalSkipReason(job); reason != "" {
				result.SkippedJobs++
				reasons[reason]++
				continue
			}

			updated := tx.Model(&models.QueueJob{}).
				Where("id = ? AND status = ? AND dismissed_at IS NULL AND cancellation_requested_at IS NULL AND updated_at = ?", job.ID, JobStatusCanceled, job.UpdatedAt).
				Update("dismissed_at", now)
			if updated.Error != nil {
				return updated.Error
			}
			rows := updated.RowsAffected
			if rows != 1 {
				result.ConflictedJobs++
				reasons["changed_during_dismissal"]++
				continue
			}
			if job.ExecutionNumber == nil && job.StartedAt == nil {
				plans := tx.Where("job_id = ?", job.ID).Delete(&models.ExecutionPlan{})
				if plans.Error != nil {
					return plans.Error
				}
				result.CleanedExecutionPlans += int(plans.RowsAffected)
			}
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			result.DismissedJobs++
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result.SkippedReasons = sortedQueuePathReasons(reasons)
	c.JSON(http.StatusOK, result)
}

func canonicalQueuePathGroup(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.TrimSpace(value) == "" || len(value) > 4096 {
		return "", fmt.Errorf("path group must be non-empty and at most 4096 characters")
	}
	if value == "Root" {
		return "Root", nil
	}
	return value, nil
}

func queueGroupFromMediaPath(mediaPath string) string {
	normalized := strings.ReplaceAll(mediaPath, "\\", "/")
	normalized = strings.TrimRight(normalized, "/")
	index := strings.LastIndex(normalized, "/")
	if index <= 0 {
		return "Root"
	}
	return normalized[:index]
}

func queuePathDismissalSkipReason(job models.QueueJob) string {
	if job.Status != JobStatusCanceled {
		return "status_" + strings.TrimSpace(job.Status)
	}
	if job.CancellationRequestedAt != nil {
		return "cancellation_pending"
	}
	if job.ValidationRunID != "" || job.ValidationOutcome != "" || job.ValidationStatus != "" || len(job.ValidationReport) > 0 || job.ValidationScore != 0 || job.ValidationOverride || job.ValidationActionable {
		return "validation_owned"
	}
	if job.PublishedAt != nil || job.PublishedPath != "" || job.PublishedFingerprint != "" || job.PublicationRetiredAt != nil || job.PublicationID != "" || job.PublicationState != "" || job.OriginalArchivedPath != "" || job.OriginalArchiveStatus != "" {
		return "publication_owned"
	}
	return ""
}

func loadQueuePathGroupJobs(db *gorm.DB, pathGroup string, resultLimit, scanLimit int) ([]models.QueueJob, bool, error) {
	jobs := make([]models.QueueJob, 0, resultLimit)
	remainingScan := scanLimit
	truncated := false
	loadedDismissedRows := false
	for _, dismissed := range []bool{false, true} {
		if len(jobs) >= resultLimit || remainingScan <= 0 {
			break
		}
		if dismissed {
			loadedDismissedRows = true
		}
		stateJobs, stateTruncated, scanned, err := loadQueuePathGroupState(db, pathGroup, resultLimit-len(jobs), remainingScan, dismissed)
		if err != nil {
			return nil, false, err
		}
		jobs = append(jobs, stateJobs...)
		remainingScan -= scanned
		if stateTruncated {
			truncated = true
			break
		}
	}
	if len(jobs) >= resultLimit && !truncated && !loadedDismissedRows && remainingScan > 0 {
		moreDismissed, probeTruncated, _, err := loadQueuePathGroupState(db, pathGroup, 0, remainingScan, true)
		if err != nil {
			return nil, false, err
		}
		truncated = len(moreDismissed) > 0 || probeTruncated
	}
	return jobs, truncated, nil
}

func loadQueuePathGroupState(db *gorm.DB, pathGroup string, resultLimit, scanLimit int, dismissed bool) ([]models.QueueJob, bool, int, error) {
	jobs := make([]models.QueueJob, 0, resultLimit)
	var lastID uint
	var scanned int
	truncated := false
	for scanned < scanLimit && len(jobs) <= resultLimit {
		pageSize := 500
		if remaining := scanLimit - scanned; remaining < pageSize {
			pageSize = remaining
		}
		var candidates []models.QueueJob
		query := db.Model(&models.QueueJob{}).Where("id > ?", lastID)
		if dismissed {
			query = query.Where("dismissed_at IS NOT NULL")
		} else {
			query = query.Where("dismissed_at IS NULL")
		}
		query = addQueuePathGroupCandidateFilter(query, pathGroup)
		if err := query.Order("id asc").Limit(pageSize).Find(&candidates).Error; err != nil {
			return nil, false, scanned, err
		}
		if len(candidates) == 0 {
			break
		}
		scanned += len(candidates)
		lastID = candidates[len(candidates)-1].ID
		for _, job := range candidates {
			if queueGroupFromMediaPath(job.MediaPath) == pathGroup {
				jobs = append(jobs, job)
				if len(jobs) > resultLimit {
					truncated = true
					break
				}
			}
		}
		if truncated {
			break
		}
		if len(candidates) < pageSize {
			break
		}
	}
	if scanned >= scanLimit {
		truncated = true
	}
	if len(jobs) > resultLimit {
		jobs = jobs[:resultLimit]
	}
	return jobs, truncated, scanned, nil
}

func addQueuePathGroupCandidateFilter(query *gorm.DB, pathGroup string) *gorm.DB {
	mediaPath := "COALESCE(NULLIF(media_path, ''), media_path_key)"
	if pathGroup == "Root" {
		rootPaths := "((instr(" + mediaPath + ", '/') = 0 AND instr(" + mediaPath + ", char(92)) = 0) OR (substr(" + mediaPath + ", 1, 1) = '/' AND instr(substr(" + mediaPath + ", 2), '/') = 0 AND instr(substr(" + mediaPath + ", 2), char(92)) = 0) OR (substr(" + mediaPath + ", 1, 1) = char(92) AND instr(substr(" + mediaPath + ", 2), '/') = 0 AND instr(substr(" + mediaPath + ", 2), char(92)) = 0))"
		return query.Where("("+rootPaths+") OR "+mediaPath+" LIKE ? ESCAPE '\\' OR "+mediaPath+" LIKE ? ESCAPE '\\'", escapeQueuePathLike("Root")+"/%", escapeQueuePathLike("Root\\")+"%")
	}
	forwardPattern := escapeQueuePathLike(pathGroup) + "/%"
	backslashPattern := escapeQueuePathLike(strings.ReplaceAll(pathGroup, "/", "\\")) + "\\\\%"
	return query.Where("("+mediaPath+" LIKE ? ESCAPE '\\' OR "+mediaPath+" LIKE ? ESCAPE '\\')", forwardPattern, backslashPattern)
}

func escapeQueuePathLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

func uniqueSortedQueueJobIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func sortedQueuePathReasons(reasons map[string]int) []queueCanceledPathSkipReason {
	keys := make([]string, 0, len(reasons))
	for reason, count := range reasons {
		if count > 0 {
			keys = append(keys, reason)
		}
	}
	sort.Strings(keys)
	result := make([]queueCanceledPathSkipReason, 0, len(keys))
	for _, reason := range keys {
		result = append(result, queueCanceledPathSkipReason{Reason: reason, Count: reasons[reason]})
	}
	return result
}
