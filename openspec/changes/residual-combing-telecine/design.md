## Context

See `proposal.md` for the motivation and `specs/residual-combing-telecine/spec.md`
for the observable contract. This audit found that the repository already has
the required canonical analysis and planning boundary, but the validated IVTC
chain stops before it consumes residual-combing evidence.

The current flow is:

```text
ffprobe/frame evidence
  -> InterlaceAnalysis and IVTCValidation
  -> CadenceAnalysis/CadenceRecommendation
  -> resolveEffectiveVideoMotionProfile
  -> fieldmatch=order=<resolved>,decimate,setfield=prog
  -> effective output FPS/progressive metadata
  -> FFmpeg MediaJobPlan and validation
```

Relevant current behavior:

- `validateIVTCContext` evaluates both TFF and BFF using fieldmatch,
  decimate, and `idet`; `applyIVTCValidation` accepts a selected order only
  when the distributed windows agree.
- `ensureCanonicalIVTCFilter` removes stale IVTC pieces and reconstructs the
  existing `fieldmatch`/`decimate`/`setfield` prefix. It currently has no
  residual-combing stage.
- `profileWithAutomaticDeinterlace` intentionally does not prefix ordinary
  deinterlacing to IVTC. Native interlaced input uses the existing `bwdif`
  helper with `deint=all` and analysis-derived parity.
- `resolveEffectiveVideoMotionProfile` is used by Preview, final media-plan
  resolution, and Queue snapshot freezing. Profile Lab also has a local
  automatic-motion helper that consumes `scan.interlaceAnalysis.automaticFilter`
  and appends progressive metadata; this is a parity risk for the new state.
- `effectiveOutputFrameRate` is already resolved before rendering and
  `validateCadenceOutputFrameRate` already prefers measured frame count/duration,
  then `avg_frame_rate`, with `r_frame_rate` as diagnostic corroboration.
- No cadence/interlace media fixtures were found in the repository. Existing
  coverage is primarily resolver, command-plan, and probe-fixture based.

The local FFmpeg runtime is 8.1.1. Its filter capabilities expose
`fieldmatch` comb matching, `bwdif=deint=interlaced`, frame-preserving
`bwdif=mode=send_frame`, and `decimate`. The current FFmpeg documentation
explicitly describes the fieldmatch output marking remaining combed frames as
interlaced and shows a selective deinterlacer between fieldmatch and decimate.

## Goals / Non-Goals

**Goals:**

- Make residual-combing repair part of the existing canonical resolved motion
  decision, not a second filter-planning engine.
- Preserve clean fieldmatched frames, resolved parity, decimation cadence,
  effective FPS, progressive output signaling, and Queue immutability.
- Make the selected fallback and its rationale inspectable in the existing
  effective execution decision so future Frame Fidelity work can understand
  the intentional transformation.
- Prove behavior with a deterministic disposable synthetic fixture plus focused
  plan, validation, and cross-surface regressions.

**Non-Goals:**

- No new cadence classifier, scene-by-scene cadence segmentation, AI/ML
  detection, or changes to existing telecine validation thresholds.
- No general mixed-content engine. Unvalidated or ambiguous input remains
  review-only.
- No changes to issue #3 Frame Fidelity, issue #4 persistence, issue #19 LAB
  orchestration, Profile design, or user-configurable deinterlacer controls.
- No new FFmpeg dependency and no production/NAS media mutation during
  development or testing.

## Decisions

### 1. Extend the existing effective motion decision

The implementation will extend the existing resolved worker decision rather
than create a second plan object or make the command builder classify media.
The requested `cadenceFieldOrder` remains distinct from effective output.
The existing effective fields remain authoritative and will be supplemented by
the smallest effective motion fields needed by this issue:

- resolved effective IVTC field order (`tff` or `bff` when accepted);
- residual-combing fallback state (`none` or selective deinterlace);
- a bounded reason/diagnostic identifying that fieldmatch residual markers are
  the reason for the fallback.

The same values will be represented in the existing `effectiveVideoDecision`
diagnostic surface and the frozen Queue worker snapshot. The final
`videoFilters` value remains the executable rendering authority. No new API or
persistence migration is required; old snapshots default the new fallback
state to `none` until a fresh canonical resolution occurs.

### 2. Use selective deinterlacing between fieldmatch and decimate

For a validated IVTC plan, the selected conceptual chain is:

```text
fieldmatch=order=<resolved>:combmatch=full,
bwdif=mode=send_frame:parity=<resolved>:deint=interlaced,
decimate,
setfield=prog
```

The exact serialized form remains subject to the existing filter-chain
canonicalization rules. `combmatch=full` is selected for the fallback plan so
fieldmatch evaluates combing beyond only scene changes and can mark residual
frames consistently. The implementation must verify that this option is
supported by the runtime capability used by MVForge; if it is not supported,
the safe result is the existing review/unsupported state rather than silently
using a less selective chain.

`bwdif` is selected over `yadif` because MVForge already uses `bwdif` as its
canonical native-interlaced deinterlacer and already has parity resolution and
command coverage for it. Only the `deint=interlaced` mode is new to the IVTC
path; native interlaced processing retains `deint=all`.

Alternatives rejected:

- **Full-frame bwdif after fieldmatch:** rejected because it would unnecessarily
  soften or interpolate clean reconstructed frames and could reduce vertical
  detail.
- **Yadif selective fallback:** technically supported and shown by FFmpeg's
  documentation, but rejected as a second canonical deinterlacer without a
  repository quality policy or existing parity helper.
- **Deinterlace before fieldmatch:** rejected because it destroys the field
  relationships that IVTC needs to reconstruct progressive frames.
- **Deinterlace after decimate:** rejected because decimate would make duplicate
  and cadence decisions using still-combed frames, and the fallback would no
  longer be part of the documented fieldmatch/decimate strategy.

### 3. Treat fieldmatch metadata as the selective input

The fallback will not run a new cadence analysis and will not inspect pixels in
the command renderer. FFmpeg fieldmatch supplies the frame-level interlaced
marking for frames it could not reconstruct; `bwdif=deint=interlaced` consumes
that marking and passes progressive frames through unchanged. `mode=send_frame`
keeps the fallback at one output frame per input frame.

Focused runtime evidence must use `showinfo`/equivalent frame metadata after
fieldmatch and after bwdif to prove that:

1. a clean matched frame is not marked for deinterlacing;
2. an injected residual combed frame is marked and is repaired; and
3. bwdif does not change the count before decimate.

If the supported worker FFmpeg build does not preserve this metadata contract,
the implementation must not approximate it with an all-frame deinterlacer. It
must retain the current review/fallback semantics and report the capability
limitation.

### 4. Reuse the canonical resolved field order for both filters

The resolver will select parity once using the existing validated IVTC
precedence. An explicit valid `cadenceFieldOrder` wins; otherwise the accepted
IVTC validation order is used; otherwise the plan is unresolved. The selected
value is passed to both `fieldmatch` and selective `bwdif`.

The fallback must not call the generic native-interlace parity helper for an
IVTC plan, because that helper can fall back to generic IDET counts or `auto`
and could disagree with the validated IVTC order. TFF and BFF therefore render
the same chain shape with only the resolved parity value changed. Missing or
conflicting parity preserves the existing warning/review behavior and cannot
produce a guessed chain.

### 5. Preserve frame count, timestamps, FPS, and progressive metadata

The fallback is intentionally placed before `decimate`. `bwdif` in
`send_frame`/selective mode does not create a second field-rate stream or drop
frames. `decimate` remains the only cadence frame-drop stage, so the existing
validated 5-to-4 IVTC behavior and `24000/1001` target remain authoritative.
No additional `fps` or `setpts` operation is introduced.

The existing progressive metadata stage remains at the end of the canonical
motion prefix. `setfield=prog` is retained or generated once after decimation;
the effective progressive state is derived from the resolved chain, not from a
UI label. Existing `-fps_mode cfr` output handling and cadence validation
continue to consume the resolved effective FPS.

### 6. Enforce Preview/LAB/Test Encode/Queue parity at the resolver boundary

Preview, final `MediaJobPlan`, Test Encode, and Queue freeze continue to call
the same backend motion resolver with the same frozen analysis. Queue retains
the result in its immutable snapshot before processing.

Profile Lab's local automatic-motion composition is a known parity boundary.
The implementation must make LAB consume the backend-resolved motion decision
or the exact canonical motion filter returned by that decision. Its local
restoration-only filter builder may continue to compose user restoration
filters, but it must not independently infer residual combing, field order, or
progressive metadata from `automaticFilter`.

The parity regression compares, for identical frozen inputs, the effective
operation, field order, fallback state, rendered motion prefix, progressive
state, and output FPS. Transport-only preview differences are outside the
comparison.

### 7. Keep detection, resolution, rendering, and validation separate

The implementation boundary remains:

```text
analysis and IVTC validation
  -> resolved effective motion decision
  -> FFmpeg filter argument rendering
  -> output probe and cadence validation
```

Analysis may record the evidence that a fallback-capable telecine plan is
allowed, but filter rendering must consume that resolved decision rather than
classifying frames or reselecting parity. Output validation must compare the
actual output with the frozen effective FPS and progressive expectation; it
must not infer a new cadence policy from output artifacts.

No issue #3 fidelity special case is added. The new effective motion fields
only expose the intentional transformation that #3 will later consume.

### 8. Reproduce the affected behavior without copyrighted media

The repository contains no committed cadence/interlace media fixture. The
regression strategy will generate a disposable deterministic fixture with the
installed FFmpeg runtime:

- generate a progressive moving test pattern at a stable source rate;
- produce a clean telecine segment and a short controlled true-interlaced or
  cadence-breaking segment with both top-first and bottom-first variants;
- run fieldmatch with and without the selective fallback, observing frame flags
  after fieldmatch and after bwdif;
- run decimate and probe frame count, duration, average FPS, base FPS, and
  progressive field metadata.

The focused automated tests will assert the rendered plan and use the small
synthetic runtime fixture to assert frame behavior where FFmpeg is available.
The existing unit tests remain independent of GPU, NAS, copyrighted media,
and production files; an environment capability check may skip only the
runtime-media portion with an explicit reason, never convert a failed filter
capability into a passing acceptance result.

Manual validation for a real user-owned sample is separate: run the same
resolved plan through Preview and Test Encode, compare clean and residual
sections visually, inspect `showinfo`/`idet` and `ffprobe` output, and confirm
Queue uses the frozen chain. The sample is not committed or copied to NAS.

## Risks / Trade-offs

- **Risk:** `combmatch=full` increases fieldmatch CPU cost on a resource-
  constrained NAS. → Measure the synthetic and user-owned sample, avoid a
  second analysis pass, and enable it only for validated IVTC plans.
- **Risk:** A false interlaced marker could invoke selective interpolation on a
  frame that was visually acceptable. → Keep `deint=interlaced`, preserve clean
  IVTC regressions, record the fallback state, and retain review-only behavior
  when validation is incomplete.
- **Risk:** Old FFmpeg builds may not support the required option or metadata
  handoff. → Probe the actual worker runtime and fail conservatively rather than
  silently selecting full-frame deinterlacing.
- **Risk:** Profile Lab can drift from backend motion resolution because it has
  a local automatic-motion helper. → Add explicit cross-surface parity tests
  and make the backend decision the only authority.
- **Risk:** Existing snapshots do not contain the new diagnostic fields. → Use
  optional fields with a `none` default and do not require a database migration.

## Migration Plan

1. Add runtime capability/reproduction evidence and focused unit tests before
   changing the canonical chain.
2. Extend the backend resolved motion decision and renderer, preserving native
   interlaced and progressive paths.
3. Route LAB automatic-motion consumption through the resolved decision and add
   Preview/Test Encode/Queue parity coverage.
4. Add output FPS, frame-count, field-order, and progressive metadata
   assertions for clean and residual-combing fixtures.
5. Run `./scripts/verify.sh --auto` and the strict OpenSpec validation.

No media, database, or snapshot migration is required. Rollback is a code
revert to the previous validated `fieldmatch`/`decimate` chain; existing source
and published outputs are unaffected.

## Open Questions

- After the synthetic and user-owned sample runs, confirm whether
  `combmatch=full` is required for every validated IVTC plan or whether the
  existing fieldmatch defaults reliably mark the affected class. This is a
  bounded runtime-tuning question; it does not change the selected placement,
  selective semantics, parity source, or cadence invariants.
