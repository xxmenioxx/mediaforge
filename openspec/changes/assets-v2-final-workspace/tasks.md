## 1. Shared hierarchy and inventory

- [x] 1.1 Complete the shared RAW/Library hierarchy and accessible local resize control with a 300px default and 220px–420px bounds; verify keyboard resizing, selection state, persisted width, and invalid stored-width recovery in focused tests.
- [x] 1.2 Render the mode-specific Library and RAW column sets through shared table presentation and keep row Asset cells filename-only; verify exact column order, status evidence, unavailable summaries, and absence of relative paths in focused tests.
- [x] 1.3 Display only an already available canonical `AdvisorResponse.score` and a truthful unavailable/not-evaluated state otherwise; verify row rendering and refresh do not invoke Advisor evaluation, while an explicit path action still uses its existing workflow.
- [x] 1.4 Separate path toolbar operations and selected-asset count from one asset overflow menu per row; verify path and row actions stay distinct and existing capability, eligibility, disabled-reason, confirmation, and Queue/Preflight behavior remains enforced.

## 2. Selected Asset Workspace

- [x] 2.1 Keep selected detail collapsible below the table and arrange preview, identity/context, status, technical summaries, and existing actions responsively; verify selection, collapse, one integrated preview action, and narrow-width accessibility in focused tests.
- [x] 2.2 Apply the conditional artwork order of available Jellyfin artwork, existing MVForge artwork/thumbnail, then generic placeholder without generating media; verify each available fallback with fixtures and record Jellyfin integration as a follow-up if no supported source exists.

## 3. Focused regression and validation

- [x] 3.1 Add and run focused observable regressions for shared RAW/Library context rendering, both table schemas, filename-only rows, Advisor evidence, overflow actions, path/row separation, resize persistence/keyboard behavior, selected detail, artwork fallback, capability enforcement, and accessibility.
- [x] 3.2 After focused tests pass, run `cd frontend && npm run lint`, `cd frontend && npm run build`, `openspec validate assets-v2-final-workspace --strict`, and `./scripts/verify.sh --auto`; record only observed results and report unsupported visual-reference behavior as a follow-up.
