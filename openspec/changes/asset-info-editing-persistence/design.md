## Context

See `proposal.md` for the motivation and behavior contract. The current
implementation provides the relevant boundaries:

- `AssetRow` owns `conversionDraft`, but its stream toggles, subtitle sidecar
  and role handlers, Category handler, recommendations, and Reset still call
  separate mutations.
- `AssetConversionOverridePanel` already has an explicit Save/Reset interaction
  for most conversion fields.
- `AssetRow` currently observes `asset.conversion` through an effect that can
  replace local draft state after an inventory refresh, and Queue's `createJob`
  can persist a track-selection draft immediately before creating a job.
- `UpdateMetadata` and `UpdateConversion` read and rewrite whole `entries` maps
  in `assetMetadataOverrides` and `assetConversionOverrides` AppSettings.
- `AssetInventory` repeats `Asset` values in flat status arrays, group asset
  arrays, and nested source-group logical-group paths.

The implementation must remain a focused Asset Info change. It must not split
`AssetsPage.tsx` broadly or change Queue freeze semantics, inventory sync, media
processing, or subtitle operations.

## Goals / Non-Goals

**Goals:**

- Give the Asset Info compound editor one semantic draft, one Save boundary,
  atomic metadata/conversion persistence, and response-based cache reconciliation.
- Reuse the backend's existing conversion validation and normalization instead
  of introducing a parallel conversion contract.
- Preserve local edits across query refreshes and across a Save response that
  belongs to an older draft revision.
- Remove group-wide blocking caused by per-asset conversion mutation state.
- Keep retained compatibility endpoints safe for concurrent AppSetting map writes.

**Non-Goals:**

- Moving ProfileAssignment, AssetScopeConfiguration, review, rename, snapshot,
  subtitle maintenance, Test Encode, or Queue into the compound transaction.
- Durable unsaved drafts, optimistic conflict resolution across separate
  browser sessions, distributed locking, a new database table, or a new state
  management framework.
- Changing the meaning of persisted configuration consumed by Queue or Test
  Encode.

## Decisions

### 1. Model Asset Info as a per-asset edit document

Use a small editor state centered on the normalized asset path:

```text
persisted baseline: { metadata, conversion }
draft:              { metadata, conversion }
dirty:              semanticDifference(baseline, draft)
saving:             captured draft revision, or none
```

The metadata portion contains the asset-level Category and the existing tags
value so a Category save does not erase tags. It does not contain path/group
metadata. The conversion portion is the complete normalized
`AssetConversionOverrideState` exposed by Asset Info, including the existing
Asset Override controls and stream/subtitle override maps.

Use semantic normalizers for both comparison and request construction. Ignore
server timestamps and equivalent empty/ordered representations when determining
dirty state. Keep the backend response as the authoritative normalized form.

Alternative considered: retain separate metadata and conversion drafts and
coordinate two Save calls. Rejected because two calls cannot provide one
caller-visible atomic revision and would preserve the current mismatch.

### 2. Capture revisions and reconcile responses conservatively

When Save begins, capture a normalized immutable request snapshot and associate
it with the current editor revision. The UI sends one request and prevents a
second request while that revision is in flight. Controls remain editable so a
newer local revision can be formed while the request runs.

On success:

1. Treat the response metadata/conversion as the baseline for the captured
   revision.
2. Compare the current draft to the captured request snapshot using semantic
   normalization.
3. If equal, replace the draft with the normalized response and mark clean.
4. If different, retain the newer current draft and leave it dirty; do not
   apply the older response over it.

On failure, retain both the prior baseline and current draft and expose retry.
An inventory prop/query update for the same asset follows the same rule: it can
initialize or refresh a clean editor, but it cannot replace a dirty draft.
Changing the edited asset initializes a new document. Close and asset-switch
entrypoints call a simple confirmation guard before discarding a dirty document.

Alternative considered: reset local state from every `asset` prop change.
Rejected because the existing `persistedConversionSignature` effect already
demonstrates how a broad Assets refresh can clobber active edits.

### 3. Use one compound HTTP contract

Add an Asset Info save operation following repository API conventions:

```http
POST /api/assets/edit?path=<asset path>
```

Request shape:

```json
{
  "metadata": { "categories": ["Drama"], "tags": [] },
  "conversion": { "keepAudioStreams": [0], "subtitleRoleOverrides": {} }
}
```

The conversion object remains the existing conversion input shape; omitted and
empty values retain its current inherit/clear semantics. Successful responses
have this shape:

```json
{
  "path": "/canonical/asset.mkv",
  "metadata": { "categories": ["Drama"], "tags": [] },
  "conversion": { "keepAudioStreams": [0] }
}
```

The frontend API types should make the response usable by the editor and cache
patch helper. The exact public name may follow an established repository
convention, but Asset Info must use one operation and one response rather than
calling the compatibility endpoints sequentially.

### 4. Share normalization and validation with compatibility conversion saves

Extract the pure conversion input-to-state construction and validation currently
performed in `UpdateConversion`. It must continue to handle the existing
upscale/sharpen/custom-height, cadence/field-order, stream-index, subtitle
role/sidecar, encoder, rate-control, frame-structure, and related fields. The
subtitle-role binding behavior that uses the latest scan and existing role
overrides remains part of this shared path.

Extract or reuse the current metadata tag/category normalization as well. The
compound handler performs path resolution and library authorization once,
normalizes both objects, and returns the normalized values actually persisted.
The existing `/api/assets/metadata` and `/api/assets/conversion` handlers remain
available to other callers and call the same reusable domain functions.

Alternative considered: have the compound handler invoke the two HTTP handlers.
Rejected because handler-to-handler calls would duplicate request handling,
make transaction boundaries unclear, and make rollback/error reporting harder.

### 5. Make the two AppSetting maps transactionally safe

Keep `assetMetadataOverrides` and `assetConversionOverrides` as the storage
format. Add one repository-consistent mutation boundary around their
read-modify-write operations:

1. Resolve and authorize the asset path before changing state.
2. Acquire a process-local guard shared by the compound and retained
   compatibility write paths.
3. Run one GORM transaction that loads both current maps through existing
   readers, applies the normalized per-asset update, and saves both maps through
   transaction-aware existing writers.
4. Return the state read from the committed transaction.
5. Release the guard after commit or rollback.

The guard may be an explicitly owned process-local mutex for the current
single-backend architecture. It is not a distributed lock. It prevents two
requests that began with stale full-map copies from losing entries for
different assets, while the database transaction guarantees that a failure in
the second setting write rolls back the first.

Compatibility endpoint writes should use the same guard for their respective
AppSetting map. Their existing request and response semantics remain otherwise
unchanged.

### 6. Patch the existing inventory cache instead of refetching it

Add a pure helper, preferably in a focused frontend utility module, with the
conceptual contract:

```text
patchAssetInventoryAsset(inventory, path, { metadata, conversion })
```

The helper normalizes paths for comparison and returns the original inventory
when no matching Asset exists. When there is a match, it creates only the
necessary changed arrays/objects and updates the matching Asset in:

- `unprocessed`, `library`, `converted`, `unverified`, `accepted`, `archive`,
  and `missing`;
- every `AssetGroup.assets` collection in the corresponding group arrays; and
- every `AssetPath.assets` collection nested under
  `sourceGroups[].logicalGroups[].assetPaths[]`.

Group counts, status, path metadata, review state, and unrelated assets are not
recomputed or changed by a metadata/conversion response. The query success
handler uses `setQueryData(['assets'], ...)`, not normal-path
`invalidateQueries(['assets'])`. Other operations that genuinely change
inventory membership may continue to invalidate the inventory.

### 7. Keep independent operations independent

The Asset Info audit yields this boundary:

| Current surface | Classification | Persistence/side effect |
| --- | --- | --- |
| Per-asset Category and existing asset metadata in the editor | Compound-editor field | Included in compound draft and Save |
| Asset Override fields, stream KEEP/DROP, stream metadata, sidecar formats, subtitle roles | Compound-editor field | Included in compound draft and Save |
| Asset video/audio/Track Profile selectors | Independent operation | Existing ProfileAssignment mutation |
| Asset Destination and path/group Scope Configuration | Independent operation | Existing scope-configuration mutation |
| Manual review toggle/details | Independent operation | Existing review mutation |
| File rename | Independent operation | Existing rename mutation and path reconciliation |
| External subtitle edit/rename/delete, extraction, OCR, translation, track maintenance | Independent operation | Existing subtitle/maintenance operation |
| Snapshot generation/refresh | Independent operation | Existing snapshot operation |
| Test Encode | Independent operation/consumer | Uses persisted effective configuration; unsaved overrides remain excluded |
| Queue and group Queue controls | Independent operation/consumer | Uses persisted configuration; dirty compound editor blocks per-asset Queue |
| Path/group Category and metadata controls | Independent operation | Existing path-scoped metadata mutation |

This avoids making one transaction responsible for unrelated side effects merely
because they share the Asset Info dialog.

### 8. Remove autosave and hidden Queue persistence in the Asset row

The Asset row will route Category, stream selection, sidecar, role,
recommendation, and Reset handlers to the compound draft only. The existing
conversion panel Save becomes the document Save action; its success/error state
is local to that asset editor.

The group-level `useIsMutating` observation for the per-asset track-selection
mutation is removed from unrelated control disabling. Compound Save pending state
is local to the edited asset. Existing `rowLocked` and open-job/converted/archive
read-only rules remain authoritative.

Before `createJob`, check the compound dirty state. A dirty editor displays a
clear “Save first; unsaved Asset Info changes are not queued” warning and does
not call either conversion or compound persistence. A clean editor queues from
the persisted asset/cache state. The current opportunistic
`updateAssetConversion` branch is removed; the existing Test Encode wording is
preserved.

### 9. Keep recommendations and Reset reviewable

`applySnapshotRecommendations` computes the same recommendation patches and
merges them into the draft, but does not call the backend. Its status text
changes to indicate that recommendations are applied to the draft and require
Save. Reset replaces only conversion draft fields with the empty normalized
override and leaves metadata untouched until the same Save action commits it.

## Risks / Trade-offs

- **Risk:** The same asset is represented in several inventory branches and one
  branch could remain stale. **Mitigation:** centralize the pure patch helper and
  test flat, group, and source-group path representations in one fixture.
- **Risk:** The process-local guard does not coordinate separate backend
  processes. **Mitigation:** MVForge's current architecture is single-backend;
  use the database transaction for atomicity and explicitly do not claim
  distributed locking.
- **Risk:** Compatibility callers may depend on the exact old endpoint shape.
  **Mitigation:** retain both endpoints, preserve their request/response
  contracts, and route them through shared normalization and guarded writes.
- **Risk:** A dirty editor can become stale relative to an external writer.
  **Mitigation:** preserve the user's draft deterministically, document that
  #4 does not add cross-session conflict resolution, and let the next ordinary
  inventory refresh establish the new persisted baseline after the editor is
  clean.
- **Risk:** Allowing edits during Save can make the UI look busy. **Mitigation:**
  keep the Save control state local, show saving/error feedback for that asset,
  and test revision-A/revision-B reconciliation explicitly.

## Migration Plan

1. Add shared backend normalization/validation and guarded transactional domain
   persistence without changing existing endpoint contracts.
2. Add the compound route and frontend API types/client method.
3. Convert Asset Info compound controls to the draft/revision model, remove
   autosave and hidden Queue persistence, and add the targeted cache patch.
4. Add focused backend and frontend regressions from `tasks.md`.
5. Validate the OpenSpec change and run the focused implementation checks when
   the change is applied. No database migration, media migration, or sidecar
   migration is required.

Rollback is code-level: remove Asset Info's use of the compound endpoint and
restore the prior frontend path while leaving the retained compatibility
endpoints and existing AppSetting data intact. The compound endpoint adds no
new persisted schema.
