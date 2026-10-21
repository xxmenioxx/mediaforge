## Purpose

Provides truthful, actionable Housekeeping evidence for maintenance temporaries and empty RAW directories while preserving exact ownership, recovery, and filesystem safety.

## ADDED Requirements

### Requirement: Maintenance temporary diagnosis preserves operation and file evidence

Housekeeping preview SHALL report the owning maintenance operation, asset path, temporary path, status and phase, finish time when available, file presence, safely measured size, ownership, backup/recovery dependency, and an actionable eligibility or blocking reason. A historical reference to a missing file SHALL be reported as already absent with zero recoverable bytes. Active, recovery-required, failed/canceled/interrupted under retention, and ambiguous operations SHALL remain blocked.

#### Scenario: Completed temporary is absent

- **WHEN** a completed maintenance operation references an absent temporary path
- **THEN** preview reports already absent and zero recoverable bytes without offering deletion

#### Scenario: Completed exact temporary may be residual

- **WHEN** a completed operation references a present regular temporary under the configured Library role and checks prove exact operation ownership, no backup/recovery dependency, no active work, no other references, and stable identity
- **THEN** preview may expose only that path for manual cleanup and describes it as a possible residual rather than abandoned

#### Scenario: Active, failed, interrupted, or ambiguous operation

- **WHEN** the operation is active, failed, canceled, interrupted, recovery-required, has a backup dependency, or its evidence is ambiguous
- **THEN** preview blocks cleanup and gives the specific known reason without treating age as proof of safety

### Requirement: Library maintenance cleanup is exact, manual, and revalidated

Housekeeping SHALL remain the only cleanup engine for Library maintenance temporaries. Cleanup SHALL require the existing preview and authorized execution controls, then revalidate the preview token, role roots, operation and asset identity, exact temporary path and file identity, backup/recovery state, references, and active-operation state immediately before removal. Cleanup SHALL be idempotent and SHALL NOT grant general Library deletion permission.

#### Scenario: Approved exact residual is removed

- **WHEN** manual execution uses a current preview for a still-identical eligible temporary
- **THEN** only that exact temporary is removed and the durable outcome and reclaimed bytes are reported

#### Scenario: Candidate changes after preview

- **WHEN** operation state, ownership, roots, references, or file identity changes after preview
- **THEN** execution fails closed, preserves the changed file, and reports a stale or blocked outcome

#### Scenario: Unsafe filesystem identity or recovery dependency

- **WHEN** the candidate is a symlink, non-regular file, unsafe hardlink, traversal path, role overlap, or has backup/recovery dependencies
- **THEN** no file is removed and the reason is reported

### Requirement: Empty RAW directory protection is attributable and fail-closed

Housekeeping preview SHALL identify records and references protecting an empty RAW directory, including source, record identifier, field/path, and lifecycle or operation state when available. It SHALL distinguish verified current protection from unresolved or suspected stale references. Physical emptiness SHALL NOT invalidate a reference or automatically unblock a directory. Historical records SHALL remain unchanged.

#### Scenario: Current reference protects an empty directory

- **WHEN** an empty RAW directory is referenced by a current asset, active Queue/reservation/Publisher/maintenance work, or intentional restore destination
- **THEN** preview identifies the applicable reference and keeps the directory blocked

#### Scenario: Stale reference is suspected

- **WHEN** a reference appears stale but canonical inventory/reconciliation has not independently resolved it
- **THEN** preview labels it suspected stale, keeps the directory blocked, and gives a safe next step when known

#### Scenario: Protection evidence is unavailable

- **WHEN** reference evidence cannot be read or associated with a record
- **THEN** preview fails closed and reports the unresolved protection evidence

### Requirement: RAW directory cleanup remains non-recursive and empty-only

Housekeeping SHALL remove a RAW directory only through the existing controlled preview and execution path when the exact leaf is within the configured RAW role, has no role overlap or protected-root status, is not a symlink, is empty at preview and execution, has stable identity, and has no current or unresolved protected references. Removal SHALL be non-recursive and preserve files, sidecars, assets, originals, destinations, and history.

#### Scenario: Unreferenced empty RAW leaf is eligible

- **WHEN** a safe leaf is literally empty and has no verified or unresolved protection
- **THEN** it is eligible only through controlled Housekeeping and is rechecked before non-recursive removal

#### Scenario: Directory contains data or changes

- **WHEN** a directory is nonempty, gains an entry, is replaced, is a symlink, or has ambiguous ownership
- **THEN** cleanup is blocked and no recursive or file deletion occurs
