package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AssetHandler struct {
	db *gorm.DB
}

type SubtitleExtractionResult struct {
	Created     []string `json:"created"`
	Existing    []string `json:"existing"`
	Unsupported []string `json:"unsupported"`
}

type SubtitleExtractionInput struct {
	StreamIndex *int   `json:"streamIndex"`
	Format      string `json:"format"`
	OCRLanguage string `json:"ocrLanguage,omitempty"`
}

type ExternalSubtitle struct {
	Path       string    `json:"path"`
	FileName   string    `json:"fileName"`
	Format     string    `json:"format"`
	Language   string    `json:"language,omitempty"`
	Default    bool      `json:"default"`
	Forced     bool      `json:"forced"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type ExternalSubtitleUpdateInput struct {
	SubtitlePath string `json:"subtitlePath" binding:"required"`
	Content      string `json:"content"`
}

type AssetPathMigrationInput struct {
	SourcePath           string `json:"sourcePath" binding:"required"`
	DestinationLibraryID uint   `json:"destinationLibraryId" binding:"required"`
}

type PublishAsIsInput struct {
	SourcePath           string `json:"sourcePath" binding:"required"`
	DestinationLibraryID uint   `json:"destinationLibraryId" binding:"required"`
}

type PublicationReconciliationInput struct {
	JobID uint   `json:"jobId" binding:"required"`
	Path  string `json:"path" binding:"required"`
}

type previewGeneration struct {
	done chan struct{}
	err  error
}

var previewCacheState = struct {
	sync.Mutex
	inFlight map[string]*previewGeneration
}{inFlight: map[string]*previewGeneration{}}

var assetInventorySyncMu sync.Mutex

type AssetInventory struct {
	Unprocessed       []Asset       `json:"unprocessed"`
	Library           []Asset       `json:"library"`
	Converted         []Asset       `json:"converted"`
	Unverified        []Asset       `json:"unverified"`
	Archive           []Asset       `json:"archive"`
	Missing           []Asset       `json:"missing"`
	UnprocessedGroups []AssetGroup  `json:"unprocessedGroups"`
	LibraryGroups     []AssetGroup  `json:"libraryGroups"`
	ConvertedGroups   []AssetGroup  `json:"convertedGroups"`
	UnverifiedGroups  []AssetGroup  `json:"unverifiedGroups"`
	ArchiveGroups     []AssetGroup  `json:"archiveGroups"`
	Reports           AssetReports  `json:"reports"`
	Sync              AssetSyncInfo `json:"sync"`
}

type AssetSyncInfo struct {
	LastSyncedAt      time.Time `json:"lastSyncedAt"`
	TotalRecords      int64     `json:"totalRecords"`
	MissingFiles      int64     `json:"missingFiles"`
	MissingActionable int       `json:"missingActionable"`
	MissingHistorical int       `json:"missingHistorical"`
}

type AssetReports struct {
	UnprocessedFiles  int   `json:"unprocessedFiles"`
	LibraryFiles      int   `json:"libraryFiles"`
	ConvertedFiles    int   `json:"convertedFiles"`
	UnverifiedFiles   int   `json:"unverifiedFiles"`
	ArchiveFiles      int   `json:"archiveFiles"`
	ArchiveBytes      int64 `json:"archiveBytes"`
	ExpiredArchive    int   `json:"expiredArchive"`
	MissingFiles      int   `json:"missingFiles"`
	MissingActionable int   `json:"missingActionable"`
	MissingHistorical int   `json:"missingHistorical"`
}

type AssetSyncResult struct {
	SyncedAt         time.Time `json:"syncedAt"`
	UnprocessedFiles int       `json:"unprocessedFiles"`
	LibraryFiles     int       `json:"libraryFiles"`
	ConvertedFiles   int       `json:"convertedFiles"`
	UnverifiedFiles  int       `json:"unverifiedFiles"`
	ArchiveFiles     int       `json:"archiveFiles"`
	ExpiredDeleted   int       `json:"expiredDeleted"`
	ReconciledFiles  int       `json:"reconciledFiles"`
	ReviewMatches    int       `json:"reviewMatches"`
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
	TrackProfileKey                string                         `json:"trackProfileKey,omitempty"`
	KeepVideoStreams               []int                          `json:"keepVideoStreams"`
	KeepAudioStreams               []int                          `json:"keepAudioStreams"`
	KeepSubtitleStreams            []int                          `json:"keepSubtitleStreams"`
	VideoMetadata                  map[int]StreamMetadataOverride `json:"videoMetadata,omitempty"`
	AudioMetadata                  map[int]StreamMetadataOverride `json:"audioMetadata,omitempty"`
	SubtitleMetadata               map[int]StreamMetadataOverride `json:"subtitleMetadata,omitempty"`
	SubtitleTransforms             []SubtitleTransform            `json:"subtitleTransforms,omitempty"`
	VideoCodec                     string                         `json:"videoCodec,omitempty"`
	AudioCodec                     string                         `json:"audioCodec,omitempty"`
	QualityMode                    string                         `json:"qualityMode,omitempty"`
	QualityValue                   int                            `json:"qualityValue,omitempty"`
	VideoPreset                    string                         `json:"videoPreset,omitempty"`
	PixFmt                         string                         `json:"pixFmt,omitempty"`
	VideoFilters                   string                         `json:"videoFilters,omitempty"`
	DeinterlaceMode                string                         `json:"deinterlaceMode,omitempty"`
	X265Params                     string                         `json:"x265Params,omitempty"`
	ProcessingMode                 string                         `json:"processingMode,omitempty"`
	PreserveHDR                    *bool                          `json:"preserveHdr,omitempty"`
	PreserveSubtitles              *bool                          `json:"preserveSubtitles,omitempty"`
	PreserveChapters               *bool                          `json:"preserveChapters,omitempty"`
	AddAACStereoTrack              *bool                          `json:"addAacStereoTrack,omitempty"`
	AACStereoDefault               *bool                          `json:"aacStereoDefault,omitempty"`
	EnhancedAudioSourceStreamIndex *int                           `json:"enhancedAudioSourceStreamIndex,omitempty"`
	UseHardwareIfAvailable         *bool                          `json:"useHardwareIfAvailable,omitempty"`
	VideoEncoder                   string                         `json:"videoEncoder,omitempty"`
	GlobalQuality                  int                            `json:"globalQuality,omitempty"`
	QSVRateControl                 string                         `json:"qsvRateControl,omitempty"`
	QSVLookAheadDepth              int                            `json:"qsvLookAheadDepth,omitempty"`
	QSVExtendedBRC                 *bool                          `json:"qsvExtendedBrc,omitempty"`
	QSVAdaptiveI                   *bool                          `json:"qsvAdaptiveI,omitempty"`
	QSVAdaptiveB                   *bool                          `json:"qsvAdaptiveB,omitempty"`
	UpdatedAt                      *time.Time                     `json:"updatedAt,omitempty"`
}

type StreamMetadataOverride struct {
	Title    string `json:"title,omitempty"`
	Language string `json:"language,omitempty"`
	Default  *bool  `json:"default,omitempty"`
	Forced   *bool  `json:"forced,omitempty"`
}

type SubtitleTransform struct {
	StreamIndex    int    `json:"streamIndex"`
	Format         string `json:"format"`
	RemoveEmbedded bool   `json:"removeEmbedded"`
	MakeDefault    bool   `json:"makeDefault"`
	Language       string `json:"language"`
	Title          string `json:"title,omitempty"`
}

type Asset struct {
	LibraryID       uint                         `json:"libraryId"`
	LibraryName     string                       `json:"libraryName"`
	Path            string                       `json:"path"`
	RelativePath    string                       `json:"relativePath"`
	GroupPath       string                       `json:"groupPath"`
	FileName        string                       `json:"fileName"`
	Extension       string                       `json:"extension"`
	SizeBytes       int64                        `json:"sizeBytes"`
	ModifiedAt      time.Time                    `json:"modifiedAt"`
	Status          string                       `json:"status"`
	Missing         bool                         `json:"missing"`
	ExpiresAt       *time.Time                   `json:"expiresAt,omitempty"`
	Review          AssetReviewState             `json:"review"`
	Metadata        AssetMetadataState           `json:"metadata"`
	Conversion      AssetConversionOverrideState `json:"conversion"`
	PublicationMode string                       `json:"publicationMode,omitempty"`
	Technical       *AssetTechnicalInfo          `json:"technical,omitempty"`
}

type AssetTechnicalInfo struct {
	VideoCodec string  `json:"videoCodec"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Duration   float64 `json:"duration"`
	Bitrate    int64   `json:"bitrate"`
	HDR        bool    `json:"hdr"`
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
	TrackProfileKey                string                         `json:"trackProfileKey"`
	KeepVideoStreams               []int                          `json:"keepVideoStreams"`
	KeepAudioStreams               []int                          `json:"keepAudioStreams"`
	KeepSubtitleStreams            []int                          `json:"keepSubtitleStreams"`
	VideoMetadata                  map[int]StreamMetadataOverride `json:"videoMetadata"`
	AudioMetadata                  map[int]StreamMetadataOverride `json:"audioMetadata"`
	SubtitleMetadata               map[int]StreamMetadataOverride `json:"subtitleMetadata"`
	SubtitleTransforms             []SubtitleTransform            `json:"subtitleTransforms"`
	VideoCodec                     string                         `json:"videoCodec"`
	AudioCodec                     string                         `json:"audioCodec"`
	QualityMode                    string                         `json:"qualityMode"`
	QualityValue                   int                            `json:"qualityValue"`
	VideoPreset                    string                         `json:"videoPreset"`
	PixFmt                         string                         `json:"pixFmt"`
	VideoFilters                   string                         `json:"videoFilters"`
	DeinterlaceMode                string                         `json:"deinterlaceMode"`
	X265Params                     string                         `json:"x265Params"`
	ProcessingMode                 string                         `json:"processingMode"`
	PreserveHDR                    *bool                          `json:"preserveHdr"`
	PreserveSubtitles              *bool                          `json:"preserveSubtitles"`
	PreserveChapters               *bool                          `json:"preserveChapters"`
	AddAACStereoTrack              *bool                          `json:"addAacStereoTrack"`
	AACStereoDefault               *bool                          `json:"aacStereoDefault"`
	EnhancedAudioSourceStreamIndex *int                           `json:"enhancedAudioSourceStreamIndex"`
	UseHardwareIfAvailable         *bool                          `json:"useHardwareIfAvailable"`
	VideoEncoder                   string                         `json:"videoEncoder"`
	GlobalQuality                  int                            `json:"globalQuality"`
	QSVRateControl                 string                         `json:"qsvRateControl"`
	QSVLookAheadDepth              int                            `json:"qsvLookAheadDepth"`
	QSVExtendedBRC                 *bool                          `json:"qsvExtendedBrc"`
	QSVAdaptiveI                   *bool                          `json:"qsvAdaptiveI"`
	QSVAdaptiveB                   *bool                          `json:"qsvAdaptiveB"`
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
	if err := resetRecoveredAssetOverrides(h.db, record.Path, destination); err != nil {
		appendSystemLog(h.db, "archive_asset_recovered_override_reset_failed", map[string]string{"archivePath": record.Path, "recoveredPath": destination}, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "asset was recovered, but its overrides could not be reset: " + err.Error()})
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

	var activePublications []models.QueueJob
	if err := h.db.Where("published_path = ? AND publication_retired_at IS NULL", path).Order("id desc").Find(&activePublications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var job models.QueueJob
	if len(activePublications) > 0 {
		job = activePublications[0]
	} else if err := h.db.Where("published_path = ?", path).Order("id desc").First(&job).Error; err != nil {
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
	retiredPublicationIDs := []uint{}
	for index := range activePublications {
		activePublications[index].PublicationRetiredAt = &retiredAt
		activePublications[index].Notes = appendNote(activePublications[index].Notes, "Safe deletion retired this publication before restoring its archived original")
		if err := h.db.Save(&activePublications[index]).Error; err != nil {
			for _, retiredID := range retiredPublicationIDs {
				_ = h.db.Model(&models.QueueJob{}).Where("id = ?", retiredID).Updates(map[string]any{"publication_retired_at": nil}).Error
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not lock the publication against automatic reconciliation: " + err.Error()})
			return
		}
		retiredPublicationIDs = append(retiredPublicationIDs, activePublications[index].ID)
	}
	if len(activePublications) > 0 {
		job = activePublications[0]
	}
	rollbackRetirements := func() {
		for _, retiredID := range retiredPublicationIDs {
			_ = h.db.Model(&models.QueueJob{}).Where("id = ?", retiredID).Updates(map[string]any{"publication_retired_at": nil}).Error
		}
	}
	quarantine := ""
	if convertedExists {
		quarantine = fmt.Sprintf("%s.mvforge-delete-%d", path, time.Now().UnixNano())
		if err := os.Rename(path, quarantine); err != nil {
			rollbackRetirements()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not prepare converted asset for safe deletion: " + err.Error()})
			return
		}
	}
	if err := moveFile(archivePath, restorePath); err != nil {
		if quarantine != "" {
			_ = os.Rename(quarantine, path)
		}
		rollbackRetirements()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "original restoration failed; converted asset was restored: " + err.Error()})
		return
	}
	if quarantine != "" {
		if err := os.Remove(quarantine); err != nil {
			rollbackArchive := moveFile(restorePath, archivePath)
			rollbackConverted := os.Rename(quarantine, path)
			rollbackRetirements()
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
	if err := resetRecoveredAssetOverrides(h.db, path, archivePath, restorePath, job.MediaPath); err != nil {
		appendSystemLog(h.db, "converted_asset_deleted_override_reset_failed", map[string]string{"convertedPath": path, "archivePath": archivePath, "restoredPath": restorePath}, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "original was restored, but its overrides could not be reset: " + err.Error()})
		return
	}
	if _, syncErr := h.syncAssetInventory(); syncErr != nil {
		appendSystemLog(h.db, "converted_asset_deleted_inventory_sync_failed", map[string]string{"convertedPath": path, "restoredPath": restorePath}, syncErr)
	}
	appendSystemLog(h.db, "converted_asset_deleted_original_restored", map[string]string{"convertedPath": path, "archivePath": archivePath, "restoredPath": restorePath, "jobId": strconv.FormatUint(uint64(job.ID), 10)}, nil)
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "convertedPath": path, "archivedOriginalPath": archivePath, "restoredPath": restorePath, "jobId": job.ID, "message": "Converted asset deleted and original restored to Raw. Reports, logs, and job history were preserved."})
}

func (h AssetHandler) ReturnPublishedAsIs(c *gin.Context) {
	path := filepath.Clean(strings.TrimSpace(c.Query("path")))
	if path == "." || path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	var publication models.DirectPublication
	if err := h.db.Where("published_path = ? AND returned_at IS NULL", path).First(&publication).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "active Publish as-is record not found"})
		return
	}
	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	restorePath := filepath.Clean(publication.SourcePath)
	if !pathIsInside(restorePath, rawRoot) {
		c.JSON(http.StatusConflict, gin.H{"error": "the original Raw destination is outside the configured Raw root"})
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "the Published as-is asset is not physically available"})
		return
	}
	if _, err := os.Stat(restorePath); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "an asset already exists at the original Raw path"})
		return
	} else if !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(err)})
		return
	}
	fingerprint, err := mediaFileFingerprint(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if publication.PublishedFingerprint != "" && fingerprint != publication.PublishedFingerprint {
		c.JSON(http.StatusConflict, gin.H{"error": "the Library file no longer matches its Publish as-is fingerprint; return was blocked"})
		return
	}

	sidecars, err := externalSubtitlesForMedia(path)
	if err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	publishedBase := strings.TrimSuffix(path, filepath.Ext(path))
	restoreBase := strings.TrimSuffix(restorePath, filepath.Ext(restorePath))
	type movePair struct{ from, to string }
	moves := []movePair{{from: path, to: restorePath}}
	for _, sidecar := range sidecars {
		suffix := strings.TrimPrefix(sidecar.Path, publishedBase)
		moves = append(moves, movePair{from: sidecar.Path, to: restoreBase + suffix})
	}
	for _, move := range moves {
		if _, err := os.Stat(move.to); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "return target already exists: " + move.to})
			return
		} else if !os.IsNotExist(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(err)})
			return
		}
	}
	completed := []movePair{}
	rollback := func() {
		for index := len(completed) - 1; index >= 0; index-- {
			_ = moveFile(completed[index].to, completed[index].from)
		}
	}
	for _, move := range moves {
		if err := moveFile(move.from, move.to); err != nil {
			rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "return to Raw failed and rollback was attempted: " + err.Error()})
			return
		}
		completed = append(completed, move)
	}

	returnedAt := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		publication.ReturnedAt = &returnedAt
		if err := tx.Save(&publication).Error; err != nil {
			return err
		}
		if err := tx.Where("path = ?", path).Delete(&models.AssetRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.ScanResult{}).Where("path = ?", path).
			Updates(map[string]any{"path": restorePath, "file_name": filepath.Base(restorePath)}).Error; err != nil {
			return err
		}
		return returnDirectPublicationOverrides(tx, path, restorePath)
	})
	if err != nil {
		rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "asset was moved but registration failed and rollback was attempted: " + err.Error()})
		return
	}
	var library models.Library
	if err := h.db.First(&library, publication.LibraryID).Error; err == nil && pathIsInside(filepath.Dir(path), library.DestinationPath) {
		if err := (PublisherHandler{db: h.db}).cleanupEmptyOriginalDirs(filepath.Dir(path), library.DestinationPath); err != nil {
			appendSystemLog(h.db, "published_as_is_empty_library_path_cleanup_failed", map[string]string{"publishedPath": path}, err)
		}
	}
	if _, syncErr := h.syncAssetInventory(); syncErr != nil {
		appendSystemLog(h.db, "published_as_is_return_inventory_sync_failed", map[string]string{"publishedPath": path, "restoredPath": restorePath}, syncErr)
	}
	appendSystemLog(h.db, "published_as_is_returned_to_raw", map[string]string{"publishedPath": path, "restoredPath": restorePath}, nil)
	c.JSON(http.StatusOK, gin.H{
		"status": "returned_to_raw", "publishedPath": path, "restoredPath": restorePath,
		"message": "Published as-is asset and external subtitles were returned to their original Raw names.",
	})
}

func (h AssetHandler) ExtractSubtitles(c *gin.Context) {
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
	if record.Status == "archive" {
		c.JSON(http.StatusConflict, gin.H{"error": "recover the archived original before generating external subtitles"})
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "converted asset is not readable from the backend container"})
		return
	}
	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "converted asset is outside configured libraries"})
		return
	}

	probe, _, err := runFFProbe(path, 20)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	streams := probe.Streams
	if !hasSubtitleProbeStream(streams) {
		if subtitleStreams, probeErr := probeSubtitleStreams(c.Request.Context(), path); probeErr == nil && len(subtitleStreams) > 0 {
			streams = append(streams, subtitleStreams...)
		}
	}
	var input SubtitleExtractionInput
	if err := c.ShouldBindJSON(&input); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.Format = strings.ToLower(strings.TrimSpace(input.Format))
	if input.Format != "" && input.Format != "srt" && input.Format != "ass" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be srt or ass"})
		return
	}
	plans, unsupported := subtitleExtractionPlansForRequest(path, streams, input)
	bitmapStreams := selectedBitmapSubtitleStreams(streams, input.StreamIndex)
	if len(plans) == 0 && len(bitmapStreams) == 0 {
		if len(unsupported) > 0 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":       "the selected subtitle track cannot be converted to SRT or ASS",
				"unsupported": unsupported,
			})
			return
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Sprintf(
			"FFprobe found no embedded subtitle tracks in %s (%s)",
			filepath.Base(path),
			probeStreamSummary(streams),
		)})
		return
	}

	result := SubtitleExtractionResult{Created: []string{}, Existing: []string{}, Unsupported: []string{}}
	for _, plan := range plans {
		if existing, statErr := os.Stat(plan.OutputPath); statErr == nil && !existing.IsDir() {
			result.Existing = append(result.Existing, plan.OutputPath)
			continue
		} else if statErr != nil && !os.IsNotExist(statErr) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(statErr)})
			return
		}

		temp, err := os.CreateTemp(filepath.Dir(plan.OutputPath), ".mvforge-subtitle-*."+plan.Format)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("cannot create subtitle beside the converted asset: %v", err)})
			return
		}
		tempPath := temp.Name()
		if err := temp.Close(); err != nil {
			_ = os.Remove(tempPath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = os.Remove(tempPath)

		args := []string{
			"-hide_banner", "-loglevel", "error", "-nostdin",
			"-i", path,
			"-map", fmt.Sprintf("0:%d", plan.StreamIndex),
			"-vn", "-an",
			"-c:s", plan.Codec,
			"-f", plan.Format,
			tempPath,
		}
		cmd := exec.CommandContext(c.Request.Context(), "ffmpeg", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			_ = os.Remove(tempPath)
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Sprintf("subtitle stream %d could not be extracted: %s", plan.StreamIndex, message)})
			return
		}
		if err := os.Link(tempPath, plan.OutputPath); err != nil {
			_ = os.Remove(tempPath)
			if os.IsExist(err) {
				result.Existing = append(result.Existing, plan.OutputPath)
				continue
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("cannot publish extracted subtitle: %v", err)})
			return
		}
		_ = os.Remove(tempPath)
		result.Created = append(result.Created, plan.OutputPath)
	}
	for _, stream := range bitmapStreams {
		ocrResult, ocrErr := generateBitmapSubtitleSidecar(c.Request.Context(), path, stream, input)
		if ocrErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":       ocrErr.Error(),
				"created":     result.Created,
				"existing":    result.Existing,
				"unsupported": result.Unsupported,
			})
			return
		}
		result.Created = append(result.Created, ocrResult.Created...)
		result.Existing = append(result.Existing, ocrResult.Existing...)
	}

	c.JSON(http.StatusOK, result)
}

func (h AssetHandler) ExternalSubtitles(c *gin.Context) {
	mediaPath, _, ok := h.externalSubtitleAsset(c)
	if !ok {
		return
	}
	values, err := externalSubtitlesForMedia(mediaPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, values)
}

func (h AssetHandler) ExternalSubtitleContent(c *gin.Context) {
	mediaPath, _, ok := h.externalSubtitleAsset(c)
	if !ok {
		return
	}
	subtitlePath, err := validatedExternalSubtitlePath(mediaPath, c.Query("subtitlePath"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content, err := os.ReadFile(subtitlePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": mediaPathReadError(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": subtitlePath, "content": string(content)})
}

func (h AssetHandler) UpdateExternalSubtitle(c *gin.Context) {
	mediaPath, _, ok := h.externalSubtitleAsset(c)
	if !ok {
		return
	}
	var input ExternalSubtitleUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subtitlePath, err := validatedExternalSubtitlePath(mediaPath, input.SubtitlePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subtitle content cannot be empty"})
		return
	}
	if len(input.Content) > 8*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "subtitle content exceeds 8 MiB"})
		return
	}
	existingInfo, err := os.Stat(subtitlePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": mediaPathReadError(err)})
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(subtitlePath), ".mvforge-subtitle-edit-*"+filepath.Ext(subtitlePath))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tempPath := temp.Name()
	if err = temp.Chmod(existingInfo.Mode()); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err = temp.WriteString(input.Content); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tempPath, subtitlePath)
	}
	if err != nil {
		_ = os.Remove(tempPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": subtitlePath, "message": "External subtitle updated."})
}

func (h AssetHandler) DeleteExternalSubtitle(c *gin.Context) {
	mediaPath, _, ok := h.externalSubtitleAsset(c)
	if !ok {
		return
	}
	var input ExternalSubtitleUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subtitlePath, err := validatedExternalSubtitlePath(mediaPath, input.SubtitlePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := os.Remove(subtitlePath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "external subtitle no longer exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": subtitlePath, "message": "External subtitle deleted."})
}

func (h AssetHandler) externalSubtitleAsset(c *gin.Context) (string, models.AssetRecord, bool) {
	path := filepath.Clean(strings.TrimSpace(c.Query("path")))
	if path == "." || path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return "", models.AssetRecord{}, false
	}
	var record models.AssetRecord
	if err := h.db.First(&record, "path = ?", path).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found in inventory"})
		return "", models.AssetRecord{}, false
	}
	if record.Status == "archive" {
		c.JSON(http.StatusConflict, gin.H{"error": "archived originals cannot manage external subtitles"})
		return "", models.AssetRecord{}, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset is not readable from the backend container"})
		return "", models.AssetRecord{}, false
	}
	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return "", models.AssetRecord{}, false
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "asset is outside configured media roots"})
		return "", models.AssetRecord{}, false
	}
	return path, record, true
}

func (h AssetHandler) PublishAsIs(c *gin.Context) {
	var input PublishAsIsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sourcePath := filepath.Clean(strings.TrimSpace(input.SourcePath))
	info, err := os.Stat(sourcePath)
	if err != nil || !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourcePath must be a readable Raw directory"})
		return
	}

	var library models.Library
	if err := h.db.First(&library, input.DestinationLibraryID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "destination library not found"})
		return
	}
	var records []models.AssetRecord
	if err := h.db.Where("status = ? AND missing = ?", "unprocessed", false).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	publishing := make([]models.AssetRecord, 0)
	for _, record := range records {
		if pathIsInside(record.Path, sourcePath) {
			publishing = append(publishing, record)
		}
	}
	if len(publishing) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "source path has no unprocessed assets"})
		return
	}
	sort.Slice(publishing, func(left, right int) bool {
		return strings.ToLower(publishing[left].Path) < strings.ToLower(publishing[right].Path)
	})
	paths := make([]string, 0, len(publishing))
	for _, record := range publishing {
		paths = append(paths, record.Path)
	}
	var openJobs int64
	if err := h.db.Model(&models.QueueJob{}).
		Where("media_path IN ? AND status IN ?", paths, []string{JobStatusQueued, JobStatusRunning}).
		Count(&openJobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if openJobs > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "source path has assets with open Queue jobs"})
		return
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sourceRoot := filepath.Clean(rawRoot)
	if candidate := filepath.Clean(strings.TrimSpace(library.SourcePath)); candidate != "." && candidate != "" && pathIsInside(sourcePath, candidate) {
		sourceRoot = candidate
	}
	relativeGroup, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil || relativeGroup == "." || relativeGroup == ".." || strings.HasPrefix(relativeGroup, ".."+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source path is outside the selected library Raw root"})
		return
	}
	relativeGroup = directPublicationRelativeGroup(relativeGroup, library.DestinationPath)
	destinationPath := filepath.Join(library.DestinationPath, relativeGroup)
	if destinationInfo, err := os.Stat(destinationPath); err == nil {
		entries, readErr := os.ReadDir(destinationPath)
		if !destinationInfo.IsDir() || readErr != nil || len(entries) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "destination path already exists"})
			return
		}
		if err := os.Remove(destinationPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "empty destination path could not be prepared: " + err.Error()})
			return
		}
	} else if !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(err)})
		return
	}

	fingerprints := make(map[string]string, len(publishing))
	for _, record := range publishing {
		fingerprint, err := mediaFileFingerprint(record.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not validate " + record.FileName + ": " + err.Error()})
			return
		}
		fingerprints[record.Path] = fingerprint
	}
	if err := moveAssetDirectory(sourcePath, destinationPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "direct publication failed: " + err.Error()})
		return
	}
	publishedPaths, rollbackNames, err := applyDirectPublicationEpisodeNames(sourcePath, destinationPath, publishing, library)
	if err != nil {
		_ = moveAssetDirectory(destinationPath, sourcePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "episode naming failed and publication was rolled back: " + err.Error()})
		return
	}
	rollback := func() {
		rollbackNames()
		if err := moveAssetDirectory(destinationPath, sourcePath); err != nil {
			appendSystemLog(h.db, "direct_publication_rollback_failed", map[string]string{"sourcePath": sourcePath, "destinationPath": destinationPath}, err)
		}
	}

	publishedAt := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, record := range publishing {
			publishedPath := publishedPaths[record.Path]
			publication := models.DirectPublication{
				SourcePath: record.Path, PublishedPath: publishedPath, LibraryID: library.ID,
				PublishedFingerprint: fingerprints[record.Path], PublishedSizeBytes: record.SizeBytes, PublishedAt: publishedAt,
			}
			if err := saveDirectPublication(tx, &publication); err != nil {
				return err
			}
			if err := tx.Where("path = ?", record.Path).Delete(&models.AssetRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.ScanResult{}).Where("path = ?", record.Path).
				Updates(map[string]any{"path": publishedPath, "file_name": filepath.Base(publishedPath)}).Error; err != nil {
				return err
			}
		}
		return resetDirectPublicationOverrides(tx, publishing, sourcePath, destinationPath, publishedPaths)
	})
	if err != nil {
		rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "files were moved but registration failed and rollback was attempted: " + err.Error()})
		return
	}
	if _, err := h.syncAssetInventory(); err != nil {
		appendSystemLog(h.db, "direct_publication_inventory_sync_failed", map[string]string{"destinationPath": destinationPath}, err)
	}
	appendSystemLog(h.db, "assets_published_as_is", map[string]string{
		"sourcePath": sourcePath, "destinationPath": destinationPath, "library": library.Name, "assets": strconv.Itoa(len(publishing)),
	}, nil)
	c.JSON(http.StatusOK, gin.H{
		"status": "published_as_is", "sourcePath": sourcePath, "destinationPath": destinationPath,
		"destinationLibraryId": library.ID, "assetsPublished": len(publishing),
		"message": "Assets were validated and published without FFmpeg conversion.",
	})
}

func saveDirectPublication(db *gorm.DB, publication *models.DirectPublication) error {
	var existing models.DirectPublication
	result := db.Where("published_path = ?", publication.PublishedPath).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return db.Create(publication).Error
	}
	publication.ID = existing.ID
	publication.CreatedAt = existing.CreatedAt
	publication.ReturnedAt = nil
	return db.Save(publication).Error
}

func applyDirectPublicationEpisodeNames(sourcePath, destinationPath string, records []models.AssetRecord, library models.Library) (map[string]string, func(), error) {
	published := make(map[string]string, len(records))
	for _, record := range records {
		relative, err := filepath.Rel(sourcePath, record.Path)
		if err != nil {
			return nil, func() {}, err
		}
		published[record.Path] = filepath.Join(destinationPath, relative)
	}
	if !libraryEpisodeNamingEnabled(library) || len(records) <= 1 {
		return published, func() {}, nil
	}

	sorted := append([]models.AssetRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Path) < strings.ToLower(sorted[j].Path)
	})
	batchName := filepath.ToSlash(filepath.Base(sourcePath))
	title := sanitizeMediaFileName(filepath.Base(sourcePath))
	season := firstPositiveInt(seasonNumberFromPath(sourcePath), 1)
	targets := map[string]string{}
	for index, record := range sorted {
		relative, err := filepath.Rel(sourcePath, record.Path)
		if err != nil {
			return nil, func() {}, err
		}
		job := models.QueueJob{MediaPath: record.Path, BatchName: batchName}
		spec := multiEpisodeNameSpec{SeriesTitle: title, Season: season, Episode: index + 1}
		namedRelative := filepath.FromSlash(formatMultiEpisodeOutputRelativePath(job, filepath.ToSlash(relative), spec))
		targets[record.Path] = filepath.Join(destinationPath, namedRelative)
	}

	seenTargets := map[string]bool{}
	for _, target := range targets {
		clean := filepath.Clean(target)
		if seenTargets[clean] {
			return nil, func() {}, fmt.Errorf("episode naming produced duplicate target %s", target)
		}
		seenTargets[clean] = true
	}
	type renamePair struct{ from, to string }
	completed := []renamePair{}
	rollback := func() {
		for index := len(completed) - 1; index >= 0; index-- {
			_ = os.Rename(completed[index].to, completed[index].from)
		}
	}
	for _, record := range sorted {
		current := published[record.Path]
		target := targets[record.Path]
		if current == target {
			continue
		}
		sidecars, err := externalSubtitlesForMedia(current)
		if err != nil && !os.IsNotExist(err) {
			rollback()
			return nil, func() {}, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			rollback()
			return nil, func() {}, err
		}
		if _, err := os.Stat(target); err == nil {
			rollback()
			return nil, func() {}, fmt.Errorf("episode target already exists: %s", target)
		} else if !os.IsNotExist(err) {
			rollback()
			return nil, func() {}, err
		}
		if err := os.Rename(current, target); err != nil {
			rollback()
			return nil, func() {}, err
		}
		completed = append(completed, renamePair{from: current, to: target})
		sourceBase := strings.TrimSuffix(current, filepath.Ext(current))
		targetBase := strings.TrimSuffix(target, filepath.Ext(target))
		for _, sidecar := range sidecars {
			suffix := strings.TrimPrefix(sidecar.Path, sourceBase)
			sidecarTarget := targetBase + suffix
			if _, err := os.Stat(sidecarTarget); err == nil {
				rollback()
				return nil, func() {}, fmt.Errorf("subtitle target already exists: %s", sidecarTarget)
			} else if !os.IsNotExist(err) {
				rollback()
				return nil, func() {}, err
			}
			if err := os.Rename(sidecar.Path, sidecarTarget); err != nil {
				rollback()
				return nil, func() {}, err
			}
			completed = append(completed, renamePair{from: sidecar.Path, to: sidecarTarget})
		}
		published[record.Path] = target
	}
	return published, rollback, nil
}

func directPublicationRelativeGroup(relativeGroup, destinationRoot string) string {
	clean := filepath.Clean(relativeGroup)
	relative := libraryOutputBaseRelativePath(
		filepath.ToSlash(clean),
		models.Library{DestinationPath: filepath.ToSlash(filepath.Clean(destinationRoot))},
	)
	return filepath.FromSlash(relative)
}

func (h AssetHandler) MigratePath(c *gin.Context) {
	var input AssetPathMigrationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sourcePath := filepath.Clean(strings.TrimSpace(input.SourcePath))
	if sourcePath == "." || sourcePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourcePath is required"})
		return
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil || !sourceInfo.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source path must be a readable Library directory"})
		return
	}

	var records []models.AssetRecord
	if err := h.db.Where("missing = ?", false).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	migrating := make([]models.AssetRecord, 0)
	for _, record := range records {
		if pathIsInside(record.Path, sourcePath) && (record.Status == "library" || record.Status == "unverified" || record.Status == "converted") {
			migrating = append(migrating, record)
		}
	}
	if len(migrating) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "source path has no Library or Converted assets"})
		return
	}
	sourceLibraryID := migrating[0].LibraryID
	for _, record := range migrating {
		if record.LibraryID != sourceLibraryID {
			c.JSON(http.StatusConflict, gin.H{"error": "source path contains assets from more than one library"})
			return
		}
	}
	migratingPaths := make([]string, 0, len(migrating))
	for _, record := range migrating {
		migratingPaths = append(migratingPaths, record.Path)
	}
	var openJobs int64
	if err := h.db.Model(&models.QueueJob{}).
		Where("media_path IN ? AND status IN ?", migratingPaths, []string{JobStatusQueued, JobStatusRunning}).
		Count(&openJobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if openJobs > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "source path has assets with open Queue jobs"})
		return
	}

	var sourceLibrary models.Library
	if err := h.db.First(&sourceLibrary, sourceLibraryID).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "source library could not be resolved"})
		return
	}
	var destinationLibrary models.Library
	if err := h.db.First(&destinationLibrary, input.DestinationLibraryID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "destination library not found"})
		return
	}
	if sourceLibrary.ID == destinationLibrary.ID {
		c.JSON(http.StatusConflict, gin.H{"error": "source and destination libraries are the same"})
		return
	}
	relativeGroup, err := filepath.Rel(sourceLibrary.DestinationPath, sourcePath)
	if err != nil || relativeGroup == "." || strings.HasPrefix(relativeGroup, ".."+string(filepath.Separator)) || relativeGroup == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source path is outside its registered library"})
		return
	}
	destinationPath := filepath.Join(destinationLibrary.DestinationPath, relativeGroup)
	if _, err := os.Stat(destinationPath); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "destination path already exists"})
		return
	} else if !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": mediaPathReadError(err)})
		return
	}

	if err := moveAssetDirectory(sourcePath, destinationPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "path migration failed: " + err.Error()})
		return
	}
	rollback := func() {
		if err := moveAssetDirectory(destinationPath, sourcePath); err != nil {
			appendSystemLog(h.db, "asset_path_migration_rollback_failed", map[string]string{"sourcePath": sourcePath, "destinationPath": destinationPath}, err)
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, record := range migrating {
			relativeToGroup, relErr := filepath.Rel(sourcePath, record.Path)
			if relErr != nil || strings.HasPrefix(relativeToGroup, ".."+string(filepath.Separator)) {
				return fmt.Errorf("invalid asset path %s", record.Path)
			}
			oldPath := record.Path
			newPath := filepath.Join(destinationPath, relativeToGroup)
			relativeToLibrary, relErr := filepath.Rel(destinationLibrary.DestinationPath, newPath)
			if relErr != nil {
				return relErr
			}
			if err := tx.Model(&models.AssetRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
				"path":          filepath.Clean(newPath),
				"root_path":     filepath.Clean(destinationLibrary.DestinationPath),
				"relative_path": filepath.ToSlash(relativeToLibrary),
				"library_id":    destinationLibrary.ID,
				"library_name":  destinationLibrary.Name,
				"synced_at":     time.Now(),
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.QueueJob{}).
				Where("published_path = ?", oldPath).
				Updates(map[string]any{"published_path": newPath, "library_id": destinationLibrary.ID, "notes": gorm.Expr("COALESCE(notes, '') || ?", "\nConverted asset migrated to "+newPath)}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.QueueJob{}).
				Where("replacement_target_path = ?", oldPath).
				Update("replacement_target_path", newPath).Error; err != nil {
				return err
			}
			if tx.Migrator().HasTable(&models.DirectPublication{}) {
				if err := tx.Model(&models.DirectPublication{}).
					Where("published_path = ? AND returned_at IS NULL", oldPath).
					Updates(map[string]any{"published_path": newPath, "library_id": destinationLibrary.ID}).Error; err != nil {
					return err
				}
			}
		}
		return migrateAssetPathOverrides(tx, sourcePath, destinationPath)
	})
	if err != nil {
		rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "path was moved but inventory update failed and rollback was attempted: " + err.Error()})
		return
	}

	appendSystemLog(h.db, "asset_path_migrated", map[string]string{
		"sourcePath": sourcePath, "destinationPath": destinationPath,
		"sourceLibrary": sourceLibrary.Name, "destinationLibrary": destinationLibrary.Name,
		"assets": strconv.Itoa(len(migrating)),
	}, nil)
	c.JSON(http.StatusOK, gin.H{
		"status": "migrated", "sourcePath": sourcePath, "destinationPath": destinationPath,
		"sourceLibraryId": sourceLibrary.ID, "destinationLibraryId": destinationLibrary.ID,
		"assetsMoved": len(migrating),
	})
}

func (h AssetHandler) ConfirmPublicationReconciliation(c *gin.Context) {
	var input PublicationReconciliationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	candidatePath := filepath.Clean(strings.TrimSpace(input.Path))
	var record models.AssetRecord
	if err := h.db.Where("path = ? AND missing = ?", candidatePath, false).First(&record).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "candidate Library asset not found"})
		return
	}
	var job models.QueueJob
	if err := h.db.First(&job, input.JobID).Error; err != nil || job.PublicationRetiredAt != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "active publication job not found"})
		return
	}
	if _, err := os.Stat(job.PublishedPath); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the job's current published path still exists"})
		return
	}
	review := reviewForPath(candidatePath, assetReviewOverrides(h.db))
	expectedTag := fmt.Sprintf("reconciliation-job-%d", job.ID)
	if review.Source != "sync-reconciliation" || !containsString(review.Tags, expectedTag) {
		c.JSON(http.StatusConflict, gin.H{"error": "candidate is not an approved Sync reconciliation match"})
		return
	}
	if job.PublishedFingerprint != "" {
		fingerprint, err := mediaFileFingerprint(candidatePath)
		if err != nil || fingerprint != job.PublishedFingerprint {
			c.JSON(http.StatusConflict, gin.H{"error": "candidate fingerprint does not match the publication"})
			return
		}
	}
	oldPath := filepath.Clean(job.PublishedPath)
	staleRecordsRemoved := int64(0)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		job.PublishedPath = candidatePath
		job.LibraryID = record.LibraryID
		job.Notes = appendNote(job.Notes, "User confirmed externally relocated converted asset: "+oldPath+" -> "+candidatePath)
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		if err := migrateSingleAssetPathOverrides(tx, oldPath, candidatePath); err != nil {
			return err
		}
		result := tx.Where("path = ? AND missing = ?", oldPath, true).Delete(&models.AssetRecord{})
		if result.Error != nil {
			return result.Error
		}
		staleRecordsRemoved = result.RowsAffected
		if staleRecordsRemoved > 0 {
			if err := tx.Where("path = ?", oldPath).Delete(&models.ScanResult{}).Error; err != nil {
				return err
			}
		}
		reviews := assetReviewOverrides(tx)
		delete(reviews, candidatePath)
		return saveAssetReviewOverrides(tx, reviews)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "reconciled", "jobId": job.ID, "oldPath": oldPath,
		"publishedPath": candidatePath, "staleRecordsRemoved": staleRecordsRemoved,
	})
}

func moveAssetDirectory(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported non-regular file: %s", path)
		}
		return copyPublishedFile(path, target, false)
	}); err != nil {
		_ = os.RemoveAll(destination)
		return err
	}
	return os.RemoveAll(source)
}

func migrateAssetPathOverrides(db *gorm.DB, sourcePath, destinationPath string) error {
	conversion := assetConversionOverrides(db)
	for oldPath, value := range conversion {
		if oldPath == sourcePath || pathIsInside(oldPath, sourcePath) {
			relative, _ := filepath.Rel(sourcePath, oldPath)
			delete(conversion, oldPath)
			conversion[filepath.Join(destinationPath, relative)] = value
		}
	}
	if err := saveAssetConversionOverrides(db, conversion); err != nil {
		return err
	}
	reviews := assetReviewOverrides(db)
	for oldPath, value := range reviews {
		if oldPath == sourcePath || pathIsInside(oldPath, sourcePath) {
			relative, _ := filepath.Rel(sourcePath, oldPath)
			delete(reviews, oldPath)
			reviews[filepath.Join(destinationPath, relative)] = value
		}
	}
	if err := saveAssetReviewOverrides(db, reviews); err != nil {
		return err
	}
	metadata := assetMetadataOverrides(db)
	for oldPath, value := range metadata {
		if oldPath == sourcePath || pathIsInside(oldPath, sourcePath) {
			relative, _ := filepath.Rel(sourcePath, oldPath)
			delete(metadata, oldPath)
			metadata[filepath.Join(destinationPath, relative)] = value
		}
	}
	return saveAssetMetadataOverrides(db, metadata)
}

func resetDirectPublicationOverrides(db *gorm.DB, records []models.AssetRecord, sourcePath, destinationPath string, publishedPaths map[string]string) error {
	conversion := assetConversionOverrides(db)
	reviews := assetReviewOverrides(db)
	metadata := assetMetadataOverrides(db)
	for _, record := range records {
		relative, err := filepath.Rel(sourcePath, record.Path)
		if err != nil {
			return err
		}
		newPath := filepath.Join(destinationPath, relative)
		if publishedPath := strings.TrimSpace(publishedPaths[record.Path]); publishedPath != "" {
			newPath = publishedPath
		}
		delete(conversion, record.Path)
		delete(reviews, record.Path)
		if value, exists := metadata[record.Path]; exists {
			delete(metadata, record.Path)
			metadata[newPath] = value
		}
	}
	if err := saveAssetConversionOverrides(db, conversion); err != nil {
		return err
	}
	if err := saveAssetReviewOverrides(db, reviews); err != nil {
		return err
	}
	return saveAssetMetadataOverrides(db, metadata)
}

func hasSubtitleProbeStream(streams []FFProbeStream) bool {
	for _, stream := range streams {
		if stream.CodecType == "subtitle" {
			return true
		}
	}
	return false
}

func probeSubtitleStreams(ctx context.Context, path string) ([]FFProbeStream, error) {
	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "error",
		"-select_streams", "s",
		"-show_entries", "stream=index,codec_type,codec_name:stream_tags=language,title",
		"-of", "json",
		path,
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("%s", message)
	}
	var probe struct {
		Streams []FFProbeStream `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		return nil, err
	}
	return probe.Streams, nil
}

func probeStreamSummary(streams []FFProbeStream) string {
	counts := map[string]int{}
	for _, stream := range streams {
		streamType := strings.TrimSpace(stream.CodecType)
		if streamType == "" {
			streamType = "unknown"
		}
		counts[streamType]++
	}
	if len(counts) == 0 {
		return "0 streams"
	}
	keys := make([]string, 0, len(counts))
	for streamType := range counts {
		keys = append(keys, streamType)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, streamType := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", streamType, counts[streamType]))
	}
	return strings.Join(parts, ", ")
}

type subtitleExtractionPlan struct {
	StreamIndex int
	Codec       string
	Format      string
	OutputPath  string
}

func subtitleExtractionPlans(mediaPath string, streams []FFProbeStream) ([]subtitleExtractionPlan, []string) {
	return subtitleExtractionPlansForRequest(mediaPath, streams, SubtitleExtractionInput{})
}

func subtitleExtractionPlansForRequest(mediaPath string, streams []FFProbeStream, input SubtitleExtractionInput) ([]subtitleExtractionPlan, []string) {
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	plans := []subtitleExtractionPlan{}
	unsupported := []string{}
	requestedFormat := strings.ToLower(strings.TrimSpace(input.Format))
	if requestedFormat != "" && requestedFormat != "srt" && requestedFormat != "ass" {
		return plans, []string{"requested output format must be srt or ass"}
	}
	for _, stream := range streams {
		if stream.CodecType != "subtitle" {
			continue
		}
		if input.StreamIndex != nil && stream.Index != *input.StreamIndex {
			continue
		}
		if !subtitleCanConvertText(stream.CodecName) {
			unsupported = append(unsupported, fmt.Sprintf("stream %d (%s)", stream.Index, stream.CodecName))
			continue
		}
		format := "srt"
		codec := "srt"
		if strings.EqualFold(stream.CodecName, "ass") || strings.EqualFold(stream.CodecName, "ssa") {
			format = "ass"
			codec = "ass"
		}
		if requestedFormat != "" {
			format = requestedFormat
			codec = requestedFormat
		}
		language := safeSubtitleFilenamePart(stream.Tags["language"])
		if language == "" {
			language = "und"
		}
		plans = append(plans, subtitleExtractionPlan{
			StreamIndex: stream.Index,
			Codec:       codec,
			Format:      format,
			OutputPath:  fmt.Sprintf("%s.%s.%d.%s", base, language, stream.Index, format),
		})
	}
	if input.StreamIndex != nil && len(plans) == 0 && len(unsupported) == 0 {
		unsupported = append(unsupported, fmt.Sprintf("subtitle stream %d was not found", *input.StreamIndex))
	}
	return plans, unsupported
}

func selectedBitmapSubtitleStreams(streams []FFProbeStream, requestedIndex *int) []FFProbeStream {
	selected := []FFProbeStream{}
	for _, stream := range streams {
		if stream.CodecType != "subtitle" || !isBitmapSubtitleCodecName(stream.CodecName) {
			continue
		}
		if requestedIndex != nil && stream.Index != *requestedIndex {
			continue
		}
		selected = append(selected, stream)
	}
	return selected
}

func externalSubtitlesForMedia(mediaPath string) ([]ExternalSubtitle, error) {
	directory := filepath.Dir(mediaPath)
	stem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	values := []ExternalSubtitle{}
	for _, entry := range entries {
		if entry.IsDir() || !externalSubtitleNameForStem(entry.Name(), stem) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(entry.Name())), ".")
		tokens := strings.Split(strings.TrimSuffix(strings.TrimPrefix(entry.Name(), stem), filepath.Ext(entry.Name())), ".")
		language := ""
		isDefault, forced := false, false
		for _, token := range tokens {
			clean := strings.ToLower(strings.TrimSpace(token))
			switch clean {
			case "", "default":
				isDefault = isDefault || clean == "default"
			case "forced":
				forced = true
			default:
				if language == "" {
					if _, err := strconv.Atoi(clean); err != nil {
						language = clean
					}
				}
			}
		}
		values = append(values, ExternalSubtitle{
			Path: filepath.Join(directory, entry.Name()), FileName: entry.Name(), Format: ext,
			Language: language, Default: isDefault, Forced: forced, SizeBytes: info.Size(), ModifiedAt: info.ModTime(),
		})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].FileName < values[j].FileName })
	return values, nil
}

func externalSubtitleNameForStem(name, stem string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".srt" && ext != ".ass" {
		return false
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return base == stem || strings.HasPrefix(base, stem+".")
}

func validatedExternalSubtitlePath(mediaPath, requested string) (string, error) {
	requested = filepath.Clean(strings.TrimSpace(requested))
	if requested == "." || requested == "" {
		return "", fmt.Errorf("subtitlePath is required")
	}
	if filepath.Dir(requested) != filepath.Dir(mediaPath) ||
		!externalSubtitleNameForStem(filepath.Base(requested), strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))) {
		return "", fmt.Errorf("subtitlePath must be an SRT or ASS sidecar belonging to this asset")
	}
	return requested, nil
}

func safeSubtitleFilenamePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Trim(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value), "-")
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
		TrackProfileKey:                strings.TrimSpace(input.TrackProfileKey),
		KeepVideoStreams:               normalizedStreamIndexes(input.KeepVideoStreams),
		KeepAudioStreams:               normalizedStreamIndexes(input.KeepAudioStreams),
		KeepSubtitleStreams:            normalizedStreamIndexes(input.KeepSubtitleStreams),
		VideoMetadata:                  normalizedStreamMetadata(input.VideoMetadata),
		AudioMetadata:                  normalizedStreamMetadata(input.AudioMetadata),
		SubtitleMetadata:               normalizedStreamMetadata(input.SubtitleMetadata),
		SubtitleTransforms:             normalizedSubtitleTransforms(input.SubtitleTransforms),
		VideoCodec:                     strings.TrimSpace(input.VideoCodec),
		AudioCodec:                     strings.TrimSpace(input.AudioCodec),
		QualityMode:                    strings.TrimSpace(input.QualityMode),
		QualityValue:                   input.QualityValue,
		VideoPreset:                    strings.TrimSpace(input.VideoPreset),
		PixFmt:                         strings.TrimSpace(input.PixFmt),
		VideoFilters:                   strings.TrimSpace(input.VideoFilters),
		DeinterlaceMode:                strings.TrimSpace(input.DeinterlaceMode),
		X265Params:                     strings.TrimSpace(input.X265Params),
		ProcessingMode:                 strings.TrimSpace(input.ProcessingMode),
		PreserveHDR:                    input.PreserveHDR,
		PreserveSubtitles:              input.PreserveSubtitles,
		PreserveChapters:               input.PreserveChapters,
		AddAACStereoTrack:              input.AddAACStereoTrack,
		AACStereoDefault:               input.AACStereoDefault,
		EnhancedAudioSourceStreamIndex: normalizedOptionalStreamIndex(input.EnhancedAudioSourceStreamIndex),
		UseHardwareIfAvailable:         input.UseHardwareIfAvailable,
		VideoEncoder:                   strings.TrimSpace(input.VideoEncoder),
		GlobalQuality:                  input.GlobalQuality,
		QSVRateControl:                 strings.TrimSpace(input.QSVRateControl),
		QSVLookAheadDepth:              input.QSVLookAheadDepth,
		QSVExtendedBRC:                 input.QSVExtendedBRC,
		QSVAdaptiveI:                   input.QSVAdaptiveI,
		QSVAdaptiveB:                   input.QSVAdaptiveB,
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

	allowed, err := h.pathBelongsToReadableMediaRoot(path)
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
	qsvRateControlOverride := strings.TrimSpace(c.Query("qsvRateControl"))
	qsvLookAheadDepthOverride, _ := strconv.Atoi(c.Query("qsvLookAheadDepth"))
	qsvExtendedBRCOverride, _ := strconv.ParseBool(c.Query("qsvExtendedBRC"))
	qsvAdaptiveIOverride, _ := strconv.ParseBool(c.Query("qsvAdaptiveI"))
	qsvAdaptiveBOverride, _ := strconv.ParseBool(c.Query("qsvAdaptiveB"))
	subtitleStreamIndex := -1
	if rawSubtitleIndex := strings.TrimSpace(c.Query("subtitleStreamIndex")); rawSubtitleIndex != "" {
		parsed, parseErr := strconv.Atoi(rawSubtitleIndex)
		if parseErr != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subtitleStreamIndex must be a non-negative stream index"})
			return
		}
		subtitleStreamIndex = parsed
	}
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

	allowed, err := h.pathBelongsToReadableMediaRoot(path)
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
	inputStart := start
	subtitlePrerollSeconds := 0
	if subtitleStreamIndex >= 0 {
		startSeconds := previewTimestampSeconds(start)
		subtitlePrerollSeconds = min(10, startSeconds)
		inputStart = secondsToTimestamp(startSeconds - subtitlePrerollSeconds)
	}
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-analyzeduration", analyzeSize,
		"-probesize", analyzeSize,
		"-ss", inputStart,
		"-i", path,
	}
	if subtitlePrerollSeconds > 0 {
		args = append(args, "-ss", strconv.Itoa(subtitlePrerollSeconds))
	}
	args = append(args, "-t", strconv.Itoa(seconds))
	videoFilter := previewVideoFilterChain(videoFiltersOverride, previewMode)
	if subtitleStreamIndex >= 0 {
		filter, filterErr := previewSubtitleFilter(path, subtitleStreamIndex, videoFilter)
		if filterErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": filterErr.Error()})
			return
		}
		args = append(args, "-filter_complex", filter, "-map", "[preview_video]")
	} else {
		args = append(args, "-map", "0:v:0?", "-vf", videoFilter)
	}
	args = append(args, previewVideoCodecArgs(
		h.db, profileID, videoCodecOverride, qualityValueOverride, videoPresetOverride, pixFmtOverride, x265ParamsOverride,
		videoEncoderOverride, useHardwareOverride, globalQualityOverride,
		qsvRateControlOverride, qsvLookAheadDepthOverride, qsvExtendedBRCOverride, qsvAdaptiveIOverride, qsvAdaptiveBOverride,
	)...)
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
	globalQuality := defaultQSVQuality(qualityValue)
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
	if len(hardwareOverrides) > 3 {
		profile.WorkerConfig["qsvRateControl"], _ = hardwareOverrides[3].(string)
	}
	if len(hardwareOverrides) > 4 {
		profile.WorkerConfig["qsvLookAheadDepth"], _ = hardwareOverrides[4].(int)
	}
	for index, key := range []string{"qsvExtendedBRC", "qsvAdaptiveI", "qsvAdaptiveB"} {
		if len(hardwareOverrides) > index+5 {
			profile.WorkerConfig[key], _ = hardwareOverrides[index+5].(bool)
		}
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

func previewSubtitleFilter(path string, streamIndex int, videoFilter string) (string, error) {
	inventory, err := probeMediaStreams(path)
	if err != nil {
		return "", fmt.Errorf("subtitle preview scan failed: %w", err)
	}
	subtitleOrdinal := -1
	subtitleCodec := ""
	for ordinal, stream := range inventory.Subtitle {
		if stream.Index == streamIndex {
			subtitleOrdinal = ordinal
			subtitleCodec = strings.ToLower(stream.Codec)
			break
		}
	}
	if subtitleOrdinal < 0 {
		return "", fmt.Errorf("subtitle stream %d is not present in the selected asset", streamIndex)
	}
	if strings.TrimSpace(videoFilter) == "" {
		videoFilter = "null"
	}
	beforeCrop, cropAndAfter := splitPreviewFiltersAtCrop(videoFilter)
	switch subtitleCodec {
	case "subrip", "srt", "ass", "ssa", "mov_text", "webvtt", "text":
		if cropAndAfter != "" {
			return fmt.Sprintf(
				"[0:v:0]%s,subtitles=filename='%s':si=%d[preview_subbed];[preview_subbed]%s[preview_video]",
				beforeCrop,
				escapeSubtitleFilterPath(path),
				subtitleOrdinal,
				cropAndAfter,
			), nil
		}
		return fmt.Sprintf(
			"[0:v:0]%s,subtitles=filename='%s':si=%d[preview_video]",
			videoFilter,
			escapeSubtitleFilterPath(path),
			subtitleOrdinal,
		), nil
	default:
		if cropAndAfter != "" {
			return fmt.Sprintf(
				"[0:v:0]%s[preview_base];[preview_base][0:%d]overlay[preview_subbed];[preview_subbed]%s[preview_video]",
				beforeCrop,
				streamIndex,
				cropAndAfter,
			), nil
		}
		return fmt.Sprintf(
			"[0:v:0]%s[preview_base];[preview_base][0:%d]overlay[preview_video]",
			videoFilter,
			streamIndex,
		), nil
	}
}

func splitPreviewFiltersAtCrop(filterChain string) (string, string) {
	filters := strings.Split(filterChain, ",")
	for index, filter := range filters {
		if strings.HasPrefix(strings.TrimSpace(filter), "crop=") {
			before := strings.TrimSpace(strings.Join(filters[:index], ","))
			after := strings.TrimSpace(strings.Join(filters[index:], ","))
			if before == "" {
				before = "null"
			}
			return before, after
		}
	}
	return filterChain, ""
}

func escapeSubtitleFilterPath(path string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`:`, `\:`,
		`'`, `\'`,
		`,`, `\,`,
		`[`, `\[`,
		`]`, `\]`,
	).Replace(path)
}

func (h AssetHandler) AudioPreview(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	profileKey := strings.TrimSpace(c.Query("profileKey"))
	filterOverride := strings.TrimSpace(c.Query("filters"))
	compatibilityPreview, _ := strconv.ParseBool(c.Query("compatibility"))
	audioMap := "0:a:0?"
	if rawStreamIndex := strings.TrimSpace(c.Query("streamIndex")); rawStreamIndex != "" {
		streamIndex, err := strconv.Atoi(rawStreamIndex)
		if err != nil || streamIndex < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "streamIndex must be a non-negative stream index"})
			return
		}
		audioMap = fmt.Sprintf("0:%d?", streamIndex)
	}
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

	allowed, err := h.pathBelongsToReadableMediaRoot(path)
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
		"-map", audioMap,
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
		filter = strings.ReplaceAll(filter, "aresample=ocl=", "aresample=ochl=")
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

func previewTimestampSeconds(value string) int {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0
	}
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])
	return max(0, hours*3600+minutes*60+seconds)
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

func (h AssetHandler) pathBelongsToReadableMediaRoot(mediaPath string) (bool, error) {
	allowed, err := h.pathBelongsToLibrary(mediaPath)
	if err != nil || allowed {
		return allowed, err
	}

	archiveRoot, err := originalsArchivePath(h.db)
	if err != nil {
		return false, err
	}
	return pathIsInside(mediaPath, archiveRoot), nil
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
	scans := []models.ScanResult{}
	if h.db.Migrator().HasTable(&models.ScanResult{}) {
		if err := h.db.Order("updated_at desc, id desc").Find(&scans).Error; err != nil {
			return AssetInventory{}, err
		}
	}
	technicalByPath := map[string]*AssetTechnicalInfo{}
	for _, scan := range scans {
		path := filepath.Clean(scan.Path)
		if _, exists := technicalByPath[path]; exists {
			continue
		}
		technicalByPath[path] = &AssetTechnicalInfo{
			VideoCodec: scan.VideoCodec,
			Width:      scan.Width,
			Height:     scan.Height,
			Duration:   scan.Duration,
			Bitrate:    scan.Bitrate,
			HDR:        scan.HDR,
		}
	}

	inventory := AssetInventory{
		Unprocessed:       []Asset{},
		Library:           []Asset{},
		Converted:         []Asset{},
		Unverified:        []Asset{},
		Archive:           []Asset{},
		Missing:           []Asset{},
		UnprocessedGroups: []AssetGroup{},
		LibraryGroups:     []AssetGroup{},
		ConvertedGroups:   []AssetGroup{},
		UnverifiedGroups:  []AssetGroup{},
		ArchiveGroups:     []AssetGroup{},
	}
	mvForgeOutputs := mvForgeOutputPaths(h.db)
	directOutputs := directPublicationPaths(h.db)
	missing := classifyMissingRecords(records)
	for _, record := range records {
		cleanPath := filepath.Clean(record.Path)
		if record.Missing && missing.HistoricalPaths[cleanPath] {
			continue
		}
		asset := assetFromRecord(record)
		asset.Technical = technicalByPath[cleanPath]
		switch record.Status {
		case "converted":
			if directOutputs[cleanPath] {
				asset.Status = "published_as_is"
				asset.PublicationMode = "as_is"
				inventory.Library = append(inventory.Library, asset)
			} else if mvForgeOutputs[cleanPath] {
				asset.Status = "converted"
				inventory.Converted = append(inventory.Converted, asset)
				inventory.Library = append(inventory.Library, asset)
			} else {
				asset.Status = "unverified"
				inventory.Unverified = append(inventory.Unverified, asset)
				inventory.Library = append(inventory.Library, asset)
			}
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
	for _, record := range records {
		if record.Missing && !missing.HistoricalPaths[filepath.Clean(record.Path)] {
			inventory.Missing = append(inventory.Missing, assetFromRecord(record))
		}
	}
	sortAssets(inventory.Missing)
	inventory.Reports = assetReports(inventory, missing)

	var last models.AssetRecord
	if err := h.db.Order("synced_at desc").First(&last).Error; err == nil {
		inventory.Sync.LastSyncedAt = last.SyncedAt
	}
	_ = h.db.Model(&models.AssetRecord{}).Count(&inventory.Sync.TotalRecords).Error
	inventory.Sync.MissingFiles = int64(missing.Total)
	inventory.Sync.MissingActionable = missing.Actionable
	inventory.Sync.MissingHistorical = missing.Historical
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

func directPublicationPaths(db *gorm.DB) map[string]bool {
	paths := map[string]bool{}
	if !db.Migrator().HasTable(&models.DirectPublication{}) {
		return paths
	}
	var publications []models.DirectPublication
	_ = db.Where("returned_at IS NULL").Find(&publications).Error
	for _, publication := range publications {
		paths[filepath.Clean(publication.PublishedPath)] = true
	}
	return paths
}

func (h AssetHandler) syncAssetInventory() (AssetSyncResult, error) {
	// Manual sync, automatic sync, publication, and recovery can all request a
	// refresh. Keep the filesystem scan and missing-record reconciliation as
	// one process-wide operation.
	assetInventorySyncMu.Lock()
	defer assetInventorySyncMu.Unlock()

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
	libraryRecords := []models.AssetRecord{}
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
			libraryRecords = append(libraryRecords, record)
		}
	}
	result.ReconciledFiles, result.ReviewMatches, err = reconcileMovedPublishedAssets(h.db, libraryRecords)
	if err != nil {
		return result, err
	}
	mvForgeOutputs := mvForgeOutputPaths(h.db)
	for _, record := range libraryRecords {
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

func reconcileMovedPublishedAssets(db *gorm.DB, candidates []models.AssetRecord) (int, int, error) {
	mode := assetReconciliationMode(db)
	if mode == "off" {
		return 0, 0, nil
	}
	var jobs []models.QueueJob
	if err := db.Where("published_path <> ? AND publication_retired_at IS NULL", "").Order("id desc").Find(&jobs).Error; err != nil {
		return 0, 0, err
	}
	claimed := map[string]bool{}
	for _, job := range jobs {
		if job.PublishedSizeBytes == 0 {
			var previous models.AssetRecord
			if err := db.Where("path = ?", job.PublishedPath).First(&previous).Error; err == nil && previous.SizeBytes > 0 {
				job.PublishedSizeBytes = previous.SizeBytes
				_ = db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Update("published_size_bytes", previous.SizeBytes).Error
			}
		}
		if info, err := os.Stat(job.PublishedPath); err == nil && !info.IsDir() {
			claimed[filepath.Clean(job.PublishedPath)] = true
			if job.PublishedFingerprint == "" || job.PublishedSizeBytes != info.Size() {
				if fingerprint, fingerprintErr := mediaFileFingerprint(job.PublishedPath); fingerprintErr == nil {
					job.PublishedFingerprint = fingerprint
					job.PublishedSizeBytes = info.Size()
					_ = db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{
						"published_fingerprint": fingerprint,
						"published_size_bytes":  info.Size(),
					}).Error
				}
			}
		}
	}

	fingerprintCache := map[string]string{}
	reconciled, reviewMatches := 0, 0
	reviews := assetReviewOverrides(db)
	reviewsChanged := false
	for _, job := range jobs {
		if claimed[filepath.Clean(job.PublishedPath)] {
			continue
		}
		matches := []models.AssetRecord{}
		for _, candidate := range candidates {
			clean := filepath.Clean(candidate.Path)
			if claimed[clean] {
				continue
			}
			if job.PublishedFingerprint != "" {
				if job.PublishedSizeBytes > 0 && candidate.SizeBytes != job.PublishedSizeBytes {
					continue
				}
				fingerprint, ok := fingerprintCache[clean]
				if !ok {
					fingerprint, _ = mediaFileFingerprint(candidate.Path)
					fingerprintCache[clean] = fingerprint
				}
				if fingerprint == job.PublishedFingerprint {
					matches = append(matches, candidate)
				}
				continue
			}
			legacyIdentity := legacyMediaPathIdentity(job.PublishedPath)
			if legacyIdentity != "" && job.PublishedSizeBytes > 0 && candidate.SizeBytes == job.PublishedSizeBytes &&
				legacyMediaPathIdentity(candidate.Path) == legacyIdentity {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 1 && job.PublishedFingerprint != "" && mode == "exact" {
			match := matches[0]
			oldPath := filepath.Clean(job.PublishedPath)
			if err := db.Transaction(func(tx *gorm.DB) error {
				job.PublishedPath = match.Path
				job.LibraryID = match.LibraryID
				job.Notes = appendNote(job.Notes, "Sync reconciled externally relocated converted asset: "+oldPath+" -> "+match.Path)
				if err := tx.Save(&job).Error; err != nil {
					return err
				}
				return migrateSingleAssetPathOverrides(tx, oldPath, match.Path)
			}); err != nil {
				return reconciled, reviewMatches, err
			}
			claimed[filepath.Clean(match.Path)] = true
			reconciled++
			continue
		}
		if len(matches) > 0 {
			for _, match := range matches {
				reviews[filepath.Clean(match.Path)] = AssetReviewState{
					RequiresReview: true,
					Reason:         fmt.Sprintf("Possible relocated publication for job %d; confirm before reconciliation.", job.ID),
					Source:         "sync-reconciliation",
					Tags:           []string{"relocated-publication", fmt.Sprintf("reconciliation-job-%d", job.ID)},
					UpdatedAt:      time.Now(),
				}
				reviewMatches++
				reviewsChanged = true
			}
		}
	}
	if reviewsChanged {
		if err := saveAssetReviewOverrides(db, reviews); err != nil {
			return reconciled, reviewMatches, err
		}
	}
	return reconciled, reviewMatches, nil
}

func legacyMediaPathIdentity(mediaPath string) string {
	if identity := missingMediaIdentity(filepath.Base(mediaPath)); identity != "" {
		return identity
	}
	return missingMediaIdentity(filepath.Base(filepath.Dir(mediaPath)))
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func migrateSingleAssetPathOverrides(db *gorm.DB, oldPath, newPath string) error {
	conversion := assetConversionOverrides(db)
	if value, exists := conversion[oldPath]; exists {
		delete(conversion, oldPath)
		conversion[newPath] = value
		if err := saveAssetConversionOverrides(db, conversion); err != nil {
			return err
		}
	}
	reviews := assetReviewOverrides(db)
	if value, exists := reviews[oldPath]; exists {
		delete(reviews, oldPath)
		reviews[newPath] = value
		if err := saveAssetReviewOverrides(db, reviews); err != nil {
			return err
		}
	}
	metadata := assetMetadataOverrides(db)
	if value, exists := metadata[oldPath]; exists {
		delete(metadata, oldPath)
		metadata[newPath] = value
		return saveAssetMetadataOverrides(db, metadata)
	}
	return nil
}

func assetReconciliationMode(db *gorm.DB) string {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "assetInventory").Error; err == nil {
		if mode := strings.ToLower(strings.TrimSpace(stringSetting(setting.Value, "reconciliationMode", ""))); mode == "off" || mode == "review" || mode == "exact" {
			return mode
		}
	}
	return "exact"
}

func mediaFileFingerprint(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d:", info.Size())
	const sampleSize int64 = 1 << 20
	offsets := []int64{0}
	if info.Size() > sampleSize*2 {
		offsets = append(offsets, info.Size()/2-sampleSize/2)
	}
	if info.Size() > sampleSize {
		offsets = append(offsets, info.Size()-sampleSize)
	}
	buffer := make([]byte, sampleSize)
	for _, offset := range offsets {
		count, readErr := file.ReadAt(buffer, offset)
		if readErr != nil && readErr != io.EOF {
			return "", readErr
		}
		if _, err := hash.Write(buffer[:count]); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
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
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"root_path", "relative_path", "group_path", "file_name", "extension",
			"size_bytes", "modified_at", "status", "library_id", "library_name",
			"missing", "expires_at", "synced_at", "updated_at",
		}),
	}).Create(&record).Error
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

type MissingClassification struct {
	Total           int
	Actionable      int
	Historical      int
	HistoricalPaths map[string]bool
}

func classifyMissingRecords(records []models.AssetRecord) MissingClassification {
	presentSizes := map[int64]struct{}{}
	presentConvertedIdentities := map[string]struct{}{}
	presentArchiveIdentities := map[string]struct{}{}
	for _, record := range records {
		if record.Missing {
			continue
		}
		if record.SizeBytes > 0 {
			presentSizes[record.SizeBytes] = struct{}{}
		}
		if record.Status == "converted" || record.Status == "archive" {
			presentConvertedIdentities[missingMediaIdentity(record.FileName)] = struct{}{}
		}
		if record.Status == "archive" {
			presentArchiveIdentities[missingMediaIdentity(record.FileName)] = struct{}{}
		}
	}

	classification := MissingClassification{HistoricalPaths: map[string]bool{}}
	for _, record := range records {
		if !record.Missing {
			continue
		}
		classification.Total++
		_, sameSizeExists := presentSizes[record.SizeBytes]
		_, replacementExists := presentConvertedIdentities[missingMediaIdentity(record.FileName)]
		_, archivedOriginalExists := presentArchiveIdentities[missingMediaIdentity(record.FileName)]
		if sameSizeExists || (record.Status == "converted" && replacementExists) || (record.Status == "unprocessed" && archivedOriginalExists) {
			classification.Historical++
			classification.HistoricalPaths[filepath.Clean(record.Path)] = true
		} else {
			classification.Actionable++
		}
	}
	return classification
}

func missingMediaIdentity(fileName string) string {
	stem := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(fileName), filepath.Ext(fileName)))
	stem = strings.NewReplacer(
		"-new", "", "_ttitle", "", "-ttitle", "",
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	).Replace(stem)
	var normalized strings.Builder
	for _, character := range stem {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			normalized.WriteRune(character)
		} else {
			normalized.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(normalized.String()), " ")
}

func assetReports(inventory AssetInventory, missing MissingClassification) AssetReports {
	report := AssetReports{
		UnprocessedFiles:  len(inventory.Unprocessed),
		LibraryFiles:      len(inventory.Library),
		ConvertedFiles:    len(inventory.Converted),
		UnverifiedFiles:   len(inventory.Unverified),
		ArchiveFiles:      len(inventory.Archive),
		MissingFiles:      missing.Total,
		MissingActionable: missing.Actionable,
		MissingHistorical: missing.Historical,
	}
	now := time.Now()
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

func resetRecoveredAssetOverrides(db *gorm.DB, paths ...string) error {
	targets := map[string]struct{}{}
	for _, path := range paths {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean != "." && clean != "" {
			targets[clean] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		conversion := assetConversionOverrides(tx)
		conversionChanged := false
		for path := range targets {
			if _, exists := conversion[path]; exists {
				delete(conversion, path)
				conversionChanged = true
			}
		}
		if conversionChanged {
			if err := saveAssetConversionOverrides(tx, conversion); err != nil {
				return err
			}
		}

		reviews := assetReviewOverrides(tx)
		reviewsChanged := false
		for path := range targets {
			if _, exists := reviews[path]; exists {
				delete(reviews, path)
				reviewsChanged = true
			}
		}
		if reviewsChanged {
			if err := saveAssetReviewOverrides(tx, reviews); err != nil {
				return err
			}
		}
		return nil
	})
}

func returnDirectPublicationOverrides(db *gorm.DB, publishedPath, restorePath string) error {
	publishedPath = filepath.Clean(publishedPath)
	restorePath = filepath.Clean(restorePath)
	conversion := assetConversionOverrides(db)
	delete(conversion, publishedPath)
	delete(conversion, restorePath)
	if err := saveAssetConversionOverrides(db, conversion); err != nil {
		return err
	}
	reviews := assetReviewOverrides(db)
	delete(reviews, publishedPath)
	delete(reviews, restorePath)
	if err := saveAssetReviewOverrides(db, reviews); err != nil {
		return err
	}
	metadata := assetMetadataOverrides(db)
	if value, exists := metadata[publishedPath]; exists {
		delete(metadata, publishedPath)
		metadata[restorePath] = value
	}
	return saveAssetMetadataOverrides(db, metadata)
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
	override.SubtitleTransforms = normalizedSubtitleTransforms(override.SubtitleTransforms)
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
		len(override.SubtitleTransforms) == 0 &&
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
		override.AACStereoDefault == nil &&
		override.EnhancedAudioSourceStreamIndex == nil &&
		override.UseHardwareIfAvailable == nil &&
		strings.TrimSpace(override.VideoEncoder) == "" &&
		override.GlobalQuality == 0 &&
		strings.TrimSpace(override.QSVRateControl) == "" &&
		override.QSVLookAheadDepth == 0 &&
		override.QSVExtendedBRC == nil &&
		override.QSVAdaptiveI == nil &&
		override.QSVAdaptiveB == nil
}

func normalizedSubtitleTransforms(values []SubtitleTransform) []SubtitleTransform {
	if len(values) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	result := make([]SubtitleTransform, 0, len(values))
	for _, value := range values {
		format := strings.ToLower(strings.TrimSpace(value.Format))
		if value.StreamIndex < 0 || (format != "srt" && format != "ass") {
			continue
		}
		if _, exists := seen[value.StreamIndex]; exists {
			continue
		}
		seen[value.StreamIndex] = struct{}{}
		value.Format = format
		value.Language = safeSubtitleFilenamePart(value.Language)
		if value.Language == "" {
			value.Language = "und"
		}
		result = append(result, value)
	}
	return result
}

func normalizedOptionalStreamIndex(index *int) *int {
	if index == nil || *index < 0 {
		return nil
	}
	value := *index
	return &value
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
