## ADDED Requirements

### Requirement: Archived originals are contextual Library evidence

The Assets inventory SHALL expose only RAW and Library locations. An archive
`AssetRecord` SHALL remain lifecycle/provenance evidence and MUST NOT be
returned as a third inventory location, an Archived table, or an archive-only
Library filter result. The existing lifecycle projection SHALL provide
read-only archived-original inspection under a related Library asset only
when #21's current-publication and source-provenance authorities resolve that
relationship.

Each inspection entry SHALL expose the existing archive record ID when
available, recorded path and size, recorded archive timestamp when available,
provenance references, availability, retention state, and retention deadline
when recorded. It MUST NOT introduce a persisted archive model or a separate
archive inventory API.

The exact `OriginalArchivedPath` on the owning QueueJob MAY locate its archive
`AssetRecord`. That path lookup alone MUST NOT associate an archive with a
current Library asset. The relationship SHALL use stable QueueJob,
source-AssetRecord, current-publication, and Library-asset identity from the
canonical projection. Missing or conflicting identity SHALL remain
unresolved; retired publication evidence SHALL remain historical. Archive
time SHALL use the canonically linked archive `AssetRecord.CreatedAt` as the
time MVForge recorded that archived-original record. `SyncedAt` is
synchronization state and MUST NOT be used as archive time. File `ModifiedAt`
and publication `PublishedAt` MUST NOT be relabeled as archive time. Legacy or
otherwise untrusted records without a reliable `CreatedAt` SHALL report archive
time as unknown. Retention state and deadline SHALL reuse existing
lifecycle/retention evidence and `ExpiresAt`; the projection MUST NOT
calculate a new deadline or change retention policy behavior. No new
persisted timestamp SHALL be introduced solely for this UX.

Missing, stale/historical, and unresolved evidence SHALL preserve the recorded
path and provenance without silently binding to another file or publication.
Such evidence SHALL expose no restore or delete action. For current, available
evidence, the projection SHALL reuse the existing restore and
`deleteArchivedOriginal` action capabilities and bounded reasons as advisory
state.

Frontend inventory navigation SHALL offer only RAW and Library. Existing
Archived deep links SHALL resolve to Library without reconstructing an
archive-only list or filter. Library filters SHALL return Library asset
records only. Asset Info for a canonically related Library asset SHALL show
archived-original path, size, archive time, provenance, retention
state/deadline, availability, and action reasons in context.

Contextual restore and delete SHALL reuse the existing recover and
archive-delete mutations. The request SHALL carry the archive `AssetRecord` ID
and expected Library asset ID as identity preconditions; existing path
parameters are locators, not proof of relationship. Permanent deletion SHALL
continue to require explicit confirmation. At execution, the backend SHALL
reload and revalidate archive, current-publication, and source-provenance
identity, active work, file availability, and configured roots. Restore SHALL
prove the source remains inside the current configured archive root and the
derived destination remains safely inside the current RAW root and unoccupied.
Delete SHALL prove the target remains a regular file inside the current
configured archive root. Both operations MUST fail closed on stale identity,
changed root, unsafe path, symlink/non-regular target, or conflict with the
current Library publication. Restore MUST leave the Library publication
intact; delete MUST preserve job/publication provenance.

#### Scenario: Current publication has stable archive provenance

- **WHEN** the selected current QueueJob publication has an
  `OriginalArchivedPath` and a stable `SourceAssetRecordID`
- **THEN** its archived original is shown under the related Library asset with
  the owning record IDs, recorded metadata, and existing retention evidence,
  without appearing as another inventory row

#### Scenario: Paths match but canonical identity does not resolve

- **WHEN** matching paths exist but canonical source/current-publication
  identity is absent, stale, or conflicting
- **THEN** the evidence remains unresolved or historical and is not attached
  to a current Library asset by path equality

#### Scenario: The archive record or file is missing

- **WHEN** the owning QueueJob still records an archived path but the archive
  record or file is no longer available
- **THEN** the Library context preserves the path and provenance, marks the
  original unavailable, and disables restore/delete

#### Scenario: The owning publication is retired

- **WHEN** an archived original belongs to retired publication evidence
- **THEN** it remains historical and is not relabeled as the current
  publication's original

#### Scenario: The user follows an Archived deep link

- **WHEN** a user opens an existing Archived inventory deep link
- **THEN** navigation resolves to Library without rebuilding an archive-only
  list or filter

#### Scenario: A capability becomes stale or a root changes

- **WHEN** archive/Library identity, active-work state, configured root, or
  filesystem target changes after capabilities were projected
- **THEN** the existing mutation revalidates and rejects the operation before
  changing the file or provenance

#### Scenario: Restore or delete targets an unsafe path

- **WHEN** the archive source escapes the configured archive root, the restore
  destination escapes RAW or already exists, or the target is a symlink or
  non-regular file
- **THEN** the backend rejects the action without mutating the target or the
  current Library publication
