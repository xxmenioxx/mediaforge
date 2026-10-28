## 1. Persistence and policy

- [x] 1.1 Add bounded memory/evidence models, migration, and default-off independent settings; verify existing-database migration and validation tests.
- [x] 1.2 Implement bounded dedupe, retention, scope resolution, search, edit/revoke/delete/export, and feedback policy; verify focused SQLite/API tests.

## 2. OCR integration

- [x] 2.1 Capture only committed approved manual and dictionary corrections with exact revisions and provenance; verify success, duplicate, stale, cancellation, and unapproved-output regressions.
- [x] 2.2 Add deterministic same-language scoped suggestions and separately opted-in bounded OCR AI prompt enrichment; verify model-free lookup, no cross-language results, and advisory-only behavior.

## 3. User controls and validation

- [x] 3.1 Add subtitles settings opt-in and correction-memory manager with search, export, feedback, edit, disable, and delete; verify focused frontend interaction tests.
- [x] 3.2 Run focused backend/frontend checks and `./scripts/verify.sh --auto`; require `ALL CHECKS PASSED` before completion.
