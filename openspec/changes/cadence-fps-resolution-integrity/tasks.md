## 1. Canonical cadence resolution

- [ ] 1.1 Define the resolved motion-plan cadence output contract so effective FPS is derived from the operations that will be rendered, preserving the existing effective-rate fields, unknown flags, and warnings; verify resolver tests reject a mode-only 24000/1001 result when no matching rendered cadence operation exists.
- [ ] 1.2 Implement and test deterministic field-order precedence: valid explicit `cadenceFieldOrder`, validated detected order, then the existing conservative fallback; verify conflicting, missing, invalid, and unvalidated detection cases remain conservative.
- [ ] 1.3 Require explicit inverse telecine to render the canonical fieldmatch+decimate chain with the resolved field order before reporting progressive 24000/1001; verify both `tff` and `bff` plans and the unresolved-order review path.
- [ ] 1.4 Gate `effectiveOutputProgressive` on resolved progressive-producing processing or reliable analysis evidence, preserving existing unknown/unreliable state; verify a progressive operation label without supporting evidence does not become trusted output metadata.

## 2. Preserve existing cadence safety behavior

- [ ] 2.1 Preserve validated automatic IVTC resolution and rendering without changing detection thresholds; verify existing validated automatic-IVTC regressions remain green and retain their effective FPS/progressive results.
- [ ] 2.2 Keep suspected or unvalidated telecine review-only and prevent unsafe automatic normalization; verify no fieldmatch/decimate normalization is rendered for insufficient evidence.
- [ ] 2.3 Keep soft-telecine removal's rendered FPS filter and effective output FPS consistent; verify the filter target, resolved metadata, and validation expectation agree.

## 3. Validation evidence and effective-plan checks

- [ ] 3.1 Update cadence validation to treat `avg_frame_rate` as primary stream-rate evidence and `r_frame_rate` as corroborating/base-rate metadata; verify a conflicting base rate cannot replace the primary average-rate decision.
- [ ] 3.2 Use reliable frame count plus duration as stronger measured evidence when available, while retaining bounded stream metadata for diagnostics; verify missing, conflicting, and measured-rate cases produce the expected validation result or review warning.
- [ ] 3.3 Make validation consume the frozen/resolved effective cadence decision rather than reconstructing expected FPS or progressive state from requested settings; verify explicit IVTC, soft-telecine removal, and conservative review cases end with no false normalization expectation.

## 4. Entry-point parity and Queue freezing

- [ ] 4.1 Route Preview, Test Encode, and Queue through the same cadence resolution inputs and precedence contract without adding surface-specific heuristics; verify identical frozen inputs produce equivalent operation, field order, effective FPS, progressive evidence, and review state.
- [ ] 4.2 Preserve Queue's immutable resolved cadence plan after freeze; verify changes to mutable cadence settings after freeze do not alter the queued operation's field order, rendered cadence chain, effective FPS, or progressive state.
- [ ] 4.3 Verify source analysis and cadence intent are frozen consistently at the existing lifecycle boundary, with no additional cadence probe or render path introduced for any entry point.

## 5. Compatibility and focused regression coverage

- [ ] 5.1 Add focused regressions covering explicit IVTC order precedence, rendered-plan FPS, progressive evidence, automatic IVTC preservation, review-only telecine, soft-telecine consistency, and validation evidence roles.
- [ ] 5.2 Add cross-entry-point regressions for Preview, Test Encode, and Queue parity plus post-freeze immutability, using identical frozen inputs and no media-policy changes outside cadence resolution.
- [ ] 5.3 Confirm no HEVC Level/Tier, GOP, cadence-detection-threshold, or Profile Lab behavior is changed; verify existing unrelated video policy and current validated cadence tests remain green.
- [ ] 5.4 Run the focused cadence/validation and cross-entry-point tests, then run `./scripts/verify.sh --auto` and `openspec validate cadence-fps-resolution-integrity --type change --strict`; mark this checklist complete only after all required evidence passes.
