## Purpose

Coordinate on-demand Ollama model residency with Queue and explicit AI operations so work can wait, warm, run, and recover without tying model status to installed-model settings or consuming an FFmpeg worker while the model is unavailable.

## ADDED Requirements

### Requirement: Model lifecycle states distinguish selection, residency, and active use

The system SHALL report one lifecycle state for a selected provider/model identity using `UNLOADED`, `WAITING_FOR_RESOURCES`, `WARMING_UP`, `READY`, `IN_USE`, `IDLE`, or `FAILED`. Installed-model discovery, provider reachability, and model metadata readiness SHALL remain distinct from lifecycle state. `GET /api/settings/ollama/runtime` SHALL derive loaded-model membership and Ollama expiry from `/api/ps`, installed-model identity from the shared provider catalog, and MVForge busy state from active MVForge operation tracking. It SHALL NOT infer that a model is in use merely because Ollama reports it as loaded.

#### Scenario: Installed model is not loaded
- **WHEN** the shared provider catalog contains the selected model but `/api/ps` does not
- **THEN** runtime status reports it as installed and `UNLOADED`, without claiming that it is ready or in use

#### Scenario: Loaded model has no active MVForge consumer
- **WHEN** `/api/ps` reports the selected model as resident and MVForge has no active operation lease for it
- **THEN** runtime status reports the observed residency and expiry with `IDLE` or `READY`, not `IN_USE`

#### Scenario: Model is used by an MVForge operation
- **WHEN** an MVForge consumer holds an active lease for the selected model
- **THEN** runtime status reports `IN_USE` for that consumer even if Ollama residency alone would not reveal its use

#### Scenario: Ollama residency cannot be read
- **WHEN** the shared provider cannot complete a bounded `/api/ps` request or returns malformed data
- **THEN** runtime status reports an unavailable or failed state with a bounded diagnostic and does not return a false empty loaded-model list

### Requirement: Models warm only on demand and remain loaded for the configured idle interval

The system SHALL request warmup only when a runnable consumer reaches its AI stage or an explicit user operation requests model work. It SHALL NOT warm a model on profile/model-setting save or solely because a Queue item exists but is not otherwise runnable. Warmup SHALL use the shared provider to POST `/api/generate` with the exact selected model, no prompt, non-streaming response, and a `keep_alive` duration. The default duration SHALL be 30 minutes and SHALL also be sent on every inference request for that model. A warmup SHALL be considered ready only after the bounded provider operation succeeds and `/api/ps` confirms the selected model is resident. The system SHALL NOT download models, restart Ollama, or force-unload a model as part of ordinary idle expiry.

#### Scenario: Explicit operation requests an unloaded model
- **WHEN** a runnable AI operation requests an installed model that is not resident
- **THEN** the system sends one on-demand warmup request through the shared provider, verifies residency, and proceeds only when ready

#### Scenario: Runnable operation requests an already resident model
- **WHEN** the selected model is resident and an operation reaches its AI stage
- **THEN** the operation uses the exact selected model and renews `keep_alive` on its inference request without pulling or switching models

#### Scenario: Queue item is not runnable
- **WHEN** a Queue item is blocked by an existing review, schedule, workspace, or worker condition before its AI stage
- **THEN** the system does not warm its model until the other conditions clear and the AI stage becomes runnable

#### Scenario: Model is absent from the provider catalog
- **WHEN** warmup is requested for a model that is not installed or Ollama reports it missing
- **THEN** the operation receives a bounded `model_not_found` failure and no automatic pull, fallback selection, or provider restart occurs

#### Scenario: Idle model expires
- **WHEN** the configured 30-minute keep-alive interval elapses without another MVForge inference request
- **THEN** Ollama may expire the model and the next residency observation reports it as unloaded; MVForge does not issue `keep_alive: 0` or otherwise evict it

### Requirement: Same-model warmup is singleflight, cancellable, and bounded

Concurrent warmup requests for the same shared provider identity and exact model ID SHALL join one in-flight warmup. Each consumer SHALL be able to cancel its own wait without cancelling other consumers; the shared warmup MAY be cancelled when no consumers remain. The default readiness window SHALL be bounded to five minutes after resource admission, including warmup and readiness confirmation. Transient failures MAY be retried only with bounded backoff and within the configured attempt/deadline limit. Timeout, provider outage, missing model, malformed response, and cancellation SHALL remain distinguishable with sanitized diagnostics. When the readiness window expires, automatic attempts SHALL stop and the consumer SHALL receive an actionable retryable failure rather than an indefinite spinner.

#### Scenario: Concurrent consumers request the same model
- **WHEN** multiple operations request warmup for the same provider and model while one warmup is in flight
- **THEN** they share that warmup result and the provider receives no duplicate concurrent warmup for that key

#### Scenario: One consumer cancels a shared wait
- **WHEN** one consumer cancels while another consumer still awaits the same warmup
- **THEN** the cancelled consumer remains cancelled and the shared warmup continues for the remaining consumer

#### Scenario: All consumers cancel
- **WHEN** every consumer awaiting a shared warmup has cancelled
- **THEN** the manager cancels its request context, prevents those consumers from resuming, and reconciles `/api/ps` before any later attempt

#### Scenario: Readiness deadline expires
- **WHEN** the provider remains unavailable or warmup does not complete within the five-minute default window
- **THEN** the operation reaches a bounded retryable failure state with a sanitized cause and automatic retries stop until a permitted retry occurs

### Requirement: Queue waits for model readiness without occupying an FFmpeg worker

When a Queue AI stage is otherwise runnable but its model is waiting for resource admission or warmup, the scheduler SHALL durably expose `WAITING_FOR_MODEL` on the Queue plan. The Queue job SHALL not occupy an active FFmpeg worker reservation while waiting, and a model-waiting item SHALL NOT prevent unrelated runnable Queue work from being claimed. `WAITING_FOR_RESOURCES` SHALL represent an admission result owned by #265; this capability SHALL NOT calculate or override resource limits. On readiness, the scheduler SHALL resume through its normal claim path only after confirming that the job is not cancelled, the frozen source revision is still valid, and the exact model selection captured for that AI stage is unchanged. Cancellation or stale identity SHALL take precedence over readiness.

#### Scenario: Queue model is not resident
- **WHEN** an otherwise runnable Queue AI stage requests a model that needs warmup
- **THEN** the plan durably reports `WAITING_FOR_MODEL`, no active FFmpeg reservation is held, and the AI stage has not run

#### Scenario: Resource admission is denied
- **WHEN** the #265 resource-protection authority declines model admission
- **THEN** the Queue plan reports `WAITING_FOR_RESOURCES` without duplicating capacity policy or holding an FFmpeg worker

#### Scenario: Another Queue item is runnable
- **WHEN** one Queue plan is waiting for a model and another Queue plan is ready
- **THEN** the ready plan remains claimable and is not blocked by the model waiter

#### Scenario: Queue wait recovers after backend restart
- **WHEN** a persisted Queue model wait is recovered after restart and the model becomes ready
- **THEN** the scheduler rechecks the job cancellation state, frozen source revision, and captured model identity before returning it to the normal claim path

#### Scenario: Queue job was cancelled while waiting
- **WHEN** readiness arrives after the associated Queue job has been cancelled
- **THEN** the scheduler does not resume the job or issue another AI request, even if the shared warmup completed for another consumer

#### Scenario: Source or model identity changed while waiting
- **WHEN** the captured source revision is stale or the waiting checkpoint no longer matches its frozen model selection
- **THEN** the scheduler fails closed, performs no resumed AI work, and requires explicit re-preparation or retry

### Requirement: Waiting AI operations resume through their owning workflow lifecycle

Each consumer SHALL persist its own bounded wait checkpoint using its canonical workflow identity and SHALL NOT create a second general-purpose operation lifecycle. A checkpoint SHALL contain enough information to revalidate the exact source revision, selected model identity, operation identity, and cancellation state without storing prompts, provider responses, or source cue payloads. Startup recovery SHALL resume only at a recorded pre-inference wait boundary; an uncertain in-flight inference SHALL become a retryable terminal result and SHALL NOT be replayed automatically. OCR and Copilot consumers added by their feature owners SHALL use this same lifecycle contract.

#### Scenario: Translation waits and resumes in the same operation
- **WHEN** an explicit Translation operation waits for its selected model and later passes source, model, and cancellation checks
- **THEN** the existing Translation operation resumes under its original operation identity and continues through its existing validation and atomic publication lifecycle

#### Scenario: Operation is cancelled while waiting
- **WHEN** a user cancels a waiting AI operation before it resumes
- **THEN** the persisted cancellation wins over model readiness and the operation creates no new provider result or publication

#### Scenario: Backend restarts before inference starts
- **WHEN** startup finds a durable model-wait checkpoint at a pre-inference boundary
- **THEN** it resumes the same operation identity only after revalidating its source, model, and cancellation state

#### Scenario: Backend restarts during uncertain provider inference
- **WHEN** startup cannot determine whether an inference request completed
- **THEN** it marks that attempt retryable and does not automatically replay the provider call or publish duplicate output

### Requirement: Model lifecycle integrates with canonical provider, resource, role, and Settings owners

The lifecycle manager SHALL resolve its endpoint and bounded Ollama transport through the shared provider owned by #261. It SHALL accept an exact consumer-selected model identity and SHALL NOT own a second endpoint, installed-model catalog, feature selection, Primary/Fast/Fallback role, or fallback decision. Resource admission and `WAITING_FOR_RESOURCES` SHALL be delegated to #265. The #266 Settings surface SHALL consume the backend runtime projection and lifecycle policy contract without contacting Ollama directly; #223 SHALL remain the authority for model roles and fallback selection.

#### Scenario: Consumer selects a model role
- **WHEN** a consumer resolves a model through #223 or a feature selection from #266
- **THEN** the lifecycle manager warms and reports that exact model without changing roles or feature selections

#### Scenario: Resource policy rejects admission
- **WHEN** #265 denies a warmup admission
- **THEN** the lifecycle manager records the resource wait and does not apply its own RAM, VRAM, or resident-model threshold

#### Scenario: Provider endpoint is changed
- **WHEN** the shared #261 endpoint changes while a model wait exists
- **THEN** the waiting request is invalidated or re-resolved by its owning workflow and no request is sent through a duplicate or stale endpoint authority

#### Scenario: Settings displays runtime state
- **WHEN** #266 requests model runtime status
- **THEN** MVForge returns backend-derived installed, loaded, active-use, waiting, warming, idle, unavailable, and expiry data without exposing prompt or provider response content
