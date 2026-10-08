## Why

Ollama models can be installed and configured while still unloaded, so the first AI request can stall or fail and Queue work can hold scarce FFmpeg capacity while waiting. MVForge needs one bounded, cancellable lifecycle seam that warms the exact selected model on demand, reports actual residency, and lets waiting work resume only when its source, model choice, and cancellation state remain valid.

## What Changes

- Define model residency states `UNLOADED`, `WAITING_FOR_RESOURCES`, `WARMING_UP`, `READY`, `IN_USE`, `IDLE`, and `FAILED`, separate from feature selection and Ollama installation/readiness.
- Warm models on demand through the #261 shared Ollama provider using `/api/generate` with `keep_alive`; discover residency from `/api/ps`. Use a 30-minute keep-alive default and propagate it on inference requests.
- Coalesce same-provider/model warmups, bound readiness waits to five minutes by default, propagate cancellation, and stop automatic retries at a defined bound.
- Persist Queue `WAITING_FOR_MODEL` through the existing execution-plan waiting state without holding an FFmpeg reservation or blocking other runnable jobs. Resume only after source revision, frozen model selection, and cancellation checks pass.
- Let explicit Translation operations report waiting/warming progress through their existing operation lifecycle and revalidate the captured source revision and model before provider work resumes.
- Expose a bounded runtime projection that distinguishes installed, loaded, and MVForge-in-use models; `/api/ps` is the residency source and MVForge operation tracking is the busy source.

Out of scope: Ollama endpoint/configuration/catalog ownership (#261); resource admission policy and capacity decisions (#265); model-role selection or fallback (#223); Settings UI and feature model selectors (#266); OCR/Copilot workflow implementation; automatic model downloads, forced/unsafe eviction, and provider restarts.

## Capabilities

### New Capabilities

- `ai-model-lifecycle`: on-demand warmup, residency reporting, bounded model waits, and safe workflow resumption.

### Modified Capabilities

- `subtitles`: explicit Translation operations may wait for model warmup and must revalidate their captured source and selected model before resuming.

## Impact

- Backend: the existing #261 Ollama provider surface, Queue execution-plan waiting/recovery, runtime reporting, and the existing subtitle operation lifecycle.
- API: bounded model residency/runtime status for the #266 Settings surface; no browser-to-Ollama calls.
- Persistence: reuse `ExecutionPlan.WaitingState` for durable Queue waits and frozen execution identity; do not create a second model catalog or model-state database.
- Dependencies: #261 shared Ollama provider is required; #265 owns admission decisions; #223 owns roles/fallback; #266 owns Settings presentation and per-feature selection. OCR/Copilot adapters consume the same lifecycle seam when their feature owners integrate it.
