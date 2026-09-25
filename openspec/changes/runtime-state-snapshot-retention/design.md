## Context

The current runtime detector builds a `RuntimeSnapshot` containing both
volatile host telemetry and stable scheduler inputs, then inserts it on every
detection. `runtimeinfo.Latest()` consequently means “newest history row” even
when callers need current state. The API startup path, the five-minute
detector, `/api/system/runtime/refresh`, runtime-policy updates, the planner,
resource reevaluation, and Test Encode preparation all participate in that
ambiguity.

The existing database already has nullable indexed
`ExecutionPlan.RuntimeSnapshotID` and `TestEncode.RuntimeSnapshotID` fields.
Housekeeping is owned by `StartAutoHousekeeper` and `RunHousekeeping`, with
preview/run endpoints and existing workspace/Test Encode cleanup. SQLite has a
single-writer constraint, so capture, provenance binding, and pruning need an
explicit transaction contract rather than independent “latest row” reads.

See `proposal.md` for the motivation and `specs/runtime/spec.md` for the
observable contract.

## Goals / Non-Goals

**Goals:**

- Make a dedicated singleton `RuntimeState` the authority for current host
  telemetry, capabilities, and effective runtime policy.
- Make stable execution evidence canonical, versioned, deterministic, and
  immutable when captured.
- Preserve explainable plan and Test Encode provenance while preventing waiting
  reevaluation and dashboard polling from generating history.
- Add one reference-aware, bounded pruning sub-operation to existing
  housekeeping with safe migration and backward-compatible reporting/API
  behavior.

**Non-Goals:**

- No runtime detector, encoder capability, or scheduler-policy redesign.
- No metrics/time-series database or indefinite telemetry history.
- No new cleanup scheduler, retention Settings UI, remote-worker redesign, or
  source/published media changes.
- No automatic SQLite `VACUUM` or general database compaction.

## Decisions

### 1. Use a dedicated `RuntimeState` table with an explicit singleton key

`RuntimeState` is a separate mutable model/table, not a designated mutable
`RuntimeSnapshot` row. It carries the current detection fields, a
`stableFingerprint`, a fingerprint schema version, and detection/update times.
The table has one unique logical identity such as `singletonKey = "host"`;
the technical row ID is an implementation key and is never exposed as an
immutable snapshot ID. Upsert is performed against that unique identity inside
the runtime-state write transaction, so there is no hidden “latest row wins”
rule.

The model initially mirrors the fields needed by current runtime consumers so
the detector does not create a second partial representation. `RuntimeSnapshot`
remains immutable and gains explicit capture metadata (stable fingerprint and
capture reason/version as needed), but existing historical values are not
rewritten.

**Alternative considered:** Reuse the newest `RuntimeSnapshot` as the mutable
state row. Rejected because it conflates audit identity with live state, makes
foreign-key protection and historical explanation ambiguous, and leaves every
current-state refresh coupled to history retention.

### 2. Define `RuntimeStableFingerprint` v1 from a canonical stable payload

The fingerprint payload is a versioned object serialized with canonical JSON
semantics and hashed with SHA-256. The stored representation is
`runtime-stable-v1:<lowercase-hex-sha256>`; the version is part of both the
payload and the encoded value.

The v1 payload contains normalized values for:

- OS, architecture, container state, CPU core count, and total memory;
- stable GPU presence/capability and metric-capability descriptors;
- the complete normalized encoder capability map, including tested modes and
  capability reasons that affect encoder selection;
- recommended, selected, preferred, and fallback profiles;
- effective runtime policy values, policy mode, and applied override keys/
  values;
- each configured disk's role, normalized identity/path, storage type, and
  stable capacity descriptor used by execution decisions.

Object keys are sorted recursively, arrays whose meaning is set-like are
normalized deterministically, and absent/empty values use one documented
normal form. Timestamps, database IDs, `createdAt`, detection order, and
serialization/map insertion order are excluded.

The following remain in `RuntimeState` and may be copied into a capture but do
not affect the stable fingerprint: CPU load, available memory, GPU utilization
percentages, battery percentage, current power source/on-battery state, free
disk bytes, transient warnings, and other rapidly changing counters. Battery
presence and total memory are stable capability evidence; battery state and
available memory are resource gates.

**Alternative considered:** Hash the whole JSON-serialized detector result.
Rejected because map ordering, timestamps, IDs, free-space counters, and
telemetry would create false transitions and duplicate audit rows.

### 3. Make `Current`, `LatestAuditSnapshot`, and `CaptureAuditSnapshot` distinct

The runtime package will expose explicit semantic boundaries equivalent to:

- `Current(db)`: read the singleton live state; it never creates history;
- `UpsertDetectedState(tx, detected)`: atomically apply a detection and handle
  the stable-transition comparison;
- `LatestAuditSnapshot(db)`: read newest immutable evidence only when an audit
  view is requested;
- `CaptureAuditSnapshot(tx, state, reason)`: create one immutable row from a
  specified state and explicit capture reason.

`DetectAndSave` is retired or becomes an explicitly named compatibility
boundary whose behavior is “detect and upsert current state,” not “save a
snapshot.” A compatibility wrapper may remain temporarily only if its name
does not make current state look like audit history; all migrated callers must
declare whether they need `Current` or immutable evidence.

The detector probes may run before opening the write transaction. The state
upsert, previous-fingerprint comparison, optional transition capture, and
current-state commit occur in one transaction. A required automatic
stable-transition capture failure rolls back the transition/state write so a
new stable authority is not published without its required evidence. Volatile
telemetry-only updates do not require an audit row.

### 4. Capture stable transitions once, with explicit provenance

Automatic `stable_transition` evidence is enabled by the initial policy and is
deduplicated by comparing the incoming fingerprint with the current state's
stored fingerprint inside the write transaction. A same-fingerprint update
only updates current telemetry. A changed fingerprint creates one transition
snapshot in that same transaction, then stores the new current fingerprint.

The state row's previous/current comparison, rather than a global unique
constraint on fingerprint, permits a legitimate A → B → A sequence to produce
one new evidence event on the return to A. Migration initialization from a
legacy snapshot seeds the current fingerprint but intentionally creates no
synthetic transition row.

Runtime-policy writes continue to validate and trigger fresh detection. An
equivalent effective policy produces only a state refresh; a changed effective
policy participates in the normal one-transition rule. Capability-cache reset
remains part of explicit refresh behavior.

### 5. Freeze `ExecutionPlan.RuntimeSnapshotID` once per material plan version

The plan's freeze boundary is the first transaction that has resolved the
material execution configuration (profile/capability authority, encoder,
constraints, and plan version) and persists the plan for evaluation or
dispatch. That transaction reads `Current`, captures a snapshot with reason
`execution_plan_freeze`, sets the plan's runtime profile and snapshot ID, and
commits the plan/reference pair atomically.

`EvaluateResources` is then a dynamic gate, not a snapshot resolver. It reads
current telemetry for free RAM, free disk, battery state, worker availability,
and reservations, but it does not change the frozen runtime profile or
`RuntimeSnapshotID`. A legacy plan with no ID may be bound once at its next
explicit material freeze; the reevaluation loop itself is not a capture
boundary.

Stable policy/capability changes apply to new plan versions. If an existing
waiting plan must adopt them, the planner creates a new material version under
its normal supersession rules instead of mutating the old plan's historical
reference.

When dynamic telemetry materially decides final dispatch, the bounded values
and decision time are recorded in the plan's existing structured evaluation /
decision evidence at the dispatch boundary. This explains the final gate
without replacing the stable audit snapshot or creating one snapshot per
WAITING transition.

**Alternative considered:** Refresh `RuntimeSnapshotID` on each
`EvaluateResources` call. Rejected because it makes a plan's provenance depend
on polling frequency and allows housekeeping to delete the row selected by an
earlier evaluation.

### 6. Freeze Test Encode runtime evidence during preparation

Test Encode capture occurs once after effective Test Encode configuration is
resolved and before FFmpeg execution begins. The preparation transaction reads
`Current`, calls `CaptureAuditSnapshot` with `test_encode_freeze`, and stores
the resulting immutable ID together with effective configuration. The
execution worker uses that frozen ID for metadata and later updates; it never
looks up the current-state technical row ID as a substitute.

If capture or the atomic binding update fails, the Test Encode remains
non-executable/failed in its existing lifecycle and no FFmpeg process starts
with missing or mutable runtime provenance. A retry/new Test Encode receives a
new capture; current-state refreshes after preparation cannot alter the ID.

### 7. Keep current-state consumers on the live boundary

The migration audit covers every current `runtimeinfo.Latest()` use found in
the repository. Dashboard, Settings runtime display, profile detection,
encoder capability lookup, workspace decisions, resource policy, and worker
telemetry use `Current`. Only explicit historical/audit views and the two
capture workflows use immutable snapshot APIs.

The runtime API response preserves existing telemetry/property names and adds
`stateKind: "live"`, `detectedAt`, `stableFingerprint`, and optional
`auditSnapshotId`. The current-state technical ID is omitted from or made
nullable in the client contract; it is never serialized as `id` with the
meaning “immutable host snapshot.” Existing consumers that only read telemetry
fields remain wire-compatible, while clients displaying provenance switch to
the explicit optional audit ID.

Dashboard and Settings use “Live runtime state · detected …” and “Runtime
detection refreshed successfully.” They show “Host snapshot #N” only for an
actual immutable audit representation. The 30-second Dashboard timer is safe
because its POST only updates the singleton state and returns live metadata.

### 8. Centralize retention references

The runtime retention service owns `ReferencedRuntimeSnapshotIDs(db)` (or an
equivalent repository boundary). Its initial providers query every non-null
`execution_plans.runtime_snapshot_id` and `test_encodes.runtime_snapshot_id`,
regardless of lifecycle status. Future canonical provenance owners register a
provider with the same service; deletion callers do not add ad-hoc protection
queries.

The relevant columns remain indexed. Where SQLite foreign keys are practical,
they use delete-restrict semantics; the service remains authoritative even if a
legacy database lacks those constraints, so old referenced IDs are protected
during migration and pruning.

### 9. Use a deterministic seven-day retention policy with bounded batches

The initial constants are:

- referenced snapshots: always retained;
- newest immutable snapshot: always retained, even if unreferenced;
- unreferenced snapshots newer than seven days: retained;
- soft unreferenced-history pressure threshold: 256 rows;
- maximum eligible deletions per housekeeping run: 100 rows.

The count threshold is additive pressure, not permission to delete recent
history: age and the protected-reference/latest rules are evaluated first.
Older unreferenced rows are ordered by `detected_at ASC, id ASC` and selected
in a maximum batch of 100. If fewer than 256 unreferenced rows exist, age still
controls eligibility; the count does not cause recent rows to be deleted.

The preview and run paths use the same candidate resolver. Preview reports
advisory candidates only. The real run opens a write transaction per bounded
batch, re-reads references and the newest-row guard, and deletes only rows
still eligible. A row that becomes referenced between preview and run is
retained.

### 10. Integrate pruning into existing housekeeping

`RunHousekeeping` gains a runtime-snapshot sub-operation after/alongside its
existing workspace and Test Encode cleanup. `StartAutoHousekeeper` remains the
only periodic owner; no runtime-specific ticker or request-triggered cleanup is
added. Preview remains dry-run across all sub-operations.

The report retains `candidates`, `removedPaths`, `recoveredBytes`, and `errors`
for compatibility, and adds a structured `runtimeSnapshots` object with:
`rowsConsidered`, `referencedKept`, `latestKept`,
`recentUnreferencedKept`, `pruneCandidates`, `deleted`, and `errors`. Runtime
rows are never encoded as filesystem paths. `recoveredBytes` changes only for
actual file deletion; runtime row deletion contributes no fake byte count.

### 11. Migrate legacy databases conservatively

`AutoMigrate` adds the `RuntimeState` table and indexes but does not scan and
delete `RuntimeSnapshot` history. A bounded initialization step runs after the
table exists: if the singleton is absent, it copies the newest legacy snapshot
ordered by `detected_at DESC, id DESC`, computes fingerprint v1 from its stable
fields, and marks the state as initialized without creating a transition row.
If no legacy snapshot exists, the first normal detector upsert initializes it.

Existing snapshots, plan IDs, Test Encode IDs, and their JSON provenance remain
unchanged. Legacy unreferenced history is eligible for the normal hourly
housekeeping owner and is removed in 100-row batches. Startup does not perform
a huge destructive cleanup. No migration, detector, API request, or
housekeeping run invokes `VACUUM`.

### 12. Make capture and pruning safe under SQLite concurrency

The detector's state transition and automatic transition capture use a single
database transaction. Plan freeze uses a transaction containing snapshot
insert plus plan row/reference write. Test Encode freeze uses the analogous
capture plus Test Encode update transaction. Housekeeping's deletion check and
delete occur in a transaction and re-query all registered references.

SQLite's single writer serializes these write transactions. Read-only current
state access may observe the prior committed state or the next committed
state, but cannot observe a half-written state/snapshot/reference set. If a
transaction encounters a busy/constraint error, the caller follows existing
error handling and does not proceed with an unbound plan/Test Encode.

### 13. Defer VACUUM and manual compaction

The initial change documents the distinction between deleting rows and
shrinking the SQLite file: `DELETE rows != SQLite file immediately shrinks`.
No automatic compaction is attempted during startup, detection, API refresh,
or housekeeping. A future manual maintenance action may be designed as an
explicit operator-controlled boundary with its own safety/observability
contract, but it is not part of #149.

## Risks / Trade-offs

- [Risk] A stable transition capture failure can make a detection request fail
  rather than silently losing required evidence. → Keep state transition and
  required capture atomic, return the error, and preserve the last committed
  state; volatile-only updates remain independent of snapshot capture.
- [Risk] A dedicated state table adds migration and response-model work. →
  Mirror the existing runtime fields, initialize from the newest legacy row,
  retain wire field names, and make the technical state ID distinct from audit
  IDs.
- [Risk] A soft count threshold does not cap a burst of recent telemetry rows
  immediately. → Preserve the seven-day evidence guarantee and use the 100-row
  bounded batch; future policy changes can add explicit operator-approved
  compaction without changing provenance semantics.
- [Risk] Current consumers may accidentally retain a historical-snapshot
  assumption. → Audit every current `Latest()` reference, provide explicit
  `Current`/audit APIs, and add API/UI regression coverage for live labels.
- [Risk] A pruning race could invalidate a newly selected plan snapshot. →
  Capture plus provenance binding is one transaction, and deletion rechecks
  references inside its own transaction.
- [Risk] SQLite row deletion may not reduce the NAS file immediately. → Keep
  runtime rows out of filesystem candidates and report no recovered bytes for
  database deletion; defer `VACUUM`.

## Migration Plan

1. Add the `RuntimeState` model/table, stable fingerprint/capture metadata,
   indexes, and bounded legacy-state initialization without deleting legacy
   snapshots.
2. Replace detector/startup/API/Settings-policy writes with detection plus
   current-state upsert and stable-transition capture where applicable.
3. Migrate current-state consumers from ambiguous `Latest()` to explicit live
   and audit boundaries.
4. Add the plan-freeze and Test Encode-freeze transactions, then remove
   snapshot-ID replacement from dynamic resource reevaluation.
5. Add canonical reference discovery and bounded retention to existing
   housekeeping, including dry-run/run reports and transactional rechecks.
6. Update API types and Dashboard/Settings labels, then run focused SQLite
   tests for state, fingerprint, capture, plan/Test Encode provenance,
   retention, migration, housekeeping, compatibility, and no-`VACUUM` paths.
7. Run `openspec validate runtime-state-snapshot-retention --type change
   --strict` and the repository's canonical verification during implementation.

Rollback is a code/schema rollback only: immutable historical rows and
references are preserved, the current state can be ignored/reinitialized from
the newest snapshot, and no media or published artifact is modified. The
implementation must not roll back by deleting runtime history or by rewriting
plan/Test Encode provenance.

## Open Questions

None. The singleton identity, fingerprint v1 field boundary, capture reasons,
plan/Test Encode freeze points, retention age/count/batch values, migration
behavior, API compatibility contract, and VACUUM deferral are fixed for this
SDD.
