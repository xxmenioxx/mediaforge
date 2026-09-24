## Context

See proposal.md for the motivation and scope. The current implementation has
one canonical subtitle planning boundary in `resolveTrackPlan`, with
`ResolvedTrackPlan`/`ResolvedTrackSidecar` carrying source codec, format, mode,
language, forced/default state, semantic role, OCR requirements, stages, and
font-attachment decisions. Queue preparation freezes profile data and the
existing #123 `SubtitleStylePolicy` into the job snapshot.

Native ASS/SSA structure is already parsed through the shared
`assDocument`, `assEvent`, `assProtectedDialogue`, and `parseASSDocument`
boundary used by Translation. The current event model retains source text
offsets, timing, style-field ranges, format fields, and protected dialogue
parts. Composition must extend this boundary so Translation and Composition
continue to share one source-preserving parser/reconstructor.

Bitmap subtitle conversion already produces canonical cues with #26 geometry
and feeds generated ASS rendering. Asset Info already reports embedded and
external subtitle eligibility with exact source revisions, while subtitle
operations share one registry, cancellation path, retry dispatch, staging,
`subtitlePublicationMu`, and publication-committed barrier. These are the
integration seams for Composition; no parallel parser, OCR path, styling
resolver, operation registry, or publisher is introduced.

## Goals / Non-Goals

**Goals:**

- Produce one canonical ASS artifact from one primary and a bounded ordered
  set of supplemental sources while preserving independent simultaneous
  events.
- Make source selection, language/role decisions, reconciliation, styling,
  output identity, and provenance backend-owned and frozen before execution.
- Preserve native ASS/SSA structure and use #26 positioning and #123 styling
  only through their existing canonical boundaries.
- Make the same resolved policy usable from Track Profiles, Queue, and Asset
  Info, with stale-source protection, retry idempotency, and atomic
  publication.
- Keep the implementation safe for a resource-constrained NAS through bounded
  source count, conflict/provenance payloads, and one normalization/OCR pass
  per source.

**Non-Goals:**

- A second ASS parser, document/cue model, serializer, style system, OCR
  implementation, operation lifecycle, or publication mechanism.
- Automatic unsafe composition of different or unknown languages, implicit
  discovery of arbitrary sidecars, or inference of compound semantic roles.
- Rewriting source media or source sidecars, muxing the composite into video,
  downloading fonts, or persisting complete cue bodies as provenance.

## Decisions

### 1. One shared source/document boundary

The existing `assDocument` boundary becomes the generic source-preserving
document representation for Translation and Composition. Its implementation
may gain structured style-section information, event field values, native
style definitions, and source provenance needed to merge documents, but this
is an extension of the existing model, not a second model. The existing
`assEvent` and `assProtectedDialogue` remain the event/protected-text
representation, and `parseASSDocument` remains the only ASS/SSA parser entry
point.

The shared reconstructor will preserve the original representation details
that are meaningful to the source (ASS versus SSA, section order where
supported, unknown lines/comments, event fields, style references, layer,
timing, margins, effects, line breaks, karaoke/drawing data, and override
tags). Translation continues to use protected dialogue parts and anchors. A
Composition document is built by importing parsed source documents into this
same model, assigning source provenance and canonical event ordinals, then
serializing once after reconciliation.

Alternative considered: parse each source into a new composition-specific cue
model and convert it back to ASS. Rejected because it would duplicate the
parser/serializer boundary and risk dropping native fields that Translation
already protects.

### 2. Normalize every source once at the canonical text boundary

The resolver first uses already-authorized source artifacts and the existing
resolved track metadata. Native ASS/SSA sources are parsed directly through
the shared ASS boundary. SRT sources enter the existing text normalization
path and are represented as canonical events with neutral/native-layout
evidence absent. Bitmap sources enter the existing OCR/correction path; the
resulting canonical cues and #26 geometry are consumed without a second OCR
pass.

The normalized event view carries:

- source identity and revision, source representation, track/sidecar
  provenance, and source ordinal;
- start/end, normalized protected/text segments, and native event fields;
- existing semantic role plus provenance/confidence/version;
- bounded composition classification/style intent;
- layout evidence: source style/alignment, inline `\\an`, exact `\\pos`,
  `\\move`, #26 geometry, and an evidence strength; and
- native style identity/definition and referenced font attachments.

No full normalized cue body is written to persisted provenance. The full view
exists only in the bounded operation workspace and in the staged ASS output.

### 3. Keep `lyrics` outside `SubtitleSemanticRole`

Composition uses a separate bounded `CompositionCueClass` (the exact Go name
is implementation detail) with the initial values `dialogue`, `sign`,
`lyrics`, `commentary`, and `unknown`. The class is an explicit composition
input/override and a style/layout intent, not a replacement for
`SubtitleSemanticRole` and not a new classifier for all existing subtitle
rules.

This preserves the current `full`, `forced`, `signs`, `commentary`, `sdh`, and
`unknown` role contract and avoids making `MVF_Lyrics` imply a new global role
classifier. A source track may have a bounded track classification such as
“signs plus songs,” but each event gets its own class. Compound labels are
never serialized as semantic cue roles. The #123 resolver receives the
existing semantic role plus the frozen explicit composition/style intent; it
may select `MVF_Lyrics` only when that frozen intent or an authorized explicit
style selection requests it.

Alternative considered: add `lyrics` to `SubtitleSemanticRole`. Rejected for
V1 because it would change role override validation, Track Profile matching,
persisted snapshots, and all existing role consumers for a style/composition
concern that does not need to redefine semantic track authority.

### 4. Resolve a versioned, bounded composition policy through Track Profiles

Composition settings are an additive, versioned bounded policy inside the
existing Track Profile data resolved by `resolveTrackPlan`. The policy has:

- a stable semantic version and an explicit maximum of eight supplemental
  sources (nine total including the primary);
- primary selection rules and supplemental candidate rules/order;
- same-language automatic selection and explicit cross-language authorization;
- source-track and cue-class/role overrides, including bounded lyrics intent;
- timing tolerance and normalization/reconciliation versions;
- style/layout preservation or explicit override behavior, referencing the
  existing `SubtitleStylePolicy` version; and
- preferred/default publication behavior and explicit manual overrides.

The resolved plan adds composition decisions beside existing subtitle
sidecar decisions. Existing Track Profile subtitle rules and transforms remain
the first selection/transform authority: composition consumes their resolved
source decisions and MUST NOT resurrect removed tracks, bypass role/language
rules, or create a parallel transform. During Queue preparation, the
composition policy, resolved contributors, revisions, source roles/languages,
style snapshot, and output mode are copied into the immutable Queue snapshot
by the existing profile freeze boundary. Legacy snapshots without composition
state remain authoritative and continue their current execution path.

The default policy is conservative: one configured primary/full-dialogue
source, same-language supplemental sources only, native source style/layout
preservation, no implicit cross-language selection, and no preferred/default
promotion unless explicitly allowed. The supplemental order is canonicalized
by stable source identity when policy marks selection order as unordered;
otherwise the explicit profile/manual precedence order is preserved.

### 5. Use a deterministic one-to-one reconciliation pass

After normalization, events are processed in primary order and then stable
supplemental source order. The policy supplies a bounded timing tolerance (the
V1 default is 250 ms, with a maximum of 2 seconds) and a normalization
version. Candidate equivalence requires normalized protected/text content,
start/end within tolerance, compatible resolved language, compatible existing
semantic role, compatible composition class/style intent, and compatible
frozen source provenance. Explicit layout evidence is also a discriminator:
events with distinct meaningful top/bottom/exact-position evidence are not
equivalent merely because their text and timestamps match.

The pass produces a stable decision record per supplemental event:

1. `kept-primary` when exactly one equivalent candidate exists and the primary
   has meaningful layout/style evidence;
2. `enriched/repaired` when exactly one equivalent candidate exists and the
   primary has only a default/missing layout or style, so approved
   supplemental evidence fills that gap;
3. `added` when no primary event is equivalent; or
4. `ambiguous/conflicting` when more than one candidate remains or policy
   compatibility cannot be established.

Exact duplicates are deduplicated as a bounded sub-decision of
`kept-primary`; they never collapse independent simultaneous events. An
ambiguous event is retained in a reviewable conflict result or fails the
operation according to the frozen conflict policy; it is never silently
merged or discarded. Repeated identical text at different timestamps remains
separate because timing and provenance are identity evidence, not text-only
labels.

Alternative considered: concatenate all same-time text into one cue. Rejected
because it destroys simultaneous signs/songs/dialogue layout and makes native
ASS event structure unrecoverable.

### 6. Define layout completeness without inventing geometry

The reconciliation/style stage evaluates `meaningful layout evidence` using
the following precedence:

1. explicit `\\pos` or `\\move` in the event;
2. explicit inline `\\an` override;
3. a native source style/alignment that is not merely the generated/default
   fallback;
4. measured #26 geometry/layout evidence with its canonical confidence; and
5. no meaningful evidence.

The first four categories are retained as evidence; they are not converted
into a new semantic role. When the primary has evidence at any of the first
four levels, it wins a duplicate/enrichment decision unless the policy
explicitly authorizes a compatible repair. When evidence is absent, the
resolver can apply the frozen style policy's bounded default alignment but
must mark the position as default/unresolved rather than claiming exact
geometry.

Native ASS/SSA source structure and override tags remain protected by default.
Generated SRT/OCR events use the existing generated-ASS rendering and
`resolveSubtitleStyle` contract only after the frozen policy enables it.

### 7. Merge ASS styles without losing source meaning

Before serializing, the shared document model creates a deterministic style
namespace for each imported source definition. A style name is identified by
its source identity plus normalized definition, not by display name alone. If
two definitions share a name but differ, both are retained and only events
from the corresponding source are remapped to collision-safe names. A native
source style named `MVF_Default` remains distinct from the #123 preset with
the same identity/display name. Existing inline reset/override tags remain
valid and source-owned.

Generated style definitions from #123 are added only when the frozen policy
authorizes them, use the existing stable preset IDs and generated-name
collision safeguards, and never replace imported native definitions.

Font attachments use the existing resolved attachment inventory and artifact
model. The composition attachment reconciliation first deduplicates
byte-identical content, then assigns deterministic safe filenames to different
content with the same source filename. If a required attachment cannot be
uniquely represented without changing the meaning of a native style, the
operation records an unresolved conflict and publishes no incomplete output.

### 8. Extend Asset Info eligibility without moving authority into the UI

The existing Asset Info subtitle eligibility response is extended with a
composition capability section rather than adding a UI-side candidate scan.
The backend returns eligible source descriptors, exact source revisions,
resolved languages, existing roles and composition classes, supported source
combination, primary/supplemental constraints, attachment/conflict reasons,
and the allowed bounded policy overrides.

The manual request contains the asset identity, one primary source descriptor,
an ordered supplemental descriptor list, the source revisions observed by the
client, and optional explicit policy/style/cross-language overrides. The
backend rechecks identity, containment/ownership, representation, revision,
role/language compatibility, and policy limits before creating an operation.
No operation is created for an invalid request. Existing canonical source
inventory and explicit selection remain the only source-discovery inputs.

The response remains bounded: source descriptors and reasons are capped by the
composition policy, and cue bodies are never returned as eligibility data.

### 9. Add only the operation kind required by the existing lifecycle

The current subtitle operation payload is optimized for one extraction target
and cannot describe one primary plus multiple frozen source descriptors,
reconciliation counts/conflicts, attachment decisions, and deterministic
output identity. A `compose` kind is therefore justified, but it is added to
the existing `subtitleExtractionOperation` registry and status model. It
reuses the same preparation, context cancellation, progress phases,
retry-dispatch, workspace cleanup, staging, validation, and publication
barrier as other subtitle operations.

The composition operation stores a bounded immutable request snapshot rather
than rereading settings during execution. Its phases are preparation/source
revision checks, normalization, reconciliation, style/attachment resolution,
validation, publication, and cleanup. Progress counts sources/events in
bounded aggregates and does not expose full cue payloads.

Cancellation signals the work context before taking the publication lock and
then follows the existing `subtitlePublicationMu`/publication-committed
barrier semantics. All canonical ASS and attachment members are staged and
validated as one publication unit. A cancellation before commitment cleans
only operation-owned staging; a cancellation racing with commitment receives
the existing stable committed/cancelled outcome and cannot remove committed
files.

Retry creates a new operation identity but copies the old frozen request
snapshot. It rechecks all source revisions and returns a stale-source result
instead of silently resolving new inputs. A terminal operation remains
terminal.

### 10. Use a deterministic identity and bounded provenance

The logical output identity is a versioned digest over the resolved ordered
source descriptors (identity and revision), language/role/classification
decisions, composition/reconciliation policy version, #123 style-policy
version, canonical ASS representation, and publication mode. It excludes
operation ID and wall-clock time. If the policy declares supplemental order
irrelevant, the digest uses the stable canonicalized order; if precedence is
meaningful, it uses the frozen explicit order.

The existing subtitle artifact identity/naming helpers receive this logical
identity and produce the canonical `.ass` path. Before publication, an
existing path is accepted only if its bounded artifact metadata contains the
same identity and source revisions. A different identity never overwrites it.

Persisted provenance records the identity/path, primary and contributing
source references/revisions, resolved language(s), existing semantic roles,
composition classifications, policy/style versions, attachment decisions,
and bounded counts for `kept-primary`, `deduplicated`,
`enriched/repaired`, `added`, and `ambiguous/conflicting`. It stores bounded
conflict summaries and no full ASS document, OCR response, or per-cue body.

Only a composite containing the configured primary/full-dialogue source can be
marked preferred/default, and only when the frozen Track Profile or manual
request explicitly authorizes it. Same-language output records that language;
an explicitly authorized cross-language output records a bounded `mixed`
resolved language plus all contributing languages.

### 11. Validate before the publication barrier

Validation uses the shared ASS parser/document boundary on the staged output
and verifies: a valid canonical ASS Events format; all event style references
resolve; protected override/style/drawing structures remain valid; every
source decision is accounted for; no forbidden implicit language composition
occurred; attachment references are reconciled; the output identity matches
the frozen snapshot; and no operation-owned output is partial. Existing
subtitle sidecar validation and native ASS/SSA preservation checks remain
applicable.

If an ambiguous conflict is not explicitly permitted by the frozen policy,
validation fails closed. If the policy permits a reviewable composite,
provenance and operation status include the bounded conflict result, and the
output is not promoted to preferred/default unless explicitly authorized.

## Risks / Trade-offs

- [ASS dialect and unknown-line loss] → Extend the current source-preserving
  model/reconstructor and add round-trip fixtures for Translation before
  enabling composition; reject unsupported structural forms rather than
  flattening them.
- [False deduplication of simultaneous cues] → Require timing, role/class,
  layout evidence, and provenance in addition to text; use one-to-one matches
  and explicit ambiguity outcomes.
- [Style or font collision changes rendered appearance] → Namespace native
  style definitions deterministically, preserve event references, reconcile
  attachments as publication members, and fail closed on unresolved required
  conflicts.
- [NAS CPU, memory, and staging pressure] → Bound sources and conflict data,
  parse/OCR each source once, avoid repeated FFmpeg work, hash exact revisions
  once per operation, and stage only the final canonical output set.
- [Policy drift between Asset Info and Queue] → Use the same backend resolver,
  include policy/style versions in the frozen snapshot and output identity,
  and reject stale snapshots after semantic version changes.
- [Legacy snapshots or artifacts lack composition metadata] → Preserve their
  existing authority and behavior; only new composition artifacts require the
  new identity/provenance fields.
- [Cancellation races with publication] → Reuse the existing pre-lock context
  signal and `subtitlePublicationMu` barrier, test both sides of the race, and
  never perform cleanup of committed paths from a losing cancellation path.

## Migration Plan

1. Add the bounded composition policy and provenance as additive fields in the
   existing profile/plan/operation JSON contracts. Existing profiles and Queue
   snapshots resolve to composition disabled/default behavior.
2. Add the shared ASS-model extensions and reconciliation/attachment helpers,
   keeping Translation and existing native extraction tests green before
   enabling Asset Info or Queue composition requests.
3. Add backend eligibility/request handling and the `compose` operation kind
   behind the backend-authoritative capability response. Existing operation
   kinds and routes remain unchanged.
4. Enable Track Profile/Queue composition only for snapshots containing the
   versioned policy. Existing snapshots continue through their legacy plan.
5. Roll back by disabling composition eligibility and rejecting new compose
   requests. Existing published source/composite artifacts remain untouched;
   in-flight operation workspaces are cleaned through the existing lifecycle.

No destructive data migration or source-sidecar rewrite is required. The
implementation must document the final bounded JSON field names and schema
version in the focused tests and artifact metadata.
