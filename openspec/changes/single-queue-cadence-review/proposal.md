## Why

The individual Queue Create route can persist a job before the canonical cadence and HEVC plan has been resolved, while Batch Preflight resolves and reviews that plan first. For ambiguous automatic IVTC, this delays a known missing-field-order decision until after claim and can leave users with a failed Queue job.

## What Changes

- Route individual Queue requests through the same one-asset preparation and review contract as Batch Preflight before creating runnable Queue work.
- Return a structured review-required result when automatic IVTC lacks an explicit or validated field order, with the missing evidence and TFF/BFF next steps.
- Bind an explicit TFF/BFF choice to the candidate asset, then show the effective frozen plan before acceptance.
- Preserve source identity/revision revalidation, the Worker’s final fail-closed gate, and existing Queue history.

## Capabilities

### New Capabilities

- `queue-single-asset-preflight`: individual Queue review, cadence-order resolution, and frozen-plan acceptance.

### Modified Capabilities

- None.

## Impact

Backend Queue handlers and tests; Queue API types; Assets, Queue, and Scanner single-item Queue flows; `FrameCadenceControls` and Batch Preflight review presentation. The optional cadence-order choice is stored only in the existing preflight candidate JSON; there is no SQLite schema migration, new persisted status, dependency, NAS access, or media processing.
