## Context

See `proposal.md` for the motivation and scope. This branch is based on the
current #2 implementation (`fix(video): repair residual combing in validated
IVTC`, now present at the branch base), so the design can consume the resolved
cadence/motion contract rather than infer cadence from rendered filters.

The audit found that `validateJobFrameFidelity` currently builds source and
output values with `firstVideoFrameCharacteristics`, then lets
`frameFidelityValue` compare them directly. Its `allowedChange` decision is
derived from `videoFilters` and `job.Notes`, including `crop=`, `scale=`,
`bwdif`, `fieldmatch`, `decimate`, and `fps=`. That is the root cause: a raw
command representation is being used as policy, so a correctly resolved
transformation can be reported as unexpected and a failed transformation can
look preserved.

There are already useful authorities to reuse:

- `firstVideoFrameCharacteristics` is the bounded raw ffprobe stream evidence;
  it already exposes geometry, SAR/DAR, average/base rates, field order,
  pixel format, bit depth, chroma location, and HEVC stream metadata.
- `validateCadenceOutputFrameRate` already gives the correct actual-FPS
  evidence precedence: measured frame count/duration, then `avg_frame_rate`,
  with `r_frame_rate` diagnostic-only.
- `validateSmartUpscaleOutput` and `validateRestorationOutput` already consume
  resolved geometry/aspect plans and compare them with actual output.
- `effectiveVideoDecision` exposes the effective encoder, pixel format,
  resolved cadence fields, progressive state, geometry, HEVC decision, color
  decision, Smart Upscale decision, and restoration plan.
- `resolvedRestorationPlanFromProfile` and `resolvedUpscaleDecisionFromProfile`
  parse persisted resolved plans rather than re-running their recommendations.
- `CaptureProfileSnapshot`/`RestoreProfileSnapshot` deep-freeze the effective
  profile, and Queue PREPARING replaces the job snapshot with the final
  resolved profile while preserving interlace/cadence analysis and
  recommendation evidence.
- HEVC validation already has a bounded bitstream evidence path; color-policy
  validation already owns color expectation. Neither should be duplicated.

The current Queue validation path restores the frozen profile but also reads a
latest `as-is` artifact and applies an override path while assembling Fidelity
inputs. The implementation must make the frozen-plan precedence explicit and
must not consult mutable current Profile or Asset Override state after the
prepared freeze. Preview inspection and Test Encode already carry effective
profiles/decisions at their execution boundaries; the evaluator should consume
those values rather than establish a new surface-specific resolver.

## Goals / Non-Goals

**Goals:**

- Establish one canonical expected-output model with per-field provenance.
- Distinguish raw storage characteristics from canonical/effective source
  characteristics without mutating source metadata.
- Make expected geometry, aspect, FPS, field structure, pixel format, bit depth,
  chroma location, and HEVC Level/Tier traceable to frozen authorities.
- Consume all #2 resolved cadence/motion fields, including TFF/BFF and
  residual-selective fallback semantics.
- Preserve existing specialized validators and reuse their evidence through
  adapters into the common model.
- Reuse existing probes and preserve the measured-FPS precedence without adding
  duplicate expensive work on the NAS.
- Provide additive, explainable report data that can later be consumed by #19.

**Non-Goals:**

- No production implementation in this planning change.
- No new cadence, restoration, Smart Upscale, color, or HEVC Level/Tier policy
  resolver.
- No source metadata rewrite, media conversion, perceptual PSNR/SSIM redesign,
  or new FFmpeg filter policy.
- No #19 LAB preview-first orchestration, background/sequential Fidelity
  windows, or global evidence cache.
- No OpenSpec archive or main-spec synchronization in this change.

## Decisions

### 1. Normalize one canonical source and one expected-output model

The implementation will introduce an internal, bounded model conceptually
equivalent to:

```text
CanonicalSource {
  rawStorage: FrameCharacteristics
  effective: FrameCharacteristics
  evidence: { known, unknown, unreliable, provenance }
}

ExpectedOutput {
  fields: map<FieldName, ExpectedField>
  planAuthority: string
  planIdentity/version: string
}

FieldResult {
  source: value
  expected: value
  actual: value
  status: passed | mismatch | unverified | not_applicable
  authority: string
  reason: string
  diagnostics: ...
}
```

The exact Go type and persisted envelope are implementation choices. The model
must preserve raw ffprobe values separately from effective values. A resolved
crop, scale, SAR, or cadence operation changes expected output; it never edits
the raw source record. Unknown values remain unknown. Canonicalization may
normalize equivalent rate/aspect spellings for comparison, but it must not
invent missing dimensions, field order, chroma location, or color metadata.

`codec` and `profile` remain optional evidence in this model. They are included
when existing frozen encoder evidence makes them meaningful, but they are not
added to the required comparison set through a new codec policy.

### 2. Build expectations by explicit authority precedence

The evaluator will resolve one immutable plan input before evaluating fields.
The precedence for Queue is:

1. A complete immutable prepared/execution artifact for the same job, when it
   contains the effective plan used to render the job.
2. The Queue `ProfileSnapshot` captured at PREPARING, including its frozen
   worker configuration and preserved analysis/recommendation evidence.
3. A compatibility adapter for an older frozen snapshot that lacks a newer
   optional field; it may use only values actually present in that snapshot.
4. Operation-local effective profile data for Preview or Test Encode at the
   final command-preparation boundary.

Mutable current Profile, current Asset Overrides, newly recomputed cadence
analysis, current restoration settings, and stale Level/Tier recommendations
are never fallback authorities for an already frozen Queue job. If two frozen
artifacts disagree, the implementation will prefer the artifact closest to the
rendered execution, record the conflict as plan-evidence diagnostics, and avoid
silently selecting a mutable value.

The expected-field adapters will use these owners:

| Field | Expected authority | Fallback/unknown rule |
| --- | --- | --- |
| Width/height | Complete resolved restoration output; otherwise resolved Smart Upscale target; otherwise effective video decision geometry | Preserve canonical effective source only when plan explicitly preserves geometry; otherwise unverified |
| SAR/DAR | Resolved restoration/upscale aspect decision, with DAR derived from resolved dimensions/SAR only when that decision is authoritative | Do not infer an intentional change from filters; missing expected aspect is unverified |
| FPS | `effectiveOutputFrameRate` from frozen cadence/motion resolution | Unknown/unreliable effective FPS is unverified; `r_frame_rate` cannot fill it |
| Field structure | Frozen `effectiveOutputProgressive` plus its reliability/evidence state; #2 field order is provenance | Explicit progressive expectation compares actual progressive/interlaced evidence; unresolved state is unverified |
| Pixel format/bit depth | Frozen effective encoder/profile decision (`pixFmt`, bit depth, Main10/10-bit semantics) | Preserve canonical source only under an explicit preserve decision; otherwise unverified |
| Chroma location | Canonical source under preserve/no-change authority | A known unexpected actual difference mismatches; no authoritative expectation is unverified |
| HEVC Level/Tier | Existing frozen HEVC decision and bitstream validator | Do not create a Level resolver or use average bitrate as peak evidence |
| Color | Existing color-policy validator | Frame Fidelity references its result and never reconstructs policy |

The restoration and Smart Upscale adapters must agree on the normalized output
geometry. A contradictory frozen plan is a plan-evidence problem and must be
reported as unresolved/mismatch diagnostics, not resolved by comparing source
dimensions or by inspecting filters.

### 3. Keep source-to-expected and expected-to-actual semantics separate

Every field will calculate two independent relationships:

```text
changedByPlan = source != expected
verdict       = expected == actual ? passed : mismatch
```

An implementation may continue emitting `changed_intentionally` for legacy
consumers, but it must derive that marker only from `source != expected` and
must never use it to make an actual mismatch pass. Existing field statuses such
as `preserved` and `changed_unexpectedly` will be mapped or retained as
compatibility data while the canonical result exposes the new status vocabulary.

At the report root, keep the existing `status` shape used by Queue validation
(`passed`/`warning`) for compatibility and add a canonical aggregate verdict.
The aggregate verdict is `mismatch` if any applicable field mismatches,
`unverified` if none mismatch but required evidence is unresolved, `passed` if
all applicable fields pass, and `not_applicable` if no covered field applies.
The root warning behavior can remain the existing consumer-facing mapping.

### 4. Consume #2 fields as semantics, not filter text

The plan adapter will copy these frozen fields into cadence provenance:

- `effectiveCadenceOperation`;
- `effectiveCadenceFieldOrder`;
- `effectiveResidualCombingFallback`;
- `effectiveOutputFrameRate`;
- `effectiveOutputProgressive` and its unreliable/unknown state;
- `effectiveAutomaticMotionFilters` as diagnostic provenance only.

For IVTC, the expected FPS and progressive field structure come from the
resolved values. TFF/BFF explains the selected cadence plan; it is not a reason
to compare output to raw source field order. Selective residual fallback is
reported as part of the plan and does not alter expected FPS. Filter strings and
`job.Notes` may be retained in diagnostics or used only when no frozen decision
exists, and even then they cannot override a frozen value.

### 5. Adapt specialized validators instead of collapsing them

Frame Fidelity will become an aggregator around existing policy boundaries:

- Smart Upscale and restoration reports supply normalized expected geometry,
  SAR/DAR, frame rate, and progressive evidence.
- The cadence validator supplies the measured-FPS comparison and diagnostic
  precedence.
- The HEVC validator supplies expected/effective Level/Tier and bounded actual
  bitstream evidence.
- The color-policy validator supplies color expected/actual and its verdict.

The common evaluator owns only the shared field-result shape and aggregate
semantics. Specialized validators retain their own policy details and remain
independently testable. This gives one expected-output model without a giant
replacement function or several competing resolvers.

### 6. Reuse one evidence collection boundary

`validateQueueJob` already collects source/output ffprobe data and the bounded
HEVC evidence used by validation. The implementation will pass or reference
those same evidence objects when constructing the canonical source and actual
output. Measured FPS will continue to use frame count/duration first, then
`avg_frame_rate`, with `r_frame_rate` diagnostic-only. No duplicate ffprobe,
frame-count, or HEVC bitstream probe will be introduced for a Fidelity report
refresh.

Actual field normalization will map ffprobe field-order spellings to a bounded
progressive/interlaced/unknown representation, derive bit depth only through
the existing pixel-format mapping, and retain raw values for diagnostics.

### 7. Make Queue freezing and operation parity explicit

Queue's PREPARING freeze is the immutability boundary. The implementation will
ensure the canonical evaluator reads the complete frozen plan from that boundary
and not a live model. The existing profile snapshot schema remains readable;
new optional expected-field data must be additive or derived from existing
frozen data, with a compatibility path that reports missing authorities as
unverified.

Preview and Test Encode will pass their final effective profile/decision through
the same plan adapter used by Queue. They may use operation-local immutable
inputs because they are not Queue lifecycle records, but they cannot decide
intent from UI labels or raw filter text. This makes the evaluator reusable by
#19 without implementing #19's scheduling and evidence-window orchestration.

### 8. Evolve the report additively

The report will retain existing root/source/output and specialized report
locations where consumers depend on them. It will add a canonical expected
output section and per-field results containing at least:

```json
{
  "source": "30000/1001",
  "expected": "24000/1001",
  "actual": "24000/1001",
  "status": "passed",
  "authority": "resolved_cadence",
  "reason": "IVTC resolved output rate"
}
```

Names may follow existing API conventions, but the information and semantics
are stable. Frontend consumers can continue using the old report while new UI
work renders Source / Expected by resolved plan / Actual output / Verdict /
Authority and reason. No frontend redesign is required in #3.

## Risks / Trade-offs

- [Risk] Older Queue snapshots do not contain every new effective field. → Use
  additive decoding, preserve raw evidence, and report `unverified` rather
  than reconstructing policy from filters or current mutable records.
- [Risk] Restoration, Smart Upscale, and effective video decisions can expose
  overlapping geometry. → Normalize through one adapter, define the precedence
  above, and report contradictory frozen plans instead of silently choosing
  source dimensions.
- [Risk] ffprobe container rates disagree with measured output. → Preserve all
  evidence, use the existing measured/average/base precedence, and expose the
  disagreement diagnostically.
- [Risk] A complete artifact and ProfileSnapshot disagree. → Prefer the frozen
  artifact closest to rendered execution, record the conflict, and never fall
  back to live Profile state.
- [Risk] Re-probing output for each field increases NAS load. → Construct all
  actual fields from the existing bounded probe/evidence collection.
- [Risk] Changing field status strings breaks a consumer. → Keep legacy report
  keys/status aliases where practical and add the canonical result structure;
  update focused tests before changing any consumer assumptions.

## Migration Plan

1. Implement the canonical source/expected/actual adapters and authority
   precedence behind the existing validation entry point.
2. Adapt specialized validators and #2 cadence fields, then replace only the
   intent-by-filter-string decisions that have authoritative frozen values.
3. Add focused backend regression coverage for the acceptance matrix and Queue
   snapshot immutability, followed by Preview/Test Encode effective-plan parity.
4. Run `openspec validate frame-fidelity-canonical-plan --type change --strict`
   and the repository's canonical implementation verifier when production code
   is later implemented.

Rollback is a code revert only. No source metadata, media, database migration,
or persisted Queue snapshot rewrite is required by this planning change.

## Open Questions

- During implementation, which existing immutable artifact envelope can carry a
  complete plan identity without adding a new persistence subsystem? The
  external precedence and fallback behavior are already fixed here; choosing
  between the existing job artifact profile and the Queue snapshot is an
  internal storage detail.
- Should the frontend type the additive canonical field map immediately or
  consume the existing report shape until a dedicated Fidelity UI change? This
  does not change backend semantics and is intentionally left to the consumer
  migration task.
