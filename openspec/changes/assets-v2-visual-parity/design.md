## Context

See `proposal.md` for motivation and `specs/assets-v2-visual-parity/spec.md` for the observable contract. `AssetsPage` owns the URL-backed RAW/Library context and reads the existing inventory summaries. `AssetsWorkspace` contains the summary, hierarchy, and selected-detail presentation. RAW hierarchy nodes already derive from canonical `sourceGroups`; Library nodes derive from existing Library groups. Deeper snapshots are queried for the selected asset. Reports currently renders from the same persisted asset-inventory response, but its sidebar link and active state use an `evidence=reports` query under `/assets`; the router has no `/reports` page.

## Goals / Non-Goals

**Goals:**

- Refine the existing presentation components to match the approved mockup's composition at responsive desktop and narrow widths.
- Keep RAW and Library as separate inventory models with a shared visual language.
- Make selected RAW hierarchy and its filtered list context easy to read.
- Give Reports a pathname-based primary destination and reuse its existing report data and actions.
- Keep inventory work bounded to existing summaries and selected-asset detail.

**Non-Goals:**

- Changing APIs, inventory projections, grouping identity, lifecycle semantics, Queue planning, or action eligibility.
- Adding report calculations, media probes, filesystem scans, dependencies, or a new hierarchy model.
- Redesigning unrelated application navigation surfaces or other product pages.

## Decisions

1. **Refine the existing Assets composition.** Keep the title and RAW/Library selector together, render the four existing canonical summary counts as compact cards, then place the search and current inventory beneath them in a bounded workspace surface. Use theme surfaces, borders, spacing, typography, and restrained accents so the layout scales without screenshot-specific coordinates. Replacing the current low divided summary strip with cards makes the values legible while keeping the header compact.

2. **Preserve separate hierarchy semantics and share their presentation rules.** Continue to project RAW from source groups and Library from Library groups. Improve each tree's panel, indentation, selected/expanded affordance, and long-name handling; derive RAW context text from the selected source group, logical group, and path already in memory. Keep context selection local to each mounted workspace and keep Library's six scan columns, RAW's source-oriented rows, and both selected-detail actions on their existing owners. A single domain tree would erase RAW source identity, while duplicated workspace implementations would drift visually.

3. **Recompose selected detail without changing its data boundary.** Keep the existing preview URL and one integrated Play control, selected snapshot query, canonical row summaries, and action callbacks. Arrange the preview, filename/context, status/profile/subtitle/timing, technical details, and actions into aligned groups that collapse vertically at narrow widths. Do not add per-row requests or inline Asset Info.

4. **Make Reports a pathname-owned route.** Add `/reports` to the application router and point the Reports navigation item there. Determine primary active state from the pathname so Assets remains active for either inventory location and Reports is active only on the Reports route. Reuse the existing inventory query key, report panel, and missing-record mutation flows on the new page. Redirect the supported legacy `evidence=reports` and `tab=reports` URLs to `/reports`; this keeps old links usable without presenting Reports as an Assets context.

5. **Use focused presentation and route regressions.** Extend the existing Assets workspace tests for selector semantics, hierarchy filtering, table columns, detail actions, and bounded selected-only queries. Add route and navigation tests for RAW, Library, legacy report links, `/reports`, and mutually exclusive active states. Keep tests tied to observable roles, labels, URL state, and the absence of per-row work rather than screenshot pixel values.

## Risks / Trade-offs

- **Risk:** Richer cards and detail groups could consume too much vertical space on smaller screens → Keep compact dimensions, use responsive stacking, and retain local table overflow rather than page-level overflow.
- **Risk:** Moving legacy report links can break bookmarks → Redirect the existing report query and legacy tab forms directly to `/reports`.
- **Risk:** Reusing the shared report inventory query can make a first Reports visit wait for the current asset API → Show the existing loading/error state and do not add a report-specific scan or endpoint.
- **Risk:** Visual changes can obscure context or status if styling is relied on alone → Retain text labels, selected/expanded ARIA state, visible focus, and status wording.

## Migration Plan

No data migration is required. Deploy the frontend route and presentation changes together. Old report-view links redirect to `/reports`; rollback restores the previous frontend bundle and does not change persisted inventory or report data.
