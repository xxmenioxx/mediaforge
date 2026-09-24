## Purpose

Provide one bounded, versioned source-to-output perceptual evidence contract
that evaluates already encoded artifacts on the frozen windows selected by
`SamplingPlan`, without turning any metric into a universal quality verdict.

## ADDED Requirements

### Requirement: Perceptual evidence SHALL be bound to one frozen SamplingPlan

The system SHALL expose a versioned `PerceptualEvidenceSet` whose identity and
provenance include the `SamplingPlan` schema/policy version, `PlanIdentity`,
source revision, and every measured `SamplingWindow.Identity`. The set SHALL
retain each window's canonical start, duration, inherited primary/roles/reasons,
and bounded fallback provenance. A consumer MAY filter the frozen windows for a
capability or presentation, but SHALL NOT select, search for, or substitute a
new scene.

#### Scenario: The same frozen plan is consumed by several surfaces

- **WHEN** LAB, Test Encode, and final Validation receive the same source
  revision, frozen `SamplingPlan`, candidate generation, reference identity,
  runtime capability identity, and metric/model versions
- **THEN** they SHALL address the same `PlanIdentity` and window identities, and
  SHALL produce the same evidence identity for every common comparable window

#### Scenario: A consumer requests a role that is not frozen

- **WHEN** a consumer asks for `high_motion`, `worst_predicted`, or another
  role that is absent from the frozen selected windows
- **THEN** the response SHALL mark that role unavailable and SHALL NOT choose a
  replacement interval or run an independent hard-scene search

#### Scenario: A legacy position-only plan is read

- **WHEN** an existing operation or Queue snapshot contains a readable legacy
  position-only plan
- **THEN** the system SHALL preserve its distributed-baseline meaning and
  provenance, SHALL not reinterpret it as hard-scene evidence, and SHALL either
  evaluate only its frozen intervals or report perceptual evidence unavailable
  when the required identities cannot be proven

### Requirement: The reference SHALL be canonical, lossless with respect to the evaluated transform, and authority-bound

For each comparable window, the system SHALL compare the candidate/output
against a canonical reference derived from the frozen effective video plan and
the existing preview/reference normalization authority. The reference identity
SHALL include the source revision, source stream identity, `SamplingPlan` and
window identity, transform identity and version, time/frame alignment contract,
and normalization provenance. The reference SHALL exclude the lossy encoder
being evaluated and SHALL retain raw source identity separately from transformed
reference evidence.

#### Scenario: An intentional cadence or geometry transform is frozen

- **WHEN** the frozen plan changes cadence/FPS, field structure, crop, scale,
  SAR/DAR, pixel domain, or color policy
- **THEN** the comparison SHALL use a reference produced by those frozen
  canonical transforms before comparing the encoded candidate, and SHALL NOT
  compare the untransformed raw source directly to the candidate as if no
  transform existed

#### Scenario: No intentional transform changes the compared domain

- **WHEN** the frozen effective plan preserves the source comparison domain
  and the existing normalization authority confirms compatible geometry and
  color semantics
- **THEN** the canonical reference MAY reuse the source decode domain while
  retaining explicit source, reference, and normalization identities

#### Scenario: A physical reference cannot be produced safely

- **WHEN** the frozen authority is missing, the source stream is unavailable,
  the transform is unsupported, or source-to-candidate alignment is ambiguous
- **THEN** the window SHALL be reported as `non_comparable` or `unavailable`
  with a bounded reason and SHALL NOT be measured against an inferred raw
  source or a lossy intermediate

### Requirement: Time and frame alignment SHALL be explicit and deterministic

The evidence contract SHALL record a versioned alignment identity that maps the
canonical source timeline through the intentional reference transforms to the
candidate/output timeline. The mapping SHALL account for frozen cadence/IVTC,
deinterlace, frame-rate, segment-start, and timestamp normalization decisions;
it SHALL NOT assume that equal frame indexes or equal wall-clock timestamps are
equivalent after a structural transform. Alignment tolerance and any dropped,
duplicated, or unmatched frame count SHALL be bounded provenance.

#### Scenario: IVTC changes source and output frame cadence

- **WHEN** a window comes from a 29.97 interlaced source and the frozen plan
  expects 23.976 progressive output
- **THEN** the reference/candidate pair SHALL use the frozen cadence mapping and
  compare aligned frames in the expected timeline, rather than pairing source
  and output frames by raw index

#### Scenario: A Test Encode is a trimmed segment

- **WHEN** a Test Encode candidate begins at a non-zero source time and its
  output timestamps are normalized to the segment
- **THEN** the alignment record SHALL bind the segment start and duration from
  the frozen window and SHALL not compare its first frame with source time zero

### Requirement: A comparable pair SHALL have exact identity and staleness fencing

The candidate identity SHALL include the candidate kind, canonical output path
or artifact identity, source/job/test identity where available, output revision
or artifact generation token/version, and frozen effective execution identity.
The comparable-pair/evidence identity SHALL include candidate identity,
reference identity, `SamplingPlan` identity, window identity, metric contract
version, metric/model versions, and runtime capability identity. Evidence SHALL
become stale when any relevant source, plan, window, transform/reference,
candidate generation/output, metric/model, or capability identity changes.

#### Scenario: The same path is replaced

- **WHEN** a source or candidate file at the same path changes its source/output
  revision, generation, or validated identity
- **THEN** old perceptual evidence SHALL not be reused, even if the pathname and
  requested interval are unchanged

#### Scenario: A Compatible Preview generation is replaced

- **WHEN** a ready artifact's generation token or generation version changes
  after evidence was frozen
- **THEN** the evidence SHALL be marked stale/retryable and SHALL not attribute
  generation-B measurements to the generation-A operation

#### Scenario: An exact evidence identity is unchanged

- **WHEN** a repeated request has the same complete pair identity and its
  referenced artifacts remain valid
- **THEN** the system MAY reuse the bounded evidence and reference/metric
  results without re-encoding or remeasuring them

### Requirement: Metric collection SHALL be capability-driven and evidence-first

The initial metric set SHALL support VMAF, XPSNR, SSIM, optional MS-SSIM, and
diagnostic PSNR. Runtime availability SHALL be discovered from the actual MVForge
FFmpeg runtime and any required model, not inferred from a Docker image or a
hardcoded build assumption. Capability provenance SHALL include a stable
capability identity/version and the metric filter/model contract used.

Each metric result SHALL distinguish at least `available`, `unsupported`,
`unavailable`, `failed_measurement`, and `non_comparable`; absent values SHALL
not be encoded as zero. A missing optional metric SHALL not make the complete
evidence set fail when other metrics can be measured.

#### Scenario: VMAF is absent but other metrics are usable

- **WHEN** the runtime has no usable `libvmaf` filter/model but can execute
  XPSNR, SSIM, or PSNR for a comparable pair
- **THEN** VMAF SHALL be `unavailable` with capability provenance, and each
  other usable metric SHALL be measured and reported independently

#### Scenario: The runtime supports MS-SSIM only on some builds

- **WHEN** the runtime capability probe reports MS-SSIM unsupported
- **THEN** the evidence SHALL preserve that capability state, SHALL not invoke
  an unsupported filter for every window, and SHALL continue supported metrics

#### Scenario: A metric process fails after another metric succeeds

- **WHEN** one metric command or filter returns a measurement error for a
  comparable window while another metric has a valid result
- **THEN** the failed metric SHALL be `failed_measurement` with bounded
  diagnostics, the successful metric SHALL remain available, and the window or
  operation SHALL not convert the successful result to zero

### Requirement: Per-window evidence SHALL preserve metric semantics and provenance

Every window record SHALL include the frozen `SamplingWindow.Identity`,
start/duration, primary flag, inherited roles/reasons, reference identity,
candidate identity, alignment identity, and a bounded result for each requested
metric. A metric result SHALL retain its direction/semantics (for example,
higher-is-better), implementation/filter contract, model/version when
applicable, units/range when defined, status, value when available, and a
bounded reason or warning. VMAF, XPSNR, SSIM, MS-SSIM, and PSNR SHALL remain
distinct metrics and SHALL not be treated as interchangeable.

#### Scenario: A window is non-comparable because geometry differs

- **WHEN** reference and candidate cannot be aligned in the canonical domain
  because required dimensions or transform semantics differ
- **THEN** each affected metric SHALL report `non_comparable` with the window's
  reference/candidate identities and reason, without fabricating a numeric
  result

#### Scenario: A metric has a valid infinite PSNR result

- **WHEN** the runtime reports a mathematically infinite diagnostic PSNR for an
  otherwise comparable pair
- **THEN** the contract SHALL preserve the metric's defined representation and
  semantics, rather than silently treating it as unavailable or a zero

### Requirement: Aggregation SHALL be deterministic, bounded, and traceable

Per-window evidence SHALL remain canonical. For each metric, the bounded
aggregate SHALL retain comparable-window count, non-comparable/unavailable/
failed counts, median/typical value, lower-percentile evidence, worst measured
value, the metric's semantics, and the identity of the worst measured window.
Unavailable, unsupported, failed, and non-comparable windows SHALL be excluded
from numeric aggregates and SHALL remain visible in their counts/reasons.

The aggregate policy SHALL be versioned and deterministic: comparable values
are ordered by metric value and then `SamplingWindow.Identity`; median uses the
middle value or the arithmetic mean of the two middle values; the initial
lower-percentile evidence is p10 using nearest-rank selection; and worst means
the lowest value for the initial higher-is-better metrics. A percentile with
fewer than the policy's minimum evidence count SHALL retain the deterministic
nearest-rank value while marking degraded percentile provenance. No aggregate
field SHALL itself imply pass/fail.

#### Scenario: Unavailable windows are mixed with comparable windows

- **WHEN** two windows have valid SSIM values, one is unavailable, and one is
  non-comparable
- **THEN** the SSIM aggregate SHALL count two comparable windows, exclude the
  other two from median/p10/worst numeric values, and retain both exclusion
  counts and reasons

#### Scenario: Two windows tie for worst value

- **WHEN** two comparable windows have the same worst metric value
- **THEN** the aggregate SHALL choose the same window on every run using the
  stable `SamplingWindow.Identity` tie-break and SHALL retain that identity

#### Scenario: Too few windows support p10

- **WHEN** fewer than the versioned minimum number of comparable windows have a
  result for a metric
- **THEN** the aggregate SHALL use its deterministic nearest-rank value from
  the available results, mark the lower-percentile evidence as degraded, and
  SHALL not fill missing windows with zero or claim full percentile coverage

### Requirement: LAB SHALL extend existing Fidelity ownership without hidden encoding

LAB perceptual evidence SHALL attach beside existing size, bitrate, effective
encoder, speed, and canonical Frame Fidelity evidence. It SHALL consume the
frozen #70 plan and ready Compatible Preview artifacts/generation identities;
the primary artifact SHALL be reused and SHALL not be encoded again for metrics.
Additional planned windows SHALL remain bounded and sequential under existing
LAB/Compatible Preview ownership. Perceptual analysis SHALL not create a second
operation state machine, a second selector, or a hidden recomputation caused by
tab changes or read-only rendering.

The operation SHALL expose queued/running/completed/failed/canceled state,
bounded progress by frozen window identity, cancellation, retryable stale or
resource errors, and partial per-window evidence. A non-comparable window is a
terminal evidence result; cancellation or infrastructure failure is not a
quality mismatch.

#### Scenario: The primary Compatible Preview is already ready

- **WHEN** LAB starts perceptual analysis for a ready primary artifact
- **THEN** the operation SHALL join that artifact generation, measure it without
  another encode, and bind the result to the primary frozen window identity

#### Scenario: A read-only LAB tab changes

- **WHEN** a user switches between LAB evidence tabs after an operation has a
  snapshot or while it is running
- **THEN** rendering SHALL read the existing operation/evidence snapshot and
  SHALL not start a new encode, metric process, or window selection

#### Scenario: LAB additional work is canceled

- **WHEN** the user cancels while an additional frozen window is queued or being
  measured
- **THEN** operation-owned processes, leases, and temporary evidence resources
  SHALL be released, the primary ready artifact SHALL remain usable, and no
  partial result SHALL be presented as a final quality verdict

### Requirement: Test Encode and final Validation SHALL reuse the same contract

Test Encode SHALL expose the canonical `PerceptualEvidenceSet` when a compatible
encoded output artifact and frozen `SamplingPlan` exist. Final Validation SHALL
consume the plan and evaluate the same reference/candidate contract against the
final output, retaining expected/reference-to-actual provenance. Neither surface
SHALL reinterpret or reselect scenes. The same source, plan, window, reference,
candidate, capability, and metric identities SHALL address the same evidence
identity across surfaces.

#### Scenario: Test Encode has a compatible output and frozen plan

- **WHEN** a completed Test Encode has a validated output identity and its
  frozen plan contains selected windows
- **THEN** Test Encode SHALL expose per-window and aggregate perceptual evidence
  using the shared contract, not a Test-Encode-specific metric schema

#### Scenario: Final Validation consumes Queue windows

- **WHEN** final Validation validates a completed Queue output with a frozen
  plan in the Queue snapshot
- **THEN** it SHALL evaluate those exact window identities and record the
  evidence in the validation report without performing a new scene search

#### Scenario: Optional capability is absent during Validation

- **WHEN** final output validation cannot use VMAF or another optional metric
  while SSIM/PSNR or other supported metrics remain measurable
- **THEN** Validation SHALL retain the partial canonical evidence and degrade
  capability reporting safely without turning metric absence into a failed
  output or an invented score

### Requirement: Structured artifact-regression evidence SHALL remain separate from quality metrics

The evidence contract SHALL provide an extension boundary for bounded,
deterministic signals such as blocking, blur/detail loss, banding, ringing/halo,
and temporal instability. These signals SHALL retain their own status,
window/reference/candidate identity, evidence source/version, and reason, and
SHALL not be folded into VMAF/XPSNR/SSIM/PSNR values. Issue #69 MAY reuse an
existing deterministic signal only when its semantics and identity match; it
SHALL not require implementing every analyzer in this change.

#### Scenario: A deterministic artifact signal is available

- **WHEN** an existing bounded analyzer provides a semantically matching
  ringing or detail-loss result for a frozen window
- **THEN** the evidence SHALL attach it under structured artifact evidence with
  independent provenance and SHALL leave metric aggregates unchanged

#### Scenario: No artifact analyzer is available

- **WHEN** the runtime or current implementation has no canonical banding,
  blur, or temporal-instability analyzer for a window
- **THEN** that artifact signal SHALL be unavailable with a bounded reason and
  SHALL not block the supported perceptual metrics

### Requirement: Persisted evidence and runtime work SHALL remain bounded and backward compatible

Persisted or returned evidence SHALL contain bounded identities, summaries,
window results, counts, warnings, capability states, and provenance; it SHALL
not contain full decoded frames, unlimited packet lists, full FFmpeg logs, or a
second general media cache. Metric/reference work SHALL be bounded by the
maximum windows in the frozen plan, use bounded concurrency/sequential LAB
ownership, support cancellation, and clean operation-owned temporary state.
Existing LAB Frame Fidelity, Compatible Preview, Test Encode, and Validation
consumers SHALL continue to work when perceptual evidence is absent or when
reading older snapshots/reports.

#### Scenario: The SamplingPlan maximum is reached

- **WHEN** a frozen plan contains its configured maximum number of windows
- **THEN** perceptual analysis SHALL evaluate no additional intervals by
  default, and persisted provenance SHALL remain within its configured bound

#### Scenario: An old LAB result has no perceptual evidence

- **WHEN** a legacy LAB operation or report contains existing Frame Fidelity
  and legacy SSIM/PSNR fields but no `PerceptualEvidenceSet`
- **THEN** existing fields SHALL remain readable, the new evidence SHALL be
  reported unavailable rather than reconstructed from mutable state, and the
  operation SHALL not require a database/media migration to display the legacy
  result

#### Scenario: Evidence cleanup runs during cancellation

- **WHEN** evidence measurement is canceled, fails, or reaches retention expiry
- **THEN** operation-owned process handles, artifact-use protections, and
  temporary files SHALL be released without deleting source media, ready shared
  artifacts, reports, or unrelated sidecars

### Requirement: Evidence SHALL remain advisory until a future policy owns interpretation

The canonical evidence contract SHALL preserve metric values, direction,
semantics, model/version, comparability, and provenance without defining an
arbitrary initial threshold such as `VMAF < X => FAIL`. LAB may expose a
backward-compatible legacy visual projection while clearly distinguishing it
from the canonical evidence status. Future #53 Quality Gate or other policy
work MAY interpret the evidence as advisory or policy-driven only through a
versioned policy that is separate from metric collection.

#### Scenario: A low metric is measured without a gate policy

- **WHEN** a comparable window has a low VMAF, XPSNR, SSIM, MS-SSIM, or PSNR
  value but no quality-gate policy is active
- **THEN** the evidence SHALL report the measured value and its semantics but
  SHALL not emit a universal fail verdict solely from that value

#### Scenario: A canonical Frame Fidelity mismatch is present

- **WHEN** the frozen output authority detects an intentional-plan mismatch and
  perceptual metrics are high
- **THEN** consumers SHALL retain both canonical Frame Fidelity mismatch and
  perceptual evidence, and SHALL not let high perceptual values erase the
  structural mismatch
