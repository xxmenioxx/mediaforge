## Purpose

Worker progress pacing lets MVForge permit controlled overlap on constrained
NAS hardware using persisted Queue progress, while retaining the existing
time-delay behavior and all authoritative claim safeguards.

## ADDED Requirements

### Requirement: Compatible worker pacing settings

The existing `workers` AppSetting value MUST remain the persistence boundary
for worker start pacing. It MUST support the canonical fields
`jobStartPacingMode`, whose value is `time` or `progress`, and
`startNextJobAtProgressPercent`, whose value is an integer from 1 through 100.
Existing worker fields MUST remain in the same object; no pacing-specific
table or second settings endpoint is introduced.

When `jobStartPacingMode` is absent, the effective mode MUST be `time`. When
the threshold is absent, the effective threshold MUST be 75. A missing mode
or threshold MUST NOT require destructive migration of existing installations.

#### Scenario: Legacy worker settings default to time pacing

- **WHEN** an existing `workers` value has no `jobStartPacingMode` and has its
  existing delay setting
- **THEN** the effective mode is `time` and the existing delay gates the next
  claim exactly as it did before this capability

#### Scenario: Existing worker fields remain compatible

- **WHEN** a worker settings value is loaded or saved with pacing fields
- **THEN** `defaultWorkerName`, `autoWorkerEnabled`, `maxConcurrentJobs`,
  `maxJobsPerBatch`, `delaySecondsBetweenJobs`, `batchCooldownSeconds`, and
  `dryRunOnly` remain part of the same canonical workers object

### Requirement: Validated pacing mode and threshold

The settings update flow MUST reject an explicitly submitted pacing mode other
than `time` or `progress`. It MUST reject an explicitly submitted progress
threshold below 1 or above 100, following the existing settings validation
response convention. The threshold MAY remain stored while time mode is active,
and the delay MAY remain stored while progress mode is active; inactive values
MUST NOT be silently erased.

#### Scenario: Threshold boundaries are accepted

- **WHEN** a worker settings update submits a threshold of 1 or 100
- **THEN** the update is accepted and the submitted integer is retained

#### Scenario: Out-of-range threshold is rejected

- **WHEN** a worker settings update submits a threshold of 0 or 101
- **THEN** the update is rejected as invalid and the existing workers setting
  is not replaced by the invalid value

#### Scenario: Invalid mode is rejected

- **WHEN** a worker settings update submits a pacing mode other than `time` or
  `progress`
- **THEN** the update is rejected as invalid

### Requirement: Time mode preserves delay pacing

When the effective pacing mode is `time`, the existing
`delaySecondsBetweenJobs` behavior MUST remain authoritative for start pacing.
The progress threshold MUST be ignored. Existing concurrency, batch cooldown,
scheduler/resource, reservation, workspace, working-hours, execution-plan,
queue-order, and claim-protection behavior MUST remain unchanged.

#### Scenario: Time mode uses the configured delay

- **WHEN** time mode is selected and the previous start is still within
  `delaySecondsBetweenJobs`
- **THEN** the next claim is blocked by the existing time-delay gate even if
  the latest job's persisted progress is at or above the configured threshold

#### Scenario: Time mode ignores the progress threshold

- **WHEN** time mode is selected and the delay has elapsed
- **THEN** the claim is not blocked by `startNextJobAtProgressPercent`

### Requirement: Progress mode uses a worker-scoped persisted reference

When the effective pacing mode is `progress`, the next claim for worker W
MUST use the most recently started Queue job assigned to W as its pacing
reference. Reference selection MUST order by `started_at DESC` and then `id
DESC` as a deterministic tie-breaker, and MUST filter by the canonical worker
name after the requested worker name has been resolved. The decision MUST use
the persisted Queue progress value, not a frontend estimate, websocket-only
state, FFmpeg-local state, or a separate in-memory tracker.

If worker W has no previously started Queue job, progress pacing MUST permit
the first claim, subject to all other claim guards.

#### Scenario: First progress-paced claim is permitted

- **WHEN** progress mode is active for a worker with no previously started job
- **THEN** the worker may claim the next runnable job if every other guard
  permits it

#### Scenario: Below-threshold reference blocks a claim

- **WHEN** the latest started running job for worker W has persisted progress
  74 and the threshold is 75
- **THEN** the next claim for W is blocked by progress pacing

#### Scenario: Exact threshold opens the pacing gate

- **WHEN** the latest started running job for worker W has persisted progress
  exactly equal to the threshold of 75
- **THEN** progress pacing permits the claim to continue to the other guards

#### Scenario: Above-threshold reference opens the pacing gate

- **WHEN** the latest started running job for worker W has persisted progress
  90 and the threshold is 75
- **THEN** progress pacing permits the claim to continue to the other guards

#### Scenario: Reference ordering is deterministic

- **WHEN** two jobs assigned to the same worker have equal `started_at` values
- **THEN** the job with the greater Queue job ID is the pacing reference

#### Scenario: Worker names isolate pacing decisions

- **WHEN** worker-a's latest job is at progress 20 and worker-b has no prior
  started job
- **THEN** worker-a is progress-paced blocked while worker-b may claim if its
  own capacity and resources permit

#### Scenario: Persisted Queue progress is authoritative

- **WHEN** the database stores a reference progress value below the threshold
  while a frontend or websocket reports a value at or above it
- **THEN** the claim remains blocked until the persisted Queue progress reaches
  the threshold or the reference becomes terminal

### Requirement: Reference handoff and terminal references

After a job is claimed, it MUST immediately become the pacing reference for
its assigned worker through its persisted worker name and start timestamp. A
later reference MUST NOT be bypassed by an older job that remains above the
threshold.

If the latest started reference is in an existing terminal Queue status,
progress pacing MUST permit the next claim subject to every other guard,
regardless of the terminal reference's progress. The implementation MUST use
the repository's canonical terminal-status semantics. It MUST NOT fall back to
an older reference when the latest reference is terminal.

#### Scenario: Newly started job takes over as reference

- **WHEN** job A is at 80 and permits job B to start, and job B is persisted at
  progress 1
- **THEN** B is the pacing reference and job C remains blocked even while A
  remains above 75

#### Scenario: Terminal reference does not permanently block

- **WHEN** the latest reference job fails, is canceled, or completes below the
  configured threshold and capacity is available
- **THEN** the next job may claim without waiting for the terminal job to reach
  the threshold

#### Scenario: Terminal latest reference does not fall back

- **WHEN** job A is running at 90, job B starts later at 20 and then fails
- **THEN** job C may become eligible from B's terminal state, and the decision
  does not fall back to A as the pacing reference

### Requirement: Progress pacing does not override claim guards

Opening the progress gate MUST only remove the progress pacing block. The claim
MUST still pass the existing `maxConcurrentJobs` and worker availability
capacity checks, scheduler resource evaluation, active reservation and
workspace constraints, working-hours policy, execution-plan eligibility,
queue ordering and claim protections, and batch cooldown rules. Progress mode
MUST ignore `delaySecondsBetweenJobs` for start pacing but MUST NOT disable
`maxJobsPerBatch` or `batchCooldownSeconds`.

When progress pacing blocks a claim, the queued execution plan MUST remain in
its current state; temporary worker pacing MUST NOT be recorded as an
execution-plan resource or schedule failure.

#### Scenario: Capacity remains a hard ceiling

- **WHEN** the reference reaches the threshold but
  `maxConcurrentJobs` or worker capacity is full
- **THEN** no new job is claimed

#### Scenario: Scheduler and working-hours guards remain authoritative

- **WHEN** the reference reaches the threshold but scheduler resources or the
  working-hours policy rejects the candidate
- **THEN** no job is claimed and the existing scheduler decision remains the
  authority

#### Scenario: Batch cooldown remains independent

- **WHEN** the progress threshold is reached but `maxJobsPerBatch` and
  `batchCooldownSeconds` still require a rest period
- **THEN** the claim remains blocked by batch cooldown and the newly eligible
  progress gate does not bypass batch accounting

#### Scenario: Delay is not authoritative in progress mode

- **WHEN** progress mode is active, the reference reaches the threshold, and
  `delaySecondsBetweenJobs` has not elapsed
- **THEN** the delay does not block the claim, but all other guards still apply

### Requirement: Atomic concurrent claim behavior

Progress reference evaluation and the queued-job-to-running transition MUST
remain within the existing authoritative atomic claim boundary. A successful
claim MUST persist its worker assignment, start timestamp, and initial Queue
progress before another claim attempt can evaluate the same worker's reference.

#### Scenario: One threshold event cannot start two jobs

- **WHEN** `maxConcurrentJobs` is 3, job A is persisted at progress 80 with a
  threshold of 75, and two claim attempts race
- **THEN** one attempt may start B, B immediately becomes the newest reference,
  and the other attempt is blocked because B has not reached 75

### Requirement: Distinct transient progress-pacing result

When progress pacing is the blocking guard, the backend MUST return a distinct
internal claim result from worker capacity, time delay, batch cooldown, and no
runnable job. The HTTP response MUST use the existing transient pacing/limit
class used for delay pacing and provide a useful progress-pacing message. This
temporary claim gate MUST NOT be represented as a queued execution-plan
`WAITING` state.

#### Scenario: Progress pacing reports a transient claim block

- **WHEN** a worker's latest running reference is below the progress threshold
  and every earlier guard would otherwise allow a claim
- **THEN** the claim returns the distinct progress-pacing result with the
  existing transient HTTP class and does not mutate the queued plan to a
  resource or schedule waiting state

### Requirement: Existing claim attempts reevaluate current progress

Progress pacing MUST be evaluated when an existing worker claim attempt occurs.
The feature MUST NOT require a new polling daemon, timer, dispatcher, or
frontend-owned scheduler. A later claim attempt MUST observe the current
persisted reference state after a progress update, terminal transition, or
backend restart.

#### Scenario: No new polling mechanism is required

- **WHEN** a worker makes its normal next claim attempt after persisted
  progress changes
- **THEN** the claim gate reevaluates the current Queue state without a pacing
  daemon or additional scheduler

### Requirement: Settings UI exposes one canonical pacing contract

Settings > Workers MUST use the existing settings load/save flow and the same
canonical workers object. It MUST provide a "Start next job by" choice between
time delay and job progress. Time mode MUST show or enable
`delaySecondsBetweenJobs`; progress mode MUST show or enable
`startNextJobAtProgressPercent` with clear percent semantics. Only the active
mode's control is authoritative, switching modes MUST preserve the inactive
value, and advanced JSON MUST remain compatible with the canonical object.

#### Scenario: Settings load defaults a missing mode

- **WHEN** Settings loads a legacy workers object without a pacing mode
- **THEN** the UI selects Time delay and displays the existing delay value

#### Scenario: Switching modes changes the authoritative control

- **WHEN** a user switches from Time delay to Job progress
- **THEN** the progress percentage control is active, the delay is not
  authoritative, and the prior delay value remains preserved

#### Scenario: Settings saves the canonical workers object

- **WHEN** a user changes pacing mode or threshold and saves either the regular
  form or compatible advanced JSON
- **THEN** the existing workers settings endpoint receives one canonical object
  containing both pacing fields and the inactive value is not discarded
