## Why

The approved #264 lifecycle needs a positive admission decision before every model warmup and inference, but MVForge has no shared policy authority for protecting a resource-constrained NAS. A provider-neutral admission service lets MVForge enforce conservative 24 GiB defaults while using measured capacity and operator-configured limits for future hardware upgrades.

## What Changes

- Add one backend-owned resource admission contract for model loads and inferences, with `ADMITTED`, `WAITING`, `DENIED`, and `UNAVAILABLE` outcomes, cancellation, fair nonblocking waits, and opaque leases with explicit idempotent release.
- Default to one resident model and one concurrent MVForge inference. Keep those limits independently configurable; do not raise them automatically when host RAM changes. Keep model-load concurrency separately gated.
- Evaluate fresh host capacity, provider-reported resident models, conservative model and inference memory estimates, and the critical-service pressure authority. Default to a configurable 6 GiB available-memory reserve as an initial value pending NAS measurement.
- Persist admission leases and wait identities so recovery does not duplicate grants or forget in-flight resource reservations. Expose bounded dry-run capacity, reason codes, and counters.
- Add Resource Protection settings and diagnostics that keep MVForge limits distinct from Ollama's actual `OLLAMA_MAX_LOADED_MODELS` and `OLLAMA_NUM_PARALLEL` settings. Unknown server values remain unknown; report restart requirements only when current and requested server configuration can be compared.
- Make the #264 lifecycle consume this port for both warmup and every inference. #265 does not import lifecycle types or own lifecycle state.

No model is forcibly evicted, Ollama is never restarted by MVForge, and resource waits do not retain FFmpeg workers or use uncontrolled polling. This change does not add model roles, provider transport, model downloads, or Queue/OCR/Copilot workflows.

## Capabilities

### New Capabilities

- `ai-resource-admission`: provider-neutral capacity policy, decisions, fair waits, leases, settings, and recovery contracts for MVForge AI operations.

### Modified Capabilities

None. The approved `ai-model-lifecycle` change consumes this capability through its defined admission port; #265 does not revise lifecycle ownership or behavior.

## Impact

- Backend: add the admission service, policy/settings persistence, lease recovery, and an adapter boundary for canonical host and provider telemetry.
- Runtime: consume the existing `RuntimeState` host measurements without adding model residency to `RuntimeState` or `RuntimeSnapshot`; keep the scheduler's Queue resource decisions separate.
- Integration: #261 remains the Ollama endpoint/transport owner; #264 remains the model lifecycle owner and supplies exact model/workflow identity and purpose. The critical-service priority authority remains with #256.
- Settings: expose MVForge policy and observed Ollama server configuration separately. No Docker socket, container control, automatic restart, or server-setting mutation is added.
- Validation: add policy and restart-recovery tests with realistic 24 GiB pressure fixtures and a configured 64 GiB/two-resident case. The 6 GiB reserve is provisional until checked against #256 NAS measurements.
