## Purpose

Provide one observable subtitle conversion contract so Asset Info, Track Profiles,
Profile Lab, and Queue produce compatible SRT, ASS, and original SSA artifacts.

## ADDED Requirements

### Requirement: Canonical converted subtitle formats
The system SHALL resolve a converted subtitle request through the canonical
pipeline and produce SRT or ASS according to the requested format. SRT SHALL be
the default when no converted format is explicitly selected.

#### Scenario: Converted SRT
- **WHEN** a text or OCR-capable bitmap subtitle is requested as converted SRT
- **THEN** the canonical pipeline produces a `.srt` artifact with normalized cues

#### Scenario: Converted ASS
- **WHEN** a text or OCR-capable bitmap subtitle is requested as converted ASS
- **THEN** the canonical pipeline produces a `.ass` artifact with the converted cues

#### Scenario: Default format
- **WHEN** a converted subtitle request omits an output format
- **THEN** the pipeline selects SRT and preserves existing SRT behavior

### Requirement: Text and bitmap source handling
The system SHALL route text subtitle streams directly through canonical
conversion and SHALL evaluate PGS and VobSub streams through OCR before deciding
whether SRT or ASS output can be generated. Bitmap streams MUST NOT be rejected
before OCR capability is evaluated.

#### Scenario: Text source
- **WHEN** an SRT, ASS, or SSA-compatible text stream is converted
- **THEN** normalized cues are emitted in the requested SRT or ASS format

#### Scenario: PGS OCR
- **WHEN** a PGS stream is requested as converted SRT or ASS
- **THEN** the stream reaches OCR and its recognized cues feed the requested artifact

#### Scenario: VobSub OCR
- **WHEN** a VobSub stream is requested as converted SRT or ASS
- **THEN** the stream reaches OCR and its recognized cues feed the requested artifact

#### Scenario: Unsupported bitmap conversion
- **WHEN** OCR capability is unavailable or cannot produce a supported result
- **THEN** the operation reports an explicit conversion failure without creating a misleading artifact

### Requirement: Lossless original ASS and SSA extraction
The system SHALL preserve original ASS and SSA representation when original
extraction is requested, including source formatting supported by stream copy,
without routing it through converted normalization.

#### Scenario: Original ASS
- **WHEN** an ASS stream is requested in original mode
- **THEN** an `.ass` artifact is extracted losslessly with its styles, positioning, override tags, and karaoke metadata preserved

#### Scenario: Original SSA
- **WHEN** an SSA stream is requested in original mode
- **THEN** an `.ssa` artifact is extracted losslessly rather than converted to SRT or ASS

### Requirement: Shared output decisions and artifact metadata
Asset Info, Track Profiles, Profile Lab, and Queue SHALL use the same resolved
subtitle source type, mode, format, OCR requirement, stages, extension, naming,
and artifact metadata. A canonical plan SHALL take precedence over entry-point
specific heuristics.

#### Scenario: Entry-point parity
- **WHEN** equivalent subtitle policy is requested from Asset Info, Track Profiles, Lab, and Queue
- **THEN** each entry point selects the same output format, mode, naming, and conversion stages

#### Scenario: Artifact identity
- **WHEN** an artifact is planned or generated
- **THEN** its metadata records whether it is original or converted and its extension matches `.srt`, `.ass`, or original `.ssa`

#### Scenario: Existing canonical artifact satisfaction
- **WHEN** a valid artifact already identified by the existing canonical pipeline satisfies an explicit transform
- **THEN** the canonical plan reuses that artifact while avoiding duplicate extraction or OCR, without introducing new sidecar discovery or matching
