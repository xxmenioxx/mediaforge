## 1. Evidence and design

- [x] 1.1 Inspect current ValidationRun, evaluateValidationRules, Publisher handoff and historical frozen binding boundaries; document why a Validation-only fix is insufficient.
- [x] 1.2 Write proposal, design and observable scenarios covering independent original intent, append-only history, stale revisions, Publisher consistency, manual authorization and immutable media.
- [x] 1.3 Review the OpenSpec with Codex, identify the exact independent authoritative per-asset fields in existing Queue/ExecutionPlan/Result schemas, and list any historical evidence gaps. Decide a concrete minimal audit schema; do not implement before this contract is proven.

## 2. Read-only local preview

- [x] 2.1 Implement a deterministic pure evidence gate: reject output-derived guesses and contaminated Queue snapshots; return no proposed track contract when independent intent is unavailable. With current models this gate cannot produce SAFE.
- [x] 2.2 Expose a bounded, read-only per-job preview with Queue/asset, active ExecutionPlan, Result and ValidationRun identity; include per-rule historical expected/actual diffs and block stale record revisions. SAFE remains unreachable until an independent immutable intent source is modeled.
- [x] 2.3 Add synthetic fixtures for crossed snapshots, the audio/subtitle/manifest failure patterns, missing/replaced sidecar IDs, stale Result-to-ValidationRun revisions, mismatched plan binding and unaffected jobs. The preview reports that live output bytes are not re-hashed and sidecar content revisions are not modeled.

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

- [x] 5.1 Run focused Go preview regressions, git diff --check through ./scripts/verify.sh --auto, and the path-selected CI/Pipeline Smoke for implementation Draft PR #371. Local focused tests and auto verification passed; CI/Pipeline Smoke passed on #371. Backend-only routing skips frontend and compliance checks.
- [ ] 5.2 Produce a written bounded NAS read-only dry-run plan and obtain explicit approval before any remote discovery.
- [ ] 5.3 Require separate user authorization for deployment, for each production reconciliation/new ValidationRun and again for any Publisher action. Do not rewrite historical snapshots, media or ResultArtifacts.
