## Context

See proposal.md for the motivating defect and scope. The Library uses the shared compact inventory table, which currently assigns fixed pixel widths to all columns. Its list region clips horizontal overflow even though the table has an inner horizontal scroller. The table is also used by RAW, whose behavior is outside this change.

## Goals / Non-Goals

**Goals:**
- Bind responsive decisions to the actual Library list width, including width changes caused by hierarchy-panel resizing.
- Keep the existing filename cell and rightmost actions visible while reducing secondary technical detail progressively.
- Preserve access to full asset information through current detail views and leave selection and row density intact.

**Non-Goals:**
- Change RAW column sizing or visibility.
- Add, remove, or change asset data, APIs, selection semantics, or archive behavior.
- Introduce a dependency, migration, or backend/runtime work.

## Decisions

- Use CSS container responsiveness rooted at the Library list region so breakpoints follow the workspace's actual width rather than assuming the viewport equals the table width. This accounts for the resizable hierarchy panel and embedded app layouts. Viewport-only media queries were considered but can misclassify a narrow Library pane on a wide display.
- At progressively narrower container sizes, collapse secondary technical columns in a fixed priority order and compact the remaining column widths. Keep the filename, status, selection, and actions visible. Existing Selected Asset/Asset Info views remain the detail path for collapsed fields.
- Keep `AssetsInventoryTable` shared, but gate responsive styles and widths on `mode === 'library'`. This avoids changing RAW behavior and keeps one semantic table and row structure.
- Retain the existing filename cell as the home for inline lifecycle indicators such as #386's Archive marker. Responsive behavior must not add a column or row height for that indicator.
- Keep the Library list boundary constrained and put the bounded, keyboard-reachable horizontal overflow fallback on the table wrapper itself for widths below the smallest responsive layout. Do not allow scroll width to escape the workspace or introduce page-level scrolling.
- Leave pagination, virtualization behavior, filename tooltips/ellipsis, row height, and action rendering unchanged. The inventory already pages at a maximum of 100 displayed rows, so no new performance work is needed.

## Risks / Trade-offs

- Container query support varies in older browsers → retain the bounded horizontal scroll fallback so controls remain reachable when responsive hiding is unavailable.
- Hiding technical columns reduces at-a-glance detail → preserve their values in the existing selected-asset information views and use tests to verify column visibility and action reachability at constrained widths.
- CSS selectors can drift if table cell order changes → scope selectors to the Library table and cover its column/action structure with focused regression tests.

## Migration Plan

No data or API migration is needed. Revert the frontend and focused tests to roll back.
