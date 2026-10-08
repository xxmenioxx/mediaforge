## Why

Subtitle Translation currently owns the only persisted Ollama endpoint, even as OCR AI Assisted and Copilot are planned to use the same external service. A backend-owned provider connection and model catalog will give those consumers one endpoint authority while preserving Translation's saved model and enablement through migration.

## What Changes

- Add one persisted, validated Ollama connection configuration shared by backend consumers; keep feature-selected models and model roles outside that connection record.
- Add backend-only connection testing and installed-model discovery through Ollama `GET /api/tags`, with bounded requests, responses, and diagnostics.
- Define per-model readiness separately from provider reachability and installed-model discovery; do not infer model residency or inference readiness from installation or `/api/show` metadata alone.
- Migrate the legacy endpoint out of `subtitleTranslationSettings` without resetting Translation's selected model, enablement preference, or safe readiness evidence; keep a deprecated API projection for the current Settings client until #266 replaces it, and surface conflicting legacy/new connection state for explicit resolution.
- Provide a canonical backend connection and model-resolution contract for consumers, including the future model roles in #223 and Settings model selectors in #266.

Out of scope: model role and fallback policy (#223), Settings UI and feature-specific model selection (#266), Copilot/OCR workflows, model pull/download, Ollama residency/warmup/resource admission (#264/#265), and non-Ollama providers.

## Capabilities

### New Capabilities

- `ollama-provider`: shared connection configuration, server-side discovery, bounded connection and per-model readiness contracts, and safe legacy migration.

### Modified Capabilities

None. The existing subtitle Translation workflow remains the consumer; this change adds a shared provider capability without changing its subtitle operation contract.

## Impact

- Backend: shared provider configuration and resolver, settings migration, connection/model discovery and readiness APIs, bounded Ollama HTTP handling, and focused migration/API tests.
- API: authenticated MVForge endpoints for provider connection testing, model discovery, and per-model readiness; the browser will never call Ollama directly.
- Persistence: one canonical Ollama endpoint record. Translation retains its model, enablement, translation limits, and readiness state in its existing feature settings, with the legacy endpoint removed after successful migration. The existing Settings client may continue reading/writing a deprecated projected endpoint field during the #266 transition; it is never a second persisted source of truth.
- Rollout: migration is idempotent and fail-closed on conflicting connection values; conflict status is surfaced for explicit resolution without modifying unrelated settings such as Jellyfin.
