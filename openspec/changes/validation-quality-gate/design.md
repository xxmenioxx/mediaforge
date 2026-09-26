## Context

See proposal.md for motivation. Current code has a manual `POST /validation/jobs/:id` in `backend/internal/handlers/validation.go`. It changes `QueueJob.ValidationStatus`, score, and `ValidationReport` in place; current statuses are `pending`, `passed`, `warning`, and `failed`. The validator probes source and output itself, may run separate A/V timing probes, and can collect perceptual evidence. It persists `colorPolicy` and `frameFidelity` as sibling report entries; `validation_persistence_test.go` protects that shape.

Queue snapshots the effective profile/audio/track configuration on `QueueJob` and refers to a versioned `ExecutionPlan` through `ActiveExecutionPlanID`. The worker's existing `kind=result` job artifact stores the result payload, output probe, execution-plan data, and a copy of the current validation report. `ValidationPage.tsx` currently divides completed, unpublished jobs into Pending and Validated tabs and invokes validation on demand. `HistoryPage.tsx` reads completed, failed, canceled, and published jobs from the queue-job list. `AutoPublisher.tick` selects completed jobs with legacy `passed` or `warning` values, while `publishQueueJob` checks those values before entering Publisher-owned filesystem work.

## Goals / Non-Goals

**Goals:**

- Make the frozen Queue execution and its active ExecutionPlan the only expected-state authority, and the exact produced Result artifact the actual-state authority.
- Make one immutable Validation run the authority for rule findings and outcome, with concise QueueJob fields as read-optimized projections.
- Put Validation after result production, with an exception-first Review Center, controlled Publisher handoff, explicit overrides, and bounded evidence work.

**Non-Goals:**

- Changing #46 Preflight, which remains the pre-conversion eligibility owner.
- Owning #54 destination selection, delivery, overwrite, original archival, or publication filesystem mutations.
- Reproducing #69 perceptual/frame analysis or making it the only pass criterion.
- Moving or renaming `report.frameFidelity` from its sibling relationship with other report sections.
- Changing encoder selection or making Validation encode, repair, or otherwise mutate media.

## Decisions

### Evidence and persistence authorities

Each data domain has one authority:

| Domain | Authority | Other representations |
| --- | --- | --- |
| Expected conversion state | Frozen Queue effective snapshots and the active, versioned ExecutionPlan captured for that execution | Current profile, asset, global, and runtime settings are not consulted to reconstruct expectations |
| Actual output state | The matching `kind=result` job artifact produced for that execution, including its `Result`, `OutputProbe`, and artifact manifest | Validation does not build a parallel media inventory or infer facts from the source path alone |
| Quality decision | An append-only Validation run bound to the plan and Result identities | QueueJob outcome/status/report fields and list API payloads are summaries that reference the latest run, not independent decision stores |

Add a stable Result artifact generation ID and a bounded output identity snapshot when the worker completes the artifact. The identity snapshot must detect ordinary same-path replacement through the generated identity and filesystem identity/metadata captured at completion; Validation rechecks that identity before evaluating and before handoff. Path equality alone is insufficient. If the system cannot verify the exact Result identity, it records `REVIEW_REQUIRED`. Do not make Validation hash or decode an entire media file on every attempt or page load.

Persist one immutable ValidationRun per attempt, with a generated run ID, schema/validator version, ruleset ID/version, Queue execution number, frozen plan ID/version/fingerprint, Result artifact ID/generation/fingerprint, final outcome, and the complete structured findings. The ValidationRun points to the Result artifact; the Result remains immutable and owns actual-state evidence only. A retry is another run, never an edit to an earlier run. Keep large evidence payloads and logs in their existing artifact locations and link them by identity. Keep the existing QueueJob validation fields only as an atomically refreshed summary for Queue, History, and Publisher queries. Historical embedded `validationReport` values remain readable as legacy snapshots and are not used to make new decisions.

Store an override as a separate append-only event linked to a ValidationRun. It records the acting user, required reason, timestamp, override schema/version, affected rule keys, and requested handoff. It never changes the run's outcome or evidence. Artifact integrity and evidence-identity failures cannot be overridden.

### Rule evaluation and outcome reduction

Use stable rule keys and a versioned ruleset. Every finding records `expected`, `actual`, `severity` (`critical`, `error`, `warning`, or `info`), `disposition` (`blocking` or `advisory`), `state`, `actionable`, a human-readable explanation, and evidence references. Rule state is one of `PASS`, `FAIL`, `WARNING`, `REVIEW_REQUIRED`, `UNVERIFIED`, or `NOT_APPLICABLE`; the last state includes a reason. Preserve unavailable and non-comparable facts rather than coercing them to pass.

The initial ruleset covers:

- Artifact integrity: result identity, presence, readability, non-empty output, and required artifact manifest entries.
- Duration/timeline: expected duration and stream actions versus measured duration, timestamps, frame count, and A/V alignment where corresponding evidence is available.
- Video properties: planned codec/level/tier, dimensions, frame rate, field/cadence, bit depth, pixel format, and color semantics versus output probe/bitstream evidence.
- Audio: planned keep/transcode/drop actions, stream membership, channel/layout and language/metadata expectations versus actual streams.
- Subtitles: planned stream actions and converted outputs, compared with embedded streams and external subtitle artifact evidence.
- Chapters: expected retain/remap/drop action versus output chapter evidence.
- Attachments/fonts: planned attachment actions and required font artifacts versus the output and artifact manifest.
- Sidecars: each plan-required sidecar's identity, presence, readability, and output relationship.
- Publication readiness: completeness of the result package and absence of blocking validation findings. This is a non-mutating handoff check; destination and delivery checks stay with #54 Publisher.

Do not use the existing weighted score as the decision reducer. Reduce the rules deterministically: a known violation of any blocking rule yields `FAIL`; an unavailable or unresolved blocking rule yields `REVIEW_REQUIRED`; otherwise an advisory warning yields `PASS_WITH_WARNINGS`; only all-required-rules-pass with no warnings yields `PASS`. An actionable advisory warning remains visible in the Review Center and suppresses auto-advancement even though the outcome remains `PASS_WITH_WARNINGS`.

### Lifecycle, retry, review, and publication handoff

Run Validation after the worker has finalized the produced Result artifact and before the job can enter the Publisher handoff state. Queue/worker lifecycle owns this transition. A manual Review Center action may request a bounded validation retry for the same immutable Result; opening or refreshing a page is read-only. If the artifact or execution must change, Queue creates a new execution and Result before Validation runs again. Validation has no encoder or repair path.

The Review Center's default query includes `REVIEW_REQUIRED`, `FAIL`, and any actionable warning, including one attached to `PASS_WITH_WARNINGS`; clean `PASS` results are absent from that query. Pending validation work is represented separately from completed exceptions. List endpoints return a bounded summary with latest run ID/outcome and override presence. The detail endpoint loads the selected run's full findings and linked evidence lazily. History retains every completed job and exposes its latest outcome/run identity and override state after publication or review closure.

Automatic advancement is controlled by the existing automation setting plus explicit allowed outcomes. Default eligibility is clean `PASS`; policy may opt in to `PASS_WITH_WARNINGS` only when no actionable warning remains. `REVIEW_REQUIRED` and `FAIL` never auto-advance. A valid override may authorize a handoff only where the targeted rules are overrideable; it does not relabel the Validation outcome. Publisher rechecks that the current result still matches the run's Result identity and retains all destination and filesystem decisions.

### Evidence reuse and NAS cost

Consume the matching Result artifact's output probe, result payload, and manifest first. Reuse #69 perceptual/frame evidence only when its source, output generation, transform, and sampling identity match the frozen execution; it is supplemental evidence and cannot alone authorize a pass. Preserve `frameFidelity` as a sibling of `colorPolicy` in the Validation report. Do not run a new full perceptual sample or equivalent output probe merely because the Review Center rendered. Missing required evidence uses only bounded, read-only targeted fallback checks; otherwise it remains unverified and routes to review.

The worker performs post-conversion Validation once as part of the job lifecycle. Review Center and History list reads use QueueJob summaries and batch queries, not per-row probes or validation calls. Full findings, large reports, and logs are fetched only when a user opens a result's detail view.

### Migration and compatibility

Use additive persistence for immutable Validation runs and override events. Keep existing QueueJob validation columns as projections during the API transition. Historical `passed`, `warning`, and `failed` values remain visible as legacy outcomes but lack the new plan/Result binding; do not promote them to a bound `PASS` or allow them to auto-publish. Revalidate a legacy completed Result before it can use the new handoff. Do not rewrite or delete historical Result artifacts; their embedded validation reports remain explicitly legacy copies. Rollback retains new run/override rows and existing columns, and disables new automatic advancement rather than deleting audit history.

## Risks / Trade-offs

- [Existing reports do not carry a stable Result generation link] → Add the identity at worker Result creation and route unbound historical results through explicit revalidation instead of guessing from path or timestamp.
- [Additional immutable run rows increase database retention] → Store compact rule summaries and references; keep media probes and logs in existing artifacts and apply bounded retention only to non-audit debug payloads.
- [Older jobs require revalidation before handoff] → Keep their outcomes visible as legacy in History and provide a bounded migration/revalidation path; never silently relabel them.
- [A QueueJob summary can drift from the latest immutable run] → Update the projection and active run reference atomically; Publisher authorizes against the canonical run, not the projection.
- [Missing evidence can increase manual review] → Reuse worker-produced Result data and make every unverified rule explicit so operators can distinguish a media fault from unavailable evidence.

## Migration Plan

1. Add append-only ValidationRun and override persistence plus a stable Result generation/output identity; retain the existing QueueJob columns as compatibility summaries.
2. Write the Result artifact once at worker completion, then create its Validation run from the matching frozen plan and Result. Publish the run ID/outcome summary atomically to the QueueJob.
3. Update Review Center, History, and Publisher eligibility reads to consume the canonical run reference; keep historical reports readable as legacy and revalidate before new-policy handoff.
4. Enable automatic advancement only after the new outcome policy is configured. Rollback disables advancement and preserves all Result, ValidationRun, and override records.
