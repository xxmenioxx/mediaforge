## Why

Validated telecine sources currently resolve to a canonical
`fieldmatch=order=<resolved>,decimate,setfield=prog` chain, but frames that
remain combed after field matching have no downstream selective deinterlacer to
consume their interlaced marking. This can publish visibly combed output even
when cadence and field order were correctly resolved, so issue #2 needs a
bounded fallback that repairs only those frames without changing clean IVTC.

## What Changes

- Extend the canonical resolved telecine motion decision with an explicit,
  explainable residual-combing fallback state.
- Arm the configured `selective` fallback for every validated automatic
  inverse-telecine plan with resolved TFF/BFF, between `fieldmatch` and
  `decimate`, using the resolved field order; clean fieldmatched frames pass
  through it unchanged.
- Preserve the existing cadence decision, output FPS, frame-count behavior,
  progressive metadata, review-only behavior, and source provenance.
- Verify the behavior with deterministic synthetic mixed-telecine evidence and
  focused plan/output regressions for clean IVTC, residual combing,
  progressive, native interlaced, both field orders, and FPS integrity.
- Keep Preview, Profile Lab, Test Encode, Queue, and final worker planning on
  the same backend-resolved motion chain.

## Capabilities

### New Capabilities

- `residual-combing-telecine`: Defines selective residual-combing repair for
  the canonical validated telecine path, including motion-plan semantics,
  field-order handling, cadence/FPS invariants, output metadata, and parity
  across execution surfaces.

### Modified Capabilities

None.

## Impact

- Affects the backend cadence/interlace resolver, existing automatic motion
  filter rendering, effective video decision metadata, output validation, and
  focused Go regressions.
- Profile Lab, Compatible Preview, Test Encode, Queue freeze, and worker plan
  construction must consume the same resolved motion decision; no frontend
  policy or UI-specific filter builder is introduced.
- FFmpeg runtime capability checks and a small disposable synthetic fixture are
  required for regression evidence. No copyrighted media, NAS data, database
  migration, or production media is changed.

## Out of Scope

- Frame Fidelity redesign for issue #3.
- Asset Info persistence (#4), LAB orchestration redesign (#19), Profile
  redesign, UI deinterlacer controls, or a second filter-planning engine.
- Scene-by-scene mixed-cadence segmentation, AI/ML detection, arbitrary new
  deinterlacers, or changes to the existing cadence classification thresholds.
