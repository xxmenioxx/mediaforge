## Purpose

Defines the canonical RAW and Library asset inventories and the independent
lifecycle, provenance, validation, publication, original-retention, evidence,
and action projections that let users operate mixed and legacy media safely.

## ADDED Requirements

### Requirement: Assets exposes only RAW and Library as inventory locations

The asset inventory SHALL expose exactly two canonical locations: `RAW` for
source inventory and `Library` for configured destination inventory. Workflow
stages, Converted, Archive, Reports, and active operations MUST NOT be modeled
as additional inventory locations.

#### Scenario: A converted published asset is listed

- **WHEN** an asset was converted, validated, published, and its original was archived
- **THEN** it appears in `Library` with those facts represented as independent dimensions and does not appear in a Converted or Archive location

#### Scenario: A source asset is listed

- **WHEN** an asset is only present in configured source inventory
- **THEN** it appears in `RAW` and not in `Library` merely because it has Queue or analysis evidence

### Requirement: The backend provides an independent lifecycle projection

For every inventory asset, the backend SHALL provide a lifecycle projection
whose dimensions remain independently addressable: location, lifecycle,
provenance, validation, publication, original retention, active operation,
attention, current publication/correlation, and evidence/history references.
The projection MUST NOT collapse these dimensions into the existing ambiguous
asset status value.

The projection SHALL use existing canonical values and evidence where they
exist. It SHALL represent unknown or unavailable evidence explicitly rather
than infer a stronger state from a path, badge, or aggregate.

#### Scenario: Independent facts coexist

- **WHEN** a Library asset is converted, published, validation-passed, and its original is archived
- **THEN** the projection reports `Library`, `converted`, validation passed, publication published, and original archived simultaneously

#### Scenario: Evidence is absent

- **WHEN** no canonical record proves a provenance, validation, publication, or original-retention fact
- **THEN** the corresponding dimension is `unknown` or `unavailable` and the system does not infer it from a filename or filesystem shape

### Requirement: Lifecycle states are derived from canonical work and evidence

The lifecycle dimension SHALL expose the existing effective work state using
the smallest compatible set of values, including available, queued, preparing,
processing, validating, publishing, published, failed, and needs review where
the canonical records support them. Active Queue and maintenance work SHALL be
reported as `activeOperation` evidence as well as contributing to lifecycle
presentation; a completed historical job MUST NOT remain active solely because
it exists.

#### Scenario: Queue claim state is visible

- **WHEN** a Queue job has been queued or claimed/preparing for an asset
- **THEN** the asset lifecycle reports queued or preparing and identifies the active operation without changing its inventory location

#### Scenario: A failed job is visible without erasing history

- **WHEN** the latest applicable operation failed
- **THEN** lifecycle reports failed, includes a bounded attention reason, and preserves the failed execution in history

### Requirement: Provenance distinguishes converted, as-is, accepted, and legacy

The provenance dimension SHALL distinguish at least `converted`,
`published_as_is`, `imported_or_accepted`, and `legacy_or_unknown` when the
canonical evidence supports those distinctions. A Library location alone MUST
NOT imply converted provenance.

#### Scenario: Published-as-is is not converted

- **WHEN** an active direct publication record proves an asset was published without conversion
- **THEN** provenance is `published_as_is` and not `converted`

#### Scenario: Legacy provenance is inconclusive

- **WHEN** a Library record has no deterministic conversion or direct-publication evidence
- **THEN** provenance is `legacy_or_unknown` and remains visible in Library

### Requirement: DirectPublication proves as-is provenance without a mode field

An active/non-returned `DirectPublication` whose current destination identity
matches a Library asset SHALL prove `published_as_is` provenance. The projection
MAY expose a presentation value such as `publicationMode: as_is`, but that value
MUST be derived from the DirectPublication evidence; the system MUST NOT add or
require a persisted DirectPublication publication-mode field.

#### Scenario: DirectPublication-only asset

- **WHEN** an active DirectPublication matches the current Library destination
- **THEN** the asset is projected as `published_as_is` and any `as_is` mode label is derived rather than persisted

#### Scenario: Returned DirectPublication is historical

- **WHEN** a DirectPublication has `ReturnedAt` set and no active publication authority proves the same destination
- **THEN** it remains history and cannot by itself prove current as-is provenance

### Requirement: Validation and publication remain projections of their authorities

Validation SHALL be projected as unavailable, pending, passed,
passed-with-warnings, review-required, or failed according to canonical
validation evidence. Publication SHALL be projected as unpublished, pending,
published, blocked, or failed according to canonical Publisher/publication
evidence. Assets SHALL NOT execute validation or publication work merely because
the projection or Asset Info is rendered.

Issue #53 remains the authority for Validation Quality Gate behavior and issue
#54 remains the authority for Publisher/delivery behavior. This capability may
link to or invoke those authorities but SHALL NOT create parallel validators or
publishers.

For current-publication selection, a valid converted publication is an
active/non-retired QueueJob publication whose current destination identity
matches the Library asset. A valid as-is publication is an active/non-returned
DirectPublication with that match. Ordering MAY be deterministic within one
publication authority using its active state, publication timestamp, and
same-table stable ID. Across QueueJob and DirectPublication, the system MUST
NOT compare unrelated IDs, `UpdatedAt`, or status specificity. If both claim
the same current Library asset without canonical supersession/history proving
which is current, the projection SHALL report an unresolved/conflict current
publication, `needs_review` attention, and `legacy_or_unknown` or explicit
conflict provenance; destructive and recovery actions SHALL fail closed.

#### Scenario: Validation evidence is unavailable

- **WHEN** an asset has no canonical validation result
- **THEN** Asset Info shows validation unavailable or pending according to the available canonical evidence and does not start validation

#### Scenario: Publication is blocked

- **WHEN** canonical publication evidence reports a blocked or failed delivery
- **THEN** the projection exposes that publication state and its bounded reason without converting it into a generic lifecycle status

#### Scenario: QueueJob is the only current publication

- **WHEN** one active/non-retired QueueJob publication matches the current Library destination and no active DirectPublication matches
- **THEN** the current publication is the QueueJob and provenance is converted

#### Scenario: DirectPublication is the only current publication

- **WHEN** one active/non-returned DirectPublication matches the current Library destination and no active QueueJob publication matches
- **THEN** the current publication is the DirectPublication and provenance is published-as-is

#### Scenario: Retired QueueJob and active DirectPublication

- **WHEN** a retired QueueJob and an active/non-returned DirectPublication reference the same current Library destination
- **THEN** the DirectPublication is current as-is and the retired QueueJob remains history

#### Scenario: Active QueueJob and returned DirectPublication

- **WHEN** an active/non-retired QueueJob matches the current Library destination and the DirectPublication for that destination has returned
- **THEN** the QueueJob is current converted and the returned DirectPublication remains history

#### Scenario: Conflicting active publication authorities

- **WHEN** an active/non-retired QueueJob and an active/non-returned DirectPublication independently claim the same current Library destination without canonical supersession
- **THEN** the result is unresolved/conflict with needs-review attention, no arbitrary winner is selected, and destructive/recovery actions fail closed

#### Scenario: One publication is provably current among historical revisions

- **WHEN** multiple historical revisions exist and one active publication is identified by its authority's current/non-retired relationship
- **THEN** that publication is current while all other revisions remain immutable history

### Requirement: Original retention is independent and safety-bounded

Original retention SHALL be projected separately from Library location and
publication. It MAY expose available, archive pending, archived, restore
pending, restored, deletion scheduled, deleted by retention, archive failed,
restore failed, or unknown only when the existing domain evidence supports the
state. Archive and restore/delete actions SHALL be backend-authorized, auditable,
restricted to configured MediaForge roots, and MUST never target or delete the
current Library publication.

#### Scenario: Archived original remains related to Library

- **WHEN** a completed validated publication has an `originalArchivedPath`
- **THEN** the Library asset exposes the archived original, its metadata, and its relationship to the current publication while the original is not a second inventory location

#### Scenario: Unsafe original deletion is rejected

- **WHEN** a delete-original request resolves outside the configured archive root, resolves to the current Library file, or lacks canonical provenance
- **THEN** the backend rejects it without deleting media or altering immutable history

### Requirement: RAW preserves source grouping and source-only inheritance

RAW SHALL preserve the existing Source Group → Logical Group → Path → Asset
organization. Source Group and related profile/default inheritance, source
metadata, analysis availability, review requirements, selection, and Queue
eligibility SHALL remain RAW responsibilities. Library assets MUST NOT inherit
their organization or destination grouping from RAW Source Groups unless an
explicit canonical Library mapping exists.

#### Scenario: RAW hierarchy is shown

- **WHEN** a source group contains multiple logical titles and paths
- **THEN** RAW exposes that hierarchy and permits selection/configuration using existing source-group authority

#### Scenario: Library grouping is independent

- **WHEN** a converted asset originated in a RAW Source Group but is now under a configured Library destination
- **THEN** Library groups it by canonical Library destination organization, not by the RAW Source Group hierarchy

### Requirement: Library contains all destination provenance classes

Library SHALL include converted, published-as-is, imported/accepted, and
legacy/unknown assets that are present in configured destination Libraries.
Converted SHALL be a provenance filter or badge, not a top-level location.
Archive and Reports SHALL be evidence or views associated with Library assets,
not inventory roots.

#### Scenario: Mixed provenance Library is visible

- **WHEN** one destination contains a converted asset, an as-is publication, an accepted import, and a legacy record
- **THEN** all four assets are visible in Library and can be filtered by provenance without moving any asset to another inventory

### Requirement: Mixed groups retain per-asset authority

Group summaries SHALL expose deterministic counts and badges derived from their
members, but a group MUST NOT acquire a lifecycle or provenance authority from
its first member, majority member, or arbitrary aggregate. Members in one group
MUST retain independent projections and action eligibility.

#### Scenario: A season contains mixed states

- **WHEN** a group has two published assets, one queued asset, one needs-review asset, and one failed asset
- **THEN** the group reports five assets and those counts while each member retains its own lifecycle, provenance, validation, and publication state

### Requirement: RAW and Library correlation uses canonical identity and history

The backend SHALL expose two separate concepts: `sourceProvenance`, which
preserves the historical source path/reference recorded by a QueueJob or
publication authority, and `currentRawCorrelation`, which is an optional
relationship to a currently known RAW AssetRecord. A matching filesystem path
MUST NOT bind a newly created/reused RAW AssetRecord to historical work unless
deterministic canonical evidence proves the identity.

Current converted and as-is publication identities SHALL use their respective
active canonical publication records and current Library destination identity.
Supported rename/move, publication, archive, and restore operations SHALL
preserve explainable history without fabricating current RAW correlation.

If existing records cannot deterministically answer the relationship, the
implementation SHALL add only the smallest durable stable reference captured at
the queue/publication boundary to the existing publication/provenance record
and SHALL not introduce complete-media hashing as the default identity
mechanism. Legacy path-only records SHALL preserve `sourceProvenance` while
leaving `currentRawCorrelation` unresolved.

#### Scenario: A RAW asset has a current converted publication

- **WHEN** a Queue execution links a source path to a non-retired published path
- **THEN** the projection preserves the job's source provenance, links the current RAW asset only when canonical identity proves it, and exposes the execution as history

#### Scenario: A RAW asset is directly published as-is

- **WHEN** an active DirectPublication records a source and current Library destination
- **THEN** the projection exposes the DirectPublication as source provenance and published-as-is publication without inferring conversion

#### Scenario: RAW is renamed before publication

- **WHEN** the supported RAW rename updates the canonical source reference before a Queue publication is created
- **THEN** the current correlation uses the reconciled canonical identity and the publication records the source relationship that was captured at the queue/publication boundary

#### Scenario: Library is renamed after publication

- **WHEN** #18 reconciliation renames or moves the current Library asset
- **THEN** the current Library publication correlation follows the reconciled current destination while historical source provenance and retired records remain unchanged

#### Scenario: Original RAW AssetRecord no longer exists

- **WHEN** a historical QueueJob or DirectPublication retains source provenance but its original RAW AssetRecord is absent
- **THEN** source provenance remains explainable and `currentRawCorrelation` is unresolved rather than fabricated

#### Scenario: A source path is later reused

- **WHEN** a historical `QueueJob.MediaPath` or `DirectPublication.SourcePath` equals the path of a different newly created RAW AssetRecord
- **THEN** the historical source path is retained but the new AssetRecord is not bound to the old work without deterministic identity evidence

#### Scenario: Legacy path-only provenance

- **WHEN** a legacy job has only a source path and no durable source AssetRecord reference
- **THEN** the path is shown as historical source provenance, current RAW correlation is unresolved, and no full-file hash is required or invented

#### Scenario: A Library asset is renamed

- **WHEN** the supported Library rename/move reconciles the current asset and publication references
- **THEN** the current correlation follows the new path while retired/historical paths and provenance remain unchanged

### Requirement: Bulk action eligibility is per asset and backend-authoritative

Every bulk action SHALL resolve each selected asset independently and return
bounded results that distinguish `selectable`, `eligible`, `skipped`, and
`blocked`, with a reason for non-eligible outcomes. Frontend badges, group
summaries, and counts MUST NOT be used as the authority for action safety.

#### Scenario: A mixed selection is resolved

- **WHEN** a selection contains an eligible RAW asset, an already queued asset, a missing asset, and an asset requiring review
- **THEN** the backend returns separate per-asset outcomes and reasons, queues only eligible assets after commit, and leaves skipped/blocked assets unchanged

#### Scenario: Action capability changes with active work

- **WHEN** an asset has active Queue or maintenance work
- **THEN** the affected actions are disabled or rejected with a backend reason while unrelated assets remain actionable

### Requirement: Action capabilities are advisory projections over canonical guards

The backend SHALL generate deterministic `AssetActionCapabilities` with
bounded reasons for UI presentation, but the projection MUST NOT become a
second implementation of rename, move, Queue, validation, publication,
archive, restore, original deletion, or track/subtitle/attachment mutation
policy. Each real mutation endpoint SHALL revalidate its canonical
operation/domain guard at execution time. A capability response MAY be stale
and SHALL NOT be treated as authorization, a transaction lock, or a substitute
for the mutation guard.

#### Scenario: Capability becomes stale before mutation

- **WHEN** Asset Info reports archive allowed and Queue work starts before the archive request is submitted
- **THEN** the canonical archive endpoint revalidates current eligibility and rejects the stale request if active work makes it unsafe

#### Scenario: Capability reason reflects a canonical guard

- **WHEN** a rename, restore, or delete-original operation is blocked by active work, missing identity, or an unsafe root
- **THEN** the capability exposes the bounded canonical reason and the endpoint enforces the same guard independently

### Requirement: Queue Remaining preserves eligibility and complete-inventory ordinals

Queue Remaining SHALL be an adapter over `QueueSelectedAssets`: preview SHALL
pass the complete logical-group Asset IDs with `commit=false`, and commit SHALL
pass the same canonical selection with `commit=true`. The adapter MAY add
current-publication skip information, but MUST NOT create a parallel
eligibility engine or alter Queue semantics. Canonical Queue outcomes SHALL be
mapped to the Assets vocabulary of selectable, eligible, skipped, and blocked
with bounded reasons.

It SHALL skip assets already published or otherwise not eligible, queue
remaining eligible RAW assets, preserve ordinal gaps caused by skipped members,
and exclude extras/additional material from normal episode positions. Queue
eligibility MUST NOT compact or renumber the complete inventory. Ordinals SHALL
come from the existing complete-inventory ordering (`assetSequenceLess`,
`episodeSequencePositions`, and existing multi-episode/direct-publication
naming authorities), not an Assets-specific ordinal service.

#### Scenario: Published members are skipped

- **WHEN** a season has published E01 and E02 plus eligible RAW E03 and E04
- **THEN** Queue Remaining previews/commits through `QueueSelectedAssets`, skips E01/E02, and queues E03/E04 using their canonical S02E03/S02E04 positions

#### Scenario: Queue results are adapted without new eligibility rules

- **WHEN** `QueueSelectedAssets` returns selected/eligible, queued, skipped, or failed results for the complete group
- **THEN** Queue Remaining maps those outcomes to selectable/eligible, queued, skipped, or blocked with the original per-asset reason and does not recompute eligibility

#### Scenario: Extras do not consume episode positions

- **WHEN** a group contains normal episodes and extras/additional material
- **THEN** normal episode ordinals are assigned by the existing complete-inventory authority and extras do not consume normal episode positions

### Requirement: Asset Info is the canonical per-asset control surface

Asset Info SHALL provide a lifecycle-oriented view with an Overview plus lazy
sections for Analysis/Advisor, Tracks, Subtitles, Attachments, Conversion,
Validation, Publication, Original, Evidence, and History. It SHALL retain
existing reusable track, subtitle, LAB, Test Encode, rename/move, Queue,
report, archive, restore, and re-conversion operations behind their canonical
boundaries.

#### Scenario: Overview is opened

- **WHEN** a user opens Asset Info for a persisted asset
- **THEN** Overview shows name, location, lifecycle, provenance, validation, original retention, destination, and bounded active/attention summaries without requiring every detail section

#### Scenario: A heavy section is opened

- **WHEN** a user explicitly opens Analysis, report evidence, subtitle, or history details
- **THEN** only that section's existing lazy endpoint/workflow is loaded and unrelated heavy payloads are not fetched

### Requirement: Overview is persisted-only and side-effect free

Opening or refreshing Overview MUST NOT start ffprobe, FFmpeg, OCR, analysis,
validation, publisher operations, filesystem scans, metric execution, or LAB
work. It SHALL render from bounded persisted projections and show unavailable or
stale evidence when the canonical source does not have a current result.

#### Scenario: No analysis exists

- **WHEN** Overview is opened for an asset without an analysis snapshot
- **THEN** it shows Analysis unavailable/missing and performs no scan or filesystem walk

### Requirement: Reports, executions, and revisions are evidence and history

Existing Result, AS-IS, validation, publication, scan, logs, reports, frozen
Queue plans, and maintenance evidence SHALL remain associated with their
canonical asset/job identity and load lazily. A Library asset MAY have multiple
conversion/publication revisions; the projection SHALL distinguish the current
non-retired publication from historical executions/publications and SHALL NOT
rewrite historical provenance when current name/path changes.


#### Scenario: Current publication differs from history

- **WHEN** a Library asset has a retired publication from job 1021 and a current publication from job 1842
- **THEN** Asset Info marks job 1842 as current and exposes job 1021 as historical without merging their reports or mutating job 1021

#### Scenario: Reports are opened on demand

- **WHEN** a user opens a Result report
- **THEN** the list view remains summary-only and the report body is fetched only for the selected evidence item

### Requirement: Persisted lifecycle dimensions support bounded filters and counts

Library filters SHALL support provenance, validation, original retention, and
workflow/attention dimensions using persisted projection evidence. Dashboard
counts SHALL retain useful visibility for RAW, Library, processing, needs
review, validation review, publication blocked, converted, and archived
originals even after Converted, Archive, and Reports tabs are removed.

Normal list rendering MUST NOT add filters that require ffprobe, analysis,
validation, report bodies, or filesystem walks.

#### Scenario: Filter a mixed Library

- **WHEN** a user filters Library by converted provenance, validation warnings, or archived originals
- **THEN** matching assets are selected from persisted lifecycle evidence and no media probe or filesystem walk runs

#### Scenario: Dashboard counts migrate

- **WHEN** the old Converted, Archive, and Reports contexts are no longer top-level tabs
- **THEN** equivalent persisted counts remain available as Library filters, dashboard cards, or attention summaries

### Requirement: Legacy records remain visible and fail closed

Legacy Library records, converted-only records, archive records, stale Source
Groups, incomplete source correlations, and old reports SHALL remain visible
through compatibility projection. When deterministic evidence is missing,
provenance SHALL be `legacy_or_unknown`, correlation SHALL be absent or
explicitly unresolved, and the system SHALL not infer Converted, publication,
validation, or original state from a path pattern.

#### Scenario: Legacy Library entry has no provenance

- **WHEN** a legacy Library record has no linked Queue or direct-publication evidence
- **THEN** it remains visible with legacy/unknown provenance and safe read-only or backend-authorized actions

#### Scenario: An old deep link is opened

- **WHEN** a user follows an existing Converted, Archive, or Reports route
- **THEN** the route redirects to the equivalent RAW/Library projection or renders a compatibility view with a clear destination; it does not silently delete or duplicate data

### Requirement: Lifecycle list/detail APIs are bounded and avoid N+1 work

The canonical inventory list/detail boundary SHALL provide a lightweight
persisted projection for list rows and a summary-plus-lazy-section contract for
detail views. Backend queries SHALL batch related records and use indexed
identity/status/path fields where appropriate. List rendering MUST NOT perform a
full filesystem walk, ffprobe, FFmpeg, OCR, Analysis, Validation, Publisher
operation, or report-body fan-out.

#### Scenario: Library list is rendered on a NAS

- **WHEN** a Library list with many assets is requested
- **THEN** the response is bounded/paginated or otherwise bounded, uses persisted records and bounded aggregates, and performs no media or filesystem work

#### Scenario: Detail sections are requested separately

- **WHEN** a user opens one history or report section
- **THEN** the server fetches only the selected section and does not issue one report query per list row

### Requirement: Integration boundaries remain owned by issues #52, #53, and #54

The asset lifecycle projection SHALL consume canonical Analysis evidence from
#52, Validation evidence from #53, and Publisher/delivery/archive policy from
#54 through explicit integration slots or adapters. Missing future evidence
SHALL render unavailable/pending according to existing evidence. Issue #21 MUST
NOT implement parallel Analysis, Validation, Publisher, archive-policy, or
workflow state machines.

#### Scenario: A redesign authority is not yet available

- **WHEN** #52, #53, or #54 has not supplied a future evidence field or endpoint
- **THEN** Assets uses the existing canonical evidence or renders the dimension unavailable and does not invent a replacement state machine

#### Scenario: A redesign authority is available later

- **WHEN** a canonical #52/#53/#54 projection is supplied
- **THEN** Asset Info consumes it at the integration boundary without changing RAW/Library location semantics or creating a second authority
