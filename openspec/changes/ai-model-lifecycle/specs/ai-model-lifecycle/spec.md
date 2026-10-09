## Purpose

Define on-demand model residency and readiness for MVForge's current explicit Translation consumer while preserving provider, resource, and workflow ownership. Queue AI integration is a future contract only because current Queue stages do not require Ollama.

## ADDED Requirements

### Requirement: Model lifecycle states distinguish selection, residency, and active use

The backend SHALL report one lifecycle state for an exact shared-provider configuration and model identity using UNLOADED, WAITING_FOR_RESOURCES, WARMING_UP, READY, IN_USE, IDLE, or FAILED. Installed-model discovery, provider connectivity, metadata readiness, model residency, and MVForge use SHALL remain separate facts. Loaded-model residency and expiry SHALL come from bounded /api/ps observation owned by the #261 provider adapter. IN_USE SHALL come only from active MVForge operation leases and SHALL NOT be inferred from Ollama residency. A valid empty /api/ps response SHALL be distinct from unavailable or malformed residency data.

#### Scenario: Installed model is not resident

- **WHEN** the #261 catalog contains the exact selected model but /api/ps does not
- **THEN** lifecycle reports installed and UNLOADED without claiming the model is ready or in use

#### Scenario: External model is resident without an MVForge consumer

- **WHEN** /api/ps reports the model and MVForge has no active inference lease
- **THEN** lifecycle reports observed residency and expiry as IDLE, not IN_USE

#### Scenario: An admitted MVForge inference is active

- **WHEN** an explicit consumer holds an active inference lease for the exact model
- **THEN** lifecycle reports IN_USE for that consumer

#### Scenario: Residency observation is unavailable

- **WHEN** bounded /api/ps discovery fails or returns malformed data
- **THEN** the backend reports unavailable residency and a bounded diagnostic rather than a false empty list

### Requirement: Runtime status is bounded and observational

The bounded backend runtime projection at `GET /api/settings/ollama/runtime` SHALL distinguish installed and resident models, include lifecycle states and reported Ollama expiry timestamps, expose active Translation operations and bounded failures/resource waits, and include observation freshness or an explicit unavailable diagnostic. The projection SHALL use a five-second cache and a three-second request deadline. #266 Settings UI and model selection are outside this change.

#### Scenario: Runtime observation is unavailable

- **WHEN** Ollama installed or residency discovery is unavailable or malformed
- **THEN** runtime status reports unavailable with a bounded diagnostic rather than an empty model list

### Requirement: Production activation stays gated on verified resource authority

`MVFORGE_AI_MODEL_LIFECYCLE_ENABLED` SHALL default to false. If explicitly enabled, protected Translation SHALL still require a configured capacity domain and a positive #265 decision before every warmup and inference. Missing/stale telemetry, missing measured model estimates, unavailable #256 critical-service authority, or unavailable #265 admission SHALL never become a grant and SHALL never fall back to legacy unadmitted Ollama requests. Production activation SHALL wait until #256 inputs and target-NAS model calibration are verified. This gate does not implement resource policy or #256 observation collection.

#### Scenario: NAS admission evidence is not yet available

- **WHEN** the lifecycle feature is enabled but #265 reports missing/stale resource or critical-service evidence
- **THEN** Translation remains visibly waiting or denied and issues no model-affecting provider request

### Requirement: Warmup and inference use the shared provider only on demand

The lifecycle SHALL use the #261 shared Ollama endpoint and bounded transport. It SHALL request warmup only for an explicit operation that has passed its feature/source gates and resource admission. Warmup SHALL POST /api/generate with the exact selected model, stream:false, keep_alive, and no prompt. The default keep_alive SHALL be 30 minutes and SHALL also be supplied to Translation /api/chat inference. Warmup readiness requires a successful bounded generate response and a matching exact model entry from /api/ps. Residency already observed in /api/ps skips warmup but does not skip inference admission.

The lifecycle SHALL NOT download or pull models, select a fallback, restart the Ollama provider, send keep_alive:0, or force-unload any resident model. Installed catalog membership and /api/show readiness alone SHALL NOT prove residency.

#### Scenario: Explicit Translation requests an unloaded model

- **WHEN** an enabled explicit Translation operation requests its exact installed model and admission is granted
- **THEN** lifecycle sends one no-prompt generate warmup, confirms residency through /api/ps, and only then permits inference

#### Scenario: Requested model is already resident

- **WHEN** /api/ps reports the exact model and #265 grants inference admission
- **THEN** Translation skips generate, calls inference with the exact model and configured keep_alive, and does not change model selection

#### Scenario: Model is missing

- **WHEN** warmup reports the exact selected model missing from the provider
- **THEN** the operation reaches a bounded model_not_found failure with no automatic download, fallback, or provider restart

#### Scenario: Idle residency expires

- **WHEN** the keep_alive period elapses without another inference request
- **THEN** Ollama may unload the model and a later /api/ps observation reports it as UNLOADED without MVForge eviction

#### Scenario: Current Queue work has no AI stage

- **WHEN** any current Queue stage runs, including subtitle preparation
- **THEN** #264 issues no Ollama warmup or inference request and creates no model wait for that Queue job

### Requirement: Every model load and inference requires #265 admission

#265 owns the merged provider-neutral service in `backend/internal/airesource`. #264 SHALL depend on a narrow acyclic interface over `Admit`, `Await`, `CancelWait`, and `Release`. Requests SHALL carry the configured capacity domain, shared provider configuration identity, exact model ID, consumer, stable workflow ID, unique request ID for each admission attempt, purpose, and operation deadline. Decisions SHALL be ADMITTED with an opaque lease, WAITING with an opaque ticket, DENIED, or UNAVAILABLE. #264 SHALL NOT duplicate thresholds, estimates, telemetry interpretation, fairness, or admission policy; #265 SHALL NOT import #264 lifecycle types or call back into the lifecycle manager.

The lifecycle manager SHALL make no RAM, VRAM, host pressure, resident-model, concurrency, or priority decision. It SHALL require ADMITTED before /api/generate and before every /api/chat inference, including inference against a resident model. It SHALL map WAITING to WAITING_FOR_RESOURCES, await/cancel the opaque ticket, and release opaque leases idempotently. DENIED SHALL fail the attempt without automatic retry. UNAVAILABLE SHALL remain a visible cancellable WAITING_FOR_RESOURCES operation until cancellation or its operation deadline, with no admission polling or model-affecting provider request. A later explicit retry SHALL use a new Translation operation identity and fresh admission request IDs. Missing or stale host/provider telemetry, an unmeasured model estimate, unavailable critical-service guard, missing capacity domain, or unavailable admission service SHALL never become a grant. The five-minute readiness window SHALL begin only after MODEL_LOAD admission; #265 controls resource-wait duration.

#### Scenario: Resource authority grants admission

- **WHEN** #265 returns ADMITTED with an opaque lease
- **THEN** lifecycle may perform only the admitted load or inference operation and releases the lease according to the #265 contract

#### Scenario: Resource authority asks the caller to wait

- **WHEN** #265 returns WAITING with retry guidance
- **THEN** lifecycle reports WAITING_FOR_RESOURCES and issues no model-affecting request before #265 grants admission

#### Scenario: #265 is unavailable or not implemented

- **WHEN** the admission port is absent or returns UNAVAILABLE
- **THEN** lifecycle reports WAITING_FOR_RESOURCES with admission_unavailable, allows cancellation, makes no /api/generate or /api/chat call, and does not install a permissive fallback

#### Scenario: Resident model has no inference admission

- **WHEN** /api/ps reports the selected model resident but #265 has not admitted inference
- **THEN** lifecycle does not call /api/chat

#### Scenario: Admission is denied

- **WHEN** #265 returns DENIED
- **THEN** lifecycle stops that attempt without adding a local policy or silently substituting a model

### Requirement: Same-model warmup is singleflight, cancellable, and bounded

Concurrent requests for the same shared-provider configuration identity and exact model ID SHALL join one warmup attempt. Waiters SHALL have independent cancellation. Cancelling one waiter SHALL not cancel warmup required by other waiters. When all waiters cancel, lifecycle SHALL cancel its request context, reconcile /api/ps before another attempt, and never resume a cancelled consumer. A later retry SHALL wait for cancellation reconciliation to finish. The default readiness window SHALL be five minutes after admission, including warmup and residency confirmation. The flight SHALL make one generate request and use bounded /api/ps polling; it SHALL not automatically repeat a failed warmup. Resource waits owned by #265 are outside this readiness window.

#### Scenario: Concurrent consumers request one model

- **WHEN** multiple admitted consumers request warmup for the same provider configuration and model
- **THEN** they share one in-flight warmup and do not issue duplicate concurrent generate requests

#### Scenario: One of several waiters cancels

- **WHEN** one consumer cancels while another waits on the same warmup
- **THEN** only the cancelled consumer stops and the shared warmup continues for remaining consumers

#### Scenario: Every waiter cancels

- **WHEN** the last waiter cancels
- **THEN** lifecycle cancels the shared request, reconciles /api/ps before retry, and resumes no cancelled consumer

#### Scenario: Readiness deadline expires

- **WHEN** warmup or residency confirmation exceeds the five-minute default after admission
- **THEN** the operation reports a bounded retryable failure and stops automatic attempts

### Requirement: Translation waiting is owned and durably recoverable by its existing operation

Lifecycle SHALL not own Translation operation state. The existing Translation operation SHALL persist its operation identity and a bounded WAITING_FOR_MODEL checkpoint in SQLite before warmup starts. The checkpoint SHALL preserve canonical source identity/reference and source revision, exact provider/model configuration identity, bounded request options, operation phase/version/deadline, and cancellation state. AssetPath SHALL be treated as a locator and not as source identity. The checkpoint SHALL not contain cues, prompts, provider responses, translated text, credentials, or endpoint URLs.

On backend startup, a complete supported checkpoint in either pre-inference WAITING_FOR_RESOURCES or WAITING_FOR_MODEL phase MAY be reconstructed under the same operation ID. Recovery SHALL re-resolve the canonical source and require identity/revision equality, resolve the current #261 configuration and require the captured non-secret configuration revision and exact model identity, and confirm that cancellation is false. Recovery SHALL reacquire #265 admission. If admission remains unavailable or waiting, the operation remains visibly waiting without polling automatically or contacting Ollama; after admission is granted, it proceeds through WAITING_FOR_MODEL and the five-minute readiness deadline begins. Incomplete, unknown-version, cancelled, changed-source, or changed-model checkpoints SHALL fail closed without provider work. Cancellation SHALL be persisted and SHALL win races with readiness.

Before first inference, the backend SHALL transactionally compare operation ID, row version, phase, source/model checkpoint, and cancellation state, then persist INFERENCE_STARTED. If restart observes INFERENCE_STARTED or a later nonterminal phase without a committed outcome, it SHALL mark RECOVERY_REQUIRED and SHALL NOT repeat inference. An explicit retry SHALL create a fresh operation ID under the existing retry contract.

#### Scenario: Translation is waiting when the backend restarts

- **WHEN** startup finds a complete uncancelled pre-inference WAITING_FOR_RESOURCES or WAITING_FOR_MODEL checkpoint and all source, revision, provider configuration, and exact model checks pass
- **THEN** it reconstructs the same operation ID and request, reacquires admission, remains waiting if admission is unavailable, and checks residency only after admission is granted

#### Scenario: Source or model identity changed

- **WHEN** recovery cannot prove the same source identity/revision or exact selected model/configuration
- **THEN** it marks the operation RECOVERY_REQUIRED or stale and makes no provider call

#### Scenario: Checkpoint is incomplete or from an unknown version

- **WHEN** required identity, revision, phase, or checkpoint fields are missing or unsupported
- **THEN** recovery fails closed and does not infer, publish, or fabricate replacement identity

#### Scenario: Cancellation races with readiness

- **WHEN** cancellation is persisted before the pre-inference compare-and-set succeeds
- **THEN** cancellation wins and no inference or publication begins

#### Scenario: Backend restarts during uncertain inference

- **WHEN** startup observes INFERENCE_STARTED without a durable completed inference/publication checkpoint
- **THEN** it records RECOVERY_REQUIRED, does not replay inference, and requires an explicit new operation for retry

### Requirement: Translation publication reconciles SQLite and files without assuming atomicity

Translation SHALL stage and validate all outputs before publication. Before changing final files, it SHALL persist a publication intent containing operation ID, source revision, exact target paths, staged-file fingerprints, and expected final-file state. After all final files are promoted and the source is rechecked, the operation's PUBLICATION_COMMITTED marker SHALL be recorded in SQLite. Asset Info Translation V1 has no separate artifact-metadata callback; if one is added later, it SHALL share that SQLite transaction. The specification SHALL treat filesystem promotion and SQLite commit as separate systems with a crash window between them.

Startup SHALL accept completion only when the committed SQLite operation marker and all expected final-file fingerprints agree. For a PUBLISHING row without the committed marker, recovery SHALL inspect final paths read-only. If every exact expected fingerprint matches, it MAY record PUBLICATION_COMMITTED in SQLite without changing files; this reconciles a completed promotion after a crash. If any expected output is missing, unreadable, or mismatched, startup SHALL mark RECOVERY_REQUIRED and preserve all evidence. It SHALL NOT automatically infer, publish, republish, roll back, delete, or clean staged/final/backup files after an ambiguous restart. If an in-process rollback cannot be verified, the same recovery-required state applies.

#### Scenario: Crash after file promotion but before SQLite commit

- **WHEN** startup finds a publication intent without a committed marker
- **THEN** it records PUBLICATION_COMMITTED only if every expected final fingerprint matches; otherwise it preserves files and marks RECOVERY_REQUIRED without a second publication

#### Scenario: SQLite commit completed and files match

- **WHEN** startup finds PUBLICATION_COMMITTED metadata and every final fingerprint matches
- **THEN** it reports the original operation completed without inference or publication

#### Scenario: Committed metadata and filesystem disagree

- **WHEN** an expected final file is missing or its fingerprint differs from the committed manifest
- **THEN** startup marks RECOVERY_REQUIRED and does not overwrite or republish the file

#### Scenario: In-process rollback is uncertain

- **WHEN** a known publication failure occurs but restoration of every prior final cannot be verified
- **THEN** the operation preserves filesystem evidence and enters RECOVERY_REQUIRED

### Requirement: Future Queue AI consumers use the existing plan wait contract

This is a deferred integration contract; #264 V1 SHALL NOT add a Queue AI stage. Current Queue stages, including subtitle preparation, SHALL remain independent of Ollama. If a Queue feature owner later introduces an explicit model-dependent stage, its plan SHALL use the existing ExecutionPlan waiting state with WAITING_FOR_MODEL and a bounded owning-workflow checkpoint. The Queue job SHALL remain queued and unclaimable while waiting, with no active FFmpeg reservation. For a model wait reached after claim, the owning workflow SHALL checkpoint completed work and release/deactivate its active reservation before waiting. The lifecycle SHALL report model readiness but SHALL NOT advance Queue stages or own Queue output state.

On resume, the owning scheduler/workflow SHALL atomically reload the job and checkpoint, verify cancellation, source identity/revision, exact model/configuration identity, checkpoint version, and owning stage, then re-evaluate existing scheduler gates and return through normal claim and reservation activation. It SHALL resume only the owning checkpoint and SHALL not replay completed work.

#### Scenario: Future Queue stage needs a nonresident model

- **WHEN** a future Queue consumer is otherwise runnable and requests an unloaded model
- **THEN** the execution plan durably reports WAITING_FOR_MODEL, the job remains queued, no active FFmpeg reservation is held, and no AI work has run

#### Scenario: Another Queue job is runnable

- **WHEN** one future Queue consumer waits for model readiness and another Queue job is ready
- **THEN** the waiting plan does not prevent the normal claim path from selecting the ready job

#### Scenario: Future Queue wait resumes

- **WHEN** model readiness returns after a wait or backend restart
- **THEN** the owning workflow resumes only after source/model/cancellation checks and normal scheduler gates pass

#### Scenario: Queue operation is cancelled or stale

- **WHEN** a waiting Queue job is cancelled or its source/model checkpoint is stale
- **THEN** it does not resume or issue AI work

### Requirement: Lifecycle preserves provider, role, admission, and Settings ownership

The lifecycle SHALL use the #261 shared endpoint, catalog, readiness, and transport. It SHALL accept an exact model identity from its consumer and SHALL NOT own model-role selection, fallback, endpoint configuration, feature selection, or resource policy. #265 SHALL own admission. #223 SHALL own Primary/Fast/Fallback and fallback decisions. #266 SHALL own feature model selection and Settings UI. Settings SHALL consume the backend runtime projection and SHALL NOT contact Ollama directly. Future Queue, OCR, and Copilot workflow state SHALL remain with their respective feature owners.

#### Scenario: Role owner chooses a model

- **WHEN** #223 or a feature owner supplies an exact resolved model identity
- **THEN** lifecycle operates on that model and does not select a replacement

#### Scenario: Endpoint configuration changes during a wait

- **WHEN** recovery finds a provider configuration revision different from the captured revision
- **THEN** the owning workflow marks the wait stale and sends no request through an implicit or duplicate endpoint authority

#### Scenario: Settings reads runtime

- **WHEN** #266 requests runtime state
- **THEN** the backend returns bounded installed, resident, active-use, waiting, warming, idle, unavailable, and expiry data without prompt or inference content
