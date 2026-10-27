## 1. Characterization

- [x] 1.1 Add same-asset concurrent partial-patch regression and verify it with `go test ./internal/handlers -run '^TestAssetTracksPatchConcurrentSameAssetPreservesPartialUpdates$' -count=1`.
- [x] 1.2 Reproduce the focused race command with isolated writable `GOCACHE`; distinguish sandbox cache access from test/toolchain failure.

## 2. Future implementation gates — not started

- [ ] 2.1 Resolve and document one canonical asset-lock identity for indexed assets, path aliases, unindexed files, renames, and migrations; verify identity collision and fallback tests.
- [ ] 2.2 Build a lock-owner inventory covering Save Tracks, Queue create/selected commit, every Batch Preflight mutation, scanner, maintenance, and path/lifecycle mutations; verify every path uses one future coordination boundary. Partial evidence: an AST tripwire now asserts the 16 current direct `assetMutationMu` owners, but `PublishAsIs`, `ApplyLibraryPathRename`, `MigratePath`, and `Rename` use other guards and are not covered by that test. Their enclosing guards and conflict behavior remain to be mapped; no future coordinator or exclusive fallback is implemented.
- [ ] 2.3 Prototype sorted, ref-counted per-asset coordination plus a global exclusive fallback; verify same-asset serialization, different-asset overlap, Queue/scan/maintenance ordering, cancellation, cleanup, and reversed multi-asset order with deterministic barriers and `go test -race`.
- [ ] 2.4 Prototype transactional per-asset conversion-override storage with idempotent backfill and old-settings compatibility; verify failure rollback, cross-asset no-lost-update behavior, Asset Edit atomicity, and path move/rename reconciliation on disposable SQLite.
- [ ] 2.5 Run the matched 4,096-entry / ~960 KiB / 16-asset / 178-in-flight benchmark for at least five samples per revision; report p50/p95/p99, lock waits, DB transaction durations, allocations, throughput, errors, and environment.
- [ ] 2.6 Apply the design's correctness and provisional performance gates; record an explicit implementation GO/NO-GO before changing production locks or migrations.
- [ ] 2.7 After an approved implementation, run focused and race tests, OpenSpec strict, `./scripts/verify.sh --auto`, and Pipeline Smoke under serialized shared validation; verify the final immutable SHA.
