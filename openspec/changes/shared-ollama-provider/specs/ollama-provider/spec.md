## Purpose

Provide one backend-owned Ollama connection and model catalog that Translation, OCR AI Assisted, Copilot, and future consumers can share without sharing or duplicating their model-selection policy.

## ADDED Requirements

### Requirement: One canonical Ollama connection

The system SHALL persist one canonical Ollama service endpoint in the shared `ollamaProviderSettings` setting. The shared connection SHALL NOT own feature-selected model IDs or Primary/Fast/Fallback role policy. Endpoint validation SHALL preserve the existing supported HTTP/HTTPS, host-required, no-URL-credentials, and no-implicit-localhost behavior. Ollama requests from every backend consumer SHALL resolve their endpoint from this shared connection.

For compatibility with the current Settings client, the Settings API SHALL project the canonical endpoint into the deprecated `subtitleTranslationSettings.ollamaEndpoint` response field and SHALL route updates received through that field to the shared setting. The projected field SHALL NOT be persisted as a second endpoint. New clients SHALL read and update the shared setting directly.

#### Scenario: Consumers share one endpoint and choose models independently
- **WHEN** Translation, OCR AI Assisted, or Copilot invokes Ollama with its own selected model ID
- **THEN** each backend request uses the shared endpoint and the consumer's supplied model ID, without creating or changing another consumer's model selection

#### Scenario: Existing Settings client updates the legacy endpoint field
- **WHEN** the current Settings client saves `subtitleTranslationSettings.ollamaEndpoint`
- **THEN** the backend updates the canonical shared endpoint, returns the projected value on subsequent reads, and stores no duplicate endpoint in `subtitleTranslationSettings`

#### Scenario: Shared endpoint changes
- **WHEN** the canonical endpoint changes
- **THEN** readiness evidence tied to the prior endpoint is stale for every consumer, and unrelated settings such as Jellyfin configuration remain unchanged

### Requirement: Backend connection test is separate from model readiness

The backend SHALL expose `POST /api/settings/ollama/test-connection` using the saved shared endpoint. The response SHALL include a bounded state equivalent to `reachable`, `unavailable`, or `invalid_configuration`, a check timestamp, and a sanitized diagnostic when needed. This operation SHALL establish endpoint reachability only; it SHALL NOT require a selected model, generate inference, change any feature's enablement, or imply that a model is installed, resident, or inference-ready. The browser SHALL call the MVForge API and SHALL NOT contact Ollama directly.

#### Scenario: Ollama endpoint is reachable
- **WHEN** a connection test reaches the configured Ollama service
- **THEN** the backend reports `reachable` without testing a model or changing Translation, OCR, or Copilot enablement

#### Scenario: Ollama endpoint is unavailable
- **WHEN** the configured Ollama service cannot be reached or returns an invalid connection response
- **THEN** the backend reports a bounded actionable failure state and does not present the failure as an empty installed-model list or mutate unrelated settings

### Requirement: Installed models are discovered through the backend

The backend SHALL expose `GET /api/settings/ollama/models` and obtain its installed-model catalog from Ollama `GET /api/tags`. A successful response SHALL include a discovery timestamp and model entries with stable model IDs/names plus bounded installed metadata when supplied by Ollama, including digest, size, modification time, and model details where available. An empty catalog SHALL be distinguishable from an unavailable, invalid, malformed, or oversized response. The backend SHALL NOT pull or download models as part of discovery.

The catalog SHALL describe installation only. It SHALL NOT claim that a model is resident in memory or ready to run inference based on `/api/tags` data. Discovery SHALL be an explicit bounded request and SHALL NOT issue one `/api/show` request per catalog entry.

#### Scenario: Installed model catalog is returned
- **WHEN** the configured Ollama service returns a valid bounded `/api/tags` response
- **THEN** the backend returns the installed model IDs and available metadata with a discovery timestamp

#### Scenario: No models are installed
- **WHEN** Ollama returns a valid `/api/tags` response with no models
- **THEN** the backend reports a successful empty catalog that is distinct from a connection or parsing failure

#### Scenario: Catalog request fails
- **WHEN** Ollama is unavailable or `/api/tags` returns an HTTP error, malformed JSON, or a response beyond the configured bound
- **THEN** the backend returns a bounded failure state and sanitized diagnostic without returning a false successful empty catalog or raw response body

#### Scenario: Installed model is not resident
- **WHEN** a model appears in the installed-model catalog but is not loaded by Ollama
- **THEN** the catalog reports it as installed and makes no residency or inference-readiness claim

### Requirement: Per-model readiness is independent and accurately named

The backend SHALL expose `POST /api/settings/ollama/model-readiness` with a model ID in the request body. It SHALL check only that model through the bounded Ollama model-information request and return a model-specific state equivalent to `available`, `model_not_found`, `unavailable`, `invalid_configuration`, or `invalid_response`, together with the checked model ID, timestamp, and sanitized diagnostic when needed. `available` SHALL mean the configured service confirmed model metadata; it SHALL NOT mean that the model is resident or that inference has been executed successfully. A model check SHALL NOT alter another model's readiness or any feature enablement.

The existing `POST /api/settings/subtitle-translation/test-connection` contract SHALL remain available for compatibility. It SHALL resolve the shared endpoint and the saved Translation model, map the model-specific result into the existing Translation readiness response, and preserve its explicit readiness and enablement rules. Normal Translation work SHALL NOT add an automatic connection or model preflight.

#### Scenario: Selected model metadata is available
- **WHEN** a per-model readiness check succeeds for the requested model ID
- **THEN** the backend reports that model as available with a timestamp and does not claim residency or successful inference

#### Scenario: Selected model is missing
- **WHEN** the configured service is reachable but the requested model is not available
- **THEN** the backend returns `model_not_found` for that model without changing other model or feature state

#### Scenario: One model check does not rewrite feature choices
- **WHEN** a readiness check is run for a model selected by one consumer
- **THEN** the backend leaves all saved model IDs, model roles, and enablement values unchanged

### Requirement: Legacy Translation configuration migrates without losing user state

Migration from `subtitleTranslationSettings` SHALL be idempotent and atomic. When only the legacy `ollamaEndpoint` exists, the system SHALL move it into the shared provider setting and remove it from persisted Translation settings only after the shared write succeeds. It SHALL preserve the saved `ollamaModel`, Translation enablement, translation limits, and readiness evidence only when its endpoint/model identity still matches the migrated configuration. Existing role/model fields unknown to this migration SHALL be preserved. If no usable legacy or shared endpoint exists, Translation SHALL remain in its current unconfigured/disabled-by-default state without inventing a localhost value.

When legacy and shared endpoints are equal, migration SHALL collapse the legacy duplicate while retaining safe Translation state. When they differ, migration SHALL report an explicit `migration_conflict`, preserve both user values and the saved model/enablement state, and block provider use until the canonical endpoint is explicitly resolved. It SHALL NOT silently choose, overwrite, or discard either value. Resolving the conflict SHALL leave one persisted endpoint and invalidate readiness whose identity no longer matches. Migration SHALL NOT change unrelated Settings records.

#### Scenario: Legacy-only Translation endpoint is migrated
- **WHEN** `subtitleTranslationSettings` contains a valid endpoint/model configuration and no shared endpoint exists
- **THEN** the shared endpoint is created, the Translation model and enablement are retained, safe matching readiness and translation limits are retained, and the legacy endpoint is no longer persisted in the Translation setting

#### Scenario: Migration is retried after a partial failure
- **WHEN** migration is run again after a failed or interrupted attempt
- **THEN** it completes without duplicating or resetting the endpoint, model, enablement, limits, or readiness evidence

#### Scenario: Shared and legacy endpoints match
- **WHEN** both settings contain the same endpoint
- **THEN** migration leaves one persisted endpoint and preserves Translation readiness only when its saved identity still matches

#### Scenario: Shared and legacy endpoints conflict
- **WHEN** both settings contain different endpoints
- **THEN** the backend exposes `migration_conflict`, makes no Ollama request through either value, and preserves the old and new endpoint values and Translation state until explicit resolution

#### Scenario: Conflict is explicitly resolved
- **WHEN** the user saves the chosen canonical endpoint through the provider Settings API
- **THEN** the conflict clears, only the shared endpoint remains persisted, and stale model readiness must be retested before dependent work proceeds

#### Scenario: Existing Translation model selection is preserved
- **WHEN** a legacy Translation setting is migrated, including one with future role fields already present
- **THEN** its saved model selection and role fields remain unchanged for #223 to interpret, with no automatic model switch

### Requirement: Shared provider contracts remain compatible with model roles and Settings consumers

The shared provider API SHALL treat a model ID as an opaque consumer-supplied selection backed by the discovered catalog. It SHALL NOT choose a Primary, Fast, or Fallback role, decide fallback eligibility, assign feature defaults, or silently replace a missing model. The #223 role policy SHALL consume the shared catalog and per-model check while owning role selection and fallback semantics. The #266 Settings UI SHALL consume the MVForge discovery/readiness APIs for provider and feature settings; it SHALL NOT add another Ollama endpoint or contact Ollama directly. Multiple features MAY select the same installed model independently.

#### Scenario: Model roles use the shared provider
- **WHEN** the #223 role policy checks a Primary, Fast, or Fallback model
- **THEN** each check uses the shared endpoint and returns model-specific readiness without the shared provider choosing or rewriting a role

#### Scenario: Settings selects an installed model
- **WHEN** the #266 Settings UI loads its model inventory
- **THEN** it receives the backend-discovered catalog and can bind the same model ID independently to multiple features without creating a second endpoint

#### Scenario: A selected model is missing or the provider is offline
- **WHEN** a saved feature selection is absent from the latest successful catalog or the provider is unavailable
- **THEN** the backend reports the selection/provider as unavailable and does not auto-switch the selection or silently enable the feature

### Requirement: Ollama provider requests are bounded and secret-safe

Connection, discovery, and model-readiness requests SHALL use bounded timeouts and response sizes. They SHALL preserve cancellation, classify transport, timeout, HTTP, malformed-response, and oversized-response failures, and return diagnostics that exclude credentials, authorization data, raw provider response bodies, and unbounded model metadata. Endpoint validation SHALL allow supported NAS/LAN/container addresses without assuming localhost. No provider operation in this capability SHALL download a model.

#### Scenario: Provider response exceeds its bound
- **WHEN** `/api/tags` or a model-information response exceeds its endpoint-specific response limit
- **THEN** the backend returns a typed bounded failure and does not parse or expose an unbounded body

#### Scenario: Provider check is cancelled or times out
- **WHEN** the caller cancels a provider request or the configured timeout expires
- **THEN** the request terminates within its bounded operation and returns the corresponding cancellation or timeout classification

#### Scenario: Provider returns sensitive or malformed diagnostics
- **WHEN** an Ollama error response contains credentials, private response content, or malformed JSON
- **THEN** the MVForge response contains only a bounded sanitized diagnostic and does not expose the raw body or secret
