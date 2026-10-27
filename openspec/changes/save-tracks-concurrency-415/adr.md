# ADR proposal: Safe Save Tracks concurrency boundary

- **Status:** Proposed; design lane only; not implemented.
- **Date:** 2026-10-27.
- **Context:** Issue #415 reports synthetic Save Tracks latency mostly in the global lifecycle mutex. That mutex also orders Queue freeze, scan admission, maintenance reservation, and asset path transitions. A second global mutex protects whole-map JSON override read-modify-write.

## Decision

Do not remove or narrow either production mutex as a Save Tracks-only change. The next implementation candidate must replace the lifecycle mutex coherently across all of its current owners with a shared per-asset coordinator (stable canonical identities, sorted multi-asset acquisition, and an exclusive fallback for unresolved/legacy paths) and replace or safely version the shared conversion-override JSON write path. Preserve current behavior until both coordination domains have an atomic replacement and the test/benchmark gates below pass.

The recommended prototype is a ref-counted keyed coordinator plus transactional normalized conversion-override rows. Queue freeze and lifecycle reservation stay within the same asset lock boundary as Save Tracks. Any global metadata setting lock remains separately ordered and may not acquire the coordinator from inside that lock or a DB transaction.

## Proposed identity contract and current evidence

The future coordinator key is the database namespace plus the stable `AssetRecord.ID`, bound to a unique current path/revision under the coordination boundary. The current `AssetRecord` model has a primary-key ID and unique path but no alias identity; Save Tracks accepts a cleaned path and does not resolve its ID before locking. Therefore unindexed files, unresolved aliases/symlinks, path transitions without verified ID continuity, and historical jobs without a current verified `SourceAssetRecordID` must use one exclusive process-wide fallback. Rename/move must retain the same record ID or stay under that fallback through filesystem and DB rebind. A filename, size, timestamp, or normalized path is not proof of identity. This is a proposed contract only; current request paths do not enforce it and no identity collision/fallback tests have been implemented, so gate 2.1 remains open.

Legacy override backfill may bind a path entry to an `AssetRecord.ID` only when the current path-to-record mapping is unique and validated in the same transaction. Ambiguous/unindexed rows remain legacy and use exclusive fallback. Path owners that do not directly reference `assetMutationMu`—including `PublishAsIs`, `ApplyLibraryPathRename`, and `Rename`—must be audited for enclosing guards and explicitly included in the future coordinator; before future keyed coordination is enabled; that fallback is not implemented in current code.

A source AST inventory now records 16 current direct `assetMutationMu` owners across Save Tracks, Queue, Preflight, scanner, maintenance, and path/lifecycle mutation. Its regression test protects the current shared mutex owner list only. It is not evidence that a future keyed coordinator has been implemented or that all runtime branches have been exercised. Gate 2.2 is complete only for this current-owner inventory; keyed-coordinator migration remains a later gate.

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
- Multi-asset paths use a stable total order; unknown identity uses a coordinated exclusive fallback.
- Transaction failures and cancellation leave no partial rows, stale lock references, or writes to another asset.

## GO / NO-GO criteria

**GO to implementation review only if:** all deterministic scenarios pass under the Go race detector; every current lock owner is mapped to the same coordination authority; identity/alias/fallback behavior is established; SQLite rollback/backfill/retry and legacy-read behavior are verified; no Queue/lifecycle invariant is relaxed; and a matched five-sample synthetic benchmark improves p95 request latency by ≥25% and p95 lifecycle-lock wait by ≥20%, with p99 regression ≤5%, error count unchanged at zero, and allocations/CPU/DB use within 10% of baseline.

**NO-GO if:** any identity is ambiguous without a shared fallback, one old mutex owner remains outside the new coordinator, transaction/snapshot ordering cannot be proven, race validation cannot run, migration is not idempotent/rollbackable, deterministic tests require sleeps, errors or lost updates appear, or benchmark gains miss the thresholds. Current status is **NO-GO for production lock changes**; this ADR authorizes only the architecture and future test plan.

## Validation evidence for this proposal

- Go: `go1.26.4`, `darwin/amd64`, `CGO_ENABLED=1`, `GOTOOLCHAIN=local`.
- Default-cache command emitted `runtime/race: package testmain: cannot find package`; `go list -race -e -test` identified the sandbox boundary: opening `/Users/anuelvs/Library/Caches/go-build/...` returned `operation not permitted`.
- With an isolated writable cache, `GOCACHE=/private/tmp/mvforge-go-cache-race-415 GOTOOLCHAIN=local go test -race ./internal/handlers -run '^TestAssetTracksPatch' -count=1` passed.
- This is a focused characterization result only; no full race suite, NAS timing, or before/after optimization measurement is claimed.
