## Purpose

Defines the compact Assets Library workspace used to navigate existing Library paths, scan assets, and open the current asset workflows without changing their backend authority.

## ADDED Requirements

### Requirement: Assets workspace navigation and summary
The Assets page MUST retain the application's existing navigation behavior and present compact inventory summaries for Total Assets, Processing, Completed, and Needs Review using available canonical inventory counts.

#### Scenario: Open the Assets workspace
- **WHEN** a user opens the Assets page
- **THEN** Assets is shown as the active application destination and the page presents compact summary values without a dashboard-sized panel

#### Scenario: Navigate with a collapsed sidebar
- **WHEN** the application sidebar is collapsed
- **THEN** its destinations remain reachable through labeled icon controls and the existing mobile drawer remains usable

### Requirement: Library hierarchy and scan-first asset list
The Library workspace MUST prioritize a hierarchical Library/path navigator beside an asset list that uses the available horizontal space. The default list MUST show Name, Status, Profile, Subtitles, Duration, and Modified, MUST omit per-row artwork, and MUST keep long names from causing page-level horizontal overflow. Selecting a Library path MUST filter the displayed assets to that context while preserving existing URL filters and row selection behavior.

#### Scenario: Browse a nested Library path
- **WHEN** a user selects a Library, folder, or nested path in the hierarchy
- **THEN** the asset list shows assets belonging to that selected context and the selected path remains visibly identified

#### Scenario: Scan the default asset list
- **WHEN** the default Library asset list is displayed
- **THEN** its six scan columns are visible, filenames are truncated with an accessible way to inspect the full name, and rows contain no artwork thumbnails

#### Scenario: Display unavailable summary evidence
- **WHEN** a list asset has no persisted profile, subtitle, or duration summary
- **THEN** the corresponding cell communicates that the value is unavailable without probing or fetching deep detail for every row

#### Scenario: Preserve asset selection
- **WHEN** a user selects an asset row
- **THEN** that asset becomes the current selection and remains selected through ordinary rerenders and query refreshes while it remains in the visible result set

### Requirement: Contextual path and asset actions
The selected Library path MUST expose Advisor as a directly discoverable action, with Queue and path configuration actions available from a compact contextual surface. Less common path and asset actions MUST remain reachable from keyboard-accessible overflow or context menus. Assign Profile and Test Encode MUST continue to enter their existing workflows, and Preview MUST have one Play control integrated into the selected asset preview surface.

#### Scenario: Use path Advisor and Queue actions
- **WHEN** a Library path is selected
- **THEN** Advisor is directly available and current Queue and path configuration actions remain reachable without changing their existing semantics

#### Scenario: Open selected asset actions
- **WHEN** an asset is selected
- **THEN** its compact detail strip provides Assign Profile, Test Encode, and a single integrated Preview/Play control that opens the existing experiences

#### Scenario: Reach secondary actions with a keyboard
- **WHEN** a user opens a path or asset overflow menu using the keyboard
- **THEN** all current secondary actions remain labeled, focusable, and operable

### Requirement: Compact selected asset detail and conditional metadata
Selecting an asset MUST reveal a compact detail strip with its filename, Library/path context, status, assigned profile when known, subtitle summary, duration, modified time, container, size, video codec, resolution, and available audio summary. Deeper persisted detail MAY be loaded for the selected asset only. Descriptive metadata and NFO-derived information MUST occupy space only when canonical data exists; the workspace MUST NOT reserve an empty metadata panel.

#### Scenario: Show available selected-asset details
- **WHEN** a user selects an asset with persisted technical or descriptive evidence
- **THEN** the detail strip shows the available evidence compactly and keeps unavailable values distinguishable

#### Scenario: Hide absent metadata
- **WHEN** an asset has no canonical descriptive metadata or NFO-derived data
- **THEN** no empty metadata section is rendered

#### Scenario: Load selected detail without list-wide probing
- **WHEN** a user selects one asset
- **THEN** any deeper persisted summary request is limited to that selected asset and no per-row probing or metadata request is started for the rest of the list

### Requirement: Responsive and accessible workspace
The Library hierarchy, list, path actions, and selected detail MUST remain usable at common desktop and laptop widths and adapt at narrow widths without page-level horizontal overflow. Interactive tree items, table rows, icon-only controls, status values, and overflow menus MUST provide keyboard access and accessible names; status MUST remain understandable without color.

#### Scenario: Use the workspace at a laptop width
- **WHEN** the viewport is narrower than a large desktop monitor
- **THEN** the tree remains usable, the asset list receives most available width, and long names do not force page-level horizontal scrolling

#### Scenario: Use the selected detail on a narrow viewport
- **WHEN** the viewport is narrow
- **THEN** the selected detail reflows its fields and actions into available width without a wide fixed panel

#### Scenario: Operate the workspace without a pointer
- **WHEN** a user navigates with a keyboard or assistive technology
- **THEN** tree selection, row selection, icon actions, menus, and readable status text remain available
