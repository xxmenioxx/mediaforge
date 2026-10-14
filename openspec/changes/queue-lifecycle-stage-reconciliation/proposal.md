## Purpose

Queue can display a completed job as `ready_to_publish` even when its persisted
lifecycle history ends at `validation_review`. The current startup backfill
derives the stage from Queue status and ignores history, so Queue's current
stage label can disagree with its persisted checklist and imply readiness that
was never recorded.

## What Changes

- Reconcile a provably stale status-derived Queue stage projection from the
  latest valid, ordered `stageHistory` event.
- Use status fallback only for legacy jobs with no lifecycle history.
- Leave malformed, out-of-order, published, terminal, or otherwise ambiguous
  records unchanged and protect concurrent transitions with a conditional
  database update.
- Keep `stageHistory` immutable during reconciliation; do not synthesize an
  event or alter Validation, Publisher, Auto Publisher, or artifact evidence.
- Show `validation_review` in Queue's persisted lifecycle checklist as a
  warning stage so it agrees with the current stage label.

## Capabilities

### New Capabilities

- `queue-lifecycle-stage-reconciliation`: Keep Queue's current stage projection
  consistent with durable lifecycle history while preserving the publication
  and Validation authorities.

## Non-goals

- Changing Validation Quality Gate rules, scoring, immutable runs/results,
  override decisions, or Auto Publisher eligibility.
- Publishing, skipping, overriding, deploying, or writing to a NAS database.
- Reconstructing invalid history or changing Publisher recovery and delivery.
