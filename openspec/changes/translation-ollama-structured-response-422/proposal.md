## Why

Ollama JSON mode asks for syntactically valid JSON but does not express the
translation-provider envelope or the exact cue set for the active batch. A
malformed-response incident was reported for an ASS translation using
`translategemma:12b`; its precise live-model failure boundary has not been
established. MVForge needs a reproducible strict request contract and bounded
failure diagnostics without weakening canonical validation.

## What Changes

- Supply Ollama `/api/chat` with a JSON Schema derived from the current bounded
  translation batch: canonical contract version, exact cue count, and IDs
  selected for that batch.
- Keep the existing decoder, exact cue ID/count/order checks, ASS protected
  structure checks, revision fence, cancellation, and atomic publication as
  authoritative.
- Bound each Ollama chat request to five minutes or the earlier enclosing
  operation deadline, without extending the operation deadline.
- Add allowlisted provider response diagnostic categories and one-based batch
  context with safe expected/actual cue counts where available.
- Exercise synthetic 4, 8, 16, and 32-cue batches against an HTTP stub and record
  local adapter benchmark results without contacting Ollama.
- Reject incomplete, malformed, reordered, invented, or structurally unsafe
  output without exposing provider bodies or subtitle content and without
  producing partial output.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: define the strict Ollama response request contract and sanitized
  batch diagnostics while preserving translation lifecycle guarantees.

## Impact

Changes are limited to the existing Ollama translation request, canonical
response validation, translation operation diagnostics, focused backend tests,
and this OpenSpec change. No model fallback, new provider, OCR behavior,
frontend review flow, database schema, or publication path is added. Synthetic
tests use an HTTP stub; naming `translategemma:12b` in a test does not qualify
that live model.
