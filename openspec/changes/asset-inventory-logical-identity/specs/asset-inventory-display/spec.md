## Purpose

Defines how the Assets inventory distinguishes a logical media/title identity from
the physical path used for storage and path-scoped operations.

## ADDED Requirements

### Requirement: Logical identity is primary in Library inventory

Library inventory rows SHALL present the logical media/title/group identity as
their primary collapsed-row label. The system SHALL NOT use generic path-scope
labels such as `Root` or `Season1` as the primary identity when a logical title
is available.

#### Scenario: Library root path has a logical title

- **WHEN** a Library asset group belongs to the logical title `Berserk` and its physical group is the logical root
- **THEN** the collapsed Library row presents `Berserk` as the primary identity and does not present `Root` as the title

#### Scenario: Library child path has a logical title

- **WHEN** a Library asset group belongs to `Attack on Titan` and its physical path is `Season1`
- **THEN** the collapsed Library row presents `Attack on Titan` as the primary identity and MAY present `Season1` only as secondary structural context

### Requirement: Converted inventory uses logical identity

Converted inventory rows SHALL follow the same logical-title-first presentation
rule as Library inventory. Converted provenance/status SHALL NOT cause physical
path scope to become the primary media identity.

#### Scenario: Converted title is displayed

- **WHEN** a converted group has canonical logical-title evidence
- **THEN** the collapsed row presents that logical title before any physical path qualifier

### Requirement: Physical path remains available as operational evidence

Changing the collapsed Library/Converted labels SHALL NOT remove physical path
information from MVForge. Full path data SHALL remain available to functionality
that requires it, including search and path-scoped operations.

#### Scenario: Search by full path

- **WHEN** a user searches for an absolute or relative path that belongs to a Library or Converted group
- **THEN** the matching group remains discoverable even if that full path is not rendered as the collapsed row's primary label

#### Scenario: Path-scoped action uses canonical path

- **WHEN** the user invokes a path-scoped configuration, snapshot, Queue, rename, move, or other supported operation
- **THEN** the operation continues using the canonical physical path rather than the display title

### Requirement: Unprocessed retains title-to-path hierarchy

The Unprocessed inventory SHALL continue to distinguish logical titles from their
physical child paths because Root/Season/Specials/path labels are explicit
configuration and selection scopes there.

#### Scenario: Nested Unprocessed title

- **WHEN** an Unprocessed logical title contains Root and `Season1` paths
- **THEN** the logical title remains the parent identity and Root/`Season1` remain visible child path labels

### Requirement: Inventory display change is presentation-only

The logical-title display correction SHALL NOT mutate inventory grouping,
filesystem paths, lifecycle state, publication provenance, profile assignments,
Queue plans, or persisted asset identity.

#### Scenario: Display correction is applied

- **WHEN** MVForge changes a Library or Converted row from path-first to logical-title-first presentation
- **THEN** the underlying asset/group IDs, paths, statuses, assignments, Queue behavior, and publication evidence remain unchanged
