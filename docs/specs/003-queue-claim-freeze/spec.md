# Queue Claim Freeze

**Status:** Draft / SDD  
**Scope:** Queue Selected, Worker claim/preparation, Library state.

## Goal

Queueing N assets must be cheap.

```text
QUEUE = intent
CLAIM/PREPARING = resolve + analyze if needed + freeze
PROCESSING = consume frozen execution
```

Do not prepare/freeze all assets during batch enqueue.

## Queue contract

`POST /api/queue/selected-assets`

### `commit:false`
Only lightweight planning:
- load selected assets;
- eligibility;
- accepted/rejected;
- batch order/basic metadata.

### `commit:true`
Only lightweight commit:
- revalidate eligibility;
- persist batch/order/queue intent;
- mark assets queued.

Neither path may require:

```text
ffprobe / FFmpeg
resolveTrackPlan()
ProfileSnapshot / TrackProfileSnapshot freeze
ResolvedTrackPlan freeze
final execution plan
per-asset ScanResult reads only for execution preparation
```

Backend remains authoritative.

## Claim contract

```text
QUEUED
  ↓
atomic CLAIM
  ↓
release DB transaction
  ↓
PREPARING
  ↓
PROCESSING
```

Claim must be short. No ffprobe/FFmpeg or expensive work while holding the claim transaction.

After claim, prepare only that asset:

```text
current AssetRecord
effective config
latest ScanResult
refresh analysis only if required
profiles
Track Profile
resolveTrackPlan()
immutable snapshots
execution plan
```

Processing starts only after successful freeze.

## Freeze semantics

Freeze occurs at claim/preparation time.

If config changes while still `QUEUED`, use the new effective config at claim.

After freeze, later config changes must not affect that execution.

Effective precedence remains:

```text
Asset > Path > LogicalGroup > SourceGroup > Global/default
```

## Scan / Track Profile

Keep separate:

```text
real scan = ffprobe/FFmpeg
ScanResult read = SQLite lookup
scope resolution = config inheritance
```

Reuse valid persisted ScanResult; refresh only when existing rules require it.

Track Profile remains per asset:

```text
Track Profile -> Asset ScanResult -> resolveTrackPlan() -> frozen ResolvedTrackPlan
```

Never share one resolved Track Plan across a batch.

## Persistence / compatibility

Prefer the existing QueueJob model with deferred/empty snapshot fields while queued.

Do not add a new Queue model unless required for correctness.

Already-frozen legacy queued jobs must remain executable.

Retry semantics:

```text
freeze incomplete -> prepare again
freeze complete   -> retry same frozen execution
```

Two workers must never prepare/process the same asset concurrently.

## Frontend

After Queue Selected succeeds:

```text
stay on current Assets/Library view
update queued state
show concise success result
```

Do not auto-navigate to Queue.

Queue Selected must not depend on per-asset `/api/asset-scope-configurations` requests.

## Performance invariant

For N assets:

```text
enqueue = O(N) lightweight DB/state work
```

Not O(N) execution preparation/media analysis.

Only claimed/preparing assets incur freeze/analysis cost.

## Non-goals

No:
- full Queue redesign;
- scheduler redesign;
- new Track Profile semantics;
- new scan freshness algorithm;
- general React Query refactor;
- Preview/Test Encode redesign.

## Acceptance

| Case | Expected |
|---|---|
| `commit:false` | lightweight planning only |
| `commit:true` | queue intent only |
| unclaimed asset | no frozen execution required |
| enqueue batch | no ffprobe/FFmpeg caused for all assets |
| claim one asset | prepare only that asset |
| valid ScanResult | reuse |
| required stale/missing analysis | refresh after claim |
| same Track Profile, different streams | separate Track Plans |
| config edit before claim | new config is frozen |
| config edit after freeze | execution unchanged |
| Queue Selected success | stay on Assets/Library |
| worker race | one claimant |
| legacy frozen job | remains executable |
