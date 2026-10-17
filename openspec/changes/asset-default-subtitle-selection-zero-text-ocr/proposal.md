## Why

Queue must carry the effective Tracks decision through Video Profile
externalization so a fallback cannot add subtitle work that the canonical plan
did not authorize. A completed bitmap OCR attempt with no recognized text also
needs a bounded, actionable result that remains distinct from a proven stream
with no eligible cues and from OCR runtime/tool failures.

## What Changes

- Keep subtitle stream selection and requested sidecar formats owned by the
  frozen canonical Track plan when Video Profile externalization runs.
- Preserve zero-recognition evidence that is available from the OCR attempt,
  including stream identity, effective language, attempted modes, and observed
  bitmap counts, while identifying unavailable evidence as unknown.
- Keep required OCR artifacts as actionable failures when text is not recovered;
  do not fabricate an empty successful sidecar or reinterpret zero recognition
  as proof of an empty source stream.
- Add synthetic regressions for inherited Asset Defaults selection, explicit
  none/exclusions, and zero-recognition versus tool/runtime failure.

Out of scope: inspection or retry of production QueueJobs/media, OCR algorithm
changes, new subtitle-selection settings, schema migrations, and publication or
Quality Gate policy changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `asset-defaults`: frozen Track defaults remain authoritative through Video
  Profile externalization and excluded streams create no subtitle work.
- `subtitles`: zero-recognition diagnostics preserve known attempt evidence and
  remain distinct from no-cue and runtime/tool outcomes.

## Impact

- Backend subtitle planning and bitmap OCR error reporting under
  `backend/internal/handlers/`.
- Focused backend unit tests and the existing Asset Defaults and subtitles
  OpenSpec capabilities.
- No API or persisted model changes are intended; existing Queue snapshots and
  artifact/publication requirements remain authoritative.
