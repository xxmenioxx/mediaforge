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

The canonical subtitle planner SHALL combine explicit requested formats,
requested mode, source codec, and the resolved fallback setting before
producing the `ResolvedTrackPlan`. Fallback eligibility SHALL be computed as:

```text
fallbackEligible =
    generateSRTFallbackForASS
    AND
    (
        (mode == converted AND effective converted formats include ASS)
        OR
        (mode == original AND source codec is native ASS or SSA)
    )
```

Only an eligible request SHALL gain an implicit SRT. Explicit SRT requests
SHALL remain SRT-only when no converted ASS output is effective, and normalized
effective formats SHALL be deduplicated.

#### Scenario: Converted ASS is fallback eligible

- **WHEN** a subtitle request has `mode=converted`, effective converted format
  ASS, and the fallback setting is enabled
- **THEN** the resolved plan SHALL contain exactly one converted ASS output and
  one converted SRT output using the existing logical `both` semantics

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

#### Scenario: Original native ASS is fallback eligible

- **WHEN** a native ASS source has `mode=original` and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain the lossless original ASS output and
  one converted SRT output, without treating the pair as logical `both`

#### Scenario: Original native SSA is fallback eligible

- **WHEN** a native SSA source has `mode=original` and the fallback setting is
  enabled
- **THEN** the resolved plan SHALL contain the lossless original SSA output and
  one converted SRT output, without treating the pair as logical `both`

#### Scenario: Original non-ASS/SSA sources are not fallback eligible

- **WHEN** an original-mode source is SRT, PGS, or VobSub and the fallback
  setting is enabled
- **THEN** the planner SHALL NOT add an implicit SRT output

#### Scenario: Disabled fallback never adds implicit SRT

- **WHEN** the fallback setting is disabled or absent for either converted ASS
  or original native ASS/SSA
- **THEN** the planner SHALL NOT add an implicit SRT output

### Requirement: Shared canonical planning across entry points

Asset Info, Track Profiles, Profile Lab, Test Encode, and Queue SHALL use the
same canonical resolver and effective-format resolution contract. Interactive
entry points MAY resolve equivalent current plans; Queue SHALL own and freeze
its execution snapshot. Where an entry point exposes ASS selection, it SHALL
make the resulting SRT fallback visible before execution when the persisted
setting is enabled. No client capability or playback policy SHALL alter the
resolved formats.

#### Scenario: Asset Info shows the effective fallback

- **WHEN** Asset Info selects ASS for a subtitle stream and fallback is enabled
- **THEN** the displayed/resolved subtitle outputs SHALL show that SRT will also
  be produced before the operation is started

#### Scenario: Profile and Lab selections agree

- **WHEN** a Track Profile or Profile Lab/Test Encode request selects ASS under
  the same settings and source conditions as Asset Info
- **THEN** its resolved effective outputs SHALL match the canonical plan,
  including the added SRT output

### Requirement: Source codec and requested subtitle mode remain distinct

The system SHALL treat source codec and requested extraction mode as separate
dimensions. Converted text and bitmap outputs SHALL use the canonical
normalization/serializer or OCR/cue lifecycle respectively. Native ASS/SSA
SHALL remain lossless only when `mode=original` is requested; an effective SRT
fallback for that mode SHALL be a separate converted output and SHALL NOT
rewrite or replace the native representation.

#### Scenario: Converted text ASS receives converted SRT fallback

- **WHEN** a text source is requested with `mode=converted`, `format=ass`, and
  fallback enabled
- **THEN** the operation SHALL produce converted ASS and converted SRT through
  the canonical normalization and serializer path

#### Scenario: Converted bitmap ASS receives converted SRT fallback

- **WHEN** a bitmap source is requested with `mode=converted`, `format=ass`,
  and fallback enabled
- **THEN** the operation SHALL produce converted ASS and converted SRT from
  one canonical OCR/cue lifecycle

#### Scenario: Native ASS in original mode publishes original plus converted SRT

- **WHEN** a native ASS subtitle is selected with `mode=original` and fallback
  enabled
- **THEN** the operation SHALL preserve the native ASS artifact and produce a
  separate converted SRT fallback through the existing canonical conversion
  path without treating the pair as logical `both`

#### Scenario: Native SSA in original mode preserves its source representation

- **WHEN** a native SSA subtitle is selected with `mode=original` and fallback
  enabled
- **THEN** the operation SHALL preserve the native SSA artifact and produce the
  additional converted SRT output without converting the original into a
  different styled format

#### Scenario: Converted logical both remains one transformation

- **WHEN** a converted subtitle request explicitly selects logical `both`
- **THEN** its SRT and ASS serializers SHALL share one normalized cue/OCR
  result, independently of the original-mode native ASS/SSA fallback case

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

Queue SHALL retain the existing lifecycle `QUEUED → CLAIM/PREPARING → resolve
effective configuration → build/freeze ResolvedTrackPlan → PROCESSING`.
Effective fallback outputs SHALL be frozen in the immutable
`ResolvedTrackPlan`/Queue snapshot during Claim/Preparing. Workers SHALL
consume that frozen plan and SHALL NOT read or reinterpret Settings for
subtitle output selection.

Before a successful freeze, preparation or retry MAY resolve the current
settings. After a successful freeze, the execution and any retry of that
frozen job SHALL use the same snapshot. A new/future job SHALL resolve current
settings.

#### Scenario: Settings changes before freeze are observed during preparation

- **WHEN** a job remains `QUEUED`, the fallback setting changes, and Claim/
  Preparing has not successfully frozen the plan
- **THEN** preparation SHALL resolve and freeze the new setting's effective
  output

#### Scenario: Settings changes after freeze do not affect execution or retry

- **WHEN** a job has successfully frozen ASS+SRT, the setting changes to off,
  and the job executes or is retried
- **THEN** execution and retry SHALL continue to use the frozen ASS+SRT
  snapshot

#### Scenario: Incomplete freeze may resolve again

- **WHEN** preparation or retry has not successfully completed the freeze
  boundary and the persisted fallback setting changes
- **THEN** the next preparation attempt MAY resolve current settings before
  freezing a plan

#### Scenario: A future job resolves current settings

- **WHEN** a new job is created after the setting changes
- **THEN** its Claim/Preparing phase SHALL resolve the current setting

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
