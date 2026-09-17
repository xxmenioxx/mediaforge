## Context

See proposal.md for the motivation and `specs/subtitles/spec.md` for the
observable contract. Asset Info already keeps subtitle extraction operations in
an in-process registry with an operation ID, status, phase, progress, result,
and error. Text subtitle extraction currently performs FFmpeg work inline,
while bitmap OCR is represented as a running operation and executes in a
background goroutine. The canonical subtitle pipeline already owns source
classification, converted format decisions, shared `both` cue generation,
staging, validation, and artifact metadata.

The cancellation design must therefore extend the existing operation lifecycle
and execution context. It must also account for the NAS process boundary:
FFmpeg, SeConv, and Tesseract can run for a long time, and terminating only a
direct process can leave descendants alive or leave temporary workspace behind.

## Goals / Non-Goals

**Goals:**

- Represent cancellable text conversion and bitmap OCR work with the existing
  Asset Info subtitle operation model.
- Give each operation one cancellation context that reaches every canonical
  extraction, OCR, normalization, rendering, and cleanup step.
- Terminate complete external-process trees promptly and classify cancellation
  separately from tool failure.
- Make sidecar publication transactional at the operation boundary: only fully
  validated requested outputs become visible.
- Preserve valid existing/completed sidecars and make a cancelled request
  retryable.
- Keep the existing Asset Info polling and restoration behavior coherent with
  the new terminal state.

**Non-Goals:**

- Introducing QueueJob, scheduler reservations, or another durable/background
  job system for Asset Info subtitle work.
- Changing the canonical subtitle resolver, OCR semantics, `both` shared-cue
  behavior, sidecar discovery/matching, or artifact naming.
- Cancelling unrelated Asset Info scans, previews, maintenance actions, or
  Queue/Worker jobs.
- Persisting in-progress operation state across backend restarts; the current
  operation registry is in-process, while source media and completed sidecars
  remain durable.

## Decisions

1. **Extend the existing operation registry and state machine.**
   Every request that has new conversion work will create or use one existing
   subtitle extraction operation before launching long-running work. The
   operation retains its current identity and progress fields, with
   `cancelled` added as a terminal status alongside the existing completion and
   error outcomes. An internal cancellation controller may be associated with
   the operation ID, but it is not a second job record or scheduling system.
   A synchronized transition makes Cancel idempotent and resolves races by
   allowing exactly one terminal outcome; once publication has committed, normal
   completion wins and a later Cancel returns that completed snapshot.
   Terminal states are immutable: retrying a cancelled request creates a fresh
   operation ID, controller, and context rather than restarting the old
   operation.

2. **Use an operation-owned work context and an independent cleanup context.**
   After validation and canonical planning, the operation creates a bounded
   work context with cancellation and runs conversion under it. The request
   context is used for request validation only; it is not retained by a
   background operation. Every helper in the canonical subtitle execution
   chain accepts and forwards the work context, checking it between stages so
   cancellation also stops work that has not yet launched a child process.
   Once work is cancelled, cleanup runs through an independent bounded context
   derived from a non-cancelled root (or an equivalent cleanup path that is not
   governed by the cancelled work context). Cleanup waits for process and pipe
   termination where needed, removes only operation-owned staging/workspace,
   records cleanup diagnostics as cancellation diagnostics rather than a
   conversion failure, and is attempted before the operation becomes
   terminally `cancelled`. Even if cleanup reaches its bound or reports an
   error, user cancellation remains the final status and no staged output may
   be published.

3. **Terminate process trees, not only direct commands.**
   External tools continue to receive argument arrays and run through the
   cancellable command path. On the supported backend runtime, the command is
   placed in an operation-owned process group. Context cancellation triggers a
   bounded graceful termination followed by group force termination if needed,
   then waits for all pipes and child processes to close. This applies equally
   to FFmpeg extraction/rendering, SeConv OCR/cleanup, Tesseract, and any
   intermediate helper they launch. The process runner remains separate from
   subtitle policy and does not construct shell commands.

4. **Make operation output staging and publication explicit.**
   Each new sidecar and OCR intermediate is created below a uniquely owned
   temporary workspace or staged path. Existing valid sidecars are identified
   by the canonical plan before execution and are never removed. A logical
   multi-output request such as converted `both` owns one staged output set:
   SRT and ASS are generated from the one canonical normalized cue result,
   validated together, and neither newly generated sidecar is published
   independently before the complete requested set crosses one publication
   barrier. If cancellation occurs while the second output is being generated,
   cleanup removes every staged output owned by the current operation. It must
   not remove final sidecars that predated the operation or were published by a
   previously completed operation. A short commit section is serialized so a
   cancellation racing with publication either aborts before commit or
   observes a fully committed result, never a partially committed file set.

5. **Classify context cancellation before generic tool errors.**
   After the independent cleanup attempt, the final operation update checks
   whether the operation work context was cancelled. Context cancellation
   produces status `cancelled` and a user-facing cancellation message; cleanup
   diagnostics remain attached to that cancellation outcome and do not
   populate the conversion-failure classification. A non-cancellation exit,
   invalid output, unavailable OCR tool, or validation error retains the
   existing failure status and diagnostic. Cancellation must not be wrapped
   into a misleading failed result merely because a child exits non-zero after
   receiving the termination signal.

6. **Add an operation cancellation endpoint without changing canonical plans.**
   Add an action endpoint addressed by operation ID, returning the current
   operation snapshot. It accepts cancellation only for a running operation,
   safely handles duplicate requests and completion races, and does not accept a
   media path as an alternate identity. The existing start, get, and list
   operations remain the source of truth for clients; responses add the
   `cancelled` status while retaining existing immediate-result behavior when a
   request has no new work to run. Retry starts a new operation identity, and
   the old cancelled snapshot can never be moved back to `running`.

7. **Stop and restart frontend polling through terminal state.**
   The Asset Info client adds the cancel action and the `cancelled` status to
   shared API types. The current polling loop treats `cancelled` as terminal,
   clears its poller bookkeeping and running progress presentation, and avoids
   converting cancellation into an error toast. While cancellation is pending,
   the control is disabled to prevent accidental repeated actions. Once the
   cancelled snapshot is visible, the existing generation action can start a
   fresh canonical operation; existing sidecar indicators remain intact.

8. **Preserve canonical subtitle responsibility.**
   The change threads cancellation through the existing subtitle plan and
   generation functions rather than adding Asset Info-specific OCR, cue
   normalization, or format-selection logic. The canonical `both` plan still
   performs extraction/OCR once and renders SRT and ASS from the shared result;
   cancellation only controls execution and commit boundaries around that plan.

## Risks / Trade-offs

- **[Risk]** Process-group behavior can differ across host platforms or wrapper
  processes. **Mitigation:** keep the backend's supported runtime explicit,
  test direct and descendant termination with disposable fixtures, and use a
  bounded escalation path before declaring cancellation complete.
- **[Risk]** A cancellation race during publication could leave an incomplete
  sidecar set. **Mitigation:** stage all new outputs, validate the complete
  requested set, and use one guarded publication barrier with no destructive
  replacement of existing artifacts.
- **[Risk]** Moving text conversion from an inline response to an operation can
  affect clients that assume a result is always returned immediately.
  **Mitigation:** retain the existing immediate result shape when no work is
  needed, make operation responses additive and explicit, and update the Asset
  Info client and API types together.
- **[Risk]** A cancelled in-memory operation may disappear on process restart.
  **Mitigation:** do not claim durable resume; preserve all source and already
  published sidecars, and allow a new request to retry the canonical conversion.
- **[Risk]** OCR cleanup and cancellation can add short-lived filesystem and
  process overhead on the NAS. **Mitigation:** scope work to one operation-owned
  temporary directory, avoid duplicate canonical work, and remove it in a
  bounded deferred cleanup path.

## Migration Plan

No database migration or Queue snapshot migration is required. Deploy the
backend operation/API changes and frontend status handling together. Existing
completed or failed operation snapshots retain their current meaning; newly
running operations gain cancellation and the new terminal status. A rollback is
code-only and leaves source media, existing sidecars, and any fully published
artifacts untouched. Any operation interrupted by a backend restart remains
non-resumable as it is today and can be started again by the user.
