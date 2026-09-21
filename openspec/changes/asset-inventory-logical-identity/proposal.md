## Why

The Assets page currently labels the first column as `Asset group`, but Library and
Converted rows render path-oriented values such as `Root` or `Season1` and show
the full filesystem path as the secondary label. This behavior was introduced by
the Assets page refactor in commit `c1ce531`, where row rendering changed from
`groupTitle(group) / groupSubpath(group)` to
`pathLabelForCollection(group) / cleanPathLabel(group.path)`.

That representation is useful for path-scoped operations, but it is the wrong
primary identity for the Library and Converted inventory. A user browsing a media
inventory should identify the title/group first; the storage path is operational
metadata.

The mismatch also conflicts with the current column label and with MVForge's
direction toward logical asset/title identity separated from physical location.

## What Changes

- Restore a logical title/group label as the primary identity for Library and Converted rows.
- Keep path identity available as secondary evidence without making the full absolute path the dominant label.
- Preserve Unprocessed's explicit logical-title → Root/Season/path hierarchy, where path labels are meaningful operational scopes.
- Preserve Archive behavior unless a focused test demonstrates the same identity regression there.
- Keep search capable of matching paths even when full paths are no longer rendered as the primary row identity.
- Do not change inventory grouping, database identity, lifecycle state, Queue behavior, profile scope, path configuration, or publication semantics.

## Capabilities

### New Capabilities

- `asset-inventory-display`: Defines the distinction between logical media identity and physical path identity in Assets inventory presentation.

### Modified Capabilities

None.

## Impact

- Primarily `frontend/src/pages/AssetsPage.tsx`.
- Add focused frontend regression coverage, preferably in a small dedicated
  `AssetsPage.inventory-display.test.tsx` rather than expanding unrelated subtitle
  or LAB tests.
- Backend/API changes are not expected unless implementation proves that the
  current inventory response lacks enough canonical logical identity.
- No database migration, media scan, asset rewrite, rename, or path mutation is
  part of this change.
