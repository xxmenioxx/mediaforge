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
Missing or insufficient evidence for an applicable expected field SHALL remain
explicitly unverified; a field with no authoritative expectation is
`not_applicable`.

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

### Requirement: Pixel-format validation SHALL distinguish working and observable domains

Frame Fidelity SHALL retain raw `source.pixelFormat`, `workingPixelFormat`, and
`actual.pixelFormat` values for diagnostics, but SHALL build expected output in
the observable domain when the frozen encoder decision describes a working
surface rather than a literal encoded-stream representation. At minimum,
semantic comparison SHALL distinguish bit depth and reliably known
chroma/subsampling family, and SHALL include Main/Main10 profile semantics when
an existing frozen encoder/profile authority provides them. The evaluator SHALL
reuse existing pixel-format bit-depth and effective encoder/profile evidence and
SHALL NOT create a broad arbitrary alias table or a second codec/profile
resolver.

For hardware encoders, supported working-to-output semantics SHALL recognize
that QSV `p010le` can produce observable HEVC Main10 10-bit 4:2:0 evidence such
as `yuv420p10le`; the evaluator SHALL NOT require `p010le == yuv420p10le` or
require `nv12` to appear literally in the final ffprobe stream. Exact
comparison SHALL remain available when the frozen plan genuinely owns the
observable representation, including explicit software output formats and
preserve/copy paths in the same semantic domain.

#### Scenario: QSV Main10 working format matches decoded output semantics
- **WHEN** the frozen plan has encoder `hevc_qsv`, working pixel format `p010le`, authoritative Main10/10-bit intent, and actual ffprobe evidence is HEVC with `yuv420p10le` and bit depth 10
- **THEN** pixel-format semantics and bit depth SHALL report `status: passed`, raw working/output spellings SHALL remain diagnostic, and the aggregate SHALL pass

#### Scenario: QSV Main10 output is actually 8-bit
- **WHEN** the frozen plan expects Main10/10-bit output from QSV but actual ffprobe evidence is `yuv420p` with bit depth 8
- **THEN** the observable pixel-format and/or bit-depth result SHALL report `mismatch` with the effective encoder/profile authority

#### Scenario: Software x265 owns an exact observable format
- **WHEN** the frozen software encoding plan authoritatively expects observable `yuv420p10le` and actual output is `yuv420p10le`
- **THEN** exact observable pixel-format comparison and bit-depth comparison SHALL both pass

#### Scenario: Working-format spelling is not an observable requirement
- **WHEN** a hardware plan records `nv12` or `p010le` only as encoder input/working format and the encoded stream reports an equivalent supported observable semantic format
- **THEN** the raw spelling difference SHALL not be a mismatch, and the report SHALL show both domains without requiring the working spelling in ffprobe output

### Requirement: Expected output SHALL come from frozen resolved authorities

Frame Fidelity SHALL evaluate expected output from the strongest immutable
resolved execution authority available for the operation. For Queue validation,
the precedence SHALL be: a complete immutable prepared/execution plan artifact
whose identity is proven to belong to the execution being validated, then the
Queue `ProfileSnapshot` captured at PREPARING together with its preserved
resolved analysis evidence, then a legacy frozen snapshot adapter. A job ID,
media path, or latest artifact modification time alone SHALL NOT prove
same-execution identity. Existing stable evidence SHALL be used where available,
including matching job/source identity plus `ActiveExecutionPlanID`, an embedded
frozen snapshot/prepared-profile identity, or an existing attempt/revision
identity. Mutable current Profile, Asset Override, recommendation, cadence, or
restoration state SHALL NOT determine expectations for an already prepared job.

Within that frozen plan, specialized resolved authorities SHALL be adapted into
one expected-output model: resolved restoration/upscale geometry owns geometry
and aspect decisions; `effectiveOutputFrameRate` owns expected FPS; the resolved
progressive decision owns field structure; the effective encoder/profile
decision owns observable pixel-format semantics and bit-depth expectations while
retaining the encoder working format diagnostically; and the existing frozen
HEVC decision owns Level/Tier expectations. A preserve decision may use the
canonical effective source semantic value as expected output. If no authority
owns a field, the field SHALL be `not_applicable`, not inferred from command
text; if an authoritative applicable field lacks required evidence, it SHALL be
`unverified`.

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

### Requirement: Aggregate verdict SHALL honor field applicability

Each canonical field SHALL be classified deliberately as applicable or
`not_applicable` based on whether the frozen plan owns an expectation for that
field. An applicable field with missing or unreliable expected/actual evidence
SHALL be `unverified`; an applicable field with contradictory actual evidence
SHALL be `mismatch`. A field with no authoritative expectation, including an
optional characteristic absent from both the plan and evidence, SHALL be
`not_applicable` and SHALL NOT downgrade an otherwise valid aggregate.
Aggregate status SHALL be `mismatch` if any applicable field mismatches,
`unverified` if none mismatch but an applicable field is unresolved, `passed`
if all applicable fields pass, and `not_applicable` if no canonical field is
applicable.

#### Scenario: Optional chroma evidence does not poison a valid job
- **WHEN** geometry, FPS, field structure, and pixel-depth expectations all pass, but the frozen plan does not own chroma location and no chroma evidence is available
- **THEN** the chroma field SHALL be `not_applicable` and the aggregate SHALL remain `passed`

#### Scenario: Authoritative missing evidence is unverified
- **WHEN** the frozen plan explicitly expects a preserved chroma location or observable semantic format but the required source or output evidence is missing
- **THEN** that applicable field SHALL be `unverified` and the aggregate SHALL be `unverified` unless another applicable field is already `mismatch`

#### Scenario: Real mismatch is not weakened by optional evidence
- **WHEN** an applicable expected FPS or geometry value differs from actual output and unrelated optional fields are absent
- **THEN** the affected field and aggregate SHALL remain `mismatch`

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
- **WHEN** the frozen effective encoder decision expects Main10/10-bit output and records QSV working format `p010le`
- **THEN** actual HEVC `yuv420p10le` with bit depth 10 SHALL pass, while actual 8-bit `yuv420p` SHALL report a pixel-format and/or bit-depth `mismatch` with the effective encoder authority; raw spellings SHALL remain visible diagnostically

#### Scenario: HEVC Level/Tier uses frozen decision
- **WHEN** a Queue job has a frozen HEVC Level/Tier decision and bounded output evidence is available
- **THEN** Frame Fidelity SHALL report the existing Level/Tier validator's expected and actual evidence and verdict, and SHALL NOT recalculate a Level/Tier recommendation from current profile state or source bitrate

#### Scenario: Color policy remains specialized
- **WHEN** the frozen color-policy validator expects preserved source color or an explicit BT.709 normalization
- **THEN** the Fidelity report MAY reference that validator's result, but a Frame Fidelity fallback SHALL NOT infer a competing color expectation from filters, notes, or codec labels

### Requirement: Execution artifacts SHALL prove same-execution identity before outranking snapshots

An AS-IS execution artifact SHALL outrank the Queue `ProfileSnapshot` only when
it is complete and demonstrably tied to the validated frozen execution through
existing stable evidence: matching job/source identity plus an execution-plan,
embedded frozen-profile/snapshot, or attempt/revision identity that matches the
job's frozen execution. A merely latest artifact selected by job ID, media path,
or modification time SHALL be treated as ambiguous. When a verified artifact
and snapshot disagree, the artifact MAY provide the execution authority but the
disagreement SHALL be retained diagnostically; values SHALL NOT be silently
combined across executions. If identity cannot be proven, Fidelity SHALL fall
back to the frozen snapshot and SHALL NOT consult mutable Profile or Asset
Override state.

#### Scenario: Verified same-execution artifact outranks snapshot
- **WHEN** an AS-IS artifact matches the Queue job/source identity and its existing execution-plan or embedded frozen-profile identity matches the execution being validated
- **THEN** the artifact MAY supply the expected-output authority, any disagreement with `ProfileSnapshot` SHALL be reported diagnostically, and no mutable state SHALL be consulted

#### Scenario: Ambiguous latest artifact falls back to snapshot
- **WHEN** multiple AS-IS artifacts exist and the selected latest artifact matches only job ID/media path but has no matching execution-plan, frozen-profile, or attempt identity
- **THEN** the artifact SHALL NOT outrank `ProfileSnapshot`, the snapshot SHALL supply expectations, and the artifact ambiguity SHALL remain diagnostic

#### Scenario: Disagreeing executions are not merged
- **WHEN** an artifact is proven to belong to a different retry/execution than the validated job snapshot and its resolved geometry or Main10 decision differs
- **THEN** the evaluator SHALL ignore that artifact for expected-output authority, SHALL use the validated frozen snapshot, and SHALL NOT combine fields from both executions

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
restoration-setting, or Level/Tier recommendation edits. A same-execution
artifact may outrank the snapshot only under the identity rule above; an
ambiguous/stale artifact SHALL fall back to the snapshot. Preview and Test
Encode effective-profile handling SHALL expose the same resolved
expected-output inputs at their operation's execution boundary, so the future
evaluator can be reused without entry-point-specific policy. This change SHALL
NOT add LAB preview-first/background Fidelity orchestration owned by #19.

#### Scenario: Later Profile edit cannot change Queue expectations
- **WHEN** a Queue job is prepared with an IVTC, restoration, or Main10 plan and the Profile is edited before validation
- **THEN** Fidelity SHALL continue to use the job's frozen plan and SHALL produce the same expected values and authorities as before the edit

#### Scenario: Preview and Test Encode expose effective expectations
- **WHEN** Preview or Test Encode resolves a profile with cadence, geometry, or encoder changes
- **THEN** its inspection/effective-profile result SHALL carry the resolved values needed by the canonical evaluator, without re-deriving expectations from raw filter strings
