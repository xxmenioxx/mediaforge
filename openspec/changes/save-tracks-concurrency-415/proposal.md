## Why

Issue #415 reports Save Tracks latency dominated by the process-wide `assetMutationMu` wait under a synthetic burst. That lock also coordinates Queue, scan, and maintenance lifecycle boundaries, while a second process-wide lock protects read-modify-write updates to the shared JSON override setting. Removing either lock without replacing both authorities can admit a stale Queue snapshot or lose another asset's update.

## What Changes

- Specify a safe concurrency architecture and deterministic verification plan for Save Tracks.
- Characterize current lock ownership and the two global serialization domains.
- Define a gated future design for per-asset lifecycle coordination plus transactional per-asset override persistence.
- Record benchmark, race-toolchain, compatibility, migration, and GO/NO-GO criteria.
- Keep this change at design stage; do not change production locking, persistence, or lifecycle behavior.

## Capabilities

This is an architecture design with no intended observable behavior change, so it intentionally has no spec delta (`skip_specs: true`).

## Impact

- Adds an OpenSpec SDD, proposed ADR, and one same-asset concurrency characterization test.
- No production code, API, persisted model, media, Queue, or lifecycle behavior changes.
- Synthetic tests use disposable SQLite and generated files. No NAS, production DB, or real media is used.
