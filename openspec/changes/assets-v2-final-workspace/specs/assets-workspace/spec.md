## Purpose

Defines one accessible Assets workspace for browsing RAW sources and configured Libraries while preserving canonical inventory, Advisor, Queue, profile, and lifecycle behavior. The current local Assets V2 Figma Make export is the visual authority; the current selected-asset composition reference is `docs/design-reference/assets-v2/asstesv2.png` (its checked-out filename contains the existing typo).

## ADDED Requirements

### Requirement: Shared workspace uses the current desktop composition

The Assets route MUST render a full-width top row containing the Assets title, compact Search, and supported top actions. Below it, one workspace MUST place the Asset Library hierarchy at left and the right inventory workspace at right. The right side MUST order the summary strip, one breadcrumb/path-action bar, compact inventory controls, independently scrolling asset list, and selected-asset context. The hierarchy MUST NOT appear beside the top row. Search MUST retain its existing query behavior and MUST NOT be buried in a legacy filter header.

#### Scenario: Render the desktop workspace
- **WHEN** the Assets route is displayed at desktop width
- **THEN** Assets and Search occupy the full-width top row, and the hierarchy and right workspace begin below that row

#### Scenario: Keep summary and path context in the right workspace
- **WHEN** Library or RAW inventory is displayed
- **THEN** summary cards appear above a single compact breadcrumb/path-action bar, followed by inventory controls and rows

### Requirement: One canonical hierarchy has Figma presentation grouping

The Assets page MUST keep one shared workspace and one canonical hierarchy. Its visible top-level sections MUST be `RAW` and `Libraries`; configured destination Library roots MUST appear below the presentation-only `Libraries` section. RAW source groups and descendants MUST remain under RAW. The presentation wrapper MUST NOT change backend identities or context selection. The hierarchy MUST NOT expose RAW/Library tabs or add Converted, Archive, or Reports as inventory roots. The Asset Library header MAY expose the existing `/libraries` configuration route through its plus control.

#### Scenario: Browse RAW and configured Libraries
- **WHEN** the hierarchy is displayed
- **THEN** its visible root rows are RAW and Libraries, and configured Library roots appear under Libraries

#### Scenario: Select a hierarchy path
- **WHEN** a user selects a RAW or Library root or descendant
- **THEN** the matching inventory appears, path context is shown in the hierarchy or breadcrumb, and the canonical identity is preserved

#### Scenario: Configure a Library from the hierarchy
- **WHEN** the user activates the Asset Library plus control
- **THEN** it opens the existing Libraries configuration route without creating a new configuration operation

### Requirement: The hierarchy is resizable, persistent, and keyboard accessible

The hierarchy panel MUST default to approximately 262px and allow resizing from approximately 220px to 390px. Its width MUST persist locally across reloads and MUST be clamped to the supported range when a stored value is outside it. The resize control, tree selection, expansion, and collapse MUST be keyboard-operable and expose accessible names, focus, and state.

#### Scenario: Resize and restore the hierarchy
- **WHEN** a user resizes the hierarchy and reloads Assets
- **THEN** the bounded width is restored locally without changing RAW or Library selection

#### Scenario: Recover from an invalid stored width
- **WHEN** stored hierarchy width is out of range or unreadable
- **THEN** the hierarchy uses a supported width and remains usable

#### Scenario: Navigate and resize without a pointer
- **WHEN** a user operates the tree and separator by keyboard or assistive technology
- **THEN** navigation, expansion, selection, resizing, and visible focus remain available

### Requirement: Summary cards use canonical counts

The right workspace MUST show Total Assets, Processing, Completed, and Needs Review. Counts MUST come from existing canonical inventory and lifecycle data. The UI MUST NOT invent trends, percentages, capacity usage, or other values absent from a canonical source.

#### Scenario: Show summary data
- **WHEN** the Assets workspace is rendered
- **THEN** the four summary cards show current supported counts without fabricated trend or percentage values

### Requirement: Existing filters are compact and controls stay above the list

Provenance, media area, validation, original, workflow, and attention filters MUST remain available through a compact Filter popover rather than a large legacy header. Historical or missing-sync details MUST NOT dominate the browsing workspace. The compact control row immediately above the list MUST show asset count and supported Sync and Filter actions. Sort MUST remain disabled with an explanation until safe sorting support exists.

#### Scenario: Open inventory filters
- **WHEN** the user activates Filter
- **THEN** the existing filter controls open in a compact popover and preserve their query behavior

#### Scenario: Use inventory controls
- **WHEN** the control row is displayed
- **THEN** count, Sync, and Filter are available, while unsupported Sort is visibly disabled

### Requirement: Library and RAW inventories share a compact, filename-only table

The Library inventory MUST display these columns in order: Asset, Advisor, Status, Codec, Resolution, Encoder, Duration, Bitrate, HDR/SDR, Size, Modified, Actions. The RAW inventory MUST display: Asset, Advisor, Status, Source / Codec, Resolution, Duration, Bitrate, Size, Modified, Actions. The Asset cell MUST contain the filename only; selected hierarchy and breadcrumb context provide the path. Rows MUST use compact bordered surfaces with subtle hover and selected states, thin row separators, and no artificial vertical gaps. Inventory data MUST come from the existing shared inventory and persisted summaries.

#### Scenario: Render Library columns
- **WHEN** a configured Library is selected
- **THEN** all twelve Library columns appear in the specified order

#### Scenario: Render RAW columns
- **WHEN** a RAW source or descendant is selected
- **THEN** all ten RAW columns appear in the specified order with source/preparation semantics

#### Scenario: Show filename without its path
- **WHEN** an asset row is rendered below a nested path
- **THEN** its Asset cell contains only the filename

#### Scenario: Show missing summaries honestly
- **WHEN** an inventory summary is absent
- **THEN** its cell communicates the unavailable value without row-specific probing or fetching

### Requirement: Advisor uses canonical cached evidence and an explicit visual state

The Advisor cell MUST use `AdvisorResponse.score` when that canonical result is already available. An available score MUST be shown compactly with a short subtle indicator whose fill represents that score. Not evaluated and unavailable/off states MUST use distinct neutral text and MUST NOT show a score indicator. Rendering a row, expanding a path, sorting, paginating, refreshing, or selecting a tree node MUST NOT start Advisor evaluation. Explicit path-level Advisor MAY populate the existing row cache key `['advisor', 'asset-row', asset.path, profileId]` through its existing action owner.

#### Scenario: Display a cached Advisor score
- **WHEN** an asset has a cached canonical Advisor result
- **THEN** its score and compact indicator appear without another evaluation

#### Scenario: Distinguish empty and unavailable Advisor states
- **WHEN** no result exists or Advisor is off/unavailable
- **THEN** restrained text identifies the actual state and no fake score bar appears

#### Scenario: Keep Advisor evaluation explicit
- **WHEN** rows render or a group expands
- **THEN** no Advisor evaluation runs

#### Scenario: Run path Advisor explicitly
- **WHEN** the user starts the supported path Advisor action
- **THEN** the existing workflow runs and available responses populate the existing row cache without row-level evaluation

### Requirement: Path actions share the breadcrumb bar and existing handlers

Path actions MUST remain separate from the row overflow menu and MUST render with the current breadcrumb as one compact visual bar. Routine actions MUST be grouped first; supported destructive path operations MUST follow a visual separator. Selected count MUST appear only when operationally useful. The toolbar MUST reuse existing callbacks, dialogs, mutations, capability checks, and lifecycle-safe behavior rather than duplicate eligibility logic.

For applicable Library paths, show supported Scan, Queue/Preflight, Move path, Rename path, Delete selected, and Delete path actions. The current migration flow targets an existing destination Library. It does not provide a distinct additional Library “Move to library” operation. For supported RAW path/title contexts, expose the existing Scan, explicit Advisor, Queue/Preflight, Configure Path, and Move to library actions when their current owners and eligibility allow them. The title/source presentation MUST NOT imply that every path action is supported at every hierarchy level. RAW path Rename is unsupported by the current path action contract and MUST NOT be fabricated. Destructive delete/return actions MUST remain capability- and lifecycle-gated.

#### Scenario: Keep breadcrumb and path actions in one bar
- **WHEN** a path or title context is selected
- **THEN** its breadcrumb and applicable path actions appear inside one compact path toolbar

#### Scenario: Use supported Library path actions
- **WHEN** an eligible Library path is selected
- **THEN** supported Scan, Queue/Preflight, migration, rename, selected deletion, and path deletion actions reuse current owners

#### Scenario: Use supported RAW path actions
- **WHEN** an eligible RAW path or title is selected
- **THEN** the toolbar exposes only the supported Scan, explicit Advisor, Queue/Preflight, Configure Path, and Move to library operations for that scope

#### Scenario: Keep destructive actions separate and safe
- **WHEN** delete or return operations are available
- **THEN** they are visually separated and continue to use existing confirmations, capabilities, and lifecycle protections

### Requirement: Row overflow actions remain canonical and capability-gated

Each compact Library or RAW row MUST expose supported asset operations through one accessible `(...)` menu. Library actions MAY include Open in Lab, Asset Info, Assign Profile, Test Encode, Queue/replacement, Advisor, and lifecycle-safe Delete. RAW actions MAY include Scan/Analyze, Advisor, Assign Profile, Queue, Open in Lab, Test Encode, Asset Info, and supported Delete. Each action MUST route through its existing dialog, Queue flow, capability contract, and backend eligibility owner. Unsupported actions MUST NOT be invented, and destructive actions MUST be visually separated in the menu.

#### Scenario: Open compact row actions
- **WHEN** the user opens a row overflow menu
- **THEN** only actions supported for that asset and context are offered or enabled

#### Scenario: Preserve existing action behavior
- **WHEN** an action is selected
- **THEN** the existing route/dialog/workflow executes and backend capability or eligibility remains authoritative

### Requirement: Selected Asset context remains attached below the scrolling inventory

When an asset is selected, a collapsible detail context MUST remain below the table and attached to the bottom of the Assets workspace while rows scroll. It MUST remain outside the inventory scroll region and MUST NOT become a modal or persistent right panel. The expanded desktop composition MUST be compact and horizontal: landscape preview with centered Play overlay and duration badge; identity/context with filename, series/collection/season/path, and compact category/Library, container, size, resolution, and codec facts; a separate Estado/Perfil/Subtítulos/Duración/Modificado group; and an Información técnica group for video codec, resolution, audio, and supported Assign Profile, Test Encode, and overflow actions. Do not show a dedicated workspace title, Quick facts/Status headings, or a redundant prominent status badge. Artwork MUST NOT dominate the panel height. Narrow layouts MUST reflow all content without page-level overflow. Collapse MUST leave a compact reopen affordance.

#### Scenario: Show selected asset context
- **WHEN** the user selects a RAW or Library asset
- **THEN** its collapsible context appears outside the inventory scroll region with the available identity, status, technical data, and eligible actions

#### Scenario: Keep context visible while rows scroll
- **WHEN** a long inventory list scrolls
- **THEN** selected detail remains attached below the list and does not cover its final rows

#### Scenario: Collapse and reopen selected context
- **WHEN** the user collapses the panel
- **THEN** filename and an accessible reopen control remain visible, and reopening restores the detail panel

#### Scenario: Play through artwork
- **WHEN** the user activates the preview artwork
- **THEN** the existing preview opens through one integrated Play control with the available duration badge

### Requirement: Artwork uses only supported sources

The current `Asset`/API contract and selected-detail caller expose no supported per-asset Jellyfin artwork or MVForge thumbnail URL. Until such a source reaches the caller, the selected context MUST show the generic placeholder. It MUST NOT use source-preview playback as artwork or generate artwork, preview, or row-level image requests. If a supported source is provided in a separately scoped change, the intended fallback order is Jellyfin artwork, existing MVForge artwork/thumbnail, then placeholder; this order is not implemented by the current contract.

#### Scenario: Render the current artwork placeholder
- **WHEN** no supported artwork URL exists
- **THEN** the generic placeholder appears without generation or unrelated sample art

### Requirement: Legacy Analysis Decisions live under Assets Settings

The main Assets browsing workspace MUST NOT render Legacy Analysis Decisions. Existing records MUST remain available in the Assets-specific Settings section and MUST NOT be deleted or rewritten as part of this visual change.

#### Scenario: Browse Assets without legacy decision history
- **WHEN** the main Assets workspace is displayed
- **THEN** Legacy Analysis Decisions are absent from normal browsing controls

#### Scenario: Reach preserved decisions from Settings
- **WHEN** the user opens Assets Settings
- **THEN** existing Legacy Analysis Decisions remain reachable from their collapsible section

### Requirement: Sidebar branding and navigation retain the current shell

The global navigation MUST use the current Figma's compact MVForge gradient triangle mark and restrained icon/spacing treatment while preserving existing routes, sections, responsive navigation, and active-state behavior. The mark MAY be rendered in CSS because the current reference defines it as a gradient geometric mark rather than a supplied logo file. The implementation MUST NOT copy the Figma Make framework or dependencies.

#### Scenario: Navigate from the branded shell
- **WHEN** the Assets route is opened from desktop or mobile navigation
- **THEN** MVForge branding appears and existing navigation destinations and behavior remain available

### Requirement: The workspace remains responsive and avoids row-specific work

The hierarchy, toolbar, table, and selected context MUST remain usable at desktop, laptop, and narrow widths. Dense table columns MAY scroll horizontally inside the table surface; hierarchy and selected context MUST reflow or stack without page-level horizontal overflow. Inventory display MUST reuse existing queries and persisted evidence and MUST NOT add per-row ffprobe, filesystem scans, Advisor evaluation, snapshot generation, preview generation, duplicate inventory requests, or unbounded row-specific fetching. Deeper supported detail MAY remain lazy for the selected asset only.

#### Scenario: Browse a large inventory
- **WHEN** many assets are present
- **THEN** only the inventory region scrolls vertically while summary, toolbar, controls, hierarchy, and selected context remain outside that scroll region

#### Scenario: Load deeper selected detail
- **WHEN** the user selects one asset with deeper persisted detail
- **THEN** only that asset's detail may be fetched lazily

#### Scenario: Use a narrow viewport
- **WHEN** the viewport cannot fit the desktop two-column workspace
- **THEN** hierarchy and content reflow, and all table/action/detail content remains reachable without page-level horizontal overflow

## Explicit visual/action gaps

- Safe sorting is not currently supported; Sort is disabled.
- No existing per-asset artwork URL reaches the selected-detail caller; the placeholder remains until a supported source is exposed.
- Library migration is one existing Move path operation; the current backend does not expose a separate second Library “Move to library” operation.
- RAW path Rename has no current supported owner.
- The reference's account/avatar, notifications, storage-capacity percentage, and trend figures are not current product contracts and are not fabricated.
- Some source/title scopes do not own every path action shown by the reference; only actions with existing owners are shown.
