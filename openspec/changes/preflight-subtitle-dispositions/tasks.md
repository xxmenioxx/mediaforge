## 1. Canonical subtitle projection and candidate resolution

- [x] 1.1 Extend the canonical resolved subtitle decision with source metadata, action provenance, and candidate edit eligibility; verify fallback, preferred-rule, explicit-Asset, and sidecar decisions with focused resolver tests.
- [x] 1.2 Add identity-bound Queue-only action overrides to candidate input and canonical resolution; verify Keep, Extract, Keep + Extract, Remove, Inherit, stale identity, and higher-priority policy conflicts with focused backend tests.
- [x] 1.3 Return the versioned per-stream projection, summary counts, policy warnings, and empty/unavailable states from Batch Preflight; verify candidate acceptance freezes the reviewed subtitle plan and no persistent Asset override is written.
- [x] 1.4 Reconcile QueueJob subtitle and font artifacts from the updated canonical plan after candidate-only overrides; cover Extract/Keep/Keep + Extract/Remove, an empty final sidecar set, and Worker retry/restart behavior.

## 2. Preflight review and candidate-only editing

- [x] 2.1 Add frontend types and a compact summary with expandable per-stream decisions, provenance, formats, OCR, and policy help; verify the `subtitleDisposition=extract` and `remove` cases in focused component tests.
- [x] 2.2 Add explicit per-stream candidate-only editing with before/after feedback, Inherit, conflict-disabled actions, save, cancel, and revert behavior; verify exact stream identity and that discarded local edits do not submit an update.

## 3. Verification

- [x] 3.1 Run focused backend resolver and Queue Preflight tests plus the focused BatchPreflightDialog frontend test; record actual results.
- [x] 3.2 Run `openspec validate preflight-subtitle-dispositions --strict` and the serialized `./scripts/verify.sh --auto`; record scope, check count, warnings, and outcome.

## Validation record

- Passed: focused Go resolver and Queue Preflight tests.
- Passed: `openspec validate preflight-subtitle-dispositions --strict`.
- Blocked: `npm test -- src/components/BatchPreflightDialog.test.tsx`; this worktree has no `node_modules`, and `npm ci --offline --ignore-scripts --no-audit --no-fund` could not find the locked `zod-validation-error@4.0.2` tarball in the issue-specific cache.
- Failed: serialized `./scripts/verify.sh --auto` selected `auto -> full` and stopped at `focused-go-test-handlers`, 0/1 checks. Existing `TestTranslationMultiBatchCheckpointStaysMonotonicAndRecoversFailure` panicked because the sandbox denied `httptest.NewServer` binding `[::1]:0` with `operation not permitted`. An isolated retry reproduced the failure. Logs are preserved at `/private/tmp/mediaforge-dupla381303-origin-main-7726510/.git/dupla/resources/issue-381-attempt-1/tmp/issue-381-verify-retry.log` and `issue-381-httptest-bind-retry.log`.

### Follow-up: Queue-only artifact reconciliation

- Passed focused backend regressions: `TestBatchPreflightProjectsSubtitleActionsAndFreezesQueueOnlyOverride` and `TestQueuePreflightSubtitleDispositionReconcilesArtifactsAndWorkerRetry`.
- Passed `openspec validate preflight-subtitle-dispositions --strict`.
- Passed `./scripts/verify.sh --auto`: `auto -> full`, 6/6 checks, 363 seconds, `ALL CHECKS PASSED`.
