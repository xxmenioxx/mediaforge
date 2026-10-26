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

### Requirement: Synthetic subtitle and review boundaries remain isolated

Any future qualification enabled by this capability SHALL use only bounded synthetic SRT/ASS or generated bitmap fixtures, exact source identity, temporary SQLite and sandbox-owned sidecars. Translation SHALL preserve cue identity, timing, order, and protected ASS structure. OCR suggestions SHALL remain subject to the existing human review flow, and publication SHALL remain inside the temporary sandbox. The qualification tool SHALL NOT change production Settings, models, resident-model state, provider configuration, or files.

#### Scenario: Preflight is run for OCR or Translation
- **WHEN** an OCR or Translation workflow is planned
- **THEN** the preflight records only the declared synthetic workflow metadata and creates no subtitle, Result, review decision, or publication side effect
