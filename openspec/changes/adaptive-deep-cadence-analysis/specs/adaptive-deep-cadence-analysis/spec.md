## Purpose

Gather a bounded amount of additional IVTC evidence for an ambiguous source and let single-asset Queue review consume only current, sufficiently distributed evidence.

## ADDED Requirements

### Requirement: Deep cadence analysis is bounded, asynchronous, and cancellable

When an operator requests Deep Cadence Analysis for an asset with ambiguous IVTC evidence, the system MUST reuse the existing validation windows and MAY analyze no more than two additional windows. It MUST evaluate TFF and BFF with the existing cadence comparison and preserve the existing per-window confidence thresholds. Additional windows MUST prefer available movement-relevant sampling evidence and MUST be temporally distributed. The comparison MUST use the canonical `fieldmatch` comb-match mode and record residual interlaced classifications and repeated-field counts before selective deinterlacing or decimation. It MUST record output frame count, measured frame rate, progressive flags before the terminal `setfield=prog` metadata marker, and timestamp-gap anomalies after selective deinterlacing and decimation. These metrics MUST remain visible per window and MUST NOT be used to relax the existing order thresholds. Missing diagnostics or timestamp anomalies MUST keep the deep result review-required. The system MUST NOT infer field order from container metadata such as `tt`.

#### Scenario: Additional windows resolve a distributed order
- **WHEN** initial evidence is ambiguous and the additional windows provide at least three valid, distributed, non-contradictory windows for the same order under existing thresholds
- **THEN** the persisted validation records the selected order, effective cadence analysis can be recomputed, and Queue review may resolve the canonical output FPS and HEVC Level/Tier

#### Scenario: Additional evidence is insufficient or contradictory
- **WHEN** fewer than three distributed windows are valid, or confident windows contradict one another
- **THEN** the order remains unresolved, the effective output FPS remains unknown where required, and Queue review remains `REVIEW_REQUIRED`

#### Scenario: Per-window evidence exposes residual combing and cadence signals
- **WHEN** `fieldmatch` leaves frames classified as interlaced or `idet` observes repeated fields in a candidate window
- **THEN** the pre-deinterlace counts are persisted with that window and remain review evidence; a later `bwdif` stage MUST NOT erase those measurements

#### Scenario: Conflicting windows are distinguished from insufficient windows
- **WHEN** valid windows confidently select different orders
- **THEN** the analysis reports a contradictory/mixed-cadence review reason and retains every window; when no order meets the existing thresholds, the reason reports insufficient evidence instead

#### Scenario: A window cannot be evaluated
- **WHEN** an additional FFmpeg/idet window cannot produce comparable evidence
- **THEN** it is recorded as inconclusive with a reason and does not count as a valid window

#### Scenario: The operator cancels analysis
- **WHEN** the operator cancels a running Deep Cadence Analysis
- **THEN** the analysis process is cancelled and partial evidence is not persisted as a completed result

### Requirement: Deep cadence evidence is versioned and bound to the physical source

Persisted deep evidence MUST include its analysis version, source revision and physical identity. Before it is used for Queue review or persisted, the system MUST verify that the source still identifies the same physical asset and snapshot revision. Evidence for a replaced, modified, missing, or superseded source MUST be treated as stale and MUST NOT resolve cadence.

#### Scenario: Source changes during analysis
- **WHEN** the source path resolves to a different physical file or revision before analysis completes
- **THEN** the operation reports stale/conflict and does not persist the result over the newer snapshot

#### Scenario: A prior analysis version is read
- **WHEN** cached IVTC evidence was produced by an older analysis version
- **THEN** the system refreshes or rejects that evidence before using it to resolve cadence

### Requirement: Single-asset Queue consumes deep evidence through canonical planning

Single-asset Queue and Batch Preflight MUST use the current persisted evidence through the existing cadence, frozen-profile, and HEVC resolvers. If deep evidence resolves IVTC, the effective output FPS and HEVC Level/Tier MUST be recalculated before the candidate is accepted. If evidence remains ambiguous or stale, the system MUST return an actionable `REVIEW_REQUIRED` result without creating a QueueJob, ExecutionPlan, or reservation. The Worker claim-time fail-closed guard MUST remain in force.

#### Scenario: Deep evidence resolves before enqueue
- **WHEN** current deep evidence validates one IVTC order and the operator reviews the resulting effective plan
- **THEN** Queue acceptance freezes the same resolved cadence, FPS, and HEVC decision in the Queue snapshot and ExecutionPlan

#### Scenario: Deep evidence does not resolve before enqueue
- **WHEN** deep analysis is inconclusive, contradictory, cancelled, unavailable, or stale
- **THEN** the candidate remains review-required and acceptance creates no runnable Queue state

#### Scenario: Explicit per-asset decision
- **WHEN** an operator explicitly chooses TFF, BFF, or Preserve Cadence for one asset
- **THEN** the existing canonical resolver produces a reviewable effective plan without changing the global profile

### Requirement: Individual Queue review presents analysis controls and evidence

The individual Queue review MUST explain why review is required, allow the operator to start or cancel Deep Cadence Analysis, and present bounded per-window evidence and its conclusion. It MUST retain asset-scoped TFF/BFF and Preserve Cadence choices and links to existing LAB/Test Encode review paths.

#### Scenario: Analysis finishes without resolving cadence
- **WHEN** the operation completes but cadence remains ambiguous
- **THEN** the UI presents attempted, valid, inconclusive, and contradictory windows and keeps Queue acceptance disabled

#### Scenario: Analysis finishes with a resolved plan
- **WHEN** the operation completes with valid distributed evidence
- **THEN** the UI refreshes the existing one-item Preflight candidate and presents the effective plan for review before acceptance
