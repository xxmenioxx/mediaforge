# Proposed design — not implemented

## Context

Issue #415's recorded synthetic fixture uses 4,096 override entries (~960 KiB) and 178 concurrent requests. The issue reports Save Tracks HTTP p50/p95/p99 of 787.5/823.7/829.0 ms, `assetMutationMu` wait of 719.9/757.6/761.6 ms, transaction duration of 64.7/70.7/79.3 ms, `os.Stat` p95 of 50 µs, and no measurable override-lock wait. These are issue-provided synthetic figures, not NAS or network measurements, and their component percentiles are not additive.

### Current lock ownership and order

| Lock | Current callers / purpose | Held work relevant to Save Tracks |
|---|---|---|
| `assetMutationMu` (`track_maintenance.go`) | `UpdateTracks`; Queue `Create`, selected/batch Queue commit; batch Preflight create/edit/update/remove/discard/accept; synchronous and snapshot scan admission; track removal/edit/AAC maintenance admission; `ReturnPublishedAsIs` | Serializes asset mutation against Queue decisions and lifecycle reservations across all paths, including unrelated assets. `UpdateTracks` holds it across path authorization/stat and the SQLite transaction. |
| `assetOverrideMutationMu` (`asset_edit.go`) | `UpdateTracks`; asset `Edit`; `UpdateConversion`; metadata/conversion updates; path migration/rename and direct publication/reconciliation; override backfill/recovery | Protects process-local read-modify-write of shared `AppSetting` JSON maps. `UpdateTracks` decodes, patches, and rewrites the full conversion override map while holding it. |

Observed combined acquisition order is `assetMutationMu → assetOverrideMutationMu`; `UpdateTracks` releases in reverse order. `ReturnPublishedAsIs` also acquires the lifecycle mutex before the override mutex. No reverse nested acquisition was found in Serena references. Other override writers take only `assetOverrideMutationMu`; Queue, scan, and maintenance entry points take only `assetMutationMu`. New code must not acquire the lifecycle coordinator while holding an override-store lock or DB transaction.

`UpdateTracks` performs path resolution and body decoding before locking. Under both locks it checks library membership and file existence, starts one SQLite transaction, rejects an open Queue job or active maintenance operation, loads the complete `assetConversionOverrides` setting, applies the patch to the current path entry, rewrites the setting, and returns the persisted value. The DB transaction makes that setting write atomic; the process-wide override mutex prevents concurrent handlers from reading the same old JSON and overwriting one another's paths. The lifecycle mutex makes its Queue/maintenance checks and update linearize against the current lock-owning operations.

Queue `Create` and selected-asset commit hold `assetMutationMu` around validation/persistence. Batch Preflight mutators use it while resolving or changing their reviewed candidate and while accepting the resulting jobs. This is the authority that prevents a Save Tracks update from crossing Queue snapshot creation. `Scan` holds it through synchronous-scan admission and increments the active-scan marker before release; analysis then proceeds without the lock. Snapshot and track-maintenance requests use the lock to validate conflicts and reserve an operation before asynchronous work begins. The active marker/operation, not a lock held during FFmpeg/analysis, blocks later conflicting edits. Track edit/removal/AAC admission checks active scan, maintenance, and Queue state under the same boundary.

The process-wide locks are in-memory and protect one MVForge process. SQLite transaction behavior alone does not make a read/modify/write of a JSON blob safe across multiple app processes.

### Current direct owner inventory

The source-level inventory reviewed for gates 2.1/2.2 is:

- Save Tracks: `asset_tracks.go:UpdateTracks`.
- Queue: `queue.go:Create`, `queue.go:queueSelectedAssets`.
- Batch Preflight: `queue_preflight.go:CreateBatchPreflight`, `PreflightSelectedAssets`, `EditBatchPreflight`, `UpdateBatchPreflight`, `RemoveBatchPreflightAsset`, `DiscardBatchPreflight`, `AcceptBatchPreflight`.
- Scanner: `scanner.go:Scan`, `StartSnapshotOperation`.
- Track maintenance: `track_maintenance.go:StartTrackRemoval`, `StartTrackEdit`, `StartAddAACTrack`.
- Path/lifecycle mutation: `assets.go:ReturnPublishedAsIs`.

These are 16 direct functions and all acquire the current `assetMutationMu` variable. `asset_mutation_lock_owners_test.go` parses production Go source and asserts this exact set, so a new or removed direct owner requires review. This is a migration tripwire for the current process-global mutex only; it does not prove a future keyed coordinator exists, and it cannot establish runtime branch coverage by static inspection. Every owner must be explicitly migrated to one replacement boundary before keyed concurrency can be considered.

The direct-reference inventory does not claim every path-mutating route already uses this mutex. `assets.go:PublishAsIs`, `ApplyLibraryPathRename`, and `Rename` (and their directory/path reconciliation helpers) do not appear among the direct `assetMutationMu` owners. Their enclosing-call guard and conflict behavior must be explicitly mapped before they are assigned a future identity key; absent that proof, a future coordinator must route the transition through its exclusive fallback. That fallback is not implemented in current code. The inventory test intentionally reports current direct owners rather than masking these future migration requirements.

## Goals / Non-Goals

**Goals:** reduce unrelated-asset serialization only after every lifecycle owner shares the replacement coordinator; preserve per-asset Queue and maintenance guards, atomic snapshots, and cross-asset update isolation; make results reproducible and race-checkable.

**Non-Goals:** implement lock changes in this design lane; weaken Queue or maintenance checks; change Queue freeze semantics; optimize for NAS/network latency; modify media, APIs, or production DB; claim the issue's benchmark figures were reproduced here.

## Gate 2.1 — identity disposition (design only; gate remains open)

`AssetRecord` provides a `uint` primary key and a unique `Path`; it has no separate lock-identity or alias table. `UpdateTracks` currently resolves an input path with `filepath.Clean` and checks configured-root membership lexically before its file stat and override transaction. That flow does not resolve a request to an `AssetRecord.ID` before locking, and lexical path normalization does not prove physical-file identity. `QueueJob` can retain an optional `SourceAssetRecordID`, but historical jobs may lack it or refer to a record that no longer represents the current path. Therefore the following is the proposed identity contract, not behavior enforced by current code:

| Case | Proposed identity / disposition | Evidence boundary |
|---|---|---|
| Indexed asset | Stable key is the MVForge database namespace plus `AssetRecord.ID`. Bind the requested path to exactly one current record and revalidate that row/path/revision inside the coordinator boundary before using the key. Do not key by mutable `Path`. | Model has primary-key `ID` and unique `Path`; current Save Tracks does not resolve the ID before locking. |
| Unindexed file | Take one exclusive process-wide fallback. Do not create a path-key from `Abs`/`Clean`; no indexed identity exists to prove aliases or concurrent indexing are coordinated. | Existing `AssetRecord` lookup is not part of `UpdateTracks` admission. |
| Path aliases / symlinks | Share an indexed key only when an authoritative path-to-record resolver proves both names bind to the same current record and revision. Otherwise use the exclusive fallback or reject according to existing path authorization. The fallback grants no filesystem authorization. | Current path checks establish configured-root membership lexically; no alias-to-record resolver was found. |
| Rename / move | Preserve the same `AssetRecord.ID` as the logical identity. Hold the exclusive fallback across the old path, new path, filesystem transition, and DB rebind; release only after the committed row still has that ID and new path. If an existing path owner cannot retain/revalidate the ID, keep it on exclusive fallback until reindex/reconciliation proves the new binding. | Current path/lifecycle owners are inventoried below, but continuity of ID is not an existing coordination contract. |
| Historical asset/job | Use `SourceAssetRecordID` only after validating that the current row and frozen provenance still identify the same source. Missing, deleted, stale, or unverified IDs use the exclusive fallback; never infer identity from filename, size, timestamp, or path text alone. | `QueueJob.SourceAssetRecordID` is optional; historical identity may be absent. |
| Identity/schema migration | Keep the database namespace and `AssetRecord.ID` as the canonical key. Backfill legacy path-keyed override entries to an ID only when one exact current record binding is proven in the transaction; ambiguous or unindexed entries remain on the legacy representation and use the exclusive fallback. Switch lock domains only when all old owners and path-transition call paths share one bridge/fallback boundary; no mixed keyed/global admission. | Existing override storage is a path-keyed JSON setting; no identity migration or bridge exists today. |

A collision or conflicting mapping (one path resolving to multiple record IDs, or aliases resolving to different IDs without a proven distinction) is unresolved identity: do not enter a keyed section; serialize through the exclusive fallback and fail the operation if canonical binding still cannot be established. Identity keys must be scoped to the same database namespace; numeric IDs from separate databases are not interchangeable. Current mutexes are process-local, so this contract alone would not coordinate multiple MVForge processes.

No executable identity resolver, alias collision test, or fallback test exists in this lane. Task 2.1 remains **incomplete**; the table narrows the future design but does not establish runtime behavior.

## Decisions

### 1. Keep the current global locks until a coordinated replacement is proven

Do not remove or narrow `assetMutationMu` by changing only `UpdateTracks`. Every caller that currently uses it to order Queue, preflight, scanning, maintenance, or path/lifecycle transitions must move to the same coordinator in one compatible change. A missed caller creates a gap where Save Tracks can pass its open-job check while Queue freezes old settings, or Queue can start while a mutation is in flight.

### 2. Prototype keyed lifecycle coordination and normalized conversion storage together

The recommended prototype uses a ref-counted keyed lock coordinator for canonical asset identities, with a short coordinator gate protecting the registry and a global exclusive fallback for paths whose stable identity cannot be resolved. Multi-asset work (selected Queue, Batch Preflight, moves/renames) acquires unique identity keys in sorted order. Single-asset Save Tracks, scan admission, and maintenance admission acquire the same key. Queue reservation/freeze and asset mutation remain inside that per-asset serialization boundary. All legacy/global and keyed paths must share the same gate until migration is complete; a mixed path/key domain is not safe.

The current JSON blob forces every path update to rewrite the entire map. Per-asset locks alone cannot protect independent path writes to one shared blob without serializing the read-modify-write. The prototype therefore also evaluates a normalized conversion-override row keyed by stable asset identity (or a fully canonical path identity where no record exists), updated with a single transactional upsert. Keep metadata JSON storage separate unless its write path is deliberately migrated too. Any combined Asset Edit must preserve atomic metadata+conversion semantics in one DB transaction.

Canonical identity is a design gate, not a guess: path cleaning alone does not prove aliases, symlinks, or rename/migration ownership are identical. Resolve an existing `AssetRecord` identity before keyed admission. If identity is missing or path migration is in progress, use the exclusive fallback gate; never let one request use a path lock while another uses an asset-ID lock for the same file. Migration must define how records without a stable ID are represented before implementation.

### 3. Preserve a strict order and short reservation boundaries

Future order: coordinator gate (shared for keyed work, exclusive fallback for unresolved/global work) → sorted per-asset locks → any remaining metadata-store lock → SQLite transaction. Do not acquire a coordinator lock from inside a transaction or while holding the metadata lock. Release keyed locks only after the guarded DB transition is committed. For asynchronous scan/maintenance, persist or register the active marker/operation before releasing the keyed lock; the background task must not retain the keyed lock for the duration of media work.

Queue batch operations acquire the complete known identity set in sorted order, revalidate membership/revision under those locks, and write the frozen reservation/job in the same transaction. If the candidate set changes, abort/re-resolve rather than proceed with a partial lock set. Existing Queue frozen snapshots and retry semantics remain immutable.

### 4. Evaluate and reject incomplete alternatives

- **Per-asset mutexes only:** rejected as a standalone optimization. They do not prevent lost updates to the shared JSON setting and are unsafe if any Queue/scan/maintenance owner still uses only the old global mutex.
- **Immutable map snapshot/precomputation only:** useful for moving parsing and patch normalization outside a critical section, but it cannot safely commit against a stale snapshot. A versioned compare-and-swap/retry must reload and merge the latest entry, and the single-row JSON rewrite remains a cross-asset serialization point. Treat as a benchmarked interim candidate, not an approved fix.
- **Normalized transactional override storage only:** reduces full-blob decode/rewrite and allows independent row updates, but leaves the lifecycle mutex as a global bottleneck and requires migration, backfill, path-move reconciliation, old-settings compatibility, and rollback semantics. It cannot deliver the requested concurrency alone.
- **Shorten the global lock by moving Queue/maintenance checks outside it:** rejected. A check outside the authority boundary races with Queue claim/freeze or maintenance reservation and can invalidate execution snapshots.

### 5. Deterministic characterization and benchmark protocol

Existing characterization: `TestAssetTracksPatchConcurrentDifferentAssetsPreservesEntries` uses a start-channel barrier and disposable SQLite to prove concurrent writes to different paths preserve both entries and an unrelated entry. Existing regressions cover direct API rejection for active Queue work despite stale UI capability, rejection during maintenance, same-file aliases, malformed requests, and historical Queue snapshot immutability. This design lane adds `TestAssetTracksPatchConcurrentSameAssetPreservesPartialUpdates`, which releases two same-path patch requests from one channel barrier and verifies both independent fields and untouched fields survive. No production hook or runtime change is added. These tests characterize outcomes; they do not claim to force a particular scheduler interleaving. Future race-specific tests must use explicit barriers at the reservation/transaction boundary rather than sleeps.

Future deterministic scenarios:

1. **Same asset, two partial saves:** barrier both requests at entry, release together, and verify linearizable merge of disjoint patch fields; repeat same-field writes and assert a documented serialization order/last committed value.
2. **Different assets:** block both at a test-only DB callback after per-asset acquisition; prove both can be in their critical sections concurrently, then verify both rows and an unrelated row survive.
3. **Queue versus Save Tracks:** pause each contender immediately before its guarded DB transition. Exactly one valid order is observed: Save commits then Queue freezes the new settings, or Queue commits and Save returns conflict without changing overrides. Never allow Queue freeze and Save commit to cross.
4. **Scan/maintenance reservation versus Save Tracks:** pause before marker/operation persistence. The edit either commits before reservation or sees an active reservation and conflicts; no window exists after reservation checks but before marker/operation creation.
5. **Failure/rollback/retry:** inject transaction failure after patch calculation but before commit, verify no setting/normalized row is partially written, then retry and verify one complete update. Exercise context cancellation before transaction entry and ensure no leaked lock reference.
6. **Multi-asset ordering:** start overlapping two-asset Queue batches with reversed input order; canonical sort must avoid deadlock and both operations must either commit consistently or report a conflict.

Benchmark methodology (same binary/toolchain/host and fixture before and after):

- Use a temporary SQLite DB and generated disposable files; seed exactly 4,096 conversion overrides totaling a recorded serialized byte count near the issue's ~960 KiB; use 16 target assets plus untouched control entries.
- Drive the same Save Tracks HTTP route with fixed request bodies and an explicit 178-worker in-flight limit (record `GOMAXPROCS`, worker count, request count, warm-up count, and elapsed time). Keep request mix and SQLite pragmas constant. Run at least 5 independent samples per revision; do not infer percentiles by adding component percentiles.
- Record raw per-request latency and nearest-rank p50/p95/p99; throughput; HTTP status/error counts; allocations and bytes per request; per-request `assetMutationMu` wait, `assetOverrideMutationMu` wait, transaction duration, and filesystem stat duration from existing structured timing evidence. Also capture DB row/transaction counts and final state correctness.
- Report environment (Go version, GOOS/GOARCH, CGO, SQLite driver/version, host CPU count, `GOMAXPROCS`) and source SHA. Do not call local fixture measurements NAS performance.
- For race tests use a writable isolated Go build cache (`GOCACHE` under a task temp directory) and `GOTOOLCHAIN=local`; run focused tests with `-race`, not a mocked race flag. A default sandbox cache outside writable roots is not a valid race-toolchain verdict.

Provisional implementation GO thresholds: all deterministic cases pass under `go test -race`; no partial or lost updates, stale Queue snapshot, or guard bypass; 0 unexpected HTTP/DB errors; migration/backfill is repeatable and rollback-safe; at least 25% p95 end-to-end latency improvement and 20% p95 global-lock wait reduction over the matched baseline, with p99 not regressing by more than 5% and allocations/CPU/DB use within 10% of baseline. These are qualification gates for discussion, not measured results. NO-GO if identity cannot be canonicalized across all owners, legacy/global and keyed paths cannot share one coordination gate, correctness tests need sleeps or production-only hooks, migration loses old settings, or improvement misses the thresholds. Until all gates pass, retain current mutex behavior.

## Risks / Trade-offs

- [Incomplete lock-owner migration creates lifecycle races] → inventory every current `assetMutationMu` reference and move all ownership in one reviewed change; add a test per owner before enabling keyed concurrency.
- [Path aliases or rename races split one asset into multiple keys] → stable identity resolver plus exclusive fallback gate; sorted lock sets for multi-asset transitions.
- [JSON map updates overwrite other paths] → do not rely on per-asset locks while keeping one shared map; use transactional normalized rows or a versioned CAS/retry that proves no lost updates.
- [SQLite write serialization limits parallel gains] → measure transaction and writer-lock wait separately; keep claims to demonstrated latency results.
- [Race check fails due sandbox cache] → isolate `GOCACHE` in writable task temp and record Go env; distinguish toolchain build from test assertion results.
- [Migration changes old setting compatibility] → idempotent backfill, legacy read compatibility, dual-source comparison, and rollback before deleting the old setting.

## Migration Plan

No migration is part of this design lane. Any implementation proposal must include an idempotent conversion of existing JSON entries, read compatibility during rollout, reconciliation for rename/path-move operations, integrity comparison, and a rollback window before removing the legacy setting. No migration may alter Queue snapshots or media.

## Open Questions

- Before implementation, determine whether every Save Tracks path has an `AssetRecord` identity and how unindexed files/aliases acquire an identity without a race. If not, preserve an exclusive global fallback and prove it coordinates with keyed requests.
- Confirm whether the supported deployment guarantees a single MVForge process. The current mutexes are process-local; multi-process deployment would require DB-level conditional updates/locking as an explicit requirement.
