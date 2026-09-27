## Context and Existing Architecture

- `backend/internal/handlers/publisher.go` exposes manual `PublishJob` and a
  shared `publishQueueJobWithMode` path. It reloads the QueueJob, checks
  completed state and `canonicalRunAllowsPublish`, reads the Result output,
  validates required subtitle artifacts, and resolves a Library and profile.
- `QueueJob` already carries `LibraryID`, `PublishMode`,
  `ActiveExecutionPlanID`, `PlannedPublishedPath`, Result `OutputPath`,
  Validation summary fields, and published/original-archive projections.
  `ExecutionPlan` holds the frozen execution/output plan. Publisher currently
  falls back to `plannedOutputPathForJob` when `PlannedPublishedPath` is empty.
- Standard publication currently copies directly to final paths with
  `os.Create`; it backs up overwrite targets and rolls back tracked paths for
  in-process errors. External/generated subtitle and font artifacts are then
  copied to their final paths. These compensations do not survive process loss
  between individual filesystem mutations.
- `archivePublishedOriginal` moves the source into the configured originals
  archive, preferring rename and falling back to copy/remove. QueueJob fields
  and `syncPublishedAssetRecords` record publication and archive evidence in
  existing `AssetRecord` rows. Final QueueJob persistence is currently separate
  from materialization, and some save/transition errors are ignored.
- `PublisherPage.tsx` polls Queue jobs and current Settings; the
  `publisherOverwriteEnabled` setting controls a publish overwrite flag.
  `HistoryPage.tsx` polls a paginated History API. The #54 contract replaces
  current-Settings overwrite decisions with frozen Queue intent and uses saved
  delivery summaries for both views.
- The current `AutoPublisher` has periodic processing and published-job
  reconciliation, but it does not provide durable publication-attempt claims,
  leases, or recovery ownership. #54 introduces that Publisher-owned authority
  and integrates it with the existing AutoPublisher/startup cadence.
- #53 owns immutable ValidationRun and override decisions bound to the exact
  Queue execution, ExecutionPlan, and canonical Result generation/revision.
  QueueJob validation columns are projections. #174 keeps `AssetRecord` and
  existing QueueJob provenance as the archive/retention authority; archive
  paths remain locators, not identity. Neither #53 nor #174 is reimplemented.

## Decisions

### 1. One publication identity, owned by Publisher

Persist one Publisher-owned publication record for a single eligible Queue
execution and its exact Result. Its immutable identity binds:

- QueueJob ID and execution number;
- active ExecutionPlan ID/version/fingerprint;
- canonical Result artifact/generation and bounded output revision;
- canonical ValidationRun ID and, when used, eligible override event ID;
- frozen Library ID, planned destination root/path identity, and publish mode;
- source AssetRecord identity when present.

The destination snapshot must contain the resolved planned target and enough
root identity to detect a Library/root change. If Queue cannot supply that
frozen destination identity, Publisher blocks handoff. It does not fall back to
current Settings, profile policy, path naming rules, or a newly resolved
Library destination. A later Queue execution or changed Result has a new
publication identity. No full-file hash is added to identify the publication;
the Result generation and its existing bounded revision are authoritative.

This record is Publisher delivery state, not an asset lifecycle/archive model
or another Queue. QueueJob remains the Queue execution and supplies a
read-optimized publication summary. A unique binding prevents two concurrent
publication records for the same execution/Result. Publisher's persisted
publication records own attempt claims, leases, and nonterminal recovery. The
existing AutoPublisher periodic processing and startup cadence invoke this
Publisher-owned work; #54 adds no generic scheduler or second Queue.

### 2. State and attempt evidence

The publication record has these states:

| State | Meaning |
| --- | --- |
| `READY` | Identity and frozen destination are valid; one attempt may be claimed. |
| `HELD` | An operator paused automatic/manual advancement with an audited reason. No filesystem work starts. |
| `PUBLISHING` | A durable attempt claim/lease exists. Its phase and manifest identify staging, final promotion, and database finalization. |
| `PUBLISHED` | The exact final media and all required sidecars are committed, and the publication plus QueueJob provenance projection transaction committed. This state is canonical and does not depend on original archival or subscriber success. |
| `BLOCKED` | Identity, destination, ownership, or path safety cannot be proven, or an unowned destination conflict needs a new Queue decision. Automatic retry is forbidden. |
| `FAILED` | An attempt ended before publication commit with a retryable I/O/resource failure and its owned partial files were safely removed or restored. An operator or bounded Publisher retry may return it to `READY` after identity is rechecked. |

`READY -> HELD` and `HELD -> READY` require an operator action; releasing a hold
revalidates the same identity. `READY -> PUBLISHING` is an atomic database
claim. `PUBLISHING -> PUBLISHED` occurs only at the success boundary below.
`FAILED -> READY` creates a new attempt under the same publication identity.
`BLOCKED` cannot be cleared by changing that identity in place: resolve the
Queue plan/conflict and produce a new eligible publication identity where
necessary. A retry of `PUBLISHED` returns the existing success without
materialization, archive, or event duplication.

Claiming is an atomic compare-and-set: only a `READY` record can transition to
`PUBLISHING`, and the same database transaction creates the active attempt ID,
claim token/fencing generation, and bounded lease expiry. A uniqueness
constraint or equivalent transaction invariant permits at most one active
attempt per publication identity. The active owner renews its lease while
working and must present its current fencing generation before every durable
phase checkpoint and filesystem mutation. An unexpired claim cannot be
reacquired. After expiry, a recovery worker conditionally reacquires only if
the persisted state, old token/generation, and expiry still match; it retains
the same logical attempt ID and installs a new token/generation. A former
owner with a stale generation cannot continue mutations.

Attempt identity is stable across crash recovery. Append-only attempt evidence
records start, bounded phase/checkpoint results, terminal outcome, timestamps,
artifact references, affected controlled paths, byte counts, and allowlisted
error codes with bounded operator text. Large logs and media evidence stay in
their existing stores and are linked by reference. An active phase is a
recoverable projection; terminal evidence is immutable. History list responses
are bounded and paginated.

Audited Publisher actions follow #53's V1 actor boundary. The server assigns
`local_operator` to interactive hold, release, retry, and other operator
decisions, and `system` to automatic and recovery actions. Requests may supply
the reason and allowed action parameters, but never a trusted actor identity;
the server rejects or ignores a client actor field. Each accepted audited
action appends versioned evidence with its action, actor reference, timestamp,
and bounded reason/parameters. Actor references retain a source/kind and
identifier shape for a future authenticated principal, without adding login
or authentication infrastructure. An action event never edits historical
attempt evidence.

### 3. Canonical handoff and frozen policy

At automatic or manual handoff, Publisher asks #53's canonical authorization
path to verify the current ValidationRun or eligible override against the exact
Queue execution, active plan, and Result identity. QueueJob validation status
fields alone do not authorize publication. A stale ValidationRun, missing or
replaced Result, unavailable identity, or an ineligible override is
`BLOCKED`; Publisher cannot waive those findings or silently bind a newer
Result.

Queue/ExecutionPlan is the sole owner of destination Library and publish mode.
Publisher consumes the frozen Library ID, resolved destination, and
`standard`/`replace-library` mode. It never reads a live overwrite setting or
recomputes the destination. A changed or missing Library/root snapshot is a
`destination_changed` block that requires a new Queue decision.

`standard` mode never replaces an existing unowned target. `replace-library`
is honored only when frozen into Queue intent and when the existing target is
the unambiguous current Library publication it names. Publisher records the
replacement target identity and holds a reversible backup until the success
transaction commits. A manual Publisher checkbox or setting cannot promote a
conflict into overwrite permission. A path occupied by an unrelated file,
ambiguous asset, or different publication is `BLOCKED`; byte equality or path
equality is not identity proof.

### 4. Controlled temporary materialization and sidecars

Publisher resolves the required delivery manifest from the bound Result and
frozen plan, including each required subtitle/font/other sidecar identity and
target. Missing, stale, unreadable, or changed required artifacts prevent
commit. Optional artifacts may be recorded as omitted with a bounded reason;
they do not weaken the required manifest.

Before mutation, Publisher rechecks the Result revision, frozen destination,
and all paths under the configured controlled roots. It rejects symlinked or
non-regular sources/targets, path escape, changed roots, and unsafe parent
components. Checks repeat at mutation time using directory-relative,
no-follow operations to limit TOCTOU races. Capacity is checked against the
bounded plan estimate and current destination free space; the copy/write path
still handles capacity changing during the attempt.

Keep the canonical Result source intact until `PUBLISHED`. Copy source media
and required sidecars to private temporary files on the destination filesystem,
flush file data and directory metadata, then promote each staged file with an
atomic same-filesystem rename. This is also the cross-filesystem path: copy
from the Result into a destination-side temporary file, then rename that temp
file. Never move the only canonical Result across filesystems or expose a
partially copied final file. Standard mode uses no-replace promotion. Replace
mode first records and moves only its proven target files to transaction-owned
backups, then promotes staged files. Preserve a durable allowlisted manifest of
temp/final/backup paths and attempt-owned file receipts.

The filesystem has no atomic multi-file rename for a flat media-plus-sidecar
set. Each final file is individually atomic; the package is not considered
published until every required final file exists and matches this attempt's
receipts and the database success transaction commits. While the record is
`PUBLISHING`, MVForge does not announce the package in inventory, History as
published, or events. If sidecar promotion fails, safely undo only files
proven to belong to this attempt and restore only its proven backups. If
ownership cannot be proved, leave the record `BLOCKED` with exact partial-path
evidence; do not touch unowned files, archive the original, or emit an event.

### 5. Exact publication, archive, and event boundary

Publication becomes canonically `PUBLISHED` at one database transaction after
the primary asset and all required sidecars have been atomically promoted,
flushed, and rechecked against the durable attempt manifest. That transaction
persists the publication record/state, QueueJob `PublishedPath`/`PublishedAt`
and related publication provenance projections, and the durable continuation
needed for original archival. If this transaction fails, the attempt remains
recoverable in `PUBLISHING`; a process/API return is not success.

Only after this transaction may Publisher archive the original. Reuse #174's
existing archive move, `AssetRecord`, `OriginalArchivedPath`, and retention
authority; record any durable archive path intent in the publication attempt
as a recovery locator, not as a new archive fact/model. Archive success,
unavailable source, and archive failure remain separate from
`Publication.state=PUBLISHED`. Archive failure never removes or rolls back the
committed Library asset. Recovery completes a move whose file operation
succeeded but whose #174 record write was interrupted, using #174's existing
recording path and canonical source/publication provenance.

After the archive outcome is recorded (including an explicit failure or
not-required outcome), insert one `AssetPublished` outbox identity unique to
the publication. The event refers to the committed media, required sidecars,
Queue/Result/Validation identities, Library, and existing archive provenance
when available. It is generic and contains no Jellyfin behavior. The outbox
dispatcher may redeliver the same event ID after an ambiguous acknowledgement;
subscribers deduplicate by that stable ID. A delivery/integration error is
recorded and retried independently and never changes `PUBLISHED` or restores
the old Library content.

### 6. Crash recovery and idempotency

At startup and during the existing AutoPublisher periodic processing, query
persisted nonterminal publication records only. #54 adds claim/lease
acquisition, renewal, expiry, conditional reacquisition, and manifest-based
recovery to that cadence; the current cadence itself is not a durable claim or
recovery authority. Only an expired `PUBLISHING` claim is reacquired, by
conditional state/token/generation/expiry comparison. Recovery retains that
logical attempt ID, verifies the same Queue execution, Result revision, frozen
destination, and attempt-owned receipts, then resumes safe missing steps.
`READY` records may be claimed, while `HELD`, `BLOCKED`, and `FAILED` records
do not trigger filesystem recovery. Reconciliation inspects only exact paths in
the persisted attempt manifest and never walks Library or RAW roots. A repeat
request for an active attempt returns its existing state. A retry after a
safely finalized failure creates a new immutable attempt ID, not a second
publication identity. No separate generic scheduler or Queue is introduced.

| Process stops after… | Recovery action |
| --- | --- |
| Claim or temporary staging | Reuse or remove only manifest-owned temp files after identity checks; resume copy or record a safe retryable failure. |
| Some final files, before all required sidecars | Verify each final/temp/backup receipt. Resume missing promotions when safe; otherwise restore attempt-owned changes or block on ambiguous ownership. No archive or event. |
| All final files, before success transaction | Revalidate all receipts and atomically commit `PUBLISHED`/QueueJob provenance once. Do not copy again or infer success from matching paths. |
| Success transaction, before/during original archive | Publication remains `PUBLISHED`; resume the existing #174 archive operation and repair its record through the existing authority. |
| Archive outcome, before outbox insertion | Insert the unique event identity idempotently after confirming committed files and recorded archive outcome. |
| Outbox delivery or subscriber processing | Retry the same event ID; do not recreate publication, rearchive, or roll back. |

Insufficient space, source read, destination write, flush, or rename errors are
`FAILED` only after the attempt's filesystem state is safely reconciled. If
the state cannot be proven or repaired without risking unrelated files, use
`BLOCKED`. A database uncertainty after all final renames remains
`PUBLISHING` until recovery proves and commits the exact attempt.

### 7. Audit, History, and Publisher UI

Publisher's default view prioritizes `HELD`, `BLOCKED`, `FAILED`, stale handoff,
and destination conflicts with a concise reason and safe next action. Active
`PUBLISHING` work shows its persisted phase; clean `READY` work is available in
an explicit view; `PUBLISHED` remains discoverable in History. The operator
can place a `READY` item on hold and release it without changing its identity.

History preserves publication identity, final target, frozen mode, current
state, attempt count/latest outcome, ValidationRun/override references, and
the existing #174 original/archive status as separate facts. Existing jobs
without a bound publication identity remain legacy history and are not
silently promoted to `PUBLISHED`.

Publisher/History list and poll APIs return stored bounded summaries in batch.
Rendering, searching, paging, or polling performs no filesystem scan, ffprobe,
Validation, full-file hashing, or path-by-path existence check. A user opens a
specific attempt for its bounded stored evidence; filesystem verification
occurs only in a Publisher mutation or recovery pass.

## Data and Migration

Add publication-specific persistence and attempt/outbox evidence additively.
Keep current QueueJob publication fields as transactionally refreshed summary
and compatibility projections; the Publisher publication record is the
authority for the new delivery state. Preserve all existing Result,
ValidationRun, QueueJob, AssetRecord, and archive evidence. Historical
`PublishedAt`/`PublishedPath` values without the new identity remain legacy
published history and are not rewritten or treated as an in-flight publication.
No source media or historical evidence is deleted by migration.

## Risks and Trade-offs

- Flat media and sidecar paths cannot commit as one filesystem transaction.
  Per-file atomic renames plus a manifest, reconciliation, and the database
  success boundary prevent MVForge from claiming an incomplete package.
- External scanners can observe individual final renames before the database
  commit. MVForge withholds inventory/event success until commit; the design
  does not promise filesystem-wide isolation from external readers.
- A changed or ambiguous Library target requires a new Queue decision instead
  of an automatic reroute, preserving frozen destination intent.
- Exactly-once external delivery cannot be guaranteed across ambiguous
  acknowledgements. One canonical outbox event identity and subscriber
  deduplication prevent duplicate logical publication events.
