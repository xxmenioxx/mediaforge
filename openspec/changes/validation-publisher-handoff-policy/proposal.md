## Why

Manual and automatic Validation currently make different Ready to Publish decisions, and the automatic gate treats an actionable advisory warning as a blocker. The handoff also ignores some stage persistence failures, so Publisher can run without a durable `ready_to_publish` event while the Queue checklist presents completed conversion as completed publication.

## What Changes

- Define one outcome-and-finding handoff contract for manual and automatic Validation. Advisory-only `PASS_WITH_WARNINGS` may reach `ready_to_publish`; `Actionable` describes follow-up work and does not override `Blocking` disposition.
- Keep automatic release under explicit policy: `validation.autoAdvanceOutcomes` must include `PASS_WITH_WARNINGS`, and Auto Publisher must be enabled. Missing policy continues to mean `PASS` only. Do not backfill or re-stage historical warning jobs during deployment.
- Preserve blocking behavior for `FAIL`, `REVIEW_REQUIRED`, unresolved blocking findings, critical missing evidence, stale or invalid Result identity, required sidecars, replacement authority, and destination failures. Preserve immutable ValidationRuns/Results, exact identity, and scoped override provenance.
- Persist and propagate Queue stage transition errors through `validating`, `ready_to_publish`, and `publishing`; make retries and restart recovery operate from durable stage/publication history.
- Keep status-only worker/Queue completion callbacks at Validation review until canonical Validation authorizes Ready; repeated completion updates must preserve the current handoff stage.
- Make the Queue checklist represent persisted lifecycle history and show conversion completion separately from publication completion.
- Add regression coverage for the transition matrix, manual/automatic parity, policy changes, retries, concurrent handoffs, persistence errors, and restart recovery.

Out of scope: changing Validation rule definitions or scoring, changing Publisher filesystem safeguards or destination semantics, modifying media/NAS state, adding a deployment migration that releases historical warning jobs, and the separate Validation UX redesign in #338.

## Capabilities

### New Capabilities

- `validation-publisher-handoff`: canonical Validation handoff, automatic authorization, durable Queue stages, and truthful conversion/publication checklist behavior.

### Modified Capabilities

None. The repository has no existing OpenSpec capability for Validation or Publisher handoff.

## Impact

- Backend: `backend/internal/handlers/worker_processes.go`, `workers.go`, `queue.go`, `validation.go`, `validation_quality_gate.go`, `auto_publisher.go`, `publisher_delivery.go`, `job_lifecycle.go`, and focused handler tests.
- Frontend: `frontend/src/components/JobDetailsDialog.tsx` and its focused tests.
- Persistence: retain current `autoAdvanceOutcomes` values and the existing append-only Validation/Publisher records; no schema migration or historical stage rewrite is planned.
- OpenSpec artifacts: retain the implemented handoff contract and transition matrix alongside the code change for review.
