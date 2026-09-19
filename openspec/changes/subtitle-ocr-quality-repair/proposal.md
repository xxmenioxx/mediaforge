## Why

OCR-generated subtitle sidecars can be structurally valid while containing
recognition errors that are difficult to detect and repair safely. GitHub issue
#39 needs a bounded review and correction workflow that reuses the canonical
OCR evidence already produced by the subtitle pipeline, keeps SRT and ASS
consistent, and never risks replacing a good published sidecar with an
unvalidated result.

## What Changes

- Add a serializer-neutral OCR quality-analysis stage over canonical OCR cues.
- Persist a lightweight internal canonical OCR cue snapshot when OCR
  publication succeeds so later analysis does not depend on the evictable OCR
  execution/reuse cache.
- Detect suspicious cues using deterministic lexical, dictionary, OCR-confusion,
  and existing recognition-evidence signals, while failing safely for proper
  nouns and unknown words.
- Replace the current structural-only `accurate` candidate scoring with an
  explicit versioned deterministic score that may use supported-language
  lexical evidence, while preserving safe behavior for unknown languages.
- Permit selective secondary OCR only when a bounded supported mechanism can
  reacquire the suspicious cue input; never rerun full-stream OCR or scan the
  complete source merely because cue input is unavailable.
- Add OCR repair provenance and internal revision data for source evidence,
  selected candidates, validation, published-artifact baseline revisions, and
  future undo support.
- Add Asset Info `Analyze OCR` and review/correction actions for clean,
  suspicious, and review cues, including keep, apply, manual text edit, safe
  batch fixes, and an optional subordinate contextual correction provider.
- Validate corrected canonical cues and atomically publish the requested SRT
  and/or ASS sidecars from the same corrected cue set.
- Bind analysis and correction to the exact published SRT/ASS revision and fail
  closed if an external edit or concurrent correction changes any member of a
  logical `both` output set.
- If a contextual correction provider is enabled later, send only bounded
  suspicious/review candidates in batches, propagate operation cancellation,
  and keep provider text subordinate to deterministic validation.
- Keep analysis read-only; failed or cancelled correction leaves the currently
  published sidecars untouched and exposes no `.bak`, `.raw`, or temporary
  files beside media.
- Preserve the existing canonical OCR lifecycle, operation/cancellation model,
  geometry/layout metadata, native ASS/SSA extraction, and publication barrier.

Translation, broad OCR replacement, exact subtitle layout editing, a second
subtitle planner, a second OCR lifecycle, and a second job system are out of
scope.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: add canonical OCR quality analysis, selective repair, review,
  provenance, and safe publication requirements while preserving the existing
  canonical subtitle/OCR pipeline.

## Impact

- Backend canonical OCR/evidence, cue-quality, correction, provenance, and
  Asset Info operation/API paths.
- Asset Info review UI and progress/error handling.
- Existing SRT/ASS serializers, validation, staging, and atomic publication
  integration.
- Internal cache/workspace cleanup and bounded NAS-safe reads for quality
  evidence. The bounded #6 execution cache remains evictable at its existing
  seven-day/256 MiB policy, while lightweight published cue snapshots remain
  available for analysis without retaining prepared bitmaps.
- Focused backend/frontend regression coverage for analysis, candidate repair,
  accurate selection, snapshot/cache separation, selective-input capability,
  revision conflicts, shared output behavior, cancellation, publication safety,
  and cleanup.
