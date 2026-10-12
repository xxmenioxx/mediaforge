## Why

Saved `Asset Info > Tracks` subtitle selections can be lost when a Video Profile automatically externalizes subtitles, so excluded streams may still be extracted, OCRed, converted, muxed, or published. The frozen Queue plan must make the saved selection authoritative so excluded tracks also consume no subtitle-processing work.

## What Changes

- Preserve the distinction between omitted (`nil`, inherit), explicit empty (`[]`, select none), and explicit indexes (select only those streams) from the asset override through Queue preparation and Worker execution.
- Give explicit asset track exclusions priority over automatic subtitle rules and Video Profile externalization while keeping the existing canonical subtitle resolver and artifact pipeline.
- Filter excluded streams before extraction, OCR, conversion, sidecar planning, FFmpeg mapping, and publication; do not create subtitle stage work when no authorized subtitle or independent font work exists.
- Keep accepted/frozen jobs immutable, retain legacy snapshot behavior, and cover Queue Preflight, retries, restarts, same-language tracks, text/bitmap sources, and unrelated stream types.
- Out of scope: changing Video Profile or OCR policy, redesigning Publisher, changing Asset Info persistence/UI, or adding a second resolver or subtitle pipeline.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: explicit per-asset subtitle exclusions remain authoritative throughout Queue planning, frozen execution, artifact generation, FFmpeg mapping, validation, and publication.

## Impact

- Affected code: `backend/internal/handlers/track_resolver.go`, `queue.go`, `media_command.go`, `subtitle_transform.go`, and the Worker/Queue/command/artifact regression tests.
- Queue snapshots and APIs keep their existing shapes; no migration or dependency is expected. Legacy snapshots remain readable and authoritative.
- Validation uses synthetic fixtures only; no production media is accessed or modified.
