## Purpose

Frame Fidelity gives users and downstream validation consumers a trustworthy,
field-level explanation of whether the output honored the frozen resolved
execution plan, while keeping intentional source-to-output transformations
distinct from output mismatches.

## ADDED Requirements

### Requirement: Fidelity SHALL compare canonical source, expected output, and actual output

For every covered field, Frame Fidelity SHALL represent the raw storage evidence,
the bounded canonical/effective source value when available, the expected output
from the frozen resolved plan, and the actual output evidence. Source metadata
MUST NOT be mutated or fabricated to make it resemble the expected output.
Missing or insufficient evidence SHALL remain explicitly unverified.

The minimum covered fields are frame width/height, sample aspect ratio (SAR),
display aspect ratio (DAR), output FPS, progressive/interlaced field structure,
pixel format, bit depth, chroma location, and applicable HEVC Level/Tier.
Codec/profile MAY be included as diagnostic fields or compared when an existing
frozen decision is authoritative, but Frame Fidelity SHALL NOT invent a new
codec/profile policy resolver.

#### Scenario: Unchanged source and output pass
- **WHEN** canonical source and actual output have the same known width, height, SAR, DAR, FPS, field structure, pixel format, bit depth, and chroma location, and the resolved plan preserves them
- **THEN** each applicable field SHALL report `status: passed` with source, expected, actual, and its authority, and the aggregate Fidelity verdict SHALL pass

#### Scenario: Missing source evidence is not fabricated
- **WHEN** source width, cadence, field structure, or another covered characteristic is missing or unreliable
- **THEN** the report SHALL retain the available raw evidence, SHALL NOT derive a replacement merely to match output, and SHALL report `unverified` unless an authoritative resolved plan independently supplies an expected value that can be compared with actual output evidence

#### Scenario: Preserve chroma location
- **WHEN** canonical source chroma location is `left`, the resolved plan has no authoritative chroma-location transformation, and output evidence is `left`
- **THEN** the chroma-location field SHALL pass; if known output evidence differs, the field SHALL be `mismatch`, and if either required value is unavailable it SHALL be `unverified`

### Requirement: Expected output SHALL come from frozen resolved authorities

Frame Fidelity SHALL evaluate expected output from the strongest immutable
resolved execution authority available for the operation. For Queue validation,
the precedence SHALL be: a complete immutable prepared/execution plan artifact
for the same job, then the Queue `ProfileSnapshot` captured at PREPARING together
with its preserved resolved analysis evidence, then a legacy frozen snapshot
adapter. Mutable current Profile, Asset Override, recommendation, cadence, or
restoration state SHALL NOT determine expectations for an already prepared job.

Within that frozen plan, specialized resolved authorities SHALL be adapted into
one expected-output model: resolved restoration/upscale geometry owns geometry
and aspect decisions; `effectiveOutputFrameRate` owns expected FPS; the resolved
progressive decision owns field structure; the effective encoder/profile
decision owns pixel format and bit-depth expectations; and the existing frozen
HEVC decision owns Level/Tier expectations. A preserve decision may use the
canonical effective source value as expected output. If no authority owns a
field, the field SHALL be unverified rather than inferred from command text.

#### Scenario: Smart Upscale geometry is expected output
- **WHEN** source is 720x480 anamorphic and the frozen Smart Upscale/restoration plan resolves 1280x720, SAR 1:1, and DAR 16:9
- **THEN** those resolved values SHALL be the expected output, and an actual 1280x720, 1:1, 16:9 result SHALL pass even though source and expected differ

#### Scenario: Wrong Smart Upscale geometry mismatches
- **WHEN** the same frozen plan expects 1280x720, SAR 1:1, and DAR 16:9 but actual output is 1280x720 with an incompatible aspect result or another geometry mismatch
- **THEN** the affected field SHALL report `mismatch` with the resolved restoration/upscale authority and SHALL NOT be downgraded to an intentional-change warning

#### Scenario: Unknown source with authoritative expected output
- **WHEN** source metadata is incomplete but the frozen resolved plan authoritatively expects a width, height, FPS, or progressive state and actual output evidence is available
- **THEN** Frame Fidelity MAY validate expected versus actual for those fields, while source values remain unknown/unverified and no missing source value is fabricated

### Requirement: Intentional transformation SHALL be distinct from validation success

For every field, `source != expected` SHALL describe an intentional or resolved
transformation only when the authority says the transformation is part of the
plan. Validation SHALL always compare `expected` with `actual`. An intentional
transformation whose actual output matches expected SHALL be `passed`; an actual
output that differs from expected SHALL be `mismatch`, regardless of whether the
source also differed. If retained for compatibility, `changed_intentionally`
SHALL mean only `source != expected` and SHALL never mean `expected != actual`.

#### Scenario: IVTC 29.97 to 23.976 is honored
- **WHEN** canonical source is 30000/1001 and interlaced, the frozen plan expects 24000/1001 and progressive, and actual output is 24000/1001 and progressive
- **THEN** FPS and field-structure fields SHALL pass, the report SHALL identify the resolved cadence authority, and any intentional-change marker SHALL not downgrade the aggregate verdict

#### Scenario: IVTC output fails to change FPS
- **WHEN** the frozen plan expects 24000/1001 progressive output but actual output remains 30000/1001 interlaced
- **THEN** FPS and field-structure fields SHALL report `mismatch`, even though the source-to-expected change was intentional

### Requirement: Cadence and motion semantics SHALL be consumed as resolved evidence

The evaluator SHALL consume #2's frozen fields
`effectiveCadenceOperation`, `effectiveCadenceFieldOrder`,
`effectiveResidualCombingFallback`, `effectiveOutputFrameRate`,
`effectiveOutputProgressive`, and `effectiveAutomaticMotionFilters` as
resolved evidence. It SHALL validate observable output characteristics and SHALL
NOT require filter-string inspection to decide expected FPS or field structure.
Residual selective fallback SHALL NOT by itself change the expected FPS.
Unreliable or unknown progressive/FPS resolution SHALL produce `unverified`,
not a fabricated expected value.

#### Scenario: Resolved TFF/BFF semantics are not raw-source warnings
- **WHEN** the frozen plan resolves a valid TFF or BFF cadence field order and the corresponding IVTC or motion operation produces the expected output
- **THEN** the field-order/cadence fields SHALL pass or be unverified according to output evidence, and the source's original field order SHALL NOT independently create an unexpected-change warning

#### Scenario: Residual selective fallback preserves expected FPS
- **WHEN** the frozen plan has a residual-combing fallback of `selective` and an effective output FPS of 24000/1001
- **THEN** FPS expectation SHALL remain 24000/1001 and the fallback SHALL be reported as plan provenance rather than treated as a separate FPS transformation

### Requirement: Actual output SHALL use existing measured and bounded evidence

Actual values SHALL come from the existing output evidence boundary, including
ffprobe stream characteristics, measured frame count/duration, and bounded
bitstream evidence where a specialized validator already requires it. Measured
FPS SHALL take precedence over `avg_frame_rate`, and `avg_frame_rate` SHALL take
precedence over `r_frame_rate`; `r_frame_rate` SHALL remain diagnostic or
corroborating evidence only. Frame Fidelity SHALL reuse already collected probe
and bitstream evidence and SHALL NOT introduce duplicate expensive probes for
the same validation.

#### Scenario: Measured FPS contradicts stream metadata
- **WHEN** frame count divided by duration reports an FPS different from `avg_frame_rate` and the resolved plan has a known expected FPS
- **THEN** validation SHALL compare expected FPS with the measured result, retain the stream disagreement as diagnostic evidence, and SHALL NOT use `r_frame_rate` to override the measured verdict

#### Scenario: Only base-rate metadata is available
- **WHEN** expected FPS is known but measured FPS and `avg_frame_rate` are unavailable while only `r_frame_rate` is present
- **THEN** the FPS field SHALL be `unverified` and SHALL identify `r_frame_rate` as diagnostic-only evidence

### Requirement: Specialized validators SHALL remain owners of their policies

Frame Fidelity SHALL aggregate or reference the existing Smart Upscale,
restoration, cadence FPS, HEVC Level/Tier, and color-policy validation results.
It SHALL adapt their canonical expected and actual evidence into the common
field-result shape without reconstructing their policies or creating a second
resolver. The existing color-policy validator SHALL remain the owner of color
expectation; Frame Fidelity MAY expose its expected/actual evidence but SHALL
NOT duplicate color-policy resolution. HEVC Level/Tier SHALL use the frozen
resolved decision and existing bounded bitstream validation.

#### Scenario: Main10 expectation is enforced
- **WHEN** the frozen effective encoder decision expects Main10/p010le and 10-bit output
- **THEN** actual Main10/p010le evidence SHALL pass, while actual 8-bit yuv420p SHALL report a pixel-format and/or bit-depth `mismatch` with the effective encoder authority

#### Scenario: HEVC Level/Tier uses frozen decision
- **WHEN** a Queue job has a frozen HEVC Level/Tier decision and bounded output evidence is available
- **THEN** Frame Fidelity SHALL report the existing Level/Tier validator's expected and actual evidence and verdict, and SHALL NOT recalculate a Level/Tier recommendation from current profile state or source bitrate

#### Scenario: Color policy remains specialized
- **WHEN** the frozen color-policy validator expects preserved source color or an explicit BT.709 normalization
- **THEN** the Fidelity report MAY reference that validator's result, but a Frame Fidelity fallback SHALL NOT infer a competing color expectation from filters, notes, or codec labels

### Requirement: Reports SHALL explain each verdict and preserve compatibility

Each canonical field result SHALL expose, directly or through an equivalent
stable structure, `source`, `expected`, `actual`, `status`, `authority`, and a
human-readable reason. Canonical field statuses SHALL distinguish `passed`,
`mismatch`, `unverified`, and `not_applicable`. The existing report root and
legacy `source`/`output` data SHALL remain available where practical; additive
canonical fields SHALL allow existing consumers to migrate without losing
diagnostic data. Aggregate reporting SHALL distinguish an output mismatch from
an intentional source-to-expected transformation.

#### Scenario: IVTC report is understandable without FFmpeg commands
- **WHEN** IVTC changes source FPS and field structure but actual output matches the resolved plan
- **THEN** the report SHALL show source 30000/1001/interlaced, expected 24000/1001/progressive, actual 24000/1001/progressive, a passed verdict, and a cadence authority/reason

#### Scenario: Non-applicable specialized field
- **WHEN** output is not HEVC and no HEVC Level/Tier validation applies
- **THEN** the HEVC field SHALL be `not_applicable` rather than a false mismatch or unverified failure

### Requirement: Queue, Preview, and Test Encode SHALL use the same resolved plan contract

Queue validation SHALL consume the frozen prepared plan/snapshot and SHALL be
immune to later mutable Profile, Asset Override, cadence analysis,
restoration-setting, or Level/Tier recommendation edits. Preview and Test Encode
effective-profile handling SHALL expose the same resolved expected-output inputs
at their operation's execution boundary, so the future evaluator can be reused
without entry-point-specific policy. This change SHALL NOT add LAB
preview-first/background Fidelity orchestration owned by #19.

#### Scenario: Later Profile edit cannot change Queue expectations
- **WHEN** a Queue job is prepared with an IVTC, restoration, or Main10 plan and the Profile is edited before validation
- **THEN** Fidelity SHALL continue to use the job's frozen plan and SHALL produce the same expected values and authorities as before the edit

#### Scenario: Preview and Test Encode expose effective expectations
- **WHEN** Preview or Test Encode resolves a profile with cadence, geometry, or encoder changes
- **THEN** its inspection/effective-profile result SHALL carry the resolved values needed by the canonical evaluator, without re-deriving expectations from raw filter strings
