## 1. Settings definition registry

- [x] 1.1 Add canonical key/domain definitions with current validators, normalizers, response projections, and transactional effects.
- [x] 1.2 Route single-key writes through the registry-backed transaction service while preserving existing keys and JSON values.

## 2. Atomic updates and protected credentials

- [x] 2.1 Add the batch endpoint and client operation; make the existing multi-key Settings save atomic.
- [x] 2.2 Add the protected credential source and move Jellyfin credential reads/writes out of AppSetting, including compatible legacy migration and redaction.
- [x] 2.3 Cover normalization, AppSetting compatibility, rollback, secret redaction, and Settings/Asset Defaults regressions.

## 3. Settings domain UI

- [x] 3.1 Split Settings rendering into domain components while keeping existing sections URL-addressable.
- [x] 3.2 Add General and Providers & Integrations, retaining MVForge preferences and Jellyfin integration behavior in their domain sections.
- [x] 3.3 Build the frontend and lint only changed frontend files.

## 4. Final validation

- [x] 4.1 Run focused backend and frontend regressions for this change.
- [x] 4.2 Run `openspec validate settings-domain-foundation --type change --strict`.
- [x] 4.3 Run `./scripts/verify.sh --auto`; result: `ALL CHECKS PASSED` (full scope, 6/6 checks).
