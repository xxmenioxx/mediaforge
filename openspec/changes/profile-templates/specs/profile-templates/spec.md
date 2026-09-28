## Purpose

Keep built-in Video Profile choices read-only and versioned while users create independent editable profiles from them. This capability also preserves existing profile assignments and frozen Queue evidence during migration.

## ADDED Requirements

### Requirement: Built-in templates are served from one canonical registry

The system MUST expose each built-in Video Profile Template with a stable key, version, name, category, description, and complete profile definition through one backend-owned catalog API. Templates MUST be read-only and MUST NOT be stored or seeded as assignable User Video Profiles. A template version change MUST affect only future clones.

#### Scenario: List built-in templates
- **WHEN** a client requests the template catalog
- **THEN** the response contains the versioned template metadata and definition for every built-in Video Profile Template
- **AND** the response comes from the same catalog used to create profiles from templates

#### Scenario: Template definitions cannot be changed
- **WHEN** a client attempts to update or delete a built-in template
- **THEN** the system rejects the mutation and the registry definition remains unchanged

#### Scenario: A template is revised
- **WHEN** a built-in template receives a new version and definition
- **THEN** new clones use the new version while existing User Video Profiles retain their saved definition and provenance

### Requirement: Template creation produces an independent User Video Profile

The system MUST create a persisted User Video Profile by copying the selected template definition and recording its template key and version. The created profile MUST be independently editable, and edits MUST NOT change its template or another profile cloned from the same template.

#### Scenario: Clone a template
- **WHEN** a client creates a User Video Profile from a known template
- **THEN** the system persists a distinct profile with the template definition, requested user name and scope, and matching template key/version provenance
- **AND** the returned profile can be edited and assigned as a User Video Profile

#### Scenario: Create a blank profile
- **WHEN** a client creates a profile without selecting a template
- **THEN** the system creates an editable User Video Profile with no template provenance

#### Scenario: Unknown or stale template version
- **WHEN** a client requests a clone from an unknown template or unsupported version
- **THEN** the system rejects the request without creating a profile

### Requirement: Only User Video Profiles can be assigned

The system MUST accept Video Profile assignments only for persisted, active User Video Profiles. Built-in template definitions MUST NOT be accepted as assignment targets.

#### Scenario: Assign a user profile
- **WHEN** an assignment references an existing active User Video Profile with a compatible scope
- **THEN** the assignment is saved against that profile's stable ID

#### Scenario: Assign a template directly
- **WHEN** an assignment references a template instead of a persisted User Video Profile
- **THEN** the system rejects the assignment and leaves existing assignments unchanged

### Requirement: Guided Setup consumes the canonical catalog

Guided Setup MUST display its template choices from the canonical template catalog and MUST provide a Blank creation choice. It MUST NOT maintain a second copy of built-in profile definitions in the frontend.

#### Scenario: Load Guided Setup choices
- **WHEN** Guided Setup loads successfully
- **THEN** it displays the template metadata and definitions returned by the catalog API
- **AND** it offers a separate Blank profile creation option

#### Scenario: Catalog unavailable
- **WHEN** the catalog request fails
- **THEN** Guided Setup reports the failure and does not present stale hard-coded template definitions as current choices

### Requirement: Migrate seeded profiles without changing active behavior or Queue evidence

The system MUST stop creating built-in templates as database User Profiles. On an existing installation, recognized seeded profiles MUST retain their profile IDs and definitions and gain provenance for the corresponding template version. Existing assignments and Queue snapshots MUST remain unchanged, and historical Queue snapshots MUST NOT be re-resolved from the current template catalog.

#### Scenario: Migrate an assigned seeded profile
- **WHEN** an existing seeded built-in profile has assignments during migration
- **THEN** the same persisted profile remains an editable User Video Profile with its original ID and definition plus template provenance
- **AND** every assignment still references that profile ID

#### Scenario: Migrate a profile referenced by Queue history
- **WHEN** a Queue record has a frozen profile snapshot for a seeded profile during migration
- **THEN** its snapshot bytes and captured profile version remain unchanged
- **AND** the system does not resolve that historical execution from a current template

#### Scenario: Seed a new installation
- **WHEN** a new installation initializes profile storage
- **THEN** built-in definitions are available from the catalog and are not inserted as User Video Profiles
