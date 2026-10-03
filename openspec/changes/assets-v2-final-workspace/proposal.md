## Why

Issue #208 completes the shared Assets V2 RAW/Library workspace. This final visual-fidelity pass re-audits the latest local reference files and updates layout, hierarchy presentation, controls, row density, sidebar branding, and selected-asset context without changing existing domain behavior.

## What Changes

- Use the current `docs/design-reference/assets-v2/figma-make/` and `docs/design-reference/assets-v2/asstesv2.png` visual references. In this checkout the selected-asset PNG has the existing `asstesv2.png` spelling.
- Place a full-width Assets/Search row above the two-column workspace. Put Asset Library at left and summary, breadcrumb/path actions, inventory controls, independently scrolling rows, and sticky selected detail at right.
- Present the two hierarchy sections as RAW and Libraries, with configured destination Library roots under Libraries; preserve canonical context IDs. Set resize bounds to 220px–390px with a 262px default and local persistence.
- Keep summary counts canonical, existing filters in a compact Filter popover, and Sort disabled while safe sorting is unsupported.
- Keep the shared RAW/Library table schemas, filename-only cells, thin row separators, selected/hover treatments, score-only Advisor indicator, and existing row-action menu.
- Recompose breadcrumb and supported path actions into one bar while reusing current callbacks, dialogs, capability checks, confirmations, and lifecycle-safe destructive behavior.
- Keep selected-asset context below the independently scrolling list, using the current reference's landscape/identity/status/technical-actions grouping and accessible collapse/reopen behavior.
- Keep generic artwork until an actual supported artwork URL reaches the selected-detail caller. Record unsupported design actions as follow-ups instead of fabricating them.
- Move Legacy Analysis Decisions from normal browsing into Assets Settings without changing saved records.
- Refine the existing MVForge sidebar mark and icon spacing within React + MUI, preserving navigation behavior.

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
