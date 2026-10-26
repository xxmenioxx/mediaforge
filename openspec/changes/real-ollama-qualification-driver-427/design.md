## Context

PR #426 is integrated at base `0cce40ac6de3f825abe568a1b71e749f37001991`. Its Compose network is intentionally `internal: true`; its `real-plan` validates an endpoint and model but never contacts Ollama. The existing plan currently reports a successful plan even though live telemetry and admission remain pending, which can be mistaken for readiness.

The #265 `airesource` service owns `MODEL_LOAD`/`INFERENCE` decisions and opaque lease lifecycle. The available `RuntimeStateHostSource` reads the local database `RuntimeState` and projects total/available memory plus CPU load (and optional GPU use). It does not provide swap pressure or provenance binding to the shared NAS. `ProviderObservation` and `CriticalServiceObservation` are data contracts, but this checkout has no concrete source adapters for Ollama residency or the critical-service guard. `PublishObservation` accepts a plain observation and does not authenticate its source. The sandbox database is new and isolated, so it does not contain the production NAS runtime state.

No code path in the current sandbox can prove that the declared Ollama endpoint is the actual shared NAS provider or that a route is restricted to only that endpoint. The mock network must remain internal, and the user explicitly prohibits NAS Docker, NAS access, real warmup, and inference in this implementation.

## Goals / Non-Goals

**Goals:**

- Replace the ambiguous successful real plan with a structured, non-contacting `BLOCKED` preflight that records an exact declared source SHA but does not claim it is bound to a verified sandbox image.
- Require exact target identity: endpoint, model, workflow, full source SHA, and run ID.
- Make every missing trust/telemetry/admission/network condition visible as a bounded reason.
- Prove by negative tests that neither preflight nor unsupported execution options open a network connection or create production side effects.
- Preserve #265 ownership and the existing mock sandbox unchanged.

**Non-Goals:**

- No real Ollama execution, warmup, endpoint probe, model change, or resident-model operation.
- No new telemetry collector, NAS agent, Docker network, egress proxy, admission API, or alternate policy engine.
- No modification of `airesource.Service`, AI model lifecycle, Translation, OCR review, or Publisher.
- No acceptance of exported/caller-provided telemetry or admission as live authorization.

## Decisions

1. **Keep this delivery fail-closed until trust infrastructure exists.** Add a non-contacting `real-preflight` alias and retain `real-plan` as a compatibility spelling. Both require exact endpoint/model/workflow/source SHA/run ID and always report `executionAllowed: false`. The source SHA is recorded as a declaration but remains unverified against a built sandbox image. The current source gaps are explicit blockers, not a state that a CLI flag can clear. Alternatives considered: adding a `run-real` command with caller-supplied evidence or a caller-provided admission boolean; rejected because they cannot prove source authenticity, capacity domain, or endpoint restriction and would bypass #265.

2. **Do not synthesize an admission success path.** Resource Admission remains owned by `airesource.Service`; only the application lifecycle may call `EnsureReady` and acquire a purpose-scoped inference lease. The Python harness has no trusted API to the isolated application's live `MODEL_LOAD` and `INFERENCE` operations, and the sandbox database cannot derive shared NAS capacity. Therefore the preflight reports those checks unavailable. Alternatives considered: publish test observations into the sandbox DB or accept Preview; rejected because mock or caller data must never authorize real work.

3. **Do not add external connectivity to the mock network.** `internal: true` remains intact. A future narrow egress path must separately attest the exact endpoint, DNS/address binding, TLS or equivalent peer identity, denied production networks, and actual engine enforcement. Until then, no endpoint-resolution or health probe is attempted. Alternatives considered: host networking, Docker host gateway, remote Docker Engine, unrestricted egress, or attaching to the production network; all rejected.

4. **Keep operator authorization a future moment-of-run gate.** This version exposes no `run-real` mode and no flag that turns a preflight into authorization. Once trusted sources and the endpoint firewall are implemented, a separate design must define one-time operator intent, bind it to the exact SHA/model/workflow/endpoint/run ID, and combine it with actual #265 leases. No state is persisted in production.

5. **Use sanitized, stable blocker identifiers.** The report names each missing gate and avoids prompts, model output, credentials, host paths, or opaque lease tokens. Endpoint and model are exact requested target fields; they are validated syntactically but explicitly marked unverified and never contacted.

## Risks / Trade-offs

- [The preflight cannot produce GO in this change] → This is intentional and accurately reflects missing telemetry and network authorities; report the implementation as BLOCKED for real qualification and keep the mock suite available.
- [Users may confuse syntactic target validation with provider identity] → Emit separate `targetDeclaration` and `providerIdentity` statuses, with identity marked unavailable.
- [Future implementations may add a route before trust contracts are ready] → OpenSpec requires both source-attributed live observations and enforceable exact-endpoint egress; tests must keep unsupported execution rejected.
- [Existing scripts may parse `real-plan` as PASS] → Keep the JSON field names stable where practical, change the result to `BLOCKED`, and update tests/runbook to make the semantic change explicit.

## Migration Plan

No persistent data or service configuration changes. The mock command and Compose project remain as-is. Operators using `real-plan` will receive `BLOCKED` rather than a PASS-shaped plan; use `real-preflight` for the explicit spelling. Rollback consists of reverting this feature branch; no NAS or model state is touched.

## Open Questions

None for this fail-closed delivery. A future issue must provide an authenticated telemetry source and enforceable exact-endpoint egress before a run-capable design can be approved.
