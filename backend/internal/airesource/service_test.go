package airesource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newServiceFixture(t *testing.T, settings Settings) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:airesource-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.AppSetting{}, &models.AIResourceAdmissionRecord{}, &models.AIResourceAdmissionCounter{}); err != nil {
		t.Fatal(err)
	}
	value := settingsValue(t, settings)
	if err := db.Create(&models.AppSetting{Key: SettingsKey, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, NewService(db)
}

func settingsValue(t *testing.T, settings Settings) models.JSONMap {
	t.Helper()
	value, err := NormalizeSettingsValue(mustJSONMap(t, settings))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustJSONMap(t *testing.T, value any) models.JSONMap {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result := models.JSONMap{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func serviceSettings() Settings {
	settings := DefaultSettings()
	settings.MaxResidentModels = 2
	settings.ModelEstimates = []ModelEstimate{
		{ProviderConfigurationID: "provider-a", ModelID: "model-a", ResidentBytes: 6 * testGiB,
			LoadPeakAdditionalBytes: 2 * testGiB, InferenceBaseBytes: testGiB, KVCacheBytesPerToken: 64 * 1024,
			DefaultContextTokens: 1024, MaximumContextTokens: 8192, Source: "benchmark", Revision: "service-test-v1"},
		{ProviderConfigurationID: "provider-a", ModelID: "model-b", ResidentBytes: 6 * testGiB,
			LoadPeakAdditionalBytes: 2 * testGiB, InferenceBaseBytes: testGiB, KVCacheBytesPerToken: 64 * 1024,
			DefaultContextTokens: 1024, MaximumContextTokens: 8192, Source: "benchmark", Revision: "service-test-v1"},
	}
	return settings
}

func serviceObservation(now time.Time, available int64, guard GuardStatus) Observation {
	return Observation{CapacityDomain: "nas-a",
		Host: HostObservation{AvailableMemoryKnown: true, TotalMemoryBytes: 24 * testGiB, AvailableMemoryBytes: available,
			ObservedAt: now, Metrics: map[string]MetricObservation{}},
		Provider: ProviderObservation{Available: true, ObservedAt: now,
			ResidentModels: []ResidentModel{{ProviderConfigurationID: "provider-a", ModelID: "model-a"}}},
		Critical: CriticalServiceObservation{Source: "priority-authority", Status: guard, ObservedAt: now},
	}
}

func serviceRequest(requestID string, purpose Purpose, modelID string) Request {
	return Request{CapacityDomain: "nas-a", ProviderConfigurationID: "provider-a", ModelID: modelID,
		ConsumerID: "test-consumer", RequestID: requestID, Purpose: purpose}
}

func TestServiceIdempotentLeaseAndRelease(t *testing.T) {
	db, service := newServiceFixture(t, serviceSettings())
	now := time.Now().UTC()
	if err := service.PublishObservation(context.Background(), serviceObservation(now, 20*testGiB, GuardAllow)); err != nil {
		t.Fatal(err)
	}
	request := serviceRequest("model-load-1", PurposeModelLoad, "model-b")
	first := service.Admit(context.Background(), request)
	if first.Status != Admitted || first.Lease == nil {
		t.Fatalf("expected model-load lease, got %+v", first)
	}
	second := service.Admit(context.Background(), request)
	if second.Status != Admitted || second.Lease == nil || second.Lease.id != first.Lease.id {
		t.Fatalf("duplicate request must reattach to its original logical lease: first=%+v second=%+v", first, second)
	}
	if err := service.Release(context.Background(), *first.Lease); err != nil {
		t.Fatal(err)
	}
	if err := service.Release(context.Background(), *first.Lease); err != nil {
		t.Fatalf("duplicate release must be idempotent: %v", err)
	}
	third := service.Admit(context.Background(), request)
	if third.Status != Denied || third.Reason != "lease_already_released" {
		t.Fatalf("released request identity must not create a second lease: %+v", third)
	}
	otherRequest := service.Admit(context.Background(), serviceRequest("model-load-2", PurposeModelLoad, "model-b"))
	if otherRequest.Status != Admitted || otherRequest.Lease == nil {
		t.Fatalf("expected separate request lease: %+v", otherRequest)
	}
	wrongOwner := Lease{id: otherRequest.Lease.id, requestKey: first.Lease.requestKey}
	if err := service.Release(context.Background(), wrongOwner); !errors.Is(err, ErrInvalidLease) {
		t.Fatalf("lease release must verify request ownership: %v", err)
	}
	var otherRecord models.AIResourceAdmissionRecord
	if err := db.Where("request_id = ?", "model-load-2").First(&otherRecord).Error; err != nil {
		t.Fatal(err)
	}
	if otherRecord.State != stateAdmitted {
		t.Fatalf("invalid release freed another request's capacity: %+v", otherRecord)
	}
	if err := service.Release(context.Background(), *otherRequest.Lease); err != nil {
		t.Fatal(err)
	}
	var records int64
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Count(&records).Error; err != nil {
		t.Fatal(err)
	}
	if records != 2 {
		t.Fatalf("expected two durable logical requests, found %d", records)
	}
}

func TestServiceFairWaitLanesCancellationAndGrantRace(t *testing.T) {
	db, service := newServiceFixture(t, serviceSettings())
	now := time.Now().UTC()
	if err := service.PublishObservation(context.Background(), serviceObservation(now, 20*testGiB, GuardPressure)); err != nil {
		t.Fatal(err)
	}
	load := service.Admit(context.Background(), serviceRequest("load-wait", PurposeModelLoad, "model-b"))
	inference := service.Admit(context.Background(), serviceRequest("inference-wait", PurposeInference, "model-a"))
	if load.Status != Waiting || load.Wait == nil || inference.Status != Waiting || inference.Wait == nil {
		t.Fatalf("both purpose lanes should durably wait on service pressure: load=%+v inference=%+v", load, inference)
	}
	if err := service.PublishObservation(context.Background(), serviceObservation(now.Add(time.Second), 14*testGiB+testGiB/2, GuardAllow)); err != nil {
		t.Fatal(err)
	}
	loadResult := service.Await(context.Background(), *load.Wait)
	if loadResult.Status != Admitted || loadResult.Lease == nil {
		t.Fatalf("model-load lane should receive the first fair turn: %+v", loadResult)
	}
	// The model-load forecast leaves too little headroom for inference. Releasing
	// it lets the other lane take the next turn.
	var loadRecord models.AIResourceAdmissionRecord
	if err := db.Where("request_id = ?", "load-wait").First(&loadRecord).Error; err != nil {
		t.Fatal(err)
	}
	if loadRecord.State != stateAdmitted {
		t.Fatalf("expected first lane grant to be journaled, got %+v", loadRecord)
	}
	var waitingRecord models.AIResourceAdmissionRecord
	if err := db.Where("request_id = ?", "inference-wait").First(&waitingRecord).Error; err != nil {
		t.Fatal(err)
	}
	if waitingRecord.State != stateWaiting {
		t.Fatalf("inference lane should retain FIFO wait while headroom is reserved, got %s", waitingRecord.State)
	}
	if err := service.Release(context.Background(), *loadResult.Lease); err != nil {
		t.Fatal(err)
	}
	inferenceResult := service.Await(context.Background(), *inference.Wait)
	if inferenceResult.Status != Admitted || inferenceResult.Lease == nil {
		t.Fatalf("eligible inference waiter should receive a lease: %+v", inferenceResult)
	}
	if cancelled := service.CancelWait(context.Background(), *inference.Wait); cancelled.Status != Admitted || cancelled.Lease == nil || cancelled.Lease.id != inferenceResult.Lease.id {
		t.Fatalf("grant/cancel race must return the granted lease for caller release: %+v", cancelled)
	}
	secondInference := service.Admit(context.Background(), serviceRequest("inference-cancel", PurposeInference, "model-a"))
	if secondInference.Status != Waiting || secondInference.Wait == nil {
		t.Fatalf("inference limit should queue the second request: %+v", secondInference)
	}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if cancelled := service.Await(cancelledContext, *secondInference.Wait); cancelled.Status != Denied || cancelled.Reason != "wait_cancelled" {
		t.Fatalf("context cancellation should remove a waiting ticket: %+v", cancelled)
	}
}

func TestServiceRestartReattachesLeaseAndBlocksWithoutFreshObservations(t *testing.T) {
	db, service := newServiceFixture(t, serviceSettings())
	now := time.Now().UTC()
	if err := service.PublishObservation(context.Background(), serviceObservation(now, 20*testGiB, GuardAllow)); err != nil {
		t.Fatal(err)
	}
	request := serviceRequest("infer-restart", PurposeInference, "model-a")
	granted := service.Admit(context.Background(), request)
	if granted.Status != Admitted || granted.Lease == nil {
		t.Fatalf("expected first inference lease: %+v", granted)
	}
	restarted := NewService(db)
	recovered := restarted.Admit(context.Background(), request)
	if recovered.Status != Admitted || recovered.Lease == nil || recovered.Lease.id != granted.Lease.id {
		t.Fatalf("restart retry must reattach to persisted lease: %+v", recovered)
	}
	newRequest := serviceRequest("infer-after-restart", PurposeInference, "model-a")
	unavailable := restarted.Admit(context.Background(), newRequest)
	if unavailable.Status != Unavailable || unavailable.Reason != "resource_observation_unavailable" {
		t.Fatalf("new grants must wait for fresh post-restart observations: %+v", unavailable)
	}
	if err := restarted.PublishObservation(context.Background(), serviceObservation(now.Add(time.Second), 20*testGiB, GuardAllow)); err != nil {
		t.Fatal(err)
	}
	blocked := restarted.Admit(context.Background(), serviceRequest("inference-blocked-after-restart", PurposeInference, "model-a"))
	if blocked.Status != Waiting || blocked.Wait == nil || blocked.Reason != "inference_limit" {
		t.Fatalf("active inference lease must survive restart reconciliation: %+v", blocked)
	}
	if err := restarted.Release(context.Background(), *recovered.Lease); err != nil {
		t.Fatal(err)
	}
	if next := restarted.Await(context.Background(), *blocked.Wait); next.Status != Admitted || next.Lease == nil {
		t.Fatalf("release should free the recovered lease for its waiter: %+v", next)
	} else if err := restarted.Release(context.Background(), *next.Lease); err != nil {
		t.Fatal(err)
	}
	if err := service.PublishObservation(context.Background(), serviceObservation(now.Add(2*time.Second), 20*testGiB, GuardPressure)); err != nil {
		t.Fatal(err)
	}
	waitRequest := serviceRequest("wait-restart", PurposeModelLoad, "model-b")
	waiting := service.Admit(context.Background(), waitRequest)
	if waiting.Status != Waiting || waiting.Wait == nil {
		t.Fatalf("expected persisted wait ticket: %+v", waiting)
	}
	recoveredWait := restarted.Admit(context.Background(), waitRequest)
	if recoveredWait.Status != Waiting || recoveredWait.Wait == nil || recoveredWait.Wait.id != waiting.Wait.id {
		t.Fatalf("restart retry must reattach to the durable wait ticket: %+v", recoveredWait)
	}
}

func TestServiceRetainsUncertainLeaseThroughGraceThenExpiresIt(t *testing.T) {
	db, service := newServiceFixture(t, serviceSettings())
	now := time.Now().UTC()
	if err := service.PublishObservation(context.Background(), serviceObservation(now, 14*testGiB+testGiB/2, GuardAllow)); err != nil {
		t.Fatal(err)
	}
	inference := service.Admit(context.Background(), serviceRequest("inference-expiry", PurposeInference, "model-a"))
	if inference.Status != Admitted || inference.Lease == nil {
		t.Fatalf("expected active inference reservation: %+v", inference)
	}
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Where("request_id = ?", "inference-expiry").Update("operation_deadline", now.Add(-31*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	load := service.Admit(context.Background(), serviceRequest("load-after-expiry", PurposeModelLoad, "model-b"))
	if load.Status != Admitted || load.Lease == nil {
		t.Fatalf("lease past its deadline plus cleanup grace should no longer reserve capacity: %+v", load)
	}
	var expired models.AIResourceAdmissionRecord
	if err := db.Where("request_id = ?", "inference-expiry").First(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if expired.State != stateExpired {
		t.Fatalf("expected durable expired lease state, got %s", expired.State)
	}
}

func TestServicePreviewDoesNotJournalOrAcquire(t *testing.T) {
	db, service := newServiceFixture(t, serviceSettings())
	now := time.Now().UTC()
	if err := service.PublishObservation(context.Background(), serviceObservation(now, 20*testGiB, GuardAllow)); err != nil {
		t.Fatal(err)
	}
	decision := service.Preview(context.Background(), serviceRequest("preview-only", PurposeModelLoad, "model-b"))
	if decision.Status != Admitted || decision.Lease != nil || decision.Wait != nil {
		t.Fatalf("preview should return only a dry-run decision: %+v", decision)
	}
	var records, outcomes int64
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Count(&records).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AIResourceAdmissionCounter{}).Count(&outcomes).Error; err != nil {
		t.Fatal(err)
	}
	if records != 0 || outcomes != 0 {
		t.Fatalf("preview wrote admission state: records=%d outcomes=%d", records, outcomes)
	}
}

func TestUnavailableRequestsAreIdempotentAndAggregateBoundedReasons(t *testing.T) {
	_, service := newServiceFixture(t, serviceSettings())
	request := serviceRequest("no-observation-1", PurposeModelLoad, "model-b")
	first := service.Admit(context.Background(), request)
	retry := service.Admit(context.Background(), request)
	if first.Status != Unavailable || retry.Status != Unavailable || first.Reason != retry.Reason {
		t.Fatalf("unknown authorities must remain an idempotent unavailable result: first=%+v retry=%+v", first, retry)
	}
	second := service.Admit(context.Background(), serviceRequest("no-observation-2", PurposeModelLoad, "model-b"))
	if second.Status != Unavailable {
		t.Fatalf("second unavailable request changed status: %+v", second)
	}
	counters, err := service.Outcomes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counters.Unavailable != 2 {
		t.Fatalf("expected two unavailable outcomes, got %+v", counters)
	}
	reasons, err := service.OutcomeReasons(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 1 || reasons[0].Reason != "resource_observation_unavailable" || reasons[0].Count != 2 {
		t.Fatalf("unexpected bounded reason aggregation: %+v", reasons)
	}
}

func TestServiceWaitExpiresWithoutPolling(t *testing.T) {
	db, service := newServiceFixture(t, serviceSettings())
	now := time.Now().UTC()
	if err := service.PublishObservation(context.Background(), serviceObservation(now, 20*testGiB, GuardPressure)); err != nil {
		t.Fatal(err)
	}
	waiting := service.Admit(context.Background(), serviceRequest("expire-wait", PurposeModelLoad, "model-b"))
	if waiting.Status != Waiting || waiting.Wait == nil {
		t.Fatalf("expected bounded wait ticket: %+v", waiting)
	}
	if err := db.Model(&models.AIResourceAdmissionRecord{}).Where("wait_id = ?", waiting.Wait.id).Update("wait_deadline", now.Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	result := service.Await(context.Background(), *waiting.Wait)
	if result.Status != Denied || result.Reason != "admission_wait_expired" {
		t.Fatalf("expired wait should resolve as denied: %+v", result)
	}
}

func TestRuntimeStateHostProjectionPreservesUnknownMemory(t *testing.T) {
	now := time.Now().UTC()
	state := models.RuntimeState{DetectedAt: now, TotalMemoryBytes: 24 * testGiB, AvailableMemoryBytes: 0, CPUCores: 4, CPULoad1: 2}
	projected := HostObservationFromRuntimeState(state)
	if projected.AvailableMemoryKnown {
		t.Fatal("zero available memory must remain unknown for fail-closed admission")
	}
	if projected.Metrics["cpuLoadPerCore"].Value != 50 {
		t.Fatalf("unexpected CPU metric projection: %+v", projected.Metrics)
	}
}

func TestRuntimeStateHostSourceReadsCanonicalMutableState(t *testing.T) {
	db, _ := newServiceFixture(t, serviceSettings())
	if err := db.AutoMigrate(&models.RuntimeState{}); err != nil {
		t.Fatal(err)
	}
	state := models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now().UTC(),
		TotalMemoryBytes: 24 * testGiB, AvailableMemoryBytes: 12 * testGiB, CPUCores: 4, CPULoad1: 1,
		StableFingerprint: "fingerprint", FingerprintVersion: "v1"}
	if err := db.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	projected, err := NewRuntimeStateHostSource(db).CurrentHostObservation(context.Background(), "nas-a")
	if err != nil {
		t.Fatal(err)
	}
	if !projected.AvailableMemoryKnown || projected.AvailableMemoryBytes != 12*testGiB || projected.TotalMemoryBytes != 24*testGiB {
		t.Fatalf("runtime source did not project the canonical memory values: %+v", projected)
	}
}
