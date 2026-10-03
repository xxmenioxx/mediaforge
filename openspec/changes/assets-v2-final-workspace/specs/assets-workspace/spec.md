## Purpose

Defines one accessible, responsive Assets workspace for browsing RAW source media and configured Libraries while preserving canonical inventory, Advisor, Queue, profile, and lifecycle behavior.

## ADDED Requirements

### Requirement: One canonical hierarchy selects RAW or Library context

The Assets page MUST present one shared workspace and one canonical hierarchy. RAW and each configured Library MUST be sibling top-level roots. The workspace MUST NOT expose separate RAW/Library tabs or add Converted, Archive, or Reports as inventory roots. Selecting a root or descendant MUST keep its canonical source or Library identity and visibly identify the selected path context.

#### Scenario: Browse RAW and configured Libraries
- **WHEN** the Assets hierarchy is displayed
- **THEN** one tree contains RAW and each configured Library as sibling roots, with no additional inventory roots or separate context tabs

#### Scenario: Select a hierarchy path
- **WHEN** a user selects a RAW or Library root or descendant
- **THEN** the matching inventory is shown, the selected path is identified by the hierarchy or breadcrumb, and canonical context identity is preserved

### Requirement: The hierarchy is resizable, persistent, and keyboard accessible

The hierarchy panel MUST have a default width of approximately 300px and allow resizing between approximately 220px and 420px. Its width MUST persist locally across reloads and MUST be constrained to the supported range when a stored value is outside it. The resize control, tree selection, expansion, and collapse MUST be operable by keyboard and expose accessible names and state.

#### Scenario: Resize and restore the hierarchy
- **WHEN** a user resizes the hierarchy within the supported range and reloads Assets
- **THEN** the chosen width is restored locally without changing the selected RAW or Library identity

#### Scenario: Recover from an invalid stored width
- **WHEN** the locally stored hierarchy width is outside the supported range or cannot be read
- **THEN** the workspace uses a supported width and remains usable

#### Scenario: Resize and navigate without a pointer
- **WHEN** a user operates the hierarchy and its resize control by keyboard or assistive technology
- **THEN** resizing, tree navigation, selection, expansion, and collapse remain reachable with visible focus and announced state

### Requirement: Library rows provide compact technical inventory

The Library inventory MUST display these columns in this order: Asset, Advisor, Status, Codec, Resolution, Encoder, Duration, Bitrate, HDR/SDR, Size, Modified, and Actions. The Asset cell MUST contain the filename only and MUST NOT repeat a relative path already represented by the selected hierarchy node and breadcrumb. Rows MUST remain compact and MUST NOT include per-row artwork.

#### Scenario: Scan Library columns
- **WHEN** a configured Library is selected
- **THEN** the inventory displays all twelve Library columns in the specified order with compact technical density

#### Scenario: Show filename without its path
- **WHEN** a Library row is rendered for an asset under a nested path
- **THEN** its Asset cell shows only the filename while the selected hierarchy and breadcrumb provide path context

#### Scenario: Show missing Library summaries
- **WHEN** an asset lacks one or more technical summary values
- **THEN** those cells communicate unavailable values without probing or fetching deep detail for every row

### Requirement: RAW rows use source and preparation semantics

The RAW inventory MUST use the shared table presentation with columns for Asset, Advisor, Status, Source / Codec, Resolution, Duration, Bitrate, Size, Modified, and Actions. The Asset cell MUST contain the filename only. RAW status labels such as Unprocessed, Analyzed, Profile Assigned, Ready for Queue, or Needs Review MUST be shown only when supported by existing canonical backend state or evidence; otherwise the row MUST retain an honest unknown or unavailable state.

#### Scenario: Scan RAW columns
- **WHEN** a RAW source, title, or path is selected
- **THEN** the inventory displays the ten RAW columns in the specified order using source and preparation summaries and preserves existing pagination behavior

#### Scenario: Avoid inventing RAW lifecycle state
- **WHEN** existing backend state does not establish a suggested RAW label
- **THEN** the row displays the available canonical state or an unknown/unavailable value and does not infer a lifecycle transition

#### Scenario: Show filename without its path in RAW
- **WHEN** a RAW asset belongs to a selected source path
- **THEN** its Asset cell shows only the filename and the selected hierarchy or breadcrumb supplies the path context

### Requirement: Advisor display uses available canonical evidence only

The compact Advisor cell MUST use `AdvisorResponse.score` when that result is already available and present it as a concise rating/score. When no result is available, the cell MUST show a restrained neutral not-evaluated state; an Advisor setting or capability that is unavailable/off MUST have a distinct unavailable state. Neither state may imply a score. Rendering a row, expanding a path group, sorting, pagination, refresh, or tree selection MUST NOT start Advisor evaluation. Advisor evaluation MUST happen only from an explicit user action. An explicit path-level Advisor workflow MAY create or refresh results through the existing owner and MUST populate/read the existing row cache key `['advisor', 'asset-row', asset.path, profileId]` where supported.

#### Scenario: Display a cached Advisor result
- **WHEN** an asset already has an available canonical Advisor response
- **THEN** its row displays that response's score without starting another evaluation

#### Scenario: Display Advisor without a result
- **WHEN** no canonical Advisor result is available for an asset
- **THEN** its row displays an unavailable or not-evaluated state without implying that evaluation succeeded

#### Scenario: Distinguish unavailable Advisor from not evaluated
- **WHEN** Advisor is unavailable or disabled for the current context
- **THEN** the cell uses a compact unavailable/off treatment that is visually distinct from the neutral not-evaluated state

#### Scenario: Render rows without Advisor work
- **WHEN** RAW or Library rows are rendered, a path group is expanded, or inventory is refreshed
- **THEN** no automatic Advisor evaluation is triggered for those rows

#### Scenario: Run path-level Advisor explicitly
- **WHEN** a user explicitly starts the supported Advisor action for a path
- **THEN** the existing path-level workflow runs and caches each resulting asset response under `['advisor', 'asset-row', asset.path, profileId]` without adding per-row evaluation on render

### Requirement: Row and path actions remain distinct and use existing eligibility

Each inventory row MUST expose supported asset actions through one keyboard-accessible (...) overflow menu, which is the canonical row action surface for the compact V2 inventory. For an eligible Library asset, the menu MUST expose each applicable supported action from Open in Lab, Asset Info, Assign Profile, Test Encode, Queue/Queue replacement, Advisor, and lifecycle-safe Delete. For an eligible RAW asset, the menu MUST expose each applicable supported action from Scan/Analyze Asset, Advisor, Assign Profile, Queue, Open in Lab, Test Encode, Asset Info, and Delete where supported. Open in Lab, Asset Info, Assign Profile, Test Encode, Queue, Advisor, Scan/Analyze, and Delete MUST route through their existing capability contracts, dialogs, and lifecycle behavior. Only actions supported for the current asset and context may be offered or enabled. Action execution MUST preserve existing backend capability and eligibility decisions and existing confirmation, preflight, and lifecycle flows. Destructive actions MUST be visually separated from routine menu actions.

Path-level actions MUST use a compact Assets V2 toolbar, distinct from both the inventory table rows and the row overflow menu. It MUST replace the legacy `AssetGroupRow`-inside-table presentation while reusing the same callbacks, mutations, confirmations, and backend behavior rather than duplicating business logic. The toolbar MUST show selected asset count and expose only relevant existing operations. For applicable RAW paths it MUST expose supported Scan, explicit Advisor, Queue/Preflight, and Configure Path; the existing Configure Path operation includes path-level profile configuration. For applicable Library paths it MUST expose supported Queue/Preflight, move/path operations, and Rename. The current path rename dialog supports Library paths only; RAW path Rename is unsupported and MUST NOT be added without an existing backend/action contract. Supported selected-delete and path-delete actions MUST remain available through a visually separated destructive overflow. Row actions MUST NOT be represented as path actions, or vice versa.

#### Scenario: Open supported row actions
- **WHEN** a user opens an asset's overflow menu
- **THEN** the menu exposes the supported actions for that row's RAW or Library context, uses accessible labels, separates destructive actions, and preserves existing dialogs/routes/workflows

#### Scenario: Enforce current action eligibility
- **WHEN** an asset or path is not eligible for a workflow action
- **THEN** the action follows the existing capability, eligibility, disabled-reason, and confirmation behavior rather than a frontend-only rule

#### Scenario: Distinguish path and row actions
- **WHEN** a user selects a path and an asset within it
- **THEN** path operations and selected-count context appear in the compact V2 toolbar while asset operations remain in that row's overflow menu; the toolbar is not rendered as a legacy table row

#### Scenario: Queue through the existing path
- **WHEN** a user starts Queue or Preflight from a path or row
- **THEN** the existing Queue planning or Batch Preflight acceptance flow is used without bypassing reviewed eligibility

### Requirement: Selected Asset Workspace remains below the table

Selecting an asset MUST show a collapsible Selected Asset Workspace below the inventory table and attached to the bottom of the Assets workspace while inventory rows scroll. It MUST NOT become a persistent right-side panel or disappear below the fold. The expanded panel MUST omit a visible "Selected Asset Workspace" heading and visible group headers such as "Quick facts" or "Status / context". On desktop it MUST use a compact, flat horizontal composition: landscape artwork with centered Play overlay and duration badge at left; filename, series/collection/season/path context, and category/library, container, size, resolution, and codec quick facts in the center; Estado, Perfil, Subtítulos, Duración, and Modificado alongside video codec, resolution, and audio summary; and primary Assign Profile, Test Encode, and (...) actions where supported. Artwork MUST NOT dominate panel height. Do not repeat status as both a prominent badge and Estado unless the visual reference clearly benefits from both. At narrow widths, the same information and primary actions MUST reflow without page-level overflow. Secondary actions MAY include Open in Lab, Asset Info, Advisor, Rescan, Rename, and Delete when supported. Preview/play MUST be available through the artwork interaction without a redundant visible Play action elsewhere.

#### Scenario: Show selected asset detail below the table
- **WHEN** a user selects a RAW or Library asset
- **THEN** its collapsible detail workspace appears below the table with available identity, context, status, technical summaries, and eligible actions

#### Scenario: Collapse selected asset detail
- **WHEN** a user collapses the Selected Asset Workspace
- **THEN** the compact bottom panel remains visible with the filename and an accessible reopen control while the inventory table remains available

#### Scenario: Keep selected detail attached while rows scroll
- **WHEN** the user scrolls a long Library or RAW inventory
- **THEN** the selected-asset panel remains visible at the bottom of the workspace, independently of row scrolling

#### Scenario: Play from the selected artwork
- **WHEN** a user activates the selected asset artwork's Play overlay
- **THEN** the existing preview behavior opens with one integrated Play control and a duration badge remains visible when duration is available

#### Scenario: Show unavailable selected detail safely
- **WHEN** selected-asset summary or preview evidence is missing
- **THEN** the workspace identifies unavailable values without generating artwork, preview, or media analysis to fill the gap

### Requirement: Artwork remains a placeholder until a supported source exists

The current Selected Asset Workspace MUST display the generic MVForge placeholder because the Asset/API contract and selected-detail caller expose no supported Jellyfin artwork or MVForge artwork/thumbnail source. It MUST NOT use the source-media preview as artwork or generate artwork, preview, or per-row image requests. Artwork integration is an explicit follow-up; when supported sources are added in a separately scoped change, the intended priority is Jellyfin-generated artwork, then existing MVForge artwork/thumbnail, then the generic placeholder. That future priority is not implemented behavior in this change.

#### Scenario: Keep the current generic placeholder when no artwork source is wired
- **WHEN** neither supported artwork URL is available to the selected-detail caller, as with the current Asset/API contract
- **THEN** the selected workspace displays a generic MVForge placeholder without generating new artwork

### Requirement: The shared workspace remains responsive and bounded

The shared tree, table, path toolbar, overflow menus, and selected detail MUST remain usable at desktop, laptop, and narrow viewport widths. They MUST reflow or scroll within their own content region without page-level horizontal overflow or hiding primary actions. Inventory rendering MUST reuse existing grouped inventory and persisted summaries without duplicate inventory queries, per-row ffprobe, filesystem scans, automatic Advisor evaluation, automatic snapshot generation, or automatic preview generation. Deeper persisted detail MAY be loaded lazily for the selected asset only.

#### Scenario: Use the workspace at a narrow width
- **WHEN** the viewport cannot fit hierarchy and inventory side by side
- **THEN** those surfaces and selected detail reflow or stack, primary actions remain reachable, and the page does not overflow horizontally

#### Scenario: Browse a large inventory
- **WHEN** a large RAW or Library inventory is displayed
- **THEN** rows use existing inventory and summary data without row-specific probes, scans, evaluations, previews, snapshots, or duplicate inventory requests

#### Scenario: Load deeper selected detail
- **WHEN** a user selects one asset that has deeper persisted detail available
- **THEN** only that asset's detail is loaded lazily and the rest of the inventory does not receive row-specific requests

### Requirement: The inventory list scrolls independently of page controls

The Assets workspace MUST give inventory rows their own vertical scroll region. Summary cards, page title, filters, path toolbar, pagination, and selected-asset panel MUST remain outside that row-scroll region. The selected-asset panel MUST remain attached to the workspace bottom while the list scrolls, and the Assets route MUST not rely on document-level scrolling for normal inventory browsing.

#### Scenario: Scroll a large inventory
- **WHEN** the visible inventory contains more rows than fit in the list area
- **THEN** the rows scroll inside the list region while summary cards, filters, pagination, and selected detail remain fixed in the workspace
