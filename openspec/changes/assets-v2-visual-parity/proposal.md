## Why

The merged Assets V2 workspace preserves RAW and Library behavior, but its summary, workspace surfaces, RAW context, and selected-asset presentation still fall short of the approved mockup. Reports is also represented as an Assets query view, which causes the main navigation to treat report evidence as a separate Assets destination.

## What Changes

- Refine the Assets header, canonical summary cards, one unified Assets hierarchy, scan-first tables, and selected-asset detail to match the approved mockup's composition and visual hierarchy.
- Show RAW and every configured Library root as sibling roots in one accessible tree; tree selection chooses the active inventory context, with no separate RAW/Library toggle.
- Make RAW source, title, and path context clear while retaining its existing hierarchy, inventory, and action owners.
- Keep a shared visual language across RAW and Library while preserving their distinct hierarchy semantics and current workflows.
- Give Reports a separate `/reports` route and main-navigation active state; keep it out of Assets inventory contexts.
- Preserve responsive behavior, keyboard access, selected-only deep detail, and bounded inventory rendering.
- Out of scope: backend or inventory architecture changes, Queue/Advisor/Profile/Test Encode/Validation behavior changes, and redesigns of Dashboard, LAB, Settings, or Asset Info.

## Capabilities

### New Capabilities

- `assets-v2-visual-parity`: Approved-mockup visual hierarchy for Assets V2 and a distinct Reports route/navigation contract.

### Modified Capabilities

None.

## Impact

- Frontend presentation in `AssetsPage`, `AssetsWorkspace`, and the application navigation; a dedicated Reports route will consume the existing persisted inventory report data.
- Focused frontend regressions for visual structure, RAW/Library context switching, selected details, bounded loading, and mutually exclusive Assets/Reports navigation states.
- No backend API, schema, inventory, media-processing, or lifecycle changes.
