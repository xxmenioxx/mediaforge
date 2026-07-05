package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	if err := h.db.First(&profile, job.ProfileID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return PublishResult{}, publishError{Status: http.StatusNotFound, Message: "profile not found", Err: err}
		}

		return PublishResult{}, err
	}

	destinationPath := plannedOutputPathForJob(h.db, job, library, profile)
	if path.Clean(job.OutputPath) != path.Clean(destinationPath) {
		if err := copyPublishedFile(job.OutputPath, destinationPath, overwrite); err != nil {
			status := http.StatusInternalServerError
			if os.IsExist(err) {
				status = http.StatusConflict
			}
			return PublishResult{}, publishError{Status: status, Err: err}
		}
	} else if !overwrite {
		if _, err := os.Stat(destinationPath); err != nil {
			return PublishResult{}, publishError{Status: http.StatusNotFound, Message: "published output path is not readable", Err: err}
		}
	}

	now := time.Now()
	job.PublishedPath = destinationPath
	job.PublishedAt = &now
	if archivedPath, err := h.archivePublishedOriginal(job); err != nil {
		return PublishResult{}, err
	} else if archivedPath != "" {
		job.Notes = appendNote(job.Notes, "Original archived: "+archivedPath)
	}

	if err := h.db.Save(&job).Error; err != nil {
		return PublishResult{}, err
	}

	return PublishResult{
		JobID:         job.ID,
		Status:        "published",
		SourcePath:    job.OutputPath,
		PublishedPath: destinationPath,
		Message:       "Output published to destination library.",
	}, nil
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
		if current == rawRootAbs || parent == rawRootAbs || !isInsideRoot(rawRootAbs, current) {
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
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "originalRetentionPolicy").Error; err == nil && setting.Value != nil {
		values = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	if value := normalizedOriginalsArchivePath(strings.TrimSpace(stringFromUnknown(values["processedOriginalsPath"]))); value != "" {
		return value, nil
	}

	pathsSetting := models.AppSetting{}
	if err := db.First(&pathsSetting, "key = ?", "paths").Error; err == nil && pathsSetting.Value != nil {
		if value := normalizedOriginalsArchivePath(strings.TrimSpace(stringFromUnknown(pathsSetting.Value["originalsArchivePath"]))); value != "" {
			return value, nil
		}
		if value := normalizedOriginalsArchivePath(strings.TrimSpace(stringFromUnknown(pathsSetting.Value["trashPath"]))); value != "" {
			return value, nil
		}
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
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
