package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AssetHandler struct {
	db *gorm.DB
}

type previewGeneration struct {
	done chan struct{}
	err  error
}

var previewCacheState = struct {
	sync.Mutex
	inFlight map[string]*previewGeneration
}{inFlight: map[string]*previewGeneration{}}

type AssetInventory struct {
	Unprocessed       []Asset       `json:"unprocessed"`
	Library           []Asset       `json:"library"`
	Converted         []Asset       `json:"converted"`
	Unverified        []Asset       `json:"unverified"`
	Archive           []Asset       `json:"archive"`
	UnprocessedGroups []AssetGroup  `json:"unprocessedGroups"`
	LibraryGroups     []AssetGroup  `json:"libraryGroups"`
	ConvertedGroups   []AssetGroup  `json:"convertedGroups"`
	UnverifiedGroups  []AssetGroup  `json:"unverifiedGroups"`
	ArchiveGroups     []AssetGroup  `json:"archiveGroups"`
	Reports           AssetReports  `json:"reports"`
	Sync              AssetSyncInfo `json:"sync"`
}

type AssetSyncInfo struct {
	LastSyncedAt time.Time `json:"lastSyncedAt"`
	TotalRecords int64     `json:"totalRecords"`
	MissingFiles int64     `json:"missingFiles"`
}

type AssetReports struct {
	UnprocessedFiles int   `json:"unprocessedFiles"`
	LibraryFiles     int   `json:"libraryFiles"`
	ConvertedFiles   int   `json:"convertedFiles"`
	UnverifiedFiles  int   `json:"unverifiedFiles"`
	ArchiveFiles     int   `json:"archiveFiles"`
	ArchiveBytes     int64 `json:"archiveBytes"`
	ExpiredArchive   int   `json:"expiredArchive"`
	MissingFiles     int   `json:"missingFiles"`
}

type AssetSyncResult struct {
	SyncedAt         time.Time `json:"syncedAt"`
	UnprocessedFiles int       `json:"unprocessedFiles"`
	LibraryFiles     int       `json:"libraryFiles"`
	ConvertedFiles   int       `json:"convertedFiles"`
	UnverifiedFiles  int       `json:"unverifiedFiles"`
	ArchiveFiles     int       `json:"archiveFiles"`
	ExpiredDeleted   int       `json:"expiredDeleted"`
}

type AssetReviewState struct {
	RequiresReview bool      `json:"requiresReview"`
	Reason         string    `json:"reason"`
	Source         string    `json:"source"`
	Tags           []string  `json:"tags"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type AssetMetadataState struct {
	Categories []string  `json:"categories"`
	Tags       []string  `json:"tags"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type AssetConversionOverrideState struct {
	TrackProfileKey     string                         `json:"trackProfileKey,omitempty"`
	KeepVideoStreams    []int                          `json:"keepVideoStreams"`
	KeepAudioStreams    []int                          `json:"keepAudioStreams"`
	KeepSubtitleStreams []int                          `json:"keepSubtitleStreams"`
	VideoMetadata       map[int]StreamMetadataOverride `json:"videoMetadata,omitempty"`
	AudioMetadata       map[int]StreamMetadataOverride `json:"audioMetadata,omitempty"`
	SubtitleMetadata    map[int]StreamMetadataOverride `json:"subtitleMetadata,omitempty"`
	VideoCodec          string                         `json:"videoCodec,omitempty"`
	AudioCodec          string                         `json:"audioCodec,omitempty"`
	QualityMode         string                         `json:"qualityMode,omitempty"`
	QualityValue        int                            `json:"qualityValue,omitempty"`
	VideoPreset         string                         `json:"videoPreset,omitempty"`
	PixFmt              string                         `json:"pixFmt,omitempty"`
	VideoFilters        string                         `json:"videoFilters,omitempty"`
	DeinterlaceMode     string                         `json:"deinterlaceMode,omitempty"`
	X265Params          string                         `json:"x265Params,omitempty"`
	ProcessingMode      string                         `json:"processingMode,omitempty"`
	PreserveHDR         *bool                          `json:"preserveHdr,omitempty"`
	PreserveSubtitles   *bool                          `json:"preserveSubtitles,omitempty"`
	PreserveChapters    *bool                          `json:"preserveChapters,omitempty"`
	AddAACStereoTrack   *bool                          `json:"addAacStereoTrack,omitempty"`
	AACStereoDefault    *bool                          `json:"aacStereoDefault,omitempty"`
	UpdatedAt           *time.Time                     `json:"updatedAt,omitempty"`
}

type StreamMetadataOverride struct {
	Title    string `json:"title,omitempty"`
	Language string `json:"language,omitempty"`
	Default  *bool  `json:"default,omitempty"`
	Forced   *bool  `json:"forced,omitempty"`
}

type Asset struct {
	LibraryID    uint                         `json:"libraryId"`
	LibraryName  string                       `json:"libraryName"`
	Path         string                       `json:"path"`
	RelativePath string                       `json:"relativePath"`
	GroupPath    string                       `json:"groupPath"`
	FileName     string                       `json:"fileName"`
	Extension    string                       `json:"extension"`
	SizeBytes    int64                        `json:"sizeBytes"`
	ModifiedAt   time.Time                    `json:"modifiedAt"`
	Status       string                       `json:"status"`
	Missing      bool                         `json:"missing"`
	ExpiresAt    *time.Time                   `json:"expiresAt,omitempty"`
	Review       AssetReviewState             `json:"review"`
	Metadata     AssetMetadataState           `json:"metadata"`
	Conversion   AssetConversionOverrideState `json:"conversion"`
}

type AssetGroup struct {
	ID           string             `json:"id"`
	LibraryID    uint               `json:"libraryId"`
	LibraryName  string             `json:"libraryName"`
	Path         string             `json:"path"`
	RelativePath string             `json:"relativePath"`
	Status       string             `json:"status"`
	FileCount    int                `json:"fileCount"`
	SizeBytes    int64              `json:"sizeBytes"`
	ModifiedAt   time.Time          `json:"modifiedAt"`
	Assets       []Asset            `json:"assets"`
	Review       AssetReviewState   `json:"review"`
	PathReview   AssetReviewState   `json:"pathReview"`
	Metadata     AssetMetadataState `json:"metadata"`
	PathMetadata AssetMetadataState `json:"pathMetadata"`
}

type AssetReviewUpdateInput struct {
	RequiresReview bool     `json:"requiresReview"`
	Reason         string   `json:"reason"`
	Source         string   `json:"source"`
	Tags           []string `json:"tags"`
}

type AssetMetadataUpdateInput struct {
	Categories []string `json:"categories"`
	Tags       []string `json:"tags"`
}

type AssetConversionUpdateInput struct {
	TrackProfileKey     string                         `json:"trackProfileKey"`
	KeepVideoStreams    []int                          `json:"keepVideoStreams"`
	KeepAudioStreams    []int                          `json:"keepAudioStreams"`
	KeepSubtitleStreams []int                          `json:"keepSubtitleStreams"`
	VideoMetadata       map[int]StreamMetadataOverride `json:"videoMetadata"`
	AudioMetadata       map[int]StreamMetadataOverride `json:"audioMetadata"`
	SubtitleMetadata    map[int]StreamMetadataOverride `json:"subtitleMetadata"`
	VideoCodec          string                         `json:"videoCodec"`
	AudioCodec          string                         `json:"audioCodec"`
	QualityMode         string                         `json:"qualityMode"`
	QualityValue        int                            `json:"qualityValue"`
	VideoPreset         string                         `json:"videoPreset"`
	PixFmt              string                         `json:"pixFmt"`
	VideoFilters        string                         `json:"videoFilters"`
	DeinterlaceMode     string                         `json:"deinterlaceMode"`
	X265Params          string                         `json:"x265Params"`
	ProcessingMode      string                         `json:"processingMode"`
	PreserveHDR         *bool                          `json:"preserveHdr"`
	PreserveSubtitles   *bool                          `json:"preserveSubtitles"`
	PreserveChapters    *bool                          `json:"preserveChapters"`
	AddAACStereoTrack   *bool                          `json:"addAacStereoTrack"`
	AACStereoDefault    *bool                          `json:"aacStereoDefault"`
}

func NewAssetHandler(db *gorm.DB) AssetHandler {
	return AssetHandler{db: db}
}

func (h AssetHandler) List(c *gin.Context) {
	inventory, err := h.assetInventoryFromDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, inventory)
}

func (h AssetHandler) Sync(c *gin.Context) {
	result, err := h.syncAssetInventory()
	if err != nil {
		appendSystemLog(h.db, "asset_inventory_sync_failed", nil, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	appendSystemLog(h.db, "asset_inventory_synced", map[string]string{
		"unprocessed": strconv.Itoa(result.UnprocessedFiles),
		"converted":   strconv.Itoa(result.ConvertedFiles),
		"archive":     strconv.Itoa(result.ArchiveFiles),
	}, nil)
	c.JSON(http.StatusOK, result)
}

func (h AssetHandler) Recover(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	var record models.AssetRecord
	if err := h.db.First(&record, "path = ?", filepath.Clean(path)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "archive asset not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if record.Status != "archive" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only archive assets can be recovered"})
		return
	}
	if record.Missing {
		c.JSON(http.StatusNotFound, gin.H{"error": "archive file is no longer physically available"})
		return
	}
	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	destination := filepath.Join(rawRoot, filepath.FromSlash(record.RelativePath))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err := os.Stat(destination); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "raw asset already exists; converted files were not changed"})
		return
	}
	if err := moveFile(record.Path, destination); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	record.Missing = true
	record.SyncedAt = now
	if err := h.db.Save(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rawRecord := record
	rawRecord.ID = 0
	rawRecord.Path = filepath.Clean(destination)
	rawRecord.RootPath = filepath.Clean(rawRoot)
	rawRecord.Status = "unprocessed"
	rawRecord.LibraryID = 0
	rawRecord.LibraryName = "Originals"
	rawRecord.Missing = false
	rawRecord.ExpiresAt = nil
	rawRecord.SyncedAt = now
	if err := upsertAssetRecord(h.db, rawRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":        "recovered",
		"sourcePath":    path,
		"recoveredPath": destination,
		"message":       "Archive asset recovered. Converted files were not deleted.",
	})
}

func (h AssetHandler) DeleteConverted(c *gin.Context) {
	path := filepath.Clean(strings.TrimSpace(c.Query("path")))
	if path == "." || path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	var record models.AssetRecord
	if err := h.db.First(&record, "path = ?", path).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "converted asset not found in inventory"})
		return
	}
	if record.Status != "converted" {
		c.JSON(http.StatusConflict, gin.H{"error": "only a MVForge converted asset can use this recovery-safe delete flow"})
		return
	}
	convertedExists := false
	if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
		convertedExists = true
	} else if statErr != nil && !os.IsNotExist(statErr) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(statErr)})
		return
	}

	var job models.QueueJob
	if err := h.db.Where("published_path = ?", path).Order("published_at desc, id desc").First(&job).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "no publishing job links this converted asset to an archived original"})
		return
	}
	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	archiveRoot, err := originalsArchivePath(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	archivePath := archivedOriginalForJob(job, rawRoot, archiveRoot)
	if archivePath == "" || !pathIsInside(archivePath, archiveRoot) {
		c.JSON(http.StatusConflict, gin.H{"error": "the archived original could not be resolved safely"})
		return
	}
	if info, statErr := os.Stat(archivePath); statErr != nil || info.IsDir() {
		c.JSON(http.StatusConflict, gin.H{"error": "the archived original is missing; converted asset was not deleted"})
		return
	}
	restorePath := filepath.Clean(job.MediaPath)
	if !pathIsInside(restorePath, rawRoot) {
		c.JSON(http.StatusConflict, gin.H{"error": "the original Raw destination is outside the configured Raw root"})
		return
	}
	if _, statErr := os.Stat(restorePath); statErr == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "an asset already exists at the original Raw path; converted asset was not deleted"})
		return
	} else if !os.IsNotExist(statErr) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(statErr)})
		return
	}

	retiredAt := time.Now()
	job.PublicationRetiredAt = &retiredAt
	job.Notes = appendNote(job.Notes, "Safe deletion retired this publication before restoring its archived original")
	if err := h.db.Save(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not lock the publication against automatic reconciliation: " + err.Error()})
		return
	}
	quarantine := ""
	if convertedExists {
		quarantine = fmt.Sprintf("%s.mvforge-delete-%d", path, time.Now().UnixNano())
		if err := os.Rename(path, quarantine); err != nil {
			job.PublicationRetiredAt = nil
			_ = h.db.Save(&job).Error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not prepare converted asset for safe deletion: " + err.Error()})
			return
		}
	}
	if err := moveFile(archivePath, restorePath); err != nil {
		if quarantine != "" {
			_ = os.Rename(quarantine, path)
		}
		job.PublicationRetiredAt = nil
		_ = h.db.Save(&job).Error
		c.JSON(http.StatusInternalServerError, gin.H{"error": "original restoration failed; converted asset was restored: " + err.Error()})
		return
	}
	if quarantine != "" {
		if err := os.Remove(quarantine); err != nil {
			rollbackArchive := moveFile(restorePath, archivePath)
			rollbackConverted := os.Rename(quarantine, path)
			job.PublicationRetiredAt = nil
			_ = h.db.Save(&job).Error
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("converted deletion failed; rollback archive=%v converted=%v: %v", rollbackArchive, rollbackConverted, err)})
			return
		}
	}
	libraryRoot := filepath.Clean(strings.TrimSpace(record.RootPath))
	if libraryRoot != "." && libraryRoot != "" && pathIsInside(filepath.Dir(path), libraryRoot) {
		if err := (PublisherHandler{db: h.db}).cleanupEmptyOriginalDirs(filepath.Dir(path), libraryRoot); err != nil {
			appendSystemLog(h.db, "converted_asset_empty_library_path_cleanup_failed", map[string]string{"convertedPath": path, "libraryRoot": libraryRoot}, err)
		}
	}

	job.Notes = appendNote(job.Notes, "Converted asset deleted after archived original was restored to Raw: "+restorePath)
	_ = h.db.Save(&job).Error
	if err := h.db.Delete(&record).Error; err != nil {
		appendSystemLog(h.db, "converted_asset_inventory_record_delete_failed", map[string]string{"convertedPath": path}, err)
	}
	if _, syncErr := h.syncAssetInventory(); syncErr != nil {
		appendSystemLog(h.db, "converted_asset_deleted_inventory_sync_failed", map[string]string{"convertedPath": path, "restoredPath": restorePath}, syncErr)
	}
	appendSystemLog(h.db, "converted_asset_deleted_original_restored", map[string]string{"convertedPath": path, "archivePath": archivePath, "restoredPath": restorePath, "jobId": strconv.FormatUint(uint64(job.ID), 10)}, nil)
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "convertedPath": path, "archivedOriginalPath": archivePath, "restoredPath": restorePath, "jobId": job.ID, "message": "Converted asset deleted and original restored to Raw. Reports, logs, and job history were preserved."})
}

func archivedOriginalForJob(job models.QueueJob, rawRoot string, archiveRoot string) string {
	if value := strings.TrimSpace(job.OriginalArchivedPath); value != "" {
		return filepath.Clean(value)
	}
	for _, line := range strings.Split(job.Notes, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Original archived:") {
			continue
		}
		if value := strings.TrimSpace(strings.TrimPrefix(line, "Original archived:")); value != "" {
			return filepath.Clean(value)
		}
	}
	if pathIsInside(job.MediaPath, rawRoot) {
		relative, err := filepath.Rel(rawRoot, job.MediaPath)
		if err == nil {
			return filepath.Join(archiveRoot, relative)
		}
	}
	return ""
}

func (h AssetHandler) UpdateConversion(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		appendSystemLog(h.db, "asset_conversion_override_save_failed", map[string]string{"path": path, "stage": "resolve_path"}, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allowed, err := h.pathBelongsToLibrary(resolvedPath)
	if err != nil {
		appendSystemLog(h.db, "asset_conversion_override_save_failed", map[string]string{"path": path, "resolvedPath": resolvedPath, "stage": "check_library"}, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		err := fmt.Errorf("media path is outside configured libraries")
		appendSystemLog(h.db, "asset_conversion_override_save_failed", map[string]string{"path": path, "resolvedPath": resolvedPath, "stage": "check_library"}, err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	var input AssetConversionUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		appendSystemLog(h.db, "asset_conversion_override_save_failed", map[string]string{"path": path, "resolvedPath": resolvedPath, "stage": "bind_json"}, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries := assetConversionOverrides(h.db)
	cleanPath := filepath.Clean(resolvedPath)
	override := AssetConversionOverrideState{
		TrackProfileKey:     strings.TrimSpace(input.TrackProfileKey),
		KeepVideoStreams:    normalizedStreamIndexes(input.KeepVideoStreams),
		KeepAudioStreams:    normalizedStreamIndexes(input.KeepAudioStreams),
		KeepSubtitleStreams: normalizedStreamIndexes(input.KeepSubtitleStreams),
		VideoMetadata:       normalizedStreamMetadata(input.VideoMetadata),
		AudioMetadata:       normalizedStreamMetadata(input.AudioMetadata),
		SubtitleMetadata:    normalizedStreamMetadata(input.SubtitleMetadata),
		VideoCodec:          strings.TrimSpace(input.VideoCodec),
		AudioCodec:          strings.TrimSpace(input.AudioCodec),
		QualityMode:         strings.TrimSpace(input.QualityMode),
		QualityValue:        input.QualityValue,
		VideoPreset:         strings.TrimSpace(input.VideoPreset),
		PixFmt:              strings.TrimSpace(input.PixFmt),
		VideoFilters:        strings.TrimSpace(input.VideoFilters),
		DeinterlaceMode:     strings.TrimSpace(input.DeinterlaceMode),
		X265Params:          strings.TrimSpace(input.X265Params),
		ProcessingMode:      strings.TrimSpace(input.ProcessingMode),
		PreserveHDR:         input.PreserveHDR,
		PreserveSubtitles:   input.PreserveSubtitles,
		PreserveChapters:    input.PreserveChapters,
		AddAACStereoTrack:   input.AddAACStereoTrack,
		AACStereoDefault:    input.AACStereoDefault,
	}
	if assetConversionOverrideEmpty(override) {
		delete(entries, cleanPath)
	} else {
		now := time.Now()
		override.UpdatedAt = &now
		entries[cleanPath] = override
	}

	if err := saveAssetConversionOverrides(h.db, entries); err != nil {
		appendSystemLog(h.db, "asset_conversion_override_save_failed", map[string]string{"path": path, "resolvedPath": resolvedPath, "stage": "save_setting"}, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	appendSystemLog(h.db, "asset_conversion_override_saved", map[string]string{"path": path, "resolvedPath": resolvedPath}, nil)
	c.JSON(http.StatusOK, gin.H{"path": cleanPath, "conversion": conversionOverrideForPath(cleanPath, entries)})
}

func (h AssetHandler) UpdateMetadata(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allowed, err := h.pathBelongsToLibrary(resolvedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	var input AssetMetadataUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	entries := assetMetadataOverrides(h.db)
	cleanPath := filepath.Clean(resolvedPath)
	categories := normalizedTags(input.Categories)
	tags := normalizedTags(input.Tags)
	if len(categories) == 0 && len(tags) == 0 {
		delete(entries, cleanPath)
	} else {
		entries[cleanPath] = AssetMetadataState{
			Categories: categories,
			Tags:       tags,
			UpdatedAt:  time.Now(),
		}
	}

	if err := saveAssetMetadataOverrides(h.db, entries); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": cleanPath, "metadata": metadataForPath(cleanPath, entries)})
}

func (h AssetHandler) UpdateReview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	allowed, err := h.pathBelongsToLibrary(resolvedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	var input AssetReviewUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reviews := assetReviewOverrides(h.db)
	cleanPath := filepath.Clean(resolvedPath)
	source := strings.TrimSpace(input.Source)
	manualApproval := !input.RequiresReview && source == "manual"
	if !manualApproval && !input.RequiresReview && strings.TrimSpace(input.Reason) == "" && len(input.Tags) == 0 {
		delete(reviews, cleanPath)
	} else {
		if source == "" {
			source = "manual"
		}
		reviews[cleanPath] = AssetReviewState{
			RequiresReview: input.RequiresReview,
			Reason:         strings.TrimSpace(input.Reason),
			Source:         source,
			Tags:           normalizedTags(input.Tags),
			UpdatedAt:      time.Now(),
		}
	}

	if err := saveAssetReviewOverrides(h.db, reviews); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": cleanPath, "review": reviews[cleanPath]})
}

func (h AssetHandler) Preview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	path = resolvedPath

	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a supported media file"})
		return
	}

	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	file, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media path is not readable from the backend container"})
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media path must point to a file"})
		return
	}

	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Disposition", `inline; filename="`+filepath.Base(path)+`"`)
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), info.ModTime(), file)
}

func (h AssetHandler) CompatiblePreview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	profileID := strings.TrimSpace(c.Query("profileId"))
	videoCodecOverride := strings.TrimSpace(c.Query("videoCodec"))
	qualityValueOverride := strings.TrimSpace(c.Query("qualityValue"))
	videoPresetOverride := strings.TrimSpace(c.Query("videoPreset"))
	pixFmtOverride := strings.TrimSpace(c.Query("pixFmt"))
	videoFiltersOverride := strings.TrimSpace(c.Query("videoFilters"))
	x265ParamsOverride := strings.TrimSpace(c.Query("x265Params"))
	videoEncoderOverride := strings.TrimSpace(c.Query("videoEncoder"))
	useHardwareOverride, _ := strconv.ParseBool(c.Query("useHardwareIfAvailable"))
	globalQualityOverride, _ := strconv.Atoi(c.Query("globalQuality"))
	previewMode := normalizedPreviewMode(c.Query("mode"))
	start, ok := boundedPreviewStart(c.Query("start"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must be HH:MM:SS or seconds"})
		return
	}
	seconds := boundedPreviewSeconds(c.Query("seconds"))
	if previewMode == "quick" && seconds > 8 {
		seconds = 8
	}
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	path = resolvedPath

	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a supported media file"})
		return
	}

	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	if info, err := os.Stat(path); err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "media path is not readable from the backend container"})
		return
	}

	analyzeSize := "100M"
	if previewMode == "quick" {
		analyzeSize = "10M"
		videoPresetOverride = "veryfast"
		x265ParamsOverride = ""
	}
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-analyzeduration", analyzeSize,
		"-probesize", analyzeSize,
		"-ss", start,
		"-t", strconv.Itoa(seconds),
		"-i", path,
		"-map", "0:v:0?",
		"-vf", previewVideoFilterChain(videoFiltersOverride, previewMode),
	}
	args = append(args, previewVideoCodecArgs(h.db, profileID, videoCodecOverride, qualityValueOverride, videoPresetOverride, pixFmtOverride, x265ParamsOverride, videoEncoderOverride, useHardwareOverride, globalQualityOverride)...)
	args = append(args,
		"-an",
		"-sn",
		"-dn",
		"-map_metadata", "-1",
		"-avoid_negative_ts", "make_zero",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)

	info, _ := os.Stat(path)
	cacheKey := previewCacheKey(path, info, args)
	cachePath := filepath.Join(os.TempDir(), "mvforge-preview-cache", cacheKey+".mp4")
	started := time.Now()
	cacheHit, err := generateCachedPreview(c.Request.Context(), cacheKey, cachePath, args)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Disposition", `inline; filename="`+strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+`-preview.mp4"`)
	c.Header("Cache-Control", "private, max-age=0, must-revalidate")
	c.Header("ETag", `"`+cacheKey+`"`)
	c.Header("X-MVForge-Preview-Mode", previewMode)
	c.Header("X-MVForge-Preview-Cache", map[bool]string{true: "hit", false: "miss"}[cacheHit])
	c.Header("X-MVForge-Preview-Generation-Ms", strconv.FormatInt(time.Since(started).Milliseconds(), 10))
	c.File(cachePath)
}

func normalizedPreviewMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "quality") {
		return "quality"
	}
	return "quick"
}

func previewCacheKey(path string, info os.FileInfo, args []string) string {
	identity := path + "\x00" + strings.Join(args, "\x00")
	if info != nil {
		identity += fmt.Sprintf("\x00%d\x00%d", info.Size(), info.ModTime().UnixNano())
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

func generateCachedPreview(ctx context.Context, key string, destination string, args []string) (bool, error) {
	if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
		return true, nil
	}

	previewCacheState.Lock()
	if active := previewCacheState.inFlight[key]; active != nil {
		previewCacheState.Unlock()
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-active.done:
			return true, active.err
		}
	}
	active := &previewGeneration{done: make(chan struct{})}
	previewCacheState.inFlight[key] = active
	previewCacheState.Unlock()

	defer func() {
		previewCacheState.Lock()
		delete(previewCacheState.inFlight, key)
		close(active.done)
		previewCacheState.Unlock()
	}()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		active.err = err
		return false, err
	}
	cleanupPreviewCache(filepath.Dir(destination), 24*time.Hour)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		active.err = fmt.Errorf("%s", message)
		return false, active.err
	}
	temporary := destination + ".partial"
	if err := os.WriteFile(temporary, output, 0o644); err != nil {
		active.err = err
		return false, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		active.err = err
		return false, err
	}
	return false, nil
}

func cleanupPreviewCache(directory string, maxAge time.Duration) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr == nil && !entry.IsDir() && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
}

func previewVideoCodecArgs(db *gorm.DB, profileID string, videoCodecOverride string, qualityValueOverride string, videoPresetOverride string, pixFmtOverride string, x265ParamsOverride string, hardwareOverrides ...any) []string {
	videoCodec := strings.TrimSpace(videoCodecOverride)
	qualityValue := 24
	videoPreset := strings.TrimSpace(videoPresetOverride)
	pixFmt := strings.TrimSpace(pixFmtOverride)
	x265Params := strings.TrimSpace(x265ParamsOverride)

	if parsedQuality, err := strconv.Atoi(strings.TrimSpace(qualityValueOverride)); err == nil && parsedQuality > 0 {
		qualityValue = parsedQuality
	}

	if videoCodec == "" && profileID != "" {
		id, err := strconv.ParseUint(profileID, 10, 64)
		if err == nil {
			var profile models.Profile
			if db.First(&profile, uint(id)).Error == nil {
				videoCodec = profile.VideoCodec
				if profile.QualityValue > 0 {
					qualityValue = profile.QualityValue
				}
				if profile.WorkerConfig != nil {
					if videoPreset == "" {
						videoPreset = workerStringValue(profile.WorkerConfig["videoPreset"])
					}
					if pixFmt == "" {
						pixFmt = workerStringValue(profile.WorkerConfig["pixFmt"])
					}
					if x265Params == "" {
						x265Params = workerStringValue(profile.WorkerConfig["x265Params"])
					}
				}
			}
		}
	}

	if videoCodec == "" || videoCodec == "copy" {
		videoCodec = "x264"
	}

	videoEncoder := "auto"
	useHardware := false
	globalQuality := qualityValue + 5
	if len(hardwareOverrides) > 0 {
		videoEncoder, _ = hardwareOverrides[0].(string)
	}
	if len(hardwareOverrides) > 1 {
		useHardware, _ = hardwareOverrides[1].(bool)
	}
	if len(hardwareOverrides) > 2 {
		if value, ok := hardwareOverrides[2].(int); ok && value > 0 {
			globalQuality = value
		}
	}
	if videoPreset == "" {
		videoPreset = "veryfast"
	}
	if pixFmt == "" {
		pixFmt = "yuv420p"
		if isTenBitVideoCodec(videoCodec) {
			pixFmt = "yuv420p10le"
		}
	}
	profile := models.Profile{
		VideoCodec: videoCodec, QualityMode: "crf", QualityValue: qualityValue,
		BitDepth: map[bool]int{true: 10, false: 8}[strings.Contains(pixFmt, "10")],
		WorkerConfig: models.JSONMap{
			"videoEncoder": videoEncoder, "useHardwareIfAvailable": useHardware,
			"globalQuality": globalQuality, "videoPreset": videoPreset,
			"pixFmt": pixFmt, "x265Params": x265Params,
		},
	}
	args := videoCodecArgs(profile)
	return append(args, videoWorkerArgs(profile)...)
}

func previewVideoFilterChain(videoFilters string, previewMode ...string) string {
	qualityMode := len(previewMode) > 0 && previewMode[0] == "quality"
	videoFilters = strings.TrimSpace(videoFilters)
	if qualityMode {
		if videoFilters == "" {
			return "null"
		}
		return videoFilters
	}
	scale := "scale='min(854,iw)':-2"
	if videoFilters == "" {
		return scale
	}
	return videoFilters + "," + scale
}

func (h AssetHandler) AudioPreview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	profileKey := strings.TrimSpace(c.Query("profileKey"))
	filterOverride := strings.TrimSpace(c.Query("filters"))
	compatibilityPreview, _ := strconv.ParseBool(c.Query("compatibility"))
	start, ok := boundedPreviewStart(c.Query("start"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must be HH:MM:SS or seconds"})
		return
	}
	seconds := boundedPreviewSeconds(c.Query("seconds"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	path = resolvedPath

	if !isMediaExtension(strings.ToLower(filepath.Ext(path))) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a supported media file"})
		return
	}

	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}

	if info, err := os.Stat(path); err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "media path is not readable from the backend container"})
		return
	}

	filterChain := "anull"
	if filterOverride != "" {
		filterChain = filterOverride
	} else if profileKey != "" {
		audioProfile, err := lookupAudioProfile(h.db, profileKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if audioProfile != nil {
			filterChain = effectiveAudioFilters(*audioProfile)
		}
	}
	filterChain = sanitizeAudioFilterChain(filterChain)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-ss", start,
		"-t", strconv.Itoa(seconds),
		"-i", path,
		"-map", "0:a:0?",
		"-vn",
		"-af", filterChain,
	}
	contentType := "audio/wav"
	extension := "wav"
	if compatibilityPreview {
		args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
		contentType = "audio/mp4"
		extension = "m4a"
	} else {
		args = append(args, "-c:a", "pcm_s16le", "-f", "wav", "pipe:1")
	}
	cmd := exec.CommandContext(c.Request.Context(), "ffmpeg", args...)

	stderr := bytes.Buffer{}
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", `inline; filename="`+strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+`-audio-preview.`+extension+`"`)
	c.Data(http.StatusOK, contentType, output)
}

var afftdnNoiseFloorPattern = regexp.MustCompile(`afftdn=([^,]*\bnf=)(-?\d+(?:\.\d+)?)`)

func sanitizeAudioFilterChain(filterChain string) string {
	filters := []string{}
	for _, rawFilter := range strings.Split(filterChain, ",") {
		filter := strings.TrimSpace(rawFilter)
		if filter == "" {
			continue
		}
		if arnndnModelMissing(filter) {
			continue
		}
		filters = append(filters, sanitizeAfftdnFilter(filter))
	}
	if len(filters) == 0 {
		return "anull"
	}
	return strings.Join(filters, ",")
}

func sanitizeAfftdnFilter(filter string) string {
	return afftdnNoiseFloorPattern.ReplaceAllStringFunc(filter, func(match string) string {
		parts := afftdnNoiseFloorPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		value, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return match
		}
		if value > -20 {
			value = -20
		}
		if value < -80 {
			value = -80
		}
		return "afftdn=" + parts[1] + trimFloat(value)
	})
}

func arnndnModelMissing(filter string) bool {
	if !strings.HasPrefix(filter, "arnndn=") {
		return false
	}
	modelPath := ""
	for _, part := range strings.Split(strings.TrimPrefix(filter, "arnndn="), ":") {
		if strings.HasPrefix(part, "m=") {
			modelPath = strings.TrimPrefix(part, "m=")
			break
		}
	}
	if strings.TrimSpace(modelPath) == "" {
		return false
	}
	if info, err := os.Stat(modelPath); err == nil && !info.IsDir() {
		return false
	}
	return true
}

func trimFloat(value float64) string {
	if value == float64(int(value)) {
		return strconv.Itoa(int(value))
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func boundedPreviewSeconds(value string) int {
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 20
	}
	if seconds < 5 {
		return 5
	}
	if seconds > 120 {
		return 120
	}
	return seconds
}

func boundedPreviewStart(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "00:00:00", true
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return "", false
		}
		if seconds > 24*60*60 {
			seconds = 24 * 60 * 60
		}
		return secondsToTimestamp(seconds), true
	}

	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return "", false
	}

	hours, errHours := strconv.Atoi(parts[0])
	minutes, errMinutes := strconv.Atoi(parts[1])
	seconds, errSeconds := strconv.Atoi(parts[2])
	if errHours != nil || errMinutes != nil || errSeconds != nil {
		return "", false
	}
	if hours < 0 || minutes < 0 || minutes > 59 || seconds < 0 || seconds > 59 {
		return "", false
	}
	if hours > 24 {
		hours = 24
		minutes = 0
		seconds = 0
	}

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds), true
}

func secondsToTimestamp(totalSeconds int) string {
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func (h AssetHandler) resolveMediaPath(mediaPath string) (string, error) {
	mediaPath = strings.TrimSpace(mediaPath)
	if filepath.IsAbs(mediaPath) {
		return filepath.Clean(mediaPath), nil
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(rawRoot, mediaPath)), nil
}

func (h AssetHandler) pathBelongsToLibrary(mediaPath string) (bool, error) {
	mediaAbs, err := filepath.Abs(mediaPath)
	if err != nil {
		return false, err
	}

	var libraries []models.Library
	if err := h.db.Find(&libraries).Error; err != nil {
		return false, err
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		return false, err
	}

	allowedRoots := []string{rawRoot}
	for _, library := range libraries {
		allowedRoots = append(allowedRoots, library.DestinationPath)
		if strings.TrimSpace(library.SourcePath) != "" {
			allowedRoots = append(allowedRoots, library.SourcePath)
		}
	}

	for _, root := range allowedRoots {
		if strings.TrimSpace(root) == "" {
			continue
		}

		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}

		relative, err := filepath.Rel(rootAbs, mediaAbs)
		if err != nil {
			continue
		}

		if relative == "." || (!strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && relative != "..") {
			return true, nil
		}
	}

	return false, nil
}

func settingPath(db *gorm.DB, key string, fallback string) (string, error) {
	paths := models.JSONMap{}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "paths").Error; err == nil && setting.Value != nil {
		paths = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	return stringSetting(paths, key, fallback), nil
}

func (h AssetHandler) assetInventoryFromDB() (AssetInventory, error) {
	records := []models.AssetRecord{}
	if err := h.db.Order("status asc, library_name asc, relative_path asc").Find(&records).Error; err != nil {
		return AssetInventory{}, err
	}

	inventory := AssetInventory{
		Unprocessed:       []Asset{},
		Library:           []Asset{},
		Converted:         []Asset{},
		Unverified:        []Asset{},
		Archive:           []Asset{},
		UnprocessedGroups: []AssetGroup{},
		LibraryGroups:     []AssetGroup{},
		ConvertedGroups:   []AssetGroup{},
		UnverifiedGroups:  []AssetGroup{},
		ArchiveGroups:     []AssetGroup{},
	}
	mvForgeOutputs := mvForgeOutputPaths(h.db)
	for _, record := range records {
		asset := assetFromRecord(record)
		switch record.Status {
		case "converted":
			if mvForgeOutputs[filepath.Clean(record.Path)] {
				asset.Status = "converted"
				inventory.Converted = append(inventory.Converted, asset)
			} else {
				asset.Status = "unverified"
				inventory.Unverified = append(inventory.Unverified, asset)
			}
			inventory.Library = append(inventory.Library, asset)
		case "archive":
			inventory.Archive = append(inventory.Archive, asset)
		default:
			if !record.Missing {
				inventory.Unprocessed = append(inventory.Unprocessed, asset)
			}
		}
	}

	reviews := assetReviewOverrides(h.db)
	metadata := assetMetadataOverrides(h.db)
	conversion := assetConversionOverrides(h.db)
	applyAssetReviews(inventory.Unprocessed, reviews)
	applyAssetReviews(inventory.Library, reviews)
	applyAssetReviews(inventory.Converted, reviews)
	applyAssetReviews(inventory.Unverified, reviews)
	applyAssetReviews(inventory.Archive, reviews)
	applyAssetMetadata(inventory.Unprocessed, metadata)
	applyAssetMetadata(inventory.Library, metadata)
	applyAssetMetadata(inventory.Converted, metadata)
	applyAssetMetadata(inventory.Unverified, metadata)
	applyAssetMetadata(inventory.Archive, metadata)
	applyAssetConversionOverrides(inventory.Unprocessed, conversion)
	applyAssetConversionOverrides(inventory.Library, conversion)
	applyAssetConversionOverrides(inventory.Converted, conversion)
	applyAssetConversionOverrides(inventory.Unverified, conversion)
	applyAssetConversionOverrides(inventory.Archive, conversion)
	sortAssets(inventory.Unprocessed)
	sortAssets(inventory.Library)
	sortAssets(inventory.Converted)
	sortAssets(inventory.Unverified)
	sortAssets(inventory.Archive)
	inventory.UnprocessedGroups = groupAssets(inventory.Unprocessed, reviews, metadata)
	inventory.LibraryGroups = groupAssets(inventory.Library, reviews, metadata)
	inventory.ConvertedGroups = groupAssets(inventory.Converted, reviews, metadata)
	inventory.UnverifiedGroups = groupAssets(inventory.Unverified, reviews, metadata)
	inventory.ArchiveGroups = groupAssets(inventory.Archive, reviews, metadata)
	sortAssetGroups(inventory.UnprocessedGroups)
	sortAssetGroups(inventory.LibraryGroups)
	sortAssetGroups(inventory.ConvertedGroups)
	sortAssetGroups(inventory.UnverifiedGroups)
	sortAssetGroups(inventory.ArchiveGroups)
	inventory.Reports = assetReports(inventory)

	var last models.AssetRecord
	if err := h.db.Order("synced_at desc").First(&last).Error; err == nil {
		inventory.Sync.LastSyncedAt = last.SyncedAt
	}
	_ = h.db.Model(&models.AssetRecord{}).Count(&inventory.Sync.TotalRecords).Error
	_ = h.db.Model(&models.AssetRecord{}).Where("missing = ?", true).Count(&inventory.Sync.MissingFiles).Error
	return inventory, nil
}

func mvForgeOutputPaths(db *gorm.DB) map[string]bool {
	paths := map[string]bool{}
	var jobs []models.QueueJob
	_ = db.Where("publication_retired_at IS NULL AND (published_path <> '' OR status = ?)", JobStatusCompleted).Find(&jobs).Error
	for _, job := range jobs {
		if value := strings.TrimSpace(job.PublishedPath); value != "" {
			paths[filepath.Clean(value)] = true
		}
		if job.Status == JobStatusCompleted {
			if value := strings.TrimSpace(job.OutputPath); value != "" {
				paths[filepath.Clean(value)] = true
			}
		}
	}
	return paths
}

func (h AssetHandler) syncAssetInventory() (AssetSyncResult, error) {
	now := time.Now()
	result := AssetSyncResult{SyncedAt: now}

	var libraries []models.Library
	if err := h.db.Order("name asc").Find(&libraries).Error; err != nil {
		return result, err
	}
	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		return result, err
	}
	archiveRoot, err := originalsArchivePath(h.db)
	if err != nil {
		return result, err
	}
	keepDays := originalRetentionDays(h.db)
	expireArchive := assetInventoryExpireArchiveFiles(h.db)

	foundPaths := map[string]struct{}{}
	rawRecords := collectAssetRecords(models.Library{Name: "Originals", SourcePath: rawRoot}, rawRoot, "unprocessed", now, keepDays)
	for _, record := range rawRecords {
		foundPaths[record.Path] = struct{}{}
		if err := upsertAssetRecord(h.db, record); err != nil {
			return result, err
		}
		result.UnprocessedFiles++
	}

	seenDestinations := map[string]struct{}{}
	mvForgeOutputs := mvForgeOutputPaths(h.db)
	for _, library := range libraries {
		destinationPath := strings.TrimSpace(library.DestinationPath)
		if destinationPath == "" {
			continue
		}
		if _, seen := seenDestinations[destinationPath]; seen {
			continue
		}
		seenDestinations[destinationPath] = struct{}{}
		for _, record := range collectAssetRecords(library, destinationPath, "converted", now, keepDays) {
			foundPaths[record.Path] = struct{}{}
			if err := upsertAssetRecord(h.db, record); err != nil {
				return result, err
			}
			result.LibraryFiles++
			if mvForgeOutputs[filepath.Clean(record.Path)] {
				result.ConvertedFiles++
			} else {
				result.UnverifiedFiles++
			}
		}
	}

	for _, record := range collectAssetRecords(models.Library{Name: "Original Archive"}, archiveRoot, "archive", now, keepDays) {
		foundPaths[record.Path] = struct{}{}
		if expireArchive && record.ExpiresAt != nil && now.After(*record.ExpiresAt) {
			if err := os.Remove(record.Path); err == nil {
				record.Missing = true
				result.ExpiredDeleted++
			}
		}
		if err := upsertAssetRecord(h.db, record); err != nil {
			return result, err
		}
		result.ArchiveFiles++
	}

	var records []models.AssetRecord
	if err := h.db.Find(&records).Error; err != nil {
		return result, err
	}
	for _, record := range records {
		if _, ok := foundPaths[record.Path]; ok {
			continue
		}
		if !record.Missing {
			record.Missing = true
			record.SyncedAt = now
			if err := h.db.Save(&record).Error; err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

func collectAssetRecords(library models.Library, root string, status string, syncedAt time.Time, keepDays int) []models.AssetRecord {
	if strings.TrimSpace(root) == "" {
		return []models.AssetRecord{}
	}

	records := []models.AssetRecord{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if !isMediaExtension(extension) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		relativePath := filepath.ToSlash(relativeAssetPath(root, path))
		record := models.AssetRecord{
			Path:         filepath.Clean(path),
			RootPath:     filepath.Clean(root),
			RelativePath: relativePath,
			GroupPath:    filepath.ToSlash(logicalAssetGroupPath(relativePath)),
			FileName:     filepath.Base(path),
			Extension:    extension,
			SizeBytes:    info.Size(),
			ModifiedAt:   info.ModTime(),
			Status:       status,
			LibraryID:    library.ID,
			LibraryName:  library.Name,
			Missing:      false,
			SyncedAt:     syncedAt,
		}
		if status == "archive" && keepDays > 0 {
			expiresAt := info.ModTime().Add(time.Duration(keepDays) * 24 * time.Hour)
			record.ExpiresAt = &expiresAt
		}
		records = append(records, record)
		return nil
	})
	return records
}

func upsertAssetRecord(db *gorm.DB, record models.AssetRecord) error {
	var existing models.AssetRecord
	if err := db.First(&existing, "path = ?", record.Path).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(&record).Error
		}
		return err
	}
	record.ID = existing.ID
	record.CreatedAt = existing.CreatedAt
	return db.Save(&record).Error
}

func assetFromRecord(record models.AssetRecord) Asset {
	return Asset{
		LibraryID:    record.LibraryID,
		LibraryName:  record.LibraryName,
		Path:         record.Path,
		RelativePath: record.RelativePath,
		GroupPath:    record.GroupPath,
		FileName:     record.FileName,
		Extension:    record.Extension,
		SizeBytes:    record.SizeBytes,
		ModifiedAt:   record.ModifiedAt,
		Status:       record.Status,
		Missing:      record.Missing,
		ExpiresAt:    record.ExpiresAt,
	}
}

func assetReports(inventory AssetInventory) AssetReports {
	report := AssetReports{
		UnprocessedFiles: len(inventory.Unprocessed),
		LibraryFiles:     len(inventory.Library),
		ConvertedFiles:   len(inventory.Converted),
		UnverifiedFiles:  len(inventory.Unverified),
		ArchiveFiles:     len(inventory.Archive),
	}
	now := time.Now()
	for _, asset := range append(append([]Asset{}, inventory.Library...), inventory.Archive...) {
		if asset.Missing {
			report.MissingFiles++
		}
	}
	for _, asset := range inventory.Archive {
		report.ArchiveBytes += asset.SizeBytes
		if asset.ExpiresAt != nil && now.After(*asset.ExpiresAt) {
			report.ExpiredArchive++
		}
	}
	return report
}

func originalRetentionDays(db *gorm.DB) int {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "originalRetentionPolicy").Error; err != nil || setting.Value == nil {
		return 30
	}
	return intValueSetting(setting.Value["keepOriginalsDays"], 30)
}

func assetInventoryExpireArchiveFiles(db *gorm.DB) bool {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetInventory").Error; err != nil || setting.Value == nil {
		return true
	}
	return boolSetting(setting.Value["expireArchiveFiles"], true)
}

func collectAssets(library models.Library, root string, status string) []Asset {
	if strings.TrimSpace(root) == "" {
		return []Asset{}
	}

	assets := []Asset{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}

		extension := strings.ToLower(filepath.Ext(path))
		if !isMediaExtension(extension) {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}

		relativePath := relativeAssetPath(root, path)
		groupPath := logicalAssetGroupPath(relativePath)

		assets = append(assets, Asset{
			LibraryID:    library.ID,
			LibraryName:  library.Name,
			Path:         path,
			RelativePath: relativePath,
			GroupPath:    groupPath,
			FileName:     filepath.Base(path),
			Extension:    extension,
			SizeBytes:    info.Size(),
			ModifiedAt:   info.ModTime(),
			Status:       status,
		})

		return nil
	})

	return assets
}

func groupAssets(assets []Asset, reviews map[string]AssetReviewState, metadata map[string]AssetMetadataState) []AssetGroup {
	groupsByKey := map[string]*AssetGroup{}
	order := []string{}

	for _, asset := range assets {
		key := assetGroupKey(asset)
		group, exists := groupsByKey[key]
		if !exists {
			groupPath := strings.Trim(asset.GroupPath, string(os.PathSeparator))
			absolutePath := absoluteAssetGroupPath(asset)
			group = &AssetGroup{
				ID:           key,
				LibraryID:    asset.LibraryID,
				LibraryName:  asset.LibraryName,
				Path:         absolutePath,
				RelativePath: groupPath,
				Status:       asset.Status,
				Assets:       []Asset{},
				PathReview:   reviewForPath(absolutePath, reviews),
				PathMetadata: metadataForPath(absolutePath, metadata),
			}
			group.Review = group.PathReview
			group.Metadata = group.PathMetadata
			groupsByKey[key] = group
			order = append(order, key)
		}

		group.FileCount++
		group.SizeBytes += asset.SizeBytes
		if asset.ModifiedAt.After(group.ModifiedAt) {
			group.ModifiedAt = asset.ModifiedAt
		}
		group.Review = mergeAssetReviewState(group.Review, asset.Review)
		group.Metadata = mergeAssetMetadataState(group.Metadata, asset.Metadata)
		group.Assets = append(group.Assets, asset)
	}

	groups := make([]AssetGroup, 0, len(order))
	for _, key := range order {
		group := groupsByKey[key]
		sortAssets(group.Assets)
		groups = append(groups, *group)
	}

	return groups
}

func assetMetadataOverrides(db *gorm.DB) map[string]AssetMetadataState {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetMetadataOverrides").Error; err != nil || setting.Value == nil {
		return map[string]AssetMetadataState{}
	}

	rawEntries, ok := setting.Value["entries"].(map[string]interface{})
	if !ok {
		return map[string]AssetMetadataState{}
	}

	entries := map[string]AssetMetadataState{}
	for rawPath, rawValue := range rawEntries {
		entry, ok := rawValue.(map[string]interface{})
		if !ok {
			continue
		}
		metadata := AssetMetadataState{
			Categories: stringSliceFromUnknown(entry["categories"]),
			Tags:       stringSliceFromUnknown(entry["tags"]),
		}
		if updatedAt, err := time.Parse(time.RFC3339, stringFromUnknown(entry["updatedAt"])); err == nil {
			metadata.UpdatedAt = updatedAt
		}
		entries[filepath.Clean(rawPath)] = metadata
	}
	return entries
}

func saveAssetMetadataOverrides(db *gorm.DB, metadata map[string]AssetMetadataState) error {
	entries := map[string]interface{}{}
	for path, item := range metadata {
		entry := map[string]interface{}{
			"categories": item.Categories,
			"tags":       item.Tags,
			"updatedAt":  item.UpdatedAt.Format(time.RFC3339),
		}
		entries[filepath.Clean(path)] = entry
	}

	setting := models.AppSetting{}
	value := models.JSONMap{"entries": entries}
	if err := db.First(&setting, "key = ?", "assetMetadataOverrides").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(&models.AppSetting{Key: "assetMetadataOverrides", Value: value}).Error
		}
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func assetConversionOverrides(db *gorm.DB) map[string]AssetConversionOverrideState {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetConversionOverrides").Error; err != nil || setting.Value == nil {
		return map[string]AssetConversionOverrideState{}
	}

	rawEntries, ok := setting.Value["entries"].(map[string]interface{})
	if !ok {
		return map[string]AssetConversionOverrideState{}
	}

	entries := map[string]AssetConversionOverrideState{}
	for rawPath, rawValue := range rawEntries {
		bytes, err := json.Marshal(rawValue)
		if err != nil {
			continue
		}
		var override AssetConversionOverrideState
		if err := json.Unmarshal(bytes, &override); err != nil {
			continue
		}
		entries[filepath.Clean(rawPath)] = override
	}
	return entries
}

func saveAssetConversionOverrides(db *gorm.DB, overrides map[string]AssetConversionOverrideState) error {
	entries := map[string]interface{}{}
	for path, override := range overrides {
		if assetConversionOverrideEmpty(override) {
			continue
		}
		entries[filepath.Clean(path)] = override
	}

	setting := models.AppSetting{}
	value := models.JSONMap{"entries": entries}
	if err := db.First(&setting, "key = ?", "assetConversionOverrides").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(&models.AppSetting{Key: "assetConversionOverrides", Value: value}).Error
		}
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func assetReviewOverrides(db *gorm.DB) map[string]AssetReviewState {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetReviewOverrides").Error; err != nil || setting.Value == nil {
		return map[string]AssetReviewState{}
	}

	rawEntries, ok := setting.Value["entries"].(map[string]interface{})
	if !ok {
		return map[string]AssetReviewState{}
	}

	reviews := map[string]AssetReviewState{}
	for rawPath, rawValue := range rawEntries {
		entry, ok := rawValue.(map[string]interface{})
		if !ok {
			continue
		}
		review := AssetReviewState{
			RequiresReview: boolSetting(entry["requiresReview"], false),
			Reason:         stringFromUnknown(entry["reason"]),
			Source:         stringFromUnknown(entry["source"]),
			Tags:           stringSliceFromUnknown(entry["tags"]),
		}
		if updatedAt, err := time.Parse(time.RFC3339, stringFromUnknown(entry["updatedAt"])); err == nil {
			review.UpdatedAt = updatedAt
		}
		reviews[filepath.Clean(rawPath)] = review
	}
	return reviews
}

func saveAssetReviewOverrides(db *gorm.DB, reviews map[string]AssetReviewState) error {
	entries := map[string]interface{}{}
	for path, review := range reviews {
		entry := map[string]interface{}{
			"requiresReview": review.RequiresReview,
			"reason":         review.Reason,
			"source":         review.Source,
			"tags":           review.Tags,
			"updatedAt":      review.UpdatedAt.Format(time.RFC3339),
		}
		entries[filepath.Clean(path)] = entry
	}

	setting := models.AppSetting{}
	value := models.JSONMap{"entries": entries}
	if err := db.First(&setting, "key = ?", "assetReviewOverrides").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.Create(&models.AppSetting{Key: "assetReviewOverrides", Value: value}).Error
		}
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func applyAssetReviews(assets []Asset, reviews map[string]AssetReviewState) {
	for index := range assets {
		assets[index].Review = reviewForPath(assets[index].Path, reviews)
	}
}

func applyAssetMetadata(assets []Asset, metadata map[string]AssetMetadataState) {
	for index := range assets {
		assets[index].Metadata = metadataForPath(assets[index].Path, metadata)
	}
}

func applyAssetConversionOverrides(assets []Asset, overrides map[string]AssetConversionOverrideState) {
	for index := range assets {
		assets[index].Conversion = conversionOverrideForPath(assets[index].Path, overrides)
	}
}

func metadataForPath(path string, metadata map[string]AssetMetadataState) AssetMetadataState {
	cleanPath := filepath.Clean(path)
	item, ok := metadata[cleanPath]
	if !ok {
		return AssetMetadataState{Categories: []string{}, Tags: []string{}}
	}
	if item.Categories == nil {
		item.Categories = []string{}
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	return item
}

func conversionOverrideForPath(path string, overrides map[string]AssetConversionOverrideState) AssetConversionOverrideState {
	cleanPath := filepath.Clean(path)
	override, ok := overrides[cleanPath]
	if !ok {
		return AssetConversionOverrideState{}
	}
	override.KeepVideoStreams = normalizedStreamIndexes(override.KeepVideoStreams)
	override.KeepAudioStreams = normalizedStreamIndexes(override.KeepAudioStreams)
	override.KeepSubtitleStreams = normalizedStreamIndexes(override.KeepSubtitleStreams)
	override.VideoMetadata = normalizedStreamMetadata(override.VideoMetadata)
	override.AudioMetadata = normalizedStreamMetadata(override.AudioMetadata)
	override.SubtitleMetadata = normalizedStreamMetadata(override.SubtitleMetadata)
	return override
}

func assetConversionOverrideEmpty(override AssetConversionOverrideState) bool {
	return strings.TrimSpace(override.TrackProfileKey) == "" &&
		override.KeepVideoStreams == nil &&
		override.KeepAudioStreams == nil &&
		override.KeepSubtitleStreams == nil &&
		len(override.VideoMetadata) == 0 &&
		len(override.AudioMetadata) == 0 &&
		len(override.SubtitleMetadata) == 0 &&
		strings.TrimSpace(override.VideoCodec) == "" &&
		strings.TrimSpace(override.AudioCodec) == "" &&
		strings.TrimSpace(override.QualityMode) == "" &&
		override.QualityValue == 0 &&
		strings.TrimSpace(override.VideoPreset) == "" &&
		strings.TrimSpace(override.PixFmt) == "" &&
		strings.TrimSpace(override.VideoFilters) == "" &&
		strings.TrimSpace(override.DeinterlaceMode) == "" &&
		strings.TrimSpace(override.X265Params) == "" &&
		strings.TrimSpace(override.ProcessingMode) == "" &&
		override.PreserveHDR == nil &&
		override.PreserveSubtitles == nil &&
		override.PreserveChapters == nil &&
		override.AddAACStereoTrack == nil &&
		override.AACStereoDefault == nil
}

func normalizedStreamMetadata(metadata map[int]StreamMetadataOverride) map[int]StreamMetadataOverride {
	normalized := map[int]StreamMetadataOverride{}
	for index, item := range metadata {
		if index < 0 {
			continue
		}
		clean := StreamMetadataOverride{
			Title:    strings.TrimSpace(item.Title),
			Language: strings.ToLower(strings.TrimSpace(item.Language)),
			Default:  item.Default,
			Forced:   item.Forced,
		}
		if clean.Title == "" && clean.Language == "" && clean.Default == nil && clean.Forced == nil {
			continue
		}
		normalized[index] = clean
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizedStreamIndexes(indexes []int) []int {
	if indexes == nil {
		return nil
	}
	seen := map[int]struct{}{}
	normalized := []int{}
	for _, index := range indexes {
		if index < 0 {
			continue
		}
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		normalized = append(normalized, index)
	}
	sort.Ints(normalized)
	return normalized
}

func reviewForPath(path string, reviews map[string]AssetReviewState) AssetReviewState {
	cleanPath := filepath.Clean(path)
	review, ok := reviews[cleanPath]
	if !ok {
		return AssetReviewState{Tags: []string{}}
	}
	if review.Tags == nil {
		review.Tags = []string{}
	}
	return review
}

func mergeAssetReviewState(current AssetReviewState, next AssetReviewState) AssetReviewState {
	if !next.RequiresReview && len(next.Tags) == 0 && next.Reason == "" {
		return current
	}
	if !current.RequiresReview && len(current.Tags) == 0 && current.Reason == "" {
		return next
	}
	merged := current
	merged.RequiresReview = current.RequiresReview || next.RequiresReview
	if merged.Reason == "" {
		merged.Reason = next.Reason
	}
	if merged.Source == "" {
		merged.Source = next.Source
	}
	if next.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = next.UpdatedAt
	}
	seenTags := map[string]struct{}{}
	tags := []string{}
	for _, tag := range append(current.Tags, next.Tags...) {
		cleanTag := strings.TrimSpace(tag)
		if cleanTag == "" {
			continue
		}
		if _, seen := seenTags[cleanTag]; seen {
			continue
		}
		seenTags[cleanTag] = struct{}{}
		tags = append(tags, cleanTag)
	}
	merged.Tags = tags
	return merged
}

func mergeAssetMetadataState(current AssetMetadataState, next AssetMetadataState) AssetMetadataState {
	if len(next.Categories) == 0 && len(next.Tags) == 0 {
		return current
	}
	merged := current
	if next.UpdatedAt.After(merged.UpdatedAt) {
		merged.UpdatedAt = next.UpdatedAt
	}
	merged.Categories = mergeStringLists(current.Categories, next.Categories)
	merged.Tags = mergeStringLists(current.Tags, next.Tags)
	return merged
}

func mergeStringLists(left []string, right []string) []string {
	seen := map[string]struct{}{}
	values := []string{}
	for _, value := range append(left, right...) {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, clean)
	}
	return values
}

func normalizedTags(tags []string) []string {
	seen := map[string]struct{}{}
	normalized := []string{}
	for _, tag := range tags {
		cleanTag := strings.TrimSpace(tag)
		if cleanTag == "" {
			continue
		}
		if _, ok := seen[cleanTag]; ok {
			continue
		}
		seen[cleanTag] = struct{}{}
		normalized = append(normalized, cleanTag)
	}
	return normalized
}

func boolSetting(value interface{}, fallback bool) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return fallback
}

func intValueSetting(value interface{}, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := strconv.Atoi(string(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func stringFromUnknown(value interface{}) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func stringSliceFromUnknown(value interface{}) []string {
	rawValues, ok := value.([]interface{})
	if !ok {
		return []string{}
	}
	values := []string{}
	for _, rawValue := range rawValues {
		if value, ok := rawValue.(string); ok {
			values = append(values, value)
		}
	}
	return normalizedTags(values)
}

func assetGroupKey(asset Asset) string {
	return fmt.Sprintf("%s:%d:%s", strings.TrimSpace(asset.Status), asset.LibraryID, strings.TrimSpace(asset.GroupPath))
}

func logicalAssetGroupPath(relativePath string) string {
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || cleanPath == string(os.PathSeparator) {
		return ""
	}
	parts := strings.Split(cleanPath, string(os.PathSeparator))
	if len(parts) <= 1 {
		return ""
	}
	if len(parts) == 2 {
		return parts[0]
	}
	return filepath.Join(parts[0], parts[1])
}

func absoluteAssetGroupPath(asset Asset) string {
	groupPath := strings.TrimSpace(asset.GroupPath)
	if groupPath == "" {
		return filepath.Dir(asset.Path)
	}

	relativePath := filepath.Clean(asset.RelativePath)
	assetPath := filepath.Clean(asset.Path)
	if relativePath == "." || !strings.HasSuffix(assetPath, relativePath) {
		return filepath.Dir(asset.Path)
	}

	root := strings.TrimSuffix(assetPath, relativePath)
	root = strings.TrimRight(root, string(os.PathSeparator))
	if root == "" {
		return filepath.Dir(asset.Path)
	}
	return filepath.Join(root, groupPath)
}

func relativeAssetPath(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return relative
}

func assetKeySet(assets []Asset) map[string]struct{} {
	keys := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		keys[assetKey(asset.RelativePath)] = struct{}{}
	}
	return keys
}

func assetKey(fileName string) string {
	return strings.ToLower(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
}

func isMediaExtension(extension string) bool {
	switch extension {
	case ".avi", ".flac", ".m4v", ".mka", ".mkv", ".mov", ".mp3", ".mp4", ".mpeg", ".mpg", ".ogg", ".opus", ".ts", ".wav", ".webm":
		return true
	default:
		return false
	}
}

func sortAssets(assets []Asset) {
	sort.SliceStable(assets, func(i, j int) bool {
		if assets[i].LibraryName == assets[j].LibraryName {
			return assets[i].RelativePath < assets[j].RelativePath
		}
		return assets[i].LibraryName < assets[j].LibraryName
	})
}

func sortAssetGroups(groups []AssetGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].LibraryName == groups[j].LibraryName {
			return groups[i].RelativePath < groups[j].RelativePath
		}
		return groups[i].LibraryName < groups[j].LibraryName
	})
}
