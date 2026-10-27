# ADR proposal: Safe Save Tracks concurrency boundary

- **Status:** Proposed; design lane only; not implemented.
- **Date:** 2026-10-27.
- **Context:** Issue #415 reports synthetic Save Tracks latency mostly in the global lifecycle mutex. That mutex also orders Queue freeze, scan admission, maintenance reservation, and asset path transitions. A second global mutex protects whole-map JSON override read-modify-write.

## Decision

Do not remove or narrow either production mutex as a Save Tracks-only change. The next implementation candidate must replace the lifecycle mutex coherently across all of its current owners with a shared per-asset coordinator (stable canonical identities, sorted multi-asset acquisition, and an exclusive fallback for unresolved/legacy paths) and replace or safely version the shared conversion-override JSON write path. Preserve current behavior until both coordination domains have an atomic replacement and the test/benchmark gates below pass.

The recommended prototype is a ref-counted keyed coordinator plus transactional normalized conversion-override rows. Queue freeze and lifecycle reservation stay within the same asset lock boundary as Save Tracks. Any global metadata setting lock remains separately ordered and may not acquire the coordinator from inside that lock or a DB transaction.

## Proposed identity contract and current evidence

The future coordinator key is the database namespace plus `AssetRecord.ID`, bound to one authorized current path and revalidated under the shared coordinator admission boundary. The current model has a `uint` primary key and an exact-path unique index, but no alias table or dedicated revision field. `resolveMediaPath` only cleans absolute paths; it does not resolve symlinks. The new synthetic characterization uses disposable SQLite and a generated symlink to show that two distinct `Path` rows can have different IDs for the same filesystem object, while an unindexed path has no ID. Therefore unindexed files, unresolved/ambiguous aliases, path transitions without verified ID continuity, and historical jobs without a current verified `SourceAssetRecordID` must use one exclusive process-wide fallback or reject. That fallback must exclude all keyed requests and scanner/index admission. Rename/move must stay on that fallback through filesystem work, DB rebind, rollback, and reconciliation unless the same ID/path binding is proven. A filename, size, timestamp, normalized path, or `SourceAssetRecordID` pointer alone is not proof of current identity. The test characterizes the missing authority; it does not test an implemented fallback. Gate 2.1 remains open.

Legacy override backfill may bind a path entry to an `AssetRecord.ID` only when the current path-to-record mapping is unique and validated in the same transaction. Ambiguous/unindexed rows remain legacy and use exclusive fallback. The path-owner audit records `PublishAsIs`, `ApplyLibraryPathRename`, `MigratePath`, and `Rename` separately from the direct-mutex set. All four currently acquire `assetOverrideMutationMu`, but none shares `assetMutationMu` with Queue, scanner, or maintenance. PublishAsIs, ApplyLibraryPathRename, and MigratePath perform their active-work/open-job checks before acquiring the override guard; Rename checks after acquiring it. Their filesystem and DB helpers do not acquire either mutex and rely on the handler caller. `ApplyLibraryPathRename` revalidates ID/path/library and requires one row during DB reconciliation; `MigratePath` updates by ID without checking affected rows; `Rename` updates by old path without checking affected rows; `PublishAsIs` records the source ID in DirectPublication and deletes the source AssetRecord before inventory sync. The AST test records these caller/guard edges and ordering, but it is not proof of a shared future boundary.

A source AST inventory records 16 current direct `assetMutationMu` owners across Save Tracks, Queue, Preflight, scanner, maintenance, and `ReturnPublishedAsIs`. A second source AST characterization now records the four indirect path/lifecycle handlers, their `assetOverrideMutationMu` ownership, key helper calls, and current check ordering. These tripwires protect the present inventory; they do not prove complete transitive ownership, runtime branch coverage, or a future keyed coordinator. Task 2.2 remains open until every direct and indirect path transition participates in one future boundary.

## Alternatives considered

1. **Delete `assetMutationMu` from Save Tracks only — rejected.** An open-job/maintenance check could race Queue freeze or a new maintenance reservation.
2. **Add per-path mutexes only — rejected.** String path aliases and moves can split lock identity; a single JSON map still permits lost read-modify-write updates across different paths.
3. **Keep global lifecycle lock but normalize conversion rows — incomplete.** It may reduce decode/write work, but does not remove the measured global wait that dominates the issue baseline.
4. **Precompute from an immutable map and commit later — not selected alone.** Without a version check and reload/merge retry it can overwrite a newer edit; with the current single setting row it still serializes writes.

## Required invariants

- Same-asset Save Tracks writes are linearizable and patch against the latest committed state.
- Cross-asset updates both survive and can overlap only when storage supports independent atomic writes.
- Queue/preflight snapshots use exactly one side of the Save Tracks linearization boundary: before the save, or after it; no mixed/stale freeze.
- Scan and maintenance checks remain coupled to active-marker/operation reservation.
- Multi-asset paths use a stable total order; unknown identity uses one exclusive fallback that excludes every keyed and scan/index request.
- Path authorization remains independent from identity resolution; aliases do not gain authorization by entering the fallback.
- Transaction failures and cancellation leave no partial rows, stale lock references, or writes to another asset.

## GO / NO-GO criteria

**GO to implementation review only if:** all deterministic scenarios pass under the Go race detector; every current lock owner is mapped to the same coordination authority; identity/alias/fallback behavior is established; SQLite rollback/backfill/retry and legacy-read behavior are verified; no Queue/lifecycle invariant is relaxed; and a matched five-sample synthetic benchmark improves p95 request latency by ≥25% and p95 lifecycle-lock wait by ≥20%, with p99 regression ≤5%, error count unchanged at zero, and allocations/CPU/DB use within 10% of baseline.

**NO-GO if:** any identity is ambiguous without a shared fallback, one old mutex owner remains outside the new coordinator, transaction/snapshot ordering cannot be proven, race validation cannot run, migration is not idempotent/rollbackable, deterministic tests require sleeps, errors or lost updates appear, or benchmark gains miss the thresholds. Current status is **NO-GO for production lock changes**; this ADR authorizes only the architecture and future test plan.

## Validation evidence for this proposal

- Go: `go1.26.4`, `darwin/amd64`, `CGO_ENABLED=1`, `GOTOOLCHAIN=local`.
- Default-cache command emitted `runtime/race: package testmain: cannot find package`; `go list -race -e -test` identified the sandbox boundary: opening `/Users/anuelvs/Library/Caches/go-build/...` returned `operation not permitted`.
- With an isolated writable cache, `GOCACHE=/private/tmp/mvforge-go-cache-race-415 GOTOOLCHAIN=local go test -race ./internal/handlers -run '^TestAssetTracksPatch' -count=1` passed.
- This is a focused characterization result only; no full race suite, NAS timing, or before/after optimization measurement is claimed.
- Scoped follow-up characterization passed: the three targeted tests (`TestAssetTracksPatchConcurrentSameAssetPreservesPartialUpdates`, `TestAssetPathLifecycleOwnerGuardAndCallInventory`, and `TestAssetRecordPathIdentityDoesNotResolveFilesystemAliases`) passed with and without `-race`, using a writable task-local `GOCACHE` and `GOTOOLCHAIN=local`.
- `openspec validate save-tracks-concurrency-415 --strict` reported the change valid; `skip_specs` is set and zero spec deltas are accepted.
- The canonical `verify.sh --auto` run through `python3 .agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto` passed auto-to-full scope, 6/6 checks, `ALL CHECKS PASSED` after lockfile-faithful frontend dependency installation.
- These checks validate characterization and repository integrity only; they do not close gates 2.1 or 2.2 or authorize production lock/persistence changes.
