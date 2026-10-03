## 1. Shared hierarchy and inventory

- [x] 1.1 Keep one canonical RAW/Library tree, present visible RAW and Libraries sections with configured roots under Libraries, and provide accessible persisted resizing at 262px default / 220px–390px bounds. Verify grouping, keyboard navigation, pointer/keyboard resize, persistence, and invalid-width recovery.
- [x] 1.2 Preserve the shared table with mode-specific Library/RAW schemas, filename-only cells, compact technical rows, and subtle vertical row separation; verify both column sets and no relative paths.
- [x] 1.3 Keep Advisor explicit-only; display canonical cached score with a short score indicator, distinct neutral/unavailable/off states, and no row evaluation. Verify explicit path Advisor caches results under `['advisor', 'asset-row', asset.path, profileId]`.
- [x] 1.4 Keep compact row overflow actions behind existing routes/dialogs and capability gates. Render path actions with their supplied icons in the compact breadcrumb bar; keep workflow actions, checkbox-derived selected count, and lifecycle-safe destructive actions on their existing owners.
- [x] 1.5 Add capability-gated Library Safe Delete row selection and an accessible visible-page header checkbox; keep active detail selection separate from bulk selection and test checked/mixed/disabled states plus safe target dispatch.

## 2. Current-reference workspace and navigation

- [x] 2.1 Re-audit the authenticated current Figma frame at node 2:1874 before editing; record that the correctly spelled local selected-asset PNG is absent and do not treat the stale misspelled PNG as current authority.
- [x] 2.2 Put Assets, Search, and supported top actions in the full-width top row; place the Asset Library hierarchy and right workspace below it, with summary cards on the right.
- [x] 2.3 Keep existing filters in the compact Filter popover; show count, Sync, Filter, and disabled unsupported Sort above the independent list region.
- [x] 2.4 Keep breadcrumb and supported path actions in one visual bar; preserve contextual scope, handler reuse, selected count, destructive separation, and backend eligibility.
- [x] 2.5 Match current compact table density, subtle vertical row separation, selected/hover state, and canonical Advisor indicator.
- [x] 2.6 Use the current purple MVForge tile and white outlined Play glyph; verify desktop and mobile navigation, remove the stale triangle, and preserve the current hover/drawer behavior without adding pinning.

## 3. Selected asset and historical settings

- [x] 3.1 Keep selected detail outside inventory scrolling and attached to the workspace bottom; preserve the current approximately 4:3 artwork and Figma grouping while showing issue #208 status/profile/subtitle/audio context, required selected-asset actions, non-duplicated technical facts, integrated Play, and accessible collapse/reopen behavior.
- [x] 3.2 Keep the generic artwork placeholder because no supported per-asset Jellyfin/MVForge artwork URL reaches the detail caller; do not use reference demo art or generate artwork. Record the actual fallback order only as a future contract.
- [x] 3.3 Keep Legacy Analysis Decisions out of the main Assets page and expose the unchanged records from the Assets Settings section.

## 4. Focused regressions and validation

- [x] 4.1 Add and run focused structural/accessibility regressions for Library Safe Delete eligibility, visible-page header selection, active-detail/bulk-selection separation, safe target dispatch, RAW header selection, required selected-detail fields/action delegation/fallbacks/non-duplication/collapse, path icons, and desktop/mobile branding.
- [x] 4.2 Re-run the focused Assets V2, hierarchy, navigation, and Assets Settings tests after restoring required selected-detail content and action owners.
- [x] 4.3 Re-run required frontend lint/build, strict OpenSpec validation, and `./scripts/verify.sh --auto`; record only current observed results.
