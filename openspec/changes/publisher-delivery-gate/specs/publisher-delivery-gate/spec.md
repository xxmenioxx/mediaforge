## ADDED Requirements

### Requirement: Publisher handoff binds the canonical eligible Result

Publisher SHALL authorize a handoff using the canonical #53 ValidationRun or
eligible override bound to the exact Queue execution, active ExecutionPlan,
and Result generation/output revision. QueueJob validation summary fields
SHALL NOT independently authorize delivery. Publisher SHALL reject stale
ValidationRun identity, replaced or unreadable Result artifacts, and
ineligible overrides without changing the Validation decision or binding to a
new Result. A new Queue execution or Result SHALL receive a distinct
publication identity.

#### Scenario: Eligible result reaches Publisher

- **WHEN** #53 authorizes the completed Queue execution's exact Result
- **THEN** Publisher creates or reuses one publication identity bound to that
  Result, run/override, plan, frozen Library destination, and publish mode

#### Scenario: Validation or Result identity is stale

- **WHEN** the run does not bind to the current execution/Result, the Result
  has been replaced, or its identity cannot be verified
- **THEN** Publisher records a `BLOCKED` reason, performs no final materialization
  or original archive, and emits no `AssetPublished` event

### Requirement: Publication identity and states are explicit and idempotent

The system SHALL persist one stable publication identity per eligible Queue
execution and canonical Result. Its immutable binding SHALL include Queue and
ExecutionPlan identity, Result identity, ValidationRun/eligible override
references, frozen Library/destination identity, publish mode, and source
provenance when available. It SHALL NOT use full-file hashes solely for
publication identity or UI display.

The publication state SHALL be `READY`, `HELD`, `PUBLISHING`, `PUBLISHED`,
`BLOCKED`, or `FAILED`. `HELD` SHALL pause work until an audited operator
release. `FAILED` SHALL mean a pre-publication attempt stopped with a
retryable error and safely reconciled filesystem state. `BLOCKED` SHALL mean
identity, destination, conflict, or safety requires resolution and SHALL NOT
auto-retry. `PUBLISHED` SHALL remain true if original archival or downstream
event delivery fails. Retrying the same identity SHALL reuse canonical success
or the active attempt; each retry after a safe failure SHALL create new
immutable attempt evidence.

#### Scenario: Operator holds and releases a ready publication

- **WHEN** an operator places a `READY` publication on hold and later releases
  it
- **THEN** it transitions through `HELD` back to `READY` with actor/reason/time
  evidence and the same immutable publication identity

#### Scenario: Duplicate publish request or retry follows a crash

- **WHEN** the same publication request is received while an attempt is active
  or after publication is complete
- **THEN** the system returns the existing attempt/state or `PUBLISHED` result
  and does not create another logical publication

### Requirement: Publisher decisions use server-assigned V1 actor authority

Every accepted interactive hold, release, retry, or other audited Publisher
decision SHALL record append-only, versioned action evidence. The server SHALL
assign the V1 interactive actor as `local_operator` and automatic/system or
recovery actions as `system`. Client requests MAY supply a reason and allowed
action parameters only; a client-supplied actor identity SHALL NOT be trusted
or persisted as the decision actor. Actor references SHALL retain a
source/kind and identifier shape that can later represent a
server-authenticated principal. This requirement SHALL NOT add login,
authentication, or authorization infrastructure. Operator action evidence
SHALL NOT mutate or rewrite historical attempt evidence.

#### Scenario: Operator requests a hold or retry

- **WHEN** a client requests an audited Publisher action with a reason and
  action parameters
- **THEN** the server records the action as `local_operator`, ignores or
  rejects any client actor identity, and appends versioned audit evidence

#### Scenario: Publisher recovery acts on an expired claim

- **WHEN** the system conditionally reacquires an expired attempt lease
- **THEN** the recovery action is recorded as `system` without changing
  completed historical attempt evidence

### Requirement: Publisher owns durable claims and nonterminal recovery

Publisher SHALL atomically claim work with a conditional `READY` to
`PUBLISHING` transition that creates one active attempt identity, claim token
or fencing generation, and bounded lease expiry. Only one active attempt
SHALL exist per publication identity. Active owners SHALL renew their lease
and validate the current fencing generation before each durable phase or
filesystem mutation. An unexpired claim SHALL NOT be reacquired. After lease
expiry, recovery SHALL conditionally reacquire only when persisted state,
claim token/generation, and expiry still match; reacquisition SHALL retain the
logical attempt identity and replace its ownership token/generation.

Publisher SHALL integrate claim, renewal, expiry, and recovery into the
existing AutoPublisher periodic processing and startup cadence. Recovery
SHALL query persisted nonterminal publication records and inspect only exact
paths in their persisted manifests; it SHALL NOT walk Library or RAW
filesystems. `READY` records may be claimed; only expired `PUBLISHING` claims
are reconciled; `HELD`, `BLOCKED`, and `FAILED` records SHALL NOT trigger
filesystem recovery. This SHALL NOT add a generic scheduler or another Queue.

#### Scenario: Concurrent workers claim one ready publication

- **WHEN** multiple Publisher workers attempt to claim the same `READY`
  publication
- **THEN** exactly one atomic claim succeeds and at most one active attempt
  exists for that publication identity

#### Scenario: Expired lease is conditionally reacquired

- **WHEN** a persisted `PUBLISHING` claim expires and recovery observes the
  same stored claim token/generation and expiry
- **THEN** one recovery worker reacquires that logical attempt with a new
  fencing generation, while stale owners cannot continue filesystem
  mutations

#### Scenario: Recovery cadence checks pending publications

- **WHEN** startup or AutoPublisher periodic processing reconciles recovery
- **THEN** it queries persisted nonterminal publication records and their
  manifest paths without scanning Library or RAW roots or creating another
  scheduler/Queue

### Requirement: Frozen destination and explicit conflict policy are honored

Publisher SHALL use the destination Library, resolved destination path, and
publish mode frozen by Queue/ExecutionPlan. It SHALL NOT calculate a
replacement destination or overwrite permission from current Settings. Missing
frozen destination identity, a changed Library/root, or an unsafe controlled
root SHALL fail closed as `BLOCKED`.

Standard mode SHALL NOT overwrite an occupied target. Replace-library mode
SHALL be explicit in the frozen Queue intent and SHALL replace only the
unambiguous current Library publication recorded as its target. A manual
Publisher control or byte/path equality SHALL NOT create overwrite authority.
Publisher SHALL preserve and restore proven replacement targets on a safely
reconciled pre-publication failure.

#### Scenario: Library destination changed after Queue freeze

- **WHEN** the current Library/root no longer matches the frozen destination
- **THEN** Publisher blocks the publication and does not reroute it using
  current Settings

#### Scenario: Existing unrelated destination conflicts

- **WHEN** a standard target is occupied, or a replace target's Library/source
  identity is missing, stale, or ambiguous
- **THEN** Publisher records a `BLOCKED` conflict and leaves the existing file
  untouched

### Requirement: Media and required sidecars commit through controlled staging

Publisher SHALL resolve required media sidecars from the bound Result and
frozen plan. It SHALL materialize media and each required sidecar in private
destination-side temporary files, preserve the canonical Result source, flush
staged data, and promote each staged file by an atomic same-filesystem rename.
Cross-filesystem sources SHALL be copied to destination-side temporary files
before promotion. Required sidecar absence, unreadability, or promotion failure
SHALL prevent publication success. Publisher SHALL contain all source,
temporary, backup, and final paths within their controlled roots and SHALL
reject symlinks, non-regular files, path escapes, and unsafe parent changes at
the mutation boundary.

#### Scenario: Required sidecar cannot be delivered

- **WHEN** any plan-required sidecar is absent, unreadable, or cannot be
  committed to its final destination
- **THEN** the publication is not `PUBLISHED`, the original is not archived,
  and the system safely rolls back attempt-owned partial files or records a
  `BLOCKED` state if ownership is ambiguous

#### Scenario: Space or filesystem operation fails

- **WHEN** free space is insufficient or read/write/flush/rename fails before
  the success transaction
- **THEN** Publisher records a bounded attempt failure, reconciles only files
  owned by that attempt, and marks `FAILED` only when retry is safe

### Requirement: Publication success has one durable boundary

Publication SHALL become canonical `PUBLISHED` only after the final media and
all required sidecars are committed and verified against the durable attempt
manifest, and one database transaction persists publication provenance and
the QueueJob publication projection. A process/API response or filesystem
path alone SHALL NOT establish success. The original SHALL be archived only
after this boundary, using #174's existing archive/AssetRecord authority.
Archive status SHALL remain a separate fact and archive failure SHALL NOT
roll back `PUBLISHED`.

#### Scenario: Process stops between media, sidecars, and database commit

- **WHEN** a process stops after some/all final file renames but before the
  database success transaction
- **THEN** the row remains `PUBLISHING`; recovery verifies attempt receipts and
  frozen identities, resumes safe missing promotions or safely restores
  attempt-owned changes, and does not archive or emit an event until success

#### Scenario: Original archive fails after publication commit

- **WHEN** the final asset and required sidecars are committed and persisted as
  `PUBLISHED` but the #174 original archive operation fails
- **THEN** the publication remains `PUBLISHED`, the archive outcome is recorded
  separately through existing provenance/attempt evidence, and no final asset
  or sidecar is rolled back

### Requirement: Recovery preserves publication and archive identity

Recovery SHALL operate from persisted publication attempts and exact path
manifests, not Library/RAW directory scans. It SHALL verify the same Result
revision, frozen destination, and attempt-owned file receipts before resuming,
restoring, or finalizing. It SHALL repair interrupted publication provenance
and interrupted #174 archive-record persistence through their existing
authorities. Ambiguous file ownership or changed identity SHALL fail closed as
`BLOCKED`; recovery SHALL NOT overwrite or remove an unowned file.

#### Scenario: Process stops after file commit but before publication row commit

- **WHEN** all final media and required sidecars are present with receipts for
  the active attempt but the database transaction did not commit
- **THEN** recovery commits publication provenance and `PUBLISHED` exactly once
  without rematerializing or inferring identity from path equality

#### Scenario: Process stops during original archival

- **WHEN** the original was moved but the existing #174 AssetRecord/provenance
  write was interrupted
- **THEN** recovery completes the existing archive recording path using the
  persisted archive locator and canonical source/publication identities,
  without creating another archive model

### Requirement: AssetPublished is emitted once after committed delivery

The system SHALL create one stable `AssetPublished` outbox event identity per
publication only after the final asset and all required sidecars are
committed, `PUBLISHED` provenance is persisted, and the original archive
outcome has been recorded. The event SHALL reference canonical Queue, Result,
Validation, Library, delivered artifacts, and available #174 archive
provenance. Subscriber failure or ambiguous delivery acknowledgement SHALL
retry the same event identity and SHALL NOT reverse publication, repeat
archival, or create a second logical event. No Jellyfin-specific behavior
belongs in Publisher.

#### Scenario: Required files are not all committed

- **WHEN** any required final asset or sidecar remains incomplete
- **THEN** no `AssetPublished` event is made available to subscribers

#### Scenario: Subscriber fails after publication

- **WHEN** a downstream subscriber fails or event acknowledgement is
  ambiguous after the outbox event is created
- **THEN** delivery is retried with the same event identity while publication
  stays `PUBLISHED`

### Requirement: Publisher and History are exception-first and read-only

Publisher SHALL default to `HELD`, `BLOCKED`, `FAILED`, stale-handoff, and
destination-conflict exceptions, with persisted phase/reason and safe next
action. History SHALL retain publication identity/state/attempt summary and
show original archive status separately using existing lifecycle evidence.
Legacy Queue jobs without a bound publication identity SHALL remain explicitly
legacy. List, search, page rendering, and polling SHALL use bounded stored
summaries and SHALL NOT scan filesystems, probe media, recalculate Validation,
or compute full-file hashes.

#### Scenario: Publisher or History renders and polls

- **WHEN** an operator opens, filters, pages, or refreshes Publisher or History
- **THEN** the API returns persisted batch summaries without filesystem scans
  or media/validation work

#### Scenario: Operator opens an exception

- **WHEN** an operator opens a held, blocked, or failed publication
- **THEN** the UI shows its stored bounded attempt evidence and explicit
  resolution/retry guidance without changing canonical publication identity
