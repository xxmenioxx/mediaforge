## Why

Asset Info currently mixes an explicit conversion Save flow with immediate writes
for stream selection, stream metadata, subtitle sidecar formats, subtitle roles,
recommendations, reset, and asset Category. Those writes can overlap, invalidate
the entire Assets inventory, block unrelated controls through a group mutation
key, and leave metadata and conversion changes representing different edit
revisions. Issue #4 makes Asset Info one coherent per-asset editor with an
explicit Save boundary and targeted cache reconciliation.

## What Changes

- Define one compound Asset Info draft containing per-asset metadata Category
  state and the existing Asset Override/conversion state, including stream
  KEEP/DROP, stream metadata, subtitle sidecar formats, and subtitle semantic
  roles.
- Make every compound-editor control draft-only; one explicit Save persists the
  metadata and conversion revision together and reports normalized authoritative
  state back to the editor.
- Add a single backend compound-save operation using the existing AppSetting
  stores, shared normalization/validation, one database transaction, and
  serialized read-modify-write protection for concurrent asset paths.
- Patch the affected asset in all rendered `AssetInventory` representations
  from the Save response instead of invalidating/refetching the whole `['assets']`
  query.
- Preserve dirty drafts across unrelated inventory refreshes, protect newer
  local edits from an older Save response, and confirm before closing or
  switching away from unsaved Asset Info changes.
- Remove Asset Info’s group-wide track-selection mutation blocking and eliminate
  Queue’s hidden pre-Queue conversion write; Queue will use persisted state only
  and require the compound draft to be saved first.
- Keep advisor recommendations and Reset as draft operations that require Save.
- Audit and explicitly preserve independent operations such as profile
  assignments, destination scope, review state, rename, snapshot generation,
  subtitle maintenance, Test Encode, and Queue actions.
- Retain `/api/assets/metadata` and `/api/assets/conversion` as compatibility
  endpoints unless implementation evidence proves they are unused; Asset Info
  will use the authoritative compound endpoint.
- Do not change inventory synchronization, Queue freeze semantics, media
  processing, subtitle extraction/OCR/translation, rename semantics, snapshot
  lifecycle, Preview/LAB behavior, or introduce a new database table.

## Capabilities

### New Capabilities

- `asset-info-editing-persistence`: Defines compound Asset Info draft,
  explicit-save, atomic persistence, queue consistency, targeted cache, and
  independent-operation behavior.

### Modified Capabilities

None.

## Impact

- Frontend: `AssetsPage.tsx`, API types/client, a pure inventory-cache patch
  helper, and focused Asset Info/editor tests.
- Backend: asset routes and handlers, reusable metadata/conversion
  normalization and validation, AppSetting override-store mutation semantics,
  and focused transactional/concurrency tests.
- Existing callers of the metadata and conversion endpoints remain supported;
  no data migration or media mutation is required.
