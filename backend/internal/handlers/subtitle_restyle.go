package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type subtitleRestyleInput struct {
	Path                      string               `json:"path" binding:"required"`
	SourcePath                string               `json:"sourcePath" binding:"required"`
	SourceRevision            string               `json:"sourceRevision,omitempty"`
	Representation            string               `json:"representation"`
	Language                  string               `json:"language,omitempty"`
	Role                      SubtitleSemanticRole `json:"role,omitempty"`
	Mode                      string               `json:"mode"`
	Replace                   bool                 `json:"replace"`
	OverrideExistingASSStyles bool                 `json:"overrideExistingASSStyles"`
	StreamIndex               *int                 `json:"streamIndex,omitempty"`
	OCRLanguage               string               `json:"ocrLanguage,omitempty"`
	OCRMode                   string               `json:"ocrMode,omitempty"`
}

type subtitleRestyleRequest struct {
	Mode           string
	SourcePath     string
	SourceRevision string
	TargetPath     string
	Representation string
	Role           SubtitleSemanticRole
	Replace        bool
	Policy         SubtitleStylePolicy
}

// Tests replace this with a deterministic barrier to prove cancellation is
// signalled before the shared publication mutex is acquired.
var subtitlePublicationBeforeLockHook func()

func (h AssetHandler) RestyleSubtitle(c *gin.Context) {
	var input subtitleRestyleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(input.Path)
	if err != nil {
		writeSubtitleAssetResolutionError(c, err)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "restyle"
	}
	if mode == "regenerate" {
		if input.StreamIndex == nil || *input.StreamIndex < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "regenerate requires an exact embedded stream index"})
			return
		}
		sourceRevision := strings.TrimSpace(input.SourceRevision)
		if sourceRevision == "" {
			sourceRevision, err = mediaFileFingerprint(mediaPath)
		}
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "the exact embedded subtitle source revision is unavailable"})
			return
		}
		fingerprint, fingerprintErr := mediaFileFingerprint(mediaPath)
		if fingerprintErr != nil || fingerprint != sourceRevision {
			c.JSON(http.StatusConflict, gin.H{"error": "the selected media source is stale"})
			return
		}
		inputFormat := "ass"
		inputStyle := subtitleStylingPolicyFromDB(h.db)
		extraction := SubtitleExtractionInput{StreamIndex: input.StreamIndex, Format: inputFormat, OCRLanguage: input.OCRLanguage, OCRMode: input.OCRMode, SubtitleStyling: inputStyle, SourceRevision: sourceRevision}
		probe, _, probeErr := runFFProbe(mediaPath, 20)
		if probeErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": probeErr.Error()})
			return
		}
		plans, unsupported := subtitleExtractionPlansForRequest(mediaPath, probe.Streams, extraction)
		bitmapStreams := selectedBitmapSubtitleStreams(probe.Streams, input.StreamIndex)
		if len(plans) == 0 && len(bitmapStreams) == 0 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the exact embedded subtitle source is unavailable", "unsupported": unsupported})
			return
		}
		h.startSubtitleExtractionOperation(c, mediaPath, mediaPath, plans, bitmapStreams, extraction)
		return
	}

	sourcePath := filepath.Clean(strings.TrimSpace(input.SourcePath))
	if sourcePath == "." || sourcePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sourcePath is required"})
		return
	}
	validated, validationErr := validatedExternalSubtitlePath(mediaPath, sourcePath)
	if validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}
	info, statErr := os.Stat(validated)
	if statErr != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "the exact subtitle source is unavailable"})
		return
	}
	fingerprint, fingerprintErr := mediaFileFingerprint(validated)
	if strings.TrimSpace(input.SourceRevision) == "" || fingerprintErr != nil || fingerprint != strings.TrimSpace(input.SourceRevision) {
		c.JSON(http.StatusConflict, gin.H{"error": "the selected subtitle source is stale"})
		return
	}
	representation := strings.ToLower(strings.TrimSpace(input.Representation))
	if representation == "" {
		representation = strings.TrimPrefix(strings.ToLower(filepath.Ext(validated)), ".")
	}
	if representation != "srt" && representation != "ass" && representation != "ssa" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "representation must be srt, ass, or ssa"})
		return
	}
	if mode != "restyle" && mode != "srt_to_ass" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be restyle, srt_to_ass, or regenerate"})
		return
	}
	if mode == "restyle" && representation == "srt" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SRT uses text/timing only; use srt_to_ass to create an ASS sibling"})
		return
	}
	if mode == "srt_to_ass" && representation != "srt" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "srt_to_ass requires an SRT source"})
		return
	}
	policy := subtitleStylingPolicyFromDB(h.db)
	if input.OverrideExistingASSStyles {
		if !policy.Settings.OverrideExistingASSStyles {
			c.JSON(http.StatusConflict, gin.H{"error": "native ASS override is disabled in Subtitle Settings"})
			return
		}
		policy.Settings.OverrideExistingASSStyles = true
	}
	target := subtitleRestyleSiblingPath(validated, representation, mode, input.Replace)
	request := subtitleRestyleRequest{Mode: mode, SourcePath: validated, SourceRevision: input.SourceRevision, TargetPath: target, Representation: representation, Role: normalizeSubtitleSemanticRole(string(input.Role)), Replace: input.Replace, Policy: policy}
	operationID, startErr := startSubtitleRestyleOperation(request)
	if startErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": startErr.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"operationId": operationID, "status": "running", "phase": "preparing", "progress": 1, "format": representation})
}

func startSubtitleRestyleOperation(request subtitleRestyleRequest) (string, error) {
	workspace, err := os.MkdirTemp(filepath.Dir(request.SourcePath), ".mvforge-subtitle-operation-*")
	if err != nil {
		return "", err
	}
	operationID := fmt.Sprintf("subtitle-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	snapshot := request
	snapshot.Policy = cloneSubtitleStylePolicy(request.Policy)
	operation := &subtitleExtractionOperation{
		ID: operationID, Kind: "restyle", Status: "running", Phase: "preparing", Progress: 1,
		StreamIndex: -1, Format: request.Representation, AssetPath: request.SourcePath,
		StylePolicyVersion: request.Policy.Version, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		restyleRequest: &snapshot, cancel: cancel, publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	go func() {
		defer cancel()
		runSubtitleRestyleOperation(workCtx, operationID, workspace, snapshot)
	}()
	return operationID, nil
}

func subtitleRestyleSiblingPath(source, representation, mode string, replace bool) string {
	if replace && mode == "restyle" {
		return source
	}
	ext := "." + representation
	base := strings.TrimSuffix(source, filepath.Ext(source))
	if mode == "srt_to_ass" {
		ext = ".ass"
	}
	candidate := base + ".styled" + ext
	for index := 2; ; index++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = fmt.Sprintf("%s.styled.%d%s", base, index, ext)
	}
}

func runSubtitleRestyleOperation(ctx context.Context, operationID, workspace string, request subtitleRestyleRequest) {
	defer cleanupSubtitleOperationWorkspace(workspace)
	stagePath := filepath.Join(workspace, "subtitle-output."+request.Representation)
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Phase = "extracting"
		operation.Progress = 30
		operation.Message = "Reading exact subtitle source"
	})
	content, err := os.ReadFile(request.SourcePath)
	if err == nil && request.Mode == "restyle" {
		content, err = restyleSourcePreservingASS(content, request.Representation, request.Policy, request.Role, request.SourceRevision)
	}
	if err == nil && request.Mode == "srt_to_ass" {
		content, err = styledASSFromCanonicalSRTWithRole(content, request.Policy, request.Role)
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		if err = os.WriteFile(stagePath, content, 0o600); err == nil {
			staged, statErr := os.Stat(stagePath)
			if statErr != nil || staged.IsDir() || staged.Size() == 0 || !subtitleRestyleValidSidecar(request.RepresentationForValidation(), content) {
				err = errors.New("restyled subtitle failed validation")
			}
		}
	}
	if err == nil {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Phase = "publishing"
			operation.Progress = 80
			operation.Message = "Publishing staged subtitle sibling"
		})
		err = publishSubtitleRestyleStage(ctx, operationID, workspace, request, stagePath)
	}
	if err != nil {
		status := "error"
		phase := "error"
		message := "Subtitle restyle failed"
		if errors.Is(err, context.Canceled) {
			status, phase, message = "cancelled", "cancelled", "Subtitle restyle cancelled"
		}
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = status
			operation.Retryable = true
			operation.Phase = phase
			operation.Message = message
			operation.Error = err.Error()
		})
		return
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = "completed"
		operation.Retryable = false
		operation.Phase = "completed"
		operation.Progress = 100
		operation.Message = "Subtitle restyle completed"
		operation.Result = &SubtitleExtractionResult{Created: []string{request.TargetPath}, Existing: []string{}, Unsupported: []string{}, Artifacts: []SubtitleArtifact{{Type: "subtitle_sidecar", Format: request.Representation, Mode: request.Mode, StagedPath: request.TargetPath, PublishedPath: request.TargetPath, Status: "published"}}}
	})
}

func subtitleRestyleValidSidecar(representation string, content []byte) bool {
	if strings.EqualFold(strings.TrimSpace(representation), "ssa") {
		return validOriginalSubtitleSidecar("ssa", content)
	}
	return validSubtitleSidecar(representation, content)
}

func (request subtitleRestyleRequest) RepresentationForValidation() string {
	if request.Mode == "srt_to_ass" {
		return "ass"
	}
	return request.Representation
}

func publishSubtitleRestyleStage(ctx context.Context, operationID, workspace string, request subtitleRestyleRequest, stagePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fingerprint, err := mediaFileFingerprint(request.SourcePath)
	if err != nil || fingerprint != request.SourceRevision {
		return fmt.Errorf("subtitle source became stale before publication")
	}
	if subtitlePublicationBeforeLockHook != nil {
		subtitlePublicationBeforeLockHook()
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	fingerprint, err = mediaFileFingerprint(request.SourcePath)
	if err != nil || fingerprint != request.SourceRevision {
		return fmt.Errorf("subtitle source became stale before publication")
	}
	if !request.Replace {
		if _, err := os.Stat(request.TargetPath); err == nil {
			return fmt.Errorf("styled subtitle sibling appeared before publication")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	backupPath := filepath.Join(workspace, "replace-backup")
	targetExisted := false
	rollback := func() {
		if targetExisted {
			_ = copyPublishedFile(backupPath, request.TargetPath, true)
			return
		}
		_ = os.Remove(request.TargetPath)
	}
	if request.Replace {
		if _, statErr := os.Stat(request.TargetPath); statErr == nil {
			targetExisted = true
			if err := copyPublishedFile(request.TargetPath, backupPath, true); err != nil {
				return err
			}
		}
	}
	if err := publishStagedSubtitleFile(stagePath, request.TargetPath); err != nil {
		rollback()
		return err
	}
	if err := ctx.Err(); err != nil {
		rollback()
		return err
	}
	markSubtitlePublicationCommitted(operationID)
	return nil
}
