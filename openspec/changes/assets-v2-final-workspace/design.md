## Context

The visual audit for this correction pass used the current authenticated Figma frame at node 2:1874 and compared it with the live MediaForge workspace. The approved local PNG path is `docs/design-reference/assets-v2/assetsv2.png`, but that file is absent in this checkout; the existing `asstesv2.png` is a stale differently spelled reference and is not treated as authority. Earlier visual assumptions are discarded where they conflict with the current Figma frame.

Keep the existing React + MUI architecture and all canonical API, inventory, hierarchy-selection, action, and lifecycle owners. The Figma export supplies visual reference and icon geometry only; do not copy its framework, generated dependency tree, application architecture, demo data, or model.

The current `Asset` contract and selected-detail caller expose no supported per-asset Jellyfin artwork or MVForge thumbnail URL. The selected workspace therefore keeps the generic placeholder. Do not use the unrelated Figma demo poster, source-preview playback as artwork, or generated artwork.

## Goals / Non-Goals

**Goals:**

- Match the current full-width Assets/Search top row and the hierarchy/right-workspace composition below it.
- Present RAW and Libraries as the two visible hierarchy roots while keeping configured Library roots grouped below a presentation-only Libraries node.
- Match current tree proportions, compact controls, dark navy surfaces, purple selection accents, compact rows, and selected-asset grouping.
- Preserve one shared RAW/Library table and every existing behavior and capability owner.
- Keep the selected context below the independently scrolling table and collapsible.
- Keep Legacy Analysis Decisions in Assets Settings while retaining its historical records.

**Non-Goals:**

- Change RAW/Library identity, table schemas, pagination, lifecycle, backend eligibility, Queue, Advisor scoring, profile resolution, Test Encode, validation, Publisher, or metadata enrichment.
- Add a safe sorting operation, account/notification contract, storage-capacity reporting, trend metrics, artwork API, or RAW path rename.
- Copy the Figma Make app, dependency tree, or data model.

## Decisions

1. **Use the current page composition.** Put Assets, compact Search, and the supported Settings action in a full-width top row. Start the two-column workspace below it. Keep the Asset Library hierarchy on the left; place summary cards, one breadcrumb/path-action bar, the inventory control row, the table scroll region, and selected detail in the right column.

2. **Use two visible hierarchy sections without changing identities.** Show RAW and Libraries as level-one items; place configured destination roots beneath Libraries. Keep the wrapper presentation-only. Default width is 262px, clamped from 220px through 390px and persisted locally. The plus icon links to the existing `/libraries` route.

3. **Keep the summary truthful.** Show Total Assets, Processing, Completed, and Needs Review using existing inventory/lifecycle counts. Reuse the existing filter/query state in a compact Filter popover. The control strip shows count, Sync, and Filter; Sort is disabled because current safe sorting is unsupported. Do not reproduce mock trends, storage percentages, avatar, or notification state.

4. **Join breadcrumb and path actions visually while retaining their existing owners.** Keep routine operations together and place supported destructive actions after a divider. For Library paths, retain Scan, Queue/Preflight, Move path, Rename path, Delete selected, and Delete path when applicable. For RAW scopes, expose only actions with current owners, including supported Scan, explicit Advisor, Queue/Preflight, Configure Path, and Move to library at eligible path levels. Reuse `AssetGroupRow` handlers and its lifecycle-safe delete/return flow. There is no separate second Library “Move to library” API action, and RAW path Rename has no supported owner; do not invent either behavior.

5. **Match the compact row surfaces and selection.** Keep mode-specific schemas, filename-only Asset cells, table-local horizontal overflow, thin borders, and the current subtle 3px vertical row separation. Use the navy row surface, restrained hover, purple active-detail selection, and one row overflow menu. The header checkbox selects only visible eligible rows and reflects checked, mixed, and disabled states. Library row selection is available only when the existing lifecycle-safe delete/return capability allows it. The Advisor score remains canonical and is paired with a short subtle indicator; not-evaluated/off/unavailable states have no indicator.

6. **Keep the list and selected detail as separate vertical regions.** Summary, breadcrumb/actions, and controls stay outside the inventory scroll region. Rows scroll independently with reserved bottom padding. Selected detail is a sibling below the scroll container and remains attached to the bottom of the right workspace; it is never a modal or persistent right panel.

7. **Match the current selected-detail frame while retaining issue #208 content.** Use the observed 4:3 preview at left with centered Play overlay and duration badge. To its right, separate a filename/path identity row from the compact metadata strip and broad MEDIA block so each group uses the available horizontal width. Place the state pill and Assign Profile, Test Encode, and More controls beside the identity, routing them through the selected row's registered action owners and existing capability/eligibility rules. Keep category/library and container facts distinct from Perfil, Subtítulos, and Audio. The bordered MEDIA region uses a responsive multi-column grid for Video (codec and resolution together), Encoder, Duration, Bitrate when present in canonical asset technical evidence, Range, Size, and Modified; never repeat codec or resolution in quick facts. Do not invent unavailable values. Play remains integrated into the artwork. Let content determine expanded panel height, keep the panel collapsible with a subtle control and compact reopen state, and reflow metadata and technical grids at smaller widths.

8. **Use only real artwork sources.** Current backend/frontend contracts provide no supported per-asset artwork URL. Keep the generic placeholder. A future supported source should use Jellyfin artwork, then an existing MVForge thumbnail, then placeholder, but this fallback is not implemented until the real source reaches the caller. Do not copy the Figma poster into the product or generate artwork.

9. **Move legacy analysis history without changing records.** Keep `LegacyAnalysisRecords` out of the main Assets page and render the same saved records under the Assets Settings section.

10. **Adapt iconography and branding inside MUI.** Keep existing MUI icons when they match the Figma treatment and render supplied path-action icons. Use a purple rounded-square mark with a white outlined Play glyph in the navigation shell; remove the old CSS triangle. Preserve route behavior, desktop hover expansion, and mobile drawer. The current frame has no persistent pin/unpin control.

11. **Do not expand unsupported visual behavior.** Figma account/avatar and notifications lack current owners; trend and storage-capacity values are not canonical; safe Sort is unavailable; a distinct second Library migration and RAW path rename are unsupported; not every RAW source/title scope owns all path actions. Omit or disable those controls and report these gaps.

12. **Verify through structural/accessibility regressions.** Test eligible Library row checkboxes, visible-page select-all, mixed/disabled states, separation between active detail and bulk selection, lifecycle-safe deletion targets, current detail fields/aspect ratio/collapse, path icons, RAW header selection, branding, and supported action availability. Do not assert screenshot coordinates or unsupported demo data.

## Risks / Trade-offs

- [The design reference includes demo-only data] → Display only canonical counts and supported current state.
- [No supported per-asset artwork source reaches the caller] → Use the generic placeholder and keep artwork integration as a follow-up.
- [Figma presents actions that are not available at every RAW/Library scope] → Show only operations owned by current flows and gated by their current capability rules.
- [Dense table columns exceed narrow widths] → Keep horizontal overflow in the table and reflow hierarchy/detail without growing document width.
- [Stored hierarchy preference is stale or malformed] → Clamp to the current 220–390px range and retain accessible keyboard resizing.

## Migration Plan

No backend migration or endpoint is planned. Preserve existing Assets query, action, and record contracts. Persist only the bounded local hierarchy width. Reverting this visual pass does not change media, inventory, historical analysis, or lifecycle data.
