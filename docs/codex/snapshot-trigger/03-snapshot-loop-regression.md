# Task 3 — Close snapshot loop regressions

Follow all applicable `AGENTS.md` files.

## Goal

Prove that snapshot success, refetches, invalidation, rerenders, and dialog
reopens cannot start an unintended second snapshot operation.

## Required

- Inspect the final snapshot mutation/query flow after Tasks 1–2.
- Ensure mutation success may refresh cached snapshot/asset data without
  triggering generation again.
- Reopening Asset Info after a successful snapshot must only read the stored
  snapshot.
- Query invalidation/refetch must never be an implicit generation trigger.
- Rerender/remount must never be an implicit generation trigger.
- Preserve explicit Analyze and Rescan behavior.
- Avoid adding another ref/key/debounce guard to simulate safety; generation
  should be impossible without an explicit action.

## Regression tests

Add focused tests for:

1. Analyze succeeds -> queries invalidate/refetch -> no second operation.
2. Rescan succeeds -> queries invalidate/refetch -> no second operation.
3. Close and reopen Asset Info -> no operation.
4. `requiresAnalysis` remains true after refetch -> no automatic operation.
5. Multiple rerenders while dialog is open -> no automatic operation.

Run all validation required by `AGENTS.md`.

## Done when

The snapshot operation count can only increase because the user explicitly
clicked Analyze or Rescan.

Inspect first, then implement and validate. Do not stop at a plan.
