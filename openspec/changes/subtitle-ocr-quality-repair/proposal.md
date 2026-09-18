## Why

OCR-generated subtitle sidecars can be structurally valid while containing
recognition errors that are difficult to detect and repair safely. GitHub issue
#39 needs a bounded review and correction workflow that reuses the canonical
OCR evidence already produced by the subtitle pipeline, keeps SRT and ASS
consistent, and never risks replacing a good published sidecar with an
unvalidated result.

## What Changes

- Add a serializer-neutral OCR quality-analysis stage over canonical OCR cues.
- Detect suspicious cues using deterministic lexical, dictionary, OCR-confusion,
  and existing recognition-evidence signals, while failing safely for proper
  nouns and unknown words.
- Improve `accurate` candidate scoring without rerunning full OCR; permit only
  bounded, selective secondary OCR for suspicious bitmap cues when evidence
  supports it.
- Add OCR repair provenance and internal revision data for source evidence,
  selected candidates, validation, and future undo support.
- Add Asset Info `Analyze OCR` and review/correction actions for clean,
  suspicious, and review cues, including keep, apply, manual text edit, safe
  batch fixes, and an optional subordinate contextual correction provider.
- Validate corrected canonical cues and atomically publish the requested SRT
  and/or ASS sidecars from the same corrected cue set.
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
  evidence; no new Library-visible artifact class.
- Focused backend/frontend regression coverage for analysis, candidate repair,
  shared output behavior, cancellation, publication safety, and cleanup.
