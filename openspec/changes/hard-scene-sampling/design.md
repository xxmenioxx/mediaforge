## Context

See `proposal.md` and `specs/quality/spec.md` for the motivation and
observable contract. The current main already has a shared internal
`SamplingPlan`/`SamplingWindow` in `backend/internal/handlers/sampling_plan.go`.
The balanced analysis policy supplies five deterministic positions
`0.08, 0.27, 0.50, 0.73, 0.92`, adaptive duration rules, an initial-window
budget, and a maximum-window policy. Frame-structure analysis consumes those
windows and exposes bounded GOP/frame-signal evidence; interlace/cadence,
crop, and restoration analysis reuse the same plan or its derived windows.

Merged #19 currently freezes a LAB operation's sampling policy and material
identity, keeps the selected Compatible Preview as the first Fidelity window,
deduplicates equivalent normalized intervals, and runs additional windows
sequentially. Its plan is still position-only. The existing LAB evidence cache
also reuses source probes by source revision and output evidence by artifact
generation, while `ScanResult.RawProbe.snapshotCache` records a fast source
fingerprint, component versions, analysis policy, and refresh provenance.

Available persisted evidence is not uniform: frame/GOP/cadence/interlace and
restoration evidence are versioned and window-aware, while current stream
bitrate and A/V packet timing are mostly global or timing-oriented. There is no
canonical per-window scene-change, motion, dark/gradient, blur, or packet-size
complexity analyzer on this main. The plan therefore consumes such evidence
only when a canonical provider supplies it and treats the rest as unavailable.

## Goals / Non-Goals

**Goals:**

- Extend the existing `SamplingPlan` ownership into a canonical plan that
  preserves baseline positions and adds deterministic candidate, role, scoring,
  consolidation, identity, and provenance semantics.
- Make source revision, evidence identity, policy/version, and selected window
  identity sufficient to freeze and explain a plan without persisting large
  datasets.
- Reuse existing scanner/LAB evidence and add call-accounting boundaries so a
  semantically equivalent probe is not repeated by plan resolution.
- Preserve #19 preview-first behavior, including explicit LAB primary ownership,
  one primary encode, sequential additional windows, and bounded capacity.
- Give #69, #9, Validation, and future consumers a stable read-only contract
  for the same windows and role subsets.

**Non-Goals:**

- No implementation of perceptual scoring (#69), QSV pumping policy (#9), new
  Validation verdicts, or new visual thresholds.
- No feature-specific scene selector, random sampling, full-file decode by
  default, or second analysis/evidence cache.
- No replacement of Frame Fidelity's canonical source/expected/actual
  authority, Compatible Preview artifact ownership, or Queue lifecycle.
- No claim that a score proves visual quality; scores only rank bounded
  selection evidence.

## Decisions

### 1. Extend the existing plan instead of introducing a parallel model

The current `SamplingPlan` remains the single domain object. Its existing
duration, normalized distributed positions, adaptive controls, and analyzer
depth fields remain compatible. The extension adds conceptually:

```text
SamplingPlan {
    schemaVersion, policyVersion, planIdentity
    assetFingerprint, sourceRevision, sourceStreamIdentity
    evidence: bounded EvidenceIdentity[]
    windows: bounded SamplingWindow[]
    fallback: { used, reason, unavailableEvidence, provenanceTruncated }
    limits: { maxWindows, maxCandidatesPerRole, maxProvenanceItems }
}

SamplingWindow {
    identity, startSeconds, durationSeconds, requestStart, requestSeconds
    primary, roles[], reasons[], boundedRoleScores[]
    evidenceRefs[], normalizationVersion, consolidatedCandidateCount
}
```

The exact Go representation can remain internal and be projected to JSON maps
where existing operation/snapshot contracts require it. The extension must not
make an analyzer own a second plan or make consumers reconstruct positions from
UI state.

### 2. Use fast source identity and explicit plan identities

The asset fingerprint reuses the existing fast-validatable source identity:
canonical path plus persisted size, modification timestamp, and mode/source
revision fields where available. It does not hash a complete large media file
as the normal plan path. The fingerprint is not a claim of content equality
beyond the repository's current source-revision contract; replacement or
identity mismatch invalidates reuse.

`windowIdentity` is a digest of `windowIdentityVersion`, source/stream identity,
timeline normalization version, and canonical start/duration quantized to a
fixed precision. It intentionally excludes role, score, policy, and consumer,
so one physical interval can carry several roles and #69/#9 can use the same
identity. `planIdentity` is a digest of the plan schema, asset fingerprint,
policy/version, normalized evidence identities, selected window identities,
roles/reasons, and fallback state. A policy change therefore creates a new
plan identity even when it happens to select the same intervals.

Canonical time normalization clamps intervals to `[0, assetDuration]`, rejects
zero/negative effective duration, applies the existing bounded duration rules,
and records the resulting values before converting them to Compatible Preview's
request granularity. Equality uses the maximum of one reliable source-frame
duration and the fixed fallback precision (33 ms in the initial contract).
The identity uses the normalized values, not the pre-clamp candidate.

Candidate consolidation is deliberately narrower than generic overlap:
intervals with both boundaries within the equality tolerance are one physical
window. Equivalent candidates union roles and reasons, retain the highest
quantized score per role, and record only bounded counts/top discarded reasons.
Materially different overlapping intervals remain separate, protecting a short
hard region from being hidden by a neighboring baseline interval.

### 3. Resolve candidates from a bounded evidence graph

The resolver follows one bounded pipeline:

```text
asset/source revision
  -> matching evidence manifest
  -> candidate windows
  -> normalize and reject out-of-range candidates
  -> score/classify available roles
  -> consolidate equivalent intervals
  -> preserve primary and apply maximum
  -> canonical ordering and plan identity
```

Candidate sources are adapters over existing canonical evidence, not new
feature selectors:

| Evidence source | Initial use | Authority/limit |
| --- | --- | --- |
| Existing distributed positions | `typical` baseline and fallback | Always available when duration permits; not a hard-scene claim |
| Frame/GOP/frame-signal windows | `high_complexity`, `high_motion`, `worst_predicted` when fields are available | Interval-specific and versioned; unavailable fields remain unavailable |
| Cadence/interlace/field windows | Corroborating difficult-motion/cadence reason | Preserves existing cadence/field authority; does not invent visual roles |
| Restoration windows | `high_texture_or_grain`, artifact/complexity reasons from noise, grain, blocking, ringing, edge detail | Existing sampled FFmpeg metrics and confidence only |
| Stream metadata/bitrate | Weak global complexity context and baseline explanation | Cannot identify a particular hard scene alone |
| Packet/timing evidence | Candidate support only when a bounded interval metric already exists | Existing packet timing is not treated as packet-size complexity |
| Existing LAB primary/reference windows | Primary preservation and `typical`/reference reason | User-selected primary outranks inferred ranking |
| Scene/motion/dark/gradient/blur providers | Future evidence adapters | No provider means unavailable; no duplicate analyzer is created here |

Evidence precedence is: frozen explicit primary ownership; matching
interval-specific measured evidence; matching distributed/frame/restoration
evidence; matching stream-global metadata; diagnostic-only hints. Every input
is carried as an evidence identity with source, version, source revision,
availability, confidence, and reuse status. A stale or ambiguous component is
not silently merged with a newer component.

When the existing snapshot manifest matches the source fingerprint, analysis
policy, and component versions, the resolver consumes it. When only global
metadata is available, it can support `typical` or weak ranking context but
cannot manufacture interval hard-scene evidence. Plan selection does not call
Frame Fidelity's output verdict to invent a source selector; after Test Encode,
the same plan can receive result annotations without being reselected.

### 4. Score evidence without turning heuristics into truth

The policy defines a stable role order, source weights, quantization precision,
per-role candidate limit, and global maximum. For each role, the resolver
computes a normalized score only from available contributions:

```text
score(role, candidate) = quantize(
    sum(weight * normalizedSignal * evidenceConfidence)
    / sum(weight * evidenceConfidence),
    policy.precision
)
```

Missing evidence is excluded from the denominator and is reported as
unavailable. No score is emitted when the role has no usable contribution.
The initial role priority for selection is `worst_predicted`,
`high_complexity`, `high_motion`, `high_texture_or_grain`, `dark_or_gradient`,
then `typical`; the policy version owns this order. This is a ranking contract,
not a quality threshold.

The resolver retains the best bounded candidates per available role, merges
equivalent candidates, and fills the global cap in role-priority/score order.
Tie-breaking is quantized score descending, evidence confidence descending,
role priority, normalized start ascending, duration ascending, then window
identity ascending. Final presentation order is primary first, then the
policy's baseline/hard-scene order; the fallback path preserves existing
distributed-position order. Map iteration, filesystem order, wall-clock time,
and goroutine completion cannot affect output.

The balanced compatibility default treats the cap as including the primary and
preserves the current effective capacity of one explicit primary plus at most
the configured five distributed positions when they are not equivalent. A
future policy can lower that cap, but no consumer can raise it for a frozen
plan.

### 5. Make fallback an explicit plan state

If no hard-scene role has enough usable interval evidence, the resolver uses the
existing distributed `SamplingPlan.windows` behavior, including short-asset
duration adaptation and duplicate start suppression. It records a bounded
fallback reason such as `insufficient_interval_evidence`, the unavailable or
stale components, and the baseline policy/version. It does not fail LAB or
require a second discovery pass merely to improve confidence.

If partial hard-scene evidence exists, the plan can select the supported roles
and add baseline windows until the cap. It must not label a baseline window as
hard merely because another role was unavailable. Fallback provenance is part
of the plan identity and frozen snapshot.

### 6. Freeze at the existing lifecycle boundaries

Interactive planning may resolve current evidence. LAB freezes the plan at
operation creation alongside the primary source revision, Compatible Preview
artifact/generation, normalization, effective plan, and sampling policy. Queue
freezes it during `PREPARING` with the existing prepared effective
configuration; processing consumes that immutable snapshot. Test Encode and
Validation use the same resolver and freeze rule when they persist their
operation/evaluation identity.

The frozen record stores the selected windows and bounded provenance, not a
pointer that would cause later re-resolution. A source fingerprint, evidence
version/identity, or policy mismatch makes a non-frozen cache entry stale and
causes recomputation. A frozen operation remains internally coherent and is
reported with its original plan; it is never silently upgraded to a later
plan. Existing persisted snapshots without the new fields are treated as
legacy distributed baseline plans and remain readable without being reinterpreted
as hard-scene plans.

### 7. Preserve #19 LAB ownership and sequential work

The LAB operation receives the canonical plan with the explicit user-selected
interval injected as the first primary window. The primary retains its current
request/artifact/generation identity and is consumed for preview, source/output
evidence, metrics, and canonical Frame Fidelity. The SamplingPlan resolver is
not allowed to register another request for an equivalent primary interval.

Additional windows are materialized through the existing Compatible Preview
request/artifact path using the frozen effective plan. The runner processes them
sequentially under the existing bounded heavy-work capacity. Each response
window includes its stable identity, roles/reasons, and request/artifact
identity; the UI may display the reason when useful but does not recalculate
roles or scores.

### 8. Define consumer adapters, not consumer selectors

The public internal contract is a read-only plan plus bounded role filtering:

- #69 receives the full frozen plan and evaluates the same identities. It may
  omit a window only for an explicit bounded capability reason, never replace
  it with a newly selected hard scene.
- #9 can request existing windows by `high_motion`,
  `high_texture_or_grain`, or `worst_predicted`, preserving the plan identity
  and original reasons. An absent role returns an explicit unavailable result.
- Validation consumes the plan and evidence references to explain coverage and
  preserve the distinction between selection evidence and output verdict.

These adapters do not own policy versions, tie-breaking, fallback, or window
identity. A later plan version can change selection only by producing a new
versioned plan identity.

### 9. Keep persistence and provenance bounded

The persisted plan stores source/policy/evidence identities, selected windows,
role score summaries, evidence references, fallback state, and bounded
discard/consolidation counts. It may retain a small deterministic top-N summary
of rejected candidates for explanation. It does not store frame arrays, full
packet lists, decoded images, complete ffprobe responses, or analyzer logs.

Plan writes are atomic at the owning operation/snapshot boundary. A plan
identity is recomputed from canonical serialized fields before it is committed;
on read, mismatched identity or source revision is rejected as stale. The
existing `ScanResult` component/version manifest remains the source of truth
for whether reusable analysis is current; the plan references it rather than
copying every analysis payload.

## Risks / Trade-offs

- **[Risk]** Existing evidence has uneven granularity and cannot support every
  requested role. → Keep roles optional, expose unavailable evidence, preserve
  distributed fallback, and never present scores as truth.
- **[Risk]** A new plan could increase additional LAB encodes. → Count the
  primary inside a hard cap, deduplicate by canonical interval identity, retain
  the current baseline capacity, and process additional work sequentially.
- **[Risk]** Fast fingerprints can miss a same-size/same-mtime replacement. →
  Reuse the current source-revision contract, include the existing mode/revision
  fields where available, and fail closed when identity cannot be established;
  a full content hash remains an explicit future policy choice.
- **[Risk]** Score-weight changes could alter selection unexpectedly. → Every
  weight, role order, normalization, and tolerance belongs to a policy version;
  frozen plans remain immutable and provenance explains the chosen version.
- **[Risk]** Evidence cache races could duplicate bounded probes. → Add
  call-accounting and identity-keyed single-flight/reuse tests around existing
  LAB/scanner evidence rather than creating another cache authority.
- **[Risk]** Consumers may accidentally reselect scenes. → Keep #69/#9 APIs
  role-filter/read-only, pass stable identities, and test that consumer calls
  do not invoke selection.

## Migration Plan

1. Implement the additive plan/provenance fields and resolver behind the
   existing distributed behavior. Read legacy position-only plans as baseline
   plans and write the new version only at new freeze boundaries.
2. Add scanner/LAB evidence adapters and call accounting, then enable hard-scene
   roles only when matching evidence is versioned and available. Preserve the
   fallback path for all older snapshots and unsupported assets.
3. Add LAB plan consumption while keeping the selected primary artifact and
   existing sequential runner. Validate primary encode reuse before enabling
   additional hard-scene candidates.
4. Add Queue/Test Encode/Validation persistence and consumer adapters for #69
   and #9. These consumers must reject stale plan identities rather than
   recomputing them.
5. Rollback is additive: disable the new policy version and resolve legacy
   distributed windows. Existing source media, Compatible Preview artifacts,
   snapshots, and frozen legacy operations remain untouched; no destructive
   data migration is required.

## Open Questions

- What initial weights and minimum evidence-confidence thresholds best balance
  hard-scene recall against NAS cost for the first policy version? The contract
  is versioned, so calibration can happen without changing identity semantics.
- Should a future canonical scene-change/motion analyzer extend the existing
  frame evidence provider or be delivered as a separate analysis capability?
  The selector contract can consume either, but the ownership must be decided
  before implementation of that analyzer.
- Which bounded role/reason summary should the first LAB UI expose beyond the
  operation API? Backend provenance is required now; UI presentation can remain
  minimal until real hard-scene evidence is available.
