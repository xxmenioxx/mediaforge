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
  SHALL produce the same evidence identity for every candidate-covered common
  comparable window

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

### Requirement: Candidate coverage SHALL be explicit for every frozen window

Every candidate SHALL expose a bounded `CandidateCoverage` contract containing
its own `coverageIdentity`, source start, source duration, covered frozen
window identities, and a versioned segment/timeline mapping identity. The
evidence set SHALL distinguish
the total frozen windows, candidate-covered windows, measured/comparable
windows, and frozen windows not represented by that candidate. A window that is
not represented SHALL use an explicit coverage state such as `not_covered` with
a bounded reason; it SHALL NOT be converted into a metric failure or an
unavailable metric result.

#### Scenario: A Test Encode covers only the primary window

- **WHEN** a frozen `SamplingPlan` contains six windows but a Test Encode
  candidate represents only the primary source segment
- **THEN** candidate coverage SHALL list only the primary window identity, and
  the evidence set SHALL report five frozen windows as `not_covered` while
  retaining their start, duration, roles, reasons, and identities

#### Scenario: Test Encode measures only represented windows

- **WHEN** a Test Encode has a proven source start/duration mapping for one
  frozen window
- **THEN** only that covered window MAY produce metric measurements, and the
  other frozen windows SHALL not be read from the candidate as though they were
  present

#### Scenario: Uncovered windows do not fail Test Encode quality evidence

- **WHEN** unrelated frozen windows are outside the Test Encode segment
- **THEN** they SHALL remain traceable as `not_covered`, SHALL be excluded from
  comparable/metric failure counts and aggregates, and SHALL not be replaced by
  a selector or another encode

#### Scenario: Final output proves full coverage

- **WHEN** final output source-to-candidate timeline identity proves that the
  complete output represents all frozen source windows
- **THEN** candidate coverage MAY list all frozen window identities and the same
  window identities MAY later produce comparable final-output evidence

#### Scenario: Candidate mapping changes

- **WHEN** a Test Encode starts at a different source time, has a different
  duration, or otherwise changes its segment mapping
- **THEN** its candidate coverage identity SHALL change and previously persisted
  evidence SHALL become stale even if the output path is unchanged

### Requirement: Multi-window evidence SHALL own exact per-window comparison pairs

`PerceptualEvidenceSet` SHALL represent a bounded collection of
`PerceptualWindowEvidence` entries in the canonical frozen `SamplingPlan`
window order; it SHALL NOT use one top-level `reference`, `candidate`, or
`coverage` identity as a substitute for those entries. The set MAY use bounded
`candidateManifest[]` and `referenceManifest[]` entries to deduplicate
provenance. Each covered window entry SHALL independently expose or resolve
its `windowIdentity`, `referenceIdentity`, `candidateIdentity`,
`coverageIdentity`, and `pairIdentity`, plus its metric results and state.

The `ComparisonPairIdentity` exposed as `pairIdentity` SHALL bind exactly the
material comparison inputs: `SamplingPlan.PlanIdentity`,
`SamplingWindow.Identity`, complete `ReferenceIdentity`, complete
`CandidateIdentity`, `CandidateCoverage`/segment-mapping identity,
alignment/comparison-domain identity, and the relevant source and candidate
revisions. It SHALL NOT include a logical metric/provider/model contract. It
SHALL exclude operation ID, UI order, map iteration order, filesystem
enumeration order, and a path used as a substitute for artifact/output
identity. A candidate-covered window SHALL prove membership in its referenced
`CandidateCoverage.coveredWindowIdentities`.

Each requested metric result SHALL have its own deterministic
`MetricResultIdentity`, derived from the material `pairIdentity` plus metric
descriptor identity, logical metric, provider/provider version, invocation
contract, model identity when applicable, and runtime capability identity.
Changing a provider or model therefore changes the affected metric-result
identity without changing the underlying material pair identity.

A `not_covered` window SHALL remain in the ordered window ledger with its
frozen interval, roles/reasons, and bounded reason, but SHALL have no candidate
manifest reference, coverage identity, or pair identity. A candidate identity
MAY be referenced by multiple windows, while each window retains its own
reference, alignment, and pair identity. The set identity SHALL be derived
from schema/policy versions, `SamplingPlan` identity, source revision, runtime
and capability contract, requested metric contracts including provider/model
identities, and the canonical ordered window ledger containing material pair
identities for covered windows and explicit `not_covered` markers for
uncovered slots. It SHALL not depend on map, goroutine, UI, filesystem, or
operation ordering.

#### Scenario: LAB has distinct Compatible Preview generations per window

- **WHEN** a frozen plan has windows A, B, and C and LAB has ready Compatible
  Preview artifacts with distinct artifact/generation identities for each
- **THEN** the evidence SHALL contain three candidate manifest entries and
  three per-window comparison pairs, each bound to its corresponding artifact
  and window, with no singular candidate identity standing for all three

#### Scenario: One material pair produces several metric results

- **WHEN** one covered window is measured with VMAF, SSIM, and PSNR
- **THEN** the window SHALL contain exactly one material `pairIdentity` and
  three distinct `MetricResultIdentity` values, each retaining its own logical
  metric/provider/model contract

#### Scenario: A VMAF provider changes without changing the material pair

- **WHEN** VMAF resolves to a different provider, provider version, model, or
  invocation contract while source/reference/candidate material is unchanged
- **THEN** the VMAF metric-result identity and containing evidence-set identity
  SHALL change, while the window's `pairIdentity` SHALL remain unchanged

#### Scenario: Candidate generation changes all metric results for that pair

- **WHEN** a covered candidate artifact generation or candidate revision changes
- **THEN** the affected window's material `pairIdentity` SHALL change and every
  metric result referencing the old pair SHALL become stale

#### Scenario: Requested metric membership changes only the set contract

- **WHEN** a request adds or removes a metric contract while material windows,
  references, candidates, and coverage remain unchanged
- **THEN** the evidence-set identity SHALL change, while existing material pair
  identities SHALL remain unchanged and reusable

#### Scenario: Replacing only LAB window B invalidates only B and the set

- **WHEN** the Compatible Preview generation for window B is replaced while
  the source, reference, and candidate identities for window A remain valid
- **THEN** B's candidate/pair evidence and the containing set identity SHALL
  become stale, while A's reusable reference and metric evidence MAY remain
  valid when its exact identities still match

#### Scenario: One final output candidate covers several windows

- **WHEN** final-output timeline proof shows that one output revision covers
  windows A, B, and C
- **THEN** the set MAY reference one deduplicated final-output candidate entry
  from all three windows, but SHALL retain distinct reference, alignment,
  coverage-membership, and pair identities for A, B, and C without duplicating
  or re-encoding the final output

#### Scenario: Test Encode creates a pair only for primary coverage

- **WHEN** a frozen multi-window plan is evaluated against a Test Encode
  segment whose `CandidateCoverage` lists only the primary window
- **THEN** only the primary window SHALL receive a candidate reference and
  comparison pair; every other frozen window SHALL remain `not_covered` and
  shall not be read from the segment as if it contained that window

#### Scenario: Not-covered windows never receive a fake pair

- **WHEN** a frozen window is absent from a candidate's covered-window list
- **THEN** its evidence entry SHALL expose `not_covered` and a bounded reason,
  SHALL be excluded from pair/metric aggregates, and SHALL not contain a
  synthesized candidate or pair identity

#### Scenario: Set identity is independent of map or goroutine order

- **WHEN** the same material per-window pairs are collected through different
  map iteration, filesystem enumeration, or goroutine completion orders
- **THEN** the frozen plan window order SHALL produce the same manifest order,
  ordered pair ledger, pair identities, metric-result identities, set identity,
  aggregates, and worst-window identity

#### Scenario: Metric cache separates providers for one pair

- **WHEN** two metric executions use the same material pair but different
  providers, models, or invocation contracts
- **THEN** metric-result cache entries SHALL remain distinct and SHALL not
  return one provider's result for the other

#### Scenario: Retrying an operation preserves material evidence identity

- **WHEN** an operation is retried with a different operation ID but the same
  source, plan, window, reference, candidate, coverage, capability, and metric
  identities
- **THEN** the pair identities and set identity SHALL remain unchanged and
  bounded cached evidence MAY be reused

#### Scenario: Candidate revision invalidates every referencing pair

- **WHEN** a final-output or other shared candidate revision/generation changes
- **THEN** every per-window pair referencing that candidate SHALL become stale,
  while unrelated candidates and their exact reference/metric evidence MAY
  remain reusable

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
It SHALL also include the segment/coverage mapping identity and the covered
frozen window identities.
The `ComparisonPairIdentity` SHALL include candidate identity, reference
identity, `SamplingPlan` identity, window identity, coverage/segment mapping,
alignment/comparison-domain identity, and relevant source/candidate revisions.
It SHALL not include a specific metric/provider/model. Each metric result SHALL
have a separate `MetricResultIdentity` containing that pair identity, metric
descriptor identity, logical metric, provider/provider version, invocation
contract, model identity when applicable, and runtime capability identity.
Evidence SHALL become stale at the smallest relevant level: source, plan,
window, transform/reference, or candidate generation/output changes invalidate
the affected pair and its metric results; metric/provider/model or capability
changes invalidate the affected metric results and containing set without
changing the material pair identity.

Cache and reuse SHALL respect identity granularity: candidate/global probes MAY
reuse a complete candidate identity; reference materialization SHALL be keyed
by the exact reference identity; comparison-pair/alignment work SHALL be keyed
by exact `pairIdentity`; metric results SHALL be keyed by pair identity plus
metric descriptor/provider/model/capability identity; and a complete evidence
set SHALL be keyed by its set contract plus canonical ordered pair/not-covered
ledger and requested metric/runtime contracts. No level SHALL use a path,
operation ID, or UI request as its sole key.

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

The initial logical metric set SHALL support VMAF, XPSNR, SSIM, optional
MS-SSIM, and diagnostic PSNR. Each logical metric SHALL resolve through an
explicit runtime provider contract containing the logical metric, provider,
provider version, invocation contract, model identity when applicable,
availability, and bounded reason. A provider MAY be a standalone FFmpeg
filter, a feature exposed by another filter/library such as libvmaf, or another
existing canonical runtime mechanism; the logical metric name SHALL NOT be
treated as its provider name. Runtime availability SHALL be discovered from the
actual MVForge FFmpeg runtime and any required model, not inferred from a
Docker image or a hardcoded build assumption. Capability provenance SHALL
include a stable capability identity/version and the resolved provider/model
contract. The `MetricResultIdentity` used for metric-result caching and
evidence hashing SHALL include the logical metric contract, actual provider,
provider/filter/library contract version, invocation contract, model identity
when applicable, its material `pairIdentity`, and runtime capability identity.
The evidence-set identity SHALL include the requested metric contracts and
runtime/provider/model contract identities in addition to its ordered
pair/not-covered ledger.

Each metric result SHALL distinguish at least `available`, `unsupported`,
`unavailable`, `failed_measurement`, and `non_comparable`; absent values SHALL
not be encoded as zero. A missing optional metric SHALL not make the complete
evidence set fail when other metrics can be measured. Capability discovery SHALL
occur once for the runtime/evidence operation and SHALL not be repeated once
per frozen window. Initial metric execution SHALL be sequential under existing
LAB heavy-work ownership unless one verified provider invocation safely returns
multiple logical metrics; no unconstrained one-process-per-metric/window
execution is permitted.

#### Scenario: VMAF is absent but other metrics are usable

- **WHEN** the runtime has no usable `libvmaf` filter/model but can execute
  XPSNR, SSIM, or PSNR for a comparable pair
- **THEN** VMAF SHALL be `unavailable` with capability provenance, and each
  other usable metric SHALL be measured and reported independently

#### Scenario: A logical metric is exposed through a verified provider

- **WHEN** the runtime exposes a verified provider/invocation contract for
  logical MS-SSIM even though no standalone filter named `ms_ssim` exists
- **THEN** MS-SSIM SHALL be `available` with that provider, provider version,
  invocation contract, and runtime capability identity, and SHALL not be
  rejected merely because the provider name differs from the logical metric

#### Scenario: The runtime does not expose a logical metric provider

- **WHEN** the runtime capability probe cannot verify a provider/invocation for
  MS-SSIM or cannot verify a usable VMAF model/provider
- **THEN** that logical metric SHALL be `unsupported` or `unavailable` with a
  bounded reason, SHALL not be invoked for every window, and SHALL not affect
  the other metrics

#### Scenario: A metric process fails after another metric succeeds

- **WHEN** one metric command or filter returns a measurement error for a
  comparable window while another metric has a valid result
- **THEN** the failed metric SHALL be `failed_measurement` with bounded
  diagnostics, the successful metric SHALL remain available, and the window or
  operation SHALL not convert the successful result to zero

#### Scenario: Provider identity changes

- **WHEN** a logical metric resolves to a different provider, provider
  invocation contract, provider version, or model identity
- **THEN** its `MetricResultIdentity` and the containing evidence-set identity
  SHALL change, the material `pairIdentity` SHALL remain stable, and a previous
  result SHALL not be reused as current evidence

### Requirement: Per-window evidence SHALL preserve metric semantics and provenance

Every window record SHALL include the frozen `SamplingWindow.Identity`,
start/duration, primary flag, inherited roles/reasons, coverage state,
reference identity, candidate identity, coverage/alignment/comparison-domain
identity, exactly one material `pairIdentity` when covered, and a bounded
result for each requested metric. A metric result SHALL retain its distinct
`MetricResultIdentity` plus direction/semantics (for example,
higher-is-better), logical metric contract, resolved provider/invocation
contract, model/version when applicable, units/range when defined, status,
canonical value when available, and a bounded reason or warning. VMAF, XPSNR,
SSIM, MS-SSIM, and PSNR SHALL remain distinct metrics and SHALL not be treated
as interchangeable.

The canonical serialized metric value SHALL be a tagged value, for example:

```text
MetricValue {
    kind: finite | positive_infinity
    value?: number
}
```

`kind: finite` SHALL include `value` as a bounded JSON number;
`kind: positive_infinity` SHALL omit a numeric value. Finite values SHALL
serialize as bounded JSON numbers. A valid positive-infinite
PSNR result SHALL serialize as `kind: positive_infinity` without emitting a raw
IEEE `+Inf` token. `NaN` SHALL never be a successful value; a parser/process
NaN SHALL become `failed_measurement` or `unavailable` with a bounded reason.
The tagged representation SHALL be used by API/frontend serialization and
canonical identity hashing.

#### Scenario: A window is non-comparable because geometry differs

- **WHEN** reference and candidate cannot be aligned in the canonical domain
  because required dimensions or transform semantics differ
- **THEN** each affected metric SHALL report `non_comparable` with the window's
  reference/candidate identities and reason, without fabricating a numeric
  result

#### Scenario: A metric has a valid infinite PSNR result

- **WHEN** the runtime reports a mathematically infinite diagnostic PSNR for an
  otherwise comparable pair
- **THEN** the contract SHALL preserve it as `MetricValue.kind:
  positive_infinity`, rather than silently treating it as unavailable or zero,
  and normal JSON encoding SHALL succeed

#### Scenario: A metric process returns NaN

- **WHEN** a metric parser receives `NaN` or another non-finite value that is
  not the explicitly supported positive-infinite PSNR result
- **THEN** the metric SHALL be `failed_measurement` or `unavailable` with a
  bounded reason, SHALL omit a successful `MetricValue`, and SHALL not enter
  numeric aggregates

#### Scenario: Metric values round-trip through JSON

- **WHEN** a finite metric value and a positive-infinite PSNR value are encoded,
  decoded, and hashed
- **THEN** finite values SHALL remain ordinary JSON numbers, the infinite value
  SHALL retain its tagged representation, no raw `Inf`/`NaN` token SHALL be
  emitted, and equivalent canonical representations SHALL produce the same
  identity

### Requirement: Aggregation SHALL be deterministic, bounded, and traceable

Per-window evidence SHALL remain canonical. For each metric, the bounded
aggregate SHALL retain total frozen-window count, candidate-covered-window
count, measured/comparable-window count, frozen-not-covered count,
non-comparable/unavailable/unsupported/failed counts, median/typical value,
lower-percentile evidence, worst measured value, the metric's semantics, and
the identity of the worst measured window. Uncovered, unavailable, unsupported,
failed, and non-comparable windows SHALL be excluded from numeric aggregates
and SHALL remain visible in their counts/reasons.

The aggregate policy SHALL be versioned and deterministic: comparable values
are ordered by metric value and then `SamplingWindow.Identity`; for
higher-is-better metrics, finite values sort before `positive_infinity`; median
uses the middle value for an odd count, the arithmetic mean when both middle
values are finite for an even count, and tagged `positive_infinity` when either
middle value is positive infinity; the initial lower-percentile evidence is p10
using nearest-rank selection; and worst means the lowest value for the initial
higher-is-better metrics. If all comparable PSNR values are positive infinity,
worst, median, and p10 are all the tagged positive-infinite value. A percentile
with fewer than the policy's minimum evidence count SHALL retain the
deterministic nearest-rank value while marking degraded percentile provenance.
No aggregate field SHALL itself imply pass/fail.

#### Scenario: Capability probing is reused across windows

- **WHEN** one evidence operation evaluates multiple frozen windows with the
  same runtime
- **THEN** each logical metric's provider capability SHALL be resolved once,
  reused by the windows, and included in evidence identity without spawning a
  new capability probe per window

#### Scenario: Unavailable windows are mixed with comparable windows

- **WHEN** two windows have valid SSIM values, one is unavailable, and one is
  non-comparable
- **THEN** the SSIM aggregate SHALL count two comparable windows, exclude the
  other two from median/p10/worst numeric values, and retain both exclusion
  counts and reasons

#### Scenario: Positive-infinite PSNR participates in aggregation

- **WHEN** comparable PSNR values contain finite values and one or more tagged
  `positive_infinity` values
- **THEN** positive infinity SHALL be ordered after finite values for the
  higher-is-better metric, SHALL remain valid in median/p10 when selected by
  nearest rank, and SHALL not become zero, unavailable, or a non-serializable
  raw number

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
Each Compatible Preview artifact SHALL represent only its corresponding frozen
window. Additional planned windows SHALL remain bounded and sequential under
existing LAB/Compatible Preview ownership. Perceptual analysis SHALL not create
a second operation state machine, a second selector, or a hidden recomputation
caused by tab changes or read-only rendering.

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

#### Scenario: LAB does not fill uncovered windows from a candidate

- **WHEN** a LAB operation has not materialized a ready artifact for a frozen
  additional window
- **THEN** that window SHALL remain traceable as not covered by the current
  candidate, and perceptual analysis SHALL not start a metrics-only encode to
  fill the gap

### Requirement: Test Encode and final Validation SHALL reuse the same contract

Test Encode SHALL expose the canonical `PerceptualEvidenceSet` when a compatible
encoded output artifact and frozen `SamplingPlan` exist, but SHALL measure only
the frozen windows proven by its `CandidateCoverage` segment mapping. A normal
Test Encode covers its frozen primary/segment, not every SamplingPlan window.
Final Validation SHALL consume the plan and evaluate the same
reference/candidate contract against the final output, retaining
expected/reference-to-actual provenance; it MAY cover all frozen windows only
when its full-output timeline identity proves that coverage. Neither surface
SHALL reinterpret or reselect scenes. The same source, plan, covered window,
reference, candidate, capability, and metric identities SHALL address the same
evidence identity across surfaces.

#### Scenario: Test Encode has a compatible output and frozen plan

- **WHEN** a completed Test Encode has a validated output identity and its
  frozen plan contains selected windows
- **THEN** Test Encode SHALL expose per-window and aggregate perceptual evidence
  only for the covered frozen window(s), SHALL retain other frozen windows as
  `not_covered`, and SHALL use the shared contract rather than a Test-Encode-
  specific metric schema

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

#### Scenario: Test Encode does not start another encode for uncovered windows

- **WHEN** a Test Encode covers only its primary window but the frozen plan has
  additional hard-scene windows
- **THEN** only the primary SHALL be measured, additional windows SHALL remain
  traceable as `not_covered`, and no selector or additional encode SHALL start

#### Scenario: Final Validation later covers the same frozen windows

- **WHEN** a final output has a proven full-output source-to-candidate timeline
  mapping for windows previously not covered by a Test Encode
- **THEN** final Validation SHALL measure those same frozen window identities
  using the shared contract rather than selecting replacement windows

### Requirement: Read-only API projections SHALL expose per-window ownership

The read-only API SHALL expose the `PerceptualEvidenceSet` identity, bounded
candidate/reference manifests, and ordered per-window entries sufficient to
answer which candidate artifact/output and reference produced each measured
window, which coverage mapping proved it, and which pair identity was used.
Manifest references MAY avoid repeating bounded provenance, but the API SHALL
make each identity resolvable without filesystem enumeration, UI order, or
operation history. A `not_covered` entry SHALL expose its frozen window
identity, coverage state, and bounded reason while omitting a fake
candidate/pair identity. Frontend consumers SHALL render these explicit
relationships and SHALL NOT reconstruct them heuristically from paths, names,
array position, or metric availability. Each returned metric result SHALL
expose its `MetricResultIdentity` or a resolvable bounded equivalent, so the
provider/model used for a value is inspectable without changing the material
pair relationship.

#### Scenario: API explains distinct LAB window artifacts

- **WHEN** three LAB windows use three Compatible Preview artifact generations
- **THEN** the read-only response SHALL expose the candidate/reference/pair
  identity for each window so a consumer can distinguish all three artifacts

#### Scenario: API explains an uncovered Test Encode window

- **WHEN** a Test Encode covers only the frozen primary window
- **THEN** the response SHALL expose the other frozen window identities as
  `not_covered` with their bounded reasons and SHALL not make the frontend
  infer a candidate pair for them

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
