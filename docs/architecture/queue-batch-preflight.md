# Queue Batch Preflight

Batch Preflight is the review boundary for multi-asset Queue creation. A
candidate is persisted as `QueueBatchPreflight`; it is not a `QueueJob`, cannot
be claimed, and does not reserve assets before acceptance.

## Resolution and review

`QueueHandler.CreateBatchPreflight` and `PreflightSelectedAssets` resolve each
bounded candidate through `prepareBatchQueueJob`, existing profile assignment
and Asset Defaults resolution, supplemental audio/track snapshots, and
`freezeQueuePreparedProfileSnapshot`. The latter uses the same PREPARING
resolution as Queue jobs and can produce a snapshot before a job is persisted.
The candidate retains the original `QueueJobInput`, stable asset/source
identity, resolved Queue snapshots, execution constraints, selected runtime
encoder, destination path, warnings, and blockers under a revision number.

`GET /queue/preflights/:id` returns a saved candidate for review. `POST .../edit`
invalidates the prior resolution and advances the revision. `PUT` resolves the
retained or edited inputs again. Assets keeps a saved candidate ID separately
from dialog visibility. Closing preserves the candidate but never opens it on
navigation or reload; an explicit Resume action fetches the latest active
revision.

## Candidate lifecycle

Each candidate has a persisted `active`, `completed`, or `discarded` status.
`POST /queue/preflights/:id/remove` removes one unqueued candidate asset by its
stable candidate asset ID. It preserves the other reviewed plans and advances
the revision; removing the last pending asset terminalizes the candidate.
`POST /queue/preflights/:id/discard` abandons every unqueued asset and retains
accepted entries and job IDs as historical candidate state. Both operations
use the expected revision transactionally and do not create, change, cancel,
or release QueueJobs, ExecutionPlans, or SchedulerReservations. Candidate rows
are retained, and terminal candidates cannot be edited, accepted, removed, or
discarded again.

The browser's `mvforge.queueBatchPreflightCandidateId` value is only a saved
reference. It does not trigger a mount-time fetch or modal open. A stale GET or
event cannot replace a newer candidate; missing and terminal candidates clear
the matching reference after an explicit recovery attempt. There is no
background polling.

## Acceptance and execution

`POST /queue/preflights/:id/accept` checks the exact candidate revision and
source identities in one transaction. It inserts only explicitly selected
ready assets, freezes the stored Queue snapshots, creates the existing
ExecutionPlans with the stored constraints and runtime snapshot, and marks
those candidate entries queued. Other entries remain in the same candidate
with their inputs and review reasons. Legacy batch and multi-asset commit
routes reject direct creation; single-job Queue creation is unchanged.

Accepted jobs carry the existing profile/audio/track snapshots plus a frozen
destination snapshot. The worker recognizes the prepared snapshot and does not
reapply mutable asset overrides, restoration, cadence, frame structure, or
encoder selection. Output planning uses the stored destination path. Historical
jobs without complete snapshots retain the existing claim-time preparation.

## Validation actions

- Compatible Preview receives candidate ID, revision, and asset ID. The backend
  validates that context and replaces submitted profile data with the frozen
  candidate profile and selected encoder before creating the normal Preview
  request.
- Test Encode resolves the same candidate fields by their stable IDs and
  revision, then uses the existing Test Encode resolver and execution path. A
  stale candidate or changed source identity is rejected.
- LAB opens the existing `/profile-lab` page in a new tab with the candidate
  IDs. LAB loads the saved profile, audio, and track snapshots into isolated
  drafts. Draft edits do not update the candidate or asset configuration.

## Persistence and compatibility

AutoMigrate adds the `queue_batch_preflights` table and lifecycle status with an
`active` default for existing rows, plus the nullable
`resolved_destination_snapshot` QueueJob column. Existing Queue snapshots are
not rewritten. Candidate JSON stores bounded batch state and references the
existing Queue profile, audio, track, ExecutionPlan, and runtime snapshot
contracts; it does not introduce another resolver or worker engine.

Preflight reuses persisted ScanResult and analysis evidence. It does not walk
the Library or run probes solely to render review. Candidate creation and each
re-resolution capture one runtime audit snapshot for the bounded asset set.

Quality review may include the versioned `sourceBitrateEvidence` component
from that same current ScanResult. After the selected runtime encoder is
resolved, Preflight calculates a BPPPF rail from the candidate's effective
output geometry, FPS, codec, encoder class, dynamic range, and optimization
goal. It adds the source provenance and rail assessment to the existing
`sourceAnalysisEvidence` manifest and shows a non-blocking warning when the
rail is crossed. The rail never edits the reviewed profile.

Acceptance copies those exact values into the Queue snapshot and
`ExecutionPlan` evaluation/decision sources. ReviewPlanner may project the
stored assessment, but it does not sample the source or calculate another
rail. Worker execution does not recompute this evidence or apply the warning as
a bitrate floor. The #42 cache adds a versioned `ScanResult.sourceBitrateEvidence`
field and refreshes only that analysis component; it does not add a second
analysis store.
