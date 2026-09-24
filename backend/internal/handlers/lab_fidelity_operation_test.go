package handlers

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func resetLabFidelityEvidenceForTest() {
	labPreviewEvidence.Lock()
	labPreviewEvidence.items = map[string]labPreviewEvidenceCacheEntry{}
	labPreviewEvidence.Unlock()
}

func TestLabFidelityWindowPlansPutPrimaryFirstAndDeduplicateBySourceInterval(t *testing.T) {
	plan := SamplingPlan{WindowSeconds: 20, Positions: []float64{0.5, 0.5, 0.8}}
	windows := labFidelityWindowPlans(40, 20, 100, plan)
	if len(windows) != 2 {
		t.Fatalf("windows=%d, want primary plus one non-overlapping policy window: %#v", len(windows), windows)
	}
	if !windows[0].Primary || windows[0].Start != 40 || windows[0].Duration != 20 {
		t.Fatalf("primary window was not preserved as the first observation: %#v", windows[0])
	}
	if windows[1].Start != 70 {
		t.Fatalf("deduplicated source-time window start=%v, want 70", windows[1].Start)
	}
}

func TestLabFidelityHeavySlotLimitsConcurrentOperations(t *testing.T) {
	labFidelityHeavySlot <- struct{}{}
	attempted := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		close(attempted)
		labFidelityHeavySlot <- struct{}{}
		close(acquired)
	}()
	<-attempted
	select {
	case <-acquired:
		t.Fatal("second LAB Fidelity operation acquired the global heavy-work slot concurrently")
	default:
	}
	<-labFidelityHeavySlot
	<-acquired
	<-labFidelityHeavySlot
}

func TestLabFidelityPrimaryConsumersShareOneActualGeneration(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	installFakeFFmpeg(t, `printf primary-artifact`)
	descriptor := testCompatiblePreviewDescriptor(t, "primary-once")
	var generations atomic.Int32
	previous := compatiblePreviewGenerationStarted
	compatiblePreviewGenerationStarted = func(*compatiblePreviewArtifact) { generations.Add(1) }
	t.Cleanup(func() { compatiblePreviewGenerationStarted = previous })

	_, firstLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || firstLease == nil {
		t.Fatalf("primary generation failed: lease=%v err=%v", firstLease, err)
	}
	firstLease.release()
	for consumer := 0; consumer < 3; consumer++ {
		hit, lease, acquireErr := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
		if acquireErr != nil || !hit || lease == nil {
			t.Fatalf("consumer %d did not reuse primary artifact: hit=%t lease=%v err=%v", consumer, hit, lease, acquireErr)
		}
		lease.release()
	}
	if got := generations.Load(); got != 1 {
		t.Fatalf("primary artifact generation count=%d, want exactly one", got)
	}
}

func TestLabFidelityCancellationCannotOverwriteTerminalOperation(t *testing.T) {
	store := labFidelityOperationStore{items: map[string]*LabFidelityOperation{}, cancels: map[string]context.CancelFunc{}}
	now := time.Now()
	store.items["operation"] = &LabFidelityOperation{ID: "operation", Status: labFidelityRunning, CreatedAt: now, UpdatedAt: now}
	canceled := false
	store.cancels["operation"] = func() { canceled = true }
	if _, ok := store.cancel("operation"); !ok || !canceled {
		t.Fatal("cancel did not transition or signal the operation")
	}
	store.update("operation", func(operation *LabFidelityOperation) { operation.Status = labFidelityCompleted })
	operation, _ := store.copy("operation")
	if operation.Status != labFidelityCanceled {
		t.Fatalf("late worker update overwrote canceled operation with %q", operation.Status)
	}
}

func TestLabFidelityArtifactUseProtectionBlocksCleanupUntilAllConsumersRelease(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	directory := t.TempDir()
	descriptor := testCompatiblePreviewDescriptor(t, "lab-guard")
	descriptor.CachePath = filepath.Join(directory, "lab-guard.mp4")
	descriptor.RetentionPolicy = compatiblePreviewEphemeralRetention
	if err := os.WriteFile(descriptor.CachePath, []byte("ready artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := newCompatiblePreviewArtifactEntry(descriptor, time.Now(), compatiblePreviewArtifactReady)
	entry.scopeExpiresAt = time.Now().Add(-time.Second)
	compatiblePreviewArtifacts.Lock()
	compatiblePreviewArtifacts.items[descriptor.ArtifactID] = entry
	compatiblePreviewArtifacts.Unlock()

	_, fidelityLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || fidelityLease == nil {
		t.Fatalf("Fidelity could not acquire ready-artifact protection: lease=%v err=%v", fidelityLease, err)
	}
	_, playbackLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || playbackLease == nil {
		t.Fatalf("playback could not share ready-artifact protection: lease=%v err=%v", playbackLease, err)
	}

	cleanupCompatiblePreviewEphemeralArtifacts(directory, time.Now())
	if _, err := os.Stat(descriptor.CachePath); err != nil {
		t.Fatalf("cleanup removed artifact while two consumers were active: %v", err)
	}
	fidelityLease.release()
	cleanupCompatiblePreviewEphemeralArtifacts(directory, time.Now())
	if _, err := os.Stat(descriptor.CachePath); err != nil {
		t.Fatalf("cleanup removed artifact while playback consumer remained: %v", err)
	}
	playbackLease.release()
	// The lease release makes the artifact eligible; make the retention
	// boundary explicit so this assertion does not depend on wall-clock
	// elapsed time or a prior test's cleanup cadence.
	entry.mu.Lock()
	entry.scopeExpiresAt = time.Now().Add(-time.Second)
	entry.mu.Unlock()
	cleanupCompatiblePreviewEphemeralArtifacts(directory, time.Now())
	if _, err := os.Stat(descriptor.CachePath); !os.IsNotExist(err) {
		entry.mu.Lock()
		waiters, state, retention, destination := entry.waiters, entry.state, entry.retentionPolicy, entry.destination
		entry.mu.Unlock()
		t.Fatalf("released artifact was not eligible for normal cleanup: err=%v waiters=%d state=%s retention=%s destination=%s directory=%s", err, waiters, state, retention, destination, directory)
	}
}

func TestLabFidelityEvidenceIdentityIncludesGeneration(t *testing.T) {
	resetLabFidelityEvidenceForTest()
	t.Cleanup(resetLabFidelityEvidenceForTest)
	old := testCompatiblePreviewDescriptor(t, "same-artifact")
	old.GenerationToken, old.GenerationVersion = "generation-a", 1
	evidence := labPreviewArtifactEvidence{ArtifactID: old.ArtifactID, GenerationToken: old.GenerationToken, GenerationVersion: old.GenerationVersion}
	labPreviewEvidenceCachePut(labPreviewEvidenceCacheKey(old), labPreviewEvidenceCacheEntry{Evidence: evidence, UpdatedAt: time.Now()})
	if _, ok := labPreviewEvidenceCacheGet(labPreviewEvidenceCacheKey(old)); !ok {
		t.Fatal("stored evidence was not reusable for the same artifact generation")
	}
	replacement := *old
	replacement.GenerationToken, replacement.GenerationVersion = "generation-b", 2
	if _, ok := labPreviewEvidenceCacheGet(labPreviewEvidenceCacheKey(&replacement)); ok {
		t.Fatal("evidence from a replaced artifact generation was reused")
	}
}

func TestLabFidelityPartialAggregateKeepsCanonicalMismatchVisible(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "aggregate")
	windows := []models.JSONMap{{
		"source":            models.JSONMap{},
		"output":            models.JSONMap{},
		"metrics":           models.JSONMap{"comparable": true, "ssim": 0.99},
		"canonicalFidelity": models.JSONMap{"status": "mismatch"},
	}}
	result := labFidelityOperationResult(labFidelityOperationInput{MaterialSignature: "signature"}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, false)
	if result["aggregateStatus"] != "analysis_in_progress" || result["canonicalStatus"] != "mismatch" {
		t.Fatalf("partial result masked canonical mismatch: %#v", result)
	}
}

func TestLabFidelityAdditionalWindowUsesFrozenEffectiveProfile(t *testing.T) {
	profile := &models.Profile{Name: "Frozen", WorkerConfig: models.JSONMap{"videoFilters": "setfield=prog"}}
	descriptor := testCompatiblePreviewDescriptor(t, "frozen-profile")
	descriptor.Request.ProfileID = 42
	descriptor.Request.Profile = &models.Profile{Name: "Mutable", WorkerConfig: models.JSONMap{"videoFilters": "changed"}}
	descriptor.EffectivePreviewProfile = profile
	request := labFidelityAdditionalWindowRequest(descriptor, labFidelityWindowPlan{Start: 12, Duration: 20})
	if request.ProfileID != 0 || request.Profile == nil || request.Profile.Name != "Frozen" {
		t.Fatalf("additional window did not use frozen profile: %#v", request)
	}
	request.Profile.WorkerConfig["videoFilters"] = "mutated-copy"
	if profile.WorkerConfig["videoFilters"] != "setfield=prog" {
		t.Fatal("additional window request shared mutable worker config with frozen descriptor")
	}
}

func TestLabFidelityOperationCleanupKeepsActiveAndRemovesExpiredTerminal(t *testing.T) {
	now := time.Now()
	store := labFidelityOperationStore{items: map[string]*LabFidelityOperation{}, cancels: map[string]context.CancelFunc{}}
	store.items["active"] = &LabFidelityOperation{ID: "active", Status: labFidelityRunning, UpdatedAt: now.Add(-48 * time.Hour)}
	store.items["expired"] = &LabFidelityOperation{ID: "expired", Status: labFidelityFailed, UpdatedAt: now.Add(-48 * time.Hour)}
	store.cleanup(now)
	if _, ok := store.items["active"]; !ok {
		t.Fatal("cleanup removed an active process-local operation")
	}
	if _, ok := store.items["expired"]; ok {
		t.Fatal("cleanup retained expired terminal operation metadata")
	}
}

func TestLabFidelityAggregateKeepsCanonicalUnverifiedAboveLaterPass(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "aggregate-unverified")
	windows := []models.JSONMap{
		{"canonicalFidelity": models.JSONMap{"status": "unverified"}},
		{"canonicalFidelity": models.JSONMap{"status": "passed"}},
	}
	result := labFidelityOperationResult(labFidelityOperationInput{}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, true)
	if result["canonicalStatus"] != "unverified" {
		t.Fatalf("later passing window downgraded canonical unverified result: %#v", result)
	}
}
