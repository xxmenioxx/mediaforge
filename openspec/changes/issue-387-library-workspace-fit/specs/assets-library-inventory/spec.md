## Purpose

Defines how the Library asset inventory remains readable and operable within the responsive Assets workspace while preserving access to asset information and row controls.

## ADDED Requirements

### Requirement: Library inventory fits its workspace

The Library inventory SHALL adapt to its available workspace width without causing horizontal overflow of the Assets page or hiding interactive row controls. Secondary technical fields MAY be visually collapsed at constrained widths only when their information remains available through existing asset detail views. The filename cell, selection controls, and row actions SHALL remain usable.

#### Scenario: Typical desktop workspace
- **WHEN** the Library is displayed at a supported desktop viewport with the hierarchy panel open
- **THEN** the inventory fits the remaining content area and the rightmost row actions remain visible without page-wide horizontal scrolling

#### Scenario: Constrained workspace or resized hierarchy panel
- **WHEN** the viewport narrows or the hierarchy panel is resized so the Library content area becomes constrained
- **THEN** secondary technical fields progressively collapse while filename, selection, status, and row actions remain reachable

#### Scenario: Long filename or path
- **WHEN** a Library asset has a filename or path longer than its visible cell
- **THEN** the compact filename cell remains within the row, provides its existing full-value affordance, and does not increase row height

#### Scenario: Large inventory
- **WHEN** the Library contains 100 or more assets
- **THEN** the current paginated inventory remains bounded to the selected page and its responsive layout does not expand the workspace width

#### Scenario: RAW compatibility
- **WHEN** the RAW inventory is displayed
- **THEN** its columns and layout remain unchanged unless an identical RAW defect is separately established
