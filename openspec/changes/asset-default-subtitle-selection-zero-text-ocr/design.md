## Context

See proposal.md and the delta specs for the observable contracts. The Queue
path freezes a `ResolvedTrackPlan` when a Tracks configuration is present. The
Video Profile fallback can also build subtitle transforms from the legacy
`KeepSubtitleStreams` shape; when both a resolved plan and legacy transforms are
absent, its current selector treats an omitted list as all source streams.
Bitmap OCR already retains bounded SeConv diagnostics in the canonical OCR
failure, while the PGS geometry parser currently returns only geometry and
cannot distinguish a successfully parsed zero-event input from unavailable
evidence.

## Goals / Non-Goals

**Goals:**

- Keep Video Profile externalization within a frozen canonical plan or an
  explicit stream-index selection. An omitted selection alone must not expand
  a video-only request into work for every source subtitle.
- Add evidence-aware PGS cue counts using the existing bounded SUP parser.
- Report a proven no-cue result separately from OCR zero recognition and runtime
  or tool failure, without creating an empty artifact.
- Preserve one canonical OCR lifecycle for an authorized logical SRT/ASS `both`
  result.

**Non-Goals:**

- Add a second subtitle resolver, alter language normalization or OCR
  recognition algorithms, or change Japanese fallback limits.
- Reclassify malformed or unparsed SUP data as empty; unknown evidence continues
  through the existing OCR path and failure classification.
- Add optional Queue sidecar semantics, change Quality Gate or publication
  requirements, mutate historical Queue snapshots, or inspect production media.

## Decisions

### Keep Video Profile fallback behind explicit track authority

The canonical Track plan remains the authority whenever it is available.
Otherwise, profile externalization may use a non-nil explicit stream-index list;
an omitted list is inheritance and is not enough evidence to select every
subtitle. Preserve the current early exits for selected Track Profiles and
existing explicit transforms. The FFmpeg and worker paths continue consuming
the same frozen plan/override, without rereading mutable Asset state.

**Alternative considered:** keep selecting all streams when the list is nil and
filter only after artifacts are planned. Rejected because extraction/OCR and
pending artifacts would already have been scheduled for streams outside the
effective selection.

### Count PGS evidence during the existing bounded parse

Extend the SUP parser to report presentation-composition events, bitmap object
references, and geometry-backed cue records alongside its existing geometry
result. A valid parse with no eligible object references is known zero-cue
evidence. Empty demux output is also a known zero-event result. Non-empty
malformed input, file-read errors, or object references without sufficient
geometry remain unknown and must not be interpreted as no cues.

For a required planned artifact with known zero cues, terminate before
Tesseract/SeConv and return an actionable no-eligible-cues failure. Existing
Queue sidecars remain required by their plan/Validation contract; no successful
empty SRT/ASS file is fabricated.

For a segmented Test Encode whose plan sets `AllowEmptySubtitleArtifacts`, the
same proven zero-cue result omits the optional sidecar and records a terminal
`skipped` progress update for Queue artifact snapshots. The Test Encode can
continue without a sidecar. This does not make full Queue sidecars optional and
does not apply to zero-recognition or unknown/malformed PGS evidence.

**Alternative considered:** treat any empty geometry result as no cues. Rejected
because the current parser can return no geometry when source evidence is
incomplete or malformed.

### Preserve zero-recognition attempt evidence

When PGS has eligible bitmap evidence but OCR recognizes no text, keep the
existing canonical failure path and append a bounded diagnostic containing the
stream index, requested/effective or attempted languages, OCR mode, available
PGS and SeConv image counts, and the reason no artifact was produced. Say
explicitly when source cue or glyph presence is unknown. Runtime/language/tool
errors retain their current distinct classifications. The OCR evidence cache
continues to commit only successful evidence, and an authorized `both` request
continues to share one OCR result.

**Alternative considered:** convert zero-recognition to a successful empty
artifact or skip all such failures. Rejected because a zero-recognition signal
does not prove the source is empty and Queue requires planned sidecars.

## Risks / Trade-offs

- **Risk:** legacy video-only jobs without a frozen selection stop synthesizing
  all subtitle sidecars. → New Queue planning continues to freeze configured
  Asset Defaults; direct fallback remains fail-closed until it receives explicit
  stream authority.
- **Risk:** PGS event counts can be mistaken for recognized text or visual
  glyphs. → Report each count by its actual meaning and never infer glyph
  presence from event/object counts.
- **Runtime impact:** the SUP evidence pass is linear over one already extracted
  PGS stream and avoids OCR startup when a valid parse proves there are no
  eligible cues; no source re-probe or duplicate OCR is added.
- **Compatibility:** no model or API migration is needed. Existing frozen Queue
  plans and legacy snapshots remain unchanged; only newly prepared plans use the
  stricter fallback boundary.

## Migration Plan

No database migration or historical job rewrite is required. Rollback is a
code revert; existing Queue snapshots, staged artifacts, and published outputs
remain unchanged.
