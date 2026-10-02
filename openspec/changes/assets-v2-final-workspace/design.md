## Context

See [proposal.md](proposal.md) for motivation and [specs/assets-workspace/spec.md](specs/assets-workspace/spec.md) for the observable contract. The current Assets route is owned by `AssetsPage`; `AssetsHierarchyPanel` in `AssetsWorkspace.tsx` already renders one presentation tree over separate RAW and Library projections. `SelectedAssetDetail` already receives persisted selected-asset summary data and callbacks for Preview, Assign Profile, and Test Encode. Keep those canonical owners and extend the existing React + MUI workspace.

The prior `assets-v2-workspace`, `assets-v2-raw-library-fidelity`, and `assets-v2-visual-parity` changes are completed planning inputs for Library density, shared RAW/Library navigation, and the unified tree. They remain historical artifacts; this change adds the final column, evidence, action, resize, and selected-workspace contract in a new `assets-workspace` capability because the main OpenSpec spec inventory contains no Assets capability.

The visual references are local working files under `docs/design-reference/assets-v2/figma-make/` and `docs/design-reference/assets-v2/asstesv2.png` (the image's existing filename is misspelled). Use the Figma Make reference for visual hierarchy, compact rows, selected state, toolbar separation, and responsive composition. Use the image for the selected workspace's landscape preview, centered Play overlay, duration badge, identity/context, technical summary, and actions. Translate those cues into existing MUI components; do not copy the reference app's architecture or dependencies.

The current frontend `Asset` inventory type has persisted technical summaries but no artwork field. A targeted search of the frontend API and backend handlers found no existing artwork or thumbnail contract. Artwork sources therefore remain conditional on data already exposed by existing supported paths; this design does not assume a new Jellyfin or media-artwork endpoint.

## Goals / Non-Goals

**Goals:**

- Complete one responsive RAW/Library workspace using the current Assets route, inventory, source/library identity, and shared presentation.
- Provide compact mode-specific technical tables, truthful cached Advisor evidence, and separate path and asset action surfaces.
- Make hierarchy resizing accessible and persist its bounded width locally.
- Keep selected deep detail lazy and visually below the table.

**Non-Goals:**

- Add a second Assets page, separate RAW/Library tabs, synthetic inventory roots, API endpoints, dependencies, or backend eligibility rules.
- Change Queue execution, Advisor scoring, profile resolution, Test Encode, validation, Publisher, metadata enrichment, lifecycle models, or media processing.
- Generate artwork, previews, snapshots, or row-level media analysis.

## Decisions

1. **Keep one route and canonical tree over distinct inventory projections.** `AssetsPage` remains the owner of current query, inventory, selection, and mutation state. The shared hierarchy presents the existing RAW source-group/logical-group/path projection and configured Library/path projection as sibling roots. Use their existing identities and context selection to filter the shared inventory surface. Do not add tabs, parallel hierarchy state, or roots for workflow/evidence destinations. This extends the existing `AssetsHierarchyPanel` composition and avoids another context authority. Alternative: duplicate the page by mode or flatten RAW into Library identity; either would create divergent navigation and lose source semantics.

2. **Use mode configuration for the table while keeping row identity shared.** Render the Library and RAW column sets from the same inventory-table surface. Use the existing persisted `Asset` fields and canonical summaries; missing encoder, bitrate, HDR, audio, or other detail remains unavailable rather than prompting per-row enrichment. The Asset cell renders only `fileName`; selected tree and breadcrumb context carries path. Keep all row actions behind one labeled overflow menu, and route each to the existing action owner. Alternative: create separate tables and action hooks; that would repeat behavior and widen the divergence between RAW and Library.

3. **Keep path workflows in their path owner and asset workflows in row/detail owners.** Put path actions, selected asset count, and path context in a toolbar adjacent to the table. Choose its Scan, Advisor, Queue/Preflight, profile, configuration, move, rename, and delete actions by the current mode and existing capabilities. Keep destructive path controls visually apart. Row overflow and selected-detail callbacks enter existing asset actions and dialogs. Queue continues through existing preflight/planning and confirmation; the UI does not compute action eligibility independently. Alternative: put every action in each row or create a new generic mutation layer; both obscure scope and risk changing lifecycle behavior.

4. **Treat Advisor as persisted evidence.** Use `AdvisorResponse.score` only when the response is already available from an existing cache/result path. Show unavailable or not evaluated when it is absent, without claiming a score. A path-level Advisor command remains an explicit user operation that can refresh existing results. Never evaluate Advisor during row rendering, sorting, pagination, or tree selection. Alternative: invoke Advisor per row for richer columns; this violates the inventory performance boundary and makes a browse action perform analysis.

5. **Add resizing as local presentation state.** Keep width in a local preference with a 300px default and 220px/420px bounds. Clamp valid numeric values on restore and use the default if the value is malformed. Expose an accessible vertical separator with keyboard resizing and visible focus while preserving tree focus/selection. At narrow widths, allow the hierarchy and list to stack or reflow; do not let the saved desktop width force page-level overflow. Alternative: persist width in backend user settings or URL state; neither is needed for a local layout preference and both add another state contract.

6. **Extend the existing selected-detail boundary below the table.** Keep `SelectedAssetDetail` below the table and collapsible. Reuse its selected asset, persisted snapshot, and current action callbacks; load deeper persisted detail for the selected asset only. Arrange the preview/artwork, identity/context, status, technical summary, and actions to match the reference composition, with responsive stacking. Use the required Jellyfin → existing MVForge artwork/thumbnail → generic placeholder priority only for sources already available through existing data/API. The current inventory `Asset` type and targeted API/handler search expose no artwork contract, so do not invent an endpoint or generate artwork to satisfy the first two fallbacks. When neither source is exposed, use the placeholder and report Jellyfin artwork integration as a follow-up. Alternative: per-row image requests or media generation; those create unbounded work and are explicitly out of scope.

7. **Preserve status and eligibility authority.** Map RAW labels from canonical source, review, profile, and lifecycle/Queue evidence already available. Do not derive a new lifecycle state from missing data or UI inference. For actions whose eligibility is not present in the current inventory response, preserve the existing backend check, preflight, and disabled-reason behavior rather than adding row-specific checks. Alternative: infer `Ready for Queue` from absence of errors or asset status alone; that can disagree with backend policy.

8. **Verify observable behavior through focused UI regressions.** Add tests for mode/context rendering, exact column sets and filename-only cells, cached/missing Advisor states without render evaluation, row overflow versus path actions, hierarchy width restoration/range/keyboard operation, selected detail placement/collapse, artwork fallback, action eligibility, responsive accessibility, and bounded requests. Then run the requested frontend lint/build, strict OpenSpec validation, and canonical repository verifier. Tests should assert roles, labels, column content/order, state, requests, and keyboard behavior rather than screenshot-specific pixel coordinates.

## Risks / Trade-offs

- [Some technical or Advisor evidence is absent from current inventory responses] → Show unavailable/not-evaluated values and retain selected-only detail loading; do not add per-row analysis.
- [Jellyfin artwork and MVForge thumbnail sources are not exposed in the inspected current Asset/API contract] → Use only an existing supported source if implementation discovers one; otherwise use the generic placeholder and report integration as a follow-up.
- [Local width storage can be stale or malformed] → Clamp supported numeric widths, fall back to 300px on unreadable state, and keep keyboard resizing available.
- [Dense columns may exceed a narrow viewport] → Keep table scrolling local to the inventory region, reflow the hierarchy and selected workspace, and retain page-level width constraints.
- [The Figma Make reference may depict workflow behavior outside existing capability contracts] → Treat it as visual guidance only and keep unsupported actions out of the product flow.

## Migration Plan

No server-side data migration or API rollout is planned. Store only the bounded hierarchy width as a local presentation preference. Keep the existing Assets route, inventory query contracts, context identity, and action owners. Reverting the frontend change removes the local preference use and restores the prior workspace without changing media or lifecycle data.
