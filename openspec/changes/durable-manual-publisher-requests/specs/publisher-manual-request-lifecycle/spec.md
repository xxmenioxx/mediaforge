## Purpose

Allows operators to request manual publication durably and follow its progress without keeping an HTTP request open during NAS delivery work.

## ADDED Requirements

### Requirement: Manual Publish acceptance is durable and explicit

Publisher SHALL recheck that the persisted Queue execution is completed, its canonical Validation and Result identity are eligible for manual delivery, and its frozen destination is available before accepting a manual Publish request. Publisher SHALL return `202 Accepted` with a stable request and publication identity only after the request and its audit evidence are durably stored. An accepted response SHALL describe queued intent and SHALL NOT claim the asset is published.

#### Scenario: Eligible request is accepted
- **WHEN** an operator submits an eligible manual Publish request with a reason
- **THEN** Publisher persists the intent and audit evidence and returns `202 Accepted` with `status=accepted`, a request identity, and a publication identity

#### Scenario: Request cannot pass a pre-acceptance check
- **WHEN** the Queue execution, canonical Validation/Result identity, or frozen destination is missing, stale, or ineligible
- **THEN** Publisher returns an actionable rejection and does not report that the request was accepted

#### Scenario: Request persistence fails
- **WHEN** Publisher cannot persist the manual intent and audit evidence
- **THEN** Publisher does not return `202 Accepted`

#### Scenario: Acceptance is not publication success
- **WHEN** the HTTP client receives an accepted response before delivery completes
- **THEN** the response and UI identify the request as accepted and do not display `Published`

### Requirement: Duplicate manual requests are idempotent

Publisher SHALL bind an active manual request to the existing publication identity for the exact Queue execution and Result. Repeated requests while that intent is accepted or processing SHALL return the same request identity and SHALL NOT create another logical publication or attempt. A new attempt after a safe failure SHALL require the audited retry action. A request for an already published identity SHALL return its stored published state without repeating delivery, archival, or event creation.

#### Scenario: Operator repeats an accepted request
- **WHEN** the same publication is requested again while its manual request is accepted or processing
- **THEN** Publisher returns the existing request and state without appending a second acceptance action or starting another attempt

#### Scenario: Request is repeated after publication
- **WHEN** an operator submits a request for an identity already in `PUBLISHED`
- **THEN** Publisher returns the existing success and performs no destination replacement, archive move, or duplicate event creation

#### Scenario: Retry follows a safe failure
- **WHEN** an operator submits an audited retry for a failed or retryable blocked publication
- **THEN** Publisher records a new accepted request and lets the existing fenced attempt state machine revalidate and claim a new attempt

### Requirement: Accepted manual requests survive restarts and respect Publisher controls

Publisher SHALL process durably accepted manual requests through its existing background publication processing and recovery cadence, including while automatic publishing is disabled. On restart, pending requests and expired publication attempts SHALL resume from persisted publication state. Manual delivery SHALL continue to use the existing Publisher state machine, exact Result identity, Validation gate, required artifact checks, frozen destination/replacement authority, leases, fencing, holds, and audit trail.

#### Scenario: Backend restarts before a request starts
- **WHEN** the backend stops after accepting a manual request but before delivery begins
- **THEN** startup recovery processes the same persisted request without requiring the operator to submit it again

#### Scenario: Backend restarts during a publication attempt
- **WHEN** an accepted request has an expired `PUBLISHING` attempt after restart
- **THEN** Publisher conditionally recovers that attempt using the existing lease and fencing rules and does not start a second active delivery

#### Scenario: Automatic publishing is disabled
- **WHEN** a manual request is accepted while the automatic Publisher setting is disabled
- **THEN** the manual request still runs under manual authorization and automatic publishing remains disabled

#### Scenario: Hold pauses accepted work
- **WHEN** an operator holds a publication after its manual request is accepted and before its attempt is claimed
- **THEN** no delivery work begins until an audited release returns the same publication to `READY`

#### Scenario: Identity or delivery evidence changes after acceptance
- **WHEN** the Result, Validation, required artifact, or frozen destination changes before execution
- **THEN** Publisher rechecks the current evidence, blocks unsafe delivery, and leaves source and unrelated destination files untouched

### Requirement: Publisher status distinguishes acceptance, progress, and completion

The bounded Publisher read model SHALL expose persisted manual request state and the existing publication/attempt progress. The Publisher UI SHALL show accepted and in-progress states separately from final `PUBLISHED`, and SHALL show stored failures with a safe next action. A failed or blocked request SHALL not be presented as a successful publication.

#### Scenario: Operator checks an accepted request
- **WHEN** an operator refreshes Publisher after receiving `202 Accepted`
- **THEN** the stored publication summary shows the request as accepted or in progress until a terminal outcome is persisted

#### Scenario: Delivery completes
- **WHEN** all required artifacts and publication provenance are committed
- **THEN** Publisher reports `PUBLISHED` and the UI displays success only from that persisted state

#### Scenario: Delivery fails
- **WHEN** Publisher records a safe retryable failure or a blocked safety condition
- **THEN** Publisher shows the stored failure reason and an actionable retry or resolution choice
