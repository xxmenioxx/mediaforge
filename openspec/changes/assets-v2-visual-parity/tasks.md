## 1. Reports Route and Navigation

- [x] 1.1 Add the dedicated `/reports` route using the existing persisted inventory report panel and preserve its current actions; verify report data and missing-record actions remain reachable.
- [x] 1.2 Point the Reports menu item to `/reports`, make active state pathname-based, and redirect legacy Assets report links; verify RAW, Library, Reports, and legacy routes with focused navigation tests.

## 2. Assets V2 Visual Refinement

- [x] 2.1 Refine the Assets header and canonical summary cards; use the hierarchy rather than a separate RAW/Library selector to identify the active context.
- [x] 2.2 Refine the shared hierarchy panel and context headings while keeping separate canonical models; verify RAW source/title/path selection filters the expected assets and Library path selection remains functional.
- [x] 2.3 Refine compact table surfaces and selected-asset detail grouping; verify Library columns, no row artwork, one integrated Play action, and valid Assign Profile/Test Encode actions.
- [x] 2.4 Preserve bounded list behavior; verify deep detail and preview work remain selected-asset-only with no per-row calls.
- [x] 2.5 Replace the RAW/Library tabs and separate tree panels with one accessible hierarchy containing RAW and configured Library roots together; verify URL cleanup, query preservation, root/descendant selection, keyboard navigation, Reports separation, and existing actions.

## 3. Validation

- [x] 3.1 Run focused unified-hierarchy and navigation tests, frontend lint, frontend build, and `openspec validate assets-v2-visual-parity --strict`; resolve failures introduced by this change.
- [x] 3.2 Run `./scripts/verify.sh --auto` and confirm the repository verifier reports `ALL CHECKS PASSED`.
