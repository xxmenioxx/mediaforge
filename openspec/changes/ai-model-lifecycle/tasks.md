## Delivery boundaries

V1 covers model lifecycle plus the existing explicit Asset Info Translation operation. Read-only /api/ps observation and lifecycle reporting can be implemented before #265. Any warmup or inference requires the #265 admission port first. If that port is absent or unavailable, V1 reports WAITING_FOR_RESOURCES/admission_unavailable, issues no /api/generate or /api/chat request, and waits for explicit retry or a deployment with the admission authority.

External prerequisite, not a #264 task: #265 defines and ships the provider-neutral admission interface and policy. The interface must be acyclic, expose only opaque leases and grant/wait/deny/unavailable outcomes, and must not import lifecycle types. #261 remains the provider transport owner.

## V1 implementation tasks

### Shared provider and lifecycle

- [ ] 1.1 Extend the #261 shared provider adapter with bounded /api/ps discovery and no-prompt /api/generate warmup, reusing endpoint validation, request cancellation, timeouts, response limits, and sanitized diagnostics.
- [ ] 1.2 Carry the configured keep_alive value on Translation /api/chat requests. Use a 30-minute default; never put prompts or source content in warmup.
- [ ] 1.3 Require a positive #265 admission lease before warmup and before every inference, including use of a resident model. Verify missing/unavailable admission produces no model-affecting provider request and no local policy fallback.
- [ ] 1.4 Implement the lifecycle states as a projection of #261 catalog and /api/ps evidence, admitted warmup, and active MVForge inference leases. Distinguish valid empty residency from unavailable data and do not add authoritative model rows to RuntimeState/RuntimeSnapshot.
- [ ] 1.5 Add same-provider-configuration/exact-model singleflight, independent waiter cancellation, final-waiter cancellation, /api/ps reconciliation, five-minute readiness deadline after admission, and finite retries.
- [ ] 1.6 Expose the bounded backend runtime projection for #266, with observed timestamp/expiry and installed, loaded, active-use, waiting, warming, idle, failed, and unavailable facts. Do not implement Settings UI or model selection here.

### Durable Translation wait and recovery

- [ ] 2.1 Add an additive SQLite record for the existing explicit Translation operation, preserving its operation ID and validated bounded request snapshot, source identity/reference/revision, exact selected provider/model/configuration revision, phase/version, cancellation, and wait deadline; exclude prompts, cues, responses, output text, secrets, and endpoint URLs.
- [ ] 2.2 Persist WAITING_FOR_RESOURCES while admission is pending and WAITING_FOR_MODEL after admission but before warmup; persist cancellation before cancelling the in-memory context. Reconstruct the same operation ID after restart only from a complete supported pre-inference wait checkpoint; reacquire admission and revalidate source, model, configuration, and cancellation before resume.
- [ ] 2.3 Persist INFERENCE_STARTED before the first inference call. Treat restart during inference or any later nonterminal phase as RECOVERY_REQUIRED; never replay inference automatically. Keep explicit retry on a new operation ID.
- [ ] 2.4 Journal publication intent and staged-file fingerprints in SQLite before final-file promotion. Commit artifact metadata with PUBLICATION_COMMITTED/COMPLETED in one SQLite transaction after file promotion and source recheck. On restart, reconcile fingerprints read-only; preserve files and fail closed on an ambiguous or mismatched publication.
- [ ] 2.5 Add focused recovery regressions for valid wait resume, cancellation race, missing/unknown checkpoint fields, changed source/model/configuration, uncertain inference, crash between file promotion and SQLite commit, completed commit, and mismatched final files.

### Lifecycle regressions and validation

- [ ] 3.1 Add focused provider/lifecycle regressions for /api/ps, warmup contract, admission absent/wait/deny/grant, exact model matching, singleflight, cancellation, timeout, and bounded retry.
- [ ] 3.2 Verify the existing Queue stage set makes no Ollama call and that lifecycle changes do not add or imply a Queue AI stage.
- [ ] 3.3 After implementation, run ./scripts/verify.sh --auto and require its final ALL CHECKS PASSED result.

## Deferred integration, outside #264 V1

Queue AI integration remains deferred until a Queue feature owner defines an actual model-dependent stage. At that time, use ExecutionPlan.WaitingState=WAITING_FOR_MODEL and an owning-workflow checkpoint; keep the job queued and unclaimable, hold no active FFmpeg reservation, release a reservation before any mid-execution wait, and resume only through the normal scheduler claim path after source/model/cancellation validation. Do not add a Queue stage in #264.

OCR and Copilot use the same lifecycle boundary only when their feature owners integrate their workflow-owned checkpoints (#224/#263). Settings UI and per-feature selectors remain with #266. Model roles and fallback remain with #223.
