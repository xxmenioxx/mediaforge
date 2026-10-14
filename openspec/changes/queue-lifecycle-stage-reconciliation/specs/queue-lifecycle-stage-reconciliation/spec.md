## Purpose

Define safe reconciliation and presentation of Queue lifecycle stages from
persisted `stageHistory` while retaining the existing Validation and Publisher
authorities.

## ADDED Requirements

### Requirement: Queue stage reconciliation preserves durable lifecycle history

At startup, Queue lifecycle backfill SHALL use the latest valid, chronologically
ordered `stageHistory` event when the current stage is a status-derived fallback
projection and differs from that event. It SHALL update only the stage
projection and its timestamp, SHALL set the timestamp from the persisted event,
and SHALL NOT create, remove, or rewrite lifecycle history. Status fallback
SHALL be used only when a legacy record has no lifecycle events.

#### Scenario: Completed warning run remains in Validation review
- **WHEN** an unpublished completed Queue job has `stage=ready_to_publish` only
  as the status-derived fallback and its latest valid event is
  `validation_review`
- **THEN** reconciliation sets the current stage and timestamp from that event,
  preserves the event history and Validation warning evidence, and performs no
  publication or automatic handoff

#### Scenario: Clean PASS history is already ready
- **WHEN** an unpublished completed job's current stage and latest valid event
  are both `ready_to_publish`
- **THEN** reconciliation leaves the row unchanged

#### Scenario: Legacy job has no lifecycle events
- **WHEN** an unpublished legacy job has no `stageHistory` events
- **THEN** reconciliation uses the existing status-to-stage fallback

### Requirement: Ambiguous, terminal, and newer state is protected

Reconciliation SHALL leave published or terminal rows unchanged. It SHALL
leave rows with malformed or out-of-order history unchanged and SHALL NOT use
status fallback for them. A current stage that does not match the status-derived
fallback SHALL be preserved as a possible later transition. A conditional
update SHALL ensure that a concurrent Queue transition committed after the
backfill read is not overwritten.

#### Scenario: Invalid or out-of-order history is retained
- **WHEN** a nonempty history contains an invalid event or timestamps are
  out of order
- **THEN** reconciliation leaves the stage, timestamp, and history unchanged

#### Scenario: Newer lifecycle stage is preserved
- **WHEN** a completed job has a current stage beyond the status fallback and
  its last recorded event is older
- **THEN** reconciliation preserves the current stage and timestamp

#### Scenario: Job is published or terminal
- **WHEN** a job has publication evidence or a terminal lifecycle state
- **THEN** reconciliation does not modify its stage projection

#### Scenario: Queue transition races reconciliation
- **WHEN** Queue commits a stage transition after reconciliation reads the row
- **THEN** the conditional update affects no row and the newer transition and
  history remain intact

### Requirement: Queue label and lifecycle checklist agree on review state

Queue SHALL continue to show its current-stage label from the persisted stage.
The lifecycle checklist SHALL include `validation_review` and render a visited
review stage with warning semantics, using the same persisted history. This
presentation SHALL NOT imply Publisher readiness or change publication
eligibility.

#### Scenario: Validation review is shown consistently
- **WHEN** the latest persisted stage is `validation_review`
- **THEN** Queue shows “validation review” as the current stage and the
  checklist shows that recorded stage as a warning

#### Scenario: Existing publication gates remain authoritative
- **WHEN** an actionable warning is present on a Validation run
- **THEN** Auto Publisher continues to block automatic publication and manual
  Publisher eligibility remains evaluated by its existing authority
