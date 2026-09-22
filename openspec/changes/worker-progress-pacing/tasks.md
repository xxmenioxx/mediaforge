## 1. Worker settings contract

- [ ] 1.1 Extend the existing backend `workers` settings parsing with canonical `jobStartPacingMode` and `startNextJobAtProgressPercent` values, defaulting missing mode to `time` and missing threshold to `75`; verify legacy worker settings still resolve to the current delay-based behavior without a new model or table.
- [ ] 1.2 Add explicit validation for submitted worker pacing mode and threshold values using the existing settings update error convention; verify mode values other than `time`/`progress` and thresholds outside `1..100` are rejected while omitted and inactive values remain compatible.
- [ ] 1.3 Extend `SettingsForm`, initial defaults, settings loading, core save, and advanced JSON reconstruction with both pacing fields; verify the existing `workers` AppSetting endpoint receives one canonical object containing active and inactive values.

## 2. Claim-path progress gate

- [ ] 2.1 Add a worker-scoped pacing-reference evaluation inside the existing claim transaction, selecting the latest started Queue job for the resolved worker by `started_at DESC, id DESC` and using persisted `QueueJob.Progress`; verify first-job, below-threshold, exact-threshold, above-threshold, and worker-isolation behavior.
- [ ] 2.2 Branch the existing start gate so time mode preserves `delaySecondsBetweenJobs` and progress mode replaces only that delay gate while `maxJobsPerBatch` and `batchCooldownSeconds` remain independent; verify threshold-open claims still encounter existing capacity, scheduler, reservation, workspace, working-hours, execution-plan, and queue guards.
- [ ] 2.3 Reuse canonical terminal-status semantics for the latest reference and prevent fallback to older jobs; verify failed, canceled, and completed latest references below threshold allow a subsequent claim when other guards permit it.
- [ ] 2.4 Add a distinct progress-pacing claim result and map it through the existing claim endpoint to the transient HTTP class used for delay pacing; verify the response is distinguishable from worker limit, delay, batch cooldown, and no-runnable-job outcomes and does not mutate a queued ExecutionPlan into `WAITING`.
- [ ] 2.5 Preserve the existing `claimJobMu`, transaction, reservation activation, and running transition so a successful claim persists `WorkerName`, `StartedAt`, and initial `Progress` before another same-worker claim evaluates pacing; verify no second claim can leapfrog the newly started reference.

## 3. Backend regressions

- [ ] 3.1 Extend `backend/internal/handlers/worker_concurrency_test.go` with settings default/parsing and validation coverage for legacy mode, explicit modes, threshold boundaries, and preservation of existing delay/batch fields; verify the focused Go tests pass.
- [ ] 3.2 Add threshold-boundary and persisted-progress tests covering first claim, 74/75/90 behavior, and rejection of frontend-only or non-persisted progress as a pacing signal; verify the claim decision reads the disposable SQLite Queue state.
- [ ] 3.3 Add deterministic reference-selection and handoff tests using equal `StartedAt` values and increasing IDs; verify a newly started B becomes the reference immediately and C remains blocked while B is below threshold even when A is above it.
- [ ] 3.4 Add terminal-reference tests for failed, canceled, and completed jobs, including the latest-terminal/no-fallback case; verify the worker is not permanently blocked and does not resume an older reference.
- [ ] 3.5 Add multi-worker isolation coverage with unrelated capacity/resource settings permissive; verify a below-threshold worker-a reference does not block worker-b's first claim.
- [ ] 3.6 Add a concurrent claim race regression with `maxConcurrentJobs = 3`, a persisted 80% reference, and two simultaneous claim attempts; verify exactly one new job starts and the second observes the new below-threshold reference.
- [ ] 3.7 Add focused hard-guard regressions for full capacity, scheduler/resource or working-hours rejection, active batch cooldown, progress mode ignoring time delay, and unchanged reservation accounting; verify progress pacing never overrides these guards.

## 4. Settings UI regressions

- [ ] 4.1 Add focused Settings coverage for loading a missing mode as Time delay, switching between Time delay and Job progress, and rendering/enabling only the active authoritative control; verify the inactive delay or threshold value remains unchanged.
- [ ] 4.2 Add focused Settings coverage for percentage semantics and `1..100` validation, normal form save, and advanced JSON round trip; verify the saved `workers` payload contains both pacing fields through the existing update flow.

## 5. Validation and handoff

- [ ] 5.1 Run the focused backend and frontend checks for the implementation, then run `./scripts/verify.sh --auto`; verify the repository verifier reports `ALL CHECKS PASSED` and report any known frontend lint baseline separately.
- [ ] 5.2 Run `openspec validate worker-progress-pacing --type change --strict`; verify all proposal, spec, design, and task artifacts validate without warnings.
