package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	subtitleSyncEngineName    = "manual"
	subtitleSyncEngineVersion = "fixed-offset-v1"
	maxSubtitleSyncOffsetMs   = int64(12 * 60 * 60 * 1000)
	maxSubtitleSyncPreview    = 12
)

type SubtitleSyncAudioReference struct {
	StreamIndex int    `json:"streamIndex"`
	StreamID    string `json:"streamId,omitempty"`
	Codec       string `json:"codec"`
	Language    string `json:"language,omitempty"`
	Title       string `json:"title,omitempty"`
}

type SubtitleSyncTiming struct {
	StartMilliseconds int64 `json:"startMilliseconds"`
	EndMilliseconds   int64 `json:"endMilliseconds"`
}

type SubtitleSyncCuePreview struct {
	CueIndex        int   `json:"cueIndex"`
	CurrentStartMs  int64 `json:"currentStartMs"`
	CurrentEndMs    int64 `json:"currentEndMs"`
	ProposedStartMs int64 `json:"proposedStartMs"`
	ProposedEndMs   int64 `json:"proposedEndMs"`
}

type SubtitleSyncProposal struct {
	ProposalID         string                     `json:"proposalId"`
	SubtitlePath       string                     `json:"subtitlePath"`
	SourceIdentity     string                     `json:"sourceIdentity"`
	SourceRevision     string                     `json:"sourceRevision"`
	Representation     string                     `json:"representation"`
	ReferenceAudio     SubtitleSyncAudioReference `json:"referenceAudio"`
	Engine             string                     `json:"engine"`
	EngineVersion      string                     `json:"engineVersion"`
	Model              string                     `json:"model"`
	OffsetMilliseconds int64                      `json:"offsetMilliseconds"`
	Scale              float64                    `json:"scale"`
	Segments           []SubtitleSyncSegment      `json:"segments"`
	QualityState       string                     `json:"qualityState"`
	AnalyzedAt         time.Time                  `json:"analyzedAt"`
	Diagnostics        []string                   `json:"diagnostics"`
	Preview            []SubtitleSyncCuePreview   `json:"preview"`
}

type SubtitleSyncSegment struct {
	StartMilliseconds  *int64  `json:"startMilliseconds,omitempty"`
	EndMilliseconds    *int64  `json:"endMilliseconds,omitempty"`
	OffsetMilliseconds int64   `json:"offsetMilliseconds"`
	Scale              float64 `json:"scale"`
}

type SubtitleSyncAnalysisInput struct {
	SubtitlePath       string
	SourceIdentity     string
	SourceRevision     string
	Representation     string
	ReferenceAudio     SubtitleSyncAudioReference
	OffsetMilliseconds int64
	Source             []byte
}

type SubtitleSyncEngine interface {
	Analyze(context.Context, SubtitleSyncAnalysisInput) (SubtitleSyncProposal, error)
	Apply(context.Context, []byte, SubtitleSyncProposal) ([]byte, error)
}

type fixedOffsetSubtitleSyncEngine struct{}

func (fixedOffsetSubtitleSyncEngine) Analyze(ctx context.Context, input SubtitleSyncAnalysisInput) (SubtitleSyncProposal, error) {
	if err := ctx.Err(); err != nil {
		return SubtitleSyncProposal{}, err
	}
	if strings.TrimSpace(input.SourceIdentity) == "" || strings.TrimSpace(input.SourceRevision) == "" {
		return SubtitleSyncProposal{}, errors.New("subtitle source identity and revision are required")
	}
	if input.ReferenceAudio.StreamIndex < 0 || strings.TrimSpace(input.ReferenceAudio.Codec) == "" {
		return SubtitleSyncProposal{}, errors.New("an explicit resolved reference audio stream is required")
	}
	if err := validateSubtitleSyncOffset(input.Representation, input.OffsetMilliseconds); err != nil {
		return SubtitleSyncProposal{}, err
	}
	_, sourceTimings, err := applySubtitleSyncTransform(input.Representation, input.Source, 0)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	_, proposedTimings, err := applySubtitleSyncTransform(input.Representation, input.Source, input.OffsetMilliseconds)
	if err != nil {
		return SubtitleSyncProposal{}, err
	}
	proposal := SubtitleSyncProposal{
		SubtitlePath: input.SubtitlePath, SourceIdentity: input.SourceIdentity, SourceRevision: input.SourceRevision,
		Representation: strings.ToLower(strings.TrimSpace(input.Representation)), ReferenceAudio: input.ReferenceAudio,
		Engine: subtitleSyncEngineName, EngineVersion: subtitleSyncEngineVersion,
		Model: "fixed_offset", OffsetMilliseconds: input.OffsetMilliseconds, Scale: 1,
		Segments: []SubtitleSyncSegment{}, QualityState: "manual_review", AnalyzedAt: time.Now().UTC(),
		Diagnostics: []string{"Manual fixed offset; review the proposed cue times before applying."},
		Preview:     subtitleSyncPreview(sourceTimings, proposedTimings),
	}
	proposal.ProposalID = subtitleSyncProposalID(proposal)
	return proposal, nil
}

func (fixedOffsetSubtitleSyncEngine) Apply(ctx context.Context, source []byte, proposal SubtitleSyncProposal) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if proposal.Model != "fixed_offset" || proposal.Engine != subtitleSyncEngineName || proposal.EngineVersion != subtitleSyncEngineVersion || proposal.Scale != 1 || len(proposal.Segments) != 0 {
		return nil, errors.New("unsupported subtitle sync proposal")
	}
	if proposal.ProposalID == "" || proposal.ProposalID != subtitleSyncProposalID(proposal) {
		return nil, errors.New("subtitle sync proposal identity is invalid")
	}
	output, _, err := applySubtitleSyncTransform(proposal.Representation, source, proposal.OffsetMilliseconds)
	return output, err
}

type subtitleSyncProposalIdentity struct {
	SourceIdentity     string                     `json:"sourceIdentity"`
	SourceRevision     string                     `json:"sourceRevision"`
	Representation     string                     `json:"representation"`
	ReferenceAudio     SubtitleSyncAudioReference `json:"referenceAudio"`
	Engine             string                     `json:"engine"`
	EngineVersion      string                     `json:"engineVersion"`
	Model              string                     `json:"model"`
	OffsetMilliseconds int64                      `json:"offsetMilliseconds"`
	Scale              float64                    `json:"scale"`
}

func subtitleSyncProposalID(proposal SubtitleSyncProposal) string {
	identity := subtitleSyncProposalIdentity{
		SourceIdentity: proposal.SourceIdentity, SourceRevision: proposal.SourceRevision,
		Representation: proposal.Representation, ReferenceAudio: proposal.ReferenceAudio,
		Engine: proposal.Engine, EngineVersion: proposal.EngineVersion, Model: proposal.Model,
		OffsetMilliseconds: proposal.OffsetMilliseconds, Scale: proposal.Scale,
	}
	encoded, _ := json.Marshal(identity)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

type subtitleSyncAnalysisRequest struct {
	Path                      string `json:"path" binding:"required"`
	SubtitlePath              string `json:"subtitlePath" binding:"required"`
	ReferenceAudioStreamIndex *int   `json:"referenceAudioStreamIndex"`
	OffsetMilliseconds        *int64 `json:"offsetMilliseconds"`
}

type subtitleSyncApplyRequest struct {
	Path                      string `json:"path" binding:"required"`
	SubtitlePath              string `json:"subtitlePath" binding:"required"`
	ReferenceAudioStreamIndex *int   `json:"referenceAudioStreamIndex"`
	OffsetMilliseconds        *int64 `json:"offsetMilliseconds"`
	SourceRevision            string `json:"sourceRevision" binding:"required"`
	ProposalID                string `json:"proposalId" binding:"required"`
}

type subtitleSyncOperationView struct {
	Proposal  SubtitleSyncProposal `json:"proposal"`
	AppliedAt *time.Time           `json:"appliedAt,omitempty"`
}

func (h AssetHandler) SubtitleSyncCapability(c *gin.Context) {
	c.JSON(200, gin.H{
		"enabled": true, "fixedOffset": true, "automaticAlignment": false, "segmentedAlignment": false,
		"engines": []gin.H{{"name": subtitleSyncEngineName, "version": subtitleSyncEngineVersion}},
	})
}

func (h AssetHandler) AnalyzeSubtitleSync(c *gin.Context) {
	var input subtitleSyncAnalysisRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.ReferenceAudioStreamIndex == nil || input.OffsetMilliseconds == nil || *input.ReferenceAudioStreamIndex < 0 {
		c.JSON(400, gin.H{"error": "path, subtitlePath, referenceAudioStreamIndex, and offsetMilliseconds are required"})
		return
	}
	proposal, _, _, err := h.resolveAndAnalyzeSubtitleSync(c.Request.Context(), input.Path, input.SubtitlePath, *input.ReferenceAudioStreamIndex, *input.OffsetMilliseconds)
	if err != nil {
		writeSubtitleSyncRequestError(c, err)
		return
	}
	c.JSON(200, proposal)
}

func (h AssetHandler) resolveAndAnalyzeSubtitleSync(ctx context.Context, assetPath, subtitlePath string, audioStreamIndex int, offsetMilliseconds int64) (SubtitleSyncProposal, string, subtitleTranslationSourceResponse, error) {
	if err := validateSubtitleSyncOffset(strings.TrimPrefix(strings.ToLower(filepath.Ext(subtitlePath)), "."), offsetMilliseconds); err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, err
	}
	mediaPath, source, err := h.resolveSubtitleSyncSource(ctx, assetPath, subtitlePath)
	if err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, err
	}
	content, err := readBoundedTranslationSubtitle(source.ExternalPath)
	if err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, errors.New("selected subtitle is not readable or valid")
	}
	audio, err := resolveSubtitleSyncAudio(ctx, mediaPath, audioStreamIndex)
	if err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, err
	}
	proposal, err := (fixedOffsetSubtitleSyncEngine{}).Analyze(ctx, SubtitleSyncAnalysisInput{
		SubtitlePath: source.ExternalPath, SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision,
		Representation: source.Representation, ReferenceAudio: audio,
		OffsetMilliseconds: offsetMilliseconds, Source: content,
	})
	if err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, err
	}
	return proposal, mediaPath, source, nil
}

func (h AssetHandler) resolveSubtitleSyncSource(ctx context.Context, assetPath, subtitlePath string) (string, subtitleTranslationSourceResponse, error) {
	selector := subtitleTranslationSourceSelector{Type: "external", External: &externalSubtitleTranslationSource{SubtitlePath: subtitlePath}}
	mediaPath, source, err := h.resolveSubtitleTranslationAssetSource(ctx, assetPath, selector)
	if err != nil {
		return "", subtitleTranslationSourceResponse{}, err
	}
	if source.Type != "external" || (source.Representation != "srt" && source.Representation != "ass" && source.Representation != "ssa") {
		return "", subtitleTranslationSourceResponse{}, errors.New("sync requires an eligible external SRT, ASS, or SSA sidecar")
	}
	return mediaPath, source, nil
}

func resolveSubtitleSyncAudio(ctx context.Context, mediaPath string, streamIndex int) (SubtitleSyncAudioReference, error) {
	streams, err := probeTranslationSourceStreams(ctx, mediaPath)
	if err != nil {
		return SubtitleSyncAudioReference{}, errors.New("unable to inspect audio streams for subtitle synchronization")
	}
	for _, stream := range streams {
		if stream.Index != streamIndex {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stream.CodecType), "audio") {
			return SubtitleSyncAudioReference{}, fmt.Errorf("stream index %d is not an audio stream", streamIndex)
		}
		streamID := ""
		if stream.ID != nil {
			streamID = strings.TrimSpace(fmt.Sprint(stream.ID))
		}
		return SubtitleSyncAudioReference{StreamIndex: stream.Index, StreamID: streamID,
			Codec: strings.ToLower(strings.TrimSpace(stream.CodecName)), Language: translationStreamLanguage(stream),
			Title: strings.TrimSpace(stream.Tags["title"])}, nil
	}
	return SubtitleSyncAudioReference{}, fmt.Errorf("audio stream index %d does not exist", streamIndex)
}

func writeSubtitleSyncRequestError(c *gin.Context, err error) {
	var resolutionErr *subtitleAssetResolutionError
	if errors.As(err, &resolutionErr) {
		writeSubtitleAssetResolutionError(c, err)
		return
	}
	status := 422
	if errors.Is(err, errTranslationSourceStale) {
		status = 409
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (h AssetHandler) StartSubtitleSync(c *gin.Context) {
	var input subtitleSyncApplyRequest
	if err := c.ShouldBindJSON(&input); err != nil || input.ReferenceAudioStreamIndex == nil || input.OffsetMilliseconds == nil || *input.ReferenceAudioStreamIndex < 0 || strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.SourceRevision) == "" {
		c.JSON(400, gin.H{"error": "path, subtitlePath, referenceAudioStreamIndex, offsetMilliseconds, sourceRevision, and proposalId are required"})
		return
	}
	proposal, mediaPath, source, err := h.resolveAndAnalyzeSubtitleSync(c.Request.Context(), input.Path, input.SubtitlePath, *input.ReferenceAudioStreamIndex, *input.OffsetMilliseconds)
	if err != nil {
		writeSubtitleSyncRequestError(c, err)
		return
	}
	if proposal.SourceRevision != strings.TrimSpace(input.SourceRevision) || proposal.ProposalID != strings.TrimSpace(input.ProposalID) {
		c.JSON(409, gin.H{"error": "subtitle sync proposal is stale; analyze the current subtitle and audio stream again"})
		return
	}
	workspace, err := os.MkdirTemp(filepath.Dir(source.ExternalPath), ".mvforge-subtitle-sync-*")
	if err != nil {
		c.JSON(500, gin.H{"error": "could not create private subtitle sync staging"})
		return
	}
	operationID := fmt.Sprintf("subtitle-sync-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	operation := &subtitleExtractionOperation{
		ID: operationID, Kind: "subtitle_sync", Status: "running", Phase: "preparing", Progress: 1,
		StreamIndex: -1, Format: source.Representation, AssetPath: mediaPath,
		CreatedAt: time.Now(), UpdatedAt: time.Now(), cancel: cancel, publicationCommittedCh: make(chan struct{}),
		SubtitleSync: &subtitleSyncOperationView{Proposal: proposal},
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	go func() {
		defer cancel()
		h.runSubtitleSyncOperation(workCtx, operationID, workspace, mediaPath, source, proposal)
	}()
	c.JSON(202, gin.H{"operationId": operationID, "status": "running", "phase": "preparing", "progress": 1})
}

func (h AssetHandler) runSubtitleSyncOperation(ctx context.Context, operationID, workspace, mediaPath string, source subtitleTranslationSourceResponse, proposal SubtitleSyncProposal) {
	defer cleanupSubtitleOperationWorkspace(workspace)
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Phase, operation.Progress, operation.Message = "validating", 25, "Applying fixed offset to canonical subtitle timing"
	})
	content, err := readBoundedTranslationSubtitle(source.ExternalPath)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	var output []byte
	if err == nil {
		output, err = (fixedOffsetSubtitleSyncEngine{}).Apply(ctx, content, proposal)
	}
	stagePath := filepath.Join(workspace, "subtitle-output."+source.Representation)
	if err == nil {
		if err = os.WriteFile(stagePath, output, 0o600); err == nil && !validTranslatedSubtitleSidecar(source.Representation, output) {
			err = errors.New("synchronized subtitle failed validation")
		}
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Phase, operation.Progress, operation.Message = "publishing", 80, "Rechecking subtitle and audio identities before publication"
		})
		err = h.publishSubtitleSyncStage(ctx, operationID, workspace, mediaPath, source, proposal, stagePath)
	}
	if err != nil {
		status, phase, message := "error", "error", "Subtitle synchronization failed"
		if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
			status, phase, message = "cancelled", "cancelled", "Subtitle synchronization cancelled"
		}
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status, operation.Phase, operation.Message = status, phase, message
			operation.Error, operation.Retryable = err.Error(), status == "error" || status == "cancelled"
		})
		return
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status, operation.Phase, operation.Progress = "completed", "completed", 100
		operation.Message, operation.Retryable = "Subtitle synchronization completed", false
		operation.Result = &SubtitleExtractionResult{Created: []string{}, Existing: []string{}, Unsupported: []string{},
			Artifacts: []SubtitleArtifact{{Type: "subtitle_sidecar", Format: source.Representation, Mode: "sync", StagedPath: source.ExternalPath, PublishedPath: source.ExternalPath, Status: "published"}}}
		if operation.SubtitleSync != nil {
			now := time.Now().UTC()
			operation.SubtitleSync.AppliedAt = &now
		}
	})
}

func (h AssetHandler) publishSubtitleSyncStage(ctx context.Context, operationID, workspace, mediaPath string, source subtitleTranslationSourceResponse, proposal SubtitleSyncProposal, stagePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(source.ExternalPath)
	if err != nil || info.IsDir() {
		return staleTranslationSourceError("selected subtitle is no longer available")
	}
	if err := os.Chmod(stagePath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("could not preserve subtitle permissions: %w", err)
	}
	backupPath := filepath.Join(workspace, "source-before-sync")
	if err := copyPublishedFile(source.ExternalPath, backupPath, true); err != nil {
		return fmt.Errorf("could not stage the previous subtitle for rollback: %w", err)
	}
	if subtitlePublicationBeforeLockHook != nil {
		subtitlePublicationBeforeLockHook()
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := resolveExternalSubtitleTranslationSource(mediaPath, source.ExternalPath)
	if err != nil || current.SourceRevision != proposal.SourceRevision || current.SourceIdentity != proposal.SourceIdentity {
		return staleTranslationSourceError("selected subtitle revision changed before publication")
	}
	audio, err := resolveSubtitleSyncAudio(ctx, mediaPath, proposal.ReferenceAudio.StreamIndex)
	if err != nil || audio != proposal.ReferenceAudio {
		return staleTranslationSourceError("selected audio stream identity changed before publication")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishStagedSubtitleFile(stagePath, source.ExternalPath); err != nil {
		return fmt.Errorf("could not publish synchronized subtitle: %w", err)
	}
	rollback := func(cause error) error {
		if restoreErr := publishStagedSubtitleFile(backupPath, source.ExternalPath); restoreErr != nil {
			return fmt.Errorf("%v; subtitle rollback failed: %w", cause, restoreErr)
		}
		return cause
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	segments, _ := json.Marshal(proposal.Segments)
	provenance := models.SubtitleSyncProvenance{
		OperationID: operationID, AssetPath: mediaPath, SubtitlePath: source.ExternalPath,
		SourceIdentity: proposal.SourceIdentity, SourceRevision: proposal.SourceRevision,
		ReferenceAudioStreamIndex: proposal.ReferenceAudio.StreamIndex,
		ReferenceAudioStreamID:    proposal.ReferenceAudio.StreamID, ReferenceAudioCodec: proposal.ReferenceAudio.Codec,
		ReferenceAudioLanguage: proposal.ReferenceAudio.Language, ReferenceAudioTitle: proposal.ReferenceAudio.Title,
		Engine: proposal.Engine, EngineVersion: proposal.EngineVersion, Model: proposal.Model,
		OffsetMilliseconds: proposal.OffsetMilliseconds, Scale: proposal.Scale, SegmentsJSON: string(segments),
		QualityState: proposal.QualityState, AnalysisAt: proposal.AnalyzedAt, AppliedAt: time.Now().UTC(),
	}
	if err := h.db.Create(&provenance).Error; err != nil {
		return rollback(fmt.Errorf("could not persist subtitle sync provenance: %w", err))
	}
	if err := ctx.Err(); err != nil {
		if deleteErr := h.db.Delete(&provenance).Error; deleteErr != nil {
			return fmt.Errorf("%v; sync provenance rollback failed: %w", err, deleteErr)
		}
		return rollback(err)
	}
	markSubtitlePublicationCommitted(operationID)
	return nil
}

func (h AssetHandler) SubtitleSyncProvenance(c *gin.Context) {
	assetPath := strings.TrimSpace(c.Query("path"))
	subtitlePath := strings.TrimSpace(c.Query("subtitlePath"))
	if assetPath == "" || subtitlePath == "" {
		c.JSON(400, gin.H{"error": "path and subtitlePath are required"})
		return
	}
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(assetPath)
	if err != nil {
		writeSubtitleAssetResolutionError(c, err)
		return
	}
	validatedPath, err := validatedExternalSubtitlePath(mediaPath, subtitlePath)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	var records []models.SubtitleSyncProvenance
	if err := h.db.Where("asset_path = ? AND subtitle_path = ?", mediaPath, validatedPath).Order("applied_at DESC").Limit(20).Find(&records).Error; err != nil {
		c.JSON(500, gin.H{"error": "could not read subtitle synchronization provenance"})
		return
	}
	c.JSON(200, gin.H{"records": records})
}
