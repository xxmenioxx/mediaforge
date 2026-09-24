## Purpose

Define a bounded, evidence-backed way to investigate temporal grain-to-blur
pumping in HEVC QSV while preserving canonical sampling, preview, encoder, and
profile ownership.

## ADDED Requirements

### Requirement: QSV pumping experiments SHALL consume only frozen canonical windows

The experiment SHALL consume the frozen #70 `SamplingPlan` through
`SamplingPlanForQSVPumping()`. Its preferred role request SHALL be exactly
`high_motion`, `high_texture_or_grain`, and `worst_predicted`; the experiment
SHALL retain the returned plan identity, policy version, window identities, and
unavailable-role list. It SHALL NOT select a new worst scene, scan the full
file solely for this issue, or reinterpret an unavailable role. A user-selected
LAB primary/repro window MAY be included as a controlled window in addition to
the available role windows.

#### Scenario: Available #70 roles are reused

- **WHEN** a frozen plan contains one or more of the requested QSV roles
- **THEN** the experiment SHALL compare only those existing window identities
  and SHALL report the same plan/policy provenance

#### Scenario: A requested role is missing

- **WHEN** `high_motion`, `high_texture_or_grain`, or `worst_predicted` is not
  present in the frozen plan
- **THEN** the experiment SHALL report that role as unavailable, SHALL NOT
  select a replacement or trigger another analyzer, and MAY continue with the
  remaining available roles and explicit primary

#### Scenario: LAB primary is outside the role subset

- **WHEN** the user-selected primary/repro interval has no requested hard-scene
  role
- **THEN** the primary SHALL remain available as its original frozen window and
  SHALL NOT be replaced or encoded again because role selection chose other
  windows

### Requirement: Each experiment SHALL have deterministic identity and prove isolation

The system SHALL record a versioned `QSVTemporalExperiment` identity containing
at least: source revision; `SamplingPlan` and `SamplingWindow` identities;
baseline and variant candidate identities; the one changed variable; requested
and effective baseline/variant values; encoder/runtime identity; filter and
effective-plan identity; evidence references; result classification; and
warnings/provenance. Candidate identity SHALL include the frozen interval and
canonical source/effective-plan facts, not only a display label.

Before causal comparison, the system SHALL compare canonical effective settings
and prove that the allowed effective delta is exactly the declared variable
(plus explicitly versioned derived fields of that variable). If another
effective setting differs unexpectedly, the result SHALL be
`invalid/non-isolated` and SHALL NOT claim that the declared variable caused the
observed change.

#### Scenario: A GOP-only comparison is isolated

- **WHEN** baseline and variant use the same source revision, frozen window,
  filters, encoder/runtime, rate-control, B-frame behavior, and all other
  effective settings, while only effective GOP differs
- **THEN** the experiment SHALL be eligible for a GOP causal comparison

#### Scenario: An unexpected second effective variable changes

- **WHEN** a BF-only candidate also changes effective rate control or GOP after
  capability resolution
- **THEN** the experiment SHALL be classified `invalid/non-isolated`, SHALL
  preserve both effective snapshots and the unexpected delta, and SHALL NOT
  attribute pumping to BF

#### Scenario: Restoration filters are present but unchanged

- **WHEN** hqdn3d, chromanr, denoise, sharpening, grain/noise, scale, or other
  restoration filters are present in both candidates with the same effective
  filter plan
- **THEN** their presence SHALL be recorded as frozen context and SHALL NOT be
  treated as evidence that restoration caused pumping

### Requirement: Requested and effective QSV behavior SHALL be captured

Every baseline and variant SHALL record requested encoder settings separately
from effective settings after canonical profile, capability, and runtime
resolution. The capture SHALL cover the relevant quality/rate-control values,
GOP and B-frame strategy, supported look-ahead/ExtBRC/MBBRC/adaptive controls,
effective encoder, fallback/downgrade reasons, and runtime capability identity.
Only controls reported as supported by the existing runtime capability model
MAY be varied. Unsupported or unverified controls SHALL be reported as
unavailable or warning state, not emitted as invented flags.

#### Scenario: Requested QSV quality falls back

- **WHEN** a requested QSV rate-control or `global_quality` value resolves to a
  different effective mode/value
- **THEN** both values, the runtime identity, and the fallback reason SHALL be
  captured, and comparison SHALL use effective behavior for isolation

#### Scenario: An optional QSV feature is unsupported

- **WHEN** a candidate requests look-ahead, ExtBRC, MBBRC, adaptive I/B, or
  another existing QSV control that the probed runtime does not support
- **THEN** the candidate SHALL not add an undocumented flag, SHALL report the
  control as unavailable, and SHALL remain eligible only for an experiment that
  does not claim that control changed

### Requirement: LAB SHALL reuse the ready baseline and preserve reference identity

Variant execution SHALL extend the existing LAB/Compatible Preview operation
and artifact lifecycle. The ready primary Compatible Preview artifact SHALL be
the baseline when it matches the frozen source revision, window, filters,
encoder, and effective plan. Baseline reuse SHALL be proven by its request ID,
artifact ID, generation token/version, canonical encode signature, and a
generation-boundary reuse count; a request-registration count alone is not
sufficient. A variant SHALL receive a distinct candidate/artifact identity and
SHALL never replace the baseline identity or the source-backed Sample A/
reference identity.

#### Scenario: A ready primary is reused

- **WHEN** LAB starts an experiment after the primary preview is ready and its
  material identity still matches
- **THEN** the operation SHALL inspect the ready baseline without starting a
  second baseline generation and SHALL retain the same baseline identity

#### Scenario: The primary becomes stale

- **WHEN** the source revision, artifact generation, interval, filter plan, or
  effective encode signature no longer matches the frozen operation
- **THEN** the experiment SHALL fail or become retryable as stale, SHALL NOT
  compare mixed generations, and SHALL leave the prior ready baseline and
  Sample A/reference lifecycle intact

#### Scenario: A variant completes

- **WHEN** a variant Compatible Preview artifact is ready
- **THEN** its result SHALL reference the variant identity while the baseline
  and Sample A/reference identities remain unchanged

### Requirement: Variant execution SHALL be explicit, sequential, bounded, and cancellable

The workflow SHALL use the existing LAB operation states and cancellation
semantics. It SHALL perform a baseline evidence step followed by explicitly
requested single-variable variants in deterministic order, with at most one
heavy QSV encode/evidence phase active for the operation and no hidden work
caused by changing tabs. The default experiment policy SHALL cap the number of
variants at three, cap retained candidate/temporal summaries, and reject or
bound additional requests before encoding. It SHALL never automatically
generate a Cartesian product of variable families. A later experiment MAY use a
chosen variant as its baseline only through explicit user or policy action.

#### Scenario: Multiple variants are requested

- **WHEN** a user requests quality-only, GOP-only, and BF-only comparisons
- **THEN** the system SHALL run no more than the bounded variant count,
  process them sequentially, and compare each directly with the same baseline
  rather than generating combinations

#### Scenario: Cancellation occurs during a variant

- **WHEN** the user cancels while a variant is preparing, encoding, or yielding
  evidence
- **THEN** cancellation SHALL stop the current operation through its existing
  context, release artifact/heavy-work protection, retain bounded terminal
  provenance, and leave the ready baseline playable

#### Scenario: A tab changes without an explicit action

- **WHEN** the user changes LAB tabs or rerenders the page without selecting a
  new experiment action
- **THEN** no baseline or variant encode SHALL start

### Requirement: Temporal evidence SHALL measure short temporal behavior without claiming a universal detector

The experiment SHALL support a bounded, deterministic temporal evidence
contract over adjacent/subsequent decoded frames or short temporal segments in
each frozen window. Evidence MAY include detail/grain-energy loss and recovery,
localized blur/detail variation, and temporal instability in textured regions,
but SHALL identify its frame span, normalization, sampling method, and
availability. The result SHALL keep `measured temporal evidence` separate from
the versioned interpretation policy. A single-frame score SHALL never be
sufficient to claim pumping.

#### Scenario: Detail loss recovers across adjacent frames

- **WHEN** a deterministic fixture contains a short sequence whose textured
  detail energy falls and then recovers in the candidate output
- **THEN** the experiment SHALL retain the measured frame-sequence evidence and
  MAY report `likely pumping` only when the versioned interpretation policy and
  confidence requirements are satisfied

#### Scenario: Evidence is insufficient

- **WHEN** the window has too few comparable frames, lacks a deterministic
  texture/detail measurement, or the evidence provider is unavailable
- **THEN** the experiment SHALL report `inconclusive` or an unavailable-evidence
  warning and SHALL NOT claim pumping from a single-frame metric

#### Scenario: No temporal instability is measured

- **WHEN** the bounded frame sequence has adequate coverage and does not show
  the policy's loss/recovery or instability pattern
- **THEN** the result MAY be `not observed` for that tested window while
  preserving the measured evidence and policy version

### Requirement: Optional #69 evidence SHALL attach without becoming a dependency

The experiment SHALL accept canonical #69 `PerceptualEvidence` when it is
available for the same source revision, plan identity, window identity, and
candidate artifact generation. It SHALL store only bounded references or
summaries and SHALL not implement a second VMAF/SSIM pipeline. If #69 evidence
is absent, stale, incompatible, or fails, the experiment SHALL continue with
deterministic temporal, Frame Fidelity, effective-setting, bitstream, size,
bitrate, and speed evidence, with a bounded warning.

#### Scenario: Canonical perceptual evidence is available

- **WHEN** #69 provides evidence matching the frozen experiment identities
- **THEN** the experiment SHALL attach it by reference and include its version
  in provenance without reselecting windows

#### Scenario: #69 is not available

- **WHEN** no matching canonical perceptual evidence exists
- **THEN** the experiment SHALL remain runnable and SHALL not fail solely
  because #69 is absent

### Requirement: Results and mitigations SHALL be conservative and versioned

The experiment SHALL support the classifications `reproducible QSV-specific
pumping`, `measurable but not clearly QSV-specific`, `inconclusive`, and `no
pumping in tested windows`. A QSV-specific classification SHALL require a
matching non-QSV/control basis or equivalent versioned evidence; QSV-only
measurements SHALL not be promoted to that claim. Any mitigation suggestion
SHALL be versioned and include its evidence basis, applicable encoder/runtime,
media characteristics, confidence, and size/speed/compression trade-offs.
Results SHALL never mutate every QSV Profile automatically.

#### Scenario: A mitigation is supported by isolated evidence

- **WHEN** an isolated comparison and temporal evidence support a lower GOP,
  BF strategy, quality/rate-control change, or supported QSV feature change
- **THEN** the system SHALL expose a versioned recommendation tied to the
  evidence and applicability metadata, without changing the existing Profile

#### Scenario: A recommendation is applied explicitly

- **WHEN** a user or explicit policy action chooses a versioned recommendation
- **THEN** it MAY seed a new/draft Profile or an explicit Profile edit, while
  existing Profiles and unrelated assets remain unchanged

#### Scenario: Decoder compatibility differs

- **WHEN** bitstream/frame-structure evidence shows a playback or device
  compatibility concern without temporal pumping evidence
- **THEN** the result SHALL report that concern separately and SHALL not label it
  as temporal quality pumping

### Requirement: Experiments SHALL remain backward-compatible and testable without NAS media

Existing LAB preview, Frame Fidelity, Queue, and Profile behavior SHALL remain
unchanged when no QSV temporal experiment is explicitly requested. Experiment
logic SHALL expose fake/injectable encoder, capability, artifact, and evidence
seams so identity, isolation, lifecycle, and temporal evidence can be tested
with fixtures. Unit tests SHALL not require NAS paths, network access, real
media, or QSV hardware.

#### Scenario: Existing LAB run does not opt in

- **WHEN** a normal LAB preview/Fidelity run is started without a QSV temporal
  experiment
- **THEN** it SHALL use the existing operation and artifact behavior with no
  additional variant work

#### Scenario: Deterministic fixtures are used

- **WHEN** the experiment tests adjacent-frame loss/recovery, role absence, or
  effective-setting fallback
- **THEN** the test SHALL use injected deterministic inputs and produce stable
  identities/evidence without accessing NAS media, a network, or hardware
