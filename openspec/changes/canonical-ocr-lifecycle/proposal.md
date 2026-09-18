## Why

GitHub issue #6 identifies the remaining lifecycle gap around the canonical OCR
result. The repository already routes SRT, ASS, and logical `both` through the
canonical subtitle planner and shares one cue result inside a single logical
transform, but equivalent requests can still repeat OCR and can expose raw
intermediates beside the media. On a resource-constrained NAS, OCR work must
have an explicit identity, safe concurrent reuse, and a lifecycle separate from
published sidecars.

## What Changes

- Define a versioned canonical OCR identity containing source identity and
  fingerprint, subtitle stream index, normalized OCR language, OCR mode, and
  canonical pipeline provenance. Output format is deliberately not part of the
  identity so SRT and ASS can consume one result.
- Add concurrency-safe reuse of equivalent in-flight OCR work and completed
  canonical OCR evidence, with exact provenance validation and invalidation
  when the source, runtime, or pipeline provenance changes.
- Keep reusable OCR evidence in an internal cache with explicit identity and
  provenance metadata; distinguish it from operation-owned temporary OCR
  workspace and staged publication files.
- Make `raw` publish the requested final format or formats directly from the
  canonical recognized cues, `clean` perform cleanup once before serializer
  fan-out, and `accurate` keep its OCR passes internal while selecting one
  canonical result and cleaning it once.
- Prevent `.raw.srt` and `.raw.ass` intermediates from being published beside
  media. Temporary OCR workspace, including raw pass output, is cleaned after
  terminal completion; only validated internal evidence may be retained.
- Preserve the existing canonical subtitle planner, logical `both` execution,
  bitmap geometry/layout metadata, Asset Info and Queue entry points, and
  cancellation plus atomic-publication boundaries.
- Add focused lifecycle, reuse, invalidation, publication, cleanup, and
  cancellation/retry regression coverage.

## Capabilities

### New Capabilities

None. This change extends the existing subtitle capability rather than adding a
second subtitle planner or OCR job system.

### Modified Capabilities

- `subtitles`: define canonical OCR identity/evidence reuse, concurrent
  equivalent-request deduplication, mode-specific publication, internal
  evidence retention, workspace cleanup, and lifecycle behavior while
  preserving the existing canonical SRT/ASS pipeline and cancellation contract.

## Impact

- Backend subtitle OCR execution, canonical artifact fan-out, operation-owned
  workspace cleanup, and internal cache/evidence management.
- Asset Info, Queue, and other existing canonical subtitle callers gain reuse
  without new planner APIs or sidecar discovery behavior.
- A bounded internal cache location and provenance manifest are introduced;
  existing media directories and public sidecar naming remain unchanged.
- No migration of published sidecars is required. Existing cancellation status,
  child-process handling, atomic publication, native ASS/SSA extraction, and
  bitmap geometry semantics remain compatibility constraints.

## Out of Scope

- Redesigning the canonical subtitle planner or logical `both` path.
- Introducing a second subtitle operation/job system or changing operation
  identity, cancellation status, polling, or publication-barrier semantics.
- Changing OCR language selection, Japanese orientation semantics, bitmap cue
  correlation, coarse ASS positioning, SRT serialization, or native ASS/SSA
  extraction except where lifecycle integration must preserve them.
- Publishing raw OCR artifacts, adding adjacent `.raw.*` sidecars, or exposing
  internal evidence as user-facing media output.
