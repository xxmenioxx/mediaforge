package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
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
	WorkerName string `json:"workerName"`
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

func (h WorkerHandler) heartbeatWorker(name string, limits workerLimits) error {
	if strings.TrimSpace(name) == "" {
		name = limits.DefaultWorkerName
	}
	if strings.TrimSpace(name) == "" {
		name = "local-worker"
	}
	encoders := models.JSONList{"libx265", "libx264"}
	runtimeProfile := ""
	if snapshot, err := runtimeinfo.Latest(h.db); err == nil {
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
		Order("queue_jobs.priority asc, queue_jobs.created_at asc").
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

	claimed, err := h.claimNextJob(input.WorkerName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			message := "no runnable queued jobs are available"
			var job models.QueueJob
			if h.db.Where("status = ?", JobStatusQueued).Order("priority asc, created_at asc").First(&job).Error == nil && job.ActiveExecutionPlanID != nil {
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

func (h WorkerHandler) claimNextJob(workerName string) (models.QueueJob, error) {
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
	if err := h.heartbeatWorker(workerName, limits); err != nil {
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

	if job.Status == JobStatusCanceled {
		cancelRunningJobProcess(job.ID)
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

	override := conversionOverrideForPath(job.MediaPath, assetConversionOverrides(h.db))
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	paths, err := h.pathSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	outputPath := plannedStagingOutputPath(h.db, job, library, effectiveProfile, paths)
	inputPath := job.MediaPath
	if job.ActiveExecutionPlanID != nil {
		var executionPlan models.ExecutionPlan
		if h.db.First(&executionPlan, *job.ActiveExecutionPlanID).Error == nil && executionPlan.WorkspaceMode == scheduler.WorkspaceModeCopyToWork {
			inputPath = filepath.Join(paths.stagingPath, fmt.Sprintf("job-%d", job.ID), "_input", filepath.Base(job.MediaPath))
		}
	}
	audioProfile, err := h.audioProfile(job.AudioProfileKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	plan, err := buildMediaJobPlanWithOverride(inputPath, outputPath, profile, audioProfile, true, override)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	args := FFmpegCommandBuilder{}.Build(plan)
	command := dryRunCommandFromArgs(args)

	job.Status = JobStatusRunning
	if job.Progress < 5 {
		job.Progress = 5
	}
	job.OutputPath = outputPath
	job.ErrorMessage = ""
	job.Notes = appendNote(job.Notes, "Dry-run command: "+command)
	if !assetConversionOverrideEmpty(override) {
		job.Notes = appendNote(job.Notes, "Asset conversion overrides applied")
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
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, job)
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
	if err := transitionJobStage(h.db, &job, JobStagePreparingWorkspace); err != nil {
		return job, http.StatusInternalServerError, err
	}
	inputPath := job.MediaPath
	workspaceMode := scheduler.WorkspaceModeDirect
	if job.ActiveExecutionPlanID != nil {
		var executionPlan models.ExecutionPlan
		if err := h.db.First(&executionPlan, *job.ActiveExecutionPlanID).Error; err == nil && executionPlan.WorkspaceMode != "" {
			workspaceMode = executionPlan.WorkspaceMode
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

	override := conversionOverrideForPath(job.MediaPath, assetConversionOverrides(h.db))
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	outputPath := plannedStagingOutputPath(h.db, job, library, effectiveProfile, paths)
	if !overwrite {
		if _, err := os.Stat(outputPath); err == nil {
			return job, http.StatusConflict, fmt.Errorf("staging output already exists; retry with overwrite enabled")
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return job, http.StatusInternalServerError, err
	}

	audioProfile, err := h.audioProfile(job.AudioProfileKey)
	if err != nil {
		return job, http.StatusInternalServerError, err
	}
	if err := transitionJobStage(h.db, &job, JobStageAnalyzingAsIs); err != nil {
		return job, http.StatusInternalServerError, err
	}

	plan, err := buildMediaJobPlanWithOverride(inputPath, outputPath, profile, audioProfile, overwrite, override)
	if err != nil {
		return job, http.StatusInternalServerError, err
	}
	args := FFmpegCommandBuilder{}.Build(plan)
	command := "ffmpeg " + shellJoin(args)
	now := time.Now()
	job.Progress = 5
	job.OutputPath = outputPath
	job.ErrorMessage = ""
	job.Notes = appendNote(job.Notes, "Conversion command: "+command)
	if !assetConversionOverrideEmpty(override) {
		job.Notes = appendNote(job.Notes, "Asset conversion overrides applied")
	}
	if audioProfile != nil {
		job.Notes = appendNote(job.Notes, "Audio enhancement profile: "+audioProfile.Key)
		job.Notes = appendNote(job.Notes, "Processing mode: "+plan.ProcessingMode)
	}
	if err := writeJobAsIsArtifact(h.db, job, plan.Profile, audioProfile, command, plan.ProcessingMode); err != nil {
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

	if err := h.startFFmpegJob(job.ID, args); err != nil {
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
	return path.Join(library.DestinationPath, outputFileRelativePath(libraryOutputBaseRelativePath(libraryRelativeMediaPath(mediaPath, library)), profile))
}

func plannedOutputPathForJob(db *gorm.DB, job models.QueueJob, library models.Library, profile models.Profile) string {
	relative := libraryOutputRelativePathForJob(db, job, library, libraryOutputBaseRelativePath(libraryRelativeMediaPath(job.MediaPath, library)))
	return path.Join(library.DestinationPath, outputFileRelativePath(relative, profile))
}

func libraryRelativeMediaPath(mediaPath string, library models.Library) string {
	relative := strings.TrimPrefix(mediaPath, strings.TrimRight(library.SourcePath, "/")+"/")
	if relative == mediaPath {
		return path.Base(mediaPath)
	}
	return relative
}

func libraryOutputBaseRelativePath(relative string) string {
	parts := pathSegments(relative)
	if len(parts) <= 1 {
		return relative
	}
	if isSourceBucketSegment(parts[0]) {
		return path.Join(parts[1:]...)
	}
	return relative
}

func isSourceBucketSegment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "movie", "movies", "series", "anime", "documentary", "documentaries", "concert", "concerts", "music-video", "music-videos":
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
	relative := fallbackRelative
	if libraryExtrasPathEnabled(library) && assetIsExtra(job.MediaPath, assetMetadataOverrides(db)) {
		return extrasOutputRelativePath(relative, libraryExtrasPathName(library))
	}
	if libraryEpisodeNamingEnabled(library) {
		relative = multiEpisodeOutputRelativePath(db, job, relative)
	}
	return relative
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
		}, nil
	}

	return nil, nil
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
	foundLoudnorm := false

	for _, rawFilter := range rawFilters {
		filter := strings.TrimSpace(rawFilter)
		if filter == "" {
			continue
		}
		if strings.HasPrefix(filter, "loudnorm=") {
			filters = append(filters, loudnorm)
			foundLoudnorm = true
			continue
		}
		filters = append(filters, filter)
	}

	if !foundLoudnorm {
		filters = append(filters, loudnorm)
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
		return "aresample=ocl=mono"
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
		return "aresample=ocl=stereo"
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

func plannedStagingOutputPath(db *gorm.DB, job models.QueueJob, library models.Library, profile models.Profile, paths pathSettings) string {
	relative := relativeMediaPath(job.MediaPath, paths.rawRoot)
	if relative == path.Base(job.MediaPath) && library.SourcePath != "" {
		relative = relativeMediaPath(job.MediaPath, library.SourcePath)
	}

	relative = libraryOutputBaseRelativePath(relative)
	relative = libraryOutputRelativePathForJob(db, job, library, relative)
	base := outputFileRelativePath(relative, profile)
	return filepath.Join(paths.stagingPath, fmt.Sprintf("job-%d", job.ID), filepath.FromSlash(base))
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

	dir := path.Dir(fallbackRelative)
	fileName := fmt.Sprintf("%s - S%02dE%02d%s", sanitizeMediaFileName(spec.SeriesTitle), spec.Season, spec.Episode, path.Ext(fallbackRelative))
	if dir == "." || dir == "/" {
		return fileName
	}
	return path.Join(dir, fileName)
}

func multiEpisodeNameSpecForJob(db *gorm.DB, job models.QueueJob) (multiEpisodeNameSpec, bool) {
	batchID := strings.TrimSpace(job.BatchID)
	if batchID == "" {
		return multiEpisodeNameSpec{}, false
	}

	var batchJobs []models.QueueJob
	if err := db.Where("batch_id = ?", batchID).Order("media_path asc, id asc").Find(&batchJobs).Error; err != nil || len(batchJobs) <= 1 {
		return multiEpisodeNameSpec{}, false
	}

	episode := 0
	for index, candidate := range batchJobs {
		if candidate.ID == job.ID || candidate.MediaPath == job.MediaPath {
			episode = index + 1
			break
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
