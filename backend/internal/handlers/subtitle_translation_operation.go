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

// subtitleTranslationOperationRequest is the bounded, secret-free execution
// snapshot retained for a possible retry. It deliberately contains no cue
// payloads, credentials, prompts, or provider responses.
type subtitleTranslationOperationRequest struct {
	AssetPath               string
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
// Task 8 can supply the source-specific translation execution later.
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
		setSubtitleTranslationTerminal(operationID, "completed", "completed", message, "")
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", nil)
		return
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", nil)
			return
		}
		updateSubtitleTranslationProgress(operationID, "cleanup", -1, -1, "Cleaning up subtitle translation")
		cleanupErr := cleanupSubtitleOperationWorkspace(workspace)
		if cleanupErr != nil {
			err = fmt.Errorf("%w; cleanup diagnostic: %v", err, cleanupErr)
		}
		setSubtitleTranslationTerminal(operationID, "error", "error", "Subtitle translation failed", boundedSubtitleTranslationError(err))
		return
	}

	if err := ctx.Err(); err != nil {
		finishSubtitleTranslationOperation(operationID, workspace, "cancelled", "Subtitle translation cancelled", nil)
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
		setSubtitleTranslationTerminal(operationID, "error", "error", "Subtitle translation failed", boundedSubtitleTranslationError(cleanupErr))
		return
	}
	setSubtitleTranslationTerminal(operationID, "completed", "completed", "Subtitle translation completed", "")
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
		if cleanupErr != nil {
			message += "; cleanup diagnostic: " + boundedSubtitleTranslationError(cleanupErr)
		}
		setSubtitleTranslationTerminal(operationID, "cancelled", "cleanup", message, "")
		return
	}
	setSubtitleTranslationTerminal(operationID, "error", "error", message, boundedSubtitleTranslationError(err))
}

func setSubtitleTranslationTerminal(operationID, status, phase, message, operationError string) {
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = status
		operation.Phase = phase
		operation.Message = message
		operation.Error = operationError
		if status == "completed" {
			operation.Progress = 100
		}
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
	"cleanup":     4,
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
		if processed >= 0 && processed > operation.Processed {
			operation.Processed = processed
		}
		if total > operation.Total {
			operation.Total = total
		}
		if operation.Total > 0 && operation.Processed > operation.Total {
			operation.Processed = operation.Total
		}
		progress := operation.Progress
		if operation.Total > 0 {
			candidate := float64(operation.Processed) / float64(operation.Total) * 100
			if candidate > progress {
				progress = candidate
			}
		}
		if progress > 100 {
			progress = 100
		}
		if progress >= operation.Progress {
			operation.Progress = progress
		}
		operation.Phase = phase
		if message != "" {
			operation.Message = message
		}
	})
}

// retrySubtitleTranslationOperation starts a fresh operation from the old
// bounded request snapshot. Terminal state, progress, cancellation, and
// workspace are never copied into the new operation.
func retrySubtitleTranslationOperation(operationID string, runner subtitleTranslationRunner) (string, error) {
	subtitleExtractionOperations.RLock()
	operation, exists := subtitleExtractionOperations.items[operationID]
	if !exists {
		subtitleExtractionOperations.RUnlock()
		return "", errors.New("subtitle translation operation not found")
	}
	if operation.Kind != subtitleTranslationOperationKind {
		subtitleExtractionOperations.RUnlock()
		return "", errors.New("operation is not a subtitle translation")
	}
	if operation.Status == "running" {
		subtitleExtractionOperations.RUnlock()
		return "", errors.New("running subtitle translation cannot be retried")
	}
	if operation.Status != "error" && operation.Status != "cancelled" {
		subtitleExtractionOperations.RUnlock()
		return "", errors.New("only failed or cancelled subtitle translation can be retried")
	}
	if operation.translationRequest == nil {
		subtitleExtractionOperations.RUnlock()
		return "", errors.New("subtitle translation retry request is unavailable")
	}
	request := *operation.translationRequest
	subtitleExtractionOperations.RUnlock()
	return startSubtitleTranslationOperation(request, runner)
}
