package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

var errQueueValidationDismissalConflict = errors.New("job is not eligible for Validation dismissal")

func queueValidationDismissalConflict(reason string) error {
	return fmt.Errorf("%w: %s", errQueueValidationDismissalConflict, reason)
}

// validateCompletedValidationDismissal keeps Dismiss limited to a current,
// unpublished Validation exception. The caller runs this inside the same
// transaction as the QueueJob dismissal and reservation release.
func validateCompletedValidationDismissal(db *gorm.DB, job models.QueueJob) error {
	if job.Status != JobStatusCompleted || job.PublishedAt != nil || strings.TrimSpace(job.PublishedPath) != "" || job.PublicationRetiredAt != nil {
		return queueValidationDismissalConflict("only completed, unpublished jobs can be dismissed")
	}
	if strings.TrimSpace(job.PublicationID) != "" || strings.TrimSpace(job.PublicationState) != "" {
		return queueValidationDismissalConflict("Publisher already has a lifecycle record for this job")
	}
	switch job.Stage {
	case JobStageReadyToPublish, JobStagePublishing, JobStageArchivingOriginal, JobStageCleaningWorkspace:
		return queueValidationDismissalConflict("the job is in a Publisher or cleanup stage")
	}
	if job.OriginalArchiveStatus == "archiving" || job.OriginalArchiveStatus == "complete" || job.OriginalArchiveStatus == "failed" || strings.TrimSpace(job.OriginalArchivedPath) != "" {
		return queueValidationDismissalConflict("original archival has started or completed")
	}
	if job.ExecutionNumber == nil || strings.TrimSpace(job.ValidationRunID) == "" {
		return queueValidationDismissalConflict("the active ValidationRun identity is missing")
	}
	if job.ValidationOutcome != models.QualityOutcomeFail && job.ValidationOutcome != models.QualityOutcomeReviewRequired {
		return queueValidationDismissalConflict("Validation outcome must be FAIL or REVIEW_REQUIRED")
	}
	if !db.Migrator().HasTable(&models.PublisherPublication{}) {
		return queueValidationDismissalConflict("Publisher lifecycle state is unavailable")
	}
	var publications int64
	if err := db.Model(&models.PublisherPublication{}).
		Where("queue_job_id = ? AND execution_number = ?", job.ID, *job.ExecutionNumber).
		Count(&publications).Error; err != nil {
		return err
	}
	if publications != 0 {
		return queueValidationDismissalConflict("a Publisher lifecycle record already exists")
	}
	if !db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		return queueValidationDismissalConflict("asset maintenance state is unavailable")
	}
	var maintenance []models.AssetMaintenanceOperation
	if err := db.Where("status IN ?", []string{maintenanceStatusQueued, maintenanceStatusRunning}).Find(&maintenance).Error; err != nil {
		return err
	}
	maintenanceContext := &queueBatchValidationContext{
		maintenance:        maintenance,
		pathStats:          map[string]queuePathStat{},
		maintenanceResults: map[string]bool{},
	}
	if maintenanceContext.activeMaintenance(job.MediaPath) {
		return queueValidationDismissalConflict("asset maintenance is active")
	}

	var run models.ValidationRun
	if err := db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error; err != nil {
		return queueValidationDismissalConflict("the active ValidationRun is missing")
	}
	if run.Outcome != job.ValidationOutcome || (run.Outcome != models.QualityOutcomeFail && run.Outcome != models.QualityOutcomeReviewRequired) {
		return queueValidationDismissalConflict("QueueJob outcome does not match the active ValidationRun")
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil || !runMatchesResult(run, artifact) {
		return queueValidationDismissalConflict("the active ValidationRun is not bound to the current Result")
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK || run.ExecutionNumber != binding.ExecutionNumber || run.ExecutionPlanID != binding.ExecutionPlanID || run.ExecutionPlanVersion != binding.ExecutionPlanVersion || run.ExecutionPlanFingerprint != binding.ExecutionPlanFingerprint || run.ResultArtifactID != identity.ArtifactID || run.ResultGenerationToken != identity.GenerationToken || run.ResultGenerationVersion != identity.GenerationVersion || run.ResultOutputRevision != identity.OutputRevision {
		return queueValidationDismissalConflict("ValidationRun identity does not match the frozen Result and ExecutionPlan")
	}
	if !frozenBindingMatches(db, job, binding) {
		return queueValidationDismissalConflict("the frozen Queue or ExecutionPlan identity is stale")
	}
	derivedOutcome, _ := reduceValidationOutcome(validationFindingsForRun(run))
	if derivedOutcome != run.Outcome {
		return queueValidationDismissalConflict("ValidationRun outcome is inconsistent with its immutable findings")
	}
	return nil
}
