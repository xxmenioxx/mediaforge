## 1. Canonical evidence state and identity

- [x] 1.1 Trace every current AnalysisPage action and persisted `ScanResult`
  consumer; map each to an existing backend authority and classify any legacy
  decision/notes behavior without introducing a new analysis domain model.
- [x] 1.2 Extend the existing asset lifecycle evidence projection with
  backend-authoritative `missing`, `ready`, `stale`, and `unavailable` states,
  source identity, stale components, and bounded reasons; verify legacy or
  ambiguous identities never become current from path equality alone.
- [x] 1.3 Reuse existing scanner cache, source revision, component refresh,
  operation status, and policy rules; verify incremental refresh preserves
  reused and failed component provenance and does not create a second pipeline.
- [x] 1.4 Add focused backend contract coverage for each state, path reuse,
  stale component refresh, partial evidence, inaccessible/missing source, and
  unavailable/legacy identity without relying on real media or NAS access.

## 2. Asset Info and route migration

- [x] 2.1 Make Asset Info Analysis consume the canonical evidence projection and
  existing lazy snapshot API; verify opening Asset Info/Overview performs zero
  scan, ffprobe, FFmpeg, OCR, Advisor, LAB, or other heavy work.
- [x] 2.2 Expose existing AS-IS, cadence/GOP, HDR/color, audio, subtitle, and
  other available evidence in the existing Analysis section with explicit
  Analyze/Refresh actions and distinct state presentation; preserve Advisor
  and Profile LAB as separate explicit capabilities.
- [x] 2.3 Remove the standalone Analysis page/navigation and add safe
  compatibility redirects for `/analysis`, `/scanner`, and supported legacy
  query forms; verify asset identity/context is preserved, Overview opens
  first, and redirects never start analysis.
- [x] 2.4 Preserve every existing `analysisRecords` decision and note. Project
  only deterministically matched entries as read-only legacy History
  annotations; keep unmatched entries accessible and visibly unlinked, and
  verify path reuse never causes false association or data loss.
- [x] 2.5 Add focused frontend coverage for lazy loading, Overview's zero-heavy-
  work invariant, missing/ready/stale/unavailable states, explicit refresh,
  route compatibility, and legacy decision visibility.

## 3. Batch Preflight and Queue freeze

- [x] 3.1 Extend the existing selected-assets preview/Batch Preflight response
  with the shared per-asset evidence status and identity using bounded batch
  reads; verify preview starts no media work and performs no per-asset request
  fan-out.
- [x] 3.2 Preserve Queue's current eligibility, claim, preparation, and
  effective-configuration authorities; at the existing immutable freeze,
  persist/reference only the canonical evidence components needed to explain
  the accepted plan.
- [x] 3.3 Add deterministic Queue tests proving frozen evidence identity and
  plan remain unchanged after current evidence/configuration changes and that
  unused evidence is not copied into the execution snapshot.
- [x] 3.4 Verify Batch Preflight and Asset Info show the same evidence identity
  and status for the same persisted observation and distinguish preflight
  preview from Queue's later preparation-time refresh.

## 4. History correlation and compatibility

- [x] 4.1 Link canonical source evidence, immutable frozen plan, Queue result,
  and a future #53 Validation record in existing per-asset History by stable
  authority/record identity; do not add report storage or implement Validation.
- [x] 4.2 Preserve historical evidence references across asset rescan, rename,
  move, replacement, and path reuse; verify unavailable/unresolved links remain
  explicit rather than attaching to current evidence by path or timestamp.
- [x] 4.3 Keep existing scanner APIs, operation records, `ScanResult`, and
  `analysisPolicy` consumers compatible; document/removal-scope any obsolete
  page-only API use before deprecation.

## 5. Validation and handoff

- [x] 5.1 Run focused backend/frontend checks for evidence state, lazy behavior,
  batch preview, freeze provenance, route migration, and History correlation;
  report unrelated baseline failures separately.
- [x] 5.2 Run `openspec validate analysis-asset-lifecycle --type change --strict`
  and verify proposal, design, specs, and tasks are valid.
- [x] 5.3 Keep #53 Validation implementation out of this #52 implementation;
  constrain production changes to the explicitly authorized #52 tasks and
  preserve the #53 handoff only.
