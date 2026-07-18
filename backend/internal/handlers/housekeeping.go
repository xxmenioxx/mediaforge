package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HousekeepingPolicy struct {
	AutoEnabled           bool `json:"autoEnabled"`
	IntervalHours         int  `json:"intervalHours"`
	FailedRetentionDays   int  `json:"failedRetentionDays"`
	CanceledRetentionDays int  `json:"canceledRetentionDays"`
	OrphanRetentionDays   int  `json:"orphanRetentionDays"`
}

type HousekeepingCandidate struct {
	Path       string `json:"path"`
	JobID      uint   `json:"jobId,omitempty"`
	Reason     string `json:"reason"`
	SizeBytes  int64  `json:"sizeBytes"`
	ModifiedAt string `json:"modifiedAt"`
}

type HousekeepingReport struct {
	RanAt          time.Time               `json:"ranAt"`
	DryRun         bool                    `json:"dryRun"`
	Candidates     []HousekeepingCandidate `json:"candidates"`
	RemovedPaths   []string                `json:"removedPaths"`
	RecoveredBytes int64                   `json:"recoveredBytes"`
	Errors         []string                `json:"errors"`
}

type HousekeepingHandler struct{ db *gorm.DB }

func NewHousekeepingHandler(db *gorm.DB) HousekeepingHandler { return HousekeepingHandler{db: db} }

func (h HousekeepingHandler) Preview(c *gin.Context) {
	report, err := RunHousekeeping(h.db, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h HousekeepingHandler) Run(c *gin.Context) {
	var preview models.AppSetting
	result := h.db.Where("key = ?", "housekeepingLastRun").Limit(1).Find(&preview)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	dryRun, _ := preview.Value["dryRun"].(bool)
	if result.RowsAffected == 0 || !dryRun || time.Since(preview.UpdatedAt) > 15*time.Minute {
		c.JSON(http.StatusConflict, gin.H{"error": "run a fresh housekeeping preview before removing files"})
		return
	}
	report, err := RunHousekeeping(h.db, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func loadHousekeepingPolicy(db *gorm.DB) (HousekeepingPolicy, error) {
	policy := HousekeepingPolicy{AutoEnabled: true, IntervalHours: 24, FailedRetentionDays: 7, CanceledRetentionDays: 3, OrphanRetentionDays: 7}
	var setting models.AppSetting
	result := db.Where("key = ?", "housekeeping").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 {
		return policy, result.Error
	}
	policy.AutoEnabled = boolSetting(setting.Value["autoEnabled"], policy.AutoEnabled)
	policy.IntervalHours = intSetting(setting.Value, "intervalHours", policy.IntervalHours)
	policy.FailedRetentionDays = intSetting(setting.Value, "failedRetentionDays", policy.FailedRetentionDays)
	policy.CanceledRetentionDays = intSetting(setting.Value, "canceledRetentionDays", policy.CanceledRetentionDays)
	policy.OrphanRetentionDays = intSetting(setting.Value, "orphanRetentionDays", policy.OrphanRetentionDays)
	return policy, nil
}

func RunHousekeeping(db *gorm.DB, dryRun bool) (HousekeepingReport, error) {
	report := HousekeepingReport{RanAt: time.Now(), DryRun: dryRun, Candidates: []HousekeepingCandidate{}, RemovedPaths: []string{}, Errors: []string{}}
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return report, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return report, err
	}
	workRoot, err := roles.Path(scheduler.StorageRoleWork)
	if err != nil {
		return report, err
	}
	root, err := filepath.Abs(workRoot)
	if err != nil {
		return report, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return report, persistHousekeepingReport(db, report)
	}
	if err != nil {
		return report, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "job-") {
			continue
		}
		id, parseErr := strconv.ParseUint(strings.TrimPrefix(entry.Name(), "job-"), 10, 64)
		if parseErr != nil || id == 0 {
			continue
		}
		candidatePath := filepath.Join(root, entry.Name())
		if filepath.Dir(candidatePath) != root {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			report.Errors = append(report.Errors, statErr.Error())
			continue
		}
		candidate, eligible, candidateErr := housekeepingCandidate(db, uint(id), candidatePath, info.ModTime(), policy, report.RanAt)
		if candidateErr != nil {
			report.Errors = append(report.Errors, candidateErr.Error())
			continue
		}
		if !eligible {
			continue
		}
		report.Candidates = append(report.Candidates, candidate)
		if dryRun {
			continue
		}
		if err := os.RemoveAll(candidate.Path); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", candidate.Path, err))
			continue
		}
		report.RemovedPaths = append(report.RemovedPaths, candidate.Path)
		report.RecoveredBytes += candidate.SizeBytes
	}
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func housekeepingCandidate(db *gorm.DB, jobID uint, path string, modified time.Time, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, bool, error) {
	candidate := HousekeepingCandidate{Path: path, JobID: jobID, ModifiedAt: modified.UTC().Format(time.RFC3339)}
	var job models.QueueJob
	result := db.Where("id = ?", jobID).Limit(1).Find(&job)
	if result.Error != nil {
		return candidate, false, result.Error
	}
	retentionDays := 0
	if result.RowsAffected == 0 {
		candidate.JobID, candidate.Reason, retentionDays = 0, "orphan workspace", policy.OrphanRetentionDays
	} else if job.Status == JobStatusCanceled {
		candidate.Reason, retentionDays = "canceled job retention expired", policy.CanceledRetentionDays
	} else if job.Status == JobStatusFailed {
		candidate.Reason, retentionDays = "failed job retention expired", policy.FailedRetentionDays
	} else if job.PublishedAt != nil {
		candidate.Reason, retentionDays = "published job workspace", 0
	} else {
		return candidate, false, nil
	}
	reference := modified
	if result.RowsAffected > 0 && job.UpdatedAt.After(reference) {
		reference = job.UpdatedAt
	}
	if now.Before(reference.Add(time.Duration(max(retentionDays, 0)) * 24 * time.Hour)) {
		return candidate, false, nil
	}
	size, err := directorySize(path)
	if err != nil {
		return candidate, false, err
	}
	candidate.SizeBytes = size
	return candidate, true, nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func persistHousekeepingReport(db *gorm.DB, report HousekeepingReport) error {
	candidates := make([]models.JSONMap, 0, len(report.Candidates))
	for _, item := range report.Candidates {
		candidates = append(candidates, models.JSONMap{"path": item.Path, "jobId": item.JobID, "reason": item.Reason, "sizeBytes": item.SizeBytes, "modifiedAt": item.ModifiedAt})
	}
	value := models.JSONMap{"ranAt": report.RanAt.UTC().Format(time.RFC3339), "dryRun": report.DryRun, "candidates": candidates, "removedPaths": report.RemovedPaths, "recoveredBytes": report.RecoveredBytes, "errors": report.Errors}
	setting := models.AppSetting{Key: "housekeepingLastRun", Value: value}
	return db.Where(models.AppSetting{Key: setting.Key}).Assign(models.AppSetting{Value: value}).FirstOrCreate(&setting).Error
}

type AutoHousekeeper struct {
	db   *gorm.DB
	stop chan struct{}
	mu   sync.Mutex
}

func StartAutoHousekeeper(db *gorm.DB) *AutoHousekeeper {
	worker := &AutoHousekeeper{db: db, stop: make(chan struct{})}
	go worker.run()
	return worker
}
func (w *AutoHousekeeper) Stop() { close(w.stop) }
func (w *AutoHousekeeper) run() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.tick()
		case <-w.stop:
			return
		}
	}
}
func (w *AutoHousekeeper) tick() {
	if !w.mu.TryLock() {
		return
	}
	defer w.mu.Unlock()
	policy, err := loadHousekeepingPolicy(w.db)
	if err != nil || !policy.AutoEnabled {
		return
	}
	var last models.AppSetting
	if w.db.Where("key = ?", "housekeepingLastRun").Limit(1).Find(&last).RowsAffected > 0 && time.Since(last.UpdatedAt) < time.Duration(max(policy.IntervalHours, 1))*time.Hour {
		return
	}
	_, _ = RunHousekeeping(w.db, false)
}
