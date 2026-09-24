package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	compatiblePreviewHTTPWaitBudgetDefault  = 5 * time.Second
	compatiblePreviewLogicalLeaseLifetime   = 45 * time.Second
	compatiblePreviewLogicalLeaseRenewal    = 15 * time.Second
	compatiblePreviewSlowUXThreshold        = 30 * time.Second
	compatiblePreviewRetryAfterSeconds      = 2
	compatiblePreviewLeaseMaxItemsDefault   = 1000
	compatiblePreviewLeaseMetadataRetention = 2 * time.Minute
	compatiblePreviewOwnerMaxLength         = 128
)

var compatiblePreviewHTTPWaitBudget = compatiblePreviewHTTPWaitBudgetDefault

// This is a variable so deterministic registry tests can exercise capacity
// without constructing one thousand long-lived preview requests. Production
// uses the documented default and never evicts active owners.
var compatiblePreviewLeaseMaxItems = compatiblePreviewLeaseMaxItemsDefault

type compatiblePreviewLeaseState string

const (
	compatiblePreviewLeaseActive   compatiblePreviewLeaseState = "active"
	compatiblePreviewLeaseReleased compatiblePreviewLeaseState = "released"
	compatiblePreviewLeaseExpired  compatiblePreviewLeaseState = "expired"
)

type compatiblePreviewLogicalLease struct {
	requestID     string
	ownerID       string
	token         string
	entry         *compatiblePreviewArtifact
	artifactLease *compatiblePreviewArtifactLease
	expiresAt     time.Time
	state         compatiblePreviewLeaseState
	releasedAt    time.Time
}

var compatiblePreviewLeases = struct {
	sync.Mutex
	byToken map[string]*compatiblePreviewLogicalLease
	byOwner map[string]*compatiblePreviewLogicalLease
}{byToken: map[string]*compatiblePreviewLogicalLease{}, byOwner: map[string]*compatiblePreviewLogicalLease{}}

var compatiblePreviewLeaseCleanupOnce sync.Once

func startCompatiblePreviewLeaseCleanup() {
	compatiblePreviewLeaseCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for now := range ticker.C {
				cleanupCompatiblePreviewLeases(now)
			}
		}()
	})
}

func compatiblePreviewLeaseOwnerKey(requestID, ownerID string) string {
	return requestID + "\x00" + ownerID
}

func validCompatiblePreviewOwnerID(ownerID string) bool {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" || len(ownerID) > compatiblePreviewOwnerMaxLength {
		return false
	}
	for _, char := range ownerID {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}

func newCompatiblePreviewLeaseToken() string {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("lease-%d", time.Now().UnixNano())
}

func cleanupCompatiblePreviewLeases(now time.Time) {
	var release []*compatiblePreviewArtifactLease
	compatiblePreviewLeases.Lock()
	for token, lease := range compatiblePreviewLeases.byToken {
		if lease.state == compatiblePreviewLeaseActive && !lease.expiresAt.After(now) {
			lease.state = compatiblePreviewLeaseExpired
			lease.releasedAt = now
			delete(compatiblePreviewLeases.byOwner, compatiblePreviewLeaseOwnerKey(lease.requestID, lease.ownerID))
			release = append(release, lease.artifactLease)
		}
		if lease.state != compatiblePreviewLeaseActive && !lease.releasedAt.IsZero() && !lease.releasedAt.Add(compatiblePreviewLeaseMetadataRetention).After(now) {
			delete(compatiblePreviewLeases.byToken, token)
		}
	}
	compatiblePreviewLeases.Unlock()
	for _, artifactLease := range release {
		artifactLease.release()
	}
}

func resetCompatiblePreviewLeases() {
	var release []*compatiblePreviewArtifactLease
	compatiblePreviewLeases.Lock()
	for token, lease := range compatiblePreviewLeases.byToken {
		if lease.state == compatiblePreviewLeaseActive {
			lease.state = compatiblePreviewLeaseReleased
			lease.releasedAt = time.Now()
			release = append(release, lease.artifactLease)
		}
		delete(compatiblePreviewLeases.byToken, token)
	}
	compatiblePreviewLeases.byOwner = map[string]*compatiblePreviewLogicalLease{}
	compatiblePreviewLeases.Unlock()
	for _, artifactLease := range release {
		artifactLease.release()
	}
}

func createCompatiblePreviewLogicalLease(requestID, ownerID string, entry *compatiblePreviewArtifact, artifactLease *compatiblePreviewArtifactLease, now time.Time) (*compatiblePreviewLogicalLease, bool, error) {
	if strings.TrimSpace(requestID) == "" || !validCompatiblePreviewOwnerID(ownerID) || entry == nil || artifactLease == nil {
		return nil, false, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "invalid_lease", State: "generating", Reason: "compatible preview owner or lease is invalid"}
	}
	cleanupCompatiblePreviewLeases(now)
	startCompatiblePreviewLeaseCleanup()
	key := compatiblePreviewLeaseOwnerKey(requestID, ownerID)
	compatiblePreviewLeases.Lock()
	if existing := compatiblePreviewLeases.byOwner[key]; existing != nil && existing.state == compatiblePreviewLeaseActive && existing.expiresAt.After(now) {
		if existing.entry != entry {
			compatiblePreviewLeases.Unlock()
			artifactLease.release()
			return nil, false, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "invalid_lease", State: "generating", Reason: "compatible preview owner is bound to a different artifact generation"}
		}
		existing.expiresAt = now.Add(compatiblePreviewLogicalLeaseLifetime)
		compatiblePreviewLeases.Unlock()
		artifactLease.release()
		return existing, true, nil
	}
	if len(compatiblePreviewLeases.byToken) >= compatiblePreviewLeaseMaxItems {
		// Retained terminal records support deterministic repeated cancel, but
		// must not prevent capacity recovery. Under pressure, discard only one
		// non-active record; active ownership is never evicted.
		for token, retained := range compatiblePreviewLeases.byToken {
			if retained.state == compatiblePreviewLeaseActive {
				continue
			}
			delete(compatiblePreviewLeases.byToken, token)
			break
		}
		if len(compatiblePreviewLeases.byToken) >= compatiblePreviewLeaseMaxItems {
			compatiblePreviewLeases.Unlock()
			return nil, false, &compatiblePreviewLifecycleError{Status: http.StatusTooManyRequests, Code: "lease_capacity", State: "generating", Retryable: true, Reason: "compatible preview logical lease capacity is exhausted"}
		}
	}
	lease := &compatiblePreviewLogicalLease{requestID: requestID, ownerID: ownerID, token: newCompatiblePreviewLeaseToken(), entry: entry, artifactLease: artifactLease, expiresAt: now.Add(compatiblePreviewLogicalLeaseLifetime), state: compatiblePreviewLeaseActive}
	compatiblePreviewLeases.byToken[lease.token] = lease
	compatiblePreviewLeases.byOwner[key] = lease
	compatiblePreviewLeases.Unlock()
	return lease, false, nil
}

func findCompatiblePreviewLease(requestID, ownerID, token string, now time.Time) (*compatiblePreviewLogicalLease, error) {
	cleanupCompatiblePreviewLeases(now)
	compatiblePreviewLeases.Lock()
	lease := compatiblePreviewLeases.byToken[strings.TrimSpace(token)]
	if lease == nil || lease.requestID != requestID || (ownerID != "" && lease.ownerID != ownerID) {
		compatiblePreviewLeases.Unlock()
		return nil, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "invalid_lease", State: "generating", Reason: "compatible preview lease is not valid for this request"}
	}
	if lease.state == compatiblePreviewLeaseExpired {
		compatiblePreviewLeases.Unlock()
		return nil, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "lease_expired", State: "generating", Reason: "compatible preview lease has expired"}
	}
	if lease.state == compatiblePreviewLeaseReleased {
		compatiblePreviewLeases.Unlock()
		return lease, nil
	}
	if !lease.expiresAt.After(now) {
		lease.state = compatiblePreviewLeaseExpired
		lease.releasedAt = now
		delete(compatiblePreviewLeases.byOwner, compatiblePreviewLeaseOwnerKey(lease.requestID, lease.ownerID))
		artifactLease := lease.artifactLease
		compatiblePreviewLeases.Unlock()
		artifactLease.release()
		return nil, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "lease_expired", State: "generating", Reason: "compatible preview lease has expired"}
	}
	compatiblePreviewLeases.Unlock()
	return lease, nil
}

func renewCompatiblePreviewLease(lease *compatiblePreviewLogicalLease, now time.Time) error {
	if lease == nil {
		return &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "invalid_lease", State: "generating", Reason: "compatible preview lease is invalid"}
	}
	compatiblePreviewLeases.Lock()
	defer compatiblePreviewLeases.Unlock()
	if lease.state == compatiblePreviewLeaseReleased {
		return nil
	}
	if lease.state != compatiblePreviewLeaseActive || !lease.expiresAt.After(now) {
		return &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "lease_expired", State: "generating", Reason: "compatible preview lease has expired"}
	}
	lease.expiresAt = now.Add(compatiblePreviewLogicalLeaseLifetime)
	return nil
}

func releaseCompatiblePreviewLogicalLease(lease *compatiblePreviewLogicalLease, now time.Time) bool {
	if lease == nil {
		return false
	}
	compatiblePreviewLeases.Lock()
	if lease.state != compatiblePreviewLeaseActive {
		compatiblePreviewLeases.Unlock()
		return false
	}
	lease.state = compatiblePreviewLeaseReleased
	lease.releasedAt = now
	delete(compatiblePreviewLeases.byOwner, compatiblePreviewLeaseOwnerKey(lease.requestID, lease.ownerID))
	artifactLease := lease.artifactLease
	compatiblePreviewLeases.Unlock()
	artifactLease.release()
	return true
}

func releaseCompatiblePreviewLeasesForArtifact(entry *compatiblePreviewArtifact, now time.Time) {
	var release []*compatiblePreviewArtifactLease
	compatiblePreviewLeases.Lock()
	for _, lease := range compatiblePreviewLeases.byToken {
		if lease.state == compatiblePreviewLeaseActive && lease.entry == entry {
			lease.state = compatiblePreviewLeaseReleased
			lease.releasedAt = now
			delete(compatiblePreviewLeases.byOwner, compatiblePreviewLeaseOwnerKey(lease.requestID, lease.ownerID))
			release = append(release, lease.artifactLease)
		}
	}
	compatiblePreviewLeases.Unlock()
	for _, artifactLease := range release {
		artifactLease.release()
	}
}

type compatiblePreviewLifecycleRequest struct {
	OwnerID    string `json:"ownerId"`
	LeaseToken string `json:"leaseToken,omitempty"`
}

func decodeCompatiblePreviewLifecycleRequest(c *gin.Context) (compatiblePreviewLifecycleRequest, error) {
	var request compatiblePreviewLifecycleRequest
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		request.OwnerID = strings.TrimSpace(c.Query("ownerId"))
		request.LeaseToken = strings.TrimSpace(c.Query("leaseToken"))
	} else {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
			return request, fmt.Errorf("compatible preview lifecycle request must contain valid JSON")
		}
		request.OwnerID = strings.TrimSpace(request.OwnerID)
		request.LeaseToken = strings.TrimSpace(request.LeaseToken)
	}
	if request.OwnerID != "" && !validCompatiblePreviewOwnerID(request.OwnerID) {
		return request, fmt.Errorf("ownerId is invalid")
	}
	return request, nil
}

func compatiblePreviewArtifactForDescriptor(descriptor *compatiblePreviewDescriptor) *compatiblePreviewArtifact {
	if descriptor == nil {
		return nil
	}
	compatiblePreviewArtifacts.Lock()
	entry := compatiblePreviewArtifacts.items[descriptor.ArtifactID]
	compatiblePreviewArtifacts.Unlock()
	return entry
}

func compatiblePreviewStatusProjection(requestID string, entry *compatiblePreviewRequestEntry, descriptor *compatiblePreviewDescriptor) map[string]any {
	response := map[string]any{"requestId": requestID, "code": "idle", "state": "idle", "retryable": false, "recreate": false}
	if descriptor != nil {
		response["artifactId"] = descriptor.ArtifactID
		response["path"] = descriptor.Path
		response["sourceRevision"] = labSourceRevisionMap(descriptor.SourceRevision)
		response["start"] = descriptor.Start
		response["seconds"] = descriptor.Seconds
		response["normalization"] = labNormalizationMap(descriptor.Normalization)
		response["canonicalEncodeSignature"] = descriptor.CanonicalEncodeSignature
		response["generationToken"] = descriptor.GenerationToken
		response["generationVersion"] = descriptor.GenerationVersion
		response["encoder"] = descriptor.EffectiveVideoEncoder
		if response["encoder"] == "" {
			response["encoder"] = descriptor.RequestedVideoEncoder
		}
		preset := strings.TrimSpace(descriptor.Request.VideoPreset)
		if preset == "" && descriptor.EffectivePreviewProfile != nil {
			preset = workerStringValue(descriptor.EffectivePreviewProfile.WorkerConfig["videoPreset"])
		}
		if preset != "" {
			response["preset"] = preset
		}
		if compatiblePreviewPublishedArtifactValidForDescriptor(descriptor) {
			response["code"], response["state"] = "ready", "ready"
		}
	}
	if entry == nil {
		return response
	}
	entry.mu.RLock()
	recoveryState, recoveryReason, runtimeDescriptor := entry.recoveryState, entry.recoveryReason, entry.Descriptor
	entry.mu.RUnlock()
	if descriptor == nil {
		descriptor = runtimeDescriptor
	}
	if recoveryState != "" {
		state := recoveryState
		if state == string(compatiblePreviewArtifactStale) {
			response["code"], response["state"] = "stale_artifact", "stale"
		} else {
			response["code"], response["state"] = state, state
		}
		failure := compatiblePreviewLifecycleFailure(recoveryState, recoveryReason).(*compatiblePreviewLifecycleError)
		response["retryable"], response["recreate"], response["reason"] = failure.Retryable, failure.Recreate, failure.Reason
		return response
	}
	runtime := compatiblePreviewArtifactForDescriptor(descriptor)
	if runtime == nil {
		return response
	}
	runtime.mu.Lock()
	state, reason, startedAt, finishedAt, cacheHit := runtime.state, runtime.err, runtime.generationStartedAt, runtime.generationFinishedAt, runtime.cacheHit
	runtime.mu.Unlock()
	if state == compatiblePreviewArtifactGenerating {
		response["code"], response["state"], response["retryable"] = "generating", "generating", true
		if !startedAt.IsZero() {
			response["startedAt"] = startedAt.Format(time.RFC3339Nano)
			response["elapsedSeconds"] = int64(time.Since(startedAt).Seconds())
		}
		if time.Since(startedAt) >= compatiblePreviewSlowUXThreshold {
			response["slow"] = true
		}
	} else if state == compatiblePreviewArtifactReady {
		response["code"], response["state"] = "ready", "ready"
		if cacheHit {
			response["cacheHit"] = true
		}
	} else {
		failure := compatiblePreviewLifecycleFailure(string(state), compatiblePreviewErrorString(reason))
		if lifecycleErr, ok := failure.(*compatiblePreviewLifecycleError); ok {
			response["code"], response["state"] = lifecycleErr.Code, lifecycleErr.State
			response["retryable"], response["recreate"], response["reason"] = lifecycleErr.Retryable, lifecycleErr.Recreate, lifecycleErr.Reason
		}
	}
	if !finishedAt.IsZero() {
		response["finishedAt"] = finishedAt.Format(time.RFC3339Nano)
	}
	return response
}

func compatiblePreviewGeneratingEnvelope(requestID string, descriptor *compatiblePreviewDescriptor, lease *compatiblePreviewLogicalLease, cacheHit bool) gin.H {
	response := gin.H{"requestId": requestID, "artifactId": descriptor.ArtifactID, "code": "generating", "state": "generating", "retryable": true, "recreate": false, "leaseToken": lease.token, "ownerId": lease.ownerID, "leaseExpiresAt": lease.expiresAt.Format(time.RFC3339Nano), "statusUrl": "/api/assets/preview/compatible/requests/" + requestID + "/status", "renewUrl": "/api/assets/preview/compatible/requests/" + requestID + "/start", "cancelUrl": "/api/assets/preview/compatible/requests/" + requestID + "/cancel", "cacheHit": cacheHit, "retryAfterSeconds": compatiblePreviewRetryAfterSeconds}
	response["renewAfterSeconds"] = int(compatiblePreviewLogicalLeaseRenewal.Seconds())
	entry := compatiblePreviewArtifactForDescriptor(descriptor)
	if entry != nil {
		entry.mu.Lock()
		startedAt := entry.generationStartedAt
		entry.mu.Unlock()
		if !startedAt.IsZero() {
			response["startedAt"] = startedAt.Format(time.RFC3339Nano)
			response["elapsedSeconds"] = int64(time.Since(startedAt).Seconds())
		}
	}
	response["encoder"] = descriptor.EffectiveVideoEncoder
	if response["encoder"] == "" {
		response["encoder"] = descriptor.RequestedVideoEncoder
	}
	if preset := strings.TrimSpace(descriptor.Request.VideoPreset); preset != "" {
		response["preset"] = preset
	}
	return response
}

func (h AssetHandler) compatiblePreviewLifecycleEntry(c *gin.Context) (*compatiblePreviewRequestEntry, *compatiblePreviewRequestTombstone, string, bool) {
	requestID := strings.TrimSpace(c.Param("requestId"))
	entry, tombstone, known := compatiblePreviewRequestLookup(requestID, time.Now())
	return entry, tombstone, requestID, known
}

func (h AssetHandler) CompatiblePreviewRequestStatus(c *gin.Context) {
	entry, tombstone, requestID, known := h.compatiblePreviewLifecycleEntry(c)
	if !known {
		c.JSON(http.StatusNotFound, gin.H{"error": "compatible preview request is unknown", "code": "unknown_request", "state": "unknown"})
		return
	}
	if tombstone != nil {
		c.JSON(http.StatusGone, gin.H{"error": tombstone.Reason, "code": "expired", "state": "expired", "retryable": true, "recreate": true})
		return
	}
	entry.mu.RLock()
	descriptor := entry.Descriptor
	entry.mu.RUnlock()
	c.JSON(http.StatusOK, compatiblePreviewStatusProjection(requestID, entry, descriptor))
}

func (h AssetHandler) CompatiblePreviewRequestStart(c *gin.Context) {
	entry, tombstone, requestID, known := h.compatiblePreviewLifecycleEntry(c)
	if !known {
		c.JSON(http.StatusNotFound, gin.H{"error": "compatible preview request is unknown", "code": "unknown_request", "state": "unknown"})
		return
	}
	if tombstone != nil {
		writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(compatiblePreviewRequestStateExpiredTombstone, tombstone.Reason))
		return
	}
	lifecycleRequest, err := decodeCompatiblePreviewLifecycleRequest(c)
	if err != nil || !validCompatiblePreviewOwnerID(lifecycleRequest.OwnerID) {
		c.JSON(http.StatusConflict, gin.H{"error": "compatible preview owner is invalid", "code": "invalid_lease", "state": "generating"})
		return
	}
	entry.mu.RLock()
	recoveryState, recoveryReason, descriptor := entry.recoveryState, entry.recoveryReason, entry.Descriptor
	entry.mu.RUnlock()
	if recoveryState != "" {
		writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(recoveryState, recoveryReason))
		return
	}
	if descriptor == nil {
		descriptor, err = h.compatiblePreviewDescriptorForEntry(entry, requestID)
		if err != nil {
			writeCompatiblePreviewError(c, err)
			return
		}
	}
	now := time.Now()
	if lifecycleRequest.LeaseToken != "" {
		lease, leaseErr := findCompatiblePreviewLease(requestID, lifecycleRequest.OwnerID, lifecycleRequest.LeaseToken, now)
		if leaseErr != nil {
			writeCompatiblePreviewError(c, leaseErr)
			return
		}
		if lease.state == compatiblePreviewLeaseReleased {
			lease.entry.mu.Lock()
			leaseState, leaseReason := lease.entry.state, compatiblePreviewErrorString(lease.entry.err)
			lease.entry.mu.Unlock()
			switch leaseState {
			case compatiblePreviewArtifactReady:
				c.JSON(http.StatusOK, compatiblePreviewStatusProjection(requestID, entry, descriptor))
			case compatiblePreviewArtifactFailed, compatiblePreviewArtifactCanceled, compatiblePreviewArtifactInterrupted, compatiblePreviewArtifactStale:
				writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(string(leaseState), leaseReason))
			default:
				c.JSON(http.StatusConflict, gin.H{"error": "compatible preview lease was released", "code": "invalid_lease", "state": "generating"})
			}
			return
		}
		lease.entry.mu.Lock()
		leaseState := lease.entry.state
		lease.entry.mu.Unlock()
		if leaseState != compatiblePreviewArtifactGenerating {
			releaseCompatiblePreviewLogicalLease(lease, now)
			if leaseState == compatiblePreviewArtifactReady {
				response := compatiblePreviewStatusProjection(requestID, entry, descriptor)
				c.JSON(http.StatusOK, response)
			} else {
				writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(string(leaseState), "compatible preview generation reached a terminal state"))
			}
			return
		}
		if err := renewCompatiblePreviewLease(lease, now); err != nil {
			writeCompatiblePreviewError(c, err)
			return
		}
		c.Header("Retry-After", strconv.Itoa(compatiblePreviewRetryAfterSeconds))
		c.JSON(http.StatusAccepted, compatiblePreviewGeneratingEnvelope(requestID, descriptor, lease, false))
		return
	}
	if !validCompatiblePreviewOwnerID(lifecycleRequest.OwnerID) {
		c.JSON(http.StatusConflict, gin.H{"error": "ownerId is invalid", "code": "invalid_lease", "state": "generating"})
		return
	}
	cacheHit, entryArtifact, artifactLease, beginErr := beginCompatiblePreviewArtifact(descriptor)
	if beginErr != nil {
		writeCompatiblePreviewError(c, compatiblePreviewActionError(entryArtifact, beginErr))
		return
	}
	if cacheHit {
		artifactLease.release()
		response := compatiblePreviewStatusProjection(requestID, entry, descriptor)
		response["cacheHit"] = true
		c.JSON(http.StatusOK, response)
		return
	}
	lease, _, leaseErr := createCompatiblePreviewLogicalLease(requestID, lifecycleRequest.OwnerID, entryArtifact, artifactLease, now)
	if leaseErr != nil {
		artifactLease.release()
		writeCompatiblePreviewError(c, leaseErr)
		return
	}
	c.Header("Retry-After", strconv.Itoa(compatiblePreviewRetryAfterSeconds))
	c.JSON(http.StatusAccepted, compatiblePreviewGeneratingEnvelope(requestID, descriptor, lease, false))
}

func (h AssetHandler) CompatiblePreviewRequestCancel(c *gin.Context) {
	entry, tombstone, requestID, known := h.compatiblePreviewLifecycleEntry(c)
	if !known {
		c.JSON(http.StatusNotFound, gin.H{"error": "compatible preview request is unknown", "code": "unknown_request", "state": "unknown"})
		return
	}
	if tombstone != nil {
		writeCompatiblePreviewError(c, compatiblePreviewLifecycleFailure(compatiblePreviewRequestStateExpiredTombstone, tombstone.Reason))
		return
	}
	lifecycleRequest, err := decodeCompatiblePreviewLifecycleRequest(c)
	if err != nil || !validCompatiblePreviewOwnerID(lifecycleRequest.OwnerID) || lifecycleRequest.LeaseToken == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "compatible preview lease is invalid", "code": "invalid_lease", "state": "generating"})
		return
	}
	lease, leaseErr := findCompatiblePreviewLease(requestID, lifecycleRequest.OwnerID, lifecycleRequest.LeaseToken, time.Now())
	if leaseErr != nil {
		writeCompatiblePreviewError(c, leaseErr)
		return
	}
	if lease.state == compatiblePreviewLeaseReleased {
		projection := compatiblePreviewStatusProjection(requestID, entry, nil)
		c.JSON(http.StatusOK, gin.H{"requestId": requestID, "code": projection["code"], "state": projection["state"], "leaseReleased": false})
		return
	}
	released := releaseCompatiblePreviewLogicalLease(lease, time.Now())
	state := "generating"
	if entry != nil {
		projection := compatiblePreviewStatusProjection(requestID, entry, nil)
		if projected, ok := projection["state"].(string); ok && projected != "idle" {
			state = projected
		}
	}
	c.JSON(http.StatusOK, gin.H{"requestId": requestID, "code": state, "state": state, "leaseReleased": released})
}

func compatiblePreviewActionError(entry *compatiblePreviewArtifact, err error) error {
	if entry != nil {
		entry.mu.Lock()
		state, reason := entry.state, compatiblePreviewErrorString(entry.err)
		entry.mu.Unlock()
		if state == compatiblePreviewArtifactFailed || state == compatiblePreviewArtifactCanceled || state == compatiblePreviewArtifactInterrupted || state == compatiblePreviewArtifactStale {
			return compatiblePreviewLifecycleFailure(string(state), reason)
		}
	}
	if err != nil && !errors.Is(err, errCompatiblePreviewWaitBudget) {
		return err
	}
	if entry == nil {
		return err
	}
	entry.mu.Lock()
	state, reason := entry.state, compatiblePreviewErrorString(entry.err)
	entry.mu.Unlock()
	if state == compatiblePreviewArtifactFailed || state == compatiblePreviewArtifactCanceled || state == compatiblePreviewArtifactInterrupted || state == compatiblePreviewArtifactStale {
		return compatiblePreviewLifecycleFailure(string(state), reason)
	}
	return err
}

func promoteCompatiblePreviewLogicalLease(c *gin.Context, descriptor *compatiblePreviewDescriptor, entry *compatiblePreviewArtifact, httpLease *compatiblePreviewArtifactLease) (*compatiblePreviewLogicalLease, error) {
	if descriptor == nil || strings.TrimSpace(descriptor.requestID) == "" {
		httpLease.release()
		return nil, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "legacy_non_followable", State: "generating", Reason: "slow legacy preview has no registered request identity", Retryable: true}
	}
	lifecycleRequest := compatiblePreviewLifecycleRequest{OwnerID: strings.TrimSpace(c.Query("ownerId")), LeaseToken: strings.TrimSpace(c.Query("leaseToken"))}
	if lifecycleRequest.LeaseToken != "" {
		lease, err := findCompatiblePreviewLease(descriptor.requestID, lifecycleRequest.OwnerID, lifecycleRequest.LeaseToken, time.Now())
		if err != nil {
			httpLease.release()
			return nil, err
		}
		if lease.state != compatiblePreviewLeaseActive || lease.entry != entry {
			httpLease.release()
			return nil, &compatiblePreviewLifecycleError{Status: http.StatusConflict, Code: "invalid_lease", State: "generating", Reason: "compatible preview lease is not active for this artifact"}
		}
		httpLease.release()
		if err := renewCompatiblePreviewLease(lease, time.Now()); err != nil {
			return nil, err
		}
		return lease, nil
	}
	if lifecycleRequest.OwnerID == "" {
		lifecycleRequest.OwnerID = "resource-" + newCompatiblePreviewLeaseToken()[:24]
	}
	lease, _, err := createCompatiblePreviewLogicalLease(descriptor.requestID, lifecycleRequest.OwnerID, entry, httpLease, time.Now())
	if err != nil {
		httpLease.release()
	}
	return lease, err
}
