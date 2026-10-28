package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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

var errQueueJobCancellationRequested = errors.New("queue job cancellation was requested before FFmpeg started")

func (h WorkerHandler) startFFmpegJob(jobID uint, args []string, durationSeconds float64) error {
	if durationSeconds <= 0 {
		durationSeconds = probeMediaDurationSeconds(h.db, jobID)
	}
	progressArgs := ffmpegArgsWithProgress(args)

	cmd := exec.CommandContext(context.Background(), "ffmpeg", progressArgs...)
	if !ffmpegProcessTreeContainmentSupported() {
		return fmt.Errorf("ffmpeg process-tree containment is unsupported on this operating system")
	}
	configureManagedProcessGroup(cmd)
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

	runningJobProcesses.Lock()
	if runningJobProcesses.commands[jobID] != nil {
		runningJobProcesses.Unlock()
		_ = stdout.Close()
		_ = stderr.Close()
		if jobLog != nil {
			_ = jobLog.Close()
		}
		return fmt.Errorf("queue job %d already has an active process", jobID)
	}
	var current models.QueueJob
	if err := h.db.First(&current, jobID).Error; err != nil {
		runningJobProcesses.Unlock()
		_ = stdout.Close()
		_ = stderr.Close()
		if jobLog != nil {
			_ = jobLog.Close()
		}
		return err
	}
	if current.CancellationRequestedAt != nil || current.Status == JobStatusCanceled {
		runningJobProcesses.Unlock()
		_ = stdout.Close()
		_ = stderr.Close()
		if jobLog != nil {
			_ = jobLog.Close()
		}
		return errQueueJobCancellationRequested
	}
	if err := cmd.Start(); err != nil {
		runningJobProcesses.Unlock()
		_ = stdout.Close()
		_ = stderr.Close()
		if jobLog != nil {
			_ = jobLog.Close()
		}
		applog.Event("error", "ffmpeg", "process_start_failed", map[string]any{"jobId": jobID}, err)
		return err
	}
	runningJobProcesses.commands[jobID] = cmd
	runningJobProcesses.Unlock()
	applog.Event("info", "ffmpeg", "process_started", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid}, nil)
	h.startSleepInhibitor(cmd.Process.Pid)

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
	if waitErr := waitFFmpegProcessGroupExit(cmd.Process.Pid); waitErr != nil {
		applog.Event("error", "ffmpeg", "process_group_exit_unverified", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid}, waitErr)
		return
	}
	unregisterRunningJobProcess(jobID)

	var job models.QueueJob
	if dbErr := h.db.First(&job, jobID).Error; dbErr != nil {
		return
	}
	defer func() { _ = scheduler.DeactivateReservationResources(h.db, jobID) }()

	if job.Status == JobStatusCanceled || job.CancellationRequestedAt != nil {
		canceledJob, finalized, finalizeErr := finalizeQueueJobCancellationAfterProcessTreeExit(h.db, jobID)
		if finalizeErr != nil {
			applog.Event("error", "queue", "cancellation_acknowledgment_persist_failed", map[string]any{"jobId": jobID}, finalizeErr)
			return
		}
		if !finalized {
			return
		}
		job = canceledJob
		_ = writeJobResultArtifact(h.db, job, map[string]any{
			"status":   "canceled",
			"timing":   jobExecutionTiming(job, durationSeconds),
			"progress": job.Progress,
		})
		return
	}

	finishedAt := time.Now()
	timing := jobExecutionTiming(job, durationSeconds)
	if err != nil {
		errorMessage := strings.TrimSpace(lastOutputLines(stderrBuffer.String(), 14))
		if errorMessage == "" {
			errorMessage = fmt.Sprintf("ffmpeg exited with error: %v", err)
		}
		job, canceled, persistErr := h.persistFFmpegExit(jobID, JobStatusFailed, 0, errorMessage, finishedAt)
		if persistErr != nil {
			applog.Event("error", "queue", "conversion_failure_persist_failed", map[string]any{"jobId": jobID}, persistErr)
			return
		}
		if canceled {
			_ = writeJobResultArtifact(h.db, job, map[string]any{"status": "canceled", "timing": timing, "progress": job.Progress})
			return
		}
		applog.Event("error", "ffmpeg", "process_failed", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid, "stderr": job.ErrorMessage}, err)
		_ = writeJobResultArtifact(h.db, job, map[string]any{
			"status":   "failed",
			"error":    job.ErrorMessage,
			"timing":   timing,
			"progress": job.Progress,
		})
		return
	}

	job, canceled, persistErr := h.persistFFmpegExit(jobID, JobStatusCompleted, 100, "", finishedAt)
	if persistErr != nil {
		applog.Event("error", "queue", "conversion_completion_persist_failed", map[string]any{"jobId": jobID}, persistErr)
		return
	}
	if canceled {
		_ = writeJobResultArtifact(h.db, job, map[string]any{"status": "canceled", "timing": timing, "progress": job.Progress})
		return
	}
	applog.Event("info", "ffmpeg", "process_completed", map[string]any{"jobId": jobID, "pid": cmd.Process.Pid}, nil)
	if artifactErr := writeJobResultArtifact(h.db, job, map[string]any{
		"status":   "completed",
		"timing":   timing,
		"progress": job.Progress,
	}); artifactErr != nil {
		job.Notes = appendNote(job.Notes, "Result artifact could not be finalized: "+artifactErr.Error())
		if err := h.db.Model(&job).Update("notes", job.Notes).Error; err != nil {
			artifactErr = fmt.Errorf("%w; persist Result failure note: %v", artifactErr, err)
		}
		if err := transitionJobStage(h.db, &job, JobStageValidationReview); err != nil {
			artifactErr = fmt.Errorf("%w; persist validation_review stage: %v", artifactErr, err)
		}
		applog.Event("error", "validation", "result_artifact_persist_failed", map[string]any{"jobId": jobID}, artifactErr)
		return
	}
	if _, pipelineErr := h.runAutomatedPipeline(job); pipelineErr != nil {
		applog.Event("error", "pipeline", "automated_handoff_failed", map[string]any{"jobId": jobID}, pipelineErr)
	}
}

func (h WorkerHandler) persistFFmpegExit(jobID uint, status string, progress int, errorMessage string, finishedAt time.Time) (models.QueueJob, bool, error) {
	var job models.QueueJob
	canceled := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&job, jobID).Error; err != nil {
			return err
		}
		if job.Status == JobStatusCanceled || job.CancellationRequestedAt != nil {
			alreadyCanceled := job.Status == JobStatusCanceled
			job.Status = JobStatusCanceled
			job.FinishedAt = &finishedAt
			if err := tx.Save(&job).Error; err != nil {
				return err
			}
			if err := transitionJobStage(tx, &job, JobStageCanceled); err != nil {
				return err
			}
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			if !alreadyCanceled {
				if err := persistCanceledJobCleanupIntent(tx, &job); err != nil {
					return err
				}
			}
			canceled = true
			return nil
		}
		previousStatus := job.Status
		job.Status = status
		job.Progress = progress
		job.ErrorMessage = errorMessage
		job.FinishedAt = &finishedAt
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		return transitionJobStage(tx, &job, stageForStatusUpdate(job, previousStatus))
	})
	if err != nil {
		return job, false, err
	}
	return job, canceled, nil
}

func (h WorkerHandler) runAutomatedPipeline(job models.QueueJob) (map[string]any, error) {
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
		if err := transitionJobStage(h.db, &job, JobStageValidationReview); err != nil {
			return result, fmt.Errorf("persist validation_review after automatic Validation was skipped: %w", err)
		}
		return result, nil
	}

	if err := transitionJobStage(h.db, &job, JobStageValidating); err != nil {
		return result, fmt.Errorf("persist validating before automatic Validation: %w", err)
	}
	gate, err := (ValidationHandler{db: h.db}).validateAndPersist(job, "system")
	if err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "validation"
		if stageErr := transitionJobStage(h.db, &job, JobStageValidationReview); stageErr != nil {
			return result, fmt.Errorf("automatic Validation failed: %w; persist validation_review: %v", err, stageErr)
		}
		return result, fmt.Errorf("automatic Validation failed: %w", err)
	}
	result["validationStatus"] = legacyValidationStatus(gate.Run.Outcome)
	result["validationOutcome"] = gate.Run.Outcome
	result["validationRunId"] = gate.Run.RunID
	result["validationScore"] = gate.Run.Score
	var current models.QueueJob
	if err := h.db.First(&current, job.ID).Error; err != nil {
		return result, fmt.Errorf("reload Queue job after automatic Validation: %w", err)
	}
	job = current
	handoffStage, reason, err := persistValidationHandoffStage(h.db, &job, gate.Run, gate.Findings, true)
	if err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "validation"
		return result, err
	}
	if handoffStage != JobStageReadyToPublish {
		result["stoppedAt"] = "validation_review"
		result["message"] = reason
		return result, nil
	}

	if !automation.AutoPublisherEnabled {
		result["stoppedAt"] = "publisher"
		result["message"] = "Automatic publishing is disabled."
		return result, nil
	}

	publishResult, err := (PublisherHandler{db: h.db}).publishQueueJobWithMode(
		job,
		false,
		true,
	)
	if err != nil {
		result["error"] = err.Error()
		result["stoppedAt"] = "publisher"
		return result, fmt.Errorf("automatic Publisher handoff failed: %w", err)
	}
	result["publishedPath"] = publishResult.PublishedPath
	result["stoppedAt"] = "complete"
	return result, nil
}

func automaticValidationOutcomeAllowed(db *gorm.DB, run models.ValidationRun) bool {
	if eligible, _ := validationHandoffDecision(run.Outcome, validationFindingsForRun(run)); !eligible {
		return false
	}
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

	return terminateFFmpegProcessGroup(cmd)
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
