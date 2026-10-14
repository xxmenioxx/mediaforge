## 1. OpenSpec and lifecycle contract

- [x] 1.1 Define reconciliation scope, protected states, and read-only
  operational verification.
- [x] 1.2 Keep the change limited to Queue stage projection and Queue
  presentation; preserve Validation and Publisher authorities.

## 2. Database reconciliation

- [x] 2.1 Validate lifecycle event stages/timestamps and distinguish absent,
  invalid, and out-of-order history.
- [x] 2.2 Reconcile only stale status-fallback projections, with a conditional
  update that preserves concurrent transitions.
- [x] 2.3 Cover repeat migrations, warning review, PASS readiness, legacy
  history absence, invalid history, published/terminal rows, newer stages, and
  concurrent transitions.

## 3. Queue presentation

- [x] 3.1 Render `validation_review` in the persisted lifecycle checklist with
  warning semantics.
- [x] 3.2 Cover consistent current-stage and checklist presentation.

## 4. Verification and delivery

- [x] 4.1 Run focused database, lifecycle, and Queue checks.
- [x] 4.2 Run `./scripts/verify.sh --auto` and require `ALL CHECKS PASSED`.
- [x] 4.3 Review the scoped diff and reconcile completed OpenSpec task boxes.
