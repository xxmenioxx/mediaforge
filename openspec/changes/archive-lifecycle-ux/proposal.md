## Why

An archived original is evidence about an existing source and its Library
publication. Showing archive records as inventory rows gives retained originals
a third location and separates them from the lifecycle and provenance that
explain them. Issue #21 already provides the canonical RAW/Library projection;
this change defines the smallest extension needed to inspect an archived
original from its related Library asset.

## What Changes

- Keep RAW and Library as the only inventory locations. Archive records remain
  evidence and do not become inventory rows or a Library archive-only list.
- Extend the existing lifecycle projection with read-only archived-original
  details only when canonical publication and source identity resolve the
  relationship to a Library asset.
- Show archive path, recorded size and archive time, provenance, retention
  state and deadline, availability, and the existing restore/delete
  capabilities in the related Library asset context. Use the canonically
  linked archive `AssetRecord.CreatedAt` as the recorded archive time; add no
  timestamp field solely for this UX.
- Remove the standalone Archived inventory/filter view. Keep inspection under
  the related Library asset.
- Reuse existing archive records, retention settings, action capabilities,
  and recovery/deletion mutations. Revalidate identity and filesystem safety
  when those mutations execute.

## Capabilities

### Modified Capabilities

- `assets`: constrain inventory locations to RAW and Library and define
  contextual archived-original inspection and safe actions.

## Non-goals

- No third inventory, archive table/model, or parallel archive API.
- No redesign of Publisher/#54, archive creation, retention policy semantics,
  retention scheduling, or publication behavior.
- No changes to canonical lifecycle or provenance identity from #21.
