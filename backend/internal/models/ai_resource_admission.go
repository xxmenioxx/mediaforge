package models

import "time"

// AIResourceAdmissionRecord stores only opaque request accounting needed to
// reattach waits and leases after a backend restart.
type AIResourceAdmissionRecord struct {
	ID                      uint      `json:"-" gorm:"primaryKey"`
	RequestKey              string    `json:"-" gorm:"not null;uniqueIndex"`
	LeaseID                 string    `json:"-" gorm:"index"`
	WaitID                  string    `json:"-" gorm:"index"`
	CapacityDomain          string    `json:"-" gorm:"not null;index:idx_ai_resource_domain_state"`
	ProviderConfigurationID string    `json:"-" gorm:"not null"`
	ModelID                 string    `json:"-" gorm:"not null"`
	ConsumerID              string    `json:"-" gorm:"not null"`
	WorkflowID              string    `json:"-" gorm:"not null;default:''"`
	RequestID               string    `json:"-" gorm:"not null"`
	Purpose                 string    `json:"-" gorm:"not null"`
	State                   string    `json:"-" gorm:"not null;index:idx_ai_resource_domain_state"`
	ReasonCode              string    `json:"-" gorm:"not null;default:''"`
	ContextTokens           int64     `json:"-" gorm:"not null;default:0"`
	Sequences               int64     `json:"-" gorm:"not null;default:0"`
	OperationDeadline       time.Time `json:"-" gorm:"not null"`
	EvaluatedHostObservedAt time.Time `json:"-"`
	WaitDeadline            time.Time `json:"-"`
	CreatedAt               time.Time `json:"-"`
	UpdatedAt               time.Time `json:"-"`
}

// AIResourceAdmissionCounter aggregates bounded outcome/reason codes and does
// not retain provider payloads or consumer diagnostics.
type AIResourceAdmissionCounter struct {
	Key       string    `json:"-" gorm:"primaryKey"`
	Outcome   string    `json:"-" gorm:"not null"`
	Reason    string    `json:"-" gorm:"not null"`
	Count     int64     `json:"-" gorm:"not null;default:0"`
	UpdatedAt time.Time `json:"-"`
}
