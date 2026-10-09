## V1 scope and rollout gate

V1 implements the shared-provider extension, admission-gated lifecycle, bounded runtime projection, and durable recovery for the existing explicit Asset Info Translation operation. The protected Translation path is off by default. #265 owns the admission service and all capacity policy; #256 telemetry, real NAS measurements, measured model estimates, and critical-service guards are external prerequisites to enabling the protected path in production. Missing or stale prerequisites remain unavailable and fail closed.

## V1 implementation tasks

### Shared provider and lifecycle

- [x] 1.1 Extend the #261 shared provider adapter with bounded `/api/ps` discovery and no-prompt `/api/generate` warmup. Reuse its endpoint resolver, HTTP client, cancellation, timeouts, response limits, and sanitized diagnostics.
- [x] 1.2 Pass `keep_alive` to Translation `/api/chat` and warmup requests, defaulting to 30 minutes. Start the five-minute readiness deadline only after MODEL_LOAD admission; never include a prompt in warmup.
- [x] 1.3 Require #265 MODEL_LOAD admission before warmup and separate INFERENCE admission before every inference, including resident models. Handle ADMITTED, WAITING, DENIED, and UNAVAILABLE with opaque leases/tickets, cancellation, and idempotent release; do not duplicate resource policy or fabricate grants.
- [x] 1.4 Implement UNLOADED, WAITING_FOR_RESOURCES, WARMING_UP, READY, IN_USE, IDLE, and FAILED as projections of provider residency and MVForge inference leases. Treat `/api/ps` as residency authority; distinguish valid empty data from unavailable or malformed data.
- [x] 1.5 Add singleflight by provider configuration and exact model, independent waiter cancellation, final-waiter cancellation, `/api/ps` reconciliation after ambiguous cancellation, bounded readiness wait, and finite retries. Do not download models, force eviction, or restart Ollama/Docker.
- [x] 1.6 Expose bounded runtime status for #266, including installed/resident models, lifecycle state, resource waits/failures, expiry when reported, active operations, freshness, and unavailable states. Add no Settings UI or model selection.

### Durable Translation wait and recovery

- [x] 2.1 Add a bounded SQLite checkpoint for the explicit Translation operation, preserving its operation ID, canonical source identity/revision, provider configuration revision, exact model, validated request metadata, phase/version, deadline, cancellation, and publication fingerprints. Exclude prompts, cues, translations, responses, credentials, and endpoint URLs.
- [x] 2.2 Persist WAITING_FOR_RESOURCES and WAITING_FOR_MODEL, persist cancellation before cancelling the in-memory context, and reconstruct the same operation ID only from a complete supported pre-inference checkpoint. Reacquire admission and validate source, model, configuration, deadline, and cancellation before resume.
- [x] 2.3 Persist INFERENCE_STARTED before provider inference. Treat uncertain inference and post-inference restart as RECOVERY_REQUIRED; never replay automatically. Explicit retries receive a new operation ID.
- [x] 2.4 Persist publication intent and fingerprints before file promotion. After promotion and source recheck, commit the operation marker in SQLite. The current Asset Info Translation publisher has no separate artifact-metadata callback. Reconcile filesystem and SQLite evidence by exact fingerprints without assuming cross-system atomicity; preserve files and fail closed on ambiguity.
- [x] 2.5 Add focused regressions for valid waiting recovery, cancellation, incomplete/unknown checkpoints, changed source/model/configuration, uncertain inference, crash between file promotion and SQLite commit, exact reconciliation, and mismatched-output preservation.

### Lifecycle regressions and validation

- [x] 3.1 Add focused provider/lifecycle regressions for warmup and residency contracts, admission outcomes, absent/stale telemetry and critical-service guards, singleflight, cancellation and retry races, leases, timeout, and bounded runtime status.
- [x] 3.2 Inspect actual Queue stages, planner, claim, and worker-reservation lifecycle. Confirm no current Queue stage requires Ollama. Document the reusable WAITING_FOR_MODEL contract for future Queue AI consumers; add no AI stage or Queue implementation to V1.
- [x] 3.3 Validate with `openspec validate ai-model-lifecycle --type change --strict` and `./scripts/verify.sh --auto`; the canonical verifier completed with `ALL CHECKS PASSED`.

## Deferred integration

Queue AI integration is deferred until a Queue feature defines a model-dependent stage. That consumer uses the existing plan waiting state with `WAITING_FOR_MODEL`, remains queued and unclaimable, releases any active FFmpeg reservation before waiting, and resumes through normal scheduling only after source/model/cancellation validation. OCR/Copilot workflow integration remains with their owners (#224/#263); Settings UI/model selection remains #266; model roles/fallback remain #223. #264 adds no Queue stage, Queue worker wait, or OCR decision changes.

## Remaining external prerequisite

Production activation remains gated on #256 critical-service telemetry and actual NAS calibration of the #265 resource estimates and guards. This cannot be completed by local code verification. Until those observations are verified, keep `MVFORGE_AI_MODEL_LIFECYCLE_ENABLED` unset/false; unavailable admission must not fall back to the legacy path when the protected lifecycle is enabled.
