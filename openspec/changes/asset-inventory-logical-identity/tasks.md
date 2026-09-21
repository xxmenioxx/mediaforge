## 1. Reproduce and define display identity

- [ ] 1.1 Add focused Library fixtures for a logical root group and a nested child path (for example Berserk root and Attack on Titan/Season1) that reproduce the current `Root/full-path` and `Season1/full-path` rendering.
- [ ] 1.2 Add expectations that Library and Converted collapsed rows use logical title/group identity as the primary label while path search still matches the same records.
- [ ] 1.3 Add a regression expectation that Unprocessed continues to expose Root/Season/path labels beneath its logical title hierarchy.

## 2. Implement mode-aware inventory display

- [ ] 2.1 In `frontend/src/pages/AssetsPage.tsx`, introduce or reuse a small display resolver that separates logical inventory identity from operational path identity.
- [ ] 2.2 Use logical-title-first rendering for Library and Converted rows; do not use the absolute path as the default collapsed-row subtitle.
- [ ] 2.3 Preserve `pathLabelForCollection()` and full path data for path-scoped configuration, Queue notes, snapshots, expanded details, search, and other operational surfaces.

## 3. Guard architecture boundaries

- [ ] 3.1 Verify no backend, database, grouping, lifecycle, profile-assignment, Queue, Publisher, or filesystem behavior changes are required.
- [ ] 3.2 If current API data cannot reliably resolve logical identity, stop and document the missing canonical field before adding a backend workaround; do not infer identity from arbitrary path slicing when stronger logical-group evidence exists.

## 4. Validation

- [ ] 4.1 Run the focused Assets inventory display tests.
- [ ] 4.2 Verify Library root, Library nested path, Converted, Unprocessed hierarchy, filtering/search, and row expansion behavior.
- [ ] 4.3 Run `./scripts/verify.sh --auto` and verify it reports `ALL CHECKS PASSED`.
