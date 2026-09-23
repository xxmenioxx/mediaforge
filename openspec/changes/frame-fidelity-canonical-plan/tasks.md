## 1. Frozen plan and canonical source model

- [ ] 1.1 Define the bounded canonical source/evidence model with separate raw storage and effective source characteristics for geometry, SAR/DAR, cadence, field structure, pixel format, bit depth, and chroma location; verify unit coverage preserves unknown and unreliable values without fabricating metadata.
- [ ] 1.2 Implement the frozen-plan authority adapter with the documented precedence (complete execution artifact, Queue `ProfileSnapshot`, legacy frozen snapshot adapter, operation-local Preview/Test Encode plan); verify a mutable current Profile or Asset Override cannot alter an already prepared Queue plan.
- [ ] 1.3 Build one expected-output model from existing resolved authorities, including restoration/Smart Upscale geometry, effective FPS/progressive state, encoder pixel format/bit depth, preserve semantics, and frozen HEVC decision; verify contradictory or missing authorities become explicit diagnostics/unverified rather than source-based guesses.

## 2. Canonical field evaluation and evidence reuse

- [ ] 2.1 Normalize actual output evidence from the existing ffprobe and bounded bitstream results, preserving raw values and reusing the existing measured-FPS precedence (frame count/duration, then `avg_frame_rate`, with `r_frame_rate` diagnostic-only); verify no duplicate expensive probe is added to Queue validation.
- [ ] 2.2 Replace direct source-to-output comparison with field results containing source, expected, actual, status, authority, and reason; verify canonical statuses are `passed`, `mismatch`, `unverified`, and `not_applicable`, with aggregate mismatch/unverified behavior defined by the spec.
- [ ] 2.3 Preserve additive compatibility data for existing Fidelity consumers, including root/source/output report locations and any retained `changed_intentionally` marker; verify the marker means only source-versus-expected and never makes expected-versus-actual mismatch pass.
- [ ] 2.4 Remove intent decisions based on `videoFilters` or `job.Notes` whenever a frozen resolved value exists, retaining raw command/filter inspection only as non-authoritative diagnostics; verify an authoritative resolved value cannot be overridden by a filter-string heuristic.

## 3. Resolved-plan and specialized-validator integration

- [ ] 3.1 Adapt #2 cadence/motion fields (`effectiveCadenceOperation`, `effectiveCadenceFieldOrder`, `effectiveResidualCombingFallback`, `effectiveOutputFrameRate`, `effectiveOutputProgressive`, and `effectiveAutomaticMotionFilters`) into expected FPS/field-structure provenance; verify IVTC 30000/1001 interlaced to 24000/1001 progressive passes when honored, mismatches when not honored, and selective fallback does not change expected FPS.
- [ ] 3.2 Reuse the existing restoration and Smart Upscale resolved plans for geometry, SAR, DAR, and progressive expectations; verify 720x480 anamorphic to 1280x720 1:1/16:9 passes and wrong geometry/aspect mismatches without source-difference warnings.
- [ ] 3.3 Derive pixel-format/bit-depth expectations from the frozen effective encoder/profile decision and preserve chroma location only under an authoritative preserve/no-change decision; verify Main10/p010le accepts matching output, rejects 8-bit yuv420p, and unknown evidence remains unverified.
- [ ] 3.4 Aggregate existing cadence FPS, HEVC Level/Tier, and color-policy validator evidence without adding policy resolvers; verify frozen HEVC expected Level/Tier and bounded actual evidence are reported, non-HEVC is not applicable, and color expectation remains owned by the color-policy validator.

## 4. Queue and operation-surface wiring

- [ ] 4.1 Wire Queue validation to the frozen prepared plan/snapshot and immutable job evidence, removing mutable-profile reads from expected-output selection; verify later Profile, Asset Override, cadence, restoration, and Level/Tier edits leave the same Queue Fidelity expectations.
- [ ] 4.2 Ensure Preview and Test Encode expose the same resolved effective-profile inputs to the canonical evaluator at their final execution boundaries; verify equivalent frozen inputs produce equivalent expected cadence, geometry, progressive, pixel-format, and HEVC fields without a UI/filter-string resolver.
- [ ] 4.3 Add the additive report/API mapping needed for consumers to render Source, Expected by resolved plan, Actual output, Verdict, Authority, and Reason while preserving existing report consumers; verify no #19 LAB orchestration or background Fidelity window is introduced.

## 5. Focused regression matrix and validation

- [ ] 5.1 Add table-driven Fidelity tests for unchanged source/output, intentional IVTC, failed IVTC FPS, expected progressive and unexpected interlaced output, Smart Upscale geometry, wrong geometry, intentional SAR/DAR, matching Main10, Main10-to-8-bit mismatch, preserved pixel format/depth, preserved chroma location, and unknown/missing evidence; verify each applicable case asserts source, expected, actual, status, and authority.
- [ ] 5.2 Add Queue and cross-surface regression tests for frozen-plan immunity to later Profile edits, #2 TFF/BFF semantics, residual selective fallback, frozen HEVC Level/Tier, actual-FPS evidence precedence, non-authoritative filter-string heuristics, and specialized-validator aggregation; verify existing source media and snapshot compatibility remain intact.
- [ ] 5.3 After #2 is merged, rebase this change onto current `origin/main`, run `openspec validate frame-fidelity-canonical-plan --type change --strict`, then run `./scripts/verify.sh --auto` for the implementation; verify the final report contains `ALL CHECKS PASSED` and no production code is changed by this planning-only commit.
