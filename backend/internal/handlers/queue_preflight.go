package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	queuePreflightReady          = "ready"
	queuePreflightRequiresReview = "requires_review"
	queuePreflightBlocked        = "blocked"
	queuePreflightQueued         = "queued"
)

var errQueueBatchPreflightConflict = errors.New("Queue Preflight candidate revision changed or candidate is terminal")

func revalidateAcceptedDeepCadenceEvidence(tx *gorm.DB, job models.QueueJob) error {
	interlace, ok := decodeInterlaceAnalysis(job.ProfileSnapshot[interlaceAnalysisSnapshotKey])
	if !ok || interlace.IVTCValidation == nil || interlace.IVTCValidation.DeepAnalysisVersion == 0 {
		return nil
	}
	validation := interlace.IVTCValidation
	identity := validation.SourceIdentity
	if validation.DeepAnalysisStatus != "resolved" || validation.SelectedOrder == "" || identity == nil || validation.EvidenceSnapshotUpdatedAt == "" {
		return fmt.Errorf("deep cadence evidence is unresolved; resolve Queue Preflight again")
	}
	info, err := os.Stat(job.MediaPath)
	if err != nil {
		return fmt.Errorf("deep cadence source is unavailable; resolve Queue Preflight again")
	}
	var latest models.ScanResult
	if err := tx.Where("path = ?", filepath.Clean(job.MediaPath)).Order("updated_at desc, id desc").First(&latest).Error; err != nil || latest.ID != identity.SnapshotID || latest.UpdatedAt.UTC().Format(time.RFC3339Nano) != validation.EvidenceSnapshotUpdatedAt {
		return fmt.Errorf("deep cadence snapshot changed after Queue Preflight; resolve the candidate again")
	}
	if !deepCadenceIdentityCurrent(tx, latest, job.MediaPath, info, identity) {
		return fmt.Errorf("deep cadence physical source identity changed after Queue Preflight; resolve the candidate again")
	}
	if identity.AssetRecordID > 0 && (job.SourceAssetRecordID == nil || *job.SourceAssetRecordID != identity.AssetRecordID) {
		return fmt.Errorf("deep cadence asset identity changed after Queue Preflight; resolve the candidate again")
	}
	current, ok := decodeInterlaceAnalysis(latest.InterlaceAnalysis)
	if !ok || current.IVTCValidation == nil || current.IVTCValidation.DeepAnalysisVersion != ivtcDeepEvidenceVersion ||
		current.IVTCValidation.SelectedOrder != validation.SelectedOrder || current.IVTCValidation.SourceIdentity == nil || *current.IVTCValidation.SourceIdentity != *identity {
		return fmt.Errorf("deep cadence evidence changed after Queue Preflight; resolve the candidate again")
	}
	return nil
}

func queuePreflightCadenceReview(profile models.Profile, snapshot models.JSONMap) *QueueBatchPreflightCadenceReview {
	interlace, _ := decodeInterlaceAnalysis(snapshot[interlaceAnalysisSnapshotKey])
	cadence, _ := decodeCadenceAnalysis(snapshot[cadenceAnalysisSnapshotKey])
	recommendation, _ := decodeCadenceRecommendation(snapshot[cadenceRecommendationSnapshotKey])
	review := &QueueBatchPreflightCadenceReview{
		CadenceType:                     cadence.Type,
		DeclaredFrameRate:               cadence.DeclaredFrameRate,
		RecommendedOperation:            recommendation.Operation,
		RecommendedFrameRate:            recommendation.OutputFrameRate,
		ReportedFieldOrder:              interlace.FieldOrder,
		EffectiveOutputFrameRate:        workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"]),
		EffectiveOutputFrameRateUnknown: profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false),
		HEVCLevelResolutionState:        workerStringValue(profile.WorkerConfig["hevcLevelResolutionState"]),
		HEVCLevelResolutionWarning:      workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]),
		HEVCLevelEffective:              workerStringValue(profile.WorkerConfig["hevcLevelEffective"]),
		HEVCLevelTier:                   workerStringValue(profile.WorkerConfig["hevcLevelTier"]),
		DeepAnalysisStatus:              "not_run",
	}
	if interlace.IVTCValidation != nil {
		review.ValidatedSelectedOrder = strings.ToLower(strings.TrimSpace(interlace.IVTCValidation.SelectedOrder))
		review.ValidWindows = interlace.IVTCValidation.ValidWindows
		review.ValidatedWindows = interlace.IVTCValidation.ValidatedWindows
		review.ObservedWindows = len(interlace.IVTCValidation.Windows)
		review.AttemptedWindows = interlace.IVTCValidation.AttemptedWindows
		if review.AttemptedWindows == 0 {
			review.AttemptedWindows = review.ObservedWindows
		}
		review.InconclusiveWindows = interlace.IVTCValidation.InconclusiveWindows
		review.ContradictoryWindows = interlace.IVTCValidation.ContradictoryWindows
		if interlace.IVTCValidation.DeepAnalysisVersion > 0 {
			review.DeepAnalysisVersion = interlace.IVTCValidation.DeepAnalysisVersion
			review.DeepAnalysisStatus = interlace.IVTCValidation.DeepAnalysisStatus
			review.SourceFrameRate = interlace.IVTCValidation.SourceFrameRate
			review.ExpectedOutputFrameRate = interlace.IVTCValidation.ExpectedOutputFrameRate
		}
		for _, window := range interlace.IVTCValidation.Windows {
			outcome := window.Outcome
			if outcome == "" {
				switch {
				case window.SelectedOrder == "":
					outcome = ivtcWindowOutcomeInconclusive
				case review.ValidatedSelectedOrder != "" && window.SelectedOrder != review.ValidatedSelectedOrder:
					outcome = ivtcWindowOutcomeContradictory
				default:
					outcome = ivtcWindowOutcomeValid
				}
			}
			review.Windows = append(review.Windows, QueueBatchPreflightCadenceWindow{
				Start: window.Start, Outcome: outcome, Reason: window.Reason,
				SelectionBasis: window.SelectionBasis, SelectedOrder: window.SelectedOrder,
				TFFProgressiveRatio: window.TFFProgressiveRatio, BFFProgressiveRatio: window.BFFProgressiveRatio,
				Confidence: window.Confidence,
			})
			if interlace.IVTCValidation.InconclusiveWindows == 0 && outcome == ivtcWindowOutcomeInconclusive {
				review.InconclusiveWindows++
			}
			if interlace.IVTCValidation.ValidWindows == 0 && outcome == ivtcWindowOutcomeValid {
				review.ValidWindows++
			}
			if interlace.IVTCValidation.ContradictoryWindows == 0 && outcome == ivtcWindowOutcomeContradictory {
				review.ContradictoryWindows++
			}
		}
		review.DeepAnalysisAvailable = review.ObservedWindows >= 3 && review.ObservedWindows < 5 && interlace.IVTCValidation.DeepAnalysisVersion == 0 ||
			interlace.IVTCValidation.DeepAnalysisVersion == ivtcDeepEvidenceVersion && interlace.IVTCValidation.DeepAnalysisStatus == "stale" && interlace.IVTCValidation.BaselineWindowCount >= 3
	}
	if review.CadenceType == "" && review.RecommendedOperation == "" && review.ReportedFieldOrder == "" {
		return nil
	}
	return review
}

func writeQueueBatchPreflightMutationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue Preflight candidate was not found"})
	case errors.Is(err, errQueueBatchPreflightConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func queuePreflightFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveQueuePreflightEffectiveProfile(profile models.Profile, selectedEncoder string, scan models.ScanResult, path string) models.Profile {
	profile = applySelectedEncoder(profile, selectedEncoder)
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["qsvAssetAnalysisPath"] = filepath.Clean(path)
	profile = normalizeHardwareQualityPreset(profile)
	streams := mediaStreamInventoryFromScan(scan)
	if mediaProcessingMode(profile) != ProcessingModeFullEncode {
		return resolveEffectiveVideoEncodingProfile(profile, streams, path)
	}
	interlace, ok := decodeInterlaceAnalysis(scan.InterlaceAnalysis)
	if !ok {
		interlace, _ = decodeInterlaceAnalysis(profile.WorkerConfig[interlaceAnalysisSnapshotKey])
	}
	cadence, ok := decodeCadenceAnalysis(scan.CadenceAnalysis)
	if !ok {
		cadence, _ = decodeCadenceAnalysis(profile.WorkerConfig[cadenceAnalysisSnapshotKey])
	}
	recommendation, ok := decodeCadenceRecommendation(scan.CadenceRecommendation)
	if !ok {
		recommendation, ok = decodeCadenceRecommendation(profile.WorkerConfig[cadenceRecommendationSnapshotKey])
	}
	if !ok {
		recommendation = recommendCadence(cadence)
	}
	return resolveMediaJobVideoProfile(profile, streams, path, interlace, cadence, recommendation)
}

type QueueBatchPreflightInput struct {
	Revision  uint            `json:"revision,omitempty"`
	BatchID   string          `json:"batchId" binding:"required"`
	BatchName string          `json:"batchName"`
	Jobs      []QueueJobInput `json:"jobs" binding:"required,min=1,max=1000,dive"`
}

type QueueBatchPreflightAcceptInput struct {
	Revision uint     `json:"revision" binding:"required"`
	AssetIDs []string `json:"assetIds" binding:"required,min=1,max=1000"`
}

type QueueBatchPreflightRemoveInput struct {
	Revision uint   `json:"revision" binding:"required"`
	AssetID  string `json:"assetId" binding:"required"`
}

type QueueBatchPreflightDiscardInput struct {
	Revision uint `json:"revision" binding:"required"`
}

type queueBatchPreflightState struct {
	BatchID             string                     `json:"batchId"`
	BatchName           string                     `json:"batchName"`
	RuntimeSnapshotID   uint                       `json:"runtimeSnapshotId,omitempty"`
	RuntimeError        string                     `json:"runtimeError,omitempty"`
	AllowMultipleGroups bool                       `json:"allowMultipleGroups,omitempty"`
	Assets              []queueBatchPreflightAsset `json:"assets"`
	QueuedJobs          []models.QueueJob          `json:"queuedJobs,omitempty"`
	ResolvedAt          time.Time                  `json:"resolvedAt"`
}

type queueBatchPreflightAsset struct {
	ID              string                            `json:"id"`
	MediaPath       string                            `json:"mediaPath"`
	AssetRecordID   *uint                             `json:"assetRecordId,omitempty"`
	SourceRevision  string                            `json:"sourceRevision,omitempty"`
	BatchID         string                            `json:"batchId"`
	BatchName       string                            `json:"batchName"`
	BatchPosition   int                               `json:"batchPosition"`
	Input           QueueJobInput                     `json:"input"`
	Status          string                            `json:"status"`
	ReviewCode      string                            `json:"reviewCode,omitempty"`
	CadenceReview   *QueueBatchPreflightCadenceReview `json:"cadenceReview,omitempty"`
	Blockers        []string                          `json:"blockers"`
	Warnings        []string                          `json:"warnings"`
	ResolvedJob     *models.QueueJob                  `json:"resolvedJob,omitempty"`
	Constraints     models.JSONMap                    `json:"constraints,omitempty"`
	QualityEvidence models.JSONMap                    `json:"qualityEvidence,omitempty"`
	SelectedEncoder string                            `json:"selectedEncoder,omitempty"`
	RuntimeProfile  string                            `json:"runtimeProfile,omitempty"`
	QueuedJobID     uint                              `json:"queuedJobId,omitempty"`
}

type QueueBatchPreflightCadenceReview struct {
	CadenceType                     string                             `json:"cadenceType,omitempty"`
	DeclaredFrameRate               string                             `json:"declaredFrameRate,omitempty"`
	RecommendedOperation            string                             `json:"recommendedOperation,omitempty"`
	RecommendedFrameRate            string                             `json:"recommendedFrameRate,omitempty"`
	ReportedFieldOrder              string                             `json:"reportedFieldOrder,omitempty"`
	ValidatedSelectedOrder          string                             `json:"validatedSelectedOrder,omitempty"`
	ValidatedWindows                int                                `json:"validatedWindows"`
	ValidWindows                    int                                `json:"validWindows"`
	ObservedWindows                 int                                `json:"observedWindows"`
	EffectiveOutputFrameRate        string                             `json:"effectiveOutputFrameRate,omitempty"`
	EffectiveOutputFrameRateUnknown bool                               `json:"effectiveOutputFrameRateUnknown"`
	HEVCLevelResolutionState        string                             `json:"hevcLevelResolutionState,omitempty"`
	HEVCLevelResolutionWarning      string                             `json:"hevcLevelResolutionWarning,omitempty"`
	HEVCLevelEffective              string                             `json:"hevcLevelEffective,omitempty"`
	HEVCLevelTier                   string                             `json:"hevcLevelTier,omitempty"`
	DeepAnalysisAvailable           bool                               `json:"deepAnalysisAvailable"`
	DeepAnalysisVersion             int                                `json:"deepAnalysisVersion,omitempty"`
	DeepAnalysisStatus              string                             `json:"deepAnalysisStatus"`
	SourceFrameRate                 string                             `json:"sourceFrameRate,omitempty"`
	ExpectedOutputFrameRate         string                             `json:"expectedOutputFrameRate,omitempty"`
	AttemptedWindows                int                                `json:"attemptedWindows"`
	InconclusiveWindows             int                                `json:"inconclusiveWindows"`
	ContradictoryWindows            int                                `json:"contradictoryWindows"`
	Windows                         []QueueBatchPreflightCadenceWindow `json:"windows,omitempty"`
}

type QueueBatchPreflightCadenceWindow struct {
	Start               float64 `json:"start"`
	Outcome             string  `json:"outcome,omitempty"`
	Reason              string  `json:"reason,omitempty"`
	SelectionBasis      string  `json:"selectionBasis,omitempty"`
	SelectedOrder       string  `json:"selectedOrder,omitempty"`
	TFFProgressiveRatio float64 `json:"tffProgressiveRatio"`
	BFFProgressiveRatio float64 `json:"bffProgressiveRatio"`
	Confidence          float64 `json:"confidence"`
}

type QueueBatchPreflightPlan struct {
	Profile              models.Profile `json:"profile"`
	ProfileSnapshot      models.JSONMap `json:"profileSnapshot"`
	ProfileVersion       int            `json:"profileVersion"`
	ProfileCapturedAt    *time.Time     `json:"profileCapturedAt,omitempty"`
	ProfileResolution    models.JSONMap `json:"profileResolution"`
	AudioProfileSnapshot models.JSONMap `json:"audioProfileSnapshot"`
	TrackProfileSnapshot models.JSONMap `json:"trackProfileSnapshot"`
	Constraints          models.JSONMap `json:"constraints"`
	QualityEvidence      models.JSONMap `json:"qualityEvidence,omitempty"`
	SelectedEncoder      string         `json:"selectedEncoder"`
	RuntimeSnapshotID    uint           `json:"runtimeSnapshotId"`
	RuntimeProfile       string         `json:"runtimeProfile"`
	DestinationSnapshot  models.JSONMap `json:"destination"`
	PublishMode          string         `json:"publishMode"`
	ProcessingMode       string         `json:"processingMode"`
}

type QueueBatchPreflightAssetResponse struct {
	ID            string                            `json:"id"`
	MediaPath     string                            `json:"mediaPath"`
	AssetRecordID *uint                             `json:"assetRecordId,omitempty"`
	BatchID       string                            `json:"batchId"`
	BatchName     string                            `json:"batchName"`
	Input         QueueJobInput                     `json:"input"`
	Status        string                            `json:"status"`
	ReviewCode    string                            `json:"reviewCode,omitempty"`
	CadenceReview *QueueBatchPreflightCadenceReview `json:"cadenceReview,omitempty"`
	Blockers      []string                          `json:"blockers"`
	Warnings      []string                          `json:"warnings"`
	QueuedJobID   uint                              `json:"queuedJobId,omitempty"`
	Plan          *QueueBatchPreflightPlan          `json:"plan,omitempty"`
}

type QueueBatchPreflightResponse struct {
	ID        string                             `json:"id"`
	Revision  uint                               `json:"revision"`
	Resolved  bool                               `json:"resolved"`
	Status    string                             `json:"status"`
	BatchID   string                             `json:"batchId"`
	BatchName string                             `json:"batchName"`
	Assets    []QueueBatchPreflightAssetResponse `json:"assets"`
	Jobs      []models.QueueJob                  `json:"jobs,omitempty"`
}

func queueBatchPreflightID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate Queue Preflight identity: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func queuePreflightStateMap(state queueBatchPreflightState) (models.JSONMap, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	value := models.JSONMap{}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func queuePreflightStateFromRecord(record models.QueueBatchPreflight) (queueBatchPreflightState, error) {
	encoded, err := json.Marshal(record.State)
	if err != nil {
		return queueBatchPreflightState{}, err
	}
	var state queueBatchPreflightState
	if err := json.Unmarshal(encoded, &state); err != nil {
		return queueBatchPreflightState{}, err
	}
	return state, nil
}

func queuePreflightSourceRevision(path string) (string, error) {
	revision, err := sourceRevisionForPath(path)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(revision)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func queuePreflightResponse(record models.QueueBatchPreflight, state queueBatchPreflightState) QueueBatchPreflightResponse {
	response := QueueBatchPreflightResponse{
		ID: record.ID, Revision: record.Revision, Resolved: record.Resolved, Status: record.Status,
		BatchID: state.BatchID, BatchName: state.BatchName,
		Assets: make([]QueueBatchPreflightAssetResponse, 0, len(state.Assets)), Jobs: state.QueuedJobs,
	}
	for _, asset := range state.Assets {
		reviewCode, cadenceReview := asset.ReviewCode, asset.CadenceReview
		if asset.Status != queuePreflightRequiresReview {
			reviewCode = ""
			if cadenceReview == nil || cadenceReview.DeepAnalysisVersion == 0 {
				cadenceReview = nil
			}
		}
		item := QueueBatchPreflightAssetResponse{
			ID: asset.ID, MediaPath: asset.MediaPath, AssetRecordID: asset.AssetRecordID,
			BatchID: asset.BatchID, BatchName: asset.BatchName, Input: asset.Input,
			Status: asset.Status, ReviewCode: reviewCode, CadenceReview: cadenceReview, Blockers: asset.Blockers, Warnings: asset.Warnings, QueuedJobID: asset.QueuedJobID,
		}
		if asset.ResolvedJob != nil {
			job := asset.ResolvedJob
			profile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
			if err == nil {
				item.Plan = &QueueBatchPreflightPlan{
					Profile: profile, ProfileSnapshot: job.ProfileSnapshot, ProfileResolution: job.ProfileResolution,
					ProfileVersion: job.ProfileVersion, ProfileCapturedAt: job.ProfileCapturedAt,
					AudioProfileSnapshot: job.AudioProfileSnapshot, TrackProfileSnapshot: job.TrackProfileSnapshot,
					Constraints: asset.Constraints, SelectedEncoder: asset.SelectedEncoder,
					QualityEvidence:   asset.QualityEvidence,
					RuntimeSnapshotID: state.RuntimeSnapshotID, RuntimeProfile: asset.RuntimeProfile,
					DestinationSnapshot: job.ResolvedDestinationSnapshot,
					PublishMode:         job.PublishMode, ProcessingMode: job.ProcessingMode,
				}
			}
		}
		response.Assets = append(response.Assets, item)
	}
	return response
}

func queuePreflightHasPendingAssets(state queueBatchPreflightState) bool {
	for _, asset := range state.Assets {
		if asset.Status != queuePreflightQueued {
			return true
		}
	}
	return false
}

func (h QueueHandler) CreateBatchPreflight(c *gin.Context) {
	var input QueueBatchPreflightInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for index := range input.Jobs {
		input.Jobs[index].ProfileResolution = nil
		input.Jobs[index].resolvedProfileResolution = nil
		input.Jobs[index].skipProfileAssignmentResolve = false
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	state, err := h.resolveQueueBatchPreflight(input, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := queueBatchPreflightID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	stateMap, err := queuePreflightStateMap(state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encode Queue Preflight candidate: " + err.Error()})
		return
	}
	record := models.QueueBatchPreflight{ID: id, Revision: 1, Resolved: true, Status: models.QueueBatchPreflightStatusActive, State: stateMap}
	if err := h.db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist Queue Preflight candidate: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, queuePreflightResponse(record, state))
}

func (QueueHandler) RequireBatchPreflight(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{
		"error": "batch queue creation requires Batch Preflight and explicit acceptance",
		"code":  "batch_preflight_required",
	})
}

func (h QueueHandler) PreflightSelectedAssets(c *gin.Context) {
	var input struct {
		AssetIDs []uint `json:"assetIds" binding:"required,min=1,max=1000"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	seen := map[uint]bool{}
	ids := make([]uint, 0, len(input.AssetIDs))
	for _, id := range input.AssetIDs {
		if id != 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds must contain a valid asset id"})
		return
	}
	var records []models.AssetRecord
	if err := h.db.Where("id IN ?", ids).Find(&records).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load selected assets: " + err.Error()})
		return
	}
	if len(records) != len(ids) {
		c.JSON(http.StatusConflict, gin.H{"error": "one or more selected assets are no longer available"})
		return
	}
	sort.SliceStable(records, func(i, j int) bool {
		leftLogical, rightLogical := filepath.Clean(records[i].LogicalGroupPath), filepath.Clean(records[j].LogicalGroupPath)
		if comparison := naturalAssetCompare(leftLogical, rightLogical); comparison != 0 {
			return comparison < 0
		}
		if comparison := naturalAssetCompare(selectedAssetPhysicalGroup(records[i]), selectedAssetPhysicalGroup(records[j])); comparison != 0 {
			return comparison < 0
		}
		return assetSequenceLess(records[i].Path, records[j].Path)
	})
	configurationResolver, err := loadAssetConfigurationResolver(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load effective configuration batch: " + err.Error()})
		return
	}
	candidateID, err := queueBatchPreflightID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rootBatchID := "selected-" + candidateID
	groupIDs, groupNames, groupPositions := map[string]string{}, map[string]string{}, map[string]int{}
	jobs := make([]QueueJobInput, 0, len(records))
	for _, record := range records {
		groupPath := selectedAssetPhysicalGroup(record)
		groupID, exists := groupIDs[groupPath]
		if !exists {
			groupID = fmt.Sprintf("%s-%03d", rootBatchID, len(groupIDs)+1)
			groupIDs[groupPath] = groupID
			name := filepath.Base(groupPath)
			if name == "." || name == string(filepath.Separator) || strings.TrimSpace(name) == "" {
				name = "Selected assets"
			}
			groupNames[groupPath] = name
		}
		groupPositions[groupPath]++
		effective := configurationResolver.resolve(record)
		processingMode := ProcessingModeAudioOnly
		if effective.Video.VideoProfileID > 0 || effective.Video.Selection == VideoAssignmentOverrideOnly || effective.Video.Selection == configSelectionValue {
			processingMode = ProcessingModeFullEncode
		}
		queueInput := QueueJobInput{
			MediaPath: record.Path, SourceAssetRecordID: &record.ID,
			PublishMode: PublishModeStandard, BatchID: groupID, BatchName: groupNames[groupPath],
			BatchPosition: groupPositions[groupPath], LibraryID: effective.Destination.DestinationLibraryID,
			ProfileID: effective.Video.VideoProfileID, AudioProfileKey: effective.Audio.ProfileKey,
			TrackProfileKey: effective.Tracks.ProfileKey, ProcessingMode: processingMode, Priority: 5,
			Notes: "Queued from selected assets", ResolveProfileAssignments: true,
		}
		queueInput.resolvedProfileResolution = resolveProfileAssignmentsFromEffective(&queueInput, effective)
		queueInput.ProfileResolution = queueInput.resolvedProfileResolution
		queueInput.skipProfileAssignmentResolve = true
		if record.Status != "unprocessed" {
			queueInput.preflightBlockers = append(queueInput.preflightBlockers, "Asset is not currently Unprocessed")
		}
		if record.Missing {
			queueInput.preflightBlockers = append(queueInput.preflightBlockers, "Asset file is missing")
		}
		hasOperation := effective.Video.VideoProfileID > 0 || effective.Video.Selection == VideoAssignmentOverrideOnly || effective.Video.Selection == VideoAssignmentAudioOnly || effective.Video.Selection == configSelectionValue || strings.TrimSpace(effective.Audio.ProfileKey) != "" || strings.TrimSpace(effective.Tracks.ProfileKey) != "" || effective.Audio.Selection == configSelectionValue || effective.Tracks.Selection == configSelectionValue
		if effective.Destination.Selection != configSelectionValue || effective.Destination.DestinationLibraryID == 0 {
			queueInput.preflightBlockers = append(queueInput.preflightBlockers, "No effective destination is configured")
		}
		if !hasOperation {
			queueInput.preflightBlockers = append(queueInput.preflightBlockers, "No effective processing operation is configured")
		}
		jobs = append(jobs, queueInput)
	}
	state, err := h.resolveQueueBatchPreflight(QueueBatchPreflightInput{BatchID: rootBatchID, BatchName: "Selected assets", Jobs: jobs}, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stateMap, err := queuePreflightStateMap(state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encode Queue Preflight candidate: " + err.Error()})
		return
	}
	record := models.QueueBatchPreflight{ID: candidateID, Revision: 1, Resolved: true, Status: models.QueueBatchPreflightStatusActive, State: stateMap}
	if err := h.db.Create(&record).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist Queue Preflight candidate: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, queuePreflightResponse(record, state))
}

func (h QueueHandler) GetBatchPreflight(c *gin.Context) {
	record, state, ok := h.loadBatchPreflight(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, queuePreflightResponse(record, state))
}

func (h QueueHandler) EditBatchPreflight(c *gin.Context) {
	var input struct {
		Revision uint `json:"revision" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var record models.QueueBatchPreflight
	if err := h.db.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue Preflight candidate was not found"})
		return
	}
	if record.Revision != input.Revision {
		c.JSON(http.StatusConflict, gin.H{"error": "Queue Preflight revision is stale"})
		return
	}
	if record.Status != models.QueueBatchPreflightStatusActive {
		c.JSON(http.StatusConflict, gin.H{"error": "Queue Preflight candidate is terminal"})
		return
	}
	state, err := queuePreflightStateFromRecord(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decode Queue Preflight candidate: " + err.Error()})
		return
	}
	result := h.db.Model(&record).Where("id = ? AND revision = ? AND status = ?", record.ID, record.Revision, models.QueueBatchPreflightStatusActive).Updates(map[string]any{
		"revision": record.Revision + 1, "resolved": false,
	})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected != 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "Queue Preflight revision changed while editing"})
		return
	}
	record.Revision = input.Revision + 1
	record.Resolved = false
	c.JSON(http.StatusOK, queuePreflightResponse(record, state))
}

func (h QueueHandler) UpdateBatchPreflight(c *gin.Context) {
	var input QueueBatchPreflightInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var record models.QueueBatchPreflight
	if err := h.db.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue Preflight candidate was not found"})
		return
	}
	if record.Revision != input.Revision {
		c.JSON(http.StatusConflict, gin.H{"error": "Queue Preflight revision is stale"})
		return
	}
	if record.Status != models.QueueBatchPreflightStatusActive {
		c.JSON(http.StatusConflict, gin.H{"error": "Queue Preflight candidate is terminal"})
		return
	}
	previous, err := queuePreflightStateFromRecord(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decode Queue Preflight candidate: " + err.Error()})
		return
	}
	previousAssets := make(map[string]queueBatchPreflightAsset, len(previous.Assets))
	for _, asset := range previous.Assets {
		if asset.Status != queuePreflightQueued {
			previousAssets[filepath.Clean(asset.MediaPath)] = asset
		}
	}
	seenPaths := make(map[string]bool, len(input.Jobs))
	if len(previousAssets) != len(input.Jobs) {
		c.JSON(http.StatusConflict, gin.H{"error": "Edit must preserve every unresolved candidate asset"})
		return
	}
	for index := range input.Jobs {
		submitted := input.Jobs[index]
		path := filepath.Clean(submitted.MediaPath)
		previousAsset, exists := previousAssets[path]
		if !exists || seenPaths[path] {
			c.JSON(http.StatusConflict, gin.H{"error": "Edit contains a changed or duplicate candidate asset identity"})
			return
		}
		seenPaths[path] = true
		input.Jobs[index].candidateInput = &submitted
		previousResolution := previousAsset.Input.ProfileResolution
		if previousAsset.ResolvedJob != nil {
			previousResolution = previousAsset.ResolvedJob.ProfileResolution
		}
		input.Jobs[index].ProfileResolution = queuePreflightEditedResolution(previousResolution, previousAsset.Input, submitted)
		input.Jobs[index].candidateInput.ProfileResolution = input.Jobs[index].ProfileResolution
		input.Jobs[index].resolvedProfileResolution = input.Jobs[index].ProfileResolution
		input.Jobs[index].ResolveProfileAssignments = false
		input.Jobs[index].skipProfileAssignmentResolve = true
		input.Jobs[index].SourceAssetRecordID = previousAsset.AssetRecordID
		if previousAsset.ResolvedJob != nil {
			previousJob := previousAsset.ResolvedJob
			if previousAsset.Input.ProfileID == submitted.ProfileID {
				input.Jobs[index].ProfileID = previousJob.ProfileID
			}
			if previousAsset.Input.AudioProfileKey == submitted.AudioProfileKey {
				input.Jobs[index].AudioProfileKey = previousJob.AudioProfileKey
				input.Jobs[index].preservedAudioSnapshot = queuePreflightCloneMap(previousJob.AudioProfileSnapshot)
			}
			if previousAsset.Input.TrackProfileKey == submitted.TrackProfileKey {
				input.Jobs[index].TrackProfileKey = previousJob.TrackProfileKey
				input.Jobs[index].preservedTrackSnapshot = queuePreflightCloneMap(previousJob.TrackProfileSnapshot)
			}
			if previousAsset.Input.ProfileID == submitted.ProfileID && previousAsset.Input.AudioProfileKey == submitted.AudioProfileKey && previousAsset.Input.TrackProfileKey == submitted.TrackProfileKey && previousAsset.Input.CadenceModeOverride == submitted.CadenceModeOverride && previousAsset.Input.CadenceFieldOrder == submitted.CadenceFieldOrder {
				input.Jobs[index].preservedProfileSnapshot = queuePreflightCloneMap(previousJob.ProfileSnapshot)
				input.Jobs[index].preservedProfileVersion = previousJob.ProfileVersion
				input.Jobs[index].preservedProfileCapturedAt = previousJob.ProfileCapturedAt
			}
			input.Jobs[index].preservedConversionOverride = queuePreflightOverrideFromSnapshot(previousJob.ProfileSnapshot)
			if previousAsset.Input.LibraryID == submitted.LibraryID {
				input.Jobs[index].preservedDestinationSnapshot = queuePreflightCloneMap(previousJob.ResolvedDestinationSnapshot)
			}
		}
	}
	state, err := h.resolveQueueBatchPreflight(input, previous.AllowMultipleGroups)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, asset := range previous.Assets {
		if asset.Status == queuePreflightQueued {
			state.Assets = append(state.Assets, asset)
		}
	}
	state.QueuedJobs = previous.QueuedJobs
	stateMap, err := queuePreflightStateMap(state)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encode Queue Preflight candidate: " + err.Error()})
		return
	}
	result := h.db.Model(&record).Where("id = ? AND revision = ? AND status = ?", record.ID, input.Revision, models.QueueBatchPreflightStatusActive).Updates(map[string]any{
		"revision": record.Revision + 1, "resolved": true, "state": stateMap,
	})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected != 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "Queue Preflight revision changed while resolving"})
		return
	}
	record.Revision = input.Revision + 1
	record.Resolved, record.State = true, stateMap
	c.JSON(http.StatusOK, queuePreflightResponse(record, state))
}

func (h QueueHandler) RemoveBatchPreflightAsset(c *gin.Context) {
	var input QueueBatchPreflightRemoveInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	assetID := strings.TrimSpace(input.AssetID)
	if assetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetId is required"})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var record models.QueueBatchPreflight
	var state queueBatchPreflightState
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
			return err
		}
		if record.Revision != input.Revision || record.Status != models.QueueBatchPreflightStatusActive {
			return errQueueBatchPreflightConflict
		}
		var err error
		state, err = queuePreflightStateFromRecord(record)
		if err != nil {
			return fmt.Errorf("decode Queue Preflight candidate: %w", err)
		}
		assetIndex := -1
		for index := range state.Assets {
			if state.Assets[index].ID == assetID {
				assetIndex = index
				break
			}
		}
		if assetIndex < 0 {
			return fmt.Errorf("%w: candidate asset is no longer present", errQueueBatchPreflightConflict)
		}
		if state.Assets[assetIndex].Status == queuePreflightQueued {
			return fmt.Errorf("%w: accepted assets cannot be removed", errQueueBatchPreflightConflict)
		}
		state.Assets = append(state.Assets[:assetIndex], state.Assets[assetIndex+1:]...)
		status := record.Status
		if !queuePreflightHasPendingAssets(state) {
			status = models.QueueBatchPreflightStatusDiscarded
		}
		stateMap, err := queuePreflightStateMap(state)
		if err != nil {
			return fmt.Errorf("encode Queue Preflight candidate: %w", err)
		}
		revision := record.Revision + 1
		result := tx.Model(&record).Where("id = ? AND revision = ? AND status = ?", record.ID, record.Revision, models.QueueBatchPreflightStatusActive).Updates(map[string]any{
			"revision": revision, "status": status, "state": stateMap,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errQueueBatchPreflightConflict
		}
		record.Revision, record.Status, record.State = revision, status, stateMap
		return nil
	})
	if err != nil {
		writeQueueBatchPreflightMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, queuePreflightResponse(record, state))
}

func (h QueueHandler) DiscardBatchPreflight(c *gin.Context) {
	var input QueueBatchPreflightDiscardInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var record models.QueueBatchPreflight
	var state queueBatchPreflightState
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
			return err
		}
		if record.Revision != input.Revision || record.Status != models.QueueBatchPreflightStatusActive {
			return errQueueBatchPreflightConflict
		}
		var err error
		state, err = queuePreflightStateFromRecord(record)
		if err != nil {
			return fmt.Errorf("decode Queue Preflight candidate: %w", err)
		}
		hadPending := queuePreflightHasPendingAssets(state)
		queued := make([]queueBatchPreflightAsset, 0, len(state.Assets))
		for _, asset := range state.Assets {
			if asset.Status == queuePreflightQueued {
				queued = append(queued, asset)
			}
		}
		state.Assets = queued
		status := models.QueueBatchPreflightStatusDiscarded
		if !hadPending && len(queued) > 0 {
			status = models.QueueBatchPreflightStatusCompleted
		}
		stateMap, err := queuePreflightStateMap(state)
		if err != nil {
			return fmt.Errorf("encode Queue Preflight candidate: %w", err)
		}
		revision := record.Revision + 1
		result := tx.Model(&record).Where("id = ? AND revision = ? AND status = ?", record.ID, record.Revision, models.QueueBatchPreflightStatusActive).Updates(map[string]any{
			"revision": revision, "status": status, "state": stateMap,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errQueueBatchPreflightConflict
		}
		record.Revision, record.Status, record.State = revision, status, stateMap
		return nil
	})
	if err != nil {
		writeQueueBatchPreflightMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, queuePreflightResponse(record, state))
}

func (h QueueHandler) resolveQueueBatchPreflight(input QueueBatchPreflightInput, allowMultipleGroups bool) (queueBatchPreflightState, error) {
	batchID := strings.TrimSpace(input.BatchID)
	if batchID == "" || len(input.Jobs) == 0 || len(input.Jobs) > 1000 {
		return queueBatchPreflightState{}, fmt.Errorf("batch id and between 1 and 1000 jobs are required")
	}
	jobs := append([]QueueJobInput(nil), input.Jobs...)
	if err := populateSourceAssetRecordIDs(h.db, jobs); err != nil {
		return queueBatchPreflightState{}, fmt.Errorf("resolve source asset identities: %w", err)
	}
	sort.SliceStable(jobs, func(i, j int) bool { return assetSequenceLess(jobs[i].MediaPath, jobs[j].MediaPath) })
	groupPaths := map[string]string{}
	seen := map[string]bool{}
	for i := range jobs {
		jobs[i].MediaPath = filepath.Clean(jobs[i].MediaPath)
		groupID := strings.TrimSpace(jobs[i].BatchID)
		if groupID == "" {
			groupID = batchID
		}
		if existing, ok := groupPaths[groupID]; ok && existing != filepath.Dir(jobs[i].MediaPath) {
			return queueBatchPreflightState{}, fmt.Errorf("all jobs in a Queue batch must belong to the same path")
		}
		if !allowMultipleGroups && len(groupPaths) == 0 {
			groupPaths[groupID] = filepath.Dir(jobs[i].MediaPath)
		} else if !allowMultipleGroups && groupPaths[groupID] != filepath.Dir(jobs[i].MediaPath) {
			return queueBatchPreflightState{}, fmt.Errorf("all jobs in a Queue batch must belong to the same path")
		} else if allowMultipleGroups {
			groupPaths[groupID] = filepath.Dir(jobs[i].MediaPath)
		}
		if seen[jobs[i].MediaPath] {
			return queueBatchPreflightState{}, fmt.Errorf("duplicate asset in batch: %s", jobs[i].MediaPath)
		}
		seen[jobs[i].MediaPath] = true
	}
	state := queueBatchPreflightState{
		BatchID: batchID, BatchName: strings.TrimSpace(input.BatchName), AllowMultipleGroups: allowMultipleGroups,
		Assets: make([]queueBatchPreflightAsset, 0, len(jobs)), ResolvedAt: time.Now().UTC(),
	}
	runtime, runtimeErr := scheduler.CaptureBatchPreflightRuntimeSnapshot(h.db)
	if runtimeErr == nil {
		state.RuntimeSnapshotID = runtime.ID
	} else {
		state.RuntimeError = runtimeErr.Error()
	}
	validation, err := newQueueBatchValidationContext(h.db)
	if err != nil {
		return queueBatchPreflightState{}, fmt.Errorf("load Queue validation context: %w", err)
	}
	paths := make([]string, 0, len(jobs))
	for _, job := range jobs {
		paths = append(paths, job.MediaPath)
	}
	reservedPaths, err := queuePreflightActiveReservations(h.db, paths)
	if err != nil {
		return queueBatchPreflightState{}, fmt.Errorf("load Queue reservations: %w", err)
	}
	sourceScans := map[string]models.ScanResult{}
	if h.db.Migrator().HasTable(&models.ScanResult{}) {
		var scans []models.ScanResult
		if h.db.Where("path IN ?", paths).Order("updated_at desc, id desc").Find(&scans).Error == nil {
			for _, scan := range scans {
				key := filepath.Clean(scan.Path)
				if _, exists := sourceScans[key]; !exists {
					sourceScans[key] = scan
				}
			}
		}
	}
	for index, input := range jobs {
		itemID, err := queueBatchPreflightID()
		if err != nil {
			return queueBatchPreflightState{}, err
		}
		itemInput := input
		if input.candidateInput != nil {
			itemInput = *input.candidateInput
			itemInput.ProfileResolution = input.ProfileResolution
		}
		item := queueBatchPreflightAsset{
			ID: itemID, MediaPath: input.MediaPath, AssetRecordID: input.SourceAssetRecordID,
			BatchID:       queuePreflightFirstNonEmpty(input.BatchID, batchID),
			BatchName:     queuePreflightFirstNonEmpty(input.BatchName, state.BatchName),
			BatchPosition: input.BatchPosition,
			Input:         itemInput, Status: queuePreflightBlocked, Blockers: []string{}, Warnings: []string{},
		}
		if item.BatchPosition <= 0 {
			item.BatchPosition = index + 1
		}
		if len(input.preflightBlockers) > 0 {
			item.Blockers = append(item.Blockers, input.preflightBlockers...)
			state.Assets = append(state.Assets, item)
			continue
		}
		if sourceRevision, err := queuePreflightSourceRevision(input.MediaPath); err == nil {
			item.SourceRevision = sourceRevision
		} else {
			item.Blockers = append(item.Blockers, "Source asset is missing or unreadable")
			state.Assets = append(state.Assets, item)
			continue
		}
		if reviewForPath(input.MediaPath, validation.reviews).RequiresReview {
			item.Status = queuePreflightRequiresReview
			item.Blockers = append(item.Blockers, "Asset requires review before queueing")
			state.Assets = append(state.Assets, item)
			continue
		}
		if reservedPaths[filepath.Clean(input.MediaPath)] {
			item.Blockers = append(item.Blockers, "Asset has an active Queue reservation")
			state.Assets = append(state.Assets, item)
			continue
		}
		if validation.assetHasOpenJob(input.MediaPath, 0) {
			item.Blockers = append(item.Blockers, "Asset already has an open Queue job")
			state.Assets = append(state.Assets, item)
			continue
		}
		if validation.activeMaintenance(input.MediaPath) {
			item.Blockers = append(item.Blockers, "Asset has an active maintenance operation")
			state.Assets = append(state.Assets, item)
			continue
		}
		input.validationContext = validation
		job, _, err := h.prepareBatchQueueJob(input)
		if err != nil {
			item.Blockers = append(item.Blockers, err.Error())
			state.Assets = append(state.Assets, item)
			continue
		}
		item.Input.ProfileResolution = job.ProfileResolution
		item.ResolvedJob = &job
		profile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
		if err == nil {
			err = freezeQueuePreparedProfileSnapshot(h.db, &job, profile)
		}
		if err != nil {
			item.Blockers = append(item.Blockers, "Resolve prepared Queue plan: "+err.Error())
			state.Assets = append(state.Assets, item)
			continue
		}
		profile, err = scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
		if err != nil {
			item.Blockers = append(item.Blockers, "Restore frozen profile snapshot: "+err.Error())
			state.Assets = append(state.Assets, item)
			continue
		}
		if normalizeQueueProcessingMode(job.ProcessingMode) == ProcessingModeFullEncode && cadenceFieldOrderReviewRequired(profile) {
			item.Status = queuePreflightRequiresReview
			item.ReviewCode = "ivtc_field_order_required"
			item.CadenceReview = queuePreflightCadenceReview(profile, job.ProfileSnapshot)
			item.Blockers = append(item.Blockers, "Automatic IVTC requires an explicit or validated field order; choose TFF or BFF for this asset.")
			item.Warnings = append(item.Warnings, "Resolve Preflight again to review the effective IVTC filters, output FPS, and HEVC Level/Tier before acceptance.")
			item.ResolvedJob = nil
			state.Assets = append(state.Assets, item)
			continue
		}
		constraints, err := scheduler.ResolveExecutionConstraints(profile)
		if err != nil {
			item.Blockers = append(item.Blockers, "Resolve execution constraints: "+err.Error())
			state.Assets = append(state.Assets, item)
			continue
		}
		item.Constraints, err = queueJSONMap(constraints)
		if err != nil {
			item.Blockers = append(item.Blockers, "Freeze execution constraints: "+err.Error())
			state.Assets = append(state.Assets, item)
			continue
		}
		if runtimeErr != nil {
			item.Blockers = append(item.Blockers, "Runtime capabilities are unavailable: "+runtimeErr.Error())
			state.Assets = append(state.Assets, item)
			continue
		}
		item.SelectedEncoder, _, _ = scheduler.ResolveExecutionPlanEncoder(profile, runtime.Encoders)
		if item.SelectedEncoder == "" {
			item.Blockers = append(item.Blockers, "No encoder allowed by the profile is usable in the current runtime")
			state.Assets = append(state.Assets, item)
			continue
		}
		if item.SelectedEncoder != constraints.PreferredEncoder {
			item.Warnings = append(item.Warnings, fmt.Sprintf("Encoder fallback: %s is selected instead of preferred %s", item.SelectedEncoder, constraints.PreferredEncoder))
		}
		job.ProfileSnapshot[models.QueuePreflightSelectedEncoderKey] = item.SelectedEncoder
		if scan, ok := sourceScans[filepath.Clean(job.MediaPath)]; ok {
			effectiveProfile := resolveQueuePreflightEffectiveProfile(profile, item.SelectedEncoder, scan, job.MediaPath)
			if freezeErr := freezeQueuePreparedProfileSnapshot(h.db, &job, effectiveProfile); freezeErr != nil {
				item.Blockers = append(item.Blockers, "Freeze effective Queue profile identity: "+freezeErr.Error())
				state.Assets = append(state.Assets, item)
				continue
			}
			frozenEffectiveProfile, restoreErr := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
			if restoreErr != nil {
				item.Blockers = append(item.Blockers, "Restore effective Queue profile identity: "+restoreErr.Error())
				state.Assets = append(state.Assets, item)
				continue
			}
			if fingerprint, fingerprintErr := effectiveProfileFingerprint(frozenEffectiveProfile, item.SelectedEncoder); fingerprintErr == nil {
				setEffectiveProfileFingerprint(job.ProfileSnapshot, fingerprint)
			} else {
				item.Warnings = append(item.Warnings, "Frozen Queue profile identity is unavailable: "+fingerprintErr.Error())
			}
			profile = frozenEffectiveProfile
			if info, statErr := os.Stat(job.MediaPath); statErr == nil {
				if evidence, evidenceOK := cachedSourceBitrateEvidenceFromScan(job.MediaPath, info, scan); evidenceOK {
					assessment := bpppfAssessmentForQueueProfile(profile, item.SelectedEncoder, scan, evidence)
					item.QualityEvidence = models.JSONMap{
						"sourceBitrateEvidence": sourceBitrateEvidenceMap(evidence),
						"bpppfAssessment":       assessment,
					}
					manifest, manifestOK := snapshotCacheMap(job.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
					if !manifestOK {
						manifest = models.JSONMap{"components": []string{}}
					}
					manifest["sourceBitrateEvidence"] = item.QualityEvidence["sourceBitrateEvidence"]
					manifest["bpppfAssessment"] = assessment
					job.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey] = manifest
					if assessment.Status == quality.BPPPFStatusWarning || assessment.Status == quality.BPPPFStatusHighRisk {
						item.Warnings = append(item.Warnings, assessment.Explanation)
					}
				}
			}
		}
		if normalizeQueueProcessingMode(job.ProcessingMode) == ProcessingModeFullEncode {
			if err := hevcLevelPlanningError(profile); err != nil {
				item.Blockers = append(item.Blockers, err.Error())
				if strings.EqualFold(strings.TrimSpace(input.CadenceModeOverride), "preserve") && profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false) {
					item.Status = queuePreflightRequiresReview
					item.ReviewCode = "cadence_output_fps_unknown"
					item.CadenceReview = queuePreflightCadenceReview(profile, job.ProfileSnapshot)
					item.Warnings = append(item.Warnings, "Preserve cadence remains unresolved. Choose a resolvable cadence decision and review the effective FPS and HEVC Level/Tier before acceptance.")
					item.ResolvedJob = nil
				}
				state.Assets = append(state.Assets, item)
				continue
			}
		}
		var library models.Library
		if err := h.db.First(&library, job.LibraryID).Error; err != nil {
			item.Blockers = append(item.Blockers, "Destination library is unavailable")
			state.Assets = append(state.Assets, item)
			continue
		}
		outputPath := ""
		if input.preservedDestinationSnapshot != nil {
			job.ResolvedDestinationSnapshot = queuePreflightCloneMap(input.preservedDestinationSnapshot)
			library.Name = workerStringValue(job.ResolvedDestinationSnapshot["libraryName"])
			library.SourcePath = workerStringValue(job.ResolvedDestinationSnapshot["sourcePath"])
			library.DestinationPath = workerStringValue(job.ResolvedDestinationSnapshot["destinationRoot"])
			outputPath = workerStringValue(job.ResolvedDestinationSnapshot["outputPath"])
			if strings.TrimSpace(library.DestinationPath) == "" || strings.TrimSpace(outputPath) == "" {
				item.Blockers = append(item.Blockers, "Preserved destination plan is incomplete")
				state.Assets = append(state.Assets, item)
				continue
			}
		} else {
			if strings.TrimSpace(library.DestinationPath) == "" {
				item.Blockers = append(item.Blockers, "Destination path is not configured")
				state.Assets = append(state.Assets, item)
				continue
			}
			outputPath = plannedOutputPathForJob(h.db, job, library, profile)
			if strings.TrimSpace(outputPath) == "" {
				item.Blockers = append(item.Blockers, "Destination output path could not be resolved")
				state.Assets = append(state.Assets, item)
				continue
			}
			job.ResolvedDestinationSnapshot = models.JSONMap{
				"libraryId": job.LibraryID, "libraryName": library.Name,
				"sourcePath": library.SourcePath, "destinationRoot": library.DestinationPath,
				"outputPath": outputPath, "publishMode": job.PublishMode,
				"profileVersion": job.ProfileVersion,
			}
		}
		if info, statErr := os.Stat(library.DestinationPath); statErr != nil || !info.IsDir() {
			item.Blockers = append(item.Blockers, "Destination path is not an accessible directory")
			state.Assets = append(state.Assets, item)
			continue
		}
		job.PlannedLibraryRoot, job.PlannedPublishedPath = library.DestinationPath, outputPath
		if !queueJobFreezeComplete(job) {
			item.Blockers = append(item.Blockers, "Queue snapshot is incomplete")
			state.Assets = append(state.Assets, item)
			continue
		}
		item.Status, item.ResolvedJob = queuePreflightReady, &job
		item.ReviewCode, item.CadenceReview = "", nil
		if cadenceReview := queuePreflightCadenceReview(profile, job.ProfileSnapshot); cadenceReview != nil && cadenceReview.DeepAnalysisVersion > 0 {
			item.CadenceReview = cadenceReview
		}
		item.RuntimeProfile = runtime.SelectedProfile
		state.Assets = append(state.Assets, item)
	}
	if err := attachPreflightProfileQualifications(h.db, state.Assets); err != nil {
		return queueBatchPreflightState{}, err
	}
	return state, nil
}

func queueJSONMap(value any) (models.JSONMap, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := models.JSONMap{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func queuePreflightCloneMap(value models.JSONMap) models.JSONMap {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	result := models.JSONMap{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil
	}
	return result
}

func queuePreflightOverrideFromSnapshot(snapshot models.JSONMap) *AssetConversionOverrideState {
	raw, exists := snapshot[assetConversionOverrideSnapshotKey]
	if !exists {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var override AssetConversionOverrideState
	if err := json.Unmarshal(encoded, &override); err != nil {
		return nil
	}
	override.assetKeepSubtitleStreamsExplicit = boolValue(snapshot[assetSubtitleSelectionExplicitSnapshotKey], false)
	return &override
}

func queuePreflightEditedResolution(previous models.JSONMap, original, edited QueueJobInput) models.JSONMap {
	resolution := models.JSONMap{}
	if encoded, err := json.Marshal(previous); err == nil {
		_ = json.Unmarshal(encoded, &resolution)
	}
	if resolution == nil {
		resolution = models.JSONMap{}
	}
	decision := func(key string) models.JSONMap {
		result := models.JSONMap{}
		if encoded, err := json.Marshal(resolution[key]); err == nil {
			_ = json.Unmarshal(encoded, &result)
		}
		if result == nil {
			result = models.JSONMap{}
		}
		result["source"] = "queue_batch_preflight_edit"
		return result
	}
	if original.ProfileID != edited.ProfileID {
		video := decision("video")
		video["selection"], video["videoProfileId"] = "profile", edited.ProfileID
		resolution["video"] = video
	}
	if original.AudioProfileKey != edited.AudioProfileKey {
		audio := decision("audio")
		if strings.TrimSpace(edited.AudioProfileKey) == "" {
			audio["selection"], audio["profileKey"] = "disabled", ""
		} else {
			audio["selection"], audio["profileKey"] = "profile", edited.AudioProfileKey
		}
		resolution["audio"] = audio
	}
	if original.TrackProfileKey != edited.TrackProfileKey {
		tracks := decision("tracks")
		if strings.TrimSpace(edited.TrackProfileKey) == "" {
			tracks["selection"], tracks["profileKey"] = "disabled", ""
		} else {
			tracks["selection"], tracks["profileKey"] = "profile", edited.TrackProfileKey
		}
		resolution["tracks"] = tracks
	}
	if original.LibraryID != edited.LibraryID {
		destination := decision("destination")
		destination["selection"], destination["libraryId"] = configSelectionValue, edited.LibraryID
		resolution["destination"] = destination
	}
	return resolution
}

func (h QueueHandler) AcceptBatchPreflight(c *gin.Context) {
	var input QueueBatchPreflightAcceptInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var response QueueBatchPreflightResponse
	var created []models.QueueJob
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var record models.QueueBatchPreflight
		if err := tx.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
			return gorm.ErrRecordNotFound
		}
		if record.Revision != input.Revision || !record.Resolved || record.Status != models.QueueBatchPreflightStatusActive {
			return fmt.Errorf("Queue Preflight revision is stale or unresolved")
		}
		state, err := queuePreflightStateFromRecord(record)
		if err != nil {
			return fmt.Errorf("decode Queue Preflight candidate: %w", err)
		}
		requested := make(map[string]bool, len(input.AssetIDs))
		for _, id := range input.AssetIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				requested[id] = true
			}
		}
		if len(requested) == 0 {
			return fmt.Errorf("select at least one ready asset to queue")
		}
		selectedPaths := make([]string, 0, len(requested))
		for _, asset := range state.Assets {
			if requested[asset.ID] {
				selectedPaths = append(selectedPaths, asset.MediaPath)
			}
		}
		reservedPaths, err := queuePreflightActiveReservations(tx, selectedPaths)
		if err != nil {
			return fmt.Errorf("load acceptance Queue reservations: %w", err)
		}
		validation, err := newQueueBatchValidationContext(tx)
		if err != nil {
			return fmt.Errorf("load acceptance validation context: %w", err)
		}
		selected := make([]*queueBatchPreflightAsset, 0, len(requested))
		for index := range state.Assets {
			asset := &state.Assets[index]
			if !requested[asset.ID] {
				continue
			}
			delete(requested, asset.ID)
			if asset.Status != queuePreflightReady || asset.ResolvedJob == nil || len(asset.Constraints) == 0 || asset.SelectedEncoder == "" {
				return fmt.Errorf("asset %s is blocked or is no longer ready for acceptance", asset.MediaPath)
			}
			if reservedPaths[filepath.Clean(asset.MediaPath)] {
				return fmt.Errorf("asset has an active Queue reservation: %s", asset.MediaPath)
			}
			currentRevision, revisionErr := queuePreflightSourceRevision(asset.MediaPath)
			if revisionErr != nil || currentRevision != asset.SourceRevision {
				return fmt.Errorf("source identity changed after Batch Preflight for %s", asset.MediaPath)
			}
			if asset.AssetRecordID != nil {
				var source models.AssetRecord
				if err := tx.First(&source, *asset.AssetRecordID).Error; err != nil || filepath.Clean(source.Path) != filepath.Clean(asset.MediaPath) {
					return fmt.Errorf("source asset identity changed after Batch Preflight for %s", asset.MediaPath)
				}
			}
			if err := revalidateAcceptedDeepCadenceEvidence(tx, *asset.ResolvedJob); err != nil {
				return err
			}
			if reviewForPath(filepath.Clean(asset.MediaPath), validation.reviews).RequiresReview {
				return fmt.Errorf("asset now requires review: %s", asset.MediaPath)
			}
			if validation.assetHasOpenJob(asset.MediaPath, 0) || validation.activeMaintenance(asset.MediaPath) {
				return fmt.Errorf("asset became reserved before acceptance: %s", asset.MediaPath)
			}
			selected = append(selected, asset)
		}
		if len(requested) != 0 {
			return fmt.Errorf("candidate contains unknown asset identities")
		}
		if len(selected) == 0 {
			return fmt.Errorf("select at least one ready asset to queue")
		}
		sort.SliceStable(selected, func(i, j int) bool { return selected[i].BatchPosition < selected[j].BatchPosition })
		var runtime models.RuntimeSnapshot
		if state.RuntimeSnapshotID == 0 || tx.First(&runtime, state.RuntimeSnapshotID).Error != nil {
			return fmt.Errorf("Batch Preflight runtime snapshot is unavailable; resolve the candidate again")
		}
		if err := h.persistAcceptedPreflightJobs(tx, record.ID, &state, selected, &runtime, &created); err != nil {
			return err
		}
		for _, asset := range selected {
			asset.Status = queuePreflightQueued
			asset.QueuedJobID = asset.ResolvedJob.ID
		}
		state.QueuedJobs = append(state.QueuedJobs, created...)
		stateMap, err := queuePreflightStateMap(state)
		if err != nil {
			return fmt.Errorf("encode accepted Queue Preflight state: %w", err)
		}
		revision := record.Revision + 1
		status := models.QueueBatchPreflightStatusActive
		if !queuePreflightHasPendingAssets(state) {
			status = models.QueueBatchPreflightStatusCompleted
		}
		result := tx.Model(&record).Where("id = ? AND revision = ? AND status = ?", record.ID, record.Revision, models.QueueBatchPreflightStatusActive).Updates(map[string]any{
			"revision": revision, "resolved": true, "status": status, "state": stateMap,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("Queue Preflight revision changed during acceptance")
		}
		record.Revision, record.Status, record.State = revision, status, stateMap
		response = queuePreflightResponse(record, state)
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Queue Preflight candidate was not found"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	response.Jobs = created
	c.JSON(http.StatusCreated, response)
}

func queuePreflightActiveReservations(db *gorm.DB, mediaPaths []string) (map[string]bool, error) {
	conflicts := map[string]bool{}
	if db == nil || len(mediaPaths) == 0 || !db.Migrator().HasTable(&models.SchedulerReservation{}) {
		return conflicts, nil
	}
	keys := make([]string, 0, len(mediaPaths))
	seen := make(map[string]bool, len(mediaPaths))
	for _, mediaPath := range mediaPaths {
		key := filepath.Clean(mediaPath)
		if key == "." || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return conflicts, nil
	}
	var reservations []models.SchedulerReservation
	if err := db.Where("asset_key IN ?", keys).
		Where("state IN ?", []string{scheduler.ReservationStateLocked, scheduler.ReservationStateActive}).
		Find(&reservations).Error; err != nil {
		return nil, err
	}
	for _, reservation := range reservations {
		conflicts[filepath.Clean(reservation.AssetKey)] = true
	}
	return conflicts, nil
}

func (h QueueHandler) persistAcceptedPreflightJobs(tx *gorm.DB, candidateID string, state *queueBatchPreflightState, selected []*queueBatchPreflightAsset, runtime *models.RuntimeSnapshot, created *[]models.QueueJob) error {
	var highest int64
	if err := tx.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
		return err
	}
	for index, asset := range selected {
		job := *asset.ResolvedJob
		job.BatchID, job.BatchName = asset.BatchID, asset.BatchName
		job.BatchPosition = asset.BatchPosition
		job.QueuePosition = highest + int64(index) + 1
		job.Status, job.Stage = JobStatusQueued, JobStageQueued
		job.StageHistory = models.JSONList{}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := scheduler.LockQueuedAsset(tx, job); err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStageQueued); err != nil {
			return err
		}
		plan, err := scheduler.CreatePendingExecutionPlanFromConstraints(tx, &job, "Execution plan frozen from accepted Batch Preflight", asset.Constraints)
		if err != nil {
			return err
		}
		plan.SelectedEncoder = asset.SelectedEncoder
		plan.RuntimeSnapshotID = &runtime.ID
		plan.RuntimeProfile = asset.RuntimeProfile
		plan.DecisionSources["selectedEncoder"] = "batch_preflight_frozen"
		plan.DecisionReasons = append(plan.DecisionReasons, "Selected encoder and runtime provenance frozen during Batch Preflight")
		plan.OutputPath = workerStringValue(job.ResolvedDestinationSnapshot["outputPath"])
		plan.DestinationRoot = workerStringValue(job.ResolvedDestinationSnapshot["destinationRoot"])
		plan.DestinationPath = plan.OutputPath
		plan.LibraryID, plan.PublishMode = job.LibraryID, job.PublishMode
		for _, warning := range asset.Warnings {
			plan.Warnings = append(plan.Warnings, warning)
		}
		if plan.Evaluation == nil {
			plan.Evaluation = models.JSONMap{}
		}
		plan.Evaluation["batchPreflightCandidateId"] = candidateID
		if asset.QualityEvidence != nil {
			plan.Evaluation["qualityEvidence"] = asset.QualityEvidence
			plan.DecisionSources["qualityEvidence"] = "batch_preflight_frozen"
		}
		if err := tx.Save(&plan).Error; err != nil {
			return err
		}
		job.ActiveExecutionPlanID = &plan.ID
		if err := tx.Model(&job).Update("active_execution_plan_id", plan.ID).Error; err != nil {
			return err
		}
		if err := tx.First(&job, job.ID).Error; err != nil {
			return err
		}
		asset.ResolvedJob = &job
		*created = append(*created, job)
	}
	return nil
}

func (h QueueHandler) loadBatchPreflight(c *gin.Context) (models.QueueBatchPreflight, queueBatchPreflightState, bool) {
	var record models.QueueBatchPreflight
	if err := h.db.First(&record, "id = ?", strings.TrimSpace(c.Param("id"))).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Queue Preflight candidate was not found"})
		return record, queueBatchPreflightState{}, false
	}
	state, err := queuePreflightStateFromRecord(record)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decode Queue Preflight candidate: " + err.Error()})
		return record, queueBatchPreflightState{}, false
	}
	return record, state, true
}
