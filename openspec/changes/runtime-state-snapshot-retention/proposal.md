## Why

Issue #149 separates live host state from immutable runtime evidence. Today
startup, the five-minute detector, the Dashboard's 30-second refresh, manual
refresh, and runtime-policy updates all call `DetectAndSave()`, so routine
telemetry creates unbounded `RuntimeSnapshot` history and makes `Latest()`
ambiguous for current-state consumers and execution provenance.

## What Changes

- Add a canonical, singleton `RuntimeState` representing the latest detected
  host state, including volatile telemetry, stable capabilities, effective
  runtime policy, and detection metadata.
- Change startup, periodic detection, Dashboard/API refresh, Settings runtime
  refresh, and equivalent current-state consumers to upsert `RuntimeState`
  rather than append an immutable snapshot.
- Define a versioned, canonical, deterministic `RuntimeStableFingerprint` that
  excludes rapidly changing telemetry and binds stable capability/policy
  evidence to a schema version.
- Keep `RuntimeSnapshot` immutable and create it only through explicit audit
  capture boundaries: execution-plan freeze, Test Encode freeze, stable
  transition evidence, or explicit diagnostic/manual audit capture.
- Freeze `ExecutionPlan.RuntimeSnapshotID` once per material plan version and
  stop resource/waiting reevaluation from replacing it.
- Capture a distinct immutable snapshot once when Test Encode execution
  configuration is frozen; never use the mutable current-state row as audit
  provenance.
- Introduce one canonical discovery boundary for all referenced runtime
  snapshot IDs, initially covering Execution Plans and Test Encodes, and make
  pruning recheck references inside the deletion transaction.
- Add deterministic, bounded RuntimeSnapshot retention to the existing
  housekeeping operation, preserving referenced rows, the newest immutable
  row, and recent unreferenced history for seven days.
- Extend housekeeping reports with a structured runtime-snapshot retention
  section without placing database rows in filesystem candidate lists or
  claiming SQLite file-space recovery.
- Initialize `RuntimeState` safely from the newest legacy snapshot without
  deleting legacy history or performing unbounded cleanup in `AutoMigrate()`.
- Keep `GET /api/system/runtime` and `POST /api/system/runtime/refresh`
  wire-compatible while adding explicit live-state/provenance metadata.
- Update Dashboard and Settings wording so live telemetry is not presented as
  a newly recorded immutable host snapshot.

## Capabilities

### New Capabilities

- `runtime`: Define current runtime state, stable fingerprints, immutable audit
  capture, execution/Test Encode provenance, retention, and housekeeping
  behavior.

### Modified Capabilities

None.

## Impact

- Backend runtime detection, runtime handlers, scheduler planning/resource
  evaluation, Test Encode preparation, database migration, models, and
  housekeeping/reporting.
- Frontend runtime API types, Dashboard runtime panel, Settings runtime view,
  and refresh-success/status labels.
- Existing `RuntimeSnapshot` rows and referenced plan/Test Encode IDs remain
  valid; no source media, published media, or sidecars are changed.
- No new scheduler/goroutine, Settings retention UI, metrics time series,
  remote-worker redesign, or automatic SQLite `VACUUM` is introduced.

## Out of Scope

- Replacing host capability detection or scheduler resource policy.
- Keeping five-minute or 30-second telemetry as an audit time series.
- Removing historical plan provenance or changing existing media execution
  policy unrelated to runtime-state provenance.
- Automatic SQLite `VACUUM`, file compaction, or a new maintenance scheduler.
- A large new Settings surface or remote-worker runtime redesign.
