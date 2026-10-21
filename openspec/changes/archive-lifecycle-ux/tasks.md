## 1. Backend projection and authority reuse

- [x] 1.1 Extend the existing `AssetLifecycleProjection` with a derived,
  read-only archived-original inspection projection using existing
  `AssetRecord`, `QueueJob`, current-publication, and retention evidence; add no
  archive table or separate inventory API.
- [x] 1.2 Include archive record identity, path, recorded size, and archive
  time from the canonically linked `AssetRecord.CreatedAt`; treat
  `SyncedAt` only as synchronization state and show unknown when `CreatedAt`
  is not reliable. Include provenance IDs, availability, retention
  state/deadline, and relationship state; keep path-only or conflicting
  relations unresolved.
- [x] 1.3 Reuse existing `AssetActionCapabilities`, original-retention
  settings/evidence, and archive-record lifecycle; do not change Publisher/#54
  or retention policy semantics.

## 2. Library filtering

- [x] 2.1 Restrict inventory locations to RAW and Library; remove archive
  groups as inventory rows and remove Archived-only filter state.
- [x] 2.2 Keep Library filters scoped to current Library assets and route
  legacy Archived deep links to Library without rebuilding an archive list.

## 3. Contextual Archived original UX

- [x] 3.1 Show archived-original path, size, archive time, provenance,
  retention state/deadline, availability, and action reasons from the related
  Library asset's Asset Info context.
- [x] 3.2 Present missing, stale/historical, and unresolved evidence without
  relabeling it or attaching it to another asset; do not add an Archived
  inventory table or location.
- [x] 3.3 Show a small Archive availability indicator beside compact Library
  filenames only for current, available projected originals; add no action or
  inventory structure.

## 4. Safe actions

- [x] 4.1 Reuse the existing recover and archive-delete mutations, adding
  archive-record and expected Library-asset identity preconditions to
  contextual requests; keep projected capabilities advisory.
- [x] 4.2 Revalidate current publication/source identity, active work,
  configured archive and RAW roots, path containment, file type/availability,
  destination collision, and current-Library protection at mutation time.
- [x] 4.3 Preserve explicit confirmation for permanent deletion, Library
  publication on restore, and job/publication provenance on delete.

## 5. Focused regressions

- [x] 5.1 Cover RAW/Library-only inventory, removal of Archived filter rows,
  and contextual inspection from the related Library asset.
- [x] 5.2 Cover stable source/publication identity, path reuse, path-only
  legacy evidence, publication conflicts/retirement, and missing/stale archive
  records without arbitrary rebinding.
- [x] 5.3 Cover archive metadata and retention evidence; confirm `CreatedAt`
  is used only for a canonical archive link, later `SyncedAt` changes do not
  change archived time, `ModifiedAt`/`PublishedAt` are not fallbacks, and
  untrusted legacy timestamps remain unknown; verify unchanged retention
  policy behavior.
- [x] 5.4 Cover stale capabilities, changed roots/identities, active work,
  symlink/non-regular/out-of-root paths, RAW traversal/collision, explicit
  delete confirmation, and preservation of the current Library publication.
- [x] 5.5 Cover current+available evidence despite denied restore permission,
  historical/source-replaced, missing, unresolved, absent lifecycle, refresh,
  and RAW rows for the compact Library indicator.

## 6. Final verification

- [x] 6.1 Run focused backend and frontend regressions for projection
  identity, archive metadata/timestamps, filtering, contextual inspection,
  missing/stale records, and safe actions.
- [x] 6.2 Run frontend lint and require 0 errors and 0 warnings.
- [x] 6.3 Run `openspec validate archive-lifecycle-ux --type change --strict`
  and confirm the change artifacts validate.
- [x] 6.4 Run `./scripts/verify.sh --auto` and require the final result
  `ALL CHECKS PASSED`.
- [ ] 6.5 Re-run the canonical verifier for the compact Library indicator and
  require the final result `ALL CHECKS PASSED`.
