## Why

Assets now has the foundation for a shared RAW and Library workspace, but issue #208 needs one consistent contract for browsing, cached Advisor evidence, path and row actions, and selected-asset detail. The final workspace should make large inventories easier to scan while preserving the existing backend and lifecycle owners.

## What Changes

- Define one Assets workspace and canonical hierarchy with RAW and configured Library roots as siblings; do not add context tabs or Converted, Archive, or Reports inventory roots.
- Make the hierarchy keyboard accessible, resizable from about 220px to 420px with a 300px default, and persist its width locally.
- Specify compact, mode-specific Library and RAW inventory columns, filename-only rows, and one overflow menu for supported row actions.
- Show only an already available canonical Advisor score; otherwise show an honest unavailable or not-evaluated state. Explicit path-level Advisor actions may populate existing results, but rendering rows never evaluates Advisor.
- Keep path workflow actions in a distinct toolbar, with relevant RAW or Library actions and visually separated destructive operations.
- Define a collapsible Selected Asset Workspace below the table with preview, identity, status/context, technical summaries, and existing primary and secondary actions.
- Use available artwork in this order: Jellyfin-generated artwork, existing MVForge artwork or thumbnail, then a generic MVForge placeholder.
- Preserve responsive behavior, existing capability and eligibility enforcement, and bounded inventory/detail loading.

## Capabilities

### New Capabilities

- `assets-workspace`: Shared RAW and Library navigation, inventory presentation, contextual actions, and selected-asset workspace.

### Modified Capabilities

None. The main OpenSpec inventory has no Assets workspace capability; existing Asset defaults and subtitle contracts are not changed by this presentation work.

## Impact

- Frontend: existing Assets route, shared workspace/tree/table/detail presentation, and focused observable tests.
- APIs and backend: no new endpoint, eligibility rule, Advisor evaluation, lifecycle state, or media processing behavior; reuse existing inventory, Advisor, Queue/Preflight, profile, preview, snapshot, and Asset Info owners.
- OpenSpec: add the `assets-workspace` capability contract for this issue; prior Assets V2 change artifacts remain historical inputs and are not rewritten.
- Non-goals: Queue execution, worker/runtime, Advisor scoring, Test Encode pipeline, profile resolution, validation, Publisher, metadata enrichment, inventory redesign, and artwork generation.

## Follow-up

The current API exposes no Jellyfin artwork or persisted MVForge artwork URL for Assets. The existing asset preview route streams the source media, so the workspace does not use it as an image. The implementation currently displays the generic placeholder; wiring canonical artwork metadata or an artwork service is a follow-up. No artwork is generated as part of this change.
