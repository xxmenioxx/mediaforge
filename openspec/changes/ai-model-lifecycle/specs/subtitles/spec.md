## MODIFIED Requirements

### Requirement: Translation provider readiness and explicit enablement

Translation SHALL be disabled by default. Configuration, readiness, and enablement SHALL remain distinct backend-authoritative states. A configured provider SHALL not become usable merely because its endpoint and model are non-empty. The backend SHALL provide an explicit Test Connection operation for the configured provider. For Ollama, readiness SHALL verify reachability of the configured endpoint and availability of the configured model through a provider-native lightweight readiness/model request, without generating a subtitle translation or requiring GPU capability. The frontend SHALL not probe the provider or infer readiness.

The backend SHALL support bounded states equivalent to disabled, not_configured, untested, ready, unavailable, model_not_found, and invalid_configuration, with secret-safe diagnostics. A readiness timestamp, if exposed, SHALL be informational and SHALL NOT be treated as a live health guarantee. Explicit enablement SHALL require successful readiness for the current configuration. Changing provider, endpoint, or model SHALL invalidate readiness and enablement.

#### Scenario: Translation disabled by default

- **WHEN** translation settings are absent or newly initialized
- **THEN** translation is disabled and Asset Info cannot start provider work

#### Scenario: Configured but untested provider

- **WHEN** an Ollama endpoint and model are configured but have not passed readiness testing
- **THEN** translation remains disabled and backend capabilities report that provider verification is required

#### Scenario: Successful Ollama readiness

- **WHEN** Test Connection reaches the configured Ollama endpoint and confirms that the configured model is available
- **THEN** readiness becomes ready, no subtitle translation is generated, and translation remains disabled until explicitly enabled

#### Scenario: Missing model

- **WHEN** the configured Ollama endpoint is reachable but the configured model is unavailable
- **THEN** readiness reports a bounded actionable model_not_found state and translation cannot be enabled

#### Scenario: Enable after readiness

- **WHEN** the current provider configuration passes readiness testing and the user explicitly enables translation
- **THEN** backend capabilities may expose translation as available for an otherwise eligible subtitle

#### Scenario: Configuration change invalidates readiness

- **WHEN** provider, endpoint, or model changes after successful verification
- **THEN** the prior readiness and enablement are invalidated and translation cannot run until the new configuration is tested successfully and enabled

#### Scenario: Runtime outage does not trigger a preflight loop

- **WHEN** an enabled provider becomes unavailable or its selected model is unloaded after setup
- **THEN** Translation uses the shared lifecycle to observe /api/ps and perform bounded on-demand warmup only after #265 admission; without an admission grant it issues no warmup or inference request, does not repeat /api/show as a health loop, and does not silently change models

## ADDED Requirements

### Requirement: Translation waits within its existing operation identity

An explicit Asset Info Translation operation SHALL remain owned by the existing subtitle operation lifecycle. Before returning an accepted operation ID, the backend SHALL persist a bounded Translation-specific SQLite record containing that original operation ID and the validated request snapshot needed to reconstruct a pre-inference wait. The record SHALL include canonical source identity/reference and source revision, while AssetPath is only a locator; source type/representation, stream index/format, exact provider/model identity, non-secret provider configuration revision, language/context/output options; checkpoint version and phase; wait deadline; cancellation state; and a compare-and-set row version. It SHALL NOT contain cues, prompts, provider responses, translated text, credentials, or endpoint URLs.

When waiting, progress SHALL identify WAITING_FOR_RESOURCES or WAITING_FOR_MODEL and the bounded cause. Missing or unavailable #265 admission SHALL be visible as admission_unavailable, cancellable, and must make no /api/generate or /api/chat request. Resource wait duration is owned by #265; the five-minute readiness deadline starts only after admission is granted.

#### Scenario: Translation reaches a model wait

- **WHEN** an explicit Translation request passes feature and source validation but awaits resource admission or model readiness
- **THEN** its original operation ID and bounded checkpoint are durable before warmup begins, and the operation remains cancellable

#### Scenario: Resource admission is unavailable

- **WHEN** #265 is missing, unavailable, or errors
- **THEN** Translation reports WAITING_FOR_RESOURCES with admission_unavailable and sends no warmup or inference request

#### Scenario: Operation is cancelled while waiting

- **WHEN** cancellation is requested before inference begins
- **THEN** the cancellation flag is committed to SQLite before the in-memory request context is cancelled, and readiness cannot override cancellation

### Requirement: Translation restart resumes only a complete pre-inference checkpoint

On backend startup, recovery SHALL reconstruct the same operation ID from a supported, complete pre-inference WAITING_FOR_RESOURCES or WAITING_FOR_MODEL checkpoint. It SHALL re-resolve the source through the canonical Translation resolver, prove the same source identity/reference and source revision, resolve the current #261 provider configuration, prove the captured non-secret provider revision and exact selected model still match, and acquire fresh #265 admission before warming or inference. If admission is still unavailable or waiting, the operation stays visibly waiting and SHALL NOT poll automatically or contact Ollama. After admission is granted, recovery proceeds through WAITING_FOR_MODEL. It SHALL compare-and-set the operation row version, phase, and cancellation state before leaving the waiting phase and SHALL recheck cancellation, source revision, and model/configuration immediately before recording INFERENCE_STARTED.

Recovery SHALL create a new operation-owned workspace after validation and SHALL NOT trust a persisted temporary path. Missing fields, unsupported checkpoint versions, changed identity/revision/model/configuration, cancellation, or failure to acquire admission SHALL fail closed without resumed inference. A path change is acceptable only when the canonical resolver proves identical source identity and revision. If the process restarts in INFERENCE_STARTED or any later nonterminal phase without a durable result, recovery SHALL mark RECOVERY_REQUIRED and SHALL NOT replay inference. Explicit retry after failure SHALL use a fresh operation ID and re-resolve current source/model according to the existing retry contract.

#### Scenario: Complete wait resumes after restart

- **WHEN** backend startup finds a complete uncancelled pre-inference WAITING_FOR_RESOURCES or WAITING_FOR_MODEL record and source, revision, provider configuration, and exact model checks pass
- **THEN** it reconstructs the original operation ID, reacquires admission, remains waiting if admission is unavailable, and resumes readiness only after admission is granted

#### Scenario: Source, model, or provider configuration changed

- **WHEN** restart cannot prove the captured source identity/revision or exact provider/model configuration
- **THEN** the original operation is marked stale or RECOVERY_REQUIRED and no provider call is made

#### Scenario: Checkpoint is incomplete or unsupported

- **WHEN** any required checkpoint identity or version field is missing or unknown
- **THEN** recovery fails closed without guessing identity, inferring, publishing, or discarding evidence

#### Scenario: Backend restarts during uncertain inference

- **WHEN** the durable phase is INFERENCE_STARTED or later but there is no complete durable result
- **THEN** recovery marks RECOVERY_REQUIRED and makes no automatic duplicate inference call

### Requirement: Translation publication has a durable intent and fail-closed reconciliation

After inference, Translation SHALL validate and stage all requested outputs in its operation-owned workspace. Before promoting any final file, it SHALL persist a SQLite publication intent containing the operation ID, source revision, exact output targets, staged-file fingerprints, and expected final-file state. It SHALL use the existing guarded publication/rollback path. Only after every output is promoted and the source has been rechecked SHALL artifact metadata and the operation's PUBLICATION_COMMITTED/COMPLETED marker be committed in one SQLite transaction. The system SHALL NOT describe the filesystem and SQLite sequence as one atomic transaction.

After restart, a publication-committed operation is complete only if its SQLite artifact metadata and every expected final-file fingerprint match. A publication intent without a committed marker, a metadata/filesystem mismatch, or an unverified in-process rollback SHALL result in RECOVERY_REQUIRED. Startup MAY compare journaled fingerprints to staged/final/backup files for bounded diagnostics, but SHALL preserve them and SHALL NOT automatically publish, republish, roll back, delete, clean, or infer. A retry requires an explicit new operation identity.

#### Scenario: Crash between file promotion and SQLite commit

- **WHEN** startup finds publication intent but no committed marker
- **THEN** it inspects recorded fingerprints for diagnosis, preserves all files, and marks RECOVERY_REQUIRED without republishing

#### Scenario: SQLite and final files confirm commit

- **WHEN** publication metadata and committed marker exist and every final file matches its recorded fingerprint
- **THEN** startup reports the original operation completed and performs no inference or publication

#### Scenario: Committed metadata does not match files

- **WHEN** a committed output is missing or its fingerprint differs from SQLite metadata
- **THEN** startup marks RECOVERY_REQUIRED and does not overwrite or republish the file

#### Scenario: Publication rollback cannot be verified

- **WHEN** a publication failure occurs and restoration of every previous final cannot be proven
- **THEN** the operation preserves filesystem evidence and remains RECOVERY_REQUIRED

