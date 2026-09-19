## ADDED Requirements

### Requirement: Persisted ASS-to-SRT fallback setting

The system SHALL persist a boolean subtitle setting equivalent to
`generateSRTFallbackForASS`. A missing value SHALL resolve to `false`, and the
default SHALL preserve existing subtitle output behavior until a user enables
the setting.

#### Scenario: Legacy settings default to fallback off

- **WHEN** an installation has no stored ASS-to-SRT fallback value
- **THEN** subtitle planning SHALL behave as if the setting were `false`

#### Scenario: Setting persists across settings reload

- **WHEN** a user enables or disables the fallback setting and settings are
  loaded again
- **THEN** the same effective boolean value SHALL be used by subsequent
  canonical subtitle planning

### Requirement: Canonical effective subtitle format resolution

The canonical subtitle planner SHALL combine explicit requested formats with
the resolved fallback setting before producing the `ResolvedTrackPlan`. When
ASS is effective and the setting is enabled, SRT SHALL be added. Explicit SRT
requests SHALL remain SRT-only when no ASS output is requested, and normalized
effective formats SHALL be deduplicated.

#### Scenario: ASS receives an opt-in SRT fallback

- **WHEN** the explicit subtitle request is ASS and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain exactly one ASS output and one SRT
  output

#### Scenario: SRT does not create another output

- **WHEN** the explicit subtitle request is SRT and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain exactly one SRT output and no ASS
  output

#### Scenario: Explicit ASS and SRT are normalized

- **WHEN** the explicit request contains ASS and SRT and the fallback setting
  is enabled
- **THEN** the resolved plan SHALL contain one ASS output and one SRT output
  without duplicate transforms

#### Scenario: ASS remains unchanged when fallback is disabled

- **WHEN** the explicit request is ASS and the fallback setting is disabled or
  absent
- **THEN** the resolved plan SHALL contain ASS without an added SRT output

### Requirement: Shared canonical planning across entry points

Asset Info, Track Profiles, Profile Lab, Test Encode, and Queue SHALL use the
same canonical effective-format behavior. Where an entry point exposes ASS
selection, it SHALL make the resulting SRT fallback visible before execution
when the persisted setting is enabled. No client capability or playback policy
SHALL alter the resolved formats.

#### Scenario: Asset Info shows the effective fallback

- **WHEN** Asset Info selects ASS for a subtitle stream and fallback is enabled
- **THEN** the displayed/resolved subtitle outputs SHALL show that SRT will also
  be produced before the operation is started

#### Scenario: Profile and Lab selections agree

- **WHEN** a Track Profile or Profile Lab/Test Encode request selects ASS under
  the same settings and source conditions as Asset Info
- **THEN** its resolved effective outputs SHALL match the canonical plan,
  including the added SRT output

### Requirement: Native ASS and SSA fallback preservation

The system SHALL preserve native ASS/SSA extraction as the original styled
representation with its original extension, styles, tags, and positioning. An
effective SRT fallback SHALL be an additional canonical text/timing output and
SHALL NOT rewrite or replace the native representation.

#### Scenario: Native ASS publishes original plus SRT

- **WHEN** a native ASS subtitle is selected with fallback enabled
- **THEN** the operation SHALL preserve the native ASS artifact and produce a
  separate SRT fallback through the existing canonical conversion path

#### Scenario: Native SSA preserves its source representation

- **WHEN** a native SSA subtitle is selected with fallback enabled
- **THEN** the operation SHALL preserve the native SSA artifact and produce the
  additional SRT output without converting the original into a different
  styled format

### Requirement: Bitmap ASS and SRT share canonical OCR evidence

For bitmap subtitles, effective ASS+SRT output SHALL be one logical subtitle
transformation. OCR, Japanese fallback, accurate candidate selection, and
existing OCR quality corrections SHALL produce one canonical cue/evidence set
before SRT and ASS serialization. ASS SHALL retain existing geometry and
positioning behavior, while SRT SHALL serialize the same corrected text and
timing without geometry.

#### Scenario: Bitmap fallback performs one OCR lifecycle

- **WHEN** a bitmap subtitle is requested as ASS with fallback enabled
- **THEN** OCR and any existing correction lifecycle SHALL execute once and
  both serializers SHALL consume the resulting canonical cues

#### Scenario: Corrections are shared by both outputs

- **WHEN** the canonical OCR quality lifecycle corrects a bitmap cue before
  publication of effective ASS+SRT outputs
- **THEN** the SRT and ASS artifacts SHALL contain the same corrected cue text
  and the ASS artifact SHALL retain the cue's canonical geometry/positioning

### Requirement: Queue freezes effective fallback outputs

Queue preparation SHALL resolve the fallback setting and freeze the resulting
effective subtitle outputs in the immutable `ResolvedTrackPlan`/Queue
snapshot. Workers SHALL consume that frozen plan and SHALL NOT read or
reinterpret Settings for subtitle output selection.

#### Scenario: Settings changes after enqueue do not change a frozen job

- **WHEN** a job is enqueued with fallback enabled and the setting is disabled
  before the Worker executes the job
- **THEN** the Worker SHALL still execute the frozen ASS+SRT plan

#### Scenario: A later retry resolves current settings

- **WHEN** a new retry is created after the previous operation terminates and
  the persisted fallback setting has changed
- **THEN** the new operation SHALL receive a newly resolved plan under the
  current setting rather than mutating the old snapshot

### Requirement: Atomic fallback publication and metadata preservation

The fallback SHALL use the existing validation, cancellation, naming, metadata,
and atomic all-output publication boundaries. Semantic roles, source and
effective forced/SDH dispositions, sidecar qualifiers, logical `both` identity,
and existing cancellation behavior SHALL be preserved independently of whether
SRT was added by fallback.

#### Scenario: Fallback outputs publish atomically

- **WHEN** one output in an effective ASS+SRT operation fails validation or the
  operation is cancelled before the publication barrier
- **THEN** no newly generated member of that operation SHALL be published and
  previously existing sidecars SHALL remain intact

#### Scenario: Role and disposition metadata survive fallback

- **WHEN** a subtitle with semantic role, forced, or SDH metadata receives an
  SRT fallback
- **THEN** the canonical filenames, artifact metadata, and supported
  publication dispositions SHALL preserve that metadata for the output set
