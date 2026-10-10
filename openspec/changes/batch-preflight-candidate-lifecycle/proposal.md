## Why

Batch Preflight currently treats closing the dialog as a temporary UI change while Assets automatically restores the persisted candidate, so dismissed candidates unexpectedly return. A blocked asset that can no longer be queued also cannot be removed, leaving the user unable to retire that candidate without changing accepted Queue history.

## What Changes

- Separate a saved candidate reference from the intent to display the dialog; closing preserves the server candidate and explicit Resume restores it.
- Add revision-checked, persistent Remove from candidate and Discard candidate operations, with terminal candidate states when no pending assets remain.
- Preserve partial acceptance history and all accepted QueueJobs, ExecutionPlans, SchedulerReservations, and source files.
- Add frontend lifecycle recovery, error feedback, and regressions for stale responses and events.

Out of scope: changing Queue execution or frozen plans, canceling accepted jobs, repairing or moving archived media, and changing the #311 archive UI work.

## Capabilities

### New Capabilities

- `queue-batch-preflight`: Define explicit close/resume, remove, and discard behavior for persisted Batch Preflight candidates.

### Modified Capabilities

None.

## Impact

- Backend: `QueueBatchPreflight` persistence, `queue_preflight.go`, preflight routes, and focused handler/migration tests.
- Frontend: the Batch Preflight dialog, Assets candidate recovery, event/storage helpers, API client/types, and focused UI/integration tests.
- Architecture documentation: the implemented Queue Batch Preflight lifecycle contract.
- No new dependencies or media/filesystem operations.
