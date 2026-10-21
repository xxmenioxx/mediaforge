## 1. Canonical assignment freshness

- [x] 1.1 Add batched current-assignment comparison to persisted Batch Preflight reads; identify stale or unavailable unmodified assignments without changing candidate state.
- [x] 1.2 Add revision-fenced explicit re-resolve behavior that refreshes unmodified canonical domains, preserves deliberate candidate overrides, and rejects stale/unavailable acceptance.
- [x] 1.3 Add handler fixtures covering Asset > Path > Logical Group > Source Group > Global/default, Path Profile A over broader Profile B, unchanged edit/reopen, explicit candidate override, stale assignment refresh, missing references, audio-only fallback, and accepted QueueJob/ExecutionPlan identity.

## 2. Named assignment review UI

- [x] 2.1 Replace numeric profile/library and raw audio/track key controls with named selectors, stable secondary diagnostics, and honest custom/disabled/none/fallback states.
- [x] 2.2 Show effective selection, assignment origin, candidate-only override, missing inventory state, and destination paths in review and Edit.
- [x] 2.3 Add before-to-after assignment summaries and require review acknowledgement after changed values; preserve batch name, processing mode, cadence, and other edits.
- [x] 2.4 Add dialog regressions for named options, provenance, missing options, explicit edits, before/after review gating, unchanged re-resolution, and audio-only fallback labeling.

## 3. OpenSpec and validation

- [x] 3.1 Validate this OpenSpec change with `openspec validate queue-preflight-assignment-review --strict`.
- [ ] 3.2 Run focused backend and frontend regressions, then run `python3 /Users/anuelvs/Documents/mediaforge/.agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto` and confirm `ALL CHECKS PASSED`.
