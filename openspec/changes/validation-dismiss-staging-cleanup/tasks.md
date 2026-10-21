## 1. Backend contract and durable records

- [x] 1.1 Add backward-compatible preview and append-only cleanup evidence models and include them in the normal database migration; cover existing database migration.
- [x] 1.2 Implement job-scoped preview issuance with current Dismiss/Validation/Result/ExecutionPlan identity, exact QueueJob ID workspace resolution, file manifest, byte totals, ownership, active-reference blockers, and missing-workspace no-op.
- [x] 1.3 Implement a one-time token-bound post-Dismiss Housekeeping operation, rechecking all bound identities and reference surfaces; isolate it from the singleton global preview and keep general dismissed-job protection unchanged.
- [x] 1.4 Extend the existing Housekeeping deletion boundary for per-job serialization and rooted, no-follow, manifest-only removal; accurately persist complete, no-op, partial, blocked, and failed outcomes and support a fresh-preview retry.
- [x] 1.5 Add focused backend tests for identity mismatch, stale/consumed tokens, QueueJob ID versus execution number, unsafe paths/roles, all active references, concurrency, partial failure/retry, zero workspace, preserved evidence, and global Housekeeping isolation.
- [x] 1.6 Stream only reference-bearing path/evidence columns for historical surfaces; retain symlink-canonical path semantics and fail-closed behavior. Cover large unrelated history, normalized paths, and external symlink aliases. Historical negative-reference checks remain O(N) to preserve symlink equivalence.
- [x] 1.7 Preserve the latest confirmed cleanup outcome when a later preview is a no-op; use a no-op preview only when no cleanup evidence exists.

## 2. Validation and History API/UI

- [x] 2.1 Add preview and scoped cleanup API routes/handlers with the exact response contract and append-only audit actor/action data; retain the existing Dismiss endpoint behavior and response.
- [x] 2.2 Extend History projection with batched latest staging outcome and physical availability, without changing historical Validation/Result evidence.
- [x] 2.3 Add a compact Validation Dismiss dialog with default-on optional cleanup, exact preview details/blockers, Dismiss-first ordering, and distinct post-Dismiss cleanup results.
- [x] 2.4 Add History review/retry using a new bounded preview and explicit confirmation; retain Dismissed, Validation, and Result provenance after cleanup.
- [x] 2.5 Add focused frontend tests for default-on/off, blocked preview, dismiss/cleanup failure separation, and History retry.

## 3. Verification and delivery

- [x] 3.1 Validate the OpenSpec change strictly and run the focused backend/frontend checks.
- [x] 3.2 Run `./scripts/verify.sh --auto` through the serialized shared-validation lock and resolve only reported in-scope failures.
- [x] 3.3 Inspect changed paths and lines for scope/secrets, commit the scoped #376 changes locally, and report the commit and validation evidence.
- [x] 3.4 Run focused regression tests and serialized `verify.sh --auto` for the historical reference and History outcome corrections; `verify.sh --auto` passed (full, 6/6, 287 s).
