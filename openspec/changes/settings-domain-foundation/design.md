## Context and Current Behavior

- `models.AppSetting` is a JSON value row keyed by the persisted setting name.
- `SettingsHandler.Update` owns key-specific validation, normalization, upsert, and side effects in one method. The ordinary list endpoint serializes stored rows, with a Jellyfin-only projection that omits the API token.
- Settings UI section navigation is already query-string addressable. The core save performs ten independent POST requests in parallel, so failures can leave a partial configuration update.
- Jellyfin currently persists `apiToken` in the `jellyfin` AppSetting value even though its API projection redacts it.
- No Settings architecture document exists in `docs/architecture/`; this design follows the current handler/model/UI code.

## Decisions

### 1. Keep the persisted setting contract and centralize policy

Retain `AppSetting` fields, JSON values, and existing setting keys. Add a registry definition per known key containing its UI domain, validator, normalizer, response projection, and optional transactional update effect. Unknown legacy keys use an identity-compatible legacy definition so the existing generic single-key API does not break. Both the existing `POST /settings/:key` and new batch endpoint call one service path.

Validators and normalizers receive the active GORM transaction where existing behavior depends on database state. The definitions own current key-specific rules; `SettingsHandler.Update` becomes transport and status mapping only. Pipeline automation, DirectPlay, and runtime-policy effects run in the same transaction as setting writes.

### 2. Make batch saves atomic

Add `POST /settings/batch` accepting a list of `{key, value}` entries. Reject malformed, duplicate, or invalid entries before commit. Normalize all entries and apply their upserts and registered effects inside one SQLite transaction. Any validation, write, or effect failure rolls back every setting and effect. Single-key updates use the same transaction path with one entry.

The existing Settings core save will send its ten values in one batch and invalidate the same queries only after success.

### 3. Keep credentials outside AppSetting

Add a `CredentialRef` value and a `ProtectedSecretSource` interface backed by a dedicated GORM model/table. Jellyfin's persisted non-secret setting contains the reference, while normal responses expose only `credentialConfigured`. Setting writes accept the existing `apiToken` and `clearCredential` input fields for compatibility, then store or clear the value through the protected source in the same transaction. Existing rows with a legacy `apiToken` are read compatibly and migrated to the protected source when listed or next updated; the legacy value is removed from `AppSetting`. Integration reads resolve the reference through the protected source with a legacy fallback during migration.

### 4. Add domain UI composition without changing settings behavior

Extend the existing section union and query-string parsing with `general` and `providers-integrations`. Place the existing MVForge preferences card in General and the existing Jellyfin integration card in Providers & Integrations. Keep other existing domains, cards, save keys, and deep-link semantics. Providers & Integrations remains a foundation for future integrations and adds no new provider implementation.

### 5. Preserve Asset Defaults and Queue semantics

Register Asset Defaults as its existing domain and reuse its current validator and persisted key. Do not change the effective resolver, precedence, freeze point, or any Queue snapshot inputs. Registry metadata is descriptive for Settings ownership and does not become a competing source for Asset Defaults resolution.

## Data and Migration

Add one secret-source table through the existing GORM migration path. No `AppSetting` schema change or key rewrite is required. Legacy Jellyfin credentials are moved idempotently while preserving the configured/not-configured state and Jellyfin behavior. All non-secret persisted JSON values remain unchanged except the Jellyfin credential is replaced with its stable `credentialRef`.

## Risks and Trade-offs

- A batch now fails as a unit if any member fails; this is intentional and replaces the current partial-save behavior for the Settings core save.
- The protected source is an API and persistence boundary; encryption-at-rest or external secret managers are not introduced by this foundation.
- Legacy Jellyfin settings may remain in their old shape until the Settings list or Jellyfin update migration path runs; all API projections redact the legacy field during that period.
