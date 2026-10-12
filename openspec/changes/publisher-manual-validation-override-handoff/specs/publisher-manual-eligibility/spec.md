## Purpose

Publisher operators need a bounded view of which completed Queue Results are eligible for manual delivery, including canonical Validation overrides, while keeping eligibility reads free of lifecycle and filesystem mutations.

## ADDED Requirements

### Requirement: Publisher exposes bounded, read-only manual eligibility

Publisher SHALL return a bounded list of recent completed, unpublished Queue jobs with backend-evaluated manual eligibility. Each item SHALL be `ALLOWED`, `ALLOWED_BY_OVERRIDE`, or `BLOCKED` and include a useful reason when blocked. Eligibility SHALL use the canonical ValidationRun and exact current Result identity/revision, eligible override evidence, required sidecars, and frozen Publisher delivery constraints; legacy QueueJob validation summaries SHALL NOT authorize publication. Listing SHALL NOT create or change PublisherPublication, QueueJob, ValidationRun, or override records, or modify staging, destination, or archive files.

#### Scenario: Review-required Result has valid Publisher override
- **WHEN** a completed job's canonical `REVIEW_REQUIRED` ValidationRun is bound to its current Result and all blocking overrideable findings have valid Publisher-scoped overrides
- **THEN** the bounded list reports `ALLOWED_BY_OVERRIDE`, regardless of its legacy `validationStatus`

#### Scenario: Candidate listing is read-only and bounded
- **WHEN** Publisher eligibility is requested with any supported limit
- **THEN** the response contains no more than the server maximum and does not create publication rows, change Queue state, or write staging, destination, or archive files

#### Scenario: Actual canonical eligibility is evaluated
- **WHEN** a completed job has a legacy status that appears publishable but its canonical Validation or frozen delivery identity is missing or invalid
- **THEN** Publisher reports `BLOCKED` with the backend reason instead of inferring authorization from the legacy status

### Requirement: Blocked manual candidates explain the failed gate

Publisher SHALL report actionable reasons for jobs blocked by mandatory findings, missing or ineligible overrides, stale or mismatched Result/Validation identity, unavailable required sidecars, or frozen Publisher delivery constraints. Mandatory findings SHALL remain non-overrideable, and override evidence SHALL remain scoped to the exact ValidationRun, Result, and requested `publisher` handoff.

#### Scenario: Mandatory findings and missing overrides remain blocked
- **WHEN** a required finding is mandatory or a blocking overrideable finding lacks a valid Publisher-scoped override
- **THEN** the candidate is `BLOCKED` and identifies the unresolved finding or missing override

#### Scenario: Stale Result or mismatched identity remains blocked
- **WHEN** the latest Result, output revision, Queue execution, active plan, or ValidationRun identity no longer matches
- **THEN** the candidate is `BLOCKED` with a stale or mismatched identity reason

#### Scenario: Required sidecar or delivery constraint fails
- **WHEN** a required sidecar is unavailable or a frozen destination cannot pass Publisher delivery preflight
- **THEN** the candidate is `BLOCKED` with the failed sidecar or delivery constraint

### Requirement: Override-backed publication requires a manual reason and confirmation

Publisher SHALL present `ALLOWED_BY_OVERRIDE` as distinct from ordinary validation success and SHALL require an explicit operator confirmation with a non-empty reason before initiating manual publication. Manual authorization SHALL NOT be reused by AutoPublisher; automatic eligibility SHALL continue to use automatic policy and handoff scope.

#### Scenario: Operator confirms override-backed publication
- **WHEN** an operator confirms an `ALLOWED_BY_OVERRIDE` candidate with a reason
- **THEN** the manual publication request carries that reason and re-evaluates current canonical eligibility before creating or advancing delivery

#### Scenario: Automatic publication cannot inherit a manual override
- **WHEN** an override is scoped to `publisher` and AutoPublisher evaluates the job
- **THEN** automatic advancement remains blocked unless the independent automatic policy authorizes it

### Requirement: Publication revalidates and preserves Validation history

At publication time Publisher SHALL revalidate the exact Result identity and output revision, mandatory findings and override evidence, required sidecars, and frozen destination/delivery constraints before file promotion. A successful publication SHALL update Publisher and Queue publication projections without rewriting ValidationRun findings or override events.

#### Scenario: Eligibility changes after listing
- **WHEN** the Result identity/revision, override evidence, required sidecar, or frozen delivery condition changes after listing and before publication
- **THEN** publication is blocked before final materialization or original archival, with the current actionable reason

#### Scenario: Successful manual publication preserves Validation history
- **WHEN** an eligible manual publication completes
- **THEN** the Publisher UI reflects the published state and the original ValidationRun findings and append-only override events remain unchanged
