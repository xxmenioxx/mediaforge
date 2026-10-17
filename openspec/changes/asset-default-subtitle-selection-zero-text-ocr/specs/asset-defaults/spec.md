## MODIFIED Requirements

### Requirement: Tracks General subtitle defaults

Tracks General SHALL support keep, remove, extract, and keep-and-extract subtitle
actions; SRT, ASS, or both output formats; and the existing canonical bitmap OCR
processing modes. Subtitle resolution SHALL reuse the existing `subtitles`
capability for source classification, extraction, OCR, cue normalization,
format rendering, validation, naming, and artifact reporting. The effective
subtitle stream set SHALL honor the resolved asset/profile, path, logical-group,
source-group, and Asset Defaults selection, including configured preferred
language rules when no more-specific decision overrides them. An explicit empty
selection or remove/skip action SHALL remain distinct from inheritance and
SHALL schedule no subtitle sidecar or OCR work. Video Profile externalization
SHALL NOT widen the canonical eligible stream set or add an unrequested sidecar
representation.

#### Scenario: Default subtitle action

- **WHEN** an unassigned asset reaches Tracks resolution with a configured global subtitle action
- **THEN** the canonical subtitle plan uses that action for eligible subtitle streams

#### Scenario: Asset Defaults preferred languages select eligible tracks

- **WHEN** an unassigned asset has subtitle streams in several languages and Tracks General configures preferred-language actions
- **THEN** the canonical plan preserves the stream identity of each track and only streams whose effective action authorizes work produce sidecars or OCR

#### Scenario: Explicit None or SKIP schedules no subtitle work

- **WHEN** the effective Tracks selection is explicitly empty or its subtitle action removes/skips every stream
- **THEN** no subtitle extraction, OCR, conversion, pending sidecar artifact, or subtitle preparation stage is scheduled

#### Scenario: Converted SRT or ASS

- **WHEN** Tracks General requests converted SRT or ASS
- **THEN** the existing canonical subtitle pipeline produces the requested artifact format and preserves its existing text-versus-bitmap and OCR boundaries

#### Scenario: Both formats share one logical OCR result

- **WHEN** Tracks General requests both SRT and ASS for a bitmap subtitle
- **THEN** one canonical extraction/OCR and normalization result feeds both rendered artifacts, with no duplicate OCR execution

#### Scenario: Existing OCR mode is selected

- **WHEN** Tracks General selects a supported existing bitmap OCR mode
- **THEN** the canonical subtitle pipeline receives that mode without introducing a new OCR semantic or a parallel OCR implementation

#### Scenario: Video Profile externalization follows canonical selection

- **WHEN** a Video Profile requests external subtitles while an effective Track plan exists
- **THEN** externalization is limited to the Track plan's eligible stream identities and effective output representations and does not re-enable removed streams or add an unrequested SRT/ASS representation

### Requirement: Shared effective resolution and snapshot safety

Preview, Test Encode, and Queue SHALL resolve identical effective Asset Defaults
for equivalent inputs. Queue SHALL freeze the resolved defaults and canonical
subtitle artifact plan during PREPARING, after atomic claim and before processing.
A normal individual Queue intent MUST resolve the current defaults at claim;
edits while it remains QUEUED therefore affect that resolution. Once PREPARING
freezes the snapshot, later edits MUST NOT rewrite that execution or its retry.
Accepted Batch Preflight jobs continue using the exact configuration reviewed
and accepted by the operator.

#### Scenario: Preview/Test Encode/Queue parity

- **WHEN** equivalent unassigned asset inputs are evaluated through Preview, Test Encode, and Queue
- **THEN** each reports the same effective Video, Audio, Track, and subtitle artifact decisions

#### Scenario: Defaults change while an individual Queue job is still queued

- **WHEN** Asset Defaults change before a normal individual Queue job is claimed
- **THEN** PREPARING resolves and freezes the current defaults

#### Scenario: Defaults change after PREPARING freeze

- **WHEN** Asset Defaults are edited after PREPARING freezes a Queue execution
- **THEN** the execution and its retry continue using the frozen snapshot

#### Scenario: Accepted Batch Preflight keeps its reviewed defaults

- **WHEN** Asset Defaults change after a Batch Preflight plan has been accepted
- **THEN** the accepted job continues using the exact reviewed and accepted snapshot
