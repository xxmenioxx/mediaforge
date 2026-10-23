package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	subtitleSyncEngineName       = "manual"
	subtitleSyncEngineVersion    = "fixed-offset-v1"
	subtitleSyncModeFixedOffset  = "fixed_offset"
	subtitleSyncModeAutomatic    = "automatic"
	maxSubtitleSyncOffsetMs      = int64(12 * 60 * 60 * 1000)
	maxSubtitleSyncPreview       = 12
	maxSubtitleSyncProposalCache = 64
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
	Confidence         float64                    `json:"confidence,omitempty"`
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
	MediaPath          string
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
		Model: subtitleSyncModelFixedOffset, OffsetMilliseconds: input.OffsetMilliseconds, Scale: 1,
		Segments: []SubtitleSyncSegment{}, QualityState: "manual_review", AnalyzedAt: time.Now().UTC(),
		Diagnostics: []string{"Manual fixed offset; review the proposed cue times before applying."},
		Preview:     subtitleSyncPreview(sourceTimings, proposedTimings),
	}
	if err := validateSubtitleSyncProposalShape(proposal); err != nil {
		return SubtitleSyncProposal{}, err
	}
	proposal.ProposalID = subtitleSyncProposalID(proposal)
	return proposal, nil
}

func (fixedOffsetSubtitleSyncEngine) Apply(ctx context.Context, source []byte, proposal SubtitleSyncProposal) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSubtitleSyncProposalShape(proposal); err != nil {
		return nil, err
	}
	if proposal.Model != subtitleSyncModelFixedOffset || proposal.Engine != subtitleSyncEngineName || proposal.EngineVersion != subtitleSyncEngineVersion {
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
	Confidence         float64                    `json:"confidence,omitempty"`
	Scale              float64                    `json:"scale"`
	QualityState       string                     `json:"qualityState"`
	Segments           []SubtitleSyncSegment      `json:"segments"`
}

func subtitleSyncProposalID(proposal SubtitleSyncProposal) string {
	identity := subtitleSyncProposalIdentity{
		SourceIdentity: proposal.SourceIdentity, SourceRevision: proposal.SourceRevision,
		Representation: proposal.Representation, ReferenceAudio: proposal.ReferenceAudio,
		Engine: proposal.Engine, EngineVersion: proposal.EngineVersion, Model: proposal.Model,
		OffsetMilliseconds: proposal.OffsetMilliseconds, Confidence: proposal.Confidence, Scale: proposal.Scale,
		QualityState: proposal.QualityState, Segments: proposal.Segments,
	}
	encoded, _ := json.Marshal(identity)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

type subtitleSyncAnalysisRequest struct {
	Path                      string `json:"path" binding:"required"`
	SubtitlePath              string `json:"subtitlePath" binding:"required"`
	ReferenceAudioStreamIndex *int   `json:"referenceAudioStreamIndex"`
	Mode                      string `json:"mode,omitempty"`
	OffsetMilliseconds        *int64 `json:"offsetMilliseconds"`
}

type subtitleSyncApplyRequest struct {
	Path                      string `json:"path" binding:"required"`
	SubtitlePath              string `json:"subtitlePath" binding:"required"`
	ReferenceAudioStreamIndex *int   `json:"referenceAudioStreamIndex"`
	Mode                      string `json:"mode,omitempty"`
	OffsetMilliseconds        *int64 `json:"offsetMilliseconds"`
	SourceRevision            string `json:"sourceRevision" binding:"required"`
	ProposalID                string `json:"proposalId" binding:"required"`
}

type subtitleSyncOperationView struct {
	Proposal  SubtitleSyncProposal `json:"proposal"`
	AppliedAt *time.Time           `json:"appliedAt,omitempty"`
}

type cachedSubtitleSyncProposal struct {
	proposal  SubtitleSyncProposal
	expiresAt time.Time
}

var subtitleSyncProposals = struct {
	sync.Mutex
	items map[string]cachedSubtitleSyncProposal
}{items: make(map[string]cachedSubtitleSyncProposal)}

func rememberSubtitleSyncProposal(proposal SubtitleSyncProposal) {
	if proposal.ProposalID == "" {
		return
	}
	now := time.Now()
	subtitleSyncProposals.Lock()
	defer subtitleSyncProposals.Unlock()
	for id, item := range subtitleSyncProposals.items {
		if !item.expiresAt.After(now) {
			delete(subtitleSyncProposals.items, id)
		}
	}
	if len(subtitleSyncProposals.items) >= maxSubtitleSyncProposalCache {
		var oldestID string
		var oldest time.Time
		for id, item := range subtitleSyncProposals.items {
			if oldestID == "" || item.expiresAt.Before(oldest) {
				oldestID, oldest = id, item.expiresAt
			}
		}
		delete(subtitleSyncProposals.items, oldestID)
	}
	subtitleSyncProposals.items[proposal.ProposalID] = cachedSubtitleSyncProposal{proposal: proposal, expiresAt: now.Add(15 * time.Minute)}
}

func takeSubtitleSyncProposal(proposalID string) (SubtitleSyncProposal, bool) {
	now := time.Now()
	subtitleSyncProposals.Lock()
	defer subtitleSyncProposals.Unlock()
	item, ok := subtitleSyncProposals.items[proposalID]
	delete(subtitleSyncProposals.items, proposalID)
	if !ok || !item.expiresAt.After(now) {
		return SubtitleSyncProposal{}, false
	}
	return item.proposal, true
}

func normalizeSubtitleSyncMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return subtitleSyncModeFixedOffset
	}
	return mode
}

func (h AssetHandler) SubtitleSyncCapability(c *gin.Context) {
	automaticAvailable := subtitleSyncAutomaticEngineAvailable()
	engines := []gin.H{{"name": subtitleSyncEngineName, "version": subtitleSyncEngineVersion}}
	automaticReason := subtitleSyncAutomaticUnavailableReason()
	if automaticAvailable {
		engines = append(engines, gin.H{"name": subtitleSyncAutoEngine, "version": subtitleSyncAutoEngineVersion})
		automaticReason = ""
	}
	c.JSON(200, gin.H{
		"enabled": true, "fixedOffset": true, "automaticAlignment": automaticAvailable, "segmentedAlignment": false,
		"automaticAlignmentReason": automaticReason, "automaticFormats": []string{"srt"}, "engines": engines,
	})
}

func (h AssetHandler) AnalyzeSubtitleSync(c *gin.Context) {
	var input subtitleSyncAnalysisRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "invalid subtitle sync analysis request"})
		return
	}
	mode := normalizeSubtitleSyncMode(input.Mode)
	if input.ReferenceAudioStreamIndex == nil || *input.ReferenceAudioStreamIndex < 0 || (mode == subtitleSyncModeFixedOffset && input.OffsetMilliseconds == nil) || (mode == subtitleSyncModeAutomatic && input.OffsetMilliseconds != nil) || (mode != subtitleSyncModeFixedOffset && mode != subtitleSyncModeAutomatic) {
		c.JSON(400, gin.H{"error": "path, subtitlePath, referenceAudioStreamIndex, and a valid analysis mode are required"})
		return
	}
	if mode == subtitleSyncModeAutomatic && !subtitleSyncAutomaticEngineAvailable() {
		c.JSON(503, gin.H{"error": subtitleSyncAutomaticUnavailableReason()})
		return
	}
	offset := int64(0)
	if input.OffsetMilliseconds != nil {
		offset = *input.OffsetMilliseconds
	}
	proposal, _, _, err := h.resolveAndAnalyzeSubtitleSync(c.Request.Context(), input.Path, input.SubtitlePath, *input.ReferenceAudioStreamIndex, mode, offset)
	if err != nil {
		writeSubtitleSyncRequestError(c, err)
		return
	}
	if mode == subtitleSyncModeAutomatic {
		rememberSubtitleSyncProposal(proposal)
	}
	c.JSON(200, proposal)
}

func (h AssetHandler) resolveAndAnalyzeSubtitleSync(ctx context.Context, assetPath, subtitlePath string, audioStreamIndex int, mode string, offsetMilliseconds int64) (SubtitleSyncProposal, string, subtitleTranslationSourceResponse, error) {
	representation := strings.TrimPrefix(strings.ToLower(filepath.Ext(subtitlePath)), ".")
	if mode == subtitleSyncModeAutomatic {
		if representation != "srt" {
			return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, errors.New("automatic alignment currently supports SRT only; ASS and SSA retain manual fixed offset")
		}
		if offsetMilliseconds != 0 {
			return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, errors.New("automatic alignment does not accept a caller-provided offset")
		}
	} else if mode != subtitleSyncModeFixedOffset {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, errors.New("subtitle synchronization analysis mode is invalid")
	} else if err := validateSubtitleSyncOffset(representation, offsetMilliseconds); err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, err
	}
	mediaPath, source, err := h.resolveSubtitleSyncSource(ctx, assetPath, subtitlePath)
	if err != nil {
		return SubtitleSyncProposal{}, "", subtitleTranslationSourceResponse{}, err
	}
	if err := requireNoSubtitleSyncRecovery(source.ExternalPath); err != nil {
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
	analysisInput := SubtitleSyncAnalysisInput{
		SubtitlePath: source.ExternalPath, MediaPath: mediaPath, SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision,
		Representation: source.Representation, ReferenceAudio: audio,
		OffsetMilliseconds: offsetMilliseconds, Source: content,
	}
	var engine SubtitleSyncEngine = fixedOffsetSubtitleSyncEngine{}
	if mode == subtitleSyncModeAutomatic {
		engine = automaticSubtitleSyncEngine{decoder: ffmpegSubtitleSyncPCMDecoder{}}
	}
	proposal, err := engine.Analyze(ctx, analysisInput)
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
	var recoveryErr *subtitleSyncRecoveryError
	if errors.As(err, &recoveryErr) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "recovery": recoveryErr.status})
		return
	}
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
	if err := c.ShouldBindJSON(&input); err != nil || input.ReferenceAudioStreamIndex == nil || *input.ReferenceAudioStreamIndex < 0 || strings.TrimSpace(input.ProposalID) == "" || strings.TrimSpace(input.SourceRevision) == "" {
		c.JSON(400, gin.H{"error": "path, subtitlePath, referenceAudioStreamIndex, sourceRevision, and proposalId are required"})
		return
	}
	mode := normalizeSubtitleSyncMode(input.Mode)
	if mode != subtitleSyncModeFixedOffset && mode != subtitleSyncModeAutomatic {
		c.JSON(400, gin.H{"error": "analysis mode or fixed offset is invalid"})
		return
	}
	mediaPath, source, err := h.resolveSubtitleSyncSource(c.Request.Context(), input.Path, input.SubtitlePath)
	if err != nil {
		writeSubtitleSyncRequestError(c, err)
		return
	}
	if err := requireNoSubtitleSyncRecovery(source.ExternalPath); err != nil {
		writeSubtitleSyncRequestError(c, err)
		return
	}
	var proposal SubtitleSyncProposal
	if mode == subtitleSyncModeAutomatic {
		if input.OffsetMilliseconds != nil {
			c.JSON(400, gin.H{"error": "automatic apply uses the reviewed proposal offset"})
			return
		}
		var found bool
		proposal, found = takeSubtitleSyncProposal(strings.TrimSpace(input.ProposalID))
		if !found {
			c.JSON(409, gin.H{"error": "automatic proposal expired; analyze the current subtitle and audio again"})
			return
		}
		if proposal.QualityState != "automatic_review" {
			c.JSON(422, gin.H{"error": "low-confidence automatic proposals cannot be applied; choose a manual fixed offset after review"})
			return
		}
		if !subtitleSyncAutomaticEngineAvailable() {
			c.JSON(503, gin.H{"error": subtitleSyncAutomaticUnavailableReason()})
			return
		}
		if proposal.SourceRevision != strings.TrimSpace(input.SourceRevision) || proposal.SubtitlePath != source.ExternalPath || proposal.SourceIdentity != source.SourceIdentity || proposal.SourceRevision != source.SourceRevision {
			err = errors.New("subtitle sync proposal is stale; analyze the current subtitle and audio stream again")
		}
		if err == nil {
			var currentAudio SubtitleSyncAudioReference
			currentAudio, err = resolveSubtitleSyncAudio(c.Request.Context(), mediaPath, *input.ReferenceAudioStreamIndex)
			if err == nil && (proposal.ReferenceAudio != currentAudio || proposal.ReferenceAudio.StreamIndex != *input.ReferenceAudioStreamIndex) {
				err = errors.New("subtitle sync proposal is stale; analyze the current subtitle and audio stream again")
			}
		}
	} else if mode == subtitleSyncModeFixedOffset && input.OffsetMilliseconds != nil {
		proposal, mediaPath, source, err = h.resolveAndAnalyzeSubtitleSync(c.Request.Context(), input.Path, input.SubtitlePath, *input.ReferenceAudioStreamIndex, mode, *input.OffsetMilliseconds)
	}
	if err != nil {
		writeSubtitleSyncRequestError(c, err)
		return
	}
	if mode == subtitleSyncModeFixedOffset && (proposal.SourceRevision != strings.TrimSpace(input.SourceRevision) || proposal.ProposalID != strings.TrimSpace(input.ProposalID)) {
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
		message := "Applying fixed offset to canonical subtitle timing"
		if proposal.Engine == subtitleSyncAutoEngine {
			message = "Applying reviewed automatic fixed-offset proposal to canonical subtitle timing"
		}
		operation.Phase, operation.Progress, operation.Message = "validating", 25, message
	})
	content, err := readBoundedTranslationSubtitle(source.ExternalPath)
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	var output []byte
	if err == nil {
		var engine SubtitleSyncEngine = fixedOffsetSubtitleSyncEngine{}
		if proposal.Engine == subtitleSyncAutoEngine {
			engine = automaticSubtitleSyncEngine{}
		}
		output, err = engine.Apply(ctx, content, proposal)
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
		if errors.Is(err, errSubtitleSyncRecoveryRequired) {
			phase, message = "recovery_required", "Subtitle synchronization requires recovery before another attempt"
		} else if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
			status, phase, message = "cancelled", "cancelled", "Subtitle synchronization cancelled"
		}
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status, operation.Phase, operation.Message = status, phase, message
			operation.Error, operation.Retryable = err.Error(), status == "error" || status == "cancelled"
			if errors.Is(err, errSubtitleSyncRecoveryRequired) {
				operation.Retryable = false
			}
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
	return h.publishSubtitleSyncStageWithHooks(ctx, operationID, workspace, mediaPath, source, proposal, stagePath, subtitleSyncPublicationHooks{})
}

type subtitleSyncPublicationHooks struct {
	createRecoveryIntent  func(string, string, string, string, string, string) (*subtitleSyncRecoveryRecord, string, error)
	restoreBackup         func(subtitleSyncRecoveryRecord, string) error
	afterPublish          func()
	afterProvenanceCreate func()
	afterCommit           func()
}

func (h AssetHandler) publishSubtitleSyncStageWithHooks(ctx context.Context, operationID, workspace, mediaPath string, source subtitleTranslationSourceResponse, proposal SubtitleSyncProposal, stagePath string, hooks subtitleSyncPublicationHooks) error {
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
	if subtitlePublicationBeforeLockHook != nil {
		subtitlePublicationBeforeLockHook()
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := requireNoSubtitleSyncRecovery(source.ExternalPath); err != nil {
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
	createRecoveryIntent := hooks.createRecoveryIntent
	if createRecoveryIntent == nil {
		createRecoveryIntent = createSubtitleSyncRecoveryIntent
	}
	recovery, markerPath, err := createRecoveryIntent(
		source.ExternalPath, mediaPath, operationID, proposal.SourceIdentity, proposal.SourceRevision, stagePath,
	)
	if err != nil {
		return err
	}
	current, err = resolveExternalSubtitleTranslationSource(mediaPath, source.ExternalPath)
	if err != nil || current.SourceRevision != proposal.SourceRevision || current.SourceIdentity != proposal.SourceIdentity {
		staleErr := staleTranslationSourceError("selected subtitle revision changed while recovery state was staged")
		if clearErr := clearSubtitleSyncRecovery(*recovery, markerPath); clearErr != nil {
			return markSubtitleSyncRecoveryRequired(recovery, markerPath, "not_persisted", fmt.Errorf("%v; unused recovery state cleanup failed: %w", staleErr, clearErr))
		}
		return staleErr
	}
	if err := publishStagedSubtitleFile(stagePath, source.ExternalPath); err != nil {
		publishErr := fmt.Errorf("could not publish synchronized subtitle: %w", err)
		currentDigest, digestErr := subtitleSyncSHA256File(source.ExternalPath)
		if digestErr == nil && currentDigest == recovery.OriginalSHA256 {
			if clearErr := clearSubtitleSyncRecovery(*recovery, markerPath); clearErr == nil {
				return publishErr
			} else {
				publishErr = fmt.Errorf("%v; recovery state cleanup failed: %w", publishErr, clearErr)
			}
		}
		return markSubtitleSyncRecoveryRequired(recovery, markerPath, "not_persisted", publishErr)
	}
	if hooks.afterPublish != nil {
		hooks.afterPublish()
	}
	if hooks.restoreBackup == nil {
		hooks.restoreBackup = restoreSubtitleSyncBackup
	}
	rollback := func(cause error, provenanceState string, deleteErr error) error {
		restoreErr := hooks.restoreBackup(*recovery, source.ExternalPath)
		if deleteErr == nil && restoreErr == nil {
			if clearErr := clearSubtitleSyncRecovery(*recovery, markerPath); clearErr == nil {
				return cause
			} else {
				restoreErr = fmt.Errorf("recovery state cleanup failed: %w", clearErr)
			}
		}
		combined := cause
		if deleteErr != nil {
			combined = fmt.Errorf("%v; subtitle provenance compensation failed: %w", combined, deleteErr)
		}
		if restoreErr != nil {
			combined = fmt.Errorf("%v; subtitle file compensation failed: %w", combined, restoreErr)
		}
		if deleteErr != nil {
			provenanceState = "unknown"
		} else if provenanceState == "present" {
			provenanceState = "removed"
		}
		return markSubtitleSyncRecoveryRequired(recovery, markerPath, provenanceState, combined)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err, "not_persisted", nil)
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
		return rollback(fmt.Errorf("could not persist subtitle sync provenance: %w", err), "unknown", nil)
	}
	if hooks.afterProvenanceCreate != nil {
		hooks.afterProvenanceCreate()
	}
	if !commitSubtitlePublicationIfActive(operationID, ctx) {
		cause := ctx.Err()
		if cause == nil {
			cause = errors.New("subtitle operation stopped before publication committed")
		}
		deleteErr := h.db.Delete(&provenance).Error
		return rollback(cause, "present", deleteErr)
	}
	if hooks.afterCommit != nil {
		hooks.afterCommit()
	}
	if err := clearSubtitleSyncRecovery(*recovery, markerPath); err != nil {
		return markSubtitleSyncRecoveryRequired(recovery, markerPath, "present", fmt.Errorf("subtitle and provenance were committed but recovery state cleanup failed: %w", err))
	}
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
	recovery, recoveryErr := inspectSubtitleSyncRecovery(validatedPath)
	if recoveryErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "subtitle synchronization recovery state could not be fully verified", "records": records, "recovery": recovery})
		return
	}
	c.JSON(200, gin.H{"records": records, "recovery": recovery})
}
