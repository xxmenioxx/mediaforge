package handlers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HousekeepingPolicy struct {
	AutoEnabled                     bool `json:"autoEnabled"`
	IntervalHours                   int  `json:"intervalHours"`
	FailedRetentionDays             int  `json:"failedRetentionDays"`
	CanceledRetentionDays           int  `json:"canceledRetentionDays"`
	OrphanRetentionDays             int  `json:"orphanRetentionDays"`
	PublishedWorkspaceRetentionDays int  `json:"publishedWorkspaceRetentionDays"`
	PublishedJobLogRetentionDays    int  `json:"publishedJobLogRetentionDays"`
	FailedJobLogRetentionDays       int  `json:"failedJobLogRetentionDays"`
	CanceledJobLogRetentionDays     int  `json:"canceledJobLogRetentionDays"`
	OrphanJobLogRetentionDays       int  `json:"orphanJobLogRetentionDays"`
	EmptyRawDirectoriesEnabled      bool `json:"emptyRawDirectoriesEnabled"`
	OrphanStageArtifactsEnabled     bool `json:"orphanStageArtifactsEnabled"`
}

type HousekeepingCandidate struct {
	CandidateID              string                            `json:"candidateId"`
	Kind                     string                            `json:"kind"`
	Path                     string                            `json:"path"`
	JobID                    uint                              `json:"jobId,omitempty"`
	OperationID              string                            `json:"operationId,omitempty"`
	TestEncodeID             uint                              `json:"testEncodeId,omitempty"`
	Reason                   string                            `json:"reason"`
	Ownership                string                            `json:"ownership"`
	SizeBytes                int64                             `json:"sizeBytes"`
	ModifiedAt               string                            `json:"modifiedAt"`
	SafeToDelete             bool                              `json:"safeToDelete"`
	ManualOnly               bool                              `json:"manualOnly,omitempty"`
	Outcome                  string                            `json:"outcome,omitempty"`
	BlockedReason            string                            `json:"blockedReason,omitempty"`
	Maintenance              *HousekeepingMaintenanceDetails   `json:"maintenance,omitempty"`
	ProtectionReferences     []HousekeepingProtectionReference `json:"protectionReferences,omitempty"`
	ProtectionReferenceCount int                               `json:"protectionReferenceCount,omitempty"`
}

type HousekeepingMaintenanceDetails struct {
	AssetPath               string `json:"assetPath"`
	AssetRecordID           uint   `json:"assetRecordId,omitempty"`
	AssetRecordStatus       string `json:"assetRecordStatus,omitempty"`
	AssetRecordMissing      bool   `json:"assetRecordMissing"`
	Status                  string `json:"status"`
	Phase                   string `json:"phase"`
	FinishedAt              string `json:"finishedAt,omitempty"`
	TemporaryPresence       string `json:"temporaryPresence"`
	TemporarySizeKnown      bool   `json:"temporarySizeKnown"`
	BackupPath              string `json:"backupPath,omitempty"`
	BackupPresence          string `json:"backupPresence"`
	RecoveryState           string `json:"recoveryState"`
	ResultFingerprint       string `json:"resultFingerprint,omitempty"`
	CurrentAssetFingerprint string `json:"currentAssetFingerprint,omitempty"`
	AssetFingerprintState   string `json:"assetFingerprintState"`
	Warning                 string `json:"warning,omitempty"`
}

type HousekeepingProtectionReference struct {
	Source   string `json:"source"`
	RecordID string `json:"recordId"`
	Field    string `json:"field"`
	Path     string `json:"path"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
}

type HousekeepingCategorySummary struct {
	Kind             string `json:"kind"`
	CandidateCount   int    `json:"candidateCount"`
	RecoverableBytes int64  `json:"recoverableBytes"`
}

type HousekeepingReport struct {
	RanAt            time.Time                      `json:"ranAt"`
	DryRun           bool                           `json:"dryRun"`
	Outcome          string                         `json:"outcome"`
	PreviewID        string                         `json:"previewId,omitempty"`
	PreviewExpiresAt *time.Time                     `json:"previewExpiresAt,omitempty"`
	Candidates       []HousekeepingCandidate        `json:"candidates"`
	Summaries        []HousekeepingCategorySummary  `json:"summaries"`
	RemovedPaths     []string                       `json:"removedPaths"`
	RecoveredBytes   int64                          `json:"recoveredBytes"`
	Errors           []string                       `json:"errors"`
	RuntimeSnapshots RuntimeSnapshotRetentionReport `json:"runtimeSnapshots"`
}

type housekeepingBoundCandidate struct {
	Candidate HousekeepingCandidate `json:"candidate"`
	Identity  string                `json:"identity"`
}

type housekeepingPreviewState struct {
	PreviewID        string                         `json:"previewId"`
	CreatedAt        time.Time                      `json:"createdAt"`
	ExpiresAt        time.Time                      `json:"expiresAt"`
	Policy           HousekeepingPolicy             `json:"policy"`
	RoleRoots        map[string]string              `json:"roleRoots"`
	Candidates       []housekeepingBoundCandidate   `json:"candidates"`
	RuntimeSnapshots RuntimeSnapshotRetentionReport `json:"runtimeSnapshots"`
	Errors           []string                       `json:"errors"`
}

const (
	housekeepingPreviewKey          = "housekeepingPreview"
	housekeepingPreviewTTL          = 5 * time.Minute
	housekeepingMaxCandidates       = 500
	housekeepingKindWorkspace       = "job_workspace"
	housekeepingKindJobLog          = "job_log"
	housekeepingKindStageArtifact   = "stage_artifact"
	housekeepingKindEmptyDirectory  = "empty_directory"
	housekeepingKindTestEncode      = "test_encode"
	housekeepingKindRuntimeSnapshot = "runtime_snapshot"
)

var errStaleHousekeepingPreview = errors.New("housekeeping preview is stale or expired; run a fresh preview")

type HousekeepingHandler struct{ db *gorm.DB }

func NewHousekeepingHandler(db *gorm.DB) HousekeepingHandler { return HousekeepingHandler{db: db} }

func (h HousekeepingHandler) Preview(c *gin.Context) {
	report, err := PreviewHousekeeping(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h HousekeepingHandler) Run(c *gin.Context) {
	var input struct {
		PreviewID string `json:"previewId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.PreviewID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "previewId is required; run a fresh housekeeping preview first"})
		return
	}
	report, err := ExecuteHousekeepingPreview(h.db, input.PreviewID)
	if err != nil {
		if errors.Is(err, errStaleHousekeepingPreview) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func loadHousekeepingPolicy(db *gorm.DB) (HousekeepingPolicy, error) {
	policy := HousekeepingPolicy{
		AutoEnabled: true, IntervalHours: 24, FailedRetentionDays: 7, CanceledRetentionDays: 3, OrphanRetentionDays: 7,
		PublishedWorkspaceRetentionDays: 0, PublishedJobLogRetentionDays: 14, FailedJobLogRetentionDays: 14,
		CanceledJobLogRetentionDays: 7, OrphanJobLogRetentionDays: 7,
		EmptyRawDirectoriesEnabled: true, OrphanStageArtifactsEnabled: true,
	}
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
	policy.PublishedWorkspaceRetentionDays = intSetting(setting.Value, "publishedWorkspaceRetentionDays", policy.PublishedWorkspaceRetentionDays)
	policy.PublishedJobLogRetentionDays = intSetting(setting.Value, "publishedJobLogRetentionDays", policy.PublishedJobLogRetentionDays)
	policy.FailedJobLogRetentionDays = intSetting(setting.Value, "failedJobLogRetentionDays", policy.FailedJobLogRetentionDays)
	policy.CanceledJobLogRetentionDays = intSetting(setting.Value, "canceledJobLogRetentionDays", policy.CanceledJobLogRetentionDays)
	policy.OrphanJobLogRetentionDays = intSetting(setting.Value, "orphanJobLogRetentionDays", policy.OrphanJobLogRetentionDays)
	policy.EmptyRawDirectoriesEnabled = boolSetting(setting.Value["emptyRawDirectoriesEnabled"], policy.EmptyRawDirectoriesEnabled)
	policy.OrphanStageArtifactsEnabled = boolSetting(setting.Value["orphanStageArtifactsEnabled"], policy.OrphanStageArtifactsEnabled)
	return policy, nil
}

func RunHousekeeping(db *gorm.DB, dryRun bool) (HousekeepingReport, error) {
	if dryRun {
		return PreviewHousekeeping(db)
	}
	return runAutomaticHousekeeping(db)
}

func housekeepingCandidate(db *gorm.DB, jobID uint, path string, modified time.Time, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, bool, error) {
	candidate := HousekeepingCandidate{Kind: housekeepingKindWorkspace, Path: path, JobID: jobID, Ownership: "queue_job workspace contract", ModifiedAt: modified.UTC().Format(time.RFC3339Nano)}
	var job models.QueueJob
	result := db.Where("id = ?", jobID).Limit(1).Find(&job)
	if result.Error != nil {
		return candidate, false, result.Error
	}
	retentionDays := 0
	if result.RowsAffected == 0 {
		candidate.JobID, candidate.Reason, candidate.Ownership, retentionDays = 0, "orphan workspace", "direct work/job-N contract", policy.OrphanRetentionDays
	} else if isProtectedHousekeepingJob(&job) {
		candidate.Reason = "job workspace is active, under review, or completed but unpublished"
		candidate.BlockedReason = "job lifecycle remains protected"
		return candidate, false, nil
	} else if job.Status == JobStatusCanceled {
		candidate.Reason, retentionDays = "canceled job retention expired", policy.CanceledRetentionDays
	} else if job.Status == JobStatusFailed {
		candidate.Reason, retentionDays = "failed job retention expired", policy.FailedRetentionDays
	} else if job.Status == JobStatusCompleted && job.PublishedAt != nil {
		candidate.Reason, retentionDays = "published job workspace", policy.PublishedWorkspaceRetentionDays
	} else {
		candidate.Reason = "workspace ownership or lifecycle is ambiguous"
		candidate.BlockedReason = "job is not in a cleanup eligible terminal state"
		return candidate, false, nil
	}
	reference := modified
	if result.RowsAffected > 0 {
		reference = job.UpdatedAt
		if job.FinishedAt != nil {
			reference = *job.FinishedAt
		}
		if job.PublishedAt != nil && job.Status == JobStatusCompleted {
			reference = *job.PublishedAt
		}
	}
	if now.Before(reference.Add(time.Duration(max(retentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "retention period has not elapsed"
		return candidate, false, nil
	}
	if jobHasActiveHousekeepingReferences(db, jobID, path) {
		candidate.BlockedReason = "active worker, reservation, or operation references this workspace"
		return candidate, false, nil
	}
	size, err := directorySizeNoSymlinks(path)
	if err != nil {
		candidate.BlockedReason = "workspace contains an unsafe or unreadable entry"
		return candidate, false, nil
	}
	candidate.SizeBytes = size
	candidate.SafeToDelete = true
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
		candidates = append(candidates, models.JSONMap{
			"candidateId": item.CandidateID, "kind": item.Kind, "path": item.Path, "jobId": item.JobID,
			"operationId": item.OperationID, "testEncodeId": item.TestEncodeID, "reason": item.Reason,
			"ownership": item.Ownership, "sizeBytes": item.SizeBytes, "modifiedAt": item.ModifiedAt,
			"safeToDelete": item.SafeToDelete, "blockedReason": item.BlockedReason,
		})
	}
	summaries := make([]models.JSONMap, 0, len(report.Summaries))
	for _, summary := range report.Summaries {
		summaries = append(summaries, models.JSONMap{"kind": summary.Kind, "candidateCount": summary.CandidateCount, "recoverableBytes": summary.RecoverableBytes})
	}
	runtimeSnapshots := models.JSONMap{"rowsConsidered": report.RuntimeSnapshots.RowsConsidered, "referencedKept": report.RuntimeSnapshots.ReferencedKept, "latestKept": report.RuntimeSnapshots.LatestKept, "recentUnreferencedKept": report.RuntimeSnapshots.RecentUnreferencedKept, "pruneCandidates": report.RuntimeSnapshots.PruneCandidates, "deleted": report.RuntimeSnapshots.Deleted, "errors": report.RuntimeSnapshots.Errors}
	value := models.JSONMap{"ranAt": report.RanAt.UTC().Format(time.RFC3339Nano), "dryRun": report.DryRun, "previewId": report.PreviewID, "previewExpiresAt": report.PreviewExpiresAt, "candidates": candidates, "summaries": summaries, "removedPaths": report.RemovedPaths, "recoveredBytes": report.RecoveredBytes, "errors": report.Errors, "runtimeSnapshots": runtimeSnapshots}
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
	w.tick()
	ticker := time.NewTicker(5 * time.Second)
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
	if _, err := processPendingCanceledJobCleanups(w.db, 2); err != nil {
		applog.Event("error", "housekeeping", "canceled_job_cleanup_cycle_failed", nil, err)
	}
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
