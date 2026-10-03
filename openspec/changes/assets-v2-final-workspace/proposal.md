## Why

Issue #208 completes the shared Assets V2 RAW/Library workspace. This final correction pass uses the current authenticated Figma frame at node 2:1874 and corrects Library Safe Delete selection, header selection semantics, path-action icon visibility, branding, and the selected-asset context without changing domain behavior.

## What Changes

- Use the current authenticated Assets V2 Figma frame (node 2:1874) as the visual authority. The approved `docs/design-reference/assets-v2/assetsv2.png` is not present in this checkout; the differently spelled `asstesv2.png` is stale and MUST NOT be treated as equivalent.
- Place a full-width Assets/Search row above the two-column workspace. Put Asset Library at left and summary, breadcrumb/path actions, inventory controls, independently scrolling rows, and sticky selected detail at right.
- Present the two hierarchy sections as RAW and Libraries, with configured destination Library roots under Libraries; preserve canonical context IDs. Set resize bounds to 220px–390px with a 262px default and local persistence.
- Keep summary counts canonical, existing filters in a compact Filter popover, and Sort disabled while safe sorting is unsupported.
- Keep the shared RAW/Library table schemas, filename-only cells, subtle vertical row separation, selected/hover treatments, score-only Advisor indicator, and existing row-action menu. Add accessible header selection over visible eligible rows, with Library Safe Delete checkboxes gated by existing lifecycle capabilities.
- Recompose breadcrumb and supported path actions into one bar while reusing current callbacks, dialogs, capability checks, confirmations, and lifecycle-safe destructive behavior.
- Keep selected-asset context below the independently scrolling list. Match the current approximately 4:3 artwork and filename/path identity; show compact quick facts and Profile/Subtitles/Duration/Modified context, then a broad MEDIA strip for Video, Encoder, Duration, Bitrate, Range, Size, Modified, and Audio. Keep Play integrated into artwork and route actions through existing selected-asset owners.
- Keep generic artwork until an actual supported artwork URL reaches the selected-detail caller. Record unsupported design actions as follow-ups instead of fabricating them.
- Move Legacy Analysis Decisions from normal browsing into Assets Settings without changing saved records.
- Use the current purple rounded-square MVForge tile with its white outlined Play glyph; preserve desktop hover and mobile drawer behavior. The current frame has no pin/unpin affordance.

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
- Library migration remains the current Move path action; a separate additional Library “Move to library” operation is unsupported.
- RAW path Rename is unsupported by current path action owners.
- The reference's account/avatar, notifications, storage-capacity percentage, and trend indicators have no current canonical product contracts and are not fabricated.
