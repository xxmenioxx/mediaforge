package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	labFidelityQueued    = "queued"
	labFidelityRunning   = "running"
	labFidelityCompleted = "completed"
	labFidelityFailed    = "failed"
	labFidelityCanceled  = "canceled"

	labFidelityOperationRetention    = 24 * time.Hour
	labFidelityOperationMaxHistory   = 500
	labFidelityOperationTimeout      = 30 * time.Minute
	labFidelityMaterialVersion       = "lab-fidelity-material-v2"
	labFidelitySamplingPolicyVersion = "lab-fidelity-sampling-v1"
)

// LabFidelityOperation is intentionally bounded. Large probe payloads remain
// behind the process-local evidence provider and are represented by the
// completed window summaries returned in Result.
type LabFidelityOperation struct {
	ID                         string         `json:"id"`
	Status                     string         `json:"status"`
	Phase                      string         `json:"phase"`
	PrimaryRequestID           string         `json:"primaryRequestId"`
	PrimaryArtifactID          string         `json:"primaryArtifactId,omitempty"`
	PrimaryGeneration          string         `json:"primaryGeneration,omitempty"`
	SourceRevision             models.JSONMap `json:"sourceRevision,omitempty"`
	PrimaryStart               float64        `json:"primaryStart,omitempty"`
	PrimaryDuration            float64        `json:"primaryDuration,omitempty"`
	Normalization              models.JSONMap `json:"normalization,omitempty"`
	FrozenPlan                 models.JSONMap `json:"frozenPlan,omitempty"`
	PlanSignature              string         `json:"planSignature,omitempty"`
	MaterialSignature          string         `json:"materialSignature,omitempty"`
	ClientCorrelationSignature string         `json:"clientCorrelationSignature,omitempty"`
	SamplingPolicyVersion      string         `json:"samplingPolicyVersion,omitempty"`
	SamplingPlanIdentity       string         `json:"samplingPlanIdentity,omitempty"`
	SamplingPolicy             models.JSONMap `json:"samplingPolicy,omitempty"`
	samplingPlan               SamplingPlan
	CompletedWindows           int              `json:"completedWindows"`
	TotalWindows               int              `json:"totalWindows"`
	CurrentStart               float64          `json:"currentStart,omitempty"`
	CurrentDuration            float64          `json:"currentDuration,omitempty"`
	Windows                    []models.JSONMap `json:"windows,omitempty"`
	Result                     models.JSONMap   `json:"result,omitempty"`
	Error                      string           `json:"error,omitempty"`
	Retryable                  bool             `json:"retryable,omitempty"`
	CreatedAt                  time.Time        `json:"createdAt"`
	UpdatedAt                  time.Time        `json:"updatedAt"`
}

type labFidelityOperationInput struct {
	PrimaryRequestID   string  `json:"primaryRequestId"`
	ReferenceRequestID string  `json:"referenceRequestId,omitempty"`
	DurationSeconds    float64 `json:"durationSeconds,omitempty"`
	// MaterialSignature is retained as a legacy wire-compatible input. It is
	// never used as the operation's material authority; the backend computes
	// MaterialSignature from the resolved descriptor below.
	MaterialSignature          string `json:"materialSignature,omitempty"`
	ClientCorrelationSignature string `json:"clientCorrelationSignature,omitempty"`
	DraftSignature             string `json:"draftSignature,omitempty"`
}

type labFidelityMaterialIdentity struct {
	Version                  string                          `json:"version"`
	SourcePath               string                          `json:"sourcePath"`
	SourceRevision           compatiblePreviewSourceRevision `json:"sourceRevision"`
	PrimaryStartSeconds      int                             `json:"primaryStartSeconds"`
	PrimaryDurationSeconds   int                             `json:"primaryDurationSeconds"`
	Normalization            previewNormalizationDecision    `json:"normalization"`
	CanonicalEncodeSignature string                          `json:"canonicalEncodeSignature"`
	ArtifactID               string                          `json:"artifactId"`
	GenerationToken          string                          `json:"generationToken"`
	GenerationVersion        int64                           `json:"generationVersion"`
	SamplingPolicyVersion    string                          `json:"samplingPolicyVersion"`
	SamplingPolicy           SamplingPlan                    `json:"samplingPolicy"`
}

func labFidelityMaterialSignature(descriptor *compatiblePreviewDescriptor, plan SamplingPlan) string {
	if descriptor == nil {
		return ""
	}
	identity := labFidelityMaterialIdentity{
		Version:                  labFidelityMaterialVersion,
		SourcePath:               descriptor.Path,
		SourceRevision:           descriptor.SourceRevision,
		PrimaryStartSeconds:      previewTimestampSeconds(descriptor.Start),
		PrimaryDurationSeconds:   max(1, descriptor.Seconds),
		Normalization:            descriptor.Normalization,
		CanonicalEncodeSignature: descriptor.CanonicalEncodeSignature,
		ArtifactID:               descriptor.ArtifactID,
		GenerationToken:          descriptor.GenerationToken,
		GenerationVersion:        descriptor.GenerationVersion,
		SamplingPolicyVersion:    labFidelitySamplingPolicyVersion,
		SamplingPolicy:           plan,
	}
	payload, err := json.Marshal(identity)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%s-%x", labFidelityMaterialVersion, digest[:])
}

func labFidelitySamplingPolicyMap(plan SamplingPlan) models.JSONMap {
	result := models.JSONMap(samplingPlanMap(plan))
	result["version"] = labFidelitySamplingPolicyVersion
	result["planVersion"] = plan.PolicyVersion
	result["planIdentity"] = plan.PlanIdentity
	return result
}

func (h AssetHandler) labSamplingPlan(path string, duration float64, revision compatiblePreviewSourceRevision, primary SamplingWindow) SamplingPlan {
	policy := frameStructureSamplingPolicy(h.db)
	if h.db != nil && h.db.Migrator().HasTable(&models.ScanResult{}) {
		var scan models.ScanResult
		if h.db.Where("path = ?", filepath.Clean(path)).Order("updated_at desc, id desc").First(&scan).Error == nil {
			if scan.Duration <= 0 {
				scan.Duration = duration
			}
			plan := canonicalSamplingPlanFromScan(scan, policy, &primary, labFidelitySamplingPolicyVersion)
			plan.SourceRevision = fmt.Sprintf("%s|%d|%d|%d", filepath.Clean(path), revision.Size, revision.ModifiedAtNS, revision.Mode)
			plan.PlanIdentity = samplingPlanIdentity(plan)
			return plan
		}
	}
	base := canonicalSamplingPlan(duration, policy)
	return resolveCanonicalSamplingPlan(SamplingPlanInput{
		BasePlan:             base,
		SourceFingerprint:    fmt.Sprintf("%s|%d|%d|%d", filepath.Clean(path), revision.Size, revision.ModifiedAtNS, revision.Mode),
		SourceRevision:       fmt.Sprintf("%s|%d|%d|%d", filepath.Clean(path), revision.Size, revision.ModifiedAtNS, revision.Mode),
		SourceStreamIdentity: "0:v:0",
		PolicyVersion:        labFidelitySamplingPolicyVersion,
		Primary:              &primary,
	})
}

type labFidelityOperationStore struct {
	sync.RWMutex
	items   map[string]*LabFidelityOperation
	cancels map[string]context.CancelFunc
}

var labFidelityOperations = labFidelityOperationStore{
	items:   map[string]*LabFidelityOperation{},
	cancels: map[string]context.CancelFunc{},
}

var labFidelitySequence atomic.Uint64
var labFidelityHeavySlot = make(chan struct{}, 1)

type labFidelityWindowPlan struct {
	Position     float64
	Start        float64
	Duration     float64
	Primary      bool
	Identity     string
	Roles        []string
	Reasons      []string
	EvidenceRefs []string
}

type labPreviewArtifactEvidence struct {
	ArtifactID        string
	GenerationToken   string
	GenerationVersion int64
	Source            previewVideoCharacteristics
	Output            previewVideoCharacteristics
	SourceFrames      QSVFrameStructureAnalysis
	OutputFrames      QSVFrameStructureAnalysis
	HEVC              models.JSONMap
	HEVCStructure     HEVCBitstreamStructure
	HEVCError         error
}

type labPreviewEvidenceCacheEntry struct {
	Evidence  labPreviewArtifactEvidence
	Metrics   *previewFrameMetrics
	UpdatedAt time.Time
}

var labPreviewEvidence = struct {
	sync.Mutex
	items map[string]labPreviewEvidenceCacheEntry
}{items: map[string]labPreviewEvidenceCacheEntry{}}

var labSourceProbeEvidence = struct {
	sync.Mutex
	items map[string]struct {
		value     previewVideoCharacteristics
		updatedAt time.Time
	}
}{items: map[string]struct {
	value     previewVideoCharacteristics
	updatedAt time.Time
}{}}

const (
	labPreviewEvidenceRetention = 24 * time.Hour
	labPreviewEvidenceMaxItems  = 500
)

func labPreviewEvidenceCacheKey(descriptor *compatiblePreviewDescriptor) string {
	return fmt.Sprintf("%s:%s:%d:%s:%s:%d:%d:%d:%d:%s", descriptor.ArtifactID, descriptor.GenerationToken, descriptor.GenerationVersion, descriptor.Path, descriptor.Start, descriptor.Seconds, descriptor.SourceRevision.Size, descriptor.SourceRevision.ModifiedAtNS, descriptor.SourceRevision.Mode, descriptor.Normalization.Mode)
}

func labPreviewEvidenceCacheGet(key string) (labPreviewEvidenceCacheEntry, bool) {
	now := time.Now()
	labPreviewEvidence.Lock()
	defer labPreviewEvidence.Unlock()
	for candidate, entry := range labPreviewEvidence.items {
		if now.Sub(entry.UpdatedAt) > labPreviewEvidenceRetention {
			delete(labPreviewEvidence.items, candidate)
		}
	}
	entry, ok := labPreviewEvidence.items[key]
	return entry, ok
}

func labPreviewEvidenceCachePut(key string, entry labPreviewEvidenceCacheEntry) {
	labPreviewEvidence.Lock()
	defer labPreviewEvidence.Unlock()
	if len(labPreviewEvidence.items) >= labPreviewEvidenceMaxItems {
		var oldestKey string
		var oldestAt time.Time
		for candidate, existing := range labPreviewEvidence.items {
			if oldestKey == "" || existing.UpdatedAt.Before(oldestAt) {
				oldestKey, oldestAt = candidate, existing.UpdatedAt
			}
		}
		if oldestKey != "" {
			delete(labPreviewEvidence.items, oldestKey)
		}
	}
	labPreviewEvidence.items[key] = entry
}

func labPreviewEvidenceCachePutMetrics(key string, metrics previewFrameMetrics) {
	labPreviewEvidence.Lock()
	defer labPreviewEvidence.Unlock()
	if cached, ok := labPreviewEvidence.items[key]; ok {
		metricsCopy := metrics
		cached.Metrics = &metricsCopy
		cached.UpdatedAt = time.Now()
		labPreviewEvidence.items[key] = cached
	}
}

func labSourceProbeKey(descriptor *compatiblePreviewDescriptor) string {
	// This provider contains stream-global characteristics only. It is shared
	// by intervals and normalization modes, but never across source revisions.
	return fmt.Sprintf("%s:%d:%d:%d", descriptor.Path, descriptor.SourceRevision.Size, descriptor.SourceRevision.ModifiedAtNS, descriptor.SourceRevision.Mode)
}

func labSourceProbeGet(key string) (previewVideoCharacteristics, bool) {
	now := time.Now()
	labSourceProbeEvidence.Lock()
	defer labSourceProbeEvidence.Unlock()
	for candidate, entry := range labSourceProbeEvidence.items {
		if now.Sub(entry.updatedAt) > labPreviewEvidenceRetention {
			delete(labSourceProbeEvidence.items, candidate)
		}
	}
	entry, ok := labSourceProbeEvidence.items[key]
	return entry.value, ok
}

func labSourceProbePut(key string, value previewVideoCharacteristics) {
	labSourceProbeEvidence.Lock()
	defer labSourceProbeEvidence.Unlock()
	if len(labSourceProbeEvidence.items) >= labPreviewEvidenceMaxItems {
		var oldestKey string
		var oldestAt time.Time
		for candidate, entry := range labSourceProbeEvidence.items {
			if oldestKey == "" || entry.updatedAt.Before(oldestAt) {
				oldestKey, oldestAt = candidate, entry.updatedAt
			}
		}
		if oldestKey != "" {
			delete(labSourceProbeEvidence.items, oldestKey)
		}
	}
	labSourceProbeEvidence.items[key] = struct {
		value     previewVideoCharacteristics
		updatedAt time.Time
	}{value: value, updatedAt: time.Now()}
}

func labFidelityTerminal(status string) bool {
	return status == labFidelityCompleted || status == labFidelityFailed || status == labFidelityCanceled
}

func (store *labFidelityOperationStore) copy(id string) (LabFidelityOperation, bool) {
	store.RLock()
	defer store.RUnlock()
	operation := store.items[id]
	if operation == nil {
		return LabFidelityOperation{}, false
	}
	copy := *operation
	copy.Windows = append([]models.JSONMap(nil), operation.Windows...)
	return copy, true
}

func (store *labFidelityOperationStore) update(id string, update func(*LabFidelityOperation)) bool {
	store.Lock()
	defer store.Unlock()
	operation := store.items[id]
	if operation == nil {
		return false
	}
	if labFidelityTerminal(operation.Status) {
		return true
	}
	update(operation)
	operation.UpdatedAt = time.Now()
	return true
}

func (store *labFidelityOperationStore) cancel(id string) (LabFidelityOperation, bool) {
	store.Lock()
	defer store.Unlock()
	operation := store.items[id]
	if operation == nil {
		return LabFidelityOperation{}, false
	}
	if labFidelityTerminal(operation.Status) {
		return *operation, true
	}
	if cancel := store.cancels[id]; cancel != nil {
		cancel()
	}
	operation.Status = labFidelityCanceled
	operation.Phase = labFidelityCanceled
	operation.Error = ""
	operation.Retryable = true
	operation.UpdatedAt = time.Now()
	return *operation, true
}

func (store *labFidelityOperationStore) cleanup(now time.Time) {
	store.Lock()
	defer store.Unlock()
	type terminal struct {
		id      string
		updated time.Time
	}
	items := make([]terminal, 0, len(store.items))
	for id, operation := range store.items {
		if operation == nil || !labFidelityTerminal(operation.Status) {
			continue
		}
		if now.Sub(operation.UpdatedAt) > labFidelityOperationRetention {
			delete(store.items, id)
			delete(store.cancels, id)
			continue
		}
		items = append(items, terminal{id: id, updated: operation.UpdatedAt})
	}
	if len(items) <= labFidelityOperationMaxHistory {
		return
	}
	for len(items) > labFidelityOperationMaxHistory {
		oldest := 0
		for index := range items {
			if items[index].updated.Before(items[oldest].updated) {
				oldest = index
			}
		}
		delete(store.items, items[oldest].id)
		delete(store.cancels, items[oldest].id)
		items = append(items[:oldest], items[oldest+1:]...)
	}
}

func (h AssetHandler) StartLabFidelityOperation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var input labFidelityOperationInput
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.PrimaryRequestID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "primaryRequestId is required"})
		return
	}
	input.PrimaryRequestID = strings.TrimSpace(input.PrimaryRequestID)
	entry, tombstone, known := compatiblePreviewRequestLookup(input.PrimaryRequestID, time.Now())
	if !known || tombstone != nil || entry == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "primary Compatible Preview request is unknown", "code": "unknown_request"})
		return
	}
	descriptor, err := h.compatiblePreviewDescriptorForEntry(entry, input.PrimaryRequestID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "primary_not_ready", "state": "not_ready", "retryable": true})
		return
	}
	if !compatiblePreviewPublishedArtifactValidForDescriptor(descriptor) {
		c.JSON(http.StatusConflict, gin.H{"error": "primary Compatible Preview artifact is not ready", "code": "primary_not_ready", "state": "not_ready", "retryable": true})
		return
	}
	duration := input.DurationSeconds
	if duration <= 0 {
		duration = float64(previewTimestampSeconds(descriptor.Start) + max(1, descriptor.Seconds))
	}
	primaryWindow := SamplingWindow{
		Position:        primaryPosition(float64(previewTimestampSeconds(descriptor.Start)), float64(max(1, descriptor.Seconds)), duration),
		StartSeconds:    float64(previewTimestampSeconds(descriptor.Start)),
		DurationSeconds: float64(max(1, descriptor.Seconds)),
		Primary:         true,
	}
	plan := h.labSamplingPlan(descriptor.Path, duration, descriptor.SourceRevision, primaryWindow)
	clientCorrelation := strings.TrimSpace(input.ClientCorrelationSignature)
	if clientCorrelation == "" {
		// Accept the pre-hardening field for wire compatibility, but never use
		// it as the operation's material authority.
		clientCorrelation = strings.TrimSpace(input.MaterialSignature)
	}
	materialSignature := labFidelityMaterialSignature(descriptor, plan)
	if materialSignature == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "could not resolve backend Fidelity material identity", "code": "stale_primary", "retryable": true})
		return
	}

	now := time.Now()
	store := &labFidelityOperations
	store.cleanup(now)
	id := fmt.Sprintf("lab-fidelity-%d-%d", now.UnixNano(), labFidelitySequence.Add(1))
	ctx, cancel := context.WithTimeout(context.Background(), labFidelityOperationTimeout)
	operation := &LabFidelityOperation{
		ID: id, Status: labFidelityQueued, Phase: "waiting_for_capacity",
		PrimaryRequestID: input.PrimaryRequestID, PrimaryArtifactID: descriptor.ArtifactID,
		PrimaryGeneration: fmt.Sprintf("%s:%d", descriptor.GenerationToken, descriptor.GenerationVersion),
		SourceRevision:    labSourceRevisionMap(descriptor.SourceRevision),
		PrimaryStart:      float64(previewTimestampSeconds(descriptor.Start)), PrimaryDuration: float64(max(1, descriptor.Seconds)),
		Normalization: labNormalizationMap(descriptor.Normalization), FrozenPlan: descriptor.ResolvedVideoDecision,
		PlanSignature: descriptor.CanonicalEncodeSignature, MaterialSignature: materialSignature,
		ClientCorrelationSignature: clientCorrelation, SamplingPolicyVersion: labFidelitySamplingPolicyVersion,
		SamplingPlanIdentity: plan.PlanIdentity,
		SamplingPolicy:       labFidelitySamplingPolicyMap(plan), samplingPlan: plan,
		TotalWindows: len(labFidelityWindowPlans(float64(previewTimestampSeconds(descriptor.Start)), float64(max(1, descriptor.Seconds)), duration, plan)),
		CreatedAt:    now, UpdatedAt: now,
	}
	store.Lock()
	store.items[id] = operation
	store.cancels[id] = cancel
	store.Unlock()
	response := *operation
	go h.executeLabFidelityOperation(ctx, cancel, input, id)
	c.JSON(http.StatusAccepted, response)
}

func (h AssetHandler) GetLabFidelityOperation(c *gin.Context) {
	operation, ok := labFidelityOperations.copy(strings.TrimSpace(c.Param("id")))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "LAB Fidelity operation is unknown", "code": "unknown_operation"})
		return
	}
	c.JSON(http.StatusOK, operation)
}

func (h AssetHandler) CancelLabFidelityOperation(c *gin.Context) {
	operation, ok := labFidelityOperations.cancel(strings.TrimSpace(c.Param("id")))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "LAB Fidelity operation is unknown", "code": "unknown_operation"})
		return
	}
	c.JSON(http.StatusOK, operation)
}

func labFidelityDescriptorMatchesOperation(operation LabFidelityOperation, descriptor *compatiblePreviewDescriptor) bool {
	if descriptor == nil || operation.PrimaryArtifactID != descriptor.ArtifactID ||
		operation.PrimaryGeneration != fmt.Sprintf("%s:%d", descriptor.GenerationToken, descriptor.GenerationVersion) ||
		operation.PlanSignature != descriptor.CanonicalEncodeSignature ||
		operation.PrimaryStart != float64(previewTimestampSeconds(descriptor.Start)) ||
		operation.PrimaryDuration != float64(max(1, descriptor.Seconds)) ||
		!reflect.DeepEqual(operation.SourceRevision, labSourceRevisionMap(descriptor.SourceRevision)) ||
		!reflect.DeepEqual(operation.Normalization, labNormalizationMap(descriptor.Normalization)) {
		return false
	}
	return operation.MaterialSignature == labFidelityMaterialSignature(descriptor, operation.samplingPlan)
}

func (h AssetHandler) executeLabFidelityOperation(ctx context.Context, cancel context.CancelFunc, input labFidelityOperationInput, id string) {
	defer func() {
		if ctx.Err() != nil {
			labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
				if !labFidelityTerminal(operation.Status) {
					operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, "interrupted", "LAB Fidelity operation was interrupted", true
				}
			})
		}
		cancel()
		labFidelityOperations.Lock()
		delete(labFidelityOperations.cancels, id)
		labFidelityOperations.Unlock()
		labFidelityOperations.cleanup(time.Now())
	}()

	select {
	case labFidelityHeavySlot <- struct{}{}:
		defer func() { <-labFidelityHeavySlot }()
	case <-ctx.Done():
		return
	}
	labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
		if operation.Status == labFidelityQueued {
			operation.Status = labFidelityRunning
			operation.Phase = "primary_evidence"
		}
	})

	entry, _, known := compatiblePreviewRequestLookup(input.PrimaryRequestID, time.Now())
	if !known || entry == nil {
		labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
			operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, labFidelityFailed, "primary Compatible Preview request disappeared", true
		})
		return
	}
	descriptor, err := h.compatiblePreviewDescriptorForEntry(entry, input.PrimaryRequestID)
	if err != nil {
		labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
			operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, labFidelityFailed, err.Error(), true
		})
		return
	}
	operation, ok := labFidelityOperations.copy(id)
	if !ok || !labFidelityDescriptorMatchesOperation(operation, descriptor) {
		labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
			operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, "stale_primary", "primary Compatible Preview generation changed before LAB Fidelity evidence began", true
		})
		return
	}
	primaryStart := float64(previewTimestampSeconds(descriptor.Start))
	primaryDuration := float64(max(1, descriptor.Seconds))
	duration := operation.samplingPlan.AssetDurationSeconds
	plan := operation.samplingPlan
	if duration <= 0 || len(plan.Positions) == 0 {
		duration = primaryStart + primaryDuration
		plan = canonicalSamplingPlan(duration, frameStructureSamplingPolicy(h.db))
	}
	windows := labFidelityWindowPlans(primaryStart, primaryDuration, duration, plan)
	labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
		operation.TotalWindows = len(windows)
		operation.SamplingPolicy = labFidelitySamplingPolicyMap(plan)
		operation.SamplingPolicyVersion = labFidelitySamplingPolicyVersion
		operation.SamplingPlanIdentity = plan.PlanIdentity
	})

	completed := make([]models.JSONMap, 0, len(windows))
	for index, window := range windows {
		if ctx.Err() != nil {
			return
		}
		labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
			operation.Phase = map[bool]string{true: "primary_evidence", false: "additional_window_encode"}[window.Primary]
			operation.CurrentStart = window.Start
			operation.CurrentDuration = window.Duration
		})

		windowDescriptor := descriptor
		if window.Primary {
			currentEntry, _, currentKnown := compatiblePreviewRequestLookup(input.PrimaryRequestID, time.Now())
			currentDescriptor, currentErr := h.compatiblePreviewDescriptorForEntry(currentEntry, input.PrimaryRequestID)
			if !currentKnown || currentErr != nil || !labFidelityDescriptorMatchesOperation(operation, currentDescriptor) {
				labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
					operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, "stale_primary", "primary Compatible Preview generation changed before LAB Fidelity evidence began", true
				})
				return
			}
			windowDescriptor = currentDescriptor
		}
		if !window.Primary {
			request := labFidelityAdditionalWindowRequest(descriptor, window)
			requestID, additionalDescriptor, requestErr := h.ensureLabCompatiblePreviewRequest(request)
			if requestErr != nil {
				labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
					operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, labFidelityFailed, requestErr.Error(), true
				})
				return
			}
			_ = requestID
			windowDescriptor = additionalDescriptor
		}
		if !window.Primary {
			labFidelityOperations.update(id, func(operation *LabFidelityOperation) { operation.Phase = "additional_window_inspect" })
		}
		result, windowErr := h.collectLabFidelityWindow(ctx, windowDescriptor, window)
		if windowErr != nil {
			if ctx.Err() != nil {
				return
			}
			labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
				operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, labFidelityFailed, windowErr.Error(), true
			})
			return
		}
		completed = append(completed, result)
		labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
			operation.CompletedWindows = index + 1
			operation.Windows = append([]models.JSONMap(nil), completed...)
			operation.Result = labFidelityOperationResultWithIdentity(input, operation, descriptor, completed, plan, len(windows), false)
		})
	}

	labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
		operation.Status = labFidelityCompleted
		operation.Phase = "aggregate"
		operation.Result = labFidelityOperationResultWithIdentity(input, operation, descriptor, completed, plan, len(windows), true)
		operation.CurrentStart = 0
		operation.CurrentDuration = 0
		operation.Error = ""
		operation.Retryable = false
	})
}

func (h AssetHandler) collectLabFidelityWindow(ctx context.Context, descriptor *compatiblePreviewDescriptor, window labFidelityWindowPlan) (models.JSONMap, error) {
	_, lease, err := acquireCompatiblePreviewArtifact(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	if lease != nil {
		defer lease.release()
	}

	evidence, err := labCollectPreviewEvidence(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	metrics, err := labGeneratePreviewFrameMetrics(ctx, descriptor, evidence)
	if err != nil {
		return nil, err
	}
	canonical := labEvaluateCanonicalFidelity(descriptor, evidence)
	return models.JSONMap{
		"position": window.Position, "startSeconds": window.Start, "durationSeconds": window.Duration,
		"windowIdentity": window.Identity, "primary": window.Primary,
		"roles": window.Roles, "reasons": window.Reasons, "evidenceRefs": window.EvidenceRefs,
		"source": evidence.Source, "output": evidence.Output, "metrics": metrics,
		"sourceFrameStructure": evidence.SourceFrames, "outputFrameStructure": evidence.OutputFrames,
		"hevcBitstreamEvidence": evidence.HEVC,
		"requestId":             descriptor.requestID,
		"artifactId":            descriptor.ArtifactID, "generationToken": descriptor.GenerationToken,
		"generationVersion": descriptor.GenerationVersion, "status": "completed",
		"canonicalFidelity": canonical,
	}, nil
}

func labFidelityAdditionalWindowRequest(descriptor *compatiblePreviewDescriptor, window labFidelityWindowPlan) compatiblePreviewRequest {
	request := descriptor.Request
	// The primary descriptor is the frozen execution authority for the whole
	// operation. Do not let a later ProfileID lookup re-resolve mutable profile
	// state for an additional window.
	request.ProfileID = 0
	request.Profile = cloneCompatiblePreviewProfile(descriptor.EffectivePreviewProfile)
	if request.Profile == nil {
		request.Profile = cloneCompatiblePreviewProfile(descriptor.PreviewProfile)
	}
	request.Start = strconv.Itoa(int(math.Floor(window.Start)))
	request.Seconds = max(1, int(math.Round(window.Duration)))
	request.Ephemeral = false
	return request
}

func labFidelityWindowPlans(primaryStart, primaryDuration, duration float64, plan SamplingPlan) []labFidelityWindowPlan {
	primaryEnd := primaryStart + primaryDuration
	primaryPlan := labFidelityWindowPlan{Position: primaryPosition(primaryStart, primaryDuration, duration), Start: primaryStart, Duration: primaryDuration, Primary: true}
	result := []labFidelityWindowPlan{primaryPlan}
	if len(plan.SelectedWindows) > 0 {
		for _, selected := range plan.SelectedWindows {
			if !selected.Primary && samplingWindowsEquivalent(SamplingWindow{StartSeconds: primaryStart, DurationSeconds: primaryDuration}, selected, 1.0/30.0) {
				continue
			}
			if selected.Primary {
				result[0].Identity = selected.Identity
				result[0].Roles = append([]string(nil), selected.Roles...)
				result[0].Reasons = append([]string(nil), selected.Reasons...)
				result[0].EvidenceRefs = append([]string(nil), selected.EvidenceRefs...)
				continue
			}
			windowDuration := float64(boundedPreviewSeconds(strconv.Itoa(int(math.Round(selected.DurationSeconds)))))
			start := math.Floor(selected.StartSeconds)
			end := start + windowDuration
			if math.Abs(start-primaryStart) <= 1.0/30.0 && math.Abs(end-primaryEnd) <= 1.0/30.0 {
				continue
			}
			duplicate := false
			for _, existing := range result {
				if math.Abs(existing.Start-start) <= 1.0/30.0 && math.Abs(existing.Start+existing.Duration-end) <= 1.0/30.0 {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			result = append(result, labFidelityWindowPlan{
				Position: selected.Position, Start: start, Duration: windowDuration,
				Identity: selected.Identity, Roles: append([]string(nil), selected.Roles...),
				Reasons: append([]string(nil), selected.Reasons...), EvidenceRefs: append([]string(nil), selected.EvidenceRefs...),
			})
			if plan.Limits.MaximumWindows > 0 && len(result) >= plan.Limits.MaximumWindows {
				break
			}
		}
		return result
	}
	tolerance := 1.0 / 30.0
	for _, position := range plan.Positions {
		windowDuration := math.Min(plan.WindowSeconds, duration)
		start := math.Max(0, math.Min(math.Max(0, duration-windowDuration), duration*position-windowDuration/2))
		// Apply the same integer start/5..120 second request normalization used
		// by Compatible Preview before comparing media-time intervals.
		start = math.Floor(start)
		windowDuration = float64(boundedPreviewSeconds(strconv.Itoa(int(math.Round(windowDuration)))))
		end := start + windowDuration
		if math.Abs(start-primaryStart) <= tolerance && math.Abs(end-primaryEnd) <= tolerance {
			continue
		}
		duplicate := false
		for _, existing := range result {
			if math.Abs(existing.Start-start) <= tolerance && math.Abs(existing.Start+existing.Duration-end) <= tolerance {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, labFidelityWindowPlan{Position: position, Start: start, Duration: windowDuration})
			if plan.Limits.MaximumWindows > 0 && len(result) >= plan.Limits.MaximumWindows {
				break
			}
		}
	}
	return result
}

func primaryPosition(start, duration, assetDuration float64) float64 {
	if assetDuration <= 0 {
		return 0
	}
	return math.Max(0, math.Min(1, (start+duration/2)/assetDuration))
}

func labSourceRevisionMap(revision compatiblePreviewSourceRevision) models.JSONMap {
	return models.JSONMap{"size": revision.Size, "modifiedAtNS": revision.ModifiedAtNS, "mode": revision.Mode}
}

func labNormalizationMap(normalization previewNormalizationDecision) models.JSONMap {
	return models.JSONMap{
		"mode": normalization.Mode, "applied": normalization.Applied, "filter": normalization.Filter,
		"reason": normalization.Reason, "sarPreserved": normalization.SARPreserved,
		"aliasWarnings": normalization.AliasWarnings,
	}
}

func labPreviewProbe(value previewVideoCharacteristics) map[string]any {
	return map[string]any{"streams": []any{map[string]any{
		"codec_name": value.Codec, "profile": value.Profile, "level": value.Level, "pix_fmt": value.PixelFormat,
		"bits_per_raw_sample": strconv.Itoa(value.BitDepth), "width": value.Width, "height": value.Height,
		"sample_aspect_ratio": value.SampleAspectRatio, "display_aspect_ratio": value.DisplayAspectRatio,
		"avg_frame_rate": value.FrameRate, "field_order": value.FieldOrder, "color_range": value.ColorRange,
		"color_space": value.ColorSpace, "color_transfer": value.ColorTransfer, "color_primaries": value.ColorPrimaries,
		"chroma_location": value.ChromaLocation,
	}}}
}

func labEvaluateCanonicalFidelity(descriptor *compatiblePreviewDescriptor, evidence labPreviewArtifactEvidence) models.JSONMap {
	profile := models.Profile{}
	if descriptor.EffectivePreviewProfile != nil {
		profile = *descriptor.EffectivePreviewProfile
	} else if descriptor.PreviewProfile != nil {
		profile = *descriptor.PreviewProfile
	}
	return evaluateCanonicalFidelity(
		fidelityAuthority{profile: profile, worker: map[string]interface{}(profile.WorkerConfig), name: "compatible_preview_frozen_plan"},
		labPreviewProbe(evidence.Source), labPreviewProbe(evidence.Output), evidence.HEVC,
	)
}

func labFidelityOperationResult(input labFidelityOperationInput, descriptor *compatiblePreviewDescriptor, windows []models.JSONMap, plan SamplingPlan, totalWindows int, complete bool) models.JSONMap {
	if len(windows) == 0 {
		return nil
	}
	first := windows[0]
	status := "analysis_in_progress"
	canonicalStatus := ""
	canonicalSummary := unknownRecord(first["canonicalFidelity"])
	for _, window := range windows {
		canonical := unknownRecord(window["canonicalFidelity"])
		switch stringFromUnknown(canonical["status"]) {
		case "mismatch":
			canonicalStatus = "mismatch"
			canonicalSummary = canonical
		case "unverified":
			if canonicalStatus != "mismatch" {
				canonicalStatus = "unverified"
				canonicalSummary = canonical
			}
		case "passed":
			if canonicalStatus == "" {
				canonicalStatus = "passed"
				canonicalSummary = canonical
			}
		}
	}
	if canonicalStatus == "" {
		canonicalStatus = "unverified"
	}
	if complete {
		status = "completed"
	}
	visualAggregate := labFidelityVisualAggregate(windows, totalWindows)
	result := models.JSONMap{
		"assetPath": descriptor.Path, "draftSignature": input.DraftSignature,
		"source": first["source"], "reference": first["source"], "conversion": first["output"],
		"sourceFrameStructure": first["sourceFrameStructure"], "outputFrameStructure": first["outputFrameStructure"],
		"qsvFrameWarnings": []string{}, "qsvFeatureStatus": QSVFeatureStatus{},
		"frameRecommendation": models.JSONMap{"targetGopFrames": 0, "targetGopSeconds": 0, "maxBFrames": 0, "confidence": "unverified", "reasons": []string{"Fidelity operation result is backend-owned."}},
		"frameValidation":     models.JSONMap{"verdict": "review", "confidence": "unverified", "reasons": []string{"Canonical Fidelity evidence is authoritative."}},
		"hevcLevelValidation": models.JSONMap{"verdict": "review", "status": "unverified", "reason": "Canonical HEVC evidence is reported per window."},
		"requestedEncoder":    descriptor.RequestedVideoEncoder, "effectiveEncoder": descriptor.EffectiveVideoEncoder,
		"requestedQSVRateControl": descriptor.RequestedQSVRateControl, "effectiveQSVRateControl": descriptor.EffectiveQSVRateControl,
		"effectiveVideoDecision": descriptor.ResolvedVideoDecision, "frameStructureRecommendation": nil,
		"referenceEncoder": "source", "ffmpegArgs": descriptor.Args, "normalization": descriptor.Normalization,
		"metrics": first["metrics"], "referenceRequestId": input.ReferenceRequestID, "conversionRequestId": descriptor.requestID,
		"canonicalFidelity": canonicalSummary, "hevcBitstreamEvidence": first["hevcBitstreamEvidence"],
		"fidelityWindows": windows, "canonicalStatus": canonicalStatus, "aggregateStatus": status,
		"visualAggregate":      visualAggregate,
		"samplingPolicy":       labFidelitySamplingPolicyMap(plan),
		"samplingPlanIdentity": plan.PlanIdentity,
		"completedAt":          map[bool]int64{true: time.Now().UnixMilli(), false: 0}[complete],
	}
	if complete {
		result["overallVerdict"] = labFidelityOverallVerdict(canonicalStatus, visualAggregate)
	}
	return result
}

func labFidelityOperationResultWithIdentity(input labFidelityOperationInput, operation *LabFidelityOperation, descriptor *compatiblePreviewDescriptor, windows []models.JSONMap, plan SamplingPlan, totalWindows int, complete bool) models.JSONMap {
	result := labFidelityOperationResult(input, descriptor, windows, plan, totalWindows, complete)
	if result == nil || operation == nil {
		return result
	}
	result["materialSignature"] = operation.MaterialSignature
	result["primaryRequestId"] = operation.PrimaryRequestID
	result["primaryArtifactId"] = operation.PrimaryArtifactID
	result["primaryGeneration"] = operation.PrimaryGeneration
	result["samplingPolicyVersion"] = operation.SamplingPolicyVersion
	result["samplingPlanIdentity"] = operation.SamplingPlanIdentity
	return result
}

func labFidelityVisualAggregate(windows []models.JSONMap, totalWindows int) models.JSONMap {
	if totalWindows <= 0 {
		totalWindows = len(windows)
	}
	comparableCount := 0
	ssimValues := make([]float64, 0, len(windows))
	psnrValues := make([]float64, 0, len(windows))
	worstIndex := -1
	worstSSIM := math.Inf(1)
	var worstPosition float64
	for index, window := range windows {
		metrics, ok := labFidelityMetrics(window["metrics"])
		if !ok || !metrics.Comparable || !isFiniteMetric(metrics.SSIM) {
			continue
		}
		comparableCount++
		ssimValues = append(ssimValues, metrics.SSIM)
		if isFiniteMetric(metrics.PSNR) {
			psnrValues = append(psnrValues, metrics.PSNR)
		}
		if metrics.SSIM < worstSSIM {
			worstSSIM = metrics.SSIM
			worstIndex = index
			worstPosition = numberFromUnknown(window["position"])
		}
	}
	aggregate := models.JSONMap{
		"comparableCount": comparableCount,
		"totalCount":      totalWindows,
		"metricVerdict":   labFidelityMetricVerdict(worstSSIM, comparableCount > 0),
	}
	if len(ssimValues) > 0 {
		aggregate["averageSSIM"] = averageMetric(ssimValues)
		aggregate["minimumSSIM"] = minMetric(ssimValues)
	}
	if len(psnrValues) > 0 {
		aggregate["averagePSNR"] = averageMetric(psnrValues)
		aggregate["minimumPSNR"] = minMetric(psnrValues)
	}
	if worstIndex >= 0 {
		aggregate["worstWindowIndex"] = worstIndex
		aggregate["worstPosition"] = worstPosition
	}
	return aggregate
}

func labFidelityMetrics(value interface{}) (previewFrameMetrics, bool) {
	switch typed := value.(type) {
	case previewFrameMetrics:
		return typed, true
	case *previewFrameMetrics:
		if typed != nil {
			return *typed, true
		}
	case models.JSONMap:
		return previewFrameMetrics{Comparable: labBoolFromUnknown(typed["comparable"]), SSIM: numberFromUnknown(typed["ssim"]), PSNR: numberFromUnknown(typed["psnr"])}, true
	case map[string]interface{}:
		return previewFrameMetrics{Comparable: labBoolFromUnknown(typed["comparable"]), SSIM: numberFromUnknown(typed["ssim"]), PSNR: numberFromUnknown(typed["psnr"])}, true
	}
	return previewFrameMetrics{}, false
}

func labBoolFromUnknown(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		return err == nil && parsed
	default:
		return false
	}
}

func numberFromUnknown(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return parsed
		}
	}
	return math.NaN()
}

func isFiniteMetric(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func averageMetric(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func minMetric(values []float64) float64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func labFidelityMetricVerdict(minimumSSIM float64, hasComparable bool) string {
	if !hasComparable || !isFiniteMetric(minimumSSIM) {
		return "Review"
	}
	if minimumSSIM >= 0.95 {
		return "Safe"
	}
	if minimumSSIM >= 0.90 {
		return "Review"
	}
	return "Reject"
}

func labFidelityOverallVerdict(canonicalStatus string, visualAggregate models.JSONMap) string {
	if canonicalStatus == "mismatch" {
		return "Reject"
	}
	if canonicalStatus != "passed" {
		return "Review"
	}
	metricVerdict := stringFromUnknown(visualAggregate["metricVerdict"])
	comparableCount := int(numberFromUnknown(visualAggregate["comparableCount"]))
	totalCount := int(numberFromUnknown(visualAggregate["totalCount"]))
	if comparableCount < totalCount {
		if metricVerdict == "Reject" {
			return "Reject"
		}
		return "Review"
	}
	if metricVerdict == "Safe" || metricVerdict == "Reject" {
		return metricVerdict
	}
	return "Review"
}

func labCollectPreviewEvidence(ctx context.Context, descriptor *compatiblePreviewDescriptor) (labPreviewArtifactEvidence, error) {
	key := labPreviewEvidenceCacheKey(descriptor)
	if cached, ok := labPreviewEvidenceCacheGet(key); ok {
		return cached.Evidence, nil
	}

	sourceKey := labSourceProbeKey(descriptor)
	source, sourceCached := labSourceProbeGet(sourceKey)
	var err error
	if !sourceCached {
		source, err = probePreviewCharacteristics(descriptor.Path)
		if err != nil {
			return labPreviewArtifactEvidence{}, err
		}
		labSourceProbePut(sourceKey, source)
	}
	output, err := probePreviewCharacteristics(descriptor.CachePath)
	if err != nil {
		return labPreviewArtifactEvidence{}, err
	}
	sourceFrames, err := analyzeVideoFrameStructureInterval(ctx, descriptor.Path, fmt.Sprintf("%d%%+%d", previewTimestampSeconds(descriptor.Start), descriptor.Seconds))
	if err != nil {
		return labPreviewArtifactEvidence{}, err
	}
	outputFrames, err := analyzeVideoFrameStructureInterval(ctx, descriptor.CachePath, fmt.Sprintf("%%+%d", descriptor.Seconds))
	if err != nil {
		return labPreviewArtifactEvidence{}, err
	}
	hevc := models.JSONMap{}
	var hevcStructure HEVCBitstreamStructure
	var hevcErr error
	if strings.EqualFold(output.Codec, "hevc") {
		hevcStructure, hevcErr = measureHEVCBitstreamStructure(ctx, descriptor.CachePath)
		hevc = hevcBitstreamEvidenceFromStructure(hevcStructure, hevcErr)
		hevc["stream"], hevc["probe"] = "0:v:0", "ffmpeg_trace_headers"
	}
	evidence := labPreviewArtifactEvidence{ArtifactID: descriptor.ArtifactID, GenerationToken: descriptor.GenerationToken, GenerationVersion: descriptor.GenerationVersion, Source: source, Output: output, SourceFrames: sourceFrames, OutputFrames: outputFrames, HEVC: hevc, HEVCStructure: hevcStructure, HEVCError: hevcErr}
	if cached, ok := labPreviewEvidenceCacheGet(key); ok {
		return cached.Evidence, nil
	}
	labPreviewEvidenceCachePut(key, labPreviewEvidenceCacheEntry{Evidence: evidence, UpdatedAt: time.Now()})
	return evidence, nil
}

func labGeneratePreviewFrameMetrics(ctx context.Context, descriptor *compatiblePreviewDescriptor, evidence labPreviewArtifactEvidence) (previewFrameMetrics, error) {
	key := labPreviewEvidenceCacheKey(descriptor)
	if cached, ok := labPreviewEvidenceCacheGet(key); ok && cached.Metrics != nil {
		return *cached.Metrics, nil
	}
	sourceWidth, sourceHeight := squarePixelFrameDimensions(evidence.Source)
	outputWidth, outputHeight := squarePixelFrameDimensions(evidence.Output)
	result := previewFrameMetrics{SourceDimensions: fmt.Sprintf("%dx%d", sourceWidth, sourceHeight), OutputDimensions: fmt.Sprintf("%dx%d", outputWidth, outputHeight)}
	if sourceWidth != outputWidth || sourceHeight != outputHeight {
		result.Reason = "PSNR and SSIM were skipped because the displayed frame geometry differs."
		labPreviewEvidenceCachePutMetrics(key, result)
		return result, nil
	}
	sourceFilter := joinPreviewFilters(descriptor.Normalization.Filter, squarePixelFrameFilter(evidence.Source), "format=yuv420p")
	outputFilter := joinPreviewFilters(squarePixelFrameFilter(evidence.Output), "format=yuv420p")
	ssim, err := runPreviewFrameMetric(ctx, descriptor.Path, descriptor.Start, sourceFilter, descriptor.CachePath, outputFilter, "ssim")
	if err != nil {
		if strings.Contains(err.Error(), "did not report SSIM") {
			result.Reason = "Frame geometry is comparable, but FFmpeg did not provide SSIM for the selected decoded frame."
			labPreviewEvidenceCachePutMetrics(key, result)
			return result, nil
		}
		return result, err
	}
	psnr, err := runPreviewFrameMetric(ctx, descriptor.Path, descriptor.Start, sourceFilter, descriptor.CachePath, outputFilter, "psnr")
	if err != nil {
		return result, err
	}
	result.Comparable, result.SSIM, result.PSNR = true, ssim, psnr
	result.Reason = "Both decoded frames were compared in the same color domain, pixel format, and square-pixel geometry."
	labPreviewEvidenceCachePutMetrics(key, result)
	return result, nil
}

// ensureLabCompatiblePreviewRequest uses the canonical #1 request identity,
// persistence, descriptor preparation, and artifact cache for additional LAB
// windows. It intentionally does not start a generation; #15/#1 lifecycle
// ownership begins when the operation acquires the resulting descriptor.
func (h AssetHandler) ensureLabCompatiblePreviewRequest(input compatiblePreviewRequest) (string, *compatiblePreviewDescriptor, error) {
	now := time.Now()
	normalized, err := normalizeCompatiblePreviewRequest(input)
	if err != nil {
		return "", nil, err
	}
	id, err := compatiblePreviewRequestIdentity(normalized)
	if err != nil {
		return "", nil, err
	}
	entry, tombstone, known := compatiblePreviewRequestLookup(id, now)
	var descriptor *compatiblePreviewDescriptor
	var installEntry *compatiblePreviewRequestEntry
	if known && entry != nil {
		entry.mu.RLock()
		recoveryState := entry.recoveryState
		entry.mu.RUnlock()
		if recoveryState == "" {
			descriptor, err = h.compatiblePreviewDescriptorForEntry(entry, id)
			if err == nil {
				installEntry = entry
			}
		}
	}
	if installEntry == nil {
		candidate := newCompatiblePreviewRequestEntry(normalized, now, h.db)
		descriptor, err = h.compatiblePreviewDescriptorForEntry(candidate, id)
		if err != nil {
			return "", nil, err
		}
		installEntry = candidate
	}
	descriptor.frozen = h.db != nil && descriptor.RetentionPolicy != compatiblePreviewEphemeralRetention
	if err := persistCompatiblePreviewPreparedRequest(h.db, id, normalized, descriptor, now); err != nil {
		return "", nil, err
	}
	if installEntry != entry || tombstone != nil || !known {
		installCompatiblePreviewRequestEntry(id, installEntry)
	}
	return id, descriptor, nil
}
