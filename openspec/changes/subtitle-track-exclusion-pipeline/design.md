## Context

See `proposal.md` and `specs/subtitles/spec.md` for the required behavior. The existing asset Tracks PATCH preserves an omitted value separately from an explicit empty list. Queue captures asset conversion overrides in the frozen profile snapshot, and canonical Track Profile jobs rebuild `ResolvedTrackPlan` during freeze. At execution, `buildMediaJobPlanWithOverride` applies automatic Video Profile externalization; currently that helper walks every source subtitle and clears `KeepSubtitleStreams`. The legacy artifact generator then iterates saved transforms directly. The resolved-plan path already separates stream dispositions from sidecar outputs, and Publisher consumes the job's generated artifacts.

Constraints: do not change persisted/API shapes, re-resolve mutable asset state after freeze, alter legacy frozen snapshots, create a second track resolver, or change unrelated media domains.

## Goals / Non-Goals

**Goals:**

- Keep the effective asset selection intact from Queue Preflight through the frozen override and Worker plan.
- Make an explicit selected-index set constrain the existing canonical resolver before automatic subtitle rules can act on excluded indexes.
- Use the same effective selection before automatic externalization and before the legacy transform loop performs any extraction/OCR/conversion work.
- Make Worker stage and artifact reporting reflect only authorized subtitle and independent font work.
- Prove excluded indexes never reach subtitle operation boundaries and never appear in FFmpeg mappings or published artifacts.

**Non-Goals:**

- Change the Asset Info Save Tracks API or user interface.
- Change Video Profile policy, OCR semantics, subtitle conversion formats, or Publisher's general sidecar ownership rules.
- Create a new resolver, persisted selection field, database migration, or alternate subtitle pipeline.

## Decisions

### Keep the persisted selection as the sole selection input

Continue to use `AssetConversionOverrideState.KeepSubtitleStreams` and the existing frozen `assetConversionOverride` snapshot. Preserve its three states: `nil` inherits, an empty slice selects none, and a non-empty slice selects only its indexes. Do not clear this field when Video Profile externalization creates transforms. This keeps Queue acceptance, restart, and retry bound to the same saved values without changing storage.

**Alternative considered:** copy a second selected-index list into subtitle transforms or artifact records. Rejected because it would create parallel selection state that could diverge from the frozen override.

### Apply exclusions inside the existing canonical resolver

When Queue freezes an effective Track Profile plan, the explicit effective asset selection will mark every unselected source subtitle as removed before language/role rules and operation overrides can assign work. The resolved plan remains the authority for canonical sidecar decisions, embedded mapping, validation, and publication. Jobs without a Track Profile continue to use the frozen override selection with the existing non-canonical transform path.

**Alternative considered:** filter a resolved plan after resolution or add a Queue-only subtitle resolver. Rejected because post-filtering can leave sidecars/defaults inconsistent, while a second resolver would split behavior among Queue and other callers.

### Filter work before automatic externalization and before execution

`applyProfileSubtitleExternalization` will derive candidate streams from `selectedSubtitleStreams(plan)`, which already represents the canonical-plan and override paths. It will generate transforms only for those candidates. The Worker will ignore excluded indexes before its legacy per-transform loop and will base the `Preparing Subtitles` decision on authorized transforms, resolved sidecars, and independently authorized font exports. Canonical plan dispositions will prevent a stale legacy transform from overriding an explicit `remove`.

**Alternative considered:** generate all transforms and remove excluded artifacts later. Rejected because extraction, OCR, FFmpeg calls, staging, and progress would already have occurred for the excluded streams.

### Preserve downstream snapshot and publication boundaries

Queue Preflight and accepted jobs keep their existing snapshot lifecycle. The Worker generates only selected `SubtitleArtifacts`; existing validation consumes those artifacts and the converted output, and Publisher publishes only ready artifact entries. The implementation will avoid a new Publisher resolver and will change its input only if a focused regression demonstrates a downstream fallback can reintroduce an excluded embedded stream.

**Alternative considered:** independently re-read current asset selections in Validation or Publisher. Rejected because it would allow mutable state to change a frozen job and could disagree with FFmpeg output.

### Keep legacy snapshots readable

Snapshots that lack a canonical resolved plan continue through the established transform fallback. A missing selection remains inheritance; an explicit selection already frozen in the job constrains that fallback. No existing QueueJob, ExecutionPlan, or result is rewritten, and no schema migration is needed.

**Alternative considered:** backfill old snapshots from current asset state. Rejected because that would retroactively mutate accepted execution decisions and could select a different stream after source changes.

## Risks / Trade-offs

- **Risk:** a selected-index check could confuse same-language tracks or stream numbering. → Compare canonical stream indexes only and test duplicate languages.
- **Risk:** stale transform metadata could still trigger a Worker stage or artifact count. → Filter before stage selection and assert no pending artifact or operation for excluded indexes.
- **Risk:** a legacy snapshot may not contain enough data to prove a prior selection. → Preserve its stored behavior; do not infer a selection from current mutable asset data.
- **Runtime impact:** filtering is an in-memory pass over the already-probed subtitle inventory and does not add media probes, FFmpeg calls, OCR work, or database reads.

## Migration Plan

No database, API, or snapshot migration is required. Newly prepared jobs use the corrected selection flow. Already frozen snapshots remain unchanged; retries consume their captured override/plan, and legacy snapshots without a selection retain inherited behavior. Rollback is a code revert; it does not require data cleanup.
