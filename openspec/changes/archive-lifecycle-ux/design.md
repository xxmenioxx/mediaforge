## Context and Existing Authorities

- `AssetLifecycleProjection` in
  `backend/internal/handlers/asset_lifecycle_projection.go` already resolves
  location, lifecycle, provenance, validation, current publication, RAW
  correlation, and advisory action capabilities. Its evidence summary exposes
  queue-job IDs and `originalArchivedPath`; it does not yet provide the
  archive record's full inspection details.
- `QueueJob` already records `ID`, `SourceAssetRecordID`, `LibraryID`,
  `PublishedPath`, publication retirement/time, and `OriginalArchivedPath`.
  `AssetRecord` already records its stable row ID, path, size, modified time,
  archive status, missing state, `ExpiresAt`, `CreatedAt`, and `SyncedAt`.
- Publisher's existing `syncPublishedAssetRecords` records an archive
  `AssetRecord` using the configured originals archive root and current
  retention setting. `assetRecordForPublishedPath` supplies the recorded size,
  sync time, and existing deadline. These remain the archive and retention
  authorities; no new persisted record is needed.
- The existing `Recover` mutation checks archive status, availability, active
  work, and destination collision. It does not currently revalidate that the
  source is inside the configured archive root. `DeleteArchive` requires
  explicit confirmation and checks archive status, active work, configured
  archive-root containment, and regular-file status. Both mutations must
  revalidate their full safety conditions at execution time.
- `AssetsPage.tsx` currently renders archive groups through a separate
  `ArchivedAssetTable`. The existing lifecycle/action projection and
  `POST /assets/recover` plus `DELETE /assets/archive` are reused for the
  contextual workflow.

## Decision

### One lifecycle projection, two inventory locations

Keep the existing `AssetLifecycleProjection` as the canonical read contract.
Add an optional nested `archivedOriginals` inspection projection to the
Library asset's lifecycle data; derive it from existing records and expose no
separate archive list endpoint. Archive `AssetRecord`s and their provenance
remain stored and available to existing backend authorities, but they are not
RAW or Library inventory rows.

Each projected entry contains only existing evidence:

- archive `AssetRecord` ID when present, recorded path, size, and availability;
- `archivedAt` from the canonically linked archive record's `CreatedAt`,
  meaning the time MVForge recorded that archived-original record. `SyncedAt`
  is synchronization state and MUST NOT be used as archive time. Do not relabel
  file `ModifiedAt` or publication `PublishedAt` as archive time; legacy or
  otherwise untrusted rows without a reliable `CreatedAt` report it as
  unknown;
- retention state from the existing lifecycle/retention projection and
  deadline from the existing archive record's `ExpiresAt`, when present;
- provenance authority and record ID, including the owning QueueJob ID and
  `SourceAssetRecordID` when available; and
- a relationship state that distinguishes current, historical/stale, and
  unresolved evidence.

Do not persist a second archive object or recompute retention. A missing archive
record may still be represented from the recorded QueueJob path and provenance,
with unavailable metadata and no actions. Preserve the recorded evidence when
the file is missing or stale.

### Canonical relationship, not path-only association

Resolve the Library asset and current publication through #21's canonical
projection: the selected current publication and its Library asset identity,
plus the QueueJob's stable ID and `SourceAssetRecordID`. Use the exact
`QueueJob.OriginalArchivedPath` only to locate its archive `AssetRecord`; path
equality between an archive record and a Library record never establishes the
relationship. Do not select a Library asset from matching names, relative
paths, or reused paths.

If source identity is path-only, current-publication evidence conflicts, or
the owning publication is retired, preserve it as unresolved or historical
evidence. Do not relabel it as the current Library asset. Ambiguous and
unresolved entries expose no restore/delete capability.

### Contextual frontend behavior

The Assets inventory navigation and location state contain only RAW and
Library. Remove the standalone Archived tab/table and any filter state that
returns archive records as inventory rows. Library filters continue to return
Library asset records only; an original-retention facet, if retained, filters
those Library assets and does not list archive records. Existing Archived
deep links resolve to Library without recreating the old archive list.

Show archived-original details in the related Library asset's Asset Info
context: recorded path, size, archive time, provenance, retention state/deadline,
availability, and the backend's bounded action reasons. A missing, stale, or
unresolved original remains visible as such under that context and is not
silently rebound to another asset.

### Existing mutations remain authoritative

Use the existing `restore` and `deleteArchivedOriginal` capability fields as
advisory UI state and keep `POST /assets/recover` and
`DELETE /assets/archive` as the mutation authorities. The contextual request
includes the archive `AssetRecord` ID and expected related Library asset ID as
identity preconditions; existing path parameters remain locators, not proof of
relationship. Permanent deletion continues to require explicit confirmation.

At execution, each handler reloads the archive record and canonical
publication/source relationship and rejects stale or mismatched preconditions,
active work, missing files, or unresolved identity. It re-reads the configured
archive root and, for restore, the configured RAW root; normalizes and checks
containment, rejects symlink/non-regular archive targets and unsafe or
occupied restore destinations, and verifies the target is not the current
Library publication. These checks are performed at the filesystem mutation
boundary, not inherited from a prior capability projection. If the file,
record, root, or relationship changes between inspection and execution, fail
closed without mutating unrelated media or provenance.

Restore continues to recover the original into RAW and leaves the current
Library publication intact. Delete continues to remove only the archived
original while preserving publication/job history. No Publisher/#54 or
retention-policy semantics change.

## Data and Migration

No new table, lifecycle model, retention rule, archive creation path, or
persisted timestamp is introduced. The inspection is derived from
`AssetRecord`, `QueueJob`, the existing current-publication resolver, and
existing retention evidence. Any optional request identity fields extend the
existing mutation requests; they do not create new mutation endpoints.

## Risks and Boundaries

- Legacy rows without stable source/publication identity may no longer be
  actionable from this contextual view. Preserve their history and show
  unresolved evidence rather than guessing from paths.
- Filesystem roots or targets can change after the UI projection. Mutation
  handlers must re-resolve both identity and safe-root conditions immediately
  before moving or deleting a file.
- This change does not define new retention deadlines, change auto-delete
  behavior, or choose a winner across conflicting publication authorities.
