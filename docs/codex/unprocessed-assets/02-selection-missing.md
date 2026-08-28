# Task 2 — Fix selection semantics with missing assets

Follow all applicable `AGENTS.md` files.

## Goal
Make Unprocessed LogicalGroup/Path selection reflect operationally selectable
assets rather than all historical records.

## Required
- Missing assets remain visible but are not selectable.
- Parent checked/indeterminate state derives only from selectable descendant
  Asset IDs.
- Selected count and selected size exclude missing assets.
- Collapsed LogicalGroups remain selectable without rendering child rows.
- Keep `selectedAssetIds` as the effective selection source of truth.
- Add focused selection-helper tests.

Example:
`E01 selected + E02 selected + E03 missing` => parent is `checked`, not
`indeterminate`.

Inspect first, then implement and validate. Do not stop at a plan.
