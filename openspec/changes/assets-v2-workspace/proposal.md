## Why

The current Assets page gives filters, group summaries, editable controls, and path actions a large share of the screen, so finding and comparing files takes more space and attention than it should. This change makes Library browsing the primary workspace while keeping existing lifecycle, profile, Advisor, Queue, Preview, and Test Encode behavior available through their current canonical flows.

## What Changes

- Replace the Library browsing surface with a compact header and a dominant library hierarchy plus scan-first asset list.
- Show a concise default file table with Name, Status, Profile, Subtitles, Duration, and Modified columns, without row artwork.
- Put path-level Advisor and Queue actions in a compact current-path header, with less common path and configuration actions in an accessible overflow menu.
- Show selected asset information in a compact detail strip with an integrated Preview/Play control, Profile assignment, Test Encode, and secondary actions.
- Render descriptive metadata only when canonical metadata is present; keep NFO and deeper technical details available through existing Asset Info flows.
- Preserve desktop and mobile application navigation, URL filters, asset selection, lifecycle evidence, and all existing backend workflows.

## Capabilities

### New Capabilities

- `assets-v2-workspace`: Minimalist Library hierarchy, asset scanning, contextual path actions, and selected-asset details.

### Modified Capabilities

<!-- Existing main specs have no Assets workspace presentation requirements; lifecycle and processing authorities are preserved unchanged. -->

## Impact

- Frontend: `AssetsPage` and focused Assets workspace presentation components; focused component and regression tests.
- Navigation: preserve the existing `AppLayout` shell and its routes and interaction behavior.
- Backend/API: no changes planned. Use existing inventory, queue, profile, snapshot, preview, and asset detail APIs.
- OpenSpec: add the `assets-v2-workspace` capability delta for the visible workspace contract.
- Out of scope: Queue execution, Batch Preflight, Advisor policy, profile resolution, Test Encode lifecycle, validation, Publisher, metadata enrichment, storage architecture, and media scanning/probing.
