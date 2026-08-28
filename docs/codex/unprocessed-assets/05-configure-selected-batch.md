# Task 5 — Make Configure selected transactional

Follow all applicable `AGENTS.md` files.

## Goal
Replace per-LogicalGroup request loops with one backend batch operation.

## Required
- Operate only on fully selected LogicalGroups.
- Apply changes at `LogicalGroup` scope only.
- Preserve `NO_CHANGE`, `INHERIT`, `EXPLICIT_VALUE`, and `DISABLED`.
- Change only dimensions explicitly requested.
- Do not create descendant Path or Asset overrides.
- Persist the batch in one DB transaction; rollback on failure.
- Invalidate affected effective-configuration data after success.
- Add tests for partial-field updates, inherit, disabled, rollback, and no Path
  overrides.

Inspect first, then implement and validate. Do not stop at a plan.
