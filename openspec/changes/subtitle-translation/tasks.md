## 1. Settings and provider contract

- [x] 1.1 Add the persisted subtitle-translation settings value through the existing `AppSetting` API for provider, endpoint, model, and bounded request/context policy; verify missing/legacy values are safe, no localhost endpoint is inferred, and secrets are not returned in UI/API settings responses.
- [x] 1.2 Define the text-only `TranslationProvider` boundary, versioned request/response contract, provider error classification, and cancellation contract; verify contract tests cover source/target language, locale, cue IDs, bounded payloads, and context omission.

## 2. Canonical cue envelope and protected structure

- [x] 2.1 Adapt the existing canonical subtitle cue/parser path into an operation-local translation envelope with stable cue IDs, source ordinals, text, timing, representation, and geometry references; verify SRT/ASS/SSA inputs produce deterministic IDs without changing existing serializers.
- [ ] 2.2 Implement ASS/SSA protected-structure extraction and reconstruction so only eligible dialogue text is translatable; verify styles, override tags, margins, positioning, karaoke/drawing tokens, line breaks, timing, order, and semantic metadata survive round-trip tests.
- [ ] 2.3 Define source-format-preserving text output and canonical bitmap output selection without adding a planner; verify SRT remains text/timing-only and ASS/SSA output retains existing structure and geometry behavior.

## 3. Validation, identity, revision, and audit

- [ ] 3.1 Implement strict cue-ID response validation for missing, duplicate, unexpected, empty-invalid, reordered, merged, split, timed, or structurally mutated provider results; verify malformed responses fail before serialization or publication.
- [ ] 3.2 Add versioned translation identity and locale-qualified artifact/naming metadata, including source artifact/stream identity, source revision, language, locale, provider/model contract, context contract, and representation; verify `es-419` and `es-ES` never collide and exact existing artifacts can be satisfied.
- [ ] 3.3 Capture and recheck bounded embedded/external source revisions at the final publication boundary; verify same-path replacement or external edits fail closed, preserve source/prior translated artifacts, and publish nothing new.
- [ ] 3.4 Persist bounded translation provenance and metrics with secret/prompt/response redaction; verify artifact metadata and Asset Info responses contain no credentials, headers, full prompts, or full provider responses and remain backward-compatible for existing artifacts.

## 4. Batching and advisory context

- [ ] 4.1 Implement bounded cue batching by cue count and encoded request size with stable global cue identity/order; verify large subtitle inputs use multiple bounded requests rather than one request per cue.
- [ ] 4.2 Add bounded neighboring-cue and optional media metadata context handling for title/year, movie/series, season/episode, synopsis, and known characters; verify missing context does not block translation and context cannot alter structure or source-language decisions.
- [ ] 4.3 Add reliable Auto English/Japanese evidence resolution using bounded existing metadata/cue evidence and explicit-language override behavior; verify ambiguity and unsupported evidence fail before provider work.

## 5. Ollama provider

- [ ] 5.1 Implement the configurable Ollama HTTP adapter for NAS/LAN/container endpoints, model selection, structured JSON batches, bounded bodies/timeouts, and CPU-only operation; verify remote endpoint/model requests and actionable missing/unreachable configuration failures.
- [ ] 5.2 Propagate the existing subtitle operation context through Ollama requests and classify cancellation, timeout, HTTP, and malformed JSON failures; verify cancellation terminates provider work without producing a publishable result.
- [ ] 5.3 Add adapter seams for future OpenAI/DeepL/Google/Azure providers without implementing them; verify unknown/unimplemented provider values fail closed and Ollama remains the only V1 implementation.

## 6. Bitmap OCR integration

- [ ] 6.1 Route PGS/VobSub translation through the existing canonical OCR lifecycle and correction boundary; verify OCR, Japanese orientation fallback, accurate selection, and OCR quality correction execute before provider translation and never through a translation-specific OCR path.
- [ ] 6.2 Preserve canonical bitmap geometry/positioning and serializer fan-out after translated cue text is merged; verify SRT remains geometry-free and ASS retains geometry while both use the translated canonical cues where requested.
- [ ] 6.3 Verify OCR failure, zero-recognition, unavailable selective input, and cancellation prevent provider invocation and publication while preserving existing OCR operation semantics.

## 7. Existing operation lifecycle and publication

- [ ] 7.1 Add translation as an operation kind/request in the existing Asset Info subtitle operation model with phases, processed/total progress, cancellation, bounded cleanup, terminal `cancelled`/failure states, and new-identity retry; verify polling/progress behavior and retry semantics.
- [ ] 7.2 Reuse existing staging, validation, source-revision checks, and atomic all-output publication barrier; verify failure or cancellation during any batch/serializer member publishes no new partial output and preserves existing source/translated artifacts.
- [ ] 7.3 Verify operation-owned workspace cleanup, completion-vs-cancellation publication race behavior, and no media-adjacent temporary/raw artifacts through focused lifecycle tests.

## 8. Asset Info backend

- [ ] 8.1 Add exact embedded-stream and external-artifact translation request/response APIs using existing ownership, semantic role/disposition, and artifact identity metadata; verify same-language external sidecars are never guessed and unsupported sources are rejected clearly.
- [ ] 8.2 Wire English/Japanese, Auto, `es-419`/`es-ES`, and Ollama configuration into the canonical translation operation without changing Queue/Profile/Worker behavior; verify text SRT/ASS/SSA and bitmap PGS/VobSub entry-point scenarios.
- [ ] 8.3 Expose bounded translation progress, provider diagnostics, cancellation, retry, stale-source, and malformed-response states through the existing operation response shape; verify secrets and full provider content are excluded.

## 9. Asset Info frontend

- [ ] 9.1 Add `Translate...` controls to the actual eligible Asset Info embedded/external subtitle entries with source-language (`Auto`/English/Japanese), target-locale (`es-419`/`es-ES`), provider, and conditional model presentation; display backend-reported media-context status/source informationally, keep missing context non-blocking, and render output-format selection only when the backend reports multiple canonical valid formats. Verify generic external subtitles and unsupported languages do not show an enabled action and the frontend does not derive eligibility.
- [ ] 9.2 Add translation progress, cancellation, terminal failure/cancelled distinction, retry, and translated artifact status using existing subtitle polling patterns; verify polling stops cleanly and source/existing artifact state remains visible.
- [ ] 9.3 Add focused frontend coverage for source/target/provider mapping, conditional model visibility, available and missing media-context status/source, Auto ambiguity messaging, locale-distinct artifact display, backend-authorized multiple-format selection, single-format no-selector behavior, and redacted diagnostics without adding Queue/Profile translation controls or independent output-format eligibility.

## 10. Regression and final validation

- [ ] 10.1 Add backend regressions for English/Japanese SRT/ASS/SSA translation, Auto ambiguity, `es-419` versus `es-ES`, cue-ID validation, ASS protection, context bounds, Ollama requests, and audit redaction.
- [ ] 10.2 Add backend regressions for PGS/VobSub OCR reuse, Japanese fallback, OCR correction before translation, stale source rejection, source preservation, cancellation/retry, cleanup, and atomic publication.
- [ ] 10.3 Add integration regressions proving no Queue/Profile automatic translation, no second planner/OCR/operation/publication path, existing semantic role/disposition and naming preservation, and exact artifact ownership.
- [ ] 10.4 Run focused backend/frontend tests, then `./scripts/verify.sh --auto`, followed by `openspec validate subtitle-translation --type change --strict`; reconcile this checklist only against passing implementation evidence and leave the change unarchived.
