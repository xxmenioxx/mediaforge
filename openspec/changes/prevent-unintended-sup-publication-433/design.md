## Context

See proposal.md for motivation. The canonical Track Plan already freezes each
selected subtitle's format, mode, settings source, and stream index. Queue
Preflight projects those sidecar decisions. Worker artifacts carry an artifact
ID, stream index, format, and mode. Publisher currently builds its manifest
from every ready artifact and uses the plan match only for `Required`; it still
sets `Promote` for unmatched artifacts.

## Goals / Non-Goals

**Goals:**

- Make the frozen per-stream sidecar decision the publication authorization
  boundary.
- Bind newly generated artifacts to the source stream signature when present,
  while reading older snapshots through their existing index and stable
  artifact identity.
- Show original/converted mode and the frozen format source in Preflight.
- Preserve original PGS and native text extraction and legacy text sidecar
  handling.

**Non-Goals:**

- Changing the resolver's existing original-format default or OCR/no-cue rules.
- Rewriting accepted Queue snapshots or migrating existing Library files.
- Adding a second subtitle planner or publication mechanism.

## Decisions

### Keep format authorization in the canonical Track Plan

Worker and Publisher consume the frozen `SidecarOutputs`; Publisher does not
infer policy from UI text, sidecar filename, or a ready status. Manifest
construction filters generated subtitle entries against the exact stream
index, artifact ID, format, mode, and eligible extraction disposition. A
source-stream signature is carried into new artifact records and compared when
present. Older plans without that artifact field remain compatible through the
existing stream index and stable `subtitle:<index>:<mode>:<format>` identity.

The alternative of changing the resolver default away from `original` was
rejected: it would alter supported native extraction and would not prevent a
stale artifact from being promoted.

### Keep OCR temporary artifacts inside the existing operation

Converted PGS continues through the canonical bitmap OCR path. Only its
requested text artifacts enter the job artifact list. The `.sup` extracted for
OCR remains an internal temporary owned by that operation; no Publisher
exception or extension-based fallback is introduced.

### Preserve legacy boundaries

Historical jobs without a resolved plan keep existing operation-artifact
compatibility, subject to explicit artifact mode, format, and stable identity
for native `.sup`. Legacy external subtitle discovery remains limited to the
existing supported SRT/ASS/SSA formats; it does not discover `.sup`. No
historical snapshot is reconstructed from current Asset state.

### Render existing provenance in Preflight

The backend's `ResolvedTrackSidecar.SettingsSources["format"]` and `Mode` are
already frozen in the Preflight projection. Extend the frontend response type
and render a clear original/converted label plus that source. Keep all policy
decisions in the backend.

## Risks / Trade-offs

- [Older artifacts lack a source-stream signature] → Match their frozen plan by
  stream index, exact format and mode, and stable artifact ID; compare the
  signature whenever the artifact contains it.
- [Strict filtering could omit an optional or historical output] → Keep
  required/optional validation unchanged and retain explicit legacy operation
  artifacts; add tests for optional skip and legacy external text behavior.
- [A Preflight source is unknown to the current frontend] → Display a safe
  human-readable fallback instead of hiding the backend-provided source.

## Migration Plan

No database migration is required. New artifact JSON adds an optional
source-stream signature. Existing snapshots and artifacts remain readable;
rollback restores the prior manifest behavior without changing persisted
snapshots or files.
