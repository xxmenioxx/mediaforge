## Context

See `proposal.md` for motivation and `specs/assets-v2-workspace/spec.md` for the user-visible contract.

The current application shell is `AppLayout`, which owns the desktop hover-expanded rail and temporary mobile drawer. `AssetsPage` owns query-string filters, the inventory and Queue queries, sync, and the Unprocessed/Library split. Unprocessed currently composes Source Group → logical title → path → AssetRow. Library currently composes filter controls → paginated AssetTable group rows → expandable AssetGroupRow path controls and AssetRow file rows. AssetRow owns lazy Asset Info, Preview, profile assignment, Advisor, Queue, and the existing TestEncodeDialog. Existing asset and group responses already carry inventory counts, file summaries, metadata state, and persisted lifecycle evidence.

## Goals / Non-Goals

**Goals:**

- Give the Library view a compact hierarchy/list composition while retaining the current Unprocessed workflow.
- Keep existing URL filters and API-backed mutation semantics authoritative.
- Mount a bounded page of asset rows and request persisted technical detail only for the selected asset.
- Keep path and asset action ownership in the existing components where that avoids duplicating workflow logic.

**Non-Goals:**

- No backend endpoint, inventory model, profile resolver, Queue planner, Advisor policy, Preview player, metadata provider, or Test Encode lifecycle changes.
- No redesign of Dashboard, Queue, LAB, Settings, Validation, or Publisher.
- No Storage panel or new mobile application layout.

## Decisions

1. **Keep page state and API ownership in AssetsPage.** The page continues to own inventory filters, canonical counts, sync, and query invalidation. The Library tree derives folders from the already-loaded `AssetGroup` summaries; it does not introduce a second backend inventory or fetch per folder. A local folder selection filters the already-authoritative groups and does not rewrite unrelated URL filter state. Alternative: add a folder endpoint or persist another URL filter; neither is needed to render the current inventory.

2. **Split presentation from the existing Unprocessed flow.** The target hierarchy is:

   ```text
   AppLayout
   └── AssetsPage
       ├── CompactAssetsHeader
       ├── Existing filter and sync controls
       ├── UnprocessedAssetsView (existing hierarchy and workflows)
       └── LibraryWorkspace
           ├── LibraryTreePanel
           └── LibraryAssetsPanel
               ├── PathActions
               ├── AssetsTable
               └── SelectedAssetDetail
   ```

   The existing URL-driven provenance, search, media-area, validation, original, workflow, attention, and report filters remain intact. The Assets page remains the same application route. Alternative: replace the entire page and both inventory contexts; that would risk unrelated workflow behavior.

3. **Reuse existing row/path workflow owners.** The Library asset table presents the requested six scan columns and mounts only the current page of rows. Existing AssetRow mutation handlers and dialogs remain the entry points for Preview, Assign Profile, Test Encode, Asset Info, Queue, Advisor, and lifecycle actions. The selected detail strip receives callbacks to those existing actions. A compact selected-path surface reuses the current path group actions; rarely used configuration and migration controls remain in an overflow menu or expandable path detail. Alternative: move each action into new hooks or duplicate API calls in the presentation layer; that increases regression risk and can diverge from canonical behavior.

4. **Use only persisted, available summary evidence.** Header widgets use `AssetInventory.counts` with current response arrays as fallback. Technical and subtitle summaries use `Asset.technical` or the selected asset's latest persisted snapshot; unknown values remain visibly unavailable. No row performs ffprobe, a per-row snapshot request, or a thumbnail request. Descriptive metadata renders only when the selected asset has canonical metadata; NFO sidecars and deeper evidence remain in the existing selected Asset Info flow. Alternative: fetch deep detail for every row; that creates unbounded request work for large libraries.

5. **Build an accessible lightweight tree from group paths.** The tree is a presentation projection over existing Library names and group relative paths. It uses standard buttons/list semantics, explicit expanded/selected state, and collapsed branches instead of a new tree framework. At narrow widths, the tree and list stack while the page keeps `minWidth: 0` and filenames remain truncated. Alternative: add a new tree dependency or scan the filesystem; neither is justified by existing data.

6. **Preserve the application shell and expose existing routes.** Keep the current collapsed icon rail and mobile drawer. Add only missing links for existing Reports and Logs destinations when needed, and preserve Assets selection for normal Assets routes. Do not add a navigation framework or pinning state that the current shell does not support.

## Risks / Trade-offs

- [Some existing API rows do not contain subtitle or media summaries] → Show unavailable values in the list and load only the selected asset's persisted snapshot for the detail strip.
- [A tree derived from groups may include many nodes] → Reuse the already-loaded summary response, render collapsed branches, and paginate only the visible asset table.
- [Path actions currently live inside AssetGroupRow] → Keep that component as the action owner and adapt its compact selected-path presentation rather than reimplementing Queue or migration logic.
- [Refactoring the large AssetsPage may disturb existing behaviors] → Extract focused presentation components, leave Unprocessed composition intact, and run existing plus new focused Assets tests before canonical verification.

## Migration Plan

No data migration or backend rollout is required. Ship the Library presentation on the existing Assets route and query contracts. Reverting the frontend commit restores the previous presentation without touching inventory, Queue, profile, metadata, or lifecycle data.
