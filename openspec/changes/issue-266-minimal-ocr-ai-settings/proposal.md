# OCR AI Settings minimum contract

## Problem

Issue #267 needs an OCR AI feature setting and an independently selected Ollama model. The current Settings contract owns Translation configuration but has no OCR AI selection. Reusing Translation's model as an implicit default would couple two consumers and could enable a feature without an explicit choice.

## Outcome

Add an OCR AI Settings record with `enabled: false` and an empty model by default. Let the operator select and persist an OCR model independently through the existing shared Ollama model catalog. Display readiness checks for the selected model without changing the saved selection or invoking inference.

## Scope

- Register and validate the additive `ocrAISettings` value through the canonical Settings registry.
- Seed missing legacy databases with the disabled, unselected default while preserving all existing Translation values.
- Add a small OCR AI card beside subtitle Translation Settings.
- Reuse the shared Ollama catalog and model-readiness endpoints; do not add a connection or model store.
- Preserve explicit model selection when the provider is offline or the model is missing.
- Add focused backend and frontend regression coverage.

## Out of scope

- Full Settings redesign under #266.
- OCR correction, cue review, inference, or subtitle publication behavior owned by #267 and #378.
- Changing Translation, Jellyfin, shared-provider, or model-catalog behavior.
