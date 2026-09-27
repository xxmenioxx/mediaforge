package models

import "time"

// ValidationRun is immutable decision evidence for one attempt against one
// frozen Queue execution and the canonical kind=result artifact identity.
type ValidationRun struct {
	ID                       uint      `json:"id" gorm:"primaryKey"`
	RunID                    string    `json:"runId" gorm:"not null;uniqueIndex"`
	JobID                    uint      `json:"jobId" gorm:"not null;index"`
	ExecutionNumber          uint      `json:"executionNumber" gorm:"not null;index"`
	ExecutionPlanID          uint      `json:"executionPlanId" gorm:"not null;index"`
	ExecutionPlanVersion     int       `json:"executionPlanVersion" gorm:"not null"`
	ExecutionPlanFingerprint string    `json:"executionPlanFingerprint" gorm:"not null"`
	ResultArtifactID         string    `json:"resultArtifactId" gorm:"not null;index"`
	ResultGenerationToken    string    `json:"resultGenerationToken" gorm:"not null;index"`
	ResultGenerationVersion  int       `json:"resultGenerationVersion" gorm:"not null"`
	ResultOutputRevision     string    `json:"resultOutputRevision" gorm:"not null;index"`
	SchemaVersion            string    `json:"schemaVersion" gorm:"not null"`
	ValidatorVersion         string    `json:"validatorVersion" gorm:"not null"`
	RulesetID                string    `json:"rulesetId" gorm:"not null"`
	RulesetVersion           string    `json:"rulesetVersion" gorm:"not null"`
	ActorSource              string    `json:"actorSource" gorm:"not null"`
	ActorID                  string    `json:"actorId" gorm:"not null"`
	Outcome                  string    `json:"outcome" gorm:"not null;index"`
	Score                    int       `json:"score" gorm:"not null"`
	ActionableWarnings       bool      `json:"actionableWarnings" gorm:"not null;default:false;index"`
	Findings                 JSONList  `json:"findings" gorm:"type:json;not null"`
	Report                   JSONMap   `json:"report" gorm:"type:json"`
	CreatedAt                time.Time `json:"createdAt" gorm:"not null;index"`
}

// ValidationOverride is an append-only operator/system handoff event. The
// server assigns ActorSource and ActorID; clients provide only the reason and
// requested rule scope.
type ValidationOverride struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	EventID          string    `json:"eventId" gorm:"not null;uniqueIndex"`
	ValidationRunID  string    `json:"validationRunId" gorm:"not null;index"`
	ActorSource      string    `json:"actorSource" gorm:"not null"`
	ActorID          string    `json:"actorId" gorm:"not null"`
	Reason           string    `json:"reason" gorm:"not null"`
	RuleKeys         JSONList  `json:"ruleKeys" gorm:"type:json;not null"`
	RequestedHandoff string    `json:"requestedHandoff" gorm:"not null"`
	OverrideVersion  string    `json:"overrideVersion" gorm:"not null"`
	CreatedAt        time.Time `json:"createdAt" gorm:"not null;index"`
}
