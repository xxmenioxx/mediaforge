## ADDED Requirements

### Requirement: ASS and SSA global presets expose effective numeric styling

Settings SHALL expose font size, auto font size, outline width, and shadow depth for each built-in and custom subtitle style preset. The editor SHALL display saved and draft values using ASS style units and SHALL allow zero outline and zero shadow. Accepted values SHALL respect the canonical backend bounds: font size 0–200, outline width 0–20, and shadow depth 0–20.

#### Scenario: Edit a built-in or custom preset
- **WHEN** an operator selects a built-in or custom preset in subtitle Settings
- **THEN** the editor shows its effective saved font size, auto-font-size state, outline width, and shadow depth
- **AND** the operator can change those values in the local draft without changing the preset identity or other subtitle settings

#### Scenario: Save numeric styling values
- **WHEN** an operator saves valid numeric values, including outline width 0 or shadow depth 0
- **THEN** the existing versioned subtitle styling setting persists those values through the canonical settings contract
- **AND** values outside the backend bounds are rejected by the canonical validator

### Requirement: Subtitle style preview explains authored units and approximates output

The existing subtitle style preview SHALL react to the selected draft's font size, auto-size result, outline width and color, shadow depth and color, bold state, primary color, and opacity. It SHALL show a dialogue example against light and dark backgrounds, identify its PlayRes reference, and explain that browser rendering approximates libass and player rendering.

#### Scenario: Preview a style draft
- **WHEN** an operator edits a style preset
- **THEN** the preview reflects the draft values without saving them
- **AND** the UI explains that ASS style units are interpreted in the context of PlayResX/PlayResY and are not screen pixels

### Requirement: Auto font size remains honestly described

The subtitle styling editor SHALL describe the actual deterministic behavior of auto font size and SHALL NOT claim that it adapts to source resolution unless a resolution-aware rule exists. A zero font size SHALL be described consistently with the canonical writer's effective fallback.

#### Scenario: Use the fixed auto-size fallback
- **WHEN** auto font size is enabled or a zero font size is resolved by the current writer
- **THEN** the editor identifies the effective 20 ASS-unit fallback and states that it is not resolution-adaptive

### Requirement: Saving global subtitle styling does not rewrite source files

Saving or resetting global subtitle style settings SHALL update only the existing settings value. It SHALL NOT rewrite, restyle, republish, or otherwise mutate existing SRT, ASS/SSA, PGS, VobSub, embedded subtitle tracks, or video. Native ASS/SSA styles SHALL remain source-owned unless an explicit eligible Restyle operation is requested under the existing native override policy.

#### Scenario: Save settings while sidecars exist
- **WHEN** an operator saves or resets global subtitle styling while subtitle files already exist
- **THEN** the save changes only the existing settings record and leaves those source files unchanged
- **AND** any native ASS/SSA style mutation remains gated by the existing explicit Restyle authorization
