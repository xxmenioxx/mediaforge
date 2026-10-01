## Purpose

Defines the approved-mockup presentation and navigation boundaries for the Assets V2 RAW and Library workspaces while preserving their canonical inventory and workflow behavior.

## ADDED Requirements

### Requirement: Assets V2 follows the approved mockup's visual hierarchy

The Assets workspace MUST present a compact header, an accessible RAW/Library selector, balanced summary cards, a distinct hierarchy panel, a scan-first asset list, and a connected selected-asset detail surface. The shared dark visual language MUST distinguish surfaces and selected state with restrained borders, spacing, and typography without relying on one viewport's pixel coordinates.

#### Scenario: Open the Assets workspace
- **WHEN** a user opens RAW or Library
- **THEN** the page identifies Assets and the active inventory context, shows canonical Total Assets, Processing, Completed, and Needs Review summaries, and separates the workspace from the summary area

#### Scenario: Scan Library assets
- **WHEN** the Library list is visible at a desktop width
- **THEN** the hierarchy and table share the available workspace, the table retains Name, Status, Profile, Subtitles, Duration, and Modified, rows contain no artwork, and long names do not create page-level horizontal scrolling

#### Scenario: Scan RAW assets
- **WHEN** a user browses RAW
- **THEN** Source Group, Logical Group or title, path, and asset remain visibly distinct, the selected node is clear, and the context heading identifies the selected source hierarchy that filters the asset list

#### Scenario: Show selected asset detail
- **WHEN** a user selects a RAW or Library asset
- **THEN** one detail surface groups its preview and single integrated Play action, filename and context, status and profile, subtitle and timing information, technical summaries, and eligible actions without rendering full Asset Info inline

#### Scenario: Keep inventory rendering bounded
- **WHEN** a large RAW or Library inventory is displayed
- **THEN** list rows do not trigger per-row previews, deep detail requests, media probes, or filesystem scans, and deeper persisted detail is loaded only for the selected asset where supported

### Requirement: RAW and Library context state stays accessible and isolated

The RAW and Library selector MUST be visible in Assets, keyboard operable, and expose its active state semantically as well as visually. Switching contexts MUST retain common search state where supported and MUST NOT carry hierarchy selection or incompatible filters into the destination context.

#### Scenario: Switch between RAW and Library
- **WHEN** a user switches either direction
- **THEN** the URL identifies the destination location, the active selector state updates, and hierarchy and context-specific filters from the source location do not constrain the destination

#### Scenario: Operate the selector and hierarchy by keyboard
- **WHEN** a user navigates the workspace without a pointer
- **THEN** the location selector and hierarchy controls remain operable with visible focus and announced selected or expanded state

### Requirement: Reports remains a separate primary navigation destination

Reports MUST be reachable as its own `/reports` route and MUST NOT be represented as an Assets inventory context or tree node. Exactly one primary navigation destination MUST be active at a time.

#### Scenario: Open Assets in either inventory location
- **WHEN** the current route is `/assets` with RAW or Library selected, including supported Assets query state
- **THEN** Assets is the sole active primary destination and Reports is inactive

#### Scenario: Open Reports
- **WHEN** the current route is `/reports` or a nested Reports route
- **THEN** Reports is the sole active primary destination and Assets is inactive

#### Scenario: Follow a legacy Reports link
- **WHEN** a user opens a legacy Assets URL that selected the report view through query or tab state
- **THEN** the user reaches the separate Reports destination without adding Reports to RAW/Library inventory choices
