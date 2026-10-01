## Purpose

Defines the approved-mockup presentation and navigation boundaries for the Assets V2 RAW and Library workspaces while preserving their canonical inventory and workflow behavior.

## ADDED Requirements

### Requirement: Assets V2 follows the approved mockup's visual hierarchy

The Assets workspace MUST present a compact header, balanced summary cards, one accessible Assets hierarchy, a scan-first asset list, and a connected selected-asset detail surface. The hierarchy MUST show the RAW source root and each configured Library root as siblings in the same tree. Tree selection MUST choose the active inventory context; a separate RAW/Library toggle MUST NOT be rendered. The shared dark visual language MUST distinguish surfaces and selected state with restrained borders, spacing, and typography without relying on one viewport's pixel coordinates.

#### Scenario: Open the Assets workspace
- **WHEN** a user opens Assets
- **THEN** the page identifies Assets and the active context, shows canonical Total Assets, Processing, Completed, and Needs Review summaries, and separates the workspace from the summary area

#### Scenario: Show RAW and configured Library roots together
- **WHEN** the Assets hierarchy is rendered
- **THEN** one accessible tree contains a visibly identified RAW source root and every configured Library root as sibling top-level nodes, using each configured root name without a generic synthetic Library root

#### Scenario: Select a RAW root or descendant
- **WHEN** a user selects RAW, a source group, logical group, or path
- **THEN** the content shows the matching canonical RAW inventory scope, `location=raw` is set, and available source-group, logical-group, and path identity is preserved in the URL; selecting the RAW root records its all-RAW scope without retaining descendant keys

#### Scenario: Select a configured Library root or descendant
- **WHEN** a user selects a configured Library root or one of its descendants
- **THEN** the content shows that root or path scope, `location=library` and `libraryId` identify the root, `libraryPath` identifies a descendant when selected, and RAW query keys are absent

#### Scenario: Scan Library assets
- **WHEN** the Library list is visible at a desktop width
- **THEN** the hierarchy and table share the available workspace, the table retains Name, Status, Profile, Subtitles, Duration, and Modified, rows contain no artwork, and long names do not create page-level horizontal scrolling

#### Scenario: Scan RAW assets
- **WHEN** a user browses RAW
- **THEN** Source Group, Logical Group or title, path, and asset remain visibly distinct, the selected node is clear, and the context heading identifies the selected source hierarchy that filters the asset list

#### Scenario: Stack the workspace at a narrow width
- **WHEN** the Assets workspace is narrower than its desktop layout
- **THEN** the hierarchy, table, and selected detail stack without page-level horizontal scrolling, and long names remain contained

#### Scenario: Show selected asset detail
- **WHEN** a user selects a RAW or Library asset
- **THEN** one detail surface groups its preview and single integrated Play action, filename and context, status and profile, subtitle and timing information, technical summaries, and eligible actions without rendering full Asset Info inline

#### Scenario: Keep inventory rendering bounded
- **WHEN** a large RAW or Library inventory is displayed
- **THEN** list rows do not trigger per-row previews, deep detail requests, media probes, or filesystem scans, and deeper persisted detail is loaded only for the selected asset where supported

### Requirement: Unified tree context stays accessible and isolated

The Assets hierarchy MUST be the only visible RAW/Library context selector. Its root and descendant selection MUST be keyboard operable, visibly indicated, and exposed with accessible selected/expanded state. RAW and Library MUST retain their distinct canonical selection identities behind the shared presentation. Crossing contexts MUST retain common search state where supported and MUST clear incompatible hierarchy keys and Library-only filters as appropriate.

#### Scenario: Cross from RAW to Library
- **WHEN** a user selects a configured Library root while in RAW
- **THEN** the URL sets `location=library`, removes `sourceGroup`, `logicalGroup`, `path`, and `rawScope` state, preserves common search, and scopes the content to the selected Library root

#### Scenario: Cross from Library to RAW
- **WHEN** a user selects RAW or a RAW descendant while in Library
- **THEN** the URL sets `location=raw`, removes Library root/path, provenance, media-area, validation, original, workflow, attention, and Library evidence filters, preserves common search, and scopes the content to the selected RAW identity using `rawScope=all` for the RAW root or canonical source-group/logical-group/path keys for a descendant

#### Scenario: Operate the unified hierarchy by keyboard
- **WHEN** a user navigates the workspace without a pointer
- **THEN** tree roots and descendants support keyboard selection, expansion, collapse, and navigation with visible focus and announced selected or expanded state

### Requirement: Reports remains a separate primary navigation destination

Reports MUST be reachable as its own `/reports` route and MUST NOT be represented as an Assets inventory context or tree node. Converted and Archive MUST NOT be top-level Assets hierarchy nodes. Exactly one primary navigation destination MUST be active at a time.

#### Scenario: Open Assets in either inventory location
- **WHEN** the current route is `/assets` with RAW or Library selected, including supported Assets query state
- **THEN** Assets is the sole active primary destination and Reports is inactive

#### Scenario: Open Reports
- **WHEN** the current route is `/reports` or a nested Reports route
- **THEN** Reports is the sole active primary destination and Assets is inactive

#### Scenario: Follow a legacy Reports link
- **WHEN** a user opens a legacy Assets URL that selected the report view through query or tab state
- **THEN** the user reaches the separate Reports destination without adding Reports to RAW/Library inventory choices
