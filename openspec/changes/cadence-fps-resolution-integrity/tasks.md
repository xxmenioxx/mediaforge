## 1. Canonical cadence resolution

- [ ] 1.1 Define the resolved motion-plan cadence output contract so effective FPS is derived from the operations that will be rendered, preserving the existing effective-rate fields, unknown flags, and warnings; verify resolver tests reject a mode-only 24000/1001 result when no matching rendered cadence operation exists.
- [ ] 1.2 Implement and test deterministic field-order precedence: valid explicit `cadenceFieldOrder`, validated IVTC order accepted by the existing automatic-IVTC validation boundary, then unresolved/review; verify generic non-IVTC detected order is insufficient and no TFF/BFF guess is introduced.
- [ ] 1.3 Require explicit inverse telecine to render the canonical fieldmatch+decimate chain with a usable resolved field order before reporting progressive 24000/1001; verify both `tff` and `bff` plans plus the no-order path produces no chain, no frozen 24000/1001, and review/unknown state.
- [ ] 1.4 Gate `effectiveOutputProgressive` on resolved progressive-producing processing or reliable analysis evidence, preserving existing unknown/unreliable state; verify a progressive operation label without supporting evidence does not become trusted output metadata.

## 2. Preserve existing cadence safety behavior

- [ ] 2.1 Preserve validated automatic IVTC resolution and rendering without changing detection thresholds; verify existing validated automatic-IVTC regressions remain green and retain their effective FPS/progressive results.
- [ ] 2.2 Keep suspected or unvalidated telecine review-only and prevent unsafe automatic normalization; verify no fieldmatch/decimate normalization is rendered for insufficient evidence.
- [ ] 2.3 Keep soft-telecine removal's rendered FPS filter and effective output FPS consistent; verify the filter target, resolved metadata, and validation expectation agree.

## 3. Validation evidence and effective-plan checks

- [ ] 3.1 Update cadence validation to use reliable frame count/duration first, `avg_frame_rate` as primary ffprobe stream-rate evidence, and `r_frame_rate` as diagnostic/corroborating base-rate metadata only; verify matching average rate with conflicting base rate remains primary and base-rate-only evidence is unverified.
- [ ] 3.2 Use reliable frame count plus duration as the strongest evidence when available, overriding stream metadata for the effective validation result; verify a measured contradiction is a mismatch and stream disagreements remain bounded diagnostics.
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
