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

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

var runningJobProcesses = struct {
	sync.Mutex
	commands map[uint]*exec.Cmd
}{
	commands: map[uint]*exec.Cmd{},
}

func (h WorkerHandler) startFFmpegJob(jobID uint, args []string, durationSeconds float64) error {
	if durationSeconds <= 0 {
		durationSeconds = probeMediaDurationSeconds(h.db, jobID)
	}
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
	jobLog, logErr := openPersistentJobLogWriter(h.db, jobID)
	if logErr != nil {
		applog.Event("warning", "ffmpeg", "job_log_unavailable", map[string]any{"jobId": jobID}, logErr)
	}

	if err := cmd.Start(); err != nil {
		if jobLog != nil {
			_ = jobLog.Close()
		}
		applog.Event("error", "ffmpeg", "process_start_failed", map[string]any{"jobId": jobID}, err)
		return err
	}
	applog.Event("info", "ffmpeg", "process_started", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid}, nil)
	h.startSleepInhibitor(cmd.Process.Pid)

	registerRunningJobProcess(jobID, cmd)
	go h.monitorFFmpegJob(jobID, cmd, stdout, stderr, durationSeconds, jobLog)

	return nil
}

func (h WorkerHandler) startSleepInhibitor(processID int) {
	if runtime.GOOS != "darwin" {
		return
	}
	snapshot, err := runtimeinfo.Current(h.db)
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

func (h WorkerHandler) monitorFFmpegJob(jobID uint, cmd *exec.Cmd, stdout io.ReadCloser, stderr io.ReadCloser, durationSeconds float64, jobLog io.WriteCloser) {
	if jobLog != nil {
		defer jobLog.Close()
	}
	var logWriter io.Writer
	if jobLog != nil {
		logWriter = jobLog
	}
	var stderrBuffer bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		if logWriter != nil {
			_, _ = io.Copy(io.MultiWriter(&stderrBuffer, logWriter), stderr)
		} else {
			_, _ = io.Copy(&stderrBuffer, stderr)
		}
		close(stderrDone)
	}()

	scanner := bufio.NewScanner(stdout)
	lastProgress := 5
	lastSaved := time.Now()
	for scanner.Scan() {
		line := scanner.Text()
		if logWriter != nil {
			_, _ = io.WriteString(logWriter, "[ffmpeg] "+line+"\n")
		}
		if nextProgress, ok := progressFromFFmpegLine(line, durationSeconds); ok {
			if nextProgress > lastProgress && (nextProgress-lastProgress >= 2 || time.Since(lastSaved) > 2*time.Second) {
				if err := h.updateRunningJobProgress(jobID, nextProgress); err == nil {
					lastProgress = nextProgress
					lastSaved = time.Now()
				}
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
		_ = cleanupCanceledJob(h.db, job)
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
	if artifactErr := writeJobResultArtifact(h.db, job, map[string]any{
		"status":   "completed",
		"timing":   timing,
		"progress": job.Progress,
	}); artifactErr != nil {
		job.Notes = appendNote(job.Notes, "Result artifact could not be finalized: "+artifactErr.Error())
		_ = h.db.Model(&job).Update("notes", job.Notes).Error
		_ = transitionJobStage(h.db, &job, JobStageValidationReview)
		return
	}
	_ = h.runAutomatedPipeline(job)
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

	if !automation.AutoValidationEnabled {
		result["stoppedAt"] = "validation"
		result["message"] = "Automatic validation is disabled."
		_ = transitionJobStage(h.db, &job, JobStageValidationReview)
		return result
	}

	_ = transitionJobStage(h.db, &job, JobStageValidating)
	gate, err := (ValidationHandler{db: h.db}).validateAndPersist(job, "system")
	if err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "validation"
		_ = transitionJobStage(h.db, &job, JobStageValidationReview)
		return result
	}
	result["validationStatus"] = legacyValidationStatus(gate.Run.Outcome)
	result["validationOutcome"] = gate.Run.Outcome
	result["validationRunId"] = gate.Run.RunID
	result["validationScore"] = gate.Run.Score
	var current models.QueueJob
	if h.db.First(&current, job.ID).Error == nil {
		job = current
	}
	if !automaticValidationOutcomeAllowed(h.db, gate.Run) {
		_ = transitionJobStage(h.db, &job, JobStageValidationReview)
		result["stoppedAt"] = "validation_review"
		result["message"] = "Validation outcome is not enabled for automatic advancement."
		return result
	}
	_ = transitionJobStage(h.db, &job, JobStageReadyToPublish)

	if !automation.AutoPublisherEnabled {
		result["stoppedAt"] = "publisher"
		result["message"] = "Automatic publishing is disabled."
		return result
	}

	publishResult, err := (PublisherHandler{db: h.db}).publishQueueJobWithMode(
		job,
		false,
		true,
	)
	if err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "publisher"
		return result
	}
	result["publishedPath"] = publishResult.PublishedPath
	result["stoppedAt"] = "complete"
	return result
}

func automaticValidationOutcomeAllowed(db *gorm.DB, run models.ValidationRun) bool {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "validation").Error; err != nil || setting.Value == nil {
		return run.Outcome == "PASS"
	}
	allowed := validationAllowedOutcomeSetting(setting.Value["autoAdvanceOutcomes"])
	if len(allowed) == 0 {
		allowed = []string{"PASS"}
	}
	for _, outcome := range allowed {
		if strings.EqualFold(strings.TrimSpace(outcome), run.Outcome) {
			if run.Outcome == "PASS_WITH_WARNINGS" && run.ActionableWarnings {
				return false
			}
			return run.Outcome == "PASS" || run.Outcome == "PASS_WITH_WARNINGS"
		}
	}
	return false
}

func validationAllowedOutcomeSetting(value any) []string {
	values := []string{}
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, raw := range typed {
			if outcome := strings.TrimSpace(fmt.Sprint(raw)); outcome != "" && outcome != "<nil>" {
				values = append(values, outcome)
			}
		}
	case models.JSONList:
		for _, raw := range typed {
			if outcome := strings.TrimSpace(fmt.Sprint(raw)); outcome != "" && outcome != "<nil>" {
				values = append(values, outcome)
			}
		}
	}
	return values
}

type pipelineAutomation struct {
	AutoAnalysisEnabled               bool
	AutoValidationEnabled             bool
	AutoPublisherEnabled              bool
	PublishedJobReconciliationEnabled bool
}

func pipelineAutomationSettings(db *gorm.DB) pipelineAutomation {
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "pipelineAutomation").Error; err == nil && setting.Value != nil {
		values = setting.Value
	}

	return pipelineAutomation{
		AutoAnalysisEnabled:               boolSetting(values["autoAnalysisEnabled"], false),
		AutoValidationEnabled:             boolSetting(values["autoValidationEnabled"], false),
		AutoPublisherEnabled:              boolSetting(values["autoPublisherEnabled"], false),
		PublishedJobReconciliationEnabled: boolSetting(values["publishedJobReconciliationEnabled"], false),
	}
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

func (h WorkerHandler) updateRunningJobProgress(jobID uint, progress int) error {
	progress = clamp(progress, 5, 95)

	result := h.db.Model(&models.QueueJob{}).
		Where("id = ? AND status = ?", jobID, JobStatusRunning).
		Update("progress", progress)

	if result.Error != nil {
		applog.Event(
			"error",
			"ffmpeg",
			"progress_update_failed",
			map[string]any{
				"jobId":    jobID,
				"progress": progress,
			},
			result.Error,
		)

		return result.Error
	}

	if result.RowsAffected == 0 {
		applog.Event(
			"warning",
			"ffmpeg",
			"progress_update_skipped",
			map[string]any{
				"jobId":    jobID,
				"progress": progress,
			},
			nil,
		)
	}

	return nil
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
	value := ""
	for _, prefix := range []string{"out_time_us=", "out_time_ms="} {
		if candidate, ok := strings.CutPrefix(line, prefix); ok {
			value = candidate
			break
		}
	}
	elapsedSeconds := 0.0
	if value != "" {
		microseconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, false
		}
		elapsedSeconds = microseconds / 1_000_000
	} else if timestamp, ok := strings.CutPrefix(line, "out_time="); ok {
		parts := strings.Split(strings.TrimSpace(timestamp), ":")
		if len(parts) != 3 {
			return 0, false
		}
		hours, hoursErr := strconv.ParseFloat(parts[0], 64)
		minutes, minutesErr := strconv.ParseFloat(parts[1], 64)
		seconds, secondsErr := strconv.ParseFloat(parts[2], 64)
		if hoursErr != nil || minutesErr != nil || secondsErr != nil {
			return 0, false
		}
		elapsedSeconds = hours*3600 + minutes*60 + seconds
	} else {
		return 0, false
	}

	percent := 5 + int((elapsedSeconds/durationSeconds)*90)
	return clamp(percent, 5, 95), true
}

func probeMediaDurationSeconds(db *gorm.DB, jobID uint) float64 {
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil {
		return 0
	}
	var scan models.ScanResult
	if err := db.Where("path = ? AND duration > 0", job.MediaPath).Order("created_at desc").First(&scan).Error; err == nil {
		return scan.Duration
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
