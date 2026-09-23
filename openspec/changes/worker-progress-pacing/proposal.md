## Why

MVForge currently spaces worker claims with a fixed time delay. That is
predictable, but it can leave constrained NAS hardware underused when a long
job has already made enough progress to safely permit controlled overlap, and
it does not adapt to short or long media. Issue #22 adds an opt-in,
worker-scoped progress gate while preserving the existing delay behavior for
all legacy installations.

## What Changes

- Extend the existing `workers` AppSetting object with a pacing mode,
  `time` or `progress`, and a validated `1..100` progress threshold.
- Default a missing pacing mode to `time`; retain existing delay-based claim
  behavior and all existing settings for current installations.
- In progress mode, evaluate the most recently started Queue job assigned to
  the resolved worker, ordered by `started_at DESC` and `id DESC`, using its
  persisted `Progress` value.
- Allow the first claim for a worker when no prior started job exists; allow a
  claim after the latest worker-scoped reference reaches the threshold or
  enters an existing terminal status, without falling back to an older job.
- Keep progress pacing inside the existing atomic claim boundary so a newly
  claimed job immediately becomes the next pacing reference and concurrent
  claim attempts cannot leapfrog it.
- Keep concurrency, scheduler/resource, reservation, workspace,
  working-hours, execution-plan, queue-order, and batch cooldown checks
  authoritative in either pacing mode.
- Return a distinct transient progress-pacing claim result consistent with
  the existing HTTP treatment for delay and worker-limit gates, without
  mutating a queued execution plan into a waiting state for this gate.
- Add local Workers settings controls for selecting time delay or job progress,
  showing only the active mode's authoritative control while preserving the
  inactive value and the canonical advanced JSON object.
- Add focused backend and frontend regressions for defaults, validation,
  threshold boundaries, reference handoff and terminal behavior, worker
  isolation, atomic races, hard guards, and settings persistence.

## Capabilities

### New Capabilities

- `worker-progress-pacing`: Configure and enforce time or persisted-progress
  worker claim pacing, including settings compatibility, worker-scoped
  reference selection, claim gating, and Settings UI behavior.

### Modified Capabilities

None.

## Impact

- Backend worker settings parsing and claim eligibility in
  `backend/internal/handlers/workers.go`, plus focused regressions in
  `backend/internal/handlers/worker_concurrency_test.go`.
- Existing `workers` AppSetting update validation in
  `backend/internal/handlers/settings.go`; no new persistence model, table,
  dispatcher, polling daemon, or migration is required.
- QueueJob's existing persisted `Progress`, `WorkerName`, `StartedAt`,
  `FinishedAt`, and `Status` fields remain the source of truth.
- Workers configuration and advanced JSON handling in
  `frontend/src/pages/SettingsPage.tsx`, with focused Settings coverage.
- Runtime behavior remains compatible with current delay pacing by default and
  is intended for controlled overlap on resource-constrained NAS hardware.

Out of scope are FFmpeg progress calculation, Queue progress semantics,
scheduler resource policy, reservation architecture, worker heartbeats,
`maxConcurrentJobs` semantics, dynamic or per-profile/per-job thresholds,
adaptive recommendations, a new dispatcher, and Queue UI redesign.
