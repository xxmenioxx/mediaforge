## Purpose

Defines how MVForge presents a current published Library asset separately from its archived original and how operators can permanently delete that original without weakening identity, filesystem, or provenance protections.

## ADDED Requirements

### Requirement: Library assets show the current published path and retain the original separately

The system SHALL present the current Publisher-confirmed path as the primary location of a converted Library asset. The archived original SHALL remain separately identified as source or archive provenance and SHALL NOT replace the current Library path after publication, inventory refresh, navigation, restart, or deletion of the original. If current publication identity is unresolved or conflicting, the system SHALL preserve the unresolved state instead of inferring a path from the original.

#### Scenario: Publisher confirms the current publication

- **WHEN** Publisher confirms a Queue job as PUBLISHED with a current published path
- **THEN** the Library asset and Asset Info show that path as the current location
- **AND** the original or archived path remains separate lifecycle evidence

#### Scenario: The archived original is deleted

- **WHEN** a confirmed permanent deletion of the archived original succeeds
- **THEN** the Library asset continues to show the current published path
- **AND** Queue, Validation, reports, logs, snapshots, and publication provenance remain available

#### Scenario: Current publication identity is ambiguous

- **WHEN** current publication identity is unresolved or conflicting
- **THEN** the system SHALL NOT present the original path as the current Library path
- **AND** actions requiring resolved identity remain unavailable

### Requirement: Permanent Archive deletion requires current identity and fresh safety checks

The system SHALL require explicit confirmation and a resolved relationship between the archived original, its source, the current Queue publication, and the current Library asset. Before unlinking, the system SHALL revalidate that relationship, active work, configured roots, symlink and file safety, and the protected Library publication. Ambiguous, stale, replaced, missing, or unsafe identities SHALL fail closed without deleting the target.

#### Scenario: A current archived original is explicitly deleted

- **WHEN** the operator confirms deletion and all current identities and filesystem checks pass
- **THEN** the system deletes only the archived original
- **AND** the current Library publication and lifecycle provenance remain intact

#### Scenario: The relationship changes before unlink

- **WHEN** the archive-to-source or current-publication relationship changes after initial validation
- **THEN** the final safety check rejects the deletion
- **AND** the archived original remains physically present

#### Scenario: Active work or an unsafe filesystem target exists

- **WHEN** related active work exists, a configured root changes, a path escapes its root, a symlink is encountered, or the current Library publication is unavailable
- **THEN** the system rejects deletion and preserves the archived original

#### Scenario: Inventory persistence fails after unlink

- **WHEN** unlink succeeds but the inventory update fails
- **THEN** the system reports a partial failure and preserves enough provenance for reconciliation
- **AND** it does not report successful deletion

#### Scenario: A batch contains both eligible and ineligible originals

- **WHEN** a confirmed batch includes originals with different identity or filesystem outcomes
- **THEN** the system reports each success and failure separately
- **AND** one failed item does not authorize or repeat deletion of another item

### Requirement: Archive deletion provides phase timing and prompt feedback

The system SHALL record separate durations for identity resolution, active-work checks, filesystem checks, unlink, inventory persistence, and the Assets refresh and feedback phases. Timing records SHALL include bounded counts needed to interpret a batch and SHALL NOT add full media paths. After the backend confirms deletion, the interface SHALL show success feedback without waiting for the full Assets inventory refresh.

#### Scenario: A single deletion completes

- **WHEN** the backend processes a confirmed single deletion
- **THEN** its timing record distinguishes identity, filesystem, unlink, persistence, and total duration
- **AND** success feedback appears after backend confirmation while Assets refreshes

#### Scenario: A batch deletion completes

- **WHEN** the backend processes a confirmed batch
- **THEN** the timing record distinguishes per-item work and aggregate counts, durations, and outcomes
- **AND** timing data does not add full media paths
