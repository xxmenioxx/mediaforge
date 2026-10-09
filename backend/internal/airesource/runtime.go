package airesource

import (
	"context"
	"fmt"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

type RuntimeStateHostSource struct{ db *gorm.DB }

var _ HostObservationSource = RuntimeStateHostSource{}

func NewRuntimeStateHostSource(db *gorm.DB) RuntimeStateHostSource {
	return RuntimeStateHostSource{db: db}
}

// CurrentHostObservation reads the existing RuntimeState singleton. It never
// runs detection or creates an audit snapshot, and callers decide when to
// publish the returned sample into the admission service.
func (source RuntimeStateHostSource) CurrentHostObservation(ctx context.Context, _ string) (HostObservation, error) {
	if source.db == nil {
		return HostObservation{}, fmt.Errorf("runtime state database is required")
	}
	state, err := runtimeinfo.Current(source.db.WithContext(ctx))
	if err != nil {
		return HostObservation{}, err
	}
	return HostObservationFromRuntimeState(state), nil
}

// HostObservationFromRuntimeState projects only measurements owned by the
// canonical mutable host view. Unavailable values remain unavailable.
func HostObservationFromRuntimeState(state models.RuntimeState) HostObservation {
	observedAt := state.DetectedAt
	host := HostObservation{
		TotalMemoryBytes: state.TotalMemoryBytes, AvailableMemoryBytes: state.AvailableMemoryBytes,
		ObservedAt: observedAt, Metrics: map[string]MetricObservation{},
	}
	host.AvailableMemoryKnown = state.TotalMemoryBytes > 0 && state.AvailableMemoryBytes > 0 && state.AvailableMemoryBytes <= state.TotalMemoryBytes
	if state.CPUCores > 0 && state.CPULoad1 >= 0 && !observedAt.IsZero() {
		host.Metrics["cpuLoadPerCore"] = MetricObservation{
			Available: true, Value: state.CPULoad1 / float64(state.CPUCores) * 100, ObservedAt: observedAt,
		}
	}
	if state.GPUUsageAvailable && !observedAt.IsZero() {
		host.Metrics["gpuUsagePercent"] = MetricObservation{Available: true, Value: state.GPUUsagePercent, ObservedAt: observedAt}
	}
	return host
}

func observationIsFresh(observation Observation, now time.Time, maximumAge time.Duration) bool {
	return fresh(observation.Host.ObservedAt, now, maximumAge) &&
		fresh(observation.Provider.ObservedAt, now, maximumAge) &&
		fresh(observation.Critical.ObservedAt, now, maximumAge)
}
