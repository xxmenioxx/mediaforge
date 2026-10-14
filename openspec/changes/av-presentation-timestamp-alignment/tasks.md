## 1. Comparable presentation evidence

- [x] 1.1 Parse first-presented video/audio timestamps and their provenance from available probes; use at most one bounded two-second fallback probe per side only when required anchors are missing, and never use `stream.start_time` as a comparison anchor. Verify with focused parser and fallback tests.
- [x] 1.2 Calculate source-relative, output-relative, and introduced offsets using frozen track identity and existing thresholds; persist per-track frames, tolerances, evidence, and decision as optional finding details while retaining existing expected/actual evidence. Verify with focused A/V and finding serialization tests.

## 2. Regression and compatibility coverage

- [x] 2.1 Cover VC-1 42 ms to 42 ms and H.264 0 ms to 0 ms as validated; cover measurable introduced offsets, multiple mapped audio tracks, missing/incomparable timestamps, and quantization boundaries in backend tests.
- [x] 2.2 Verify new Validation findings preserve Result identity and canonical Publisher outcome behavior, and confirm historical ValidationRun data remains readable and unchanged with absent optional details.

## 3. Validation and reconciliation

- [x] 3.1 Run the focused backend tests and `./scripts/verify.sh --auto`; require the canonical verifier to report `ALL CHECKS PASSED`.
- [x] 3.2 Review the final diff against issue #334 scope, validate the OpenSpec change, and reconcile all task checkboxes only after their behavior and verification are complete.
