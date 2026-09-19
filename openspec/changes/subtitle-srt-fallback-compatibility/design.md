## Context

The existing `subtitles` capability already resolves source tracks and
requested sidecars through one canonical planner, freezes the resulting
`ResolvedTrackPlan` for Queue execution during Claim/Preparing, and routes text and bitmap subtitles
through shared serializers/OCR evidence. Native ASS/SSA extraction preserves
the original styled representation, while bitmap subtitles use the canonical
OCR lifecycle and can preserve geometry for ASS positioning.

The change adds one persisted opt-in setting without creating another decision
path. See `proposal.md` for motivation and
`specs/subtitles/spec.md` for the observable contract.

## Goals / Non-Goals

**Goals:**

- Resolve `generateSRTFallbackForASS` before the canonical subtitle plan is
  frozen, with a false default for existing installations.
- Normalize effective requested formats once, preserving explicit requests and
  removing duplicate SRT/ASS outputs.
- Keep one canonical cue/evidence lifecycle for ASS+SRT bitmap OCR and for
  corrections applied before both serializers.
- Make the effective SRT consequence visible in existing output-selection UI
  and keep Asset Info, Profiles, Lab/Test Encode, and Queue aligned through
  the same canonical resolution contract.
- Preserve Queue snapshot, cancellation, validation, publication, metadata,
  and logical-`both` boundaries.

**Non-Goals:**

- A second subtitle planner, worker-side Settings interpretation, OCR
  lifecycle, job system, or publication path.
- Client capability detection, playback policy, translation, or a change to
  native ASS/SSA styling/positioning semantics.
- Enabling fallback by default or adding SRT output for requests that do not
  select ASS.

## Decisions

### 1. Persist one backward-compatible setting

Store a boolean setting equivalent to `generateSRTFallbackForASS` through the
existing application settings persistence/API. Missing legacy values resolve
to `false`; saving `false` remains observable and does not require a migration
of existing subtitle profiles. The setting is read while resolving a request,
not by a Worker during execution.

An explicit SRT request is already satisfied and is not duplicated. The
setting augments an effective request containing ASS; it never removes an
explicit format or changes a request that contains neither ASS nor SRT.

### 2. Resolve and normalize at the canonical planning boundary

The existing canonical resolver receives the explicit requested formats and
the resolved fallback setting, adds SRT only when ASS is effective, and then
normalizes/deduplicates the formats before constructing `ResolvedTrackPlan`.
The plan records the effective outputs and the setting/effective-output
decision as provenance where the existing snapshot/artifact metadata permits.

The normalization contract is:

| Explicit request | Setting | Effective output |
| --- | --- | --- |
| ASS | on | ASS + SRT |
| SRT | on or off | SRT |
| ASS + SRT | on | ASS + SRT |
| ASS | off | ASS |

Asset Info, Track Profiles, Profile Lab, Test Encode, and Queue use the same
canonical resolver and effective-format resolution contract. Interactive entry
points may produce equivalent current plans; Queue owns and freezes its
execution snapshot. Existing profile/rule precedence and semantic-role,
forced, SDH, language, stream, naming, and sidecar metadata remain unchanged.

### 3. Keep source codec separate from requested extraction mode

Source codec and requested extraction mode are independent dimensions. A text
source requested with `mode=converted` and `format=ass` is a converted ASS
output; when fallback is enabled, it produces converted ASS plus converted
SRT through the existing canonical normalization and serializer path. A
bitmap source follows the same converted-output contract using the shared
canonical OCR/cue lifecycle.

Separately, a native ASS/SSA source requested with `mode=original` preserves
the native styled representation as the original artifact (`.ass` or `.ssa`)
with its styles, tags, and positioning intact. If fallback is effective, a
separate converted SRT output is generated through the existing canonical
conversion path. The native original remains untouched.

The original-plus-fallback case is not logical `both`: logical `both` remains
the existing converted SRT+ASS transformation sharing one normalized cue/OCR
result. The output model distinguishes original extraction from converted
outputs while retaining source/stream identity, role/disposition metadata,
canonical naming, and existing publication rules.

### 4. Share bitmap OCR and correction evidence

For a bitmap source with effective ASS+SRT output, the resolver represents one
logical subtitle transformation. The existing shared OCR lifecycle prepares
and recognizes the bitmap input once, produces one canonical cue/evidence set,
and applies any existing OCR quality correction before serializers run. ASS is
rendered from those cues plus retained geometry/layout; SRT serializes the
same corrected cue text and timing without geometry. Serializers never invoke
OCR or correction independently.

Existing Japanese orientation fallback, accurate-mode selection, geometry
positioning, cancellation, shared-flight ownership, and all-output atomic
publication barriers remain in force.

### 5. Freeze effective outputs at Queue Claim/Preparing

Queue lifecycle remains `QUEUED → CLAIM/PREPARING → resolve effective
configuration → build/freeze ResolvedTrackPlan → PROCESSING`. A setting change
while a job is still queued and before successful freeze is therefore observed
when preparation resolves the plan. The frozen snapshot contains the effective
sidecar outputs and relevant provenance, and Workers consume only that frozen
`ResolvedTrackPlan`.

After successful freeze, changing the persisted fallback setting cannot add,
remove, or reinterpret a sidecar for that execution. A retry or resumed
preparation whose freeze is incomplete may resolve again using current
settings; a retry of a job with a complete frozen snapshot reuses that same
execution snapshot. A new/future job resolves current settings. Existing
stale-snapshot and operation cancellation semantics are preserved.

### 6. Keep UI and entry points observationally consistent

Existing Asset Info, Track Profile, and Profile Lab/Test Encode controls keep
their current format-selection model. Where ASS output can be selected, the
UI shows that SRT will also be produced when the persisted fallback setting is
enabled, before Queue execution. The display is derived from backend-resolved
effective behavior or the same canonical settings-backed data; it is not a
client capability check and does not make an independent output decision.

Asset Info, profile resolution, Lab/Test Encode, and Queue use the same
effective-format contract. Queue owns the frozen execution snapshot; no entry
point invents a sibling transform, fallback interpretation, or publication
path.

### 7. Preserve publication and compatibility boundaries

Validation covers the complete requested output set. When fallback adds SRT,
publication uses the existing atomic all-output barrier, preserving existing
sidecars on cancellation/failure and avoiding partial newly generated output.
Sidecar names and qualifiers continue to derive from the canonical artifact
identity, including semantic roles and forced/SDH metadata. Logical `both`
remains one transformation and is not duplicated by fallback normalization.

Installations without the setting behave exactly as before. The additional
SRT sidecar is opt-in and is not used for playback policy or translation.

## Risks / Trade-offs

- [Risk] An enabled fallback increases subtitle work and storage for ASS
  requests. → Keep it opt-in, deduplicate explicit SRT, share OCR/correction,
  and expose the effective output before Queue.
- [Risk] A setting change could otherwise alter a queued job. → Resolve before
  Queue freeze and have Workers consume only the immutable snapshot.
- [Risk] One output could publish without the other. → Reuse the existing
  complete-output validation and atomic publication barrier.
- [Risk] Native ASS/SSA styling could be lost in compatibility conversion. →
  Preserve native extraction as-is and generate SRT as an additional
  text/timing-only artifact.
- [Risk] Bitmap OCR could run twice for two serializers. → Require one logical
  transform and one canonical OCR/evidence set before serializer fan-out.

## Migration Plan

1. Add the persisted setting with a missing-value default of `false`.
2. Add canonical effective-format resolution and include it in plan/snapshot
   provenance.
3. Integrate existing serializers, OCR lifecycle, validation, publication, and
   UI surfaces without changing their independent contracts.
4. Roll back by disabling the setting and deploying the previous resolver;
   already published sidecars are preserved according to existing artifact
   policy.

## Open Questions

None. The approved scope fixes the setting default, resolution boundary,
format normalization, shared evidence behavior, and Queue freeze contract.
