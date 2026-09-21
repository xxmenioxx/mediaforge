package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	subtitleTranslationOperationKind     = "translation"
	maxSubtitleTranslationRequestPath    = 4096
	maxSubtitleTranslationRequestField   = 1024
	maxSubtitleTranslationOperationError = 2048
)

type subtitleTranslationFailureClass string

const (
	subtitleTranslationFailureProviderConfiguration subtitleTranslationFailureClass = "provider_configuration"
	subtitleTranslationFailureProviderUnavailable   subtitleTranslationFailureClass = "provider_unavailable"
	subtitleTranslationFailureProviderTimeout       subtitleTranslationFailureClass = "provider_timeout"
	subtitleTranslationFailureMalformedResponse     subtitleTranslationFailureClass = "malformed_response"
	subtitleTranslationFailureStaleSource           subtitleTranslationFailureClass = "stale_source"
	subtitleTranslationFailureCancelled             subtitleTranslationFailureClass = "cancelled"
	subtitleTranslationFailureTranslationFailed     subtitleTranslationFailureClass = "translation_failed"
)

type subtitleTranslationOperationView struct {
	Provider                string `json:"provider,omitempty"`
	Model                   string `json:"model,omitempty"`
	RequestedSourceLanguage string `json:"requestedSourceLanguage,omitempty"`
	TargetLocale            string `json:"targetLocale,omitempty"`
	OutputRepresentation    string `json:"outputRepresentation,omitempty"`
	ContextMode             string `json:"contextMode,omitempty"`
	ContextSource           string `json:"contextSource,omitempty"`
	FailureClass            string `json:"failureClass,omitempty"`
	Diagnostic              string `json:"diagnostic,omitempty"`
	Retryable               bool   `json:"retryable"`
	Cancellable             bool   `json:"cancellable"`
}

// subtitleTranslationOperationRequest is the bounded, secret-free execution
// snapshot retained for a possible retry. It deliberately contains no cue
// payloads, credentials, prompts, or provider responses.
type subtitleTranslationOperationRequest struct {
	AssetPath               string
	SourceType              string
	StreamIndex             int
	Format                  string
	SourceIdentity          string
	SourceReference         string
	RequestedSourceLanguage string
	TargetLocale            string
	RequestedRepresentation string
	Provider                string
	ProviderModel           string
	ContextMode             string
	ContextSource           string
	SourceRevision          string
}

// subtitleTranslationRunner is the operation-owned execution seam. The
// runner reports bounded progress through updateSubtitleTranslationProgress;
// Asset Info supplies the exact source-specific execution request.
type subtitleTranslationRunner func(
	context.Context,
	string,
	string,
	subtitleTranslationOperationRequest,
) error

func validateSubtitleTranslationOperationRequest(request subtitleTranslationOperationRequest) error {
	if len(request.AssetPath) > maxSubtitleTranslationRequestPath {
		return fmt.Errorf("translation asset path exceeds %d bytes", maxSubtitleTranslationRequestPath)
	}
	fields := []struct {
		name  string
		value string
	}{
		{"format", request.Format},
		{"source identity", request.SourceIdentity},
		{"source reference", request.SourceReference},
		{"requested source language", request.RequestedSourceLanguage},
		{"target locale", request.TargetLocale},
		{"requested representation", request.RequestedRepresentation},
		{"provider", request.Provider},
		{"provider model", request.ProviderModel},
		{"context mode", request.ContextMode},
		{"context source", request.ContextSource},
		{"source revision", request.SourceRevision},
	}
	if len(request.SourceType) > maxSubtitleTranslationRequestField {
		return fmt.Errorf("translation source type exceeds %d bytes", maxSubtitleTranslationRequestField)
	}
	for _, field := range fields {
		if len(field.value) > maxSubtitleTranslationRequestField {
			return fmt.Errorf("translation %s exceeds %d bytes", field.name, maxSubtitleTranslationRequestField)
		}
	}
	return nil
}

// startSubtitleTranslationOperation registers a translation operation in the
// existing subtitle operation registry and starts its bounded private work.
// Source selection and publication intentionally remain outside this seam.
func startSubtitleTranslationOperation(
	request subtitleTranslationOperationRequest,
	runner subtitleTranslationRunner,
) (string, error) {
	if runner == nil {
		return "", errors.New("translation operation runner is required")
	}
	if err := validateSubtitleTranslationOperationRequest(request); err != nil {
		return "", err
	}

	root := canonicalOCRWorkspaceRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("cannot create translation workspace root: %w", err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-translation-*")
	if err != nil {
		return "", fmt.Errorf("cannot create translation workspace: %w", err)
	}

	operationID := fmt.Sprintf("subtitle-translation-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	now := time.Now()
	requestSnapshot := request
	operation := &subtitleExtractionOperation{
		ID:                     operationID,
		Kind:                   subtitleTranslationOperationKind,
		Status:                 "running",
		Phase:                  "preparing",
		Progress:               1,
		StreamIndex:            request.StreamIndex,
		Format:                 request.Format,
		AssetPath:              request.AssetPath,
		Message:                "Preparing subtitle translation",
		Translation:            newSubtitleTranslationOperationView(request),
		CreatedAt:              now,
		UpdatedAt:              now,
		translationRequest:     &requestSnapshot,
		cancel:                 cancel,
		publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()

	go func() {
		defer cancel()
		runSubtitleTranslationOperation(workCtx, operationID, workspace, requestSnapshot, runner)
	}()
	return operationID, nil
}

func newSubtitleTranslationOperationView(request subtitleTranslationOperationRequest) *subtitleTranslationOperationView {
	return &subtitleTranslationOperationView{
		Provider:                request.Provider,
		Model:                   request.ProviderModel,
		RequestedSourceLanguage: request.RequestedSourceLanguage,
		TargetLocale:            request.TargetLocale,
		OutputRepresentation:    request.RequestedRepresentation,
		ContextMode:             request.ContextMode,
		ContextSource:           request.ContextSource,
		Cancellable:             true,
	}
}

func cloneSubtitleExtractionOperationSnapshot(operation *subtitleExtractionOperation) subtitleExtractionOperation {
	snapshot := *operation
	if operation.Translation != nil {
		translation := *operation.Translation
		snapshot.Translation = &translation
	}
	if operation.Result != nil {
		result := *operation.Result
		result.Created = append([]string(nil), operation.Result.Created...)
		result.Existing = append([]string(nil), operation.Result.Existing...)
		result.Unsupported = append([]string(nil), operation.Result.Unsupported...)
		result.Artifacts = append([]SubtitleArtifact(nil), operation.Result.Artifacts...)
		snapshot.Result = &result
	}
	return snapshot
}

func subtitleTranslationFailureClassOf(err error) subtitleTranslationFailureClass {
	if errors.Is(err, errTranslationSourceStale) {
		return subtitleTranslationFailureStaleSource
	}
	var providerErr *TranslationProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		switch providerErr.Class {
		case translationProviderErrorConfiguration:
			return subtitleTranslationFailureProviderConfiguration
		case translationProviderErrorUnavailable:
			return subtitleTranslationFailureProviderUnavailable
		case translationProviderErrorTimeout:
			return subtitleTranslationFailureProviderTimeout
		case translationProviderErrorMalformed:
			return subtitleTranslationFailureMalformedResponse
		case translationProviderErrorCancelled:
			return subtitleTranslationFailureCancelled
		}
	}
	if errors.Is(err, context.Canceled) {
		return subtitleTranslationFailureCancelled
	}
	return subtitleTranslationFailureTranslationFailed
}

func subtitleTranslationDiagnostic(class subtitleTranslationFailureClass) string {
	switch class {
	case subtitleTranslationFailureProviderConfiguration:
		return "Translation provider configuration is invalid."
	case subtitleTranslationFailureProviderUnavailable:
		return "Translation provider is unavailable."
	case subtitleTranslationFailureProviderTimeout:
		return "Translation provider request timed out."
	case subtitleTranslationFailureMalformedResponse:
		return "Translation provider returned a malformed response."
	case subtitleTranslationFailureStaleSource:
		return "Subtitle source changed before publication."
	case subtitleTranslationFailureCancelled:
		return "Subtitle translation cancelled."
	default:
		return "Subtitle translation failed."
	}
}

func runSubtitleTranslationOperation(
	ctx context.Context,
	operationID string,
	workspace string,
	request subtitleTranslationOperationRequest,
	runner subtitleTranslationRunner,
) {
	if errors.Is(ctx.Err(), context.Canceled) {
		finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", nil)
		return
	}
	err := runner(ctx, operationID, workspace, request)
	if subtitleOperationPublicationCommitted(operationID) {
		cleanupErr := cleanupSubtitleOperationWorkspace(workspace)
		message := "Subtitle translation completed"
		if cleanupErr != nil {
			message += "; cleanup diagnostic: " + boundedSubtitleTranslationError(cleanupErr)
		}
		setSubtitleTranslationTerminal(operationID, "completed", "completed", message, nil)
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", err)
		return
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", err)
			return
		}
		updateSubtitleTranslationProgress(operationID, "cleanup", -1, -1, "Cleaning up subtitle translation")
		cleanupErr := cleanupSubtitleOperationWorkspace(workspace)
		if cleanupErr != nil {
			err = fmt.Errorf("%w; cleanup diagnostic: %v", err, cleanupErr)
		}
		setSubtitleTranslationTerminal(operationID, "error", "error", "Subtitle translation failed", err)
		return
	}

	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", err)
		} else {
			finishSubtitleTranslationOperation(operationID, workspace, "error", "Subtitle translation failed", err)
		}
		return
	}
	updateSubtitleTranslationProgress(operationID, "validating", -1, -1, "Validating translated subtitles")
	updateSubtitleTranslationProgress(operationID, "cleanup", -1, -1, "Cleaning up subtitle translation")
	cleanupErr := cleanupSubtitleOperationWorkspace(workspace)
	if errors.Is(ctx.Err(), context.Canceled) {
		finishSubtitleTranslationOperation(operationID, "", "cancelled", "Subtitle translation cancelled", nil)
		return
	}
	if cleanupErr != nil {
		setSubtitleTranslationTerminal(operationID, "error", "error", "Subtitle translation failed", cleanupErr)
		return
	}
	setSubtitleTranslationTerminal(operationID, "completed", "completed", "Subtitle translation completed", nil)
}

func finishSubtitleTranslationOperation(
	operationID string,
	workspace string,
	status string,
	message string,
	err error,
) {
	updateSubtitleTranslationProgress(operationID, "cleanup", -1, -1, "Cleaning up subtitle translation")
	cleanupErr := error(nil)
	if workspace != "" {
		cleanupErr = cleanupSubtitleOperationWorkspace(workspace)
	}
	if cleanupErr != nil {
		if err != nil {
			err = fmt.Errorf("%w; cleanup diagnostic: %v", err, cleanupErr)
		} else {
			err = cleanupErr
		}
	}
	if status == "cancelled" {
		message = "Subtitle translation cancelled"
		if cleanupErr != nil {
			message += "; cleanup diagnostic unavailable"
		}
		setSubtitleTranslationTerminal(operationID, "cancelled", "cleanup", message, err)
		return
	}
	setSubtitleTranslationTerminal(operationID, "error", "error", message, err)
}

func setSubtitleTranslationTerminal(operationID, status, phase, message string, err error) {
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = status
		operation.Phase = phase
		operation.Message = message
		failureClass := subtitleTranslationFailureClassOf(err)
		if status == "cancelled" {
			failureClass = subtitleTranslationFailureCancelled
		}
		if status == "completed" {
			failureClass = ""
		}
		if operation.Translation == nil && operation.translationRequest != nil {
			operation.Translation = newSubtitleTranslationOperationView(*operation.translationRequest)
		}
		if operation.Translation != nil {
			operation.Translation.FailureClass = string(failureClass)
			operation.Translation.Diagnostic = ""
			operation.Translation.Retryable = status == "error" || status == "cancelled"
			operation.Translation.Cancellable = false
			if failureClass != "" {
				operation.Translation.Diagnostic = subtitleTranslationDiagnostic(failureClass)
			}
		}
		if status == "cancelled" || status == "completed" {
			operation.Error = ""
		} else if failureClass != "" {
			operation.Error = subtitleTranslationDiagnostic(failureClass)
		} else {
			operation.Error = subtitleTranslationDiagnostic(subtitleTranslationFailureTranslationFailed)
		}
		if status == "completed" {
			operation.Progress = 100
		}
	})
}

func setSubtitleTranslationResult(operationID string, artifacts []SubtitleArtifact) {
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		created := make([]string, 0, len(artifacts))
		for _, artifact := range artifacts {
			if artifact.PublishedPath != "" {
				created = append(created, artifact.PublishedPath)
			}
		}
		copiedArtifacts := append([]SubtitleArtifact(nil), artifacts...)
		operation.Result = &SubtitleExtractionResult{Created: created, Artifacts: copiedArtifacts}
	})
}

func boundedSubtitleTranslationError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	runes := []rune(message)
	if len(runes) <= maxSubtitleTranslationOperationError {
		return message
	}
	return string(runes[:maxSubtitleTranslationOperationError]) + "..."
}

var subtitleTranslationPhaseRank = map[string]int{
	"preparing":   0,
	"ocr":         1,
	"translating": 2,
	"validating":  3,
	"publishing":  4,
	"cleanup":     5,
}

var subtitleTranslationPhaseProgress = map[string][2]float64{
	"preparing":   {1, 10},
	"ocr":         {10, 40},
	"translating": {40, 80},
	"validating":  {85, 85},
	"publishing":  {90, 90},
	"cleanup":     {95, 95},
}

func updateSubtitleTranslationProgress(operationID, phase string, processed, total int, message string) {
	if _, ok := subtitleTranslationPhaseRank[phase]; !ok {
		return
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		if operation.Kind != subtitleTranslationOperationKind {
			return
		}
		currentRank := subtitleTranslationPhaseRank[operation.Phase]
		requestedRank := subtitleTranslationPhaseRank[phase]
		if requestedRank < currentRank {
			return
		}
		phaseChanged := requestedRank > currentRank
		if phaseChanged && (processed >= 0 || total >= 0) {
			operation.Processed = 0
			operation.Total = 0
		}
		if processed >= 0 {
			if phaseChanged || processed > operation.Processed {
				operation.Processed = processed
			}
		}
		if total >= 0 && (phaseChanged || total > operation.Total) {
			operation.Total = total
		}
		if operation.Total > 0 && operation.Processed > operation.Total {
			operation.Processed = operation.Total
		}
		bounds := subtitleTranslationPhaseProgress[phase]
		progress := bounds[0]
		if operation.Total > 0 {
			ratio := float64(operation.Processed) / float64(operation.Total)
			progress = bounds[0] + ratio*(bounds[1]-bounds[0])
		}
		if progress > 100 {
			progress = 100
		}
		if progress < operation.Progress {
			progress = operation.Progress
		}
		operation.Progress = progress
		operation.Phase = phase
		if message != "" {
			operation.Message = message
		}
		if operation.Translation != nil && operation.Status == "running" {
			operation.Translation.Cancellable = !operation.publicationCommitted
		}
	})
}

// retrySubtitleTranslationOperation starts a fresh operation from the old
// bounded request snapshot. Terminal state, progress, cancellation, and
// workspace are never copied into the new operation.
func retrySubtitleTranslationOperation(operationID string, runner subtitleTranslationRunner) (string, error) {
	request, err := subtitleTranslationRetryRequest(operationID)
	if err != nil {
		return "", err
	}
	return startSubtitleTranslationOperation(request, runner)
}

func subtitleTranslationRetryRequest(operationID string) (subtitleTranslationOperationRequest, error) {
	subtitleExtractionOperations.RLock()
	operation, exists := subtitleExtractionOperations.items[operationID]
	if !exists {
		subtitleExtractionOperations.RUnlock()
		return subtitleTranslationOperationRequest{}, errors.New("subtitle translation operation not found")
	}
	if operation.Kind != subtitleTranslationOperationKind {
		subtitleExtractionOperations.RUnlock()
		return subtitleTranslationOperationRequest{}, errors.New("operation is not a subtitle translation")
	}
	if operation.Status == "running" {
		subtitleExtractionOperations.RUnlock()
		return subtitleTranslationOperationRequest{}, errors.New("running subtitle translation cannot be retried")
	}
	if operation.Status != "error" && operation.Status != "cancelled" {
		subtitleExtractionOperations.RUnlock()
		return subtitleTranslationOperationRequest{}, errors.New("only failed or cancelled subtitle translation can be retried")
	}
	if operation.translationRequest == nil {
		subtitleExtractionOperations.RUnlock()
		return subtitleTranslationOperationRequest{}, errors.New("subtitle translation retry request is unavailable")
	}
	request := *operation.translationRequest
	subtitleExtractionOperations.RUnlock()
	return request, nil
}

func retrySubtitleTranslationOperationWithRequest(
	operationID string,
	request subtitleTranslationOperationRequest,
	runner subtitleTranslationRunner,
) (string, error) {
	if _, err := subtitleTranslationRetryRequest(operationID); err != nil {
		return "", err
	}
	return startSubtitleTranslationOperation(request, runner)
}
