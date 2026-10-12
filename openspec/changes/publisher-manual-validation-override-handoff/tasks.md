## 1. Backend Eligibility and Handoff

- [x] 1.1 Add a bounded read-only Publisher eligibility endpoint using canonical manual Validation, exact Result identity/revision, override evidence, required sidecars, and frozen delivery preflight; cover allowed, override-backed, blocked, and no-mutation cases with focused backend tests.
- [x] 1.2 Preserve and regress publication-time revalidation for current Result/Validation identity, mandatory findings, override scope, required sidecars, and destination constraints; prove AutoPublisher cannot use Publisher-only authorization.

## 2. Publisher Interface

- [x] 2.1 Add typed eligibility API support and replace legacy-status filtering with backend results; show actionable blocked reasons and `ALLOWED_BY_OVERRIDE` with a confirmation dialog and required operator reason.
- [x] 2.2 Add focused frontend regressions for override status, blocked reasons, confirmation/reason submission, and UI refresh after publication without Validation history mutation.

## 3. Validation

- [x] 3.1 Run strict OpenSpec validation, focused backend/frontend tests, and `./scripts/verify.sh --auto`; finish only when the canonical verifier reports `ALL CHECKS PASSED`.
