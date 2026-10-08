## Context

See `proposal.md` for the motivation and scope. Today `subtitleTranslationSettings` persists the Ollama endpoint together with Translation's model, enablement, readiness identity/timestamp/diagnostic, and request limits. The backend validates the endpoint as HTTP or HTTPS with a host and no URL credentials; it does not choose a localhost default. `translationSettingsForUpdate` rejects generic enablement, and `TestTranslationConnection` is the readiness-validated path for the saved model. The existing Settings client reads and writes `subtitleTranslationSettings.ollamaEndpoint`.

The Ollama adapter already has bounded `/api/chat` and `/api/show` requests, including a bounded model readiness result, but no model catalog route. The existing Translation readiness request posts the selected model as `{"name": "..."}` to `/api/show`. Current Ollama documentation defines `GET /api/tags` for locally available model metadata and `GET /api/version` for service version/reachability; `/api/tags` includes model name, digest, size, modification time, and optional details ([Ollama List models](https://docs.ollama.com/api/tags), [Ollama API reference](https://github.com/ollama/ollama/blob/main/docs/api.md)).

Issues #223 and #266 own role/fallback behavior and the Settings UI respectively. #261 supplies the shared endpoint, catalog, and backend readiness seam those changes consume.

## Goals / Non-Goals

**Goals:**

- Establish one persisted endpoint authority and one backend adapter used by all Ollama consumers.
- Keep provider reachability, installed model discovery, and per-model metadata readiness as distinct states.
- Migrate Translation configuration without losing its chosen model, enablement, request limits, or readiness that still matches.
- Keep the current Settings client working until #266 adopts the shared provider setting.
- Provide bounded MVForge APIs that #223 and #266 can reuse without placing role or feature policy in the provider.

**Non-Goals:**

- Add Settings UI, change Settings navigation, or add OCR/Copilot feature settings.
- Choose feature models, define Primary/Fast/Fallback roles, or implement fallback behavior.
- Detect resident models, warm up/unload models, schedule inference, or perform inference as a readiness probe.
- Pull models or add another provider.

## Decisions

### Persist one provider connection; keep model choices with consumers

Persist the shared connection under the `ollamaProviderSettings` AppSetting key, with `endpoint` as its sole active provider value. Do not add a provider-wide selected model or copy the endpoint into Translation, OCR, Copilot, or role records. There is no Ollama credential setting in the current implementation, so this change adds none. If a future supported authentication mode needs a credential, only a protected credential reference may be added; credentials must not be placed in the URL or ordinary Settings response.

Use the existing endpoint validation rules and preserve NAS/LAN/container addresses. A consumer passes its model ID to the shared provider adapter; the adapter resolves the endpoint from `ollamaProviderSettings`. Keep model IDs and role fields in consumer-owned settings so #223 and #266 can evolve them independently.

The current Settings client is a temporary compatibility constraint. Its Settings API response projects the shared endpoint back into `subtitleTranslationSettings.ollamaEndpoint`; an update carrying that legacy field writes the shared setting through the same backend validation and transaction. The projection is computed for compatibility and is not stored in the Translation value. New clients use `ollamaProviderSettings` directly. This lets #261 change backend ownership without changing the existing Settings UI.

**Alternative rejected:** leave the endpoint in Translation and have other features read it. That would make Translation the hidden owner of unrelated providers and keep future clients coupled to a feature-specific setting.

### Separate service connection, model inventory, and model metadata readiness

Expose these MVForge APIs, backed only by server-side calls to the configured Ollama service:

| MVForge API | Ollama operation | Contract |
| --- | --- | --- |
| `POST /api/settings/ollama/test-connection` | `GET /api/version` | Tests service reachability only; returns `state`, `checkedAt`, and an optional bounded diagnostic. |
| `GET /api/settings/ollama/models` | `GET /api/tags` | Returns `state`, `discoveredAt`, and installed models with `id`, `name`, and bounded optional metadata (`digest`, `sizeBytes`, `modifiedAt`, and selected `details`). |
| `POST /api/settings/ollama/model-readiness` | `POST /api/show` | Accepts `{ "modelId": "..." }`; returns `modelId`, `state`, `checkedAt`, and an optional bounded diagnostic. |

Use response states that distinguish `available`/reachable, `model_not_found`, `unavailable`, `invalid_configuration`, and `invalid_response`. A successful tags response with zero entries is a successful empty inventory, not an outage. Keep the existing `POST /api/settings/subtitle-translation/test-connection` route and response shape; it resolves the shared endpoint, checks the saved Translation model, and maps the result into the existing Translation readiness states. It must remain an explicit readiness operation, not a preflight added to normal Translation requests.

Use `available` for successful `/api/show` metadata only. `/api/tags` establishes installation; `/api/show` establishes model metadata availability. Neither reports that a model is loaded or that inference succeeds. Do not call `/api/ps` or `/api/chat` for these contracts. The provider layer returns a per-request result rather than adding a global readiness registry; consumer-owned settings continue to hold any enablement/readiness evidence they already own.

**Alternative rejected:** treat `/api/tags` as per-model readiness or run a generation to test every model. The former overstates readiness, while the latter has workload and memory costs and can load models as a side effect.

### Bound catalog work and keep diagnostics safe

Make discovery explicit and issue one `/api/tags` request for the whole inventory; do not issue `/api/show` once per listed model. Parse and return only fields needed for a model picker and safe diagnostics. Apply request timeouts, cancellation, and endpoint-specific bounded response reads. Reuse the current model readiness limit for `/api/show` (256 KiB, retained after #252 accepted valid larger model metadata); give `/api/tags` its own response budget because it is a list. The model catalog is returned per request with a server timestamp; do not add a persisted or periodically refreshed copy in #261.

Return typed failure states for transport, timeout/cancellation, non-success HTTP responses, malformed JSON, and response-limit breaches. Do not expose provider response bodies, credentials, authorization data, Modelfiles, or other oversized metadata in diagnostics. Do not call `/api/pull`.

### Migrate endpoint state atomically and preserve Translation settings

Run an idempotent AppSetting migration transaction before provider requests are served. Read `ollamaProviderSettings` and `subtitleTranslationSettings` together and handle these cases:

1. **Legacy only:** validate and copy the legacy endpoint to `ollamaProviderSettings.endpoint`; preserve Translation's model, enablement, limits, and readiness only if the existing readiness identity matches the same endpoint/model; then remove `ollamaEndpoint` from the persisted Translation value.
2. **No legacy endpoint or shared endpoint:** leave the provider unconfigured and preserve Translation's existing disabled-by-default behavior. Never synthesize localhost.
3. **Equal legacy and shared endpoint:** keep the shared value, remove the duplicate legacy persisted field, and preserve only matching readiness.
4. **Different endpoints:** keep both values intact temporarily, surface `migration_conflict`, and fail closed for Ollama calls. Do not select, overwrite, or discard either endpoint. An explicit provider-setting or legacy compatibility-field save chooses the canonical endpoint, removes the legacy copy in the same transaction, clears the conflict, and requires readiness to be checked again when identity changed.

If a transaction fails, leave its input values unchanged so startup or the next settings request can retry. Preserve unrecognized consumer model/role fields while normalizing Translation settings; this migration owns only the endpoint field. An existing explicit enablement value remains saved through migration. Where readiness is stale or a conflict exists, the current Translation readiness/enablement gate continues to prevent provider work until its existing explicit checks succeed.

**Alternative rejected:** silently prefer either the old or shared value on mismatch. That risks sending media-related work to an endpoint the user did not choose and loses the other saved value.

### Keep the #223 and #266 contracts composable

The model catalog returns IDs; it does not define a role schema or feature selection. #223 can map Primary/Fast/Fallback role IDs to the catalog and call per-model readiness while retaining its own fallback rules. #266 can show the shared endpoint and catalog in Settings and retain independent feature model choices. The same installed model may be selected for multiple features. Missing or offline selections are reported unavailable; the provider does not silently replace them or toggle unrelated feature settings.

**Alternative rejected:** persist role or per-feature models in `ollamaProviderSettings`. That would make the shared connection setting a competing selection source and conflict with both follow-up issues.

## Risks / Trade-offs

- **Existing Settings client still sends the legacy endpoint field** → Keep a read/write projection until #266 adopts the shared key; test round trips and ensure the projected value is never persisted twice.
- **Conflicting endpoints block AI provider use until a choice is made** → Preserve both values and model/enablement state, return an explicit conflict status, and let the existing compatibility field or #266's provider form resolve it.
- **Large `/api/tags` response exceeds a conservative limit** → Bound memory and return a typed diagnostic; set a separate catalog limit from fixture evidence and test an over-limit body.
- **Installed or metadata-available is mistaken for inference-ready** → Name the API state `available`, document its exact evidence, and expose no residency/inference-ready state in this change.
- **Rollback to an older binary expects the legacy endpoint field** → Capture an AppSetting backup before migration; rollback across the data move requires restoring that snapshot or applying a forward compatibility fix. The migration transaction protects against partial writes but is not a reverse migration.
- **A shared endpoint outage coincides with unrelated Settings work** → Keep provider failures scoped to provider/readiness responses; do not update or invalidate Jellyfin or other independent settings.

## Migration Plan

1. Deploy the backend with shared setting resolution, the bounded API endpoints, and the legacy Settings API projection in place.
2. Run the AppSetting migration transaction before serving Ollama requests. Normal legacy-only installations retain their selected model and enabled state; mismatched old/new endpoints report a conflict and make no Ollama requests until resolved.
3. Keep the legacy projection while the current Settings page is in use. #266 switches the UI to `ollamaProviderSettings` and these backend APIs; #223 consumes the catalog and model check while retaining role policy.
4. Remove the deprecated projection only in a later compatibility cleanup after all supported clients use the shared key. Do not bundle that removal with #261.
5. For rollback to a pre-migration binary, restore the backed-up AppSetting values. Do not attempt a partial rollback that leaves the old binary without its endpoint.

## Open Questions

None. The shared key, conflict policy, readiness evidence, compatibility adapter, and endpoint contracts are defined above; exact response byte budgets for the new tags endpoint can be set from bounded fixture tests without changing these contracts.
