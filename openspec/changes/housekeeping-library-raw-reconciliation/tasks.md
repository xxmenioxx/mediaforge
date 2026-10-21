## 1. Maintenance temporary evidence and safety

- [ ] 1.1 Classify active, completed, failed, canceled, interrupted, and ambiguous maintenance operations; verify each preview reason and presence/size state with disposable SQLite/filesystem fixtures.
- [ ] 1.2 Add narrowly scoped Library temporary ownership and recovery checks; verify only exact eligible manual residuals can enter the existing Housekeeping preview.
- [ ] 1.3 Revalidate operation, asset, backup, references, roots, symlink/hardlink state, and file identity at execution; verify stale/replaced candidates remain intact and retries are idempotent.
- [ ] 1.4 Persist/report deferred temporary cleanup failures without changing operation history; verify warning behavior using injected runtime failures.

## 2. Empty RAW directory protection evidence

- [ ] 2.1 Return bounded structured protection references for AssetRecord, Queue, reservation, Publisher, and maintenance sources; verify identifiers, fields, lifecycle states, stale-suspected labeling, and query failure blocking.
- [ ] 2.2 Preserve empty-only non-recursive execution and revalidation of roots, ancestors, symlinks, entries, identity, references, and role overlap; verify fixtures for current, stale-suspected, and unreferenced leaves.

## 3. Operator presentation and integration

- [ ] 3.1 Extend existing Housekeeping preview types and Settings presentation with operation state, actual evidence, protection references, eligibility, and actionable blockers; verify loading, no-op, blocked, and eligible states.
- [ ] 3.2 Run strict OpenSpec validation and focused backend/frontend regressions, then `./scripts/verify.sh --auto`; require ALL CHECKS PASSED or document an external sandbox limitation.
