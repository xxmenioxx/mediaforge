## Purpose

Publisher presents current actionable work before historical detail while preserving bounded structured evidence and access to the complete audit history.

## ADDED Requirements

### Requirement: Publisher prioritizes a unified current-state asset list

The Publisher view SHALL present eligible jobs and persisted publication records in one primary list, with filename or job identity and frozen destination visible. It SHALL provide search by filename, job, or destination and state filters. The default view SHALL order active work as Ready, Publishing, Failed/Blocked, then Held, across both read models, and SHALL make Published records available through an explicit filter or history context.

#### Scenario: Eligible work is visible on initial load
- **WHEN** Publisher eligibility contains eligible jobs and publication summaries contain older records
- **THEN** the primary list initially shows active or attention-needed eligible work before historical published-only records

#### Scenario: Active states follow operational priority
- **WHEN** the bounded eligibility and publication responses contain Ready, Publishing, Failed/Blocked, and Held records
- **THEN** the unified list orders those records by Ready, Publishing, Failed/Blocked, and Held, regardless of which response supplied each row

#### Scenario: User filters and searches current state
- **WHEN** the user selects a publication-state filter or searches by filename, job, or destination
- **THEN** the single list shows only matching records while keeping eligibility disposition distinct from publication state

#### Scenario: Published history is explicitly selected
- **WHEN** the user selects Published or opens full History
- **THEN** published records or the complete audit history remain accessible

### Requirement: Unified Publisher records preserve exact identity and bounded evidence

The Publisher view SHALL correlate an eligibility job with a publication only when Queue job and execution identities match exactly. Distinct publication IDs and their attempts, evidence, and audit facts SHALL remain distinct. The view SHALL identify eligibility-only, publication-only, and historical records without inferring identity from filename or destination resemblance. Counts and summaries from bounded API windows SHALL be labeled as bounded and SHALL NOT be represented as global totals.

#### Scenario: Same execution appears in both read models
- **WHEN** an eligibility job and one or more publication records refer to the same Queue job and execution
- **THEN** the view avoids a duplicate eligibility-only row while preserving each distinct publication ID and its evidence

#### Scenario: Historical publication belongs to a replaced execution
- **WHEN** a publication's execution differs from the currently eligible execution for the same job identifier
- **THEN** both records remain distinct and no current eligibility or evidence is overwritten

#### Scenario: Read model windows are bounded
- **WHEN** the displayed data comes from bounded eligibility and publication responses
- **THEN** summary counts and evidence are described as bounded-window information rather than complete global counts or history

### Requirement: Publisher evidence remains on demand and actions retain backend authority

The Publisher view SHALL keep attempt, archive, validation, and audit summaries available without placing full evidence ahead of current work, and SHALL link users to the complete History route. Publish, Hold, Release, Retry, and Discard SHALL retain their existing confirmation, reason, eligibility, and backend authorization behavior. Mutations SHALL refresh affected Publisher and Queue data.

#### Scenario: User opens row evidence
- **WHEN** the user expands or opens a Publisher row's details
- **THEN** available bounded attempt, archive, validation, and audit summaries are shown with timestamps and publication identity intact

#### Scenario: Backend rejects an action
- **WHEN** an action appears available in the UI but current backend eligibility or authorization rejects it
- **THEN** the rejection is surfaced and no UI state is treated as authorization

#### Scenario: Mutation succeeds
- **WHEN** a Publisher action succeeds
- **THEN** affected read models refresh and the persisted response remains the source of displayed state
