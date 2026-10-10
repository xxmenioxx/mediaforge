## 1. Canonical identities

- [x] 1.1 Add the versioned reusable profile-definition identity at the source profile capture boundary and preserve it across Queue freeze; verify assignment-source stability and sensitivity to real definition/version changes.
- [x] 1.2 Add the effective-profile fingerprint and candidate `ConfigurationHash` from frozen post-resolution values; verify T105-style automatic decisions change execution identity without changing definition identity, while runtime paths/IDs do not contaminate it.

## 2. Test Encode and Qualification

- [x] 2.1 Persist definition and effective-profile identities with requested/effective snapshots and Qualification rows through the existing migration path; verify legacy rows remain unchanged and report an explicit missing-identity cause.
- [x] 2.2 Validate frozen profile, execution, source, candidate/generation, SamplingPlan, #69 evidence/reference, and policy identities with stage-specific diagnostics; verify positive current evidence and negative stale/forged evidence never produce a false `PASS`.

## 3. Queue Preflight

- [x] 3.1 Resolve and fingerprint Preflight's frozen profile through the shared effective video resolvers and match both definition and effective-profile identities; verify exact matching evidence appears and real profile/override changes are rejected with a cause.
- [x] 3.2 Preserve exact source-asset and Test Encode generation boundaries through Preflight acceptance; verify evidence cannot transfer between assets and acceptance retains the reviewed qualification without re-encoding.

## 4. Documentation and validation

- [x] 4.1 Document the three identity layers, capture stages, and legacy behavior in the Queue Preflight architecture guide; verify the document matches the implemented snapshot/evidence flow.
- [x] 4.2 Run focused backend regressions, `git diff --check`, and `openspec validate frozen-profile-fingerprint-consistency --strict`; fix reported failures and rerun the failing check.
- [x] 4.3 Run `./scripts/verify.sh --auto` and require `ALL CHECKS PASSED` before committing and pushing the feature branch.

## Verification results

- Focused handler regressions for T105, profile fingerprints, Qualification currentness/persistence/legacy handling, and Queue Preflight passed.
- `git diff --check` passed.
- `openspec validate frozen-profile-fingerprint-consistency --strict` passed.
- `./scripts/verify.sh --auto` passed the full scope, 6/6 checks, with `ALL CHECKS PASSED`.
