## Purpose

Provide one stable language identity contract for media track discovery and
selection across Asset Info, Settings, Profiles, Preflight, and Queue while
preserving stream identity, source metadata, and already frozen executions.

## ADDED Requirements

### Requirement: Track language identity is canonical and fail-safe

The system SHALL compare media track languages using trimmed, lower-case
language values and the existing supported ISO-639-2-compatible catalog.
Equivalent aliases SHALL resolve to the catalog's canonical code, including
`ko` and `ko-KR` to `kor`, and `fr`, `fre`, and `fr-FR` to `fra`. Existing
aliases such as `es`/`esp`, `en`, `ja`, `de`/`ger`, `it`, `pt`, and `zh`/`chi`
SHALL retain their current canonical behavior. An underscore variant of the
two regional tags SHALL be accepted where container metadata uses it. Blank or `und` metadata SHALL
remain undetermined; unrecognized values SHALL be preserved in normalized
lower-case form and MUST NOT be guessed from a partial or unrelated tag.
Normalizing source track metadata SHALL affect matching and display only;
original source metadata and sidecar paths SHALL remain available and SHALL
NOT be renamed as a side effect.

#### Scenario: Korean and French aliases identify the same track language
- **WHEN** equivalent `ko`, `ko-KR`, `kor`, `fr`, `fr-FR`, `fre`, or `fra` values differ only by casing or surrounding whitespace
- **THEN** language-based policy treats the Korean values as `kor` and the French values as `fra`

#### Scenario: Existing supported aliases remain equivalent
- **WHEN** a track or policy uses an existing alias such as `esp`, `en`, `ja`, `ger`, or `chi`
- **THEN** it matches the corresponding existing canonical language without changing the supported language set

#### Scenario: Unknown and undetermined metadata fail closed
- **WHEN** a stream language is blank, `und`, or an unrecognized regional value such as `fr-CA`
- **THEN** blank and `und` remain undetermined, while an unrecognized value is preserved in lower-case form and is not inferred to be a supported base language

#### Scenario: Source metadata and sidecar paths remain stable
- **WHEN** a source stream or existing subtitle sidecar carries a recognized alias or regional tag
- **THEN** policy matching may use its canonical identity while the source tag and existing path remain available without an automatic rename

### Requirement: Saved language policies resolve consistently

Settings preferred languages, Audio and Track Profile language lists/defaults,
Asset overrides, Asset Defaults, and language-scoped subtitle rules SHALL use
the same canonical identity when saved, reloaded, inherited, and resolved.
Legacy saved aliases SHALL remain readable and SHALL resolve equivalently
without requiring a data migration. Unknown profile language values SHALL
remain explicit and SHALL NOT silently become a supported language.

#### Scenario: Preferred languages survive save and reload
- **WHEN** a user saves preferred languages using aliases or mixed casing and reloads Settings
- **THEN** the ordered preferences retain their language meaning and deduplicate equivalent canonical values without reordering unrelated entries

#### Scenario: Profile languages resolve against stream metadata aliases
- **WHEN** a saved Audio or Track Profile requests a language using a legacy alias or recognized tag and a matching stream uses its equivalent code
- **THEN** the profile resolves that stream using the canonical language identity

#### Scenario: Explicit per-asset stream selections remain authoritative
- **WHEN** an asset has an explicit selected stream-index list, including an empty list
- **THEN** language normalization does not add, remove, or replace any selected stream index

### Requirement: Canonical language matching preserves distinct streams

Language normalization SHALL NOT merge streams or substitute language equality
for stream identity. Language-based policies SHALL resolve to the matching
stream indexes, and explicit per-asset stream choices SHALL remain authoritative
over inherited profile language rules.

#### Scenario: Duplicate-language streams remain independent
- **WHEN** two streams have equivalent canonical languages but different stream indexes
- **THEN** each stream retains its own action and selecting one index does not select or exclude the other

#### Scenario: Same-language aliases select matching profile streams
- **WHEN** a profile selects `kor` and source streams report `ko` and `ko-KR` at distinct indexes
- **THEN** both indexes are included by the language policy and remain separately represented in the resolved plan

### Requirement: Queue freezes normalized language decisions without rewriting history

New Queue and Batch Preflight resolutions SHALL freeze the effective language-
based actions and selected stream indexes in the existing track plan. Worker
execution SHALL use that frozen plan even if settings, profiles, or asset
overrides later change. Existing active QueueJobs and legacy snapshots SHALL
remain readable and immutable; normalization SHALL NOT retroactively rewrite
their execution plans or archives.

#### Scenario: Preflight and Queue preserve alias-resolved actions
- **WHEN** Batch Preflight resolves aliases in profile language preferences and the candidate is accepted
- **THEN** the accepted Queue snapshot freezes the resulting actions and exact stream indexes used for output planning

#### Scenario: Later profile edits do not change a frozen plan
- **WHEN** language preferences or profile rules change after the Queue plan is frozen
- **THEN** the existing execution and its retries retain their frozen track actions and stream indexes

#### Scenario: Legacy snapshots remain readable
- **WHEN** an active or archived Queue snapshot predates canonical track language normalization
- **THEN** it remains readable and its stored execution plan is not migrated or rewritten
