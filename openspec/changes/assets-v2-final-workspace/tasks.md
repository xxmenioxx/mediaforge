## 1. Shared hierarchy and inventory

- [x] 1.1 Keep one canonical RAW/Library tree, present visible RAW and Libraries sections with configured roots under Libraries, and provide accessible persisted resizing at 262px default / 220px–390px bounds. Verify grouping, keyboard navigation, pointer/keyboard resize, persistence, and invalid-width recovery.
- [x] 1.2 Preserve the shared table with mode-specific Library/RAW schemas, filename-only cells, compact technical rows, and thin contiguous row surfaces; verify both column sets and no relative paths.
- [x] 1.3 Keep Advisor explicit-only; display canonical cached score with a short score indicator, distinct neutral/unavailable/off states, and no row evaluation. Verify explicit path Advisor caches results under `['advisor', 'asset-row', asset.path, profileId]`.
- [x] 1.4 Keep compact row overflow actions behind existing routes/dialogs and capability gates. Render path actions in the compact breadcrumb bar with eligible workflow actions, selected count, and visually separated lifecycle-safe destructive actions; reuse the existing action handlers.

## 2. Current-reference workspace and navigation

- [x] 2.1 Re-audit only the current Figma Make export and selected-asset PNG before editing; update the design/spec to remove stale hierarchy, spacing, color, action, and layout assumptions.
- [x] 2.2 Put Assets, Search, and supported top actions in the full-width top row; place the Asset Library hierarchy and right workspace below it, with summary cards on the right.
- [x] 2.3 Keep existing filters in the compact Filter popover; show count, Sync, Filter, and disabled unsupported Sort above the independent list region.
- [x] 2.4 Keep breadcrumb and supported path actions in one visual bar; preserve contextual scope, handler reuse, selected count, destructive separation, and backend eligibility.
- [x] 2.5 Match current compact table density, contiguous row surfaces, selected/hover state, and canonical Advisor indicator without creating artificial row gaps.
- [x] 2.6 Refine the existing navigation shell with the current MVForge gradient triangle mark and compact icon treatment while preserving every route and mobile/desktop behavior.

## 3. Selected asset and historical settings

- [x] 3.1 Keep selected detail outside inventory scrolling and attached to the workspace bottom; render the current four-part landscape/identity/status/technical-actions composition and accessible collapse/reopen affordance.
- [x] 3.2 Keep the generic artwork placeholder because no supported per-asset Jellyfin/MVForge artwork URL reaches the detail caller; do not use reference demo art or generate artwork. Record the actual fallback order only as a future contract.
- [x] 3.3 Keep Legacy Analysis Decisions out of the main Assets page and expose the unchanged records from the Assets Settings section.

## 4. Focused regressions and validation

- [x] 4.1 Add and run structural/accessibility regressions for the top row and hierarchy order, RAW/Libraries grouping, summary placement, filter controls, row surface, separate list scrolling, path bar/actions, Advisor states, selected detail composition/collapse, Assets Settings relocation, and MVForge branding.
- [x] 4.2 Run focused Assets V2, hierarchy, navigation, and Assets Settings tests; observed result: 5 test files passed, 32 tests passed.
- [x] 4.3 Validation observed: focused tests passed (5 files / 32 tests); frontend lint passed with the Babel large-file notice; frontend build passed with Vite's large-chunk warning; strict OpenSpec validation passed; `./scripts/verify.sh --auto` selected full scope and passed 10/10 checks.
