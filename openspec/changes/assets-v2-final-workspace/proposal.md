## Why

Assets now has the foundation for a shared RAW and Library workspace, but issue #208 needs one consistent contract for browsing, cached Advisor evidence, path and row actions, and selected-asset detail. The final workspace should make large inventories easier to scan while preserving the existing backend and lifecycle owners.

## What Changes

- Define one Assets workspace and canonical hierarchy with RAW and configured Library roots as siblings; do not add context tabs or Converted, Archive, or Reports inventory roots.
- Make the hierarchy keyboard accessible, resizable from about 220px to 420px with a 300px default, and persist its width locally.
- Specify compact, mode-specific Library and RAW inventory columns, filename-only rows, and a canonical (...) row menu containing supported actions for the current mode and asset.
- Show a compact canonical Advisor score when already available, with distinct neutral not-evaluated and unavailable/off states. Advisor evaluation is explicit-only; rendering or expanding a path never evaluates it.
- Keep path workflow actions in a compact V2 toolbar separate from rows, with mode-relevant actions, selected count, and visually separated destructive operations. Reuse existing AssetGroupRow action handlers and lifecycle owners.
- Define a compact, collapsible Selected Asset Workspace below the table: landscape artwork/preview at left, filename and collection/path context with quick facts, status/context and technical summaries, and primary plus overflow actions.
- Prefer supported Jellyfin artwork, then an existing MVForge asset thumbnail, then a generic placeholder. Do not claim the first two sources are integrated unless an existing supported URL/source is available to the caller.
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

The current API/Asset contract exposes no Jellyfin artwork or persisted MVForge artwork URL for Assets. The existing asset preview route streams source media, so it cannot serve as artwork. Keep the generic placeholder in the workspace and do not describe artwork fallback as integrated until an existing supported source is wired. Adding artwork metadata or an artwork service is a follow-up outside this issue; do not generate artwork here.

The existing path rename dialog is scoped to Library paths. RAW path rename is not exposed by a supported path action/backend contract, so this toolbar keeps Rename available only for Library and RAW path rename remains a follow-up if a canonical operation is added.

Completion tasks track the remaining row-action, explicit-only Advisor, path-toolbar, selected-detail, and regression requirements; do not report those requirements as shipped until implementation and focused tests demonstrate them.
