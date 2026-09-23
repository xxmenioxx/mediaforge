package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	compatiblePreviewRequestStateActive           = "active"
	compatiblePreviewRequestStateStale            = "stale"
	compatiblePreviewRequestStateExpiredTombstone = "expired_tombstone"
	compatiblePreviewTombstoneRetention           = 2 * time.Hour
	compatiblePreviewTombstoneMaxItems            = 500
	compatiblePreviewPreparedSnapshotVersion      = 1
	compatiblePreviewPreparedSnapshotMaxBytes     = 512 << 10
	compatiblePreviewNormalizedRequestMaxBytes    = 64 << 10
	compatiblePreviewDurableTouchInterval         = 30 * time.Second
	compatiblePreviewRecoveryReasonMaxBytes       = 1024
)

type compatiblePreviewPreparedSnapshot struct {
	Version                  int                              `json:"version"`
	Path                     string                           `json:"path"`
	SourceRevision           compatiblePreviewSourceRevision  `json:"sourceRevision"`
	ArtifactID               string                           `json:"artifactId"`
	CanonicalEncodeSignature string                           `json:"canonicalEncodeSignature"`
	CanonicalFinalCachePath  string                           `json:"canonicalFinalCachePath"`
	Start                    string                           `json:"start"`
	Seconds                  int                              `json:"seconds"`
	PreviewMode              string                           `json:"previewMode"`
	Normalization            previewNormalizationDecision     `json:"normalization"`
	RequestedProfile         *models.Profile                  `json:"requestedProfile,omitempty"`
	EffectiveProfile         *models.Profile                  `json:"effectiveProfile,omitempty"`
	ResolvedVideoDecision    models.JSONMap                   `json:"resolvedVideoDecision,omitempty"`
	RequestedVideoEncoder    string                           `json:"requestedVideoEncoder"`
	EffectiveVideoEncoder    string                           `json:"effectiveVideoEncoder"`
	RequestedQSVRateControl  string                           `json:"requestedQsvRateControl"`
	EffectiveQSVRateControl  string                           `json:"effectiveQsvRateControl"`
	CanonicalFFmpegArgs      []string                         `json:"canonicalFfmpegArgs"`
	RetentionPolicy          compatiblePreviewRetentionPolicy `json:"retentionPolicy"`
}

type compatiblePreviewRequestTombstone struct {
	RequestID             string
	NormalizedRequestJSON string
	ExpiresAt             time.Time
	TombstoneExpiresAt    time.Time
	Reason                string
}

type compatiblePreviewLifecycleError struct {
	Status    int
	Code      string
	State     string
	Reason    string
	Retryable bool
	Recreate  bool
}

func (err *compatiblePreviewLifecycleError) Error() string {
	if err == nil {
		return "compatible preview request is unavailable"
	}
	if err.Reason != "" {
		return err.Reason
	}
	return "compatible preview request is " + err.Code
}

func compatiblePreviewLifecycleFailure(state, reason string) error {
	status := 409
	code := state
	responseState := state
	retryable := false
	recreate := false
	switch state {
	case compatiblePreviewRequestStateExpiredTombstone:
		status = 410
		code = "expired"
		responseState = "expired"
		retryable = true
		recreate = true
	case string(compatiblePreviewArtifactStale):
		code = "stale_artifact"
		retryable = true
		recreate = true
	case string(compatiblePreviewArtifactInterrupted):
		retryable = true
		recreate = true
	case string(compatiblePreviewArtifactFailed), string(compatiblePreviewArtifactCanceled):
		recreate = true
	}
	return &compatiblePreviewLifecycleError{Status: status, Code: code, State: responseState, Reason: boundCompatiblePreviewRecoveryReason(reason), Retryable: retryable, Recreate: recreate}
}

func boundCompatiblePreviewRecoveryReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) <= compatiblePreviewRecoveryReasonMaxBytes {
		return reason
	}
	return reason[:compatiblePreviewRecoveryReasonMaxBytes] + " [truncated]"
}

func compatiblePreviewPreparedSnapshotForDescriptor(descriptor *compatiblePreviewDescriptor) (string, int, error) {
	if descriptor == nil {
		return "", 0, errors.New("compatible preview descriptor is required")
	}
	snapshot := compatiblePreviewPreparedSnapshot{
		Version:                  compatiblePreviewPreparedSnapshotVersion,
		Path:                     descriptor.Path,
		SourceRevision:           descriptor.SourceRevision,
		ArtifactID:               descriptor.ArtifactID,
		CanonicalEncodeSignature: descriptor.CanonicalEncodeSignature,
		CanonicalFinalCachePath:  descriptor.CachePath,
		Start:                    descriptor.Start,
		Seconds:                  descriptor.Seconds,
		PreviewMode:              descriptor.PreviewMode,
		Normalization:            descriptor.Normalization,
		RequestedProfile:         cloneCompatiblePreviewProfile(descriptor.PreviewProfile),
		EffectiveProfile:         cloneCompatiblePreviewProfile(descriptor.EffectivePreviewProfile),
		ResolvedVideoDecision:    descriptor.ResolvedVideoDecision,
		RequestedVideoEncoder:    descriptor.RequestedVideoEncoder,
		EffectiveVideoEncoder:    descriptor.EffectiveVideoEncoder,
		RequestedQSVRateControl:  descriptor.RequestedQSVRateControl,
		EffectiveQSVRateControl:  descriptor.EffectiveQSVRateControl,
		CanonicalFFmpegArgs:      append([]string(nil), descriptor.Args...),
		RetentionPolicy:          descriptor.RetentionPolicy,
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return "", 0, fmt.Errorf("encode compatible preview prepared snapshot: %w", err)
	}
	if len(payload) > compatiblePreviewPreparedSnapshotMaxBytes {
		return "", 0, fmt.Errorf("compatible preview prepared snapshot exceeds %d bytes", compatiblePreviewPreparedSnapshotMaxBytes)
	}
	return string(payload), snapshot.Version, nil
}

func compatiblePreviewDescriptorFromSnapshot(request compatiblePreviewRequest, payload string, db *gorm.DB, requestID string) (*compatiblePreviewDescriptor, error) {
	if len(payload) == 0 || len(payload) > compatiblePreviewPreparedSnapshotMaxBytes {
		return nil, fmt.Errorf("compatible preview prepared snapshot is missing or oversized")
	}
	var snapshot compatiblePreviewPreparedSnapshot
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		return nil, fmt.Errorf("decode compatible preview prepared snapshot: %w", err)
	}
	if snapshot.Version != compatiblePreviewPreparedSnapshotVersion {
		return nil, fmt.Errorf("unsupported compatible preview prepared snapshot version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.Path) == "" || strings.TrimSpace(snapshot.ArtifactID) == "" || strings.TrimSpace(snapshot.CanonicalEncodeSignature) == "" || len(snapshot.CanonicalFFmpegArgs) == 0 {
		return nil, fmt.Errorf("compatible preview prepared snapshot is incomplete")
	}
	if snapshot.RetentionPolicy != compatiblePreviewPersistentRetention && snapshot.RetentionPolicy != compatiblePreviewEphemeralRetention {
		return nil, fmt.Errorf("compatible preview prepared snapshot has invalid retention policy")
	}
	descriptor := &compatiblePreviewDescriptor{
		Request:                  request,
		Path:                     snapshot.Path,
		SourceRevision:           snapshot.SourceRevision,
		Start:                    snapshot.Start,
		Seconds:                  snapshot.Seconds,
		PreviewMode:              snapshot.PreviewMode,
		Normalization:            snapshot.Normalization,
		Args:                     append([]string(nil), snapshot.CanonicalFFmpegArgs...),
		PreviewProfile:           cloneCompatiblePreviewProfile(snapshot.RequestedProfile),
		EffectivePreviewProfile:  cloneCompatiblePreviewProfile(snapshot.EffectiveProfile),
		ResolvedVideoDecision:    snapshot.ResolvedVideoDecision,
		RequestedVideoEncoder:    snapshot.RequestedVideoEncoder,
		EffectiveVideoEncoder:    snapshot.EffectiveVideoEncoder,
		RequestedQSVRateControl:  snapshot.RequestedQSVRateControl,
		EffectiveQSVRateControl:  snapshot.EffectiveQSVRateControl,
		CanonicalEncodeSignature: snapshot.CanonicalEncodeSignature,
		ArtifactID:               snapshot.ArtifactID,
		RetentionPolicy:          snapshot.RetentionPolicy,
		CachePath:                snapshot.CanonicalFinalCachePath,
		persistenceDB:            db,
		requestID:                requestID,
		frozen:                   true,
	}
	if !compatiblePreviewCanonicalArtifactPath(descriptor.ArtifactID, descriptor.CachePath) {
		return nil, fmt.Errorf("compatible preview prepared snapshot has an unsafe cache path")
	}
	return descriptor, nil
}

func compatiblePreviewCacheRoot() string {
	if configured := strings.TrimSpace(os.Getenv("MVFORGE_COMPATIBLE_PREVIEW_CACHE_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	return filepath.Join(os.TempDir(), "mvforge-preview-cache")
}

func compatiblePreviewCanonicalArtifactPath(artifactID, candidate string) bool {
	if strings.TrimSpace(artifactID) == "" || strings.TrimSpace(candidate) == "" {
		return false
	}
	root, err := filepath.Abs(compatiblePreviewCacheRoot())
	if err != nil {
		return false
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}
	expected := filepath.Join(root, artifactID+".mp4")
	expected, _ = filepath.Abs(expected)
	return path == expected && filepath.Base(path) == artifactID+".mp4"
}

func compatiblePreviewPublishedArtifactValid(artifactID, path string) bool {
	if !compatiblePreviewCanonicalArtifactPath(artifactID, path) {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func compatiblePreviewPublishedArtifactValidForDescriptor(descriptor *compatiblePreviewDescriptor) bool {
	if descriptor == nil {
		return false
	}
	if descriptor.persistenceDB != nil && descriptor.RetentionPolicy == compatiblePreviewPersistentRetention {
		return compatiblePreviewPublishedArtifactValid(descriptor.ArtifactID, descriptor.CachePath)
	}
	info, err := os.Lstat(descriptor.CachePath)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func newCompatiblePreviewGenerationToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

func compatiblePreviewPersistedArtifactFields(descriptor *compatiblePreviewDescriptor, now time.Time) map[string]any {
	return map[string]any{
		"canonical_encode_signature": descriptor.CanonicalEncodeSignature,
		"cache_path":                 descriptor.CachePath,
		"source_path":                descriptor.Path,
		"source_size":                descriptor.SourceRevision.Size,
		"source_modified_at_ns":      descriptor.SourceRevision.ModifiedAtNS,
		"source_mode":                descriptor.SourceRevision.Mode,
		"retention_policy":           string(descriptor.RetentionPolicy),
		"updated_at":                 now,
		"last_used_at":               now,
		"expires_at":                 now.Add(24 * time.Hour),
	}
}

func persistCompatiblePreviewPreparedRequest(db *gorm.DB, requestID string, request compatiblePreviewRequest, descriptor *compatiblePreviewDescriptor, now time.Time) error {
	if db == nil || descriptor == nil || descriptor.RetentionPolicy == compatiblePreviewEphemeralRetention {
		return nil
	}
	if !compatiblePreviewCanonicalArtifactPath(descriptor.ArtifactID, descriptor.CachePath) {
		return fmt.Errorf("compatible preview descriptor has an unsafe cache path")
	}
	normalizedRequest, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode compatible preview request: %w", err)
	}
	if len(normalizedRequest) > compatiblePreviewNormalizedRequestMaxBytes {
		return fmt.Errorf("compatible preview request exceeds %d bytes", compatiblePreviewNormalizedRequestMaxBytes)
	}
	snapshot, version, err := compatiblePreviewPreparedSnapshotForDescriptor(descriptor)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		artifact := models.CompatiblePreviewArtifactRecord{
			ArtifactID: descriptor.ArtifactID,
			State:      string(compatiblePreviewArtifactAbsent),
		}
		queryErr := tx.Where("artifact_id = ?", descriptor.ArtifactID).First(&artifact).Error
		if queryErr != nil && !errors.Is(queryErr, gorm.ErrRecordNotFound) {
			return queryErr
		}
		if errors.Is(queryErr, gorm.ErrRecordNotFound) {
			artifact = models.CompatiblePreviewArtifactRecord{ArtifactID: descriptor.ArtifactID, State: string(compatiblePreviewArtifactAbsent), CreatedAt: now}
		}
		if artifact.CreatedAt.IsZero() {
			artifact.CreatedAt = now
		}
		for key, value := range compatiblePreviewPersistedArtifactFields(descriptor, now) {
			switch key {
			case "canonical_encode_signature":
				artifact.CanonicalEncodeSignature = value.(string)
			case "cache_path":
				artifact.CachePath = value.(string)
			case "source_path":
				artifact.SourcePath = value.(string)
			case "source_size":
				artifact.SourceSize = value.(int64)
			case "source_modified_at_ns":
				artifact.SourceModifiedAtNS = value.(int64)
			case "source_mode":
				artifact.SourceMode = value.(uint32)
			case "retention_policy":
				artifact.RetentionPolicy = value.(string)
			case "updated_at":
				artifact.UpdatedAt = value.(time.Time)
			case "last_used_at":
				artifact.LastUsedAt = value.(time.Time)
			case "expires_at":
				artifact.ExpiresAt = value.(time.Time)
			}
		}
		if err := tx.Save(&artifact).Error; err != nil {
			return err
		}
		requestRecord := models.CompatiblePreviewRequestRecord{
			RequestID: requestID, State: compatiblePreviewRequestStateActive,
			NormalizedRequestJSON: string(normalizedRequest), PreparedSnapshotVersion: version,
			PreparedSnapshotJSON: snapshot, ArtifactID: descriptor.ArtifactID,
			CreatedAt: now, UpdatedAt: now, AccessedAt: now,
			ExpiresAt: now.Add(compatiblePreviewRequestRetention), RecoveryReason: "",
		}
		var existing models.CompatiblePreviewRequestRecord
		if err := tx.Where("request_id = ?", requestID).First(&existing).Error; err == nil {
			if existing.State == compatiblePreviewRequestStateActive {
				requestRecord.CreatedAt = existing.CreatedAt
			}
		}
		return tx.Save(&requestRecord).Error
	})
}

func persistCompatiblePreviewRequestTouch(db *gorm.DB, requestID string, accessedAt, expiresAt time.Time) {
	if db == nil || strings.TrimSpace(requestID) == "" {
		return
	}
	_ = db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ? AND state = ?", requestID, compatiblePreviewRequestStateActive).Updates(map[string]any{"accessed_at": accessedAt, "expires_at": expiresAt, "updated_at": accessedAt}).Error
}

func persistCompatiblePreviewRequestTombstone(db *gorm.DB, tombstone compatiblePreviewRequestTombstone) {
	if db == nil || tombstone.RequestID == "" {
		return
	}
	_ = db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ?", tombstone.RequestID).Updates(map[string]any{
		"state":                     compatiblePreviewRequestStateExpiredTombstone,
		"normalized_request_json":   "",
		"prepared_snapshot_json":    "",
		"prepared_snapshot_version": 0,
		"artifact_id":               "",
		"expires_at":                tombstone.ExpiresAt,
		"tombstone_expires_at":      tombstone.TombstoneExpiresAt,
		"recovery_reason":           boundCompatiblePreviewRecoveryReason(tombstone.Reason),
		"updated_at":                time.Now(),
	}).Error
}

func persistCompatiblePreviewRequestStale(db *gorm.DB, requestID, reason string) {
	if db == nil || requestID == "" {
		return
	}
	_ = db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ? AND state = ?", requestID, compatiblePreviewRequestStateActive).Updates(map[string]any{
		"state": compatiblePreviewRequestStateStale, "recovery_reason": boundCompatiblePreviewRecoveryReason(reason), "updated_at": time.Now(),
	}).Error
}

func persistCompatiblePreviewArtifactGenerationStart(db *gorm.DB, descriptor *compatiblePreviewDescriptor, now time.Time) (string, int64, error) {
	token := newCompatiblePreviewGenerationToken()
	version := int64(1)
	if db == nil || descriptor == nil {
		return token, version, nil
	}
	var record models.CompatiblePreviewArtifactRecord
	if err := db.Where("artifact_id = ?", descriptor.ArtifactID).First(&record).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, err
		}
		record = models.CompatiblePreviewArtifactRecord{ArtifactID: descriptor.ArtifactID, CreatedAt: now}
	} else if record.GenerationVersion > 0 {
		version = record.GenerationVersion + 1
	}
	fields := compatiblePreviewPersistedArtifactFields(descriptor, now)
	fields["state"] = string(compatiblePreviewArtifactGenerating)
	fields["generation_token"] = token
	fields["generation_version"] = version
	fields["generation_started_at"] = now
	fields["generation_finished_at"] = time.Time{}
	fields["recovery_reason"] = ""
	if err := db.Transaction(func(tx *gorm.DB) error {
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		if record.State == "" {
			record.State = string(compatiblePreviewArtifactGenerating)
		}
		if err := tx.Where("artifact_id = ?", descriptor.ArtifactID).FirstOrCreate(&record).Error; err != nil {
			return err
		}
		return tx.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ?", descriptor.ArtifactID).Updates(fields).Error
	}); err != nil {
		return "", 0, err
	}
	return token, version, nil
}

func persistCompatiblePreviewArtifactTransition(db *gorm.DB, artifactID, token string, version int64, state compatiblePreviewArtifactState, reason string, now time.Time) (bool, error) {
	if db == nil || artifactID == "" || token == "" {
		return true, nil
	}
	updates := map[string]any{"state": string(state), "recovery_reason": boundCompatiblePreviewRecoveryReason(reason), "updated_at": now, "last_used_at": now, "generation_finished_at": now}
	result := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ? AND generation_token = ? AND generation_version = ?", artifactID, token, version).Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func persistCompatiblePreviewGenerationCompletion(entry *compatiblePreviewArtifact, artifactID, token string, version int64, state compatiblePreviewArtifactState, reason string, now time.Time) (bool, error) {
	var db *gorm.DB
	if entry != nil {
		entry.mu.Lock()
		db = entry.persistenceDB
		entry.mu.Unlock()
	}
	updated, err := persistCompatiblePreviewArtifactTransition(db, artifactID, token, version, state, reason, now)
	if err != nil || !updated {
		return updated, err
	}
	compatiblePreviewArtifacts.Lock()
	runtimeOwner := entry != nil && compatiblePreviewArtifacts.items[artifactID] == entry
	compatiblePreviewArtifacts.Unlock()
	if !runtimeOwner {
		return updated, nil
	}
	markCompatiblePreviewRequestsForArtifactRecovery(artifactID, state, reason)
	return true, nil
}

func markCompatiblePreviewRequestsForArtifactRecovery(artifactID string, state compatiblePreviewArtifactState, reason string) {
	if artifactID == "" || (state != compatiblePreviewArtifactFailed && state != compatiblePreviewArtifactCanceled) {
		return
	}
	compatiblePreviewRequests.Lock()
	entries := make([]*compatiblePreviewRequestEntry, 0, len(compatiblePreviewRequests.items))
	for _, entry := range compatiblePreviewRequests.items {
		entries = append(entries, entry)
	}
	compatiblePreviewRequests.Unlock()
	for _, entry := range entries {
		entry.mu.Lock()
		if entry.Descriptor != nil && entry.Descriptor.ArtifactID == artifactID && entry.recoveryState == "" {
			entry.recoveryState = string(state)
			entry.recoveryReason = boundCompatiblePreviewRecoveryReason(reason)
		}
		entry.mu.Unlock()
	}
}

func compatiblePreviewArtifactPathDurablyReferenced(db *gorm.DB, path string) bool {
	if db == nil || !compatiblePreviewCanonicalArtifactPath(artifactIDFromDestination(path), path) {
		return false
	}
	var count int64
	_ = db.Model(&models.CompatiblePreviewRequestRecord{}).Where("state IN ? AND artifact_id = ?", []string{compatiblePreviewRequestStateActive, compatiblePreviewRequestStateStale}, artifactIDFromDestination(path)).Count(&count).Error
	return count > 0
}

func cleanupCompatiblePreviewArtifactRecords(db *gorm.DB, now time.Time) error {
	if db == nil {
		return nil
	}
	referenced := db.Model(&models.CompatiblePreviewRequestRecord{}).Select("artifact_id").Where("state IN ? AND artifact_id <> ''", []string{compatiblePreviewRequestStateActive, compatiblePreviewRequestStateStale})
	return db.Where("retention_policy = ? AND state <> ? AND expires_at <= ? AND artifact_id NOT IN (?)", string(compatiblePreviewPersistentRetention), string(compatiblePreviewArtifactGenerating), now, referenced).Delete(&models.CompatiblePreviewArtifactRecord{}).Error
}

func markCompatiblePreviewArtifactStaleForPath(db *gorm.DB, path, reason string) {
	if db == nil {
		return
	}
	_ = db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ? AND cache_path = ? AND state = ?", artifactIDFromDestination(path), path, compatiblePreviewArtifactReady).Updates(map[string]any{
		"state": string(compatiblePreviewArtifactStale), "recovery_reason": boundCompatiblePreviewRecoveryReason(reason), "updated_at": time.Now(),
	}).Error
}

func compatiblePreviewArtifactRecordsByID(db *gorm.DB, requestRecords []models.CompatiblePreviewRequestRecord) (map[string]models.CompatiblePreviewArtifactRecord, error) {
	result := make(map[string]models.CompatiblePreviewArtifactRecord)
	if db == nil {
		return result, nil
	}
	artifactIDs := make([]string, 0, len(requestRecords))
	seen := make(map[string]struct{}, len(requestRecords))
	for _, request := range requestRecords {
		artifactID := strings.TrimSpace(request.ArtifactID)
		if artifactID == "" {
			continue
		}
		if _, exists := seen[artifactID]; exists {
			continue
		}
		seen[artifactID] = struct{}{}
		artifactIDs = append(artifactIDs, artifactID)
	}
	if len(artifactIDs) == 0 {
		return result, nil
	}
	var records []models.CompatiblePreviewArtifactRecord
	if err := db.Where("artifact_id IN ?", artifactIDs).Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		result[record.ArtifactID] = record
	}
	return result, nil
}

func reconcileCompatiblePreviewArtifactRecord(db *gorm.DB, record *models.CompatiblePreviewArtifactRecord, now time.Time) (models.CompatiblePreviewArtifactRecord, error) {
	if record == nil {
		return models.CompatiblePreviewArtifactRecord{}, errors.New("compatible preview artifact record is required")
	}
	validFinal := compatiblePreviewPublishedArtifactValid(record.ArtifactID, record.CachePath)
	switch record.State {
	case string(compatiblePreviewArtifactGenerating):
		if validFinal {
			result := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ? AND generation_token = ? AND generation_version = ?", record.ArtifactID, record.GenerationToken, record.GenerationVersion).Updates(map[string]any{"state": string(compatiblePreviewArtifactReady), "recovery_reason": "", "updated_at": now, "last_used_at": now, "generation_finished_at": now})
			if result.Error != nil {
				return *record, result.Error
			}
			record.State = string(compatiblePreviewArtifactReady)
			record.RecoveryReason = ""
		} else {
			result := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ? AND generation_token = ? AND generation_version = ?", record.ArtifactID, record.GenerationToken, record.GenerationVersion).Updates(map[string]any{"state": string(compatiblePreviewArtifactInterrupted), "recovery_reason": "backend restarted while generation was active", "updated_at": now, "generation_finished_at": now})
			if result.Error != nil {
				return *record, result.Error
			}
			record.State = string(compatiblePreviewArtifactInterrupted)
			record.RecoveryReason = "backend restarted while generation was active"
		}
	case string(compatiblePreviewArtifactReady):
		if !validFinal {
			result := db.Model(&models.CompatiblePreviewArtifactRecord{}).Where("artifact_id = ? AND generation_token = ? AND generation_version = ?", record.ArtifactID, record.GenerationToken, record.GenerationVersion).Updates(map[string]any{"state": string(compatiblePreviewArtifactStale), "recovery_reason": "published preview artifact is missing or invalid", "updated_at": now, "generation_finished_at": now})
			if result.Error != nil {
				return *record, result.Error
			}
			record.State = string(compatiblePreviewArtifactStale)
			record.RecoveryReason = "published preview artifact is missing or invalid"
		}
	}
	return *record, nil
}

func sortedCompatiblePreviewTombstones(records []models.CompatiblePreviewRequestRecord) []models.CompatiblePreviewRequestRecord {
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i].TombstoneExpiresAt, records[j].TombstoneExpiresAt
		if left == nil && right == nil {
			return records[i].UpdatedAt.Before(records[j].UpdatedAt)
		}
		if left == nil {
			return true
		}
		if right == nil {
			return false
		}
		return left.Before(*right)
	})
	return records
}

func compatiblePreviewTombstoneForRecord(record models.CompatiblePreviewRequestRecord, now time.Time, reason string) compatiblePreviewRequestTombstone {
	expiresAt := record.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now
	}
	return compatiblePreviewRequestTombstone{
		RequestID: record.RequestID,
		ExpiresAt: expiresAt, TombstoneExpiresAt: now.Add(compatiblePreviewTombstoneRetention), Reason: reason,
	}
}

func expireCompatiblePreviewRequestRecords(db *gorm.DB, now time.Time) error {
	var records []models.CompatiblePreviewRequestRecord
	if err := db.Where("state IN ? AND expires_at <= ?", []string{compatiblePreviewRequestStateActive, compatiblePreviewRequestStateStale}, now).Find(&records).Error; err != nil {
		return err
	}
	for _, record := range records {
		tombstone := compatiblePreviewTombstoneForRecord(record, now, "request expired")
		if err := db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ? AND state = ?", record.RequestID, record.State).Updates(map[string]any{
			"state": compatiblePreviewRequestStateExpiredTombstone, "normalized_request_json": "", "prepared_snapshot_version": 0, "prepared_snapshot_json": "", "artifact_id": "",
			"tombstone_expires_at": tombstone.TombstoneExpiresAt, "recovery_reason": tombstone.Reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
	}
	var active []models.CompatiblePreviewRequestRecord
	if err := db.Where("state IN ?", []string{compatiblePreviewRequestStateActive, compatiblePreviewRequestStateStale}).Order("accessed_at ASC").Find(&active).Error; err != nil {
		return err
	}
	if len(active) <= compatiblePreviewRequestMaxItems {
		return nil
	}
	for _, record := range active[:len(active)-compatiblePreviewRequestMaxItems] {
		tombstone := compatiblePreviewTombstoneForRecord(record, now, "request retention bound reached")
		if err := db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ? AND state = ?", record.RequestID, record.State).Updates(map[string]any{
			"state": compatiblePreviewRequestStateExpiredTombstone, "normalized_request_json": "", "prepared_snapshot_version": 0, "prepared_snapshot_json": "", "artifact_id": "",
			"tombstone_expires_at": tombstone.TombstoneExpiresAt, "recovery_reason": tombstone.Reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func purgeCompatiblePreviewTombstoneRecords(db *gorm.DB, now time.Time) error {
	var tombstones []models.CompatiblePreviewRequestRecord
	if err := db.Where("state = ?", compatiblePreviewRequestStateExpiredTombstone).Order("tombstone_expires_at ASC").Find(&tombstones).Error; err != nil {
		return err
	}
	for index, record := range tombstones {
		purge := record.TombstoneExpiresAt == nil || !record.TombstoneExpiresAt.After(now) || index < len(tombstones)-compatiblePreviewTombstoneMaxItems
		if purge {
			if err := db.Where("request_id = ? AND state = ?", record.RequestID, compatiblePreviewRequestStateExpiredTombstone).Delete(&models.CompatiblePreviewRequestRecord{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func resetCompatiblePreviewRuntimeForReconcile() {
	resetCompatiblePreviewLeases()
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items = map[string]*compatiblePreviewRequestEntry{}
	compatiblePreviewRequests.tombstones = map[string]compatiblePreviewRequestTombstone{}
	compatiblePreviewRequests.Unlock()
	compatiblePreviewArtifacts.Lock()
	for _, entry := range compatiblePreviewArtifacts.items {
		entry.mu.Lock()
		if entry.cancel != nil {
			entry.cancel()
		}
		entry.mu.Unlock()
	}
	compatiblePreviewArtifacts.items = map[string]*compatiblePreviewArtifact{}
	compatiblePreviewArtifacts.Unlock()
}

func restoreCompatiblePreviewRequestRecord(db *gorm.DB, record models.CompatiblePreviewRequestRecord, artifacts map[string]models.CompatiblePreviewArtifactRecord, now time.Time) error {
	var request compatiblePreviewRequest
	if len(record.NormalizedRequestJSON) == 0 || len(record.NormalizedRequestJSON) > compatiblePreviewNormalizedRequestMaxBytes || json.Unmarshal([]byte(record.NormalizedRequestJSON), &request) != nil {
		return db.Model(&models.CompatiblePreviewRequestRecord{}).Where("request_id = ?", record.RequestID).Updates(map[string]any{"state": compatiblePreviewRequestStateStale, "recovery_reason": "normalized request metadata is invalid", "updated_at": now}).Error
	}
	entry := &compatiblePreviewRequestEntry{
		Request: request, CreatedAt: record.CreatedAt, AccessedAt: record.AccessedAt, ExpiresAt: record.ExpiresAt,
		DurableAccessedAt: record.AccessedAt, persistenceDB: db,
	}
	if record.State == compatiblePreviewRequestStateStale {
		entry.recoveryState = string(compatiblePreviewArtifactStale)
		entry.recoveryReason = record.RecoveryReason
	} else {
		if record.PreparedSnapshotVersion != compatiblePreviewPreparedSnapshotVersion {
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = fmt.Sprintf("unsupported prepared preview snapshot version %d", record.PreparedSnapshotVersion)
			persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
			compatiblePreviewRequests.Lock()
			compatiblePreviewRequests.items[record.RequestID] = entry
			compatiblePreviewRequests.Unlock()
			return nil
		}
		descriptor, err := compatiblePreviewDescriptorFromSnapshot(request, record.PreparedSnapshotJSON, db, record.RequestID)
		if err != nil {
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = "prepared preview snapshot is invalid: " + err.Error()
			persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
		} else if allowed, authorizationErr := (AssetHandler{db: db}).pathBelongsToReadableMediaRoot(descriptor.Path); authorizationErr != nil {
			entry.Descriptor = descriptor
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = "resolved source path is no longer inside a configured media root"
			persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
		} else if !allowed {
			entry.Descriptor = descriptor
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = "resolved source path is no longer inside a configured media root"
			persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
		} else if record.ArtifactID != descriptor.ArtifactID {
			entry.Descriptor = descriptor
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = "request artifact relationship does not match its frozen snapshot"
			persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
		} else if !descriptor.SourceRevision.matches(descriptor.Path) {
			entry.Descriptor = descriptor
			entry.recoveryState = string(compatiblePreviewArtifactStale)
			entry.recoveryReason = "source revision changed after preview preparation"
			persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
		} else {
			entry.Descriptor = descriptor
			artifact, exists := artifacts[descriptor.ArtifactID]
			if !exists {
				entry.recoveryState = string(compatiblePreviewArtifactStale)
				entry.recoveryReason = "durable artifact relationship is missing"
				persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
			} else if artifact.CachePath != descriptor.CachePath || artifact.SourcePath != descriptor.Path || artifact.SourceSize != descriptor.SourceRevision.Size || artifact.SourceModifiedAtNS != descriptor.SourceRevision.ModifiedAtNS || artifact.SourceMode != descriptor.SourceRevision.Mode || artifact.CanonicalEncodeSignature != descriptor.CanonicalEncodeSignature || !compatiblePreviewCanonicalArtifactPath(artifact.ArtifactID, artifact.CachePath) {
				entry.Descriptor = descriptor
				entry.recoveryState = string(compatiblePreviewArtifactStale)
				entry.recoveryReason = "durable artifact metadata does not match the frozen preview relationship"
				persistCompatiblePreviewRequestStale(db, record.RequestID, entry.recoveryReason)
			} else {
				descriptor.GenerationToken = artifact.GenerationToken
				descriptor.GenerationVersion = artifact.GenerationVersion
				descriptor.persistenceDB = db
				switch artifact.State {
				case string(compatiblePreviewArtifactReady):
					compatiblePreviewArtifacts.Lock()
					if _, exists := compatiblePreviewArtifacts.items[descriptor.ArtifactID]; !exists {
						artifactEntry := newCompatiblePreviewArtifactEntry(descriptor, now, compatiblePreviewArtifactReady)
						artifactEntry.cacheHit = true
						compatiblePreviewArtifacts.items[descriptor.ArtifactID] = artifactEntry
					}
					compatiblePreviewArtifacts.Unlock()
				case string(compatiblePreviewArtifactInterrupted), string(compatiblePreviewArtifactFailed), string(compatiblePreviewArtifactCanceled), string(compatiblePreviewArtifactStale):
					entry.recoveryState = artifact.State
					entry.recoveryReason = artifact.RecoveryReason
				}
			}
		}
	}
	compatiblePreviewRequests.Lock()
	compatiblePreviewRequests.items[record.RequestID] = entry
	compatiblePreviewRequests.Unlock()
	return nil
}

// ReconcileCompatiblePreviewPersistence rebuilds the process-local #1
// registries from bounded durable metadata. It never resumes a process or
// performs media probing; final-file and source-revision checks are stat-only.
func ReconcileCompatiblePreviewPersistence(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	now := time.Now()
	if err := expireCompatiblePreviewRequestRecords(db, now); err != nil {
		return err
	}
	if err := purgeCompatiblePreviewTombstoneRecords(db, now); err != nil {
		return err
	}
	if err := cleanupCompatiblePreviewArtifactRecords(db, now); err != nil {
		return err
	}
	var requestRecords []models.CompatiblePreviewRequestRecord
	if err := db.Where("state IN ? AND expires_at > ?", []string{compatiblePreviewRequestStateActive, compatiblePreviewRequestStateStale}, now).Order("accessed_at DESC").Limit(compatiblePreviewRequestMaxItems).Find(&requestRecords).Error; err != nil {
		return err
	}
	var tombstoneRecords []models.CompatiblePreviewRequestRecord
	if err := db.Where("state = ? AND tombstone_expires_at > ?", compatiblePreviewRequestStateExpiredTombstone, now).Order("tombstone_expires_at ASC").Limit(compatiblePreviewTombstoneMaxItems).Find(&tombstoneRecords).Error; err != nil {
		return err
	}
	artifactRecords, err := compatiblePreviewArtifactRecordsByID(db, requestRecords)
	if err != nil {
		return err
	}
	for id, record := range artifactRecords {
		copy := record
		updated, reconcileErr := reconcileCompatiblePreviewArtifactRecord(db, &copy, now)
		if reconcileErr != nil {
			return reconcileErr
		}
		artifactRecords[id] = updated
	}
	resetCompatiblePreviewRuntimeForReconcile()
	compatiblePreviewRequests.Lock()
	for _, record := range tombstoneRecords {
		if record.TombstoneExpiresAt != nil {
			compatiblePreviewRequests.tombstones[record.RequestID] = compatiblePreviewRequestTombstone{RequestID: record.RequestID, ExpiresAt: record.ExpiresAt, TombstoneExpiresAt: *record.TombstoneExpiresAt, Reason: record.RecoveryReason}
		}
	}
	compatiblePreviewRequests.Unlock()
	for _, record := range requestRecords {
		if err := restoreCompatiblePreviewRequestRecord(db, record, artifactRecords, now); err != nil {
			return err
		}
	}
	return nil
}
