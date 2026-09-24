## Why

Profile Lab's `Process Video` currently combines the selected Sample B preview,
distributed Frame Fidelity encodes, inspection, and metric aggregation in one
frontend mutation. The selected preview is therefore not an explicit ready
boundary, and the selected `Start`/`Seconds` window can be encoded again when
the sampling policy requests the same region. On a resource-constrained NAS,
this delays useful feedback and performs avoidable encode, probe, and metric
work.

## What Changes

- Make the selected asset, `Start`, `Seconds`, effective video plan, and
  normalization the primary Frame Fidelity window.
- Reuse the primary Compatible Preview request and ready artifact for Sample B
  playback, inspection, metrics, canonical Frame Fidelity, and comparison
  frames; the primary window has one canonical encode.
- Define an explicit `Preview Ready` boundary independent from Fidelity
  completion, with separate Preview and Fidelity UI states.
- Move post-preview Fidelity orchestration into a bounded backend-owned
  operation with status, cancellation, partial results, retry, and bounded
  retention.
- Define the Preview Ready boundary and bind Sample B to the existing primary
  Compatible Preview transport before #15; after #15, consume its ready/native
  media contract through a thin adapter without moving transport ownership into
  #19.
- Execute additional sampled windows sequentially through Compatible Preview
  request/artifact identity, with deterministic time-window deduplication.
- Add bounded per-artifact evidence reuse for compatible structured probes,
  frame structure, metrics, and specialized evidence while keeping
  fundamentally different probes distinct. Protect ready artifacts during
  active evidence reads with the existing #1 serving/analysis consumer
  mechanism, and release that protection on completion or cancellation.
- Consume the canonical #3 source/expected/actual/field-verdict evaluator;
  preserve current SSIM/PSNR thresholds and make canonical mismatches visible
  even when visual metrics are strong.
- Preserve the source/reference cache by backend-authoritative material identity
  (including source revision and Compatible Preview artifact generation) and
  make source reference evidence independent of processed-profile changes.
- Add deterministic coverage for preview-first readiness, one primary encode,
  sequential windows, evidence reuse, cancellation, retry, stale signatures,
  restart behavior, cache invalidation, and canonical mismatch reporting.

Out of scope:

- No production implementation, Queue job, general scheduler, distributed
  worker framework, or general persistent operation engine in this change.
- No replacement of #1 Compatible Preview request/artifact ownership or #14
  persistence/recovery; no second preview lease or cache system.
- #19 does not own Compatible Preview transport or status evolution. It does
  not change `VideoPreviewRequest`, Blob/native media transport, Range/seek
  behavior, browser credentials or Cookie policy, #13 cookie isolation, or
  native readiness-race recovery. Those remain #15 responsibilities; #19 may
  consume an already-merged #15 contract through an adapter.
- No re-tuning of existing visual thresholds, new x265-specific Fidelity
  policy, cadence/GOP/pixel/color/HEVC resolver, or unrelated media policy.
- No automatic persistence or resumption of a live LAB Fidelity operation
  across backend restarts; only its Compatible Preview artifacts remain
  recoverable through #14.
- No PR creation, merge, or OpenSpec archive is part of this planning change.

## Capabilities

### New Capabilities

- `lab-preview-first-fidelity`: Preview-first LAB Process Video behavior,
  backend-owned Fidelity operations, primary/additional window reuse, evidence
  reuse, and canonical Fidelity presentation.

### Modified Capabilities

- None. The existing #1/#14 Compatible Preview lifecycle and merged #3 Frame
  Fidelity capability are dependencies consumed by this new capability, not
  redefined here.

## Impact

- Frontend: `ProfileLabPage`, preview readiness state, Fidelity status polling,
  cancellation/retry controls, and read-only Frame/Technical/Characteristics
  consumers. Playback keeps the current pre-#15 transport and consumes #15's
  native ready-media behavior only when that contract is already available.
- Backend: a bounded LAB Fidelity operation API/store, sequential runner,
  bounded NAS capacity guard, per-artifact evidence provider, and an adapter
  into the canonical #3 evaluator, using #1's existing artifact-use protection
  while evidence is actively read.
- Compatible Preview: additive consumer/evidence integration only; request,
  artifact, single-flight, publication, lease, cache, and #14 recovery remain
  authoritative. #15 remains authoritative for transport/status evolution and
  native media readiness behavior.
- Tests: focused frontend orchestration/API tests and backend lifecycle,
  concurrency, evidence, restart, cache, aggregation, and canonical-verdict
  tests. Validation remains `openspec validate ... --strict` for this branch;
  production verification belongs to the later apply phase.
