package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const subtitleTranslationQualityReviewMaxBytes = 256 * 1024

var subtitleTranslationQualityDecisionMu sync.Mutex

type subtitleTranslationQualityTarget struct {
	Format         string
	FinalPath      string
	SourceRevision string
	Artifact       SubtitleArtifact
}

type subtitleTranslationQualityDecision struct {
	CueID  string    `json:"cueId"`
	Action string    `json:"action"`
	Text   string    `json:"text,omitempty"`
	At     time.Time `json:"at"`
}

type subtitleTranslationQualityReviewResponse struct {
	OperationID string                               `json:"operationId"`
	Status      string                               `json:"status"`
	Quality     subtitleTranslationQualityReport     `json:"quality"`
	Decisions   []subtitleTranslationQualityDecision `json:"decisions"`
}

type subtitleTranslationQualityReviewListItem struct {
	OperationID string                               `json:"operationId"`
	Status      string                               `json:"status"`
	Quality     subtitleTranslationQualityReport     `json:"quality"`
	Decisions   []subtitleTranslationQualityDecision `json:"decisions"`
	UpdatedAt   time.Time                            `json:"updatedAt"`
}

func (h AssetHandler) ListSubtitleTranslationQualityReviews(c *gin.Context) {
	assetPath := strings.TrimSpace(c.Query("assetPath"))
	sourceIdentity := strings.TrimSpace(c.Query("sourceIdentity"))
	if h.db == nil || assetPath == "" || sourceIdentity == "" || len(assetPath) > 2048 || len(sourceIdentity) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "asset path and source identity are required"})
		return
	}
	var reviews []models.SubtitleTranslationQualityReview
	if err := h.db.Where("asset_path = ? AND source_identity = ?", assetPath, sourceIdentity).Order("updated_at DESC").Limit(25).Find(&reviews).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "translation quality reviews are unavailable"})
		return
	}
	items := make([]subtitleTranslationQualityReviewListItem, 0, len(reviews))
	for _, review := range reviews {
		var report subtitleTranslationQualityReport
		var decisions []subtitleTranslationQualityDecision
		if decodeQualityReviewJSON(review.QualityReport, &report) != nil || decodeQualityReviewJSON(review.Decisions, &decisions) != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "translation quality review evidence is invalid"})
			return
		}
		report.Persisted = true
		items = append(items, subtitleTranslationQualityReviewListItem{OperationID: review.OperationID, Status: review.Status, Quality: report, Decisions: decisions, UpdatedAt: review.UpdatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"reviews": items})
}

func (h AssetHandler) GetSubtitleTranslationQualityReview(c *gin.Context) {
	operationID := strings.TrimSpace(c.Param("id"))
	if operationID == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "translation operation id is required"})
		return
	}
	var review models.SubtitleTranslationQualityReview
	if err := h.db.First(&review, "operation_id = ?", operationID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "translation quality review not found"})
		return
	}
	var report subtitleTranslationQualityReport
	var decisions []subtitleTranslationQualityDecision
	if decodeQualityReviewJSON(review.QualityReport, &report) != nil || decodeQualityReviewJSON(review.Decisions, &decisions) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "translation quality review evidence is invalid"})
		return
	}
	report.Persisted = true
	c.JSON(http.StatusOK, subtitleTranslationQualityReviewResponse{OperationID: operationID, Status: review.Status, Quality: report, Decisions: decisions})
}

type subtitleTranslationQualityReviewInput struct {
	CueID  string `json:"cueId" binding:"required"`
	Action string `json:"action" binding:"required"`
	Text   string `json:"text"`
}

func persistSubtitleTranslationQualityReview(db *gorm.DB, operationID string, request subtitleTranslationOperationRequest, report subtitleTranslationQualityReport, artifacts []SubtitleArtifact) error {
	if db == nil || operationID == "" || len(artifacts) == 0 {
		return errors.New("translation quality review persistence is unavailable")
	}
	requestBytes, err := json.Marshal(request)
	if err != nil || len(requestBytes) > maxSubtitleTranslationRequestField*8 {
		return errors.New("translation quality review request exceeded its bound")
	}
	targets := make([]subtitleTranslationQualityTarget, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Mode != "translation" || artifact.PublishedPath == "" || artifact.Translation == nil {
			return errors.New("translation quality review target identity is incomplete")
		}
		revision, err := captureTranslationExternalSourceRevision(artifact.PublishedPath, "")
		if err != nil {
			return errors.New("published translation revision could not be captured")
		}
		artifact.StagedPath = ""
		targets = append(targets, subtitleTranslationQualityTarget{
			Format: artifact.Format, FinalPath: filepath.Clean(artifact.PublishedPath),
			SourceRevision: revision.key(), Artifact: artifact,
		})
	}
	targetBytes, err := json.Marshal(targets)
	if err != nil || len(targetBytes) > subtitleTranslationQualityReviewMaxBytes {
		return errors.New("translation quality review targets exceeded their bound")
	}
	reportBytes, err := json.Marshal(report)
	if err != nil || len(reportBytes) > subtitleTranslationQualityReviewMaxBytes {
		return errors.New("translation quality report exceeded its bound")
	}
	if len(report.Findings) > subtitleTranslationQualityMaxFindings {
		return errors.New("translation quality report contains too many findings")
	}
	row := models.SubtitleTranslationQualityReview{
		OperationID: operationID, AssetPath: request.AssetPath, SourceIdentity: request.SourceIdentity,
		SourceRevision: request.SourceRevision, RequestMetadata: string(requestBytes), TargetsMetadata: string(targetBytes),
		QualityReport: string(reportBytes), Decisions: "[]", Status: "open",
	}
	if len(report.Findings) == 0 {
		row.Status = "reviewed"
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&row).Error
	})
}

func (h AssetHandler) DecideSubtitleTranslationQuality(c *gin.Context) {
	operationID := strings.TrimSpace(c.Param("id"))
	if operationID == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "translation operation id is required"})
		return
	}
	var input subtitleTranslationQualityReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a cue id and review action are required"})
		return
	}
	input.CueID = strings.TrimSpace(input.CueID)
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	if input.Action != "keep" && input.Action != "edit" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "review action must be keep or edit"})
		return
	}
	if input.Action == "edit" && (strings.TrimSpace(input.Text) == "" || len([]rune(input.Text)) > 4000) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "edited cue text must contain 1 to 4000 characters"})
		return
	}
	if len(input.CueID) > 256 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cue id exceeds its bound"})
		return
	}
	subtitleTranslationQualityDecisionMu.Lock()
	defer subtitleTranslationQualityDecisionMu.Unlock()
	var review models.SubtitleTranslationQualityReview
	if err := h.db.First(&review, "operation_id = ?", operationID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "translation quality review not found"})
		return
	}
	if review.Status != "open" {
		c.JSON(http.StatusConflict, gin.H{"error": "translation quality review is already complete"})
		return
	}
	var request subtitleTranslationOperationRequest
	var report subtitleTranslationQualityReport
	var targets []subtitleTranslationQualityTarget
	if decodeQualityReviewJSON(review.RequestMetadata, &request) != nil || decodeQualityReviewJSON(review.QualityReport, &report) != nil || decodeQualityReviewJSON(review.TargetsMetadata, &targets) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "translation quality review evidence is invalid"})
		return
	}
	finding, ok := subtitleTranslationQualityFindingByID(report, input.CueID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "cue is not part of this quality review"})
		return
	}
	decisions := make([]subtitleTranslationQualityDecision, 0)
	if decodeQualityReviewJSON(review.Decisions, &decisions) != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "translation quality decisions are invalid"})
		return
	}
	for _, decision := range decisions {
		if decision.CueID == input.CueID {
			c.JSON(http.StatusConflict, gin.H{"error": "cue was already reviewed"})
			return
		}
	}
	if input.Action == "edit" {
		if err := h.applySubtitleTranslationQualityEdit(c.Request.Context(), review, request, targets, finding, input.Text); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
			return
		}
	}
	if err := h.validateSubtitleTranslationQualityIdentity(c.Request.Context(), review, request, targets); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}
	decisions = append(decisions, subtitleTranslationQualityDecision{CueID: input.CueID, Action: input.Action, Text: input.Text, At: time.Now().UTC()})
	decisionBytes, err := json.Marshal(decisions)
	if err != nil || len(decisionBytes) > subtitleTranslationQualityReviewMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "translation quality decisions exceeded their bound"})
		return
	}
	status := "open"
	if len(decisions) == len(report.Findings) {
		status = "reviewed"
	}
	result := h.db.Model(&models.SubtitleTranslationQualityReview{}).Where("operation_id = ? AND status = ?", operationID, "open").Updates(map[string]any{"decisions": string(decisionBytes), "status": status, "updated_at": time.Now().UTC()})
	if result.Error != nil || result.RowsAffected != 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "translation quality review changed before the decision was saved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"operationId": operationID, "cueId": input.CueID, "status": status})
}

func decodeQualityReviewJSON[T any](encoded string, target *T) error {
	if len(encoded) == 0 || len(encoded) > subtitleTranslationQualityReviewMaxBytes {
		return errors.New("translation quality review field is outside its bound")
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("translation quality review field has trailing content")
	}
	return nil
}

func subtitleTranslationQualityFindingByID(report subtitleTranslationQualityReport, id string) (subtitleTranslationQualityFinding, bool) {
	for _, finding := range report.Findings {
		if finding.CueID == id {
			return finding, true
		}
	}
	return subtitleTranslationQualityFinding{}, false
}

func (h AssetHandler) applySubtitleTranslationQualityEdit(ctx context.Context, review models.SubtitleTranslationQualityReview, request subtitleTranslationOperationRequest, targets []subtitleTranslationQualityTarget, finding subtitleTranslationQualityFinding, text string) error {
	if ctx == nil || h.db == nil || len(targets) == 0 {
		return errors.New("translation review edit context is incomplete")
	}
	mediaPath, source, stream, err := h.resolveTranslationExecutionSource(ctx, request)
	if err != nil {
		return err
	}
	sourceRevision, err := translationSourceRevisionForResolvedSource(source, stream)
	if err != nil || sourceRevision.key() != review.SourceRevision || source.SourceIdentity != review.SourceIdentity {
		return staleTranslationSourceError("original subtitle source changed after translation")
	}
	if filepath.Clean(mediaPath) == "." {
		return errors.New("translation media identity is unavailable")
	}
	if err := validateTranslationQualityTargetPaths(request, source, mediaPath, targets); err != nil {
		return err
	}
	outputs := make(map[string][]byte, len(targets))
	for _, target := range targets {
		if filepath.Clean(target.FinalPath) == filepath.Clean(translationSourceRevisionPath(sourceRevision)) || target.Artifact.Translation == nil || target.Artifact.Mode != "translation" {
			return errors.New("translation review target identity is invalid")
		}
		currentRevision, err := captureTranslationExternalSourceRevision(target.FinalPath, "")
		if err != nil || currentRevision.key() != target.SourceRevision {
			return staleTranslationSourceError("translated sidecar changed after quality analysis")
		}
		data, err := os.ReadFile(target.FinalPath)
		if err != nil || !validTranslatedSubtitleSidecar(target.Format, data) {
			return errors.New("translated sidecar is missing or no longer valid")
		}
		output, err := applySubtitleTranslationQualityText(target.Format, source, data, finding, text)
		if err != nil || !validTranslatedSubtitleSidecar(target.Format, output) {
			return errors.New("edited translated subtitle failed canonical validation")
		}
		outputs[target.Format] = output
	}
	workspaceRoot := canonicalOCRWorkspaceRoot()
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		return errors.New("translation review staging root is unavailable")
	}
	workspace, err := os.MkdirTemp(workspaceRoot, ".mvforge-translation-review-*")
	if err != nil {
		return errors.New("translation review workspace could not be created")
	}
	defer cleanupSubtitleOperationWorkspace(workspace)
	publicationTargets := make([]subtitleTranslationPublicationTarget, 0, len(targets))
	for _, target := range targets {
		publicationTargets = append(publicationTargets, subtitleTranslationPublicationTarget{Format: target.Format, FinalPath: target.FinalPath, Artifact: target.Artifact})
	}
	staged, err := stageSubtitleTranslationOutputs(workspace, outputs, publicationTargets)
	if err != nil {
		return err
	}
	_, err = publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx: ctx, OperationID: "subtitle-translation-review-" + review.OperationID, Workspace: workspace,
		SourceRevision: sourceRevision,
		RecheckSource: func(expected translationSourceRevision) error {
			if err := h.recheckAssetInfoTranslationSource(ctx, request, expected); err != nil {
				return err
			}
			for _, target := range targets {
				current, err := captureTranslationExternalSourceRevision(target.FinalPath, "")
				if err != nil || current.key() != target.SourceRevision {
					return staleTranslationSourceError("translated sidecar changed before the reviewed edit could be published")
				}
			}
			return nil
		},
		Targets: staged,
	})
	if err != nil {
		return err
	}
	for index := range targets {
		revision, err := captureTranslationExternalSourceRevision(targets[index].FinalPath, "")
		if err != nil {
			return errors.New("edited translation revision could not be captured")
		}
		targets[index].SourceRevision = revision.key()
	}
	encodedTargets, err := json.Marshal(targets)
	if err != nil || len(encodedTargets) > subtitleTranslationQualityReviewMaxBytes {
		return errors.New("edited translation review evidence exceeded its bound")
	}
	result := h.db.Model(&models.SubtitleTranslationQualityReview{}).
		Where("operation_id = ? AND status = ? AND updated_at = ?", review.OperationID, "open", review.UpdatedAt).
		Updates(map[string]any{"targets_metadata": string(encodedTargets), "updated_at": time.Now().UTC()})
	if result.Error != nil || result.RowsAffected != 1 {
		return errors.New("translation review changed while the edit was being published")
	}
	return nil
}

func (h AssetHandler) validateSubtitleTranslationQualityIdentity(ctx context.Context, review models.SubtitleTranslationQualityReview, request subtitleTranslationOperationRequest, targets []subtitleTranslationQualityTarget) error {
	_, source, stream, err := h.resolveTranslationExecutionSource(ctx, request)
	if err != nil {
		return err
	}
	revision, err := translationSourceRevisionForResolvedSource(source, stream)
	if err != nil || revision.key() != review.SourceRevision || source.SourceIdentity != review.SourceIdentity {
		return staleTranslationSourceError("original subtitle source changed after translation")
	}
	mediaPath := filepath.Clean(request.AssetPath)
	for _, target := range targets {
		current, err := captureTranslationExternalSourceRevision(target.FinalPath, "")
		if err != nil || current.key() != target.SourceRevision {
			return staleTranslationSourceError("translated sidecar changed after quality analysis")
		}
	}
	return validateTranslationQualityTargetPaths(request, source, mediaPath, targets)
}

func validateTranslationQualityTargetPaths(request subtitleTranslationOperationRequest, source subtitleTranslationSourceResponse, mediaPath string, targets []subtitleTranslationQualityTarget) error {
	outputs := make(map[string][]byte, len(targets))
	for _, target := range targets {
		if target.Artifact.Translation == nil || target.Artifact.Mode != "translation" || target.Artifact.Format != target.Format {
			return errors.New("translation review target metadata is invalid")
		}
		identity, err := translationIdentityFromMetadata(*target.Artifact.Translation)
		if err != nil || !translationArtifactMatchesIdentity(target.Artifact, identity) {
			return errors.New("translation review target provenance is invalid")
		}
		outputs[target.Format] = nil
	}
	if len(outputs) == 0 {
		return errors.New("translation review has no publication targets")
	}
	effectiveLanguage := targets[0].Artifact.Translation.EffectiveSourceLanguage
	expected, err := translationPublicationTargets(request, source, effectiveLanguage, TranslationProviderMetrics{}, "quality-review", outputs, mediaPath)
	if err != nil || len(expected) != len(targets) {
		return errors.New("translation review destination identity could not be verified")
	}
	expectedByFormat := make(map[string]string, len(expected))
	for _, target := range expected {
		expectedByFormat[target.Format] = filepath.Clean(target.FinalPath)
	}
	for _, target := range targets {
		if filepath.Clean(target.FinalPath) != expectedByFormat[target.Format] {
			return errors.New("translation review destination no longer matches its canonical identity")
		}
	}
	return nil
}

func applySubtitleTranslationQualityText(format string, source subtitleTranslationSourceResponse, content []byte, finding subtitleTranslationQualityFinding, text string) ([]byte, error) {
	if finding.SourceOrdinal < 0 || strings.TrimSpace(finding.CueID) == "" {
		return nil, errors.New("translation quality cue identity is invalid")
	}
	if format == translationRepresentationSRT {
		cues := parseSRTBitmapSubtitleCues(string(content))
		translated := make(map[int]string, len(cues))
		found := false
		for _, cue := range cues {
			translated[cue.SourceOrdinal] = cue.Text
			if cue.SourceOrdinal == finding.SourceOrdinal {
				translated[cue.SourceOrdinal], found = text, true
			}
		}
		if !found {
			return nil, errors.New("translation cue is missing from the published sidecar")
		}
		return serializeTranslatedTextCues(translationRepresentationSRT, cues, translated, translationRepresentationSRT)
	}
	document, err := parseTranslationASSDocument(format, string(content))
	if err != nil {
		return nil, err
	}
	cues := document.TranslationSourceCues(source.SourceIdentity, "")
	translated := make(map[int]string, len(cues))
	found := false
	for _, cue := range cues {
		translated[cue.SourceOrdinal] = cue.Text
		if cue.SourceOrdinal == finding.SourceOrdinal {
			translated[cue.SourceOrdinal], found = text, true
		}
	}
	if !found {
		return nil, errors.New("translation cue is missing from the published sidecar")
	}
	return serializeTranslatedASSDocument(document, translated, format)
}
