## Purpose

This capability gives operators a safe, non-contacting assessment of whether an exact Ollama qualification target is ready, while preventing a plan or simulated decision from authorizing access to shared production resources.

## ADDED Requirements

### Requirement: Real-provider preflight is explicit and non-contacting

The real-provider preflight SHALL require an exact endpoint, exact model, supported synthetic workflow, full source SHA, and run identity. It SHALL NOT contact Ollama, inspect production Settings or SQLite, start a real workflow, warm a model, or publish content. It SHALL report `executionAllowed: false` and `realInferenceExecuted: false` for every result.

#### Scenario: A complete target is supplied without live authorities
- **WHEN** an operator supplies all exact target fields to the preflight
- **THEN** the tool reports `BLOCKED` while authentic telemetry, live Resource Admission, provider residency, critical-service guard, restricted egress, and moment-of-run authorization are unavailable
- **AND** the declared source SHA remains unverified against a sandbox image
- **AND** no network or production resource is contacted

#### Scenario: An exact target field is missing or ambiguous
- **WHEN** endpoint, model, workflow, full source SHA, or run identity is omitted or malformed
- **THEN** the preflight rejects the request without resolving defaults or contacting a provider

### Requirement: Plans, previews, and caller-provided evidence never authorize execution

The preflight SHALL NOT accept caller-provided booleans, recorded telemetry, fixture observations, simulated admission, `Preview` decisions, or mock-provider responses as live authority. No executable real warmup or inference mode SHALL be exposed until every trusted source and network boundary is implemented and verified.

#### Scenario: Simulated or caller-supplied evidence claims readiness
- **WHEN** a caller attempts to use a plan, preview, mock decision, or supplied evidence as authorization
- **THEN** the request remains blocked and no Ollama endpoint is contacted

#### Scenario: Real execution is requested before the implementation gate is lifted
- **WHEN** a caller selects an unsupported real execution mode or authorization option
- **THEN** the CLI rejects the option before creating containers, opening sockets, or writing outside the run-owned temporary sandbox

### Requirement: Missing or unsafe resource evidence fails closed

The qualification preflight SHALL require fresh, source-attributed observations bound to the actual shared capacity domain, including host memory and swap pressure, CPU, Ollama resident models, critical-service activity, and live #265 admission for both `MODEL_LOAD` and `INFERENCE`. It SHALL require positive purpose-scoped leases from the canonical admission service, exact provider/model identity, one-inference concurrency, bounded cancellation/timeout, and a verified egress restriction to the exact endpoint before any future execution may be enabled. Missing, stale, contradictory, untrusted, or mismatched evidence SHALL produce NO-GO and SHALL NOT be replaced by static memory values, preview results, synthetic signals, or caller assertions.

#### Scenario: Host telemetry or its provenance is unavailable
- **WHEN** any required host, swap, CPU, capacity-domain, provider-residency, or critical-service observation is absent, stale, contradictory, or not source-attributed
- **THEN** the result is `BLOCKED` and no model load or inference occurs

#### Scenario: Admission does not grant the exact purpose-scoped lease
- **WHEN** either model-load or inference admission is `WAITING`, `DENIED`, `UNAVAILABLE`, mismatched, or represented only by a preview
- **THEN** the result is `BLOCKED`, with no warmup, inference, or lease substitution

#### Scenario: Endpoint restriction cannot be proven
- **WHEN** the runtime cannot demonstrate that only the exact declared Ollama endpoint is reachable and production networks remain inaccessible
- **THEN** the result is `BLOCKED` and the driver does not create an external route

### Requirement: Production admission accepts only authenticated, source-bound observations
The production `airesource` integration SHALL read a bounded signed observation snapshot from a configured local path and verify its Ed25519 signature before using any facts. It SHALL bind the snapshot to the configured capacity domain and exact collector, host, provider-residency, and critical-service source identities. It SHALL require explicit RAM, swap, and CPU values with independent timestamps, plus provider and critical-service timestamps. The existing #265 evaluator SHALL enforce freshness and policy for both `MODEL_LOAD` and `INFERENCE`; no second admission policy is permitted. A source read, signature, identity, or validation failure SHALL clear cached trusted evidence and return no lease. In trusted-source mode, unsigned in-memory publication SHALL be rejected. The backend SHALL hold only the public verification key; the signing key remains outside the application.

#### Scenario: Signed, fresh observation reaches canonical admission
- **WHEN** the configured snapshot has a valid signature, exact configured source identities and capacity domain, valid RAM/swap/CPU values, and all timestamps satisfy policy freshness
- **THEN** `airesource` evaluates it through the existing #265 policy and lease lifecycle
- **AND** the snapshot reader performs no provider request or state change

#### Scenario: Evidence is stale, forged, unavailable, or bound to another source/domain
- **WHEN** any required file, signature, identity, capacity domain, value, or timestamp is invalid or unavailable for a new admission attempt
- **THEN** the trusted service clears its cached sample and returns `UNAVAILABLE` without creating a lease
- **AND** an older cache entry or unsigned `PublishObservation` cannot replace the failed source
- **AND** an already persisted active request keeps #265's existing idempotent lease reattachment semantics

#### Scenario: Pressure thresholds are missing or exceeded
- **WHEN** a signed source provides valid and fresh CPU/swap measurements but either trusted pressure threshold is not configured
- **THEN** canonical admission returns `UNAVAILABLE` without granting a lease
- **WHEN** both thresholds are configured and a signed CPU or swap measurement exceeds its threshold
- **THEN** canonical admission returns `WAITING` without granting a lease
- **AND** an unsigned or mock observation cannot satisfy these conditions

#### Scenario: Signed CPU or swap evidence is stale
- **WHEN** the RAM, provider, and guard timestamps are current but signed CPU or swap evidence is stale
- **THEN** `ObservationFresh` returns false and the corresponding pressure field in `ObservationStatus` reports `stale`
- **AND** Resource Admission returns `UNAVAILABLE` without granting a new lease

#### Scenario: A signed source is present but execution gates are missing
- **WHEN** telemetry is signed and fresh but provider endpoint identity, exact-endpoint egress enforcement, sandbox image/SHA binding, or moment-of-run authorization is unverified
- **THEN** `real-preflight` remains `BLOCKED` and no warmup or inference is exposed

### Requirement: Synthetic subtitle and review boundaries remain isolated

Any future qualification enabled by this capability SHALL use only bounded synthetic SRT/ASS or generated bitmap fixtures, exact source identity, temporary SQLite and sandbox-owned sidecars. Translation SHALL preserve cue identity, timing, order, and protected ASS structure. OCR suggestions SHALL remain subject to the existing human review flow, and publication SHALL remain inside the temporary sandbox. The qualification tool SHALL NOT change production Settings, models, resident-model state, provider configuration, or files.

#### Scenario: Preflight is run for OCR or Translation
- **WHEN** an OCR or Translation workflow is planned
- **THEN** the preflight records only the declared synthetic workflow metadata and creates no subtitle, Result, review decision, or publication side effect
