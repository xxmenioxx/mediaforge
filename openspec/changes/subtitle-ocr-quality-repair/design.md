## Context

The existing `subtitles` capability already defines the canonical subtitle
planner and OCR lifecycle. Canonical OCR evidence is serializer-neutral, shared
by SRT, ASS, and logical `both`, and carries bitmap geometry/layout metadata.
The existing operation model owns cancellation, cleanup, validation, and the
atomic all-output publication barrier. This change adds quality analysis and
correction at the boundary described in the proposal; it does not replace
those contracts or create another planner, OCR lifecycle, or job system.

## Goals / Non-Goals

**Goals:**

- Analyze canonical OCR cues deterministically and make uncertain recognition
  visible before changing a published artifact.
- Reuse canonical OCR evidence and prepared bitmap inputs, doing bounded
  selective work only for suspicious cues.
- Produce one corrected canonical cue set that both SRT and ASS serializers
  consume without changing subtitle structure or layout metadata.
- Support Asset Info review, explicit user decisions, safe batch correction,
  provenance, retry, cancellation, cleanup, and atomic publication.
- Keep NAS reads, candidate generation, cache lookup, and retention bounded.

**Non-Goals:**

- Translation, language conversion, or a general subtitle editor.
- A replacement for canonical OCR, the existing `accurate` mode, Japanese
  orientation fallback, bitmap geometry, or native ASS/SSA extraction.
- Whole-stream or whole-media OCR reruns solely to repair a few suspicious
  cues.
- A second subtitle planner, OCR lifecycle, operation model, or job system.
- Publishing `.bak`, `.raw`, or temporary files beside media.

## Decisions

### 1. Insert repair after canonical evidence and before serialization

The only correction boundary is:

`canonical OCR evidence -> quality/correction -> canonical final cues -> SRT/ASS serializers`

The canonical evidence object remains the source of cue identity, operation-
local timing, order, text, ASS tags/styles, and bitmap geometry/layout. A
quality or repair result references that object and carries text candidates and
provenance; it does not create a format-shaped OCR result. The final cue set is
therefore the single input to SRT and ASS. This reuses the existing shared
`both` path instead of adding an output-specific branch.

An alternative of repairing generated SRT and ASS files independently was
rejected because it would allow output drift, lose geometry context, and make
`both` perform duplicate work. An alternative of repairing native ASS/SSA was
rejected because native extraction is intentionally lossless and outside the
OCR lifecycle.

### 2. Keep canonical cue structure immutable during correction

The repair layer may replace cue text and append correction metadata only. It
must copy or validate the original cue ID, timing, order, ASS override tags,
styles, geometry, and positioning metadata before a candidate is accepted.
Providers receive bounded cue text/context and return text suggestions plus
diagnostics. They cannot return timing, positioning, style, or other structural
subtitle edits.

SRT serialization continues to use text and timing only. ASS serialization
uses the same corrected text together with the retained canonical styles and
geometry. This makes preservation independent of whether a raw OCR ASS
intermediate happened to contain layout tags.

### 3. Use a deterministic, read-only quality analyzer

The analyzer runs over canonical cues and bounded recognition evidence. Its
first pass combines:

- normalized lexical and dictionary evidence;
- a bounded list of known OCR confusions and suspicious token patterns;
- available recognition confidence, alternate candidates, and existing
  `accurate`-mode evidence;
- cue-level consistency signals that do not require scanning unrelated media.

It emits clean, suspicious, or review findings with reasons and bounded
evidence. It does not publish, rewrite canonical cues, start full OCR, or
assume that an unknown word is wrong. Proper nouns, names, and insufficiently
supported tokens remain unchanged or go to review.

Candidate scoring is deterministic and versioned. It may compare the existing
canonical text with candidates already produced by the canonical lifecycle
and, where eligible, a selective secondary OCR candidate. A candidate is
auto-applicable only if the validator and confidence threshold agree;
otherwise the finding remains reviewable. Repeating analysis with the same
canonical evidence and versions is expected to produce the same result.

### 4. Make repair selective and representation-aware

For bitmap cues, selective repair can reuse the prepared bitmap/PGS/VobSub
representation owned by the canonical OCR evidence. DVD/VobSub preprocessing
is bounded and OCR-only; it is limited to suspicious cue input and does not
perform heuristic byte scanning or a full source-media pass. Existing OCR
language, mode, runtime capability, and Japanese fallback semantics remain
authoritative.

The candidate set is ordered for explainability: original canonical text,
existing canonical/accurate candidates, eligible secondary-OCR candidates,
then optional contextual-provider suggestions. A provider suggestion is never
trusted by itself. Deterministic validation must accept it, and a user can
keep or reject it. No provider is a translation service and no translation
candidate is silently treated as OCR repair.

### 5. Use the existing operation model for review and correction

`Analyze OCR` is a read-only action represented through the existing Asset
Info subtitle operation conventions. It reports aggregate counts and cue-level
findings. Applying a suggestion, manual text edit, or safe batch is a
correction execution in that same operation/cancellation model, with the
canonical OCR evidence and repair provenance frozen for the operation.

Each operation owns only its serializer/validation/publication staging, as in
the existing canonical lifecycle. Any shared canonical OCR execution and its
temporary evidence workspace remain owned by that lifecycle. Cancelling one
subscriber does not cancel a shared flight used by another subscriber; when
the shared flight itself ends, existing bounded process-tree cancellation and
independent cleanup rules apply.

### 6. Publish only validated final outputs at the existing barrier

Correction writes candidate SRT/ASS outputs to operation-owned staging. The
full requested output set is validated before the existing publication barrier
commits. For `both`, SRT and ASS are one publication unit. If analysis,
validation, child execution, or cancellation fails before the barrier, the
currently published sidecars remain untouched and all operation-owned staging
is removed. A subscriber terminal state does not delete an active shared OCR
workspace.

No `.bak`, `.raw`, or temporary path is exposed beside the asset. Previous
published artifacts remain in place until a successful atomic replacement;
revision and correction provenance are retained in the internal store so
future undo can be designed without relying on Library-visible backups.

### 7. Separate evidence identity from correction provenance

Quality results and repaired evidence reference the canonical OCR identity
defined by `canonical-ocr-lifecycle`, including source/stream/preparation,
language/mode, runtime, and parser/provenance inputs. Quality analyzer and
repair contract versions are included in internal provenance so implementation
changes fail closed to a fresh analysis or candidate evaluation.

The normal lookup path uses bounded source/stat identity and prepared subtitle
representation evidence already available to the canonical lifecycle. It must
not invoke the whole-media `mediaFileFingerprint()` path for large MKV/BD
assets merely to analyze a cue. Internal quality findings, candidate evidence,
and revision records are committed only after integrity checks; temporary
workspaces are never cache entries.

### 8. Keep NAS and runtime work bounded

The analyzer limits cue text, token, diagnostic, and candidate sizes. Secondary
OCR is limited by cue count, prepared-input size, timeout, and the existing
operation cancellation context. Runtime/tool probes are reused through the
canonical OCR lifecycle's bounded capability/provenance facilities rather than
repeated per cue. Cleanup uses the existing independent bounded cleanup path.

This trades some recall for predictable NAS load and safe review. A cue that
cannot be evaluated within the bounds is reported for review rather than
silently changed.

## Risks / Trade-offs

- [False positive] A valid name or unusual word may look suspicious → require
  deterministic evidence and fail safe to unchanged/review status.
- [False negative] Some OCR errors will not be detected → expose bounded
  evidence and manual review instead of claiming complete correction.
- [Provider drift] A local/LLM suggestion may vary or introduce translation →
  keep it optional, record provider provenance, validate deterministically, and
  never let it modify structure or auto-publish by itself.
- [NAS load] Selective OCR and dictionary work may compete with media jobs →
  bound work, reuse canonical evidence, and avoid whole-media hashing/scans.
- [Atomicity failure] A serializer or validation failure could leave staged
  files → keep staging private, clean it on every terminal path, and commit
  only through the existing all-output barrier.
- [Revision growth] Retaining provenance can consume storage → store bounded
  internal metadata and evidence references, apply the existing retention
  policy, and never place backups beside media.

## Migration Plan

1. Add the quality/correction contracts and internal provenance fields as
   backward-compatible optional data; existing canonical evidence and
   published artifacts remain readable.
2. Implement analyzer and review in read-only mode first, with focused tests
   proving no sidecar mutation and no duplicate OCR.
3. Enable selective repair and final publication behind the existing subtitle
   operation path, preserving cancellation and the all-output barrier.
4. Roll back by disabling repair actions while retaining the existing canonical
   OCR conversion path; do not delete existing sidecars or internal evidence.

No migration of published media is required. Existing SRT/ASS sidecars and
native ASS/SSA artifacts remain valid and are not rewritten by analysis alone.

## Open Questions

None. Provider selection, retention duration, and exact UI wording can be
chosen during implementation without changing the specified canonical boundary
or publication semantics.
