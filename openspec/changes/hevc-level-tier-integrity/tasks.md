## 1. Canonical Level/Tier resolution

- [ ] 1.1 Extend the existing HEVC resolver input and decision contract to include effective output geometry, effective FPS, encoder/rate-control mode, applied bounded maxrate/VBV/HRD evidence, intent mode, enforcement mode, and an input fingerprint; verify with resolver unit tests that source average bitrate is ignored.
- [ ] 1.2 Resolve Auto and Recommended from the lowest supported Main-Tier Level satisfying picture size, sample rate, and real bounded rate constraints; verify geometry/FPS and QSV VBR/CBR target/maxrate scenarios, including target-only VBR and CBR fallback behavior.
- [ ] 1.3 Make stored Recommended decisions stale-safe and explicit Levels preflight-safe; verify changed geometry, cadence-resolved FPS, and rate-control inputs recompute Recommended while incompatible explicit Levels block valid command planning without silent up-leveling.
- [ ] 1.4 Preserve the existing cadence and frame-structure resolution order and make the resolved Level/Tier decision immutable at Preview/Test Encode command preparation and Queue PREPARING freeze; verify post-freeze profile changes do not alter the frozen snapshot.

## 2. Command and quality-semantics alignment

- [ ] 2.1 Render QSV Level/Tier and rate-control arguments from the frozen decision; verify command tests show the same effective maxrate/buffer values that participated in planning.
- [ ] 2.2 Apply the x265 enforcement policy without silently changing quality semantics: advisory Auto/Recommended without a bounded peak-rate input, hard enforcement for compatible bounded or explicit decisions, and surfaced VBV impact; verify `level-idc`/tier parameters and omission cases.
- [ ] 2.3 Expose the frozen Level/Tier, rate evidence, compatibility, and enforcement metadata through the canonical effective video decision consumed by Preview, Test Encode, Queue, and Fidelity; verify all entry points agree on a shared fixture.

## 3. Output bitstream evidence

- [ ] 3.1 Extend the existing bounded FFmpeg `trace_headers` evidence path to parse `general_level_idc` and `general_tier_flag` for the selected video stream using argument arrays, a context bound, and bounded diagnostics; verify parser fixtures for Main Tier, alternate values, malformed headers, and unavailable runtime support.
- [ ] 3.2 Keep header Level/Tier evidence separate from ffprobe average bitrate and planned maxrate/VBV evidence; verify an average `bit_rate` alone cannot produce a validated MaxBR/CPB status.
- [ ] 3.3 Cache or persist one parsed output evidence record per validation operation so Fidelity refreshes do not rerun the header probe; verify legacy outputs without evidence remain readable and unverified.

## 4. Validation and Fidelity reporting

- [ ] 4.1 Validate final output Level and Tier against the immutable plan using authoritative header evidence, leaving actual values unavailable when evidence is absent; verify matching, mismatching, and unsupported-probe outcomes.
- [ ] 4.2 Report expected versus actual Level/Tier and separate validated, mismatch, and unavailable/unverified states in the existing Fidelity payload; verify planned Main Tier is never emitted as observed Tier without `general_tier_flag` evidence.
- [ ] 4.3 Validate planned output requirements and peak-rate evidence without treating average ffprobe bitrate as MaxBR proof; verify warning/mismatch behavior for incompatible geometry, frame rate, and bounded output constraints.
- [ ] 4.4 Prove end-to-end parity from resolver through command, output evidence, queue validation, Test Encode validation, and Fidelity using focused backend regression tests; verify no validation path recomputes a different Level/Tier after freeze.

## 5. Repository verification

- [ ] 5.1 Run the focused HEVC resolver, command, evidence, validation, and Fidelity tests and address only failures in this change; verify all targeted tests pass.
- [ ] 5.2 Run `./scripts/verify.sh --auto` and verify the output ends with `ALL CHECKS PASSED`.
