# Implementation Plan — Regrain

**Depends on:** `spec.md`

## 1. Approach

Extend the existing canonical Restoration pipeline. Do not create a parallel FFmpeg path.

```text
Profile/Lab
-> videoFilters
-> resolveRestorationPlan
-> canonicalizeRestorationFilterChain
-> Smart Upscale
-> ResolvedRestorationPlan
-> Preview / Test Encode / Queue / Worker
```

Regrain is semantic, optional, and `Off` by default.

WorkerConfig:

```text
regrain: off | light | medium | strong | custom
regrainLumaStrength
regrainChromaStrength
regrainTemporal
regrainDistribution: gaussian | uniform
```

Presets:

```text
light  -> noise=c0s=1:c0f=t
medium -> noise=c0s=2:c0f=t
strong -> noise=c0s=3:c0f=t
```

Canonical order:

```text
denoise -> upscale -> SAR -> final sharpen -> regrain -> field metadata
```

## 2. Files

### Backend

`backend/internal/handlers/media_command.go`
- add `restorationStageRegrain`;
- map canonical `noise` to Regrain;
- keep unknown Advanced Filters as stable barriers.

`backend/internal/handlers/restoration_plan.go`
- expose stage name `regrain`;
- preserve frozen-plan semantics.

`backend/internal/handlers/restoration_recommendations.go`
- add `regrain` recommendation;
- use existing recommendation states/apply-lock;
- patch only Regrain fields.

`backend/internal/handlers/restoration_analysis.go`
- no calibration change in this feature unless separately approved.

`backend/internal/handlers/upscale_domain.go`
- expected tests only; `noise` should become geometry-neutral through the known Restoration stage.

### Frontend

`frontend/src/utils/restorationFilters.ts`
- add Regrain to controlled Restoration filters;
- render presets/custom deterministically;
- avoid duplicate canonical `noise`.

`frontend/src/components/RestorationControls.tsx`
- add `Off / Fine · Light / Fine · Medium / Fine · Strong / Custom`;
- show custom controls only for Custom.

## 3. Recommendation rule

Add:

```text
id = regrain
domain = Regrain
```

Actionable only when:

```text
Grain availability = available
confidence >= medium
severity = low | medium | high
AND denoise active/recommended
AND Smart Upscale applied/recommended
```

Mapping:

```text
low    -> Light
medium -> Medium
high   -> Strong
```

Current `bitplanenoise` evidence is ambiguous. Do not invent thresholds. Until calibrated evidence exists, return `manual_review` or `no_recommendation` with no patch.

## 4. Tests

Cover only:

```text
stage/order
preset rendering
Off/default compatibility
Smart Upscale geometry with Regrain
unknown Advanced Filter behavior
Preview/Test Encode/Queue frozen parity
Video Codec copy behavior
recommendation evidence/context gates
recommendation patch scope
```

## 5. Validation

```bash
./scripts/verify.sh --auto
```

Done when the spec is satisfied and verification passes.
