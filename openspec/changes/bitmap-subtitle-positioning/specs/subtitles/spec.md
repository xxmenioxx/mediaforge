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
- **THEN** the cleaned ASS retains each resolved coarse alignment and does not
  collapse positioned events to one default style

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
