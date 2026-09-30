## Purpose

Give Test Encode runs one durable, bounded inventory and safe lifecycle controls across Settings, Asset Info, and LAB while preserving the existing Test Encode execution and output behavior.

## ADDED Requirements

### Requirement: Test Encodes have one canonical retained inventory

The system MUST expose persisted Test Encode runs through one backend-authoritative inventory that includes the run ID, source asset ID and path, creation time, sample start and duration, profile and resolved encoder summary, execution status, Keep/Temporary retention, output path and existence, owned artifact count, known disk usage, and the latest reconciliation result. Inventory reads MUST be bounded and support filtering by execution status, retention, reconciliation result, age, and asset or Library. Execution status and reconciliation result MUST remain separate.

#### Scenario: Inventory returns run and artifact evidence
- **WHEN** a user opens Test Runs for persisted Test Encodes
- **THEN** each returned run includes its execution and retention state, output evidence, artifact count, known size, and reconciliation evidence
- **AND** filesystem findings do not change the Test Encode execution status

#### Scenario: Inventory is filtered and bounded
- **WHEN** a user filters Test Runs by status, retention, reconciliation result, age, asset, or Library
- **THEN** the backend returns only matching runs within the requested bounded page
- **AND** the inventory does not enumerate RAW or Library trees

### Requirement: Test Encode lifecycle defaults are domain-scoped Settings

The system MUST provide a dedicated Test Encode Settings domain for Test Encode-specific sample defaults, default retention, temporary retention age, failed/cancelled retention age, and bounded reconciliation/cleanup policy. These settings MUST NOT duplicate encoder, codec, quality, filter, track, subtitle, or Profile parameters. Existing Settings validation and atomic update behavior MUST apply. Existing installations MUST retain safe defaults and compatible retention behavior.

#### Scenario: User updates Test Encode defaults
- **WHEN** a valid Test Encode policy is saved
- **THEN** the complete policy is validated and persisted atomically under its domain setting
- **AND** future Test Encodes use those lifecycle/sample defaults without changing resolved Profile or encoder behavior

#### Scenario: Existing settings are migrated
- **WHEN** an installation has legacy Test Encode retention in general housekeeping settings and no Test Encode domain setting
- **THEN** the Test Encode domain receives compatible retention values and conservative defaults for newly introduced fields
- **AND** existing Test Encode rows remain discoverable with their current paths and Keep values

### Requirement: Keep and Temporary are explicit retention choices

The system MUST preserve each run's Keep or Temporary choice independently of execution and reconciliation status. Keep runs MUST remain visible and reconcilable, MUST be excluded from ordinary automatic cleanup, and MUST support explicit Keep-to-Temporary, Temporary-to-Keep, and Keep-to-Delete actions without re-encoding. Deleting a Keep run MUST require explicit confirmation.

#### Scenario: Keep is released
- **WHEN** a user changes a Keep run to Temporary
- **THEN** the run remains in the canonical inventory and receives the configured temporary expiration
- **AND** its output and execution result are unchanged

#### Scenario: Kept output is missing
- **WHEN** the output for a Keep run is manually removed
- **THEN** the run remains visible with Keep retention and Missing Output reconciliation
- **AND** the user can explicitly remove the stale record

#### Scenario: Automatic cleanup excludes Keep
- **WHEN** ordinary housekeeping evaluates expired Test Encodes
- **THEN** it does not delete any Keep run or its artifacts

#### Scenario: User explicitly deletes Keep
- **WHEN** a user confirms deletion of a Keep run
- **THEN** the backend applies the same ownership and path safety checks as for Temporary runs
- **AND** it removes only that run's proven owned files and record state

### Requirement: Test Encode artifact ownership is persisted and backfilled

The system MUST persist explicit ownership for each Test Encode output, temporary output, and generated sidecar, with its run identity, configured Library root identity, path, artifact type, creation time, and known size. Artifact ownership MUST be derived from persisted Test Encode metadata, never from a filename alone. Existing output, temporary, and subtitle artifact paths MUST be backfilled idempotently without moving files or dropping historical/kept runs. Asset Info and LAB MUST continue to use the same Test Encode history and inventory.

#### Scenario: Existing Test Encodes receive ownership records
- **WHEN** the persistence migration runs on historical Test Encode rows
- **THEN** it records their existing output, temporary, and subtitle artifact paths as owned artifacts
- **AND** repeated migration runs do not duplicate ownership records or alter files

#### Scenario: Asset Info and LAB read the canonical history
- **WHEN** Asset Info or LAB displays Test Encode history
- **THEN** it reads the same canonical Test Encode runs and retention state as Test Runs
- **AND** the existing LAB Test Encode request and resolved execution remain functional

### Requirement: Reconciliation is explicit, bounded, and separate from execution

The system MUST provide an explicit bounded reconciliation operation that inspects only canonical Test Encode records, configured roots, and paths derived from persisted owned artifacts. Reconciliation MUST NOT recursively scan RAW or ordinary Library roots or run ffprobe solely to classify files. It MUST record Healthy, Missing Output, Partial, Orphaned, Interrupted/Stale, or Cleanup Failed findings separately from execution status. Active operations MUST NOT be marked interrupted or stale. Orphan files may be reported as owned only when persisted MVForge artifact metadata proves ownership; ambiguous or unowned files MUST NOT be deleted.

#### Scenario: Complete run is healthy
- **WHEN** a persisted completed Test Encode and all expected owned artifacts exist
- **THEN** reconciliation records Healthy and preserves the execution status

#### Scenario: Kept output is missing
- **WHEN** a persisted Keep run's primary output is absent
- **THEN** reconciliation records Missing Output without recreating the output or changing Keep or execution status

#### Scenario: Media is missing while an owned sidecar remains
- **WHEN** the primary media path is absent and a persisted owned sidecar still exists
- **THEN** reconciliation records Partial and identifies the remaining owned artifact

#### Scenario: Expected sidecar is missing
- **WHEN** the primary media exists and an expected owned sidecar is absent
- **THEN** reconciliation records Partial without deleting the media

#### Scenario: Interrupted execution is inactive
- **WHEN** a persisted run is still marked active but no active Test Encode operation owns it
- **THEN** reconciliation records Interrupted/Stale after the bounded stale check
- **AND** an active Test Encode operation is never marked stale

#### Scenario: Artifact evidence has no active run
- **WHEN** persisted artifact ownership proves a file belongs to a missing or deleted Test Encode record
- **THEN** reconciliation reports the artifact as Orphaned and offers explicit cleanup
- **AND** a file without that ownership evidence is neither treated as owned nor deleted

### Requirement: Cleanup deletes only explicitly owned safe artifacts

Manual, bulk, orphan, and housekeeping cleanup MUST delete individual persisted owned artifacts only after confirming canonical ownership, containment under the configured root bound to that Test Encode, no traversal or symlink escape, no source or current Library asset identity, no unrelated sidecar, and no active-operation reference. Paths outside allowed roots MUST be rejected. Cleanup MUST NOT recursively remove a directory. Missing paths MUST be handled idempotently, and ambiguous files MUST remain untouched.

#### Scenario: Artifact path is outside its configured root
- **WHEN** cleanup is requested for an owned path outside the Test Encode's configured root
- **THEN** the backend rejects that path and leaves the file untouched

#### Scenario: Artifact path traverses or escapes through a symlink
- **WHEN** cleanup is requested for a path containing traversal or whose resolved parent escapes the configured root
- **THEN** the backend rejects that path and leaves the target untouched

#### Scenario: Source or current Library asset is protected
- **WHEN** a proposed owned path resolves to the source media or a current Library asset
- **THEN** cleanup rejects the path and does not modify that media

#### Scenario: Active operation owns the artifact
- **WHEN** cleanup is requested while an active Test Encode operation references the artifact
- **THEN** cleanup rejects the request without changing the file or operation

#### Scenario: Failed or cancelled temporary artifacts are explicitly removed
- **WHEN** a failed or cancelled run is inactive and its owned temporary artifacts remain
- **THEN** cleanup removes only those exact registered files and records any cleanup failure

#### Scenario: Missing path is already cleaned
- **WHEN** cleanup encounters an owned artifact path that is already absent
- **THEN** it completes idempotently without attempting to remove a containing directory

### Requirement: Housekeeping reuses bounded Test Encode cleanup policy

Automatic Test Encode cleanup MUST run through the existing housekeeping mechanism in small predictable batches. It MUST exclude Keep and active runs, use the configured Temporary and Failed/Cancelled retention ages, and reconcile missing-output records without inferring deletion of unrelated files.

#### Scenario: Eligible Temporary run expires
- **WHEN** a completed Temporary run reaches its configured retention age
- **THEN** housekeeping may remove its safe owned artifacts and update its retained record state

#### Scenario: Failed or Cancelled run reaches retention age
- **WHEN** an inactive Failed or Cancelled run reaches its configured shorter retention age
- **THEN** housekeeping may remove only its safe owned artifacts in the current bounded batch

#### Scenario: Missing output expires
- **WHEN** an expired persisted run has a missing output
- **THEN** housekeeping may update or remove stale database state but never infers another filesystem path to delete

### Requirement: Settings and Test Runs expose practical lifecycle actions

The Settings UI MUST provide a Test Encode domain with lifecycle defaults and retained Test Runs. Test Runs MUST expose View, Keep, Release Keep, Delete, and Reconcile actions where applicable, practical status/retention/reconciliation/age/asset/Library filters, and bounded listing. Bulk deletion MUST show a confirmation summary with Test Encode count, media output count, other artifact count, estimated reclaimed bytes, and Keep count; Keep deletion MUST be explicitly confirmed. Destructive controls MUST report backend rejection and cleanup failures.

#### Scenario: User reviews a bulk deletion
- **WHEN** a user selects multiple Test Encodes for deletion
- **THEN** the confirmation shows selected run count, media and other artifact counts, known estimated storage, and Keep count
- **AND** the user explicitly confirms before the backend attempts cleanup

#### Scenario: User filters retained runs
- **WHEN** a user applies execution, retention, reconciliation, age, asset, or Library filters
- **THEN** the Test Runs view requests the matching bounded canonical inventory
- **AND** its actions update or reconcile those same persisted runs
