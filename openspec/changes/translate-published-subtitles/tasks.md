## 1. Backend Translation Guard

- [x] 1.1 Reuse the canonical subtitle lifecycle action in capability and start source resolution; preserve exact-source, root, archive, and revision guards, and verify active Queue/maintenance work returns a concrete blocker.
- [x] 1.2 Add focused backend coverage for published lifecycle variants, active work, missing/unsupported/archive sources, provider readiness reasons, and exact sidecar identity; verify the translation source/capability tests pass.
- [x] 1.3 Keep the generic `Subtitles` action capability unchanged for publication conflicts and add a Translation-only guard for conflicting or unresolved publication identity/provenance across Capability, Start, Retry, and the publication barrier.
- [x] 1.4 Replace Translation's broad lifecycle history loads with indexed per-asset publication, maintenance, and referenced-source queries that reuse `ResolveAssetLifecycleProjection`.
- [x] 1.5 Add high-history regressions proving unrelated records are not loaded and relevant active Queue work and Queue/DirectPublication conflicts remain visible.
- [x] 1.6 Add a published QueueJob end-to-end Translation regression and assert video, original sidecar, and Publisher evidence remain unchanged; retain Accepted and DirectPublication fixtures.
- [x] 1.7 Fail closed when the bounded publication candidate set exceeds its safety cap.
- [x] 1.8 Restore unprocessed RAW assets without publication identity; add Capability → Start → Completed coverage and retain Queue, maintenance, missing-file, source-revision, and archive blockers.
- [x] 1.9 Replace suffix-expression publication lookups with indexed canonical path keys; backfill historical rows idempotently and validate publication/source lookup plans with SQLite `EXPLAIN QUERY PLAN`.
- [x] 1.10 Allow Translation for an identity-verified converted Library publication whose QueueJob records an archived original; retain physical Archive and unauthorized-path blockers and prove Capability → Start → Completed leaves all original and Publisher evidence unchanged.

## 2. Asset Info Translation Controls

- [x] 2.1 Remove only the general row editing lock from embedded/external Translate controls, preserve lifecycle action gating and adjacent mutation guards, and expose a specific reason when translation is lifecycle-blocked; verify Asset Info integration tests across converted, published-as-is, accepted, and active states.
- [x] 2.2 Verify provider-unavailable and source-ineligible capability responses appear inside the dialog with Start disabled, while exact external and embedded source selection remains unchanged.

## 3. Validation

- [x] 3.1 Run issue-specific UI and backend tests, then run `./scripts/verify.sh --auto` through the serialized shared-validation lock; verify the canonical output reports `ALL CHECKS PASSED`.
- [x] 3.2 Re-run focused Translation and lifecycle tests plus strict OpenSpec validation, then run `./scripts/verify.sh --auto` and update only the hardening tasks completed here.
