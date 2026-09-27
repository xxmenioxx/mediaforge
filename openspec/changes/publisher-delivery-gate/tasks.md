## 1. Publication identity and persistence

- [ ] 1.1 Define the Publisher-owned publication identity bound to one Queue
  execution, frozen ExecutionPlan/destination/mode, canonical Result revision,
  ValidationRun/eligible override, and source provenance; preserve QueueJob as
  execution authority and #174 AssetRecord as archive authority.
- [ ] 1.2 Persist explicit publication state, bounded append-only attempt
  evidence, active attempt/lease phase, and transactionally maintained QueueJob
  summary projections; keep historical unbound publications readable as legacy.
- [ ] 1.3 Persist server-assigned `local_operator`/`system` actor references and
  append-only versioned evidence for audited Publisher actions without
  rewriting historical attempt evidence or adding authentication infrastructure.

## 2. Handoff from #53

- [ ] 2.1 Route manual and automatic handoff through #53's canonical run or
  eligible override authorization and verify exact execution, plan, Result
  generation/revision, and required delivery manifest.
- [ ] 2.2 Reject stale ValidationRun, replaced/unverifiable Result, missing
  frozen destination identity, and ineligible overrides without rebinding,
  recalculating Settings policy, or changing #53 evidence.

## 3. Atomic materialization and conflicts

- [ ] 3.1 Implement controlled-root checks, private destination-side staging,
  free-space/read/write/flush handling, and atomic final rename for primary
  media and required artifacts.
- [ ] 3.2 Enforce frozen standard versus replace-library policy, existing-target
  conflict handling, reversible backups, and safe pre-commit rollback.
- [ ] 3.3 Persist the exact manifest and define the database transaction that
  marks `PUBLISHED` only after final media, required sidecars, and publication
  provenance are committed.

## 4. Sidecars

- [ ] 4.1 Stage, verify, and atomically promote each plan-required sidecar from
  the canonical Result manifest; record optional artifact outcomes separately.
- [ ] 4.2 Verify partial-sidecar failure never marks publication complete,
  archives the original, or enables `AssetPublished` emission.

## 5. Archive and retention integration

- [ ] 5.1 Invoke original archival only after canonical `PUBLISHED` commit and
  reuse #174's existing AssetRecord, QueueJob provenance, and retention policy.
- [ ] 5.2 Keep publication success independent from archive status and recover
  interrupted file-move/AssetRecord persistence without adding an archive
  lifecycle model.

## 6. AssetPublished event

- [ ] 6.1 Add one durable outbox identity per publication after required files,
  publication provenance, and original-archive outcome are recorded.
- [ ] 6.2 Retry the same event identity after delivery failure/ambiguous ack;
  keep subscriber and future Jellyfin behavior outside Publisher filesystem
  logic and never roll back `PUBLISHED`.

## 7. Publisher and History UI

- [ ] 7.1 Make Publisher exception-first for held, blocked, failed, stale
  handoff, and destination-conflict cases with stored bounded evidence and
  audited hold/release/retry actions.
- [ ] 7.2 Project publication and separate #174 archive facts into History;
  use bounded batch summaries without per-row filesystem checks or scans on
  render/poll.

## 8. Recovery and idempotency

- [ ] 8.1 Recover exact persisted publication attempts after stops during
  staging, partial final promotion, database commit, and original archival;
  introduce atomic `READY -> PUBLISHING` claims, bounded lease renewal/expiry,
  and conditional reacquisition with fencing; recover only persisted
  nonterminal records/manifests through AutoPublisher/startup cadence, never
  by walking Library/RAW roots. Fail closed when file ownership or identity is
  ambiguous.
- [ ] 8.2 Make duplicate handoff/retry idempotent for `PUBLISHING` and
  `PUBLISHED`, and require new Queue/Result identity for changed intent.

## 9. Focused regressions

- [ ] 9.1 Cover canonical #53 binding, stale/replaced Result rejection,
  destination changes/conflicts, frozen overwrite/replace policy, manual hold,
  server actor assignment, and rejection/ignoring of client-supplied actor IDs.
- [ ] 9.2 Cover temp materialization, atomic per-file rename, sidecar failure,
  free-space/read/write errors, controlled roots/symlinks, and rollback safety.
- [ ] 9.3 Cover crash/restart at every persisted boundary, concurrent claim
  exclusion, lease expiry/conditional reacquisition/fencing, idempotent
  recovery, #174 archive provenance, one event identity, subscriber failure,
  and read-only Publisher/History rendering and polling.

## 10. Final verification

- [ ] 10.1 Run focused backend and frontend regressions for the changed
  Publisher, Queue, archive, event, and UI contracts.
- [ ] 10.2 Run frontend lint and require exactly 0 errors and 0 warnings.
- [ ] 10.3 Run
  `openspec validate publisher-delivery-gate --type change --strict`.
- [ ] 10.4 Run `./scripts/verify.sh --auto`; the required result is
  `ALL CHECKS PASSED`. Record any environment limitation separately.
