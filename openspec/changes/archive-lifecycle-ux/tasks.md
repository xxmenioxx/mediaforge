## 1. Backend projection and authority reuse

- [ ] 1.1 Extend the existing `AssetLifecycleProjection` with a derived,
  read-only archived-original inspection projection using existing
  `AssetRecord`, `QueueJob`, current-publication, and retention evidence; add no
  archive table or separate inventory API.
- [ ] 1.2 Include archive record identity, path, recorded size/time,
  provenance IDs, availability, retention state/deadline, and relationship
  state; keep path-only or conflicting relations unresolved.
- [ ] 1.3 Reuse existing `AssetActionCapabilities`, original-retention
  settings/evidence, and archive-record lifecycle; do not change Publisher/#54
  or retention policy semantics.

## 2. Library filtering

- [ ] 2.1 Restrict inventory locations to RAW and Library; remove archive
  groups as inventory rows and remove Archived-only filter state.
- [ ] 2.2 Keep Library filters scoped to current Library assets and route
  legacy Archived deep links to Library without rebuilding an archive list.

## 3. Contextual Archived original UX

- [ ] 3.1 Show archived-original path, size, archive time, provenance,
  retention state/deadline, availability, and action reasons from the related
  Library asset's Asset Info context.
- [ ] 3.2 Present missing, stale/historical, and unresolved evidence without
  relabeling it or attaching it to another asset; do not add an Archived
  inventory table or location.

## 4. Safe actions

- [ ] 4.1 Reuse the existing recover and archive-delete mutations, adding
  archive-record and expected Library-asset identity preconditions to
  contextual requests; keep projected capabilities advisory.
- [ ] 4.2 Revalidate current publication/source identity, active work,
  configured archive and RAW roots, path containment, file type/availability,
  destination collision, and current-Library protection at mutation time.
- [ ] 4.3 Preserve explicit confirmation for permanent deletion, Library
  publication on restore, and job/publication provenance on delete.

## 5. Focused regressions

- [ ] 5.1 Cover RAW/Library-only inventory, removal of Archived filter rows,
  and contextual inspection from the related Library asset.
- [ ] 5.2 Cover stable source/publication identity, path reuse, path-only
  legacy evidence, publication conflicts/retirement, and missing/stale archive
  records without arbitrary rebinding.
- [ ] 5.3 Cover archive metadata and retention evidence, unknown timestamps or
  deadlines, and unchanged retention-policy behavior.
- [ ] 5.4 Cover stale capabilities, changed roots/identities, active work,
  symlink/non-regular/out-of-root paths, RAW traversal/collision, explicit
  delete confirmation, and preservation of the current Library publication.

## 6. Final verification

- [ ] 6.1 Run `openspec validate archive-lifecycle-ux --type change --strict`
  and confirm the change artifacts validate.
- [ ] 6.2 After implementation is authorized, run the requested focused
  regressions and report any remaining limitations.
