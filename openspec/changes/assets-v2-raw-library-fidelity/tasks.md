## 1. Context switching and URL compatibility

- [x] 1.1 Replace the Unprocessed/Library control with an accessible RAW/Library selector backed by `location`; preserve common search, clear incompatible Library filters on RAW entry, and keep legacy links mapped to valid contexts. Verify switching in focused page tests.
- [x] 1.2 Keep RAW and Library hierarchy and asset selection state isolated across context changes, retaining a deep-link selection only when its identity is valid in the destination. Verify URL and selection behavior with mocked inventory data.

## 2. RAW workspace integration

- [x] 2.1 Add a collapsed, keyboard-operable Source Group → Logical Group/title → Path hierarchy from existing source-group summaries and filter visible RAW assets by the selected node. Verify root and nested paths, selection, and no new inventory calls.
- [x] 2.2 Reuse the compact scan list, selected-asset detail, and existing AssetRow actions for RAW; preserve Source Group/title/path configuration, Advisor, analysis, selection, Needs Review, and Queue/Batch Preflight owners. Verify existing entry points and selected-only detail loading.

## 3. Approved workspace presentation

- [x] 3.1 Tune shared hierarchy/list proportions, header and action density, row spacing, dividers, selected treatment, typography, summary, and selected preview against the approved image using responsive constraints. Verify Library and RAW visual structure at desktop and narrow breakpoints.

## 4. Regression coverage and validation

- [x] 4.1 Add focused regressions for context switching, legacy URLs, hierarchy isolation, RAW hierarchy/actions, Library’s six columns and path actions, selected detail and one Play control, metadata absence, accessibility, and responsive structure; run the focused Assets and navigation suites.
- [x] 4.2 Run `openspec validate assets-v2-raw-library-fidelity --strict`, `npm run lint`, `npm run build`, and `./scripts/verify.sh --auto`; resolve only failures introduced by this change and record exact results.

## 5. Issue #388 exact navigation selection

- [x] 5.1 Keep explicit stale RAW and Library URL selections scoped and empty instead of substituting search matches or unrelated locations.
- [x] 5.2 Cover same-name nodes, nested and sibling paths, selection changes, browser Back/Forward, and invalid deep links.
