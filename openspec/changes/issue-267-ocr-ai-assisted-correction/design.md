# Design: OCR AI-assisted correction suggestions

## Existing authorities

Canonical OCR review owns cue identity, the selected OCR source, published artifact revision, and baseline revision. Deterministic dictionary correction and quality analysis remain the first analysis stages. Existing OCR approval and safe publication remain the only route for accepted corrections. The shared Ollama proofreader owns prompt construction and strict response decoding; the shared AI lifecycle owns resource admission.

## Decisions

### Use only the explicit OCR model

Read `ocrAISettings.enabled` and `ocrAISettings.model` for this feature. Disabled, absent, or invalid settings produce a visible report state and no inference. Never read Translation settings or choose a default model. Reuse the existing shared Ollama endpoint configuration and proofreader implementation, passing the selected OCR model directly.

### Require resource admission before inference

Do not send a chat request unless the shared AI lifecycle and inference admission gate are available. The selected model must be resident and available under the existing shared model lifecycle. Resource admission, missing model, offline provider, cancellation, and malformed/provider errors are reported as non-fatal OCR AI states. They do not remove deterministic findings, mutate canonical cues, or block OCR review and Translation.

### Limit and fence input

Consider only suspicious cues that remain after deterministic analysis. Bound cue text, neighboring context, cues per batch, and total cues. Include only nearby text context needed to interpret a suspicious cue. Before and after each batch, verify that the exact canonical source, selected stream/language/mode, artifact identity/revision, baseline revision, and OCR settings still match the captured target.

### Validate suggestions atomically

Use the existing strict proofreader decoder. Require the expected cue IDs, count, order, source hashes, selected model identity, and bounded suggestion text. Do not expose any suggestions unless all batches complete against the same target; discard the accumulated suggestions if a response is malformed, cancelled, or the source/configuration becomes stale. Suggestions are transient report data and do not change SRT/ASS sidecars, timings, positions, styles, tags, provenance, approvals, or publication state.

### Keep decisions human-only

Review OCR displays Original, Dictionary suggestion, and AI Suggestion separately. Accepting an AI suggestion only stages text as a human `Correct` decision. The operator must inspect the decision preview and confirm through the existing canonical approval/publication flow. Correct, Reject, and Leave Pending retain their existing meanings. No suggestion is applied automatically; source or baseline changes invalidate it.

## Validation

Mocked provider tests prove disabled/no-model and unavailable-admission paths make no inference request; exact model, target, cue hash, revision, ordering, bounds, and stale invalidation are enforced; malformed responses do not partially publish suggestions; and the canonical Translation retry recognizes existing OCR approvals without modifying the Translation engine. Frontend tests prove all three text versions are distinguishable and AI actions only stage or dismiss an unpersisted suggestion. No real Ollama or media is used.
