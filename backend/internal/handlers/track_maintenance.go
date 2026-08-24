package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	assetMutationMu         sync.Mutex
	activeSynchronousScanMu sync.Mutex
	activeSynchronousScans  = map[string]int{}
)

const (
	maintenanceStatusQueued   = "queued"
	maintenanceStatusRunning  = "running"
	maintenanceStatusComplete = "completed"
	maintenanceStatusFailed   = "failed"
)

type TrackMaintenanceStream struct {
	Index       int               `json:"index"`
	Type        string            `json:"type"`
	Codec       string            `json:"codec"`
	Language    string            `json:"language,omitempty"`
	Title       string            `json:"title,omitempty"`
	FileName    string            `json:"fileName,omitempty"`
	Profile     string            `json:"profile,omitempty"`
	Width       int               `json:"width,omitempty"`
	Height      int               `json:"height,omitempty"`
	Channels    int               `json:"channels,omitempty"`
	Layout      string            `json:"layout,omitempty"`
	Default     bool              `json:"default"`
	Forced      bool              `json:"forced"`
	AttachedPic bool              `json:"attachedPic"`
	StillImage  bool              `json:"stillImage"`
	Tags        map[string]string `json:"-"`
}

type TrackMaintenanceInventory struct {
	Path                      string                   `json:"path"`
	Fingerprint               string                   `json:"fingerprint"`
	Streams                   []TrackMaintenanceStream `json:"streams"`
	Chapters                  int                      `json:"chapters"`
	MaintenanceAllowed        bool                     `json:"maintenanceAllowed"`
	MaintenanceDisabledReason string                   `json:"maintenanceDisabledReason,omitempty"`
}

type trackMaintenanceRuntime struct {
	probeInventory func(context.Context, string) (TrackMaintenanceInventory, error)
	remux          func(context.Context, []string) error
	analyze        func(string) (models.ScanResult, error)
	stat           func(string) (os.FileInfo, error)
	rename         func(string, string) error
	remove         func(string) error
	fingerprint    func(string) (string, error)
	persist        func(AssetHandler, string, string, string, models.ScanResult, time.Time) error
}

func defaultTrackMaintenanceRuntime() *trackMaintenanceRuntime {
	return &trackMaintenanceRuntime{
		probeInventory: probeTrackMaintenanceInventory,
		remux: func(ctx context.Context, args []string) error {
			cmd := exec.CommandContext(ctx, "ffmpeg", args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("ffmpeg remux failed: %s", strings.TrimSpace(stderr.String()))
			}
			return nil
		},
		analyze: func(path string) (models.ScanResult, error) {
			info, err := os.Stat(path)
			if err != nil {
				return models.ScanResult{}, err
			}
			probe, raw, err := runFFProbe(path, 20)
			if err != nil {
				return models.ScanResult{}, err
			}
			return buildScanResult(path, info.Size(), probe, raw), nil
		},
		stat:        os.Stat,
		rename:      os.Rename,
		remove:      os.Remove,
		fingerprint: mediaFileFingerprint,
		persist:     persistTrackMaintenanceResult,
	}
}

func (h AssetHandler) maintenanceRuntime() *trackMaintenanceRuntime {
	if h.trackMaintenanceRuntime != nil {
		return h.trackMaintenanceRuntime
	}
	return defaultTrackMaintenanceRuntime()
}

type trackProbeResponse struct {
	Streams []struct {
		Index         int               `json:"index"`
		CodecType     string            `json:"codec_type"`
		CodecName     string            `json:"codec_name"`
		Profile       string            `json:"profile"`
		Width         int               `json:"width"`
		Height        int               `json:"height"`
		Channels      int               `json:"channels"`
		ChannelLayout string            `json:"channel_layout"`
		Tags          map[string]string `json:"tags"`
		Disposition   map[string]int    `json:"disposition"`
	} `json:"streams"`
	Chapters []json.RawMessage `json:"chapters"`
}

func probeTrackMaintenanceInventory(ctx context.Context, path string) (TrackMaintenanceInventory, error) {
	args := []string{"-v", "error", "-print_format", "json", "-show_streams", "-show_chapters", path}
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var output bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &stderr
	if err := cmd.Run(); err != nil {
		return TrackMaintenanceInventory{}, fmt.Errorf("probe track inventory: %s", strings.TrimSpace(stderr.String()))
	}
	var response trackProbeResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return TrackMaintenanceInventory{}, fmt.Errorf("parse track inventory: %w", err)
	}
	fingerprint, err := mediaFileFingerprint(path)
	if err != nil {
		return TrackMaintenanceInventory{}, fmt.Errorf("fingerprint asset: %w", err)
	}
	inventory := TrackMaintenanceInventory{Path: filepath.Clean(path), Fingerprint: fingerprint, Chapters: len(response.Chapters)}
	for _, stream := range response.Streams {
		inventory.Streams = append(inventory.Streams, TrackMaintenanceStream{
			Index: stream.Index, Type: stream.CodecType, Codec: stream.CodecName, Profile: stream.Profile,
			Language: stream.Tags["language"], Title: stream.Tags["title"], FileName: stream.Tags["filename"],
			Width: stream.Width, Height: stream.Height, Channels: stream.Channels, Layout: stream.ChannelLayout,
			Default: stream.Disposition["default"] == 1, Forced: stream.Disposition["forced"] == 1,
			AttachedPic: stream.Disposition["attached_pic"] == 1, StillImage: stream.Disposition["still_image"] == 1,
			Tags: stream.Tags,
		})
	}
	return inventory, nil
}

func validateTrackRemoval(inventory TrackMaintenanceInventory, requested []int) ([]TrackMaintenanceStream, error) {
	if len(requested) == 0 {
		return nil, fmt.Errorf("select at least one track to remove")
	}
	remove := map[int]bool{}
	for _, index := range requested {
		if remove[index] {
			return nil, fmt.Errorf("stream index %d was selected more than once", index)
		}
		remove[index] = true
	}
	known := map[int]TrackMaintenanceStream{}
	for _, stream := range inventory.Streams {
		known[stream.Index] = stream
	}
	for index := range remove {
		stream, ok := known[index]
		if !ok {
			return nil, fmt.Errorf("stream index %d no longer exists", index)
		}
		if stream.Type == "data" || (stream.Type != "video" && stream.Type != "audio" && stream.Type != "subtitle" && stream.Type != "attachment") {
			return nil, fmt.Errorf("stream index %d has unsupported maintenance type %q", index, stream.Type)
		}
	}
	remaining := make([]TrackMaintenanceStream, 0, len(inventory.Streams)-len(remove))
	playableVideo := 0
	for _, stream := range inventory.Streams {
		if remove[stream.Index] {
			continue
		}
		remaining = append(remaining, stream)
		if stream.Type == "video" && !stream.AttachedPic && !stream.StillImage {
			playableVideo++
		}
	}
	hadPlayableVideo := false
	for _, stream := range inventory.Streams {
		hadPlayableVideo = hadPlayableVideo || (stream.Type == "video" && !stream.AttachedPic && !stream.StillImage)
	}
	if hadPlayableVideo && playableVideo == 0 {
		return nil, fmt.Errorf("at least one playable video stream must remain")
	}
	return remaining, nil
}

func buildRemoveTracksFFmpegArgs(input, output string, remaining []TrackMaintenanceStream) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-i", input}
	sort.SliceStable(remaining, func(i, j int) bool { return remaining[i].Index < remaining[j].Index })
	for _, stream := range remaining {
		args = append(args, "-map", fmt.Sprintf("0:%d", stream.Index))
	}
	return append(args, "-map_metadata", "0", "-map_chapters", "0", "-c", "copy", output)
}

func streamTypeOutputIndex(streams []TrackMaintenanceStream, target TrackMaintenanceStream) int {
	result := 0
	for _, stream := range streams {
		if stream.Index == target.Index {
			return result
		}
		if stream.Type == target.Type {
			result++
		}
	}
	return -1
}

func buildEditTrackFFmpegArgs(input, output string, inventory TrackMaintenanceInventory, edited TrackMaintenanceStream) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-i", input, "-map", "0", "-map_metadata", "0", "-map_chapters", "0", "-c", "copy"}
	typeIndex := streamTypeOutputIndex(inventory.Streams, edited)
	typeCode := map[string]string{"video": "v", "audio": "a", "subtitle": "s"}[edited.Type]
	specifier := fmt.Sprintf("%s:%d", typeCode, typeIndex)
	args = append(args,
		"-metadata:s:"+specifier, "title="+edited.Title,
		"-metadata:s:"+specifier, "language="+edited.Language,
	)
	if edited.Default && (edited.Type == "audio" || edited.Type == "subtitle") {
		for _, stream := range inventory.Streams {
			if stream.Type != edited.Type || stream.Index == edited.Index {
				continue
			}
			otherIndex := streamTypeOutputIndex(inventory.Streams, stream)
			otherDisposition := "0"
			if stream.Type == "subtitle" && stream.Forced {
				otherDisposition = "forced"
			}
			args = append(args, fmt.Sprintf("-disposition:%s:%d", typeCode, otherIndex), otherDisposition)
		}
	}
	disposition := "0"
	if edited.Default {
		disposition = "default"
	}
	if edited.Forced {
		if disposition == "0" {
			disposition = "forced"
		} else {
			disposition += "+forced"
		}
	}
	return append(args, "-disposition:"+specifier, disposition, output)
}

func buildAddAACTrackFFmpegArgs(input, output string, inventory TrackMaintenanceInventory, request addAACTrackInput) []string {
	audioIndex := 0
	for _, stream := range inventory.Streams {
		if stream.Type == "audio" {
			audioIndex++
		}
	}
	args := []string{"-hide_banner", "-nostdin", "-y", "-i", input, "-map", "0", "-map", fmt.Sprintf("0:%d", request.SourceStreamIndex), "-map_metadata", "0", "-map_chapters", "0", "-c", "copy",
		fmt.Sprintf("-c:a:%d", audioIndex), "aac", fmt.Sprintf("-b:a:%d", audioIndex), fmt.Sprintf("%dk", request.BitrateKbps),
		fmt.Sprintf("-metadata:s:a:%d", audioIndex), "title=" + request.Title, fmt.Sprintf("-metadata:s:a:%d", audioIndex), "language=" + request.Language,
	}
	if request.Channels == "stereo" {
		args = append(args, fmt.Sprintf("-ac:a:%d", audioIndex), "2")
	}
	if request.Default {
		for index := 0; index < audioIndex; index++ {
			args = append(args, fmt.Sprintf("-disposition:a:%d", index), "0")
		}
		args = append(args, fmt.Sprintf("-disposition:a:%d", audioIndex), "default")
	} else {
		args = append(args, fmt.Sprintf("-disposition:a:%d", audioIndex), "0")
	}
	return append(args, output)
}

func trackStreamSignature(stream TrackMaintenanceStream) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d|%d|%d|%s|%t|%t|%t|%t",
		stream.Type, stream.Codec, stream.Profile, stream.Language, stream.Title, stream.FileName,
		stream.Width, stream.Height, stream.Channels, stream.Layout,
		stream.Default, stream.Forced, stream.AttachedPic, stream.StillImage)
}

func validateRemuxedTrackInventory(expected []TrackMaintenanceStream, actual TrackMaintenanceInventory) error {
	if len(expected) != len(actual.Streams) {
		return fmt.Errorf("remux stream count changed unexpectedly: got %d want %d", len(actual.Streams), len(expected))
	}
	for index := range expected {
		if trackStreamSignature(expected[index]) != trackStreamSignature(actual.Streams[index]) {
			return fmt.Errorf("remux stream %d does not match retained input stream %d", index, expected[index].Index)
		}
	}
	return nil
}

func activeAssetMaintenance(db *gorm.DB, path string) (bool, error) {
	if db == nil || !db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		return false, nil
	}
	var operations []models.AssetMaintenanceOperation
	if err := db.Where("status IN ?", []string{maintenanceStatusQueued, maintenanceStatusRunning}).Find(&operations).Error; err != nil {
		return false, err
	}
	requested, statErr := os.Stat(filepath.Clean(path))
	for _, operation := range operations {
		if filepath.Clean(operation.AssetPath) == filepath.Clean(path) {
			return true, nil
		}
		if statErr == nil {
			if candidate, err := os.Stat(operation.AssetPath); err == nil && os.SameFile(requested, candidate) {
				return true, nil
			}
		}
	}
	return false, nil
}

type removeTracksInput struct {
	Path                string `json:"path" binding:"required"`
	StreamIndexes       []int  `json:"streamIndexes" binding:"required"`
	ExpectedFingerprint string `json:"expectedFingerprint" binding:"required"`
	Confirmed           bool   `json:"confirmed"`
}

type editTrackInput struct {
	Path                string `json:"path" binding:"required"`
	StreamIndex         int    `json:"streamIndex"`
	ExpectedFingerprint string `json:"expectedFingerprint" binding:"required"`
	Title               string `json:"title"`
	Language            string `json:"language"`
	Default             bool   `json:"default"`
	Forced              bool   `json:"forced"`
}

type addAACTrackInput struct {
	Path                string `json:"path" binding:"required"`
	SourceStreamIndex   int    `json:"sourceStreamIndex"`
	ExpectedFingerprint string `json:"expectedFingerprint" binding:"required"`
	BitrateKbps         int    `json:"bitrateKbps"`
	Channels            string `json:"channels"`
	Title               string `json:"title"`
	Language            string `json:"language"`
	Default             bool   `json:"default"`
}

func (h AssetHandler) TrackMaintenanceInventory(c *gin.Context) {
	path, record, err := h.resolveTrackInventoryAsset(c.Query("path"))
	if err != nil {
		trackMaintenanceHTTPError(c, err)
		return
	}
	inventory, err := h.maintenanceRuntime().probeInventory(c.Request.Context(), path)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	inventory.MaintenanceAllowed, inventory.MaintenanceDisabledReason = trackMaintenanceAvailability(path, record)
	c.JSON(200, inventory)
}

func (h AssetHandler) StartTrackRemoval(c *gin.Context) {
	var input removeTracksInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if !input.Confirmed {
		c.JSON(400, gin.H{"error": "explicit confirmation is required because removed tracks cannot be recovered"})
		return
	}
	path, record, err := h.resolveTrackMaintenanceAsset(input.Path)
	if err != nil {
		trackMaintenanceHTTPError(c, err)
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	if synchronousScanActive(path) || snapshotOperationActive(path) {
		c.JSON(409, gin.H{"error": "asset has an active scan and cannot be modified"})
		return
	}
	if active, err := activeAssetMaintenance(h.db, path); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	} else if active {
		c.JSON(409, gin.H{"error": "asset already has an active maintenance operation"})
		return
	}
	if active, err := (QueueHandler{db: h.db}).assetHasOpenJob(path, 0); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	} else if active {
		c.JSON(409, gin.H{"error": "asset has an active Queue job and cannot be modified"})
		return
	}
	inventory, err := h.maintenanceRuntime().probeInventory(c.Request.Context(), path)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	if inventory.Fingerprint != strings.TrimSpace(input.ExpectedFingerprint) {
		c.JSON(409, gin.H{"error": "asset changed after the track inventory was loaded; review the current streams and try again"})
		return
	}
	remaining, err := validateTrackRemoval(inventory, input.StreamIndexes)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	operation := models.AssetMaintenanceOperation{
		ID: "maintenance-" + strconv.FormatInt(now.UnixNano(), 10), OperationType: "remove_tracks",
		AssetRecordID: record.ID, AssetPath: path, AssetStatus: record.Status,
		RequestedIndexes: intListToJSON(input.StreamIndexes), OriginalFingerprint: inventory.Fingerprint,
		Status: maintenanceStatusQueued, Phase: "queued", Progress: 0, CreatedAt: now, UpdatedAt: now,
	}
	if err := h.db.Create(&operation).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	go h.executeTrackRemoval(operation.ID, inventory, remaining)
	c.JSON(202, operation)
}

func (h AssetHandler) StartTrackEdit(c *gin.Context) {
	var input editTrackInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	path, record, inventory, ok := h.prepareTrackMutation(c, input.Path, input.ExpectedFingerprint)
	if !ok {
		return
	}
	editedIndex := -1
	for index := range inventory.Streams {
		if inventory.Streams[index].Index == input.StreamIndex {
			editedIndex = index
			break
		}
	}
	if editedIndex < 0 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("stream index %d no longer exists", input.StreamIndex)})
		return
	}
	edited := inventory.Streams[editedIndex]
	if edited.Type != "audio" && edited.Type != "subtitle" && edited.Type != "video" {
		c.JSON(400, gin.H{"error": "metadata editing is supported for video, audio, and subtitle tracks"})
		return
	}
	edited.Title = strings.TrimSpace(input.Title)
	edited.Language = strings.ToLower(strings.TrimSpace(input.Language))
	edited.Default = input.Default
	edited.Forced = input.Forced && edited.Type == "subtitle"
	expected := append([]TrackMaintenanceStream(nil), inventory.Streams...)
	if edited.Default && (edited.Type == "audio" || edited.Type == "subtitle") {
		for index := range expected {
			if expected[index].Type == edited.Type {
				expected[index].Default = false
			}
		}
	}
	expected[editedIndex] = edited
	operation := newTrackMaintenanceOperation("edit_track", record, path, inventory.Fingerprint, []int{input.StreamIndex})
	if err := h.db.Create(&operation).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	go h.executeTrackMutation(operation.ID, inventory, expected,
		func(output string) []string { return buildEditTrackFFmpegArgs(path, output, inventory, edited) },
		func(actual TrackMaintenanceInventory) error { return validateRemuxedTrackInventory(expected, actual) })
	c.JSON(202, operation)
}

func (h AssetHandler) StartAddAACTrack(c *gin.Context) {
	var input addAACTrackInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if input.BitrateKbps != 128 && input.BitrateKbps != 160 && input.BitrateKbps != 192 && input.BitrateKbps != 256 && input.BitrateKbps != 320 {
		c.JSON(400, gin.H{"error": "AAC bitrate must be 128, 160, 192, 256, or 320 kbps"})
		return
	}
	if input.Channels != "source" && input.Channels != "stereo" {
		c.JSON(400, gin.H{"error": "AAC channels must be source or stereo"})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	path, record, inventory, ok := h.prepareTrackMutation(c, input.Path, input.ExpectedFingerprint)
	if !ok {
		return
	}
	var source *TrackMaintenanceStream
	for index := range inventory.Streams {
		if inventory.Streams[index].Index == input.SourceStreamIndex && inventory.Streams[index].Type == "audio" {
			source = &inventory.Streams[index]
			break
		}
	}
	if source == nil {
		c.JSON(400, gin.H{"error": "selected AAC source audio track no longer exists"})
		return
	}
	input.Language = strings.ToLower(strings.TrimSpace(input.Language))
	if input.Language == "" {
		input.Language = source.Language
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = "AAC Stereo (MVForge)"
	}
	expected := append([]TrackMaintenanceStream(nil), inventory.Streams...)
	if input.Default {
		for index := range expected {
			if expected[index].Type == "audio" {
				expected[index].Default = false
			}
		}
	}
	derived := TrackMaintenanceStream{Type: "audio", Codec: "aac", Language: input.Language, Title: input.Title, Default: input.Default, Channels: source.Channels, Layout: source.Layout}
	if input.Channels == "stereo" {
		derived.Channels, derived.Layout = 2, "stereo"
	}
	expected = append(expected, derived)
	operation := newTrackMaintenanceOperation("add_aac_track", record, path, inventory.Fingerprint, []int{input.SourceStreamIndex})
	if err := h.db.Create(&operation).Error; err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	go h.executeTrackMutation(operation.ID, inventory, expected,
		func(output string) []string { return buildAddAACTrackFFmpegArgs(path, output, inventory, input) },
		func(actual TrackMaintenanceInventory) error { return validateAddedAACInventory(expected, actual) })
	c.JSON(202, operation)
}

func newTrackMaintenanceOperation(operationType string, record models.AssetRecord, path, fingerprint string, indexes []int) models.AssetMaintenanceOperation {
	now := time.Now()
	return models.AssetMaintenanceOperation{
		ID: "maintenance-" + strconv.FormatInt(now.UnixNano(), 10), OperationType: operationType,
		AssetRecordID: record.ID, AssetPath: path, AssetStatus: record.Status,
		RequestedIndexes: intListToJSON(indexes), OriginalFingerprint: fingerprint,
		Status: maintenanceStatusQueued, Phase: "queued", Progress: 0, CreatedAt: now, UpdatedAt: now,
	}
}

func (h AssetHandler) prepareTrackMutation(c *gin.Context, requestedPath, expectedFingerprint string) (string, models.AssetRecord, TrackMaintenanceInventory, bool) {
	path, record, err := h.resolveTrackMaintenanceAsset(requestedPath)
	if err != nil {
		trackMaintenanceHTTPError(c, err)
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	}
	if synchronousScanActive(path) || snapshotOperationActive(path) {
		c.JSON(409, gin.H{"error": "asset has an active scan and cannot be modified"})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	}
	if active, err := activeAssetMaintenance(h.db, path); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	} else if active {
		c.JSON(409, gin.H{"error": "asset already has an active maintenance operation"})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	}
	if active, err := (QueueHandler{db: h.db}).assetHasOpenJob(path, 0); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	} else if active {
		c.JSON(409, gin.H{"error": "asset has an active Queue job and cannot be modified"})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	}
	inventory, err := h.maintenanceRuntime().probeInventory(c.Request.Context(), path)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	}
	if inventory.Fingerprint != strings.TrimSpace(expectedFingerprint) {
		c.JSON(409, gin.H{"error": "asset changed after the track inventory was loaded; review the current streams and try again"})
		return "", models.AssetRecord{}, TrackMaintenanceInventory{}, false
	}
	return path, record, inventory, true
}

func synchronousScanActive(path string) bool {
	activeSynchronousScanMu.Lock()
	defer activeSynchronousScanMu.Unlock()
	return activeSynchronousScans[filepath.Clean(path)] > 0
}

func markSynchronousScan(path string, delta int) {
	activeSynchronousScanMu.Lock()
	defer activeSynchronousScanMu.Unlock()
	clean := filepath.Clean(path)
	activeSynchronousScans[clean] += delta
	if activeSynchronousScans[clean] <= 0 {
		delete(activeSynchronousScans, clean)
	}
}

func snapshotOperationActive(path string) bool {
	snapshotOperations.RLock()
	defer snapshotOperations.RUnlock()
	clean := filepath.Clean(path)
	requested, requestedErr := os.Stat(clean)
	for _, operation := range snapshotOperations.items {
		if operation.Status != "running" {
			continue
		}
		if filepath.Clean(operation.AssetPath) == clean {
			return true
		}
		if requestedErr == nil {
			if candidate, err := os.Stat(operation.AssetPath); err == nil && os.SameFile(requested, candidate) {
				return true
			}
		}
	}
	return false
}

func recoverInterruptedTrackMaintenance(db *gorm.DB) {
	if db == nil || !db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		return
	}
	var operations []models.AssetMaintenanceOperation
	if db.Where("status IN ?", []string{maintenanceStatusQueued, maintenanceStatusRunning}).Find(&operations).Error != nil {
		return
	}
	for _, operation := range operations {
		warning := "MVForge restarted before maintenance completed; the original asset remained in place."
		if backupInfo, err := os.Stat(operation.BackupPath); err == nil && !backupInfo.IsDir() {
			if currentInfo, currentErr := os.Stat(operation.AssetPath); currentErr == nil && !currentInfo.IsDir() {
				interruptedResult := operation.TemporaryPath + ".interrupted"
				if renameErr := os.Rename(operation.AssetPath, interruptedResult); renameErr == nil {
					if restoreErr := os.Rename(operation.BackupPath, operation.AssetPath); restoreErr == nil {
						warning = "MVForge restarted during replacement; the original was restored and the interrupted result was retained for inspection."
					} else {
						_ = os.Rename(interruptedResult, operation.AssetPath)
						warning = "Automatic rollback failed; inspect the recorded backup before modifying this asset."
					}
				}
			} else if os.IsNotExist(currentErr) {
				if os.Rename(operation.BackupPath, operation.AssetPath) == nil {
					warning = "MVForge restarted during replacement; the missing original path was restored from its backup."
				}
			}
		}
		_ = os.Remove(operation.TemporaryPath)
		now := time.Now()
		_ = db.Model(&models.AssetMaintenanceOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{
			"status": maintenanceStatusFailed, "phase": "interrupted", "progress": 0,
			"error_message": "maintenance interrupted by application restart", "warning": warning,
			"finished_at": now, "updated_at": now,
		}).Error
	}
}

func (h AssetHandler) GetMaintenanceOperation(c *gin.Context) {
	var operation models.AssetMaintenanceOperation
	if err := h.db.First(&operation, "id = ?", c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(404, gin.H{"error": "maintenance operation not found"})
		} else {
			c.JSON(500, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(200, operation)
}

func (h AssetHandler) resolveTrackMaintenanceAsset(value string) (string, models.AssetRecord, error) {
	path, record, err := h.resolveTrackInventoryAsset(value)
	if err != nil {
		return "", models.AssetRecord{}, err
	}
	allowed, reason := trackMaintenanceAvailability(path, record)
	if !allowed {
		return "", models.AssetRecord{}, fmt.Errorf("%s", reason)
	}
	return path, record, nil
}

func (h AssetHandler) resolveTrackInventoryAsset(value string) (string, models.AssetRecord, error) {
	path := filepath.Clean(strings.TrimSpace(value))
	if path == "." || !filepath.IsAbs(path) {
		return "", models.AssetRecord{}, fmt.Errorf("asset path must be absolute")
	}
	var record models.AssetRecord
	if err := h.db.Where("path = ? AND missing = ?", path, false).First(&record).Error; err != nil {
		return "", models.AssetRecord{}, err
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", models.AssetRecord{}, fmt.Errorf("asset is not physically available")
	}
	return path, record, nil
}

func trackMaintenanceAvailability(path string, record models.AssetRecord) (bool, string) {
	if record.LibraryID == 0 || record.Status == "archive" || record.Status == "unprocessed" || record.Status == "missing" {
		return false, "Original Raw and Archive assets are protected; track changes are applied during conversion."
	}
	if strings.ToLower(filepath.Ext(path)) != ".mkv" {
		return false, "Track editing currently supports active Library/Converted MKV assets only."
	}
	return true, ""
}

func trackMaintenanceHTTPError(c *gin.Context, err error) {
	if err == gorm.ErrRecordNotFound {
		c.JSON(404, gin.H{"error": "asset not found"})
		return
	}
	c.JSON(400, gin.H{"error": err.Error()})
}

func intListToJSON(values []int) models.JSONList {
	result := make(models.JSONList, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func (h AssetHandler) updateMaintenance(id, status, phase string, progress int, updates map[string]any) {
	values := map[string]any{"status": status, "phase": phase, "progress": progress, "updated_at": time.Now()}
	for key, value := range updates {
		values[key] = value
	}
	_ = h.db.Model(&models.AssetMaintenanceOperation{}).Where("id = ?", id).Updates(values).Error
}

func (h AssetHandler) executeTrackRemoval(id string, inventory TrackMaintenanceInventory, remaining []TrackMaintenanceStream) {
	h.executeTrackMutation(id, inventory, remaining,
		func(output string) []string { return buildRemoveTracksFFmpegArgs(inventory.Path, output, remaining) },
		func(actual TrackMaintenanceInventory) error { return validateRemuxedTrackInventory(remaining, actual) })
}

func (h AssetHandler) executeTrackMutation(id string, inventory TrackMaintenanceInventory, _ []TrackMaintenanceStream, buildArgs func(string) []string, validate func(TrackMaintenanceInventory) error) {
	runtime := h.maintenanceRuntime()
	path := inventory.Path
	extension := filepath.Ext(path)
	temporary := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+id+".tmp"+extension)
	backup := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+id+".backup"+extension)
	h.updateMaintenance(id, maintenanceStatusRunning, "remuxing", 10, map[string]any{"started_at": time.Now(), "temporary_path": temporary, "backup_path": backup})
	defer func() { _ = runtime.remove(temporary) }()
	args := buildArgs(temporary)
	if err := runtime.remux(context.Background(), args); err != nil {
		h.failMaintenance(id, "remuxing", err)
		return
	}
	h.updateMaintenance(id, maintenanceStatusRunning, "validating", 55, nil)
	actual, err := runtime.probeInventory(context.Background(), temporary)
	if err != nil || validate(actual) != nil || actual.Chapters != inventory.Chapters {
		if err == nil {
			err = validate(actual)
		}
		if err == nil && actual.Chapters != inventory.Chapters {
			err = fmt.Errorf("chapter count changed unexpectedly: got %d want %d", actual.Chapters, inventory.Chapters)
		}
		h.failMaintenance(id, "validating", err)
		return
	}
	info, err := runtime.stat(temporary)
	if err != nil || info.Size() <= 0 {
		h.failMaintenance(id, "validating", fmt.Errorf("remux output is empty or unavailable"))
		return
	}
	snapshot, err := runtime.analyze(temporary)
	if err != nil {
		h.failMaintenance(id, "analyzing", fmt.Errorf("analyze remux candidate: %w", err))
		return
	}
	snapshot.Path = path
	snapshot.FileName = filepath.Base(path)
	snapshot.SizeBytes = info.Size()
	applySnapshotDirectPlay(h.db, &snapshot)
	h.updateMaintenance(id, maintenanceStatusRunning, "committing", 85, nil)
	if err := runtime.rename(path, backup); err != nil {
		h.failMaintenance(id, "committing", fmt.Errorf("prepare recoverable backup: %w", err))
		return
	}
	if err := runtime.rename(temporary, path); err != nil {
		_ = runtime.rename(backup, path)
		h.failMaintenance(id, "committing", fmt.Errorf("replace asset: %w", err))
		return
	}
	rollback := func() { _ = runtime.remove(path); _ = runtime.rename(backup, path) }
	finalFingerprint, err := runtime.fingerprint(path)
	if err != nil {
		rollback()
		h.failMaintenance(id, "committing", fmt.Errorf("verify replaced asset: %w", err))
		return
	}
	finished := time.Now()
	if err := runtime.persist(h, id, path, finalFingerprint, snapshot, finished); err != nil {
		rollback()
		h.failMaintenance(id, "committing", fmt.Errorf("persist maintenance result: %w", err))
		return
	}
	if err := runtime.remove(backup); err != nil && !os.IsNotExist(err) {
		h.updateMaintenance(id, maintenanceStatusComplete, "completed", 100, map[string]any{"warning": "maintenance completed, but the temporary recovery backup could not be removed: " + err.Error()})
	}
}

func validateAddedAACInventory(expected []TrackMaintenanceStream, actual TrackMaintenanceInventory) error {
	if len(actual.Streams) != len(expected) {
		return fmt.Errorf("AAC remux stream count changed unexpectedly: got %d want %d", len(actual.Streams), len(expected))
	}
	last := len(expected) - 1
	for index := 0; index < last; index++ {
		if trackStreamSignature(expected[index]) != trackStreamSignature(actual.Streams[index]) {
			return fmt.Errorf("AAC remux changed original stream %d", index)
		}
	}
	want, got := expected[last], actual.Streams[last]
	if got.Type != "audio" || got.Codec != "aac" || got.Language != want.Language || got.Title != want.Title || got.Default != want.Default {
		return fmt.Errorf("derived AAC track metadata or codec does not match the request")
	}
	if want.Channels > 0 && got.Channels != want.Channels {
		return fmt.Errorf("derived AAC channel count=%d want=%d", got.Channels, want.Channels)
	}
	return nil
}

func persistTrackMaintenanceResult(h AssetHandler, id, path, finalFingerprint string, snapshot models.ScanResult, finished time.Time) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("path = ?", path).Delete(&models.ScanResult{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}
		finalInfo, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err := tx.Model(&models.AssetRecord{}).Where("id = ?", snapshotAssetRecordID(tx, path)).Updates(map[string]any{"size_bytes": finalInfo.Size(), "modified_at": finalInfo.ModTime(), "synced_at": finished}).Error; err != nil {
			return err
		}
		return tx.Model(&models.AssetMaintenanceOperation{}).Where("id = ?", id).Updates(map[string]any{
			"status": maintenanceStatusComplete, "phase": "completed", "progress": 100,
			"result_fingerprint": finalFingerprint, "finished_at": finished, "updated_at": finished,
		}).Error
	})
}

func snapshotAssetRecordID(db *gorm.DB, path string) uint {
	var record models.AssetRecord
	_ = db.Select("id").Where("path = ?", path).First(&record).Error
	return record.ID
}

func (h AssetHandler) failMaintenance(id, phase string, err error) {
	message := "maintenance failed"
	if err != nil {
		message = err.Error()
	}
	h.updateMaintenance(id, maintenanceStatusFailed, phase, 0, map[string]any{"error_message": message, "finished_at": time.Now()})
}
