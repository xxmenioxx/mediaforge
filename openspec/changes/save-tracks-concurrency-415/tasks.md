## 1. Characterization

- [x] 1.1 Add same-asset concurrent partial-patch regression and verify it with `go test ./internal/handlers -run '^TestAssetTracksPatchConcurrentSameAssetPreservesPartialUpdates$' -count=1`.
- [x] 1.2 Reproduce the focused race command with isolated writable `GOCACHE`; distinguish sandbox cache access from test/toolchain failure.

## 2. Future implementation gates — not started

- [ ] 2.1 Resolve and document one canonical asset-lock identity for indexed assets, path aliases, unindexed files, renames, and migrations; verify identity collision and fallback tests.
- [x] 2.2 Add a source AST inventory test for every current direct `assetMutationMu` owner across Save Tracks, Queue, Batch Preflight, scanner, maintenance, and path/lifecycle mutation. This verifies the current shared process-global gate only; every owner still requires explicit migration before any future keyed coordinator is enabled.
- [ ] 2.3 Prototype sorted, ref-counted per-asset coordination plus a global exclusive fallback; verify same-asset serialization, different-asset overlap, Queue/scan/maintenance ordering, cancellation, cleanup, and reversed multi-asset order with deterministic barriers and `go test -race`.
- [ ] 2.4 Prototype transactional per-asset conversion-override storage with idempotent backfill and old-settings compatibility; verify failure rollback, cross-asset no-lost-update behavior, Asset Edit atomicity, and path move/rename reconciliation on disposable SQLite.
- [ ] 2.5 Run the matched 4,096-entry / ~960 KiB / 16-asset / 178-in-flight benchmark for at least five samples per revision; report p50/p95/p99, lock waits, DB transaction durations, allocations, throughput, errors, and environment.
- [ ] 2.6 Apply the design's correctness and provisional performance gates; record an explicit implementation GO/NO-GO before changing production locks or migrations.
- [ ] 2.7 After an approved implementation, run focused and race tests, OpenSpec strict, `./scripts/verify.sh --auto`, and Pipeline Smoke under serialized shared validation; verify the final immutable SHA.
