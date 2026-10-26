## Context

PR #430 (built on #426) is integrated on the current main base. The sandbox Compose network remains `internal: true`; `real-preflight` validates an endpoint and model but never contacts Ollama. It always reports `BLOCKED`, with execution disabled.

The #265 `airesource` service remains the sole owner of `MODEL_LOAD`/`INFERENCE` decisions and opaque lease lifecycle. `RuntimeStateHostSource` reads local database state and is not shared-NAS authority. This increment adds a bounded Ed25519-signed snapshot reader, configured with an exact capacity domain and expected collector, host, provider, and critical-guard identities. It requires fresh RAM, swap, and CPU samples, maps them into the existing provider-neutral `Observation`, and uses the existing evaluator and leases. The production route refreshes from this source for Preview/Admit and rejects unsigned `PublishObservation` calls in trusted-source mode. Source errors clear cached trusted evidence. The legacy in-process service constructor remains available for existing tests and non-production adapters.

The signed reader does not create or publish a NAS snapshot. No host-side signer is installed, no key or snapshot is provisioned, and no real source has been inspected. The current provider and critical-service fields define an observation contract, not proof that Ollama `/api/ps` or service activity was measured correctly. The intended host observer must read memory/swap from OS telemetry, calculate CPU from successive counters, query Ollama residency through an explicitly read-only observation path, and obtain Jellyfin/MVForge/FFmpeg/worker activity from least-privilege host/service-manager signals. It must not need the Docker socket, change Ollama state, pull/evict models, or access media libraries. The Ed25519 private key must remain outside the backend; only a read-only public verification key and snapshot mount belong in the application. Until that host authority and its measured inputs are provisioned and reviewed, trusted admission returns `UNAVAILABLE`.

No code path in the current sandbox can prove that the declared Ollama endpoint is the actual shared NAS provider or that a route is restricted to only that endpoint. The snapshot's provider source identity is not endpoint identity. The mock network must remain internal, and this implementation does not access NAS Docker, the NAS, real warmup, or inference.

## Goals / Non-Goals

**Goals:**

- Replace the ambiguous successful real plan with a structured, non-contacting `BLOCKED` preflight that records an exact declared source SHA but does not claim it is bound to a verified sandbox image.
- Require exact target identity: endpoint, model, workflow, full source SHA, and run ID.
- Make every missing trust/telemetry/admission/network condition visible as a bounded reason.
- Verify signed, fresh observations and exact capacity/source identity before the production #265 admission service can use them.
- Prove by negative tests that neither preflight nor unsupported execution options open a network connection or create production side effects.
- Preserve #265 ownership and the existing mock sandbox unchanged.

**Non-Goals:**

- No real Ollama execution, warmup, endpoint probe, model change, or resident-model operation.
- No new telemetry collector/NAS agent, Docker network, egress proxy, alternate policy engine, or real execution route.
- No modification of AI model lifecycle, Translation, OCR review, or Publisher.
- No acceptance of exported/caller-provided telemetry or admission as live authorization.

## Decisions

1. **Keep qualification fail-closed until every authority and egress gate exists.** `real-preflight` and its `real-plan` compatibility spelling require exact endpoint/model/workflow/source SHA/run ID and always report `executionAllowed: false`. The source SHA remains unverified against a built sandbox image. Adding an authenticated reader makes evidence usable by the production #265 evaluator when explicitly provisioned, but does not authorize the qualification driver or prove real inputs. Missing source/configuration remains unavailable; no CLI flag can clear these blockers. Alternatives considered: caller-provided evidence/admission booleans or a run command; rejected because they cannot prove source authenticity, capacity domain, or endpoint restriction.

2. **Use the canonical admission evaluator, not a parallel policy engine.** `airesource.Service` refreshes the configured signed source, replaces rather than merges cached evidence, and passes it through its existing policy/lease path. A new trusted-source constructor rejects unsigned publication; source failures evict cached observations. `Preview` remains lease-free and never authorizes work. Existing application lifecycle owners remain responsible for `EnsureReady` and purpose-scoped leases. The isolated sandbox has no NAS source and continues to report live admission unavailable.

3. **Do not add external connectivity to the mock network.** `internal: true` remains intact. A future narrow egress path must separately attest the exact endpoint, DNS/address binding, TLS or equivalent peer identity, denied production networks, and actual engine enforcement. Until then, no endpoint-resolution or health probe is attempted. Alternatives considered: host networking, Docker host gateway, remote Docker Engine, unrestricted egress, or attaching to the production network; all rejected.

4. **Authenticate snapshots with asymmetric signing.** The fixed-schema JSON payload is signed as its exact embedded UTF-8 bytes using Ed25519; the application reads only a raw public verification key. It rejects non-regular/symlink final paths, oversized files, unknown JSON fields, invalid RAM/swap/CPU values, duplicate resident identities, failed signatures, and capacity/collector/component-source mismatches. RAM, provider, and critical observations plus CPU/swap metrics carry independent timestamps and must satisfy #265 freshness checks. A signed label alone does not establish that the named source is truthful: the host signer, its data acquisition, key custody, read-only mounts, and owner identity still require independent review.

5. **Keep operator authorization a future moment-of-run gate.** This version exposes no `run-real` mode and no flag that turns a preflight into authorization. Once trusted sources and the endpoint firewall are implemented, a separate design must define one-time operator intent, bind it to the exact SHA/model/workflow/endpoint/run ID, and combine it with actual #265 leases. No state is persisted in production.

6. **Use sanitized, stable blocker identifiers.** The report names each missing gate and avoids prompts, model output, credentials, host paths, or opaque lease tokens. Endpoint and model are exact requested target fields; they are validated syntactically but explicitly marked unverified and never contacted.

### Signed pressure thresholds and status

A trusted signature authenticates measurements, not an admission pressure policy. The existing #265 evaluator now fails closed with `trusted_pressure_threshold_missing` unless **both** `cpuUsagePercent` and `swapPressurePercent` have explicit validated `pressureThresholds` settings. No thresholds are inferred from the signature and no second admission policy is introduced. Above configured thresholds, the canonical evaluator returns `WAITING` without a lease. Signed CPU and swap samples have independent freshness exposed as `observations.cpuUsage` and `observations.swapPressure`; aggregate `ObservationFresh` requires both. This does not authorize real Ollama traffic or replace critical-service guards.

## Risks / Trade-offs

- [A signed snapshot could be mistaken for a trustworthy measurement] → The signature proves integrity and signer identity only; require external review of key custody, host collection methods, provider/critical-service inputs, capacity-domain binding, and freshness.
- [Users may confuse syntactic target validation with provider identity] → Emit separate `targetDeclaration` and `providerIdentity` statuses, with identity marked unavailable.
- [Future implementations may add a route before trust contracts are ready] → OpenSpec requires both source-attributed live observations and enforceable exact-endpoint egress; tests must keep unsupported execution rejected.
- [Existing scripts may parse `real-plan` as PASS] → Keep the JSON field names stable where practical, change the result to `BLOCKED`, and update tests/runbook to make the semantic change explicit.

## Migration Plan

No persistent data or database schema changes. The mock command and Compose project remain as-is. Optional reader configuration uses `MVFORGE_TRUSTED_OBSERVATION_PATH`, `MVFORGE_TRUSTED_OBSERVATION_PUBLIC_KEY_PATH`, `MVFORGE_TRUSTED_CAPACITY_DOMAIN`, `MVFORGE_TRUSTED_COLLECTOR_ID`, `MVFORGE_TRUSTED_HOST_SOURCE_ID`, `MVFORGE_TRUSTED_PROVIDER_SOURCE_ID`, and `MVFORGE_TRUSTED_CRITICAL_SOURCE_ID`. No production values are provisioned here. Operators using `real-plan` continue to receive `BLOCKED`; use `real-preflight` for the explicit spelling. Rollback consists of reverting this feature branch; no NAS or model state is touched.

## Open Questions

The remaining blockers are the host snapshot publisher and protected signing-key custody; verified RAM/swap/CPU, Ollama residency, and critical-service collection; exact Ollama endpoint identity and enforced endpoint-only egress; image/SHA binding; and moment-of-run authorization. These must be completed and reviewed before any run-capable design or GO result is allowed.
