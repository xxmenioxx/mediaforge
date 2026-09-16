## Purpose

Defines a single, non-destructive fallback policy for unassigned assets across
Video, Audio, and Tracks while preserving the existing canonical subtitle
pipeline and all more-specific asset and group decisions.

## ADDED Requirements

### Requirement: Asset Defaults configuration

The system SHALL expose Settings → Asset Defaults with independent Video
General, Audio General, and Tracks General sections. Each section SHALL select
either Custom or an existing domain profile as its source. Selecting a profile
SHALL persist its reference and SHALL NOT duplicate the profile's editable
fields into the default setting.

#### Scenario: Custom defaults are visible

- **WHEN** a user opens Asset Defaults
- **THEN** Video, Audio, and Tracks each show a Custom or Profile source state and Custom is the initial source for a new/default installation

#### Scenario: Existing profile is selected

- **WHEN** a user selects an existing Video, Audio, or Track Profile as that domain's default source
- **THEN** the setting stores the profile reference and resolves the profile through the existing profile system without copying its fields

### Requirement: Fallback precedence

Asset Defaults SHALL be fallbacks only. Effective resolution SHALL preserve the
precedence `explicit asset override or asset profile > path > logical-group >
source-group > Asset Defaults > built-in fallback`, independently for Video,
Audio, and Tracks. An explicit disabled state SHALL remain distinct from
inheritance.

#### Scenario: Specific assignment overrides a default

- **WHEN** an asset has an explicit asset, path, logical-group, or source-group decision for one domain and Asset Defaults also defines that domain
- **THEN** the more-specific decision is used and the Asset Default does not replace it

#### Scenario: Unassigned asset uses defaults

- **WHEN** an asset has no more-specific decision for a domain
- **THEN** that domain resolves from Asset Defaults before using the built-in MVForge fallback

#### Scenario: Explicit disable remains authoritative

- **WHEN** a more-specific scope explicitly disables a domain or option
- **THEN** Asset Defaults do not re-enable it through inheritance

### Requirement: Video General defaults

Video General Custom defaults SHALL support a Copy choice, encoders reported as
supported by the current worker/runtime, and optional removal of non-primary
non-movie video streams. The system MUST NOT present an invented encoder as
runtime-supported or silently remove extra video streams when the removal
option is disabled.

#### Scenario: Runtime-supported encoder choices

- **WHEN** Video General displays encoder choices
- **THEN** it lists Copy and only encoders supported by the applicable runtime capability probe

#### Scenario: Remove non-primary video tracks

- **WHEN** the resolved Video General policy enables removal of non-movie video tracks
- **THEN** planning keeps the primary movie/program video stream and removes eligible extra video streams

#### Scenario: Preserve extra video tracks by default

- **WHEN** the removal option is disabled or has not been explicitly changed
- **THEN** existing non-primary video tracks remain eligible for preservation according to the current policy

### Requirement: Audio General defaults

Audio General Custom defaults SHALL support optional compatible-track creation,
its quality and channel-count settings, source selection from the resolved or
default audio stream, and a preferred default audio language. More-specific
Audio Profile and asset/path decisions SHALL remain authoritative.

#### Scenario: Compatibility track is disabled

- **WHEN** Audio General disables compatible-track creation
- **THEN** planning does not add a compatibility audio track through the global default

#### Scenario: Compatibility track uses resolved source

- **WHEN** Audio General enables compatible-track creation with configured quality and channel count
- **THEN** planning derives the added track from the resolved/default audio stream and applies those configured values

#### Scenario: Preferred default audio language

- **WHEN** Audio General specifies a preferred audio language and a matching resolved stream exists
- **THEN** that stream is selected as the default audio stream without overriding a more-specific explicit default decision

### Requirement: Tracks General subtitle defaults

Tracks General SHALL support keep, remove, extract, and keep-and-extract subtitle
actions; SRT, ASS, or both output formats; and the existing canonical bitmap OCR
processing modes. Subtitle resolution SHALL reuse the existing `subtitles`
capability for source classification, extraction, OCR, cue normalization,
format rendering, validation, naming, and artifact reporting.

#### Scenario: Default subtitle action

- **WHEN** an unassigned asset reaches Tracks resolution with a configured global subtitle action
- **THEN** the canonical subtitle plan uses that action for eligible subtitle streams

#### Scenario: Converted SRT or ASS

- **WHEN** Tracks General requests converted SRT or ASS
- **THEN** the existing canonical subtitle pipeline produces the requested artifact format and preserves its existing text-versus-bitmap and OCR boundaries

#### Scenario: Both formats share one logical OCR result

- **WHEN** Tracks General requests both SRT and ASS for a bitmap subtitle
- **THEN** one canonical extraction/OCR and normalization result feeds both rendered artifacts, with no duplicate OCR execution

#### Scenario: Existing OCR mode is selected

- **WHEN** Tracks General selects a supported existing bitmap OCR mode
- **THEN** the canonical subtitle pipeline receives that mode without introducing a new OCR semantic or a parallel OCR implementation

### Requirement: Attachment and font defaults

Tracks General SHALL expose attachment keep/remove policy separately from font
attachment keep/export/remove policy. Existing ASS/SSA font dependency warnings,
provenance, and validation SHALL remain authoritative.

#### Scenario: Attachments and fonts are independently configured

- **WHEN** a user configures attachment handling and font handling in Tracks General
- **THEN** the effective plan records both policies distinctly and applies each to its corresponding artifact/stream decisions

#### Scenario: ASS dependency warning remains authoritative

- **WHEN** a resolved subtitle policy retains an ASS/SSA subtitle while font attachments are removed or not exported
- **THEN** the existing dependency warning and validation behavior remains visible and is not suppressed by Asset Defaults

### Requirement: Shared effective resolution and snapshot safety

Preview, Test Encode, and Queue SHALL resolve identical effective Asset Defaults
for equivalent inputs. Queue SHALL freeze the resolved defaults and canonical
subtitle artifact plan in its snapshot before processing. Editing Asset Defaults
after queue capture SHALL affect only future resolution and SHALL NOT rewrite an
existing queued snapshot or its execution.

#### Scenario: Preview/Test Encode/Queue parity

- **WHEN** equivalent unassigned asset inputs are evaluated through Preview, Test Encode, and Queue
- **THEN** each reports the same effective Video, Audio, Track, and subtitle artifact decisions

#### Scenario: Defaults change after Queue capture

- **WHEN** Asset Defaults are edited after a Queue job has captured its effective snapshot
- **THEN** the queued job and any retry continue using the captured snapshot while a future job can use the new defaults

### Requirement: Compatibility-preserving initialization

New or migrated Asset Defaults SHALL preserve current non-destructive MVForge
behavior until the user explicitly changes a setting. Existing asset/path/group
assignments and existing Queue snapshots SHALL remain valid without migration to
destructive subtitle or attachment removal behavior.

#### Scenario: Existing installation retains behavior

- **WHEN** an existing installation first receives Asset Defaults without user edits
- **THEN** its effective behavior remains equivalent to the current built-in fallback and no existing media or snapshot is rewritten

#### Scenario: Invalid or unavailable profile reference

- **WHEN** a stored default profile reference is missing, disabled, or unavailable
- **THEN** resolution reports the issue explicitly and uses the safe built-in fallback without silently changing a more-specific assignment
