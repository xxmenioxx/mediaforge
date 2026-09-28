## Why

Queue batch creation currently inserts lightweight jobs before the effective per-asset configuration is frozen. Operators cannot review the exact profile, track decisions, or destination that execution will consume, and direct multi-asset commit paths can bypass that review.

## What Changes

- Add a persisted, revisioned Batch Preflight candidate that retains original selections/configuration and backend-resolved per-asset plans without creating QueueJobs.
- Route every batch and multi-asset Queue creation path through candidate review and explicit acceptance.
- Resolve effective profile, Asset Defaults, track/audio/subtitle, restoration, cadence, frame structure, and destination decisions through the existing Queue preparation and worker preparation helpers.
- Atomically create accepted QueueJobs and their existing ExecutionPlans from the exact reviewed snapshots; retain unaccepted assets and their configuration in the candidate.
- Reuse existing Preview, LAB, and Test Encode behavior with candidate identity and frozen configuration where supported; LAB drafts remain isolated until an explicit apply action re-resolves Preflight.

Out of scope: a new Queue engine, second profile/track resolver, new LAB or Test Encode engine, Worker-side material re-resolution, Autopilot/cohort behavior, #42 BPPPF, #55 Observability, #57 Compliance, and V2 redesign.

## Capabilities

### New Capabilities

- `queue-batch-preflight`: Persisted per-asset plan review and explicit Queue acceptance.

### Modified Capabilities

None. The current OpenSpec inventory has no Queue lifecycle capability specification to amend.

## Impact

- Queue and selected-assets API, QueueJob snapshots, SQLite candidate persistence, worker claim preparation, compatible Preview/Test Encode handoff, and the Assets/Queue frontend.
- Additive SQLite table/columns only. Existing QueueJobs and historical snapshots remain readable and are not rewritten.
