# Design: minimum OCR AI Settings contract

## Current contract

The Settings registry persists JSON values by key. Translation keeps its own selected model, while #261 owns the shared Ollama connection, installed-model catalog, and per-model metadata readiness endpoint.

## Decisions

### Persist an independent feature setting

Use the additive `ocrAISettings` key with `enabled` and `model` fields. Its default is disabled with no model selected. The model remains an opaque identifier selected by the operator. Translation's model is never read to populate this value, and the same model ID remains valid for both features when selected separately.

The registry trims and bounds the model identifier, validates its type, and preserves extension fields. An enabled setting requires a non-empty model after trimming; disabled settings may keep the model empty or retain a selection. Validation is local and does not make network calls during a Settings write. Model existence and provider availability are checked through the existing #261 endpoints.

### Add a safe default to existing databases

When Settings are listed, an idempotent additive migration inserts the default OCR AI row only if it is missing. It does not alter, copy from, or delete the existing `subtitleTranslationSettings` row. There is no SQLite schema change.

### Keep provider evidence derived

Installed-model discovery and readiness checks are user-triggered. Their responses are view state only; they do not rewrite the selected model or enabled flag. Missing models and unavailable providers remain visible with the saved identifier intact. Enabling OCR AI requires an explicit user action and a successful readiness check in the card. Settings never invokes inference or modifies subtitle cues.

### Reuse the Settings draft lifecycle

The OCR AI card is controlled by the Settings page's `ordinarySettings` form and registered as the `subtitles` draft section. Its edits use the existing unsaved-change guard, tab draft recovery, and shared section save flow. Model catalog and readiness responses remain ephemeral view state and never replace the explicitly selected draft model.

## Compatibility

Existing databases gain a disabled/unselected OCR AI preference. Existing Translation configuration and shared provider settings remain unchanged. Existing unknown OCR AI extension fields survive normalization and saves.

## Validation

Focused backend tests cover normalization, invalid values, default seeding, idempotence, and preservation of existing Translation settings. Focused frontend tests cover defaults, explicit model discovery and readiness, unavailable/missing models, independent selection, draft preservation through refetch/navigation and save/refetch, and disabling without provider calls.
