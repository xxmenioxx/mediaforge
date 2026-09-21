package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type subtitleTranslationPublicationTarget struct {
	Format    string
	StagePath string
	FinalPath string
	Artifact  SubtitleArtifact
}

type subtitleTranslationPublicationRequest struct {
	Ctx                   context.Context
	OperationID           string
	Workspace             string
	SourceRevision        translationSourceRevision
	CurrentEmbeddedSource func() (translationEmbeddedSourceSnapshot, error)
	RecheckSource         func(translationSourceRevision) error
	Targets               []subtitleTranslationPublicationTarget
	CommitMetadata        func([]SubtitleArtifact) error
	RollbackMetadata      func([]SubtitleArtifact) error
	PublishStage          func(string, string) error
}

func validTranslatedSubtitleSidecar(format string, content []byte) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case translationRepresentationSRT, translationRepresentationASS:
		return validSubtitleSidecar(strings.ToLower(strings.TrimSpace(format)), content)
	case translationRepresentationSSA:
		return validOriginalSubtitleSidecar(translationRepresentationSSA, content)
	default:
		return false
	}
}

// stageSubtitleTranslationOutputs writes an already validated translation
// result into the operation workspace. It requires an exact output set so a
// later publication cannot accidentally publish a partial logical result.
func stageSubtitleTranslationOutputs(
	workspace string,
	outputs map[string][]byte,
	targets []subtitleTranslationPublicationTarget,
) ([]subtitleTranslationPublicationTarget, error) {
	workspace = filepath.Clean(strings.TrimSpace(workspace))
	if workspace == "." || workspace == "" {
		return nil, errors.New("translation staging workspace is required")
	}
	if len(outputs) == 0 || len(outputs) != len(targets) {
		return nil, errors.New("translation staging set is incomplete")
	}

	normalizedOutputs := make(map[string][]byte, len(outputs))
	for format, content := range outputs {
		normalized := strings.ToLower(strings.TrimSpace(format))
		if normalized == "" || normalizedOutputs[normalized] != nil {
			return nil, fmt.Errorf("translation staging contains duplicate or empty format %q", format)
		}
		if len(content) == 0 || int64(len(content)) > maxPublishedSubtitleBytes || !validTranslatedSubtitleSidecar(normalized, content) {
			return nil, fmt.Errorf("translated %s output is empty, oversized, or invalid", normalized)
		}
		normalizedOutputs[normalized] = append([]byte(nil), content...)
	}

	result := append([]subtitleTranslationPublicationTarget(nil), targets...)
	seen := make(map[string]struct{}, len(result))
	for index := range result {
		format := strings.ToLower(strings.TrimSpace(result[index].Format))
		if format == "" {
			return nil, errors.New("translation publication target format is required")
		}
		if _, exists := seen[format]; exists {
			return nil, fmt.Errorf("translation publication contains duplicate format %q", format)
		}
		if strings.ToLower(strings.TrimSpace(result[index].Artifact.Format)) != format {
			return nil, fmt.Errorf("translation %s artifact format does not match publication target", format)
		}
		seen[format] = struct{}{}
		content, exists := normalizedOutputs[format]
		if !exists {
			return nil, fmt.Errorf("translation staging is missing %s output", format)
		}
		if strings.TrimSpace(result[index].FinalPath) == "" {
			return nil, fmt.Errorf("translation %s destination is required", format)
		}
		stagePath := filepath.Join(workspace, fmt.Sprintf("translation-output-%s.%s", format, format))
		result[index].Format = format
		result[index].StagePath = stagePath
		result[index].Artifact.StagedPath = stagePath
		result[index].Artifact.Status = "ready"
		result[index].Artifact.SizeBytes = int64(len(content))
	}
	if len(seen) != len(normalizedOutputs) {
		return nil, errors.New("translation staging contains an unexpected output")
	}
	for index := range result {
		if err := os.WriteFile(result[index].StagePath, normalizedOutputs[result[index].Format], 0o600); err != nil {
			for cleanupIndex := 0; cleanupIndex <= index; cleanupIndex++ {
				_ = os.Remove(result[cleanupIndex].StagePath)
			}
			return nil, fmt.Errorf("cannot stage translated %s output: %w", result[index].Format, err)
		}
	}
	return result, nil
}

func validateSubtitleTranslationPublicationRequest(request subtitleTranslationPublicationRequest) error {
	if request.Ctx == nil {
		return errors.New("translation publication context is required")
	}
	if strings.TrimSpace(request.OperationID) == "" {
		return errors.New("translation publication operation identity is required")
	}
	workspace := filepath.Clean(strings.TrimSpace(request.Workspace))
	if workspace == "." || workspace == "" {
		return errors.New("translation publication workspace is required")
	}
	if err := validateTranslationSourceRevision(request.SourceRevision); err != nil {
		return staleTranslationSourceError("captured source revision is invalid: %v", err)
	}
	if len(request.Targets) == 0 {
		return errors.New("translation publication requires at least one output")
	}
	sourcePath := translationSourceRevisionPath(request.SourceRevision)
	seenFormats := make(map[string]struct{}, len(request.Targets))
	seenDestinations := make(map[string]struct{}, len(request.Targets))
	for index, target := range request.Targets {
		format := strings.ToLower(strings.TrimSpace(target.Format))
		if format != translationRepresentationSRT && format != translationRepresentationASS && format != translationRepresentationSSA {
			return fmt.Errorf("unsupported translated publication format %q", target.Format)
		}
		if _, exists := seenFormats[format]; exists {
			return fmt.Errorf("translated publication contains duplicate format %q", format)
		}
		seenFormats[format] = struct{}{}
		stagePath := filepath.Clean(strings.TrimSpace(target.StagePath))
		relative, err := filepath.Rel(workspace, stagePath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("translated %s stage is outside the operation workspace", format)
		}
		info, err := os.Stat(stagePath)
		if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxPublishedSubtitleBytes {
			return fmt.Errorf("translated %s stage is missing or empty", format)
		}
		content, err := os.ReadFile(stagePath)
		if err != nil || !validTranslatedSubtitleSidecar(format, content) {
			return fmt.Errorf("translated %s stage failed subtitle validation", format)
		}
		finalPath := filepath.Clean(strings.TrimSpace(target.FinalPath))
		if finalPath == "." || finalPath == "" {
			return fmt.Errorf("translated %s destination is required", format)
		}
		if finalPath == sourcePath {
			return errors.New("translated publication cannot overwrite the source subtitle")
		}
		if _, exists := seenDestinations[finalPath]; exists {
			return fmt.Errorf("translated publication contains duplicate destination %q", finalPath)
		}
		seenDestinations[finalPath] = struct{}{}
		parentInfo, err := os.Stat(filepath.Dir(finalPath))
		if err != nil || !parentInfo.IsDir() {
			return fmt.Errorf("translated %s destination directory is unavailable", format)
		}
		if target.Artifact.Translation == nil {
			return fmt.Errorf("translated %s artifact metadata is missing", format)
		}
		if strings.ToLower(strings.TrimSpace(target.Artifact.Format)) != format {
			return fmt.Errorf("translated %s artifact format does not match publication target", format)
		}
		identity, err := translationIdentityFromMetadata(*target.Artifact.Translation)
		if err != nil {
			return fmt.Errorf("translated %s artifact metadata is invalid: %w", format, err)
		}
		if identity.OutputRepresentation != format || identity.SourceRevision != request.SourceRevision.key() || !translationArtifactMatchesIdentity(target.Artifact, identity) {
			return fmt.Errorf("translated %s artifact metadata does not match the captured source identity", format)
		}
		_ = index
	}
	return nil
}

func translationSourceRevisionPath(revision translationSourceRevision) string {
	switch revision.Kind {
	case translationSourceRevisionExternal:
		if revision.External != nil {
			return filepath.Clean(revision.External.ArtifactPath)
		}
	case translationSourceRevisionEmbedded:
		if revision.Embedded != nil {
			return filepath.Clean(revision.Embedded.SourcePath)
		}
	}
	return ""
}

func recheckSubtitleTranslationPublicationSource(request subtitleTranslationPublicationRequest) error {
	if request.RecheckSource != nil {
		return request.RecheckSource(request.SourceRevision)
	}
	switch request.SourceRevision.Kind {
	case translationSourceRevisionExternal:
		return recheckTranslationExternalSourceRevision(request.SourceRevision)
	case translationSourceRevisionEmbedded:
		if request.CurrentEmbeddedSource == nil {
			return staleTranslationSourceError("current embedded subtitle snapshot is unavailable")
		}
		current, err := request.CurrentEmbeddedSource()
		if err != nil {
			return staleTranslationSourceError("current embedded subtitle snapshot is unavailable: %v", err)
		}
		return recheckTranslationEmbeddedSourceRevision(request.SourceRevision, current)
	default:
		return staleTranslationSourceError("unsupported translation source revision")
	}
}

type subtitleTranslationRollbackEntry struct {
	FinalPath  string
	BackupPath string
	Existed    bool
}

// publishSubtitleTranslationOutputs holds the canonical subtitle publication
// mutex from final source recheck through metadata commit and publication
// barrier marking. It never calls a provider.
func publishSubtitleTranslationOutputs(request subtitleTranslationPublicationRequest) ([]SubtitleArtifact, error) {
	if err := validateSubtitleTranslationPublicationRequest(request); err != nil {
		return nil, err
	}
	if err := request.Ctx.Err(); err != nil {
		return nil, err
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := request.Ctx.Err(); err != nil {
		return nil, err
	}
	if err := recheckSubtitleTranslationPublicationSource(request); err != nil {
		return nil, err
	}
	if err := validateSubtitleTranslationPublicationRequest(request); err != nil {
		return nil, err
	}

	rollbackEntries := make([]subtitleTranslationRollbackEntry, len(request.Targets))
	for index, target := range request.Targets {
		finalPath := filepath.Clean(target.FinalPath)
		rollbackEntries[index] = subtitleTranslationRollbackEntry{FinalPath: finalPath}
		info, err := os.Stat(finalPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("cannot snapshot translated %s destination", target.Format)
		}
		backupPath := filepath.Join(request.Workspace, fmt.Sprintf(".translation-rollback-%d", index))
		if err := copyPublishedFile(finalPath, backupPath, true); err != nil {
			return nil, fmt.Errorf("cannot snapshot translated %s destination: %w", target.Format, err)
		}
		rollbackEntries[index].BackupPath = backupPath
		rollbackEntries[index].Existed = true
	}

	rollback := func() {
		for _, entry := range rollbackEntries {
			if entry.Existed {
				_ = copyPublishedFile(entry.BackupPath, entry.FinalPath, true)
			} else {
				_ = os.Remove(entry.FinalPath)
			}
		}
	}
	publishStage := request.PublishStage
	if publishStage == nil {
		publishStage = publishStagedSubtitleFile
	}
	publishedArtifacts := make([]SubtitleArtifact, len(request.Targets))
	for index, target := range request.Targets {
		if err := request.Ctx.Err(); err != nil {
			rollback()
			return nil, err
		}
		if err := publishStage(target.StagePath, target.FinalPath); err != nil {
			rollback()
			return nil, fmt.Errorf("cannot publish translated %s output: %w", target.Format, err)
		}
		publishedArtifacts[index] = target.Artifact
		publishedArtifacts[index].StagedPath = target.FinalPath
		publishedArtifacts[index].PublishedPath = target.FinalPath
		publishedArtifacts[index].Status = "published"
	}
	if request.Ctx.Err() != nil {
		rollback()
		return nil, request.Ctx.Err()
	}
	if request.CommitMetadata != nil {
		if err := request.CommitMetadata(publishedArtifacts); err != nil {
			rollback()
			if request.RollbackMetadata != nil {
				if rollbackErr := request.RollbackMetadata(publishedArtifacts); rollbackErr != nil {
					return nil, fmt.Errorf("cannot commit translated artifact metadata: %w; metadata rollback diagnostic: %s", err, boundedSubtitleTranslationError(rollbackErr))
				}
			}
			return nil, fmt.Errorf("cannot commit translated artifact metadata: %w", err)
		}
	}
	markSubtitlePublicationCommitted(request.OperationID)
	return publishedArtifacts, nil
}
