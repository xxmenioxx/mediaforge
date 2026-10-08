## 1. Shared provider authority

- [ ] 1.1 Add the `ollamaProviderSettings` endpoint setting, current endpoint validation, and a backend resolver that accepts consumer model IDs without storing model/role choices; verify resolver and validation unit tests for valid remote HTTP/HTTPS URLs, invalid URLs, credentials, and missing configuration.
- [ ] 1.2 Route all existing Ollama Translation runtime and readiness requests through the shared endpoint while retaining the saved Translation model; verify the existing Translation readiness and provider request tests pass without adding runtime preflight calls.

## 2. Safe migration and current Settings compatibility

- [ ] 2.1 Add an idempotent atomic AppSetting migration for legacy-only, absent, equal, invalid, and conflicting endpoint states; verify temporary SQLite tests preserve Translation model, enablement, limits, matching readiness, future role fields, and unrelated settings, and prove conflicts fail closed without discarding either value.
- [ ] 2.2 Add the read/write projection for `subtitleTranslationSettings.ollamaEndpoint` so the current Settings client remains compatible while only `ollamaProviderSettings.endpoint` is persisted; verify Settings API save/refetch/remount round trips and assert there is no second persisted endpoint.
- [ ] 2.3 Verify explicit conflict resolution through the shared provider setting clears the conflict atomically, preserves the Translation model and enablement value, and requires stale readiness to be retested.

## 3. Backend Ollama connection, discovery, and model readiness APIs

- [ ] 3.1 Add the connection-only `POST /api/settings/ollama/test-connection` operation using the shared endpoint; verify reachable, invalid, unavailable, timeout, cancellation, and sanitized failure results without model checks or feature enablement changes.
- [ ] 3.2 Add `GET /api/settings/ollama/models` backed by one bounded `GET /api/tags` request; verify empty versus offline results, model ID/name and optional metadata mapping, timestamps, malformed/HTTP/oversized responses, no per-model fan-out, no raw-body leakage, and no pull requests.
- [ ] 3.3 Add `POST /api/settings/ollama/model-readiness` backed by the bounded existing `/api/show` contract; verify model-specific available/missing/error states, timeout/cancellation/redaction, and that success is not reported as residency or inference readiness.
- [ ] 3.4 Preserve `POST /api/settings/subtitle-translation/test-connection` as the compatibility adapter to the shared endpoint and saved model; verify its response shape and readiness/enablement rules remain compatible.

## 4. Consumer contract compatibility

- [ ] 4.1 Add provider contract coverage showing consumer-supplied model IDs are checked against the shared catalog without changing feature selections or adding Primary/Fast/Fallback routing; verify #223 can own role/fallback policy independently.
- [ ] 4.2 Add API contract coverage for #266 showing Settings can use the shared endpoint and installed-model catalog for independent feature selections, including the same model selected by multiple features and missing/offline models without silent selection or enablement changes.

## 5. Validation and rollout readiness

- [ ] 5.1 Run focused backend and current Settings compatibility tests, then `./scripts/verify.sh --auto`; record only the canonical verifier result and resolve any reported failures before completion.
- [ ] 5.2 Run `openspec validate shared-ollama-provider --type change --strict` and confirm the migration, API contracts, #223/#266 boundaries, and rollback guidance are represented by the validated artifacts.
