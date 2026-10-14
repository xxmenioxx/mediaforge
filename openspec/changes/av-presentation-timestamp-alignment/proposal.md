## Why

Validation can report a false introduced A/V offset when the source video stream start and its first presented frame have different timestamps. Comparing presentation anchors on both sides prevents this warning while preserving detection of real offsets.

## What Changes

- Measure source and output video alignment from their first presented frame and use a presentation timestamp anchor for each mapped audio track.
- Reuse presentation evidence already available; when it is missing, allow only the existing bounded read-only timing probe. Keep the finding `UNVERIFIED` when comparable evidence remains unavailable.
- Persist per-track source and output relative offsets, introduced offset and frames, tolerance, evidence provenance, and decision in the Validation finding.
- Preserve track identity matching, quantization tolerance, immutable historical ValidationRuns, Result identity, and Publisher handoff behavior.
- Out of scope: `video.properties` cadence, chapter findings, QSV or FFmpeg conversion settings, and global Validation policy changes.

## Capabilities

### New Capabilities

- `validation-av-alignment`: Defines comparable, provenance-bearing A/V presentation evidence and per-track Validation decisions.

### Modified Capabilities

<!-- No existing main spec owns this contract; the active validation-quality-gate change is not present in the main spec inventory. -->

## Impact

- Backend timing evidence and Validation finding construction in `backend/internal/handlers`.
- Focused backend timing and Validation persistence tests.
- No API request changes, database migration, media mutation, conversion command changes, or Publisher policy changes.
