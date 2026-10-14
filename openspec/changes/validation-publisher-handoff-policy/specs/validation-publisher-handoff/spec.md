## Purpose

Define when a completed Queue execution can leave Validation review, when Auto Publisher is authorized to deliver it, and how persisted Queue history distinguishes conversion from publication.

## ADDED Requirements

### Requirement: Validation handoff uses blocking disposition independently from actionability

Manual and automatic Validation SHALL use the same canonical finding dispositions to decide whether an execution is eligible for `ready_to_publish`. `Blocking` disposition controls whether a finding prevents handoff; `Actionable` indicates follow-up work and SHALL NOT independently block an advisory finding. The recorded outcome and findings SHALL remain unchanged and visible, including `PASS_WITH_WARNINGS`.

#### Scenario: PASS advances through either Validation route
- **WHEN** a canonical ValidationRun has outcome `PASS` and no unresolved blocking finding
- **THEN** manual Validation and enabled automatic Validation advance the job to `ready_to_publish`, subject to automatic outcome policy on the automatic route

#### Scenario: Actionable advisory warning remains eligible for handoff
- **WHEN** a canonical ValidationRun has outcome `PASS_WITH_WARNINGS` and every warning finding is advisory, including a finding with `Actionable=true`
- **THEN** manual Validation advances the job to `ready_to_publish`, and automatic Validation does so only when `PASS_WITH_WARNINGS` is explicitly enabled by automatic outcome policy
- **AND** the ValidationRun and user-visible result remain `PASS_WITH_WARNINGS` with the advisory finding intact

#### Scenario: Nonactionable advisory warning remains eligible for handoff
- **WHEN** a canonical ValidationRun has outcome `PASS_WITH_WARNINGS` and every warning finding is advisory and nonactionable
- **THEN** the same manual and automatic handoff rules apply as for an actionable advisory warning

#### Scenario: Blocking warning or review outcome stays in review
- **WHEN** a ValidationRun contains an unresolved blocking `WARNING`, has outcome `REVIEW_REQUIRED`, or has an outcome inconsistent with its canonical findings
- **THEN** the job SHALL remain in `validation_review` and SHALL NOT be eligible for Publisher delivery
- **EXCEPT** an operator may use the existing exact, Publisher-scoped manual override for every overrideable blocking finding in a `REVIEW_REQUIRED` run, with the required reason; that path SHALL NOT move the Validation outcome or job into automatic handoff

#### Scenario: Failure or critical missing evidence stays blocked
- **WHEN** Validation returns `FAIL`, or critical evidence is `UNVERIFIED` or otherwise unresolved with blocking disposition
- **THEN** the job SHALL remain blocked from `ready_to_publish` and Publisher delivery regardless of actionability, score, legacy status, or override

#### Scenario: Automatic Validation is disabled
- **WHEN** automatic Validation is disabled for a worker completion
- **THEN** the automatic route SHALL NOT create a ValidationRun or hand the job to Auto Publisher, and SHALL persist the existing review/hold state
- **AND** a later explicit manual Validation request SHALL use the same canonical handoff contract

### Requirement: Automatic warning publication requires explicit policy authorization

Auto Publisher SHALL deliver a `PASS_WITH_WARNINGS` execution only when the canonical handoff is eligible, `validation.autoAdvanceOutcomes` explicitly includes `PASS_WITH_WARNINGS`, Auto Publisher is enabled, and all independent Publisher authorization checks pass. Missing or empty outcome policy SHALL continue to allow `PASS` only. Deploying this behavior SHALL NOT rewrite historical stages or automatically release historical warning jobs.

#### Scenario: Explicit warning policy enables automatic publication
- **WHEN** enabled automatic Validation produces an advisory-only `PASS_WITH_WARNINGS` run, the configured automatic outcome list includes `PASS_WITH_WARNINGS`, and Auto Publisher is enabled
- **THEN** the job may proceed from `ready_to_publish` to `publishing` after canonical identity and Publisher preflight checks succeed

#### Scenario: Warning policy is absent or does not include warnings
- **WHEN** automatic Validation produces `PASS_WITH_WARNINGS` and the automatic outcome list is missing, empty, or excludes that outcome
- **THEN** the job SHALL remain in `validation_review` on the automatic validation route and Auto Publisher SHALL reject the outcome

#### Scenario: Automatic publishing is disabled
- **WHEN** Validation handoff is eligible but Auto Publisher is disabled
- **THEN** the job SHALL remain durably at `ready_to_publish` without starting a publishing attempt

#### Scenario: Policy changes are applied at authorization time
- **WHEN** automatic outcome policy changes before a new automatic handoff or Publisher authorization
- **THEN** the current policy SHALL determine that authorization, while existing immutable ValidationRun data and prior stage history remain unchanged

#### Scenario: Deployment preserves historical warning jobs
- **WHEN** the application starts with historical jobs in `validation_review` or with no explicit warning authorization
- **THEN** startup SHALL NOT move those jobs to `ready_to_publish` or publish them as a side effect of deployment
- **AND** historical jobs already authorized by an explicit warning policy remain subject to the normal current Publisher checks

### Requirement: Publisher authorization preserves canonical evidence and delivery controls

Manual and automatic Publisher SHALL require a canonical ValidationRun bound to the exact current Result artifact, frozen execution, and output content identity. Publisher SHALL continue to enforce required sidecars, destination readiness, replacement authority, source provenance, delivery manifest integrity, and fencing/retry rules. Publisher-only manual overrides SHALL NOT authorize Auto Publisher, and rule overrides SHALL remain bound to their exact run, identity, rule scope, actor, and requested handoff.

#### Scenario: Missing or stale Result identity blocks publication
- **WHEN** the canonical ValidationRun is missing, unbound, refers to a different Result generation, or the output content revision has changed
- **THEN** neither manual nor automatic Publisher may authorize delivery

#### Scenario: Required sidecar or destination preflight fails
- **WHEN** a required sidecar is missing, destination validation fails, or required replacement authority is absent
- **THEN** Publisher SHALL record or return the existing blocked failure and SHALL NOT mark the job published

#### Scenario: Manual override remains scoped to manual publication
- **WHEN** an exact eligible manual override covers every overrideable blocking finding in a `REVIEW_REQUIRED` run and the operator supplies the required reason
- **THEN** manual Publisher may proceed subject to all remaining controls
- **AND** that override SHALL NOT authorize Auto Publisher

#### Scenario: Immutable evidence survives retry and policy changes
- **WHEN** Validation is retried or automatic policy changes after a ValidationRun is created
- **THEN** prior ValidationRuns and Results remain immutable, and any new ValidationRun or override retains its own exact identity and provenance

### Requirement: Queue stage history records durable handoff and publication work

The job lifecycle SHALL record actual stage transitions in order. A failed persistence of `validating`, `ready_to_publish`, or `publishing` SHALL be returned to the caller and SHALL prevent dependent Validation or Publisher work from proceeding as though the transition succeeded. A short-lived `ready_to_publish` stage SHALL still be persisted before Auto Publisher claims `publishing`.

#### Scenario: Successful automatic handoff records intermediate Ready stage
- **WHEN** automatic Validation succeeds and Auto Publisher starts in the same worker pipeline
- **THEN** persisted history SHALL show `validating → ready_to_publish → publishing` in that order before later publication stages

#### Scenario: Ready stage persistence fails
- **WHEN** a `ready_to_publish` stage write fails after Validation
- **THEN** the error SHALL be propagated and Publisher SHALL NOT be invoked

#### Scenario: Publishing stage persistence fails
- **WHEN** Publisher cannot persist the `publishing` stage while claiming a publication attempt
- **THEN** the publication claim and stage update SHALL roll back together, and no delivery attempt SHALL begin

#### Scenario: Status-only completion does not bypass Validation
- **WHEN** a worker status callback or Queue status update marks conversion `completed` without running canonical Validation
- **THEN** the job SHALL remain in `validation_review` and SHALL NOT be moved directly to `ready_to_publish`
- **AND** repeated completion updates SHALL preserve the current handoff stage, including terminal publication state

#### Scenario: Retry and restart recover from persisted state
- **WHEN** the process restarts while a job is `ready_to_publish` or while a durable Publisher attempt is `publishing`
- **THEN** recovery SHALL use existing persisted job/publication identity, lease, and attempt evidence; it SHALL NOT invent missing stage transitions or create a second concurrent active attempt

#### Scenario: Concurrent handoffs do not lose stage history
- **WHEN** manual and automatic Validation or multiple Publisher ticks race for the same Queue execution
- **THEN** persisted stage history and fenced publication state SHALL represent the winning durable transition without authorizing two delivery attempts

### Requirement: Queue checklist separates conversion and publication completion

The Queue checklist SHALL derive lifecycle progress from persisted stage history and publication evidence. A completed conversion SHALL NOT be presented as a completed publication unless publication completion is recorded.

#### Scenario: Converted output awaits publication
- **WHEN** a job has completed conversion and is in `validation_review` or `ready_to_publish` without published evidence
- **THEN** the checklist SHALL show conversion complete and publication incomplete or pending

#### Scenario: Publication completes
- **WHEN** a job has a persisted successful publication stage or canonical publication record
- **THEN** the checklist SHALL show publication complete using that persisted evidence

#### Scenario: Historical jobs have incomplete stage history
- **WHEN** a historical job lacks one or more new lifecycle events
- **THEN** the checklist SHALL show only milestones supported by persisted history and current canonical evidence, without inferring publication from conversion status
