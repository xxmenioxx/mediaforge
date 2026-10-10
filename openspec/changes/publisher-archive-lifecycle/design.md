## Context

See `proposal.md` and `specs/publisher-archive-lifecycle/spec.md`. Publisher, QueueJob, and AssetRecord already persist publication, archive, and canonical asset-link evidence. Queue `status` completes independently; the `stage` is the pipeline timeline. Archive recovery uses Publisher leases, fencing generations, and persisted move intent.

## Goals / Non-Goals

**Goals:**

- Record stage transitions at the existing fenced Publisher and archive claim/outcome boundaries.
- Keep `QueueJob.status`, publication success, archive status, and canonical archived-original linkage independent.
- Make the existing active-work projection include a published job while original archival is pending or active, protecting rename and other existing mutations of that exact asset.
- Refresh Queue and Asset Info views only for nonterminal work and stop after a terminal archive outcome.

**Non-Goals:**

- No new lifecycle model, archive table, migration, polling service, name-based resolver, archive policy, or changes to publication recovery semantics.
- No attempt to infer or backfill stage history for existing jobs.
- No Queue status change, media operation, production-data mutation, or issue #312 work.

## Decisions

### Stage history follows persisted lifecycle claims

- In the transaction that acquires a Publisher attempt, append `publishing` to the owning QueueJob only after the fenced publication claim succeeds. Recovery claims use the same transition path.
- `commitPublishedPackage` commits publication and leaves the stage at `publishing`; it does not write `completed` while archival is pending.
- Acquire the original archive lease and persist the Queue stage `archiving_original` in one database transaction. Lease renewals do not create repeated stage events.
- In `recordArchiveOutcome`, append `completed` in the same transaction that accepts the fenced terminal archive outcome and synchronizes the canonical archive record. A stale owner cannot commit the stage transition.
- Keep `QueueJob.status` unchanged. Queue status is the overall Queue result; Publisher/archive progression is represented by `stage`, publication state, and archive status.
- Older rows are never backfilled. UI stage timestamps come only from `stageHistory`; unvisited stages are shown as `not recorded`. An active `PUBLISHING` or `archiving` state may mark its matching stage active without creating a timestamp.

### Archive evidence is published only after the move

`QueueJob.OriginalArchivedPath` and the archived `AssetRecord` remain unset until the actual move has completed and the existing archive outcome transaction succeeds. Publisher's existing fenced move-intent locator remains internal to recovery and SHALL NOT be treated as the Queue or Asset Info archive association. The existing lifecycle projection remains the only authority for linking archive evidence to a current Library asset.

### Active-work checks cover post-publication archival

The updated issue confirms the race on the converted Library path: manual Rename checked `QueueJob.MediaPath` (the RAW source), while Publisher continued archiving that frozen source after publication. Resolve the current `AssetRecord` before manual Rename and use the canonical active-work predicate so the exact `MediaPath` and `PublishedPath` identities are both considered, along with `SourceAssetRecordID` when available. Keep existing guards for queued/running work and maintenance. For a published Queue job, only archive `pending` or `archiving` temporarily blocks Rename; `complete`, `not_applicable`, and `failed` return to the existing terminal-state policy. Do not make converted provenance or Queue `completed` a permanent rename prohibition. Never match names.

The lifecycle projection SHALL return a specific temporary Rename capability reason for a canonically related active original archive. Asset Info shows that archival is in progress and becomes available again when its refreshed canonical projection observes a terminal state. The HTTP endpoint also rejects an active archive with `409` before moving the media or sidecars.

### Manual Rename preserves owned subtitle sidecars and current references

Continue using `buildManualAssetRenamePlan()`, `ownedSubtitleRenamePlans()`, and `applyLibraryRenamePlan()`. The existing subtitle discovery rule recognizes adjacent SRT, ASS, and SSA files with the exact media stem or a dot-delimited suffix; target names replace only the media stem and preserve language, role, track, and format suffixes. Preflight all media and subtitle destinations before moving anything, and reverse completed moves if a later move or database reconciliation fails. Reconcile the current `AssetRecord`, `ScanResult`, path-keyed overrides, profile assignments, and current Queue/DirectPublication paths. Leave immutable Publisher history and receipts, raw `MediaPath`, and `OriginalArchivedPath` unchanged.

NFO and artwork files are not included in this rename plan: the current code has no canonical sidecar ownership record for them. Do not infer ownership from a neighboring basename. A separate follow-up should define persisted or otherwise authoritative NFO/artwork association and add rollback-safe rename support.

### UI refresh is state-driven and targeted

Use one frontend predicate for existing queued/running work plus `publicationState=PUBLISHING` and archive `pending`/`archiving`. `AutoPublisher.tick()` selects `READY` publications for automatic work only when `pipelineAutomation.autoPublisherEnabled` is true; include `READY` in polling only when that setting is loaded and enabled. Queue jobs refresh at the existing Queue cadence; Assets and its lifecycle projection refresh while the same predicate is true. Terminal archive states stop both intervals. Keep archive state and failure summary visible in Job Details, Queue rows, and Asset Info's original-retention section. Existing `archivedOriginals` continues to render only canonical projection results.

### Stage timeline does not synthesize skipped history

Terminal Queue status alone does not prove that every unvisited stage was skipped. The Job Details timeline will use persisted events for timestamps and success, state fields for an active Publisher/archive stage, and `not recorded` for unvisited stages. For an active stage with no recorded end event, duration is measured to the present; a terminal event whose `finishedAt` predates it has no fabricated duration.

## Risks / Trade-offs

- [Risk] A transaction can fail after the filesystem move but before Queue/archive provenance commits → retain the existing Publisher locator, lease/fencing checks, and recovery path; keep QueueJob `OriginalArchivedPath` blank until recovery commits canonical outcome.
- [Risk] Expanding active-work detection could leave a rename unavailable if persisted archive state is stale → only `pending` and `archiving` for an exact published Queue job count; terminal outcomes release the guard.
- [Risk] Auto-publisher-ready jobs could appear stale if UI polling ignores the setting, while unconditional `READY` polling could run forever when AutoPublisher is off → consult the existing `pipelineAutomation` setting and poll `READY` only when enabled.
- [Risk] Loose NFO/artwork basename matching could rename another file → keep those files out of scope until canonical ownership is modeled; propose a separate follow-up.
- [Risk] Historical stage lists will show more `not recorded` states → this preserves evidence and avoids inventing prior events.
- [Risk] Polling adds bounded database/API requests on active long-running archives → poll only Queue and Assets lifecycle queries at their existing intervals and stop on terminal status.

## Migration Plan

No schema migration or data backfill. Deploy the code normally. New lifecycle transitions are recorded when new fenced claims/outcomes occur; a recovered pending archive records only the archive transition observed after deployment. Existing stage history remains unchanged. Rollback requires no data conversion because the existing fields and status values remain compatible.
