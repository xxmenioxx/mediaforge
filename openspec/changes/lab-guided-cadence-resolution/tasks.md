## 1. Backend resolution contract

- [x] 1.1 Add the versioned LAB `resolution_required` diagnostic for the specific unresolved-FPS Auto GOP planning failure, preserving legacy error/status fields; verify with a handler test that asserts source/profile evidence identity, requested cadence/GOP, effective known state, field blockers, and eligible actions.
- [x] 1.2 Resolve the bounded recommendation set through canonical cadence, GOP, encoder/HEVC, and Fidelity authorities; verify ambiguous IVTC, validated cadence Auto GOP, supported Preserve, unsupported transforms, and mixed cadence with focused backend tests.
- [x] 1.3 Bind Apply/Test requests to source, scan, and profile revisions and keep application asset-scoped; verify stale plans are rejected and the accepted plan's effective decision/fingerprint matches the Queue preflight plan in backend parity tests.
- [x] 1.4 Reuse the existing opt-in bounded Deep Cadence operation only when its identity checks allow it; verify ineligible/stale evidence is not retried and recommendation generation launches no additional FFmpeg work.
- [x] 1.5 Attach an evidence-backed Fidelity diagnostic to successful Preview operation results when required canonical fields remain UNVERIFIED; preserve independent planning and Fidelity statuses.

## 2. LAB resolution assistant

- [x] 2.1 Add API types and render a compact `Why Auto needs review` panel with plan readiness, per-field authority/reason, and the backend-recommended alternative; verify backend `UNVERIFIED` cannot be presented as Safe in focused ProfileLab tests.
- [x] 2.2 Wire Apply to an asset-scoped draft and Test alternative through existing compatible Preview/Test Encode; expose Re-analyze cadence only when backend eligible; verify deterministic plan application, stale state refresh, and no global default mutation in focused frontend tests.
- [x] 2.3 Offer operator-requested TFF/BFF comparison previews only for ambiguous field order, never auto-selecting a winner; verify both alternatives remain explicit choices and the result remains review-blocked until canonical evidence passes.
- [x] 2.4 Show the existing Resolution Assistant for successful previews with canonical UNVERIFIED fields, using gate-aware wording and only actions eligible for the reported fields.

## 3. Integrated validation

- [x] 3.1 Run focused backend and frontend regression tests covering the historical ambiguous-IVTC fixture, field-level Preserve Fidelity reasons, plan parity, stale/mixed/unsupported cases, no implicit order guess, and bounded work; record observed results.
- [x] 3.2 Run `python3 /Users/anuelvs/Documents/mediaforge/.agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto` and record `ALL CHECKS PASSED` or preserve the failure evidence.
- [x] 3.3 Add backend and frontend regressions proving successful Preview plus canonical UNVERIFIED exposes actionable evidence without changing the verdict or claiming Safe; `verify.sh --auto` passed (full, 7/7, 250 s).
