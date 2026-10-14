## Context

See `proposal.md` for motivation and `specs/validation-publisher-handoff/spec.md` for the behavior contract.

The local worker marks Queue conversion `status=completed`, writes the Result artifact, and enters the automated pipeline. A status-only worker callback and Queue status update also use `terminalStageForStatus`, which currently maps `completed` directly to `ready_to_publish` without a ValidationRun. Manual Validation has its own endpoint. Both Validation routes persist immutable `ValidationRun` records, but manual routing currently requires exactly `PASS`; automatic routing consults `validation.autoAdvanceOutcomes` and separately rejects `ActionableWarnings`. The canonical Publisher gate also rejects actionable unresolved findings, even when their disposition is advisory.

Auto Publisher selects only completed jobs already at `ready_to_publish` (or durable publication rows eligible for recovery). Its publication claim transaction persists the `publishing` stage with the fenced attempt. Publisher delivery independently verifies the ValidationRun/Result identity, sidecars, replacement authorization, destination, and manifest. Startup recovery does not promote review jobs. The Queue dialog currently treats Queue `status=completed` as a terminal lifecycle state even though that status is set when conversion ends.

The backend instructions point to overview, Queue lifecycle, and conversion-pipeline architecture pages that are absent in this checkout. This change keeps the handoff contract and transition matrix in its OpenSpec artifacts.

## Goals / Non-Goals

**Goals:**

- Reuse one pure outcome/finding decision for manual and automatic Validation handoff.
- Keep automatic authorization separate from handoff eligibility and make the current setting the explicit opt-in for warning outcomes.
- Preserve exact Validation/Result identity, immutable runs, bounded manual overrides, and Publisher delivery controls.
- Make stage history writes safe under concurrent transitions and prevent work from proceeding after a failed stage write.
- Keep the conversion-complete projection distinct from final publication completion in the Queue dialog.

**Non-Goals:**

- Change Validation rules, scores, policy schema, Result format, or Publisher destination and filesystem semantics.
- Add a policy alias, automatic migration, historical stage backfill, or bulk release operation.
- Make an advisory warning blocking because it is actionable, or make a blocking finding advisory because it is not actionable.
- Redesign the broader Validation page or Publisher workflow from #338/#326.

## Decisions

### One handoff decision, then a separate automatic authorization check

Add a pure canonical handoff decision over the immutable ValidationRun outcome and findings. It accepts `PASS` only when there is no unresolved blocking finding. It accepts `PASS_WITH_WARNINGS` only when the warning findings are advisory and no unresolved blocking finding contradicts the outcome. `Actionable` remains a review/UI signal and is not part of that decision. `FAIL`, `REVIEW_REQUIRED`, blocking `WARNING`, critical blocking `UNVERIFIED`, and inconsistent outcome/finding combinations do not hand off to Ready.

Manual `ValidateJob` and the worker route use this same decision to choose `ready_to_publish` or `validation_review`. The automatic route additionally requires the current `validation.autoAdvanceOutcomes` list to include the outcome. The Publisher automatic gate rechecks that policy at delivery time. The existing independent `pipelineAutomation.autoPublisherEnabled` switch still controls whether a worker starts publishing immediately and whether Auto Publisher ticks process ready jobs.

The existing manual override contract from `publisher-manual-validation-override-handoff` remains a separate Publisher authorization for a `REVIEW_REQUIRED` run: only exact Publisher-scoped evidence covering every overrideable blocking finding, plus the required operator reason, can authorize manual delivery. It does not change the Validation outcome or stage, does not authorize Auto Publisher, and does not make `FAIL`, mandatory evidence, or identity failures publishable. No legacy `validation_status`, score, or policy skip can substitute for canonical evidence.

**Alternatives considered:** Derive handoff from `ActionableWarnings` or add a second manual/automatic outcome list. Both preserve duplicate/inconsistent gates and recreate the current bug. Reuse one finding decision and apply the existing automatic policy only as a second authorization step.

### Preserve the existing explicit automatic opt-in semantics

Do not add a setting or migration. Missing or empty `autoAdvanceOutcomes` continues to mean `PASS` only, matching the current default. Existing settings are read without rewriting; `PASS_WITH_WARNINGS` requires the operator's explicit value. Manual advisory-only warnings may become Ready without changing automatic authorization. Auto Publisher can claim a warning job only when the current automatic policy includes the warning outcome and the separate Auto Publisher switch is enabled.

No migration moves historical `validation_review` jobs to Ready. Historical jobs already at Ready are evaluated with the current policy and all normal canonical Publisher checks; an existing explicit warning opt-in is treated as operator authorization. A policy change is effective at the next authorization check: removing warnings stops a not-yet-claimed automatic attempt, while adding warnings is an explicit operator opt-in for eligible ready warning jobs. ValidationRun findings, frozen policy identity, Result bytes, and stage history are never rewritten by that policy change.

**Alternatives considered:** Default warnings on for simpler UX, or freeze automatic authorization inside a new ValidationRun field. The first could release historical output after deployment; the second requires a schema/API migration and duplicates a live operational authorization. Retain the existing persisted setting with its fail-closed default and recheck it at the Publisher boundary.

### Persist a stage before starting its dependent work

`runAutomatedPipeline` becomes error-returning. It first persists `validating` when automatic Validation is enabled; it stops if that write fails. It then persists the ValidationRun, computes the shared decision, and persists `validation_review` or `ready_to_publish`. If writing Ready fails, it returns the persistence error and does not call Publisher. When Auto Publisher is disabled, a successful handoff remains at Ready for the later tick. A status-only completion callback without a canonical Result/ValidationRun maps to `validation_review`; only the actual Validation handoff may move a job to Ready. Repeated status updates preserve a completed job's current handoff stage, and already-published jobs remain terminal.

`monitorFFmpegJob` remains responsible for conversion completion and Result artifact persistence, but it must not swallow a stage or pipeline error. It reports the error through the worker error/log path and leaves the last durable lifecycle stage intact. The manual endpoint also propagates an error when it cannot persist the fallback review stage after Validation fails, retaining both causes.

Make `transitionJobStage` reload the current persisted row inside a transaction, append history to that current state, and condition the update on the observed stage/timestamp. A competing transition either observes the already-persisted target (idempotent no-op) or returns a conflict/error; it cannot overwrite a newer history with a stale slice. The helper updates the caller's model only after a successful write. In `claimPublication`, `publishing`, publication state, lease, and attempt evidence remain one transaction. A failure there rolls the claim back before filesystem work.

**Alternatives considered:** Continue to mutate stale in-memory stage histories and rely on startup reconciliation, or write a separate transition event table. Reconciliation cannot prevent a Publisher attempt after a failed Ready write; another table/schema is unnecessary because Queue already stores ordered stage history.

### Keep retries and restart recovery on the existing durable Publisher authority

Do not add a timer or delay between Ready and Publisher. A successful immediate automatic path records Ready before `claimPublication` records Publishing. Auto Publisher's existing cadence handles Ready jobs; expired `publishing` leases recover the same durable attempt with a new fencing generation. A pre-claim sidecar failure stays at Ready with a blocked publication record. Post-claim failures keep their durable failed/blocked attempt result; only the existing retry/recovery paths may claim again. A concurrent manual or automatic request must still pass the transaction/fence and canonical revalidation.

Policy is evaluated from the current settings at the automatic gate and again during bound-delivery verification. If policy changes before claim, the job cannot publish under a removed authorization. If it changes after the durable claim, delivery verification rechecks before promotion; an interrupted claimed attempt recovers through the same identity and fence rules.

### Present conversion and publication as different milestones

Treat Queue `status=completed` as evidence that conversion finished, not that the complete lifecycle is terminal. In the dialog, lifecycle terminal state comes from persisted terminal stage (`completed`, `failed`, or `canceled`) and the `publishedAt`/publication record, not from conversion status. Display conversion completion separately from publication completion. The existing `completed` stage is written by the successful Publisher commit, so it remains the final lifecycle event; no synthetic historical stage is added.

The lifecycle checklist continues to render persisted `stageHistory`. For a completed conversion whose stage is `validation_review`, `ready_to_publish`, or `publishing`, the current lifecycle step remains pending/active as appropriate and publication remains incomplete. Only a persisted completed/published event marks the final milestone successful. Historical gaps remain “not recorded”; the UI does not invent a publication event from Queue status.

**Alternatives considered:** Infer `completed` from Queue status, as today, or synthesize a `completed` history event when rendering. Both conflate conversion with publication or fabricate lifecycle history.

## Transition Matrix

| Validation outcome and canonical findings | Route and policy | Handoff / Publisher authorization | Durable stage and retry behavior |
|---|---|---|---|
| `PASS`; no unresolved blocking findings | Manual Validation | Handoff eligible. Manual Publisher still rechecks exact identity and delivery preflights. | `validating → ready_to_publish`; `publishing` only after a successful claim. |
| `PASS`; no unresolved blocking findings | Automatic Validation enabled; `PASS` allowed (missing/empty list keeps this default) | Handoff eligible; Auto Publisher additionally requires its own enabled switch and all Publisher gates. | `validating → ready_to_publish → publishing` when auto publish starts; otherwise remain Ready. |
| `PASS_WITH_WARNINGS`; all findings advisory, `Actionable=false` | Manual Validation | Handoff eligible; warning remains visible. | `validating → ready_to_publish`. |
| `PASS_WITH_WARNINGS`; all findings advisory, `Actionable=true` | Manual Validation | Same as nonactionable advisory; actionability does not change blocking disposition. Warning remains visible. | `validating → ready_to_publish`. |
| `PASS_WITH_WARNINGS`; all findings advisory, either actionability value | Automatic Validation enabled; warning explicitly in `autoAdvanceOutcomes` | Handoff eligible; Auto Publisher requires enabled switch, current warning authorization, exact identity, and delivery checks. | Ready is persisted before Publishing. |
| `PASS_WITH_WARNINGS`; all findings advisory, either actionability value | Automatic Validation enabled; warning absent from policy | No automatic handoff; manual Validation remains eligible. | Automatic route ends at `validation_review`; no publication attempt. |
| `PASS_WITH_WARNINGS` with an unresolved blocking finding, or inconsistent outcome/findings | Manual or automatic Validation | No Ready handoff. A `REVIEW_REQUIRED` manual Publisher exception is possible only under the exact #326 override contract for overrideable findings; automatic Publisher remains blocked. | `validating → validation_review`; no automatic claim. |
| `REVIEW_REQUIRED` with blocking overrideable findings | Manual or automatic Validation | No Ready handoff. Manual Publisher may proceed only with exact Publisher-scoped overrides for every eligible finding and a nonempty reason; automatic Publisher is denied. | Remains `validation_review`; manual Publisher attempt is separately audited. |
| `REVIEW_REQUIRED` with mandatory/non-overrideable findings | Manual or automatic Validation | Blocked for both Publisher routes. | `validating → validation_review`; no publication row/attempt where canonical gate rejects before identity persistence. |
| `FAIL` | Manual or automatic Validation | Blocked for both Publisher routes; overrides, score, legacy status, and skip settings do not authorize it. | `validating → validation_review`; no publication attempt. |
| Critical `UNVERIFIED` or other unresolved blocking evidence | Manual or automatic Validation | Blocked regardless of actionability or automatic outcome list. | `validating → validation_review`; no publication attempt. |
| Advisory `UNVERIFIED` included in a valid `PASS_WITH_WARNINGS` run | Manual; automatic only with explicit warning policy | Same behavior as other advisory warnings if canonical rules classify it as noncritical; critical evidence cannot be classified advisory to bypass a mandatory rule. | `validating → ready_to_publish` only when outcome/finding consistency holds. |
| Automatic Validation disabled | Worker completion | No automatic ValidationRun or Publisher handoff; later manual Validation uses the shared decision. | Persist `validation_review`; no `validating` event for skipped validation. |
| Status-only worker/Queue update sets conversion status to completed without running Validation | Callback or manual Queue status update | No canonical ValidationRun exists to authorize Ready or Publisher. A later manual Validation is required. | `validation_review`; never map status completion directly to `ready_to_publish`. Repeated completion preserves the current lifecycle stage. |
| Auto Publisher disabled after eligible handoff | Automatic or manual Validation | Job is eligible at Ready, but no immediate/periodic automatic attempt occurs. Manual Publisher remains independently available. | Remain durably `ready_to_publish`. |
| Missing/invalid Result identity, stale output revision, or execution binding mismatch | Manual or automatic Publisher | Canonical gate rejects before publication claim; legacy status/outcome cannot substitute. | Remain at existing review/ready stage; no `publishing` transition. |
| Required sidecar unavailable before claim | Manual or automatic Publisher | Publication is blocked; no delivery starts. Existing blocked-state behavior determines retry/repair. | Remain `ready_to_publish`; no `publishing` event. |
| Destination, frozen manifest, or replacement authority fails before/after claim | Manual or automatic Publisher | Block/fail under existing Publisher preflight and fencing behavior; never mark published. | Before claim: stay Ready. After claim: `publishing` is recorded and the durable attempt is failed/blocked for safe retry or review. |
| Transient Publisher error before original archival | Publisher attempt | Do not infer publication; preserve exact attempt failure and retry ownership rules. | Attempt may safely return to Ready only through the existing rollback path; failed/blocked records require the existing explicit retry path. |
| Restart while Ready | Auto Publisher enabled | Tick selects only persisted Ready jobs and rechecks current policy and canonical identity. Review jobs are not promoted. | Ready history remains; a new claim records Publishing. |
| Restart while Publishing with expired lease | Auto Publisher recovery | Recover the same publication/attempt identity with a higher fence; do not start a second active attempt. | Keep the original Publishing stage event; recovery evidence records the renewed attempt ownership. |
| Concurrent Validation handoff or Publisher requests | Same Queue execution | Conditional stage updates prevent stale history overwrite; the publication transaction/fence permits at most one active attempt. Losers receive a conflict or idempotent current result. | Only committed transitions enter history; a failed stage write stops dependent work. |
| Automatic policy changes before authorization | New handoff or not-yet-claimed Publisher attempt | Re-evaluate current policy. Removing warnings blocks; explicitly adding warnings authorizes eligible warning jobs but not blocking findings. | No ValidationRun or history rewrite; existing Ready warning jobs are eligible only after explicit policy opt-in and normal checks. |

## Risks / Trade-offs

- [Risk] Existing installations that already explicitly list `PASS_WITH_WARNINGS` can automatically publish eligible jobs already at Ready after deployment → this is treated as the existing operator opt-in; absent/empty settings remain PASS-only, and no historical stage backfill is performed.
- [Risk] A concurrent stage writer may now receive a conflict instead of silently overwriting history → fail closed, return the persistence/concurrency error, and let the caller retry from the current durable row.
- [Risk] A destination can fail after `publishing` is durably claimed → preserve the attempt as failed/blocked and rely on existing manual retry or fenced restart recovery; do not claim publication completion.
- [Risk] Historical jobs can have incomplete stage history → show “not recorded” and use current canonical publication evidence without fabricating lifecycle events.
- [Risk] SQLite transaction/CAS overhead adds a read and conditional write to rare lifecycle boundaries → negligible compared with media processing and publication I/O; no polling or file scan is added.

## Migration Plan

No database migration or bulk data update. Preserve all existing `validation.autoAdvanceOutcomes` values. Missing/empty settings remain PASS-only. Deploying code does not move historical review jobs. A deliberate operator change to include `PASS_WITH_WARNINGS` is the opt-in; rollback restores the previous code while leaving immutable Validation and Publisher history untouched. Normal rollback must not rewrite already committed publication records or files.

## Open Questions

None. The existing #326 manual override contract resolves the review-required manual exception; automatic authorization remains independently policy-gated.
