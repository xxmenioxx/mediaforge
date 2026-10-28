## 1. Cancellation and cleanup lifecycle

- [x] 1.1 Persist a running-job cancellation request and make queued cancellation terminal with atomic reservation release; verify queued/running and worker-callback race tests.
- [x] 1.2 Schedule policy-guarded canceled-job cleanup outside request transactions and persist observable cleanup status/error; verify slow-cleanup response and cleanup-failure tests.
- [x] 1.3 Preserve canceled history and lifecycle blockers through worker late callbacks; verify asset lifecycle projection and reservation regressions.

## 2. Exact path-group dismissal

- [x] 2.1 Add bounded preview and transactional dismissal APIs for exact normalized Queue path groups with optional selected IDs and result counts; verify exact-path, mixed-status, validation ownership, idempotency, and conflict tests.
- [x] 2.2 Keep batch dismissal eligibility/history behavior while removing synchronous filesystem cleanup; verify existing batch dismissal tests and a slow-cleanup barrier.

## 3. Queue presentation and validation

- [x] 3.1 Add path-group preview/selection/confirmation UI with backend counts, cancellation stopping/cleanup feedback, and one Queue/Assets invalidation; verify focused Queue mutation tests.
- [x] 3.2 Measure dismissal for 1, 25, and 100 canceled records with synthetic slow cleanup and record observed backend timings without claiming NAS results. Ten-iteration SQLite request timings: 0.76 ms / 4.75 ms / 17.40 ms; the blocked-cleanup regression passed.
- [x] 3.3 Run `./scripts/verify.sh --auto` through the serialized dupla shared-validation lock and record the observed check result. Auto scope selected full; all 7 checks passed.
