## Purpose

Defines the responsive Assets V2 workspace for browsing source media in RAW and destination media in Library while preserving each inventory's canonical hierarchy and existing workflows.

## ADDED Requirements

### Requirement: Assets exposes RAW and Library as the only inventory contexts

The Assets workspace MUST expose exactly two canonical top-level inventory contexts, RAW and Library. Workflow, lifecycle, provenance, validation, publication, and evidence concepts MUST remain filters or details within those inventories rather than additional top-level contexts.

#### Scenario: Open either inventory context
- **WHEN** a user opens Assets or selects a context
- **THEN** RAW and Library are the only top-level inventory choices and the selected context is represented semantically as well as visually

#### Scenario: Switch contexts with URL state
- **WHEN** a user switches between RAW and Library
- **THEN** the `location` query value reflects the destination context, common search state remains usable, and filters that only apply to the previous context are removed

#### Scenario: Keep context-local selection isolated
- **WHEN** a user switches inventory contexts
- **THEN** RAW hierarchy state does not filter Library, Library hierarchy state does not filter RAW, and an asset selection is retained only when that asset is present in the destination context

#### Scenario: Open a legacy Assets link
- **WHEN** a user follows a supported legacy Assets URL
- **THEN** the link resolves to RAW or Library with its compatible filter or selection behavior, and no legacy workflow becomes a top-level inventory context

### Requirement: RAW preserves source hierarchy in the V2 workspace

RAW MUST present source inventory using the existing Source Group, Logical Group or title, path, and asset relationships. Selecting a source group, title, or path MUST constrain the visible assets to that context without flattening the hierarchy or changing source identity.

#### Scenario: Browse a nested RAW source
- **WHEN** a user expands a Source Group and selects a Logical Group or path
- **THEN** the hierarchy remains visible, the selected context is identified, and only matching RAW assets appear in the list

#### Scenario: Browse a source with root and nested paths
- **WHEN** a Logical Group contains root assets and child paths
- **THEN** the workspace keeps root assets and child paths distinguishable and lets the user reach assets at each level

#### Scenario: Render RAW without inventory rescans
- **WHEN** a user opens RAW or changes its selected hierarchy node
- **THEN** the workspace uses existing grouped inventory data without a filesystem walk or per-asset media probe

### Requirement: RAW and Library share the scan-first workspace presentation

Both contexts MUST use a compact hierarchy-and-list layout with restrained dividers and row selection. Asset rows MUST omit thumbnails, constrain long filenames, and present unavailable summaries clearly. The Library default list MUST retain the Name, Status, Profile, Subtitles, Duration, and Modified columns. Library lifecycle and provenance values MUST remain represented as row state or filters rather than new top-level contexts.

#### Scenario: Scan the Library list
- **WHEN** a user opens the default Library list
- **THEN** its six scan columns are visible, row thumbnails are absent, and long filenames do not cause page-level horizontal overflow

#### Scenario: Scan RAW assets
- **WHEN** a user opens a RAW path or title
- **THEN** its asset list uses the same compact row density and alignment as Library while retaining source-appropriate status and configuration summaries

#### Scenario: Browse mixed Library states
- **WHEN** Library contains converted, published-as-is, accepted, review, validation, or archived-original evidence
- **THEN** those states remain represented within Library and do not appear as separate top-level tabs

### Requirement: Existing context actions retain their canonical workflows

RAW MUST keep its existing Advisor, analysis where supported, Queue or Batch Preflight, configuration, path or title, asset selection, and Needs Review workflows reachable. Library MUST keep its existing path and asset actions reachable. These actions MUST preserve their existing eligibility, planning, and mutation behavior.

#### Scenario: Use RAW path and title actions
- **WHEN** a user selects a RAW source, title, or path
- **THEN** applicable Advisor, Queue or Batch Preflight, configuration, and path or title actions remain available through compact contextual or overflow controls

#### Scenario: Queue RAW assets
- **WHEN** a user starts Queue from RAW
- **THEN** the existing Batch Preflight and acceptance flow is used and no shortcut bypasses the reviewed plan

#### Scenario: Use Library path actions
- **WHEN** a user selects a Library path
- **THEN** its existing Advisor, Queue, configuration, migration, and secondary actions remain available with their current eligibility rules

### Requirement: Selected asset detail remains concise and uses one Preview action

Selecting an asset MUST show a compact detail surface with available preview, filename, context path, lifecycle status, profile, subtitles, duration, modified time, container, size, video, resolution, audio, and applicable Assign Profile, Test Encode, and secondary actions. Preview MUST use one integrated Play control. Deep Asset Info remains a separate control surface, and descriptive metadata MUST render only when canonical data exists.

#### Scenario: Show selected RAW detail
- **WHEN** a user selects a RAW asset
- **THEN** the selected asset detail shows its source context and available summaries, with RAW-valid actions and no full Asset Info content inline

#### Scenario: Show selected Library detail
- **WHEN** a user selects a Library asset
- **THEN** the selected asset detail shows Library context and available summaries, with existing Assign Profile and Test Encode eligibility preserved

#### Scenario: Play a selected asset preview
- **WHEN** a user activates the selected asset preview
- **THEN** one integrated Play control opens the existing Preview behavior without a duplicate View or Play action

#### Scenario: Hide unavailable descriptive metadata
- **WHEN** the selected asset has no canonical descriptive metadata
- **THEN** no empty metadata section is shown

#### Scenario: Load selected detail efficiently
- **WHEN** a user selects one asset
- **THEN** any deeper persisted detail is requested only for that selected asset and is not fetched or probed for every row

### Requirement: Workspace remains responsive and accessible

The hierarchy, asset list, actions, and selected detail MUST remain usable at common desktop and laptop widths and reflow at narrow widths without page-level horizontal overflow. The context selector, hierarchy controls, row selection, icon actions, and overflow menus MUST be keyboard operable and named; selected state and status meaning MUST not depend on color alone.

#### Scenario: Use the workspace at a laptop width
- **WHEN** the viewport is narrower than a large desktop display
- **THEN** the hierarchy remains usable, the asset list retains most available width, and long names remain constrained

#### Scenario: Use the workspace on a narrow viewport
- **WHEN** the viewport cannot fit hierarchy, list, and detail side by side
- **THEN** those surfaces stack or reflow without hiding primary actions or introducing page-level horizontal scrolling

#### Scenario: Navigate contexts and hierarchy by keyboard
- **WHEN** a user operates Assets without a pointer
- **THEN** RAW and Library selection, hierarchy expansion and selection, asset selection, and overflow actions remain reachable with visible focus and semantic state
