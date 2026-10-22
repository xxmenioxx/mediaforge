## Context

See proposal.md for motivation. Batch Preflight persists candidate inputs and resolved QueueJob snapshots in revisioned JSON state. The resolved job already carries `TrackProfileSnapshot`, including the canonical `ResolvedTrackPlan`; the current dialog reduces that to a generic “Frozen track selection and subtitle policy” label. The subtitle resolver already owns disposition, stream role, sidecar formats, OCR requirements, and artifact planning.

Constraints: Preflight must not inspect mutable Settings to infer actions, run media probes or OCR, write Asset Tracks overrides, or change already accepted jobs. Candidate acceptance must continue to consume its stored snapshot.

## Goals / Non-Goals

**Goals:**

- Project canonical subtitle decisions into a versioned, compact API shape.
- Keep per-stream action and sidecar/OCR details aligned with the existing frozen track plan.
- Carry explicit Queue-only edits through candidate revisions and the canonical resolver.
- Reject stale stream identities and preserve explicit Asset Tracks and matching higher-priority subtitle rules.

**Non-Goals:**

- A second subtitle resolver or frontend policy engine.
- Settings, Track Profile, Asset Defaults, or `assetConversionOverrides` mutations.
- A database migration or changes to subtitle generation, OCR, or publication engines.

## Decisions

### Project the frozen canonical plan

Add a versioned subtitle projection to each Queue Preflight plan response. Build it from the `resolvedTrackPlan` already held in `TrackProfileSnapshot`; do not re-resolve from current settings for rendering. Extend canonical per-stream plan entries with available source title/default metadata, action source, and whether a Queue override is compatible. Return ordered source-stream decisions, resolved sidecar outputs, summary counts, policy provenance, and an explicit state for missing evidence, no source streams, or no selected subtitle work. Sidecar counts remain source-stream counts even when one stream produces both SRT and ASS.

The resolver records whether each action came from a matching subtitle rule, preferred subtitle rule, default action, explicit Asset Tracks exclusion, or candidate override. The effective track assignment remains available from the existing profile-resolution snapshot. This keeps both action provenance and scope provenance attached to backend-owned evidence.

### Keep candidate edits inside candidate revision state

Add an optional per-stream action override to the existing Queue Preflight job input. Entries include the source stream index and exact canonical stream signature; omitting an entry means inherit. The current source revision and signature must both validate before applying an override. Never remap by language. The candidate input is stored only in QueueBatchPreflight JSON state; no `assetConversionOverrides`, Asset Defaults, or Track Profile write occurs.

During resolution, copy the frozen Track Profile snapshot, attach the candidate override to the canonical planner input, and resolve the effective track plan again. Overrides may change only streams the canonical planner marks eligible: an explicit Asset Tracks exclusion or a matching higher-priority subtitle rule remains authoritative. If an edit is stale or conflicts with one of those constraints, reject the candidate update with a conflict and require the operator to re-open and re-resolve. Omitted/removed entries restore inherited policy.

### Preserve the accepted snapshot boundary

Store the resolved projection and candidate action provenance with the accepted Queue track snapshot. Existing Worker and downstream subtitle-artifact, Validation, and Publisher consumers continue reading that frozen snapshot. A later setting or Asset edit cannot change it. Old snapshots without the projection remain valid and are not rewritten or synthetically re-resolved.

### Keep the frontend a renderer/editor of candidate intent

The default asset view shows backend-provided counts and policy warnings. A collapsed detail section renders one row per source stream, including planned representations and OCR. The Queue-only editor starts with every row inherited, offers an explicit action per eligible stream, shows before/after changes, and submits only changed identities. Canceling local edits drops local state; saving advances and resolves the candidate revision. The UI does not infer selection from preferred languages or derive disposition from current Settings.

No additional media scans, OCR, FFmpeg work, or per-asset API requests are introduced; the projection is included in the existing candidate response.

## Risks / Trade-offs

- [A candidate is reopened after its source or scan changes] → Bind each edit to source revision, stream index, and canonical stream signature; reject stale updates without language-based remapping.
- [A candidate edit could otherwise bypass a saved Asset Tracks subset or a more-specific rule] → Have the canonical planner expose edit eligibility and enforce it again when resolving the request.
- [Old snapshots lack newly projected fields] → Represent unavailable evidence explicitly and leave historical snapshot contents unchanged.
- [Large assets have many subtitle streams] → Keep the summary compact and render the full matrix only on demand with a bounded scrolling region.

## Migration Plan

No SQL migration is required. Candidate state and Queue track snapshots accept additive JSON fields; older candidates and jobs continue using their existing resolved snapshots. Rollback removes the new presentation/edit fields while preserving the already stored canonical `ResolvedTrackPlan` data.
