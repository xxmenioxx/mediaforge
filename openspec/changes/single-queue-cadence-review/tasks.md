## 1. Backend Queue review

- [x] 1.1 Classify canonical unresolved automatic IVTC as `requires_review` in one-item Batch Preflight and expose missing-order guidance plus frozen cadence/HEVC review evidence.
- [x] 1.2 Apply candidate-scoped TFF/BFF or Preserve choices through the existing conversion override and canonical freeze; verify IVTC filters/FPS/HEVC and fail closed when Preserve has unknown FPS without changing stored profile configuration.
- [x] 1.3 Gate direct single Queue Create before persistence on unresolved IVTC and require reviewed Preflight for explicit cadence edits; verify zero QueueJobs, ExecutionPlans, and reservations for ambiguous evidence.
- [x] 1.4 Verify candidate acceptance revalidates source identity/revision and keeps the accepted profile snapshot and ExecutionPlan aligned.

## 2. Frontend review flow

- [x] 2.1 Show TFF/BFF field-order controls for asset-scoped auto cadence when scan evidence recommends IVTC; keep profile-wide controls unchanged.
- [x] 2.2 Add candidate-scoped Auto/TFF/BFF/Preserve choices, expose cadence evidence, and show the effective plan/visual-review guidance before acceptance.
- [x] 2.3 Route supported individual Queue UI actions through one-item Batch Preflight; verify the folder Queue action uses that candidate path.

## 3. Validation

- [x] 3.1 Run focused backend and frontend regressions for #2191 ambiguity, TFF/validated order, Preserve with known/unknown FPS, stale source acceptance, and candidate scope.
- [x] 3.2 Run `./scripts/verify.sh --auto` and confirm `ALL CHECKS PASSED`.
