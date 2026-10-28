## 1. Cancellation and cleanup lifecycle

- [x] 1.1 Persist a running-job cancellation request and make queued cancellation terminal with atomic reservation release; verify queued/running and worker-callback race tests.
- [ ] 1.2 Partially implemented: cleanup is policy-guarded, status-tracked, bounded, and outside request transactions; transfer execution and restart reconciliation to durable Housekeeping ownership and verify recovery before marking complete.
- [x] 1.3 Preserve canceled history and lifecycle blockers through worker late callbacks; verify asset lifecycle projection and reservation regressions.

## 2. Exact path-group dismissal

- [x] 2.1 Add bounded preview and transactional dismissal APIs for exact normalized Queue path groups with optional selected IDs and result counts; verify exact-path, mixed-status, validation ownership, idempotency, and conflict tests.
- [x] 2.2 Keep batch dismissal eligibility/history behavior while removing synchronous filesystem cleanup; verify existing batch dismissal tests and a slow-cleanup barrier.

## 3. Queue presentation and validation

- [x] 3.1 Add path-group preview/selection/confirmation UI with backend counts, cancellation stopping/cleanup feedback, and one Queue/Assets invalidation; verify focused Queue mutation tests.
- [x] 3.2 Measure dismissal for 1, 25, and 100 canceled records; record before/after local request timings without claiming NAS results. Five runs of 30 requests each showed median SQLite timings of 0.599 / 5.553 / 19.912 ms before and 0.377 / 4.527 / 17.690 ms after; the separate blocked-cleanup regression passed.
- [x] 3.3 Run `./scripts/verify.sh --auto` through the serialized dupla shared-validation lock and record the final observed check result. Auto scope selected full; 6/6 checks passed.
