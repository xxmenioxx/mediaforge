## ADDED Requirements

### Requirement: Bitmap cue geometry is preserved for OCR-generated ASS

For a converted bitmap subtitle, the canonical pipeline SHALL retain optional
spatial metadata for each recognized cue independently from its text and
timing. Geometry SHALL be obtained only from a bitmap representation that the
running implementation can actually parse or otherwise verify; stream-level
dimensions or undocumented ffprobe/SeConv fields SHALL NOT be treated as cue
geometry. When geometry is unavailable, invalid, or cannot be correlated
unambiguously, the affected cue SHALL remain valid and use the existing
bottom-center layout fallback.

Spatial metadata SHALL remain authoritative canonical cue metadata through OCR
cleanup. Raw OCR ASS, when produced, SHALL be rendered from the pre-clean
canonical cues plus their geometry. Final cleaned ASS SHALL be rendered, or
have layout applied, from the cleaned canonical cues plus the same retained
geometry; correctness SHALL NOT depend on SeConv preserving already-serialized
ASS alignment tags.

Every geometry event and recognized OCR cue SHALL have timing provenance.
prepared-input-local timing SHALL be used directly. original-source-absolute
timing SHALL be translated exactly once into the prepared-input timeline using
the operation's known preparation mapping. unknown or ambiguous timing
provenance SHALL receive no invented offset and SHALL NOT produce an
authoritative geometry correlation; the affected cue SHALL use the safe
fallback.

#### Scenario: Bitmap cue geometry is available

- **WHEN** a bitmap subtitle cue has verifiable canvas and bitmap bounds and
  the recognized cue can be correlated to that source event
- **THEN** the canonical cue result retains the cue's relative spatial region
  for an OCR-generated ASS serializer without changing its text or timing

#### Scenario: Geometry is unavailable

- **WHEN** the source representation or runtime does not provide parseable cue
  geometry, or the geometry provider cannot establish a reliable correlation
- **THEN** the OCR conversion succeeds with the existing bottom-center ASS
  fallback and does not invent coordinates or fail solely because metadata is
  unavailable

#### Scenario: Simultaneous cues remain distinct

- **WHEN** two bitmap cues overlap in time but occur in different source
  regions
- **THEN** the canonical result keeps both recognized cues as separate events
  and assigns their independently correlated coarse regions without merging
  them because their timestamps overlap

#### Scenario: Prepared-input-local timings are not double-adjusted

- **WHEN** bitmap source events and recognized OCR cues are produced from a
  prepared or segmented input with a non-zero segment or sample start and both
  are marked prepared-input-local
- **THEN** both timing sets are matched directly in that local timeline, the
  segment/sample start is not applied again, and the expected geometry remains
  associated with the cue rather than being shifted by a second offset

#### Scenario: Original-source-absolute timing is translated once

- **WHEN** bitmap source-event timing is original-source-absolute and recognized
  OCR cue timing is prepared-input-local
- **THEN** the source event is translated exactly once through the known
  preparation mapping before correlation, without changing emitted subtitle
  timing

#### Scenario: Ambiguous timing provenance falls back safely

- **WHEN** geometry-event or OCR-cue timing provenance is unknown or ambiguous
- **THEN** no offset is invented, no authoritative geometry match is made, and
  the affected OCR ASS cue uses the documented safe fallback

### Requirement: OCR-generated ASS uses deterministic coarse layout

OCR-generated ASS SHALL consume retained bitmap cue geometry through a
deterministic coarse relative-region resolver. The first layout contract SHALL
use vertical thirds and, when reliable horizontal data is available, left,
center, or right alignment mapped to supported ASS alignment tags. It SHALL
not claim exact per-pixel positioning. Newly generated raw OCR ASS and the
cleaned OCR ASS SHALL retain the resolved layout metadata.

#### Scenario: Upper-region cue

- **WHEN** a correlated bitmap cue's relative center is in the upper screen
  region
- **THEN** OCR-generated ASS places that event at a top alignment while
  preserving its recognized text, line breaks, and timing

#### Scenario: Middle-region cue

- **WHEN** a correlated bitmap cue's relative center is in the middle screen
  region
- **THEN** OCR-generated ASS places that event at a middle alignment while
  preserving its recognized text, line breaks, and timing

#### Scenario: Lower-region cue

- **WHEN** a correlated bitmap cue's relative center is in the lower screen
  region
- **THEN** OCR-generated ASS places that event at a bottom alignment while
  preserving its recognized text, line breaks, and timing

#### Scenario: Cleanup retains layout

- **WHEN** OCR-generated ASS passes through the existing cleanup mode
- **THEN** cleanup operates on canonical cue text/timing while geometry remains
  attached to cue identity, and the cleaned ASS is rendered with each retained
  coarse alignment rather than relying on SeConv to preserve serialized ASS
  tags

#### Scenario: Raw and cleaned ASS use the same geometry

- **WHEN** an OCR conversion produces both a raw ASS representation and a
  cleaned ASS artifact
- **THEN** raw ASS uses pre-clean canonical cues plus geometry, cleaned ASS uses
  cleaned canonical cues plus that same geometry, and cleanup cannot silently
  replace spatial metadata with its default style

### Requirement: Spatial metadata does not alter SRT or native subtitle behavior

The canonical SRT serializer SHALL remain text-and-timing only and SHALL
ignore bitmap spatial metadata. Original ASS and SSA extraction SHALL remain
lossless and SHALL not consume OCR geometry resolution. A logical converted
both request SHALL perform one canonical bitmap recognition and produce SRT
and ASS from the same recognized cue result, with only the ASS serializer
consuming spatial metadata.

#### Scenario: SRT remains text and timing only

- **WHEN** a bitmap subtitle is converted to SRT with correlated geometry
- **THEN** the SRT contains the recognized text and timing without ASS
  alignment or position tags, and geometry does not alter the artifact identity

#### Scenario: Native ASS is unchanged

- **WHEN** an original ASS subtitle is extracted
- **THEN** the existing native ASS representation, styles, override tags,
  positioning, and timing are preserved without OCR geometry processing

#### Scenario: Native SSA is unchanged

- **WHEN** an original SSA subtitle is extracted
- **THEN** the existing native SSA representation is preserved without
  conversion or bitmap layout inference

#### Scenario: Shared both consumes one enriched cue result

- **WHEN** a bitmap subtitle requests converted SRT and ASS as one logical
  both operation
- **THEN** recognition and geometry correlation occur once, SRT serializes the
  shared text/timing result, ASS serializes the same result with coarse layout,
  and neither serializer starts another OCR operation
