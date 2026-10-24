## 1. Strict Ollama Response Contract

- [x] 1.1 Send a per-batch JSON Schema to Ollama while retaining the canonical decoder and validation.
- [x] 1.2 Reject incomplete outer envelopes and truncation metadata without retaining provider content.
- [x] 1.3 Emit allowlisted provider response categories and one-based batch diagnostics through the existing translation operation view.
- [x] 1.4 Validate protected ASS structure per batch before any execution output is returned.

## 2. Regressions

- [x] 2.1 Cover stubbed Ollama schema request, synthetic EN to es-419 ASS success, and protected structure preservation.
- [x] 2.2 Cover HTTP/envelope, empty content, invalid/truncated inner JSON, wrong contract version, cue count/identity/order/text, and oversized/truncated responses.
- [x] 2.3 Cover later-batch failure, cancellation, revision fencing, and atomic publication without partial output or source mutation.
- [x] 2.4 Assert diagnostics omit cue IDs/text, raw response, source path, and credentials.

## 3. Validation and Evidence

- [x] 3.1 Run focused translation backend and lifecycle tests using synthetic fixtures/stubs only.
- [x] 3.2 Run OpenSpec strict validation and `git diff --check`.
- [x] 3.3 Run the repository verifier under `shared-validation`.
- [x] 3.4 Record that no live TranslateGemma 12B compatibility or quality claim was tested.
