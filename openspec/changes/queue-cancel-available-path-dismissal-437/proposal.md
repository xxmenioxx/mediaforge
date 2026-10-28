## Why

Queue dismissal currently waits for per-job filesystem cleanup, so large canceled batches can keep the request open. Running cancellation can also appear terminal before the worker has acknowledged process exit, which risks making the RAW asset look reusable too early.

## What Changes

- Keep queued cancellation immediate, but represent running cancellation as requested/stopping until the worker reports its terminal result.
- Add exact path/logical-group preview and bulk dismissal for terminal canceled jobs across batches, with skipped and conflict counts.
- Keep filesystem cleanup out of dismissal transactions and expose its state separately from Queue history.
- Refresh Queue and affected asset lifecycle state after cancellation or dismissal without issuing per-job catalog refreshes.
- Add focused persistence, race, exact-path, idempotency, and responsiveness coverage.

This change moves cancellation cleanup ownership into the existing durable Housekeeping cycle, while preserving exact work-root boundaries and leaving Queue dismissal as database-only work. Started FFmpeg jobs use process-group termination and retain ownership until the group is verified absent. A descendant that escapes its process group is outside the verified contract and remains NO-GO. No Housekeeping roots are expanded. Deleting RAW or Library content and changing completed/Validation/Publisher dismissal rules remain out of scope.

## Capabilities

### New Capabilities

- `queue-cancellation-and-dismissal`: cancellation acknowledgment, asset availability, and safe canceled-job dismissal by exact path/logical group.

### Modified Capabilities

None.

## Impact

- Backend Queue and Worker status transitions, queue-job persistence and cleanup leases, Scheduler reservation release, Housekeeping execution, process-tree termination, and asset lifecycle projection.
- Queue API routes and response types for preview and path-scoped dismissal.
- Queue UI cancellation/dismissal feedback and Queue/asset query invalidation.
- Focused backend and frontend regressions; no new dependencies or source-media operations.
