package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const translationCheckpointRecoveryBatch = 1000

func (h AssetHandler) recoverProtectedTranslationOperations() {
	if h.db == nil || !h.db.Migrator().HasTable(&models.TranslationOperationCheckpoint{}) {
		return
	}
	var checkpoints []models.TranslationOperationCheckpoint
	if err := h.db.Where("execution_phase NOT IN ?", []string{"COMPLETED", "FAILED", "CANCELLED"}).Order("updated_at asc").Limit(translationCheckpointRecoveryBatch).Find(&checkpoints).Error; err != nil {
		return
	}
	var providerSettingsErr error
	for _, checkpoint := range checkpoints {
		request, metadataErr := decodeTranslationCheckpointRequest(checkpoint)
		if checkpoint.Cancelled {
			if checkpoint.InferenceStarted {
				markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "cancelled_after_inference_started")
				checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			} else {
				_ = updateTranslationCheckpointPhase(h.db, checkpoint.OperationID, "CANCELLED", "")
				checkpoint.ExecutionPhase = "CANCELLED"
			}
			h.restoreTranslationOperation(checkpoint, request, "cancelled", "cancelled", "Subtitle translation was cancelled")
			continue
		}
		if checkpoint.ExecutionPhase == "PUBLICATION_COMMITTED" || checkpoint.ExecutionPhase == "PUBLISHING" {
			if reconcileTranslationPublication(h.db, &checkpoint) {
				h.restoreTranslationOperation(checkpoint, request, "completed", "completed", "Subtitle translation completed")
			} else {
				forceTranslationRecoveryRequired(h.db, checkpoint.OperationID, "publication_files_do_not_match_durable_intent")
				checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
				h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Published output requires manual recovery")
			}
			continue
		}
		if checkpoint.InferenceStarted || checkpoint.ExecutionPhase == "INFERENCE_STARTED" || checkpoint.ExecutionPhase == "RECOVERY_REQUIRED" {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "inference_outcome_uncertain_after_restart")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Inference outcome is uncertain; use a new Translation operation ID")
			continue
		}
		if checkpoint.ExecutionPhase != "WAITING_FOR_RESOURCES" && checkpoint.ExecutionPhase != "WAITING_FOR_MODEL" {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "checkpoint_is_not_a_complete_waiting_state")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Translation checkpoint is incomplete and requires manual recovery")
			continue
		}
		if !h.aiModelLifecycle.protectedEnabled() {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "protected_lifecycle_disabled_after_restart")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Protected Translation recovery is disabled")
			continue
		}
		if metadataErr != nil || validateTranslationWaitingCheckpoint(checkpoint, request, time.Now()) != nil {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "waiting_checkpoint_validation_failed")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Translation checkpoint failed identity or deadline validation")
			continue
		}
		if providerSettingsErr == nil {
			providerSettingsErr = MigrateOllamaProviderSettings(h.db)
		}
		settings := loadTranslationSettings(h.db)
		if providerSettingsErr != nil || validateTranslationProviderCheckpoint(settings, checkpoint) != nil {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "provider_model_or_configuration_changed")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Selected model or provider configuration changed")
			continue
		}
		configurationRevision := checkpoint.ProviderConfigurationRevision
		validationCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, source, stream, sourceErr := h.resolveTranslationExecutionSource(validationCtx, request)
		if sourceErr == nil && source.SourceRevision == checkpoint.SourceRevision {
			var revision translationSourceRevision
			revision, sourceErr = translationSourceRevisionForResolvedSource(source, stream)
			if sourceErr == nil && revision.key() != checkpoint.SourceRevision {
				sourceErr = errors.New("source revision changed")
			}
		}
		cancel()
		if sourceErr != nil {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "source_identity_or_revision_changed")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Translation source changed or could not be verified")
			continue
		}
		provider, err := h.translationProviderForSettings(settings)
		if err != nil {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "provider_unavailable_during_recovery")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Translation provider could not be reconstructed")
			continue
		}
		runner := h.subtitleTranslationRunner(request, settings, provider, configurationRevision, checkpoint.AdmissionWorkflowID, true)
		if _, err := startSubtitleTranslationOperationWithIdentity(request, runner, checkpoint.OperationID, checkpoint.Deadline, nil); err != nil {
			markTranslationRecoveryRequired(h.db, checkpoint.OperationID, "waiting_operation_reconstruction_failed")
			checkpoint.ExecutionPhase = "RECOVERY_REQUIRED"
			h.restoreTranslationOperation(checkpoint, request, "error", "recovery_required", "Waiting Translation operation could not be reconstructed")
		}
	}
}

func decodeTranslationCheckpointRequest(checkpoint models.TranslationOperationCheckpoint) (subtitleTranslationOperationRequest, error) {
	var request subtitleTranslationOperationRequest
	if len(checkpoint.RequestMetadata) == 0 || len(checkpoint.RequestMetadata) > 16*1024 {
		return request, errors.New("Translation checkpoint metadata is incomplete")
	}
	decoder := json.NewDecoder(strings.NewReader(checkpoint.RequestMetadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return subtitleTranslationOperationRequest{}, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return subtitleTranslationOperationRequest{}, errors.New("Translation checkpoint metadata has trailing content")
	}
	if err := validateSubtitleTranslationOperationRequest(request); err != nil {
		return subtitleTranslationOperationRequest{}, err
	}
	return request, nil
}

func validateTranslationWaitingCheckpoint(checkpoint models.TranslationOperationCheckpoint, request subtitleTranslationOperationRequest, now time.Time) error {
	if checkpoint.CheckpointVersion != 1 || checkpoint.Cancelled || checkpoint.InferenceStarted || !checkpoint.Deadline.After(now) {
		return errors.New("Translation waiting checkpoint version, cancellation, inference, or deadline is invalid")
	}
	if checkpoint.ExecutionPhase != "WAITING_FOR_RESOURCES" && checkpoint.ExecutionPhase != "WAITING_FOR_MODEL" {
		return errors.New("Translation checkpoint is not a resumable waiting phase")
	}
	if strings.TrimSpace(checkpoint.OperationID) == "" || strings.TrimSpace(checkpoint.AdmissionWorkflowID) == "" || strings.TrimSpace(checkpoint.SourceIdentity) == "" || strings.TrimSpace(checkpoint.SourceRevision) == "" || strings.TrimSpace(checkpoint.ProviderConfigurationRevision) == "" || strings.TrimSpace(checkpoint.SelectedModel) == "" || checkpoint.RowVersion == 0 {
		return errors.New("Translation waiting checkpoint is missing required identity")
	}
	if request.ProviderModel != checkpoint.SelectedModel || request.SourceIdentity != checkpoint.SourceIdentity || request.SourceRevision != checkpoint.SourceRevision {
		return errors.New("Translation waiting checkpoint request identity does not match its durable identity")
	}
	return nil
}

func validateTranslationProviderCheckpoint(settings translationSettings, checkpoint models.TranslationOperationCheckpoint) error {
	configurationRevision := providerConfigurationRevision(settings.OllamaEndpoint, settings.OllamaModel)
	if settings.Provider != translationProviderOllama || settings.OllamaModel != checkpoint.SelectedModel || configurationRevision != checkpoint.ProviderConfigurationRevision {
		return errors.New("Translation provider, model, or configuration changed")
	}
	return nil
}

func (h AssetHandler) restoreTranslationOperation(checkpoint models.TranslationOperationCheckpoint, request subtitleTranslationOperationRequest, status, phase, message string) {
	if strings.TrimSpace(request.SourceIdentity) == "" {
		request.SourceIdentity = checkpoint.SourceIdentity
	}
	if strings.TrimSpace(request.SourceRevision) == "" {
		request.SourceRevision = checkpoint.SourceRevision
	}
	if strings.TrimSpace(request.ProviderModel) == "" {
		request.ProviderModel = checkpoint.SelectedModel
	}
	requestCopy := request
	operation := &subtitleExtractionOperation{
		ID: checkpoint.OperationID, Status: status, Phase: phase, Progress: 1,
		StreamIndex: request.StreamIndex, Format: request.Format, AssetPath: request.AssetPath,
		Message: message, Kind: subtitleTranslationOperationKind,
		Translation: newSubtitleTranslationOperationView(request), CreatedAt: checkpoint.CreatedAt,
		UpdatedAt: time.Now().UTC(), translationRequest: &requestCopy,
		publicationCommitted:   checkpoint.ExecutionPhase == "PUBLICATION_COMMITTED",
		publicationCommittedCh: make(chan struct{}),
	}
	if operation.publicationCommitted {
		close(operation.publicationCommittedCh)
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operation.ID] = operation
	subtitleExtractionOperations.Unlock()
}
