## Why

Frame Fidelity currently compares source metadata directly with output metadata and uses raw filter/notes strings to guess whether differences were intentional. That makes resolved IVTC, restoration, Smart Upscale, encoder bit-depth, and aspect-ratio decisions look suspicious even when the output is correct, while allowing an output to pass merely because it resembles the source. Issue #3 defines a canonical source → frozen resolved plan → expected output → actual output evaluation so validation is correct and explainable after #2's cadence/motion contract.

## What Changes

- Define a bounded canonical source representation that separates raw storage characteristics from effective source geometry/cadence/field evidence and preserves missing evidence as unverified.
- Define one expected-output model sourced from frozen resolved execution state, including geometry, SAR/DAR, FPS, field structure, observable pixel-format semantics, bit depth, chroma location, and applicable HEVC Level/Tier expectations.
- Distinguish requested/encoder working pixel format from observable encoded-output characteristics; compare semantic bit depth/subsampling/profile evidence where working and decoded pixel-format spellings differ, while retaining raw spellings diagnostically.
- Replace Frame Fidelity's intent-by-filter-string decisions with field-by-field expected-versus-actual evaluation; retain command/filter inspection only as diagnostic fallback when no authoritative resolved value exists.
- Consume #2's resolved cadence/motion fields, including effective operation, field order, residual fallback, output FPS, progressive state, and automatic motion filters, without validating filter strings themselves.
- Reuse existing Smart Upscale, restoration, cadence FPS, HEVC Level/Tier, and color-policy validators as authorities or specialized evidence producers instead of creating parallel policy resolvers.
- Define frozen Queue precedence, require execution artifacts to prove same-execution identity before outranking `ProfileSnapshot`, and ensure later Profile, override, cadence, restoration, or Level/Tier edits cannot change expectations for a prepared job.
- Define field applicability so optional missing evidence does not poison an otherwise valid aggregate, while authoritative missing evidence remains unverified and real mismatches remain mismatches.
- Evolve reports additively so each field can explain source, expected, actual, status, authority, and reason while preserving existing consumers where practical.
- Add focused backend regression coverage for unchanged media, intentional and failed transformations, missing evidence, frozen Queue plans, #2 field-order semantics, HEVC evidence, and heuristic non-overrides.

## Capabilities

### New Capabilities

- `frame-fidelity-canonical-plan`: Canonical source characterization, frozen resolved expected-output evaluation, field-level Fidelity reporting, and specialized-validator aggregation.

### Modified Capabilities

None. The repository has no existing synced capability spec for Frame Fidelity, cadence, restoration, Smart Upscale, or HEVC Level/Tier; this change references their current implementation contracts without rewriting those requirements.

## Impact

- Backend validation and report aggregation, especially `validateJobFrameFidelity`, `frameFidelityValue`, `firstVideoFrameCharacteristics`, and cadence output-rate validation.
- Queue snapshot/artifact consumption, restoration and upscale plan reuse, existing HEVC bitstream evidence, and color-policy reporting.
- Preview and Test Encode effective-profile consumers must be able to hand the same resolved plan to the future evaluator; #19's orchestration, background windows, and evidence-reuse workflow remain out of scope.
- No source metadata mutation, media rewrite, production/NAS operation, OpenSpec archive, or production implementation is included. The branch already starts at the merged #2 commit `816a1f1b36689fcc70d715541deda5e5fe2854d4`; no additional #2 merge or rebase prerequisite remains for implementation.
