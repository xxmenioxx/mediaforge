## Why

LAB can already produce bounded source-to-candidate evidence for a Test Encode, but that evidence has no versioned, explainable policy for answering whether the resolved profile is a reasonable pre-batch choice for this asset. Operators need a confidence signal before reviewing a batch, while final output quality must remain owned by #53 Validation.

## What Changes

- Add one deterministic, versioned Profile Qualification evaluator for an existing candidate and its identity-matched #69 evidence over the frozen #70 SamplingPlan.
- Store immutable, auditable per-candidate qualification decisions and expose staleness when source, resolved profile, candidate, sampling, reference, or policy identity changes.
- Show the outcome, rules, provenance, and worst evidence window in the existing LAB/Test Encode workflow.
- Surface the exact qualification reviewed in Queue Batch Preflight and freeze that reference on acceptance without adding a default hard block or Worker recomputation.
- Keep BPPPF as separate planning context; keep final Validation independent and authoritative.

Out of scope: #71 candidate search, #72 historical calibration, cohorts, new perceptual or artifact metrics, new scene selection, duplicate Test Encodes, LAB V2, Validation bypasses, and automatic profile tuning.

## Capabilities

### New Capabilities

- `quality/profile-qualification-policy`: identity-bound, rule-by-rule confidence evaluation for one asset, resolved profile fingerprint, existing Test Encode candidate, and bounded quality evidence.

### Modified Capabilities

None. Queue Preflight and Validation integrations preserve their existing contracts; the qualification reference is additive review evidence.

## Impact

- Backend: Test Encode snapshots, quality policy/evaluator, immutable SQLite persistence, profile qualification API, Queue Preflight projection and acceptance snapshots.
- Frontend: existing Profile LAB Test Encode details and Preflight review presentation.
- OpenSpec: new quality capability and implementation design/tasks.
- Migrations: one narrow append-only qualification table, indexes, and profile/source identity columns for newly created Test Encodes. Historical rows are not assigned synthetic fingerprints or outcomes.
