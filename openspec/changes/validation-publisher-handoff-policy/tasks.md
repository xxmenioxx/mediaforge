## 1. Canonical Validation handoff and authorization

- [x] 1.1 Add the shared outcome/finding handoff decision and a table-driven test matrix for PASS, advisory PASS_WITH_WARNINGS with actionable and nonactionable findings, blocking warnings, REVIEW_REQUIRED, FAIL, advisory/critical UNVERIFIED, and inconsistent outcomes.
- [x] 1.2 Route manual and automatic Validation through the shared decision; verify manual/automatic parity, missing/empty policy defaults, explicit PASS_WITH_WARNINGS opt-in, current-policy changes, and Auto Publisher disabled behavior.
- [x] 1.3 Keep Publisher authorization separate from handoff; verify canonical Result/output identity, required sidecars, destination/replacement constraints, automatic denial of manual overrides, exact manual REVIEW_REQUIRED override behavior, and unconditional FAIL/critical-evidence blocking.

## 2. Durable lifecycle, concurrency, and recovery

- [x] 2.1 Make Queue stage transitions operate on the latest persisted history with conditional conflict detection; verify concurrent stage writers cannot lose events and stale transitions return an error or an idempotent current result.
- [x] 2.2 Propagate `validating`, `validation_review`, and `ready_to_publish` persistence errors through worker, manual Validation, and status-update paths; keep status-only conversion completion at review, verify a failed Ready write prevents Publisher invocation, and retain fallback-stage errors.
- [x] 2.3 Keep the `publishing` stage and fenced publication claim atomic; verify stage-write failure rolls back the claim, retries do not duplicate active attempts, and expired-lease restart recovery reuses the persisted attempt identity.
- [x] 2.4 Verify Auto Publisher does not promote historical review jobs, rechecks current warning policy, and blocks identity, sidecar, destination, and replacement-authority failures without marking publication complete.

## 3. Queue checklist and OpenSpec reconciliation

- [x] 3.1 Separate conversion completion from lifecycle terminal/publication completion in the Queue checklist; verify completed-conversion jobs at review, Ready, and Publishing remain visibly unpublished until persisted publication evidence exists.
- [x] 3.2 Reconcile the OpenSpec requirement, design, and transition matrix with the implemented stage, policy, retry, and restart behavior.

## 4. Integration and completion

- [x] 4.1 Run `./scripts/verify.sh --auto` and OpenSpec strict validation, resolve reported failures, reconcile completed checkboxes, and review the scoped diff and final worktree status.
