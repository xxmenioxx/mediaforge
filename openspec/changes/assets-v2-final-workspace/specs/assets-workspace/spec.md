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
- **THEN** the inventory displays the ten RAW columns in the specified order using source and preparation summaries

#### Scenario: Avoid inventing RAW lifecycle state
- **WHEN** existing backend state does not establish a suggested RAW label
- **THEN** the row displays the available canonical state or an unknown/unavailable value and does not infer a lifecycle transition

#### Scenario: Show filename without its path in RAW
- **WHEN** a RAW asset belongs to a selected source path
- **THEN** its Asset cell shows only the filename and the selected hierarchy or breadcrumb supplies the path context

### Requirement: Advisor display uses available canonical evidence only

The Advisor cell MUST use `AdvisorResponse.score` when that result is already available. When no result is available, the cell MUST show an honest unavailable or not-evaluated state and MUST NOT imply a score. Rendering an inventory row MUST NOT start Advisor evaluation. An explicit path-level Advisor workflow MAY create or refresh results through the existing owner.

#### Scenario: Display a cached Advisor result
- **WHEN** an asset already has an available canonical Advisor response
- **THEN** its row displays that response's score without starting another evaluation

#### Scenario: Display Advisor without a result
- **WHEN** no canonical Advisor result is available for an asset
- **THEN** its row displays an unavailable or not-evaluated state without implying that evaluation succeeded

#### Scenario: Render rows without Advisor work
- **WHEN** RAW or Library rows are rendered or refreshed
- **THEN** no automatic Advisor evaluation is triggered for those rows

#### Scenario: Run path-level Advisor explicitly
- **WHEN** a user explicitly starts the supported Advisor action for a path
- **THEN** the existing path-level workflow runs and any persisted results become available to the workspace through its existing data owner

### Requirement: Row and path actions remain distinct and use existing eligibility

Each inventory row MUST expose supported asset actions through one keyboard-accessible overflow menu. Library actions MAY include Open in Lab, Asset Info, Assign Profile, Test Encode, Queue, and Delete. RAW actions MAY include Scan Asset, Analyze, Advisor, Assign Profile, Queue, Open in Lab, Test Encode, Asset Info, and Delete. Only actions supported for the current asset and context may be offered or enabled. Action execution MUST preserve existing backend capability and eligibility decisions and existing confirmation, preflight, and lifecycle flows.

Path-level actions MUST remain in a distinct toolbar rather than being duplicated as row actions. The toolbar MUST show the selected asset count and expose only relevant existing operations, including applicable Scan, Advisor, Queue/Preflight, Assign Profile, Configure Path, move, rename, and delete operations. Destructive path actions MUST be visually separated from routine workflow actions.

#### Scenario: Open supported row actions
- **WHEN** a user opens an asset's overflow menu
- **THEN** the menu exposes one set of supported asset-level actions with accessible labels and preserves their existing workflows

#### Scenario: Enforce current action eligibility
- **WHEN** an asset or path is not eligible for a workflow action
- **THEN** the action follows the existing capability, eligibility, disabled-reason, and confirmation behavior rather than a frontend-only rule

#### Scenario: Distinguish path and row actions
- **WHEN** a user selects a path and an asset within it
- **THEN** path operations and selected-count context appear in the path toolbar while asset operations remain in that row's overflow menu

#### Scenario: Queue through the existing path
- **WHEN** a user starts Queue or Preflight from a path or row
- **THEN** the existing Queue planning or Batch Preflight acceptance flow is used without bypassing reviewed eligibility

### Requirement: Selected Asset Workspace remains below the table

Selecting an asset MUST show a collapsible Selected Asset Workspace below the inventory table; it MUST NOT become a persistent right-side panel. The workspace MUST organize available artwork and preview, filename and collection/series/season context, Estado, Perfil, Subtítulos, Duración, Modificado, container, size, resolution, video codec, and audio summary. Its primary actions MUST include Assign Profile, Test Encode, and More when supported. Secondary actions MAY include Open in Lab, Asset Info, Advisor, Rescan, Rename, and Delete when supported. Preview/play MUST be available through the artwork interaction without a redundant visible Play action elsewhere.

#### Scenario: Show selected asset detail below the table
- **WHEN** a user selects a RAW or Library asset
- **THEN** its collapsible detail workspace appears below the table with available identity, context, status, technical summaries, and eligible actions

#### Scenario: Collapse selected asset detail
- **WHEN** a user collapses the Selected Asset Workspace
- **THEN** the inventory table remains available and the selected asset can still be reached through the existing selection interaction

#### Scenario: Play from the selected artwork
- **WHEN** a user activates the selected asset artwork's Play overlay
- **THEN** the existing preview behavior opens with one integrated Play control and a duration badge remains visible when duration is available

#### Scenario: Show unavailable selected detail safely
- **WHEN** selected-asset summary or preview evidence is missing
- **THEN** the workspace identifies unavailable values without generating artwork, preview, or media analysis to fill the gap

### Requirement: Artwork follows the available-source fallback order

The Selected Asset Workspace MUST prefer Jellyfin-generated artwork when it is already available, then existing MVForge asset artwork or thumbnail, then a generic MVForge placeholder. Artwork selection MUST NOT trigger automatic artwork generation or an unbounded per-row data request.

#### Scenario: Prefer artwork from the highest available source
- **WHEN** artwork is available from Jellyfin and an existing MVForge thumbnail
- **THEN** the selected workspace displays the Jellyfin-generated artwork

#### Scenario: Fall back to existing MVForge artwork
- **WHEN** Jellyfin artwork is unavailable and an existing MVForge asset artwork or thumbnail is available
- **THEN** the selected workspace displays that existing MVForge artwork

#### Scenario: Fall back to a generic placeholder
- **WHEN** neither Jellyfin nor existing MVForge artwork is available
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
