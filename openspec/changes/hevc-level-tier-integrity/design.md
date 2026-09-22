## Context

The existing backend already has a single `resolveHEVCLevel` path used by the
effective media profile, QSV command arguments, x265 parameter construction,
and `effectiveVideoDecision`. It records effective geometry/FPS and a Main-Tier
recommendation, but the current recommendation passes a zero bitrate, QSV
already renders bounded `target`/`maxrate`/`bufsize` values later in the plan,
and Fidelity currently treats ffprobe stream `bit_rate` as a bitrate check while
returning no observed Tier.

The repository also has an FFmpeg `trace_headers` path used for bounded
bitstream-structure analysis. Its parser currently extracts frame-structure
fields, not `general_level_idc` or `general_tier_flag`. This follow-up tightens
the contract around that existing resolver and evidence boundary; it does not
implement application code.

## Goals / Non-Goals

**Goals:**

- Make effective output requirements, resolved Level/Tier, command arguments,
  output evidence, validation, and Fidelity one traceable chain.
- Make x265 enforcement and stale Recommended behavior deterministic.
- Use one precise evidence-status vocabulary across validation and Fidelity.
- Run bounded output inspection at most once per output validation and reuse the
  stored result through the existing report/job-artifact boundary.
- Include only rate constraints that are genuinely bounded in the rendered
  command, including QSV VBR/CBR behavior.

**Non-Goals:**

- Changing cadence detection or cadence resolution from issue #11.
- Redesigning GOP or frame-structure policy.
- Refactoring Profile Lab or introducing a second HEVC resolver.
- Changing unrelated encoder quality policy or inventing encoder capabilities.
- Treating Main Tier as High Tier, or silently switching to High Tier when a
  supported Main-Tier constraint cannot be satisfied.
- Introducing a new global cache or persistence subsystem for output evidence.

## Decisions

### 1. Extend the existing resolver with a complete frozen decision

`resolveHEVCLevel` remains the only policy authority. Its inputs are assembled
after effective motion/cadence and geometry decisions and after quality intent
has produced the effective encoder rate-control settings. The canonical frozen
decision records, at minimum:

- effective width, height, and FPS;
- selected Level and supported Tier;
- Auto, Recommended, or explicit intent;
- the effective bounded maxrate/VBV constraint and its source, or an explicit
  unavailable reason;
- limiting factor and compatibility status;
- x265 enforcement mode and every maxrate/buffer/VBV consequence;
- an input fingerprint covering geometry, FPS, encoder, rate-control mode,
  target/maxrate/buffer and other values that can change the resolution.

Command builders, validation, and Fidelity consume this frozen decision. They
do not recalculate Level/Tier independently. The recommendation object can
carry the additional evidence, but a second HEVC resolver is not introduced.

### 2. Use effective bounded rate evidence, not source bitrate

The resolver consumes the effective output constraint, not the source stream's
average bitrate. The rate input is selected as follows:

1. Use an applied maxrate or equivalent VBV/HRD ceiling when one exists.
2. For QSV VBR/CBR, use the values that the same plan will render. A target
   contributes to Level MaxBR only when the effective command also uses it as
   maxrate; a VBR target by itself is not a peak bound.
3. Treat CRF, ICQ, and other unbounded quality modes as having no known peak
   constraint unless an explicit VBV/maxrate policy exists.
4. Never substitute ffprobe source or output average `bit_rate` for a peak
   constraint.

The resolver chooses the lowest supported Main-Tier level satisfying picture
size, luma sample rate, and the selected bounded rate. If the requirements
cannot fit the supported Main-Tier table, it returns a visible incompatible or
unresolved decision; it does not infer High Tier.

### 3. Make Auto, Recommended, and explicit intent deterministic

Auto resolves from current effective inputs. Recommended is a derived result
with an input fingerprint and follows exactly this state machine:

1. Matching fingerprint: reuse the stored recommendation.
2. Changed fingerprint with sufficient current geometry, FPS, and applicable
   rate-control inputs: synchronously recompute before command planning and
   replace the stale recommendation.
3. Changed fingerprint with insufficient current inputs: mark the decision
   unresolved, never use the stale recommendation as authority, and emit no
   Level/Tier enforcement from it.

Explicit Level is separate user intent. It is never replaced by Recommended
data, silently up-leveled, or treated as a stored recommendation. Explicit
Level always undergoes compatibility preflight; an incompatible request
prevents a valid command plan.

Preview and Test Encode freeze the decision at their final command-preparation
boundary. Queue freezes it at PREPARING after current effective configuration,
analysis, and required overrides are resolved. The Queue snapshot, command,
output inspection, final validation, and Fidelity all use that frozen decision.
Changes after the freeze affect a later operation only.

### 4. Make x265 enforcement deterministic

The x265 enforcement state is resolved before command rendering using exactly
these rules:

- Auto or Recommended with no real bounded peak-rate constraint uses advisory
  enforcement. The command does not inject `level-idc` or `high-tier`, so CRF/
  ICQ quality semantics remain unbounded.
- Auto or Recommended with a real bounded maxrate/VBV constraint uses hard
  enforcement after compatibility preflight. Every maxrate, buffer, and VBV
  consequence of that enforcement is already represented in the canonical
  frozen decision before the command is rendered.
- Explicit Level uses hard enforcement. Compatibility failure prevents a valid
  command plan; the implementation never silently up-levels it. Any x265
  maxrate, buffer, or VBV consequence is recorded in the frozen decision before
  rendering.

QSV receives the resolved Level/Tier command arguments and the same bounded
rate-control values used for planning. This policy is preferred over allowing
the command builder to decide whether `level-idc` is safe, because x265 can
derive or enable VBV constraints from Level and that decision must remain
visible and immutable.

### 5. Use precise evidence-status semantics

For applicable HEVC validation and Fidelity fields, the status vocabulary is
exactly:

- `validated`: authoritative evidence exists and matches the claim.
- `mismatch`: authoritative evidence exists and differs from the claim.
- `unavailable`: authoritative evidence could not be obtained, including an
  unsupported runtime, missing or invalid parameter-set evidence, a legacy
  output with no stored evidence, or a failed bounded probe.
- `unverified`: evidence is available but insufficient to prove the claim, such
  as ffprobe average `bit_rate` for MaxBR/CPB conformance.

Mismatch is never inferred from insufficient evidence. A missing authoritative
Level/Tier header is `unavailable`; an average bitrate that cannot prove a
peak-rate claim is `unverified`. Existing non-HEVC `not_applicable` behavior
remains outside this HEVC evidence vocabulary.

### 6. Require bounded bitstream-header evidence for final Level/Tier claims

Plain ffprobe stream fields are useful for codec/geometry and can provide a
normalized Level cross-check, but they are not the authoritative source for
both `general_level_idc` and `general_tier_flag`. Ordinary ffprobe metadata does
not reliably expose Tier, and average `bit_rate` does not represent HEVC
MaxBR/CPB peak behavior.

Final output evidence uses a bounded FFmpeg `trace_headers` inspection of the
selected video stream to extract `general_level_idc` and
`general_tier_flag`. The implementation reuses the existing trace-header
execution and parser boundary, extends its evidence type, and uses argument
arrays with a context timeout and bounded stderr/output handling. It inspects
only the header/parameter-set evidence needed for the decision, not a decode or
transcode. A runtime without a usable filter or a stream without parseable
headers produces `unavailable` authoritative Level/Tier evidence.

Peak-rate status is separate from Level/Tier header status. A configured
maxrate or VBV/HRD value proves the planned bound, not necessarily the actual
encoded peak. Actual peak conformance is `validated` only when output evidence
specifically represents a peak/CPB/HRD bound; ffprobe average bitrate remains
informational and produces `unverified` for that claim.

### 7. Run and reuse output evidence once per validation

For each applicable HEVC output validation, the bounded `trace_headers` probe
runs at most once. Its parsed Level/Tier evidence, probe outcome, and evidence
status are stored in the existing validation report or job-artifact boundary
used by the operation. Fidelity and report refreshes consume that stored record
and do not rerun FFmpeg merely to render the report. No new global cache or
persistence subsystem is introduced unless implementation proves the existing
boundary cannot store the result.

The existing frame-structure `trace_headers` consumers can share the low-level
parser/probe without becoming another Level/Tier policy owner.

### 8. Compare Fidelity against the frozen plan

The final Fidelity field receives the frozen expected Level/Tier and the stored
output evidence record. It reports expected and actual values independently,
with the exact status vocabulary above for Level, Tier, and rate constraint. A
missing actual Tier is not filled from expected Main Tier. The overall result
can be `validated` only when required authoritative evidence exists and matches;
authoritative differing evidence is `mismatch`; missing authoritative evidence
is `unavailable`; insufficient evidence is `unverified`.

Validation can use the existing job artifact/profile snapshot selection to get
the effective worker configuration, but it does not call a fresh resolver that
can reinterpret the output after the execution freeze.

## Risks / Trade-offs

- [Header probe is unavailable on some FFmpeg builds] → Mark authoritative
  Level/Tier evidence `unavailable` and expose the capability limitation
  instead of inferring a pass.
- [A bounded header probe still consumes runtime resources] → Inspect one video
  stream, stop after required parameter-set evidence, use a context deadline,
  cap captured diagnostic output, and run it at most once per output
  validation.
- [x265 hard Level enforcement can reduce quality] → Use advisory mode exactly
  for unbounded Auto/Recommended decisions and record every hard-enforcement
  rate-control consequence before rendering.
- [Existing stored recommendations lack an input fingerprint] → Treat them as
  changed/insufficient and unresolved until current inputs are sufficient for a
  synchronous recomputation; never use the old value as authority.
- [Older validation records lack actual header evidence] → Preserve them and
  report affected comparisons as `unavailable`.
- [A configured maxrate does not prove actual peak output] → Keep planned
  constraint and observed peak evidence separate; average bitrate yields
  `unverified`, never `validated`, for MaxBR/CPB conformance.

## Migration Plan

No media, sidecar, or production database migration is required. New decision
and evidence fields are additive to profile snapshots, job artifacts, and
Fidelity responses. On first use, existing stored Recommended values are
treated as changed; they are synchronously recomputed when current inputs are
sufficient and otherwise remain unresolved. Existing outputs remain readable,
but fields without authoritative header or peak evidence report
`unavailable` or `unverified` according to the vocabulary above. Rollback is a
code rollback; older readers can ignore additive fields without rewriting source
media.
