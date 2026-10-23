## Why

Subtitle timing errors need a reviewable correction tied to an explicitly chosen audio stream. Asset Info currently has no canonical sync operation, so a manual correction would otherwise bypass subtitle ownership, cancellation, validation, and safe publication.

## What Changes

- Add a reusable subtitle synchronization proposal and timing-transform contract, starting with fixed offset and leaving room for affine and segmented proposals.
- Expose sync analysis and review from Asset Info for eligible SRT, ASS, and supported SSA subtitle sources, requiring an explicit reference audio stream.
- Reuse protected SRT/ASS timing representation, existing subtitle operation cancellation, source revision checks, staging, validation, and atomic publication.
- Preserve dialogue, ASS structure, artifact identity, and source metadata; apply only after the user reviews a bounded before/after cue preview.
- Report engine availability from the backend and keep fixed-offset operation usable when optional alignment engines are unavailable.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: add explicit, reviewable audio synchronization proposals and safe application for text subtitle artifacts.

## Impact

- Backend subtitle source resolution, sync proposal/timing transform, Asset Info API, operation lifecycle, and safe subtitle publication.
- Frontend Asset Info subtitle actions, explicit audio selection, proposal review, and API types/client methods.
- Focused SRT/ASS preservation, revision, cancellation, invalid-output, and publication tests.
- Add one compact SQLite provenance table for successful applies. External engine adapters remain optional and must not be required for fixed-offset operation.
