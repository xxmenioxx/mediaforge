# Tasks — Queue Claim Freeze

**Depends on:** `spec.md`, `plan.md`

## Task 1 — Discover boundaries
- Trace Queue Selected → preparation → worker claim.
- Locate first worker assumption that snapshots already exist.
- Locate frontend success navigation/state refresh.
- Report exact files/symbols and smallest safe boundary before editing.

## Task 2 — Lightweight Queue Selected
- Keep `commit:false` to eligibility/confirmation.
- Keep `commit:true` to intent, batch metadata/order, queued state.
- Remove deep freeze, Track Plan resolution, and media analysis.
- Preserve existing scheduling/cancel/retry/publish behavior.

## Task 3 — Freeze after claim
- Keep claim atomic/short.
- Release DB transaction before expensive work.
- Prepare only claimed asset.
- Resolve current effective config.
- Reuse/refresh ScanResult per existing rules.
- Resolve profiles + Track Profile + `ResolvedTrackPlan`.
- Freeze immutable execution before processing.
- Preserve legacy frozen jobs.

## Task 4 — Failure/concurrency
- Prevent duplicate preparation.
- Make preparation failure recoverable.
- Incomplete freeze → prepare again.
- Completed freeze → retry same snapshot.
- Preserve restart/recovery behavior.

## Task 5 — Frontend
- Remove automatic Queue navigation.
- Update queued asset state in current Assets/Library view.
- Avoid Queue-specific per-asset scope requests.

## Task 6 — Regression
Cover:

```text
single + batch
commit:false/true
batch order
replace_library_asset
claim race
config before/after freeze
Track Profile differences
valid/missing ScanResult
cancel/retry/restart
legacy frozen jobs
stay on Library
```

## Task 7 — Validate

```bash
./scripts/verify.sh --auto
```

If it fails, follow `AGENTS.md`.

**Done:** spec satisfied and `ALL CHECKS PASSED`.
