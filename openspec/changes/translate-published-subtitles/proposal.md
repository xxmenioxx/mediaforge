## Why

Asset Info currently applies conversion and profile editing locks to the independent subtitle translation action. This prevents operators from opening the translation dialog for otherwise valid subtitles on published Library assets, so the authoritative source and provider capability checks never run.

## What Changes

- Keep translation available for eligible, owned subtitles on unprocessed RAW assets without publication identity and on published Library assets across converted, published-as-is, and accepted lifecycle variants.
- Use the canonical subtitle action guard for active Queue/maintenance conflicts and show a concrete blocker when it denies translation.
- Open the existing translation dialog to check the exact selected source and backend provider readiness; keep Start disabled when the backend reports an ineligible or unavailable source/provider.
- Preserve exact source identity, existing text/bitmap support boundaries, and separate atomic translated-sidecar publication.
- Fail closed for Translation when the current publication authority, Library identity, or raw-source provenance is conflicting or unresolved; keep the general `Subtitles` lifecycle capability policy unchanged.
- Build the per-asset Translation lifecycle projection from bounded QueueJob, DirectPublication, active maintenance, Library, and referenced AssetRecord queries while reusing the canonical projection resolver; index canonical publication path identity so normalized historical aliases remain discoverable without basename scans.
- Revalidate lifecycle identity again at the existing publication barrier so a conflict introduced during provider execution cannot publish a translated sidecar.

Out of scope: changing Queue/profile editing permissions, unlocking rename/delete/track or other sidecar mutations, authorizing archive originals, changing provider configuration, or adding a translation/OCR path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: define translation entry behavior across published lifecycle variants, separate outer-action discoverability from translation readiness, and retain backend lifecycle/source blockers.

## Impact

Frontend changes are limited to Asset Info translation controls and their disabled-state explanation. Backend changes reuse the existing asset lifecycle action capability and source resolver for translation capability/start checks. An internal indexed path-identity column and idempotent backfill preserve normalized associations while avoiding per-Library publication-history scans; this does not change the API or publication contract. Focused backend regressions cover RAW and published variants, active work, unavailable capability, exact source selection, revision changes, and archive/missing/unsupported blockers.
