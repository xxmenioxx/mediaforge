# Tasks — Regrain

**Depends on:** `spec.md`, `plan.md`

## Task 1 — Canonical backend stage

- Add `restorationStageRegrain`.
- Map canonical `noise` to Regrain.
- Place it after final sharpen and before field metadata.
- Preserve unknown Advanced Filters as stable barriers.
- Add focused ordering/recognition tests.

**Acceptance**

```text
hqdn3d -> scale -> setsar -> cas -> noise
```

## Task 2 — Frontend semantic Regrain

- Add `regrain` config: `off | light | medium | strong | custom`.
- Add preset rendering:
  - Light → `noise=c0s=1:c0f=t`
  - Medium → `noise=c0s=2:c0f=t`
  - Strong → `noise=c0s=3:c0f=t`
- Add Custom controls from the spec.
- Add `noise` to controlled Restoration filters.
- Avoid duplicate canonical `noise`.
- Keep missing/legacy config = Off.

## Task 3 — Smart Upscale + frozen execution

- Verify Regrain is geometry-neutral.
- Verify Smart Upscale resolves normally with Regrain enabled.
- Verify `ResolvedRestorationPlan` exposes stage `regrain`.
- Preserve Preview/Test Encode/Queue/Worker parity.
- Preserve Video Codec `copy` re-encode warning behavior.

## Task 4 — Advisor recommendation

Add:

```text
id = regrain
domain = Regrain
```

Rules:

```text
available + confidence >= medium + severity low/medium/high
+ denoise active/recommended
+ Smart Upscale applied/recommended
    -> Light / Medium / Strong

ambiguous / unavailable / low confidence / unknown severity
    -> manual_review or no_recommendation
    -> no patch
```

- Patch only Regrain fields.
- Respect existing apply-lock behavior.
- Do not calibrate `bitplanenoise` thresholds in this task.

## Task 5 — Regression coverage

Cover:

```text
Off/default compatibility
preset exact strings
custom deterministic rendering
canonical order
Smart Upscale geometry
unknown Advanced Filter behavior
frozen Queue behavior
Video Codec copy behavior
Advisor gates and patch scope
```

## Task 6 — Validate

Run only:

```bash
./scripts/verify.sh --auto
```

If a check fails, follow the existing `AGENTS.md` failure workflow.

**Done:** spec satisfied and `ALL CHECKS PASSED`.
