## Purpose

Define one observable cadence-resolution contract so effective frame rate,
progressive output, validation evidence, and entry-point behavior remain
consistent with the motion processing that will actually be rendered.

## ADDED Requirements

### Requirement: Effective cadence output is derived from the resolved motion plan

The system SHALL derive `effectiveOutputFrameRate` from the rendered resolved
motion plan and its reliable analysis evidence, rather than from the requested
cadence mode alone. A cadence operation label SHALL NOT by itself establish the
effective frame rate or progressive output state.

#### Scenario: Rendered motion plan determines effective FPS

- **WHEN** a resolved cadence plan renders a frame-rate filter or cadence chain
  whose output rate differs from the source or requested mode
- **THEN** the effective output FPS SHALL match the rendered plan and SHALL be
  exposed consistently to Preview, Test Encode, Queue, and validation

#### Scenario: Requested mode cannot manufacture 24000/1001

- **WHEN** inverse telecine is requested but the resolved motion plan does not
  contain a valid cadence conversion that produces 24000/1001
- **THEN** the system SHALL NOT report `effectiveOutputFrameRate` as 24000/1001
  solely because the request was labeled inverse telecine

### Requirement: Explicit inverse telecine requires resolved field order and canonical rendering

An explicit inverse-telecine request SHALL resolve a usable field order before
the output can be reported as progressive 24000/1001. The rendered processing
plan SHALL contain the canonical fieldmatch-plus-decimate operation for that
field order. The only valid automatic detection input is the validated IVTC
field-order evidence accepted by the existing automatic-IVTC path; a generic
interlace-analysis detected field order is insufficient. If no usable order is
available, the result SHALL be unresolved/review-only rather than silently
selecting TFF or BFF.

#### Scenario: Explicit field order renders canonical IVTC

- **WHEN** inverse telecine is requested with a valid explicit `tff` or `bff`
  field order
- **THEN** the resolved plan SHALL render fieldmatch with that order followed
  by decimate, and only then MAY report progressive 24000/1001 output

#### Scenario: Explicit inverse telecine without usable order stays conservative

- **WHEN** inverse telecine is requested and neither an explicit nor validated
  detected field order is available
- **THEN** the system SHALL not render fieldmatch+decimate, SHALL not freeze
  `effectiveOutputFrameRate` as 24000/1001, SHALL not claim trusted progressive
  output, and SHALL expose the existing review/unknown warning state

### Requirement: Field-order precedence is deterministic

Field order SHALL be selected in this order: an explicit valid
`cadenceFieldOrder`, then a validated IVTC field order accepted by the existing
automatic-IVTC path, then unresolved/review. The validated detected value SHALL
come from the canonical IVTC validation boundary, such as its selected order;
generic `InterlaceAnalysis.DetectedFieldOrder` SHALL NOT qualify on its own.
Invalid, unsupported, conflicting, or unvalidated detected values SHALL NOT
override a valid explicit order or be treated as proof of a safe
inverse-telecine render.

#### Scenario: Explicit order wins over detected order

- **WHEN** a request contains a valid explicit field order and analysis reports
  a different detected order
- **THEN** the resolved cadence plan SHALL use the explicit order and retain
  the result as the authoritative requested intent

#### Scenario: Validated detection is used when no explicit order exists

- **WHEN** field order is automatic and analysis provides a validated detected
  order
- **THEN** the resolved cadence plan SHALL use that detected order for the
  canonical cadence render

#### Scenario: Invalid detection falls back conservatively

- **WHEN** field order is automatic and detection is missing, conflicting, or
  not validated
- **THEN** the system SHALL leave field order unresolved, SHALL expose the
  existing review-required or unknown state, and SHALL not guess TFF or BFF

### Requirement: Progressive output state requires processing or reliable evidence

`effectiveOutputProgressive` SHALL be true only when the resolved processing
actually produces progressive frames or reliable analysis evidence supports
that conclusion. A UI or operation label alone SHALL NOT mark output
progressive. Incomplete or unsupported evidence SHALL preserve the existing
unknown/review behavior.

#### Scenario: Rendered progressive processing supports the progressive state

- **WHEN** the resolved plan contains a validated progressive-producing
  cadence/deinterlace operation and the plan is accepted for rendering
- **THEN** the effective output SHALL be marked progressive with its resolved
  evidence

#### Scenario: Label-only progressive state is rejected

- **WHEN** a profile or operation is labeled progressive but its resolved
  processing and reliable analysis do not support progressive output
- **THEN** the system SHALL not report a trusted progressive result and SHALL
  retain the conservative review/unknown state

### Requirement: Existing cadence classifications remain safety-preserving

Validated automatic IVTC behavior SHALL remain unchanged. Suspected or
unvalidated telecine SHALL remain review-only and SHALL not trigger unsafe
normalization. Soft-telecine removal SHALL keep its rendered FPS filter and
reported effective FPS consistent.

#### Scenario: Validated automatic IVTC remains unchanged

- **WHEN** existing automatic analysis validates telecine and supplies the
  canonical automatic IVTC plan
- **THEN** the system SHALL retain the existing fieldmatch/decimate behavior
  and effective output semantics

#### Scenario: Suspected telecine remains review-only

- **WHEN** analysis suspects telecine without sufficient validation
- **THEN** the system SHALL expose a review-required result and SHALL not apply
  automatic IVTC normalization

#### Scenario: Soft-telecine removal reports the rendered rate

- **WHEN** soft telecine removal renders an FPS filter
- **THEN** the reported effective output FPS SHALL match that filter and SHALL
  not retain a contradictory source or requested cadence rate

### Requirement: Validation uses a weighted frame-rate evidence hierarchy

Validation SHALL use reliable frame count plus duration as the strongest
measured evidence. When reliable measured evidence is unavailable, it SHALL
use stream `avg_frame_rate` as the primary ffprobe stream-rate evidence.
`r_frame_rate` SHALL be diagnostic/corroborating base-rate metadata only and
SHALL not validate a result by itself. A reliable measured rate SHALL override
stream metadata for the effective validation result. Conflicting or incomplete
evidence SHALL produce a bounded mismatch, warning, or unverified result rather
than an invented exact FPS.

#### Scenario: Average stream rate is primary evidence

- **WHEN** output validation has a usable `avg_frame_rate` and only
  corroborating `r_frame_rate` metadata
- **THEN** the expected-versus-observed rate decision SHALL use
  `avg_frame_rate` as the primary stream-rate value

#### Scenario: Base rate corroborates without overriding

- **WHEN** `r_frame_rate` differs from `avg_frame_rate` because it represents
  base or container timing metadata
- **THEN** validation SHALL use it as corroborating evidence and SHALL not
  replace the primary average-rate decision solely because it is present

#### Scenario: Average rate without base-rate agreement remains primary

- **WHEN** `avg_frame_rate` matches the expected rate but `r_frame_rate` differs
- **THEN** the primary FPS result MAY remain validated and the disagreement
  SHALL remain available as diagnostic evidence

#### Scenario: Base rate alone is unverified

- **WHEN** `avg_frame_rate` is missing and only `r_frame_rate` matches the
  expected rate
- **THEN** validation SHALL remain unverified rather than accepting the base
  rate as proof

#### Scenario: Measured frame count and duration override stream metadata

- **WHEN** reliable frame count and duration are available
- **THEN** validation SHALL use the measured rate as the effective result while
  retaining stream metadata for diagnosis and conflict reporting; a measured
  contradiction SHALL be reported as a mismatch

### Requirement: Preview, Test Encode, and Queue share frozen cadence intent

Preview, Test Encode, and Queue SHALL resolve cadence intent through the same
canonical inputs and precedence rules. Given identical frozen source
analysis, requested cadence settings, explicit field order, and effective
configuration, they SHALL produce equivalent resolved cadence operation,
field-order decision, effective FPS, progressive state, and review status.
Queue SHALL consume its frozen resolved intent during execution rather than
reinterpreting mutable settings.

#### Scenario: Entry points agree for identical frozen inputs

- **WHEN** Preview, Test Encode, and Queue receive identical frozen cadence
  inputs and source analysis
- **THEN** each SHALL resolve the same cadence intent and effective output
  metadata

#### Scenario: Queue execution preserves the frozen decision

- **WHEN** cadence settings change after Queue freezes its resolved plan
- **THEN** Queue execution SHALL retain the frozen cadence intent and SHALL not
  resolve a different FPS or field order from mutable settings

#### Scenario: Conservative evidence is consistent across entry points

- **WHEN** identical inputs contain suspected or incomplete telecine evidence
- **THEN** Preview, Test Encode, and Queue SHALL all preserve review-only or
  unknown behavior without one surface applying unsafe normalization
