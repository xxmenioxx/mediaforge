## Purpose

Defines the canonical, auditable quality decision between a completed Queue execution and publication, using frozen expected state and the exact produced Result as evidence.

## ADDED Requirements

### Requirement: Validation runs bind immutable execution and Result evidence

Every Validation run SHALL have a stable run identity, validator contract version, ruleset identity and version, Queue execution identity, frozen plan identity/version, and produced Result identity. The run SHALL record the expected and actual values it evaluates and SHALL remain immutable after completion. A run without verifiable plan or Result identity SHALL NOT be treated as a passing decision.

#### Scenario: Completed output is evaluated against its frozen execution
- **WHEN** a completed Queue execution has a matching frozen plan and produced Result
- **THEN** Validation records one uniquely identified run bound to those exact evidence identities and evaluates expected values from the frozen plan against actual values from that Result

#### Scenario: Evidence identity is missing or changed
- **WHEN** the plan identity is unavailable or the produced artifact no longer matches the Result identity
- **THEN** Validation records an unverified finding and returns `REVIEW_REQUIRED` without treating the evidence as a clean pass

### Requirement: Validation evaluates required rule categories

The ruleset SHALL evaluate artifact integrity, duration and timeline, video properties, audio tracks and actions, subtitles, chapters, attachments and fonts, sidecars, and publication readiness. Each applicable rule result SHALL identify a stable rule key, expected value, actual value, severity, blocking or advisory disposition, finding state, and evidence reference. Rules SHALL record `NOT_APPLICABLE` with a reason when the frozen plan does not require that check; unavailable or non-comparable required evidence SHALL remain explicit and SHALL NOT be represented as passed.

#### Scenario: Required media and artifact checks are reported
- **WHEN** Validation evaluates a produced Result
- **THEN** its report contains rule findings for every required category, including expected-versus-actual stream and artifact actions derived from the frozen plan

#### Scenario: A plan makes a category inapplicable
- **WHEN** the frozen plan explicitly does not require a category such as subtitle conversion or attachment preservation
- **THEN** the corresponding rule is recorded as `NOT_APPLICABLE` with the plan-based reason instead of being omitted or failed

#### Scenario: Required evidence cannot be compared
- **WHEN** a required actual value or its matching evidence reference is unavailable
- **THEN** the rule is recorded as unverified and the overall outcome requires review

### Requirement: Overall outcomes follow rule severity and actionability

Validation SHALL return exactly one overall outcome: `PASS`, `PASS_WITH_WARNINGS`, `REVIEW_REQUIRED`, or `FAIL`. A known violation of a blocking rule SHALL produce `FAIL`; an unresolved, unavailable, or explicitly review-required blocking finding SHALL produce `REVIEW_REQUIRED`; advisory warnings with no blocking findings SHALL produce `PASS_WITH_WARNINGS`; and `PASS` SHALL require all required rules to pass with no warnings. An informational score, when present, SHALL NOT replace these outcomes or the rule findings.

#### Scenario: Every required rule passes cleanly
- **WHEN** all required rules pass and no advisory warning or review finding exists
- **THEN** Validation returns `PASS`

#### Scenario: Advisory warnings remain
- **WHEN** all blocking rules pass and one or more advisory warnings remain
- **THEN** Validation returns `PASS_WITH_WARNINGS` and retains each warning's actionability

#### Scenario: A blocking rule has a known violation
- **WHEN** a blocking rule has a known violation
- **THEN** Validation returns `FAIL`

#### Scenario: A blocking rule cannot be resolved
- **WHEN** a blocking rule cannot be evaluated from trustworthy evidence
- **THEN** Validation returns `REVIEW_REQUIRED`

### Requirement: Review Center defaults to exceptions and History retains all outcomes

The Review Center SHALL default to results with `REVIEW_REQUIRED`, `FAIL`, or an actionable warning, including actionable warnings attached to `PASS_WITH_WARNINGS`. It SHALL provide an explicit way to inspect other outcomes without making clean `PASS` results part of the default exception list. History SHALL retain discoverable completed results and their Validation outcome, run identity, and override state after they leave active review or are published.

#### Scenario: Exceptions are the default review list
- **WHEN** a user opens the Review Center
- **THEN** the default list contains `REVIEW_REQUIRED`, `FAIL`, and results with actionable warnings while omitting clean `PASS` results

#### Scenario: A clean pass leaves active review
- **WHEN** a `PASS` result advances from the Review Center
- **THEN** the result and its Validation identity remain discoverable in History with its final publication state

### Requirement: Overrides are auditable and do not alter evidence or findings

An authorized user override SHALL record the actor, non-empty reason, timestamp, override contract version, targeted Validation run, and the findings or handoff decision it covers. An override SHALL be a separate auditable event and SHALL NOT edit the immutable Validation outcome, rule results, frozen plan, or produced Result evidence. Evidence identity or integrity failures SHALL not be bypassed by an override.

#### Scenario: User overrides an eligible review decision
- **WHEN** an authorized user provides a reason for an eligible Validation finding
- **THEN** the system records a versioned override linked to that run and preserves the original outcome and rule report

#### Scenario: Override reason is missing or evidence identity is invalid
- **WHEN** the override has no reason or the finding is an evidence identity or integrity failure
- **THEN** the override is rejected and no publisher handoff is authorized

### Requirement: Retry and requeue preserve the Validation evidence boundary

A Validation retry SHALL evaluate the same immutable plan and Result identities and SHALL create a new run linked to those identities. If the produced artifact, execution, or frozen plan changes, Queue SHALL create a new execution/Result identity before Validation can decide on it. Validation SHALL NOT re-encode, mutate, repair, or publish media. Only `PASS` may auto-advance under the configured policy by default; `PASS_WITH_WARNINGS` may auto-advance only when policy explicitly permits it and no actionable warning remains. `REVIEW_REQUIRED` and `FAIL` SHALL never auto-advance. Publisher SHALL retain ownership of destination checks, delivery, overwrite, archival, and filesystem mutation.

#### Scenario: Validator is retried against the same output
- **WHEN** a bounded Validation retry is requested and evidence identities still match
- **THEN** a new immutable Validation run is created without creating or changing an encoded artifact

#### Scenario: Output must be produced again
- **WHEN** an output is changed, replaced, or re-encoded
- **THEN** Queue records a new execution and Result identity before a new Validation run can authorize handoff

#### Scenario: Outcome is eligible for automatic advancement
- **WHEN** automatic advancement is enabled for a clean `PASS`, or explicitly enabled for a `PASS_WITH_WARNINGS` without actionable findings
- **THEN** Validation makes the result eligible for Publisher handoff without performing publication

#### Scenario: Review or failure blocks automatic advancement
- **WHEN** the outcome is `REVIEW_REQUIRED` or `FAIL`, or a `PASS_WITH_WARNINGS` includes an actionable finding
- **THEN** automatic advancement is blocked and the result remains discoverable for review

### Requirement: Validation reuses bounded evidence without UI-triggered rescans

Validation SHALL reuse matching ffprobe and Result evidence already collected for the Queue execution. Any fallback evidence collection SHALL be bounded, read-only, and tied to the same Result identity. Opening, rendering, refreshing, or filtering the Review Center or History SHALL NOT trigger media scans, ffprobe work, perceptual sampling, or encoding. Large evidence and logs SHALL be loaded on demand while list views use bounded summaries.

#### Scenario: Matching Result evidence is available
- **WHEN** Validation runs against a Result containing current probe and artifact evidence
- **THEN** it evaluates that evidence without repeating equivalent media probes or perceptual analysis

#### Scenario: Result evidence is incomplete
- **WHEN** a required field is missing from the matching Result
- **THEN** Validation uses only bounded read-only fallback checks for that identity or records the field as unverified; it does not start an unbounded scan or encode

#### Scenario: User opens or refreshes a list view
- **WHEN** the Review Center or History page renders, refreshes, or applies a filter
- **THEN** the page uses stored summary data and performs no media rescan or validation rerun

#### Scenario: User requests detailed evidence
- **WHEN** a user opens a specific result's details
- **THEN** the system loads only that result's stored detailed findings or logs on demand

### Requirement: Validation preserves adjacent pipeline and evidence ownership

Validation SHALL operate only after conversion has produced a Result. It SHALL NOT perform or replace #46 pre-conversion Preflight, encode or repair media, or perform #54 publication and delivery. Matching #69 perceptual/frame evidence MAY be reused as supplemental evidence, but SHALL NOT be the sole authority for a pass. The report SHALL preserve `report.frameFidelity` as a sibling report alongside other specialized reports such as `report.colorPolicy`.

#### Scenario: Preflight remains the pre-conversion owner
- **WHEN** a job has #46 Preflight evidence before conversion
- **THEN** Validation waits for the produced Result and evaluates it against the frozen Queue plan without rerunning or replacing Preflight

#### Scenario: Matching frame evidence supplements the rule results
- **WHEN** matching #69 perceptual or frame-fidelity evidence exists for the frozen plan and Result
- **THEN** Validation may reference it as supplemental evidence, preserves `report.frameFidelity` as a sibling of `report.colorPolicy`, and still evaluates the required rules

#### Scenario: Publisher receives an eligible result
- **WHEN** Validation or an eligible override authorizes handoff
- **THEN** it passes the bound Result reference and decision to Publisher without changing or publishing the media artifact
