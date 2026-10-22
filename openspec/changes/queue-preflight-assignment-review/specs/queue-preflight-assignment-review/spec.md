## Purpose

Batch Preflight assignment review makes the candidate's exact effective Video, Audio, Track, and destination choices understandable and preserves the operator's review boundary when assignments change.

## ADDED Requirements

### Requirement: Candidate assignments SHALL be named and provenance-aware

Batch Preflight SHALL present the effective Video Profile, Audio Profile, Track Profile, and destination Library using recognizable names when available, with stable IDs or keys as secondary diagnostics. It SHALL distinguish the candidate-effective or frozen value, its assignment origin, and any explicit candidate-only override. Custom configuration, disabled/none selection, and audio-only fallback SHALL be labeled honestly. Missing inventory entries SHALL remain identifiable and SHALL NOT be replaced by a guessed default.

#### Scenario: Review an inherited Path assignment
- **WHEN** an asset's effective video profile and destination are assigned at Path scope
- **THEN** Batch Preflight shows their names and paths, identifies the Path origin, and shows the exact frozen profile version and destination snapshot

#### Scenario: Review an explicit candidate override
- **WHEN** an operator changes a profile or destination in Edit
- **THEN** the resolved candidate labels that value as a candidate-only override and retains the prior assignment origin as context

#### Scenario: Review an audio-only fallback
- **WHEN** an audio-only candidate has no selected video profile and Queue preparation uses its fallback profile snapshot
- **THEN** Batch Preflight labels the profile as an audio-only fallback and does not present it as a selected full-encode profile

#### Scenario: Referenced inventory entry is missing
- **WHEN** the current profile, audio/track profile, or Library inventory no longer contains a candidate reference
- **THEN** Batch Preflight identifies the missing reference and its frozen candidate value, without selecting a replacement

### Requirement: Editing SHALL preserve the candidate's exact current assignment

Entering Edit, closing and reopening, and resolving unchanged assignment inputs SHALL preserve the candidate's effective profile, assignment provenance, destination, and frozen snapshots. A changed profile, audio/track selection, or Library SHALL become an explicit candidate-only override. Candidate revision conflicts SHALL remain visible and SHALL NOT apply stale edits.

#### Scenario: Reopen and resolve unchanged inputs
- **WHEN** an operator edits a candidate and leaves all assignment values unchanged
- **THEN** its canonical assignment and exact reviewed frozen plan remain unchanged

#### Scenario: Select another named profile
- **WHEN** an operator selects a different named Video Profile and resolves the candidate
- **THEN** only that candidate records the explicit override and the resulting frozen plan uses the selected profile's exact ID and version

#### Scenario: Submit against an old candidate revision
- **WHEN** Edit or Resolve uses a candidate revision that has already advanced
- **THEN** the operation is rejected with actionable stale-candidate feedback and does not replace the latest candidate state

### Requirement: Changed canonical assignments SHALL require explicit refresh and review

Batch Preflight SHALL detect when an unmodified canonical assignment no longer matches current Asset, Path, Logical Group, Source Group, or Global/default resolution. It SHALL explain the old and current choices. Accept SHALL be blocked while an assignment is stale or unavailable. An explicit re-resolve SHALL refresh unmodified canonical assignments while retaining deliberate candidate-only overrides, and any resulting assignment change SHALL be shown before acceptance and SHALL require operator review.

#### Scenario: Path assignment changes after candidate resolution
- **WHEN** a Path assignment changes while a candidate still contains the previous resolved value
- **THEN** the candidate reports the before and current assignments and cannot be accepted until the operator explicitly re-resolves or makes a deliberate candidate override

#### Scenario: Explicit refresh changes the plan
- **WHEN** the operator explicitly re-resolves a stale candidate and its selected profile, source, or destination changes
- **THEN** Batch Preflight shows a concise before-to-after summary and requires review before Accept & Queue is enabled

#### Scenario: Explicit candidate override survives refresh
- **WHEN** a domain already has a deliberate candidate-only override and another canonical domain is refreshed
- **THEN** the candidate override remains unchanged while the other domain uses the current canonical assignment

### Requirement: Accepted Queue execution SHALL match the reviewed candidate

Accept SHALL create QueueJobs and ExecutionPlans from the exact candidate plan displayed after required review. Profile identity and version, processing mode, selected encoder, assignment provenance, Audio/Track snapshots, and destination SHALL agree with the accepted plan. Missing or stale references SHALL fail visibly without silent fallback.

#### Scenario: Accept reviewed named assignments
- **WHEN** an operator accepts a ready candidate after reviewing its assignments
- **THEN** each accepted QueueJob and ExecutionPlan preserves the exact displayed profile/version, selected encoder, supplemental snapshots, and Library destination

#### Scenario: Canonical Path assignment outranks a broader default
- **WHEN** a Path assigns Profile A while a broader Source Group or Global/default assigns Profile B and no Asset override exists
- **THEN** selected-assets Preflight resolves and freezes Profile A; an explicit Asset assignment may supersede it
