## Why

Structural validation protects subtitle artifacts from malformed translation output, but it does not identify suspicious omissions, meaning signals, terminology drift, or target usability concerns. Asset Info needs bounded, deterministic cues for human review that remain separate from OCR confidence.

## What Changes

- Analyze translated cues after the existing structural validator succeeds and before the canonical serializer/publication path.
- Return bounded, advisory quality findings and cue summaries through the existing translation operation response.
- Expose a focused review view in Asset Info with source and translated cue text, reasons, and a keep/edit workflow that preserves canonical cue identity and structure.
- Persist bounded quality provenance and review decisions against the canonical translation operation so review state can be reloaded.
- Allow an optional reviewer model distinct from the translator; suggestions remain advisory and use the same cue identity contract.
- Keep quality analysis optional/advisory; do not change translation eligibility or publish partial/invalid output.
- Record OCR uncertainty separately as source evidence and never treat translation as correcting uncertain OCR.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: add observable translation-quality findings and human review behavior after structural validation.

## Impact

The change affects translation-specific backend analysis, persisted review data and endpoints, frontend settings/API/Asset Info review UI and tests, and a delta spec. It reuses the current Ollama transport, cue validator, serializer, revision checks, staging, and atomic Publisher. One translation-linked review model is added; no new provider runtime or publication pipeline is introduced. The optional reviewer model is separate from the translator and compares only bounded deterministic flagged cue pairs.
