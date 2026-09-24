## Why

HEVC QSV can turn fine grain and textured motion into a repeating sequence of
detail loss, temporary blur, and grain recovery. The effect is temporal, so a
single-frame score or a full-file quality scan cannot establish whether a
rate-control, GOP, B-frame, or supported QSV-control change caused it. MVForge
needs a bounded, evidence-backed LAB investigation that preserves the existing
source, preview, and encoder-plan authority.

## What Changes

- Add a `QSVTemporalExperiment` contract for deterministic baseline-versus-
  variant comparisons over a frozen source revision, SamplingPlan, and
  SamplingWindow.
- Consume only the #70 canonical `SamplingPlanForQSVPumping()` selection for
  `high_motion`, `high_texture_or_grain`, and `worst_predicted`; report missing
  roles instead of selecting replacement scenes or scanning the whole file.
- Preserve a user-selected LAB primary/repro window and reuse its ready
  Compatible Preview artifact as the experiment baseline without encoding it
  again.
- Define one-variable-at-a-time QSV comparisons for supported quality/rate
  control, GOP, B-frame, and already-supported capability-gated QSV controls.
  Capture requested and effective settings, runtime identity, filters, and
  bitstream/frame evidence for every candidate.
- Reject causal attribution when an unexpected effective setting changes; the
  experiment is recorded as `invalid/non-isolated`.
- Add a bounded temporal-evidence contract for adjacent-frame or short-segment
  detail/grain loss and recovery, with measured evidence kept separate from a
  versioned interpretation (`likely pumping`, `inconclusive`, or `not
  observed`).
- Attach optional canonical #69 `PerceptualEvidence` through an adapter when it
  exists, while continuing with deterministic existing evidence when it does
  not. #9 will not implement a second VMAF/SSIM pipeline.
- Reuse the existing LAB/Compatible Preview operation and artifact lifecycle:
  variants are explicit, sequential, cancellable, bounded, and never replace
  the baseline or Sample A/reference identity.
- Represent evidence-backed, versioned mitigation recommendations without
  silently changing existing Profiles.

The initial experiment is diagnostic and bounded. It does not automatically
generate a Cartesian product, tune all QSV controls, run expensive full-file
analysis, or promote one asset's result into a global profile default.

## Capabilities

### New Capabilities

- `quality`: Frozen, isolated QSV temporal experiments, bounded temporal
  evidence, optional perceptual-evidence consumption, and versioned mitigation
  recommendations.

### Modified Capabilities

- None. The #70 SamplingPlan, #3 Frame Fidelity, existing LAB/Compatible
  Preview lifecycle, and existing QSV capability resolution remain authorities;
  this change adds a consumer contract rather than changing their requirements.

## Impact

- Backend LAB orchestration and Compatible Preview adapters will gain a
  consumer for frozen #70 windows, experiment identity, candidate setting
  capture, temporal evidence, and isolation validation.
- Existing QSV capability/effective-plan and bitstream/frame-structure
  evidence will be reused; unsupported controls remain unavailable.
- LAB API/client types and UI will expose explicit variant execution and
  bounded progress through the existing operation lifecycle, with no hidden
  tab-triggered work.
- Focused unit tests will use injectable encoder, capability, artifact, and
  evidence seams. They will not require NAS media, network access, or QSV
  hardware.
- This SDD creates documentation only; production code, tests, Profiles, and
  media are unchanged in this phase.
