# Task 4 — Implement approved Queue selected backend plan

Follow all applicable `AGENTS.md` files.

Implement the approved Task 4 plan below exactly, adapting only where current
repository structure requires it.

1. Existing functions/endpoints to reuse.
   - effectiveAssetConfiguration() for authoritative inheritance resolution.
   - QueueHandler.resolveProfileAssignments() to materialize effective video, audio, tracks, category, destination, and provenance.
   - QueueHandler.prepareBatchQueueJob() for review, duplicate, maintenance, publish-mode, profile, and snapshot validation.
   - QueueHandler.captureProfile(), captureOverrideOnlyProfile(), and captureSupplementalProfiles() for immutable per-job snapshots.
   - scheduler.LockQueuedAsset(), transitionJobStage(), and scheduler.CreatePendingExecutionPlan() for atomic Queue creation.
   - assetSequenceLess() for natural asset ordering.
   - Existing POST /api/queue/batches remains unchanged for other callers.
   - Existing Task 02 selection helpers remain the frontend source of selected Asset IDs.
   - The Task 03 effective-configuration batch endpoint remains available for display but is no longer used to decide Queue selected.
2. Exact files to change.
   - backend/internal/handlers/queue.go
   - backend/internal/handlers/queue_test.go
   - backend/internal/handlers/swagger.go
   - backend/internal/routes/routes.go
   - frontend/src/api/types.ts
   - frontend/src/api/client.ts
   - frontend/src/pages/AssetsPage.tsx
   - frontend/src/pages/AssetsPage.unprocessed-render.test.tsx
3. Minimal API contract changes.
Add:
POST /api/queue/selected-assets
Request:
{
  "assetIds": [12, 15, 18],
  "commit": false
}
- assetIds is required, non-empty, deduplicated by the backend, and bounded to a safe batch size.
- commit: false performs authoritative planning only.
- commit: true repeats all resolution and eligibility checks before creating jobs.
- The frontend sends no paths, profile IDs, destination IDs, eligibility decisions, or prepared Queue jobs.
Response:
{
  "summary": {
    "selected": 3,
    "eligible": 1,
    "queued": 0,
    "skipped": 1,
    "failed": 1,
    "titleCount": 2,
    "sizeBytes": 734003200
  },
  "results": [
    {
      "assetId": 12,
      "outcome": "eligible",
      "reason": "",
      "batchId": "selected-...",
      "batchName": "Season 1"
    },
    {
      "assetId": 15,
      "outcome": "skipped",
      "reason": "already_queued",
      "message": "Asset already has an open Queue job"
    },
    {
      "assetId": 18,
      "outcome": "failed",
      "reason": "invalid_configuration",
      "message": "No effective destination is configured"
    }
  ],
  "batches": []
}
For commit: true, eligible results become queued and include jobId; batches contains created batch IDs and names. Expected non-queueable assets return HTTP 200 with explicit per-asset outcomes. Invalid request structure returns 400; unexpected persistence failures return 500 only when no reliable partial result can be returned.
Stable reason codes:
- not_found
- missing
- needs_review
- already_queued
- active_maintenance
- invalid_configuration
- reservation_conflict
- queue_creation_failed
4. Step-by-step implementation sequence.
5. Add request, summary, result, batch-result, and response types in queue.go.
6. Load all requested AssetRecord rows in one query, deduplicate IDs, and emit not_found results for absent IDs.
7. Sort records by natural LogicalGroupPath → GroupPath → Asset order using normalized paths and assetSequenceLess().
8. For each asset, revalidate missing, Needs Review, open Queue job, and active maintenance state.
9. Resolve effective configuration exclusively in the backend using effectiveAssetConfiguration() and resolveProfileAssignments().
10. Reject assets lacking an effective destination or any valid processing operation; do not require a direct Path assignment.
11. Build internal QueueJobInput values from resolved backend state. Preserve the existing backend fallback required for audio/track-only jobs without exposing it to React.
12. Run existing preparation and snapshot functions for every eligible asset during both planning and commit.
13. Group prepared assets by physical containing path, assign deterministic batch IDs/names, and preserve the sorted LogicalGroup/Path/Asset order.
14. On commit: true, hold assetMutationMu, re-run resolution and eligibility, then persist each prepared path batch using extracted reusable transaction logic from CreateBatch.
15. Preserve atomicity within each physical-path batch. If one path batch fails, mark its affected assets explicitly and continue with independent path batches so partial success remains visible.
16. Register the route and document the request/response in Swagger.
17. Replace the frontend preparation call with commit: false and render counts from the backend response.
18. Replace the frontend loop over createQueueBatch() with one commit: true request.
19. Keep the existing dialog layout, selection state, confirmation step, invalidations, and Queue navigation. Show explicit skipped/failed summaries after commit instead of collapsing them into a generic error.
20. Remove frontend effective-configuration, fallback-profile, eligibility, path-grouping, and QueueJobInput construction from UnprocessedSelectionToolbar.
21. Tests to add/update.
    - Backend planning returns one ordered result for every requested Asset ID.
    - Duplicate IDs are processed once.
    - Unknown IDs return not_found.
    - Missing assets return missing.
    - Needs Review assets return needs_review.
    - Open jobs return already_queued.
    - Active maintenance returns active_maintenance.
    - Inherited LogicalGroup or SourceGroup configuration queues successfully without a direct Path assignment.
    - Missing destination and no-operation configurations return invalid_configuration.
    - Planning creates no Queue jobs, reservations, execution plans, or lifecycle history.
    - Commit revalidates state rather than trusting the preceding plan.
    - Commit creates immutable video, audio, tracks, overrides, and profile-resolution snapshots per asset.
    - Natural LogicalGroup → Path → Asset ordering is preserved across created batches and jobs.
    - One conflicting path batch does not hide or roll back successful independent path batches.
    - Each skipped or failed asset has a stable reason code and message.
    - Existing CreateBatch natural-order and atomic rollback tests continue passing.
    - Frontend Queue selected sends only Asset IDs plus commit.
    - Frontend planning no longer calls effectiveAssetConfigurations() or createQueueBatch().
    - Confirmation performs one backend commit request.
    - Dialog counts come from the backend response.
    - Partial success remains visible and successful commit invalidates queueJobs and assets.
22. Risks or compatibility concerns.
    - POST /api/queue/batches must remain backward compatible for Queue, folder, and other existing callers.
    - Extracting its persistence transaction must preserve atomic batch creation, reservations, stage history, queue positions, and execution plans.
    - Planning snapshots are provisional; only commit snapshots are persisted, and commit must always resolve again.
    - Audio/track-only jobs currently rely on a fallback video profile ID for legacy storage. Moving that choice into the backend must preserve audio_only processing and effective provenance.
    - Batch IDs must avoid collisions across repeated submissions while remaining deterministic within one response.
    - A commit retried after a network timeout may encounter jobs created by the first request; those assets should return already_queued, not create duplicates.
    - Partial success changes frontend error handling: HTTP success does not imply every selected asset queued.
    - Large selections require a bounded request size and bulk AssetRecord loading to avoid new N+1 HTTP or database behavior.
    - Existing uncommitted work from Tasks 01–03 must be preserved without unrelated formatting or UI changes.

## Constraints
- Frontend input is primarily selected Asset IDs.
- Backend owns effective config, eligibility, Queue planning, and snapshots.
- Do not require direct Path assignments when inheritance is valid.
- Preserve duplicate prevention, Needs Review, missing rules, natural order,
  and per-asset snapshots.
- Partial success must be explicit, never hidden behind a generic total error.
- Do not redesign Unprocessed UI or other lifecycle tabs.

Add focused regression tests and run required validation.

Implement; do not produce another plan.
