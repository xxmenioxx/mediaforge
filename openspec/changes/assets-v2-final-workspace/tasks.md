## 1. Shared hierarchy and inventory

- [x] 1.1 Keep one canonical RAW/Library tree, present visible RAW and Libraries sections with configured roots under Libraries, and provide accessible persisted resizing at 262px default / 220px–390px bounds. Verify grouping, keyboard navigation, pointer/keyboard resize, persistence, and invalid-width recovery.
- [x] 1.2 Preserve the shared table with mode-specific Library/RAW schemas, filename-only cells, compact technical rows, and subtle vertical row separation; verify both column sets and no relative paths.
- [x] 1.3 Keep Advisor explicit-only; display canonical cached score with a short score indicator, distinct neutral/unavailable/off states, and no row evaluation. Verify explicit path Advisor caches results under `['advisor', 'asset-row', asset.path, profileId]`.
- [x] 1.4 Keep compact row overflow actions behind existing routes/dialogs and capability gates. Render path actions with their supplied icons in the compact breadcrumb bar; keep workflow actions, checkbox-derived selected count, and lifecycle-safe destructive actions on their existing owners.
- [x] 1.5 Add capability-gated Library Safe Delete row selection and an accessible visible-page header checkbox; keep active detail selection separate from bulk selection and test checked/mixed/disabled states plus safe target dispatch.
- [x] 1.6 Keep the existing RuntimeState storage summary at the bottom of the scrollable hierarchy in both RAW and Library workspaces; reuse its query/cache and preserve loading, error, unavailable, and known-zero states without additional scans or polling.
- [x] 1.7 Resolve path Queue and explicit Advisor actions from canonical effective asset configuration; preserve inherited/disabled/custom states, react to inherited destination changes, and submit backend-resolved custom assignments without silent frontend returns.
- [x] 1.8 Show pending, success, failure, and partial-failure feedback for V2 path Queue/Preflight, publish-as-is, migration, and bulk actions in visible workspace surfaces.
- [x] 1.9 Recheck active work and Publish-as-is lifecycle under existing synchronization/reservation locks immediately before returning files to RAW; preserve rollback and add an active-work regression.
- [x] 1.10 Report the exact measured filesystem path and volume ID; group shared Library/Workspace volumes once, keep distinct Library volumes separate, preserve unavailable destinations without parent-capacity fallback, and test shared, distinct, partial, and fully unavailable probes. Render capacity as filesystem usage, not media-folder size.

## 2. Current-reference workspace and navigation

- [x] 2.1 Re-audit the exact current Figma Make page `Update Selected Asset Workspace` (file SPURUawBZid2U8AjG95bsn, code node 0-6) before editing; verify and use its matching local image `docs/design-reference/assets-v2/asstesv2.png` as the composition reference.
- [x] 2.2 Put Assets, Search, and supported top actions in the full-width top row; place the Asset Library hierarchy and right workspace below it, with summary cards on the right.
- [x] 2.3 Keep existing filters in the compact Filter popover; show count, Sync, Filter, and disabled unsupported Sort above the independent list region.
- [x] 2.4 Keep breadcrumb and supported path actions in one visual bar; preserve contextual scope, handler reuse, selected count, destructive separation, and backend eligibility.
- [x] 2.5 Match current compact table density, subtle vertical row separation, selected/hover state, and canonical Advisor indicator.
- [x] 2.6 Use the current angular teal/blue/violet MVForge mark without a purple tile or Play glyph; verify desktop and mobile navigation, remove the obsolete mark, and preserve the current hover/drawer behavior without adding pinning.

## 3. Selected asset and historical settings

- [x] 3.1 Keep selected detail outside inventory scrolling and attached to the workspace bottom; match the current approximately 16:10 landscape preview, two-line filename/path identity, category/library/container/size/resolution/codec quick facts, a single Status pill, Profile/Subtitles/Duration/Modified context, and compact icon-led Video/Resolution/Audio technical information. Separate and align the restrained Assign Profile/Test Encode/More action column. Preserve unavailable and zero values, route actions through existing gates, keep Play integrated into artwork, and retain accessible collapse/reopen behavior.
- [x] 3.2 Keep the generic artwork placeholder because no supported per-asset Jellyfin/MVForge artwork URL reaches the detail caller; do not use reference demo art or generate artwork. Record the actual fallback order only as a future contract.
- [x] 3.3 Keep Legacy Analysis Decisions out of the main Assets page and expose the unchanged records from the Assets Settings section.

## 4. Focused regressions and validation

- [x] 4.1 Add and run focused regressions for RAW selected-detail Play/Assign Profile/Test Encode/More routes, canonical effective path configuration, inherited destination updates, custom individual Queue, visible path feedback, active-work Return Published-as-is blocking, storage footer availability, bulk-selection/lifecycle safety, and selected-detail composition.
- [x] 4.2 Re-run the focused Assets V2 and action-flow tests after the selected-detail, path configuration, and storage footer changes.
- [x] 4.3 Run focused changed-area tests and `./scripts/verify.sh --auto`; record only observed results and require `ALL CHECKS PASSED` (26 focused frontend tests, targeted runtimeinfo and handler tests, verifier auto→full 8/8 checks, and strict OpenSpec validation passed).
- [x] 4.4 Reproduce an assignment-backed individual Library Queue with INHERIT destination, freeze the asset's own Library only when IDs match and the path is inside that Library, and compare with folder Queue/Preflight's existing Unprocessed rule.
- [x] 4.5 Capture populated RAW and Library views with selected assets at the reference viewport. Record that the review server's legacy disk response prevented live confirmation of shared-volume display until it loads the current backend; do not claim the stale screenshot validates the new volume UI.
