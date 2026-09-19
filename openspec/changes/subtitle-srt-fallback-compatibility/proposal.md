## Why

Users who select styled ASS/SSA subtitles often need a broadly compatible SRT
sidecar as well. Issue #8 requires that compatibility output be opt-in,
resolved consistently by the canonical subtitle planner, and frozen before
processing begins so a later settings change cannot alter the requested
artifacts.

## What Changes

- Add a persisted `generateSRTFallbackForASS` subtitle setting whose default is
  `false`, with eligibility determined at the canonical planning boundary from
  requested mode and source codec.
- Resolve the setting together with explicit subtitle format requests before
  the canonical subtitle planner produces `ResolvedTrackPlan`.
- Normalize and deduplicate effective outputs so an ASS request can add one SRT
  output without changing explicit SRT-only requests.
- Reuse the existing canonical conversion, OCR, correction, logical-`both`,
  validation, cancellation, and atomic publication paths for the additional
  output.
- Expose the effective SRT consequence in existing Asset Info, Track Profile,
  and Lab/Test Encode subtitle-output surfaces before Queue execution where
  those surfaces already expose ASS selection.
- Freeze the effective outputs during Queue Claim/Preparing in the Queue
  snapshot; Workers consume that frozen plan and do not read or reinterpret
  Settings.

The default preserves current behavior. Existing native ASS/SSA extraction,
semantic-role and disposition metadata, bitmap geometry, sidecar naming, and
publication semantics remain authoritative.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: add opt-in ASS-to-SRT fallback resolution and its shared
  canonical planning, serialization, UI, Queue, and publication contracts.

## Impact

- Backend settings persistence and the canonical subtitle resolution inputs are
  extended by one backward-compatible boolean setting.
- `ResolvedTrackPlan` and Queue snapshots represent the normalized effective
  subtitle outputs. Interactive entry points resolve the same canonical
  contract, while Queue freezes its plan during Claim/Preparing before
  execution.
- Native ASS/SSA and bitmap OCR paths may publish an additional SRT sidecar
  when enabled, but each logical transformation still has one canonical cue
  and OCR/evidence lifecycle.
- Frontend subtitle-output controls need to show the effective SRT fallback
  consequence without implementing independent capability or playback logic.
- No new planner, OCR lifecycle, job system, or publication path is introduced.

## Out of Scope

- Changing the default subtitle outputs.
- Client capability detection, playback policy, translation, or a new OCR
  implementation.
- Converting every subtitle format to SRT when ASS was not requested.
