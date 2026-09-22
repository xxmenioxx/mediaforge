## Context

The current video path already carries cadence settings and resolved worker
metadata such as `effectiveOutputFrameRate`, `effectiveOutputProgressive`,
`effectiveOutputFrameRateUnknown`, and cadence warnings. FFmpeg command
construction can render `fieldmatch`/`decimate`, while validation reads both
stream metadata and measured output evidence. The gap is that requested cadence
labels, rendered filters, effective metadata, and validation expectations are
not yet one explicitly enforced contract. See `proposal.md` and
`specs/cadence/spec.md` for the observable requirements.

## Goals / Non-Goals

**Goals:**

- Make the resolved motion plan the single source of effective FPS and
  progressive-output truth.
- Make explicit IVTC field-order selection and canonical rendering observable
  before a 24000/1001 progressive result is accepted.
- Preserve validated automatic IVTC and conservative review behavior.
- Align validation evidence with the meaning of `avg_frame_rate`,
  `r_frame_rate`, and measured frame count/duration.
- Ensure Preview, Test Encode, and Queue consume equivalent frozen cadence
  intent.

**Non-Goals:**

- No cadence detector threshold or classification redesign.
- No HEVC Level/Tier work, GOP redesign, or Profile Lab refactor.
- No new cadence resolver, FFmpeg rendering path, validation subsystem, or
  operation/job model.

## Decisions

### 1. Resolve cadence metadata after the motion plan is renderable

The canonical cadence resolution boundary will produce both the rendered motion
operations and their effective metadata as one result. The result will derive
the effective FPS from the actual FPS/cadence filters that will be rendered,
with reliable analysis evidence used only where the plan itself cannot fully
describe the result. Requested cadence mode remains intent, not proof.

The existing resolved worker fields remain the compatibility surface. Unknown or
unreliable values will continue to use the existing unknown flags and warning
fields instead of fabricating a rate. This keeps downstream worker, HEVC-policy,
and validation consumers compatible while making their provenance explicit.

**Alternative considered:** Set `effectiveOutputFrameRate` directly from the
requested mode. Rejected because a mode label does not prove a field-order
choice, rendered filter chain, or actual output rate.

### 2. Resolve field order before explicit IVTC rendering

Explicit inverse telecine will use one precedence function: a valid explicit
`cadenceFieldOrder`, then validated detected field order, then the current
conservative fallback. The selected order is passed into the canonical
fieldmatch+decimate render construction. The result cannot claim progressive
24000/1001 until that chain is present in the resolved plan.

Invalid or unvalidated detection will not be upgraded by a UI label or by an
implicit guess. Existing automatic IVTC output is preserved when its analysis
has already validated the field order and operation.

**Alternative considered:** Let FFmpeg or each entry point infer field order at
render time. Rejected because it would make Preview, Test Encode, and Queue
diverge and would make effective metadata unavailable before execution.

### 3. Treat progressive output as an evidence-backed state

The resolver will set `effectiveOutputProgressive` only from recognized
progressive-producing processing or reliable analysis evidence associated with
the resolved source/plan. The existing unreliable/unknown representation stays
available for incomplete evidence. A cadence operation name such as
`inverse_telecine` is not sufficient on its own.

Suspected telecine remains a review-only result. No new detection threshold is
introduced; this change only prevents an unvalidated classification from being
treated as a completed normalization decision.

### 4. Keep soft-telecine removal internally consistent

Soft-telecine removal will continue to use the existing FPS filter path. Its
resolved effective FPS is recorded from the same target that is rendered by the
filter, so the command and metadata cannot describe different rates. This does
not alter the existing decision to remove or preserve soft telecine.

### 5. Use explicit evidence roles during validation

Validation will retain all available rate evidence but assign it stable roles:
`avg_frame_rate` is the primary stream-rate observation, `r_frame_rate` is
corroborating/base-rate metadata, and frame count divided by duration is the
stronger measured observation when both values are reliable. The expected
effective rate comes from the frozen resolved motion plan. Conflicts produce a
bounded warning/unknown result rather than silently choosing a contradictory
value.

This keeps diagnostics useful for container metadata while preventing
`r_frame_rate` from falsely validating a conversion that did not render the
expected rate.

### 6. Reuse the frozen cross-entry-point plan

Preview, Test Encode, and Queue will continue to use the existing canonical
resolution boundary. The required integration coverage will construct
identical frozen source analysis and cadence inputs for each surface and compare
the resolved operation, field order, effective FPS, progressive evidence, and
review state. Queue remains responsible for freezing the result before
processing; workers do not reread mutable cadence settings.

**Alternative considered:** Add surface-specific repairs in Preview,
Test Encode, or Queue. Rejected because that would create policy drift and a
second cadence decision path.

### 7. Preserve compatibility and safety defaults

Existing validated automatic IVTC, conservative fallback behavior, FFmpeg
command generation outside cadence, and unrelated encoder policies remain
unchanged. Missing or unsupported analysis must not cause BT.709-like implicit
normalization of cadence, a false progressive state, or a fabricated exact FPS.
No persisted migration is expected because the change tightens interpretation
of resolved metadata and preserves current settings and plan shapes.

## Risks / Trade-offs

- [Risk] Some existing profiles contain effective metadata produced by older
  resolution behavior. → Preserve existing fields and unknown/warning states,
  and add focused compatibility tests before changing consumers.
- [Risk] Container rates can disagree with measured output. → Keep all bounded
  evidence, assign primary/corroborating/measured roles, and fail validation
  conservatively on unresolved conflicts.
- [Risk] Explicit IVTC without a validated order may produce a less automated
  result. → Prefer review-only behavior to publishing a falsely normalized
  progressive output.
- [Risk] Recomputing cadence across entry points could add NAS analysis work. →
  Reuse the existing frozen analysis and resolved plan rather than introducing
  additional probes or render passes.

## Migration Plan

1. Add focused resolver and render-plan regressions for explicit field order,
   effective FPS, progressive evidence, automatic IVTC, review-only telecine,
   and soft-telecine consistency.
2. Align validation expectations with the resolved plan and the evidence roles
   without changing unrelated validation policies.
3. Add Preview/Test Encode/Queue parity coverage using identical frozen inputs.
4. Run the canonical verifier and retain existing profiles/settings unchanged.

Rollback is limited to reverting the cadence-resolution and validation changes;
no data migration or media rewrite is required. Existing source media and
published outputs remain untouched.

## Open Questions

None. The required precedence, evidence hierarchy, compatibility boundary, and
non-goals are fixed by the issue scope and specification.
