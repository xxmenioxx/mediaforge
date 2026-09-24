## Context

See `proposal.md` and `specs/quality/spec.md` for the problem and observable
contract. The current code already provides the relevant ownership boundaries:

- #70 owns the versioned `SamplingPlan`/`SamplingWindow` model and
  `SamplingPlanForQSVPumping()`, which returns selected windows plus unavailable
  roles without selecting replacements.
- LAB's existing `LabFidelityOperation` freezes source/material/plan identity,
  keeps the selected Compatible Preview primary first, processes additional
  windows sequentially, uses a bounded heavy-work slot, and supports
  cancellation.
- Compatible Preview owns request identity, effective profile/plan resolution,
  artifact generation, generation token/version, and artifact protection.
- Existing QSV resolution and frame/bitstream evidence distinguish requested
  from effective rate control/quality and expose capability-gated controls.
- Frame Fidelity and current LAB evidence are technical/effective-plan evidence,
  not a temporal pumping detector. No #69 perceptual evidence provider is
  assumed to exist on this branch.

The design therefore adds a consumer and experiment mode to existing LAB
orchestration. It does not create another scene selector, preview cache,
operation state machine, or quality-metric pipeline.

## Goals / Non-Goals

**Goals:**

- Make every comparison attributable to one frozen source/window and one
  declared effective variable.
- Reuse the ready primary artifact and preserve Sample A/reference and existing
  Compatible Preview lifecycle semantics.
- Produce bounded temporal evidence that can support cautious interpretation,
  while making visual inspection an explicit part of the result boundary.
- Capture requested/effective QSV settings, runtime capability identity, actual
  frame/bitstream evidence, and provenance for reproducible follow-up.
- Keep NAS heavy work sequential and offer a deterministic fake/injectable test
  seam.

**Non-Goals:**

- No #9-owned scene selection, worst-scene scan, full-file analysis, or
  Cartesian autotuning.
- No undocumented QSV arguments, new QSV capabilities, or decoder/playback
  compatibility fix.
- No universal pumping detector and no replacement for #69.
- No automatic mutation of existing Profiles or global QSV policy.
- No requirement for the Doctor Who/clouds repro or any real NAS/GPU media in
  unit tests.

## Decisions

### 1. Extend the existing LAB operation; keep generation in Compatible Preview

`LabFidelityOperation` remains the owner of experiment orchestration, progress,
cancellation, retention, and the existing queued/running/completed/failed/
canceled lifecycle. It gains an explicit QSV temporal experiment mode or
child-variant data within that operation; it does not get a parallel state
machine.

The operation calls the existing Compatible Preview request/artifact service
for a variant descriptor. Compatible Preview remains the owner of normalized
request identity, effective profile/encoder plan, capability resolution,
single-flight generation, artifact paths, generation identity, and artifact
protection. The experiment runner never launches raw FFmpeg and never decides
whether an encoder feature is supported.

Alternatives rejected:

- A separate QSV experiment worker would duplicate LAB lifecycle/cancellation
  and make preview identity ambiguous.
- Raw FFmpeg from #9 would bypass effective-plan capture and artifact safety.

### 2. Freeze #70 selection and preserve the explicit primary

At explicit experiment start, the backend resolves or accepts the already
frozen LAB plan and verifies source revision, plan identity, policy version, and
window identities. It then calls:

```text
selection = SamplingPlanForQSVPumping(frozenPlan)
```

The role selection is the only hard-scene input. The requested roles are fixed
in this policy version and are stored with the experiment. `selection.Windows`
is used as returned; `selection.UnavailableRoles` is copied to provenance and
the UI/API. There is no fallback to a newly ranked scene when a role is absent.

The explicit LAB primary is added as a controlled repro window from the frozen
operation material identity. If it is equivalent to a selected window under
the canonical window tolerance, the primary identity is retained and the role
metadata is joined. Otherwise it remains first and the role windows follow in
canonical plan order. A primary outside the role subset is therefore still
testable without becoming evidence that it is a hard scene.

The operation converts this ordered, deduplicated list into `targets[]`, with
one target per canonical SamplingWindow identity. Each target has one baseline
slot before it can have variants. The primary target points at the already-ready
primary Compatible Preview artifact. A non-primary target first looks up a
ready artifact for its exact interval/window identity; if none exists, the
existing LAB/Compatible Preview lifecycle materializes that target's baseline.
The temporal-evidence provider consumes those artifacts but never raw-encodes
or creates a second artifact lifecycle. A target for role window B can never
use the primary artifact as its baseline merely because both belong to one
experiment.

The experiment freezes the selected target list before any baseline or variant
request is created. Later scan results, policy changes, profile edits, or role
availability cannot change an in-flight experiment; they create a new material
identity instead.
Later scan results, policy changes, profile edits, or role availability cannot
change an in-flight experiment; they create a new material identity instead.

### 3. Use deterministic per-window target identities

The persisted/API experiment summary is conceptually:

```text
QSVTemporalExperiment {
    schemaVersion
    experimentId
    sourceRevision
    samplingPlanIdentity
    samplingPolicyVersion
    targets[]                       // bounded, deterministic order
    unavailableRoles[]
    experimentPolicyVersion
    bounds: { maxTargets, maxVariantsPerTarget,
              maxTotalVariantEncodes, maxControlCandidates,
              maxHeavyEncodes }
    interpretationPolicyVersion
    warnings[]
    provenance
}

QSVTemporalTarget {
    windowIdentity
    interval: { start, duration }
    roles[]
    reasons[]
    primary
    baseline: QSVExperimentCandidate
    variants: QSVTemporalVariant[]
    temporalEvidence[]
    perceptualEvidence[]            // optional #69 references
    controlCandidates[]             // optional, separately bounded
    classification
    recommendation?
}

QSVTemporalVariant {
    variantIdentity
    changedVariable
    requestedDelta
    effectiveDelta
    isolationResult
    candidate: QSVExperimentCandidate
    temporalEvidence[]
    perceptualEvidence[]            // optional #69 references
}

QSVExperimentCandidate {
    candidateIdentity
    sourceRevision
    samplingWindowIdentity
    requestId
    artifactId
    generationToken
    generationVersion
    canonicalEncodeSignature
    requestedQSVSettings
    effectiveQSVSettings
    effectiveProfile/plan
    filterPlanIdentity
    runtimeCapabilityIdentity
    frameStructure/HEVC evidence reference
    size, bitrate, speed summaries
}

EncoderControlCandidate {
    windowIdentity
    sourceRevision
    encoderFamily
    artifactIdentity
    generationIdentity
    effectivePlan
    filterPlanIdentity
    normalizationIdentity
    evidenceIdentity
}
```

Each target owns exactly one frozen SamplingWindow and exactly one matching
baseline candidate/artifact generation. The explicit primary target's baseline
is the already-ready primary artifact; a different role window has its own
baseline identity even when the same experiment contains both targets. Each
variant is nested under one target and compares only with that target's
baseline. Target-level temporal/#69 evidence and classification cannot be
mistaken for experiment-global evidence.

`candidateIdentity` is a versioned digest of source revision, window identity,
normalized interval, requested candidate, effective plan, filter identity,
encoder/runtime identity, and generation identity. `experimentId` is a digest
of the schema/policy, frozen selection, ordered target identities, every target
baseline/generation, bounded variant identities/deltas, and attached control
identities. Any target baseline or generation change therefore changes the
experiment identity. Display labels, result scores, and role names do not
substitute for these identities.

### 4. Define one-variable isolation over effective settings

Each variant under a target declares exactly one `changedVariable` and a
requested/effective delta relative to that target's baseline. Initial families
are:

| Family | One allowed comparison variable | Frozen context |
| --- | --- | --- |
| Quality/rate control | `qsv.global_quality` or effective QSV rate-control mode | all other quality, bitrate, GOP, BF, and feature settings |
| GOP | resolved GOP length, including source-relative/recommended mode | rate control, BF, adaptive controls, cadence, filters |
| B-frame strategy | effective `-bf`/maximum B depth or adaptive-B mode, one at a time | GOP, rate control, adaptive-I and other controls |
| Existing QSV controls | one supported look-ahead, ExtBRC, MBBRC, adaptive-I/B, or other canonical control | all other controls and filters |

The exact candidate uses the canonical existing profile/worker resolvers. A
requested difference is not enough: the comparison uses a canonical effective
settings map after capability resolution. The validator computes an effective
delta and allows only the declared variable plus a versioned, named derived
field (for example a resolved GOP frame count derived from the GOP choice).
Any additional delta, including an implicit rate-control or B-frame change,
marks that target/variant comparison `invalid/non-isolated` and prevents causal
attribution. An `EncoderControlCandidate` is deliberately not passed through
this validator because it answers encoder-specific attribution rather than
QSV mitigation causality.

The snapshot stores requested and effective values side by side, including
fallback/downgrade reasons. Actual QSV output evidence is separate from
requested intent: frame counts, I/P/B distribution, B-run length, HEVC
bitstream evidence, size, bitrate, and speed are recorded when available.
`rdo` or another existing QSV setting may be captured as context, but it is not
treated as causal evidence unless the runtime's canonical capability and
effective plan make that control meaningful and it is the declared variable.

Arbitrary undocumented flags are never candidates. A control unsupported by the
runtime is reported unavailable before heavy work begins.

### 5. Keep restoration and effective media plans frozen

Within each target, the baseline and every variant use the same source
revision, exact media interval, normalization, restoration filter chain,
scaling, audio/subtitle handling, effective video plan, and reference identity.
The filter/effective-plan identity is included in all candidates and compared
during isolation. Different targets intentionally have different intervals and
baseline artifact identities; they are not cross-window A/B pairs.

The initial experiment does not vary restoration filters. If a future explicit
filter experiment is added, its declared filter delta must use the same
isolation validator; filter presence alone can never explain pumping. This also
prevents attributing a GOP result when a profile resolver changed restoration,
quality, or frame structure at the same time.

### 6. Prove baseline reuse through artifact and generation identity

For the primary target, the operation records the existing ready primary
request as its baseline candidate and verifies, before evidence collection and
after heavy-work capacity is acquired, the same source revision, normalized
primary interval, effective plan/filter identity, artifact ID, and generation
token/version. The existing generation boundary/call-accounting seam must prove
zero new baseline generation attempts for this target; cache/request POST counts
alone are insufficient.

For every non-primary target, the operation performs the same identity check
against an exact-interval ready artifact. A matching artifact is reused; a
cache miss requests baseline materialization through the existing Compatible
Preview lifecycle, subject to the complete experiment cap. That baseline is
owned by the role target and cannot be substituted with the primary artifact.

If any target baseline is stale, the target returns a bounded retryable stale
result and never compares a new generation with its frozen baseline. A variant
always uses a distinct request/artifact/candidate identity nested under its
target, even when it happens to resolve to an equivalent command; no variant
overwrites any target baseline.

### 7. Run a bounded sequential variant workflow

The existing LAB operation follows this sequence:

```text
frozen #70 selection -> bounded targets[]
    -> target A: reuse/materialize baseline -> evidence
    -> target A: variant 1 -> evidence -> isolation
    -> target A: variant 2 -> evidence -> isolation
    -> target B: reuse/materialize baseline -> evidence
    -> target B: variant 1 -> evidence -> isolation
    -> ... -> target classifications/recommendations
```

`QSVTemporalExperimentPolicyV1` makes the complete bound deterministic before
any request is created:

| Bound | Initial maximum | Meaning |
| --- | ---: | --- |
| Target windows | 3 | Explicit primary plus at most two role windows, in canonical order |
| Variants per target | 3 | Direct baseline-relative QSV variants under one target |
| Total QSV variant encodes | 6 | Hard experiment-wide cap, selected in target/variant order |
| Optional control candidates | 3 | At most one explicitly requested non-QSV control per target |
| Baseline materializations | 3 | At most one baseline request per target; ready artifacts reuse |
| Total heavy encodes | 12 | 3 baseline + 6 QSV variants + 3 controls; ready reuse lowers actual work |

The policy version and all bounds are part of the experiment identity. A request
for three variants on every target is therefore truncated/rejected after the
first six QSV variants according to deterministic target/variant order; it is
not an implicit unbounded matrix. A control candidate is counted separately
and is created only when the existing LAB workflow explicitly requests that
bounded control work. The control is never a QSV mitigation variant.

The existing process-local LAB heavy-work slot permits at most one heavy
operation at a time. Baselines, QSV variants, and explicit controls run
sequentially; the QSV evidence provider itself performs no encode. The runner
checks all caps before creating requests and retains only bounded per-target
candidate summaries, temporal observations, setting snapshots, control
references, and evidence references. Raw frames, complete FFmpeg logs, and
unlimited metric histories are not copied into the experiment.

The chosen variant is never silently promoted. A later operation can use it as
baseline only after an explicit user/policy action that creates a new frozen
experiment identity.

### 8. Define temporal evidence as a bounded measurement contract

The temporal provider is an injectable adapter over the existing artifact/evidence
boundary. For each target/variant pair it consumes that target's baseline
artifact, variant artifact, same source timeline, and deterministic short
segment, then returns evidence nested under the target/variant relationship. It
never compares artifacts from different target windows. The contract is
conceptually:

```text
TemporalEvidence {
    schemaVersion
    targetWindowIdentity
    variantIdentity
    sourceRevision
    baselineArtifactIdentity
    variantArtifactIdentity
    sampleStart, sampleDuration, frameCount
    normalizationIdentity
    region/mask identity
    frameObservations[]             // bounded adjacent-frame summaries
    detailEnergyByFrame
    grainEnergyByFrame
    blurOrHighFrequencyProxyByFrame
    lossRecoveryEvents[]
    temporalVariationSummary
    availability
    evidenceReferences[]
}
```

The first provider implementation may use deterministic high-frequency/detail
energy, local variance/edge proxies, and fixed textured-region masks over
adjacent frames or a short segment. It must compare a sequence, not one frame,
and must expose the actual frame span and normalization. The measurements are
stored independently from interpretation.

Interpretation is a versioned policy with conservative labels. It can label a
candidate pair `likely pumping` only when comparable frame coverage shows a
loss/recovery or localized instability pattern with the policy's confidence
requirements. Missing coverage, ambiguous changes, or provider errors yield
`inconclusive`; adequate coverage without the pattern yields `not observed`.
The policy is explicitly not a universal detector and must leave manual visual
inspection as the final confirmation for texture, grain character, and scene
context. The UI/API shall show measured evidence and interpretation separately.

### 9. Attach optional #69 evidence through an adapter

The experiment accepts an optional canonical perceptual-evidence provider keyed
by source revision, SamplingPlan identity, target window identity, candidate
artifact generation, and evidence version. If matching #69 evidence exists,
the target stores a bounded reference/summary and records its provenance. It
does not select windows, run its own VMAF/SSIM, or make #69 a prerequisite.
Evidence for one target cannot be attached to another target merely because
the source and plan match.

If #69 is absent, stale, incompatible, or unavailable, the operation continues
with temporal evidence plus existing Frame Fidelity, effective-setting,
bitstream/frame-structure, size, bitrate, and speed evidence. Adapter failure
is a warning unless the user explicitly asked for a required external evidence
source, which is outside this initial scope.

### 10. Keep the non-QSV control contract separate from mitigation variants

`EncoderControlCandidate` answers whether a measured temporal pattern is
encoder-specific; it is not another QSV quality/GOP/BF variant. A control may
reuse an existing compatible x265/Test Encode/LAB artifact, but only after
proving the same source revision, exact target window identity/interval,
cadence/reference normalization, crop/scale/restoration/filter intent, and
comparable quality/reference domain. The control records its encoder family,
effective plan, artifact/generation identity, filter/normalization identity, and
evidence identity. Encoder-specific and rate-control mechanics may differ and
are recorded as control context; they are not passed through the QSV effective
delta validator.

The initial workflow does not create an x265/control encode merely to earn a
label. A control encode is allowed only when the existing LAB workflow
explicitly requests it within the `maxControlCandidates` and `maxHeavyEncodes`
bounds. Without a valid control, the target can still emit a mitigation
recommendation from isolated QSV baseline-to-variant evidence, but its
classification cannot exceed `measurable_not_clearly_qsv_specific`.

This separation is intentional: QSV baseline-to-variant evidence answers
mitigation causality, while QSV candidate-to-control evidence answers
encoder-specific attribution. A visual statement that x265 looks better is
manual context, not canonical control evidence.

### 11. Classify QSV specificity and recommendations conservatively

The result classifier runs per target. It first rejects a non-isolated QSV
baseline-to-variant comparison, then considers target temporal evidence and
technical evidence. The four public outcomes are:

1. `reproducible_qsv_specific_pumping`: sufficient QSV temporal evidence,
   isolated/reproducible QSV evidence, matching target/window/reference/control
   identities, and a non-QSV/control result where the same temporal pattern is
   absent or materially reduced under the versioned interpretation policy, with
   no structural Frame Fidelity mismatch invalidating comparability.
2. `measurable_not_clearly_qsv_specific`: temporal change is measured, but no
   valid matching control proves QSV-specific causality. This is the required
   ceiling when no control exists; manual visual preference is not a control.
3. `inconclusive`: evidence is unavailable, non-comparable, or non-isolated.
4. `no_pumping_in_tested_windows`: bounded windows had sufficient evidence and
   did not show the policy pattern.

The QSV baseline-to-variant comparison answers mitigation causality. The QSV
candidate-to-control comparison answers encoder-specific attribution. The
control is not a tuning variant and its different encoder/rate-control
mechanics are not sent through the QSV isolation validator. Decoder/device
compatibility findings are reported as a separate technical concern.

An optional `QSVPumpingMitigationRecommendation` is owned by a target and is a
versioned summary of a chosen effective QSV delta, evidence references,
applicable encoder/runtime, media characteristics (for example texture/grain
and motion roles), confidence, and size/speed/compression trade-offs. An
isolated QSV mitigation recommendation MAY be emitted even when encoder-specific
attribution is unavailable. The recommendation is exposed as an advisor/draft
input. It does not call profile mutation or change existing profiles; explicit
application creates a new/draft profile or an explicit edit.

### 12. Use explicit LAB actions and preserve existing UI/reference behavior

The existing `Process Video` action continues to own primary preview readiness.
The QSV experiment starts only from an explicit user action after the primary
is ready. Existing progress polling, cancellation, retry, and stale-run checks
are reused. A tab change or render cannot start an experiment. The API/client
adds only the bounded experiment request, status, evidence summary, and
recommendation fields needed by this mode.

Sample A remains source-backed and the existing reference preview remains the
canonical reference. Sample B remains the ready primary artifact for the
primary target; each additional target displays its own baseline and variants,
all separately labeled and compared only within that target. No giant
fetch-to-Blob transport, new media transport, or duplicate primary baseline
registration is part of this design.

### 13. Make all causal and temporal behavior fixture-testable

The implementation will inject seams for frozen-plan input, effective-plan/
capability resolution, Compatible Preview generation/accounting, artifact
identity, temporal evidence, optional #69 evidence, and clock/cancellation where
needed. Unit fixtures cover deterministic windows and adjacent-frame summaries;
they do not read NAS paths, contact the network, invoke a real GPU, or require
the Doctor Who repro. A focused integration seam may count generation attempts
and artifact protections without encoding media.

## Risks / Trade-offs

- [Risk] Existing QSV fallback behavior changes a second effective variable
  behind the requested value. → Compare canonical effective snapshots, record
  fallback reasons, and return `invalid/non-isolated` rather than a causal
  claim.
- [Risk] Temporal proxies mistake normal scene texture changes for pumping. →
  Keep measurements separate from versioned interpretation, require adjacent
  sequence evidence, retain confidence/warnings, and require visual inspection
  for final confirmation.
- [Risk] Per-target variants could multiply across hard-scene windows and
  overload the NAS. → Use the versioned three-target/six-total-variant/twelve-
  heavy-encode cap, check it before request creation, and serialize all heavy
  work.
- [Risk] A primary artifact is accidentally compared with a different role
  window. → Require one baseline identity per target and reject cross-window
  temporal/evidence joins.
- [Risk] No matching non-QSV control exists on the same target. → Keep the
  target measurable for QSV mitigation while capping attribution at
  `measurable_not_clearly_qsv_specific`.
- [Risk] Additional QSV previews compete with Jellyfin/Immich on the NAS. →
  serialize heavy work, reuse the ready baseline, cap retained evidence, and
  make cancellation release existing artifact protection.
- [Risk] A #69 provider has incompatible identity or version. → Attach it only
  when source/plan/window/generation identities match and continue with existing
  deterministic evidence otherwise.
- [Risk] Users interpret a non-QSV playback warning as temporal quality loss. →
  expose decoder compatibility and pumping classifications as separate fields.
- [Risk] A recommendation becomes a silent global policy. → Store it as a
  versioned, evidence-linked suggestion and require explicit profile/draft
  application.

## Migration Plan

No database or media migration is required for the SDD. Implementation should
be additive: preserve existing LAB operation payloads and normal preview/Fidelity
runs, make new experiment fields optional, and gate the new mode behind an
explicit action. A rollback disables the experiment entry point and ignores
its bounded operation records; existing Compatible Preview artifacts, source
media, Profiles, and historical Fidelity data remain untouched.

## Open Questions

None that change the selected contract or ownership. The first temporal proxy's
calibration thresholds are implementation-level policy values and must be
versioned with deterministic fixtures before they can support interpretation.
