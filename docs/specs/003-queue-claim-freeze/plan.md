# Plan — Queue Claim Freeze

**Depends on:** `spec.md`

## Approach

Refactor the existing path, do not create a parallel Queue implementation.

```text
Queue Selected -> lightweight intent
Worker CLAIM    -> prepare/freeze -> processing
```

## Discovery anchors

Inspect only what is needed around:

```text
QueueSelectedAssets
resolveSelectedAssetsQueue
prepareBatchQueueJob
captureSupplementalProfiles
resolveTrackPlan
worker claim / next runnable job
Queue Selected frontend success/navigation
```

## Backend

### Queue Selected
- `commit:false`: lightweight preview only.
- `commit:true`: persist intent/order/status only.
- preserve batch ID/order, publish mode, priority, cancel/retry/scheduler behavior.
- no deep freeze or media analysis.

### Worker
After atomic claim:

```text
release transaction
-> resolve effective config
-> reuse/refresh ScanResult
-> resolve profiles + Track Profile
-> resolveTrackPlan()
-> freeze immutable execution
-> processing
```

No expensive work inside claim transaction.

## Persistence

Prefer deferred snapshots in existing QueueJob.

Compatibility:

```text
legacy frozen job -> use frozen state
new unresolved job -> freeze after claim
```

## Frontend

On success:
- remain on Assets/Library;
- refresh queued state;
- show result;
- no Queue-specific per-asset scope fetches.

## Tests

Cover:
- lightweight preview/commit;
- batch metadata/order;
- atomic claim;
- claim-time config;
- ScanResult reuse/refresh;
- per-asset Track Plan;
- post-freeze immutability;
- preparation failure/retry;
- legacy frozen jobs;
- no forced navigation.

## Validate

```bash
./scripts/verify.sh --auto
```
