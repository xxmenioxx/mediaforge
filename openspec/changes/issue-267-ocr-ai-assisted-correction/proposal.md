# OCR AI-assisted correction suggestions

## Problem

OCR review can identify suspicious cues and apply deterministic dictionary corrections, but it cannot offer bounded model-assisted suggestions for remaining OCR errors. Suggestions must preserve canonical cue identity and remain subject to human review.

## Outcome

Add optional Ollama suggestions to the existing canonical OCR review report. OCR AI uses only the independently configured `ocrAISettings` model, runs only for bounded suspicious cues after deterministic analysis, and never edits or publishes subtitles automatically.

## Scope

- Add an OCR-owned adapter that reuses the existing Ollama proofreader request and response validation.
- Require the shared resource admission gate and the explicitly selected model to be available before inference.
- Fence each request and returned suggestion to the exact canonical OCR review target, cue ID, source hash, and baseline revision.
- Show Original, deterministic Dictionary suggestion, and AI Suggestion in Review OCR with explicit human-only actions.
- Keep unavailable, invalid, stale, cancelled, and provider-failure outcomes visible without blocking canonical OCR review or Translation.
- Add synthetic/mock regressions, including Translation retry with existing canonical OCR approvals.

## Out of scope

- Automatic correction, approval, translation, or publication of model output.
- Changes to Translation provider/validation, Translation settings, shared Ollama connectors, or shared resource admission.
- Real Ollama inference or claims about model accuracy.
