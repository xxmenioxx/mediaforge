## Context

See `proposal.md` for motivation and `specs/assets-v2-raw-library-fidelity/spec.md` for the user-visible contract. The #180 implementation already has `AssetsSummaryStrip`, `LibraryTreePanel`, `LibraryAssetTable`, and `SelectedAssetDetail`; `AssetRow` owns the current Preview, profile assignment, Test Encode, Asset Info, and secondary actions. RAW is still rendered by `UnprocessedAssetsView` and nested `UnprocessedLogicalGroup`/path tables, which own source configuration, title actions, bulk selection, and Batch Preflight entry points. `AssetsPage` owns inventory queries and URL filters. No new backend data or endpoint is required.

## Goals / Non-Goals

**Goals:**

- Put the existing RAW and Library inventories behind one URL-backed location switch.
- Render RAW through a shared compact hierarchy/list/detail frame while retaining source hierarchy and current action owners.
- Keep common search state stable and isolate context-local filter and hierarchy state.
- Tune shared presentation with responsive proportions and restrained density guided by the approved screenshot.

**Non-Goals:**

- Do not change Asset, Source Group, Logical Group, Queue, lifecycle, profile qualification, Advisor, Test Encode, Validation, or Publisher models and decisions.
- Do not fetch inventory, snapshots, previews, or effective configuration per row; do not add filesystem scans or media probes.
- Do not move full Asset Info into the workspace or add metadata providers or workflow tabs.

## Decisions

1. **Use the existing URL location as the context authority.** Derive RAW versus Library from `location=raw|library`, keep legacy tab conversion in `AssetsPage`, and place the accessible switch near the page title. Switching preserves common `search`, writes the destination `location`, clears incompatible Library-only filters when entering RAW, and drops a legacy asset deep-link selection unless its identity can be proven in the destination. RAW hierarchy state remains local to its mounted workspace; Library tree state remains local to `LibraryAssetTable`.

   Alternative: persist source/title/path selections as new query parameters. This would add a second state contract and risk stale path identity without a user requirement.

2. **Project RAW hierarchy from existing source summaries.** Build a collapsed keyboard-operable tree from `AssetSourceGroup.logicalGroups[].assetPaths`. Source, title, and path selections filter existing grouped assets; the selected assets become existing `AssetGroup` path projections for the shared scan list. Keep root assets distinguishable from child paths and retain group identities. No endpoint or filesystem lookup is added.

   Alternative: flatten source assets into the Library tree or keep a title accordion as the only RAW navigation. The first loses source semantics; the second preserves the existing visual divergence.

3. **Reuse current action owners at each RAW scope.** Keep `SourceGroupConfigureButton` for source configuration, `UnprocessedLogicalGroup` title workflows for snapshots/Advisor/publish-as-is/configuration, `AssetGroupRow` for path actions, `UnprocessedSelectionToolbar` for selected Queue/Batch Preflight and group configuration, and `AssetRow` for asset actions. Adapt these surfaces into compact context actions and the shared list rather than reimplementing mutations or Queue planning.

   Alternative: create parallel action hooks in the new workspace. That risks policy drift and would duplicate current transaction, confirmation, and preflight behavior.

4. **Share list/detail presentation, keep inventory models separate.** Reuse the existing compact `AssetRow` and `SelectedAssetDetail` behavior for both inventories. The Library tree remains library-specific; RAW uses its own source hierarchy. Selected detail owns one lazy persisted snapshot request and registers callbacks to the selected row's existing AssetRow actions. Preserve canonical summary fallbacks and hide metadata when absent.

   Alternative: duplicate the selected-detail implementation in RAW or make both inventories share one domain tree model. Duplication risks behavior drift; a shared domain model would erase meaningful source/destination differences.

5. **Tune dimensions with responsive constraints.** Give the hierarchy a larger but bounded share of the desktop workspace, keep the table flexible, and enlarge the selected preview at desktop widths while stacking it with details and actions at narrow widths. Keep compact summaries and subtle dividers; do not use screenshot-resolution pixel coordinates.

   Alternative: set fixed screenshot-sized panels. That would harm laptop and mobile usability.

6. **Bound work to existing grouped inventory and the visible page.** Reuse the one inventory response and current paginated rows. Derive hierarchy nodes from those summaries, keep branches collapsed, batch effective configuration only for the visible RAW page where required, and request deep persisted technical detail only for the selected asset. Do not add per-row preview, snapshot, Test Encode inventory, or probing requests.

## Risks / Trade-offs

- [The RAW source summary can contain many nested paths] → Keep the tree collapsed by default, derive it from existing response data, and paginate the asset table.
- [Existing title and path components contain complex actions] → Reuse their mutation and confirmation owners, changing only their presentation or mounting context; keep focused workflow regressions.
- [Context changes can leave incompatible query parameters behind] → Centralize switching in one location update and test preserved common search plus removed Library-only filters.
- [A larger detail preview can crowd laptop layouts] → Bound it with responsive width/height values and stack it below the desktop breakpoint.

## Migration Plan

No data or backend migration is needed. Keep legacy Assets query aliases working through the existing URL conversion. The UI can be rolled back independently; the `location` query values and inventory authorities remain unchanged.
