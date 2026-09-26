## Why

Validation is the post-conversion safety gate between a completed Queue job and publication, but it currently produces a mutable per-job status/report after a manual request. A versioned, evidence-bound decision is needed so publication, review, retries, and automatic advancement all use the same auditable result.

## What Changes

- Define Validation as a rule-oriented quality gate over the frozen Queue execution and the produced Result evidence, with explicit outcomes and rule-level expected/actual values.
- Bind each immutable Validation run to stable plan and Result identities; retain override and revalidation history without creating another source of media evidence.
- Make the Review Center focus on `REVIEW_REQUIRED`, `FAIL`, and actionable warnings; keep clean passing jobs discoverable in History.
- Define controlled auto-advancement and requeue boundaries, plus bounded evidence reuse for Validation.

Out of scope: #46 pre-conversion Preflight, #54 publication and delivery behavior, changes to #69 perceptual/frame evidence production, changes to #165 `report.frameFidelity`, encoder policy, and any re-encoding from Validation.

## Capabilities

### New Capabilities

- `validation-quality-gate`: Canonical, auditable post-conversion rule evaluation and handoff eligibility for Queue results.

### Modified Capabilities

None. The current OpenSpec inventory has no Validation, Queue lifecycle, History, or Publisher capability specification to amend.

## Impact

- Backend Validation, Queue execution snapshots, existing job Result artifacts, and the current publisher eligibility boundary.
- Validation API and Review Center behavior; Queue job summary fields and History discovery.
- SQLite compatibility and migration behavior for versioned Validation records, overrides, and existing report/status projections.
