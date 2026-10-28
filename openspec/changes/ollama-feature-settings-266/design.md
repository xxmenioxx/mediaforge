## Context

The shared provider owns one saved Ollama endpoint and backend-mediated connection, catalog, readiness, and runtime observations. Translation settings predate the provider surface; OCR AI already has an independent Settings value and catalog/readiness UI. Settings domains provide URL-addressable navigation, scoped saves, and draft preservation.

## Goals / Non-Goals

**Goals:**

- Make the existing provider contract visible once and reuse it for three independent feature choices.
- Preserve existing Translation, OCR AI, OCR Dictionary, subtitle styling/compatibility, and Jellyfin behavior.
- Add Copilot configuration with safe defaults without implementing Copilot inference.

**Non-Goals:**

- Provider roles or automatic fallback policy owned by #223.
- Copilot execution owned by #224, OCR correction owned by #267, or Translation engine changes.
- Model download, warmup, runtime lifecycle policy, or resource-admission behavior owned by #264/#265.
- Direct browser-to-Ollama networking or changes to Jellyfin configuration.

## Decisions

### Reuse the provider as the only connection and catalog authority

The Providers & Integrations domain uses the existing backend APIs for connection testing, installed-model discovery, model readiness, and runtime status. The operator saves one endpoint. Catalog and connection actions stay ephemeral and do not alter feature selections. This avoids a second endpoint owner and keeps provider secrets and network access server-side.

### Persist feature settings independently

Translation keeps its existing setting and migration path. OCR AI keeps `ocrAISettings`. Copilot adds a small registry value with its own enabled/model/preference fields, defaulting to disabled, empty model, and Balanced. The same model identifier can appear in all selectors because the selections are separate values. Missing or offline selections are displayed as unavailable and are not rewritten.

### Preserve domain drafts and save boundaries

Subtitles owns its compatibility, styling, Translation, OCR AI, and Dictionary presentation. Providers & Integrations owns the provider endpoint and connection/catalog actions. AI Copilot owns Copilot settings. The existing section-draft mechanism remains the save authority; provider observations must not replace a dirty draft, and saving one section must not write unrelated settings.

### Keep runtime status observational

Residency and readiness are displayed from existing runtime APIs. Settings writes do not trigger model loading or inference. Copilot remains disabled by default, and selecting a model does not enable Copilot or OCR AI.

### Add defaults without a database schema migration

Copilot configuration is an additive Settings registry value with idempotent default insertion. Existing Translation and OCR AI rows are preserved. No SQL schema, provider endpoint, model catalog, or Jellyfin migration is introduced.

## Risks / Trade-offs

- **A model can be installed but not resident or ready** → show these states separately and preserve the selected model.
- **Provider refresh can race with unsaved edits** → keep catalog/runtime observations outside saved Settings drafts and test navigation/refetch behavior.
- **Feature settings can be mistaken for permission to run inference** → require explicit enablement per feature and keep inference outside this change.
- **Legacy Translation values may have different shapes** → retain the current migration/normalization contract and test save/reload compatibility.
