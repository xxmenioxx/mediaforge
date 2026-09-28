## 1. Candidate model and canonical resolution

- [x] 1.1 Add revisioned QueueBatchPreflight persistence and additive migration coverage; retain original config and resolved candidate state without QueueJobs.
- [x] 1.2 Factor PREPARING snapshot calculation from QueueJob persistence and resolve candidate profiles, Asset Defaults, track/audio plans, frame structure, cadence, restoration, runtime encoder, and destination with existing resolvers.
- [x] 1.3 Add bounded create/read/edit/resolve candidate APIs with per-asset blockers and warnings.

## 2. Acceptance and compatibility

- [x] 2.1 Add atomic explicit-subset acceptance from the exact current candidate revision; preserve unresolved assets and reject stale candidates.
- [x] 2.2 Prevent legacy batch and multi-asset commit paths from inserting QueueJobs without Preflight; keep single-job and historical snapshots compatible.
- [x] 2.3 Make accepted Queue execution consume frozen profile, runtime encoder, track, and destination decisions without worker material re-resolution.

## 3. Frontend review and validation actions

- [x] 3.1 Add candidate review/edit/accept UI that preserves selections/configuration and displays resolved and preserved behavior.
- [x] 3.2 Reuse existing Preview, LAB, and Test Encode flows with exact candidate context; keep LAB edits isolated unless explicitly applied and re-resolved.
- [x] 3.3 Add frontend lifecycle tests for resolve, edit/invalidate, subset approval/acceptance, blockers, and candidate-bound validation actions.

## 4. Verification and compatibility

- [x] 4.1 Add focused backend tests for all batch creation paths, canonical plan fidelity, snapshots, stale candidates, blockers, partial acceptance, and historical compatibility.
- [x] 4.2 Run focused backend/frontend tests, changed-file lint, frontend build, and `openspec validate queue-batch-preflight --strict`.
- [x] 4.3 Run the canonical repository verifier and address only its reported failures.
