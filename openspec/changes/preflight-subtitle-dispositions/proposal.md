## Why

Batch Preflight currently shows a generic track snapshot instead of explaining the effective action for each source subtitle stream. This can lead operators to mistake preferred-language action rules for a language allowlist and accept a Queue plan that processes more languages than expected.

## What Changes

- Expose a versioned per-stream subtitle decision projection from the frozen Queue Preflight plan, including effective disposition, planned sidecars/OCR, and available source identity and provenance.
- Add a compact summary, expandable decision list, and explanation of the difference between preferred subtitle rules and stream filtering.
- Allow explicit candidate-only per-stream action overrides that are revisioned, resolved by the canonical track planner, and frozen into accepted Queue snapshots.
- Keep omitted override entries inherited, validate stream identity against the current candidate source, and provide a clear way to revert candidate choices.
- Keep changes isolated from saved Asset Tracks, Asset Defaults, and Track Profiles; preserve existing accepted and historical Queue snapshots.

Out of scope: changing global subtitle preferences or fallback actions, redesigning OCR or translation, automatic stream selection by language, and changing already accepted Queue jobs.

## Capabilities

### New Capabilities

- `queue-batch-preflight`: Review exact source subtitle dispositions and apply one-time candidate-only disposition overrides.

### Modified Capabilities

None. The existing `subtitles` and `asset-defaults` capabilities remain authoritative for policy resolution and artifact/OCR behavior; this change adds Queue Preflight review and candidate intent behavior.

## Impact

- Backend: canonical `ResolvedTrackPlan`, Batch Preflight candidate input/state/response, frozen track snapshots, and focused handler/resolver tests.
- Frontend: Batch Preflight API types, compact per-asset summary, decision expansion, candidate-only override editor, and focused component tests.
- No schema migration, dependency, media probe, or sidecar/OCR behavior change is required; candidate JSON state and accepted Queue snapshots remain additive and backward compatible.
