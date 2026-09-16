## Why

MVForge has specific asset, path, group, and profile decisions but no complete
project-wide fallback policy for new or otherwise unassigned assets. Issue #23
requires one explicit Asset Defaults setting so Video, Audio, and Tracks share
the same final fallback without weakening existing assignments or changing the
current non-destructive behavior by default.

## What Changes

- Add a Settings → Asset Defaults section with independent Video General, Audio General, and Tracks General policies.
- Allow each domain to select Custom or an existing domain profile as its default source, storing a profile reference rather than copied profile fields.
- Resolve Asset Defaults below explicit asset, path, logical-group, source-group, and profile decisions, and above built-in fallbacks.
- Add Video defaults for runtime-supported encoders plus Copy and optional removal of non-primary/non-movie video tracks.
- Add Audio defaults for compatibility-track creation, quality, channel count, resolved/default source selection, and preferred default language.
- Add Tracks defaults for subtitle keep/remove/extract/keep-and-extract, SRT/ASS/both output, existing bitmap OCR modes, attachment policy, and explicit font export policy.
- Reuse the existing canonical `subtitles` capability for subtitle planning, OCR, cue normalization, and artifact generation; selecting both formats performs one logical extraction/OCR and renders both outputs from the shared result.
- Keep Preview, Test Encode, and Queue on the same effective-default resolution path, and preserve immutable queued snapshots when settings change.
- Preserve current behavior on fresh installs and existing users until Asset Defaults are explicitly changed.

## Capabilities

### New Capabilities

- `asset-defaults`: Project-wide fallback policies for Video, Audio, and Tracks, including settings UX, precedence, effective resolution, and snapshot compatibility.

### Modified Capabilities

- `subtitles`: Extend the existing canonical subtitle contract so one logical `both` request produces SRT and ASS from one normalized cue result, while preserving existing OCR semantics, discovery rules, and pipeline ownership.

## Impact

- Backend settings persistence and validation, effective asset/profile resolution, video/audio/track planning, runtime encoder capability exposure, and Queue snapshot capture.
- Settings UI and shared API types for Asset Defaults.
- Preview, Test Encode, Queue, and Worker planning paths, plus focused precedence and parity regressions.
- No media storage, publication, source deletion, new OCR semantics, or parallel subtitle/OCR implementation are introduced; the existing canonical subtitle planner is extended only as needed to represent the shared `both` output.
