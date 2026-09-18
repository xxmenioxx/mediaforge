## Why

GitHub issue [#26](https://github.com/xxmenioxx/mediaforge/issues/26) reports
that OCR-generated ASS retains recognized text and timing but collapses bitmap
subtitles from different screen regions into the same bottom-center style.
This is visible for simultaneous dialogue and song/lyric cues, and the loss is
already present in the raw OCR ASS, so cleanup is not the source of the
problem.

## What Changes

- Extend the canonical bitmap OCR result with optional cue spatial metadata,
  kept separate from SRT text and timing serialization.
- Obtain geometry only from bitmap representations and runtime capabilities
  that can be verified in this repository: parse available PGS SUP event data
  produced by the existing FFmpeg extraction path, and use a capability-driven
  VobSub geometry provider only when an available source representation exposes
  parseable event geometry.
- Correlate geometry with recognized cues without merging simultaneous cues,
  using cue timing and stable source-event order as the primary association.
- Normalize source-event and OCR cue timing by provenance: prepared-input-local
  values are used directly, original-source-absolute values are translated
  exactly once into the prepared-input timeline, and unknown or ambiguous
  values receive no invented offset and fall back safely. This covers
  non-zero segment/sample execution without double-adjustment.
- Add a deterministic coarse ASS layout resolver using relative vertical
  regions and optional left/center/right alignment. Use a safe bottom-center
  fallback when geometry is missing, ambiguous, or unsupported.
- Keep spatial metadata authoritative through OCR cleanup: render raw ASS from
  pre-clean canonical cues plus geometry, and render cleaned ASS from cleaned
  canonical cues plus the same retained geometry. Keep SRT text/timing-only
  behavior unchanged; correctness must not depend on SeConv preserving
  serialized ASS alignment tags.
- Keep native ASS/SSA extraction lossless and unchanged, and preserve canonical
  artifact identity, OCR language, cleanup, sidecar publication, and logical
  \`both\` shared-result execution.
- Add focused regression coverage for simultaneous regional cues, missing
  geometry, cleanup retention, SRT, native ASS/SSA, and shared \`both\` output.

Out of scope are exact per-pixel \`\\pos(x,y)\` reconstruction, new ffprobe or
SeConv geometry flags/fields that the current runtime does not provide, changes
to OCR recognition or language policy, bitmap subtitle styling beyond coarse
layout, sidecar discovery, and a second subtitle conversion pipeline.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- \`subtitles\`: OCR-generated ASS may consume verified bitmap cue geometry for
  deterministic coarse positioning while SRT and native ASS/SSA contracts
  remain unchanged.

## Impact

- Backend canonical subtitle/OCR planning, bitmap preparation, cue-result
  representation, ASS serialization, validation, and cleanup.
- Focused Go fixtures for PGS/VobSub geometry availability and canonical
  Asset Info/Queue subtitle artifact generation.
- No database migration or new job system. Existing entry points continue to
  use the canonical subtitle pipeline; frontend API shape and native subtitle
  behavior remain unchanged.
