## Why

MVForge can normalize or preserve individual subtitle sources, but it cannot
produce one canonical ASS artifact from a primary subtitle and bounded
supplemental sources while preserving simultaneous dialogue, signs, songs,
styles, positioning, language safety, and provenance. A composition operation
must therefore build on the existing canonical OCR/text path, ASS structure
model, styling policy, Track Profile resolution, Asset Info eligibility, and
atomic subtitle publication boundaries rather than introducing another
subtitle pipeline.

## What Changes

- Add a backend-authoritative subtitle composition contract for one primary and
  one or more supplemental ASS/SSA, SRT, or OCR-derived canonical text sources.
- Resolve and freeze source identities/revisions, languages, source-track
  roles, bounded composition policy, style policy/version, output identity, and
  publication mode before work starts.
- Normalize all supported inputs into the existing shared ASS event model and
  reconcile them deterministically as an evolving canonical event set in
  primary-first, stable supplemental order, without concatenating overlapping
  events. Exact matches may be deduplicated, meaningful primary layout may be
  enriched, absent supplemental cues may be added, and ambiguous matches must
  become reviewable conflicts.
- Preserve native ASS/SSA styles, override tags, timing, positioning, and
  event independence by default; use the existing #123 style resolver only
  under the frozen style policy. Resolve source-style name collisions and
  reconcile required font attachments with bounded diagnostics.
- Extend `resolveTrackPlan` with a versioned bounded composition policy and
  freeze its resolved result into Queue snapshots, including primary and
  supplemental selection, language/role rules, preferred/default behavior,
  styling, and explicit overrides.
- Enforce that preferred/default promotion requires both the configured
  primary/full-dialogue source and an explicit promotion request in the frozen
  profile/manual policy; no manual exception can waive the source requirement.
- Add backend-authoritative Asset Info `Compose subtitles` eligibility and
  manual-request contracts using the existing subtitle operation registry,
  cancellation/retry lifecycle, staging, `subtitlePublicationMu`, and atomic
  publication barrier.
- Publish one deterministic canonical `.ass` identity for an ordered or
  canonicalized source set, making retries idempotent and rejecting stale
  sources without modifying source media or source sidecars.
- Record bounded provenance for source tracks/sidecars, resolved languages and
  roles, reconciliation decisions, conflicts, attachments, and output
  identity/path.
- Keep `SubtitleSemanticRole` backward compatible and introduce a separate
  bounded composition classification for cue/style intent, including lyrics;
  compound track labels such as `Signs & Songs` are not semantic cue roles.

Out of scope for this change: production implementation in this planning
phase; a second ASS parser, cue model, serializer, styling system, OCR path,
operation lifecycle, or publication system; automatic unsafe cross-language
composition; full cue-body provenance; video muxing; and arbitrary new
sidecar discovery beyond sources already authorized by the canonical pipeline
and explicit manual selection.

## Capabilities

### New Capabilities

None. Composition extends the existing subtitle capability and must remain
visible through its canonical contracts.

### Modified Capabilities

- `subtitles`: add canonical multi-source composition, deterministic
  reconciliation, language/role safety, Track Profile and Asset Info contracts,
  frozen lifecycle state, atomic canonical ASS publication, bounded
  provenance, and idempotent output identity.

## Impact

- Backend subtitle planning and domain types: `ResolvedTrackPlan`, subtitle
  source/sidecar metadata, Track Profile snapshots, and Queue preparation.
- Existing ASS/SSA structural parsing and reconstruction in
  `translation_ass_structure.go`, including `assDocument`, `assEvent`, and
  `assProtectedDialogue`.
- Existing #26 canonical bitmap/OCR positioning and #123
  `SubtitleStylePolicy`/`resolveSubtitleStyle` boundaries.
- Asset Info subtitle eligibility, manual operation/request endpoints, and the
  shared subtitle operation cancellation, retry, staging, validation, and
  publication code.
- Persisted bounded provenance and canonical output identity; existing sources,
  sidecars, and non-composition subtitle behavior remain backward compatible.
- Focused backend, lifecycle, parser/reconciliation, policy-snapshot, and
  Asset Info tests will be added during implementation. This SDD phase changes
  no production code or tests.
