## Why

Test Encode evaluation currently has a shared distributed `SamplingPlan`, but
its windows are position-based and do not express why a region is
representative or difficult. LAB Fidelity can therefore miss bounded,
evidence-backed hard scenes, while future perceptual quality, QSV pumping, and
Validation consumers could each grow an incompatible selector. Issue #70
establishes one backend-authoritative plan that reuses existing analysis,
preserves preview-first ownership, and makes hard-scene selection deterministic
and explainable on a resource-constrained NAS.

## What Changes

- Extend the existing backend `SamplingPlan`/`SamplingWindow` concept into a
  versioned canonical plan containing the asset/source fingerprint, evidence
  identities, policy identity, normalized window identities, roles/reasons, and
  bounded selection provenance.
- Add deterministic candidate discovery and bounded scoring for available
  motion, scene-change, texture/grain, dark/gradient, complexity, artifact, and
  existing Fidelity/reference evidence. Scores remain selection evidence, not
  absolute visual truth; roles may share a window and categories are optional.
- Preserve the existing distributed windows as the baseline/fallback, including
  deterministic ordering, duration normalization, overlap tolerance, and a
  bounded maximum window count.
- Make insufficient or stale evidence a non-fatal condition: fall back to the
  distributed baseline and record the bounded fallback/provenance reason.
- Freeze the plan when a LAB operation or Queue execution requires persisted
  selection, binding it to source revision, policy/version, evidence identity,
  selected windows, and role/reason summaries. Later analysis or policy changes
  must not reinterpret that frozen plan.
- Keep the user-selected LAB `Start` + `Seconds` interval as the primary
  preview/Fidelity window. Hard-scene windows complement it, reuse the primary
  artifact, and run additional Fidelity work sequentially and bounded.
- Define consumer contracts for future #69 perceptual quality and #9 QSV
  pumping so they consume the canonical windows and identities rather than
  selecting their own hard scenes.

Out of scope:

- No production implementation, database migration, analyzer rollout, #69
  perceptual-quality implementation, #9 QSV policy implementation, or new
  Validation verdict policy in this planning change.
- No random selection, full-file expensive decode by default, unbounded
  frame-level evidence persistence, or feature-specific selector.
- No replacement of Compatible Preview ownership, Frame Fidelity authority,
  existing cadence/frame-structure policy, restoration semantics, or current
  visual thresholds.
- No new duplicate analyzer when an existing canonical evidence source is
  semantically sufficient; evidence that does not exist remains unavailable.
- No PR, archive, commit, push, or production/NAS operation is included.

## Capabilities

### New Capabilities

- `quality`: Backend-authoritative deterministic hard-scene `SamplingPlan`
  resolution, bounded scoring, window identity, provenance, fallback, freeze
  semantics, and consumer contracts.

### Modified Capabilities

- None. The current repository has no synced quality capability spec. Existing
  LAB preview-first/Fidelity, frame-structure, cadence, restoration, and
  snapshot behavior are implementation dependencies and ownership boundaries;
  this change does not redefine their existing requirements.

## Impact

- Backend: extend `SamplingPlan`/`SamplingWindow` and the canonical resolver;
  adapt scanner snapshot evidence and LAB Fidelity operation snapshots; expose
  bounded plan summaries for future Queue, Validation, #69, and #9 consumers.
- Existing evidence: reuse source fingerprints and versioned snapshot
  components, distributed frame/GOP/cadence/interlace signals, restoration
  noise/grain/blocking/detail evidence, stream metadata/bitrate, packet/timing
  evidence when present, and the LAB primary/reference window.
- LAB: primary preview remains the selected request/artifact with exactly one
  primary encode; additional plan windows remain sequential and bounded.
- Persistence: store only bounded plan/candidate summaries and evidence
  references, never full frame datasets. Source revision or policy/evidence
  version changes invalidate reusable plans.
- Tests: deterministic plan fixtures, fallback/consolidation/tie-breaking,
  frozen identity, evidence reuse/call accounting, LAB ownership, sequential
  Fidelity, and #69/#9 consumer contracts.
