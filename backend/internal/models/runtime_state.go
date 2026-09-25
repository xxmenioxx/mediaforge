package models

import "time"

const RuntimeStateSingletonKey = "host"

// RuntimeState is the mutable, singleton view of the most recently detected
// host. Its ID is a technical row identity and is never an audit snapshot ID.
// RuntimeSnapshot remains the immutable evidence model.
type RuntimeState struct {
	ID                    uint      `json:"id" gorm:"primaryKey"`
	SingletonKey          string    `json:"-" gorm:"not null;uniqueIndex:idx_runtime_state_singleton"`
	DetectedAt            time.Time `json:"detectedAt" gorm:"not null;index"`
	OS                    string    `json:"os"`
	Architecture          string    `json:"architecture"`
	Container             bool      `json:"container"`
	CPUCores              int       `json:"cpuCores"`
	CPULoad1              float64   `json:"cpuLoad1"`
	GPUDetected           bool      `json:"gpuDetected"`
	GPUUsageAvailable     bool      `json:"gpuUsageAvailable"`
	GPUUsagePercent       float64   `json:"gpuUsagePercent"`
	GPUMediaUsagePercent  float64   `json:"gpuMediaUsagePercent"`
	GPURenderUsagePercent float64   `json:"gpuRenderUsagePercent"`
	GPUMetricSource       string    `json:"gpuMetricSource"`
	TotalMemoryBytes      int64     `json:"totalMemoryBytes"`
	AvailableMemoryBytes  int64     `json:"availableMemoryBytes"`
	BatteryPresent        bool      `json:"batteryPresent"`
	BatteryPercent        int       `json:"batteryPercent"`
	PowerSource           string    `json:"powerSource"`
	OnBattery             bool      `json:"onBattery"`
	Disks                 JSONMap   `json:"disks" gorm:"type:json"`
	Encoders              JSONMap   `json:"encoders" gorm:"type:json"`
	MetricCapabilities    JSONMap   `json:"metricCapabilities" gorm:"type:json"`
	RecommendedProfile    string    `json:"recommendedProfile"`
	SelectedProfile       string    `json:"selectedProfile"`
	PreferredProfile      string    `json:"preferredProfile"`
	FallbackProfile       string    `json:"fallbackProfile"`
	AppliedOverrides      JSONList  `json:"appliedOverrides" gorm:"type:json"`
	EffectivePolicy       JSONMap   `json:"effectivePolicy" gorm:"type:json"`
	SelectionReasons      JSONList  `json:"selectionReasons" gorm:"type:json"`
	Warnings              JSONList  `json:"warnings" gorm:"type:json"`
	StableFingerprint     string    `json:"stableFingerprint" gorm:"not null;index"`
	FingerprintVersion    string    `json:"fingerprintVersion" gorm:"not null"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`

	// These fields are response-only metadata. They are intentionally not
	// persisted and make live-state/audit provenance distinct at the API edge.
	StateKind       string `json:"stateKind,omitempty" gorm:"-"`
	AuditSnapshotID *uint  `json:"auditSnapshotId,omitempty" gorm:"-"`
}
