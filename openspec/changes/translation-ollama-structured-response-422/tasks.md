## 1. Strict Ollama Response Contract

- [x] 1.1 Send a per-batch JSON Schema to Ollama while retaining the canonical decoder and validation.
- [x] 1.2 Reject incomplete outer envelopes and truncation metadata without retaining provider content.
- [x] 1.3 Emit allowlisted provider response categories and one-based batch diagnostics through the existing translation operation view.
- [x] 1.4 Validate protected ASS structure per batch before any execution output is returned.
- [x] 1.5 Bound each Ollama chat request to five minutes or the earlier caller operation deadline.

## 2. Regressions

- [x] 2.1 Cover stubbed Ollama schema request, synthetic EN to es-419 ASS success, and protected structure preservation.
- [x] 2.2 Cover HTTP/envelope, empty content, invalid/truncated inner JSON, wrong contract version, cue count/identity/order/text, and oversized/truncated responses.
- [x] 2.3 Cover later-batch failure, cancellation, revision fencing, and atomic publication without partial output or source mutation.
- [x] 2.4 Assert diagnostics omit cue IDs/text, raw response, source path, and credentials.
- [x] 2.5 Exercise synthetic 4/8/16/32-cue Ollama batches through a stub and benchmark the adapter without live Ollama.
- [x] 2.6 Assert the request deadline honors both its five-minute cap and an earlier operation deadline.

## 3. Validation and Evidence

- [x] 3.1 Run focused translation backend and lifecycle tests using synthetic fixtures/stubs only.
- [x] 3.2 Run OpenSpec strict validation and `git diff --check`.
- [x] 3.3 Run the repository verifier under `shared-validation`.
- [x] 3.4 Record that no live TranslateGemma 12B compatibility or quality claim was tested.
- [x] 3.5 Record the measured synthetic 4/8/16/32-cue adapter benchmark results.

Synthetic stub benchmark, 100 iterations per size (`go test ./internal/handlers -run '^$' -bench '^BenchmarkOllamaTranslationProviderSyntheticBatchSizes$' -benchmem -benchtime=100x`, darwin/amd64, Intel Core i7-9750H @ 2.60GHz):

- 4 cues: 235,882 ns/op, 25,425 B/op, 223 allocs/op.
- 8 cues: 259,809 ns/op, 32,799 B/op, 235 allocs/op.
- 16 cues: 310,614 ns/op, 39,420 B/op, 252 allocs/op.
- 32 cues: 409,738 ns/op, 61,551 B/op, 293 allocs/op.

These are local adapter and loopback HTTP stub measurements; they do not
measure Ollama inference or qualify a model.
