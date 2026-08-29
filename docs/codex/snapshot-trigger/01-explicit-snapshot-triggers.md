# Task 1 — Make snapshot generation explicit-only

Follow all applicable `AGENTS.md` files.

## Goal

Opening Asset Info must never start snapshot generation automatically.

Snapshot generation may start only from an explicit user action such as:

- `Analyze asset`
- `Rescan`

## Current bug

An existing snapshot can still trigger another analysis when Asset Info opens if
backend state reports `requiresAnalysis=true`.

The current automatic trigger can also re-arm after snapshot updates,
query invalidation, rerenders, or remounts and enter a repeated execution loop.

## Required

- Inspect the current Asset Info / snapshot trigger flow first.
- Remove the automatic `snapshot.mutate(...)` path tied to dialog open,
  `requiresAnalysis`, query refresh, rerender, or remount.
- Remove/refactor any `automaticSnapshotKey` or equivalent guard that exists
  only to support automatic generation.
- Opening Asset Info must be read-only.
- `Analyze asset` still generates a snapshot exactly once when explicitly clicked.
- `Rescan` still generates a forced snapshot exactly once when explicitly clicked.
- Do not change the snapshot backend contract unless necessary.
- Do not redesign Asset Info.

## Tests

Cover at least:

- current stored snapshot + open dialog => no snapshot operation;
- stale stored snapshot + open dialog => no snapshot operation;
- no snapshot + open dialog => no snapshot operation;
- click Analyze => exactly one operation;
- click Rescan => exactly one forced operation.

Inspect first, then implement and validate. Do not stop at a plan.
