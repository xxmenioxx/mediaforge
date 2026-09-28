## Context

Batch configuration is currently built in `frontend/src/pages/AssetsPage.tsx` by `AssetGroupRow/queueGroup` and `AssetCollectionRows/queuePaths`. Unprocessed selection uses `/api/queue/selected-assets` preview/commit. The backend batch boundary is `QueueHandler.CreateBatch` -> `prepareBatchQueueJob` -> `persistPreparedQueueBatch`; single-job intent is `QueueHandler.Create`.

`prepareBatchQueueJob` delegates effective assignments to `effectiveAssetConfiguration` / `resolveProfileAssignmentsFromEffective`. `captureProfile` applies Asset Defaults and asset overrides, freezes a Queue profile snapshot, and calls `captureInterlaceSnapshot`, which reuses persisted ScanResult cadence/interlace/restoration evidence. `captureSupplementalProfilesWithSettings` freezes audio and track profiles; `resolveTrackPlan` and `freezeEffectiveTrackPlan` preserve exact stream, subtitle, OCR, and sidecar choices. `freezeQueuePreparedProfileSnapshot` is the canonical PREPARING resolver for final geometry, cadence/FPS, GOP, and HEVC constraints, but currently requires a persisted QueueJob.

`scheduler.CreatePendingExecutionPlan` builds the existing scheduler plan from a frozen profile snapshot. `ReviewPlanner.EvaluateReviewPlan` binds runtime evidence, selects a usable encoder, and evaluates resource/review state. Worker `prepareClaimedQueueJob` freezes incomplete legacy/intent jobs at claim; a complete Queue snapshot bypasses that capture. Execution renders the final path through `plannedOutputPathForJob`, then `saveQueueJobWithDeliverySnapshot` updates the same ExecutionPlan destination projection.

Compatible Preview uses the existing `/assets/preview/compatible/requests` path. LAB is the existing `/profile-lab` route. Test Encode uses `/test-encodes` and `resolveTestEncodeRequest`. These remain the validation engines; candidate actions pass a stable candidate and asset identity into them rather than building another command path.

## Goals / Non-Goals

**Goals:**

- Keep one persisted candidate revision containing both original per-asset inputs and the exact resolved Queue snapshot consumed by acceptance.
- Make candidate edits invalidate the previous resolved revision, then call the canonical resolver again.
- Let operators accept only ready assets; keep unresolved assets and their configuration in the candidate.
- Freeze all material decisions before Queue visibility, so settings, profile, Asset Defaults, and resolver changes cannot alter accepted snapshots.
- Use bounded batch reads and persisted analysis. Do not scan the Library or launch ffprobe just to render candidate review.

**Non-Goals:**

- Replacing `ReviewPlanner`, changing existing Queue lifecycle state meanings, or rewriting historical Queue snapshots.
- Creating a QueueJob to store a draft candidate.
- Introducing Autopilot or cohort semantics that are absent from V1.

## Decisions

### Candidate persistence and revisioning

Add an additive `QueueBatchPreflight` record keyed by an opaque candidate ID. Its JSON state stores the batch name/ID, original `QueueJobInput` selections and per-asset review status, resolved QueueJob snapshots, review findings, and current revision. It is not a QueueJob and is never claimable. `GET` restores the exact state; `Edit` marks the resolved revision stale; `Resolve` increments the revision and replaces resolved plans from the retained inputs. Accept requires the exact current revision.

### Canonical plan resolution

Preflight uses `prepareBatchQueueJob` with the existing assignment/default/profile/track resolvers, then invokes a persistence-free form of `freezeQueuePreparedProfileSnapshot`. This factors the existing PREPARING resolution so the candidate can freeze the same profile before a QueueJob exists. Capture the current persisted runtime capability snapshot once per preflight and use the existing scheduler encoder-selection rule. Do not create a second profile, track, cadence, restoration, GOP, or encoder resolver.

### Acceptance and partial acceptance

Acceptance runs in one SQLite transaction. It checks the current candidate revision, confirms each selected asset is still ready and unreserved, inserts the exact stored QueueJob snapshots, locks assets, creates the existing ExecutionPlans, and updates the candidate with queued job IDs plus remaining review entries. No acceptance request re-resolves material fields. Any failure rolls back both Queue insertion and candidate progress.

### Queue compatibility

Existing `/queue/jobs` single-job behavior and historical QueueJobs stay compatible. The legacy batch-create and multi-asset commit endpoints no longer insert QueueJobs directly; they require/return Preflight state. Existing Queue snapshots remain immutable. Complete accepted snapshots bypass claim-time profile capture; incomplete historical snapshots retain current claim preparation compatibility.

### Validation actions

Preview and Test Encode use existing APIs/engines and receive the candidate ID, exact asset identity, and candidate revision. LAB opens the existing route in a new tab with the same context; local LAB edits do not mutate candidate state. Any explicit apply updates only candidate inputs and resolves a new revision. Do not generate Test Encodes automatically or create new encoders/commands.

### NAS performance

Resolve only the selected bounded asset set. Reuse persisted ScanResult and AssetAnalysis evidence, existing batch validation context, and one runtime snapshot per candidate. Review rendering reads persisted plan summaries only; it performs no media probing, Library walk, OCR, or hash computation.

## Migration and compatibility

AutoMigrate creates the new candidate table. Existing tables and QueueJob snapshots are untouched. Old jobs without a complete snapshot keep the existing claim-time preparation path; accepted preflight snapshots carry a completion marker and use the existing QueueJob/ProfileSnapshot/AudioProfileSnapshot/TrackProfileSnapshot/ExecutionPlan authorities. Rollback can leave candidate rows readable without affecting Queue execution.

## Risks / Trade-offs

- [Existing PREPARING capture is coupled to a persisted QueueJob] -> Factor its snapshot calculation from its persistence step and share the same resolver with candidate and legacy claim preparation.
- [A candidate may be edited while another acceptance is in flight] -> Require a revision match and update candidate plus QueueJobs in one transaction.
- [Some assets cannot be accepted] -> Keep their configured inputs and reasons in the candidate; accept only an explicit subset of ready assets.
- [Destination naming can otherwise drift after acceptance] -> Store the resolved destination snapshot in the accepted QueueJob and make existing output-path consumers prefer it.
- [Runtime state may be unavailable or encoder selection may fail] -> Preserve the canonical scheduler error/warning and block acceptance until a new resolved candidate has a usable runtime decision.
