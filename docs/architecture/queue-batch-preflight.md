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

`GET /queue/preflights/:id` restores a candidate for review. `POST .../edit`
invalidates the prior resolution and advances the revision. `PUT` resolves the
retained or edited inputs again. The Assets page remembers the candidate ID so
an unresolved candidate can be reopened and edited after navigation or reload.

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

AutoMigrate adds only the `queue_batch_preflights` table and the nullable
`resolved_destination_snapshot` QueueJob column. Existing Queue snapshots are
not rewritten. Candidate JSON stores bounded batch state and references the
existing Queue profile, audio, track, ExecutionPlan, and runtime snapshot
contracts; it does not introduce another resolver or worker engine.

Preflight reuses persisted ScanResult and analysis evidence. It does not walk
the Library or run probes solely to render review. Candidate creation and each
re-resolution capture one runtime audit snapshot for the bounded asset set.
