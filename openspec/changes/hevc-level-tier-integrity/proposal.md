## Why

GitHub issue #12 exposes a correctness gap in the existing HEVC Level/Tier
plumbing: planning, FFmpeg rendering, final bitstream evidence, validation, and
Fidelity can disagree about what was required and what was produced. The
change defines one authoritative contract so bounded output constraints affect
Auto/Recommended planning, unsupported evidence remains explicitly unverified,
and users receive an accurate expected-versus-actual report.

## What Changes

- Resolve HEVC Level/Tier from effective output geometry, effective FPS/cadence
  results, and real bounded output rate-control constraints, including QSV
  target/max bitrate when applicable.
- Keep source bitrate and output MaxBR/CPB evidence distinct; do not use an
  ffprobe average `bit_rate` as proof of a peak-rate limit.
- Define Auto, Recommended, and explicit Level behavior, including deterministic
  stale-recommendation handling and the point at which the resolved decision is
  frozen for execution.
- Make Preview, Test Encode, Queue, command rendering, and final validation
  consume the same resolved Level/Tier decision.
- Determine and specify the evidence contract for `general_level_idc` and
  `general_tier_flag`, using bounded bitstream-header inspection when ordinary
  ffprobe metadata cannot prove the field.
- Report Fidelity with one precise status vocabulary: `validated`, `mismatch`,
  `unavailable`, and `unverified`, each tied to authoritative evidence.
- Make x265 enforcement deterministic: advisory without a real bounded peak
  constraint, hard after compatibility preflight with a real bounded
  maxrate/VBV constraint, and hard for explicit Level intent.
- Ensure all maxrate/buffer/VBV consequences are in the canonical frozen
  decision before command rendering, so x265 cannot silently change quality
  semantics.
- Run the bounded output header probe at most once per output validation and
  reuse its stored evidence for Fidelity/report refreshes without a new global
  cache or persistence subsystem.
- Add focused regression coverage for planning, QSV/x265 command arguments,
  output evidence, stale recommendations, and Fidelity outcomes.
- Keep cadence logic from #11, GOP design, LAB structure, and unrelated quality
  policy unchanged.

## Capabilities

### New Capabilities

- `hevc-level`: Authoritative HEVC Level/Tier resolution, command contract,
  output evidence, validation, and Fidelity reporting.

### Modified Capabilities

None.

## Impact

- Backend HEVC resolution, media-plan/command rendering, output inspection,
  queue validation, and Fidelity serialization/UI consumption.
- FFmpeg runtime capabilities determine whether bounded header evidence is
  available; missing evidence remains `unavailable` rather than an inferred
  pass, while insufficient evidence remains `unverified`.
- Existing stored profiles and queue snapshots remain readable. Stored
  recommendations must be re-evaluated against current effective output inputs
  before command emission, while an execution snapshot remains immutable after
  the defined freeze boundary.
- No source media, production/NAS data, or application code is changed by this
  planning-only artifact.
