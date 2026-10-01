## Purpose

Define the LAB V2 Test Encode experimentation workspace while preserving the existing Profile Lab and retained Test Encode authorities.

## ADDED Requirements

### Requirement: LAB SHALL expose selected asset context, profile scope, and one test range

LAB SHALL show compact available context for the selected asset, including its name, location, container, size, resolution, codec, and duration where persisted evidence provides those values. LAB SHALL provide an explicit, visibly selected `Path` or `Asset` scope using the existing profile draft scope. That choice SHALL control the scope of the current profile draft and SHALL NOT create path-wide Test Encode execution. LAB SHALL present one editable sample range and use that range for the next Test Encode. LAB SHALL provide one visually dominant `Run Test Encode` action; secondary supported actions SHALL remain in a compact menu or existing profile workflow.

#### Scenario: Select an asset and inspect its context

- **WHEN** an asset is selected in LAB
- **THEN** its available persisted context is shown in a compact header
- **AND** missing fields are described as unavailable rather than probed for presentation

#### Scenario: Change profile scope

- **WHEN** the user selects `Path` or `Asset`
- **THEN** the existing active profile draft scope changes and the selected value is clear by text and control state
- **AND** the Test Encode remains scoped to the selected asset and sample range

#### Scenario: Edit and run a sample range

- **WHEN** the user edits the start and duration in LAB and runs a Test Encode
- **THEN** the request uses that custom sample window
- **AND** the user does not need to leave LAB to change the window

#### Scenario: Open Asset Info

- **WHEN** the user chooses the supported Asset Info action
- **THEN** LAB navigates to the existing Asset Info flow for the selected asset

### Requirement: LAB SHALL keep profile parameters in one capability-driven editing surface

The parameter workspace SHALL reuse the current video, audio, and track drafts, profile selection, save/review flows, encoder-capability inputs, and canonical configuration behavior. A profile selector SHALL appear once in the visible workspace. Requested and effective values SHALL remain distinguishable. Unsupported controls SHALL remain unavailable and SHALL NOT be inferred from the design mockup.

#### Scenario: Select a saved video profile

- **WHEN** the user selects a video profile in LAB
- **THEN** the existing LAB profile-selection behavior populates the current draft
- **AND** that profile is not selected again in the header

#### Scenario: Choose an encoder-specific control

- **WHEN** a profile or encoder does not support a control
- **THEN** LAB follows its existing capability-driven availability and does not expose a hard-coded mockup control as usable

#### Scenario: Continue editing audio or tracks

- **WHEN** the user opens Audio or Tracks editing
- **THEN** the existing draft, profile save/review, and track-selection behaviors remain available in LAB

### Requirement: LAB SHALL compare the source reference and selected Test Encode

LAB SHALL present equivalent Original and Test Encode result panes. The selected result SHALL be a persisted ready Test Encode for the selected source whose output is available. The comparison SHALL use that run's sample window, synchronize playback position and play/pause where both panes support playback, and keep the Original reference reusable. Selecting an existing result SHALL NOT create another Test Encode or redundantly regenerate the Original reference. An unavailable or unsupported media preview SHALL be identified without changing the retained run.

#### Scenario: Compare a ready Test Encode

- **WHEN** the user selects a ready Test Encode with an available output
- **THEN** the Original and Result panes show the selected run's source window and output at equivalent dimensions
- **AND** the selected run is identified as the result being compared

#### Scenario: Synchronize playback

- **WHEN** the user plays, pauses, or scrubs either supported pane
- **THEN** the other pane follows the equivalent position within the Test Encode sample window

#### Scenario: Select a historical result

- **WHEN** the user selects a different retained Test Encode
- **THEN** LAB updates the result pane, sample-window context, and result evidence from that persisted run
- **AND** it does not create a run or an Original reference as a side effect

#### Scenario: No result is available

- **WHEN** there is no ready retained Test Encode with an available output
- **THEN** the Result pane explains that a test result is not available and keeps the Run Test Encode action reachable

### Requirement: LAB SHALL show honest comparison metrics and canonical evidence

Original metrics SHALL come from existing persisted asset/snapshot evidence. Result size and execution details SHALL come from the selected Test Encode. PSNR, SSIM, VMAF, qualification, and rule explanations SHALL be shown only when the selected run already contains compatible canonical evidence. LAB SHALL distinguish unavailable, stale, and current evidence and SHALL NOT calculate replacement quality scores or reinterpret qualification rules.

#### Scenario: Show source and result size

- **WHEN** source file size and output sample size are available
- **THEN** LAB shows those persisted sizes separately
- **AND** it shows a size difference only when equivalent source and output sample sizes are persisted
- **AND** otherwise it explains that the source sample size is unavailable rather than comparing a full source file with a short Test Encode

#### Scenario: Show canonical quality evidence

- **WHEN** the selected Test Encode contains canonical perceptual evidence or a profile qualification
- **THEN** LAB presents the stored metric values and qualification state with its existing freshness and provenance meaning
- **AND** detailed qualification remains reachable through the existing Test Encode evidence view

#### Scenario: Quality evidence is absent or stale

- **WHEN** the selected run has no compatible quality evidence or the stored qualification is stale
- **THEN** LAB labels evidence as unavailable or stale
- **AND** it does not report a synthetic passing result

### Requirement: LAB SHALL present bounded retained Test Encode history

LAB SHALL display the Original reference and a bounded, recent set of persisted Test Encode runs from the canonical inventory. Each run SHALL communicate its identity, execution state, creation/completion time where available, sample window, output size, retention, and selection for comparison. Active runs SHALL retain their existing cancellation action. History and lifecycle controls SHALL operate on canonical Test Encode records and SHALL NOT create frontend-only retained state.

#### Scenario: Review previous runs

- **WHEN** a selected asset has retained Test Encodes
- **THEN** LAB shows a compact bounded history and identifies the currently compared result
- **AND** selecting a run updates the comparison from the same canonical record

#### Scenario: Cancel an active Test Encode

- **WHEN** a retained Test Encode is waiting or generating
- **THEN** LAB exposes its existing cancellation action and shows its persisted status
- **AND** cancellation does not publish, archive, replace, or delete the source asset

#### Scenario: Preserve retention and cleanup ownership

- **WHEN** the user opens detailed Test Encode management
- **THEN** Keep, Temporary, cleanup, reconciliation, and qualification actions continue to use the existing Test Encode lifecycle UI and backend
- **AND** the LAB history strip does not invent another retention or ownership model

### Requirement: LAB SHALL remain accessible, responsive, and bounded

The workspace SHALL use the existing application shell, theme, and production component patterns. At narrower widths it SHALL stack context, parameters, and comparison panes without page-level horizontal overflow. Keyboard users SHALL be able to change scope, range, parameters, selected result, playback, and secondary actions with visible focus and accessible names. LAB SHALL use existing persisted evidence and preview services; it SHALL NOT perform per-render ffprobe, filesystem scans, duplicate inventory queries, unnecessary preview generation, or unbounded history rendering.

#### Scenario: Use LAB at a narrow viewport

- **WHEN** available width requires a narrow layout
- **THEN** context may wrap, the parameter panel moves above the comparison, and Original/Result panes stack
- **AND** long asset names remain contained without page-level horizontal scrolling

#### Scenario: Operate the workspace with a keyboard

- **WHEN** the user navigates LAB without a pointer
- **THEN** scope, range editing, primary and secondary actions, result selection, and comparison controls remain operable and labeled

#### Scenario: Open LAB with retained history

- **WHEN** the user opens LAB for an asset with many historical runs
- **THEN** inventory access and rendered history remain bounded
- **AND** source or result evidence is not regenerated merely to render history

#### Scenario: Run a LAB Test Encode

- **WHEN** the user runs a Test Encode from LAB
- **THEN** the existing Test Encode executor, cancellation, cleanup, validation, and retention lifecycle remains authoritative
- **AND** the run does not enqueue, publish, archive, or replace a production Library asset
