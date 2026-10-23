package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const subtitleSyncRecoveryVersion = 1

var errSubtitleSyncRecoveryRequired = errors.New("subtitle synchronization recovery is required")

type subtitleSyncRecoveryRecord struct {
	Version         int       `json:"version"`
	State           string    `json:"state"`
	OperationID     string    `json:"operationId"`
	AssetPath       string    `json:"assetPath"`
	SubtitlePath    string    `json:"subtitlePath"`
	BackupPath      string    `json:"backupPath"`
	SourceIdentity  string    `json:"sourceIdentity"`
	SourceRevision  string    `json:"sourceRevision"`
	OriginalSHA256  string    `json:"originalSha256"`
	PublishedSHA256 string    `json:"publishedSha256"`
	BackupSHA256    string    `json:"backupSha256"`
	OriginalMode    uint32    `json:"originalMode"`
	ProvenanceState string    `json:"provenanceState"`
	Reason          string    `json:"reason,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type subtitleSyncRecoveryStatus struct {
	State                 string                      `json:"state"`
	MarkerPath            string                      `json:"markerPath"`
	Record                *subtitleSyncRecoveryRecord `json:"record,omitempty"`
	CurrentTargetSHA256   string                      `json:"currentTargetSha256,omitempty"`
	TargetMatchesRecorded bool                        `json:"targetMatchesRecorded"`
	BackupVerified        bool                        `json:"backupVerified"`
	VerificationError     string                      `json:"verificationError,omitempty"`
}

type subtitleSyncRecoveryError struct {
	status *subtitleSyncRecoveryStatus
	cause  error
}

func (e *subtitleSyncRecoveryError) Error() string {
	if e == nil {
		return errSubtitleSyncRecoveryRequired.Error()
	}
	message := "subtitle synchronization recovery is required"
	if e.status != nil && e.status.MarkerPath != "" {
		message += "; inspect persisted recovery state at " + e.status.MarkerPath
	}
	if e.cause != nil {
		message += ": " + e.cause.Error()
	}
	return message
}

func (e *subtitleSyncRecoveryError) Unwrap() error { return e.cause }

func (e *subtitleSyncRecoveryError) Is(target error) bool {
	return target == errSubtitleSyncRecoveryRequired
}

func subtitleSyncRecoveryMarkerPath(target string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(target)))
	if err != nil || absolute == "." || strings.TrimSpace(target) == "" {
		return "", errors.New("subtitle recovery target path is invalid")
	}
	sum := sha256.Sum256([]byte(absolute))
	name := fmt.Sprintf(".mvforge-subtitle-sync-recovery-%x.json", sum[:16])
	return filepath.Join(filepath.Dir(absolute), name), nil
}

func inspectSubtitleSyncRecovery(target string) (*subtitleSyncRecoveryStatus, error) {
	markerPath, err := subtitleSyncRecoveryMarkerPath(target)
	if err != nil {
		return nil, err
	}
	status := &subtitleSyncRecoveryStatus{State: "unreadable", MarkerPath: markerPath}
	info, err := os.Stat(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		status.VerificationError = "recovery marker cannot be inspected"
		return status, err
	}
	if info.IsDir() || info.Size() <= 0 || info.Size() > 64*1024 {
		status.VerificationError = "recovery marker has an invalid size or type"
		return status, errors.New(status.VerificationError)
	}
	content, err := os.ReadFile(markerPath)
	if err != nil {
		status.VerificationError = "recovery marker cannot be read"
		return status, err
	}
	var record subtitleSyncRecoveryRecord
	if err := json.Unmarshal(content, &record); err != nil {
		status.VerificationError = "recovery marker is not valid JSON"
		return status, err
	}
	status.Record = &record
	status.State = record.State
	absoluteTarget, targetErr := filepath.Abs(filepath.Clean(target))
	absoluteRecordTarget, recordTargetErr := filepath.Abs(filepath.Clean(record.SubtitlePath))
	absRecordBackup, backupPathErr := filepath.Abs(filepath.Clean(record.BackupPath))
	if targetErr != nil || recordTargetErr != nil || backupPathErr != nil ||
		record.Version != subtitleSyncRecoveryVersion || record.OperationID == "" ||
		absoluteRecordTarget != absoluteTarget || filepath.Dir(filepath.Dir(absRecordBackup)) != filepath.Dir(absoluteTarget) ||
		filepath.Base(absRecordBackup) != "original-subtitle" ||
		!strings.HasPrefix(filepath.Base(filepath.Dir(absRecordBackup)), ".mvforge-subtitle-sync-recovery-backup-") ||
		record.OriginalSHA256 == "" || record.PublishedSHA256 == "" || record.BackupSHA256 == "" {
		status.State = "unreadable"
		status.VerificationError = "recovery marker does not identify this target and a bounded backup"
		return status, errors.New(status.VerificationError)
	}
	currentDigest, currentErr := subtitleSyncSHA256File(absoluteTarget)
	if currentErr == nil {
		status.CurrentTargetSHA256 = currentDigest
		status.TargetMatchesRecorded = currentDigest == record.OriginalSHA256 || currentDigest == record.PublishedSHA256
	} else {
		status.VerificationError = "current subtitle target cannot be fingerprinted"
	}
	backupDigest, backupErr := subtitleSyncSHA256File(absRecordBackup)
	status.BackupVerified = backupErr == nil && backupDigest == record.BackupSHA256 && backupDigest == record.OriginalSHA256
	if !status.BackupVerified {
		status.State = "backup_unverified"
		status.VerificationError = "recovery backup is missing or its SHA-256 does not match the marker"
		return status, errors.New(status.VerificationError)
	}
	if currentErr != nil || !status.TargetMatchesRecorded {
		status.State = "target_unverified"
		if status.VerificationError == "" {
			status.VerificationError = "current subtitle digest matches neither the original nor the staged proposal"
		}
		return status, errors.New(status.VerificationError)
	}
	return status, nil
}

func requireNoSubtitleSyncRecovery(target string) error {
	status, err := inspectSubtitleSyncRecovery(target)
	if status == nil && err == nil {
		return nil
	}
	if err == nil {
		err = errSubtitleSyncRecoveryRequired
	}
	return &subtitleSyncRecoveryError{status: status, cause: err}
}

func createSubtitleSyncRecoveryIntent(target, assetPath, operationID, sourceIdentity, sourceRevision, stagedPath string) (*subtitleSyncRecoveryRecord, string, error) {
	absoluteTarget, err := filepath.Abs(filepath.Clean(target))
	if err != nil || absoluteTarget == "." {
		return nil, "", errors.New("subtitle recovery target path is invalid")
	}
	markerPath, err := subtitleSyncRecoveryMarkerPath(absoluteTarget)
	if err != nil {
		return nil, "", err
	}
	if status, inspectErr := inspectSubtitleSyncRecovery(absoluteTarget); status != nil || inspectErr != nil {
		return nil, markerPath, &subtitleSyncRecoveryError{status: status, cause: inspectErr}
	}
	backupDir, err := os.MkdirTemp(filepath.Dir(absoluteTarget), ".mvforge-subtitle-sync-recovery-backup-")
	if err != nil {
		return nil, markerPath, fmt.Errorf("could not create private subtitle recovery backup: %w", err)
	}
	backupPath := filepath.Join(backupDir, "original-subtitle")
	cleanupBackup := true
	defer func() {
		if cleanupBackup {
			_ = os.RemoveAll(backupDir)
		}
	}()

	sourceInfo, err := os.Stat(absoluteTarget)
	if err != nil || sourceInfo.IsDir() || sourceInfo.Size() > maxPublishedSubtitleBytes {
		return nil, markerPath, staleTranslationSourceError("selected subtitle is no longer available")
	}
	if err := copySubtitleSyncBackup(absoluteTarget, backupPath); err != nil {
		return nil, markerPath, fmt.Errorf("could not create verified subtitle recovery backup: %w", err)
	}
	originalDigest, err := subtitleSyncSHA256File(backupPath)
	if err != nil {
		return nil, markerPath, fmt.Errorf("could not fingerprint subtitle recovery backup: %w", err)
	}
	publishedDigest, err := subtitleSyncSHA256File(stagedPath)
	if err != nil {
		return nil, markerPath, fmt.Errorf("could not fingerprint staged subtitle: %w", err)
	}
	currentDigest, err := subtitleSyncSHA256File(absoluteTarget)
	if err != nil || currentDigest != originalDigest {
		return nil, markerPath, staleTranslationSourceError("selected subtitle changed while its recovery backup was being staged")
	}
	now := time.Now().UTC()
	record := &subtitleSyncRecoveryRecord{
		Version: subtitleSyncRecoveryVersion, State: "publishing", OperationID: operationID,
		AssetPath: filepath.Clean(assetPath), SubtitlePath: absoluteTarget, BackupPath: backupPath,
		SourceIdentity: sourceIdentity, SourceRevision: sourceRevision,
		OriginalSHA256: originalDigest, PublishedSHA256: publishedDigest, BackupSHA256: originalDigest,
		OriginalMode: uint32(sourceInfo.Mode().Perm()), ProvenanceState: "not_persisted",
		CreatedAt: now, UpdatedAt: now,
	}
	markerCreated, err := createSubtitleSyncRecoveryMarker(markerPath, *record)
	if err != nil {
		if markerCreated {
			cleanupBackup = false
			return record, markerPath, markSubtitleSyncRecoveryRequired(record, markerPath, "not_persisted", fmt.Errorf("could not durably record subtitle recovery intent; publication aborted: %w", err))
		}
		return nil, markerPath, fmt.Errorf("could not durably record subtitle recovery intent; publication aborted: %w", err)
	}
	cleanupBackup = false
	return record, markerPath, nil
}

func copySubtitleSyncBackup(sourcePath, backupPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	backup, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(backup, source); err != nil {
		_ = backup.Close()
		return err
	}
	if err := backup.Sync(); err != nil {
		_ = backup.Close()
		return err
	}
	if err := backup.Close(); err != nil {
		return err
	}
	return syncSubtitleSyncDirectory(filepath.Dir(backupPath))
}

func createSubtitleSyncRecoveryMarker(markerPath string, record subtitleSyncRecoveryRecord) (bool, error) {
	content, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(filepath.Dir(markerPath), ".mvforge-subtitle-sync-recovery-marker-*")
	if err != nil {
		return false, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return false, err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := os.Link(tempPath, markerPath); err != nil {
		return false, err
	}
	if err := syncSubtitleSyncDirectory(filepath.Dir(markerPath)); err != nil {
		return true, err
	}
	return true, nil
}

func updateSubtitleSyncRecoveryMarker(markerPath string, record subtitleSyncRecoveryRecord) error {
	content, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(markerPath), ".mvforge-subtitle-sync-recovery-update-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, markerPath); err != nil {
		return err
	}
	return syncSubtitleSyncDirectory(filepath.Dir(markerPath))
}

func markSubtitleSyncRecoveryRequired(record *subtitleSyncRecoveryRecord, markerPath, provenanceState string, cause error) *subtitleSyncRecoveryError {
	status := &subtitleSyncRecoveryStatus{State: "unreadable", MarkerPath: markerPath, Record: record}
	if record != nil {
		record.State = "recovery_required"
		record.ProvenanceState = provenanceState
		record.UpdatedAt = time.Now().UTC()
		if cause != nil {
			record.Reason = cause.Error()
		}
		if updateErr := updateSubtitleSyncRecoveryMarker(markerPath, *record); updateErr != nil {
			if cause == nil {
				cause = updateErr
			} else {
				cause = fmt.Errorf("%v; recovery marker update failed: %w", cause, updateErr)
			}
			if persisted, _ := inspectSubtitleSyncRecovery(record.SubtitlePath); persisted != nil {
				status = persisted
			}
		} else {
			status.State = record.State
		}
	}
	if cause == nil {
		cause = errSubtitleSyncRecoveryRequired
	}
	if record != nil {
		if digest, err := subtitleSyncSHA256File(record.SubtitlePath); err == nil {
			status.CurrentTargetSHA256 = digest
			status.TargetMatchesRecorded = digest == record.OriginalSHA256 || digest == record.PublishedSHA256
		}
		if digest, err := subtitleSyncSHA256File(record.BackupPath); err == nil && digest == record.BackupSHA256 {
			status.BackupVerified = true
		}
	}
	return &subtitleSyncRecoveryError{status: status, cause: cause}
}

func restoreSubtitleSyncBackup(record subtitleSyncRecoveryRecord, target string) error {
	backupDigest, err := subtitleSyncSHA256File(record.BackupPath)
	if err != nil || backupDigest != record.BackupSHA256 || backupDigest != record.OriginalSHA256 {
		return errors.New("subtitle recovery backup failed its SHA-256 check")
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".mvforge-subtitle-sync-restore-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	backup, err := os.Open(record.BackupPath)
	if err != nil {
		_ = temp.Close()
		return err
	}
	_, copyErr := io.Copy(temp, backup)
	closeBackupErr := backup.Close()
	if copyErr != nil {
		_ = temp.Close()
		return copyErr
	}
	if closeBackupErr != nil {
		_ = temp.Close()
		return closeBackupErr
	}
	if err := temp.Chmod(os.FileMode(record.OriginalMode)); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	if err := syncSubtitleSyncDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	currentDigest, err := subtitleSyncSHA256File(target)
	if err != nil || currentDigest != record.OriginalSHA256 {
		return errors.New("restored subtitle did not match its verified recovery digest")
	}
	return nil
}

func clearSubtitleSyncRecovery(record subtitleSyncRecoveryRecord, markerPath string) error {
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := syncSubtitleSyncDirectory(filepath.Dir(markerPath)); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Dir(record.BackupPath))
	return nil
}

func subtitleSyncSHA256File(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Size() > maxPublishedSubtitleBytes {
		return "", errors.New("subtitle recovery file exceeds its size bound")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncSubtitleSyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
