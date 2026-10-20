## Purpose

Defines the operator controlled cleanup of one dismissed Validation job's staging workspace, from an exact read-only preview through durable cleanup outcomes and safe History retry.

## ADDED Requirements

### Requirement: Validation Dismiss previews optional staging cleanup

The Validation Dismiss flow SHALL offer “Also clean staged output” selected by default and show a read-only preview bound to the exact QueueJob before dismissal. The preview SHALL identify the QueueJob ID, execution identity, ownership, reason, exact workspace files and sidecars with byte totals, and every blocking condition. A displayed execution number SHALL NOT identify `work/job-<id>`.

#### Scenario: Eligible job has a safely owned staging workspace
- **WHEN** an operator previews an eligible completed, unpublished job with current canonical Validation outcome `FAIL` or `REVIEW_REQUIRED`
- **THEN** the preview SHALL show exact files and total bytes for that job's verified direct `work/job-<QueueJob.ID>` workspace, its QueueJob and execution identities, ownership, and cleanup reason
- **AND** the operator SHALL explicitly confirm Dismiss before any cleanup can begin

#### Scenario: Workspace is absent or empty
- **WHEN** the eligible job has no workspace or an empty verified workspace
- **THEN** preview and cleanup SHALL report an idempotent no-op with zero recovered bytes
- **AND** History SHALL show that no staging bytes were removed

#### Scenario: Workspace ownership or storage is unsafe
- **WHEN** the workspace is missing ownership proof, does not contain the persisted OutputPath when one is present, contains symlinks or special files, or storage roles overlap unsafely
- **THEN** preview SHALL identify the blocking reason, SHALL NOT authorize cleanup, and SHALL still allow the operator to Dismiss without cleanup

#### Scenario: User disables staging cleanup
- **WHEN** the operator unchecks “Also clean staged output” and confirms Dismiss
- **THEN** the existing Dismiss lifecycle SHALL commit and the workspace SHALL be retained without invoking cleanup
- **AND** History SHALL show staging retained

### Requirement: Cleanup is a separate, job-scoped action bound to the preview

Staging cleanup SHALL occur only after the existing transactional Dismiss has committed and the operator explicitly confirms cleanup. The cleanup authorization SHALL be one-time, expiring, and bound to one QueueJob ID, execution/ExecutionPlan identity, Result and ValidationRun identity, storage-role roots, exact workspace path, tree identity, file manifest, and byte total. Cleanup SHALL use the existing Housekeeping candidate and deletion engine with an explicit job-scoped opt-in; it SHALL NOT invoke or mutate the global Housekeeping preview. General and automatic Housekeeping SHALL continue to protect dismissed completed/unpublished workspaces.

#### Scenario: Confirmed cleanup removes only the previewed workspace
- **WHEN** the operator confirms cleanup after Dismiss and all bound identities and safety checks still match
- **THEN** cleanup SHALL remove only that job's canonical work workspace using the Housekeeping deletion path
- **AND** RAW originals, Library publications, originals archive, reports, logs, Result artifacts, ExecutionPlans, ValidationRuns, findings, and diagnostic evidence SHALL remain intact

#### Scenario: Displayed execution number differs from QueueJob ID
- **WHEN** the execution number and QueueJob ID differ
- **THEN** preview and cleanup SHALL remain bound to the QueueJob ID and its execution identity, and SHALL target only `work/job-<QueueJob.ID>`

#### Scenario: Preview is stale or state changes
- **WHEN** the preview expires or lifecycle, Validation, Result, ExecutionPlan, storage roles, ownership, paths, file identities, files, bytes, or active references change before deletion
- **THEN** cleanup SHALL be blocked without deleting files and SHALL report the blocking reason

#### Scenario: Job has an active reference or a check fails
- **WHEN** a Worker/process, reservation, Queue operation, Publisher lease/attempt, maintenance operation, preview, Test Encode, sidecar/artifact, or another job may reference the workspace, or any required reference check cannot complete
- **THEN** cleanup SHALL fail closed and retain the workspace

#### Scenario: Filesystem mutation races with cleanup
- **WHEN** a path component or entry is replaced, linked, or changed while cleanup is running
- **THEN** filesystem access SHALL remain rooted to the configured work role, refuse symlink traversal and special files, recheck identities at deletion, and SHALL NOT remove any path outside the exact workspace
- **AND** any detected race SHALL stop cleanup and be recorded as blocked or failed

### Requirement: Dismiss and cleanup have independent durable outcomes

Once Dismiss commits, a later cleanup failure or blocked result SHALL NOT roll back Dismiss or be reported as a Dismiss failure. Every confirmed cleanup attempt SHALL append durable evidence containing QueueJob and execution identities, preview/tree identity, actor/action, outcome and timestamp, paths, actual recovered bytes, and any blocking or error reason. Partial removal SHALL be reported accurately and SHALL NOT be counted as fully cleaned; retries SHALL re-preview the remaining tree and SHALL be idempotent without authorizing deletion of newly created or reassigned content.

#### Scenario: Cleanup succeeds after Dismiss
- **WHEN** all files in the authorized workspace are safely removed after Dismiss
- **THEN** Dismiss SHALL remain committed, evidence SHALL record actual recovered bytes and removed paths, and History SHALL show “Staging cleaned”

#### Scenario: Cleanup is blocked or fails after Dismiss
- **WHEN** post-Dismiss cleanup is unsafe or a deletion fails
- **THEN** Dismiss SHALL remain committed, evidence SHALL record the blocked or failed outcome and reason, and History SHALL show “Cleanup failed/blocked” while retaining the Validation and Result provenance
- **AND** History SHALL offer a new bounded preview and retry when the job remains eligible

#### Scenario: Cleanup partially removes workspace
- **WHEN** some previewed files are removed before a later deletion fails
- **THEN** evidence SHALL report only actual removed paths and bytes and the partial failure reason
- **AND** retry SHALL preview the remaining workspace and SHALL NOT claim previously absent bytes were recovered again

#### Scenario: Repeated or concurrent confirmation
- **WHEN** the same preview is confirmed more than once or concurrent Dismiss/cleanup requests race
- **THEN** at most one cleanup SHALL consume the preview, competing mutations SHALL be serialized or rejected, and repeats SHALL return the durable outcome without deleting a newly created workspace

### Requirement: History preserves provenance and reports staging availability

History SHALL retain dismissed QueueJob, Validation, and Result provenance and show staging state distinctly as retained, cleaned, cleanup failed/blocked, or no workspace. Physical output unavailability after cleanup SHALL be presented as staging availability and SHALL NOT alter historical Validation outcomes or immutable evidence. Safe review and retry actions SHALL be available for retained or failed/blocked staging when a fresh job-scoped preview permits them.

#### Scenario: Dismissed job remains inspectable after cleanup
- **WHEN** staging cleanup succeeds for a dismissed job
- **THEN** History SHALL retain the Dismissed marker, Validation outcome and Result identity
- **AND** unavailable historical output files SHALL not change the recorded Validation quality result

#### Scenario: General Housekeeping runs later
- **WHEN** automatic or operator initiated general Housekeeping runs after a job is dismissed
- **THEN** it SHALL NOT delete that completed unpublished workspace solely because the job was dismissed
- **AND** explicit Validation staging cleanup evidence SHALL remain available regardless of later Housekeeping reports
