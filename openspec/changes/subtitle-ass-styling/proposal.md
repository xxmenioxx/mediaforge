## Why

MVForge can already preserve native ASS/SSA styling and can position OCR-derived
ASS cues from canonical bitmap geometry, but generated ASS appearance is not
configurable and style selection is not represented as a reusable policy. Users
need predictable colors, role-aware defaults, and safe Asset Info restyling
without losing source structure, rerunning OCR unnecessarily, or changing SRT.

## What Changes

- Add a versioned, backend-validated subtitle styling setting in the existing
  AppSetting infrastructure.
- Define immutable stable MVForge preset IDs (`MVF_Default`, `MVF_Top`,
  `MVF_Lyrics`, and `MVF_Sign`) plus collision-safe custom preset IDs.
- Add a canonical resolver that selects a configured style from canonical role
  and layout evidence while keeping role, style, alignment, and exact position
  as separate data.
- Apply configured styling to generated SRT-to-ASS and bitmap-OCR-to-ASS
  output through the existing canonical cue and ASS rendering paths.
- Preserve native ASS/SSA style definitions, event references, karaoke, inline
  tags, positioning, margins, and layers by default; make explicit restyling
  opt-in and structurally conservative.
- Add normalized HEX color validation, ASS alpha/BGR conversion, preset palette,
  bounded saved/recent color state, and a browser-only Settings preview.
- Add an Asset Info single-sidecar Restyle / Regenerate operation that captures
  exact source identity, reuses existing OCR and operation/publication lifecycle
  components, and publishes non-destructively by default.
- Freeze the normalized style policy/version when an operation starts or Queue
  execution is frozen so later Settings edits affect only future work.

Out of scope: subtitle composition for #131, batch restyling, AI style choice,
new OCR or ASS serializers, translation behavior changes, SRT styling,
player-specific guarantees, font downloading, and FFmpeg rendering for the
Settings preview.

## Capabilities

### New Capabilities

- None. This change extends the existing subtitle capability.

### Modified Capabilities

- `subtitles`: add configurable generated ASS styling, source-style preservation
  and explicit restyle/regenerate behavior while preserving existing canonical
  conversion, OCR, SRT, cancellation, and publication contracts.

## Impact

- Backend subtitle settings validation/resolution, canonical ASS cue rendering,
  native ASS/SSA structure handling, Asset Info subtitle operations, artifact
  provenance, and staged sidecar publication.
- Settings and Asset Info React surfaces, with backend-authoritative eligibility
  and operation state.
- No new database/table, OCR engine, provider, public subtitle cue model, or
  font-attachment subsystem. Existing `AppSetting`, canonical OCR evidence,
  subtitle operation registry, staging, validation, cancellation, and atomic
  publication remain authoritative.
