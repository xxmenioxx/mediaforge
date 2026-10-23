## ADDED Requirements

### Requirement: Translation quality analysis is advisory and follows structural validation

The canonical translation operation SHALL run deterministic quality checks only after the translated cues pass existing structural validation. Findings SHALL be bounded, tied to canonical cue identity, and advisory; a heuristic alone SHALL NOT reject otherwise structurally valid output or change its cue IDs, count, timing, ASS structure, or publication state. The analysis SHALL distinguish source OCR uncertainty from translation quality and SHALL NOT treat translation as repair of uncertain source text.

#### Scenario: Structurally valid translation receives review signals

- **WHEN** translated cues pass canonical structural validation and one or more deterministic quality checks identify suspicious text
- **THEN** the existing operation exposes bounded cue findings and continues through the canonical serializer and atomic publication path without changing cue structure

#### Scenario: Structural validation failure prevents quality analysis and publication

- **WHEN** a translated result fails existing cue or ASS structural validation
- **THEN** quality findings do not authorize it, the existing failure behavior is retained, and no partial output is published

#### Scenario: Uncertain OCR remains source uncertainty

- **WHEN** canonical OCR evidence marks a source cue uncertain
- **THEN** translation-quality review reports that uncertainty as separate source evidence and does not represent a translation suggestion as OCR correction or source approval

### Requirement: Deterministic findings support focused human review

Asset Info SHALL present bounded translation-quality findings with the canonical source cue, translated cue, and reason. The operator SHALL be able to keep the translation or edit the reviewed cue through the existing canonical cue and publication contracts. Provider suggestions, when present, SHALL remain advisory and SHALL never be applied automatically. Manual decisions SHALL preserve cue identity, timing, and ASS protected structure, and SHALL pass existing validation and atomic publication before they replace a translated artifact.

#### Scenario: Operator keeps a flagged cue

- **WHEN** the operator reviews a flagged translation cue and keeps its current text
- **THEN** the cue remains unchanged and the decision is represented as a human review outcome

#### Scenario: Operator edits a flagged cue

- **WHEN** the operator manually edits translated cue text
- **THEN** the existing canonical cue identity, timing, ASS protected structure, revision fencing, validation, staging, and atomic publication contracts remain in force

#### Scenario: Independent reviewer is unavailable

- **WHEN** no independent second-opinion reviewer is configured or available
- **THEN** deterministic analysis and human review remain usable without blocking or changing translation publication

### Requirement: Quality provenance and decisions survive operation reload

The system SHALL persist bounded quality findings, source and output revision identity, reviewer model provenance, and explicit human decisions in a translation-linked review record. Review edits SHALL recheck both source and published target identity, preserve cue structure through canonical serialization, and publish only through the existing revision-fenced atomic Publisher. The record SHALL be readable after restart; no staged edit or raw provider response SHALL be persisted.

#### Scenario: Review resumes after restart

- **WHEN** an operator reopens a translation operation with an outstanding review
- **THEN** the persisted findings and decisions are loaded and cues already decided remain marked reviewed

#### Scenario: Stale source or translated target blocks a decision

- **WHEN** the source or translated target revision differs from the identity captured in the review record
- **THEN** Keep/Edit is rejected and no reviewed edit is published

#### Scenario: SRT manual edit preserves every cue boundary

- **WHEN** an operator edits one SRT cue, including text with line breaks
- **THEN** the canonical edit SHALL preserve cue count, cue order, every timestamp, and all unrelated cue text; input containing an SRT timing delimiter SHALL be rejected

#### Scenario: Publisher metadata persistence fails after file promotion

- **WHEN** a reviewed edit is promoted but its SQLite revision or decision commit fails
- **THEN** the canonical Publisher SHALL compensate the file and metadata together, preserve the prior published revision, and permit a safe retry; if compensation cannot be confirmed, the review SHALL be marked recovery-required and further decisions SHALL fail closed

#### Scenario: Editable cue text is loaded from verified output

- **WHEN** an operator starts editing a flagged cue
- **THEN** Asset Info SHALL request the complete cue text by review and cue identity, and the backend SHALL return it only after revalidating the source identity and published target revision; the truncated finding preview SHALL never seed the editor

### Requirement: Optional independent second opinion is bounded and human-only

The system SHALL use a separately configured reviewer model when enabled, SHALL reject using the translator model as reviewer, and SHALL validate reviewer cue identities and response bounds. Reviewer suggestions SHALL never be applied automatically. The request SHALL contain only the deterministic flagged cue pairs needed for review.

#### Scenario: Reviewer proposes a cue edit

- **WHEN** the configured independent reviewer returns a suggestion for a flagged cue
- **THEN** Asset Info presents it as advisory and applies it only after an explicit operator edit decision and canonical publication

#### Scenario: Reviewer model is unavailable

- **WHEN** the configured model cannot return valid reviewer evidence
- **THEN** the report records a bounded unavailable state and deterministic human review remains available
