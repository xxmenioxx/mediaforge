## Why

Ollama can have the selected model installed but unloaded, so an explicit Translation request can stall or fail on its first inference. MVForge needs one bounded model-lifecycle seam that observes real Ollama residency, warms only on demand, and lets a waiting operation resume only when its source, selected model, and cancellation state still match. Any model load or inference must pass through #265 admission. Until #256 supplies verified NAS observations and real resource calibration, the protected Translation path remains disabled by default and fails closed when its admission evidence is unavailable.

## What Changes

- Define lifecycle states UNLOADED, WAITING_FOR_RESOURCES, WARMING_UP, READY, IN_USE, IDLE, and FAILED, separate from installed-model discovery, provider connectivity, and feature selection.
- Extend the shared #261 provider owner with bounded /api/ps discovery and no-prompt /api/generate warmup using keep_alive. Use a 30-minute default and carry it on inference requests.
- Consume the merged, acyclic `backend/internal/airesource` service owned by #265 before either warmup or inference. If admission is unavailable, expose WAITING_FOR_RESOURCES with an actionable reason and issue no model-affecting Ollama request.
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
- Dependencies: #261 is merged and remains provider owner. #265 is merged and supplies the admission service consumed by #264. Production activation remains blocked on #256 telemetry/critical-service observations and actual NAS model calibration. #223 supplies the exact role-selected model; #266 consumes the runtime API and owns UI/selection; Queue, OCR, and Copilot integration waits for their owning feature work.

## Rollout Gate and Deferred Integration

Production lifecycle activation is controlled by `MVFORGE_AI_MODEL_LIFECYCLE_ENABLED`, which defaults to false. Enabling it never bypasses #265: a missing capacity domain, current host/provider telemetry, measured model estimate, or #256 critical-service guard results in a visible wait or denial and no model-affecting Ollama request. The route shares one admission service between Settings and Translation.

Before enabling this path on the NAS, #256 observation and critical-service sources must be integrated and freshness verified, and the selected model estimates must be calibrated from actual NAS measurements. These are external rollout prerequisites, not fabricated test fixtures or #264 resource-policy tasks. Queue has no current Ollama stage; its reusable wait contract remains deferred until a feature owner adds a real Queue AI consumer.
