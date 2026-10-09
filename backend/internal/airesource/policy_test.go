package airesource

import (
	"testing"
	"time"
)

const testGiB int64 = 1 << 30

func policyFixture(now time.Time, total, available int64) (Settings, Request, Observation) {
	settings := DefaultSettings()
	settings.ModelEstimates = []ModelEstimate{{
		ProviderConfigurationID: "provider-a", ModelID: "model-a", ResidentBytes: 6 * testGiB,
		LoadPeakAdditionalBytes: 2 * testGiB, InferenceBaseBytes: testGiB,
		KVCacheBytesPerToken: 64 * 1024, DefaultContextTokens: 1024, MaximumContextTokens: 8192,
		Source: "benchmark", Revision: "nas-fixture-v1",
	}}
	request := Request{CapacityDomain: "nas-a", ProviderConfigurationID: "provider-a", ModelID: "model-a", ConsumerID: "consumer", RequestID: "request", Purpose: PurposeModelLoad}
	observation := Observation{
		CapacityDomain: "nas-a",
		Host:           HostObservation{AvailableMemoryKnown: true, TotalMemoryBytes: total, AvailableMemoryBytes: available, ObservedAt: now, Metrics: map[string]MetricObservation{}},
		Provider:       ProviderObservation{Available: true, ObservedAt: now, ResidentModels: []ResidentModel{}},
		Critical:       CriticalServiceObservation{Source: "nas-priority-guard", Status: GuardAllow, ObservedAt: now},
	}
	return settings, request, observation
}

func TestDefaultResourceProtectionPolicy(t *testing.T) {
	settings := DefaultSettings()
	if !settings.Enabled || settings.MaxResidentModels != 1 || settings.MaxConcurrentInferences != 1 || settings.MaxConcurrentModelLoads != 1 {
		t.Fatalf("unexpected default admission limits: %+v", settings)
	}
	if settings.MinimumAvailableMemoryBytes != 6*testGiB || settings.MaximumAdmissionWaitSeconds != 300 || settings.TelemetryMaximumAgeSeconds != 60 {
		t.Fatalf("unexpected default reserve or bounded wait: %+v", settings)
	}
}

func TestResourcePolicy24GiBHeadroomAndCriticalPressure(t *testing.T) {
	now := time.Now().UTC()
	settings, request, observation := policyFixture(now, 24*testGiB, 10*testGiB)
	decision := evaluate(settings, request, observation, nil, now)
	if decision.Status != Waiting || decision.Reason != "insufficient_memory_headroom" {
		t.Fatalf("24 GiB NAS should wait when 6 GiB reserve plus 8 GiB load estimate exceeds 10 GiB available: %+v", decision)
	}

	observation.Host.AvailableMemoryBytes = 16 * testGiB
	decision = evaluate(settings, request, observation, nil, now)
	if decision.Status != Admitted {
		t.Fatalf("expected sufficient 24 GiB NAS headroom to admit load, got %+v", decision)
	}

	observation.Critical.Status = GuardPressure
	decision = evaluate(settings, request, observation, nil, now)
	if decision.Status != Waiting || decision.Reason != "critical_service_pressure" {
		t.Fatalf("critical service pressure must wait: %+v", decision)
	}
	observation.Critical.Status = GuardUnavailable
	decision = evaluate(settings, request, observation, nil, now)
	if decision.Status != Unavailable || decision.Reason != "critical_service_guard_unavailable" {
		t.Fatalf("unknown critical-service state must fail closed: %+v", decision)
	}
}

func TestResourcePolicy24GiBUnknownStaleExternalResidentAndEstimate(t *testing.T) {
	now := time.Now().UTC()
	settings, request, observation := policyFixture(now, 24*testGiB, 18*testGiB)
	observation.Critical.Source = ""
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable {
		t.Fatalf("missing service guard must be unavailable, got %+v", got)
	}
	observation.Critical.Source = "nas-priority-guard"
	observation.Host.AvailableMemoryKnown = false
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable {
		t.Fatalf("unknown memory must be unavailable, got %+v", got)
	}
	observation.Host.AvailableMemoryKnown = true
	observation.Host.ObservedAt = now.Add(-2 * time.Minute)
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable {
		t.Fatalf("stale host memory must be unavailable: %+v", got)
	}
	observation.Host.ObservedAt = now
	observation.Provider.ObservedAt = now.Add(-2 * time.Minute)
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable {
		t.Fatalf("stale provider census must be unavailable, got %+v", got)
	}
	observation.Provider.ObservedAt = now
	settings.MaxResidentModels = 1
	observation.Provider.ResidentModels = []ResidentModel{{ProviderConfigurationID: "outside-provider", ModelID: "externally-loaded"}}
	if got := evaluate(settings, request, observation, nil, now); got.Status != Waiting || got.Reason != "resident_model_limit" {
		t.Fatalf("external resident must consume the shared resident limit: %+v", got)
	}
	settings.MaxResidentModels = 2
	settings.ModelEstimates = nil
	observation.Provider.ResidentModels = nil
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable || got.Reason != "model_estimate_missing" {
		t.Fatalf("missing estimate must not be treated as zero-cost: %+v", got)
	}
	settings, request, observation = policyFixture(now, 24*testGiB, 18*testGiB)
	request.Purpose = PurposeInference
	request.ContextTokens = 9000
	observation.Provider.ResidentModels = []ResidentModel{{ProviderConfigurationID: "provider-a", ModelID: "model-a"}}
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable || got.Reason != "model_estimate_incompatible" {
		t.Fatalf("estimate outside its context coverage must be unavailable: %+v", got)
	}
}

func TestResourcePolicy64GiBAllowsConfiguredSecondResidentWithHeadroom(t *testing.T) {
	now := time.Now().UTC()
	settings, request, observation := policyFixture(now, 64*testGiB, 28*testGiB)
	settings.MaxResidentModels = 2
	settings.ModelEstimates = append(settings.ModelEstimates, ModelEstimate{
		ProviderConfigurationID: "provider-b", ModelID: "model-b", ResidentBytes: 10 * testGiB,
		LoadPeakAdditionalBytes: 2 * testGiB, InferenceBaseBytes: testGiB,
		KVCacheBytesPerToken: 64 * 1024, DefaultContextTokens: 1024, MaximumContextTokens: 8192,
		Source: "benchmark", Revision: "64g-fixture-v1",
	})
	request.ProviderConfigurationID, request.ModelID = "provider-b", "model-b"
	observation.Provider.ResidentModels = []ResidentModel{{ProviderConfigurationID: "provider-a", ModelID: "already-resident"}}
	decision := evaluate(settings, request, observation, nil, now)
	if decision.Status != Admitted {
		t.Fatalf("configured two-resident policy should pass at 64 GiB with measured headroom: %+v", decision)
	}
	observation.Host.AvailableMemoryBytes = 17 * testGiB
	decision = evaluate(settings, request, observation, nil, now)
	if decision.Status != Waiting {
		t.Fatalf("second resident still requires load plus reserve headroom: %+v", decision)
	}
}

func TestResourcePolicyCountsSeparateLimitsAndCombinedReservationForecast(t *testing.T) {
	now := time.Now().UTC()
	settings, request, observation := policyFixture(now, 24*testGiB, 14*testGiB)
	observation.Provider.ResidentModels = []ResidentModel{{ProviderConfigurationID: "provider-a", ModelID: "model-a"}}
	request.Purpose = PurposeInference
	reservation := activeReservation{ProviderConfigurationID: "provider-a", ModelID: "model-a", Purpose: PurposeInference,
		EvaluatedHostObservedAt: now}
	if got := evaluate(settings, request, observation, []activeReservation{reservation}, now); got.Status != Waiting || got.Reason != "inference_limit" {
		t.Fatalf("separate inference cap must block a second inference: %+v", got)
	}
	request.Purpose = PurposeModelLoad
	request.ModelID = "model-b"
	settings.MaxResidentModels = 2
	settings.ModelEstimates = append(settings.ModelEstimates, ModelEstimate{
		ProviderConfigurationID: "provider-a", ModelID: "model-b", ResidentBytes: 6 * testGiB,
		LoadPeakAdditionalBytes: 2 * testGiB, InferenceBaseBytes: testGiB,
		KVCacheBytesPerToken: 64 * 1024, DefaultContextTokens: 1024, MaximumContextTokens: 8192,
		Source: "benchmark", Revision: "nas-fixture-v1",
	})
	decision := evaluate(settings, request, observation, []activeReservation{reservation}, now)
	if decision.Status != Waiting || decision.Reason != "insufficient_memory_headroom" {
		t.Fatalf("concurrent model load must include unobserved in-flight inference demand: %+v", decision)
	}
	observation.Host.ObservedAt = now.Add(time.Second)
	decision = evaluate(settings, request, observation, []activeReservation{reservation}, now)
	if decision.Status != Admitted {
		t.Fatalf("newer host sample already accounts for existing inference use: %+v", decision)
	}
}

func TestResourcePolicyRequiresConfiguredPressureMetricsAndModelLoadLimit(t *testing.T) {
	now := time.Now().UTC()
	settings, request, observation := policyFixture(now, 24*testGiB, 20*testGiB)
	settings.ModelEstimates = append(settings.ModelEstimates, ModelEstimate{
		ProviderConfigurationID: "provider-a", ModelID: "model-b", ResidentBytes: 6 * testGiB,
		LoadPeakAdditionalBytes: 2 * testGiB, InferenceBaseBytes: testGiB,
		KVCacheBytesPerToken: 64 * 1024, DefaultContextTokens: 1024, MaximumContextTokens: 8192,
		Source: "benchmark", Revision: "nas-fixture-v1",
	})
	settings.RequiredMetrics = []string{"swapPressurePercent"}
	if got := evaluate(settings, request, observation, nil, now); got.Status != Unavailable || got.Reason != "required_pressure_metric_unavailable" {
		t.Fatalf("configured but missing pressure metric must block admission: %+v", got)
	}
	settings.RequiredMetrics = nil
	reservation := activeReservation{ProviderConfigurationID: "provider-a", ModelID: "model-b", Purpose: PurposeModelLoad,
		EvaluatedHostObservedAt: now}
	if got := evaluate(settings, request, observation, []activeReservation{reservation}, now); got.Status != Waiting || got.Reason != "model_load_limit" {
		t.Fatalf("model-load leases need an independent concurrency limit: %+v", got)
	}
}

func TestResourceProtectionSettingsValidation(t *testing.T) {
	settings := DefaultSettings()
	settings.MaxResidentModels = 0
	if err := ValidateSettings(settings); err == nil {
		t.Fatal("zero resident limit passed validation")
	}
	settings = DefaultSettings()
	settings.PressureThresholds = map[string]float64{"unknownMetric": 50}
	if err := ValidateSettings(settings); err == nil {
		t.Fatal("unknown pressure metric passed validation")
	}
	settings = DefaultSettings()
	settings.ModelEstimates = []ModelEstimate{{ProviderConfigurationID: "provider", ModelID: "model", ResidentBytes: 1,
		LoadPeakAdditionalBytes: 0, InferenceBaseBytes: 0, KVCacheBytesPerToken: 1,
		DefaultContextTokens: 1, MaximumContextTokens: 1, Source: "guess", Revision: "r1"}}
	if err := ValidateSettings(settings); err == nil {
		t.Fatal("untraceable estimate source passed validation")
	}
}
