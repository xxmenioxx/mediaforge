## 1. Canonical spatial metadata and source geometry

- [x] 1.1 Define the internal canonical bitmap cue and optional geometry model, including validated canvas/bounds, timing, source ordinal, and bounded parser limits; verify malformed and out-of-bounds geometry is rejected without failing OCR.
- [x] 1.2 Implement PGS SUP geometry extraction from the existing operation-owned FFmpeg output using presentation-composition/object-definition data; verify canvas, event timing, bitmap bounds, and stable source ordering with disposable SUP fixtures.
- [x] 1.3 Fail closed to missing-geometry fallback for the current VobSub PS/PES representation because no bounded, structure-validated PS/PES-to-SPU extractor is available; verify the representative wrapped-stream unavailable path without adding an unverified FFmpeg/SeConv contract.
- [x] 1.4 Define provenance-based timing normalization: use prepared-input-local values directly, translate original-source-absolute values exactly once, and fail closed for unknown or ambiguous provenance; verify that a non-zero segment/sample start is not mechanically applied a second time.

## 2. Cue correlation and coarse layout

- [x] 2.1 Correlate recognized OCR cues with source bitmap events by normalized timing and deterministic source order, retaining one-to-one cue identity and leaving ambiguous matches unpositioned; verify simultaneous cues in different regions remain separate.
- [x] 2.2 Implement the coarse ASS alignment resolver using relative vertical thirds and reliable horizontal thirds mapped to ASS alignment tags, with deterministic bottom-center fallback and no exact position claims; verify upper, middle, lower, left, center, right, boundary, and missing-geometry cases.
- [x] 2.3 Keep geometry independent from SRT serialization and verify SRT contains only recognized text/timing while generated ASS receives the expected event-level alignment.

## 3. Canonical pipeline integration

- [x] 3.1 Thread the enriched bitmap cue result through direct OCR-generated ASS and cleanup without making layout correctness depend on serialized ASS tags surviving SeConv; verify raw ASS renders from pre-clean cues plus geometry, cleaned ASS renders from cleaned cues plus the same geometry, and OCR language, timing, line breaks, and artifact identity remain unchanged.
- [x] 3.2 Integrate geometry-aware ASS and geometry-free SRT fan-out into the existing canonical resolved-artifact path without introducing a second operation or OCR pass; verify Asset Info, Queue, and other canonical callers use the same behavior.
- [x] 3.3 Preserve native ASS and SSA extraction as lossless stream-copy behavior that bypasses bitmap geometry; verify native styles, override positioning, karaoke data, and representation remain unchanged.
- [x] 3.4 Preserve logical both shared-result execution so one bitmap recognition and one geometry correlation feed SRT and ASS, with only ASS consuming spatial metadata; verify command counts, shared cue identity, and independently valid artifacts.

## 4. Regression coverage and validation

- [x] 4.1 Add focused backend regression fixtures for simultaneous top/bottom cues, including overlapping timings, and verify each event retains its own coarse ASS region without merging.
- [x] 4.2 Add focused regression coverage for missing geometry and a cue overlapping multiple non-equivalent source events, verifying the ambiguous cue remains unpositioned, conversion succeeds with deterministic bottom-center ASS fallback, and SRT remains unchanged.
- [x] 4.3 Add focused regression coverage using a non-zero segment/sample start where geometry and OCR cues are already prepared-input-local; assert the correct match and explicitly fail if a second segment-start adjustment occurs. Also cover one original-source-absolute translation.
- [x] 4.4 Add cleanup-retention, raw-ASS, SRT, native ASS, native SSA, and logical both regression coverage against disposable FFmpeg/SeConv fixtures; verify no serializer invokes OCR and no existing sidecar identity changes.
- [x] 4.5 Run ./scripts/verify.sh --auto and verify the result reports ALL CHECKS PASSED.
