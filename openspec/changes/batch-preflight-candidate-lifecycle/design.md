## Context

See proposal.md for motivation and the delta spec for user-visible behavior. The current candidate row stores `Revision`, `Resolved`, and review data in JSON but has no terminal lifecycle state. `UpdateBatchPreflight` requires every unresolved asset and the resolver requires at least one job, so neither operation can safely remove the final blocked asset. The Assets page currently fetches the saved candidate on mount and opens it; the dialog's Close action only clears component state.

The existing candidate is not a QueueJob and does not reserve an asset. Accept already uses a transaction to create QueueJobs and frozen ExecutionPlans from the reviewed snapshots. This design keeps those boundaries unchanged.

## Goals / Non-Goals

**Goals:**

- Persist candidate lifecycle and safely remove or abandon only unaccepted candidate work.
- Preserve revision fencing, existing accepted-job snapshots, and partial-acceptance provenance.
- Make saved-candidate recovery an explicit user action without polling or automatic modal restoration.
- Keep Edit/Resolve semantics and the canonical Queue resolver unchanged.

**Non-Goals:**

- Canceling or rewriting accepted QueueJobs, ExecutionPlans, or SchedulerReservations.
- Changing Queue execution, media files, archive behavior, or #311's archive UI refresh.
- Adding candidate discovery across users/devices or multiple saved-candidate history UI.

## Decisions

### Persist explicit lifecycle state

Add a `status` column to `QueueBatchPreflight` with `active`, `completed`, and `discarded` values. AutoMigrate gives existing rows the `active` default. Include status in API responses. Existing accept marks a candidate completed only when all its entries are queued; otherwise it remains active. Candidate rows are retained instead of hard-deleted.

### Add dedicated remove and discard operations

Keep `PUT` Edit/Resolve behavior unchanged. Add revision-bearing `POST .../:id/remove` and `POST .../:id/discard` endpoints. Each operation runs in a database transaction, checks active status and the expected revision, and updates with an `id + revision + status` compare-and-swap predicate. Stale or duplicate submissions return a conflict without changing state.

Remove identifies a candidate item by its stable candidate asset ID, rejects queued items, removes only that item from the active candidate state, and advances the revision. It does not re-resolve other assets: their inputs, source identities, and exact reviewed plans are unchanged. Removing the last unqueued item terminalizes the candidate as discarded, unless the candidate contains only accepted entries, in which case it is completed. Discard removes all unqueued entries from active eligibility, retains accepted entries and queued job IDs in the stored state, and marks the candidate discarded. Neither operation creates, updates, deletes, or releases QueueJobs, ExecutionPlans, or reservations.

Existing Edit, Resolve, Accept, Remove, and Discard paths reject terminal candidates. GET returns the persisted terminal status so explicit Resume can clear the matching stale browser reference. Accept remains transactional and changes status to completed when it accepts the final pending entry.

### Separate candidate reference from dialog intent

Keep `mvforge.queueBatchPreflightCandidateId` as the saved candidate reference. Remove automatic GET-and-open from the Assets mount effect. Add an explicit Resume action that fetches the referenced candidate and opens it only if the request is still current and the candidate remains active. Create/open events display the just-created active candidate; they do not represent a persisted auto-open preference. Close dismisses the modal and leaves the reference intact. Discard and terminal acceptance clear it only when its ID still matches, preventing an older response from clearing a newer candidate.

Use an in-memory request generation and discarded-ID guard around events and Resume requests. Close, a new candidate event, and discard invalidate older lookups. A late lookup may update neither the modal nor the reference after its generation or candidate ID changes. No polling is added. Storage access failure leaves the current page usable and does not silently create or discard server candidates.

### Keep UI actions unambiguous

Expose separate Close, Discard candidate, and Discard edits actions. Show Remove from candidate for each unqueued item in review and edit states; removing in edit mode also removes that identity from the local edit and baseline drafts after the server confirms the new revision. A terminal remove/discard closes the dialog and clears the saved reference. Revision conflicts retain the current dialog and show recovery guidance so the user can explicitly reload the latest candidate.

### Tests and documentation

Use disposable SQLite databases and handler routers to verify transactional status/revision changes, restart persistence, and preservation of accepted jobs/plans/reservations. Frontend tests cover storage and modal lifecycle, including deferred GET/event races. Update the existing Queue Batch Preflight architecture guide with the implemented lifecycle contract; do not change media or NAS data.

## Risks / Trade-offs

- Existing rows need a safe lifecycle value → use an additive non-null column with an `active` default and migration coverage.
- Multiple clients can submit the same revision → use transactional compare-and-swap updates and reject terminal states.
- A stale UI might submit a valid-looking old event → bind display to the saved candidate ID, request generation, and active status; never restore automatically from GET.
- Removing an asset intentionally drops it from the active candidate's asset list → preserve all accepted entries and their job IDs; only explicitly removed unqueued work leaves active eligibility.
- Storage may be unavailable → allow immediate event-based open in the current page while keeping explicit recovery best-effort and reporting unavailable references clearly.
