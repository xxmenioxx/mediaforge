## 1. Archive identity and deletion performance

- [x] 1.1 Bound current-publication projection reads to the requested Library asset and linked Queue, source, and Archive records; verify scoped counts and SQLite query plans with the projection regression tests.
- [x] 1.2 Preserve explicit confirmation, per-item batch outcomes, and fresh pre-unlink identity, active-work, root, symlink, and Library-file checks; verify stale and unsafe cases leave the original intact.
- [x] 1.3 Record backend phase timings and compare global versus scoped projection work with the Archive deletion benchmark.

## 2. Published path and operator feedback

- [x] 2.1 Present the current Publisher path as the Library location and retain the original as separate provenance; verify inventory after publication.
- [x] 2.2 Refresh Assets on a newly observed current PUBLISHED path and measure request, inventory refresh, and feedback phases; verify focused frontend tests.

## 3. Validation

- [x] 3.1 Run focused backend and frontend regression tests and strict OpenSpec validation for this change.
- [x] 3.2 Run ./scripts/verify.sh --auto through the serialized dupla lock and record the final result.
