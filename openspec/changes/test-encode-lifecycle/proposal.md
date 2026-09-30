## Why

Test Encode records, retention, sidecar ownership, and cleanup are currently spread across one row, per-asset history, and general housekeeping. This makes retained tests hard to manage globally and leaves reconciliation and safe cleanup dependent on incomplete path metadata.

## What Changes

- Add one canonical persisted Test Encode inventory with explicit ownership for the encoded output, temporary output, and generated subtitle sidecars.
- Add separate persisted reconciliation results for missing, partial, interrupted, and orphaned artifacts without changing execution status.
- Add a Test Encode Settings domain for lifecycle defaults and retained Test Runs, using the existing atomic Settings update path.
- Add bounded reconciliation and selected cleanup while preserving Keep/Temporary semantics and the current Test Encode output/execution contract.
- Backfill artifact ownership and retention defaults for existing Test Encodes; continue exposing the same canonical history to Asset Info and LAB.
- Reuse housekeeping for automatic cleanup of eligible non-Keep runs in small batches.
- Keep LAB V2, Queue/Worker redesign, Profile redesign, and unrelated sidecar or storage cleanup out of scope.

## Capabilities

### New Capabilities

- `test-encode-lifecycle`: canonical Test Encode inventory, retention, artifact ownership, reconciliation, safe cleanup, Settings, and UI behavior.

### Modified Capabilities

None. The repository has no existing main spec for Test Encode or Settings domains; this change defines the Test Encode lifecycle contract.

## Impact

- Backend: `TestEncode` persistence and migration/backfill, Test Encode and Settings handlers/routes, artifact reconciliation and cleanup, and existing housekeeping integration.
- Frontend: Settings domain navigation and controls, bounded Test Runs inventory/actions, API types/client, and the shared Test Encode history component used by Asset Info and LAB.
- Storage: current outputs remain under the configured destination root of the Test Encode's bound Library. Cleanup acts only on exact persisted artifact paths and does not scan or recursively delete Library directories.
- Compatibility: existing Test Encode status values, execution planning, Jellyfin refresh behavior, and retained records remain available.
