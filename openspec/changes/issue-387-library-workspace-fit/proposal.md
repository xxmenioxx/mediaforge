## Why

The Library inventory's fixed-width columns can exceed the content area left by the Assets hierarchy and selected-asset workspace. The enclosing list clips horizontal overflow, leaving filename details or row actions unreachable at common viewport and sidebar sizes.

## What Changes

- Keep Library rows within their available workspace width by collapsing secondary technical columns progressively and constraining the table at flex boundaries.
- Preserve the filename cell, selection behavior, compact row height, tooltips, and per-row actions; do not add columns or page-wide horizontal scrolling.
- Keep RAW layout unchanged and retain all underlying asset data for existing Selected Asset and Asset Info views.
- Add focused responsive regression coverage for constrained widths, sidebar sizing, long names, large inventories, and rightmost actions.

## Capabilities

### New Capabilities
- `assets-library-inventory`: Responsive, accessible presentation of the Library asset inventory within the Assets workspace.

### Modified Capabilities
- None.

## Impact

- Frontend: `AssetsInventoryTable`, the Library workspace/list container in `AssetsPage`, and focused inventory/workspace tests.
- No API, backend, data, dependency, or migration changes. No NAS/runtime impact.
- Out of scope: RAW table layout, new inventory fields/columns, changes to asset data, and the related #386 archive indicator implementation.
