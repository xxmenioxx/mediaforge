## Purpose

A bounded, explicit, auditable way to assess and optionally reconcile historical completed Results whose frozen per-asset track expectations are demonstrably contaminated. Normal Validation remains unchanged for all other jobs.

## ADDED Requirements

### Requirement: Historical reconciliation preview is read-only and fail-closed

The system SHALL expose an operator-initiated preview for a completed, unpublished, blocked canonical Result. Preview SHALL read independent execution-time intent and existing result/package evidence; it SHALL NOT create a ValidationRun, alter Queue, Publisher or frozen evidence, perform conversion, or authorize publication. It SHALL return SAFE, BLOCKED or NEEDS_EVIDENCE with actionable reasons, original-versus-proposed expectations and identity bindings.

#### Scenario: Independent per-asset intent proves a crossed snapshot
- **WHEN** original per-asset intent and a matching frozen execution/Result package independently establish the expected track actions and required sidecars
- **THEN** preview shows a bounded diff from the contaminated track snapshot and reports exactly which findings could be re-evaluated

#### Scenario: Only the converted output suggests an expected plan
- **WHEN** the expected actions can be guessed from actual stream counts or current global defaults but not proved from immutable execution-time intent
- **THEN** preview is NEEDS_EVIDENCE and no approval or publisher eligibility can be derived

#### Scenario: A Result or source identity is missing, stale or mismatched
- **WHEN** the canonical generation, output revision, frozen Queue binding, plan fingerprint or required sidecar identities are absent or changed
- **THEN** preview is BLOCKED or NEEDS_EVIDENCE with the exact missing proof and no side effects

### Requirement: Reconciliation commit is explicitly authorized and append-only

A commit SHALL require server-validated operator approval with a reason and exact expected job/execution/plan/Result/expectation revisions. It SHALL revalidate all identities and required package evidence at commit time, then append a versioned, immutable reconciliation record and a newly bound ValidationRun; historical Queue frozen snapshots, ExecutionPlans, ResultArtifacts and prior ValidationRuns SHALL never be rewritten.

#### Scenario: Approved reconciliation is safe
- **WHEN** the original per-asset intended actions and package expectations are independently provable and all revision checks still match
- **THEN** a new ValidationRun references a reconciliation ID and the exact immutable reconciled expected contract, while the old ValidationRun and its findings remain visible

#### Scenario: Candidate is stale before commit
- **WHEN** any expected plan, output generation, output revision, sidecar revision, frozen snapshot fingerprint, or lifecycle state has changed
- **THEN** the commit fails atomically without appending an authoritative correction

#### Scenario: Request is retried
- **WHEN** the same approved idempotency key targets the same exact identities and contract fingerprint
- **THEN** the server returns the existing committed outcome, without a duplicate run or mismatched projection

#### Scenario: Active run projection changes
- **WHEN** a committed reconciled run becomes active
- **THEN** only the QueueJob active ValidationRun pointer and summary projections may update atomically; frozen Queue/TrackProfile snapshots and execution/result history are unchanged

### Requirement: Validation checks the actual Result against independent corrected expectations

For a reconciled run, the canonical evaluator SHALL use the identity-bound expected-track/sidecar contract only for the proven contaminated dimensions; other canonical rules and the original frozen Result/Queue binding SHALL remain mandatory. It SHALL re-evaluate real tracks, manifest IDs, actual sidecars, chapter/attachment actions and non-track findings without inventing evidence or lowering rule severity.

#### Scenario: Sidecar expectations change but output is truly incomplete
- **WHEN** an independently proven expected sidecar is missing from the actual Result manifest or filesystem
- **THEN** the sidecar or manifest rule still fails and Publisher remains blocked

#### Scenario: Video output FPS remains unsupported
- **WHEN** track findings are reconciled but video.properties still has insufficient required FPS evidence
- **THEN** that finding remains UNVERIFIED and the outcome and Publisher eligibility reflect the normal blocking policy

#### Scenario: Track results disagree with corrected execution-time intent
- **WHEN** any observed audio, subtitle, attachment or package action conflicts with the proven corrected contract
- **THEN** the canonical failure is retained and no PASS is manufactured

### Requirement: Publisher honors the same reconciled contract and remains manual

A reconciled ValidationRun SHALL NOT automatically publish or bypass the existing Publisher gate. On a separately approved manual handoff, Publisher SHALL revalidate Result identity, active run, reconciliation fingerprint and actual package readiness using the *same* corrected contract, without rewriting the stored QueueJob or weakening existing filesystem/destination/archive checks. Unsupported or stale package materialization SHALL block publication.

#### Scenario: Reconciled validation passes
- **WHEN** a new reconciled run passes all required canonical rules
- **THEN** Publisher still requires separate explicit confirmation and its normal independent destination/sidecar checks

#### Scenario: Publisher would consume the historical corrupted sidecar plan
- **WHEN** Publisher cannot construct a verified effective package based on the reconciliation contract
- **THEN** it rejects the handoff instead of defaulting to the old contaminated snapshot or skipping the check

### Requirement: Existing paths and historical evidence remain compatible

Normal Queue, Validation retry, and Publisher SHALL behave as before for jobs without a reconciliation record. The reconciliation feature SHALL be opt-in, auditable, removable from the active workflow without deleting audit records, and must not depend on access to production files during repository tests.

#### Scenario: Legacy failed run is inspected
- **WHEN** an operator opens a historical ValidationRun
- **THEN** its original outcome/findings remain unmodified, and any later reconciled run is separately identified with full provenance

#### Scenario: Normal job without reconciliation
- **WHEN** a new or historical uncorrected job follows ordinary Validation and Publisher
- **THEN** the canonical path does not use any reconciliation overlay and original gates remain enforced
