package handlers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	PublishModeStandard       = "standard"
	PublishModeReplaceLibrary = "replace_library_asset"
)

// Publishing can be triggered manually while the automatic publisher is also
// reconciling jobs. Serialize original archival so two publishers cannot both
// copy the same source across different mounts before either removes it.
var originalArchiveMu sync.Mutex

type PublisherHandler struct {
	db *gorm.DB
}

type PublishJobInput struct {
	Overwrite bool `json:"overwrite"`
}

type PublishResult struct {
	JobID         uint   `json:"jobId"`
	Status        string `json:"status"`
	SourcePath    string `json:"sourcePath"`
	PublishedPath string `json:"publishedPath"`
	Message       string `json:"message"`
}

type publishError struct {
	Status  int
	Message string
	Err     error
}

func (e publishError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "publish failed"
}

func NewPublisherHandler(db *gorm.DB) PublisherHandler {
	return PublisherHandler{db: db}
}

func (h PublisherHandler) PublishJob(c *gin.Context) {
	var input PublishJobInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var job models.QueueJob
	if err := h.db.First(&job, c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, err := h.publishQueueJob(job, input.Overwrite)
	if err != nil {
		status := http.StatusInternalServerError
		if publishErr, ok := err.(publishError); ok && publishErr.Status > 0 {
			status = publishErr.Status
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h PublisherHandler) publishQueueJob(job models.QueueJob, overwrite bool) (PublishResult, error) {
	if job.Status != JobStatusCompleted {
		return PublishResult{}, publishError{Status: http.StatusBadRequest, Message: "job must be completed before publishing"}
	}
	if job.ValidationStatus != ValidationStatusPassed && job.ValidationStatus != ValidationStatusWarning {
		return PublishResult{}, publishError{Status: http.StatusBadRequest, Message: "job must be validated before publishing"}
	}
	if job.OutputPath == "" {
		return PublishResult{}, publishError{Status: http.StatusBadRequest, Message: "job output path is required"}
	}

	if _, err := os.Stat(job.OutputPath); err != nil {
		return PublishResult{}, publishError{Status: http.StatusNotFound, Message: "job output is not readable from the backend container", Err: err}
	}

	var library models.Library
	if err := h.db.First(&library, job.LibraryID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return PublishResult{}, publishError{Status: http.StatusNotFound, Message: "library not found", Err: err}
		}

		return PublishResult{}, err
	}

	var profile models.Profile
	if len(job.ProfileSnapshot) > 0 {
		var err error
		profile, err = scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
		if err != nil {
			return PublishResult{}, err
		}
	} else if err := h.db.First(&profile, job.ProfileID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return PublishResult{}, publishError{Status: http.StatusNotFound, Message: "profile not found", Err: err}
		}
		return PublishResult{}, err
	}

	destinationPath := strings.TrimSpace(job.PlannedPublishedPath)
	if destinationPath == "" {
		destinationPath = plannedOutputPathForJob(h.db, job, library, profile)
		job.PlannedPublishedPath = destinationPath
	}
	if job.PublishMode == PublishModeReplaceLibrary {
		return h.publishLibraryReplacement(job, library, profile)
	}
	if err := transitionJobStage(h.db, &job, JobStagePublishing); err != nil {
		return PublishResult{}, err
	}
	if path.Clean(job.OutputPath) != path.Clean(destinationPath) {
		alreadyPublished := false
		if !overwrite {
			if _, err := os.Stat(destinationPath); err == nil {
				equal, compareErr := filesEqual(job.OutputPath, destinationPath)
				if compareErr != nil {
					return PublishResult{}, compareErr
				}
				if !equal {
					return PublishResult{}, publishError{Status: http.StatusConflict, Message: "destination exists with different content"}
				}
				alreadyPublished = true
			}
		}
		if !alreadyPublished {
			if err := copyPublishedFile(job.OutputPath, destinationPath, overwrite); err != nil {
				status := http.StatusInternalServerError
				if os.IsExist(err) {
					status = http.StatusConflict
				}
				return PublishResult{}, publishError{Status: status, Err: err}
			}
		}
	} else if !overwrite {
		if _, err := os.Stat(destinationPath); err != nil {
			return PublishResult{}, publishError{Status: http.StatusNotFound, Message: "published output path is not readable", Err: err}
		}
	}
	if _, err := copyExternalSubtitleSidecars(job.MediaPath, destinationPath); err != nil {
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "video output is ready but existing subtitle sidecars could not be copied to Library; original was not archived", Err: err}
	}
	if _, err := publishSubtitleArtifacts(&job, destinationPath, overwrite); err != nil {
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "video output is ready but subtitle sidecars could not be published; original was not archived", Err: err}
	}

	if err := transitionJobStage(h.db, &job, JobStageArchivingOriginal); err != nil {
		return PublishResult{}, err
	}
	if archivedPath, err := h.archivePublishedOriginal(job); err != nil {
		job.Notes = appendNote(job.Notes, "Original archive failed: "+err.Error())
		_ = h.db.Save(&job).Error
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "output copied but original could not be archived; publication will be retried", Err: err}
	} else if archivedPath != "" {
		job.Notes = appendNote(job.Notes, "Original archived: "+archivedPath)
		job.OriginalArchivedPath = archivedPath
	}

	now := time.Now()
	job.PublishedPath = destinationPath
	job.PublishedAt = &now
	if info, statErr := os.Stat(destinationPath); statErr == nil {
		if fingerprint, fingerprintErr := mediaFileFingerprint(destinationPath); fingerprintErr == nil {
			job.PublishedFingerprint = fingerprint
			job.PublishedSizeBytes = info.Size()
		} else {
			job.Notes = appendNote(job.Notes, "Published fingerprint warning: "+fingerprintErr.Error())
		}
	}
	if err := transitionJobStage(h.db, &job, JobStageAnalyzingFinal); err != nil {
		return PublishResult{}, err
	}
	if _, snapshotErr := captureFinalAssetSnapshot(h.db, destinationPath); snapshotErr != nil {
		job.Notes = appendNote(job.Notes, "Final asset snapshot warning: "+snapshotErr.Error())
	} else {
		job.Notes = appendNote(job.Notes, "Final asset snapshot captured: "+destinationPath)
	}
	if syncErr := syncPublishedAssetRecords(h.db, job, library); syncErr != nil {
		job.Notes = appendNote(job.Notes, "Incremental asset inventory sync warning: "+syncErr.Error())
		appendSystemLog(h.db, "published_asset_incremental_sync_failed", map[string]string{"jobId": strconv.FormatUint(uint64(job.ID), 10), "publishedPath": destinationPath}, syncErr)
	} else {
		job.Notes = appendNote(job.Notes, "Asset inventory synchronized incrementally")
	}
	if path.Clean(job.OutputPath) != path.Clean(destinationPath) {
		if err := transitionJobStage(h.db, &job, JobStageCleaningWorkspace); err != nil {
			return PublishResult{}, err
		}
		if err := h.cleanupStagedJob(job); err != nil {
			job.Notes = appendNote(job.Notes, "Staged output cleanup warning: "+err.Error())
		}
	}

	_ = h.db.Save(&job).Error
	_ = transitionJobStage(h.db, &job, JobStageCompleted)
	_ = scheduler.ReleaseReservation(h.db, job.ID)

	return PublishResult{
		JobID:         job.ID,
		Status:        "published",
		SourcePath:    job.OutputPath,
		PublishedPath: destinationPath,
		Message:       "Output published to destination library.",
	}, nil
}

func (h PublisherHandler) publishLibraryReplacement(job models.QueueJob, library models.Library, profile models.Profile) (PublishResult, error) {
	if !pathIsInside(job.MediaPath, library.DestinationPath) {
		return PublishResult{}, publishError{Status: http.StatusBadRequest, Message: "library replacement source is outside the selected library destination"}
	}
	target := outputFileRelativePath(job.MediaPath, profile)
	if path.Clean(target) != path.Clean(job.MediaPath) {
		if _, err := os.Stat(target); err == nil {
			return PublishResult{}, publishError{Status: http.StatusConflict, Message: "replacement destination already exists"}
		}
	}
	if err := transitionJobStage(h.db, &job, JobStagePublishing); err != nil {
		return PublishResult{}, err
	}
	temporary := filepath.Join(filepath.Dir(target), fmt.Sprintf(".%s.mvforge-job-%d.tmp", filepath.Base(target), job.ID))
	if err := copyPublishedFile(job.OutputPath, temporary, true); err != nil {
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "could not prepare replacement beside library asset", Err: err}
	}
	equal, err := filesEqual(job.OutputPath, temporary)
	if err != nil || !equal {
		_ = os.Remove(temporary)
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "prepared replacement failed integrity verification", Err: err}
	}
	if err := transitionJobStage(h.db, &job, JobStageArchivingOriginal); err != nil {
		_ = os.Remove(temporary)
		return PublishResult{}, err
	}
	archivedPath, err := h.libraryOriginalArchivePath(job, library)
	if err != nil {
		_ = os.Remove(temporary)
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "original library asset could not be archived", Err: err}
	}
	job.ReplacementTargetPath = target
	job.OriginalArchivedPath = archivedPath
	if err := h.db.Save(&job).Error; err != nil {
		_ = os.Remove(temporary)
		return PublishResult{}, err
	}
	if err := moveFile(job.MediaPath, archivedPath); err != nil {
		_ = os.Remove(temporary)
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "original library asset could not be archived", Err: err}
	}
	if err := os.Rename(temporary, target); err != nil {
		rollbackErr := moveFile(archivedPath, job.MediaPath)
		if rollbackErr != nil {
			return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "replacement failed and original rollback also failed", Err: fmt.Errorf("replace: %v; rollback: %v", err, rollbackErr)}
		}
		job.ReplacementTargetPath = ""
		job.OriginalArchivedPath = ""
		_ = h.db.Save(&job).Error
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "replacement failed; original was restored", Err: err}
	}
	publishedSubtitles, subtitleErr := publishSubtitleArtifacts(&job, target, false)
	if subtitleErr != nil {
		_ = os.Remove(target)
		for _, published := range publishedSubtitles {
			_ = os.Remove(published)
		}
		rollbackErr := moveFile(archivedPath, job.MediaPath)
		if rollbackErr != nil {
			return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "subtitle publication failed and original rollback also failed", Err: fmt.Errorf("subtitle publish: %v; rollback: %v", subtitleErr, rollbackErr)}
		}
		job.ReplacementTargetPath = ""
		job.OriginalArchivedPath = ""
		_ = h.db.Save(&job).Error
		return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: "subtitle publication failed; original library asset was restored", Err: subtitleErr}
	}
	job.Notes = appendNote(job.Notes, "Library original archived: "+archivedPath)
	job.PublishedPath = target
	now := time.Now()
	job.PublishedAt = &now
	if info, statErr := os.Stat(target); statErr == nil {
		if fingerprint, fingerprintErr := mediaFileFingerprint(target); fingerprintErr == nil {
			job.PublishedFingerprint = fingerprint
			job.PublishedSizeBytes = info.Size()
		} else {
			job.Notes = appendNote(job.Notes, "Published fingerprint warning: "+fingerprintErr.Error())
		}
	}
	if err := transitionJobStage(h.db, &job, JobStageAnalyzingFinal); err != nil {
		return PublishResult{}, err
	}
	if _, snapshotErr := captureFinalAssetSnapshot(h.db, target); snapshotErr != nil {
		job.Notes = appendNote(job.Notes, "Final asset snapshot warning: "+snapshotErr.Error())
	} else {
		job.Notes = appendNote(job.Notes, "Final asset snapshot captured: "+target)
	}
	if syncErr := syncPublishedAssetRecords(h.db, job, library); syncErr != nil {
		job.Notes = appendNote(job.Notes, "Incremental asset inventory sync warning: "+syncErr.Error())
		appendSystemLog(h.db, "published_asset_incremental_sync_failed", map[string]string{"jobId": strconv.FormatUint(uint64(job.ID), 10), "publishedPath": target}, syncErr)
	} else {
		job.Notes = appendNote(job.Notes, "Asset inventory synchronized incrementally")
	}
	if err := h.db.Save(&job).Error; err != nil {
		return PublishResult{}, err
	}
	if err := transitionJobStage(h.db, &job, JobStageCleaningWorkspace); err == nil {
		if cleanupErr := h.cleanupStagedJob(job); cleanupErr != nil {
			job.Notes = appendNote(job.Notes, "Staged output cleanup warning: "+cleanupErr.Error())
		}
	}
	_ = h.db.Save(&job).Error
	_ = transitionJobStage(h.db, &job, JobStageCompleted)
	_ = scheduler.ReleaseReservation(h.db, job.ID)
	return PublishResult{JobID: job.ID, Status: "published", SourcePath: job.OutputPath, PublishedPath: target, Message: "Library asset safely replaced; original archived."}, nil
}

func syncPublishedAssetRecords(db *gorm.DB, job models.QueueJob, library models.Library) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	now := time.Now()
	published, err := assetRecordForPublishedPath(job.PublishedPath, library.DestinationPath, "converted", library, now, 0)
	if err != nil {
		return fmt.Errorf("index published asset: %w", err)
	}

	var archived *models.AssetRecord
	if strings.TrimSpace(job.OriginalArchivedPath) != "" {
		archiveRoot, rootErr := originalsArchivePath(db)
		if rootErr != nil {
			return fmt.Errorf("resolve archive root: %w", rootErr)
		}
		archiveLibrary := models.Library{Name: "Originals"}
		record, recordErr := assetRecordForPublishedPath(job.OriginalArchivedPath, archiveRoot, "archive", archiveLibrary, now, originalRetentionDays(db))
		if recordErr != nil {
			return fmt.Errorf("index archived original: %w", recordErr)
		}
		archived = &record
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if sourcePath := filepath.Clean(strings.TrimSpace(job.MediaPath)); sourcePath != "." && sourcePath != filepath.Clean(job.PublishedPath) {
			if err := tx.Model(&models.AssetRecord{}).Where("path = ?", sourcePath).Updates(map[string]interface{}{"missing": true, "synced_at": now}).Error; err != nil {
				return err
			}
		}
		if err := upsertAssetRecord(tx, published); err != nil {
			return err
		}
		if archived != nil {
			if err := upsertAssetRecord(tx, *archived); err != nil {
				return err
			}
		}
		return nil
	})
}

func assetRecordForPublishedPath(mediaPath, root, status string, library models.Library, syncedAt time.Time, keepDays int) (models.AssetRecord, error) {
	mediaPath = filepath.Clean(strings.TrimSpace(mediaPath))
	root = filepath.Clean(strings.TrimSpace(root))
	if mediaPath == "." || root == "." || !pathIsInside(mediaPath, root) {
		return models.AssetRecord{}, fmt.Errorf("path %s is outside root %s", mediaPath, root)
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		return models.AssetRecord{}, err
	}
	if info.IsDir() {
		return models.AssetRecord{}, fmt.Errorf("media path is a directory")
	}
	relativePath := filepath.ToSlash(relativeAssetPath(root, mediaPath))
	record := models.AssetRecord{
		Path: mediaPath, RootPath: root, RelativePath: relativePath,
		GroupPath: filepath.ToSlash(logicalAssetGroupPath(relativePath)), FileName: filepath.Base(mediaPath),
		Extension: strings.ToLower(filepath.Ext(mediaPath)), SizeBytes: info.Size(), ModifiedAt: info.ModTime(),
		Status: status, LibraryID: library.ID, LibraryName: library.Name, Missing: false, SyncedAt: syncedAt,
	}
	if status == "archive" && keepDays > 0 {
		expiresAt := info.ModTime().Add(time.Duration(keepDays) * 24 * time.Hour)
		record.ExpiresAt = &expiresAt
	}
	return record, nil
}

func (h PublisherHandler) libraryOriginalArchivePath(job models.QueueJob, library models.Library) (string, error) {
	archiveRoot, err := originalsArchivePath(h.db)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(library.DestinationPath, job.MediaPath)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid library asset path")
	}
	destination := uniqueArchivePath(filepath.Join(archiveRoot, "library-replacements", fmt.Sprintf("library-%d", library.ID), relative))
	return destination, nil
}

func pathIsInside(candidate string, root string) bool {
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	return candidateAbs != rootAbs && isInsideRoot(rootAbs, candidateAbs)
}

func filesEqual(leftPath, rightPath string) (bool, error) {
	leftInfo, err := os.Stat(leftPath)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(rightPath)
	if err != nil {
		return false, err
	}
	if leftInfo.Size() != rightInfo.Size() {
		return false, nil
	}
	leftHash, err := fileSHA256(leftPath)
	if err != nil {
		return false, err
	}
	rightHash, err := fileSHA256(rightPath)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func fileSHA256(filePath string) ([sha256.Size]byte, error) {
	var result [sha256.Size]byte
	file, err := os.Open(filePath)
	if err != nil {
		return result, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return result, err
	}
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func (h PublisherHandler) cleanupStagedJob(job models.QueueJob) error {
	stagingRoot, err := settingPath(h.db, "stagingPath", "/media/staging")
	if err != nil {
		return err
	}
	if roles, roleErr := scheduler.LoadStorageRoles(h.db); roleErr == nil {
		if rolePath, pathErr := roles.Path(scheduler.StorageRoleWork); pathErr == nil {
			stagingRoot = rolePath
		}
	}
	root, err := filepath.Abs(stagingRoot)
	if err != nil {
		return err
	}
	output, err := filepath.Abs(job.OutputPath)
	if err != nil {
		return err
	}
	if !isInsideRoot(root, output) {
		return fmt.Errorf("refusing to remove staged output outside %s", root)
	}
	jobRoot := filepath.Join(root, fmt.Sprintf("job-%d", job.ID))
	if isInsideRoot(jobRoot, output) {
		return os.RemoveAll(jobRoot)
	}
	return fmt.Errorf("output is not inside expected job directory %s", jobRoot)
}

func copyPublishedFile(source string, destination string, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(destination); err == nil {
			return os.ErrExist
		}
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}

	return output.Sync()
}

func (h PublisherHandler) archivePublishedOriginal(job models.QueueJob) (string, error) {
	originalArchiveMu.Lock()
	defer originalArchiveMu.Unlock()

	if strings.TrimSpace(job.MediaPath) == "" {
		return "", nil
	}
	if _, err := os.Stat(job.MediaPath); err != nil {
		return "", nil
	}

	rawRoot, err := settingPath(h.db, "rawRoot", "/media/raw")
	if err != nil {
		return "", err
	}
	archiveRoot, err := originalsArchivePath(h.db)
	if err != nil {
		return "", err
	}

	relative := relativeMediaPath(job.MediaPath, rawRoot)
	destination := filepath.Join(archiveRoot, filepath.FromSlash(relative))
	destination = uniqueArchivePath(destination)
	if err := moveFile(job.MediaPath, destination); err != nil {
		return "", err
	}
	_ = h.cleanupEmptyOriginalDirs(filepath.Dir(job.MediaPath), rawRoot)

	return destination, nil
}

func (h PublisherHandler) cleanupEmptyOriginalDirs(startDir string, rawRoot string) error {
	rawRootAbs, err := filepath.Abs(rawRoot)
	if err != nil {
		return err
	}
	current, err := filepath.Abs(startDir)
	if err != nil {
		return err
	}
	if !isInsideRoot(rawRootAbs, current) || current == rawRootAbs {
		return nil
	}

	for {
		parent := filepath.Dir(current)
		if current == rawRootAbs || !isInsideRoot(rawRootAbs, current) {
			return nil
		}

		entries, err := os.ReadDir(current)
		if err != nil {
			if os.IsNotExist(err) {
				current = parent
				continue
			}
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(current); err != nil {
			return err
		}
		current = parent
	}
}

func originalsArchivePath(db *gorm.DB) (string, error) {
	roleArchiveRoot := ""
	if roles, err := scheduler.LoadStorageRoles(db); err == nil {
		if rolePath, pathErr := roles.Path(scheduler.StorageRoleOriginalsArchive); pathErr == nil {
			roleArchiveRoot = rolePath
		}
	}
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "originalRetentionPolicy").Error; err == nil && setting.Value != nil {
		values = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	pathsSetting := models.AppSetting{}
	configuredArchiveRoot := roleArchiveRoot
	if err := db.First(&pathsSetting, "key = ?", "paths").Error; err == nil && pathsSetting.Value != nil && configuredArchiveRoot == "" {
		configuredArchiveRoot = normalizedOriginalsArchivePath(strings.TrimSpace(stringFromUnknown(pathsSetting.Value["originalsArchivePath"])))
		if value := normalizedOriginalsArchivePath(strings.TrimSpace(stringFromUnknown(pathsSetting.Value["trashPath"]))); value != "" {
			configuredArchiveRoot = value
		}
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	if value := normalizedOriginalsArchivePath(strings.TrimSpace(stringFromUnknown(values["processedOriginalsPath"]))); value != "" {
		const legacyRoot = "/media/originals_archive"
		if configuredArchiveRoot != "" && strings.HasPrefix(value, legacyRoot+"/") {
			return filepath.Join(configuredArchiveRoot, strings.TrimPrefix(value, legacyRoot+"/")), nil
		}
		return value, nil
	}
	if configuredArchiveRoot != "" {
		return configuredArchiveRoot, nil
	}

	return "/media/originals_archive", nil
}

func normalizedOriginalsArchivePath(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "/media/trash") {
		return strings.Replace(value, "/media/trash", "/media/originals_archive", 1)
	}
	return value
}

func uniqueArchivePath(destination string) string {
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		return destination
	}

	extension := filepath.Ext(destination)
	base := strings.TrimSuffix(destination, extension)
	for index := 1; ; index++ {
		candidate := fmt.Sprintf("%s-%s-%d%s", base, time.Now().Format("20060102-150405"), index, extension)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func moveFile(source string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := copyPublishedFile(source, destination, true); err != nil {
		return err
	}
	return os.Remove(source)
}

func copyExternalSubtitleSidecars(sourceMediaPath string, destinationMediaPath string) ([]string, error) {
	sourceMediaPath = filepath.Clean(strings.TrimSpace(sourceMediaPath))
	destinationMediaPath = filepath.Clean(strings.TrimSpace(destinationMediaPath))
	if sourceMediaPath == "." || destinationMediaPath == "." || sourceMediaPath == destinationMediaPath {
		return nil, nil
	}
	sidecars, err := externalSubtitlesForMedia(sourceMediaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sourceBase := strings.TrimSuffix(sourceMediaPath, filepath.Ext(sourceMediaPath))
	destinationBase := strings.TrimSuffix(destinationMediaPath, filepath.Ext(destinationMediaPath))
	copied := make([]string, 0, len(sidecars))
	for _, sidecar := range sidecars {
		suffix := strings.TrimPrefix(sidecar.Path, sourceBase)
		if suffix == sidecar.Path || suffix == "" {
			continue
		}
		destination, alreadyPublished, resolveErr := resolveSidecarDestination(sidecar.Path, destinationBase, suffix)
		if resolveErr != nil {
			return copied, resolveErr
		}
		if alreadyPublished {
			continue
		}
		if err := copyPublishedFile(sidecar.Path, destination, false); err != nil {
			if os.IsExist(err) {
				equal, compareErr := filesEqual(sidecar.Path, destination)
				if compareErr == nil && equal {
					continue
				}
			}
			return copied, err
		}
		copied = append(copied, destination)
	}
	return copied, nil
}

func resolveSidecarDestination(sourcePath string, destinationBase string, suffix string) (string, bool, error) {
	for index := 0; ; index++ {
		marker := ""
		if index == 1 {
			marker = ".mvf"
		} else if index > 1 {
			marker = fmt.Sprintf(".mvf-%d", index)
		}
		candidate := destinationBase + marker + suffix
		info, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			return candidate, false, nil
		}
		if err != nil {
			return "", false, err
		}
		if info.IsDir() {
			continue
		}
		equal, err := filesEqual(sourcePath, candidate)
		if err != nil {
			return "", false, err
		}
		if equal {
			return candidate, true, nil
		}
	}
}
