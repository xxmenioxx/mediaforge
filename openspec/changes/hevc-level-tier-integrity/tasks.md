## 1. Canonical Level/Tier resolution

- [x] 1.1 Extend the existing HEVC resolver input and frozen decision contract to include effective output geometry, effective FPS, encoder/rate-control mode, applied bounded maxrate/VBV evidence, intent mode, enforcement mode, consequences, and an input fingerprint; verify with resolver tests that source average bitrate is ignored.
- [x] 1.2 Resolve Auto and Recommended from the lowest supported Main-Tier Level satisfying picture size, sample rate, and real bounded rate constraints; verify geometry/FPS and QSV VBR/CBR target/maxrate scenarios, including target-only VBR and CBR fallback behavior.
- [x] 1.3 Implement deterministic Recommended fingerprint handling: matching fingerprint reuses, changed fingerprint with sufficient inputs synchronously recomputes, and changed fingerprint with insufficient inputs is unresolved with no stale Level/Tier enforcement; verify each state and keep Explicit Level as separate user intent.
- [x] 1.4 Preserve the existing cadence and frame-structure resolution order and make the resolved Level/Tier decision immutable at Preview/Test Encode command preparation and Queue PREPARING freeze; verify post-freeze profile changes do not alter the frozen snapshot.

## 2. Deterministic command and quality-semantics alignment

- [x] 2.1 Render QSV Level/Tier and rate-control arguments from the frozen decision; verify command tests show the same effective maxrate/buffer values that participated in planning.
- [x] 2.2 Implement deterministic x265 enforcement: advisory Auto/Recommended without a real bounded peak constraint with no `level-idc`/`high-tier`, hard bounded Auto/Recommended after compatibility preflight, and hard Explicit Level after compatibility preflight; verify incompatible explicit plans are rejected and no stale Recommended value reaches rendering.
- [x] 2.3 Record every x265 maxrate, buffer, and VBV consequence in the canonical frozen decision before command rendering; verify rendered arguments cannot introduce an unrecorded quality or rate-control change.
- [x] 2.4 Expose the frozen Level/Tier, rate evidence, compatibility, enforcement, and consequences through the canonical effective video decision consumed by Preview, Test Encode, Queue, and Fidelity; verify all entry points agree on a shared fixture.

## 3. Output bitstream evidence and reuse

- [x] 3.1 Extend the existing bounded FFmpeg `trace_headers` evidence path to parse `general_level_idc` and `general_tier_flag` for the selected video stream using argument arrays, a context bound, and bounded diagnostics; verify parser fixtures for matching values, mismatches, malformed headers, and unsupported runtime behavior.
- [x] 3.2 Define and apply the exact evidence statuses: `validated` for authoritative matching evidence, `mismatch` for authoritative differing evidence, `unavailable` when authoritative evidence cannot be obtained, and `unverified` when available evidence is insufficient; verify ffprobe average `bit_rate` is only `unverified` for MaxBR/CPB claims.
- [x] 3.3 Run the bounded `trace_headers` probe at most once per applicable output validation and store its parsed evidence/outcome in the existing validation report or job-artifact boundary; verify Fidelity/report refreshes reuse the stored record without FFmpeg and no new global cache or persistence subsystem is added.

## 4. Validation and Fidelity reporting

- [x] 4.1 Validate final output Level and Tier against the immutable plan using authoritative stored header evidence, leaving actual values `unavailable` when evidence is absent; verify matching, mismatching, failed-probe, legacy-output, and unsupported-runtime outcomes.
- [x] 4.2 Report expected versus actual Level/Tier and rate evidence using the exact status vocabulary in the existing Fidelity payload; verify planned Main Tier is never emitted as observed Tier without stored `general_tier_flag` evidence.
- [x] 4.3 Validate planned output requirements and peak-rate evidence without treating average ffprobe bitrate as MaxBR proof; verify insufficient evidence is `unverified` rather than `validated` or `mismatch`.
- [x] 4.4 Prove end-to-end parity from resolver through command, output evidence, queue validation, Test Encode validation, and Fidelity using focused backend regression tests; verify no validation path recomputes a different Level/Tier after freeze.

## 5. Repository verification

- [x] 5.1 Run the focused HEVC resolver, command, evidence, validation, and Fidelity tests and address only failures in this change; verify all targeted tests pass.
- [x] 5.2 Run `./scripts/verify.sh --auto` and verify the output ends with `ALL CHECKS PASSED`.
