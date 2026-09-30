## 1. Persistence and Settings policy

- [x] 1.1 Add canonical Test Encode artifact ownership and separate reconciliation fields; backfill existing output, temporary, and subtitle paths idempotently, and verify historical/kept migration regressions with disposable SQLite tests.
- [x] 1.2 Register and migrate the Test Encode Settings policy through the existing atomic Settings path; preserve legacy retention and verify validation, conservative defaults, and configured sample-window behavior with focused backend tests.

## 2. Backend inventory, reconciliation, and cleanup

- [x] 2.1 Persist exact output, temporary, and generated subtitle ownership across Test Encode success, failure, cancellation, and restart recovery; verify existing LAB and execution behavior with focused lifecycle tests.
- [x] 2.2 Implement bounded filtered inventory, persisted reconciliation findings, and size summaries over canonical runs/artifacts; verify healthy, missing, partial, interrupted, and proven-orphan cases without scans or ffprobe.
- [x] 2.3 Implement safe individual, bulk, stale-record, and proven-orphan cleanup with path containment, identity, symlink, and active-operation checks; verify every required deletion-safety regression and Keep confirmation.
- [x] 2.4 Reuse existing housekeeping for small-batch Temporary and Failed/Cancelled retention; verify Keep/active exclusion, missing-output handling, and idempotent cleanup.

## 3. Settings and shared UI

- [x] 3.1 Extend the shared Test Encode API/types and history UI with retention, reconciliation, output evidence, and configured sample defaults; verify Asset Info/LAB behavior with focused frontend tests.
- [x] 3.2 Add Test Encode Settings and bounded Test Runs filters/actions, including Keep confirmation and bulk artifact/storage summary; verify Settings and destructive-action UI tests.
- [x] 3.3 Remove Test Encode retention controls from general housekeeping while preserving legacy setting compatibility; verify Settings URL/domain and legacy-value regressions.

## 4. Validation

- [x] 4.1 Run focused backend and frontend tests, frontend lint/build, and `openspec validate test-encode-lifecycle --strict`; resolve only scoped failures.
- [x] 4.2 Run `./scripts/verify.sh --auto` and `./scripts/verify.sh --full`; record the observed verifier check counts and warnings.
