## 1. Evidence and design

- [x] 1.1 Inspect current ValidationRun, evaluateValidationRules, Publisher handoff and historical frozen binding boundaries; document why a Validation-only fix is insufficient.
- [x] 1.2 Write proposal, design and observable scenarios covering independent original intent, append-only history, stale revisions, Publisher consistency, manual authorization and immutable media.
- [ ] 1.3 Review the OpenSpec with Codex, identify the exact independent authoritative per-asset fields in existing Queue/ExecutionPlan/Result schemas, and list any historical evidence gaps. Decide a concrete minimal audit schema; do not implement before this contract is proven.

## 2. Read-only local preview

- [ ] 2.1 Implement a deterministic pure expected-contract resolver that rejects output-derived guesses and contaminated execution-time input.
- [ ] 2.2 Expose a bounded, read-only per-job reconciliation preview with SAFE / BLOCKED / NEEDS_EVIDENCE and per-rule expected/actual diff, guarded by canonical Result/ExecutionPlan identity.
- [ ] 2.3 Add synthetic fixtures for crossed snapshots, absent independent intent, replaced output/revision, stale sidecars and normal unaffected jobs.

## 3. Authorized persistence and Validation

- [ ] 3.1 Add a versioned append-only reconciliation record (or demonstrate an existing immutable model suffices), with server actor/reason and exact plan/Result/expectation fingerprints.
- [ ] 3.2 Add explicit commit/revalidate and idempotency, atomic active ValidationRun projection update and immutable prior-run preservation.
- [ ] 3.3 Extend the canonical Validation evaluator with a narrowly scoped proven expectation overlay; preserve all non-track, Result identity, timeline, manifest and policy checks.
- [ ] 3.4 Test genuine missing sidecars/tracks and unrelated video.properties UNVERIFIED remain blocked despite corrected track expectations.

## 4. Publisher, review UX and permissions

- [ ] 4.1 Require Publisher to consume the same identity-bound reconciled package for eligibility, sidecar validation and materialization. Keep destination/overwrite/archive checks; prohibit auto-publication.
- [ ] 4.2 Add explicit Review Center preview, reason, approval and provenance; never provide a bulk commit or infer SAFE in frontend.
- [ ] 4.3 Verify concurrent commit/publish, retries, stale active runs, rollback/disable and false-positive paths with focused integration tests.

## 5. Validation and rollout boundaries

- [ ] 5.1 Run scoped Go, frontend and synthetic E2E tests, git diff --check and ./scripts/verify.sh --auto; open a separate implementation Draft PR and require CI/Pipeline Smoke.
- [ ] 5.2 Produce a written bounded NAS read-only dry-run plan and obtain explicit approval before any remote discovery.
- [ ] 5.3 Require separate user authorization for deployment, for each production reconciliation/new ValidationRun and again for any Publisher action. Do not rewrite historical snapshots, media or ResultArtifacts.
