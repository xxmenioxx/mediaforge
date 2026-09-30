## Context

See `proposal.md` for the motivation and `specs/test-encode-lifecycle/spec.md` for the behavior contract.

The current `models.TestEncode` row is already the canonical persisted run and per-asset history. It records source identity, Library ID, sample window, resolved configuration, execution status, Keep/expiration, output and temporary paths, output size, and subtitle-artifact JSON. `ListTestEncodes` can filter by source path; the shared TestEncode dialog is used in Asset Info and LAB. There is no Test Encode artifact-owner model or persisted filesystem reconciliation state. Compatible Preview has a separate cache-artifact model for its own lifecycle and is not an owner of Test Encode files.

Test Encode statuses are `waiting`, `generating`, `ready`, `failed`, `canceled`, and `deleted`. Active cancellation ownership is an in-process map; startup recovery currently marks interrupted runs Failed. `KeepTestEncode` clears expiration when kept and restores a retention expiration when released. Housekeeping already owns automatic cleanup but queries every expired non-Keep Test Encode at once. Test Encode output is planned within the `Library.DestinationPath` bound to the run, and successful output triggers the existing Library refresh.

Settings persist as `AppSetting` JSON values. The #36 domain foundation provides a canonical settings registry and transactional batch update path. The UI has query-addressable domains; Test Encode retention currently appears inside the Operations housekeeping controls and can also be read from the legacy `housekeeping` value.

## Goals / Non-Goals

**Goals:**

- Extend the existing TestEncode run record and its artifact ownership as one canonical lifecycle, retaining execution status and historical runs.
- Make inventory, reconciliation, explicit cleanup, and automatic retention bounded and safe on NAS storage.
- Keep existing Test Encode planning, output placement, Jellyfin refresh, and shared Asset Info/LAB request flow.
- Put Test Encode-owned defaults and Test Runs in the existing Settings architecture.

**Non-Goals:**

- No second Queue, scheduler, Test Encode execution state machine, LAB history, or Profile/configuration model.
- No changes to encoder/profile resolution, FFmpeg planning, Queue/Worker contracts, or Preview cache ownership.
- No recursive RAW or Library scans, media probing during reconciliation, or unrelated media/sidecar cleanup.
- No storage dashboard, LAB V2, or new Test Encode output root that would change current Library/Jellyfin behavior.

## Decisions

### Extend TestEncode ownership with a child artifact record

Add a `TestEncodeArtifact` child model keyed by Test Encode ID and Library ID, with exact path, artifact type, creation time, and known size. It extends the existing TestEncode artifact contract; it is not a second run inventory. Keep `OutputPath`, `TemporaryPath`, and `SubtitleArtifacts` as compatibility fields and backfill them into the child records. The child rows retain ownership evidence for tombstoned/deleted runs and allow a missing parent run to be reported as an orphan. Do not cascade-delete artifact records when a run is removed.

Alternative considered: keep adding fields to subtitle JSON. That representation cannot reliably express media, temporary, and other artifact roles or orphan ownership. The child relation is a normalized extension of the current canonical run, while the `TestEncode` row remains the only run inventory.

### Preserve the configured Library-bound output root

Resolve the cleanup root from the persisted run's `LibraryID` and that Library's configured `DestinationPath`. This preserves current output planning and Library refresh behavior. Persisted artifact paths are the only candidates; reconciliation does not walk that root. Cleanup also checks source identity, current `AssetRecord` paths, active references, traversal, and resolved parent containment. It removes individual files only. A legacy path that fails these checks stays untouched and reports a cleanup/reconciliation failure.

Alternative considered: moving all new outputs to the work storage role would isolate Test Encodes but would change the existing output path contract and Library refresh behavior. It is excluded for this issue.

### Keep execution and reconciliation state separate

Leave existing `TestEncode.Status` values and transitions intact. Add persisted reconciliation state, timestamp, and message fields, plus response-only output-exists, artifact-count, and known-size summaries. Reconciliation loads a bounded set of TestEncode rows, their artifact rows, and proven orphan artifact rows, then uses bounded `stat` calls on exact paths. It never probes media. The active operation registry protects active runs; an active run is skipped, and an inactive persisted active-status run is only marked Interrupted/Stale after the stale grace interval.

An artifact row without an active TestEncode parent is strong MVForge ownership evidence for an orphan. Files that have no artifact row cannot be discovered or deleted, since a filename is not ownership evidence and this change does not scan Library contents.

### Reuse Settings and housekeeping

Use the existing `testEncodePolicy` setting key for default sample duration/start mode, default Keep/Temporary retention, and completed versus Failed/Cancelled retention. Register it with the #36 settings domain/validation registry and save through the existing transaction path. Read the legacy Test Encode retention nested in `housekeeping` during migration and as compatibility fallback; remove the Test Encode field from Operations UI after the new domain is available.

Use `RunHousekeeping` and its existing `AutoHousekeeper`; limit eligible Test Encode cleanup to a fixed small batch. Keep and active runs remain excluded. Explicit actions can delete Keep runs only after confirmation and the same backend ownership checks.

### Keep the canonical inventory across UI surfaces

Extend the existing Test Encode list endpoint with bounded filtering and inventory summaries while preserving source-path history calls. Settings Test Runs and the existing shared TestEncode dialog consume those same rows. Bulk deletion confirmation is based on the selected canonical rows and their persisted artifact sizes; each backend delete revalidates all paths and ownership. Add only the minimal orphan-artifact view/action needed to clean artifacts whose persisted owner row is absent or deleted.

### Additive migration and backfill

Add the artifact table and reconciliation columns through `database.Migrate`. Run an idempotent backfill from each row's output path, temporary path, and subtitle-artifact JSON; preserve row IDs, statuses, Keep values, expirations, and files. Seed/normalize `testEncodePolicy` from existing legacy retention where the new key is absent, otherwise preserve the new key. Regression tests use disposable SQLite databases with historical ready/kept and deleted records. No filesystem move or cleanup runs as part of migration.

## Risks / Trade-offs

- **Test Encode artifacts share a configured Library root** → Treat only exact persisted artifact rows as candidates; reject paths matching a source or current AssetRecord; never scan or recursively delete Library content.
- **Filesystem deletion and SQLite updates cannot commit atomically** → Remove one exact file at a time, preserve failed ownership rows, record Cleanup Failed, and make retries idempotent.
- **Legacy paths may already be missing, escaped, or indexed as Library assets** → Backfill their ownership evidence without deleting files; report findings and require explicit remediation for paths that fail current safety checks.
- **A large history could make inventory expensive** → Bound pages and reconciliation batches, reuse persisted sizes, and stat only the current bounded set.
- **Housekeeping policy changes can affect space use** → Preserve existing 24-hour Temporary behavior by default, choose a shorter conservative Failed/Cancelled retention, and never automatically delete Keep or active runs.

## Migration Plan

1. Auto-migrate the TestEncodeArtifact table and reconciliation fields.
2. Idempotently backfill each existing run's output, temporary, and subtitle paths, and migrate legacy retention into `testEncodePolicy` only when that key is absent.
3. Deploy backend inventory, reconciliation, and cleanup using the existing Library-bound roots; no files move during migration.
4. Deploy Settings and shared UI consumers over the canonical inventory. Keep old source-path list behavior during rollout.
5. If application code is rolled back, additive schema/data remain readable by the prior version; prior cleanup behavior may ignore the child artifact records. No destructive schema rollback is required.

## Open Questions

None. The current Library-bound output location is retained to preserve the observed Test Encode/Jellyfin behavior.
