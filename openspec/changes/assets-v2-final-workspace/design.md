## Context

The visual audit for this correction pass used the exact current Figma Make file [Update Selected Asset Workspace](https://www.figma.com/make/SPURUawBZid2U8AjG95bsn/Update-Selected-Asset-Workspace?fullscreen=1&t=19SWEXKGPLmIkECD-1&code-node-id=0-6), at code node `0-6`, and its matching local composition image `docs/design-reference/assets-v2/asstesv2.png`. The file's `asstesv2.png` spelling is intentional for this checkout; it was opened and compared with the Figma page. The localhost workspace was also compared before editing and reloaded for post-change review. After reload, its storage footer and empty RAW/Library shells were visible, but inventory data had disappeared, so the live page could not confirm row or selected-detail rendering in that post-change review. The local design image and Figma page remain the composition authorities. Earlier visual assumptions are discarded where they conflict with these current references.

Keep the existing React + MUI architecture and all canonical API, inventory, hierarchy-selection, action, and lifecycle owners. The Figma export supplies visual reference and icon geometry only; do not copy its framework, generated dependency tree, application architecture, demo data, or model.

The current `Asset` contract and selected-detail caller expose no supported per-asset Jellyfin artwork or MVForge thumbnail URL. The selected workspace therefore keeps the generic placeholder. Do not use the unrelated Figma demo poster, source-preview playback as artwork, or generated artwork.

## Goals / Non-Goals

**Goals:**

- Match the current full-width Assets/Search top row and the hierarchy/right-workspace composition below it.
- Present RAW and Libraries as the two visible hierarchy roots while keeping configured Library roots grouped below a presentation-only Libraries node.
- Match current tree proportions, compact controls, dark navy surfaces, purple selection accents, compact rows, and selected-asset grouping.
- Keep the existing storage summary at the bottom of the hierarchy in both modes, using the shared RuntimeState query and preserving its loading, error, and unavailable states.
- Preserve one shared RAW/Library table and every existing behavior and capability owner.
- Keep the selected context below the independently scrolling table and collapsible.
- Keep Legacy Analysis Decisions in Assets Settings while retaining its historical records.

**Non-Goals:**

- Change RAW/Library identity, table schemas, pagination, lifecycle, backend eligibility, Queue, Advisor scoring, profile resolution, Test Encode, validation, Publisher, or metadata enrichment.
- Add a safe sorting operation, account/notification contract, storage scan or new capacity source, trend metrics, artwork API, or RAW path rename.
- Copy the Figma Make app, dependency tree, or data model.

## Decisions

1. **Use the current page composition.** Put Assets, compact Search, and the supported Settings action in a full-width top row. Start the two-column workspace below it. Keep the Asset Library hierarchy on the left; place summary cards, one breadcrumb/path-action bar, the inventory control row, the table scroll region, and selected detail in the right column.

2. **Use two visible hierarchy sections without changing identities.** Show RAW and Libraries as level-one items; place configured destination roots beneath Libraries. Keep the wrapper presentation-only. Default width is 262px, clamped from 220px through 390px and persisted locally. The plus icon links to the existing `/libraries` route. Keep the shared storage summary below the scrollable tree, backed by the existing `['runtime-state']` query and real disk fields; render loading, error, unavailable, and zero-capacity states without a second polling path or new filesystem scan.

3. **Keep the summary truthful.** Show Total Assets, Processing, Completed, and Needs Review using existing inventory/lifecycle counts. Reuse the existing filter/query state in a compact Filter popover. The control strip shows count, Sync, and Filter; Sort is disabled because current safe sorting is unsupported. Do not reproduce mock trends, avatar, or notification state. The hierarchy storage summary may derive used bytes and percentage only from valid `RuntimeState.disks` total/available values.

4. **Join breadcrumb and path actions visually while retaining their existing owners.** Keep routine operations together and place supported destructive actions after a divider. For Library paths, retain Scan, Queue/Preflight, Move path, Rename path, Delete selected, and Delete path when applicable. For RAW scopes, expose only actions with current owners, including supported Scan, explicit Advisor, Queue/Preflight, Configure Path, and Move to library at eligible path levels. Logical RAW title groups expose their current Scan, explicit Advisor, Configure Path, and Move to library flows through the shared breadcrumb toolbar; this scope has no direct Queue or delete-path owner. Reuse the existing AssetGroupRow and logical-title handlers and their lifecycle-safe behavior. There is no separate second Library “Move to library” API action, and RAW path Rename has no supported owner; do not invent either behavior.

5. **Match the compact row surfaces and selection.** Keep mode-specific schemas, filename-only Asset cells, table-local horizontal overflow, thin borders, and the current subtle 4px vertical row separation. Give the Asset column enough width to preserve episode identifiers in long filenames. Use the navy row surface, restrained hover, purple active-detail selection, and one row overflow menu. The header checkbox selects only visible eligible rows and reflects checked, mixed, and disabled states. Library row selection is available only when the existing lifecycle-safe delete/return capability allows it. The Advisor score remains canonical and is paired with a short subtle indicator; not-evaluated/off/unavailable states have no indicator.

6. **Keep the list and selected detail as separate vertical regions.** Summary, breadcrumb/actions, and controls stay outside the inventory scroll region. Rows scroll independently with reserved bottom padding. Selected detail is a sibling below the scroll container and remains attached to the bottom of the right workspace; it is never a modal or persistent right panel.

7. **Match the current selected-detail frame while retaining issue #208 content.** Use the current landscape preview proportion (approximately 16:10) at left with centered Play overlay and duration badge. To its right, show filename/path identity and compact category/library, container, size, resolution, and codec facts. Use the state pill as the single visible Status value, then show Profile, Subtitles, Duration, and Modified in a compact context group. Keep the technical group compact with Video codec, Resolution, and Audio summary. Values may repeat between quick facts, context, and technical grouping where issue #208 or the current reference calls for them. Read only existing asset technical evidence and the selected persisted snapshot; show unavailable fields explicitly and preserve confirmed zero values. Align Assign Profile, Test Encode, and More actions with the technical group, using the selected row's registered owners and existing capability/eligibility rules. Play remains integrated into the artwork. Let content determine expanded panel height, keep the panel collapsible with a subtle control and compact reopen state, and reflow metadata and technical groups at smaller widths.

8. **Use only real artwork sources.** Current backend/frontend contracts provide no supported per-asset artwork URL. Keep the generic placeholder. A future supported source should use Jellyfin artwork, then an existing MVForge thumbnail, then placeholder, but this fallback is not implemented until the real source reaches the caller. Do not copy the Figma poster into the product or generate artwork.

9. **Move legacy analysis history without changing records.** Keep `LegacyAnalysisRecords` out of the main Assets page and render the same saved records under the Assets Settings section.

10. **Adapt iconography and branding inside MUI.** Keep existing MUI icons when they match the Figma treatment and render supplied path-action icons. Use a purple rounded-square mark with a white outlined Play glyph in the navigation shell; remove the old CSS triangle. Preserve route behavior, desktop hover expansion, and mobile drawer. The current frame has no persistent pin/unpin control.

11. **Do not expand unsupported visual behavior.** Figma account/avatar and notifications lack current owners; trend values are not canonical; safe Sort is unavailable; a distinct second Library migration and RAW path rename are unsupported; not every RAW source/title scope owns all path actions. Omit or disable those controls and report these gaps. Storage uses only the existing RuntimeState disk values and adds no scan or polling behavior.

12. **Verify through structural/accessibility regressions.** Test eligible Library row checkboxes, visible-page select-all, mixed/disabled states, separation between active detail and bulk selection, lifecycle-safe deletion targets, current detail fields/aspect ratio/collapse, path icons, RAW header selection, branding, and supported action availability. Do not assert screenshot coordinates or unsupported demo data.

## Risks / Trade-offs

- [The design reference includes demo-only data] → Display only canonical counts and supported current state.
- [No supported per-asset artwork source reaches the caller] → Use the generic placeholder and keep artwork integration as a follow-up.
- [Figma presents actions that are not available at every RAW/Library scope] → Show only operations owned by current flows and gated by their current capability rules.
- [Dense table columns exceed narrow widths] → Keep horizontal overflow in the table and reflow hierarchy/detail without growing document width.
- [Stored hierarchy preference is stale or malformed] → Clamp to the current 220–390px range and retain accessible keyboard resizing.

## Migration Plan

No backend migration or endpoint is planned. Preserve existing Assets query, action, and record contracts. Persist only the bounded local hierarchy width. Reverting this visual pass does not change media, inventory, historical analysis, or lifecycle data.
