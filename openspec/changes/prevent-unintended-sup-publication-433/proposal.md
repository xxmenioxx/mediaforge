## Why

Publisher currently adds every ready subtitle artifact to its delivery manifest. A match to the frozen Track Plan only affects the `Required` flag, so a stale or mismatched artifact can still be promoted. Bind publication to the frozen subtitle decision while preserving explicitly authorized native extraction, including PGS `.sup`.

## What Changes

- Require generated subtitle artifacts to match the frozen stream, format, mode, and artifact identity before Publisher promotes them.
- Keep OCR intermediates operation-owned; converted bitmap outputs publish only the requested text representations.
- Preserve explicit original extraction and existing legacy artifact/external text sidecar behavior where frozen operation metadata authorizes it.
- Keep Preflight's canonical mode, format, and decision-source projection aligned with the Worker and Publisher plan.

Out of scope: changing the existing original-format default, migrating or deleting existing Library sidecars, changing OCR/no-cue policy, or production/NAS diagnosis and cleanup.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: publication must promote only subtitle artifacts authorized by the frozen resolved output decision, with safe legacy compatibility.

## Impact

Backend subtitle artifact metadata and Publisher manifest construction, focused resolver/worker/manifest regressions, and the subtitles OpenSpec contract. No API migration, database migration, dependency, or frontend policy resolver is required. Existing Queue snapshots remain immutable.
