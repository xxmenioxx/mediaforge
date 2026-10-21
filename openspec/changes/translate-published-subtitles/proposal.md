## Why

Asset Info currently applies conversion and profile editing locks to the independent subtitle translation action. This prevents operators from opening the translation dialog for otherwise valid subtitles on published Library assets, so the authoritative source and provider capability checks never run.

## What Changes

- Keep translation available for eligible, owned subtitles on published Library assets across converted, published-as-is, and accepted lifecycle variants.
- Use the canonical subtitle action guard for active Queue/maintenance conflicts and show a concrete blocker when it denies translation.
- Open the existing translation dialog to check the exact selected source and backend provider readiness; keep Start disabled when the backend reports an ineligible or unavailable source/provider.
- Preserve exact source identity, existing text/bitmap support boundaries, and separate atomic translated-sidecar publication.
- Fail closed for Translation when the current publication authority, Library identity, or raw-source provenance is conflicting or unresolved; keep the general `Subtitles` lifecycle capability policy unchanged.
- Build the per-asset Translation lifecycle projection from bounded QueueJob, DirectPublication, active maintenance, Library, and referenced AssetRecord queries while reusing the canonical projection resolver.
- Revalidate lifecycle identity again at the existing publication barrier so a conflict introduced during provider execution cannot publish a translated sidecar.

Out of scope: changing Queue/profile editing permissions, unlocking rename/delete/track or other sidecar mutations, authorizing archive originals, changing provider configuration, or adding a translation/OCR path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: define translation entry behavior across published lifecycle variants, separate outer-action discoverability from translation readiness, and retain backend lifecycle/source blockers.

## Impact

Frontend changes are limited to Asset Info translation controls and their disabled-state explanation. Backend changes reuse the existing asset lifecycle action capability and source resolver for translation capability/start checks. No API schema, database migration, provider dependency, media rewrite, or publication contract change is planned. Focused UI and backend regression tests will cover published variants, active work, unavailable capability, exact source selection, and archive/missing/unsupported blockers.
