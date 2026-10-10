## Purpose

Queue Batch Preflight lets operators manage persisted review candidates independently from dialog visibility while keeping accepted Queue work immutable.

## ADDED Requirements

### Requirement: Close preserves a candidate and Resume is explicit

Closing Batch Preflight MUST dismiss only the dialog and MUST preserve the active server candidate. Navigating, returning to Assets, or reloading MUST NOT open a saved candidate automatically. A deliberate Resume action MUST restore the latest active candidate revision.

#### Scenario: Close and navigate
- **WHEN** an operator closes an active candidate and navigates away from and back to Assets
- **THEN** the dialog remains closed and the persisted candidate remains available for explicit Resume

#### Scenario: Close and reload
- **WHEN** an operator closes an active candidate and reloads Assets
- **THEN** the dialog remains closed and no QueueJob is created

#### Scenario: Resume a saved candidate
- **WHEN** an operator explicitly resumes a saved active candidate
- **THEN** the latest persisted revision is shown in Batch Preflight

#### Scenario: Resume a missing or terminal candidate
- **WHEN** an operator resumes a candidate that no longer exists or is terminal
- **THEN** the stale browser reference is cleared, the dialog stays closed, and the operator receives clear feedback

### Requirement: Remove an unqueued asset from a candidate

The system MUST allow an operator to persistently remove any unqueued candidate asset, including ready, review-required, and blocked assets. Removal MUST be fenced by the current candidate revision, MUST advance the revision, and MUST NOT change accepted Queue work or the remaining assets' reviewed plans.

#### Scenario: Remove a blocked asset
- **WHEN** an operator removes a blocked unqueued asset from the current candidate revision
- **THEN** that asset can no longer be accepted or resumed as part of that candidate
- **AND** the removal persists after application restart

#### Scenario: Remove an asset while other assets remain
- **WHEN** an operator removes one unqueued asset from a candidate that has other pending or accepted assets
- **THEN** the candidate advances to a new revision and the remaining plans and accepted history are preserved

#### Scenario: Remove the last pending asset
- **WHEN** an operator removes the final unqueued asset
- **THEN** the candidate enters a terminal state without submitting an empty batch for resolution
- **AND** any previously accepted Queue work remains unchanged

#### Scenario: Remove with a stale revision
- **WHEN** an operator removes an asset using an obsolete candidate revision
- **THEN** the operation returns a conflict and changes neither the candidate nor accepted Queue work

### Requirement: Discard a candidate without changing accepted work

Discard candidate MUST permanently abandon all remaining unqueued assets and terminalize the candidate. It MUST preserve partial-acceptance history and MUST NOT create, delete, cancel, or mutate QueueJobs, ExecutionPlans, SchedulerReservations, source files, or publication artifacts.

#### Scenario: Discard an unaccepted candidate
- **WHEN** an operator discards an active candidate
- **THEN** no unqueued asset from that candidate can be resumed or accepted
- **AND** no QueueJob is created by discarding

#### Scenario: Discard after partial acceptance
- **WHEN** a candidate has accepted QueueJobs and the operator discards its remaining assets
- **THEN** accepted jobs, frozen plans, reservations, and acceptance history remain unchanged
- **AND** only the remaining candidate work is abandoned

#### Scenario: Discard with a stale revision
- **WHEN** an operator discards a candidate using an obsolete revision
- **THEN** the operation returns a conflict and leaves candidate and accepted work unchanged

### Requirement: Candidate lifecycle is durable and terminal states are not reusable

The system MUST persist clear active, completed, and discarded candidate states. Terminal candidates MUST reject edit, resolve, accept, remove, and discard mutations. A new explicit Preflight MUST create a distinct active candidate after an earlier candidate is discarded.

#### Scenario: Retry a terminal candidate mutation
- **WHEN** a client submits a mutation for a completed or discarded candidate
- **THEN** the system rejects the mutation without changing persisted state

#### Scenario: Create a new candidate after discard
- **WHEN** an operator starts a new Preflight after discarding a candidate
- **THEN** the new candidate has an independent identity and can be reviewed and accepted normally

#### Scenario: Recover state after restart
- **WHEN** the application restarts after a candidate removal or discard
- **THEN** the stored revision and terminal or active state remain authoritative
