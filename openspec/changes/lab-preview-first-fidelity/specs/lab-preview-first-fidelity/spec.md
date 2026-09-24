## Purpose

This capability makes Profile Lab's selected Process Video sample usable as
soon as its primary Compatible Preview artifact is ready, while a bounded
backend operation completes canonical Frame Fidelity and distributed visual
metrics without duplicating the primary encode.

## ADDED Requirements

### Requirement: The selected Process Video window SHALL be the primary Fidelity window

The system SHALL treat the selected asset, `Start`, `Seconds`, effective video
plan, and preview normalization as one primary Fidelity window. The primary
window SHALL retain a canonical Compatible Preview request identity and, once
generated, its artifact identity. Sample B playback, inspection, frame
metrics, canonical Frame Fidelity actual-output evidence, and comparison frames
SHALL all refer to that same primary artifact.

The system SHALL expose an explicit Preview Ready boundary. Preview Ready
means that the primary artifact is valid, complete, and usable for playback;
it SHALL NOT mean that all Fidelity windows or aggregation have completed.

#### Scenario: Primary preview becomes ready before Fidelity completes

- **WHEN** the primary Compatible Preview artifact becomes valid and a
  background Fidelity operation is still blocked on an additional-window
  barrier
- **THEN** Sample B SHALL become playable and the UI SHALL report Preview
  Ready while Fidelity remains independently reported as running

#### Scenario: Primary preview failure prevents preview readiness

- **WHEN** the primary Compatible Preview cannot be generated, published, or
  recovered as a valid artifact
- **THEN** Sample B SHALL not be reported Ready, the normal Compatible Preview
  failure/recovery semantics SHALL remain visible, and Fidelity SHALL not
  claim a usable primary window

#### Scenario: Playback and Fidelity use one primary artifact

- **WHEN** playback, inspection, metrics, Frame Fidelity, and comparison-frame
  consumers request the selected window
- **THEN** each consumer SHALL identify the same primary request and artifact,
  and no second generation for that source revision, time window, effective
  plan, and normalization SHALL be required

### Requirement: Fidelity SHALL be a bounded backend-owned operation

Starting Process Video SHALL create or attach to a bounded LAB Fidelity
operation after the primary preview is ready. The operation SHALL expose an
opaque ID, canonical state, current phase, primary request/artifact identity,
completed-window count, total-window count, current window start/duration,
partial window results, and bounded terminal diagnostics through the existing
MVForge route conventions.

The operation state SHALL distinguish at least `queued`, `running`,
`completed`, `failed`, and `canceled`. Its phases SHALL distinguish primary
evidence, additional-window generation/evidence, and aggregation. Preview
Ready SHALL remain a separate state from operation completion.

#### Scenario: Starting Fidelity does not block a ready preview

- **WHEN** the primary artifact is already usable and operation creation or
  background capacity acquisition is delayed
- **THEN** Sample B SHALL remain Ready and the UI SHALL not wait for the
  Fidelity start response before enabling playback

#### Scenario: Status reports bounded incremental progress

- **WHEN** the operation has completed the primary window and is processing a
  later window
- **THEN** status SHALL expose the completed count, total count, phase, and
  current time window without returning unbounded FFmpeg logs

#### Scenario: Incomplete results are labeled as incomplete

- **WHEN** one or more windows are complete but additional Fidelity work is
  still running or has not produced a usable result
- **THEN** completed window results MAY be displayed, but the aggregate SHALL
  remain explicitly in progress or incomplete and SHALL NOT be presented as a
  final Safe or Reject result

### Requirement: Additional windows SHALL use canonical Compatible Preview identity and deterministic deduplication

The primary window SHALL be placed first in the operation's ordered window
set. Additional windows SHALL be derived from the existing LAB sampling policy
without changing its configured thresholds or default positions. Each
additional window SHALL use Compatible Preview request/artifact identity and
the frozen plan from the operation; raw ad hoc FFmpeg execution SHALL not
replace that lifecycle.

Window deduplication SHALL compare canonical media-time intervals after source
duration clamping and request normalization. A sampling position alone SHALL
not determine identity. A policy window whose normalized start and end are
equivalent to the primary interval within the source timebase tolerance SHALL
be omitted as a duplicate; a merely nearby but materially different interval
SHALL remain an additional observation.

#### Scenario: The selected window coincides with a policy position

- **WHEN** the selected primary interval is equivalent to the interval derived
  from the configured 50% sampling position
- **THEN** the primary window SHALL occupy the first observation and the 50%
  policy window SHALL not create another Compatible Preview artifact

#### Scenario: A different interval is retained

- **WHEN** a policy position produces a clamped interval with a materially
  different start or end from the selected primary interval
- **THEN** the policy interval SHALL remain as one additional window even if
  its floating-point position is close to the primary position

#### Scenario: Repeated identical Process Video intent reuses artifacts

- **WHEN** the same source revision, time window, effective plan, and
  normalization are requested again
- **THEN** Compatible Preview SHALL return or join the existing canonical
  request/artifact identity according to #1 and #14, without treating the LAB
  operation ID as a new encode identity

### Requirement: Heavy additional Fidelity work SHALL be sequential and capacity-bounded

For one operation, additional window generation and its required heavy
evidence work SHALL complete before the next additional window begins. Heavy
work includes Compatible Preview encoding, source/output frame extraction,
SSIM/PSNR FFmpeg comparisons, frame-structure probing, and specialized
bitstream probing such as HEVC `trace_headers`. The frontend SHALL not use a
promise fan-out that starts multiple such windows concurrently.

The backend SHALL use a small process-local capacity guard for concurrent LAB
Fidelity heavy work when no existing shared media-analysis capacity primitive
is available. The guard SHALL be bounded and operation-scoped, not a general
scheduler or Queue replacement; its initial policy SHALL allow at most one
additional heavy window across concurrent LAB Fidelity operations.

#### Scenario: One operation never overlaps additional heavy windows

- **WHEN** an operation processes additional windows 2 through N
- **THEN** the maximum observed number of concurrent additional heavy-window
  executions for that operation SHALL be one

#### Scenario: Concurrent LAB operations respect bounded capacity

- **WHEN** two LAB Fidelity operations request additional heavy work at the
  same time
- **THEN** one operation SHALL wait in a bounded queued/capacity phase while
  the capacity guard prevents concurrent additional heavy-window generation,
  and neither operation SHALL create a parallel NAS encode fan-out

### Requirement: Compatible structured evidence SHALL be reusable by immutable artifact identity

The system SHALL provide a bounded evidence view for each ready Compatible
Preview artifact. Evidence reuse SHALL be keyed by immutable artifact identity
and, where required to distinguish replacement attempts, its generation token
or version. It SHALL NOT rely only on a mutable path string, request ID, or
Profile ID. Replacing or retrying an artifact generation SHALL make prior
generation evidence ineligible for the new artifact generation.

Where the evidence boundary supports it, one shareable result SHALL serve
playback metadata, inspection, metrics preparation, canonical Frame Fidelity,
HEVC validation, frame-structure consumers, and comparison-frame generation.
The result SHALL avoid copying large raw probes into every operation response;
responses MAY expose bounded summaries and evidence references instead.

Before reading a ready artifact file or deriving structured probe, frame,
metric, or HEVC evidence from it, the operation SHALL acquire the existing #1
serving/analysis consumer protection, or the canonical equivalent exposed by
#1. It SHALL release that protection when the bounded evidence read completes,
fails, or is canceled. This protection SHALL only prevent cleanup from racing
an active read: it SHALL not create a second artifact lease, a generation
consumer for completed media, a new generating state, or indefinite retention.
Playback serving protection and Fidelity analysis protection for the same ready
artifact SHALL be independent.

#### Scenario: Multiple consumers reuse one artifact probe

- **WHEN** inspection, metrics preparation, and canonical Fidelity consume one
  ready artifact in one operation
- **THEN** the compatible structured probe for that artifact generation SHALL
  be obtained once and reused by those consumers where the evidence is
  sufficient

#### Scenario: Replaced generation cannot reuse stale evidence

- **WHEN** a failed or stale artifact is regenerated with a new generation
  token/version
- **THEN** evidence keyed to the old artifact generation SHALL not be used to
  report the new output

#### Scenario: Specialized bitstream evidence remains distinct

- **WHEN** HEVC Level/Tier or another policy specifically requires bounded
  bitstream evidence from `trace_headers`
- **THEN** the operation SHALL retain that specialized evidence as a separate
  source and SHALL not claim that generic ffprobe stream metadata replaced it

#### Scenario: Primary evidence protection prevents cleanup races

- **WHEN** the primary artifact is Ready, Fidelity acquires its analysis
  protection, and artifact cleanup runs during the bounded evidence read
- **THEN** the artifact SHALL remain available for that read; after the
  protection is released, later cleanup SHALL be allowed to apply its normal
  #1/#14 retention policy

#### Scenario: Cancellation releases analysis protection

- **WHEN** Fidelity is canceled while a primary or additional artifact evidence
  read holds the LAB analysis protection
- **THEN** the operation SHALL release that protection, SHALL not force artifact
  deletion, and SHALL leave the already-ready primary Preview usable

#### Scenario: Shared playback and Fidelity protections are independent

- **WHEN** playback holds a serving protection and Fidelity holds an analysis
  protection for the same ready artifact
- **THEN** Fidelity completion or cancellation SHALL release only its own
  protection and playback SHALL remain valid under its independent #1
  protection

### Requirement: LAB SHALL consume canonical #3 Frame Fidelity semantics

For every completed window, LAB SHALL consume the merged #3 canonical
evaluator using the frozen effective plan, canonical source evidence, expected
output, actual output evidence, and any existing specialized evidence. LAB
SHALL expose the same field-level semantics: Source, Expected, Actual, Verdict,
Authority, and Reason, with canonical statuses such as passed, mismatch,
unverified, and not applicable.

LAB SHALL not recreate geometry intent, cadence intent, pixel semantics,
HEVC policy, color policy, preserve authority, or unresolved authority. The
frozen plan used by an operation SHALL remain authoritative even if the
Profile is edited afterward.

#### Scenario: High SSIM does not hide a canonical mismatch

- **WHEN** a window has excellent SSIM and PSNR but canonical #3 reports an
  applicable output FPS mismatch
- **THEN** the FPS mismatch SHALL remain visible with its canonical authority
  and reason, and the combined presentation SHALL not label the window or
  overall result Safe solely because of the visual metric score

#### Scenario: Intentional transformation passes when actual output matches

- **WHEN** the frozen plan intentionally changes source cadence or geometry and
  the actual output matches the canonical expected values
- **THEN** the canonical field verdict SHALL pass, the intentional source-to-
  expected change SHALL remain explainable, and it SHALL not be reported as an
  output mismatch

#### Scenario: Missing authoritative evidence remains unverified

- **WHEN** a canonical field is applicable but source, expected, or actual
  evidence is insufficient
- **THEN** the field SHALL remain unverified rather than being inferred from
  mutable UI state or a non-authoritative probe

### Requirement: Visual metrics and canonical validation SHALL remain separate and honest

The system SHALL preserve the current `fidelityMetricVerdict`, average/minimum
SSIM, average/minimum PSNR, comparable-window count, worst-region reporting,
and existing Review/Safe/Reject thresholds unless a #3 canonical mismatch
requires an additional authoritative failure presentation. Visual metric
aggregation SHALL be separate from canonical plan/output validation.

An operation SHALL not publish a final visual aggregate until all required
windows are complete or explicitly recorded as failed/uncomparable. A real
canonical mismatch SHALL remain visible even when visual metrics are
comparable and above threshold. Infrastructure failure SHALL be represented as
operation/window failure, not mislabeled as a canonical mismatch.

#### Scenario: All complete windows retain existing metric semantics

- **WHEN** every required window has comparable SSIM/PSNR metrics and no
  canonical mismatch
- **THEN** the aggregate SHALL use the existing thresholds and report average,
  minimum, comparable count, and worst region without retuning them

#### Scenario: One uncomparable window prevents Safe

- **WHEN** an additional window cannot produce comparable metrics but other
  windows are successful
- **THEN** the operation SHALL preserve the window as uncomparable, continue
  according to its failure policy, and SHALL not report a final Safe aggregate

### Requirement: Backend SHALL own the Fidelity aggregate verdict

The backend LAB Fidelity operation SHALL calculate and publish the bounded
visual aggregate, canonical status, aggregate status, and terminal overall
verdict. The visual aggregate SHALL expose comparable count, total count,
average/minimum SSIM and PSNR, worst measured window/position, and the existing
metric verdict thresholds. The frontend SHALL render those fields without
recomputing aggregate comparability or canonical-versus-visual precedence.

#### Scenario: Backend Reject remains authoritative in the frontend

- **WHEN** the operation result reports `overallVerdict=Reject` while completed
  windows have high visual metrics
- **THEN** the Frame Fidelity UI SHALL display Reject and SHALL not derive Safe
  from the per-window metrics

#### Scenario: Partial aggregate has no final verdict

- **WHEN** the operation result has `aggregateStatus=analysis_in_progress`
- **THEN** the UI SHALL display analysis in progress and SHALL not display a
  final Safe or Reject aggregate

### Requirement: Cancellation SHALL be independent from a ready primary preview

Canceling a LAB Fidelity operation SHALL cancel only work owned by that
operation. It SHALL not invalidate, delete, or make unplayable a primary
Compatible Preview artifact that has already reached Preview Ready. Shared
additional artifacts SHALL follow #1 consumer/lease semantics; cancellation
SHALL not unconditionally kill a generation that has another consumer.

#### Scenario: Cancel after primary readiness

- **WHEN** Sample B is Ready, an additional Fidelity window is running, and
  the user cancels Fidelity
- **THEN** the operation SHALL become canceled, Sample B SHALL remain
  playable, and the primary artifact SHALL remain cache-valid

#### Scenario: Cancel does not remove another consumer's shared work

- **WHEN** an additional Compatible Preview artifact is shared with another
  valid consumer and the LAB operation is canceled
- **THEN** only the LAB-owned consumer SHALL be released and the shared
  generation SHALL continue under #1 rules

### Requirement: Retry SHALL reuse valid primary and additional artifacts

Retrying a failed or canceled operation SHALL reuse the same valid primary
request/artifact and any valid cached evidence. It SHALL recompute or continue
only missing, failed, stale, or invalid additional windows. A retry SHALL not
re-encode Sample B solely because Fidelity was canceled, interrupted, or
failed.

#### Scenario: Retry after cancellation does not re-encode Sample B

- **WHEN** Fidelity is canceled after the primary artifact and some additional
  windows are complete, then the user retries with the same material signature
- **THEN** the retry SHALL use the ready primary artifact, reuse valid evidence
  and completed additional artifacts, and start no new primary generation

#### Scenario: Retry rejects stale partial evidence

- **WHEN** a partial window artifact has been replaced or its source revision
  no longer matches the operation signature
- **THEN** the retry SHALL discard that stale evidence and use a new canonical
  Compatible Preview request/artifact identity for the affected window

### Requirement: Process-local operation persistence SHALL be explicit

LAB Fidelity operation metadata SHALL be bounded and process-local in v1. The
operation store SHALL expose terminal cleanup by age and/or maximum history,
using the existing bounded operation retention pattern where practical. Its
cleanup SHALL not delete canonical Compatible Preview artifacts; artifact
retention remains owned by #1/#14.

After a backend restart, a process-local operation SHALL not claim to remain
running or completed. A valid persistent primary request/artifact SHALL remain
recoverable through #14, and the user SHALL be able to start a new Fidelity
operation that reuses it.

#### Scenario: Restart preserves primary but not live operation ownership

- **WHEN** the backend restarts after the primary artifact is ready and while
  Fidelity is running
- **THEN** the old operation SHALL be absent or explicitly interrupted by the
  process-local contract, the primary artifact SHALL remain recoverable through
  #14, and a retry SHALL not require a primary re-encode

#### Scenario: Operation metadata cleanup is independent

- **WHEN** a completed, failed, or canceled operation exceeds the bounded
  retention policy
- **THEN** its metadata MAY be removed while its canonical Compatible Preview
  artifacts remain governed by their own #1/#14 retention policy

### Requirement: Material signatures and frozen plans SHALL protect current UI state

Each operation and each returned result SHALL be bound to a material signature
covering the source revision, asset, `Start`, `Seconds`, normalization, and
frozen effective video plan. A late status or result from an older signature
SHALL not become the current `lastFidelityInspection` for a newer signature.
Changing tabs SHALL only render already-collected results and SHALL not start
encoding, probing, or metrics work.

#### Scenario: Profile mutation cannot overwrite a newer run

- **WHEN** operation A starts, the user edits codec, quality, preset, filters,
  cadence, upscale, or another plan input, and operation A later completes
- **THEN** the UI SHALL ignore A for the current signature B unless the user
  explicitly selects the older run, and A's windows SHALL retain A's frozen
  expected plan

#### Scenario: Tab changes are read-only

- **WHEN** the user switches among Previews, Technical, Frames,
  Characteristics, and Warnings while Fidelity is running or complete
- **THEN** no additional heavy media generation or duplicate frame metrics
  operation SHALL start solely because of the tab change

### Requirement: Source reference caching SHALL be material-signature based

The source/reference preview and its reusable inspection/evidence SHALL remain
available when its material signature is unchanged. The signature SHALL
include the source revision, selected source time window, and normalization
decision; it SHALL not include the processed Profile object identity. A change
only to the processed profile SHALL not regenerate equivalent source/reference
evidence. A source revision, `Start`, `Seconds`, or normalization change SHALL
invalidate it.

#### Scenario: Profile-only edit reuses source reference

- **WHEN** the source, source interval, and normalization are unchanged but the
  processed video Profile changes
- **THEN** the source/reference request and valid evidence SHALL remain reusable
  while the processed primary window receives its new signature

#### Scenario: Source material change invalidates reference cache

- **WHEN** source revision, selected interval, or normalization changes
- **THEN** the previous reference cache SHALL not be attributed to the new run
  and a new source/reference request or evidence identity SHALL be used

### Requirement: The #15 integration SHALL be an adapter, not a duplicate lifecycle

The LAB orchestration SHALL own only primary readiness, the Preview Ready
boundary, and Fidelity operation orchestration. It SHALL depend on a thin
`ensure primary Compatible Preview ready` abstraction. On the current main
branch it SHALL be implementable using the existing registered request/resource
behavior and, after the primary artifact is valid, SHALL leave Sample B on the
existing Compatible Preview playback transport.

#15 SHALL own Compatible Preview transport/status evolution, including the
generating/status/lease lifecycle, native ready-media URLs, Range/seek
behavior, browser credential and Cookie policy including #13 interaction, and
native readiness-race recovery. When #15 provides its ready/generating/status/
lease contract, the #19 adapter MAY consume that contract and its native
ready-media behavior without changing LAB operation states.

LAB SHALL not add a second preview lease, generic preview status engine,
timeout policy, transport policy, `credentials: omit` policy, Cookie policy,
Range/seek policy, readiness-race recovery, or x265-specific lifecycle. A
`VideoPreviewRequest` transport change SHALL not be a #19 task unless it is
already provided by merged #15. Preview generation failures, recovery, and
shared ownership SHALL remain delegated to #1, #14, and #15's contract.

#### Scenario: Current main can run before #15 lands

- **WHEN** the application has #1/#14 but not #15's status helpers
- **THEN** Process Video SHALL wait for the primary artifact using the existing
  Compatible Preview behavior, mark Sample B Preview Ready, retain the current
  playback transport, and start backend Fidelity without requiring #15
  implementation

#### Scenario: #15 can replace the readiness/media adapter implementation

- **WHEN** #15 is available and reports generating followed by ready
- **THEN** LAB SHALL consume those states and the native ready-media contract
  through the thin adapter, preserve the same primary artifact and Fidelity
  operation identity, and SHALL not create another preview lifecycle or
  duplicate #15's readiness-race recovery

#### Scenario: #19 does not redefine Compatible Preview transport

- **WHEN** #19 is implemented before #15 or alongside a main branch that does
  not yet contain #15
- **THEN** #19 SHALL not change `VideoPreviewRequest`, Blob/native transport,
  `credentials: omit`, Cookie behavior, #13 isolation, Range/seek behavior,
  or native readiness-race handling; those changes SHALL remain outside #19

### Requirement: Failure classification SHALL preserve preview usability

Primary preview failure SHALL prevent Preview Ready and retain normal
Compatible Preview recovery behavior. A background Fidelity infrastructure
failure after Preview Ready SHALL leave Sample B playable and expose a
retryable Fidelity failure. A canonical validation mismatch SHALL be a result,
not an infrastructure failure. An additional-window metrics failure MAY be
recorded as uncomparable and continued according to the existing aggregation
philosophy, but it SHALL prevent a final Safe result.

#### Scenario: Background failure leaves Sample B usable

- **WHEN** the primary artifact is Ready and an additional generation,
  evidence, or operation capacity failure occurs
- **THEN** Sample B SHALL remain playable, Fidelity SHALL show a bounded
  retryable failure or incomplete window, and the user SHALL not be forced to
  discard the primary preview

#### Scenario: Canonical mismatch is not reported as infrastructure failure

- **WHEN** the operation completes evidence collection and #3 reports a real
  field mismatch
- **THEN** the operation SHALL retain the canonical mismatch result and SHALL
  not classify the completed evidence as an FFmpeg or operation failure
