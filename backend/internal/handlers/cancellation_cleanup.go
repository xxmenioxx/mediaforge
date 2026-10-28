package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	canceledCleanupStatusNotRequired = "not_required"
	canceledCleanupStatusPending     = "pending"
	canceledCleanupStatusRunning     = "running"
	canceledCleanupStatusCompleted   = "completed"
	canceledCleanupStatusFailed      = "failed"
	canceledCleanupMaxAttempts       = 5
	canceledCleanupDeferralBaseDelay = 15 * time.Second
	canceledCleanupDeferralMaxDelay  = 5 * time.Minute
)

var (
	errCanceledCleanupDeferred  = errors.New("canceled-job cleanup deferred while ownership is active")
	errCanceledCleanupBlocked   = errors.New("canceled-job cleanup is no longer authorized")
	errCanceledCleanupLeaseLost = errors.New("canceled-job cleanup lease is no longer owned")
)

type canceledCleanupClaim struct {
	Job        models.QueueJob
	LeaseToken string
}

type cancellationPolicy struct {
	DeleteGeneratedFiles           bool
	DeletePartialOutputFromStaging bool
	ControlledRoots                []string
}

func loadCancellationPolicy(db *gorm.DB) (cancellationPolicy, error) {
	policy := cancellationPolicy{
		ControlledRoots: []string{"/media/staging", "/mwp/work", "/mwp/work/temp", "/tmp/mvforge"},
	}
	var setting models.AppSetting
	result := db.Where("key = ?", "cancellationPolicy").Limit(1).Find(&setting)
	if result.Error != nil {
		return cancellationPolicy{}, result.Error
	}
	if result.RowsAffected > 0 {
		policy.DeleteGeneratedFiles = boolSetting(setting.Value["deleteGeneratedFiles"], false)
		policy.DeletePartialOutputFromStaging = boolSetting(setting.Value["deletePartialOutputFromStaging"], false)
		policy.ControlledRoots = stringSliceFromUnknown(setting.Value["controlledRoots"])
	}
	return policy, nil
}

// persistCanceledJobCleanupIntent is called from the same transaction that
// makes a QueueJob terminal. The QueueJob owns one stable cleanup operation;
// Housekeeping later claims and executes it outside that transaction.
func persistCanceledJobCleanupIntent(tx *gorm.DB, job *models.QueueJob) error {
	if tx == nil || job == nil || job.ID == 0 || job.Status != JobStatusCanceled {
		return errors.New("cannot persist cleanup intent without a canceled QueueJob transaction")
	}
	if job.CleanupOperationID != "" || job.CleanupStatus == canceledCleanupStatusNotRequired || job.CleanupStatus == canceledCleanupStatusCompleted {
		return nil
	}
	policy, err := loadCancellationPolicy(tx)
	if err != nil {
		return err
	}
	if !policy.DeleteGeneratedFiles && !policy.DeletePartialOutputFromStaging {
		legacyPending := job.CleanupStatus == canceledCleanupStatusPending || job.CleanupStatus == canceledCleanupStatusRunning || job.CleanupStatus == canceledCleanupStatusFailed
		job.CleanupStatus = canceledCleanupStatusNotRequired
		updates := map[string]any{"cleanup_status": canceledCleanupStatusNotRequired}
		if legacyPending {
			updates["cleanup_error"] = "cleanup policy is disabled; persisted operation was not resumed"
			job.CleanupError = updates["cleanup_error"].(string)
		}
		return tx.Model(&models.QueueJob{}).
			Where("id = ? AND status = ? AND cleanup_operation_id = ''", job.ID, JobStatusCanceled).
			Updates(updates).Error
	}

	operationID := fmt.Sprintf("queue-canceled-job-%d", job.ID)
	outputPath := strings.TrimSpace(job.OutputPath)
	cleanOutput := filepath.Clean(outputPath)
	status := canceledCleanupStatusPending
	diagnostic := ""
	if outputPath == "" || cleanOutput == "." || !filepath.IsAbs(cleanOutput) || !pathInsideAnyRoot(cleanOutput, policy.ControlledRoots) {
		status = canceledCleanupStatusFailed
		diagnostic = "cleanup target is missing or outside the configured cancellation roots"
	}
	if status == canceledCleanupStatusPending && policy.DeleteGeneratedFiles {
		workspace := filepath.Dir(cleanOutput)
		if filepath.Base(workspace) != fmt.Sprintf("job-%d", job.ID) || !pathInsideAnyRoot(workspace, policy.ControlledRoots) {
			status = canceledCleanupStatusFailed
			diagnostic = "cleanup workspace does not match the canonical QueueJob workspace"
		}
	}
	if status == canceledCleanupStatusPending && !policy.DeleteGeneratedFiles && !policy.DeletePartialOutputFromStaging {
		status = canceledCleanupStatusNotRequired
	}

	job.CleanupOperationID = operationID
	job.CleanupTargetPath = cleanOutput
	job.CleanupOwnerRevision = canceledCleanupOwnerRevision(*job)
	job.CleanupStatus = status
	job.CleanupError = diagnostic
	job.CleanupLeaseToken = ""
	job.CleanupLeaseExpiresAt = nil
	job.CleanupAttemptCount = 0
	job.CleanupDeferralCount = 0
	job.CleanupNextAttemptAt = nil
	updates := map[string]any{
		"cleanup_operation_id":     operationID,
		"cleanup_target_path":      cleanOutput,
		"cleanup_owner_revision":   job.CleanupOwnerRevision,
		"cleanup_status":           status,
		"cleanup_error":            diagnostic,
		"cleanup_lease_token":      "",
		"cleanup_lease_expires_at": nil,
		"cleanup_attempt_count":    0,
		"cleanup_deferral_count":   0,
		"cleanup_next_attempt_at":  nil,
	}
	result := tx.Model(&models.QueueJob{}).
		Where("id = ? AND status = ? AND cleanup_operation_id = ''", job.ID, JobStatusCanceled).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		var current models.QueueJob
		if err := tx.First(&current, job.ID).Error; err != nil {
			return err
		}
		if current.CleanupOperationID != operationID {
			return errors.New("QueueJob cleanup intent changed while cancellation was being persisted")
		}
		*job = current
	}
	return nil
}

func canceledCleanupOwnerRevision(job models.QueueJob) string {
	requested := ""
	if job.CancellationRequestedAt != nil {
		requested = job.CancellationRequestedAt.UTC().Format(time.RFC3339Nano)
	}
	finished := ""
	if job.FinishedAt != nil {
		finished = job.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	processTree := ""
	if job.ProcessTreeExitVerifiedAt != nil {
		processTree = job.ProcessTreeExitVerifiedAt.UTC().Format(time.RFC3339Nano)
	}
	return fmt.Sprintf("job:%d|cancel:%s|finished:%s|process-tree:%s", job.ID, requested, finished, processTree)
}

func reconcileCanceledJobCleanupIntents(db *gorm.DB, limit int) (int, error) {
	if db == nil || limit <= 0 {
		return 0, nil
	}
	var jobs []models.QueueJob
	if err := db.Where("status = ? AND cleanup_operation_id = '' AND cleanup_status IN ?", JobStatusCanceled, []string{canceledCleanupStatusPending, canceledCleanupStatusRunning, canceledCleanupStatusFailed}).
		Order("id ASC").Limit(limit).Find(&jobs).Error; err != nil {
		return 0, err
	}
	reconciled := 0
	for _, candidate := range jobs {
		err := db.Transaction(func(tx *gorm.DB) error {
			var current models.QueueJob
			if err := tx.First(&current, candidate.ID).Error; err != nil {
				return err
			}
			if current.Status != JobStatusCanceled || current.CleanupOperationID != "" {
				return nil
			}
			if err := persistCanceledJobCleanupIntent(tx, &current); err != nil {
				return err
			}
			reconciled++
			return nil
		})
		if err != nil {
			return reconciled, err
		}
	}
	return reconciled, nil
}

func claimNextCanceledJobCleanup(db *gorm.DB, now time.Time, leaseDuration time.Duration) (canceledCleanupClaim, bool, error) {
	if db == nil {
		return canceledCleanupClaim{}, false, errors.New("cleanup database is unavailable")
	}
	if leaseDuration <= 0 {
		return canceledCleanupClaim{}, false, errors.New("cleanup lease duration must be positive")
	}
	var claim canceledCleanupClaim
	claimed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		// Retire malformed leases and capped rows before selecting work. The
		// bounded query below limits eligible operations, not earlier rows that
		// are waiting, leased, malformed, or permanently capped.
		if err := tx.Model(&models.QueueJob{}).
			Where("status = ? AND cleanup_operation_id <> '' AND cleanup_status = ? AND (cleanup_lease_expires_at IS NULL OR cleanup_lease_token = '')", JobStatusCanceled, canceledCleanupStatusRunning).
			Updates(map[string]any{
				"cleanup_status":           canceledCleanupStatusFailed,
				"cleanup_error":            "cleanup lease metadata is incomplete; operation left for review",
				"cleanup_lease_token":      "",
				"cleanup_lease_expires_at": nil,
				"cleanup_next_attempt_at":  nil,
				"cleanup_attempt_count":    canceledCleanupMaxAttempts,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.QueueJob{}).
			Where("status = ? AND cleanup_operation_id <> '' AND cleanup_attempt_count >= ? AND cleanup_status = ?", JobStatusCanceled, canceledCleanupMaxAttempts, canceledCleanupStatusPending).
			Updates(map[string]any{
				"cleanup_status":          canceledCleanupStatusFailed,
				"cleanup_error":           "cleanup retry limit exhausted",
				"cleanup_next_attempt_at": nil,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.QueueJob{}).
			Where("status = ? AND cleanup_operation_id <> '' AND cleanup_status = ? AND cleanup_lease_expires_at <= ? AND cleanup_attempt_count >= ?", JobStatusCanceled, canceledCleanupStatusRunning, now, canceledCleanupMaxAttempts-1).
			Updates(map[string]any{
				"cleanup_status":           canceledCleanupStatusFailed,
				"cleanup_error":            "cleanup lease expired after the retry limit",
				"cleanup_lease_token":      "",
				"cleanup_lease_expires_at": nil,
				"cleanup_next_attempt_at":  nil,
				"cleanup_attempt_count":    canceledCleanupMaxAttempts,
			}).Error; err != nil {
			return err
		}

		var jobs []models.QueueJob
		if err := tx.Where(`status = ? AND cleanup_operation_id <> '' AND (
			(cleanup_status = ? AND cleanup_attempt_count < ? AND (cleanup_next_attempt_at IS NULL OR cleanup_next_attempt_at <= ?)) OR
			(cleanup_status = ? AND cleanup_attempt_count < ? AND (cleanup_next_attempt_at IS NULL OR cleanup_next_attempt_at <= ?)) OR
			(cleanup_status = ? AND cleanup_lease_expires_at <= ? AND cleanup_attempt_count < ?)
		)`,
			JobStatusCanceled,
			canceledCleanupStatusPending, canceledCleanupMaxAttempts, now,
			canceledCleanupStatusFailed, canceledCleanupMaxAttempts, now,
			canceledCleanupStatusRunning, now, canceledCleanupMaxAttempts-1).
			Order("id ASC").Limit(64).Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			token, err := newCanceledCleanupLeaseToken()
			if err != nil {
				return err
			}
			where := tx.Model(&models.QueueJob{}).
				Where("id = ? AND status = ? AND cleanup_operation_id = ? AND cleanup_status = ? AND cleanup_lease_token = ?", job.ID, JobStatusCanceled, job.CleanupOperationID, job.CleanupStatus, job.CleanupLeaseToken)
			if job.CleanupStatus == canceledCleanupStatusRunning {
				where = where.Where("cleanup_lease_expires_at <= ?", now)
			} else if job.CleanupNextAttemptAt == nil {
				where = where.Where("cleanup_next_attempt_at IS NULL")
			} else {
				where = where.Where("cleanup_next_attempt_at <= ?", now)
			}
			expiresAt := now.Add(leaseDuration)
			updates := map[string]any{
				"cleanup_status":           canceledCleanupStatusRunning,
				"cleanup_lease_token":      token,
				"cleanup_lease_expires_at": expiresAt,
				"cleanup_next_attempt_at":  nil,
			}
			if job.CleanupStatus == canceledCleanupStatusRunning {
				// An expired lease is an interrupted execution. Deferrals below do
				// not increment this failure budget.
				updates["cleanup_attempt_count"] = gorm.Expr("cleanup_attempt_count + 1")
			}
			result := where.Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			if err := tx.First(&claim.Job, job.ID).Error; err != nil {
				return err
			}
			claim.LeaseToken = token
			claimed = true
			return nil
		}
		return nil
	})
	return claim, claimed, err
}

func assertCanceledJobCleanupLease(db *gorm.DB, claim canceledCleanupClaim, now time.Time) error {
	if claim.Job.ID == 0 || claim.LeaseToken == "" || claim.Job.CleanupOperationID == "" {
		return errCanceledCleanupLeaseLost
	}
	var count int64
	err := db.Model(&models.QueueJob{}).
		Where("id = ? AND status = ? AND cleanup_operation_id = ? AND cleanup_status = ? AND cleanup_lease_token = ? AND cleanup_lease_expires_at > ?", claim.Job.ID, JobStatusCanceled, claim.Job.CleanupOperationID, canceledCleanupStatusRunning, claim.LeaseToken, now).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return errCanceledCleanupLeaseLost
	}
	return nil
}

func renewCanceledJobCleanupLease(db *gorm.DB, claim canceledCleanupClaim, now time.Time, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		return errors.New("cleanup lease duration must be positive")
	}
	result := db.Model(&models.QueueJob{}).
		Where("id = ? AND status = ? AND cleanup_operation_id = ? AND cleanup_status = ? AND cleanup_lease_token = ? AND cleanup_lease_expires_at > ?", claim.Job.ID, JobStatusCanceled, claim.Job.CleanupOperationID, canceledCleanupStatusRunning, claim.LeaseToken, now).
		Update("cleanup_lease_expires_at", now.Add(leaseDuration))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errCanceledCleanupLeaseLost
	}
	return nil
}

func finishCanceledJobCleanup(db *gorm.DB, claim canceledCleanupClaim, now time.Time, runErr error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var current models.QueueJob
		if err := tx.First(&current, claim.Job.ID).Error; err != nil {
			return err
		}
		if current.CleanupOperationID != claim.Job.CleanupOperationID || current.CleanupLeaseToken != claim.LeaseToken || current.CleanupStatus != canceledCleanupStatusRunning || current.CleanupLeaseExpiresAt == nil || !current.CleanupLeaseExpiresAt.After(now) {
			return errCanceledCleanupLeaseLost
		}
		updates := map[string]any{
			"cleanup_lease_token":      "",
			"cleanup_lease_expires_at": nil,
			"cleanup_next_attempt_at":  nil,
		}
		if runErr == nil {
			updates["cleanup_status"] = canceledCleanupStatusCompleted
			updates["cleanup_error"] = ""
		} else if errors.Is(runErr, errCanceledCleanupDeferred) {
			updates["cleanup_status"] = canceledCleanupStatusPending
			deferralCount := current.CleanupDeferralCount + 1
			delay := canceledCleanupDeferralDelay(deferralCount)
			updates["cleanup_deferral_count"] = deferralCount
			updates["cleanup_error"] = boundedCanceledCleanupError(fmt.Errorf("%v (ownership deferral %d; retry in %s)", runErr, deferralCount, delay))
			next := now.Add(delay)
			updates["cleanup_next_attempt_at"] = next
		} else {
			updates["cleanup_status"] = canceledCleanupStatusFailed
			updates["cleanup_error"] = boundedCanceledCleanupError(runErr)
			failedAttemptCount := current.CleanupAttemptCount + 1
			updates["cleanup_attempt_count"] = failedAttemptCount
			if !errors.Is(runErr, errCanceledCleanupBlocked) && failedAttemptCount < canceledCleanupMaxAttempts {
				next := now.Add(canceledCleanupRetryDelay(failedAttemptCount))
				updates["cleanup_next_attempt_at"] = next
			}
		}
		result := tx.Model(&models.QueueJob{}).
			Where("id = ? AND status = ? AND cleanup_operation_id = ? AND cleanup_status = ? AND cleanup_lease_token = ?", current.ID, JobStatusCanceled, claim.Job.CleanupOperationID, canceledCleanupStatusRunning, claim.LeaseToken).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errCanceledCleanupLeaseLost
		}
		return nil
	})
}

func canceledCleanupRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 2 * time.Minute
	case 3:
		return 4 * time.Minute
	case 4:
		return 8 * time.Minute
	default:
		return 16 * time.Minute
	}
}

func canceledCleanupDeferralDelay(deferralCount int) time.Duration {
	delay := canceledCleanupDeferralBaseDelay
	for remaining := deferralCount - 1; remaining > 0 && delay < canceledCleanupDeferralMaxDelay; remaining-- {
		delay *= 2
		if delay > canceledCleanupDeferralMaxDelay {
			delay = canceledCleanupDeferralMaxDelay
		}
	}
	return delay
}

func newCanceledCleanupLeaseToken() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func boundedCanceledCleanupError(err error) string {
	if err == nil {
		return ""
	}
	message := []rune(strings.TrimSpace(err.Error()))
	if len(message) > 1000 {
		message = message[:1000]
	}
	return string(message)
}

func pathInsideAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if strings.TrimSpace(root) != "" && pathIsInside(path, filepath.Clean(root)) {
			return true
		}
	}
	return false
}
