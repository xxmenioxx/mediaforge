package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCanceled  = "canceled"
)

var (
	errWorkerLimitReached        = errors.New("worker claim limit reached")
	errWorkerDelayActive         = errors.New("worker delay between jobs is still active")
	errWorkerBatchCooldownActive = errors.New("worker batch cooldown is still active")
	claimJobMu                   sync.Mutex
)

type WorkerHandler struct {
	db *gorm.DB
}

type ClaimJobInput struct {
	WorkerName     string          `json:"workerName"`
	Encoders       models.JSONList `json:"encoders,omitempty"`
	RuntimeProfile string          `json:"runtimeProfile,omitempty"`
}

type UpdateJobStatusInput struct {
	Status       string `json:"status" binding:"required"`
	Progress     int    `json:"progress"`
	OutputPath   string `json:"outputPath"`
	ErrorMessage string `json:"errorMessage"`
}

type ExecuteJobInput struct {
	Overwrite bool `json:"overwrite"`
}

type audioEnhancementProfile struct {
	Key              string
	Filters          string
	OutputCodec      string
	RNNoiseModelPath string
	ChannelMode      string
	ForceStereoMode  string
	StereoDelayMs    float64
	StereoWidth      float64
	EQBands          map[string]float64
	TargetLoudness   float64
	TruePeak         float64
}

type workerLimits struct {
	DefaultWorkerName       string
	AutoWorkerEnabled       bool
	MaxConcurrentJobs       int
	MaxJobsPerBatch         int
	DelaySecondsBetweenJobs int
	BatchCooldownSeconds    int
}

func NewWorkerHandler(db *gorm.DB) WorkerHandler {
	return WorkerHandler{db: db}
}

func (h WorkerHandler) ListNodes(c *gin.Context) {
	var workers []models.WorkerNode
	if err := h.db.Order("name asc").Find(&workers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cutoff := time.Now().Add(-scheduler.WorkerHeartbeatTimeout)
	for i := range workers {
		if workers[i].LastSeenAt.Before(cutoff) && workers[i].Status != "offline" {
			workers[i].Status = "offline"
			_ = h.db.Model(&workers[i]).Update("status", "offline").Error
		}
	}
	c.JSON(http.StatusOK, workers)
}

func (h WorkerHandler) heartbeatWorker(name string, limits workerLimits, reportedEncoders models.JSONList, reportedRuntimeProfile string) error {
	if strings.TrimSpace(name) == "" {
		name = limits.DefaultWorkerName
	}
	if strings.TrimSpace(name) == "" {
		name = "local-worker"
	}
	encoders := models.JSONList{}
	runtimeProfile := ""
	if len(reportedEncoders) > 0 {
		for _, raw := range reportedEncoders {
			encoder, ok := raw.(string)
			if ok && strings.TrimSpace(encoder) != "" && !jsonListContains(encoders, strings.TrimSpace(encoder)) {
				encoders = append(encoders, strings.TrimSpace(encoder))
			}
		}
		runtimeProfile = strings.TrimSpace(reportedRuntimeProfile)
	} else if snapshot, err := runtimeinfo.Latest(h.db); err == nil {
		runtimeProfile = snapshot.SelectedProfile
		for encoder, raw := range snapshot.Encoders {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			usable, _ := entry["usable"].(bool)
			if usable && !jsonListContains(encoders, encoder) {
				encoders = append(encoders, encoder)
			}
		}
	}
	var worker models.WorkerNode
	result := h.db.Where("name = ?", name).Limit(1).Find(&worker)
	if result.Error != nil {
		return result.Error
	}
	now := time.Now()
	if result.RowsAffected == 0 {
		worker = models.WorkerNode{Name: name, Status: "online", MaxConcurrentJobs: max(limits.MaxConcurrentJobs, 1), Encoders: encoders, RuntimeProfile: runtimeProfile, LastSeenAt: now}
		return h.db.Create(&worker).Error
	}
	return h.db.Model(&worker).Updates(map[string]any{"status": "online", "max_concurrent_jobs": max(limits.MaxConcurrentJobs, 1), "encoders": encoders, "runtime_profile": runtimeProfile, "last_seen_at": now}).Error
}

func jsonListContains(values models.JSONList, expected string) bool {
	for _, raw := range values {
		if value, ok := raw.(string); ok && value == expected {
			return true
		}
	}
	return false
}

func (h WorkerHandler) workerLimits() (workerLimits, error) {
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := h.db.First(&setting, "key = ?", "workers").Error; err == nil && setting.Value != nil {
		values = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return workerLimits{}, err
	}

	return workerLimits{
		DefaultWorkerName:       stringSetting(values, "defaultWorkerName", "local-worker"),
		AutoWorkerEnabled:       boolSetting(values["autoWorkerEnabled"], true),
		MaxConcurrentJobs:       intSetting(values, "maxConcurrentJobs", 1),
		MaxJobsPerBatch:         intSetting(values, "maxJobsPerBatch", 10),
		DelaySecondsBetweenJobs: intSetting(values, "delaySecondsBetweenJobs", 30),
		BatchCooldownSeconds:    intSetting(values, "batchCooldownSeconds", 600),
	}, nil
}

func forceFreshSnapshotBeforeExecution(db *gorm.DB) bool {
	if db == nil {
		return false
	}

	var setting models.AppSetting
	if err := db.
		First(&setting, "key = ?", "pipelineAutomation").
		Error; err != nil {
		return false
	}

	if setting.Value == nil {
		return false
	}

	return boolSetting(
		setting.Value["forceFreshSnapshotBeforeExecution"],
		false,
	)
}

func canClaimJob(tx *gorm.DB, limits workerLimits) error {
	if limits.MaxConcurrentJobs > 0 {
		var running int64
		if err := tx.Model(&models.QueueJob{}).Where("status = ?", JobStatusRunning).Count(&running).Error; err != nil {
			return err
		}
		if running >= int64(limits.MaxConcurrentJobs) {
			return errWorkerLimitReached
		}
	}

	if limits.DelaySecondsBetweenJobs > 0 {
		var lastStarted models.QueueJob
		err := tx.Where("started_at IS NOT NULL").Order("started_at desc").First(&lastStarted).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == nil && lastStarted.StartedAt != nil && time.Since(*lastStarted.StartedAt) < time.Duration(limits.DelaySecondsBetweenJobs)*time.Second {
			return errWorkerDelayActive
		}
	}

	if limits.MaxJobsPerBatch > 0 && limits.BatchCooldownSeconds > 0 {
		var recentStarted []models.QueueJob
		if err := tx.Where("started_at IS NOT NULL").
			Order("started_at desc").
			Limit(limits.MaxJobsPerBatch).
			Find(&recentStarted).Error; err != nil {
			return err
		}
		if len(recentStarted) >= limits.MaxJobsPerBatch {
			oldestInBatch := recentStarted[len(recentStarted)-1]
			if oldestInBatch.StartedAt != nil && time.Since(*oldestInBatch.StartedAt) < time.Duration(limits.BatchCooldownSeconds)*time.Second {
				return errWorkerBatchCooldownActive
			}
		}
	}

	return nil
}

func nextClaimableJob(tx *gorm.DB, limits workerLimits) (models.QueueJob, error) {
	var jobs []models.QueueJob
	if err := tx.Model(&models.QueueJob{}).
		Joins("LEFT JOIN execution_plans ON execution_plans.id = queue_jobs.active_execution_plan_id").
		Where("queue_jobs.status = ?", JobStatusQueued).
		Where("queue_jobs.active_execution_plan_id IS NULL OR execution_plans.status = ?", scheduler.ExecutionPlanReady).
		Order("queue_jobs.queue_position asc, queue_jobs.priority asc, queue_jobs.created_at asc").
		Find(&jobs).Error; err != nil {
		return models.QueueJob{}, err
	}
	if len(jobs) == 0 {
		return models.QueueJob{}, gorm.ErrRecordNotFound
	}

	for _, job := range jobs {
		if job.ActiveExecutionPlanID == nil {
			return job, nil
		}
		var plan models.ExecutionPlan
		if err := tx.First(&plan, *job.ActiveExecutionPlanID).Error; err != nil {
			return models.QueueJob{}, err
		}
		workspaceDecision, err := scheduler.EvaluateWorkspace(tx, plan)
		if err != nil {
			return models.QueueJob{}, err
		}
		plan.Evaluation["workspace"] = workspaceDecision
		plan.WorkspaceMode = workspaceDecision.Mode
		if !workspaceDecision.Allowed {
			plan.Status, plan.WaitingState = scheduler.ExecutionPlanWaiting, "WAITING_SSD_SPACE"
			plan.DecisionReasons = append(plan.DecisionReasons, workspaceDecision.Reason)
			if err := tx.Save(&plan).Error; err != nil {
				return models.QueueJob{}, err
			}
			continue
		}
		workingDecision, classification, err := scheduler.EvaluatePlanWorkingHours(tx, time.Now())
		if err != nil {
			return models.QueueJob{}, err
		}
		plan.Evaluation["workingHours"] = workingDecision
		plan.Evaluation["jobClassification"] = classification
		if !workingDecision.Allowed {
			plan.Status, plan.WaitingState = scheduler.ExecutionPlanWaiting, "WAITING_SCHEDULE_WINDOW"
			plan.DecisionReasons = append(plan.DecisionReasons, workingDecision.Reason)
			if err := tx.Save(&plan).Error; err != nil {
				return models.QueueJob{}, err
			}
			continue
		}
		resourceDecision, err := scheduler.EvaluateResources(tx, &plan)
		if err != nil {
			return models.QueueJob{}, err
		}
		if resourceDecision.Allowed {
			return job, nil
		}
		plan.Status, plan.WaitingState = scheduler.ExecutionPlanWaiting, resourceDecision.WaitingState
		plan.Evaluation["resources"] = resourceDecision
		plan.Evaluation["resourceWaitReasons"] = resourceDecision.Reasons
		plan.DecisionReasons = append(plan.DecisionReasons, "Dispatch deferred because scheduler resources are unavailable")
		if err := tx.Save(&plan).Error; err != nil {
			return models.QueueJob{}, err
		}
	}
	return models.QueueJob{}, gorm.ErrRecordNotFound
}

func (h WorkerHandler) ClaimNext(c *gin.Context) {
	var input ClaimJobInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claimed, err := h.claimNextJob(input.WorkerName, input.Encoders, input.RuntimeProfile)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			message := "no runnable queued jobs are available"
			var job models.QueueJob
			if h.db.Where("status = ?", JobStatusQueued).Order("queue_position asc, priority asc, created_at asc").First(&job).Error == nil && job.ActiveExecutionPlanID != nil {
				var plan models.ExecutionPlan
				if h.db.First(&plan, *job.ActiveExecutionPlanID).Error == nil && plan.WaitingState != "" {
					message = fmt.Sprintf("queued job %d is blocked in %s", job.ID, plan.WaitingState)
				}
			}
			c.JSON(http.StatusNotFound, gin.H{"error": message})
			return
		}
		if err == errWorkerLimitReached {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "worker claim limit reached"})
			return
		}
		if err == errWorkerDelayActive {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "worker delay between jobs is still active"})
			return
		}
		if err == errWorkerBatchCooldownActive {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "worker batch cooldown is still active"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, claimed)
}

func (h WorkerHandler) claimNextJob(workerName string, reportedEncoders models.JSONList, reportedRuntimeProfile string) (models.QueueJob, error) {
	// SQLite only permits one writer at a time. More importantly, slot checks,
	// reservation activation, and the job transition must be atomic from the
	// perspective of every manual and automatic worker in this process.
	claimJobMu.Lock()
	defer claimJobMu.Unlock()

	limits, err := h.workerLimits()
	if err != nil {
		return models.QueueJob{}, err
	}
	if workerName == "" {
		workerName = limits.DefaultWorkerName
	}
	if err := h.heartbeatWorker(workerName, limits, reportedEncoders, reportedRuntimeProfile); err != nil {
		return models.QueueJob{}, err
	}
	if workerName == "" {
		workerName = "local-worker"
	}

	var claimed models.QueueJob
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := canClaimJob(tx, limits); err != nil {
			return err
		}

		job, err := nextClaimableJob(tx, limits)
		if err != nil {
			return err
		}
		plan := models.ExecutionPlan{}
		if job.ActiveExecutionPlanID != nil {
			if err := tx.First(&plan, *job.ActiveExecutionPlanID).Error; err != nil {
				return err
			}
		}
		if err := scheduler.ActivateReservation(tx, job, plan, workerName); err != nil {
			return err
		}

		now := time.Now()
		if err := assignExecutionNumber(tx, &job); err != nil {
			return err
		}
		job.Status = JobStatusRunning
		job.Progress = 1
		job.WorkerName = workerName
		job.ErrorMessage = ""
		job.StartedAt = &now
		job.FinishedAt = nil

		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStageClaimed); err != nil {
			return err
		}
		if job.ActiveExecutionPlanID != nil {
			if err := tx.Model(&models.ExecutionPlan{}).Where("id = ?", *job.ActiveExecutionPlanID).Updates(map[string]any{"status": scheduler.ExecutionPlanDispatched, "waiting_state": ""}).Error; err != nil {
				return err
			}
		}

		claimed = job
		return nil
	})
	if err != nil {
		if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
			return models.QueueJob{}, gorm.ErrRecordNotFound
		}
		return models.QueueJob{}, err
	}
	return claimed, nil
}

func assignExecutionNumber(tx *gorm.DB, job *models.QueueJob) error {
	if job.ExecutionNumber != nil {
		return nil
	}
	var next uint
	if err := tx.Model(&models.QueueJob{}).
		Select("COALESCE(MAX(execution_number), 0) + 1").
		Scan(&next).Error; err != nil {
		return err
	}
	job.ExecutionNumber = &next
	return nil
}

func (h WorkerHandler) UpdateJobStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	var input UpdateJobStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !validJobStatus(input.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job status"})
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

	now := time.Now()
	job.Status = input.Status
	job.Progress = normalizedProgress(input.Status, input.Progress)
	if input.OutputPath != "" {
		job.OutputPath = input.OutputPath
	}
	if input.ErrorMessage != "" {
		job.ErrorMessage = input.ErrorMessage
	}

	canceledRunningProcess := false
	if job.Status == JobStatusCanceled {
		canceledRunningProcess = cancelRunningJobProcess(job.ID)
	}

	if job.Status == JobStatusRunning && job.StartedAt == nil {
		job.StartedAt = &now
	}

	if terminalJobStatus(job.Status) {
		job.FinishedAt = &now
	}

	if err := h.db.Save(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := transitionJobStage(h.db, &job, terminalStageForStatus(job.Status)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job.Status == JobStatusCanceled {
		_ = scheduler.ReleaseReservation(h.db, job.ID)
		if !canceledRunningProcess {
			_ = cleanupCanceledJob(h.db, job)
		}
	} else if terminalJobStatus(job.Status) {
		_ = scheduler.DeactivateReservationResources(h.db, job.ID)
	}

	c.JSON(http.StatusOK, job)
}

func (h WorkerHandler) DryRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
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

	if job.Status != JobStatusRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job must be running before dry-run execution"})
		return
	}

	var library models.Library
	if err := h.db.First(&library, job.LibraryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	profile, err := h.profileForJob(job)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	override := conversionOverrideForJob(job, assetConversionOverrides(h.db))
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	effectiveProfile = applyFrozenCadenceResolution(job, effectiveProfile)
	effectiveProfile, err = resolveAutomaticFrameStructure(
		h.db,
		job.MediaPath,
		effectiveProfile,
	)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": err.Error(),
		})
		return
	}
	paths, err := h.pathSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	outputPath := plannedStagingOutputPath(job, effectiveProfile, paths)
	inputPath := job.MediaPath
	if job.ActiveExecutionPlanID != nil {
		var executionPlan models.ExecutionPlan
		if h.db.First(&executionPlan, *job.ActiveExecutionPlanID).Error == nil && executionPlan.WorkspaceMode == scheduler.WorkspaceModeCopyToWork {
			inputPath = filepath.Join(paths.stagingPath, fmt.Sprintf("job-%d", job.ID), "_input", filepath.Base(job.MediaPath))
		}
	}
	audioProfile, err := h.audioProfileForJob(job)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	plan, err := buildMediaJobPlanWithOverride(inputPath, outputPath, effectiveProfile, audioProfile, true, override)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	applyEpisodeVideoTrackTitle(h.db, &plan, job, library)
	args := FFmpegCommandBuilder{}.Build(plan)
	command := dryRunCommandFromArgs(args)

	job.Status = JobStatusRunning
	if job.Progress < 5 {
		job.Progress = 5
	}
	job.OutputPath = outputPath
	job.PlannedPublishedPath = plannedOutputPathForJob(h.db, job, library, effectiveProfile)
	job.ErrorMessage = ""
	job.Notes = appendNote(job.Notes, "Dry-run command: "+command)
	if !assetConversionOverrideEmpty(override) {
		job.Notes = appendNote(job.Notes, "Asset conversion overrides applied")
	}
	for _, warning := range plan.StreamValidationWarnings {
		job.Notes = appendNote(job.Notes, "Stream validation warning: "+warning)
	}
	job.FinishedAt = nil

	if err := h.db.Save(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
}

func (h WorkerHandler) Execute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	var input ExecuteJobInput
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

	job, status, err := h.executeQueueJob(job, input.Overwrite)
	if err != nil {
		h.failClaimedJobExecution(job.ID, err)
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, job)
}

// failClaimedJobExecution closes the gap between reserving a job and starting
// FFmpeg. A missing input, profile, path setting, or workspace preparation
// failure must not leave an active reservation permanently stuck at "claimed".
func (h WorkerHandler) failClaimedJobExecution(jobID uint, executionErr error) {
	if jobID == 0 || executionErr == nil || hasRunningJobProcess(jobID) {
		return
	}
	var job models.QueueJob
	if err := h.db.First(&job, jobID).Error; err != nil || job.Status != JobStatusRunning {
		return
	}
	now := time.Now()
	job.Status = JobStatusFailed
	job.Progress = 0
	job.ErrorMessage = executionErr.Error()
	job.FinishedAt = &now
	job.Notes = appendNote(job.Notes, "Execution failed after the worker claimed the job; its reservation was released")
	_ = transitionJobStage(h.db, &job, JobStageFailed)
	_ = scheduler.DeactivateReservationResources(h.db, job.ID)
	_ = h.db.Save(&job).Error
}

func (h WorkerHandler) executeQueueJob(job models.QueueJob, overwrite bool) (models.QueueJob, int, error) {
	if job.Status != JobStatusRunning {
		return job, http.StatusBadRequest, fmt.Errorf("job must be running before conversion execution")
	}

	if _, err := os.Stat(job.MediaPath); err != nil {
		return job, http.StatusBadRequest, fmt.Errorf("input media is not readable: %v", err)
	}

	var library models.Library
	if err := h.db.First(&library, job.LibraryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return job, http.StatusNotFound, fmt.Errorf("library not found")
		}
		return job, http.StatusInternalServerError, err
	}

	profile, err := h.profileForJob(job)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return job, http.StatusNotFound, fmt.Errorf("profile not found")
		}
		return job, http.StatusInternalServerError, err
	}

	paths, err := h.pathSettings()
	if err != nil {
		return job, http.StatusInternalServerError, err
	}

	if forceFreshSnapshotBeforeExecution(h.db) {
		if _, err := refreshSnapshotBeforeExecution(
			h.db,
			job.MediaPath,
		); err != nil {
			return job,
				http.StatusUnprocessableEntity,
				fmt.Errorf(
					"forced fresh snapshot before execution: %w",
					err,
				)
		}

		job.Notes = appendNote(
			job.Notes,
			"Fresh asset snapshot generated before execution",
		)

		if err := h.db.Save(&job).Error; err != nil {
			return job, http.StatusInternalServerError, err
		}
	}

	if err := transitionJobStage(h.db, &job, JobStagePreparingWorkspace); err != nil {
		return job, http.StatusInternalServerError, err
	}
	inputPath := job.MediaPath
	workspaceMode := scheduler.WorkspaceModeDirect
	selectedEncoder := ""
	if job.ActiveExecutionPlanID != nil {
		var executionPlan models.ExecutionPlan
		if err := h.db.First(&executionPlan, *job.ActiveExecutionPlanID).Error; err == nil {
			if executionPlan.WorkspaceMode != "" {
				workspaceMode = executionPlan.WorkspaceMode
			}
			selectedEncoder = strings.TrimSpace(executionPlan.SelectedEncoder)
		}
	}
	if workspaceMode == scheduler.WorkspaceModeCopyToWork {
		if err := transitionJobStage(h.db, &job, JobStageCopyingWorkspace); err != nil {
			return job, http.StatusInternalServerError, err
		}
		inputPath = filepath.Join(paths.stagingPath, fmt.Sprintf("job-%d", job.ID), "_input", filepath.Base(job.MediaPath))
		if err := copyPublishedFile(job.MediaPath, inputPath, true); err != nil {
			return job, http.StatusInternalServerError, fmt.Errorf("prepare workspace input: %w", err)
		}
		job.Notes = appendNote(job.Notes, "Workspace input copied to: "+inputPath)
	}

	override := conversionOverrideForJob(job, assetConversionOverrides(h.db))
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	effectiveProfile = applySelectedEncoder(effectiveProfile, selectedEncoder)
	effectiveProfile = applyFrozenCadenceResolution(job, effectiveProfile)

	effectiveProfile, err = resolveAutomaticFrameStructure(
		h.db,
		job.MediaPath,
		effectiveProfile,
	)
	if err != nil {
		return job, http.StatusUnprocessableEntity, err
	}
	if effectiveProfile.WorkerConfig == nil {
		effectiveProfile.WorkerConfig = models.JSONMap{}
	} else {
		workerConfig := models.JSONMap{}
		for key, value := range effectiveProfile.WorkerConfig {
			workerConfig[key] = value
		}
		effectiveProfile.WorkerConfig = workerConfig
	}
	effectiveProfile.WorkerConfig["qsvAssetAnalysisPath"] = job.MediaPath
	if frozen, ok := job.ProfileSnapshot[interlaceAnalysisSnapshotKey]; ok {
		effectiveProfile.WorkerConfig[interlaceAnalysisSnapshotKey] = frozen
	}
	if frozen, ok := job.ProfileSnapshot[cadenceAnalysisSnapshotKey]; ok {
		effectiveProfile.WorkerConfig[cadenceAnalysisSnapshotKey] = frozen
	}
	if frozen, ok := job.ProfileSnapshot[cadenceRecommendationSnapshotKey]; ok {
		effectiveProfile.WorkerConfig[cadenceRecommendationSnapshotKey] = frozen
	}
	outputPath := plannedStagingOutputPath(job, effectiveProfile, paths)
	if !overwrite {
		if _, err := os.Stat(outputPath); err == nil {
			return job, http.StatusConflict, fmt.Errorf("staging output already exists; retry with overwrite enabled")
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return job, http.StatusInternalServerError, err
	}

	audioProfile, err := h.audioProfileForJob(job)
	if err != nil {
		return job, http.StatusInternalServerError, err
	}
	if err := transitionJobStage(h.db, &job, JobStageAnalyzingAsIs); err != nil {
		return job, http.StatusInternalServerError, err
	}

	plan, err := buildMediaJobPlanWithOverride(inputPath, outputPath, effectiveProfile, audioProfile, overwrite, override)
	if err != nil {
		return job, http.StatusInternalServerError, err
	}
	plan.SourceAssetPath = job.MediaPath
	applyEpisodeVideoTrackTitle(h.db, &plan, job, library)
	if len(plan.Override.SubtitleTransforms) > 0 {
		if err := transitionJobStage(h.db, &job, JobStagePreparingSubtitles); err != nil {
			return job, http.StatusInternalServerError, err
		}
		job.Progress = 3
		_ = h.db.Save(&job).Error
	}
	subtitleArtifacts, err := generateSubtitleArtifacts(context.Background(), plan)
	if err != nil {
		job.Status = JobStatusFailed
		job.ErrorMessage = err.Error()
		job.Notes = appendNote(job.Notes, "Subtitle transformation failed before media conversion: "+err.Error())
		_ = transitionJobStage(h.db, &job, JobStageFailed)
		_ = scheduler.DeactivateReservationResources(h.db, job.ID)
		_ = h.db.Save(&job).Error
		return job, http.StatusUnprocessableEntity, err
	}
	job.SubtitleArtifacts = subtitleArtifactsJSON(subtitleArtifacts)
	args := FFmpegCommandBuilder{}.Build(plan)
	command := "ffmpeg " + shellJoin(args)
	now := time.Now()
	job.Progress = 5
	job.OutputPath = outputPath
	job.PlannedPublishedPath = plannedOutputPathForJob(h.db, job, library, effectiveProfile)
	job.ErrorMessage = ""
	job.Notes = appendNote(job.Notes, "Conversion command: "+command)
	encoderDecision := encoderDecisionForProfile(plan.Profile)
	job.Notes = appendNote(job.Notes, fmt.Sprintf("Encoder decision: requested=%s effective=%s", stringFromUnknown(encoderDecision["requested"]), stringFromUnknown(encoderDecision["effective"])))
	if resolvedVideoEncoder(plan.Profile) == "hevc_videotoolbox" {
		job.Notes = appendNote(job.Notes, fmt.Sprintf("VideoToolbox strategy: realtime requested=%t effective=%t; B-frames requested=%s effective=%s; multiplier=%.2f; observed=%d; fallback=%s",
			profileWorkerBool(plan.Profile, "videoToolboxRequestedRealtime", profileWorkerBool(plan.Profile, "videoToolboxRealtime", false)),
			profileWorkerBool(plan.Profile, "videoToolboxEffectiveRealtime", profileWorkerBool(plan.Profile, "videoToolboxRealtime", false)),
			workerStringValue(plan.Profile.WorkerConfig["videoToolboxRequestedBFramePolicy"]),
			workerStringValue(plan.Profile.WorkerConfig["videoToolboxEffectiveBFramePolicy"]),
			workerNumberValue(plan.Profile.WorkerConfig["videoToolboxBFrameEfficiencyMultiplier"], 1),
			workerIntValue(plan.Profile.WorkerConfig["videoToolboxObservedBFrameCount"], 0),
			workerStringValue(plan.Profile.WorkerConfig["videoToolboxBFrameDowngradeReason"])))
	}
	if effectiveRateControl := workerStringValue(plan.Profile.WorkerConfig["qsvEffectiveRateControl"]); effectiveRateControl != "" {
		job.Notes = appendNote(job.Notes, fmt.Sprintf("QSV rate control: requested=%s effective=%s fallback=%s",
			workerStringValue(plan.Profile.WorkerConfig["qsvRequestedRateControl"]), effectiveRateControl,
			workerStringValue(plan.Profile.WorkerConfig["qsvRateControlFallbackReason"])))
	}
	if adjustment := workerIntValue(plan.Profile.WorkerConfig["qsvAssetQualityAdjustment"], 0); adjustment != 0 {
		job.Notes = appendNote(job.Notes, fmt.Sprintf("QSV asset quality: requested=%d adjustment=%+d effective=%d reasons=%s",
			workerIntValue(plan.Profile.WorkerConfig["qsvRequestedGlobalQuality"], 0), adjustment,
			workerIntValue(plan.Profile.WorkerConfig["qsvEffectiveGlobalQuality"], 0),
			strings.Join(workerStringSlice(plan.Profile.WorkerConfig["qsvAssetQualityReasons"]), "; ")))
	}
	if encoderDecision["downgraded"] == true {
		job.Notes = appendNote(job.Notes, "Encoder downgrade: "+stringFromUnknown(encoderDecision["reason"]))
	}
	if !assetConversionOverrideEmpty(override) {
		job.Notes = appendNote(job.Notes, "Asset conversion overrides applied")
	}
	for _, warning := range plan.StreamValidationWarnings {
		job.Notes = appendNote(job.Notes, "Stream validation warning: "+warning)
	}
	if audioProfile != nil {
		job.Notes = appendNote(job.Notes, "Audio enhancement profile: "+audioProfile.Key)
		job.Notes = appendNote(job.Notes, "Processing mode: "+plan.ProcessingMode)
	}
	if err := writeJobAsIsArtifact(h.db, job, plan.Profile, audioProfile, command, plan.ProcessingMode, resolveEffectiveStreamPlan(plan)); err != nil {
		job.Notes = appendNote(job.Notes, "AS-IS artifact warning: "+err.Error())
	}
	if job.StartedAt == nil {
		job.StartedAt = &now
	}
	if err := h.db.Save(&job).Error; err != nil {
		return job, http.StatusInternalServerError, err
	}
	if err := transitionJobStage(h.db, &job, JobStageConverting); err != nil {
		return job, http.StatusInternalServerError, err
	}

	if err := h.startFFmpegJob(job.ID, args, plan.Streams.Duration); err != nil {
		job.Status = JobStatusFailed
		job.Progress = 0
		job.ErrorMessage = err.Error()
		finishedAt := time.Now()
		job.FinishedAt = &finishedAt
		_ = transitionJobStage(h.db, &job, JobStageFailed)
		_ = scheduler.DeactivateReservationResources(h.db, job.ID)
		if saveErr := h.db.Save(&job).Error; saveErr != nil {
			return job, http.StatusInternalServerError, saveErr
		}
		return job, http.StatusInternalServerError, err
	}

	return job, http.StatusAccepted, nil
}

func applyFrozenCadenceResolution(job models.QueueJob, profile models.Profile) models.Profile {
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	if frozen, ok := job.ProfileSnapshot[cadenceAnalysisSnapshotKey]; ok {
		profile.WorkerConfig[cadenceAnalysisSnapshotKey] = frozen
	}
	if frozen, ok := job.ProfileSnapshot[cadenceRecommendationSnapshotKey]; ok {
		profile.WorkerConfig[cadenceRecommendationSnapshotKey] = frozen
	}
	analysis, analysisOK := decodeCadenceAnalysis(profile.WorkerConfig[cadenceAnalysisSnapshotKey])
	recommendation, recommendationOK := decodeCadenceRecommendation(profile.WorkerConfig[cadenceRecommendationSnapshotKey])
	interlace, _ := decodeInterlaceAnalysis(job.ProfileSnapshot[interlaceAnalysisSnapshotKey])
	profile = profileWithResolvedFieldAndCadenceModes(profile, interlace)
	if analysisOK {
		if !recommendationOK {
			recommendation = recommendCadence(analysis)
		}
		profile = profileWithCadenceOutputDecision(profile, analysis, recommendation)
	}
	return profile
}

func refreshSnapshotBeforeExecution(
	db *gorm.DB,
	mediaPath string,
) (models.ScanResult, error) {
	if db == nil {
		return models.ScanResult{}, fmt.Errorf(
			"fresh execution snapshot requires a database",
		)
	}

	cleanPath := filepath.Clean(mediaPath)

	info, err := os.Stat(cleanPath)
	if err != nil {
		return models.ScanResult{}, fmt.Errorf(
			"fresh execution snapshot cannot read input media: %w",
			err,
		)
	}

	if info.IsDir() {
		return models.ScanResult{}, fmt.Errorf(
			"fresh execution snapshot requires a media file",
		)
	}

	result, _, err := NewScannerHandler(db).scanResolvedFile(
		cleanPath,
		info,
		ScanRequest{
			Force: true,
		},
		nil,
	)
	if err != nil {
		return models.ScanResult{}, fmt.Errorf(
			"fresh execution snapshot failed: %w",
			err,
		)
	}

	return result, nil
}

func automaticFrameStructureSnapshot(db *gorm.DB, mediaPath string) (models.ScanResult, error) {
	if db == nil {
		return models.ScanResult{}, fmt.Errorf("automatic frame structure requires a database")
	}

	cleanPath := filepath.Clean(mediaPath)

	var scan models.ScanResult
	err := db.
		Where("path = ?", cleanPath).
		Order("updated_at desc, id desc").
		First(&scan).
		Error

	if err == nil && !snapshotRequiresFrameStructureRefresh(scan) {
		if ensureFrameStructureRecommendation(&scan) {
			if updateErr := db.Model(&scan).
				Update(
					"frame_structure_recommendation",
					scan.FrameStructureRecommendation,
				).Error; updateErr != nil {
				return models.ScanResult{}, updateErr
			}
		}

		return scan, nil
	}

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ScanResult{}, err
	}

	info, statErr := os.Stat(cleanPath)
	if statErr != nil {
		return models.ScanResult{}, fmt.Errorf(
			"automatic frame structure cannot read input media: %w",
			statErr,
		)
	}

	if info.IsDir() {
		return models.ScanResult{}, fmt.Errorf(
			"automatic frame structure requires a media file",
		)
	}

	result, _, scanErr := NewScannerHandler(db).scanResolvedFile(
		cleanPath,
		info,
		ScanRequest{
			Force: false,
		},
		nil,
	)
	if scanErr != nil {
		return models.ScanResult{}, fmt.Errorf(
			"automatic frame structure snapshot failed: %w",
			scanErr,
		)
	}

	if snapshotRequiresFrameStructureRefresh(result) {
		return models.ScanResult{}, fmt.Errorf(
			"automatic frame structure could not produce a usable asset snapshot",
		)
	}

	if ensureFrameStructureRecommendation(&result) {
		if updateErr := db.Model(&result).
			Update(
				"frame_structure_recommendation",
				result.FrameStructureRecommendation,
			).Error; updateErr != nil {
			return models.ScanResult{}, updateErr
		}
	}

	return result, nil
}

// resolveAutomaticFrameStructure turns the reusable Auto intent into effective,
// asset-specific values at execution time. The captured profile remains the
// requested configuration; only the worker's effective copy is changed.
func resolveAutomaticFrameStructure(
	db *gorm.DB,
	mediaPath string,
	profile models.Profile,
) (models.Profile, error) {
	if normalizedFrameStructureMode(
		workerStringValue(profile.WorkerConfig["frameStructureMode"]),
	) != "auto" {
		return profile, nil
	}

	workerConfig := models.JSONMap{}
	for key, value := range profile.WorkerConfig {
		workerConfig[key] = value
	}
	profile.WorkerConfig = workerConfig

	scan, err := automaticFrameStructureSnapshot(db, mediaPath)
	if err != nil {
		return models.Profile{}, fmt.Errorf(
			"resolve automatic frame structure: %w",
			err,
		)
	}

	fps := parseFrameRateValue(workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"]))
	if fps <= 0 {
		fps = scanFrameRate(scan)
	}
	recommendationSet := buildFrameStructureRecommendationSetForFPS(scan, fps)

	recommendation, ok := recommendationSet.ByMode["balanced"]
	if !ok {
		return models.Profile{}, fmt.Errorf(
			"automatic frame structure did not produce a balanced recommendation",
		)
	}

	if recommendation.TargetGOPFrames <= 0 {
		return models.Profile{}, fmt.Errorf(
			"automatic frame structure did not produce a valid GOP recommendation",
		)
	}

	if recommendation.MaxBFrames <= 0 {
		return models.Profile{}, fmt.Errorf(
			"automatic frame structure did not produce a valid B-frame recommendation",
		)
	}

	workerConfig["frameStructureGopMode"] = "recommended"
	workerConfig["frameStructureGopFrames"] = recommendation.TargetGOPFrames
	workerConfig["frameStructureBFrameMode"] = "recommended"
	workerConfig["frameStructureMaxBFrames"] = recommendation.MaxBFrames
	workerConfig["qsvAdaptiveI"] = true
	workerConfig["qsvAdaptiveB"] = true
	workerConfig["frameStructureAutoResolved"] = true
	workerConfig["frameStructureAutoConfidence"] = recommendation.Confidence
	if previous, exists := workerConfig["frameStructureRecommendation"]; exists {
		workerConfig["sourceFrameStructureRecommendation"] = previous
	}
	workerConfig["frameStructureRecommendation"] = models.JSONMap{
		"fps": fps, "targetGopFrames": recommendation.TargetGOPFrames,
		"targetGopSeconds": recommendation.TargetGOPSeconds, "maxBFrames": recommendation.MaxBFrames,
		"confidence": recommendation.Confidence, "reasons": recommendation.Reasons, "warnings": recommendation.Warnings,
	}

	return profile, nil
}

func scanFrameRate(scan models.ScanResult) float64 {
	for _, raw := range scan.VideoStreams {
		stream, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		for _, key := range []string{"avgFrameRate", "realFrameRate"} {
			if fps := parseFrameRateValue(stringFromUnknown(stream[key])); fps > 0 {
				return fps
			}
		}
	}

	return 0
}

func parseFrameRateValue(value string) float64 {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) == 2 {
		numerator, numeratorErr := strconv.ParseFloat(parts[0], 64)
		denominator, denominatorErr := strconv.ParseFloat(parts[1], 64)
		if numeratorErr == nil && denominatorErr == nil && denominator > 0 && numerator/denominator <= 240 {
			return numerator / denominator
		}
	}
	fps, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err == nil && fps > 0 && fps <= 240 {
		return fps
	}
	return 0
}

func (h WorkerHandler) profileForJob(job models.QueueJob) (models.Profile, error) {
	if len(job.ProfileSnapshot) > 0 {
		return scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	}
	var profile models.Profile
	if err := h.db.First(&profile, job.ProfileID).Error; err != nil {
		return models.Profile{}, err
	}
	return profile, nil
}

const assetConversionOverrideSnapshotKey = "assetConversionOverride"

func currentConversionOverrideForJob(job models.QueueJob, entries map[string]AssetConversionOverrideState) AssetConversionOverrideState {
	override := conversionOverrideForPath(job.MediaPath, entries)
	if len(job.TrackProfileSnapshot) > 0 {
		var profileOverride AssetConversionOverrideState
		if encoded, err := json.Marshal(job.TrackProfileSnapshot); err == nil && json.Unmarshal(encoded, &profileOverride) == nil {
			override = mergeTrackProfileBelowAssetOverride(profileOverride, override)
		}
	}
	return applyJobSelectionsToConversionOverride(job, override)
}

func mergeTrackProfileBelowAssetOverride(profile, asset AssetConversionOverrideState) AssetConversionOverrideState {
	if asset.KeepVideoStreams == nil {
		asset.KeepVideoStreams = profile.KeepVideoStreams
	}
	if asset.KeepAudioStreams == nil {
		asset.KeepAudioStreams = profile.KeepAudioStreams
	}
	if asset.KeepSubtitleStreams == nil {
		asset.KeepSubtitleStreams = profile.KeepSubtitleStreams
	}
	if len(asset.VideoMetadata) == 0 {
		asset.VideoMetadata = profile.VideoMetadata
	}
	if len(asset.AudioMetadata) == 0 {
		asset.AudioMetadata = profile.AudioMetadata
	}
	if len(asset.SubtitleMetadata) == 0 {
		asset.SubtitleMetadata = profile.SubtitleMetadata
	}
	if len(asset.SubtitleTransforms) == 0 {
		asset.SubtitleTransforms = profile.SubtitleTransforms
	}
	return asset
}

func conversionOverrideForJob(job models.QueueJob, entries map[string]AssetConversionOverrideState) AssetConversionOverrideState {
	if frozen, ok := job.ProfileSnapshot[assetConversionOverrideSnapshotKey]; ok {
		if encoded, err := json.Marshal(frozen); err == nil {
			var override AssetConversionOverrideState
			if json.Unmarshal(encoded, &override) == nil {
				return applyJobSelectionsToConversionOverride(job, override)
			}
		}
	}
	return currentConversionOverrideForJob(job, entries)
}

func applyJobSelectionsToConversionOverride(job models.QueueJob, override AssetConversionOverrideState) AssetConversionOverrideState {
	if value := strings.TrimSpace(job.TrackProfileKey); value != "" {
		override.TrackProfileKey = value
	}
	// Processing mode is derived by the queue selection, never from a stale
	// per-asset override saved by the former Work mode control.
	override.ProcessingMode = normalizeQueueProcessingMode(job.ProcessingMode)
	return override
}

func validJobStatus(status string) bool {
	switch status {
	case JobStatusQueued, JobStatusRunning, JobStatusCompleted, JobStatusFailed, JobStatusCanceled:
		return true
	default:
		return false
	}
}

func terminalJobStatus(status string) bool {
	return status == JobStatusCompleted || status == JobStatusFailed || status == JobStatusCanceled
}

func normalizedProgress(status string, progress int) int {
	if status == JobStatusCompleted {
		return 100
	}

	if status == JobStatusQueued {
		return 0
	}

	if progress < 0 {
		return 0
	}

	if progress > 100 {
		return 100
	}

	return progress
}

func plannedOutputPath(mediaPath string, library models.Library, profile models.Profile) string {
	return path.Join(library.DestinationPath, outputFileRelativePath(libraryOutputBaseRelativePath(libraryRelativeMediaPath(mediaPath, library), library), profile))
}

func plannedOutputPathForJob(db *gorm.DB, job models.QueueJob, library models.Library, profile models.Profile) string {
	relative := libraryOutputRelativePathForJob(db, job, library, libraryOutputBaseRelativePath(libraryRelativeMediaPath(job.MediaPath, library), library))
	return path.Join(library.DestinationPath, outputFileRelativePath(relative, profile))
}

func libraryRelativeMediaPath(mediaPath string, library models.Library) string {
	relative := strings.TrimPrefix(mediaPath, strings.TrimRight(library.SourcePath, "/")+"/")
	if relative == mediaPath {
		return path.Base(mediaPath)
	}
	return relative
}

func libraryOutputBaseRelativePath(relative string, library models.Library) string {
	parts := pathSegments(relative)
	if len(parts) <= 1 {
		return relative
	}
	destinationCategory := normalizedLibrarySegment(path.Base(path.Clean(library.DestinationPath)))
	libraryCategory := normalizedLibrarySegment(library.Name)
	for len(parts) > 1 {
		first := normalizedLibrarySegment(parts[0])
		if !isSourceBucketSegment(parts[0]) && first != destinationCategory && first != libraryCategory {
			break
		}
		parts = parts[1:]
	}
	return path.Join(parts...)
}

func normalizedLibrarySegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("_", "-", " ", "-").Replace(value)
	return strings.Trim(value, "-")
}

func isSourceBucketSegment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "movie", "movies", "series", "anime", "anime-movie", "anime-movies", "documentary", "documentaries", "concert", "concerts", "music-video", "music-videos":
		return true
	default:
		return false
	}
}

func outputFileRelativePath(relative string, profile models.Profile) string {
	extension := "." + strings.TrimPrefix(profile.Container, ".")
	return strings.TrimSuffix(relative, path.Ext(relative)) + extension
}

func libraryOutputRelativePathForJob(db *gorm.DB, job models.QueueJob, library models.Library, fallbackRelative string) string {
	if previousRelative := retiredPublishedRelativePath(db, job, library); previousRelative != "" {
		previousRelative = libraryOutputBaseRelativePath(previousRelative, library)

		previousEpisodeID := episodeIdentifierFromName(
			path.Base(previousRelative),
		)

		sourceEpisodeID := episodeIdentifierFromName(
			path.Base(job.MediaPath),
		)

		// When episode naming is enabled, calculate the episode identity using the
		// same current rules that will be used for the final published filename.
		// This prevents stale publication history from overriding a Season-folder
		// ordinal such as Season2/21.mkv -> S02E01.
		if libraryEpisodeNamingEnabled(library) {
			if spec, ok := multiEpisodeNameSpecForJob(db, job); ok {
				sourceEpisodeID = fmt.Sprintf(
					"S%02dE%02d",
					spec.Season,
					spec.Episode,
				)
			}
		}

		if sourceEpisodeID == "" {
			if episode, ok := leadingEpisodeNumberFromName(
				path.Base(job.MediaPath),
			); ok {
				season := firstPositiveInt(
					seasonNumberFromPath(job.BatchName),
					seasonNumberFromPath(job.MediaPath),
					1,
				)

				sourceEpisodeID = fmt.Sprintf(
					"S%02dE%02d",
					season,
					episode,
				)
			}
		}

		historicalEpisodeMatches := sourceEpisodeID == "" || previousEpisodeID == "" || sourceEpisodeID == previousEpisodeID
		if historicalEpisodeMatches && (!libraryEpisodeNamingEnabled(library) || previousEpisodeID != "" || nonEpisodeCreditName(path.Base(previousRelative)) != "") {
			return previousRelative
		}
	}
	relative := fallbackRelative
	if libraryExtrasPathEnabled(library) && assetIsExtra(job.MediaPath, assetMetadataOverrides(db)) {
		return extrasOutputRelativePath(relative, libraryExtrasPathName(library))
	}
	if libraryEpisodeNamingEnabled(library) {
		relative = multiEpisodeOutputRelativePath(db, job, relative)
	}
	return relative
}

func retiredPublishedRelativePath(db *gorm.DB, job models.QueueJob, library models.Library) string {
	if db == nil || strings.TrimSpace(job.MediaPath) == "" || strings.TrimSpace(library.DestinationPath) == "" {
		return ""
	}
	var previousJobs []models.QueueJob
	query := db.Where(
		"media_path = ? AND library_id = ? AND published_path <> ? AND publication_retired_at IS NOT NULL",
		job.MediaPath,
		job.LibraryID,
		"",
	)
	if job.ID > 0 {
		query = query.Where("id <> ?", job.ID)
	}
	if err := query.Order("published_at desc, id desc").Find(&previousJobs).Error; err != nil || len(previousJobs) == 0 {
		return ""
	}

	destination := filepath.Clean(strings.TrimSpace(library.DestinationPath))
	sourceStem := strings.TrimSuffix(filepath.Base(job.MediaPath), filepath.Ext(job.MediaPath))
	fallback := ""
	for _, previous := range previousJobs {
		published := filepath.Clean(strings.TrimSpace(previous.PublishedPath))
		relative, err := filepath.Rel(destination, published)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		relative = filepath.ToSlash(relative)
		if fallback == "" {
			fallback = relative
		}
		publishedStem := strings.TrimSuffix(filepath.Base(published), filepath.Ext(published))
		if publishedStem != sourceStem {
			return relative
		}
	}
	return fallback
}

func dryRunCommand(inputPath string, outputPath string, profile models.Profile) string {
	videoArgs := codecArgs("-c:v", profile.VideoCodec, profile.QualityMode, profile.QualityValue)
	audioArgs := codecArgs("-c:a", profile.AudioCodec, "", 0)
	subtitleArgs := "-c:s copy"
	if !profile.PreserveSubtitles {
		subtitleArgs = "-sn"
	}

	mapChapters := "-map_chapters 0"
	if !profile.PreserveChapters {
		mapChapters = "-map_chapters -1"
	}

	return fmt.Sprintf(
		"ffmpeg -i %q -map 0 %s %s %s %s %q",
		inputPath,
		videoArgs,
		audioArgs,
		subtitleArgs,
		mapChapters,
		outputPath,
	)
}

func conversionArgs(inputPath string, outputPath string, profile models.Profile, audioProfile *audioEnhancementProfile, overwrite bool) []string {
	args := []string{"-hide_banner"}
	if overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	args = append(args, "-i", inputPath, "-map", "0")
	args = append(args, splitArgs(codecArgs("-c:v", profile.VideoCodec, profile.QualityMode, profile.QualityValue))...)
	if audioProfile != nil {
		filterChain := effectiveAudioFilters(*audioProfile)
		codec := audioProfile.OutputCodec
		if codec == "" || codec == "copy" {
			codec = "aac"
		}
		args = append(args, "-c:a", "copy", "-filter:a:0", filterChain, "-c:a:0", ffmpegCodecName(codec))
	} else {
		args = append(args, splitArgs(codecArgs("-c:a", profile.AudioCodec, "", 0))...)
	}
	if profile.PreserveSubtitles {
		args = append(args, "-c:s", "copy")
	} else {
		args = append(args, "-sn")
	}
	if profile.PreserveChapters {
		args = append(args, "-map_chapters", "0")
	} else {
		args = append(args, "-map_chapters", "-1")
	}
	args = append(args, outputPath)
	return args
}

func (h WorkerHandler) audioProfile(key string) (*audioEnhancementProfile, error) {
	return lookupAudioProfile(h.db, key)
}

func (h WorkerHandler) audioProfileForJob(job models.QueueJob) (*audioEnhancementProfile, error) {
	if len(job.AudioProfileSnapshot) > 0 {
		return audioProfileFromMap(job.AudioProfileKey, job.AudioProfileSnapshot), nil
	}
	return h.audioProfile(job.AudioProfileKey)
}

func lookupAudioProfile(db *gorm.DB, key string) (*audioEnhancementProfile, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}

	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "audioEnhancementProfiles").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	profiles, ok := workerSlice(setting.Value["profiles"])
	if !ok {
		return nil, nil
	}

	for _, value := range profiles {
		candidate, ok := workerMap(value)
		if !ok {
			continue
		}
		if workerStringValue(candidate["key"]) != key {
			continue
		}
		return audioProfileFromMap(key, candidate), nil
	}

	return nil, nil
}

func audioProfileFromMap(key string, candidate map[string]any) *audioEnhancementProfile {
	return &audioEnhancementProfile{
		Key:              key,
		Filters:          workerStringValue(candidate["filters"]),
		OutputCodec:      workerStringValue(candidate["outputCodec"]),
		RNNoiseModelPath: workerStringValue(candidate["rnnoiseModelPath"]),
		ChannelMode:      workerStringValue(candidate["channelMode"]),
		ForceStereoMode:  workerStringValue(candidate["forceStereoMode"]),
		StereoDelayMs:    workerNumberValue(candidate["stereoDelayMs"], 12),
		StereoWidth:      workerNumberValue(candidate["stereoWidth"], 20),
		EQBands:          workerNumberMap(candidate["eqBands"]),
		TargetLoudness:   workerNumberValue(candidate["targetLoudness"], -18),
		TruePeak:         workerNumberValue(candidate["truePeak"], -2),
	}
}

func effectiveAudioFilters(profile audioEnhancementProfile) string {
	parts := []string{}
	if rnnoise := rnnoiseFilter(profile.RNNoiseModelPath); rnnoise != "" {
		parts = append(parts, rnnoise)
	}
	if channel := channelFilter(profile); channel != "" {
		parts = append(parts, channel)
	}
	if baseFilters := normalizedBaseAudioFilters(profile); baseFilters != "" {
		parts = append(parts, baseFilters)
	}
	if eq := eqFilterChain(profile.EQBands); eq != "" {
		parts = append(parts, eq)
	}
	if len(parts) == 0 {
		return "anull"
	}
	return sanitizeAudioFilterChain(strings.Join(parts, ","))
}

func normalizedBaseAudioFilters(profile audioEnhancementProfile) string {
	rawFilters := strings.Split(profile.Filters, ",")
	filters := []string{}
	loudnorm := loudnormFilter(profile)

	for _, rawFilter := range rawFilters {
		filter := strings.TrimSpace(rawFilter)
		if filter == "" {
			continue
		}
		if strings.HasPrefix(filter, "loudnorm=") {
			filters = append(filters, loudnorm)
			continue
		}
		filters = append(filters, filter)
	}

	return strings.Join(filters, ",")
}

func loudnormFilter(profile audioEnhancementProfile) string {
	target := profile.TargetLoudness
	peak := profile.TruePeak
	lra := "11"
	for _, rawFilter := range strings.Split(profile.Filters, ",") {
		filter := strings.TrimSpace(rawFilter)
		if !strings.HasPrefix(filter, "loudnorm=") {
			continue
		}
		for _, part := range strings.Split(filter, ":") {
			if strings.HasPrefix(part, "LRA=") {
				lra = strings.TrimPrefix(part, "LRA=")
			}
		}
	}
	return fmt.Sprintf("loudnorm=I=%s:TP=%s:LRA=%s", trimGain(target), trimGain(peak), lra)
}

func rnnoiseFilter(modelPath string) string {
	modelPath = strings.TrimSpace(modelPath)
	if modelPath == "" {
		return ""
	}
	if info, err := os.Stat(modelPath); err != nil || info.IsDir() {
		return ""
	}
	return "arnndn=m=" + modelPath
}

func channelFilter(profile audioEnhancementProfile) string {
	switch profile.ChannelMode {
	case "dual-mono":
		return "pan=stereo|c0=c0|c1=c0"
	case "force-stereo":
		return forceStereoFilter(profile.ForceStereoMode)
	case "downmix-mono":
		return "aresample=ochl=mono"
	case "light-stereo":
		delay := float64(int(clampFloat(profile.StereoDelayMs, 1, 40) + 0.5))
		width := clampFloat(profile.StereoWidth, 0, 100)
		feedback := trimGain((width / 100) * 0.12)
		crossfeed := trimGain(maxFloat(0.05, 0.22-width/600))
		delayText := trimGain(delay)
		return fmt.Sprintf("pan=stereo|c0=c0|c1=c0,adelay=0|%s,stereowiden=delay=%s:feedback=%s:crossfeed=%s:drymix=0.9", delayText, delayText, feedback, crossfeed)
	default:
		return ""
	}
}

func forceStereoFilter(mode string) string {
	switch mode {
	case "first-two":
		return "pan=stereo|c0=c0|c1=c1"
	case "duplicate-first":
		return "pan=stereo|c0=c0|c1=c0"
	default:
		return "aresample=ochl=stereo"
	}
}

func eqFilterChain(bands map[string]float64) string {
	frequencies := []string{"60", "120", "250", "500", "1000", "2000", "4000", "8000", "12000"}
	filters := []string{}
	for _, frequency := range frequencies {
		gain := bands[frequency]
		if gain == 0 {
			continue
		}
		filters = append(filters, fmt.Sprintf("equalizer=f=%s:t=q:w=1:g=%s", frequency, trimGain(gain)))
	}
	return strings.Join(filters, ",")
}

func trimGain(value float64) string {
	if value == float64(int(value)) {
		return strconv.Itoa(int(value))
	}
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func clampFloat(value float64, minimum float64, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func maxFloat(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func codecArgs(flag string, codec string, qualityMode string, qualityValue int) string {
	if codec == "" || codec == "copy" {
		return flag + " copy"
	}

	originalCodec := codec
	codec = ffmpegCodecName(codec)
	args := fmt.Sprintf("%s %s", flag, codec)
	if flag == "-c:v" && isTenBitVideoCodec(originalCodec) {
		args += " -pix_fmt yuv420p10le"
	}
	if qualityMode == "crf" && qualityValue > 0 {
		return fmt.Sprintf("%s -crf %d", args, qualityValue)
	}

	return args
}

func ffmpegCodecName(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "x264", "h264":
		return "libx264"
	case "x265", "h265", "hevc", "x265_10bit", "h265_10bit", "hevc_10bit":
		return "libx265"
	default:
		return codec
	}
}

func isTenBitVideoCodec(codec string) bool {
	normalized := strings.ToLower(strings.TrimSpace(codec))
	return normalized == "x265_10bit" || normalized == "h265_10bit" || normalized == "hevc_10bit"
}

type pathSettings struct {
	rawRoot     string
	stagingPath string
}

func (h WorkerHandler) pathSettings() (pathSettings, error) {
	roles, roleErr := scheduler.LoadStorageRoles(h.db)
	if roleErr == nil {
		raw, rawErr := roles.Path(scheduler.StorageRoleRaw)
		work, workErr := roles.Path(scheduler.StorageRoleWork)
		if rawErr == nil && workErr == nil {
			return pathSettings{rawRoot: raw, stagingPath: work}, nil
		}
	}
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := h.db.First(&setting, "key = ?", "paths").Error; err == nil && setting.Value != nil {
		values = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return pathSettings{}, err
	}

	return pathSettings{
		rawRoot:     workerStringSetting(values, "rawRoot", "/media/raw"),
		stagingPath: workerStringSetting(values, "stagingPath", "/media/staging"),
	}, nil
}

func plannedStagingOutputPath(job models.QueueJob, profile models.Profile, paths pathSettings) string {
	base := outputFileRelativePath(filepath.Base(job.MediaPath), profile)
	return filepath.Join(paths.stagingPath, fmt.Sprintf("job-%d", job.ID), base)
}

func libraryEpisodeNamingEnabled(library models.Library) bool {
	return boolSetting(library.ValidationRules["episodeNamingEnabled"], false)
}

func libraryExtrasPathEnabled(library models.Library) bool {
	return boolSetting(library.ValidationRules["extrasPathEnabled"], false)
}

func libraryExtrasPathName(library models.Library) string {
	value := strings.Trim(strings.TrimSpace(stringFromUnknown(library.ValidationRules["extrasPathName"])), "/\\")
	if value == "" {
		return "extras"
	}
	return sanitizeMediaFileName(value)
}

func assetIsExtra(mediaPath string, metadata map[string]AssetMetadataState) bool {
	state := metadataForPath(mediaPath, metadata)
	for _, value := range append(state.Categories, state.Tags...) {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "extra", "extras", "special", "specials", "bonus":
			return true
		}
	}
	return false
}

func extrasOutputRelativePath(relative string, extrasPathName string) string {
	parts := pathSegments(relative)
	if len(parts) == 0 {
		return relative
	}
	for _, part := range parts[:len(parts)-1] {
		if strings.EqualFold(part, extrasPathName) || strings.EqualFold(part, "extra") || strings.EqualFold(part, "extras") {
			return relative
		}
	}
	if len(parts) == 1 {
		return path.Join(extrasPathName, parts[0])
	}
	return path.Join(path.Join(parts[:len(parts)-1]...), extrasPathName, parts[len(parts)-1])
}

type multiEpisodeNameSpec struct {
	SeriesTitle string
	Season      int
	Episode     int
}

func multiEpisodeOutputRelativePath(db *gorm.DB, job models.QueueJob, fallbackRelative string) string {
	spec, ok := multiEpisodeNameSpecForJob(db, job)
	if !ok {
		return fallbackRelative
	}
	return formatMultiEpisodeOutputRelativePath(job, fallbackRelative, spec)
}

func formatMultiEpisodeOutputRelativePath(job models.QueueJob, fallbackRelative string, spec multiEpisodeNameSpec) string {
	dir := path.Dir(fallbackRelative)
	title := sanitizeMediaFileName(spec.SeriesTitle)
	episodeID := fmt.Sprintf("S%02dE%02d", spec.Season, spec.Episode)
	if nested := nestedAssetPathLabel(job); nested != "" {
		title = nested
		if sourceEpisodeID := episodeIdentifierFromName(path.Base(job.MediaPath)); sourceEpisodeID != "" {
			episodeID = sourceEpisodeID
		} else if creditName := nonEpisodeCreditName(path.Base(job.MediaPath)); creditName != "" {
			return path.Join(dir, fmt.Sprintf("%s-%s%s", title, creditName, path.Ext(fallbackRelative)))
		}
	}
	fileName := fmt.Sprintf("%s-%s%s", title, episodeID, path.Ext(fallbackRelative))
	if nestedAssetPathLabel(job) == "" {
		fileName = fmt.Sprintf("%s - %s%s", title, episodeID, path.Ext(fallbackRelative))
	}
	if dir == "." || dir == "/" {
		return fileName
	}
	return path.Join(dir, fileName)
}

func nonEpisodeCreditName(fileName string) string {
	stem := strings.TrimSuffix(fileName, path.Ext(fileName))
	upper := strings.ToUpper(strings.TrimSpace(stem))
	if !strings.HasPrefix(upper, "NCOP") && !strings.HasPrefix(upper, "NCED") {
		return ""
	}
	if bracket := strings.LastIndex(stem, "["); bracket > 0 && strings.HasSuffix(strings.TrimSpace(stem), "]") {
		stem = strings.TrimSpace(stem[:bracket])
	}
	return sanitizeMediaFileName(stem)
}

func nestedAssetPathLabel(job models.QueueJob) string {
	mediaParts := pathSegments(path.Dir(job.MediaPath))
	batchParts := pathSegments(job.BatchName)
	if len(mediaParts) == 0 || len(batchParts) == 0 {
		return ""
	}
	for start := len(mediaParts) - len(batchParts); start >= 0; start-- {
		matches := true
		for index := range batchParts {
			if !strings.EqualFold(mediaParts[start+index], batchParts[index]) {
				matches = false
				break
			}
		}
		if matches && start+len(batchParts) < len(mediaParts) {
			return sanitizeMediaFileName(strings.Join(mediaParts[start+len(batchParts):], "-"))
		}
	}
	return ""
}

func episodeIdentifierFromName(fileName string) string {
	upper := strings.ToUpper(fileName)
	for start := 0; start < len(upper); start++ {
		if upper[start] != 'S' {
			continue
		}
		seasonEnd := start + 1
		for seasonEnd < len(upper) && upper[seasonEnd] >= '0' && upper[seasonEnd] <= '9' {
			seasonEnd++
		}
		if seasonEnd == start+1 || seasonEnd >= len(upper) || upper[seasonEnd] != 'E' {
			continue
		}
		episodeEnd := seasonEnd + 1
		for episodeEnd < len(upper) && upper[episodeEnd] >= '0' && upper[episodeEnd] <= '9' {
			episodeEnd++
		}
		if episodeEnd > seasonEnd+1 {
			season, _ := strconv.Atoi(upper[start+1 : seasonEnd])
			episode, _ := strconv.Atoi(upper[seasonEnd+1 : episodeEnd])
			return fmt.Sprintf("S%02dE%02d", season, episode)
		}
	}
	return ""
}

func seasonFolderEpisodePosition(
	db *gorm.DB,
	job models.QueueJob,
) (int, int, bool) {
	season := firstPositiveInt(
		seasonNumberFromPath(path.Dir(
			strings.ReplaceAll(job.MediaPath, "\\", "/"),
		)),
		seasonNumberFromPath(job.BatchName),
	)

	if season <= 0 {
		return 0, 0, false
	}

	episode := episodeNumberFromAssetGroup(
		db,
		job.MediaPath,
	)

	if episode <= 0 {
		return 0, 0, false
	}

	return season, episode, true
}

func multiEpisodeNameSpecForJob(
	db *gorm.DB,
	job models.QueueJob,
) (multiEpisodeNameSpec, bool) {
	// Explicit SxxExx in the filename is authoritative.
	if season, episode, ok :=
		seasonEpisodeFromName(path.Base(job.MediaPath)); ok {

		title := episodeSeriesTitle(
			job.BatchName,
			job.MediaPath,
		)

		if title == "" {
			return multiEpisodeNameSpec{}, false
		}

		return multiEpisodeNameSpec{
			SeriesTitle: title,
			Season:      season,
			Episode:     episode,
		}, true
	}

	// Inside an explicit Season folder, filenames establish natural
	// ordering but the episode number is the ordinal inventory position.
	if season, episode, ok :=
		seasonFolderEpisodePosition(db, job); ok {

		title := episodeSeriesTitle(
			job.BatchName,
			job.MediaPath,
		)

		if title == "" {
			return multiEpisodeNameSpec{}, false
		}

		return multiEpisodeNameSpec{
			SeriesTitle: title,
			Season:      season,
			Episode:     episode,
		}, true
	}

	// Existing metadata behavior remains useful outside Season folders.
	if season, episode, ok :=
		episodeIdentityFromScan(db, job.MediaPath); ok {

		title := episodeSeriesTitle(
			job.BatchName,
			job.MediaPath,
		)

		if title == "" {
			return multiEpisodeNameSpec{}, false
		}

		if season <= 0 {
			season = firstPositiveInt(
				seasonNumberFromPath(job.BatchName),
				seasonNumberFromPath(job.MediaPath),
				1,
			)
		}

		return multiEpisodeNameSpec{
			SeriesTitle: title,
			Season:      season,
			Episode:     episode,
		}, true
	}

	if episode, ok := leadingEpisodeNumberFromName(path.Base(job.MediaPath)); ok {
		title := episodeSeriesTitle(job.BatchName, job.MediaPath)
		if title == "" {
			return multiEpisodeNameSpec{}, false
		}
		season := firstPositiveInt(seasonNumberFromPath(job.BatchName), seasonNumberFromPath(job.MediaPath), 1)
		return multiEpisodeNameSpec{SeriesTitle: title, Season: season, Episode: episode}, true
	}

	// The complete asset inventory is the stable source of episode position.
	// A partial batch must not renumber episode 3 as episode 1 merely because
	// episodes 1 and 2 were not selected for this run.
	if episode := episodeNumberFromAssetGroup(db, job.MediaPath); episode > 0 {
		title := episodeSeriesTitle(job.BatchName, job.MediaPath)
		if title == "" {
			return multiEpisodeNameSpec{}, false
		}
		season := firstPositiveInt(seasonNumberFromPath(job.BatchName), seasonNumberFromPath(job.MediaPath), 1)
		return multiEpisodeNameSpec{SeriesTitle: title, Season: season, Episode: episode}, true
	}

	batchID := strings.TrimSpace(job.BatchID)
	var batchJobs []models.QueueJob
	if batchID != "" {
		_ = db.Where("batch_id = ?", batchID).Order("CASE WHEN batch_position > 0 THEN batch_position ELSE 2147483647 END asc, created_at asc, id asc").Find(&batchJobs).Error
	}

	episode := 0
	if len(batchJobs) > 1 {
		for index, candidate := range batchJobs {
			if candidate.ID == job.ID || candidate.MediaPath == job.MediaPath {
				episode = index + 1
				break
			}
		}
	}
	if episode == 0 {
		return multiEpisodeNameSpec{}, false
	}

	title := episodeSeriesTitle(job.BatchName, job.MediaPath)
	if title == "" {
		return multiEpisodeNameSpec{}, false
	}

	season := firstPositiveInt(
		seasonNumberFromPath(job.BatchName),
		seasonNumberFromPath(job.MediaPath),
		1,
	)

	return multiEpisodeNameSpec{SeriesTitle: title, Season: season, Episode: episode}, true
}

func episodeIdentityFromScan(db *gorm.DB, mediaPath string) (int, int, bool) {
	if db == nil || !db.Migrator().HasTable(&models.ScanResult{}) {
		return 0, 0, false
	}
	var scan models.ScanResult
	if err := db.Where("path = ?", filepath.Clean(mediaPath)).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return 0, 0, false
	}
	format := unknownRecord(scan.RawProbe["format"])
	if format == nil {
		return 0, 0, false
	}
	tags := unknownRecord(format["tags"])
	if tags == nil {
		return 0, 0, false
	}
	episode := firstMetadataNumber(tags, "episode_id", "episode", "episode_sort", "track")
	if episode <= 0 {
		return 0, 0, false
	}
	season := firstMetadataNumber(tags, "season_number", "season", "season_sort")
	return season, episode, true
}

func firstMetadataNumber(tags map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		for storedKey, raw := range tags {
			if !strings.EqualFold(storedKey, key) {
				continue
			}
			value := strings.TrimSpace(fmt.Sprint(raw))
			end := 0
			for end < len(value) && value[end] >= '0' && value[end] <= '9' {
				end++
			}
			if end > 0 {
				parsed, _ := strconv.Atoi(value[:end])
				if parsed > 0 {
					return parsed
				}
			}
		}
	}
	return 0
}

func seasonEpisodeFromName(fileName string) (int, int, bool) {
	identifier := episodeIdentifierFromName(fileName)
	if identifier == "" {
		return 0, 0, false
	}
	separator := strings.Index(identifier, "E")
	season, seasonErr := strconv.Atoi(strings.TrimPrefix(identifier[:separator], "S"))
	episode, episodeErr := strconv.Atoi(identifier[separator+1:])
	return season, episode, seasonErr == nil && episodeErr == nil && episode > 0
}

func leadingEpisodeNumberFromName(fileName string) (int, bool) {
	stem := strings.TrimSpace(strings.TrimSuffix(path.Base(fileName), path.Ext(fileName)))
	end := 0
	for end < len(stem) && stem[end] >= '0' && stem[end] <= '9' {
		end++
	}
	if end == 0 || end >= len(stem) {
		return 0, false
	}
	switch stem[end] {
	case '-', '_', '.', ' ':
	default:
		return 0, false
	}
	episode, err := strconv.Atoi(stem[:end])
	return episode, err == nil && episode > 0 && episode <= 999
}

func episodeNumberFromAssetGroup(db *gorm.DB, mediaPath string) int {
	if db == nil || strings.TrimSpace(mediaPath) == "" {
		return 0
	}
	cleanPath := filepath.Clean(mediaPath)
	var asset models.AssetRecord
	if err := db.Where(
		"path = ?",
		cleanPath,
	).First(&asset).Error; err != nil ||
		strings.TrimSpace(asset.GroupPath) == "" {
		return 0
	}
	var siblings []models.AssetRecord
	if err := db.Where(
		"root_path = ? AND group_path = ?",
		asset.RootPath,
		asset.GroupPath,
	).Find(&siblings).Error; err != nil || len(siblings) <= 1 {
		return 0
	}
	sort.SliceStable(siblings, func(i, j int) bool {
		return naturalLess(siblings[i].RelativePath, siblings[j].RelativePath)
	})
	for index := range siblings {
		if filepath.Clean(siblings[index].Path) == cleanPath {
			return index + 1
		}
	}
	return 0
}

func naturalLess(left, right string) bool {
	leftRunes, rightRunes := []rune(strings.ToLower(left)), []rune(strings.ToLower(right))
	for li, ri := 0, 0; li < len(leftRunes) && ri < len(rightRunes); {
		if unicode.IsDigit(leftRunes[li]) && unicode.IsDigit(rightRunes[ri]) {
			ln, rn := li, ri
			for ln < len(leftRunes) && unicode.IsDigit(leftRunes[ln]) {
				ln++
			}
			for rn < len(rightRunes) && unicode.IsDigit(rightRunes[rn]) {
				rn++
			}
			lv, _ := strconv.ParseUint(string(leftRunes[li:ln]), 10, 64)
			rv, _ := strconv.ParseUint(string(rightRunes[ri:rn]), 10, 64)
			if lv != rv {
				return lv < rv
			}
			if ln-li != rn-ri {
				return ln-li < rn-ri
			}
			li, ri = ln, rn
			continue
		}
		if leftRunes[li] != rightRunes[ri] {
			return leftRunes[li] < rightRunes[ri]
		}
		li++
		ri++
	}
	return len(leftRunes) < len(rightRunes)
}

func applyEpisodeVideoTrackTitle(db *gorm.DB, plan *MediaJobPlan, job models.QueueJob, library models.Library) {
	if plan == nil || !libraryEpisodeNamingEnabled(library) || len(plan.Streams.Video) == 0 {
		return
	}
	if _, ok := multiEpisodeNameSpecForJob(db, job); !ok {
		return
	}
	if plan.Override.VideoMetadata == nil {
		plan.Override.VideoMetadata = map[int]StreamMetadataOverride{}
	}
	streamIndex := plan.Streams.Video[0].Index
	metadata := plan.Override.VideoMetadata[streamIndex]
	if strings.TrimSpace(metadata.Title) != "" {
		return
	}
	metadata.Title = strings.TrimSuffix(filepath.Base(job.MediaPath), filepath.Ext(job.MediaPath))
	plan.Override.VideoMetadata[streamIndex] = metadata
}

func episodeSeriesTitle(batchName string, mediaPath string) string {
	candidate := strings.TrimSpace(batchName)
	if candidate == "" {
		candidate = path.Dir(strings.ReplaceAll(mediaPath, "\\", "/"))
	}

	parts := pathSegments(candidate)
	for index := len(parts) - 1; index >= 0; index-- {
		if seasonNumberFromPath(parts[index]) > 0 {
			continue
		}
		return strings.TrimSpace(parts[index])
	}
	return ""
}

func pathSegments(value string) []string {
	normalized := strings.ReplaceAll(value, "\\", "/")
	normalized = path.Clean("/" + normalized)
	rawParts := strings.Split(strings.TrimPrefix(normalized, "/"), "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		if trimmed := strings.TrimSpace(part); trimmed != "" && trimmed != "." {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func seasonNumberFromPath(value string) int {
	for _, part := range pathSegments(value) {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(normalized, "season") {
			if season, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(normalized, "season"))); err == nil && season > 0 {
				return season
			}
		}
		if strings.HasPrefix(normalized, "s") && len(normalized) <= 4 {
			if season, err := strconv.Atoi(strings.TrimPrefix(normalized, "s")); err == nil && season > 0 {
				return season
			}
		}
	}
	return 0
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func sanitizeMediaFileName(value string) string {
	value = strings.NewReplacer("/", "-", "\\", "-", "\x00", "").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func relativeMediaPath(mediaPath string, root string) string {
	normalizedRoot := strings.TrimRight(root, "/")
	if normalizedRoot != "" && strings.HasPrefix(mediaPath, normalizedRoot+"/") {
		relative := strings.TrimPrefix(mediaPath, normalizedRoot+"/")
		relative = path.Clean("/" + relative)
		return strings.TrimPrefix(relative, "/")
	}
	return path.Base(mediaPath)
}

func splitArgs(value string) []string {
	return strings.Fields(value)
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n\"'|") {
			quoted = append(quoted, strconv.Quote(arg))
			continue
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}

func appendNote(notes string, note string) string {
	if strings.TrimSpace(notes) == "" {
		return note
	}
	return strings.TrimSpace(notes) + "\n" + note
}

func lastOutputLines(value string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func intSetting(values models.JSONMap, key string, fallback int) int {
	value, ok := values[key]
	if !ok {
		return fallback
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	}

	return fallback
}

func workerStringSetting(values models.JSONMap, key string, fallback string) string {
	value, ok := values[key]
	if !ok {
		return fallback
	}

	if typed, ok := value.(string); ok && typed != "" {
		return typed
	}

	return fallback
}

func workerSlice(value interface{}) ([]interface{}, bool) {
	switch typed := value.(type) {
	case []interface{}:
		return typed, true
	case []models.JSONMap:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return values, true
	case []map[string]interface{}:
		values := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return values, true
	default:
		return nil, false
	}
}

func workerMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case models.JSONMap:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func workerStringValue(value interface{}) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func workerNumberValue(value interface{}, fallback float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func workerNumberMap(value interface{}) map[string]float64 {
	result := map[string]float64{}
	values, ok := workerMap(value)
	if !ok {
		return result
	}

	for key, raw := range values {
		switch typed := raw.(type) {
		case float64:
			result[key] = typed
		case int:
			result[key] = float64(typed)
		case int64:
			result[key] = float64(typed)
		case json.Number:
			parsed, err := typed.Float64()
			if err == nil {
				result[key] = parsed
			}
		}
	}

	return result
}
