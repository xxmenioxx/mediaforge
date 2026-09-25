## Purpose

Provide one authoritative live runtime state while preserving bounded,
immutable runtime evidence for execution plans, Test Encodes, and explicit
audits without turning routine telemetry refreshes into history.

## ADDED Requirements

### Requirement: Current runtime state is separate from immutable audit history

The system SHALL expose one canonical current runtime state containing the
latest detected OS, architecture/container state, CPU, memory, GPU, power,
disk, encoder, profile, policy, override, reason, warning, and detection-time
values. Updating current state SHALL replace the singleton state atomically and
SHALL NOT append an immutable `RuntimeSnapshot` merely because detection was
performed.

#### Scenario: First detection initializes current state

- **WHEN** the host is detected and no current runtime state exists
- **THEN** the system creates the singleton current state and returns its live
  detection data without requiring or creating an immutable audit snapshot

#### Scenario: Repeated detection updates one current state

- **WHEN** periodic detection or a UI refresh detects the host again
- **THEN** the same current-state identity is updated atomically and the count
  of immutable `RuntimeSnapshot` rows is unchanged unless an explicit capture
  rule applies

#### Scenario: Volatile telemetry changes

- **WHEN** CPU load, available memory, GPU utilization, battery percentage, or
  free disk bytes changes while stable host capabilities remain the same
- **THEN** current state reflects the new telemetry and no duplicate stable
  transition snapshot is created

### Requirement: Stable runtime fingerprint is canonical and version-bound

The system SHALL compute a deterministic `RuntimeStableFingerprint` from a
versioned canonical representation of execution-relevant stable authority. The
representation SHALL include OS, architecture, container state, CPU core count,
total physical memory only when reliably stable, encoder capability booleans and
modes that affect execution, stable GPU presence and codec/encoder capability,
recommended/selected/preferred/fallback profiles, effective runtime policy,
applied overrides with their effective values, and each configured disk's role,
normalized identity/path, and stable storage type. It SHALL use canonical field
names, normalized values, recursively ordered object keys, and an explicit
fingerprint schema version; it SHALL NOT include timestamps or database IDs.

The fingerprint SHALL exclude CPU load, available memory, GPU utilization,
GPU-utilization availability for the current sample, telemetry-only GPU metric
source, transient probe errors, free disk bytes, filesystem counters, transient
warnings, current battery percentage/power source, and capability diagnostic
strings containing transient environment or error text. If a stable enum-like
capability provenance value is required for an execution decision, it SHALL be
normalized into a bounded stable value before hashing; arbitrary capability
reasons SHALL not affect the fingerprint.

#### Scenario: Equivalent map order produces the same fingerprint

- **WHEN** two detections contain the same stable values but JSON/map keys are
  presented in different insertion orders
- **THEN** both detections produce the same versioned stable fingerprint

#### Scenario: Volatile fields are excluded

- **WHEN** only CPU load, available RAM, GPU utilization, battery percentage,
  power-source state, or free disk bytes changes
- **THEN** the stable fingerprint remains unchanged

#### Scenario: Transient capability reason is excluded

- **WHEN** an encoder keeps the same capability booleans/modes but its probe
  reason changes between equivalent transient messages such as timeout,
  temporary device busy, or permission-read failure
- **THEN** the stable fingerprint remains unchanged

#### Scenario: GPU telemetry source is unavailable

- **WHEN** GPU utilization or the source used to collect the current GPU
  sample becomes temporarily unavailable while GPU presence/capability is
  unchanged
- **THEN** the stable fingerprint remains unchanged

#### Scenario: Stable capability or policy changes

- **WHEN** encoder capability, GPU capability, disk role/type identity, runtime
  profile selection, or effective runtime policy changes
- **THEN** the canonical stable fingerprint changes

#### Scenario: Encoder usability changes

- **WHEN** an execution-relevant encoder capability changes from `usable=true`
  to `usable=false`, or an execution-relevant mode such as `main10` or
  `adaptiveB` changes
- **THEN** the canonical stable fingerprint changes

#### Scenario: Disk identity changes without free-space change

- **WHEN** a configured disk role, normalized device/path identity, or stable
  storage type changes
- **THEN** the canonical stable fingerprint changes

### Requirement: Immutable runtime snapshots require an explicit capture boundary

The system SHALL create immutable `RuntimeSnapshot` evidence only through a
canonical capture operation with an explicit reason. Supported reasons SHALL
include `execution_plan_freeze`, `test_encode_freeze`, `stable_transition`,
and `diagnostic_audit` (or equivalent stable API values). Dashboard polling,
periodic detection, opening Settings, GET current runtime, and ordinary runtime
refresh SHALL not capture immutable evidence by themselves.

#### Scenario: Dashboard polling remains history-safe

- **WHEN** the Dashboard performs its 30-second refresh repeatedly
- **THEN** current state is refreshed and no immutable snapshot is appended
  solely because of those polls

#### Scenario: Explicit diagnostic capture

- **WHEN** an authorized diagnostic/audit action explicitly requests a capture
- **THEN** exactly one immutable snapshot is created from the current state with
  the requested reason and its stable fingerprint/provenance

#### Scenario: Equivalent runtime-policy write

- **WHEN** runtime policy is written with an effective value equivalent to the
  current policy
- **THEN** current state is refreshed, but no duplicate transition snapshot is
  created

### Requirement: Stable transitions produce one deduplicated audit event

When automatic stable-transition evidence is enabled by the initial policy, a
detected stable fingerprint transition SHALL create at most one immutable
snapshot for the new transition state, with reason `stable_transition`. The
same fingerprint SHALL not create repeated transition snapshots while it
remains current. A transition back to an earlier fingerprint SHALL be treated
as a new transition event. Initializing state from legacy history SHALL not
manufacture a transition snapshot.

#### Scenario: Fingerprint A transitions to B

- **WHEN** a successful atomic detection changes the current fingerprint from A
  to B
- **THEN** one transition snapshot for B is captured and subsequent detections
  with B do not append more transition snapshots

#### Scenario: Fingerprint returns to A

- **WHEN** the current fingerprint changes from B back to A after A was seen
  previously
- **THEN** the return is eligible for one new transition snapshot because it is
  a new transition event

### Requirement: Current-state APIs remain compatible and identify provenance

`GET /api/system/runtime` SHALL continue to return current runtime information
using the existing telemetry and policy field meanings. `POST
/api/system/runtime/refresh` SHALL reset relevant capability caches, perform
fresh detection, atomically upsert current state, and return that live state.
Responses SHALL expose explicit metadata equivalent to `stateKind: live`,
`detectedAt`, and `stableFingerprint`. If a compatibility `id` field is
returned, it SHALL mean only the mutable `RuntimeState` technical row ID and
SHALL never mean or be labeled as an immutable snapshot ID. The V1
`auditSnapshotId` field SHALL be omitted or null for normal live GET responses
and for ordinary refreshes whose stable fingerprint is unchanged. A refresh
that atomically creates a `stable_transition` audit snapshot MAY return that
new immutable ID as transition provenance; later live refreshes SHALL omit or
null the field rather than carry that old ID forward as if current telemetry
were the snapshot. If the response type cannot make this distinction clear,
the field SHALL be named `transitionAuditSnapshotId` instead. Existing field
names and clients that consume current telemetry without provenance assumptions
SHALL remain compatible.

#### Scenario: GET has no history to create

- **WHEN** a client calls GET current runtime after state has been initialized
- **THEN** the response is live current state and the call does not create an
  immutable snapshot

#### Scenario: Unchanged refresh has no audit identity

- **WHEN** a client posts a runtime refresh and the stable fingerprint is
  unchanged
- **THEN** the response is `stateKind: live`, includes current
  `stableFingerprint` and `detectedAt`, and omits or returns null for audit
  transition provenance

#### Scenario: Stable-transition refresh returns bounded provenance

- **WHEN** a refresh atomically changes the stable fingerprint and captures one
  `stable_transition` snapshot
- **THEN** that response may include the newly created immutable ID as
  `auditSnapshotId` (or the explicitly named transition field), and a later
  live refresh with no new transition omits or nulls that field

#### Scenario: Refresh failure is reported without partial state

- **WHEN** fresh detection or the atomic state update fails
- **THEN** the endpoint returns an error and does not expose a partially written
  current state or an audit ID for an uncommitted snapshot

### Requirement: Current-state consumers do not use ambiguous audit semantics

Dashboard, Settings runtime views, encoder recommendation/capability lookup,
workspace/resource policy, worker runtime telemetry, and other current-state
consumers SHALL read the canonical current state. Immutable snapshots SHALL be
read only through explicit audit/provenance APIs such as current state,
latest audit snapshot, or capture audit snapshot with distinct semantics.
No compatibility wrapper named `Latest` SHALL silently mean both live state and
latest immutable history.

#### Scenario: Recommendation reads live capabilities

- **WHEN** an encoder recommendation or runtime-policy view needs current host
  capabilities
- **THEN** it reads current state without creating or requiring a new audit
  snapshot

#### Scenario: Audit lookup is explicit

- **WHEN** a caller needs immutable historical evidence
- **THEN** it requests an explicit audit snapshot/provenance operation and does
  not receive a live state row as historical evidence

### Requirement: Execution plan provenance freezes once per material plan version

Each material `ExecutionPlan` version SHALL capture one immutable runtime
snapshot when its resolved execution authority is frozen, before the plan is
persisted for evaluation/dispatch. The plan's `RuntimeSnapshotID` SHALL remain
unchanged for that version, including while the plan is waiting or being
reevaluated. Stable profile/capability decisions SHALL come from the frozen
evidence; dynamic resource gates SHALL be evaluated from current state. The
freeze operation SHALL be idempotent: if the material plan version already has
`RuntimeSnapshotID` set, repeated evaluation, retry, polling, or status
handling SHALL reuse that ID and SHALL not capture another snapshot. Only a
genuinely new material plan version MAY capture new evidence.

#### Scenario: Plan freeze captures provenance

- **WHEN** a planner resolves a material plan version and freezes its execution
  configuration
- **THEN** the plan and its immutable runtime snapshot reference are committed
  atomically and the reference explains the stable execution authority

#### Scenario: WAITING reevaluation does not replace provenance

- **WHEN** free RAM, free disk, battery state, worker capacity, or another
  dynamic resource changes and the plan is reevaluated
- **THEN** the reevaluation uses current state, may change the waiting decision,
  but does not create an unbounded snapshot sequence or replace
  `RuntimeSnapshotID`

#### Scenario: Repeated plan evaluation is idempotent

- **WHEN** the same material plan version is evaluated or retried after its
  `RuntimeSnapshotID` has already been persisted
- **THEN** it reuses the existing immutable snapshot reference and the
  immutable snapshot count does not increase

#### Scenario: Final dynamic evidence remains explainable

- **WHEN** dynamic telemetry materially determines the final dispatch decision
- **THEN** the bounded values used for that decision are retained in the
  plan's structured decision evidence or one bounded audit capture, without
  silently replacing the plan's frozen snapshot ID

### Requirement: Test Encode captures immutable runtime evidence at freeze

An explicit Test Encode SHALL capture one immutable runtime snapshot when its
effective execution configuration is frozen/prepared, before encoding begins.
`TestEncode.RuntimeSnapshotID` SHALL reference that immutable evidence and
SHALL not be populated from the current-state row identity. Later current-state
updates SHALL not change the reference. Preparation SHALL be idempotent for the
same Test Encode record: once `RuntimeSnapshotID` is set, polling, status
handling, or retry of that execution record SHALL not capture another snapshot.
A genuinely new Test Encode record/operation MAY capture new evidence.

#### Scenario: Test Encode starts with one frozen snapshot

- **WHEN** Test Encode preparation resolves its effective profile, encoder, and
  execution configuration
- **THEN** preparation captures one immutable snapshot and stores its ID with
  the frozen Test Encode configuration before FFmpeg execution

#### Scenario: Test Encode capture fails safely

- **WHEN** immutable evidence cannot be committed during Test Encode freeze
- **THEN** the Test Encode does not begin execution with an unbound or mutable
  runtime reference

#### Scenario: Prepared Test Encode retry is idempotent

- **WHEN** a prepared Test Encode record is polled, status-updated, or retried
- **THEN** its existing `RuntimeSnapshotID` is reused and no additional
  immutable snapshot is captured

### Requirement: Runtime snapshot references have one retention boundary

The system SHALL provide one canonical reference-discovery boundary for all
runtime snapshot IDs protected from deletion. It SHALL initially discover
references from `ExecutionPlan.RuntimeSnapshotID` and
`TestEncode.RuntimeSnapshotID`, and SHALL be extensible for future registered
canonical provenance references without scattering protection queries across
callers.

#### Scenario: Referenced plan history is discoverable

- **WHEN** a plan references an immutable runtime snapshot
- **THEN** the retention boundary reports that ID as referenced even if the plan
  is waiting, completed, or otherwise not the newest plan

#### Scenario: Future provenance source is registered

- **WHEN** a future canonical audit reference is added to the registry
- **THEN** retention protects its IDs through the same discovery boundary
  without requiring unrelated deletion paths to be edited

### Requirement: Retention is deterministic, bounded, and reference-safe

The default runtime snapshot policy SHALL always retain referenced snapshots and
the newest immutable snapshot, retain unreferenced snapshots younger than seven
days, and make every unreferenced snapshot aged at least seven days eligible
for pruning. Eligible rows SHALL be ordered oldest first by detection time then
ID, and housekeeping SHALL delete at most 100 eligible rows per run. V1 SHALL
have no count threshold or hidden history-count configuration. Retention SHALL
never claim that deleting rows reclaimed equivalent filesystem bytes.

#### Scenario: Referenced old snapshot is retained

- **WHEN** an old snapshot is referenced by an Execution Plan or Test Encode
- **THEN** it is kept regardless of age

#### Scenario: Newest and recent unreferenced rows are retained

- **WHEN** housekeeping evaluates the newest immutable row or an unreferenced
  row less than seven days old
- **THEN** it is kept and excluded from the deletion candidates

#### Scenario: Old unreferenced rows are bounded

- **WHEN** unreferenced rows are at least seven days old
- **THEN** they are eligible immediately, the oldest eligible rows are selected
  deterministically, and no more than 100 are deleted in one run

#### Scenario: Fewer than 256 old rows are still eligible

- **WHEN** 50 unreferenced snapshots are at least seven days old
- **THEN** all 50 are eligible even though the history count is below 256

#### Scenario: Recent history is protected by age

- **WHEN** 500 unreferenced snapshots are younger than seven days
- **THEN** none is deleted before reaching the seven-day age boundary

#### Scenario: Large old history is deleted oldest first

- **WHEN** 500 unreferenced snapshots are at least seven days old
- **THEN** the oldest 100 are deleted in this housekeeping run and the
  remaining eligible rows wait for later bounded runs

### Requirement: Housekeeping owns runtime snapshot pruning and reporting

Runtime snapshot pruning SHALL be one sub-operation of the existing
housekeeping run. It SHALL not add another scheduler, periodic goroutine,
runtime GET/refresh deletion path, or automatic `VACUUM`. The backward-
compatible housekeeping report SHALL add a structured runtime snapshot section
with at least rows considered, referenced kept, latest kept, recent
unreferenced kept, prune candidates, deleted rows, and errors. Database rows
SHALL not be represented as fake filesystem candidates, and recovered file
bytes SHALL count only actual file cleanup.

#### Scenario: Dry-run previews without deletion

- **WHEN** housekeeping preview runs in dry-run mode
- **THEN** it reports deterministic runtime snapshot candidates and counts but
  deletes no database rows and does not report recovered filesystem bytes for
  them

#### Scenario: Housekeeping run deletes bounded candidates

- **WHEN** the existing housekeeping run executes normally
- **THEN** it performs the bounded runtime snapshot sub-operation, reports kept
  and deleted counts, and preserves existing workspace/Test Encode report
  fields and behavior

### Requirement: Pruning rechecks references transactionally

Runtime snapshot deletion SHALL re-evaluate reference protection inside the
deletion transaction. Capture plus the write of its Execution Plan or Test
Encode provenance reference SHALL be atomic, or use an equivalent protection
contract, so housekeeping cannot delete a snapshot between capture and
reference persistence. A stale preview SHALL not authorize deletion without a
fresh eligibility check.

#### Scenario: Capture races with housekeeping

- **WHEN** an Execution Plan or Test Encode is binding a newly captured snapshot
  while housekeeping considers that snapshot
- **THEN** SQLite serialization/transaction boundaries ensure the reference is
  visible to the deletion check or the candidate is retained

#### Scenario: Reference appears after preview

- **WHEN** a snapshot becomes referenced after a dry-run preview but before the
  real housekeeping run
- **THEN** the real run rechecks the reference and does not delete it

### Requirement: Existing databases migrate without destructive startup cleanup

Adding current-state persistence SHALL preserve all existing immutable
snapshots and referenced IDs. If no current state exists, migration/startup
SHALL initialize it from the newest existing immutable snapshot without
creating a synthetic transition audit row. Legacy unreferenced history SHALL be
cleaned only through the bounded housekeeping sub-operation, not through a
large unbounded `AutoMigrate()` action. Normal runtime detection and
housekeeping SHALL never invoke SQLite `VACUUM` automatically.

#### Scenario: Legacy state is initialized

- **WHEN** an existing database has immutable snapshots but no current state
- **THEN** the newest snapshot's current values and computed stable fingerprint
  initialize current state, all legacy rows remain immutable, and referenced
  IDs remain valid

#### Scenario: Large legacy history is bounded

- **WHEN** an existing database contains many old unreferenced snapshots
- **THEN** startup/migration performs no unbounded destructive scan or rewrite;
  later housekeeping processes only its bounded batch per run

#### Scenario: SQLite file size is not overstated

- **WHEN** pruning deletes runtime snapshot rows
- **THEN** the report does not claim equivalent filesystem byte recovery, and no
  automatic `VACUUM` is invoked

### Requirement: Live and immutable evidence have distinct UI semantics

Dashboard and Settings SHALL label current telemetry as live runtime state,
including its detection time, and SHALL show a host snapshot number only when
the user is explicitly viewing immutable audit evidence. Even when a live
refresh returns transition audit provenance, the main object SHALL remain
visibly live and that optional ID SHALL not be rendered as `Host snapshot #...`.
Settings refresh success SHALL describe detection/state refresh and SHALL not
claim that a snapshot was recorded unless a capture actually occurred.

#### Scenario: Dashboard displays live state

- **WHEN** Dashboard renders the response from GET or refresh runtime
- **THEN** it uses live-state wording such as `Live runtime state` with the
  detection time and does not display a fabricated host snapshot number,
  including when the response contains transition audit provenance

#### Scenario: Immutable evidence is shown explicitly

- **WHEN** a plan, Test Encode, or explicit audit view displays immutable
  runtime evidence
- **THEN** it may show `Host snapshot #<id>` with the corresponding provenance
  context
