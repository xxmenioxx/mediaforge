package handlers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/applog"
	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

var runningJobProcesses = struct {
	sync.Mutex
	commands map[uint]*exec.Cmd
}{
	commands: map[uint]*exec.Cmd{},
}

func (h WorkerHandler) startFFmpegJob(jobID uint, args []string) error {
	durationSeconds := probeMediaDurationSeconds(h.db, jobID)
	progressArgs := ffmpegArgsWithProgress(args)

	cmd := exec.CommandContext(context.Background(), "ffmpeg", progressArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		applog.Event("error", "ffmpeg", "process_start_failed", map[string]any{"jobId": jobID}, err)
		return err
	}
	applog.Event("info", "ffmpeg", "process_started", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid}, nil)
	h.startSleepInhibitor(cmd.Process.Pid)

	registerRunningJobProcess(jobID, cmd)
	go h.monitorFFmpegJob(jobID, cmd, stdout, stderr, durationSeconds)

	return nil
}

func (h WorkerHandler) startSleepInhibitor(processID int) {
	if runtime.GOOS != "darwin" {
		return
	}
	snapshot, err := runtimeinfo.Latest(h.db)
	if err != nil {
		return
	}
	config, err := scheduler.LoadRuntimeBehavior(h.db, snapshot.SelectedProfile)
	if err != nil || !config.PreventSleepDuringJobs {
		return
	}
	command := exec.Command("caffeinate", "-dimsu", "-w", strconv.Itoa(processID))
	if err := command.Start(); err == nil {
		go func() { _ = command.Wait() }()
	}
}

func (h WorkerHandler) monitorFFmpegJob(jobID uint, cmd *exec.Cmd, stdout io.ReadCloser, stderr io.ReadCloser, durationSeconds float64) {
	var stderrBuffer bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuffer, stderr)
		close(stderrDone)
	}()

	scanner := bufio.NewScanner(stdout)
	lastProgress := 5
	lastSaved := time.Now()
	for scanner.Scan() {
		line := scanner.Text()
		if nextProgress, ok := progressFromFFmpegLine(line, durationSeconds); ok {
			if nextProgress > lastProgress && (nextProgress-lastProgress >= 2 || time.Since(lastSaved) > 2*time.Second) {
				h.updateRunningJobProgress(jobID, nextProgress)
				lastProgress = nextProgress
				lastSaved = time.Now()
			}
		}
	}

	err := cmd.Wait()
	<-stderrDone
	unregisterRunningJobProcess(jobID)

	var job models.QueueJob
	if dbErr := h.db.First(&job, jobID).Error; dbErr != nil {
		return
	}
	defer func() { _ = scheduler.DeactivateReservationResources(h.db, jobID) }()

	if job.Status == JobStatusCanceled {
		_ = transitionJobStage(h.db, &job, JobStageCanceled)
		if job.FinishedAt == nil {
			finishedAt := time.Now()
			job.FinishedAt = &finishedAt
			_ = h.db.Save(&job).Error
		}
		_ = writeJobResultArtifact(h.db, job, map[string]any{
			"status":   "canceled",
			"timing":   jobExecutionTiming(job, durationSeconds),
			"progress": job.Progress,
		})
		return
	}

	finishedAt := time.Now()
	job.FinishedAt = &finishedAt
	timing := jobExecutionTiming(job, durationSeconds)
	if err != nil {
		job.Status = JobStatusFailed
		job.Progress = 0
		job.ErrorMessage = strings.TrimSpace(lastOutputLines(stderrBuffer.String(), 14))
		_ = transitionJobStage(h.db, &job, JobStageFailed)
		if job.ErrorMessage == "" {
			job.ErrorMessage = fmt.Sprintf("ffmpeg exited with error: %v", err)
		}
		_ = h.db.Save(&job).Error
		applog.Event("error", "ffmpeg", "process_failed", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid, "stderr": job.ErrorMessage}, err)
		_ = writeJobResultArtifact(h.db, job, map[string]any{
			"status":   "failed",
			"error":    job.ErrorMessage,
			"timing":   timing,
			"progress": job.Progress,
		})
		return
	}

	job.Status = JobStatusCompleted
	job.Progress = 100
	job.ErrorMessage = ""
	applog.Event("info", "ffmpeg", "process_completed", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid}, nil)
	_ = h.db.Save(&job).Error
	_ = transitionJobStage(h.db, &job, JobStageValidating)
	automationResult := h.runAutomatedPipeline(job)
	_ = writeJobResultArtifact(h.db, job, map[string]any{
		"status":     "completed",
		"timing":     timing,
		"progress":   job.Progress,
		"automation": automationResult,
	})
}

func (h WorkerHandler) runAutomatedPipeline(job models.QueueJob) map[string]any {
	automation := pipelineAutomationSettings(h.db)
	result := map[string]any{
		"analysis":   automation.AutoAnalysisEnabled,
		"validation": automation.AutoValidationEnabled,
		"publisher":  automation.AutoPublisherEnabled,
	}

	if automation.AutoAnalysisEnabled {
		result["analysisStatus"] = "passed"
	} else {
		result["analysisStatus"] = "skipped"
	}

	if review := reviewForPath(job.MediaPath, assetReviewOverrides(h.db)); review.RequiresReview {
		result["stoppedAt"] = "review"
		result["message"] = "Asset is marked as requiring human review."
		return result
	}

	if !automation.AutoValidationEnabled {
		result["stoppedAt"] = "validation"
		result["message"] = "Automatic validation is disabled."
		return result
	}

	_ = transitionJobStage(h.db, &job, JobStageValidating)
	validationResult := validateQueueJob(h.db, job)
	job.ValidationStatus = validationResult.Status
	job.ValidationScore = validationResult.Score
	job.ValidationReport = validationResult.Report
	if err := h.db.Save(&job).Error; err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "validation"
		return result
	}
	result["validationStatus"] = validationResult.Status
	result["validationScore"] = validationResult.Score
	_ = transitionJobStage(h.db, &job, JobStageDirectPlayAnalysis)

	minimumScore := validationMinimumScore(h.db)
	if validationResult.Status == ValidationStatusFailed || validationResult.Score < minimumScore {
		_ = transitionJobStage(h.db, &job, JobStageFailed)
		result["stoppedAt"] = "validation_review"
		result["message"] = "Validation score requires human review."
		return result
	}

	if !automation.AutoPublisherEnabled {
		_ = transitionJobStage(h.db, &job, JobStageReadyToPublish)
		result["stoppedAt"] = "publisher"
		result["message"] = "Automatic publishing is disabled."
		return result
	}

	publishResult, err := (PublisherHandler{db: h.db}).publishQueueJob(job, false)
	if err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "publisher"
		return result
	}
	result["publishedPath"] = publishResult.PublishedPath
	result["stoppedAt"] = "complete"
	return result
}

type pipelineAutomation struct {
	AutoAnalysisEnabled   bool
	AutoValidationEnabled bool
	AutoPublisherEnabled  bool
}

func pipelineAutomationSettings(db *gorm.DB) pipelineAutomation {
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "pipelineAutomation").Error; err == nil && setting.Value != nil {
		values = setting.Value
	}

	return pipelineAutomation{
		AutoAnalysisEnabled:   boolSetting(values["autoAnalysisEnabled"], false),
		AutoValidationEnabled: boolSetting(values["autoValidationEnabled"], false),
		AutoPublisherEnabled:  boolSetting(values["autoPublisherEnabled"], false),
	}
}

func validationMinimumScore(db *gorm.DB) int {
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "validation").Error; err == nil && setting.Value != nil {
		values = setting.Value
	}
	return intSetting(values, "minimumScore", 90)
}

func jobExecutionTiming(job models.QueueJob, mediaDurationSeconds float64) map[string]any {
	result := map[string]any{
		"mediaDurationSeconds": mediaDurationSeconds,
	}
	if job.StartedAt != nil {
		result["startedAt"] = job.StartedAt.UTC().Format(time.RFC3339)
	}
	if job.FinishedAt != nil {
		result["finishedAt"] = job.FinishedAt.UTC().Format(time.RFC3339)
	}
	if job.StartedAt != nil && job.FinishedAt != nil {
		result["elapsedSeconds"] = job.FinishedAt.Sub(*job.StartedAt).Seconds()
	}
	return result
}

func (h WorkerHandler) updateRunningJobProgress(jobID uint, progress int) {
	progress = clamp(progress, 5, 95)
	_ = h.db.Model(&models.QueueJob{}).
		Where("id = ? AND status = ?", jobID, JobStatusRunning).
		Updates(map[string]any{"progress": progress}).Error
}

func registerRunningJobProcess(jobID uint, cmd *exec.Cmd) {
	runningJobProcesses.Lock()
	defer runningJobProcesses.Unlock()
	runningJobProcesses.commands[jobID] = cmd
}

func unregisterRunningJobProcess(jobID uint) {
	runningJobProcesses.Lock()
	defer runningJobProcesses.Unlock()
	delete(runningJobProcesses.commands, jobID)
}

func hasRunningJobProcess(jobID uint) bool {
	runningJobProcesses.Lock()
	defer runningJobProcesses.Unlock()
	return runningJobProcesses.commands[jobID] != nil
}

func cancelRunningJobProcess(jobID uint) bool {
	runningJobProcesses.Lock()
	cmd := runningJobProcesses.commands[jobID]
	runningJobProcesses.Unlock()

	if cmd == nil || cmd.Process == nil {
		return false
	}

	_ = cmd.Process.Kill()
	return true
}

func ffmpegArgsWithProgress(args []string) []string {
	progressArgs := []string{"-progress", "pipe:1", "-nostats"}
	if len(args) > 0 && args[0] == "-hide_banner" {
		return append(append([]string{"-hide_banner"}, progressArgs...), args[1:]...)
	}
	return append(progressArgs, args...)
}

func progressFromFFmpegLine(line string, durationSeconds float64) (int, bool) {
	if durationSeconds <= 0 {
		return 0, false
	}
	value, ok := strings.CutPrefix(line, "out_time_ms=")
	if !ok {
		return 0, false
	}

	microseconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}

	elapsedSeconds := microseconds / 1_000_000
	percent := 5 + int((elapsedSeconds/durationSeconds)*90)
	return clamp(percent, 5, 95), true
}

func probeMediaDurationSeconds(db *gorm.DB, jobID uint) float64 {
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil {
		return 0
	}

	output, err := exec.Command(
		"ffprobe",
		"-v",
		"error",
		"-show_entries",
		"format=duration",
		"-of",
		"default=noprint_wrappers=1:nokey=1",
		job.MediaPath,
	).Output()
	if err != nil {
		return 0
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0
	}

	return duration
}
