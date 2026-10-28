## Purpose

Defines safe Queue cancellation, lifecycle availability, and removal of terminal canceled Queue history by the exact path group shown in Queue.

## ADDED Requirements

### Requirement: Cancellation waits for running work to stop

Canceling a queued job SHALL make it terminal Canceled and release its reservation atomically. Canceling a running job SHALL retain its running ownership and reservation while cancellation is requested, and SHALL become terminal Canceled only after the worker reports its terminal result. Late worker callbacks SHALL NOT restore a dismissed or terminal canceled job to active work.

#### Scenario: Queued job is canceled before claim
- **WHEN** an operator cancels a queued job that has not started
- **THEN** the job becomes Canceled and its Scheduler reservation is released in the same authoritative transaction
- **AND** its RAW asset can become Available when no other lifecycle blocker exists, without removing the Queue record

#### Scenario: Running job is stopping
- **WHEN** an operator cancels a running job
- **THEN** the job remains active with an observable cancellation request until the worker acknowledges termination
- **AND** its reservation prevents a new claim or duplicate execution during that interval

#### Scenario: Worker acknowledges cancellation
- **WHEN** the worker reports a terminal canceled result after its process exits
- **THEN** the Queue record becomes terminal Canceled, the reservation is released, and the RAW asset can become Available when eligible

#### Scenario: Late callback follows terminal cancellation
- **WHEN** a late worker callback attempts to return a terminal canceled job to queued or running work
- **THEN** the terminal cancellation remains authoritative and the job is not claimed again

#### Scenario: Cancel does not dismiss history
- **WHEN** a job becomes Canceled
- **THEN** its Queue history, logs, reports, and execution provenance remain available until an explicit Remove action

### Requirement: Preview and remove canceled jobs for one exact Queue path group

Queue SHALL preview and dismiss only eligible terminal canceled jobs whose exact normalized parent-path group matches the selected Queue path group. The operation SHALL span batches, leave every other status and validation-owned or published job untouched, and SHALL NOT remove filesystem content.

#### Scenario: Preview a mixed-status path group
- **WHEN** the operator previews a path containing canceled and non-canceled jobs across batches
- **THEN** the preview reports selected, dismissible, and skipped counts with skip reasons and stable job identities
- **AND** only terminal canceled, non-validation-owned, unpublished records are dismissible

#### Scenario: Confirm selected canceled jobs
- **WHEN** the operator confirms a preview with all eligible IDs or a selected subset
- **THEN** one bounded backend operation dismisses only IDs that still belong to that exact path group and remain eligible
- **AND** queued, stopping, running, completed, failed, published, validation-owned, and out-of-group records remain unchanged

#### Scenario: Similar or symlinked paths are distinct
- **WHEN** another Queue record has a path-prefix lookalike, same filename in another directory, or a different symlink path
- **THEN** it is not included unless its normalized Queue path-group identity exactly matches the selected group

#### Scenario: Retry after partial or successful dismissal
- **WHEN** the operator retries the same path dismissal after a conflict or a successful request
- **THEN** the response reports dismissed, already-dismissed, skipped, and conflicted counts without mutating an ineligible or changed record

#### Scenario: No eligible canceled jobs exist
- **WHEN** preview finds no eligible canceled records
- **THEN** Queue does not offer a confirmable removal action

### Requirement: Dismissal returns independently of filesystem cleanup

Queue dismissal SHALL commit history visibility changes and reservation reconciliation without waiting for filesystem I/O. Existing guarded cancellation cleanup SHALL remain separate from dismissal, and any cleanup status or error exposed by Queue SHALL be truthful and independently observable.

#### Scenario: Dismiss a large canceled selection
- **WHEN** the operator dismisses eligible canceled jobs while a cleanup operation is slow
- **THEN** the Queue response returns after its bounded database transaction without waiting for that filesystem operation

#### Scenario: Cleanup fails
- **WHEN** an authorized canceled-job cleanup operation fails
- **THEN** its failure remains visible through the cleanup operation status and does not roll back or falsely report the Queue dismissal

#### Scenario: Existing batch removal
- **WHEN** the operator removes a canceled batch
- **THEN** the existing all-canceled eligibility rule and history-preservation behavior remain in force, and the response does not run filesystem cleanup synchronously

### Requirement: Queue and Assets reflect cancellation transitions

Queue SHALL show cancellation requested/stopping separately from terminal Canceled. Queue and Assets SHALL refresh once per cancellation or bulk-dismissal action so lifecycle projections reflect terminal state without per-job catalog rescans.

#### Scenario: Cancellation is still stopping
- **WHEN** the worker has not acknowledged a running job's cancellation
- **THEN** Queue presents a stopping state and Assets continues to show its active operation rather than Available

#### Scenario: Cancellation is terminal
- **WHEN** queued cancellation commits or a running worker acknowledges cancellation
- **THEN** Queue reflects the terminal state and Assets reflects Available only when the existing lifecycle rules find no other blocker

#### Scenario: Path dismissal completes
- **WHEN** path dismissal succeeds or partially conflicts
- **THEN** Queue and Assets queries are invalidated once and the UI reports backend dismissed/already-dismissed/skipped/conflicted counts
