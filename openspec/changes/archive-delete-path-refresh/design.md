## Context

See proposal.md for the problem and scope. Archive deletion must keep its current identity, active-work, root, symlink, and pre-unlink checks. Queue publication and archived-original linkage already have one canonical resolver in AssetLifecycleProjection; this change bounds the resolver inputs for deletion rather than adding a second relationship policy.

## Goals / Non-Goals

**Goals:**

- Keep Archive deletion queries proportional to the records related to the requested original and its current Library publication.
- Preserve fresh identity and filesystem fencing immediately before unlink.
- Make the Publisher-confirmed Library path visible after publication while keeping original/archive provenance separate.
- Measure database/projection, filesystem, unlink, persistence, refresh, and feedback time independently.

**Non-Goals:**

- Do not make permanent deletion asynchronous or parallel.
- Do not change Publisher identity, replacement, sidecar, or delivery gates.
- Do not delete Library media, Queue history, Validation evidence, reports, logs, snapshots, or publication provenance.
- Do not claim NAS latency improvement from an in-memory benchmark.

## Decisions

1. **Bound query inputs, reuse the canonical projection.** The Archive deletion adapter loads the requested Library record, Queue and direct-publication rows associated by source identity or published path, related source records, and archive records referenced by those rows. It passes that bounded set to ResolveAssetLifecycleProjection. Batch owner resolution begins with QueueJob.OriginalArchivedPath and follows the published paths to Library records. The ordinary Assets inventory continues using its existing batch projection path.
   - **Alternative considered:** recompute projections from every AssetRecord for each delete. This repeats work proportional to the full catalog for a single item and multiplies that cost for a batch.
   - **Alternative considered:** infer Archive ownership from filenames or paths alone. This cannot resolve current publication identity safely.

2. **Keep two fresh identity validations.** The first check resolves the current Archive, source, Queue publication, and Library relationship. Immediately before unlink, the handler resolves them again, rechecks active work and configured roots, and validates both the Archive target and protected Library file. The final check remains fresh even though the first check's data was already loaded.
   - **Alternative considered:** reuse the first projection through unlink. That could miss a concurrent publication, rename, restore, or active-work change.
   - **Alternative considered:** remove one of the two validations for speed. That would weaken the TOCTOU fence required for destructive work.

3. **Index the relationship lookups without changing stored identity.** GORM indexes QueueJob.PublishedPath and QueueJob.OriginalArchivedPath, which are used to find current Library publications and archived-original owners. Existing records remain intact; startup migration creates missing indexes without rewriting publication or AssetRecord data.
   - **Alternative considered:** add a cache of Archive ownership. Cached identity could become stale between validation and unlink.

4. **Use Publisher state as the path authority and invalidate the existing Assets query.** Backend inventory continues to derive the Library asset from its current Publisher/Queue projection. The frontend observes already-loaded Queue jobs and invalidates Assets when a job newly becomes PUBLISHED or its current publication identity/path changes. It does not infer a Library path from the raw or archived original.
   - **Alternative considered:** maintain a frontend-only published-path mapping. That would duplicate backend identity and become stale across replace, restart, or conflict cases.

5. **Record bounded timing data at the existing operation boundaries.** Backend timing events separate identity, active-work, filesystem, unlink, inventory persistence, and total time; batch events include counts and aggregate durations. Frontend performance measures separate request, Assets refresh, and feedback rendering, retaining a bounded rolling set of measures per phase. Timing events do not add full media paths.
   - **Alternative considered:** report only total endpoint duration. That cannot distinguish catalog/query work from NAS filesystem latency or UI refresh time.

## Risks / Trade-offs

- [The indexed OR query may still return many historical publications for a heavily replaced asset] → Filter results through the canonical projection and current-publication rules; benchmark representative catalog and replacement counts.
- [SQLite indexes consume storage and add write cost] → Limit indexes to the two relationship lookup fields and leave existing rows and IDs unchanged.
- [The in-memory benchmark does not represent NAS filesystem behavior] → Keep phase timings for actual operations and report database, filesystem, unlink, and frontend refresh separately.
- [A publication may change after the final identity check] → Keep the final identity, active-work, root, and filesystem checks immediately before synchronous unlink; unresolved cases remain fail-closed.

## Migration Plan

No row backfill or media migration is required. GORM AutoMigrate adds the missing QueueJob indexes. Rollback can revert the code; any remaining unused SQLite indexes are harmless and may be removed by a later explicit migration if storage evidence warrants it. No NAS or production database operation is part of this change.

## Open Questions

None.
