## Context

The proposal defines the user-visible pacing contract. The current
implementation stores worker configuration in the `workers` AppSetting JSON
value. `WorkerHandler.workerLimits()` loads the existing worker fields from
that object, while `SettingsPage` maps the same object into its form, saves it
through the existing settings mutation, and exposes it through advanced JSON.

The current claim path resolves the requested or default worker name, updates
the worker heartbeat, serializes claims with the process-level `claimJobMu`,
and runs capacity checks, queue selection, reservation activation, and the
queued-to-running transition in one database transaction. The transition
persists `QueueJob.Status`, `Progress`, `WorkerName`, and `StartedAt`. The
current claim guards include concurrency, global time delay, and batch
cooldown; queue selection separately evaluates workspace, working hours, and
scheduler resources before reservation activation. Queue jobs already retain
`Progress`, `WorkerName`, `StartedAt`, `FinishedAt`, and `Status`, and the
existing terminal-status helper recognizes completed, failed, and canceled
jobs.

## Goals / Non-Goals

**Goals:**

- Add a backward-compatible time/progress pacing contract to the existing
  workers setting.
- Make progress pacing worker-scoped, restart-safe, deterministic, and atomic
  with the existing claim and reservation transition.
- Keep all existing capacity, scheduler, reservation, queue, working-hours,
  execution-plan, and batch protections authoritative.
- Make Settings controls and advanced JSON represent the same canonical object.
- Keep implementation and tests focused on the existing worker claim and
  Settings paths.

**Non-Goals:**

- Recompute, reinterpret, or redesign Queue/FFmpeg progress semantics.
- Add a pacing table, in-memory progress tracker, polling daemon, dispatcher,
  or frontend scheduler.
- Change max-concurrency semantics, worker heartbeats, scheduler resource
  policy, reservation architecture, or execution-plan waiting behavior.
- Add adaptive recommendations, per-profile or per-job thresholds, or Queue UI
  redesign.

## Decisions

### 1. Extend the existing workers object with explicit defaults

Use these canonical fields in the existing `workers` AppSetting value:

- `jobStartPacingMode`: `time` or `progress`.
- `startNextJobAtProgressPercent`: integer `1..100`, default `75` when absent.

The backend worker-limits value should carry the parsed mode and threshold so
claim policy receives one resolved settings snapshot. Missing mode resolves to
`time`, which preserves current installations without a data migration. The
threshold default is harmless in time mode because the time branch does not
read it. Existing delay, batch, capacity, and other fields remain in the same
object.

The existing settings update handler should validate the `workers` object when
the new fields are present, using its established bad-request convention. It
must accept legacy payloads that omit either new field and must not silently
delete inactive values. A new persistence model or migration would create
unnecessary compatibility and rollback risk, so it is rejected in favor of
load-time defaults.

### 2. Put the mode-specific gate inside the existing claim boundary

Keep `claimNextJob()` as the only claim entrypoint. After the worker name is
resolved to the same canonical name used for heartbeat and assignment, pass
that name and the resolved worker limits into the existing claim-guard path.
Evaluate the mode-specific start gate inside the transaction protected by
`claimJobMu`, alongside the existing capacity and batch checks. Do not create a
second dispatcher or a separate synchronization mechanism.

The existing responsibility split remains:

1. Existing capacity semantics run first and continue to enforce the current
   `maxConcurrentJobs` / worker-availability behavior.
2. Time mode uses the current delay check unchanged.
3. Progress mode uses the worker-scoped persisted reference check instead of
   the delay check.
4. Existing batch cooldown remains an independent guard.
5. `nextClaimableJob()` continues to enforce queue ordering and evaluate
   workspace, working-hours, and scheduler resource decisions.
6. Reservation activation and the running transition remain in the same
   transaction.

This ordering keeps a progress gate from changing scheduler or execution-plan
state. A progress block returns before queue selection can mutate a plan, and
an opened progress gate still has to pass every later guard.

### 3. Select exactly one persisted worker-scoped reference

In progress mode, select one Queue job with all of the following properties:

- `worker_name` equals the resolved worker name;
- `started_at` is not null;
- ordered by `started_at DESC, id DESC`;
- no fallback to another job after the selected reference is terminal.

No row means the first claim is allowed. If the selected row is terminal,
reuse the existing `terminalJobStatus` semantics and allow the pacing gate. If
it is not terminal, compare its persisted `Progress` to the inclusive
threshold. Progress below the threshold returns the new internal pacing
result; progress equal to or above it permits the claim to continue.

The reference query deliberately does not search for any job over the
threshold. Once a later job starts, its persisted `StartedAt`, `WorkerName`,
and initial progress of `1` make it the new reference immediately. The ID
tie-breaker makes equal timestamps deterministic, including in tests and fast
claims.

### 4. Preserve atomic handoff and transient error semantics

Add a distinct internal error, conceptually
`errWorkerProgressPacingActive`, and map it in the existing `ClaimNext` HTTP
handling to the same transient status class used for delay pacing with a
progress-specific message. Keep it distinct from capacity, delay, batch, and
no-runnable-job outcomes for direct backend tests and useful diagnostics.

The existing process mutex and transaction are sufficient for the required
in-process race: the first claimant persists the newly running job before the
second claimant can evaluate the reference query. The second attempt then
sees the newly started job, rather than the older threshold-crossing job, and
is blocked while the new reference remains below threshold. Reservation
activation and capacity counting remain part of the same atomic section.

### 5. Keep Settings UI and advanced JSON on one path

Extend the existing `SettingsForm.workers`, initial defaults,
`settingsToForm()`, and advanced-settings reconstruction with both pacing
fields. Add a local Workers-card mode selector and conditional controls:

- Time delay mode renders/enables the existing delay field as authoritative.
- Job progress mode renders/enables the percentage threshold as authoritative,
  with a visible `%` semantic.
- Switching modes changes authority without clearing the inactive value.

`saveCoreSettings()` continues sending the complete `workers` value through
the existing update endpoint. Advanced JSON continues parsing into the same
canonical form before save, so it can carry both fields without a second API
or a frontend-only policy decision. Settings UI tests should exercise the
normal form and advanced JSON round trips rather than broad page snapshots.

### 6. Focus regressions on real claim and settings seams

Extend `backend/internal/handlers/worker_concurrency_test.go` with disposable
SQLite fixtures that seed persisted Queue jobs, worker settings, runnable
plans, reservations, and progress values. Keep unrelated global capacity and
resource settings permissive when testing worker scoping, then add separate
tests where capacity, scheduler, working-hours, and batch cooldown deliberately
block a claim.

Add focused frontend Settings coverage in a dedicated worker-pacing test or the
nearest existing Settings test location. Assert load defaults, mode switching,
conditional authority, validation boundaries, canonical workers payloads, and
inactive-value preservation; do not add broad snapshots.

## Risks / Trade-offs

- [Risk] A reference query that omits the worker filter could let another
  worker's progress release a claim. → Mitigate with an explicit canonical
  worker-name predicate and worker-a/worker-b regression coverage.
- [Risk] Equal timestamps could produce nondeterministic handoff behavior. →
  Mitigate with the required descending Queue job ID tie-breaker and a focused
  test.
- [Risk] A terminal latest reference could permanently block if code falls
  through to its progress value or an older reference. → Mitigate by checking
  canonical terminal status before progress and testing failed, canceled, and
  completed states without fallback.
- [Risk] A second concurrent claimant could leapfrog the newly started job. →
  Mitigate by keeping evaluation, reservation activation, and persistence under
  the existing mutex and transaction, with a two-claim race regression.
- [Risk] Progress mode could accidentally disable a hard scheduler or batch
  guard. → Mitigate by preserving the current guard order and asserting each
  guard independently.
- [Risk] A generic workers settings update could overwrite fields not shown by
  a new UI control. → Mitigate by retaining both values in the form and
  advanced JSON, accepting omitted legacy fields with defaults, and testing
  round trips.
- [Risk] Persisted progress can lag the actual encoder state. → This is an
  intentional correctness boundary: QueueJob progress is the authoritative
  durable signal and avoids restart-dependent in-memory behavior.

## Migration Plan

1. Deploy the backend and frontend changes without a database migration.
2. Resolve missing mode as `time` and missing threshold as `75` at load/form
   boundaries; leave existing delay and batch values untouched.
3. Existing installations therefore continue using time pacing until an
   operator selects progress mode and saves the workers settings.
4. To roll back, remove the new code and leave the extra JSON fields in place;
   the existing implementation ignores unknown worker fields and continues
   using the stored delay value. No Queue data or media is modified by the
   pacing change.
