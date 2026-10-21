package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	PublicationReady      = "READY"
	PublicationHeld       = "HELD"
	PublicationPublishing = "PUBLISHING"
	PublicationPublished  = "PUBLISHED"
	PublicationBlocked    = "BLOCKED"
	PublicationFailed     = "FAILED"
	publisherLease        = 15 * time.Minute
)

const (
	publicationActorOperator = "local_operator"
	publicationActorSystem   = "system"
)

const publisherArchiveLease = 15 * time.Minute

type publicationManifestEntry struct {
	Kind              string `json:"kind"`
	Source            string `json:"source"`
	Final             string `json:"final"`
	Temporary         string `json:"temporary,omitempty"`
	Backup            string `json:"backup,omitempty"`
	Required          bool   `json:"required"`
	Promote           bool   `json:"promote"`
	SizeBytes         int64  `json:"sizeBytes,omitempty"`
	Device            uint64 `json:"device,omitempty"`
	Inode             uint64 `json:"inode,omitempty"`
	PreviousSizeBytes int64  `json:"previousSizeBytes,omitempty"`
	PreviousDevice    uint64 `json:"previousDevice,omitempty"`
	PreviousInode     uint64 `json:"previousInode,omitempty"`
	Outcome           string `json:"outcome,omitempty"`
}

type publisherLeaseOwner struct {
	publicationID string
	attemptID     string
	fence         uint64
	actorSource   string
	actorID       string
}

type publisherActionInput struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type PublisherPublicationSummary struct {
	Publication PublisherPublicationView             `json:"publication"`
	Attempts    []models.PublisherPublicationAttempt `json:"attempts"`
	Evidence    []models.PublisherAttemptEvidence    `json:"evidence"`
	Audit       []models.PublisherAuditEvent         `json:"audit"`
}

type PublisherPublicationView struct {
	PublicationID              string          `json:"publicationId"`
	QueueJobID                 uint            `json:"queueJobId"`
	ExecutionNumber            uint            `json:"executionNumber"`
	LibraryID                  uint            `json:"libraryId"`
	DestinationRoot            string          `json:"destinationRoot"`
	DestinationPath            string          `json:"destinationPath"`
	PublishMode                string          `json:"publishMode"`
	ResultArtifactID           string          `json:"resultArtifactId"`
	ValidationRunID            string          `json:"validationRunId"`
	ValidationOverrideEventIDs models.JSONList `json:"validationOverrideEventIds,omitempty"`
	State                      string          `json:"state"`
	HoldReason                 string          `json:"holdReason,omitempty"`
	FailureCode                string          `json:"failureCode,omitempty"`
	FailureSummary             string          `json:"failureSummary,omitempty"`
	AttemptCount               int             `json:"attemptCount"`
	FencingGeneration          uint64          `json:"fencingGeneration"`
	PublishedAt                *time.Time      `json:"publishedAt,omitempty"`
	OriginalArchiveStatus      string          `json:"originalArchiveStatus"`
	OriginalArchivedPath       string          `json:"originalArchivedPath,omitempty"`
	CreatedAt                  time.Time       `json:"createdAt"`
	UpdatedAt                  time.Time       `json:"updatedAt"`
}

type PublisherEligibilityJob struct {
	ID                uint       `json:"id"`
	ExecutionNumber   *uint      `json:"executionNumber,omitempty"`
	MediaPath         string     `json:"mediaPath"`
	OutputPath        string     `json:"outputPath"`
	DestinationPath   string     `json:"destinationPath,omitempty"`
	PublishedPath     string     `json:"publishedPath"`
	PublishedAt       *time.Time `json:"publishedAt,omitempty"`
	ValidationStatus  string     `json:"validationStatus"`
	ValidationOutcome string     `json:"validationOutcome"`
	ValidationScore   int        `json:"validationScore"`
	ValidationRunID   string     `json:"validationRunId,omitempty"`
}

type PublisherEligibilityCandidate struct {
	Job    PublisherEligibilityJob `json:"job"`
	Status string                  `json:"status"`
	Reason string                  `json:"reason,omitempty"`
}

type PublisherEligibilityResponse struct {
	Items []PublisherEligibilityCandidate `json:"items"`
	Limit int                             `json:"limit"`
}

type AssetPublishedSubscriber func(context.Context, string, models.JSONMap) error

type assetPublishedSubscriberRegistration struct {
	id      uint64
	handler AssetPublishedSubscriber
}

var assetPublishedSubscribers []assetPublishedSubscriberRegistration
var assetPublishedSubscribersMu sync.RWMutex
var assetPublishedNextSubscriberID uint64

// RegisterAssetPublishedSubscriber adds a generic downstream integration.
// Subscriber failures are isolated from the publication commit and the same
// stable event ID is retried by the outbox dispatcher.
func RegisterAssetPublishedSubscriber(subscriber AssetPublishedSubscriber) func() {
	if subscriber == nil {
		return func() {}
	}
	assetPublishedSubscribersMu.Lock()
	assetPublishedNextSubscriberID++
	id := assetPublishedNextSubscriberID
	assetPublishedSubscribers = append(assetPublishedSubscribers, assetPublishedSubscriberRegistration{id: id, handler: subscriber})
	assetPublishedSubscribersMu.Unlock()
	return func() {
		assetPublishedSubscribersMu.Lock()
		defer assetPublishedSubscribersMu.Unlock()
		for index := range assetPublishedSubscribers {
			if assetPublishedSubscribers[index].id == id {
				assetPublishedSubscribers = append(assetPublishedSubscribers[:index], assetPublishedSubscribers[index+1:]...)
				return
			}
		}
	}
}

// Publish mode and overwrite authority are frozen on QueueJob. The legacy
// boolean argument remains accepted for compatibility and is ignored.
func (h PublisherHandler) publishQueueJobWithMode(job models.QueueJob, _ bool, automatic bool) (PublishResult, error) {
	return h.handoffAndPublish(job, automatic)
}

func publisherToken(prefix string) (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(token[:]), nil
}

func publisherBoundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func (h PublisherHandler) handoffAndPublish(job models.QueueJob, automatic bool) (PublishResult, error) {
	return h.handoffAndPublishWithReason(job, automatic, "")
}

func (h PublisherHandler) handoffAndPublishWithReason(job models.QueueJob, automatic bool, reason string) (PublishResult, error) {
	if job.ID == 0 || h.db == nil {
		return PublishResult{}, publishError{Status: http.StatusBadRequest, Message: "a persisted Queue job is required"}
	}
	if err := h.db.First(&job, job.ID).Error; err != nil {
		return PublishResult{}, err
	}
	if job.DismissedAt != nil {
		return PublishResult{}, publishError{Status: http.StatusConflict, Message: "dismissed Queue jobs cannot be published"}
	}
	if job.PublishedAt != nil && strings.TrimSpace(job.PublishedPath) != "" {
		if job.ExecutionNumber == nil {
			return PublishResult{JobID: job.ID, Status: "already_published", SourcePath: job.OutputPath, PublishedPath: job.PublishedPath, Message: "Historical publication is recorded on the Queue job; no delivery mutation was performed."}, nil
		}
		var publication models.PublisherPublication
		err := h.db.Where("queue_job_id = ? AND execution_number = ?", job.ID, *job.ExecutionNumber).First(&publication).Error
		if err == nil && publication.State == PublicationPublished {
			return PublishResult{JobID: job.ID, Status: "already_published", SourcePath: job.OutputPath, PublishedPath: job.PublishedPath, Message: "Publication is already committed."}, nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Older Queue publications predate the Publisher-owned record. Keep
			// them readable and idempotent without treating the legacy summary as
			// authority to mutate any destination files again.
			return PublishResult{JobID: job.ID, Status: "already_published", SourcePath: job.OutputPath, PublishedPath: job.PublishedPath, Message: "Historical publication is recorded on the Queue job; no delivery mutation was performed."}, nil
		}
		if err != nil {
			return PublishResult{}, err
		}
	}
	if job.Status != JobStatusCompleted {
		return PublishResult{}, publishError{Status: http.StatusConflict, Message: "job must be completed before publishing"}
	}
	if allowed, reason := canonicalRunAllowsPublish(h.db, job, automatic); !allowed {
		return PublishResult{}, publishError{Status: http.StatusConflict, Message: reason}
	}
	if !automatic {
		overrideEventIDs, err := canonicalEligibleOverrideEventIDs(h.db, job, false)
		if err != nil {
			return PublishResult{}, publishError{Status: http.StatusConflict, Message: "canonical Validation override evidence could not be verified", Err: err}
		}
		if len(overrideEventIDs) > 0 && strings.TrimSpace(reason) == "" {
			return PublishResult{}, publishError{Status: http.StatusBadRequest, Message: "an operator reason is required for override-backed manual publication"}
		}
	}
	identity, resultIdentity, err := publisherIdentityForJob(h.db, job, automatic)
	if err != nil {
		return PublishResult{}, err
	}
	publication, err := h.persistPublicationIdentity(identity, resultIdentity)
	if err != nil {
		return PublishResult{}, err
	}
	actorSource, actorID := publicationActorOperator, publicationActorOperator
	if automatic {
		actorSource, actorID = publicationActorSystem, publicationActorSystem
	} else {
		if reason = publisherBoundedText(reason, 500); reason == "" {
			reason = "Manual Publisher handoff"
		}
		if err := h.appendPublicationAudit(publication.PublicationID, "publish", reason, nil); err != nil {
			return PublishResult{}, err
		}
	}
	if publication.State == PublicationHeld {
		return PublishResult{JobID: job.ID, Status: "held", SourcePath: job.OutputPath, PublishedPath: publication.DestinationPath, Message: "Publication is held by an operator."}, nil
	}
	if publication.State == PublicationPublished {
		return PublishResult{JobID: job.ID, Status: "already_published", SourcePath: job.OutputPath, PublishedPath: publication.DestinationPath, Message: "Publication is already committed."}, nil
	}
	if checks, _, _ := validateRequiredSubtitleArtifacts(job); len(checks) > 0 {
		for _, check := range checks {
			if check.Status == "failed" {
				h.markPublicationBlocked(publication, "required_sidecar_missing", "Required delivery sidecar is not ready: "+check.Message, actorSource, actorID)
				return PublishResult{}, publishError{Status: http.StatusConflict, Message: "required subtitle artifacts are not ready for publication: " + check.Message}
			}
		}
	}
	if publication.State == PublicationBlocked {
		if automatic {
			return PublishResult{JobID: job.ID, Status: "blocked", SourcePath: job.OutputPath, PublishedPath: publication.DestinationPath, Message: publication.FailureSummary}, nil
		}
		if err := h.resetPublicationForRetry(publication.PublicationID, actorSource, actorID, "retry", "Manual retry requested."); err != nil {
			return PublishResult{}, err
		}
	}
	if publication.State == PublicationFailed && !automatic {
		if err := h.resetPublicationForRetry(publication.PublicationID, actorSource, actorID, "retry", "Manual retry requested."); err != nil {
			return PublishResult{}, err
		}
	}
	claimed, owner, err := h.claimPublication(publication.PublicationID, actorSource, actorID, false)
	if err != nil {
		return PublishResult{}, err
	}
	if !claimed {
		return PublishResult{JobID: job.ID, Status: "publishing", SourcePath: job.OutputPath, PublishedPath: publication.DestinationPath, Message: "Another active attempt owns this publication."}, nil
	}
	return h.runPublicationAttempt(job, publication, owner)
}

func publisherIdentityForJob(db *gorm.DB, job models.QueueJob, automatic bool) (models.PublisherPublication, resultArtifactIdentity, error) {
	if job.ExecutionNumber == nil || job.ActiveExecutionPlanID == nil || strings.TrimSpace(job.PlannedLibraryRoot) == "" || strings.TrimSpace(job.PlannedPublishedPath) == "" {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "Queue execution has no frozen plan destination identity"}
	}
	var plan models.ExecutionPlan
	if err := db.Where("id = ? AND job_id = ?", *job.ActiveExecutionPlanID, job.ID).First(&plan).Error; err != nil {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "active frozen ExecutionPlan could not be verified", Err: err}
	}
	if strings.TrimSpace(job.OutputPath) == "" {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "canonical Result output is missing"}
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "canonical Result artifact could not be read", Err: err}
	}
	resultIdentity, ok := resultIdentityFromArtifact(artifact)
	if !ok {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "canonical Result identity is missing"}
	}
	binding, ok := resultBindingFromArtifact(artifact)
	if !ok || binding.ExecutionNumber != *job.ExecutionNumber || binding.ExecutionPlanID != plan.ID || binding.ExecutionPlanVersion != plan.Version {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "canonical Result is bound to a replaced Queue execution or ExecutionPlan"}
	}
	destination := strings.TrimSpace(job.PlannedPublishedPath)
	if job.PublishMode == PublishModeReplaceLibrary {
		destination = strings.TrimSpace(job.ReplacementTargetPath)
	}
	if destination == "" || !pathIsInside(destination, job.PlannedLibraryRoot) {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "frozen destination is missing or outside the planned Library root"}
	}
	if plan.LibraryID != job.LibraryID || filepath.Clean(plan.DestinationRoot) != filepath.Clean(job.PlannedLibraryRoot) || filepath.Clean(plan.DestinationPath) != filepath.Clean(destination) || plan.PublishMode != job.PublishMode || filepath.Clean(plan.OutputPath) != filepath.Clean(job.OutputPath) {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "active ExecutionPlan does not own the Queue-frozen destination identity"}
	}
	var sourceRecordCreatedAt, sourceRecordModifiedAt time.Time
	var sourceRecordSize int64
	if job.SourceAssetRecordID != nil {
		var sourceRecord models.AssetRecord
		if err := db.First(&sourceRecord, *job.SourceAssetRecordID).Error; err != nil || filepath.Clean(sourceRecord.Path) != filepath.Clean(job.MediaPath) || sourceRecord.CreatedAt.IsZero() || sourceRecord.Missing {
			return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "Queue source no longer matches its canonical AssetRecord identity", Err: err}
		}
		sourceRecordCreatedAt, sourceRecordModifiedAt, sourceRecordSize = sourceRecord.CreatedAt, sourceRecord.ModifiedAt, sourceRecord.SizeBytes
	}
	overrideEventIDs, err := canonicalEligibleOverrideEventIDs(db, job, automatic)
	if err != nil {
		return models.PublisherPublication{}, resultArtifactIdentity{}, publishError{Status: http.StatusConflict, Message: "canonical Validation override evidence could not be bound", Err: err}
	}
	validationOverrides := make(models.JSONList, len(overrideEventIDs))
	for index, eventID := range overrideEventIDs {
		validationOverrides[index] = eventID
	}
	return models.PublisherPublication{
		QueueJobID: job.ID, ExecutionNumber: *job.ExecutionNumber, ExecutionPlanID: plan.ID,
		ExecutionPlanVersion: plan.Version, ExecutionPlanFingerprint: resultBindingFingerprint(binding),
		LibraryID: job.LibraryID, DestinationRoot: filepath.Clean(job.PlannedLibraryRoot), DestinationPath: filepath.Clean(destination),
		PublishMode: job.PublishMode, ResultArtifactID: resultIdentity.ArtifactID,
		ResultGenerationToken: resultIdentity.GenerationToken, ResultGenerationVersion: resultIdentity.GenerationVersion,
		ResultOutputRevision: resultIdentity.OutputRevision, ValidationRunID: job.ValidationRunID, ValidationOverrideEventIDs: validationOverrides,
		SourceAssetRecordID: job.SourceAssetRecordID, SourceAssetRecordCreatedAt: sourceRecordCreatedAt,
		SourceAssetRecordModifiedAt: sourceRecordModifiedAt, SourceAssetRecordSizeBytes: sourceRecordSize,
		SourcePath: job.MediaPath,
		State:      PublicationReady, Manifest: publicationManifestForJob(job, destination), OriginalArchiveStatus: "pending",
	}, resultIdentity, nil
}

func publisherJSONListStringsEqual(value models.JSONList, expected []string) bool {
	if len(value) != len(expected) {
		return false
	}
	for index, raw := range value {
		if fmt.Sprint(raw) != expected[index] {
			return false
		}
	}
	return true
}

func publisherJSONListsEqual(left, right models.JSONList) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if fmt.Sprint(left[index]) != fmt.Sprint(right[index]) {
			return false
		}
	}
	return true
}

func resultBindingFingerprint(binding resultExecutionBinding) string {
	return binding.ExecutionPlanFingerprint
}

func publicationManifestForJob(job models.QueueJob, destination string) models.JSONList {
	return buildPublicationManifestForJob(job, destination, true)
}

func publicationManifestForEligibility(job models.QueueJob, destination string) models.JSONList {
	return buildPublicationManifestForJob(job, destination, false)
}

func buildPublicationManifestForJob(job models.QueueJob, destination string, includeExternalSubtitles bool) models.JSONList {
	entries := []publicationManifestEntry{{Kind: "primary", Source: job.OutputPath, Final: destination, Required: true, Promote: true}}
	base := strings.TrimSuffix(destination, filepath.Ext(destination))
	used := map[string]bool{filepath.Clean(destination): true}
	trackPlan, hasTrackPlan := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	for _, artifact := range subtitleArtifactsFromJSON(job.SubtitleArtifacts) {
		if artifact.Status != "" && artifact.Status != "ready" {
			continue
		}
		language := safeSubtitleFilenamePart(artifact.Language)
		if language == "" {
			language = "und"
		}
		suffix := "." + language + subtitleArtifactDispositionSuffix(artifact) + "." + strings.TrimPrefix(strings.ToLower(artifact.Format), ".")
		final := base + suffix
		if used[filepath.Clean(final)] {
			suffix = fmt.Sprintf(".%s.%d%s.%s", language, artifact.StreamIndex, subtitleArtifactDispositionSuffix(artifact), strings.TrimPrefix(strings.ToLower(artifact.Format), "."))
			final = base + suffix
		}
		used[filepath.Clean(final)] = true
		required := false
		if hasTrackPlan {
			for _, planned := range trackPlan.SidecarOutputs {
				if planned.StreamIndex == artifact.StreamIndex && (planned.Format == "" || strings.EqualFold(planned.Format, artifact.Format)) {
					required = true
					break
				}
			}
		}
		entries = append(entries, publicationManifestEntry{Kind: "subtitle", Source: artifact.StagedPath, Final: final, Required: required, Promote: true})
	}
	for _, font := range fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts) {
		if font.Status == "" || font.Status == "ready" {
			final := base + ".fonts/" + filepath.Base(font.SafeFilename)
			entries = append(entries, publicationManifestEntry{Kind: "font", Source: font.StagedPath, Final: final, Required: true, Promote: true})
		}
	}
	if !hasTrackPlan && includeExternalSubtitles {
		sidecars, err := externalSubtitlesForMedia(job.MediaPath)
		if err == nil {
			sourceBase := strings.TrimSuffix(job.MediaPath, filepath.Ext(job.MediaPath))
			for _, sidecar := range sidecars {
				suffix := strings.TrimPrefix(sidecar.Path, sourceBase)
				if suffix == "" || suffix == sidecar.Path {
					continue
				}
				final := base + suffix
				if !used[filepath.Clean(final)] {
					used[filepath.Clean(final)] = true
					entries = append(entries, publicationManifestEntry{Kind: "external_subtitle", Source: sidecar.Path, Final: final, Required: false, Promote: true})
				}
			}
		}
	}
	encoded, _ := json.Marshal(entries)
	var manifest models.JSONList
	_ = json.Unmarshal(encoded, &manifest)
	return manifest
}

func (h PublisherHandler) persistPublicationIdentity(candidate models.PublisherPublication, _ resultArtifactIdentity) (models.PublisherPublication, error) {
	var current models.PublisherPublication
	err := h.db.Where("queue_job_id = ? AND execution_number = ?", candidate.QueueJobID, candidate.ExecutionNumber).First(&current).Error
	if err == nil {
		sourceRecordIdentityChanged := (current.SourceAssetRecordID == nil) != (candidate.SourceAssetRecordID == nil)
		if current.SourceAssetRecordID != nil && candidate.SourceAssetRecordID != nil && *current.SourceAssetRecordID != *candidate.SourceAssetRecordID {
			sourceRecordIdentityChanged = true
		}
		if current.ExecutionNumber != candidate.ExecutionNumber || current.ExecutionPlanID != candidate.ExecutionPlanID || current.ExecutionPlanVersion != candidate.ExecutionPlanVersion || current.ExecutionPlanFingerprint != candidate.ExecutionPlanFingerprint || current.ResultArtifactID != candidate.ResultArtifactID || current.ResultGenerationToken != candidate.ResultGenerationToken || current.ResultGenerationVersion != candidate.ResultGenerationVersion || current.ResultOutputRevision != candidate.ResultOutputRevision || current.ValidationRunID != candidate.ValidationRunID || !publisherJSONListsEqual(current.ValidationOverrideEventIDs, candidate.ValidationOverrideEventIDs) || sourceRecordIdentityChanged || current.SourceAssetRecordCreatedAt != candidate.SourceAssetRecordCreatedAt || current.SourceAssetRecordModifiedAt != candidate.SourceAssetRecordModifiedAt || current.SourceAssetRecordSizeBytes != candidate.SourceAssetRecordSizeBytes || filepath.Clean(current.DestinationRoot) != filepath.Clean(candidate.DestinationRoot) || filepath.Clean(current.DestinationPath) != filepath.Clean(candidate.DestinationPath) || current.PublishMode != candidate.PublishMode {
			if current.ValidationRunID == "" && candidate.ValidationRunID != "" {
				recovered, ok, recoveryErr := h.recoverUnattemptedValidationIdentity(current, candidate)
				if recoveryErr != nil {
					return current, recoveryErr
				}
				if ok {
					return recovered, nil
				}
			}
			if current.State != PublicationPublished {
				h.markPublicationBlocked(current, "identity_changed", "Queue execution, Result, Validation, or frozen destination identity changed.", publicationActorSystem, publicationActorSystem)
			}
			return current, publishError{Status: http.StatusConflict, Message: "publication identity no longer matches the Queue execution"}
		}
		return current, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return current, err
	}
	candidate.PublicationID, err = publisherToken("publication")
	if err != nil {
		return current, err
	}
	now := time.Now().UTC()
	candidate.CreatedAt, candidate.UpdatedAt = now, now
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var job models.QueueJob
		if err := tx.Select("id", "dismissed_at").First(&job, candidate.QueueJobID).Error; err != nil {
			return err
		}
		if job.DismissedAt != nil {
			return publishError{Status: http.StatusConflict, Message: "dismissed Queue jobs cannot create a Publisher record"}
		}
		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
		result := tx.Model(&models.QueueJob{}).Where("id = ? AND dismissed_at IS NULL", candidate.QueueJobID).Updates(map[string]any{
			"publication_id": candidate.PublicationID, "publication_state": PublicationReady, "publication_summary": "Ready for Publisher delivery.",
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return publishError{Status: http.StatusConflict, Message: "Queue job was dismissed before Publisher handoff"}
		}
		return nil
	})
	if err != nil {
		if h.db.Where("queue_job_id = ? AND execution_number = ?", candidate.QueueJobID, candidate.ExecutionNumber).First(&current).Error == nil {
			return current, nil
		}
		return current, err
	}
	return candidate, nil
}

func samePublisherIdentityExceptValidationRun(current, candidate models.PublisherPublication) bool {
	sourceRecordIdentityChanged := (current.SourceAssetRecordID == nil) != (candidate.SourceAssetRecordID == nil)
	if current.SourceAssetRecordID != nil && candidate.SourceAssetRecordID != nil && *current.SourceAssetRecordID != *candidate.SourceAssetRecordID {
		sourceRecordIdentityChanged = true
	}
	return current.QueueJobID == candidate.QueueJobID &&
		current.ExecutionNumber == candidate.ExecutionNumber &&
		current.ExecutionPlanID == candidate.ExecutionPlanID &&
		current.ExecutionPlanVersion == candidate.ExecutionPlanVersion &&
		current.ExecutionPlanFingerprint == candidate.ExecutionPlanFingerprint &&
		current.LibraryID == candidate.LibraryID &&
		filepath.Clean(current.DestinationRoot) == filepath.Clean(candidate.DestinationRoot) &&
		filepath.Clean(current.DestinationPath) == filepath.Clean(candidate.DestinationPath) &&
		current.PublishMode == candidate.PublishMode &&
		current.ResultArtifactID == candidate.ResultArtifactID &&
		current.ResultGenerationToken == candidate.ResultGenerationToken &&
		current.ResultGenerationVersion == candidate.ResultGenerationVersion &&
		current.ResultOutputRevision == candidate.ResultOutputRevision &&
		publisherJSONListsEqual(current.ValidationOverrideEventIDs, candidate.ValidationOverrideEventIDs) &&
		!sourceRecordIdentityChanged &&
		current.SourceAssetRecordCreatedAt == candidate.SourceAssetRecordCreatedAt &&
		current.SourceAssetRecordModifiedAt == candidate.SourceAssetRecordModifiedAt &&
		current.SourceAssetRecordSizeBytes == candidate.SourceAssetRecordSizeBytes &&
		filepath.Clean(current.SourcePath) == filepath.Clean(candidate.SourcePath) &&
		publisherJSONListsEqual(current.Manifest, candidate.Manifest)
}

func publisherManifestHasNoMutation(manifest models.JSONList) bool {
	entries, err := manifestEntries(manifest)
	if err != nil || len(entries) == 0 {
		return false
	}
	for _, entry := range entries {
		if entry.Temporary != "" || entry.Backup != "" || entry.SizeBytes != 0 || entry.Device != 0 || entry.Inode != 0 || entry.PreviousSizeBytes != 0 || entry.PreviousDevice != 0 || entry.PreviousInode != 0 || entry.Outcome != "" {
			return false
		}
	}
	return true
}

func publisherArchiveIsUntouched(publication models.PublisherPublication) bool {
	return publication.OriginalArchiveStatus == "pending" &&
		publication.OriginalArchivedPath == "" &&
		publication.OriginalArchiveSourcePath == "" &&
		publication.OriginalArchiveTemporaryPath == "" &&
		publication.OriginalArchiveSizeBytes == 0 &&
		publication.OriginalArchiveDevice == 0 &&
		publication.OriginalArchiveInode == 0 &&
		publication.OriginalArchiveTempDevice == 0 &&
		publication.OriginalArchiveTempInode == 0 &&
		publication.OriginalArchiveClaimID == "" &&
		publication.OriginalArchiveLeaseExpiresAt == nil &&
		publication.OriginalArchiveFencingGeneration == 0 &&
		publication.OriginalArchiveError == ""
}

func (h PublisherHandler) recoverUnattemptedValidationIdentity(current, candidate models.PublisherPublication) (models.PublisherPublication, bool, error) {
	if current.ValidationRunID != "" || candidate.ValidationRunID == "" ||
		current.State != PublicationBlocked || current.FailureCode != "identity_changed" ||
		current.AttemptCount != 0 || current.FencingGeneration != 0 || current.ActiveAttemptID != "" || current.LeaseExpiresAt != nil ||
		current.PublishedAt != nil || !publisherArchiveIsUntouched(current) || !publisherManifestHasNoMutation(current.Manifest) ||
		!samePublisherIdentityExceptValidationRun(current, candidate) {
		return current, false, nil
	}
	eventID, err := publisherToken("publisher-action")
	if err != nil {
		return current, false, err
	}
	now := time.Now().UTC()
	recovered := false
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var fresh models.PublisherPublication
		if err := tx.Where("publication_id = ?", current.PublicationID).First(&fresh).Error; err != nil {
			return err
		}
		if fresh.ValidationRunID != "" || fresh.State != PublicationBlocked || fresh.FailureCode != "identity_changed" ||
			fresh.AttemptCount != 0 || fresh.FencingGeneration != 0 || fresh.ActiveAttemptID != "" || fresh.LeaseExpiresAt != nil ||
			fresh.PublishedAt != nil || !publisherArchiveIsUntouched(fresh) || !publisherManifestHasNoMutation(fresh.Manifest) ||
			!samePublisherIdentityExceptValidationRun(fresh, candidate) {
			return nil
		}
		var attempts, evidence int64
		if err := tx.Model(&models.PublisherPublicationAttempt{}).Where("publication_id = ?", fresh.PublicationID).Count(&attempts).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.PublisherAttemptEvidence{}).Where("publication_id = ?", fresh.PublicationID).Count(&evidence).Error; err != nil {
			return err
		}
		if attempts != 0 || evidence != 0 {
			return nil
		}
		updated := tx.Model(&models.PublisherPublication{}).
			Where("publication_id = ? AND state = ? AND failure_code = ? AND validation_run_id = '' AND attempt_count = 0 AND fencing_generation = 0", fresh.PublicationID, PublicationBlocked, "identity_changed").
			Updates(map[string]any{"validation_run_id": candidate.ValidationRunID, "state": PublicationReady, "failure_code": "", "failure_summary": "", "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return nil
		}
		jobUpdate := tx.Model(&models.QueueJob{}).Where("id = ? AND execution_number = ?", fresh.QueueJobID, fresh.ExecutionNumber).
			Updates(map[string]any{"publication_id": fresh.PublicationID, "publication_state": PublicationReady, "publication_summary": "Validation identity was recovered before any Publisher attempt."})
		if jobUpdate.Error != nil {
			return jobUpdate.Error
		}
		if jobUpdate.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		var auditCount int64
		if err := tx.Model(&models.PublisherAuditEvent{}).Where("publication_id = ?", fresh.PublicationID).Count(&auditCount).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.PublisherAuditEvent{
			EventID: eventID, PublicationID: fresh.PublicationID, Version: int(auditCount + 1),
			Action: "recover_validation_identity", ActorSource: publicationActorSystem, ActorID: publicationActorSystem,
			Reason:     "Recovered an empty ValidationRun binding before any publication attempt or filesystem mutation.",
			Parameters: models.JSONMap{"recovery": "zero_attempt_validation_handoff_race", "validationRunId": candidate.ValidationRunID},
			CreatedAt:  now,
		}).Error; err != nil {
			return err
		}
		recovered = true
		return nil
	})
	if err != nil || !recovered {
		return current, recovered, err
	}
	if err := h.db.Where("publication_id = ?", current.PublicationID).First(&current).Error; err != nil {
		return models.PublisherPublication{}, false, err
	}
	return current, true, nil
}

func (h PublisherHandler) appendPublicationAudit(publicationID, action, reason string, parameters models.JSONMap) error {
	return h.appendPublicationAuditAs(publicationID, action, reason, parameters, publicationActorOperator, publicationActorOperator)
}

func (h PublisherHandler) appendPublicationAuditAs(publicationID, action, reason string, parameters models.JSONMap, actorSource, actorID string) error {
	eventID, err := publisherToken("publisher-action")
	if err != nil {
		return err
	}
	return h.db.Transaction(func(tx *gorm.DB) error {
		var current models.PublisherPublication
		if err := tx.Where("publication_id = ?", publicationID).First(&current).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.PublisherAuditEvent{}).Where("publication_id = ?", publicationID).Count(&count).Error; err != nil {
			return err
		}
		event := models.PublisherAuditEvent{EventID: eventID, PublicationID: publicationID, Version: int(count + 1), Action: action, ActorSource: actorSource, ActorID: actorID, Reason: publisherBoundedText(reason, 500), Parameters: parameters, CreatedAt: time.Now().UTC()}
		return tx.Create(&event).Error
	})
}

func (h PublisherHandler) markPublicationBlocked(publication models.PublisherPublication, code, summary, actorSource, actorID string) {
	now := time.Now().UTC()
	_ = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state IN ?", publication.PublicationID, []string{PublicationReady, PublicationFailed, PublicationBlocked}).Updates(map[string]any{"state": PublicationBlocked, "failure_code": publisherBoundedText(code, 100), "failure_summary": publisherBoundedText(summary, 1000), "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.QueueJob{}).Where("id = ?", publication.QueueJobID).Updates(map[string]any{"publication_id": publication.PublicationID, "publication_state": PublicationBlocked, "publication_summary": publisherBoundedText(summary, 1000)}).Error; err != nil {
			return err
		}
		id, err := publisherToken("publisher-action")
		if err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.PublisherAuditEvent{}).Where("publication_id = ?", publication.PublicationID).Count(&count).Error; err != nil {
			return err
		}
		return tx.Create(&models.PublisherAuditEvent{EventID: id, PublicationID: publication.PublicationID, Version: int(count + 1), Action: "handoff_blocked", ActorSource: actorSource, ActorID: actorID, Reason: publisherBoundedText(summary, 500), CreatedAt: now}).Error
	})
}

func (h PublisherHandler) resetPublicationForRetry(publicationID, actorSource, actorID, action, reason string) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		var current models.PublisherPublication
		if err := tx.Where("publication_id = ?", publicationID).First(&current).Error; err != nil {
			return err
		}
		if current.State != PublicationFailed && current.State != PublicationBlocked {
			return publishError{Status: http.StatusConflict, Message: "publication is not retryable in its current state"}
		}
		if err := tx.Model(&current).Where("state = ?", current.State).Updates(map[string]any{"state": PublicationReady, "failure_code": "", "failure_summary": "", "hold_reason": "", "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.QueueJob{}).Where("id = ?", current.QueueJobID).Updates(map[string]any{"publication_state": PublicationReady, "publication_summary": "Retry requested; waiting for a fenced attempt."}).Error; err != nil {
			return err
		}
		id, err := publisherToken("publisher-action")
		if err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.PublisherAuditEvent{}).Where("publication_id = ?", publicationID).Count(&count).Error; err != nil {
			return err
		}
		return tx.Create(&models.PublisherAuditEvent{EventID: id, PublicationID: publicationID, Version: int(count + 1), Action: action, ActorSource: actorSource, ActorID: actorID, Reason: publisherBoundedText(reason, 500), CreatedAt: time.Now().UTC()}).Error
	})
}

func (h PublisherHandler) claimPublication(publicationID, actorSource, actorID string, recovery bool) (bool, publisherLeaseOwner, error) {
	owner := publisherLeaseOwner{publicationID: publicationID, actorSource: actorSource, actorID: actorID}
	now := time.Now().UTC()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var publication models.PublisherPublication
		if err := tx.Where("publication_id = ?", publicationID).First(&publication).Error; err != nil {
			return err
		}
		var queueJob models.QueueJob
		if err := tx.Select("id", "dismissed_at").First(&queueJob, publication.QueueJobID).Error; err != nil {
			return err
		}
		if queueJob.DismissedAt != nil {
			return publishError{Status: http.StatusConflict, Message: "dismissed Queue jobs cannot be claimed by Publisher"}
		}
		if publication.AttemptCount >= 100 && !recovery {
			if err := tx.Model(&publication).Where("state = ?", PublicationReady).Updates(map[string]any{"state": PublicationBlocked, "failure_code": "attempt_limit", "failure_summary": "Publication reached the bounded retry limit; create a new Queue execution after review.", "updated_at": now}).Error; err != nil {
				return err
			}
			return tx.Model(&models.QueueJob{}).Where("id = ?", publication.QueueJobID).Updates(map[string]any{"publication_state": PublicationBlocked, "publication_summary": "Publication reached the bounded retry limit."}).Error
		}
		if recovery {
			if strings.TrimSpace(publication.ActiveAttemptID) == "" {
				return nil
			}
			owner.attemptID = publication.ActiveAttemptID
		} else {
			attemptID, err := publisherToken("attempt")
			if err != nil {
				return err
			}
			owner.attemptID = attemptID
		}
		owner.fence = publication.FencingGeneration + 1
		query := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND fencing_generation = ?", publicationID, publication.FencingGeneration)
		if recovery {
			query = query.Where("state = ? AND active_attempt_id = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", PublicationPublishing, publication.ActiveAttemptID, now)
		} else {
			query = query.Where("state = ? AND (active_attempt_id = '' OR active_attempt_id IS NULL)", PublicationReady)
		}
		lease := now.Add(publisherLease)
		updates := map[string]any{"state": PublicationPublishing, "active_attempt_id": owner.attemptID, "lease_expires_at": lease, "fencing_generation": owner.fence, "updated_at": now, "failure_code": "", "failure_summary": ""}
		if !recovery {
			updates["attempt_count"] = publication.AttemptCount + 1
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		var job models.QueueJob
		if err := tx.First(&job, publication.QueueJobID).Error; err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStagePublishing); err != nil {
			return err
		}
		if !recovery {
			attempt := models.PublisherPublicationAttempt{AttemptID: owner.attemptID, PublicationID: publicationID, FencingGeneration: owner.fence, ActorSource: actorSource, ActorID: actorID, CreatedAt: now}
			if err := tx.Create(&attempt).Error; err != nil {
				return err
			}
		}
		evidenceID, err := publisherToken("attempt-evidence")
		if err != nil {
			return err
		}
		var evidenceCount int64
		if err := tx.Model(&models.PublisherAttemptEvidence{}).Where("attempt_id = ?", owner.attemptID).Count(&evidenceCount).Error; err != nil {
			return err
		}
		outcome := "acquired"
		if recovery {
			outcome = "reacquired"
		}
		if err := tx.Create(&models.PublisherAttemptEvidence{EvidenceID: evidenceID, PublicationID: publicationID, AttemptID: owner.attemptID, FencingGeneration: owner.fence, Version: int(evidenceCount + 1), Phase: "claim", Outcome: outcome, CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Model(&models.QueueJob{}).Where("id = ?", publication.QueueJobID).Updates(map[string]any{"publication_id": publicationID, "publication_state": PublicationPublishing, "publication_summary": "Publisher attempt is active."}).Error
	})
	if err != nil {
		return false, owner, err
	}
	var current models.PublisherPublication
	if err := h.db.Where("publication_id = ?", publicationID).First(&current).Error; err != nil {
		return false, owner, err
	}
	if current.State == PublicationBlocked && current.FailureCode == "attempt_limit" {
		return false, owner, publishError{Status: http.StatusConflict, Message: current.FailureSummary}
	}
	return owner.attemptID != "" && current.ActiveAttemptID == owner.attemptID && current.FencingGeneration == owner.fence, owner, nil
}

func (h PublisherHandler) assertPublicationLease(owner publisherLeaseOwner, renew bool) error {
	now := time.Now().UTC()
	updates := map[string]any{}
	if renew {
		updates["lease_expires_at"] = now.Add(publisherLease)
		updates["updated_at"] = now
	}
	query := h.db.Model(&models.PublisherPublication{}).
		Where("publication_id = ? AND state = ? AND active_attempt_id = ? AND fencing_generation = ? AND lease_expires_at > ?", owner.publicationID, PublicationPublishing, owner.attemptID, owner.fence, now).
		Where("EXISTS (SELECT 1 FROM queue_jobs WHERE queue_jobs.id = publisher_publications.queue_job_id AND queue_jobs.dismissed_at IS NULL)")
	if renew {
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("publisher claim expired or was fenced by another attempt")
		}
		return nil
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("publisher claim expired or was fenced by another attempt")
	}
	return nil
}

func (h PublisherHandler) appendAttemptEvidence(owner publisherLeaseOwner, phase, outcome, code, summary string, paths ...string) error {
	id, err := publisherToken("attempt-evidence")
	if err != nil {
		return err
	}
	boundedPaths := make(models.JSONList, 0, len(paths))
	for _, value := range paths {
		if len(boundedPaths) == 20 {
			break
		}
		boundedPaths = append(boundedPaths, publisherBoundedText(value, 1024))
	}
	now := time.Now().UTC()
	return h.db.Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state = ? AND active_attempt_id = ? AND fencing_generation = ? AND lease_expires_at > ?", owner.publicationID, PublicationPublishing, owner.attemptID, owner.fence, now).Count(&active).Error; err != nil {
			return err
		}
		if active != 1 {
			return errors.New("publisher claim expired or was fenced before attempt evidence append")
		}
		var count int64
		if err := tx.Model(&models.PublisherAttemptEvidence{}).Where("attempt_id = ?", owner.attemptID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 20 {
			return nil
		}
		return tx.Create(&models.PublisherAttemptEvidence{EvidenceID: id, PublicationID: owner.publicationID, AttemptID: owner.attemptID, FencingGeneration: owner.fence, Version: int(count + 1), Phase: publisherBoundedText(phase, 50), Outcome: publisherBoundedText(outcome, 50), Code: publisherBoundedText(code, 100), Summary: publisherBoundedText(summary, 1000), Paths: boundedPaths, CreatedAt: now}).Error
	})
}

func (h PublisherHandler) finishAttempt(owner publisherLeaseOwner, state, code, summary string) error {
	now := time.Now().UTC()
	return h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state = ? AND active_attempt_id = ? AND fencing_generation = ? AND lease_expires_at > ?", owner.publicationID, PublicationPublishing, owner.attemptID, owner.fence, now).Updates(map[string]any{"state": state, "active_attempt_id": "", "lease_expires_at": nil, "failure_code": publisherBoundedText(code, 100), "failure_summary": publisherBoundedText(summary, 1000), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("publisher attempt lost its fencing claim")
		}
		var publication models.PublisherPublication
		if err := tx.Where("publication_id = ?", owner.publicationID).First(&publication).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.QueueJob{}).Where("id = ?", publication.QueueJobID).Updates(map[string]any{"publication_state": state, "publication_summary": publisherBoundedText(summary, 1000)}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (h PublisherHandler) ListPublications(c *gin.Context) {
	var publications []models.PublisherPublication
	if err := h.db.Order("updated_at desc").Limit(100).Find(&publications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(publications) == 0 {
		c.JSON(http.StatusOK, gin.H{"items": []PublisherPublicationSummary{}, "limit": 100})
		return
	}
	ids := make([]string, 0, len(publications))
	for _, publication := range publications {
		ids = append(ids, publication.PublicationID)
	}
	var attempts []models.PublisherPublicationAttempt
	var evidence []models.PublisherAttemptEvidence
	var audit []models.PublisherAuditEvent
	if err := h.db.Where("publication_id IN ?", ids).Order("created_at desc").Limit(500).Find(&attempts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Where("publication_id IN ?", ids).Order("created_at desc").Limit(1000).Find(&evidence).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Where("publication_id IN ?", ids).Order("created_at desc").Limit(500).Find(&audit).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]PublisherPublicationSummary, 0, len(publications))
	for _, publication := range publications {
		item := PublisherPublicationSummary{Publication: PublisherPublicationView{
			PublicationID: publication.PublicationID, QueueJobID: publication.QueueJobID, ExecutionNumber: publication.ExecutionNumber,
			LibraryID: publication.LibraryID, DestinationRoot: publication.DestinationRoot, DestinationPath: publication.DestinationPath,
			PublishMode: publication.PublishMode, ResultArtifactID: publication.ResultArtifactID, ValidationRunID: publication.ValidationRunID,
			ValidationOverrideEventIDs: publication.ValidationOverrideEventIDs,
			State:                      publication.State, HoldReason: publication.HoldReason, FailureCode: publication.FailureCode,
			FailureSummary: publication.FailureSummary, AttemptCount: publication.AttemptCount,
			FencingGeneration: publication.FencingGeneration, PublishedAt: publication.PublishedAt,
			OriginalArchiveStatus: publication.OriginalArchiveStatus, OriginalArchivedPath: publication.OriginalArchivedPath,
			CreatedAt: publication.CreatedAt, UpdatedAt: publication.UpdatedAt,
		}, Attempts: []models.PublisherPublicationAttempt{}, Evidence: []models.PublisherAttemptEvidence{}, Audit: []models.PublisherAuditEvent{}}
		for _, attempt := range attempts {
			if attempt.PublicationID == publication.PublicationID && len(item.Attempts) < 10 {
				item.Attempts = append(item.Attempts, attempt)
			}
		}
		for _, record := range evidence {
			if record.PublicationID == publication.PublicationID && len(item.Evidence) < 20 {
				item.Evidence = append(item.Evidence, record)
			}
		}
		for _, record := range audit {
			if record.PublicationID == publication.PublicationID && len(item.Audit) < 20 {
				item.Audit = append(item.Audit, record)
			}
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "limit": 100})
}

const publisherEligibilityLimit = 100

func (h PublisherHandler) ListManualEligibility(c *gin.Context) {
	limit := publisherEligibilityLimit
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		requested, err := strconv.Atoi(rawLimit)
		if err != nil || requested < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		if requested < limit {
			limit = requested
		}
	}

	var jobs []models.QueueJob
	if err := h.db.Where("status = ? AND published_at IS NULL AND dismissed_at IS NULL", JobStatusCompleted).
		Order("updated_at desc, id desc").Limit(limit).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]PublisherEligibilityCandidate, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, h.manualEligibilityForJob(job))
	}
	c.JSON(http.StatusOK, PublisherEligibilityResponse{Items: items, Limit: limit})
}

func (h PublisherHandler) manualEligibilityForJob(job models.QueueJob) PublisherEligibilityCandidate {
	candidate := PublisherEligibilityCandidate{
		Job: PublisherEligibilityJob{
			ID: job.ID, ExecutionNumber: job.ExecutionNumber, MediaPath: job.MediaPath, OutputPath: job.OutputPath,
			PublishedPath: job.PublishedPath, PublishedAt: job.PublishedAt, ValidationStatus: job.ValidationStatus,
			ValidationOutcome: job.ValidationOutcome, ValidationScore: job.ValidationScore, ValidationRunID: job.ValidationRunID,
		},
		Status: "BLOCKED",
	}
	block := func(reason string) PublisherEligibilityCandidate {
		candidate.Reason = reason
		return candidate
	}
	if job.DismissedAt != nil {
		return block("Queue job was dismissed and cannot be published")
	}

	if allowed, reason := canonicalRunAllowsPublish(h.db, job, false); !allowed {
		return block(reason)
	}
	publication, _, err := publisherIdentityForJob(h.db, job, false)
	if err != nil {
		return block(err.Error())
	}
	candidate.Job.DestinationPath = publication.DestinationPath
	if err := checkControlledPublisherRootReadOnly(publication.DestinationRoot); err != nil {
		return block(err.Error())
	}
	if checks, _, _ := validateRequiredSubtitleArtifacts(job); len(checks) > 0 {
		for _, check := range checks {
			if check.Status == "failed" {
				return block("required subtitle artifacts are not ready: " + check.Message)
			}
		}
	}
	if checks, _, _ := validateRequiredFontAttachmentArtifacts(job); len(checks) > 0 {
		for _, check := range checks {
			if check.Status == "failed" {
				return block("required font attachment is not ready: " + check.Message)
			}
		}
	}
	if job.PublishMode == PublishModeReplaceLibrary {
		if err := (PublisherHandler{db: h.db}).verifyReplacementAuthority(job, publication); err != nil {
			return block(err.Error())
		}
	}

	manifest := publicationManifestForEligibility(job, publication.DestinationPath)
	entries, err := manifestEntries(manifest)
	if err != nil || len(entries) == 0 {
		return block("frozen delivery manifest is missing or invalid")
	}
	previewRoot := filepath.Join(publication.DestinationRoot, ".mvforge-staging", ".eligibility-preview")
	for index := range entries {
		entries[index].Temporary = filepath.Join(previewRoot, fmt.Sprintf("artifact-%03d.part", index))
		if job.PublishMode == PublishModeReplaceLibrary && entries[index].Kind == "primary" && filepath.Clean(entries[index].Final) == filepath.Clean(job.MediaPath) {
			entries[index].Backup = filepath.Join(previewRoot, "original.backup")
		}
	}
	if err := h.preflightPublicationForEligibility(job, publication, &entries); err != nil {
		return block(err.Error())
	}
	overrideEventIDs, err := canonicalEligibleOverrideEventIDs(h.db, job, false)
	if err != nil {
		return block("canonical Validation override evidence could not be read")
	}
	candidate.Status = "ALLOWED"
	if len(overrideEventIDs) > 0 {
		candidate.Status = "ALLOWED_BY_OVERRIDE"
	}
	return candidate
}

func checkControlledPublisherRootReadOnly(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return errors.New("frozen Library destination root must be absolute")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("frozen Library destination root is unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("frozen Library destination root is not a controlled directory")
	}
	return nil
}

func (h PublisherHandler) PublisherAction(c *gin.Context) {
	var input publisherActionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	input.Reason = publisherBoundedText(input.Reason, 500)
	if input.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a reason is required for audited Publisher decisions"})
		return
	}
	var publication models.PublisherPublication
	if err := h.db.Where("publication_id = ?", c.Param("publicationId")).First(&publication).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "publication not found"})
		return
	}
	var queueJob models.QueueJob
	if err := h.db.Select("id", "dismissed_at").First(&queueJob, publication.QueueJobID).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if queueJob.DismissedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "dismissed Queue jobs cannot be managed by Publisher"})
		return
	}
	switch input.Action {
	case "hold":
		err := h.operatorPublicationTransition(publication, PublicationHeld, input.Action, input.Reason, []string{PublicationReady, PublicationFailed, PublicationBlocked})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"publicationId": publication.PublicationID, "state": PublicationHeld})
	case "release":
		err := h.operatorPublicationTransition(publication, PublicationReady, input.Action, input.Reason, []string{PublicationHeld})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"publicationId": publication.PublicationID, "state": PublicationReady})
	case "retry":
		if publication.State == PublicationHeld {
			c.JSON(http.StatusConflict, gin.H{"error": "release the publication hold before retrying"})
			return
		}
		if err := h.resetPublicationForRetry(publication.PublicationID, publicationActorOperator, publicationActorOperator, input.Action, input.Reason); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		var job models.QueueJob
		if err := h.db.First(&job, publication.QueueJobID).Error; err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		result, err := h.handoffAndPublish(job, false)
		if err != nil {
			status := http.StatusInternalServerError
			var publishErr publishError
			if errors.As(err, &publishErr) && publishErr.Status > 0 {
				status = publishErr.Status
			}
			c.JSON(status, gin.H{"error": err.Error(), "publicationId": publication.PublicationID})
			return
		}
		c.JSON(http.StatusOK, result)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be hold, release, or retry"})
	}
}

func (h PublisherHandler) operatorPublicationTransition(publication models.PublisherPublication, nextState, action, reason string, allowedStates []string) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state IN ?", publication.PublicationID, allowedStates).Updates(map[string]any{"state": nextState, "hold_reason": func() string {
			if nextState == PublicationHeld {
				return publisherBoundedText(reason, 500)
			}
			return ""
		}(), "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("publication state changed; refresh Publisher and retry the action")
		}
		if err := tx.Model(&models.QueueJob{}).Where("id = ?", publication.QueueJobID).Updates(map[string]any{"publication_state": nextState, "publication_summary": publisherBoundedText(action+": "+reason, 1000)}).Error; err != nil {
			return err
		}
		id, err := publisherToken("publisher-action")
		if err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.PublisherAuditEvent{}).Where("publication_id = ?", publication.PublicationID).Count(&count).Error; err != nil {
			return err
		}
		return tx.Create(&models.PublisherAuditEvent{EventID: id, PublicationID: publication.PublicationID, Version: int(count + 1), Action: action, ActorSource: publicationActorOperator, ActorID: publicationActorOperator, Reason: publisherBoundedText(reason, 500), CreatedAt: time.Now().UTC()}).Error
	})
}

func (p *AutoPublisher) dispatchAssetPublishedOutbox() {
	assetPublishedSubscribersMu.RLock()
	subscribers := append([]assetPublishedSubscriberRegistration{}, assetPublishedSubscribers...)
	assetPublishedSubscribersMu.RUnlock()
	if len(subscribers) == 0 {
		return
	}
	now := time.Now().UTC()
	var events []models.AssetPublishedOutbox
	if err := p.db.Where("status = ? OR (status = ? AND lease_expires_at <= ?)", "pending", "delivering", now).Order("created_at asc").Limit(25).Find(&events).Error; err != nil {
		return
	}
	for _, event := range events {
		lease := now.Add(5 * time.Minute)
		result := p.db.Model(&models.AssetPublishedOutbox{}).Where("id = ? AND (status = ? OR (status = ? AND lease_expires_at <= ?))", event.ID, "pending", "delivering", now).Updates(map[string]any{"status": "delivering", "attempt_count": event.AttemptCount + 1, "lease_expires_at": lease, "updated_at": now})
		if result.Error != nil || result.RowsAffected != 1 {
			continue
		}
		var deliveryErr error
		for _, subscriber := range subscribers {
			if err := subscriber.handler(context.Background(), event.EventID, event.Payload); err != nil {
				deliveryErr = err
				break
			}
		}
		if deliveryErr != nil {
			_ = p.db.Model(&models.AssetPublishedOutbox{}).Where("id = ? AND status = ?", event.ID, "delivering").Updates(map[string]any{"status": "pending", "lease_expires_at": nil, "last_error": publisherBoundedText(deliveryErr.Error(), 1000), "updated_at": time.Now().UTC()}).Error
			continue
		}
		deliveredAt := time.Now().UTC()
		_ = p.db.Model(&models.AssetPublishedOutbox{}).Where("id = ? AND status = ?", event.ID, "delivering").Updates(map[string]any{"status": "delivered", "lease_expires_at": nil, "last_error": "", "delivered_at": deliveredAt, "updated_at": deliveredAt}).Error
	}
}

func manifestEntries(values models.JSONList) ([]publicationManifestEntry, error) {
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var entries []publicationManifestEntry
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func manifestValue(entries []publicationManifestEntry) models.JSONList {
	encoded, _ := json.Marshal(entries)
	var values models.JSONList
	_ = json.Unmarshal(encoded, &values)
	return values
}

func (h PublisherHandler) runPublicationAttempt(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner) (PublishResult, error) {
	if err := h.db.Where("publication_id = ?", owner.publicationID).First(&publication).Error; err != nil {
		return PublishResult{}, err
	}
	if err := h.db.First(&job, publication.QueueJobID).Error; err != nil {
		return PublishResult{}, err
	}
	if err := h.verifyBoundDelivery(job, publication, owner.actorID == publicationActorSystem); err != nil {
		return h.blockPublicationAttempt(job, publication, owner, "stale_identity", err.Error())
	}
	entries, err := manifestEntries(publication.Manifest)
	if err != nil || len(entries) == 0 {
		return h.blockPublicationAttempt(job, publication, owner, "manifest_invalid", "Persisted delivery manifest is missing or invalid.")
	}
	if err := h.assertPublicationLease(owner, true); err != nil {
		return PublishResult{}, err
	}
	if err := ensureControlledPublisherRoot(publication.DestinationRoot); err != nil {
		return h.blockPublicationAttempt(job, publication, owner, "controlled_root", err.Error())
	}
	if job.PublishMode == PublishModeReplaceLibrary {
		if err := h.verifyReplacementAuthority(job, publication); err != nil {
			return h.blockPublicationAttempt(job, publication, owner, "replacement_identity", err.Error())
		}
	}

	stagingRoot := filepath.Join(publication.DestinationRoot, ".mvforge-staging", publication.PublicationID, owner.attemptID)
	if err := ensureNoSymlinkPath(publication.DestinationRoot, stagingRoot, false); err != nil {
		return h.blockPublicationAttempt(job, publication, owner, "staging_root", err.Error())
	}
	if err := h.assertPublicationLease(owner, true); err != nil {
		return PublishResult{}, err
	}
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return h.failPublicationAttempt(job, publication, owner, "staging_directory", err)
	}
	if err := ensureNoSymlinkPath(publication.DestinationRoot, stagingRoot, true); err != nil {
		return h.blockPublicationAttempt(job, publication, owner, "staging_root", err.Error())
	}
	for i := range entries {
		if entries[i].Temporary == "" {
			entries[i].Temporary = filepath.Join(stagingRoot, fmt.Sprintf("artifact-%03d.part", i))
		}
		if entries[i].Backup == "" && job.PublishMode == PublishModeReplaceLibrary && entries[i].Kind == "primary" && filepath.Clean(entries[i].Final) == filepath.Clean(job.MediaPath) {
			entries[i].Backup = filepath.Join(stagingRoot, "original.backup")
		}
	}
	if err := h.persistManifest(owner, entries); err != nil {
		return PublishResult{}, err
	}
	if err := h.preflightPublication(job, publication, owner, &entries); err != nil {
		if isPublicationBlocked(err) {
			return h.blockPublicationAttempt(job, publication, owner, "destination_conflict", err.Error())
		}
		return h.failPublicationAttempt(job, publication, owner, "destination_preflight", err)
	}
	if err := h.persistManifest(owner, entries); err != nil {
		return PublishResult{}, err
	}
	for i := range entries {
		if !entries[i].Promote {
			continue
		}
		if tokenMatches(entries[i].Final, entries[i]) {
			entries[i].Outcome = "committed"
			continue
		}
		if err := h.assertPublicationLease(owner, true); err != nil {
			return PublishResult{}, err
		}
		if tokenMatches(entries[i].Temporary, entries[i]) {
			continue
		}
		if _, statErr := os.Lstat(entries[i].Temporary); statErr == nil && entries[i].Device == 0 {
			return h.blockPublicationAttempt(job, publication, owner, "staging_ownership_ambiguous", "A temporary delivery path exists without a persisted ownership receipt.")
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return h.failPublicationAttempt(job, publication, owner, "staging_stat", statErr)
		}
		if err := h.stagePublicationFile(owner, entries[i].Source, entries[i].Temporary); err != nil {
			if entries[i].Required {
				return h.failPublicationAttempt(job, publication, owner, "stage_failed", err)
			}
			entries[i].Promote = false
			entries[i].Outcome = "optional_stage_failed: " + publisherBoundedText(err.Error(), 300)
			continue
		}
		if err := recordFileOwnership(entries[i].Temporary, &entries[i]); err != nil {
			return h.failPublicationAttempt(job, publication, owner, "stage_identity", err)
		}
		if err := h.persistManifest(owner, entries); err != nil {
			return PublishResult{}, err
		}
	}
	if err := h.persistManifest(owner, entries); err != nil {
		return PublishResult{}, err
	}
	if err := h.verifyBoundDelivery(job, publication, owner.actorID == publicationActorSystem); err != nil {
		return h.rollbackOrBlock(job, publication, owner, entries, "result_changed", err.Error())
	}
	if err := h.promotePublicationManifest(job, publication, owner, entries); err != nil {
		return h.rollbackOrBlock(job, publication, owner, entries, "promotion_failed", err.Error())
	}
	if err := h.commitPublishedPackage(job, publication, owner, entries); err != nil {
		// Final receipts are already promoted and persisted in the manifest. A
		// provenance transaction error is uncertain at this boundary, so preserve
		// the PUBLISHING claim for fenced recovery instead of rolling back owned
		// final files as an ordinary pre-publication failure.
		return PublishResult{}, fmt.Errorf("publication provenance commit failed; recovery may retry: %w", err)
	}
	_ = scheduler.ReleaseReservation(h.db, job.ID)
	if publication.DestinationPath != job.OutputPath {
		_ = h.cleanupStagedJob(job)
	}
	h.finishOriginalArchive(job, publication, entries)
	return PublishResult{JobID: job.ID, Status: "published", SourcePath: job.OutputPath, PublishedPath: publication.DestinationPath, Message: "Output and required sidecars were committed to the frozen destination."}, nil
}

func (h PublisherHandler) verifyBoundDelivery(job models.QueueJob, publication models.PublisherPublication, automatic bool) error {
	if job.ID != publication.QueueJobID || job.Status != JobStatusCompleted || job.ExecutionNumber == nil || *job.ExecutionNumber != publication.ExecutionNumber || job.ActiveExecutionPlanID == nil || *job.ActiveExecutionPlanID != publication.ExecutionPlanID || job.LibraryID != publication.LibraryID || job.PublishMode != publication.PublishMode || filepath.Clean(job.PlannedLibraryRoot) != filepath.Clean(publication.DestinationRoot) {
		return errors.New("Queue execution or frozen destination binding changed after publication identity was created")
	}
	destination := job.PlannedPublishedPath
	if job.PublishMode == PublishModeReplaceLibrary {
		destination = job.ReplacementTargetPath
	}
	if filepath.Clean(destination) != filepath.Clean(publication.DestinationPath) {
		return errors.New("frozen destination path changed after publication identity was created")
	}
	if (job.SourceAssetRecordID == nil) != (publication.SourceAssetRecordID == nil) || job.SourceAssetRecordID != nil && *job.SourceAssetRecordID != *publication.SourceAssetRecordID {
		return errors.New("Queue source AssetRecord identity changed after publication identity was created")
	}
	if publication.SourceAssetRecordID != nil {
		var record models.AssetRecord
		if err := h.db.First(&record, *publication.SourceAssetRecordID).Error; err != nil || filepath.Clean(record.Path) != filepath.Clean(publication.SourcePath) || record.Missing || !record.CreatedAt.Equal(publication.SourceAssetRecordCreatedAt) || !record.ModifiedAt.Equal(publication.SourceAssetRecordModifiedAt) || record.SizeBytes != publication.SourceAssetRecordSizeBytes {
			return errors.New("source AssetRecord provenance changed after publication identity was created")
		}
	}
	var plan models.ExecutionPlan
	if err := h.db.Where("id = ? AND job_id = ?", publication.ExecutionPlanID, job.ID).First(&plan).Error; err != nil || plan.Version != publication.ExecutionPlanVersion || plan.LibraryID != publication.LibraryID || filepath.Clean(plan.DestinationRoot) != filepath.Clean(publication.DestinationRoot) || filepath.Clean(plan.DestinationPath) != filepath.Clean(publication.DestinationPath) || plan.PublishMode != publication.PublishMode || filepath.Clean(plan.OutputPath) != filepath.Clean(job.OutputPath) {
		return errors.New("active ExecutionPlan destination binding changed after publication identity was created")
	}
	if allowed, reason := canonicalRunAllowsPublish(h.db, job, automatic); !allowed {
		return errors.New(reason)
	}
	overrideEventIDs, err := canonicalEligibleOverrideEventIDs(h.db, job, automatic)
	if err != nil || !publisherJSONListStringsEqual(publication.ValidationOverrideEventIDs, overrideEventIDs) {
		return errors.New("canonical Validation override identity changed after publication handoff")
	}
	artifact, _, err := readLatestJobArtifact(h.db, job, "result")
	if err != nil {
		return fmt.Errorf("canonical Result artifact cannot be read: %w", err)
	}
	identity, ok := resultIdentityFromArtifact(artifact)
	if !ok || identity.ArtifactID != publication.ResultArtifactID || identity.GenerationToken != publication.ResultGenerationToken || identity.GenerationVersion != publication.ResultGenerationVersion || identity.OutputRevision != publication.ResultOutputRevision {
		return errors.New("canonical Result was replaced or its Validation binding is stale")
	}
	if evidence, err := resultOutputRevisionForIdentity(job.OutputPath, publication.ResultOutputRevision); err != nil || evidence.ContentRevision != publication.ResultOutputRevision {
		return errors.New("canonical Result content revision changed after Validation")
	}
	var run models.ValidationRun
	if err := h.db.Where("run_id = ? AND job_id = ?", publication.ValidationRunID, job.ID).First(&run).Error; err != nil || run.RunID != job.ValidationRunID || run.ExecutionPlanID != publication.ExecutionPlanID || run.ResultArtifactID != publication.ResultArtifactID || run.ResultGenerationToken != publication.ResultGenerationToken || run.ResultGenerationVersion != publication.ResultGenerationVersion || run.ResultOutputRevision != publication.ResultOutputRevision {
		return errors.New("ValidationRun no longer authorizes this exact Result and execution")
	}
	return nil
}

func (h PublisherHandler) verifyReplacementAuthority(job models.QueueJob, publication models.PublisherPublication) error {
	if publication.SourceAssetRecordID == nil || filepath.Clean(job.MediaPath) != filepath.Clean(publication.SourcePath) || !pathIsInside(job.MediaPath, publication.DestinationRoot) {
		return errors.New("replace-library publication lacks the frozen canonical source asset identity")
	}
	var record models.AssetRecord
	if err := h.db.First(&record, *publication.SourceAssetRecordID).Error; err != nil || filepath.Clean(record.Path) != filepath.Clean(job.MediaPath) || record.LibraryID != publication.LibraryID || record.Missing || !record.CreatedAt.Equal(publication.SourceAssetRecordCreatedAt) || !record.ModifiedAt.Equal(publication.SourceAssetRecordModifiedAt) || record.SizeBytes != publication.SourceAssetRecordSizeBytes {
		return errors.New("replace-library source no longer matches its canonical AssetRecord")
	}
	if !assetRecordFileSnapshotMatches(record.Path, publication) {
		return errors.New("replace-library file changed since its canonical AssetRecord was frozen")
	}
	if publication.DestinationPath != filepath.Clean(job.MediaPath) {
		if _, err := os.Lstat(publication.DestinationPath); err == nil {
			return errors.New("replace-library destination exists but is not the exact canonical source asset")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func assetRecordFileSnapshotMatches(path string, publication models.PublisherPublication) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != publication.SourceAssetRecordSizeBytes {
		return false
	}
	return info.ModTime().Equal(publication.SourceAssetRecordModifiedAt)
}

func isPublicationBlocked(err error) bool {
	var blocked interface{ PublicationBlocked() bool }
	return errors.As(err, &blocked) && blocked.PublicationBlocked()
}

type publicationBlockedError struct{ message string }

func (e publicationBlockedError) Error() string            { return e.message }
func (e publicationBlockedError) PublicationBlocked() bool { return true }

func ensureControlledPublisherRoot(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return errors.New("frozen Library destination root must be absolute")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("frozen Library destination root is not a controlled directory")
	}
	return nil
}

func checkPublisherCapacity(requiredBytes, availableBytes uint64) error {
	if requiredBytes > availableBytes {
		return fmt.Errorf("insufficient destination free space: need %d bytes, have %d bytes", requiredBytes, availableBytes)
	}
	return nil
}

func (h PublisherHandler) persistManifest(owner publisherLeaseOwner, entries []publicationManifestEntry) error {
	result := h.db.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state = ? AND active_attempt_id = ? AND fencing_generation = ? AND lease_expires_at > ?", owner.publicationID, PublicationPublishing, owner.attemptID, owner.fence, time.Now().UTC()).Updates(map[string]any{"manifest": manifestValue(entries), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("publisher attempt lost its fencing claim before manifest persistence")
	}
	return nil
}

func (h PublisherHandler) preflightPublication(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, entries *[]publicationManifestEntry) error {
	return h.preflightPublicationWithLease(job, publication, entries, func() error {
		return h.assertPublicationLease(owner, false)
	})
}

func (h PublisherHandler) preflightPublicationForEligibility(job models.QueueJob, publication models.PublisherPublication, entries *[]publicationManifestEntry) error {
	return h.preflightPublicationWithLease(job, publication, entries, nil)
}

func (h PublisherHandler) preflightPublicationWithLease(job models.QueueJob, publication models.PublisherPublication, entries *[]publicationManifestEntry, checkLease func() error) error {
	for i := range *entries {
		entry := &(*entries)[i]
		if !pathIsInside(entry.Final, publication.DestinationRoot) || !pathIsInside(entry.Temporary, publication.DestinationRoot) {
			return publicationBlockedError{message: "delivery manifest path escapes the frozen Library root"}
		}
		if !entry.Promote {
			continue
		}
		if err := ensureNoSymlinkPath(publication.DestinationRoot, filepath.Dir(entry.Final), false); err != nil {
			return publicationBlockedError{message: err.Error()}
		}
		if checkLease != nil {
			if err := checkLease(); err != nil {
				return err
			}
		}
		if entry.Backup != "" {
			if _, backupErr := os.Lstat(entry.Backup); backupErr == nil {
				if !previousTokenMatches(entry.Backup, *entry) {
					return publicationBlockedError{message: "replacement backup ownership is ambiguous"}
				}
			} else if !os.IsNotExist(backupErr) {
				return backupErr
			}
		}
		if tokenMatches(entry.Final, *entry) {
			continue
		}
		if _, err := os.Lstat(entry.Final); err == nil {
			if job.PublishMode == PublishModeReplaceLibrary && entry.Kind == "primary" && filepath.Clean(entry.Final) == filepath.Clean(job.MediaPath) && entry.Backup != "" {
				if _, backupErr := os.Lstat(entry.Backup); backupErr == nil {
					return publicationBlockedError{message: "replacement target and backup are both present"}
				}
				if !recordPreviousOwnership(entry.Final, entry) {
					return publicationBlockedError{message: "replacement target ownership identity is unavailable"}
				}
				continue
			}
			if !entry.Required {
				entry.Promote = false
				entry.Outcome = "skipped_existing_destination"
				continue
			}
			if tokenMatches(entry.Final, *entry) {
				continue
			}
			return publicationBlockedError{message: "destination already exists and is not owned by this publication"}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	var requiredBytes uint64
	for _, entry := range *entries {
		if !entry.Required || !entry.Promote {
			continue
		}
		if checkLease != nil {
			if err := checkLease(); err != nil {
				return err
			}
		}
		info, err := os.Lstat(entry.Source)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 {
			return publicationBlockedError{message: "required delivery source is not a regular controlled file"}
		}
		size := uint64(info.Size())
		if ^uint64(0)-requiredBytes < size {
			return errors.New("required delivery capacity estimate overflowed")
		}
		requiredBytes += size
	}
	if requiredBytes > 0 {
		const publisherFilesystemOverhead = uint64(1 << 20)
		if ^uint64(0)-requiredBytes < publisherFilesystemOverhead {
			return errors.New("required delivery capacity estimate overflowed")
		}
		requiredBytes += publisherFilesystemOverhead
	}
	availableBytes, err := publisherAvailableBytes(publication.DestinationRoot)
	if err != nil {
		return fmt.Errorf("read destination free space: %w", err)
	}
	if err := checkPublisherCapacity(requiredBytes, availableBytes); err != nil {
		return err
	}
	return nil
}

func ensureNoSymlinkPath(root, targetDir string, create bool) error {
	root = filepath.Clean(root)
	if create {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
	}
	rootInfo, rootErr := os.Lstat(root)
	if rootErr == nil {
		if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
			return errors.New("filesystem root is a symlink or is not a directory")
		}
	} else if !os.IsNotExist(rootErr) {
		return rootErr
	}
	if !pathIsInsideOrEqual(targetDir, root) {
		return errors.New("destination directory is outside the frozen Library root")
	}
	if create {
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return err
		}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return errors.New("destination directory escapes the frozen Library root")
	}
	current := rootAbs
	parts := strings.Split(relative, string(filepath.Separator))
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("destination path contains a symlink or non-directory component: %s", current)
		}
	}
	return nil
}

func pathIsInsideOrEqual(candidate, root string) bool {
	candidateAbs, candidateErr := filepath.Abs(candidate)
	rootAbs, rootErr := filepath.Abs(root)
	if candidateErr != nil || rootErr != nil {
		return false
	}
	candidateAbs, rootAbs = filepath.Clean(candidateAbs), filepath.Clean(rootAbs)
	return candidateAbs == rootAbs || pathIsInside(candidateAbs, rootAbs)
}

func archivePathOwnershipError(publication models.PublisherPublication, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != publication.OriginalArchiveSizeBytes {
		return errors.New("archive file size does not match persisted original ownership evidence")
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	if deviceErr != nil || inodeErr != nil {
		return errors.New("archive filesystem ownership identity is unavailable")
	}
	if (device == publication.OriginalArchiveDevice && inode == publication.OriginalArchiveInode && inode != 0) || (device == publication.OriginalArchiveTempDevice && inode == publication.OriginalArchiveTempInode && inode != 0) {
		return nil
	}
	return errors.New("archive path is not owned by the persisted original move")
}

func movePublicationArchive(db *gorm.DB, publication *models.PublisherPublication, source, destination string) error {
	if publication == nil || strings.TrimSpace(publication.OriginalArchiveTemporaryPath) == "" {
		return errors.New("original archive move has no persisted attempt path")
	}
	assertArchiveLease := func() error { return assertOriginalArchiveLease(db, *publication, true) }
	if err := assertArchiveLease(); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(filepath.Dir(destination), filepath.Dir(destination), true); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		if ownershipErr := archivePathOwnershipError(*publication, destination); ownershipErr != nil {
			return ownershipErr
		}
		if _, sourceErr := os.Lstat(source); sourceErr == nil {
			if !archiveSourceTokenMatches(source, *publication) {
				return errors.New("archive source identity changed during interrupted move recovery")
			}
			equal, compareErr := filesEqual(source, destination)
			if compareErr != nil || !equal {
				return errors.New("archive source and destination differ during interrupted move recovery")
			}
			if err := assertArchiveLease(); err != nil {
				return err
			}
			if err := os.Remove(source); err != nil {
				return err
			}
			if err := syncDirectory(filepath.Dir(source)); err != nil {
				return err
			}
		}
		return syncDirectory(filepath.Dir(destination))
	} else if !os.IsNotExist(err) {
		return err
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return errors.New("archive source must be a regular non-symlink file")
	}
	if !archiveSourceTokenMatches(source, *publication) {
		return errors.New("archive source identity changed before original move")
	}
	if err := assertArchiveLease(); err != nil {
		return err
	}
	if err := atomicRenameNoReplace(source, destination); err == nil {
		if err := syncDirectory(filepath.Dir(destination)); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(source))
	} else if os.IsExist(err) {
		return errors.New("archive destination became occupied")
	}

	temporary := publication.OriginalArchiveTemporaryPath
	if _, err := os.Lstat(temporary); err == nil {
		if publication.OriginalArchiveTempInode == 0 || !archiveTokenMatches(temporary, *publication) {
			return errors.New("archive staging path ownership is ambiguous")
		}
		if err := assertArchiveLease(); err != nil {
			return err
		}
		if err := os.Remove(temporary); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	if err := assertArchiveLease(); err != nil {
		_ = input.Close()
		return err
	}
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = input.Close()
		return err
	}
	buffer := make([]byte, 4*1024*1024)
	var copyErr error
	for {
		if copyErr = assertArchiveLease(); copyErr != nil {
			break
		}
		var read int
		var readErr error
		read, readErr = input.Read(buffer)
		if read > 0 {
			written := 0
			for written < read {
				var count int
				count, copyErr = output.Write(buffer[written:read])
				written += count
				if copyErr != nil {
					break
				}
			}
		}
		if copyErr != nil {
			break
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			copyErr = readErr
			break
		}
	}
	inputErr := input.Close()
	if copyErr == nil {
		copyErr = inputErr
	}
	if copyErr == nil {
		copyErr = output.Sync()
	}
	outputErr := output.Close()
	if copyErr == nil {
		copyErr = outputErr
	}
	if copyErr != nil {
		return copyErr
	}
	if err := syncDirectory(filepath.Dir(temporary)); err != nil {
		return err
	}
	tempInfo, err := os.Stat(temporary)
	if err != nil {
		return err
	}
	publication.OriginalArchiveTempDevice, _ = strconv.ParseUint(statIdentityField(tempInfo, "Dev"), 10, 64)
	publication.OriginalArchiveTempInode, _ = strconv.ParseUint(statIdentityField(tempInfo, "Ino"), 10, 64)
	if publication.OriginalArchiveTempInode == 0 {
		return errors.New("archive staging ownership identity is unavailable")
	}
	result := db.Model(publication).Where("publication_id = ? AND state = ? AND original_archive_status = ? AND original_archive_claim_id = ? AND original_archive_fencing_generation = ? AND original_archive_lease_expires_at > ?", publication.PublicationID, PublicationPublished, "archiving", publication.OriginalArchiveClaimID, publication.OriginalArchiveFencingGeneration, time.Now().UTC()).Updates(map[string]any{"original_archive_temp_device": publication.OriginalArchiveTempDevice, "original_archive_temp_inode": publication.OriginalArchiveTempInode, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("original archive claim expired before staging ownership persisted")
	}
	if err := assertArchiveLease(); err != nil {
		return err
	}
	if err := atomicRenameNoReplace(temporary, destination); err != nil {
		if !os.IsExist(err) || archivePathOwnershipError(*publication, destination) != nil {
			return err
		}
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := assertArchiveLease(); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(source))
}

func archiveTokenMatches(path string, publication models.PublisherPublication) bool {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != publication.OriginalArchiveSizeBytes {
		return false
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	return deviceErr == nil && inodeErr == nil && device == publication.OriginalArchiveTempDevice && inode == publication.OriginalArchiveTempInode && inode != 0
}

func archiveSourceTokenMatches(path string, publication models.PublisherPublication) bool {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != publication.OriginalArchiveSizeBytes {
		return false
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	return deviceErr == nil && inodeErr == nil && device == publication.OriginalArchiveDevice && inode == publication.OriginalArchiveInode && inode != 0
}

func claimOriginalArchive(db *gorm.DB, publication models.PublisherPublication) (models.PublisherPublication, bool, error) {
	claimID, err := publisherToken("archive-claim")
	if err != nil {
		return publication, false, err
	}
	now := time.Now().UTC()
	nextFence := publication.OriginalArchiveFencingGeneration + 1
	claimed := false
	err = db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.PublisherPublication{}).
			Where("publication_id = ? AND state = ? AND original_archive_fencing_generation = ?", publication.PublicationID, PublicationPublished, publication.OriginalArchiveFencingGeneration).
			Where("(original_archive_status IN ? OR (original_archive_status = ? AND original_archive_lease_expires_at IS NOT NULL AND original_archive_lease_expires_at <= ?))", []string{"pending", "failed"}, "archiving", now).
			Updates(map[string]any{"original_archive_status": "archiving", "original_archive_claim_id": claimID, "original_archive_lease_expires_at": now.Add(publisherArchiveLease), "original_archive_fencing_generation": nextFence, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		claimed = true
		var job models.QueueJob
		if err := tx.First(&job, publication.QueueJobID).Error; err != nil {
			return err
		}
		job.OriginalArchiveStatus = "archiving"
		if err := tx.Model(&job).Updates(map[string]any{"original_archive_status": "archiving", "original_archived_path": ""}).Error; err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStageArchivingOriginal); err != nil {
			return err
		}
		return tx.Where("publication_id = ?", publication.PublicationID).First(&publication).Error
	})
	if err != nil {
		return publication, false, err
	}
	if !claimed {
		return publication, false, nil
	}
	return publication, true, nil
}

func assertOriginalArchiveLease(db *gorm.DB, publication models.PublisherPublication, renew bool) error {
	now := time.Now().UTC()
	query := db.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state = ? AND original_archive_status = ? AND original_archive_claim_id = ? AND original_archive_fencing_generation = ? AND original_archive_lease_expires_at > ?", publication.PublicationID, PublicationPublished, "archiving", publication.OriginalArchiveClaimID, publication.OriginalArchiveFencingGeneration, now)
	if renew {
		result := query.Updates(map[string]any{"original_archive_lease_expires_at": now.Add(publisherArchiveLease), "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("original archive claim expired or was fenced by another recovery owner")
		}
		return nil
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("original archive claim expired or was fenced by another recovery owner")
	}
	return nil
}

func (h PublisherHandler) stagePublicationFile(owner publisherLeaseOwner, source, temporary string) error {
	if err := h.assertPublicationLease(owner, false); err != nil {
		return err
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return errors.New("publication source must be a regular non-symlink file")
	}
	if err := h.assertPublicationLease(owner, true); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(temporary), 0o700); err != nil {
		return err
	}
	if err := h.assertPublicationLease(owner, true); err != nil {
		return err
	}
	if err := os.Link(source, temporary); err == nil {
		if err := syncDirectory(filepath.Dir(temporary)); err != nil {
			return err
		}
		return h.assertPublicationLease(owner, true)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := h.assertPublicationLease(owner, true); err != nil {
		return err
	}
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	buffer := make([]byte, 4*1024*1024)
	for {
		if err := h.assertPublicationLease(owner, true); err != nil {
			_ = output.Close()
			return err
		}
		read, readErr := input.Read(buffer)
		if read > 0 {
			written := 0
			for written < read {
				count, writeErr := output.Write(buffer[written:read])
				written += count
				if writeErr != nil {
					_ = output.Close()
					return writeErr
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = output.Close()
			return readErr
		}
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(temporary)); err != nil {
		return err
	}
	return h.assertPublicationLease(owner, true)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func recordFileOwnership(path string, entry *publicationManifestEntry) error {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Size() <= 0 {
		if err == nil {
			err = errors.New("staged delivery artifact is empty or not a file")
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("staged delivery artifact is not a regular file")
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	if deviceErr != nil || inodeErr != nil || inode == 0 {
		return errors.New("filesystem does not provide stable staging ownership identity")
	}
	entry.SizeBytes, entry.Device, entry.Inode = info.Size(), device, inode
	entry.Outcome = "staged"
	return nil
}

func tokenMatches(path string, entry publicationManifestEntry) bool {
	if entry.Device == 0 || entry.Inode == 0 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != entry.SizeBytes {
		return false
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	return deviceErr == nil && inodeErr == nil && device == entry.Device && inode == entry.Inode
}

func recordPreviousOwnership(path string, entry *publicationManifestEntry) bool {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return false
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	if deviceErr != nil || inodeErr != nil || inode == 0 {
		return false
	}
	if entry.PreviousInode != 0 {
		return previousTokenMatches(path, *entry)
	}
	entry.PreviousSizeBytes, entry.PreviousDevice, entry.PreviousInode = info.Size(), device, inode
	return true
}

func previousTokenMatches(path string, entry publicationManifestEntry) bool {
	if entry.PreviousInode == 0 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != entry.PreviousSizeBytes {
		return false
	}
	device, deviceErr := strconv.ParseUint(statIdentityField(info, "Dev"), 10, 64)
	inode, inodeErr := strconv.ParseUint(statIdentityField(info, "Ino"), 10, 64)
	return deviceErr == nil && inodeErr == nil && device == entry.PreviousDevice && inode == entry.PreviousInode
}

func (h PublisherHandler) promotePublicationManifest(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, entries []publicationManifestEntry) error {
	for i := range entries {
		entry := &entries[i]
		if !entry.Promote {
			continue
		}
		if err := h.promotePublicationArtifact(job, publication, owner, entries, i); err != nil {
			if entry.Required {
				return err
			}
			if leaseErr := h.assertPublicationLease(owner, false); leaseErr != nil {
				return leaseErr
			}
			if tokenMatches(entry.Final, *entry) {
				entry.Outcome = "committed_optional"
			} else {
				if _, statErr := os.Lstat(entry.Temporary); statErr == nil {
					if !tokenMatches(entry.Temporary, *entry) {
						return errors.New("optional sidecar temporary path ownership is ambiguous")
					}
					if leaseErr := h.assertPublicationLease(owner, false); leaseErr != nil {
						return leaseErr
					}
					if removeErr := os.Remove(entry.Temporary); removeErr != nil {
						return removeErr
					}
					if syncErr := syncDirectory(filepath.Dir(entry.Temporary)); syncErr != nil {
						return syncErr
					}
				} else if !os.IsNotExist(statErr) {
					return statErr
				}
				entry.Promote = false
				entry.Outcome = "optional_promotion_failed: " + publisherBoundedText(err.Error(), 300)
			}
			if persistErr := h.persistManifest(owner, entries); persistErr != nil {
				return persistErr
			}
		}
	}
	return nil
}

func (h PublisherHandler) promotePublicationArtifact(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, entries []publicationManifestEntry, index int) error {
	entry := &entries[index]
	if err := h.assertPublicationLease(owner, true); err != nil {
		return err
	}
	if tokenMatches(entry.Final, *entry) {
		entry.Outcome = "committed"
		return nil
	}
	if err := ensureNoSymlinkPath(publication.DestinationRoot, filepath.Dir(entry.Final), true); err != nil {
		return err
	}
	if entry.Backup != "" {
		_, finalErr := os.Lstat(entry.Final)
		_, backupErr := os.Lstat(entry.Backup)
		if backupErr == nil {
			if !previousTokenMatches(entry.Backup, *entry) || !os.IsNotExist(finalErr) {
				return errors.New("replacement backup or target ownership is ambiguous")
			}
		} else if !os.IsNotExist(backupErr) {
			return backupErr
		} else if finalErr == nil {
			if !recordPreviousOwnership(entry.Final, entry) {
				return errors.New("replacement target ownership is unavailable")
			}
			if err := h.persistManifest(owner, entries); err != nil {
				return err
			}
			if err := h.assertPublicationLease(owner, false); err != nil {
				return err
			}
			if err := atomicRenameNoReplace(entry.Final, entry.Backup); err != nil {
				return err
			}
			if !previousTokenMatches(entry.Backup, *entry) {
				return errors.New("replacement backup did not preserve original ownership")
			}
			if err := syncDirectory(filepath.Dir(entry.Final)); err != nil {
				return err
			}
			if err := syncDirectory(filepath.Dir(entry.Backup)); err != nil {
				return err
			}
		} else if !os.IsNotExist(finalErr) {
			return finalErr
		} else {
			return errors.New("replacement target and owned backup are both missing")
		}
	}
	if !tokenMatches(entry.Temporary, *entry) {
		return errors.New("staged file is missing or no longer owned by this publication attempt")
	}
	if _, err := os.Lstat(entry.Final); err == nil {
		return errors.New("final path was occupied after destination preflight")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := h.assertPublicationLease(owner, false); err != nil {
		return err
	}
	if err := atomicRenameNoReplace(entry.Temporary, entry.Final); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(entry.Final)); err != nil {
		return err
	}
	if !tokenMatches(entry.Final, *entry) {
		return errors.New("final rename did not preserve staged artifact ownership")
	}
	entry.Outcome = "committed"
	if err := h.persistManifest(owner, entries); err != nil {
		return err
	}
	return nil
}

func (h PublisherHandler) commitPublishedPackage(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, entries []publicationManifestEntry) error {
	for _, entry := range entries {
		if entry.Required && entry.Promote && (!tokenMatches(entry.Final, entry) || entry.Outcome != "committed") {
			return fmt.Errorf("required artifact is not committed: %s", entry.Final)
		}
	}
	if err := h.assertPublicationLease(owner, false); err != nil {
		return err
	}
	if err := h.verifyBoundDelivery(job, publication, owner.actorID == publicationActorSystem); err != nil {
		return err
	}
	var library models.Library
	if err := h.db.First(&library, publication.LibraryID).Error; err != nil {
		return err
	}
	// Use the Queue-frozen root for AssetRecord provenance, never the current
	// Library destination setting.
	library.DestinationPath = publication.DestinationRoot
	now := time.Now().UTC()
	primaryInfo, err := os.Stat(publication.DestinationPath)
	if err != nil || primaryInfo.IsDir() {
		return errors.New("committed primary media is missing")
	}
	job.PublishedPath, job.PublishedAt = publication.DestinationPath, &now
	job.PublishedSizeBytes = primaryInfo.Size()
	job.PublicationID, job.PublicationState = publication.PublicationID, PublicationPublished
	job.PublicationSummary = "Primary media, required sidecars, and provenance committed."
	if err := h.assertPublicationLease(owner, true); err != nil {
		return err
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var current models.PublisherPublication
		if err := tx.Where("publication_id = ? AND state = ? AND active_attempt_id = ? AND fencing_generation = ? AND lease_expires_at > ?", owner.publicationID, PublicationPublishing, owner.attemptID, owner.fence, now).First(&current).Error; err != nil {
			return err
		}
		if err := syncPublishedAssetRecordsTx(tx, job, library, now); err != nil {
			return err
		}
		result := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state = ? AND active_attempt_id = ? AND fencing_generation = ? AND lease_expires_at > ?", owner.publicationID, PublicationPublishing, owner.attemptID, owner.fence, now).Updates(map[string]any{"state": PublicationPublished, "published_at": now, "active_attempt_id": "", "lease_expires_at": nil, "manifest": manifestValue(entries), "failure_code": "", "failure_summary": "", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Publisher attempt lost its fencing claim before provenance commit")
		}
		evidenceID, err := publisherToken("attempt-evidence")
		if err != nil {
			return err
		}
		var evidenceCount int64
		if err := tx.Model(&models.PublisherAttemptEvidence{}).Where("attempt_id = ?", owner.attemptID).Count(&evidenceCount).Error; err != nil {
			return err
		}
		if evidenceCount < 20 {
			if err := tx.Create(&models.PublisherAttemptEvidence{EvidenceID: evidenceID, PublicationID: owner.publicationID, AttemptID: owner.attemptID, FencingGeneration: owner.fence, Version: int(evidenceCount + 1), Phase: "commit", Outcome: "published", Summary: "Required files and publication provenance committed.", CreatedAt: now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"published_path":          job.PublishedPath,
			"published_path_key":      models.CanonicalPublicationPathKey(job.PublishedPath),
			"published_at":            now,
			"published_size_bytes":    job.PublishedSizeBytes,
			"publication_id":          publication.PublicationID,
			"publication_state":       PublicationPublished,
			"publication_summary":     job.PublicationSummary,
			"original_archive_status": "pending",
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func syncPublishedAssetRecordsTx(tx *gorm.DB, job models.QueueJob, library models.Library, now time.Time) error {
	published, err := assetRecordForPublishedPath(job.PublishedPath, library.DestinationPath, "converted", library, now, 0)
	if err != nil {
		return fmt.Errorf("index published asset: %w", err)
	}
	if err := upsertAssetRecord(tx, published); err != nil {
		return err
	}
	if strings.TrimSpace(job.OriginalArchivedPath) == "" {
		return nil
	}
	archiveRoot, err := originalsArchivePath(tx)
	if err != nil {
		return fmt.Errorf("resolve original archive root: %w", err)
	}
	archiveLibrary := models.Library{Name: "Originals"}
	archived, err := assetRecordForPublishedPath(job.OriginalArchivedPath, archiveRoot, "archive", archiveLibrary, now, originalRetentionDays(tx))
	if err != nil {
		return fmt.Errorf("index archived original: %w", err)
	}
	return upsertAssetRecord(tx, archived)
}

func (h PublisherHandler) blockPublicationAttempt(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, code, summary string) (PublishResult, error) {
	_ = h.appendAttemptEvidence(owner, "handoff", "blocked", code, summary)
	if err := h.finishAttempt(owner, PublicationBlocked, code, summary); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{}, publishError{Status: http.StatusConflict, Message: summary}
}

func (h PublisherHandler) failPublicationAttempt(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, code string, cause error) (PublishResult, error) {
	if err := h.assertPublicationLease(owner, false); err != nil {
		return PublishResult{}, err
	}
	entries, _ := manifestEntries(publication.Manifest)
	ambiguous := false
	for i := range entries {
		if entries[i].Temporary == "" {
			continue
		}
		if _, err := os.Lstat(entries[i].Temporary); os.IsNotExist(err) {
			continue
		} else if err != nil {
			ambiguous = true
			continue
		}
		if !tokenMatches(entries[i].Temporary, entries[i]) {
			ambiguous = true
			continue
		}
		if err := h.assertPublicationLease(owner, false); err != nil {
			return PublishResult{}, err
		}
		if err := os.Remove(entries[i].Temporary); err != nil {
			ambiguous = true
		} else {
			_ = syncDirectory(filepath.Dir(entries[i].Temporary))
		}
	}
	message := "Publisher could not materialize the frozen delivery package: " + cause.Error()
	state, outcome := PublicationFailed, "failed"
	if ambiguous {
		state, outcome, code = PublicationBlocked, "blocked", "ambiguous_ownership"
		message = "Publisher found a temporary path whose ownership cannot be proven; manual reconciliation is required."
	}
	_ = h.appendAttemptEvidence(owner, "delivery", outcome, code, message)
	if err := h.finishAttempt(owner, state, code, message); err != nil {
		return PublishResult{}, err
	}
	if state == PublicationBlocked {
		return PublishResult{}, publishError{Status: http.StatusConflict, Message: message, Err: cause}
	}
	return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: message, Err: cause}
}

func (h PublisherHandler) rollbackOrBlock(job models.QueueJob, publication models.PublisherPublication, owner publisherLeaseOwner, entries []publicationManifestEntry, code, message string) (PublishResult, error) {
	if err := h.assertPublicationLease(owner, false); err != nil {
		return PublishResult{}, err
	}
	ambiguous := false
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if tokenMatches(entry.Final, entry) {
			if err := h.assertPublicationLease(owner, false); err != nil {
				return PublishResult{}, err
			}
			if err := os.Remove(entry.Final); err != nil && !os.IsNotExist(err) {
				ambiguous = true
			}
			_ = syncDirectory(filepath.Dir(entry.Final))
		} else if _, err := os.Lstat(entry.Final); err == nil && entry.Outcome == "committed" {
			ambiguous = true
		}
		if entry.Backup != "" {
			if _, err := os.Lstat(entry.Backup); err == nil {
				if !previousTokenMatches(entry.Backup, entry) {
					ambiguous = true
				} else if _, targetErr := os.Lstat(entry.Final); os.IsNotExist(targetErr) {
					if err := h.assertPublicationLease(owner, false); err != nil {
						return PublishResult{}, err
					}
					if err := atomicRenameNoReplace(entry.Backup, entry.Final); err != nil {
						ambiguous = true
					} else {
						_ = syncDirectory(filepath.Dir(entry.Final))
					}
				} else {
					ambiguous = true
				}
			} else if !os.IsNotExist(err) {
				ambiguous = true
			}
		}
		if tokenMatches(entry.Temporary, entry) {
			if err := h.assertPublicationLease(owner, false); err != nil {
				return PublishResult{}, err
			}
			_ = os.Remove(entry.Temporary)
		} else if entry.Temporary != "" {
			if _, err := os.Lstat(entry.Temporary); err == nil {
				ambiguous = true
			} else if !os.IsNotExist(err) {
				ambiguous = true
			}
		}
	}
	if ambiguous {
		message = "Publication rollback found a path whose ownership is ambiguous; manual reconciliation is required."
		code = "ambiguous_ownership"
		_ = h.appendAttemptEvidence(owner, "rollback", "blocked", code, message)
		if err := h.finishAttempt(owner, PublicationBlocked, code, message); err != nil {
			return PublishResult{}, err
		}
		return PublishResult{}, publishError{Status: http.StatusConflict, Message: message}
	}
	_ = h.persistManifest(owner, entries)
	_ = h.appendAttemptEvidence(owner, "rollback", "complete", code, message)
	if err := h.finishAttempt(owner, PublicationFailed, code, message); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{}, publishError{Status: http.StatusInternalServerError, Message: message}
}

func (h PublisherHandler) finishOriginalArchive(job models.QueueJob, publication models.PublisherPublication, entries []publicationManifestEntry) {
	originalArchiveMu.Lock()
	defer originalArchiveMu.Unlock()
	var current models.PublisherPublication
	if h.db.Where("publication_id = ?", publication.PublicationID).First(&current).Error != nil || current.State != PublicationPublished {
		return
	}
	if current.OriginalArchiveStatus == "complete" || current.OriginalArchiveStatus == "not_applicable" {
		return
	}
	claimed, ok, claimErr := claimOriginalArchive(h.db, current)
	if claimErr != nil || !ok {
		return
	}
	current = claimed

	var archiveSource string
	archiveWasPlanned := strings.TrimSpace(current.OriginalArchiveSourcePath) != ""
	if archiveWasPlanned {
		archiveSource = current.OriginalArchiveSourcePath
	}
	if job.PublishMode == PublishModeReplaceLibrary {
		if archiveSource == "" {
			for _, entry := range entries {
				if entry.Kind == "primary" && entry.Backup != "" {
					if _, err := os.Lstat(entry.Backup); err == nil {
						archiveSource = entry.Backup
					}
					break
				}
			}
		}
	}
	if archiveSource == "" {
		archiveSource = job.MediaPath
	}
	if strings.TrimSpace(archiveSource) == "" {
		h.recordArchiveOutcome(job, current, "not_applicable", "", "", "No original source was associated with this publication.")
		return
	}
	if _, err := os.Lstat(archiveSource); os.IsNotExist(err) {
		if current.OriginalArchivedPath != "" {
			if archiveErr := archivePathOwnershipError(current, current.OriginalArchivedPath); archiveErr == nil {
				h.recordArchiveOutcome(job, current, "complete", current.OriginalArchivedPath, archiveSource, "Original archive move was recovered after restart.")
				return
			}
		}
		if archiveWasPlanned {
			h.recordArchiveOutcome(job, current, "failed", current.OriginalArchivedPath, archiveSource, "planned archive source and destination are both missing; manual reconciliation is required")
			return
		}
		h.recordArchiveOutcome(job, current, "not_applicable", "", archiveSource, "Original source was already absent after publication.")
		return
	} else if err != nil {
		h.recordArchiveOutcome(job, current, "failed", current.OriginalArchivedPath, archiveSource, err.Error())
		return
	}

	var library models.Library
	if err := h.db.First(&library, current.LibraryID).Error; err != nil {
		h.recordArchiveOutcome(job, current, "failed", current.OriginalArchivedPath, archiveSource, err.Error())
		return
	}
	library.DestinationPath = current.DestinationRoot
	var destination string
	var archiveRoot string
	var err error
	if job.PublishMode == PublishModeReplaceLibrary {
		destination, err = h.libraryOriginalArchivePath(job, library)
		if err == nil {
			archiveRoot, err = originalsArchivePath(h.db)
		}
	} else {
		var rawRoot string
		rawRoot, err = settingPath(h.db, "rawRoot", "/media/raw")
		if err == nil {
			archiveRoot, err = originalsArchivePath(h.db)
		}
		if err == nil {
			relative := relativeMediaPath(archiveSource, rawRoot)
			if relative == "" || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				err = errors.New("original source is outside the configured RAW archive root")
			} else {
				destination = uniqueArchivePath(filepath.Join(archiveRoot, filepath.FromSlash(relative)))
			}
		}
	}
	if err != nil {
		h.recordArchiveOutcome(job, current, "failed", current.OriginalArchivedPath, archiveSource, err.Error())
		return
	}
	if current.OriginalArchivedPath != "" && filepath.Clean(current.OriginalArchiveSourcePath) == filepath.Clean(archiveSource) {
		destination = current.OriginalArchivedPath
	} else {
		if _, err := os.Lstat(destination); err == nil {
			h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, "archive destination became occupied before the move")
			return
		} else if !os.IsNotExist(err) {
			h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, err.Error())
			return
		}
	}
	if err := assertOriginalArchiveLease(h.db, current, false); err != nil {
		return
	}
	if err := ensureNoSymlinkPath(archiveRoot, filepath.Dir(destination), true); err != nil {
		h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, err.Error())
		return
	}
	sourceInfo, err := os.Lstat(archiveSource)
	if err != nil || sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		if err == nil {
			err = errors.New("original archive source must be a regular non-symlink file")
		}
		h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, err.Error())
		return
	}
	if current.OriginalArchiveSourcePath == "" || filepath.Clean(current.OriginalArchiveSourcePath) != filepath.Clean(archiveSource) {
		current.OriginalArchiveSizeBytes = sourceInfo.Size()
		current.OriginalArchiveDevice, _ = strconv.ParseUint(statIdentityField(sourceInfo, "Dev"), 10, 64)
		current.OriginalArchiveInode, _ = strconv.ParseUint(statIdentityField(sourceInfo, "Ino"), 10, 64)
		current.OriginalArchiveTemporaryPath = destination + ".mvforge-" + publication.PublicationID + ".part"
		current.OriginalArchiveTempDevice, current.OriginalArchiveTempInode = 0, 0
	}
	if current.OriginalArchiveInode == 0 {
		h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, "original file identity is unavailable; archive ownership cannot be established")
		return
	}
	result := h.db.Model(&current).Where("state = ? AND original_archive_status = ? AND original_archive_claim_id = ? AND original_archive_fencing_generation = ? AND original_archive_lease_expires_at > ?", PublicationPublished, "archiving", current.OriginalArchiveClaimID, current.OriginalArchiveFencingGeneration, time.Now().UTC()).Updates(map[string]any{"original_archived_path": destination, "original_archive_source_path": archiveSource, "original_archive_temporary_path": current.OriginalArchiveTemporaryPath, "original_archive_size_bytes": current.OriginalArchiveSizeBytes, "original_archive_device": current.OriginalArchiveDevice, "original_archive_inode": current.OriginalArchiveInode, "original_archive_temp_device": current.OriginalArchiveTempDevice, "original_archive_temp_inode": current.OriginalArchiveTempInode, "original_archive_error": "", "updated_at": time.Now().UTC()})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error == nil {
			result.Error = errors.New("original archive claim expired before move intent persistence")
		}
		h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, result.Error.Error())
		return
	}
	if err := movePublicationArchive(h.db, &current, archiveSource, destination); err != nil {
		h.recordArchiveOutcome(job, current, "failed", destination, archiveSource, err.Error())
		return
	}
	h.recordArchiveOutcome(job, current, "complete", destination, archiveSource, "Original archived using the existing #174 AssetRecord contract.")
}

func (h PublisherHandler) recordArchiveOutcome(job models.QueueJob, publication models.PublisherPublication, status, destination, source, message string) {
	now := time.Now().UTC()
	if err := h.db.First(&job, publication.QueueJobID).Error; err != nil {
		return
	}
	if status == "complete" {
		job.OriginalArchivedPath = destination
	} else {
		job.OriginalArchivedPath = ""
	}
	job.OriginalArchiveStatus = status
	job.PublicationID = publication.PublicationID
	job.PublicationState = PublicationPublished
	job.PublicationSummary = "Published; original archive " + status + "."
	if status == "failed" {
		job.PublicationSummary = "Published; original archive failed: " + publisherBoundedText(message, 500)
		job.Notes = appendNote(job.Notes, "Original archive failed after publication: "+publisherBoundedText(message, 500))
	}
	var library models.Library
	_ = h.db.First(&library, publication.LibraryID).Error
	library.DestinationPath = publication.DestinationRoot
	archiveRecordErr := h.db.Transaction(func(tx *gorm.DB) error {
		if status == "complete" {
			if err := syncPublishedAssetRecordsTx(tx, job, library, now); err != nil {
				return err
			}
			if sourcePath := filepath.Clean(strings.TrimSpace(job.MediaPath)); sourcePath != "." && sourcePath != filepath.Clean(job.PublishedPath) {
				if err := tx.Model(&models.AssetRecord{}).Where("path = ?", sourcePath).Updates(map[string]any{"missing": true, "synced_at": now}).Error; err != nil {
					return err
				}
			}
		}
		updates := map[string]any{
			"original_archive_source_path": source,
			"original_archive_status":      status, "original_archive_claim_id": "",
			"original_archive_lease_expires_at": nil, "original_archive_error": "", "updated_at": now,
		}
		if status == "complete" {
			updates["original_archived_path"] = destination
		}
		if status == "failed" {
			updates["original_archive_error"] = publisherBoundedText(message, 1000)
		}
		if status == "complete" || status == "not_applicable" {
			updates["original_archive_temporary_path"] = ""
			updates["original_archive_temp_device"] = 0
			updates["original_archive_temp_inode"] = 0
		}
		publicationResult := tx.Model(&models.PublisherPublication{}).Where("publication_id = ? AND state = ? AND original_archive_status = ? AND original_archive_claim_id = ? AND original_archive_fencing_generation = ? AND original_archive_lease_expires_at > ?", publication.PublicationID, PublicationPublished, "archiving", publication.OriginalArchiveClaimID, publication.OriginalArchiveFencingGeneration, now).Updates(updates)
		if publicationResult.Error != nil {
			return publicationResult.Error
		}
		if publicationResult.RowsAffected != 1 {
			return errors.New("original archive claim expired before outcome persistence")
		}
		var queueJob models.QueueJob
		if err := tx.First(&queueJob, job.ID).Error; err != nil {
			return err
		}
		queueUpdates := map[string]any{"original_archived_path": job.OriginalArchivedPath, "original_archive_status": status, "publication_state": PublicationPublished, "publication_summary": job.PublicationSummary, "notes": job.Notes}
		if err := tx.Model(&queueJob).Updates(queueUpdates).Error; err != nil {
			return err
		}
		if err := transitionJobStage(tx, &queueJob, JobStageCompleted); err != nil {
			return err
		}
		return ensureAssetPublishedOutbox(tx, publication, destination, status, now)
	})
	if archiveRecordErr != nil {
		appendSystemLog(h.db, "publisher_archive_outcome_persistence_failed", map[string]string{"publicationId": publication.PublicationID, "archiveStatus": status}, archiveRecordErr)
	}
}

func ensureAssetPublishedOutbox(tx *gorm.DB, publication models.PublisherPublication, archivePath, archiveStatus string, now time.Time) error {
	var existing models.AssetPublishedOutbox
	if err := tx.Where("publication_id = ?", publication.PublicationID).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&models.AssetPublishedOutbox{
		EventID: "asset-published-" + publication.PublicationID, PublicationID: publication.PublicationID,
		EventType: "AssetPublished", Payload: models.JSONMap{
			"publicationId": publication.PublicationID, "queueJobId": publication.QueueJobID,
			"libraryId": publication.LibraryID, "destinationPath": publication.DestinationPath,
			"resultArtifactId": publication.ResultArtifactID, "resultGenerationToken": publication.ResultGenerationToken,
			"resultGenerationVersion": publication.ResultGenerationVersion, "validationRunId": publication.ValidationRunID,
			"validationOverrideEventIds": publication.ValidationOverrideEventIDs,
			"sourceAssetRecordId":        publication.SourceAssetRecordID, "archiveStatus": archiveStatus, "originalArchivedPath": archivePath,
		}, Status: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error
}

func (p *AutoPublisher) recoverPublisherPublications() error {
	var recoveryErrors []error
	now := time.Now().UTC()
	var expired []models.PublisherPublication
	if err := p.db.Where("state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?", PublicationPublishing, now).Order("updated_at asc").Limit(25).Find(&expired).Error; err != nil {
		return fmt.Errorf("load expired Publisher attempts: %w", err)
	}
	for _, publication := range expired {
		claimed, owner, err := (PublisherHandler{db: p.db}).claimPublication(publication.PublicationID, publicationActorSystem, publicationActorSystem, true)
		if err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("reclaim publication %s: %w", publication.PublicationID, err))
			continue
		}
		if !claimed {
			continue
		}
		var job models.QueueJob
		if err := p.db.First(&job, publication.QueueJobID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_, blockErr := (PublisherHandler{db: p.db}).blockPublicationAttempt(job, publication, owner, "queue_job_missing", "Persisted Queue execution is missing; publication is blocked for review.")
				if blockErr != nil {
					recoveryErrors = append(recoveryErrors, fmt.Errorf("block publication %s for missing Queue job: %w", publication.PublicationID, blockErr))
				}
			} else {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("load Queue job %d for publication %s: %w", publication.QueueJobID, publication.PublicationID, err))
			}
			continue
		}
		if _, err := (PublisherHandler{db: p.db}).runPublicationAttempt(job, publication, owner); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("resume publication %s: %w", publication.PublicationID, err))
		}
	}
	var published []models.PublisherPublication
	if err := p.db.Where("state = ? AND (original_archive_status IN ? OR (original_archive_status = ? AND original_archive_lease_expires_at IS NOT NULL AND original_archive_lease_expires_at <= ?))", PublicationPublished, []string{"pending", "failed"}, "archiving", now).Order("updated_at asc").Limit(25).Find(&published).Error; err != nil {
		return errors.Join(append(recoveryErrors, fmt.Errorf("load published original archives: %w", err))...)
	}
	for _, publication := range published {
		var job models.QueueJob
		if err := p.db.First(&job, publication.QueueJobID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("load Queue job %d for published archive: %w", publication.QueueJobID, err))
			}
			continue
		}
		entries, _ := manifestEntries(publication.Manifest)
		(PublisherHandler{db: p.db}).finishOriginalArchive(job, publication, entries)
	}
	return errors.Join(recoveryErrors...)
}
