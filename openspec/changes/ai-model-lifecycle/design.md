## Context

See proposal.md for motivation and scope.

The merged #261 provider owns the canonical Ollama endpoint, installed catalog, and metadata readiness. Its API client currently performs bounded GET requests for version, tags, and show; it has no /api/ps or /api/generate operation. Lifecycle transport extends that same owner and must not add another endpoint resolver or HTTP client. Translation keeps its feature-owned /api/chat cue contract.

Queue currently has a persisted ExecutionPlan with Status, WaitingState, Evaluation, and Reservation. Job stages are defined in backend/internal/handlers/job_lifecycle.go; they cover claim, workspace, media analysis, subtitle preparation, conversion, validation, publication, and cleanup. None is an Ollama or Translation stage. The scheduler planner reevaluates schedule, workspace, and worker/resource waiters. claimNextJob selects queued jobs with ready plans, activates a SchedulerReservation and sets the job running in one transaction, then prepares and freezes the execution plan. A plan that is not ready is returned to the queue. The normal pre-claim path does not currently wait for a model. Existing subtitle preparation is part of media processing and does not call Translation.

Explicit Asset Info Translation already captures a request with a source identity/reference/revision, selected provider/model, language, and output options, then rechecks source revision before publishing. The operation registry in subtitleExtractionOperations is process-local. Publication stages files, promotes them, may commit SQLite metadata, and signals publication completion in process memory. Filesystem changes and SQLite cannot share one atomic transaction.

## Goals / Non-Goals

**Goals:**

- Keep #261 as the sole Ollama endpoint and transport owner.
- Make admission a mandatory gate for all model-affecting requests, including inference against an already resident model.
- Ship explicit Translation as the only current consumer of the V1 wait/recovery path.
- Resume a waiting Translation operation under its original identity only from a complete, durable pre-inference checkpoint.
- Define a future Queue contract that uses existing plan waiting state and releases active FFmpeg reservations before waiting.

**Non-Goals:**

- Implementing #265 admission policy, thresholds, fairness, or capacity estimates in #264.
- Adding an AI stage or Translation behavior to current Queue processing.
- Moving workflow state into the lifecycle manager or creating a general-purpose AI operation store.
- Owning model roles, feature selectors, Settings UI, OCR, or Copilot workflows.

## Decisions

### Extend the #261 provider owner

Add bounded GET /api/ps discovery and POST /api/generate warmup to the #261 adapter. Reuse its validated shared endpoint, request contexts, HTTP client, timeouts, response-size limits, and sanitized diagnostics. Translation continues to own prompts and /api/chat translation requests; it passes the same keep_alive value on inference. Warmup sends the exact selected model, stream:false, keep_alive, and no prompt field. A successful bounded warmup plus an exact /api/ps residency match is required before reporting the model ready. Installed catalog membership or /api/show success is not residency evidence.

The lifecycle manager accepts a provider configuration identity and exact model ID, never an endpoint or model choice. Keep residency as a live projection of /api/ps, and keep MVForge use as active operation leases. Do not add residency to RuntimeState or RuntimeSnapshot, which remain host and encoder authorities. Distinguish a valid empty /api/ps response from provider unavailability or malformed data.

### Make #265 admission a required, acyclic port

#265 is merged and owns `backend/internal/airesource`. #264 depends on a private narrow interface over the shared service's `Admit`, `Await`, `CancelWait`, and `Release` methods. Admission requests carry the capacity domain, the #261-derived opaque provider configuration ID, exact model ID, consumer, stable workflow ID, unique request ID for each admission attempt, purpose (MODEL_LOAD or INFERENCE), and operation deadline. Decisions are ADMITTED with an opaque lease, WAITING with an opaque ticket, DENIED, or UNAVAILABLE. Leases and wait tickets remain opaque; release and wait cancellation use #265's idempotent contract. The adapter direction is one-way: #264 consumes #265, and #265 does not import lifecycle or Translation types.

The lifecycle manager depends only on this port. It does not calculate RAM/VRAM needs, host pressure, resident-model caps, inference concurrency, priorities, or retry fairness. It maps #265 WAITING to WAITING_FOR_RESOURCES and passes back #265 guidance. It releases leases according to the #265 lease contract, including when a warmup fails and /api/ps confirms the model is absent. It does not force-unload resident models.

A warmup or inference request is forbidden unless admission returns ADMITTED. This applies even when /api/ps already reports the model resident, because inference itself can consume protected NAS resources. When #265 returns WAITING, the operation awaits its opaque ticket with cancellation and bounded admission wait owned by #265. When it returns UNAVAILABLE, the Translation operation remains visibly WAITING_FOR_RESOURCES with an actionable bounded reason until cancellation or its operation deadline; it does not poll or retry admission automatically. DENIED fails that attempt. No unavailable/denied case calls /api/generate or /api/chat. A later explicit retry uses a new operation ID and fresh admission request IDs. Read-only /api/ps status remains available because it does not load or infer.

The five-minute readiness deadline begins after MODEL_LOAD admission is granted. Time spent in a #265 resource wait is governed only by #265. The production route constructs exactly one `airesource.Service` and injects it into Settings and Translation; consumers do not create separate services with divergent in-memory observations or reservations. The service has no real #256 observation producer in this change. Missing/stale telemetry, missing measured estimates, an unavailable critical-service guard, or an unset capacity domain therefore fails closed. `MVFORGE_AI_MODEL_LIFECYCLE_ENABLED` defaults false; setting it true does not bypass any admission check.

**Alternative rejected:** default to admission granted until #265 is available. That would silently bypass NAS protection. Duplicating thresholds in #264 would create conflicting policy, while having #265 import lifecycle internals would create a circular dependency.

### Project model lifecycle from provider evidence and operation leases

Report UNLOADED when no exact model entry is observed in /api/ps; WAITING_FOR_RESOURCES when #265 has not admitted the request; WARMING_UP during the admitted shared warmup; READY after warmup succeeds and /api/ps confirms residency but before a consumer lease begins; IN_USE while an admitted MVForge inference lease is active; IDLE for observed residency without an active MVForge lease; and FAILED for a bounded warmup/readiness failure. An externally loaded model is observed as IDLE until MVForge has its own readiness evidence and use lease. Status includes observation time and expiry; it does not claim exclusive ownership.

Use one short shared cache/rate limit for status polling. Settings consumes the backend projection and never contacts Ollama. No local authoritative residency database is needed.

### Warm only when an explicit consumer requests work

Warm on explicit Translation demand only after all feature readiness and source validation gates pass and #265 grants admission. If the exact model is already resident, skip generate but still acquire inference admission. Otherwise singleflight by shared-provider/configuration identity and exact model ID; issue one no-prompt generate and confirm with /api/ps. Default keep_alive is 30 minutes and applies to chat/inference as well. Ollama controls idle expiry; never send keep_alive:0, pull models, restart Ollama, or force eviction.

The lifecycle API has a default five-minute readiness window after admission and a one-request warmup flight followed by bounded /api/ps polling. It does not automatically retry warmup. An explicit Translation retry uses a new operation identity. One waiter's cancellation does not cancel other waiters. When all waiters leave, cancel the shared request, reconcile /api/ps, and hold a later retry until that reconciliation finishes. Cancellation wins before any consumer resumes.

### Persist only Translation's resumable wait checkpoint

Keep Translation as the owner of operation identity, source identity, progress, and cancellation. Store a Translation-specific durable row in SQLite and treat the current in-memory operation map as a cache/view. Persist the original operation ID and a bounded request snapshot: canonical source identity/reference plus path locator, source type/representation, stream index/format, source revision key, exact provider/model ID, non-secret provider configuration revision, language/context/output options, phase/status, checkpoint version, cancellation flag, wait deadline, and row version. A path is a locator, not source identity. Do not store cues, prompts, provider responses, translated text, credentials, or endpoint URLs.

Persist WAITING_FOR_RESOURCES when admission is pending or unavailable, and persist WAITING_FOR_MODEL after admission is granted but before warmup begins. On startup, reconstruct the same operation ID and request snapshot only from either complete supported pre-inference wait phase, when every required identity field is valid, cancellation is false, and no inference or publication intent was recorded. Re-resolve the source through the existing Translation resolver and compare canonical source identity and revision; resolve the current shared #261 provider configuration and require the captured provider configuration revision and exact selected model to match. A changed path may be accepted only if the canonical identity/revision resolver proves it is the same source. Missing or changed data marks the operation RECOVERY_REQUIRED without provider work. Recovery reacquires admission; if #265 is still unavailable or returns WAITING, retain the resource wait without polling automatically or calling Ollama. A granted admission moves the operation to WAITING_FOR_MODEL and starts the five-minute readiness deadline.

Create a fresh operation-owned workspace after validation; never trust or reuse an arbitrary persisted workspace path. Use a compare-and-set transaction on operation ID, row version, phase, and cancellation before leaving the waiting checkpoint. Reacquire #265 admission, inspect /api/ps, and warm only if still needed. Recheck cancellation, source revision, and model/configuration immediately before durably changing phase to INFERENCE_STARTED and issuing the first inference call. Cancellation is persisted before cancelling the process context.

After restart, only a complete WAITING_FOR_MODEL checkpoint is automatically resumable. If startup finds INFERENCE_STARTED or a later nonterminal phase, mark RECOVERY_REQUIRED and do not repeat inference. An explicit retry creates a new operation ID using the current source/model, consistent with the existing retry contract.

### Reconcile filesystem publication with SQLite without claiming atomicity

After inference completes, validate all outputs and stage them in the operation workspace. Before touching final paths, persist a publication intent in the operation row containing the operation ID, source revision, exact artifact targets, staged-file fingerprints, and expected final-file state. Promote files through the existing guarded publication path and rollback behavior. After every final file is promoted and the source is rechecked, commit the operation's PUBLICATION_COMMITTED marker in SQLite. The current Asset Info Translation publisher has no separate artifact-metadata callback; if a future owner adds one, it must share the same SQLite transaction as that marker. This is an ordered protocol, not a cross-system atomic transaction.

On startup, a row with PUBLICATION_COMMITTED is complete only if its operation marker and every expected final-file fingerprint match; otherwise report RECOVERY_REQUIRED and do not republish. For PUBLISHING without a committed marker, recovery reads final files only. If every exact expected final fingerprint is present, it records PUBLICATION_COMMITTED in SQLite without changing files; this reconciles a promotion that completed before the process crashed. If any target is missing, unreadable, or mismatched, mark RECOVERY_REQUIRED, preserve files, and do not auto-promote, roll back, delete, clean, infer, or retry publication. If an in-process rollback cannot be verified, use the same recovery-required handling. Incomplete or unknown checkpoint versions also fail closed. This prevents duplicate inference and publication while acknowledging that SQLite and filesystem cannot commit atomically.

**Alternative rejected:** treat filesystem renames plus SQLite metadata as one atomic operation. They are separate systems and a crash can occur between them. Automatic replay or rollback after an ambiguous crash could overwrite user files or publish a second result.

### Define Queue waiting for future consumers only

No current Queue JobStage requires Ollama, so #264 adds no Queue stage, model invocation, Queue Translation flow, or Queue implementation task to V1.

When a Queue feature owner later introduces a model-dependent stage, the reusable wait contract uses the existing ExecutionPlan.Status=waiting and WaitingState=WAITING_FOR_MODEL. Its bounded Evaluation checkpoint carries the owning stage key, stable consumer ID, source identity/revision, exact provider/model configuration identity, deadline, cancellation/version, and completed-work checkpoint. The Queue job remains queued and the plan is not claimable. Before an AI stage reached after claim can wait, its owning workflow must checkpoint completed work and release/deactivate the active worker reservation; the model lifecycle manager never parks an active FFmpeg worker.

When ready, the owning scheduler/workflow reloads the job and plan atomically, confirms it is not cancelled, validates the frozen source revision, exact selected model/configuration, checkpoint version, and owning stage, then reevaluates existing schedule/workspace/resource gates and returns to the normal claim/reservation path. It resumes from the owning checkpoint without replaying completed work. The lifecycle manager reports readiness only; it does not own Queue stage progression or output state. Queue integration stays deferred until a real Queue AI consumer is specified.

### Keep model identity and presentation with their owners

#223 selects Primary/Fast/Fallback roles and fallback behavior. Feature settings and UI belong to #266. Each consumer passes one resolved model identity; lifecycle never chooses or silently substitutes a model. #261 owns endpoint and provider transport. #265 owns admission. Translation owns its checkpoint. Future Queue/OCR/Copilot owners own their workflows.

## Risks / Trade-offs

- A warmup can consume substantial NAS resources → require a positive #265 lease for both load and inference; fail closed when the port is absent.
- Ollama can finish loading after cancellation → cancellation wins for the consumer; reconcile /api/ps before another warmup and do not evict the resident model.
- External clients can change residency or expiry → report observed state with a timestamp and no exclusivity claim.
- Large models can exceed five minutes → return a bounded actionable timeout; do not retry indefinitely.
- Translation restart can find a partial or unknown filesystem publication → preserve files, mark RECOVERY_REQUIRED, and require explicit resolution; do not automatically replay.
- Queue has no current Ollama stage → defer its integration until the owning feature defines that consumer.

## Migration Plan

1. Add the Translation-specific SQLite checkpoint with an additive migration. Existing in-memory operations cannot be reconstructed after an old process exits; do not fabricate checkpoints for them.
2. Deploy read-only /api/ps observation and lifecycle reporting independently. #265 is already wired, but effectful warmup and inference stay disabled by default. The route does not fabricate #256 observations: if the admission service lacks a current host sample, provider residency sample, critical-service guard, measured estimate, or configured capacity domain, the operation remains waiting/denied and sends no model-affecting request.
3. Enable Translation warmup/inference only after #256 observation sources and critical-service guards are validated on the target NAS and selected model estimates are calibrated from actual measurements. The feature flag does not bypass the #265 admission decision.
4. Existing Queue jobs and execution plans remain unchanged. A future Queue AI consumer must add its own migration and integration under its feature issue.
5. On rollback, preserve waiting rows, publication intents, artifacts, and staged evidence for inspection. Never dispatch them without validation or delete files as rollback cleanup. Real NAS calibration and #256 integration are external rollout gates, not #264 implementation work or test-fixture substitutes.
