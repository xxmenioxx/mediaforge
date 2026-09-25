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

func TestLabFidelityMaterialSignatureIsBackendOwnedAndBoundToExecutionIdentity(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "material-authority")
	descriptor.Path = "/media/raw/movie.mkv"
	descriptor.SourceRevision = compatiblePreviewSourceRevision{Size: 10, ModifiedAtNS: 20, Mode: 0o600}
	descriptor.Start = "00:00:05"
	descriptor.Seconds = 20
	descriptor.GenerationToken = "generation-a"
	descriptor.GenerationVersion = 1
	plan := SamplingPlan{AssetDurationSeconds: 120, WindowSeconds: 20, Positions: []float64{0.08, 0.27, 0.5}, InitialWindows: 3, InterlaceValidation: "automatic", CropDepth: "normal"}
	first := labFidelityMaterialSignature(descriptor, plan)
	if first == "" || first == "client-correlation" {
		t.Fatalf("backend material signature was not resolved independently: %q", first)
	}
	if got := labFidelityMaterialSignature(descriptor, plan); got != first {
		t.Fatalf("backend material signature was not deterministic: %q != %q", got, first)
	}
	replacement := *descriptor
	replacement.GenerationToken = "generation-b"
	replacement.GenerationVersion = 2
	if got := labFidelityMaterialSignature(&replacement, plan); got == first {
		t.Fatal("replaced artifact generation reused the old backend material signature")
	}
	changedPolicy := plan
	changedPolicy.Positions = []float64{0.08, 0.5, 0.92}
	if got := labFidelityMaterialSignature(descriptor, changedPolicy); got == first {
		t.Fatal("material signature ignored a materially changed sampling policy")
	}
}

func TestLabFidelityRunnerRejectsPrimaryGenerationReplacementBeforeEvidence(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "stale-primary")
	descriptor.SourceRevision = compatiblePreviewSourceRevision{Size: 10, ModifiedAtNS: 20, Mode: 0o600}
	descriptor.GenerationToken = "generation-a"
	descriptor.GenerationVersion = 1
	plan := SamplingPlan{AssetDurationSeconds: 120, WindowSeconds: 20, Positions: []float64{0.5}, InitialWindows: 1, InterlaceValidation: "automatic", CropDepth: "normal"}
	operation := LabFidelityOperation{
		PrimaryArtifactID: descriptor.ArtifactID, PrimaryGeneration: "generation-a:1",
		SourceRevision: labSourceRevisionMap(descriptor.SourceRevision), PrimaryStart: 0, PrimaryDuration: 2,
		Normalization: labNormalizationMap(descriptor.Normalization), PlanSignature: descriptor.CanonicalEncodeSignature,
		MaterialSignature: labFidelityMaterialSignature(descriptor, plan), samplingPlan: plan,
	}
	replaced := *descriptor
	replaced.GenerationToken = "generation-b"
	replaced.GenerationVersion = 2
	if labFidelityDescriptorMatchesOperation(operation, &replaced) {
		t.Fatal("operation accepted replacement generation B as frozen generation A")
	}
	if !labFidelityDescriptorMatchesOperation(operation, descriptor) {
		t.Fatal("operation rejected its unchanged frozen primary descriptor")
	}
}

func TestLabFidelityEvidenceKeysSeparateGlobalAndIntervalIdentity(t *testing.T) {
	base := testCompatiblePreviewDescriptor(t, "evidence-keys")
	base.SourceRevision = compatiblePreviewSourceRevision{Size: 10, ModifiedAtNS: 20, Mode: 0o600}
	base.Start, base.Seconds = "00:00:05", 20
	base.Normalization.Mode = "normalize_bt709"
	globalEquivalent := *base
	globalEquivalent.Start, globalEquivalent.Seconds = "00:00:30", 10
	globalEquivalent.Normalization.Mode = "preserve"
	if labSourceProbeKey(base) != labSourceProbeKey(&globalEquivalent) {
		t.Fatal("stream-global source evidence was incorrectly duplicated per interval or normalization")
	}
	if labPreviewEvidenceCacheKey(base) == labPreviewEvidenceCacheKey(&globalEquivalent) {
		t.Fatal("interval/output evidence key did not include interval and normalization identity")
	}
	replacedSource := *base
	replacedSource.SourceRevision.ModifiedAtNS++
	if labSourceProbeKey(base) == labSourceProbeKey(&replacedSource) {
		t.Fatal("stream-global source evidence key reused a replaced source revision")
	}
}

func TestLabFidelityThreeFrozenWindowsBindDistinctGenerationsAndKeepAReusable(t *testing.T) {
	resetLabFidelityEvidenceForTest()
	t.Cleanup(resetLabFidelityEvidenceForTest)
	plan := SamplingPlan{PlanIdentity: "lab-plan", SourceRevision: "source-lab", SelectedWindows: []SamplingWindow{
		{Identity: "primary-a", Primary: true, StartSeconds: 10, DurationSeconds: 20, Roles: []string{"primary"}},
		{Identity: "hard-b", StartSeconds: 100, DurationSeconds: 20, Roles: []string{"hard_scene"}},
		{Identity: "hard-c", StartSeconds: 200, DurationSeconds: 20, Roles: []string{"hard_scene"}},
	}}
	descriptors := []*compatiblePreviewDescriptor{
		testCompatiblePreviewDescriptor(t, "lab-a"), testCompatiblePreviewDescriptor(t, "lab-b"), testCompatiblePreviewDescriptor(t, "lab-c"),
	}
	for index, descriptor := range descriptors {
		descriptor.GenerationToken = string(rune('a' + index))
		descriptor.GenerationVersion = int64(index + 1)
	}
	windowIDs := []string{"primary-a", "hard-b", "hard-c"}
	pairs := make([]ComparisonPairIdentity, 0, len(descriptors))
	for index, descriptor := range descriptors {
		candidate, err := ComputeCandidateIdentity(CandidateIdentity{
			Kind: "compatible_preview", ArtifactID: descriptor.ArtifactID, GenerationToken: descriptor.GenerationToken,
			GenerationVersion: descriptor.GenerationVersion, OutputRevision: descriptor.GenerationToken, SourceRevision: plan.SourceRevision,
			StreamIdentity: "0:v:0", SegmentStartSeconds: 0, SegmentDurationSeconds: 20, TimelineIdentity: descriptor.GenerationToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		reference, err := ComputeReferenceIdentity(ReferenceIdentity{
			SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: windowIDs[index], SourceRevision: plan.SourceRevision,
			SourceStreamIdentity: "0:v:0", TransformIdentity: "transform-lab", TransformVersion: "v1",
			AlignmentIdentity: "alignment-" + windowIDs[index], ComparisonDomainIdentity: "domain-" + windowIDs[index],
		})
		if err != nil {
			t.Fatal(err)
		}
		pair, err := ComputeComparisonPairIdentity(ComparisonPairIdentity{
			SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: windowIDs[index], Reference: reference, Candidate: candidate,
			CoverageIdentity: descriptor.GenerationToken, SegmentMappingIdentity: descriptor.GenerationToken,
			AlignmentIdentity: reference.AlignmentIdentity, ComparisonDomainIdentity: reference.ComparisonDomainIdentity,
			SourceRevision: plan.SourceRevision, CandidateRevision: descriptor.GenerationToken,
		})
		if err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, pair)
		labPreviewEvidenceCachePutPair(pair)
	}
	if pairs[0].Candidate.Identity == pairs[1].Candidate.Identity || pairs[1].Candidate.Identity == pairs[2].Candidate.Identity || pairs[0].Identity == pairs[1].Identity {
		t.Fatal("LAB windows did not retain distinct artifact-generation/pair identities")
	}
	if _, ok := labPreviewEvidenceCacheGetPair(pairs[0].Identity); !ok {
		t.Fatal("primary window material evidence was not reusable")
	}
	replacedB := pairs[1]
	replacedB.Candidate.GenerationToken = "b-replacement"
	replacedB.Candidate.OutputRevision = "b-replacement"
	replacedB, replacementErr := ComputeComparisonPairIdentity(replacedB)
	if replacementErr != nil {
		t.Fatal(replacementErr)
	}
	if replacedB.Identity == pairs[1].Identity {
		t.Fatal("replacing LAB window B generation did not invalidate B pair")
	}
	if _, ok := labPreviewEvidenceCacheGetPair(pairs[0].Identity); !ok {
		t.Fatal("replacing LAB window B invalidated reusable primary A evidence")
	}
}

func TestLabFidelityPerceptualCancellationReleasesHeavySlot(t *testing.T) {
	plan := SamplingPlan{PlanIdentity: "cancel-plan", SelectedWindows: []SamplingWindow{{Identity: "primary", Primary: true, StartSeconds: 0, DurationSeconds: 5}}}
	labFidelityHeavySlot <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collectPerceptualEvidenceUnderHeavySlot(ctx, plan, models.Profile{}, perceptualArtifactInput{}); err != context.Canceled {
		t.Fatalf("canceled perceptual operation returned %v instead of context.Canceled", err)
	}
	<-labFidelityHeavySlot
	select {
	case labFidelityHeavySlot <- struct{}{}:
		<-labFidelityHeavySlot
	default:
		t.Fatal("canceled perceptual operation leaked the LAB heavy-work slot")
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
	result := labFidelityOperationResult(labFidelityOperationInput{MaterialSignature: "signature"}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, 2, false)
	if result["aggregateStatus"] != "analysis_in_progress" || result["canonicalStatus"] != "mismatch" {
		t.Fatalf("partial result masked canonical mismatch: %#v", result)
	}
	if result["overallVerdict"] != nil {
		t.Fatalf("partial result exposed a final overall verdict: %#v", result["overallVerdict"])
	}
	aggregate, ok := result["visualAggregate"].(models.JSONMap)
	if !ok || aggregate["comparableCount"] != 1 || aggregate["totalCount"] != 2 || aggregate["metricVerdict"] != "Safe" {
		t.Fatalf("partial visual aggregate was not backend-calculated: %#v", result["visualAggregate"])
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
	result := labFidelityOperationResult(labFidelityOperationInput{}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, 2, true)
	if result["canonicalStatus"] != "unverified" {
		t.Fatalf("later passing window downgraded canonical unverified result: %#v", result)
	}
	if result["overallVerdict"] != "Review" {
		t.Fatalf("canonical unverified result became Safe or Reject: %#v", result["overallVerdict"])
	}
}

func TestLabFidelityAggregatePreservesVisualThresholdsAndCanonicalPrecedence(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "aggregate-thresholds")
	windows := []models.JSONMap{
		{"position": 0.2, "metrics": models.JSONMap{"comparable": true, "ssim": 0.96, "psnr": 42}, "canonicalFidelity": models.JSONMap{"status": "passed"}},
		{"position": 0.8, "metrics": models.JSONMap{"comparable": true, "ssim": 0.94, "psnr": 38}, "canonicalFidelity": models.JSONMap{"status": "passed"}},
	}
	result := labFidelityOperationResult(labFidelityOperationInput{}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, 2, true)
	aggregate := result["visualAggregate"].(models.JSONMap)
	if aggregate["comparableCount"] != 2 || aggregate["totalCount"] != 2 || aggregate["metricVerdict"] != "Review" || result["overallVerdict"] != "Review" {
		t.Fatalf("visual threshold aggregate changed unexpectedly: %#v overall=%v", aggregate, result["overallVerdict"])
	}

	windows[1]["canonicalFidelity"] = models.JSONMap{"status": "mismatch"}
	result = labFidelityOperationResult(labFidelityOperationInput{}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, 2, true)
	if result["overallVerdict"] != "Reject" {
		t.Fatalf("canonical mismatch did not force Reject: %#v", result["overallVerdict"])
	}
}

func TestLabFidelityAggregateMissingVisualEvidenceCannotBecomeSafe(t *testing.T) {
	descriptor := testCompatiblePreviewDescriptor(t, "aggregate-missing")
	windows := []models.JSONMap{
		{"metrics": models.JSONMap{"comparable": true, "ssim": 0.99}, "canonicalFidelity": models.JSONMap{"status": "passed"}},
		{"metrics": models.JSONMap{"comparable": false}, "canonicalFidelity": models.JSONMap{"status": "passed"}},
	}
	result := labFidelityOperationResult(labFidelityOperationInput{}, descriptor, windows, SamplingPlan{WindowSeconds: 20}, 2, true)
	if result["overallVerdict"] != "Review" {
		t.Fatalf("missing visual evidence became Safe or Reject unexpectedly: %#v", result["overallVerdict"])
	}
}

func TestLabFidelityCancellationReleasesProtectionAndRetryReusesPrimary(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	installFakeFFmpeg(t, `printf primary-artifact`)
	descriptor := testCompatiblePreviewDescriptor(t, "cancel-retry-primary")
	descriptor.RetentionPolicy = compatiblePreviewEphemeralRetention
	directory := filepath.Dir(descriptor.CachePath)
	var generations atomic.Int32
	previous := compatiblePreviewGenerationStarted
	compatiblePreviewGenerationStarted = func(*compatiblePreviewArtifact) { generations.Add(1) }
	t.Cleanup(func() { compatiblePreviewGenerationStarted = previous })

	_, primaryLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || primaryLease == nil {
		t.Fatalf("primary preview was not generated: lease=%v err=%v", primaryLease, err)
	}
	primaryLease.release()

	_, fidelityLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || fidelityLease == nil {
		t.Fatalf("Fidelity could not protect primary artifact: lease=%v err=%v", fidelityLease, err)
	}
	_, playbackLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || playbackLease == nil {
		t.Fatalf("playback could not share primary artifact: lease=%v err=%v", playbackLease, err)
	}

	// Cancellation owns only the Fidelity consumer. The playback consumer keeps
	// the same ready artifact protected while the retry reuses it.
	fidelityLease.release()
	cleanupCompatiblePreviewEphemeralArtifacts(directory, time.Now())
	if _, err := os.Stat(descriptor.CachePath); err != nil {
		t.Fatalf("canceling Fidelity removed the ready primary artifact: %v", err)
	}
	hit, retryLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || !hit || retryLease == nil {
		t.Fatalf("Fidelity retry did not reuse the ready primary artifact: hit=%t lease=%v err=%v", hit, retryLease, err)
	}
	retryLease.release()
	if got := generations.Load(); got != 1 {
		t.Fatalf("Fidelity retry re-encoded the primary artifact: generation count=%d", got)
	}

	playbackLease.release()
	entry := compatiblePreviewArtifactEntryForTest(t, descriptor.ArtifactID)
	entry.mu.Lock()
	entry.scopeExpiresAt = time.Now().Add(-time.Second)
	entry.mu.Unlock()
	cleanupCompatiblePreviewEphemeralArtifacts(directory, time.Now())
	if _, err := os.Stat(descriptor.CachePath); !os.IsNotExist(err) {
		t.Fatalf("released artifact did not become eligible for normal cleanup: %v", err)
	}
}

func TestLabFidelityAdditionalInfrastructureFailureIsRetryableWithoutChangingPrimary(t *testing.T) {
	now := time.Now()
	store := labFidelityOperationStore{items: map[string]*LabFidelityOperation{}, cancels: map[string]context.CancelFunc{}}
	store.items["fidelity-failure"] = &LabFidelityOperation{
		ID: "fidelity-failure", Status: labFidelityRunning, Phase: "additional_window_metrics",
		PrimaryRequestID: "primary-request", PrimaryArtifactID: "primary-artifact", PrimaryGeneration: "generation:1",
		Result: models.JSONMap{"canonicalStatus": "passed", "aggregateStatus": "analysis_in_progress"}, CreatedAt: now, UpdatedAt: now,
	}
	store.update("fidelity-failure", func(operation *LabFidelityOperation) {
		operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, "additional_window_metrics", "metrics runner failed", true
	})
	operation, ok := store.copy("fidelity-failure")
	if !ok || operation.Status != labFidelityFailed || !operation.Retryable || operation.PrimaryArtifactID != "primary-artifact" {
		t.Fatalf("additional infrastructure failure did not remain retryable with the ready primary identity: %#v", operation)
	}
	if operation.Result["canonicalStatus"] != "passed" || operation.Result["aggregateStatus"] != "analysis_in_progress" {
		t.Fatalf("infrastructure failure was reported as a canonical mismatch/final aggregate: %#v", operation.Result)
	}
}

func TestLabFidelityProcessLocalRestartLosesOperationButReusesPrimaryArtifact(t *testing.T) {
	resetCompatiblePreviewArtifactsForTest()
	t.Cleanup(resetCompatiblePreviewArtifactsForTest)
	installFakeFFmpeg(t, `printf restart-primary`)
	descriptor := testCompatiblePreviewDescriptor(t, "restart-primary")
	var generations atomic.Int32
	previous := compatiblePreviewGenerationStarted
	compatiblePreviewGenerationStarted = func(*compatiblePreviewArtifact) { generations.Add(1) }
	t.Cleanup(func() { compatiblePreviewGenerationStarted = previous })
	_, primaryLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || primaryLease == nil {
		t.Fatalf("primary preview was not generated before restart simulation: lease=%v err=%v", primaryLease, err)
	}
	primaryLease.release()

	previousItems, previousCancels := labFidelityOperations.items, labFidelityOperations.cancels
	labFidelityOperations.Lock()
	labFidelityOperations.items = map[string]*LabFidelityOperation{
		"lost-operation": {ID: "lost-operation", Status: labFidelityRunning, PrimaryRequestID: "primary-request", PrimaryArtifactID: descriptor.ArtifactID},
	}
	labFidelityOperations.cancels = map[string]context.CancelFunc{"lost-operation": func() {}}
	labFidelityOperations.Unlock()
	t.Cleanup(func() {
		labFidelityOperations.Lock()
		labFidelityOperations.items, labFidelityOperations.cancels = previousItems, previousCancels
		labFidelityOperations.Unlock()
	})
	if _, ok := labFidelityOperations.copy("lost-operation"); !ok {
		t.Fatal("process-local Fidelity operation was not present before restart simulation")
	}
	labFidelityOperations.Lock()
	labFidelityOperations.items = map[string]*LabFidelityOperation{}
	labFidelityOperations.cancels = map[string]context.CancelFunc{}
	labFidelityOperations.Unlock()
	if _, ok := labFidelityOperations.copy("lost-operation"); ok {
		t.Fatal("process-local Fidelity operation survived the restart simulation")
	}
	hit, retryLease, err := acquireCompatiblePreviewArtifact(context.Background(), descriptor)
	if err != nil || !hit || retryLease == nil {
		t.Fatalf("restart retry did not reuse the recovered Compatible Preview artifact: hit=%t lease=%v err=%v", hit, retryLease, err)
	}
	retryLease.release()
	if got := generations.Load(); got != 1 {
		t.Fatalf("restart retry re-encoded the primary artifact: generation count=%d", got)
	}
}

func compatiblePreviewArtifactEntryForTest(t *testing.T, artifactID string) *compatiblePreviewArtifact {
	t.Helper()
	compatiblePreviewArtifacts.Lock()
	entry := compatiblePreviewArtifacts.items[artifactID]
	compatiblePreviewArtifacts.Unlock()
	if entry == nil {
		t.Fatalf("artifact %q was not registered", artifactID)
	}
	return entry
}
