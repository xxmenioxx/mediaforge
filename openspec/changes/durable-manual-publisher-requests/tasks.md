## 1. Durable manual request contract

- [x] 1.1 Add persisted manual request identity/state/reason/timestamps to the existing Publisher publication row and verify legacy rows migrate with empty request state.
- [x] 1.2 Change manual Publish and retry actions to persist intent plus audit evidence before returning `202 Accepted`; verify canonical pre-acceptance rejection, duplicate idempotency, and no acceptance on database failure.
- [x] 1.3 Expose accepted request state in the bounded publication summary and verify API responses distinguish accepted, active, published, and failed states.

## 2. Background execution and recovery

- [x] 2.1 Process accepted manual requests in the existing Publisher worker even when automatic publishing is disabled; verify worker processing still uses manual authorization and the existing fenced attempt path.
- [x] 2.2 Reconcile pending requests after simulated restart, active-attempt recovery, and retry; verify one logical delivery and no duplicate destination/archive/event mutation.
- [x] 2.3 Verify holds pause accepted work, release resumes it, and terminal failures remain actionable without automatic retry loops.
- [x] 2.4 Update the isolated Pipeline Smoke to treat HTTP 202 as durable acceptance and wait for the exact persisted publication identity to reach Published before checking delivered files.

## 3. Publisher UI

- [x] 3.1 Update API types and Publisher UI to show accepted/in-progress separately from persisted `PUBLISHED`, display stored failure reasons, and keep safe retry actions; verify with frontend build/focused checks.

## 4. Validation

- [x] 4.1 Measure manual Publish request ACK latency on supported local SQLite fixtures and verify p95 is at most one second without running delivery work in the HTTP handler.
- [x] 4.2 Run focused Publisher backend/frontend regressions and `python3 /Users/anuelvs/Documents/mediaforge/.agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto`; record the verifier result and check count (auto -> full, 6/6 checks, ALL CHECKS PASSED).
