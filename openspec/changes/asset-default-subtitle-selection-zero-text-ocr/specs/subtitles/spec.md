## ADDED Requirements

### Requirement: Bitmap OCR zero-cue and zero-recognition outcomes remain distinct

For a planned PGS or VobSub sidecar, the system SHALL distinguish evidence that
proves no eligible bitmap cues from a completed OCR attempt that recognizes no
text, and from OCR runtime, language, cancellation, malformed-input, and tool
failures. Zero recognition SHALL NOT be treated as proof that a source stream is
empty, decorative, or contains no text glyphs. Diagnostics SHALL identify the
source stream, effective or attempted OCR language, OCR mode and bounded
attempts, cue/event counts when known, and the terminal decision; unavailable
counts SHALL remain explicitly unknown. A required artifact with no recovered
text or no eligible cues SHALL fail actionably without publishing a fabricated
empty SRT/ASS artifact.

A segmented Test Encode whose plan explicitly allows empty subtitle artifacts
MAY omit a PGS sidecar when bounded parsing proves there are no eligible cues.
The omission SHALL be terminally recorded as skipped during Queue artifact
preparation, SHALL not run OCR recognition or publish an empty sidecar, and
SHALL allow the Test Encode to continue. Queue sidecars remain required when
their plan does not allow omission.

#### Scenario: PGS evidence proves there are no eligible cues

- **WHEN** a bounded PGS demux/parse establishes that the selected stream has no presentation events or eligible bitmap cues
- **THEN** OCR and serialization do not run, the failure identifies the known zero event/cue counts and says the required artifact was not created

#### Scenario: Optional segmented Test Encode sidecar has no eligible PGS cues

- **WHEN** a segmented Test Encode plan allows empty subtitle artifacts and bounded PGS parsing proves there are no eligible cues
- **THEN** the sidecar is skipped without OCR recognition or serialization, Queue artifact progress reaches a terminal skipped state, and the Test Encode continues without a fabricated sidecar

#### Scenario: OCR completes with bitmap events but recognizes no text

- **WHEN** OCR completes with the canonical zero-recognition signal for a selected bitmap stream
- **THEN** the operation reports zero recognition with stream identity, effective/attempted language, OCR mode, available bitmap counts, and an actionable failure reason without classifying the source as empty or creating an empty sidecar

#### Scenario: Candidate cue evidence is unavailable

- **WHEN** zero recognition is reported but the source event or cue count cannot be established
- **THEN** the diagnostic states that the count and glyph presence are unknown and does not relabel the result as a no-cue stream

#### Scenario: OCR runtime or tool failure remains distinct

- **WHEN** the OCR language, traineddata, runtime, or child tool is unavailable or fails
- **THEN** the operation reports that runtime/tool failure separately from a completed zero-recognition attempt
