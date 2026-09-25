## Context

See `proposal.md` and `specs/assets/spec.md` for the problem and behavior
contract. The current implementation already has most of the canonical facts,
but exposes them through overlapping inventory buckets.

### Current authorities discovered

The current backend and frontend boundaries are:

| Concern | Existing authority/evidence | Current limitation to resolve |
| --- | --- | --- |
| Inventory record | `models.AssetRecord` with path, roots, relative/group paths, Source Group, Library, missing, and legacy status | One `status` is currently used for several dimensions |
| Source organization | `models.SourceGroup`, `AssetSourceGroup`, `AssetLogicalGroup`, and `AssetPath` | Source hierarchy is currently returned beside Library/status buckets |
| Destination organization | `models.Library` and `AssetRecord.LibraryID`/`LibraryName`/`RootPath`/`RelativePath` | Library grouping must stop borrowing RAW grouping assumptions |
| Queue intent/work | `models.QueueJob.Status`, `Stage`, `StageHistory`, output/published paths, timestamps, and `AssetMaintenanceOperation` | Active work is joined in UI-specific ways |
| Queue eligibility | `QueueHandler.resolveSelectedAssetsQueue` and its per-asset outcome/reason response | Assets UI must consume this rather than infer eligibility |
| Queue lifecycle | Queue claim/preparation/freeze semantics in `docs/specs/003-queue-claim-freeze/` and worker claim/prepare flow | Assets must not freeze or prepare jobs |
| Validation | `QueueJob.ValidationStatus`, score, report, and existing validation handlers | #53 owns the future Quality Gate |
| Converted publication | non-retired `QueueJob` records with `PublishedPath`, `PublishedAt`, and `PublicationRetiredAt` | Current Library AssetRecord does not alone identify its source job |
| Published as-is | `models.DirectPublication` with `SourcePath`, `PublishedPath`, `LibraryID`, `ReturnedAt` | Must remain provenance evidence, not a status bucket |
| Original retention | `QueueJob.OriginalArchivedPath`, archive AssetRecords, retention expiry, and existing archive/restore/delete guards | Archive is currently exposed as an inventory |
| Analysis | persisted `models.ScanResult`, snapshots, and existing analysis/snapshot endpoints | #52 owns the Analysis lifecycle redesign |
| Evidence/history | Queue jobs, execution plans, artifacts, reports, notes, logs, and maintenance operations | Reports currently appear as a top-level Assets tab |
| Rename/move | Library rename reconciliation updates current AssetRecord paths, scan paths, path-keyed state, active publication references, and DirectPublication paths | Historical/retired records must remain immutable |

The current `assetInventoryFromDB` reads persisted records and builds
`unprocessed`, `library`, `converted`, `unverified`, `accepted`, `archive`, and
`missing` lists. It also derives technical summaries from persisted ScanResult
and completed QueueJob evidence. Explicit inventory sync remains the place for
filesystem reconciliation; normal list/detail rendering must remain persisted
and bounded.

## Goals / Non-Goals

**Goals:**

- Introduce one backend-owned lifecycle projection consumed by RAW, Library,
  Asset Info, filters, counts, and bulk controls.
- Preserve current Queue, worker freeze, episode-order, publication, validation,
  archive, subtitle, LAB, and #18 rename authorities.
- Make current Library publication and RAW correlation deterministic and
  path-change-safe without making complete-media hashing the default.
- Land the navigation and detail migration incrementally with compatibility for
  legacy records and deep links.
- Keep list rendering safe for a resource-constrained NAS through batched,
  indexed persisted queries and lazy evidence loading.

**Non-Goals:**

- A lifecycle table, workflow engine, second Queue, or second episode-numbering
  implementation.
- Replacing `AssetRecord`, Queue, Publisher, Validation, Analysis, or the
  existing report/artifact storage model.
- Executing scans, FFmpeg, OCR, Validation, Publisher, or archive policy from
  list/detail rendering.
- Rewriting historical records when current Library names or paths change.

## Decisions

### 1. Canonical RAW versus Library distinction

`location` is a projection dimension with exactly two values:

```text
RAW     = source inventory under configured raw/source roots
Library = current inventory under configured Library destination roots
```

The projection resolver starts from `AssetRecord` and configured `Library`
roots/IDs. Existing status values remain compatibility input during migration,
but do not become the public authority. An archive AssetRecord is evidence for
the related Library asset when a relationship exists; it is not a third
location. A missing historical record is evidence/history, not a new location.

RAW retains Source Group → Logical Group → Path → Asset. Library grouping uses
the canonical destination Library identity and relative destination path. A
Source Group is never used as a Library parent unless an explicit persisted
mapping says so.

### 2. Lifecycle projection and dimension ownership

The backend will construct a read model conceptually shaped as:

```text
AssetLifecycleProjection {
    location
    lifecycle
    provenance
    validation
    publication
    original
    activeOperation
    attention
    currentPublication
    correlation
    evidenceSummary
    actions
}
```

The exact Go/JSON names may follow existing API conventions. The important
property is that each field is independently resolved and serializable without
mutating the underlying records.

Persisted inputs are:

- `AssetRecord` for current inventory identity, current path, roots, Library,
  source-group metadata, missing state, and retained compatibility status.
- `Library` and path/scope configuration for destination ownership and RAW
  inheritance.
- QueueJob status/stage/history, output and publication paths, validation
  fields, timestamps, archive path, and retirement fields.
- `DirectPublication` for active as-is publication and source/destination
  evidence.
- `AssetMaintenanceOperation` for active maintenance and failure evidence.
- `ScanResult` and current Analysis evidence for the Analysis integration slot.
- Existing artifacts, execution plans, logs, and reports for evidence/history.

Derived fields are location, lifecycle, provenance, normalized validation and
publication presentation, original retention presentation, active-operation
summary, attention reasons, group counts/badges, filters, and action
capabilities. No lifecycle transition is persisted separately by #21.

The resolver uses the existing canonical status/stage constants and maps only
where a public dimension needs a distinct name (for example existing
validation `warning` becomes `passed_with_warnings`). It must preserve
`unavailable`, `unknown`, and `pending` rather than manufacture a positive
state.

### 3. No new lifecycle table in the base change

A new lifecycle table is not necessary. Existing records already provide
current inventory, active work, validation, publication, original archive, and
history authority. A new table would create a second transition authority and
would require synchronization for every Queue, Publisher, Validation, rename,
restore, and retention path.

The implementation may add read indexes or a non-authoritative cache only if a
measured query plan requires it, but such a cache cannot own transitions or
replace canonical records. If correlation tests find an unresolvable case, the
smallest durable addition is a nullable immutable source/current-publication
reference on the existing publication/provenance record, with a compatibility
backfill only where the existing source and publication evidence is
deterministic. Do not add complete-media hashing as the default.

### 4. Current Library publication and RAW↔Library correlation

For converted work, the resolver matches a current Library `AssetRecord.Path`
to the active, non-retired QueueJob publication path and retains the QueueJob's
`MediaPath` as the source-side relationship. For published-as-is work, it
matches the current Library path to an active `DirectPublication.PublishedPath`
and uses `DirectPublication.SourcePath` as the source relationship. Current
selection is deterministic: active/non-retired evidence, then newest
publication timestamp and stable ID ordering. Retired jobs/publications are
history only.

`AssetRecord.SourcePath` remains source-group/path metadata and is not treated as
the sole publication join, because current published records can lack that
field. The correlation resolver must expose unresolved/legacy when neither
canonical relationship can prove the source.

The existing #18 rename/move reconciliation is the current-path authority. It
updates the current AssetRecord and ScanResult path, path-keyed overrides and
assignments, active QueueJob publication references, and active
DirectPublication paths. It intentionally does not rewrite retired QueueJob or
returned DirectPublication history. The lifecycle projection consumes that
reconciled current evidence; it does not implement a second rename path.

### 5. Converted versus published-as-is

Converted provenance is proven by the active/non-retired QueueJob/output
relationship and the existing MVForge output classification. Published-as-is
is proven by an active `DirectPublication` (and its existing publication mode),
not by `AssetRecord.Status == converted` or by a path shape. Accepted/imported
uses existing accepted/direct-ingest evidence. Any Library record that cannot
be proven remains legacy/unknown.

The Library list exposes these as filters/badges. It never removes converted
diagnostics, resolved plan, encoder/profile data, source relation, re-convert
action, or history when the Converted tab disappears.

### 6. Validation, publication, and original retention boundaries

The projection adapts existing evidence into separate fields:

```text
validation: unavailable | pending | passed | passed_with_warnings |
            review_required | failed
publication: unpublished | pending | published | blocked | failed
original: available | archive_pending | archived | restore_pending |
          restored | deletion_scheduled | deleted_by_retention |
          archive_failed | restore_failed | unknown
```

These are presentation values, not new transition enums. Unsupported states
remain unavailable/unknown until canonical records support them.

- #53 owns validation execution, review, retry, and the Quality Gate. #21
  reads/links its evidence.
- #54 owns publication/delivery rules and archive policy. #21 reads current
  destination, publication timestamp, integration evidence, and original
  outcome, and invokes canonical actions only where APIs exist.
- Existing publisher/archive path containment and safety checks remain in force.
  #21 does not infer that publication succeeded because a file exists.

### 7. Mixed groups are summaries, not state machines

The backend returns group members plus deterministic summary counts/badges. It
does not set `group.lifecycle` or `group.provenance` from a first/majority
member. If a compact group label is needed, it uses neutral counts such as
`2 Published · 1 Queued · 1 Needs review · 1 Failed` and exposes the member
projections as the authority.

RAW grouping reuses Source Group/Logical Group/Path. Library grouping is based
on Library ID and canonical destination relative/group path. Group summaries
must be computed in the same batch query/resolver pass as their members to
avoid frontend joins and N+1 detail calls.

### 8. Queue Remaining eligibility ownership

Queue remains the sole owner of eligibility. The Assets surface may request a
preview/commit for all members of a logical group, but it delegates per-asset
resolution to the existing `QueueSelectedAssets` contract and its effective
configuration, review, missing, open-job, maintenance, destination, and
operation checks. The response continues to distinguish eligible, queued,
skipped, failed, and bounded reasons.

Queue Remaining is therefore a selection policy over a complete group, not a
new queue engine. Already published/current Library members are skipped by
canonical publication/eligibility evidence; eligible RAW members are passed to
the existing queue intent path. Commit remains lightweight and does not probe,
freeze, or analyze every selected asset.

### 9. Episode ordinal ownership

Episode ordinals remain owned by the existing complete-inventory ordering and
multi-episode naming helpers used by Queue/worker and direct-publication
planning. The Assets redesign must call those authorities or expose their
result; it must not renumber selected rows in TypeScript or introduce an Assets
ordinal algorithm.

The complete set of records in the relevant source/group determines positions.
Filtering or selecting only E03/E04 cannot compact them to E01/E02. Existing
extras/additional-material path policy remains separate and does not consume
normal episode positions.

### 10. Backend action-capability projection

The canonical detail/list response will include or link an
`AssetActionCapabilities` projection conceptually shaped as:

```text
actions {
    rename, move, queue, reconvert, lab, refreshAnalysis,
    manageTracks, manageSubtitles, manageAttachments,
    cancelWork, reviewValidation, retryValidation,
    publish, retryPublication, archive, restore, deleteArchivedOriginal,
    viewReports, viewHistory
}
```

Each action has `allowed` plus a bounded stable reason when it is not allowed.
The backend resolves active work, location, missing state, provenance,
validation/publication evidence, root containment, and current API support.
The frontend may hide or disable actions based on this projection but must not
reimplement safety/eligibility rules.

The projection does not grant new capabilities to legacy/unknown records. It
fails closed for destructive or recovery actions when identity, roots, or
canonical evidence are insufficient.

### 11. Asset Info loading and section boundaries

Asset Info is split into a persisted summary and explicit lazy sections:

| Section | Normal load | Authority/evidence |
| --- | --- | --- |
| Overview | Immediate, lightweight | lifecycle projection, AssetRecord, current Library/publication summary |
| Analysis / Advisor | Lazy | #52 slot plus persisted ScanResult/advisor evidence |
| Tracks / Subtitles / Attachments | Lazy | existing snapshot/sidecar/maintenance APIs |
| Conversion | Lazy or existing persisted summary | Queue plan, profile/override, output/result artifacts |
| Validation | Lazy | #53/current Queue validation evidence |
| Publication | Lazy | #54/current QueueJob/DirectPublication/integration evidence |
| Original | Lazy | archive records, original path/retention evidence, canonical archive APIs |
| Evidence / History | Lazy and paginated/bounded | jobs, execution plans, artifacts, reports, logs, revisions |

Opening Overview must not enable snapshot/scan work. Existing heavy controls
remain explicit actions. Asset Info should reuse current dialog geometry and
components where possible, moving only ownership/section loading boundaries.

### 12. Reports, revisions, and current versus historical records

No report storage is added. Existing `JobArtifactsResponse`, QueueJob artifact
paths/reports, validation reports, logs, snapshots, and execution plans are
linked from the asset evidence/history sections. List rows carry summaries and
IDs/links only; report bodies remain on demand.

Current publication is the active non-retired publication selected by the
correlation resolver. Retired QueueJobs and returned DirectPublications remain
immutable historical revisions. Re-conversion creates a new execution and
publication revision; it does not mutate the old result to make it appear
current. A current AssetRecord path/name change updates only current
references, consistent with #18.

### 13. Navigation and compatibility routes

The final Assets navigation is:

```text
RAW | Library
```

Converted becomes a Library provenance filter. Archive becomes Original
retention/evidence under related Library assets. Reports become evidence,
history, dashboard counts, and lazy links.

Existing routes/deep links are handled in this order:

1. Redirect Converted to Library with a provenance filter when the route can be
   mapped safely.
2. Redirect Archive to Library with an original-retention filter or selected
   asset when identity is available.
3. Redirect Reports to the Library dashboard/evidence surface when no asset is
   selected; retain a compatibility report view if an old report link cannot
   be mapped without losing context.
4. Return an explicit not-found/compatibility message only when no safe mapping
   exists.

No route migration deletes underlying data or silently changes provenance.

### 14. Dashboard/count migration

Replace category-derived counts with projection queries/aggregates for RAW,
Library, active processing, needs review, validation review, publication
blocked, converted provenance, and archived originals. Missing/historical path
counts remain attention evidence. Counts must use the same backend filters and
correlation rules as list rows so a Library item is not counted as both a
second location and an archive inventory.

Existing space-saved conversion metrics remain evidence/summary data and are
not used to classify provenance by themselves.

### 15. Legacy compatibility and migration strategy

The initial rollout is a compatibility projection over existing rows. It does
not delete, relocate, rewrite, or require a complete data migration. Deterministic
backfill is limited to derived/read-only fields or a narrowly scoped durable
reference if correlation tests prove one is required. Ambiguous legacy rows
remain visible as legacy/unknown.

Stale Source Groups remain available for RAW provenance/navigation where their
records still point to them, but they do not become Library organization.
Existing reports, jobs, logs, snapshots, archived files, and sidecars remain
at their current storage locations. Rollback can restore the old navigation
projection while leaving canonical records untouched.

### 16. Query and index strategy for NAS performance

The list resolver will batch-load AssetRecords and related evidence into maps
keyed by stable IDs and normalized paths. It will not issue one query per asset
or load report bodies for rows. Query predicates should use existing indexes on
AssetRecord path/status/library/source-group, QueueJob status/stage/publication
retirement/validation/original fields, DirectPublication source/published/
returned fields, and maintenance status/path. Composite indexes are added only
after checking the actual query plans and migration compatibility.

Normal `GET /api/assets`-style reads use persisted inventory and bounded
aggregates. Explicit Sync remains the filesystem reconciliation operation. A
detail request may load one selected ScanResult or report body after the user
asks for it; it must not turn into a list-wide scan.

### 17. Integration contracts for #52, #53, and #54

The ownership boundary is explicit:

```text
#21 owns: inventory/navigation, lifecycle projection, mixed-state presentation,
         Asset Info aggregation/control surface, action eligibility,
         RAW/Library correlation, filters, history projection

#52 owns: Analysis lifecycle and Analysis UI redesign
#53 owns: Validation Quality Gate / Review Center
#54 owns: Publisher/delivery/archive policy and redesign
```

#21 consumes #52 analysis readiness/evidence, #53 validation state/evidence,
and #54 publication/delivery/original policy through adapters or existing
endpoints. Until those issues land, current ScanResult, QueueJob validation,
DirectPublication, archive records, and publisher evidence remain the fallback
canonical sources. No placeholder may execute work or claim a stronger state.

### 18. Implementation phases and reversibility

The change is delivered in bounded phases:

- Phase A: resolver/projection contracts, correlation, action capabilities,
  backend tests, query shape, and compatibility output.
- Phase B: RAW/Library navigation, mixed groups, filters, bulk eligibility,
  Queue Remaining facade, and dashboard/count adapters.
- Phase C: Asset Info Overview and lazy lifecycle/evidence sections, retaining
  existing controls and current/historical distinction.
- Phase D: legacy/deep-link compatibility, performance/index tuning, and
  frontend regression cleanup.

Each phase can be guarded by the response/navigation compatibility layer. A
rollback restores the previous UI projection and routes without rolling back
Queue jobs, publications, reports, paths, or media.

## Risks / Trade-offs

- **Risk:** Existing `AssetRecord.Status` and current UI lists contain useful
  compatibility behavior that may be lost during a projection rewrite.  
  **Mitigation:** keep status as migration input, compare old/new fixtures, and
  do not remove underlying lists or records until the replacement projections
  have focused coverage.

- **Risk:** A current Library path may not be provably related to a RAW source
  in old installations.  
  **Mitigation:** expose unresolved legacy correlation and fail closed; add only
  the smallest durable publication reference if deterministic joins are proven
  insufficient.

- **Risk:** Existing list construction loads broad persisted sets and can create
  slow joins on a NAS.  
  **Mitigation:** batch maps, bounded aggregates, pagination/lazy sections,
  explain-plan checks, and no filesystem/media work on list render.

- **Risk:** Group badges could be mistaken for authoritative state.  
  **Mitigation:** use neutral count summaries and always render member-level
  lifecycle/provenance/action evidence.

- **Risk:** #52/#53/#54 may evolve their response shapes after #21 lands.  
  **Mitigation:** isolate adapters, preserve unavailable/pending values, and
  consume canonical fields without duplicating their state machines.

- **Risk:** Removing old tabs could strand bookmarks or operational workflows.
  **Mitigation:** implement explicit redirects/compatibility views and test old
  route behavior before removing top-level navigation.

## Migration Plan

1. Add projection and compatibility response types without changing media or
   persisted lifecycle transitions.
2. Add backend resolver tests using existing AssetRecord, QueueJob,
   DirectPublication, scan, maintenance, archive, and legacy fixtures.
3. Expose the projection through the bounded inventory/detail boundary and keep
   old fields/routes during the compatibility window.
4. Switch navigation and dashboard/filter consumers to RAW/Library while
   preserving Converted/Archive/Reports links as redirects or compatibility
   views.
5. Migrate Asset Info to persisted-only Overview and lazy sections, then remove
   obsolete top-level contexts after route coverage passes.
6. Add measured query/index improvements and verify no N+1 or filesystem/media
   work occurs on normal list/detail rendering.
7. Rollback, if required, by restoring the previous navigation/projection
   consumer; do not undo or delete canonical jobs, publications, evidence,
   originals, sidecars, or historical records.

## Explicit Design Questions

1. **Canonical RAW/Library distinction:** location is source-root RAW versus configured destination-root Library; stages and evidence are not locations.
2. **Current Library publication:** active non-retired QueueJob publication or active DirectPublication, selected deterministically by current path and publication evidence.
3. **Correlation across rename/move:** #18 reconciliation updates current AssetRecord and active publication references; historical rows remain unchanged.
4. **Persisted versus derived:** persisted canonical records hold facts; the lifecycle projection, counts, badges, filters, attention, and actions are derived.
5. **New lifecycle table:** no; it would duplicate transitions. Add only a minimal publication reference if deterministic correlation tests require it.
6. **Mixed groups:** member projections remain authoritative; groups expose deterministic counts/badges only.
7. **Queue Remaining eligibility:** QueueHandler's existing per-asset eligibility contract.
8. **Episode ordinals:** complete-inventory ordering and existing worker/direct-publication naming authorities.
9. **Converted versus published-as-is:** active QueueJob/output evidence versus active DirectPublication evidence; no path/status inference.
10. **Archived originals:** QueueJob `OriginalArchivedPath` plus archive AssetRecord/retention evidence, linked through current publication correlation.
11. **Legacy assets:** visible in Library with legacy/unknown provenance and unresolved correlation where necessary.
12. **Action eligibility:** backend `AssetActionCapabilities` projection with bounded reasons.
13. **Asset Info loading:** persisted Overview immediately; Analysis, reports, scans, subtitles, validation, publication, original, and history sections lazy.
14. **Reports/history:** existing artifact/report/job/plan/log records, linked and lazy; no new store.
15. **Revisions/current publication:** active non-retired publication is current; retired jobs/publications are immutable history.
16. **Old routes:** redirect to a safe RAW/Library filter or compatibility view; not-found only when mapping is unsafe.
17. **Dashboard counts:** projection-based persisted aggregates replace category-tab counts while retaining useful attention/provenance visibility.
18. **Query/index strategy:** batch maps, indexed identity/status/path predicates, bounded list/detail responses, and no list-wide media/filesystem work.
19. **#52/#53/#54 consumption:** adapters consume their evidence; #21 owns no analysis, validation, publisher, archive-policy, or workflow engine.
20. **Incremental phases:** projection/correlation, navigation/list, Asset Info, then compatibility/performance, each reversible at the consumer layer.
