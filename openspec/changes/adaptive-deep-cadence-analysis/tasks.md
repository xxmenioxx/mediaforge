## 1. Backend adaptive evidence

- [x] 1.1 Extend IVTC window evidence with version, bounded outcomes/reasons, and source identity; test JSON compatibility and stale-version rejection.
- [x] 1.2 Add a deterministic selector that reuses three windows and chooses at most two additional distributed candidates, preferring existing motion/telecine evidence; test priority, no duplicates, and insufficient candidates.
- [x] 1.3 Implement deep continuation with the existing TFF/BFF FFmpeg/idet authority and thresholds; test convergence, inconclusive windows, contradictions, and distributed coverage.
- [x] 1.4 Extend the existing snapshot operation for async deep requests, progress, timeout, cancellation, and no-partial-persist; test cancellation and stale physical identity/revision.
- [x] 1.5 Recompute cadence evidence after a validated order and verify canonical Queue freeze recalculates FPS and HEVC without changing #352 claim guards.
- [x] 1.6 Align IVTC diagnostics with canonical `combmatch=full` before fallback/decimation; persist residual-interlace and repeated-field metrics, distinguish insufficient from contradictory evidence, and reject stale Level/Tier when effective FPS is unknown.
- [x] 1.7 Fix deduplicated candidate priority updates and cover historical #2191 ratios, selector ordering, and no-Queue persistence.

## 2. Queue and frontend review

- [x] 2.1 Re-resolve single-item Batch Preflight from current deep evidence; verify unresolved/stale evidence creates no QueueJob, ExecutionPlan, or reservation and resolved evidence freezes the same plan.
- [x] 2.2 Add API/UI start, cancel, progress, per-window evidence, explicit TFF/BFF, Preserve, and existing LAB/Test Encode actions; add focused frontend regressions.

## 3. Synthetic and repository validation

- [x] 3.1 Add a hard-telecine fixture generator to the #284 sandbox/image and regression-test its isolation; keep full output-fidelity smoke in #285 and add no Docker build/workflow.
- [x] 3.2 Run focused backend/frontend tests, `git diff --check`, `openspec validate adaptive-deep-cadence-analysis`, and `./scripts/verify.sh --auto`; require `ALL CHECKS PASSED`.
- [x] 3.3 Update the existing Draft PR #365 with `Closes #364`, validation, CI, and smoke results; do not merge or deploy.
- [x] 3.4 Add synthetic diagnostic regressions for residual interlace/repeated-field evidence, preserved #308/#310 interlaced fallback, and truly conclusive distributed resolution.
- [ ] 3.5 Run focused checks and `./scripts/verify.sh --auto`, complete CI, and update Draft PR #365 without merge or deploy.
