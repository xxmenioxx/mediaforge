## Why

Ollama can have the selected model installed but unloaded, so an explicit Translation request can stall or fail on its first inference. MVForge needs one bounded model-lifecycle seam that observes real Ollama residency, warms only on demand, and lets a waiting operation resume only when its source, selected model, and cancellation state still match. Any model load or inference must pass through NAS resource admission; without #265, lifecycle work must fail closed.

## What Changes

- Define lifecycle states UNLOADED, WAITING_FOR_RESOURCES, WARMING_UP, READY, IN_USE, IDLE, and FAILED, separate from installed-model discovery, provider connectivity, and feature selection.
- Extend the shared #261 provider owner with bounded /api/ps discovery and no-prompt /api/generate warmup using keep_alive. Use a 30-minute default and carry it on inference requests.
- Require an acyclic resource-admission port owned by #265 before either warmup or inference. If the port is absent or unavailable, expose WAITING_FOR_RESOURCES with an admission-unavailable reason and issue no model-affecting Ollama request.
- Coalesce warmup for the same provider/model, support independent waiter cancellation, reconcile residency after cancellation, and bound readiness to five minutes after admission by default.
- Make explicit Translation the only current workflow integration in V1. Persist its existing operation identity and a bounded pre-inference wait checkpoint in SQLite; reconstruct it after restart only after revalidating source identity/revision, selected model/configuration, and cancellation.
- Define a reusable future Queue WAITING_FOR_MODEL contract using existing execution-plan waiting state, without adding a Queue AI stage or implementing Queue AI integration in V1.

Out of scope: provider configuration/catalog ownership (#261); NAS resource policy (#265); model roles/fallback selection (#223); Settings UI and feature selection (#266); Queue AI-stage creation/integration until a Queue feature owner introduces an AI consumer; OCR/Copilot workflow integration (#224/#263); automatic model downloads, forced eviction, or provider restarts.

## Capabilities

### New Capabilities

- ai-model-lifecycle: demand-driven model residency, admission-gated warmup, bounded waits, and safe recovery contracts.

### Modified Capabilities

- subtitles: explicit Translation can wait for model readiness and durably recover only from a pre-inference checkpoint.

## Impact

- Backend: extend the #261 provider adapter; add lifecycle coordination and runtime projection; persist recovery metadata in the existing explicit Translation operation lifecycle.
- Queue: document the future contract only. Current Queue stages do not require Ollama, so no Queue code, stage, or consumer is introduced by this change.
- Persistence: add a Translation-specific SQLite checkpoint for the existing operation owner. Do not create a generic AI task store or persist model residency as authoritative state.
- Dependencies: #261 is merged and remains provider owner. #265 must supply the admission port before warmup or inference can be enabled. #223 supplies the exact role-selected model; #266 consumes the runtime API and owns UI/selection; Queue, OCR, and Copilot integration waits for their owning feature work.

