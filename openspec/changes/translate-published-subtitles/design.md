## Context

See `proposal.md` and the modified `subtitles` requirements. Asset Info currently combines its general `rowLocked` state with the lifecycle `manageSubtitles` capability when rendering Translate actions. The existing translation dialog already asks the backend about the exact source and provider state, while the backend source resolver enforces inventory ownership, Library roots, archive exclusion, readability, and source revision.

## Goals / Non-Goals

**Goals:** Allow eligible subtitle translation on published Library lifecycle variants; retain the canonical lifecycle blocker for active Queue/maintenance work; explain source and provider blockers at the layer that owns them.

**Non-Goals:** Change profile/Queue or adjacent subtitle mutation permissions, add archive access, alter supported subtitle representations, or change translation execution/publication.

## Decisions

- Use the backend `manageSubtitles` lifecycle action as the outer action guard. Remove only `rowLocked` from translation button state; leave it and the lifecycle capability on all adjacent editing and destructive actions. Pass a human-readable reason to disabled translation controls.
- Revalidate the canonical subtitle lifecycle action in the shared translation source resolver used by capability and start. Reuse the existing lifecycle projection and exact source resolver rather than adding a translation-specific active-work model. Map denied lifecycle states to a bounded actionable reason; source ownership, archive exclusion, representation eligibility, and source revision continue through their existing checks.
- Keep `ResolveAssetActionCapabilities.Subtitles` unchanged for generic subtitle operations: a `PublicationConflict` does not independently block that capability. Add a Translation-only provenance check after exact-source resolution. Accept recognized `accepted` Library assets without publication rows, or a current Queue/Direct publication with one authority, exact Library path/asset identity, and a resolved raw-source correlation. Reject conflict, legacy/unknown provenance, missing authority IDs, mismatched published paths, and unresolved/multiple raw-source references.
- Replace Translation's full `assetLifecycleProjections` table loads with a scoped loader that feeds the same `ResolveAssetLifecycleProjection` function. It reads the selected Library, QueueJobs and DirectPublications linked by `SourceAssetRecordID` or the selected destination path, active maintenance linked by asset ID/path, and only source AssetRecords referenced by matching publication rows. It also examines a capped set of same-Library, same-basename publication paths so Go's existing `cleanAssetLifecyclePath` normalization still sees non-identical strings such as `dir/../asset.mkv`. These queries use the existing source-ID, path, Library, and maintenance indexes; more than 256 candidates of one authority fail closed for lifecycle review. It does not load ScanResults, review overrides, or analysis policy because Translation's lifecycle action decision does not consume them. Batch Asset inventory continues to use its existing full projection path.
- The current publication code already calls `resolveTranslationExecutionSource` again inside its publication barrier. Because that function uses the same Translation guard, lifecycle identity/provenance changes between Start and publication prevent the final sidecar rename. Keep source revision revalidation and atomic publication unchanged.
- Keep provider eligibility in the current capability response. Opening the dialog queries the exact selected embedded stream or external path; the dialog displays the returned reason and keeps Start disabled when the source is ineligible or the provider is unavailable. No client-side provider probing or source substitution is added.
- Preserve the current operation runner and atomic locale-qualified sidecar publication. No database migration or API shape change is needed.

## Risks / Trade-offs

- Rechecking a lifecycle projection during explicit capability and start requests reads shared lifecycle state. This is limited to an operator-selected source and uses existing projection logic; it adds no asset polling or per-row API requests.
- Lifecycle state can change between dialog load, Start, provider execution, and publication. Start repeats the guard and exact source/revision resolution; the publication barrier repeats lifecycle and source revision checks so stale capability data cannot authorize publication.
- Historical full scans previously returned path-equivalent publications after `cleanAssetLifecyclePath` normalization. The scoped path candidate query preserves this equivalence by checking indexed Library and bounded basename candidates before the canonical projection filters paths in Go. If the bounded candidate set is too large to inspect safely, Translation fails closed. Publication rows linked by source AssetRecord identity remain discoverable independently of path spelling. Legacy published assets without a complete current identity fail closed.
- Disabled controls need to keep their explanation reachable despite native disabled-button pointer behavior. Wrap only the Translate control for tooltip access; do not alter adjacent buttons.

## Migration Plan

No persisted data, API schema, or deployment migration is required. Reverting the application change restores the prior UI and source guard behavior without data cleanup.
