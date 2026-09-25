package handlers

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	AssetLocationRaw     = "raw"
	AssetLocationLibrary = "library"

	AssetLifecycleAvailable   = "available"
	AssetLifecycleQueued      = "queued"
	AssetLifecyclePreparing   = "preparing"
	AssetLifecycleProcessing  = "processing"
	AssetLifecycleValidating  = "validating"
	AssetLifecyclePublishing  = "publishing"
	AssetLifecyclePublished   = "published"
	AssetLifecycleFailed      = "failed"
	AssetLifecycleNeedsReview = "needs_review"

	AssetProvenanceConverted       = "converted"
	AssetProvenancePublishedAsIs   = "published_as_is"
	AssetProvenanceImported        = "imported_or_accepted"
	AssetProvenanceLegacyOrUnknown = "legacy_or_unknown"

	AssetValidationUnavailable        = "unavailable"
	AssetValidationPending            = "pending"
	AssetValidationPassed             = "passed"
	AssetValidationPassedWithWarnings = "passed_with_warnings"
	AssetValidationReviewRequired     = "review_required"
	AssetValidationFailed             = "failed"

	AssetPublicationUnpublished = "unpublished"
	AssetPublicationPending     = "pending"
	AssetPublicationPublished   = "published"
	AssetPublicationBlocked     = "blocked"
	AssetPublicationFailed      = "failed"

	AssetOriginalAvailable         = "available"
	AssetOriginalArchivePending    = "archive_pending"
	AssetOriginalArchived          = "archived"
	AssetOriginalRestorePending    = "restore_pending"
	AssetOriginalRestored          = "restored"
	AssetOriginalDeletionScheduled = "deletion_scheduled"
	AssetOriginalDeleted           = "deleted_by_retention"
	AssetOriginalArchiveFailed     = "archive_failed"
	AssetOriginalRestoreFailed     = "restore_failed"
	AssetOriginalUnknown           = "unknown"

	assetPublicationAuthorityQueue     = "queue_job"
	assetPublicationAuthorityDirect    = "direct_publication"
	assetCurrentPublicationPublished   = "published"
	assetCurrentPublicationUnpublished = "unpublished"
	assetCurrentPublicationConflict    = "unresolved_conflict"

	assetCorrelationResolved   = "resolved"
	assetCorrelationUnresolved = "unresolved"

	assetActionReasonUnsupported         = "unsupported"
	assetActionReasonIdentityUnresolved  = "identity_unresolved"
	assetActionReasonActiveWork          = "active_work"
	assetActionReasonMissing             = "missing"
	assetActionReasonUnsafeRoot          = "unsafe_root"
	assetActionReasonUnknownProvenance   = "unknown_provenance"
	assetActionReasonPublicationConflict = "publication_conflict"
	assetActionReasonRequiresReview      = "needs_review"
	assetActionReasonNotRaw              = "not_raw"
	assetActionReasonNotLibrary          = "not_library"
	assetActionReasonNoOriginal          = "original_unavailable"
	assetActionReasonAlreadyArchived     = "already_archived"
)

// AssetLifecycleProjection is a read model over existing AssetRecord,
// QueueJob, DirectPublication, maintenance, and evidence records. It does not
// persist transitions or replace any of those authorities.
type AssetLifecycleProjection struct {
	Location              string                        `json:"location"`
	Lifecycle             string                        `json:"lifecycle"`
	Provenance            string                        `json:"provenance"`
	Validation            string                        `json:"validation"`
	Publication           string                        `json:"publication"`
	Original              string                        `json:"original"`
	ActiveOperation       *AssetActiveOperation         `json:"activeOperation,omitempty"`
	Attention             []string                      `json:"attention,omitempty"`
	CurrentPublication    AssetCurrentPublication       `json:"currentPublication"`
	SourceProvenance      []AssetSourceProvenance       `json:"sourceProvenance,omitempty"`
	CurrentRawCorrelation AssetCurrentRawCorrelation    `json:"currentRawCorrelation"`
	EvidenceSummary       AssetLifecycleEvidenceSummary `json:"evidenceSummary"`
	Actions               AssetActionCapabilities       `json:"actions"`
}

type AssetActiveOperation struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Stage  string `json:"stage,omitempty"`
}

type AssetCurrentPublication struct {
	State               string     `json:"state"`
	Authority           string     `json:"authority,omitempty"`
	ID                  uint       `json:"id,omitempty"`
	LibraryAssetID      uint       `json:"libraryAssetId,omitempty"`
	PublishedPath       string     `json:"publishedPath,omitempty"`
	PublishedAt         *time.Time `json:"publishedAt,omitempty"`
	ConflictAuthorities []string   `json:"conflictAuthorities,omitempty"`
}

type AssetSourceProvenance struct {
	Authority     string     `json:"authority"`
	RecordID      uint       `json:"recordId"`
	SourcePath    string     `json:"sourcePath"`
	PublishedPath string     `json:"publishedPath,omitempty"`
	SourceAssetID *uint      `json:"sourceAssetRecordId,omitempty"`
	Current       bool       `json:"current"`
	PublishedAt   *time.Time `json:"publishedAt,omitempty"`
}

type AssetCurrentRawCorrelation struct {
	State            string `json:"state"`
	RawAssetRecordID uint   `json:"rawAssetRecordId,omitempty"`
	LibraryAssetID   uint   `json:"libraryAssetId,omitempty"`
}

type AssetLifecycleEvidenceSummary struct {
	QueueJobIDs          []uint `json:"queueJobIds,omitempty"`
	DirectPublicationIDs []uint `json:"directPublicationIds,omitempty"`
	HasAnalysis          bool   `json:"hasAnalysis"`
	OriginalArchivedPath string `json:"originalArchivedPath,omitempty"`
}

type AssetActionCapability struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type AssetActionCapabilities struct {
	Rename           AssetActionCapability `json:"rename"`
	Move             AssetActionCapability `json:"move"`
	Queue            AssetActionCapability `json:"queue"`
	Reconvert        AssetActionCapability `json:"reconvert"`
	LAB              AssetActionCapability `json:"lab"`
	AnalysisRefresh  AssetActionCapability `json:"refreshAnalysis"`
	Tracks           AssetActionCapability `json:"manageTracks"`
	Subtitles        AssetActionCapability `json:"manageSubtitles"`
	Attachments      AssetActionCapability `json:"manageAttachments"`
	Cancel           AssetActionCapability `json:"cancelWork"`
	ValidationReview AssetActionCapability `json:"reviewValidation"`
	ValidationRetry  AssetActionCapability `json:"retryValidation"`
	Publish          AssetActionCapability `json:"publish"`
	PublicationRetry AssetActionCapability `json:"retryPublication"`
	Archive          AssetActionCapability `json:"archive"`
	Restore          AssetActionCapability `json:"restore"`
	DeleteOriginal   AssetActionCapability `json:"deleteArchivedOriginal"`
	Reports          AssetActionCapability `json:"viewReports"`
	History          AssetActionCapability `json:"viewHistory"`
}

// AssetActionGuardContext is the canonical state already checked by operation
// guards. Capabilities render this state; they do not authorize a mutation.
type AssetActionGuardContext struct {
	IdentityKnown         bool
	Location              string
	Missing               bool
	HasActiveQueue        bool
	HasActiveMaintenance  bool
	RootSafe              bool
	ProvenanceKnown       bool
	PublicationConflict   bool
	ReviewRequired        bool
	QueueEligible         bool
	HasCurrentPublication bool
	OriginalAvailable     bool
	OriginalArchived      bool
}

// AssetLifecycleProjectionInput contains one asset plus the batch-loaded
// canonical records needed to resolve it. Callers should load these slices in
// batches; the resolver itself performs no database, filesystem, or media work.
type AssetLifecycleProjectionInput struct {
	Record             models.AssetRecord
	Records            []models.AssetRecord
	Libraries          []models.Library
	QueueJobs          []models.QueueJob
	DirectPublications []models.DirectPublication
	Maintenance        []models.AssetMaintenanceOperation
	ScanResults        []models.ScanResult
	Review             AssetReviewState
	ArchiveRoot        string
}

type assetPublicationEvidence struct {
	authority       string
	id              uint
	sourcePath      string
	publishedPath   string
	sourceAssetID   *uint
	libraryID       uint
	publishedAt     time.Time
	current         bool
	originalArchive string
	job             *models.QueueJob
	direct          *models.DirectPublication
}

// ResolveAssetLifecycleProjection resolves independent lifecycle dimensions
// from the existing canonical records. A path match can identify a current
// Library destination, but it cannot bind a current RAW AssetRecord without a
// durable source identity reference.
func ResolveAssetLifecycleProjection(input AssetLifecycleProjectionInput) AssetLifecycleProjection {
	record := input.Record
	records := input.Records
	if len(records) == 0 {
		records = []models.AssetRecord{record}
	}

	libraries := make(map[uint]models.Library, len(input.Libraries))
	for _, library := range input.Libraries {
		libraries[library.ID] = library
	}
	recordsByID := make(map[uint]models.AssetRecord, len(records))
	for _, candidate := range records {
		recordsByID[candidate.ID] = candidate
	}

	location := assetLifecycleLocation(record, libraries)
	evidence := collectAssetPublicationEvidence(record, input, libraries)
	currentQueue := selectCurrentPublication(evidence, assetPublicationAuthorityQueue)
	currentDirect := selectCurrentPublication(evidence, assetPublicationAuthorityDirect)

	projection := AssetLifecycleProjection{
		Location:    location,
		Provenance:  AssetProvenanceLegacyOrUnknown,
		Validation:  AssetValidationUnavailable,
		Publication: AssetPublicationUnpublished,
		Original:    AssetOriginalUnknown,
		CurrentPublication: AssetCurrentPublication{
			State:          assetCurrentPublicationUnpublished,
			LibraryAssetID: assetLifecycleLibraryAssetID(record, recordsByID, libraries, nil),
		},
		CurrentRawCorrelation: AssetCurrentRawCorrelation{State: assetCorrelationUnresolved},
		EvidenceSummary: AssetLifecycleEvidenceSummary{
			QueueJobIDs:          uniqueSortedQueueIDs(evidence),
			DirectPublicationIDs: uniqueSortedDirectIDs(evidence),
			HasAnalysis:          hasScanForRecord(record, input.ScanResults),
		},
	}

	if currentQueue != nil && currentDirect != nil {
		projection.CurrentPublication.State = assetCurrentPublicationConflict
		projection.CurrentPublication.ConflictAuthorities = []string{
			assetPublicationAuthorityQueue,
			assetPublicationAuthorityDirect,
		}
		projection.Provenance = AssetProvenanceLegacyOrUnknown
		projection.Publication = AssetPublicationBlocked
		appendAssetAttention(&projection, "publication_conflict")
	} else if currentQueue != nil {
		projection.CurrentPublication = currentPublicationFromEvidence(*currentQueue, record, recordsByID, libraries)
		projection.Provenance = AssetProvenanceConverted
		projection.Publication = AssetPublicationPublished
	} else if currentDirect != nil {
		projection.CurrentPublication = currentPublicationFromEvidence(*currentDirect, record, recordsByID, libraries)
		projection.Provenance = AssetProvenancePublishedAsIs
		projection.Publication = AssetPublicationPublished
	} else if location == AssetLocationLibrary && strings.EqualFold(strings.TrimSpace(record.Status), "accepted") {
		projection.Provenance = AssetProvenanceImported
	}

	projection.SourceProvenance = sourceProvenanceFromEvidence(evidence, currentQueue, currentDirect)
	projection.CurrentRawCorrelation = resolveCurrentRawCorrelation(record, recordsByID, evidence, input, libraries)

	relevantJobs := relevantQueueJobs(record, location, input.QueueJobs)
	activeOperation, activeQueue, activeMaintenance := resolveActiveOperation(record, location, relevantJobs, input.Maintenance)
	projection.ActiveOperation = activeOperation
	if activeOperation != nil {
		appendAssetAttention(&projection, "active_work")
	}
	if input.Review.RequiresReview {
		appendAssetAttention(&projection, firstNonEmptyAssetText(input.Review.Reason, "review_required"))
	}

	latestJob := latestRelevantQueueJob(relevantJobs)
	if currentQueue != nil && currentQueue.job != nil {
		latestJob = currentQueue.job
	}
	projection.Validation = validationProjectionForJob(latestJob, activeOperation)
	if latestJob != nil && latestJob.Status == JobStatusFailed {
		projection.Publication = AssetPublicationFailed
		appendAssetAttention(&projection, "publication_failed")
	}
	if projection.CurrentPublication.State == assetCurrentPublicationConflict {
		projection.Lifecycle = AssetLifecycleNeedsReview
	} else if activeOperation != nil {
		projection.Lifecycle = lifecycleForActiveOperation(activeOperation)
	} else if latestJob != nil && latestJob.Status == JobStatusFailed {
		projection.Lifecycle = AssetLifecycleFailed
	} else if input.Review.RequiresReview {
		projection.Lifecycle = AssetLifecycleNeedsReview
	} else if projection.CurrentPublication.State == assetCurrentPublicationPublished {
		projection.Lifecycle = AssetLifecyclePublished
	} else {
		projection.Lifecycle = AssetLifecycleAvailable
	}

	if latestJob != nil && strings.TrimSpace(latestJob.OriginalArchivedPath) != "" {
		projection.Original = AssetOriginalArchived
		projection.EvidenceSummary.OriginalArchivedPath = filepath.Clean(latestJob.OriginalArchivedPath)
	} else if projection.CurrentRawCorrelation.State == assetCorrelationResolved {
		if raw, ok := recordsByID[projection.CurrentRawCorrelation.RawAssetRecordID]; ok && !raw.Missing {
			projection.Original = AssetOriginalAvailable
		}
	} else if strings.EqualFold(strings.TrimSpace(record.Status), "archive") {
		projection.Original = AssetOriginalArchived
	}

	rootSafe := assetLifecycleRootSafe(record, location, libraries, input.ArchiveRoot)
	projection.Actions = ResolveAssetActionCapabilities(AssetActionGuardContext{
		IdentityKnown:         record.ID > 0 && cleanAssetLifecyclePath(record.Path) != "",
		Location:              location,
		Missing:               record.Missing,
		HasActiveQueue:        activeQueue,
		HasActiveMaintenance:  activeMaintenance,
		RootSafe:              rootSafe,
		ProvenanceKnown:       projection.Provenance != AssetProvenanceLegacyOrUnknown,
		PublicationConflict:   projection.CurrentPublication.State == assetCurrentPublicationConflict,
		ReviewRequired:        input.Review.RequiresReview,
		QueueEligible:         location == AssetLocationRaw && strings.EqualFold(strings.TrimSpace(record.Status), "unprocessed") && !record.Missing && !input.Review.RequiresReview && activeOperation == nil,
		HasCurrentPublication: projection.CurrentPublication.State == assetCurrentPublicationPublished,
		OriginalAvailable:     projection.Original == AssetOriginalAvailable,
		OriginalArchived:      projection.Original == AssetOriginalArchived,
	})
	return projection
}

// ResolveAssetActionCapabilities renders advisory UI capabilities from
// canonical guard state. Mutation handlers remain responsible for rechecking
// the same guard at execution time.
func ResolveAssetActionCapabilities(state AssetActionGuardContext) AssetActionCapabilities {
	capabilities := AssetActionCapabilities{}
	setAllAssetActionCapabilities(&capabilities, false, assetActionReasonUnsupported)
	if !state.IdentityKnown {
		setAllAssetActionCapabilities(&capabilities, false, assetActionReasonIdentityUnresolved)
		return capabilities
	}

	capabilities.Reports = AssetActionCapability{Allowed: true}
	capabilities.History = AssetActionCapability{Allowed: true}

	if state.Missing {
		setAssetActionCapabilityReasons(&capabilities, assetActionReasonMissing,
			&capabilities.Rename, &capabilities.Move, &capabilities.Queue, &capabilities.Reconvert,
			&capabilities.LAB, &capabilities.AnalysisRefresh, &capabilities.Tracks,
			&capabilities.Subtitles, &capabilities.Attachments, &capabilities.Archive,
			&capabilities.Restore, &capabilities.DeleteOriginal, &capabilities.Publish,
			&capabilities.PublicationRetry)
		return capabilities
	}

	if state.PublicationConflict {
		setAssetActionCapabilityReasons(&capabilities, assetActionReasonPublicationConflict,
			&capabilities.Rename, &capabilities.Move, &capabilities.Reconvert,
			&capabilities.Archive, &capabilities.Restore, &capabilities.DeleteOriginal,
			&capabilities.Publish, &capabilities.PublicationRetry)
	}
	if state.HasActiveQueue || state.HasActiveMaintenance {
		setAssetActionCapabilityReasons(&capabilities, assetActionReasonActiveWork,
			&capabilities.Rename, &capabilities.Move, &capabilities.Reconvert,
			&capabilities.Archive, &capabilities.Restore, &capabilities.DeleteOriginal)
	}

	if state.Location == AssetLocationRaw && state.QueueEligible {
		capabilities.Queue = AssetActionCapability{Allowed: true}
	} else if state.Location != AssetLocationRaw {
		capabilities.Queue = AssetActionCapability{Reason: assetActionReasonNotRaw}
	} else if state.ReviewRequired {
		capabilities.Queue = AssetActionCapability{Reason: assetActionReasonRequiresReview}
	} else if !state.QueueEligible {
		capabilities.Queue = AssetActionCapability{Reason: assetActionReasonActiveWork}
	}

	if state.Location == AssetLocationLibrary && state.RootSafe && !state.HasActiveQueue && !state.HasActiveMaintenance && !state.PublicationConflict {
		capabilities.Rename = AssetActionCapability{Allowed: true}
		capabilities.Move = AssetActionCapability{Allowed: true}
	}
	if state.Location != AssetLocationLibrary {
		capabilities.Rename = AssetActionCapability{Reason: assetActionReasonNotLibrary}
		capabilities.Move = AssetActionCapability{Reason: assetActionReasonNotLibrary}
	}
	if state.Location == AssetLocationLibrary && state.HasCurrentPublication && state.ProvenanceKnown && !state.HasActiveQueue && !state.HasActiveMaintenance && !state.PublicationConflict {
		capabilities.Reconvert = AssetActionCapability{Allowed: true}
	}
	if !state.RootSafe {
		setAssetActionCapabilityReasons(&capabilities, assetActionReasonUnsafeRoot,
			&capabilities.Rename, &capabilities.Move, &capabilities.Archive,
			&capabilities.Restore, &capabilities.DeleteOriginal)
	}
	if !state.HasActiveQueue && !state.HasActiveMaintenance {
		capabilities.LAB = AssetActionCapability{Allowed: true}
		capabilities.AnalysisRefresh = AssetActionCapability{Allowed: true}
		capabilities.Tracks = AssetActionCapability{Allowed: true}
		capabilities.Subtitles = AssetActionCapability{Allowed: true}
		capabilities.Attachments = AssetActionCapability{Allowed: true}
	}
	if state.HasActiveQueue {
		capabilities.Cancel = AssetActionCapability{Allowed: true}
	}
	if state.ReviewRequired || state.HasCurrentPublication {
		capabilities.ValidationReview = AssetActionCapability{Allowed: state.ReviewRequired}
		if !state.ReviewRequired {
			capabilities.ValidationReview.Reason = assetActionReasonUnsupported
		}
	}
	if state.ReviewRequired || state.HasCurrentPublication {
		capabilities.ValidationRetry = AssetActionCapability{Allowed: true}
	}
	if state.PublicationConflict {
		capabilities.Publish = AssetActionCapability{Reason: assetActionReasonPublicationConflict}
		capabilities.PublicationRetry = AssetActionCapability{Reason: assetActionReasonPublicationConflict}
	}
	if state.Location != AssetLocationLibrary {
		capabilities.Publish = AssetActionCapability{Reason: assetActionReasonNotLibrary}
		capabilities.PublicationRetry = AssetActionCapability{Reason: assetActionReasonNotLibrary}
	}
	if state.Location == AssetLocationLibrary && !state.HasActiveQueue && !state.HasActiveMaintenance && !state.PublicationConflict && state.ProvenanceKnown {
		capabilities.Publish = AssetActionCapability{Allowed: true}
	}
	if state.OriginalAvailable && state.HasCurrentPublication && state.ProvenanceKnown && !state.HasActiveQueue && !state.HasActiveMaintenance && !state.PublicationConflict {
		capabilities.Archive = AssetActionCapability{Allowed: true}
	}
	if state.OriginalArchived && state.RootSafe && !state.HasActiveQueue && !state.HasActiveMaintenance && !state.PublicationConflict {
		capabilities.Restore = AssetActionCapability{Allowed: true}
		capabilities.DeleteOriginal = AssetActionCapability{Allowed: true}
	} else if state.OriginalArchived && !state.RootSafe {
		capabilities.Restore = AssetActionCapability{Reason: assetActionReasonUnsafeRoot}
		capabilities.DeleteOriginal = AssetActionCapability{Reason: assetActionReasonUnsafeRoot}
	} else if state.OriginalArchived {
		capabilities.Restore = AssetActionCapability{Reason: assetActionReasonActiveWork}
		capabilities.DeleteOriginal = AssetActionCapability{Reason: assetActionReasonActiveWork}
	} else {
		capabilities.Restore = AssetActionCapability{Reason: assetActionReasonNoOriginal}
		capabilities.DeleteOriginal = AssetActionCapability{Reason: assetActionReasonNoOriginal}
	}
	if !state.ProvenanceKnown {
		setAssetActionCapabilityReasons(&capabilities, assetActionReasonUnknownProvenance,
			&capabilities.Archive, &capabilities.Restore, &capabilities.DeleteOriginal)
	}
	return capabilities
}

func setAllAssetActionCapabilities(capabilities *AssetActionCapabilities, allowed bool, reason string) {
	value := AssetActionCapability{Allowed: allowed, Reason: reason}
	capabilities.Rename, capabilities.Move, capabilities.Queue = value, value, value
	capabilities.Reconvert, capabilities.LAB, capabilities.AnalysisRefresh = value, value, value
	capabilities.Tracks, capabilities.Subtitles, capabilities.Attachments = value, value, value
	capabilities.Cancel, capabilities.ValidationReview, capabilities.ValidationRetry = value, value, value
	capabilities.Publish, capabilities.PublicationRetry, capabilities.Archive = value, value, value
	capabilities.Restore, capabilities.DeleteOriginal, capabilities.Reports, capabilities.History = value, value, value, value
}

func setAssetActionCapabilityReasons(capabilities *AssetActionCapabilities, reason string, values ...*AssetActionCapability) {
	for _, value := range values {
		if value != nil && !value.Allowed {
			value.Reason = reason
		}
	}
}

func assetLifecycleLocation(record models.AssetRecord, libraries map[uint]models.Library) string {
	if record.LibraryID > 0 {
		return AssetLocationLibrary
	}
	path := cleanAssetLifecyclePath(record.Path)
	for _, library := range libraries {
		if path != "" && pathIsInside(path, library.DestinationPath) {
			return AssetLocationLibrary
		}
	}
	return AssetLocationRaw
}

func assetLifecycleDestinationMatches(record models.AssetRecord, libraryID uint, publishedPath string, libraries map[uint]models.Library) bool {
	if cleanAssetLifecyclePath(record.Path) == "" || cleanAssetLifecyclePath(record.Path) != cleanAssetLifecyclePath(publishedPath) || libraryID == 0 {
		return false
	}
	if record.LibraryID > 0 {
		return record.LibraryID == libraryID
	}
	library, ok := libraries[libraryID]
	return ok && pathIsInside(record.Path, library.DestinationPath)
}

func collectAssetPublicationEvidence(record models.AssetRecord, input AssetLifecycleProjectionInput, libraries map[uint]models.Library) []assetPublicationEvidence {
	evidence := make([]assetPublicationEvidence, 0, len(input.QueueJobs)+len(input.DirectPublications))
	for index := range input.QueueJobs {
		job := input.QueueJobs[index]
		publishedPath := strings.TrimSpace(job.PublishedPath)
		if publishedPath == "" {
			continue
		}
		sourceID := cloneUintPointer(job.SourceAssetRecordID)
		currentDestination := assetLifecycleDestinationMatches(record, job.LibraryID, publishedPath, libraries)
		sourceMatch := sourceID != nil && *sourceID == record.ID
		if !currentDestination && !sourceMatch {
			continue
		}
		publishedAt := time.Time{}
		if job.PublishedAt != nil {
			publishedAt = *job.PublishedAt
		}
		evidence = append(evidence, assetPublicationEvidence{
			authority: assetPublicationAuthorityQueue,
			id:        job.ID, sourcePath: filepath.Clean(job.MediaPath), publishedPath: filepath.Clean(publishedPath),
			sourceAssetID: sourceID, libraryID: job.LibraryID, publishedAt: publishedAt,
			current:         (currentDestination || sourceMatch) && job.PublicationRetiredAt == nil && queueJobIsCurrentPublication(job),
			originalArchive: job.OriginalArchivedPath, job: &job,
		})
	}
	for index := range input.DirectPublications {
		publication := input.DirectPublications[index]
		publishedPath := strings.TrimSpace(publication.PublishedPath)
		if publishedPath == "" {
			continue
		}
		currentDestination := assetLifecycleDestinationMatches(record, publication.LibraryID, publishedPath, libraries)
		sourceID := cloneUintPointer(publication.SourceAssetRecordID)
		sourceMatch := sourceID != nil && *sourceID == record.ID
		if !currentDestination && !sourceMatch {
			continue
		}
		evidence = append(evidence, assetPublicationEvidence{
			authority: assetPublicationAuthorityDirect,
			id:        publication.ID, sourcePath: filepath.Clean(publication.SourcePath), publishedPath: filepath.Clean(publishedPath),
			sourceAssetID: sourceID, libraryID: publication.LibraryID, publishedAt: publication.PublishedAt,
			current: (currentDestination || sourceMatch) && publication.ReturnedAt == nil,
			direct:  &publication,
		})
	}
	return evidence
}

func queueJobIsCurrentPublication(job models.QueueJob) bool {
	if job.PublicationRetiredAt != nil || strings.TrimSpace(job.PublishedPath) == "" {
		return false
	}
	return job.Status == JobStatusCompleted || job.PublishedAt != nil
}

func selectCurrentPublication(evidence []assetPublicationEvidence, authority string) *assetPublicationEvidence {
	candidates := make([]assetPublicationEvidence, 0)
	for _, item := range evidence {
		if item.authority == authority && item.current {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if !candidates[left].publishedAt.Equal(candidates[right].publishedAt) {
			return candidates[left].publishedAt.After(candidates[right].publishedAt)
		}
		return candidates[left].id > candidates[right].id
	})
	selected := candidates[0]
	return &selected
}

func currentPublicationFromEvidence(evidence assetPublicationEvidence, record models.AssetRecord, records map[uint]models.AssetRecord, libraries map[uint]models.Library) AssetCurrentPublication {
	return AssetCurrentPublication{
		State:          assetCurrentPublicationPublished,
		Authority:      evidence.authority,
		ID:             evidence.id,
		LibraryAssetID: assetLifecycleLibraryAssetID(record, records, libraries, &evidence),
		PublishedPath:  evidence.publishedPath,
		PublishedAt:    timePointerOrNil(evidence.publishedAt),
	}
}

func assetLifecycleLibraryAssetID(record models.AssetRecord, records map[uint]models.AssetRecord, libraries map[uint]models.Library, evidence *assetPublicationEvidence) uint {
	if assetLifecycleLocation(record, libraries) == AssetLocationLibrary {
		return record.ID
	}
	if evidence == nil {
		return 0
	}
	for _, candidate := range records {
		if assetLifecycleLocation(candidate, libraries) != AssetLocationLibrary {
			continue
		}
		if assetLifecycleDestinationMatches(candidate, evidence.libraryID, evidence.publishedPath, libraries) {
			return candidate.ID
		}
	}
	return 0
}

func sourceProvenanceFromEvidence(evidence []assetPublicationEvidence, currentQueue, currentDirect *assetPublicationEvidence) []AssetSourceProvenance {
	if len(evidence) == 0 {
		return nil
	}
	current := make(map[string]struct{}, 2)
	for _, item := range []*assetPublicationEvidence{currentQueue, currentDirect} {
		if item != nil {
			current[item.authority+":"+itoaAssetLifecycle(item.id)] = struct{}{}
		}
	}
	result := make([]AssetSourceProvenance, 0, len(evidence))
	seen := map[string]struct{}{}
	for _, item := range evidence {
		key := item.authority + ":" + itoaAssetLifecycle(item.id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, AssetSourceProvenance{
			Authority: item.authority, RecordID: item.id, SourcePath: item.sourcePath,
			PublishedPath: item.publishedPath, SourceAssetID: cloneUintPointer(item.sourceAssetID),
			Current:     func() bool { _, ok := current[key]; return ok }(),
			PublishedAt: timePointerOrNil(item.publishedAt),
		})
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftTime, rightTime := result[left].PublishedAt, result[right].PublishedAt
		if leftTime != nil && rightTime != nil && !leftTime.Equal(*rightTime) {
			return leftTime.After(*rightTime)
		}
		if result[left].Authority != result[right].Authority {
			return result[left].Authority < result[right].Authority
		}
		return result[left].RecordID > result[right].RecordID
	})
	return result
}

func resolveCurrentRawCorrelation(record models.AssetRecord, records map[uint]models.AssetRecord, evidence []assetPublicationEvidence, input AssetLifecycleProjectionInput, libraries map[uint]models.Library) AssetCurrentRawCorrelation {
	rawIDs := map[uint]struct{}{}
	for _, item := range evidence {
		if item.sourceAssetID == nil || *item.sourceAssetID == 0 {
			continue
		}
		source, ok := records[*item.sourceAssetID]
		if !ok || assetLifecycleLocation(source, libraries) != AssetLocationRaw {
			continue
		}
		rawIDs[source.ID] = struct{}{}
	}
	if len(rawIDs) == 0 {
		return AssetCurrentRawCorrelation{State: assetCorrelationUnresolved}
	}
	if len(rawIDs) > 1 {
		return AssetCurrentRawCorrelation{State: assetCorrelationUnresolved}
	}
	var rawID uint
	for id := range rawIDs {
		rawID = id
	}
	libraryID := uint(0)
	if assetLifecycleLocation(record, libraries) == AssetLocationLibrary {
		libraryID = record.ID
	} else {
		for _, candidate := range records {
			if assetLifecycleLocation(candidate, libraries) != AssetLocationLibrary {
				continue
			}
			for _, item := range evidence {
				if item.sourceAssetID != nil && *item.sourceAssetID == rawID && assetLifecycleDestinationMatches(candidate, item.libraryID, item.publishedPath, libraries) {
					libraryID = candidate.ID
					break
				}
			}
			if libraryID != 0 {
				break
			}
		}
	}
	return AssetCurrentRawCorrelation{State: assetCorrelationResolved, RawAssetRecordID: rawID, LibraryAssetID: libraryID}
}

func relevantQueueJobs(record models.AssetRecord, location string, jobs []models.QueueJob) []models.QueueJob {
	result := make([]models.QueueJob, 0)
	for _, job := range jobs {
		if job.SourceAssetRecordID != nil && *job.SourceAssetRecordID == record.ID {
			result = append(result, job)
			continue
		}
		if location == AssetLocationRaw && cleanAssetLifecyclePath(job.MediaPath) == cleanAssetLifecyclePath(record.Path) {
			result = append(result, job)
			continue
		}
		if location == AssetLocationLibrary && cleanAssetLifecyclePath(job.PublishedPath) == cleanAssetLifecyclePath(record.Path) {
			result = append(result, job)
		}
	}
	return result
}

func resolveActiveOperation(record models.AssetRecord, location string, jobs []models.QueueJob, maintenance []models.AssetMaintenanceOperation) (*AssetActiveOperation, bool, bool) {
	var activeQueue []models.QueueJob
	for _, job := range jobs {
		if job.DismissedAt != nil {
			continue
		}
		if job.Status == JobStatusQueued || job.Status == JobStatusRunning || (job.Status == JobStatusCompleted && job.PublishedAt == nil) {
			activeQueue = append(activeQueue, job)
		}
	}
	sort.SliceStable(activeQueue, func(left, right int) bool { return activeQueue[left].ID > activeQueue[right].ID })
	var activeMaintenance []models.AssetMaintenanceOperation
	for _, operation := range maintenance {
		if operation.Status != maintenanceStatusQueued && operation.Status != maintenanceStatusRunning {
			continue
		}
		if (operation.AssetRecordID != 0 && operation.AssetRecordID == record.ID) || cleanAssetLifecyclePath(operation.AssetPath) == cleanAssetLifecyclePath(record.Path) {
			activeMaintenance = append(activeMaintenance, operation)
		}
	}
	sort.SliceStable(activeMaintenance, func(left, right int) bool { return activeMaintenance[left].ID > activeMaintenance[right].ID })
	if len(activeQueue) > 0 {
		job := activeQueue[0]
		return &AssetActiveOperation{Kind: "queue", ID: itoaAssetLifecycle(job.ID), Status: job.Status, Stage: job.Stage}, true, len(activeMaintenance) > 0
	}
	if len(activeMaintenance) > 0 {
		operation := activeMaintenance[0]
		return &AssetActiveOperation{Kind: "maintenance", ID: operation.ID, Status: operation.Status, Stage: operation.Phase}, false, true
	}
	return nil, false, false
}

func latestRelevantQueueJob(jobs []models.QueueJob) *models.QueueJob {
	if len(jobs) == 0 {
		return nil
	}
	sort.SliceStable(jobs, func(left, right int) bool {
		leftTime, rightTime := jobs[left].UpdatedAt, jobs[right].UpdatedAt
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return jobs[left].ID > jobs[right].ID
	})
	job := jobs[0]
	return &job
}

func lifecycleForActiveOperation(operation *AssetActiveOperation) string {
	if operation == nil {
		return AssetLifecycleAvailable
	}
	if operation.Kind == "maintenance" {
		return AssetLifecycleProcessing
	}
	switch strings.ToLower(strings.TrimSpace(operation.Stage)) {
	case JobStageQueued:
		return AssetLifecycleQueued
	case JobStageClaimed, JobStagePreparingWorkspace, JobStageCopyingWorkspace, JobStagePreparingSubtitles:
		return AssetLifecyclePreparing
	case JobStageValidating, JobStageDirectPlayAnalysis, JobStageAnalyzingFinal:
		return AssetLifecycleValidating
	case JobStageReadyToPublish, JobStagePublishing, JobStageArchivingOriginal:
		return AssetLifecyclePublishing
	default:
		return AssetLifecycleProcessing
	}
}

func validationProjectionForJob(job *models.QueueJob, operation *AssetActiveOperation) string {
	if job == nil {
		return AssetValidationUnavailable
	}
	value := strings.ToLower(strings.TrimSpace(job.ValidationStatus))
	switch value {
	case ValidationStatusPassed:
		return AssetValidationPassed
	case ValidationStatusWarning:
		return AssetValidationPassedWithWarnings
	case "review", "review_required", "needs_review":
		return AssetValidationReviewRequired
	case "failed", "error":
		return AssetValidationFailed
	case "pending", "running", "validating":
		return AssetValidationPending
	default:
		if operation != nil && operation.Kind == "queue" && strings.Contains(strings.ToLower(operation.Stage), "validat") {
			return AssetValidationPending
		}
		return AssetValidationUnavailable
	}
}

func assetLifecycleRootSafe(record models.AssetRecord, location string, libraries map[uint]models.Library, archiveRoot string) bool {
	path := cleanAssetLifecyclePath(record.Path)
	if path == "" {
		return false
	}
	if location == AssetLocationLibrary {
		if library, ok := libraries[record.LibraryID]; ok {
			return pathIsInside(path, library.DestinationPath)
		}
	}
	if strings.EqualFold(strings.TrimSpace(record.Status), "archive") && strings.TrimSpace(archiveRoot) != "" {
		return pathIsInside(path, archiveRoot)
	}
	return false
}

func hasScanForRecord(record models.AssetRecord, scans []models.ScanResult) bool {
	path := cleanAssetLifecyclePath(record.Path)
	for _, scan := range scans {
		if cleanAssetLifecyclePath(scan.Path) == path {
			return true
		}
	}
	return false
}

func uniqueSortedQueueIDs(evidence []assetPublicationEvidence) []uint {
	seen := map[uint]struct{}{}
	for _, item := range evidence {
		if item.authority == assetPublicationAuthorityQueue && item.id != 0 {
			seen[item.id] = struct{}{}
		}
	}
	result := make([]uint, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func uniqueSortedDirectIDs(evidence []assetPublicationEvidence) []uint {
	seen := map[uint]struct{}{}
	for _, item := range evidence {
		if item.authority == assetPublicationAuthorityDirect && item.id != 0 {
			seen[item.id] = struct{}{}
		}
	}
	result := make([]uint, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func appendAssetAttention(projection *AssetLifecycleProjection, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range projection.Attention {
		if existing == value {
			return
		}
	}
	projection.Attention = append(projection.Attention, value)
}

func firstNonEmptyAssetText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cleanAssetLifecyclePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return ""
	}
	return clean
}

func cloneUintPointer(value *uint) *uint {
	if value == nil || *value == 0 {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func timePointerOrNil(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copyValue := value
	return &copyValue
}

func itoaAssetLifecycle(value uint) string {
	if value == 0 {
		return "0"
	}
	// Avoid fmt in the hot projection path while keeping the helper local.
	const digits = "0123456789"
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}

// AssetBulkEligibilityResult adapts the canonical QueueSelectedAssets result
// for later Assets consumers without changing Queue outcomes or reasons.
type AssetBulkEligibilityResult struct {
	AssetID    uint   `json:"assetId"`
	Selectable bool   `json:"selectable"`
	Eligible   bool   `json:"eligible"`
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	BatchID    string `json:"batchId,omitempty"`
	BatchName  string `json:"batchName,omitempty"`
	JobID      uint   `json:"jobId,omitempty"`
}

type AssetBulkEligibilityResponse struct {
	Results []AssetBulkEligibilityResult     `json:"results"`
	Summary QueueSelectedAssetsSummary       `json:"summary"`
	Batches []QueueSelectedAssetsBatchResult `json:"batches"`
}

func AdaptQueueSelectedAssetsResponse(response QueueSelectedAssetsResponse) AssetBulkEligibilityResponse {
	result := AssetBulkEligibilityResponse{
		Results: make([]AssetBulkEligibilityResult, 0, len(response.Results)),
		Summary: response.Summary,
		Batches: append([]QueueSelectedAssetsBatchResult(nil), response.Batches...),
	}
	for _, item := range response.Results {
		selectable := item.Reason != "not_found"
		eligible := item.Outcome == "eligible" || item.Outcome == "queued"
		result.Results = append(result.Results, AssetBulkEligibilityResult{
			AssetID: item.AssetID, Selectable: selectable, Eligible: eligible,
			Outcome: item.Outcome, Reason: item.Reason, Message: item.Message,
			BatchID: item.BatchID, BatchName: item.BatchName, JobID: item.JobID,
		})
	}
	return result
}

func (h AssetHandler) assetLifecycleProjections(records []models.AssetRecord) (map[uint]*AssetLifecycleProjection, error) {
	projections := make(map[uint]*AssetLifecycleProjection, len(records))
	if h.db == nil || len(records) == 0 {
		return projections, nil
	}

	var libraries []models.Library
	if h.db.Migrator().HasTable(&models.Library{}) {
		if err := h.db.Find(&libraries).Error; err != nil {
			return nil, err
		}
	}
	var jobs []models.QueueJob
	if h.db.Migrator().HasTable(&models.QueueJob{}) {
		if err := h.db.Find(&jobs).Error; err != nil {
			return nil, err
		}
	}
	var directPublications []models.DirectPublication
	if h.db.Migrator().HasTable(&models.DirectPublication{}) {
		if err := h.db.Find(&directPublications).Error; err != nil {
			return nil, err
		}
	}
	var maintenance []models.AssetMaintenanceOperation
	if h.db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		if err := h.db.Where("status IN ?", []string{maintenanceStatusQueued, maintenanceStatusRunning}).Find(&maintenance).Error; err != nil {
			return nil, err
		}
	}
	var scans []models.ScanResult
	if h.db.Migrator().HasTable(&models.ScanResult{}) {
		if err := h.db.Find(&scans).Error; err != nil {
			return nil, err
		}
	}
	reviews := map[string]AssetReviewState{}
	archiveRoot := "/media/originals_archive"
	if h.db.Migrator().HasTable(&models.AppSetting{}) {
		reviews = assetReviewOverrides(h.db)
		var err error
		archiveRoot, err = settingPath(h.db, "originalsArchivePath", archiveRoot)
		if err != nil {
			return nil, err
		}
	}
	for _, record := range records {
		projection := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
			Record: record, Records: records, Libraries: libraries, QueueJobs: jobs,
			DirectPublications: directPublications, Maintenance: maintenance,
			ScanResults: scans, Review: reviewForPath(filepath.Clean(record.Path), reviews),
			ArchiveRoot: archiveRoot,
		})
		copyProjection := projection
		projections[record.ID] = &copyProjection
	}
	return projections, nil
}

// assetHasActiveWork is the shared execution-time guard used by destructive
// asset mutations and capability snapshots. It prefers the durable source
// AssetRecord reference and retains path matching for legacy rows.
func (h AssetHandler) assetHasActiveWork(record models.AssetRecord) (bool, error) {
	if h.db == nil {
		return false, nil
	}
	paths := []string{filepath.Clean(record.Path)}
	if sourcePath := cleanAssetLifecyclePath(record.SourcePath); sourcePath != "" && sourcePath != paths[0] {
		paths = append(paths, sourcePath)
	}
	var queueCount int64
	if h.db.Migrator().HasTable(&models.QueueJob{}) {
		if err := h.db.Model(&models.QueueJob{}).
			Where("dismissed_at IS NULL").
			Where("(status IN ? OR (status = ? AND published_at IS NULL))", []string{JobStatusQueued, JobStatusRunning}, JobStatusCompleted).
			Where("source_asset_record_id = ? OR media_path IN ? OR published_path IN ? OR planned_published_path IN ? OR replacement_target_path IN ? OR original_archived_path IN ?", record.ID, paths, paths, paths, paths, paths).
			Count(&queueCount).Error; err != nil {
			return false, err
		}
	}
	if queueCount > 0 {
		return true, nil
	}
	var maintenanceCount int64
	if h.db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		if err := h.db.Model(&models.AssetMaintenanceOperation{}).
			Where("status IN ?", []string{maintenanceStatusQueued, maintenanceStatusRunning}).
			Where("asset_record_id = ? OR asset_path IN ?", record.ID, paths).
			Count(&maintenanceCount).Error; err != nil {
			return false, err
		}
	}
	return maintenanceCount > 0, nil
}
