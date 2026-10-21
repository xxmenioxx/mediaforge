## 1. Backend Translation Guard

- [x] 1.1 Reuse the canonical subtitle lifecycle action in capability and start source resolution; preserve exact-source, root, archive, and revision guards, and verify active Queue/maintenance work returns a concrete blocker.
- [x] 1.2 Add focused backend coverage for published lifecycle variants, active work, missing/unsupported/archive sources, provider readiness reasons, and exact sidecar identity; verify the translation source/capability tests pass.

## 2. Asset Info Translation Controls

- [x] 2.1 Remove only the general row editing lock from embedded/external Translate controls, preserve lifecycle action gating and adjacent mutation guards, and expose a specific reason when translation is lifecycle-blocked; verify Asset Info integration tests across converted, published-as-is, accepted, and active states.
- [x] 2.2 Verify provider-unavailable and source-ineligible capability responses appear inside the dialog with Start disabled, while exact external and embedded source selection remains unchanged.

## 3. Validation

- [x] 3.1 Run issue-specific UI and backend tests, then run `./scripts/verify.sh --auto` through the serialized shared-validation lock; verify the canonical output reports `ALL CHECKS PASSED`.
