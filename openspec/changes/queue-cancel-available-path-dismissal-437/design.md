## Context

See `proposal.md` for motivation and `specs/queue-cancellation-and-dismissal/spec.md` for the observable contract. Queue currently reports a running job as canceled and releases its reservation as soon as cancellation is requested, even though the local process may still be exiting. Batch dismissal also performs guarded cleanup synchronously. Queue path groups use the lexical parent of a normalized media path, while batch cards are keyed by batch ID.

## Goals / Non-Goals

**Goals:** preserve active ownership until worker acknowledgment, support exact path-group removal across batches, keep dismissal bounded to database work, and expose cleanup outcomes.

**Non-Goals:** add a new QueueJob status, change completed/Validation/Publisher dismissal policy, follow symlinks to infer path identity, or remove source/Library files.

## Decisions

1. **Persist cancellation request without changing the public job status.** Add `cancellation_requested_at` while a job remains `running`. A queued job transitions directly to `canceled`; the same transaction releases its Scheduler reservation. A running job retains its reservation and active lifecycle projection until the local process monitor or worker status endpoint acknowledges a terminal result. That acknowledgment owns the terminal transition and reservation release. Compare-and-update checks prevent stale cancellation requests or callbacks from overwriting a newer state. Late callbacks cannot move a terminal canceled record back to active status. GORM AutoMigrate adds the nullable request field; existing rows remain unchanged.

2. **Derive path-group identity the same way as Queue display.** Normalize separators and compare the exact lexical parent group key of `QueueJob.MediaPath`; the special `Root` group follows the existing Queue helper. A prefix query may narrow candidate rows, but every preview and mutation rechecks exact group equality. The operation never resolves or mutates filesystem paths. Preview scans at most 10,000 candidate rows and returns at most 500 stable IDs in ID order; confirmation sends at most 500 selected IDs. One transaction uses compare-and-update for each ID and reports already-dismissed, skipped, and conflicted outcomes. Never-executed placeholders are soft-dismissed so retries can report them as already dismissed; their unused ExecutionPlans are removed.

3. **Keep cleanup outside dismissal and reuse its existing policy/guard.** Remove the post-transaction `cleanupCanceledJob` loop from batch dismissal. Cancellation continues to use the existing policy-gated cleanup helper, but schedules it after the lifecycle transaction and records not-required/pending/running/completed/failed state and error text on the Queue record. A small process-local concurrency limit bounds filesystem work. The existing Housekeeping preview/run flow remains the authority for broader orphan/workspace cleanup; neither Queue dismissal nor this cancellation helper expands its controlled roots.

4. **Use one Queue-level preview/confirm flow for path removal.** Add preview and dismiss API operations accepting the exact path-group key and optional selected IDs. The UI can start the flow from any eligible canceled row, display backend counts/reasons and selectable candidates, then send one bounded dismiss request. Batch removal remains available with its all-canceled predicate. Both flows invalidate Queue and Assets once after the response.

5. **Measure database dismissal separately from cleanup.** Add focused tests/benchmarks for 1, 25, and 100 canceled records with an injected slow cleanup barrier. The API must complete while the cleanup barrier is blocked; report measured local database times as test evidence and do not claim NAS benchmark results. Ten-iteration SQLite request measurements on the development host were 0.76 ms, 4.75 ms, and 17.40 ms respectively. These are post-change request timings; they exclude NAS I/O, asset catalog scans, and browser rendering.

## Risks / Trade-offs

- [A stale worker callback could race a cancellation request] → compare state inside the transaction and let the worker's final status own completion; keep the reservation while cancellation is pending.
- [A server restart could interrupt process-local cleanup] → persist cleanup state before starting work and leave pending/running state visible for reconciliation; retain Housekeeping as a safe manual path.
- [Queue path display is lexical, not filesystem-resolved identity] → use the same stable key for UI and backend, exact equality after candidate lookup, and no filesystem mutation in path dismissal.
- [Large paths could exceed a single safe request] → cap preview candidates and require the operator to confirm the returned bounded set; accept an explicit selected-ID subset.

## Migration Plan

Add a nullable cancellation-request timestamp plus cleanup status/error fields through the existing QueueJob AutoMigrate path. Legacy rows with empty cleanup state are treated as not scheduled. No status or execution-snapshot backfill is required. Rollback keeps the added columns; older binaries ignore them and continue reading existing Queue fields.
