## 1. Persisted dictionary and management API

- [x] 1.1 Add the bounded SQLite/GORM dictionary-entry model, migration, indexes, immutable IDs, version/audit fields, and idempotent English built-in seeding; verify migration, seed idempotency, and restart persistence with focused backend tests.
- [x] 1.2 Implement bounded CRUD, search/filter, enable/disable, user-rule validation, and built-in mutation protection; verify paging, supported-language validation, duplicate/collision responses, and built-in protections with handler tests.

## 2. Contextual matching and canonical integration

- [x] 2.1 Preserve raw language authority separately from the existing OCR fallback and add strict dictionary-language eligibility; verify English aliases, unknown language, Spanish, Japanese, and legacy snapshots with focused tests.
- [x] 2.2 Implement the bounded literal matcher, explicit case and word-boundary policies, English starter rules, preview/test-match, and deterministic priority conflict handling; verify contractions, `am |`, punctuation, casing, proper names, words, ASS markup, separators, and equal-priority conflicts with table-driven tests.
- [x] 2.3 Route candidates through the existing canonical OCR analyzer/correction and Translation preparation paths; verify both consumers produce the same validated cue changes, unmatched bars remain reviewable, and dictionary rules do not bypass the OCR gate or trigger a second OCR pass.

## 3. Canonical OCR re-evaluation and publication

- [x] 3.1 Add snapshot-based re-evaluation preview with cue diffs, quality counts, artifact revision, dictionary revision, and an explicit language choice for ambiguous legacy snapshots; verify preview bindings, unchanged published files/snapshots, and no PGS/VobSub OCR invocation.
- [x] 3.2 Apply accepted candidates through the existing correction operation with stale revision checks, rule/version and before/after hash provenance, and bounded predecessor snapshot history; verify successful re-analysis/rollback and stale, failed, or cancelled operations preserve the prior authoritative baseline.
- [x] 3.3 Preserve serializer and publication integrity for SRT, ASS, and logical `both`; verify cue identity/order/timing, ASS tags/styles/geometry/positioning, common corrected text, and all-output atomic rollback with focused regression tests.

## 4. Isolated dictionary component

- [x] 4.1 Add API types/client and an embeddable dictionary component for search, custom-rule CRUD, built-in enablement, validation, and test-match preview; verify component states and interactions with focused frontend tests and leave Settings page/navigation integration to issue #266.

## 5. Integration validation

- [x] 5.1 Run strict OpenSpec validation and `./scripts/verify.sh --auto`; resolve only reported failures and require `ALL CHECKS PASSED` before implementation completion.
