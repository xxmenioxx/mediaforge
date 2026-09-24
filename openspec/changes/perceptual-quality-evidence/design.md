## Context

This design extends the current main after #70 Hard-scene Sampling. The
existing `SamplingPlan`/`SamplingWindow` contract in
`backend/internal/handlers/sampling_plan.go` already carries bounded plan and
window identities, source revision, roles/reasons, fallback provenance, and a
read-only `SamplingPlanForPerceptualQuality` consumer adapter. That adapter is
the only window input for this change.

The current Frame Fidelity evaluator in
`backend/internal/handlers/validation_fidelity.go` already separates raw source,
canonical source, frozen expected output, and actual output. Its effective
cadence, field, geometry, pixel, color, and HEVC authorities come from the
resolved/frozen plan rather than UI text. It is the authority to extend for a
physical comparison reference; #69 must not duplicate cadence, crop, scale,
SAR/DAR, field, or color policy.

LAB currently owns the bounded operation, cancellation, one-slot heavy-work
guard, primary Compatible Preview generation identity, sequential additional
windows, and the process-local evidence caches in
`backend/internal/handlers/lab_fidelity_operation.go`. Compatible Preview
descriptors and artifacts already expose source revision, canonical encode
signature, artifact ID, generation token/version, effective preview profile,
resolved video decision, and preview normalization. The current metrics path in
`assets.go` runs one decoded-frame SSIM and PSNR comparison and has no VMAF,
XPSNR, or MS-SSIM capability contract.

Queue PREPARING freezes the effective profile and `samplingPlan` in the Queue
profile snapshot. Test Encode persists effective configuration and a validation
report. Final Validation persists its report and calls the shared Frame Fidelity
evaluator. Runtime snapshots are immutable and already carry encoder capability
evidence, while `capabilities.CheckEncoder` and `runtimeinfo.Latest` establish
the existing runtime-probe and snapshot-reuse pattern.

The resource boundary is a NAS, so #69 must evaluate bounded windows from
already encoded artifacts. It may use an FFmpeg filter graph or bounded decoded
frames to form a logical reference, but it must never create a lossy sample
encode solely because a metrics request was made.

## Goals / Non-Goals

**Goals:**

- Define one versioned, bounded `PerceptualEvidenceSet` with exact identity and
  provenance for source/reference-to-candidate comparison.
- Make the frozen #70 `SamplingPlan` and `SamplingWindow.Identity` the only
  window authority for LAB, Test Encode, and final Validation.
- Reuse the #3 Frame Fidelity authority and #19/#70 LAB/Compatible Preview
  lifecycle while adding a thin reference adapter and metric provider.
- Detect actual FFmpeg metric capabilities once per runtime/evidence operation,
  record their identity, and isolate optional metric failures.
- Make per-window evidence canonical and aggregates deterministic without
  introducing a quality threshold or universal verdict.
- Keep work bounded, cancelable, cacheable by exact identity, and testable with
  injected capability and process seams.

**Non-Goals:**

- No new hard-scene selector, scene analyzer, worst-scene search, or full-file
  metric mode by default.
- No second cadence/crop/scale/color/Frame Fidelity resolver and no inference
  from FFmpeg filter text when a frozen effective authority exists.
- No duplicate encode, no general media cache, and no replacement of the
  Compatible Preview artifact lifecycle.
- No implementation of all blocking/blur/banding/ringing/temporal analyzers,
  #9 QSV pumping policy, or #53 quality-gate thresholds and pass/fail policy.

## Decisions

### 1. Canonical reference transformation is owned by the frozen Frame Fidelity authority

The existing `fidelityAuthority`, `fidelityExpectedOutput`, and
`frozenFidelityExecutionPlan` boundary remains the owner of what the effective
video plan means. `evaluateCanonicalFidelity` continues to own structural
source/expected/actual semantics and mismatch precedence. The new physical
reference adapter will consume that authority plus the frozen
`previewNormalizationDecision` produced by `buildPreviewNormalization`; it
will not resolve cadence, crop, scale, field order, SAR/DAR, or color policy a
second time.

The adapter will be a narrow shared boundary, proposed as
`canonicalPerceptualReference` beside the existing LAB evidence provider. Its
output is a logical reference descriptor/filter plan, not a lossy encoded media
file:

```text
CanonicalReference {
    identity
    sourceRevision
    sourceStreamIdentity
    samplingPlanIdentity
    windowIdentity
    transformIdentity
    transformVersion
    alignmentIdentity
    normalizationProvenance
    sourceTimeline
    comparisonDomain
    status, reason
}
```

The adapter translates only already resolved/frozen fields into a bounded FFmpeg
comparison graph. Cadence/IVTC, deinterlace, effective FPS, and progressive
authority come from the frozen video decision consumed by Frame Fidelity;
geometry/aspect comes from its resolved restoration/upscale/effective geometry;
color comes from the existing color policy/preview normalization evidence. If a
required field is not authoritative, the adapter returns an explicit unresolved
reference rather than guessing from current settings or raw command text.

An alternative was to compare raw source and candidate and let each metric
apply its own resize/colorspace/cadence options. That would make unlike metrics
and consumers disagree about intentional transforms, so it is rejected. A
second general transform resolver is also rejected because it would violate #3
and #19 authority.

### 2. Source time is mapped through an explicit frozen alignment contract

The source `SamplingWindow.StartSeconds` and `DurationSeconds` remain in the
canonical source timeline selected by #70. The reference adapter records a
versioned `alignmentContract` with:

```text
source timeline/window
    -> cadence/field transform mapping
    -> canonical reference frames/PTS
    -> candidate output timeline/PTS
```

For unchanged cadence, source and reference time are equivalent subject to the
recorded timestamp tolerance. For IVTC/FPS transforms, the mapping is derived
from the frozen `effectiveCadenceOperation`, `effectiveOutputFrameRate`, and
`effectiveOutputProgressive` evidence already consumed by Frame Fidelity. For
deinterlace, crop, scale, SAR/DAR, and color-only transforms, time is preserved
but the comparison domain changes. For Test Encode segments, the frozen segment
start is subtracted only at the candidate container boundary; it is not treated
as source time zero.

The provider will compare aligned decoded frames in the canonical domain using
the exact bounded window. It will record source/candidate frame counts, PTS
range, dropped/duplicated/unmatched frame counts, and tolerance in bounded
provenance. Frame-index pairing is allowed only after the alignment adapter has
proven that the timelines are equivalent. If the mapping is missing or
ambiguous, the window is `non_comparable`; no nearest raw frame is silently
chosen.

An alternative was to use `-ss` plus equal frame counts for every window. That
is acceptable only for the already normalized one-frame legacy path and cannot
represent IVTC, trimmed Test Encode timestamps, or final-output alignment, so
it is retained only as an adapter optimization when the contract proves it
equivalent.

### 3. The comparable pair identity is exact and generation-fenced

The proposed internal model is bounded and serializable:

```text
PerceptualEvidenceSet {
    schemaVersion: "perceptual-evidence/v1"
    policyVersion: "perceptual-evidence-policy/v1"
    evidenceIdentity
    samplingPlan: { schemaVersion, policyVersion, planIdentity }
    reference: ReferenceIdentity
    candidate: CandidateIdentity
    coverage: CandidateCoverage
    sourceRevision
    runtime: { capabilityIdentity, ffmpegVersion, metrics }
    metricContracts: MetricDescriptor[]
    windows: PerceptualWindowEvidence[]
    aggregates: MetricAggregate[]
    artifactEvidence: ArtifactEvidenceSet
    warnings
    unavailableCapabilities
    provenance
}

CandidateCoverage {
    sourceStart
    sourceDuration
    coveredWindowIdentities[]
    segmentMappingIdentity
}
```

`ReferenceIdentity` contains source revision, source stream, plan/window
identity, transform/version, alignment version, comparison-domain
normalization, and source-reference materialization revision when one exists.
`CandidateIdentity` contains candidate kind (`compatible_preview`,
`test_encode`, or `final_output`), artifact/output identity, canonical path
identity, output revision, generation token/version where available, frozen
execution identity, and segment mapping. `CandidateCoverage` is derived from
that segment mapping and explicitly names the frozen window identities
physically represented by the candidate. A Compatible Preview normally covers
one corresponding window; a Test Encode normally covers its frozen primary
segment; a final output can cover all windows only after full-output timeline
proof. The pair identity hashes canonical serialized versions of these fields,
including coverage, plus metric/model and runtime capability identities. It
does not use a path alone.

The model distinguishes these invalidation causes: source replacement, plan or
window replacement, frozen transform/reference replacement, candidate artifact
generation/output replacement, metric/model contract change, and capability
contract change. A stale result remains diagnosable but is not served as
current evidence.

Identity is deliberately separate from a display or operation ID. A retry may
have a new operation ID while reusing the same evidence identity; a new
generation at the same path necessarily creates a new identity.

### 4. Metric execution extends the existing LAB Fidelity evidence provider

Metric execution will live beside, and be called by, the existing LAB
Fidelity runner rather than a new metrics state machine. The implementation
will factor the current boundaries as follows:

- `labCollectPreviewEvidence` remains the bounded source/output probe and
  frame-structure provider, extended to accept a canonical reference identity
  and candidate identity.
- `labGeneratePreviewFrameMetrics` becomes the legacy SSIM/PSNR adapter over
  the shared provider rather than a second implementation.
- `runPreviewFrameMetric` becomes an injectable bounded metric process seam;
  VMAF, XPSNR, MS-SSIM, SSIM, and PSNR adapters use the same cancellation,
  stderr limits, alignment, and result decoding boundary.
- `collectLabFidelityWindow` attaches the canonical per-window evidence after
  the artifact is ready and the existing artifact-use protection is held.
- `labFidelityOperationResult` publishes the bounded evidence set/aggregate
  beside the existing size, bitrate, encoder, speed, and Frame Fidelity fields.

The provider creates a `CandidateCoverage` entry before metric execution. A
Compatible Preview descriptor covers only the frozen window represented by its
own source interval and generation. A LAB result can therefore contain the
full frozen plan manifest while marking windows without a ready corresponding
artifact as `not_covered`; it does not read a primary artifact for an unrelated
window.

For additional #70 windows, the existing LAB/Compatible Preview operation may
materialize a planned preview artifact as part of its preview-first workflow,
subject to its bounded sequential ownership. The perceptual provider itself
accepts a ready artifact handle and SHALL never issue an encode request. A
metrics-only consumer with no candidate artifact reports unavailable or waits
for the owning preview/Test Encode/Queue lifecycle; it does not create a
replacement encode.

The alternative of a new `QualityService` with independent lifecycle,
selection, leases, and cache is rejected because it would duplicate
`labFidelityOperation.go`, #19 artifact ownership, and #70 plan consumption.

### 5. Existing artifact lifecycle and evidence cache are reused

Before reading a Compatible Preview artifact, the provider uses the existing
`acquireCompatiblePreviewArtifact` protection and releases it on success,
failure, or cancellation. It never removes or cancels a shared ready artifact.
`compatiblePreviewDescriptor` remains the source of canonical encode signature,
source revision, normalization, effective preview plan, artifact ID, and
generation token/version. The durable artifact record and its cleanup rules
remain the retention authority.

The current process-local LAB evidence cache is extended, not replaced:

```text
source probe cache       -> source path + source revision + stream identity
artifact evidence cache  -> artifact ID + generation token/version
reference cache          -> reference identity + alignment identity
metric result cache      -> pair identity + metric/model/capability identity
```

The old `labPreviewEvidenceCacheKey` is therefore expanded or wrapped so a
source path, artifact path, or request ID alone can never hit a perceptual
result. The source-global probe may be shared across windows when its source
revision matches; interval/reference/metric evidence always includes window,
normalization, and transform identities. These are bounded entries under the
existing LAB retention/max-item discipline, not a second general media cache.

The ready primary preview is joined by artifact/generation identity. A primary
request or playback tab cannot cause a second generation merely because
perceptual evidence also consumes it.

### 6. Final Validation obtains the same contract from the frozen Queue snapshot

Queue PREPARING already stores `samplingPlan` and `samplingPlanIdentity` in the
immutable `ProfileSnapshot`; #69 will read it through the existing
`samplingPlanFromQueueJob` boundary and `SamplingPlanForPerceptualQuality`.
`validateQueueJob` will pass the frozen plan, source probe, final-output probe,
verified execution identity, and frozen effective authority into the shared
provider. It will not ask the current scanner, current Profile, or current
Asset Override to select or reinterpret windows.

The source/reference side uses the same `canonicalPerceptualReference` adapter
as LAB. The candidate side uses a `final_output` identity tied to the verified
Queue execution artifact and its output revision. Existing
`validateJobFrameFidelity` remains the structural authority and receives the
same frozen authority; the perceptual evidence is attached as a separate
report section. Validation may mark all frozen windows as covered only after
the final-output timeline mapping proves that the complete output represents
each source interval; otherwise it records the uncovered windows explicitly.
Uncovered windows do not become metric failures, and optional metric absence
does not turn the evidence or output into a validation failure.

For older Queue jobs, the existing legacy plan adapter may expose frozen
distributed windows when it can prove their identity. If no plan/source/output
identity is sufficient, Validation keeps current structural behavior and
records perceptual evidence as unavailable; it never performs a new selection.

### 7. Test Encode uses the same candidate/reference/evidence contract

At Test Encode effective-plan freeze, the operation will freeze or reference a
bounded `SamplingPlan` using the source revision and the resolved effective
video authority. On successful completion, `executeTestEncode` will publish a
candidate identity containing the Test Encode ID, output revision, configuration
hash, effective execution identity, and segment mapping. The existing
`TestEncode.ValidationReport` is the persistence boundary for the bounded
evidence set; it is not a new metric schema.

Test Encode evidence requests accept the completed artifact/output identity and
the frozen plan plus an explicit `CandidateCoverage`. They reuse the same
reference adapter, capability snapshot, metric descriptors, per-window result
model, and aggregation code used by LAB and Validation. A normal Test Encode
candidate exposes only its frozen primary/segment window as covered; other
frozen windows remain traceable as `not_covered` and are not read from the
segment. If the Test Encode output is not compatible with its covered
window/reference contract, it reports `non_comparable` or unavailable rather
than silently measuring a different interval. No evidence request starts an
additional encode to create coverage.

The same logical source and candidate with identical identities may reuse LAB
or Validation evidence only when all identity fields match. Surface labels and
operation IDs do not affect the evidence identity.

### 8. FFmpeg metric capabilities are detected from the actual runtime once

The existing `backend/internal/capabilities` package will gain a metric
capability snapshot adjacent to encoder capability evidence. A runtime probe
will inspect the actual FFmpeg executable used by MVForge for logical metrics
and their providers:

- FFmpeg executable/version/build identity;
- verified provider/invocation availability and help/contract signature for
  VMAF, XPSNR, SSIM, MS-SSIM, and PSNR. A provider may be a standalone FFmpeg
  filter, a feature exposed through another filter/library such as libvmaf, or
  another existing canonical runtime mechanism;
- actual VMAF provider and usable model identity/version, or a bounded
  model/provider-unavailable reason;
- supported input/format constraints needed by the canonical comparison domain.

The capability record is explicitly logical-to-provider rather than
metric-to-filter:

```text
MetricCapability {
    metric                  // logical VMAF/XPSNR/SSIM/MS-SSIM/PSNR
    provider
    providerVersion
    invocationContract
    modelIdentity?
    availability
    reason?
}
```

MS-SSIM is `available` only when a provider and invocation have been verified;
the absence of a standalone `ms_ssim` filter is not itself decisive. VMAF is
available only when the provider and required model identity are usable in the
actual runtime.

The probe is injectable in tests and cached once per runtime snapshot/evidence
operation, never once per window. The immutable runtime snapshot pattern from
`models.RuntimeSnapshot`/`runtimeinfo.Latest` is reused. A small separate
`MetricCapabilities` JSON field is preferable to overloading encoder fields;
legacy snapshots without it remain readable and can use an explicit live probe
or report unknown capability. The runtime identity is a digest of executable
version, filter help/contract signatures, model identity, and capability schema
version, not a Docker image name.

Metric support is resolved before window execution. Unsupported or unavailable
metrics are recorded once in `unavailableCapabilities` and per-window metric
results inherit that status without repeatedly spawning capability probes.
Available metric processes use `exec.CommandContext`-style cancellation and
argument arrays; all output/error parsing is bounded and injectable.

Metric processes are bounded separately from capability probing. The initial
implementation runs metric work sequentially under the existing LAB heavy-work
ownership: one active metric invocation per owned window, with no unconstrained
FFmpeg process per metric/window. A single verified invocation may collect more
than one logical metric only when its provider contract and parser prove the
identity of every returned metric. Any future parallelism requires an explicit
versioned resource policy and capacity test; it cannot be introduced by a UI
loop or a per-window goroutine.

An alternative was to assume `libvmaf` because the development container has
it, or to make all metrics mandatory. Both fail on constrained NAS runtimes and
would make optional absence an application failure, so they are rejected.

### 9. Metric semantics and deterministic aggregation are separate layers

Metric descriptors are immutable contract data, not UI labels:

```text
MetricDescriptor {
    key: vmaf | xpsnr | ssim | ms_ssim | psnr
    direction: higher_is_better
    unit/range
    provider
    providerVersion
    invocationContract
    modelIdentity/version
    capabilityIdentity
}

MetricResult {
    status: available | unsupported | unavailable |
            failed_measurement | non_comparable
    value: MetricValue
    reason
    metric descriptor identity
}

MetricValue {
    kind: finite | positive_infinity
    value?: number
}
```

The initial metrics are higher-is-better, but direction remains explicit so a
future metric cannot be compared by name alone. The provider records a numeric
value only for `available`. `kind: finite` requires a normal bounded JSON
number, while `kind: positive_infinity` omits a numeric value. Raw IEEE `+Inf`
and `NaN` never cross the JSON or identity boundary. A parser NaN is
`failed_measurement` or `unavailable`, not a successful metric. The canonical
set has no threshold/verdict field.

For each metric, aggregation filters to `available` values from candidate-
covered windows only. It sorts finite values ascending and places tagged
`positive_infinity` after finite values for higher-is-better metrics; ties are
broken by `SamplingWindow.Identity`. Median is the middle value for an odd
count, the arithmetic mean only when both middle values are finite for an even
count, and the tagged positive-infinite value when either middle value is
positive infinity. The policy-versioned lower percentile is p10 using nearest
rank `ceil(0.10*n)`, clamped to the available count. `worst` is the first
sorted value for higher-is-better metrics; direction generalizes that choice
later. If fewer than three comparable windows support p10, the value is still
the deterministic nearest-rank value but the aggregate marks
`lowerPercentileDegraded` and records the requested minimum. If all comparable
PSNR values are positive infinity, worst, median, and p10 are tagged positive
infinity. There is no zero padding and no aggregate pass/fail.

The aggregate retains total frozen-window count, candidate-covered-window
count, measured/comparable count, not-covered count, unavailable, unsupported,
failed, and non-comparable counts, median, p10, worst value, metric-specific
status, degraded provenance, and `worstWindowIdentity`. Ties cannot depend on
map order, filesystem order, or goroutine completion.

The existing LAB `visualAggregate` and its `metricVerdict` threshold are kept
as a compatibility projection for old clients until a separately versioned
policy replaces it. The new `PerceptualEvidenceSet` and Validation SHALL NOT
derive a universal quality verdict from that legacy field or introduce a new
threshold. #53 owns future advisory/gating interpretation.

### 10. Persisted/recomputed boundaries and deferred concerns are explicit

Persisted bounded evidence:

- Queue final Validation persists the `PerceptualEvidenceSet` summary in the
  existing `QueueJob.ValidationReport` boundary, alongside Frame Fidelity.
- Test Encode persists the same bounded set in `TestEncode.ValidationReport`.
- LAB returns it in the bounded operation snapshot/result and reuses valid
  process-local evidence through the existing LAB cache; a process restart may
  recompute evidence from the ready artifact without re-encoding it.
- Persisted coverage includes total frozen, candidate-covered, measured, and
  not-covered window identities/counts, so a partial Test Encode result cannot
  be mistaken for a six-window result.
- Queue/Test Encode snapshots persist the frozen SamplingPlan and candidate
  execution identity required to decide whether a stored set is still current.

Never persisted: full decoded frames, unlimited packet/frame arrays, full
FFmpeg stderr, or an unbounded list of candidate scenes. Frame previews and
diagnostic logs remain read-only bounded consumers of existing artifact paths.

The structured artifact-evidence extension is a separate map with the same
window/reference/candidate identity and statuses. Initial #69 defines the
boundary and adapters only; it reuses an existing deterministic signal only
when source/transform/window semantics match. Full blocking, blur/detail,
banding, ringing/halo, and temporal-instability analyzers are deferred.

Future #53 Quality Gate owns threshold and policy interpretation. Future #9
QSV pumping may consume `SamplingPlanForQSVPumping` and this evidence but may
not select windows. Future #53/#9 policy versions are included in evidence
identity when they interpret evidence, but #69 collection remains neutral.

## Risks / Trade-offs

- **[Risk]** The current Frame Fidelity authority may not expose enough physical
  transform information for a specific source. → Add the smallest frozen
  `ReferenceTransform`/alignment adapter and report unresolved reference
  evidence; do not infer a second cadence or color policy.
- **[Risk]** FFmpeg builds expose different filter names, models, or numeric
  output formats. → Capability-probe the actual runtime, version metric/filter
  contracts, inject the process boundary, and isolate metric status.
- **[Risk]** A metric provider could accidentally make an additional preview
  encode look like metric work. → Require a ready candidate artifact handle at
  the metric boundary and count encode starts separately from metric starts.
- **[Risk]** A same-path source/output replacement could hit a process cache. →
  Include source/output revisions and generation token/version in every key and
  revalidate them before returning a cached set.
- **[Risk]** Per-window FFmpeg work can exceed NAS capacity. → Consume the #70
  hard maximum, keep LAB additional work sequential under `labFidelityHeavySlot`,
  avoid full-file defaults, bound stderr/provenance, and honor cancellation.
- **[Risk]** Users may read high metrics as proof that a structural transform is
  correct. → Keep Frame Fidelity as a separate canonical structural result and
  preserve both results; do not add a #69 threshold or verdict.
- **[Risk]** Older snapshots lack a frozen plan or metric capability. → Use
  explicit legacy/unavailable states, keep old fields readable, and avoid
  migration that modifies source media or production reports.

## Migration Plan

1. Add the bounded evidence model, canonical reference/alignment adapter, and
   injectable metric/capability seams without changing existing metric or
   Frame Fidelity output.
2. Extend the runtime snapshot capability boundary and the existing LAB
   evidence cache/artifact generation keys. Add identity validation before
   serving cached evidence.
3. Attach the provider to LAB's existing operation after ready-artifact
   acquisition, preserving primary reuse, sequential windows, cancellation,
   partial results, and legacy visual fields.
4. Add Test Encode completion and final Validation adapters that consume their
   frozen plan/candidate identity and persist bounded evidence reports.
5. Add read-only API/frontend projections beside current LAB Fidelity tabs;
   tab rendering must consume snapshots and never start work.
6. Enable or disable optional metric adapters by actual runtime capability. A
   rollback can omit the new evidence section and retain legacy SSIM/PSNR and
   Frame Fidelity fields; no source media, ready artifact, or old report is
   deleted.

## Open Questions

None that change the approved contract. The exact FFmpeg filter argument syntax
and model packaging are implementation details constrained by the runtime
capability probe and the reference/alignment contract above; they must not
change identity semantics or add a new selection authority.
