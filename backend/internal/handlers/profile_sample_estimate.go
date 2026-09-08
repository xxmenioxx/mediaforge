package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const profileSampleEstimatePolicySettingKey = "profileSampleEstimatePolicy"

const (
	defaultProfileSampleEstimateHardwareWindows         = 5
	defaultProfileSampleEstimateSoftwareWindows         = 3
	defaultProfileSampleEstimateOperationTimeoutMinutes = 30
)

const (
	profileSampleEstimateOperationRetention  = 24 * time.Hour
	profileSampleEstimateOperationMaxHistory = 500
)

const (
	profileSampleEstimateQueued    = "queued"
	profileSampleEstimateRunning   = "running"
	profileSampleEstimateCompleted = "completed"
	profileSampleEstimateFailed    = "failed"
	profileSampleEstimateCanceled  = "canceled"
)

type ProfileSampleEstimateOperation struct {
	ID                    string         `json:"id"`
	Status                string         `json:"status"`
	Phase                 string         `json:"phase"`
	Progress              float64        `json:"progress"`
	CurrentSample         int            `json:"currentSample"`
	SampleCount           int            `json:"sampleCount"`
	CurrentSampleProgress float64        `json:"currentSampleProgress"`
	EncodedSeconds        float64        `json:"encodedSeconds"`
	TotalSampleSeconds    float64        `json:"totalSampleSeconds"`
	Speed                 float64        `json:"speed"`
	ETASeconds            int64          `json:"etaSeconds"`
	Result                models.JSONMap `json:"result,omitempty"`
	Error                 string         `json:"error,omitempty"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type profileSampleEstimateProgress struct {
	Phase                 string
	CurrentSample         int
	SampleCount           int
	CurrentSampleProgress float64
	EncodedSeconds        float64
	TotalSampleSeconds    float64
	Progress              float64
	Speed                 float64
	ETASeconds            int64
}

type profileSampleEstimatePolicy struct {
	HardwareWindows         int
	SoftwareWindows         int
	OperationTimeoutMinutes int
}

func defaultProfileSampleEstimatePolicy() profileSampleEstimatePolicy {
	return profileSampleEstimatePolicy{
		HardwareWindows:         defaultProfileSampleEstimateHardwareWindows,
		SoftwareWindows:         defaultProfileSampleEstimateSoftwareWindows,
		OperationTimeoutMinutes: defaultProfileSampleEstimateOperationTimeoutMinutes,
	}
}

func profileSampleEstimateInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		converted := int(typed)
		return converted, int64(converted) == typed
	case uint:
		converted := int(typed)
		return converted, converted >= 0 && uint(converted) == typed
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		converted := int(typed)
		return converted, converted >= 0 && uint32(converted) == typed
	case uint64:
		converted := int(typed)
		return converted, converted >= 0 && uint64(converted) == typed
	case float32:
		value := float64(typed)
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return 0, false
		}
		converted := int(value)
		return converted, float64(converted) == value
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed {
			return 0, false
		}
		converted := int(typed)
		return converted, float64(converted) == typed
	default:
		return 0, false
	}
}

func profileSampleEstimatePolicyFromValue(value models.JSONMap, strict bool) (profileSampleEstimatePolicy, error) {
	policy := defaultProfileSampleEstimatePolicy()
	fields := []struct {
		key         string
		minimum     int
		maximum     int
		destination *int
	}{
		{key: "hardwareWindows", minimum: 1, maximum: 10, destination: &policy.HardwareWindows},
		{key: "softwareWindows", minimum: 1, maximum: 10, destination: &policy.SoftwareWindows},
		{key: "operationTimeoutMinutes", minimum: 5, maximum: 120, destination: &policy.OperationTimeoutMinutes},
	}
	for _, field := range fields {
		raw, present := value[field.key]
		if !present {
			continue
		}
		parsed, valid := profileSampleEstimateInteger(raw)
		if !valid || parsed < field.minimum || parsed > field.maximum {
			if strict {
				return profileSampleEstimatePolicy{}, fmt.Errorf("%s must be an integer between %d and %d", field.key, field.minimum, field.maximum)
			}
			continue
		}
		*field.destination = parsed
	}
	return policy, nil
}

func profileSampleEstimatePolicyValue(value models.JSONMap) (models.JSONMap, error) {
	policy, err := profileSampleEstimatePolicyFromValue(value, true)
	if err != nil {
		return nil, err
	}
	return models.JSONMap{
		"hardwareWindows":         policy.HardwareWindows,
		"softwareWindows":         policy.SoftwareWindows,
		"operationTimeoutMinutes": policy.OperationTimeoutMinutes,
	}, nil
}

func loadProfileSampleEstimatePolicy(db *gorm.DB) profileSampleEstimatePolicy {
	defaults := defaultProfileSampleEstimatePolicy()
	if db == nil {
		return defaults
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", profileSampleEstimatePolicySettingKey).Error; err != nil {
		return defaults
	}
	policy, _ := profileSampleEstimatePolicyFromValue(setting.Value, false)
	return policy
}

func profileSampleEstimateUsesSoftwareEncoder(encoder string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(encoder)), "lib")
}

func profileSampleEstimateWindowCount(policy profileSampleEstimatePolicy, effectiveEncoder string) int {
	if profileSampleEstimateUsesSoftwareEncoder(effectiveEncoder) {
		return policy.SoftwareWindows
	}
	return policy.HardwareWindows
}

func distributedProfileSampleStarts(duration float64, sampleSeconds int, count int) []float64 {
	if duration <= 0 || sampleSeconds <= 0 || count <= 0 {
		return []float64{}
	}
	maxStart := math.Max(0, duration-float64(sampleSeconds))
	if maxStart == 0 || count == 1 {
		return []float64{math.Round(maxStart*0.5*1000) / 1000}
	}
	margin := 0.08
	if count <= 3 {
		margin = 0.20
	}
	starts := make([]float64, 0, count)
	for index := 0; index < count; index++ {
		position := margin + (1-2*margin)*float64(index)/float64(count-1)
		start := math.Round(position*maxStart*1000) / 1000
		if len(starts) == 0 || start != starts[len(starts)-1] {
			starts = append(starts, start)
		}
	}
	return starts
}

type profileSampleEstimateOperationStore struct {
	sync.RWMutex
	items   map[string]*ProfileSampleEstimateOperation
	cancels map[string]context.CancelFunc
}

var profileSampleEstimateOperations = profileSampleEstimateOperationStore{
	items:   map[string]*ProfileSampleEstimateOperation{},
	cancels: map[string]context.CancelFunc{},
}

var profileSampleEstimateSequence atomic.Uint64

type profileSampleEstimateRunner func(context.Context, profileSampleEstimateInput, func(profileSampleEstimateProgress)) (models.JSONMap, error)

type profileSampleEstimatePolicyRunner func(context.Context, profileSampleEstimateInput, profileSampleEstimatePolicy, func(profileSampleEstimateProgress)) (models.JSONMap, error)

func profileSampleEstimateRunnerWithPolicy(policy profileSampleEstimatePolicy, run profileSampleEstimatePolicyRunner) profileSampleEstimateRunner {
	return func(ctx context.Context, input profileSampleEstimateInput, progress func(profileSampleEstimateProgress)) (models.JSONMap, error) {
		return run(ctx, input, policy, progress)
	}
}

type profileSampleEstimateError struct {
	status int
	err    error
}

func (e profileSampleEstimateError) Error() string { return e.err.Error() }
func (e profileSampleEstimateError) Unwrap() error { return e.err }

func newProfileSampleEstimateError(status int, message string) error {
	return profileSampleEstimateError{status: status, err: fmt.Errorf("%s", message)}
}

func profileSampleEstimateHTTPStatus(err error) int {
	var runErr profileSampleEstimateError
	if errors.As(err, &runErr) {
		return runErr.status
	}
	return http.StatusInternalServerError
}

func (store *profileSampleEstimateOperationStore) copy(id string) (ProfileSampleEstimateOperation, bool) {
	store.RLock()
	defer store.RUnlock()
	operation := store.items[id]
	if operation == nil {
		return ProfileSampleEstimateOperation{}, false
	}
	copy := *operation
	if operation.Result != nil {
		copy.Result = models.JSONMap{}
		for key, value := range operation.Result {
			copy.Result[key] = value
		}
	}
	return copy, true
}

func (store *profileSampleEstimateOperationStore) update(id string, update func(*ProfileSampleEstimateOperation)) bool {
	store.Lock()
	defer store.Unlock()
	operation := store.items[id]
	if operation == nil {
		return false
	}
	update(operation)
	operation.UpdatedAt = time.Now()
	return true
}

func (store *profileSampleEstimateOperationStore) cancel(id string) (ProfileSampleEstimateOperation, bool, bool) {
	store.Lock()
	defer store.Unlock()
	operation := store.items[id]
	if operation == nil {
		return ProfileSampleEstimateOperation{}, false, false
	}
	if profileSampleEstimateTerminal(operation.Status) {
		return *operation, true, false
	}
	if cancel := store.cancels[id]; cancel != nil {
		cancel()
	}
	operation.Status = profileSampleEstimateCanceled
	operation.Phase = profileSampleEstimateCanceled
	operation.Error = ""
	operation.UpdatedAt = time.Now()
	return *operation, true, true
}

func (store *profileSampleEstimateOperationStore) cleanup(now time.Time) {
	store.Lock()
	defer store.Unlock()

	type terminalOperation struct {
		id        string
		updatedAt time.Time
	}
	terminal := make([]terminalOperation, 0, len(store.items))
	for id, operation := range store.items {
		if operation == nil || !profileSampleEstimateTerminal(operation.Status) {
			continue
		}
		if now.Sub(operation.UpdatedAt) > profileSampleEstimateOperationRetention {
			delete(store.items, id)
			delete(store.cancels, id)
			continue
		}
		terminal = append(terminal, terminalOperation{id: id, updatedAt: operation.UpdatedAt})
	}
	if len(terminal) <= profileSampleEstimateOperationMaxHistory {
		return
	}
	sort.Slice(terminal, func(i, j int) bool {
		if terminal[i].updatedAt.Equal(terminal[j].updatedAt) {
			return terminal[i].id > terminal[j].id
		}
		return terminal[i].updatedAt.After(terminal[j].updatedAt)
	})
	for _, operation := range terminal[profileSampleEstimateOperationMaxHistory:] {
		delete(store.items, operation.id)
		delete(store.cancels, operation.id)
	}
}

func profileSampleEstimateTerminal(status string) bool {
	return status == profileSampleEstimateCompleted || status == profileSampleEstimateFailed || status == profileSampleEstimateCanceled
}

func launchProfileSampleEstimateOperation(
	store *profileSampleEstimateOperationStore,
	slot chan struct{},
	input profileSampleEstimateInput,
	timeout time.Duration,
	run profileSampleEstimateRunner,
) ProfileSampleEstimateOperation {
	now := time.Now()
	store.cleanup(now)
	id := fmt.Sprintf("profile-sample-estimate-%d-%d", now.UnixNano(), profileSampleEstimateSequence.Add(1))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	operation := &ProfileSampleEstimateOperation{
		ID: id, Status: profileSampleEstimateQueued, Phase: "waiting_for_capacity", CreatedAt: now, UpdatedAt: now,
	}
	store.Lock()
	store.items[id] = operation
	store.cancels[id] = cancel
	store.Unlock()
	response := *operation
	go executeProfileSampleEstimateOperation(ctx, cancel, store, slot, id, input, run)
	return response
}

func executeProfileSampleEstimateOperation(
	ctx context.Context,
	cancel context.CancelFunc,
	store *profileSampleEstimateOperationStore,
	slot chan struct{},
	id string,
	input profileSampleEstimateInput,
	run profileSampleEstimateRunner,
) {
	defer func() {
		cancel()
		store.Lock()
		delete(store.cancels, id)
		store.Unlock()
		store.cleanup(time.Now())
	}()

	select {
	case slot <- struct{}{}:
		defer func() { <-slot }()
	case <-ctx.Done():
		finishProfileSampleEstimateContext(store, id, ctx.Err())
		return
	}

	store.update(id, func(operation *ProfileSampleEstimateOperation) {
		if operation.Status == profileSampleEstimateQueued {
			operation.Status = profileSampleEstimateRunning
			operation.Phase = "preparing"
		}
	})
	if ctx.Err() != nil {
		finishProfileSampleEstimateContext(store, id, ctx.Err())
		return
	}

	result, err := run(ctx, input, func(progress profileSampleEstimateProgress) {
		store.update(id, func(operation *ProfileSampleEstimateOperation) {
			if operation.Status != profileSampleEstimateRunning {
				return
			}
			operation.Phase = progress.Phase
			operation.Progress = progress.Progress
			operation.CurrentSample = progress.CurrentSample
			operation.SampleCount = progress.SampleCount
			operation.CurrentSampleProgress = progress.CurrentSampleProgress
			operation.EncodedSeconds = progress.EncodedSeconds
			operation.TotalSampleSeconds = progress.TotalSampleSeconds
			operation.Speed = progress.Speed
			operation.ETASeconds = progress.ETASeconds
		})
	})
	if err != nil {
		if ctx.Err() != nil {
			finishProfileSampleEstimateContext(store, id, ctx.Err())
			return
		}
		store.update(id, func(operation *ProfileSampleEstimateOperation) {
			if operation.Status != profileSampleEstimateRunning {
				return
			}
			operation.Status = profileSampleEstimateFailed
			operation.Phase = profileSampleEstimateFailed
			operation.Error = err.Error()
		})
		return
	}
	store.update(id, func(operation *ProfileSampleEstimateOperation) {
		if operation.Status != profileSampleEstimateRunning {
			return
		}
		operation.Status = profileSampleEstimateCompleted
		operation.Phase = profileSampleEstimateCompleted
		operation.Progress = 100
		operation.CurrentSampleProgress = 100
		operation.EncodedSeconds = operation.TotalSampleSeconds
		operation.ETASeconds = 0
		operation.Result = result
	})
}

func finishProfileSampleEstimateContext(store *profileSampleEstimateOperationStore, id string, err error) {
	store.update(id, func(operation *ProfileSampleEstimateOperation) {
		if operation.Status == profileSampleEstimateCanceled {
			return
		}
		if err == context.Canceled {
			operation.Status = profileSampleEstimateCanceled
			operation.Phase = profileSampleEstimateCanceled
			operation.Error = ""
			return
		}
		operation.Status = profileSampleEstimateFailed
		operation.Phase = profileSampleEstimateFailed
		operation.Error = "sample estimate operation exceeded configured runtime limit"
	})
}

func (h AssetHandler) StartProfileSampleEstimateOperation(c *gin.Context) {
	var input profileSampleEstimateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and profile are required"})
		return
	}
	policy := loadProfileSampleEstimatePolicy(h.db)
	operation := launchProfileSampleEstimateOperation(
		&profileSampleEstimateOperations,
		profileSampleEstimateSlot,
		input,
		time.Duration(policy.OperationTimeoutMinutes)*time.Minute,
		profileSampleEstimateRunnerWithPolicy(policy, h.runProfileSampleEstimateWithPolicy),
	)
	c.JSON(http.StatusAccepted, operation)
}

func (h AssetHandler) GetProfileSampleEstimateOperation(c *gin.Context) {
	profileSampleEstimateOperations.cleanup(time.Now())
	operation, ok := profileSampleEstimateOperations.copy(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile sample estimate operation not found"})
		return
	}
	c.JSON(http.StatusOK, operation)
}

func (h AssetHandler) CancelProfileSampleEstimateOperation(c *gin.Context) {
	operation, found, canceled := profileSampleEstimateOperations.cancel(c.Param("id"))
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile sample estimate operation not found"})
		return
	}
	if !canceled {
		c.JSON(http.StatusConflict, gin.H{"error": "profile sample estimate operation has already finished"})
		return
	}
	c.JSON(http.StatusAccepted, operation)
}

func (h AssetHandler) runProfileSampleEstimate(
	ctx context.Context,
	input profileSampleEstimateInput,
	progress func(profileSampleEstimateProgress),
) (models.JSONMap, error) {
	return h.runProfileSampleEstimateWithPolicy(ctx, input, loadProfileSampleEstimatePolicy(h.db), progress)
}

func (h AssetHandler) runProfileSampleEstimateWithPolicy(
	ctx context.Context,
	input profileSampleEstimateInput,
	policy profileSampleEstimatePolicy,
	progress func(profileSampleEstimateProgress),
) (models.JSONMap, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return nil, newProfileSampleEstimateError(http.StatusBadRequest, "a readable media path is required")
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		return nil, newProfileSampleEstimateError(http.StatusBadRequest, "a readable media path is required")
	}
	allowed, err := h.pathBelongsToReadableMediaRoot(resolvedPath)
	if err != nil || !allowed {
		return nil, newProfileSampleEstimateError(http.StatusForbidden, "media path is outside configured libraries")
	}
	streams, err := probeMediaStreams(resolvedPath)
	if err != nil || streams.Duration <= 0 {
		return nil, newProfileSampleEstimateError(http.StatusBadRequest, "could not determine media duration")
	}
	seconds := min(60, max(5, input.Seconds))
	if input.Seconds == 0 {
		seconds = 20
	}
	profile := normalizeHardwareQualityPreset(input.Profile)
	if len(streams.Video) == 0 {
		return nil, newProfileSampleEstimateError(http.StatusBadRequest, "asset has no video stream")
	}
	if streams.Video[0].Bitrate <= 0 {
		streams.Video[0].Bitrate = estimatedVideoBitrate(streams)
	}
	qualityIntent := qualityIntentForMedia(profile, resolvedPath, streams)
	profile = applyVideoToolboxQualityRecommendation(profile, qualityIntent)
	profile = applyQSVQualityRecommendation(profile, qualityIntent, capabilities.CheckEncoder("hevc_qsv"))
	profile = resolveHEVCLevel(profile, streams)
	profile = profileWithFinalColorPolicy(profile, streams.Video[0], resolvedVideoEncoder(profile))
	codecArgs := videoCodecArgsForSource(profile, &streams.Video[0])
	effectiveEncoder := argumentValue(codecArgs, "-c:v")
	workerArgs := videoWorkerArgsForSource(profile, &streams.Video[0])
	dir, err := os.MkdirTemp("", "mvforge-sample-estimate-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	starts := distributedProfileSampleStarts(
		streams.Duration,
		seconds,
		profileSampleEstimateWindowCount(policy, effectiveEncoder),
	)
	totalSampleSeconds := float64(len(starts) * seconds)
	encodingStartedAt := time.Now()
	emit := func(value profileSampleEstimateProgress) {
		if progress != nil {
			progress(value)
		}
	}
	emit(profileSampleEstimateProgress{Phase: "preparing", SampleCount: len(starts), TotalSampleSeconds: totalSampleSeconds})
	totalBytes := int64(0)
	completed := []float64{}
	for index, start := range starts {
		output := filepath.Join(dir, fmt.Sprintf("sample-%d.mkv", index))
		args := []string{"-hide_banner", "-loglevel", "error", "-ss", fmt.Sprintf("%.3f", start), "-i", resolvedPath, "-t", strconv.Itoa(seconds), "-map", "0:v:0?"}
		args = append(args, codecArgs...)
		args = append(args, workerArgs...)
		args = append(args, "-an", "-sn", "-dn", "-map_metadata", "-1", "-f", "matroska", "-y", output)
		err := runProfileSampleEstimateFFmpeg(ctx, args, float64(seconds), func(encodedSeconds, speed float64) {
			emit(profileSampleEstimateProgressValues(index, index+1, len(starts), float64(seconds), encodedSeconds, speed, time.Since(encodingStartedAt)))
		})
		if err != nil {
			return nil, fmt.Errorf("sample estimate failed: %w", err)
		}
		if info, statErr := os.Stat(output); statErr == nil && info.Size() > 0 {
			totalBytes += info.Size()
			completed = append(completed, start)
			emit(profileSampleEstimateProgressValues(index, index+1, len(starts), float64(seconds), float64(seconds), 0, time.Since(encodingStartedAt)))
		}
	}
	emit(profileSampleEstimateProgress{Phase: "finalizing", Progress: 100, CurrentSample: len(starts), SampleCount: len(starts), CurrentSampleProgress: 100, EncodedSeconds: totalSampleSeconds, TotalSampleSeconds: totalSampleSeconds})
	measuredSeconds := float64(len(completed) * seconds)
	if measuredSeconds <= 0 {
		return nil, fmt.Errorf("sample estimate produced no output")
	}
	videoBytes := int64(float64(totalBytes) / measuredSeconds * streams.Duration)
	result := models.JSONMap{"assetPath": resolvedPath, "durationSeconds": streams.Duration, "sampleSeconds": seconds, "sampleStarts": completed, "sampleCount": len(completed), "measuredVideoBytes": totalBytes, "estimatedVideoBytes": videoBytes, "measuredVideoBitrate": int64(float64(totalBytes) * 8 / measuredSeconds), "confidence": "high", "source": "distributed_profile_samples", "effectiveEncoder": effectiveEncoder, "hardwareQualityPreset": workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]), "sourceVideoBitrate": streams.Video[0].Bitrate, "sourceWidth": streams.Video[0].Width, "sourceHeight": streams.Video[0].Height, "persisted": false}
	if input.ProfileID > 0 {
		var saved models.Profile
		if h.db.First(&saved, input.ProfileID).Error == nil && scheduler.ProfileEstimateFingerprint(saved) == scheduler.ProfileEstimateFingerprint(input.Profile) {
			if err := persistProfileSampleEstimate(h.db, resolvedPath, saved, result); err != nil {
				applog.Event("warn", "analysis", "profile_sample_estimate_persist_failed", map[string]any{"path": resolvedPath, "profileId": input.ProfileID}, err)
			} else {
				result["persisted"] = true
			}
		}
	}
	applog.Event("info", "analysis", "profile_sample_estimate", map[string]any{"path": resolvedPath, "profileId": input.ProfileID, "estimatedVideoBytes": videoBytes, "effectiveEncoder": result["effectiveEncoder"], "sampleCount": len(completed), "persisted": result["persisted"]}, nil)
	return result, nil
}

func profileSampleEstimateProgressValues(
	completedSamples int,
	currentSample int,
	sampleCount int,
	sampleSeconds float64,
	currentSampleEncodedSeconds float64,
	ffmpegSpeed float64,
	elapsed time.Duration,
) profileSampleEstimateProgress {
	if sampleCount <= 0 || sampleSeconds <= 0 {
		return profileSampleEstimateProgress{Phase: "encoding"}
	}
	currentEncoded := math.Max(0, math.Min(sampleSeconds, currentSampleEncodedSeconds))
	total := float64(sampleCount) * sampleSeconds
	encoded := math.Max(0, math.Min(total, float64(completedSamples)*sampleSeconds+currentEncoded))
	progress := math.Max(0, math.Min(100, encoded/total*100))
	currentProgress := math.Max(0, math.Min(100, currentEncoded/sampleSeconds*100))
	speed := ffmpegSpeed
	if speed <= 0 && elapsed > 0 {
		speed = encoded / elapsed.Seconds()
	}
	if math.IsNaN(speed) || math.IsInf(speed, 0) || speed < 0 {
		speed = 0
	}
	eta := int64(0)
	if speed > 0 && encoded < total {
		seconds := (total - encoded) / speed
		if !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds > 0 {
			eta = int64(math.Ceil(seconds))
		}
	}
	return profileSampleEstimateProgress{
		Phase: "encoding", Progress: progress, CurrentSample: currentSample, SampleCount: sampleCount,
		CurrentSampleProgress: currentProgress, EncodedSeconds: encoded, TotalSampleSeconds: total, Speed: speed, ETASeconds: eta,
	}
}

func runProfileSampleEstimateFFmpeg(ctx context.Context, args []string, duration float64, progress func(encodedSeconds, speed float64)) error {
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
	lastSeconds, lastSpeed := 0.0, 0.0
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		seconds, speed, changed := profileSampleEstimateProgressLine(scanner.Text(), lastSeconds, lastSpeed)
		if !changed {
			continue
		}
		lastSeconds, lastSpeed = seconds, speed
		if progress != nil {
			progress(lastSeconds, lastSpeed)
		}
	}
	waitErr := cmd.Wait()
	<-done
	if scanErr := scanner.Err(); scanErr != nil && waitErr == nil {
		return scanErr
	}
	if waitErr != nil {
		message := strings.TrimSpace(lastOutputLines(stderrBuffer.String(), 14))
		if message == "" {
			message = waitErr.Error()
		}
		return fmt.Errorf("FFmpeg failed: %s", message)
	}
	return nil
}

func profileSampleEstimateProgressLine(line string, previousSeconds, previousSpeed float64) (float64, float64, bool) {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"out_time_us=", "out_time_ms="} {
		if value, ok := strings.CutPrefix(line, prefix); ok {
			microseconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || math.IsNaN(microseconds) || math.IsInf(microseconds, 0) {
				return previousSeconds, previousSpeed, false
			}
			return math.Max(0, microseconds/1_000_000), previousSpeed, true
		}
	}
	if value, ok := strings.CutPrefix(line, "out_time="); ok {
		parts := strings.Split(strings.TrimSpace(value), ":")
		if len(parts) != 3 {
			return previousSeconds, previousSpeed, false
		}
		hours, hoursErr := strconv.ParseFloat(parts[0], 64)
		minutes, minutesErr := strconv.ParseFloat(parts[1], 64)
		seconds, secondsErr := strconv.ParseFloat(parts[2], 64)
		if hoursErr != nil || minutesErr != nil || secondsErr != nil {
			return previousSeconds, previousSpeed, false
		}
		return math.Max(0, hours*3600+minutes*60+seconds), previousSpeed, true
	}
	if value, ok := strings.CutPrefix(line, "speed="); ok {
		value = strings.TrimSuffix(strings.TrimSpace(value), "x")
		speed, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(speed) || math.IsInf(speed, 0) || speed < 0 {
			return previousSeconds, previousSpeed, false
		}
		return previousSeconds, speed, true
	}
	return previousSeconds, previousSpeed, false
}
