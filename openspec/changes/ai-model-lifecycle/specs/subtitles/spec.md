## MODIFIED Requirements

### Requirement: Translation provider readiness and explicit enablement

Translation SHALL be disabled by default. Configuration, readiness, and
enablement SHALL remain distinct backend-authoritative states. A configured
provider SHALL not become usable merely because its endpoint and model are
non-empty. The backend SHALL provide an explicit `Test Connection` operation
for the configured provider. For Ollama, readiness SHALL verify both
reachability of the configured endpoint and availability of the configured
model through a provider-native lightweight readiness/model request, without
generating a subtitle translation or requiring GPU capability. The frontend
SHALL not probe the provider or infer readiness.

The backend SHALL support bounded states equivalent to `disabled`,
`not_configured`, `untested`, `ready`, `unavailable`, `model_not_found`, and
`invalid_configuration`, with secret-safe diagnostics. A readiness timestamp,
if exposed, SHALL be informational and SHALL NOT be treated as a live health
guarantee. Explicit enablement SHALL require successful readiness for the
current configuration. Changing provider, endpoint, or model SHALL invalidate
readiness and enablement.

#### Scenario: Translation disabled by default

- **WHEN** translation settings are absent or newly initialized
- **THEN** translation is disabled and Asset Info cannot start provider work

#### Scenario: Configured but untested provider

- **WHEN** an Ollama endpoint and model are configured but have not passed
  readiness testing
- **THEN** translation remains disabled and backend capabilities report that
  provider verification is required

#### Scenario: Successful Ollama readiness

- **WHEN** `Test Connection` reaches the configured Ollama endpoint and
  confirms that the configured model is available
- **THEN** readiness becomes `ready`, no subtitle translation is generated,
  and translation remains disabled until explicitly enabled

#### Scenario: Missing model

- **WHEN** the configured Ollama endpoint is reachable but the configured
  model is unavailable
- **THEN** readiness reports a bounded actionable `model_not_found` state and
  translation cannot be enabled

#### Scenario: Enable after readiness

- **WHEN** the current provider configuration passes readiness testing and
  the user explicitly enables translation
- **THEN** backend capabilities may expose translation as available for an
  otherwise eligible subtitle

#### Scenario: Configuration change invalidates readiness

- **WHEN** provider, endpoint, or model changes after successful verification
- **THEN** the prior readiness and enablement are invalidated and translation
  cannot run until the new configuration is tested successfully and enabled

#### Scenario: Runtime outage does not trigger a preflight loop

- **WHEN** an enabled provider becomes unavailable or its selected model is
  unloaded after setup
- **THEN** Translation uses the lifecycle manager to inspect residency and,
  when needed, perform bounded on-demand warmup before inference; it does not
  repeat the `/api/show` metadata readiness check, silently change models, or
  enter an unbounded health-check loop

## ADDED Requirements

### Requirement: Translation resumes only for its captured source and model

An explicit Translation operation that waits for model warmup SHALL remain in
the existing subtitle operation lifecycle and SHALL preserve its operation
identity, bounded progress, and cancellation behavior. Before provider
inference resumes, the backend SHALL verify that the operation is still
uncancelled, its captured source revision still matches, and its exact
selected model identity still matches the operation checkpoint. A stale or
cancelled operation SHALL make no resumed provider call and SHALL publish no
new artifact. A retry after terminal failure SHALL follow the existing
retry contract and use a fresh operation identity with source and model
re-resolved.

#### Scenario: Translation model becomes ready while operation waits

- **WHEN** the selected model becomes ready before the operation's bounded
  wait expires and its source, model, and cancellation checks pass
- **THEN** the same Translation operation resumes and keeps its existing
  validation and atomic publication barriers

#### Scenario: Source changes while Translation waits

- **WHEN** the source revision changes before model readiness
- **THEN** the operation fails as stale before resuming inference and leaves
  the source and previously published artifacts unchanged

#### Scenario: Selected model changes while Translation waits

- **WHEN** the persisted operation checkpoint no longer matches its captured
  model identity
- **THEN** the operation fails closed rather than using the newly selected
  model or an implicit role fallback

#### Scenario: Cancellation races with model readiness

- **WHEN** cancellation and readiness occur concurrently
- **THEN** cancellation wins, the operation does not resume provider work, and
  no new translated artifact is published

#### Scenario: Waiting Translation is recovered after restart

- **WHEN** backend startup recovers a Translation operation that was durably
  waiting before inference
- **THEN** it preserves the operation identity and resumes only after
  rechecking source revision, model identity, and cancellation state
