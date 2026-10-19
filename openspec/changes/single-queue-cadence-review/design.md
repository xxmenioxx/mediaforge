## Context

See proposal.md. Batch Preflight already owns effective profile preparation, canonical cadence/HEVC resolution, frozen-plan presentation, and transactional acceptance. The single Queue handler currently takes a shorter persistence path. Cadence resolution already records an explicit warning and unknown effective FPS when IVTC lacks an explicit or validated field order.

## Goals / Non-Goals

**Goals:**
- Reuse the existing Queue Batch Preflight preparation and acceptance boundary for user-visible single-asset Queue review.
- Preserve the cadence resolver as the sole authority for IVTC order, filters, and effective output FPS, and the HEVC planner as the sole authority for Level/Tier.
- Store optional cadence-mode and field-order choices on one candidate asset, show the re-resolved plan and source evidence, and queue only after acceptance.
- Keep direct API creation fail-closed for ambiguous automatic IVTC before creating Queue state.

**Non-Goals:**
- Change IVTC confidence thresholds, infer order from stream metadata, modify Job #2191 or any historical plan, or change the Worker’s final gate.
- Read NAS state, inspect production media, run ffprobe/FFmpeg on production inputs, or introduce new analysis.
- Add a database column, migration, new persisted lifecycle status, or reusable profile/global cadence preference.

## Decisions

1. **Use the existing one-item Batch Preflight candidate.** Single Queue UI actions create a candidate with one asset and use the existing review/accept dialog. This was chosen over a second preview resolver or a new review service because the candidate already preserves the exact frozen plan, source revision, and atomic acceptance behavior.

2. **Bind cadence choices to candidate input.** Add optional `cadenceModeOverride` and `cadenceFieldOrder` values to the existing candidate job input. `preserve`, `auto`, and `inverse_telecine` are resolved by the existing canonical cadence authority; TFF/BFF are accepted only as explicit field-order choices. The backend overlays these values on an in-memory asset conversion override before the canonical freeze. They are serialized only with that candidate asset; they do not write Asset, Profile, or global configuration.

3. **Represent ambiguity with the existing review status.** When the frozen profile contains the canonical unresolved-IVTC warning and unknown effective FPS, the candidate asset is `requires_review`, with blocker text naming the missing explicit/validated order and the available TFF/BFF action. The candidate also exposes frozen cadence evidence: detected/recommended cadence, stream-reported field order, validated `SelectedOrder` and window coverage, effective FPS, and HEVC resolution. If a Preserve choice still cannot resolve FPS/HEVC, it remains `requires_review`. These checks run before runtime selection so unavailable encoder data cannot hide the cadence reason. No new status is introduced.

4. **Guard direct single Queue creation.** When a persisted scan exists, run the same canonical profile/cadence/HEVC preparation before `/api/queue/jobs` persists a job, using a video-only probe so unrelated track snapshots do not become a new Queue-intent requirement. If cadence remains unresolved, return structured `REVIEW_REQUIRED` details without creating a job, plan, reservation, or dispatch. Without a scan, no cadence/HEVC execution evidence exists yet, so keep the existing lightweight Queue intent and let claim perform canonical preparation and fail closed before ExecutionPlan dispatch. Explicit candidate cadence edits require Preflight. Audio-only Queue semantics remain unchanged; the user-facing single Queue controls use the reviewable candidate path.

5. **Revalidate and preserve the accepted plan.** Candidate edits advance the existing candidate revision; acceptance continues to revalidate source identity/revision and reservations transactionally. Accepted jobs and ExecutionPlans are produced by the existing Batch Preflight persistence path, and downstream Worker/Validation continue using their frozen snapshots.

6. **Keep asset settings appropriately scoped.** `FrameCadenceControls` may show field-order selection in `auto` only when the scan recommends IVTC and only when its caller opts in for an asset-scoped editor. Profile-wide and LAB controls keep their current visibility semantics. The candidate dialog offers Auto/TFF/BFF and Preserve cadence for the matching asset, and explains that a resolvable IVTC plan still needs visual review for combing, duplicates, and judder.

## Risks / Trade-offs

- [An old or edited candidate may lack the optional field] → Missing values retain existing automatic resolution; acceptance still requires a ready candidate.
- [Runtime or source data can change while review is open] → Existing candidate revision, source identity/revision, reservation, and runtime snapshot acceptance checks remain authoritative; stale candidates are rejected or must be resolved again.
- [Direct API callers do not receive the full review dialog] → The handler returns an actionable structured review response, while supported UI entry points open the existing Batch Preflight dialog.
- [Candidate resolution adds runtime snapshot and bounded source/destination checks] → It uses the same work as one-asset Batch Preflight and performs no new media analysis or FFmpeg work.

## Migration Plan

No SQLite migration is required. The new optional field is backward-compatible in existing candidate JSON. Rollback consists of reverting the code; existing QueueJobs, ExecutionPlans, and candidate rows remain readable and are not rewritten.
