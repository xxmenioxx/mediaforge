package airesource

import (
	"context"
	"time"
)

type DecisionStatus string

const (
	Admitted    DecisionStatus = "ADMITTED"
	Waiting     DecisionStatus = "WAITING"
	Denied      DecisionStatus = "DENIED"
	Unavailable DecisionStatus = "UNAVAILABLE"
)

type Purpose string

const (
	PurposeModelLoad Purpose = "MODEL_LOAD"
	PurposeInference Purpose = "INFERENCE"
)

type GuardStatus string

const (
	GuardAllow       GuardStatus = "allow"
	GuardPressure    GuardStatus = "pressure"
	GuardUnavailable GuardStatus = "unavailable"
)

type Request struct {
	CapacityDomain          string    `json:"capacityDomain"`
	ProviderConfigurationID string    `json:"providerConfigurationId"`
	ModelID                 string    `json:"modelId"`
	ConsumerID              string    `json:"consumerId"`
	WorkflowID              string    `json:"workflowId"` // Stable across explicit retries for the same workflow.
	RequestID               string    `json:"requestId"`  // Unique to one admission attempt; keep WorkflowID stable when retrying.
	Purpose                 Purpose   `json:"purpose"`
	ContextTokens           int64     `json:"contextTokens,omitempty"`
	Sequences               int64     `json:"sequences,omitempty"`
	OperationDeadline       time.Time `json:"operationDeadline,omitempty"`
}

type Lease struct {
	id         string
	requestKey string
}

type WaitHandle struct {
	id         string
	requestKey string
}

type Decision struct {
	Status     DecisionStatus
	Reason     string
	Lease      *Lease
	Wait       *WaitHandle
	RetryAfter time.Duration
}

type MetricObservation struct {
	Available  bool
	Value      float64
	ObservedAt time.Time
}

type HostObservation struct {
	SourceIdentity       string
	AvailableMemoryKnown bool
	TotalMemoryBytes     int64
	AvailableMemoryBytes int64
	ObservedAt           time.Time
	Metrics              map[string]MetricObservation
}

type ResidentModel struct {
	ProviderConfigurationID string
	ModelID                 string
}

type ProviderObservation struct {
	SourceIdentity string
	Available      bool
	ObservedAt     time.Time
	ResidentModels []ResidentModel
}

type CriticalServiceObservation struct {
	Source     string
	Status     GuardStatus
	Reason     string
	ObservedAt time.Time
}

// Observation is a normalized, provider-neutral capacity sample. Adapters
// publish it; the admission package never polls a provider or host itself.
type Observation struct {
	CapacityDomain      string
	Host                HostObservation
	Provider            ProviderObservation
	Critical            CriticalServiceObservation
	authenticatedSource bool
}

// TrustedObservationSource supplies one source-authenticated snapshot for the
// requested shared capacity domain. Implementations must not infer missing
// signals or contact a provider as part of reading the snapshot.
type TrustedObservationSource interface {
	CurrentObservation(context.Context, string) (Observation, error)
}

type ObservationPublisher interface {
	PublishObservation(context.Context, Observation) error
}

// These narrow source ports let the owning runtime, provider, and service
// priority authorities supply observations without transferring ownership.
type HostObservationSource interface {
	CurrentHostObservation(context.Context, string) (HostObservation, error)
}

type ProviderResidencySource interface {
	CurrentProviderObservation(context.Context, string) (ProviderObservation, error)
}

type CriticalServiceGuard interface {
	CurrentCriticalServiceObservation(context.Context, string) (CriticalServiceObservation, error)
}

type Admission interface {
	Admit(context.Context, Request) Decision
	Preview(context.Context, Request) Decision
	Await(context.Context, WaitHandle) Decision
	CancelWait(context.Context, WaitHandle) Decision
	Release(context.Context, Lease) error
}

type Counters struct {
	Admitted    int64 `json:"admitted"`
	Waiting     int64 `json:"waiting"`
	Denied      int64 `json:"denied"`
	Unavailable int64 `json:"unavailable"`
}

type AuthorityStatus struct {
	HostMemory           string `json:"hostMemory"`
	ProviderResidency    string `json:"providerResidency"`
	CriticalServiceGuard string `json:"criticalServiceGuard"`
}

type OutcomeReason struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
	Count   int64  `json:"count"`
}
