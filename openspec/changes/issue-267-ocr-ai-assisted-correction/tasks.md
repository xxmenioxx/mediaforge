## 1. OCR AI adapter and report

- [x] 1.1 Read only explicit `ocrAISettings`; reuse the shared Ollama proofreader with that model.
- [x] 1.2 Use shared `EnsureReady` and bounded model-load admission for an installed nonresident OCR model, then require inference admission and confirmed residency.
- [x] 1.3 Bound suspicious cue batches and context; fence requests and responses to the exact review target and revisions.
- [x] 1.4 Keep provider, resource, cancellation, malformed, stale, and configuration-change outcomes visible and non-fatal to canonical OCR review.

## 2. Human-only OCR review

- [x] 2.1 Extend OCR report types and show Original, Dictionary, and AI suggestion distinctly.
- [x] 2.2 Keep Accept, Correct, Reject, and Leave Pending within the existing human review/confirmation flow; invalidate stale suggestions.

## 3. Regression coverage

- [x] 3.1 Test explicit model selection, resident and cold-start paths, denied admission, warmup failure, cancellation, source/settings changes during warmup, malformed response, and absence of inference or partial suggestions after failures.
- [x] 3.4 Verify OCR AI model-load/inference ConsumerID attribution and preserve Translation ConsumerID.
- [x] 3.2 Test suggestion cue ID/hash/revision/order/bounds and absence of partial results.
- [x] 3.3 Test human-only UI actions and Translation retry with current canonical OCR approvals.

## 4. Validation

- [x] 4.1 Run focused backend and frontend tests under `shared-validation`.
- [x] 4.2 Validate this OpenSpec change in strict mode.
- [x] 4.3 Run `git diff --check`; leave the shared verifier to the orchestrator.
