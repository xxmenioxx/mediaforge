## Why

MVForge already freezes bounded `SamplingPlan` windows and LAB can compare one
decoded source frame with a ready Compatible Preview artifact, but the evidence
has no canonical contract for source/reference transforms, multi-window metrics,
capabilities, identity, or reuse by Test Encode and final Validation. Issue #69
defines that missing contract now that #70 is authoritative for window selection.

## What Changes

- Add a versioned, bounded `PerceptualEvidenceSet` containing source/reference,
  candidate/output, frozen plan/window identities, runtime capability and metric
  provenance, per-window results, deterministic aggregates, warnings, and
  unavailable capabilities.
- Make candidate coverage explicit so a Compatible Preview or Test Encode
  reports which frozen windows it physically represents; uncovered windows stay
  traceable and are not treated as metric failures or filled by new encodes.
- Extend the existing LAB Fidelity evidence boundary so perceptual analysis
  consumes ready Compatible Preview/Test Encode/final-output artifacts and
  never encodes a sample solely to measure it.
- Define one canonical reference adapter from the resolved/frozen video plan
  and existing preview normalization, including transform identity/version,
  frame/time alignment, source revision, and non-comparable behavior.
- Support logical VMAF, XPSNR, SSIM, optional MS-SSIM, and diagnostic PSNR via
  explicit runtime provider contracts, without assuming a same-named FFmpeg
  filter, optional library, or model is installed.
- Make #70 frozen windows the only evaluation windows for LAB, Test Encode, and
  final Validation; consumers may filter them but cannot select replacements.
- Replace canonical quality interpretation with evidence-first semantics. The
  contract will not add an arbitrary universal VMAF/SSIM pass threshold; the
  existing LAB visual verdict remains a compatibility projection until a future
  policy such as #53 owns interpretation.
- Define bounded, identity-sensitive caching, cancellation, sequential work,
  and deterministic tests for source replacement, artifact generation
  replacement, optional capability absence, and aggregate traceability.

Out of scope: a new scene selector/analyzer, full-file sampling by default,
duplicate encode or general media cache, new cadence/crop/scale/color policy,
universal pass/fail thresholds, a complete artifact-analyzer suite, #9 QSV
pumping policy, and #53 quality-gate decisions.

## Capabilities

### New Capabilities

- `quality`: canonical perceptual evidence and source-to-output comparison over
  frozen SamplingPlan windows, shared by LAB, Test Encode, and Validation.

### Modified Capabilities

None. The #70 `quality` delta remains the authoritative producer of
`SamplingPlan`; this change adds the consumer contract in a new change-local
capability because no main `openspec/specs/quality/spec.md` exists yet.

## Impact

- Backend handler boundaries around `lab_fidelity_operation.go`, the existing
  Compatible Preview artifact/evidence cache, `validation_fidelity.go`, Test
  Encode completion, `sampling_plan.go`, and runtime FFmpeg capability probes.
- A shared internal/API DTO and bounded process/persistence identity for
  evidence, with adapters for Queue snapshots and final Validation reports.
- LAB read-only presentation and operation progress, while preserving the
  existing preview-first artifact lifecycle and native media behavior.
- Focused Go/frontend contract tests using fake FFmpeg capability/process
  seams, temporary fixtures, and no NAS, network, optional-libvmaf host, or
  production media.
