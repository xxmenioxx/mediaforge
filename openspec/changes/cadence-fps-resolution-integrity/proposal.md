## Why

Issue #11 addresses a correctness gap where cadence intent, rendered motion
processing, and reported output FPS can diverge. The change formalizes one
backend-authoritative resolution contract so explicit IVTC, automatic IVTC,
soft-telecine removal, validation, and the Preview/Test Encode/Queue entry
points cannot report a progressive or 24000/1001 result without evidence from
the resolved processing plan.

## What Changes

- Resolve effective output FPS from the rendered resolved motion plan rather
  than from the requested cadence mode alone.
- Require explicit inverse telecine to resolve field order and render the
  canonical fieldmatch+decimate chain before reporting 24000/1001 or
  progressive output.
- Apply deterministic field-order precedence: explicit `cadenceFieldOrder`,
  validated detected field order, then the existing conservative fallback.
- Require `effectiveOutputProgressive` to be supported by resolved processing
  and/or reliable analysis evidence; retain review-only behavior for suspected
  or unvalidated telecine.
- Keep validated automatic IVTC unchanged and keep soft-telecine removal's FPS
  filter consistent with its effective output FPS.
- Make validation distinguish primary `avg_frame_rate`, corroborating
  `r_frame_rate`, and stronger measured frame-count/duration evidence.
- Prove Preview, Test Encode, and Queue resolve identical cadence intent from
  identical frozen inputs.

## Capabilities

### New Capabilities

- `cadence`: Define the canonical cadence/motion-plan resolution, effective FPS
  and progressive-state evidence, validation interpretation, and parity
  contract for Preview, Test Encode, and Queue.

### Modified Capabilities

None.

## Impact

- The canonical motion/cadence resolver, FFmpeg plan generation, validation,
  and their focused regression tests will gain explicit consistency checks.
- Preview, Test Encode, and Queue must consume the same resolved cadence intent
  and frozen inputs without introducing entry-point-specific heuristics.
- Existing validated automatic IVTC, conservative review behavior, source
  preservation, and unrelated video policy remain unchanged.
- No migration, public API redesign, HEVC Level/Tier change, GOP redesign, or
  Profile Lab refactor is included.

## Out of Scope

- Redesigning cadence detection thresholds or telecine classification.
- HEVC Level/Tier work tracked by issue #12.
- GOP redesign or new frame-structure policy.
- A Profile Lab refactor.
- Changing validated automatic IVTC behavior or introducing a second cadence
  resolver.
