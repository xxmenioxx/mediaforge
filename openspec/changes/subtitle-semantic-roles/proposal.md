## Why

Subtitle streams are currently identified and selected primarily by stream index,
language, and existing dispositions. That is insufficient when an asset contains
multiple same-language tracks whose intended use differs, such as full subtitles,
forced dialogue, signs, commentary, or SDH. GitHub issue #37 adds a canonical,
player-agnostic semantic role while preserving the existing stream identity and
canonical subtitle pipeline.

## What Changes

- Add canonical subtitle roles: `full`, `forced`, `signs`, `commentary`, `sdh`,
  and `unknown`.
- Resolve each subtitle stream's role independently from language and existing
  dispositions, retaining stream index as the authoritative track identity.
- Give explicit stream metadata/dispositions precedence over bounded heuristics;
  ambiguous or insufficient evidence resolves safely to `unknown`.
- Add authoritative manual per-asset/per-stream role overrides with reset-to-auto
  behavior, and expose role, provenance, and confidence in Asset Info.
- Extend Track Profiles with optional role filtering while keeping existing
  language-only profiles backward compatible.
- Reuse `resolveTrackPlan`, freeze resolved roles in `ResolvedTrackPlan` and Queue
  snapshots, and require Workers to consume those frozen decisions without
  reclassification.
- Carry role metadata through subtitle artifacts, sidecars, and publishing;
  supported `forced` and `sdh` conventions may be mapped at publication boundaries
  without making the canonical model player-specific.
- Keep classification bounded and metadata-based; role resolution MUST NOT invoke
  OCR or introduce a new subtitle planner, job system, or dashboard.

## Capabilities

### New Capabilities

None. Semantic roles are a requirement extension to the existing subtitle
capability.

### Modified Capabilities

- `subtitles`: add canonical semantic role resolution, manual overrides, optional
  role-aware Track Profile filtering, Queue freezing, Asset Info observability,
  and role-aware artifact/publishing metadata while preserving existing subtitle
  conversion, OCR, geometry, native ASS/SSA, and publication semantics.

## Impact

- Backend track metadata models, `resolveTrackPlan`, profile matching, asset
  snapshots, Queue snapshot freezing, Worker consumption, subtitle artifact
  metadata, and publishing conventions.
- Asset Info and Track Profile UI/API contracts for role display, provenance,
  confidence, manual override, reset-to-auto, and optional role filters.
- Existing language-only profiles and existing subtitle sidecar behavior remain
  compatible; role filtering is opt-in and `unknown` remains a safe result when
  classification cannot be justified.

Out of scope: Direct Play policy (#8), translation, OCR quality repair, regional
Spanish inference, a new OCR/planner/job system, and the Subtitles dashboard.
