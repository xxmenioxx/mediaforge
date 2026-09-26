package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	subtitleOperationTimeout = 2 * time.Hour
	subtitleCleanupTimeout   = 10 * time.Second
	subtitleProcessGrace     = 2 * time.Second
)

var subtitlePublicationMu sync.Mutex

type subtitleExtractionTarget struct {
	plan           subtitleExtractionPlan
	stream         FFProbeStream
	finalPath      string
	stagePath      string
	artifact       SubtitleArtifact
	styling        SubtitleStylePolicy
	sourceRevision string
}

func (h AssetHandler) startSubtitleExtractionOperation(
	c *gin.Context,
	sourceMediaPath string,
	destinationMediaPath string,
	streamPlans []subtitleExtractionPlan,
	bitmapStreams []FFProbeStream,
	input SubtitleExtractionInput,
) {
	result, targets, err := prepareSubtitleExtractionTargets(destinationMediaPath, streamPlans, bitmapStreams, input)
	if err != nil {
		c.JSON(500, map[string]string{"error": err.Error()})
		return
	}
	if len(targets) == 0 {
		c.JSON(200, result)
		return
	}

	workspace, err := os.MkdirTemp(filepath.Dir(destinationMediaPath), ".mvforge-subtitle-operation-*")
	if err != nil {
		c.JSON(500, map[string]string{"error": fmt.Sprintf("cannot create subtitle staging workspace: %v", err)})
		return
	}
	for index := range targets {
		targets[index].stagePath = filepath.Join(workspace, filepath.Base(targets[index].finalPath))
	}

	operationID := fmt.Sprintf("subtitle-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	streamIndex := -1
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if len(bitmapStreams) > 0 {
		streamIndex = bitmapStreams[0].Index
	} else if len(targets) > 0 {
		streamIndex = targets[0].plan.StreamIndex
	}
	if format == "" && len(targets) > 0 {
		format = targets[0].artifact.Format
	}
	effectiveSettings := make([]ResolvedTrackSidecar, 0, len(targets))
	for _, target := range targets {
		effectiveSettings = append(effectiveSettings, target.plan.Decision)
	}
	now := time.Now()
	operation := &subtitleExtractionOperation{
		ID:                     operationID,
		Kind:                   "extraction",
		Status:                 "running",
		Phase:                  "preparing",
		Progress:               1,
		StreamIndex:            streamIndex,
		Format:                 format,
		AssetPath:              sourceMediaPath,
		StylePolicyVersion:     input.SubtitleStyling.Version,
		EffectiveSettings:      effectiveSettings,
		Message:                "Preparing subtitle conversion",
		CreatedAt:              now,
		UpdatedAt:              now,
		cancel:                 cancel,
		publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()

	go func() {
		defer cancel()
		runSubtitleExtractionOperation(workCtx, operationID, workspace, sourceMediaPath, result, targets)
	}()
	c.JSON(202, map[string]any{
		"operationId":       operationID,
		"status":            "running",
		"phase":             "preparing",
		"progress":          1,
		"streamIndex":       streamIndex,
		"format":            format,
		"effectiveSettings": effectiveSettings,
	})
}

func prepareSubtitleExtractionTargets(
	destinationMediaPath string,
	streamPlans []subtitleExtractionPlan,
	bitmapStreams []FFProbeStream,
	input SubtitleExtractionInput,
) (SubtitleExtractionResult, []subtitleExtractionTarget, error) {
	result := SubtitleExtractionResult{Created: []string{}, Existing: []string{}, Unsupported: []string{}}
	targets := make([]subtitleExtractionTarget, 0, len(streamPlans)+2)
	for _, plan := range streamPlans {
		stream := FFProbeStream{Index: plan.StreamIndex, CodecName: plan.Decision.Codec, CodecType: "subtitle", Tags: map[string]string{"language": plan.Decision.Language}}
		if plan.SourceStream.CodecType == "subtitle" {
			stream = plan.SourceStream
		}
		target := subtitleExtractionTarget{
			plan:           plan,
			stream:         stream,
			finalPath:      plan.OutputPath,
			artifact:       subtitleExtractionArtifact(plan, plan.OutputPath),
			styling:        input.SubtitleStyling,
			sourceRevision: input.SourceRevision,
		}
		if info, statErr := os.Stat(target.finalPath); statErr == nil && !info.IsDir() {
			target.artifact.Status = "existing"
			target.artifact.SizeBytes = info.Size()
			result.Existing = append(result.Existing, target.finalPath)
			result.Artifacts = append(result.Artifacts, target.artifact)
			continue
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return result, nil, fmt.Errorf("%s", mediaPathReadError(statErr))
		}
		targets = append(targets, target)
	}

	if len(bitmapStreams) > 0 {
		stream := bitmapStreams[0]
		format := strings.ToLower(strings.TrimSpace(input.Format))
		if format == "" {
			format = "srt"
		}
		requestedGroups := effectiveSubtitleSidecarFormats(stream.CodecName, []string{format}, input.GenerateSRTFallbackForASS)
		language := safeSubtitleFilenamePart(stream.Tags["language"])
		if language == "" {
			language = normalizedOCRLanguage(input.OCRLanguage, stream.Tags["language"])
		}
		roleStream := map[string]any{
			"index": stream.Index, "codec": stream.CodecName, "language": stream.Tags["language"],
			"title": stream.Tags["title"], "forced": stream.Disposition["forced"] == 1,
		}
		role := classifySubtitleRole(models.ScanResult{Path: destinationMediaPath, SubtitleStreams: models.JSONList{roleStream}}, roleStream, nil)
		sourceForced := stream.Disposition["forced"] == 1
		base := strings.TrimSuffix(destinationMediaPath, filepath.Ext(destinationMediaPath))
		makeBitmapPlan := func(outputFormat, requestedGroup, mode string) subtitleExtractionPlan {
			ocrLanguage, ocrMode := "", ""
			if mode == "converted" {
				ocrLanguage, ocrMode = input.OCRLanguage, normalizedOCRMode(input.OCRMode)
			}
			return subtitleExtractionPlan{
				StreamIndex: stream.Index,
				Mode:        mode,
				Format:      outputFormat,
				OutputPath:  fmt.Sprintf("%s.%s.%d.%s", base, language, stream.Index, outputFormat),
				Decision: ResolvedTrackSidecar{
					StreamIndex: stream.Index, Codec: strings.ToLower(stream.CodecName), SourceType: "bitmap",
					Language: language, Format: outputFormat, RequestedFormat: requestedGroup, Mode: mode,
					Forced: sourceForced || role.Role == SubtitleRoleForced, SourceForced: sourceForced,
					Role: role.Role, RoleProvenance: role.Provenance, RoleConfidence: role.Confidence, RoleClassifierVersion: role.ClassifierVersion,
					OCRRequired: mode == "converted", OCRLanguage: ocrLanguage, OCRMode: ocrMode,
					Stages: subtitleArtifactStages("bitmap", mode), Extension: "." + outputFormat,
					NamingKey:           fmt.Sprintf("subtitle:%d:%s:%s", stream.Index, mode, outputFormat),
					LogicalTransformKey: fmt.Sprintf("subtitle:%d:%s:%s", stream.Index, mode, requestedGroup),
				},
			}
		}
		appendTarget := func(plan subtitleExtractionPlan) error {
			target := subtitleExtractionTarget{
				plan: plan, stream: stream, finalPath: plan.OutputPath, styling: input.SubtitleStyling, sourceRevision: input.SourceRevision,
				artifact: subtitleExtractionArtifact(plan, plan.OutputPath),
			}
			if info, statErr := os.Stat(target.finalPath); statErr == nil && !info.IsDir() {
				target.artifact.Status = "existing"
				target.artifact.SizeBytes = info.Size()
				result.Existing = append(result.Existing, target.finalPath)
				result.Artifacts = append(result.Artifacts, target.artifact)
				return nil
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return fmt.Errorf("%s", mediaPathReadError(statErr))
			}
			targets = append(targets, target)
			return nil
		}
		if format == "original" {
			originalFormat, mode, err := resolveSubtitleSidecarFormat(stream.CodecName, "original")
			if err != nil {
				result.Unsupported = append(result.Unsupported, fmt.Sprintf("stream %d (%s)", stream.Index, stream.CodecName))
				return result, targets, nil
			}
			if err := appendTarget(makeBitmapPlan(originalFormat, "original", mode)); err != nil {
				return result, nil, err
			}
			return result, targets, nil
		}
		for _, requestedGroup := range requestedGroups {
			outputFormats := []string{requestedGroup}
			if requestedGroup == "both" {
				outputFormats = []string{"srt", "ass"}
			}
			for _, outputFormat := range outputFormats {
				if err := appendTarget(makeBitmapPlan(outputFormat, requestedGroup, "converted")); err != nil {
					return result, nil, err
				}
			}
		}
	}
	return result, targets, nil
}

func subtitleExtractionArtifact(plan subtitleExtractionPlan, path string) SubtitleArtifact {
	decision := plan.Decision
	return SubtitleArtifact{
		ArtifactID: subtitleArtifactID(plan.StreamIndex, decision.Mode, decision.Format),
		Type:       "subtitle_sidecar", StreamIndex: plan.StreamIndex, SourceCodec: decision.Codec,
		SourceType: decision.SourceType, Format: decision.Format, RequestedFormat: decision.RequestedFormat,
		Mode: decision.Mode, Language: decision.Language, Default: decision.Default, Forced: decision.Forced, SourceForced: decision.SourceForced,
		Role: decision.Role, RoleProvenance: decision.RoleProvenance, RoleConfidence: decision.RoleConfidence, RoleClassifierVersion: decision.RoleClassifierVersion, OCRRequired: decision.OCRRequired,
		OCRLanguage: decision.OCRLanguage, OCRMode: decision.OCRMode, Stages: append([]string(nil), decision.Stages...),
		Extension: decision.Extension, NamingKey: decision.NamingKey, LogicalTransformKey: decision.LogicalTransformKey,
		StagedPath: path, Status: "planned",
	}
}

func runSubtitleExtractionOperation(
	ctx context.Context,
	operationID string,
	workspace string,
	sourceMediaPath string,
	initialResult SubtitleExtractionResult,
	targets []subtitleExtractionTarget,
) {
	created, createdPaths, err := executeSubtitleExtractionTargets(ctx, operationID, workspace, sourceMediaPath, targets)
	if err == nil && ctx.Err() == nil {
		if len(targets) > 0 && targets[0].sourceRevision != "" {
			fingerprint, fingerprintErr := mediaFileFingerprint(sourceMediaPath)
			if fingerprintErr != nil || fingerprint != targets[0].sourceRevision {
				err = fmt.Errorf("subtitle source became stale before publication")
			}
		}
	}
	if err == nil && ctx.Err() == nil {
		created, createdPaths, err = publishSubtitleExtractionTargets(operationID, ctx, targets, created, createdPaths)
	}
	cleanupErr := cleanupSubtitleOperationWorkspace(workspace)

	if subtitleOperationPublicationCommitted(operationID) {
		finalResult := initialResult
		finalResult.Created = append(finalResult.Created, createdPaths...)
		for _, artifact := range created {
			finalResult.Artifacts = append(finalResult.Artifacts, artifact)
		}
		updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
			op.Status = "completed"
			op.Phase = "completed"
			op.Progress = 100
			op.Message = "Subtitle conversion completed"
			op.Result = &finalResult
		})
		return
	}

	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		message := "Subtitle conversion cancelled"
		if cleanupErr != nil {
			message += "; cleanup diagnostic: " + cleanupErr.Error()
		}
		updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
			op.Status = "cancelled"
			op.Phase = "cleanup"
			op.Message = message
			op.Error = ""
		})
		return
	}
	if err != nil {
		message := err.Error()
		if cleanupErr != nil {
			message += "; cleanup diagnostic: " + cleanupErr.Error()
		}
		updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
			op.Status = "error"
			op.Phase = "error"
			op.Message = "Subtitle conversion failed"
			op.Error = message
		})
		return
	}

	finalResult := initialResult
	finalResult.Created = append(finalResult.Created, createdPaths...)
	for _, artifact := range created {
		finalResult.Artifacts = append(finalResult.Artifacts, artifact)
	}
	updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
		op.Status = "completed"
		op.Phase = "completed"
		op.Progress = 100
		op.Message = "Subtitle conversion completed"
		op.Result = &finalResult
	})
}

func executeSubtitleExtractionTargets(
	ctx context.Context,
	operationID string,
	workspace string,
	sourceMediaPath string,
	targets []subtitleExtractionTarget,
) ([]SubtitleArtifact, []string, error) {
	created := make([]SubtitleArtifact, 0, len(targets))
	createdPaths := make([]string, 0, len(targets))
	processedBoth := map[string]bool{}
	for index := range targets {
		if err := ctx.Err(); err != nil {
			return created, createdPaths, err
		}
		target := &targets[index]
		if strings.EqualFold(target.plan.Decision.RequestedFormat, "both") && strings.EqualFold(target.plan.Mode, "converted") {
			key := target.plan.Decision.LogicalTransformKey
			if !processedBoth[key] {
				group := make([]*subtitleExtractionTarget, 0, 2)
				for candidate := range targets {
					if targets[candidate].plan.Decision.LogicalTransformKey == key {
						group = append(group, &targets[candidate])
					}
				}
				groupCreated, err := executeBothSubtitleTargets(ctx, operationID, workspace, sourceMediaPath, group)
				created = append(created, groupCreated...)
				processedBoth[key] = true
				if err != nil {
					return created, createdPaths, err
				}
			}
			continue
		}
		artifact, err := executeSingleSubtitleTarget(ctx, operationID, workspace, sourceMediaPath, target)
		if err != nil {
			return created, createdPaths, err
		}
		created = append(created, artifact)
		createdPaths = append(createdPaths, artifact.StagedPath)
	}
	return created, createdPaths, nil
}

func executeSingleSubtitleTarget(
	ctx context.Context,
	operationID string,
	workspace string,
	sourceMediaPath string,
	target *subtitleExtractionTarget,
) (SubtitleArtifact, error) {
	updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
		op.Phase = "extracting"
		op.Progress = 20
		op.Message = "Extracting subtitle"
	})
	plan := MediaJobPlan{InputPath: sourceMediaPath}
	var args []string
	generatedASS := false
	if target.plan.Mode == "original" {
		sourceCodec := strings.TrimSpace(target.plan.Decision.Codec)
		if sourceCodec == "" {
			sourceCodec = target.stream.CodecName
		}
		_, muxer, supported := originalSubtitleExtractionFormat(sourceCodec)
		if !supported || strings.TrimSpace(muxer) == "" {
			return target.artifact, fmt.Errorf("subtitle stream %d codec %s does not support original extraction", target.plan.StreamIndex, sourceCodec)
		}
		args = originalSubtitleExtractionArgs(plan, target.plan.StreamIndex, muxer, target.stagePath)
	} else if isBitmapSubtitleCodecName(target.stream.CodecName) {
		updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
			op.Phase = "ocr"
			op.Progress = 20
			op.Message = "Running OCR"
		})
		ocrResult, err := generateBitmapSubtitleAtPathWithResult(ctx, sourceMediaPath, target.stream, target.plan.Format, target.plan.Decision.OCRLanguage, target.plan.Decision.OCRMode, target.stagePath, 0, 0, subtitleOCRProgress(operationID), subtitleStylePolicyWithRole(target.styling, target.plan.Decision.Role))
		if err != nil {
			return target.artifact, err
		}
		target.artifact.EffectiveOCRLanguage = ocrResult.EffectiveLanguage
		target.artifact.CanonicalEvidence = ocrResult.Evidence
		return validateStagedSubtitleArtifact(target)
	} else {
		if strings.EqualFold(target.plan.Format, "ass") && target.styling.Settings.Enabled && target.styling.Settings.ApplyToGeneratedASS {
			if err := renderTextSubtitleASSFromSource(ctx, sourceMediaPath, target.plan.StreamIndex, target.stagePath, target.styling, target.plan.Decision.Role); err != nil {
				return target.artifact, err
			}
			generatedASS = true
		} else {
			args = textSubtitleExtractionArgs(plan, target.plan.StreamIndex, target.plan.Format, target.stagePath)
		}
	}
	if !generatedASS {
		var stderr bytes.Buffer
		if err := runSubtitleCommand(ctx, "ffmpeg", args, nil, &stderr); err != nil {
			return target.artifact, fmt.Errorf("subtitle stream %d extraction failed: %s", target.plan.StreamIndex, fallback(strings.TrimSpace(stderr.String()), err.Error()))
		}
	}
	return validateStagedSubtitleArtifact(target)
}

func executeBothSubtitleTargets(
	ctx context.Context,
	operationID string,
	workspace string,
	sourceMediaPath string,
	targets []*subtitleExtractionTarget,
) ([]SubtitleArtifact, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
		op.Phase = "extracting"
		op.Progress = 20
		op.Message = "Generating shared subtitle conversion"
	})
	canonicalPath := filepath.Join(workspace, "canonical.srt")
	stream := targets[0].stream
	var bitmapOCR bitmapOCRResult
	if isBitmapSubtitleCodecName(stream.CodecName) {
		updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
			op.Phase = "ocr"
			op.Progress = 20
			op.Message = "Running OCR"
		})
		ocrResult, err := generateBitmapSubtitleAtPathWithResult(ctx, sourceMediaPath, stream, "srt", targets[0].plan.Decision.OCRLanguage, targets[0].plan.Decision.OCRMode, canonicalPath, 0, 0, subtitleOCRProgress(operationID), subtitleStylePolicyWithRole(targets[0].styling, targets[0].plan.Decision.Role))
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			target.artifact.EffectiveOCRLanguage = ocrResult.EffectiveLanguage
			target.artifact.CanonicalEvidence = ocrResult.Evidence
		}
		bitmapOCR = bitmapOCRResult{Cues: ocrResult.Cues, Geometry: ocrResult.Geometry}
	} else {
		args := textSubtitleExtractionArgs(MediaJobPlan{InputPath: sourceMediaPath}, stream.Index, "srt", canonicalPath)
		var stderr bytes.Buffer
		if err := runSubtitleCommand(ctx, "ffmpeg", args, nil, &stderr); err != nil {
			return nil, fmt.Errorf("subtitle stream %d extraction failed: %s", stream.Index, fallback(strings.TrimSpace(stderr.String()), err.Error()))
		}
	}
	canonicalContent, err := os.ReadFile(canonicalPath)
	if err != nil || !validSubtitleSidecar("srt", canonicalContent) {
		return nil, fmt.Errorf("subtitle stream %d produced an invalid canonical SRT sidecar", stream.Index)
	}

	created := make([]SubtitleArtifact, 0, len(targets))
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return created, err
		}
		if target.plan.Format == "srt" {
			if err := os.WriteFile(target.stagePath, canonicalContent, 0o644); err != nil {
				return created, err
			}
		} else {
			updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
				op.Phase = "extracting"
				op.Progress = 60
				op.Message = "Rendering ASS from shared subtitle result"
			})
			var stderr bytes.Buffer
			if len(bitmapOCR.Cues) > 0 && (len(bitmapOCR.Geometry) > 0 || (target.styling.Settings.Enabled && target.styling.Settings.ApplyToGeneratedASS)) {
				if err := os.WriteFile(target.stagePath, renderBitmapASSCuesWithPolicyAndRole(bitmapOCR.Cues, target.styling, target.plan.Decision.Role), 0o644); err != nil {
					return created, err
				}
			} else if target.styling.Settings.Enabled && target.styling.Settings.ApplyToGeneratedASS {
				styled, styleErr := styledASSFromCanonicalSRTWithRole(canonicalContent, target.styling, target.plan.Decision.Role)
				if styleErr != nil {
					return created, styleErr
				}
				if err := os.WriteFile(target.stagePath, styled, 0o644); err != nil {
					return created, err
				}
			} else if err := runSubtitleCommand(ctx, "ffmpeg", subtitleCueRenderArgs(canonicalPath, "ass", target.stagePath), nil, &stderr); err != nil {
				return created, fmt.Errorf("subtitle stream %d ASS render failed: %s", stream.Index, fallback(strings.TrimSpace(stderr.String()), err.Error()))
			}
		}
		artifact, err := validateStagedSubtitleArtifact(target)
		if err != nil {
			return created, err
		}
		created = append(created, artifact)
	}
	return created, nil
}

func subtitleOCRProgress(operationID string) OCRProgressFunc {
	return func(processed, total int) {
		updateSubtitleExtractionOperation(operationID, func(op *subtitleExtractionOperation) {
			op.Phase = "ocr"
			op.Processed = processed
			op.Total = total
			op.Message = "Running OCR"
			if total > 0 {
				ratio := float64(processed) / float64(total)
				if ratio < 0 {
					ratio = 0
				}
				if ratio > 1 {
					ratio = 1
				}
				op.Progress = 20 + ratio*60
			}
		})
	}
}

func validateStagedSubtitleArtifact(target *subtitleExtractionTarget) (SubtitleArtifact, error) {
	content, readErr := os.ReadFile(target.stagePath)
	info, statErr := os.Stat(target.stagePath)
	if readErr != nil || statErr != nil || info.IsDir() || info.Size() == 0 {
		return target.artifact, fmt.Errorf("subtitle stream %d produced an empty %s sidecar", target.plan.StreamIndex, strings.ToUpper(target.plan.Format))
	}
	valid := validSubtitleSidecar(target.plan.Format, content)
	if target.plan.Mode == "original" {
		valid = validOriginalSubtitleSidecar(target.plan.Format, content)
	}
	if !valid {
		return target.artifact, fmt.Errorf("subtitle stream %d produced an invalid %s sidecar", target.plan.StreamIndex, strings.ToUpper(target.plan.Format))
	}
	target.artifact.Status = "ready"
	target.artifact.SizeBytes = info.Size()
	target.artifact.StagedPath = target.finalPath
	return target.artifact, nil
}

func publishSubtitleExtractionTargets(
	operationID string,
	ctx context.Context,
	targets []subtitleExtractionTarget,
	created []SubtitleArtifact,
	createdPaths []string,
) ([]SubtitleArtifact, []string, error) {
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return created, nil, err
	}
	published := make([]string, 0, len(createdPaths))
	createdByPath := map[string]SubtitleArtifact{}
	for _, artifact := range created {
		createdByPath[artifact.StagedPath] = artifact
	}
	for index := range targets {
		target := &targets[index]
		outputs := []struct{ stage, final string }{}
		if _, exists := createdByPath[target.finalPath]; exists {
			outputs = append(outputs, struct{ stage, final string }{target.stagePath, target.finalPath})
		}
		for _, output := range outputs {
			if info, err := os.Stat(output.final); err == nil && !info.IsDir() {
				continue
			} else if err != nil && !os.IsNotExist(err) {
				rollbackSubtitlePublication(published)
				return created, nil, err
			}
			if err := os.Link(output.stage, output.final); err != nil {
				if os.IsExist(err) {
					continue
				}
				rollbackSubtitlePublication(published)
				return created, nil, fmt.Errorf("cannot publish extracted subtitle: %w", err)
			}
			published = append(published, output.final)
		}
	}
	for _, target := range targets {
		artifact, exists := createdByPath[target.finalPath]
		if !exists || artifact.CanonicalEvidence == nil {
			continue
		}
		if err := commitPublishedCanonicalOCRSnapshot(ctx, target.finalPath, target.plan.Format, target.plan.Decision.LogicalTransformKey, artifact.CanonicalEvidence, operationID, ""); err != nil {
			rollbackSubtitlePublication(published)
			return created, nil, fmt.Errorf("cannot commit published OCR baseline: %w", err)
		}
	}
	markSubtitlePublicationCommitted(operationID)
	return created, published, nil
}

func rollbackSubtitlePublication(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func cleanupSubtitleOperationWorkspace(workspace string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), subtitleCleanupTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- os.RemoveAll(workspace) }()
	select {
	case err := <-done:
		return err
	case <-cleanupCtx.Done():
		return fmt.Errorf("subtitle cleanup timed out")
	}
}

func markSubtitlePublicationCommitted(operationID string) {
	subtitleExtractionOperations.Lock()
	defer subtitleExtractionOperations.Unlock()
	operation, exists := subtitleExtractionOperations.items[operationID]
	if !exists || operation.publicationCommitted {
		return
	}
	operation.publicationCommitted = true
	if operation.Translation != nil {
		operation.Translation.Cancellable = false
	}
	if operation.publicationCommittedCh != nil {
		close(operation.publicationCommittedCh)
	}
}

func subtitleOperationPublicationCommitted(operationID string) bool {
	subtitleExtractionOperations.RLock()
	defer subtitleExtractionOperations.RUnlock()
	operation, exists := subtitleExtractionOperations.items[operationID]
	return exists && operation.publicationCommitted
}

func (h *AssetHandler) CancelSubtitleExtractionOperation(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(400, gin.H{"error": "operation id is required"})
		return
	}

	// Every subtitle publisher waits on subtitlePublicationMu and checks its
	// context before the final source recheck. Signal cancellation before taking
	// that mutex so a queued publisher cannot pass the barrier merely because
	// the cancel request was waiting behind it.
	subtitleExtractionOperations.RLock()
	if operation, exists := subtitleExtractionOperations.items[id]; exists && operation.Status == "running" && !operation.publicationCommitted && operation.cancel != nil {
		operation.cancel()
	}
	subtitleExtractionOperations.RUnlock()

	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	subtitleExtractionOperations.Lock()
	operation, exists := subtitleExtractionOperations.items[id]
	if !exists {
		subtitleExtractionOperations.Unlock()
		c.JSON(404, gin.H{"error": "subtitle extraction operation not found"})
		return
	}
	if operation.Status == "running" && !operation.publicationCommitted && operation.cancel != nil {
		operation.cancel()
	}
	snapshot := cloneSubtitleExtractionOperationSnapshot(operation)
	subtitleExtractionOperations.Unlock()

	if snapshot.Status == "running" {
		c.JSON(202, snapshot)
		return
	}
	c.JSON(200, snapshot)
}

func runSubtitleCommand(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	configureSubtitleProcessGroup(cmd)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	return waitSubtitleProcess(ctx, cmd, waitDone)
}

func waitSubtitleProcess(ctx context.Context, cmd *exec.Cmd, waitDone <-chan error) error {
	select {
	case err := <-waitDone:
		return err
	case <-ctx.Done():
		terminateSubtitleProcessTree(cmd)
		err := <-waitDone
		if err == nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ctx.Err(), err)
	}
}

func configureSubtitleProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateSubtitleProcessTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	timer := time.NewTimer(subtitleProcessGrace)
	defer timer.Stop()
	<-timer.C
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
