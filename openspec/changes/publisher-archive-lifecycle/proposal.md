## Why

Publisher can commit publication and complete the Queue job while original archival is still pending or running. The stage history and UI currently hide that work, and Assets may stop refreshing before the canonical archived-original record appears.

## What Changes

- Persist real `publishing`, `archiving_original`, and `completed` Queue stage transitions at their fenced lifecycle boundaries.
- Keep archive status separate from `QueueJob.status`, and display pending, archiving, complete, failed, and not-applicable states with the available failure summary.
- Refresh Queue jobs and Assets only while Queue, Publisher, or original archival work is nonterminal, then stop polling. A `READY` publication is polled only when `pipelineAutomation.autoPublisherEnabled` is true, because AutoPublisher claims it automatically only in that mode.
- Keep Asset Info bound to the existing canonical archived-original projection and refresh it after the archive record commits.
- Temporarily block Rename only while the exact canonical published Queue job has a pending or active original archive; allow converted Library assets to be renamed again after `complete`, `not_applicable`, or another existing terminal-state policy. Preserve owned subtitle sidecars and rollback behavior.
- Keep historical stage history evidence-only; do not backfill events or associate files by name.

## Capabilities

### New Capabilities

- `publisher-archive-lifecycle`: truthful Publisher/archive timeline, separate archive status, bounded refresh, and safe Asset Info/Rename behavior during archival.

### Modified Capabilities

None. This change adds the missing archive operation lifecycle behavior around existing Publisher and Asset lifecycle authorities; it does not change publication policy or the canonical archived-original association contract.

## Impact

- Backend: Publisher claim/commit/outcome transitions, Queue active-work checks for both source and published paths, asset lifecycle projection, manual Rename state reconciliation, owned subtitle sidecars, and focused lifecycle/recovery/Rename tests.
- Frontend: Queue and Assets query refresh, Queue/Asset Info archive status, and Job Details stage history rendering.
- Persistence: no new model or migration. Existing QueueJob, PublisherPublication, and AssetRecord fields remain authoritative.
- Out of scope: issue #312, changing the meaning of `QueueJob.status=completed`, publication policy, archive retention, historical backfills, name-based association, and any media or production data changes. NFO and artwork rename ownership remains unsupported because the current asset model has no canonical association for those files; track that as a separate follow-up rather than inferring ownership from adjacent names.
