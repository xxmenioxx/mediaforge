## Context

See [proposal.md](proposal.md) for the problem and [the capability spec](specs/lab-guided-cadence-resolution/spec.md) for observable behavior. The current preview chain in `assets.go` calls `resolveEffectiveVideoMotionProfile`, `resolveAutomaticFrameStructure`, then effective encoding/HEVC resolution. `resolveAutomaticFrameStructure` intentionally errors when Auto GOP requires FPS while `effectiveOutputFrameRateUnknown` is true. It must not silently substitute source FPS.

Canonical Fidelity is evaluated separately by `evaluateCanonicalFidelity` through `labEvaluateCanonicalFidelity` against the effective frozen preview profile. It reports required fields for width, height, frame rate, field structure, pixel format, bit depth, and applicable HEVC level/tier; SAR/DAR and chroma location are evaluated when applicable. `UNVERIFIED` records unresolved authority/value or missing actual evidence and is distinct from mismatch. Cadence Preserve can supply source frame-rate expectations when source probe evidence exists, but it does not itself make `effectiveOutputProgressive` reliable or set `fieldStructureMode=preserve`. Thus a profile can preserve cadence while Fidelity's field-structure expectation remains unresolved. If the operator explicitly preserves field structure, Fidelity uses source structure, which still depends on observable source/output evidence.

## Goals / Non-Goals

**Goals:**

- Convert only the cadence/GOP planning failure into a versioned diagnostic with legacy error compatibility.
- Offer bounded actions from existing evidence and reuse canonical plan/evaluation paths.
- Keep per-asset draft state and test planning aligned with Queue's frozen effective decision.

**Non-Goals:**

- A new cadence/FPS/GOP/Fidelity resolver, automatic field-order selection, or synthetic PASS.
- Automatic Deep Cadence execution, arbitrary FPS overrides, global Profile/Asset Default changes, Queue mutation, enqueue, or publish.
- Database migration or changed fidelity thresholds.

## Decisions

1. **Keep planning diagnosis at the LAB backend boundary.** Wrap the specific unresolved-effective-FPS Auto GOP planning failure in a typed resolution diagnostic while retaining status, `error`, and existing error details. Do not reinterpret unrelated Preview, filesystem, encoder, HEVC, or operation failures as resolution suggestions. Include a version and explicit per-field blockers (value, authority state, reason), requested cadence/GOP, any known effective FPS/progressive status, and source scan/profile/source revision identity.

2. **Evaluate alternatives by applying them to cloned asset-scoped plans.** A small fixed candidate budget (at most three planning candidates, excluding a separately requested TFF/BFF comparison) bounds work. Each candidate re-enters the same `resolvePreviewVideoProfile`/effective-motion/GOP/encoding path. `RESOLVED` means the planning pipeline produced a complete effective decision; it does not mean Fidelity passed. Candidate failures remain `NEEDS_REVIEW` or `UNAVAILABLE` with reasons. No candidate invokes FFmpeg during recommendation generation.

3. **Order actions by evidence strength.** Reuse validated cadence first and let normal Auto GOP/HEVC resolution run. If evidence is insufficient, expose the existing Deep Cadence snapshot operation only when its source/snapshot checks say eligible; it is opt-in and bounded by existing operation limits. Offer preserve only when source FPS, required field structure/geometry, and absence of conflicting FPS transforms are supported. Keep explicit TFF/BFF short preview/Test Encode comparisons behind an operator request when field-order evidence is ambiguous. A fixed/custom GOP cannot mask unknown FPS.

4. **Bind accepted plans to evidence revisions.** Recommendation/apply input carries the current source revision and scan/profile identity. Apply and Test alternative revalidate these before resolving. Store the selection only in the LAB draft used for that asset. Reuse compatible Preview/Test Encode and effective video decision/fingerprint fields; Queue remains governed by its existing preflight and immutable freeze. Changes after freeze do not change that execution.

5. **Make Fidelity's evidence authoritative in the UI.** The panel renders backend field status, authority state, and reason. It does not infer confidence from a selected Preserve mode. Planning status and sample verdict are distinct; only existing canonical aggregation and metric/coverage policy can show Safe.

## Risks / Trade-offs

- [A recommendation could become stale after analysis or source edits] → Bind it to exact source and scan revisions and reject stale apply/test requests.
- [Bounded candidate planning could still duplicate expensive work] → Candidate evaluation performs metadata resolution only; reuse scan/deep-cadence evidence and avoid FFmpeg until an explicit test action.
- [Preserve may retain visible telecine/combing] → Present it as an alternative with those trade-offs and leave final Fidelity to its canonical policy.
- [Clients depend on existing error shape] → Add the versioned envelope alongside existing fields and keep current status semantics.

## Migration Plan

No persisted schema changes are planned. Rollback is removal of the additive diagnostic and LAB controls; existing error responses, saved profiles, and frozen Queue snapshots remain usable.

## Open Questions

None. Exact eligibility continues to be decided by existing backend evidence and authority rules.
