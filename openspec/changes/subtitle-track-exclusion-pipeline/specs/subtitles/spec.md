## ADDED Requirements

### Requirement: Explicit asset subtitle selection bounds the conversion pipeline

An explicit per-asset `keepSubtitleStreams` selection SHALL constrain subtitle
work from Queue planning through publication. Omitted or `nil` selection SHALL
inherit existing policy; an explicit empty list SHALL authorize no subtitle
streams; a non-empty list SHALL authorize only the listed stream indexes.
Automatic Video Profile externalization and Track Profile subtitle rules SHALL
not reintroduce an explicitly excluded stream. Selection SHALL be based on
stream index, not language, so duplicate-language tracks remain independent.

#### Scenario: Selected subset excludes every other stream before work
- **WHEN** source subtitle streams have indexes `[2,3,4]` and the saved asset selection is `[2,4]`
- **THEN** stream 3 has an excluded disposition and no extraction, OCR, conversion, sidecar, FFmpeg mapping, validation, or publication operation is scheduled for it

#### Scenario: Explicit empty selection authorizes no subtitle work
- **WHEN** the saved asset selection is `[]` and a Video Profile requests external SRT or ASS subtitles
- **THEN** no subtitle stream is mapped or submitted for extraction, OCR, conversion, sidecar generation, validation, or publication

#### Scenario: Omitted selection inherits existing policy
- **WHEN** `keepSubtitleStreams` is omitted or `nil`
- **THEN** existing Video Profile and Track Profile subtitle policy continues to determine eligible streams

#### Scenario: Video Profile externalizes only selected tracks without a Track Profile
- **WHEN** no Track Profile is assigned, the Video Profile requests external SRT or ASS, and the asset selects a subset of source subtitle indexes
- **THEN** automatic externalization creates sidecar work only for selected indexes and preserves the saved selection in the effective plan

#### Scenario: Track Profile rules cannot restore an excluded track
- **WHEN** an explicit asset selection excludes a stream that would otherwise match an automatic language, role, or stream rule
- **THEN** the excluded stream remains removed and contributes no sidecar output to the effective plan

#### Scenario: Duplicate-language tracks use their own indexes
- **WHEN** two subtitle streams share a language and the asset selects only one stream index
- **THEN** only the selected index is eligible for embedded output or subtitle artifact work

### Requirement: Queue freezes subtitle eligibility and artifact work

Queue Preflight and the frozen Queue execution SHALL preserve the exact effective
asset subtitle selection. The Worker, subtitle artifact inventory, retries, and
restarts SHALL consume that frozen selection without rereading mutable asset
selection state. Excluded streams SHALL not appear as pending or running
subtitle artifacts. A job with no authorized subtitle work SHALL not enter
`Preparing Subtitles` solely for excluded tracks; independently authorized font
attachment work MAY still require that stage. Existing accepted jobs and legacy
snapshots SHALL remain immutable and readable.

#### Scenario: Preflight and frozen plan preserve the reviewed selection
- **WHEN** Queue Preflight reviews source indexes `[2,3,4]` with asset selection `[2,4]` and the job is accepted
- **THEN** the accepted snapshot records only 2 and 4 as eligible subtitle streams and records 3 as excluded

#### Scenario: Selection changes after freeze do not change retry or restart
- **WHEN** the asset selection changes after a Queue job has frozen its execution plan
- **THEN** that job, its retry, and its restart continue using the frozen stream indexes, while a newly planned job may use the changed selection

#### Scenario: All subtitles are excluded
- **WHEN** a frozen job has an explicit empty subtitle selection and no independent font attachment work
- **THEN** it creates no subtitle artifacts and skips `Preparing Subtitles`

#### Scenario: Legacy snapshots remain authoritative
- **WHEN** an existing frozen job has no canonical resolved track plan
- **THEN** the job remains readable and retains its historical snapshot behavior without being retroactively rewritten

### Requirement: Excluded subtitle streams never reach subtitle operations or output

The system SHALL filter excluded streams before invoking extraction, OCR,
conversion, or sidecar generation. FFmpeg mappings, generated `SubtitleArtifacts`,
validation inputs, and Publisher outputs SHALL agree with the selected indexes
in the frozen job. A blocked or excluded stream SHALL not be resurrected by a
legacy transform or downstream fallback. Subtitle changes SHALL not alter
audio, video, chapter, or attachment selections.

#### Scenario: Excluded text and bitmap streams are not processed
- **WHEN** selected and excluded subtitle streams include ASS, SRT, and PGS/VobSub sources
- **THEN** only selected streams reach text extraction/conversion or bitmap OCR, and excluded streams produce no sidecars

#### Scenario: FFmpeg and Publisher follow the frozen selection
- **WHEN** the Worker executes and publishes a job with an explicit subtitle subset
- **THEN** FFmpeg maps only eligible embedded subtitles and Publisher exposes only authorized generated sidecars

#### Scenario: Excluded streams have no stale artifact progress
- **WHEN** an excluded stream is present in source metadata or an old transform list
- **THEN** no pending, running, completed, or failed `SubtitleArtifact` is reported for that index

#### Scenario: Other stream domains remain unchanged
- **WHEN** an asset has independent audio, video, chapter, and attachment selections while a subtitle is excluded
- **THEN** those non-subtitle selections and outputs retain their existing behavior
