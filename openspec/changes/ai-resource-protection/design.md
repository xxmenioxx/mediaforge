## Context

See `proposal.md` for motivation and scope. The approved #264 change requires a positive #265 decision before warmup and before every inference, and explicitly leaves the admission API to #265. #261 owns the Ollama endpoint and transport; #264 extends that provider owner with `/api/ps` and warmup calls. Neither may be duplicated here.

The current `RuntimeState` is the mutable host view and exposes `DetectedAt`, available/total memory, CPU/GPU metrics, and metric capabilities. `RuntimeSnapshot` is immutable evidence. The scheduler's `resource_policy.go` gates Queue/FFmpeg reservations and treats missing available memory differently from this service's fail-closed contract, so reuse its host telemetry source but not its decision policy. The critical-service priority authority remains #256.

## Goals / Non-Goals

**Goals:**

- Give #264 and later AI consumers one acyclic, provider-neutral admission port.
- Reserve model-load capacity and inference capacity independently, with persistent, releasable grants and waits.
- Make every admission explainable from current telemetry, configured estimates, and explicit policy values.
- Keep NAS resource decisions separate from provider configuration and existing Queue worker reservations.

**Non-Goals:**

- Owning Ollama HTTP, endpoint resolution, `/api/ps`, model lifecycle, inference prompts, feature/model roles, or workflow recovery.
- Changing Queue scheduler policy, controlling Docker, restarting Ollama, evicting models, pulling models, or tuning global CPU/GPU settings.
- Claiming that MVForge admission can serialize or protect work started by unrelated Ollama clients; those residents are counted, while server limits and external work remain separately reported.

## Decisions

### Keep the admission contract below lifecycle and provider packages

Put the contract and policy engine in a backend package that can be imported by #264 without importing handlers, Ollama transport, or lifecycle packages. The port owns its purpose values (`MODEL_LOAD`, `INFERENCE`), decision states, opaque lease/wait handles, and bounded reason codes. Inputs carry opaque capacity-domain/provider-configuration keys, exact model IDs, stable consumer and workflow identities, a request ID unique to one admission attempt, and neutral estimate hints such as the selected context profile. An explicit retry retains the workflow identity and uses a new request ID; repeating an active request ID remains idempotent, while terminal `UNAVAILABLE` stays terminal for that attempt. Inputs do not carry endpoints, secrets, feature state, or lifecycle structs.

Use a nonblocking request operation, an event-driven wait/reattach operation, ticket cancellation, explicit lease release, and a dry-run preview. A wait can remain durable without occupying a worker. `WAITING` has a ticket; `UNAVAILABLE` does not enter a retry queue. The lifecycle maps outcomes into its own states and remains solely responsible for whether and when to call the provider.

**Alternative rejected:** let #265 call back into #264 or reuse lifecycle states in its API. That reverses ownership and recreates the circular dependency the approved change forbids.

### Reuse authoritative observations, not adjacent policy

Read fresh host measurements through the existing runtime authority and consume provider residency through a normalized observation supplied by the #261/#264 adapter. The admission package does not make provider calls or persist a second authoritative residency inventory. Its observation includes source, sample time, known/unknown metric status, provider configuration identity, and every resident model in the capacity domain, including models loaded by other clients.

The 24 GiB policy starts with one resident, one MVForge inference, one model load at a time, and a configurable 6 GiB available-memory reserve. Treat 6 GiB as the issue's initial value, not a measured safety guarantee. Use a 60-second default freshness bound for host and resident observations, aligned with the runtime collector before release. Do not admit on zero, stale, or missing required measurements.

Keep the scheduler's FFmpeg worker reservations separate. Resource admission computes projected AI headroom from current available memory and conservative incremental demand; it does not subtract model memory twice when the host measurement already includes resident usage. Each model profile distinguishes resident footprint, transient load peak, and per-inference working set/KV-cache assumptions. Profiles are keyed by opaque provider/model identity and include estimate source and revision; they are not a second model catalog. Model file size alone is not a sufficient estimate. #264 passes actual workload hints when available and does not calculate RAM/VRAM itself. A missing or incompatible estimate yields `UNAVAILABLE` with a reason instead of a guessed zero.

The current policy evaluator uses fresh CPU, iGPU, swap/zram, and service pressure metrics when the relevant authority provides them. Unknown optional metrics remain visibly unknown; metrics marked required by active policy block admission if absent. Jellyfin and other critical-service allow/wait/unavailable decisions come from #256's authority, so #265 does not define service rankings or weaken that guard.

**Alternative rejected:** reuse the scheduler's `ResourceDecision` or extend `RuntimeState` with model lifecycle state. The scheduler decides whether Queue work can reserve FFmpeg workers, and runtime snapshots are host evidence; neither is the authority for AI model residency or inference leases.

### Keep model-load, residency, and inference accounting distinct

Maintain separate limits for resident models, active model loads, and active MVForge inferences. Defaults are one resident model, one model load, and one inference. Resident count comes from the current provider census, not persisted MVForge ownership, and includes externally loaded models. A model-load lease reserves admission before warmup but cannot authorize inference; after warmup/reconciliation it is released and current provider telemetry continues to count residency. Every inference obtains a separate one-request lease even if its model is already resident.

The inference ceiling is global across MVForge consumers in the capacity domain; the resident ceiling counts distinct provider/model identities observed there. Future settings may raise resident count to two or more, but admission still checks current headroom and estimates. A 64 GiB host does not auto-select two residents. A load and an inference may overlap only when both separate limits and combined projected memory/critical-service checks allow them.

**Alternative rejected:** count only models loaded by MVForge. The shared provider may contain models loaded by unrelated clients, and ignoring them would oversubscribe the same host. The service counts those models but never unloads them.

### Persist leases and waits without persisting lifecycle state

Store policy settings and a minimal admission journal in SQLite. Key requests idempotently by stable consumer ID, per-attempt request ID, purpose, and domain; retain and validate the stable workflow ID with the record. Persist opaque lease identity, bounded operation deadline, wait order/deadline, and release state; do not store source paths, prompts, provider payloads, credentials, or lifecycle state. The journal is policy accounting, not an authoritative model-residency database.

On process start, enter a reconciliation state and issue no grants until required host and provider observations are fresh. Restore unexpired grants and reattach a lifecycle retry to its existing logical request ID. An explicit new attempt after terminal `UNAVAILABLE` retains the stable workflow ID and gets a new request ID. Count resident models from the provider census, including external residents. Keep an uncertain inference lease counted until the bounded provider-request deadline plus cleanup grace; do not clear all leases simply because the backend restarted. The #264 recovery path decides whether a workflow resumes and reacquires by its stable identity.

Release is idempotent. #264 holds a model-load lease through warmup and residency reconciliation, then releases it; release never unloads the model. An inference lease spans exactly one provider request and remains held until the call ends or the bounded crash-recovery expiry is reached. Cancellation of a waiting ticket removes it; cancellation after grant releases the grant only after the provider operation is known to have ended or its safety deadline expires.

**Alternative rejected:** keep all accounting in memory and reset it at startup. A restart could otherwise forget an active inference and grant another one while Ollama is still consuming resources.

### Use bounded event-driven fairness

Keep separate FIFO wait lanes for `MODEL_LOAD` and `INFERENCE`, with fair turn-taking between eligible lanes. Reevaluate on lease release or a new canonical telemetry observation, not a high-frequency admission timer. The default maximum admission wait is five minutes, distinct from #264's five-minute model-readiness deadline after admission. On expiry, return `DENIED/admission_wait_expired`. On missing authority or stale required evidence, return `UNAVAILABLE` immediately; do not queue a retry loop. Cancellation wins before a later grant, and a grant/cancel race must return an explicit lease for release or a confirmed cancellation.

**Alternative rejected:** let the waiting consumer poll resource status or keep a Queue/FFmpeg worker parked. That consumes scarce capacity and creates uncontrolled refresh traffic.

### Separate MVForge policy from actual Ollama process settings

Persist MVForge's limits and estimates under a dedicated AI resource-protection setting. The enabled switch defaults on; disabling it denies new admission with `protection_disabled` while preserving release of existing leases. It is a safe-stop switch, not a bypass around #264's required grant.

Report `OLLAMA_MAX_LOADED_MODELS` and `OLLAMA_NUM_PARALLEL` in a separate server-configuration projection. Use `unknown` unless an approved read-only source supplies actual current values; neither `/api/ps` nor MVForge limits prove those environment values. Compare requested and observed server configuration only when both are explicitly available, then report `restart required` without making the change. Saving MVForge policy does not modify Ollama, its environment, Docker, or the server's restart state.

**Alternative rejected:** present MVForge limits as Ollama's effective limits or infer environment values from current residency. Both would overstate what the backend knows and could mislead operators about external clients.

## Risks / Trade-offs

- **Model estimates can be wrong** → label source and revision, include load peak and context-sensitive inference cache, require measured/overridden profiles when no benchmark-backed profile exists, and keep the 6 GiB reserve provisional until #256 NAS measurements confirm it.
- **Telemetry gaps can pause AI work** → expose `UNAVAILABLE` and its source/reason, keep waits cancellable, and retry only after an explicit request or recovery action.
- **External clients can change Ollama residency between observations** → recheck immediately before a load, include all observed residents, report actual server limits separately, and document that MVForge cannot serialize clients it does not own.
- **Persistent leases can outlive their caller** → use bounded request deadlines and cleanup grace, stable idempotency keys, and restart reconciliation; never revoke an uncertain inference grant immediately.
- **The #256 critical-service contract may not yet be available** → keep a narrow injected guard; return `UNAVAILABLE` when a required protected-service decision cannot be obtained rather than creating a competing priority policy.

## Migration Plan

1. Add validated defaults and the admission journal without changing existing provider or lifecycle records.
2. Wire host, provider-residency, estimate, and #256 critical-service observations. Keep new effectful operations denied/unavailable until all required signals are wired.
3. Integrate #264 so both warmup and every inference require a positive lease. Keep its workflow checkpoints and lifecycle states in #264.
4. Expose policy, capacity preview, counters, and separate Ollama server-configuration evidence in Settings.
5. Validate the 24 GiB fixtures and benchmark the provisional reserve on the NAS before enabling the policy for production workloads. A 64 GiB profile may configure two residents only after its estimates and headroom pass the same evaluator.

For rollback, stop issuing new grants and preserve existing lease/wait records for diagnosis. Do not unload models, reset the journal, restart Ollama, or dispatch old waits automatically after rollback.
