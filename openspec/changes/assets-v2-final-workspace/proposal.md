## Why

Issue #208 completes the shared Assets V2 RAW/Library workspace. This correction pass uses the exact Figma Make page `Update Selected Asset Workspace`, file `SPURUawBZid2U8AjG95bsn`, code node `0-6`, and its matching local image `docs/design-reference/assets-v2/asstesv2.png`. It corrects selected-asset presentation, real filesystem attribution, and the inherited Library destination used during individual Queue preparation while preserving canonical behavior.

## What Changes

- Use only the current Figma Make reference at code node `0-6` and the matching local `docs/design-reference/assets-v2/asstesv2.png`; disregard earlier references and visual assumptions.
- Place a full-width Assets/Search row above the two-column workspace. Put Asset Library at left and summary, breadcrumb/path actions, inventory controls, independently scrolling rows, and sticky selected detail at right.
- Present the two hierarchy sections as RAW and Libraries, with configured destination Library roots under Libraries; preserve canonical context IDs. Set resize bounds to 220px–390px with a 262px default and local persistence.
- Keep summary counts canonical, existing filters in a compact Filter popover, and Sort disabled while safe sorting is unsupported.
- Keep the shared RAW/Library table schemas, filename-only cells, subtle vertical row separation, selected/hover treatments, score-only Advisor indicator, and existing row-action menu. Add accessible header selection over visible eligible rows, with Library Safe Delete checkboxes gated by existing lifecycle capabilities.
- Recompose breadcrumb and supported path actions into one bar while reusing current callbacks, dialogs, capability checks, confirmations, and lifecycle-safe destructive behavior.
- Keep selected-asset context below the independently scrolling list. Use the landscape preview, filename and series/path context, compact category/container/size/resolution/codec facts, Status/Profile/Subtitles/Duration/Modified context, video/resolution/audio technical values, and existing selected-asset actions. Keep the action group aligned in its own column, keep Play integrated into artwork, and route actions through existing owners.
- Keep generic artwork until an actual supported artwork URL reaches the selected-detail caller. Record unsupported design actions as follow-ups instead of fabricating them.
- Move Legacy Analysis Decisions from normal browsing into Assets Settings without changing saved records.
- Use the current two-color teal/blue/violet angular MVForge mark without a purple Play tile; preserve desktop hover and mobile drawer behavior. The current frame has no pin/unpin affordance.
- Report measured filesystem path and volume identity. Group configured paths sharing a filesystem once, keep distinct volumes separate, and show inaccessible destinations without attributing parent capacity. Values describe filesystem capacity and usage, not media-folder size.
- During claim preparation, preserve the asset's own Library only when destination config is INHERIT, the queued Library matches the record's Library, and the asset path is inside that Library destination. Explicit DISABLED and Custom states remain distinct; RAW folder Preflight keeps its current unprocessed/effective-destination rules.

## Capabilities

### New Capabilities

- `assets-workspace`: Shared RAW and Library layout, hierarchy, inventory, path/row actions, filters, and selected-asset context.

### Modified Capabilities

None. No existing backend or lifecycle contract changes.

## Impact

- Frontend: `AssetsPage`, shared Assets components, navigation shell, Assets Settings, and focused observable tests.
- APIs and backend: no new endpoint, model, eligibility rule, Advisor evaluation, Queue behavior, artwork pipeline, or media processing behavior.
- OpenSpec: current visual/layout behavior and explicit unsupported gaps are recorded in the `assets-workspace` capability.

## Follow-up

- Safe inventory sorting is not currently supported; Sort stays disabled.
- No supported per-asset Jellyfin artwork or MVForge thumbnail URL reaches the selected-detail caller; artwork integration remains a follow-up.
- The running visual-review server returned legacy disk objects without measured volume identities, so its screenshot showed separate Library and Workspace labels for directories that the local filesystem confirms share a device. Current backend unit tests cover grouping by `volumeId`; visual confirmation of the merged live footer requires the review server to load this backend revision.
- Library migration remains the current Move path action; a separate additional Library “Move to library” operation is unsupported.
- RAW path Rename is unsupported by current path action owners.
- The reference's account/avatar, notifications, and trend indicators have no current canonical product contracts and are not fabricated. Filesystem utilization percentage is shown only when derived from measured capacity and available bytes.
