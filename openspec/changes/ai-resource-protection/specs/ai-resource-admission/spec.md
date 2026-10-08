## Purpose

This capability admits MVForge model loads and inference only when shared host capacity and the configured resource policy allow them. It gives lifecycle and future AI consumers one provider-neutral authority with bounded waits, recoverable leases, and explicit failure states.

## ADDED Requirements

### Requirement: AI consumers use one provider-neutral admission contract

The backend SHALL expose a resource-admission contract owned independently of provider and lifecycle implementations. A request SHALL identify an opaque capacity domain, provider configuration identity, exact model ID, stable consumer ID, purpose (`MODEL_LOAD` or `INFERENCE`), and any workload hints required by the resource estimator. The contract SHALL NOT import or expose #264 lifecycle types, choose a model, resolve a provider endpoint, or issue provider requests.

Each decision SHALL be exactly one of `ADMITTED`, `WAITING`, `DENIED`, or `UNAVAILABLE`. `ADMITTED` SHALL include an opaque lease. `WAITING` SHALL include an opaque wait handle and SHALL NOT include a lease. `DENIED` SHALL identify a terminal policy reason for that attempt. `UNAVAILABLE` SHALL identify a missing, stale, or failed admission authority or required capacity signal. No consumer SHALL interpret an error or unavailable result as permission to proceed.

#### Scenario: Lifecycle asks permission to warm a model
- **WHEN** a consumer submits a model-load request with an exact model identity and stable request identity
- **THEN** it receives one of the four admission outcomes, and only `ADMITTED` contains a lease that can authorize the warmup

#### Scenario: Lifecycle asks permission to infer with a resident model
- **WHEN** a consumer submits an inference request for an already resident model
- **THEN** the request still requires a separate positive inference admission decision

#### Scenario: Admission authority is absent or errors
- **WHEN** a consumer cannot obtain a decision from the resource-admission service
- **THEN** it receives no implicit grant and issues no model-affecting provider request

### Requirement: Residency, model loading, and inference have independent limits

The default policy SHALL allow at most one resident model in a capacity domain, at most one concurrent MVForge inference in that domain, and at most one concurrent model load. The resident and inference limits SHALL be independently configurable, including a resident limit of two or more. A host memory upgrade SHALL NOT silently raise any configured limit. Admission SHALL count all provider-reported resident models in the domain, including models loaded by clients outside MVForge; the MVForge inference limit SHALL count MVForge leases only.

The model-load permit SHALL be separate from inference permits. A model load SHALL reserve resident capacity before warmup, but SHALL NOT consume the inference counter. A resident model SHALL not bypass inference admission. A configured protection disable switch SHALL deny new admission with a visible reason; it SHALL NOT install a permissive bypass. Existing leases SHALL remain releasable.

#### Scenario: Conservative 24 GiB defaults
- **WHEN** a new resource policy is initialized for the current NAS class
- **THEN** it defaults to one resident model, one concurrent MVForge inference, and one concurrent model load

#### Scenario: Two models are configured after a capacity upgrade
- **WHEN** an operator configures two resident models and one concurrent inference on a larger host, and current capacity evidence satisfies both estimates and the reserve
- **THEN** a second model load may be admitted without increasing the inference limit

#### Scenario: Inference capacity is full while residency has room
- **WHEN** one inference is active under the default policy and another consumer requests inference
- **THEN** the second request waits even if its model is resident and the resident limit has not been reached

#### Scenario: An unrelated client has loaded a model
- **WHEN** provider telemetry reports a resident model loaded outside MVForge and that model consumes the configured resident allowance
- **THEN** a new MVForge model load waits or is denied by policy and no model is unloaded

### Requirement: Admission uses fresh capacity evidence and protects critical services

Before granting a model load or inference, the service SHALL evaluate a fresh host-capacity observation, a fresh provider resident-model census, a usable model-specific resource estimate, and the critical-service guard for configured protected services. Unless the canonical source has a stricter freshness bound, host and resident observations older than 60 seconds SHALL be stale. The host available-memory reserve SHALL default to a configurable 6 GiB initial value; this value is provisional until validated against measured NAS workloads. The estimate SHALL distinguish incremental resident/load memory from per-inference working memory and SHALL account for the requested context/KV-cache profile when that input is available. Missing estimates SHALL NOT be treated as zero, and installed model size alone SHALL NOT be represented as a guaranteed resident or inference footprint.

The service SHALL use critical-service priority and pressure decisions from the canonical authority owned by #256, rather than define a competing service-priority policy. Pressure that currently prevents safe AI work SHALL produce `WAITING`. Missing or stale evidence required by the active policy SHALL produce `UNAVAILABLE`. A demand that cannot fit under the configured ceiling even when transient pressure clears SHALL produce `DENIED`. CPU, iGPU, swap/zram, and per-service metrics SHALL be treated as unknown when their source reports them unavailable, never as zero; configured required metrics that are missing or stale SHALL prevent admission.

#### Scenario: 24 GiB host has insufficient post-load headroom
- **WHEN** a 24 GiB test snapshot reports 14 GiB available, the configured reserve is 6 GiB, and the selected model's warmup and resident estimate is 9 GiB
- **THEN** the model load waits, creates no lease, and issues no warmup request because projected available memory would fall below the reserve

#### Scenario: Host has enough headroom for a configured second resident
- **WHEN** a 64 GiB test profile has two configured resident slots, current telemetry is fresh, the second model's load estimate fits while preserving the reserve, and the critical-service guard allows work
- **THEN** the second model load is admitted while inference remains limited to one

#### Scenario: Jellyfin or another protected service is under pressure
- **WHEN** the canonical critical-service guard reports pressure for Jellyfin or another configured protected service
- **THEN** new MVForge loads and inferences wait without changing that service or the Ollama container

#### Scenario: Required telemetry is missing or stale
- **WHEN** host memory, resident-model census, required model estimate, or a configured required critical-service signal is unavailable or stale
- **THEN** admission returns `UNAVAILABLE` with a bounded reason and grants no lease

### Requirement: Waiting is fair, bounded, cancellable, and does not poll aggressively

The service SHALL return `WAITING` without blocking a Queue, FFmpeg, or provider worker. It SHALL keep bounded wait queues for model-load and inference purposes, use FIFO order within each purpose and fair turn-taking between eligible purposes, and reevaluate on lease release or a new canonical telemetry observation. The default maximum admission wait SHALL be five minutes and SHALL be configurable. Expired waits SHALL terminate with a bounded `DENIED` reason. A wait handle SHALL support event-driven wait/reattachment and cancellation; cancellation SHALL remove the request before any later grant. If cancellation races with a grant, the caller SHALL receive the granted lease or a confirmed cancellation and SHALL release any lease already granted.

`UNAVAILABLE` SHALL not enter an automatic retry loop. A consumer may submit a fresh request after the missing authority or telemetry is restored or after an explicit retry. The service SHALL NOT create an AI-specific high-frequency telemetry poller.

#### Scenario: A model load cannot fit at current pressure
- **WHEN** valid capacity evidence shows a temporary shortfall but the model can fit after resources are released
- **THEN** the service returns `WAITING` immediately with a wait handle and leaves the scarce worker available

#### Scenario: A waiting consumer cancels
- **WHEN** cancellation is accepted before an admission grant
- **THEN** the ticket is removed and the consumer is never granted later from that ticket

#### Scenario: A wait deadline expires
- **WHEN** the configured admission wait deadline elapses without safe capacity
- **THEN** the service ends the attempt with `DENIED` and an `admission_wait_expired` reason

#### Scenario: Telemetry is unavailable
- **WHEN** a capacity source becomes unavailable
- **THEN** the service returns `UNAVAILABLE` and does not poll or retry the same request automatically

### Requirement: Opaque leases have explicit release and restart semantics

Every admitted request SHALL receive a purpose-scoped opaque lease that consumers cannot forge or reinterpret. `Release` SHALL be explicit and idempotent. A model-load lease SHALL be released after the lifecycle owner completes or reconciles the warmup against the provider resident census; it SHALL not authorize inference. An inference lease SHALL cover exactly one MVForge provider request and SHALL be released on completion or confirmed cancellation. Releasing a lease SHALL never unload a model or restart a provider.

The service SHALL persist the minimal lease and wait identity needed for idempotent reacquisition and crash recovery, keyed by stable consumer/request identity and purpose. On restart it SHALL withhold new grants until required host and provider telemetry is fresh, restore unexpired reservations, and count provider-reported residents including external models. An uncertain inference reservation SHALL remain held through the bounded provider-request deadline plus cleanup grace; restart SHALL NOT immediately treat it as idle. A lifecycle recovery SHALL be able to reacquire the same logical request without allocating a duplicate lease.

#### Scenario: A lease is released twice
- **WHEN** a consumer repeats `Release` for the same lease
- **THEN** the service completes idempotently without freeing capacity belonging to another request

#### Scenario: The backend restarts during inference
- **WHEN** an inference lease has not been explicitly released at restart
- **THEN** the reservation remains counted until its recorded bounded expiry and cleanup grace, and no duplicate inference grant is issued

#### Scenario: Lifecycle recovery reacquires a model-load lease
- **WHEN** the lifecycle owner retries the same stable consumer/request identity after restart
- **THEN** the service reattaches to the existing logical lease or wait identity instead of creating a duplicate reservation

#### Scenario: A model-load lease is released while the model remains resident
- **WHEN** warmup completes and its lease is released but the provider census still reports the model resident
- **THEN** that model continues to count against the resident limit until provider telemetry confirms it is absent

### Requirement: Settings and diagnostics distinguish MVForge policy from Ollama configuration

Resource Protection settings SHALL expose the enabled state, resident and inference limits, load concurrency, available-memory reserve, model-specific estimate state, and bounded wait policy. A capacity preview SHALL use the same evaluator as admission, create no lease, and label stale or estimated values. Operational counters SHALL distinguish admitted, waiting, denied, and unavailable decisions with bounded reason codes.

The UI and API SHALL present MVForge policy separately from the Ollama server's `OLLAMA_MAX_LOADED_MODELS` and `OLLAMA_NUM_PARALLEL` values. Actual server values SHALL be shown as unknown unless a read-only source explicitly reports them; `/api/ps`, MVForge policy values, and model residency SHALL NOT be used to infer those environment settings. A restart requirement SHALL be shown only when explicit current and requested server configuration evidence can be compared. Saving MVForge resource policy SHALL NOT change Ollama configuration or restart a container.

#### Scenario: MVForge and Ollama limits differ
- **WHEN** the MVForge policy allows one inference but the observed Ollama server configuration reports a different parallel limit
- **THEN** diagnostics show both values and their sources separately without claiming that MVForge changed Ollama

#### Scenario: Ollama server configuration cannot be observed
- **WHEN** no approved read-only source exposes the Ollama environment values
- **THEN** those values display as unknown and the system does not infer them from residency or MVForge policy

#### Scenario: Only MVForge policy is saved
- **WHEN** an operator changes a resident or inference policy limit
- **THEN** the MVForge policy is updated without changing the Ollama container, environment, or restart state

#### Scenario: Capacity preview runs under pressure
- **WHEN** a user requests a dry-run admission preview
- **THEN** the preview returns the current bounded reason and estimate without acquiring a lease or starting model work
