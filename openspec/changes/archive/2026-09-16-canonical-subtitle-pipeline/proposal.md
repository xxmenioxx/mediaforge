## Why

Subtitle extraction and conversion initiated from Asset Info can diverge from
the canonical Pipeline behavior. This produces inconsistent ASS/SRT artifacts,
especially when preserving ASS/SSA streams or handling bitmap subtitles, and
can cause duplicate or incorrectly selected sidecars. Issue #30 establishes a
single observable contract so all entry points agree on subtitle outputs.

## What Changes

- Define one canonical subtitle conversion decision path in Pipeline and reuse
  it from Asset Info, Track Profiles, Profile Lab, and Queue/Workers.
- Preserve existing SRT output behavior and lossless ASS/SSA extraction when an
  original representation is requested.
- Resolve ASS/SRT output selection from the canonical track/output plan rather
  than Asset Info-specific heuristics.
- Keep bitmap subtitle handling explicit: OCR is used only where the canonical
  Pipeline path supports it; unsupported conversions report a clear failure.
- Record and expose the same artifact format, mode, naming, and status from
  Asset Info, Track Profiles, Profile Lab, Pipeline, and Queue/Worker
  operations.
- Reuse only sidecar artifacts already identified by the existing canonical
  pipeline; do not add new discovery or matching semantics.
- Add focused regression coverage for text extraction, output precedence, and
  bitmap/OCR boundaries.
- Out of scope: video conversion, Track Profile redesign, unrelated Asset Info
  actions, and changes to media storage or publication semantics beyond subtitle
  artifacts.

## Capabilities

### New Capabilities

- `subtitles`: Canonical subtitle extraction, conversion, output selection, and
  artifact reporting shared by Pipeline, Asset Info, Track Profiles, Profile
  Lab, and Queue/Workers.

### Modified Capabilities

- None.

## Impact

- Backend subtitle planning, artifact generation, and Asset Info handlers will
  reuse the canonical Pipeline resolver and artifact model.
- Existing API responses may gain consistent artifact metadata, while existing
  SRT consumers remain compatible.
- Focused backend and frontend tests will exercise parity across Asset Info,
  Track Profiles, Profile Lab, and Pipeline without requiring changes to source
  media or external services.
