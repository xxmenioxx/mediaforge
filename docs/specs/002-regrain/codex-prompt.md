# Codex Prompt — Regrain

Implement:

```text
docs/specs/002-regrain/spec.md
```

Use:

```text
docs/specs/002-regrain/plan.md
docs/specs/002-regrain/tasks.md
```

Treat `spec.md` as the behavioral source of truth.

Execute `tasks.md` in order and keep the implementation minimal.

Key constraints:

```text
- Regrain is a canonical Restoration stage.
- Order: denoise -> upscale -> SAR -> final sharpen -> regrain -> field metadata.
- Presets:
  light  -> noise=c0s=1:c0f=t
  medium -> noise=c0s=2:c0f=t
  strong -> noise=c0s=3:c0f=t
- Regrain must be geometry-neutral for Smart Upscale.
- Preview, Test Encode, Queue, and Worker must share the frozen resolved plan.
- Advisor may recommend Regrain only when the spec evidence/context gates pass.
- Current ambiguous bitplanenoise evidence must not be given invented thresholds.
- Applying a recommendation must patch only Regrain fields.
- Preserve unknown Advanced Filter barrier behavior.
```

Expected primary files:

```text
backend/internal/handlers/media_command.go
backend/internal/handlers/restoration_plan.go
backend/internal/handlers/restoration_recommendations.go
backend/internal/handlers/upscale_domain.go

frontend/src/utils/restorationFilters.ts
frontend/src/components/RestorationControls.tsx
```

`backend/internal/handlers/restoration_analysis.go` should not change unless a concrete spec-required issue proves it necessary.

Add only focused tests required by `tasks.md`.

Validate with:

```bash
./scripts/verify.sh --auto
```

At completion report only:

```text
Changed:
- files/symbols

Validation:
- ./scripts/verify.sh --auto
- PASS/FAIL

Limitations:
- only material limitations
```
