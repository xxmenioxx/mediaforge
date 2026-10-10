## Purpose

Defines how reusable profile definitions, frozen execution configuration, and per-asset evidence are identified across Test Encode, Profile Qualification, and Queue Preflight. It keeps valid evidence discoverable while preventing evidence from one asset or configuration from qualifying another.

## ADDED Requirements

### Requirement: Profile and execution identities are distinct and versioned

The system MUST persist a versioned reusable profile-definition identity independently from the effective execution identity. The definition identity MUST be stable when the same profile is assigned through PATH, Source Group, Logical Group, or ASSET, and MUST change when its saved definition or version changes. It MUST exclude assignment origin, source path, asset identity, and runtime-only analysis context. The execution identity MUST represent the effective profile and output-affecting options frozen for the candidate after applicable Asset Overrides, LAB draft choices, and automatic resolution, including QSV ICQ/GQ, HEVC Level, GOP/B-frames, cadence, color, and restoration decisions. Candidate and evidence identities MUST remain bound to their exact source and Test Encode generation.

#### Scenario: Same profile is assigned at different scopes
- **WHEN** an unchanged reusable profile is resolved through PATH, Source Group, Logical Group, and ASSET for the same effective settings
- **THEN** each resolution has the same profile-definition identity, while any asset-specific execution or evidence identity remains separately bound to its candidate

#### Scenario: Automatic planning changes the frozen profile representation
- **WHEN** Test Encode planning resolves an automatic video decision after capturing the reusable profile identity
- **THEN** Qualification validates the post-plan execution identity against the frozen post-plan snapshot and does not compare it directly to the pre-plan profile identity

#### Scenario: A saved profile definition changes
- **WHEN** a profile's saved output-affecting definition or version changes while its database ID remains the same
- **THEN** its profile-definition identity changes and prior qualification evidence is not current for that definition

### Requirement: Snapshots bind each identity to its capture stage

Test Encode MUST retain the reusable definition identity with the requested/resolved profile snapshot and capture the effective execution identity atomically with the final profile used to build FFmpeg arguments. `ConfigurationHash` and #69 candidate configuration identity MUST be traceable to that frozen execution, without runtime identifiers, executor labels, or output paths changing the identity. Queue Preflight MUST compare the identity metadata preserved in its frozen `ProfileSnapshot` with the matching Test Encode and Qualification; it MUST NOT substitute mutable current profile configuration for either frozen snapshot.

#### Scenario: T105-style effective-asset Test Encode resolves QSV and video decisions
- **WHEN** an `effective_asset` Test Encode resolves QSV quality, HEVC level, GOP/B-frames, cadence, color, or restoration during plan construction
- **THEN** the stored effective profile, execution identity, `ConfigurationHash`, and #69 candidate identity all describe the same frozen candidate, and Qualification does not report a spurious fingerprint mismatch

#### Scenario: Frozen settings are edited after capture
- **WHEN** profile, assignment, Asset Override, LAB draft, or runtime settings change after a candidate snapshot is frozen
- **THEN** re-evaluation uses the persisted snapshot and identities, and the existing candidate's execution identity does not change

#### Scenario: Preflight accepts exact matching evidence
- **WHEN** a Preflight asset has the same source asset and revision, profile-definition identity, effective profile identity, Test Encode candidate/generation, evidence, and active Qualification policy
- **THEN** Preflight shows that exact Qualification and preserves it in the accepted plan without creating a new Test Encode

#### Scenario: Preflight sees a changed effective profile
- **WHEN** the asset's frozen effective profile differs from the one bound to a Test Encode, even though the reusable profile definition is the same
- **THEN** Preflight does not present that candidate's Qualification as current and explains the incompatible identity

### Requirement: Evidence cannot be reused across assets or generations

Qualification and Preflight MUST require matching source asset identity, source path and physical revision, Test Encode ID and output generation, effective execution identity, SamplingPlan identity, #69 evidence and reference identities, and Qualification policy version. Matching only a profile-definition identity or profile database ID MUST NOT yield a current result. A mismatch MUST NOT be converted into `PASS` by skipping another identity check.

#### Scenario: Same definition is used for a different asset
- **WHEN** another asset uses the same reusable profile definition and has a different source identity
- **THEN** the original asset's Test Encode, candidate identity, and Qualification are not treated as evidence for the other asset

#### Scenario: Candidate configuration or generation changes
- **WHEN** a #69 candidate references another Test Encode generation, effective execution identity, SamplingPlan, or reference evidence
- **THEN** Qualification reports stale or review-required evidence and does not return `PASS`

### Requirement: Legacy and incompatible identities fail closed with actionable causes

Historical snapshots and Qualification records MUST remain immutable. When a record lacks the versioned definition or execution identity needed for exact validation, the system MUST report `STALE`, `REVIEW_REQUIRED`, or not-qualified with a specific legacy or mismatch cause. It MUST NOT synthesize a passing result, rewrite historical records, or trigger an encode while viewing, attaching, or re-evaluating existing evidence. Qualification remains a non-hard-blocking Preflight confidence signal, and final Validation remains authoritative.

#### Scenario: Legacy evidence lacks the new identity contract
- **WHEN** a historical Test Encode or Qualification has no verifiable frozen identity for one of the required stages
- **THEN** it remains historical and is explicitly marked stale or review-required with the missing identity and stage identified

#### Scenario: A failed or stale assessment is revisited
- **WHEN** an operator views or re-evaluates an earlier failed or stale assessment
- **THEN** the system does not promote it to `PASS`, rewrite its history, or create a replacement Test Encode implicitly

#### Scenario: Qualification passes but final output fails Validation
- **WHEN** an existing Qualification is passing but final Validation finds a failure in the produced Queue result
- **THEN** the existing authoritative Validation outcome and Queue/Publisher behavior are unchanged
