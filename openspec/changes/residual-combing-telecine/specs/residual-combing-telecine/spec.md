## Purpose

Define a bounded, backend-authoritative way to remove residual combing from
validated telecine output while preserving clean inverse-telecine cadence,
field-order decisions, progressive signaling, and cross-surface parity.

## ADDED Requirements

### Requirement: Residual combing is repaired selectively within validated IVTC

For every automatic inverse-telecine plan whose telecine classification has
been validated and whose field order resolves to TFF or BFF, the resolved
motion plan SHALL arm a `selective` residual-combing fallback between field
matching and decimation. The fallback SHALL process only frames that the field
matching stage marks as still interlaced or combed; clean progressive frames
SHALL pass through unchanged. Arming `selective` SHALL describe configured
runtime behavior, not claim that the asset was globally found to contain
combing or that every frame will be deinterlaced.

#### Scenario: Clean fieldmatched telecine arms selective fallback without damage

- **WHEN** field matching reconstructs a progressive telecine sequence without
  residual interlaced frames in the sampled or synthetic evidence
- **THEN** the effective plan SHALL still report the `selective` fallback as
  armed, progressive fieldmatched frames SHALL pass through unchanged, and the
  clean IVTC detail, motion, duplicate-frame removal, and cadence SHALL remain
  equivalent to the existing validated fieldmatch/decimate behavior

#### Scenario: Residual combed frames receive selective repair

- **WHEN** validated field matching leaves one or more output frames marked as
  interlaced or combed
- **THEN** the resolved chain SHALL selectively deinterlace those frames before
  decimation, while preserving one output frame per input frame through the
  fallback stage; the plan state SHALL remain `selective` whether zero or many
  frames are repaired at runtime

#### Scenario: Unvalidated telecine stays conservative

- **WHEN** telecine or field order is suspected but the canonical validation
  boundary has not accepted it
- **THEN** the system SHALL preserve the existing review/unknown behavior and
  SHALL not add an automatic residual-combing fallback or guess a field order

### Requirement: Sampled IVTC validation evidence remains diagnostic

The bounded IVTC validation evidence SHALL establish whether IVTC and TFF/BFF
parity are accepted, but SHALL NOT be used to predict that the full asset is
free of residual combing or to disable the configured selective fallback. Once
IVTC is accepted with a resolved field order, execution SHALL rely on
fieldmatch's per-frame marking. The system SHALL not perform a second full-asset
residual-combing analysis solely to decide whether the selective fallback is
present.

#### Scenario: Clean sampled windows do not disable the fallback

- **WHEN** sampled validation windows show progressive/classified ratios that
  support clean IVTC and the source resolves to TFF or BFF
- **THEN** those ratios SHALL remain diagnostic evidence and the effective plan
  SHALL still arm `residualFallback=selective`

#### Scenario: Validation ratios remain available as evidence

- **WHEN** IVTC validation evaluates TFF and BFF progressive/classified ratios
- **THEN** the accepted order and diagnostic ratios SHALL remain available to
  explain the decision without being interpreted as a full-asset residual-frame
  count or a guarantee of zero repaired frames

#### Scenario: Execution does not add a duplicate residual scan

- **WHEN** a validated IVTC plan is resolved for execution
- **THEN** the system SHALL arm the selective fallback from the accepted plan
  and SHALL not perform another full media analysis pass solely to decide
  whether to insert it

### Requirement: The fallback preserves the resolved cadence and output FPS

The selective fallback SHALL occur before the canonical decimation stage and
SHALL not introduce a second frame-rate conversion, timestamp rewrite, or
field-rate output mode. The effective output FPS SHALL remain the FPS resolved
by the existing cadence plan, including 24000/1001 for validated IVTC when that
is the resolved target.

#### Scenario: Validated IVTC retains its resolved FPS

- **WHEN** a validated telecine plan resolves to 24000/1001 and uses the
  residual-combing fallback
- **THEN** the rendered output SHALL retain the same decimation cadence and
  effective 24000/1001 FPS as the existing canonical plan

#### Scenario: Fallback does not double the frame rate

- **WHEN** the fallback processes residual combed frames
- **THEN** it SHALL use frame-rate-preserving behavior, and output frame count,
  timestamps, and duplicate-frame removal SHALL continue to be governed by
  the existing fieldmatch/decimate plan

#### Scenario: Output validation agrees with the resolved plan

- **WHEN** output validation measures frame count/duration or reads output
  frame-rate metadata
- **THEN** the expected FPS SHALL come from the frozen resolved cadence plan and
  SHALL be accepted only when measured or primary stream evidence agrees within
  the existing validation tolerance

### Requirement: Field order and parity remain resolver-driven

The field order selected by the canonical resolver SHALL be the single source
of parity for field matching and selective deinterlacing. Validated TFF and BFF
plans SHALL remain deterministic, and an unresolved order SHALL not be replaced
with a hard-coded default.

#### Scenario: TFF parity is preserved across the chain

- **WHEN** the resolver selects TFF for a validated telecine source
- **THEN** field matching and the selective deinterlacer SHALL consume TFF, and
  the effective motion decision SHALL report TFF as the selected field order

#### Scenario: BFF parity is preserved across the chain

- **WHEN** the resolver selects BFF for a validated telecine source
- **THEN** field matching and the selective deinterlacer SHALL consume BFF, and
  the effective motion decision SHALL report BFF as the selected field order

#### Scenario: Missing parity remains review-only

- **WHEN** neither explicit configuration nor validated analysis supplies TFF
  or BFF
- **THEN** the system SHALL not render an automatic IVTC/fallback chain and
  SHALL retain the existing review-required or unknown state

### Requirement: Successful IVTC output is signaled as progressive

After field matching, the armed selective residual-combing stage, and
decimation complete, the resolved plan SHALL preserve progressive output
metadata. Pixel-level progressive output SHALL not be accompanied by misleading
interlaced field signaling.

#### Scenario: Progressive metadata follows repaired IVTC output

- **WHEN** a validated IVTC plan renders successfully with its armed selective
  fallback, whether it repairs zero or many frames
- **THEN** the output SHALL carry the existing progressive field metadata and
  the effective plan SHALL report trusted progressive output

#### Scenario: Native interlaced processing is unchanged

- **WHEN** analysis classifies a source as true native interlaced rather than
  validated telecine
- **THEN** the existing canonical full interlaced-source deinterlace behavior
  SHALL remain authoritative and the residual-IVTC fallback SHALL not be used

#### Scenario: Progressive input does not gain a motion chain

- **WHEN** analysis classifies a source as native progressive
- **THEN** no telecine field matching, selective fallback, or decimation chain
  SHALL be added solely because this capability exists

### Requirement: All execution surfaces consume one resolved motion decision

Profile Lab, Compatible Preview, Test Encode, Queue freeze, and final worker
plan construction SHALL consume the same backend-resolved motion decision for
identical frozen analysis and effective configuration. No surface SHALL infer
residual combing or append a private fallback based only on UI state.

#### Scenario: Identical inputs produce identical motion chains

- **WHEN** the same frozen interlace/cadence analysis, selected field order,
  and effective profile are supplied to Preview, LAB, Test Encode, and Queue
- **THEN** each surface SHALL resolve the same fallback state, field order,
  cadence operation, progressive state, and effective FPS, subject only to
  documented transport-specific preview differences

#### Scenario: Queue preserves the frozen fallback decision

- **WHEN** profile or analysis settings change after Queue freezes the resolved
  plan
- **THEN** worker execution SHALL retain the frozen motion chain and SHALL not
  reclassify or remove the residual-combing fallback

### Requirement: The resolved plan explains the fallback decision

The effective execution decision SHALL distinguish requested cadence intent from
the resolved cadence operation, resolved field order, residual-combing fallback
state, output FPS, and progressive result. For validated IVTC that state SHALL
be `residualFallback=selective`, meaning “repair only fieldmatch output frames
still marked interlaced”; it SHALL not mean that the source was globally
classified as damaged or that every frame was deinterlaced. The decision SHALL
retain enough canonical information for later Frame Fidelity work to recognize
the intentional telecine transformation without adding Fidelity-specific
behavior in this change.

#### Scenario: Selective fallback selection is inspectable

- **WHEN** a validated telecine plan resolves TFF or BFF
- **THEN** the effective plan and diagnostic evidence SHALL identify the
  fallback as `selective`, its resolved parity, its position before decimation,
  and the reason as fieldmatch residual-combing semantics rather than an
  assertion that all frames require repair

#### Scenario: Non-IVTC paths remain distinguishable from selective IVTC

- **WHEN** the source is progressive, native interlaced, unvalidated telecine,
  or unresolved in field order
- **THEN** the effective plan SHALL report no IVTC selective fallback and SHALL
  preserve the applicable existing progressive, native-interlaced, or
  review/unknown behavior

#### Scenario: Existing provenance is preserved

- **WHEN** the resolved plan or output is persisted for Queue or validation
- **THEN** source analysis, field-order evidence, cadence decision, and the
  fallback rationale SHALL remain associated with the execution evidence
  without modifying source media or unrelated provenance records
