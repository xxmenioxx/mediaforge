package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

func PreviewHousekeeping(db *gorm.DB) (HousekeepingReport, error) {
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		return HousekeepingReport{}, err
	}
	now := time.Now().UTC()
	state, err := buildHousekeepingPreviewState(db, policy, roots, now)
	if err != nil {
		return HousekeepingReport{}, err
	}
	if err := saveHousekeepingPreview(db, state); err != nil {
		return HousekeepingReport{}, err
	}
	report := housekeepingReportForState(state, true)
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func ExecuteHousekeepingPreview(db *gorm.DB, previewID string) (HousekeepingReport, error) {
	previewID = strings.TrimSpace(previewID)
	if previewID == "" {
		return HousekeepingReport{}, errStaleHousekeepingPreview
	}
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		return HousekeepingReport{}, err
	}
	state, err := consumeHousekeepingPreview(db, previewID, policy, roots, time.Now().UTC())
	if err != nil {
		return HousekeepingReport{}, err
	}
	report := housekeepingReportForState(state, false)
	report.RanAt = time.Now().UTC()
	applyHousekeepingPlan(db, roots, policy, &state, &report)
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func runAutomaticHousekeeping(db *gorm.DB) (HousekeepingReport, error) {
	report := HousekeepingReport{RanAt: time.Now().UTC(), Candidates: []HousekeepingCandidate{}, Summaries: []HousekeepingCategorySummary{}, RemovedPaths: []string{}, Errors: []string{}, RuntimeSnapshots: RuntimeSnapshotRetentionReport{Errors: []string{}}}
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return report, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return report, err
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		return report, err
	}
	state, err := buildHousekeepingPreviewState(db, policy, roots, report.RanAt)
	if err != nil {
		return report, err
	}
	for index := range state.Candidates {
		bound := &state.Candidates[index]
		if !bound.Candidate.ManualOnly {
			continue
		}
		bound.Candidate.SafeToDelete = false
		bound.Candidate.Outcome = "blocked"
		bound.Candidate.BlockedReason = "Library maintenance temporary cleanup requires an operator preview"
		bound.Candidate.CandidateID = housekeepingCandidateID(bound.Candidate, bound.Identity)
	}
	report = housekeepingReportForState(state, false)
	applyHousekeepingPlan(db, roots, policy, &state, &report)
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func buildHousekeepingPreviewState(db *gorm.DB, policy HousekeepingPolicy, roots map[string]string, now time.Time) (housekeepingPreviewState, error) {
	previewID, err := newHousekeepingPreviewID()
	if err != nil {
		return housekeepingPreviewState{}, err
	}
	state := housekeepingPreviewState{
		PreviewID: previewID, CreatedAt: now, ExpiresAt: now.Add(housekeepingPreviewTTL), Policy: policy,
		RoleRoots: roots, Candidates: []housekeepingBoundCandidate{},
		RuntimeSnapshots: RuntimeSnapshotRetentionReport{Errors: []string{}}, Errors: []string{},
	}
	if err := addWorkspaceCandidates(db, roots, policy, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	if err := addJobLogCandidates(db, roots, policy, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	if policy.OrphanStageArtifactsEnabled {
		if err := addStageArtifactCandidates(db, roots, policy, now, &state); err != nil {
			state.Errors = append(state.Errors, err.Error())
		}
	} else {
		state.Errors = append(state.Errors, "orphan stage artifact cleanup is disabled by policy")
	}
	if err := addTestEncodeCandidates(db, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	if err := addRuntimeSnapshotCandidates(db, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	unsafeRaw := unsafeRawRoleRoot(roots)
	if unsafeRaw {
		state.Errors = append(state.Errors, "configured RAW storage role overlaps another storage role; empty RAW directory cleanup is blocked")
	}
	if err := addEmptyDirectoryCandidates(db, roots, unsafeRaw, policy.EmptyRawDirectoriesEnabled, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	sort.SliceStable(state.Candidates, func(i, j int) bool {
		left, right := state.Candidates[i].Candidate, state.Candidates[j].Candidate
		if housekeepingCandidateOrder(left.Kind) != housekeepingCandidateOrder(right.Kind) {
			return housekeepingCandidateOrder(left.Kind) < housekeepingCandidateOrder(right.Kind)
		}
		if left.Kind == housekeepingKindEmptyDirectory && right.Kind == housekeepingKindEmptyDirectory {
			leftDepth, rightDepth := pathDepth(left.Path), pathDepth(right.Path)
			if leftDepth != rightDepth {
				return leftDepth > rightDepth
			}
		}
		return left.Path < right.Path
	})
	return state, nil
}

func addWorkspaceCandidates(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, state *housekeepingPreviewState) error {
	root := roots[string(scheduler.StorageRoleWork)]
	entries, err := readRoleRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read work storage role: %w", err)
	}
	unsafeRoleRoot := unsafeOperationalRoleRoot(roots, "work")
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "job-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, statErr := os.Lstat(path)
		candidate := HousekeepingCandidate{Kind: housekeepingKindWorkspace, Path: path, Ownership: "direct work/job-N workspace contract", Reason: "job workspace retention", ModifiedAt: now.Format(time.RFC3339Nano)}
		if unsafeRoleRoot {
			candidate.BlockedReason = "configured work storage role overlaps RAW, Library, or originals archive"
		}
		if statErr != nil {
			candidate.BlockedReason = "workspace could not be inspected"
			candidate.Reason = "workspace ownership could not be verified"
			addBoundCandidate(state, candidate, "unavailable:"+statErr.Error())
			continue
		}
		candidate.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			candidate.Reason = "job workspace path is not a direct regular directory"
			candidate.BlockedReason = "symlink or non-directory workspace entries are never followed or removed"
			addBoundCandidate(state, candidate, statIdentity(info, false))
			continue
		}
		suffix := strings.TrimPrefix(entry.Name(), "job-")
		id, parseErr := strconv.ParseUint(suffix, 10, 64)
		if parseErr != nil || id == 0 || strconv.FormatUint(id, 10) != suffix {
			candidate.Reason = "ambiguous work directory"
			candidate.BlockedReason = "workspace name does not identify a canonical QueueJob ID"
			addBoundCandidate(state, candidate, statIdentity(info, true))
			continue
		}
		candidate.JobID = uint(id)
		classified, _, classifyErr := housekeepingCandidate(db, uint(id), path, info.ModTime(), policy, now)
		if classifyErr != nil {
			candidate.Reason = "workspace lifecycle could not be read"
			candidate.BlockedReason = classifyErr.Error()
		} else {
			candidate = classified
		}
		identity, identityErr := directoryTreeIdentity(path)
		if identityErr != nil {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace contains a symlink, special file, or unreadable entry"
			identity = statIdentity(info, true)
		} else {
			candidate.SizeBytes, _ = directorySizeNoSymlinks(path)
			if unsafeRoleRoot {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured work storage role overlaps RAW, Library, or originals archive"
			}
		}
		addBoundCandidate(state, candidate, identity)
	}
	return nil
}

func addJobLogCandidates(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, state *housekeepingPreviewState) error {
	root := roots[string(scheduler.StorageRoleLogs)]
	entries, err := readRoleRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read logs storage role: %w", err)
	}
	unsafeRoleRoot := unsafeOperationalRoleRoot(roots, "logs")
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "job-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		suffix := strings.TrimSuffix(strings.TrimPrefix(name, "job-"), ".log")
		id, parseErr := strconv.ParseUint(suffix, 10, 64)
		if parseErr != nil || id == 0 || strconv.FormatUint(id, 10) != suffix {
			continue
		}
		path := filepath.Join(root, name)
		info, statErr := os.Lstat(path)
		candidate := HousekeepingCandidate{Kind: housekeepingKindJobLog, Path: path, JobID: uint(id), Ownership: "canonical logs/job-<queueJobId>.log name", Reason: "job log retention", ModifiedAt: now.Format(time.RFC3339Nano)}
		if statErr != nil {
			candidate.BlockedReason = "job log could not be inspected"
			candidate.Reason = "job log ownership could not be verified"
			addBoundCandidate(state, candidate, "unavailable:"+statErr.Error())
			continue
		}
		candidate.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		identity := statIdentity(info, false)
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			candidate.BlockedReason = "job logs must be regular files without symlink components"
			candidate.Reason = "job log path is unsafe"
			addBoundCandidate(state, candidate, identity)
			continue
		}
		candidate.SizeBytes = info.Size()
		if unsafeRoleRoot {
			candidate.BlockedReason = "configured logs storage role overlaps RAW, Library, or originals archive"
		}
		var job models.QueueJob
		result := db.Where("id = ?", uint(id)).Limit(1).Find(&job)
		if result.Error != nil {
			candidate.BlockedReason = result.Error.Error()
		} else if result.RowsAffected == 0 {
			candidate.JobID = 0
			candidate.Ownership = "canonical job log filename; missing QueueJob row"
			candidate.Reason = "orphan job log retention expired"
			candidate.SafeToDelete = now.After(info.ModTime().Add(time.Duration(max(policy.OrphanJobLogRetentionDays, 0)) * 24 * time.Hour))
			if !candidate.SafeToDelete {
				candidate.BlockedReason = "orphan log retention period has not elapsed"
			}
		} else {
			candidate = classifyJobLog(job, path, info, policy, now)
		}
		if unsafeRoleRoot {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "configured logs storage role overlaps RAW, Library, or originals archive"
		}
		if candidate.SafeToDelete && jobHasActiveHousekeepingReferences(db, uint(id), "") {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "active worker, reservation, or operation still owns this job"
		}
		addBoundCandidate(state, candidate, identity)
	}
	return nil
}

func classifyJobLog(job models.QueueJob, path string, info os.FileInfo, policy HousekeepingPolicy, now time.Time) HousekeepingCandidate {
	candidate := HousekeepingCandidate{Kind: housekeepingKindJobLog, Path: path, JobID: job.ID, Ownership: "queue_job lifecycle plus canonical job log name", ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), SizeBytes: info.Size()}
	if isProtectedHousekeepingJob(&job) {
		candidate.Reason = "job log retained for active, unpublished, or unresolved review lifecycle"
		candidate.BlockedReason = "job is queued, active, completed but unpublished, or has unresolved review/block state"
		return candidate
	}
	var retentionDays int
	var reference time.Time
	switch {
	case job.Status == JobStatusFailed:
		candidate.Reason, retentionDays = "failed job log retention expired", policy.FailedJobLogRetentionDays
	case job.Status == JobStatusCanceled:
		candidate.Reason, retentionDays = "canceled job log retention expired", policy.CanceledJobLogRetentionDays
	case job.Status == JobStatusCompleted && job.PublishedAt != nil:
		candidate.Reason, retentionDays = "published job log retention expired", policy.PublishedJobLogRetentionDays
		if job.PublishedAt != nil {
			reference = *job.PublishedAt
		}
	default:
		candidate.Reason = "job log lifecycle is not terminal"
		candidate.BlockedReason = "only failed, canceled, or published jobs have expiring logs"
		return candidate
	}
	if reference.IsZero() {
		reference = job.UpdatedAt
		if job.FinishedAt != nil {
			reference = *job.FinishedAt
		}
	}
	if now.Before(reference.Add(time.Duration(max(retentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "job log retention period has not elapsed"
		return candidate
	}
	if jobHasActiveHousekeepingReferencesFromJob(&job) {
		candidate.BlockedReason = "worker or operation still owns this job"
		return candidate
	}
	candidate.SafeToDelete = true
	return candidate
}

func addStageArtifactCandidates(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, state *housekeepingPreviewState) error {
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("TRIM(temporary_path) <> ''").Order("updated_at ASC").Order("id ASC").Limit(housekeepingMaxCandidates).Find(&operations).Error; err != nil {
			return fmt.Errorf("load owned maintenance artifacts: %w", err)
		}
		for _, operation := range operations {
			candidate, identity := maintenanceArtifactCandidate(db, roots, operation, policy, now)
			if coveredBySafeWorkspace(state.Candidates, candidate.Path) {
				continue
			}
			addBoundCandidate(state, candidate, identity)
		}
	}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		if err := db.Where("TRIM(original_archive_temporary_path) <> ''").Order("updated_at ASC").Order("publication_id ASC").Limit(housekeepingMaxCandidates).Find(&publications).Error; err != nil {
			return fmt.Errorf("load owned publisher staging artifacts: %w", err)
		}
		for _, publication := range publications {
			candidate, identity := publisherStageArtifactCandidate(db, roots, publication, policy, now)
			if coveredBySafeWorkspace(state.Candidates, candidate.Path) {
				continue
			}
			addBoundCandidate(state, candidate, identity)
		}
	}
	return nil
}

func maintenanceArtifactCandidate(db *gorm.DB, roots map[string]string, operation models.AssetMaintenanceOperation, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, string) {
	candidate := HousekeepingCandidate{Kind: housekeepingKindStageArtifact, OperationID: operation.ID, Path: operation.TemporaryPath, Ownership: "asset_maintenance_operation.temporary_path", Reason: "maintenance temporary artifact", ModifiedAt: operation.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	details := &HousekeepingMaintenanceDetails{
		AssetPath: operation.AssetPath, AssetRecordID: operation.AssetRecordID, Status: operation.Status, Phase: operation.Phase,
		TemporaryPresence: "unknown", BackupPath: operation.BackupPath, BackupPresence: "unknown", RecoveryState: "unresolved",
		ResultFingerprint: operation.ResultFingerprint, AssetFingerprintState: "unavailable", Warning: operation.Warning,
	}
	candidate.Maintenance = details
	if operation.FinishedAt != nil {
		candidate.ModifiedAt = operation.FinishedAt.UTC().Format(time.RFC3339Nano)
		details.FinishedAt = operation.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	identity := "unresolved"
	if operation.TemporaryPath == "" {
		candidate.BlockedReason = "operation has no persisted temporary path"
		candidate.Outcome = "blocked"
		return candidate, identity
	}

	if roots["library"] != "" && pathWithin(roots["library"], filepath.Clean(operation.TemporaryPath)) {
		candidate.ManualOnly = true
		role, path, info, pathErr := resolveOwnedFilePath(roots, operation.TemporaryPath, []string{"library"})
		if pathErr != nil || role != "library" {
			if pathErr == nil {
				pathErr = errors.New("maintenance temporary path did not resolve to the Library storage role")
			}
			candidate.BlockedReason = pathErr.Error()
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		candidate.Path = path
		candidate.Ownership = "asset_maintenance_operation.temporary_path under Library storage role"
		if unsafeLibraryRoleRoot(roots) {
			candidate.BlockedReason = "configured Library storage role overlaps another storage role"
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		if info == nil {
			details.TemporaryPresence = "absent"
			details.TemporarySizeKnown = true
		} else {
			details.TemporaryPresence = "present"
			if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				candidate.SizeBytes = info.Size()
				details.TemporarySizeKnown = true
			}
			identity = statIdentity(info, false)
		}
		if info == nil {
			if err := classifyLibraryMaintenanceArtifact(db, roots, operation, candidate, info); err != nil {
				candidate.BlockedReason = err.Error()
				candidate.Outcome = "blocked"
				return candidate, identity
			}
			_ = loadMaintenanceAssetRecord(db, operation, details)
			if assetFingerprint, _, assetErr := fingerprintLibraryMaintenanceAsset(roots, operation.AssetPath); assetErr == nil {
				details.CurrentAssetFingerprint = assetFingerprint
				if operation.ResultFingerprint != "" && assetFingerprint == operation.ResultFingerprint {
					details.AssetFingerprintState = "verified_match"
				} else if operation.ResultFingerprint != "" {
					details.AssetFingerprintState = "mismatch"
				} else {
					details.AssetFingerprintState = "result_fingerprint_unavailable"
				}
			}
			candidate.Reason = "completed maintenance temporary output is already absent"
			candidate.Outcome = "already_absent"
			candidate.SafeToDelete = false
			candidate.SizeBytes = 0
			return candidate, "missing"
		}
		assetRecordErr := loadMaintenanceAssetRecord(db, operation, details)
		if err := classifyLibraryMaintenanceArtifact(db, roots, operation, candidate, info); err != nil {
			candidate.BlockedReason = err.Error()
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		if assetRecordErr != nil {
			candidate.BlockedReason = assetRecordErr.Error()
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		fingerprint, fingerprintInfo, fingerprintErr := fingerprintSafeMaintenanceFile(path)
		if fingerprintErr != nil {
			candidate.BlockedReason = fingerprintErr.Error()
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		identity = statIdentity(fingerprintInfo, false)
		if fingerprint != operation.ResultFingerprint {
			candidate.BlockedReason = "temporary output fingerprint does not match the completed operation result"
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		assetFingerprint, _, assetErr := fingerprintLibraryMaintenanceAsset(roots, operation.AssetPath)
		if assetErr != nil {
			candidate.BlockedReason = assetErr.Error()
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		if assetFingerprint != operation.ResultFingerprint {
			details.CurrentAssetFingerprint = assetFingerprint
			details.AssetFingerprintState = "mismatch"
			candidate.BlockedReason = "current Library asset fingerprint does not match the completed operation result"
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		details.CurrentAssetFingerprint = assetFingerprint
		details.AssetFingerprintState = "verified_match"
		if reason := operationArtifactReferenceBlock(db, operation.ID, path); reason != "" {
			candidate.BlockedReason = reason
			candidate.Outcome = "blocked"
			return candidate, identity
		}
		candidate.Reason = "verified residual Library maintenance temporary output"
		candidate.SafeToDelete = true
		candidate.Outcome = "eligible"
		details.RecoveryState = "completed result verified; no backup or active reference remains"
		return candidate, identity
	}

	role, path, info, pathErr := resolveOwnedFilePath(roots, operation.TemporaryPath, []string{"work", "cache", "reports", "logs"})
	if pathErr != nil {
		candidate.BlockedReason = pathErr.Error()
		candidate.Outcome = "blocked"
		return candidate, identity
	}
	candidate.Path, candidate.Ownership = path, "asset_maintenance_operation.temporary_path under "+role+" storage role"
	if info == nil {
		details.TemporaryPresence = "absent"
		details.TemporarySizeKnown = true
		if operationIsActive(operation.Status) {
			candidate.BlockedReason = "owning maintenance operation is still active"
			candidate.Outcome = "blocked"
			return candidate, "missing"
		}
		if !operationIsAbandoned(operation.Status, operation.Phase) {
			candidate.BlockedReason = "operation status and phase do not establish an abandoned temporary artifact"
			candidate.Outcome = "blocked"
			return candidate, "missing"
		}
		candidate.Reason = "maintenance temporary artifact is already absent"
		candidate.Outcome = "already_absent"
		return candidate, "missing"
	}
	details.TemporaryPresence = "present"
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		candidate.BlockedReason = "owned stage artifacts must be regular non-symlink files"
		candidate.Outcome = "blocked"
		return candidate, statIdentity(info, false)
	}
	candidate.SizeBytes = info.Size()
	details.TemporarySizeKnown = true
	identity = statIdentity(info, false)
	if operationIsActive(operation.Status) {
		candidate.BlockedReason = "owning maintenance operation is still active"
		candidate.Outcome = "blocked"
		return candidate, identity
	}
	if !operationIsAbandoned(operation.Status, operation.Phase) {
		candidate.BlockedReason = "operation status and phase do not establish an abandoned temporary artifact"
		candidate.Outcome = "blocked"
		return candidate, identity
	}
	retentionReference := operation.UpdatedAt
	if operation.FinishedAt != nil {
		retentionReference = *operation.FinishedAt
	}
	if now.Before(retentionReference.Add(time.Duration(max(policy.OrphanRetentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "abandoned artifact retention period has not elapsed"
		candidate.Outcome = "blocked"
		return candidate, identity
	}
	if reason := operationArtifactReferenceBlock(db, operation.ID, path); reason != "" {
		candidate.BlockedReason = reason
		candidate.Outcome = "blocked"
		return candidate, identity
	}
	candidate.SafeToDelete = true
	candidate.Outcome = "eligible"
	return candidate, identity
}

func loadMaintenanceAssetRecord(db *gorm.DB, operation models.AssetMaintenanceOperation, details *HousekeepingMaintenanceDetails) error {
	if operation.AssetRecordID == 0 || !db.Migrator().HasTable(&models.AssetRecord{}) {
		return errors.New("maintenance asset identity could not be verified")
	}
	var record models.AssetRecord
	result := db.Select("id, path, status, missing").Where("id = ?", operation.AssetRecordID).Limit(1).Find(&record)
	if result.Error != nil {
		return errors.New("maintenance asset identity could not be verified")
	}
	if result.RowsAffected == 0 || !sameHousekeepingPath(record.Path, operation.AssetPath) {
		return errors.New("maintenance operation no longer matches its AssetRecord path")
	}
	details.AssetRecordStatus = record.Status
	details.AssetRecordMissing = record.Missing
	return nil
}

func classifyLibraryMaintenanceArtifact(db *gorm.DB, roots map[string]string, operation models.AssetMaintenanceOperation, candidate HousekeepingCandidate, temporaryInfo os.FileInfo) error {
	details := candidate.Maintenance
	if unsafeLibraryRoleRoot(roots) {
		return errors.New("configured Library storage role overlaps another storage role")
	}
	expectedTemporary, expectedBackup, pathsErr := expectedMaintenanceArtifactPaths(operation)
	if pathsErr != nil || !sameHousekeepingPath(expectedTemporary, operation.TemporaryPath) || !sameHousekeepingPath(expectedBackup, operation.BackupPath) {
		return errors.New("maintenance temporary or backup path does not match the exact operation naming pattern")
	}
	for _, path := range []string{operation.TemporaryPath, operation.BackupPath, operation.AssetPath} {
		symlink, err := hasSymlinkWithinRole(path, roots["library"])
		if err != nil {
			details.RecoveryState = "path components could not be inspected"
			return errors.New("maintenance path contains an uninspectable component")
		}
		if symlink {
			details.RecoveryState = "path contains a symlink under Library"
			return errors.New("maintenance temporary, backup, or asset path contains a Library symlink component")
		}
	}
	if operation.BackupPath == "" {
		details.BackupPresence = "unknown"
		details.RecoveryState = "backup path is not recorded"
		return errors.New("maintenance backup path is not recorded")
	}
	_, backupPath, backupInfo, backupErr := resolveOwnedFilePath(roots, operation.BackupPath, []string{"library"})
	if backupErr != nil || !sameHousekeepingPath(backupPath, operation.BackupPath) {
		details.BackupPresence = "unknown"
		details.RecoveryState = "backup path could not be safely resolved"
		return errors.New("maintenance backup path could not be safely resolved under the configured Library role")
	}
	if backupInfo != nil {
		details.BackupPresence = "present"
		details.RecoveryState = "recovery backup remains present"
		return errors.New("maintenance recovery backup is still present and must be preserved")
	}
	details.BackupPresence = "absent"
	if operationIsActive(operation.Status) {
		details.RecoveryState = "active operation"
		return errors.New("maintenance operation is still active and may require recovery")
	}
	if strings.ToLower(strings.TrimSpace(operation.Status)) != "completed" || strings.ToLower(strings.TrimSpace(operation.Phase)) != "completed" || operation.FinishedAt == nil {
		details.RecoveryState = "failed, interrupted, or unconfirmed operation"
		return errors.New("maintenance operation is not a confirmed completed operation; retain the output for recovery or inspection")
	}
	if details.Warning != "" {
		warning := strings.ToLower(details.Warning)
		if strings.Contains(warning, "backup") || strings.Contains(warning, "rollback") || strings.Contains(warning, "recover") || strings.Contains(warning, "restore") {
			details.RecoveryState = "operation warning indicates unresolved backup or recovery work"
			return errors.New("maintenance operation has an unresolved backup or recovery warning")
		}
	}
	if temporaryInfo != nil && (!temporaryInfo.Mode().IsRegular() || temporaryInfo.Mode()&os.ModeSymlink != 0) {
		details.RecoveryState = "temporary path is not a regular file"
		return errors.New("Library maintenance temporary output must be a regular non-symlink file")
	}
	if temporaryInfo == nil {
		details.RecoveryState = "completed operation; temporary output is already absent"
		return nil
	}
	if strings.TrimSpace(operation.ResultFingerprint) == "" {
		details.RecoveryState = "completed result fingerprint is unavailable"
		return errors.New("completed maintenance result fingerprint is unavailable")
	}
	if details.AssetRecordMissing {
		details.RecoveryState = "AssetRecord is marked missing"
		return errors.New("maintenance AssetRecord is marked missing; reconcile inventory before cleanup")
	}
	if err := validateLibraryMaintenanceAsset(roots, operation.AssetPath); err != nil {
		details.RecoveryState = "current Library asset could not be verified"
		return err
	}
	details.RecoveryState = "completed result and recovery paths verified"
	return nil
}

func expectedMaintenanceArtifactPaths(operation models.AssetMaintenanceOperation) (string, string, error) {
	if operation.ID == "" || filepath.Base(operation.ID) != operation.ID || operation.AssetPath == "" || !filepath.IsAbs(operation.AssetPath) || filepath.Clean(operation.AssetPath) != operation.AssetPath {
		return "", "", errors.New("maintenance operation identity or asset path is unsafe")
	}
	extension := filepath.Ext(operation.AssetPath)
	temporary := filepath.Join(filepath.Dir(operation.AssetPath), "."+filepath.Base(operation.AssetPath)+"."+operation.ID+".tmp"+extension)
	backup := filepath.Join(filepath.Dir(operation.AssetPath), "."+filepath.Base(operation.AssetPath)+"."+operation.ID+".backup"+extension)
	return temporary, backup, nil
}

func validateLibraryMaintenanceAsset(roots map[string]string, path string) error {
	role, resolved, info, err := resolveOwnedFilePath(roots, path, []string{"library"})
	if err != nil || role != "library" || !sameHousekeepingPath(path, resolved) {
		return errors.New("maintenance asset path is not safely contained by the configured Library role")
	}
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("canonical Library asset is missing or is not a regular non-symlink file")
	}
	return nil
}

func fingerprintLibraryMaintenanceAsset(roots map[string]string, path string) (string, os.FileInfo, error) {
	if err := validateLibraryMaintenanceAsset(roots, path); err != nil {
		return "", nil, err
	}
	return fingerprintSafeMaintenanceFile(path)
}

func fingerprintSafeMaintenanceFile(path string) (string, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("maintenance output must be a regular non-symlink file")
	}
	links, known := housekeepingFileLinkCount(before)
	if !known || links != 1 {
		return "", nil, errors.New("maintenance output has an unsafe or unverifiable hard-link count")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return "", nil, errors.New("maintenance output changed while it was opened")
	}
	fingerprint, err := mediaFileFingerprintForOpenFile(file, opened)
	if err != nil {
		return "", nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) || statIdentity(before, false) != statIdentity(after, false) {
		return "", nil, errors.New("maintenance output changed while its fingerprint was calculated")
	}
	links, known = housekeepingFileLinkCount(after)
	if !known || links != 1 {
		return "", nil, errors.New("maintenance output acquired an unsafe or unverifiable hard link")
	}
	return fingerprint, after, nil
}

func hasSymlinkWithinRole(path, root string) (bool, error) {
	if path == "" || root == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false, errors.New("path is not absolute and clean")
	}
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	remainder := strings.TrimPrefix(path, current)
	for _, component := range strings.Split(remainder, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		resolved, err := filepath.EvalSymlinks(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if !pathWithin(root, resolved) {
			continue
		}
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

func housekeepingFileLinkCount(info os.FileInfo) (uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return 0, false
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName("Nlink")
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() < 0 {
			return 0, false
		}
		return uint64(field.Int()), true
	default:
		return 0, false
	}
}

func unsafeLibraryRoleRoot(roots map[string]string) bool {
	library := roots["library"]
	if library == "" {
		return true
	}
	for role, other := range roots {
		if role != "library" && other != "" && (pathWithin(library, other) || pathWithin(other, library)) {
			return true
		}
	}
	return false
}

func publisherStageArtifactCandidate(db *gorm.DB, roots map[string]string, publication models.PublisherPublication, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, string) {
	candidate := HousekeepingCandidate{Kind: housekeepingKindStageArtifact, OperationID: publication.PublicationID, JobID: publication.QueueJobID, Path: publication.OriginalArchiveTemporaryPath, Ownership: "publisher_publication.original_archive_temporary_path", Reason: "abandoned Publisher staging artifact", ModifiedAt: publication.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	identity := "unresolved"
	if publication.OriginalArchiveTemporaryPath == "" {
		candidate.BlockedReason = "publication has no persisted staging path"
		return candidate, identity
	}
	if !publisherArchiveStageIsAbandoned(publication) {
		candidate.BlockedReason = "Publisher archive lifecycle may still require recovery"
		return candidate, identity
	}
	if publication.OriginalArchiveLeaseExpiresAt != nil && publication.OriginalArchiveLeaseExpiresAt.After(now) {
		candidate.BlockedReason = "Publisher archive lease is active"
		return candidate, identity
	}
	if now.Before(publication.UpdatedAt.Add(time.Duration(max(policy.OrphanRetentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "abandoned artifact retention period has not elapsed"
	}
	role, path, info, pathErr := resolveOwnedFilePath(roots, publication.OriginalArchiveTemporaryPath, []string{"originals_archive"})
	if pathErr != nil {
		candidate.BlockedReason = pathErr.Error()
		return candidate, identity
	}
	candidate.Path, candidate.Ownership = path, "publisher_publication temporary path under "+role+" storage role"
	for _, protected := range []string{publication.OriginalArchivedPath, publication.OriginalArchiveSourcePath, publication.SourcePath, publication.DestinationPath} {
		if protected != "" && path == filepath.Clean(protected) {
			candidate.BlockedReason = "staging path aliases a canonical publication or archived original"
			return candidate, identity
		}
	}
	if info == nil {
		candidate.SafeToDelete = candidate.BlockedReason == ""
		return candidate, "missing"
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		candidate.BlockedReason = "publisher staging artifacts must be regular non-symlink files"
		return candidate, statIdentity(info, false)
	}
	candidate.SizeBytes = info.Size()
	identity = statIdentity(info, false)
	if reason := publisherArtifactReferenceBlock(db, publication.PublicationID, path); reason != "" {
		candidate.BlockedReason = reason
		return candidate, identity
	}
	if candidate.BlockedReason == "" {
		candidate.SafeToDelete = true
	}
	return candidate, identity
}

func addTestEncodeCandidates(db *gorm.DB, now time.Time, state *housekeepingPreviewState) error {
	if !db.Migrator().HasTable(&models.TestEncode{}) {
		return nil
	}
	var tests []models.TestEncode
	if err := db.Where("deleted_at IS NULL AND keep = ? AND expires_at IS NOT NULL AND expires_at <= ? AND status NOT IN ?", false, now, []string{testEncodeDeleted, "expired"}).Order("expires_at ASC").Order("id ASC").Limit(100).Find(&tests).Error; err != nil {
		return err
	}
	for _, test := range tests {
		candidate := HousekeepingCandidate{Kind: housekeepingKindTestEncode, Path: test.OutputPath, TestEncodeID: test.ID, Ownership: "test_encode_artifact persisted owner", Reason: "Test Encode retention expired", ModifiedAt: test.UpdatedAt.UTC().Format(time.RFC3339Nano)}
		diskUsage, usageErr := (AssetHandler{db: db}).testEncodeOwnedArtifactDiskUsage(test)
		identity, identityErr := testEncodeCandidateIdentity(db, test)
		if usageErr == nil && identityErr != nil {
			usageErr = identityErr
		}
		if usageErr != nil {
			candidate.BlockedReason = usageErr.Error()
		} else {
			candidate.SizeBytes = diskUsage
			candidate.SafeToDelete = test.Status != testEncodeWaiting && test.Status != testEncodeRunning && !testEncodeOperationActive(test.ID)
			if !candidate.SafeToDelete {
				candidate.BlockedReason = "Test Encode is active or has an in-process owner"
			}
		}
		addBoundCandidate(state, candidate, identity)
	}
	return nil
}

func addRuntimeSnapshotCandidates(db *gorm.DB, now time.Time, state *housekeepingPreviewState) error {
	if !db.Migrator().HasTable(&models.RuntimeSnapshot{}) {
		return nil
	}
	plan, err := resolveRuntimeSnapshotRetention(db, now, NewRuntimeSnapshotReferenceRegistry())
	if err != nil {
		return err
	}
	state.RuntimeSnapshots = plan.RuntimeSnapshotRetentionReport
	for _, snapshot := range plan.candidates {
		path := fmt.Sprintf("runtime-snapshot/%d", snapshot.ID)
		candidate := HousekeepingCandidate{Kind: housekeepingKindRuntimeSnapshot, Path: path, OperationID: strconv.FormatUint(uint64(snapshot.ID), 10), Ownership: "runtime_snapshot_retention registry", Reason: "old unreferenced runtime snapshot", ModifiedAt: snapshot.DetectedAt.UTC().Format(time.RFC3339Nano), SafeToDelete: true}
		addBoundCandidate(state, candidate, fmt.Sprintf("runtime:%d:%d", snapshot.ID, snapshot.DetectedAt.UnixNano()))
	}
	return nil
}

func addEmptyDirectoryCandidates(db *gorm.DB, roots map[string]string, unsafeRaw, rawEnabled bool, state *housekeepingPreviewState) error {
	removedFiles := map[string]struct{}{}
	removedWorkspaces := []string{}
	protectedWorkspaces := []string{}
	for _, bound := range state.Candidates {
		candidate := bound.Candidate
		switch candidate.Kind {
		case housekeepingKindWorkspace:
			if candidate.SafeToDelete {
				removedWorkspaces = append(removedWorkspaces, candidate.Path)
			} else {
				protectedWorkspaces = append(protectedWorkspaces, candidate.Path)
			}
		case housekeepingKindJobLog, housekeepingKindStageArtifact:
			if candidate.SafeToDelete {
				removedFiles[candidate.Path] = struct{}{}
			}
		}
	}
	sharedPlannedDirectories := map[string]struct{}{}
	seenDirectories := map[string]struct{}{}
	if rawEnabled {
		protectionReferences, protectionReferenceCounts, err := protectedRawDirectoryReferences(db, roots[string(scheduler.StorageRoleRaw)])
		if err != nil {
			return err
		}
		protectedRaw := make(map[string]struct{}, len(protectionReferences))
		for path := range protectionReferences {
			protectedRaw[path] = struct{}{}
		}
		rawRoot := roots[string(scheduler.StorageRoleRaw)]
		if err := addEmptyDirectoryRoleCandidates(rawRoot, "raw", roots, true, unsafeRaw, nil, nil, protectedRaw, protectionReferences, protectionReferenceCounts, sharedPlannedDirectories, seenDirectories, state); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, role := range []string{"logs", "reports", "cache", "work"} {
		root := roots[role]
		if root == "" {
			continue
		}
		protected, err := protectedOperationalDirectories(db, root, role == "work")
		if err != nil {
			return err
		}
		if role == "work" {
			for _, path := range protectedWorkspaces {
				protectHousekeepingPathAndParents(protected, root, path)
			}
		}
		for _, roleRoot := range roots {
			if pathWithin(root, roleRoot) {
				protectHousekeepingPathAndParents(protected, root, roleRoot)
			}
		}
		if err := addEmptyDirectoryRoleCandidates(root, role, roots, false, unsafeOperationalRoleRoot(roots, role), removedFiles, removedWorkspaces, protected, nil, nil, sharedPlannedDirectories, seenDirectories, state); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func addEmptyDirectoryRoleCandidates(root, role string, allRoots map[string]string, raw, unsafeRoot bool, removedFiles map[string]struct{}, removedWorkspaces []string, protected map[string]struct{}, protectionReferences map[string][]HousekeepingProtectionReference, protectionReferenceCounts map[string]int, plannedDirectories, seenDirectories map[string]struct{}, state *housekeepingPreviewState) error {
	if root == "" {
		return nil
	}
	directories := []string{}
	symlinkDirectories := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if raw {
				if target, statErr := os.Stat(path); statErr == nil && target.IsDir() {
					symlinkDirectories = append(symlinkDirectories, path)
				}
			}
			return nil
		}
		if entry.IsDir() && filepath.Clean(path) != filepath.Clean(root) {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, path := range symlinkDirectories {
		if _, exists := seenDirectories[filepath.Clean(path)]; exists {
			continue
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		candidate := HousekeepingCandidate{
			Kind: housekeepingKindEmptyDirectory, Path: path, OperationID: role,
			Ownership:  "empty_directory under " + role + " storage role",
			Reason:     "symlink directory path is never followed or removed",
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), SafeToDelete: false,
			Outcome: "blocked", BlockedReason: "RAW directory path is a symlink and cannot be safely inspected",
		}
		if unsafeRoot {
			candidate.BlockedReason = "configured RAW storage role overlaps another storage role"
		}
		addBoundCandidate(state, candidate, statIdentity(info, false))
		seenDirectories[filepath.Clean(path)] = struct{}{}
	}
	sort.Slice(directories, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(directories[i]), pathDepth(directories[j])
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directories[i] < directories[j]
	})
	for _, path := range directories {
		if _, exists := seenDirectories[filepath.Clean(path)]; exists {
			continue
		}
		if isConfiguredRoleRoot(allRoots, path) {
			seenDirectories[filepath.Clean(path)] = struct{}{}
			continue
		}
		insideWorkspace := false
		for _, workspace := range removedWorkspaces {
			if pathWithin(workspace, path) {
				insideWorkspace = true
				break
			}
		}
		if insideWorkspace {
			continue
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		emptyAfterPlan, currentEmpty, emptyErr := directoryWillBeEmpty(path, removedFiles, plannedDirectories)
		if emptyErr != nil || !emptyAfterPlan {
			continue
		}
		candidate := HousekeepingCandidate{
			Kind: housekeepingKindEmptyDirectory, Path: path, OperationID: role,
			Ownership: "empty_directory under " + role + " storage role",
			Reason:    "directory is literally empty", ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		}
		if !currentEmpty {
			candidate.Reason = "directory will be empty after previewed owned-file cleanup"
		}
		if _, isProtected := protected[filepath.Clean(path)]; isProtected {
			candidate.BlockedReason = "directory is referenced by a current or stale asset, Queue source/restore target, or active operation"
			candidate.Outcome = "blocked"
			candidate.ProtectionReferences = protectionReferences[filepath.Clean(path)]
			candidate.ProtectionReferenceCount = protectionReferenceCounts[filepath.Clean(path)]
			if len(candidate.ProtectionReferences) > 0 {
				candidate.BlockedReason = summarizeRawDirectoryProtection(candidate.ProtectionReferences, candidate.ProtectionReferenceCount)
			}
		} else {
			candidate.SafeToDelete = true
			candidate.Outcome = "eligible"
			plannedDirectories[filepath.Clean(path)] = struct{}{}
		}
		if raw && !currentEmpty && !emptyAfterPlan {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "RAW directories are removable only when literally empty at preview and execution time"
		} else if raw && !currentEmpty && candidate.SafeToDelete {
			candidate.Reason = "directory will be empty after previewed empty child directories are removed"
		}
		if unsafeRoot {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "configured RAW storage role overlaps another storage role"
			candidate.Outcome = "blocked"
		}
		addBoundCandidate(state, candidate, statIdentity(info, true))
		seenDirectories[filepath.Clean(path)] = struct{}{}
	}
	return nil
}

func directoryWillBeEmpty(path string, removedFiles, removedDirectories map[string]struct{}) (emptyAfterPlan, currentEmpty bool, err error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, false, err
	}
	if len(entries) == 0 {
		return true, true, nil
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if _, ok := removedFiles[filepath.Clean(child)]; ok {
			continue
		}
		if _, ok := removedDirectories[filepath.Clean(child)]; ok {
			continue
		}
		return false, false, nil
	}
	return true, false, nil
}

func protectedRawDirectories(db *gorm.DB, root string) (map[string]struct{}, error) {
	references, _, err := protectedRawDirectoryReferences(db, root)
	if err != nil {
		return nil, err
	}
	protected := make(map[string]struct{}, len(references))
	for path := range references {
		protected[path] = struct{}{}
	}
	return protected, nil
}

func protectedRawDirectoryReferences(db *gorm.DB, root string) (map[string][]HousekeepingProtectionReference, map[string]int, error) {
	references := map[string][]HousekeepingProtectionReference{}
	counts := map[string]int{}
	if root == "" {
		return references, counts, nil
	}
	root = canonicalHousekeepingPath(root)
	add := func(path string, directory bool, reference HousekeepingProtectionReference) {
		if strings.TrimSpace(path) == "" {
			return
		}
		path = canonicalHousekeepingPath(path)
		if path == "" || !filepath.IsAbs(path) {
			return
		}
		if !directory {
			path = filepath.Dir(path)
		}
		for pathWithin(root, path) {
			key := filepath.Clean(path)
			items := references[key]
			duplicate := false
			for _, current := range items {
				if current.Source == reference.Source && current.RecordID == reference.RecordID && current.Field == reference.Field && current.Path == reference.Path {
					duplicate = true
					break
				}
			}
			if !duplicate {
				counts[key]++
				if len(items) < 20 {
					references[key] = append(items, reference)
				}
			}
			if key == filepath.Clean(root) {
				break
			}
			path = filepath.Dir(path)
		}
	}
	if db.Migrator().HasTable(&models.AssetRecord{}) {
		var assets []models.AssetRecord
		if err := db.Select("id, path, source_path, root_path, group_path, status, missing").Find(&assets).Error; err != nil {
			return nil, nil, err
		}
		for _, asset := range assets {
			state := "current"
			reason := "AssetRecord still claims this path; verify it through the canonical asset scan if stale."
			if asset.Missing {
				state = "stale_reference_suspected"
				reason = "AssetRecord is marked missing; re-scan or reconcile inventory before removing this directory."
			}
			for _, field := range []struct {
				name      string
				path      string
				directory bool
			}{{"path", asset.Path, false}, {"source_path", asset.SourcePath, false}, {"root_path", asset.RootPath, true}, {"group_path", asset.GroupPath, true}} {
				if field.path == "" {
					continue
				}
				fieldState, fieldReason := state, reason
				if !asset.Missing && !field.directory && (field.name == "path" || field.name == "source_path") {
					info, statErr := os.Lstat(field.path)
					if errors.Is(statErr, os.ErrNotExist) {
						fieldState = "stale_reference_suspected"
						fieldReason = "AssetRecord path is absent while the record is not marked missing; re-scan or reconcile inventory before cleanup."
					} else if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
						fieldState = "evidence_unresolved"
						fieldReason = "AssetRecord filesystem identity could not be verified; reconcile inventory before cleanup."
					}
				}
				add(field.path, field.directory, HousekeepingProtectionReference{
					Source: "AssetRecord", RecordID: strconv.FormatUint(uint64(asset.ID), 10), Field: field.name,
					Path: field.path, State: fieldState, Reason: fieldReason,
				})
			}
		}
	}

	reservationsByJob := map[uint][]models.SchedulerReservation{}
	if db.Migrator().HasTable(&models.SchedulerReservation{}) {
		var reservations []models.SchedulerReservation
		if err := db.Where("state IN ?", []string{scheduler.ReservationStateLocked, scheduler.ReservationStateActive}).Select("id, job_id, asset_key, state").Find(&reservations).Error; err != nil {
			return nil, nil, err
		}
		for _, reservation := range reservations {
			reservationsByJob[reservation.JobID] = append(reservationsByJob[reservation.JobID], reservation)
			add(reservation.AssetKey, false, HousekeepingProtectionReference{
				Source: "SchedulerReservation", RecordID: strconv.FormatUint(uint64(reservation.ID), 10),
				Field: "asset_key", Path: reservation.AssetKey, State: reservation.State,
				Reason: "Active scheduler reservation retains this source or restore path.",
			})
		}
	}

	publicationsByJob := map[uint][]models.PublisherPublication{}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		now := time.Now().UTC()
		if err := db.Where("TRIM(active_attempt_id) <> '' OR lease_expires_at > ? OR original_archive_lease_expires_at > ?", now, now).
			Select("id, publication_id, queue_job_id, source_path, destination_path, original_archived_path, original_archive_source_path, original_archive_temporary_path, active_attempt_id, lease_expires_at, original_archive_lease_expires_at, state").Find(&publications).Error; err != nil {
			return nil, nil, err
		}
		for _, publication := range publications {
			publicationsByJob[publication.QueueJobID] = append(publicationsByJob[publication.QueueJobID], publication)
			for _, field := range []struct{ name, path string }{
				{"source_path", publication.SourcePath}, {"destination_path", publication.DestinationPath},
				{"original_archived_path", publication.OriginalArchivedPath}, {"original_archive_source_path", publication.OriginalArchiveSourcePath},
				{"original_archive_temporary_path", publication.OriginalArchiveTemporaryPath},
			} {
				add(field.path, false, HousekeepingProtectionReference{
					Source: "PublisherPublication", RecordID: publication.PublicationID, Field: field.name,
					Path: field.path, State: publication.State, Reason: "Active Publisher attempt or lease retains this publication path.",
				})
			}
		}
	}

	if db.Migrator().HasTable(&models.QueueJob{}) {
		var jobs []models.QueueJob
		if err := db.Select("id, media_path, output_path, published_path, planned_published_path, replacement_target_path, original_archived_path, subtitle_artifacts, status, stage, validation_status, validation_outcome, validation_actionable, validation_override, published_at, started_at, finished_at").Find(&jobs).Error; err != nil {
			return nil, nil, err
		}
		for _, job := range jobs {
			reservations := reservationsByJob[job.ID]
			publications := publicationsByJob[job.ID]
			if !jobHasActiveHousekeepingReferencesFromJob(&job) && len(reservations) == 0 && len(publications) == 0 {
				continue
			}
			state := strings.TrimSpace(job.Status)
			if strings.TrimSpace(job.Stage) != "" {
				state += "/" + strings.TrimSpace(job.Stage)
			}
			reason := "QueueJob lifecycle still requires its source, result, or restore paths."
			for _, field := range []struct{ name, path string }{
				{"media_path", job.MediaPath}, {"output_path", job.OutputPath}, {"published_path", job.PublishedPath},
				{"planned_published_path", job.PlannedPublishedPath}, {"replacement_target_path", job.ReplacementTargetPath},
				{"original_archived_path", job.OriginalArchivedPath},
			} {
				add(field.path, false, HousekeepingProtectionReference{
					Source: "QueueJob", RecordID: strconv.FormatUint(uint64(job.ID), 10), Field: field.name,
					Path: field.path, State: state, Reason: reason,
				})
				for _, reservation := range reservations {
					add(field.path, false, HousekeepingProtectionReference{
						Source: "SchedulerReservation", RecordID: strconv.FormatUint(uint64(reservation.ID), 10),
						Field: "QueueJob." + field.name, Path: field.path, State: reservation.State,
						Reason: "Active scheduler reservation retains this Queue source or restore path.",
					})
				}
				for _, publication := range publications {
					add(field.path, false, HousekeepingProtectionReference{
						Source: "PublisherPublication", RecordID: publication.PublicationID,
						Field: "QueueJob." + field.name, Path: field.path, State: publication.State,
						Reason: "Active Publisher attempt or lease retains this Queue source or restore path.",
					})
				}
			}
			walkHousekeepingJSONStrings(job.SubtitleArtifacts, func(path string) {
				add(path, false, HousekeepingProtectionReference{
					Source: "QueueJob", RecordID: strconv.FormatUint(uint64(job.ID), 10), Field: "subtitle_artifacts",
					Path: path, State: state, Reason: reason,
				})
			})
		}
	}

	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Select("id, asset_path, temporary_path, backup_path, status, phase, warning").Find(&operations).Error; err != nil {
			return nil, nil, err
		}
		for _, operation := range operations {
			backupNeedsRecovery := false
			if strings.TrimSpace(operation.BackupPath) != "" {
				if _, err := os.Lstat(operation.BackupPath); err == nil || !errors.Is(err, os.ErrNotExist) {
					backupNeedsRecovery = true
				}
			}
			warning := strings.ToLower(operation.Warning)
			recoveryWarning := strings.Contains(warning, "backup") || strings.Contains(warning, "rollback") || strings.Contains(warning, "recover") || strings.Contains(warning, "restore")
			if !operationIsActive(operation.Status) && !backupNeedsRecovery && !recoveryWarning {
				continue
			}
			reason := "Active or recovery-required maintenance ownership retains this path."
			if backupNeedsRecovery || recoveryWarning {
				reason = "Maintenance backup or recovery warning retains this path until recovery is resolved."
			}
			for _, field := range []struct{ name, path string }{{"asset_path", operation.AssetPath}, {"temporary_path", operation.TemporaryPath}, {"backup_path", operation.BackupPath}} {
				add(field.path, false, HousekeepingProtectionReference{
					Source: "AssetMaintenanceOperation", RecordID: operation.ID, Field: field.name,
					Path: field.path, State: operation.Status + "/" + operation.Phase,
					Reason: reason,
				})
			}
		}
	}
	for path := range references {
		sort.Slice(references[path], func(i, j int) bool {
			left, right := references[path][i], references[path][j]
			if left.Source != right.Source {
				return left.Source < right.Source
			}
			if left.RecordID != right.RecordID {
				return left.RecordID < right.RecordID
			}
			if left.Field != right.Field {
				return left.Field < right.Field
			}
			return left.Path < right.Path
		})
	}
	return references, counts, nil
}

func summarizeRawDirectoryProtection(references []HousekeepingProtectionReference, total int) string {
	if len(references) == 0 {
		return "directory protection evidence could not be resolved; re-scan or reconcile inventory before cleanup"
	}
	parts := make([]string, 0, min(len(references), 3))
	for _, reference := range references[:min(len(references), 3)] {
		parts = append(parts, fmt.Sprintf("%s #%s %s (%s)", reference.Source, reference.RecordID, reference.Field, reference.State))
	}
	message := "directory is protected by " + strings.Join(parts, "; ")
	if total > len(parts) {
		message += fmt.Sprintf("; %d additional references", total-len(parts))
	}
	return message + ". Re-scan or reconcile inventory to verify stale references before cleanup."
}

func protectedOperationalDirectories(db *gorm.DB, root string, includeQueueWorkspaces bool) (map[string]struct{}, error) {
	protected := map[string]struct{}{}
	if root == "" {
		return protected, nil
	}
	root = canonicalHousekeepingPath(root)
	if includeQueueWorkspaces && db.Migrator().HasTable(&models.QueueJob{}) {
		var jobs []models.QueueJob
		if err := db.Select("id, media_path, worker_name, status, stage, published_at").Find(&jobs).Error; err != nil {
			return nil, err
		}
		for _, job := range jobs {
			if !isProtectedHousekeepingJob(&job) && !jobHasActiveHousekeepingReferences(db, job.ID, filepath.Join(root, fmt.Sprintf("job-%d", job.ID))) {
				continue
			}
			protectHousekeepingPathAndParents(protected, root, filepath.Join(root, fmt.Sprintf("job-%d", job.ID)))
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("status IN ?", []string{"queued", "running"}).Select("asset_path, temporary_path, backup_path").Find(&operations).Error; err != nil {
			return nil, err
		}
		for _, operation := range operations {
			for _, path := range []string{operation.AssetPath, operation.TemporaryPath, operation.BackupPath} {
				if strings.TrimSpace(path) == "" || !housekeepingPathWithin(root, path) {
					continue
				}
				protectHousekeepingPathAndParents(protected, root, path)
			}
		}
	}
	return protected, nil
}

func protectHousekeepingPathAndParents(protected map[string]struct{}, root, path string) {
	root = canonicalHousekeepingPath(root)
	path = canonicalHousekeepingPath(path)
	if !pathWithin(root, path) {
		return
	}
	for current := filepath.Clean(path); pathWithin(root, current); current = filepath.Dir(current) {
		protected[filepath.Clean(current)] = struct{}{}
		if filepath.Clean(current) == filepath.Clean(root) {
			break
		}
	}
}

func addBoundCandidate(state *housekeepingPreviewState, candidate HousekeepingCandidate, identity string) {
	if len(state.Candidates) >= housekeepingMaxCandidates {
		if !containsHousekeepingString(state.Errors, "candidate set limit reached; remaining entries were not previewed") {
			state.Errors = append(state.Errors, "candidate set limit reached; remaining entries were not previewed")
		}
		return
	}
	candidate.CandidateID = housekeepingCandidateID(candidate, identity)
	state.Candidates = append(state.Candidates, housekeepingBoundCandidate{Candidate: candidate, Identity: identity})
}

func housekeepingCandidateID(candidate HousekeepingCandidate, identity string) string {
	copy := candidate
	copy.CandidateID = ""
	material, _ := json.Marshal(struct {
		Candidate HousekeepingCandidate
		Identity  string
	}{copy, identity})
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:16])
}

func housekeepingReportForState(state housekeepingPreviewState, dryRun bool) HousekeepingReport {
	report := HousekeepingReport{
		RanAt: state.CreatedAt, DryRun: dryRun, Outcome: "pending", PreviewID: state.PreviewID,
		PreviewExpiresAt: &state.ExpiresAt, Candidates: make([]HousekeepingCandidate, 0, len(state.Candidates)),
		Summaries: []HousekeepingCategorySummary{}, RemovedPaths: []string{}, Errors: append([]string{}, state.Errors...), RuntimeSnapshots: state.RuntimeSnapshots,
	}
	for _, bound := range state.Candidates {
		report.Candidates = append(report.Candidates, bound.Candidate)
	}
	if !dryRun {
		report.PreviewID = ""
		report.PreviewExpiresAt = nil
	} else {
		report.Outcome = "preview"
	}
	report.Summaries = housekeepingSummaries(report.Candidates)
	return report
}

func housekeepingSummaries(candidates []HousekeepingCandidate) []HousekeepingCategorySummary {
	byKind := map[string]HousekeepingCategorySummary{}
	for _, candidate := range candidates {
		summary := byKind[candidate.Kind]
		summary.Kind = candidate.Kind
		summary.CandidateCount++
		if candidate.SafeToDelete {
			summary.RecoverableBytes += candidate.SizeBytes
		}
		byKind[candidate.Kind] = summary
	}
	summaries := make([]HousekeepingCategorySummary, 0, len(byKind))
	for _, summary := range byKind {
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Kind < summaries[j].Kind })
	return summaries
}

func applyHousekeepingPlan(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, state *housekeepingPreviewState, report *HousekeepingReport) {
	report.RuntimeSnapshots = state.RuntimeSnapshots
	report.Candidates = make([]HousekeepingCandidate, len(state.Candidates))
	var executionContext *housekeepingExecutionContext
	for _, bound := range state.Candidates {
		if bound.Candidate.Kind == housekeepingKindEmptyDirectory && bound.Candidate.OperationID == "raw" && bound.Candidate.SafeToDelete {
			executionContext = &housekeepingExecutionContext{}
			protectionReferences, protectionCounts, err := protectedRawDirectoryReferences(db, roots["raw"])
			executionContext.rawProtectionReferences = protectionReferences
			executionContext.rawProtectionCounts = protectionCounts
			executionContext.rawProtectionErr = err
			break
		}
	}
	for index, bound := range state.Candidates {
		candidate := bound.Candidate
		if candidate.SafeToDelete {
			updated, removed, err := executeHousekeepingCandidateWithContext(db, roots, policy, report.RanAt, bound, nil, executionContext)
			candidate = updated
			if err != nil {
				candidate.SafeToDelete = false
				candidate.Outcome = "failed"
				candidate.BlockedReason = err.Error()
				report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", candidate.Path, err))
			}
			if removed {
				candidate.SafeToDelete = false
				candidate.Outcome = "cleaned"
				report.RemovedPaths = append(report.RemovedPaths, candidate.Path)
				report.RecoveredBytes += candidate.SizeBytes
				if candidate.Kind == housekeepingKindRuntimeSnapshot {
					report.RuntimeSnapshots.Deleted++
				}
			} else if err == nil && candidate.SafeToDelete && candidate.Outcome == "" {
				candidate.SafeToDelete = false
				candidate.Outcome = "already_absent"
				candidate.SizeBytes = 0
			}
			if !candidate.SafeToDelete && candidate.Outcome == "eligible" {
				candidate.Outcome = "blocked"
			}
		}
		if candidate.Outcome == "" {
			if candidate.SafeToDelete {
				candidate.Outcome = "eligible"
			} else {
				candidate.Outcome = "blocked"
			}
		}
		report.Candidates[index] = candidate
	}
	report.Summaries = housekeepingSummaries(report.Candidates)
	report.Outcome = housekeepingReportOutcome(report.Candidates)
}

func housekeepingReportOutcome(candidates []HousekeepingCandidate) string {
	if len(candidates) == 0 {
		return "no_op"
	}
	cleaned, absent, blocked, failed := 0, 0, 0, 0
	for _, candidate := range candidates {
		switch candidate.Outcome {
		case "cleaned":
			cleaned++
		case "already_absent":
			absent++
		case "failed":
			failed++
		case "blocked":
			blocked++
		}
	}
	if cleaned > 0 {
		if cleaned != len(candidates) {
			return "partial"
		}
		return "cleaned"
	}
	if failed > 0 {
		return "failed"
	}
	if blocked > 0 {
		return "blocked"
	}
	if absent == len(candidates) {
		return "already_absent"
	}
	return "no_op"
}

func saveHousekeepingPreview(db *gorm.DB, state housekeepingPreviewState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	value := models.JSONMap{}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return db.Save(&models.AppSetting{Key: housekeepingPreviewKey, Value: value}).Error
}

func consumeHousekeepingPreview(db *gorm.DB, previewID string, policy HousekeepingPolicy, roots map[string]string, now time.Time) (housekeepingPreviewState, error) {
	var state housekeepingPreviewState
	err := db.Transaction(func(tx *gorm.DB) error {
		var setting models.AppSetting
		result := tx.Where("key = ?", housekeepingPreviewKey).Limit(1).Find(&setting)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errStaleHousekeepingPreview
		}
		data, err := json.Marshal(setting.Value)
		if err != nil {
			return errStaleHousekeepingPreview
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return errStaleHousekeepingPreview
		}
		if state.PreviewID != previewID || !now.Before(state.ExpiresAt) || !reflect.DeepEqual(state.Policy, policy) || !reflect.DeepEqual(state.RoleRoots, roots) {
			return errStaleHousekeepingPreview
		}
		deleted := tx.Where("key = ? AND updated_at = ?", housekeepingPreviewKey, setting.UpdatedAt).Delete(&models.AppSetting{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != 1 {
			return errStaleHousekeepingPreview
		}
		return nil
	})
	if err != nil {
		return housekeepingPreviewState{}, err
	}
	return state, nil
}

func newHousekeepingPreviewID() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func housekeepingRoleRoots(roles scheduler.StorageRoles) (map[string]string, error) {
	roots := make(map[string]string, len(roles))
	for role, config := range roles {
		path := strings.TrimSpace(config.Path)
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		absolute = filepath.Clean(absolute)
		if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
			absolute = filepath.Clean(resolved)
		} else if !errors.Is(resolveErr, os.ErrNotExist) {
			return nil, resolveErr
		}
		roots[string(role)] = absolute
	}
	return roots, nil
}

func unsafeRawRoleRoot(roots map[string]string) bool {
	raw := roots["raw"]
	if raw == "" {
		return false
	}
	for _, role := range []string{"library", "originals_archive", "work", "cache", "reports", "logs"} {
		other := roots[role]
		if other != "" && (pathWithin(raw, other) || pathWithin(other, raw)) {
			return true
		}
	}
	return false
}

func unsafeOperationalRoleRoot(roots map[string]string, role string) bool {
	root := roots[role]
	if root == "" {
		return false
	}
	if overlapsProtectedStorageRoot(roots, root) {
		return true
	}
	for _, otherRole := range []string{"work", "cache", "reports", "logs"} {
		if otherRole == role {
			continue
		}
		other := roots[otherRole]
		if other == "" || (!pathWithin(root, other) && !pathWithin(other, root)) {
			continue
		}
		if (role == "reports" && otherRole == "logs") || (role == "logs" && otherRole == "reports") {
			continue
		}
		return true
	}
	return false
}

func publisherArchiveStageIsAbandoned(publication models.PublisherPublication) bool {
	status := strings.ToLower(strings.TrimSpace(publication.OriginalArchiveStatus))
	if status == "complete" || status == "not_applicable" {
		return true
	}
	return publication.State == PublicationPublished && (status == "failed" || status == "interrupted" || status == "canceled" || status == "cancelled")
}

func coveredBySafeWorkspace(candidates []housekeepingBoundCandidate, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for _, bound := range candidates {
		candidate := bound.Candidate
		if candidate.Kind == housekeepingKindWorkspace && candidate.SafeToDelete && pathWithin(candidate.Path, path) {
			return true
		}
	}
	return false
}

func readRoleRoot(root string) ([]os.DirEntry, error) {
	if root == "" {
		return nil, os.ErrNotExist
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("configured storage role root is not a real directory: %s", root)
	}
	return os.ReadDir(root)
}

func resolveOwnedFilePath(roots map[string]string, path string, allowedRoles []string) (string, string, os.FileInfo, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", "", nil, errors.New("owned artifact path is not an absolute clean path")
	}
	resolved, err := resolveWithoutFollowingLeaf(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", nil, errors.New("artifact path contains an unsafe or escaping symlink")
	}
	if resolved == "" {
		resolved = filepath.Clean(path)
	}
	role := ""
	longest := -1
	for _, allowed := range allowedRoles {
		root := roots[allowed]
		if root == "" || !pathWithin(root, resolved) || filepath.Clean(root) == filepath.Clean(resolved) {
			continue
		}
		if len(root) > longest {
			role, longest = allowed, len(root)
		} else if len(root) == longest {
			return "", "", nil, errors.New("artifact storage role is ambiguous")
		}
	}
	if role == "" {
		return "", "", nil, errors.New("artifact is outside configured temporary storage roles")
	}
	if role != "originals_archive" && role != "raw" && role != "library" && overlapsProtectedStorageRoot(roots, resolved) {
		return "", "", nil, errors.New("artifact path overlaps RAW, Library, or originals archive storage")
	}
	if (role == "work" || role == "cache" || role == "reports" || role == "logs") && unsafeOperationalRoleRoot(roots, role) {
		return "", "", nil, errors.New("configured operational storage role overlaps another protected role")
	}
	if err != nil {
		return role, resolved, nil, nil
	}
	info, statErr := os.Lstat(resolved)
	if errors.Is(statErr, os.ErrNotExist) {
		return role, resolved, nil, nil
	}
	if statErr != nil {
		return "", "", nil, statErr
	}
	return role, resolved, info, nil
}

func resolveWithoutFollowingLeaf(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || filepath.Clean(absolute) != path {
		return "", errors.New("path is not clean")
	}
	parent := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if errors.Is(err, os.ErrNotExist) {
		return "", os.ErrNotExist
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func overlapsProtectedStorageRoot(roots map[string]string, path string) bool {
	for _, role := range []string{"raw", "library", "originals_archive"} {
		root := roots[role]
		if root == "" {
			continue
		}
		if pathWithin(root, path) || pathWithin(path, root) {
			return true
		}
	}
	return false
}

func pathWithin(root, path string) bool {
	root, path = filepath.Clean(root), filepath.Clean(path)
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isConfiguredRoleRoot(roots map[string]string, path string) bool {
	for _, root := range roots {
		if root != "" && filepath.Clean(root) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func pathDepth(path string) int {
	clean := filepath.Clean(path)
	depth := 0
	for clean != filepath.Dir(clean) {
		depth++
		clean = filepath.Dir(clean)
	}
	return depth
}

func statIdentity(info os.FileInfo, directory bool) string {
	dev, ino, hasStableID := filesystemDeviceInode(info)
	if directory {
		if hasStableID {
			return fmt.Sprintf("dir:%d:%d:%o", dev, ino, info.Mode().Perm())
		}
		return fmt.Sprintf("dir:%s:%d", info.Mode(), info.ModTime().UnixNano())
	}
	if hasStableID {
		return fmt.Sprintf("file:%d:%d:%o:%d:%d", dev, ino, info.Mode().Perm(), info.Size(), info.ModTime().UnixNano())
	}
	return fmt.Sprintf("file:%s:%d:%d:%d", info.Mode(), info.Size(), info.ModTime().UnixNano(), info.ModTime().Unix())
}

func filesystemDeviceInode(info os.FileInfo) (uint64, uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return 0, 0, false
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, 0, false
	}
	read := func(name string) (uint64, bool) {
		field := value.FieldByName(name)
		if !field.IsValid() {
			return 0, false
		}
		switch field.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return field.Uint(), true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return uint64(field.Int()), true
		default:
			return 0, false
		}
	}
	dev, devOK := read("Dev")
	ino, inoOK := read("Ino")
	return dev, ino, devOK && inoOK
}

func directoryTreeIdentity(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("unsafe workspace entry %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", relative, statIdentity(info, info.IsDir()))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func directorySizeNoSymlinks(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink found under workspace")
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		} else if !entry.IsDir() {
			return errors.New("special file found under workspace")
		}
		return nil
	})
	return total, err
}

func containsHousekeepingString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func housekeepingCandidateOrder(kind string) int {
	switch kind {
	case housekeepingKindStageArtifact, housekeepingKindJobLog, housekeepingKindTestEncode:
		return 0
	case housekeepingKindWorkspace:
		return 1
	case housekeepingKindRuntimeSnapshot:
		return 2
	case housekeepingKindEmptyDirectory:
		return 3
	default:
		return 4
	}
}

func executeHousekeepingCandidate(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, bound housekeepingBoundCandidate) (HousekeepingCandidate, bool, error) {
	return executeHousekeepingCandidateWithStaging(db, roots, policy, now, bound, nil)
}

func executeHousekeepingCandidateWithStaging(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, bound housekeepingBoundCandidate, explicit *explicitValidationStagingAuthorization) (HousekeepingCandidate, bool, error) {
	return executeHousekeepingCandidateWithContext(db, roots, policy, now, bound, explicit, nil)
}

type housekeepingExecutionContext struct {
	rawProtectionReferences map[string][]HousekeepingProtectionReference
	rawProtectionCounts     map[string]int
	rawProtectionErr        error
}

func executeHousekeepingCandidateWithContext(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, bound housekeepingBoundCandidate, explicit *explicitValidationStagingAuthorization, executionContext *housekeepingExecutionContext) (HousekeepingCandidate, bool, error) {
	candidate := bound.Candidate
	if candidate.CandidateID == "" || candidate.CandidateID != housekeepingCandidateID(candidate, bound.Identity) {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "preview candidate identity is invalid"
		return candidate, false, nil
	}
	if !candidate.SafeToDelete {
		return candidate, false, nil
	}
	if candidate.Kind == housekeepingKindRuntimeSnapshot {
		removed, reason, err := executeRuntimeSnapshotCandidate(db, candidate, bound.Identity, now)
		if reason != "" {
			candidate.SafeToDelete = false
			candidate.BlockedReason = reason
		}
		return candidate, removed, err
	}
	if candidate.Kind == housekeepingKindTestEncode {
		return executeTestEncodeCandidate(db, candidate, bound.Identity, now)
	}
	allowedRoles := []string{}
	switch candidate.Kind {
	case housekeepingKindWorkspace:
		allowedRoles = []string{"work"}
	case housekeepingKindJobLog:
		allowedRoles = []string{"logs"}
	case housekeepingKindEmptyDirectory:
		allowedRoles = []string{candidate.OperationID}
	case housekeepingKindStageArtifact:
		if strings.HasPrefix(candidate.Ownership, "publisher_publication") {
			allowedRoles = []string{"originals_archive"}
		} else if candidate.ManualOnly {
			if candidate.Maintenance == nil {
				candidate.SafeToDelete = false
				candidate.Outcome = "blocked"
				candidate.BlockedReason = "Library maintenance candidate details are missing"
				return candidate, false, nil
			}
			allowedRoles = []string{"library"}
		} else {
			allowedRoles = []string{"work", "cache", "reports", "logs"}
		}
	default:
		candidate.SafeToDelete = false
		candidate.BlockedReason = "unknown candidate kind"
		return candidate, false, nil
	}
	role, path, info, pathErr := resolveOwnedFilePath(roots, candidate.Path, allowedRoles)
	if pathErr != nil {
		candidate.SafeToDelete = false
		candidate.BlockedReason = pathErr.Error()
		return candidate, false, nil
	}
	if path != candidate.Path {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "candidate path resolved differently from the previewed path"
		return candidate, false, nil
	}
	if info == nil {
		candidate.SizeBytes = 0
		candidate.SafeToDelete = false
		candidate.Outcome = "already_absent"
		candidate.Reason = "owned artifact is already absent; no cleanup is required"
		if candidate.Maintenance != nil {
			candidate.Maintenance.TemporaryPresence = "absent"
			candidate.Maintenance.TemporarySizeKnown = true
		}
		return candidate, false, nil
	}
	identity := statIdentity(info, info.IsDir())
	if candidate.Kind == housekeepingKindWorkspace {
		identity, pathErr = directoryTreeIdentity(path)
		if pathErr != nil {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace contains an unsafe or unreadable entry"
			return candidate, false, nil
		}
	}
	if identity != bound.Identity {
		candidate.SafeToDelete = false
		candidate.Outcome = "blocked"
		candidate.BlockedReason = "path identity changed after preview"
		return candidate, false, nil
	}
	switch candidate.Kind {
	case housekeepingKindWorkspace:
		if explicit != nil {
			if candidate.JobID == 0 || candidate.Reason != "operator confirmed Validation Dismiss cleanup" {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "explicit cleanup authorization does not match this workspace candidate"
				return candidate, false, nil
			}
			if err := validateExplicitDismissedStagingIdentity(db, candidate.JobID, *explicit); err != nil {
				candidate.SafeToDelete = false
				candidate.BlockedReason = err.Error()
				return candidate, false, nil
			}
			if explicitValidationStagingHasActiveReferences(db, candidate.JobID, candidate.Path) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "active Worker, Queue, Publisher, maintenance, Test Encode, artifact, or other job reference appeared before cleanup"
				return candidate, false, nil
			}
			files, tree, total, manifestErr := validationStagingManifest(path)
			if manifestErr != nil || tree != explicit.Snapshot.TreeIdentity || total != candidate.SizeBytes || !reflect.DeepEqual(files, explicit.Snapshot.Files) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "workspace contents changed immediately before cleanup"
				return candidate, false, nil
			}
			removedPaths, recoveredBytes, removeErr := removeValidationStagingManifest(roots["work"], path, explicit.Snapshot.Files)
			candidate.SizeBytes = recoveredBytes
			if removeErr != nil {
				return candidate, false, &validationStagingPartialRemovalError{paths: removedPaths, bytes: recoveredBytes, cause: removeErr}
			}
			return candidate, true, nil
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace is no longer a real directory"
			return candidate, false, nil
		}
		id := candidate.JobID
		if id == 0 {
			parsed, parseErr := strconv.ParseUint(strings.TrimPrefix(filepath.Base(path), "job-"), 10, 64)
			if parseErr != nil || parsed == 0 {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "orphan workspace name is no longer canonical"
				return candidate, false, nil
			}
			id = uint(parsed)
		}
		current, eligible, err := housekeepingCandidate(db, id, path, info.ModTime(), policy, now)
		if err != nil {
			return candidate, false, err
		}
		if !eligible || !current.SafeToDelete || current.Reason != candidate.Reason {
			candidate.SafeToDelete = false
			candidate.BlockedReason = current.BlockedReason
			if candidate.BlockedReason == "" {
				candidate.BlockedReason = "workspace lifecycle or retention changed after preview"
			}
			return candidate, false, nil
		}
		currentSize, err := directorySizeNoSymlinks(path)
		if err != nil {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace became unsafe after preview"
			return candidate, false, nil
		}
		if currentSize != candidate.SizeBytes {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace size changed after preview"
			return candidate, false, nil
		}
		finalIdentity, identityErr := directoryTreeIdentity(path)
		if identityErr != nil || finalIdentity != bound.Identity {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace contents changed immediately before cleanup"
			return candidate, false, nil
		}
		if err := os.RemoveAll(path); err != nil {
			return candidate, false, err
		}
		return candidate, true, nil
	case housekeepingKindJobLog:
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "job log is no longer a regular non-symlink file"
			return candidate, false, nil
		}
		jobID := candidate.JobID
		if jobID == 0 {
			parsed, parseErr := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "job-"), ".log"), 10, 64)
			if parseErr != nil || parsed == 0 {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "orphan log name is no longer canonical"
				return candidate, false, nil
			}
			jobID = uint(parsed)
		}
		var job models.QueueJob
		result := db.Where("id = ?", jobID).Limit(1).Find(&job)
		if result.Error != nil {
			return candidate, false, result.Error
		}
		current := HousekeepingCandidate{Kind: housekeepingKindJobLog, Path: path, Reason: "orphan job log retention expired", ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), SizeBytes: info.Size(), SafeToDelete: true}
		if result.RowsAffected > 0 {
			current = classifyJobLog(job, path, info, policy, now)
		}
		if current.SafeToDelete && jobHasActiveHousekeepingReferences(db, jobID, "") {
			current.SafeToDelete = false
			current.BlockedReason = "active worker, reservation, or operation still owns this job"
		}
		if !current.SafeToDelete || current.Reason != candidate.Reason {
			candidate.SafeToDelete = false
			candidate.BlockedReason = current.BlockedReason
			if candidate.BlockedReason == "" {
				candidate.BlockedReason = "job lifecycle or log retention changed after preview"
			}
			return candidate, false, nil
		}
		removed, changed, err := removeHousekeepingFileIfUnchanged(path, bound.Identity)
		if changed {
			candidate.SafeToDelete = false
			candidate.Outcome = "blocked"
			candidate.BlockedReason = "job log changed immediately before cleanup"
		} else if err == nil && !removed {
			candidate.SafeToDelete = false
			candidate.Outcome = "already_absent"
			candidate.SizeBytes = 0
		}
		return candidate, removed, err
	case housekeepingKindStageArtifact:
		currentRoots := roots
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "stage artifact is no longer a regular non-symlink file"
			return candidate, false, nil
		}
		if strings.HasPrefix(candidate.Ownership, "publisher_publication") {
			current, err := publisherArtifactStillOwned(db, candidate, now)
			if err != nil {
				return candidate, false, err
			}
			if !current {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "Publisher staging ownership or archive state changed after preview"
				return candidate, false, nil
			}
		} else {
			if candidate.ManualOnly {
				roles, err := scheduler.LoadStorageRoles(db)
				if err != nil {
					return candidate, false, err
				}
				currentRoots, err = housekeepingRoleRoots(roles)
				if err != nil {
					return candidate, false, err
				}
				if !reflect.DeepEqual(currentRoots, roots) || unsafeLibraryRoleRoot(currentRoots) {
					candidate.SafeToDelete = false
					candidate.Outcome = "blocked"
					candidate.BlockedReason = "Library role roots changed or overlap another storage role after preview"
					return candidate, false, nil
				}
			}
			current, currentIdentity, err := maintenanceArtifactStillOwned(db, currentRoots, candidate, policy, now)
			if err != nil {
				return candidate, false, err
			}
			if !current.SafeToDelete || currentIdentity != bound.Identity || current.CandidateID != candidate.CandidateID {
				candidate.SafeToDelete = false
				candidate.Outcome = current.Outcome
				if current.SafeToDelete {
					candidate.Outcome = "blocked"
				}
				candidate.Maintenance = current.Maintenance
				candidate.BlockedReason = current.BlockedReason
				if candidate.BlockedReason == "" {
					candidate.BlockedReason = "operation, role, asset, reference, or file identity changed after preview"
				}
				return candidate, false, nil
			}
		}
		if candidate.ManualOnly {
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				candidate.SafeToDelete = false
				candidate.Outcome = "blocked"
				candidate.BlockedReason = "Library maintenance temporary output changed or is no longer a regular file"
				return candidate, false, nil
			}
			links, known := housekeepingFileLinkCount(info)
			if !known || links != 1 {
				candidate.SafeToDelete = false
				candidate.Outcome = "blocked"
				candidate.BlockedReason = "Library maintenance temporary output has an unsafe or unverifiable hard-link count"
				return candidate, false, nil
			}
			latestRoles, err := scheduler.LoadStorageRoles(db)
			if err != nil {
				return candidate, false, err
			}
			latestRoots, err := housekeepingRoleRoots(latestRoles)
			if err != nil {
				return candidate, false, err
			}
			if !reflect.DeepEqual(currentRoots, latestRoots) || unsafeLibraryRoleRoot(latestRoots) {
				candidate.SafeToDelete = false
				candidate.Outcome = "blocked"
				candidate.BlockedReason = "Library role roots changed or overlap another storage role immediately before cleanup"
				return candidate, false, nil
			}
		}
		removed, changed, err := removeHousekeepingFileIfUnchanged(path, bound.Identity)
		if changed {
			candidate.SafeToDelete = false
			candidate.Outcome = "blocked"
			candidate.BlockedReason = "stage artifact changed immediately before cleanup"
		} else if err == nil && !removed {
			candidate.SafeToDelete = false
			candidate.Outcome = "already_absent"
			candidate.SizeBytes = 0
			if candidate.Maintenance != nil {
				candidate.Maintenance.TemporaryPresence = "absent"
				candidate.Maintenance.TemporarySizeKnown = true
			}
		}
		return candidate, removed, err
	case housekeepingKindEmptyDirectory:
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "directory was replaced or became a symlink"
			return candidate, false, nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return candidate, false, err
		}
		if len(entries) != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "directory contains an entry at execution time"
			return candidate, false, nil
		}
		if role == "raw" {
			var protectionReferences map[string][]HousekeepingProtectionReference
			var protectionCounts map[string]int
			var protectionErr error
			if executionContext != nil {
				protectionReferences = executionContext.rawProtectionReferences
				protectionCounts = executionContext.rawProtectionCounts
				protectionErr = executionContext.rawProtectionErr
			} else {
				protectionReferences, protectionCounts, protectionErr = protectedRawDirectoryReferences(db, roots["raw"])
			}
			if protectionErr != nil {
				return candidate, false, protectionErr
			}
			if references, exists := protectionReferences[filepath.Clean(path)]; exists {
				candidate.SafeToDelete = false
				candidate.Outcome = "blocked"
				candidate.ProtectionReferences = references
				candidate.ProtectionReferenceCount = protectionCounts[filepath.Clean(path)]
				candidate.BlockedReason = summarizeRawDirectoryProtection(references, candidate.ProtectionReferenceCount)
				return candidate, false, nil
			}
			if unsafeRawRoleRoot(roots) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured RAW storage role overlaps Library or originals archive"
				return candidate, false, nil
			}
			if filepath.Clean(path) == filepath.Clean(roots["raw"]) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured RAW root is never removable"
				return candidate, false, nil
			}
		} else if role != "raw" {
			if unsafeOperationalRoleRoot(roots, role) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured operational storage role overlaps another protected role"
				return candidate, false, nil
			}
			protected, err := protectedOperationalDirectories(db, roots[role], role == "work")
			if err != nil {
				return candidate, false, err
			}
			if _, exists := protected[filepath.Clean(path)]; exists {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "work directory is referenced by an active job or operation"
				return candidate, false, nil
			}
		}
		if filepath.Clean(path) == filepath.Clean(roots[role]) || isConfiguredRoleRoot(roots, path) {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "configured storage role root is never removable"
			return candidate, false, nil
		}
		removed, changed, err := removeEmptyHousekeepingDirectoryIfUnchanged(path, bound.Identity)
		if changed {
			candidate.SafeToDelete = false
			candidate.Outcome = "blocked"
			candidate.BlockedReason = "directory became nonempty or changed immediately before cleanup"
		} else if err == nil && !removed {
			candidate.SafeToDelete = false
			candidate.Outcome = "already_absent"
			candidate.SizeBytes = 0
		}
		return candidate, removed, err
	}
	return candidate, false, nil
}

func removeHousekeepingFileIfUnchanged(path, identity string) (removed, changed bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || statIdentity(info, false) != identity {
		return false, true, nil
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	return true, false, nil
}

func removeEmptyHousekeepingDirectoryIfUnchanged(path, identity string) (removed, changed bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || statIdentity(info, true) != identity {
		return false, true, nil
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if len(entries) != 0 {
		return false, true, nil
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	return true, false, nil
}

func executeTestEncodeCandidate(db *gorm.DB, candidate HousekeepingCandidate, identity string, now time.Time) (HousekeepingCandidate, bool, error) {
	var test models.TestEncode
	result := db.Where("id = ?", candidate.TestEncodeID).Limit(1).Find(&test)
	if result.Error != nil {
		return candidate, false, result.Error
	}
	if result.RowsAffected == 0 || test.Keep || test.DeletedAt != nil || test.ExpiresAt == nil || test.ExpiresAt.After(now) || test.Status == testEncodeWaiting || test.Status == testEncodeRunning || test.Status == testEncodeDeleted || test.Status == "expired" || testEncodeOperationActive(test.ID) {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode is kept, active, missing, or no longer expired"
		return candidate, false, nil
	}
	if filepath.Clean(test.OutputPath) != filepath.Clean(candidate.Path) {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode output ownership changed after preview"
		return candidate, false, nil
	}
	currentIdentity, err := testEncodeCandidateIdentity(db, test)
	if err != nil {
		return candidate, false, err
	}
	if currentIdentity != identity {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode artifact identities changed after preview"
		return candidate, false, nil
	}
	diskUsage, err := (AssetHandler{db: db}).testEncodeOwnedArtifactDiskUsage(test)
	if err != nil {
		candidate.SafeToDelete = false
		candidate.BlockedReason = err.Error()
		return candidate, false, nil
	}
	if diskUsage != candidate.SizeBytes {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode artifact size changed after preview"
		return candidate, false, nil
	}
	mutated, cleanupErr := (AssetHandler{db: db}).removeTestEncodeFilesWithEvidence(test)
	if test.Status == testEncodeReady && mutated {
		_ = recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestRemoved, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, test.ID), LibraryID: test.LibraryID, TestEncodeID: test.ID, Reconcile: true})
	}
	if cleanupErr != nil {
		_ = db.Model(&test).Updates(map[string]any{"reconciliation_status": "cleanup_failed", "reconciliation_message": cleanupErr.Error(), "last_reconciled_at": now}).Error
		return candidate, false, cleanupErr
	}
	_ = db.Model(&test).Updates(map[string]any{"status": "expired", "phase": "expired", "deleted_at": &now, "output_path": "", "temporary_path": "", "subtitle_artifacts": models.JSONList{}, "updated_at": now}).Error
	return candidate, true, nil
}

func testEncodeCandidateIdentity(db *gorm.DB, test models.TestEncode) (string, error) {
	paths := []string{test.OutputPath, test.TemporaryPath}
	for _, raw := range test.SubtitleArtifacts {
		if path, ok := raw.(string); ok {
			paths = append(paths, path)
		}
	}
	if db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var artifacts []models.TestEncodeArtifact
		if err := db.Where("test_encode_id = ?", test.ID).Order("path ASC").Find(&artifacts).Error; err != nil {
			return "", err
		}
		for _, artifact := range artifacts {
			paths = append(paths, fmt.Sprintf("%d:%s:%s:%t", artifact.ID, artifact.Path, artifact.Type, artifact.Expected))
		}
	}
	sort.Strings(paths)
	material, err := json.Marshal(paths)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(material)
	diskUsage, err := (AssetHandler{db: db}).testEncodeOwnedArtifactDiskUsage(test)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("test-encode:%d:%s:%d", test.ID, hex.EncodeToString(sum[:]), diskUsage), nil
}

func executeRuntimeSnapshotCandidate(db *gorm.DB, candidate HousekeepingCandidate, identity string, now time.Time) (bool, string, error) {
	id, err := strconv.ParseUint(candidate.OperationID, 10, 64)
	if err != nil || id == 0 {
		return false, "runtime snapshot identity is invalid", nil
	}
	var snapshot models.RuntimeSnapshot
	result := db.Where("id = ?", uint(id)).Limit(1).Find(&snapshot)
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected == 0 {
		return true, "", nil
	}
	if identity != fmt.Sprintf("runtime:%d:%d", snapshot.ID, snapshot.DetectedAt.UnixNano()) {
		return false, "runtime snapshot identity changed after preview", nil
	}
	latestID, err := newestRuntimeSnapshotID(db)
	if err != nil {
		return false, "", err
	}
	if latestID == snapshot.ID || now.Before(snapshot.DetectedAt.Add(runtimeSnapshotRetentionAge)) {
		return false, "runtime snapshot is now latest or inside the retention window", nil
	}
	referenced, err := NewRuntimeSnapshotReferenceRegistry().isReferenced(db, snapshot.ID)
	if err != nil {
		return false, "", err
	}
	if referenced {
		return false, "runtime snapshot gained a historical execution reference", nil
	}
	deleted := db.Where("id = ? AND detected_at = ?", snapshot.ID, snapshot.DetectedAt).Delete(&models.RuntimeSnapshot{})
	return deleted.RowsAffected == 1, "", deleted.Error
}

func jobHasActiveHousekeepingReferences(db *gorm.DB, jobID uint, workspace string) bool {
	var job models.QueueJob
	jobResult := db.Where("id = ?", jobID).Limit(1).Find(&job)
	if jobResult.Error != nil {
		return true
	}
	if jobResult.RowsAffected > 0 && jobHasActiveHousekeepingReferencesFromJob(&job) {
		return true
	}
	if db.Migrator().HasTable(&models.SchedulerReservation{}) {
		var count int64
		if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ? AND state IN ?", jobID, []string{scheduler.ReservationStateLocked, scheduler.ReservationStateActive}).Count(&count).Error; err != nil {
			return true
		}
		if count > 0 {
			return true
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("status IN ?", []string{"queued", "running"}).Find(&operations).Error; err != nil {
			return true
		}
		for _, operation := range operations {
			for _, path := range []string{operation.AssetPath, operation.TemporaryPath, operation.BackupPath} {
				if housekeepingPathWithin(workspace, path) {
					return true
				}
			}
		}
	}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		if err := db.Where("queue_job_id = ?", jobID).Find(&publications).Error; err != nil {
			return true
		}
		now := time.Now().UTC()
		for _, publication := range publications {
			if publication.ActiveAttemptID != "" || (publication.LeaseExpiresAt != nil && publication.LeaseExpiresAt.After(now)) || (publication.OriginalArchiveLeaseExpiresAt != nil && publication.OriginalArchiveLeaseExpiresAt.After(now)) {
				return true
			}
		}
	}
	return false
}

func jobHasActiveHousekeepingReferencesFromJob(job *models.QueueJob) bool {
	if job == nil {
		return false
	}
	if isProtectedHousekeepingJob(job) || (job.StartedAt != nil && job.FinishedAt == nil) {
		return true
	}
	return false
}

func isProtectedHousekeepingJob(job *models.QueueJob) bool {
	if job == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(job.Status))
	stage := strings.ToLower(strings.TrimSpace(job.Stage))
	if status == JobStatusQueued || status == JobStatusRunning || status == "validating" || status == "validation_review" || status == "ready" || status == "publishing" || status == "review" || status == "blocked" {
		return true
	}
	if strings.Contains(status, "review") || strings.Contains(status, "validat") || strings.Contains(status, "publish") || strings.Contains(status, "block") || strings.Contains(status, "ready") {
		return true
	}
	if strings.Contains(stage, "review") || strings.Contains(stage, "validat") || strings.Contains(stage, "publish") || strings.Contains(stage, "ready") || strings.Contains(stage, "block") {
		return true
	}
	for _, value := range []string{job.ValidationStatus, job.ValidationOutcome} {
		value = strings.ToLower(strings.TrimSpace(value))
		if strings.Contains(value, "review") || strings.Contains(value, "block") || strings.Contains(value, "pending") || strings.Contains(value, "required") {
			return true
		}
	}
	if job.ValidationActionable && !job.ValidationOverride {
		return true
	}
	return job.Status == JobStatusCompleted && job.PublishedAt == nil
}

func operationIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "running", "active", "in_progress", "in-progress", "processing", "publishing", "archiving":
		return true
	default:
		return false
	}
}

func operationIsAbandoned(status, phase string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	phase = strings.ToLower(strings.TrimSpace(phase))
	if operationIsActive(status) {
		return false
	}
	return status == "failed" || status == "canceled" || status == "cancelled" || status == "interrupted" || status == "abandoned" || strings.Contains(phase, "interrupted") || strings.Contains(phase, "canceled") || strings.Contains(phase, "cancelled") || strings.Contains(phase, "failed")
}

func operationArtifactReferenceBlock(db *gorm.DB, operationID, path string) string {
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("id <> ? AND status IN ?", operationID, []string{"queued", "running"}).Find(&operations).Error; err != nil {
			return "active maintenance operation references could not be verified"
		}
		for _, operation := range operations {
			if sameHousekeepingPath(operation.TemporaryPath, path) || sameHousekeepingPath(operation.BackupPath, path) || sameHousekeepingPath(operation.AssetPath, path) {
				return "active maintenance operation references this artifact"
			}
		}
	}
	return protectedFileReferenceBlock(db, path, operationID, "")
}

func publisherArtifactReferenceBlock(db *gorm.DB, publicationID, path string) string {
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		if err := db.Where("publication_id <> ?", publicationID).Find(&publications).Error; err != nil {
			return "Publisher staging references could not be verified"
		}
		for _, publication := range publications {
			if sameHousekeepingPath(publication.OriginalArchiveTemporaryPath, path) {
				return "another Publisher operation references this staging artifact"
			}
		}
		var owner models.PublisherPublication
		result := db.Select("manifest").Where("publication_id = ?", publicationID).Limit(1).Find(&owner)
		if result.Error != nil || result.RowsAffected == 0 {
			return "Publisher manifest references could not be verified"
		}
		if housekeepingJSONContainsPath(owner.Manifest, path) {
			return "Publisher manifest references this staging artifact"
		}
	}
	return protectedFileReferenceBlock(db, path, "", publicationID)
}

func protectedFileReferenceBlock(db *gorm.DB, path, exceptOperationID, exceptPublicationID string) string {
	if db.Migrator().HasTable(&models.AssetRecord{}) {
		var assets []models.AssetRecord
		if err := db.Select("path, source_path").Find(&assets).Error; err != nil {
			return "AssetRecord references could not be verified"
		}
		for _, asset := range assets {
			if sameHousekeepingPath(asset.Path, path) || sameHousekeepingPath(asset.SourcePath, path) {
				return "current or historical AssetRecord references this artifact"
			}
		}
	}
	if db.Migrator().HasTable(&models.QueueJob{}) {
		var jobs []models.QueueJob
		if err := db.Select("media_path, output_path, published_path, planned_published_path, replacement_target_path, original_archived_path, subtitle_artifacts").Find(&jobs).Error; err != nil {
			return "QueueJob references could not be verified"
		}
		for _, job := range jobs {
			for _, reference := range []string{job.MediaPath, job.OutputPath, job.PublishedPath, job.PlannedPublishedPath, job.ReplacementTargetPath, job.OriginalArchivedPath} {
				if sameHousekeepingPath(reference, path) {
					return "QueueJob provenance references this artifact"
				}
			}
			if housekeepingJSONContainsPath(job.SubtitleArtifacts, path) {
				return "QueueJob required sidecar ownership references this artifact"
			}
		}
	}
	if db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var count int64
		if err := db.Model(&models.TestEncodeArtifact{}).Where("path = ?", path).Count(&count).Error; err != nil {
			return "Test Encode artifact ownership could not be verified"
		}
		if count > 0 {
			return "Test Encode artifact ownership references this path"
		}
	}
	if db.Migrator().HasTable(&models.TestEncode{}) {
		var tests []models.TestEncode
		if err := db.Select("output_path, temporary_path, subtitle_artifacts").Find(&tests).Error; err != nil {
			return "Test Encode lifecycle references could not be verified"
		}
		for _, test := range tests {
			if sameHousekeepingPath(test.OutputPath, path) || sameHousekeepingPath(test.TemporaryPath, path) {
				return "Test Encode lifecycle references this artifact"
			}
			for _, raw := range test.SubtitleArtifacts {
				if artifactPath, ok := raw.(string); ok && sameHousekeepingPath(artifactPath, path) {
					return "Test Encode subtitle ownership references this artifact"
				}
			}
		}
	}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		query := db.Select("publication_id, destination_path, source_path, original_archived_path, original_archive_source_path, original_archive_temporary_path")
		if exceptPublicationID != "" {
			query = query.Where("publication_id <> ?", exceptPublicationID)
		}
		if err := query.Find(&publications).Error; err != nil {
			return "Publisher lifecycle references could not be verified"
		}
		for _, publication := range publications {
			for _, reference := range []string{publication.DestinationPath, publication.SourcePath, publication.OriginalArchivedPath, publication.OriginalArchiveSourcePath, publication.OriginalArchiveTemporaryPath} {
				if sameHousekeepingPath(reference, path) {
					return "Publisher lifecycle or publication history references this artifact"
				}
			}
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		query := db.Select("id, temporary_path, backup_path, asset_path")
		if exceptOperationID != "" {
			query = query.Where("id <> ?", exceptOperationID)
		}
		if err := query.Find(&operations).Error; err != nil {
			return "persisted operation references could not be verified"
		}
		for _, operation := range operations {
			for _, reference := range []string{operation.TemporaryPath, operation.BackupPath, operation.AssetPath} {
				if sameHousekeepingPath(reference, path) {
					return "another persisted operation references this artifact"
				}
			}
		}
	}
	_ = exceptOperationID
	return ""
}

func sameHousekeepingPath(left, right string) bool {
	left = canonicalHousekeepingPath(left)
	right = canonicalHousekeepingPath(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}

func housekeepingPathWithin(root, path string) bool {
	root = canonicalHousekeepingPath(root)
	path = canonicalHousekeepingPath(path)
	return root != "" && path != "" && pathWithin(root, path)
}

func housekeepingJSONContainsPath(value any, path string) bool {
	found := false
	walkHousekeepingJSONStrings(value, func(candidate string) {
		if sameHousekeepingPath(candidate, path) {
			found = true
		}
	})
	return found
}

func walkHousekeepingJSONStrings(value any, visit func(string)) {
	if value == nil || visit == nil {
		return
	}
	item := reflect.ValueOf(value)
	if !item.IsValid() {
		return
	}
	for item.Kind() == reflect.Interface || item.Kind() == reflect.Pointer {
		if item.IsNil() {
			return
		}
		item = item.Elem()
	}
	switch item.Kind() {
	case reflect.String:
		visit(item.String())
	case reflect.Slice, reflect.Array:
		for index := 0; index < item.Len(); index++ {
			walkHousekeepingJSONStrings(item.Index(index).Interface(), visit)
		}
	case reflect.Map:
		iterator := item.MapRange()
		for iterator.Next() {
			walkHousekeepingJSONStrings(iterator.Value().Interface(), visit)
		}
	}
}

func canonicalHousekeepingPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	absolute = filepath.Clean(absolute)
	current := absolute
	missing := []string{}
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return absolute
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func maintenanceArtifactStillOwned(db *gorm.DB, roots map[string]string, candidate HousekeepingCandidate, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, string, error) {
	var operation models.AssetMaintenanceOperation
	result := db.Where("id = ?", candidate.OperationID).Limit(1).Find(&operation)
	if result.Error != nil {
		return candidate, "", result.Error
	}
	if result.RowsAffected == 0 {
		candidate.SafeToDelete = false
		candidate.Outcome = "blocked"
		candidate.BlockedReason = "maintenance operation record disappeared after preview"
		return candidate, "", nil
	}
	current, identity := maintenanceArtifactCandidate(db, roots, operation, policy, now)
	current.CandidateID = housekeepingCandidateID(current, identity)
	return current, identity, nil
}

func publisherArtifactStillOwned(db *gorm.DB, candidate HousekeepingCandidate, now time.Time) (bool, error) {
	var publication models.PublisherPublication
	result := db.Where("publication_id = ?", candidate.OperationID).Limit(1).Find(&publication)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	if !sameHousekeepingPath(publication.OriginalArchiveTemporaryPath, candidate.Path) || !publisherArchiveStageIsAbandoned(publication) || (publication.OriginalArchiveLeaseExpiresAt != nil && publication.OriginalArchiveLeaseExpiresAt.After(now)) {
		return false, nil
	}
	return publisherArtifactReferenceBlock(db, publication.PublicationID, candidate.Path) == "", nil
}
