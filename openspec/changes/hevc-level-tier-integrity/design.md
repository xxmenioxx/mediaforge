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
fields, not `general_level_idc` or `general_tier_flag`. This change should reuse
that evidence path and the existing canonical resolver rather than create a
parallel HEVC policy.

## Goals / Non-Goals

**Goals:**

- Make effective output requirements, resolved Level/Tier, command arguments,
  output evidence, validation, and Fidelity one traceable chain.
- Include only rate constraints that are genuinely bounded in the rendered
  command, including QSV VBR/CBR behavior.
- Preserve the distinction between planned intent, encoder enforcement, and
  observed bitstream evidence.
- Make stale recommendations and post-freeze configuration changes explicit.
- Keep header inspection bounded and suitable for a resource-constrained NAS.

**Non-Goals:**

- Changing cadence detection or cadence resolution from issue #11.
- Redesigning GOP or frame-structure policy.
- Refactoring Profile Lab or introducing a second HEVC resolver.
- Changing unrelated encoder quality policy or inventing encoder capabilities.
- Treating Main Tier as High Tier, or silently switching to High Tier when a
  supported Main-Tier constraint cannot be satisfied.

## Decisions

### 1. Extend the existing resolver with a complete immutable decision

`resolveHEVCLevel` remains the only policy authority. Its inputs are assembled
after effective motion/cadence and geometry decisions and after quality intent
has produced the effective encoder rate-control settings. The resulting
decision records, at minimum:

- effective width, height, and FPS;
- selected Level and supported Tier;
- whether the Level is Auto, Recommended, or explicit intent;
- bounded-rate value and source when present, or an unavailable/unverified
  reason;
- limiting factor and compatibility status;
- x265 enforcement mode (`hard` or `advisory`) and any rate-control impact;
- an input fingerprint covering geometry, FPS, encoder, rate-control mode,
  target/maxrate/buffer and other values that can change the resolution.

This preserves the current resolution order and prevents command builders or
Fidelity code from recomputing a different answer. The recommendation object
may evolve to carry this evidence, but the implementation must not add a
second resolver in command generation, validation, or React.

### 2. Use effective bounded rate evidence, not source bitrate

The resolver will consume the effective output constraint, not the source
stream's average bitrate. The rate input is selected as follows:

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

### 3. Treat Recommended as derived and freeze only prepared execution

Auto is recomputed from current effective inputs. Recommended is a persisted
derived result with an input fingerprint; a fingerprint mismatch causes
recalculation or an explicit stale state before command emission. Explicit
Level remains user intent and is not silently raised, but an incompatible
explicit request prevents a valid command plan from being emitted.

Preview and Test Encode freeze the decision at their final command-preparation
boundary. Queue freezes it at PREPARING after current effective configuration,
analysis, and required overrides are resolved. The Queue snapshot, command,
output inspection, final validation, and Fidelity all use that frozen decision.
Changes after the freeze affect a later operation only.

### 4. Make x265 enforcement semantics explicit

QSV continues to receive the resolved Level/Tier command arguments and the same
bounded rate-control values used for planning. For x265, `level-idc` and
`high-tier=0` are not treated as harmless metadata because x265 can derive or
enable VBV constraints from Level.

The plan therefore has an enforcement state:

- Auto/Recommended with no bounded peak-rate input are advisory for x265. The
  command does not inject `level-idc` merely to claim conformance, preserving
  CRF/ICQ semantics while allowing final evidence to be unverified or
  different.
- Auto/Recommended with a real bounded rate input may use hard enforcement when
  the bounded rate is part of the resolved plan.
- Explicit Level uses hard enforcement only as an explicit user conformance
  request, and the plan reports the resulting VBV/quality consequence.

This keeps one resolved decision while making the command's enforcement choice
observable instead of silently changing quality behavior.

### 5. Require bounded bitstream-header evidence for final Level/Tier claims

Plain ffprobe stream fields are useful for codec/geometry and may provide a
normalized Level cross-check, but they are not the authoritative source for
both `general_level_idc` and `general_tier_flag`. In particular, ordinary
ffprobe metadata does not reliably expose Tier, and average `bit_rate` does not
represent HEVC MaxBR/CPB peak behavior.

Final output evidence will use a bounded FFmpeg `trace_headers` inspection of
the selected video stream to extract `general_level_idc` and
`general_tier_flag`. The implementation should reuse the existing
trace-header execution and parser boundary, extend its evidence type, and use
argument arrays with a context timeout and bounded stderr/output handling. It
must inspect only the header/parameter-set evidence needed for the decision,
not decode or transcode the output. A runtime without a usable filter or a
stream without parseable headers yields unavailable/unverified evidence.

Peak-rate status is separate from Level/Tier header status. A configured
maxrate or VBV/HRD value proves the planned bound, not necessarily the actual
encoded peak. Actual peak conformance is validated only when output evidence
specifically represents a peak/CPB/HRD bound; ffprobe average bitrate remains
informational.

### 6. Compare Fidelity against the frozen plan, with evidence-aware states

The final Fidelity field receives the frozen expected Level/Tier and the output
evidence record. It reports expected and actual values independently, with
separate evidence-aware statuses for Level, Tier, and rate constraint where
needed. A missing actual Tier is not filled from the expected Main Tier. The
overall result can be `validated` only when the required evidence exists and
matches; differing evidence is `mismatch`; missing or unsupported evidence is
`unavailable`/`unverified`.

Validation may use the existing job artifact/profile snapshot selection to get
the effective worker configuration, but it must not call a fresh resolver that
can reinterpret the output after the execution freeze.

### 7. Reuse existing plan and evidence boundaries

The implementation should update the current effective profile path, QSV/x265
renderers, job-output validation, and Fidelity serialization in place. Header
inspection should be performed once per output validation and its parsed result
stored with the validation evidence or job artifact so repeated Fidelity views
do not rerun FFmpeg. Existing frame-structure `trace_headers` consumers may
share the low-level parser/probe without becoming another Level/Tier policy
owner.

## Risks / Trade-offs

- [Header probe is unavailable on some FFmpeg builds] → Keep expected values,
  mark actual Level/Tier unavailable/unverified, and expose the capability
  limitation instead of inferring a pass.
- [A bounded header probe still consumes runtime resources] → Inspect one video
  stream, stop after required parameter-set evidence, use a context deadline,
  cap captured diagnostic output, and cache evidence for the output/job.
- [x265 hard Level enforcement can reduce CRF quality] → Use advisory mode for
  unbounded Auto/Recommended decisions and surface hard enforcement plus its
  rate-control consequence for bounded or explicit decisions.
- [Existing stored recommendations lack an input fingerprint] → Treat them as
  stale on first resolution and recompute from current effective inputs; keep
  old records readable.
- [Older validation records lack actual header evidence] → Preserve them and
  report only the affected comparisons as unavailable/unverified.
- [A command's configured maxrate does not prove its actual peak output] → Keep
  planned constraint and observed peak evidence separate; never upgrade an
  average bitrate comparison to validated MaxBR conformance.

## Migration Plan

No media, sidecar, or production database migration is required. New decision
and evidence fields are additive to profile snapshots, job artifacts, and
Fidelity responses. On first use, existing stored Recommended values are
treated as stale when their inputs cannot be proven current. Existing outputs
remain validatable, but fields without header or peak evidence report
unavailable/unverified. Rollback is a code rollback; the additive fields can be
ignored by older readers without rewriting source media.
