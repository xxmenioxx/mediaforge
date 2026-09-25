## Why

Assets currently presents several overlapping buckets as if they were inventory
locations: Unprocessed, Library, Converted, Archive, and Reports. The backend
already has the facts needed to distinguish source inventory, current Library
records, Queue work, validation, publication, original retention, and history,
but the current API and UI flatten those facts into `Asset.status` and separate
lists. This makes mixed groups, legacy records, Queue Remaining, rename/move
provenance, and per-asset actions difficult to explain safely.

Issue #21 establishes RAW and Library as the only canonical inventory locations
and exposes the other facts as independent lifecycle dimensions and evidence.

## What Changes

- Introduce an authoritative backend asset lifecycle projection over existing
  `AssetRecord`, `QueueJob`, publication, scan/analysis, maintenance, and
  archive evidence.
- Separate `location`, `lifecycle`, `provenance`, `validation`, `publication`,
  `original retention`, active operation, attention, and historical evidence;
  do not replace them with a second persisted workflow state machine.
- Correct DirectPublication semantics: an active/non-returned
  `DirectPublication` proves `published_as_is`; any `publicationMode: as_is`
  value is derived presentation only, with no persisted mode field.
- Make RAW the source-only inventory, preserving Source Group → Logical Group →
  Path → Asset organization and RAW-specific inheritance and Queue eligibility.
- Make Library the destination inventory for converted, published-as-is,
  imported/accepted, and legacy/unknown assets; remove Converted, Archive, and
  Reports as top-level inventory contexts without deleting their domain data.
- Add backend-authoritative mixed-group summaries, persisted-dimension filters,
  per-asset bulk action eligibility, and separate historical source provenance
  from optional current RAW correlation/current publication references.
- Preserve Queue Remaining and existing episode ordinal authorities: Queue
  Remaining delegates to `QueueSelectedAssets`, eligibility is resolved per
  asset, while complete inventory determines ordinals and extras do not consume
  normal episode positions.
- Recast Asset Info as the canonical per-asset lifecycle control surface with a
  persisted-only Overview and lazy Analysis, Tracks, Subtitles, Attachments,
  Conversion, Validation, Publication, Original, Evidence, and History sections.
- Expose backend-generated but non-authoritative action capabilities so frontend
  controls do not infer safety from badges or duplicate Queue, Validation,
  Publisher, archive, or rename policy; mutation endpoints revalidate their
  canonical guards at execution time.
- Migrate dashboard counts, filters, deep links, legacy compatibility, and list
  query/index strategy for NAS-safe bounded rendering without ffprobe, FFmpeg,
  full filesystem walks, analysis, validation, or report-body fan-out.

## Capabilities

### New Capabilities

- `assets`: Canonical RAW/Library inventory, independent lifecycle projection,
  mixed-state grouping, Asset Info aggregation, action eligibility, history,
  correlation, compatibility, and performance behavior.

### Modified Capabilities

None. Existing Queue, rename, subtitle, and asset-default behavior remain
authorities consumed by this capability; no existing OpenSpec capability has a
matching asset lifecycle contract.

## Impact

- Backend asset inventory response/types, lifecycle projection and action
  capability projection, bulk eligibility, QueueSelectedAssets adapters,
  publication/correlation queries, dashboard aggregates, and focused
  migrations/indexes if implementation evidence requires them.
- Frontend Assets navigation/list/filter/group projections and Asset Info lazy
  sections, while retaining existing track, subtitle, LAB, Test Encode, rename,
  Queue, report, archive, and restore operations behind their canonical APIs.
- Compatibility handling for existing Converted, Archive, Reports, and deep-link
  routes; no destructive data migration or report-storage rewrite.
- Integration slots for Analysis (#52), Validation (#53), and Publisher (#54).
  Those issues retain ownership of their redesigns and state machines.

Out of scope: implementing #52 Analysis, #53 Validation Quality Gate, #54
Publisher/delivery redesign, replacing Queue or episode numbering, creating a
new workflow engine or report store, full rescans on render, automatic original
deletion, fabricated legacy provenance, or making Library mirror RAW Source
Groups.
