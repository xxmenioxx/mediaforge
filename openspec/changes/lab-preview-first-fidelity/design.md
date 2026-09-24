## Context

See `proposal.md` for the motivation and contract. This design is based on
the branch base `d05a86b5af8e9c74365e4f36c96e5500fbeeb1c4`, which contains the
merged #3 canonical Frame Fidelity change and the #1/#14 Compatible Preview
lifecycle.

### Current Process Video sequence

The current `ProfileLabPage` flow is one React Query mutation. The observed
sequence is:

1. `Process Video` validates the asset and `Start`, aborts the previous
   Fidelity and video controllers, resets the inspection, and records the
   current profile/request signatures.
2. It registers a browser-compatible source/reference request on every run,
   even when `referencePreviewCacheRef` has the same frontend signature. The
   registration is the backend-authoritative source-revision validation seam;
   it is cheap and does not itself encode media. The returned `requestId` and
   `cacheIdentity` are compared with the cached request/artifact identity
   before any cached Sample A inspection is reused. It also creates one
   processed conversion request.
3. `onPrepared` stores both request IDs and mounts the processed video request.
   The current `VideoPreviewRequest` fetches the video response into a Blob;
   the backend resource call may be waiting for the artifact while the
   mutation continues.
4. The mutation inspects the uncached source/reference request, then loops
   through every position returned by `labFrameSamplingPolicy`. For each
   position it registers another processed request, awaits `inspect`, then
   awaits `metrics`. The loop is sequential after it starts, but it is still
   frontend-owned and can overlap the primary video fetch.
5. `inspect` currently probes source and output characteristics, frame
   structure, optional QSV context, and HEVC `trace_headers` evidence. The
   `metrics` endpoint calls `probePreviewCharacteristics` again for both paths
   before running SSIM and PSNR comparisons.
6. After all windows, the frontend chooses the representative middle
   inspection, aggregates frame structure and visual metrics, derives client
   QSV/HEVC presentation values, and only then publishes one
   `lastFidelityInspection`. `FrameFidelityComparison` reads those windows
   and computes the current visual aggregate from them.

The primary processed request from step 2 is not explicitly the first Fidelity
window. The sampling loop creates its own request for the policy positions, so
the selected `Start`/`Seconds` interval can be encoded again. A tab render can
also request frame URLs which perform frame extraction on demand. There is no
backend LAB operation identity, no explicit Preview Ready boundary, and no
shared structured evidence bundle spanning inspect and metrics.

### Existing ownership boundaries

- #1 owns Compatible Preview request identity, frozen descriptors, artifact
  identity, single-flight generation, cache/publication, and consumer/lease
  lifecycle.
- #14 persists non-ephemeral request/artifact relationships, source revision,
  generation token/version, and restart recovery. A valid ready artifact can
  survive a backend restart; a live encode cannot be resumed.
- #3 owns canonical source/expected/actual field semantics and specialized
  authority such as cadence, pixel semantics, color policy, and HEVC
  Level/Tier. The current evaluator is exposed internally through a Queue
  authority adapter; LAB needs a compatible-preview authority adapter, not a
  second comparison algorithm.
- #15 owns Compatible Preview transport/status evolution: generating/status/
  lease lifecycle, native ready-media URLs, Range/seek behavior, browser
  credential and Cookie policy including #13 interaction, and native
  readiness-race recovery. #15 is not required on this branch; its contract
  is a future adapter input, not a second LAB lifecycle.

## Goals / Non-Goals

**Goals:**

- Make the processed selected window the first and reusable Fidelity window.
- Publish Sample B readiness as soon as that artifact is valid and playable.
- Run post-readiness Fidelity in a bounded backend operation with deterministic
  progress, cancellation, retry, and partial-result behavior.
- Reuse #1/#14 identities and a bounded evidence view so playback, inspection,
  metrics, canonical validation, and frames do not create duplicate encodes or
  probes where the evidence is equivalent. While evidence is being read,
  acquire and release the existing #1 serving/analysis artifact protection;
  #19 does not create another lease or retention mechanism.
- Preserve visual thresholds and make canonical #3 mismatches authoritative
  and visible.
- Protect the NAS with sequential per-operation windows and one global
  process-local LAB Fidelity heavy-work slot initially.

**Non-Goals:**

- No Queue job, distributed worker, general scheduler, or general persistent
  operation framework.
- No replacement of #1/#14 request, artifact, lease, cache, publication, or
  persistence ownership.
- No Compatible Preview transport/status policy. #19 does not redefine
  `VideoPreviewRequest`, Blob/native media transport, credentials, Cookie,
  Range/seek, #13 isolation, or readiness-race recovery; those belong to #15.
- No live-operation persistence or process-resume behavior in v1.
- No changes to visual thresholds, encoder policy, cadence/GOP policy, or
  specialized #3 resolvers.
- No implementation in this planning change.

## Decisions

### 1. Split primary readiness from Fidelity orchestration

`Process Video` will create/resolve the primary Compatible Preview request for
the selected asset, `Start`, `Seconds`, normalization, and effective video
plan. A thin adapter named conceptually `ensureCompatiblePreviewReady` will
wait for the artifact to become ready.

The adapter has two implementations over time, while keeping transport
ownership outside #19:

- On this branch's current main, it uses the existing registered video
  resource behavior, whose request context waits for #1's artifact generation.
  Once the primary artifact is ready, Sample B is marked Preview Ready and
  continues using the existing Compatible Preview playback transport,
  including the current `VideoPreviewRequest` behavior.
- After #15 lands, the adapter can consume its ready/generating/status/lease
  contract and native ready-media behavior at a practical cadence. It must not
  copy #15's timeout, lease, status, credentials, Cookie, Range/seek, or
  readiness-race policy into LAB.

Only after the primary artifact is ready does the frontend mark Sample B
Ready. It then submits the primary request ID to the LAB operation API without
awaiting that POST for playback. Primary evidence and all additional windows
run after that boundary. Source/reference preview preparation may happen in
parallel and is not allowed to delay Sample B readiness.

Before #15 is merged, #19 does not change the playback transport: the ready
primary request continues through the existing Compatible Preview/
`VideoPreviewRequest` abstraction. After #15 is merged, the same adapter may
consume its native ready-media contract. In neither case does #19 redefine
Blob/native transport, `credentials: omit`, Cookie behavior, Range/seek
behavior, or native readiness-race recovery. A `VideoPreviewRequest` transport
change is outside this change unless it is already supplied by merged #15.

### 2. Use a bounded process-local LAB Fidelity operation

The backend will add a route family following the repository's existing
operation convention, conceptually:

```text
POST /api/lab/fidelity/operations
GET  /api/lab/fidelity/operations/:id
POST /api/lab/fidelity/operations/:id/cancel
```

The exact route names should be confirmed against the route package during
implementation. A start request contains only bounded identity and policy
inputs: the primary Compatible Preview request ID, an optional small UI
correlation signature, and bounded duration/policy inputs. The backend resolves
the frozen primary descriptor, source revision, effective plan, normalization,
artifact identity, generation token/version, and sampling policy from #1/#14
and the backend LAB resolver;
the request does not repeat an unbounded Profile or media-analysis payload.

The server rejects or reports a retryable `primary_not_ready` condition if the
primary artifact is not ready. This keeps the explicit Preview Ready boundary
and avoids an operation that silently owns the primary preview. A future
backend-coordinated start may resolve both steps, but it must still expose
primary readiness independently.

The operation snapshot is shaped conceptually as:

```text
LabFidelityOperation {
    id, state, phase, materialSignature, clientCorrelationSignature,
    primaryRequestId, primaryArtifactId, primaryGeneration,
    sourceRevision, normalizedInterval, normalization,
    canonicalEncodeSignature, samplingPolicyVersion,
    windows: bounded summaries/references,
    completedWindows, totalWindows,
    currentStart, currentDuration,
    partialResult, aggregateState,
    retryable, error, createdAt, updatedAt
}
```

Each window keeps position/start/duration, request/artifact identity, status,
metrics summary, canonical Fidelity summary/reference, and source/output
characteristics. Large raw probes live behind bounded evidence references or
the artifact evidence provider rather than being copied into every response.

States are `queued`, `running`, `completed`, `failed`, and `canceled`.
Phases are `primary_evidence`, `additional_window_encode`,
`additional_window_inspect`, `additional_window_metrics`, and `aggregate`.
The operation may expose completed window results while running; its aggregate
is `analysis_in_progress` until all required windows have a terminal result.

The implementation should reuse the existing Profile Sample Estimate operation
pattern: a mutex-protected in-memory store, context cancellation, a bounded
capacity channel, status snapshots, and age/max-history cleanup. It must not
turn that pattern into a general operation engine.

`materialSignature` is a backend-owned SHA-256 identity over bounded,
canonical descriptor facts: source path/revision, normalized primary interval,
normalization, canonical encode signature, artifact ID, generation token/
version, and the frozen sampling policy/version. A client correlation
signature may be echoed for UI diagnostics, but it cannot replace or alter the
backend material identity. After capacity acquisition, the runner re-resolves
the primary request and rejects a changed artifact/generation/source revision/
plan as retryable `stale_primary`; it never attributes generation B evidence
to an operation frozen for generation A.

### 3. Freeze a coherent primary and window plan

At operation start, the backend resolves and freezes:

- source revision and source path identity from the primary request;
- selected start/end interval and normalized duration;
- effective preview profile/plan and normalization;
- the current LAB sampling policy, including configured positions,
  `windowSeconds`, adaptive behavior, and policy version/signature.

The primary interval is the first window. Additional intervals are derived from
the frozen policy and clamped to the source duration using the same semantics as
the current `labFrameSamplingPolicy`. A backend-owned resolver should become
the authoritative implementation; the frontend may display its result but
must not calculate a second execution plan.

For each policy interval, compare canonical media-time `[start,end]` after
clamping and Compatible Preview request normalization. Treat intervals as
equivalent when both boundaries are within one source-frame timebase tolerance
(falling back to a fixed millisecond precision when measured FPS is
unavailable). Only equivalent intervals deduplicate. The primary is always
retained first; materially different overlapping windows remain additional
observations.

An additional request is prepared through the existing Compatible Preview
request service with the same frozen effective plan and its own canonical
time interval. Its resource/evidence calls acquire the #1 artifact; the LAB
runner never launches a raw FFmpeg command or creates a second cache key.

### 4. Make primary artifact reuse an explicit invariant

The primary request ID returned during preview preparation is stored in the
operation. The operation resolves its artifact ID and #14 generation token/
version before running primary evidence. Playback, inspect, metrics, canonical
Fidelity, and frame comparison all use those identities.

The implementation must instrument the canonical generation boundary, not just
count POST registrations. The invariant is one generation attempt for the
primary artifact identity; a ready cache hit or another #1 consumer may join
without an encode. If a generation is replaced, the new token/version is a
new evidence identity.

### 5. Add a bounded artifact evidence provider

The current inspect and metrics paths will be refactored behind a shared
evidence provider. A bounded view may contain:

```text
PreviewArtifactEvidence {
    artifactId
    generationToken/version
    structuredProbe
    sourceCharacteristics
    outputCharacteristics
    measuredFPS
    sourceFrameStructure
    outputFrameStructure
    hevcBitstreamEvidence
    frameReferences
    metricsBySourceNormalization
}
```

The key is `(artifactId, generationToken/version)` for output evidence. A
stream-global source probe is keyed by source path/revision only and is shared
across intervals and normalization modes. Interval-specific source evidence is
keyed by source revision plus canonical `[start,end]`; normalized frame/metric
evidence additionally includes normalization. Derived frame references and
metrics include their relevant source/normalization identity. The provider is process-local and bounded in v1;
it may use the durable #1/#14 artifact encoder evidence as an input. It is not
a second artifact cache and does not change artifact publication or retention.

Before reading any ready artifact file, structured probe, extracted frame,
metric input, or HEVC trace evidence, the provider acquires the existing #1
serving/analysis consumer protection, or the canonical equivalent exposed by
#1. The bounded evidence work then runs and releases that protection in a
`defer`-equivalent cleanup path, including cancellation and failure. This is
artifact use protection only: it does not create another artifact lease, turn
a ready artifact back into `generating`, create a generation consumer for
completed media, or extend retention indefinitely. Playback and Fidelity may
hold independent protections for the same ready artifact; releasing the LAB
protection cannot invalidate playback or another #1 consumer.

Evidence sharing boundaries are explicit:

| Evidence | Reusable by | Not a replacement for |
| --- | --- | --- |
| Structured ffprobe stream metadata | playback metadata, inspect, metrics geometry, #3 actual values | measured cadence/frame structure or bitstream headers |
| Measured frame count/duration/FPS | FPS validation and displays | exact stream metadata fields it did not measure |
| Frame-structure ffprobe interval evidence | inspect, #3 cadence/field consumers | stream-only ffprobe metadata |
| HEVC `trace_headers` result | #3 HEVC Level/Tier and technical views | generic ffprobe level/profile fields |
| Normalized source/output frame references | comparison UI and one metric calculation | another artifact generation |
| SSIM/PSNR result for source identity + artifact | aggregate and read-only tabs | canonical plan/output verdict |

`compatibleAssetFrameMetrics` must accept or obtain this evidence rather than
calling `probePreviewCharacteristics` for both inputs on every request. The
frame URL remains a read-only consumer of the artifact/evidence provider and
must not cause a new Compatible Preview encode. A cache miss may perform the
necessary frame extraction once, with its key tied to immutable identities.

### 6. Adapt the canonical #3 evaluator instead of rebuilding Fidelity

The #3 evaluator currently obtains authority through a Queue job snapshot and
verified execution artifact. The implementation will factor a shared authority
input or add a Compatible Preview adapter that supplies the same canonical
frozen profile/worker/resolved-video-decision values. The adapter will use the
primary/additional Compatible Preview descriptor and its frozen effective plan;
it will not read the mutable current Profile.

Each window passes canonical source evidence, the frozen authority, output
structured evidence, and existing specialized evidence into the #3 evaluator.
The result is stored by reference/summary in the operation and rendered with
`Source`, `Expected`, `Actual`, `Verdict`, `Authority`, and `Reason` fields.
No LAB-specific geometry, cadence, pixel-format alias, color, preserve, or
HEVC resolver is introduced.

The combined presentation is deliberately deterministic:

1. A completed canonical `mismatch` in any applicable field forces an overall
   `Reject`/canonical-mismatch presentation, even if SSIM is excellent.
2. Canonical `unverified` or incomplete required evidence prevents `Safe` and
   is shown as `Review`/unverified when the operation is otherwise complete.
3. If canonical fields pass or are not applicable for all windows, the existing
   SSIM/PSNR aggregation and thresholds decide the visual Safe/Review/Reject
   presentation.
4. While windows remain incomplete, the aggregate is `analysis_in_progress`,
   never a final Safe or Reject. Infrastructure failure is shown as failed or
   retryable and is not converted into a canonical mismatch.

The backend Fidelity operation is the sole authority that calculates and
publishes the bounded aggregate DTO. It includes `comparableCount`,
`totalCount`, average/minimum SSIM and PSNR, the worst measured window,
`metricVerdict`, `canonicalStatus`, `aggregateStatus`, and a terminal
`overallVerdict` when complete. `FrameFidelityComparison` renders these fields
and may format a selected window's own metrics, but it does not recompute
comparability, canonical precedence, or Safe/Review/Reject policy.

### 7. Run additional work sequentially with scoped capacity

The operation runner processes the ordered list as follows:

```text
primary evidence
additional window 2: prepare/join -> ready -> evidence -> result
additional window 3: prepare/join -> ready -> evidence -> result
additional window 4: prepare/join -> ready -> evidence -> result
...
aggregate
```

The next additional window cannot start its heavy generation/evidence phase
until the previous one reaches a terminal window result. A metrics result that
is uncomparable is still a terminal window result and may allow the next
window; an infrastructure error in generation/inspection/evidence fails the
operation with completed partial results retained for retry.

Because the current repository has a capacity channel for sample estimates but
no generic media-analysis semaphore, #19 should introduce one small
process-local LAB Fidelity heavy-work slot or factor a clearly shared scoped
primitive if implementation discovery finds one. It is capacity only, not a
Queue scheduler. One operation may hold the slot only for the active heavy
window; multiple operations remain queued rather than creating a NAS encode
fan-out.

### 8. Define cancellation, retry, and restart behavior

The operation owns its context and any #1 consumer/analysis protections it
acquired for evidence reads. Cancel calls the operation cancel function and
releases only those LAB-owned protections and other operation-owned work. It
never cancels the primary ready artifact, turns it back into `generating`, or
unconditionally stops a shared #1 generation. Playback serving protection and
Fidelity analysis protection are independent: completion or cancellation of
one cannot invalidate the other. Terminal snapshots remain observable until
operation cleanup.

Retry creates a new process-local operation for the same material signature,
or uses an explicit retry action if that matches repository conventions. It
resolves the same primary request/artifact and recomputes the bounded window
plan. Existing canonical request IDs, ready artifacts, and evidence references
are reused when their source revision and generation identities still match.
The retry does not assume that an old partial operation remains live.

No operation record is restored on backend restart. #14 reconciliation may
restore a valid primary request/artifact; a status poll for the old process-
local operation stops when it receives not-found/interrupted semantics, and a
new retry reuses the restored primary artifact. Derived process-local evidence
may be recomputed after restart without re-encoding the artifact.

### 9. Keep source/reference and processed primary separate

Sample A remains source-backed. Its browser-compatible reference may use a
cached H.264/AAC Compatible Preview artifact for playback, but #3 source
authority and metric input use the original source plus the selected
normalization evidence. The system must not encode a processed artifact merely
to obtain Sample A's source frame.

The cache signature currently represented by
`labReferenceRequestSignature(path,start,seconds,normalization)` remains a
small frontend reuse hint. Every attempted reuse first re-registers the
canonical reference request through #1/#14; the returned request ID and
`cacheIdentity` (canonical artifact ID) must match the cached request ID and
artifact ID. Only then may the observational #15 status response confirm the
frozen ready generation, interval, source revision, and normalization.
`CachedLabReferencePreview` retains request ID, artifact ID, generation
token/version, source revision, normalized interval, and normalization
alongside bounded inspection evidence. The processed Profile is excluded. A
profile-only edit therefore performs another cheap registration but reuses the
reference artifact and inspection; a same-path source replacement, interval,
or normalization change resolves a new canonical identity and inspects it
once. Registration count and actual artifact generation count are separate
accounting dimensions.

### 10. Keep the frontend states independent and read-only outside orchestration

The frontend will model at least:

```text
Preview:  idle | generating | ready | error
Fidelity: idle | running | completed | failed | canceled
```

After primary readiness it starts/polls the LAB operation at a practical
non-aggressive cadence (for example, roughly two seconds while running),
stopping on terminal state or not-found. If #15 supplies a shared polling
helper, the primary-ready adapter may consume it; LAB does not create a
generic preview polling engine or duplicate #15's readiness-race recovery.

On pre-#15 main, the adapter marks Sample B Ready after the primary artifact
is valid and leaves playback on the existing Compatible Preview transport. On
post-#15 main, it consumes #15's ready/native-media contract. This handoff does
not change LAB operation states and does not make `VideoPreviewRequest`,
credentials, Cookie, Range/seek, or native transport policy a #19 concern.

The UI shows `Sample B: Ready` independently from `Frame Fidelity: Analyzing
window X of Y`, exposes completed windows incrementally, and provides cancel
and retry for Fidelity without disabling playback. A result is applied only if
its operation ID, primary request/artifact and generation identity, backend
material signature, and current local correlation still match the active run.
The check happens before state application, so a late operation whose request
ignores AbortSignal cannot temporarily install stale `lastFidelityInspection`
data. Tab changes render snapshots and artifact/evidence references only; they
do not start hidden media work.

### 11. Use a bounded failure policy

Primary failure is a Preview failure and follows #1/#14 recovery. A bounded
metrics result that explicitly reports non-comparable evidence is retained as
an uncomparable window and the runner continues; the final aggregate cannot be
Safe. A metrics/probe runner error, Compatible Preview generation failure,
inspect failure, canonical-evidence failure, or operation-capacity failure is
an operation failure with bounded diagnostics, partial results, and retry. A
canonical mismatch is a valid completed result, not an operation failure.

No failure response exposes full command logs or unbounded probe data. Existing
bounded diagnostic limits remain authoritative.

### 12. Account for expensive work explicitly

The implementation review must use this call-accounting table. It describes
expected heavy calls, not measured wall-clock savings:

| Work | Current Process Video | Preview-first Fidelity |
| --- | --- | --- |
| Selected processed primary encode | One primary video fetch can generate one artifact, then a policy window may generate the same interval again | One canonical primary artifact generation; all primary consumers join it |
| Source/reference encode | One on initial reference-cache miss; reused only by the current frontend signature | One on material-signature miss; independent of processed Profile |
| Additional window encode | One per policy position requested by the React loop, including a duplicate of the selected region when identities do not join | At most one canonical Compatible Preview generation per unique additional interval; ready artifacts join |
| Generic output/source probe | Inspect probes both; metrics probes both again for each window | One shareable structured probe per artifact generation where fields suffice |
| Frame structure probe | Included in each `inspect` response/window | One bounded evidence result per artifact/window, reused by consumers |
| HEVC bitstream probe | `trace_headers` in each HEVC `inspect` | Separate cached specialized evidence per artifact generation |
| SSIM/PSNR | One metrics call per policy window, with its own probes and comparisons | One metric result per source identity + artifact + normalization; no tab recomputation |
| Comparison frames | On-demand frame endpoint extraction from request IDs; no encode, but no LAB evidence ownership | Read-only artifact/evidence references, with bounded frame caching where safe and #1 analysis protection during extraction |

The table intentionally avoids savings percentages. Implementation tests must
count generation/probe/metric provider calls and prove the invariants instead.

### 13. Test seams and deterministic matrix

The runner, Compatible Preview adapter, capacity guard, evidence provider, and
#3 authority adapter must accept injectable functions or interfaces so tests
can use barriers and counters rather than sleeps.

Required deterministic cases:

| Area | Deterministic assertion |
| --- | --- |
| Preview-first | Primary ready signal is emitted while an additional-window barrier remains blocked |
| Primary once | Canonical generation count for the primary artifact is exactly one across playback, inspect, metrics, Fidelity, and frames |
| Sequential windows | Barrier/counter reports maximum additional heavy concurrency of one |
| Primary deduplication | Selected interval equivalent to a policy interval yields one primary observation and no duplicate request generation |
| Evidence reuse | One artifact/generation causes one shareable structured probe and one metrics result for repeated consumers |
| Specialized evidence | HEVC trace/header provider is called separately from generic ffprobe evidence |
| Evidence protection | Active primary/additional evidence reads hold #1 analysis/serving protection; release permits normal later cleanup |
| Cleanup race | Cleanup cannot remove a ready artifact while the LAB evidence protection is held; later cleanup remains under #1/#14 |
| Shared consumers | Independent playback and Fidelity protections coexist; releasing Fidelity does not invalidate playback |
| Cancellation | Cancel after primary readiness leaves the primary playable and marks only Fidelity canceled |
| Retry | Retry reuses primary and completed valid additional artifacts; no primary generation call occurs |
| Signature safety | Late operation A cannot update UI state for current signature B |
| Frozen plan | Profile edits after start do not alter A's expected plan or window result |
| Reference cache | Equivalent source signature reuses; source revision/normalization/interval changes invalidate |
| Restart | Clearing the process-local operation store loses live operation state but a valid #14 primary artifact is reused on retry |
| Background failure | Additional infrastructure failure leaves Preview Ready and exposes retryable Fidelity failure |
| Canonical mismatch | High SSIM plus #3 FPS mismatch remains visible and forces the combined canonical failure presentation |
| Partial aggregate | Completed windows are visible while aggregate remains explicitly in progress and cannot be final Safe/Reject |
| No hidden encode | Switching every Fidelity tab produces no generation or metric-start calls |

## Risks / Trade-offs

- [Risk] A backend operation starts with a request whose artifact is replaced
  between readiness and operation start. → Resolve and persist artifact ID plus
  #14 generation token/version at operation start; reject stale relationships
  and require a retry that re-resolves the request.
- [Risk] Evidence caching could incorrectly reuse a path after a retry. → Key
  output evidence by artifact ID and generation token/version, and source
  evidence by source revision and normalized interval; never use path/Profile
  identity alone.
- [Risk] A process-local operation disappears while the primary remains
  usable. → Make restart semantics explicit, stop polling on old operation
  absence, and offer retry against the persistent primary request/artifact.
- [Risk] One global LAB slot improves NAS safety but delays concurrent users. →
  Report queued capacity separately from preview readiness and keep the guard
  small and replaceable; do not hide the delay as a preview failure.
- [Risk] Canonical #3 authority is currently Queue-shaped. → Extract or adapt
  the evaluator input without changing Queue precedence or creating a LAB
  policy resolver; add exact field-level tests before wiring the UI.
- [Risk] #19 could accidentally absorb Compatible Preview transport policy while
  making Sample B ready. → Keep the pre-#15 existing playback transport
  unchanged, and let post-#15 behavior come only through a thin adapter. #15
  owns native URLs, Range/seek, credentials/Cookie, #13 isolation, status/
  lease lifecycle, and readiness-race recovery.
- [Risk] Cleanup could delete a ready artifact during evidence extraction. →
  Acquire the existing #1 serving/analysis protection for every bounded
  evidence read and release it on completion, failure, or cancellation; do not
  add a second lease or indefinite retention policy.
- [Risk] Partial metrics can be mistaken for a final verdict. → Keep explicit
  `analysis_in_progress`/uncomparable states and disable final Safe until every
  required window is terminal and canonical evidence is authoritative.

## Migration Plan

1. Add the backend operation/evidence and canonical-authority seams behind the
   new API while leaving existing Compatible Preview routes and #14 recovery
   unchanged.
2. Add the primary-ready adapter and frontend independent state model. Keep the
   existing Process Video entry point but change it to start the primary path,
   then fire the bounded Fidelity operation after readiness. Before #15 is
   merged, preserve the existing Compatible Preview playback transport; if
   #15 merges first, base/rebase the implementation branch onto that main and
   consume #15's ready/native-media contract through the adapter without
   duplicating its lifecycle.
3. Switch Frame/Characteristics/Warnings consumers to operation snapshots and
   evidence references; preserve visual metric labels and thresholds.
4. Add deterministic backend/frontend tests, including artifact cleanup races,
   cancellation release, and independent playback/Fidelity protections; run
   focused checks, and then run `./scripts/verify.sh --auto` during the later
   implementation phase.

Rollback is a code rollback of the LAB operation/UI integration. It does not
delete Compatible Preview artifacts or #14 records; existing artifact cleanup
continues under its original owner.
