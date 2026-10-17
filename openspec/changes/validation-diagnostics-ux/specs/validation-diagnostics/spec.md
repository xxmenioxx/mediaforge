## Purpose

This capability presents immutable Validation evidence in a readable form for operators and keeps complete technical run data accessible for support and engineering.

## ADDED Requirements

### Requirement: Validation run summary keeps canonical outcome and Publisher handoff distinct

The Validation details view SHALL identify the asset and exact job/run, show the canonical run outcome and score, summarize findings by state or severity, and display effective Publisher handoff as a separate backend-provided result. The presentation SHALL NOT infer handoff eligibility from the run outcome, score, warning color, or overrides.

#### Scenario: Run summary shows canonical and effective outcomes separately
- **WHEN** a user opens a Validation run with a backend effective handoff result
- **THEN** the view shows the asset and run identity, canonical outcome, score, finding summary, and the supplied effective handoff as distinct values

#### Scenario: Historical handoff is not recorded
- **WHEN** a historical run has an effective handoff of `HISTORICAL_ONLY` or another unknown value
- **THEN** the view displays that supplied value distinctly and does not describe the run as currently eligible to publish

### Requirement: Diagnostics are concise, readable, and preserve finding states

The diagnostics view SHALL present findings using Check, Status, Diagnosis, and Action information without embedding nested JSON in table cells. Findings SHALL be ordered severity-first, placing FAIL, WARNING, and UNVERIFIED before PASS. Known rules SHALL use friendly labels and concise explanations; unknown rules SHALL retain their identifier and use a safe generic explanation. The view SHALL distinguish PASS, WARNING, FAIL, UNVERIFIED, REVIEW_REQUIRED, NOT_APPLICABLE, and a manual SKIPPED audit event, and SHALL NOT show SKIPPED as PASS.

#### Scenario: Findings are shown in severity-first order
- **WHEN** a run contains failed, warning, unverified, passed, and not-applicable findings
- **THEN** failed, warning, and unverified findings appear before passed findings, with remaining states still distinct and legible

#### Scenario: Unknown rule remains understandable
- **WHEN** a run contains a rule key with no registered presentation formatter
- **THEN** the diagnostic shows the original key, canonical status, and available explanation with a generic readable value presentation

#### Scenario: Passed findings can be inspected without dominating the view
- **WHEN** a run contains passed findings
- **THEN** the user can filter or expand passed findings while the default diagnostics prioritize findings requiring attention

### Requirement: Finding details explain observed evidence and missing evidence truthfully

An expanded finding SHALL present its explanation, expected and observed values in friendly fields when supported, evidence references or origin, and a next action when the existing finding provides enough information. Missing optional evidence SHALL be identified as UNVERIFIED or unavailable and SHALL NOT be inferred as PASS. Structured video-property and available audio/video per-track evidence, including timing offsets, SHALL remain inspectable. Long identifiers, paths, and track labels SHALL remain readable without forcing page-wide horizontal scrolling.

#### Scenario: Structured A/V evidence is available
- **WHEN** a finding contains structured video properties or per-track A/V timing evidence
- **THEN** the expanded detail shows relevant field names, track labels, units, offsets, tolerances, and evidence origin in readable form

#### Scenario: Historical run omits optional evidence
- **WHEN** a run predates optional per-track evidence or lacks an expected evidence field
- **THEN** the detail identifies the value as unavailable or UNVERIFIED and does not fabricate an observed value or passing status

#### Scenario: Long evidence values are displayed
- **WHEN** a finding contains a long path, track label, or identifier
- **THEN** the value wraps or can be inspected without obscuring status, action controls, or the rest of the page

### Requirement: Override actions remain auditable and do not repair canonical findings

The diagnostics view SHALL preserve the existing per-rule Skip and Skip all overrideable actions, justification and approval flow, and exact-run override matching. A matching override SHALL be shown with its available actor, reason, and time while leaving the underlying finding state and run outcome visible and unchanged.

#### Scenario: Skipped finding remains canonically failed
- **WHEN** an exact-run override covers a failed or warning finding
- **THEN** the view shows both the canonical finding state and a separate SKIPPED audit marker with the override details

#### Scenario: Skip all remains limited to unresolved overrideable findings
- **WHEN** the user selects Skip all overrideable findings
- **THEN** the existing flow targets only unresolved overrideable findings for that run and continues to require its existing justification and authorization

### Requirement: Complete technical run evidence is available on demand

The technical evidence view SHALL expose a readable tree or pretty-printed JSON representation of the complete immutable ValidationRun returned by the existing detail API, including its identity, report, and findings, without altering or omitting run fields. It SHALL support copy and JSON download with the run identity in the exported filename and SHALL provide a usable fallback when the preferred browser clipboard or download mechanism is unavailable. Technical evidence SHALL be separate from the main diagnostics table.

#### Scenario: User inspects and exports immutable run JSON
- **WHEN** the user opens technical evidence for a run
- **THEN** the complete original run object is available to inspect, copy, and download without changing the canonical record

#### Scenario: Copy or download API is unavailable
- **WHEN** a preferred clipboard or file-download browser API is unavailable or rejects the operation
- **THEN** the interface provides an accessible fallback or a clear recoverable error and retains access to the run JSON

#### Scenario: Technical evidence is empty or still loading
- **WHEN** run details are loading, unavailable, or contain no findings
- **THEN** the view communicates the current loading, error, or empty state without presenting incomplete data as a complete export

### Requirement: Validation diagnostics remain responsive and accessible

The Validation details view SHALL remain usable at desktop and narrow viewport widths, support keyboard access to filters, finding expansion, overrides, and technical evidence actions, and pair status color with text or an icon. Loading, empty, and error states SHALL be understandable in English.

#### Scenario: Narrow viewport retains access to diagnostics
- **WHEN** the user opens run details on a narrow viewport
- **THEN** the summary, finding state, diagnosis, action, and technical evidence controls remain reachable without requiring page-wide horizontal scrolling

#### Scenario: Status is conveyed without color alone
- **WHEN** a finding or handoff state is displayed
- **THEN** its meaning is available through text or an accessible icon as well as color
