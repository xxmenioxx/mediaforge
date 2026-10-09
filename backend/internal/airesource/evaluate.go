package airesource

import (
	"fmt"
	"math"
	"time"
)

type activeReservation struct {
	ProviderConfigurationID string
	ModelID                 string
	Purpose                 Purpose
	ContextTokens           int64
	Sequences               int64
	EvaluatedHostObservedAt time.Time
}

func evaluate(settings Settings, request Request, observation Observation, reservations []activeReservation, now time.Time) Decision {
	if !settings.Enabled {
		return Decision{Status: Denied, Reason: "protection_disabled"}
	}
	if observation.CapacityDomain != request.CapacityDomain {
		return Decision{Status: Unavailable, Reason: "capacity_domain_mismatch"}
	}
	maxAge := time.Duration(settings.TelemetryMaximumAgeSeconds) * time.Second
	if !fresh(observation.Host.ObservedAt, now, maxAge) || !observation.Host.AvailableMemoryKnown || observation.Host.TotalMemoryBytes <= 0 || observation.Host.AvailableMemoryBytes < 0 || observation.Host.AvailableMemoryBytes > observation.Host.TotalMemoryBytes {
		return Decision{Status: Unavailable, Reason: "host_memory_unavailable"}
	}
	if !observation.Provider.Available || !fresh(observation.Provider.ObservedAt, now, maxAge) {
		return Decision{Status: Unavailable, Reason: "provider_residency_unavailable"}
	}
	if observation.Critical.Source == "" || !fresh(observation.Critical.ObservedAt, now, maxAge) {
		return Decision{Status: Unavailable, Reason: "critical_service_guard_unavailable"}
	}
	criticalPressure := false
	switch observation.Critical.Status {
	case GuardAllow:
	case GuardPressure:
		criticalPressure = true
	default:
		return Decision{Status: Unavailable, Reason: "critical_service_guard_unavailable"}
	}

	profile, ok := modelEstimate(settings, request.ProviderConfigurationID, request.ModelID)
	if !ok {
		return Decision{Status: Unavailable, Reason: "model_estimate_missing"}
	}
	residentKeys := make(map[string]struct{}, len(observation.Provider.ResidentModels))
	for _, resident := range observation.Provider.ResidentModels {
		if resident.ProviderConfigurationID == "" || resident.ModelID == "" {
			return Decision{Status: Unavailable, Reason: "provider_residency_invalid"}
		}
		residentKeys[modelKey(resident.ProviderConfigurationID, resident.ModelID)] = struct{}{}
	}
	requestedModelKey := modelKey(request.ProviderConfigurationID, request.ModelID)
	_, isResident := residentKeys[requestedModelKey]
	if request.Purpose == PurposeInference && request.ContextTokens > profile.MaximumContextTokens {
		return Decision{Status: Unavailable, Reason: "model_estimate_incompatible"}
	}
	metricDecision := evaluateMetrics(settings, observation.Host, now, maxAge)
	if metricDecision.Status != "" {
		return metricDecision
	}

	activeInferences := 0
	activeLoads := 0
	pendingResidentKeys := make(map[string]struct{})
	for _, reservation := range reservations {
		key := modelKey(reservation.ProviderConfigurationID, reservation.ModelID)
		switch reservation.Purpose {
		case PurposeInference:
			activeInferences++
		case PurposeModelLoad:
			activeLoads++
			if _, exists := residentKeys[key]; !exists {
				pendingResidentKeys[key] = struct{}{}
			}
		}
	}

	var demand int64
	switch request.Purpose {
	case PurposeModelLoad:
		if isResident {
			demand = profile.LoadPeakAdditionalBytes
		} else {
			if _, alreadyLoading := pendingResidentKeys[requestedModelKey]; alreadyLoading {
				return Decision{Status: Waiting, Reason: "model_load_in_progress"}
			}
			if activeLoads >= settings.MaxConcurrentModelLoads {
				return Decision{Status: Waiting, Reason: "model_load_limit"}
			}
			if len(residentKeys)+len(pendingResidentKeys) >= settings.MaxResidentModels {
				return Decision{Status: Waiting, Reason: "resident_model_limit"}
			}
			if profile.ResidentBytes > math.MaxInt64-profile.LoadPeakAdditionalBytes {
				return Decision{Status: Denied, Reason: "model_estimate_overflow"}
			}
			demand = profile.ResidentBytes + profile.LoadPeakAdditionalBytes
		}
	case PurposeInference:
		if !isResident {
			return Decision{Status: Denied, Reason: "model_not_resident"}
		}
		if activeInferences >= settings.MaxConcurrentInferences {
			return Decision{Status: Waiting, Reason: "inference_limit"}
		}
		contextTokens := request.ContextTokens
		if contextTokens == 0 {
			contextTokens = profile.DefaultContextTokens
		}
		sequences := request.Sequences
		if sequences == 0 {
			sequences = 1
		}
		if contextTokens > profile.MaximumContextTokens {
			return Decision{Status: Unavailable, Reason: "model_estimate_incompatible"}
		}
		if contextTokens < 1 || sequences < 1 || sequences > 256 {
			return Decision{Status: Denied, Reason: "inference_profile_mismatch"}
		}
		if contextTokens > math.MaxInt64/sequences || contextTokens*sequences > math.MaxInt64/profile.KVCacheBytesPerToken {
			return Decision{Status: Denied, Reason: "inference_estimate_overflow"}
		}
		kvBytes := contextTokens * sequences * profile.KVCacheBytesPerToken
		if profile.InferenceBaseBytes > math.MaxInt64-kvBytes {
			return Decision{Status: Denied, Reason: "inference_estimate_overflow"}
		}
		demand = profile.InferenceBaseBytes + kvBytes
	default:
		return Decision{Status: Denied, Reason: "invalid_purpose"}
	}
	if criticalPressure {
		return Decision{Status: Waiting, Reason: "critical_service_pressure"}
	}

	var projectedReservations int64
	for _, reservation := range reservations {
		// A newer host sample already includes work admitted against the older
		// sample. Only reserve forecasts not yet reflected in observed memory.
		if observation.Host.ObservedAt.After(reservation.EvaluatedHostObservedAt) {
			continue
		}
		reservedBytes, known := reservationDemand(settings, observation, reservation)
		if !known {
			return Decision{Status: Unavailable, Reason: "active_reservation_estimate_missing"}
		}
		if reservedBytes > math.MaxInt64-projectedReservations {
			return Decision{Status: Denied, Reason: "active_reservation_estimate_overflow"}
		}
		projectedReservations += reservedBytes
	}
	if demand > math.MaxInt64-projectedReservations {
		return Decision{Status: Denied, Reason: "combined_estimate_overflow"}
	}
	demand += projectedReservations

	if settings.MinimumAvailableMemoryBytes > observation.Host.TotalMemoryBytes || demand > observation.Host.TotalMemoryBytes-settings.MinimumAvailableMemoryBytes {
		return Decision{Status: Denied, Reason: "model_estimate_exceeds_capacity"}
	}
	if observation.Host.AvailableMemoryBytes < settings.MinimumAvailableMemoryBytes || demand > observation.Host.AvailableMemoryBytes-settings.MinimumAvailableMemoryBytes {
		return Decision{Status: Waiting, Reason: "insufficient_memory_headroom"}
	}
	return Decision{Status: Admitted, Reason: "capacity_available"}
}

func reservationDemand(settings Settings, observation Observation, reservation activeReservation) (int64, bool) {
	profile, ok := modelEstimate(settings, reservation.ProviderConfigurationID, reservation.ModelID)
	if !ok {
		return 0, false
	}
	if reservation.Purpose == PurposeModelLoad {
		for _, resident := range observation.Provider.ResidentModels {
			if modelKey(resident.ProviderConfigurationID, resident.ModelID) == modelKey(reservation.ProviderConfigurationID, reservation.ModelID) {
				return profile.LoadPeakAdditionalBytes, true
			}
		}
		if profile.ResidentBytes > math.MaxInt64-profile.LoadPeakAdditionalBytes {
			return 0, false
		}
		return profile.ResidentBytes + profile.LoadPeakAdditionalBytes, true
	}
	contextTokens := reservation.ContextTokens
	if contextTokens == 0 {
		contextTokens = profile.DefaultContextTokens
	}
	sequences := reservation.Sequences
	if sequences == 0 {
		sequences = 1
	}
	if contextTokens < 1 || contextTokens > profile.MaximumContextTokens || sequences < 1 || sequences > 256 ||
		contextTokens > math.MaxInt64/sequences || contextTokens*sequences > math.MaxInt64/profile.KVCacheBytesPerToken {
		return 0, false
	}
	kvBytes := contextTokens * sequences * profile.KVCacheBytesPerToken
	if profile.InferenceBaseBytes > math.MaxInt64-kvBytes {
		return 0, false
	}
	return profile.InferenceBaseBytes + kvBytes, true
}

func evaluateMetrics(settings Settings, host HostObservation, now time.Time, maxAge time.Duration) Decision {
	for _, metricName := range settings.RequiredMetrics {
		metric, exists := host.Metrics[metricName]
		if !exists || !metric.Available || !fresh(metric.ObservedAt, now, maxAge) || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) || metric.Value < 0 {
			return Decision{Status: Unavailable, Reason: "required_pressure_metric_unavailable"}
		}
	}
	for metricName, threshold := range settings.PressureThresholds {
		metric, exists := host.Metrics[metricName]
		if !exists || !metric.Available || !fresh(metric.ObservedAt, now, maxAge) || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) || metric.Value < 0 {
			return Decision{Status: Unavailable, Reason: "configured_pressure_metric_unavailable"}
		}
		if metric.Value > threshold {
			return Decision{Status: Waiting, Reason: fmt.Sprintf("pressure_%s", metricName)}
		}
	}
	return Decision{}
}

func fresh(observedAt, now time.Time, maxAge time.Duration) bool {
	if observedAt.IsZero() || observedAt.After(now.Add(5*time.Second)) {
		return false
	}
	return now.Sub(observedAt) <= maxAge
}

func modelKey(providerConfigurationID, modelID string) string {
	return providerConfigurationID + "\x00" + modelID
}
