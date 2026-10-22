## Why

Batch Preflight exposes numeric profile and library IDs and raw audio/track keys even though its candidate already contains the effective frozen plan and assignment provenance. Operators cannot quickly tell whether a value came from the Asset, Path, Logical Group, Source Group, Global/default, or an explicit candidate edit, and a stale assignment can look current.

## What Changes

- Show named Video, Audio, Track, and destination selections with stable IDs/keys as secondary diagnostics, alongside the exact candidate-effective snapshot and assignment origin.
- Preserve unchanged candidate assignments and snapshots. Detect when a canonical assignment changed after candidate resolution and provide an explicit re-resolve action that presents before/after values for review before acceptance.
- Keep candidate-only edits explicit and visible; retain missing profile or library identities as explained errors instead of guessing replacements.
- Verify selected-assets Path precedence and the reviewed-to-accepted frozen plan with synthetic regression fixtures.

Out of scope: changing global assignment precedence, changing accepted Queue jobs or ExecutionPlans, modifying production/NAS data, or changing Test Encode and LAB behavior.

## Capabilities

### New Capabilities

- `queue-preflight-assignment-review`: Human-readable, provenance-aware review and explicit refresh of effective assignments in Batch Preflight.

### Modified Capabilities

None.

## Impact

- Backend: selected-assets and persisted-candidate preflight read/update/accept handlers; no database migration is expected.
- Frontend: Batch Preflight dialog and its Assets, Queue, and Scanner entry points, reusing the existing profile, library, and settings inventories.
- Tests: focused handler and dialog regressions; no new dependencies or media/filesystem operations.
