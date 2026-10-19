## Purpose

Defines safe individual Queue review when automatic cadence resolution needs operator input, while keeping the accepted Queue job, ExecutionPlan, and downstream Validation bound to one frozen effective plan.

## ADDED Requirements

### Requirement: Individual Queue checks unresolved automatic IVTC before job creation

The system SHALL resolve an individual Queue request through the canonical cadence and HEVC planning authorities before creating a QueueJob. If automatic IVTC is recommended but neither an explicit nor validated field order is available, the system MUST return a structured `REVIEW_REQUIRED` result that identifies the missing field-order evidence and offers asset-scoped TFF/BFF choices. It MUST NOT create a QueueJob, ExecutionPlan, Queue reservation, or Worker failure for that request.

#### Scenario: Historical #2191 evidence has ambiguous IVTC order

- **WHEN** a single Queue request uses `hard_telecine` at 30000/1001, cadence mode `auto`, field order `auto`, stream metadata reports `tt`, and IVTC validation has only 1 of 3 windows with no validated `SelectedOrder`
- **THEN** Queue returns `REVIEW_REQUIRED` before job creation, reports that an explicit or validated field order is missing, and suggests choosing TFF or BFF for the asset

#### Scenario: Queue intent has no persisted source scan

- **WHEN** a direct Queue request has no saved scan or cadence evidence
- **THEN** Queue may persist its lightweight intent without inventing cadence or HEVC evidence, and claim remains responsible for canonical preparation and blocking an unresolved plan before ExecutionPlan dispatch

#### Scenario: Automatic IVTC has a validated selected order

- **WHEN** the canonical cadence analysis recommends IVTC and provides a validated `SelectedOrder`
- **THEN** Queue resolves the same canonical IVTC filters, effective output FPS, and HEVC decision as Batch Preflight without requiring an unsupported inference from stream metadata

### Requirement: Cadence order overrides are scoped to one candidate asset

An explicit TFF/BFF choice made during single-asset review SHALL apply only to that candidate asset. It MUST NOT change a reusable profile, global setting, or unrelated asset. The candidate SHALL be resolved again after the choice, and its effective plan SHALL be shown before acceptance.

#### Scenario: Operator selects TFF for one candidate asset

- **WHEN** an operator chooses TFF for one asset whose automatic IVTC order is unresolved
- **THEN** only that asset candidate is resolved with TFF, the resulting filters, FPS, and HEVC decision are displayed, and no QueueJob is created until the operator accepts the displayed plan

#### Scenario: Operator selects BFF for one candidate asset

- **WHEN** an operator chooses BFF for one asset whose automatic IVTC order is unresolved
- **THEN** only that asset candidate is resolved with BFF and its effective plan is displayed for review before acceptance

#### Scenario: Operator chooses Preserve cadence

- **WHEN** an operator changes the candidate cadence decision to Preserve
- **THEN** the existing cadence/HEVC authorities resolve the source-rate plan; it is ready only when effective FPS and HEVC Level/Tier are valid, and remains review-required when FPS is unknown
- **AND** the UI displays the preserved source cadence rate without adding an IVTC filter chain

### Requirement: Accepted individual Queue work preserves candidate identity and plan

Acceptance SHALL revalidate source identity, source revision, candidate revision, and active Queue reservations atomically. The created QueueJob and ExecutionPlan MUST use the exact accepted cadence, FPS, profile, and HEVC snapshot; Worker and Validation MUST continue to consume that frozen plan. Existing historical QueueJobs and ExecutionPlans MUST remain unchanged.

#### Scenario: Candidate identity changes before acceptance

- **WHEN** the source identity or candidate revision changes after review but before acceptance
- **THEN** acceptance is rejected and no QueueJob is created from the stale candidate

#### Scenario: Accepted candidate reaches Worker and Validation

- **WHEN** an operator accepts a ready single-asset candidate
- **THEN** the QueueJob and ExecutionPlan retain the reviewed effective cadence/FPS/HEVC snapshot and Validation evaluates that same execution identity

#### Scenario: Existing Queue history remains immutable

- **WHEN** the single-asset preflight behavior is introduced
- **THEN** existing QueueJob snapshots, ExecutionPlans, and the historical Job #2191 are not rewritten
