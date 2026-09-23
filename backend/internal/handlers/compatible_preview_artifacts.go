package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	"github.com/gin-gonic/gin"
)

const (
	compatiblePreviewGenerationHandoffGrace = 100 * time.Millisecond
	compatiblePreviewDiagnosticLimit        = 256 * 1024
	compatiblePreviewCacheMaxItems          = 500
	compatiblePreviewCacheMaxBytes          = 2 << 30
)

type compatiblePreviewRetentionPolicy string

const (
	compatiblePreviewPersistentRetention compatiblePreviewRetentionPolicy = "persistent"
	compatiblePreviewEphemeralRetention  compatiblePreviewRetentionPolicy = "ephemeral"
)

type compatiblePreviewSourceRevision struct {
	Size         int64
	ModifiedAtNS int64
	Mode         uint32
}

func sourceRevisionForPath(path string) (compatiblePreviewSourceRevision, error) {
	info, err := os.Stat(path)
	if err != nil {
		return compatiblePreviewSourceRevision{}, err
	}
	if info.IsDir() {
		return compatiblePreviewSourceRevision{}, fmt.Errorf("media path is a directory")
	}
	return compatiblePreviewSourceRevision{
		Size:         info.Size(),
		ModifiedAtNS: info.ModTime().UnixNano(),
		Mode:         uint32(info.Mode()),
	}, nil
}

func (revision compatiblePreviewSourceRevision) matches(path string) bool {
	current, err := sourceRevisionForPath(path)
	return err == nil && current == revision
}

type compatiblePreviewDescriptor struct {
	Request                  compatiblePreviewRequest
	Path                     string
	SourceRevision           compatiblePreviewSourceRevision
	Start                    string
	Seconds                  int
	PreviewMode              string
	Normalization            previewNormalizationDecision
	Args                     []string
	PreviewProfile           *models.Profile
	EffectivePreviewProfile  *models.Profile
	ResolvedVideoDecision    models.JSONMap
	RequestedVideoEncoder    string
	EffectiveVideoEncoder    string
	RequestedQSVRateControl  string
	EffectiveQSVRateControl  string
	CanonicalEncodeSignature string
	ArtifactID               string
	RetentionPolicy          compatiblePreviewRetentionPolicy
	CachePath                string
}

type compatiblePreviewPrepareError struct {
	status int
	err    error
}

func (err *compatiblePreviewPrepareError) Error() string {
	if err == nil || err.err == nil {
		return "compatible preview preparation failed"
	}
	return err.err.Error()
}

func (err *compatiblePreviewPrepareError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

func compatiblePreviewPrepareFailure(status int, err error) error {
	return &compatiblePreviewPrepareError{status: status, err: err}
}

func compatiblePreviewPrepareStatus(err error) int {
	var prepareErr *compatiblePreviewPrepareError
	if errors.As(err, &prepareErr) && prepareErr.status > 0 {
		return prepareErr.status
	}
	return 500
}

func cloneCompatiblePreviewProfile(profile *models.Profile) *models.Profile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	return &cloned
}

func compatiblePreviewDescriptorFallback(input compatiblePreviewRequest, requestScope string) (*compatiblePreviewDescriptor, error) {
	start, ok := boundedPreviewStart(input.Start)
	if !ok {
		return nil, compatiblePreviewPrepareFailure(400, fmt.Errorf("start must be HH:MM:SS or seconds"))
	}
	seconds := input.Seconds
	if input.Mode == "quick" && seconds > 8 {
		seconds = 8
	}
	revision, _ := sourceRevisionForPath(input.Path)
	normalization := previewNormalizationDecision{Mode: input.PreviewNormalization, Reason: "preview descriptor was prepared without a configured database"}
	args := []string{
		"-hide_banner", "-loglevel", "verbose", "-ss", start, "-i", input.Path,
		"-t", strconv.Itoa(seconds), "-map", "0:v:0?", "-c:v", "libx264", "-f", "mp4", "pipe:1",
	}
	descriptor := &compatiblePreviewDescriptor{
		Request:         input,
		Path:            input.Path,
		SourceRevision:  revision,
		Start:           start,
		Seconds:         seconds,
		PreviewMode:     input.Mode,
		Normalization:   normalization,
		Args:            args,
		PreviewProfile:  cloneCompatiblePreviewProfile(input.Profile),
		RetentionPolicy: compatiblePreviewPersistentRetention,
	}
	if input.Ephemeral {
		descriptor.RetentionPolicy = compatiblePreviewEphemeralRetention
	}
	descriptor.CanonicalEncodeSignature = compatiblePreviewEncodeSignature(descriptor)
	descriptor.ArtifactID = compatiblePreviewArtifactID(descriptor.CanonicalEncodeSignature, descriptor.RetentionPolicy, requestScope)
	descriptor.CachePath = filepath.Join(os.TempDir(), "mvforge-preview-cache", descriptor.ArtifactID+".mp4")
	return descriptor, nil
}

func (h AssetHandler) prepareCompatiblePreviewDescriptor(input compatiblePreviewRequest, requestScope string) (*compatiblePreviewDescriptor, error) {
	normalized, err := normalizeCompatiblePreviewRequest(input)
	if err != nil {
		return nil, compatiblePreviewPrepareFailure(400, err)
	}
	if h.db == nil {
		return compatiblePreviewDescriptorFallback(normalized, requestScope)
	}

	path := normalized.Path
	profileID := strconv.FormatUint(uint64(normalized.ProfileID), 10)
	previewProfile := cloneCompatiblePreviewProfile(normalized.Profile)
	videoCodecOverride := strings.TrimSpace(normalized.VideoCodec)
	qualityValueOverride := strconv.FormatFloat(normalized.QualityValue, 'f', -1, 64)
	videoPresetOverride := strings.TrimSpace(normalized.VideoPreset)
	pixFmtOverride := strings.TrimSpace(normalized.PixelFormat)
	videoFiltersOverride := strings.TrimSpace(normalized.VideoFilters)
	x265ParamsOverride := strings.TrimSpace(normalized.X265Params)
	videoEncoderOverride := strings.TrimSpace(normalized.VideoEncoder)
	useHardwareOverride := normalized.UseHardware
	globalQualityOverride := normalized.GlobalQuality
	qsvRateControlOverride := strings.TrimSpace(normalized.QSVRateControl)
	qsvLookAheadDepthOverride := normalized.QSVLookAheadDepth
	qsvExtendedBRCOverride := normalized.QSVExtendedBRC
	qsvAdaptiveIOverride := normalized.QSVAdaptiveI
	qsvAdaptiveBOverride := normalized.QSVAdaptiveB
	qsvMBBRCModeOverride := normalizedQSVMBBRCMode(normalized.QSVMBBRCMode)
	qsvRDOModeOverride := normalizedQSVRDOMode(normalized.QSVRDOMode)
	qsvPStrategyOverride := normalized.QSVPStrategy
	previewNormalizationMode := normalized.PreviewNormalization
	subtitleStreamIndex := -1
	if normalized.SubtitleStreamIndex != nil {
		subtitleStreamIndex = *normalized.SubtitleStreamIndex
	}
	previewMode := normalized.Mode
	start, ok := boundedPreviewStart(normalized.Start)
	if !ok {
		return nil, compatiblePreviewPrepareFailure(400, fmt.Errorf("start must be HH:MM:SS or seconds"))
	}
	seconds := normalized.Seconds
	if previewMode == "quick" && seconds > 8 {
		seconds = 8
	}
	if path == "" {
		return nil, compatiblePreviewPrepareFailure(400, fmt.Errorf("path is required"))
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		return nil, compatiblePreviewPrepareFailure(500, err)
	}
	path = resolvedPath
	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		return nil, compatiblePreviewPrepareFailure(400, fmt.Errorf("path is not a supported media file"))
	}
	allowed, err := h.pathBelongsToReadableMediaRoot(path)
	if err != nil {
		return nil, compatiblePreviewPrepareFailure(500, err)
	}
	if !allowed {
		return nil, compatiblePreviewPrepareFailure(403, fmt.Errorf("media path is outside configured libraries"))
	}
	sourceRevision, err := sourceRevisionForPath(path)
	if err != nil {
		return nil, compatiblePreviewPrepareFailure(404, fmt.Errorf("media path is not readable from the backend container"))
	}

	analyzeSize := "100M"
	if previewMode == "quick" {
		analyzeSize = "10M"
		videoPresetOverride = "veryfast"
		x265ParamsOverride = ""
	}
	inputStart := start
	subtitlePrerollSeconds := 0
	if subtitleStreamIndex >= 0 {
		startSeconds := previewTimestampSeconds(start)
		subtitlePrerollSeconds = min(10, startSeconds)
		inputStart = secondsToTimestamp(startSeconds - subtitlePrerollSeconds)
	}
	args := []string{
		"-hide_banner", "-loglevel", "verbose", "-analyzeduration", analyzeSize,
		"-probesize", analyzeSize, "-ss", inputStart, "-i", path,
	}
	if subtitlePrerollSeconds > 0 {
		args = append(args, "-ss", strconv.Itoa(subtitlePrerollSeconds))
	}
	args = append(args, "-t", strconv.Itoa(seconds))
	normalization := buildPreviewNormalization(path, previewNormalizationMode)
	videoFilter := joinPreviewFilters(normalization.Filter, previewVideoFilterChain(videoFiltersOverride, previewMode))
	requestedVideoEncoder := strings.ToLower(strings.TrimSpace(videoEncoderOverride))
	if requestedVideoEncoder == "" && previewProfile != nil {
		requestedVideoEncoder = strings.ToLower(strings.TrimSpace(resolvedVideoEncoder(*previewProfile)))
	}

	var effectivePreviewProfile *models.Profile
	var resolvedVideoDecision models.JSONMap
	var videoCodecArguments []string
	if previewProfile != nil {
		profile := normalizeHardwareQualityPreset(*previewProfile)
		if streams, probeErr := probeMediaStreams(path); probeErr == nil && len(streams.Video) > 0 {
			if streams.Video[0].Bitrate <= 0 {
				streams.Video[0].Bitrate = estimatedVideoBitrate(streams)
			}
			interlace := h.previewInterlaceAnalysis(path, streams)
			cadence, cadenceRecommendation := h.previewCadenceResolution(path)
			profile, probeErr = resolvePreviewVideoProfile(h.db, path, profile, streams, interlace, cadence, cadenceRecommendation)
			if probeErr != nil {
				return nil, compatiblePreviewPrepareFailure(422, probeErr)
			}
			resolvedVideoDecision = effectiveVideoDecision(profile)
			profile = profileWithPreviewDisplayNormalization(profile, normalization.Filter)
			effectivePreviewProfile = &profile
			videoCodecArguments = append(videoCodecArgsForSource(profile, &streams.Video[0]), videoWorkerArgsForSource(profile, &streams.Video[0])...)
			videoCodecArguments = append(videoCodecArguments, cadenceOutputArgs(profile)...)
		} else {
			profile = resolveHEVCLevel(profile, MediaStreamInventory{})
			profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
			profile.WorkerConfig["videoFilters"] = joinPreviewFilters(normalization.Filter, existingVideoFilters(profile))
			effectivePreviewProfile = &profile
			resolvedVideoDecision = effectiveVideoDecision(profile)
			videoCodecArguments = append(videoCodecArgs(profile), videoWorkerArgs(profile)...)
		}
	} else {
		videoCodecArguments = previewVideoCodecArgs(h.db, profileID, videoCodecOverride, qualityValueOverride, videoPresetOverride, pixFmtOverride, x265ParamsOverride, videoEncoderOverride, useHardwareOverride, globalQualityOverride, qsvRateControlOverride, qsvLookAheadDepthOverride, qsvExtendedBRCOverride, qsvAdaptiveIOverride, qsvAdaptiveBOverride, qsvMBBRCModeOverride, qsvRDOModeOverride, min(2, max(0, qsvPStrategyOverride)))
	}

	if effectivePreviewProfile != nil {
		videoFilter = argumentValue(videoCodecArguments, "-vf")
		if strings.TrimSpace(videoFilter) == "" {
			videoFilter = existingVideoFilters(*effectivePreviewProfile)
		}
	}
	if subtitleStreamIndex >= 0 {
		filter, filterErr := previewSubtitleFilter(path, subtitleStreamIndex, videoFilter)
		if filterErr != nil {
			return nil, compatiblePreviewPrepareFailure(400, filterErr)
		}
		args = append(args, "-filter_complex", filter, "-map", "[preview_video]")
		if effectivePreviewProfile != nil {
			effectivePreviewProfile.WorkerConfig["videoFilters"] = ""
			videoCodecArguments = removeFFmpegOption(videoCodecArguments, "-vf")
		}
	} else {
		args = append(args, "-map", "0:v:0?")
		if effectivePreviewProfile == nil {
			args = append(args, "-vf", videoFilter)
		}
	}
	effectiveVideoEncoder := argumentValue(videoCodecArguments, "-c:v")
	args = append(args, videoCodecArguments...)
	if normalization.Applied && normalization.Mode == "normalize_bt709" {
		args = append(args, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-color_range", "tv")
	}
	args = append(args, "-an", "-sn", "-dn", "-map_metadata", "-1", "-avoid_negative_ts", "make_zero", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")

	requestedQSVRateControl := ""
	if previewProfile != nil {
		requestedQSVRateControl = strings.ToLower(strings.TrimSpace(workerStringValue(previewProfile.WorkerConfig["qsvRateControl"])))
	}
	effectiveQSVRateControl := ""
	if effectivePreviewProfile != nil {
		effectiveQSVRateControl = strings.ToLower(strings.TrimSpace(workerStringValue(effectivePreviewProfile.WorkerConfig["qsvEffectiveRateControl"])))
		if effectiveQSVRateControl == "" {
			effectiveQSVRateControl = strings.ToLower(strings.TrimSpace(workerStringValue(effectivePreviewProfile.WorkerConfig["qsvRateControl"])))
		}
	}

	descriptor := &compatiblePreviewDescriptor{
		Request:                 normalized,
		Path:                    path,
		SourceRevision:          sourceRevision,
		Start:                   start,
		Seconds:                 seconds,
		PreviewMode:             previewMode,
		Normalization:           normalization,
		Args:                    append([]string(nil), args...),
		PreviewProfile:          previewProfile,
		EffectivePreviewProfile: effectivePreviewProfile,
		ResolvedVideoDecision:   resolvedVideoDecision,
		RequestedVideoEncoder:   requestedVideoEncoder,
		EffectiveVideoEncoder:   effectiveVideoEncoder,
		RequestedQSVRateControl: requestedQSVRateControl,
		EffectiveQSVRateControl: effectiveQSVRateControl,
		RetentionPolicy:         compatiblePreviewPersistentRetention,
	}
	if normalized.Ephemeral {
		descriptor.RetentionPolicy = compatiblePreviewEphemeralRetention
	}
	descriptor.CanonicalEncodeSignature = compatiblePreviewEncodeSignature(descriptor)
	descriptor.ArtifactID = compatiblePreviewArtifactID(descriptor.CanonicalEncodeSignature, descriptor.RetentionPolicy, requestScope)
	descriptor.CachePath = filepath.Join(os.TempDir(), "mvforge-preview-cache", descriptor.ArtifactID+".mp4")
	return descriptor, nil
}

func (h AssetHandler) compatiblePreviewDescriptorForEntry(entry *compatiblePreviewRequestEntry, requestScope string) (*compatiblePreviewDescriptor, error) {
	if entry == nil {
		return nil, fmt.Errorf("compatible preview request is not available")
	}
	entry.prepareMu.Lock()
	defer entry.prepareMu.Unlock()

	entry.mu.RLock()
	input := entry.Request
	descriptor := entry.Descriptor
	entry.mu.RUnlock()
	if descriptor != nil && descriptor.SourceRevision.matches(descriptor.Path) {
		touchCompatiblePreviewArtifactScope(descriptor.ArtifactID, time.Now())
		return descriptor, nil
	}

	prepared, err := h.prepareCompatiblePreviewDescriptor(input, requestScope)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	entry.Descriptor = prepared
	entry.mu.Unlock()
	touchCompatiblePreviewArtifactScope(prepared.ArtifactID, time.Now())
	return prepared, nil
}

func writeCompatiblePreviewError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error": "compatible preview generation canceled",
			"code":  "canceled",
		})
		return
	}
	var preparationErr *compatiblePreviewPrepareError
	if errors.As(err, &preparationErr) {
		c.JSON(preparationErr.status, gin.H{"error": preparationErr.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": boundedCompatiblePreviewError("", err).Error()})
}

func compatiblePreviewEncodeSignature(descriptor *compatiblePreviewDescriptor) string {
	args := make([]string, 0, len(descriptor.Args))
	for index := 0; index < len(descriptor.Args); index++ {
		if descriptor.Args[index] == "-loglevel" && index+1 < len(descriptor.Args) {
			index++
			continue
		}
		args = append(args, descriptor.Args[index])
	}
	identity := struct {
		Path             string
		SourceRevision   compatiblePreviewSourceRevision
		Start            string
		Seconds          int
		PreviewMode      string
		Normalization    previewNormalizationDecision
		Args             []string
		EffectiveProfile *models.Profile
		VideoDecision    models.JSONMap
	}{
		Path: descriptor.Path, SourceRevision: descriptor.SourceRevision, Start: descriptor.Start,
		Seconds: descriptor.Seconds, PreviewMode: descriptor.PreviewMode, Normalization: descriptor.Normalization,
		Args: args, EffectiveProfile: descriptor.EffectivePreviewProfile, VideoDecision: descriptor.ResolvedVideoDecision,
	}
	encoded, _ := json.Marshal(identity)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func compatiblePreviewArtifactID(signature string, policy compatiblePreviewRetentionPolicy, requestScope string) string {
	scope := "persistent"
	if policy == compatiblePreviewEphemeralRetention {
		scope = "ephemeral\x00" + requestScope
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(signature+"\x00"+scope)))
}

type compatiblePreviewArtifactState string

const (
	compatiblePreviewArtifactAbsent     compatiblePreviewArtifactState = "absent"
	compatiblePreviewArtifactGenerating compatiblePreviewArtifactState = "generating"
	compatiblePreviewArtifactReady      compatiblePreviewArtifactState = "ready"
	compatiblePreviewArtifactCanceled   compatiblePreviewArtifactState = "canceled"
	compatiblePreviewArtifactFailed     compatiblePreviewArtifactState = "failed"
)

type compatiblePreviewArtifact struct {
	mu              sync.Mutex
	done            chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	state           compatiblePreviewArtifactState
	err             error
	waiters         int
	cancelTimer     *time.Timer
	destination     string
	logPath         string
	retentionPolicy compatiblePreviewRetentionPolicy
	scopeExpiresAt  time.Time
	lastUsedAt      time.Time
}

var compatiblePreviewArtifacts = struct {
	sync.Mutex
	items map[string]*compatiblePreviewArtifact
}{items: map[string]*compatiblePreviewArtifact{}}

type compatiblePreviewArtifactLease struct {
	entry *compatiblePreviewArtifact
	once  sync.Once
}

func (lease *compatiblePreviewArtifactLease) release() {
	if lease == nil || lease.entry == nil {
		return
	}
	lease.once.Do(func() { releaseCompatiblePreviewArtifact(lease.entry) })
}

func compatiblePreviewArtifactPathActive(path string) bool {
	compatiblePreviewArtifacts.Lock()
	entries := make([]*compatiblePreviewArtifact, 0, len(compatiblePreviewArtifacts.items))
	for _, entry := range compatiblePreviewArtifacts.items {
		entries = append(entries, entry)
	}
	compatiblePreviewArtifacts.Unlock()
	for _, entry := range entries {
		entry.mu.Lock()
		active := (entry.destination == path || entry.logPath == path) &&
			(entry.waiters > 0 || entry.state == compatiblePreviewArtifactGenerating)
		entry.mu.Unlock()
		if active {
			return true
		}
	}
	return false
}

func cleanupCompatiblePreviewEphemeralArtifacts(directory string, now time.Time) {
	compatiblePreviewArtifacts.Lock()
	entries := make(map[string]*compatiblePreviewArtifact, len(compatiblePreviewArtifacts.items))
	for artifactID, entry := range compatiblePreviewArtifacts.items {
		entries[artifactID] = entry
	}
	compatiblePreviewArtifacts.Unlock()

	for artifactID, entry := range entries {
		entry.mu.Lock()
		active := entry.waiters > 0 || entry.state == compatiblePreviewArtifactGenerating
		expired := entry.retentionPolicy == compatiblePreviewEphemeralRetention &&
			!entry.scopeExpiresAt.IsZero() && !now.Before(entry.scopeExpiresAt) && !active
		destination, logPath := entry.destination, entry.logPath
		entry.mu.Unlock()
		if !expired || filepath.Dir(destination) != directory {
			continue
		}
		_ = os.Remove(destination)
		_ = os.Remove(logPath)
		compatiblePreviewArtifacts.Lock()
		if compatiblePreviewArtifacts.items[artifactID] == entry {
			delete(compatiblePreviewArtifacts.items, artifactID)
		}
		compatiblePreviewArtifacts.Unlock()
	}
}

func touchCompatiblePreviewArtifactScope(artifactID string, now time.Time) {
	compatiblePreviewArtifacts.Lock()
	entry := compatiblePreviewArtifacts.items[artifactID]
	compatiblePreviewArtifacts.Unlock()
	if entry == nil {
		return
	}
	entry.mu.Lock()
	entry.lastUsedAt = now
	if entry.retentionPolicy == compatiblePreviewEphemeralRetention {
		entry.scopeExpiresAt = now.Add(compatiblePreviewRequestRetention)
	}
	entry.mu.Unlock()
}

func compatiblePreviewArtifactScopeExpired(artifactID string, now time.Time) bool {
	compatiblePreviewArtifacts.Lock()
	entry := compatiblePreviewArtifacts.items[artifactID]
	compatiblePreviewArtifacts.Unlock()
	if entry == nil {
		return true
	}
	entry.mu.Lock()
	expired := entry.retentionPolicy == compatiblePreviewEphemeralRetention && !entry.scopeExpiresAt.After(now) && entry.waiters == 0 && entry.state != compatiblePreviewArtifactGenerating
	entry.mu.Unlock()
	return expired
}

type compatiblePreviewBoundedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *compatiblePreviewBoundedBuffer) Write(input []byte) (int, error) {
	if buffer.limit <= 0 {
		return len(input), nil
	}
	remaining := buffer.limit - buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(input), nil
	}
	if len(input) > remaining {
		_, _ = buffer.Buffer.Write(input[:remaining])
		buffer.truncated = true
		return len(input), nil
	}
	return buffer.Buffer.Write(input)
}

func boundedCompatiblePreviewError(stderr string, err error) error {
	message := strings.TrimSpace(stderr)
	if message == "" && err != nil {
		message = err.Error()
	}
	if message == "" {
		message = "compatible preview generation failed"
	}
	if len(message) > compatiblePreviewDiagnosticLimit {
		message = message[:compatiblePreviewDiagnosticLimit] + " [truncated]"
	}
	return errors.New(message)
}

func runCompatiblePreviewGeneration(entry *compatiblePreviewArtifact, args []string) {
	destination := entry.destination
	directory := filepath.Dir(destination)
	var generationErr error
	state := compatiblePreviewArtifactReady
	if err := os.MkdirAll(directory, 0o755); err != nil {
		generationErr = err
		state = compatiblePreviewArtifactFailed
	} else {
		cleanupPreviewCache(directory, 24*time.Hour)
		staging, err := os.CreateTemp(directory, ".mvforge-preview-*.partial")
		if err != nil {
			generationErr = err
			state = compatiblePreviewArtifactFailed
		} else {
			stagingPath := staging.Name()
			stderr := &compatiblePreviewBoundedBuffer{limit: compatiblePreviewDiagnosticLimit}
			cmd := exec.CommandContext(entry.ctx, "ffmpeg", args...)
			cmd.Stdout = staging
			cmd.Stderr = stderr
			runErr := cmd.Run()
			closeErr := staging.Close()
			if runErr != nil || closeErr != nil {
				if runErr != nil && errors.Is(entry.ctx.Err(), context.Canceled) {
					state = compatiblePreviewArtifactCanceled
					generationErr = context.Canceled
				} else {
					state = compatiblePreviewArtifactFailed
					generationErr = boundedCompatiblePreviewError(stderr.String(), firstNonNilError(runErr, closeErr))
				}
				_ = os.Remove(stagingPath)
			} else if errors.Is(entry.ctx.Err(), context.Canceled) {
				state = compatiblePreviewArtifactCanceled
				generationErr = context.Canceled
				_ = os.Remove(stagingPath)
			} else if info, statErr := os.Stat(stagingPath); statErr != nil || info.Size() <= 0 {
				state = compatiblePreviewArtifactFailed
				generationErr = boundedCompatiblePreviewError(stderr.String(), fmt.Errorf("generated preview artifact is empty"))
				_ = os.Remove(stagingPath)
			} else if renameErr := os.Rename(stagingPath, destination); renameErr != nil {
				state = compatiblePreviewArtifactFailed
				generationErr = renameErr
				_ = os.Remove(stagingPath)
			} else {
				logPath := destination + ".encoder.log"
				logStaging, logErr := os.CreateTemp(directory, ".mvforge-preview-log-*.partial")
				if logErr == nil {
					logStagingPath := logStaging.Name()
					_, _ = io.Copy(logStaging, bytes.NewReader(stderr.Bytes()))
					if closeLogErr := logStaging.Close(); closeLogErr == nil {
						if renameLogErr := os.Rename(logStagingPath, logPath); renameLogErr != nil {
							_ = os.Remove(logStagingPath)
						}
					} else {
						_ = os.Remove(logStagingPath)
					}
				}
			}
		}
	}
	if state == compatiblePreviewArtifactReady {
		if info, err := os.Stat(destination); err != nil || info.Size() <= 0 {
			state = compatiblePreviewArtifactFailed
			generationErr = boundedCompatiblePreviewError("", fmt.Errorf("published preview artifact is not usable"))
		}
	}
	entry.mu.Lock()
	entry.state = state
	entry.err = generationErr
	if entry.cancelTimer != nil {
		entry.cancelTimer.Stop()
		entry.cancelTimer = nil
	}
	close(entry.done)
	entry.mu.Unlock()
	if state != compatiblePreviewArtifactReady {
		compatiblePreviewArtifacts.Lock()
		if compatiblePreviewArtifacts.items[artifactIDFromDestination(destination)] == entry {
			delete(compatiblePreviewArtifacts.items, artifactIDFromDestination(destination))
		}
		compatiblePreviewArtifacts.Unlock()
	}
}

func artifactIDFromDestination(destination string) string {
	base := filepath.Base(destination)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func newCompatiblePreviewArtifactEntry(descriptor *compatiblePreviewDescriptor, now time.Time, state compatiblePreviewArtifactState) *compatiblePreviewArtifact {
	entry := &compatiblePreviewArtifact{done: make(chan struct{}), state: state, destination: descriptor.CachePath, logPath: descriptor.CachePath + ".encoder.log", retentionPolicy: descriptor.RetentionPolicy, scopeExpiresAt: now.Add(compatiblePreviewRequestRetention), lastUsedAt: now}
	if state == compatiblePreviewArtifactGenerating {
		entry.ctx, entry.cancel = context.WithCancel(context.Background())
		entry.waiters = 1
	} else if state == compatiblePreviewArtifactReady {
		close(entry.done)
	}
	return entry
}

func acquireCompatiblePreviewArtifact(ctx context.Context, descriptor *compatiblePreviewDescriptor) (bool, *compatiblePreviewArtifactLease, error) {
	if descriptor == nil {
		return false, nil, fmt.Errorf("compatible preview descriptor is required")
	}
	destination := descriptor.CachePath
	now := time.Now()
	if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
		compatiblePreviewArtifacts.Lock()
		entry := compatiblePreviewArtifacts.items[descriptor.ArtifactID]
		if entry == nil {
			entry = newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactReady)
			compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
			entry.mu.Lock()
		} else {
			entry.mu.Lock()
			if entry.state == compatiblePreviewArtifactCanceled || entry.state == compatiblePreviewArtifactFailed {
				entry.mu.Unlock()
				entry = newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactReady)
				compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
				entry.mu.Lock()
			}
		}
		entry.waiters++
		entry.lastUsedAt = now
		entry.mu.Unlock()
		compatiblePreviewArtifacts.Unlock()
		return true, &compatiblePreviewArtifactLease{entry: entry}, nil
	}

	compatiblePreviewArtifacts.Lock()
	entry := compatiblePreviewArtifacts.items[descriptor.ArtifactID]
	startGeneration := false
	if entry == nil {
		entry = newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactGenerating)
		compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
		startGeneration = true
	} else {
		entry.mu.Lock()
		switch entry.state {
		case compatiblePreviewArtifactGenerating:
			if entry.cancelTimer != nil {
				entry.cancelTimer.Stop()
				entry.cancelTimer = nil
			}
			entry.waiters++
			entry.lastUsedAt = now
			entry.mu.Unlock()
		case compatiblePreviewArtifactReady:
			// The published-file check above failed, so a ready entry is stale.
			entry.mu.Unlock()
			entry = newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactGenerating)
			compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
			startGeneration = true
		case compatiblePreviewArtifactCanceled, compatiblePreviewArtifactFailed, compatiblePreviewArtifactAbsent:
			// Terminal non-ready entries must never accept a new consumer.
			entry.mu.Unlock()
			entry = newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactGenerating)
			compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
			startGeneration = true
		default:
			entry.mu.Unlock()
			entry = newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactGenerating)
			compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
			startGeneration = true
		}
	}
	compatiblePreviewArtifacts.Unlock()
	if startGeneration {
		go runCompatiblePreviewGeneration(entry, descriptor.Args)
	}

	select {
	case <-ctx.Done():
		releaseCompatiblePreviewArtifact(entry)
		return false, nil, ctx.Err()
	case <-entry.done:
		entry.mu.Lock()
		state, generationErr := entry.state, entry.err
		entry.mu.Unlock()
		if state != compatiblePreviewArtifactReady {
			releaseCompatiblePreviewArtifact(entry)
			return false, nil, generationErr
		}
		return false, &compatiblePreviewArtifactLease{entry: entry}, nil
	}
}

func releaseCompatiblePreviewArtifact(entry *compatiblePreviewArtifact) {
	if entry == nil {
		return
	}
	entry.mu.Lock()
	if entry.waiters > 0 {
		entry.waiters--
	}
	entry.lastUsedAt = time.Now()
	if entry.state == compatiblePreviewArtifactGenerating && entry.waiters == 0 && entry.cancelTimer == nil {
		entry.cancelTimer = time.AfterFunc(compatiblePreviewGenerationHandoffGrace, func() {
			entry.mu.Lock()
			if entry.state == compatiblePreviewArtifactGenerating && entry.waiters == 0 {
				entry.state = compatiblePreviewArtifactCanceled
				entry.cancel()
			}
			entry.mu.Unlock()
		})
	}
	entry.mu.Unlock()
}

func firstNonNilError(errorsToCheck ...error) error {
	for _, err := range errorsToCheck {
		if err != nil {
			return err
		}
	}
	return nil
}
