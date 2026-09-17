## Context

See proposal.md for the issue motivation and scope. The existing canonical
subtitle pipeline already:

- classifies PGS and VobSub as bitmap inputs;
- extracts PGS to an operation-owned SUP with an explicit FFmpeg stream map;
- runs SeConv/Tesseract OCR and applies the selected cleanup mode;
- validates subtitle output and fans one recognized result out to SRT, ASS, or
  logical both; and
- preserves original ASS and SSA through the lossless extraction path.

The current FFProbeStream model has stream-level width and height but no
per-cue bitmap coordinates. The current SeConv argument and output contract
also does not expose a documented cue-geometry field. The design therefore
must not infer cue placement from stream dimensions, undocumented ffprobe
properties, SeConv diagnostics, or a guessed command-line option.

## Goals / Non-Goals

**Goals:**

- Produce a reusable canonical bitmap cue result containing text, timing, and
  optional verified spatial metadata.
- Preserve independent bitmap cue events, including simultaneous cues in
  different regions.
- Generate deterministic coarse ASS alignment for OCR-derived ASS while
  leaving SRT text/timing serialization unchanged.
- Keep the existing canonical Asset Info, Queue, Profile Lab, and other
  subtitle paths on the same recognition and fan-out model.
- Make missing or ambiguous geometry a successful, safe bottom-center fallback.

**Non-Goals:**

- Exact per-pixel coordinates, \pos(x,y), perspective correction, or
  reconstruction of original font styling.
- Adding undocumented ffprobe/SeConv geometry fields or relying on a runtime
  capability that has not been probed or validated.
- Changing OCR language, recognition, cleanup policy, sidecar discovery,
  artifact identity, operation ownership, native ASS/SSA extraction, or SRT
  semantics.

## Decisions

### 1. Keep geometry in the canonical cue model, not in SRT

Introduce an internal canonical cue representation with the existing text and
timing plus optional geometry:

    CanonicalSubtitleCue {
      start
      end
      text
      sourceOrdinal
      geometry: optional BitmapCueGeometry
    }

    BitmapCueGeometry {
      canvasWidth
      canvasHeight
      bitmapX
      bitmapY
      bitmapWidth
      bitmapHeight
      sourceOrdinal
    }

The geometry object is optional and is not serialized into SRT. SRT continues
to be generated from only cue text and timing. ASS receives the same canonical
cues and may consume the optional geometry. This keeps SRT stable and prevents
ASS-only metadata from leaking into the text interchange format.

The model is an intermediate canonical result, not a new job system or a
second subtitle pipeline. Existing OCR, normalization, validation, cleanup,
publication, and operation ownership remain the surrounding stages.

Geometry is authoritative on this canonical cue object, not on an ASS file.
The cleanup adapter must preserve cue identity and carry its geometry alongside
any cleaned text, line-break, or timing changes. SeConv must receive the
geometry-free cleanup representation; serialized ASS alignment tags are never
the source of truth and their survival in SeConv output is not required.

### 2. Obtain geometry from available bitmap representations

Geometry acquisition is a best-effort provider selected by source type and
verified input availability:

- For PGS, reuse the SUP already produced by the existing explicit FFmpeg
  extraction. Parse the PGS presentation-composition and object-definition
  segments from that file. Presentation composition supplies the canvas and
  object composition supplies bitmap placement; object-definition data supplies
  bitmap dimensions. Composition timing and stable segment/object order become
  the source event identity. The parser must reject incomplete or inconsistent
  records rather than manufacture coordinates.
- For VobSub, use geometry only when an authorized, parseable VobSub source
  representation is already available, such as a companion IDX/SUB pair.
  Parse the representation's verified canvas, event timing, and subtitle
  packet bounds. The provider must not assume that the current FFProbeStream
  fields or direct SeConv input contain cue coordinates.
- If an embedded or runtime-specific VobSub representation cannot be obtained
  through a supported existing capability, report geometry unavailable and
  continue OCR with the bottom-center fallback. Do not add an unverified
  FFmpeg VobSub muxer dependency merely to force geometry extraction.

The providers run once per bitmap conversion and reuse the operation-owned
prepared input/workspace. They do not invoke Tesseract, SeConv, or a second
OCR pass. Parser limits on event count, segment size, and retained diagnostics
must prevent malformed subtitle data from causing unbounded memory or work.

#### Operation-local timing normalization

Before correlation, source bitmap events and OCR cues are converted to one
operation-local timeline whose zero is the start of the prepared input. For a
full conversion this is the source timeline. For a segmented or sampled
conversion, source event timestamps and geometry are shifted by the same
non-zero segment/sample start used to prepare the input, while OCR timestamps
are interpreted relative to that prepared input. Matching never compares an
absolute source timestamp with an unshifted sample timestamp.

The normalized timeline is used only for geometry correlation. Existing output
timing and any later offset restoration remain governed by the canonical
subtitle pipeline, so this change does not alter published cue timing.

Alternative: request coordinates from ffprobe or add SeConv geometry flags.
Rejected because neither contract is present in the repository/runtime and
would make layout depend on unsupported behavior.

### 3. Correlate recognized cues deterministically and one-to-one

The OCR result is normalized into canonical cues before output fan-out. Each
source bitmap event and recognized OCR cue first receives a start/end interval
in the operation-local prepared-input timeline described above. Correlation
uses the following ordered rules:

1. Prefer a unique recognized cue whose normalized start/end interval matches
   or overlaps the source event within the existing subtitle timing precision.
2. For simultaneous or otherwise equal timing candidates, match in stable
   source-event and recognized-cue order.
3. Assign geometry only when the candidate is unique under those rules;
   otherwise leave that cue unpositioned.

The resolver never groups cues solely because timestamps overlap and never
reuses one source event's geometry for multiple unrelated recognized cues.
Unmatched recognized cues remain valid text/timing cues and use the fallback.
This makes simultaneous top and bottom cues remain separate even when OCR
returns identical times.

Alternative: correlate only by cue index. Rejected because OCR can omit,
split, or reorder cues. Alternative: merge overlapping cues before matching.
Rejected because it loses independent signs, lyrics, and dialogue.

### 4. Resolve a coarse ASS alignment grid

For each correlated cue, calculate the bitmap rectangle center relative to the
verified canvas. Use deterministic thirds:

- vertical center less than one third: top;
- vertical center from one third to less than two thirds: middle; and
- vertical center at or above two thirds: bottom.

When reliable horizontal geometry exists, apply the analogous left, center,
and right thirds. Map the resulting 3x3 region to ASS alignment tags:

    top:    \an7  \an8  \an9
    middle: \an4  \an5  \an6
    bottom: \an1  \an2  \an3

If horizontal data is not reliable, use the center column. If no geometry is
available, use explicit bottom-center (\an2) or the equivalent existing
default. The resolver emits supported ASS override alignment tags in the
event-level output and does not emit \pos(x,y).

Layout is applied only while serializing OCR-derived ASS. The raw OCR ASS,
when required, is rendered directly from the pre-clean canonical cues plus
geometry. Cleanup receives a geometry-free representation of those canonical
cues; its output is normalized back into cleaned canonical cues while cue
identity retains the original geometry. The final cleaned ASS is then rendered
from the cleaned canonical cues plus that retained geometry. This makes layout
correctness independent of whether SeConv preserves serialized ASS override
tags. The SRT serializer ignores the geometry field, and native ASS/SSA
extraction bypasses the resolver.

Alternative: mutate one global ASS style for each track. Rejected because
simultaneous cues can require different positions. Alternative: use exact
coordinates immediately. Rejected because the first issue scope calls for
coarse, verifiable regions and the current runtime does not provide a stable
coordinate contract to preserve.

### 5. Preserve one canonical result for SRT, ASS, and both

For converted bitmap SRT or ASS, the canonical operation performs recognition,
normalization, geometry acquisition, timing normalization, and correlation
once. For logical both, the same enriched cue slice is sent to:

- the existing text/timing-only SRT serializer; and
- the ASS serializer with the coarse alignment resolver.

Serializers cannot start OCR, reacquire geometry, or select a different
correlation. Artifact naming, logical transform keys, OCR language metadata,
staging, validation, and publication remain owned by the existing canonical
artifact flow.

For direct OCR ASS, raw output uses the pre-clean enriched cues and cleaned
output uses cleaned enriched cues. For shared both, SRT and ASS are rendered
from the same geometry-enriched result and no auxiliary OCR operation is
created. A non-zero segment/sample start affects matching through the local
timeline only; it does not cause a second extraction or recognition.

### 6. Keep runtime and migration impact additive

No database migration is required. Geometry is ephemeral canonical conversion
data unless existing artifact diagnostics already carry the cue result. No new
frontend route or job type is required. Asset Info and Queue continue to
observe the existing artifact status and identity while the generated OCR ASS
content gains coarse layout.

The parser should be implemented without a new external runtime dependency.
The existing Docker FFmpeg/SeConv/Tesseract image remains authoritative; any
capability probe used for an optional VobSub representation must fail closed
to missing-geometry fallback.

## Risks / Trade-offs

- **[Risk]** Bitmap formats can contain incomplete, repeated, or unusual
  composition records. **Mitigation:** validate bounds and timing, retain
  source ordinals, and omit geometry for records that cannot be correlated.
- **[Risk]** OCR can omit or split cues, making timing-only correlation
  ambiguous. **Mitigation:** use one-to-one matching with stable order only as
  a tie-breaker and fall back per cue when uniqueness is not proven.
- **[Risk]** Cleanup could remove or rewrite ASS override tags.
  **Mitigation:** run cleanup retention tests for raw and cleaned outputs and
  keep the layout resolver scoped to generated ASS.
- **[Risk]** Parsing VobSub from an embedded container may not be supported by
  the installed FFmpeg path. **Mitigation:** do not assume a muxer or hidden
  metadata; report unavailable geometry and preserve valid bottom-center OCR.
- **[Risk]** Parsing every bitmap event adds CPU and memory on the NAS.
  **Mitigation:** parse the already extracted representation once, bound input
  sizes/counts, and avoid any additional OCR or media scan.

## Migration Plan

Deploy the backend change without schema migration. Existing SRT, native
ASS/SSA, sidecar names, and artifact records remain readable. New OCR ASS
generation adds event-level alignment tags; previously generated sidecars are
not rewritten automatically. If the geometry provider or ASS layout resolver
is disabled during rollback, the canonical pipeline returns to its existing
bottom-center OCR ASS output without affecting source media or existing
sidecars.
