## Context and Existing Behavior

`database.Migrate` invokes `backfillQueueLifecycleStages` at startup. The
backfill maps `completed` Queue status to `ready_to_publish` without inspecting
`QueueJob.StageHistory`, and updates `stage_updated_at` without appending a
transition event. `transitionJobStage` is the normal lifecycle writer and
persists `stage`, `stage_updated_at`, and `stage_history` together. Queue's
current-stage label reads `job.stage`, while the job details checklist renders
the recorded history. The checklist currently omits `validation_review`.

Validation owns immutable findings and outcomes. Publisher owns manual
publication; Auto Publisher independently applies automatic eligibility and
blocks actionable warnings. This change only reconciles Queue's stage
projection and its presentation.

## Decisions

### 1. History takes precedence only for a provable stale fallback projection

When history exists, validate every event as a recognized lifecycle stage with
a parseable RFC3339 timestamp and nondecreasing event time. A malformed or
out-of-order history is ambiguous and is left unchanged. An empty/missing
history is the only case eligible for status fallback.

The reconciliation candidate must be unpublished, nonterminal, and currently
equal to the legacy status-derived fallback. When its latest valid event
disagrees, the event is the durable stage authority. A current stage that is
not that fallback is preserved as a possible later transition. Published and
terminal rows are never rewritten.

### 2. Update the projection atomically and idempotently

Update only `stage` and `stage_updated_at`, setting the latter to the latest
persisted event time. Do not edit or append `stage_history`. The SQL update
compares the loaded stage, status, publication state, history, and stage update
time so a transition committed after the read wins. A repeat migration sees a
stage equal to the same latest event and performs no update.

### 3. Queue presents the persisted review state

Add `validation_review` to the lifecycle checklist between validation and
post-validation analysis. A visited review stage renders as a warning. The
existing stage label remains sourced from `job.stage`; no frontend-derived
publication readiness is added.

### 4. Keep adjacent authorities unchanged

The migration does not read or change ValidationRun/Result identities,
Publisher records, Auto Publisher settings, policy, scoring, or output files.
The existing automatic gate remains responsible for actionable warnings;
manual Publisher eligibility remains separate.

## Data and Migration

The startup reconciliation is repeatable and introduces no schema or history
rewrite. Legacy rows with no events retain the existing status-based stage
fallback. Existing valid histories are read-only evidence. Invalid or
ambiguous rows remain available for operator investigation and are not guessed
into a new lifecycle state.

## Verification

Before/after operational checks should use a read-only SQLite connection and
compare Queue stage with the last `stage_history` event for targeted job IDs.
For warning jobs, also inspect `validation_outcome`, actionable-warning state,
publication fields, and ensure no published timestamp/path was introduced.
No production/NAS write is part of this change.
