## Context

The translation provider currently calls Ollama `/api/chat` with `stream:false`
and `format:"json"`, then decodes `translation-provider-v1` and runs the
canonical cue and ASS validators. JSON mode does not constrain the response to
that application-specific envelope. The user-visible failure class is generic
`malformed_response`, and operation diagnostics intentionally omit provider
payloads.

The incident report identifies model, languages, representation, phase, and
failure class, but no live response was retained. The exact cause remains
unknown. No NAS media, production database, or live Ollama model is used for
this change.

## Decisions

- Send Ollama a JSON Schema object in `format`, generated from each existing
  bounded batch. Require the canonical `contractVersion` and `cues`; bind cue
  count to the batch and restrict cue IDs to the current batch IDs. Disallow
  extra object properties. The schema may constrain the allowed ID set but does
  not claim ordering enforcement; `validateTranslationBatchResult` remains the
  authority for exact order, count, identity, and nonempty text.
- Keep the canonical decoder and validators fail-closed. Do not salvage JSON,
  guess IDs, split or merge cues, or accept partial batches. Validate protected
  ASS dialogue structure before returning the execution result, and leave cue
  timing, styles, tags, and serialization under MVForge control.
- Recognize explicit completion metadata (`done:false` or
  `done_reason:"length"`) as a truncated response. Keep response body reads
  bounded and classify an oversized envelope without retaining its contents.
- Represent diagnostic details with a fixed category allowlist, one-based batch
  index/count, optional safe cue counts, and optional HTTP status. Provider and
  model remain the existing operation metadata. Never serialize error causes,
  cue IDs, subtitle text, prompts, raw response bodies, source paths, endpoint
  credentials, or authorization headers.
- Keep timeout, cancellation, provider unavailability, source revision fencing,
  staging cleanup, retry identity, and the all-output publication barrier
  unchanged. A failure in any batch returns no translated output.
- Use synthetic ASS fixtures and a stub Ollama HTTP server. A test request using
  the string `translategemma:12b` verifies adapter behavior only; no claim of
  compatibility or translation quality for the real model is made.

## Risks / Trade-offs

JSON Schema structured output support depends on the configured Ollama server
and model implementation. Unsupported schema requests fail as provider errors;
there is no implicit retry with JSON mode or another model. Even schema-shaped
output can be semantically wrong, so identity, ordering, nonempty-text, and ASS
validation remain mandatory.

## Migration Plan

No database migration is required. The operation response gains an optional
sanitized `providerDiagnostics` object; existing generic `failureClass` and
`diagnostic` fields remain unchanged. Existing operations remain readable.
