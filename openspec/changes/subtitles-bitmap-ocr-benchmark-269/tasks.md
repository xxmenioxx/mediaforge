## 1. Baseline and Capabilities

- [x] 1.1 Inspect the canonical SeConv invocation, accurate-mode color pass, and existing candidate selection; verify with Serena symbol inspection and installed SeConv/Tesseract help.
- [x] 1.2 Record verified versus unknown color, alpha, threshold, and segmentation behavior in design.md; verify the issue's expected SeConv version against the local version output.

## 2. Synthetic Benchmark

- [x] 2.1 Add a deterministic standard-library harness with embedded human-readable fixture ground truth and CER/WER, latency, CPU, memory, and output-line measurements; verify syntax and run it with three repeats in an isolated work directory.
- [x] 2.2 Record observed measurements separately from estimates and document unmeasured correction, media, and subtitle integrity dimensions in design.md.

## 3. Adoption Decision

- [x] 3.1 Decide whether results justify production preprocessing; keep the canonical implementation unchanged when representative bitmap/runtime evidence is absent, and verify the decision is explicit in design.md.

## 4. Validation

- [x] 4.1 Run focused benchmark validation and `./scripts/verify.sh --auto`; record exact outcomes and any limitations.
- [x] 4.2 Keep peak RSS run-wide rather than candidate-specific and enforce report shape in the harness; verify each row excludes RSS and the report contains exactly one aggregate value.

## 5. Representative-fixture qualification (pending)

- [ ] 5.1 Obtain authorized, disposable PGS/VobSub cue fixtures with human-verified confusable glyph and shadow/outline ground truth.
- [ ] 5.2 Measure the unchanged SeConv baseline and supported alternatives on the target runtime, including accuracy, correction/review errors, cue-level resources, and SRT/ASS integrity before reconsidering production preprocessing.
