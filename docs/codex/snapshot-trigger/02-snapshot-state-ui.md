# Task 2 — Normalize snapshot UI state

Follow all applicable `AGENTS.md` files.

## Goal

Make Asset Info distinguish snapshot existence from snapshot freshness.

## Required behavior

Use explicit conceptual states:

```text
missing
current
stale
```

### Missing

No stored snapshot exists.

Show:

```text
No snapshot available
[Analyze asset]
```

Do not auto-run analysis.

### Current

Stored snapshot exists and is current.

Show the normal Asset Info tabs.

Do not start analysis.

### Stale

Stored snapshot exists but backend reports it should be refreshed.

Keep showing the existing snapshot and normal tabs.

Also show a compact warning/action such as:

```text
Snapshot needs refresh
[Rescan]
```

Do not auto-run analysis.

## Required

- Do not use `requiresAnalysis` as a proxy for snapshot existence.
- Derive clear booleans/state for:
  - stored snapshot exists;
  - refresh required;
  - operation pending.
- Avoid contradictory empty/loading/tab states.
- Preserve all existing Asset Info tabs and actions.
- Reuse existing snapshot data while stale.
- Do not redesign unrelated Assets UI.

## Tests

Cover missing/current/stale rendering and verify the correct action is visible
for each state.

Inspect first, then implement and validate. Do not stop at a plan.
