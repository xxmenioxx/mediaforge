## Why

Ollama connection ownership and model discovery already exist, but Settings does not present the provider and its feature choices as one coherent configuration surface. Translation, OCR AI, and Copilot need independent model selections so operators can share an installed model deliberately without implicit fallback or accidental inference.

## What Changes

- Present one Ollama endpoint, backend connection test, installed-model catalog, and runtime residency/readiness status in Providers & Integrations.
- Organize Subtitles settings around independent Translation and OCR AI enabled/model choices while preserving existing subtitle compatibility, styling, and OCR Dictionary access.
- Add an independently saved Copilot enabled/model/preference setting with a disabled default.
- Preserve provider and feature drafts across navigation/refetch and keep connection tests distinct from persistent saves.
- Preserve existing Translation settings and allow the same installed model to be selected by multiple features.

## Capabilities

### New Capabilities

- `ollama-feature-settings`: shared provider presentation and independent feature-specific model/settings controls.

### Modified Capabilities

None. The canonical provider and subtitle capabilities remain owned by their existing changes; this capability defines the integrated Settings behavior.

## Impact

- Frontend Settings domains, navigation, draft handling, and API presentation.
- Additive backend Settings registry validation/defaulting for Copilot configuration.
- Existing Ollama provider, model catalog, readiness, Translation, OCR AI, OCR Dictionary, and Jellyfin settings contracts are reused.
- No new dependencies, database schema migration, model download, warmup, or inference.
