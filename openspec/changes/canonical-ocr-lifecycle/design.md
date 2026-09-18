## Context

The existing canonical subtitle pipeline already resolves converted SRT, ASS,
and logical `both` requests through one planner. Within one `both` transform,
`generateSharedConvertedSubtitleArtifacts` performs one text normalization or
bitmap OCR operation and fans the canonical cues out to SRT and ASS. Bitmap OCR
also carries canonical geometry through cleanup for ASS, while Asset Info uses
the existing operation context, process-tree cancellation, staging workspace,
and publication barrier.

The remaining gap is lifecycle scope rather than subtitle planning. Direct OCR
callers currently own their execution independently, and non-raw bitmap OCR
can move raw output to an adjacent `.raw.srt` or `.raw.ass` path. There is no
versioned OCR identity or reusable internal evidence boundary that spans
equivalent canonical callers. The design must fit around these existing seams
without changing source classification, cue correlation, geometry semantics,
operation status, or publication ordering.

## Goals / Non-Goals

**Goals:**

- Make canonical OCR evidence format-independent, provenance-keyed, reusable,
  and safe to share across concurrent equivalent consumers.
- Keep one canonical OCR lifecycle for raw, clean, and accurate modes, with
  exactly the requested serializer fan-out and no public raw intermediates.
- Isolate reusable internal evidence from operation-owned workspace and staged
  publication files.
- Preserve independent cancellation of existing operations while allowing an
  in-flight shared execution to serve remaining consumers safely.
- Bound cache growth, invalidate stale evidence, and keep NAS I/O predictable.

**Non-Goals:**

- A new subtitle planner, public job system, or replacement for the existing
  Asset Info operation model.
- Changes to OCR language selection, Japanese orientation fallback, bitmap
  geometry acquisition/correlation, coarse ASS layout, native ASS/SSA
  extraction, or SRT serialization.
- New sidecar discovery, publication of internal evidence, or changes to
  cancellation status and the atomic publication barrier.

## Decisions

### 1. Add lifecycle management at the canonical OCR seam

The lifecycle manager wraps the existing canonical bitmap OCR/evidence stage
used by Asset Info and the resolved Queue/other artifact path. Callers continue
to pass the resolved stream, language, mode, timing/preparation inputs, and
operation context through the existing planner. The manager returns an
immutable canonical cue result plus effective-language and provenance metadata;
the caller continues to use the existing SRT and ASS serializers and artifact
publication code.

This keeps `both` as one logical transform and makes output format fan-out a
consumer of one result. Adding a second Asset Info cache path or a second Queue
planner was rejected because it would reintroduce divergent OCR, naming,
geometry, and cancellation behavior.

### 2. Define a versioned, format-independent OCR identity

The identity is a canonical serialized tuple with an explicit schema version:

```text
OCRIdentity {
  schemaVersion
  sourceIdentity       // canonical source/asset identity and normalized path
  sourceFingerprint    // existing bounded media fingerprint plus file stat evidence
  streamIndex
  sourceRepresentation // codec and prepared-input/extraction representation
  requestedOCRLanguage // normalized request, including auto resolution input
  normalizedOCRMode    // raw, clean, or accurate
  pipelineProvenance   // canonical OCR/parser/geometry/runtime provenance
}
```

The source fingerprint reuses the repository's existing bounded media
fingerprint convention and records the stat evidence needed to detect a
replacement at the same path. `sourceRepresentation` includes the subtitle
codec and any operation-local preparation mapping that affects the bitmap
input. `pipelineProvenance` is a structured version/fingerprint containing the
canonical OCR lifecycle schema, OCR normalization/cleanup contract, PGS or
VobSub extraction/parser contract, geometry contract, and the runtime OCR
capability/tool provenance used by the attempt. A change to any OCR-affecting
component produces a new identity rather than silently reusing old evidence.

Requested output format and output filename are excluded intentionally. The
identity is also independent of whether the consumer is Asset Info, Queue, or
another canonical entry point. Effective language is recorded in evidence and
diagnostics after execution; it does not replace the requested language in the
identity, so a Japanese fallback remains evidence for the requested Japanese
policy while runtime provenance protects against incompatible capability
changes.

### 3. Store immutable evidence separately from temporary workspace

Reusable evidence is a serializer-neutral canonical payload containing the
recognized cue text/timing/source ordinals, retained bitmap geometry, requested
and effective language, OCR mode, identity, provenance, completeness marker,
and integrity metadata. For clean and accurate modes, the committed canonical
payload represents the post-cleanup canonical cues; raw OCR pass material used
to reach it is not public and is not required to remain after commit.

The cache is a backend-managed internal directory, outside raw media,
destination media, and public sidecar directories. Each entry uses an
identity-derived key and an atomically committed manifest/payload pair. A
reader accepts an entry only when the manifest identity and provenance match,
the completeness marker is present, and payload integrity/shape validation
passes. Writes use a private temporary file followed by an atomic rename, so a
crash cannot create a valid-looking partial entry.

Cache retention is bounded by age and total size using the existing application
data/staging configuration rather than an unbounded media-directory cache.
Eviction removes only internal evidence entries. A cache miss, malformed entry,
or old schema version is safe and simply causes fresh canonical OCR. Existing
published sidecars are not cache entries and are not deleted by cache cleanup.

### 4. Deduplicate in-flight work with subscriber-aware ownership

An in-memory synchronized flight registry maps an OCR identity to one active
producer and an immutable result channel. A cache hit completes without a
flight. On a miss, the first consumer becomes the producer; equivalent
consumers join the flight and receive the same result after the evidence is
validated and, when eligible, committed.

Each consumer is a subscriber tied to its existing operation context. A
subscriber cancellation detaches that consumer and prevents its serializers
and publication from continuing. It does not cancel the shared work while
another subscriber remains. When the last subscriber detaches, the manager
cancels the shared child-process context through the existing FFmpeg/SeConv/
Tesseract process-tree path. The flight then completes as cancelled and never
commits reusable evidence. A producer failure follows the same rule: all
waiters receive the failure, the flight is removed, and a later retry may start
again.

The registry is lifecycle coordination, not a second job system: operation
identity, progress, status, cancellation endpoint, retry, staging, and
publication remain owned by the existing subtitle operation model.

### 5. Make mode lifecycle explicit before serializer fan-out

The canonical lifecycle has one mode-specific evidence production path:

1. Resolve the identity and obtain a cache hit or shared in-flight execution.
2. Run the existing recognition and any existing accurate-mode internal passes
   against the operation-owned prepared input.
3. Select one canonical recognized cue result, retaining geometry and effective
   language metadata.
4. For `raw`, commit/use that recognized canonical result directly.
5. For `clean` and `accurate`, run the existing cleanup once, normalize its
   output back into canonical cues, retain geometry by cue identity, and then
   commit/use the final canonical result.
6. Serialize only the requested SRT and/or ASS outputs from that result and
   pass them through the existing validation and publication barrier.

Raw pass files, accurate-mode alternatives, cleanup inputs, and canonical
intermediate files remain under the operation workspace or internal evidence
cache. The lifecycle does not populate `rawFinalPath` or publish `.raw.*`
files. ASS continues to consume canonical geometry; SRT continues to ignore
it. Logical `both` therefore performs no extra OCR or cleanup for the second
serializer.

### 6. Preserve operation cancellation and atomic publication

The existing operation context controls all shared OCR work and child
processes. A cancelled operation stops waiting, removes only its own staged
outputs through the existing independent bounded cleanup path, and reaches
`cancelled` after cleanup is attempted. If another subscriber keeps shared OCR
alive, that operation's cancellation does not terminate the remaining
consumer's work or publish into the cancelled operation's paths.

Every consumer still stages requested outputs in its operation-owned workspace.
All requested outputs are validated before the existing publication barrier;
the barrier and its race-safe completion-wins semantics are unchanged. A
cancelled or failed operation cannot commit a cache entry or publish partial
sidecars. Existing sidecars and evidence from completed prior operations are
never removed as part of another operation's cleanup.

### 7. Roll out without migrating published media

The first cache schema starts empty and is populated only by successful new
canonical OCR executions. Existing `.srt`, `.ass`, native `.ssa`, and any
legacy adjacent raw files are not imported as reusable OCR evidence, and no
new adjacent raw files are created. Existing sidecar satisfaction and artifact
identity rules remain unchanged. Rollback can disable cache reads/writes while
leaving published sidecars and source media untouched; operation workspace
cleanup remains required in either mode.

## Risks / Trade-offs

- **Risk:** A shared producer can outlive one cancelled operation.
  **Mitigation:** subscriber contexts detach independently; last-subscriber
  cancellation terminates the shared process tree, and the cancelled operation
  never publishes or changes terminal state.
- **Risk:** Pipeline/runtime provenance is incomplete and stale evidence could
  be reused. **Mitigation:** version every OCR/parser/cleanup contract, include
  runtime capability provenance, validate manifests strictly, and fail closed
  to a cache miss.
- **Risk:** Internal evidence consumes NAS storage. **Mitigation:** keep only
  serializer-neutral completed evidence, bound age and total size, and clean
  operation workspaces eagerly after every terminal path.
- **Risk:** A crash can leave an orphaned flight or workspace. **Mitigation:**
  in-flight state is memory-only and cannot be reused after restart; atomic
  cache commits and bounded stale-workspace cleanup prevent partial evidence
  from becoming valid.
- **Risk:** Mode-specific cached results could diverge from existing cleanup
  semantics. **Mitigation:** keep raw/clean/accurate normalization authoritative
  inside the existing canonical OCR implementation and add mode-specific
  execution-count and publication regressions.

## Migration Plan

1. Add the identity/evidence schema and internal cache manager without changing
   existing public artifact metadata or sidecar names.
2. Route existing canonical bitmap OCR callers through the manager, preserving
   their operation contexts, geometry result, progress, and publication barrier.
3. Remove publication of raw intermediates and make each mode serialize only
   requested final outputs.
4. Add bounded cache retention and safe cleanup/invalid-entry handling.
5. Rollback is code-only: cache entries may be ignored or deleted by the
   internal cache policy, while source media, existing sidecars, and operation
   status history remain intact.
